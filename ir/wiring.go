package ir

import (
	"encoding/json"
	"fmt"
	"time"
)

// Wiring は op の本体（docs/ir.md「配線」の Wiring）。
type Wiring struct {
	Statements []Statement `json:"statements"`
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (w Wiring) MarshalJSON() ([]byte, error) { type alias Wiring; return marshalStruct(alias(w)) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (w *Wiring) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, w) }

// Statement は配線の文（docs/ir.md「配線」の Statement）。
// 具体型は値型の [ConstStmt]、[ReturnStmt]、[CallStmt]、[MatchStmt]。
type Statement interface {
	// Kind は JSON の判別子（"const"、"return"、"call"、"match"）。
	Kind() string
	isStatement()
}

// ConstStmt は `const name = expr;`（docs/ir.md「配線」の Statement kind "const"）。
type ConstStmt struct {
	Name string `json:"name"`
	Expr Expr   `json:"expr"`
	Span Span   `json:"span"`
}

// ReturnStmt は `return expr;`（docs/ir.md「配線」の Statement kind "return"）。
type ReturnStmt struct {
	Expr Expr `json:"expr"`
	Span Span `json:"span"`
}

// CallStmt は文としての呼び出し（docs/ir.md「配線」の Statement kind "call"）。
type CallStmt struct {
	Call Call `json:"call"`
	Span Span `json:"span"`
}

// MatchStmt は文としての match（docs/ir.md「配線」の Statement kind "match"）。
type MatchStmt struct {
	Match Match `json:"match"`
	Span  Span  `json:"span"`
}

var (
	_ Statement = ConstStmt{}
	_ Statement = ReturnStmt{}
	_ Statement = CallStmt{}
	_ Statement = MatchStmt{}
)

// Kind は "const"。
func (ConstStmt) Kind() string { return "const" }

// Kind は "return"。
func (ReturnStmt) Kind() string { return "return" }

// Kind は "call"。
func (CallStmt) Kind() string { return "call" }

// Kind は "match"。
func (MatchStmt) Kind() string { return "match" }

func (ConstStmt) isStatement()  {}
func (ReturnStmt) isStatement() {}
func (CallStmt) isStatement()   {}
func (MatchStmt) isStatement()  {}

// MarshalJSON は kind を付けて書く。
func (s ConstStmt) MarshalJSON() ([]byte, error) {
	type alias ConstStmt
	return marshalKind("const", alias(s))
}

// MarshalJSON は kind を付けて書く。
func (s ReturnStmt) MarshalJSON() ([]byte, error) {
	type alias ReturnStmt
	return marshalKind("return", alias(s))
}

// MarshalJSON は kind を付けて書く。
func (s CallStmt) MarshalJSON() ([]byte, error) {
	type alias CallStmt
	return marshalKind("call", alias(s))
}

// MarshalJSON は kind を付けて書く。
func (s MatchStmt) MarshalJSON() ([]byte, error) {
	type alias MatchStmt
	return marshalKind("match", alias(s))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *ConstStmt) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *ReturnStmt) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }

var statementTable = map[string]func([]byte) (Statement, error){
	"const":  asValue[Statement, ConstStmt],
	"return": asValue[Statement, ReturnStmt],
	"call":   asValue[Statement, CallStmt],
	"match":  asValue[Statement, MatchStmt],
}

// UnmarshalStatement は kind で分けて Statement を読む（docs/ir.md「JSON」）。
func UnmarshalStatement(data []byte) (Statement, error) {
	return decodeUnion("statement", data, statementTable)
}

// Expr は式（docs/ir.md「配線」の Expr）。具体型は値型の [CallExpr]、[MatchExpr]、[AtomExpr]。
type Expr interface {
	// Kind は JSON の判別子（"call"、"match"、"atom"）。
	Kind() string
	isExpr()
}

// CallExpr は式としての呼び出し（docs/ir.md「配線」の Expr kind "call"）。
// match のアームの本体（[ArmBody]）にも使う。
type CallExpr struct {
	Call Call `json:"call"`
}

// MatchExpr は式としての match（docs/ir.md「配線」の Expr kind "match"）。
type MatchExpr struct {
	Match Match `json:"match"`
}

