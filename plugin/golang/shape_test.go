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

// docs/plugin-go.md「op の置き場」の表の三つの行（上から順に、最初に当たる行）: 配線、never を返す op、Leaves。
// 葉に付いたデコレータが reads / effects を運んでいても（Op.carried）、置き場は Leaves のまま。
func TestShapePlaces(t *testing.T) {
	v := viewOf("shop", namespace("shop",
		state("shop.items", ir.Array(ir.String())),
		state("shop.token", ir.Optional(ir.String())),
		effect("shop.addItem", ir.BuiltinTarget{State: "shop.items", Operation: ir.OpPost}),
		read("shop.itemByName", "shop.items", ir.Optional(ir.String())),
		exception("shop.Gone"),
		// 1: 単位の対象の配線（read と効果を呼ぶものも、op を呼ぶものも）
		with(op("shop.wire", nil), wired(call("shop.leaf"), call("shop.outside"), call("shop.outsideWire"))),
		with(op("shop.touch", nil), wired(callRead("shop.itemByName"), callEffect("shop.addItem"), callDirect("shop.token", ir.OpDelete))),
		// 2: never を返す op
		with(op("shop.gone", ir.Never()), throwing("shop.Gone")),
		// 3: 葉。デコレータだけを持つ葉も、デコレータが reads / effects を運ぶ葉も
		op("shop.leaf", nil),
		with(op("shop.marked", nil), decorated("x.mark")),
		with(op("shop.carriedFx", nil), decorated("x.fx"), carryingEffects("shop.addItem")),
		with(op("shop.carriedRd", ir.Optional(ir.String())), decorated("x.rd"), carryingReads("shop.token")),
		with(op("shop.carriedBoth", nil), decorated("x.fx"), carryingEffects("shop.addItem"), carryingReads("shop.items", "shop.token")),
		// 3: 単位の対象でない op は、単位の対象の配線が呼んでいるものだけを、本体のない op として出す
		with(op("shop.outside", nil), inFile("batch.hozo")),
		with(op("shop.outsideWire", nil), inFile("batch.hozo"), wired(call("shop.deep"))), // 配線でも、葉として扱う
		with(op("shop.deep", nil), inFile("batch.hozo")),                                  // 単位の対象でない配線の先は辿らない
		with(op("shop.unused", nil), inFile("batch.hozo")),
	))
	v.Files = []string{"shop.hozo"}
	s := ShapeOf(v)
	want := map[string]Place{
		"shop.wire": Wiring, "shop.touch": Wiring, "shop.gone": Never,
		"shop.leaf": Leaf, "shop.marked": Leaf, "shop.carriedFx": Leaf, "shop.carriedRd": Leaf, "shop.carriedBoth": Leaf,
		"shop.outside": Leaf, "shop.outsideWire": Leaf,
	}
	if got := placesOf(s); !reflect.DeepEqual(got, want) {
		t.Errorf("places = %v\nwant %v", got, want)
	}
	// Ops は宣言順
	var order []string
	for _, o := range s.Ops {
		order = append(order, strings.TrimPrefix(o.FQN, "shop."))
	}
	if got := strings.Join(order, ","); got != "wire,touch,gone,leaf,marked,carriedFx,carriedRd,carriedBoth,outside,outsideWire" {
		t.Errorf("Ops の順 = %s", got)
	}
	// Leaves は、置き場が Leaf の op のすべて（宣言順）
	if got := strings.Join(s.Leaves, ","); got != "shop.leaf,shop.marked,shop.carriedFx,shop.carriedRd,shop.carriedBoth,shop.outside,shop.outsideWire" {
		t.Errorf("Leaves = %s", got)
	}
	// New の引数: leaves、状態の単位（宣言順）。複数の状態にまたがる口（SharedState）はない
	wantNew := []Param{{"leaves", "Leaves"}, {"items", "ItemsState"}, {"token", "TokenState"}}
	if !reflect.DeepEqual(s.New, wantNew) {
		t.Errorf("New = %+v\nwant %+v", s.New, wantNew)
	}
}

