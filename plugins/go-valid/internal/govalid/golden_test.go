package govalid

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
	"github.com/mew-ton/hozo/plugin/govalid"
)

// goldenFixtures は、出力をゴールデンと比べる入力（testdata/<名前>/）と、それが確かめる docs/plugin-go-valid.md の見出し。
// behavior は、生成したコードを動かすテスト testdata/<名前>/behavior_test.go を持つもの。
var goldenFixtures = []struct {
	name     string
	covers   string
	behavior bool
}{
	{name: "constraints", covers: "「制約」の表の各行、「scalar」「model」（省略可能、array、record）、「出力」の置き場と補助の関数、「Validate を持つ型」（その場で書いた model、union と exception）", behavior: true},
	{name: "locals", covers: "元の scalar のパッケージの名前が、生成したコードの局所の名前や、標準ライブラリのパッケージの名前と当たるとき（docs/plugin-go.md「パッケージと import パス」の import の名前）"},
}

// TestGolden は、入力ごとに、出力がゴールデン testdata/<名前>/expected/ とバイト単位で同じこと、go/format を通した形で
// あること、同じ View から同じバイト列を返すこと、基底 go の出力と合わせてコンパイルできる（一時のモジュールで go vet を通る）
// ことを確かめる。
func TestGolden(t *testing.T) {
	for _, fx := range goldenFixtures {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			views := loadViews(t, fx.name)
			root, files := emitFixture(t, fx.name)
			checkFormatted(t, files)
			compareTree(t, fx.name, files)
			// 決定性: もう一度、別のルート（同じモジュールのパス）で出力しても、同じバイト列
			again, _ := emitViews(t, newRoot(t), views)
			if len(again) != len(files) {
				t.Errorf("二度目の出力のファイルの数が違う: %d と %d", len(files), len(again))
			}
			for p, content := range files {
				if again[p] != content {
					t.Errorf("%s が、二度目の出力で変わった", p)
				}
			}
			vet(t, root, views, files)
		})
	}
}

// TestBehavior は、生成したコードを一時のモジュールで実際に動かす（docs/testing.md「単体テスト」）: 入力の behavior_test.go を、
// 基底 go の出力と go-valid の出力を書いたモジュールのルートに置いて、go test を走らせる。確かめることは、それぞれのファイルの頭。
func TestBehavior(t *testing.T) {
	for _, fx := range goldenFixtures {
		if !fx.behavior {
			continue
		}
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			root, files := emitFixture(t, fx.name)
			compose(t, root, loadViews(t, fx.name), files)
			test, err := os.ReadFile(filepath.Join("testdata", fx.name, "behavior_test.go"))
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
		})
	}
}

// TestPlacement は「出力」の置き場: validate.go は、N の、型のパッケージに一つ（docs/plugin-go.md「型のパッケージ」）。関数を持つ
// 名前空間では <Dir>/types/（パッケージ types）、持たない名前空間では入口と同じパッケージ。
func TestPlacement(t *testing.T) {
	_, files := emitFixture(t, "constraints")
	want := map[string]string{
		"gen/type/validate.go":        "package pkgtype\n",
		"gen/app/validate.go":         "package app\n",
		"gen/split/types/validate.go": "package types\n",
	}
	if len(files) != len(want) {
		t.Errorf("ファイル = %v, want %d 個", sortedPaths(files), len(want))
	}
	for p, pkg := range want {
		if !strings.Contains(files[p], "\n"+pkg) {
			t.Errorf("%s が %q を持たない", p, strings.TrimSpace(pkg))
		}
	}
}

