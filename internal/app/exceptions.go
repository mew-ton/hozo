package app

import (
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/gen/hozo/compiler"
	"github.com/mew-ton/hozo/internal/source"
)

// このファイルは、診断を運ぶ例外（cli.hozo の CheckFailed、ConfigInvalid、LanguageMismatch、OutputConflict、TestsNotRun、
// CoverageUnusable）の作り方と読み方。六つとも、引数は一つ（error と warning のすべて。`Diagnostic[]`）で、生成した構造体は、その引数の名前のフィールドを一つ持つ
// （docs/plugin-go.md「exception」）。
//
// **引数の名前（= 生成した構造体のフィールドの名前）を書くのは、carried の宣言と、carriedBy の最後の行の二か所だけ**で、
// internal/app のほかのコードとテストは、下の関数を通す。cli.hozo の引数の名前を変えたら、carried のフィールドの名前を合わせる:
// 合っていなければ、下の型変換（生成した構造体と、フィールドの名前と型が同じでなければ書けない）がコンパイルできない。
// hozoc の json の表示は、この名前によらない（[Presenter] が docs/diagnostics.md「json」の形で出す）。

// carried は、診断を運ぶ例外の構造体の形: 引数ごとのフィールドが一つ。
type carried struct{ Diagnostics []compiler.Diagnostic }

// checkFailed は例外 compiler.CheckFailed（値）。diags は error と warning のすべて。
func checkFailed(diags []diag.Diagnostic, fs *source.FileSet) error {
	return compiler.CheckFailed(carried{toDiagnostics(diags, fs)})
}

// configInvalid は例外 compiler.ConfigInvalid（値）。
func configInvalid(diags []diag.Diagnostic, fs *source.FileSet) error {
	return compiler.ConfigInvalid(carried{toDiagnostics(diags, fs)})
}

// languageMismatch は例外 compiler.LanguageMismatch（値）。
func languageMismatch(diags []diag.Diagnostic, fs *source.FileSet) error {
	return compiler.LanguageMismatch(carried{toDiagnostics(diags, fs)})
}

// outputConflict は例外 compiler.OutputConflict（値）。
func outputConflict(diags []diag.Diagnostic, fs *source.FileSet) error {
	return compiler.OutputConflict(carried{toDiagnostics(diags, fs)})
}

// testsNotRun は例外 compiler.TestsNotRun（値）。diags は、既に境界の形（位置は行と列）の、error と warning のすべて。
func testsNotRun(diags []compiler.Diagnostic) error {
	return compiler.TestsNotRun(carried{diags})
}

// coverageUnusable は例外 compiler.CoverageUnusable（値）。
func coverageUnusable(diags []compiler.Diagnostic) error {
	return compiler.CoverageUnusable(carried{diags})
}

// carriedBy は、例外の値 v（errors.As で取り出した値か、cli.Failure の Value）が、診断を運ぶ例外なら、その診断と true。
func carriedBy(v any) ([]compiler.Diagnostic, bool) {
	var c carried
	switch e := v.(type) {
	case compiler.CheckFailed:
		c = carried(e)
	case compiler.ConfigInvalid:
		c = carried(e)
	case compiler.LanguageMismatch:
		c = carried(e)
	case compiler.OutputConflict:
		c = carried(e)
	case compiler.TestsNotRun:
		c = carried(e)
	case compiler.CoverageUnusable:
		c = carried(e)
	default:
		return nil, false
	}
	return c.Diagnostics, true
}
