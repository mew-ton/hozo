package app

import (
	"bytes"
	"context"
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
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/ir"
	sdk "github.com/mew-ton/hozo/plugin"
)

// OutputsFile は、出力（単位と名前空間の組）ごとの Dir(U, N) に置く、前回書いたパスの一覧（docs/cli.md「emit」の 5 と 7）。
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
	source  string // 出どころ（CFG-5 のメッセージ用）: 「トランスパイラ <名前>」
	where   string // どの出力のものか（CFG-5 のメッセージ用）: 何番目の [[emit]] かと名前空間
	dir     string // その出力の Dir(U, N)。記録（.hozoc-outputs.json）の置き場
}

// OutputsSync は、状態の分類 OutputsSync（cli.hozo の `classify OutputsSync of outputs`）の単位の実装: 述語 UpToDate
// （問い合わせ）と、効果 syncOutputs を持つ葉の op writeOutputs（命令）。hozoc.New の引数 outputsSync に渡す（[App.Ops]）。
func (a *App) OutputsSync() hozoc.OutputsSyncState { return outputsSync{a} }

// outputsSync は [App.OutputsSync] が返す値。
type outputsSync struct{ a *App }

// WriteOutputs は cli.hozo の op writeOutputs（docs/cli.md「emit」の 1〜7）。効果 syncOutputs を持つ葉の op なので、
// 生成したコードでは、状態の分類 OutputsSync の単位の命令になる。内部の op で、配線の emit が、check の後に呼ぶ（テストは、
// 内側のパッケージ internal/gen/hozoc/inner の Ops から呼ぶ）。checked は Check が返したもの
// （warning）で、その中身（Program、設定、マニフェスト）は Check が a.checked に置いた Project から取る。
// 書き出すファイルは、どれもトランスパイラが返したもので、hozoc が自分で作るファイルは、出力先ごとの記録だけ。
//
//  0. 設定なし（hozo.toml が見つからない。docs/config.md「置き場と探し方」）なら、書き出すものがないので CFG-7 を
//     一つ作って ConfigInvalid（docs/cli.md「手で書くもの」）。check の warning も一緒に運ぶ。
//  1. namespaces が空でなければ、名前空間がそのどれかと同じか子孫である出力だけを対象にする。引数とその子孫のどれも、
//     どの単位の対象でもなければ UnknownNamespace。
//  2. 対象の各出力について、単位の各トランスパイラを起動し、files と warnings と errors を集める。
//     errors は集めておき、すべて起動し終えてから CheckFailed にする。
//  3. すべての出力のファイルを集めてからパスの重複を確かめる（CFG-5 → OutputConflict）。
//  4. dryRun なら書く予定のパスを Written に入れて返す。
//  5. 対象の各出力の <Dir(U, N)>/.hozoc-outputs.json を読み、今回書かないパスのファイルを消す。空になったディレクトリも、
//     Dir(U, N) の下の範囲で消す。
//  6. 辞書順にファイルを書く。内容が同じなら書き直さない（Written には入れる）。
//  7. .hozoc-outputs.json を書く。書くものが一つもない出力では作らず、前回のものがあれば消す。
//
// 1〜3 は [App.plan]。
func (s outputsSync) WriteOutputs(ctx context.Context, in hozoc.EmitInput, checked hozoc.CheckResult) (hozoc.EmitResult, error) {
	a := s.a
	if err := ctx.Err(); err != nil {
		return hozoc.EmitResult{}, err
	}
	p := a.checked
	if p == nil || len(checked.Warnings) != len(p.Diags) {
		return hozoc.EmitResult{}, errors.New("hozoc: WriteOutputs の checked が直前の Check の結果でない")
	}
	// 0. 設定なし
	if p.NoConfig {
		return hozoc.EmitResult{}, noConfig(p)
	}
	pl, err := a.plan(ctx, p, in.Namespaces)
	if err != nil {
		return hozoc.EmitResult{}, err
	}

	result := hozoc.EmitResult{Warnings: toDiagnostics(pl.warnings, p.Files), Written: []hozoc.Path{}, Removed: []hozoc.Path{}}
	for _, o := range pl.outputs {
		result.Written = append(result.Written, hozoc.Path(o.path))
	}

	// 4. dry-run
	if in.DryRun != nil && *in.DryRun {
		return result, nil
	}

	// 5. 前回の出力のうち今回書かないものを消す
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
			// 記録のパスに従うのは、その記録の持ち主の Dir(U, N) の下のものだけ（docs/cli.md「emit」の 5）。
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
			removeEmptyDirs(p.Root, dir, prev)
		}
	}
	sort.Slice(result.Removed, func(i, j int) bool { return result.Removed[i] < result.Removed[j] })

	// 6. 書く（内容が同じなら書き直さない）
	for _, o := range pl.outputs {
		if err := writeIfChanged(filepath.Join(p.Root, filepath.FromSlash(o.path)), o.content); err != nil {
			return result, fmt.Errorf("hozoc: %s を書けない: %w", o.path, err)
		}
	}

	// 7. .hozoc-outputs.json
	for _, dir := range pl.dirs {
		record := filepath.Join(p.Root, filepath.FromSlash(dir), OutputsFile)
		data := pl.record(dir)
		if data == nil {
			// 何も書かない出力（どのトランスパイラもファイルを返さなかった、in で出力でなくなった）には記録も置かない
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
// 設定から生成し直したものと一致しているか。生成したコードでは、状態の分類 OutputsSync の単位の問い合わせになり、判定関数
// hozoc.ClassifyOutputsSync が呼ぶ。
//
// すべての出力について書き出す予定（[App.plan]。トランスパイラも起動する）を作り、ディスクと比べる。一致していると言うのは、
// 書く予定のファイルがすべて同じ内容であり、.hozoc-outputs.json が今回の一覧と同じで（何も書かない出力にはなく）、
// 前回書いて今回は書かないファイルが残っていないとき。emit を走らせても何も書かず何も消さない、ということ。
// 仕様や設定に誤りがあって生成し直せなければ、その例外を返す（check / emit と同じもの）。
//
// 見るのは、直前の Check が解析した Project（同じ App で check か emit を走らせた後）。なければ、作業ディレクトリから
// hozo.toml を探して解析する。
func (s outputsSync) UpToDate(ctx context.Context) (bool, error) {
	a := s.a
	if err := ctx.Err(); err != nil {
		return false, err
	}
	p := a.checked
	if p == nil {
		var err error
		if p, err = a.load(); err != nil {
			return false, err
		}
	}
	if p.NoConfig {
		return false, noConfig(p)
	}
	pl, err := a.plan(ctx, p, nil)
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

// plan は一度の emit が書き出す予定のすべて（docs/cli.md「emit」の 1〜3 の結果）。
type plan struct {
	// dirs は対象の出力の Dir(U, N)（ルートからの相対パス。単位の設定の順、名前空間の IR の順）。対象の単位の対象の名前空間で、
	// in のせいで出力でなくなったものの Dir(U, N) も含む（前回のファイルを消すため。docs/cli.md「emit」の 5）。
	dirs []string
	// outputs は書く予定のファイル（パスの辞書順。重複はない）。
	outputs []output
	// warnings は check の warning に、トランスパイラの warning を足したもの（並べ替え済み）。
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

// plan は docs/cli.md「emit」の 1〜3: 対象を決め、対象の出力ごとに単位のトランスパイラを起動して files と warnings と errors を
// 集め、パスの重複を確かめる。何も書かない。誤りは cli.hozo の例外で返す（UnknownNamespace、proc.Crashed /
// proc.MalformedOutput、ConfigInvalid、CheckFailed、OutputConflict）。
//
// hozoc 本体はコードを出力しない（docs/plugin-protocol.md「この文書が決めること」）。どのトランスパイラも、自分の出力の View だけ
// から応答を作る（同「起動」）ので、対象でない出力のトランスパイラは起動しない。対象を絞っても、書き出す内容は同じになる。
//
// トランスパイラの files の path はルートからの相対パスで、その起動の Dir(U, N) の下でなければならない
// （docs/plugin-protocol.md「emit」）。そうでないもの（`..`、絶対パス、別のディレクトリ、別の出力の Dir、Dir(U, N) そのもの）は
// PLG-1、同じ応答の中の重複は PLG-2 で、どちらも proc.MalformedOutput。
func (a *App) plan(ctx context.Context, p *project.Project, namespaces []string) (*plan, error) {
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
	// otherDirs は、出力 k の Dir(U, N) の下にある、別の出力の Dir（同じ単位の子孫の名前空間のもの）。そこは k のトランスパイラの
	// 置き場ではない（PLG-1 の「別の出力の Dir」）。
	otherDirs := func(k outputKey) []string {
		dir := p.Config.Dir(k.unit, k.ns)
		var out []string
		for ns := range views[k.unit] {
			if d := p.Config.Dir(k.unit, ns); ns != k.ns && config.Under(d, dir) {
				out = append(out, d)
			}
		}
		return out
	}

	// warnings は check の warning（p.Diags）から始める（EmitResult.warnings は check の warning と、トランスパイラの warnings）。
	warnings := append([]diag.Diagnostic{}, p.Diags...)
	var failures []diag.Diagnostic
	var outputs []output

	// 2. トランスパイラ。順序は、単位の設定の順、名前空間の IR の順、transpilers に書いた順（docs/plugin-protocol.md「起動」）。
	// errors があっても、残りのトランスパイラを起動して、すべて集める。
	for _, k := range targets {
		view := views[k.unit][k.ns]
		dir := p.Config.Dir(k.unit, k.ns)
		nested := otherDirs(k)
		for _, t := range p.Units[k.unit].Transpilers {
			if err := ctx.Err(); err != nil {
				return nil, err // 中断されたら、残りのトランスパイラを起動しない
			}
			resp, err := a.transpile(p, t, view)
			if err != nil {
				var pe *hostplugin.PluginError
				if errors.As(err, &pe) {
					d := pe.Diagnostic(p.ManifestPath(t.Plugin))
					return nil, hozoc.ConfigInvalid{Value: diagnostics(append(append([]diag.Diagnostic{}, warnings...), d), p.Files)}
				}
				return nil, err
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
				for _, other := range nested {
					ok = ok && rel != other && !config.Under(rel, other)
				}
				if !ok {
					return nil, asProc(&hostplugin.MalformedOutputError{Plugin: t.Plugin.Name,
						Reason: fmt.Sprintf("PLG-1: files の path %q が、%s の出力先 %s の下にない", f.Path, where(k), dir)})
				}
				if seen[rel] {
					return nil, asProc(&hostplugin.MalformedOutputError{Plugin: t.Plugin.Name, Reason: fmt.Sprintf("PLG-2: files の path %q が二度ある", f.Path)})
				}
				seen[rel] = true
				outputs = append(outputs, output{path: rel, content: []byte(f.Content), source: "トランスパイラ " + t.Name, where: where(k), dir: dir})
			}
		}
	}
	if len(failures) > 0 {
		all := append(append([]diag.Diagnostic{}, failures...), warnings...)
		diag.Sort(all, p.Files, nil)
		return nil, hozoc.CheckFailed{Value: diagnostics(all, p.Files)}
	}
	diag.Sort(warnings, p.Files, nil)
	sort.SliceStable(outputs, func(i, j int) bool { return outputs[i].path < outputs[j].path })

	// 3. CFG-5
	if conflicts := findConflicts(outputs); len(conflicts) > 0 {
		all := append(conflicts, warnings...)
		return nil, hozoc.OutputConflict{Value: diagnostics(all, p.Files)}
	}
	pl.outputs, pl.warnings = outputs, warnings
	return pl, nil
}

// transpile は、トランスパイラ t を一度起動して、出力一つ（view）を展開させる（docs/plugin-protocol.md「起動」）。cli.hozo の op
// runEmit（[App.RunEmit]）と同じ起動を、本物の View（ir.View）を持つ要求で、internal/plugin.Runner を直に使って行う
// （docs/cli.md「手で書くもの」: cli.hozo の model View は中身を持たないので、runEmit の入口からは IR を渡せない）。
// 作業ディレクトリはプロジェクトのルート、要求の transpiler はトランスパイラの名前、view は単位での名前空間から見た IR。
// 誤りは proc.Crashed / proc.MalformedOutput（asProc）か、終了コード 2 の *hostplugin.PluginError（呼ぶ側が MAN-* にする）。
func (a *App) transpile(p *project.Project, t project.Transpiler, view *ir.View) (*sdk.EmitResponse, error) {
	req := sdk.EmitRequest{Protocol: hostplugin.Protocol, Direction: "emit", Transpiler: t.Name, View: view}
	var resp sdk.EmitResponse
	if err := (hostplugin.Runner{Root: p.Root, Stderr: a.Stderr, Env: a.Env}).Run(t.Plugin.Manifest, t.Plugin.Dir, &req, &resp); err != nil {
		return nil, asProc(err)
	}
	return &resp, nil
}

// RunEmit は cli.hozo の op runEmit: プラグインを一つ起動して、トランスパイラ一つに展開させる（docs/plugin-protocol.md「起動」）。
// hozoc.Leaves のメソッド。どの配線からも呼ばれない内部の op で、プラグインとの境界の記述としてある（docs/cli.md
// 「手で書くもの」）: plugin を起動し（作業ディレクトリは plugin.root、マニフェストのディレクトリは plugin.dir）、req を JSON にして
// 送り、応答を hozoc.EmitResponse に読んで返す。hozoc.View は中身のない model なので、この入口から送る要求の view は空の
// オブジェクトになる。hozoc 自身の emit は、この入口を呼ばず、[App.transpile] を使う。
// 起動の失敗と異常終了は proc.Crashed、応答の形の誤りは proc.MalformedOutput。
func (a *App) RunEmit(ctx context.Context, plugin hozoc.Plugin, req hozoc.EmitRequest) (hozoc.EmitResponse, error) {
	if err := ctx.Err(); err != nil {
		return hozoc.EmitResponse{}, err
	}
	if len(plugin.Command) == 0 {
		return hozoc.EmitResponse{}, asProc(&hostplugin.CrashedError{Plugin: plugin.Name, ExitCode: -1, Err: errors.New("command が空")})
	}
	manifest := &hostplugin.Manifest{Name: plugin.Name, Command: plugin.Command, Protocol: req.Protocol}
	var resp sdk.EmitResponse
	if err := (hostplugin.Runner{Root: string(plugin.Root), Stderr: a.Stderr, Env: a.Env}).Run(manifest, string(plugin.Dir), &req, &resp); err != nil {
		return hozoc.EmitResponse{}, asProc(err)
	}
	out := hozoc.EmitResponse{Files: []hozoc.OutputFile{}, Warnings: []hozoc.PluginWarning{}, Errors: []hozoc.PluginError{}}
	for _, f := range resp.Files {
		out.Files = append(out.Files, hozoc.OutputFile{Path: hozoc.Path(f.Path), Content: f.Content})
	}
	for _, w := range resp.Warnings {
		out.Warnings = append(out.Warnings, hozoc.PluginWarning{At: w.At, Message: w.Message})
	}
	for _, e := range resp.Errors {
		out.Errors = append(out.Errors, hozoc.PluginError{Code: e.Code, At: e.At, Message: e.Message})
	}
	return out, nil
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
// dir の下かどうかは確かめない（従うかどうかは呼ぶ側が決める。WriteOutputs の 5）。
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

// removeEmptyDirs は、消したファイル removed（ルートからの相対パス）のディレクトリが空になっていれば、出力先 dir の下の範囲で、
// そのディレクトリも消す（docs/cli.md「emit」の 5: Go の基底の内側のフォルダ inner が要らなくなったとき）。親へ向かって、
// 空でないディレクトリか dir そのものに当たるまで続ける。dir そのものは消さない。
func removeEmptyDirs(root, dir, removed string) {
	for d := path.Dir(removed); config.Under(d, dir); d = path.Dir(d) {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(d))); err != nil {
			return // 空でない（か、消せない）。そこで止める
		}
	}
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
