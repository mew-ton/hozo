package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// このファイルは、cli.hozo の状態 sources、config、outputs の単位の実装（docs/cli.md「手で書くもの」の表）。どれも、形で決まる
// 問い合わせ（IsEmpty と Get）を持つ。生成した配線は、それを呼ばない（cli.hozo の配線は、状態を名前で読まない）: テストが、
// 効果の結果の判定に使うためのもの。値は、作業ディレクトリから探した設定で、ディスクから読む。呼ぶたびに読み直す。
// outputs の単位は、read outputChanges（配線 writeOutputs が呼ぶ）も持つ。

// load は、作業ディレクトリから hozo.toml を探して解析する（Check と同じ解析。入力の config は取らない）。plugins が偽なら、
// プラグインは、起動しない（状態の単位が、仕様と設定を読み直すだけの呼び出し）。真なら、Check と同じに、プラグインに頼み、
// 答えを受け取る（生成し直して比べるとき。[App.collect] が、その答えを使う）。error の診断は、check と同じ例外で返す。
func (a *App) load(plugins bool) (*project.Project, error) {
	p := project.LoadWith(project.Options{Root: a.Dir, Stderr: a.Stderr, CacheDir: a.cacheDir, PluginPath: a.pluginPath, Home: a.Home, NoPlugins: !plugins})
	if p.RunErr != nil {
		return nil, asProc(p.RunErr)
	}
	if err := classify(p.Diags, p.Files); err != nil {
		return nil, err
	}
	return p, nil
}

// Sources は、状態 sources（仕様のファイル。`@fs.files("**/*.hozo")`）の単位の実装: 設定の sources の glob に合うファイル。
// hozoc.New の引数 sources に渡す（[App.Ops]）。
func (a *App) Sources() hozoc.SourcesState { return sourcesState{a} }

type sourcesState struct{ a *App }

// Get は、sources の glob に合うファイル（ルートからの相対パスと内容）を、パスの辞書順に返す。語彙のファイルは含まない。
// 仕様に誤りがあっても、ファイルは返す（読むだけで、検査の結果は見ない）。
func (s sourcesState) Get(ctx context.Context) ([]hozoc.SourceFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a := s.a
	p := project.LoadWith(project.Options{Root: a.Dir, Stderr: a.Stderr, CacheDir: a.cacheDir, PluginPath: a.pluginPath, Home: a.Home, NoPlugins: true})
	if p.RunErr != nil {
		return nil, asProc(p.RunErr)
	}
	content := map[string]string{}
	for i := 0; i < p.Files.Len(); i++ {
		id := source.FileID(i)
		content[p.Files.Path(id)] = string(p.Files.Content(id))
	}
	out := []hozoc.SourceFile{}
	for _, path := range p.Sources {
		out = append(out, hozoc.SourceFile{Path: hozoc.Path(path), Content: content[path]})
	}
	return out, nil
}

// IsEmpty は、sources の glob に合うファイルが一つもなければ true。
func (s sourcesState) IsEmpty(ctx context.Context) (bool, error) {
	files, err := s.Get(ctx)
	return len(files) == 0, err
}

// Config は、状態 config（プロジェクトの設定。`@fs.files("hozo.toml")`）の単位の実装: 作業ディレクトリから親へ向かって探した
// hozo.toml（docs/config.md「置き場と探し方」）。hozoc.New の引数 config に渡す（[App.Ops]）。
func (a *App) Config() hozoc.ConfigState { return configState{a} }

type configState struct{ a *App }

// Get は、hozo.toml（パスは、そのディレクトリ = プロジェクトのルートからの相対パス "hozo.toml"）。見つからなければ
// （設定なし）nil。
func (s configState) Get(ctx context.Context) (*hozoc.SourceFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := s.a.Dir
	if dir == "" {
		dir = "."
	}
	path, ok := config.Find(dir)
	if !ok {
		return nil, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("hozoc: %s を読めない: %w", config.FileName, err)
	}
	return &hozoc.SourceFile{Path: config.FileName, Content: string(content)}, nil
}

// IsEmpty は、hozo.toml が見つからなければ true。
func (s configState) IsEmpty(ctx context.Context) (bool, error) {
	file, err := s.Get(ctx)
	return file == nil, err
}

