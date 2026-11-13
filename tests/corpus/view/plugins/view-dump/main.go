// Command view-dump は tests/view_test.go のためのプラグイン。emit の要求の view（docs/ir.md「その名前空間から見た IR」）を
// 2 スペースの字下げで JSON にし直し、<view.config.dir>/view.json の一つのファイルとして返す。テストはそのファイルを読んで、
// プラグインが受け取った View が hozoc の計算した View と同じであることを確かめる。既定の出力の対象は、テストの語彙の
// @t.entry が付いた op を起点に選ぶ（docs/plugin-protocol.md「選択」）。check と import は持たない。
//
// トランスパイラを二つ提供する（docs/plugin-protocol.md「マニフェスト」）。要求の transpiler で処理を分ける:
// view-dump は上のとおり。view-none は何も書かず、何も選ばない（api の単位に書く。api は何も出力しない）。
// ほかの名前は code "unsupported"。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/mew-ton/hozo/plugin"
)

// entryDecorator は起点を選ぶデコレータ（tests/corpus/view/t.hozo）。
const entryDecorator = "t.entry"

type handler struct{}

func (handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req.View == nil {
		return nil, fmt.Errorf("view がない")
	}
	switch req.Transpiler {
	case "view-dump":
	case "view-none":
		return &plugin.EmitResponse{}, nil
	default:
		return nil, fmt.Errorf("transpiler %q: %w", req.Transpiler, plugin.ErrUnsupported)
	}
	content, err := json.MarshalIndent(req.View, "", "  ")
	if err != nil {
		return nil, err
	}
	return &plugin.EmitResponse{
		Files:  []plugin.File{{Path: path.Join(req.View.Config.Dir, "view.json"), Content: string(content) + "\n"}},
		Select: plugin.Select{Roots: plugin.OpsWith(req.View, entryDecorator)},
	}, nil
}

func (handler) Check(*plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return nil, plugin.ErrUnsupported
}

func (handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}

func main() { os.Exit(plugin.Main(handler{})) }
