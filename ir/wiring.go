package ir

import "time"

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
// match のアームの本体の値（[ArmBody] の kind "atom"）にも使う。none はそこには入れず [NoneBody] にする。
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

// Call は配線の呼び出し（docs/ir.md「配線」の Call）: op、read、効果のどれか（docs/check.md「配線からの呼び出し」）。
type Call struct {
	// Callee は呼び出し先。解決できなかった呼び出しでは nil。
	Callee Callee `json:"callee"`
	// Args は引数。位置で書いた引数を、呼び出し先の引数名に対応付けたもの（docs/types.md「呼び出しの引数」）。
	// 直に呼ぶ組み込みの効果の、値を取る操作の引数名は [DirectEffectParam]。
	Args []CallArg `json:"args"`
	// Bound は bind で埋まる位置（docs/check.md「bind」）。計算結果。呼び出し先が op でなければ空。
	Bound []BoundPosition `json:"bound"`
	Span  Span            `json:"span"`
}

// DirectEffectParam は、直に呼ぶ組み込みの効果の、値を取る操作の引数名（docs/ir.md「配線」の Call の args）。
const DirectEffectParam = "value"

// CalledOp は呼び出し先が op なら、その FQN。read か効果の呼び出し（と、解決できなかった呼び出し）なら ""。
func (c Call) CalledOp() string {
	if o, ok := c.Callee.(OpCallee); ok {
		return o.Op
	}
	return ""
}

// Callee は呼び出し先（docs/ir.md「配線」の Callee）。具体型は値型の [OpCallee]（kind "op"）、[ReadCallee]（kind "read"）、
// [EffectCallee]（kind "effect"）で、== で比べられる。
type Callee interface {
	// Kind は JSON の判別子（"op"、"read"、"effect"）。
	Kind() string
	// String は表示のための文字列: op と read は宣言の FQN、効果は [EffectRef] の String。
	String() string
	isCallee()
}

// OpCallee は op の呼び出し。
type OpCallee struct {
	// Op は op の FQN。
	Op string `json:"op"`
}

// ReadCallee は read の宣言の呼び出し（`userByEmail(email)`）。名前を付けない read は、呼び出しではなく [StateAtom]。
type ReadCallee struct {
	// Read は read の宣言の FQN。
	Read string `json:"read"`
}

// EffectCallee は効果の呼び出し。Effect が [NamedEffect] なら effect の宣言の呼び出し（`createOrder(order)`）、
// [DirectEffect] なら組み込みの効果を直に呼んだもの（`orders.post(order)`）。
type EffectCallee struct {
	Effect EffectRef `json:"effect"`
}

var (
	_ Callee = OpCallee{}
	_ Callee = ReadCallee{}
	_ Callee = EffectCallee{}
)

// Kind は "op"。
func (OpCallee) Kind() string { return "op" }

// Kind は "read"。
func (ReadCallee) Kind() string { return "read" }

// Kind は "effect"。
func (EffectCallee) Kind() string { return "effect" }

// String は op の FQN。
func (c OpCallee) String() string { return c.Op }

// String は read の宣言の FQN。
func (c ReadCallee) String() string { return c.Read }

// String は効果の表示（[EffectRef] の String）。
func (c EffectCallee) String() string {
	if c.Effect == nil {
		return ""
	}
	return c.Effect.String()
}

func (OpCallee) isCallee()     {}
func (ReadCallee) isCallee()   {}
func (EffectCallee) isCallee() {}

// MarshalJSON は kind を付けて書く。
func (c OpCallee) MarshalJSON() ([]byte, error) {
	type alias OpCallee
	return marshalKind("op", alias(c))
}

// MarshalJSON は kind を付けて書く。
func (c ReadCallee) MarshalJSON() ([]byte, error) {
	type alias ReadCallee
	return marshalKind("read", alias(c))
}

// MarshalJSON は kind を付けて書く。
func (c EffectCallee) MarshalJSON() ([]byte, error) {
	type alias EffectCallee
	return marshalKind("effect", alias(c))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (c *EffectCallee) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, c) }

var calleeTable = map[string]func([]byte) (Callee, error){
	"op":     asValue[Callee, OpCallee],
	"read":   asValue[Callee, ReadCallee],
	"effect": asValue[Callee, EffectCallee],
}