// AtomExpr は式としてのアトム（docs/ir.md「配線」の Expr kind "atom"）。
type AtomExpr struct {
	Atom Atom `json:"atom"`
}

var (
	_ Expr = CallExpr{}
	_ Expr = MatchExpr{}
	_ Expr = AtomExpr{}
)

// Kind は "call"。
func (CallExpr) Kind() string { return "call" }

// Kind は "match"。
func (MatchExpr) Kind() string { return "match" }

// Kind は "atom"。
func (AtomExpr) Kind() string { return "atom" }

func (CallExpr) isExpr()  {}
func (MatchExpr) isExpr() {}
func (AtomExpr) isExpr()  {}

// MarshalJSON は kind を付けて書く。
func (e CallExpr) MarshalJSON() ([]byte, error) {
	type alias CallExpr
	return marshalKind("call", alias(e))
}

// MarshalJSON は kind を付けて書く。
func (e MatchExpr) MarshalJSON() ([]byte, error) {
	type alias MatchExpr
	return marshalKind("match", alias(e))
}

// MarshalJSON は kind を付けて書く。
func (e AtomExpr) MarshalJSON() ([]byte, error) {
	type alias AtomExpr
	return marshalKind("atom", alias(e))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (e *AtomExpr) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, e) }

var exprTable = map[string]func([]byte) (Expr, error){
	"call":  asValue[Expr, CallExpr],
	"match": asValue[Expr, MatchExpr],
	"atom":  asValue[Expr, AtomExpr],
}

// UnmarshalExpr は kind で分けて Expr を読む（docs/ir.md「JSON」）。
func UnmarshalExpr(data []byte) (Expr, error) {
	return decodeUnion("expr", data, exprTable)
}

// Call は op の呼び出し（docs/ir.md「配線」の Call）。
type Call struct {
	// Op は呼ぶ op の FQN。
	Op string `json:"op"`
	// Args は引数。位置で書いた引数を引数名に対応付けたもの。
	Args []CallArg `json:"args"`
	// Bound は bind で埋まる位置（docs/check.md「bind」）。計算結果。
	Bound []BoundPosition `json:"bound"`
	Span  Span            `json:"span"`
}

// CallArg は呼び出しの引数（docs/ir.md「配線」の Call の args）。
type CallArg struct {
	Param string `json:"param"`
	Value Atom   `json:"value"`
}

// BoundPosition は bind で埋まる位置（docs/ir.md「配線」の BoundPosition）。
type BoundPosition struct {
	// Path は引数名から始まるフィールドのパス（["in", "token"]）。
	Path []string `json:"path"`
	// State は埋める状態の FQN。
	State string `json:"state"`
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (c Call) MarshalJSON() ([]byte, error) { type alias Call; return marshalStruct(alias(c)) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (c *Call) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, c) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (a *CallArg) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, a) }

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (b BoundPosition) MarshalJSON() ([]byte, error) {
	type alias BoundPosition
	return marshalStruct(alias(b))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (b *BoundPosition) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, b) }

// Atom は式の葉（docs/ir.md「配線」の Atom）。具体型は値型の [NameAtom]、[LiteralAtom]、[NoneAtom]、[RecordAtom]。
type Atom interface {
	// Kind は JSON の判別子（"name"、"literal"、"none"、"record"）。
	Kind() string
	isAtom()
}

// NameAtom は名前の参照（docs/ir.md「配線」の Atom kind "name"）。
type NameAtom struct {
	// Path は ["query", "page"] のようなパス。先頭は束縛名か引数名。
	Path []string `json:"path"`
	Span Span     `json:"span"`
}

// LiteralAtom はリテラル（docs/ir.md「配線」の Atom kind "literal"）。
type LiteralAtom struct {
	Literal Literal `json:"literal"`
	Span    Span    `json:"span"`
}

// NoneAtom は `none`（docs/ir.md「配線」の Atom kind "none"）。
type NoneAtom struct {
	Span Span `json:"span"`
}

// RecordAtom はレコードリテラル（docs/ir.md「配線」の Atom kind "record"）。`{ item }` は `{ item: item }` に展開済み。
type RecordAtom struct {
	Fields []RecordField `json:"fields"`
	Span   Span          `json:"span"`
}

