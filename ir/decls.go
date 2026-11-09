package ir

import (
	"encoding/json"
	"fmt"
)

// Model は model 宣言（docs/ir.md「model / scalar / union」の Model）。
type Model struct {
	DeclBase
	// Fields はフィールドの列。`is X omit ...` なら展開済み。
	Fields []Field `json:"fields"`
	// DerivedFrom は `is X omit ...` の記録。
	DerivedFrom *DerivedFrom `json:"derivedFrom,omitempty"`
	// SynthesizedFrom はその場で書いた model の出どころ（持ち主の FQN。"server.OrderPage.Ready"）。
	SynthesizedFrom string `json:"synthesizedFrom,omitempty"`
}

// DerivedFrom は `model P is U omit a, b;` の記録（docs/ir.md「model / scalar / union」の derivedFrom）。
type DerivedFrom struct {
	// Base は元の model の FQN。
	Base string `json:"base"`
	// Omit は除いたフィールド名。
	Omit []string `json:"omit"`
}

// Field は model のフィールド（docs/ir.md「model / scalar / union」の Field）。
type Field struct {
	Name        string `json:"name"`
	Description *Doc   `json:"description,omitempty"`
	// Type はフィールドの型。省略可能なら optional。
	Type Type `json:"type"`
	// Default は既定値。
	Default    Literal       `json:"default,omitempty"`
	Decorators []Application `json:"decorators"`
	Span       Span          `json:"span"`
}

// Scalar は scalar 宣言（docs/ir.md「model / scalar / union」の Scalar）。
type Scalar struct {
	DeclBase
	// Base は core か、別の scalar への ref。
	Base Type `json:"base"`
}

// UnionVariant は Union の種類（docs/ir.md「model / scalar / union」の variant）。
type UnionVariant string

// Union の種類。arms は model のバリアント、set は例外か効果の集合。
const (
	UnionArms UnionVariant = "arms"
	UnionSet  UnionVariant = "set"
)

// Union は union 宣言（docs/ir.md「model / scalar / union」の Union）。
// Arms と Members はどちらも常に JSON に出し、使わない方は []。
type Union struct {
	DeclBase
	Variant UnionVariant `json:"variant"`
	// Arms は Variant が arms のときのアーム。
	Arms []UnionArm `json:"arms"`
	// Members は Variant が set のときのメンバー。メンバーがユニオンなら展開済み。
	Members []UnionMember `json:"members"`
}

// UnionArm は arms のユニオンのアーム（docs/ir.md「model / scalar / union」の arms）。
type UnionArm struct {
	Name        string `json:"name"`
	Type        Type   `json:"type"`
	Description *Doc   `json:"description,omitempty"`
	Span        Span   `json:"span"`
}

// UnionMember は set のユニオンのメンバー（docs/ir.md「model / scalar / union」の members）。
type UnionMember struct {
	// Target は例外か効果の FQN。
	Target      string `json:"target"`
	Description *Doc   `json:"description,omitempty"`
	Span        Span   `json:"span"`
}

// StateShape は状態の形（docs/ir.md「state / exception / effect」の shape）。
type StateShape string

// 状態の形。
const (
	StateList   StateShape = "list"
	StateSingle StateShape = "single"
)

// State は state 宣言（docs/ir.md「state / exception / effect」の State）。
type State struct {
	DeclBase
	Shape   StateShape `json:"shape"`
	Element Type       `json:"element"`
}

// Exception は exception 宣言（docs/ir.md「state / exception / effect」の Exception）。
type Exception struct {
	DeclBase
	// Value は運ぶ値の型。ref か、SynthesizedFrom を持つ model への ref。
	Value Type `json:"value,omitempty"`
}

// Operation は状態への操作（docs/ir.md「state / exception / effect」の operation）。
type Operation string

// 状態への操作。
const (
	OpPost   Operation = "post"
	OpPut    Operation = "put"
	OpDelete Operation = "delete"
)

// Effect は effect 宣言（docs/ir.md「state / exception / effect」の Effect）。
type Effect struct {
	DeclBase
	// State は状態の FQN。
	State     string    `json:"state"`
	Operation Operation `json:"operation"`
}

// Classify は classify 宣言（docs/ir.md「classify」）。
type Classify struct {
	DeclBase
	Subject Type `json:"subject"`
	// Cases はケース。default は含まない。
	Cases      []ClassifyCase `json:"cases"`
	HasDefault bool           `json:"hasDefault"`
	// DefaultDescription は default に付いた DocComment。
	DefaultDescription *Doc `json:"defaultDescription,omitempty"`
	// FromCondition は condition の糖衣から来たか（Cases は一つ、HasDefault は true）。
	FromCondition bool `json:"fromCondition"`
}

