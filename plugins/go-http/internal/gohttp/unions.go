package gohttp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは、通信に出る union（docs/plugin-go-http.md「通信の形」の「union」）: 出せる union かの検査（GOHTTP-5）と、
// 二つの http.go に出す、表 httpUnions と、読み書きの補助。

// wireUnion は、経路の op の、本文、結果、例外の引数の型の中に現れた union（arms）一つ。
type wireUnion struct {
	decl *ir.Union
	arms []wireArm
	// problem は、通信に出せない理由（出せるなら ""）。
	problem string
}

// wireArm は、通信に出る union のアーム一つ。
type wireArm struct {
	name string
	// goName は、アームの値の Go の型の名前（修飾なし。union を宣言した名前空間の、定義のパッケージの名前）: その名前空間の model なら、
	// その構造体。それ以外は、包む型 <Union><アーム名>（docs/plugin-go.md「union」）。
	goName  string
	wrapped bool
	// keys は、決め手: そのアームの model の、省略可能でないフィールドのうち、ほかのどのアームの、どのフィールドとも、名前が違うもの。
	keys []string
}

// checkJSON は GOHTTP-5: 型 t（what: 本文、結果、例外の引数の、どの型か）の中の union が、どれも、通信に出せること。
// 出せる union は、最初に現れた順に、g.unions に集める。
func (g *generator) checkJSON(op *ir.Op, t ir.Type, what string) {
	for _, u := range g.unionsIn(t, map[string]bool{}) {
		if u.problem != "" {
			g.errorf(codeHTTPJSON, op.Name, "経路の op %s の%s %s が、通信に出せない union %s を中に持つ: %s（通信に出せるのは、アームが、どれも、フィールドを持つ model で、アームごとに、そのアームだけが持つ、省略可能でないフィールドがある union）",
				op.Name, what, golang.TypeDisplay(t), u.decl.Name, u.problem)
		}
	}
}

// unionsIn は、型 t の中の union（arms）を、辿った順に返す（アームの型の中の union も）。初めて見た union は、調べて、覚える。
func (g *generator) unionsIn(t ir.Type, visited map[string]bool) []*wireUnion {
	switch x := t.(type) {
	case ir.OptionalType:
		return g.unionsIn(x.Of, visited)
	case ir.ArrayType:
		return g.unionsIn(x.Of, visited)
	case ir.CaseType:
		return g.unionsIn(x.Of, visited)
	case ir.RefType:
		if visited[x.Target] {
			return nil
		}
		visited[x.Target] = true
		switch d := g.p.Lookup(x.Target).(type) {
		case *ir.Union:
			if d.Variant != ir.UnionArms {
				return nil
			}
			out := []*wireUnion{g.wireUnion(d)}
			for _, a := range d.Arms {
				out = append(out, g.unionsIn(a.Type, visited)...)
			}
			return out
		case *ir.Model:
			var out []*wireUnion
			for _, f := range d.Fields {
				out = append(out, g.unionsIn(f.Type, visited)...)
			}
			if d.Index != nil {
				out = append(out, g.unionsIn(d.Index.Value, visited)...)
			}
			return out
		}
	}
	return nil
}

// wireUnion は、union u を、通信に出せるか調べた結果（一度だけ調べる）。出せる union は、g.unions に足す。
func (g *generator) wireUnion(u *ir.Union) *wireUnion {
	if w, ok := g.unionOf[u.Name]; ok {
		return w
	}
	if g.unionOf == nil {
		g.unionOf = map[string]*wireUnion{}
	}
	w := &wireUnion{decl: u}
	g.unionOf[u.Name] = w
	ns := g.declNS[u.Name]
	models := make([]*ir.Model, len(u.Arms))
	for i, a := range u.Arms {
		if r, ok := a.Type.(ir.RefType); ok {
			if m, isModel := g.p.Lookup(r.Target).(*ir.Model); isModel && m.Index == nil {
				models[i] = m
			}
		}
		if models[i] == nil {
			w.problem = fmt.Sprintf("アーム %s の型 %s が、フィールドを持つ model でない", a.Name, golang.TypeDisplay(a.Type))
			return w
		}
	}
	for i, a := range u.Arms {
		arm := wireArm{name: a.Name}
		if g.declNS[models[i].Name] == ns {
			arm.goName = golang.TypeName(models[i].Name, ns)
		} else {
			arm.goName, arm.wrapped = golang.TypeName(u.Name, ns)+golang.Capitalize(a.Name), true
		}
		others := map[string]bool{}
		for j := range models {
			if j != i {
				for _, f := range models[j].Fields {
					others[f.Name] = true
				}
			}
		}
		for _, f := range models[i].Fields {
			if _, optional := f.Type.(ir.OptionalType); !optional && !others[f.Name] {
				arm.keys = append(arm.keys, f.Name)
			}
		}
		if len(arm.keys) == 0 {
			w.problem = fmt.Sprintf("アーム %s（%s）に、そのアームだけが持つ、省略可能でないフィールドがない", a.Name, models[i].Name)
			return w
		}
		w.arms = append(w.arms, arm)
	}
	g.unions = append(g.unions, w)
	return w
}

