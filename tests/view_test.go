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

// entryDecorator はテスト用の印のデコレータ（tests/corpus/view/t.hozo）。SDK の plugin.OpsWith が、このデコレータの付いた、
// 単位の対象の op を返すことを確かめるのに使う（相乗りが自分の対象を探す道具。docs/plugin-protocol.md「選択をなくしたこと」）。
const entryDecorator = "t.entry"

// loadViewCorpus はコーパスを internal/project で解析し、web から見た View を返す。error があれば失敗。
func loadViewCorpus(t *testing.T) (*project.Project, *ir.View) {
	t.Helper()
	p := project.LoadWith(project.Options{Root: viewCorpus, Stderr: io.Discard, PluginPath: repoPlugins(t)})
	if p.Errors() > 0 {
		var b strings.Builder
		for _, d := range p.Diags {
			fmt.Fprintf(&b, "%s[%s]: %s\n", d.Severity, d.Code, d.Message)
		}
		t.Fatalf("tests/corpus/view に error がある:\n%s", b.String())
	}
	v := viewsOf(p)["web"]
	if v == nil {
		t.Fatalf("Views() に web がない: %v", keysOf(viewsOf(p)))
	}
	return p, v
}

// viewsOf は Project の View を、名前空間 → View で返す（Views() は単位ごと）。同じ名前空間を複数の単位が出力していれば、
// 設定の順で最初の単位のもの。単位ごとに見るテストは p.Views() を直接使う。
func viewsOf(p *project.Project) map[string]*ir.View {
	out := map[string]*ir.View{}
	for _, m := range p.Views() {
		for ns, v := range m {
			if out[ns] == nil {
				out[ns] = v
			}
		}
	}
	return out
}

