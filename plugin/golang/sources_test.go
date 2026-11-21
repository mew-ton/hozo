package golang

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、ThrowSources（docs/ide-chain.md「テストの規約」の「インテグレーションテストの作り直し」の「Hozo が作る、例外のケース」）:
// 入口から辿れる、例外の出どころの一覧と、Hozo が作れるかの判定。IR は、手で組む（Signature.exceptions は、検査が計算したものを、
// 書き写す）。

// valCall は `val name = <op>(args…);`。args は、引数の名前（呼ぶ先の引数と、同じ名前の値を渡す）。
func valCall(name, opFQN string, args ...string) ir.Statement {
	c := ir.Call{Callee: ir.OpCallee{Op: opFQN}}
	for _, a := range args {
		c.Args = append(c.Args, ir.CallArg{Param: a, Value: ir.NameAtom{Path: []string{a}}})
	}
	return ir.ValStmt{Name: name, Expr: ir.CallExpr{Call: c}}
}

func valRead(name, readFQN string) ir.Statement {
	return ir.ValStmt{Name: name, Expr: ir.CallExpr{Call: ir.Call{Callee: ir.ReadCallee{Read: readFQN}}}}
}

// matchOn は `match (subject by classify) { arms }`。subject が "" なら、状態の分類での match（`match by classify`）。
func matchOn(subject, classifyFQN string, arms ...ir.MatchArm) ir.Statement {
	m := ir.Match{Subject: ir.ClassifyOwnedSubject{}, Classify: classifyFQN, Arms: arms}
	if subject != "" {
		m.Subject = ir.ValueSubject{Atom: ir.NameAtom{Path: strings.Split(subject, ".")}}
	}
	return ir.MatchStmt{Match: m}
}

func armThrow(exception string, cases ...string) ir.MatchArm {
	return ir.MatchArm{Cases: cases, Body: ir.ThrowBody{Exception: exception}}
}

func armEffect(effectFQN string, cases ...string) ir.MatchArm {
	return ir.MatchArm{Cases: cases, Body: ir.CallExpr{Call: ir.Call{Callee: ir.EffectCallee{Effect: ir.NamedEffect{Effect: effectFQN}}}}}
}

func armAtom(cases ...string) ir.MatchArm {
	return ir.MatchArm{Cases: cases, Body: ir.AtomExpr{Atom: ir.NameAtom{Path: []string{"v"}}}}
}

// neverOp は、never を返す op: throws の例外を、いつも投げる。
func neverOp(fqn, exception string) *ir.Op {
	o := with(op(fqn, ir.Never()), throwing(exception))
	o.Throws = []ir.ThrowsClause{{Exceptions: []string{exception}}}
	return o
}

// sourcesOf は、入口 entry の出どころを、一行ずつの文字列にする: `<例外> <出どころ> made|human <にせものに言うこと>`。
func sourcesOf(t *testing.T, v *ir.View, entry string) []string {
	t.Helper()
	kit, err := TestKitOf(v, "")
	if err != nil || kit.Blocked != "" {
		t.Fatalf("TestKitOf: %v, Blocked = %q", err, kit.Blocked)
	}
	var out []string
	for _, s := range ThrowSources(v.Program, kit, entry) {
		line := s.Exception + " " + s.Label + " human"
		if s.Made {
			line = s.Exception + " " + s.Label + " made"
		}
		for _, st := range s.Steps {
			at := strings.Join(st.Path, ".")
			switch st.Verb {
			case StepThrows:
				line += " " + at + ".Throws(" + st.Exception + ")"
			case StepReturnsNone:
				line += " " + at + ".Returns(nil)"
			case StepReturnsBool:
				line += fmt.Sprintf(" %s.Returns(%t)", at, st.Bool)
			case StepIs:
				line += " " + at + ".Is(" + st.Classify + "." + st.Case + ")"
			}
		}
		out = append(out, line)
	}
	return out
}

