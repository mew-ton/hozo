package ir

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// roundTrip は v を書き、同じ型の新しい値に読み、もう一度書く。
// 読んだ値が元と等しく（nil の列の規約で）、二度目のバイト列が一度目と同じことを確かめる。
func roundTrip(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	got := reflect.New(reflect.TypeOf(v)).Interface()
	if err := json.Unmarshal(b, got); err != nil {
		t.Fatalf("unmarshal %T: %v\n%s", v, err, b)
	}
	gotV := reflect.ValueOf(got).Elem().Interface()
	if !reflect.DeepEqual(gotV, v) {
		t.Errorf("%T: round trip differs\nwant %#v\ngot  %#v\njson %s", v, v, gotV, b)
	}
	b2, err := json.Marshal(gotV)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, b2) {
		t.Errorf("%T: second marshal differs\n1: %s\n2: %s", v, b, b2)
	}
	return b
}

// unionRoundTrip は interface の値を単独で書き、Unmarshal* で読み戻す。
func unionRoundTrip[I any](t *testing.T, v I, dec func([]byte) (I, error), wantJSON string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	if string(b) != wantJSON {
		t.Errorf("%T: json\nwant %s\ngot  %s", v, wantJSON, b)
	}
	got, err := dec(b)
	if err != nil {
		t.Fatalf("decode %T: %v", v, err)
	}
	if !reflect.DeepEqual(got, v) {
		t.Errorf("%T: round trip differs\nwant %#v\ngot  %#v", v, v, got)
	}
}

func sp() Span { return span("a.hozo", 1, 2, 3, 4) }

const spJSON = `"span":{"file":"a.hozo","start":{"line":1,"col":2},"end":{"line":3,"col":4}}`

func TestRoundTripTypes(t *testing.T) {
	unionRoundTrip(t, String(), UnmarshalType, `{"kind":"core","name":"string"}`)
	unionRoundTrip(t, Boolean(), UnmarshalType, `{"kind":"core","name":"boolean"}`)
	unionRoundTrip(t, Int(), UnmarshalType, `{"kind":"core","name":"int"}`)
	unionRoundTrip(t, Float(), UnmarshalType, `{"kind":"core","name":"float"}`)
	unionRoundTrip(t, Time(), UnmarshalType, `{"kind":"core","name":"Time"}`)
	unionRoundTrip(t, Duration(), UnmarshalType, `{"kind":"core","name":"Duration"}`)
	unionRoundTrip(t, Ref("server.User"), UnmarshalType, `{"kind":"ref","target":"server.User"}`)
	unionRoundTrip(t, Optional(Int()), UnmarshalType, `{"kind":"optional","of":{"kind":"core","name":"int"}}`)
	unionRoundTrip(t, Array(Optional(Int())), UnmarshalType, `{"kind":"array","of":{"kind":"optional","of":{"kind":"core","name":"int"}}}`)
	unionRoundTrip(t, Record(String(), Ref("X")), UnmarshalType, `{"kind":"record","key":{"kind":"core","name":"string"},"value":{"kind":"ref","target":"X"}}`)
	unionRoundTrip(t, Case("s.C", []string{"B", "A"}, Ref("s.T")), UnmarshalType,
		`{"kind":"case","classify":"s.C","cases":["B","A"],"of":{"kind":"ref","target":"s.T"}}`)
	// cases は空でも [] で出す。
	unionRoundTrip(t, Case("s.C", nil, Int()), UnmarshalType, `{"kind":"case","classify":"s.C","cases":[],"of":{"kind":"core","name":"int"}}`)
	// never は Op.returns にだけ現れる（docs/ir.md「型」）。
	unionRoundTrip(t, Never(), UnmarshalType, `{"kind":"never"}`)
	if !IsNever(Never()) || IsNever(Int()) || IsNever(nil) || Never().String() != "never" {
		t.Errorf("IsNever / String of never")
	}
	if !TypeEqual(Never(), Never()) || TypeEqual(Never(), Int()) {
		t.Errorf("TypeEqual of never")
	}
}

func TestRoundTripParamTypes(t *testing.T) {
	unionRoundTrip(t, ParamType(TypeParam{Types: []Type{Int(), Float()}}), UnmarshalParamType,
		`{"kind":"type","types":[{"kind":"core","name":"int"},{"kind":"core","name":"float"}]}`)
	unionRoundTrip(t, ParamType(AnyParam{Of: AnyState}), UnmarshalParamType, `{"kind":"any","of":"state"}`)
	unionRoundTrip(t, ParamType(AnyParam{Of: AnyEffect, On: &EffectOn{State: "db", Operation: OpDelete}}), UnmarshalParamType,
		`{"kind":"any","of":"effect","on":{"state":"db","operation":"delete"}}`)
}

func TestRoundTripLiterals(t *testing.T) {
	unionRoundTrip(t, Literal(StringLiteral{Value: "a\"b"}), UnmarshalLiteral, `{"kind":"string","value":"a\"b"}`)
	unionRoundTrip(t, Literal(IntLiteral{Value: -9007199254740993}), UnmarshalLiteral, `{"kind":"int","value":-9007199254740993}`)
	unionRoundTrip(t, Literal(FloatLiteral{Value: 2.5}), UnmarshalLiteral, `{"kind":"float","value":2.5}`)
	unionRoundTrip(t, Literal(BooleanLiteral{Value: false}), UnmarshalLiteral, `{"kind":"boolean","value":false}`)
	unionRoundTrip(t, Literal(DurationLiteral{Value: 5 * time.Minute}), UnmarshalLiteral, `{"kind":"duration","value":300000000000}`)
}

