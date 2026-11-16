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
		{"server.ClientError.Value", "server", "ClientErrorValue"},
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
	// 大文字化だけでは、当たる名前を見つけない（GO-2 は基底のプラグイン）。
	if FieldName("id") != FieldName("Id") {
		t.Error("id and Id must map to the same name so the base can detect GO-2")
	}
}

func TestMethodName(t *testing.T) {
	tests := []struct{ in, want string }{{"login", "Login"}, {"openOrders", "OpenOrders"}, {"go", "Go"}, {"Login", "Login"}, {"server.login", "Login"}}
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
		// 事前宣言の識別子（docs/plugin-go.md「名前」の「局所の名前」）
		{"error", "error_"}, {"nil", "nil_"}, {"len", "len_"}, {"new", "new_"}, {"int64", "int64_"},
		{"errors", "errors"}, {"Error", "Error"},
		// 生成したコードが使う名前（呼ぶ側が reserved で渡す）: o、ctx、err、ptr、import するパッケージの名前
		{"o", "o_"}, {"ctx", "ctx_"}, {"err", "err_"}, {"ptr", "ptr_"}, {"time", "time_"}, {"pkgtype", "pkgtype_"}, {"body", "body"},
	}
	reserved := map[string]bool{"o": true, "ctx": true, "err": true, "ptr": true, "time": true, "pkgtype": true}
	for _, tt := range tests {
		if got := LocalName(tt.in, reserved); got != tt.want {
			t.Errorf("LocalName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := LocalName("ctx", nil); got != "ctx" {
		t.Errorf("LocalName(ctx, nil) = %q（reserved がなければ、予約語と事前宣言の識別子だけを避ける）", got)
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
	tests := []struct{ in, want string }{{"createSession", "EffectCreateSession"}, {"removeToken", "EffectRemoveToken"}, {"Push", "EffectPush"}, {"store.createUser", "EffectCreateUser"}}
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

// docs/plugin-go.md「名前」の「状態の単位」。
func TestStateUnitName(t *testing.T) {
	tests := []struct{ in, want string }{{"users", "UsersState"}, {"store.users", "UsersState"}, {"hozoc.OutputsSync", "OutputsSyncState"}, {"loginBody", "LoginBodyState"}}
	for _, tt := range tests {
		if got := StateUnitName(tt.in); got != tt.want {
			t.Errorf("StateUnitName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// docs/plugin-go.md「名前」の「別の名前空間の口」と「New」の引数の名前。
func TestDepName(t *testing.T) {
	tests := []struct{ ns, typ, param string }{
		{"store", "StoreOps", "store"},
		{"shop.admin", "ShopAdminOps", "shopAdmin"},
		{"Shop.admin.audit", "ShopAdminAuditOps", "shopAdminAudit"},
		{"type", "TypeOps", "type_"}, // Go の予約語
		{"error", "ErrorOps", "error_"},
		{"mailer", "MailerOps", "mailer"},
	}
	for _, tt := range tests {
		if typ, param := DepName(tt.ns); typ != tt.typ || param != tt.param {
			t.Errorf("DepName(%q) = %q, %q, want %q, %q", tt.ns, typ, param, tt.typ, tt.param)
		}
	}
}

// docs/plugin-go.md「名前」の「継ぎ目のメソッド」と「継ぎ目を持つ op の、中身のメソッド」。
func TestSeamNames(t *testing.T) {
	for _, tt := range []struct{ op, decorator, want string }{
		{"login", "rate.limited", "LoginRateLimited"},
		{"server.login", "rate.limited", "LoginRateLimited"},
		{"foo", "trace.span", "FooTraceSpan"},
		{"loginRate", "limited", "LoginRateLimited"}, // op login の @rate.limited と当たる（GO-2 は基底のプラグイン）
	} {
		if got := SeamMethod(tt.op, tt.decorator); got != tt.want {
			t.Errorf("SeamMethod(%q, %q) = %q, want %q", tt.op, tt.decorator, got, tt.want)
		}
	}
	for _, tt := range []struct{ op, want string }{{"login", "loginBody"}, {"server.Login", "loginBody"}, {"getMe", "getMeBody"}} {
		if got := BodyName(tt.op); got != tt.want {
			t.Errorf("BodyName(%q) = %q, want %q", tt.op, got, tt.want)
		}
	}
}

func TestIsPredeclared(t *testing.T) {
	for _, k := range []string{"error", "string", "nil", "len", "any", "true", "iota", "min"} {
		if !IsPredeclared(k) {
			t.Errorf("IsPredeclared(%q) = false", k)
		}
	}
	for _, k := range []string{"", "type", "Error", "ctx"} {
		if IsPredeclared(k) {
			t.Errorf("IsPredeclared(%q) = true", k)
		}
	}
}