// RecordField はレコードリテラルのフィールド（docs/ir.md「配線」の Atom kind "record" の fields）。
type RecordField struct {
	Name  string `json:"name"`
	Value Atom   `json:"value"`
}

var (
	_ Atom = NameAtom{}
	_ Atom = LiteralAtom{}
	_ Atom = NoneAtom{}
	_ Atom = RecordAtom{}
)

// Kind は "name"。
func (NameAtom) Kind() string { return "name" }

// Kind は "literal"。
func (LiteralAtom) Kind() string { return "literal" }

// Kind は "none"。
func (NoneAtom) Kind() string { return "none" }

// Kind は "record"。
func (RecordAtom) Kind() string { return "record" }

func (NameAtom) isAtom()    {}
func (LiteralAtom) isAtom() {}
func (NoneAtom) isAtom()    {}
func (RecordAtom) isAtom()  {}

// MarshalJSON は kind を付けて書く。
func (a NameAtom) MarshalJSON() ([]byte, error) {
	type alias NameAtom
	return marshalKind("name", alias(a))
}

// MarshalJSON は kind を付けて書く。
func (a LiteralAtom) MarshalJSON() ([]byte, error) {
	type alias LiteralAtom
	return marshalKind("literal", alias(a))
}

// MarshalJSON は kind を付けて書く。
func (a NoneAtom) MarshalJSON() ([]byte, error) {
	type alias NoneAtom
	return marshalKind("none", alias(a))
}

// MarshalJSON は kind を付けて書く。
func (a RecordAtom) MarshalJSON() ([]byte, error) {
	type alias RecordAtom
	return marshalKind("record", alias(a))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (a *NameAtom) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, a) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (a *LiteralAtom) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, a) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (a *RecordAtom) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, a) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (f *RecordField) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, f) }

var atomTable = map[string]func([]byte) (Atom, error){
	"name":    asValue[Atom, NameAtom],
	"literal": asValue[Atom, LiteralAtom],
	"none":    asValue[Atom, NoneAtom],
	"record":  asValue[Atom, RecordAtom],
}

// UnmarshalAtom は kind で分けて Atom を読む（docs/ir.md「JSON」）。
func UnmarshalAtom(data []byte) (Atom, error) {
	return decodeUnion("atom", data, atomTable)
}

// Match は分類による分岐（docs/ir.md「配線」の Match）。
type Match struct {
	// Subject は対象。値（[ValueSubject]）、基底の状態（[StateSubject]）、分類の宣言が持つ（[ClassifyOwnedSubject]）のどれか。
	Subject MatchSubject `json:"subject"`
	// Classify は classify の FQN。組み込みの分類なら状態の形 "list" か "nullable"。
	Classify string     `json:"classify"`
	Arms     []MatchArm `json:"arms"`
	Span     Span       `json:"span"`
}

// MatchSubject は match の対象（docs/ir.md「配線」の Match の subject）。具体型は値型の [ValueSubject]
// （kind "value"）、[StateSubject]（kind "state"）、[ClassifyOwnedSubject]（kind "classify"）。
type MatchSubject interface {
	// Kind は JSON の判別子（"value"、"state"、"classify"）。
	Kind() string
	isMatchSubject()
}

// ValueSubject は値の分類での match の対象（`match item by C`。docs/ir.md「配線」の subject kind "value"）。
type ValueSubject struct {
	Atom Atom `json:"atom"`
}

// StateSubject は基底の状態の、組み込みの分類での match の対象（`match carts by list`。
// docs/ir.md「配線」の subject kind "state"）。
type StateSubject struct {
	// State は状態の FQN。
	State string `json:"state"`
}

// ClassifyOwnedSubject は状態の分類での match（`match by C`）。対象は classify が持つので、ここには何もない
// （docs/ir.md「配線」の subject kind "classify"）。
type ClassifyOwnedSubject struct{}

var (
	_ MatchSubject = ValueSubject{}
	_ MatchSubject = StateSubject{}
	_ MatchSubject = ClassifyOwnedSubject{}
)

// Kind は "value"。
func (ValueSubject) Kind() string { return "value" }

