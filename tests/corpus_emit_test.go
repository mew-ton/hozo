package tests

import (
	"bytes"
	"flag"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// update はゴールデンを書き直す（docs/testing.md「コーパス」: `go test ./... -update`）。
var update = flag.Bool("update", false, "update golden files (docs/testing.md「コーパス」)")

// corpora はゴールデンを持つコーパス（docs/testing.md「コーパス」）。needsGo は、出力の Go を、go の道具で確かめるもの。
// noOutput は、emit が何も書かないもの（emit のゴールデンは持たない）。noGoOutput は、書き出すものに Go のファイルがないもの
// （書き出した Go の go vet をしない）。
//
// 書き出すファイルは、どれもトランスパイラが返したもの（docs/plugin-protocol.md「この文書が決めること」）。design / state / units の
// 単位のトランスパイラは、リポジトリの plugins/ の本物の基底 go で、テストが HOZO_PLUGIN_PATH として渡す（hozocCorpus）。
// view は、コーパスの plugins/view-dump。emit のゴールデンは、実装が書いたものを -update で置き、差分をレビューで見る、
// 後戻りの検出のためのもの（サンプルの期待する出力とは役目が違う）。
var corpora = []struct {
	name       string
	needsGo    bool
	noOutput   bool
	noGoOutput bool
}{
	{name: "design", needsGo: true}, // 設計書の例。server と browser と、使う型の置き場の http と page
	// plugins/view-dump（テストのための基底）が、受け取った View を view.json に書き出す
	{name: "view", needsGo: true, noGoOutput: true},
	{name: "state", needsGo: true}, // 状態の分類・遷移の効果・状態での match
	// 単位: 同じ名前空間の二つの単位、in、子孫を名指しする単位、単位をまたぐ一方向の import（docs/config.md「単位」）
	{name: "units", needsGo: true},
	// モデルが先: 出力しない名前空間、宣言に付けた印のデコレータ。単位のトランスパイラ none（テストのための基底。中身は、にせもののプラグイン）は
	// ファイルを返さないので、emit は何も書かない
	{name: "model", noOutput: true},
}

// repoPlugins は、リポジトリの plugins/ の絶対パス（HOZO_PLUGIN_PATH として渡す）。
func repoPlugins(t *testing.T) string {
	t.Helper()
	plugins, err := filepath.Abs(repoPluginsDir)
	if err != nil {
		t.Fatal(err)
	}
	return plugins
}

// hozocCorpus は、コーパス（か、それを写したディレクトリ）dir で hozoc を走らせる。リポジトリの plugins/ を HOZO_PLUGIN_PATH として
// 渡す（docs/testing.md「コーパス」: 単位のトランスパイラは、リポジトリの plugins/ の本物）。ほかの環境は渡さない。
func hozocCorpus(t *testing.T, dir string, args []string, stdout, stderr io.Writer) int {
	t.Helper()
	return hozocMainEnv(dir, pluginPathEnv(repoPlugins(t)), args, stdout, stderr)
}

// corpusDir はコーパス name のディレクトリ（tests/ からの相対パス）。
func corpusDir(name string) string { return filepath.Join("corpus", name) }

// goldenDir はコーパス name のゴールデンの置き場 `tests/corpus/<名前>/testdata/expected/`（docs/testing.md「コーパス」）。
// ゴールデンの .go を `go build ./...` / `go vet ./...` がパッケージとして拾わないように、Go が無視する testdata の下に置く。
func goldenDir(name string) string { return filepath.Join(corpusDir(name), "testdata", "expected") }

// TestCorpusCheckGolden は各コーパスの `hozoc build --no-emit` の text 出力（warning を含む）がゴールデン
// `tests/corpus/<名前>/testdata/expected/check.txt` と一致すること（docs/testing.md「コーパス」）。error が出れば失敗。
func TestCorpusCheckGolden(t *testing.T) {
	for _, c := range corpora {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := hozocCorpus(t, corpusDir(c.name), []string{"build", "--no-emit"}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("hozoc build --no-emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
			}
			golden := filepath.Join(goldenDir(c.name), "check.txt")
			compareGolden(t, golden, stdout.Bytes())
		})
	}
}

