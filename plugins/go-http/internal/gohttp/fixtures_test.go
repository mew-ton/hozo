package gohttp

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
	"sync"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// このファイルは、単体テストの入力と、ゴールデンの比べ方（docs/testing.md「単体テスト」の `plugins/go-http`）。
//
// 入力は testdata/<名前>/ の小さなプロジェクト（hozo.toml と .hozo）。このパッケージは internal/ を import できない
// （docs/packages.md「依存の向き」）ので、.hozo を自分では解析しない: hozoc がトランスパイラに渡すのと同じ View を、
// tests/pluginviews_test.go の TestPluginViewFixtures が testdata/<名前>/views.json に置き、ここではそれを読む。
// 期待する出力は testdata/<名前>/expected/（プロジェクトのルートからの相対パスの木）。仕様を変えたら
// `go test ./tests -run TestPluginViewFixtures -update`、その後に `go test ./plugins/go-http/... -update`。
//
// go-http の出力は、View だけから作る（基底と go-valid の応答を受け取らない。docs/plugin-protocol.md「起動」）。出力がコンパイル
// できることと、振る舞いを確かめるときだけ、基底 go と go-valid の実行ファイルを、hozoc と同じに（標準入出力の JSON で）起動し、
// その出力と合わせる: 相乗りが、依存するトランスパイラと同じ置き場と名前を、自分で求められていることの確かめになる。

var update = flag.Bool("update", false, "ゴールデン（testdata/<名前>/expected/）を書き直す")

// modulePath は、テストが作るプロジェクトのモジュールのパス（import パスに現れる）。
const modulePath = "example.com/app"

// repoRoot はリポジトリのルート（生成したコードが import する runtime と、基底のプラグインの置き場）。
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

// request は、hozoc がトランスパイラ transpiler に送る emit の要求（docs/plugin-protocol.md「emit」）。
func request(view *ir.View, transpiler string) *plugin.EmitRequest {
	return &plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", Transpiler: transpiler, View: view}
}

// listed は、view の単位が、transpilers に transpiler を並べているか（hozoc が、その出力に transpiler を起動するか）。
func listed(view *ir.View, transpiler string) bool {
	for _, name := range view.Config.Transpilers {
		if name == transpiler {
			return true
		}
	}
	return false
}

// emitViews は、views のうち go-http を並べた単位のものを、root のプロジェクトとして出力し、ファイル（パス → 内容）と、
// 診断と、warning を集める。
func emitViews(t *testing.T, root string, views []*ir.View) (map[string]string, []plugin.Diagnostic, []plugin.Warning) {
	t.Helper()
	files := map[string]string{}
	var diags []plugin.Diagnostic
	var warnings []plugin.Warning
	for _, v := range views {
		if !listed(v, TranspilerName) {
			continue
		}
		out, err := Emit(request(v, TranspilerName))
		if err != nil {
			t.Fatalf("%s: %v", v.Viewer, err)
		}
		diags = append(diags, out.Errors...)
		warnings = append(warnings, out.Warnings...)
		if len(out.Errors) > 0 && (len(out.Files) > 0 || len(out.Warnings) > 0) {
			t.Errorf("%s: 診断があるのに、ファイルか warning を返した", v.Viewer)
		}
		for _, f := range out.Files {
			if _, dup := files[f.Path]; dup {
				t.Errorf("二つの出力が同じパス %s を返した", f.Path)
			}
			files[f.Path] = f.Content
		}
	}
	return files, diags, warnings
}

