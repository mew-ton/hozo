package gocli

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

var update = flag.Bool("update", false, "golden ファイルを書き直す")

// repoRoot はこのリポジトリのルート（go.mod のある場所）。
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod not at %s: %v", root, err)
	}
	return root
}

// handlerFor は HOZO_PROJECT_ROOT を root にした Handler。
func handlerFor(root string) Handler {
	return Handler{Getenv: func(k string) string {
		if k == "HOZO_PROJECT_ROOT" {
			return root
		}
		return ""
	}}
}

func emit(t *testing.T, h Handler, v *ir.View) *plugin.EmitResponse {
	t.Helper()
	resp, err := h.Emit(&plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", View: v})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	return resp
}

func checkGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to create)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden (run with -update to accept):\n%s", name, diffHint(string(want), got))
	}
}

// diffHint は最初に違う行の前後を示す。
func diffHint(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			return "line " + strconv.Itoa(i+1) + ":\n  want: " + a + "\n  got:  " + b
		}
	}
	return "(no differing line)"
}

func TestEmitHozocGolden(t *testing.T) {
	root := repoRoot(t)
	resp := emit(t, handlerFor(root), hozocView())
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v", resp.Errors)
	}
	if len(resp.Files) != 1 || resp.Files[0].Path != "internal/gen/hozoc/cli/cli.go" {
		t.Fatalf("files = %+v", resp.Files)
	}
	src := resp.Files[0].Content
	if !strings.Contains(src, `"github.com/mew-ton/hozo/internal/gen/hozoc"`) {
		t.Errorf("import path of the namespace package not found in:\n%s", src)
	}
	if !strings.Contains(src, "func Main(ops hozoc.Ops, p Presenter, args []string, env func(string) string, stdout, stderr io.Writer) int") {
		t.Errorf("Main has the wrong signature")
	}
	// 見分けた例外の Failure も Err を運ぶ（値のない例外の表示に Presenter が元の error のメッセージを使えるように）。
	for _, want := range []string{
		`Failure{Name: "hozoc.CheckFailed", Value: e.Value, Err: err}, 1`,
		`Failure{Name: "hozoc.UnknownNamespace", Err: err}, 64`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated cli.go lacks %q", want)
		}
	}
	checkGolden(t, "hozoc_cli.go.golden", src)
}

func TestEmitDemoGolden(t *testing.T) {
	root := repoRoot(t)
	resp := emit(t, handlerFor(root), demoView())
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v", resp.Errors)
	}
	if len(resp.Files) != 1 {
		t.Fatalf("files = %+v", resp.Files)
	}
	checkGolden(t, "demo_cli.go.golden", resp.Files[0].Content)
}

func TestEmitIsDeterministic(t *testing.T) {
	root := repoRoot(t)
	a := emit(t, handlerFor(root), hozocView()).Files[0].Content
	b := emit(t, handlerFor(root), hozocView()).Files[0].Content
	if a != b {
		t.Error("two emits of the same view differ")
	}
}

// TestEmitUsesDir は出力先と import パスが View.config.dir（Dir(N)）から求まること（docs/plugin-go-cli.md「出力」）。
// 親の out を継いだ名前空間では dir が out のサブディレクトリになっている。
func TestEmitUsesDir(t *testing.T) {
	v := hozocView()
	v.Config.Out = "internal/gen"
	v.Config.Dir = "internal/gen/hozoc"
	v.Namespaces[v.Viewer] = v.Config
	resp := emit(t, handlerFor(repoRoot(t)), v)
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v", resp.Errors)
	}
	if len(resp.Files) != 1 || resp.Files[0].Path != "internal/gen/hozoc/cli/cli.go" {
		t.Fatalf("files = %+v", resp.Files)
	}
	if !strings.Contains(resp.Files[0].Content, `"github.com/mew-ton/hozo/internal/gen/hozoc"`) {
		t.Errorf("import path should come from dir:\n%s", resp.Files[0].Content[:600])
	}
}

