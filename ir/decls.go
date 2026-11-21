package ir

import (
	"encoding/json"
	"fmt"
)

// Model は model 宣言（docs/ir.md「model / scalar / union」の Model）。
type Model struct {
	DeclBase
	// Fields はフィールドの列。継承した model なら、受け継いだ分も入れた展開済みの並び
	// （docs/types.md「継承した model」の「フィールド」）。
	// インデックスの model なら空。
	Fields []Field `json:"fields"`
	// Index は、インデックスの model のインデックス（`model Attrs { [name: string]: string[]; }`。
	// docs/types.md「インデックスの model」）。ふつうの model なら nil。
	Index *ModelIndex `json:"index,omitempty"`
	// Extends は継承した model（`model D extends B { ... }`）の記録。
	Extends *ModelExtends `json:"extends,omitempty"`
	// SynthesizedFrom はその場で書いた model の出どころ（持ち主の FQN。"server.OrderPage.Ready"）。
	SynthesizedFrom string `json:"synthesizedFrom,omitempty"`
}

// ModelIndex は、インデックスの model のインデックス（docs/ir.md「model / scalar / union」の index）。
type ModelIndex struct {
	// Name は、キーの名前（説明のための名前）。
	Name string `json:"name"`
	// Key は、キーの型（string か、元が string の scalar）。
	Key Type `json:"key"`
	// Value は、値の型。
	Value Type `json:"value"`
	// Span は、インデックスの位置（`[` から `;` まで）。
	Span Span `json:"span"`
}

// ModelExtends は `model D extends B { ... }` の記録（docs/ir.md「model / scalar / union」の extends）。
// 足したフィールドは、Fields のうち、元の model の Fields に名前がないもの。
type ModelExtends struct {
	// Base は元の model の FQN（直接の元）。
	Base string `json:"base"`
	// Removed は `override f: never;` でなくしたフィールドの名前（書かれた順）。
	Removed []string `json:"removed"`
	// Overridden は `override` で型を変えたか、デコレータを足したフィールドの名前（書かれた順）。
	// これらのフィールドの説明は、`override` に書いたもの（なければなし）。
	Overridden []string `json:"overridden"`
}

