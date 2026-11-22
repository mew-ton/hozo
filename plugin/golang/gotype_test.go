package golang

import (
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// typesView は、型のテストの View: 見る側 server（model、scalar、union、合成 model）と、別の名前空間 type（単位 2 が出力する）。
func typesView() *ir.View {
	v := viewOf("server",
		namespace("server",
			model("server.User"), scalar("server.Email", ir.String()), scalar("server.Count", ir.Int()),
			scalar("server.When", ir.Datetime()), scalar("server.Alias", ir.Ref("server.Email")), scalar("server.Flag", ir.Boolean()),
			union("server.Page", ir.UnionArm{Name: "Ready", Type: ir.Ref("server.Page.Ready")}),
			model("server.Page.Ready"),
			// インデックスの model: Go では、名前付きのマップの型（nil が「なし」）
			&ir.Model{DeclBase: ir.DeclBase{Name: "server.Counts"}, Fields: []ir.Field{}, Index: &ir.ModelIndex{Name: "name", Key: ir.String(), Value: ir.Int()}},
		),
		namespace("type", model("type.Session"), scalar("type.Token", ir.String()), union("type.Shape")),
	)
	v.Namespaces["type"] = []ir.NamespaceConfig{conf(2, "gen/type")}
	return v
}

// docs/plugin-go.md「型」の表の行に一つずつ。
func TestGoType(t *testing.T) {
	v := typesView()
	l, err := LayoutOf(v)
	if err != nil {
		t.Fatal(err)
	}
	im := NewImports(v, l, true)
	tests := []struct {
		t    ir.Type
		want string
	}{
		{ir.String(), "string"}, {ir.Boolean(), "bool"}, {ir.Int(), "int64"}, {ir.Float(), "float64"},
		{ir.Datetime(), "time.Time"}, {ir.Duration(), "time.Duration"},
		{ir.Date(), "runtime.Date"}, {ir.Time(), "runtime.TimeOfDay"}, {ir.Optional(ir.Date()), "*runtime.Date"},
		// optional(T): array、インデックスの model への ref、union（arms）への ref はそのまま。それ以外は *T
		{ir.Optional(ir.String()), "*string"},
		{ir.Optional(ir.Ref("server.User")), "*User"},
		{ir.Optional(ir.Ref("server.Email")), "*Email"},
		{ir.Optional(ir.Array(ir.String())), "[]string"},
		{ir.Optional(ir.Ref("server.Counts")), "Counts"}, {ir.Ref("server.Counts"), "Counts"},
		{ir.Optional(ir.Ref("server.Page")), "Page"},
		{ir.Optional(ir.Ref("type.Shape")), "pkgtype.Shape"},
		{ir.Array(ir.Ref("server.User")), "[]User"},
		{ir.Array(ir.Optional(ir.Int())), "[]*int64"},
		// ref: model は構造体、scalar は名前付きの型、union はインターフェース。合成 model は連結した名前
		{ir.Ref("server.User"), "User"}, {ir.Ref("server.Email"), "Email"}, {ir.Ref("server.Page"), "Page"},
		{ir.Ref("server.Page.Ready"), "PageReady"},
		// 別の名前空間の型は、置き場の入口のパッケージの名前で修飾する
		{ir.Ref("type.Session"), "pkgtype.Session"},
		{ir.Optional(ir.Ref("type.Token")), "*pkgtype.Token"},
		// case(C, cases, of) は of の型
		{ir.Case("server.Kind", []string{"A"}, ir.Ref("server.User")), "User"},
		{ir.Optional(ir.Case("server.Kind", []string{"A"}, ir.Array(ir.String()))), "[]string"},
	}
	for _, tt := range tests {
		if got := GoType(tt.t, im); got != tt.want {
			t.Errorf("GoType(%s) = %q, want %q", tt.t, got, tt.want)
		}
	}
	// void と never は型にならない
	if got := GoType(nil, im); got != "" {
		t.Errorf("GoType(nil) = %q", got)
	}
	if got := GoType(ir.Never(), im); got != "" {
		t.Errorf("GoType(never) = %q", got)
	}
	if err := im.Err(); err != nil {
		t.Errorf("Err = %v", err)
	}
	// 使った import だけが入る
	want := "import (\n\t\"time\"\n\n\tpkgtype \"example.com/app/gen/type\"\n\t\"github.com/mew-ton/hozo/runtime\"\n)\n"
	if got := im.Render(); got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
	// imports が nil なら修飾なし
	if got := GoType(ir.Optional(ir.Ref("type.Session")), nil); got != "*Session" {
		t.Errorf("GoType(nil imports) = %q", got)
	}
}

func TestIsNillable(t *testing.T) {
	p := typesView().Program
	for _, tt := range []struct {
		t    ir.Type
		want bool
	}{
		{ir.Array(ir.String()), true}, {ir.Ref("server.Counts"), true}, {ir.Ref("server.Page"), true},
		{ir.Case("c", nil, ir.Array(ir.String())), true},
		{ir.String(), false}, {ir.Ref("server.User"), false}, {ir.Ref("server.Email"), false}, {ir.Datetime(), false},
	} {
		if got := IsNillable(tt.t, p); got != tt.want {
			t.Errorf("IsNillable(%s) = %v, want %v", tt.t, got, tt.want)
		}
	}
	if IsNillable(ir.Ref("server.Page"), nil) {
		t.Error("Program がなければ、ref は nil にならない型として扱う")
	}
}

// docs/plugin-go.md「呼び出し」の「零値」。
func TestZero(t *testing.T) {
	v := typesView()
	l, _ := LayoutOf(v)
	im := NewImports(v, l, true)
	for _, tt := range []struct {
		t    ir.Type
		want string
	}{
		{ir.Optional(ir.Ref("server.User")), "nil"}, {ir.Ref("server.Page"), "nil"}, {ir.Array(ir.Int()), "nil"},
		{ir.Ref("server.Counts"), "nil"}, {ir.Optional(ir.Ref("server.Page")), "nil"},
		{ir.Ref("server.User"), "User{}"}, {ir.Ref("type.Session"), "pkgtype.Session{}"}, {ir.Ref("server.Page.Ready"), "PageReady{}"},
		{ir.Datetime(), "time.Time{}"}, {ir.Ref("server.When"), "When{}"},
		{ir.Date(), "runtime.Date{}"}, {ir.Time(), "runtime.TimeOfDay{}"},
		{ir.String(), `""`}, {ir.Ref("server.Email"), `""`}, {ir.Ref("server.Alias"), `""`}, {ir.Ref("type.Token"), `""`},
		{ir.Int(), "0"}, {ir.Float(), "0"}, {ir.Duration(), "0"}, {ir.Ref("server.Count"), "0"},
		{ir.Boolean(), "false"}, {ir.Ref("server.Flag"), "false"},
		{ir.Case("c", nil, ir.Ref("server.User")), "User{}"},
		{nil, ""}, {ir.Never(), ""},
	} {
		if got := Zero(tt.t, im); got != tt.want {
			t.Errorf("Zero(%v) = %q, want %q", tt.t, got, tt.want)
		}
	}
}

// docs/plugin-go.md「コメント」の「型の表示」。
// TestRowDisplay は、コメントの中の行の遷移の書き方（docs/plugin-go.md「コメント」の「行の遷移の表示」）。
func TestRowDisplay(t *testing.T) {
	effect := func(op ir.Operation, row *ir.RowTransition) *ir.Effect {
		return &ir.Effect{Target: ir.BuiltinTarget{State: "shop.orders", Operation: op}, Row: row}
	}
	for _, tt := range []struct {
		e    *ir.Effect
		want string
	}{
		{effect(ir.OpPost, &ir.RowTransition{Param: "order", Classify: "shop.Stage", To: "Accepted"}), "as shop.Stage.Accepted"},
		{effect(ir.OpPut, &ir.RowTransition{Param: "order", Classify: "shop.Stage", To: "InProduction"}), "as shop.Stage.InProduction"},
		{effect(ir.OpPut, &ir.RowTransition{Param: "order"}), "as else"},
		{effect(ir.OpDelete, &ir.RowTransition{Param: "order", Classify: "shop.Life", To: "Absent"}), "as shop.Life.Absent"},
		// 行の遷移を持たない効果と、宛先が組み込みの効果でない効果は ""
		{effect(ir.OpPut, nil), ""},
		{&ir.Effect{Target: ir.TransitionTarget{Of: "shop.Checkout"}, Row: &ir.RowTransition{Param: "order"}}, ""},
	} {
		if got := RowDisplay(tt.e); got != tt.want {
			t.Errorf("RowDisplay(%+v) = %q, want %q", tt.e.Row, got, tt.want)
		}
	}
}

// TestNarrowedParamsDisplay は、コメントの中の、絞り込んだ引数の書き方（docs/plugin-go.md「コメント」の「絞り込んだ引数の表示」）。
func TestNarrowedParamsDisplay(t *testing.T) {
	order := ir.Ref("shop.Order")
	param := func(name string, t ir.Type) ir.OpParam { return ir.OpParam{Name: name, Type: t} }
	for _, tt := range []struct {
		params []ir.OpParam
		want   string
	}{
		{nil, ""},
		{[]ir.OpParam{param("order", order)}, ""},
		{[]ir.OpParam{param("order", ir.Case("shop.Stage", []string{"Accepted"}, order))}, "order on shop.Stage.Accepted"},
		{[]ir.OpParam{param("at", ir.Datetime()), param("order", ir.Case("shop.Stage", []string{"Draft", "Accepted"}, order))}, "order on shop.Stage.Draft | shop.Stage.Accepted"},
		{[]ir.OpParam{
			param("order", ir.Case("shop.Stage", []string{"Accepted"}, order)),
			param("payment", ir.Case("pay.Standing", []string{"Held"}, ir.Ref("pay.Payment"))),
		}, "order on shop.Stage.Accepted、payment on pay.Standing.Held"},
	} {
		if got := NarrowedParamsDisplay(&ir.Effect{Params: tt.params}); got != tt.want {
			t.Errorf("NarrowedParamsDisplay(%+v) = %q, want %q", tt.params, got, tt.want)
		}
	}
}

func TestTypeDisplay(t *testing.T) {
	for _, tt := range []struct {
		t    ir.Type
		want string
	}{
		{ir.String(), "string"}, {ir.Datetime(), "datetime"}, {ir.Ref("type.User"), "type.User"},
		{ir.Optional(ir.Ref("type.User")), "type.User?"}, {ir.Array(ir.Ref("type.Order")), "type.Order[]"},
		{ir.Case("store.Login", []string{"LoggedIn"}, ir.Ref("type.Session")), "type.Session"},
	} {
		if got := TypeDisplay(tt.t); got != tt.want {
			t.Errorf("TypeDisplay(%s) = %q, want %q", tt.t, got, tt.want)
		}
	}
}
