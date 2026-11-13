package golang

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

var update = flag.Bool("update", false, "ゴールデン（testdata/<case>/expected）を更新する")

// goldenCase は golden テスト一つ: 一つの Program から出す View の列（別の名前空間の import を含めて
// コンパイルできるように、使う側と使われる側の両方を出す）と、それぞれの選択。
type goldenCase struct {
	name  string
	views []goldenView
}

// goldenView は View 一つと、その名前空間に設定されたプラグインが返した選択（を合わせたもの）。
type goldenView struct {
	view *ir.View
	sel  plugin.Select
}

// namespaces は各テストの View.Namespaces。server / browser / inventory / catalog / app が Go で
// out = Dir = gen/<ns>（out を自分で書いたので P は空）、語彙は未設定。
func namespaces() map[string]ir.NamespaceConfig {
	m := map[string]ir.NamespaceConfig{}
	for _, ns := range []string{"server", "browser", "inventory", "catalog", "app"} {
		m[ns] = nsConfig(ns)
	}
	return m
}

// nsConfig は名前空間 ns の設定（out を自分で書いた名前空間）。
func nsConfig(ns string, plugins ...string) ir.NamespaceConfig {
	return ir.NamespaceConfig{Language: "go", Out: "gen/" + ns, Dir: "gen/" + ns, Plugins: plugins}
}

var goldenCases = []goldenCase{
	{name: "server", views: []goldenView{{serverView(), serverSelect()}}},
	// 配線、Deps、bind、値の分類の match、包む関数（wraps）、別の名前空間の型の import と複製
	{name: "browser", views: []goldenView{{browserServerView(), serverSelect()}, {browserView(), browserSelect()}}},
	{name: "constraint", views: []goldenView{{constraintView(), constraintSelect()}}},
	{name: "derived", views: []goldenView{{derivedView(), derivedSelect()}}},
	// 状態の単位、状態の分類、遷移の効果、状態での match（docs/emit-go.md「state」「classify」「match」）
	{name: "state", views: []goldenView{{stateView("shop"), stateSelect("shop")}, {stateView("depot"), stateSelect("depot")}}},
}

