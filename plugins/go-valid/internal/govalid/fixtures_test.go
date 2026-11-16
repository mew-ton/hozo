package govalid

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// このファイルは、単体テストの入力と、ゴールデンの比べ方（docs/testing.md「単体テスト」の `plugins/go-valid`）。
//
// 入力は testdata/<名前>/ の小さなプロジェクト（hozo.toml と .hozo）。このパッケージは internal/ を import できない
// （docs/packages.md「依存の向き」）ので、.hozo を自分では解析しない: hozoc がトランスパイラに渡すのと同じ View を、
// tests/pluginviews_test.go の TestPluginViewFixtures が testdata/<名前>/views.json に置き、ここではそれを読む。
// 期待する出力は testdata/<名前>/expected/（プロジェクトのルートからの相対パスの木）。仕様を変えたら
// `go test ./tests -run TestPluginViewFixtures -update`、その後に `go test ./plugins/go-valid/... -update`。
//
// go-valid の出力は、View だけから作る（基底の応答を受け取らない。docs/plugin-protocol.md「起動」）。出力がコンパイルできることと、
// 振る舞いを確かめるときだけ、基底 go の実行ファイルを、hozoc と同じに（標準入出力の JSON で）起動し、その出力と合わせる:
// 相乗りが、基底と同じ置き場と名前を、自分で求められていることの確かめになる。

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

// listed は、view の単位が、transpilers に go-valid を並べているか（hozoc が go-valid を起動する出力か）。
func listed(view *ir.View) bool {
	for _, name := range view.Config.Transpilers {
		if name == TranspilerName {
			return true
		}
	}
	return false
}

// emitViews は、views のうち go-valid を並べた単位のものを、root のプロジェクトとして出力し、ファイル（パス → 内容）と、
// 診断を集める。
func emitViews(t *testing.T, root string, views []*ir.View) (map[string]string, []plugin.Diagnostic) {
	t.Helper()
	files := map[string]string{}
	var diags []plugin.Diagnostic
	for _, v := range views {
		if !listed(v) {
			continue
		}
		out, errs, err := Emit(request(v, TranspilerName), root)
		if err != nil {
			t.Fatalf("%s: %v", v.Viewer, err)
		}
		diags = append(diags, errs...)
		if len(errs) > 0 && len(out) > 0 {
			t.Errorf("%s: 診断があるのに、ファイルを返した", v.Viewer)
		}
		for _, f := range out {
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

// checkFormatted は、生成した Go が go/format を通した形で、一行目がヘッダであること（docs/plugin-go-valid.md「出力」）を確かめる。
func checkFormatted(t *testing.T, files map[string]string) {
	t.Helper()
	for _, p := range sortedPaths(files) {
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
		if !strings.HasPrefix(files[p], "// Code generated by hozoc go-valid "+plugin.Version+"; DO NOT EDIT.\n") {
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

// basePlugin は、基底 go のプラグインの実行ファイル（テストの間に一度だけビルドする）。
var basePlugin struct {
	once sync.Once
	dir  string
	path string
	err  error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if basePlugin.dir != "" {
		os.RemoveAll(basePlugin.dir)
	}
	os.Exit(code)
}

// baseFiles は、views のそれぞれについて、基底 go の実行ファイルを hozoc と同じに起動し（要求を標準入力に、応答を標準出力から。
// docs/plugin-protocol.md「起動」）、返ったファイルを集める。go-valid の出力と合わせて、コンパイルできることを確かめるためのもの。
func baseFiles(t *testing.T, root string, views []*ir.View) map[string]string {
	t.Helper()
	goBin := goTool(t)
	basePlugin.once.Do(func() {
		basePlugin.dir, basePlugin.err = os.MkdirTemp("", "hozo-base-plugin")
		if basePlugin.err != nil {
			return
		}
		basePlugin.path = filepath.Join(basePlugin.dir, "hozoc-plugin-go")
		cmd := exec.Command(goBin, "build", "-o", basePlugin.path, "github.com/mew-ton/hozo/plugins/go")
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			basePlugin.err = fmt.Errorf("基底のプラグインをビルドできない: %v\n%s", err, out)
		}
	})
	if basePlugin.err != nil {
		t.Fatal(basePlugin.err)
	}
	files := map[string]string{}
	for _, v := range views {
		req, err := json.Marshal(request(v, v.Config.Base))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(basePlugin.path)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOZO_PROJECT_ROOT="+root)
		cmd.Stdin = bytes.NewReader(req)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("基底 go（%s）: %v\n%s", v.Viewer, err, stderr.String())
		}
		var resp plugin.EmitResponse
		if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
			t.Fatalf("基底 go（%s）の応答を読めない: %v", v.Viewer, err)
		}
		for _, d := range resp.Errors {
			t.Errorf("基底 go（%s）の診断 %s: %s", v.Viewer, d.Code, d.Message)
		}
		for _, f := range resp.Files {
			files[f.Path] = f.Content
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	return files
}

// compose は、root に、基底 go の出力と、go-valid の出力 files を書く（hozoc emit が書き出すものと同じ木）。
func compose(t *testing.T, root string, views []*ir.View, files map[string]string) {
	t.Helper()
	base := baseFiles(t, root, views)
	for p := range files {
		if _, clash := base[p]; clash {
			t.Errorf("基底 go と同じパス %s を返した（CFG-5）", p)
		}
	}
	writeFiles(t, root, base)
	writeFiles(t, root, files)
}

// vet は、基底 go の出力と合わせて、生成したコードが `go vet` を通る（コンパイルできる）ことを確かめる。
func vet(t *testing.T, root string, views []*ir.View, files map[string]string) {
	t.Helper()
	compose(t, root, views, files)
	if out, err := runGo(t, root, "vet", "./..."); err != nil {
		t.Errorf("生成したコードが go vet を通らない: %v\n%s", err, out)
	}
}