// Field は model のフィールド（docs/ir.md「model / scalar / union」の Field）。
type Field struct {
	Name        string `json:"name"`
	Description *Doc   `json:"description,omitempty"`
	// Type はフィールドの型。省略可能なら optional。
	Type       Type          `json:"type"`
	Decorators []Application `json:"decorators"`
	// Span は位置。受け継いだままのフィールドは元の model のフィールドの位置、`override` したフィールドは `override` の位置。
	Span Span `json:"span"`
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

// StateShape は状態の形（docs/ir.md「state / exception / effect」の shape）。列数の範囲で決まる
// （docs/check.md「状態の形」）。
type StateShape string

// 状態の形。list は 0 以上、nullable は 0 か 1、single はちょうど 1。
const (
	StateList     StateShape = "list"
	StateNullable StateShape = "nullable"
	StateSingle   StateShape = "single"
)

// 標準の分類のケース名（docs/check.md「標準の分類」）。基底の状態の組み込みの分類は、これを状態に当てたもの。
// 名前は、形ごとに違う: list は Empty / HasItems、nullable は None / Some。
const (
	CaseEmpty    = "Empty"
	CaseHasItems = "HasItems"
	CaseNone     = "None"
	CaseSome     = "Some"
)

// 標準の分類の名前（docs/check.md「標準の分類」）。Match.Classify に、FQN の代わりに置く。
const (
	ClassifyList     = "list"
	ClassifyNullable = "nullable"
)

// IsStandardClassify は name が標準の分類の名前（"list" か "nullable"）か。
func IsStandardClassify(name string) bool { return name == ClassifyList || name == ClassifyNullable }

// StandardCases は、標準の分類 name（"list" か "nullable"）のケース名の列: 値や要素がない側、ある側の順
// （list は Empty、HasItems。nullable は None、Some）。標準の分類の名前でなければ nil。毎回新しい列を返す。
func StandardCases(name string) []string {
	switch name {
	case ClassifyList:
		return []string{CaseEmpty, CaseHasItems}
	case ClassifyNullable:
		return []string{CaseNone, CaseSome}
	}
	return nil
}

// ShapeOf は状態の型から形と要素を決める（docs/check.md「状態の形」の表）: 最も外側が array なら list、
// optional なら nullable、それ以外は single。list と nullable の要素は of、single は型そのもの。
func ShapeOf(t Type) (StateShape, Type) {
	switch x := t.(type) {
	case ArrayType:
		return StateList, x.Of
	case OptionalType:
		return StateNullable, x.Of
	}
	return StateSingle, t
}

// HasBuiltinClassify は、この形の状態が組み込みの分類を持つか（list と nullable）。
// single は常に一列なので持たない（docs/check.md「状態の形」）。
func (s StateShape) HasBuiltinClassify() bool { return s == StateList || s == StateNullable }

// BuiltinCases は、この形の状態の、組み込みの分類のケース名の列（[StandardCases] と同じ順）。組み込みの分類を
// 持たない形（single）なら nil。
func (s StateShape) BuiltinCases() []string { return StandardCases(string(s)) }

// State は state 宣言（docs/ir.md「state / exception / effect」の State）。
type State struct {
	DeclBase
	// Type は状態の型。配線が名前で参照したときの値の型（docs/types.md「形の書き方」）。
	Type Type `json:"type"`
	// Shape は列数の範囲。Type の最も外側から決まる（[ShapeOf]）。
	Shape StateShape `json:"shape"`
	// Element は要素の型。Type から決まる（[ShapeOf]）。optional にはならない（STATE-3。当たった状態は optional を外して載せる）。
	Element Type `json:"element"`
}

// Exception は exception 宣言（docs/ir.md「state / exception / effect」の Exception）。
type Exception struct {
	DeclBase
	// Params は例外が運ぶ値（`exception X(a: T, b: U);`）。値を運ばない例外は空（JSON では []）。
	Params []OpParam `json:"params"`
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
	// Params は効果が受け取る値（`effect fn e(order: Order): effects …;`）。なければ空（JSON では []）。
	Params []OpParam `json:"params"`
	// Throws はこの効果が投げうる例外の FQN（`: throws E effects …`。docs/check.md「read と効果」）。ユニオンは展開済み。
	// 書いた順で、なければ空（JSON では []）。投げたとき、その効果は、起きていない。
	Throws []string `json:"throws"`
	// Target は宛先。一つだけ（docs/check.md「状態の形」の「効果の宛先」）。
	Target EffectTarget `json:"target"`
	// Writes はこの効果が書き込む基底の状態の FQN（docs/check.md「状態の形」の「効果が書き込む状態」）。
	Writes []string `json:"writes"`
	// Row は行の遷移（`as` の節。docs/check.md「行の遷移」）。書いていなければ nil（JSON ではキーごと省く）。
	// 持つのは、Target が [BuiltinTarget] の効果だけ。
	Row *RowTransition `json:"row,omitempty"`
}

// RowTransition は行の遷移（docs/ir.md「state / exception / effect」の RowTransition）: 効果の後、受け取った行が、
// どのケースにあるか。書き込む状態と操作は、[Effect] の Target にある。
type RowTransition struct {
	// Param は行の引数の名前（Effect.Params のうち、型が、状態の要素の型か、それを絞り込んだ型の、ただ一つのもの）。
	// 効果の前のケースは、この引数の型が言う（[CaseType] なら、その Cases。そうでなければ、どのケースでも）。
	Param string `json:"param"`
	// Classify は行の分類の FQN（値の分類）。`as else` では空（JSON ではキーごと省く）。
	Classify string `json:"classify,omitempty"`
	// To は効果の後のケースの名前。空なら、決まっていない（put の `as else`。JSON ではキーごと省く）。post では、いつも持つ。
	To string `json:"to,omitempty"`
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *RowTransition) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// Read は read 宣言（docs/ir.md「read」）。状態を読むことに付けた名前。
type Read struct {
	DeclBase
	// Params は読むときに受け取る値。なければ空（JSON では []）。
	Params []OpParam `json:"params"`
	// Returns は結果の型。nil にも [NeverType] にもならない。
	Returns Type `json:"returns"`
	// State は読む基底の状態の FQN。宛先は一つだけ（docs/check.md「read の宛先」）。
	State string `json:"state"`
}

// EffectTarget は効果の宛先（docs/ir.md「state / exception / effect」の EffectTarget）。
// 具体型は値型の [BuiltinTarget]（kind "builtin"）と [TransitionTarget]（kind "transition"）。
type EffectTarget interface {
	// Kind は JSON の判別子（"builtin" か "transition"）。
	Kind() string
	isEffectTarget()
}

// BuiltinTarget は基底の状態の組み込みの効果（`effects s.post`。docs/ir.md「state / exception / effect」の
// EffectTarget kind "builtin"）。
type BuiltinTarget struct {
	// State は状態の FQN。
	State     string    `json:"state"`
	Operation Operation `json:"operation"`
}

// TransitionTarget は分類の遷移（`effects X as A -> B`。docs/ir.md「state / exception / effect」の
// EffectTarget kind "transition"）。
type TransitionTarget struct {
	// Of は状態の分類の FQN か、組み込みの分類を持つ基底の状態の FQN。
	Of   string  `json:"of"`
	From CaseSet `json:"from"`
	To   CaseSet `json:"to"`
}

var (
	_ EffectTarget = BuiltinTarget{}
	_ EffectTarget = TransitionTarget{}
)

// Kind は "builtin"。
func (BuiltinTarget) Kind() string { return "builtin" }

// Kind は "transition"。
func (TransitionTarget) Kind() string { return "transition" }

func (BuiltinTarget) isEffectTarget()    {}
func (TransitionTarget) isEffectTarget() {}

// MarshalJSON は kind を付けて書く。
func (t BuiltinTarget) MarshalJSON() ([]byte, error) {
	type alias BuiltinTarget
	return marshalKind("builtin", alias(t))
}

// MarshalJSON は kind を付けて書く。
func (t TransitionTarget) MarshalJSON() ([]byte, error) {
	type alias TransitionTarget
	return marshalKind("transition", alias(t))
}

var effectTargetTable = map[string]func([]byte) (EffectTarget, error){
	"builtin":    asValue[EffectTarget, BuiltinTarget],
	"transition": asValue[EffectTarget, TransitionTarget],
}

// UnmarshalEffectTarget は kind で分けて EffectTarget を読む（docs/ir.md「JSON」）。
func UnmarshalEffectTarget(data []byte) (EffectTarget, error) {
	return decodeUnion("effect target", data, effectTargetTable)
}

// CaseSet は遷移の前か後のケースの集合: "any" かケース名の列（書かれた順。docs/ir.md「state / exception / effect」
// の CaseSet）。Any が true なら JSON は "any"、そうでなければ列。
type CaseSet struct {
	Any   bool
	Cases []string
}

// AnyCase は `any`（どのケースでも）を表す CaseSet。
func AnyCase() CaseSet { return CaseSet{Any: true} }

// Cases はケース名の列の CaseSet。
func Cases(names ...string) CaseSet { return CaseSet{Cases: names} }

// MarshalJSON は "any" かケース名の列を書く。
func (c CaseSet) MarshalJSON() ([]byte, error) {
	if c.Any {
		return []byte(`"any"`), nil
	}
	if c.Cases == nil {
		return []byte(`[]`), nil
	}
	return json.Marshal(c.Cases)
}

// UnmarshalJSON は "any" かケース名の列を読む。
func (c *CaseSet) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s != "any" {
			return fmt.Errorf("ir: case set: unknown value %q", s)
		}
		*c = CaseSet{Any: true}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("ir: case set: want \"any\" or array: %w", err)
	}
	if len(list) == 0 {
		list = nil
	}
	*c = CaseSet{Cases: list}
	return nil
}