// TestViewFromEntry は、tests/corpus/view（起点は @t.entry の付いた web の op）から hozoc が作る View
// （docs/ir.md「その名前空間から見た IR」）が文書の要求どおりであることを確かめる。
// 全体はゴールデン tests/corpus/view/testdata/expected/view.web.json と比べ（-update で更新）、
// 要求ごとの期待値はサブテスト（名前はその要求の文書の見出し）で明示する。
func TestViewFromEntry(t *testing.T) {
	p, v := loadViewCorpus(t)
	prog := v.Program

	t.Run("packages.md plugin（公開）/OpsWith", func(t *testing.T) {
		// viewer の op のうち @t.entry が付いたもの。配線（openUser）と、ignores を持つ void の配線（signOut）
		expect(t, "@t.entry の付いた op", entryOps(v), []string{"web.openUser", "web.signOut"})
		// SDK の OpsWith（docs/packages.md「`plugin`（公開）」）も同じものを返す。相乗りは、これで自分の対象を探す
		expect(t, "plugin.OpsWith", sdk.OpsWith(v, entryDecorator), []string{"web.openUser", "web.signOut"})
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
		expect(t, "config（web の単位での web の設定）", v.Config, ir.NamespaceConfig{Index: 2, Language: "go", Base: "view-dump", Out: "gen/web", Dir: "gen/web",
			Transpilers: []string{"view-dump"}, Options: map[string]map[string]any{}, Seams: map[string]string{}})
		// files は、単位の in に合ったユーザーのファイルと、読み込んだすべての語彙のファイル。web の単位は in を省いているので、
		// 読み込んだすべてのファイル（ユーザーのファイルを辞書順に、その後に語彙のファイルを辞書順に）
		expect(t, "files", v.Files, []string{"api.hozo", "t.hozo", "web.hozo",
			"hozo:vocab/actor.hozo", "hozo:vocab/business.hozo", "hozo:vocab/db.hozo", "hozo:vocab/fs.hozo", "hozo:vocab/http.hozo", "hozo:vocab/localstorage.hozo", "hozo:vocab/page.hozo", "hozo:vocab/prd.hozo", "hozo:vocab/proc.hozo", "hozo:vocab/usecase.hozo", "hozo:vocab/valid.hozo"})
		expect(t, "InUnit（viewer の宣言で、span.file が files にある）", []bool{v.InUnit("web.openUser"), v.InUnit("api.getUser"), v.InUnit("web.nope")}, []bool{true, false, false})
		// namespaces は出力のあるすべての名前空間（viewer を含む）。値は、その名前空間を出力する単位ごとの設定。
		// t はどの単位の対象でもないので入らない。page は、web が使う語彙の型（page.Route）の置き場の単位が出力する
		expect(t, "namespaces のキー", keysOf(v.Namespaces), []string{"api", "page", "web"})
		expect(t, "namespaces[api]", v.Namespaces["api"], []ir.NamespaceConfig{{Index: 1, Language: "go", Base: "view-dump", Out: "gen/api", Dir: "gen/api",
			Transpilers: []string{"view-dump"}, Options: map[string]map[string]any{}, Seams: map[string]string{}}})
		expect(t, "namespaces[web] は config だけ", v.Namespaces["web"], []ir.NamespaceConfig{v.Config})
		// ops はすべての op について
		expect(t, "ops のキー（export の有無によらず、すべての op）", keysOf(v.Ops), []string{
			"api.arrivedName", "api.currentName", "api.findByName", "api.findUser", "api.forgetUser", "api.getUser", "api.greeting", "api.guard", "api.idle", "api.issue", "api.keep",
			"api.login", "api.logout", "api.memberName", "api.mint", "api.nameOf", "api.onlyArrived",
			"api.renew", "api.renewAgain", "api.renewAudited", "api.renewDeclared", "api.restUser", "api.rotate", "api.sessionOf", "api.shownName", "api.signIn", "api.standingName", "api.standingNote", "api.tokenOrNone", "api.touch", "api.unauthorized", "api.verify", "api.viaAlias", "api.whenLive",
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
		expect(t, "名前空間の順", names, []string{"", "api", "t", "web", "actor", "business", "db", "fs", "http", "page", "prd", "proc", "usecase", "valid"})
		// nullable<api.Token>: 空になりうることは形が持ち、要素の型は optional にならない（docs/ir.md「state / exception / effect」）
		expect(t, "web.token の要素（別の名前空間の scalar は FQN）", typeStr(declOf(t, prog, "web.token").(*ir.State).Element), "api.Token")
		expect(t, "web.location の要素（語彙の型は FQN）", typeStr(declOf(t, prog, "web.location").(*ir.State).Element), "page.Route")
		expect(t, "fetchUser が呼ぶ op（修飾なしの参照も FQN）", renderBody(opOf(t, prog, "web.openUser").Body)[0], "val user = web.fetchUser(q: {id: id})")
		expect(t, "web.goLogin の宛先（状態は FQN）", declOf(t, prog, "web.goLogin").(*ir.Effect).Target, ir.EffectTarget(ir.BuiltinTarget{State: "web.location", Operation: ir.OpPut}))
		expect(t, "web.goLogin が書き込む状態", declOf(t, prog, "web.goLogin").(*ir.Effect).Writes, []string{"web.location"})
		// 空にするのは delete（docs/decisions.md「設計書から離れた決定」S2）
		expect(t, "web.removeToken の宛先", declOf(t, prog, "web.removeToken").(*ir.Effect).Target, ir.EffectTarget(ir.BuiltinTarget{State: "web.token", Operation: ir.OpDelete}))
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
		expect(t, "web.greet の Span の行は op の行", opOf(t, prog, "web.greet").Span.Start.Line, lineOf(t, "web.hozo", "fn greet("))
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
			"nick: string?",    // 正規化: T?? は T?
			"tags: string[]",   // array
			"attrs: api.Attrs", // インデックスの model への ref
		})
		attrs := declOf(t, prog, "api.Attrs").(*ir.Model)
		expect(t, "インデックスの model（Model.index。fields は空）",
			attrs.Index != nil && attrs.Index.Name == "name" && typeStr(attrs.Index.Key) == "string" && typeStr(attrs.Index.Value) == "string[]" && len(attrs.Fields) == 0, true)
		expect(t, "scalar の base が別の scalar への ref", typeStr(declOf(t, prog, "api.AdminToken").(*ir.Scalar).Base), "api.Token")
		expect(t, "絞り込まれた型 case", typeStr(opOf(t, prog, "web.adminPageVm").Params[0].Type), "api.PublicUser on web.UserKind.Admin")
		expect(t, "void は returns が null", opOf(t, prog, "web.signOut").Returns == nil, true)
		expect(t, "戻り値の型", typeStr(opOf(t, prog, "web.openUser").Returns), "web.UserPage")
		expect(t, "state の shape", declOf(t, prog, "api.users").(*ir.State).Shape, ir.StateList)
		expect(t, "state の shape（nullable）", declOf(t, prog, "web.token").(*ir.State).Shape, ir.StateNullable)
		expect(t, "state の shape（single）", declOf(t, prog, "web.location").(*ir.State).Shape, ir.StateSingle)
	})

	t.Run("ir.md 正規化/model extends", func(t *testing.T) {
		m := declOf(t, prog, "api.PublicUser").(*ir.Model)
		expect(t, "fields は、なくしたものを除いて元の並びのまま写す", renderFields(m), []string{"id: api.UserId", "name: string", "nick: string?", "tags: string[]", "attrs: api.Attrs"})
		expect(t, "extends", m.Extends, &ir.ModelExtends{Base: "api.User", Removed: []string{"secret", "hash"}})
		expect(t, "写したフィールドはデコレータも保つ", renderApplications(m.Fields[0].Decorators), []string{"@db.key"})
		user := declOf(t, prog, "api.User").(*ir.Model)
		expect(t, "元の model に extends はない", user.Extends == nil, true)
		// override で型を変える・デコレータを足す・なくす、フィールドを足す
		a := declOf(t, prog, "api.Admin").(*ir.Model)
		expect(t, "Admin の fields（override は元の位置のまま、足したフィールドは末尾）", renderFields(a), []string{
			"id: api.AdminId", "name: string", "secret: string", "nick: string?", "tags: string[]", "attrs: api.Attrs", "level: int",
		})
		expect(t, "Admin の extends（書かれた順）", a.Extends, &ir.ModelExtends{Base: "api.User", Removed: []string{"hash"}, Overridden: []string{"id", "name", "secret"}})
		expect(t, "override に付けたデコレータは、受け継いだものの後ろ", [][]string{renderApplications(a.Fields[0].Decorators), renderApplications(a.Fields[1].Decorators)}, [][]string{{"@db.key"}, {"@db.unique"}})
		// 説明: override したフィールドは override に書いたもの（なければなし）。していないフィールドは受け継ぐ
		expect(t, "override に書いた説明", docOf(fieldOf(t, prog, "api.Admin", "name").Description), "表に出す名前")
		expect(t, "override に説明がなければ説明なし（元の説明は受け継がない）", fieldOf(t, prog, "api.Admin", "secret").Description == nil, true)
		expect(t, "元のフィールドの説明は変わらない", docOf(fieldOf(t, prog, "api.User", "secret").Description), "表に出さない")
		expect(t, "PublicUser は secret をなくしている", indexOf(renderFields(m), "secret: string"), -1)
		// 位置: 受け継いだままのフィールドは元の位置、override したフィールドは override の位置
		expect(t, "受け継いだままのフィールドの span", fieldOf(t, prog, "api.Admin", "tags").Span, fieldOf(t, prog, "api.User", "tags").Span)
		expect(t, "override したフィールドの span の行", fieldOf(t, prog, "api.Admin", "id").Span.Start.Line, lineOf(t, "api.hozo", "override id: AdminId;"))
		// model の説明とデコレータは受け継がない
		expect(t, "Admin の説明は自分のもの", docOf(a.Description), "管理者")
	})

	t.Run("ir.md 正規化/形の語と状態の型", func(t *testing.T) {
		expect(t, "形の語は array / optional / そのままの型", renderFields(declOf(t, prog, "api.Shapes").(*ir.Model)), []string{
			"ids: api.UserId[]", "nick: string?", "one: api.Token", "twice: string?",
		})
		state := func(name string) []string {
			st := declOf(t, prog, name).(*ir.State)
			return []string{typeStr(st.Type), string(st.Shape), typeStr(st.Element)}
		}
		expect(t, "state users: list<User>", state("api.users"), []string{"api.User[]", "list", "api.User"})
		expect(t, "state current: Token?（nullable<Token> と同じ）", state("api.current"), []string{"api.Token?", "nullable", "api.Token"})
		expect(t, "state motd: string（single<string> と同じ）", state("api.motd"), []string{"string", "single", "string"})
		expect(t, "state token: nullable<api.Token>", state("web.token"), []string{"api.Token?", "nullable", "api.Token"})
		expect(t, "state location: single<page.Route>", state("web.location"), []string{"page.Route", "single", "page.Route"})
	})

	t.Run("ir.md 正規化/export と reads と never", func(t *testing.T) {
		find := opOf(t, prog, "api.findUser")
		expect(t, "export op は exported", []bool{find.Exported, opOf(t, prog, "api.getUser").Exported, opOf(t, prog, "api.nameOf").Exported, opOf(t, prog, "web.openUser").Exported}, []bool{true, true, false, false})
		expect(t, "reads は ReadRef の列一つ（直に書いた状態は direct、read の名前は named。重複は最初の出現に寄せる）", find.Reads,
			[]ir.ReadRef{ir.DirectRead{State: "api.users"}, ir.NamedRead{Read: "api.sessionsOf"}})
		expect(t, "reads を書いていなければ []", readStrings(opOf(t, prog, "api.getUser").Reads), []string{})
		expect(t, "never を返す op の returns", opOf(t, prog, "api.unauthorized").Returns, ir.Type(ir.Never()))
	})

	t.Run("ir.md classify/op の分類とケースの as", func(t *testing.T) {
		l := declOf(t, prog, "api.Lookup").(*ir.Classify)
		expect(t, "op の分類の subject（type は op の returns）", l.Subject, ir.ClassifySubject(ir.OpSubject{Op: "api.findUser", Type: ir.Optional(ir.Ref("api.User"))}))
		expect(t, "ケースの as（op の分類は state を持たない）", []any{l.Cases[0].As[0].State, typeStr(l.Cases[0].As[0].Type), l.Cases[0].As[0].By}, []any{"", "api.User", ir.NarrowByPresence})
		expect(t, "型で決まる分類", l.DecidedBy, ir.DecidedByType)
		s := declOf(t, prog, "api.SignedIn").(*ir.Classify)
		expect(t, "状態の分類の subject", s.Subject, ir.ClassifySubject(ir.StatesSubject{States: []string{"api.current"}}))
		expect(t, "ケースの as（状態の分類は state に FQN）", []any{s.Cases[0].As[0].State, typeStr(s.Cases[0].As[0].Type), s.Cases[0].As[0].By}, []any{"api.current", "api.Token", ir.NarrowByPresence})
		expect(t, "as を書かないケースの as は []、述語で決まる", []any{len(declOf(t, prog, "web.UserKind").(*ir.Classify).Cases[0].As), declOf(t, prog, "web.UserKind").(*ir.Classify).DecidedBy, declOf(t, prog, "web.Verified").(*ir.Classify).DecidedBy},
			[]any{0, ir.DecidedByPredicate, ir.DecidedByPredicate})
	})

	t.Run("ir.md 配線/状態の参照とアームの本体", func(t *testing.T) {
		expect(t, "currentName の本体（アームの throw）", renderBody(opOf(t, prog, "api.currentName").Body), []string{
			"val user = api.findUser(id: id)",
			"return match (user by api.Lookup) { Found => api.nameOf(user: user); else(Missing) => throw api.NotFound }",
		})
		expect(t, "tokenOrNone の本体（状態の名前は Atom の state、アームの値と none）", renderBody(opOf(t, prog, "api.tokenOrNone").Body), []string{
			"return match (state api.current by nullable) { Some => state api.current; None => none }",
		})
		expect(t, "greeting の本体（修飾して書いた状態の名前も同じ）", renderBody(opOf(t, prog, "api.greeting").Body), []string{"return state api.motd"})
		expect(t, "guard の本体（状態の分類での match、never を返す op の呼び出し）", renderBody(opOf(t, prog, "api.guard").Body), []string{
			"return match (by api.SignedIn) { Yes => state api.current; No => api.unauthorized() }",
		})
		arms := func(op string) []ir.MatchArm {
			return opOf(t, prog, op).Body.Statements[len(opOf(t, prog, op).Body.Statements)-1].(ir.ReturnStmt).Expr.(ir.MatchExpr).Match.Arms
		}
		narrowed := func(ns []ir.Narrowing) []string {
			out := []string{}
			for _, n := range ns {
				out = append(out, strings.TrimPrefix(n.State+": ", ": ")+typeStr(n.Type))
			}
			return out
		}
		// 値の分類と op の分類のアームは、いつも case(C, Ks, B)（docs/types.md「アームの中の対象の型」の 3）
		expect(t, "as のケースのアームでは、対象は、書いた型を、そのケースに絞り込んだ型", narrowed(arms("api.currentName")[0].Narrowed), []string{"api.User on api.Lookup.Found"})
		expect(t, "値がない側の残りのケースを受ける else のアームは、もとの型が外のまま", narrowed(arms("api.currentName")[1].Narrowed), []string{"api.User? on api.Lookup.Missing"})
		expect(t, "by nullable の Some だけのアームで T", narrowed(arms("api.tokenOrNone")[0].Narrowed), []string{"api.Token"})
		expect(t, "None のアームは外のまま", narrowed(arms("api.tokenOrNone")[1].Narrowed), []string{})
		expect(t, "状態の分類での match の narrowed は state を持つ", narrowed(arms("api.guard")[0].Narrowed), []string{"api.current: api.Token"})
		// 型を書かないケースも同じ形
		open := opOf(t, prog, "web.openUser").Body.Statements[1].(ir.ValStmt).Expr.(ir.MatchExpr).Match
		expect(t, "型を書かないケースの narrowed", narrowed(open.Arms[0].Narrowed), []string{"api.PublicUser on web.UserKind.Admin"})
	})

	t.Run("ir.md classify/残りのケース", func(t *testing.T) {
		typing := func(c *ir.Classify, name string) []any {
			cs := c.Case(name)
			if cs == nil || len(cs.As) != 1 {
				t.Fatalf("%s のケース %s の as = %+v", c.Name, name, cs)
			}
			return []any{cs.As[0].State, typeStr(cs.As[0].Type), cs.As[0].By}
		}
		// presence: 残りのケースは cases に入り、by は else、type は対象の型のまま。型で決まる
		m := declOf(t, prog, "api.Membership").(*ir.Classify)
		expect(t, "cases（残りのケースを含む）", []string{m.Cases[0].Name, m.Cases[1].Name}, []string{"Member", "Stranger"})
		expect(t, "残りのケースの as（presence）", typing(m, "Stranger"), []any{"", "api.User?", ir.NarrowByElse})
		expect(t, "decidedBy", m.DecidedBy, ir.DecidedByType)
		// variant: 残りのアームがちょうど一つなら、type はそのアームの型
		d := declOf(t, prog, "api.Detail").(*ir.Classify)
		expect(t, "型を書いたケースの as（variant）", typing(d, "Short"), []any{"", "string", ir.NarrowByVariant})
		expect(t, "残りのケースの as（variant、残りのアームが一つ）", typing(d, "Full"), []any{"", "api.User", ir.NarrowByElse})
		expect(t, "decidedBy", d.DecidedBy, ir.DecidedByType)
		// 配線: 残りのケースのアームは、名前で書く。narrowed は、対象の型が外と違うアームにだけ出る
		expect(t, "memberName の本体", renderBody(opOf(t, prog, "api.memberName").Body), []string{
			"val user = api.findUser(id: id)",
			"return match (user by api.Membership) { Member => api.nameOf(user: user); Stranger => throw api.NotFound }",
		})
		arms := func(op string) []ir.MatchArm {
			return opOf(t, prog, op).Body.Statements[len(opOf(t, prog, op).Body.Statements)-1].(ir.ReturnStmt).Expr.(ir.MatchExpr).Match.Arms
		}
		types := func(ns []ir.Narrowing) []string {
			out := []string{}
			for _, n := range ns {
				out = append(out, typeStr(n.Type))
			}
			return out
		}
		expect(t, "presence の残りのケースのアームは、もとの型が外のまま", types(arms("api.memberName")[1].Narrowed), []string{"api.User? on api.Membership.Stranger"})
		expect(t, "variant の残りのケースのアーム（残りのアームが一つ）は、もとの型が、そのアームの型", types(arms("api.shownName")[1].Narrowed), []string{"api.User on api.Detail.Full"})
	})

	t.Run("ir.md state / exception / effect と read/引数", func(t *testing.T) {
		// 例外の引数（運ぶ値）: api.Invalid(reason: string)。その場の model は合成しない
		exc := declOf(t, prog, "api.Invalid").(*ir.Exception)
		expect(t, "api.Invalid の params", renderOpParams(exc.Params), []string{"reason: string"})
		expect(t, "api.Invalid の @param", exc.Description.Params, []ir.DocParam{{Name: "reason", Text: "不正の理由"}})
		expect(t, "api.Invalid.Value は合成しない", prog.Lookup("api.Invalid.Value") == nil, true)
		expect(t, "値を運ばない例外の params は []", len(declOf(t, prog, "api.Unauthorized").(*ir.Exception).Params), 0)
		// 効果の引数（受け取る値）
		expect(t, "api.createSession の params", renderOpParams(declOf(t, prog, "api.createSession").(*ir.Effect).Params), []string{"session: api.Session"})
		expect(t, "web.goLogin の params は []", len(declOf(t, prog, "web.goLogin").(*ir.Effect).Params), 0)
		// read: 引数、結果の型、宛先の状態。付けたデコレータ（対象 read）と、何度も付けた @repeatable の適用（書かれた順）
		read := declOf(t, prog, "api.userById").(*ir.Read)
		expect(t, "api.userById の params / returns / state", []string{strings.Join(renderOpParams(read.Params), ", "), typeStr(read.Returns), read.State},
			[]string{"id: api.UserId", "api.User?", "api.users"})
		expect(t, "api.userById の decorators", renderApplications(read.Decorators), []string{"@db.find"})
		expect(t, "api.sessionsOf の decorators", renderApplications(declOf(t, prog, "api.sessionsOf").Common().Decorators), []string{
			"@db.list", "@db.where(column: \"revoked\", value: false)", "@db.where(column: \"kind\", value: \"web\")",
		})
		where := declOf(t, prog, "db.where").(*ir.Decorator)
		expect(t, "db.where は repeatable、対象は read", []any{where.Repeatable, where.Targets}, []any{true, []ir.DecoratorTarget{ir.TargetRead}})
		expect(t, "db.find は repeatable でない", declOf(t, prog, "db.find").(*ir.Decorator).Repeatable, false)
	})

	t.Run("ir.md 正規化/配線の呼び出しと throw の引数", func(t *testing.T) {
		// callee は、行き着いた宣言の種類で決まる: read、効果の宣言（named）、直に呼ぶ組み込みの効果（direct）、op
		expect(t, "signIn の本体", renderBody(opOf(t, prog, "api.signIn").Body), []string{
			"val user = api.userById(id: id)",
			"val token = match (user by api.Lookup) { Found => api.mint(user: user); Missing => throw api.NotFound }",
			"val session = match (user by api.Lookup) { Found => api.sessionOf(user: user, token: token); Missing => throw api.NotFound }",
			"api.sessions.post(value: session)",
			"api.current.post(value: token)",
			"api.current.put(value: token)",
			"return match (creds by api.Strength) { Weak => throw api.Invalid(reason: \"weak password\"); Strong => api.keep(token: token) }",
		})
		stmts := opOf(t, prog, "api.signIn").Body.Statements
		expect(t, "read の呼び出しの callee", stmts[0].(ir.ValStmt).Expr.(ir.CallExpr).Call.Callee, ir.Callee(ir.ReadCallee{Read: "api.userById"}))
		expect(t, "直に呼ぶ組み込みの効果の callee（list の post）", stmts[3].(ir.CallStmt).Call.Callee,
			ir.Callee(ir.EffectCallee{Effect: ir.DirectEffect{State: "api.sessions", Operation: ir.OpPost}}))
		expect(t, "修飾して書いた状態の put", stmts[5].(ir.CallStmt).Call.Callee,
			ir.Callee(ir.EffectCallee{Effect: ir.DirectEffect{State: "api.current", Operation: ir.OpPut}}))
		expect(t, "nullable の delete は引数なし", renderBody(opOf(t, prog, "api.keep").Body), []string{"api.current.delete()", "return token"})
		login := opOf(t, prog, "api.login").Body.Statements
		expect(t, "効果の宣言の呼び出しの callee", login[1].(ir.CallStmt).Call.Callee, ir.Callee(ir.EffectCallee{Effect: ir.NamedEffect{Effect: "api.createSession"}}))
		expect(t, "op の呼び出しの callee", login[0].(ir.ValStmt).Expr.(ir.CallExpr).Call.Callee, ir.Callee(ir.OpCallee{Op: "api.issue"}))
		// read と効果の呼び出しの bound は、いつも空
		expect(t, "read の呼び出しの bound", len(stmts[0].(ir.ValStmt).Expr.(ir.CallExpr).Call.Bound), 0)
	})

	t.Run("check.md 状態に触れるもの/配線からの呼び出しと節に書く要素", func(t *testing.T) {
		signIn := opOf(t, prog, "api.signIn")
		// 直に呼んだ効果は direct で、同じ宛先の宣言（createSession on sessions.post）とは別の効果。行の順
		expect(t, "signIn の F", effectStrings(signIn.Signature.Effects), []string{"api.sessions.post", "api.current.post", "api.current.put", "api.current.delete"})
		expect(t, "signIn の節に書いた effects（直に書いた要素）", effectStrings(signIn.Effects), []string{"api.sessions.post", "api.current.post", "api.current.put", "api.current.delete"})
		// 読む状態は、書き方によらない: 呼んだ read の宛先の状態
		expect(t, "signIn の R", signIn.Signature.Reads, []string{"api.users"})
		expect(t, "signIn の節に書いた reads（read の名前）", readStrings(signIn.Reads), []string{"api.userById"})
		expect(t, "signIn の E（throw は、引数があっても、例外がそれだけ）", signIn.Signature.Exceptions, []string{"api.NotFound", "api.Invalid"})
		expect(t, "read と効果を呼ぶ配線は純粋でない", []bool{signIn.Signature.Pure, opOf(t, prog, "api.findUser").Signature.Pure, opOf(t, prog, "web.greet").Signature.Pure}, []bool{false, false, false})
		// 葉は状態に触れない: 節も、運ばれるものもない葉は、いつも純粋
		for _, name := range []string{"api.issue", "api.verify", "api.whenLive", "api.mint"} {
			leaf := opOf(t, prog, name)
			expect(t, name+" は葉で、reads も effects も持たず、純粋", []any{leaf.Body == nil, len(leaf.Reads), len(leaf.Effects), leaf.Signature.Pure}, []any{true, 0, 0, true})
		}
	})

	t.Run("ir.md 正規化/その場の model", func(t *testing.T) {
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
		expect(t, "effects は EffectRef の列（効果の名前は named）", opOf(t, prog, "api.login").Effects, ir.NamedEffects("api.createSession"))
		expect(t, "配線で effects を省略すれば []", effectStrings(opOf(t, prog, "web.openUser").Effects), []string{})
	})

	t.Run("ir.md classify/閉じた分類", func(t *testing.T) {
		// 二つに分かれる分類も、両側がケース（docs/check.md「閉じた分類」）
		c := declOf(t, prog, "web.Verified").(*ir.Classify)
		expect(t, "二つに分かれる分類の cases", caseNames(c), []string{"Verified", "Unverified"})
		expect(t, "二つに分かれる分類の subject", c.Subject, ir.ClassifySubject(ir.TypeSubject{Type: ir.Ref("api.PublicUser")}))
		k := declOf(t, prog, "web.UserKind").(*ir.Classify)
		expect(t, "classify の cases", caseNames(k), []string{"Admin", "Member"})
		// 残りのケースを書かない分類も、型で決まる（docs/check.md「ケースの判定」）
		a := declOf(t, prog, "api.Arrival").(*ir.Classify)
		expect(t, "Arrival の cases と decidedBy", []any{caseNames(a), a.DecidedBy}, []any{[]string{"Arrived"}, ir.DecidedByType})
	})

	t.Run("ir.md 配線/else のアーム", func(t *testing.T) {
		armsOf := func(op string) []ir.MatchArm {
			stmts := opOf(t, prog, op).Body.Statements
			return stmts[len(stmts)-1].(ir.ReturnStmt).Expr.(ir.MatchExpr).Match.Arms
		}
		narrowed := func(a ir.MatchArm) []string {
			out := []string{}
			for _, n := range a.Narrowed {
				out = append(out, typeStr(n.Type))
			}
			return out
		}
		// else のアームの cases は、ほかのアームが並べなかったケース（分類の宣言の順）。else は true
		s := armsOf("api.standingName")
		expect(t, "standingName のアーム", []any{s[0].Cases, s[0].Else, s[1].Cases, s[1].Else},
			[]any{[]string{"Closed"}, false, []string{"Fresh", "Active", "Idle", "Dormant"}, true})
		// 受けるケースが二つ以上の else のアームは、受けるケースに絞り込んだ型で、もとの型は変えない（docs/types.md「アームの中の対象の型」の 3.1）
		expect(t, "standingName の else のアームの narrowed", narrowed(s[1]), []string{"api.User on api.Standing.Fresh | api.Standing.Active | api.Standing.Idle | api.Standing.Dormant"})
		expect(t, "standingName の本体", renderBody(opOf(t, prog, "api.standingName").Body), []string{
			"return match (user by api.Standing) { Closed => throw api.NotFound; else(Fresh, Active, Idle, Dormant) => api.nameOf(user: user) }",
		})
		// 名前で並べたアームの cases は、書かれた順
		n := armsOf("api.standingNote")
		expect(t, "standingNote のアーム", []any{n[0].Cases, n[0].Else, n[1].Cases, n[2].Cases, n[2].Else},
			[]any{[]string{"Idle", "Fresh"}, false, []string{"Active"}, []string{"Dormant", "Closed"}, true})
		// 受けるケースがちょうど一つの else のアームは、そのケースを名前で書いたアームと同じに絞り込まれる
		r := armsOf("api.arrivedName")
		expect(t, "arrivedName の else のアーム", []any{r[1].Cases, r[1].Else, narrowed(r[1])}, []any{[]string{"Member"}, true, []string{"api.User on api.Membership.Member"}})
		// ケースが一つの分類での match
		o := armsOf("api.onlyArrived")
		expect(t, "onlyArrived のアーム", []any{len(o), o[0].Cases, o[0].Else, narrowed(o[0])}, []any{1, []string{"Arrived"}, false, []string{"api.User on api.Arrival.Arrived"}})
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
		expect(t, "traced の params（型で受ける）", renderParams(declOf(t, prog, "web.traced").(*ir.Decorator).Params), []string{"key: string", "within: duration"})
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
		// フィールドの既定値は書けない（docs/grammar.md「model」）ので、リテラルはデコレータの引数で確かめる
		expect(t, "Query に付けたデコレータの引数（int、duration はナノ秒、boolean）", renderApplications(declOf(t, prog, "web.Query").Common().Decorators), []string{
			"@web.limits(page: 1, timeout: 30000000000ns, strict: false)",
		})
		expect(t, "Query の fields（既定値を持たない）", renderFields(declOf(t, prog, "web.Query").(*ir.Model)), []string{
			"id: api.UserId", "page: int?", "timeout: duration?", "strict: boolean?",
		})
	})

	t.Run("ir.md decorator/reads と effects の節、残りの引数、対象 effect", func(t *testing.T) {
		find := declOf(t, prog, "api.find").(*ir.Decorator)
		expect(t, "find の reads（any state の引数は、direct の state に $param:名前）", find.Reads, ir.StateReads("$param:table"))
		expect(t, "find の effects", find.Effects, []ir.EffectRef(nil))
		expect(t, "find の params の rest", []bool{find.Params[0].Rest, find.Params[1].Rest}, []bool{false, false})
		effects := declOf(t, prog, "api.effects").(*ir.Decorator)
		expect(t, "予約語の名前のデコレータ effects の params（残りの引数は rest、optional ではない）", effects.Params,
			[]ir.Param{{Name: "targets", Type: ir.AnyParam{Of: ir.AnyEffect}, Rest: true}})
		expect(t, "effects の effects / reads（節は書かれた順に連結）", []any{effects.Effects, effects.Reads}, []any{ir.NamedEffects("$param:targets"), ir.StateReads("api.sessions")})
		into := declOf(t, prog, "api.into").(*ir.Decorator)
		expect(t, "into の targets", into.Targets, []ir.DecoratorTarget{ir.TargetEffect})
		expect(t, "効果の宣言の decorators", renderApplications(declOf(t, prog, "api.renameUser").Common().Decorators), []string{"@api.into(table: api.users)"})
		// 標準の語彙の db: read と効果の宣言に付けるデコレータは、節を持たない
		expect(t, "db.list の targets と reads", []any{declOf(t, prog, "db.list").(*ir.Decorator).Targets, len(declOf(t, prog, "db.list").(*ir.Decorator).Reads)}, []any{[]ir.DecoratorTarget{ir.TargetRead}, 0})
		expect(t, "db.insert の targets", declOf(t, prog, "db.insert").(*ir.Decorator).Targets, []ir.DecoratorTarget{ir.TargetEffect})
		expect(t, "状態に付けた @db.table", renderApplications(declOf(t, prog, "api.users").Common().Decorators), []string{"@db.table"})
	})

	t.Run("ir.md 正規化/残りの引数と Op.carried", func(t *testing.T) {
		rotate := opOf(t, prog, "api.rotate")
		expect(t, "残りの引数に渡した値は list の Arg 一つ（一つずつ ref か refs）", renderApplications(rotate.Decorators), []string{
			"@api.effects(targets: [api.createSession, api.deleteSession | api.renameUser])",
		})
		expect(t, "rotate の Op.effects は空（葉は節を書けない）", rotate.Effects, []ir.EffectRef(nil))
		expect(t, "rotate の Op.carried（decorators の順、節の順、引数に書いた順）", rotate.Carried,
			ir.Carried{Reads: ir.StateReads("api.sessions"), Effects: ir.NamedEffects("api.createSession", "api.deleteSession", "api.renameUser")})
		idle := opOf(t, prog, "api.idle")
		expect(t, "何も渡さない残りの引数は items が []", renderApplications(idle.Decorators), []string{"@api.effects(targets: [])"})
		expect(t, "idle の Op.carried", idle.Carried, ir.Carried{Reads: ir.StateReads("api.sessions")})
		via := opOf(t, prog, "api.viaAlias")
		expect(t, "名前付きデコレータの固定した引数（ユニオンは展開）", renderApplications(via.Decorators), []string{
			"@api.effects(targets: [api.createSession, api.deleteSession | api.renameUser]) via api.both",
		})
		expect(t, "viaAlias の Op.carried", via.Carried, ir.Carried{Reads: ir.StateReads("api.sessions"), Effects: ir.NamedEffects("api.createSession", "api.deleteSession", "api.renameUser")})
		find := opOf(t, prog, "api.findByName")
		expect(t, "findByName の Op.reads は空（葉は節を書けない）", find.Reads, []ir.ReadRef(nil))
		expect(t, "findByName の Op.carried", find.Carried, ir.Carried{Reads: ir.StateReads("api.users")})
		expect(t, "@see は予約語のデコレータ名も指せる", find.Description.See, []string{"api.find", "api.effects"})
		// デコレータの付いていない op の carried は空（JSON では {reads: [], effects: []}）
		expect(t, "nameOf の Op.carried", opOf(t, prog, "api.nameOf").Carried, ir.Carried{})
	})

	t.Run("check.md 計算/デコレータが運ぶ reads と effects", func(t *testing.T) {
		// 葉の op が直に持つ効果は Op.carried.effects、直に読む状態は Op.carried.reads の要素の状態
		expect(t, "api.rotate（op の throws の relations は、運ばれた効果に掛かる）", sigOf(opOf(t, prog, "api.rotate").Signature), sig{
			E: []string{"api.Invalid"}, F: []string{"api.createSession", "api.deleteSession", "api.renameUser"},
			Rel: []string{"api.Invalid / api.createSession = prevented",
				"api.Invalid / api.deleteSession = prevented", "api.Invalid / api.renameUser = prevented"},
			Succ: []string{"api.createSession = kept", "api.deleteSession = kept", "api.renameUser = kept"},
		})
		expect(t, "api.rotate の R", opOf(t, prog, "api.rotate").Signature.Reads, []string{"api.sessions"})
		expect(t, "api.findByName の R（運ばれたもの）", opOf(t, prog, "api.findByName").Signature.Reads, []string{"api.users"})
		expect(t, "api.idle の F は空、R は運ばれた状態", []any{effectStrings(opOf(t, prog, "api.idle").Signature.Effects), opOf(t, prog, "api.idle").Signature.Reads}, []any{[]string{}, []string{"api.sessions"}})
		// 呼び出し元へは、配線が呼んだ効果と read と同じに伝わる
		renew := opOf(t, prog, "api.renew")
		expect(t, "api.renew の F（伝播）", effectStrings(renew.Signature.Effects), []string{"api.createSession", "api.deleteSession", "api.renameUser"})
		expect(t, "api.renew の R（伝播）", renew.Signature.Reads, []string{"api.sessions", "api.users"})
		expect(t, "api.renew の Op.carried は空（デコレータが付いていない）", renew.Carried, ir.Carried{})
		// 運ぶ節を持つデコレータの付いた配線: 運ばれたものは Op.carried に載り、本体が行き着くものの後ろに並ぶ
		audited := opOf(t, prog, "api.renewAudited")
		carried := ir.Carried{Reads: ir.StateReads("api.motd", "api.sessions"), Effects: ir.NamedEffects("api.touchUser", "api.renameUser")}
		expect(t, "api.renewAudited の Op.carried（decorators の順、節の順、引数に書いた順）", audited.Carried, carried)
		expect(t, "api.renewAudited の Op.reads / Op.effects は書いたもののまま（空）", []any{audited.Reads, audited.Effects}, []any{[]ir.ReadRef(nil), []ir.EffectRef(nil)})
		expect(t, "api.renewAudited（本体も行き着く renameUser は位置を変えず、本体とデコレータの分が違うので unknown。運ばれただけの touchUser は、関係を書いていないので unknown）",
			sigOf(audited.Signature), sig{
				E: []string{"api.Invalid"}, F: []string{"api.createSession", "api.deleteSession", "api.renameUser", "api.touchUser"},
				Rel: []string{"api.Invalid / api.createSession = prevented", "api.Invalid / api.deleteSession = prevented",
					"api.Invalid / api.renameUser = unknown", "api.Invalid / api.touchUser = unknown"},
				Succ: []string{"api.createSession = kept", "api.deleteSession = kept", "api.renameUser = kept", "api.touchUser = kept"},
			})
		expect(t, "api.renewAudited の R（本体、運ばれたもの）", audited.Signature.Reads, []string{"api.sessions", "api.users", "api.motd"})
		expect(t, "運ばれたものを持つ配線は純粋でない", audited.Signature.Pure, false)
		// 節を書いた配線: 書いた節は、運ばれたものを含む Sig と等しい（READS-2 / EFF-2 は出ない: check.txt）。運ばれた効果の rel は、書いた prevents
		declared := opOf(t, prog, "api.renewDeclared")
		expect(t, "api.renewDeclared の Op.carried", declared.Carried, carried)
		expect(t, "api.renewDeclared（書いた prevents が、運ばれた効果にも掛かる）", sigOf(declared.Signature), sig{
			E: []string{"api.Invalid"}, F: []string{"api.createSession", "api.deleteSession", "api.renameUser", "api.touchUser"},
			Rel: []string{"api.Invalid / api.createSession = prevented", "api.Invalid / api.deleteSession = prevented",
				"api.Invalid / api.renameUser = prevented", "api.Invalid / api.touchUser = prevented"},
			Succ: []string{"api.createSession = kept", "api.deleteSession = kept", "api.renameUser = kept", "api.touchUser = kept"},
		})
		// 配線に運ばれたものも、呼び出し元へ伝わる（見る側にもよらない）
		again := opOf(t, prog, "api.renewAgain")
		expect(t, "api.renewAgain の F（配線に運ばれた touchUser を含む）", effectStrings(again.Signature.Effects), []string{"api.createSession", "api.deleteSession", "api.renameUser", "api.touchUser"})
		expect(t, "api.renewAgain の R（配線に運ばれた motd を含む）", again.Signature.Reads, []string{"api.sessions", "api.users", "api.motd"})
		expect(t, "web から見た api.renewAgain", []any{effectStrings(v.Ops["api.renewAgain"].Effects), v.Ops["api.renewAgain"].Reads},
			[]any{effectStrings(again.Signature.Effects), again.Signature.Reads})
		// 見る側によらない
		expect(t, "web から見た api.rotate の effects", v.Ops["api.rotate"].Effects, opOf(t, prog, "api.rotate").Signature.Effects)
		expect(t, "デコレータの付いた op は純粋でない", opOf(t, prog, "api.idle").Signature.Pure, false)
	})

	t.Run("ir.md 配線", func(t *testing.T) {
		expect(t, "openUser の本体（{ id } は { id: id }、cases は書かれた順、return に束縛名）", renderBody(opOf(t, prog, "web.openUser").Body), []string{
			"val user = web.fetchUser(q: {id: id})",
			"val page = match (user by web.UserKind) { Admin => web.adminPageVm(user: user); Member => web.userPageVm(user: user) }",
			"return page",
		})
		expect(t, "welcome の本体（match の文、アームの none）", renderBody(opOf(t, prog, "web.welcome").Body), []string{
			"match (user by web.Verified) { Verified => web.greet(user: user); Unverified => none }",
		})
		expect(t, "login の本体（入れ子のレコードリテラルとフィールド参照）", renderBody(opOf(t, prog, "web.login").Body), []string{
			"return api.login(in: {creds: {name: form.name, password: form.password}})",
		})
		expect(t, "reload の本体（呼び出しの文）", renderBody(opOf(t, prog, "web.reload").Body), []string{
			"val user = web.fetchUser(q: {id: id})", "api.touch(id: id)", "return user",
		})
		expect(t, "葉の body は null", opOf(t, prog, "api.getUser").Body == nil, true)
	})

	t.Run("ir.md bind", func(t *testing.T) {
		expect(t, "binds", renderBinds(prog.Binds), []string{"web: bind web.token to api.*"})
	})

	t.Run("check.md 計算/Sig 効果を呼ぶ配線", func(t *testing.T) {
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
		apiView := viewsOf(p)["api"]
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

	t.Run("check.md 計算/reads", func(t *testing.T) {
		reads := func(name string) []string { return orEmptyStrings(opOf(t, prog, name).Signature.Reads) }
		expect(t, "配線の R: 呼んだ read の宛先の状態（行の順）", reads("api.findUser"), []string{"api.users", "api.sessions"})
		expect(t, "配線の R: 呼び出し先の R が伝わる", reads("api.currentName"), []string{"api.users", "api.sessions"})
		expect(t, "配線の R: match の対象と、アームの値が参照する状態", reads("api.tokenOrNone"), []string{"api.current"})
		expect(t, "配線の R: return の値が参照する状態", reads("api.greeting"), []string{"api.motd"})
		expect(t, "配線の R: match by C は C の対象の状態", reads("api.guard"), []string{"api.current"})
		// bind で埋まる位置の状態も読む（Call.bound）。別の名前空間の呼び出し先の R は、そのまま伝わる
		expect(t, "配線の R: Call.bound の状態", reads("web.fetchUser"), []string{"web.token"})
		expect(t, "葉の R は、運ばれたものだけ（なければ []）", reads("api.getUser"), []string{})
		// R は見る側によらない
		expect(t, "web から見た api.findUser の reads", v.Ops["api.findUser"].Reads, []string{"api.users", "api.sessions"})
	})

	t.Run("check.md 計算/純粋", func(t *testing.T) {
		pure := func(name string) bool { return opOf(t, prog, name).Signature.Pure }
		expect(t, "デコレータがなく、状態を読まず、効果を持たない葉は純粋", []bool{pure("api.nameOf"), pure("web.userPageVm")}, []bool{true, true})
		expect(t, "throws は純粋さを変えない", pure("api.unauthorized"), true)
		expect(t, "デコレータが付いていれば純粋でない", pure("api.getUser"), false)
		expect(t, "状態を読めば純粋でない", []bool{pure("api.findUser"), pure("api.tokenOrNone"), pure("api.currentName")}, []bool{false, false, false})
		expect(t, "効果を持てば純粋でない", []bool{pure("api.touch"), pure("web.greet")}, []bool{false, false})
		// 見る側によらない
		expect(t, "web から見た pure は Program と同じ", []bool{v.Ops["api.nameOf"].Pure, v.Ops["api.findUser"].Pure}, []bool{true, false})
	})

	t.Run("check.md 計算/配線 throw と返らないアーム", func(t *testing.T) {
		// アームの throw は、例外がそれだけで効果を持たない呼び出しとして数える
		expect(t, "api.currentName", sigOf(opOf(t, prog, "api.currentName").Signature), sig{E: []string{"api.NotFound"}, F: []string{}, Rel: []string{}, Succ: []string{}})
		// never を返す op の呼び出しの例外も伝わる
		expect(t, "api.guard", sigOf(opOf(t, prog, "api.guard").Signature), sig{E: []string{"api.Unauthorized"}, F: []string{}, Rel: []string{}, Succ: []string{}})
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
		expect(t, "signOut の Op.effects", effectStrings(opOf(t, prog, "web.signOut").Effects), []string{})
		expect(t, "signOut の Signature.effects", effectStrings(opOf(t, prog, "web.signOut").Signature.Effects), []string{"api.deleteSession", "web.removeToken"})
		expect(t, "reload の Signature.effects", effectStrings(opOf(t, prog, "web.reload").Signature.Effects), []string{"api.touchUser"})
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
		expect(t, "api から見た View にもない", viewsOf(p)["api"].Program.Lookup("web.getUser$in") == nil, true)
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
		greet := jsonDecl(t, web, "web.userPageVm")
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
		if _, ok := jsonDecl(t, api, "api.User")["extends"]; ok {
			t.Error("継承していない model に extends のキーを出さない")
		}
		expect(t, "kind は判別子", jsonDecl(t, api, "api.ClientError")["kind"], "union")
		expect(t, "アームの cases はケース名の列", jsonDecl(t, web, "web.welcome")["body"].(map[string]any)["statements"].([]any)[0].(map[string]any)["match"].(map[string]any)["arms"].([]any)[1].(map[string]any)["cases"], []any{"Unverified"})
		// 版 0.8: else のアームの cases は、それが受けるケース。else は、どのアームにも出す
		arms := jsonDecl(t, api, "api.standingName")["body"].(map[string]any)["statements"].([]any)[0].(map[string]any)["expr"].(map[string]any)["match"].(map[string]any)["arms"].([]any)
		expect(t, "else のアームの JSON", []any{arms[1].(map[string]any)["cases"], arms[1].(map[string]any)["else"], arms[0].(map[string]any)["else"]},
			[]any{[]any{"Fresh", "Active", "Idle", "Dormant"}, true, false})
		// 版 0.8: classify に hasDefault、defaultDescription、fromCondition のキーは、ない
		verified := jsonDecl(t, web, "web.Verified")
		for _, key := range []string{"hasDefault", "defaultDescription", "fromCondition"} {
			if _, ok := verified[key]; ok {
				t.Errorf("classify の JSON に %s のキーがある", key)
			}
		}
		// 版 0.9、0.16: 行の遷移（Effect.row）。書いていない効果は、row のキーを持たない。`as else` は classify と to を持たない。
		// 前のケースは、行の引数の型（kind が case）が言い、row には、ない
		expect(t, "行の遷移の JSON（post）", jsonDecl(t, api, "api.enrol")["row"], map[string]any{"param": "user", "classify": "api.Standing", "to": "Fresh"})
		expect(t, "行の遷移の JSON（put）", jsonDecl(t, api, "api.rest")["row"], map[string]any{"param": "user", "classify": "api.Standing", "to": "Idle"})
		expect(t, "行の引数の型の JSON（put、前のケースの並び）", jsonDecl(t, api, "api.rest")["params"].([]any)[0].(map[string]any)["type"],
			map[string]any{"kind": "case", "classify": "api.Standing", "cases": []any{"Fresh", "Active"}, "of": map[string]any{"kind": "ref", "target": "api.User"}})
		expect(t, "行の遷移の JSON（as else）", jsonDecl(t, api, "api.amend")["row"], map[string]any{"param": "user"})
		expect(t, "行の遷移の JSON（delete）", jsonDecl(t, api, "api.forget")["row"], map[string]any{"param": "user", "classify": "api.Enrolment", "to": "Unknown"})
		if _, ok := jsonDecl(t, api, "api.createSession")["row"]; ok {
			t.Error("行の遷移を書いていない効果に row のキーを出さない")
		}
		// 版 0.9: ない側のケース（Classify.absent、CaseTyping.by の none）。ない側のケースを持たない分類は、absent のキーを持たない
		enrolment := jsonDecl(t, api, "api.Enrolment")
		expect(t, "ない側のケースの JSON", []any{enrolment["absent"], enrolment["decidedBy"], enrolment["cases"].([]any)[2].(map[string]any)["as"].([]any)[0].(map[string]any)["by"]},
			[]any{"Unknown", "predicate", "none"})
		expect(t, "as else で書いた、ない側のケース", jsonDecl(t, api, "api.Membership")["absent"], "Stranger")
		if _, ok := jsonDecl(t, api, "api.Standing")["absent"]; ok {
			t.Error("ない側のケースを持たない分類に absent のキーを出さない")
		}
		expect(t, "ops のキーは FQN の辞書順", jsonKeysInOrder(t, marshalView(t, v), "ops"), keysOf(v.Ops))
	})
}

// stateCorpus は tests/corpus/state（その README.md に見出し → 構文の対応がある）。
const stateCorpus = "corpus/state"

// TestStateCorpusIR は、状態を分類で表す決定（docs/decisions.md「設計書から離れた決定」S1〜S5）の構文が、文書どおりの形で
// IR（internal/project の Load → Sema.Program）に載ることを確かめる。サブテストの名前は要求の文書の見出し。
// このコーパスは check だけを確かめる（emit は tests/corpus/state/README.md「emit のゴールデンがない理由」）。
func TestStateCorpusIR(t *testing.T) {
	p := project.LoadWith(project.Options{Root: stateCorpus, Stderr: io.Discard, PluginPath: repoPlugins(t)})
	if len(p.Diags) > 0 {
		var b strings.Builder
		for _, d := range p.Diags {
			fmt.Fprintf(&b, "%s[%s]: %s\n", d.Severity, d.Code, d.Message)
		}
		t.Fatalf("tests/corpus/state に診断がある（error も warning も想定しない）:\n%s", b.String())
	}
	prog := p.Sema.Program
	effect := func(name string) *ir.Effect { return declOf(t, prog, name).(*ir.Effect) }
	classify := func(name string) *ir.Classify { return declOf(t, prog, name).(*ir.Classify) }
	matches := func(op string) []ir.Match {
		var out []ir.Match
		for _, st := range opOf(t, prog, op).Body.Statements {
			switch x := st.(type) {
			case ir.MatchStmt:
				out = append(out, x.Match)
			case ir.ValStmt:
				if m, ok := x.Expr.(ir.MatchExpr); ok {
					out = append(out, m.Match)
				}
			}
		}
		return out
	}

	t.Run("ir.md state / exception / effect/State.shape", func(t *testing.T) {
		expect(t, "list", declOf(t, prog, "shop.carts").(*ir.State).Shape, ir.StateList)
		expect(t, "nullable", declOf(t, prog, "shop.token").(*ir.State).Shape, ir.StateNullable)
		expect(t, "single", declOf(t, prog, "shop.title").(*ir.State).Shape, ir.StateSingle)
		// element は optional にならない（STATE-3）
		expect(t, "nullable の element", typeStr(declOf(t, prog, "shop.token").(*ir.State).Element), "shop.Token")
	})

	t.Run("ir.md classify/subject", func(t *testing.T) {
		// 状態の分類: 基底の状態の FQN（書かれた順）
		expect(t, "Checkout の subject", classify("shop.Checkout").Subject, ir.ClassifySubject(ir.StatesSubject{States: []string{"shop.carts", "shop.orders"}}))
		expect(t, "Checkout の cases", caseNames(classify("shop.Checkout")), []string{"Shopping", "Ordered"})
		// 二つに分かれる状態の分類も、両側がケース（docs/check.md「閉じた分類」）
		loggedIn := classify("shop.LoggedIn")
		expect(t, "LoggedIn の subject", loggedIn.Subject, ir.ClassifySubject(ir.StatesSubject{States: []string{"shop.token"}}))
		expect(t, "LoggedIn の cases と decidedBy", []any{caseNames(loggedIn), loggedIn.DecidedBy}, []any{[]string{"LoggedIn", "LoggedOut"}, ir.DecidedByPredicate})
		// 値の分類
		expect(t, "LineKind の subject", classify("shop.LineKind").Subject, ir.ClassifySubject(ir.TypeSubject{Type: ir.Ref("shop.Line")}))
	})

	t.Run("ir.md state / exception / effect/Effect.target builtin", func(t *testing.T) {
		// 組み込みの効果: 書き込むのはその基底の状態（docs/check.md「状態の形」の「効果が書き込む状態」）
		for _, tt := range []struct {
			name, state string
			op          ir.Operation
		}{
			{"shop.addLine", "shop.carts", ir.OpPost},
			{"shop.rename", "shop.title", ir.OpPut},
			{"shop.removeToken", "shop.token", ir.OpDelete},
		} {
			expect(t, tt.name+" の target", effect(tt.name).Target, ir.EffectTarget(ir.BuiltinTarget{State: tt.state, Operation: tt.op}))
			expect(t, tt.name+" の writes", effect(tt.name).Writes, []string{tt.state})
		}
	})

	t.Run("ir.md state / exception / effect/Effect.target transition", func(t *testing.T) {
		both := []string{"shop.carts", "shop.orders"}
		for _, tt := range []struct {
			name   string
			target ir.TransitionTarget
			writes []string
		}{
			// 宛先が状態の分類: 書き込むのはその対象のすべての基底の状態
			{"shop.placeOrder", ir.TransitionTarget{Of: "shop.Checkout", From: ir.Cases("Shopping"), To: ir.Cases("Ordered")}, both},
			// CaseSet: "any" か、ケース名の列（書かれた順）
			{"shop.startOver", ir.TransitionTarget{Of: "shop.Checkout", From: ir.AnyCase(), To: ir.Cases("Shopping")}, both},
			{"shop.touchCheckout", ir.TransitionTarget{Of: "shop.Checkout", From: ir.Cases("Shopping", "Ordered"), To: ir.AnyCase()}, both},
			{"shop.signIn", ir.TransitionTarget{Of: "shop.LoggedIn", From: ir.AnyCase(), To: ir.Cases("LoggedIn")}, []string{"shop.token"}},
			// 宛先が基底の状態（組み込みの分類。ケースは、list が Empty / HasItems、nullable が None / Some）: 書き込むのはその状態
			{"shop.clearCart", ir.TransitionTarget{Of: "shop.carts", From: ir.Cases("HasItems"), To: ir.Cases("Empty")}, []string{"shop.carts"}},
			{"shop.saveToken", ir.TransitionTarget{Of: "shop.token", From: ir.Cases("None"), To: ir.Cases("Some")}, []string{"shop.token"}},
		} {
			expect(t, tt.name+" の target", effect(tt.name).Target, ir.EffectTarget(tt.target))
			expect(t, tt.name+" の writes", effect(tt.name).Writes, tt.writes)
		}
	})

	t.Run("ir.md 配線/Match.subject", func(t *testing.T) {
		// 基底の状態の、組み込みの分類での match: classify は状態の形
		byNullable := matches("shop.openTop")[0]
		expect(t, "match token by nullable の subject（value の atom が state）", renderAtom(byNullable.SubjectAtom()), "state shop.token")
		expect(t, "match token by nullable の classify", byNullable.Classify, "nullable")
		byList := matches("shop.openCart")[0]
		expect(t, "match carts by list の subject（value の atom が state）", renderAtom(byList.SubjectAtom()), "state shop.carts")
		expect(t, "match carts by list の classify", byList.Classify, "list")
		expect(t, "組み込みの分類のアーム（書かれた順）", renderBody(opOf(t, prog, "shop.openCart").Body), []string{
			"match (state shop.carts by list) { Empty => shop.showEmpty(); HasItems => shop.emptyCart() }",
		})
		// 状態の分類での match: 対象は classify が持つ
		byClassify := matches("shop.checkout")[0]
		expect(t, "match by Checkout の subject", byClassify.Subject, ir.MatchSubject(ir.ClassifyOwnedSubject{}))
		expect(t, "match by Checkout の classify", byClassify.Classify, "shop.Checkout")
		// 値の分類での match と、状態の分類での match が同じ op に並ぶ
		add := matches("shop.add")
		value, ok := add[0].Subject.(ir.ValueSubject)
		if !ok {
			t.Fatalf("match line by LineKind の subject = %#v", add[0].Subject)
		}
		expect(t, "match line by LineKind の atom", renderAtom(value.Atom), "line")
		expect(t, "match line by LineKind の classify", add[0].Classify, "shop.LineKind")
		expect(t, "match by LoggedIn の subject", add[1].Subject, ir.MatchSubject(ir.ClassifyOwnedSubject{}))
		expect(t, "match by LoggedIn の classify", add[1].Classify, "shop.LoggedIn")
		expect(t, "add の本体", renderBody(opOf(t, prog, "shop.add").Body), []string{
			"val normalized = match (line by shop.LineKind) { Single => shop.packSingle(line: line); Bulk => shop.packBulk(line: line) }",
			"match (by shop.LoggedIn) { LoggedIn => shop.addToCart(line: normalized); else(LoggedOut) => shop.showLogin() }",
		})
	})

	t.Run("ir.md JSON", func(t *testing.T) {
		got, err := json.MarshalIndent(prog, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		// jsonNamespace は View の JSON（program の下）を読むので、Program をその形に包む
		shop := jsonNamespace(t, map[string]any{"program": decodeJSON(t, got)}, "shop")
		expect(t, "State.shape", jsonDecl(t, shop, "shop.token")["shape"], "nullable")
		expect(t, "Classify.subject（states）", jsonDecl(t, shop, "shop.Checkout")["subject"], map[string]any{"kind": "states", "states": []any{"shop.carts", "shop.orders"}})
		expect(t, "Effect.target（builtin）", jsonDecl(t, shop, "shop.removeToken")["target"], map[string]any{"kind": "builtin", "state": "shop.token", "operation": "delete"})
		expect(t, "Effect.target（transition）", jsonDecl(t, shop, "shop.placeOrder")["target"], map[string]any{"kind": "transition", "of": "shop.Checkout", "from": []any{"Shopping"}, "to": []any{"Ordered"}})
		// CaseSet の "any" は文字列
		expect(t, "CaseSet の any", jsonDecl(t, shop, "shop.startOver")["target"].(map[string]any)["from"], "any")
		expect(t, "Effect.writes", jsonDecl(t, shop, "shop.placeOrder")["writes"], []any{"shop.carts", "shop.orders"})
		stmt := func(op string, i int) map[string]any {
			return jsonDecl(t, shop, op)["body"].(map[string]any)["statements"].([]any)[i].(map[string]any)
		}
		byNullable := stmt("shop.openTop", 0)["match"].(map[string]any)
		// 状態を名前で書いた対象は、value の atom が state（docs/ir.md「版」の 0.3）
		subject := byNullable["subject"].(map[string]any)
		expect(t, "Match.subject（value）の kind", subject["kind"], "value")
		atom := subject["atom"].(map[string]any)
		expect(t, "Match.subject の atom（state）", []any{atom["kind"], atom["state"], atom["path"]}, []any{"state", "shop.token", []any{}})
		expect(t, "Match.classify（標準の分類）", byNullable["classify"], "nullable")
		byClassify := stmt("shop.checkout", 0)["expr"].(map[string]any)["match"].(map[string]any)
		expect(t, "Match.subject（classify）", byClassify["subject"], map[string]any{"kind": "classify"})
		// 読み戻して書いても同じバイト列
		var back ir.Program
		if err := json.Unmarshal(got, &back); err != nil {
			t.Fatalf("Program の JSON を読み戻せない: %v", err)
		}
		again, _ := json.MarshalIndent(&back, "", "  ")
		if !bytes.Equal(again, got) {
			t.Errorf("Program の JSON を読み戻して書くと変わる:\n%s", firstDiff(string(got), string(again)))
		}
	})

	t.Run("types.md 式の型/match", func(t *testing.T) {
		// 状態の match の型も値の match と同じ決め方: none のアームがあれば optional(T)。`return placed;` が Order? に渡せる
		expect(t, "checkout の戻り値", typeStr(opOf(t, prog, "shop.checkout").Returns), "shop.Order?")
		expect(t, "checkout の本体", renderBody(opOf(t, prog, "shop.checkout").Body), []string{
			"val placed = match (by shop.Checkout) { Shopping => shop.submitOrder(); Ordered => none }",
			"return placed",
		})
	})

	t.Run("check.md 計算/手順 0（配線）の match", func(t *testing.T) {
		// 状態の match もアームごとの呼び出しとして計算する。一部のアームにしかない効果は succ が unknown
		expect(t, "openCart の Signature", sigOf(opOf(t, prog, "shop.openCart").Signature), sig{
			E: []string{}, F: []string{"shop.clearCart"}, Rel: []string{}, Succ: []string{"shop.clearCart = unknown"},
		})
		// 遷移の効果も、組み込みの効果と同じく伝播する（検査器は遷移を行をまたいで追わない。docs/check.md「状態の形」）
		expect(t, "checkout の Signature", sigOf(opOf(t, prog, "shop.checkout").Signature), sig{
			E: []string{"shop.EmptyCart"}, F: []string{"shop.placeOrder"},
			Rel:  []string{"shop.EmptyCart / shop.placeOrder = prevented"},
			Succ: []string{"shop.placeOrder = unknown"},
		})
		expect(t, "add の Signature", sigOf(opOf(t, prog, "shop.add").Signature), sig{
			E: []string{}, F: []string{"shop.addLine"}, Rel: []string{}, Succ: []string{"shop.addLine = unknown"},
		})
	})

	t.Run("check.md 状態と効果/STATE-5、計算/手順 0（配線）", func(t *testing.T) {
		// 葉の op は状態に触れない（reads / effects の節を書けない）。効果を持つのは、効果を呼ぶ配線
		for _, ns := range prog.Namespaces {
			for _, d := range ns.Decls {
				if o, ok := d.(*ir.Op); ok && o.Body == nil && len(o.Effects)+len(o.Reads) > 0 {
					t.Errorf("葉の op %s が reads / effects の節を持っている: %v %v", o.Name, o.Reads, o.Effects)
				}
			}
		}
		expect(t, "storeToken の effects（節に書いたもの）", effectStrings(opOf(t, prog, "shop.storeToken").Effects), []string{"shop.saveToken"})
		expect(t, "storeToken の本体（効果の呼び出し）", renderBody(opOf(t, prog, "shop.storeToken").Body), []string{"shop.saveToken(token: token)"})
		expect(t, "markLoggedIn の effects", effectStrings(opOf(t, prog, "shop.markLoggedIn").Effects), []string{"shop.signIn"})
		login := opOf(t, prog, "shop.login")
		expect(t, "login の本体", renderBody(login.Body), []string{"shop.storeToken(token: token)", "shop.markLoggedIn()"})
		expect(t, "login の effects（宣言）", effectStrings(login.Effects), []string{})
		// F は呼び出しの effects を行の順に連結したもの
		expect(t, "login の Signature", sigOf(login.Signature), sig{
			E: []string{}, F: []string{"shop.saveToken", "shop.signIn"}, Rel: []string{},
			Succ: []string{"shop.saveToken = kept", "shop.signIn = kept"},
		})
	})
}

// TestViewDeliveredToPlugin は、実物の `hozoc emit`（internal/app の App と生成した cli.Main。tests/selfcheck_test.go と同じ組み立て）が
// view-dump プラグイン（tests/corpus/view/plugins/view-dump。`go run` で動く）に渡す View が、internal/project の web の View と
// 同じであることを確かめる。プラグインは受け取った view を <view.config.dir>/view.json に書き出すので、それを読み戻して比べる。
// コーパスは一時ディレクトリに写してモジュールのルートにする（copyCorpusAsModule）。
func TestViewDeliveredToPlugin(t *testing.T) {
	requireGo(t)
	root, _ := copyCorpusAsModule(t, "view")
	var stdout, stderr bytes.Buffer
	code := hozocCorpus(t, root, []string{"emit"}, &stdout, &stderr)
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
		t.Errorf("プラグインが受け取った View が、internal/project が作った web の View と違う:\n%s", firstDiff(string(wantJSON), string(gotJSON)))
	}
	// プラグインが書いたバイト列そのもの（SDK の ir.View を 2 スペースで書き直したもの）も同じになる（docs/ir.md「JSON」の決定性）
	if !bytes.Equal(data, wantJSON) {
		t.Errorf("プラグインが書いた view.json のバイト列が web の View の JSON と違う:\n%s", firstDiff(string(wantJSON), string(data)))
	}
	expect(t, "view.version", got.Version, ir.Version)
	expect(t, "view.viewer", got.Viewer, "web")
	expect(t, "view.config.dir", got.Config.Dir, "gen/web")
	expect(t, "view.config.transpilers", got.Config.Transpilers, []string{"view-dump"})
	expect(t, "view.config.base", got.Config.Base, "view-dump")
	expect(t, "view.files（ユーザーのファイル）", got.Files[:3], []string{"api.hozo", "t.hozo", "web.hozo"})
	expect(t, "view.namespaces のキー", keysOf(got.Namespaces), []string{"api", "page", "web"})
	expect(t, "@t.entry の付いた op", entryOps(&got), []string{"web.openUser", "web.signOut"})

	// docs/plugin-protocol.md「emit」: 応答は files と warnings だけ（select はない）。hozoc 本体はコードを出力しないので、
	// 書き出されるのは、トランスパイラが返した view.json と、出力先ごとの記録だけ。api と page の単位も、同じ基底で出力を持つ
	t.Run("plugin-protocol.md emit/応答", func(t *testing.T) {
		m, dir := viewDumpBuilt(t, root)
		req := sdk.EmitRequest{Protocol: hostplugin.Protocol, Direction: "emit", Transpiler: "view-dump", View: want}
		var resp sdk.EmitResponse
		if err := (hostplugin.Runner{Root: root, Stderr: io.Discard}).Run(m, dir, &req, &resp); err != nil {
			t.Fatal(err)
		}
		if len(resp.Files) != 1 || resp.Files[0].Path != "gen/web/view.json" || len(resp.Warnings) != 0 || len(resp.Errors) != 0 {
			t.Errorf("view-dump の応答 = %+v", resp)
		}
		var written []string
		for _, l := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
			if p, ok := strings.CutPrefix(l, "written: "); ok {
				written = append(written, p)
			}
		}
		expect(t, "hozoc emit が書いたファイル", written, []string{"gen/api/view.json", "gen/page/view.json", "gen/web/view.json"})
	})

	// docs/plugin-protocol.md「要求と応答」: protocol が合わなければ code "protocol"（MAN-2）、direction か、要求の transpiler を
	// 実装していなければ code "unsupported"（MAN-5）で、どちらも終了コード 2。同じプラグインを internal/plugin.Runner で直接起動して確かめる。
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
	t.Run("plugin-protocol.md 要求と応答/知らない transpiler", func(t *testing.T) {
		m, dir := viewDumpBuilt(t, root)
		req := sdk.EmitRequest{Protocol: hostplugin.Protocol, Direction: "emit", Transpiler: "view-nope", View: want}
		var resp sdk.EmitResponse
		err := hostplugin.Runner{Root: root, Stderr: io.Discard}.Run(m, dir, &req, &resp)
		var pe *hostplugin.PluginError
		if !errors.As(err, &pe) || pe.Code != hostplugin.CodeUnsupported || !strings.Contains(pe.Message, "view-nope") {
			t.Fatalf("知らない transpiler の要求に対する誤り = %v, want *PluginError{Code: %q}", err, hostplugin.CodeUnsupported)
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
	out := sig{E: orEmptyStrings(s.Exceptions), F: effectStrings(s.Effects), Rel: []string{}, Succ: []string{}}
	for _, r := range s.Relations {
		out.Rel = append(out.Rel, fmt.Sprintf("%s / %s = %s", r.Exception, r.Effect, r.State))
	}
	for _, e := range s.SuccessEffects {
		out.Succ = append(out.Succ, fmt.Sprintf("%s = %s", e.Effect, e.State))
	}
	return out
}

// effectStrings は効果の列の表示（名前で書いた効果は宣言の FQN、直に書いた効果は "<状態の FQN>.<操作>"）。
func effectStrings(es []ir.EffectRef) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.String())
	}
	return out
}

// readStrings は reads の要素の列の表示（read の宣言の FQN か、状態の FQN）。
func readStrings(rs []ir.ReadRef) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.String())
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
		out = append(out, f.Name+": "+typeStr(f.Type))
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
		s += " " + string(r.Kind) + " effects"
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
	case ir.StateAtom:
		return "state " + strings.Join(append([]string{x.State}, x.Path...), ".")
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
	s := c.Callee.String() + "(" + strings.Join(args, ", ") + ")"
	if len(c.Bound) > 0 {
		s += " bound[" + strings.Join(renderBound(c.Bound), ", ") + "]"
	}
	return s
}