func TestRoundTripAtoms(t *testing.T) {
	unionRoundTrip(t, Atom(NameAtom{Path: []string{"query", "page"}, Span: sp()}), UnmarshalAtom,
		`{"kind":"name","path":["query","page"],`+spJSON+`}`)
	// 状態の参照（docs/ir.md「配線」の Atom の state）。状態そのものなら path は []。
	unionRoundTrip(t, Atom(StateAtom{State: "browser.token", Span: sp()}), UnmarshalAtom,
		`{"kind":"state","state":"browser.token","path":[],`+spJSON+`}`)
	unionRoundTrip(t, Atom(StateAtom{State: "browser.session", Path: []string{"token"}, Span: sp()}), UnmarshalAtom,
		`{"kind":"state","state":"browser.session","path":["token"],`+spJSON+`}`)
	unionRoundTrip(t, Atom(LiteralAtom{Literal: IntLiteral{Value: 1}, Span: sp()}), UnmarshalAtom,
		`{"kind":"literal","literal":{"kind":"int","value":1},`+spJSON+`}`)
	unionRoundTrip(t, Atom(NoneAtom{Span: sp()}), UnmarshalAtom, `{"kind":"none",`+spJSON+`}`)
	unionRoundTrip(t, Atom(RecordAtom{Span: sp()}), UnmarshalAtom, `{"kind":"record","fields":[],`+spJSON+`}`)
	unionRoundTrip(t, Atom(RecordAtom{Fields: []RecordField{{Name: "item", Value: NameAtom{Path: []string{"item"}, Span: sp()}}}, Span: sp()}), UnmarshalAtom,
		`{"kind":"record","fields":[{"name":"item","value":{"kind":"name","path":["item"],`+spJSON+`}}],`+spJSON+`}`)
}

func TestRoundTripExprsAndStatements(t *testing.T) {
	call := Call{Op: "s.f", Args: []CallArg{{Param: "x", Value: NoneAtom{Span: sp()}}}, Bound: []BoundPosition{{Path: []string{"x", "y"}, State: "db"}}, Span: sp()}
	callJSON := `{"op":"s.f","args":[{"param":"x","value":{"kind":"none",` + spJSON + `}}],"bound":[{"path":["x","y"],"state":"db"}],` + spJSON + `}`
	match := Match{
		Subject:  ValueSubject{Atom: NameAtom{Path: []string{"u"}, Span: sp()}},
		Classify: "s.C",
		Arms: []MatchArm{
			{Cases: ArmCases{Cases: []string{"A", "B"}}, Body: CallExpr{Call: call}, Span: sp()},
			{Cases: DefaultArm(), Body: NoneBody{}, Span: sp()},
		},
		Span: sp(),
	}
	matchJSON := `{"subject":{"kind":"value","atom":{"kind":"name","path":["u"],` + spJSON + `}},"classify":"s.C","arms":[` +
		`{"cases":["A","B"],"body":{"kind":"call","call":` + callJSON + `},"narrowed":[],` + spJSON + `},` +
		`{"cases":"default","body":{"kind":"none"},"narrowed":[],` + spJSON + `}],` + spJSON + `}`

	unionRoundTrip(t, Expr(CallExpr{Call: call}), UnmarshalExpr, `{"kind":"call","call":`+callJSON+`}`)
	unionRoundTrip(t, Expr(MatchExpr{Match: match}), UnmarshalExpr, `{"kind":"match","match":`+matchJSON+`}`)
	unionRoundTrip(t, Expr(AtomExpr{Atom: NoneAtom{Span: sp()}}), UnmarshalExpr, `{"kind":"atom","atom":{"kind":"none",`+spJSON+`}}`)

	unionRoundTrip(t, ArmBody(CallExpr{Call: call}), UnmarshalArmBody, `{"kind":"call","call":`+callJSON+`}`)
	unionRoundTrip(t, ArmBody(NoneBody{}), UnmarshalArmBody, `{"kind":"none"}`)
	unionRoundTrip(t, ArmBody(AtomExpr{Atom: NameAtom{Path: []string{"session"}, Span: sp()}}), UnmarshalArmBody,
		`{"kind":"atom","atom":{"kind":"name","path":["session"],`+spJSON+`}}`)
	unionRoundTrip(t, ArmBody(ThrowBody{Exception: "type.Unauthorized", Span: sp()}), UnmarshalArmBody,
		`{"kind":"throw","exception":"type.Unauthorized",`+spJSON+`}`)
	// アームの中で絞り込まれた型（docs/ir.md「配線」の Narrowing）。状態の分類での match だけが state を持つ。
	narrowed := MatchArm{Cases: ArmCases{Cases: []string{"A"}}, Body: NoneBody{}, Narrowed: []Narrowing{{Type: Ref("s.T")}, {State: "s.token", Type: Int()}}, Span: sp()}
	if b := roundTrip(t, narrowed); string(b) != `{"cases":["A"],"body":{"kind":"none"},"narrowed":[{"type":{"kind":"ref","target":"s.T"}},{"state":"s.token","type":{"kind":"core","name":"int"}}],`+spJSON+`}` {
		t.Errorf("narrowed arm: %s", b)
	}

	unionRoundTrip(t, Statement(ConstStmt{Name: "x", Expr: AtomExpr{Atom: NoneAtom{Span: sp()}}, Span: sp()}), UnmarshalStatement,
		`{"kind":"const","name":"x","expr":{"kind":"atom","atom":{"kind":"none",`+spJSON+`}},`+spJSON+`}`)
	unionRoundTrip(t, Statement(ReturnStmt{Expr: CallExpr{Call: call}, Span: sp()}), UnmarshalStatement,
		`{"kind":"return","expr":{"kind":"call","call":`+callJSON+`},`+spJSON+`}`)
	unionRoundTrip(t, Statement(CallStmt{Call: call, Span: sp()}), UnmarshalStatement, `{"kind":"call","call":`+callJSON+`,`+spJSON+`}`)
	unionRoundTrip(t, Statement(MatchStmt{Match: match, Span: sp()}), UnmarshalStatement, `{"kind":"match","match":`+matchJSON+`,`+spJSON+`}`)

	// 空の配線。
	roundTrip(t, Wiring{})
	if b := roundTrip(t, Wiring{}); string(b) != `{"statements":[]}` {
		t.Errorf("empty wiring: %s", b)
	}
}

