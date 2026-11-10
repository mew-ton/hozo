package syntax

import (
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
)

// syntaxError は構文エラーの診断を作る。Code は "SYNTAX"（docs/diagnostics.md「構造」）。
func syntaxError(span source.Span, message string) diag.Diagnostic {
	return codedError("SYNTAX", span, message)
}

// codedError は code を指定した構文解析の誤り。リテラルが 64 ビットに収まらないときの LIT-1（docs/types.md「診断」）に使う。
func codedError(code string, span source.Span, message string) diag.Diagnostic {
	s := span
	return diag.Diagnostic{
		Severity: diag.Error,
		Code:     code,
		Message:  message,
		Span:     &s,
	}
}
