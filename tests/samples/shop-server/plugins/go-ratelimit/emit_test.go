package main

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

// このファイルは go-ratelimit の単体テスト（../../README.md「サンプルの中のプラグイン」の「出力の規則」の各行）。期待する出力との
// バイト単位の比較（ブラックボックスのテスト）は、リポジトリの TestSamples の transpilers/go-ratelimit が、サンプルの
// gen/server/ratelimit.go で行う。ここは、サンプルが使わない場合を、小さな入力とゴールデンで確かめる。
//
// 入力は testdata/<名前>/ の小さなプロジェクト（hozo.toml と .hozo）。プラグインは .hozo を自分では解析しない: hozoc が
// トランスパイラに渡すのと同じ View を、リポジトリの tests/pluginviews_test.go の TestPluginViewFixtures が
// testdata/<名前>/views.json に置き、ここではそれを読む（docs/testing.md「単体テスト」）。期待する出力は
// testdata/<名前>/expected/。仕様を変えたら、リポジトリのルートで `go test ./tests -run TestPluginViewFixtures -update`、
// その後に、このディレクトリで `GOTOOLCHAIN=local go test . -update`。
//
// ゴールデンとの比較は、基底を起動せず、View だけを入力にする。生成したコードをコンパイルして動かすテストだけは、基底の
// ファイルも要る（ratelimit.go は、基底の型と Seams を使う）ので、基底のプラグインをビルドして、同じ View から出させる。

var update = flag.Bool("update", false, "ゴールデン（testdata/<名前>/expected/）を書き直す")

// modulePath は、テストが作るプロジェクトのモジュールのパス（import パスに現れる）。
const modulePath = "example.com/app"

// goTool は go のコマンド。なければテストを飛ばす。
func goTool(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go が PATH にない")
	}
	return goBin
}

var (
	repoOnce sync.Once
	repoDir  string
	repoErr  error
)

// repoRoot は、リポジトリのモジュール github.com/mew-ton/hozo のディレクトリ（生成したコードが import する runtime と、基底の
// プラグインの置き場）。サンプルの go.mod の replace の先を、go に聞く。
func repoRoot(t *testing.T) string {
	t.Helper()
	goBin := goTool(t)
	repoOnce.Do(func() {
		out, err := exec.Command(goBin, "list", "-m", "-f", "{{.Dir}}", "github.com/mew-ton/hozo").Output()
		if err != nil {
			repoErr = fmt.Errorf("リポジトリのモジュールの場所が分からない: %v", err)
			return
		}
		repoDir = strings.TrimSpace(string(out))
	})
	if repoErr != nil {
		t.Fatal(repoErr)
	}
	return repoDir
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
		t.Fatalf("%v（リポジトリのルートで `go test ./tests -run TestPluginViewFixtures -update` で作る）", err)
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

// ours は、views のうち、単位に go-ratelimit を書いた出力の View（hozoc が go-ratelimit を起動するもの）。
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
		t.Fatal("単位に go-ratelimit を書いた View がない")
	}
	return out
}

// request は、hozoc が go-ratelimit に送る emit の要求（docs/plugin-protocol.md「emit」）。
func request(view *ir.View) *plugin.EmitRequest {
	return &plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", Transpiler: TranspilerName, View: view}
}

// emitViews は、views のそれぞれを root のプロジェクトとして出力し、ファイル（パス → 内容）と診断を集める。warning は返さないはず。
func emitViews(t *testing.T, root string, views []*ir.View) (map[string]string, []plugin.Diagnostic) {
	t.Helper()
	files := map[string]string{}
	var diags []plugin.Diagnostic
	for _, v := range views {
		out, err := Emit(request(v), root)
		if err != nil {
			t.Fatalf("%s: %v", v.Viewer, err)
		}
		if len(out.Errors) > 0 && len(out.Files) > 0 {
			t.Errorf("%s: 診断があるのに、ファイルを返した", v.Viewer)
		}
		if len(out.Warnings) > 0 {
			t.Errorf("%s: warning を返した: %v", v.Viewer, out.Warnings)
		}
		diags = append(diags, out.Errors...)
		for _, f := range out.Files {
			if want := v.Config.Dir + "/" + FileName; f.Path != want {
				t.Errorf("%s: パス %s を返した（期待は %s）", v.Viewer, f.Path, want)
			}
			files[f.Path] = f.Content
		}
	}
	return files, diags
}

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

