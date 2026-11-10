package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/ir"
	sdk "github.com/mew-ton/hozo/plugin"
)

// viewCorpus は tests/corpus/view（その README.md に見出し → 構文の対応がある）。
const viewCorpus = "corpus/view"

// entryDecorator はテスト用の起点のデコレータ（tests/corpus/view/t.hozo、plugins/view-dump/hozo-plugin.toml の [emit] entry）。
const entryDecorator = "t.entry"

// loadViewCorpus はコーパスを internal/project で解析し、web から見た View を返す。error があれば失敗。
func loadViewCorpus(t *testing.T) (*project.Project, *ir.View) {
	t.Helper()
	p := project.LoadWith(project.Options{Root: viewCorpus, Stderr: io.Discard})
	if p.Errors() > 0 {
		var b strings.Builder
		for _, d := range p.Diags {
			fmt.Fprintf(&b, "%s[%s]: %s\n", d.Severity, d.Code, d.Message)
		}
		t.Fatalf("tests/corpus/view に error がある:\n%s", b.String())
	}
	v := p.Views()["web"]
	if v == nil {
		t.Fatalf("Views() に web がない: %v", keysOf(p.Views()))
	}
	return p, v
}

// TestViewFromEntry は、tests/corpus/view（起点は @t.entry の付いた web の op）から hozoc が作る View
// （docs/ir.md「その名前空間から見た IR」）が文書の要求どおりであることを確かめる。
// 全体はゴールデン tests/corpus/view/testdata/expected/view.web.json と比べ（-update で更新）、
// 要求ごとの期待値はサブテスト（名前はその要求の文書の見出し）で明示する。
func TestViewFromEntry(t *testing.T) {
	p, v := loadViewCorpus(t)
	prog := v.Program

	t.Run("plugin-protocol.md entry/起点の op", func(t *testing.T) {
		// 起点は viewer の op のうち @t.entry が付いたもの。配線（openUser）と、ignores を持つ void の配線（signOut）
		expect(t, "@t.entry の付いた op", entryOps(v), []string{"web.openUser", "web.signOut"})
	})

	t.Run("golden", func(t *testing.T) {
		got := marshalView(t, v)
		compareGolden(t, filepath.Join(goldenDir("view"), "view.web.json"), got)
		// docs/ir.md「JSON」: 同じ入力から同じバイト列。読み戻して書いても同じになる
		var back ir.View
		if err := json.Unmarshal(got, &back); err != nil {
			t.Fatalf("View の JSON を読み戻せない: %v", err)
		}
		if again := marshalView(t, &back); !bytes.Equal(again, got) {
			t.Errorf("View の JSON を読み戻して書くと変わる:\n%s", firstDiff(string(got), string(again)))
		}
	})

	t.Run("ir.md その名前空間から見た IR/View", func(t *testing.T) {
		expect(t, "version", v.Version, ir.Version)
		expect(t, "viewer", v.Viewer, "web")
		expect(t, "config（Conf(web) と Dir(web)）", v.Config, ir.NamespaceConfig{Language: "go", Out: "gen/web", Dir: "gen/web", Plugins: []string{"view-dump"}})
		// namespaces は設定のあるすべての名前空間（viewer を含む）。t は state も op も持たず設定もないので入らない
		expect(t, "namespaces のキー", keysOf(v.Namespaces), []string{"api", "web"})
		expect(t, "namespaces[api]", v.Namespaces["api"], ir.NamespaceConfig{Language: "go", Out: "gen/api", Dir: "gen/api", Plugins: []string{}})
		expect(t, "namespaces[web] は config と同じ", v.Namespaces["web"], v.Config)
		// ops はすべての op について
		expect(t, "ops のキー", keysOf(v.Ops), []string{
			"api.getUser", "api.login", "api.logout", "api.touch",
			"web.adminPageVm", "web.clearToken", "web.fetchUser", "web.greet", "web.login", "web.openUser",
			"web.reload", "web.signOut", "web.userPageVm", "web.welcome",
		})
		// viewer が op の名前空間と同じなら Program の signature と一致する
		for _, name := range []string{"web.openUser", "web.signOut", "web.login", "web.reload", "web.welcome"} {
			expect(t, "ops["+name+"] と Program の signature", sigOf(v.Ops[name].Signature), sigOf(opOf(t, prog, name).Signature))
		}
		// program は全体そのまま（viewer 以外の名前空間の decls は元の Program と同じ数）
		expect(t, "api の decls の数", len(prog.Namespace("api").Decls), len(p.Sema.Program.Namespace("api").Decls))
	})

	t.Run("ir.md 原則/FQN と宣言の順序", func(t *testing.T) {
		var names []string
		for _, ns := range prog.Namespaces {
			names = append(names, ns.Name)
		}
		// グローバルが先頭、ユーザーのファイル（api.hozo、t.hozo、web.hozo）の順、その後に語彙（hozo:vocab/*.hozo）の辞書順
		expect(t, "名前空間の順", names, []string{"", "api", "t", "web", "db", "fs", "http", "page", "proc", "valid"})
		expect(t, "web.token の要素（別の名前空間の scalar は FQN）", typeStr(declOf(t, prog, "web.token").(*ir.State).Element), "api.Token?")
		expect(t, "web.location の要素（語彙の型は FQN）", typeStr(declOf(t, prog, "web.location").(*ir.State).Element), "page.Route")
		expect(t, "fetchUser が呼ぶ op（修飾なしの参照も FQN）", renderBody(opOf(t, prog, "web.openUser").Body)[0], "const user = web.fetchUser(q: {id: id})")
		expect(t, "web.goLogin の状態", declOf(t, prog, "web.goLogin").(*ir.Effect).State, "web.location")
		// 複数回の `namespace web { }` は一つの Namespace。二つ目のブロックの宣言（LoginForm、login）は一つ目の末尾に続く
		expect(t, "web は一つの Namespace", countNamespaces(prog, "web"), 1)
		webNames := declNames(prog.Namespace("web"))
		expect(t, "二つ目のブロックの宣言は一つ目の後", webNames[indexOf(webNames, "web.signOut")+1:indexOf(webNames, "web.signOut")+3], []string{"web.LoginForm", "web.login"})
	})

	t.Run("ir.md Program/Span", func(t *testing.T) {
		expect(t, "api.getUser の Span.file（hozo.toml からの相対パス）", opOf(t, prog, "api.getUser").Span.File, "api.hozo")
		expect(t, "web.openUser の Span.file", opOf(t, prog, "web.openUser").Span.File, "web.hozo")
		expect(t, "page.Route の Span.file（標準の語彙）", declOf(t, prog, "page.Route").Common().Span.File, "hozo:vocab/page.hozo")
		// 宣言の位置はデコレータから（docs/grammar.md「ファイルと宣言」の Decl = [DocComment] {Decorator} Declaration。DocComment は含まない）
		expect(t, "web.openUser の Span の行は最初のデコレータの行", opOf(t, prog, "web.openUser").Span.Start.Line, lineOf(t, "web.hozo", "@t.entry"))
		expect(t, "web.greet の Span の行は op の行", opOf(t, prog, "web.greet").Span.Start.Line, lineOf(t, "web.hozo", "op greet("))
		expect(t, "位置は 1 始まり", opOf(t, prog, "web.openUser").Span.Start.Col, 3)
	})

	t.Run("ir.md Program/Namespace.description", func(t *testing.T) {
		expect(t, "web の説明", docOf(prog.Namespace("web").Description), "ユーザーページを持つフロントエンド")
		expect(t, "t の説明", docOf(prog.Namespace("t").Description), "テストの語彙")
		expect(t, "api の説明", docOf(prog.Namespace("api").Description), "ユーザーとセッションを持つ API")
	})

	t.Run("ir.md Doc", func(t *testing.T) {
		expect(t, "openUser の Doc（@param は引数名、@throws は FQN、@see は FQN）", *opOf(t, prog, "web.openUser").Description, ir.Doc{
			Summary: "ユーザーページを開く。", Body: "ユーザーページを開く。",
			Params: []ir.DocParam{{Name: "id", Text: "表示するユーザー"}},
			Throws: []ir.DocThrows{{Exception: "web.Offline", Text: "オフラインで取れなかった"}},
			See:    []string{"web.fetchUser"},
		})
		expect(t, "fetchUser の Doc（別の名前空間の @throws / @see）", *opOf(t, prog, "web.fetchUser").Description, ir.Doc{
			Summary: "ユーザーを取る。", Body: "ユーザーを取る。",
			Params: []ir.DocParam{{Name: "q", Text: "問い合わせ"}},
			Throws: []ir.DocThrows{{Exception: "api.NotFound", Text: "いない"}, {Exception: "web.Offline", Text: "オフラインで取れなかった"}},
			See:    []string{"api.getUser"},
		})
		expect(t, "api.getUser の @see", opOf(t, prog, "api.getUser").Description.See, []string{"api.PublicUser"})
		// docs/check.md「名前空間」DOC-1、docs/decisions.md 44: external なデコレータの例外（Signature.exceptions にはない）も @throws に書ける
		expect(t, "api.getUser の @throws（external の Timeout を含む）", opOf(t, prog, "api.getUser").Description.Throws, []ir.DocThrows{
			{Exception: "api.NotFound", Text: "識別子のユーザーがいない"},
			{Exception: "api.Timeout", Text: "応答が返らない（external の例外も @throws に書ける。docs/check.md DOC-1）"},
		})
		expect(t, "フィールドの説明", docOf(fieldOf(t, prog, "api.User", "secret").Description), "表に出さない")
		expect(t, "ユニオンのアームの説明", docOf(declOf(t, prog, "web.UserPage").(*ir.Union).Arms[0].Description), "表示できる")
		expect(t, "classify のケースの説明", docOf(declOf(t, prog, "web.UserKind").(*ir.Classify).Cases[0].Description), "管理者")
		expect(t, "説明のない宣言は nil", opOf(t, prog, "web.greet").Description == nil, true)
	})

	t.Run("ir.md 型", func(t *testing.T) {
		expect(t, "api.User のフィールド", renderFields(declOf(t, prog, "api.User").(*ir.Model)), []string{
			"id: api.UserId", "name: string", "secret: string", "hash: string",
			"nick: string?",                   // 正規化: T?? は T?
			"tags: string[]",                  // array
			"attrs: Record<string, string[]>", // record
		})
		expect(t, "scalar の base が別の scalar への ref", typeStr(declOf(t, prog, "api.AdminToken").(*ir.Scalar).Base), "api.Token")
		expect(t, "絞り込まれた型 case", typeStr(opOf(t, prog, "web.adminPageVm").Params[0].Type), "web.UserKind.Admin<api.PublicUser>")
		expect(t, "void は returns が null", opOf(t, prog, "web.signOut").Returns == nil, true)
		expect(t, "戻り値の型", typeStr(opOf(t, prog, "web.openUser").Returns), "web.UserPage")
		expect(t, "state の shape", declOf(t, prog, "api.users").(*ir.State).Shape, ir.StateList)
		expect(t, "state の shape（single）", declOf(t, prog, "web.token").(*ir.State).Shape, ir.StateSingle)
	})

	t.Run("ir.md 正規化/model is omit", func(t *testing.T) {
		m := declOf(t, prog, "api.PublicUser").(*ir.Model)
		expect(t, "fields は omit を除いて複製", renderFields(m), []string{"id: api.UserId", "name: string", "nick: string?", "tags: string[]", "attrs: Record<string, string[]>"})
		expect(t, "derivedFrom", m.DerivedFrom, &ir.DerivedFrom{Base: "api.User", Omit: []string{"secret", "hash"}})
		expect(t, "複製したフィールドはデコレータも保つ", renderApplications(m.Fields[0].Decorators), []string{"@db.key"})
		expect(t, "元の model に derivedFrom はない", declOf(t, prog, "api.User").(*ir.Model).DerivedFrom == nil, true)
	})

	t.Run("ir.md 正規化/その場の model", func(t *testing.T) {
		// 例外の値: api.Invalid.Value（synthesizedFrom は例外の FQN）。宣言の位置は例外の直後
		exc := declOf(t, prog, "api.Invalid").(*ir.Exception)
		expect(t, "api.Invalid の value は ref", typeStr(exc.Value), "api.Invalid.Value")
		val := declOf(t, prog, "api.Invalid.Value").(*ir.Model)
		expect(t, "api.Invalid.Value の synthesizedFrom", val.SynthesizedFrom, "api.Invalid")
		expect(t, "api.Invalid.Value のフィールド", renderFields(val), []string{"reason: string"})
		expect(t, "api.Invalid.Value は Invalid の直後", declNames(prog.Namespace("api"))[indexOf(declNames(prog.Namespace("api")), "api.Invalid")+1], "api.Invalid.Value")
		expect(t, "値のない例外は value なし", declOf(t, prog, "api.Unauthorized").(*ir.Exception).Value == nil, true)
		// ユニオンのアーム: web.UserPage.Ready。宣言済みの model のアームはそのまま ref
		u := declOf(t, prog, "web.UserPage").(*ir.Union)
		expect(t, "UserPage の variant", u.Variant, ir.UnionArms)
		expect(t, "UserPage のアーム", renderArms(u), []string{"Ready: web.UserPage.Ready", "Missing: web.Unavailable"})
		ready := declOf(t, prog, "web.UserPage.Ready").(*ir.Model)
		expect(t, "UserPage.Ready の synthesizedFrom", ready.SynthesizedFrom, "web.UserPage")
		expect(t, "UserPage.Ready のフィールド", renderFields(ready), []string{"user: api.PublicUser"})
		webNames := declNames(prog.Namespace("web"))
		expect(t, "UserPage.Ready は UserPage の直後", webNames[indexOf(webNames, "web.UserPage")+1], "web.UserPage.Ready")
	})

	t.Run("ir.md 正規化/ユニオンの展開", func(t *testing.T) {
		u := declOf(t, prog, "api.ClientError").(*ir.Union)
		expect(t, "ClientError の variant", u.Variant, ir.UnionSet)
		expect(t, "ClientError のメンバー", memberTargets(u), []string{"api.Unauthorized", "api.NotFound"})
		// メンバーにユニオン（ClientError）があれば展開して平らにする
		expect(t, "AnyError のメンバー", memberTargets(declOf(t, prog, "api.AnyError").(*ir.Union)), []string{"api.Unauthorized", "api.NotFound", "api.Timeout"})
		// throws ClientError keeps effects → 例外の FQN に展開した一つの ThrowsClause
		expect(t, "touch の throws", renderThrows(opOf(t, prog, "api.touch").Throws), []string{"throws api.Unauthorized | api.NotFound keeps effects"})
		expect(t, "login の throws", renderThrows(opOf(t, prog, "api.login").Throws), []string{"throws api.Invalid prevents effects"})
		expect(t, "openUser の throws（修飾なしも FQN）", renderThrows(opOf(t, prog, "web.openUser").Throws), []string{"throws web.Offline | api.NotFound"})
		// 複数行の throws は ThrowsClause の列のまま（まとめない）
		expect(t, "reload の throws", renderThrows(opOf(t, prog, "web.reload").Throws), []string{"throws api.NotFound | api.Unauthorized", "throws web.Offline"})
		expect(t, "effects は FQN の列", opOf(t, prog, "api.login").Effects, []string{"api.createSession"})
		expect(t, "配線で effects を省略すれば []", orEmptyStrings(opOf(t, prog, "web.openUser").Effects), []string{})
	})

	t.Run("ir.md 正規化/条件", func(t *testing.T) {
		c := declOf(t, prog, "web.Verified").(*ir.Classify)
		expect(t, "condition は cases 一つ", caseNames(c), []string{"Verified"})
		expect(t, "condition の hasDefault", c.HasDefault, true)
		expect(t, "condition の fromCondition", c.FromCondition, true)
		expect(t, "condition の subject", typeStr(c.Subject), "api.PublicUser")
		k := declOf(t, prog, "web.UserKind").(*ir.Classify)
		expect(t, "classify の cases", caseNames(k), []string{"Admin", "Member"})
		expect(t, "classify の hasDefault", k.HasDefault, false)
		expect(t, "classify の fromCondition", k.FromCondition, false)
	})

	t.Run("ir.md decorator/$param と external", func(t *testing.T) {
		d := declOf(t, prog, "web.redirectOn").(*ir.Decorator)
		expect(t, "redirectOn の params", renderParams(d.Params), []string{"error: any exception", "target: any effect"})
		expect(t, "redirectOn の catches（引数名は $param:名前）", renderCatches(d.Catches), []string{"catches $param:error halts $param:target"})
		expect(t, "targets の省略は [op]", d.Targets, []ir.DecoratorTarget{ir.TargetOp})
		expect(t, "external の既定", d.External, false)
		r := declOf(t, prog, "api.remote").(*ir.Decorator)
		expect(t, "@external", r.External, true)
		expect(t, "remote の throws", renderThrows(r.Throws), []string{"throws api.Timeout"})
		f := declOf(t, prog, "web.offlineFallback").(*ir.Decorator)
		expect(t, "offlineFallback の catches", renderCatches(f.Catches), []string{"catches api.Timeout recovers"})
		expect(t, "offlineFallback の throws", renderThrows(f.Throws), []string{"throws web.Offline"})
		expect(t, "ignoreTimeout の catches", renderCatches(declOf(t, prog, "web.ignoreTimeout").(*ir.Decorator).Catches), []string{"catches api.Timeout ignores"})
		expect(t, "traced の params（型で受ける）", renderParams(declOf(t, prog, "web.traced").(*ir.Decorator).Params), []string{"key: string", "within: Duration"})
	})

	t.Run("ir.md 正規化/名前付きデコレータ", func(t *testing.T) {
		a := declOf(t, prog, "web.redirectToLogin").(*ir.Decorator)
		expect(t, "alias の target", a.Alias.Target, "web.redirectOn")
		expect(t, "alias の args（引数名に対応付け済み）", renderArgs(a.Alias.Args), "error: api.Unauthorized, target: web.goLogin")
		expect(t, "alias は params / throws / catches を持たない", len(a.Params)+len(a.Throws)+len(a.Catches), 0)
		// 適用は汎用デコレータに展開し、via に名前付きの FQN
		app := opOf(t, prog, "web.openUser").Decorators[1]
		expect(t, "decorator は展開済み", app.Decorator, "web.redirectOn")
		expect(t, "via", app.Via, "web.redirectToLogin")
		expect(t, "args は固定した引数", renderArgs(app.Args), "error: api.Unauthorized, target: web.goLogin")
	})

	t.Run("ir.md デコレータの適用", func(t *testing.T) {
		// 書かれた順（上から下）。最後の要素（traced）が最も内側。op の引数を値に取る引数は $param:名前、5m はナノ秒
		expect(t, "openUser の decorators", renderApplications(opOf(t, prog, "web.openUser").Decorators), []string{
			"@t.entry",
			"@web.redirectOn(error: api.Unauthorized, target: web.goLogin) via web.redirectToLogin",
			"@web.traced(key: $param:id, within: 300000000000ns)",
		})
		expect(t, "signOut の decorators", renderApplications(opOf(t, prog, "web.signOut").Decorators), []string{"@t.entry", "@web.ignoreTimeout"})
		expect(t, "getUser の decorators", renderApplications(opOf(t, prog, "api.getUser").Decorators), []string{"@api.remote", "@api.requiresAuth"})
		expect(t, "位置で書いた引数は param に対応付け", renderApplications(declOf(t, prog, "api.Invalid").Common().Decorators), []string{"@http.status(code: 400)"})
	})

	t.Run("ir.md 正規化/リテラル", func(t *testing.T) {
		expect(t, "Query の既定値（int、Duration はナノ秒、boolean）", renderFields(declOf(t, prog, "web.Query").(*ir.Model)), []string{
			"id: api.UserId", "page: int = 1", "timeout: Duration = 30000000000ns", "strict: boolean = false",
		})
		expect(t, "LoginInput の既定値", renderFields(declOf(t, prog, "api.LoginInput").(*ir.Model)), []string{"creds: api.Credentials", "remember: boolean = false"})
	})

	t.Run("ir.md 配線", func(t *testing.T) {
		expect(t, "openUser の本体（{ id } は { id: id }、cases は書かれた順、return に束縛名）", renderBody(opOf(t, prog, "web.openUser").Body), []string{
			"const user = web.fetchUser(q: {id: id})",
			"const page = match user by web.UserKind { Admin => web.adminPageVm(user: user); Member => web.userPageVm(user: user) }",
			"return page",
		})
		expect(t, "welcome の本体（match の文、default と none）", renderBody(opOf(t, prog, "web.welcome").Body), []string{
			"match user by web.Verified { Verified => web.greet(user: user); default => none }",
		})
		expect(t, "login の本体（入れ子のレコードリテラルとフィールド参照）", renderBody(opOf(t, prog, "web.login").Body), []string{
			"return api.login(in: {creds: {name: form.name, password: form.password}})",
		})
		expect(t, "reload の本体（呼び出しの文）", renderBody(opOf(t, prog, "web.reload").Body), []string{
			"const user = web.fetchUser(q: {id: id})", "api.touch(id: id)", "return user",
		})
		expect(t, "葉の body は null", opOf(t, prog, "api.getUser").Body == nil, true)
	})

	t.Run("ir.md bind", func(t *testing.T) {
		expect(t, "binds", renderBinds(prog.Binds), []string{"web: bind web.token to api.*"})
	})

	t.Run("check.md 計算/Sig 葉", func(t *testing.T) {
		expect(t, "api.login（prevents effects）", sigOf(opOf(t, prog, "api.login").Signature), sig{
			E: []string{"api.Invalid"}, F: []string{"api.createSession"},
			Rel: []string{"api.Invalid / api.createSession = prevented"}, Succ: []string{"api.createSession = kept"},
		})
		expect(t, "api.touch（ユニオンを展開した E と keeps）", sigOf(opOf(t, prog, "api.touch").Signature), sig{
			E: []string{"api.Unauthorized", "api.NotFound"}, F: []string{"api.touchUser"},
			Rel:  []string{"api.Unauthorized / api.touchUser = kept", "api.NotFound / api.touchUser = kept"},
			Succ: []string{"api.touchUser = kept"},
		})
		expect(t, "web.greet（throws なし）", sigOf(opOf(t, prog, "web.greet").Signature), sig{E: []string{}, F: []string{"web.goHome"}, Rel: []string{}, Succ: []string{"web.goHome = kept"}})
	})

	t.Run("check.md 計算/Sig external", func(t *testing.T) {
		// DEC-10: external の throws は v = ns(O) では飛ばし、境界の外（web）から見たときだけ E に入る
		expect(t, "Program の api.getUser.signature（デコレータは内側から: requiresAuth の Unauthorized）", sigOf(opOf(t, prog, "api.getUser").Signature), sig{
			E: []string{"api.NotFound", "api.Unauthorized"}, F: []string{}, Rel: []string{}, Succ: []string{},
		})
		expect(t, "web から見た api.getUser は Timeout を含む", sigOf(v.Ops["api.getUser"].Signature), sig{
			E: []string{"api.NotFound", "api.Unauthorized", "api.Timeout"}, F: []string{}, Rel: []string{}, Succ: []string{},
		})
		expect(t, "Program の api.login.signature に Timeout はない", opOf(t, prog, "api.login").Signature.Exceptions, []string{"api.Invalid"})
		expect(t, "web から見た api.login（Timeout の relations はない → unknown）", sigOf(v.Ops["api.login"].Signature), sig{
			E: []string{"api.Invalid", "api.Timeout"}, F: []string{"api.createSession"},
			Rel:  []string{"api.Invalid / api.createSession = prevented", "api.Timeout / api.createSession = unknown"},
			Succ: []string{"api.createSession = kept"},
		})
		expect(t, "Program の api.logout.signature", sigOf(opOf(t, prog, "api.logout").Signature), sig{E: []string{}, F: []string{"api.deleteSession"}, Rel: []string{}, Succ: []string{"api.deleteSession = kept"}})
		expect(t, "web から見た api.logout", sigOf(v.Ops["api.logout"].Signature), sig{
			E: []string{"api.Timeout"}, F: []string{"api.deleteSession"},
			Rel: []string{"api.Timeout / api.deleteSession = unknown"}, Succ: []string{"api.deleteSession = kept"},
		})
		// api から見た View では Program の signature と一致する
		apiView := p.Views()["api"]
		if apiView == nil {
			t.Fatal("Views() に api がない")
		}
		expect(t, "api から見た api.getUser", sigOf(apiView.Ops["api.getUser"].Signature), sigOf(opOf(t, prog, "api.getUser").Signature))
	})

	t.Run("check.md 計算/配線 recovers", func(t *testing.T) {
		// §8.2: Timeout のとき createSession は unknown なので、recovers で吸収すると成功のアームでも unknown（DEC-5）。
		// Offline は throws の節に relations がないので unknown
		expect(t, "web.login", sigOf(opOf(t, prog, "web.login").Signature), sig{
			E: []string{"api.Invalid", "web.Offline"}, F: []string{"api.createSession"},
			Rel:  []string{"api.Invalid / api.createSession = prevented", "web.Offline / api.createSession = unknown"},
			Succ: []string{"api.createSession = unknown"},
		})
		// 効果のない配線では recovers は E を差し引くだけ。E は本体の出どころの順に、Offline を末尾に足す
		expect(t, "web.fetchUser", sigOf(opOf(t, prog, "web.fetchUser").Signature), sig{
			E: []string{"api.NotFound", "api.Unauthorized", "web.Offline"}, F: []string{}, Rel: []string{}, Succ: []string{},
		})
	})

	t.Run("check.md 計算/配線 match", func(t *testing.T) {
		// match の行で一部のアームにしかない効果は succ が unknown
		expect(t, "web.welcome", sigOf(opOf(t, prog, "web.welcome").Signature), sig{E: []string{}, F: []string{"web.goHome"}, Rel: []string{}, Succ: []string{"web.goHome = unknown"}})
	})

	t.Run("check.md 計算/配線 出どころ", func(t *testing.T) {
		// NotFound / Unauthorized の出どころは二つ: 行 1（fetchUser。touchUser は行 1 の呼び出しにないので prevented）と
		// 行 2（touch。Sc.rel は kept）→ 違うので unknown。Offline の出どころは行 1 だけ → prevented。succ は touch の kept
		expect(t, "web.reload", sigOf(opOf(t, prog, "web.reload").Signature), sig{
			E: []string{"api.NotFound", "api.Unauthorized", "web.Offline"}, F: []string{"api.touchUser"},
			Rel: []string{
				"api.NotFound / api.touchUser = unknown",
				"api.Unauthorized / api.touchUser = unknown",
				"web.Offline / api.touchUser = prevented",
			},
			Succ: []string{"api.touchUser = kept"},
		})
	})

	t.Run("check.md 計算/halts", func(t *testing.T) {
		// redirectToLogin（= redirectOn(api.Unauthorized, goLogin)）: Unauthorized を E から除き、goLogin を F の末尾に足し、
		// 残りのすべての例外について rel = prevented、succ(goLogin) = prevented
		expect(t, "web.openUser", sigOf(opOf(t, prog, "web.openUser").Signature), sig{
			E: []string{"api.NotFound", "web.Offline"}, F: []string{"web.goLogin"},
			Rel:  []string{"api.NotFound / web.goLogin = prevented", "web.Offline / web.goLogin = prevented"},
			Succ: []string{"web.goLogin = prevented"},
		})
	})

	t.Run("check.md 計算/ignores", func(t *testing.T) {
		// Timeout の rel は deleteSession で unknown、removeToken で prevented（行 1 の呼び出しの効果にない）。
		// ignores はどちらも kept でないので succ を unknown にし、Timeout を E から除く
		expect(t, "web.signOut", sigOf(opOf(t, prog, "web.signOut").Signature), sig{
			E: []string{}, F: []string{"api.deleteSession", "web.removeToken"}, Rel: []string{},
			Succ: []string{"api.deleteSession = unknown", "web.removeToken = unknown"},
		})
	})

	t.Run("check.md 計算/配線 効果の伝播", func(t *testing.T) {
		// F は各呼び出しの effects を行の順に連結したもの（書いていない effects は Op.effects が [] でも Signature にある）
		expect(t, "signOut の Op.effects", orEmptyStrings(opOf(t, prog, "web.signOut").Effects), []string{})
		expect(t, "signOut の Signature.effects", opOf(t, prog, "web.signOut").Signature.Effects, []string{"api.deleteSession", "web.removeToken"})
		expect(t, "reload の Signature.effects", opOf(t, prog, "web.reload").Signature.Effects, []string{"api.touchUser"})
	})

	t.Run("check.md bind/Pos", func(t *testing.T) {
		// 位置が model のフィールド（["in", "token"]）: 引数は web.getUser$in への ref になる
		get := v.Ops["api.getUser"]
		expect(t, "api.getUser の bound", renderBound(get.Bound), []string{"in.token <- web.token"})
		expect(t, "api.getUser の params", renderOpParams(get.Params), []string{"in: web.getUser$in"})
		// 位置が引数そのもの（["token"]）: 引数を取り除く
		logout := v.Ops["api.logout"]
		expect(t, "api.logout の bound", renderBound(logout.Bound), []string{"token <- web.token"})
		expect(t, "api.logout の params", renderOpParams(logout.Params), []string{})
		// 位置のない op はそのまま
		expect(t, "api.touch の bound", renderBound(v.Ops["api.touch"].Bound), []string{})
		expect(t, "api.touch の params", renderOpParams(v.Ops["api.touch"].Params), []string{"id: api.UserId"})
		expect(t, "api.login の params", renderOpParams(v.Ops["api.login"].Params), []string{"in: api.LoginInput"})
		// Program の op の params は変えない
		expect(t, "Program の api.getUser.params", renderOpParams(opOf(t, prog, "api.getUser").Params), []string{"in: api.GetUserInput"})
	})

	t.Run("ir.md その名前空間から見た IR/params", func(t *testing.T) {
		// 合成した model は viewer の Namespace.decls の末尾。synthesizedFrom は op の FQN。元の Program にはない
		webNames := declNames(prog.Namespace("web"))
		expect(t, "web の decls の末尾", webNames[len(webNames)-1], "web.getUser$in")
		synth := declOf(t, prog, "web.getUser$in").(*ir.Model)
		expect(t, "web.getUser$in の synthesizedFrom", synth.SynthesizedFrom, "api.getUser")
		expect(t, "web.getUser$in のフィールド（token を除く）", renderFields(synth), []string{"id: api.UserId"})
		expect(t, "元の Program に合成した model はない", p.Sema.Program.Lookup("web.getUser$in") == nil, true)
		expect(t, "api から見た View にもない", p.Views()["api"].Program.Lookup("web.getUser$in") == nil, true)
		expect(t, "web の decls は元より一つ多い", len(webNames), len(p.Sema.Program.Namespace("web").Decls)+1)
	})

	t.Run("check.md bind/Call.bound", func(t *testing.T) {
		// 配線の呼び出しの Call.bound: 明示していない位置だけ（BIND-3）
		expect(t, "fetchUser の api.getUser({ id: q.id })", renderBody(opOf(t, prog, "web.fetchUser").Body), []string{
			"return api.getUser(in: {id: q.id}) bound[in.token <- web.token]",
		})
		expect(t, "signOut の api.logout()", renderBody(opOf(t, prog, "web.signOut").Body), []string{
			"api.logout() bound[token <- web.token]", "web.clearToken()",
		})
		// web の中の呼び出しと、位置のない api.touch には bound はない（上の「ir.md 配線」の reload）
	})

	t.Run("ir.md JSON", func(t *testing.T) {
		raw := decodeJSON(t, marshalView(t, v))
		web := jsonNamespace(t, raw, "web")
		signOut := jsonDecl(t, web, "web.signOut")
		if r, ok := signOut["returns"]; !ok || r != nil {
			t.Errorf("void の op の returns は null で出す: %v", signOut["returns"])
		}
		greet := jsonDecl(t, web, "web.greet")
		if b, ok := greet["body"]; !ok || b != nil {
			t.Errorf("葉の op の body は null で出す: %v", greet["body"])
		}
		if _, ok := greet["description"]; ok {
			t.Errorf("説明のない宣言に description のキーを出さない: %v", greet["description"])
		}
		expect(t, "空の列は [] で出す（greet の decorators）", greet["decorators"], []any{})
		openUser := jsonDecl(t, web, "web.openUser")
		apps := openUser["decorators"].([]any)
		if _, ok := apps[0].(map[string]any)["via"]; ok {
			t.Errorf("名前付きデコレータを通さない適用に via のキーを出さない: %v", apps[0])
		}
		expect(t, "名前付きデコレータを通した適用の via", apps[1].(map[string]any)["via"], "web.redirectToLogin")
		api := jsonNamespace(t, raw, "api")
		if _, ok := jsonDecl(t, api, "api.Unauthorized")["value"]; ok {
			t.Error("値のない例外に value のキーを出さない")
		}
		if _, ok := jsonDecl(t, api, "api.User")["derivedFrom"]; ok {
			t.Error("is で作っていない model に derivedFrom のキーを出さない")
		}
		expect(t, "kind は判別子", jsonDecl(t, api, "api.ClientError")["kind"], "union")
		expect(t, "default のアームは \"default\"", jsonDecl(t, web, "web.welcome")["body"].(map[string]any)["statements"].([]any)[0].(map[string]any)["match"].(map[string]any)["arms"].([]any)[1].(map[string]any)["cases"], "default")
		expect(t, "ops のキーは FQN の辞書順", jsonKeysInOrder(t, marshalView(t, v), "ops"), keysOf(v.Ops))
	})
}

