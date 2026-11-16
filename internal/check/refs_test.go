package check

import (
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// reads と effects の要素の、二つの書き方（docs/check.md「直に書く要素」）: 読む状態は書き方によらず、効果は書き方ごとに別。

// readDecl は read の宣言。
func readDecl(fqn, state string) *ir.Read {
	return &ir.Read{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)}, State: state}
}

// refsProgram は、read の宣言と、直に書いた要素を持つ store。
//   - findByEmail: 名前で書いた reads（read byEmail on users）
//   - listUsers: 直に書いた reads（users）
//   - audit: 名前で書いた reads（allLogs）と、直に書いた reads（users）
//   - addUser: 名前で書いた effects（createUser on users.post）
//   - putUser: 直に書いた effects（users.post）
//   - both: reads byEmail と、デコレータが運ぶ直に書いた users と allLogs
func refsProgram(extra ...ir.Decl) *ir.Program {
	user := ir.Ref("store.User")
	find := leaf("store.findByEmail", ir.Optional(user), param("email", ir.String()))
	find.Reads = ir.NamedReads("store.byEmail")
	list := reads(leaf("store.listUsers", ir.Array(user)), "store.users")
	audit := leaf("store.audit", nil)
	audit.Reads = []ir.ReadRef{ir.NamedRead{Read: "store.allLogs"}, ir.DirectRead{State: "store.users"}}
	add := leaf("store.addUser", nil, param("u", user))
	add.Effects = ir.NamedEffects("store.createUser")
	put := leaf("store.putUser", nil, param("u", user))
	put.Effects = []ir.EffectRef{ir.DirectEffect{State: "store.users", Operation: ir.OpPost}}
	both := leaf("store.both", nil)
	both.Reads = ir.NamedReads("store.byEmail")
	both.Carried = ir.Carried{Reads: []ir.ReadRef{ir.DirectRead{State: "store.users"}, ir.NamedRead{Read: "store.allLogs"}}}
	decls := []ir.Decl{
		model("store.User", field("email", ir.String())),
		state("store.users", ir.StateList, user),
		state("store.logs", ir.StateList, ir.String()),
		readDecl("store.byEmail", "store.users"),
		readDecl("store.allLogs", "store.logs"),
		eff("store.createUser", "store.users", ir.OpPost),
		find, list, audit, add, put, both,
	}
	return program(nil, namespace("store", append(decls, extra...)...))
}

// TestReadsNamed は、名前で書いた reads が、その read の宛先の状態として R に入ること。書き方が違っても、同じ状態は一つ。
func TestReadsNamed(t *testing.T) {
	w := wire("store.w", nil, nil,
		cst("a", call("store.findByEmail", carg("email", nameAtom("e")))),
		cst("b", call("store.listUsers")),
		cs(call("store.audit")),
	)
	p := refsProgram(w)
	Compute(p)
	wantStrings(t, "R(名前で書いた read)", sigOf(t, p, "store.findByEmail").Reads, []string{"store.users"})
	wantStrings(t, "R(直に書いた状態)", sigOf(t, p, "store.listUsers").Reads, []string{"store.users"})
	wantStrings(t, "R(混ぜたもの。書かれた順)", sigOf(t, p, "store.audit").Reads, []string{"store.logs", "store.users"})
	wantStrings(t, "R(書いたものの後ろに、運ばれたもの。同じ状態は一つ)", sigOf(t, p, "store.both").Reads, []string{"store.users", "store.logs"})
	wantStrings(t, "R(配線。read の名前は伝わらず、状態が伝わる)", sigOf(t, p, "store.w").Reads, []string{"store.users", "store.logs"})
	if sigOf(t, p, "store.findByEmail").Pure {
		t.Errorf("名前で書いた read を読む op は、純粋でない")
	}
}

