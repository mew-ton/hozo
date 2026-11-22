package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/testmap"
	"github.com/mew-ton/hozo/ir"
	sdk "github.com/mew-ton/hozo/plugin"
)

// outputKey は出力一つ: 単位（Project.Units の添字）と、その単位が出力する名前空間（docs/cli.md「書き出し」）。
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

// CollectOutputs は cli.hozo の op collectOutputs（docs/cli.md「書き出し」の 1〜3）: 検査（Check）が、プラグインから受け取った答えから、
// 出力ごとのファイルを集める。プラグインは、起動しない。hozoc.Leaves のメソッド。葉の op なので、状態（生成したファイル）には触れない: 読むのも書くのも、
// 配線 writeOutputs が、この後に呼ぶ read outputChanges（[App.Outputs]）と効果 syncOutputs（[App.OutputsSync]）。内部の op で、
// テストは、内側のパッケージ internal/gen/hozoc/inner の Ops から呼ぶ。checked は Check が返したもの（warning）で、その中身
// （Program、設定、マニフェスト）は Check が a.checked に置いた Project から取る。集めるファイルは、どれもトランスパイラが
// 返したもの。
//
//  0. 設定なし（hozo.toml が見つからない。docs/config.md「置き場と探し方」）なら、書き出すものがないので CFG-7 を
//     一つ作って ConfigInvalid（docs/cli.md「書き出し」の 0）。check の warning も一緒に運ぶ。検査だけ（in の noEmit）なら、
//     誤りにせず、空の予定を返す。
//  1. namespaces が空でなければ、名前空間がそのどれかと同じか子孫である出力だけを対象にする。引数とその子孫のどれも、
//     どの単位の対象でもなければ UnknownNamespace。
//  2. 対象の各出力について、単位の各トランスパイラの答えから、files を集める（path とテストの対応表を確かめる:
//     PLG-1 / PLG-2 / PLG-4 は proc.MalformedOutput）。
//  3. すべての出力のファイルを集めてからパスの重複を確かめる（CFG-5 → OutputConflict）。
//
// 1〜3 は [App.collect]。返す予定（hozoc.EmitPlan）は、warning と、ファイル（パスの辞書順）と、入力の dryRun と noEmit。置き場を含む形は
// a.planned に置く（[App.plannedFor]）。
//
// ctx が中断されていれば、その error を返す（[App.collect]）。
func (a *App) CollectOutputs(ctx context.Context, in hozoc.BuildInput, checked hozoc.CheckResult) (hozoc.EmitPlan, error) {
	a.planned = nil
	p := a.checked
	if p == nil || len(checked.Warnings) != len(p.Diags) {
		return hozoc.EmitPlan{}, errors.New("hozoc: CollectOutputs の checked が直前の Check の結果でない")
	}
	// 0. 設定なし。検査だけ（--no-emit）なら、集めるものがないので、空の予定を返す
	if p.NoConfig {
		if in.NoEmit != nil && *in.NoEmit {
			a.planned = &outputPlan{root: p.Root, warnings: p.Diags}
			return hozoc.EmitPlan{Warnings: toDiagnostics(p.Diags, p.Files), Files: []hozoc.OutputFile{}, DryRun: in.DryRun, NoEmit: in.NoEmit}, nil
		}
		return hozoc.EmitPlan{}, noConfig(p)
	}
	pl, err := a.collect(ctx, p, in.Namespaces)
	if err != nil {
		return hozoc.EmitPlan{}, err
	}
	a.planned = pl
	plan := hozoc.EmitPlan{Warnings: toDiagnostics(pl.warnings, p.Files), Files: make([]hozoc.OutputFile, 0, len(pl.outputs)), DryRun: in.DryRun, NoEmit: in.NoEmit}
	for _, o := range pl.outputs {
		plan.Files = append(plan.Files, hozoc.OutputFile{Path: hozoc.Path(o.path), Content: string(o.content)})
	}
	return plan, nil
}

// plannedFor は、境界の予定 plan（hozoc.EmitPlan。warning とファイルだけ）の、置き場を含む形: 直前の CollectOutputs が a.planned に
// 置いたもの。plan が、その CollectOutputs の返したものでなければ（ファイルの並びが違う）、誤り。
func (a *App) plannedFor(plan hozoc.EmitPlan) (*outputPlan, error) {
	pl := a.planned
	same := pl != nil && len(plan.Files) == len(pl.outputs)
	for i := 0; same && i < len(plan.Files); i++ {
		same = string(plan.Files[i].Path) == pl.outputs[i].path
	}
	if !same {
		return nil, errors.New("hozoc: plan が直前の CollectOutputs の結果でない")
	}
	return pl, nil
}

// dryRun は、予定 plan が、書き出さずに、書き出す予定だけを返すものか（cli.hozo の EmitPlan.dryRun）。
func dryRun(plan hozoc.EmitPlan) bool { return plan.DryRun != nil && *plan.DryRun }

// noEmit は、予定 plan が、検査だけのものか（cli.hozo の EmitPlan.noEmit。hozoc build --no-emit）: 書き出さず、書き出す予定も
// 返さない。置き場は、読まない。
func noEmit(plan hozoc.EmitPlan) bool { return plan.NoEmit != nil && *plan.NoEmit }

