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

// generatedDirs は spec/cli.hozo からの生成物の置き場（hozo.toml の二つの単位 [[emit]] の out。docs/packages.md「生成するもの」）:
// 名前空間 hozo.compiler（ユースケース）の出力（入口、内側の inner）と、名前空間 hozo.cli（コマンドの行）の出力（入口、go-cli の cli）。
var generatedDirs = []string{"internal/gen/hozo/compiler", "internal/gen/hozo/cli"}

// TestGeneratedUpToDate は、コミットされた internal/gen/hozo/compiler/** と internal/gen/hozo/cli/** が spec/cli.hozo から遅れていないこと
// （docs/testing.md「生成物の一致」、docs/requirements.md「段階」の 3）。`go test ./...` だけで分かるように、
// CI の `make generate && git diff --exit-code` と同じことをここでも確かめる。
//
// リポジトリを一時ディレクトリに写し（spec/*.hozo と spec/decisions/*.hozo、hozo.toml、go.mod、go.sum は複製。プラグインの置き場 plugins/ と、
// ir/、plugin/、runtime/ はシンボリックリンク。プラグインの WASM は、TestMain がビルドしたもの）、そこで hozoc build を走らせ、出た置き場の中身を
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
	// 仕様のファイルは、spec/*.hozo と spec/decisions/*.hozo を、すべて（spec/cli.hozo の宣言が、設計だけのファイルの宣言を指す。
	// docs/packages.md「自己生成の手順」の 2）
	names := []string{"hozo.toml", "go.mod", "go.sum"}
	for _, pattern := range []string{filepath.Join("spec", "*.hozo"), filepath.Join("spec", "decisions", "*.hozo")} {
		specs, err := filepath.Glob(filepath.Join(repo, pattern))
		if err != nil || len(specs) == 0 {
			t.Fatalf("仕様のファイル %s が見つからない: %v", pattern, err)
		}
		for _, s := range specs {
			rel, err := filepath.Rel(repo, s)
			if err != nil {
				t.Fatal(err)
			}
			names = append(names, rel)
		}
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
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
	code := hozocMainIn(root, []string{"build"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("hozoc build の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}

	compareGenerated(t, repo, root, generatedDirs, "spec/cli.hozo")
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

// pluginProjects は、プラグインのプロジェクト（plugins/<名前>/hozo.toml。internal/tools/selfgen の pluginProjects と同じ）。
// 置き場は plugins/<名前>/internal/spec（docs/plugin-go-http.md「go-http 自身の要求」、docs/plugin-go-sqlite.md「go-sqlite 自身の要求」）。
var pluginProjects = []string{"go-http", "go-sqlite"}

// TestGeneratedPluginSpecsUpToDate は、プラグインのプロジェクトごとに、コミットされた plugins/<名前>/internal/spec/** が、
// そのプラグイン自身の要求の仕様 plugins/<名前>/spec/<名前>.hozo から遅れていないこと（docs/testing.md「生成物の一致」）。
// リポジトリの中身を一時ディレクトリに、置き場への道の上のディレクトリだけを本物にし、仕様の置き場 plugins/<名前>/spec は複製し、
// ほかをシンボリックリンクで足して（mirrorDir。置き場そのものは足さない）、その plugins/<名前> で hozoc build を走らせ、出た置き場の中身を、コミット済みのものとバイト単位で比べる。
func TestGeneratedPluginSpecsUpToDate(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go がない")
	}
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range pluginProjects {
		t.Run(name, func(t *testing.T) {
			dirs := []string{"plugins/" + name + "/internal/spec"}
			root := t.TempDir()
			if err := mirrorDir(repo, root, ".", dirs, []string{"plugins/" + name + "/spec"}); err != nil {
				t.Skipf("一時ディレクトリに写せない: %v", err)
			}
			var stdout, stderr bytes.Buffer
			if code := hozocMainIn(filepath.Join(root, "plugins", name), []string{"build"}, &stdout, &stderr); code != 0 {
				t.Fatalf("plugins/%s で hozoc build の終了コード %d\n%s%s", name, code, stdout.String(), stderr.String())
			}
			compareGenerated(t, repo, root, dirs, "plugins/"+name+"/spec/"+name+".hozo")
		})
	}
}

// compareGenerated は、root に生成した置き場 dirs の中身が、リポジトリ repo のコミット済みのものと、バイト単位で同じこと。
// source は、メッセージに出す、生成の元の仕様。
func compareGenerated(t *testing.T, repo, root string, dirs []string, source string) {
	t.Helper()
	for _, generatedDir := range dirs {
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
				t.Errorf("%s/%s はコミットされているが、%s からは生成されない（make generate で消える）", generatedDir, p, source)
			case !inWant:
				t.Errorf("%s/%s は %s から生成されるが、コミットされていない（make generate）", generatedDir, p, source)
			case !bytes.Equal(w, g):
				t.Errorf("%s/%s が %s から遅れている（make generate）:\n%s", generatedDir, p, source, firstDiff(string(w), string(g)))
			}
		}
	}
}

// mirrorDir は、repo の dir（"/" 区切りの相対パス）を、root に足す: 置き場（outs）への道の上のディレクトリは、本物の
// ディレクトリにして中を同じに足し、置き場そのものは足さない（hozoc build が書く）。仕様のファイルの置き場（copies）は、
// 中のファイルを複製する（hozo.toml の sources の glob は、シンボリックリンクに合わない）。ほかは、シンボリックリンク
// （docs/packages.md「自己生成の手順」の 2。internal/tools/selfgen の mirror と同じ）。
func mirrorDir(repo, root, dir string, outs, copies []string) error {
	onPath := dir == "."
	for _, c := range copies {
		if c == dir {
			return copyDir(filepath.Join(repo, filepath.FromSlash(dir)), filepath.Join(root, filepath.FromSlash(dir)))
		}
		if strings.HasPrefix(c, dir+"/") {
			onPath = true
		}
	}
	for _, out := range outs {
		if out == dir {
			return nil
		}
		if dir == "." || strings.HasPrefix(out, dir+"/") {
			onPath = true
		}
	}
	if !onPath {
		return os.Symlink(filepath.Join(repo, filepath.FromSlash(dir)), filepath.Join(root, filepath.FromSlash(dir)))
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(repo, filepath.FromSlash(dir)))
	if err != nil {
		return err
	}
	for _, e := range entries {
		next := e.Name()
		if dir != "." {
			next = dir + "/" + e.Name()
		}
		if err := mirrorDir(repo, root, next, outs, copies); err != nil {
			return err
		}
	}
	return nil
}

// copyDir は、ディレクトリ src の下を、dst に複製する（ディレクトリと普通のファイルだけ。internal/tools/selfgen の copyTree と同じ）。
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
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
