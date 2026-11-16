package golang

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

func placesOf(s *Shape) map[string]Place {
	out := map[string]Place{}
	for _, o := range s.Ops {
		out[o.FQN] = o.Place
	}
	return out
}

func opShape(s *Shape, fqn string) OpShape {
	for _, o := range s.Ops {
		if o.FQN == fqn {
			return o
		}
	}
	return OpShape{}
}

// docs/plugin-go.md「op の置き場」の表の行に一つずつ（上から順に、最初に当たる行）と、「効果の宛先の単位」。
func TestShapePlaces(t *testing.T) {
	tr := func(of string) ir.EffectTarget {
		return ir.TransitionTarget{Of: of, From: ir.AnyCase(), To: ir.Cases("A")}
	}
	v := viewOf("shop", namespace("shop",
		state("shop.items", ir.Array(ir.String())),
		state("shop.token", ir.Optional(ir.String())),
		state("shop.title", ir.String()),
		classify("shop.Sync", ir.StatesSubject{States: []string{"shop.items", "shop.token"}}, ir.DecidedByPredicate, "UpToDate"),
		classify("shop.Logged", ir.StatesSubject{States: []string{"shop.token"}}, ir.DecidedByType, "In"),
		effect("shop.addItem", ir.BuiltinTarget{State: "shop.items", Operation: ir.OpPost}),
		effect("shop.clear", tr("shop.items")),
		effect("shop.sync", tr("shop.Sync")),
		effect("shop.login", tr("shop.Logged")),
		exception("shop.Gone"),
		// 1: 単位の対象の配線
		with(op("shop.wire", nil), wired(call("shop.leaf"), call("shop.outside"), call("shop.outsideWire"))),
		// 2: never を返す op
		with(op("shop.gone", ir.Never()), throwing("shop.Gone")),
		// 3: 効果を持つ葉 → 効果の宛先の単位の命令
		with(op("shop.add", nil), effecting("shop.addItem")),                         // 組み込みの効果 → その基底の状態
		with(op("shop.empty", nil), effecting("shop.clear")),                         // 基底の状態の遷移 → その基底の状態
		with(op("shop.write", nil), effecting("shop.sync")),                          // 述語で決まる状態の分類の遷移 → その分類の単位
		with(op("shop.signIn", nil), effecting("shop.login")),                        // 型で決まる状態の分類の遷移 → ただ一つの対象の状態
		with(op("shop.both", nil), effecting("shop.addItem"), reading("shop.token")), // 効果と reads の両方 → 効果の宛先の単位
		// 4: 効果を持たず、reads が一つの葉 → その状態の単位の問い合わせ
		with(op("shop.find", ir.Optional(ir.String())), reading("shop.items")),
		// 5: 効果を持たず、reads が二つ以上の葉 → SharedState
		with(op("shop.report", ir.String()), reading("shop.items", "shop.title")),
		// 6: それ以外の葉（デコレータだけを持つ葉を含む）
		op("shop.leaf", nil),
		with(op("shop.marked", nil), decorated("x.mark")),
		// 単位の対象でない op: 単位の対象の配線が呼んでいるものだけを、本体のない op として出す
		with(op("shop.outside", nil), inFile("batch.hozo")),
		with(op("shop.outsideWire", nil), inFile("batch.hozo"), wired(call("shop.deep"))), // 配線でも、葉として扱う（6）
		with(op("shop.deep", nil), inFile("batch.hozo")),                                  // 単位の対象でない配線の先は辿らない
		with(op("shop.unused", nil), inFile("batch.hozo")),
	))
	v.Files = []string{"shop.hozo"}
	s := ShapeOf(v)
	want := map[string]Place{
		"shop.wire": Wiring, "shop.gone": Never,
		"shop.add": StateCommand, "shop.empty": StateCommand, "shop.write": StateCommand, "shop.signIn": StateCommand, "shop.both": StateCommand,
		"shop.find": StateQuery, "shop.report": SharedQuery,
		"shop.leaf": Leaf, "shop.marked": Leaf, "shop.outside": Leaf, "shop.outsideWire": Leaf,
	}
	if got := placesOf(s); !reflect.DeepEqual(got, want) {
		t.Errorf("places = %v\nwant %v", got, want)
	}
	for fqn, unit := range map[string]string{"shop.add": "shop.items", "shop.empty": "shop.items", "shop.write": "shop.Sync",
		"shop.signIn": "shop.token", "shop.both": "shop.items", "shop.find": "shop.items", "shop.wire": "", "shop.report": ""} {
		if got := opShape(s, fqn).Unit; got != unit {
			t.Errorf("%s の単位 = %q, want %q", fqn, got, unit)
		}
	}
	// Ops は宣言順
	var order []string
	for _, o := range s.Ops {
		order = append(order, strings.TrimPrefix(o.FQN, "shop."))
	}
	if got := strings.Join(order, ","); got != "wire,gone,add,empty,write,signIn,both,find,report,leaf,marked,outside,outsideWire" {
		t.Errorf("Ops の順 = %s", got)
	}
	if got := strings.Join(s.Leaves, ","); got != "shop.leaf,shop.marked,shop.outside,shop.outsideWire" {
		t.Errorf("Leaves = %s", got)
	}
	if got := strings.Join(s.Shared, ","); got != "shop.report" {
		t.Errorf("Shared = %s", got)
	}
	// 状態の単位: 基底の状態と、述語で決まる状態の分類を、宣言順に。型で決まる状態の分類は、単位を持たない
	wantUnits := []StateUnit{
		{FQN: "shop.items", TypeName: "ItemsState", Param: "items", Shape: "list", Queries: []string{"shop.find"}, Commands: []string{"shop.add", "shop.empty", "shop.both"}},
		{FQN: "shop.token", TypeName: "TokenState", Param: "token", Shape: "nullable", Commands: []string{"shop.signIn"}},
		{FQN: "shop.title", TypeName: "TitleState", Param: "title", Shape: "single"},
		{FQN: "shop.Sync", TypeName: "SyncState", Param: "sync", Commands: []string{"shop.write"}, Predicates: []string{"UpToDate"}},
	}
	if !reflect.DeepEqual(s.States, wantUnits) {
		t.Errorf("States = %+v\nwant %+v", s.States, wantUnits)
	}
	// New の引数: leaves、状態の単位（宣言順）、shared
	wantNew := []Param{{"leaves", "Leaves"}, {"items", "ItemsState"}, {"token", "TokenState"}, {"title", "TitleState"}, {"sync", "SyncState"}, {"shared", "SharedState"}}
	if !reflect.DeepEqual(s.New, wantNew) {
		t.Errorf("New = %+v\nwant %+v", s.New, wantNew)
	}
}

