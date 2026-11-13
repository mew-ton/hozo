package golang

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// selectionView は docs/emit-go.md「出力の対象」の手順を一つずつ使う View。見る側は app で、別の名前空間 other
// （View.Namespaces にない。出力しない相手）の op を配線から呼ぶ。
//
//	root(in)      配線。@mark（どのプラグインも引き受けない印）。mid、other.fetch、stopHere、wrapped、addItem、leaf を呼ぶ
//	mid(in)       配線。値の分類 Kind で match し、k1 / k2 を呼ぶ。状態 items を match で読む
//	leaf(in)      葉。効果なし
//	addItem(in)   葉。効果 add（on items.post）
//	advance()     葉。効果 step（on Phase: A -> B）。root からは呼ばれないが、起点に選ぶ
//	stopHere()    配線。deeper を呼ぶ。select の stops
//	wrapped()     配線。@cache @audit。deeper2 を呼ぶ。select の wraps は @cache だけ
//	unselected(h) 葉。どこからも辿られない。その引数の型 Hidden もどこからも参照されない
func selectionView() *ir.View {
	in := ir.Ref("app.In")
	arm := func(op string, cases ...string) ir.MatchArm {
		return stateArm(callp(op, carg("in", name("in"))), cases...)
	}
	app := ir.Namespace{Name: "app", Decls: []ir.Decl{
		&ir.Model{DeclBase: base("app.In", nil), Fields: []ir.Field{field("x", ir.String())}},
		&ir.Model{DeclBase: base("app.Out", nil), Fields: []ir.Field{field("y", ir.Ref("app.Nested"))}},
		&ir.Model{DeclBase: base("app.Nested", nil)},
		&ir.Model{DeclBase: base("app.Hidden", nil)},
		&ir.Model{DeclBase: base("app.Spare", nil)},
		&ir.Model{DeclBase: base("app.Deep", nil)},
		&ir.State{DeclBase: base("app.items", nil), Shape: ir.StateList, Element: in},
		&ir.State{DeclBase: base("app.spare", nil), Shape: ir.StateSingle, Element: ir.Ref("app.Spare")},
		&ir.State{DeclBase: base("app.untouched", nil), Shape: ir.StateList, Element: ir.String()},
		&ir.Classify{DeclBase: base("app.Phase", nil), Subject: ir.StatesSubject{States: []string{"app.untouched"}},
			Cases: []ir.ClassifyCase{{Name: "A"}, {Name: "B"}}},
		&ir.Classify{DeclBase: base("app.Kind", nil), Subject: ir.TypeSubject{Type: in}, Cases: []ir.ClassifyCase{{Name: "K1"}, {Name: "K2"}}},
		&ir.Classify{DeclBase: base("app.UnusedKind", nil), Subject: ir.TypeSubject{Type: in}, Cases: []ir.ClassifyCase{{Name: "U"}}, HasDefault: true},
		&ir.Effect{DeclBase: base("app.add", nil), Target: ir.BuiltinTarget{State: "app.items", Operation: ir.OpPost}, Writes: []string{"app.items"}},
		&ir.Effect{DeclBase: base("app.step", nil), Target: ir.TransitionTarget{Of: "app.Phase", From: ir.Cases("A"), To: ir.Cases("B")}, Writes: []string{"app.untouched"}},
		&ir.Effect{DeclBase: base("app.unusedEffect", nil), Target: ir.BuiltinTarget{State: "app.untouched", Operation: ir.OpPost}, Writes: []string{"app.untouched"}},
		&ir.Decorator{DeclBase: base("app.mark", nil), Targets: []ir.DecoratorTarget{ir.TargetOp}},
		&ir.Decorator{DeclBase: base("app.cache", nil), Targets: []ir.DecoratorTarget{ir.TargetOp}},
		&ir.Decorator{DeclBase: base("app.audit", nil), Targets: []ir.DecoratorTarget{ir.TargetOp}},
		&ir.Op{DeclBase: base("app.root", nil, app("app.mark")), Params: []ir.OpParam{param("in", in)}, Returns: ir.Ref("app.Out"),
			Body: wiring(
				ir.ConstStmt{Name: "a", Expr: ir.CallExpr{Call: call("app.mid", carg("in", name("in")))}, Span: sp("x.hozo", 1)},
				ir.CallStmt{Call: call("other.fetch"), Span: sp("x.hozo", 1)},
				ir.CallStmt{Call: call("app.stopHere"), Span: sp("x.hozo", 1)},
				ir.CallStmt{Call: call("app.wrapped"), Span: sp("x.hozo", 1)},
				ir.CallStmt{Call: call("app.addItem", carg("in", name("a"))), Span: sp("x.hozo", 1)},
				ir.ReturnStmt{Expr: ir.CallExpr{Call: call("app.leaf", carg("in", name("a")))}, Span: sp("x.hozo", 1)},
			), Signature: effectsSig("app.add")},
		&ir.Op{DeclBase: base("app.mid", nil), Params: []ir.OpParam{param("in", in)}, Returns: in,
			Body: wiring(
				matchStmt(byState("app.items", ir.StateList, stateArm(nil, ir.CaseEmpty), stateArm(nil, ir.CaseHasItems))),
				ir.ReturnStmt{Expr: ir.MatchExpr{Match: ir.Match{Subject: ir.ValueSubject{Atom: name("in")}, Classify: "app.Kind",
					Arms: []ir.MatchArm{arm("app.k1", "K1"), arm("app.k2", "K2")}, Span: sp("x.hozo", 1)}}, Span: sp("x.hozo", 1)},
			)},
		&ir.Op{DeclBase: base("app.k1", nil), Params: []ir.OpParam{param("in", in)}, Returns: in},
		&ir.Op{DeclBase: base("app.k2", nil), Params: []ir.OpParam{param("in", in)}, Returns: in},
		&ir.Op{DeclBase: base("app.leaf", nil), Params: []ir.OpParam{param("in", in)}, Returns: ir.Ref("app.Out")},
		&ir.Op{DeclBase: base("app.addItem", nil), Params: []ir.OpParam{param("in", in)}, Effects: []string{"app.add"}, Signature: effectsSig("app.add")},
		&ir.Op{DeclBase: base("app.advance", nil), Effects: []string{"app.step"}, Signature: effectsSig("app.step")},
		&ir.Op{DeclBase: base("app.stopHere", nil), Body: wiring(ir.CallStmt{Call: call("app.deeper", carg("d", ir.NoneAtom{})), Span: sp("x.hozo", 1)})},
		&ir.Op{DeclBase: base("app.deeper", nil), Params: []ir.OpParam{param("d", ir.Optional(ir.Ref("app.Deep")))}},
		&ir.Op{DeclBase: base("app.wrapped", nil, app("app.cache"), app("app.audit")), Body: wiring(ir.CallStmt{Call: call("app.deeper2"), Span: sp("x.hozo", 1)})},
		&ir.Op{DeclBase: base("app.deeper2", nil)},
		&ir.Op{DeclBase: base("app.unselected", nil), Params: []ir.OpParam{param("h", ir.Ref("app.Hidden"))}},
	}}
	other := ir.Namespace{Name: "other", Decls: []ir.Decl{
		&ir.Op{DeclBase: base("other.fetch", nil), Body: wiring(ir.CallStmt{Call: call("other.inner"), Span: sp("x.hozo", 1)})},
		&ir.Op{DeclBase: base("other.inner", nil)},
	}}
	p := &ir.Program{Version: ir.Version, Namespaces: []ir.Namespace{{Name: ""}, validVocab(), app, other}}
	v := &ir.View{Viewer: "app", Config: nsConfig("app"), Namespaces: namespaces(), Program: p, Ops: map[string]*ir.ViewedSignature{}}
	for _, ns := range []ir.Namespace{app, other} {
		for _, d := range ns.Decls {
			if op, ok := d.(*ir.Op); ok {
				v.Ops[op.Name] = &ir.ViewedSignature{Signature: op.Signature, Params: op.Params}
			}
		}
	}
	return v
}

