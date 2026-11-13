package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

const (
	schemaPath = "../../../schema/ir.schema.json"
	goldenDir  = "../../../ir/testdata"
)

// TestNoDrift は、生成し直した結果がコミット済みの schema/ir.schema.json と
// バイト単位で一致することを確かめる（docs/testing.md「生成物の一致」）。更新は go generate ./schema/...。
func TestNoDrift(t *testing.T) {
	got, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("schema/ir.schema.json is stale: run `go generate ./schema/...`")
	}
	// 二度生成しても同じ（決定性）。
	again, _ := Generate()
	if !bytes.Equal(got, again) {
		t.Fatal("Generate is not deterministic")
	}
}

func loadValidator(t *testing.T) *validator {
	t.Helper()
	b, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	root, err := decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return &validator{root: root.(map[string]any)}
}

func readJSON(t *testing.T, path string) any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := decode(b)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return v
}

// TestGoldensValidate は ir/testdata のゴールデンがスキーマのルート（Program か View）に合うことを確かめる。
// ゴールデンは docs/ir.md「構造」のすべての種類の節を含む（ir/sample_test.go）。
func TestGoldensValidate(t *testing.T) {
	v := loadValidator(t)
	files, err := filepath.Glob(filepath.Join(goldenDir, "*.golden.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no goldens in %s (%v)", goldenDir, err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			inst := readJSON(t, f)
			if err := v.validate(v.root, inst, "$"); err != nil {
				t.Error(err)
			}
			// それぞれの定義にも直接合う（ルートの oneOf がたまたま通ったのではない）。
			def := map[string]string{"program.golden.json": "Program", "view.golden.json": "View"}[filepath.Base(f)]
			if def != "" {
				if err := v.validate(ref(def), inst, "$"); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

// TestGoldensCoverKinds は、ゴールデンがすべてのユニオンのすべての変種を一つ以上含むことを確かめる
// （含まなければスキーマの検査がその変種を通っていない）。
func TestGoldensCoverKinds(t *testing.T) {
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if k, ok := x["kind"].(string); ok {
				seen[k] = true
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(readJSON(t, filepath.Join(goldenDir, "program.golden.json")))
	walk(readJSON(t, filepath.Join(goldenDir, "view.golden.json")))
	for _, u := range unions {
		for _, vt := range u.variants {
			kind := reflect.New(vt).Interface().(kinder).Kind()
			if !seen[kind] {
				t.Errorf("goldens contain no %s of kind %q", u.iface.Name(), kind)
			}
		}
	}
}

// TestNegative は、形の違う JSON をスキーマが退けることを確かめる。
func TestNegative(t *testing.T) {
	v := loadValidator(t)
	cases := []struct {
		name, def, json string
		ok              bool
	}{
		{"core type", "Type", `{"kind":"core","name":"string"}`, true},
		{"unknown kind", "Type", `{"kind":"nope"}`, false},
		{"missing kind", "Type", `{"target":"x"}`, false},
		{"bad core name", "Type", `{"kind":"core","name":"bytes"}`, false},
		{"extra property", "Type", `{"kind":"ref","target":"x","extra":1}`, false},
		{"nested optional", "Type", `{"kind":"optional","of":{"kind":"array","of":{"kind":"core","name":"int"}}}`, true},
		{"record", "Type", `{"kind":"record","key":{"kind":"core","name":"string"},"value":{"kind":"ref","target":"a.B"}}`, true},
		{"case type missing cases", "Type", `{"kind":"case","classify":"c","of":{"kind":"core","name":"int"}}`, false},
		{"int literal", "Literal", `{"kind":"int","value":3}`, true},
		{"int literal float", "Literal", `{"kind":"int","value":3.5}`, false},
		{"float literal integral", "Literal", `{"kind":"float","value":1}`, true},
		{"duration integer", "Literal", `{"kind":"duration","value":300000000000}`, true},
		{"duration float", "Literal", `{"kind":"duration","value":1.5}`, false},
		{"duration string", "Literal", `{"kind":"duration","value":"5m"}`, false},
		{"boolean literal", "Literal", `{"kind":"boolean","value":true}`, true},
		{"boolean literal as string", "Literal", `{"kind":"boolean","value":"true"}`, false},
		// Relation は kind だけ（docs/ir.md「decorator」）。effects（"all" か名前の列）はもうない。
		{"relation prevents", "Relation", `{"kind":"prevents"}`, true},
		{"relation keeps", "Relation", `{"kind":"keeps"}`, true},
		{"relation with effects all", "Relation", `{"kind":"prevents","effects":"all"}`, false},
		{"relation with effects list", "Relation", `{"kind":"keeps","effects":["a.b"]}`, false},
		{"relation bad kind", "Relation", `{"kind":"stops"}`, false},
		{"relation missing kind", "Relation", `{}`, false},
		// Namespace.decorators は必須（空でも []。docs/ir.md「Program」）。
		{"namespace with decorators", "Namespace", `{"name":"a","decorators":[{"decorator":"m.ctx","args":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}],"decls":[]}`, true},
		{"namespace empty decorators", "Namespace", `{"name":"a","decorators":[],"decls":[]}`, true},
		{"namespace missing decorators", "Namespace", `{"name":"a","decls":[]}`, false},
		{"arm cases default", "ArmCases", `"default"`, true},
		{"arm cases list", "ArmCases", `["A","B"]`, true},
		{"arm cases other word", "ArmCases", `"other"`, false},
		// 状態を分類で表す決定の形（docs/ir.md「state / exception / effect」「classify」「配線」）。
		{"state shape nullable", "StateShape", `"nullable"`, true},
		{"state shape unknown", "StateShape", `"maybe"`, false},
		{"effect target builtin", "EffectTarget", `{"kind":"builtin","state":"a.s","operation":"delete"}`, true},
		{"effect target builtin bad operation", "EffectTarget", `{"kind":"builtin","state":"a.s","operation":"clear"}`, false},
		{"effect target builtin with cases", "EffectTarget", `{"kind":"builtin","state":"a.s","operation":"put","from":"any"}`, false},
		{"effect target transition", "EffectTarget", `{"kind":"transition","of":"a.C","from":["A","B"],"to":"any"}`, true},
		{"effect target transition missing to", "EffectTarget", `{"kind":"transition","of":"a.C","from":["A"]}`, false},
		{"effect target transition bad word", "EffectTarget", `{"kind":"transition","of":"a.C","from":"all","to":["A"]}`, false},
		{"effect target old shape", "EffectTarget", `{"state":"a.s","operation":"put"}`, false},
		{"case set any", "CaseSet", `"any"`, true},
		{"case set list", "CaseSet", `["Empty","HasItems"]`, true},
		{"case set default", "CaseSet", `"default"`, false},
		{"classify subject type", "ClassifySubject", `{"kind":"type","type":{"kind":"ref","target":"a.M"}}`, true},
		{"classify subject states", "ClassifySubject", `{"kind":"states","states":["a.carts","a.orders"]}`, true},
		{"classify subject bare type", "ClassifySubject", `{"kind":"ref","target":"a.M"}`, false},
		{"classify subject states as type", "ClassifySubject", `{"kind":"states","type":{"kind":"ref","target":"a.M"}}`, false},
		{"match subject value", "MatchSubject", `{"kind":"value","atom":{"kind":"none","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}}`, true},
		{"match subject state", "MatchSubject", `{"kind":"state","state":"a.carts"}`, true},
		{"match subject classify", "MatchSubject", `{"kind":"classify"}`, true},
		{"match subject classify with state", "MatchSubject", `{"kind":"classify","state":"a.carts"}`, false},
		{"match subject bare atom", "MatchSubject", `{"kind":"name","path":["x"],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		{"arm body none", "ArmBody", `{"kind":"none"}`, true},
		{"arm body none with span", "ArmBody", `{"kind":"none","span":{}}`, false},
		{"atom none", "Atom", `{"kind":"none","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"atom none without span", "Atom", `{"kind":"none"}`, false},
		{"pos line string", "Pos", `{"line":"1","col":1}`, false},
		{"param type any", "ParamType", `{"kind":"any","of":"effect","on":{"state":"db","operation":"post"}}`, true},
		{"param type any bad of", "ParamType", `{"kind":"any","of":"field"}`, false},
		{"param type bad operation", "ParamType", `{"kind":"any","of":"effect","on":{"state":"db","operation":"get"}}`, false},
		{"arg value variant", "ArgValue", `{"kind":"variant","union":"a.U","arm":"Ready"}`, true},
		{"arg value list nested", "ArgValue", `{"kind":"list","items":[{"kind":"ref","target":"x"},{"kind":"refs","targets":[]}]}`, true},
		{"arg value literal null", "ArgValue", `{"kind":"literal","literal":null}`, false},
		{"namespace config", "NamespaceConfig", `{"language":"go","out":"gen","dir":"gen","plugins":[]}`, true},
		{"namespace config missing plugins", "NamespaceConfig", `{"language":"go","out":"gen","dir":"gen"}`, false},
		{"bind", "Bind", `{"namespace":"browser","state":"db","target":{"namespace":"server","deep":true},"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"bind deep string", "Bind", `{"namespace":"browser","state":"db","target":{"namespace":"server","deep":"true"},"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		// プラグインのプロトコル（docs/plugin-protocol.md「要求と応答」「起動」）。
		{"plugin error protocol", "PluginError", `{"error":{"code":"protocol","message":"m"}}`, true},
		{"plugin error unsupported", "PluginError", `{"error":{"code":"unsupported","message":"m"}}`, true},
		{"plugin error other code", "PluginError", `{"error":{"code":"crash","message":"m"}}`, false},
		{"plugin error no message", "PluginError", `{"error":{"code":"protocol"}}`, false},
		{"emit response", "EmitResponse", `{"files":[{"path":"a/b.go","content":"x"}],"warnings":[{"at":"a.b","message":"m"}]}`, true},
		{"emit response with errors", "EmitResponse", `{"files":[],"warnings":[],"errors":[{"code":"CLI-1","at":"a.b","message":"m"}]}`, true},
		{"emit response missing warnings", "EmitResponse", `{"files":[]}`, false},
		{"check response", "CheckResponse", `{"diagnostics":[{"severity":"warning","code":"PAGE-1","message":"m","at":"a.b"}]}`, true},
		{"check response no severity", "CheckResponse", `{"diagnostics":[{"code":"PAGE-1","message":"m","at":"a.b"}]}`, false},
		{"check response bad severity", "CheckResponse", `{"diagnostics":[{"severity":"info","code":"PAGE-1","message":"m","at":"a.b"}]}`, false},
		{"import request", "ImportRequest", `{"protocol":"0.1","direction":"import","format":"openapi","files":[{"path":"openapi.yaml","content":""}]}`, true},
		{"import request wrong direction", "ImportRequest", `{"protocol":"0.1","direction":"emit","format":"openapi","files":[]}`, false},
		{"check request wrong protocol", "CheckRequest", `{"protocol":"0.2","direction":"check","view":{}}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inst, err := decode([]byte(c.json))
			if err != nil {
				t.Fatal(err)
			}
			err = v.validate(ref(c.def), inst, "$")
			if c.ok && err != nil {
				t.Errorf("want valid, got: %v", err)
			}
			if !c.ok && err == nil {
				t.Errorf("want invalid, but validated")
			}
		})
	}
}

// TestNegativeMutations は、ゴールデンを一箇所だけ壊したものを退けることを確かめる。
func TestNegativeMutations(t *testing.T) {
	v := loadValidator(t)
	program := func() map[string]any {
		return readJSON(t, filepath.Join(goldenDir, "program.golden.json")).(map[string]any)
	}
	view := func() map[string]any {
		return readJSON(t, filepath.Join(goldenDir, "view.golden.json")).(map[string]any)
	}
	firstDecl := func(p map[string]any, kind string) map[string]any {
		for _, ns := range p["namespaces"].([]any) {
			for _, d := range ns.(map[string]any)["decls"].([]any) {
				if dm := d.(map[string]any); dm["kind"] == kind {
					return dm
				}
			}
		}
		t.Fatalf("no %s in golden", kind)
		return nil
	}
	cases := []struct {
		name string
		inst func() any
	}{
		{"program without version", func() any { p := program(); delete(p, "version"); return p }},
		{"program with other version", func() any { p := program(); p["version"] = "0.2"; return p }},
		{"program with extra key", func() any { p := program(); p["extra"] = 1; return p }},
		{"program binds null", func() any { p := program(); p["binds"] = nil; return p }},
		{"op without returns", func() any { p := program(); delete(firstDecl(p, "op"), "returns"); return p }},
		{"op without body", func() any { p := program(); delete(firstDecl(p, "op"), "body"); return p }},
		{"op body as string", func() any { p := program(); firstDecl(p, "op")["body"] = "leaf"; return p }},
		{"op without signature", func() any { p := program(); delete(firstDecl(p, "op"), "signature"); return p }},
		{"model field default null", func() any {
			p := program()
			firstDecl(p, "model")["fields"].([]any)[0].(map[string]any)["default"] = nil
			return p
		}},
		{"decl description null", func() any { p := program(); firstDecl(p, "scalar")["description"] = nil; return p }},
		{"exception value null", func() any { p := program(); firstDecl(p, "exception")["value"] = nil; return p }},
		{"decl without span", func() any { p := program(); delete(firstDecl(p, "state"), "span"); return p }},
		{"decl without decorators", func() any { p := program(); delete(firstDecl(p, "state"), "decorators"); return p }},
		{"union bad variant", func() any { p := program(); firstDecl(p, "union")["variant"] = "mixed"; return p }},
		{"state bad shape", func() any { p := program(); firstDecl(p, "state")["shape"] = "map"; return p }},
		{"decorator bad target", func() any {
			p := program()
			firstDecl(p, "decorator")["targets"] = []any{"op", "union"}
			return p
		}},
		{"view without ops", func() any { w := view(); delete(w, "ops"); return w }},
		{"view op signature without params", func() any {
			w := view()
			for _, sig := range w["ops"].(map[string]any) {
				delete(sig.(map[string]any), "params")
				break
			}
			return w
		}},
		{"view namespaces as array", func() any { w := view(); w["namespaces"] = []any{}; return w }},
		{"view config missing dir", func() any { w := view(); delete(w["config"].(map[string]any), "dir"); return w }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := v.validate(v.root, c.inst(), "$"); err == nil {
				t.Error("want invalid, but validated")
			}
		})
	}
}

// TestSchemaMatchesGo は、スキーマの手書きの部分（ユニオン、kind、特別な型）と Go の ir パッケージの
// 実際の JSON が一致することを reflect で確かめる:
//   - $defs のオブジェクトごとに、Go のゼロ値を json.Marshal したキーの集合が required と一致する
//     （omitempty のないフィールドは常に出て、omitempty のものはゼロ値では出ない）。
//   - 変種のゼロ値の kind が、スキーマの const と一致する。
//   - 列のフィールドはゼロ値でも [] で出る（null にならない）。
func TestSchemaMatchesGo(t *testing.T) {
	v := loadValidator(t)
	g := &gen{defs: map[string]any{}, kinds: map[reflect.Type]string{}, types: map[string]reflect.Type{}}
	if err := g.build(); err != nil {
		t.Fatal(err)
	}
	defs := v.root["$defs"].(map[string]any)
	names := make([]string, 0, len(g.types))
	for n := range g.types {
		names = append(names, n)
	}
	sort.Strings(names)
	checked := 0
	for _, name := range names {
		rt := g.types[name]
		if rt.Kind() != reflect.Struct {
			continue
		}
		if _, special := specials[rt]; special {
			continue
		}
		def := defs[name].(map[string]any)
		zero := reflect.New(rt).Interface()
		b, err := json.Marshal(zero)
		if err != nil {
			t.Errorf("%s: marshal zero value: %v", name, err)
			continue
		}
		inst, err := decode(b)
		if err != nil {
			t.Fatal(err)
		}
		obj := inst.(map[string]any)
		gotKeys := make([]string, 0, len(obj))
		for k := range obj {
			gotKeys = append(gotKeys, k)
		}
		sort.Strings(gotKeys)
		var want []string
		for _, r := range toList(def["required"]) {
			want = append(want, r.(string))
		}
		sort.Strings(want)
		// Program / View は空の version を "0.1" に補うので、const と一致する。
		if !reflect.DeepEqual(gotKeys, want) {
			t.Errorf("%s: zero value marshals keys %v, schema requires %v", name, gotKeys, want)
		}
		if kind, ok := g.kinds[rt]; ok {
			if obj["kind"] != kind {
				t.Errorf("%s: zero value has kind %v, schema const is %q", name, obj["kind"], kind)
			}
		}
		if name == "Op" {
			// returns と body は null（void と葉）。null を出すのはこの二つだけ。
			if obj["returns"] != nil || obj["body"] != nil {
				t.Errorf("Op zero value: returns=%v body=%v, want null", obj["returns"], obj["body"])
			}
		}
		// ir の空の列は [] で出る（docs/ir.md「JSON」）。required の列のフィールドは null ではなく配列。
		// plugin の型は MarshalJSON を持たず、nil の列を plugin.Run が [] に直すので、ここでは見ない。
		if rt.PkgPath() == typeOf[ir.Program]().PkgPath() {
			for k, val := range obj {
				ps := def["properties"].(map[string]any)[k].(map[string]any)
				if ps["type"] == "array" && val == nil {
					t.Errorf("%s.%s: zero value marshals null, schema wants an array", name, k)
				}
			}
		}
		checked++
	}
	if checked < 60 {
		t.Errorf("only %d struct definitions checked", checked)
	}
	// ゴールデンを Go で読み戻して書き直しても、同じくスキーマに合う（Go の読み書きとスキーマが同じ形）。
	for _, c := range []struct {
		file string
		v    any
	}{{"program.golden.json", &ir.Program{}}, {"view.golden.json", &ir.View{}}} {
		raw, err := os.ReadFile(filepath.Join(goldenDir, c.file))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, c.v); err != nil {
			t.Fatal(err)
		}
		again, err := json.Marshal(c.v)
		if err != nil {
			t.Fatal(err)
		}
		inst, _ := decode(again)
		if err := v.validate(v.root, inst, c.file); err != nil {
			t.Error(err)
		}
	}
}

// TestEnumsMatchGo は、enums の表の値が ir パッケージの定数と一致することを、
// 定数をスキーマの型に代入して確かめる（定数が増えたら表に足す）。
func TestEnumsMatchGo(t *testing.T) {
	want := map[reflect.Type][]string{
		typeOf[ir.CoreName]():        {string(ir.CoreString), string(ir.CoreBoolean), string(ir.CoreInt), string(ir.CoreFloat), string(ir.CoreTime), string(ir.CoreDuration)},
		typeOf[ir.UnionVariant]():    {string(ir.UnionArms), string(ir.UnionSet)},
		typeOf[ir.StateShape]():      {string(ir.StateList), string(ir.StateNullable), string(ir.StateSingle)},
		typeOf[ir.Operation]():       {string(ir.OpPost), string(ir.OpPut), string(ir.OpDelete)},
		typeOf[ir.DecoratorTarget](): {string(ir.TargetOp), string(ir.TargetField), string(ir.TargetState), string(ir.TargetException), string(ir.TargetScalar), string(ir.TargetModel), string(ir.TargetNamespace)},
		typeOf[ir.AnyOf]():           {string(ir.AnyException), string(ir.AnyEffect), string(ir.AnyState), string(ir.AnyOp), string(ir.AnyModel), string(ir.AnyVariant)},
		typeOf[ir.RelationKind]():    {string(ir.Prevents), string(ir.Keeps)},
		typeOf[ir.CatchMode]():       {string(ir.Recovers), string(ir.Halts), string(ir.Ignores)},
		typeOf[ir.EffectState]():     {string(ir.Prevented), string(ir.Kept), string(ir.Unknown)},
	}
	if len(want) != len(enums) {
		t.Errorf("enums table has %d entries, test knows %d", len(enums), len(want))
	}
	for rt, w := range want {
		if got := enums[rt]; !reflect.DeepEqual(got, w) {
			t.Errorf("%s: enums table %v, constants %v", rt.Name(), got, w)
		}
	}
}