func TestEmitNoGoMod(t *testing.T) {
	dir := t.TempDir()
	resp := emit(t, handlerFor(dir), hozocView())
	if len(resp.Files) != 0 || len(resp.Errors) != 1 || resp.Errors[0].Code != "GOEMIT-1" || resp.Errors[0].At != "hozoc" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestEmitNoCommands(t *testing.T) {
	ns := ir.Namespace{Name: "empty", Decls: []ir.Decl{
		opDecl("empty.run", "", nil, nil, nil),
	}}
	resp := emit(t, handlerFor(repoRoot(t)), view(ns, "gen/empty"))
	if len(resp.Files) != 0 || len(resp.Errors) != 0 || len(resp.Warnings) != 1 || resp.Warnings[0].At != "empty" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestEmitDefaultRootIsCwd(t *testing.T) {
	// HOZO_PROJECT_ROOT がなければ作業ディレクトリ。テストの作業ディレクトリはこのパッケージなので、
	// その上の go.mod（リポジトリのルート）が見つかり、import パスはそこからの相対になる。
	h := Handler{Getenv: func(string) string { return "" }}
	resp := emit(t, h, hozocView())
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v", resp.Errors)
	}
	if !strings.Contains(resp.Files[0].Content, `"github.com/mew-ton/hozo/plugins/go-cli/internal/gocli/internal/gen/hozoc"`) {
		t.Errorf("import path should be relative to cwd:\n%s", resp.Files[0].Content[:600])
	}
}

func TestCheckAndImportUnsupported(t *testing.T) {
	if _, err := (Handler{}).Check(&plugin.CheckRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Check err = %v", err)
	}
	if _, err := (Handler{}).Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import err = %v", err)
	}
}

// --- CLI-1〜CLI-5 ---

// withDecls は hozoc の名前空間に宣言を足した View。
func withDecls(decls ...ir.Decl) *ir.View {
	ns := hozocNamespace()
	ns.Decls = append(ns.Decls, decls...)
	return view(ns, "internal/gen/hozoc")
}

func expectErrors(t *testing.T, v *ir.View, want ...[2]string) {
	t.Helper()
	resp := emit(t, handlerFor(repoRoot(t)), v)
	if len(resp.Files) != 0 {
		t.Errorf("files must be empty when there are errors, got %d", len(resp.Files))
	}
	var got [][2]string
	for _, e := range resp.Errors {
		got = append(got, [2]string{e.Code, e.At})
		if e.Message == "" {
			t.Errorf("%s at %s has no message", e.Code, e.At)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("errors = %+v\nwant %v", resp.Errors, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("error %d = %v, want %v (all: %+v)", i, got[i], want[i], resp.Errors)
		}
	}
}

func TestCLI1(t *testing.T) {
	t.Run("two params", func(t *testing.T) {
		expectErrors(t, withDecls(
			opDecl("hozoc.two", "", []ir.OpParam{
				{Name: "a", Type: ir.Ref("hozoc.CheckInput"), Span: sp(0)},
				{Name: "b", Type: ir.Ref("hozoc.CheckInput"), Span: sp(0)},
			}, nil, nil, cliCommand("two")),
		), [2]string{"CLI-1", "hozoc.two"})
	})
	t.Run("scalar param", func(t *testing.T) {
		expectErrors(t, withDecls(
			opDecl("hozoc.one", "", []ir.OpParam{{Name: "a", Type: ir.String(), Span: sp(0)}}, nil, nil, cliCommand("one")),
		), [2]string{"CLI-1", "hozoc.one"})
	})
	t.Run("ref to a scalar", func(t *testing.T) {
		expectErrors(t, withDecls(
			opDecl("hozoc.one", "", param("a", "hozoc.Path"), nil, nil, cliCommand("one")),
		), [2]string{"CLI-1", "hozoc.one"})
	})
}

func TestCLI2(t *testing.T) {
	t.Run("no decorator", func(t *testing.T) {
		expectErrors(t, withDecls(
			mdl("hozoc.In", "", fld("x", "", ir.String())),
			opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
		), [2]string{"CLI-2", "hozoc.In.x"})
	})
	t.Run("arg and flag", func(t *testing.T) {
		expectErrors(t, withDecls(
			mdl("hozoc.In", "", fld("x", "", ir.String(), cliArg(), cliFlag("x"))),
			opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
		), [2]string{"CLI-2", "hozoc.In.x"})
	})
	t.Run("env only is fine", func(t *testing.T) {
		resp := emit(t, handlerFor(repoRoot(t)), withDecls(
			mdl("hozoc.In", "", fld("x", "", ir.String(), cliEnv("X"))),
			opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
		))
		if len(resp.Errors) != 0 {
			t.Errorf("errors = %+v", resp.Errors)
		}
	})
}

func TestCLI3(t *testing.T) {
	cases := map[string]ir.Field{
		"model":             fld("x", "", ir.Ref("hozoc.Position"), cliFlag("x")),
		"record":            fld("x", "", ir.Record(ir.String(), ir.String()), cliFlag("x")),
		"time":              fld("x", "", ir.Time(), cliFlag("x")),
		"duration":          fld("x", "", ir.Duration(), cliArg()),
		"optional array":    fld("x", "", ir.Optional(ir.Array(ir.String())), cliArg()),
		"array of int":      fld("x", "", ir.Array(ir.Int()), cliArg()),
		"array as flag":     fld("x", "", ir.Array(ir.String()), cliFlag("x")),
		"array as env":      fld("x", "", ir.Array(ir.String()), cliEnv("X")),
		"boolean arg":       fld("x", "", ir.Boolean(), cliArg()),
		"optional bool arg": fld("x", "", ir.Optional(ir.Boolean()), cliArg()),
		"array of model":    fld("x", "", ir.Array(ir.Ref("hozoc.Position")), cliArg()),
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			expectErrors(t, withDecls(
				mdl("hozoc.In", "", f),
				opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
			), [2]string{"CLI-3", "hozoc.In.x"})
		})
	}
	t.Run("accepted types", func(t *testing.T) {
		resp := emit(t, handlerFor(repoRoot(t)), withDecls(
			scl("hozoc.Count", ir.Int()),
			scl("hozoc.Count2", ir.Ref("hozoc.Count")),
			mdl("hozoc.In", "",
				fld("s", "", ir.String(), cliFlag("s")),
				fld("b", "", ir.Boolean(), cliFlag("b")),
				fld("i", "", ir.Int(), cliFlag("i")),
				fld("f", "", ir.Float(), cliFlag("f")),
				fld("os", "", ir.Optional(ir.String()), cliFlag("os")),
				fld("ob", "", ir.Optional(ir.Boolean()), cliFlag("ob")),
				fld("oi", "", ir.Optional(ir.Int()), cliFlag("oi")),
				fld("of", "", ir.Optional(ir.Float()), cliFlag("of")),
				fld("sc", "", ir.Ref("hozoc.Count2"), cliFlag("sc")),
				fld("osc", "", ir.Optional(ir.Ref("hozoc.Path")), cliFlag("osc")),
				fld("a", "", ir.Int(), cliArg()),
				fld("rest", "", ir.Array(ir.Ref("hozoc.Path")), cliArg()),
			),
			opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
		))
		if len(resp.Errors) != 0 {
			t.Errorf("errors = %+v", resp.Errors)
		}
	})
}