// TestSelection は docs/emit-go.md「出力の対象」の 1〜6 と「デコレータ」: 出力するのはプラグインが選んだ宣言だけ。
func TestSelection(t *testing.T) {
	root := newProjectRoot(t)
	v := selectionView()
	sel := plugin.Select{
		Roots:  []string{"app.root", "app.advance"},
		Stops:  []string{"app.stopHere", "app.deeper2"}, // deeper2 はどこからも辿られないので、stops に書いても選ばれない
		States: []string{"app.spare"},
		Wraps:  []plugin.Wrap{{Op: "app.wrapped", Decorator: "app.cache"}},
	}
	files, diags := Emit(v, Options{Root: root, Select: sel})
	if len(diags) > 0 {
		t.Fatalf("診断が出た: %s", formatDiags(diags))
	}
	ops, states := contentOf(files, "gen/app/ops.go"), contentOf(files, "gen/app/states.go")
	wiringGo, types, classify := contentOf(files, "gen/app/wiring.go"), contentOf(files, "gen/app/types.go"), contentOf(files, "gen/app/classify.go")

	t.Run("2. op を辿る: 選ばれた配線が呼ぶ op を選び、選ばれなかった op は出さない", func(t *testing.T) {
		wantContains(t, "ops.go", ops,
			[]string{"type RootFunc func(", "type MidFunc func(", "type K1Func func(", "type K2Func func(", "type LeafFunc func(", "type AddItemFunc func(", "type AdvanceFunc func(",
				"\tRoot(in In) (Out, error)\n", "\tMid(in In) (In, error)\n"},
			[]string{"Unselected", "Deeper"})
		// 本体を出すのは辿った配線だけ
		wantContains(t, "wiring.go", wiringGo,
			[]string{"func (o *ops) Root(in In) (Out, error) {", "func (o *ops) Mid(in In) (In, error) {"},
			[]string{"func (o *ops) StopHere(", "func (o *ops) Wrapped(", "Deeper"})
	})
	t.Run("2. stops の op は口だけを出し、本体は辿らない", func(t *testing.T) {
		// 配線でも葉と同じ扱い: Leaves のフィールドになり、呼び先の deeper は選ばれない
		wantContains(t, "ops.go", ops,
			[]string{"type StopHereFunc func() error", fieldRE("StopHere", "StopHereFunc"), "if o.leaves.StopHere == nil {", "\tStopHere() error\n"},
			[]string{"DeeperFunc", "Deeper2Func"})
		if strings.Contains(wiringGo, "func (o *ops) StopHere(") {
			t.Errorf("stops の op の本体が wiring.go にある:\n%s", wiringGo)
		}
		if !strings.Contains(wiringGo, "o.StopHere()") {
			t.Errorf("root の本体が stopHere を呼んでいない:\n%s", wiringGo)
		}
	})
	t.Run("2. wraps の op も本体は辿らず、引き受けたデコレータだけが包む関数になる", func(t *testing.T) {
		wantContains(t, "ops.go", ops,
			[]string{
				"type WrappedCacheWrapper func(next WrappedFunc) error",
				"type CacheWrappers struct {\n\tWrapped WrappedCacheWrapper\n}",
				"type Wrappers struct {\n\tCache CacheWrappers\n}",
				fieldRE("Wrapped", "WrappedFunc"), // 口は Leaves
				"return o.wrappers.Cache.Wrapped(f0)",
			},
			// @audit（wrapped）と @mark（root）は、どのプラグインも引き受けない印。何も生成しない
			[]string{"Audit", "Mark"})
	})
	t.Run("3. 状態: 葉の効果の宛先、配線が match で読む状態、states", func(t *testing.T) {
		wantContains(t, "states.go", states,
			[]string{
				// items: mid が match で読み、addItem の効果 add（on items.post）の宛先
				"type ItemsState struct {", "\tIsEmpty func() (bool, error)\n", fieldRE("AddItem", "AddItemFunc"),
				// spare: states で選んだ
				"type SpareState struct {", "\tGet func() (Spare, error)\n",
				// Phase: advance の効果 step（on Phase: A -> B）の宛先。その対象の基底の状態 untouched は選ばない
				"type PhaseState struct {", "\tA func() (bool, error)\n", fieldRE("Advance", "AdvanceFunc"),
				"type StateSet struct {\n\tItems ItemsState\n\tSpare SpareState\n\tPhase PhaseState\n}",
				"var States StateSet", "func ClassifyPhase() (Phase, error) {",
			},
			[]string{"Untouched"})
		// 効果を持つ葉は状態の単位の命令で、Leaves には載らない
		wantContains(t, "ops.go", ops,
			[]string{"if States.Items.AddItem == nil {", "return States.Items.AddItem(in)", "return States.Phase.Advance()"},
			[]string{fieldRE("AddItem", "AddItemFunc"), fieldRE("Advance", "AdvanceFunc")})
	})
	t.Run("4. 分類: 選ばれた配線の match が使う値の分類", func(t *testing.T) {
		wantContains(t, "classify.go", classify,
			[]string{"type Kind int", "type KindCases interface {", "func ClassifyKind(c KindCases, v In) (Kind, error) {", "type Phase int"},
			[]string{"UnusedKind", "PhaseCases"})
		wantContains(t, "ops.go", ops, []string{"type Classifiers struct {\n\tKind KindCases\n}"}, nil)
	})
	t.Run("5. 別の名前空間の op は Deps に載り、辿らない", func(t *testing.T) {
		wantContains(t, "ops.go", ops,
			[]string{"type Deps struct {\n\tOther OtherOps\n}", "type OtherOps struct {\n\tFetch OtherFetchFunc\n}", "type OtherFetchFunc func() error"},
			[]string{"Inner"})
		wantContains(t, "wiring.go", wiringGo, []string{"if err := o.deps.Other.Fetch(); err != nil {"}, nil)
	})
	t.Run("6. 型: 選ばれた op・状態・分類が参照する型と、それらが参照する型。選ばれた op の効果の定数", func(t *testing.T) {
		wantContains(t, "types.go", types,
			[]string{"type In struct {", "type Out struct {", "type Nested struct{}", "type Spare struct{}", "const EffectAdd ", "const EffectStep "},
			[]string{"Hidden", "Deep", "EffectUnusedEffect"})
	})
	t.Run("New の引数は要るものだけ", func(t *testing.T) {
		wantContains(t, "ops.go", ops, []string{"func New(leaves Leaves, deps Deps, wrappers Wrappers, classifiers Classifiers) Ops {"}, nil)
	})
	compileFiles(t, root, files)

	t.Run("Exports は viewer の型だけ", func(t *testing.T) {
		got := strings.Join(Exports(v, sel), ",")
		if want := "app.In,app.Out,app.Nested,app.Spare"; got != want {
			t.Errorf("Exports = %s, want %s", got, want)
		}
	})
	t.Run("起点を変えれば出力も変わる", func(t *testing.T) {
		// stopHere を起点にして stops に書かなければ、本体を出し、deeper まで辿る
		files, diags := Emit(v, Options{Root: root, Select: plugin.Select{Roots: []string{"app.stopHere"}}})
		if len(diags) > 0 {
			t.Fatalf("診断が出た: %s", formatDiags(diags))
		}
		wantContains(t, "wiring.go", contentOf(files, "gen/app/wiring.go"), []string{"func (o *ops) StopHere() error {", "return o.Deeper(nil)"}, nil)
		wantContains(t, "ops.go", contentOf(files, "gen/app/ops.go"),
			[]string{"type Leaves struct {\n\tDeeper DeeperFunc\n}", "func New(leaves Leaves) Ops {"},
			[]string{"Root", "Deps", "Wrappers", "Classifiers"})
		wantContains(t, "types.go", contentOf(files, "gen/app/types.go"), []string{"type Deep struct{}"}, []string{"type In struct"})
		if s := contentOf(files, "gen/app/states.go"); s != "" {
			t.Errorf("状態を選んでいないのに states.go が出た:\n%s", s)
		}
		compileFiles(t, newProjectRoot(t), files)
	})
	t.Run("葉がすべて状態の命令なら New は引数を取らない", func(t *testing.T) {
		files, diags := Emit(v, Options{Root: root, Select: plugin.Select{Roots: []string{"app.advance"}}})
		if len(diags) > 0 {
			t.Fatalf("診断が出た: %s", formatDiags(diags))
		}
		wantContains(t, "ops.go", contentOf(files, "gen/app/ops.go"), []string{"func New() Ops {\n\treturn &ops{}\n}", "type ops struct{}"}, []string{"Leaves"})
		compileFiles(t, newProjectRoot(t), files)
	})
}

