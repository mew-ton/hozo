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
	// その構造体。それ以外は、包む型 <Union><アーム名>（plugins/go/spec/go.hozo の Requirement.Types の「union」）。
	goName  string
	wrapped bool
	// keys は、決め手（httpvocab.Arm.Keys）。
	keys []string
	// tag は、印のキー（httpvocab.Union.Tag。印を持つ union の、model のアーム）。value は、その印の JSON。
	// リテラルの型のアーム（literal）では、tag は空で、value は、そのリテラルの JSON。
	tag, value string
	literal    bool
}

// wireConst は、通信に出る、リテラルの型のフィールドを持つ model 一つと、そのフィールドの並び（フィールドの順）。
type wireConst struct {
	decl   *ir.Model
	fields []constField
}

// constField は、リテラルの型のフィールド一つ: JSON のキー、値の JSON、その前にある Go の項目の数。
type constField struct {
	at         int
	key, value string
}

// wireUnions は、httpvocab が読み解いた、通信に出せる union（最初に現れた順）に、Go の型の名前を足して、g.unions に置く。
func (g *generator) wireUnions(unions []*httpvocab.Union) {
	for _, u := range unions {
		ns := g.declNS[u.Decl.Name]
		w := &wireUnion{decl: u.Decl}
		for _, a := range u.Arms {
			arm := wireArm{name: a.Name, keys: a.Keys}
			switch {
			case a.Literal != nil:
				// リテラルの型のアーム: 値を持たない、包む型（plugins/go/spec/go.hozo の Requirement.Types の「union」）
				arm.goName, arm.literal, arm.value = golang.TypeName(u.Decl.Name, ns)+golang.Capitalize(a.Name), true, httpvocab.LiteralJSON(a.Literal)
			case g.declNS[a.Model.Name] == ns:
				arm.goName = golang.TypeName(a.Model.Name, ns)
			default:
				arm.goName, arm.wrapped = golang.TypeName(u.Decl.Name, ns)+golang.Capitalize(a.Name), true
			}
			if a.TagValue != nil {
				arm.tag, arm.value = u.Tag, httpvocab.LiteralJSON(a.TagValue)
			}
			w.arms = append(w.arms, arm)
		}
		g.unions = append(g.unions, w)
	}
}

// wireConsts は、httpvocab が集めた、通信に出る、リテラルの型のフィールドを持つ model（最初に現れた順）を、g.consts に置く。
func (g *generator) wireConsts(models []*ir.Model) {
	for _, m := range models {
		c := &wireConst{decl: m}
		stored := 0
		for _, f := range m.Fields {
			if lt, ok := f.Type.(ir.LiteralType); ok {
				c.fields = append(c.fields, constField{at: stored, key: f.Name, value: httpvocab.LiteralJSON(lt.Literal)})
				continue
			}
			stored++
		}
		g.consts = append(g.consts, c)
	}
}

// rawString は、文字列 s の、Go の文字列リテラル: バッククォートを含まなければ、生の文字列リテラル。
func rawString(s string) string {
	if strings.Contains(s, "`") {
		return strconv.Quote(s)
	}
	return "`" + s + "`"
}

// wireScalar は、通信に出る、元が日時の核の型（datetime、date、time）の scalar 一つと、その元の核の型。
type wireScalar struct {
	decl *ir.Scalar
	core ir.CoreName
}

// codec は、union の読み書きの補助（httpMarshal、httpUnmarshal と、三つの表）を出すか（docs/plugin-go-http.md「補助」の
// 「union の読み書き」）: 通信に出る型の中に、union か、リテラルの型のフィールドを持つ model か、元が日時の核の型の scalar があるとき。
func (g *generator) codec() bool {
	return len(g.unions) > 0 || len(g.consts) > 0 || len(g.scalars) > 0
}

// wireScalars は、経路 routes の、本文（@http.body、@http.bodyObj のフィールド）、結果、@http.status を持つ例外の引数の型の中の、
// 元が日時の核の型の scalar を、最初に現れた順に g.scalars に足す（optional、array、model のフィールドとインデックスの値、union の
// アームの型を辿る。union の辿り方と同じ）。scalar は、宣言されたもの（何段の scalar を経ても）だけで、核の型そのものは載せない
// （基底は、核の型を、JSON の読み書きを持つ Go の型で出す）。
func (g *generator) wireScalars(routes []route) {
	visited := map[string]bool{}
	for i := range routes {
		for _, in := range routes[i].inputs {
			if in.place == inBody {
				g.scalarsIn(in.field.Type, visited)
			}
		}
		g.scalarsIn(routes[i].op.Returns, visited)
	}
	for _, st := range g.statuses(routes) {
		for _, p := range st.exception.Params {
			g.scalarsIn(p.Type, visited)
		}
	}
}