// noConfig は、設定なしで emit しようとしたときの ConfigInvalid（CFG-7。docs/cli.md「手で書くもの」）。
func noConfig(p *project.Project) error {
	missing := diag.Diagnostic{Severity: diag.Error, Code: "CFG-7", File: config.FileName,
		Message: "hozo.toml が見つからないので、書き出す単位がない（作業ディレクトリから親へ向かって探した。--config か HOZO_CONFIG で指定できる）"}
	return configInvalid(append([]diag.Diagnostic{missing}, p.Diags...), p.Files)
}

// outputPlan は一度の emit が書き出す予定のすべて（docs/cli.md「書き出し」の 1〜3 の結果）。境界の hozoc.EmitPlan が持たない、
// 置き場（ルートと、出力ごとの Dir(U, N)）を持つ。
type outputPlan struct {
	// root はプロジェクトのルート（パスの基準）。
	root string
	// dirs は対象の出力の Dir(U, N)（ルートからの相対パス。単位の設定の順、名前空間の IR の順）。対象の単位の対象の名前空間で、
	// in のせいで出力でなくなったものの Dir(U, N) も含む（前回のファイルを消すため。docs/cli.md「書き出し」の 5）。
	dirs []string
	// outputs は書く予定のファイル（パスの辞書順。重複はない）。
	outputs []output
	// warnings は check の warning に、トランスパイラの warning を足したもの（並べ替え済み）。
	warnings []diag.Diagnostic
	// tests は求めるテスト: 集めたファイルのうち、テストの対応表（docs/plugin-protocol.md「テストの対応表」）のテスト
	// （出力の順、トランスパイラの順、対応表の中の順）。
	tests []plannedTest
}

// plannedTest は、求めるテスト一つと、その出どころ。
type plannedTest struct {
	testmap.Test
	// mapPath は、そのテストを持つ対応表のパス（ルートからの相対パス）。
	mapPath string
	// unit は、その対応表を出した単位（Project.Units の添字）。カバレッジを読む相手（単位の基底のプラグイン）を決める。
	unit int
	// config は、その対応表を出した出力の設定（View の config）。カバレッジの要求の outputs に載せる
	// （docs/plugin-protocol.md「coverage」）。
	config ir.NamespaceConfig
}

