// Command pick は tests/corpus のためのプラグイン。emit の応答で、既定の出力の対象の選択（docs/plugin-protocol.md「選択」）
// だけを返す。ファイルは出さない。check と import は持たない。
//
// 何を選ぶかは、マニフェストのディレクトリ（HOZO_PLUGIN_DIR）の pick.json に、デコレータの FQN で書く:
//
//	{
//	  "roots":  ["page.route"],   // このデコレータが付いた op を起点にする
//	  "stops":  ["page.dialog"],  // このデコレータが付いた op は、選ぶが本体は辿らない
//	  "states": ["db"],           // このデコレータが付いた状態を選ぶ
//	  "wraps":  ["t.cache"],      // このデコレータの適用を引き受ける（包む関数が生成される）
//	  "all":    false             // true なら、すべての op を起点にし、すべての状態（基底の状態と状態の分類）を選ぶ
//	}
//
// 見るのは view.viewer の名前空間の宣言だけ。roots / stops / wraps は名前付きデコレータを通した適用（via）も数える
// （plugin.OpsWith）。本物のプラグインは View を見て自分で決める（デコレータを読まなくてもよい）が、テストでは
// どのコーパスが何を選ぶかを pick.json に並べて見えるようにする。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// rules は pick.json の形。
type rules struct {
	Roots  []string `json:"roots"`
	Stops  []string `json:"stops"`
	States []string `json:"states"`
	Wraps  []string `json:"wraps"`
	All    bool     `json:"all"`
}

type handler struct{}

func (handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req.View == nil || req.View.Program == nil {
		return nil, fmt.Errorf("view がない")
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOZO_PLUGIN_DIR"), "pick.json"))
	if err != nil {
		return nil, err
	}
	var r rules
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("pick.json: %w", err)
	}
	return &plugin.EmitResponse{Select: pick(req.View, r)}, nil
}

// pick は view の見る側の名前空間から、r に従って選ぶ。
func pick(view *ir.View, r rules) plugin.Select {
	var sel plugin.Select
	add := func(dst *[]string, fqn string) {
		for _, x := range *dst {
			if x == fqn {
				return
			}
		}
		*dst = append(*dst, fqn)
	}
	for _, d := range r.Roots {
		for _, op := range plugin.OpsWith(view, d) {
			add(&sel.Roots, op)
		}
	}
	for _, d := range r.Stops {
		for _, op := range plugin.OpsWith(view, d) {
			add(&sel.Stops, op)
		}
	}
	for _, d := range r.Wraps {
		for _, op := range plugin.OpsWith(view, d) {
			sel.Wraps = append(sel.Wraps, plugin.Wrap{Op: op, Decorator: d})
		}
	}
	ns := view.Program.Namespace(view.Viewer)
	if ns == nil {
		return sel
	}
	for _, decl := range ns.Decls {
		switch x := decl.(type) {
		case *ir.Op:
			if r.All {
				add(&sel.Roots, x.Name)
			}
		case *ir.State:
			if r.All || decorated(x.Decorators, r.States) {
				add(&sel.States, x.Name)
			}
		case *ir.Classify:
			if _, ok := x.SubjectStates(); ok && r.All {
				add(&sel.States, x.Name)
			}
		}
	}
	return sel
}

// decorated は適用の列に decorators のどれかがあるか。
func decorated(apps []ir.Application, decorators []string) bool {
	for _, a := range apps {
		for _, d := range decorators {
			if a.Decorator == d || a.Via == d {
				return true
			}
		}
	}
	return false
}

func (handler) Check(*plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return nil, plugin.ErrUnsupported
}

func (handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}

func main() { os.Exit(plugin.Main(handler{})) }
