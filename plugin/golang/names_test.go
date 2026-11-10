package golang

import "testing"

func TestTypeName(t *testing.T) {
	tests := []struct{ fqn, ns, want string }{
		{"server.User", "server", "User"},
		{"server.user", "server", "User"},
		{"browser.OrderPage.Ready", "browser", "OrderPageReady"},
		{"server.LoginFailed.Value", "server", "LoginFailedValue"},
		{"server.admin.Report", "server.admin", "Report"},
		{"db", "", "Db"},
		{"browser.getMe$in", "browser", "GetMeInInput"},
		{"web.getUser$in$auth", "web", "GetUserInAuthInput"},
		{"server.User", "browser", "ServerUser"},
		{"server.OrderPage.Ready", "server", "OrderPageReady"},
		{"server.type", "server", "Type"},
	}
	for _, tt := range tests {
		if got := TypeName(tt.fqn, tt.ns); got != tt.want {
			t.Errorf("TypeName(%q, %q) = %q, want %q", tt.fqn, tt.ns, got, tt.want)
		}
	}
}

func TestFieldName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"id", "Id"}, {"userId", "UserId"}, {"url", "Url"}, {"URL", "URL"}, {"Id", "Id"},
		{"type", "Type"}, {"func", "Func"}, {"range", "Range"},
		{"_private", "_private"}, {"", ""}, {"éclair", "Éclair"},
	}
	for _, tt := range tests {
		if got := FieldName(tt.in); got != tt.want {
			t.Errorf("FieldName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	// 大文字化だけでは衝突を検出しない（GOEMIT-2 は呼ぶ側）。
	if FieldName("id") != FieldName("Id") {
		t.Error("id and Id must map to the same name so the caller can detect GOEMIT-2")
	}
}

func TestMethodName(t *testing.T) {
	tests := []struct{ in, want string }{{"login", "Login"}, {"openOrders", "OpenOrders"}, {"go", "Go"}, {"Login", "Login"}}
	for _, tt := range tests {
		if got := MethodName(tt.in); got != tt.want {
			t.Errorf("MethodName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLocalName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"in", "in"}, {"query", "query"}, {"type", "type_"}, {"range", "range_"}, {"func", "func_"},
		{"default", "default_"}, {"select", "select_"}, {"Type", "Type"}, {"types", "types"},
	}
	for _, tt := range tests {
		if got := LocalName(tt.in); got != tt.want {
			t.Errorf("LocalName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsKeyword(t *testing.T) {
	for _, k := range []string{"break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough", "for",
		"func", "go", "goto", "if", "import", "interface", "map", "package", "range", "return", "select", "struct", "switch", "type", "var"} {
		if !IsKeyword(k) {
			t.Errorf("IsKeyword(%q) = false", k)
		}
	}
	for _, k := range []string{"", "string", "error", "nil", "true", "len", "Type", "int"} {
		if IsKeyword(k) {
			t.Errorf("IsKeyword(%q) = true", k)
		}
	}
}

func TestEffectConst(t *testing.T) {
	tests := []struct{ in, want string }{{"createSession", "EffectCreateSession"}, {"removeToken", "EffectRemoveToken"}, {"Push", "EffectPush"}}
	for _, tt := range tests {
		if got := EffectConst(tt.in); got != tt.want {
			t.Errorf("EffectConst(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCaseConst(t *testing.T) {
	tests := []struct{ classify, c, want string }{
		{"WarrantyKind", "Yamada", "WarrantyKindYamada"},
		{"WarrantyKind", "standard", "WarrantyKindStandard"},
		{"WarrantyKind", "noWarranty", "WarrantyKindNoWarranty"},
		{"WarrantyKind", "default", "WarrantyKindDefault"},
		{"IsAdult", "Default", "IsAdultDefault"},
	}
	for _, tt := range tests {
		if got := CaseConst(tt.classify, tt.c); got != tt.want {
			t.Errorf("CaseConst(%q, %q) = %q, want %q", tt.classify, tt.c, got, tt.want)
		}
	}
}

func TestPackageName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"server", "server"}, {"server.admin", "admin"}, {"Browser", "browser"}, {"my-api", "myapi"},
		{"a.b.cliV2", "cliv2"}, {"x.日本", "pkg"}, {"x.2fa", "pkg2fa"}, {"x.type", "pkgtype"}, {"", "pkg"},
	}
	for _, tt := range tests {
		if got := PackageName(tt.in); got != tt.want {
			t.Errorf("PackageName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