func renderMatch(m ir.Match) string {
	var arms []string
	for _, a := range m.Arms {
		// 名前で並べたアームは `A, B`、else のアームは `else(受けるケース)`
		cases := strings.Join(a.Cases, ", ")
		if a.Else {
			cases = "else(" + cases + ")"
		}
		body := fmt.Sprintf("<%T>", a.Body)
		switch b := a.Body.(type) {
		case ir.CallExpr:
			body = renderCall(b.Call)
		case ir.AtomExpr:
			body = renderAtom(b.Atom)
		case ir.NoneBody:
			body = "none"
		case ir.ThrowBody:
			body = "throw " + b.Exception
			if len(b.Args) > 0 {
				var args []string
				for _, a := range b.Args {
					args = append(args, a.Param+": "+renderAtom(a.Value))
				}
				body += "(" + strings.Join(args, ", ") + ")"
			}
		}
		arms = append(arms, cases+" => "+body)
	}
	return renderMatchHead(m) + " { " + strings.Join(arms, "; ") + " }"
}

// renderMatchHead は match の頭を、対象の形（docs/ir.md「配線」の Match.subject）ごとに書く。
func renderMatchHead(m ir.Match) string {
	switch s := m.Subject.(type) {
	case ir.ValueSubject:
		return "match (" + renderAtom(s.Atom) + " by " + m.Classify + ")"
	case ir.ClassifyOwnedSubject:
		return "match (by " + m.Classify + ")"
	}
	return "match (<nil> by " + m.Classify + ")"
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
		case ir.ValStmt:
			out = append(out, "val "+s.Name+" = "+renderExpr(s.Expr))
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
