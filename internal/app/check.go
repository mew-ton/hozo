package app

import (
	"context"
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/gen/hozo/compiler"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// Check は spec/cli.hozo の op check（手順は、その説明）。compiler.Leaves のメソッド。internal/project.LoadWith が 1〜4 を行い、ここは 5:
// error が一つでもあれば例外（値は error と warning のすべて）、なければ CheckResult（warning）。
// 解析した Project は CollectOutputs のために a.checked に置く。
//
// hozo.toml が見つからず、--config / HOZO_CONFIG の指定もなければ「設定なし」で検査する（docs/config.md「置き場と探し方」）:
// 作業ディレクトリがルート、sources は既定で、CFG-* は出ない（internal/project が除く）。例外になるかどうかは
// 残りの診断だけで決まる。指定されたファイルが読めないときは設定なしではなく CFG-7（ConfigInvalid）。
//
// プラグインを探す場所のうちプロジェクトの外のもの（docs/plugin-protocol.md「探索」の 2 と 3）は、入力の pluginPath
// （HOZO_PLUGIN_PATH）と a.Home。プロセスの環境は読まない。
//
// 例外の選び方（spec/cli.hozo の check の手順の 5、docs/config.md「診断」の最後の段落、docs/diagnostics.md「終了コード」）:
// error の中に CFG-*（CFG-5 以外）か MAN-* があれば設定の誤りで、それらがすべて CFG-16 なら LanguageMismatch、
// そうでなければ ConfigInvalid。設定の誤りがなければ CheckFailed。
// プラグインの check が異常終了すれば compiler.Crashed / compiler.MalformedOutput（asProc）。
//
// プラグインは、受け取った ctx で起動する: ctx が中断されていれば、次のプラグインを起動せずに、その error を返す。
// cmd/hozoc が渡す ctx は context.Background() で、中断されることはない。
func (a *App) Check(ctx context.Context, in compiler.CheckInput) (compiler.CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return compiler.CheckResult{}, err
	}
	a.checked, a.planned = nil, nil
	a.pluginPath = str(in.PluginPath)
	a.cacheDir = str(in.CacheDir)
	p := project.LoadWith(project.Options{Root: a.Dir, ConfigPath: str(in.Config), Stderr: a.Stderr, CacheDir: a.cacheDir,
		PluginPath: a.pluginPath, Home: a.Home})
	if p.RunErr != nil {
		return compiler.CheckResult{}, asProc(p.RunErr)
	}
	diags := p.Diags
	if err := classify(diags, p.Files); err != nil {
		return compiler.CheckResult{}, err
	}
	a.checked = p
	return compiler.CheckResult{Warnings: toDiagnostics(diags, p.Files)}, nil
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
		return languageMismatch(diags, fs)
	case cfgErrors > 0:
		return configInvalid(diags, fs)
	}
	return checkFailed(diags, fs)
}

// isConfigCode は設定とマニフェストの診断のコードか（CFG-*、MAN-*）。CFG-5 は OutputConflict なので除く。
func isConfigCode(code string) bool {
	if code == "CFG-5" {
		return false
	}
	return strings.HasPrefix(code, "CFG-") || strings.HasPrefix(code, "MAN-")
}