// UnmarshalCallee は kind で分けて Callee を読む（docs/ir.md「JSON」）。
func UnmarshalCallee(data []byte) (Callee, error) {
	return decodeUnion("callee", data, calleeTable)
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

// Atom は式の葉（docs/ir.md「配線」の Atom）。具体型は値型の [NameAtom]、[StateAtom]、[LiteralAtom]、[NoneAtom]、[RecordAtom]。
type Atom interface {
	// Kind は JSON の判別子（"name"、"state"、"literal"、"none"、"record"）。
	Kind() string
	isAtom()
}

// NameAtom は名前の参照（docs/ir.md「配線」の Atom kind "name"）。
type NameAtom struct {
	// Path は ["query", "page"] のようなパス。先頭は束縛名か引数名。
	Path []string `json:"path"`
	Span Span     `json:"span"`
}

// StateAtom は状態の参照（docs/ir.md「配線」の Atom kind "state"。docs/check.md「状態の参照」）。
type StateAtom struct {
	// State は状態の FQN。
	State string `json:"state"`
	// Path はその値のフィールドのパス。状態そのものなら空（JSON では []）。
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
	_ Atom = StateAtom{}
	_ Atom = NameAtom{}
	_ Atom = LiteralAtom{}
	_ Atom = NoneAtom{}
	_ Atom = RecordAtom{}
)

// Kind は "name"。
func (NameAtom) Kind() string { return "name" }

// Kind は "state"。
func (StateAtom) Kind() string { return "state" }

func (StateAtom) isAtom() {}

// MarshalJSON は kind を付けて書く。
func (a StateAtom) MarshalJSON() ([]byte, error) {
	type alias StateAtom
	return marshalKind("state", alias(a))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (a *StateAtom) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, a) }

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
	"state":   asValue[Atom, StateAtom],
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
	// Subject は対象。値（[ValueSubject]）か、分類の宣言が持つ（[ClassifyOwnedSubject]）。
	Subject MatchSubject `json:"subject"`
	// Classify は classify の FQN。標準の分類なら "list" か "nullable"。
	Classify string     `json:"classify"`
	Arms     []MatchArm `json:"arms"`
	Span     Span       `json:"span"`
}

// MatchSubject は match の対象（docs/ir.md「配線」の Match の subject）。具体型は値型の [ValueSubject]
// （kind "value"）と [ClassifyOwnedSubject]（kind "classify"）。
type MatchSubject interface {
	// Kind は JSON の判別子（"value" か "classify"）。
	Kind() string
	isMatchSubject()
}

// ValueSubject は値を対象にした match の対象（値の分類、op の分類、標準の分類。`match item by C`。
// docs/ir.md「配線」の subject kind "value"）。状態を名前で書いた対象（`match token by nullable`）は、Atom が [StateAtom]。
type ValueSubject struct {
	Atom Atom `json:"atom"`
}

// ClassifyOwnedSubject は状態の分類での match（`match by C`）。対象は classify が持つので、ここには何もない
// （docs/ir.md「配線」の subject kind "classify"）。
type ClassifyOwnedSubject struct{}

var (
	_ MatchSubject = ValueSubject{}
	_ MatchSubject = ClassifyOwnedSubject{}
)

// Kind は "value"。
func (ValueSubject) Kind() string { return "value" }

// Kind は "classify"。
func (ClassifyOwnedSubject) Kind() string { return "classify" }

func (ValueSubject) isMatchSubject()         {}
func (ClassifyOwnedSubject) isMatchSubject() {}

// MarshalJSON は kind を付けて書く。
func (s ValueSubject) MarshalJSON() ([]byte, error) {
	type alias ValueSubject
	return marshalKind("value", alias(s))
}

// MarshalJSON は kind を付けて書く。
func (ClassifyOwnedSubject) MarshalJSON() ([]byte, error) { return []byte(`{"kind":"classify"}`), nil }

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *ValueSubject) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }

var matchSubjectTable = map[string]func([]byte) (MatchSubject, error){
	"value":    asValue[MatchSubject, ValueSubject],
	"classify": asValue[MatchSubject, ClassifyOwnedSubject],
}

// UnmarshalMatchSubject は kind で分けて MatchSubject を読む（docs/ir.md「JSON」）。
func UnmarshalMatchSubject(data []byte) (MatchSubject, error) {
	return decodeUnion("match subject", data, matchSubjectTable)
}