func (g *generator) scalarsIn(t ir.Type, visited map[string]bool) {
	switch x := t.(type) {
	case ir.OptionalType:
		g.scalarsIn(x.Of, visited)
	case ir.ArrayType:
		g.scalarsIn(x.Of, visited)
	case ir.CaseType:
		g.scalarsIn(x.Of, visited)
	case ir.RefType:
		if visited[x.Target] {
			return
		}
		visited[x.Target] = true
		switch d := g.p.Lookup(x.Target).(type) {
		case *ir.Scalar:
			if core, ok := g.timeCore(d); ok {
				g.scalars = append(g.scalars, wireScalar{decl: d, core: core})
			}
		case *ir.Union:
			for _, a := range d.Arms {
				g.scalarsIn(a.Type, visited)
			}
		case *ir.Model:
			for _, f := range d.Fields {
				g.scalarsIn(f.Type, visited)
			}
			if d.Index != nil {
				g.scalarsIn(d.Index.Value, visited)
			}
		}
	}
}

// timeCore は、scalar s の元の核の型（scalar の並びを辿った先）が、日時の核の型（datetime、date、time）なら、それ。
func (g *generator) timeCore(s *ir.Scalar) (ir.CoreName, bool) {
	seen := map[string]bool{}
	for base := s.Base; ; {
		switch x := base.(type) {
		case ir.CoreType:
			switch x.Name {
			case ir.CoreDatetime, ir.CoreDate, ir.CoreTime:
				return x.Name, true
			}
			return "", false
		case ir.RefType:
			next, ok := g.p.Lookup(x.Target).(*ir.Scalar)
			if !ok || seen[x.Target] {
				return "", false
			}
			seen[x.Target] = true
			base = next.Base
		default:
			return "", false
		}
	}
}

