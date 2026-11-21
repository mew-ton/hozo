package golang

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// docs/plugin-go.md「名前」の「名前が当たれば GO-2」が並べる、定義のパッケージの公開の名前を、種類ごとに一つずつ。
// 並びは、宣言順（宣言ごとの名前）、状態の単位、Ops、New、Leaves、Seams、別の名前空間の口。
func TestPublicNames(t *testing.T) {
	// model から出す名前は、構造体の型だけ（値を作る関数 New<Model> は出さない。docs/plugin-go.md「model」）
	page := model("shop.Page", field("size", ir.Int()))
	synthesized := model("shop.leaf$in")
	synthesized.SynthesizedFrom = "shop.leaf"
	arm := model("shop.Shape.Circle") // その場で書いたアームの model（op からの合成ではない）
	arm.SynthesizedFrom = "shop.Shape"
	set := union("shop.Errors")
	set.Variant = ir.UnionSet
	// 分類の定数は、宣言したケースの数だけ（docs/plugin-go.md「分類」。<C>Default はない）
	kind := classify("shop.Kind", ir.TypeSubject{Type: ir.String()}, ir.DecidedByPredicate, "Big", "Small")
	sync := classify("shop.Sync", ir.StatesSubject{States: []string{"shop.items"}}, ir.DecidedByPredicate, "UpToDate", "OutOfDate")
	v := viewOf("shop",
		namespace("shop",
			model("shop.Item"), page, synthesized, arm,
			scalar("shop.Sku", ir.String()),
			exception("shop.Missing"),
			union("shop.Shape", ir.UnionArm{Name: "Circle", Type: ir.Ref("shop.Shape.Circle")}, ir.UnionArm{Name: "text", Type: ir.String()},
				ir.UnionArm{Name: "Far", Type: ir.Ref("other.Far")}),
			set,
			effect("shop.addItem", nil),
			kind, sync,
			classify("shop.Present", ir.TypeSubject{Type: ir.Optional(ir.String())}, ir.DecidedByType, "Some"),
			state("shop.items", ir.Array(ir.Ref("shop.Item"))), state("shop.token", ir.Optional(ir.String())),
			op("shop.leaf", nil),
			with(op("shop.both", nil), decorated("x.rd"), carryingReads("shop.items", "shop.token")), // 二つの状態を読む葉も Leaves（SharedState はない）
			read("shop.itemBySku", "shop.items", ir.Optional(ir.Ref("shop.Item"))),                   // read は、状態の単位のメソッド。公開の名前を足さない
			with(op("shop.wire", nil), wired(call("other.run")), decorated("rate.limited")),
		),
		namespace("other", model("other.Far"), with(op("other.run", nil), decorated("t.mark"))),
	)
	v.Config.Seams = map[string]string{"rate.limited": "go-ratelimit"}
	type row struct {
		name         string
		kind         PublicKind
		decl, detail string
		what         string
	}
	want := []row{
		{"Item", PublicType, "shop.Item", "", "model shop.Item"},
		{"Page", PublicType, "shop.Page", "", "model shop.Page"},
		{"ShapeCircle", PublicType, "shop.Shape.Circle", "", "model shop.Shape.Circle"},
		{"Sku", PublicType, "shop.Sku", "", "scalar shop.Sku"},
		{"Missing", PublicType, "shop.Missing", "", "exception shop.Missing"},
		{"Shape", PublicType, "shop.Shape", "", "union shop.Shape"},
		{"ShapeText", PublicArm, "shop.Shape", "text", "union shop.Shape のアーム text を包む型"},
		{"ShapeFar", PublicArm, "shop.Shape", "Far", "union shop.Shape のアーム Far を包む型"},
		{"EffectAddItem", PublicEffect, "shop.addItem", "", "effect shop.addItem"},
		{"Kind", PublicClassify, "shop.Kind", "", "classify shop.Kind"},
		{"KindBig", PublicCase, "shop.Kind", "Big", "classify shop.Kind のケースの定数 KindBig"},
		{"KindSmall", PublicCase, "shop.Kind", "Small", "classify shop.Kind のケースの定数 KindSmall"},
		{"ClassifyKind", PublicClassifyFunc, "shop.Kind", "", "classify shop.Kind の判定関数"},
		{"KindCases", PublicCases, "shop.Kind", "", "classify shop.Kind の述語のインターフェース"},
		{"Sync", PublicClassify, "shop.Sync", "", "classify shop.Sync"},
		{"SyncUpToDate", PublicCase, "shop.Sync", "UpToDate", "classify shop.Sync のケースの定数 SyncUpToDate"},
		{"SyncOutOfDate", PublicCase, "shop.Sync", "OutOfDate", "classify shop.Sync のケースの定数 SyncOutOfDate"},
		{"ClassifySync", PublicClassifyFunc, "shop.Sync", "", "classify shop.Sync の判定関数"}, // 状態の分類は <C>Cases を持たない（述語は状態の単位）
		{"SyncState", PublicStateUnit, "shop.Sync", "", "状態の単位 shop.Sync"},                // 状態の単位は宣言順（分類 Sync が、状態より先に宣言されている）
		{"ItemsState", PublicStateUnit, "shop.items", "", "状態の単位 shop.items"},
		{"TokenState", PublicStateUnit, "shop.token", "", "状態の単位 shop.token"},
		{"Ops", PublicOps, "", "", "インターフェース Ops"},
		{"New", PublicNew, "", "", "関数 New"},
		{"Leaves", PublicLeaves, "", "", "インターフェース Leaves"},
		{"Seams", PublicSeams, "", "", "インターフェース Seams"},
		{"OtherOps", PublicDep, "", "other", "別の名前空間 other の口"},
	}
	got := PublicNames(v)
	if len(got) != len(want) {
		var names []string
		for _, n := range got {
			names = append(names, n.Name)
		}
		t.Fatalf("PublicNames = %d 個（%s）, want %d 個", len(got), strings.Join(names, " "), len(want))
	}
	for i, w := range want {
		if g := got[i]; g.Name != w.name || g.Kind != w.kind || g.Decl != w.decl || g.Detail != w.detail || g.What != w.what {
			t.Errorf("%d: %+v, want %+v", i, g, w)
		}
	}
	// 型だけの名前空間（op を持たない）: Ops も New もない
	for _, n := range PublicNames(viewOf("type", namespace("type", model("type.User")))) {
		if n.Kind != PublicType {
			t.Errorf("型だけの名前空間に %+v", n)
		}
	}
}

