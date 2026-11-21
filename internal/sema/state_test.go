package sema

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// 状態を分類で表す決定（docs/decisions.md「設計書から離れた決定」S1〜S5）の、解決と正規化。
// 期待値は docs/ir.md「state / exception / effect」「classify」「配線」と docs/check.md「状態の形」から。

const stateSource = `
namespace shop {
  model Line { id: string; }
  model Order { id: string; }
  scalar Token extends string;

  state carts: list<Line>;
  state orders: list<Order>;
  state token: nullable<Token>;
  state clock: single<datetime>;

  classify Checkout of carts, orders {
    Shopping;
    Ordered;
    Idle;
  }
  classify LoggedIn of token { LoggedIn; LoggedOut; }
  classify LineKind of Line { Gift; Normal; }

  effect fn addLine(): effects carts.post;
  effect fn tick(): effects clock.put;
  effect fn logout(): effects token.delete;
  effect fn placeOrder(): effects Checkout as Shopping -> Ordered;
  effect fn reset(): effects Checkout as any -> Shopping;
  effect fn touch(): effects Checkout as Ordered, Shopping -> any;
  effect fn clearCart(): effects carts as HasItems -> Empty;
  effect fn login(): effects token as None -> Some;
  effect fn signIn(): effects LoggedIn as any -> LoggedIn;

  fn showEmpty(): void;
  fn showCart(): void;
  fn me(): Token;
  fn gift(line: Line on LineKind.Gift): Line;
  fn normal(line: Line): Line;
  fn newOrder(): Order;
  fn order(): Order effects placeOrder { placeOrder(); return newOrder(); }

  fn view(line: Line): Order {
    match(carts by list){
      Empty => showEmpty();
      HasItems => showCart();
    };
    val   who = match(token by nullable){
      Some => me();
      None => none;
    };
    val   kind = match(line by LineKind){
      Gift => gift(line);
      Normal => normal(line);
    };
    val   placed = match(by Checkout){
      Shopping, Ordered => order();
      else => order();
    };
    match(by LoggedIn){
      LoggedIn => showCart();
      LoggedOut => none;
    };
    return placed;
  }
}`

func analyzeState(t *testing.T) *ir.Program {
	t.Helper()
	res, fs := analyzeProject(t, project{"shop.hozo": stateSource}, false)
	noErrors(t, res, fs)
	for _, d := range res.Diags {
		if d.Code != "WIRE-8" { // who と kind は使っていない（意図した warning）
			t.Errorf("想定しない診断: %s %s", d.Code, d.Message)
		}
	}
	return res.Program
}

func jsonEq(t *testing.T, name string, got, want any) {
	t.Helper()
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	if string(g) != string(w) {
		t.Errorf("%s:\n got  %s\n want %s", name, g, w)
	}
}

// TestStateShapes は docs/ir.md「state / exception / effect」の State.shape: 三つの形。
func TestStateShapes(t *testing.T) {
	p := analyzeState(t)
	for name, want := range map[string]ir.StateShape{
		"shop.carts": ir.StateList, "shop.token": ir.StateNullable, "shop.clock": ir.StateSingle,
	} {
		if got := p.Lookup(name).(*ir.State).Shape; got != want {
			t.Errorf("%s の形 = %s, want %s", name, got, want)
		}
	}
	jsonEq(t, "nullable の要素は中身の型", p.Lookup("shop.token").(*ir.State).Element, ir.Ref("shop.Token"))
}

