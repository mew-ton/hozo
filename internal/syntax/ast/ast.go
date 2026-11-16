// Package ast は .hozo の構文木。docs/grammar.md の非終端記号ごとに一つの構造体を持ち、
// すべての節はバイトの範囲（source.Span）を持つ。DocComment は付いた先の節が持つ
// （docs/packages.md「internal/syntax/token、internal/syntax/ast、internal/syntax」）。
//
// 構文木は表面の形をそのまま持ち、名前の解決や正規化はしない（docs/ir.md が持ち主）。
package ast

import "github.com/mew-ton/hozo/internal/source"

// Node は構文木のすべての節。
type Node interface {
	// NodeSpan は節の範囲。
	NodeSpan() source.Span
}

// Ident は識別子（docs/grammar.md「識別子」）。フィールド名の位置（Field.Name、Override.Name、
// RecordField.Name、NameAtom.Name の二つ目以降の要素）では FieldName で、Name が予約語のこともある。
type Ident struct {
	Span source.Span
	Name string
}

// NodeSpan は Node の実装。
func (n *Ident) NodeSpan() source.Span { return n.Span }

// QualifiedName は "." でつないだ識別子（docs/grammar.md「識別子」）。Segments は一つ以上。
type QualifiedName struct {
	Span     source.Span
	Segments []Ident
}

// NodeSpan は Node の実装。
func (n *QualifiedName) NodeSpan() source.Span { return n.Span }

// String は "." でつないだ名前を返す。
func (n *QualifiedName) String() string {
	s := ""
	for i, seg := range n.Segments {
		if i > 0 {
			s += "."
		}
		s += seg.Name
	}
	return s
}

// Last は最後の識別子。
func (n *QualifiedName) Last() Ident { return n.Segments[len(n.Segments)-1] }

// DocComment はドキュメンテーションコメント（docs/grammar.md「ドキュメンテーションコメント」）。
type DocComment struct {
	Span source.Span
	// Raw は /** と */ を除いた中身。
	Raw string
	// Body は本文。各行の先頭の空白と * とその後ろの空白一つを落とし、改行でつないだもの。
	// タグの行は含まない。前後の空行は落とす。
	Body string
	// Tags は @param / @throws / @see の行。
	Tags []DocTag
}

// NodeSpan は Node の実装。
func (n *DocComment) NodeSpan() source.Span { return n.Span }

// DocTagKind はタグの種類。
type DocTagKind int

const (
	// DocParam は @param 名前 説明。
	DocParam DocTagKind = iota
	// DocThrows は @throws 例外 説明。
	DocThrows
	// DocSee は @see 名前。
	DocSee
)

// String はタグの名前（"@param" など）。
func (k DocTagKind) String() string {
	switch k {
	case DocParam:
		return "@param"
	case DocThrows:
		return "@throws"
	case DocSee:
		return "@see"
	}
	return "@?"
}

// DocTag はタグの行。
type DocTag struct {
	Span source.Span
	Kind DocTagKind
	// Name は @param の引数名、@throws の例外、@see の名前。
	Name QualifiedName
	// Text は説明。@see と、説明を省いたときは空。
	Text string
}

// NodeSpan は Node の実装。
func (n *DocTag) NodeSpan() source.Span { return n.Span }

// LiteralKind はリテラルの種類。
type LiteralKind int

const (
	// StringLit は文字列。
	StringLit LiteralKind = iota
	// IntLit は整数。
	IntLit
	// FloatLit は小数。
	FloatLit
	// BoolLit は true / false。
	BoolLit
	// DurationLit は期間。
	DurationLit
)

// Literal はリテラル（docs/grammar.md「リテラル」）。Kind に応じた一つの値のフィールドだけが意味を持つ。
type Literal struct {
	Span source.Span
	Kind LiteralKind
	// Raw は元の字句。
	Raw string
	// Str はエスケープを戻した文字列（StringLit）。
	Str string
	// Int は整数の値（IntLit）。
	Int int64
	// Float は小数の値（FloatLit）。
	Float float64
	// Bool は真偽の値（BoolLit）。
	Bool bool
	// Duration はナノ秒で表した期間（DurationLit）。
	Duration int64
}

