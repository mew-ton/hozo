package golang

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// docs/plugin-go.md「名前」の「名前が当たれば GO-2」が並べる、定義のパッケージの公開の名前を、種類ごとに一つずつ。
// 並びは、宣言順（宣言ごとの名前）、状態の単位、Ops、New、Leaves、SharedState、Seams、別の名前空間の口。
func TestPublicNames(t *testing.T) {
	withDefault := model("shop.Page", field("size", ir.Int()))
	withDefault.Fields[0].Default = ir.IntLiteral{Value: 10}
	synthesized := model("shop.leaf$in")
	synthesized.SynthesizedFrom = "shop.leaf"
	arm := model("shop.Shape.Circle") // その場で書いたアームの model（op からの合成ではない）
	arm.SynthesizedFrom = "shop.Shape"
	set := union("shop.Errors")
	set.Variant = ir.UnionSet
	kind := classify("shop.Kind", ir.TypeSubject{Type: ir.String()}, ir.DecidedByPredicate, "Big")
	kind.HasDefault = true
	sync := classify("shop.Sync", ir.StatesSubject{States: []string{"shop.items"}}, ir.DecidedByPredicate, "UpToDate")
	sync.HasDefault = false
	v := viewOf("shop",
		namespace("shop",
			model("shop.Item"), withDefault, synthesized, arm,
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
			with(op("shop.both", nil), reading("shop.items", "shop.token")),
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
		{"NewPage", PublicNewModel, "shop.Page", "", "model shop.Page の NewPage"},
		{"ShapeCircle", PublicType, "shop.Shape.Circle", "", "model shop.Shape.Circle"},
		{"Sku", PublicType, "shop.Sku", "", "scalar shop.Sku"},
		{"Missing", PublicType, "shop.Missing", "", "exception shop.Missing"},
		{"Shape", PublicType, "shop.Shape", "", "union shop.Shape"},
		{"ShapeText", PublicArm, "shop.Shape", "text", "union shop.Shape のアーム text を包む型"},
		{"ShapeFar", PublicArm, "shop.Shape", "Far", "union shop.Shape のアーム Far を包む型"},
		{"EffectAddItem", PublicEffect, "shop.addItem", "", "effect shop.addItem"},
		{"Kind", PublicClassify, "shop.Kind", "", "classify shop.Kind"},
		{"KindBig", PublicCase, "shop.Kind", "Big", "classify shop.Kind のケースの定数 KindBig"},
		{"KindDefault", PublicCase, "shop.Kind", "default", "classify shop.Kind のケースの定数 KindDefault"},
		{"ClassifyKind", PublicClassifyFunc, "shop.Kind", "", "classify shop.Kind の判定関数"},
		{"KindCases", PublicCases, "shop.Kind", "", "classify shop.Kind の述語のインターフェース"},
		{"Sync", PublicClassify, "shop.Sync", "", "classify shop.Sync"},
		{"SyncUpToDate", PublicCase, "shop.Sync", "UpToDate", "classify shop.Sync のケースの定数 SyncUpToDate"},
		{"ClassifySync", PublicClassifyFunc, "shop.Sync", "", "classify shop.Sync の判定関数"}, // 状態の分類は <C>Cases を持たない（述語は状態の単位）
		{"SyncState", PublicStateUnit, "shop.Sync", "", "状態の単位 shop.Sync"},                // 状態の単位は宣言順（分類 Sync が、状態より先に宣言されている）
		{"ItemsState", PublicStateUnit, "shop.items", "", "状態の単位 shop.items"},
		{"TokenState", PublicStateUnit, "shop.token", "", "状態の単位 shop.token"},
		{"Ops", PublicOps, "", "", "インターフェース Ops"},
		{"New", PublicNew, "", "", "関数 New"},
		{"Leaves", PublicLeaves, "", "", "インターフェース Leaves"},
		{"SharedState", PublicShared, "", "", "インターフェース SharedState"},
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
