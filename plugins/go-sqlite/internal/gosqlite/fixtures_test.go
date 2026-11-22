package gosqlite

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

// このファイルは、単体テストの入力と、ゴールデンの比べ方（docs/testing.md「単体テスト」の `plugins/go-sqlite`）。
//
// 入力は testdata/<名前>/ の小さなプロジェクト（hozo.toml と .hozo）。このパッケージは internal/ を import できない
// （docs/packages.md「依存の向き」）ので、.hozo を自分では解析しない: hozoc がトランスパイラに渡すのと同じ View を、
// tests/pluginviews_test.go の TestPluginViewFixtures が testdata/<名前>/views.json に置き、ここではそれを読む。
// 期待する出力は testdata/<名前>/expected/（プロジェクトのルートからの相対パスの木と、warning の warnings.txt、
// 診断の errors.txt）。仕様を変えたら `go test ./tests -run TestPluginViewFixtures -update`、その後に
// `go test ./plugins/go-sqlite/... -update`。
//
// ゴールデンとの比較は、基底を起動せず、View だけを入力にする（基底の応答に依らないことの確かめ）。生成したコードを
// コンパイルして動かすテストだけは、基底のファイルも要る（db.go は、基底の型と状態の単位を使う）ので、基底のプラグインを
// ビルドして、同じ View から出させる（baseFiles）。

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

// addDriver は、root のモジュールに、SQLite のドライバを足す（生成したコードを本物の SQLite の上で動かすテストのため）。
// ドライバと版は、サンプル shop-server が使うもの（tests/samples/shop-server/go.mod。cgo の要らない modernc.org/sqlite）で、
// その go.mod と go.sum を、モジュールのパスと replace だけを書き換えて写す。モジュールキャッシュになければ、go が GOPROXY から取りに行く（[runGo]）。
func addDriver(t *testing.T, root string) {
	t.Helper()
	sample := filepath.Join(repoRoot(t), "tests", "samples", "shop-server")
	gomod, err := os.ReadFile(filepath.Join(sample, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(gomod), "\n")
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "module "):
			lines[i] = "module " + modulePath
		case strings.HasPrefix(l, "replace github.com/mew-ton/hozo => "):
			lines[i] = "replace github.com/mew-ton/hozo => " + filepath.ToSlash(repoRoot(t))
		}
	}
	writeFile(t, filepath.Join(root, "go.mod"), strings.Join(lines, "\n"))
	gosum, err := os.ReadFile(filepath.Join(sample, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "go.sum"), string(gosum))
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