// Classify は classify 宣言（docs/ir.md「classify」）。
type Classify struct {
	DeclBase
	// Subject は対象。値の分類なら型（[TypeSubject]）、op の分類なら op（[OpSubject]）、
	// 状態の分類なら基底の状態（[StatesSubject]）。
	Subject ClassifySubject `json:"subject"`
	// Cases は宣言したケースのすべて（宣言順）。分類は閉じていて、名前のないケースはない（docs/check.md「閉じた分類」）。
	Cases []ClassifyCase `json:"cases"`
	// DecidedBy はケースが何で決まるか（docs/check.md「ケースの判定」）。type なら述語を持たない。
	DecidedBy DecidedBy `json:"decidedBy"`
	// Absent は、ない側のケース（docs/check.md「ない側のケース」）の名前。なければ空（JSON ではキーごと省く）。
	// DecidedBy が predicate でも、このケースだけは述語を持たず、値がないことで決まる。
	Absent string `json:"absent,omitempty"`
}

// DecidedBy は分類のケースが何で決まるか（docs/ir.md「classify」の decidedBy）。
type DecidedBy string

// ケースの判定。型で決まる分類は述語を持たない（docs/check.md「ケースの判定」）。
const (
	DecidedByType      DecidedBy = "type"
	DecidedByPredicate DecidedBy = "predicate"
	// DecidedByNone は、対象のない分類（`classify C of never`）: 判定するものがなく、述語も持たない
	// （docs/check.md「分類」の「対象のない分類」）。
	DecidedByNone DecidedBy = "none"
)

