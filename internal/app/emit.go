package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/emit/golang"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/ir"
	sdk "github.com/mew-ton/hozo/plugin"
)

// OutputsFile は各名前空間の Dir(N) に置く、前回書いたパスの一覧（docs/cli.md「emit」の 6 と 8）。
// JSON の文字列の配列で、パスはルートからの相対パス（"/" 区切り）、辞書順。
const OutputsFile = ".hozoc-outputs.json"

// output は書き出す予定のファイル一つ。
type output struct {
	path    string // ルートからの相対パス（"/" 区切り）
	content []byte
	source  string // 出どころ（CFG-5 のメッセージ用）: 「既定の出力」か「プラグイン <名前>」
	ns      string // 名前空間
}

// WriteOutputs は cli.hozo の op writeOutputs（docs/cli.md「emit」の 1〜8）。効果 syncOutputs を持つ葉の op なので、
// 生成したコードでは状態の単位 hozoc.States.OutputsSync の命令になる（[App.States]）。checked は Check が返したもの
// （warning）で、その中身（Program、設定、マニフェスト）は Check が a.checked に置いた Project から取る。
//
//  0. 設定なし（hozo.toml が見つからない。docs/config.md「置き場と探し方」）なら、書き出すものがないので CFG-7 を
//     一つ作って ConfigInvalid（docs/cli.md「手で書くもの」）。check の warning も一緒に運ぶ。
//  1. namespaces が空でなければ、その名前空間（と子孫）だけを対象にする。hozo.toml にない名前（設定を解決できない）なら UnknownNamespace。
//  2. 対象の各名前空間について、Conf(N).plugins の各プラグイン（[emit] を持つもの）を runEmit で起動し、files と select を集める。
//  3. 集めた select から、language が go の名前空間の既定の出力（internal/emit/golang）を作る。
//     GOEMIT-* とプラグインの errors は集めておき、すべて作り終えてから CheckFailed にする。
//  4. すべての名前空間のファイルを集めてからパスの重複を確かめる（CFG-5 → OutputConflict）。
//  5. dryRun なら書く予定のパスを Written に入れて返す。
//  6. 各名前空間の <Dir(N)>/.hozoc-outputs.json を読み、今回書かないパスのファイルを消す。
//  7. 辞書順にファイルを書く。内容が同じなら書き直さない（Written には入れる）。
//  8. .hozoc-outputs.json を書く。書くものが一つもない名前空間では作らず、前回のものがあれば消す。
//
// 1〜4 は [App.plan]。
func (a *App) WriteOutputs(in hozoc.EmitInput, checked hozoc.CheckResult) (hozoc.EmitResult, error) {
	p := a.checked
	if p == nil || len(checked.Warnings) != len(p.Diags) {
		return hozoc.EmitResult{}, errors.New("hozoc: WriteOutputs の checked が直前の Check の結果でない")
	}
	// 0. 設定なし
	if p.NoConfig {
		return hozoc.EmitResult{}, noConfig(p)
	}
	pl, err := a.plan(p, in.Namespaces)
	if err != nil {
		return hozoc.EmitResult{}, err
	}

	result := hozoc.EmitResult{Warnings: toDiagnostics(pl.warnings, p.Files), Written: []hozoc.Path{}, Removed: []hozoc.Path{}}
	for _, o := range pl.outputs {
		result.Written = append(result.Written, hozoc.Path(o.path))
	}

	// 5. dry-run
	if in.DryRun != nil && *in.DryRun {
		return result, nil
	}

	// 6. 前回の出力のうち今回書かないものを消す
	current := map[string]bool{}
	for _, o := range pl.outputs {
		current[o.path] = true
	}
	for _, ns := range pl.targets {
		if pl.dirs[ns] == "" {
			continue
		}
		previous, err := readOutputsFile(p.Root, pl.dirs[ns])
		if err != nil {
			return result, err
		}
		for _, prev := range previous {
			if current[prev] {
				continue
			}
			abs := filepath.Join(p.Root, filepath.FromSlash(prev))
			if err := os.Remove(abs); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return result, fmt.Errorf("hozoc: %s を消せない: %w", prev, err)
			}
			result.Removed = append(result.Removed, hozoc.Path(prev))
		}
	}
	sort.Slice(result.Removed, func(i, j int) bool { return result.Removed[i] < result.Removed[j] })

	// 7. 書く（内容が同じなら書き直さない）
	for _, o := range pl.outputs {
		if err := writeIfChanged(filepath.Join(p.Root, filepath.FromSlash(o.path)), o.content); err != nil {
			return result, fmt.Errorf("hozoc: %s を書けない: %w", o.path, err)
		}
	}

	// 8. .hozoc-outputs.json
	for _, ns := range pl.targets {
		dir := pl.dirs[ns]
		if dir == "" {
			continue
		}
		record := filepath.Join(p.Root, filepath.FromSlash(dir), OutputsFile)
		data := pl.record(ns)
		if data == nil {
			// 何も出力しない名前空間（プラグインがない、何も選ばれていない）には記録も置かない
			if err := os.Remove(record); err != nil && !os.IsNotExist(err) {
				return result, fmt.Errorf("hozoc: %s/%s を消せない: %w", dir, OutputsFile, err)
			}
			continue
		}
		if err := writeIfChanged(record, data); err != nil {
			return result, fmt.Errorf("hozoc: %s/%s を書けない: %w", dir, OutputsFile, err)
		}
	}
	return result, nil
}

