// 入力 constraints の出力を動かす（plugins/go-valid の TestBehavior が、基底 go の出力と go-valid の出力を書いたモジュールの
// ルートに置いて走らせる）。確かめること（docs/plugin-go-valid.md）:
//
//   - 「制約」の表の各行の条件（min / max / length / pattern / format）と、違反の Constraint（デコレータの名前）。
//   - 違反がなければ nil、あれば *runtime.ValidationError。違反は、確かめた順にすべて集める（最初の一つで止めない）。
//   - 「scalar」: 元の scalar の Validate が先、次に自分の制約。Path は ""。
//   - 「model」: フィールドの順に、型の Validate（1）、フィールドの制約（2）。Path はフィールド名を . でつないだもの。
//   - 省略可能（nil は確かめない）、array（Path に添字）、record（キーの辞書順。Path にキー）。
//   - Validate を持たない型には、メソッドがない。入口のパッケージの別名からも、そのまま呼べる。
package behavior

import (
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/app"
	"example.com/app/gen/split"
	pkgtype "example.com/app/gen/type"
	"github.com/mew-ton/hozo/runtime"
)

type validator interface{ Validate() error }

// violations は、v.Validate() の違反を "<path>: <constraint>" の列にする（Path が空なら constraint だけ）。nil なら空。
func violations(t *testing.T, v validator) []string {
	t.Helper()
	err := v.Validate()
	if err == nil {
		return nil
	}
	var ve *runtime.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("%T の Validate が *runtime.ValidationError でない誤りを返した: %v", v, err)
	}
	if len(ve.Violations) == 0 {
		t.Fatalf("%T の Validate が、違反のない *runtime.ValidationError を返した", v)
	}
	out := make([]string, len(ve.Violations))
	for i, x := range ve.Violations {
		out[i] = x.String()
	}
	return out
}

func expect(t *testing.T, v validator, want ...string) {
	t.Helper()
	got := violations(t, v)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%T(%+v): 違反 = %q, want %q", v, v, got, want)
	}
}

func ptr[T any](v T) *T { return &v }

func TestScalarConstraints(t *testing.T) {
	// min / max（int）: 境界は含む
	expect(t, app.Percent(1))
	expect(t, app.Percent(100))
	expect(t, app.Percent(0), "min")
	expect(t, app.Percent(101), "max")
	// min / max（float）。書かれた順（max、min）に確かめる
	expect(t, app.Ratio(0.5))
	expect(t, app.Ratio(10))
	expect(t, app.Ratio(0.49), "min")
	expect(t, app.Ratio(10.01), "max")
	// length: 文字（コードポイント）の数で数える。片方だけなら、その比較だけ
	expect(t, app.Code("abc"))
	expect(t, app.Code("ab"), "length")
	expect(t, app.Code("あいう"))
	expect(t, app.Short("あいうえお"))
	expect(t, app.Short("あいうえおか"), "length")
	expect(t, app.Short(""))
	// pattern
	expect(t, app.Sku("AB-12"))
	expect(t, app.Sku("ab-12"), "pattern")
	// format
	expect(t, app.Link("https://example.com/a"))
	expect(t, app.Link("/relative"), "format")
	expect(t, app.Uid("123e4567-e89b-12d3-a456-426614174000"))
	expect(t, app.Uid("123"), "format")
	expect(t, app.Stamp("2024-01-02T03:04:05Z"))
	expect(t, app.Stamp("2024-01-02"), "format")
	expect(t, pkgtype.Email("a@example.com"))
	expect(t, pkgtype.Email("nobody"), "format")
	// 名前付きデコレータを通した制約
	expect(t, app.Count(1))
	expect(t, app.Count(0), "min")
}

func TestScalarBase(t *testing.T) {
	// 元の scalar の制約だけ
	expect(t, app.Tiny("ok"))
	expect(t, app.Tiny("toolong"), "length")
	// 元の scalar が先、次に自分の制約
	expect(t, app.TinyCode("abc"))
	expect(t, app.TinyCode("a"), "length")
	expect(t, app.TinyCode("toolong"), "length")
	// 別の名前空間の scalar
	expect(t, app.WorkEmail("a@example.com"))
	expect(t, app.WorkEmail("x"), "format")
	expect(t, app.BossEmail("x"), "format")
	expect(t, app.ShortSlug("abc"))
	expect(t, app.ShortSlug("ABC"), "pattern")
	expect(t, app.ShortSlug("abcdefghij"), "length")
	expect(t, app.ShortSlug("ABCDEFGHIJ"), "pattern", "length")
	// pattern の変数の名前が当たっても、それぞれの正規表現で確かめる
	expect(t, app.ItemColor("red"))
	expect(t, app.ItemColor("#00ff00"), "pattern")
}

func validItem() app.Item {
	return app.Item{
		Sku: "AB-1", Name: "pen", Price: 10, Count: 1, Color: "#00ff00", Image: "https://example.com/pen.png",
		Code: "abc", Seller: "s@example.com", From: pkgtype.Address{City: "Tokyo"},
	}
}