// TestViewDeliveredToPlugin は、実物の `hozoc emit`（internal/app の Leaves と生成した cli.Main。tests/selfcheck_test.go と同じ組み立て）が
// view-dump プラグイン（tests/corpus/view/plugins/view-dump。`go run` で動く）に渡す View が、internal/project の Views()["web"] と
// 同じであることを確かめる。プラグインは受け取った view を <view.config.dir>/view.json に書き出すので、それを読み戻して比べる。
// コーパスは一時ディレクトリに写してモジュールのルートにする（copyCorpusAsModule）。
func TestViewDeliveredToPlugin(t *testing.T) {
	requireGo(t)
	root, _ := copyCorpusAsModule(t, "view")
	var stdout, stderr bytes.Buffer
	code := hozocMainIn(root, []string{"emit"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("hozoc emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "written: gen/web/view.json\n") {
		t.Errorf("プラグインの view.json が written にない:\n%s", stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(root, "gen", "web", "view.json"))
	if err != nil {
		t.Fatalf("プラグインの出力を読めない: %v", err)
	}

	_, want := loadViewCorpus(t)
	var got ir.View
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("プラグインが受け取った view を ir.View として読めない: %v", err)
	}
	wantJSON := marshalView(t, want)
	if gotJSON := marshalView(t, &got); !bytes.Equal(gotJSON, wantJSON) {
		t.Errorf("プラグインが受け取った View が Views()[\"web\"] と違う:\n%s", firstDiff(string(wantJSON), string(gotJSON)))
	}
	// プラグインが書いたバイト列そのもの（SDK の ir.View を 2 スペースで書き直したもの）も同じになる（docs/ir.md「JSON」の決定性）
	if !bytes.Equal(data, wantJSON) {
		t.Errorf("プラグインが書いた view.json のバイト列が Views()[\"web\"] の JSON と違う:\n%s", firstDiff(string(wantJSON), string(data)))
	}
	expect(t, "view.version", got.Version, ir.Version)
	expect(t, "view.viewer", got.Viewer, "web")
	expect(t, "view.config.dir", got.Config.Dir, "gen/web")
	expect(t, "view.config.plugins", got.Config.Plugins, []string{"view-dump"})
	expect(t, "view.namespaces のキー", keysOf(got.Namespaces), []string{"api", "web"})
	expect(t, "起点の op", entryOps(&got), []string{"web.openUser", "web.signOut"})

	// docs/plugin-protocol.md「要求と応答」: protocol が合わなければ code "protocol"（MAN-2）、direction を実装していなければ
	// code "unsupported"（MAN-5）で、どちらも終了コード 2。同じプラグインを internal/plugin.Runner で直接起動して確かめる。
	// マニフェストの `go run` を通すと `go run` が終了コード 2 を 1 に変えてしまう（proc.Crashed になる）ので、
	// ここではプラグインを go build した実行ファイルで起動する（internal/plugin の echo-plugin のテストと同じ）。
	t.Run("plugin-protocol.md 要求と応答/protocol", func(t *testing.T) {
		m, dir := viewDumpBuilt(t, root)
		req := sdk.EmitRequest{Protocol: "9.9", Direction: "emit", View: want}
		var resp sdk.EmitResponse
		err := hostplugin.Runner{Root: root, Stderr: io.Discard}.Run(m, dir, &req, &resp)
		var pe *hostplugin.PluginError
		if !errors.As(err, &pe) || pe.Code != hostplugin.CodeProtocol {
			t.Fatalf("protocol 9.9 の要求に対する誤り = %v, want *PluginError{Code: %q}", err, hostplugin.CodeProtocol)
		}
		expect(t, "診断は MAN-2", pe.Diagnostic("plugins/view-dump/hozo-plugin.toml").Code, "MAN-2")
	})
	t.Run("plugin-protocol.md 要求と応答/unsupported", func(t *testing.T) {
		m, dir := viewDumpBuilt(t, root)
		req := sdk.ImportRequest{Protocol: hostplugin.Protocol, Direction: "import", Format: "openapi", Files: []sdk.File{}}
		var resp sdk.ImportResponse
		err := hostplugin.Runner{Root: root, Stderr: io.Discard}.Run(m, dir, &req, &resp)
		var pe *hostplugin.PluginError
		if !errors.As(err, &pe) || pe.Code != hostplugin.CodeUnsupported {
			t.Fatalf("import の要求に対する誤り = %v, want *PluginError{Code: %q}", err, hostplugin.CodeUnsupported)
		}
		expect(t, "診断は MAN-5", pe.Diagnostic("plugins/view-dump/hozo-plugin.toml").Code, "MAN-5")
	})
}

// viewDumpBuilt は view-dump を go build して、その実行ファイルを command にしたマニフェストと、マニフェストのディレクトリ
// （一時ディレクトリ root に写したもの）を返す。マニフェストの command（`go run`）は使わない: `go run` はプログラムの
// 終了コードを伝えず、0 以外ならすべて 1 にするため（docs/plugin-protocol.md「起動」の終了コード 2 が見えない）。
func viewDumpBuilt(t *testing.T, root string) (*hostplugin.Manifest, string) {
	t.Helper()
	dir := filepath.Join(root, "plugins", "view-dump")
	m, diags := hostplugin.LoadManifest(filepath.Join(dir, hostplugin.ManifestFileName))
	if m == nil || len(diags) > 0 {
		t.Fatalf("view-dump のマニフェストを読めない: %+v", diags)
	}
	exe := filepath.Join(root, "bin", "view-dump")
	build := exec.Command("go", "build", "-o", exe, "./"+filepath.ToSlash(filepath.Join(viewCorpus, "plugins", "view-dump")))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("view-dump を go build できない: %v\n%s", err, out)
	}
	m.Command = []string{exe}
	return m, dir
}

// ---- 期待値の書き方 ----

// expect は got と want を比べ、違えば両方を JSON で示す。比較も JSON で行う（docs/ir.md「JSON」の形が等しいこと。
// nil の列と空の列の違いは見ない）。
func expect(t *testing.T, what string, got, want any) {
	t.Helper()
	g, w := show(got), show(want)
	if g == w || reflect.DeepEqual(got, want) {
		return
	}
	t.Errorf("%s:\n  got:  %s\n  want: %s", what, g, w)
}

func show(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}

// sig は Signature を読みやすい形にしたもの（docs/check.md「Sig(O, v)」の E / F / rel / succ）。
type sig struct {
	E, F []string
	Rel  []string // "X / e = state"（E の順、F の順）
	Succ []string // "e = state"（F の順）
}

func sigOf(s ir.Signature) sig {
	out := sig{E: orEmptyStrings(s.Exceptions), F: orEmptyStrings(s.Effects), Rel: []string{}, Succ: []string{}}
	for _, r := range s.Relations {
		out.Rel = append(out.Rel, fmt.Sprintf("%s / %s = %s", r.Exception, r.Effect, r.State))
	}
	for _, e := range s.SuccessEffects {
		out.Succ = append(out.Succ, fmt.Sprintf("%s = %s", e.Effect, e.State))
	}
	return out
}

func orEmptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// entryOps は viewer の op のうち entryDecorator が付いたものの FQN（出現順）。
func entryOps(v *ir.View) []string {
	out := []string{}
	ns := v.Program.Namespace(v.Viewer)
	if ns == nil {
		return out
	}
	for _, d := range ns.Decls {
		o, ok := d.(*ir.Op)
		if !ok {
			continue
		}
		for _, a := range o.Decorators {
			if a.Decorator == entryDecorator {
				out = append(out, o.Name)
			}
		}
	}
	return out
}

func marshalView(t *testing.T, v *ir.View) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

func declOf(t *testing.T, p *ir.Program, fqn string) ir.Decl {
	t.Helper()
	d := p.Lookup(fqn)
	if d == nil {
		t.Fatalf("宣言 %s がない", fqn)
	}
	return d
}

func opOf(t *testing.T, p *ir.Program, fqn string) *ir.Op {
	t.Helper()
	o, ok := declOf(t, p, fqn).(*ir.Op)
	if !ok {
		t.Fatalf("%s は op でない", fqn)
	}
	return o
}

func fieldOf(t *testing.T, p *ir.Program, model, name string) ir.Field {
	t.Helper()
	m := declOf(t, p, model).(*ir.Model)
	for _, f := range m.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("%s にフィールド %s がない", model, name)
	return ir.Field{}
}

func docOf(d *ir.Doc) string {
	if d == nil {
		return "<nil>"
	}
	return d.Summary
}

func declNames(ns *ir.Namespace) []string {
	var out []string
	for _, d := range ns.Decls {
		out = append(out, d.Common().Name)
	}
	return out
}

func countNamespaces(p *ir.Program, name string) int {
	n := 0
	for _, ns := range p.Namespaces {
		if ns.Name == name {
			n++
		}
	}
	return n
}

func indexOf(xs []string, x string) int {
	for i, y := range xs {
		if y == x {
			return i
		}
	}
	return -1
}

func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// lineOf は tests/corpus/view/<file> で needle を含む最初の行（1 始まり）。
func lineOf(t *testing.T, file, needle string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(viewCorpus, file))
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(string(data), "\n") {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	t.Fatalf("%s に %q がない", file, needle)
	return 0
}

// ---- IR を読みやすい文字列にする（位置は落とす） ----

func typeStr(ty ir.Type) string {
	if ty == nil {
		return "void"
	}
	return fmt.Sprint(ty)
}

func renderLiteral(l ir.Literal) string {
	switch x := l.(type) {
	case ir.StringLiteral:
		return fmt.Sprintf("%q", x.Value)
	case ir.IntLiteral:
		return fmt.Sprintf("%d", x.Value)
	case ir.FloatLiteral:
		return fmt.Sprintf("%g", x.Value)
	case ir.BooleanLiteral:
		return fmt.Sprintf("%t", x.Value)
	case ir.DurationLiteral:
		return fmt.Sprintf("%dns", int64(x.Value))
	}
	return fmt.Sprintf("<%T>", l)
}

func renderFields(m *ir.Model) []string {
	out := []string{}
	for _, f := range m.Fields {
		s := f.Name + ": " + typeStr(f.Type)
		if f.Default != nil {
			s += " = " + renderLiteral(f.Default)
		}
		out = append(out, s)
	}
	return out
}

func renderArms(u *ir.Union) []string {
	out := []string{}
	for _, a := range u.Arms {
		out = append(out, a.Name+": "+typeStr(a.Type))
	}
	return out
}

func memberTargets(u *ir.Union) []string {
	out := []string{}
	for _, m := range u.Members {
		out = append(out, m.Target)
	}
	return out
}

func caseNames(c *ir.Classify) []string {
	out := []string{}
	for _, k := range c.Cases {
		out = append(out, k.Name)
	}
	return out
}

func renderParams(ps []ir.Param) []string {
	out := []string{}
	for _, p := range ps {
		var ty string
		switch pt := p.Type.(type) {
		case ir.TypeParam:
			var ts []string
			for _, x := range pt.Types {
				ts = append(ts, typeStr(x))
			}
			ty = strings.Join(ts, " | ")
		case ir.AnyParam:
			ty = "any " + string(pt.Of)
			if pt.On != nil {
				ty += fmt.Sprintf(" on %s.%s", pt.On.State, pt.On.Operation)
			}
		}
		if p.Optional {
			ty += "?"
		}
		out = append(out, p.Name+": "+ty)
	}
	return out
}

func renderRelations(rs []ir.Relation) string {
	var s string
	for _, r := range rs {
		s += " " + string(r.Kind) + " "
		if r.Effects.All {
			s += "effects"
		} else {
			s += strings.Join(r.Effects.Effects, " | ")
		}
	}
	return s
}

func renderThrows(cls []ir.ThrowsClause) []string {
	out := []string{}
	for _, c := range cls {
		out = append(out, "throws "+strings.Join(c.Exceptions, " | ")+renderRelations(c.Relations))
	}
	return out
}

func renderCatches(cls []ir.CatchesClause) []string {
	out := []string{}
	for _, c := range cls {
		s := "catches " + strings.Join(c.Exceptions, " | ") + " " + string(c.Mode)
		if c.Mode == ir.Halts {
			s += " " + c.Halts
		}
		out = append(out, s)
	}
	return out
}

func renderArgValue(v ir.ArgValue) string {
	switch x := v.(type) {
	case ir.LiteralValue:
		return renderLiteral(x.Literal)
	case ir.RefValue:
		return x.Target
	case ir.RefsValue:
		return strings.Join(x.Targets, " | ")
	case ir.VariantValue:
		return x.Union + "." + x.Arm
	case ir.ListValue:
		var items []string
		for _, i := range x.Items {
			items = append(items, renderArgValue(i))
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	return fmt.Sprintf("<%T>", v)
}

func renderArgs(args []ir.Arg) string {
	var parts []string
	for _, a := range args {
		parts = append(parts, a.Param+": "+renderArgValue(a.Value))
	}
	return strings.Join(parts, ", ")
}

func renderApplications(apps []ir.Application) []string {
	out := []string{}
	for _, a := range apps {
		s := "@" + a.Decorator
		if len(a.Args) > 0 {
			s += "(" + renderArgs(a.Args) + ")"
		}
		if a.Via != "" {
			s += " via " + a.Via
		}
		out = append(out, s)
	}
	return out
}

func renderAtom(a ir.Atom) string {
	switch x := a.(type) {
	case ir.NameAtom:
		return strings.Join(x.Path, ".")
	case ir.LiteralAtom:
		return renderLiteral(x.Literal)
	case ir.NoneAtom:
		return "none"
	case ir.RecordAtom:
		var fs []string
		for _, f := range x.Fields {
			fs = append(fs, f.Name+": "+renderAtom(f.Value))
		}
		return "{" + strings.Join(fs, ", ") + "}"
	}
	return fmt.Sprintf("<%T>", a)
}

func renderBound(bs []ir.BoundPosition) []string {
	out := []string{}
	for _, b := range bs {
		out = append(out, strings.Join(b.Path, ".")+" <- "+b.State)
	}
	return out
}

func renderCall(c ir.Call) string {
	var args []string
	for _, a := range c.Args {
		args = append(args, a.Param+": "+renderAtom(a.Value))
	}
	s := c.Op + "(" + strings.Join(args, ", ") + ")"
	if len(c.Bound) > 0 {
		s += " bound[" + strings.Join(renderBound(c.Bound), ", ") + "]"
	}
	return s
}

func renderMatch(m ir.Match) string {
	var arms []string
	for _, a := range m.Arms {
		cases := "default"
		if !a.Cases.Default {
			cases = strings.Join(a.Cases.Cases, " | ")
		}
		body := "none"
		if ce, ok := a.Body.(ir.CallExpr); ok {
			body = renderCall(ce.Call)
		}
		arms = append(arms, cases+" => "+body)
	}
	return "match " + renderAtom(m.Subject) + " by " + m.Classify + " { " + strings.Join(arms, "; ") + " }"
}

func renderExpr(e ir.Expr) string {
	switch x := e.(type) {
	case ir.CallExpr:
		return renderCall(x.Call)
	case ir.MatchExpr:
		return renderMatch(x.Match)
	case ir.AtomExpr:
		return renderAtom(x.Atom)
	}
	return fmt.Sprintf("<%T>", e)
}

// renderBody は配線の本体を一行ずつ文字列にする。葉（nil）なら nil。
func renderBody(w *ir.Wiring) []string {
	if w == nil {
		return nil
	}
	out := []string{}
	for _, st := range w.Statements {
		switch s := st.(type) {
		case ir.ConstStmt:
			out = append(out, "const "+s.Name+" = "+renderExpr(s.Expr))
		case ir.ReturnStmt:
			out = append(out, "return "+renderExpr(s.Expr))
		case ir.CallStmt:
			out = append(out, renderCall(s.Call))
		case ir.MatchStmt:
			out = append(out, renderMatch(s.Match))
		default:
			out = append(out, fmt.Sprintf("<%T>", st))
		}
	}
	return out
}

func renderOpParams(ps []ir.OpParam) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Name+": "+typeStr(p.Type))
	}
	return out
}