// IsSubjectless は、対象のない分類（`classify C of never`）か。
func (c *Classify) IsSubjectless() bool { return c.DecidedBy == DecidedByNone }

// NarrowBy は絞り込みの種類（docs/ir.md「classify」の CaseTyping の by。docs/types.md「型で決まる絞り込み」）。
type NarrowBy string

// 絞り込みの種類。presence と variant は型で決まる絞り込み、predicate は述語の要る絞り込み。else は、型を書かない
// `対象 as else`（残りのケース。docs/check.md「残りのケース」）、none は、型を書かない `対象 as none`（ない側のケース。
// 同「ない側のケース」）。
const (
	NarrowByPresence  NarrowBy = "presence"
	NarrowByVariant   NarrowBy = "variant"
	NarrowByPredicate NarrowBy = "predicate"
	NarrowByElse      NarrowBy = "else"
	NarrowByNone      NarrowBy = "none"
)

// CaseTyping はケースの `対象 as T`、`対象 as else`、`対象 as none`（docs/ir.md「classify」の CaseTyping。
// docs/check.md「ケースの型」「残りのケース」「ない側のケース」）。
type CaseTyping struct {
	// State は状態の分類のとき、その対象の状態の FQN。値の分類と op の分類では持たない（対象は一つ）。
	State string `json:"state,omitempty"`
	// Type はそのケースでの対象の型。By が else か none なら、書いた型がないので、docs/types.md「残りのケースでの対象の型」。
	Type Type `json:"type"`
	// By は絞り込みの種類。
	By   NarrowBy `json:"by"`
	Span Span     `json:"span"`
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *CaseTyping) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// ClassifySubject は分類の対象（docs/ir.md「classify」の subject）。
// 具体型は値型の [TypeSubject]（kind "type"）、[OpSubject]（kind "op"）、[StatesSubject]（kind "states"）。
type ClassifySubject interface {
	// Kind は JSON の判別子（"type"、"op"、"states"）。
	Kind() string
	isClassifySubject()
}

// OpSubject は op の分類の対象（docs/ir.md「classify」の subject kind "op"）。
type OpSubject struct {
	// Op は対象の op の FQN。
	Op string `json:"op"`
	// Type はその op の returns（対象の型）。
	Type Type `json:"type"`
}

// TypeSubject は値の分類の対象の型（docs/ir.md「classify」の subject kind "type"）。
type TypeSubject struct {
	Type Type `json:"type"`
}

// StatesSubject は状態の分類の対象（docs/ir.md「classify」の subject kind "states"）。
type StatesSubject struct {
	// States は基底の状態の FQN（書かれた順）。
	States []string `json:"states"`
}

var (
	_ ClassifySubject = TypeSubject{}
	_ ClassifySubject = OpSubject{}
	_ ClassifySubject = StatesSubject{}
)

// Kind は "type"。
func (TypeSubject) Kind() string { return "type" }

// Kind は "op"。
func (OpSubject) Kind() string { return "op" }

func (OpSubject) isClassifySubject() {}

// MarshalJSON は kind を付けて書く。
func (s OpSubject) MarshalJSON() ([]byte, error) {
	type alias OpSubject
	return marshalKind("op", alias(s))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *OpSubject) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }

// Kind は "states"。
func (StatesSubject) Kind() string { return "states" }

func (TypeSubject) isClassifySubject()   {}
func (StatesSubject) isClassifySubject() {}

// MarshalJSON は kind を付けて書く。
func (s TypeSubject) MarshalJSON() ([]byte, error) {
	type alias TypeSubject
	return marshalKind("type", alias(s))
}