// TestStateUnits は docs/emit-go.md「state」: 選ばれた状態は、配線が使わなくても（op が一つもなくても）単位を生成する。
// 問い合わせは形で決まり、並びは宣言の順。
func TestStateUnits(t *testing.T) {
	root := newProjectRoot(t)
	carts, token, title, checkout := stateDecls()
	t.Run("states only", func(t *testing.T) {
		files, diags := emitSmall(root, carts, token, title, checkout)
		if len(diags) != 0 {
			t.Fatalf("診断が出た: %s", formatDiags(diags))
		}
		if got := strings.Join(paths(files), ","); got != "gen/app/states.go,gen/app/classify.go" {
			t.Fatalf("states.go と classify.go だけを期待したが %s", got)
		}
		wantContains(t, "states.go", contentOf(files, "gen/app/states.go"), []string{
			"type CartsState struct {\n\t// 問い合わせ: 組み込みの分類。空（Empty）なら true。HasItems はその否定。\n\tIsEmpty func() (bool, error)\n}",
			"type TokenState struct {\n\t// 問い合わせ: 組み込みの分類。空（Empty）なら true。HasItems はその否定。\n\tIsEmpty func() (bool, error)\n\t// 問い合わせ: 状態の値。空なら nil。\n\tGet func() (*string, error)\n}",
			"type TitleState struct {\n\t// 問い合わせ: 状態の値。\n\tGet func() (string, error)\n}",
			"type CheckoutState struct {\n\t// 問い合わせ: ケース Shopping の述語。\n\tShopping func() (bool, error)\n\t// 問い合わせ: ケース Ordered の述語。\n\tOrdered func() (bool, error)\n}",
			"type StateSet struct {\n\tCarts    CartsState\n\tToken    TokenState\n\tTitle    TitleState\n\tCheckout CheckoutState\n}",
			"var States StateSet\n",
			"func ClassifyCheckout() (Checkout, error) {",
		}, []string{"context"})
		compileFiles(t, newProjectRoot(t), files)
	})
	t.Run("only the selected states", func(t *testing.T) {
		v := smallView(carts, token, title, checkout)
		files, diags := Emit(v, Options{Root: root, Select: plugin.Select{States: []string{"app.token"}}})
		if len(diags) != 0 {
			t.Fatalf("診断が出た: %s", formatDiags(diags))
		}
		wantContains(t, "states.go", contentOf(files, "gen/app/states.go"),
			[]string{"type StateSet struct {\n\tToken TokenState\n}"}, []string{"Carts", "Title", "Checkout"})
	})
	t.Run("match by nullable in wiring", func(t *testing.T) {
		show := &ir.Op{DeclBase: base("app.show", nil)}
		open := &ir.Op{DeclBase: base("app.open", nil), Body: wiring(matchStmt(byState("app.token", ir.StateNullable,
			stateArm(nil, ir.CaseEmpty), stateArm(callp("app.show"), ir.CaseHasItems))))}
		v := smallView(carts, token, show, open)
		files, diags := Emit(v, Options{Root: root, Select: plugin.Select{Roots: []string{"app.open"}}})
		if len(diags) != 0 {
			t.Fatalf("診断が出た: %s", formatDiags(diags))
		}
		// 状態は New の引数にならない（パッケージの変数 States を読む）
		wantContains(t, "ops.go", contentOf(files, "gen/app/ops.go"), []string{"func New(leaves Leaves) Ops {", "type ops struct {\n\tleaves Leaves\n}"}, []string{"context"})
		wantContains(t, "wiring.go", contentOf(files, "gen/app/wiring.go"), []string{
			"if States.Token.IsEmpty == nil {\n\t\treturn &runtime.MissingError{Kind: \"state\", Name: \"app.token\"}\n\t}",
			"empty, err := States.Token.IsEmpty()", "if !empty {",
		}, []string{"context", "ctx"})
		// carts はどこからも選ばれない
		wantContains(t, "states.go", contentOf(files, "gen/app/states.go"), []string{"type TokenState struct {"}, []string{"Carts"})
		compileFiles(t, newProjectRoot(t), files)
	})
	t.Run("argument named like the generated names", func(t *testing.T) {
		// docs/emit-go.md「名前」: 生成したコードが使う名前（o、err、ptr、States、runtime）と同じ引数名は末尾に `_`
		show := &ir.Op{DeclBase: base("app.show", nil), Params: []ir.OpParam{param("States", ir.String()), param("o", ir.String()), param("err", ir.String()), param("runtime", ir.String())}}
		open := &ir.Op{DeclBase: base("app.open", nil), Params: show.Params, Body: wiring(
			matchStmt(byState("app.token", ir.StateNullable,
				stateArm(nil, ir.CaseEmpty), stateArm(callp("app.show", carg("States", name("States")), carg("o", name("o")), carg("err", name("err")), carg("runtime", name("runtime"))), ir.CaseHasItems))))}
		v := smallView(token, show, open)
		files, diags := Emit(v, Options{Root: root, Select: plugin.Select{Roots: []string{"app.open"}}})
		if len(diags) != 0 {
			t.Fatalf("診断が出た: %s", formatDiags(diags))
		}
		wantContains(t, "wiring.go", contentOf(files, "gen/app/wiring.go"),
			[]string{"func (o *ops) Open(States_ string, o_ string, err_ string, runtime_ string) error {", "err = o.Show(States_, o_, err_, runtime_)"}, nil)
		compileFiles(t, newProjectRoot(t), files)
	})
}

