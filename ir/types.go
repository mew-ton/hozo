package ir

import (
	"fmt"
	"sort"
	"strings"
)

// Type は型（docs/ir.md「型」）。具体型は値型の [CoreType]、[RefType]、[OptionalType]、
// [ArrayType]、[RecordType]、[CaseType]。void は型ではなく Op.Returns が nil であること。
// 等しさは [TypeEqual]（docs/types.md「等しい」）、表示は String（Hozo の構文）。
type Type interface {
	// Kind は JSON の判別子（"core"、"ref"、"optional"、"array"、"record"、"case"）。
	Kind() string
	// String は Hozo の構文で型を書く（docs/grammar.md「型」）。
	String() string
	isType()
}

// CoreName は核の型の名前（docs/ir.md「型」の core の name）。
type CoreName string

// 核の型の名前（docs/grammar.md「型」の CoreType）。
const (
	CoreString   CoreName = "string"
	CoreBoolean  CoreName = "boolean"
	CoreInt      CoreName = "int"
	CoreFloat    CoreName = "float"
	CoreTime     CoreName = "Time"
	CoreDuration CoreName = "Duration"
)

// CoreType は核の型（docs/ir.md「型」の kind "core"）。
type CoreType struct {
	Name CoreName `json:"name"`
}

// RefType は model / scalar / union への参照（docs/ir.md「型」の kind "ref"）。
type RefType struct {
	// Target は宣言の FQN。
	Target string `json:"target"`
}

// OptionalType は省略可能な型 T?（docs/ir.md「型」の kind "optional"）。
// 入れ子にはしない（docs/ir.md「正規化」の T?? → T?）。
type OptionalType struct {
	Of Type `json:"of"`
}

// ArrayType は列の型 T[]（docs/ir.md「型」の kind "array"）。
type ArrayType struct {
	Of Type `json:"of"`
}

// RecordType は Record<K, V>（docs/ir.md「型」の kind "record"）。
type RecordType struct {
	Key   Type `json:"key"`
	Value Type `json:"value"`
}

// CaseType は分類のケースで絞り込まれた型 C.Case<T>（docs/ir.md「型」の kind "case"）。
type CaseType struct {
	// Classify は classify の FQN。
	Classify string `json:"classify"`
	// Cases はケース名。昇順ではなく match のアームに書かれた順。
	Cases []string `json:"cases"`
	// Of は絞り込まれる前の型。
	Of Type `json:"of"`
}

var (
	_ Type = CoreType{}
	_ Type = RefType{}
	_ Type = OptionalType{}
	_ Type = ArrayType{}
	_ Type = RecordType{}
	_ Type = CaseType{}
)

// Kind は "core"。
func (CoreType) Kind() string { return "core" }

// Kind は "ref"。
func (RefType) Kind() string { return "ref" }

// Kind は "optional"。
func (OptionalType) Kind() string { return "optional" }

// Kind は "array"。
func (ArrayType) Kind() string { return "array" }

// Kind は "record"。
func (RecordType) Kind() string { return "record" }

// Kind は "case"。
func (CaseType) Kind() string { return "case" }

func (CoreType) isType()     {}
func (RefType) isType()      {}
func (OptionalType) isType() {}
func (ArrayType) isType()    {}
func (RecordType) isType()   {}
func (CaseType) isType()     {}

// String は核の型の名前。
func (t CoreType) String() string { return string(t.Name) }

// String は参照先の FQN。
func (t RefType) String() string { return t.Target }

// String は "T?"。
func (t OptionalType) String() string { return typeString(t.Of) + "?" }

// String は "T[]"。
func (t ArrayType) String() string { return typeString(t.Of) + "[]" }

// String は "Record<K, V>"。
func (t RecordType) String() string {
	return "Record<" + typeString(t.Key) + ", " + typeString(t.Value) + ">"
}

// String は "C.Case<T>"。ケースが複数なら "C.(A | B)<T>"（表面の構文にはない形）。
func (t CaseType) String() string {
	cases := ""
	switch len(t.Cases) {
	case 0:
		cases = "()"
	case 1:
		cases = t.Cases[0]
	default:
		cases = "(" + strings.Join(t.Cases, " | ") + ")"
	}
	return t.Classify + "." + cases + "<" + typeString(t.Of) + ">"
}

func typeString(t Type) string {
	if t == nil {
		return "void"
	}
	return t.String()
}

// MarshalJSON は kind を付けて書く。
func (t CoreType) MarshalJSON() ([]byte, error) {
	type alias CoreType
	return marshalKind("core", alias(t))
}

// MarshalJSON は kind を付けて書く。
func (t RefType) MarshalJSON() ([]byte, error) {
	type alias RefType
	return marshalKind("ref", alias(t))
}