// docs/ir.md「配線」の Match の subject の二つの形。状態を名前で書いた対象は、value の atom が state。
func TestRoundTripMatchSubjects(t *testing.T) {
	unionRoundTrip(t, MatchSubject(ValueSubject{Atom: NameAtom{Path: []string{"item"}, Span: sp()}}), UnmarshalMatchSubject,
		`{"kind":"value","atom":{"kind":"name","path":["item"],`+spJSON+`}}`)
	stateAtomJSON := `{"kind":"state","state":"browser.token","path":[],` + spJSON + `}`
	unionRoundTrip(t, MatchSubject(ValueSubject{Atom: StateAtom{State: "browser.token", Span: sp()}}), UnmarshalMatchSubject, `{"kind":"value","atom":`+stateAtomJSON+`}`)
	if _, err := UnmarshalMatchSubject([]byte(`{"kind":"state","state":"browser.token"}`)); err == nil {
		t.Error("want error: subject kind \"state\" は 0.3 でなくなった")
	}
	unionRoundTrip(t, MatchSubject(ClassifyOwnedSubject{}), UnmarshalMatchSubject, `{"kind":"classify"}`)

	arm := MatchArm{Cases: ArmCases{Cases: []string{CaseEmpty, CaseHasItems}}, Body: NoneBody{}, Span: sp()}
	armJSON := `{"cases":["Empty","HasItems"],"body":{"kind":"none"},"narrowed":[],` + spJSON + `}`
	// 標準の分類なら classify は "list" か "nullable"。
	b := roundTrip(t, Match{Subject: ValueSubject{Atom: StateAtom{State: "browser.token", Span: sp()}}, Classify: "nullable", Arms: []MatchArm{arm}, Span: sp()})
	if want := `{"subject":{"kind":"value","atom":` + stateAtomJSON + `},"classify":"nullable","arms":[` + armJSON + `],` + spJSON + `}`; string(b) != want {
		t.Errorf("match by nullable\nwant %s\ngot  %s", want, b)
	}
	b = roundTrip(t, Match{Subject: ClassifyOwnedSubject{}, Classify: "shop.Checkout", Arms: []MatchArm{arm}, Span: sp()})
	if want := `{"subject":{"kind":"classify"},"classify":"shop.Checkout","arms":[` + armJSON + `],` + spJSON + `}`; string(b) != want {
		t.Errorf("match by C\nwant %s\ngot  %s", want, b)
	}

	m := Match{Subject: ValueSubject{Atom: NoneAtom{Span: sp()}}}
	if m.SubjectAtom() == nil || m.Builtin() {
		t.Errorf("value match: SubjectAtom = %v, Builtin = %v", m.SubjectAtom(), m.Builtin())
	}
	if _, ok := m.SubjectState(); ok {
		t.Errorf("value match: SubjectState")
	}
	m = Match{Subject: ValueSubject{Atom: StateAtom{State: "s"}}, Classify: "list"}
	if s, ok := m.SubjectState(); m.SubjectAtom() == nil || !m.Builtin() || !ok || s != "s" {
		t.Errorf("state match: SubjectAtom = %v, Builtin = %v, SubjectState = %q %v", m.SubjectAtom(), m.Builtin(), s, ok)
	}
	// パスを持つ状態の参照は、状態そのものではない。
	m = Match{Subject: ValueSubject{Atom: StateAtom{State: "s", Path: []string{"f"}}}, Classify: "nullable"}
	if _, ok := m.SubjectState(); ok || !m.Builtin() {
		t.Errorf("state field match: SubjectState ok = %v, Builtin = %v", ok, m.Builtin())
	}
	m = Match{Subject: ClassifyOwnedSubject{}}
	if m.SubjectAtom() != nil || m.Builtin() {
		t.Errorf("classify match: SubjectAtom = %v, Builtin = %v", m.SubjectAtom(), m.Builtin())
	}
	if _, err := UnmarshalMatchSubject([]byte(`{"kind":"name","path":["u"]}`)); err == nil {
		t.Error("want error: a bare Atom is not a MatchSubject")
	}
}