// SubjectAtom は値を対象にした match の対象の Atom。状態の分類での match なら nil。
func (m Match) SubjectAtom() Atom {
	if s, ok := m.Subject.(ValueSubject); ok {
		return s.Atom
	}
	return nil
}

// Builtin は標準の分類での match（`by list` / `by nullable`）か。
func (m Match) Builtin() bool { return IsStandardClassify(m.Classify) }

// SubjectState は、対象が状態そのもの（[StateAtom] でパスが空。`match token by nullable`）なら、その状態の FQN。
func (m Match) SubjectState() (state string, ok bool) {
	a, ok := m.SubjectAtom().(StateAtom)
	if !ok || len(a.Path) > 0 {
		return "", false
	}
	return a.State, true
}

// MatchArm は match のアーム（docs/ir.md「配線」の MatchArm）。
type MatchArm struct {
	// Cases はこのアームが受けるケース（docs/check.md「アームが受けるケース」）。名前で並べたアームなら、書かれた順。
	// else のアームなら、ほかのアームが並べなかったケースを、分類の宣言の順に（標準の分類は Empty、HasItems の順）。
	// JSON では、なくても []。
	Cases []string `json:"cases"`
	// Else は `else => …` と書いたアームか。プラグインは、Cases だけを読めば足りる。
	Else bool    `json:"else"`
	Body ArmBody `json:"body"`
	// Narrowed はこのアームの中で、外と型が違う対象（docs/types.md「アームの中の対象の型」）。なければ空（JSON では []）。
	Narrowed []Narrowing `json:"narrowed"`
	Span     Span        `json:"span"`
}

// Narrowing はアームの中で絞り込まれた対象の型（docs/ir.md「配線」の Narrowing）。
type Narrowing struct {
	// State は状態の分類での match（subject が classify）のとき、絞り込まれる状態の FQN。
	// subject が value なら持たない（絞り込まれるのは subject の atom）。
	State string `json:"state,omitempty"`
	// Type はアームの中での型。
	Type Type `json:"type"`
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (n *Narrowing) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, n) }

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (a MatchArm) MarshalJSON() ([]byte, error) { type alias MatchArm; return marshalStruct(alias(a)) }

// ArmBody は match のアームの本体（docs/ir.md「配線」の MatchArm の body）。
// 具体型は値型の [CallExpr]（kind "call"）、[AtomExpr]（kind "atom"）、[NoneBody]（kind "none"）、[ThrowBody]（kind "throw"）。
type ArmBody interface {
	// Kind は JSON の判別子（"call"、"atom"、"none"、"throw"）。
	Kind() string
	isArmBody()
}

// NoneBody はアームの本体の `none`（docs/ir.md「配線」の MatchArm の body kind "none"）。
type NoneBody struct{}

// ThrowBody はアームの本体の `throw E`、`throw E(値, …)`（docs/ir.md「配線」の MatchArm の body kind "throw"）。
type ThrowBody struct {
	// Exception は例外の FQN。
	Exception string `json:"exception"`
	// Args は位置で書いた値を、例外の Params の名前に対応付けたもの。なければ空（JSON では []）。
	Args []CallArg `json:"args"`
	Span Span      `json:"span"`
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (b *ThrowBody) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, b) }

var (
	_ ArmBody = CallExpr{}
	_ ArmBody = AtomExpr{}
	_ ArmBody = NoneBody{}
	_ ArmBody = ThrowBody{}
)

// Kind は "none"。
func (NoneBody) Kind() string { return "none" }

// Kind は "throw"。
func (ThrowBody) Kind() string { return "throw" }

func (CallExpr) isArmBody()  {}
func (AtomExpr) isArmBody()  {}
func (NoneBody) isArmBody()  {}
func (ThrowBody) isArmBody() {}

// MarshalJSON は kind を付けて書く。
func (NoneBody) MarshalJSON() ([]byte, error) { return []byte(`{"kind":"none"}`), nil }

// MarshalJSON は kind を付けて書く。
func (b ThrowBody) MarshalJSON() ([]byte, error) {
	type alias ThrowBody
	return marshalKind("throw", alias(b))
}

var armBodyTable = map[string]func([]byte) (ArmBody, error){
	"call":  asValue[ArmBody, CallExpr],
	"atom":  asValue[ArmBody, AtomExpr],
	"none":  asValue[ArmBody, NoneBody],
	"throw": asValue[ArmBody, ThrowBody],
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