// Kind は "state"。
func (StateSubject) Kind() string { return "state" }

// Kind は "classify"。
func (ClassifyOwnedSubject) Kind() string { return "classify" }

func (ValueSubject) isMatchSubject()         {}
func (StateSubject) isMatchSubject()         {}
func (ClassifyOwnedSubject) isMatchSubject() {}

// MarshalJSON は kind を付けて書く。
func (s ValueSubject) MarshalJSON() ([]byte, error) {
	type alias ValueSubject
	return marshalKind("value", alias(s))
}

// MarshalJSON は kind を付けて書く。
func (s StateSubject) MarshalJSON() ([]byte, error) {
	type alias StateSubject
	return marshalKind("state", alias(s))
}

// MarshalJSON は kind を付けて書く。
func (ClassifyOwnedSubject) MarshalJSON() ([]byte, error) { return []byte(`{"kind":"classify"}`), nil }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *ValueSubject) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }

var matchSubjectTable = map[string]func([]byte) (MatchSubject, error){
	"value":    asValue[MatchSubject, ValueSubject],
	"state":    asValue[MatchSubject, StateSubject],
	"classify": asValue[MatchSubject, ClassifyOwnedSubject],
}

// UnmarshalMatchSubject は kind で分けて MatchSubject を読む（docs/ir.md「JSON」）。
func UnmarshalMatchSubject(data []byte) (MatchSubject, error) {
	return decodeUnion("match subject", data, matchSubjectTable)
}

// SubjectAtom は値の分類での match の対象の Atom。状態での match なら nil。
func (m Match) SubjectAtom() Atom {
	if s, ok := m.Subject.(ValueSubject); ok {
		return s.Atom
	}
	return nil
}

// Builtin は組み込みの分類での match（`by list` / `by nullable`）か。
func (m Match) Builtin() bool {
	_, ok := m.Subject.(StateSubject)
	return ok
}

// MatchArm は match のアーム（docs/ir.md「配線」の Match の arms）。
type MatchArm struct {
	Cases ArmCases `json:"cases"`
	Body  ArmBody  `json:"body"`
	Span  Span     `json:"span"`
}

// ArmCases はアームが受けるケース: ケース名の列か default（docs/ir.md「配線」の Match の cases）。
// Default が true なら JSON は "default"、そうでなければ列。
type ArmCases struct {
	Default bool
	Cases   []string
}

// DefaultArm は default のアームを表す ArmCases。
func DefaultArm() ArmCases { return ArmCases{Default: true} }

// MarshalJSON は "default" かケース名の列を書く。
func (c ArmCases) MarshalJSON() ([]byte, error) {
	if c.Default {
		return []byte(`"default"`), nil
	}
	if c.Cases == nil {
		return []byte(`[]`), nil
	}
	return json.Marshal(c.Cases)
}

// UnmarshalJSON は "default" かケース名の列を読む。
func (c *ArmCases) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s != "default" {
			return fmt.Errorf("ir: arm cases: unknown value %q", s)
		}
		*c = ArmCases{Default: true}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("ir: arm cases: want \"default\" or array: %w", err)
	}
	if len(list) == 0 {
		list = nil
	}
	*c = ArmCases{Cases: list}
	return nil
}

// ArmBody は match のアームの本体（docs/ir.md「配線」の Match の body）。
// 具体型は値型の [CallExpr]（kind "call"）と [NoneBody]（kind "none"）。
type ArmBody interface {
	// Kind は JSON の判別子（"call" か "none"）。
	Kind() string
	isArmBody()
}

// NoneBody はアームの本体の `none`（docs/ir.md「配線」の Match の body kind "none"）。
type NoneBody struct{}

var (
	_ ArmBody = CallExpr{}
	_ ArmBody = NoneBody{}
)

// Kind は "none"。
func (NoneBody) Kind() string { return "none" }

func (CallExpr) isArmBody() {}
func (NoneBody) isArmBody() {}

// MarshalJSON は kind を付けて書く。
func (NoneBody) MarshalJSON() ([]byte, error) { return []byte(`{"kind":"none"}`), nil }

var armBodyTable = map[string]func([]byte) (ArmBody, error){
	"call": asValue[ArmBody, CallExpr],
	"none": asValue[ArmBody, NoneBody],
}