// unionTable は、表 httpUnions、表 httpConsts、表 httpScalars（docs/plugin-go-http.md「補助」の「union の読み書き」）。im は、
// このファイルの import（名前空間のパッケージの修飾と、日時の核の型の Go の型の import を記録する）。reflect の名前は、決まった名前（helperPackages）。
func (g *generator) unionTable(im *golang.Imports) string {
	qualifier := im.Qualifier
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
			how := "keys: []string{" + strings.Join(keys, ", ") + "}"
			switch {
			case a.literal:
				how = "value: " + rawString(a.value)
			case a.tag != "":
				how = "tag: " + strconv.Quote(a.tag) + ", value: " + rawString(a.value)
			}
			wrap := ""
			if a.wrapped {
				wrap = ", wrap: true"
			}
			fmt.Fprintf(&b, "{typ: reflect.TypeFor[%s%s](), %s%s},\n", q, a.goName, how, wrap)
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// httpConsts は、リテラルの型のフィールドを持つ model ごとの、そのフィールドの並び（フィールドの順）。\n")
	b.WriteString("var httpConsts = map[reflect.Type][]httpConst{\n")
	for _, c := range g.consts {
		ns := g.declNS[c.decl.Name]
		fmt.Fprintf(&b, "reflect.TypeFor[%s%s](): {\n", qualifier(ns), golang.TypeName(c.decl.Name, ns))
		for _, f := range c.fields {
			fmt.Fprintf(&b, "{at: %d, key: %s, value: %s},\n", f.at, strconv.Quote(f.key), rawString(f.value))
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// httpScalars は、元が日時の核の型の scalar ごとの、元の型。通信の上では、元の型の JSON で読み書きする。\n")
	b.WriteString("var httpScalars = map[reflect.Type]reflect.Type{\n")
	for _, sc := range g.scalars {
		ns := g.declNS[sc.decl.Name]
		fmt.Fprintf(&b, "reflect.TypeFor[%s%s](): reflect.TypeFor[%s](),\n", qualifier(ns), golang.TypeName(sc.decl.Name, ns), golang.GoType(ir.CoreType{Name: sc.core}, im))
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
var helperUnionCodec = helper{imports: []string{"bytes", "encoding/json", "errors", "fmt", "reflect", "strings"}, text: `// httpArm は、通信に出る union のアーム一つ: 値として持つ Go の型と、見分け方。keys は、決め手（そのアームだけが持つ、
// 省略可能でないフィールドの、JSON のキー）。tag と value は、印のキーと、そのアームの印の JSON。tag がなく、value だけなら、
// リテラルの型のアームで、value は、その値の JSON。wrap は、包む型（中身は、フィールド Value）。
type httpArm struct {
	typ   reflect.Type
	keys  []string
	tag   string
	value string
	wrap  bool
}

// httpConst は、リテラルの型のフィールド一つ: JSON のキーと、値の JSON。at は、その前にある、Go の項目の数（書く位置）。
type httpConst struct {
	at    int
	key   string
	value string
}

// httpUnmarshal は、JSON の data を、dst（ポインタ）の指す先に読む。union は、アームの値の JSON、そのままで、どのアームかは、
// リテラルの型のアームなら、値で、model のアームなら、印の値か、決め手のキーで決める。リテラルの型のフィールドは、キーがないか、
// 値が違えば、誤り。union も、リテラルの型のフィールドも、中に持たない型は、encoding/json に、そのまま渡す。
func httpUnmarshal(data []byte, dst any) error {
	return httpDecode(data, reflect.ValueOf(dst).Elem())
}

// httpMarshal は、v を JSON にする。union は、アームの値の JSON、そのまま（包む型は、その Value。リテラルの型のアームは、その値）。
// リテラルの型のフィールドは、仕様のフィールドの順の位置に、キーと値を足す。
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
	if base, ok := httpScalars[t]; ok {
		// 日時の scalar: 元の型の JSON で読んで、scalar の型に変える
		x := reflect.New(base)
		if err := json.Unmarshal(data, x.Interface()); err != nil {
			return err
		}
		dst.Set(x.Elem().Convert(t))
		return nil
	}
	null := string(bytes.TrimSpace(data)) == "null"
	switch t.Kind() {
	case reflect.Interface:
		if null {
			dst.SetZero()
			return nil
		}
		arms, found := httpUnions[t], -1
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(data, &obj); err != nil {
			// オブジェクトでない: リテラルの型のアームのうち、値が同じもの
			for i := range arms {
				if arms[i].tag == "" && arms[i].value != "" && httpSame(data, arms[i].value) {
					dst.Set(reflect.New(arms[i].typ).Elem())
					return nil
				}
			}
			return fmt.Errorf("union %s: no arm's value", t)
		}
		for i := range arms {
			if arms[i].tag != "" {
				// 印で見分ける union: 印のキーの値が、同じアーム
				if raw, ok := obj[arms[i].tag]; ok && found < 0 && httpSame(raw, arms[i].value) {
					found = i
				}
				continue
			}
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
		for _, c := range httpConsts[t] {
			if raw, ok := obj[c.key]; !ok || !httpSame(raw, c.value) {
				return fmt.Errorf("%s: field %s is not %s", t, c.key, c.value)
			}
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
	if base, ok := httpScalars[t]; ok {
		// 日時の scalar: 元の型に変えて、元の型の JSON で書く
		return json.Marshal(v.Convert(base).Interface())
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
		if arm := httpArmOf(t); arm != nil {
			if !arm.wrap {
				return []byte(arm.value), nil
			}
			return httpEncode(v.Field(0))
		}
		var b bytes.Buffer
		b.WriteByte('{')
		first := true
		consts := httpConsts[t]
		for i := 0; i <= t.NumField(); i++ {
			// リテラルの型のフィールド: 仕様のフィールドの順の位置に
			for _, c := range consts {
				if c.at != i {
					continue
				}
				key, _ := json.Marshal(c.key)
				if !first {
					b.WriteByte(',')
				}
				first = false
				b.Write(key)
				b.WriteByte(':')
				b.WriteString(c.value)
			}
			if i == t.NumField() {
				break
			}
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

// httpHasUnion は、型 t の値の中に、union（か、実行時に型の決まる値）か、リテラルの型のフィールドを持つ model か、日時の scalar が
// 現れうるか。現れない型は、encoding/json に、そのまま渡す。
func httpHasUnion(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	if httpScalars[t] != nil {
		return true
	}
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Map:
		return httpHasUnion(t.Elem(), seen)
	case reflect.Struct:
		if httpArmOf(t) != nil || httpConsts[t] != nil {
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

// httpArmOf は、型 t が、union のアームを包む型（中身を持つ型と、リテラルの型のアームの、値を持たない型）なら、そのアーム。
func httpArmOf(t reflect.Type) *httpArm {
	for _, arms := range httpUnions {
		for i := range arms {
			if arms[i].typ == t && (arms[i].wrap || arms[i].tag == "" && arms[i].value != "") {
				return &arms[i]
			}
		}
	}
	return nil
}

// httpSame は、JSON の raw が、JSON の value と、同じ値か（数は、数として比べる）。
func httpSame(raw []byte, value string) bool {
	var a, b any
	if json.Unmarshal(raw, &a) != nil || json.Unmarshal([]byte(value), &b) != nil {
		return false
	}
	return reflect.DeepEqual(a, b)
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
