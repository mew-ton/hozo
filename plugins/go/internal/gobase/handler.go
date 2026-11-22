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

// Handler は go の plugin.Handler。emit と coverage を実装する。
type Handler struct {
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
	stderr := h.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	files, errs := Emit(req, stderr)
	if len(errs) > 0 {
		return &plugin.EmitResponse{Errors: errs}, nil
	}
	return &plugin.EmitResponse{Files: files}, nil
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

// Coverage は、go test -coverprofile の形のカバレッジから、要求のファイルの、実行された行を取り出す
// （docs/plugin-go.md「カバレッジを読む」）。
func (h Handler) Coverage(req *plugin.CoverageRequest) (*plugin.CoverageResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("coverage: no request")
	}
	return Coverage(req)
}
