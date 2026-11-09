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
		Subject:  NameAtom{Path: []string{"u"}, Span: sp()},
		Classify: "s.C",
		Arms: []MatchArm{
			{Cases: ArmCases{Cases: []string{"A", "B"}}, Body: CallExpr{Call: call}, Span: sp()},
			{Cases: DefaultArm(), Body: NoneBody{}, Span: sp()},
		},
		Span: sp(),
	}
	matchJSON := `{"subject":{"kind":"name","path":["u"],` + spJSON + `},"classify":"s.C","arms":[` +
		`{"cases":["A","B"],"body":{"kind":"call","call":` + callJSON + `},` + spJSON + `},` +
		`{"cases":"default","body":{"kind":"none"},` + spJSON + `}],` + spJSON + `}`

	unionRoundTrip(t, Expr(CallExpr{Call: call}), UnmarshalExpr, `{"kind":"call","call":`+callJSON+`}`)
	unionRoundTrip(t, Expr(MatchExpr{Match: match}), UnmarshalExpr, `{"kind":"match","match":`+matchJSON+`}`)
	unionRoundTrip(t, Expr(AtomExpr{Atom: NoneAtom{Span: sp()}}), UnmarshalExpr, `{"kind":"atom","atom":{"kind":"none",`+spJSON+`}}`)

	unionRoundTrip(t, ArmBody(CallExpr{Call: call}), UnmarshalArmBody, `{"kind":"call","call":`+callJSON+`}`)
	unionRoundTrip(t, ArmBody(NoneBody{}), UnmarshalArmBody, `{"kind":"none"}`)

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
	want := `{"version":"0.1","namespaces":[],"binds":[]}`
	if string(b) != want {
		t.Errorf("want %s\ngot  %s", want, b)
	}
	b, err = json.Marshal(View{Program: &Program{}})
	if err != nil {
		t.Fatal(err)
	}
	want = `{"version":"0.1","viewer":"","config":{"language":"","out":"","dir":"","plugins":[]},"namespaces":{},"program":{"version":"0.1","namespaces":[],"binds":[]},"ops":{}}`
	if string(b) != want {
		t.Errorf("want %s\ngot  %s", want, b)
	}
}

func TestEmptySlicesAndOptionals(t *testing.T) {
	b, err := json.Marshal(&Op{DeclBase: DeclBase{Name: "f", Span: sp()}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"kind":"op","name":"f","decorators":[],` + spJSON + `,"params":[],"returns":null,"effects":[],"throws":[],"body":null,` +
		`"signature":{"exceptions":[],"effects":[],"relations":[],"successEffects":[]}}`
	if string(b) != want {
		t.Errorf("want %s\ngot  %s", want, b)
	}
	// 省略可能なフィールドは、値がなければキーごと省く。
	b, _ = json.Marshal(&Model{DeclBase: DeclBase{Name: "m", Span: sp()}})
	for _, key := range []string{"description", "derivedFrom", "synthesizedFrom", "null"} {
		if strings.Contains(string(b), key) {
			t.Errorf("model without %s must not mention it: %s", key, b)
		}
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
	if n := strings.Count(string(b), "null"); n != count {
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
	v := &View{Program: &Program{}, Namespaces: map[string]NamespaceConfig{
		"z": {Language: "go", Out: "gen", Dir: "gen/z"}, "a.b": {Language: "go", Out: "gen", Dir: "gen/a/b"}, "a": {Language: "go", Out: "gen", Dir: "gen/a"},
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
	for _, k := range []string{`"a":{`, `"a.b":{`, `"z":{`} {
		j := strings.Index(s, k)
		if j < 0 || j < last {
			t.Fatalf("namespaces keys not in lexicographic order: %s", b)
		}
		last = j
	}
	if !strings.Contains(s, `"a":{"language":"go","out":"gen","dir":"gen/a","plugins":[]}`) {
		t.Errorf("namespace config shape: %s", b)
	}
	var back View
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Namespaces) != 3 || back.Namespaces["a.b"].Dir != "gen/a/b" {
		t.Errorf("namespaces not read back: %+v", back.Namespaces)
	}
}

func TestRelationEffectsAndArmCases(t *testing.T) {
	var r Relation
	if err := json.Unmarshal([]byte(`{"kind":"prevents","effects":"all"}`), &r); err != nil || !r.Effects.All {
		t.Errorf("all: %v %+v", err, r)
	}
	if err := json.Unmarshal([]byte(`{"kind":"keeps","effects":["a","b"]}`), &r); err != nil || r.Effects.All || !reflect.DeepEqual(r.Effects.Effects, []string{"a", "b"}) {
		t.Errorf("list: %v %+v", err, r)
	}
	if err := json.Unmarshal([]byte(`{"kind":"keeps","effects":"some"}`), &r); err == nil {
		t.Error("want error for unknown effects string")
	}
	b, _ := json.Marshal(Relation{Kind: Keeps})
	if string(b) != `{"kind":"keeps","effects":[]}` {
		t.Errorf("empty relation: %s", b)
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
		"core": CoreType{}, "ref": RefType{}, "optional": OptionalType{}, "array": ArrayType{}, "record": RecordType{}, "case": CaseType{},
		"type": TypeParam{}, "any": AnyParam{},
		"const": ConstStmt{}, "return": ReturnStmt{},
		"atom": AtomExpr{},
		"name": NameAtom{}, "literal": LiteralAtom{}, "none": NoneAtom{},
		"string": StringLiteral{}, "int": IntLiteral{}, "float": FloatLiteral{}, "boolean": BooleanLiteral{}, "duration": DurationLiteral{},
		"refs": RefsValue{}, "variant": VariantValue{}, "list": ListValue{},
		"model": &Model{}, "scalar": &Scalar{}, "union": &Union{}, "state": &State{}, "exception": &Exception{}, "effect": &Effect{}, "classify": &Classify{}, "decorator": &Decorator{}, "op": &Op{},
	}
	for want, v := range cases {
		if v.Kind() != want {
			t.Errorf("%T.Kind() = %q, want %q", v, v.Kind(), want)
		}
	}
	for _, v := range []interface{ Kind() string }{CallStmt{}, CallExpr{}, RecordAtom{}, LiteralValue{}, RefValue{}, MatchStmt{}, MatchExpr{}, NoneBody{}} {
		switch v.(type) {
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
