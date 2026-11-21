// Package diag は診断の構造、並べ替え、text と JSON の表示を持つ（docs/diagnostics.md）。
package diag

import (
	"github.com/mew-ton/hozo/internal/source"
)

// Severity は診断の重さ。
type Severity int

const (
	// Error は誤り。一つでもあれば hozoc check は失敗する（docs/diagnostics.md「終了コード」）。
	Error Severity = iota
	// Warning は警告。
	Warning
)

// String は表示に使う名前（"error" / "warning"）を返す。
func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	}
	return "unknown"
}

// MarshalText は JSON で "error" / "warning" にする。
func (s Severity) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// Diagnostic は一件の診断（docs/diagnostics.md「構造」）。
type Diagnostic struct {
	Severity Severity
	// Code は規則の ID（"REL-4"、"RES-1"、"SYNTAX"、"PLUGIN"）。
	Code string
	// Message は一文。何が、どこで、どう違うか。名前は FQN で。
	Message string
	// Span は位置（バイトのオフセット）。位置がなければ nil。
	Span *source.Span
	// File は Span が nil のときのファイル（hozo.toml など）。空なら全体。
	File string
	// Related は関連する位置（重複の相手、例外の出どころの行など）。
	Related []Related
	// Data は機械が読む補足（docs/check.md「Data」）。JSON にだけ出す。
	Data map[string]any
}

// Related は診断に関連する位置。
type Related struct {
	Span    source.Span
	Message string
}

// Path は診断のファイルのパスを返す。Span があればその FileSet 上のパス、なければ File。
// どちらもなければ空文字列。
func (d *Diagnostic) Path(fs *source.FileSet) string {
	if d.Span != nil && fs != nil {
		return fs.Path(d.Span.File)
	}
	return d.File
}

// Count は error と warning の数を返す。
func Count(diags []Diagnostic) (errors, warnings int) {
	for _, d := range diags {
		switch d.Severity {
		case Error:
			errors++
		case Warning:
			warnings++
		}
	}
	return errors, warnings
}
