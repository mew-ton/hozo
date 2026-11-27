package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/gen/hozo/compiler"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/testmap"
	"github.com/mew-ton/hozo/ir"
	sdk "github.com/mew-ton/hozo/plugin"
)

// MatchTests は spec/cli.hozo の op matchTests（docs/cli.md「tests」の 1〜6）: 検査を通った仕様から、テストの対応表を集め、
// カバレッジの「実行された行」と突き合わせる。compiler.Leaves のメソッド。ファイルは書き出さない（置き場のファイルと、
// カバレッジのファイルを、読むだけ）。hozoc は、テストを走らせず、カバレッジの形も知らない: 形を読むのは、単位の基底の
// プラグイン（direction "coverage"。docs/plugin-protocol.md「coverage」）。checked は Check が返したもの（[App.CollectOutputs]
// と同じ扱い）。
func (a *App) MatchTests(ctx context.Context, in compiler.CoverageInput, checked compiler.CheckResult) (compiler.TestsResult, error) {
	p := a.checked
	if p == nil || len(checked.Warnings) != len(p.Diags) {
		return compiler.TestsResult{}, errors.New("hozoc: MatchTests の checked が直前の Check の結果でない")
	}
	if p.NoConfig {
		return compiler.TestsResult{}, noConfig(p)
	}
	// 1. ファイルを集める（書き出さない）
	pl, err := a.collect(ctx, p, nil)
	if err != nil {
		return compiler.TestsResult{}, err
	}
	warnings := toDiagnostics(pl.warnings, p.Files)
	result := compiler.TestsResult{Warnings: warnings, Ran: []string{}}
	// 2. 求めるテスト
	if len(pl.tests) == 0 {
		return result, nil
	}
	// 3. 置き場の出力が、集めたものと同じか（TESTS-2）
	if stale := staleOutputs(pl); len(stale) > 0 {
		return compiler.TestsResult{}, coverageUnusable(append(stale, warnings...))
	}
	// 4. カバレッジを読む（TESTS-3）
	covered, unusable, err := a.readCoverage(ctx, p, pl, string(in.Coverage))
	if err != nil {
		return compiler.TestsResult{}, err
	}
	if len(unusable) > 0 {
		return compiler.TestsResult{}, coverageUnusable(append(unusable, warnings...))
	}
	// 5. 突き合わせる（TESTS-1）。テストは、その単位の基底のプラグインが返した行と比べる
	var missing []compiler.Diagnostic
	for i := range pl.tests {
		t := &pl.tests[i]
		switch testmap.Match([]testmap.Test{t.Test}, covered[basePlugin(p, t.unit).Name])[0] {
		case testmap.Ran:
			result.Ran = append(result.Ran, t.Label())
		case testmap.NotRun:
			missing = append(missing, testsDiagnostic("TESTS-1", t.File, t.Lines[0],
				fmt.Sprintf("求めるテスト %s が、走っていない（カバレッジが、%s の %d〜%d 行目を、実行済みと言っていない）", t.Label(), t.File, t.Lines[0], t.Lines[1])))
		case testmap.Unmeasured:
			missing = append(missing, testsDiagnostic("TESTS-1", t.File, t.Lines[0],
				fmt.Sprintf("求めるテスト %s が、走ったと確かめられない（カバレッジが、ファイル %s を測っていない）", t.Label(), t.File)))
		}
	}
	if len(missing) > 0 {
		return compiler.TestsResult{}, testsNotRun(append(missing, warnings...))
	}
	return result, nil
}

// testsDiagnostic は TESTS-* の診断一つ（error）。位置は、ファイル file（ルートからの相対パスか、カバレッジのファイル）と、
// line が 1 以上なら、その行の頭。仕様のファイルの位置ではないので、境界の形（compiler.Diagnostic）で直に作る。
func testsDiagnostic(code, file string, line int, message string) compiler.Diagnostic {
	d := compiler.Diagnostic{Severity: diag.Error.String(), Code: code, Message: message}
	if file != "" {
		path := compiler.Path(file)
		d.File = &path
	}
	if line >= 1 {
		d.Start = &compiler.Position{Line: int64(line), Col: 1}
	}
	return d
}

// staleOutputs は docs/cli.md「tests」の 3: 対応表と、対応表が指すファイルのうち、置き場にないもの、集めたものと内容が
// 違うものごとに、TESTS-2 を一つ（パスの辞書順）。
func staleOutputs(pl *outputPlan) []compiler.Diagnostic {
	want := map[string]bool{}
	for _, t := range pl.tests {
		want[t.mapPath], want[t.File] = true, true
	}
	var out []compiler.Diagnostic
	for _, o := range pl.outputs { // パスの辞書順
		if !want[o.path] {
			continue
		}
		disk, err := os.ReadFile(filepath.Join(pl.root, filepath.FromSlash(o.path)))
		switch {
		case err != nil:
			out = append(out, testsDiagnostic("TESTS-2", o.path, 0,
				fmt.Sprintf("%s が、置き場にない（hozoc build をやり直して、テストを走らせ直す）", o.path)))
		case !bytes.Equal(disk, o.content):
			out = append(out, testsDiagnostic("TESTS-2", o.path, 0,
				fmt.Sprintf("置き場の %s が、今の仕様から集めたものと違う（hozoc build をやり直して、テストを走らせ直す）", o.path)))
		}
	}
	return out
}