// NodeSpan は Node の実装。
func (n *Literal) NodeSpan() source.Span { return n.Span }

func (n *Literal) exprNode()    {}
func (n *Literal) atomNode()    {}
func (n *Literal) armBodyNode() {}

// File は一つの .hozo（docs/grammar.md「ファイルと宣言」）。
type File struct {
	Span  source.Span
	Decls []Decl
}

// NodeSpan は Node の実装。
func (n *File) NodeSpan() source.Span { return n.Span }

// Decl は宣言（docs/grammar.md「ファイルと宣言」の Decl）。
// *NamespaceDecl、*BindDecl と、Declaration の各構造体が実装する。
type Decl interface {
	Node
	declNode()
}

// Decorator はデコレータの適用（docs/grammar.md「デコレータの適用」）。
type Decorator struct {
	Span source.Span
	// Name は DecoratorRef（docs/grammar.md「識別子」）。最後の要素は予約語のこともある（@db.list、@effects）。
	Name QualifiedName
	// Args は引数。"( )" がなければ nil、"()" なら空でない nil 以外（長さ 0）。
	Args []*Arg
}

// NodeSpan は Node の実装。
func (n *Decorator) NodeSpan() source.Span { return n.Span }

// Arg はデコレータの引数。
type Arg struct {
	Span source.Span
	// Name は名前で渡すときの名前。位置で渡すときは nil。
	Name  *Ident
	Value *ArgValue
}

// NodeSpan は Node の実装。
func (n *Arg) NodeSpan() source.Span { return n.Span }

// ArgValue は引数の値。IsList なら List（"[ ]"）、そうでなければ Terms（"|" でつないだ一つ以上）。
type ArgValue struct {
	Span   source.Span
	IsList bool
	Terms  []*ArgTerm
	List   []*ArgValue
}

// NodeSpan は Node の実装。
func (n *ArgValue) NodeSpan() source.Span { return n.Span }

// ArgTerm はリテラルか名前。どちらか一方が nil でない。
type ArgTerm struct {
	Span    source.Span
	Literal *Literal
	Name    *QualifiedName
}

// NodeSpan は Node の実装。
func (n *ArgTerm) NodeSpan() source.Span { return n.Span }

// NamespaceDecl は namespace（docs/grammar.md「ファイルと宣言」）。
type NamespaceDecl struct {
	Span  source.Span
	Doc   *DocComment
	Name  QualifiedName
	Decls []Decl
}

// NodeSpan は Node の実装。
func (n *NamespaceDecl) NodeSpan() source.Span { return n.Span }
func (n *NamespaceDecl) declNode()             {}

// BindDecl は bind（docs/grammar.md「bind」）。
type BindDecl struct {
	Span   source.Span
	Name   Ident
	Target QualifiedName
	// Deep は "**"（子孫の名前空間も含む）。false なら "*"。
	Deep bool
}

// NodeSpan は Node の実装。
func (n *BindDecl) NodeSpan() source.Span { return n.Span }
func (n *BindDecl) declNode()             {}

// ModelDecl は model（docs/grammar.md「model」）。
type ModelDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	Name       Ident
	// Extends は "extends X" の元の model。なければ nil。
	Extends *QualifiedName
	// Body は "{ }"。
	Body *ModelBody
}

// NodeSpan は Node の実装。
func (n *ModelDecl) NodeSpan() source.Span { return n.Span }
func (n *ModelDecl) declNode()             {}

// ModelBody は "{ Member... }"。model の本体と、その場で書く model（ArmType）。
// メンバーは Field と Override の二つの列に分けて持つ（それぞれ書かれた順）。
type ModelBody struct {
	Span      source.Span
	Fields    []*Field
	Overrides []*Override
}

// NodeSpan は Node の実装。
func (n *ModelBody) NodeSpan() source.Span { return n.Span }

// Field は model のフィールド。
type Field struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	// Name は FieldName（予約語も書ける）。
	Name Ident
	Type Type
}