// docs/plugin-go.md「状態の単位」: N の read と効果は、配線が呼んでいるかによらず、すべて、読む状態の単位と、宛先の単位の
// メソッド（宣言順）。形で決まる命令は、単位の対象の配線が直に呼んでいる操作だけ（post、put、delete の順）。
// 「効果の宛先の単位」の四つの行。
func TestShapeStateUnits(t *testing.T) {
	tr := func(of string) ir.EffectTarget {
		return ir.TransitionTarget{Of: of, From: ir.AnyCase(), To: ir.Cases("A")}
	}
	v := viewOf("shop", namespace("shop",
		state("shop.items", ir.Array(ir.String())),
		state("shop.token", ir.Optional(ir.String())),
		state("shop.title", ir.String()),
		classify("shop.Sync", ir.StatesSubject{States: []string{"shop.items", "shop.token"}}, ir.DecidedByPredicate, "UpToDate"),
		classify("shop.Logged", ir.StatesSubject{States: []string{"shop.token"}}, ir.DecidedByType, "In"),
		// 効果: 組み込みの効果 → その基底の状態、基底の状態の遷移 → その基底の状態、述語で決まる状態の分類の遷移 → その分類の単位、
		// 型で決まる状態の分類の遷移 → ただ一つの対象の状態
		effect("shop.login", tr("shop.Logged")),
		effect("shop.addItem", ir.BuiltinTarget{State: "shop.items", Operation: ir.OpPost}, param("item", ir.String())),
		effect("shop.clear", tr("shop.items")),
		effect("shop.sync", tr("shop.Sync")),
		effect("shop.unused", ir.BuiltinTarget{State: "shop.token", Operation: ir.OpDelete}), // どの配線も呼ばない効果も出す
		// read: Read.state の単位（宣言順）。どの配線も呼ばない read も出す
		read("shop.second", "shop.items", ir.Array(ir.String())),
		read("shop.itemByName", "shop.items", ir.Optional(ir.String()), param("name", ir.String())),
		read("shop.heading", "shop.title", ir.Int()),
		// 直に呼ぶ組み込みの効果: 呼んだ順によらず post、put、delete の順。同じ操作を二度呼んでも一つ
		with(op("shop.wire", nil), wired(callDirect("shop.token", ir.OpDelete), callDirect("shop.token", ir.OpPost),
			callDirect("shop.token", ir.OpDelete), callDirect("shop.title", ir.OpPut), callRead("shop.itemByName"), callEffect("shop.addItem"),
			matchBy("nullable", armDirect("shop.token", ir.OpPut, ir.CaseSome)))),
		// 単位の対象でない配線が直に呼ぶものは、数えない
		with(op("shop.outside", nil), inFile("batch.hozo"), wired(callDirect("shop.items", ir.OpPost))),
	))
	v.Files = []string{"shop.hozo"}
	s := ShapeOf(v)
	// 状態の単位: 基底の状態と、述語で決まる状態の分類を、宣言順に。型で決まる状態の分類は、単位を持たない
	want := []StateUnit{
		{FQN: "shop.items", TypeName: "ItemsState", Param: "items", Shape: "list",
			Reads: []string{"shop.second", "shop.itemByName"}, Effects: []string{"shop.addItem", "shop.clear"}},
		{FQN: "shop.token", TypeName: "TokenState", Param: "token", Shape: "nullable", Builtins: []string{"post", "put", "delete"},
			Effects: []string{"shop.login", "shop.unused"}},
		{FQN: "shop.title", TypeName: "TitleState", Param: "title", Shape: "single", Builtins: []string{"put"}, Reads: []string{"shop.heading"}},
		{FQN: "shop.Sync", TypeName: "SyncState", Param: "sync", Effects: []string{"shop.sync"}, Predicates: []string{"UpToDate"}},
	}
	if !reflect.DeepEqual(s.States, want) {
		t.Errorf("States = %+v\nwant %+v", s.States, want)
	}
	// read と効果の呼び出しは、op の置き場にも、別の名前空間の口にも入らない
	if len(s.Ops) != 1 || s.Ops[0].Place != Wiring || len(s.Leaves) != 0 || len(s.Deps) != 0 {
		t.Errorf("Ops = %+v, Leaves = %v, Deps = %+v", s.Ops, s.Leaves, s.Deps)
	}
	// 単位の対象を変えても、read と効果は変わらない（変わるのは、形で決まる命令だけ）
	v.Files = []string{"batch.hozo"}
	other := ShapeOf(v)
	for i := range want {
		want[i].Builtins = nil
	}
	want[0].Builtins = []string{"post"}
	if !reflect.DeepEqual(other.States, want) {
		t.Errorf("単位の対象を変えた States = %+v\nwant %+v", other.States, want)
	}
}