// docs/plugin-go.md「op の形」: 純粋でない op は ctx を取り error を返す。純粋で throws がある op は ctx を取らず error を返す。
// 純粋で throws がない op は、どちらもない。
func TestSignature(t *testing.T) {
	root := projectRoot(t)
	v := typesView()
	l, _ := LayoutOf(v, root)
	user, email := ir.Ref("server.User"), ir.Ref("server.Email")
	tests := []struct {
		name            string
		op              *ir.Op
		params, results string
		ctx, err        bool
	}{
		{"純粋でない、値を返す", with(op("server.find", ir.Optional(user), param("email", email)), impure()),
			"ctx context.Context, email Email", "(*User, error)", true, true},
		{"純粋でない、値を返さない", with(op("server.add", nil, param("user", user)), impure()),
			"ctx context.Context, user User", "error", true, true},
		{"純粋で throws がある", with(op("server.check", user, param("user", user)), throwing("server.Bad")),
			"user User", "(User, error)", false, true},
		{"純粋で throws があり、値を返さない", with(op("server.check2", nil), throwing("server.Bad")), "", "error", false, true},
		{"純粋で throws がない", op("server.page", ir.Array(user), param("users", ir.Array(user)), param("page", ir.Int())),
			"users []User, page int64", "[]User", false, false},
		{"戻り値がなく、純粋で、throws もない", op("server.noop", nil, param("x", ir.String())), "x string", "", false, false},
		{"never を返す op は、値を返さず error だけを返す", with(op("server.unauthorized", ir.Never()), throwing("server.Bad")), "", "error", false, true},
		{"never を返す op にデコレータが付いていれば ctx も取る", with(op("server.un2", ir.Never()), throwing("server.Bad"), impure()),
			"ctx context.Context", "error", true, true},
		{"別の名前空間の型と、局所の名前", op("server.x", ir.Ref("type.Session"), param("type", ir.Ref("type.Token")), param("ctx", ir.String()), param("o", ir.Int())),
			"type_ pkgtype.Token, ctx_ string, o_ int64", "pkgtype.Session", false, false},
	}
	reserved := map[string]bool{"o": true, "ctx": true, "err": true, "ptr": true}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			im := NewImports(v, l, root, true)
			params, results := Signature(tt.op, im, reserved)
			if params != tt.params || results != tt.results {
				t.Errorf("Signature = (%q, %q), want (%q, %q)", params, results, tt.params, tt.results)
			}
			if hasCtx := im.Name("context") != ""; hasCtx != tt.ctx {
				t.Errorf("context の import = %v, want %v", hasCtx, tt.ctx)
			}
			s := shapeOf(&ir.Program{Namespaces: []ir.Namespace{namespace("server", tt.op)}}, "server", func(*ir.Op) bool { return true }, nil)
			if s.Ops[0].Ctx != tt.ctx || s.Ops[0].Err != tt.err {
				t.Errorf("OpShape: Ctx = %v, Err = %v, want %v, %v", s.Ops[0].Ctx, s.Ops[0].Err, tt.ctx, tt.err)
			}
		})
	}
}

