package ir

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// marshalStruct は、メソッドを持たない別名型の値 v を encoding/json で書く。
// 書く前に、omitempty のない nil の列と写像を空にし、docs/ir.md「JSON」の
// 「空の列は [] で出す（省かない）」を満たす。入れ子の（埋め込みを含む）構造体にも再帰する。
func marshalStruct(v any) ([]byte, error) {
	rv := reflect.ValueOf(v)
	cp := reflect.New(rv.Type()).Elem()
	cp.Set(rv)
	fillEmpty(cp)
	return json.Marshal(cp.Interface())
}

func fillEmpty(v reflect.Value) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if f.Anonymous && fv.Kind() == reflect.Struct {
			fillEmpty(fv)
			continue
		}
		if f.PkgPath != "" { // unexported
			continue
		}
		switch fv.Kind() {
		case reflect.Slice:
			if fv.IsNil() && !hasOmitEmpty(f) {
				fv.Set(reflect.MakeSlice(fv.Type(), 0, 0))
			}
		case reflect.Map:
			if fv.IsNil() && !hasOmitEmpty(f) {
				fv.Set(reflect.MakeMap(fv.Type()))
			}
		case reflect.Struct:
			fillEmpty(fv)
		}
	}
}

func hasOmitEmpty(f reflect.StructField) bool {
	tag := f.Tag.Get("json")
	_, opts, _ := strings.Cut(tag, ",")
	for _, o := range strings.Split(opts, ",") {
		if o == "omitempty" {
			return true
		}
	}
	return false
}

func jsonName(f reflect.StructField) (string, bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		name = f.Name
	}
	return name, true
}

// marshalKind は marshalStruct の結果の先頭に "kind" を足す（docs/ir.md「JSON」の判別子）。
func marshalKind(kind string, v any) ([]byte, error) {
	b, err := marshalStruct(v)
	if err != nil {
		return nil, err
	}
	head := `{"kind":` + quote(kind)
	if len(b) > 2 { // not "{}"
		head += ","
	}
	return append([]byte(head), b[1:]...), nil
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// unmarshalStruct は JSON のオブジェクトを構造体 dst（ポインタ）にフィールドごとに読む。
// ユニオンの interface 型のフィールド（とその列）は kind で分けて読み、
// 埋め込んだ構造体には同じオブジェクトで再帰する。空の列と写像は nil にする。
// 知らないキーは無視し、null は値なし（ゼロ値）として扱う。
func unmarshalStruct(data []byte, dst any) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v := reflect.ValueOf(dst).Elem()
	return setFields(raw, v)
}

