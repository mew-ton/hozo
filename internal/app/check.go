package app

import (
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// Check は cli.hozo の op check（docs/cli.md「check」）。internal/project.Load が 1〜4 を行い、ここは 5:
// error が一つでもあれば例外（値は error と warning のすべて）、なければ CheckResult（warning）。
// 解析した Project は WriteOutputs のために a.checked に置く。
//
// hozo.toml が見つからず、--config / HOZO_CONFIG の指定もなければ「設定なし」で検査する（docs/config.md「置き場と探し方」）:
// 作業ディレクトリがルート、sources は既定で、CFG-* は出ない（internal/project が除く）。例外になるかどうかは
// 残りの診断だけで決まる。指定されたファイルが読めないときは設定なしではなく CFG-7（ConfigInvalid）。
//
// 例外の選び方（docs/cli.md「check」の 5、docs/config.md「診断」の最後の段落、docs/diagnostics.md「終了コード」）:
// error の中に CFG-*（CFG-5 以外）か MAN-* があれば設定の誤りで、それらがすべて CFG-16 なら LanguageMismatch、
// そうでなければ ConfigInvalid。設定の誤りがなければ CheckFailed。
// プラグインの check が異常終了すれば proc.Crashed / proc.MalformedOutput（asProc）。
func (a *App) Check(in hozoc.CheckInput) (hozoc.CheckResult, error) {
	a.checked = nil
	p := project.LoadWith(project.Options{Root: a.Dir, ConfigPath: str(in.Config), Stderr: a.Stderr, Env: a.Env})
	if p.RunErr != nil {
		return hozoc.CheckResult{}, asProc(p.RunErr)
	}
	diags := p.Diags
	if err := classify(diags, p.Files); err != nil {
		return hozoc.CheckResult{}, err
	}
	a.checked = p
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
		if d.Code != "CFG-16" {
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