// docs/plugin-go.md「別の名前空間の op」「New」: 単位の対象の配線が呼ぶ別の名前空間ごとに口が一つ。メソッドは、呼ぶものだけを
// 相手の宣言順に。New の引数は、leaves、状態の単位、shared、分類の述語（FQN の辞書順）、別の名前空間（FQN の辞書順）、seams。
func TestShapeDepsAndNew(t *testing.T) {
	kind := classify("type.Kind", ir.TypeSubject{Type: ir.String()}, ir.DecidedByPredicate, "A")
	byType := classify("type.Present", ir.TypeSubject{Type: ir.Optional(ir.String())}, ir.DecidedByType, "Some")
	v := viewOf("server",
		namespace("server",
			state("server.type", ir.String()),   // New の引数の名前は type_（Go の予約語）
			state("server.Leaves", ir.String()), // leaves と当たる → leaves_
			classify("server.Own", ir.OpSubject{Op: "server.leaf", Type: ir.String()}, ir.DecidedByPredicate, "X"),
			classify("server.Unused", ir.TypeSubject{Type: ir.String()}, ir.DecidedByPredicate, "X"),
			with(op("server.leaf", ir.String()), internal()),
			with(op("server.run", nil), decorated("rate.limited"),
				wired(call("store.b"), call("shop.admin.ban"), call("store.a"), call("server.leaf"), call("store.b"),
					matchBy("type.Kind", armCall("mailer.send", "A")), matchBy("server.Own"), matchBy("type.Present"), matchBy("nullable"))),
			with(op("server.outside", nil), inFile("batch.hozo"), wired(call("other.x"), matchBy("server.Unused"))),
		),
		namespace("store", op("store.a", nil), op("store.unused", nil), op("store.b", nil)),
		namespace("shop.admin", op("shop.admin.ban", nil)),
		namespace("mailer", op("mailer.send", nil)),
		namespace("other", op("other.x", nil)),
		namespace("type", kind, byType),
	)
	v.Files = []string{"server.hozo"}
	v.Config.Seams = map[string]string{"rate.limited": "go-ratelimit"}
	s := ShapeOf(v)
	wantDeps := []Dep{
		{Namespace: "mailer", TypeName: "MailerOps", Param: "mailer", Ops: []string{"mailer.send"}},
		{Namespace: "shop.admin", TypeName: "ShopAdminOps", Param: "shopAdmin", Ops: []string{"shop.admin.ban"}},
		{Namespace: "store", TypeName: "StoreOps", Param: "store", Ops: []string{"store.a", "store.b"}}, // 相手の宣言順。呼ぶものだけ
	}
	if !reflect.DeepEqual(s.Deps, wantDeps) {
		t.Errorf("Deps = %+v\nwant %+v", s.Deps, wantDeps)
	}
	// 述語で決まる値の分類と op の分類のうち、単位の対象の配線が match に使うもの（別の名前空間のものを含む）。FQN の辞書順
	if got := strings.Join(s.Classify, ","); got != "server.Own,type.Kind" {
		t.Errorf("Classify = %s", got)
	}
	wantNew := []Param{{"leaves", "Leaves"}, {"type_", "TypeState"}, {"leaves_", "LeavesState"}, {"own", "OwnCases"}, {"kind", "KindCases"},
		{"mailer", "MailerOps"}, {"shopAdmin", "ShopAdminOps"}, {"store", "StoreOps"}, {"seams", "Seams"}}
	if !reflect.DeepEqual(s.New, wantNew) {
		t.Errorf("New = %+v\nwant %+v", s.New, wantNew)
	}
	if s.States[0].Param != "type_" || s.States[1].Param != "leaves_" {
		t.Errorf("States の Param = %q, %q", s.States[0].Param, s.States[1].Param)
	}
	// 引数が一つもない出力
	if s := ShapeOf(viewOf("type", namespace("type", model("type.User")))); len(s.New) != 0 || len(s.Ops) != 0 {
		t.Errorf("型だけの名前空間: %+v", s)
	}
	// 分けた出力では、引数の名前に inner を使わない（入口の New が inner.New を呼ぶ）
	v = viewOf("a", namespace("a", state("a.inner", ir.String()), with(op("a.f", nil), internal())))
	if s := ShapeOf(v); s.New[1].Name != "inner_" {
		t.Errorf("分けた出力の状態 inner の引数 = %+v", s.New)
	}
	v = viewOf("a", namespace("a", state("a.inner", ir.String()), op("a.f", nil)))
	if s := ShapeOf(v); s.New[1].Name != "inner" {
		t.Errorf("分けない出力の状態 inner の引数 = %+v", s.New)
	}
	// 引数の名前に ops を使わない（New の本体が、構造体 ops のリテラルを書く）。状態でも、別の名前空間でも
	v = viewOf("a", namespace("a", state("a.ops", ir.String()), op("a.f", nil)))
	if s := ShapeOf(v); s.New[1].Name != "ops_" || s.States[0].Param != "ops_" {
		t.Errorf("状態 ops の引数 = %+v", s.New)
	}
	v = viewOf("a",
		namespace("a", with(op("a.f", nil), wired(call("ops.run")))),
		namespace("ops", with(op("ops.run", nil), decorated("t.mark"))))
	if s := ShapeOf(v); len(s.New) != 1 || s.New[0].Name != "ops_" || s.Deps[0].Param != "ops_" {
		t.Errorf("別の名前空間 ops の引数 = %+v", s.New)
	}
}