// emitFixture は、入力 name を新しいプロジェクトのルートで出力する。診断があれば失敗。
func emitFixture(t *testing.T, name string) (root string, files map[string]string) {
	t.Helper()
	root = newRoot(t)
	files, diags, _ := emitViews(t, root, loadViews(t, name))
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

// isTestMap は、パス p が、このトランスパイラのテストの対応表（docs/plugin-go-http.md「インテグレーションテストの口」の「置き場と名前」）か。
func isTestMap(p string) bool { return path.Base(p) == plugin.TestMapName(TranspilerName) }

// scanTestRuns は、Go のソース src（置き場は file）の、いちばん内側の `t.Run(<名前>, func(t *testing.T) {` … `})` を、
// 字下げだけを頼りに探す（go/parser を使う golang.TestMap とは、別の読み方）: 名前は、外の段から / でつなぎ、行は、
// 開く行の次の行から、閉じる行の前の行まで（plugins/go/spec/go.hozo の Requirement.UnitTestPort の「テストの対応表」）。
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

// checkTestMaps は、出力のテストの対応表を、出力の Go のテキストに当てて確かめる: テストのパッケージのファイル（http.go）が
// あるディレクトリにだけ、一つあり、中身は、そのファイルの、いちばん内側の t.Run を、行の順に、テスト一つを一行で並べたもの
// （plugins/go/spec/go.hozo の Requirement.UnitTestPort の「テストの対応表」）。
func checkTestMaps(t *testing.T, files map[string]string) {
	t.Helper()
	want := map[string]string{} // 対応表のパス → 中身
	for p, content := range files {
		if isTestMap(p) || !strings.Contains(content, "t.Run(") {
			continue
		}
		tests := scanTestRuns(t, p, content)
		if len(tests) == 0 {
			t.Errorf("%s が t.Run を持つが、テストが一つもない", p)
			continue
		}
		f := plugin.TestMapFile(path.Dir(p), TranspilerName, tests)
		want[f.Path] = f.Content
	}
	for _, p := range sortedPaths(files) {
		if !isTestMap(p) {
			continue
		}
		expected, ok := want[p]
		delete(want, p)
		if !ok {
			t.Errorf("%s: t.Run のないディレクトリに、対応表を出した", p)
			continue
		}
		if files[p] != expected {
			t.Errorf("%s が、出力の Go のテキストの t.Run と合わない:\n%s", p, firstDiff(expected, files[p]))
		}
	}
	for p := range want {
		t.Errorf("%s を出していない（同じディレクトリのファイルが、t.Run を持つ）", p)
	}
}

// checkFormatted は、生成した Go が go/format を通した形で、一行目がヘッダであること（docs/plugin-go-http.md「出力」）を確かめる。
func checkFormatted(t *testing.T, files map[string]string) {
	t.Helper()
	checkTestMaps(t, files)
	for _, p := range sortedPaths(files) {
		if isTestMap(p) {
			continue
		}
		if filepath.Base(p) != FileName {
			t.Errorf("%s でないファイル %s を出力した", FileName, p)
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
		if !strings.HasPrefix(files[p], "// Code generated by hozoc go-http "+plugin.Version+"; DO NOT EDIT.\n") {
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

// built は、依存するトランスパイラ（基底 go と go-valid）のプラグインの実行ファイル（テストの間に一度だけビルドする）。
var built struct {
	once sync.Once
	dir  string
	err  error
}

// dependencies は、go-http が依存するトランスパイラ（マニフェストの requires）と、そのプラグインのパッケージ。
var dependencies = []struct{ transpiler, pkg string }{
	{"go", "github.com/mew-ton/hozo/plugins/go"},
	{"go-valid", "github.com/mew-ton/hozo/plugins/go-valid"},
}

func TestMain(m *testing.M) {
	code := m.Run()
	if built.dir != "" {
		os.RemoveAll(built.dir)
	}
	os.Exit(code)
}

// dependencyFiles は、views のそれぞれについて、単位に並んだ依存するトランスパイラ（基底 go と go-valid）の実行ファイルを、hozoc と
// 同じに起動し（要求を標準入力に、応答を標準出力から。docs/plugin-protocol.md「起動」）、返ったファイルを集める。go-http の出力と
// 合わせて、コンパイルできることを確かめるためのもの。
func dependencyFiles(t *testing.T, root string, views []*ir.View) map[string]string {
	t.Helper()
	goBin := goTool(t)
	built.once.Do(func() {
		built.dir, built.err = os.MkdirTemp("", "hozo-plugins")
		if built.err != nil {
			return
		}
		for _, dep := range dependencies {
			cmd := exec.Command(goBin, "build", "-o", filepath.Join(built.dir, dep.transpiler), dep.pkg)
			cmd.Dir = repoRoot(t)
			if out, err := cmd.CombinedOutput(); err != nil {
				built.err = fmt.Errorf("プラグイン %s をビルドできない: %v\n%s", dep.transpiler, err, out)
				return
			}
		}
	})
	if built.err != nil {
		t.Fatal(built.err)
	}
	files := map[string]string{}
	for _, v := range views {
		for _, dep := range dependencies {
			if !listed(v, dep.transpiler) {
				continue
			}
			req, err := json.Marshal(request(v, dep.transpiler).Batch())
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(filepath.Join(built.dir, dep.transpiler))
			cmd.Dir = root
			cmd.Env = os.Environ()
			cmd.Stdin = bytes.NewReader(req)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%s（%s）: %v\n%s", dep.transpiler, v.Viewer, err, stderr.String())
			}
			resp, err := plugin.SingleResult(stdout.Bytes())
			if err != nil {
				t.Fatalf("%s（%s）の応答を読めない: %v", dep.transpiler, v.Viewer, err)
			}
			for _, d := range resp.Errors {
				t.Errorf("%s（%s）の診断 %s: %s", dep.transpiler, v.Viewer, d.Code, d.Message)
			}
			for _, f := range resp.Files {
				if _, dup := files[f.Path]; dup {
					t.Errorf("二つのトランスパイラが同じパス %s を返した", f.Path)
				}
				files[f.Path] = f.Content
			}
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	return files
}

// compose は、root に、依存するトランスパイラの出力と、go-http の出力 files を書く（hozoc build が書き出すものと同じ木）。
func compose(t *testing.T, root string, views []*ir.View, files map[string]string) {
	t.Helper()
	deps := dependencyFiles(t, root, views)
	for p := range files {
		if _, clash := deps[p]; clash {
			t.Errorf("依存するトランスパイラと同じパス %s を返した（CFG-5）", p)
		}
	}
	writeFiles(t, root, deps)
	writeFiles(t, root, files)
}

// vet は、依存するトランスパイラの出力と合わせて、生成したコードが `go vet` を通る（コンパイルできる）ことを確かめる。
func vet(t *testing.T, root string, views []*ir.View, files map[string]string) {
	t.Helper()
	compose(t, root, views, files)
	if out, err := runGo(t, root, "vet", "./..."); err != nil {
		t.Errorf("生成したコードが go vet を通らない: %v\n%s", err, out)
	}
}

// noModule は、views の写しで、どの出力の設定も、基底へのオプション module を持たないもの（基底が GO-8 を返す設定。
// docs/plugin-go.md「診断」）。
func noModule(views []*ir.View) []*ir.View {
	strip := func(c ir.NamespaceConfig) ir.NamespaceConfig {
		options := map[string]map[string]any{}
		for name, table := range c.Options {
			if name != c.Base {
				options[name] = table
			}
		}
		c.Options = options
		return c
	}
	out := make([]*ir.View, len(views))
	for i, view := range views {
		v := *view
		v.Config = strip(view.Config)
		v.Namespaces = map[string][]ir.NamespaceConfig{}
		for ns, list := range view.Namespaces {
			for _, c := range list {
				v.Namespaces[ns] = append(v.Namespaces[ns], strip(c))
			}
		}
		out[i] = &v
	}
	return out
}
