package sema

import (
	"strings"
	"unicode"

	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	syntoken "github.com/mew-ton/hozo/internal/syntax/token"
)

// checkName は NAME-1（docs/check.md「名前空間」。warning）: model / scalar / exception / union / classify の名前は
// 先頭が大文字、state / op / effect / read / decorator は先頭が小文字。`_` で始まる名前も警告。語彙は対象外。
func (a *analyzer) checkName(s *symbol) {
	upper := s.kind == kindModel || s.kind == kindScalar || s.kind == kindException || s.kind == kindUnion || s.kind == kindClassify
	// RES-7（docs/resolve.md「名前空間の併合と重複」）: 核の型と形の名前は、型と分類の宣言の名前にできない。
	// 型の位置と match の by の位置は、この名前を、解決せずに読む（docs/grammar.md「型」）ので、宣言しても、名前だけでは指せない
	if upper && syntoken.IsTypeName(s.name) {
		a.errorf("RES-7", s.nameSpan, "%s の名前に %s は使えない（はじめから宣言されている、型の名前。型の位置では、いつも、その型として読む）", s.kind, s.name)
	}
	if s.vocab {
		return
	}
	a.checkCase(s.name, s.nameSpan, upper, s.kind.String())
}

// checkCaseName は NAME-1 の classify のケース名（先頭が大文字）。
func (a *analyzer) checkCaseName(s *symbol, name ast.Ident) {
	if s.vocab {
		return
	}
	a.checkCase(name.Name, name.Span, true, "classify のケース")
}

func (a *analyzer) checkCase(name string, span source.Span, upper bool, what string) {
	if name == "" {
		return
	}
	if strings.HasPrefix(name, "_") {
		a.warnf("NAME-1", span, "%s の名前 %s は _ で始めない", what, name)
		return
	}
	first := rune(name[0])
	switch {
	case upper && !unicode.IsUpper(first):
		a.warnf("NAME-1", span, "%s の名前 %s は UpperCamelCase で書く", what, name)
	case !upper && !unicode.IsLower(first):
		a.warnf("NAME-1", span, "%s の名前 %s は lowerCamelCase で書く", what, name)
	}
}