// MarshalJSON は kind を付けて書く。
func (t OptionalType) MarshalJSON() ([]byte, error) {
	type alias OptionalType
	return marshalKind("optional", alias(t))
}

// MarshalJSON は kind を付けて書く。
func (t ArrayType) MarshalJSON() ([]byte, error) {
	type alias ArrayType
	return marshalKind("array", alias(t))
}

// MarshalJSON は kind を付けて書く。
func (t RecordType) MarshalJSON() ([]byte, error) {
	type alias RecordType
	return marshalKind("record", alias(t))
}

// MarshalJSON は kind を付けて書く。
func (t CaseType) MarshalJSON() ([]byte, error) {
	type alias CaseType
	return marshalKind("case", alias(t))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (t *OptionalType) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, t) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (t *ArrayType) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, t) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (t *RecordType) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, t) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (t *CaseType) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, t) }

var typeTable = map[string]func([]byte) (Type, error){
	"core":     asValue[Type, CoreType],
	"ref":      asValue[Type, RefType],
	"optional": asValue[Type, OptionalType],
	"array":    asValue[Type, ArrayType],
	"record":   asValue[Type, RecordType],
	"case":     asValue[Type, CaseType],
}

// UnmarshalType は kind で分けて Type を読む（docs/ir.md「JSON」）。
func UnmarshalType(data []byte) (Type, error) {
	return decodeUnion("type", data, typeTable)
}

// String は核の型 string を作る（docs/ir.md「型」の kind "core"）。
func String() Type { return CoreType{Name: CoreString} }

// Boolean は核の型 boolean を作る（docs/ir.md「型」の kind "core"）。
func Boolean() Type { return CoreType{Name: CoreBoolean} }

// Int は核の型 int を作る（docs/ir.md「型」の kind "core"）。
func Int() Type { return CoreType{Name: CoreInt} }

// Float は核の型 float を作る（docs/ir.md「型」の kind "core"）。
func Float() Type { return CoreType{Name: CoreFloat} }

// Time は核の型 Time を作る（docs/ir.md「型」の kind "core"）。
func Time() Type { return CoreType{Name: CoreTime} }

// Duration は核の型 Duration を作る（docs/ir.md「型」の kind "core"）。
func Duration() Type { return CoreType{Name: CoreDuration} }

// Ref は宣言の FQN への参照を作る（docs/ir.md「型」の kind "ref"）。
func Ref(fqn string) Type { return RefType{Target: fqn} }

// Optional は T? を作る。of がすでに optional ならそのまま返す（docs/ir.md「正規化」の T?? → T?）。
func Optional(of Type) Type {
	if _, ok := of.(OptionalType); ok {
		return of
	}
	return OptionalType{Of: of}
}

// Array は T[] を作る（docs/ir.md「型」の kind "array"）。
func Array(of Type) Type { return ArrayType{Of: of} }

// Record は Record<K, V> を作る（docs/ir.md「型」の kind "record"）。
func Record(key, value Type) Type { return RecordType{Key: key, Value: value} }

// Case は C.Case<T> を作る（docs/ir.md「型」の kind "case"）。cases は書かれた順のまま。
func Case(classify string, cases []string, of Type) Type {
	return CaseType{Classify: classify, Cases: cases, Of: of}
}

// TypeEqual は docs/types.md「等しい」。構造が等しければ等しい。ref は FQN が等しいとき。
// case は classify と cases の集合と of が等しいとき。nil 同士（void）は等しい。
func TypeEqual(a, b Type) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch x := a.(type) {
	case CoreType:
		y, ok := b.(CoreType)
		return ok && x.Name == y.Name
	case RefType:
		y, ok := b.(RefType)
		return ok && x.Target == y.Target
	case OptionalType:
		y, ok := b.(OptionalType)
		return ok && TypeEqual(x.Of, y.Of)
	case ArrayType:
		y, ok := b.(ArrayType)
		return ok && TypeEqual(x.Of, y.Of)
	case RecordType:
		y, ok := b.(RecordType)
		return ok && TypeEqual(x.Key, y.Key) && TypeEqual(x.Value, y.Value)
	case CaseType:
		y, ok := b.(CaseType)
		return ok && x.Classify == y.Classify && sameSet(x.Cases, y.Cases) && TypeEqual(x.Of, y.Of)
	}
	panic(fmt.Sprintf("ir: unknown Type %T", a))
}

func sameSet(a, b []string) bool {
	as := uniqueSorted(a)
	bs := uniqueSorted(b)
	if len(as) != len(bs) {
		return false
	}
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func uniqueSorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	n := 0
	for i, v := range out {
		if i == 0 || v != out[i-1] {
			out[n] = v
			n++
		}
	}
	return out[:n]
}