// NodeSpan は Node の実装。
func (n *Field) NodeSpan() source.Span { return n.Span }

// Override は元の model のフィールドに触れるメンバー（docs/grammar.md「model」の Override）。
// 形は三つ: 型を書かない（Type が nil で Never が false）、型を変える（Type）、なくす（Never）。
type Override struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	// Name は FieldName（予約語も書ける）。
	Name Ident
	// Type は ":" の後ろの型。書いていないか never なら nil。
	Type Type
	// Never は ": never"（そのフィールドをなくす）。
	Never bool
	// NeverSpan は never の範囲。Never でなければゼロ値。
	NeverSpan source.Span
}

// NodeSpan は Node の実装。
func (n *Override) NodeSpan() source.Span { return n.Span }

// ScalarDecl は scalar（docs/grammar.md「scalar」）。
type ScalarDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	Name       Ident
	Base       Type
}

// NodeSpan は Node の実装。
func (n *ScalarDecl) NodeSpan() source.Span { return n.Span }
func (n *ScalarDecl) declNode()             {}

// UnionDecl は union（docs/grammar.md「union」）。
type UnionDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	Name       Ident
	Members    []*UnionMember
}

// NodeSpan は Node の実装。
func (n *UnionDecl) NodeSpan() source.Span { return n.Span }
func (n *UnionDecl) declNode()             {}

// UnionMember はユニオンのメンバー。Arm と Type の組（"Ready: { ... };"）か、
// Target（"server.Unauthorized;"）のどちらか一方。
type UnionMember struct {
	Span   source.Span
	Doc    *DocComment
	Arm    *Ident
	Type   *ArmType
	Target *QualifiedName
}

// NodeSpan は Node の実装。
func (n *UnionMember) NodeSpan() source.Span { return n.Span }

// ArmType はユニオンのアームと例外の値の型。Type か Body のどちらか一方が nil でない。
type ArmType struct {
	Span source.Span
	Type Type
	Body *ModelBody
}

// NodeSpan は Node の実装。
func (n *ArmType) NodeSpan() source.Span { return n.Span }

// StateDecl は state（docs/grammar.md「state」）。形は型から決まる（docs/check.md「状態の形」）。
type StateDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	Name       Ident
	Type       Type
}

// NodeSpan は Node の実装。
func (n *StateDecl) NodeSpan() source.Span { return n.Span }
func (n *StateDecl) declNode()             {}

// ExceptionDecl は exception（docs/grammar.md「exception / effect」）。
type ExceptionDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	Name       Ident
	// Value は運ぶ値の型。なければ nil。
	Value *ArmType
}

// NodeSpan は Node の実装。
func (n *ExceptionDecl) NodeSpan() source.Span { return n.Span }
func (n *ExceptionDecl) declNode()             {}

// StateOp は状態への操作（docs/grammar.md「exception / effect」の StateOp）。
type StateOp int

const (
	// Post はレコードが増える。
	Post StateOp = iota
	// Put は値が変わる。
	Put
	// Delete はレコードが減る。
	Delete
)

// String は "post" / "put" / "delete"。
func (s StateOp) String() string {
	switch s {
	case Post:
		return "post"
	case Put:
		return "put"
	case Delete:
		return "delete"
	}
	return "?"
}

// StateOpRef は "状態.操作"（sessions.post）。字句では一つの QualifiedName で、
// 構文解析で最後の要素を StateOp として切り出したもの（docs/grammar.md「exception / effect」）。
type StateOpRef struct {
	Span  source.Span
	State QualifiedName
	Op    StateOp
	// OpSpan は操作の識別子の範囲。
	OpSpan source.Span
}

// NodeSpan は Node の実装。
func (n *StateOpRef) NodeSpan() source.Span { return n.Span }

// EffectDecl は effect（docs/grammar.md「exception / effect」）。宛先は一つだけで、On か Transition のどちらか一方が nil でない。
type EffectDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	Name       Ident
	// On は基底の状態の組み込みの効果（"状態.操作"）。遷移なら nil。
	On *StateOpRef
	// Transition は分類の遷移（"X: A -> B"）。組み込みの効果なら nil。
	Transition *Transition
}

