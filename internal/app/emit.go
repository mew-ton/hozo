package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

// OutputsFile は、出力（単位と名前空間の組）ごとの Dir(U, N) に置く、前回書いたパスの一覧（docs/cli.md「emit」の 6 と 8）。
// JSON の文字列の配列で、パスはルートからの相対パス（"/" 区切り）、辞書順。
const OutputsFile = ".hozoc-outputs.json"

// outputKey は出力一つ: 単位（Project.Units の添字）と、その単位が出力する名前空間（docs/cli.md「emit」）。
type outputKey struct {
	unit int
	ns   string
}

// output は書き出す予定のファイル一つ。
type output struct {
	path    string // ルートからの相対パス（"/" 区切り）
	content []byte
	source  string // 出どころ（CFG-5 のメッセージ用）: 「既定の出力」か「トランスパイラ <名前>」
	where   string // どの出力のものか（CFG-5 のメッセージ用）: 何番目の [[emit]] かと名前空間
	dir     string // その出力の Dir(U, N)。記録（.hozoc-outputs.json）の置き場
}

// WriteOutputs は cli.hozo の op writeOutputs（docs/cli.md「emit」の 1〜8）。効果 syncOutputs を持つ葉の op なので、
// 生成したコードでは状態の単位 hozoc.States.OutputsSync の命令になる（[App.States]）。checked は Check が返したもの
// （warning）で、その中身（Program、設定、マニフェスト）は Check が a.checked に置いた Project から取る。
//
//  0. 設定なし（hozo.toml が見つからない。docs/config.md「置き場と探し方」）なら、書き出すものがないので CFG-7 を
//     一つ作って ConfigInvalid（docs/cli.md「手で書くもの」）。check の warning も一緒に運ぶ。
//  1. namespaces が空でなければ、名前空間がそのどれかと同じか子孫である出力だけを対象にする。引数とその子孫のどれも、
//     どの単位の対象でもなければ UnknownNamespace。
//  2. 対象の各出力について、単位の各トランスパイラを runEmit で起動し、files と select を集める。
//  3. 集めた select から、言語が go の単位の既定の出力（internal/emit/golang）を作る。
//     GOEMIT-* とトランスパイラの errors は集めておき、すべて作り終えてから CheckFailed にする。
//  4. すべての出力のファイルを集めてからパスの重複を確かめる（CFG-5 → OutputConflict）。
//  5. dryRun なら書く予定のパスを Written に入れて返す。
//  6. 対象の各出力の <Dir(U, N)>/.hozoc-outputs.json を読み、今回書かないパスのファイルを消す。
//  7. 辞書順にファイルを書く。内容が同じなら書き直さない（Written には入れる）。
//  8. .hozoc-outputs.json を書く。書くものが一つもない出力では作らず、前回のものがあれば消す。
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
	for _, dir := range pl.dirs {
		previous, err := readOutputsFile(p.Root, dir)
		if err != nil {
			return result, err
		}
		for _, prev := range previous {
			// 記録のパスに従うのは、その記録の持ち主の Dir(U, N) の下のものだけ（docs/cli.md「emit」の 6）。
			// それ以外（`..`、絶対パス、別のディレクトリ）は hozoc がこの記録に書くはずのないパスなので、
			// 消さず、報告もしない。
			if current[prev] || !config.Under(prev, dir) {
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
	for _, dir := range pl.dirs {
		record := filepath.Join(p.Root, filepath.FromSlash(dir), OutputsFile)
		data := pl.record(dir)
		if data == nil {
			// 何も書かない出力（何も選ばれていない、in で出力でなくなった）には記録も置かない
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
// すべての出力について書き出す予定（[App.plan]。トランスパイラも起動する）を作り、ディスクと比べる。一致していると言うのは、
// 書く予定のファイルがすべて同じ内容であり、.hozoc-outputs.json が今回の一覧と同じで（何も書かない出力にはなく）、
// 前回書いて今回は書かないファイルが残っていないとき。emit を走らせても何も書かず何も消さない、ということ。
// 仕様や設定に誤りがあって生成し直せなければ、その例外を返す（check / emit と同じもの）。
//
// 見るのは、直前の Check が解析した Project（同じ App で check か emit を走らせた後）。なければ、作業ディレクトリから
// hozo.toml を探して解析する。
func (a *App) UpToDate() (bool, error) {
	p := a.checked
	if p == nil {
		p = project.LoadWith(project.Options{Root: a.Dir, Stderr: a.Stderr, Env: a.Env, PluginPath: a.pluginPath, Home: a.Home})
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
	for _, dir := range pl.dirs {
		record, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(dir), OutputsFile))
		if err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("hozoc: %s/%s を読めない: %w", dir, OutputsFile, err)
		}
		if want := pl.record(dir); !bytes.Equal(record, want) {
			// 記録が今回の一覧と違う。前回書いて今回は書かないファイルが残っていれば、それは余分
			return false, nil
		}
	}
	return true, nil
}

// noConfig は、設定なしで emit しようとしたときの ConfigInvalid（CFG-7。docs/cli.md「手で書くもの」）。
func noConfig(p *project.Project) error {
	missing := diag.Diagnostic{Severity: diag.Error, Code: "CFG-7", File: config.FileName,
		Message: "hozo.toml が見つからないので、書き出す単位がない（作業ディレクトリから親へ向かって探した。--config か HOZO_CONFIG で指定できる）"}
	return hozoc.ConfigInvalid{Value: diagnostics(append([]diag.Diagnostic{missing}, p.Diags...), p.Files)}
}

// plan は一度の emit が書き出す予定のすべて（docs/cli.md「emit」の 1〜4 の結果）。
type plan struct {
	// dirs は対象の出力の Dir(U, N)（ルートからの相対パス。単位の設定の順、名前空間の IR の順）。対象の単位の対象の名前空間で、
	// in のせいで出力でなくなったものの Dir(U, N) も含む（前回のファイルを消すため。docs/cli.md「emit」の 6）。
	dirs []string
	// outputs は書く予定のファイル（パスの辞書順。重複はない）。
	outputs []output
	// warnings は check の warning に、既定の出力とトランスパイラの warning を足したもの（並べ替え済み）。
	warnings []diag.Diagnostic
}

// record は出力先 dir の .hozoc-outputs.json の中身（今回書くパスの一覧。辞書順の JSON の配列）。
// 書くものが一つもなければ nil（記録を置かない）。
func (pl *plan) record(dir string) []byte {
	var paths []string
	for _, o := range pl.outputs {
		if o.dir == dir {
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

// plan は docs/cli.md「emit」の 1〜4: 対象を決め、トランスパイラを起動して files と select を集め、select から既定の出力を作り、
// パスの重複を確かめる。何も書かない。誤りは cli.hozo の例外で返す（UnknownNamespace、proc.Crashed / proc.MalformedOutput、
// ConfigInvalid、CheckFailed、OutputConflict）。
//
// 既定の出力は、トランスパイラが選んだ宣言だけを出す（docs/emit-go.md「出力の対象」）ので、トランスパイラを先に起動する。
// 出力が別の名前空間 M の型を使うときは、M を出力する Go の単位のうち対象でないもののトランスパイラも起動して select だけを使う
// （どの単位の出力がその型を持つかで、import するか複製するかが決まる。docs/emit-go.md「配置」）。
// 出力どうしの import の循環（GOEMIT-5）は、対象の出力から import を辿って届くすべての出力について確かめる。
//
// トランスパイラの files の path はルートからの相対パスで、その起動の Dir(U, N) の下でなければならない
// （docs/plugin-protocol.md「emit」）。そうでないもの（`..`、絶対パス、別のディレクトリ、Dir(U, N) そのもの）は PLG-1、
// 同じ応答の中の重複は PLG-2、select が単位の対象でない宣言や付いていないデコレータを指していれば PLG-3 で、
// どれも proc.MalformedOutput。
func (a *App) plan(p *project.Project, namespaces []string) (*plan, error) {
	views := p.Views()

	// 1. 対象の出力。引数 n が選ぶのは、仕様にある名前空間のうち n 自身と n の子孫で、どれかの単位の対象のもの
	// （その名前空間を対象にするすべての単位の分）。n 自身がどの単位の対象でなくても、子孫を対象にする単位があればよい。
	// 一つも選ばない引数（仕様にない名前、どの単位の対象でもない名前空間）があれば UnknownNamespace。
	pl := &plan{}
	var targets []outputKey
	matched := map[string]bool{}
	for u := range p.Units {
		for _, ns := range p.Covered(u) {
			if len(namespaces) > 0 && !underAny(ns, namespaces, matched) {
				continue
			}
			pl.dirs = append(pl.dirs, p.Config.Dir(u, ns))
			if views[u][ns] != nil {
				targets = append(targets, outputKey{u, ns})
			}
		}
	}
	var unknown []string
	for _, n := range namespaces {
		if !matched[n] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return nil, &unknownNamespace{names: unknown}
	}
	where := func(k outputKey) string {
		return fmt.Sprintf("%d 番目の [[emit]] の名前空間 %s", p.Units[k.unit].Index, k.ns)
	}

	// warnings は check の warning（p.Diags）から始める（EmitResult.warnings は check の warning と、GOEMIT-* の warning、
	// トランスパイラの warnings のすべて）。
	warnings := append([]diag.Diagnostic{}, p.Diags...)
	var failures []diag.Diagnostic
	var outputs []output

	// 2. トランスパイラ。selects は出力ごとの、単位のトランスパイラの select を合わせたもの。keep が false なら select だけを使う。
	selects := map[outputKey]sdk.Select{}
	runTranspilers := func(k outputKey, keep bool) error {
		view := views[k.unit][k.ns]
		dir := p.Config.Dir(k.unit, k.ns)
		var sels []sdk.Select
		for _, t := range p.Units[k.unit].Transpilers {
			resp, err := a.runEmit(p, t, view)
			if err != nil {
				var pe *hostplugin.PluginError
				if errors.As(err, &pe) {
					d := pe.Diagnostic(p.ManifestPath(t.Plugin))
					return hozoc.ConfigInvalid{Value: diagnostics(append(append([]diag.Diagnostic{}, warnings...), d), p.Files)}
				}
				return err
			}
			if err := hostplugin.CheckSelect(t.Plugin.Name, view, resp.Select); err != nil {
				return asProc(err)
			}
			sels = append(sels, resp.Select)
			if !keep {
				continue
			}
			for _, w := range resp.Warnings {
				warnings = append(warnings, p.PluginWarning(t.Name, w))
			}
			for _, e := range resp.Errors {
				e.Severity = "error"
				failures = append(failures, p.PluginDiagnostic(t.Name, e))
			}
			seen := map[string]bool{}
			for _, f := range resp.Files {
				rel, ok := pluginPath(f.Path, dir)
				if !ok {
					return asProc(&hostplugin.MalformedOutputError{Plugin: t.Plugin.Name,
						Reason: fmt.Sprintf("PLG-1: files の path %q が、%s の出力先 %s の下にない", f.Path, where(k), dir)})
				}
				if seen[rel] {
					return asProc(&hostplugin.MalformedOutputError{Plugin: t.Plugin.Name, Reason: fmt.Sprintf("PLG-2: files の path %q が二度ある", f.Path)})
				}
				seen[rel] = true
				outputs = append(outputs, output{path: rel, content: []byte(f.Content), source: "トランスパイラ " + t.Name, where: where(k), dir: dir})
			}
		}
		selects[k] = hostplugin.MergeSelect(sels...)
		return nil
	}
	for _, k := range targets {
		if err := runTranspilers(k, true); err != nil {
			return nil, err
		}
	}

	// 3. 既定の出力。別の名前空間 M の型は、M を出力する Go の単位のうち、設定の順で最初に出力がその型を持つものから import する
	// （docs/emit-go.md「配置」）。exports は出力が持つ型（その出力の選択だけで決まる）。
	isGo := func(u int) bool { return p.Units[u].Language == "go" }
	exports := map[outputKey][]string{}
	exportsOf := func(k outputKey) ([]string, error) {
		if ex, done := exports[k]; done {
			return ex, nil
		}
		if _, ran := selects[k]; !ran {
			if err := runTranspilers(k, false); err != nil {
				return nil, err
			}
		}
		exports[k] = golang.Exports(views[k.unit][k.ns], selects[k])
		return exports[k], nil
	}
	options := map[outputKey]golang.Options{}
	optionsOf := func(k outputKey) (golang.Options, error) {
		if o, done := options[k]; done {
			return o, nil
		}
		if _, ran := selects[k]; !ran {
			if err := runTranspilers(k, false); err != nil {
				return golang.Options{}, err
			}
		}
		exported := map[string]string{}
		for _, m := range golang.Uses(views[k.unit][k.ns], selects[k]) {
			// 自分の単位が M も出力するなら、その出力を先に見る（単位どうしは独立で、同じ単位の中で閉じられる
			// 依存を別の単位へ向けない）。残りは設定の順（docs/emit-go.md「配置」）。
			covering := p.Config.Covering(m)
			sort.SliceStable(covering, func(i, j int) bool { return covering[i] == k.unit && covering[j] != k.unit })
			for _, u := range covering {
				if !isGo(u) || views[u][m] == nil {
					continue
				}
				ex, err := exportsOf(outputKey{u, m})
				if err != nil {
					return golang.Options{}, err
				}
				for _, fqn := range ex {
					if _, earlier := exported[fqn]; !earlier {
						exported[fqn] = p.Config.Dir(u, m)
					}
				}
			}
		}
		options[k] = golang.Options{Root: p.Root, Select: selects[k], Exported: exported}
		return options[k], nil
	}
	for _, k := range targets {
		if !isGo(k.unit) {
			continue
		}
		opts, err := optionsOf(k)
		if err != nil {
			return nil, err
		}
		files, ds := golang.Emit(views[k.unit][k.ns], opts)
		warnings, failures = splitDiags(ds, warnings, failures)
		for _, f := range files {
			outputs = append(outputs, output{path: config.CleanRel(f.Path), content: f.Content, source: "既定の出力", where: where(k), dir: p.Config.Dir(k.unit, k.ns)})
		}
	}
	if len(failures) == 0 {
		// GOEMIT-5: 出力どうしの import の循環
		cycles, err := a.importCycles(p, views, targets, isGo, optionsOf)
		if err != nil {
			return nil, err
		}
		failures = append(failures, cycles...)
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

// importCycles は GOEMIT-5（docs/emit-go.md「配置」「診断」）: 出力どうしの import が循環していれば、輪ごとに診断を一つ作る。
// 対象の出力 targets から、既定の出力が実際に import するパッケージ（golang.Imports）を辿って届くすべての出力を見る
// （対象でない出力も、select から既定の出力を作って import だけを見る。docs/cli.md「emit」の 3）。位置は hozo.toml。
func (a *App) importCycles(p *project.Project, views []map[string]*ir.View, targets []outputKey, isGo func(int) bool,
	optionsOf func(outputKey) (golang.Options, error)) ([]diag.Diagnostic, error) {
	byDir := map[string]outputKey{}
	for u := range views {
		for ns := range views[u] {
			byDir[p.Config.Dir(u, ns)] = outputKey{u, ns}
		}
	}
	edges := map[outputKey][]outputKey{}
	var order []outputKey // 見た順（決定的）
	work := append([]outputKey{}, targets...)
	for len(work) > 0 {
		k := work[0]
		work = work[1:]
		if _, seen := edges[k]; seen || !isGo(k.unit) {
			continue
		}
		opts, err := optionsOf(k)
		if err != nil {
			return nil, err
		}
		edges[k] = []outputKey{}
		order = append(order, k)
		for _, dir := range golang.Imports(views[k.unit][k.ns], opts) {
			if to, ok := byDir[config.CleanRel(dir)]; ok {
				edges[k] = append(edges[k], to)
				work = append(work, to)
			}
		}
	}
	label := func(k outputKey) string {
		return fmt.Sprintf("%d 番目の [[emit]] の名前空間 %s（%s）", p.Units[k.unit].Index, k.ns, p.Config.Dir(k.unit, k.ns))
	}
	// 深さ優先で辿り、辿っている途中の出力に戻る辺を見つけたら、その輪を一つ報告する。同じ出力の集まりの輪は一度だけ。
	const (
		white = iota
		grey
		black
	)
	color := map[outputKey]int{}
	reported := map[string]bool{}
	var stack []outputKey
	var diags []diag.Diagnostic
	var visit func(k outputKey)
	visit = func(k outputKey) {
		color[k] = grey
		stack = append(stack, k)
		for _, to := range edges[k] {
			switch color[to] {
			case white:
				visit(to)
			case grey:
				i := len(stack) - 1
				for stack[i] != to {
					i--
				}
				cycle := append([]outputKey{}, stack[i:]...)
				var names, id []string
				for _, c := range cycle {
					names = append(names, label(c))
					id = append(id, p.Config.Dir(c.unit, c.ns))
				}
				sort.Strings(id)
				if key := strings.Join(id, "\x00"); !reported[key] {
					reported[key] = true
					names = append(names, label(to))
					diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: golang.CodeImportCycle, File: relConfig(p),
						Message: "出力どうしの import が循環している: " + strings.Join(names, " → ") +
							"。互いに使う型を一方の名前空間に寄せるか、相手の型を選ばないようにする"})
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[k] = black
	}
	for _, k := range order {
		if color[k] == white {
			visit(k)
		}
	}
	return diags, nil
}

// relConfig は hozo.toml の、診断の File に使う形（ルートからの相対パス）。
func relConfig(p *project.Project) string {
	if p.Config.Path == "" {
		return config.FileName
	}
	if rel, err := filepath.Rel(p.Root, p.Config.Path); err == nil {
		return filepath.ToSlash(rel)
	}
	return p.Config.Path
}

// runEmit は cli.hozo の op runEmit: プラグインを一つ起動して、トランスパイラ一つに展開させる（docs/plugin-protocol.md「起動」）。
// どの配線からも呼ばれず、go-cli の選択に入らない（docs/cli.md「手で書くもの」）ので、生成した Ops には載らない。
// cli.hozo には境界の記述として残り、ここではその中身をそのまま関数にする: 起動は internal/plugin.Runner で、
// 作業ディレクトリはプロジェクトのルート、要求の transpiler はトランスパイラの名前、view は単位での名前空間から見た IR。
// 誤りは proc.Crashed / proc.MalformedOutput（asProc）か、終了コード 2 の *hostplugin.PluginError（呼ぶ側が MAN-* にする）。
func (a *App) runEmit(p *project.Project, t project.Transpiler, view *ir.View) (*sdk.EmitResponse, error) {
	req := sdk.EmitRequest{Protocol: hostplugin.Protocol, Direction: "emit", Transpiler: t.Name, View: view}
	var resp sdk.EmitResponse
	if err := (hostplugin.Runner{Root: p.Root, Stderr: a.Stderr, Env: a.Env}).Run(t.Plugin.Manifest, t.Plugin.Dir, &req, &resp); err != nil {
		return nil, asProc(err)
	}
	return &resp, nil
}

// underAny は ns が names のどれかと等しいか、その子孫か。当たった names の要素を matched に記す（すべて見る）。
func underAny(ns string, names []string, matched map[string]bool) bool {
	hit := false
	for _, n := range names {
		if ns == n || strings.HasPrefix(ns, n+".") {
			matched[n] = true
			hit = true
		}
	}
	return hit
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

// pluginPath はトランスパイラの files の path をルートからの相対パスに正規化する。その起動の Dir(U, N)（dir）の下に
// なければ false（PLG-1。docs/plugin-protocol.md「emit」）: 空、絶対パス、`..` で外へ出るもの、別のディレクトリ、dir そのもの。
func pluginPath(p, dir string) (string, bool) {
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") || filepath.VolumeName(p) != "" {
		return "", false
	}
	clean := config.CleanRel(p)
	if !config.Under(clean, dir) {
		return "", false
	}
	return clean, true
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
				sources = append(sources, fmt.Sprintf("%s（%s）", o.source, o.where))
			}
			diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "CFG-5", File: outputs[i].path,
				Message: fmt.Sprintf("%s に二つ以上の出どころが書こうとした: %s", outputs[i].path, strings.Join(sources, "、"))})
		}
		i = j
	}
	return diags
}

// readOutputsFile は <root>/<dir>/.hozoc-outputs.json を読む。ファイルがなければ空。パスは正規化するだけで、
// dir の下かどうかは確かめない（従うかどうかは呼ぶ側が決める。WriteOutputs の 6）。
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
		paths[i] = config.CleanRel(p)
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