func TestCLI4(t *testing.T) {
	t.Run("array not last", func(t *testing.T) {
		expectErrors(t, withDecls(
			mdl("hozoc.In", "",
				fld("rest", "", ir.Array(ir.String()), cliArg()),
				fld("x", "", ir.String(), cliArg()),
			),
			opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
		), [2]string{"CLI-4", "hozoc.In.rest"})
	})
	t.Run("two arrays", func(t *testing.T) {
		expectErrors(t, withDecls(
			mdl("hozoc.In", "",
				fld("a", "", ir.Array(ir.String()), cliArg()),
				fld("b", "", ir.Array(ir.String()), cliArg()),
			),
			opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
		), [2]string{"CLI-4", "hozoc.In.a"})
	})
	t.Run("array last with a flag after it is fine", func(t *testing.T) {
		resp := emit(t, handlerFor(repoRoot(t)), withDecls(
			mdl("hozoc.In", "",
				fld("a", "", ir.Array(ir.String()), cliArg()),
				fld("b", "", ir.String(), cliFlag("b")),
			),
			opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
		))
		if len(resp.Errors) != 0 {
			t.Errorf("errors = %+v", resp.Errors)
		}
	})
}

func TestCLI5(t *testing.T) {
	t.Run("duplicate flag", func(t *testing.T) {
		expectErrors(t, withDecls(
			mdl("hozoc.In", "",
				fld("a", "", ir.String(), cliFlag("x")),
				fld("b", "", ir.String(), cliFlag("x")),
			),
			opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
		), [2]string{"CLI-5", "hozoc.In.b"})
	})
	t.Run("duplicate command", func(t *testing.T) {
		expectErrors(t, withDecls(
			opDecl("hozoc.check2", "", param("in", "hozoc.CheckInput"), nil, nil, cliCommand("check")),
		), [2]string{"CLI-5", "hozoc.check2"})
	})
}

func TestAllErrorsAreReported(t *testing.T) {
	// 一つの入力に複数の誤りがあれば、止まらずにすべて報告する。
	expectErrors(t, withDecls(
		mdl("hozoc.In", "",
			fld("a", "", ir.String()),
			fld("b", "", ir.Time(), cliFlag("b")),
		),
		opDecl("hozoc.run", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
		opDecl("hozoc.run2", "", param("in", "hozoc.In"), nil, nil, cliCommand("run")),
	), [2]string{"CLI-2", "hozoc.In.a"}, [2]string{"CLI-3", "hozoc.In.b"}, [2]string{"CLI-5", "hozoc.run2"})
}
