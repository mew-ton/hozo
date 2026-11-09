package app

import (
	"context"
	"strings"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// Check は cli.hozo の op check（docs/cli.md「check」）。internal/project.Load が 1〜4 を行い、ここは 5:
// error が一つでもあれば例外（値は error と warning のすべて）、なければ CheckResult（warning）。
// 解析した Project は WriteOutputs のために l.checked に置く。
//
// 例外の選び方（docs/cli.md「check」の 1、docs/config.md「診断」の最後の段落、docs/diagnostics.md「終了コード」）:
// hozo.toml が見つからなければ CFG-7 で ConfigInvalid。error の中に CFG-*（CFG-5 以外）か MAN-* があれば設定の誤りで、
// それらがすべて CFG-4 なら LanguageMismatch、そうでなければ ConfigInvalid。設定の誤りがなければ CheckFailed。
// プラグインの check が異常終了すれば proc.Crashed / proc.MalformedOutput（asProc）。
func (l *Leaves) Check(_ context.Context, in hozoc.CheckInput) (hozoc.CheckResult, error) {
	l.checked = nil
	p := project.LoadWith(project.Options{Root: l.Dir, ConfigPath: str(in.Config), Stderr: l.Stderr, Env: l.Env})
	if p.RunErr != nil {
		return hozoc.CheckResult{}, asProc(p.RunErr)
	}
	diags := p.Diags
	if p.NoConfig {
		missing := diag.Diagnostic{Severity: diag.Error, Code: "CFG-7", File: config.FileName,
			Message: "hozo.toml が見つからない（作業ディレクトリから親へ向かって探した。--config か HOZO_CONFIG で指定できる）"}
		diags = append([]diag.Diagnostic{missing}, diags...)
		return hozoc.CheckResult{}, hozoc.ConfigInvalid{Value: diagnostics(diags, p.Files)}
	}
	if err := classify(diags, p.Files); err != nil {
		return hozoc.CheckResult{}, err
	}
	l.checked = p
	return hozoc.CheckResult{Warnings: toDiagnostics(diags, p.Files)}, nil
}

// classify は診断の列から例外を決める。error がなければ nil。
func classify(diags []diag.Diagnostic, fs *source.FileSet) error {
	errs, _ := diag.Count(diags)
	if errs == 0 {
		return nil
	}
	cfgErrors, mismatchOnly := 0, true
	for _, d := range diags {
		if d.Severity != diag.Error || !isConfigCode(d.Code) {
			continue
		}
		cfgErrors++
		if d.Code != "CFG-4" {
			mismatchOnly = false
		}
	}
	switch {
	case cfgErrors > 0 && mismatchOnly:
		return hozoc.LanguageMismatch{Value: diagnostics(diags, fs)}
	case cfgErrors > 0:
		return hozoc.ConfigInvalid{Value: diagnostics(diags, fs)}
	}
	return hozoc.CheckFailed{Value: diagnostics(diags, fs)}
}

// isConfigCode は設定とマニフェストの診断のコードか（CFG-*、MAN-*）。CFG-5 は OutputConflict なので除く。
func isConfigCode(code string) bool {
	if code == "CFG-5" {
		return false
	}
	return strings.HasPrefix(code, "CFG-") || strings.HasPrefix(code, "MAN-")
}
