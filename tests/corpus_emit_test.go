package tests

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// update はゴールデンを書き直す（docs/testing.md「コーパス」: `go test ./... -update`）。
var update = flag.Bool("update", false, "update golden files (docs/testing.md「コーパス」)")

// corpora はゴールデンを持つコーパス（docs/testing.md「コーパス」）。needsGo は emit が `go run` のプラグインを起動するもの。
// noOutput は、emit が何も書かないもの（emit のゴールデンは持たない）。
//
// 既定の出力は、プラグインが選んだ宣言だけを出す（docs/emit-go.md「出力の対象」）。emit のゴールデンを持つコーパスは、
// どれも選ぶプラグインを設定している: design / state / select は tests/plugins/pick（コーパスの plugins/pick/pick.json の
// デコレータで選ぶ）、view は plugins/view-dump（@t.entry の op を起点にする）。
var corpora = []struct {
	name     string
	needsGo  bool
	noOutput bool
}{
	{name: "design", needsGo: true}, // 語彙のデコレータ（http / page / db）で選ぶ
	{name: "view", needsGo: true},   // plugins/view-dump。api はプラグインがないので何も出力しない
	{name: "state", needsGo: true},  // 状態の分類・遷移の効果・状態での match。すべての op と状態を選ぶ
	{name: "select", needsGo: true}, // 出力の対象: 起点、stops、wraps、states、別の名前空間の op
	// モデルが先: 名前空間のデコレータ、出力しない名前空間、印のデコレータ。出力する名前空間 shop にプラグインがないので、
	// emit は何も書かない（docs/emit-go.md「出力の対象」）
	{name: "model", noOutput: true},
}

// corpusDir はコーパス name のディレクトリ（tests/ からの相対パス）。
func corpusDir(name string) string { return filepath.Join("corpus", name) }

// goldenDir はコーパス name のゴールデンの置き場 `tests/corpus/<名前>/testdata/expected/`（docs/testing.md「コーパス」）。
// ゴールデンの .go を `go build ./...` / `go vet ./...` がパッケージとして拾わないように、Go が無視する testdata の下に置く。
func goldenDir(name string) string { return filepath.Join(corpusDir(name), "testdata", "expected") }

// TestCorpusCheckGolden は各コーパスの `hozoc check` の text 出力（warning を含む）がゴールデン
// `tests/corpus/<名前>/testdata/expected/check.txt` と一致すること（docs/testing.md「コーパス」）。error が出れば失敗。
func TestCorpusCheckGolden(t *testing.T) {
	for _, c := range corpora {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := hozocMainIn(corpusDir(c.name), []string{"check"}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("hozoc check の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
			}
			golden := filepath.Join(goldenDir(c.name), "check.txt")
			compareGolden(t, golden, stdout.Bytes())
		})
	}
}

// TestCorpusEmitGolden は各コーパスの `hozoc emit` が書き出すファイルの木が、ゴールデン `tests/corpus/<名前>/testdata/expected/emit/`
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
			code := hozocMainIn(root, []string{"emit"}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("hozoc emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
			}
			got := readTree(t, root)
			for p := range before {
				delete(got, p) // 入力（.hozo、hozo.toml、go.mod など）は書き出したものではない
			}
			if c.noOutput {
				// 選ぶプラグインがない: 終了コード 0 で、何も書かない（.hozoc-outputs.json も置かない）
				if len(got) != 0 || stdout.String() != "" {
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
			vetEmitted(t, root, got)
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

// requireGo は `go` が PATH になければテストを飛ばす（`go run` で動くプラグインのため）。
func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go がない")
	}
}

// copyCorpusAsModule はコーパス name を一時ディレクトリに写し、そこを Go のモジュールのルートにする:
// リポジトリの go.mod と go.sum を複製し、ir/、plugin/、tests/ をシンボリックリンクで足す（tests/generated_test.go と同じやり方）。
// こうすると、コーパスのプラグインのマニフェストの `go run github.com/mew-ton/hozo/tests/corpus/<名前>/plugins/<p>` が、
// プロジェクトのルート（= 作業ディレクトリ）から同じモジュールのパッケージとして解決でき、既定の出力の import パス
// （docs/emit-go.md「配置」: out の上の go.mod の module）も `github.com/mew-ton/hozo/<out>` に決まる。
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
	for _, dir := range []string{"ir", "plugin", "tests"} {
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