// docs/ir.md「state / exception / effect」の EffectTarget と CaseSet。
func TestRoundTripEffectTargets(t *testing.T) {
	unionRoundTrip(t, EffectTarget(BuiltinTarget{State: "shop.carts", Operation: OpPost}), UnmarshalEffectTarget,
		`{"kind":"builtin","state":"shop.carts","operation":"post"}`)
	unionRoundTrip(t, EffectTarget(TransitionTarget{Of: "shop.Checkout", From: Cases("Shopping"), To: Cases("Ordered")}), UnmarshalEffectTarget,
		`{"kind":"transition","of":"shop.Checkout","from":["Shopping"],"to":["Ordered"]}`)
	unionRoundTrip(t, EffectTarget(TransitionTarget{Of: "shop.carts", From: AnyCase(), To: Cases("B", "A")}), UnmarshalEffectTarget,
		`{"kind":"transition","of":"shop.carts","from":"any","to":["B","A"]}`)
	unionRoundTrip(t, EffectTarget(TransitionTarget{Of: "shop.carts", From: Cases("HasItems"), To: AnyCase()}), UnmarshalEffectTarget,
		`{"kind":"transition","of":"shop.carts","from":["HasItems"],"to":"any"}`)

	// writes は空でも [] で出す。target は kind で分かれる。
	b := roundTrip(t, Effect{DeclBase: DeclBase{Name: "e", Span: sp()}, Target: BuiltinTarget{State: "s", Operation: OpPut}})
	if want := `{"kind":"effect","name":"e","decorators":[],` + spJSON + `,"target":{"kind":"builtin","state":"s","operation":"put"},"writes":[]}`; string(b) != want {
		t.Errorf("effect\nwant %s\ngot  %s", want, b)
	}
	b = roundTrip(t, Effect{
		DeclBase: DeclBase{Name: "e", Span: sp()},
		Target:   TransitionTarget{Of: "C", From: Cases("A"), To: AnyCase()},
		Writes:   []string{"s1", "s2"},
	})
	if want := `{"kind":"effect","name":"e","decorators":[],` + spJSON + `,"target":{"kind":"transition","of":"C","from":["A"],"to":"any"},"writes":["s1","s2"]}`; string(b) != want {
		t.Errorf("effect\nwant %s\ngot  %s", want, b)
	}

	var c CaseSet
	if err := json.Unmarshal([]byte(`"any"`), &c); err != nil || !c.Any || c.Cases != nil {
		t.Errorf("any: %v %+v", err, c)
	}
	if err := json.Unmarshal([]byte(`"all"`), &c); err == nil {
		t.Error("want error for unknown case set string")
	}
	if err := json.Unmarshal([]byte(`["A","B"]`), &c); err != nil || c.Any || !reflect.DeepEqual(c.Cases, []string{"A", "B"}) {
		t.Errorf("cases: %v %+v", err, c)
	}
	if err := json.Unmarshal([]byte(`{"any":true}`), &c); err == nil {
		t.Error("want error for an object")
	}
	if b, _ := json.Marshal(CaseSet{}); string(b) != `[]` {
		t.Errorf("empty case set: %s", b)
	}
	if _, err := UnmarshalEffectTarget([]byte(`{"kind":"state","state":"s"}`)); err == nil {
		t.Error("want error for unknown effect target kind")
	}
}

// docs/ir.md「classify」の subject の三つの形。
func TestRoundTripClassifySubjects(t *testing.T) {
	unionRoundTrip(t, ClassifySubject(TypeSubject{Type: Ref("server.Item")}), UnmarshalClassifySubject,
		`{"kind":"type","type":{"kind":"ref","target":"server.Item"}}`)
	unionRoundTrip(t, ClassifySubject(StatesSubject{States: []string{"shop.carts", "shop.orders"}}), UnmarshalClassifySubject,
		`{"kind":"states","states":["shop.carts","shop.orders"]}`)
	unionRoundTrip(t, ClassifySubject(StatesSubject{}), UnmarshalClassifySubject, `{"kind":"states","states":[]}`)
	unionRoundTrip(t, ClassifySubject(OpSubject{Op: "store.findUser", Type: Optional(Ref("type.User"))}), UnmarshalClassifySubject,
		`{"kind":"op","op":"store.findUser","type":{"kind":"optional","of":{"kind":"ref","target":"type.User"}}}`)
	oc := &Classify{Subject: OpSubject{Op: "store.findUser", Type: Optional(Ref("type.User"))}, Cases: []ClassifyCase{{Name: "Registered"}}}
	if op, ok := oc.SubjectOp(); !ok || op != "store.findUser" || !TypeEqual(oc.SubjectType(), Optional(Ref("type.User"))) {
		t.Errorf("SubjectOp = %q %v, SubjectType = %v", op, ok, oc.SubjectType())
	}
	if _, ok := oc.SubjectStates(); ok {
		t.Error("SubjectStates of an op classify")
	}
	if oc.Case("Registered") == nil || oc.Case("X") != nil {
		t.Error("Classify.Case")
	}
	// ケースの `対象 as T`（docs/ir.md「classify」の CaseTyping）。as は空でも [] で出す。
	cs := ClassifyCase{Name: "Registered", As: []CaseTyping{{Type: Ref("type.User"), By: NarrowByPresence, Span: sp()}}, Span: sp()}
	if b := roundTrip(t, cs); string(b) != `{"name":"Registered","as":[{"type":{"kind":"ref","target":"type.User"},"by":"presence",`+spJSON+`}],`+spJSON+`}` {
		t.Errorf("case with as: %s", b)
	}
	if b := roundTrip(t, ClassifyCase{Name: "A", Span: sp()}); string(b) != `{"name":"A","as":[],`+spJSON+`}` {
		t.Errorf("case without as: %s", b)
	}
	if b := roundTrip(t, CaseTyping{State: "s.token", Type: Int(), By: NarrowByPredicate, Span: sp()}); string(b) != `{"state":"s.token","type":{"kind":"core","name":"int"},"by":"predicate",`+spJSON+`}` {
		t.Errorf("case typing with state: %s", b)
	}

	c := &Classify{Subject: TypeSubject{Type: Int()}}
	if c.SubjectType() == nil {
		t.Error("SubjectType of a value classify")
	}
	if _, ok := c.SubjectStates(); ok {
		t.Error("SubjectStates of a value classify")
	}
	if _, ok := c.SubjectOp(); ok {
		t.Error("SubjectOp of a value classify")
	}
	c = &Classify{Subject: StatesSubject{States: []string{"a.s"}}}
	if c.SubjectType() != nil {
		t.Error("SubjectType of a state classify")
	}
	if states, ok := c.SubjectStates(); !ok || !reflect.DeepEqual(states, []string{"a.s"}) {
		t.Errorf("SubjectStates = %v, %v", states, ok)
	}
	if (&Classify{}).SubjectType() != nil {
		t.Error("SubjectType of an unresolved classify")
	}
	if _, err := UnmarshalClassifySubject([]byte(`{"kind":"ref","target":"server.Item"}`)); err == nil {
		t.Error("want error: a bare Type is not a ClassifySubject")
	}
}