// TestStateClassifyWithoutMatch は docs/emit-go.md「classify」の「配線で match by C に使われない分類でも生成する」と、
// 「effect」の遷移のコメント（組み込みの効果のコメントは今まで通り）。分類は葉の効果の宛先として選ばれる。
func TestStateClassifyWithoutMatch(t *testing.T) {
	root := newProjectRoot(t)
	carts, token, _, checkout := stateDecls()
	leaf := func(name, effect string) *ir.Op {
		return &ir.Op{DeclBase: base(name, nil), Effects: []string{effect}, Signature: effectsSig(effect)}
	}
	files, diags := emitSmall(root, carts, token, checkout,
		&ir.Effect{DeclBase: base("app.logout", nil), Target: ir.BuiltinTarget{State: "app.token", Operation: ir.OpDelete}, Writes: []string{"app.token"}},
		&ir.Effect{DeclBase: base("app.placeOrder", doc("注文を確定する")), Target: ir.TransitionTarget{Of: "app.Checkout", From: ir.Cases("Shopping"), To: ir.Cases("Ordered")}, Writes: []string{"app.carts"}},
		&ir.Effect{DeclBase: base("app.clearCart", nil), Target: ir.TransitionTarget{Of: "app.carts", From: ir.Cases(ir.CaseHasItems), To: ir.Cases(ir.CaseEmpty)}, Writes: []string{"app.carts"}},
		&ir.Effect{DeclBase: base("app.touch", nil), Target: ir.TransitionTarget{Of: "app.Checkout", From: ir.Cases("Shopping", "Ordered"), To: ir.AnyCase()}, Writes: []string{"app.carts"}},
		leaf("app.doLogout", "app.logout"), leaf("app.doPlace", "app.placeOrder"), leaf("app.doClear", "app.clearCart"), leaf("app.doTouch", "app.touch"),
	)
	if len(diags) != 0 {
		t.Fatalf("診断が出た: %s", formatDiags(diags))
	}
	for file, wants := range map[string][]string{
		"gen/app/classify.go": {"type Checkout int", "CheckoutShopping Checkout = iota"},
		"gen/app/states.go": {
			// 命令は op の宣言の順。効果の宛先が組み込みの効果でも遷移でも、その状態の単位に入る
			"\t// 命令: 効果 app.clearCart（on app.carts: HasItems -> Empty）を持つ葉の op app.doClear。\n\tDoClear DoClearFunc\n",
			"\t// 命令: 効果 app.logout（on app.token.delete）を持つ葉の op app.doLogout。\n\tDoLogout DoLogoutFunc\n",
			"\tDoPlace DoPlaceFunc\n\t// 命令: 効果 app.touch（on app.Checkout: Shopping | Ordered -> any）を持つ葉の op app.doTouch。\n\tDoTouch DoTouchFunc\n",
			"func ClassifyCheckout() (Checkout, error) {",
			`return 0, &runtime.ClassificationError{Classify: "app.Checkout", Hits: hits}`,
		},
		"gen/app/types.go": {
			"// EffectLogout は効果 app.logout（状態 app.token への delete）。\nconst EffectLogout",
			"// EffectPlaceOrder は注文を確定する\n//\n// on app.Checkout: Shopping -> Ordered\nconst EffectPlaceOrder",
			"// EffectClearCart は効果 app.clearCart。\n//\n// on app.carts: HasItems -> Empty\nconst EffectClearCart",
			"// on app.Checkout: Shopping | Ordered -> any\nconst EffectTouch",
		},
	} {
		wantContains(t, file, contentOf(files, file), wants, []string{"context", "CheckoutCases", "Classifiers"})
	}
	// 葉がすべて状態の命令なので、Leaves も Classifiers もない
	wantContains(t, "gen/app/ops.go", contentOf(files, "gen/app/ops.go"), []string{"func New() Ops {"}, []string{"Leaves", "Classifiers"})
	compileFiles(t, root, files)
}