// TestTargets は「Validate を持つ型」: メソッドを出すのは、N の scalar と model だけ。その場で書いた model（union のアームの
// `{ … }`）は、普通の model として数える。union と exception には出さない: 例外は、引数の型が Validate を持つ型でも（scalar、model、
// 別の名前空間の型、包まれた型）、持たない（例外の値を確かめる者は、いない）。
func TestTargets(t *testing.T) {
	_, files := emitFixture(t, "constraints")
	content := files["gen/app/validate.go"]
	var view *ir.View
	for _, v := range loadViews(t, "constraints") {
		if v.Viewer == "app" {
			view = v
		}
	}
	// 入力が、確かめたい形を持っていること: 引数の型が Validate を持つ例外と、その場で書いた model
	rejected, ok := view.Program.Lookup("app.Rejected").(*ir.Exception)
	if !ok || len(rejected.Params) < 2 {
		t.Fatalf("入力に、引数を二つ以上持つ例外 app.Rejected がない")
	}
	validated := 0
	for _, p := range rejected.Params {
		if r, isRef := govalid.Through(view.Program, p.Type).(ir.RefType); isRef && govalid.HasValidate(view.Program, r.Target) {
			validated++
		}
	}
	if validated < 2 {
		t.Errorf("例外 app.Rejected の引数のうち、型が Validate を持つものが %d 個（二つ以上のはず）", validated)
	}
	if m, ok := view.Program.Lookup("app.Outcome.Failed").(*ir.Model); !ok || m.SynthesizedFrom != "app.Outcome" {
		t.Fatalf("入力に、union app.Outcome のアームの、その場で書いた model がない")
	}
	for _, want := range []string{"func (m OutcomeFailed) Validate() error {", "func (m Item) Validate() error {", "func (v Percent) Validate() error {"} {
		if !strings.Contains(content, want) {
			t.Errorf("gen/app/validate.go に、%q がない", want)
		}
	}
	for _, banned := range []string{"Rejected)", "Plainly)", "OutcomeSkipped)", "Outcome)", "Shape)", "OutcomeDone)"} {
		if strings.Contains(content, banned+" Validate()") {
			t.Errorf("gen/app/validate.go が、%s Validate() を出した", banned)
		}
	}
	// メソッドのレシーバは、Validate を持つ N の scalar と model と、union の、包む型のアームのうち、アームの型が Validate を持つ型への
	// ref のもの（「出力」）の、ちょうどすべて（宣言順。包む型は、union の位置に、アームの順）
	var want, got []string
	for _, d := range view.Program.Namespace("app").Decls {
		switch x := d.(type) {
		case *ir.Scalar, *ir.Model:
			if govalid.HasValidate(view.Program, d.Common().Name) {
				want = append(want, golang.TypeName(d.Common().Name, "app"))
			}
		case *ir.Union:
			for _, a := range x.Arms {
				r, isRef := a.Type.(ir.RefType)
				if !isRef || !govalid.HasValidate(view.Program, r.Target) {
					continue
				}
				if _, isModel := view.Program.Lookup(r.Target).(*ir.Model); isModel && strings.HasPrefix(r.Target, "app.") {
					continue // N の model のアームは、包まない
				}
				want = append(want, golang.TypeName(x.Name, "app")+golang.Capitalize(a.Name))
			}
		}
	}
	for _, line := range strings.Split(content, "\n") {
		if rest, isMethod := strings.CutPrefix(line, "func ("); isMethod {
			got = append(got, strings.TrimSuffix(strings.Fields(rest)[1], ")"))
		}
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("gen/app/validate.go のメソッドのレシーバ = %v, want %v", got, want)
	}
}