func TestStateShapes(t *testing.T) {
	for shape, want := range map[StateShape]bool{StateList: true, StateNullable: true, StateSingle: false} {
		if shape.HasBuiltinClassify() != want {
			t.Errorf("%s.HasBuiltinClassify() = %v, want %v", shape, !want, want)
		}
	}
	if !reflect.DeepEqual(BuiltinCases(), []string{"Empty", "HasItems"}) {
		t.Errorf("BuiltinCases = %v", BuiltinCases())
	}
	b := roundTrip(t, State{DeclBase: DeclBase{Name: "token", Span: sp()}, Type: Optional(Ref("Token")), Shape: StateNullable, Element: Ref("Token")})
	if want := `{"kind":"state","name":"token","decorators":[],` + spJSON + `,"type":{"kind":"optional","of":{"kind":"ref","target":"Token"}},"shape":"nullable","element":{"kind":"ref","target":"Token"}}`; string(b) != want {
		t.Errorf("state\nwant %s\ngot  %s", want, b)
	}
	// 形は型の最も外側から決まる（docs/check.md「状態の形」の表）。
	shapes := []struct {
		t     Type
		shape StateShape
		elem  Type
	}{
		{Array(Ref("U")), StateList, Ref("U")},
		{Optional(Ref("U")), StateNullable, Ref("U")},
		{Ref("U"), StateSingle, Ref("U")},
		{Optional(Array(Ref("U"))), StateNullable, Array(Ref("U"))},
		{Array(Optional(Ref("U"))), StateList, Optional(Ref("U"))},
		{Record(String(), Int()), StateSingle, Record(String(), Int())},
	}
	for _, tt := range shapes {
		shape, elem := ShapeOf(tt.t)
		if shape != tt.shape || !TypeEqual(elem, tt.elem) {
			t.Errorf("ShapeOf(%s) = %s, %s; want %s, %s", tt.t, shape, elem, tt.shape, tt.elem)
		}
	}
	if !IsStandardClassify("list") || !IsStandardClassify("nullable") || IsStandardClassify("single") || IsStandardClassify("a.C") {
		t.Error("IsStandardClassify")
	}
}

func TestRoundTripArgValues(t *testing.T) {
	unionRoundTrip(t, ArgValue(LiteralValue{Literal: BooleanLiteral{Value: true}}), UnmarshalArgValue, `{"kind":"literal","literal":{"kind":"boolean","value":true}}`)
	unionRoundTrip(t, ArgValue(RefValue{Target: "$param:to"}), UnmarshalArgValue, `{"kind":"ref","target":"$param:to"}`)
	unionRoundTrip(t, ArgValue(RefsValue{Targets: []string{"a.X", "a.Y"}}), UnmarshalArgValue, `{"kind":"refs","targets":["a.X","a.Y"]}`)
	unionRoundTrip(t, ArgValue(VariantValue{Union: "s.Page", Arm: "Ready"}), UnmarshalArgValue, `{"kind":"variant","union":"s.Page","arm":"Ready"}`)
	unionRoundTrip(t, ArgValue(ListValue{}), UnmarshalArgValue, `{"kind":"list","items":[]}`)
	unionRoundTrip(t, ArgValue(ListValue{Items: []ArgValue{RefValue{Target: "a"}, ListValue{Items: []ArgValue{LiteralValue{Literal: IntLiteral{Value: 1}}}}}}), UnmarshalArgValue,
		`{"kind":"list","items":[{"kind":"ref","target":"a"},{"kind":"list","items":[{"kind":"literal","literal":{"kind":"int","value":1}}]}]}`)
}

