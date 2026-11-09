package sema

import (
	"strings"
	"unicode"

	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
)

// checkName は NAME-1（docs/check.md「名前空間」。warning）: model / scalar / exception / union / classify の名前は
// 先頭が大文字、state / op / effect / decorator は先頭が小文字。`_` で始まる名前も警告。語彙は対象外。
func (a *analyzer) checkName(s *symbol) {
	if s.vocab {
		return
	}
	upper := s.kind == kindModel || s.kind == kindScalar || s.kind == kindException || s.kind == kindUnion || s.kind == kindClassify
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
		a.warnf("NAME-1", span, "%s の名前 %s は _ で始めない（設計書 §3.3）", what, name)
		return
	}
	first := rune(name[0])
	switch {
	case upper && !unicode.IsUpper(first):
		a.warnf("NAME-1", span, "%s の名前 %s は UpperCamelCase で書く（設計書 §3.3）", what, name)
	case !upper && !unicode.IsLower(first):
		a.warnf("NAME-1", span, "%s の名前 %s は lowerCamelCase で書く（設計書 §3.3）", what, name)
	}
}