// UpToDate は cli.hozo の分類 OutputsSync の述語（docs/cli.md「cli.hozo」「手で書くもの」）: 生成したファイルが、今の仕様と
// 設定から生成し直したものと一致しているか。生成したコードでは状態の単位 hozoc.States.OutputsSync の問い合わせになる。
//
// すべての名前空間について書き出す予定（[App.plan]。プラグインも起動する）を作り、ディスクと比べる。一致していると言うのは、
// 書く予定のファイルがすべて同じ内容であり、.hozoc-outputs.json が今回の一覧と同じで（何も出力しない名前空間にはなく）、
// 前回書いて今回は書かないファイルが残っていないとき。emit を走らせても何も書かず何も消さない、ということ。
// 仕様や設定に誤りがあって生成し直せなければ、その例外を返す（check / emit と同じもの）。
//
// 見るのは、直前の Check が解析した Project（同じ App で check か emit を走らせた後）。なければ、作業ディレクトリから
// hozo.toml を探して解析する。
func (a *App) UpToDate() (bool, error) {
	p := a.checked
	if p == nil {
		p = project.LoadWith(project.Options{Root: a.Dir, Stderr: a.Stderr, Env: a.Env})
		if p.RunErr != nil {
			return false, asProc(p.RunErr)
		}
		if err := classify(p.Diags, p.Files); err != nil {
			return false, err
		}
	}
	if p.NoConfig {
		return false, noConfig(p)
	}
	pl, err := a.plan(p, nil)
	if err != nil {
		return false, err
	}
	current := map[string]bool{}
	for _, o := range pl.outputs {
		current[o.path] = true
		if old, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(o.path))); err != nil || !bytes.Equal(old, o.content) {
			return false, nil // 足りないか、古い
		}
	}
	for _, ns := range pl.targets {
		dir := pl.dirs[ns]
		if dir == "" {
			continue
		}
		record, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(dir), OutputsFile))
		if err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("hozoc: %s/%s を読めない: %w", dir, OutputsFile, err)
		}
		if want := pl.record(ns); !bytes.Equal(record, want) {
			// 記録が今回の一覧と違う。前回書いて今回は書かないファイルが残っていれば、それは余分
			return false, nil
		}
	}
	return true, nil
}

// noConfig は、設定なしで emit しようとしたときの ConfigInvalid（CFG-7。docs/cli.md「手で書くもの」）。
func noConfig(p *project.Project) error {
	missing := diag.Diagnostic{Severity: diag.Error, Code: "CFG-7", File: config.FileName,
		Message: "hozo.toml が見つからないので、書き出す名前空間がない（作業ディレクトリから親へ向かって探した。--config か HOZO_CONFIG で指定できる）"}
	return hozoc.ConfigInvalid{Value: diagnostics(append([]diag.Diagnostic{missing}, p.Diags...), p.Files)}
}