// docs/plugin-go.md「状態の単位」のメソッドの形: read は (<結果の型>, error)、効果は error、形で決まる命令は value 一つか、なし。
// どれも ctx を取る。
func TestUnitSignatures(t *testing.T) {
	v := typesView()
	l, _ := LayoutOf(v)
	user, email := ir.Ref("server.User"), ir.Ref("server.Email")
	reserved := map[string]bool{"o": true, "ctx": true, "err": true, "ptr": true}
	sig := func(f func(im *Imports) (string, string)) string {
		im := NewImports(v, l, true)
		params, results := f(im)
		if im.Name("context") == "" {
			t.Errorf("(%s) %s: context を import していない", params, results)
		}
		return "(" + params + ") " + results
	}
	byEmail := read("server.userByEmail", "server.users", ir.Optional(user), param("email", email), param("ctx", ir.String()))
	if got := sig(func(im *Imports) (string, string) { return ReadSignature(byEmail, im, reserved) }); got != "(ctx context.Context, email Email, ctx_ string) (*User, error)" {
		t.Errorf("ReadSignature = %s", got)
	}
	count := read("server.count", "server.users", ir.Int())
	if got := sig(func(im *Imports) (string, string) { return ReadSignature(count, im, reserved) }); got != "(ctx context.Context) (int64, error)" {
		t.Errorf("引数のない read: %s", got)
	}
	create := effect("server.createUser", ir.BuiltinTarget{State: "server.users", Operation: ir.OpPost}, param("user", user), param("type", ir.Ref("type.Token")))
	if got := sig(func(im *Imports) (string, string) { return EffectSignature(create, im, reserved) }); got != "(ctx context.Context, user User, type_ pkgtype.Token) error" {
		t.Errorf("EffectSignature = %s", got)
	}
	clear := effect("server.clear", ir.BuiltinTarget{State: "server.users", Operation: ir.OpDelete})
	if got := sig(func(im *Imports) (string, string) { return EffectSignature(clear, im, reserved) }); got != "(ctx context.Context) error" {
		t.Errorf("引数のない効果: %s", got)
	}
	users, token := state("server.users", ir.Array(user)), state("server.token", ir.Optional(ir.Ref("type.Token")))
	for _, tt := range []struct {
		state     *ir.State
		operation string
		want      string
		value     bool
	}{
		{users, "post", "(ctx context.Context, value User) error", true},
		{token, "post", "(ctx context.Context, value pkgtype.Token) error", true}, // 要素の型（optional を外したもの）
		{token, "put", "(ctx context.Context, value pkgtype.Token) error", true},
		{token, "delete", "(ctx context.Context) error", false},
	} {
		if got := sig(func(im *Imports) (string, string) { return BuiltinSignature(tt.state, tt.operation, im) }); got != tt.want {
			t.Errorf("BuiltinSignature(%s, %s) = %s, want %s", tt.state.Name, tt.operation, got, tt.want)
		}
		if got := BuiltinTakesValue(tt.operation); got != tt.value {
			t.Errorf("BuiltinTakesValue(%s) = %v", tt.operation, got)
		}
	}
}

