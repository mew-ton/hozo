package check

import (
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// logoutProgram は設計書 §9 の `browser.logout` の例（docs/testing.md「単体テスト」）。
// §5.2 の browser.token と §5.3 の bind、§8.1 の ignoreFailure、http の語彙の post を含む。
func logoutProgram() *ir.Program {
	ignoreFailure := decorator("browser.ignoreFailure")
	ignoreFailure.Catches = []ir.CatchesClause{catches(ir.Ignores, "", "http.NetworkError", "http.ServerError")}

	serverLogout := leaf("server.logout", nil, param("in", ir.Ref("server.LogoutInput")))
	serverLogout.Effects = ir.NamedEffects("server.deleteSession")
	serverLogout.Decorators = []ir.Application{app("http.post", ir.Arg{Param: "path", Value: ir.LiteralValue{Literal: ir.StringLiteral{Value: "/logout"}}})}

	clearToken := leaf("browser.clearToken", nil)
	clearToken.Effects = ir.NamedEffects("browser.removeToken")

	requestLogout := wire("browser.requestLogout", nil, nil, cs(call("server.logout", carg("in", rec()))))
	requestLogout.Decorators = []ir.Application{app("browser.ignoreFailure")}

	logout := wire("browser.logout", nil, nil, cs(call("browser.requestLogout")), cs(call("browser.clearToken")))

	return program(
		[]ir.Bind{bind("browser", "browser.token", "server", false)},
		httpVocab(),
		namespace("server",
			scalar("server.Token", ir.String()),
			model("server.Session", field("token", ir.Ref("server.Token")), field("userId", ir.String())),
			state("server.sessions", ir.StateList, ir.Ref("server.Session")),
			eff("server.deleteSession", "server.sessions", ir.OpDelete),
			model("server.LogoutInput", field("token", ir.Optional(ir.Ref("server.Token")))),
			serverLogout,
		),
		namespace("browser",
			// 空になりうる状態は nullable<T>、空にするのは delete（docs/decisions.md「設計書から離れた決定」S2）
			state("browser.token", ir.StateNullable, ir.Ref("server.Token")),
			eff("browser.removeToken", "browser.token", ir.OpDelete),
			clearToken,
			ignoreFailure,
			requestLogout,
			logout,
		),
	)
}

// TestExampleLogout は §9: `browser.logout` の成功のアームでは server.deleteSession は不明（http.* の例外を無視したため）、
// removeToken は起きている。requestLogout は例外を投げない。server.logout({}) の token は bind で埋まる。
func TestExampleLogout(t *testing.T) {
	p := logoutProgram()
	diags := Check(p, cfgFor("server", "browser"))
	wantCodes(t, diags) // 診断なし

	rl := sigOf(t, p, "browser.requestLogout")
	wantStrings(t, "requestLogout.E", rl.Exceptions, nil)
	wantStrings(t, "requestLogout.F", effectNames(rl.Effects), []string{"server.deleteSession"})
	if got := succOf(t, rl, "server.deleteSession"); got != ir.Unknown {
		t.Errorf("requestLogout succ(deleteSession) = %s, want unknown", got)
	}

	lo := sigOf(t, p, "browser.logout")
	wantStrings(t, "logout.E", lo.Exceptions, nil)
	wantStrings(t, "logout.F", effectNames(lo.Effects), []string{"server.deleteSession", "browser.removeToken"})
	if got := succOf(t, lo, "server.deleteSession"); got != ir.Unknown {
		t.Errorf("logout succ(deleteSession) = %s, want unknown", got)
	}
	if got := succOf(t, lo, "browser.removeToken"); got != ir.Kept {
		t.Errorf("logout succ(removeToken) = %s, want kept", got)
	}

	// server から見た server.logout は http の例外を投げない（DEC-10）
	sl := sigOf(t, p, "server.logout")
	wantStrings(t, "server.logout.E", sl.Exceptions, nil)
	if got := succOf(t, sl, "server.deleteSession"); got != ir.Kept {
		t.Errorf("server.logout succ = %s", got)
	}

	// browser から見れば投げる
	views := viewsOf(p, cfgFor("server", "browser"))
	vs := views["browser"].Ops["server.logout"]
	wantStrings(t, "View(browser).server.logout.E", vs.Exceptions, []string{"http.ServerError", "http.NetworkError"})
	if got := vs.Relations[0].State; got != ir.Unknown {
		t.Errorf("rel(ServerError, deleteSession) = %s, want unknown", got)
	}

	// bind: server.logout({}) の token
	rows := rowsOf(p.Lookup("browser.requestLogout").(*ir.Op).Body)
	bound := rows[0].calls[0].call.Bound
	if len(bound) != 1 || joinPath(bound[0].Path) != "in.token" || bound[0].State != "browser.token" {
		t.Errorf("bound = %+v", bound)
	}

	// ignoreFailure を外すと、例外は requestLogout と logout に伝わる。どちらも throws の節を書いていないので、THROWS-1 は出ない
	p2 := logoutProgram()
	p2.Lookup("browser.requestLogout").(*ir.Op).Decorators = nil
	wantNoCode(t, Check(p2, cfgFor("server", "browser")), "THROWS-1")
	wantStrings(t, "requestLogout.E", sigOf(t, p2, "browser.requestLogout").Exceptions, []string{"http.ServerError", "http.NetworkError"})
	wantStrings(t, "logout.E", sigOf(t, p2, "browser.logout").Exceptions, []string{"http.ServerError", "http.NetworkError"})

	// 節を書いた配線は、伝わった例外を覆うこと: 片方だけを書いた logout に THROWS-1
	p3 := logoutProgram()
	p3.Lookup("browser.requestLogout").(*ir.Op).Decorators = nil
	p3.Lookup("browser.logout").(*ir.Op).Throws = []ir.ThrowsClause{throws([]string{"http.NetworkError"})}
	t1 := wantOneCode(t, Check(p3, cfgFor("server", "browser")), "THROWS-1")
	wantStrings(t, "missing", t1.Data["missing"].([]string), []string{"http.ServerError"})
}

// TestExamplePlaceOrder は §7.3 の placeOrder。設計書の原文は葉の op に
// `effects createOrder | notifyUser throws NotificationFailed keeps createOrder prevents notifyUser` と書くが、
// 葉の op は effects の節を書けず（STATE-5。docs/check.md「葉の op」）、prevents / keeps の目的語は effects だけ（docs/decisions.md S9）
// なので、効果ごとの葉の op を配線でつなぐ。例外ごとの効果の状態は本体から計算され、原文が書いていた関係と同じになる。
func TestExamplePlaceOrder(t *testing.T) {
	base := func() []ir.Decl {
		return []ir.Decl{
			state("server.orders", ir.StateList, ir.String()),
			state("server.notifications", ir.StateList, ir.String()),
			eff("server.createOrder", "server.orders", ir.OpPost),
			eff("server.notifyUser", "server.notifications", ir.OpPost),
			exc("server.NotificationFailed"),
			model("server.OrderInput"),
			model("server.Order"),
		}
	}

	// 効果ごとに葉の op を分けて配線でつなぐと、状態は §9 の規則で計算される
	create := leaf("server.createOrderOp", ir.Ref("server.Order"), param("in", ir.Ref("server.OrderInput")))
	create.Effects = ir.NamedEffects("server.createOrder")
	notify := leaf("server.notify", nil, param("order", ir.Ref("server.Order")))
	notify.Effects = ir.NamedEffects("server.notifyUser")
	notify.Throws = []ir.ThrowsClause{throws([]string{"server.NotificationFailed"}, prevents())}
	wired := func(cl ...ir.ThrowsClause) *ir.Program {
		w := wire("server.placeOrder", ir.Ref("server.Order"), []ir.OpParam{param("in", ir.Ref("server.OrderInput"))},
			cst("order", call("server.createOrderOp", carg("in", nameAtom("in")))),
			cs(call("server.notify", carg("order", nameAtom("order")))),
			retAtom(nameAtom("order")),
		)
		w.Throws = cl
		return program(nil, namespace("server", append(base(), create, notify, w)...))
	}
	p2 := wired(throws([]string{"server.NotificationFailed"}))
	wantCodes(t, Check(p2, cfgFor("server")))
	sig2 := sigOf(t, p2, "server.placeOrder")
	wantStrings(t, "F", effectNames(sig2.Effects), []string{"server.createOrder", "server.notifyUser"})
	if got := relOf(t, sig2, "server.NotificationFailed", "server.createOrder"); got != ir.Kept {
		t.Errorf("wired rel(NotificationFailed, createOrder) = %s, want kept (上の行)", got)
	}
	if got := relOf(t, sig2, "server.NotificationFailed", "server.notifyUser"); got != ir.Prevented {
		t.Errorf("wired rel(NotificationFailed, notifyUser) = %s, want prevented (notify の宣言)", got)
	}
	wantStrings(t, "E", sig2.Exceptions, []string{"server.NotificationFailed"})
	if got := succOf(t, sig2, "server.createOrder"); got != ir.Kept {
		t.Errorf("wired succ(createOrder) = %s, want kept", got)
	}
	// 関係は計算されるので書かない。書けば目的語はすべての効果で、kept と prevented が混ざるこの op ではどちらも食い違う（REL-4）
	wantCodes(t, Check(wired(throws([]string{"server.NotificationFailed"}, prevents())), cfgFor("server")), "REL-4")
	wantCodes(t, Check(wired(throws([]string{"server.NotificationFailed"}, keeps())), cfgFor("server")), "REL-4")
}
