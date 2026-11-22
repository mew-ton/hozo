package httpvocab

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// このファイルは、http の語彙の、書いたものの整合の検査（docs/plugin-http.md「診断」の HTTP-1〜HTTP-15）の単体テスト
// （docs/testing.md「単体テスト」の `plugin/httpvocab`）。
//
// 入力は testdata/<名前>/ の .hozo。このパッケージは internal/ を import できない（docs/packages.md「依存の向き」）ので、
// .hozo を自分では解析しない: hozoc が語彙のプラグインに渡すのと同じ仕様（docs/plugin-protocol.md「read」の spec）を、
// tests/pluginviews_test.go の TestPluginSpecFixtures が testdata/<名前>/spec.json に置き、ここではそれを読む。
// 期待する診断は testdata/<名前>/expected/errors.txt（一行に、コード、位置、メッセージをタブで区切って。返した順）。

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
	return &spec
}

// diagnosticFixtures は、診断を返す入力（testdata/<名前>/）と、その診断のコード。
var diagnosticFixtures = []struct{ name, code string }{
	{"http1", CodeExport},      // @http.get / @http.post の付いた op に export がない
	{"http2", CodeParams},      // 経路の op の引数が、0 個でも model 一つでもない
	{"http3", CodePlace},       // 入力の model のフィールドが、場所の六つのデコレータのどれも持たない、または二つ以上持つ
	{"http4", CodeFieldType},   // ヘッダかクエリから読むフィールド（全体の形なら、その model のフィールド）の型が受け付けられない
	{"http5", CodeJSON},        // 本文、結果、@http.status を持つ例外の引数の型に、通信に出せない union がある
	{"http6", CodeGetBody},     // @http.get の op の入力が、本文のデコレータのフィールドを持つ
	{"http7", CodePath},        // パスが / で始まらない、または { か } を含む
	{"http8", CodeDuplicate},   // 一つの名前空間の中に、同じメソッドとパスの経路が二つある
	{"http9", CodeWholeType},   // 全体の形を付けたフィールドの型が、model でない
	{"http10", CodeWholeMix},   // 一つの場所に、全体の形が二つ以上ある、または、全体の形と項目の形の両方がある
	{"http11", CodeNoItem},     // @http.required か @http.default の付いたフィールドが、項目の形を持たない
	{"http12", CodeOptional},   // @http.required か @http.default の付いたフィールドの型が、省略可能
	{"http13", CodeBoth},       // 一つのフィールドに、@http.required と @http.default の両方がある
	{"http14", CodeDefault},    // @http.default のリテラルが、フィールドの型に渡せない
	{"http15", CodeConstraint}, // @http.default の値が、フィールドの制約（@valid.*）に合わない
}

// TestDiagnosticCodes は、「診断」の表の ID（HTTP-1〜HTTP-15。欠番はない）のどれもが、入力を一つ持つこと。
func TestDiagnosticCodes(t *testing.T) {
	seen := map[string]bool{}
	for _, fx := range diagnosticFixtures {
		seen[fx.code] = true
	}
	for n := 1; n <= 15; n++ {
		if code := "HTTP-" + strconv.Itoa(n); !seen[code] {
			t.Errorf("%s の入力がない", code)
		}
	}
	if len(seen) != 15 {
		t.Errorf("診断の入力のコードが %d 種類（15 のはず）", len(seen))
	}
}

// TestDiagnostics は docs/plugin-http.md「診断」の HTTP-1〜HTTP-15: 診断のコード・位置・メッセージがゴールデンと同じこと。
// 同じ仕様から、同じ診断を返すこと（決定性）も確かめる。
func TestDiagnostics(t *testing.T) {
	for _, fx := range diagnosticFixtures {
		t.Run(fx.name, func(t *testing.T) {
			spec := loadSpec(t, fx.name)
			diags := Check(spec.Program)
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
			if again := Check(loadSpec(t, fx.name).Program); !sameDiagnostics(diags, again) {
				t.Error("二度目の検査で、診断が変わった")
			}
		})
	}
}

func sameDiagnostics(a, b []plugin.Diagnostic) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDiagnosticPositions は、診断の位置（docs/plugin-http.md「診断」の表）: 表に書いた宣言の FQN。
func TestDiagnosticPositions(t *testing.T) {
	for name, want := range map[string][]string{
		"http1": {"app.hidden", "app.also"},
		"http2": {"app.two", "app.core", "app.byId", "app.optional", "app.many", "app.either", "app.byIndex"},
		"http3": {"app.In"},
		// 全体の形で受け取る model のフィールドの型は、その model の位置
		"http4":  {"app.HeaderIn", "app.QueryIn", "app.Meta", "app.Paging"},
		"http5":  {"app.putShape", "app.putAlbum", "app.putDrawing", "app.getDrawing", "app.getShapes", "app.getSame", "app.getEmpty", "app.getSub", "app.getIndexed", "app.clash", "app.refused"},
		"http6":  {"app.fetch", "app.both", "app.whole"},
		"http7":  {"app.relative", "app.empty", "app.absolute", "app.byId", "app.brace"},
		"http8":  {"app.second", "app.third", "app.again"},
		"http9":  {"app.In", "app.More"},
		"http10": {"app.Twice", "app.Mixed"},
		// 全体の形のフィールドは入力の model、その中のフィールドは、全体の形で受け取る model の位置
		"http11": {"app.In", "app.Paging"},
		"http12": {"app.In"},
		"http13": {"app.In"},
		"http14": {"app.In"},
		"http15": {"app.In"},
	} {
		got := map[string]bool{}
		for _, d := range Check(loadSpec(t, name).Program) {
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

// TestRoutesByNamespace は「対象」: 経路は、名前空間ごとに読み、HTTP-8（同じメソッドとパス）は、一つの名前空間の中でだけ比べる。
// target で絞れば、絞った op だけが、経路になる（トランスパイラが、自分の出力の op だけを読むときの形）。
func TestRoutesByNamespace(t *testing.T) {
	spec := loadSpec(t, "namespaces")
	if diags := Check(spec.Program); len(diags) != 0 {
		t.Fatalf("別の名前空間の、同じ経路が、診断になった: %+v", diags)
	}
	r := NewReader(spec.Program)
	a, b := r.Routes("a", nil), r.Routes("b", nil)
	if len(a) != 2 || len(b) != 1 || a[0].Method != "GET" || a[0].Path != "/items" || b[0].Path != "/items" {
		t.Fatalf("経路 a = %+v, b = %+v", a, b)
	}
	if a[1].Model == nil || len(a[1].Inputs) != 1 || a[1].Inputs[0].Location() != "body.name" || !a[1].Inputs[0].Required || !a[1].HasBody() {
		t.Errorf("入力の読み方 = %+v", a[1].Inputs)
	}
	only := NewReader(spec.Program).Routes("a", func(op *ir.Op) bool { return op.Name == "a.put" })
	if len(only) != 1 || only[0].Op.Name != "a.put" {
		t.Errorf("絞った経路 = %+v", only)
	}
	if got := NewReader(spec.Program).Routes("nowhere", nil); got != nil {
		t.Errorf("ない名前空間の経路 = %+v", got)
	}
}
