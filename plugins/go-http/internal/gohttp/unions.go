package gohttp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
	"github.com/mew-ton/hozo/plugin/httpvocab"
)

// このファイルは、通信に出る union（docs/plugin-http.md「通信の形」の「union」）の、Go の読み書き: 二つの http.go に出す、表 httpUnions と、
// 読み書きの補助。どの union が、通信に出せるか（HTTP-5）と、アームの決め手を求めるのは、plugin/httpvocab。

// wireUnion は、経路の op の、本文、結果、例外の引数の型の中に現れた、通信に出せる union（arms）一つ。
type wireUnion struct {
	decl *ir.Union
	arms []wireArm
}

// wireArm は、通信に出る union のアーム一つ。
type wireArm struct {
	name string
	// goName は、アームの値の Go の型の名前（修飾なし。union を宣言した名前空間の、定義のパッケージの名前）: その名前空間の model なら、
	// その構造体。それ以外は、包む型 <Union><アーム名>（docs/plugin-go.md「union」）。
	goName  string
	wrapped bool
	// keys は、決め手（httpvocab.Arm.Keys）。
	keys []string
}

// wireUnions は、httpvocab が読み解いた、通信に出せる union（最初に現れた順）に、Go の型の名前を足して、g.unions に置く。
func (g *generator) wireUnions(unions []*httpvocab.Union) {
	for _, u := range unions {
		ns := g.declNS[u.Decl.Name]
		w := &wireUnion{decl: u.Decl}
		for _, a := range u.Arms {
			arm := wireArm{name: a.Name, keys: a.Keys}
			if g.declNS[a.Model.Name] == ns {
				arm.goName = golang.TypeName(a.Model.Name, ns)
			} else {
				arm.goName, arm.wrapped = golang.TypeName(u.Decl.Name, ns)+golang.Capitalize(a.Name), true
			}
			w.arms = append(w.arms, arm)
		}
		g.unions = append(g.unions, w)
	}
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