// TestMatchShapeMismatch は、match の形と分類が合わない IR（MATCH-7 を通っていれば来ない）を、黙って誤ったコードにせず
// GOEMIT-INTERNAL の error にして、ファイルを出さないこと。
func TestMatchShapeMismatch(t *testing.T) {
	root := newProjectRoot(t)
	carts, token, title, checkout := stateDecls()
	kind := &ir.Classify{DeclBase: base("app.Kind", nil), Subject: ir.TypeSubject{Type: ir.String()}, Cases: []ir.ClassifyCase{{Name: "A"}, {Name: "B"}}}
	both := []ir.MatchArm{stateArm(nil, ir.CaseEmpty), stateArm(nil, ir.CaseHasItems)}
	wire := func(m ir.Match) *ir.Op {
		return &ir.Op{DeclBase: base("app.run", nil), Params: []ir.OpParam{param("s", ir.String())}, Body: wiring(matchStmt(m))}
	}
	tests := []struct {
		name  string
		decls []ir.Decl
		want  string // メッセージに含む語
	}{
		{"by list on single", []ir.Decl{title, wire(byState("app.title", ir.StateList, both...))}, "app.title が名前空間 app の list の状態でない"},
		{"by list on nullable", []ir.Decl{token, wire(byState("app.token", ir.StateList, both...))}, "app.token が名前空間 app の list の状態でない"},
		{"by nullable on unknown state", []ir.Decl{wire(byState("app.nope", ir.StateNullable, both...))}, "app.nope が名前空間 app の nullable の状態でない"},
		{"by list on a non-state", []ir.Decl{wire(byState("valid.min", ir.StateList, both...))}, "valid.min が名前空間 app の list の状態でない"},
		{"by list without HasItems", []ir.Decl{carts, wire(byState("app.carts", ir.StateList, stateArm(nil, ir.CaseEmpty)))}, "アームが Empty / HasItems を尽くしていない"},
		{"match by value classify", []ir.Decl{kind, wire(byClassify("app.Kind", stateArm(nil, "A"), stateArm(nil, "B")))}, "app.Kind が状態の分類でない"},
		{"match by unknown classify", []ir.Decl{wire(byClassify("app.Nope", stateArm(nil, "A")))}, "app.Nope が状態の分類でない"},
		{"value match by state classify", []ir.Decl{carts, checkout,
			wire(ir.Match{Subject: ir.ValueSubject{Atom: name("s")}, Classify: "app.Checkout", Arms: []ir.MatchArm{stateArm(nil, "Shopping"), stateArm(nil, "Ordered")}, Span: sp("x.hozo", 1)})},
			"app.Checkout が値の分類でない"},
		{"match without subject", []ir.Decl{carts, wire(ir.Match{Classify: "list", Arms: both, Span: sp("x.hozo", 1)})}, "対象の種類が分からない"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := smallView(tt.decls...)
			files, diags := Emit(v, Options{Root: root, Select: plugin.Select{Roots: []string{"app.run"}}})
			if files != nil {
				t.Errorf("ファイルが出た: %d", len(files))
			}
			if len(diags) != 1 {
				t.Fatalf("診断が %d 個、期待は 1 個: %s", len(diags), formatDiags(diags))
			}
			if d := diags[0]; d.Code != CodeInternal || !strings.Contains(d.Message, tt.want) {
				t.Errorf("診断 = %s[%s] %q, want GOEMIT-INTERNAL に %q", d.Severity, d.Code, d.Message, tt.want)
			}
		})
	}
}

