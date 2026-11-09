package golang

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

var update = flag.Bool("update", false, "ゴールデン（testdata/<case>/expected）を更新する")

// goldenCase は golden テスト一つ: 一つの Program から出す View の列（別の名前空間の import を含めて
// コンパイルできるように、使う側と使われる側の両方を出す）。
type goldenCase struct {
	name      string
	views     []*ir.View
	manifests []ManifestInfo
}

// languageOf は各テストの名前空間の Conf。server / browser / inventory / catalog が Go、語彙は未設定。
func languageOf(ns string) (lang, out string, ok bool) {
	switch ns {
	case "server", "browser", "inventory", "catalog":
		return "go", "gen/" + ns, true
	}
	return "", "", false
}

var goldenCases = []goldenCase{
	{name: "server", views: []*ir.View{serverView()}},
	{name: "browser", views: []*ir.View{browserServerView(), browserView()}, manifests: browserManifests()},
	{name: "constraint", views: []*ir.View{constraintView()}},
	{name: "derived", views: []*ir.View{derivedView()}},
}

// repoRoot はこのリポジトリのルート（go.mod のあるディレクトリ）。
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root の go.mod がない: %v", err)
	}
	return root
}

// newProjectRoot は go.mod を持つ一時のプロジェクトのルートを作る。
// github.com/mew-ton/hozo はこのリポジトリに replace する。
func newProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gomod := fmt.Sprintf("module example.com/app\n\ngo 1.24\n\nrequire github.com/mew-ton/hozo v0.0.0\n\nreplace github.com/mew-ton/hozo => %s\n", filepath.ToSlash(repoRoot(t)))
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestGolden(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			root := newProjectRoot(t)
			var files []File
			for _, v := range tc.views {
				fs, diags := Emit(v, Options{Root: root, Manifests: tc.manifests, LanguageOf: languageOf})
				if len(diags) > 0 {
					t.Fatalf("%s: 診断が出た: %s", v.Viewer, formatDiags(diags))
				}
				files = append(files, fs...)
			}
			if len(files) == 0 {
				t.Fatal("ファイルが出なかった")
			}
			checkGolden(t, filepath.Join("testdata", tc.name, "expected"), files)
			compileFiles(t, root, files)
		})
	}
}

// checkGolden は files と expected の木を比べる。-update なら expected を書き直す。
func checkGolden(t *testing.T, dir string, files []File) {
	t.Helper()
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if *update {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			p := filepath.Join(dir, filepath.FromSlash(f.Path))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, f.Content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	want := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		want[filepath.ToSlash(rel)] = b
		return err
	})
	if err != nil {
		t.Fatalf("expected を読めない（-update で作る）: %v", err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[f.Path] = true
		w, ok := want[f.Path]
		if !ok {
			t.Errorf("expected にないファイル: %s\n%s", f.Path, f.Content)
			continue
		}
		if !bytes.Equal(w, f.Content) {
			t.Errorf("%s が expected と違う（-update で更新）:\n%s", f.Path, firstDiff(string(w), string(f.Content)))
		}
	}
	for p := range want {
		if !got[p] {
			t.Errorf("出なかったファイル: %s", p)
		}
	}
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

// compileFiles は生成したファイルを root に書き、go vet ./... が通ることを確かめる。
// go がなければ飛ばす。モジュールの解決はオフライン（replace だけ）。
func compileFiles(t *testing.T, root string, files []File) {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go が PATH にない")
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, f.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(goBin, "vet", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("生成したコードが go vet を通らない: %v\n%s", err, out)
	}
}

func formatDiags(diags []diag.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		fmt.Fprintf(&b, "\n  %s[%s]: %s", d.Severity, d.Code, d.Message)
	}
	return b.String()
}

// codesOf は診断のコードの列（出た順）。
func codesOf(diags []diag.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return out
}

// smallView は model / scalar だけの小さな View を作る（否定のテスト用）。
func smallView(decls ...ir.Decl) *ir.View {
	ns := ir.Namespace{Name: "app", Decls: decls}
	p := &ir.Program{Version: ir.Version, Namespaces: []ir.Namespace{{Name: ""}, validVocab(), ns}}
	return &ir.View{Viewer: "app", Config: ir.ViewConfig{Language: "go", Out: "gen/app"}, Program: p, Ops: map[string]*ir.ViewedSignature{}}
}

