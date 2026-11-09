package ir

// Application はデコレータの適用（docs/ir.md「デコレータの適用」の Application）。
type Application struct {
	// Decorator は汎用デコレータの FQN。名前付きなら展開済み。
	Decorator string `json:"decorator"`
	// Via は名前付きデコレータを通して付けたなら、その FQN。
	Via string `json:"via,omitempty"`
	// Args は引数。引数名に対応付け済み。省略された省略可能な引数は含まない。
	Args []Arg `json:"args"`
	Span Span  `json:"span"`
}

// Arg はデコレータの引数の値（docs/ir.md「デコレータの適用」の Arg）。
type Arg struct {
	Param string   `json:"param"`
	Value ArgValue `json:"value"`
}

// ArgValue はデコレータの引数の値（docs/ir.md「デコレータの適用」の ArgValue）。
// 具体型は値型の [LiteralValue]、[RefValue]、[RefsValue]、[VariantValue]、[ListValue]。
type ArgValue interface {
	// Kind は JSON の判別子（"literal"、"ref"、"refs"、"variant"、"list"）。
	Kind() string
	isArgValue()
}

// LiteralValue はリテラルの引数（docs/ir.md「デコレータの適用」の ArgValue kind "literal"）。
type LiteralValue struct {
	Literal Literal `json:"literal"`
}

// RefValue は宣言への参照の引数（docs/ir.md「デコレータの適用」の ArgValue kind "ref"）。
type RefValue struct {
	// Target は宣言の FQN、または op の引数名（"$param:名前"）。
	Target string `json:"target"`
}

// RefsValue は `A | B` の引数（docs/ir.md「デコレータの適用」の ArgValue kind "refs"）。ユニオンは展開済み。
type RefsValue struct {
	Targets []string `json:"targets"`
}

// VariantValue は `ProductPage.Ready` のようなバリアントの引数（docs/ir.md「デコレータの適用」の ArgValue kind "variant"）。
type VariantValue struct {
	// Union はユニオンの FQN。
	Union string `json:"union"`
	Arm   string `json:"arm"`
}

// ListValue は列の引数（docs/ir.md「デコレータの適用」の ArgValue kind "list"）。
type ListValue struct {
	Items []ArgValue `json:"items"`
}

var (
	_ ArgValue = LiteralValue{}
	_ ArgValue = RefValue{}
	_ ArgValue = RefsValue{}
	_ ArgValue = VariantValue{}
	_ ArgValue = ListValue{}
)

// Kind は "literal"。
func (LiteralValue) Kind() string { return "literal" }

// Kind は "ref"。
func (RefValue) Kind() string { return "ref" }

// Kind は "refs"。
func (RefsValue) Kind() string { return "refs" }

// Kind は "variant"。
func (VariantValue) Kind() string { return "variant" }

// Kind は "list"。
func (ListValue) Kind() string { return "list" }

func (LiteralValue) isArgValue() {}
func (RefValue) isArgValue()     {}
func (RefsValue) isArgValue()    {}
func (VariantValue) isArgValue() {}
func (ListValue) isArgValue()    {}

// MarshalJSON は kind を付けて書く。
func (v LiteralValue) MarshalJSON() ([]byte, error) {
	type alias LiteralValue
	return marshalKind("literal", alias(v))
}

// MarshalJSON は kind を付けて書く。
func (v RefValue) MarshalJSON() ([]byte, error) {
	type alias RefValue
	return marshalKind("ref", alias(v))
}

// MarshalJSON は kind を付けて書く。
func (v RefsValue) MarshalJSON() ([]byte, error) {
	type alias RefsValue
	return marshalKind("refs", alias(v))
}

// MarshalJSON は kind を付けて書く。
func (v VariantValue) MarshalJSON() ([]byte, error) {
	type alias VariantValue
	return marshalKind("variant", alias(v))
}

// MarshalJSON は kind を付けて書く。
func (v ListValue) MarshalJSON() ([]byte, error) {
	type alias ListValue
	return marshalKind("list", alias(v))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (v *LiteralValue) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, v) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (v *RefsValue) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, v) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (v *ListValue) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, v) }

var argValueTable = map[string]func([]byte) (ArgValue, error){
	"literal": asValue[ArgValue, LiteralValue],
	"ref":     asValue[ArgValue, RefValue],
	"refs":    asValue[ArgValue, RefsValue],
	"variant": asValue[ArgValue, VariantValue],
	"list":    asValue[ArgValue, ListValue],
}

// UnmarshalArgValue は kind で分けて ArgValue を読む（docs/ir.md「JSON」）。
func UnmarshalArgValue(data []byte) (ArgValue, error) {
	return decodeUnion("arg value", data, argValueTable)
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (a Application) MarshalJSON() ([]byte, error) {
	type alias Application
	return marshalStruct(alias(a))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (a *Application) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, a) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (a *Arg) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, a) }

// Bind は bind 宣言（docs/ir.md「bind」）。
type Bind struct {
	// Namespace は bind を書いた名前空間の FQN。
	Namespace string `json:"namespace"`
	// State は状態の FQN。
	State  string     `json:"state"`
	Target BindTarget `json:"target"`
	Span   Span       `json:"span"`
}

// BindTarget は bind の対象（docs/ir.md「bind」の target）。
type BindTarget struct {
	// Namespace は対象の名前空間の FQN。
	Namespace string `json:"namespace"`
	// Deep は `**` か。
	Deep bool `json:"deep"`
}
