// Package token は .hozo の字句の種類（docs/grammar.md「字句」）。
package token

import "github.com/mew-ton/hozo/internal/source"

// Kind は字句の種類。
type Kind int

// 字句の種類。予約語は docs/grammar.md「予約語」の表の順。
const (
	// EOF はファイルの終わり。
	EOF Kind = iota
	// Illegal は字句として読めなかったもの。字句解析器が SYNTAX を出している。
	Illegal
	// Ident は識別子（docs/grammar.md「識別子」）。文脈キーワードもこれになる。
	Ident
	// QualifiedName は "." でつないだ識別子（server.User）。
	QualifiedName
	// DocComment は /** ... */。
	DocComment

	// String は文字列リテラル。Text は引用符とエスケープを含む元の字句。
	String
	// Int は整数リテラル。負の数を含む。
	Int
	// Float は小数リテラル。負の数を含む。
	Float
	// Duration は期間リテラル（5m、30s）。
	Duration

	// 宣言の頭。
	Namespace
	Model
	Scalar
	Union
	State
	Fn
	Op
	Exception
	Effect
	Read
	Classify
	Decorator
	Bind

	// 修飾（宣言やフィールドの前に置く）。
	Export
	Override

	// 節の頭。
	Extends
	Of
	On
	To
	By
	For
	As
	Effects
	Reads
	Throws
	Prevents
	Keeps
	Catches
	Recovers
	Halts
	Ignores
	Any
	Case

	// 文の頭と値。
	Val
	Return
	Match
	Disjoint
	Collect
	Default
	Else
	None
	Throw
	Try
	Catch

	// リテラル。
	True
	False

	// 記号（docs/grammar.md「記号」）。
	LBrace    // {
	RBrace    // }
	LParen    // (
	RParen    // )
	LBracket  // [
	RBracket  // ]
	Less      // <
	Greater   // >
	Colon     // :
	Semicolon // ;
	Comma     // ,
	Dot       // .
	Assign    // =
	Question  // ?
	Pipe      // |
	At        // @
	Star      // *
	StarStar  // **
	Arrow     // =>
	ThinArrow // ->
	Ellipsis  // ...
)

// Keywords は予約語から Kind への表（docs/grammar.md「予約語」）。
// docs/testing.md「予約語の一致」のテストの元。
var Keywords = map[string]Kind{
	"namespace": Namespace,
	"model":     Model,
	"scalar":    Scalar,
	"union":     Union,
	"state":     State,
	"fn":        Fn,
	"op":        Op,
	"exception": Exception,
	"effect":    Effect,
	"read":      Read,
	"classify":  Classify,
	"decorator": Decorator,
	"bind":      Bind,

	"export":   Export,
	"override": Override,

	"extends":  Extends,
	"of":       Of,
	"on":       On,
	"to":       To,
	"by":       By,
	"for":      For,
	"as":       As,
	"effects":  Effects,
	"reads":    Reads,
	"throws":   Throws,
	"prevents": Prevents,
	"keeps":    Keeps,
	"catches":  Catches,
	"recovers": Recovers,
	"halts":    Halts,
	"ignores":  Ignores,
	"any":      Any,
	"case":     Case,

	"val":      Val,
	"return":   Return,
	"match":    Match,
	"disjoint": Disjoint,
	"collect":  Collect,
	"default":  Default,
	"else":     Else,
	"none":     None,
	"throw":    Throw,
	"try":      Try,
	"catch":    Catch,

	"true":  True,
	"false": False,
}

// 核の型と形の名前（docs/grammar.md「予約語」「型」）。予約語ではなく、はじめから宣言されている、型の名前で、字句としては Ident。
// 型の位置でだけ、その型になる。同じ名前の型と分類は、宣言できない（docs/resolve.md「診断」の RES-7）。
const (
	// VoidName は、値を返さない op の、戻り値の位置の語。
	VoidName = "void"
	// NeverName は、値が一つもない型の語。
	NeverName = "never"
)

// CoreTypeNames は、核の型の名前（docs/grammar.md「型」の CoreType）。
var CoreTypeNames = []string{"string", "boolean", "int", "float", "datetime", "date", "time", "duration"}

// ShapeNames は、形の語（docs/grammar.md「型」の TypePrefix: list<T>、nullable<T>、single<T>）。
var ShapeNames = []string{"list", "nullable", "single"}

// TypeNames は、核の型と形の名前の 13 語を、docs/grammar.md「予約語」の順に返す。
func TypeNames() []string {
	names := append([]string(nil), CoreTypeNames...)
	names = append(names, VoidName, NeverName)
	return append(names, ShapeNames...)
}

// IsCoreTypeName は s が核の型の名前かを返す。
func IsCoreTypeName(s string) bool { return contains(CoreTypeNames, s) }

// IsShapeName は s が形の語かを返す。
func IsShapeName(s string) bool { return contains(ShapeNames, s) }

// IsTypeName は s が核の型と形の名前（13 語のどれか）かを返す。
func IsTypeName(s string) bool {
	return s == VoidName || s == NeverName || IsCoreTypeName(s) || IsShapeName(s)
}

func contains(words []string, s string) bool {
	for _, w := range words {
		if w == s {
			return true
		}
	}
	return false
}

// Contextual は文脈キーワード（docs/grammar.md「予約語」の二つ目の表）。
// 決まった位置でだけ意味を持ち、字句としては Ident。
var Contextual = []string{"outer", "field", "variant", "post", "put", "delete"}

// IsContextual は s が文脈キーワードかを返す。
func IsContextual(s string) bool { return contains(Contextual, s) }

var symbols = map[Kind]string{
	LBrace: "{", RBrace: "}", LParen: "(", RParen: ")", LBracket: "[", RBracket: "]",
	Less: "<", Greater: ">", Colon: ":", Semicolon: ";", Comma: ",", Dot: ".",
	Assign: "=", Question: "?", Pipe: "|", At: "@", Star: "*", StarStar: "**", Arrow: "=>", ThinArrow: "->", Ellipsis: "...",
}

var keywordNames = invert(Keywords)

func invert(m map[string]Kind) map[Kind]string {
	r := make(map[Kind]string, len(m))
	for s, k := range m {
		r[k] = s
	}
	return r
}

// String は診断に使う名前。予約語と記号はその字句そのもの、それ以外は種類の名前。
func (k Kind) String() string {
	if s, ok := keywordNames[k]; ok {
		return s
	}
	if s, ok := symbols[k]; ok {
		return s
	}
	switch k {
	case EOF:
		return "ファイルの終わり"
	case Illegal:
		return "不正な字句"
	case Ident:
		return "識別子"
	case QualifiedName:
		return "修飾名"
	case DocComment:
		return "ドキュメンテーションコメント"
	case String:
		return "文字列"
	case Int:
		return "整数"
	case Float:
		return "小数"
	case Duration:
		return "期間"
	}
	return "不明な字句"
}

// IsKeyword は k が予約語かを返す。
func (k Kind) IsKeyword() bool {
	_, ok := keywordNames[k]
	return ok
}

// IsLiteral は k がリテラル（String、Int、Float、Duration、true、false）かを返す。
func (k Kind) IsLiteral() bool {
	switch k {
	case String, Int, Float, Duration, True, False:
		return true
	}
	return false
}

// Token は一つの字句。
type Token struct {
	Kind Kind
	// Text は元の字句。予約語・記号・識別子はその文字列、リテラルは引用符やエスケープを含む元の形、
	// DocComment は /** と */ を除いた中身。
	Text string
	Span source.Span
}
