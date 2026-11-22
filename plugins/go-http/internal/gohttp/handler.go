// Package gohttp は、HTTP のサーバーのトランスパイラ go-http の中身（docs/plugin-go-http.md）。
//
// Handler が plugin.Handler を実装し、View（出力ごとに一つ。見る側は出力する名前空間 N）から、N の入口のパッケージの http.go を
// 作る: `@http.get` / `@http.post` の付いた op ごとに経路を一つ持つ、net/http のハンドラ。置き場と名前は公開の SDK
// plugin/golang で求める（基底と go-valid の出力は見ない。docs/plugin-protocol.md「相乗りが基底の形を知る方法」）。
//
// 経路ごと、フィールドごとに出すコードは、デコレータの引数だけから作り、どの型でも同じ補助の呼び出しにする
// （docs/requirements.md「出力を推論しない」）: 値で変わる振る舞いは、http.go の補助（helpers.go）が、実行時に決める。
// 型を見るのは、書いたものの整合の検査（emit.go、inputs.go）だけで、食い違いは、診断（GOHTTP-*）で返す。どの型が `Validate` を
// 持つか（GOHTTP-11）と、補う値が制約に合うか（GOHTTP-19）は、plugin/govalid で求める。
package gohttp

import (
	"fmt"

	"github.com/mew-ton/hozo/plugin"
)

// TranspilerName は、このプラグインが提供するただ一つのトランスパイラの名前（docs/plugin-go-http.md「マニフェスト」）。
const TranspilerName = "go-http"

// Handler は go-http の plugin.Handler。emit と、emit と同じ検査で答える check を実装する。
type Handler struct {
}

var _ plugin.Handler = Handler{}

// Emit は View.viewer の名前空間の http.go を作る（docs/plugin-go-http.md「出力」）。診断（GOHTTP-*）があれば Files は空で、
// Errors にそれを入れる。経路の op が一つもなければ、ファイルを出さず warning を一つ返す。
// 要求の transpiler が [TranspilerName] でなければ plugin.ErrUnsupported（code "unsupported"）。
func (h Handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req == nil || req.View == nil {
		return nil, fmt.Errorf("emit: request has no view")
	}
	if req.Transpiler != TranspilerName {
		return nil, fmt.Errorf("transpiler %q is not provided by go-http (it provides %q): %w", req.Transpiler, TranspilerName, plugin.ErrUnsupported)
	}
	out, err := Emit(req)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}
	if len(out.Errors) > 0 {
		return &plugin.EmitResponse{Errors: out.Errors}, nil
	}
	return &plugin.EmitResponse{Files: out.Files, Warnings: out.Warnings}, nil
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Coverage は実装しない（カバレッジを読むのは、基底）。
func (Handler) Coverage(*plugin.CoverageRequest) (*plugin.CoverageResponse, error) {
	return nil, plugin.ErrUnsupported
}
