package plugin

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	sdk "github.com/mew-ton/hozo/plugin"
)

// selectView は PLG-3 のテストの View: 見る側は web。web には op（デコレータ付き）、基底の状態、状態の分類、
// 値の分類、model があり、別の名前空間 api にも op と状態がある。
func selectView() *ir.View {
	base := func(name string, decos ...ir.Application) ir.DeclBase {
		return ir.DeclBase{Name: name, Decorators: decos}
	}
	web := ir.Namespace{Name: "web", Decls: []ir.Decl{
		&ir.Model{DeclBase: base("web.Page")},
		&ir.State{DeclBase: base("web.token"), Shape: ir.StateNullable, Element: ir.String()},
		&ir.Classify{DeclBase: base("web.LoggedIn"), Subject: ir.StatesSubject{States: []string{"web.token"}}, Cases: []ir.ClassifyCase{{Name: "LoggedIn"}}, HasDefault: true},
		&ir.Classify{DeclBase: base("web.Kind"), Subject: ir.TypeSubject{Type: ir.String()}, Cases: []ir.ClassifyCase{{Name: "A"}}, HasDefault: true},
		&ir.Op{DeclBase: base("web.open",
			ir.Application{Decorator: "t.cache"},
			ir.Application{Decorator: "t.guarded", Via: "web.guardMe"})},
		&ir.Op{DeclBase: base("web.plain")},
	}}
	api := ir.Namespace{Name: "api", Decls: []ir.Decl{
		&ir.State{DeclBase: base("api.users"), Shape: ir.StateList, Element: ir.String()},
		&ir.Op{DeclBase: base("api.getUser")},
	}}
	return &ir.View{Viewer: "web", Program: &ir.Program{Version: ir.Version, Namespaces: []ir.Namespace{{Name: ""}, web, api}}}
}

