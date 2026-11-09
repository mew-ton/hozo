// Package gocli は go-cli プラグインの中身（docs/plugin-go-cli.md）。
//
// Handler が plugin.Handler を実装し、View の見る側の名前空間の @cli.command の op を
// cli/cli.go（パッケージ cli）にする。「対象」の CLI-1〜CLI-5 は emit の応答の errors。
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

// OutputPath は生成するファイルの、View.config.dir（Dir(N)。docs/config.md「名前空間の設定の解決」）からの
// 相対パス（docs/plugin-go-cli.md「出力」）。応答に書くパスは [OutputPathFor]。
const OutputPath = "cli/cli.go"

// OutputPathFor は view の名前空間の cli.go の、プロジェクトのルートからの相対パス（`/` 区切り）: `<View.config.dir>/cli/cli.go`。
func OutputPathFor(view *ir.View) string {
	return path.Join(dirOf(view), OutputPath)
}

// dirOf は View.config.dir（Dir(N)）。空なら View.config.out（P が空の Dir(N) と同じ。hozoc の View では常に dir がある）。
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
// @cli.command の op が一つもなければファイルを出さず warning を一つ返す。
// CLI-* があれば Files は空で Errors にそれを入れる。go.mod が見つからなければ GOEMIT-1（docs/emit-go.md「診断」）。
func (h Handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req == nil || req.View == nil {
		return nil, fmt.Errorf("emit: request has no view")
	}
	view := req.View
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
			return &plugin.EmitResponse{Errors: []plugin.Diagnostic{{
				Code: "GOEMIT-1", At: view.Viewer,
				Message: fmt.Sprintf("名前空間 %s の out %q の上に go.mod がない（%v）", view.Viewer, view.Config.Out, err),
			}}}, nil
		}
		return nil, fmt.Errorf("emit: import path of %s: %w", dir, err)
	}

	prog, err := analyze(view, importPath)
	if err != nil {
		var ae *analyzeError
		if errors.As(err, &ae) && ae.Err == nil {
			return &plugin.EmitResponse{Errors: ae.Errors}, nil
		}
		return nil, fmt.Errorf("emit: %w", err)
	}
	if len(prog.Commands) == 0 {
		return &plugin.EmitResponse{Warnings: []plugin.Warning{{
			At: view.Viewer, Message: fmt.Sprintf("名前空間 %s に @cli.command の op がないので cli.go を生成しない", view.Viewer),
		}}}, nil
	}
	src, err := generate(prog)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}
	return &plugin.EmitResponse{Files: []plugin.File{{Path: OutputPathFor(view), Content: string(src)}}}, nil
}

// Check は実装しない（語彙に固有の検査はない）。
func (Handler) Check(*plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}
