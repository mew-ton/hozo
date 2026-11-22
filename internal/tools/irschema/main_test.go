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
		// 型の種類 record は、なくした（docs/ir.md「版」の 0.14）。インデックスの model は、Model.index
		{"record type is gone", "Type", `{"kind":"record","key":{"kind":"core","name":"string"},"value":{"kind":"ref","target":"a.B"}}`, false},
		{"model index", "ModelIndex", `{"name":"name","key":{"kind":"core","name":"string"},"value":{"kind":"ref","target":"a.B"},"span":{"file":"a.hozo","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"model index missing value", "ModelIndex", `{"name":"name","key":{"kind":"core","name":"string"},"span":{"file":"a.hozo","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
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
		// Namespace は decorators を持たない（デコレータは名前空間に付けられない。docs/ir.md「Program」）。
		{"namespace", "Namespace", `{"name":"a","decls":[]}`, true},
		{"namespace with decorators", "Namespace", `{"name":"a","decorators":[],"decls":[]}`, false},
		// アームのケースは、いつもケース名の列。else のアームは else: true で、cases は、それが受けるケース（docs/ir.md「配線」の MatchArm）。
		{"match arm", "MatchArm", `{"cases":["A","B"],"else":false,"body":{"kind":"none"},"narrowed":[],"span":{"file":"a.hozo","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"match arm else", "MatchArm", `{"cases":["C"],"else":true,"body":{"kind":"none"},"narrowed":[],"span":{"file":"a.hozo","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"match arm cases default", "MatchArm", `{"cases":"default","else":false,"body":{"kind":"none"},"narrowed":[],"span":{"file":"a.hozo","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		{"match arm without else", "MatchArm", `{"cases":["A"],"body":{"kind":"none"},"narrowed":[],"span":{"file":"a.hozo","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
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
		// 0.3: subject の state はなくなり、状態を名前で書いた対象は value の atom が state（docs/ir.md「版」）。
		{"match subject state", "MatchSubject", `{"kind":"state","state":"a.carts"}`, false},
		{"match subject value state atom", "MatchSubject", `{"kind":"value","atom":{"kind":"state","state":"a.carts","path":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}}`, true},
		{"atom state without path", "Atom", `{"kind":"state","state":"a.carts","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		{"classify subject op", "ClassifySubject", `{"kind":"op","op":"a.find","type":{"kind":"optional","of":{"kind":"ref","target":"a.M"}}}`, true},
		{"classify subject op without type", "ClassifySubject", `{"kind":"op","op":"a.find"}`, false},
		{"case typing", "CaseTyping", `{"type":{"kind":"ref","target":"a.M"},"by":"presence","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"case typing with state", "CaseTyping", `{"state":"a.s","type":{"kind":"ref","target":"a.M"},"by":"predicate","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		// 版 0.7: 残りのケース `対象 as else`（docs/ir.md「版」）
		{"case typing else", "CaseTyping", `{"type":{"kind":"optional","of":{"kind":"ref","target":"a.M"}},"by":"else","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"case typing bad by", "CaseTyping", `{"type":{"kind":"ref","target":"a.M"},"by":"guess","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		{"classify case without as", "ClassifyCase", `{"name":"A","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		{"decided by", "DecidedBy", `"type"`, true},
		{"decided by unknown", "DecidedBy", `"value"`, false},
		{"type never", "Type", `{"kind":"never"}`, true},
		{"type never with of", "Type", `{"kind":"never","of":{"kind":"core","name":"int"}}`, false},
		{"arm body atom", "ArmBody", `{"kind":"atom","atom":{"kind":"name","path":["x"],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}}`, true},
		{"arm body throw", "ArmBody", `{"kind":"throw","exception":"a.E","args":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"arm body throw with args", "ArmBody", `{"kind":"throw","exception":"a.E","args":[{"param":"wait","value":{"kind":"none","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}}],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"arm body throw without args", "ArmBody", `{"kind":"throw","exception":"a.E","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		// 版 0.6: 呼び出し先、reads / effects の要素、引数を持つ exception / effect / read、repeatable（docs/ir.md「版」）
		{"callee op", "Callee", `{"kind":"op","op":"a.f"}`, true},
		{"callee read", "Callee", `{"kind":"read","read":"a.byId"}`, true},
		{"callee effect named", "Callee", `{"kind":"effect","effect":{"kind":"named","effect":"a.add"}}`, true},
		{"callee effect direct", "Callee", `{"kind":"effect","effect":{"kind":"direct","state":"a.rows","operation":"post"}}`, true},
		{"callee effect bare name", "Callee", `{"kind":"effect","effect":"a.add"}`, false},
		{"callee state", "Callee", `{"kind":"state","state":"a.rows"}`, false},
		{"call with callee", "Call", `{"callee":{"kind":"read","read":"a.byId"},"args":[],"bound":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, true},
		{"call with op (old shape)", "Call", `{"op":"a.f","args":[],"bound":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		{"read ref named", "ReadRef", `{"kind":"named","read":"a.byId"}`, true},
		{"read ref direct", "ReadRef", `{"kind":"direct","state":"a.rows"}`, true},
		{"read ref bare name", "ReadRef", `"a.rows"`, false},
		{"effect ref direct bad operation", "EffectRef", `{"kind":"direct","state":"a.rows","operation":"get"}`, false},
		{"decl read", "Decl", `{"kind":"read","name":"a.byId","decorators":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}},"params":[],"returns":{"kind":"core","name":"int"},"state":"a.rows"}`, true},
		{"decl read without returns", "Decl", `{"kind":"read","name":"a.byId","decorators":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}},"params":[],"state":"a.rows"}`, false},
		{"decl exception with params", "Decl", `{"kind":"exception","name":"a.E","decorators":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}},"params":[{"name":"wait","type":{"kind":"core","name":"duration"},"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}]}`, true},
		{"decl exception with value (old shape)", "Decl", `{"kind":"exception","name":"a.E","decorators":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}},"params":[],"value":{"kind":"ref","target":"a.M"}}`, false},
		{"decl exception without params", "Decl", `{"kind":"exception","name":"a.E","decorators":[],"span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		{"param type any read", "ParamType", `{"kind":"any","of":"read"}`, false},
		{"arm body throw without exception", "ArmBody", `{"kind":"throw","span":{"file":"f","start":{"line":1,"col":1},"end":{"line":1,"col":2}}}`, false},
		{"narrowing", "Narrowing", `{"type":{"kind":"ref","target":"a.M"}}`, true},
		{"narrowing with state", "Narrowing", `{"state":"a.s","type":{"kind":"ref","target":"a.M"}}`, true},
		{"narrowing without type", "Narrowing", `{"state":"a.s"}`, false},
		{"model extends", "ModelExtends", `{"base":"a.B","removed":["x"],"overridden":[]}`, true},
		{"model extends old shape", "ModelExtends", `{"base":"a.B","omit":["x"]}`, false},
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
		{"namespace config", "NamespaceConfig", `{"index":1,"language":"go","base":"go","out":"gen","dir":"gen","transpilers":["go","go-cli"],"options":{"go-cli":{"x":1,"y":{"z":[true,"s"]}}},"seams":{"rate.limited":"go-ratelimit"}}`, true},
		{"namespace config empty options and seams", "NamespaceConfig", `{"index":1,"language":"go","base":"go","out":"gen","dir":"gen","transpilers":["go"],"options":{},"seams":{}}`, true},
		{"namespace config missing transpilers", "NamespaceConfig", `{"index":1,"language":"go","base":"go","out":"gen","dir":"gen","options":{},"seams":{}}`, false},
		{"namespace config missing base", "NamespaceConfig", `{"index":1,"language":"go","out":"gen","dir":"gen","transpilers":["go"],"options":{},"seams":{}}`, false},
		{"namespace config missing seams", "NamespaceConfig", `{"index":1,"language":"go","base":"go","out":"gen","dir":"gen","transpilers":["go"],"options":{}}`, false},
		{"namespace config options not a table of tables", "NamespaceConfig", `{"index":1,"language":"go","base":"go","out":"gen","dir":"gen","transpilers":["go"],"options":{"go":1},"seams":{}}`, false},
		{"namespace config seams value not a string", "NamespaceConfig", `{"index":1,"language":"go","base":"go","out":"gen","dir":"gen","transpilers":["go"],"options":{},"seams":{"a.b":1}}`, false},
		{"namespace config with plugins", "NamespaceConfig", `{"index":1,"language":"go","base":"go","out":"gen","dir":"gen","transpilers":["go"],"options":{},"seams":{},"plugins":[]}`, false},
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
		// emit の要求と応答は、出力ごと、トランスパイラごとの並び（docs/plugin-protocol.md「emit」）
		{"emit batch response", "EmitBatchResponse", `{"outputs":[{"results":[{"files":[],"warnings":[]},{"files":[],"warnings":[],"errors":[{"code":"X-1","at":"a.b","message":"m"}]}]},{"results":[]}]}`, true},
		{"emit batch response empty", "EmitBatchResponse", `{"outputs":[]}`, true},
		{"emit batch response old shape", "EmitBatchResponse", `{"files":[],"warnings":[]}`, false},
		{"emit batch response no results", "EmitBatchResponse", `{"outputs":[{}]}`, false},
		{"emit batch response bad result", "EmitBatchResponse", `{"outputs":[{"results":[{"files":[]}]}]}`, false},
		{"emit output no view", "EmitOutput", `{"transpilers":["go"]}`, false},
		{"emit batch wrong protocol", "EmitBatch", `{"protocol":"0.2","direction":"emit","spec":{"version":"0.23","namespaces":{},"program":{"version":"0.23","namespaces":[],"binds":[]}},"outputs":[]}`, false},
		{"emit batch", "EmitBatch", `{"protocol":"0.23","direction":"emit","spec":{"version":"0.23","namespaces":{},"program":{"version":"0.23","namespaces":[],"binds":[]}},"outputs":[]}`, true},
		{"emit batch wrong direction", "EmitBatch", `{"protocol":"0.23","direction":"check","spec":{"version":"0.23","namespaces":{},"program":{"version":"0.23","namespaces":[],"binds":[]}},"outputs":[]}`, false},
		{"import request", "ImportRequest", `{"protocol":"0.23","direction":"import","format":"openapi","files":[{"path":"openapi.yaml","content":""}]}`, true},
		{"import request wrong direction", "ImportRequest", `{"protocol":"0.23","direction":"emit","format":"openapi","files":[]}`, false},
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
		{"program with other version", func() any { p := program(); p["version"] = "0.1"; return p }},
		{"program with extra key", func() any { p := program(); p["extra"] = 1; return p }},
		{"program binds null", func() any { p := program(); p["binds"] = nil; return p }},
		{"op without returns", func() any { p := program(); delete(firstDecl(p, "op"), "returns"); return p }},
		{"op without body", func() any { p := program(); delete(firstDecl(p, "op"), "body"); return p }},
		{"op body as string", func() any { p := program(); firstDecl(p, "op")["body"] = "leaf"; return p }},
		{"op without signature", func() any { p := program(); delete(firstDecl(p, "op"), "signature"); return p }},
		// フィールドの既定値は版 0.5 でなくした（docs/ir.md「版」）: default は知らないキー
		{"model field with default", func() any {
			p := program()
			firstDecl(p, "model")["fields"].([]any)[0].(map[string]any)["default"] = map[string]any{"kind": "int", "value": 1}
			return p
		}},
		{"op without carried", func() any { p := program(); delete(firstDecl(p, "op"), "carried"); return p }},
		{"op carried without effects", func() any {
			p := program()
			delete(firstDecl(p, "op")["carried"].(map[string]any), "effects")
			return p
		}},
		{"decorator without reads", func() any { p := program(); delete(firstDecl(p, "decorator"), "reads"); return p }},
		{"decorator param without rest", func() any {
			p := program()
			delete(firstDecl(p, "decorator")["params"].([]any)[0].(map[string]any), "rest")
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
		// namespaces の値は、その名前空間を出力する単位ごとの設定の列（docs/ir.md「その名前空間から見た IR」）
		{"view namespaces value as object", func() any {
			w := view()
			w["namespaces"] = map[string]any{"browser": w["config"]}
			return w
		}},
		{"view without files", func() any { w := view(); delete(w, "files"); return w }},
		{"view files as string", func() any { w := view(); w["files"] = "a.hozo"; return w }},
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
		// Program / View は空の version を ir.Version に補うので、const と一致する。
		if (len(gotKeys) != 0 || len(want) != 0) && !reflect.DeepEqual(gotKeys, want) {
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
		typeOf[ir.CoreName]():        {string(ir.CoreString), string(ir.CoreBoolean), string(ir.CoreInt), string(ir.CoreFloat), string(ir.CoreDatetime), string(ir.CoreDate), string(ir.CoreTime), string(ir.CoreDuration)},
		typeOf[ir.UnionVariant]():    {string(ir.UnionArms), string(ir.UnionSet)},
		typeOf[ir.StateShape]():      {string(ir.StateList), string(ir.StateNullable), string(ir.StateSingle)},
		typeOf[ir.Operation]():       {string(ir.OpPost), string(ir.OpPut), string(ir.OpDelete)},
		typeOf[ir.DecoratorTarget](): {string(ir.TargetOp), string(ir.TargetField), string(ir.TargetState), string(ir.TargetException), string(ir.TargetScalar), string(ir.TargetModel), string(ir.TargetEffect), string(ir.TargetRead)},
		typeOf[ir.AnyOf]():           {string(ir.AnyException), string(ir.AnyEffect), string(ir.AnyState), string(ir.AnyOp), string(ir.AnyModel), string(ir.AnyVariant)},
		typeOf[ir.RelationKind]():    {string(ir.Prevents), string(ir.Keeps)},
		typeOf[ir.CatchMode]():       {string(ir.Recovers), string(ir.Halts), string(ir.Ignores)},
		typeOf[ir.EffectState]():     {string(ir.Prevented), string(ir.Kept), string(ir.Unknown)},
		typeOf[ir.DecidedBy]():       {string(ir.DecidedByType), string(ir.DecidedByPredicate)},
		typeOf[ir.NarrowBy]():        {string(ir.NarrowByPresence), string(ir.NarrowByVariant), string(ir.NarrowByPredicate), string(ir.NarrowByElse), string(ir.NarrowByNone)},
		typeOf[ir.EntrySourceKind](): {string(ir.SourceLeaf), string(ir.SourceEffect), string(ir.SourceExternal), string(ir.SourceBoundary), string(ir.SourceDecorator), string(ir.SourceArm), string(ir.SourceNever), string(ir.SourceUnknown)},
		typeOf[ir.EntryStepVerb]():   {string(ir.StepThrows), string(ir.StepNone), string(ir.StepIs)},
		typeOf[ir.EntryTargetKind](): {string(ir.StepLeaf), string(ir.StepEffect), string(ir.StepRead), string(ir.StepState), string(ir.StepOp), string(ir.StepDecorator), string(ir.StepClassify)},
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