// MarshalJSON は kind を付けて書く。
func (s StatesSubject) MarshalJSON() ([]byte, error) {
	type alias StatesSubject
	return marshalKind("states", alias(s))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *TypeSubject) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *StatesSubject) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }

var classifySubjectTable = map[string]func([]byte) (ClassifySubject, error){
	"type":   asValue[ClassifySubject, TypeSubject],
	"op":     asValue[ClassifySubject, OpSubject],
	"states": asValue[ClassifySubject, StatesSubject],
}

// UnmarshalClassifySubject は kind で分けて ClassifySubject を読む（docs/ir.md「JSON」）。
func UnmarshalClassifySubject(data []byte) (ClassifySubject, error) {
	return decodeUnion("classify subject", data, classifySubjectTable)
}

// SubjectType は値の分類と op の分類の対象の型（op の分類ならその op の returns）。
// 状態の分類か、対象が決まっていなければ nil。
func (c *Classify) SubjectType() Type {
	switch s := c.Subject.(type) {
	case TypeSubject:
		return s.Type
	case OpSubject:
		return s.Type
	}
	return nil
}

// SubjectOp は op の分類の対象の op の FQN。op の分類でなければ ok が false。
func (c *Classify) SubjectOp() (op string, ok bool) {
	s, ok := c.Subject.(OpSubject)
	return s.Op, ok
}

// Case はケースを名前で引く。なければ nil。
func (c *Classify) Case(name string) *ClassifyCase {
	for i := range c.Cases {
		if c.Cases[i].Name == name {
			return &c.Cases[i]
		}
	}
	return nil
}

// ElseCase は残りのケース（`対象 as else`。docs/check.md「残りのケース」）。なければ nil。
func (c *Classify) ElseCase() *ClassifyCase {
	for i := range c.Cases {
		if c.Cases[i].IsElse() {
			return &c.Cases[i]
		}
	}
	return nil
}

// SubjectStates は状態の分類の対象の基底の状態の FQN。値の分類か op の分類なら ok が false。
func (c *Classify) SubjectStates() (states []string, ok bool) {
	s, ok := c.Subject.(StatesSubject)
	return s.States, ok
}

// ClassifyCase は classify のケース（docs/ir.md「classify」の cases）。
type ClassifyCase struct {
	Name        string `json:"name"`
	Description *Doc   `json:"description,omitempty"`
	// Decorators は、ケースの前に書いたデコレータ（書かれた順）。なければ空（JSON では []）。
	Decorators []Application `json:"decorators"`
	// As は `対象 as T` と `対象 as else` の並び（書かれた順）。なければ空（JSON では []）。
	As   []CaseTyping `json:"as"`
	Span Span         `json:"span"`
}

// IsElse は、残りのケース（`対象 as else`。docs/check.md「残りのケース」）か。
func (c ClassifyCase) IsElse() bool {
	for _, as := range c.As {
		if as.By == NarrowByElse {
			return true
		}
	}
	return false
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (x ClassifyCase) MarshalJSON() ([]byte, error) {
	type alias ClassifyCase
	return marshalStruct(alias(x))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *ClassifyCase) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

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
	TargetEffect    DecoratorTarget = "effect"
	TargetRead      DecoratorTarget = "read"
	// TargetCase は、分類のケース（`case`、`case of never`、`case of Actor`）。絞りは Decorator.CaseTargets。
	TargetCase DecoratorTarget = "case"
)

