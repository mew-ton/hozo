// Package gocli は go-cli プラグインの中身（docs/plugin-go-cli.md）。
//
// Handler が plugin.Handler を実装し、View の見る側の名前空間の @cli.command の op を
// cli/cli.go（パッケージ cli）にする。「対象」の CLI-1〜CLI-5 は emit の応答の errors。
// 既定の出力の対象は応答の select で選ぶ（docs/plugin-go-cli.md「マニフェスト」の「選ぶもの」）。
package gocli

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// TranspilerName は、このプラグインが提供するただ一つのトランスパイラの名前（docs/plugin-go-cli.md「マニフェスト」）。
// hozo.toml の単位の transpilers に書く名前で、emit の要求の transpiler に入ってくる。
const TranspilerName = "go-cli"

// OutputPath は生成するファイルの、View.config.dir（単位での Dir(U, N)。docs/config.md「単位の対象」）からの
// 相対パス（docs/plugin-go-cli.md「出力」）。応答に書くパスは [OutputPathFor]。
const OutputPath = "cli/cli.go"

// OutputPathFor は view の名前空間の cli.go の、プロジェクトのルートからの相対パス（`/` 区切り）: `<View.config.dir>/cli/cli.go`。
func OutputPathFor(view *ir.View) string {
	return path.Join(dirOf(view), OutputPath)
}

// dirOf は View.config.dir（Dir(U, N)）。空なら View.config.out（単位の namespace 自身の Dir と同じ。hozoc の View では常に dir がある）。
func dirOf(view *ir.View) string {
	d := view.Config.Dir
	if d == "" {
		d = view.Config.Out
	}
	return path.Clean(filepath.ToSlash(d))
}

// Handler は go-cli の plugin.Handler。emit だけを実装する。
type Handler struct {
	// Getenv は環境変数を引く関数。nil なら os.Getenv。HOZO_PROJECT_ROOT（docs/plugin-protocol.md「起動」）を
	// 名前空間のパッケージの import パスを求めるルートに使う。空なら作業ディレクトリ（= プロジェクトのルート）。
	Getenv func(string) string
}

var _ plugin.Handler = Handler{}

// Emit は View.viewer の名前空間の CLI の骨組みを `<View.config.dir>/cli/cli.go` にする（docs/plugin-go-cli.md「出力」）。
// 見るのは単位の対象の op だけ（docs/config.md「in」）。@cli.command の op が一つもなければファイルを出さず warning を一つ返す。
// CLI-* があれば Files は空で Errors にそれを入れる。go.mod が見つからなければ GOEMIT-1（docs/emit-go.md「診断」）。
// 要求の transpiler が [TranspilerName] でなければ plugin.ErrUnsupported（code "unsupported"）。
//
// どの応答も select を持つ: roots は @cli.command の付いた op、stops は @proc.stdio の付いた op
// （プロセスの境界を越える op は辿らず、口だけを出す）。cli.go は、roots の op が既定の出力の Ops に載ることを前提に書く。
func (h Handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req == nil || req.View == nil {
		return nil, fmt.Errorf("emit: request has no view")
	}
	if req.Transpiler != TranspilerName {
		return nil, fmt.Errorf("transpiler %q is not provided by go-cli (it provides %q): %w", req.Transpiler, TranspilerName, plugin.ErrUnsupported)
	}
	view := req.View
	sel := Select(view)
	getenv := h.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	root := getenv("HOZO_PROJECT_ROOT")
	if root == "" {
		root = "."
	}
	dir := dirOf(view)
	importPath, err := golang.ImportPath(root, dir)
	if err != nil {
		if errors.Is(err, golang.ErrNoGoMod) {
			return &plugin.EmitResponse{Select: sel, Errors: []plugin.Diagnostic{{
				Code: "GOEMIT-1", At: view.Viewer,
				Message: fmt.Sprintf("名前空間 %s の出力先 %q の上に go.mod がない（%v）", view.Viewer, dir, err),
			}}}, nil
		}
		return nil, fmt.Errorf("emit: import path of %s: %w", dir, err)
	}

	prog, err := analyze(view, importPath)
	if err != nil {
		var ae *analyzeError
		if errors.As(err, &ae) && ae.Err == nil {
			return &plugin.EmitResponse{Select: sel, Errors: ae.Errors}, nil
		}
		return nil, fmt.Errorf("emit: %w", err)
	}
	if len(prog.Commands) == 0 {
		return &plugin.EmitResponse{Select: sel, Warnings: []plugin.Warning{{
			At: view.Viewer, Message: fmt.Sprintf("名前空間 %s に @cli.command の op がないので cli.go を生成しない", view.Viewer),
		}}}, nil
	}
	src, err := generate(prog)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}
	return &plugin.EmitResponse{Select: sel, Files: []plugin.File{{Path: OutputPathFor(view), Content: string(src)}}}, nil
}

// Select は go-cli が選ぶ既定の出力の対象（docs/plugin-go-cli.md「マニフェスト」の「選ぶもの」）:
// roots は @cli.command の付いた op、stops は @proc.stdio の付いた op。どちらも単位の対象の op だけ（plugin.OpsWith）。
// states と wraps は空。
func Select(view *ir.View) plugin.Select {
	return plugin.Select{
		Roots:  plugin.OpsWith(view, decCommand),
		Stops:  plugin.OpsWith(view, decStdio),
		States: []string{},
		Wraps:  []plugin.Wrap{},
	}
}

// Check は実装しない（語彙に固有の検査はない）。
func (Handler) Check(*plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}
