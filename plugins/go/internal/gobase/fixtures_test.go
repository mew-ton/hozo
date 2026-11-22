package gobase

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// このファイルは、単体テストの入力と、ゴールデンの比べ方（docs/testing.md「単体テスト」の `plugins/go`）。
//
// 入力は testdata/<名前>/ の小さなプロジェクト（hozo.toml と .hozo）。このパッケージは internal/ を import できない
// （docs/packages.md「依存の向き」）ので、.hozo を自分では解析しない: hozoc がトランスパイラに渡すのと同じ View を、
// tests/pluginviews_test.go の TestPluginViewFixtures が testdata/<名前>/views.json に置き、ここではそれを読む。
// 期待する出力は testdata/<名前>/expected/（プロジェクトのルートからの相対パスの木）。仕様を変えたら
// `go test ./tests -run TestPluginViewFixtures -update`、その後に `go test ./plugins/go/... -update`。

var update = flag.Bool("update", false, "ゴールデン（testdata/<名前>/expected/）を書き直す")

// modulePath は、テストが作るプロジェクトのモジュールのパス（import パスに現れる）。
const modulePath = "example.com/app"

// repoRoot はリポジトリのルート（生成したコードが import する runtime の置き場）。
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// newRoot は、go.mod だけを持つプロジェクトのルートを作る。生成したコードが import する runtime は、リポジトリのものを replace で指す。
func newRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gomod := fmt.Sprintf("module %s\n\ngo 1.24\n\nrequire github.com/mew-ton/hozo v0.0.0\n\nreplace github.com/mew-ton/hozo => %s\n", modulePath, filepath.ToSlash(repoRoot(t)))
	writeFile(t, filepath.Join(root, "go.mod"), gomod)
	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// loadViews は、入力 name の View の列（単位の順、その中は名前空間の辞書順）を読む。
func loadViews(t *testing.T, name string) []*ir.View {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name, "views.json"))
	if err != nil {
		t.Fatalf("%v（`go test ./tests -run TestPluginViewFixtures -update` で作る）", err)
	}
	var views []*ir.View
	if err := json.Unmarshal(data, &views); err != nil {
		t.Fatalf("testdata/%s/views.json を読めない: %v", name, err)
	}
	if len(views) == 0 {
		t.Fatalf("testdata/%s/views.json に View がない", name)
	}
	return views
}

// request は、hozoc が基底 go に送る emit の要求（docs/plugin-protocol.md「emit」）。
func request(view *ir.View) *plugin.EmitRequest {
	return &plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", Transpiler: "go", View: view}
}

// emitViews は、views のそれぞれを root のプロジェクトとして出力し、ファイル（パス → 内容）と、診断を集める。
func emitViews(t *testing.T, root string, views []*ir.View) (map[string]string, []plugin.Diagnostic) {
	t.Helper()
	files := map[string]string{}
	var diags []plugin.Diagnostic
	for _, v := range views {
		var stderr bytes.Buffer
		out, errs := Emit(request(v), &stderr)
		diags = append(diags, errs...)
		if len(errs) > 0 && len(out) > 0 {
			t.Errorf("%s: 診断があるのに、ファイルを返した", v.Viewer)
		}
		if stderr.Len() > 0 {
			t.Errorf("%s: 標準エラーに書いた:\n%s", v.Viewer, stderr.String())
		}
		for i, f := range out {
			if i > 0 && out[i-1].Path >= f.Path {
				t.Errorf("%s: ファイルがパスの辞書順でない: %s、%s", v.Viewer, out[i-1].Path, f.Path)
			}
			if _, dup := files[f.Path]; dup {
				t.Errorf("二つの出力が同じパス %s を返した", f.Path)
			}
			files[f.Path] = f.Content
		}
	}
	return files, diags
}

// emitFixture は、入力 name を新しいプロジェクトのルートで出力する。診断があれば失敗。
func emitFixture(t *testing.T, name string) (root string, files map[string]string) {
	t.Helper()
	root = newRoot(t)
	files, diags := emitViews(t, root, loadViews(t, name))
	for _, d := range diags {
		t.Errorf("診断 %s（%s）: %s", d.Code, d.At, d.Message)
	}
	if t.Failed() {
		t.FailNow()
	}
	return root, files
}