func TestRoundTripDecls(t *testing.T) {
	// サンプルの Program のすべての宣言を、単独でも往復させる。
	p := sampleProgram()
	kinds := map[string]bool{}
	for _, ns := range p.Namespaces {
		for _, d := range ns.Decls {
			kinds[d.Kind()] = true
			b, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(b, []byte(`{"kind":"`+d.Kind()+`"`)) {
				t.Errorf("%s: kind must come first: %.60s", d.Common().Name, b)
			}
			got, err := UnmarshalDecl(b)
			if err != nil {
				t.Fatalf("%s: %v", d.Common().Name, err)
			}
			if !reflect.DeepEqual(got, d) {
				t.Errorf("%s: round trip differs\nwant %#v\ngot  %#v", d.Common().Name, d, got)
			}
			// 具体型への直接の読み込みも効く。
			roundTrip(t, reflect.ValueOf(d).Elem().Interface())
		}
	}
	for _, k := range []string{"model", "scalar", "union", "state", "exception", "effect", "classify", "decorator", "op"} {
		if !kinds[k] {
			t.Errorf("sample program lacks a %q decl", k)
		}
	}
}

func TestRoundTripProgramAndView(t *testing.T) {
	roundTrip(t, *sampleProgram())
	roundTrip(t, *sampleView())
	roundTrip(t, Doc{})
	roundTrip(t, Signature{})
	roundTrip(t, ViewedSignature{})
	roundTrip(t, Application{Decorator: "d", Span: sp()})
	roundTrip(t, Bind{Namespace: "a", State: "s", Target: BindTarget{Namespace: "b", Deep: true}, Span: sp()})
}

func TestEmptyProgramJSON(t *testing.T) {
	b, err := json.Marshal(Program{})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":"0.5","namespaces":[],"binds":[]}`
	if string(b) != want {
		t.Errorf("want %s\ngot  %s", want, b)
	}
	b, err = json.Marshal(View{Program: &Program{}})
	if err != nil {
		t.Fatal(err)
	}
	want = `{"version":"0.5","viewer":"","config":{"index":0,"language":"","base":"","out":"","dir":"","transpilers":[],"options":{},"seams":{}},"files":[],"namespaces":{},"program":{"version":"0.5","namespaces":[],"binds":[]},"ops":{}}`
	if string(b) != want {
		t.Errorf("want %s\ngot  %s", want, b)
	}
}