// ClassifyCase は classify のケース（docs/ir.md「classify」の cases）。
type ClassifyCase struct {
	Name        string `json:"name"`
	Description *Doc   `json:"description,omitempty"`
	Span        Span   `json:"span"`
}

// DecoratorTarget はデコレータを付けられる宣言の種類（docs/ir.md「decorator」の targets）。
type DecoratorTarget string

// デコレータの対象。
const (
	TargetOp        DecoratorTarget = "op"
	TargetField     DecoratorTarget = "field"
	TargetState     DecoratorTarget = "state"
	TargetException DecoratorTarget = "exception"
	TargetScalar    DecoratorTarget = "scalar"
	TargetModel     DecoratorTarget = "model"
)

// Decorator は decorator 宣言（docs/ir.md「decorator」）。
// Alias を持つ宣言は Params / Targets / Throws / Catches を持たず（JSON では []）、Alias.Target のものを使う。
type Decorator struct {
	DeclBase
	Params []Param `json:"params"`
	// Targets は対象。表面で省略されていれば ["op"]（正規化の側が埋める）。
	Targets []DecoratorTarget `json:"targets"`
	// External は @external が付いているか（DEC-10）。
	External bool            `json:"external"`
	Throws   []ThrowsClause  `json:"throws"`
	Catches  []CatchesClause `json:"catches"`
	// Alias は名前付きデコレータの記録。
	Alias *DecoratorAlias `json:"alias,omitempty"`
}

// DecoratorAlias は名前付きデコレータの中身（docs/ir.md「decorator」の alias）。
type DecoratorAlias struct {
	// Target は汎用デコレータの FQN。
	Target string `json:"target"`
	Args   []Arg  `json:"args"`
}

// Param はデコレータの引数（docs/ir.md「decorator」の Param）。説明は DeclBase.Description.Params から引く。
type Param struct {
	Name     string    `json:"name"`
	Type     ParamType `json:"type"`
	Optional bool      `json:"optional"`
}

// ParamType はデコレータの引数の型（docs/ir.md「decorator」の ParamType）。
// 具体型は値型の [TypeParam]（kind "type"）と [AnyParam]（kind "any"）。
type ParamType interface {
	// Kind は JSON の判別子（"type" か "any"）。
	Kind() string
	isParamType()
}

// TypeParam は型で受ける引数（docs/ir.md「decorator」の ParamType kind "type"）。
type TypeParam struct {
	// Types は受け付ける型。`int | float` なら二つ。普通は一つ。
	Types []Type `json:"types"`
}

// AnyOf は any で受ける宣言の種類（docs/ir.md「decorator」の ParamType kind "any" の of）。
type AnyOf string

// any の種類。
const (
	AnyException AnyOf = "exception"
	AnyEffect    AnyOf = "effect"
	AnyState     AnyOf = "state"
	AnyOp        AnyOf = "op"
	AnyModel     AnyOf = "model"
	AnyVariant   AnyOf = "variant"
)

// AnyParam は `any exception` のように宣言の参照で受ける引数（docs/ir.md「decorator」の ParamType kind "any"）。
type AnyParam struct {
	Of AnyOf `json:"of"`
	// On は `any effect on s.op` の制約。
	On *EffectOn `json:"on,omitempty"`
}

// EffectOn は `any effect on <state>.<operation>` の制約（docs/ir.md「decorator」の on）。
type EffectOn struct {
	// State は状態の FQN。
	State     string    `json:"state"`
	Operation Operation `json:"operation"`
}

var (
	_ ParamType = TypeParam{}
	_ ParamType = AnyParam{}
)

// Kind は "type"。
func (TypeParam) Kind() string { return "type" }

// Kind は "any"。
func (AnyParam) Kind() string { return "any" }

func (TypeParam) isParamType() {}
func (AnyParam) isParamType()  {}

// MarshalJSON は kind を付けて書く。
func (p TypeParam) MarshalJSON() ([]byte, error) {
	type alias TypeParam
	return marshalKind("type", alias(p))
}

// MarshalJSON は kind を付けて書く。
func (p AnyParam) MarshalJSON() ([]byte, error) {
	type alias AnyParam
	return marshalKind("any", alias(p))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (p *TypeParam) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, p) }

var paramTypeTable = map[string]func([]byte) (ParamType, error){
	"type": asValue[ParamType, TypeParam],
	"any":  asValue[ParamType, AnyParam],
}