// TestStateElementOptional は STATE-3 のときの IR: 当たるのは list の状態だけで、list<T> として載る
// （element は optional にならない）。single<T?> と nullable<T?> は nullable<T> と同じ型なので当たらない。
func TestStateElementOptional(t *testing.T) {
	res := runAnnotated(t, project{"a.hozo": `
namespace a {
  state tokens: list<string?>;         // ERROR: STATE-3
  state marks: string?[];              // ERROR: STATE-3
  state token: single<string?>;
  state other: nullable<string?>;
  state rows: list<string>?;
}`}, false)
	for _, name := range []string{"a.tokens", "a.marks"} {
		st := res.Program.Lookup(name).(*ir.State)
		if st.Shape != ir.StateList {
			t.Errorf("%s: shape = %s", name, st.Shape)
		}
		jsonEq(t, name+" の type", st.Type, ir.Array(ir.String()))
		jsonEq(t, name+" の element", st.Element, ir.String())
	}
	for _, name := range []string{"a.token", "a.other"} {
		st := res.Program.Lookup(name).(*ir.State)
		if st.Shape != ir.StateNullable {
			t.Errorf("%s: shape = %s", name, st.Shape)
		}
		jsonEq(t, name+" の type", st.Type, ir.Optional(ir.String()))
		jsonEq(t, name+" の element", st.Element, ir.String())
	}
	// 形を決めるのは最も外側だけ: T[]? は nullable（要素は T[]）
	rows := res.Program.Lookup("a.rows").(*ir.State)
	jsonEq(t, "rows", []any{rows.Shape, rows.Element}, []any{ir.StateNullable, ir.Array(ir.String())})
}

// TestClassifySubject は docs/ir.md「classify」の subject: 型一つなら type、状態なら states（書かれた順）。
func TestClassifySubject(t *testing.T) {
	p := analyzeState(t)
	jsonEq(t, "値の分類", p.Lookup("shop.LineKind").(*ir.Classify).Subject, ir.TypeSubject{Type: ir.Ref("shop.Line")})
	checkout := p.Lookup("shop.Checkout").(*ir.Classify)
	jsonEq(t, "状態の分類", checkout.Subject, ir.StatesSubject{States: []string{"shop.carts", "shop.orders"}})
	if len(checkout.Cases) != 3 || checkout.Cases[2].Name != "Idle" {
		t.Errorf("Checkout = %+v", checkout)
	}
	cond := p.Lookup("shop.LoggedIn").(*ir.Classify)
	jsonEq(t, "二つに分かれる状態の分類", cond.Subject, ir.StatesSubject{States: []string{"shop.token"}})
	if len(cond.Cases) != 2 || cond.Cases[0].Name != "LoggedIn" || cond.Cases[1].Name != "LoggedOut" || cond.DecidedBy != ir.DecidedByPredicate {
		t.Errorf("LoggedIn = %+v", cond)
	}
}

// TestClassifySubjectOrder は states が書かれた順であること（宣言の順ではない）。
func TestClassifySubjectOrder(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  state x: list<string>;
  state y: list<string>;
  classify C of y, x { A; }
}`}, false)
	noErrors(t, res, fs)
	jsonEq(t, "states", res.Program.Lookup("a.C").(*ir.Classify).Subject, ir.StatesSubject{States: []string{"a.y", "a.x"}})
	hasRef(t, res, fs, "a.hozo:5:17 decl a.y")
	hasRef(t, res, fs, "a.hozo:5:20 decl a.x")
}

// TestClassifySubjectInvalid は CLS-3 のときの IR: 対象は決まらない（null）。重複だけなら重複を除いた状態の分類。
func TestClassifySubjectInvalid(t *testing.T) {
	res := runAnnotated(t, project{"a.hozo": `