// docs/plugin-go.md「op の形」: どの op も、最初の引数に ctx を取り、最後の戻り値に error を返す。純粋かどうか
// （Op.signature.pure）でも、throws の有無でも、形は変わらない。
func TestSignature(t *testing.T) {
	v := typesView()
	l, _ := LayoutOf(v)
	user, email := ir.Ref("server.User"), ir.Ref("server.Email")
	tests := []struct {
		name            string
		op              *ir.Op
		params, results string
	}{
		{"純粋でない、値を返す", with(op("server.find", ir.Optional(user), param("email", email)), impure()),
			"ctx context.Context, email Email", "(*User, error)"},
		{"純粋でない、値を返さない", with(op("server.add", nil, param("user", user)), impure()),
			"ctx context.Context, user User", "error"},
		{"純粋で throws がある", with(op("server.check", user, param("user", user)), throwing("server.Bad")),
			"ctx context.Context, user User", "(User, error)"},
		{"純粋で throws があり、値を返さない", with(op("server.check2", nil), throwing("server.Bad")), "ctx context.Context", "error"},
		{"純粋で throws がない", op("server.page", ir.Array(user), param("users", ir.Array(user)), param("page", ir.Int())),
			"ctx context.Context, users []User, page int64", "([]User, error)"},
		{"戻り値がなく、純粋で、throws もない", op("server.noop", nil, param("x", ir.String())), "ctx context.Context, x string", "error"},
		{"never を返す op は、値を返さず error だけを返す", with(op("server.unauthorized", ir.Never()), throwing("server.Bad")),
			"ctx context.Context", "error"},
		{"never を返す op にデコレータが付いていても同じ", with(op("server.un2", ir.Never()), throwing("server.Bad"), impure()),
			"ctx context.Context", "error"},
		{"別の名前空間の型と、局所の名前", op("server.x", ir.Ref("type.Session"), param("type", ir.Ref("type.Token")), param("ctx", ir.String()), param("o", ir.Int())),
			"ctx context.Context, type_ pkgtype.Token, ctx_ string, o_ int64", "(pkgtype.Session, error)"},
	}
	reserved := map[string]bool{"o": true, "ctx": true, "err": true, "ptr": true}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			im := NewImports(v, l, true)
			params, results := Signature(tt.op, im, reserved)
			if params != tt.params || results != tt.results {
				t.Errorf("Signature = (%q, %q), want (%q, %q)", params, results, tt.params, tt.results)
			}
			if im.Name("context") == "" {
				t.Errorf("context を import していない")
			}
			// 継ぎ目のメソッドと body も、同じ形（同「Seams」）
			seamParams, seamResults := SeamSignature(tt.op, im, reserved)
			if want := tt.params + ", body func(" + tt.params + ") " + tt.results; seamParams != want || seamResults != tt.results {
				t.Errorf("SeamSignature = (%q, %q), want (%q, %q)", seamParams, seamResults, want, tt.results)
			}
		})
	}
}

// docs/plugin-go.md「op の形」: 形は op の宣言だけで決まる。純粋さ（Signature.pure）と、計算した例外（Signature.exceptions）を
// 変えても、メソッドのテキストは一バイトも変わらない。
func TestSignatureIgnoresPurity(t *testing.T) {
	v := typesView()
	l, _ := LayoutOf(v)
	user := ir.Ref("server.User")
	text := func(o *ir.Op) string {
		params, results := Signature(o, NewImports(v, l, true), nil)
		return "(" + params + ") " + results
	}
	for _, returns := range []ir.Type{nil, user, ir.Never()} {
		pure := text(op("server.f", returns, param("user", user)))
		for name, o := range map[string]*ir.Op{
			"純粋でない":        with(op("server.f", returns, param("user", user)), impure()),
			"throws がある":   with(op("server.f", returns, param("user", user)), throwing("server.Bad")),
			"デコレータが付いている":  with(op("server.f", returns, param("user", user)), decorated("rate.limited")),
			"純粋でなく、throws": with(op("server.f", returns, param("user", user)), impure(), throwing("server.Bad")),
		} {
			if got := text(o); got != pure {
				t.Errorf("%s（戻り値 %v）: %s、純粋で throws のない op は %s（同じでなければならない）", name, returns, got, pure)
			}
		}
	}
}

// docs/plugin-go.md「別の名前空間の op」「New」: 単位の対象の配線が呼ぶ別の名前空間ごとに口が一つ。メソッドは、呼ぶものだけを
// 相手の宣言順に。New の引数は、leaves、状態の単位、分類の述語（FQN の辞書順）、別の名前空間（FQN の辞書順）、seams。
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
			// デコレータが reads を運ぶ葉（置き場は Leaves のまま）
			with(op("server.find", nil), decorated("trace.span"), carryingReads("server.items")),
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
	v := typesView()
	l, _ := LayoutOf(v)
	im := NewImports(v, l, true)
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