// docs/plugin-go.md「継ぎ目」: 継ぎ目を持つ op は、N の単位の対象の op のうち、単位の継ぎ目にあるデコレータが付いたもの。
// Seams のメソッドは、op の宣言順、その中はデコレータの書かれた順。中身の名前を持つのは、配線と never を返す op だけ。
func TestShapeSeams(t *testing.T) {
	v := viewOf("server",
		namespace("server",
			// 配線に、二つの継ぎ目と、間の継ぎ目を持たないデコレータ
			with(op("server.foo", nil), decorated("trace.span", "x.mark", "rate.limited"), wired(call("server.leaf"), call("store.find"))),
			// 葉: 中身は口のメソッドのまま（名前を変えない）
			with(op("server.leaf", nil), decorated("rate.limited"), internal()),
			// never を返す op
			with(op("server.gone", ir.Never()), decorated("rate.limited"), throwing("server.Gone")),
			// 状態の単位に置かれる葉
			with(op("server.find", nil), decorated("trace.span"), reading("server.items")),
			state("server.items", ir.Array(ir.String())),
			// 単位の継ぎ目にないデコレータだけ
			with(op("server.plain", nil), decorated("x.mark")),
			// 単位の対象でない op（本体のない op として出すもの）は、継ぎ目を持たない
			with(op("server.outside", nil), decorated("rate.limited"), inFile("batch.hozo")),
			with(op("server.caller", nil), wired(call("server.outside"))),
			exception("server.Gone"),
		),
		// 別の名前空間の op のデコレータは、呼ぶ側には出さない
		namespace("store", with(op("store.find", nil), decorated("rate.limited"))),
	)
	v.Files = []string{"server.hozo", "store.hozo"}
	v.Config.Seams = map[string]string{"rate.limited": "go-ratelimit", "trace.span": "go-trace", "never.used": "go-x"}
	s := ShapeOf(v)
	want := []Seam{
		{"server.foo", "trace.span", "go-trace", "FooTraceSpan"},
		{"server.foo", "rate.limited", "go-ratelimit", "FooRateLimited"},
		{"server.leaf", "rate.limited", "go-ratelimit", "LeafRateLimited"},
		{"server.gone", "rate.limited", "go-ratelimit", "GoneRateLimited"},
		{"server.find", "trace.span", "go-trace", "FindTraceSpan"},
	}
	if !reflect.DeepEqual(s.Seams, want) {
		t.Errorf("Seams = %+v\nwant %+v", s.Seams, want)
	}
	for fqn, w := range map[string]struct {
		seams, body string
	}{
		"server.foo": {"trace.span,rate.limited", "fooBody"}, "server.leaf": {"rate.limited", ""}, "server.gone": {"rate.limited", "goneBody"},
		"server.find": {"trace.span", ""}, "server.plain": {"", ""}, "server.outside": {"", ""}, "server.caller": {"", ""},
	} {
		o := opShape(s, fqn)
		if strings.Join(o.Seams, ",") != w.seams || o.Body != w.body {
			t.Errorf("%s: Seams = %v, Body = %q, want %q, %q", fqn, o.Seams, o.Body, w.seams, w.body)
		}
	}
	// 継ぎ目は最後の引数
	if last := s.New[len(s.New)-1]; last != (Param{"seams", "Seams"}) {
		t.Errorf("New の最後 = %+v", last)
	}
	// 単位の継ぎ目にあるデコレータが、単位の対象のどの op にも付いていなければ、Seams も New の引数 seams もない
	v.Config.Seams = map[string]string{"never.used": "go-x"}
	if s = ShapeOf(v); len(s.Seams) != 0 || s.New[len(s.New)-1].Name == "seams" {
		t.Errorf("付いていないデコレータの継ぎ目: %+v", s.Seams)
	}
	// 継ぎ目の有無は、置き場と口（Leaves、状態の単位）を変えない
	withSeams := ShapeOf(func() *ir.View { v.Config.Seams = map[string]string{"rate.limited": "go-ratelimit"}; return v }())
	if !reflect.DeepEqual(placesOf(withSeams), placesOf(s)) || !reflect.DeepEqual(withSeams.Leaves, s.Leaves) || !reflect.DeepEqual(withSeams.States, s.States) {
		t.Error("継ぎ目の有無で、置き場か口が変わった")
	}
}

