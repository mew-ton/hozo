// Package gocli は go-cli プラグインの中身（docs/plugin-go-cli.md）。
//
// Handler が plugin.Handler を実装し、View の見る側の名前空間の @cli.command の op を
// cli/cli.go（パッケージ cli）にする。「対象」の CLI-1〜CLI-5 は emit の応答の errors。
package gocli

import (
	"errors"
	"fmt"
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// OutputPath は生成するファイルの、View.config.out からの相対パス（docs/plugin-go-cli.md「出力」）。
const OutputPath = "cli/cli.go"

// Handler は go-cli の plugin.Handler。emit だけを実装する。
type Handler struct {
	// Getenv は環境変数を引く関数。nil なら os.Getenv。HOZO_PROJECT_ROOT（docs/plugin-protocol.md「起動」）を
	// 名前空間のパッケージの import パスを求めるルートに使う。空なら作業ディレクトリ（= プロジェクトのルート）。
	Getenv func(string) string
}

var _ plugin.Handler = Handler{}

// Emit は View.viewer の名前空間の CLI の骨組みを cli/cli.go にする。
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
	importPath, err := golang.ImportPath(root, view.Config.Out)
	if err != nil {
		if errors.Is(err, golang.ErrNoGoMod) {
			return &plugin.EmitResponse{Errors: []plugin.Diagnostic{{
				Code: "GOEMIT-1", At: view.Viewer,
				Message: fmt.Sprintf("out %q の上に go.mod がない（%v）", view.Config.Out, err),
			}}}, nil
		}
		return nil, fmt.Errorf("emit: import path of %s: %w", view.Config.Out, err)
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
	return &plugin.EmitResponse{Files: []plugin.File{{Path: OutputPath, Content: string(src)}}}, nil
}

// Check は実装しない（語彙に固有の検査はない）。
func (Handler) Check(*plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}