// docs/plugin-go.md「単体テストの口」の「テストのパッケージ」: 基底が、テストのパッケージに出す公開の名前。並びは、まとめの
// LeafCases と RunLeaves、葉ごとの <Op>Cases と <Op>Case、判定の口を出す分類ごとの <C>Examples と Run<C>。
func TestPublicTestNames(t *testing.T) {
	kind := classify("shop.Kind", ir.TypeSubject{Type: ir.String()}, ir.DecidedByPredicate, "Big", "Small")
	sync := classify("shop.Sync", ir.StatesSubject{States: []string{"shop.items"}}, ir.DecidedByPredicate, "UpToDate", "OutOfDate")
	v := viewOf("shop",
		namespace("shop",
			kind, sync,
			classify("shop.Present", ir.TypeSubject{Type: ir.Optional(ir.String())}, ir.DecidedByType, "Some"),
			state("shop.items", ir.Array(ir.String())),
			op("shop.leaf", nil),
			with(op("shop.wire", nil), wired(call("shop.leaf"))),
		),
	)
	want := []TestName{
		{"LeafCases", "", "葉のケースのまとめ LeafCases"},
		{"RunLeaves", "", "葉のケースを走らせる RunLeaves"},
		{"LeafCases", "shop.leaf", "op shop.leaf のケースの口 LeafCases"}, // 当たる名前も、そのまま二つ返す
		{"LeafCase", "shop.leaf", "op shop.leaf のケース LeafCase"},
		{"KindExamples", "shop.Kind", "分類 shop.Kind の例の口 KindExamples"},
		{"RunKind", "shop.Kind", "分類 shop.Kind の例を走らせる RunKind"},
		// 状態の分類（Sync）と、型で決まる分類（Present）と、配線の op（wire）は、口を持たない
	}
	got := PublicTestNames(v)
	if len(got) != len(want) {
		t.Fatalf("PublicTestNames = %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("%d: %+v, want %+v", i, got[i], w)
		}
	}
	// 口が一つもない出力（基底が、テストのパッケージを作らない出力）では、空
	if got := PublicTestNames(viewOf("shop", namespace("shop", with(op("shop.wire", nil), wired(call("other.run")))),
		namespace("other", op("other.run", nil)))); len(got) != 0 {
		t.Errorf("口のない出力に %+v", got)
	}
}
