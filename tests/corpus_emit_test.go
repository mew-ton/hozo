package tests

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// update はゴールデンを書き直す（docs/testing.md「コーパス」: `go test ./... -update`）。
var update = flag.Bool("update", false, "update golden files (docs/testing.md「コーパス」)")

// corpora はゴールデンを持つコーパス（docs/testing.md「コーパス」）。needsGo は emit が `go run` のプラグインを起動するもの。
// emit が false のコーパスは check のゴールデンだけを持つ（既定の出力がまだ扱えない構文を含む。そのコーパスの README.md）。
var corpora = []struct {
	name    string
	needsGo bool
	emit    bool
}{
	{"design", false, true},
	{"view", true, true}, // plugins/view-dump を `go run` で起動する
	// 状態の分類・遷移の効果・状態での match。internal/emit/golang が GOEMIT-INTERNAL にする（TestCorpusEmitUnsupported）
	{"state", false, false},
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
		if !c.emit {
			continue
		}
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
			if len(got) == 0 {
				t.Fatalf("emit が何も書かなかった:\n%s%s", stdout.String(), stderr.String())
			}
			compareTree(t, filepath.Join(goldenDir(c.name), "emit"), got)
		})
	}
}

// TestCorpusEmitUnsupported は、既定の出力がまだ扱えない構文を含むコーパス（corpora の emit が false のもの）に
// `hozoc emit` が黙って誤ったコードを書かず、GOEMIT-INTERNAL の error で終わり（終了コード 1。docs/diagnostics.md
// 「終了コード」）、何も書き出さないことを確かめる。
// TODO(emit): docs/emit-go.md「state」「classify」「配線の本体」の出力を実装したら、corpora の emit を true にして
// ゴールデンを作り、このテストを消す。
func TestCorpusEmitUnsupported(t *testing.T) {
	for _, c := range corpora {
		if c.emit {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			root, before := copyCorpusAsModule(t, c.name)
			var stdout, stderr bytes.Buffer
			code := hozocMainIn(root, []string{"emit"}, &stdout, &stderr)
			out := stdout.String() + stderr.String()
			if code != 1 {
				t.Errorf("hozoc emit の終了コード %d, want 1\n%s", code, out)
			}
			for _, want := range []string{
				"error[GOEMIT-INTERNAL]: 状態の分類は未対応: classify shop.Checkout",
				"error[GOEMIT-INTERNAL]: 遷移の効果は未対応: effect shop.placeOrder on shop.Checkout",
				"error[GOEMIT-INTERNAL]: 遷移の効果は未対応: effect shop.clearCart on shop.carts",
				"error[GOEMIT-INTERNAL]: 状態での match は未対応: op shop.openTop の match shop.token by nullable",
				"error[GOEMIT-INTERNAL]: 状態での match は未対応: op shop.openCart の match shop.carts by list",
				"error[GOEMIT-INTERNAL]: 状態の分類での match は未対応: op shop.checkout の match by shop.Checkout",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("出力に %q がない:\n%s", want, out)
				}
			}
			got := readTree(t, root)
			for p := range before {
				delete(got, p)
			}
			if len(got) != 0 {
				t.Errorf("error なのにファイルを書いた: %v", sortedKeys(got, nil))
			}
		})
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