func renderBinds(bs []ir.Bind) []string {
	out := []string{}
	for _, b := range bs {
		star := "*"
		if b.Target.Deep {
			star = "**"
		}
		out = append(out, fmt.Sprintf("%s: bind %s to %s.%s", b.Namespace, b.State, b.Target.Namespace, star))
	}
	return out
}

// ---- JSON をそのまま見る ----

func decodeJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func jsonNamespace(t *testing.T, raw map[string]any, name string) []any {
	t.Helper()
	for _, ns := range raw["program"].(map[string]any)["namespaces"].([]any) {
		if m := ns.(map[string]any); m["name"] == name {
			return m["decls"].([]any)
		}
	}
	t.Fatalf("JSON に名前空間 %s がない", name)
	return nil
}

func jsonDecl(t *testing.T, decls []any, name string) map[string]any {
	t.Helper()
	for _, d := range decls {
		if m := d.(map[string]any); m["name"] == name {
			return m
		}
	}
	t.Fatalf("JSON に宣言 %s がない", name)
	return nil
}

// jsonKeysInOrder は data のトップレベルのオブジェクト key の中のキーを、書かれた順に返す。
func jsonKeysInOrder(t *testing.T, data []byte, key string) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("JSON がオブジェクトでない: %v %v", tok, err)
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		if tok.(string) != key {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
			t.Fatalf("%s がオブジェクトでない: %v %v", key, tok, err)
		}
		var keys []string
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				t.Fatal(err)
			}
			keys = append(keys, k.(string))
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				t.Fatal(err)
			}
		}
		return keys
	}
	t.Fatalf("JSON に %s がない", key)
	return nil
}