// TestNone は「出力」: Validate を持つ型が N に一つもなければ、何も出さない（診断も warning もない）。
func TestNone(t *testing.T) {
	root := newRoot(t)
	files, diags := emitViews(t, root, loadViews(t, "none"))
	if len(files) != 0 || len(diags) != 0 {
		t.Fatalf("files=%v 診断=%+v, want どちらも空", sortedPaths(files), diags)
	}
	resp, err := Handler{}.Emit(request(loadViews(t, "none")[0], TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Warnings) != 0 || len(resp.Errors) != 0 {
		t.Errorf("応答 = %+v, want 空", resp)
	}
}

// diagnosticFixtures は、診断を返す入力（testdata/<名前>/）。期待する診断は testdata/<名前>/expected/errors.txt
// （一行に、コード、位置、メッセージをタブで区切って。View の順、その中は返した順）。
var diagnosticFixtures = []struct{ name, code string }{
	{"govalid1", codeValidConstraint}, // 制約が元の型に合わない、length に min も max もない、pattern が RE2 でない、format の名前を知らない、配列か record そのもの
	// GOVALID-2 は欠番（フィールドの既定値を言語からなくした。docs/plugin-go-valid.md「診断」）。番号は使い回さない
	{"govalid3", codeValidHome},   // Validate を持つ別の名前空間の型の置き場の単位が、go-valid を並べていない
	{"govalid4", codeValidUnion},  // フィールドの型が union で、そのアームの型が、Validate を持つ型そのものでなく、その中に Validate を持つ型がある
	{"govalid5", codeValidCustom}, // 標準でない valid の制約が付いている
	{"govalid6", codeValidField},  // Validate を持つ model が、validate という名前のフィールドを持つ
	{"govalid7", codeValidOption}, // [emit.options.go-valid] に、知らないキーがある
}

// TestDiagnosticCodes は、「診断」の表の ID（GOVALID-1〜GOVALID-7。GOVALID-2 は欠番）のどれもが、入力を一つ持つこと。
func TestDiagnosticCodes(t *testing.T) {
	seen := map[string]bool{}
	for _, fx := range diagnosticFixtures {
		seen[fx.code] = true
	}
	for _, code := range []string{codeValidConstraint, codeValidHome, codeValidUnion, codeValidCustom, codeValidField, codeValidOption} {
		if !seen[code] {
			t.Errorf("%s の入力がない", code)
		}
	}
	if seen["GOVALID-2"] {
		t.Error("欠番の GOVALID-2 の入力がある")
	}
	if len(seen) != 6 {
		t.Errorf("診断の入力のコードが %d 種類（6 のはず）", len(seen))
	}
}

// TestDiagnostics は docs/plugin-go-valid.md「診断」の GOVALID-1〜GOVALID-7: 診断のコード・位置・メッセージがゴールデンと同じで、
// 診断を返した出力がファイルを返さないこと（emitViews が確かめる）。
func TestDiagnostics(t *testing.T) {
	for _, fx := range diagnosticFixtures {
		t.Run(fx.name, func(t *testing.T) {
			_, diags := emitViews(t, newRoot(t), loadViews(t, fx.name))
			if len(diags) == 0 {
				t.Fatal("診断がない")
			}
			var b bytes.Buffer
			for _, d := range diags {
				if d.Code != fx.code {
					t.Errorf("コード %s の診断がある（%s だけのはず）: %s", d.Code, fx.code, d.Message)
				}
				b.WriteString(d.Code + "\t" + d.At + "\t" + d.Message + "\n")
			}
			compareTree(t, fx.name, map[string]string{"errors.txt": b.String()})
		})
	}
}

// TestDiagnosticPositions は、診断の位置（docs/plugin-go-valid.md「診断」の表）: 付いた scalar か model の FQN。GOVALID-7 は位置なし。
func TestDiagnosticPositions(t *testing.T) {
	at := func(name string) map[string]bool {
		_, diags := emitViews(t, newRoot(t), loadViews(t, name))
		set := map[string]bool{}
		for _, d := range diags {
			set[d.At] = true
		}
		return set
	}
	for name, want := range map[string][]string{
		"govalid1": {"app.Name", "app.Wait", "app.Count", "app.Flag", "app.At", "app.Alias", "app.Half", "app.Any", "app.Look", "app.Phone", "app.Form"},
		"govalid3": {"app.User", "app.WorkEmail"},
		"govalid4": {"app.Lists"},
		"govalid5": {"app.Jan", "app.Order"},
		"govalid6": {"app.Form", "app.Outer"},
		"govalid7": {""},
	} {
		got := at(name)
		if len(got) != len(want) {
			t.Errorf("%s: 位置 = %v, want %v", name, got, want)
		}
		for _, w := range want {
			if !got[w] {
				t.Errorf("%s: 位置 %q の診断がない（%v）", name, w, got)
			}
		}
	}
}

// TestNoModule: 基底のオプション module がないとき（基底が GO-8 を返す）、go-valid は、ファイルも診断も返さない。
// 仕様だけで決まる診断は、module がなくても返す。
func TestNoModule(t *testing.T) {
	files, diags := emitViews(t, "", noModule(loadViews(t, "constraints")))
	if len(files) != 0 || len(diags) != 0 {
		t.Errorf("files=%v 診断=%+v, want どちらも空（GO-8 は基底が返す）", sortedPaths(files), diags)
	}
	_, diags = emitViews(t, "", noModule(loadViews(t, "govalid1")))
	if len(diags) == 0 || diags[0].Code != codeValidConstraint {
		t.Errorf("診断 = %+v, want GOVALID-1", diags)
	}
}

// TestBrokenView: View が欠けている（Program がない、viewer の名前空間が Program にない）のは、利用者の仕様の誤りではないので、
// 診断ではなく error で終わる（hozoc からは proc.Crashed）。
func TestBrokenView(t *testing.T) {
	view := loadViews(t, "constraints")[0]
	for name, v := range map[string]*ir.View{
		"Program がない":          {Version: ir.Version, Viewer: "type", Config: view.Config},
		"viewer が Program にない": {Version: ir.Version, Viewer: "nowhere", Config: view.Config, Program: view.Program},
	} {
		files, diags, err := Emit(request(v, TranspilerName))
		if err == nil || len(files) != 0 || len(diags) != 0 {
			t.Errorf("%s: files=%d 診断=%+v err=%v, want error だけ", name, len(files), diags, err)
		}
	}
}

// TestHandler は、プラグインの入口（Handler）: 要求の View から出力し、診断は応答の errors で返す
// （docs/plugin-protocol.md「emit」）。import は持たない。
func TestHandler(t *testing.T) {
	view := loadViews(t, "constraints")[0]
	root := newRoot(t)
	h := Handler{}
	resp, err := h.Emit(request(view, TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 1 || len(resp.Errors) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v", len(resp.Files), resp.Errors, resp.Warnings)
	}
	want, _ := emitViews(t, root, []*ir.View{view})
	if f := resp.Files[0]; want[f.Path] != f.Content {
		t.Errorf("%s が Emit の結果と違う", f.Path)
	}
	// 診断は errors で返し、ファイルは返さない
	resp, err = h.Emit(request(loadViews(t, "govalid7")[0], TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Errors) != 2 || resp.Errors[0].Code != codeValidOption {
		t.Errorf("files=%d errors=%+v, want GOVALID-7 が二つ", len(resp.Files), resp.Errors)
	}
	// ほかのトランスパイラの名前は unsupported
	if _, err := h.Emit(request(view, "go")); !errors.Is(err, plugin.ErrUnsupported) {
		t.Errorf("知らない transpiler の err = %v, want ErrUnsupported", err)
	}
	if _, err := h.Emit(&plugin.EmitRequest{Transpiler: TranspilerName}); err == nil {
		t.Error("View のない要求で、エラーにならない")
	}
	if _, err := h.Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import の err = %v, want ErrUnsupported", err)
	}
}

// TestArrayCountAndUnionText は、配列のフィールドの length と、union のアームの検証の、出力のテキスト（docs/plugin-go-valid.md
// 「model」の表、「出力」の、包む型の Validate）を、文書から手で書いたものと比べる。
func TestArrayCountAndUnionText(t *testing.T) {
	_, files := emitFixture(t, "constraints")
	got := files["gen/app/validate.go"]
	for _, want := range []string{
		`// Validate は app.Tagged の各フィールドの制約を確かめる。違反の Path はフィールド名を . でつないだもの。
func (m Tagged) Validate() error {
	var violations []runtime.Violation
	for i0, x0 := range m.Tags {
		violations = validAppend(violations, "tags["+strconv.Itoa(i0)+"]", x0.Validate())
	}
	if l := len(m.Tags); l < 1 || l > 3 {
		violations = append(violations, runtime.Violation{Path: "tags", Constraint: "count"})
	}
	if l := len(m.Notes); l > 2 {
		violations = append(violations, runtime.Violation{Path: "notes", Constraint: "count"})
	}
	if l := len(m.Later); l < 2 {
		violations = append(violations, runtime.Violation{Path: "later", Constraint: "count"})
	}
	if len(violations) == 0 {
		return nil
	}
	return &runtime.ValidationError{Violations: violations}
}
`,
		`// Validate は union app.Party のアーム At（type.Address）の制約を確かめる。違反がなければ nil。
func (v PartyAt) Validate() error {
	return v.Value.Validate()
}

// Validate は union app.Party のアーム Mail（type.Email）の制約を確かめる。違反がなければ nil。
func (v PartyMail) Validate() error {
	return v.Value.Validate()
}
`,
		`// Validate は app.Deal の各フィールドの制約を確かめる。違反の Path はフィールド名を . でつないだもの。
func (m Deal) Validate() error {
	var violations []runtime.Violation
	if u0, ok := m.Buyer.(interface{ Validate() error }); ok {
		violations = validAppend(violations, "buyer", u0.Validate())
	}
	if u0, ok := m.Seller.(interface{ Validate() error }); ok {
		violations = validAppend(violations, "seller", u0.Validate())
	}
	for i0, x0 := range m.Others {
		if u1, ok := x0.(interface{ Validate() error }); ok {
			violations = validAppend(violations, "others["+strconv.Itoa(i0)+"]", u1.Validate())
		}
	}
	for _, k0 := range validKeys(m.ByName) {
		x0 := m.ByName[k0]
		if u1, ok := x0.(interface{ Validate() error }); ok {
			violations = validAppend(violations, "byName["+string(k0)+"]", u1.Validate())
		}
	}
	if len(violations) == 0 {
		return nil
	}
	return &runtime.ValidationError{Violations: violations}
}
`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("gen/app/validate.go に、手で書いたテキストがない:\n%s", want)
		}
	}
}
