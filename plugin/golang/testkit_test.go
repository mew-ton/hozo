package golang

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// kitView は、app が、store（葉を一つ持つ）と mailer の op を呼ぶ仕様の、app の単位 1 の View。store と mailer を出力する単位は、
// テストが足す。
func kitView() *ir.View {
	return viewOf("app",
		namespace("app",
			op("app.leaf", nil),
			with(op("app.wire", nil), wired(call("app.leaf"), call("store.find"), call("mailer.send"))),
		),
		namespace("store",
			op("store.pick", nil),
			with(op("store.find", nil), wired(call("store.pick"))),
		),
		namespace("mailer", op("mailer.send", nil)),
	)
}

// docs/plugin-go.md「テストのにせもの」の「別の名前空間」: 別の名前空間に渡すものは、それを出力する、同じ基底の単位の数で決まる。
func TestTestKitDeps(t *testing.T) {
	// 単位がない（外の仕組み）: どちらも、にせもの
	k, err := TestKitOf(kitView(), "")
	if err != nil || k.Blocked != "" || !k.HasFakes() {
		t.Fatalf("TestKitOf = %+v, %v", k, err)
	}
	var kinds []KitKind
	var fields []string
	for _, p := range k.Params {
		kinds = append(kinds, p.Kind)
		fields = append(fields, p.Field+":"+p.Fake)
	}
	if got := strings.Join(fields, " "); got != "Leaves:LeavesFake Mailer:MailerOpsFake Store:StoreOpsFake" {
		t.Errorf("単位のない別の名前空間: %s（%v）", got, kinds)
	}
	if len(k.Reals) != 1 || k.Reals[0] != (KitReal{Name: "leaves", Kind: KitLeaves, Namespace: "app"}) {
		t.Errorf("Reals = %+v", k.Reals)
	}

	// 同じ基底の単位が、ちょうど一つ: その出力の配線を、本物で通す。その出力の、人が書いたもの（葉）は、名前を付け直して受け取る
	v := kitView()
	v.Namespaces["store"] = []ir.NamespaceConfig{conf(2, "gen/store")}
	k, err = TestKitOf(v, "")
	if err != nil || k.Blocked != "" {
		t.Fatalf("TestKitOf = %+v, %v", k, err)
	}
	store := k.Params[2]
	if store.Kind != KitUnit || store.Field != "Store" || store.Fake != "" || store.Unit == nil || store.Unit.Dir != "gen/store" || !store.Unit.HasFakes() {
		t.Errorf("単位が一つの別の名前空間: %+v", store)
	}
	if len(k.Reals) != 2 || k.Reals[1] != (KitReal{Name: "storeLeaves", Kind: KitLeaves, Namespace: "store"}) {
		t.Errorf("Reals = %+v", k.Reals)
	}
	names := TestFakeNames(v)
	var got []string
	for _, n := range names {
		got = append(got, n.Name)
	}
	if s := strings.Join(got, " "); s != "LeavesFake MailerOpsFake Fakes NewOps" {
		t.Errorf("TestFakeNames = %s", s)
	}

	// 通す出力のある名前空間でも、呼ぶ op が、どれも、入口（@entry のデコレータを持つ op）なら、流れの端: 相手の入口の並びを、
	// にせものにする（「流れの端」の、相手の入口）。その出力の葉は、受け取らない
	v = kitView()
	v.Namespaces["store"] = []ir.NamespaceConfig{conf(2, "gen/store")}
	served := &ir.Decorator{DeclBase: base("store.served", "store.hozo"), Fn: true, Entry: true, Targets: []ir.DecoratorTarget{ir.TargetOp}}
	v.Program.Namespaces[1].Decls = append(v.Program.Namespaces[1].Decls, served)
	find := v.Program.Lookup("store.find").(*ir.Op)
	find.Decorators = append(find.Decorators, ir.Application{Decorator: "store.served"})
	k, err = TestKitOf(v, "")
	if err != nil || k.Blocked != "" {
		t.Fatalf("TestKitOf = %+v, %v", k, err)
	}
	if store = k.Params[2]; store.Kind != KitExternal || !store.Entry || store.Field != "Store" || store.Fake != "StoreOpsFake" || store.Unit != nil {
		t.Errorf("呼ぶ op が、どれも入口の名前空間: %+v", store)
	}
	if len(k.Reals) != 1 {
		t.Errorf("Reals = %+v", k.Reals)
	}
	// 入口でない op も呼ぶなら、本物で通す
	wire := v.Program.Lookup("app.wire").(*ir.Op)
	wire.Body.Statements = append(wire.Body.Statements, call("store.pick"))
	if k, _ = TestKitOf(v, ""); k.Params[2].Kind != KitUnit {
		t.Errorf("入口でない op も呼ぶ名前空間を、通していない: %+v", k.Params[2])
	}

	// 同じ基底の単位が、二つ: 通す出力を、選ばない。組み立てを作れず、fakes.go を持たない
	v = kitView()
	v.Namespaces["store"] = []ir.NamespaceConfig{conf(2, "gen/store"), conf(3, "gen/storemem")}
	k, err = TestKitOf(v, "")
	if err != nil || k.Blocked == "" || k.HasFakes() || len(TestFakeNames(v)) != 0 {
		t.Errorf("単位が二つの別の名前空間: Blocked = %q, HasFakes = %v, %v", k.Blocked, k.HasFakes(), err)
	}
	// 別の基底の単位は、数えない
	other := conf(3, "gen/storets")
	other.Base, other.Language = "ts", "ts"
	v.Namespaces["store"] = []ir.NamespaceConfig{conf(2, "gen/store"), other}
	if k, _ = TestKitOf(v, ""); k.Blocked != "" {
		t.Errorf("別の基底の単位を、数えた: %q", k.Blocked)
	}
	// 同じ単位が出力するものがあれば、それを通す（ほかに、同じ基底の単位があっても）
	v.Namespaces["store"] = []ir.NamespaceConfig{conf(2, "gen/store"), conf(1, "gen/app/store")}
	if k, _ = TestKitOf(v, ""); k.Blocked != "" || k.Params[2].Unit.Dir != "gen/app/store" {
		t.Errorf("同じ単位の出力を、通していない: %+v", k.Params[2])
	}
}