// NodeSpan は Node の実装。
func (n *EffectDecl) NodeSpan() source.Span { return n.Span }
func (n *EffectDecl) declNode()             {}

// ReadDecl は read（docs/grammar.md「read」）: "read 名前 on 状態"。状態を読むことに名前を付ける。
type ReadDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	Name       Ident
	// On は読む基底の状態の名前。
	On QualifiedName
}

// NodeSpan は Node の実装。
func (n *ReadDecl) NodeSpan() source.Span { return n.Span }
func (n *ReadDecl) declNode()             {}

// Transition は分類の遷移 "QualifiedName : CaseSet -> CaseSet"（docs/grammar.md「exception / effect」）。
// Of は状態を対象にした classify か、組み込みの分類を持つ基底の状態。
type Transition struct {
	Span source.Span
	Of   QualifiedName
	From *CaseSet
	To   *CaseSet
}

// NodeSpan は Node の実装。
func (n *Transition) NodeSpan() source.Span { return n.Span }

// CaseSet は遷移の前か後のケースの集合。Any なら "any" で、Cases は空。
type CaseSet struct {
	Span  source.Span
	Any   bool
	Cases []Ident
}

// NodeSpan は Node の実装。
func (n *CaseSet) NodeSpan() source.Span { return n.Span }

// ClassifyDecl は classify（docs/grammar.md「classify / condition」）。
type ClassifyDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	Name       Ident
	// Of は対象（ClassifySubject）。一つ以上。型か op か状態の名前かは名前の解決が決める。
	Of    []Type
	Cases []*ClassifyCase
}

// NodeSpan は Node の実装。
func (n *ClassifyDecl) NodeSpan() source.Span { return n.Span }
func (n *ClassifyDecl) declNode()             {}

// ClassifyCase は分類のケース。Default なら Name は nil。
type ClassifyCase struct {
	Span    source.Span
	Doc     *DocComment
	Name    *Ident
	Default bool
	// As は "対象 as 型" の並び（CaseTyping）。なければ空。default には書けない。
	As []*CaseTyping
}

// NodeSpan は Node の実装。
func (n *ClassifyCase) NodeSpan() source.Span { return n.Span }

// CaseTyping はケースの "対象 as 型"（docs/grammar.md「classify / condition」）。
// Subject は宣言の of の後ろと同じに書いた対象（型か op か状態の名前。名前の解決が決める）。
type CaseTyping struct {
	Span    source.Span
	Subject Type
	Type    Type
}

// NodeSpan は Node の実装。
func (n *CaseTyping) NodeSpan() source.Span { return n.Span }

// ConditionDecl は condition。
type ConditionDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	Name       Ident
	// Of は対象（ClassifySubject）。一つ以上。
	Of []Type
}

// NodeSpan は Node の実装。
func (n *ConditionDecl) NodeSpan() source.Span { return n.Span }
func (n *ConditionDecl) declNode()             {}

// DecoratorDecl はデコレータの宣言（docs/grammar.md「decorator の宣言」）。
// Alias が nil でなければ二つ目の形（名前付きのデコレータ）で、Params / For / Clauses は空。
type DecoratorDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	// External は @external の印。
	External bool
	// ExternalSpan は @external の範囲。External でなければゼロ値。
	ExternalSpan source.Span
	// Name は DecoratorName（docs/grammar.md「識別子」）。予約語のこともある（decorator list(…)）。
	Name   Ident
	Params []*Param
	// For は付けられる対象。省略なら空（意味は op。設計書 §8.2）。
	For []*Target
	// Clauses は節。*ThrowsClause、*CatchesClause と、デコレータが op に運ぶ *ReadsClause、*EffectsClause
	// （docs/grammar.md「decorator の宣言」の DecoReadsClause / DecoEffectsClause）。書かれた順。
	Clauses []Clause
	Alias   *DecoratorAlias
}