// Outputs は、状態 outputs（生成したファイル）の単位の実装: 出力先ごとの記録（.hozoc-outputs.json。docs/cli.md「書き出し」の 5 と 7）に
// 載っているファイルと、それに触れる read outputChanges。hozoc.New の引数 outputs に渡す（[App.Ops]）。
func (a *App) Outputs() hozoc.OutputsState { return outputsState{a} }

type outputsState struct{ a *App }

// Get は、すべての単位の対象の名前空間の Dir(U, N) の記録に載っているファイル（ルートからの相対パスと内容）を、単位の設定の順、
// 名前空間の IR の順、記録の順（パスの辞書順）に返す。記録のパスに従うのは、SyncOutputs と同じく、その記録の持ち主の
// Dir(U, N) の下のものだけ。記録にあってもディスクにないファイルは、返さない。設定なしなら、空。
// 置き場（単位と、その対象の名前空間）は仕様から決まるので、仕様や設定に誤りがあれば、その例外を返す（check と同じもの）。
func (s outputsState) Get(ctx context.Context) ([]hozoc.OutputFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := s.a.load(false)
	if err != nil {
		return nil, err
	}
	out := []hozoc.OutputFile{}
	seen := map[string]bool{}
	for u := range p.Units {
		for _, ns := range p.Covered(u) {
			dir := p.Config.Dir(u, ns)
			paths, err := readOutputsFile(p.Root, dir)
			if err != nil {
				return nil, err
			}
			for _, path := range paths {
				if seen[path] || !config.Under(path, dir) {
					continue
				}
				content, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(path)))
				if err != nil {
					if os.IsNotExist(err) {
						continue
					}
					return nil, fmt.Errorf("hozoc: %s を読めない: %w", path, err)
				}
				seen[path] = true
				out = append(out, hozoc.OutputFile{Path: hozoc.Path(path), Content: string(content)})
			}
		}
	}
	return out, nil
}

// IsEmpty は、記録に載っているファイルが一つもなければ true。
func (s outputsState) IsEmpty(ctx context.Context) (bool, error) {
	files, err := s.Get(ctx)
	return len(files) == 0, err
}

// OutputChanges は cli.hozo の read outputChanges（on outputs。docs/cli.md「書き出し」の 4〜7 の、書くファイルと消すファイルを決める
// ところ）: 書き出す予定 plan を、今の生成したファイルと比べる。読むだけで、何も変えない（書き出すのは、効果 syncOutputs。
// [outputsSync.SyncOutputs]）。配線 writeOutputs が、collectOutputs の後、syncOutputs の前に呼び、その結果を返す。plan は、
// 直前の CollectOutputs が返したもの（置き場は [App.plannedFor] から取る）。
//
//   - Warnings は、予定の warning（check の warning に、トランスパイラの warning を足したもの）。
//   - Written は、書く予定のファイル（パスの辞書順）。内容が同じで書き直さないものも入れる（同 6）。
//   - Removed は、消すファイル: 対象の各出力の記録にあって、今回は書かないパスのうち、ディスクにあるもの（[outputPlan.stale]）。
//     パスの辞書順。plan が dryRun なら、決めずに空（同 4: 書く予定のパスだけを返す）。
//   - plan が noEmit（hozoc build --no-emit）なら、Written も Removed も空（同 4）。
func (s outputsState) OutputChanges(ctx context.Context, plan hozoc.EmitPlan) (hozoc.BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return hozoc.BuildResult{}, err
	}
	pl, err := s.a.plannedFor(plan)
	if err != nil {
		return hozoc.BuildResult{}, err
	}
	result := hozoc.BuildResult{Warnings: plan.Warnings, Written: make([]hozoc.Path, 0, len(plan.Files)), Removed: []hozoc.Path{}}
	if noEmit(plan) {
		return result, nil // 検査だけ: 書くものも、消すものも、ない（置き場は、読まない）
	}
	for _, f := range plan.Files {
		result.Written = append(result.Written, f.Path)
	}
	if dryRun(plan) {
		return result, nil
	}
	stale, err := pl.stale()
	if err != nil {
		return hozoc.BuildResult{}, err
	}
	for _, f := range stale {
		result.Removed = append(result.Removed, hozoc.Path(f.path))
	}
	sort.Slice(result.Removed, func(i, j int) bool { return result.Removed[i] < result.Removed[j] })
	return result, nil
}