// docs/plugin-protocol.md「選択」と「診断」の PLG-3: view.viewer の名前空間にない FQN、存在しない FQN、
// wraps がその op に付いていないデコレータを指している。どれも proc.MalformedOutput。
func TestCheckSelect(t *testing.T) {
	view := selectView()
	ok := []struct {
		name string
		sel  sdk.Select
	}{
		{"empty", sdk.Select{}},
		{"roots and stops", sdk.Select{Roots: []string{"web.open", "web.plain"}, Stops: []string{"web.plain"}}},
		{"base state and state classification", sdk.Select{States: []string{"web.token", "web.LoggedIn"}}},
		{"wraps by decorator", sdk.Select{Wraps: []sdk.Wrap{{Op: "web.open", Decorator: "t.cache"}}}},
		{"wraps by generic decorator behind a named one", sdk.Select{Wraps: []sdk.Wrap{{Op: "web.open", Decorator: "t.guarded"}}}},
		{"wraps by named decorator (via)", sdk.Select{Wraps: []sdk.Wrap{{Op: "web.open", Decorator: "web.guardMe"}}}},
	}
	for _, c := range ok {
		t.Run("ok/"+c.name, func(t *testing.T) {
			if err := CheckSelect("p", view, c.sel); err != nil {
				t.Errorf("err = %v", err)
			}
		})
	}
	ng := []struct {
		name string
		sel  sdk.Select
		want string // メッセージに含む語
	}{
		{"roots outside the viewer namespace", sdk.Select{Roots: []string{"api.getUser"}}, `roots の "api.getUser" は名前空間 web の宣言でない`},
		{"roots unknown", sdk.Select{Roots: []string{"web.nope"}}, `roots の "web.nope" は存在しない`},
		{"roots not an op", sdk.Select{Roots: []string{"web.Page"}}, `roots の "web.Page" は op でない（model）`},
		{"stops outside the viewer namespace", sdk.Select{Stops: []string{"api.getUser"}}, `stops の "api.getUser" は名前空間 web の宣言でない`},
		{"stops unknown", sdk.Select{Stops: []string{"nope"}}, `stops の "nope" は存在しない`},
		{"states outside the viewer namespace", sdk.Select{States: []string{"api.users"}}, `states の "api.users" は名前空間 web の宣言でない`},
		{"states unknown", sdk.Select{States: []string{"web.nope"}}, `states の "web.nope" は存在しない`},
		{"states names an op", sdk.Select{States: []string{"web.open"}}, `states の "web.open" は状態でない（op）`},
		{"states names a value classification", sdk.Select{States: []string{"web.Kind"}}, `states の "web.Kind" は状態の分類でない`},
		{"wraps op outside the viewer namespace", sdk.Select{Wraps: []sdk.Wrap{{Op: "api.getUser", Decorator: "t.cache"}}}, `wraps の "api.getUser" は名前空間 web の宣言でない`},
		{"wraps op unknown", sdk.Select{Wraps: []sdk.Wrap{{Op: "web.nope", Decorator: "t.cache"}}}, `wraps の "web.nope" は存在しない`},
		{"wraps decorator not on the op", sdk.Select{Wraps: []sdk.Wrap{{Op: "web.plain", Decorator: "t.cache"}}}, `wraps の op "web.plain" にデコレータ "t.cache" は付いていない`},
		{"wraps decorator on another op only", sdk.Select{Wraps: []sdk.Wrap{{Op: "web.open", Decorator: "t.retry"}}}, `wraps の op "web.open" にデコレータ "t.retry" は付いていない`},
		{"wraps without decorator", sdk.Select{Wraps: []sdk.Wrap{{Op: "web.open"}}}, `wraps の op "web.open" にデコレータ "" は付いていない`},
		// 正しい項目の後ろにある誤りも見つける
		{"second root", sdk.Select{Roots: []string{"web.open", "api.getUser"}}, `roots の "api.getUser"`},
	}
	for _, c := range ng {
		t.Run("ng/"+c.name, func(t *testing.T) {
			err := CheckSelect("p", view, c.sel)
			var me *MalformedOutputError
			if !errors.As(err, &me) || !errors.Is(err, ErrMalformedOutput) {
				t.Fatalf("err = %v, want *MalformedOutputError（proc.MalformedOutput）", err)
			}
			if me.Plugin != "p" || !strings.HasPrefix(me.Reason, "PLG-3: ") || !strings.Contains(me.Reason, c.want) {
				t.Errorf("Plugin = %q, Reason = %q, want PLG-3 に %q", me.Plugin, me.Reason, c.want)
			}
		})
	}
}

// docs/plugin-protocol.md「選択」: hozoc は名前空間に設定されたすべてのプラグインの select を合わせる。
func TestMergeSelect(t *testing.T) {
	a := sdk.Select{Roots: []string{"web.open"}, Stops: []string{"web.plain"}, Wraps: []sdk.Wrap{{Op: "web.open", Decorator: "t.cache"}}}
	b := sdk.Select{Roots: []string{"web.plain", "web.open"}, States: []string{"web.token"},
		Wraps: []sdk.Wrap{{Op: "web.open", Decorator: "t.cache"}, {Op: "web.open", Decorator: "web.guardMe"}}}
	got := MergeSelect(a, b)
	want := sdk.Select{
		Roots:  []string{"web.open", "web.plain"},
		Stops:  []string{"web.plain"},
		States: []string{"web.token"},
		Wraps:  []sdk.Wrap{{Op: "web.open", Decorator: "t.cache"}, {Op: "web.open", Decorator: "web.guardMe"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MergeSelect = %+v\nwant %+v", got, want)
	}
	// 何も合わせなければ、何も選ばない（列は nil にしない）
	if got := MergeSelect(); !reflect.DeepEqual(got, sdk.Select{Roots: []string{}, Stops: []string{}, States: []string{}, Wraps: []sdk.Wrap{}}) {
		t.Errorf("MergeSelect() = %#v", got)
	}
}