// NodeSpan は Node の実装。
func (n *DecoratorDecl) NodeSpan() source.Span { return n.Span }
func (n *DecoratorDecl) declNode()             {}

// DecoratorAlias は "= DecoratorRef Args" の部分。
type DecoratorAlias struct {
	Span source.Span
	// Name は DecoratorRef。最後の要素は予約語のこともある。
	Name QualifiedName
	Args []*Arg
}

// NodeSpan は Node の実装。
func (n *DecoratorAlias) NodeSpan() source.Span { return n.Span }

// Param はデコレータの引数の宣言。Rest なら残りの引数（"..." Ident ":" RestType。docs/grammar.md「decorator の宣言」）。
type Param struct {
	Span source.Span
	Name Ident
	Type *ParamType
	// Rest は残りの引数か。引数の並びの最後の一つだけが true になりうる。
	Rest bool
	// RestSpan は "..." の範囲。Rest でなければゼロ値。
	RestSpan source.Span
}

// NodeSpan は Node の実装。
func (n *Param) NodeSpan() source.Span { return n.Span }

// ParamType はデコレータの引数の型。Any が nil でなければ "any ... [?]"、
// そうでなければ Types（"|" でつないだ一つ以上の Type）。
type ParamType struct {
	Span source.Span
	Any  *AnyType
	// Optional は Any の後ろの "?"。
	Optional bool
	Types    []Type
}

// NodeSpan は Node の実装。
func (n *ParamType) NodeSpan() source.Span { return n.Span }

// AnyKind は any の種類。
type AnyKind int

const (
	// AnyException は any exception。
	AnyException AnyKind = iota
	// AnyEffect は any effect。
	AnyEffect
	// AnyRead は any read。
	AnyRead
	// AnyState は any state。
	AnyState
	// AnyOp は any op。
	AnyOp
	// AnyModel は any model。
	AnyModel
	// AnyVariant は any variant。
	AnyVariant
)

// String は種類の語。
func (k AnyKind) String() string {
	switch k {
	case AnyException:
		return "exception"
	case AnyEffect:
		return "effect"
	case AnyRead:
		return "read"
	case AnyState:
		return "state"
	case AnyOp:
		return "op"
	case AnyModel:
		return "model"
	case AnyVariant:
		return "variant"
	}
	return "?"
}

// AnyType は "any 種類 [on 状態.操作]"。
type AnyType struct {
	Span source.Span
	Kind AnyKind
	// On は "on 状態.操作"。なければ nil。
	On *StateOpRef
}

// NodeSpan は Node の実装。
func (n *AnyType) NodeSpan() source.Span { return n.Span }

// TargetKind はデコレータを付けられる対象の種類。
type TargetKind int

const (
	// TargetOp は op。
	TargetOp TargetKind = iota
	// TargetField は field。
	TargetField
	// TargetState は state。
	TargetState
	// TargetException は exception。
	TargetException
	// TargetScalar は scalar。
	TargetScalar
	// TargetModel は model。
	TargetModel
	// TargetEffect は effect。
	TargetEffect
	// TargetRead は read。
	TargetRead
)

// String は対象の語。
func (k TargetKind) String() string {
	switch k {
	case TargetOp:
		return "op"
	case TargetField:
		return "field"
	case TargetState:
		return "state"
	case TargetException:
		return "exception"
	case TargetScalar:
		return "scalar"
	case TargetModel:
		return "model"
	case TargetEffect:
		return "effect"
	case TargetRead:
		return "read"
	}
	return "?"
}

// Target は for の対象。
type Target struct {
	Span source.Span
	Kind TargetKind
}

// NodeSpan は Node の実装。
func (n *Target) NodeSpan() source.Span { return n.Span }

// Clause はデコレータの宣言と op の節。*ThrowsClause、*CatchesClause、*EffectsClause、*ReadsClause が実装する。
// どれをどこに書けるかは構文解析器が決める（docs/grammar.md「decorator の宣言」「op」）。
type Clause interface {
	Node
	clauseNode()
}

