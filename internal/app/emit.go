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

// WriteOutputs は cli.hozo の op writeOutputs（docs/cli.md「emit」の 1〜8）。checked は Check が返したもの（warning）で、
// その中身（Program、設定、マニフェスト）は Check が l.checked に置いた Project から取る。
//
//  0. 設定なし（hozo.toml が見つからない。docs/config.md「置き場と探し方」）なら、書き出すものがないので CFG-7 を
//     一つ作って ConfigInvalid（docs/cli.md「手で書くもの」）。check の warning も一緒に運ぶ。
//  1. namespaces が空でなければ、その名前空間（と子孫）だけを対象にする。hozo.toml にない名前（設定を解決できない）なら UnknownNamespace。
//  2. 対象の各名前空間について、language が go なら既定の出力（internal/emit/golang）を作る。
//  3. Conf(N).plugins の各プラグイン（[emit] を持つもの）を RunEmit で起動し、files を集める。
//     GOEMIT-* とプラグインの errors は集めておき、すべて呼び終えてから CheckFailed にする。
//  4. すべての名前空間のファイルを集めてからパスの重複を確かめる（CFG-5 → OutputConflict）。
//  5. dryRun なら書く予定のパスを Written に入れて返す。
//  6. 各名前空間の <Dir(N)>/.hozoc-outputs.json を読み、今回書かないパスのファイルを消す。
//  7. 辞書順にファイルを書く。内容が同じなら書き直さない（Written には入れる）。
//  8. .hozoc-outputs.json を書く。
//
// プラグインの files の path はルートからの相対パス（docs/plugin-protocol.md「emit」）。`..` や絶対パスは PLG-1、
// 同じ応答の中の重複は PLG-2 で、どちらも proc.MalformedOutput。
func (l *Leaves) WriteOutputs(ctx context.Context, in hozoc.EmitInput, checked hozoc.CheckResult) (hozoc.EmitResult, error) {
	p := l.checked
	if p == nil || len(checked.Warnings) != len(p.Diags) {
		return hozoc.EmitResult{}, errors.New("hozoc: WriteOutputs の checked が直前の Check の結果でない")
	}
	// 0. 設定なし
	if p.NoConfig {
		missing := diag.Diagnostic{Severity: diag.Error, Code: "CFG-7", File: config.FileName,
			Message: "hozo.toml が見つからないので、書き出す名前空間がない（作業ディレクトリから親へ向かって探した。--config か HOZO_CONFIG で指定できる）"}
		return hozoc.EmitResult{}, hozoc.ConfigInvalid{Value: diagnostics(append([]diag.Diagnostic{missing}, p.Diags...), p.Files)}
	}
	views := p.Views()

	// 1. 対象の名前空間
	targets, err := selectTargets(p, views, in.Namespaces)
	if err != nil {
		return hozoc.EmitResult{}, err
	}

	// 2〜3. 既定の出力とプラグイン。warnings は check の warning（p.Diags。checked.Warnings と同じもの）から始める
	// （EmitResult.warnings は check の warning と、GOEMIT-* の warning、プラグインの warnings のすべて）。
	var outputs []output
	warnings := append([]diag.Diagnostic{}, p.Diags...)
	var failures []diag.Diagnostic
	for _, ns := range targets {
		view := views[ns]
		conf, _ := p.Config.Resolve(ns)
		manifests := p.Plugins(ns)
		if conf.Language == "go" {
			infos := make([]golang.ManifestInfo, 0, len(manifests))
			for _, m := range manifests {
				info := golang.ManifestInfo{Name: m.Name}
				if m.Emit != nil {
					info.Entry, info.Stop, info.Decorate = m.Emit.Entry, m.Emit.Stop, m.Emit.Decorate
				}
				infos = append(infos, info)
			}
			files, ds := golang.Emit(view, golang.Options{Root: p.Root, Manifests: infos})
			warnings, failures = splitDiags(ds, warnings, failures)
			for _, f := range files {
				outputs = append(outputs, output{path: cleanPath(f.Path), content: f.Content, source: "既定の出力", ns: ns})
			}
		}
		for _, m := range manifests {
			if !m.HasEmit() {
				continue
			}
			l.view = view
			resp, err := l.RunEmit(ctx,
				hozoc.Plugin{Name: m.Name, Command: m.Command, Dir: hozoc.Path(p.ManifestDirs[m.Name]), Root: hozoc.Path(p.Root)},
				hozoc.EmitRequest{Protocol: hostplugin.Protocol, Direction: "emit"})
			if err != nil {
				var pe *hostplugin.PluginError
				if errors.As(err, &pe) {
					d := pe.Diagnostic(p.ManifestPath(m.Name))
					return hozoc.EmitResult{}, hozoc.ConfigInvalid{Value: diagnostics(append(append([]diag.Diagnostic{}, warnings...), d), p.Files)}
				}
				return hozoc.EmitResult{}, err
			}
			for _, w := range resp.Warnings {
				warnings = append(warnings, p.PluginWarning(m.Name, sdk.Warning{At: w.At, Message: w.Message}))
			}
			for _, e := range resp.Errors {
				failures = append(failures, p.PluginDiagnostic(m.Name, sdk.Diagnostic{Severity: "error", Code: e.Code, At: e.At, Message: e.Message}))
			}
			seen := map[string]bool{}
			for _, f := range resp.Files {
				rel, ok := pluginPath(string(f.Path))
				if !ok {
					return hozoc.EmitResult{}, asProc(&hostplugin.MalformedOutputError{Plugin: m.Name, Reason: fmt.Sprintf("PLG-1: files の path %q がルートの外に出る", f.Path)})
				}
				if seen[rel] {
					return hozoc.EmitResult{}, asProc(&hostplugin.MalformedOutputError{Plugin: m.Name, Reason: fmt.Sprintf("PLG-2: files の path %q が二度ある", f.Path)})
				}
				seen[rel] = true
				outputs = append(outputs, output{path: rel, content: []byte(f.Content), source: "プラグイン " + m.Name, ns: ns})
			}
		}
	}
	if len(failures) > 0 {
		all := append(append([]diag.Diagnostic{}, failures...), warnings...)
		diag.Sort(all, p.Files, nil)
		return hozoc.EmitResult{}, hozoc.CheckFailed{Value: diagnostics(all, p.Files)}
	}
	diag.Sort(warnings, p.Files, nil)
	sort.SliceStable(outputs, func(i, j int) bool { return outputs[i].path < outputs[j].path })

	// 4. CFG-5
	if conflicts := findConflicts(outputs); len(conflicts) > 0 {
		all := append(conflicts, warnings...)
		return hozoc.EmitResult{}, hozoc.OutputConflict{Value: diagnostics(all, p.Files)}
	}

	result := hozoc.EmitResult{Warnings: toDiagnostics(warnings, p.Files), Written: []hozoc.Path{}, Removed: []hozoc.Path{}}
	for _, o := range outputs {
		result.Written = append(result.Written, hozoc.Path(o.path))
	}

	// 5. dry-run
	if in.DryRun != nil && *in.DryRun {
		return result, nil
	}

	// 6. 前回の出力のうち今回書かないものを消す
	current := map[string]bool{}
	byNS := map[string][]string{}
	for _, o := range outputs {
		current[o.path] = true
		byNS[o.ns] = append(byNS[o.ns], o.path)
	}
	for _, ns := range targets {
		dir := outputsDir(views[ns])
		if dir == "" {
			continue
		}
		previous, err := readOutputsFile(p.Root, dir)
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
	for _, o := range outputs {
		if err := writeIfChanged(filepath.Join(p.Root, filepath.FromSlash(o.path)), o.content); err != nil {
			return result, fmt.Errorf("hozoc: %s を書けない: %w", o.path, err)
		}
	}

	// 8. .hozoc-outputs.json
	for _, ns := range targets {
		dir := outputsDir(views[ns])
		if dir == "" {
			continue
		}
		paths := byNS[ns]
		if paths == nil {
			paths = []string{}
		}
		sort.Strings(paths)
		data, err := json.MarshalIndent(paths, "", "  ")
		if err != nil {
			return result, err
		}
		if err := writeIfChanged(filepath.Join(p.Root, filepath.FromSlash(dir), OutputsFile), append(data, '\n')); err != nil {
			return result, fmt.Errorf("hozoc: %s/%s を書けない: %w", dir, OutputsFile, err)
		}
	}
	return result, nil
}

// RunEmit は cli.hozo の op runEmit: プラグインを一つ起動して展開させる（docs/plugin-protocol.md「起動」）。
// 起動は internal/plugin.Runner で、作業ディレクトリは plugin.Root。要求の view は cli.hozo では中身を書かない
// （model View {}）ので、WriteOutputs が l.view に置いた IR を渡す。
// 誤りは proc.Crashed / proc.MalformedOutput（asProc）か、終了コード 2 の *hostplugin.PluginError（呼ぶ側が MAN-* にする）。
func (l *Leaves) RunEmit(_ context.Context, pl hozoc.Plugin, req hozoc.EmitRequest) (hozoc.EmitResponse, error) {
	m := &hostplugin.Manifest{Name: pl.Name, Command: pl.Command}
	sdkReq := sdk.EmitRequest{Protocol: req.Protocol, Direction: req.Direction, View: l.view}
	var resp sdk.EmitResponse
	if err := (hostplugin.Runner{Root: string(pl.Root), Stderr: l.Stderr, Env: l.Env}).Run(m, string(pl.Dir), &sdkReq, &resp); err != nil {
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
