package validvocab

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// このファイルは、valid の語彙の、書いたものの整合の検査（docs/plugin-valid.md「診断」の VALID-1〜VALID-4）と、制約の読み方
// （同「制約」）の単体テスト（docs/testing.md「単体テスト」の `plugin/validvocab`）。
//
// 入力は testdata/<名前>/ の .hozo。このパッケージは internal/ を import できない（docs/packages.md「依存の向き」）ので、
// .hozo を自分では解析しない: hozoc が語彙のプラグインに渡すのと同じ仕様（docs/plugin-protocol.md「read」の spec）を、
// tests/pluginviews_test.go の TestPluginSpecFixtures が testdata/<名前>/spec.json に置き、ここではそれを読む。
// 期待する診断は testdata/<名前>/expected/errors.txt（一行に、コード、位置、メッセージをタブで区切って。返した順）。

// program は、名前空間を order の順に持つ仕様。
func program(namespaces map[string][]ir.Decl, order ...string) *ir.Program {
	p := &ir.Program{}
	for _, name := range order {
		p.Namespaces = append(p.Namespaces, ir.Namespace{Name: name, Decls: namespaces[name]})
	}
	return p
}

// loadSpec は、入力 name の仕様（testdata/<name>/spec.json）。
func loadSpec(t *testing.T, name string) *ir.Spec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name, "spec.json"))
	if err != nil {
		t.Fatalf("%v（go test ./tests -run TestPluginSpecFixtures -update で作る）", err)
	}
	var spec ir.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Program == nil {
		t.Fatalf("testdata/%s/spec.json に program がない", name)
	}
	if name == "clean" {
		addCustom(t, spec.Program)
	}
	return &spec
}

// addCustom は、入力 clean の IR に、標準でない valid の制約 @valid.checkDigit を足す: scalar app.Jan と、app.Form / app.More の
// フィールド code（受け継いだ、展開済みのフィールドにも）。標準でない valid の制約は、ほかのプラグインの語彙のファイルが、valid に
// 足したもので、仕様（.hozo）からは、足せない（docs/vocab.md「語彙の名前空間」。RES-8）ので、読んだ後の IR に足す。
func addCustom(t *testing.T, p *ir.Program) {
	t.Helper()
	custom := ir.Application{Decorator: "valid.checkDigit"}
	jan, ok := p.Lookup("app.Jan").(*ir.Scalar)
	if !ok {
		t.Fatal("clean に app.Jan がない")
	}
	jan.Decorators = append(jan.Decorators, custom)
	n := 0
	for _, fqn := range []string{"app.Form", "app.More"} {
		m, ok := p.Lookup(fqn).(*ir.Model)
		if !ok {
			t.Fatalf("clean に %s がない", fqn)
		}
		for i := range m.Fields {
			if m.Fields[i].Name == "code" {
				m.Fields[i].Decorators = append(m.Fields[i].Decorators, custom)
				n++
			}
		}
	}
	if n != 2 {
		t.Fatalf("clean の、フィールド code が %d 個（app.Form と、受け継いだ app.More の二つのはず）", n)
	}
}

// diagnosticFixtures は、診断を返す入力（testdata/<名前>/）と、その診断のコード。
var diagnosticFixtures = []struct{ name, code string }{
	{"valid1", CodeTarget},   // 制約が、付けた先に合わない
	{"valid2", CodeArgument}, // 制約の引数が、足りないか、先に合わない
	{"valid3", CodePattern},  // pattern の regex を、RE2 で解釈できない
	{"valid4", CodeFormat},   // format の name が、四つの形式のどれでもない
}

// TestDiagnosticCodes は、「診断」の表の ID（VALID-1〜VALID-4。欠番はない）のどれもが、入力を一つ持つこと。
func TestDiagnosticCodes(t *testing.T) {
	seen := map[string]bool{}
	for _, fx := range diagnosticFixtures {
		seen[fx.code] = true
	}
	for n := 1; n <= 4; n++ {
		if code := "VALID-" + strconv.Itoa(n); !seen[code] {
			t.Errorf("%s の入力がない", code)
		}
	}
	if len(seen) != 4 {
		t.Errorf("診断の入力のコードが %d 種類（4 のはず）", len(seen))
	}
}