// ThrowsClause は "throws X, Y { Relation }"。
type ThrowsClause struct {
	Span       source.Span
	Exceptions []QualifiedName
	Relations  []*Relation
}

// NodeSpan は Node の実装。
func (n *ThrowsClause) NodeSpan() source.Span { return n.Span }
func (n *ThrowsClause) clauseNode()           {}

// RelationKind は例外と効果の関係（設計書 §7.3）。
type RelationKind int

const (
	// Prevents は「X のとき、効果は起きていない」。
	Prevents RelationKind = iota
	// Keeps は「X のとき、効果は起きている」。
	Keeps
)

// String は "prevents" / "keeps"。
func (k RelationKind) String() string {
	if k == Keeps {
		return "keeps"
	}
	return "prevents"
}

// Relation は "prevents effects" / "keeps effects"。目的語は常にその op のすべての効果で、
// 効果を名前で書き分ける形はない（docs/grammar.md「決めたこと」）。
type Relation struct {
	Span source.Span
	Kind RelationKind
}

// NodeSpan は Node の実装。
func (n *Relation) NodeSpan() source.Span { return n.Span }

// CatchOutcome は吸収したあとの振る舞い。
type CatchOutcome int

const (
	// Recovers は代わりの値を返す。
	Recovers CatchOutcome = iota
	// Halts は呼び出し元に戻らず、効果を起こす。
	Halts
	// Ignores は無視する。
	Ignores
)

// String は "recovers" / "halts" / "ignores"。
func (o CatchOutcome) String() string {
	switch o {
	case Recovers:
		return "recovers"
	case Halts:
		return "halts"
	case Ignores:
		return "ignores"
	}
	return "?"
}

// CatchesClause は "catches X, Y (recovers | halts E | ignores)"。
type CatchesClause struct {
	Span       source.Span
	Exceptions []QualifiedName
	Outcome    CatchOutcome
	// Halts は Outcome が Halts のときの効果。それ以外は nil。
	Halts *QualifiedName
}

// NodeSpan は Node の実装。
func (n *CatchesClause) NodeSpan() source.Span { return n.Span }
func (n *CatchesClause) clauseNode()           {}

// EffectsClause は "effects a, s.post"（docs/grammar.md「op」）。op の節と、デコレータの宣言の節（DecoEffectsClause）。
type EffectsClause struct {
	Span source.Span
	// Effects は要素（EffectElem）: 効果の名前か、直に書いた「状態.操作」。どちらかは名前の解決が決める。
	Effects []QualifiedName
}

// NodeSpan は Node の実装。
func (n *EffectsClause) NodeSpan() source.Span { return n.Span }
func (n *EffectsClause) clauseNode()           {}

// ReadsClause は "reads a, b"（docs/grammar.md「op」）。op の節と、デコレータの宣言の節（DecoReadsClause）。
type ReadsClause struct {
	Span source.Span
	// Reads は要素（ReadElem）: read の名前か、直に書いた状態の名前。どちらかは名前の解決が決める。
	Reads []QualifiedName
}

// NodeSpan は Node の実装。
func (n *ReadsClause) NodeSpan() source.Span { return n.Span }
func (n *ReadsClause) clauseNode()           {}

// OpDecl は op（docs/grammar.md「op」）。Body が nil なら葉、あれば配線。
type OpDecl struct {
	Span       source.Span
	Doc        *DocComment
	Decorators []*Decorator
	// Export は "export" の印（別の名前空間から呼べる op）。
	Export bool
	// ExportSpan は export の範囲。Export でなければゼロ値。
	ExportSpan source.Span
	Name       Ident
	Params     []*OpParam
	// Return は戻り値の型。省略なら nil（void と同じ意味。設計書 §7.1）。
	Return  *ReturnType
	Clauses []Clause
	Body    *Block
}

// NodeSpan は Node の実装。
func (n *OpDecl) NodeSpan() source.Span { return n.Span }
func (n *OpDecl) declNode()             {}

// OpParam は op の引数。
type OpParam struct {
	Span source.Span
	Name Ident
	Type Type
}