func TestNoGoMod(t *testing.T) {
	root := t.TempDir() // go.mod を置かない。/tmp の上にも go.mod はない前提
	files, diags := Emit(smallView(&ir.Scalar{DeclBase: base("app.Id", nil), Base: ir.String()}), Options{Root: root})
	if files != nil {
		t.Errorf("ファイルが出た: %v", files)
	}
	if got := codesOf(diags); len(got) != 1 || got[0] != CodeNoGoMod {
		t.Errorf("GOEMIT-1 を期待したが %s", formatDiags(diags))
	}
}

func TestNameCollision(t *testing.T) {
	root := newProjectRoot(t)
	t.Run("fields", func(t *testing.T) {
		v := smallView(&ir.Model{DeclBase: base("app.M", nil), Fields: []ir.Field{field("id", ir.String()), field("Id", ir.String())}})
		_, diags := Emit(v, Options{Root: root})
		if got := codesOf(diags); len(got) != 1 || got[0] != CodeNameCollision {
			t.Errorf("GOEMIT-2 を期待したが %s", formatDiags(diags))
		}
	})
	t.Run("ops", func(t *testing.T) {
		v := smallView(&ir.Op{DeclBase: base("app.run", nil)}, &ir.Op{DeclBase: base("app.Run", nil)})
		_, diags := Emit(v, Options{Root: root})
		if got := codesOf(diags); len(got) != 1 || got[0] != CodeNameCollision {
			t.Errorf("GOEMIT-2 を期待したが %s", formatDiags(diags))
		}
	})
	t.Run("types", func(t *testing.T) {
		v := smallView(&ir.Model{DeclBase: base("app.user", nil)}, &ir.Scalar{DeclBase: base("app.User", nil), Base: ir.String()})
		_, diags := Emit(v, Options{Root: root})
		if got := codesOf(diags); len(got) != 1 || got[0] != CodeNameCollision {
			t.Errorf("GOEMIT-2 を期待したが %s", formatDiags(diags))
		}
	})
}

func TestBadConstraint(t *testing.T) {
	root := newProjectRoot(t)
	cases := map[string]ir.Decl{
		"min on string":           &ir.Scalar{DeclBase: base("app.S", nil, app("valid.min", arg("value", lit(ir.IntLiteral{Value: 1})))), Base: ir.String()},
		"length on int":           &ir.Scalar{DeclBase: base("app.S", nil, app("valid.length", arg("max", lit(ir.IntLiteral{Value: 1})))), Base: ir.Int()},
		"bad regex":               &ir.Scalar{DeclBase: base("app.S", nil, app("valid.pattern", arg("regex", lit(ir.StringLiteral{Value: "("})))), Base: ir.String()},
		"unknown format":          &ir.Scalar{DeclBase: base("app.S", nil, app("valid.format", arg("name", lit(ir.StringLiteral{Value: "phone"})))), Base: ir.String()},
		"format on inherited int": &ir.Scalar{DeclBase: base("app.S", nil, app("valid.format", arg("name", lit(ir.StringLiteral{Value: "email"})))), Base: ir.Ref("app.N")},
		"field max on model": &ir.Model{DeclBase: base("app.S", nil), Fields: []ir.Field{{Name: "n", Type: ir.Ref("app.N2"), Span: sp("x", 1),
			Decorators: []ir.Application{app("valid.max", arg("value", lit(ir.IntLiteral{Value: 1})))}}}},
	}
	keys := make([]string, 0, len(cases))
	for k := range cases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Run(k, func(t *testing.T) {
			v := smallView(cases[k],
				&ir.Scalar{DeclBase: base("app.N", nil), Base: ir.Int()},
				&ir.Model{DeclBase: base("app.N2", nil)})
			files, diags := Emit(v, Options{Root: root})
			if files != nil {
				t.Error("ファイルが出た")
			}
			if got := codesOf(diags); len(got) != 1 || got[0] != CodeBadConstraint {
				t.Errorf("GOEMIT-3 を期待したが %s", formatDiags(diags))
			}
		})
	}
}

