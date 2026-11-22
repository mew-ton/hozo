// Command view-dump は tests/view_test.go のためのプラグイン。トランスパイラ view-dump（requires を持たない、テストのための基底）を
// 提供する: emit の要求の view（docs/ir.md「その名前空間から見た IR」）を 2 スペースの字下げで JSON にし直し、
// <view.config.dir>/view.json の一つのファイルとして返す。テストはそのファイルを読んで、プラグインが受け取った View が
// hozoc の計算した View と同じであることを確かめる。check と import は持たない。ほかの名前の transpiler は code "unsupported"。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/mew-ton/hozo/plugin"
)

type handler struct{}

func (handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req.View == nil {
		return nil, fmt.Errorf("view がない")
	}
	if req.Transpiler != "view-dump" {
		return nil, fmt.Errorf("transpiler %q: %w", req.Transpiler, plugin.ErrUnsupported)
	}
	content, err := json.MarshalIndent(req.View, "", "  ")
	if err != nil {
		return nil, err
	}
	return &plugin.EmitResponse{
		Files: []plugin.File{{Path: path.Join(req.View.Config.Dir, "view.json"), Content: string(content) + "\n"}},
	}, nil
}

func (handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Coverage は実装しない（カバレッジを読むのは、基底）。
func (handler) Coverage(*plugin.CoverageRequest) (*plugin.CoverageResponse, error) {
	return nil, plugin.ErrUnsupported
}

func main() { os.Exit(plugin.Main(handler{})) }