// unionTable は、表 httpUnions（docs/plugin-go-http.md「補助」の「union の読み書き」）。qualifier は、名前空間のパッケージの修飾
// （Imports.Qualifier）。reflect の名前は、決まった名前（helperPackages）。
func (g *generator) unionTable(qualifier func(namespace string) string) string {
	var b strings.Builder
	b.WriteString("// httpUnions は、通信に出る union ごとの、アームの並び（union の宣言の順）。\n")
	b.WriteString("var httpUnions = map[reflect.Type][]httpArm{\n")
	for _, u := range g.unions {
		ns := g.declNS[u.decl.Name]
		q := qualifier(ns)
		fmt.Fprintf(&b, "reflect.TypeFor[%s%s](): {\n", q, golang.TypeName(u.decl.Name, ns))
		for _, a := range u.arms {
			keys := make([]string, len(a.keys))
			for i, k := range a.keys {
				keys[i] = strconv.Quote(k)
			}
			wrap := ""
			if a.wrapped {
				wrap = ", wrap: true"
			}
			fmt.Fprintf(&b, "{typ: reflect.TypeFor[%s%s](), keys: []string{%s}%s},\n", q, a.goName, strings.Join(keys, ", "), wrap)
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n\n")
	return b.String()
}

// withUnions は、補助 h の中の、encoding/json の読み書きを、union の読み書き（httpUnmarshal、httpMarshal）に替えたもの。
func withUnions(h helper) helper {
	text := h.text
	for _, r := range [][2]string{
		{"json.Unmarshal(raw, dst)", "httpUnmarshal(raw, dst)"},
		{"json.Unmarshal(data, dst)", "httpUnmarshal(data, dst)"},
		{"json.Unmarshal(raw, &e)", "httpUnmarshal(raw, &e)"},
		{"json.Marshal(v)", "httpMarshal(v)"},
		{"json.Marshal(c.whole)", "httpMarshal(c.whole)"},
		{"json.Marshal(item.value)", "httpMarshal(item.value)"},
	} {
		text = strings.ReplaceAll(text, r[0], r[1])
	}
	return helper{text: text, imports: h.imports}
}

// helperUnionCodec は、union の読み書きの補助（docs/plugin-go-http.md「補助」の「union の読み書き」）。表 httpUnions の後ろに出す。
var helperUnionCodec = helper{imports: []string{"bytes", "encoding/json", "errors", "fmt", "reflect", "strings"}, text: `// httpArm は、通信に出る union のアーム一つ: 値として持つ Go の型と、決め手（そのアームだけが持つ、省略可能でないフィールドの、
// JSON のキー）。wrap は、包む型（中身は、フィールド Value）。
type httpArm struct {
	typ  reflect.Type
	keys []string
	wrap bool
}

// httpUnmarshal は、JSON の data を、dst（ポインタ）の指す先に読む。union は、アームの値の JSON、そのままで、どのアームかは、
// 決め手のキーで決める。union を中に持たない型は、encoding/json に、そのまま渡す。
func httpUnmarshal(data []byte, dst any) error {
	return httpDecode(data, reflect.ValueOf(dst).Elem())
}

// httpMarshal は、v を JSON にする。union は、アームの値の JSON、そのまま（包む型は、その Value）。
func httpMarshal(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return httpEncode(reflect.ValueOf(v))
}

func httpDecode(data []byte, dst reflect.Value) error {
	t := dst.Type()
	if !httpHasUnion(t, map[reflect.Type]bool{}) {
		return json.Unmarshal(data, dst.Addr().Interface())
	}
	null := string(bytes.TrimSpace(data)) == "null"
	switch t.Kind() {
	case reflect.Interface:
		if null {
			dst.SetZero()
			return nil
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(data, &obj); err != nil {
			return err
		}
		arms, found := httpUnions[t], -1
		for i := range arms {
			for _, k := range arms[i].keys {
				if _, ok := obj[k]; !ok {
					continue
				}
				if found >= 0 && found != i {
					return fmt.Errorf("union %s: fields of two arms", t)
				}
				found = i
			}
		}
		if found < 0 {
			return fmt.Errorf("union %s: no arm's field", t)
		}
		v := reflect.New(arms[found].typ).Elem()
		into := v
		if arms[found].wrap {
			into = v.Field(0)
		}
		if err := httpDecode(data, into); err != nil {
			return err
		}
		dst.Set(v)
		return nil
	case reflect.Pointer:
		if null {
			dst.SetZero()
			return nil
		}
		v := reflect.New(t.Elem())
		if err := httpDecode(data, v.Elem()); err != nil {
			return err
		}
		dst.Set(v)
		return nil
	case reflect.Slice:
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		if items == nil {
			dst.SetZero()
			return nil
		}
		v := reflect.MakeSlice(t, len(items), len(items))
		for i := range items {
			if err := httpDecode(items[i], v.Index(i)); err != nil {
				return err
			}
		}
		dst.Set(v)
		return nil
	case reflect.Map:
		raw := reflect.New(reflect.MapOf(t.Key(), reflect.TypeFor[json.RawMessage]()))
		if err := json.Unmarshal(data, raw.Interface()); err != nil {
			return err
		}
		if raw.Elem().IsNil() {
			dst.SetZero()
			return nil
		}
		v := reflect.MakeMapWithSize(t, raw.Elem().Len())
		for it := raw.Elem().MapRange(); it.Next(); {
			item := reflect.New(t.Elem()).Elem()
			if err := httpDecode(it.Value().Bytes(), item); err != nil {
				return err
			}
			v.SetMapIndex(it.Key(), item)
		}
		dst.Set(v)
		return nil
	case reflect.Struct:
		if null {
			return nil
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(data, &obj); err != nil {
			return err
		}
		for i := 0; i < t.NumField(); i++ {
			name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			if name == "" {
				name = t.Field(i).Name
			}
			if raw, ok := obj[name]; ok && t.Field(i).IsExported() {
				if err := httpDecode(raw, dst.Field(i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return errors.New("union: unsupported type " + t.String())
}

func httpEncode(v reflect.Value) ([]byte, error) {
	t := v.Type()
	if !httpHasUnion(t, map[reflect.Type]bool{}) {
		return json.Marshal(v.Interface())
	}
	switch t.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return []byte("null"), nil
		}
		return httpEncode(v.Elem())
	case reflect.Slice:
		if v.IsNil() {
			return []byte("null"), nil
		}
		var b bytes.Buffer
		b.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			item, err := httpEncode(v.Index(i))
			if err != nil {
				return nil, err
			}
			b.Write(item)
		}
		b.WriteByte(']')
		return b.Bytes(), nil
	case reflect.Map:
		if v.IsNil() {
			return []byte("null"), nil
		}
		items := map[string]json.RawMessage{}
		for it := v.MapRange(); it.Next(); {
			item, err := httpEncode(it.Value())
			if err != nil {
				return nil, err
			}
			items[fmt.Sprint(it.Key().Interface())] = item
		}
		return json.Marshal(items)
	case reflect.Struct:
		if httpWrapped(t) {
			return httpEncode(v.Field(0))
		}
		var b bytes.Buffer
		b.WriteByte('{')
		first := true
		for i := 0; i < t.NumField(); i++ {
			name, opts, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			if !t.Field(i).IsExported() || name == "-" || strings.Contains(opts, "omitempty") && httpEmpty(v.Field(i)) {
				continue
			}
			if name == "" {
				name = t.Field(i).Name
			}
			item, err := httpEncode(v.Field(i))
			if err != nil {
				return nil, err
			}
			key, _ := json.Marshal(name)
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.Write(key)
			b.WriteByte(':')
			b.Write(item)
		}
		b.WriteByte('}')
		return b.Bytes(), nil
	}
	return nil, errors.New("union: unsupported type " + t.String())
}

// httpHasUnion は、型 t の値の中に、union（か、実行時に型の決まる値）が現れうるか。現れない型は、encoding/json に、そのまま渡す。
func httpHasUnion(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Map:
		return httpHasUnion(t.Elem(), seen)
	case reflect.Struct:
		if httpWrapped(t) {
			return true
		}
		if t.Implements(reflect.TypeFor[json.Marshaler]()) || reflect.PointerTo(t).Implements(reflect.TypeFor[json.Unmarshaler]()) {
			return false
		}
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).IsExported() && httpHasUnion(t.Field(i).Type, seen) {
				return true
			}
		}
	}
	return false
}

// httpWrapped は、型 t が、union のアームを包む型か。
func httpWrapped(t reflect.Type) bool {
	for _, arms := range httpUnions {
		for i := range arms {
			if arms[i].wrap && arms[i].typ == t {
				return true
			}
		}
	}
	return false
}

// httpEmpty は、encoding/json の omitempty が、キーを省く値か。
func httpEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return v.IsZero() && v.Kind() != reflect.Struct
}

`}