// 名前空間どうしが、輪になって呼び合えば、組み立てを作れない。op が一つもない出力と、New が引数を取らない出力は、fakes.go を持たない。
func TestTestKitBlockedAndEmpty(t *testing.T) {
	v := viewOf("a",
		namespace("a", with(op("a.run", nil), wired(call("b.run")))),
		namespace("b", with(op("b.run", nil), wired(call("a.run")))),
	)
	v.Namespaces["b"] = []ir.NamespaceConfig{conf(2, "gen/b")}
	if k, _ := TestKitOf(v, ""); k.Blocked == "" || k.HasFakes() {
		t.Errorf("輪になって呼び合う名前空間: Blocked = %q", k.Blocked)
	}
	// 状態だけの名前空間（op がない: New がない）
	if k, _ := TestKitOf(viewOf("s", namespace("s", state("s.items", ir.Array(ir.String())))), ""); k.HasFakes() {
		t.Errorf("op のない出力が、fakes.go を持つ")
	}
	// 引数を取らない New の出力を通す、別の名前空間は、Fakes にフィールドを持たない
	v = viewOf("a",
		namespace("a", with(op("a.run", nil), wired(call("b.run")))),
		namespace("b", with(op("b.run", nil), wired())),
	)
	v.Namespaces["b"] = []ir.NamespaceConfig{conf(2, "gen/b")}
	k, _ := TestKitOf(v, "")
	if !k.HasFakes() || k.Params[0].Kind != KitUnit || k.Params[0].Field != "" || k.Params[0].Unit.HasFakes() {
		t.Errorf("引数を取らない New の出力を通す: %+v", k.Params)
	}
}
