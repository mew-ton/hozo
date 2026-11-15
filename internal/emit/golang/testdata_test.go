package golang

import (
	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// このファイルは golden テストの入力。internal/sema と internal/check が作る形の ir.View を
// 手で組み立てる（docs/ir.md「構造」「その名前空間から見た IR」）。Signature と Call.Bound は
// docs/check.md「計算」の結果を手で書いてある。

func sp(file string, line int) ir.Span {
	return ir.Span{File: file, Start: ir.Pos{Line: line, Col: 1}, End: ir.Pos{Line: line, Col: 2}}
}

func doc(s string) *ir.Doc { return &ir.Doc{Summary: s, Body: s} }

func lit(l ir.Literal) ir.ArgValue { return ir.LiteralValue{Literal: l} }

func app(decorator string, args ...ir.Arg) ir.Application {
	return ir.Application{Decorator: decorator, Args: args, Span: sp("x.hozo", 1)}
}

func arg(param string, v ir.ArgValue) ir.Arg { return ir.Arg{Param: param, Value: v} }

func name(path ...string) ir.Atom { return ir.NameAtom{Path: path, Span: sp("x.hozo", 1)} }

func call(op string, args ...ir.CallArg) ir.Call {
	return ir.Call{Op: op, Args: args, Span: sp("x.hozo", 1)}
}

func carg(param string, v ir.Atom) ir.CallArg { return ir.CallArg{Param: param, Value: v} }

func record(fields ...ir.RecordField) ir.Atom {
	return ir.RecordAtom{Fields: fields, Span: sp("x.hozo", 1)}
}

func field(name string, t ir.Type) ir.Field {
	return ir.Field{Name: name, Type: t, Span: sp("x.hozo", 1)}
}

func param(name string, t ir.Type) ir.OpParam {
	return ir.OpParam{Name: name, Type: t, Span: sp("x.hozo", 1)}
}

func base(name string, d *ir.Doc, decos ...ir.Application) ir.DeclBase {
	return ir.DeclBase{Name: name, Description: d, Decorators: decos, Span: sp("x.hozo", 1)}
}

// validVocab は標準の語彙 valid（vocab/valid.hozo）。extra はユーザーが足した制約。
func validVocab(extra ...ir.Decl) ir.Namespace {
	numeric := ir.TypeParam{Types: []ir.Type{ir.Int(), ir.Float()}}
	str := ir.TypeParam{Types: []ir.Type{ir.String()}}
	optInt := ir.TypeParam{Types: []ir.Type{ir.Optional(ir.Int())}}
	targets := []ir.DecoratorTarget{ir.TargetScalar, ir.TargetField}
	decls := []ir.Decl{
		&ir.Decorator{DeclBase: base("valid.min", nil), Params: []ir.Param{{Name: "value", Type: numeric}}, Targets: targets},
		&ir.Decorator{DeclBase: base("valid.max", nil), Params: []ir.Param{{Name: "value", Type: numeric}}, Targets: targets},
		&ir.Decorator{DeclBase: base("valid.length", nil), Params: []ir.Param{{Name: "min", Type: optInt, Optional: true}, {Name: "max", Type: optInt, Optional: true}}, Targets: targets},
		&ir.Decorator{DeclBase: base("valid.pattern", nil), Params: []ir.Param{{Name: "regex", Type: str}}, Targets: targets},
		&ir.Decorator{DeclBase: base("valid.format", nil), Params: []ir.Param{{Name: "name", Type: str}}, Targets: targets},
	}
	return ir.Namespace{Name: "valid", Decls: append(decls, extra...)}
}

// httpNS は標準の語彙 http のうち使う分。
func httpNS() ir.Namespace {
	str := ir.TypeParam{Types: []ir.Type{ir.String()}}
	return ir.Namespace{Name: "http", Decls: []ir.Decl{
		&ir.Exception{DeclBase: base("http.ServerError", doc("サーバー側の失敗（5xx 全般）"))},
		&ir.Exception{DeclBase: base("http.NetworkError", doc("運搬の失敗（ステータスなし）"))},
		&ir.Decorator{DeclBase: base("http.post", nil), Params: []ir.Param{{Name: "path", Type: str}}, Targets: []ir.DecoratorTarget{ir.TargetOp}, External: true,
			Throws: []ir.ThrowsClause{{Exceptions: []string{"http.ServerError", "http.NetworkError"}}}},
		&ir.Decorator{DeclBase: base("http.retry", doc("運搬に失敗したらやり直す。")), Targets: []ir.DecoratorTarget{ir.TargetOp}},
	}}
}

// pageNS は標準の語彙 page のうち使う分。
func pageNS() ir.Namespace {
	str := ir.TypeParam{Types: []ir.Type{ir.String()}}
	return ir.Namespace{Name: "page", Decls: []ir.Decl{
		&ir.Scalar{DeclBase: base("page.Route", doc("ルートの位置（path と params）")), Base: ir.String()},
		&ir.Decorator{DeclBase: base("page.route", nil), Params: []ir.Param{{Name: "path", Type: str}}, Targets: []ir.DecoratorTarget{ir.TargetOp}},
	}}
}

// serverNS は注文サービスの名前空間: model / scalar / union / exception / effect / 葉の op。
func serverNS() ir.Namespace {
	const f = "server.hozo"
	loginSig := ir.Signature{
		Exceptions:     []string{"server.InvalidCredentials"},
		Effects:        []string{"server.createSession"},
		Relations:      []ir.SignatureRelation{{Exception: "server.InvalidCredentials", Effect: "server.createSession", State: ir.Prevented}},
		SuccessEffects: []ir.SuccessEffect{{Effect: "server.createSession", State: ir.Kept}},
	}
	return ir.Namespace{
		Name:        "server",
		Description: doc("注文サービス。\n\n葉の op は人が実装する。"),
		Decls: []ir.Decl{
			&ir.Scalar{DeclBase: base("server.Email", doc("メールアドレス"),
				app("valid.format", arg("name", lit(ir.StringLiteral{Value: "email"}))),
				app("valid.length", arg("max", lit(ir.IntLiteral{Value: 254}))),
			), Base: ir.String()},
			&ir.Scalar{DeclBase: base("server.UserId", nil,
				app("valid.pattern", arg("regex", lit(ir.StringLiteral{Value: "^u_[0-9]+$"}))),
			), Base: ir.String()},
			&ir.Scalar{DeclBase: base("server.PosInt", doc("1 以上の整数"),
				app("valid.min", arg("value", lit(ir.IntLiteral{Value: 1}))),
			), Base: ir.Int()},
			&ir.Scalar{DeclBase: base("server.Token", nil), Base: ir.String()},
			&ir.Scalar{DeclBase: base("server.Ratio", nil,
				app("valid.min", arg("value", lit(ir.FloatLiteral{Value: 0}))),
				app("valid.max", arg("value", lit(ir.FloatLiteral{Value: 1}))),
			), Base: ir.Float()},
			&ir.Model{DeclBase: base("server.User", doc("利用者。")), Fields: []ir.Field{
				field("id", ir.Ref("server.UserId")),
				field("email", ir.Ref("server.Email")),
				{Name: "name", Description: doc("表示名"), Type: ir.String(), Span: sp(f, 1),
					Decorators: []ir.Application{app("valid.length", arg("min", lit(ir.IntLiteral{Value: 1})), arg("max", lit(ir.IntLiteral{Value: 80})))}},
				field("token", ir.Optional(ir.Ref("server.Token"))),
				field("tags", ir.Array(ir.String())),
				field("alts", ir.Array(ir.Ref("server.Email"))),
				field("attrs", ir.Record(ir.String(), ir.Ref("server.Email"))),
				field("joined", ir.Optional(ir.Time())),
			}},
			&ir.Model{DeclBase: base("server.PublicUser", doc("公開する利用者の情報。")), Fields: []ir.Field{
				field("id", ir.Ref("server.UserId")),
				{Name: "name", Description: doc("表示名"), Type: ir.String(), Span: sp(f, 1),
					Decorators: []ir.Application{app("valid.length", arg("min", lit(ir.IntLiteral{Value: 1})), arg("max", lit(ir.IntLiteral{Value: 80})))}},
				field("tags", ir.Array(ir.String())),
			}, Extends: &ir.ModelExtends{Base: "server.User", Removed: []string{"email", "token", "alts", "attrs", "joined"}}},
			&ir.Model{DeclBase: base("server.LoginInput", nil), Fields: []ir.Field{
				field("email", ir.Ref("server.Email")),
				field("password", ir.String()),
			}},
			&ir.Model{DeclBase: base("server.LoginOk", nil), Fields: []ir.Field{
				field("token", ir.Ref("server.Token")),
				field("user", ir.Ref("server.User")),
			}},
			&ir.Model{DeclBase: base("server.GetMeInput", nil), Fields: []ir.Field{
				field("token", ir.Ref("server.Token")),
				field("verbose", ir.Boolean()),
			}},
			&ir.Model{DeclBase: base("server.OrdersQuery", doc("注文の一覧の条件。")), Fields: []ir.Field{
				{Name: "page", Description: doc("ページ番号（1 始まり）"), Type: ir.Ref("server.PosInt"), Default: ir.IntLiteral{Value: 1}, Span: sp(f, 1)},
				field("status", ir.Optional(ir.String())),
				{Name: "timeout", Type: ir.Duration(), Default: ir.DurationLiteral{Value: 30_000_000_000}, Span: sp(f, 1)},
				{Name: "ratio", Type: ir.Optional(ir.Ref("server.Ratio")), Default: ir.FloatLiteral{Value: 0.5}, Span: sp(f, 1)},
			}},
			&ir.Model{DeclBase: base("server.OrderRow", nil), Fields: []ir.Field{
				field("id", ir.String()),
				field("total", ir.Float()),
			}},
			&ir.Union{DeclBase: base("server.OrderPage", doc("注文ページの表示。")), Variant: ir.UnionArms, Arms: []ir.UnionArm{
				{Name: "Ready", Type: ir.Ref("server.OrderPage.Ready"), Span: sp(f, 1)},
				{Name: "Empty", Type: ir.Ref("server.OrderPage.Empty"), Description: doc("注文がない"), Span: sp(f, 1)},
				{Name: "Unavailable", Type: ir.Ref("server.OrdersUnavailable"), Span: sp(f, 1)},
			}},
			&ir.Model{DeclBase: base("server.OrderPage.Ready", nil), SynthesizedFrom: "server.OrderPage", Fields: []ir.Field{
				field("userName", ir.String()),
				field("orders", ir.Array(ir.Ref("server.OrderRow"))),
			}},
			&ir.Model{DeclBase: base("server.OrderPage.Empty", nil), SynthesizedFrom: "server.OrderPage"},
			&ir.Model{DeclBase: base("server.OrdersUnavailable", nil), Fields: []ir.Field{field("reason", ir.String())}},
			&ir.Exception{DeclBase: base("server.InvalidCredentials", doc("メールアドレスかパスワードが一致しない"))},
			&ir.Exception{DeclBase: base("server.Unauthorized", nil)},
			&ir.Exception{DeclBase: base("server.ClientError", doc("入力の検証に失敗した")), Value: ir.Ref("server.ClientError.Value")},
			&ir.Model{DeclBase: base("server.ClientError.Value", nil), SynthesizedFrom: "server.ClientError", Fields: []ir.Field{
				field("violations", ir.Array(ir.String())),
			}},
			&ir.Union{DeclBase: base("server.ApiError", nil), Variant: ir.UnionSet, Members: []ir.UnionMember{
				{Target: "server.Unauthorized", Span: sp(f, 1)}, {Target: "server.ClientError", Span: sp(f, 1)},
			}},
			&ir.State{DeclBase: base("server.sessions", nil), Shape: ir.StateList, Element: ir.Ref("server.Token")},
			&ir.Effect{DeclBase: base("server.createSession", doc("セッションを作る")), Target: ir.BuiltinTarget{State: "server.sessions", Operation: ir.OpPost}, Writes: []string{"server.sessions"}},
			&ir.Effect{DeclBase: base("server.deleteSession", nil), Target: ir.BuiltinTarget{State: "server.sessions", Operation: ir.OpDelete}, Writes: []string{"server.sessions"}},
			&ir.Op{DeclBase: base("server.login", &ir.Doc{Summary: "ログインする。", Body: "ログインする。",
				Params: []ir.DocParam{{Name: "in", Text: "メールアドレスとパスワード"}}},
				app("http.post", arg("path", lit(ir.StringLiteral{Value: "/login"}))),
			), Params: []ir.OpParam{param("in", ir.Ref("server.LoginInput"))}, Returns: ir.Ref("server.LoginOk"),
				Effects:   []string{"server.createSession"},
				Throws:    []ir.ThrowsClause{{Exceptions: []string{"server.InvalidCredentials"}, Relations: []ir.Relation{{Kind: ir.Prevents}}}},
				Signature: loginSig},
			&ir.Op{DeclBase: base("server.getMe", doc("自分の情報。")), Params: []ir.OpParam{param("in", ir.Ref("server.GetMeInput"))}, Returns: ir.Ref("server.PublicUser"),
				Throws:    []ir.ThrowsClause{{Exceptions: []string{"server.Unauthorized"}}},
				Signature: ir.Signature{Exceptions: []string{"server.Unauthorized"}}},
			&ir.Op{DeclBase: base("server.listOrders", nil), Params: []ir.OpParam{param("token", ir.Ref("server.Token")), param("query", ir.Ref("server.OrdersQuery"))},
				Returns:   ir.Array(ir.Ref("server.OrderRow")),
				Throws:    []ir.ThrowsClause{{Exceptions: []string{"server.Unauthorized"}}},
				Signature: ir.Signature{Exceptions: []string{"server.Unauthorized"}}},
			&ir.Op{DeclBase: base("server.logout", nil), Params: []ir.OpParam{param("token", ir.Ref("server.Token"))}, Returns: nil,
				Effects:   []string{"server.deleteSession"},
				Signature: ir.Signature{Effects: []string{"server.deleteSession"}, SuccessEffects: []ir.SuccessEffect{{Effect: "server.deleteSession", State: ir.Kept}}}},
			&ir.Op{DeclBase: base("server.orderPage", doc("注文ページの VM。")), Params: []ir.OpParam{param("me", ir.Ref("server.User")), param("orders", ir.Array(ir.Ref("server.OrderRow")))},
				Returns: ir.Ref("server.OrderPage")},
			&ir.Op{DeclBase: base("server.publicUser", doc("公開する情報だけを取る。")), Params: []ir.OpParam{param("id", ir.Ref("server.UserId"))}, Returns: ir.Ref("server.PublicUser"),
				Throws:    []ir.ThrowsClause{{Exceptions: []string{"server.ClientError"}}},
				Signature: ir.Signature{Exceptions: []string{"server.ClientError"}}},
		},
	}
}

// serverSelect は server の選択: すべての op が起点（状態 sessions は、login と logout の効果の宛先として選ばれる）。
func serverSelect() plugin.Select {
	return plugin.Select{Roots: []string{"server.login", "server.getMe", "server.listOrders", "server.logout", "server.orderPage", "server.publicUser"}}
}

// serverView は server から見た View。
func serverView() *ir.View {
	p := &ir.Program{Version: ir.Version, Namespaces: []ir.Namespace{{Name: ""}, validVocab(), httpNS(), serverNS()}}
	v := &ir.View{Viewer: "server", Config: nsConfig("server"), Namespaces: namespaces(), Files: allFiles(p), Program: p, Ops: map[string]*ir.ViewedSignature{}}
	for _, d := range p.Namespace("server").Decls {
		if op, ok := d.(*ir.Op); ok {
			v.Ops[op.Name] = &ir.ViewedSignature{Signature: op.Signature, Params: op.Params}
		}
	}
	return v
}

// browserNS はブラウザの名前空間: 配線、match、server への bind、デコレータ、classify。
// デコレータのうち、選択（browserSelect）の wraps が引き受けるのは render の @retry と @guardMe、logout の @tracked、
// pickFirst の @http.retry。openMe に付いたものはどれも引き受けられない（印。何も生成しない）。
func browserNS() ir.Namespace {
	const f = "browser.hozo"
	intT := ir.TypeParam{Types: []ir.Type{ir.Int()}}
	return ir.Namespace{
		Name:        "browser",
		Description: doc("ブラウザの画面。"),
		Decls: []ir.Decl{
			&ir.State{DeclBase: base("browser.token", nil), Shape: ir.StateNullable, Element: ir.Ref("server.Token")},
			&ir.Model{DeclBase: base("browser.Location", nil), Fields: []ir.Field{field("route", ir.Ref("page.Route"))}},
			&ir.Model{DeclBase: base("browser.RenderInput", nil), Fields: []ir.Field{
				field("token", ir.Optional(ir.Ref("server.Token"))),
				field("user", ir.Ref("server.PublicUser")),
				field("where", ir.Optional(ir.Ref("browser.Location"))),
			}},
			&ir.Union{DeclBase: base("browser.MeView", nil), Variant: ir.UnionArms, Arms: []ir.UnionArm{
				{Name: "Me", Type: ir.Ref("server.PublicUser"), Span: sp(f, 1)},
				{Name: "Anonymous", Type: ir.Ref("browser.MeView.Anonymous"), Span: sp(f, 1)},
			}},
			&ir.Model{DeclBase: base("browser.MeView.Anonymous", nil), SynthesizedFrom: "browser.MeView"},
			&ir.Classify{DeclBase: base("browser.WarrantyKind", doc("保証の種類。")), Subject: ir.TypeSubject{Type: ir.Ref("server.OrderRow")},
				Cases: []ir.ClassifyCase{
					{Name: "Standard", Description: doc("標準の保証"), Span: sp(f, 1)},
					{Name: "Yamada", Span: sp(f, 1)},
				}, HasDefault: true, DefaultDescription: doc("保証なし")},
			&ir.Classify{DeclBase: base("browser.IsBig", nil), Subject: ir.TypeSubject{Type: ir.Ref("server.OrderRow")},
				Cases: []ir.ClassifyCase{{Name: "IsBig", Span: sp(f, 1)}}, HasDefault: true, FromCondition: true},
			&ir.Decorator{DeclBase: base("browser.retry", doc("失敗したら times 回までやり直す。")), Params: []ir.Param{{Name: "times", Type: intT}}, Targets: []ir.DecoratorTarget{ir.TargetOp}},
			&ir.Decorator{DeclBase: base("browser.tracked", nil), Targets: []ir.DecoratorTarget{ir.TargetOp}},
			&ir.Decorator{DeclBase: base("browser.guarded", nil), Targets: []ir.DecoratorTarget{ir.TargetOp},
				Catches: []ir.CatchesClause{{Exceptions: []string{"server.Unauthorized"}, Mode: ir.Halts, Halts: "browser.redirected"}},
				Throws:  []ir.ThrowsClause{{Exceptions: []string{"$param:on"}, Relations: []ir.Relation{{Kind: ir.Prevents}}}}},
			&ir.Decorator{DeclBase: base("browser.guardMe", nil), Alias: &ir.DecoratorAlias{Target: "browser.guarded", Args: []ir.Arg{arg("on", ir.RefValue{Target: "http.NetworkError"})}}},
			&ir.Effect{DeclBase: base("browser.redirected", nil), Target: ir.BuiltinTarget{State: "browser.token", Operation: ir.OpPut}, Writes: []string{"browser.token"}},
			// 配線: 自分の情報を取って描画し、公開情報を返す。
			&ir.Op{DeclBase: base("browser.openMe", doc("自分のページを開く。"),
				app("page.route", arg("path", lit(ir.StringLiteral{Value: "/me"}))),
				app("browser.retry", arg("times", lit(ir.IntLiteral{Value: 3}))),
				ir.Application{Decorator: "browser.guarded", Via: "browser.guardMe", Args: []ir.Arg{arg("on", ir.RefValue{Target: "http.NetworkError"})}, Span: sp(f, 1)},
				app("browser.tracked"),
			), Params: nil, Returns: ir.Ref("server.PublicUser"),
				Body: &ir.Wiring{Statements: []ir.Statement{
					ir.ConstStmt{Name: "me", Expr: ir.CallExpr{Call: ir.Call{Op: "server.getMe",
						Args:  []ir.CallArg{carg("in", record(ir.RecordField{Name: "verbose", Value: ir.LiteralAtom{Literal: ir.BooleanLiteral{Value: true}, Span: sp(f, 1)}}))},
						Bound: []ir.BoundPosition{{Path: []string{"in", "token"}, State: "browser.token"}}, Span: sp(f, 1)}}, Span: sp(f, 1)},
					ir.CallStmt{Call: ir.Call{Op: "browser.render",
						Args:  []ir.CallArg{carg("in", record(ir.RecordField{Name: "user", Value: name("me")}, ir.RecordField{Name: "where", Value: ir.NoneAtom{Span: sp(f, 1)}}))},
						Bound: []ir.BoundPosition{{Path: []string{"in", "token"}, State: "browser.token"}}, Span: sp(f, 1)}, Span: sp(f, 1)},
					ir.ReturnStmt{Expr: ir.AtomExpr{Atom: name("me")}, Span: sp(f, 1)},
				}},
				Signature: ir.Signature{Exceptions: []string{"http.ServerError", "http.NetworkError"}, Effects: []string{"browser.redirected"}}},
			// 葉。外側から @retry、@guardMe（名前付き。中身は guarded）。包む関数は内側の @guardMe から順に包む。
			&ir.Op{DeclBase: base("browser.render", doc("画面を描く。"),
				app("browser.retry", arg("times", lit(ir.IntLiteral{Value: 2}))),
				ir.Application{Decorator: "browser.guarded", Via: "browser.guardMe", Args: []ir.Arg{arg("on", ir.RefValue{Target: "http.NetworkError"})}, Span: sp(f, 1)},
			), Params: []ir.OpParam{param("in", ir.Ref("browser.RenderInput"))}, Returns: nil},
			// 配線: 注文の一覧から一行を選ぶ（match）。
			&ir.Op{DeclBase: base("browser.chooseLine", nil), Params: []ir.OpParam{param("query", ir.Ref("server.OrdersQuery"))}, Returns: ir.Optional(ir.Ref("server.OrderRow")),
				Body: &ir.Wiring{Statements: []ir.Statement{
					ir.ConstStmt{Name: "rows", Expr: ir.CallExpr{Call: ir.Call{Op: "server.listOrders",
						Args:  []ir.CallArg{carg("query", name("query"))},
						Bound: []ir.BoundPosition{{Path: []string{"token"}, State: "browser.token"}}, Span: sp(f, 1)}}, Span: sp(f, 1)},
					ir.ConstStmt{Name: "first", Expr: ir.CallExpr{Call: call("browser.pickFirst", carg("rows", name("rows")))}, Span: sp(f, 1)},
					ir.ConstStmt{Name: "line", Expr: ir.MatchExpr{Match: ir.Match{Subject: ir.ValueSubject{Atom: name("first")}, Classify: "browser.WarrantyKind", Arms: []ir.MatchArm{
						{Cases: ir.ArmCases{Cases: []string{"Standard"}}, Body: ir.CallExpr{Call: call("browser.standardLine", carg("row", name("first")))}, Span: sp(f, 1)},
						{Cases: ir.ArmCases{Cases: []string{"Yamada"}}, Body: ir.CallExpr{Call: call("browser.yamadaLine", carg("row", name("first")), carg("page", name("query", "page")))}, Span: sp(f, 1)},
						{Cases: ir.DefaultArm(), Body: ir.NoneBody{}, Span: sp(f, 1)},
					}, Span: sp(f, 1)}}, Span: sp(f, 1)},
					ir.MatchStmt{Match: ir.Match{Subject: ir.ValueSubject{Atom: name("first")}, Classify: "browser.IsBig", Arms: []ir.MatchArm{
						{Cases: ir.ArmCases{Cases: []string{"IsBig"}}, Body: ir.CallExpr{Call: call("browser.celebrate")}, Span: sp(f, 1)},
						{Cases: ir.DefaultArm(), Body: ir.NoneBody{}, Span: sp(f, 1)},
					}, Span: sp(f, 1)}, Span: sp(f, 1)},
					ir.ReturnStmt{Expr: ir.AtomExpr{Atom: name("line")}, Span: sp(f, 1)},
				}},
				Signature: ir.Signature{Exceptions: []string{"server.Unauthorized"}}},
			// 葉。別の名前空間のデコレータ（包む関数の名前に名前空間が付く）。
			&ir.Op{DeclBase: base("browser.pickFirst", nil, app("http.retry")), Params: []ir.OpParam{param("rows", ir.Array(ir.Ref("server.OrderRow")))}, Returns: ir.Ref("server.OrderRow")},
			&ir.Op{DeclBase: base("browser.standardLine", nil), Params: []ir.OpParam{param("row", ir.Case("browser.WarrantyKind", []string{"Standard"}, ir.Ref("server.OrderRow")))}, Returns: ir.Ref("server.OrderRow")},
			&ir.Op{DeclBase: base("browser.yamadaLine", nil), Params: []ir.OpParam{param("row", ir.Ref("server.OrderRow")), param("page", ir.Optional(ir.Ref("server.PosInt")))}, Returns: ir.Ref("server.OrderRow")},
			&ir.Op{DeclBase: base("browser.celebrate", nil)},
			// 葉。ユニオンを返す（アームの一つは別のパッケージの model なので、包む型ができる）。
			&ir.Op{DeclBase: base("browser.meView", nil), Returns: ir.Ref("browser.MeView")},
			// 配線: void。最後の呼び出しをそのまま返す。選択の wraps が @tracked を引き受けるので、本体は辿られず、口だけが出る
			// （clearToken と server.logout は出力の対象にならない）。
			&ir.Op{DeclBase: base("browser.logout", nil, app("browser.tracked")), Returns: nil,
				Body: &ir.Wiring{Statements: []ir.Statement{
					ir.CallStmt{Call: ir.Call{Op: "server.logout", Bound: []ir.BoundPosition{{Path: []string{"token"}, State: "browser.token"}}, Span: sp(f, 1)}, Span: sp(f, 1)},
					ir.CallStmt{Call: call("browser.clearToken"), Span: sp(f, 1)},
				}},
				Signature: ir.Signature{Effects: []string{"server.deleteSession"}}},
			&ir.Op{DeclBase: base("browser.clearToken", nil)},
			// 配線: return call。呼び出しの型がそのまま戻り値の型（model から model への射影はない）。
			&ir.Op{DeclBase: base("browser.fetchMe", nil), Params: []ir.OpParam{param("in", ir.Ref("server.GetMeInput"))}, Returns: ir.Ref("server.PublicUser"),
				Body: &ir.Wiring{Statements: []ir.Statement{
					ir.ReturnStmt{Expr: ir.CallExpr{Call: ir.Call{Op: "server.getMe", Args: []ir.CallArg{carg("in", name("in"))},
						Bound: []ir.BoundPosition{{Path: []string{"in", "token"}, State: "browser.token"}}, Span: sp(f, 1)}}, Span: sp(f, 1)},
				}},
				Signature: ir.Signature{Exceptions: []string{"server.Unauthorized"}}},
		},
	}
}

// browserProgram は server と browser を含む Program。binds は browser.token を server.** と browser に。
func browserProgram() *ir.Program {
	return &ir.Program{Version: ir.Version,
		Namespaces: []ir.Namespace{{Name: ""}, validVocab(), httpNS(), pageNS(), serverNS(), browserNS()},
		Binds: []ir.Bind{
			{Namespace: "browser", State: "browser.token", Target: ir.BindTarget{Namespace: "server", Deep: true}, Span: sp("browser.hozo", 1)},
			{Namespace: "browser", State: "browser.token", Target: ir.BindTarget{Namespace: "browser"}, Span: sp("browser.hozo", 1)},
		},
	}
}

// browserView は browser から見た View。server の op は bind で token の位置が消えている。
func browserView() *ir.View {
	p := browserProgram()
	ns := p.Namespace("browser")
	// bind で合成した model（docs/ir.md「その名前空間から見た IR」）を browser の末尾に足す。
	ns.Decls = append(ns.Decls, &ir.Model{
		DeclBase: base("browser.getMe$in", nil), SynthesizedFrom: "server.getMe",
		Fields: []ir.Field{field("verbose", ir.Boolean())},
	})
	v := &ir.View{Viewer: "browser", Config: nsConfig("browser", "router"), Namespaces: namespaces(), Files: allFiles(p), Program: p, Ops: map[string]*ir.ViewedSignature{}}
	for _, d := range ns.Decls {
		if op, ok := d.(*ir.Op); ok {
			v.Ops[op.Name] = &ir.ViewedSignature{Signature: op.Signature, Params: op.Params}
		}
	}
	for _, d := range p.Namespace("server").Decls {
		if op, ok := d.(*ir.Op); ok {
			v.Ops[op.Name] = &ir.ViewedSignature{Signature: op.Signature, Params: op.Params}
		}
	}
	v.Ops["server.login"].Exceptions = []string{"server.InvalidCredentials", "http.ServerError", "http.NetworkError"}
	v.Ops["server.getMe"].Params = []ir.OpParam{param("in", ir.Ref("browser.getMe$in"))}
	v.Ops["server.getMe"].Bound = []ir.BoundPosition{{Path: []string{"in", "token"}, State: "browser.token"}}
	v.Ops["server.listOrders"].Params = []ir.OpParam{param("query", ir.Ref("server.OrdersQuery"))}
	v.Ops["server.listOrders"].Bound = []ir.BoundPosition{{Path: []string{"token"}, State: "browser.token"}}
	v.Ops["server.logout"].Params = nil
	v.Ops["server.logout"].Bound = []ir.BoundPosition{{Path: []string{"token"}, State: "browser.token"}}
	return v
}

// browserServerView は browser のテストで一緒に出す server の View（同じ Program）。
func browserServerView() *ir.View {
	v := serverView()
	v.Program = browserProgram()
	return v
}

// browserSelect は browser の選択（プラグイン router が返したものとして）: 起点は配線の openMe / chooseLine / fetchMe と、葉の meView。
// wraps は render の @retry と @guardMe（名前付きデコレータの FQN で指す）、logout の @tracked、pickFirst の @http.retry。
func browserSelect() plugin.Select {
	return plugin.Select{
		Roots: []string{"browser.openMe", "browser.chooseLine", "browser.fetchMe", "browser.meView"},
		Wraps: []plugin.Wrap{
			{Op: "browser.render", Decorator: "browser.retry"},
			{Op: "browser.render", Decorator: "browser.guardMe"},
			{Op: "browser.logout", Decorator: "browser.tracked"},
			{Op: "browser.pickFirst", Decorator: "http.retry"},
		},
	}
}

// constraintView はユーザーが valid に足した制約と scalar の継承。
func constraintView() *ir.View {
	intT := ir.TypeParam{Types: []ir.Type{ir.Int()}}
	valid := validVocab(
		&ir.Decorator{DeclBase: base("valid.checkDigit", doc("検査数字が合う")), Targets: []ir.DecoratorTarget{ir.TargetScalar, ir.TargetField}},
		&ir.Decorator{DeclBase: base("valid.multipleOf", nil), Params: []ir.Param{{Name: "base", Type: intT}}, Targets: []ir.DecoratorTarget{ir.TargetScalar, ir.TargetField}},
	)
	inv := ir.Namespace{Name: "inventory", Decls: []ir.Decl{
		&ir.Scalar{DeclBase: base("inventory.Isbn", doc("ISBN。"),
			app("valid.length", arg("min", lit(ir.IntLiteral{Value: 10})), arg("max", lit(ir.IntLiteral{Value: 13}))),
			app("valid.checkDigit"),
		), Base: ir.String()},
		&ir.Scalar{DeclBase: base("inventory.Sku", doc("ISBN に接頭辞を足した SKU。"),
			app("valid.pattern", arg("regex", lit(ir.StringLiteral{Value: `^[A-Z]`}))),
		), Base: ir.Ref("inventory.Isbn")},
		&ir.Scalar{DeclBase: base("inventory.Qty", nil,
			app("valid.multipleOf", arg("base", lit(ir.IntLiteral{Value: 6}))),
			app("valid.max", arg("value", lit(ir.IntLiteral{Value: 120}))),
		), Base: ir.Int()},
		&ir.Model{DeclBase: base("inventory.Item", nil), Fields: []ir.Field{
			field("isbn", ir.Ref("inventory.Isbn")),
			field("qty", ir.Ref("inventory.Qty")),
			{Name: "note", Type: ir.Optional(ir.String()), Span: sp("x.hozo", 1),
				Decorators: []ir.Application{app("valid.length", arg("max", lit(ir.IntLiteral{Value: 10})))}},
			{Name: "code", Type: ir.String(), Span: sp("x.hozo", 1),
				Decorators: []ir.Application{app("valid.checkDigit")}},
			field("alts", ir.Array(ir.Ref("inventory.Sku"))),
		}},
		// 型は op から辿って選ばれる（docs/emit-go.md「出力の対象」の 6）。
		&ir.Op{DeclBase: base("inventory.put", nil), Params: []ir.OpParam{param("item", ir.Ref("inventory.Item"))}},
	}}
	p := &ir.Program{Version: ir.Version, Namespaces: []ir.Namespace{{Name: ""}, valid, inv}}
	return &ir.View{Viewer: "inventory", Config: nsConfig("inventory"), Namespaces: namespaces(), Files: allFiles(p), Program: p, Ops: map[string]*ir.ViewedSignature{}}
}

// constraintSelect は inventory の選択: put が起点。
func constraintSelect() plugin.Select { return plugin.Select{Roots: []string{"inventory.put"}} }

// derivedView は継承した model（Model.extends。docs/emit-go.md「移行の間の扱い」）: ほかの model と同じ別々の構造体で、
// 元の model からの変換関数は出さない。別の model の値は、写す op（人が中身を書く葉の op）を呼んで作る。
func derivedView() *ir.View {
	cat := ir.Namespace{Name: "catalog", Decls: []ir.Decl{
		&ir.Model{DeclBase: base("catalog.Product", nil), Fields: []ir.Field{
			field("id", ir.String()),
			field("name", ir.String()),
			field("price", ir.Float()),
			field("secret", ir.String()),
			{Name: "stock", Type: ir.Int(), Default: ir.IntLiteral{Value: 0}, Span: sp("x.hozo", 1)},
			field("note", ir.Optional(ir.String())),
		}},
		&ir.Model{DeclBase: base("catalog.PublicProduct", nil), Fields: []ir.Field{
			field("id", ir.String()),
			field("name", ir.String()),
			field("price", ir.Float()),
			{Name: "stock", Type: ir.Int(), Default: ir.IntLiteral{Value: 0}, Span: sp("x.hozo", 1)},
			field("note", ir.Optional(ir.String())),
		}, Extends: &ir.ModelExtends{Base: "catalog.Product", Removed: []string{"secret"}}},
		&ir.Model{DeclBase: base("catalog.Summary", nil), Fields: []ir.Field{
			field("id", ir.String()),
			field("note", ir.Optional(ir.String())),
			field("name", ir.Optional(ir.String())),
			{Name: "featured", Type: ir.Boolean(), Default: ir.BooleanLiteral{Value: false}, Span: sp("x.hozo", 1)},
		}},
		&ir.Op{DeclBase: base("catalog.load", nil), Params: []ir.OpParam{param("id", ir.String())}, Returns: ir.Ref("catalog.Product"),
			Reads: []string{"catalog.products"}, Signature: ir.Signature{Reads: []string{"catalog.products"}}},
		// 写す op: 仕様の上では普通の葉の op で、人が中身を書く
		&ir.Op{DeclBase: base("catalog.toPublic", nil), Params: []ir.OpParam{param("p", ir.Ref("catalog.Product"))}, Returns: ir.Ref("catalog.PublicProduct"),
			Signature: ir.Signature{Pure: true}},
		&ir.Op{DeclBase: base("catalog.toSummary", nil), Params: []ir.OpParam{param("p", ir.Ref("catalog.Product"))}, Returns: ir.Ref("catalog.Summary"),
			Signature: ir.Signature{Pure: true}},
		// export は見ない。reads は関数の型のコメントの Reads: の行になる
		&ir.Op{DeclBase: base("catalog.publish", nil), Exported: true, Params: []ir.OpParam{param("id", ir.String())}, Returns: ir.Ref("catalog.PublicProduct"),
			Body: &ir.Wiring{Statements: []ir.Statement{
				ir.ConstStmt{Name: "p", Expr: ir.CallExpr{Call: call("catalog.load", carg("id", name("id")))}, Span: sp("x.hozo", 1)},
				ir.ReturnStmt{Expr: ir.CallExpr{Call: call("catalog.toPublic", carg("p", name("p")))}, Span: sp("x.hozo", 1)},
			}},
			Signature: ir.Signature{Reads: []string{"catalog.products"}}},
		&ir.Op{DeclBase: base("catalog.summarize", nil), Params: []ir.OpParam{param("p", ir.Ref("catalog.Product"))}, Returns: ir.Ref("catalog.Summary"),
			Body: &ir.Wiring{Statements: []ir.Statement{
				ir.ReturnStmt{Expr: ir.CallExpr{Call: call("catalog.toSummary", carg("p", name("p")))}, Span: sp("x.hozo", 1)},
			}},
			Signature: ir.Signature{Pure: true}},
	}}
	p := &ir.Program{Version: ir.Version, Namespaces: []ir.Namespace{{Name: ""}, cat}}
	v := &ir.View{Viewer: "catalog", Config: nsConfig("catalog"), Namespaces: namespaces(), Files: allFiles(p), Program: p, Ops: map[string]*ir.ViewedSignature{}}
	for _, d := range cat.Decls {
		if op, ok := d.(*ir.Op); ok {
			v.Ops[op.Name] = &ir.ViewedSignature{Signature: op.Signature, Params: op.Params}
		}
	}
	return v
}

// derivedSelect は catalog の選択: 配線の publish と summarize が起点（load は publish から辿られる）。
func derivedSelect() plugin.Select {
	return plugin.Select{Roots: []string{"catalog.publish", "catalog.summarize"}}
}

// ---- 状態を分類で表す決定（docs/decisions.md「設計書を変える決定」S1〜S6）の出力 ----

// stateArm は match のアーム一つ。body が nil なら none。
func stateArm(body *ir.Call, cases ...string) ir.MatchArm {
	a := ir.MatchArm{Cases: ir.ArmCases{Cases: cases}, Body: ir.NoneBody{}, Span: sp("x.hozo", 1)}
	if body != nil {
		a.Body = ir.CallExpr{Call: *body}
	}
	return a
}

// defaultArm は default のアーム。body が nil なら none。
func defaultArm(body *ir.Call) ir.MatchArm {
	a := stateArm(body)
	a.Cases = ir.DefaultArm()
	return a
}

func callp(op string, args ...ir.CallArg) *ir.Call {
	c := call(op, args...)
	return &c
}

// byState は基底の状態の、組み込みの分類での match（`match s by list` / `by nullable`）。
func byState(state string, shape ir.StateShape, arms ...ir.MatchArm) ir.Match {
	return ir.Match{Subject: ir.ValueSubject{Atom: ir.StateAtom{State: state, Span: sp("x.hozo", 1)}}, Classify: string(shape), Arms: arms, Span: sp("x.hozo", 1)}
}

// byClassify は状態の分類での match（`match by C`）。
func byClassify(classify string, arms ...ir.MatchArm) ir.Match {
	return ir.Match{Subject: ir.ClassifyOwnedSubject{}, Classify: classify, Arms: arms, Span: sp("x.hozo", 1)}
}

func wiring(stmts ...ir.Statement) *ir.Wiring { return &ir.Wiring{Statements: stmts} }

func matchStmt(m ir.Match) ir.Statement { return ir.MatchStmt{Match: m, Span: sp("x.hozo", 1)} }

func constMatch(name string, m ir.Match) ir.Statement {
	return ir.ConstStmt{Name: name, Expr: ir.MatchExpr{Match: m}, Span: sp("x.hozo", 1)}
}

func returnName(path ...string) ir.Statement {
	return ir.ReturnStmt{Expr: ir.AtomExpr{Atom: name(path...)}, Span: sp("x.hozo", 1)}
}

func effectsSig(effects ...string) ir.Signature { return ir.Signature{Effects: effects} }

// shopNS は店の名前空間: 三つの形の基底の状態、状態の分類（default なし）と condition（default あり）、値の分類、
// 組み込みの効果と遷移の効果、状態での match（match by C と match s by list / by nullable）の配線。
// 選択（stateSelect）は配線を起点にする。状態は、配線の match と bind、葉の効果の宛先から選ばれ、title だけは states で選ぶ。
// 値の分類 LineKind はどの配線の match にも使われないので、出力の対象にならない。
func shopNS() ir.Namespace {
	return ir.Namespace{
		Name:        "shop",
		Description: doc("カートと注文を持つ店。"),
		Decls: []ir.Decl{
			&ir.Scalar{DeclBase: base("shop.Token", doc("ログインセッションのトークン")), Base: ir.String()},
			&ir.Model{DeclBase: base("shop.Line", nil), Fields: []ir.Field{field("item", ir.String())}},
			&ir.Model{DeclBase: base("shop.Order", nil), Fields: []ir.Field{field("id", ir.String())}},
			&ir.Model{DeclBase: base("shop.MeInput", nil), Fields: []ir.Field{field("token", ir.Ref("shop.Token")), field("verbose", ir.Boolean())}},
			&ir.Model{DeclBase: base("shop.Profile", nil), Fields: []ir.Field{field("name", ir.String())}},

			&ir.State{DeclBase: base("shop.carts", doc("カートの行")), Shape: ir.StateList, Element: ir.Ref("shop.Line")},
			&ir.State{DeclBase: base("shop.orders", doc("注文")), Shape: ir.StateList, Element: ir.Ref("shop.Order")},
			&ir.State{DeclBase: base("shop.token", doc("手元のトークン")), Shape: ir.StateNullable, Element: ir.Ref("shop.Token")},
			// single は組み込みの分類を持たない（問い合わせは Get だけ）。
			&ir.State{DeclBase: base("shop.title", doc("店の名前")), Shape: ir.StateSingle, Element: ir.String()},

			// 状態の分類。default なし。
			&ir.Classify{DeclBase: base("shop.Checkout", doc("買い物の進み具合")), Subject: ir.StatesSubject{States: []string{"shop.carts", "shop.orders"}},
				Cases: []ir.ClassifyCase{
					{Name: "Shopping", Description: doc("カートに入れている途中"), Span: sp("x.hozo", 1)},
					{Name: "Ordered", Description: doc("注文を出した"), Span: sp("x.hozo", 1)},
				}},
			// condition の糖衣の状態の分類。default あり。
			&ir.Classify{DeclBase: base("shop.LoggedIn", doc("ログインしている")), Subject: ir.StatesSubject{States: []string{"shop.token"}},
				Cases: []ir.ClassifyCase{{Name: "LoggedIn", Span: sp("x.hozo", 1)}}, HasDefault: true, FromCondition: true},
			// 値の分類。どの配線の match にも使われない。
			&ir.Classify{DeclBase: base("shop.LineKind", nil), Subject: ir.TypeSubject{Type: ir.Ref("shop.Line")},
				Cases: []ir.ClassifyCase{{Name: "Single", Span: sp("x.hozo", 1)}, {Name: "Bulk", Span: sp("x.hozo", 1)}}},

			// 組み込みの効果。
			&ir.Effect{DeclBase: base("shop.addLine", doc("カートに行を足す")), Target: ir.BuiltinTarget{State: "shop.carts", Operation: ir.OpPost}, Writes: []string{"shop.carts"}},
			// 状態の分類の遷移。説明があっても遷移はコメントに出る。
			&ir.Effect{DeclBase: base("shop.placeOrder", doc("注文を確定する")),
				Target: ir.TransitionTarget{Of: "shop.Checkout", From: ir.Cases("Shopping"), To: ir.Cases("Ordered")}, Writes: []string{"shop.carts", "shop.orders"}},
			// 説明のない遷移。CaseSet の any。
			&ir.Effect{DeclBase: base("shop.startOver", nil),
				Target: ir.TransitionTarget{Of: "shop.Checkout", From: ir.AnyCase(), To: ir.Cases("Shopping")}, Writes: []string{"shop.carts", "shop.orders"}},
			// ケースの和。
			&ir.Effect{DeclBase: base("shop.touchCheckout", doc("進み具合を記録し直す")),
				Target: ir.TransitionTarget{Of: "shop.Checkout", From: ir.Cases("Shopping", "Ordered"), To: ir.AnyCase()}, Writes: []string{"shop.carts", "shop.orders"}},
			// 基底の状態の組み込みの分類の遷移。
			&ir.Effect{DeclBase: base("shop.clearCart", doc("カートを空にする")),
				Target: ir.TransitionTarget{Of: "shop.carts", From: ir.Cases(ir.CaseHasItems), To: ir.Cases(ir.CaseEmpty)}, Writes: []string{"shop.carts"}},

			// 葉。
			&ir.Op{DeclBase: base("shop.submitOrder", nil), Returns: ir.Ref("shop.Order"), Effects: []string{"shop.placeOrder"}, Signature: effectsSig("shop.placeOrder")},
			&ir.Op{DeclBase: base("shop.emptyCart", nil), Effects: []string{"shop.clearCart"}, Signature: effectsSig("shop.clearCart")},
			&ir.Op{DeclBase: base("shop.showLogin", nil)},
			&ir.Op{DeclBase: base("shop.showCart", nil)},
			&ir.Op{DeclBase: base("shop.showEmpty", nil)},
			&ir.Op{DeclBase: base("shop.showOrders", nil)},
			&ir.Op{DeclBase: base("shop.sampleLine", nil), Returns: ir.Ref("shop.Line")},
			&ir.Op{DeclBase: base("shop.firstLine", nil), Returns: ir.Ref("shop.Line")},
			&ir.Op{DeclBase: base("shop.me", doc("自分の情報。token は bind で埋まる。")), Params: []ir.OpParam{param("in", ir.Ref("shop.MeInput"))}, Returns: ir.Ref("shop.Profile")},

			// match by C の値の形: none のアームがあるので optional。
			&ir.Op{DeclBase: base("shop.checkout", doc("注文を出す。もう出していれば何もしない。")), Returns: ir.Optional(ir.Ref("shop.Order")),
				Body: wiring(
					constMatch("placed", byClassify("shop.Checkout",
						stateArm(callp("shop.submitOrder"), "Shopping"),
						stateArm(nil, "Ordered"))),
					returnName("placed"),
				), Signature: effectsSig("shop.placeOrder")},
			// match by C の文の形。default のアーム（condition）と、ケースを並べたアーム。
			&ir.Op{DeclBase: base("shop.open", nil),
				Body: wiring(
					matchStmt(byClassify("shop.LoggedIn",
						stateArm(callp("shop.showCart"), "LoggedIn"),
						defaultArm(callp("shop.showLogin")))),
					matchStmt(byClassify("shop.Checkout",
						stateArm(callp("shop.showOrders"), "Shopping", "Ordered"))),
				)},
			// match s by nullable の値の形: none のアームがあるので optional。HasItems のアームの呼び出しは bind で token が埋まる
			// （アームの中で States から取る）。
			&ir.Op{DeclBase: base("shop.who", nil), Returns: ir.Optional(ir.Ref("shop.Profile")),
				Body: wiring(
					constMatch("profile", byState("shop.token", ir.StateNullable,
						stateArm(nil, ir.CaseEmpty),
						stateArm(&ir.Call{Op: "shop.me",
							Args:  []ir.CallArg{carg("in", record(ir.RecordField{Name: "verbose", Value: ir.LiteralAtom{Literal: ir.BooleanLiteral{Value: true}, Span: sp("x.hozo", 1)}}))},
							Bound: []ir.BoundPosition{{Path: []string{"in", "token"}, State: "shop.token"}}, Span: sp("x.hozo", 1)}, ir.CaseHasItems))),
					returnName("profile"),
				)},
			// match s by list の文の形。
			&ir.Op{DeclBase: base("shop.openCart", nil),
				Body: wiring(
					matchStmt(byState("shop.carts", ir.StateList,
						stateArm(callp("shop.showEmpty"), ir.CaseEmpty),
						stateArm(callp("shop.emptyCart"), ir.CaseHasItems))),
				), Signature: effectsSig("shop.clearCart")},
			// match s by list の値の形（none がないので optional にならない）。アームは HasItems が先に書かれている。
			&ir.Op{DeclBase: base("shop.pick", nil), Returns: ir.Ref("shop.Line"),
				Body: wiring(
					ir.ReturnStmt{Expr: ir.MatchExpr{Match: byState("shop.carts", ir.StateList,
						stateArm(callp("shop.firstLine"), ir.CaseHasItems),
						stateArm(callp("shop.sampleLine"), ir.CaseEmpty))}, Span: sp("x.hozo", 1)},
				)},
			// 文の形で片方のアームが none（その枝は書かない）、両方が none（述語だけ呼ぶ）、一つのアームに両方のケース（分岐しない）。
			&ir.Op{DeclBase: base("shop.refresh", nil),
				Body: wiring(
					matchStmt(byState("shop.carts", ir.StateList,
						stateArm(nil, ir.CaseEmpty),
						stateArm(callp("shop.showCart"), ir.CaseHasItems))),
					matchStmt(byState("shop.token", ir.StateNullable,
						stateArm(callp("shop.showLogin"), ir.CaseEmpty),
						stateArm(nil, ir.CaseHasItems))),
					matchStmt(byState("shop.orders", ir.StateList,
						stateArm(nil, ir.CaseEmpty),
						stateArm(nil, ir.CaseHasItems))),
					matchStmt(byState("shop.orders", ir.StateList,
						stateArm(callp("shop.showOrders"), ir.CaseEmpty, ir.CaseHasItems))),
					constMatch("line", byState("shop.carts", ir.StateList,
						stateArm(callp("shop.firstLine"), ir.CaseEmpty, ir.CaseHasItems))),
				)},
		},
	}
}

// depotNS は倉庫の名前空間: list / nullable の状態と状態の分類（default あり）。選ばれる状態は、配線が match で読む
// 分類 Level と、葉の order の効果の宛先 items。lock はどこからも選ばれず、Level の対象でも出力されない。
func depotNS() ir.Namespace {
	return ir.Namespace{
		Name: "depot",
		Decls: []ir.Decl{
			&ir.State{DeclBase: base("depot.items", nil), Shape: ir.StateList, Element: ir.String()},
			&ir.State{DeclBase: base("depot.lock", nil), Shape: ir.StateNullable, Element: ir.String()},
			&ir.Classify{DeclBase: base("depot.Level", nil), Subject: ir.StatesSubject{States: []string{"depot.items", "depot.lock"}},
				Cases:      []ir.ClassifyCase{{Name: "Low", Span: sp("x.hozo", 1)}, {Name: "High", Span: sp("x.hozo", 1)}},
				HasDefault: true, DefaultDescription: doc("鍵がかかっていて数えられない")},
			&ir.Effect{DeclBase: base("depot.restock", nil), Target: ir.BuiltinTarget{State: "depot.items", Operation: ir.OpPost}, Writes: []string{"depot.items"}},
			&ir.Effect{DeclBase: base("depot.drain", nil),
				Target: ir.TransitionTarget{Of: "depot.Level", From: ir.Cases("High"), To: ir.Cases("Low")}, Writes: []string{"depot.items", "depot.lock"}},
			&ir.Effect{DeclBase: base("depot.unlock", nil),
				Target: ir.TransitionTarget{Of: "depot.lock", From: ir.Cases(ir.CaseHasItems), To: ir.Cases(ir.CaseEmpty)}, Writes: []string{"depot.lock"}},
			&ir.Op{DeclBase: base("depot.count", nil), Returns: ir.Int()},
			&ir.Op{DeclBase: base("depot.order", nil), Effects: []string{"depot.restock"}, Signature: effectsSig("depot.restock")},
			&ir.Op{DeclBase: base("depot.report", nil), Returns: ir.Int(),
				Body: wiring(
					matchStmt(byClassify("depot.Level",
						stateArm(callp("depot.order"), "Low"),
						stateArm(nil, "High"),
						defaultArm(nil))),
					ir.ReturnStmt{Expr: ir.CallExpr{Call: call("depot.count")}, Span: sp("x.hozo", 1)},
				), Signature: effectsSig("depot.restock")},
		},
	}
}

// stateSelect は state のテストの選択: 配線が起点。shop は single の状態 title を states で選ぶ。
func stateSelect(viewer string) plugin.Select {
	if viewer == "depot" {
		return plugin.Select{Roots: []string{"depot.report"}}
	}
	return plugin.Select{
		Roots:  []string{"shop.checkout", "shop.open", "shop.who", "shop.openCart", "shop.pick", "shop.refresh"},
		States: []string{"shop.title"},
	}
}

// stateNamespaces は state のテストの View.Namespaces。
func stateNamespaces() map[string][]ir.NamespaceConfig {
	return map[string][]ir.NamespaceConfig{"shop": {nsConfig("shop")}, "depot": {nsConfig("depot")}}
}

// stateView は viewer（shop か depot）から見た View。bind は shop.token を shop の op に。
func stateView(viewer string) *ir.View {
	p := &ir.Program{Version: ir.Version,
		Namespaces: []ir.Namespace{{Name: ""}, validVocab(), shopNS(), depotNS()},
		Binds:      []ir.Bind{{Namespace: "shop", State: "shop.token", Target: ir.BindTarget{Namespace: "shop"}, Span: sp("x.hozo", 1)}},
	}
	v := &ir.View{Viewer: viewer, Config: nsConfig(viewer), Namespaces: stateNamespaces(), Files: allFiles(p), Program: p, Ops: map[string]*ir.ViewedSignature{}}
	for _, d := range p.Namespace(viewer).Decls {
		if op, ok := d.(*ir.Op); ok {
			v.Ops[op.Name] = &ir.ViewedSignature{Signature: op.Signature, Params: op.Params}
		}
	}
	return v
}