// TestCorpusEmitGolden は各コーパスの `hozoc build` が書き出すファイルの木が、ゴールデン `tests/corpus/<名前>/testdata/expected/emit/`
// とバイト単位で一致すること（docs/testing.md「コーパス」）。コーパスを一時ディレクトリに写して emit し
// （copyCorpusAsModule）、新しくできたファイルをすべて比べる。差分はファイルごとに示す。更新は `go test ./tests -update`。
func TestCorpusEmitGolden(t *testing.T) {
	for _, c := range corpora {
		t.Run(c.name, func(t *testing.T) {
			if c.needsGo {
				requireGo(t)
			}
			root, before := copyCorpusAsModule(t, c.name)
			var stdout, stderr bytes.Buffer
			code := hozocCorpus(t, root, []string{"build"}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("hozoc build の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
			}
			got := readTree(t, root)
			for p := range before {
				delete(got, p) // 入力（.hozo、hozo.toml、go.mod など）は書き出したものではない
			}
			if c.noOutput {
				// どのトランスパイラもファイルを返さない: 終了コード 0 で、何も書かない（.hozoc-outputs.json も置かない）
				// stdout に出てよいのは、CFG-26 の warning（と、その要約）だけ: コーパスの check.txt のゴールデンが持つもの
				if rest, _ := splitUninterpreted(stdout.String()); len(got) != 0 || rest != "" {
					t.Errorf("emit が書いた: %v\n%s", sortedKeys(got), stdout.String())
				}
				if _, err := os.Stat(filepath.Join(goldenDir(c.name), "emit")); !os.IsNotExist(err) {
					t.Errorf("%s は何も出力しないのに、emit のゴールデンがある", c.name)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("emit が何も書かなかった:\n%s%s", stdout.String(), stderr.String())
			}
			compareTree(t, filepath.Join(goldenDir(c.name), "emit"), got)
			if !c.noGoOutput {
				vetEmitted(t, root, got)
			}
		})
	}
}

// vetEmitted は、emit が root に書き出した Go のパッケージ（got のうち .go を持つディレクトリ）が `go vet` を通ることを
// 確かめる。ゴールデンと一致していても、コンパイルできない出力では意味がないため。root は copyCorpusAsModule の
// モジュール（module は github.com/mew-ton/hozo）で、生成したコードが import する runtime/ をここでシンボリックリンクに
// して足す（比較の後なので、書き出したファイルの木には入らない）。モジュールの解決はオフライン。go がなければ飛ばす。
func vetEmitted(t *testing.T, root string, got map[string][]byte) {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Log("go がないので、書き出した Go の go vet を飛ばす")
		return
	}
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, "runtime"), filepath.Join(root, "runtime")); err != nil {
		t.Fatalf("runtime のシンボリックリンクを作れない: %v", err)
	}
	seen := map[string]bool{}
	var pkgs []string
	for _, p := range sortedKeys(got) {
		if dir := path.Dir(p); strings.HasSuffix(p, ".go") && !seen[dir] {
			seen[dir] = true
			pkgs = append(pkgs, "./"+dir)
		}
	}
	if len(pkgs) == 0 {
		t.Fatal("emit が Go のファイルを書かなかった")
	}
	cmd := exec.Command(goBin, append([]string{"vet"}, pkgs...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("emit が書き出した Go が go vet を通らない（%s）: %v\n%s", strings.Join(pkgs, " "), err, out)
	}
}

// requireGo は `go` が PATH になければテストを飛ばす（出力の Go を、go の道具で確かめるため）。
func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go がない")
	}
}

// copyCorpusAsModule はコーパス name を一時ディレクトリに写し、そこを Go のモジュールのルートにする:
// リポジトリの go.mod と go.sum を複製し、ir/、plugin/、plugins/、tests/ をシンボリックリンクで足す（tests/generated_test.go と
// 同じやり方。コーパスが自分の plugins/ を持つなら、そちらを使い、リポジトリの plugins/ は足さない）。
// こうすると、生成したコード（import パスは、hozo.toml の [options.go] の module から決まる。docs/plugin-go.md「パッケージと
// import パス」）を、リポジトリと同じモジュールのパッケージとして、go の道具で確かめられる。プラグインは、マニフェストの
// wasm（TestMain がビルドしたもの）を動かすので、作業ディレクトリにも、go.mod にも、依らない。
// `testdata/`（ゴールデン）は写さない。戻り値は一時ディレクトリと、emit の前にあったファイル（readTree の形）。
func copyCorpusAsModule(t *testing.T, name string) (string, map[string][]byte) {
	t.Helper()
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	src := corpusDir(name)
	err = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "testdata" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(root, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(repo, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{"ir", "plugin", "plugins", "tests"} {
		if _, err := os.Lstat(filepath.Join(root, dir)); err == nil {
			continue // コーパスが自分で持つディレクトリ（plugins/）
		}
		if err := os.Symlink(filepath.Join(repo, dir), filepath.Join(root, dir)); err != nil {
			t.Skipf("シンボリックリンクを作れない: %v", err)
		}
	}
	return root, readTree(t, root)
}

// compareGolden は got をゴールデン path と比べる。-update ならまず書き直す。
func compareGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ゴールデン %s を読めない（-update で作る）: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s がゴールデンと違う（-update で更新）:\n%s", path, firstDiff(string(want), string(got)))
	}
}

// compareTree は書き出したファイルの木 got をゴールデンのディレクトリ dir と比べる。-update なら dir を got で置き換える。
func compareTree(t *testing.T, dir string, got map[string][]byte) {
	t.Helper()
	if *update {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		for p, content := range got {
			abs := filepath.Join(dir, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	want := readTree(t, dir)
	if len(want) == 0 {
		t.Fatalf("ゴールデン %s がない（-update で作る）", dir)
	}
	for _, p := range sortedKeys(want, got) {
		w, inWant := want[p]
		g, inGot := got[p]
		switch {
		case !inGot:
			t.Errorf("%s/%s はゴールデンにあるが、emit は書かない（-update で更新）", dir, p)
		case !inWant:
			t.Errorf("%s/%s は emit が書くが、ゴールデンにない（-update で更新）", dir, p)
		case !bytes.Equal(w, g):
			t.Errorf("%s/%s がゴールデンと違う（-update で更新）:\n%s", dir, p, firstDiff(string(w), string(g)))
		}
	}
}