// TestReads2Named は READS-2: 配線に書いた reads は、要素の状態の集合で比べる。read の名前で書いても、状態で書いても同じ。
func TestReads2Named(t *testing.T) {
	body := func(name string) *ir.Op {
		return wire(name, nil, nil, cst("a", call("store.findByEmail", carg("email", nameAtom("e")))), cs(call("store.audit")))
	}
	byName := body("store.byName")
	byName.Reads = ir.NamedReads("store.byEmail", "store.allLogs")
	byState := reads(body("store.byState"), "store.users", "store.logs")
	mixed := body("store.mixed")
	mixed.Reads = []ir.ReadRef{ir.DirectRead{State: "store.logs"}, ir.NamedRead{Read: "store.byEmail"}}
	short := body("store.short")
	short.Reads = ir.NamedReads("store.byEmail") // logs が足りない
	p := refsProgram(byName, byState, mixed, short)
	diags := withCode(Check(p, cfgFor("store")), "READS-2")
	if len(diags) != 1 {
		t.Fatalf("READS-2 = %d 件, want 1（store.short だけ）: %s", len(diags), messages(diags))
	}
	missing, _ := diags[0].Data["missing"].([]string)
	wantStrings(t, "Data.missing", missing, []string{"store.logs"})
}

// TestEffectsDirect は、直に書いた効果（状態の組み込みの効果そのもの）が F に入り、同じ宛先の宣言とは別の効果として
// 数えられること。rel と succ も、効果ごと。
func TestEffectsDirect(t *testing.T) {
	post := ir.DirectEffect{State: "store.users", Operation: ir.OpPost}
	put := leaf("store.putOrFail", nil)
	put.Effects = []ir.EffectRef{post}
	put.Throws = []ir.ThrowsClause{throws([]string{"store.Full"}, prevents())}
	w := wire("store.w", nil, nil,
		cs(call("store.addUser", carg("u", nameAtom("u")))),
		cs(call("store.putUser", carg("u", nameAtom("u")))),
		cs(call("store.putOrFail")),
	)
	p := refsProgram(exc("store.Full"), put, w)
	Compute(p)
	wantStrings(t, "F(直に書いた効果)", effectNames(sigOf(t, p, "store.putUser").Effects), []string{"store.users.post"})
	sig := sigOf(t, p, "store.w")
	if len(sig.Effects) != 2 || sig.Effects[0] != (ir.NamedEffect{Effect: "store.createUser"}) || sig.Effects[1] != ir.EffectRef(post) {
		t.Fatalf("F(配線) = %v, want [宣言 store.createUser、直に書いた store.users.post]（宛先が同じでも別の効果）", sig.Effects)
	}
	// 行 3 の Full の出どころ: 行 1・2 の効果はどちらも起きている（行 j < i）
	if got := relOf(t, sig, "store.Full", "store.createUser"); got != ir.Kept {
		t.Errorf("rel(Full, createUser) = %s, want kept", got)
	}
	if got := relOf(t, sig, "store.Full", post); got != ir.Kept {
		t.Errorf("rel(Full, users.post) = %s, want kept", got)
	}
	if got := relOf(t, sigOf(t, p, "store.putOrFail"), "store.Full", post); got != ir.Prevented {
		t.Errorf("葉の rel(Full, users.post) = %s, want prevented", got)
	}
	if got := succOf(t, sig, post); got != ir.Kept {
		t.Errorf("succ(users.post) = %s, want kept", got)
	}
}

// TestEff2Direct は EFF-2: 配線に書いた effects は、効果の同一性で比べる。直に書いた効果は、同じ宛先の宣言の代わりにならない。
func TestEff2Direct(t *testing.T) {
	post := ir.DirectEffect{State: "store.users", Operation: ir.OpPost}
	named := wire("store.viaNamed", nil, nil, cs(call("store.addUser", carg("u", nameAtom("u")))))
	named.Effects = []ir.EffectRef{post} // 呼び出し先の効果は、宣言 createUser
	direct := wire("store.viaDirect", nil, nil, cs(call("store.putUser", carg("u", nameAtom("u")))))
	direct.Effects = []ir.EffectRef{post} // 合っている
	p := refsProgram(named, direct)
	diags := withCode(Check(p, cfgFor("store")), "EFF-2")
	if len(diags) != 1 {
		t.Fatalf("EFF-2 = %d 件, want 1（store.viaNamed だけ）: %s", len(diags), messages(diags))
	}
	missing, _ := diags[0].Data["missing"].([]string)
	extra, _ := diags[0].Data["extra"].([]string)
	wantStrings(t, "Data.missing（宣言は FQN）", missing, []string{"store.createUser"})
	wantStrings(t, "Data.extra（直に書いた効果は <状態の FQN>.<操作>）", extra, []string{"store.users.post"})
}