namespace a {
  model M { id: string; }
  state x: list<M>;
  classify Mixed of x, M { A; }        // ERROR: CLS-3
  classify Twice of x, x { A; }        // ERROR: CLS-3
  fn f(): void;
  // 対象が決まらなかった分類での match には、MATCH-7 を重ねない
  fn g(): void {
    match(by Mixed){ A => f(); };
    match(by Twice){ A => f(); };
  }
}`}, false)
	if s := res.Program.Lookup("a.Mixed").(*ir.Classify).Subject; s != nil {
		t.Errorf("Mixed の subject = %#v", s)
	}
	jsonEq(t, "Twice", res.Program.Lookup("a.Twice").(*ir.Classify).Subject, ir.StatesSubject{States: []string{"a.x"}})
}

// TestEffectTargets は docs/ir.md「state / exception / effect」の Effect.target と、docs/check.md「状態の形」の
// 「効果が書き込む状態」（Effect.writes）。
func TestEffectTargets(t *testing.T) {
	p := analyzeState(t)
	tests := []struct {
		name   string
		target ir.EffectTarget
		writes []string
	}{
		// 組み込みの効果: 書き込むのはその基底の状態
		{"shop.addLine", ir.BuiltinTarget{State: "shop.carts", Operation: ir.OpPost}, []string{"shop.carts"}},
		{"shop.tick", ir.BuiltinTarget{State: "shop.clock", Operation: ir.OpPut}, []string{"shop.clock"}},
		{"shop.logout", ir.BuiltinTarget{State: "shop.token", Operation: ir.OpDelete}, []string{"shop.token"}},
		// 遷移で宛先が状態の分類: その対象のすべての基底の状態
		{"shop.placeOrder", ir.TransitionTarget{Of: "shop.Checkout", From: ir.Cases("Shopping"), To: ir.Cases("Ordered")}, []string{"shop.carts", "shop.orders"}},
		{"shop.reset", ir.TransitionTarget{Of: "shop.Checkout", From: ir.AnyCase(), To: ir.Cases("Shopping")}, []string{"shop.carts", "shop.orders"}},
		// ケースの列は書かれた順
		{"shop.touch", ir.TransitionTarget{Of: "shop.Checkout", From: ir.Cases("Ordered", "Shopping"), To: ir.AnyCase()}, []string{"shop.carts", "shop.orders"}},
		{"shop.signIn", ir.TransitionTarget{Of: "shop.LoggedIn", From: ir.AnyCase(), To: ir.Cases("LoggedIn")}, []string{"shop.token"}},
		// 遷移で宛先が基底の状態: その状態
		{"shop.clearCart", ir.TransitionTarget{Of: "shop.carts", From: ir.Cases("HasItems"), To: ir.Cases("Empty")}, []string{"shop.carts"}},
		{"shop.login", ir.TransitionTarget{Of: "shop.token", From: ir.Cases("None"), To: ir.Cases("Some")}, []string{"shop.token"}},
	}
	for _, tt := range tests {
		e := p.Lookup(tt.name).(*ir.Effect)
		if !reflect.DeepEqual(e.Target, tt.target) {
			t.Errorf("%s の target = %#v, want %#v", tt.name, e.Target, tt.target)
		}
		if !reflect.DeepEqual(e.Writes, tt.writes) {
			t.Errorf("%s の writes = %v, want %v", tt.name, e.Writes, tt.writes)
		}
	}
}

// TestEffectTargetRefs は遷移の名前とケース名の参照（定義へのジャンプ）。組み込みのケースには定義がない。
func TestEffectTargetRefs(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  state s: list<string>;
  classify Phase of s { Idle; Busy; }
  effect fn start(): effects Phase as Idle -> Busy;
  effect fn drain(): effects s as HasItems -> Empty;
}`}, false)
	noErrors(t, res, fs)
	hasRef(t, res, fs, "a.hozo:5:30 decl a.Phase")
	hasRef(t, res, fs, "a.hozo:5:39 case a.Phase.Idle")
	hasRef(t, res, fs, "a.hozo:5:47 case a.Phase.Busy")
	hasRef(t, res, fs, "a.hozo:6:30 decl a.s")
	for _, s := range refStrings(res, fs) {
		if s == "a.hozo:6:35 case a.s.HasItems" || s == "a.hozo:6:47 case a.s.Empty" {
			t.Errorf("組み込みのケースに参照がある: %s", s)
		}
	}
}