// UnmarshalParamType は kind で分けて ParamType を読む（docs/ir.md「JSON」）。
func UnmarshalParamType(data []byte) (ParamType, error) {
	return decodeUnion("paramType", data, paramTypeTable)
}

// ThrowsClause は throws の一節（docs/ir.md「decorator」の ThrowsClause）。複数行の throws は節の列（docs/ir.md「正規化」）。
type ThrowsClause struct {
	// Exceptions は例外の FQN。ユニオンは展開済み。デコレータの引数名なら "$param:名前"。
	Exceptions []string   `json:"exceptions"`
	Relations  []Relation `json:"relations"`
}

// RelationKind は例外と効果の関係の種類（docs/ir.md「decorator」の Relation）。
type RelationKind string

// 関係の種類。
const (
	Prevents RelationKind = "prevents"
	Keeps    RelationKind = "keeps"
)

// Relation は例外が効果に及ぼす関係（docs/ir.md「decorator」の Relation）。
type Relation struct {
	Kind    RelationKind    `json:"kind"`
	Effects RelationEffects `json:"effects"`
}

// RelationEffects は Relation の effects: "all"（`effects` と書いたもの）か効果の FQN の列
// （docs/ir.md「decorator」の Relation）。All が true なら JSON は "all"、そうでなければ列。
type RelationEffects struct {
	All     bool
	Effects []string
}

// AllEffects は `effects`（すべての効果）を表す RelationEffects。
func AllEffects() RelationEffects { return RelationEffects{All: true} }

// MarshalJSON は "all" か FQN の列を書く。
func (e RelationEffects) MarshalJSON() ([]byte, error) {
	if e.All {
		return []byte(`"all"`), nil
	}
	if e.Effects == nil {
		return []byte(`[]`), nil
	}
	return json.Marshal(e.Effects)
}

// UnmarshalJSON は "all" か FQN の列を読む。
func (e *RelationEffects) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s != "all" {
			return fmt.Errorf("ir: relation effects: unknown value %q", s)
		}
		*e = RelationEffects{All: true}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("ir: relation effects: want \"all\" or array: %w", err)
	}
	if len(list) == 0 {
		list = nil
	}
	*e = RelationEffects{Effects: list}
	return nil
}

// CatchMode は catches の扱い（docs/ir.md「decorator」の CatchesClause）。
type CatchMode string

// catches の扱い。
const (
	Recovers CatchMode = "recovers"
	Halts    CatchMode = "halts"
	Ignores  CatchMode = "ignores"
)

// CatchesClause は catches の一節（docs/ir.md「decorator」の CatchesClause）。
type CatchesClause struct {
	// Exceptions は ThrowsClause.Exceptions と同じ形。
	Exceptions []string  `json:"exceptions"`
	Mode       CatchMode `json:"mode"`
	// Halts は Mode が halts のときの効果。FQN か "$param:名前"。
	Halts string `json:"halts,omitempty"`
}

// Op は op 宣言（docs/ir.md「op」）。
type Op struct {
	DeclBase
	Params []OpParam `json:"params"`
	// Returns は戻り値の型。nil は void（JSON では null）。
	Returns Type `json:"returns"`
	// Effects は書かれた effects の FQN。配線で省略していれば空。
	Effects []string `json:"effects"`
	// Throws は書かれた throws。
	Throws []ThrowsClause `json:"throws"`
	// Body は配線。nil は葉（JSON では null）。
	Body *Wiring `json:"body"`
	// Signature は計算結果。自分の名前空間から見た関数の型（docs/check.md「計算」）。
	Signature Signature `json:"signature"`
}

// OpParam は op の引数（docs/ir.md「op」の OpParam）。説明は Op.Description.Params から引く。
type OpParam struct {
	Name string `json:"name"`
	Type Type   `json:"type"`
	Span Span   `json:"span"`
}

// EffectState は効果の状態（docs/ir.md「op」の Signature の state）。
type EffectState string

// 効果の状態。
const (
	Prevented EffectState = "prevented"
	Kept      EffectState = "kept"
	Unknown   EffectState = "unknown"
)

// Signature は docs/check.md「計算」の Sig(O, v) の結果（docs/ir.md「op」の Signature）。
type Signature struct {
	// Exceptions は v から見て投げうる例外の FQN。
	Exceptions []string `json:"exceptions"`
	// Effects は伝播を含むすべての効果と、デコレータの halts の効果。
	Effects []string `json:"effects"`
	// Relations は Exceptions × Effects のすべての組。
	Relations []SignatureRelation `json:"relations"`
	// SuccessEffects は成功のアームでの効果の状態。
	SuccessEffects []SuccessEffect `json:"successEffects"`
}

