package syntax

import (
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
)

// syntaxError は構文エラーの診断を作る。Code は "SYNTAX"（docs/diagnostics.md「構造」）。
func syntaxError(span source.Span, message string) diag.Diagnostic {
	s := span
	return diag.Diagnostic{
		Severity: diag.Error,
		Code:     "SYNTAX",
		Message:  message,
		Span:     &s,
	}
}
