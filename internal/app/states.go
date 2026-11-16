package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// このファイルは、cli.hozo の状態 sources、config、outputs の単位の実装（docs/cli.md「手で書くもの」の表）。どれも、形で決まる
// 問い合わせ（IsEmpty と Get）だけを持つ。生成した配線は呼ばない（cli.hozo の配線は、状態を名前で読まない）: テストが、
// 効果の結果の判定に使うためのもの。値は、作業ディレクトリから探した設定で、ディスクから読む。呼ぶたびに読み直す。

// load は、作業ディレクトリから hozo.toml を探して解析する（Check と同じ解析。入力の config は取らない）。プラグインの check の
// 異常終了と、error の診断は、check と同じ例外で返す。
func (a *App) load() (*project.Project, error) {
	p := project.LoadWith(project.Options{Root: a.Dir, Stderr: a.Stderr, Env: a.Env, PluginPath: a.pluginPath, Home: a.Home})
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
	p := project.LoadWith(project.Options{Root: a.Dir, Stderr: a.Stderr, Env: a.Env, PluginPath: a.pluginPath, Home: a.Home})
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

// Outputs は、状態 outputs（生成したファイル）の単位の実装: 出力先ごとの記録（.hozoc-outputs.json。docs/cli.md「emit」の 5 と 7）に
// 載っているファイル。hozoc.New の引数 outputs に渡す（[App.Ops]）。
func (a *App) Outputs() hozoc.OutputsState { return outputsState{a} }

type outputsState struct{ a *App }

// Get は、すべての単位の対象の名前空間の Dir(U, N) の記録に載っているファイル（ルートからの相対パスと内容）を、単位の設定の順、
// 名前空間の IR の順、記録の順（パスの辞書順）に返す。記録のパスに従うのは、WriteOutputs と同じく、その記録の持ち主の
// Dir(U, N) の下のものだけ。記録にあってもディスクにないファイルは、返さない。設定なしなら、空。
// 置き場（単位と、その対象の名前空間）は仕様から決まるので、仕様や設定に誤りがあれば、その例外を返す（check と同じもの）。
func (s outputsState) Get(ctx context.Context) ([]hozoc.OutputFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := s.a.load()
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