// basePlugin は、単位 unit（Project.Units の添字）の基底のトランスパイラを提供するプラグイン。基底が決まらない単位は、
// 検査（CFG-20 など）で止まっているので、ここには来ない。
func basePlugin(p *project.Project, unit int) *hostplugin.Plugin {
	u := p.Units[unit]
	for _, t := range u.Transpilers {
		if t.Name == u.Base {
			return t.Plugin
		}
	}
	return nil
}

// readCoverage は docs/cli.md「tests」の 4: カバレッジのファイル path（相対パスの基準は作業ディレクトリ）を読み、対応表を
// 持つ単位の基底のプラグインごとに direction "coverage" を送る（docs/plugin-protocol.md「coverage」）。戻り値の covered は、
// プラグインの名前 → 実行された行。unusable は TESTS-3。誤りは、プラグインの異常終了など（compiler.Crashed /
// compiler.MalformedOutput / ConfigInvalid）。
func (a *App) readCoverage(ctx context.Context, p *project.Project, pl *outputPlan, path string) (covered map[string][]sdk.CoveredFile, unusable []compiler.Diagnostic, err error) {
	abs := path
	if !filepath.IsAbs(abs) {
		dir := a.Dir
		if dir == "" {
			if dir, err = os.Getwd(); err != nil {
				return nil, nil, err
			}
		}
		abs = filepath.Join(dir, abs)
	}
	content, readErr := os.ReadFile(abs)
	switch {
	case readErr != nil:
		return nil, []compiler.Diagnostic{testsDiagnostic("TESTS-3", path, 0, fmt.Sprintf("カバレッジのファイル %s を読めない: %v", path, cause(readErr)))}, nil
	case !utf8.Valid(content):
		return nil, []compiler.Diagnostic{testsDiagnostic("TESTS-3", path, 0, fmt.Sprintf("カバレッジのファイル %s が、UTF-8 の文字列でない", path))}, nil
	}

	// 相手: 対応表を持つ単位の基底のプラグイン（単位の設定の順）と、そのプラグインに聞くファイル
	var plugins []*hostplugin.Plugin
	files := map[string]map[string]bool{}
	outputs := map[string][]ir.NamespaceConfig{} // プラグインの名前 → files を出した出力の設定（単位の設定の順、名前空間の IR の順）
	seenOutput := map[string]bool{}
	for u := range p.Units {
		for _, t := range pl.tests {
			if t.unit != u {
				continue
			}
			pg := basePlugin(p, u)
			if files[pg.Name] == nil {
				files[pg.Name] = map[string]bool{}
				plugins = append(plugins, pg)
			}
			files[pg.Name][t.File] = true
			if key := pg.Name + "\x00" + t.config.Dir; !seenOutput[key] {
				seenOutput[key] = true
				outputs[pg.Name] = append(outputs[pg.Name], t.config)
			}
		}
	}
	covered = map[string][]sdk.CoveredFile{}
	for _, pg := range plugins {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if !pg.Manifest.Coverage.Enabled {
			unusable = append(unusable, testsDiagnostic("TESTS-3", path, 0,
				fmt.Sprintf("基底のプラグイン %s は、カバレッジを読めない（マニフェストの [coverage] enabled が true でない）", pg.Name)))
			continue
		}
		req := sdk.CoverageRequest{Protocol: hostplugin.Protocol, Direction: "coverage", Coverage: sdk.File{Path: abs, Content: string(content)}, Outputs: outputs[pg.Name]}
		for f := range files[pg.Name] {
			req.Files = append(req.Files, f)
		}
		sort.Strings(req.Files)
		var resp sdk.CoverageResponse
		if err := (hostplugin.Runner{Stderr: a.Stderr, CacheDir: a.cacheDir}).Run(pg.Name, pg.Manifest.WasmPath(pg.Dir), &req, &resp); err != nil {
			var pe *hostplugin.PluginError
			if errors.As(err, &pe) {
				d := pe.Diagnostic(p.ManifestPath(pg))
				return nil, nil, configInvalid(append(append([]diag.Diagnostic{}, pl.warnings...), d), p.Files)
			}
			return nil, nil, asProc(err)
		}
		if !resp.Recognized {
			unusable = append(unusable, testsDiagnostic("TESTS-3", path, 0,
				fmt.Sprintf("基底のプラグイン %s は、カバレッジのファイル %s を、読める形と認めなかった", pg.Name, path)))
			continue
		}
		for _, f := range resp.Files {
			if files[pg.Name][f.Path] { // 要求の files にないパスの要素は、読まない
				covered[pg.Name] = append(covered[pg.Name], f)
			}
		}
	}
	return covered, unusable, nil
}

// cause は、ファイルの誤り err の、パスを除いた原因（*fs.PathError の Err）。包まれていなければ err。
func cause(err error) error {
	if inner := errors.Unwrap(err); inner != nil {
		return inner
	}
	return err
}