func setFields(raw map[string]json.RawMessage, v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if f.Anonymous && fv.Kind() == reflect.Struct {
			if err := setFields(raw, fv); err != nil {
				return err
			}
			continue
		}
		if f.PkgPath != "" {
			continue
		}
		name, ok := jsonName(f)
		if !ok {
			continue
		}
		r, ok := raw[name]
		if !ok || isNull(r) {
			fv.Set(reflect.Zero(fv.Type()))
			continue
		}
		if err := decodeValue(r, fv); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func isNull(r json.RawMessage) bool {
	return strings.TrimSpace(string(r)) == "null"
}

func decodeValue(r json.RawMessage, fv reflect.Value) error {
	t := fv.Type()
	if dec, ok := unionDecoders[t]; ok {
		val, err := dec(r)
		if err != nil {
			return err
		}
		fv.Set(reflect.ValueOf(val))
		return nil
	}
	if t.Kind() == reflect.Slice {
		if dec, ok := unionDecoders[t.Elem()]; ok {
			var items []json.RawMessage
			if err := json.Unmarshal(r, &items); err != nil {
				return err
			}
			if len(items) == 0 {
				fv.Set(reflect.Zero(t))
				return nil
			}
			s := reflect.MakeSlice(t, len(items), len(items))
			for i, item := range items {
				val, err := dec(item)
				if err != nil {
					return fmt.Errorf("[%d]: %w", i, err)
				}
				s.Index(i).Set(reflect.ValueOf(val))
			}
			fv.Set(s)
			return nil
		}
	}
	if err := json.Unmarshal(r, fv.Addr().Interface()); err != nil {
		return err
	}
	if (t.Kind() == reflect.Slice || t.Kind() == reflect.Map) && fv.Len() == 0 {
		fv.Set(reflect.Zero(t))
	}
	return nil
}

// unionDecoders は interface 型ごとに、kind で分けて読む関数。init で埋める。
var unionDecoders map[reflect.Type]func(json.RawMessage) (any, error)

func init() {
	unionDecoders = map[reflect.Type]func(json.RawMessage) (any, error){
		reflect.TypeOf((*Type)(nil)).Elem():      lift(UnmarshalType),
		reflect.TypeOf((*ParamType)(nil)).Elem(): lift(UnmarshalParamType),
		reflect.TypeOf((*Statement)(nil)).Elem(): lift(UnmarshalStatement),
		reflect.TypeOf((*Expr)(nil)).Elem():      lift(UnmarshalExpr),
		reflect.TypeOf((*Atom)(nil)).Elem():      lift(UnmarshalAtom),
		reflect.TypeOf((*ArmBody)(nil)).Elem():   lift(UnmarshalArmBody),
		reflect.TypeOf((*Literal)(nil)).Elem():   lift(UnmarshalLiteral),
		reflect.TypeOf((*ArgValue)(nil)).Elem():  lift(UnmarshalArgValue),
		reflect.TypeOf((*Decl)(nil)).Elem():      lift(UnmarshalDecl),

		reflect.TypeOf((*EffectTarget)(nil)).Elem():    lift(UnmarshalEffectTarget),
		reflect.TypeOf((*ReadRef)(nil)).Elem():         lift(UnmarshalReadRef),
		reflect.TypeOf((*EffectRef)(nil)).Elem():       lift(UnmarshalEffectRef),
		reflect.TypeOf((*ClassifySubject)(nil)).Elem(): lift(UnmarshalClassifySubject),
		reflect.TypeOf((*MatchSubject)(nil)).Elem():    lift(UnmarshalMatchSubject),
	}
}

func lift[I any](f func([]byte) (I, error)) func(json.RawMessage) (any, error) {
	return func(r json.RawMessage) (any, error) {
		v, err := f(r)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
}

// kindOf は JSON のオブジェクトの kind を読む。
func kindOf(data []byte) (string, error) {
	var k struct {
		Kind *string `json:"kind"`
	}
	if err := json.Unmarshal(data, &k); err != nil {
		return "", err
	}
	if k.Kind == nil {
		return "", fmt.Errorf("ir: missing kind")
	}
	return *k.Kind, nil
}

// decodeUnion は kind で table を引き、該当する具体型に読む。
func decodeUnion[I any](what string, data []byte, table map[string]func([]byte) (I, error)) (I, error) {
	var zero I
	kind, err := kindOf(data)
	if err != nil {
		return zero, fmt.Errorf("ir: %s: %w", what, err)
	}
	dec, ok := table[kind]
	if !ok {
		return zero, fmt.Errorf("ir: %s: unknown kind %q", what, kind)
	}
	v, err := dec(data)
	if err != nil {
		return zero, fmt.Errorf("ir: %s %q: %w", what, kind, err)
	}
	return v, nil
}

// asValue は T を読み、その値を I として返す（値型でユニオンを実装する具体型用）。
func asValue[I any, T any](data []byte) (I, error) {
	p := new(T)
	if err := json.Unmarshal(data, p); err != nil {
		var zero I
		return zero, err
	}
	return any(*p).(I), nil
}

// asPointer は T を読み、そのポインタを I として返す（ポインタでユニオンを実装する具体型用）。
func asPointer[I any, T any](data []byte) (I, error) {
	p := new(T)
	if err := json.Unmarshal(data, p); err != nil {
		var zero I
		return zero, err
	}
	return any(p).(I), nil
}