// TestDiagnostics は docs/plugin-valid.md「診断」の VALID-1〜VALID-4: 診断のコード・位置・メッセージがゴールデンと同じこと。
// 同じ仕様から、同じ診断を返すこと（決定性）も確かめる。
func TestDiagnostics(t *testing.T) {
	for _, fx := range diagnosticFixtures {
		t.Run(fx.name, func(t *testing.T) {
			diags := Check(loadSpec(t, fx.name).Program)
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
			path := filepath.Join("testdata", fx.name, "expected", "errors.txt")
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if b.String() != string(want) {
				t.Errorf("%s と違う:\n--- 期待\n%s--- 実際\n%s", path, want, b.String())
			}
			if again := Check(loadSpec(t, fx.name).Program); !reflect.DeepEqual(diags, again) {
				t.Error("二度目の検査で、診断が変わった")
			}
		})
	}
}

// TestClean は「対象」: 標準の制約の、正しい使い方と、標準でない valid の制約（読まない）は、診断にならない。
// 受け継いだ制約も、同じに読む（展開済みのフィールド）。
func TestClean(t *testing.T) {
	p := loadSpec(t, "clean").Program
	if diags := Check(p); len(diags) != 0 {
		t.Fatalf("診断になった: %+v", diags)
	}
	if Check(nil) != nil {
		t.Error("仕様がないのに、診断がある")
	}
	form, _ := p.Lookup("app.Form").(*ir.Model)
	more, _ := p.Lookup("app.More").(*ir.Model)
	if form == nil || more == nil {
		t.Fatal("app.Form か app.More がない")
	}
	names := func(m *ir.Model, field string) []string {
		for i := range m.Fields {
			if m.Fields[i].Name == field {
				cons, problems := Field(p, m, &m.Fields[i])
				if len(problems) != 0 {
					t.Errorf("%s.%s: 誤り %+v", m.Name, field, problems)
				}
				var out []string
				for _, c := range cons {
					out = append(out, c.Name)
				}
				return out
			}
		}
		t.Fatalf("%s にフィールド %s がない", m.Name, field)
		return nil
	}
	for _, m := range []*ir.Model{form, more} {
		for field, want := range map[string][]string{
			"tags":  {NameCount},           // 配列のフィールドの length は、要素の数
			"later": {NameCount},           // 省略可能な配列も
			"name":  {"pattern", "format"}, // 省略可能は、中の型で見る。書かれた順
			"ratio": {"min"},
			"code":  nil, // 標準でない valid の制約（addCustom が足した）は、読まない
			"attrs": nil,
			"page":  nil, // 型の scalar に付いた制約は、フィールドの制約ではない
		} {
			if got := names(m, field); !reflect.DeepEqual(got, want) {
				t.Errorf("%s.%s の制約 = %v, want %v", m.Name, field, got, want)
			}
		}
	}
	if got := names(more, "extra"); !reflect.DeepEqual(got, []string{"max"}) {
		t.Errorf("app.More.extra の制約 = %v", got)
	}
}