// SignatureRelation は例外が起きたときの効果の状態（docs/ir.md「op」の Signature の relations）。
type SignatureRelation struct {
	Exception string      `json:"exception"`
	Effect    string      `json:"effect"`
	State     EffectState `json:"state"`
}

// SuccessEffect は成功のアームでの効果の状態（docs/ir.md「op」の Signature の successEffects）。
type SuccessEffect struct {
	Effect string      `json:"effect"`
	State  EffectState `json:"state"`
}

// Decl の実装。

// Kind は "model"。
func (*Model) Kind() string { return "model" }

// Kind は "scalar"。
func (*Scalar) Kind() string { return "scalar" }

// Kind は "union"。
func (*Union) Kind() string { return "union" }

// Kind は "state"。
func (*State) Kind() string { return "state" }

// Kind は "exception"。
func (*Exception) Kind() string { return "exception" }

// Kind は "effect"。
func (*Effect) Kind() string { return "effect" }

// Kind は "classify"。
func (*Classify) Kind() string { return "classify" }

// Kind は "decorator"。
func (*Decorator) Kind() string { return "decorator" }

// Kind は "op"。
func (*Op) Kind() string { return "op" }

func (*Model) isDecl()     {}
func (*Scalar) isDecl()    {}
func (*Union) isDecl()     {}
func (*State) isDecl()     {}
func (*Exception) isDecl() {}
func (*Effect) isDecl()    {}
func (*Classify) isDecl()  {}
func (*Decorator) isDecl() {}
func (*Op) isDecl()        {}

var (
	_ Decl = (*Model)(nil)
	_ Decl = (*Scalar)(nil)
	_ Decl = (*Union)(nil)
	_ Decl = (*State)(nil)
	_ Decl = (*Exception)(nil)
	_ Decl = (*Effect)(nil)
	_ Decl = (*Classify)(nil)
	_ Decl = (*Decorator)(nil)
	_ Decl = (*Op)(nil)
)

// MarshalJSON は kind を付けて書く。
func (d Model) MarshalJSON() ([]byte, error) { type alias Model; return marshalKind("model", alias(d)) }

// MarshalJSON は kind を付けて書く。
func (d Scalar) MarshalJSON() ([]byte, error) {
	type alias Scalar
	return marshalKind("scalar", alias(d))
}

// MarshalJSON は kind を付けて書く。
func (d Union) MarshalJSON() ([]byte, error) { type alias Union; return marshalKind("union", alias(d)) }

// MarshalJSON は kind を付けて書く。
func (d State) MarshalJSON() ([]byte, error) { type alias State; return marshalKind("state", alias(d)) }

// MarshalJSON は kind を付けて書く。
func (d Exception) MarshalJSON() ([]byte, error) {
	type alias Exception
	return marshalKind("exception", alias(d))
}

// MarshalJSON は kind を付けて書く。
func (d Effect) MarshalJSON() ([]byte, error) {
	type alias Effect
	return marshalKind("effect", alias(d))
}

// MarshalJSON は kind を付けて書く。
func (d Classify) MarshalJSON() ([]byte, error) {
	type alias Classify
	return marshalKind("classify", alias(d))
}

// MarshalJSON は kind を付けて書く。
func (d Decorator) MarshalJSON() ([]byte, error) {
	type alias Decorator
	return marshalKind("decorator", alias(d))
}

// MarshalJSON は kind を付けて書く。
func (d Op) MarshalJSON() ([]byte, error) { type alias Op; return marshalKind("op", alias(d)) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Model) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Scalar) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Union) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *State) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Exception) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Effect) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Classify) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Decorator) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Op) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// 残りの構造体の JSON（nil の列を [] にし、ユニオンのフィールドを読むため）。

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (x DerivedFrom) MarshalJSON() ([]byte, error) {
	type alias DerivedFrom
	return marshalStruct(alias(x))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *DerivedFrom) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (x Field) MarshalJSON() ([]byte, error) { type alias Field; return marshalStruct(alias(x)) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *Field) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *UnionArm) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (x DecoratorAlias) MarshalJSON() ([]byte, error) {
	type alias DecoratorAlias
	return marshalStruct(alias(x))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *DecoratorAlias) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *Param) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (x ThrowsClause) MarshalJSON() ([]byte, error) {
	type alias ThrowsClause
	return marshalStruct(alias(x))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *ThrowsClause) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (x CatchesClause) MarshalJSON() ([]byte, error) {
	type alias CatchesClause
	return marshalStruct(alias(x))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *CatchesClause) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *OpParam) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (x Signature) MarshalJSON() ([]byte, error) {
	type alias Signature
	return marshalStruct(alias(x))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *Signature) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }
