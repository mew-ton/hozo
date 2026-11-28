package golang

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、ThrowSources と CaughtSources（plugins/go/spec/go.hozo の Requirement.Fakes の「求めるケースを、にせものに当てる」）:
// 本体が求めたケース（View.entries）を、Go のにせものに当てる。出どころを求めること（docs/check.md「入口ごとの、求めるケース」）は、
// 本体のテスト（internal/check/entries_test.go）が確かめるので、ここでは、IR も、View.entries も、手で組む。

// mapView は、app（単位 1）が、store（単位 2。葉 pick を持つ）と、mailer（単位がない）と、api（単位 3。呼ぶ op は、入口）の op を
// 呼ぶ仕様の、app の View。app.run は、継ぎ目のデコレータ app.limited を持つ。View.entries は、テストが足す。
func mapView() *ir.View {
	add := effect("app.addItem", ir.BuiltinTarget{State: "app.items", Operation: ir.OpPost}, param("v", ir.String()))
	add.Throws = []string{"app.Taken"}
	served := &ir.Decorator{DeclBase: base("api.served", "api.hozo"), Fn: true, Entry: true, Targets: []ir.DecoratorTarget{ir.TargetOp},
		OuterThrows: []ir.ThrowsClause{{Exceptions: []string{"api.Down"}}}}
	limited := &ir.Decorator{DeclBase: base("app.limited", "app.hozo"), Fn: true, Targets: []ir.DecoratorTarget{ir.TargetOp},
		Throws: []ir.ThrowsClause{{Exceptions: []string{"app.Limited"}}}}
	v := viewOf("app",
		namespace("app",
			exception("app.Rejected"), exception("app.Taken"), exception("app.Limited"),
			limited,
			state("app.items", ir.Array(ir.String())),
			add,
			read("app.first", "app.items", ir.Optional(ir.String())),
			classify("app.Size", ir.TypeSubject{Type: ir.String()}, ir.DecidedByPredicate, "Big", "Small"),
			classify("app.Sync", ir.StatesSubject{States: []string{"app.items"}}, ir.DecidedByPredicate, "UpToDate", "OutOfDate"),
			with(op("app.leaf", nil), throwing("app.Rejected")),
			with(op("app.run", nil, param("v", ir.String())), decorated("app.limited"), wired(
				ir.ValStmt{Name: "x", Expr: ir.CallExpr{Call: ir.Call{Callee: ir.ReadCallee{Read: "app.first"}}}},
				ir.MatchStmt{Match: ir.Match{Subject: ir.ValueSubject{Atom: ir.NameAtom{Path: []string{"v"}}}, Classify: "app.Size", Arms: []ir.MatchArm{
					{Cases: []string{"Small"}, Body: ir.CallExpr{Call: ir.Call{Callee: ir.EffectCallee{Effect: ir.NamedEffect{Effect: "app.addItem"}}}}},
					{Cases: []string{"Big"}, Body: ir.ThrowBody{Exception: "app.Rejected"}},
				}}},
				matchBy("app.Sync", armCall("app.leaf", "UpToDate"), ir.MatchArm{Cases: []string{"OutOfDate"}, Body: ir.ThrowBody{Exception: "app.Rejected"}}),
				call("store.find"), call("mailer.send"), call("api.submit"),
			)),
		),
		namespace("store",
			exception("store.Missing"),
			with(op("store.pick", nil), throwing("store.Missing")),
			with(op("store.find", nil), wired(call("store.pick"))),
		),
		namespace("mailer", exception("mailer.Undelivered"), with(op("mailer.send", nil), throwing("mailer.Undelivered"))),
		namespace("api", exception("api.Down"), exception("api.Refused"), served,
			with(op("api.submit", nil), decorated("api.served"), throwing("api.Refused"))),
	)
	v.Config.Seams["app.limited"] = "go-ratelimit"
	v.Namespaces["store"] = []ir.NamespaceConfig{conf(2, "gen/store")}
	v.Namespaces["api"] = []ir.NamespaceConfig{conf(3, "gen/api")}
	return v
}

// mapped は、出どころの並びを、一行ずつの文字列にする: `<例外> <出どころ> <口の名前の部分> made|human <にせものに言うこと か 理由>`。
func mapped(list []ThrowSource) []string {
	var out []string
	for _, s := range list {
		line := s.Exception + " " + s.Label + " " + s.Ident + " human " + s.Why
		if s.Made {
			line = s.Exception + " " + s.Label + " " + s.Ident + " made"
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

func kitOf(t *testing.T, v *ir.View) *TestKit {
	t.Helper()
	kit, err := TestKitOf(v)
	if err != nil || kit.Blocked != "" {
		t.Fatalf("TestKitOf: %v, Blocked = %q", err, kit.Blocked)
	}
	return kit
}

func throwsStep(kind ir.EntryTargetKind, name, exception string, via ...string) ir.EntryStep {
	return ir.EntryStep{Via: via, Verb: ir.StepThrows, Kind: kind, Name: name, Exception: exception}
}

// 差し替えに言うこと（ir.EntryStep）の、相手の種類ごとに、Fakes の、どのフィールドに、何を言うか（plugins/go/spec/go.hozo の Requirement.Fakes の
// 「求めるケースを、にせものに当てる」の表）。
func TestThrowSourcesSteps(t *testing.T) {
	v := mapView()
	seam := throwsStep(ir.StepDecorator, "app.run", "app.Limited")
	seam.Decorator = "app.limited"
	v.Entries = []ir.Entry{{Op: "app.run", Sources: []ir.EntrySource{
		// 葉: 葉のにせものが、投げる
		{Exception: "app.Rejected", Kind: ir.SourceLeaf, Label: "app.leaf", Name: "app.leaf", Steps: []ir.EntryStep{throwsStep(ir.StepLeaf, "app.leaf", "app.Rejected")}},
		// 効果: それを持つ状態の単位のにせものが、投げる
		{Exception: "app.Taken", Kind: ir.SourceEffect, Label: "app.addItem", Name: "app.addItem", Steps: []ir.EntryStep{throwsStep(ir.StepEffect, "app.addItem", "app.Taken")}},
		// 値の分類の述語が、ケースを言う。その前に、read が「ない」を返す
		{Exception: "app.Rejected", Kind: ir.SourceArm, Label: "app.run.Big", Name: "app.run", Cases: []string{"Big"}, Steps: []ir.EntryStep{
			{Verb: ir.StepNone, Kind: ir.StepRead, Name: "app.first"},
			{Verb: ir.StepIs, Kind: ir.StepClassify, Name: "app.Size", Case: "Big"},
		}},
		// 状態の分類の述語: そのケースだけ、真
		{Exception: "app.Rejected", Kind: ir.SourceArm, Label: "app.run.OutOfDate", Name: "app.run", Cases: []string{"OutOfDate"}, Steps: []ir.EntryStep{
			{Verb: ir.StepIs, Kind: ir.StepClassify, Name: "app.Sync", Case: "OutOfDate"},
		}},
		// 状態を、じかに読む値が、ない
		{Exception: "app.Rejected", Kind: ir.SourceArm, Label: "app.run.Absent", Name: "app.run", Cases: []string{"Absent"}, Steps: []ir.EntryStep{
			{Verb: ir.StepNone, Kind: ir.StepState, Name: "app.items"},
		}},
		// 本体のないデコレータ: 継ぎ目のにせものが、投げる
		{Exception: "app.Limited", Kind: ir.SourceDecorator, Label: "app.run@app.limited", Name: "app.run", Decorator: "app.limited", Steps: []ir.EntryStep{seam}},
		// 通す出力（store）の中の葉: Fakes のフィールド Store の先
		{Exception: "store.Missing", Kind: ir.SourceLeaf, Label: "store.pick", Name: "store.pick", Steps: []ir.EntryStep{throwsStep(ir.StepLeaf, "store.pick", "store.Missing", "store")}},
		// 流れの端の op（単位のない名前空間）
		{Exception: "mailer.Undelivered", Kind: ir.SourceExternal, Label: "mailer.send", Name: "mailer.send", Steps: []ir.EntryStep{throwsStep(ir.StepOp, "mailer.send", "mailer.Undelivered")}},
		// 流れの端の op（相手の入口）と、その境界
		{Exception: "api.Refused", Kind: ir.SourceExternal, Label: "api.submit", Name: "api.submit", Steps: []ir.EntryStep{throwsStep(ir.StepOp, "api.submit", "api.Refused")}},
		{Exception: "api.Down", Kind: ir.SourceBoundary, Label: "api.submit@api.served", Name: "api.submit", Decorator: "api.served", Steps: []ir.EntryStep{throwsStep(ir.StepOp, "api.submit", "api.Down")}},
		// never を返す op: 届けば、投げる（言うことが、ない）
		{Exception: "app.Rejected", Kind: ir.SourceNever, Label: "app.fail", Name: "app.fail"},
	}}}
	wantSources(t, mapped(ThrowSources(v, kitOf(t, v), "app.run")),
		"app.Rejected app.leaf Leaf made Leaves.Leaf.Throws(app.Rejected)",
		"app.Taken app.addItem AddItem made Items.AddItem.Throws(app.Taken)",
		"app.Rejected app.run.Big RunBig made Items.First.Returns(nil) Size.Is(app.Size.Big)",
		"app.Rejected app.run.OutOfDate RunOutOfDate made Sync.UpToDate.Returns(false) Sync.OutOfDate.Returns(true)",
		"app.Rejected app.run.Absent RunAbsent made Items.Get.Returns(nil)",
		"app.Limited app.run@app.limited RunAppLimited made Seams.RunAppLimited.Throws(app.Limited)",
		"store.Missing store.pick Pick made Store.Leaves.Pick.Throws(store.Missing)",
		"mailer.Undelivered mailer.send Send made Mailer.Send.Throws(mailer.Undelivered)",
		"api.Refused api.submit Submit made Api.Submit.Throws(api.Refused)",
		"api.Down api.submit@api.served SubmitApiServed made Api.Submit.Throws(api.Down)",
		"app.Rejected app.fail Fail made",
	)
	// View.entries にない入口は、nil
	if got := ThrowSources(v, kitOf(t, v), "app.leaf"); got != nil {
		t.Errorf("View.entries にない入口: %+v", got)
	}
}

// 本体が「入れない」と言った出どころは、その理由のまま、作れない。本体が「入れる」と言っても、Go のにせものに言えなければ、作れない
// （理由は、ここが言う）。
func TestThrowSourcesNotMade(t *testing.T) {
	v := mapView()
	// 継ぎ目を、単位の設定から外す: デコレータ app.limited の継ぎ目が、ない
	delete(v.Config.Seams, "app.limited")
	seam := throwsStep(ir.StepDecorator, "app.run", "app.Limited")
	seam.Decorator = "app.limited"
	v.Entries = []ir.Entry{{Op: "app.run", Sources: []ir.EntrySource{
		{Exception: "app.Rejected", Kind: ir.SourceLeaf, Label: "app.leaf", Name: "app.leaf", Why: "決め手が、入口の入力"},
		{Exception: "app.Limited", Kind: ir.SourceDecorator, Label: "app.run@app.limited", Name: "app.run", Decorator: "app.limited", Steps: []ir.EntryStep{seam}},
		// 本物で通している名前空間（store）の op は、にせものにできない
		{Exception: "store.Missing", Kind: ir.SourceExternal, Label: "store.find", Name: "store.find", Steps: []ir.EntryStep{throwsStep(ir.StepOp, "store.find", "store.Missing")}},
		// 通していない名前空間の中
		{Exception: "api.Refused", Kind: ir.SourceLeaf, Label: "api.inner", Name: "api.inner", Steps: []ir.EntryStep{throwsStep(ir.StepLeaf, "api.inner", "api.Refused", "api")}},
		{Exception: "app.Taken", Kind: ir.SourceUnknown, Why: "出どころが、見つからない"},
	}}}
	wantSources(t, mapped(ThrowSources(v, kitOf(t, v), "app.run")),
		"app.Rejected app.leaf Leaf human 決め手が、入口の入力",
		"app.Limited app.run@app.limited RunAppLimited human デコレータ app.limited の継ぎ目が、この単位にない",
		"store.Missing store.find Find human 名前空間 store を、本物で通しているので、op store.find を、にせものにできない",
		"api.Refused api.inner Inner human 名前空間 api を、本物で通していない",
		"app.Taken   human 出どころが、見つからない",
	)
}

// アームの出どころの、口のメソッドの名前の部分: 配線の op の名前（デコレータの本体の中なら、デコレータの名前を続ける）に、catch の中なら
// Catch、その後ろに、ケースの名前。受ける道（View.entries の caught）も、同じ当て方。
func TestSourceIdentAndCaught(t *testing.T) {
	v := mapView()
	v.Entries = []ir.Entry{{Op: "app.run",
		Sources: []ir.EntrySource{
			{Exception: "app.Rejected", Kind: ir.SourceArm, Label: "app.run@app.guarded.Shut", Name: "app.run", Decorator: "app.guarded", Cases: []string{"Shut"}},
			{Exception: "app.Rejected", Kind: ir.SourceArm, Label: "app.run.catch", Name: "app.run", Catch: true},
			{Exception: "app.Rejected", Kind: ir.SourceArm, Label: "app.run.catch.Lost.Late", Name: "app.run", Catch: true, Cases: []string{"Lost", "Late"}},
		},
		Caught: []ir.EntrySource{
			{Exception: "mailer.Undelivered", Kind: ir.SourceExternal, Label: "mailer.send", Name: "mailer.send", Steps: []ir.EntryStep{throwsStep(ir.StepOp, "mailer.send", "mailer.Undelivered")}},
			{Exception: "store.Missing", Kind: ir.SourceLeaf, Label: "store.pick", Name: "store.pick", Why: "同じ出どころが、受ける文の外でも、呼ばれる"},
		},
	}}
	kit := kitOf(t, v)
	wantSources(t, mapped(ThrowSources(v, kit, "app.run")),
		"app.Rejected app.run@app.guarded.Shut RunAppGuardedShut made",
		"app.Rejected app.run.catch RunCatch made",
		"app.Rejected app.run.catch.Lost.Late RunCatchLostLate made",
	)
	wantSources(t, mapped(CaughtSources(v, kit, "app.run")),
		"mailer.Undelivered mailer.send Send made Mailer.Send.Throws(mailer.Undelivered)",
		"store.Missing store.pick Pick human 同じ出どころが、受ける文の外でも、呼ばれる",
	)
}