// runGo は、root のモジュールで go のコマンドを走らせる。モジュールの解決はオフライン。
func runGo(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(goTool(t), args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off", "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	return string(out), err
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

// basePlugin は、基底のプラグイン go（リポジトリの plugins/go）をビルドした実行ファイル。
func basePlugin(t *testing.T) string {
	t.Helper()
	goBin, repo := goTool(t), repoRoot(t)
	baseOnce.Do(func() {
		dir, err := os.MkdirTemp("", "hozoc-plugin-go-*")
		if err != nil {
			baseErr = err
			return
		}
		baseBin = filepath.Join(dir, "hozoc-plugin-go")
		cmd := exec.Command(goBin, "build", "-o", baseBin, "github.com/mew-ton/hozo/plugins/go")
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			baseErr = fmt.Errorf("基底のプラグインをビルドできない: %v\n%s", err, out)
		}
	})
	if baseErr != nil {
		t.Fatal(baseErr)
	}
	return baseBin
}

// writeProject は、入力 name のすべての出力（基底のファイルと、go-ratelimit のファイル files）を root に書く。基底のプラグインは、
// hozoc と同じ約束（標準入力に要求、標準出力に応答、HOZO_PROJECT_ROOT。docs/plugin-protocol.md「起動」）で、View ごとに起動する。
func writeProject(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	bin := basePlugin(t)
	for _, v := range loadViews(t, name) {
		req, err := json.Marshal(&plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", Transpiler: v.Config.Base, View: v})
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOZO_PROJECT_ROOT="+root)
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
		if len(resp.Errors) > 0 {
			t.Fatalf("基底 %s（%s）の診断: %+v", v.Config.Base, v.Viewer, resp.Errors)
		}
		for _, f := range resp.Files {
			if _, dup := files[f.Path]; dup {
				t.Errorf("go-ratelimit のファイル %s が、基底のファイルと当たる", f.Path)
			}
			writeFile(t, filepath.Join(root, filepath.FromSlash(f.Path)), f.Content)
		}
	}
	for p, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(p)), content)
	}
}

// TestGolden は、入力 seams の出力がゴールデンとバイト単位で同じこと、go/format を通した形で、一行目がヘッダであること、
// 同じ View から同じバイト列を返すこと、対象のない名前空間（plain）にはファイルを出さないこと、生成したコードが、基底の出力と
// 合わせてコンパイルできる（一時のモジュールで go vet を通る）ことを確かめる。
func TestGolden(t *testing.T) {
	root := newRoot(t)
	views := ours(t, loadViews(t, "seams"))
	files, diags := emitViews(t, root, views)
	for _, d := range diags {
		t.Errorf("診断 %s（%s）: %s", d.Code, d.At, d.Message)
	}
	if got, want := sortedPaths(files), []string{"gen/mixed/ratelimit.go", "gen/shop/ratelimit.go"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("出力したファイルが %v（期待は %v。対象のない plain には出さない）", got, want)
	}
	for p, content := range files {
		formatted, err := format.Source([]byte(content))
		if err != nil || string(formatted) != content {
			t.Errorf("%s は go/format を通した形でない（%v）", p, err)
		}
		if !strings.HasPrefix(content, "// Code generated by hozoc go-ratelimit "+plugin.Version+"; DO NOT EDIT.\n") {
			t.Errorf("%s の一行目がヘッダでない", p)
		}
	}
	compareTree(t, "seams", files)
	again, _ := emitViews(t, newRoot(t), views)
	for p, content := range files {
		if again[p] != content {
			t.Errorf("%s が、二度目の出力で変わった", p)
		}
	}
	// var _ Seams = RateSeams{} は、基底の Seams のメソッドが、すべて自分の対象のときだけ
	if !strings.Contains(files["gen/shop/ratelimit.go"], "\nvar _ Seams = RateSeams{}\n") {
		t.Error("shop の ratelimit.go に、var _ Seams = RateSeams{} がない")
	}
	if strings.Contains(files["gen/mixed/ratelimit.go"], "var _ Seams") {
		t.Error("mixed（継ぎ目を宣言したトランスパイラが二つ）の ratelimit.go に、var _ Seams がある")
	}
	writeProject(t, root, "seams", files)
	if out, err := runGo(t, root, "vet", "./..."); err != nil {
		t.Errorf("生成したコードが go vet を通らない: %v\n%s", err, out)
	}
}