// NodeSpan は Node の実装。
func (n *OpParam) NodeSpan() source.Span { return n.Span }

// ReturnType は戻り値の型。Void か Never なら Type は nil。
type ReturnType struct {
	Span source.Span
	Void bool
	// Never は "never"（決して値を返さない op）。
	Never bool
	Type  Type
}

// NodeSpan は Node の実装。
func (n *ReturnType) NodeSpan() source.Span { return n.Span }

// Type は型（docs/grammar.md「型」）。*CoreType、*RecordType、*ShapeType、*NamedType、*ArrayType、*OptionalType が実装する。
type Type interface {
	Node
	typeNode()
}

// CoreType は核の型（string、boolean、int、float、Time、Duration）。
type CoreType struct {
	Span source.Span
	Name string
}

// NodeSpan は Node の実装。
func (n *CoreType) NodeSpan() source.Span { return n.Span }
func (n *CoreType) typeNode()             {}

// RecordType は Record<K, V>。
type RecordType struct {
	Span  source.Span
	Key   Type
	Value Type
}

// NodeSpan は Node の実装。
func (n *RecordType) NodeSpan() source.Span { return n.Span }
func (n *RecordType) typeNode()             {}

// ShapeType は形の語で書いた型（list<T> / nullable<T> / single<T>。docs/grammar.md「型」）。
// T[] / T? / T の別の書き方で、IR では同じ型になる。
type ShapeType struct {
	Span source.Span
	// Shape は "list" / "nullable" / "single"。
	Shape string
	// ShapeSpan は形の語の範囲。
	ShapeSpan source.Span
	Elem      Type
}

// NodeSpan は Node の実装。
func (n *ShapeType) NodeSpan() source.Span { return n.Span }
func (n *ShapeType) typeNode()             {}

// NamedType は宣言の名前。Arg は分類のケースで絞り込んだ型の型引数（WarrantyKind.Yamada<server.Item>）。なければ nil。
type NamedType struct {
	Span source.Span
	Name QualifiedName
	Arg  Type
}

// NodeSpan は Node の実装。
func (n *NamedType) NodeSpan() source.Span { return n.Span }
func (n *NamedType) typeNode()             {}

// ArrayType は T[]。
type ArrayType struct {
	Span source.Span
	Elem Type
}

// NodeSpan は Node の実装。
func (n *ArrayType) NodeSpan() source.Span { return n.Span }
func (n *ArrayType) typeNode()             {}

// OptionalType は T?。
type OptionalType struct {
	Span source.Span
	Elem Type
}

// NodeSpan は Node の実装。
func (n *OptionalType) NodeSpan() source.Span { return n.Span }
func (n *OptionalType) typeNode()             {}

// Block は配線の本体（docs/grammar.md「配線」）。
type Block struct {
	Span  source.Span
	Stmts []Stmt
}

// NodeSpan は Node の実装。
func (n *Block) NodeSpan() source.Span { return n.Span }

// Stmt は文。*ConstStmt、*ReturnStmt、*CallStmt、*MatchStmt が実装する。
type Stmt interface {
	Node
	stmtNode()
}

// ConstStmt は "const x = Expr;"。
type ConstStmt struct {
	Span  source.Span
	Name  Ident
	Value Expr
}

// NodeSpan は Node の実装。
func (n *ConstStmt) NodeSpan() source.Span { return n.Span }
func (n *ConstStmt) stmtNode()             {}

// ReturnStmt は "return Expr;"。
type ReturnStmt struct {
	Span  source.Span
	Value Expr
}

// NodeSpan は Node の実装。
func (n *ReturnStmt) NodeSpan() source.Span { return n.Span }
func (n *ReturnStmt) stmtNode()             {}

// CallStmt は "Call;"。
type CallStmt struct {
	Span source.Span
	Call *Call
}

// NodeSpan は Node の実装。
func (n *CallStmt) NodeSpan() source.Span { return n.Span }
func (n *CallStmt) stmtNode()             {}

// MatchStmt は "Match;"。
type MatchStmt struct {
	Span  source.Span
	Match *Match
}