func wantSources(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("出どころ =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// 述語で決まる分類の match の、throw のアームは、判定のにせもので、入らせる（アームの表の 3）。決め手が、入口の入力でも、作れる。
// throw でないアームが、一つだけなら、その中の出どころに、何もしなくても届く（表の 1）: effect fn が投げる例外は、端のにせものが投げる。
func TestThrowSourcesPredicateAndEffect(t *testing.T) {
	add := effect("app.addItem", ir.BuiltinTarget{State: "app.items", Operation: ir.OpPost}, param("v", ir.String()))
	add.Throws = []string{"app.Taken"}
	v := viewOf("app", namespace("app",
		exception("app.Rejected"), exception("app.Taken"),
		state("app.items", ir.Array(ir.String())),
		add,
		classify("app.Size", ir.TypeSubject{Type: ir.String()}, ir.DecidedByPredicate, "Big", "Small"),
		with(op("app.add", nil, param("v", ir.String())), throwing("app.Rejected", "app.Taken"),
			wired(matchOn("v", "app.Size", armEffect("app.addItem", "Small"), armThrow("app.Rejected", "Big")))),
	))
	wantSources(t, sourcesOf(t, v, "app.add"),
		"app.Rejected app.add.Big made Size.Is(app.Size.Big)",
		"app.Taken app.addItem made Items.AddItem.Throws(app.Taken)")
}

// 状態の分類の述語で決まる match の、throw のアームは、状態の単位のにせものの述語で、入らせる: そのケースだけ、真。
func TestThrowSourcesStatePredicate(t *testing.T) {
	v := viewOf("app", namespace("app",
		exception("app.Stale"),
		state("app.items", ir.Array(ir.String())),
		classify("app.Sync", ir.StatesSubject{States: []string{"app.items"}}, ir.DecidedByPredicate, "UpToDate", "OutOfDate"),
		op("app.leaf", nil),
		with(op("app.run", nil), throwing("app.Stale"),
			wired(matchOn("", "app.Sync", armCall("app.leaf", "UpToDate"), armThrow("app.Stale", "OutOfDate")))),
	))
	wantSources(t, sourcesOf(t, v, "app.run"),
		"app.Stale app.run.OutOfDate made Sync.UpToDate.Returns(false) Sync.OutOfDate.Returns(true)")
}

// 道の上の、値が要るアームで、throw でないアームが、ほかにもあれば、その中の出どころは、作れない（表の 4）。一つだけなら、作れる（表の 1）。
// 値がないことで入るアームは、値の出どころ（read）が、「ない」を返す（表の 2）。
func TestThrowSourcesPathArms(t *testing.T) {
	build := func(arms ...ir.MatchArm) *ir.View {
		return viewOf("app", namespace("app",
			exception("app.Bad"), exception("app.Missing"),
			state("app.items", ir.Array(ir.String())),
			read("app.itemById", "app.items", ir.Optional(ir.String())),
			with(op("app.check", nil, param("v", ir.String())), throwing("app.Bad")),
			with(op("app.run", nil), throwing("app.Bad", "app.Missing"),
				wired(valRead("v", "app.itemById"), matchOn("v", ir.ClassifyNullable, arms...))),
		))
	}
	// Some => check(v)（throw でないアームは、これ一つ）、None => throw Missing
	wantSources(t, sourcesOf(t, build(armCall("app.check", ir.CaseSome), armThrow("app.Missing", ir.CaseNone)), "app.run"),
		"app.Bad app.check made Leaves.Check.Throws(app.Bad)",
		"app.Missing app.run.None made Items.ItemById.Returns(nil)")
	// Some => check(v)、None => v（throw でないアームが、二つ）: Some は、値が要るので、入らせられない
	got := sourcesOf(t, build(armCall("app.check", ir.CaseSome), armAtom(ir.CaseNone)), "app.run")
	if len(got) != 2 || got[0] != "app.Bad app.check human" || got[1] != "app.Missing  human" {
		t.Errorf("値が要るアームの中の出どころ = %q（期待は、作れない app.check と、出どころの見つからない app.Missing）", got)
	}
}

// never を返す op は、届けば、投げる（にせものに、何も言わない）。途中で受けられて、入口まで伝わらない例外は、出どころに数えない。
// 同じ葉を、二か所から呼んでも、出どころは、一つ。
func TestThrowSourcesNeverCaughtAndShared(t *testing.T) {
	v := viewOf("app", namespace("app",
		exception("app.Boom"), exception("app.Caught"), exception("app.Bad"),
		neverOp("app.fail", "app.Boom"),
		with(op("app.leaf", nil), throwing("app.Bad", "app.Caught")),
		// inner は、Caught を受ける（計算した例外に、Caught がない）
		with(op("app.inner", nil), throwing("app.Bad"), wired(call("app.leaf"))),
		with(op("app.other", nil), throwing("app.Bad", "app.Caught"), wired(call("app.leaf"))),
		with(op("app.run", nil), throwing("app.Bad", "app.Boom"), wired(call("app.inner"), call("app.inner"), call("app.fail"))),
		with(op("app.both", nil), throwing("app.Bad", "app.Caught"), wired(call("app.inner"), call("app.other"))),
	))
	// run: 葉 leaf は、inner を通して、二度呼ばれるが、出どころは、一つ。Caught は、inner が受けるので、数えない
	wantSources(t, sourcesOf(t, v, "app.run"),
		"app.Bad app.leaf made Leaves.Leaf.Throws(app.Bad)",
		"app.Boom app.fail made")
	// both: Caught は、other を通る道でだけ、入口まで伝わる
	wantSources(t, sourcesOf(t, v, "app.both"),
		"app.Bad app.leaf made Leaves.Leaf.Throws(app.Bad)",
		"app.Caught app.leaf made Leaves.Leaf.Throws(app.Caught)")
}

// TestThrowSourcesDecoratorBody は、本体のあるデコレータ（docs/plugin-go.md「本体のあるデコレータ」）: 本物を通すので、本体を、
// 配線として辿る。本体の throw のアームは、出どころ（<op の FQN>@<デコレータの FQN>.<ケース>）。proceed() の所で、内側（op の中身）へ
// 進む。proceed() の後ろの呼び出しが投げる例外も、出どころ。
func TestThrowSourcesDecoratorBody(t *testing.T) {
	// decorator fn guarded() { match (by Gate) { Open => proceed(); Shut => throw Closed; } }（Gate は、状態の分類の述語）
	guarded := &ir.Decorator{DeclBase: base("a.guarded", "a.hozo"), Fn: true, Targets: []ir.DecoratorTarget{ir.TargetOp},
		Body: &ir.Wiring{Statements: []ir.Statement{
			matchOn("", "a.Gate",
				ir.MatchArm{Cases: []string{"Open"}, Body: ir.CallExpr{Call: ir.Call{Callee: ir.ProceedCallee{}}}},
				armThrow("a.Closed", "Shut")),
		}}}
	// decorator fn noted() { proceed(); note(); }（note は、Failed を投げうる葉）
	noted := &ir.Decorator{DeclBase: base("a.noted", "a.hozo"), Fn: true, Targets: []ir.DecoratorTarget{ir.TargetOp},
		Body: &ir.Wiring{Statements: []ir.Statement{
			ir.CallStmt{Call: ir.Call{Callee: ir.ProceedCallee{}}},
			call("a.note"),
		}}}
	v := viewOf("a", namespace("a",
		exception("a.Closed"), exception("a.Failed"), exception("a.Broken"),
		state("a.door", ir.String()),
		classify("a.Gate", ir.StatesSubject{States: []string{"a.door"}}, ir.DecidedByPredicate, "Open", "Shut"),
		guarded, noted,
		with(op("a.note", nil), internal(), throwing("a.Failed")),
		with(op("a.work", nil), internal(), throwing("a.Broken")),
		with(op("a.entry", nil), decorated("a.guarded", "a.noted"), wired(call("a.work")), throwing("a.Closed", "a.Broken", "a.Failed")),
	))
	wantSources(t, sourcesOf(t, v, "a.entry"),
		// 本体の throw のアーム: 状態の分類の述語のにせものが、Shut だけを真にする
		"a.Closed a.entry@a.guarded.Shut made Gate.Open.Returns(false) Gate.Shut.Returns(true)",
		// proceed() の先（op の中身）の葉
		"a.Broken a.work made Leaves.Work.Throws(a.Broken)",
		// proceed() の後ろで、デコレータの本体が呼ぶ葉
		"a.Failed a.note made Leaves.Note.Throws(a.Failed)",
	)
}