func TestModel(t *testing.T) {
	expect(t, validItem())
	// すべての違反を、フィールドの順に集める。型の Validate（1）が、フィールドの制約（2）より先
	expect(t, app.Item{Price: -1, Count: 11, Code: "toolong"},
		"sku: pattern", "name: length", "price: min", "count: max", "color: pattern", "image: format",
		"code: length", "seller: format", "from.city: length")
	bad := validItem()
	bad.Price, bad.Code = 10000, "ab"
	expect(t, bad, "price: max", "code: length")
	// 継承した model は、受け継いだ制約と、足したフィールドの制約を確かめる
	expect(t, app.PublicItem{Sku: "AB-1", Name: "pen", Color: "#00ff00", Image: "https://example.com/", Code: "abc",
		Seller: "s@example.com", From: pkgtype.Address{City: "Tokyo"}, Stock: 0}, "stock: min")
	// 既定値を入れた値は、制約に合う
	expect(t, app.NewPaging())
	expect(t, app.Paging{}, "page: min", "size: min", "sort: length", "link: format")
	// その場で書いた model
	expect(t, app.RejectedValue{Code: 200}, "code: min")
	expect(t, app.Drawing{Layer: 0}, "layer: min")
}

func TestOptional(t *testing.T) {
	// 値がなければ、確かめない
	expect(t, app.Optionals{})
	item := validItem()
	expect(t, app.Optionals{Link: ptr(app.Link("https://example.com/")), Limit: ptr(int64(1)), Nick: ptr("ab"), Item: &item,
		Code: ptr(app.Code("abc")), Address: &pkgtype.Address{City: "Kyoto"}})
	expect(t, app.Optionals{Link: ptr(app.Link("x")), Limit: ptr(int64(0)), Nick: ptr("a"), Item: &app.Item{},
		Code: ptr(app.Code("toolong")), Address: &pkgtype.Address{}},
		"link: format", "limit: min", "nick: length",
		"item.sku: pattern", "item.name: length", "item.color: pattern", "item.image: format", "item.code: length", "item.seller: format", "item.from.city: length",
		"code: length", "address.city: length")
	// 型の Validate と、フィールドの制約の両方
	expect(t, app.Optionals{Code: ptr(app.Code("ab"))}, "code: length")
}

func TestArray(t *testing.T) {
	expect(t, app.Lists{})
	item := validItem()
	expect(t, app.Lists{
		Items: []app.Item{item, {Sku: "AB-1", Name: "", Color: "#00ff00", Image: "https://example.com/", Code: "abc", Seller: "s@example.com", From: pkgtype.Address{City: "x"}}},
		Grid:  [][]app.Percent{{1, 0}, {}, {101}},
		Maybe: []*app.Item{nil, &item, {Name: "x", Color: "#00ff00", Image: "https://example.com/", Code: "abc", Seller: "s@example.com", From: pkgtype.Address{City: "x"}}},
		Later: []app.Uid{"123e4567-e89b-12d3-a456-426614174000", "no"},
		Names: []string{""},
	}, "items[1].name: length", "grid[0][1]: min", "grid[2][0]: max", "maybe[2].sku: pattern", "later[1]: format")
	// 互いを参照する model: 入れ子の Path
	expect(t, app.Tree{Children: []app.Tree{{}, {Leaf: &app.Leaf{Weight: -1, Parent: &app.Tree{Leaf: &app.Leaf{Weight: -2}}}}}},
		"children[1].leaf.weight: min", "children[1].leaf.parent.leaf.weight: min")
}

func TestRecord(t *testing.T) {
	expect(t, app.Maps{})
	item := validItem()
	m := app.Maps{
		ByName: map[string]app.Item{"z": {}, "a": item, "m": {Sku: "AB-1", Color: "#00ff00", Image: "https://example.com/", Code: "abc", Seller: "s@example.com", From: pkgtype.Address{City: "x"}}},
		BySku:  map[app.Sku]app.Percent{"ZZ-1": 0, "AA-1": 101, "MM-1": 50},
		Groups: map[string][]app.Item{"g": {item, {Sku: "AB-1", Name: "n", Color: "#00ff00", Image: "https://example.com/", Code: "abc", Seller: "x", From: pkgtype.Address{City: "x"}}}},
		Pages:  []map[string]app.Ratio{{"b": 11, "a": 0}, nil},
		Opt:    map[string]*app.Link{"none": nil, "bad": ptr(app.Link("x")), "ok": ptr(app.Link("https://example.com/"))},
		Labels: map[string]string{"": ""},
		// キーは確かめない（Sku の形でないキーがあっても、違反にならない）
		Counts: map[app.Sku]int64{"not a sku": 1},
	}
	want := []string{
		// キーの辞書順
		"byName[m].name: length",
		"byName[z].sku: pattern", "byName[z].name: length", "byName[z].color: pattern", "byName[z].image: format", "byName[z].code: length", "byName[z].seller: format", "byName[z].from.city: length",
		"bySku[AA-1]: max", "bySku[ZZ-1]: min",
		"groups[g][1].seller: format",
		"pages[0][a]: min", "pages[0][b]: max",
		"opt[bad]: format",
	}
	// マップの走査の順によらず、いつも同じ順
	for i := 0; i < 20; i++ {
		expect(t, m, want...)
	}
}

func TestNoValidate(t *testing.T) {
	for _, v := range []any{app.Plain(""), app.PlainCount(0), app.Free{}, app.Bare{}, app.Rejected{}, app.ShapeLabel{}, pkgtype.Address{}.City} {
		if _, has := reflect.TypeOf(v).MethodByName("Validate"); has {
			t.Errorf("%T が Validate を持つ", v)
		}
	}
}

// 内部の op を持つ名前空間: メソッドは内側のパッケージの型に付き、入口の別名から、そのまま呼べる。
func TestEntryAlias(t *testing.T) {
	expect(t, split.Input{Name: "x"})
	expect(t, split.Input{Contact: ptr(pkgtype.Email("x"))}, "name: length", "contact: format")
}