// TestStandardCaseNames は、標準の分類のケースの名前が、形ごとに違うこと（docs/check.md「分類」の「標準の分類」）: list は
// Empty / HasItems、nullable は None / Some。形に合わない名前を書けば、アームは MATCH-2、状態の遷移は STATE-4 で、
// メッセージが、その形のケースを並べる。
func TestStandardCaseNames(t *testing.T) {
	res := runAnnotated(t, project{"a.hozo": `
namespace a {
  state items: list<string>;
  state token: nullable<string>;
  fn f();
  effect fn fill(): effects items as Empty -> HasItems;
  effect fn save(): effects token as None -> Some;
  effect fn oldNames(): effects token as Empty -> HasItems;   // ERROR: STATE-4
  effect fn otherShape(): effects items as None -> Some;      // ERROR: STATE-4
  fn ok() {
    match (items by list) { Empty => f(); HasItems => none; }
    match (token by nullable) { None => f(); Some => none; }
  }
  fn ng() {
    match (token by nullable) { Empty => f(); HasItems => none; }   // ERROR: MATCH-2
    match (items by list) { None => f(); Some => none; }            // ERROR: MATCH-2
  }
}`}, false)
	for _, want := range []struct{ code, part string }{
		{"STATE-4", "（nullable）の組み込みの分類のケースではない（None / Some）"},
		{"STATE-4", "（list）の組み込みの分類のケースではない（Empty / HasItems）"},
		{"MATCH-2", "標準の分類 nullable のケース（None / Some）"},
		{"MATCH-2", "標準の分類 list のケース（Empty / HasItems）"},
	} {
		found := false
		for _, d := range res.Diags {
			if d.Code == want.code && strings.Contains(d.Message, want.part) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s のメッセージに %q を含むものがない", want.code, want.part)
		}
	}
}

// TestEffectTargetInvalid は宛先が決まらないときの IR: target は null、writes は空。
func TestEffectTargetInvalid(t *testing.T) {
	res := runAnnotated(t, project{"a.hozo": `
namespace a {
  model M { id: string; }
  state clock: single<string>;
  state items: list<M>;
  classify Kind of M { A; B; }
  effect fn onSingle(): effects clock as Empty -> HasItems; // ERROR: STATE-4
  effect fn value(): effects Kind as A -> B;                // ERROR: RES-4
  effect fn unknown(): effects nothing as A -> B;           // ERROR: RES-1
  effect fn badCase(): effects items as Empty -> Full;      // ERROR: STATE-4
}`}, false)
	for _, name := range []string{"a.onSingle", "a.value", "a.unknown"} {
		e := res.Program.Lookup(name).(*ir.Effect)
		if e.Target != nil || e.Writes != nil {
			t.Errorf("%s: target = %#v, writes = %v", name, e.Target, e.Writes)
		}
	}
	// ケースの誤りでは、宛先は書かれたまま残る
	bad := res.Program.Lookup("a.badCase").(*ir.Effect)
	jsonEq(t, "badCase", bad.Target, ir.TransitionTarget{Of: "a.items", From: ir.Cases("Empty"), To: ir.Cases("Full")})
	jsonEq(t, "badCase の writes", bad.Writes, []string{"a.items"})
}