// plan は一度の emit が書き出す予定のすべて（docs/cli.md「emit」の 1〜4 の結果）。
type plan struct {
	// targets は対象の名前空間（Program の順）。dirs はその Dir(N)（ルートからの相対パス）。
	targets []string
	dirs    map[string]string
	// outputs は書く予定のファイル（パスの辞書順。重複はない）。
	outputs []output
	// warnings は check の warning に、既定の出力とプラグインの warning を足したもの（並べ替え済み）。
	warnings []diag.Diagnostic
}

// record は名前空間 ns の .hozoc-outputs.json の中身（今回書くパスの一覧。辞書順の JSON の配列）。
// 書くものが一つもなければ nil（記録を置かない）。
func (pl *plan) record(ns string) []byte {
	var paths []string
	for _, o := range pl.outputs {
		if o.ns == ns {
			paths = append(paths, o.path)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return nil // 文字列の配列なので起きない
	}
	return append(data, '\n')
}

// plan は docs/cli.md「emit」の 1〜4: 対象を決め、プラグインを起動して files と select を集め、select から既定の出力を作り、
// パスの重複を確かめる。何も書かない。誤りは cli.hozo の例外で返す（UnknownNamespace、proc.Crashed / proc.MalformedOutput、
// ConfigInvalid、CheckFailed、OutputConflict）。
//
// 既定の出力は、プラグインが選んだ宣言だけを出す（docs/emit-go.md「出力の対象」）ので、プラグインを先に起動する。
// 対象の名前空間 N の出力が、対象でない Go の名前空間 M の型を使うときは、M のプラグインも起動して select だけを使う
// （M の出力がその型を持つかで、import するか複製するかが決まる。docs/emit-go.md「配置」）。
//
// プラグインの files の path はルートからの相対パス（docs/plugin-protocol.md「emit」）。`..` や絶対パスは PLG-1、
// 同じ応答の中の重複は PLG-2、select が対象の名前空間にない宣言や付いていないデコレータを指していれば PLG-3 で、
// どれも proc.MalformedOutput。
func (a *App) plan(p *project.Project, namespaces []string) (*plan, error) {
	views := p.Views()

	// 1. 対象の名前空間
	targets, err := selectTargets(p, views, namespaces)
	if err != nil {
		return nil, err
	}
	pl := &plan{targets: targets, dirs: map[string]string{}}
	for _, ns := range targets {
		pl.dirs[ns] = outputsDir(views[ns])
	}

	// warnings は check の warning（p.Diags）から始める（EmitResult.warnings は check の warning と、GOEMIT-* の warning、
	// プラグインの warnings のすべて）。
	warnings := append([]diag.Diagnostic{}, p.Diags...)
	var failures []diag.Diagnostic
	var outputs []output

	// 2. プラグイン。selects は名前空間ごとの select を合わせたもの。keep が false なら select だけを使う。
	selects := map[string]sdk.Select{}
	runPlugins := func(ns string, keep bool) error {
		var sels []sdk.Select
		for _, m := range p.Plugins(ns) {
			if !m.HasEmit() {
				continue
			}
			resp, err := a.runEmit(p, m, views[ns])
			if err != nil {
				var pe *hostplugin.PluginError
				if errors.As(err, &pe) {
					d := pe.Diagnostic(p.ManifestPath(m.Name))
					return hozoc.ConfigInvalid{Value: diagnostics(append(append([]diag.Diagnostic{}, warnings...), d), p.Files)}
				}
				return err
			}
			if err := hostplugin.CheckSelect(m.Name, views[ns], resp.Select); err != nil {
				return asProc(err)
			}
			sels = append(sels, resp.Select)
			if !keep {
				continue
			}
			for _, w := range resp.Warnings {
				warnings = append(warnings, p.PluginWarning(m.Name, w))
			}
			for _, e := range resp.Errors {
				e.Severity = "error"
				failures = append(failures, p.PluginDiagnostic(m.Name, e))
			}
			seen := map[string]bool{}
			for _, f := range resp.Files {
				rel, ok := pluginPath(f.Path)
				if !ok {
					return asProc(&hostplugin.MalformedOutputError{Plugin: m.Name, Reason: fmt.Sprintf("PLG-1: files の path %q がルートの外に出る", f.Path)})
				}
				if seen[rel] {
					return asProc(&hostplugin.MalformedOutputError{Plugin: m.Name, Reason: fmt.Sprintf("PLG-2: files の path %q が二度ある", f.Path)})
				}
				seen[rel] = true
				outputs = append(outputs, output{path: rel, content: []byte(f.Content), source: "プラグイン " + m.Name, ns: ns})
			}
		}
		selects[ns] = hostplugin.MergeSelect(sels...)
		return nil
	}
	for _, ns := range targets {
		if err := runPlugins(ns, true); err != nil {
			return nil, err
		}
	}

	// 3. 既定の出力。exported は、Go の名前空間の出力が持つ型（別の名前空間の出力はそれを import する）。
	isGo := func(ns string) bool {
		conf, ok := p.Config.Resolve(ns)
		return ok && conf.Language == "go"
	}
	exported := map[string]bool{}
	export := func(ns string) {
		for _, fqn := range golang.Exports(views[ns], selects[ns]) {
			exported[fqn] = true
		}
	}
	for _, ns := range targets {
		if !isGo(ns) {
			continue
		}
		export(ns)
		for _, used := range golang.Uses(views[ns], selects[ns]) {
			if _, done := selects[used]; done || views[used] == nil {
				continue
			}
			if err := runPlugins(used, false); err != nil {
				return nil, err
			}
			export(used)
		}
	}
	for _, ns := range targets {
		if !isGo(ns) {
			continue
		}
		files, ds := golang.Emit(views[ns], golang.Options{Root: p.Root, Select: selects[ns], Exported: exported})
		warnings, failures = splitDiags(ds, warnings, failures)
		for _, f := range files {
			outputs = append(outputs, output{path: cleanPath(f.Path), content: f.Content, source: "既定の出力", ns: ns})
		}
	}
	if len(failures) > 0 {
		all := append(append([]diag.Diagnostic{}, failures...), warnings...)
		diag.Sort(all, p.Files, nil)
		return nil, hozoc.CheckFailed{Value: diagnostics(all, p.Files)}
	}
	diag.Sort(warnings, p.Files, nil)
	sort.SliceStable(outputs, func(i, j int) bool { return outputs[i].path < outputs[j].path })

	// 4. CFG-5
	if conflicts := findConflicts(outputs); len(conflicts) > 0 {
		all := append(conflicts, warnings...)
		return nil, hozoc.OutputConflict{Value: diagnostics(all, p.Files)}
	}
	pl.outputs, pl.warnings = outputs, warnings
	return pl, nil
}

// runEmit は cli.hozo の op runEmit: プラグインを一つ起動して展開させる（docs/plugin-protocol.md「起動」）。
// どの配線からも呼ばれず、go-cli の選択に入らない（docs/cli.md「手で書くもの」）ので、生成した Ops には載らない。
// cli.hozo には境界の記述として残り、ここではその中身をそのまま関数にする: 起動は internal/plugin.Runner で、
// 作業ディレクトリはプロジェクトのルート、要求の view は名前空間から見た IR。
// 誤りは proc.Crashed / proc.MalformedOutput（asProc）か、終了コード 2 の *hostplugin.PluginError（呼ぶ側が MAN-* にする）。
func (a *App) runEmit(p *project.Project, m *hostplugin.Manifest, view *ir.View) (*sdk.EmitResponse, error) {
	req := sdk.EmitRequest{Protocol: hostplugin.Protocol, Direction: "emit", View: view}
	var resp sdk.EmitResponse
	if err := (hostplugin.Runner{Root: p.Root, Stderr: a.Stderr, Env: a.Env}).Run(m, p.ManifestDirs[m.Name], &req, &resp); err != nil {
		return nil, asProc(err)
	}
	return &resp, nil
}

// selectTargets は対象の名前空間（View のある名前空間）を Program の順で返す（docs/cli.md「emit」の 1）。
// names が空でなければ、その名前空間と子孫だけ。設定を解決できない名前（hozo.toml にもその祖先にもテーブルがない）があれば UnknownNamespace。
func selectTargets(p *project.Project, views map[string]*ir.View, names []string) ([]string, error) {
	var unknown []string
	for _, n := range names {
		if _, ok := p.Config.Resolve(n); !ok {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return nil, &unknownNamespace{names: unknown}
	}
	var targets []string
	for i := range p.Sema.Program.Namespaces {
		ns := p.Sema.Program.Namespaces[i].Name
		if views[ns] == nil {
			continue
		}
		if len(names) > 0 && !underAny(ns, names) {
			continue
		}
		targets = append(targets, ns)
	}
	return targets, nil
}

// underAny は ns が names のどれかと等しいか、その子孫か。
func underAny(ns string, names []string) bool {
	for _, n := range names {
		if ns == n || strings.HasPrefix(ns, n+".") {
			return true
		}
	}
	return false
}

// splitDiags は診断を warning と error に分ける。
func splitDiags(ds, warnings, failures []diag.Diagnostic) ([]diag.Diagnostic, []diag.Diagnostic) {
	for _, d := range ds {
		if d.Severity == diag.Error {
			failures = append(failures, d)
		} else {
			warnings = append(warnings, d)
		}
	}
	return warnings, failures
}

// pluginPath はプラグインの files の path をルートからの相対パスに正規化する。`..` を含む、絶対パス、空なら false（PLG-1）。
func pluginPath(p string) (string, bool) {
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") || filepath.VolumeName(p) != "" {
		return "", false
	}
	clean := path.Clean(filepath.ToSlash(p))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// cleanPath はルートからの相対パスを "/" 区切りに正規化する。
func cleanPath(p string) string {
	return path.Clean(filepath.ToSlash(p))
}

// findConflicts は同じパスが二つ以上の出どころから出ていれば CFG-5 の診断を作る（docs/config.md「診断」）。
// outputs はパスの順に並んでいること。
func findConflicts(outputs []output) []diag.Diagnostic {
	var diags []diag.Diagnostic
	for i := 0; i < len(outputs); {
		j := i + 1
		for j < len(outputs) && outputs[j].path == outputs[i].path {
			j++
		}
		if j-i > 1 {
			var sources []string
			for _, o := range outputs[i:j] {
				sources = append(sources, fmt.Sprintf("%s（名前空間 %s）", o.source, o.ns))
			}
			diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "CFG-5", File: outputs[i].path,
				Message: fmt.Sprintf("%s に二つ以上の出どころが書こうとした: %s", outputs[i].path, strings.Join(sources, "、"))})
		}
		i = j
	}
	return diags
}

// outputsDir は .hozoc-outputs.json を置く名前空間のディレクトリ Dir(N)（View.config.dir。空なら out）。
func outputsDir(v *ir.View) string {
	if v == nil {
		return ""
	}
	d := v.Config.Dir
	if d == "" {
		d = v.Config.Out
	}
	if d == "" {
		return ""
	}
	return cleanPath(d)
}

// readOutputsFile は <root>/<dir>/.hozoc-outputs.json を読む。ファイルがなければ空。
func readOutputsFile(root, dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), OutputsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("hozoc: %s/%s を読めない: %w", dir, OutputsFile, err)
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("hozoc: %s/%s を読めない: %w", dir, OutputsFile, err)
	}
	for i, p := range paths {
		paths[i] = cleanPath(p)
	}
	return paths, nil
}

// writeIfChanged は内容が違うときだけ書く。ディレクトリは作る。
func writeIfChanged(abs string, content []byte) error {
	if old, err := os.ReadFile(abs); err == nil && bytes.Equal(old, content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, content, 0o644)
}