// TestZeroValues は docs/emit-go.md「配線の本体」の零値: 実装が埋められていないとき（と配線の途中の失敗）に、
// 戻り値の型の零値と error を返す。名前付きの型（scalar）は、元の型の零値をその型に変換したもの。
func TestZeroValues(t *testing.T) {
	root := newProjectRoot(t)
	leaf := func(name string, ret ir.Type) *ir.Op { return &ir.Op{DeclBase: base("app."+name, nil), Returns: ret} }
	files, diags := emitSmall(root,
		&ir.Scalar{DeclBase: base("app.Id", nil), Base: ir.String()},
		&ir.Scalar{DeclBase: base("app.Count", nil), Base: ir.Int()},
		&ir.Scalar{DeclBase: base("app.Flag", nil), Base: ir.Boolean()},
		&ir.Scalar{DeclBase: base("app.Ratio", nil), Base: ir.Float()},
		&ir.Scalar{DeclBase: base("app.At", nil), Base: ir.Time()},
		&ir.Scalar{DeclBase: base("app.Wait", nil), Base: ir.Duration()},
		&ir.Scalar{DeclBase: base("app.Sub", nil), Base: ir.Ref("app.Id")},
		&ir.Model{DeclBase: base("app.M", nil)},
		&ir.Union{DeclBase: base("app.U", nil), Variant: ir.UnionArms, Arms: []ir.UnionArm{{Name: "M", Type: ir.Ref("app.M"), Span: sp("x.hozo", 1)}}},
		leaf("id", ir.Ref("app.Id")), leaf("count", ir.Ref("app.Count")), leaf("flag", ir.Ref("app.Flag")), leaf("ratio", ir.Ref("app.Ratio")),
		leaf("at", ir.Ref("app.At")), leaf("wait", ir.Ref("app.Wait")), leaf("sub", ir.Ref("app.Sub")),
		leaf("model", ir.Ref("app.M")), leaf("union", ir.Ref("app.U")), leaf("opt", ir.Optional(ir.Ref("app.Id"))), leaf("list", ir.Array(ir.Ref("app.Id"))),
		leaf("text", ir.String()), leaf("num", ir.Int()), leaf("when", ir.Time()), leaf("void", nil),
	)
	if len(diags) != 0 {
		t.Fatalf("診断が出た: %s", formatDiags(diags))
	}
	ops := contentOf(files, "gen/app/ops.go")
	for op, zero := range map[string]string{
		"id": `Id("")`, "count": "Count(0)", "flag": "Flag(false)", "ratio": "Ratio(0)", "at": "At{}", "wait": "Wait(0)", "sub": `Sub("")`,
		"model": "M{}", "union": "nil", "opt": "nil", "list": "nil", "text": `""`, "num": "0", "when": "time.Time{}",
	} {
		want := "\t\treturn " + zero + `, &runtime.MissingError{Kind: "leaf", Name: "app.` + op + `"}`
		if !strings.Contains(ops, want) {
			t.Errorf("ops.go に %q がない", want)
		}
	}
	if want := "\t\treturn &runtime.MissingError{Kind: \"leaf\", Name: \"app.void\"}"; !strings.Contains(ops, want) {
		t.Errorf("ops.go に %q がない:\n%s", want, ops)
	}
	compileFiles(t, root, files)
}
