package golang

import (
	"testing"

	"github.com/mew-ton/hozo/ir"
)

func TestGoTypeCore(t *testing.T) {
	tests := []struct {
		t    ir.Type
		want string
	}{
		{ir.String(), "string"},
		{ir.Boolean(), "bool"},
		{ir.Int(), "int64"},
		{ir.Float(), "float64"},
		{ir.Time(), "time.Time"},
		{ir.Duration(), "time.Duration"},
		{nil, ""},
	}
	for _, tt := range tests {
		var im Imports
		if got := GoType(tt.t, &im); got != tt.want {
			t.Errorf("GoType(%v) = %q, want %q", tt.t, got, tt.want)
		}
		if got := GoType(tt.t, nil); got != tt.want {
			t.Errorf("GoType(%v, nil) = %q, want %q", tt.t, got, tt.want)
		}
	}
	var im Imports
	GoType(ir.Time(), &im)
	GoType(ir.Duration(), &im)
	if got := im.Render(); got != "import \"time\"\n" {
		t.Errorf("Time/Duration must import time once: %q", got)
	}
	var none Imports
	GoType(ir.Int(), &none)
	if none.Render() != "" {
		t.Errorf("int must not import anything: %q", none.Render())
	}
}

func TestGoTypeComposite(t *testing.T) {
	im := &Imports{Viewer: "server"}
	user := ir.Ref("server.User")
	tests := []struct {
		t    ir.Type
		want string
	}{
		{user, "User"},
		{ir.Ref("server.OrderPage.Ready"), "OrderPageReady"},
		{ir.Ref("server.getMe$in"), "GetMeInInput"},
		{ir.Optional(user), "*User"},
		{ir.Optional(ir.String()), "*string"},
		{ir.Optional(ir.Int()), "*int64"},
		{ir.Optional(ir.Time()), "*time.Time"},
		{ir.Optional(ir.Array(user)), "[]User"},
		{ir.Optional(ir.Record(ir.String(), user)), "map[string]User"},
		{ir.Array(user), "[]User"},
		{ir.Array(ir.Optional(user)), "[]*User"},
		{ir.Array(ir.Array(ir.Int())), "[][]int64"},
		{ir.Record(ir.String(), ir.Array(ir.Float())), "map[string][]float64"},
		{ir.Record(ir.Ref("server.UserId"), ir.Optional(ir.Boolean())), "map[UserId]*bool"},
		{ir.Case("server.WarrantyKind", []string{"Standard"}, ir.Ref("server.Item")), "Item"},
		{ir.Optional(ir.Case("server.WarrantyKind", []string{"Standard"}, ir.Ref("server.Item"))), "*Item"},
		{ir.Optional(ir.Case("server.C", []string{"A"}, ir.Array(ir.Int()))), "[]int64"},
		{ir.Array(ir.Case("server.C", []string{"A", "B"}, ir.Duration())), "[]time.Duration"},
	}
	for _, tt := range tests {
		if got := GoType(tt.t, im); got != tt.want {
			t.Errorf("GoType(%v) = %q, want %q", tt.t, got, tt.want)
		}
	}
}

func TestGoTypeRefWithoutResolver(t *testing.T) {
	// Resolve がなければ、viewer の下はローカル、ほかは先頭が大文字でないセグメントを名前空間とみなしてローカル（複製した型）。
	im := &Imports{Viewer: "browser"}
	tests := []struct{ fqn, want string }{
		{"browser.OrdersQuery", "OrdersQuery"},
		{"browser.OrderPage.Ready", "OrderPageReady"},
		{"server.User", "User"},
		{"server.admin.Report", "Report"},
		{"server.OrderPage.Ready", "OrderPageReady"},
		{"Db", "Db"},
		{"db", "Db"},
	}
	for _, tt := range tests {
		if got := GoType(ir.Ref(tt.fqn), im); got != tt.want {
			t.Errorf("GoType(ref %q) = %q, want %q", tt.fqn, got, tt.want)
		}
	}
	if im.Render() != "" {
		t.Errorf("no imports expected without a resolver: %q", im.Render())
	}
	// imports が nil なら Viewer "" でローカル。
	if got := GoType(ir.Ref("server.User"), nil); got != "ServerUser" {
		t.Errorf("GoType(ref, nil) = %q", got)
	}
}

func TestGoTypeRefWithResolver(t *testing.T) {
	im := &Imports{
		Viewer: "browser",
		Resolve: func(fqn string) (namespace, pkgPath, pkgName string, local bool) {
			switch {
			case hasPrefix(fqn, "browser."):
				return "browser", "", "", true
			case hasPrefix(fqn, "server.admin."):
				return "server.admin", "example.com/app/gen/server/admin", "admin", false
			case hasPrefix(fqn, "server."):
				return "server", "example.com/app/gen/server", "server", false
			case hasPrefix(fqn, "ts."):
				// TS の名前空間の型は browser のパッケージに複製されている（§4.5）。
				return "ts", "", "", true
			}
			return "", "", "", true
		},
	}
	tests := []struct {
		t    ir.Type
		want string
	}{
		{ir.Ref("browser.OrdersQuery"), "OrdersQuery"},
		{ir.Ref("server.User"), "server.User"},
		{ir.Ref("server.OrderPage.Ready"), "server.OrderPageReady"},
		{ir.Optional(ir.Ref("server.Token")), "*server.Token"},
		{ir.Array(ir.Ref("server.admin.Report")), "[]admin.Report"},
		{ir.Ref("ts.Widget"), "Widget"},
		{ir.Ref("db"), "Db"},
		{ir.Record(ir.Ref("server.UserId"), ir.Time()), "map[server.UserId]time.Time"},
	}
	for _, tt := range tests {
		if got := GoType(tt.t, im); got != tt.want {
			t.Errorf("GoType(%v) = %q, want %q", tt.t, got, tt.want)
		}
	}
	want := "import (\n\t\"time\"\n\n\t\"example.com/app/gen/server\"\n\t\"example.com/app/gen/server/admin\"\n)\n"
	if got := im.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestGoTypeResolverAlias(t *testing.T) {
	// 同じパッケージ名が二つの名前空間から来るとき、二つ目は別名で修飾する。
	im := &Imports{
		Viewer: "app",
		Resolve: func(fqn string) (namespace, pkgPath, pkgName string, local bool) {
			if hasPrefix(fqn, "a.core.") {
				return "a.core", "example.com/x/a/core", "core", false
			}
			return "b.core", "example.com/x/b/core", "core", false
		},
	}
	if got := GoType(ir.Ref("a.core.T"), im); got != "core.T" {
		t.Errorf("first = %q", got)
	}
	if got := GoType(ir.Ref("b.core.U"), im); got != "core2.U" {
		t.Errorf("second = %q", got)
	}
	if got := im.Render(); got != "import (\n\t\"example.com/x/a/core\"\n\tcore2 \"example.com/x/b/core\"\n)\n" {
		t.Errorf("Render() = %q", got)
	}
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

func TestGuessNamespace(t *testing.T) {
	tests := []struct{ fqn, want string }{
		{"server.User", "server"}, {"server.OrderPage.Ready", "server"}, {"server.admin.Report", "server.admin"},
		{"Db", ""}, {"db", ""}, {"a.b.c", "a.b"},
	}
	for _, tt := range tests {
		if got := guessNamespace(tt.fqn); got != tt.want {
			t.Errorf("guessNamespace(%q) = %q, want %q", tt.fqn, got, tt.want)
		}
	}
}