func TestEmptySlicesAndOptionals(t *testing.T) {
	b, err := json.Marshal(&Op{DeclBase: DeclBase{Name: "f", Span: sp()}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"kind":"op","name":"f","decorators":[],` + spJSON + `,"exported":false,"params":[],"returns":null,"effects":[],"reads":[],"carried":{"reads":[],"effects":[]},"throws":[],"body":null,` +
		`"signature":{"exceptions":[],"effects":[],"relations":[],"successEffects":[],"reads":[],"pure":false}}`
	if string(b) != want {
		t.Errorf("want %s\ngot  %s", want, b)
	}
	// 省略可能なフィールドは、値がなければキーごと省く。
	b, _ = json.Marshal(&Model{DeclBase: DeclBase{Name: "m", Span: sp()}})
	for _, key := range []string{"description", "extends", "synthesizedFrom", "null"} {
		if strings.Contains(string(b), key) {
			t.Errorf("model without %s must not mention it: %s", key, b)
		}
	}
	// 継承した model の記録は、空の列も [] で出す。
	b, _ = json.Marshal(&Model{DeclBase: DeclBase{Name: "m", Span: sp()}, Extends: &ModelExtends{Base: "b"}})
	if !strings.Contains(string(b), `"extends":{"base":"b","removed":[],"overridden":[]}`) {
		t.Errorf("model with extends: %s", b)
	}
	b, _ = json.Marshal(Field{Name: "f", Type: Int(), Span: sp()})
	if strings.Contains(string(b), "default") || strings.Contains(string(b), "description") {
		t.Errorf("field: %s", b)
	}
	b, _ = json.Marshal(&Exception{DeclBase: DeclBase{Name: "e", Span: sp()}})
	if strings.Contains(string(b), "value") {
		t.Errorf("exception without value: %s", b)
	}
	b, _ = json.Marshal(Application{Decorator: "d", Span: sp()})
	if strings.Contains(string(b), "via") {
		t.Errorf("application without via: %s", b)
	}
	b, _ = json.Marshal(CatchesClause{Mode: Recovers})
	if string(b) != `{"exceptions":[],"mode":"recovers"}` {
		t.Errorf("catches: %s", b)
	}
}

func TestNoNullExceptReturnsAndBody(t *testing.T) {
	b, err := json.Marshal(sampleView())
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, key := range []string{`"returns":null`, `"body":null`} {
		count += strings.Count(string(b), key)
	}
	// 値としての null だけを数える（"nullable" の中の null は数えない）。
	if n := strings.Count(string(b), ":null"); n != count {
		t.Errorf("found %d nulls, but only %d are returns/body", n, count)
	}
}

func TestOpsKeysSorted(t *testing.T) {
	v := &View{Program: &Program{}, Ops: map[string]*ViewedSignature{"z.op": {}, "a.op": {}, "m.op": {}, "a.aa": {}}}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	order := []string{`"a.aa"`, `"a.op"`, `"m.op"`, `"z.op"`}
	last := -1
	for _, k := range order {
		i := strings.Index(s, k)
		if i < 0 || i < last {
			t.Fatalf("keys not in lexicographic order: %s", b)
		}
		last = i
	}
	// 何度書いても同じ。
	for i := 0; i < 20; i++ {
		b2, _ := json.Marshal(v)
		if !bytes.Equal(b, b2) {
			t.Fatal("non-deterministic output")
		}
	}
}

func TestNamespacesKeysSorted(t *testing.T) {
	v := &View{Program: &Program{}, Namespaces: map[string][]NamespaceConfig{
		"z": {{Language: "go", Out: "gen", Dir: "gen/z"}}, "a.b": {{Language: "go", Out: "gen", Dir: "gen/a/b"}},
		"a": {{Language: "go", Out: "gen", Dir: "gen/a"}, {Language: "go", Out: "cmd", Dir: "cmd", Transpilers: []string{"t"}}},
	}}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, `"namespaces":{`)
	if i < 0 {
		t.Fatalf("no namespaces: %s", b)
	}
	last := i
	for _, k := range []string{`"a":[`, `"a.b":[`, `"z":[`} {
		j := strings.Index(s, k)
		if j < 0 || j < last {
			t.Fatalf("namespaces keys not in lexicographic order: %s", b)
		}
		last = j
	}
	if !strings.Contains(s, `"a":[{"index":0,"language":"go","base":"","out":"gen","dir":"gen/a","transpilers":[],"options":{},"seams":{}},{"index":0,"language":"go","base":"","out":"cmd","dir":"cmd","transpilers":["t"],"options":{},"seams":{}}]`) {
		t.Errorf("namespace config shape: %s", b)
	}
	var back View
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Namespaces) != 3 || len(back.Namespaces["a"]) != 2 || back.Namespaces["a.b"][0].Dir != "gen/a/b" || back.Namespaces["a"][1].Transpilers[0] != "t" {
		t.Errorf("namespaces not read back: %+v", back.Namespaces)
	}
}

func TestRelationAndArmCases(t *testing.T) {
	// Relation は kind だけ（docs/ir.md「decorator」）。目的語は常にその op のすべての効果。
	b, _ := json.Marshal(Relation{Kind: Keeps})
	if string(b) != `{"kind":"keeps"}` {
		t.Errorf("relation: %s", b)
	}
	var r Relation
	if err := json.Unmarshal([]byte(`{"kind":"prevents"}`), &r); err != nil || r.Kind != Prevents {
		t.Errorf("prevents: %v %+v", err, r)
	}
	var c ArmCases
	if err := json.Unmarshal([]byte(`"default"`), &c); err != nil || !c.Default {
		t.Errorf("default: %v %+v", err, c)
	}
	if err := json.Unmarshal([]byte(`"other"`), &c); err == nil {
		t.Error("want error for unknown cases string")
	}
	if err := json.Unmarshal([]byte(`["A"]`), &c); err != nil || c.Default || !reflect.DeepEqual(c.Cases, []string{"A"}) {
		t.Errorf("cases: %v %+v", err, c)
	}
}

func TestUnmarshalErrors(t *testing.T) {
	if _, err := UnmarshalType([]byte(`{"name":"string"}`)); err == nil {
		t.Error("want error for missing kind")
	}
	if _, err := UnmarshalType([]byte(`{"kind":"tuple"}`)); err == nil {
		t.Error("want error for unknown kind")
	}
	if _, err := UnmarshalDecl([]byte(`{"kind":"model","name":"m","fields":[{"name":"f","type":{"kind":"nope"}}]}`)); err == nil {
		t.Error("want error for nested unknown kind")
	}
	var p Program
	if err := json.Unmarshal([]byte(`[]`), &p); err == nil {
		t.Error("want error for non-object")
	}
}

func TestUnmarshalNullAndMissing(t *testing.T) {
	var op Op
	if err := json.Unmarshal([]byte(`{"kind":"op","name":"f","returns":null,"body":null}`), &op); err != nil {
		t.Fatal(err)
	}
	if op.Returns != nil || op.Body != nil || op.Params != nil {
		t.Errorf("want nil returns/body/params: %+v", op)
	}
	var m Model
	if err := json.Unmarshal([]byte(`{"kind":"model","name":"m","fields":[],"unknownKey":1}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Fields != nil {
		t.Error("[] must read as nil")
	}
}

func TestKinds(t *testing.T) {
	cases := map[string]interface{ Kind() string }{
		"core": CoreType{}, "ref": RefType{}, "optional": OptionalType{}, "array": ArrayType{}, "record": RecordType{}, "case": CaseType{}, "never": NeverType{},
		"throw": ThrowBody{},
		"type":  TypeParam{}, "any": AnyParam{},
		"const": ConstStmt{}, "return": ReturnStmt{},
		"atom": AtomExpr{},
		"name": NameAtom{}, "literal": LiteralAtom{}, "none": NoneAtom{},
		"string": StringLiteral{}, "int": IntLiteral{}, "float": FloatLiteral{}, "boolean": BooleanLiteral{}, "duration": DurationLiteral{},
		"refs": RefsValue{}, "variant": VariantValue{}, "list": ListValue{},
		"builtin": BuiltinTarget{}, "transition": TransitionTarget{}, "states": StatesSubject{}, "value": ValueSubject{},
		"model": &Model{}, "scalar": &Scalar{}, "union": &Union{}, "state": &State{}, "exception": &Exception{}, "effect": &Effect{}, "classify": &Classify{}, "decorator": &Decorator{}, "op": &Op{},
	}
	for want, v := range cases {
		if v.Kind() != want {
			t.Errorf("%T.Kind() = %q, want %q", v, v.Kind(), want)
		}
	}
	for _, v := range []interface{ Kind() string }{CallStmt{}, CallExpr{}, RecordAtom{}, LiteralValue{}, RefValue{}, MatchStmt{}, MatchExpr{}, NoneBody{},
		TypeSubject{}, OpSubject{}, StateAtom{}, ClassifyOwnedSubject{}} {
		switch v.(type) {
		case TypeSubject:
			if v.Kind() != "type" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		case OpSubject:
			if v.Kind() != "op" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		case StateAtom:
			if v.Kind() != "state" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		case ClassifyOwnedSubject:
			if v.Kind() != "classify" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		case CallStmt, CallExpr:
			if v.Kind() != "call" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		case MatchStmt, MatchExpr:
			if v.Kind() != "match" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		case RecordAtom:
			if v.Kind() != "record" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		case LiteralValue:
			if v.Kind() != "literal" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		case RefValue:
			if v.Kind() != "ref" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		case NoneBody:
			if v.Kind() != "none" {
				t.Errorf("%T: %q", v, v.Kind())
			}
		}
	}
}

// TestNamespaceConfigJSON は NamespaceConfig の JSON（docs/ir.md「その名前空間から見た IR」）: 項目の並び、options と seams の
// キーの辞書順（入れ子のテーブルも）、なければ {}。
func TestNamespaceConfigJSON(t *testing.T) {
	c := NamespaceConfig{Index: 5, Language: "go", Base: "go", Out: "gen/server", Dir: "gen/server/admin",
		Transpilers: []string{"go", "go-http"},
		Options:     map[string]map[string]any{"go-http": {"validate": true, "limits": map[string]any{"b": int64(2), "a": 1.5}}, "go": {}},
		Seams:       map[string]string{"trace.span": "go-trace", "rate.limited": "go-ratelimit"}}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"index":5,"language":"go","base":"go","out":"gen/server","dir":"gen/server/admin","transpilers":["go","go-http"],` +
		`"options":{"go":{},"go-http":{"limits":{"a":1.5,"b":2},"validate":true}},"seams":{"rate.limited":"go-ratelimit","trace.span":"go-trace"}}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	var back NamespaceConfig
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Index != 5 || back.Base != "go" || back.Seams["rate.limited"] != "go-ratelimit" || back.Options["go-http"]["validate"] != true {
		t.Errorf("読み戻した値が違う: %+v", back)
	}
	empty, _ := json.Marshal(NamespaceConfig{})
	if want := `{"index":0,"language":"","base":"","out":"","dir":"","transpilers":[],"options":{},"seams":{}}`; string(empty) != want {
		t.Errorf("got  %s\nwant %s", empty, want)
	}
}

// TestVersion05Fields は版 0.5 で足した項目（docs/ir.md「版」）の JSON の形: Decorator.targets の effect、
// Decorator.reads / effects、Param.rest、Op.carried。空の列は [] で出し、読み戻すと同じ値になる。
func TestVersion05Fields(t *testing.T) {
	dec := &Decorator{
		DeclBase: DeclBase{Name: "db.effects", Span: sp()},
		Params:   []Param{{Name: "targets", Type: AnyParam{Of: AnyEffect}, Rest: true}},
		Targets:  []DecoratorTarget{TargetOp, TargetEffect},
		Effects:  []string{"$param:targets"},
	}
	b := roundTrip(t, *dec)
	for _, want := range []string{
		`"params":[{"name":"targets","type":{"kind":"any","of":"effect"},"optional":false,"rest":true}]`,
		`"targets":["op","effect"]`,
		`"reads":[]`,
		`"effects":["$param:targets"]`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("decorator の JSON に %s がない: %s", want, b)
		}
	}
	// 節を持たない宣言も、reads と effects を [] で出す。
	b, _ = json.Marshal(&Decorator{DeclBase: DeclBase{Name: "d", Span: sp()}})
	if !strings.Contains(string(b), `"catches":[],"reads":[],"effects":[]`) {
		t.Errorf("節のない decorator: %s", b)
	}
	// 残りの引数でない Param は rest: false を出す（省かない）。
	b, _ = json.Marshal(Param{Name: "p", Type: AnyParam{Of: AnyState}})
	if string(b) != `{"name":"p","type":{"kind":"any","of":"state"},"optional":false,"rest":false}` {
		t.Errorf("param: %s", b)
	}
	op := &Op{
		DeclBase: DeclBase{Name: "store.placeOrder", Span: sp()},
		Reads:    []string{"store.users"},
		Effects:  []string{"store.e1"},
		Carried:  Carried{Reads: []string{"store.orders", "store.users"}, Effects: []string{"store.e1", "store.e2"}},
	}
	b = roundTrip(t, *op)
	if want := `"effects":["store.e1"],"reads":["store.users"],"carried":{"reads":["store.orders","store.users"],"effects":["store.e1","store.e2"]}`; !strings.Contains(string(b), want) {
		t.Errorf("op の JSON に %s がない: %s", want, b)
	}
	// 直に読む状態と直に持つ効果は、書いたものの後ろに運ばれたものを並べ、重複を最初の出現に寄せる。
	if got := op.DirectReads(); !reflect.DeepEqual(got, []string{"store.users", "store.orders"}) {
		t.Errorf("DirectReads = %v", got)
	}
	if got := op.DirectEffects(); !reflect.DeepEqual(got, []string{"store.e1", "store.e2"}) {
		t.Errorf("DirectEffects = %v", got)
	}
	if got := (&Op{}).DirectReads(); got != nil {
		t.Errorf("何も読まない op の DirectReads = %v", got)
	}
	// carried は単独でも、空の列を [] で出す。
	b, _ = json.Marshal(Carried{})
	if string(b) != `{"reads":[],"effects":[]}` {
		t.Errorf("carried: %s", b)
	}
}