// UnmarshalArmBody は kind で分けて ArmBody を読む（docs/ir.md「JSON」）。
func UnmarshalArmBody(data []byte) (ArmBody, error) {
	return decodeUnion("arm body", data, armBodyTable)
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (m Match) MarshalJSON() ([]byte, error) { type alias Match; return marshalStruct(alias(m)) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (m *Match) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, m) }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (a *MatchArm) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, a) }

// Literal はリテラル（docs/ir.md「配線」の Literal）。
// 具体型は値型の [StringLiteral]、[IntLiteral]、[FloatLiteral]、[BooleanLiteral]、[DurationLiteral]。
type Literal interface {
	// Kind は JSON の判別子（"string"、"int"、"float"、"boolean"、"duration"）。
	Kind() string
	isLiteral()
}

// StringLiteral は文字列（docs/ir.md「配線」の Literal kind "string"）。
type StringLiteral struct {
	Value string `json:"value"`
}

// IntLiteral は整数（docs/ir.md「配線」の Literal kind "int"）。64 ビットの範囲は LIT-1 が保証する。
type IntLiteral struct {
	Value int64 `json:"value"`
}

// FloatLiteral は浮動小数点数（docs/ir.md「配線」の Literal kind "float"）。
type FloatLiteral struct {
	Value float64 `json:"value"`
}

// BooleanLiteral は真偽値（docs/ir.md「配線」の Literal kind "boolean"）。
type BooleanLiteral struct {
	Value bool `json:"value"`
}

// DurationLiteral は期間（docs/ir.md「配線」の Literal kind "duration"）。JSON は整数のナノ秒。
type DurationLiteral struct {
	Value time.Duration `json:"value"`
}

var (
	_ Literal = StringLiteral{}
	_ Literal = IntLiteral{}
	_ Literal = FloatLiteral{}
	_ Literal = BooleanLiteral{}
	_ Literal = DurationLiteral{}
)

// Kind は "string"。
func (StringLiteral) Kind() string { return "string" }

// Kind は "int"。
func (IntLiteral) Kind() string { return "int" }

// Kind は "float"。
func (FloatLiteral) Kind() string { return "float" }

// Kind は "boolean"。
func (BooleanLiteral) Kind() string { return "boolean" }

// Kind は "duration"。
func (DurationLiteral) Kind() string { return "duration" }

func (StringLiteral) isLiteral()   {}
func (IntLiteral) isLiteral()      {}
func (FloatLiteral) isLiteral()    {}
func (BooleanLiteral) isLiteral()  {}
func (DurationLiteral) isLiteral() {}

// MarshalJSON は kind を付けて書く。
func (l StringLiteral) MarshalJSON() ([]byte, error) {
	type alias StringLiteral
	return marshalKind("string", alias(l))
}

// MarshalJSON は kind を付けて書く。
func (l IntLiteral) MarshalJSON() ([]byte, error) {
	type alias IntLiteral
	return marshalKind("int", alias(l))
}

// MarshalJSON は kind を付けて書く。
func (l FloatLiteral) MarshalJSON() ([]byte, error) {
	type alias FloatLiteral
	return marshalKind("float", alias(l))
}

// MarshalJSON は kind を付けて書く。
func (l BooleanLiteral) MarshalJSON() ([]byte, error) {
	type alias BooleanLiteral
	return marshalKind("boolean", alias(l))
}

// MarshalJSON は kind を付けて書く。
func (l DurationLiteral) MarshalJSON() ([]byte, error) {
	type alias DurationLiteral
	return marshalKind("duration", alias(l))
}

var literalTable = map[string]func([]byte) (Literal, error){
	"string":   asValue[Literal, StringLiteral],
	"int":      asValue[Literal, IntLiteral],
	"float":    asValue[Literal, FloatLiteral],
	"boolean":  asValue[Literal, BooleanLiteral],
	"duration": asValue[Literal, DurationLiteral],
}

// UnmarshalLiteral は kind で分けて Literal を読む（docs/ir.md「JSON」）。
func UnmarshalLiteral(data []byte) (Literal, error) {
	return decodeUnion("literal", data, literalTable)
}