// sortedPaths は files のパスの辞書順。
func sortedPaths(files map[string]string) []string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// readTree は dir の下のファイル（dir からの相対パス → 内容）。dir がなければ空。
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// compareTree は、出力 got を、入力 name のゴールデン testdata/<name>/expected/ と比べる。-update なら、先に書き直す。
func compareTree(t *testing.T, name string, got map[string]string) {
	t.Helper()
	dir := filepath.Join("testdata", name, "expected")
	if *update {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		for p, content := range got {
			writeFile(t, filepath.Join(dir, filepath.FromSlash(p)), content)
		}
	}
	want := readTree(t, dir)
	if len(want) == 0 {
		t.Fatalf("ゴールデン %s がない（-update で作る）", dir)
	}
	for _, p := range sortedPaths(want) {
		if _, ok := got[p]; !ok {
			t.Errorf("%s を出力しなかった", p)
		}
	}
	for _, p := range sortedPaths(got) {
		w, ok := want[p]
		if !ok {
			t.Errorf("ゴールデンにない %s を出力した", p)
			continue
		}
		if got[p] != w {
			t.Errorf("%s がゴールデンと違う（-update で更新）:\n%s", p, firstDiff(w, got[p]))
		}
	}
}

// firstDiff は、want と got の最初に違う行を示す。
func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return fmt.Sprintf("  %d 行目\n    want: %q\n    got:  %q", i+1, wl, gl)
		}
	}
	return "  （違いなし）"
}

// isTestMap は、パス p が、基底のテストの対応表（docs/plugin-go.md「テストの対応表」）か。
func isTestMap(p string) bool { return path.Base(p) == plugin.TestMapName(TranspilerName) }

// goFiles は、files のうち、Go のファイル（テストの対応表を除いたもの）。
func goFiles(files map[string]string) map[string]string {
	out := map[string]string{}
	for p, content := range files {
		if !isTestMap(p) {
			out[p] = content
		}
	}
	return out
}

// scanTestRuns は、Go のソース src（置き場は file）の、いちばん内側の `t.Run(<名前>, func(t *testing.T) {` … `})` を、
// 字下げだけを頼りに探す（go/parser を使う golang.TestMap とは、別の読み方）: 名前は、外の段から / でつなぎ、行は、
// 開く行の次の行から、閉じる行の前の行まで（docs/plugin-go.md「テストの対応表」）。
func scanTestRuns(t *testing.T, file, src string) []plugin.TestEntry {
	t.Helper()
	type run struct {
		indent, start int
		name          string
		parent        bool
	}
	var stack []run
	var out []plugin.TestEntry
	for i, l := range strings.Split(src, "\n") {
		body := strings.TrimLeft(l, "\t")
		indent := len(l) - len(body)
		if strings.HasPrefix(body, "t.Run(") {
			rest := strings.TrimPrefix(body, "t.Run(")
			if !strings.HasSuffix(rest, ", func(t *testing.T) {") {
				t.Fatalf("%s の %d 行目の t.Run が、その場の関数で、行の終わりに { を持つ形でない: %s", file, i+1, l)
			}
			name, err := strconv.Unquote(strings.TrimSuffix(rest, ", func(t *testing.T) {"))
			if err != nil {
				t.Fatalf("%s の %d 行目の t.Run の名前を読めない: %s", file, i+1, l)
			}
			if len(stack) > 0 {
				stack[len(stack)-1].parent = true
			}
			stack = append(stack, run{indent: indent, start: i + 1, name: name})
			continue
		}
		if len(stack) > 0 && body == "})" && indent == stack[len(stack)-1].indent {
			r := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if r.parent {
				continue
			}
			names := make([]string, 0, len(stack)+1)
			for _, s := range stack {
				names = append(names, s.name)
			}
			out = append(out, plugin.TestEntry{Name: strings.Join(append(names, r.name), "/"), File: file, Lines: [2]int{r.start + 1, i}})
		}
	}
	if len(stack) > 0 {
		t.Fatalf("%s の t.Run が、閉じていない", file)
	}
	return out
}