// NodeSpan は Node の実装。
func (n *MatchStmt) NodeSpan() source.Span { return n.Span }
func (n *MatchStmt) stmtNode()             {}

// Expr は式。*Call、*Match と Atom が実装する。
type Expr interface {
	Node
	exprNode()
}

// Atom は呼び出しの引数とアームの本体に書ける値。*NameAtom、*Literal、*NoneAtom、*RecordLit が実装する。
type Atom interface {
	Expr
	ArmBody
	atomNode()
}

// Call は呼び出し。引数は Atom だけ（設計書 §9「A 正規形」）。
type Call struct {
	Span   source.Span
	Callee QualifiedName
	Args   []Atom
}

// NodeSpan は Node の実装。
func (n *Call) NodeSpan() source.Span { return n.Span }
func (n *Call) exprNode()             {}
func (n *Call) armBodyNode()          {}

// NameAtom は束縛した名前・op の引数・今の名前空間の状態と、そのフィールド（query.page）。
type NameAtom struct {
	Span source.Span
	// Name は Path（docs/grammar.md「識別子」）。二つ目以降の要素は FieldName で、予約語のこともある（res.op）。
	Name QualifiedName
}

// NodeSpan は Node の実装。
func (n *NameAtom) NodeSpan() source.Span { return n.Span }
func (n *NameAtom) exprNode()             {}
func (n *NameAtom) atomNode()             {}
func (n *NameAtom) armBodyNode()          {}

// NoneAtom は none。
type NoneAtom struct {
	Span source.Span
}

// NodeSpan は Node の実装。
func (n *NoneAtom) NodeSpan() source.Span { return n.Span }
func (n *NoneAtom) exprNode()             {}
func (n *NoneAtom) atomNode()             {}
func (n *NoneAtom) armBodyNode()          {}

// RecordLit は "{ a: x, b }"。
type RecordLit struct {
	Span   source.Span
	Fields []*RecordField
}

// NodeSpan は Node の実装。
func (n *RecordLit) NodeSpan() source.Span { return n.Span }
func (n *RecordLit) exprNode()             {}
func (n *RecordLit) atomNode()             {}
func (n *RecordLit) armBodyNode()          {}

// RecordField はレコードのフィールド。Value が nil なら "{ item }" の略記（"{ item: item }" の意味）。
type RecordField struct {
	Span source.Span
	// Name は Value があれば FieldName（予約語も書ける）、略記なら Ident。
	Name  Ident
	Value Atom
}

// NodeSpan は Node の実装。
func (n *RecordField) NodeSpan() source.Span { return n.Span }

// Match は "match [Atom] by (QualifiedName | list | nullable) { MatchArm... }"（docs/grammar.md「配線」）。
type Match struct {
	Span source.Span
	// Subject は対象。書かれていなければ nil（状態の分類での match）。
	Subject Atom
	// Classifier は by の後ろ。Builtin なら予約語 list / nullable で、要素は一つ（Name はその語）。
	Classifier QualifiedName
	// Builtin は by の後ろが形の語（標準の分類）か。
	Builtin bool
	Arms    []*MatchArm
}

// NodeSpan は Node の実装。
func (n *Match) NodeSpan() source.Span { return n.Span }
func (n *Match) exprNode()             {}

// ArmBody はアームの本体（docs/grammar.md「配線」の ArmBody）。*Call、Atom の各構造体、*Throw が実装する。
type ArmBody interface {
	Node
	armBodyNode()
}

// Throw は "throw E"。アームの本体にだけ書ける。
type Throw struct {
	Span      source.Span
	Exception QualifiedName
}

// NodeSpan は Node の実装。
func (n *Throw) NodeSpan() source.Span { return n.Span }
func (n *Throw) armBodyNode()          {}

// MatchArm は "A | B => ArmBody;"。Body は *Call か Atom か *Throw。
type MatchArm struct {
	Span    source.Span
	Cases   []Ident
	Default bool
	Body    ArmBody
}

// NodeSpan は Node の実装。
func (n *MatchArm) NodeSpan() source.Span { return n.Span }
