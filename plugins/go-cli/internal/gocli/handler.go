// Package gocli は、コマンドラインのトランスパイラ go-cli の中身（docs/plugin-go-cli.md）。
//
// Handler が plugin.Handler を実装し、View（出力ごとに一つ。見る側は出力する名前空間 N）から、`<Dir(U, N)>/cli/cli.go`
// （パッケージ cli）を作る: `@cli.command` の付いた op ごとにサブコマンドを一つ持つ Main。N のパッケージの置き場と名前は公開の SDK
// plugin/golang で、どの型が `Validate` を持つかは plugin/govalid で求める（基底と go-valid の出力は見ない。
// docs/plugin-protocol.md「相乗りが基底の形を知る方法」）。語彙の使い方の誤りは、診断（CLI-*）で返す。
package gocli

import (
	"fmt"
	"os"

	"github.com/mew-ton/hozo/plugin"
)

// TranspilerName は、このプラグインが提供するただ一つのトランスパイラの名前（docs/plugin-go-cli.md「マニフェスト」）。
const TranspilerName = "go-cli"

// Handler は go-cli の plugin.Handler。emit だけを実装する。
type Handler struct {
	// Getenv は環境変数を引く関数。nil なら os.Getenv。HOZO_PROJECT_ROOT（docs/plugin-protocol.md「起動」）を、import パスを
	// 求めるルートに使う。空なら作業ディレクトリ（= プロジェクトのルート）。
	Getenv func(string) string
}

var _ plugin.Handler = Handler{}

// Emit は View.viewer の名前空間の cli/cli.go を作る（docs/plugin-go-cli.md「出力」）。診断（CLI-*）があれば Files は空で、
// Errors にそれを入れる。`@cli.command` の op が一つもなければ、ファイルを出さず warning を一つ返す。
// 要求の transpiler が [TranspilerName] でなければ plugin.ErrUnsupported（code "unsupported"）。
func (h Handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req == nil || req.View == nil {
		return nil, fmt.Errorf("emit: request has no view")
	}
	if req.Transpiler != TranspilerName {
		return nil, fmt.Errorf("transpiler %q is not provided by go-cli (it provides %q): %w", req.Transpiler, TranspilerName, plugin.ErrUnsupported)
	}
	getenv := h.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	root := getenv("HOZO_PROJECT_ROOT")
	if root == "" {
		root = "."
	}
	out, err := Emit(req, root)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}
	if len(out.Errors) > 0 {
		return &plugin.EmitResponse{Errors: out.Errors}, nil
	}
	return &plugin.EmitResponse{Files: out.Files, Warnings: out.Warnings}, nil
}

// Check は実装しない（語彙の使い方の誤りは、emit の応答の errors で返す）。
func (Handler) Check(*plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}
