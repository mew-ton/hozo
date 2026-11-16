package tests

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// generatedDirs は cli.hozo からの生成物の置き場（hozo.toml の二つの単位 [[emit]] の out。docs/packages.md「生成するもの」）:
// 名前空間 hozoc の出力（入口、内側の inner、go-cli の cli）と、語彙 proc の例外の型。
var generatedDirs = []string{"internal/gen/hozoc", "internal/gen/proc"}

// TestGeneratedUpToDate は、コミットされた internal/gen/hozoc/** と internal/gen/proc/** が cli.hozo から遅れていないこと
// （docs/testing.md「生成物の一致」、docs/requirements.md「段階」の 3）。`go test ./...` だけで分かるように、
// CI の `make generate && git diff --exit-code` と同じことをここでも確かめる。
//
// リポジトリを一時ディレクトリに写し（cli.hozo、hozo.toml、go.mod、go.sum は複製。プラグインを `go run` で動かすために
// 要る ir/、plugin/、plugins/、runtime/ はシンボリックリンク）、そこで hozoc emit を走らせ、出た置き場の中身を
// コミット済みのものとバイト単位で比べる。go.mod が同じ module なので import パスも同じになる。
// 差があれば `make generate` で生成し直してコミットする。
func TestGeneratedUpToDate(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go がない")
	}
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "go.mod")); err != nil {
		t.Fatalf("リポジトリのルートが見つからない: %v", err)
	}
	root := t.TempDir()
	for _, name := range []string{"cli.hozo", "hozo.toml", "go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{"ir", "plugin", "plugins", "runtime"} {
		if err := os.Symlink(filepath.Join(repo, dir), filepath.Join(root, dir)); err != nil {
			t.Skipf("シンボリックリンクを作れない: %v", err)
		}
	}

	var stdout, stderr bytes.Buffer
	code := hozocMainIn(root, []string{"emit"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("hozoc emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}

	for _, generatedDir := range generatedDirs {
		want := readTree(t, filepath.Join(repo, filepath.FromSlash(generatedDir)))
		got := readTree(t, filepath.Join(root, filepath.FromSlash(generatedDir)))
		if len(want) == 0 {
			t.Fatalf("%s がコミットされていない", generatedDir)
		}
		for _, p := range sortedKeys(want, got) {
			w, inWant := want[p]
			g, inGot := got[p]
			switch {
			case !inGot:
				t.Errorf("%s/%s はコミットされているが、cli.hozo からは生成されない（make generate で消える）", generatedDir, p)
			case !inWant:
				t.Errorf("%s/%s は cli.hozo から生成されるが、コミットされていない（make generate）", generatedDir, p)
			case !bytes.Equal(w, g):
				t.Errorf("%s/%s が cli.hozo から遅れている（make generate）:\n%s", generatedDir, p, firstDiff(string(w), string(g)))
			}
		}
	}
	// 置き場の外には、何も書き出さない
	for p := range readTree(t, filepath.Join(root, "internal")) {
		inPlace := false
		for _, generatedDir := range generatedDirs {
			inPlace = inPlace || strings.HasPrefix("internal/"+p, generatedDir+"/")
		}
		if !inPlace {
			t.Errorf("置き場の外の internal/%s を書き出した", p)
		}
	}
}

// readTree は dir の下の普通のファイルを、dir からの相対パス（"/" 区切り）→ 内容で返す。dir がなければ空。
func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() { // シンボリックリンク（tests/corpus_emit_test.go の copyCorpusAsModule）は読まない
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = b
		return err
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return out
}

func sortedKeys(maps ...map[string][]byte) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

// firstDiff は最初に違う行の前後を示す。
func firstDiff(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("line %d\n  want: %q\n  got:  %q", i+1, w, g)
		}
	}
	return "(no difference)"
}