// TestMatchSubjects は docs/ir.md「配線」の Match.subject の形と、Match.classify（標準の分類なら "list" か "nullable"）。
// 状態を名前で書いた対象は、value の atom が state。
func TestMatchSubjects(t *testing.T) {
	p := analyzeState(t)
	stmts := p.Lookup("shop.view").(*ir.Op).Body.Statements

	byList := stmts[0].(ir.MatchStmt).Match
	jsonEq(t, "by list の subject", byList.Subject, ir.ValueSubject{Atom: ir.StateAtom{State: "shop.carts", Span: byList.Subject.(ir.ValueSubject).Atom.(ir.StateAtom).Span}})
	if state, ok := byList.SubjectState(); !ok || state != "shop.carts" {
		t.Errorf("by list の SubjectState = %q, %v", state, ok)
	}
	if byList.Classify != "list" {
		t.Errorf("by list の classify = %q", byList.Classify)
	}
	jsonEq(t, "by list のアーム", [][]string{byList.Arms[0].Cases, byList.Arms[1].Cases},
		[][]string{{"Empty"}, {"HasItems"}})

	byNullable := stmts[1].(ir.ValStmt).Expr.(ir.MatchExpr).Match
	jsonEq(t, "by nullable の subject", byNullable.Subject, ir.ValueSubject{Atom: ir.StateAtom{State: "shop.token", Span: byNullable.Subject.(ir.ValueSubject).Atom.(ir.StateAtom).Span}})
	if byNullable.Classify != "nullable" {
		t.Errorf("by nullable の classify = %q", byNullable.Classify)
	}
	// アームは書かれた順
	jsonEq(t, "by nullable のアーム", []any{byNullable.Arms[0].Cases, byNullable.Arms[1].Cases, byNullable.Arms[1].Body},
		[]any{[]string{"Some"}, []string{"None"}, ir.NoneBody{}})

	byValue := stmts[2].(ir.ValStmt).Expr.(ir.MatchExpr).Match
	value, ok := byValue.Subject.(ir.ValueSubject)
	if !ok {
		t.Fatalf("値の match の subject = %#v", byValue.Subject)
	}
	if name, ok := value.Atom.(ir.NameAtom); !ok || !reflect.DeepEqual(name.Path, []string{"line"}) {
		t.Errorf("値の match の atom = %#v", value.Atom)
	}
	if byValue.Classify != "shop.LineKind" {
		t.Errorf("値の match の classify = %q", byValue.Classify)
	}

	byClassify := stmts[3].(ir.ValStmt).Expr.(ir.MatchExpr).Match
	jsonEq(t, "match by C の subject", byClassify.Subject, ir.ClassifyOwnedSubject{})
	if byClassify.Classify != "shop.Checkout" {
		t.Errorf("match by C の classify = %q", byClassify.Classify)
	}
	// 状態の分類での match の else のアーム: cases は、並べなかったケース
	jsonEq(t, "match by C のアーム", []any{byClassify.Arms[0].Cases, byClassify.Arms[0].Else, byClassify.Arms[1].Cases, byClassify.Arms[1].Else},
		[]any{[]string{"Shopping", "Ordered"}, false, []string{"Idle"}, true})

	byCondition := stmts[4].(ir.MatchStmt).Match
	jsonEq(t, "二つに分かれる状態の分類での match の subject", byCondition.Subject, ir.ClassifyOwnedSubject{})
	if byCondition.Classify != "shop.LoggedIn" {
		t.Errorf("二つに分かれる状態の分類での match の classify = %q", byCondition.Classify)
	}

	// 読み戻しても同じ（docs/ir.md「JSON」）
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var back ir.Program
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(&back)
	if string(again) != string(b) {
		t.Error("JSON の往復で変わる")
	}
}

// TestMatchStateRefs は状態の match の対象・分類・ケース名の参照。
func TestMatchStateRefs(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  state s: list<string>;
  classify Phase of s { Idle; Busy; }
  fn f(): void;
  fn g(): void {
    match(s by list){ Empty => f(); HasItems => none; };
    match(by Phase){ Idle => f(); Busy => none; };
  }
}`}, false)
	noErrors(t, res, fs)
	hasRef(t, res, fs, "a.hozo:7:11 decl a.s")
	hasRef(t, res, fs, "a.hozo:8:14 decl a.Phase")
	hasRef(t, res, fs, "a.hozo:8:22 case a.Phase.Idle")
	hasRef(t, res, fs, "a.hozo:8:35 case a.Phase.Busy")
}

// TestMatchStateTyping は docs/types.md「match の型」: 状態を対象にした match の型は、値の match と同じ決め方。
// 対象と関係のない引数は、そのままの型で照合する（TYPE-4）。
func TestMatchStateTyping(t *testing.T) {
	runAnnotated(t, project{"a.hozo": `
namespace a {
  model M { id: string; }
  model N { name: string; }
  state items: list<M>;
  state token: nullable<string>;
  classify Stock of items { None; Some; }
  fn m(): M;
  fn n(): N;
  fn nothing(): void;
  fn takeM(m: M): void;
  fn takeOptM(m: M?): void;
  fn takeN(n: N): void;