func TestBadDefault(t *testing.T) {
	root := newProjectRoot(t)
	v := smallView(
		&ir.Scalar{DeclBase: base("app.PosInt", nil, app("valid.min", arg("value", lit(ir.IntLiteral{Value: 1})))), Base: ir.Int()},
		&ir.Scalar{DeclBase: base("app.Code", nil, app("valid.pattern", arg("regex", lit(ir.StringLiteral{Value: "^[A-Z]+$"})))), Base: ir.String()},
		&ir.Model{DeclBase: base("app.Q", nil), Fields: []ir.Field{
			{Name: "page", Type: ir.Ref("app.PosInt"), Default: ir.IntLiteral{Value: 0}, Span: sp("x", 1)},
			{Name: "code", Type: ir.Ref("app.Code"), Default: ir.StringLiteral{Value: "abc"}, Span: sp("x", 2)},
			{Name: "ok", Type: ir.Ref("app.PosInt"), Default: ir.IntLiteral{Value: 1}, Span: sp("x", 3)},
			{Name: "name", Type: ir.String(), Default: ir.StringLiteral{Value: "toolong"}, Span: sp("x", 4),
				Decorators: []ir.Application{app("valid.length", arg("max", lit(ir.IntLiteral{Value: 3})))}},
		}},
	)
	files, diags := Emit(v, Options{Root: root})
	if files != nil {
		t.Error("ファイルが出た")
	}
	want := []string{CodeBadDefault, CodeBadDefault, CodeBadDefault}
	if got := codesOf(diags); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("GOEMIT-4 を三つ期待したが %s", formatDiags(diags))
	}
}

func TestFormatFailure(t *testing.T) {
	root := newProjectRoot(t)
	// 識別子にならないフィールド名（構文解析は通さないが、生成器の不具合の再現として）。
	v := smallView(&ir.Model{DeclBase: base("app.M", nil), Fields: []ir.Field{field("bad-name", ir.String())}})
	files, diags := Emit(v, Options{Root: root})
	if files != nil {
		t.Error("ファイルが出た")
	}
	if got := codesOf(diags); len(got) != 1 || got[0] != CodeInternal {
		t.Fatalf("GOEMIT-INTERNAL を期待したが %s", formatDiags(diags))
	}
	failed := filepath.Join(root, "gen", "app", FailedDir, "types.go")
	b, err := os.ReadFile(failed)
	if err != nil {
		t.Fatalf("整形前の内容が %s にない: %v", failed, err)
	}
	if !bytes.Contains(b, []byte("Bad-name")) {
		t.Errorf("整形前の内容が違う:\n%s", b)
	}
	if !strings.Contains(diags[0].Message, FailedDir) {
		t.Errorf("メッセージに書き出し先がない: %s", diags[0].Message)
	}
}

// TestDeterministic は同じ入力から同じバイト列が出ること。
func TestDeterministic(t *testing.T) {
	root := newProjectRoot(t)
	first, _ := Emit(browserView(), Options{Root: root, Manifests: browserManifests(), LanguageOf: languageOf})
	for i := 0; i < 3; i++ {
		again, _ := Emit(browserView(), Options{Root: root, Manifests: browserManifests(), LanguageOf: languageOf})
		if len(first) != len(again) {
			t.Fatal("ファイルの数が違う")
		}
		for j := range first {
			if first[j].Path != again[j].Path || !bytes.Equal(first[j].Content, again[j].Content) {
				t.Fatalf("%s が毎回違う", first[j].Path)
			}
		}
	}
}

// TestSubPath は「配置」の P: 親と同じ out を継いだ子はサブディレクトリに置く。
func TestSubPath(t *testing.T) {
	root := newProjectRoot(t)
	v := smallView(&ir.Scalar{DeclBase: base("app.Id", nil), Base: ir.String()})
	v.Viewer = "app.admin"
	v.Program.Namespaces[2].Name = "app.admin"
	v.Program.Namespaces[2].Decls[0].(*ir.Scalar).Name = "app.admin.Id"
	files, diags := Emit(v, Options{Root: root, LanguageOf: func(ns string) (string, string, bool) {
		if ns == "app" || ns == "app.admin" {
			return "go", "gen/app", true
		}
		return "", "", false
	}})
	if len(diags) > 0 {
		t.Fatal(formatDiags(diags))
	}
	if len(files) == 0 || files[0].Path != "gen/app/admin/types.go" {
		t.Errorf("gen/app/admin/types.go を期待したが %v", files)
	}
	if !bytes.Contains(files[0].Content, []byte("package admin\n")) {
		t.Errorf("package admin を期待した:\n%s", files[0].Content)
	}
}