// Decorator は decorator 宣言（docs/ir.md「decorator」）。
// Alias を持つ宣言は Params / Targets / Throws / Catches / Reads / Effects を持たず（JSON では []）、Alias.Target のものを使う。
type Decorator struct {
	DeclBase
	Params []Param `json:"params"`
	// Fn は、関数に付けるデコレータ（`decorator fn`。docs/grammar.md「decorator の宣言」）か。真なら Targets は ["op"]。
	// 節（Throws / Catches / Reads / Effects）と Body を持てるのは、Fn が真の宣言だけ。
	Fn bool `json:"fn"`
	// Targets は対象。`decorator fn` なら ["op"]（正規化の側が埋める）。
	Targets []DecoratorTarget `json:"targets"`
	// CaseTargets は、Targets の "case" ごとの絞り（書かれた順）。なければ空（JSON では []）。
	CaseTargets []CaseKind `json:"caseTargets"`
	// External は @external が付いているか（DEC-10）。
	External bool `json:"external"`
	// Repeatable は @repeatable が付いているか。偽なら、一つの宣言に一度だけ付けられる（DEC-13）。
	Repeatable bool            `json:"repeatable"`
	Throws     []ThrowsClause  `json:"throws"`
	Catches    []CatchesClause `json:"catches"`
	// Reads は `reads` の節の要素（書かれた順。節が複数なら連結）。FQN の位置に、デコレータの引数名 "$param:名前" も来る:
	// any state の引数は [DirectRead] の State に。
	// 付いた op に何が運ばれるかは Op.Carried（docs/check.md「デコレータが運ぶ reads と effects」）。
	Reads []ReadRef `json:"reads"`
	// Effects は `effects` の節の要素（同じ。ユニオンは展開済み）。any effect の引数は [NamedEffect] の Effect に "$param:名前"。
	Effects []EffectRef `json:"effects"`
	// Body は、デコレータの本体（docs/grammar.md「デコレータの本体」）。本体のない宣言は nil。
	// 文は op の配線と同じ形で、包んだ op の中身を呼ぶ所は、Callee が [ProceedCallee] の呼び出し。
	// 値の型の引数（Param）は、[NameAtom] の Path の頭の名前で読む。
	Body *Wiring `json:"body,omitempty"`
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
	// Rest は残りの引数（`...名前: 型`。docs/grammar.md「decorator の宣言」）。Params の最後の一つだけが true になりうる。
	// true なら Optional は false で、Type は渡す値一つの型。
	Rest bool `json:"rest"`
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
	// AnyClassifyCase は、分類のケース（`any case`、`any case of never`、`any case of Actor`）。絞りは AnyParam.Case。
	AnyClassifyCase AnyOf = "case"
)

// AnyParam は `any exception` のように宣言の参照で受ける引数（docs/ir.md「decorator」の ParamType kind "any"）。
type AnyParam struct {
	Of AnyOf `json:"of"`
	// On は `any effect on s.op` の制約。
	On *EffectOn `json:"on,omitempty"`
	// Case は、Of が case のときの、分類のケースの絞り。ほかは nil。
	Case *CaseKind `json:"case,omitempty"`
}

// CaseKind は、デコレータの宣言で言う、分類のケースの絞り（docs/ir.md「decorator」の CaseKind）。立つのは、どちらか一つか、
// なし（`case of any` と、その省略の `case`）。
type CaseKind struct {
	// Classify は、分類の名前で絞ったとき（`case of Actor`）の、分類の FQN。
	Classify string `json:"classify,omitempty"`
	// Subject は、対象の型で絞ったとき（`case of Order`、`case of never`）の、その型。`of never` は NeverType。
	Subject Type `json:"subject,omitempty"`
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (k CaseKind) MarshalJSON() ([]byte, error) { type alias CaseKind; return marshalStruct(alias(k)) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (k *CaseKind) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, k) }