// record は出力先 dir の .hozoc-outputs.json の中身（今回書くパスの一覧。辞書順の JSON の配列）。
// 書くものが一つもなければ nil（記録を置かない）。
func (pl *outputPlan) record(dir string) []byte {
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

// collect は docs/cli.md「書き出し」の 1〜3: 対象を決め、対象の出力ごとに、単位のトランスパイラの答え（p.Answers）から files を
// 集め、パスの重複を確かめる。ファイルには触れない。誤りは cli.hozo の例外で返す（UnknownNamespace、proc.MalformedOutput、
// OutputConflict）。
//
// hozoc 本体はコードを出力しない（docs/plugin-protocol.md「この文書が決めること」）。どのトランスパイラも、一つの出力の答えを、
// その出力の View だけから作る（同「起動」）ので、対象を絞っても、書き出す内容は同じになる。プラグインは、ここでは起動しない:
// 答えは、Check が受け取ったもの（p.Answers）で、対象でない出力の答えは、使わないだけ。
//
// トランスパイラの files の path はルートからの相対パスで、その出力の Dir(U, N) の下でなければならない
// （docs/plugin-protocol.md「emit」）。そうでないもの（`..`、絶対パス、別のディレクトリ、別の出力の Dir、Dir(U, N) そのもの）は
// PLG-1、同じ応答の中の重複は PLG-2 で、どちらも proc.MalformedOutput。
func (a *App) collect(ctx context.Context, p *project.Project, namespaces []string) (*outputPlan, error) {
	views := p.Views()

	// 1. 対象の出力。引数 n が選ぶのは、仕様にある名前空間のうち n 自身と n の子孫で、どれかの単位の対象のもの
	// （その名前空間を対象にするすべての単位の分）。n 自身がどの単位の対象でなくても、子孫を対象にする単位があればよい。
	// 一つも選ばない引数（仕様にない名前、どの単位の対象でもない名前空間）があれば UnknownNamespace。
	pl := &outputPlan{root: p.Root}
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

	// warnings は check の warning（p.Diags）: トランスパイラの warning（PLUGIN）も、Check が、もう足している
	// （docs/cli.md「検査」の 4）。
	warnings := append([]diag.Diagnostic{}, p.Diags...)
	var outputs []output

	// 2. トランスパイラの答え。Check が、プラグインから受け取ったもの（p.Answers）を使い、ここでは、起動しない
	// （docs/plugin-protocol.md「起動」: 一度の実行で、同じプラグインに、同じことを、二度、頼まない）。順序は、単位の設定の順、
	// 名前空間の IR の順、transpilers に書いた順。errors は、ここには、ない（一つでもあれば、Check が CheckFailed で終わっている）。
	for _, k := range targets {
		view := views[k.unit][k.ns]
		dir := p.Config.Dir(k.unit, k.ns)
		nested := otherDirs(k)
		for _, t := range p.Units[k.unit].Transpilers {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			resp := p.Answer(k.unit, k.ns, t.Name)
			if resp == nil {
				return nil, fmt.Errorf("hozoc: %s の、トランスパイラ %s の答えが、ない（直前の Check が、プラグインを起動していない）", where(k), t.Name)
			}
			if len(resp.Errors) > 0 {
				return nil, fmt.Errorf("hozoc: %s の、トランスパイラ %s の答えに errors がある（Check が、CheckFailed にしているはず）", where(k), t.Name)
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
			// テストの対応表（docs/plugin-protocol.md「テストの対応表」）: 形を確かめて、求めるテストに足す。違えば PLG-4
			for _, f := range resp.Files {
				rel, _ := pluginPath(f.Path, dir)
				if !testmap.IsMap(rel) {
					continue
				}
				// file は、応答の files の path の一つ: 書かれたままのパスでも、畳んだパスでもよい
				paths := map[string]bool{}
				for _, g := range resp.Files {
					paths[g.Path] = true
				}
				for r := range seen {
					paths[r] = true
				}
				m, err := testmap.Parse(sdk.File{Path: rel, Content: f.Content}, t.Name, paths)
				if err != nil {
					return nil, asProc(&hostplugin.MalformedOutputError{Plugin: t.Plugin.Name,
						Reason: fmt.Sprintf("PLG-4: files の、テストの対応表 %q が、約束の形でない: %v", f.Path, err)})
				}
				for _, e := range m.Tests {
					e.File, _ = pluginPath(e.File, dir)
					pl.tests = append(pl.tests, plannedTest{Test: testmap.Test{Transpiler: t.Name, TestEntry: e}, mapPath: rel, unit: k.unit, config: view.Config})
				}
			}
		}
	}
	sort.SliceStable(outputs, func(i, j int) bool { return outputs[i].path < outputs[j].path })

	// 3. CFG-5
	if conflicts := findConflicts(outputs); len(conflicts) > 0 {
		all := append(conflicts, warnings...)
		return nil, outputConflict(all, p.Files)
	}
	pl.outputs, pl.warnings = outputs, warnings
	return pl, nil
}

// RunEmit は cli.hozo の op runEmit: プラグインを一つ起動して、受け持つ出力のすべてを、まとめて展開させる
// （docs/plugin-protocol.md「起動」）。hozoc.Leaves のメソッド。どの配線からも呼ばれない内部の op で、プラグインとの境界の記述と
// してある（docs/cli.md「手で書くもの」）: plugin を起動し（作業ディレクトリは plugin.root、マニフェストのディレクトリは
// plugin.dir）、req を JSON にして送り、応答を hozoc.EmitResponse に読んで返す。hozoc.Spec と hozoc.OutputView は中身のない
// model なので、この入口から送る要求の spec と、出力の view は空のオブジェクトになる。hozoc 自身の検査（[App.Check]）は、
// この入口を呼ばず、internal/project が、本物の仕様を持つ要求で、同じ起動をする。
// 起動の失敗と異常終了は proc.Crashed、応答の形の誤りは proc.MalformedOutput。
func (a *App) RunEmit(ctx context.Context, plugin hozoc.Plugin, req hozoc.EmitRequest) (hozoc.EmitResponse, error) {
	if err := ctx.Err(); err != nil {
		return hozoc.EmitResponse{}, err
	}
	if len(plugin.Command) == 0 {
		return hozoc.EmitResponse{}, asProc(&hostplugin.CrashedError{Plugin: plugin.Name, ExitCode: -1, Err: errors.New("command が空")})
	}
	manifest := &hostplugin.Manifest{Name: plugin.Name, Command: plugin.Command, Protocol: req.Protocol}
	var resp sdk.EmitBatchResponse
	if err := (hostplugin.Runner{Root: string(plugin.Root), Stderr: a.Stderr, Env: a.Env}).Run(manifest, string(plugin.Dir), &req, &resp); err != nil {
		return hozoc.EmitResponse{}, asProc(err)
	}
	out := hozoc.EmitResponse{Outputs: []hozoc.OutputResult{}}
	for _, o := range resp.Outputs {
		output := hozoc.OutputResult{Results: []hozoc.TranspilerResult{}}
		for _, r := range o.Results {
			result := hozoc.TranspilerResult{Files: []hozoc.OutputFile{}, Warnings: []hozoc.PluginWarning{}, Errors: []hozoc.PluginError{}}
			for _, f := range r.Files {
				result.Files = append(result.Files, hozoc.OutputFile{Path: hozoc.Path(f.Path), Content: f.Content})
			}
			for _, w := range r.Warnings {
				result.Warnings = append(result.Warnings, hozoc.PluginWarning{At: w.At, Message: w.Message})
			}
			for _, e := range r.Errors {
				result.Errors = append(result.Errors, hozoc.PluginError{Code: e.Code, At: e.At, Message: e.Message})
			}
			output.Results = append(output.Results, result)
		}
		out.Outputs = append(out.Outputs, output)
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