  // すべてのアームが同じ型 → その型。none のアームがあれば optional
  fn same(): M {
    return match(items by list){ Empty => m(); HasItems => m(); };
  }
  fn optional(): void {
    val   x = match(token by nullable){ None => none; Some => m(); };
    takeOptM(x);
    takeM(x);                                                          // ERROR: TYPE-4
  }
  fn byClassify(): M {
    return match(by Stock){ None => m(); Some => m(); };
  }
  // 値のある呼び出しがなければ「値がない」
  fn noValue(): void {
    match(items by list){ Empty => nothing(); HasItems => none; };
    val   y = match(by Stock){ None => nothing(); Some => none; };     // ERROR: WIRE-2 // WARN: WIRE-8
  }
  // 型がそろわない、値のある呼び出しと値のない呼び出しが混ざる
  fn mixed(): void {
    val   a = match(items by list){ Empty => m(); HasItems => n(); };          // ERROR: TYPE-8 // WARN: WIRE-8
    val   b = match(by Stock){ None => m(); Some => nothing(); };              // ERROR: TYPE-8 // WARN: WIRE-8
    val   c = match(token by nullable){ None => m(); Some => m(); };      // WARN: WIRE-8
  }
  // 対象でない引数は、アームの中でもそのままの型で照合する
  fn args(v: M): void {
    match(items by list){ Empty => takeM(v); HasItems => takeN(v); };  // ERROR: TYPE-4
    match(by Stock){ None => takeM(v); Some => takeN(v); };            // ERROR: TYPE-4
  }
  // 値がある match を束縛せずに書けない
  fn unbound(): void {
    match(items by list){ Empty => m(); HasItems => m(); };            // ERROR: WIRE-2
  }
}`}, false)
}

// TestEffectOnAcceptsOnlyBuiltin は docs/types.md「デコレータの引数」の `any effect on s.op`: 宛先が組み込みの効果
// { state: s, operation: op } の効果だけが満たす。
func TestEffectOnAcceptsOnlyBuiltin(t *testing.T) {
	res := runAnnotated(t, project{"a.hozo": `
namespace a {
  state location: list<string>;
  effect fn go(): effects location.put;
  effect fn leave(): effects location as HasItems -> Empty;
  decorator nav(target: any effect on location.put);
  @nav(go)
  fn ok(): void;
  @nav(leave)                          // ERROR: DEC-7
  fn ng(): void;
}`}, false)
	nav := res.Program.Lookup("a.nav").(*ir.Decorator)
	jsonEq(t, "on", nav.Params[0].Type, ir.AnyParam{Of: ir.AnyEffect, On: &ir.EffectOn{State: "a.location", Operation: ir.OpPut}})
	jsonEq(t, "引数", res.Program.Lookup("a.ok").(*ir.Op).Decorators[0].Args[0].Value, ir.RefValue{Target: "a.go"})
}

// TestMatchStateClassifyWithSubject は「状態の分類なのに対象がある」（MATCH-7）のとき、対象に書いた束縛を使ったことにして
// WIRE-8 を重ねないこと。
func TestMatchStateClassifyWithSubject(t *testing.T) {
	res := runAnnotated(t, project{"a.hozo": `
namespace a {
  model M { id: string; }
  state items: list<M>;
  classify Stock of items { None; Some; }
  fn get(): M;
  fn f(): void;
  fn g(): void {
    val   m = get();
    match(m by Stock){ None => f(); Some => none; };   // ERROR: MATCH-7
  }
}`}, false)
	// 対象は分類の宣言が持つ形で IR に載る
	m := res.Program.Lookup("a.g").(*ir.Op).Body.Statements[1].(ir.MatchStmt).Match
	jsonEq(t, "subject", m.Subject, ir.ClassifyOwnedSubject{})
	if m.Classify != "a.Stock" {
		t.Errorf("classify = %q", m.Classify)
	}
}

// TestClassifySubjectUnresolved は、対象の名前が見つからないときは CLS-3 ではなく RES-1 になること
// （状態か型かが決まらない。docs/resolve.md「手続き」1.7）。
func TestClassifySubjectUnresolved(t *testing.T) {
	res := runAnnotated(t, project{"a.hozo": `
namespace a {
  state carts: list<string>;
  classify One of nothing { A; }                 // ERROR: RES-1
  classify Two of carts, nothing { A; }          // ERROR: RES-1
  classify Qualified of carts, b.nothing { A; }  // ERROR: RES-1
}`}, false)
	for _, name := range []string{"a.One", "a.Two", "a.Qualified"} {
		if s := res.Program.Lookup(name).(*ir.Classify).Subject; s != nil {
			t.Errorf("%s の subject = %#v, want nil", name, s)
		}
	}
}
