// Package govalid は、検証のトランスパイラ go-valid の中身（docs/plugin-go-valid.md）。
//
// Handler が plugin.Handler を実装し、View（出力ごとに一つ。見る側は出力する名前空間 N）から、N の定義のパッケージの
// validate.go を作る: `Validate` を持つ scalar と model ごとに、制約（`@valid.*`）を確かめるメソッド一つ。
// どの型が `Validate` を持つかは公開の SDK plugin/govalid で、置き場と名前は plugin/golang で求める（基底の出力は見ない。
// docs/plugin-protocol.md「相乗りが基底の形を知る方法」）。制約の使い方の誤りは、診断（GOVALID-*）で返す。
package govalid

import (
	"fmt"

	"github.com/mew-ton/hozo/plugin"
)

// TranspilerName は、このプラグインが提供するただ一つのトランスパイラの名前（docs/plugin-go-valid.md「マニフェスト」）。
const TranspilerName = "go-valid"

// Handler は go-valid の plugin.Handler。emit と、emit と同じ検査で答える check を実装する。
type Handler struct {
}

var _ plugin.Handler = Handler{}

// Emit は View.viewer の名前空間の validate.go を作る（docs/plugin-go-valid.md「出力」）。診断（GOVALID-*）があれば Files は
// 空で、Errors にそれを入れる。`Validate` を持つ型がなければ、何も返さない（warning もない）。
// 要求の transpiler が [TranspilerName] でなければ plugin.ErrUnsupported（code "unsupported"）。
func (h Handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req == nil || req.View == nil {
		return nil, fmt.Errorf("emit: request has no view")
	}
	if req.Transpiler != TranspilerName {
		return nil, fmt.Errorf("transpiler %q is not provided by go-valid (it provides %q): %w", req.Transpiler, TranspilerName, plugin.ErrUnsupported)
	}
	files, errs, err := Emit(req)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}
	if len(errs) > 0 {
		return &plugin.EmitResponse{Errors: errs}, nil
	}
	return &plugin.EmitResponse{Files: files}, nil
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Coverage は実装しない（カバレッジを読むのは、基底）。
func (Handler) Coverage(*plugin.CoverageRequest) (*plugin.CoverageResponse, error) {
	return nil, plugin.ErrUnsupported
}