// checkTestMaps は、出力のテストの対応表（docs/plugin-go.md「テストの対応表」）を、出力の Go のテキストに当てて確かめる:
// t.Run を持つ Go のファイルがあるディレクトリにだけ、一つあり、中身は、そのディレクトリの Go のファイル（パスの順は、
// doc.go、leaves.go、classify.go、seams.go: 基底が出す順）の、いちばん内側の t.Run を、行の順に、テスト一つを一行で並べたもの。
func checkTestMaps(t *testing.T, files map[string]string) {
	t.Helper()
	want := map[string][]plugin.TestEntry{} // ディレクトリ → テスト
	for _, name := range []string{fileDoc, fileLeaves, fileClassify, fileSeams} {
		for _, p := range sortedPaths(files) {
			if isTestMap(p) || path.Base(p) != name || !strings.Contains(files[p], "t.Run(") {
				continue
			}
			want[path.Dir(p)] = append(want[path.Dir(p)], scanTestRuns(t, p, files[p])...)
		}
	}
	for p, content := range files {
		if !isTestMap(p) && strings.Contains(content, "t.Run(") && path.Base(p) != fileDoc && path.Base(p) != fileLeaves && path.Base(p) != fileClassify && path.Base(p) != fileSeams {
			t.Errorf("%s が t.Run を持つが、テストのパッケージのファイルでない", p)
		}
	}
	for _, p := range sortedPaths(files) {
		if !isTestMap(p) {
			continue
		}
		tests := want[path.Dir(p)]
		delete(want, path.Dir(p))
		if len(tests) == 0 {
			t.Errorf("%s: t.Run のないディレクトリに、対応表を出した", p)
			continue
		}
		if expected := plugin.TestMapFile(path.Dir(p), TranspilerName, tests).Content; files[p] != expected {
			t.Errorf("%s が、出力の Go のテキストの t.Run と合わない:\n%s", p, firstDiff(expected, files[p]))
		}
	}
	for dir := range want {
		t.Errorf("%s: t.Run を持つのに、対応表を出していない", dir)
	}
}

// checkFormatted は、生成した Go が go/format を通した形であること（docs/plugin-go.md「ファイル」）を確かめる。Go でない
// ファイルは、テストの対応表だけ（中身は checkTestMaps）。
func checkFormatted(t *testing.T, files map[string]string) {
	t.Helper()
	checkTestMaps(t, files)
	for _, p := range sortedPaths(files) {
		if isTestMap(p) {
			continue
		}
		if !strings.HasSuffix(p, ".go") {
			t.Errorf("Go でないファイル %s を出力した", p)
			continue
		}
		formatted, err := format.Source([]byte(files[p]))
		if err != nil {
			t.Errorf("%s を go/format で整形できない: %v", p, err)
			continue
		}
		if string(formatted) != files[p] {
			t.Errorf("%s は go/format を通した形でない", p)
		}
		if !strings.HasPrefix(files[p], "// Code generated by hozoc go "+plugin.Version+"; DO NOT EDIT.\n") {
			t.Errorf("%s の一行目がヘッダでない", p)
		}
	}
}

// goTool は go のコマンド。なければテストを飛ばす。
func goTool(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go が PATH にない")
	}
	return goBin
}

// runGo は、root のモジュールで go のコマンドを走らせる。モジュールの解決はオフライン。
func runGo(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(goTool(t), args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// writeFiles は files を root の下に書く。
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(p)), content)
	}
}

// vet は、files を root に書いて、生成したコードが `go vet` を通る（コンパイルできる）ことを確かめる。
func vet(t *testing.T, root string, files map[string]string) {
	t.Helper()
	writeFiles(t, root, files)
	if out, err := runGo(t, root, "vet", "./..."); err != nil {
		t.Errorf("生成したコードが go vet を通らない: %v\n%s", err, out)
	}
}