// ours は、views のうち、単位に go-sqlite を書いた出力の View（hozoc が go-sqlite を起動するもの）。
func ours(t *testing.T, views []*ir.View) []*ir.View {
	t.Helper()
	var out []*ir.View
	for _, v := range views {
		for _, name := range v.Config.Transpilers {
			if name == TranspilerName {
				out = append(out, v)
				break
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("単位に go-sqlite を書いた View がない")
	}
	return out
}

// request は、hozoc が go-sqlite に送る emit の要求（docs/plugin-protocol.md「emit」）。
func request(view *ir.View) *plugin.EmitRequest {
	return &plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", Transpiler: TranspilerName, View: view}
}

// result は、いくつかの View の出力を集めたもの。
type result struct {
	files    map[string]string // パス → 内容
	warnings []plugin.Warning
	errors   []plugin.Diagnostic
}

// emitViews は、views のそれぞれを root のプロジェクトとして出力し、ファイルと warning と診断を集める。
func emitViews(t *testing.T, root string, views []*ir.View) result {
	t.Helper()
	res := result{files: map[string]string{}}
	for _, v := range views {
		out, err := Emit(request(v))
		if err != nil {
			t.Fatalf("%s: %v", v.Viewer, err)
		}
		if len(out.Errors) > 0 && len(out.Files) > 0 {
			t.Errorf("%s: 診断があるのに、ファイルを返した", v.Viewer)
		}
		res.warnings = append(res.warnings, out.Warnings...)
		res.errors = append(res.errors, out.Errors...)
		for _, f := range out.Files {
			if _, dup := res.files[f.Path]; dup {
				t.Errorf("二つの出力が同じパス %s を返した", f.Path)
			}
			if !strings.HasPrefix(f.Path, v.Config.Dir+"/") {
				t.Errorf("%s: 出力先 %s の外のパス %s を返した", v.Viewer, v.Config.Dir, f.Path)
			}
			res.files[f.Path] = f.Content
		}
	}
	return res
}

// emitFixture は、入力 name を新しいプロジェクトのルートで出力する。診断があれば失敗。
func emitFixture(t *testing.T, name string) (root string, res result) {
	t.Helper()
	root = newRoot(t)
	res = emitViews(t, root, ours(t, loadViews(t, name)))
	for _, d := range res.errors {
		t.Errorf("診断 %s（%s）: %s", d.Code, d.At, d.Message)
	}
	if t.Failed() {
		t.FailNow()
	}
	return root, res
}

// warningsText は、warning を一行ずつ（位置、メッセージをタブで区切って）書いたもの。
func warningsText(ws []plugin.Warning) string {
	var b strings.Builder
	for _, w := range ws {
		b.WriteString(w.At + "\t" + w.Message + "\n")
	}
	return b.String()
}

// errorsText は、診断を一行ずつ（コード、位置、メッセージをタブで区切って）書いたもの。
func errorsText(ds []plugin.Diagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.Code + "\t" + d.At + "\t" + d.Message + "\n")
	}
	return b.String()
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

// checkFormatted は、生成した Go が go/format を通した形で、一行目がヘッダであること（docs/plugin-go-sqlite.md「出力」）を確かめる。
func checkFormatted(t *testing.T, files map[string]string) {
	t.Helper()
	for _, p := range sortedPaths(files) {
		if filepath.Base(p) != FileName {
			t.Errorf("db.go でないファイル %s を出力した", p)
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
		if !strings.HasPrefix(files[p], "// Code generated by hozoc go-sqlite "+plugin.Version+"; DO NOT EDIT.\n") {
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

// runGo は、root のモジュールで go のコマンドを走らせる。
// SQLite のドライバ（[addDriver]）は、サンプルの go.sum の通りのものをモジュールキャッシュから使う。キャッシュにあれば
// ネットワークは要らない。なければ go が GOPROXY から取りに行く（サンプルの go のサブテストと同じ。tests/samples/README.md「テスト」）。
// ツールチェーンは取りに行かない（GOTOOLCHAIN=local）。
func runGo(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(goTool(t), args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off", "GOTOOLCHAIN=local")
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

// 基底のプラグインの実行ファイル（テストの間に一度だけビルドする。TestMain が、終わりに消す）。
var (
	baseOnce sync.Once
	baseBin  string
	baseErr  error
)

// TestMain は、テストを走らせた後に、ビルドした基底のプラグインを消す。
func TestMain(m *testing.M) {
	code := m.Run()
	if baseBin != "" {
		os.RemoveAll(filepath.Dir(baseBin))
	}
	os.Exit(code)
}

// basePlugin は、基底のプラグイン go（plugins/go）をビルドした実行ファイル。
func basePlugin(t *testing.T) string {
	t.Helper()
	goBin := goTool(t)
	baseOnce.Do(func() {
		dir, err := os.MkdirTemp("", "hozoc-plugin-go-*")
		if err != nil {
			baseErr = err
			return
		}
		baseBin = filepath.Join(dir, "hozoc-plugin-go")
		cmd := exec.Command(goBin, "build", "-o", baseBin, "github.com/mew-ton/hozo/plugins/go")
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			baseErr = fmt.Errorf("基底のプラグインをビルドできない: %v\n%s", err, out)
		}
	})
	if baseErr != nil {
		t.Fatal(baseErr)
	}
	return baseBin
}

// baseFiles は、views のすべての出力の、基底 go のファイル（パス → 内容）。基底のプラグインを、hozoc と同じ約束
// （標準入力に要求、標準出力に応答。docs/plugin-protocol.md「起動」）で、View ごとに起動する。
func baseFiles(t *testing.T, root string, views []*ir.View) map[string]string {
	t.Helper()
	bin := basePlugin(t)
	files := map[string]string{}
	for _, v := range views {
		req, err := json.Marshal(&plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", Transpiler: v.Config.Base, View: v})
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin)
		cmd.Dir = root
		cmd.Env = os.Environ()
		cmd.Stdin = bytes.NewReader(req)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("基底 %s（%s）: %v\n%s%s", v.Config.Base, v.Viewer, err, stdout.String(), stderr.String())
		}
		var resp plugin.EmitResponse
		if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
			t.Fatalf("基底 %s（%s）の応答を読めない: %v", v.Config.Base, v.Viewer, err)
		}
		for _, d := range resp.Errors {
			t.Errorf("基底 %s（%s）の診断 %s: %s", v.Config.Base, v.Viewer, d.Code, d.Message)
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

// writeProject は、入力 name のすべての出力（基底のファイルと、go-sqlite のファイル files）を root に書く。
func writeProject(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	base := baseFiles(t, root, loadViews(t, name))
	for p := range files {
		if _, dup := base[p]; dup {
			t.Errorf("go-sqlite のファイル %s が、基底のファイルと当たる", p)
		}
	}
	writeFiles(t, root, base)
	writeFiles(t, root, files)
}

// vet は、入力 name の基底のファイルと files を root に書いて、`go vet` を通る（コンパイルできる）ことを確かめる。
func vet(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	writeProject(t, root, name, files)
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