// Matches は、分類 c のケースが、この絞りに合うか（docs/types.md「デコレータの引数」）。分類の名前なら、その分類。
// 対象の型なら、値の分類で、対象の型が、その型と等しいもの。絞りがなければ、どの分類でも。
func (k CaseKind) Matches(c *Classify) bool {
	if c == nil {
		return false
	}
	if k.Classify != "" {
		return k.Classify == c.Name
	}
	if k.Subject != nil {
		s, ok := c.Subject.(TypeSubject)
		return ok && TypeEqual(s.Type, k.Subject)
	}
	return true
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
// 目的語は常にその op のすべての効果（`effects`）なので、種類だけを持つ。
type Relation struct {
	Kind RelationKind `json:"kind"`
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
	// Exported は `export` が付いているか（docs/check.md「export」）。
	Exported bool      `json:"exported"`
	Params   []OpParam `json:"params"`
	// Returns は戻り値の型。nil は void（JSON では null）。決して返さない op は [NeverType]。
	Returns Type `json:"returns"`
	// Effects は書かれた effects（書かれた順。ユニオンは展開済み）。配線で省略していれば空。
	// デコレータが運ぶものは含まない。
	Effects []EffectRef `json:"effects"`
	// Reads は書かれた reads（書かれた順、重複は最初の出現に寄せる）。書いていなければ空。
	// デコレータが運ぶものは含まない。
	Reads []ReadRef `json:"reads"`
	// Carried はデコレータの reads / effects の節が、この op に運ぶもの
	// （docs/check.md「デコレータが運ぶ reads と effects」）。
	Carried Carried `json:"carried"`
	// Throws は書かれた throws。
	Throws []ThrowsClause `json:"throws"`
	// Body は配線。nil は葉（JSON では null）。
	Body *Wiring `json:"body"`
	// Signature は計算結果。自分の名前空間から見た関数の型（docs/check.md「計算」）。
	Signature Signature `json:"signature"`
}

// Carried はデコレータが op に運ぶもの（docs/ir.md「op」の carried）。書いたもの（Op.Reads / Op.Effects）とは分けて持つ。
type Carried struct {
	// Reads は運ばれた reads の要素。decorators の順、節の順、引数に書いた順。重複は最初の出現に寄せる。
	Reads []ReadRef `json:"reads"`
	// Effects は運ばれた effects の要素（ユニオンは展開済み）。同じ順。
	Effects []EffectRef `json:"effects"`
}

// MarshalJSON は docs/ir.md「JSON」の形で書く（空の列は []）。
func (x Carried) MarshalJSON() ([]byte, error) { type alias Carried; return marshalStruct(alias(x)) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *Carried) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// DirectReads は葉の op が直に読むもの: Reads の後ろに Carried.Reads を並べ、重複を最初の出現に寄せたもの
// （docs/check.md「デコレータが運ぶ reads と effects」）。配線の op が読むものは、本体が行き着くものを含む
// Signature.Reads にあり、これでは求まらない。直に読む状態は [Program.ReadStates] で求める。
func (o *Op) DirectReads() []ReadRef { return mergeUnique(o.Reads, o.Carried.Reads) }

// DirectEffects は葉の op が直に持つ効果: Effects の後ろに Carried.Effects を並べ、重複を最初の出現に寄せたもの
// （同上）。配線の op の効果は Signature.Effects にある。デコレータの halts の効果は含まない。
func (o *Op) DirectEffects() []EffectRef { return mergeUnique(o.Effects, o.Carried.Effects) }

// mergeUnique は二つの列を順に並べ、重複を最初の出現に寄せる。
func mergeUnique[T comparable](a, b []T) []T {
	var out []T
	seen := map[T]bool{}
	for _, list := range [][]T{a, b} {
		for _, s := range list {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
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
	// Effects は伝播を含むすべての効果（op が書いたものと、デコレータが運ぶもの）と、デコレータの halts の効果。
	Effects []EffectRef `json:"effects"`
	// Relations は Exceptions × Effects のすべての組。
	Relations []SignatureRelation `json:"relations"`
	// SuccessEffects は成功のアームでの効果の状態。
	SuccessEffects []SuccessEffect `json:"successEffects"`
	// Reads は読む基底の状態の FQN。伝播と、デコレータが運ぶものを含む（docs/check.md「reads」の R）。見る側によらない。
	// read の宣言は、その State として入る。
	Reads []string `json:"reads"`
	// Pure は純粋か（docs/check.md「純粋」）。見る側によらない。
	Pure bool `json:"pure"`
}

// SignatureRelation は例外が起きたときの効果の状態（docs/ir.md「op」の Signature の relations）。
type SignatureRelation struct {
	Exception string      `json:"exception"`
	Effect    EffectRef   `json:"effect"`
	State     EffectState `json:"state"`
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *SignatureRelation) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// SuccessEffect は成功のアームでの効果の状態（docs/ir.md「op」の Signature の successEffects）。
type SuccessEffect struct {
	Effect EffectRef   `json:"effect"`
	State  EffectState `json:"state"`
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *SuccessEffect) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

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

// Kind は "read"。
func (*Read) Kind() string { return "read" }

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
func (*Read) isDecl()      {}
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
	_ Decl = (*Read)(nil)
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
func (d Read) MarshalJSON() ([]byte, error) { type alias Read; return marshalKind("read", alias(d)) }

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
func (d *Read) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Classify) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Decorator) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Op) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// 残りの構造体の JSON（nil の列を [] にし、ユニオンのフィールドを読むため）。

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (x ModelExtends) MarshalJSON() ([]byte, error) {
	type alias ModelExtends
	return marshalStruct(alias(x))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *ModelExtends) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (x ModelIndex) MarshalJSON() ([]byte, error) {
	type alias ModelIndex
	return marshalStruct(alias(x))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (x *ModelIndex) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, x) }

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