// docs/plugin-go.md「Seams」のメソッドの形: 引数は、op の形の引数の後ろに body func(<op の形の引数>) <op の形の戻り値>。
// body という名前が op の引数の局所の名前と当たれば、当たらなくなるまで、body の方の末尾に _ を足す。
func TestSeamSignature(t *testing.T) {
	root := projectRoot(t)
	v := typesView()
	l, _ := LayoutOf(v, root)
	im := NewImports(v, l, root, true)
	login := with(op("server.login", ir.Ref("server.User"), param("in", ir.Ref("server.Email"))), impure())
	params, results := SeamSignature(login, im, nil)
	if params != "ctx context.Context, in Email, body func(ctx context.Context, in Email) (User, error)" || results != "(User, error)" {
		t.Errorf("SeamSignature = (%q, %q)", params, results)
	}
	void := with(op("server.ping", nil, param("body", ir.String()), param("body_", ir.Int())), impure())
	params, results = SeamSignature(void, im, nil)
	if params != "ctx context.Context, body string, body_ int64, body__ func(ctx context.Context, body string, body_ int64) error" || results != "error" {
		t.Errorf("body と当たる引数: (%q, %q)", params, results)
	}
	if got := SeamBodyParam(void, nil); got != "body__" {
		t.Errorf("SeamBodyParam = %q", got)
	}
	if got := SeamBodyParam(login, nil); got != "body" {
		t.Errorf("SeamBodyParam = %q", got)
	}
}