// TestScalar は「制約」の表: scalar に付いた標準の制約を、書かれた順に、引数ごとに読む。元の型は、継承を辿った核の型。
func TestScalar(t *testing.T) {
	p := loadSpec(t, "clean").Program
	read := func(fqn string) []Constraint {
		s, ok := p.Lookup(fqn).(*ir.Scalar)
		if !ok {
			t.Fatalf("%s がない", fqn)
		}
		cons, problems := Scalar(p, s)
		if len(problems) != 0 {
			t.Errorf("%s: 誤り %+v", fqn, problems)
		}
		return cons
	}
	if got, want := read("app.Tag"), []Constraint{{Decorator: Length, Name: "length", Core: ir.CoreString, HasMin: true, Min: 1, HasMax: true, Max: 20}}; !reflect.DeepEqual(got, want) {
		t.Errorf("app.Tag = %+v, want %+v", got, want)
	}
	if got, want := read("app.Page"), []Constraint{
		{Decorator: Min, Name: "min", Core: ir.CoreInt, IntBound: 1, BoundText: "1"},
		{Decorator: Max, Name: "max", Core: ir.CoreInt, IntBound: 100, BoundText: "100"},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("app.Page = %+v, want %+v", got, want)
	}
	if got, want := read("app.SmallPage"), []Constraint{{Decorator: Max, Name: "max", Core: ir.CoreInt, IntBound: 10, BoundText: "10"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("app.SmallPage = %+v, want %+v", got, want)
	}
	if got := read("app.Jan"); got != nil {
		t.Errorf("利用者が足した制約を読んだ: %+v", got)
	}
}

// TestNames は「対象」: 標準の制約は五つ。`@valid.*` は、名前空間 valid に宣言されたデコレータ（利用者が足したものを含む）。
func TestNames(t *testing.T) {
	for _, d := range []string{Min, Max, Length, Pattern, Format} {
		if !Standard(d) || !Applied(d) {
			t.Errorf("%s が、標準の制約でない", d)
		}
	}
	if Standard("valid.checkDigit") || !Applied("valid.checkDigit") {
		t.Error("標準の五つのほかの制約は、標準でない `@valid.*`")
	}
	for _, d := range []string{"valid.sub.x", "valid.sub.min", "validx.min", "x.valid.min", "valid", "valid.", "min", "http.query", "db.key", ""} {
		if Standard(d) || Applied(d) {
			t.Errorf("%q は、valid の語彙のデコレータでない", d)
		}
	}
	if got := Formats(); !reflect.DeepEqual(got, []string{"email", "uri", "uuid", "date-time"}) {
		t.Errorf("形式の名前 = %v", got)
	}
}

// TestDiagnosticPositions は、診断の位置（docs/plugin-valid.md「診断」）: 制約の付いた scalar か、フィールドを持つ model の FQN。
func TestDiagnosticPositions(t *testing.T) {
	for name, want := range map[string][]string{
		"valid1": {"app.Name", "app.Wait", "app.Count", "app.Flag", "app.At", "app.Alias", "app.Form"},
		"valid2": {"app.Half", "app.Any", "app.Form"},
		"valid3": {"app.Look", "app.Form"},
		"valid4": {"app.Phone", "app.Form"},
	} {
		got := map[string]bool{}
		var diags []plugin.Diagnostic = Check(loadSpec(t, name).Program)
		for _, d := range diags {
			got[d.At] = true
		}
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

// TestIntBoundExact は「制約」: 元の型が int の先の min / max の value は、整数を、丸めずに読む（2^53 を超える値と、
// int64 の最小値も）。整数の小数のリテラル（1.0）も、整数として読む。整数でない小数と、int64 に収まらない小数は VALID-2。
func TestIntBoundExact(t *testing.T) {
	read := func(lit ir.Literal) (Constraint, []Problem) {
		s := &ir.Scalar{DeclBase: ir.DeclBase{Name: "app.N", Decorators: []ir.Application{{
			Decorator: Min, Args: []ir.Arg{{Param: "value", Value: ir.LiteralValue{Literal: lit}}},
		}}}, Base: ir.Int()}
		cons, problems := Scalar(&ir.Program{}, s)
		if len(cons) == 1 {
			return cons[0], problems
		}
		return Constraint{}, problems
	}
	for _, v := range []int64{math.MinInt64, math.MaxInt64, 1<<53 + 1, -(1<<53 + 1)} {
		c, problems := read(ir.IntLiteral{Value: v})
		if len(problems) != 0 {
			t.Errorf("%d: 誤り %+v", v, problems)
			continue
		}
		if c.IntBound != v {
			t.Errorf("%d: IntBound = %d", v, c.IntBound)
		}
		if want := strconv.FormatInt(v, 10); c.BoundText != want {
			t.Errorf("%d: BoundText = %s, want %s", v, c.BoundText, want)
		}
	}
	if c, problems := read(ir.FloatLiteral{Value: 3}); len(problems) != 0 || c.IntBound != 3 {
		t.Errorf("3.0: IntBound = %d, 誤り %+v", c.IntBound, problems)
	}
	for _, f := range []float64{1.5, 1 << 63, math.Inf(1)} {
		if _, problems := read(ir.FloatLiteral{Value: f}); len(problems) != 1 || problems[0].Code != CodeArgument {
			t.Errorf("%v: 誤り %+v, want VALID-2", f, problems)
		}
	}
	if c, problems := read(ir.FloatLiteral{Value: -(1 << 63)}); len(problems) != 0 || c.IntBound != math.MinInt64 {
		t.Errorf("-2^63 の小数: IntBound = %d, 誤り %+v", c.IntBound, problems)
	}
}