// TestBehavior は、生成したコードを一時のモジュールで実際に動かす: 入力の behavior_test.go を、基底の出力と go-ratelimit の
// 出力を置いたモジュールのルートに置いて、go test を走らせる。確かめることは、そのファイルの頭。
func TestBehavior(t *testing.T) {
	root := newRoot(t)
	files, diags := emitViews(t, root, ours(t, loadViews(t, "seams")))
	if len(diags) > 0 {
		t.Fatalf("診断 %+v", diags)
	}
	writeProject(t, root, "seams", files)
	test, err := os.ReadFile(filepath.Join("testdata", "seams", "behavior_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "behavior_test.go"), string(test))
	out, err := runGo(t, root, "test", "-count=1", "-v", ".")
	if err != nil {
		t.Fatalf("生成したコードのテストが落ちた: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--- PASS: Test") {
		t.Fatalf("生成したコードのテストが一つも走らなかった:\n%s", out)
	}
}

// TestDiagnostics は、診断 GORATE-1（知らないオプション）と GORATE-2（公開の名前が当たる）: コード・位置・メッセージが
// ゴールデン testdata/<名前>/expected/errors.txt と同じで、診断を返した出力がファイルを返さないこと。
func TestDiagnostics(t *testing.T) {
	for _, fx := range []struct{ name, code string }{
		{"gorate1", codeRateOption},
		{"gorate2", codeRateCollision},
	} {
		t.Run(fx.name, func(t *testing.T) {
			_, diags := emitViews(t, newRoot(t), ours(t, loadViews(t, fx.name)))
			if len(diags) == 0 {
				t.Fatal("診断がない")
			}
			var b strings.Builder
			for _, d := range diags {
				if d.Code != fx.code {
					t.Errorf("コード %s の診断がある（%s だけのはず）: %s", d.Code, fx.code, d.Message)
				}
				b.WriteString(d.Code + "\t" + d.At + "\t" + d.Message + "\n")
			}
			compareTree(t, fx.name, map[string]string{"errors.txt": b.String()})
		})
	}
	// GORATE-2 は、当たった出力にだけ出る: quiet（対象がなく、ファイルを出さない）と clean には出ない
	for _, v := range ours(t, loadViews(t, "gorate2")) {
		files, diags := emitViews(t, newRoot(t), []*ir.View{v})
		wantErr := v.Viewer == "types" || v.Viewer == "funcs"
		wantFile := v.Viewer == "clean"
		if (len(diags) > 0) != wantErr || (len(files) == 1) != wantFile {
			t.Errorf("%s: files=%d 診断=%d", v.Viewer, len(files), len(diags))
		}
	}
}

// TestNoGoMod は、Dir(U, N) の上に go.mod がないとき: 報告するのは基底（GO-1）で、go-ratelimit は、ファイルも診断も返さない。
func TestNoGoMod(t *testing.T) {
	files, diags := emitViews(t, t.TempDir(), ours(t, loadViews(t, "seams")))
	if len(files) != 0 || len(diags) != 0 {
		t.Fatalf("files=%d 診断=%v, want どちらも空", len(files), diags)
	}
}

// TestHandler は、プラグインの入口（Handler）: HOZO_PROJECT_ROOT をルートにして出力する。ほかのトランスパイラの名前と、
// check、import は unsupported。View の欠けた要求は、診断ではなく誤り。
func TestHandler(t *testing.T) {
	root := newRoot(t)
	h := Handler{Getenv: func(key string) string {
		if key == "HOZO_PROJECT_ROOT" {
			return root
		}
		return ""
	}}
	view := ours(t, loadViews(t, "seams"))[0]
	resp, err := h.Emit(request(view))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := emitViews(t, root, []*ir.View{view})
	if len(resp.Files) != 1 || len(resp.Errors) != 0 || want[resp.Files[0].Path] != resp.Files[0].Content {
		t.Fatalf("files=%d errors=%v", len(resp.Files), resp.Errors)
	}
	other := request(view)
	other.Transpiler = "go"
	if _, err := h.Emit(other); err == nil {
		t.Error("知らない transpiler で、エラーにならない")
	}
	if _, err := h.Check(&plugin.CheckRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Check の err = %v, want ErrUnsupported", err)
	}
	if _, err := h.Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import の err = %v, want ErrUnsupported", err)
	}
	if _, err := Emit(request(&ir.View{Version: ir.Version, Viewer: "shop", Config: view.Config}), root); err == nil {
		t.Error("Program のない View で、誤りを返さない")
	}
}
