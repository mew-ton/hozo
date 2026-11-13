package plugin

import (
	"fmt"

	"github.com/mew-ton/hozo/ir"
	sdk "github.com/mew-ton/hozo/plugin"
)

// CheckSelect は、プラグイン plugin が名前空間 view.Viewer の emit の応答で返した select を確かめる
// （docs/plugin-protocol.md「選択」の最後の項目）。誤りがあれば PLG-3 の *MalformedOutputError（proc.MalformedOutput）:
//
//   - roots / stops / wraps の op が、view.Viewer の名前空間の op でない（別の名前空間の宣言、存在しない FQN、op でない宣言）。
//   - states が、view.Viewer の名前空間の状態（基底の状態か、状態の分類。docs/emit-go.md「state」）でない。
//   - wraps の decorator が、その op に付いていない（Application の decorator にも via にもない）。
//
// 最初に見つけた一つを返す。誤りがなければ nil。
func CheckSelect(plugin string, view *ir.View, sel sdk.Select) error {
	bad := func(format string, args ...any) error {
		return &MalformedOutputError{Plugin: plugin, Reason: "PLG-3: select の " + fmt.Sprintf(format, args...)}
	}
	// own は FQN が view.Viewer の名前空間の宣言ならそれを返す。そうでなければ、何が違うかを言う。
	own := func(fqn string) (ir.Decl, string) {
		if view == nil || view.Program == nil {
			return nil, "View に Program がない"
		}
		if ns := view.Program.Namespace(view.Viewer); ns != nil {
			for _, d := range ns.Decls {
				if d != nil && d.Common().Name == fqn {
					return d, ""
				}
			}
		}
		if view.Program.Lookup(fqn) != nil {
			return nil, fmt.Sprintf("名前空間 %s の宣言でない", view.Viewer)
		}
		return nil, "存在しない"
	}
	op := func(list, fqn string) (*ir.Op, error) {
		d, why := own(fqn)
		if d == nil {
			return nil, bad("%s の %q は%s", list, fqn, why)
		}
		o, ok := d.(*ir.Op)
		if !ok {
			return nil, bad("%s の %q は op でない（%s）", list, fqn, d.Kind())
		}
		return o, nil
	}
	for _, fqn := range sel.Roots {
		if _, err := op("roots", fqn); err != nil {
			return err
		}
	}
	for _, fqn := range sel.Stops {
		if _, err := op("stops", fqn); err != nil {
			return err
		}
	}
	for _, fqn := range sel.States {
		d, why := own(fqn)
		if d == nil {
			return bad("states の %q は%s", fqn, why)
		}
		switch x := d.(type) {
		case *ir.State:
		case *ir.Classify:
			if _, ok := x.SubjectStates(); !ok {
				return bad("states の %q は状態の分類でない（値の分類）", fqn)
			}
		default:
			return bad("states の %q は状態でない（%s）", fqn, d.Kind())
		}
	}
	for _, w := range sel.Wraps {
		o, err := op("wraps", w.Op)
		if err != nil {
			return err
		}
		found := false
		for _, app := range o.Decorators {
			if w.Decorator != "" && (app.Decorator == w.Decorator || app.Via == w.Decorator) {
				found = true
				break
			}
		}
		if !found {
			return bad("wraps の op %q にデコレータ %q は付いていない", w.Op, w.Decorator)
		}
	}
	return nil
}

// MergeSelect は、名前空間に設定されたプラグインの select を一つに合わせる（docs/plugin-protocol.md「選択」:
// 「すべてのプラグインの select を合わせ」）。順序は渡された順で、同じ項目は一度だけ。結果の列は空でも nil にしない。
func MergeSelect(sels ...sdk.Select) sdk.Select {
	out := sdk.Select{Roots: []string{}, Stops: []string{}, States: []string{}, Wraps: []sdk.Wrap{}}
	add := func(dst *[]string, seen map[string]bool, src []string) {
		for _, s := range src {
			if !seen[s] {
				seen[s] = true
				*dst = append(*dst, s)
			}
		}
	}
	roots, stops, states, wraps := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[sdk.Wrap]bool{}
	for _, s := range sels {
		add(&out.Roots, roots, s.Roots)
		add(&out.Stops, stops, s.Stops)
		add(&out.States, states, s.States)
		for _, w := range s.Wraps {
			if !wraps[w] {
				wraps[w] = true
				out.Wraps = append(out.Wraps, w)
			}
		}
	}
	return out
}
