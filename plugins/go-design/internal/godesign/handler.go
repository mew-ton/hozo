// Package godesign は、設計の図の語彙からテストを受け取る口を出すトランスパイラ go-design の中身（docs/plugin-go-design.md）。
//
// Handler が plugin.Handler を実装し、View（出力ごとに一つ。見る側は出力する名前空間 N）から、N のテストのパッケージの design.go を
// 作る: `@business.flow` の付いた op（フロー）ごとの、E2E テストのケースを受け取る口と、オプション requirements に並べた分類の
// ケース（要求）ごとの、テストの入れ物を受け取る口と、それぞれを走らせる関数。置き場と名前は公開の SDK plugin/golang で求める
// （基底の出力は見ない。docs/plugin-protocol.md「相乗りが基底の形を知る方法」）。
//
// 出すコードは、宣言だけから作る（docs/requirements.md「出力を推論しない」）: フローの op の引数と戻り値の型、名前、説明。
// 要求のケースの名前と説明、`@prd.ref` の引数。フローの配線を見るのは、書いたものの整合の検査（emit.go の GODESIGN-3）だけで、
// 食い違いは、診断（GODESIGN-*）で返す。
package godesign

import (
	"fmt"

	"github.com/mew-ton/hozo/plugin"
)

// TranspilerName は、このプラグインが提供するただ一つのトランスパイラの名前（docs/plugin-go-design.md「マニフェスト」）。
const TranspilerName = "go-design"

// Handler は go-design の plugin.Handler。emit と、emit と同じ検査で答える check を実装する。
type Handler struct {
}

var _ plugin.Handler = Handler{}

// Emit は View.viewer の名前空間の、テストのパッケージの design.go を作る（docs/plugin-go-design.md「出力」）。診断（GODESIGN-*）が
// あれば Files も Warnings も空で、Errors にそれを入れる。フローも、要求もなければ、ファイルを出さず warning を一つ返す。
// 要求の transpiler が [TranspilerName] でなければ plugin.ErrUnsupported（code "unsupported"）。
func (h Handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req == nil || req.View == nil {
		return nil, fmt.Errorf("emit: request has no view")
	}
	if req.Transpiler != TranspilerName {
		return nil, fmt.Errorf("transpiler %q is not provided by go-design (it provides %q): %w", req.Transpiler, TranspilerName, plugin.ErrUnsupported)
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

// Check は、emit と同じ検査をして、emit が errors で返す誤りを、検査の段でも返す（docs/plugin-protocol.md「check」、
// docs/decisions.md「決めたこと」404）。warning は、emit のときにだけ出る。
func (h Handler) Check(req *plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return plugin.CheckByEmit(h, []string{TranspilerName}, req)
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Coverage は実装しない（カバレッジを読むのは、基底）。
func (Handler) Coverage(*plugin.CoverageRequest) (*plugin.CoverageResponse, error) {
	return nil, plugin.ErrUnsupported
}