// emitAll は views をそれぞれの選択で出す。別の Go の名前空間の出力が持つ型（Exports）は import させる
// （internal/app が Options.Exported に入れるのと同じ）。診断が出れば失敗。
func emitAll(t *testing.T, root string, views []goldenView) []File {
	t.Helper()
	exported := map[string]bool{}
	for _, v := range views {
		for _, fqn := range Exports(v.view, v.sel) {
			exported[fqn] = true
		}
	}
	var files []File
	for _, v := range views {
		fs, diags := Emit(v.view, Options{Root: root, Select: v.sel, Exported: exported})
		if len(diags) > 0 {
			t.Fatalf("%s: 診断が出た: %s", v.view.Viewer, formatDiags(diags))
		}
		files = append(files, fs...)
	}
	return files
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
			files := emitAll(t, root, tc.views)
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

// smallView は名前空間 app だけの小さな View を作る。
func smallView(decls ...ir.Decl) *ir.View {
	ns := ir.Namespace{Name: "app", Decls: decls}
	p := &ir.Program{Version: ir.Version, Namespaces: []ir.Namespace{{Name: ""}, validVocab(), ns}}
	v := &ir.View{Viewer: "app", Config: nsConfig("app"), Namespaces: namespaces(), Program: p, Ops: map[string]*ir.ViewedSignature{}}
	for _, d := range decls {
		if op, ok := d.(*ir.Op); ok {
			v.Ops[op.Name] = &ir.ViewedSignature{Signature: op.Signature, Params: op.Params}
		}
	}
	return v
}

// use は、型 types（FQN）を引数に取る葉の op app.use。型は op から辿って選ばれる（docs/emit-go.md「出力の対象」の 6）ので、
// 型だけを確かめるテストはこの op を起点にする。
func use(types ...string) *ir.Op {
	op := &ir.Op{DeclBase: base("app.use", nil)}
	for i, fqn := range types {
		op.Params = append(op.Params, param(fmt.Sprintf("p%d", i), ir.Ref(fqn)))
	}
	return op
}

// selectAll は、viewer のすべての op を起点にし、すべての状態（基底の状態と状態の分類）を選ぶ選択。
func selectAll(v *ir.View) plugin.Select {
	var sel plugin.Select
	for _, d := range v.Program.Namespace(v.Viewer).Decls {
		switch x := d.(type) {
		case *ir.Op:
			sel.Roots = append(sel.Roots, x.Name)
		case *ir.State:
			sel.States = append(sel.States, x.Name)
		case *ir.Classify:
			if _, ok := x.SubjectStates(); ok {
				sel.States = append(sel.States, x.Name)
			}
		}
	}
	return sel
}

// emitSmall は smallView(decls...) を selectAll で出す。
func emitSmall(root string, decls ...ir.Decl) ([]File, []diag.Diagnostic) {
	v := smallView(decls...)
	return Emit(v, Options{Root: root, Select: selectAll(v)})
}

// contentOf は files のうち Path が suffix で終わるものの中身。なければ ""。
func contentOf(files []File, suffix string) string {
	for _, f := range files {
		if strings.HasSuffix(f.Path, suffix) {
			return string(f.Content)
		}
	}
	return ""
}

// paths は files の Path の列（出た順）。
func paths(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

// wantContains は content に wants のすべてがあり、nots のどれもないことを確かめる。
// `re:` で始まる項目は正規表現（gofmt が桁をそろえる構造体のフィールドは [fieldRE] で書く）。
func wantContains(t *testing.T, file, content string, wants, nots []string) {
	t.Helper()
	has := func(s string) bool {
		if re, ok := strings.CutPrefix(s, "re:"); ok {
			return regexp.MustCompile(re).MatchString(content)
		}
		return strings.Contains(content, s)
	}
	for _, want := range wants {
		if !has(want) {
			t.Errorf("%s に %q がない:\n%s", file, want, content)
		}
	}
	for _, not := range nots {
		if has(not) {
			t.Errorf("%s に %q がある:\n%s", file, not, content)
		}
	}
}

// fieldRE は構造体のフィールド `<name> <typ>` の行に当たる wantContains の項目（間の空白の数は問わない）。
func fieldRE(name, typ string) string {
	return `re:(?m)^\t` + regexp.QuoteMeta(name) + ` +` + regexp.QuoteMeta(typ) + `$`
}

// TestNothingSelected は docs/emit-go.md「出力の対象」: 何も選ばれていなければ（プラグインがない、select を返さない）、
// 宣言があっても何も出力しない。エラーでも warning でもない。go.mod がなくても GOEMIT-1 にならない。
func TestNothingSelected(t *testing.T) {
	carts, token, title, checkout := stateDecls()
	v := smallView(
		&ir.Scalar{DeclBase: base("app.Id", nil), Base: ir.String()},
		&ir.Model{DeclBase: base("app.M", nil), Fields: []ir.Field{field("id", ir.Ref("app.Id"))}},
		carts, token, title, checkout,
		&ir.Op{DeclBase: base("app.run", nil), Params: []ir.OpParam{param("m", ir.Ref("app.M"))}},
	)
	v.Program.Namespaces[2].Description = doc("説明だけでは doc.go も出ない")
	for name, root := range map[string]string{"with go.mod": newProjectRoot(t), "without go.mod": t.TempDir()} {
		files, diags := Emit(v, Options{Root: root})
		if len(files) != 0 || len(diags) != 0 {
			t.Errorf("%s: 何も選んでいないのに files = %v, diags = %s", name, paths(files), formatDiags(diags))
		}
	}
	if got := Exports(v, plugin.Select{}); len(got) != 0 {
		t.Errorf("Exports = %v, want none", got)
	}
}

func TestNoGoMod(t *testing.T) {
	root := t.TempDir() // go.mod を置かない。/tmp の上にも go.mod はない前提
	files, diags := emitSmall(root, &ir.Scalar{DeclBase: base("app.Id", nil), Base: ir.String()}, use("app.Id"))
	if files != nil {
		t.Errorf("ファイルが出た: %v", files)
	}
	if got := codesOf(diags); len(got) != 1 || got[0] != CodeNoGoMod {
		t.Errorf("GOEMIT-1 を期待したが %s", formatDiags(diags))
	}
}

func TestNameCollision(t *testing.T) {
	root := newProjectRoot(t)
	one := func(t *testing.T, decls ...ir.Decl) {
		t.Helper()
		files, diags := emitSmall(root, decls...)
		if got := codesOf(diags); files != nil || len(got) != 1 || got[0] != CodeNameCollision {
			t.Errorf("GOEMIT-2 を一つ期待したが %s", formatDiags(diags))
		}
	}
	t.Run("fields", func(t *testing.T) {
		one(t, &ir.Model{DeclBase: base("app.M", nil), Fields: []ir.Field{field("id", ir.String()), field("Id", ir.String())}}, use("app.M"))
	})
	t.Run("ops", func(t *testing.T) {
		// メソッド名と、関数の型の名前（RunFunc）の両方が衝突するが、どちらも同じ原因
		_, diags := emitSmall(root, &ir.Op{DeclBase: base("app.run", nil)}, &ir.Op{DeclBase: base("app.Run", nil)})
		if got := codesOf(diags); len(got) != 2 || got[0] != CodeNameCollision || got[1] != CodeNameCollision {
			t.Errorf("GOEMIT-2 を期待したが %s", formatDiags(diags))
		}
	})
	t.Run("types", func(t *testing.T) {
		one(t, &ir.Model{DeclBase: base("app.user", nil)}, &ir.Scalar{DeclBase: base("app.User", nil), Base: ir.String()}, use("app.user", "app.User"))
	})
	t.Run("type and op func type", func(t *testing.T) {
		// model RunFunc と、op run の関数の型 RunFunc
		one(t, &ir.Model{DeclBase: base("app.RunFunc", nil)}, &ir.Op{DeclBase: base("app.run", nil), Params: []ir.OpParam{param("m", ir.Ref("app.RunFunc"))}})
	})
	t.Run("type and generated name", func(t *testing.T) {
		// model Leaves と、生成したコードの Leaves
		one(t, &ir.Model{DeclBase: base("app.Leaves", nil)}, use("app.Leaves"))
	})
	carts, _, _, _ := stateDecls()
	t.Run("states in StateSet", func(t *testing.T) {
		// StateSet のフィールド名 Carts と、単位の型 CartsState の両方が衝突するが、原因は同じ
		upper := &ir.State{DeclBase: base("app.Carts", nil), Shape: ir.StateNullable, Element: ir.String()}
		_, diags := emitSmall(root, carts, upper)
		if got := codesOf(diags); len(got) != 2 || got[0] != CodeNameCollision || got[1] != CodeNameCollision {
			t.Errorf("GOEMIT-2 を期待したが %s", formatDiags(diags))
		}
	})
	t.Run("query and command in a unit", func(t *testing.T) {
		// docs/emit-go.md「state」: 単位の中で IsEmpty / Get / ケース名と、op 名を先頭大文字にしたものが衝突
		add := &ir.Effect{DeclBase: base("app.add", nil), Target: ir.BuiltinTarget{State: "app.carts", Operation: ir.OpPost}, Writes: []string{"app.carts"}}
		isEmpty := &ir.Op{DeclBase: base("app.isEmpty", nil), Effects: []string{"app.add"}, Signature: effectsSig("app.add")}
		one(t, carts, add, isEmpty)
	})
	t.Run("case and command in a unit", func(t *testing.T) {
		_, _, _, checkout := stateDecls()
		move := &ir.Effect{DeclBase: base("app.move", nil), Target: ir.TransitionTarget{Of: "app.Checkout", From: ir.AnyCase(), To: ir.Cases("Ordered")}, Writes: []string{"app.carts"}}
		ordered := &ir.Op{DeclBase: base("app.ordered", nil), Effects: []string{"app.move"}, Signature: effectsSig("app.move")}
		one(t, carts, checkout, move, ordered)
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
	others := []ir.Decl{
		&ir.Scalar{DeclBase: base("app.N", nil), Base: ir.Int()},
		&ir.Model{DeclBase: base("app.N2", nil)},
	}
	for _, k := range keys {
		t.Run(k, func(t *testing.T) {
			files, diags := emitSmall(root, append([]ir.Decl{cases[k], use("app.S")}, others...)...)
			if files != nil {
				t.Error("ファイルが出た")
			}
			if got := codesOf(diags); len(got) != 1 || got[0] != CodeBadConstraint {
				t.Errorf("GOEMIT-3 を期待したが %s", formatDiags(diags))
			}
		})
		t.Run(k+"/not selected", func(t *testing.T) {
			// 出力の対象でない宣言は確かめない（エラーでも warning でもない）
			files, diags := emitSmall(root, append([]ir.Decl{cases[k], use("app.N")}, others...)...)
			if len(files) == 0 || len(diags) != 0 {
				t.Errorf("選んでいない宣言で診断が出た: %s", formatDiags(diags))
			}
		})
	}
}

func TestBadDefault(t *testing.T) {
	root := newProjectRoot(t)
	files, diags := emitSmall(root,
		&ir.Scalar{DeclBase: base("app.PosInt", nil, app("valid.min", arg("value", lit(ir.IntLiteral{Value: 1})))), Base: ir.Int()},
		&ir.Scalar{DeclBase: base("app.Code", nil, app("valid.pattern", arg("regex", lit(ir.StringLiteral{Value: "^[A-Z]+$"})))), Base: ir.String()},
		&ir.Model{DeclBase: base("app.Q", nil), Fields: []ir.Field{
			{Name: "page", Type: ir.Ref("app.PosInt"), Default: ir.IntLiteral{Value: 0}, Span: sp("x", 1)},
			{Name: "code", Type: ir.Ref("app.Code"), Default: ir.StringLiteral{Value: "abc"}, Span: sp("x", 2)},
			{Name: "ok", Type: ir.Ref("app.PosInt"), Default: ir.IntLiteral{Value: 1}, Span: sp("x", 3)},
			{Name: "name", Type: ir.String(), Default: ir.StringLiteral{Value: "toolong"}, Span: sp("x", 4),
				Decorators: []ir.Application{app("valid.length", arg("max", lit(ir.IntLiteral{Value: 3})))}},
		}},
		use("app.Q"),
	)
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
	files, diags := emitSmall(root, &ir.Model{DeclBase: base("app.M", nil), Fields: []ir.Field{field("bad-name", ir.String())}}, use("app.M"))
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
	views := []goldenView{{browserServerView(), serverSelect()}, {browserView(), browserSelect()}}
	first := emitAll(t, root, views)
	for i := 0; i < 3; i++ {
		again := emitAll(t, root, []goldenView{{browserServerView(), serverSelect()}, {browserView(), browserSelect()}})
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

// TestDir は「配置」: 名前空間のファイルは Dir(N)（View.Config.Dir）の下に置く。
// 親の out を継いだ子は Dir がサブディレクトリになっている。
func TestDir(t *testing.T) {
	root := newProjectRoot(t)
	v := smallView(&ir.Scalar{DeclBase: base("app.admin.Id", nil), Base: ir.String()},
		&ir.Op{DeclBase: base("app.admin.use", nil), Params: []ir.OpParam{param("id", ir.Ref("app.admin.Id"))}})
	v.Viewer = "app.admin"
	v.Program.Namespaces[2].Name = "app.admin"
	v.Config = ir.NamespaceConfig{Language: "go", Out: "gen/app", Dir: "gen/app/admin"}
	v.Namespaces = map[string]ir.NamespaceConfig{"app": nsConfig("app"), "app.admin": v.Config}
	files, diags := Emit(v, Options{Root: root, Select: selectAll(v)})
	if len(diags) > 0 {
		t.Fatal(formatDiags(diags))
	}
	types := contentOf(files, "gen/app/admin/types.go")
	if !strings.Contains(types, "package admin\n") || !strings.Contains(types, "type Id string") {
		t.Errorf("gen/app/admin/types.go（package admin）を期待したが %v:\n%s", paths(files), types)
	}
}

// TestDirFallsBackToOut は Dir が空の View（手で組んだもの）では Conf.out に置くこと（doc.go「文書にない決定」）。
func TestDirFallsBackToOut(t *testing.T) {
	root := newProjectRoot(t)
	v := smallView(&ir.Scalar{DeclBase: base("app.Id", nil), Base: ir.String()}, use("app.Id"))
	v.Config.Dir = ""
	files, diags := Emit(v, Options{Root: root, Select: selectAll(v)})
	if len(diags) > 0 {
		t.Fatal(formatDiags(diags))
	}
	if contentOf(files, "gen/app/types.go") == "" {
		t.Errorf("gen/app/types.go を期待したが %v", paths(files))
	}
}

// TestImportOrCopy は「配置」: 別の名前空間 M の型は、M が Go で同じプロジェクト（View.Namespaces にある）で、M の出力が
// その型を持つ（Options.Exported）なら Dir(M) のパッケージを import して使い、そうでなければ viewer のパッケージに複製する。
func TestImportOrCopy(t *testing.T) {
	root := newProjectRoot(t)
	other := ir.Namespace{Name: "lib.core", Decls: []ir.Decl{
		&ir.Scalar{DeclBase: base("lib.core.Id", nil), Base: ir.String()},
		&ir.Scalar{DeclBase: base("lib.core.Tag", nil), Base: ir.String()},
		&ir.Op{DeclBase: base("lib.core.find", nil), Params: []ir.OpParam{param("id", ir.Ref("lib.core.Id"))}},
	}}
	v := smallView(&ir.Model{DeclBase: base("app.User", nil), Fields: []ir.Field{field("id", ir.Ref("lib.core.Id")), field("tag", ir.Ref("lib.core.Tag"))}}, use("app.User"))
	v.Program.Namespaces = append(v.Program.Namespaces, other)
	v.Namespaces["lib.core"] = ir.NamespaceConfig{Language: "go", Out: "gen/lib", Dir: "gen/lib/core"}
	libView := &ir.View{Viewer: "lib.core", Config: v.Namespaces["lib.core"], Namespaces: v.Namespaces, Program: v.Program, Ops: map[string]*ir.ViewedSignature{}}
	libSelect := plugin.Select{Roots: []string{"lib.core.find"}}
	sel := selectAll(v)

	// lib.core の出力が持つ型は Id だけ（find から辿れる）。Tag はどの op も参照しない
	if got := Exports(libView, libSelect); strings.Join(got, ",") != "lib.core.Id" {
		t.Fatalf("Exports(lib.core) = %v", got)
	}
	if got := Uses(v, sel); strings.Join(got, ",") != "lib.core" {
		t.Errorf("Uses(app) = %v, want [lib.core]", got)
	}
	if got := Uses(libView, libSelect); len(got) != 0 {
		t.Errorf("Uses(lib.core) = %v, want none", got)
	}

	files := emitAll(t, root, []goldenView{{libView, libSelect}, {v, sel}})
	types := contentOf(files, "gen/app/types.go")
	wantContains(t, "gen/app/types.go", types,
		[]string{`"example.com/app/gen/lib/core"`, "Id  core.Id", "Tag Tag", "type Tag string"}, // Id は import、Tag は複製
		[]string{"type Id string"})
	compileFiles(t, root, files)

	// 相手の出力がその型を持たなければ（プラグインがない、選ばれていない）、Go の名前空間でも複製する
	files, diags := Emit(v, Options{Root: root, Select: sel})
	if len(diags) > 0 {
		t.Fatal(formatDiags(diags))
	}
	wantContains(t, "gen/app/types.go", contentOf(files, "gen/app/types.go"), []string{"type Id string", "type Tag string"}, []string{"gen/lib"})

	// View.Namespaces になければ（語彙など）、Exported にあっても複製する
	delete(v.Namespaces, "lib.core")
	files, diags = Emit(v, Options{Root: root, Select: sel, Exported: map[string]bool{"lib.core.Id": true}})
	if len(diags) > 0 {
		t.Fatal(formatDiags(diags))
	}
	wantContains(t, "gen/app/types.go", contentOf(files, "gen/app/types.go"), []string{"type Id string", "type Tag string"}, []string{"gen/lib"})
	if got := Uses(v, sel); len(got) != 0 {
		t.Errorf("Uses(app) = %v, want none", got)
	}
}

// TestCopiesForeignException は「配置」の複製が、op の（viewer から見た）Signature.exceptions にある
// 別の名前空間（語彙）の exception と、その値が参照する model にも及ぶこと。葉の実装者が投げ、
// go-cli の cli.go が errors.As で見分けるための Go の型が viewer のパッケージに要る
// （cli.hozo の proc.Crashed / proc.MalformedOutput で見つかった不具合の回帰テスト）。
func TestCopiesForeignException(t *testing.T) {
	root := newProjectRoot(t)
	proc := ir.Namespace{Name: "proc", Decls: []ir.Decl{
		&ir.Model{DeclBase: base("proc.Detail", doc("異常終了の内容")), Fields: []ir.Field{field("code", ir.Int())}},
		&ir.Exception{DeclBase: base("proc.Crashed", doc("プロセスが異常終了した")), Value: ir.Ref("proc.Detail")},
		&ir.Exception{DeclBase: base("proc.MalformedOutput", doc("出力が約束した形でなかった"))},
	}}
	run := &ir.Op{DeclBase: base("app.run", doc("走らせる")), Returns: ir.String(),
		Signature: ir.Signature{Exceptions: []string{"proc.Crashed", "proc.MalformedOutput"}}}
	ns := ir.Namespace{Name: "app", Decls: []ir.Decl{run}}
	p := &ir.Program{Version: ir.Version, Namespaces: []ir.Namespace{{Name: ""}, validVocab(), proc, ns}}
	v := &ir.View{Viewer: "app", Config: nsConfig("app"), Namespaces: namespaces(), Program: p,
		Ops: map[string]*ir.ViewedSignature{"app.run": {Signature: run.Signature, Params: run.Params}}}
	files, diags := Emit(v, Options{Root: root, Select: selectAll(v)})
	if len(diags) > 0 {
		t.Fatalf("診断が出た: %s", formatDiags(diags))
	}
	wantContains(t, "gen/app/types.go", contentOf(files, "gen/app/types.go"), []string{
		"type Crashed struct{ Value Detail }",
		`func (Crashed) HozoException() string { return "proc.Crashed" }`,
		"type MalformedOutput struct{}",
		"type Detail struct {",
	}, nil)
	compileFiles(t, root, files)
}

// stateDecls は状態のテストで使う宣言: list / nullable / single の状態と、状態の分類。
func stateDecls() (carts, token, title *ir.State, checkout *ir.Classify) {
	carts = &ir.State{DeclBase: base("app.carts", nil), Shape: ir.StateList, Element: ir.String()}
	token = &ir.State{DeclBase: base("app.token", nil), Shape: ir.StateNullable, Element: ir.String()}
	title = &ir.State{DeclBase: base("app.title", nil), Shape: ir.StateSingle, Element: ir.String()}
	checkout = &ir.Classify{DeclBase: base("app.Checkout", nil), Subject: ir.StatesSubject{States: []string{"app.carts"}},
		Cases: []ir.ClassifyCase{{Name: "Shopping"}, {Name: "Ordered"}}}
	return
}

// TestBehavior は、golden の View から生成したコードを実際に動かす: 一時のモジュールに書き、人やプラグインが実装する側
// （Leaves、States、Deps、Wrappers、Classifiers、Constraints）の偽物を置いたテスト testdata/<case>/behavior_test.go を
// そのパッケージに足して `go test` する。確かめるのは docs/emit-go.md「実装の単位」（nil のフィールドは
// *runtime.MissingError）、「state」「classify」「match」「配線の本体」「デコレータ」の振る舞い。
func TestBehavior(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go が PATH にない")
	}
	for _, tc := range []struct{ name, pkg string }{
		{"state", "shop"},           // 状態の単位、状態の分類、状態での match
		{"browser", "browser"},      // Deps、Wrappers、Classifiers、bind
		{"constraint", "inventory"}, // valid に足した制約
	} {
		t.Run(tc.name, func(t *testing.T) {
			var views []goldenView
			for _, g := range goldenCases {
				if g.name == tc.name {
					views = g.views
				}
			}
			root := newProjectRoot(t)
			files := emitAll(t, root, views)
			behavior, err := os.ReadFile(filepath.Join("testdata", tc.name, "behavior_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, File{Path: "gen/" + tc.pkg + "/behavior_test.go", Content: behavior})
			compileFiles(t, root, files)
			cmd := exec.Command(goBin, "test", "./gen/"+tc.pkg+"/")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("生成したコードのテストが落ちた: %v\n%s", err, out)
			}
		})
	}
}
