// Package gobase は Go の基底のプラグイン go の中身（docs/plugin-go.md）。
//
// Handler が plugin.Handler を実装し、View（出力ごとに一つ。見る側は出力する名前空間 N）から、View.config.dir の下の
// ファイルを作る。形（置き場、名前、型、op の置き場と形、継ぎ目）は公開の SDK plugin/golang で求め、ここでは、
// ファイルのテキストを作り、当たる名前（GO-2）や import の循環（GO-3）などの診断（GO-*）を返す。
package gobase

import (
	"fmt"
	"os"

	"github.com/mew-ton/hozo/plugin"
)

// TranspilerName は、このプラグインが提供するただ一つのトランスパイラの名前（docs/plugin-go.md「マニフェスト」）。
const TranspilerName = "go"

// Handler は go の plugin.Handler。emit だけを実装する。
type Handler struct {
	// Getenv は環境変数を引く関数。nil なら os.Getenv。HOZO_PROJECT_ROOT（docs/plugin-protocol.md「起動」）を、import パスを
	// 求めるルートに使う。空なら作業ディレクトリ（= プロジェクトのルート）。
	Getenv func(string) string
	// Stderr は、整形に失敗したとき（GO-INTERNAL）に、整形前の内容を書く先。nil なら os.Stderr。
	Stderr interface{ Write([]byte) (int, error) }
}

var _ plugin.Handler = Handler{}

// Emit は View.viewer の名前空間の出力を作る（docs/plugin-go.md）。診断（GO-*）があれば Files は空で、Errors にそれを入れる。
// 要求の transpiler が [TranspilerName] でなければ plugin.ErrUnsupported（code "unsupported"）。
func (h Handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req == nil || req.View == nil {
		return nil, fmt.Errorf("emit: request has no view")
	}
	if req.Transpiler != TranspilerName {
		return nil, fmt.Errorf("transpiler %q is not provided by go (it provides %q): %w", req.Transpiler, TranspilerName, plugin.ErrUnsupported)
	}
	getenv := h.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	root := getenv("HOZO_PROJECT_ROOT")
	if root == "" {
		root = "."
	}
	stderr := h.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	files, errs := Emit(req, root, stderr)
	if len(errs) > 0 {
		return &plugin.EmitResponse{Errors: errs}, nil
	}
	return &plugin.EmitResponse{Files: files}, nil
}

// Check は実装しない（基底は、語彙に固有の検査を持たない）。
func (Handler) Check(*plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}