// docs/plugin-go.md「効果の入口の確かめ」: 確かめる引数は、効果の引数のうち、型が、そのまま、述語で決まる値の分類か op の分類で
// 絞り込んだ型のもの。型で決まる分類、状態の分類、optional や array の中の絞り込みは、確かめない。
func TestCheckedParams(t *testing.T) {
	order := ir.Ref("app.Order")
	on := func(of ir.Type, classify string, cases ...string) ir.Type {
		return ir.CaseType{Classify: classify, Cases: cases, Of: of}
	}
	e := effect("app.move", ir.BuiltinTarget{State: "app.orders", Operation: ir.OpPut},
		param("order", on(order, "app.Stage", "Sent", "Done")),
		param("plain", order),
		param("byType", on(order, "app.Known", "Found")),
		param("item", on(ir.Ref("stock.Item"), "stock.Level", "Out")),
		param("found", on(ir.Optional(order), "app.Lookup", "Hit")),
		param("inList", ir.Array(on(order, "app.Stage", "Sent"))),
		param("inOptional", ir.Optional(on(order, "app.Stage", "Sent"))),
		param("byState", on(order, "app.Backlog", "Calm")))
	v := viewOf("app",
		namespace("app",
			model("app.Order"), state("app.orders", ir.Array(order)), e,
			effect("app.keep", ir.BuiltinTarget{State: "app.orders", Operation: ir.OpPost}, param("order", order)),
			classify("app.Stage", ir.TypeSubject{Type: order}, ir.DecidedByPredicate, "Draft", "Sent", "Done"),
			classify("app.Known", ir.TypeSubject{Type: ir.Optional(order)}, ir.DecidedByType, "Found", "Missing"),
			classify("app.Lookup", ir.OpSubject{Op: "app.find", Type: ir.Optional(order)}, ir.DecidedByPredicate, "Hit", "Miss"),
			classify("app.Backlog", ir.StatesSubject{States: []string{"app.orders"}}, ir.DecidedByPredicate, "Calm", "Rush"),
			op("app.find", ir.Optional(order)),
		),
		namespace("stock", model("stock.Item"),
			classify("stock.Level", ir.TypeSubject{Type: ir.Ref("stock.Item")}, ir.DecidedByPredicate, "Plenty", "Out")),
	)
	want := []CheckedParam{
		{Param: "order", Classify: "app.Stage", Cases: []string{"Sent", "Done"}},
		{Param: "item", Classify: "stock.Level", Cases: []string{"Out"}},
		{Param: "found", Classify: "app.Lookup", Cases: []string{"Hit"}},
	}
	if got := CheckedParams(v.Program, e); !reflect.DeepEqual(got, want) {
		t.Errorf("CheckedParams = %+v\nwant %+v", got, want)
	}
	if got := CheckedParams(v.Program, v.Program.Lookup("app.keep").(*ir.Effect)); len(got) != 0 {
		t.Errorf("絞り込んだ引数のない効果: %+v", got)
	}
	if c, ks, ok := PredicateNarrowing(v.Program, on(order, "app.Stage", "Draft")); !ok || c != "app.Stage" || !reflect.DeepEqual(ks, []string{"Draft"}) {
		t.Errorf("PredicateNarrowing = %q, %v, %v", c, ks, ok)
	}
	for name, typ := range map[string]ir.Type{"絞り込んでいない型": order, "型で決まる分類": on(order, "app.Known", "Found"),
		"状態の分類": on(order, "app.Backlog", "Calm"), "並びの中": ir.Array(on(order, "app.Stage", "Sent")), "型がない": nil} {
		if _, _, ok := PredicateNarrowing(v.Program, typ); ok {
			t.Errorf("%s: PredicateNarrowing が ok", name)
		}
	}
	if got := CheckedUnitName("app.orders"); got != "checkedOrdersState" {
		t.Errorf("CheckedUnitName = %q", got)
	}
	// 入口の確かめが使う分類の述語は、どの配線も match に使わなくても、New が受け取る（別の名前空間のものを含む。FQN の辞書順）
	s := ShapeOf(v)
	if got := strings.Join(s.Classify, ","); got != "app.Lookup,app.Stage,stock.Level" {
		t.Errorf("Classify = %s", got)
	}
	wantNew := []Param{{"leaves", "Leaves"}, {"orders", "OrdersState"}, {"backlog", "BacklogState"},
		{"lookup", "LookupCases"}, {"stage", "StageCases"}, {"level", "LevelCases"}}
	if !reflect.DeepEqual(s.New, wantNew) {
		t.Errorf("New = %+v\nwant %+v", s.New, wantNew)
	}
}
