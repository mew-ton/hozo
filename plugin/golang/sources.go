package golang

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、入口ごとの、求めるケース（docs/check.md「入口ごとの、求めるケース」）を、Go のにせものに、当てる
// （plugins/go/spec/go.hozo の Requirement.Fakes の「求めるケースを、にせものに当てる」）。出どころと、そこへ入らせる方法を求めるのは、
// 本体で（View.entries）、ここは、差し替えに言うこと（ir.EntryStep）を、テストの中の組み立て（TestKit）の、にせもののフィールドに、
// 直すだけ。入口のデコレータの相乗り（docs/plugin-go-http.md「インテグレーションテストの口」）が、ケースを作るのに使う。

// StepVerb は、にせものに言うことの種類。
type StepVerb int

const (
	// StepThrows は、`<にせもの>.Throws(<例外>{})`。
	StepThrows StepVerb = iota + 1
	// StepReturnsNone は、`<にせもの>.Returns(nil)`: 値がない（か、空の並び）を返す。
	StepReturnsNone
	// StepReturnsBool は、`<にせもの>.Returns(<真偽>)`: 状態の分類の述語。
	StepReturnsBool
	// StepIs は、`<にせもの>.Is(<ケースの定数>)`: 値の分類・op の分類の述語。
	StepIs
)

// FakeStep は、正常のケースの用意の後に、にせものに言うこと一つ。
type FakeStep struct {
	// Path は、入口の出力の Fakes から、にせもの（のフィールド）までの、フィールドの名前（`Store`、`Sessions`、`SessionByToken`）。
	Path []string
	Verb StepVerb
	// Exception は、StepThrows の、例外の FQN。Bool は、StepReturnsBool の値。Classify と Case は、StepIs の、分類の FQN とケース名。
	Exception      string
	Bool           bool
	Classify, Case string
}

// ThrowSource は、入口の op の、出どころ一つ（か、受ける道一つ）を、Go のにせものに、当てたもの。
type ThrowSource struct {
	// Exception は、例外の FQN（受ける道なら、受けられる例外）。
	Exception string
	Kind      ir.EntrySourceKind
	// Label は、テストの名前の <出どころ>（ir.EntrySource.Label）。unknown は ""。
	Label string
	// Ident は、口のメソッドの名前の、出どころの部分（Go の識別子）。unknown は ""。
	Ident string
	// Made は、Hozo が、そのケースを作れるか: 差し替えで入れて、その差し替えを、どれも、Go のにせものに、言える。作れるなら、
	// Steps が、正常のケースの用意の後に、にせものに言うこと（順に）。作れないなら、Why が、その理由（日本語の句）。
	Made  bool
	Steps []FakeStep
	Why   string
}

// ThrowSources は、view の出力の入口 entry が投げうる例外の、出どころ（View.entries の、その入口の sources の順）。
// 入口が、View.entries になければ nil。
func ThrowSources(view *ir.View, kit *TestKit, entry string) []ThrowSource {
	e := entryOf(view, kit, entry)
	if e == nil {
		return nil
	}
	return mapSources(view.Program, kit, e.Sources)
}

// CaughtSources は、view の出力の入口 entry の、受ける道（View.entries の、その入口の caught の順）。
func CaughtSources(view *ir.View, kit *TestKit, entry string) []ThrowSource {
	e := entryOf(view, kit, entry)
	if e == nil {
		return nil
	}
	return mapSources(view.Program, kit, e.Caught)
}

func entryOf(view *ir.View, kit *TestKit, entry string) *ir.Entry {
	if view == nil || view.Program == nil || kit == nil {
		return nil
	}
	return view.Entry(entry)
}

func mapSources(p *ir.Program, kit *TestKit, list []ir.EntrySource) []ThrowSource {
	out := make([]ThrowSource, 0, len(list))
	for _, s := range list {
		t := ThrowSource{Exception: s.Exception, Kind: s.Kind, Label: s.Label, Ident: sourceIdent(s), Why: s.Why}
		if s.Why == "" {
			t.Steps, t.Why = fakeSteps(p, kit, s.Steps)
			t.Made = t.Why == ""
			if !t.Made {
				t.Steps = nil
			}
		}
		out = append(out, t)
	}
	return out
}

// sourceIdent は、出どころ s の、口のメソッドの名前の部分: 葉・効果・流れの端の op・never を返す op は、その名前。境界と、本体のない
// デコレータは、op の名前に、デコレータの名前を続けたもの。アームは、配線の op の名前（デコレータの本体の中なら、デコレータの名前を
// 続けたもの）に、catch の中なら Catch、その後ろに、ケースの名前。
func sourceIdent(s ir.EntrySource) string {
	switch s.Kind {
	case ir.SourceUnknown:
		return ""
	case ir.SourceBoundary, ir.SourceDecorator:
		return SeamMethod(s.Name, s.Decorator)
	case ir.SourceArm:
		id := MethodName(s.Name)
		if s.Decorator != "" {
			id = SeamMethod(s.Name, s.Decorator)
		}
		if s.Catch {
			id += "Catch"
		}
		for _, k := range s.Cases {
			id += Capitalize(k)
		}
		return id
	}
	return MethodName(s.Name)
}

// fakeSteps は、差し替えに言うこと steps を、kit の出力の Fakes の、にせものに言うことに、直す。言えないものが、一つでもあれば、理由。
func fakeSteps(p *ir.Program, kit *TestKit, steps []ir.EntryStep) ([]FakeStep, string) {
	var out []FakeStep
	for _, st := range steps {
		k, prefix, why := kitAt(kit, st.Via)
		if why != "" {
			return nil, why
		}
		at := func(names ...string) []string { return append(append([]string(nil), prefix...), names...) }
		verb := StepThrows
		if st.Verb == ir.StepNone {
			verb = StepReturnsNone
		}
		switch st.Kind {
		case ir.StepLeaf:
			leaves, ok := kitParam(k, KitLeaves, "")
			if !ok {
				return nil, "op " + st.Name + " の葉が、通す出力にない"
			}
			out = append(out, FakeStep{Path: at(leaves.Field, MethodName(st.Name)), Verb: verb, Exception: st.Exception})
		case ir.StepEffect, ir.StepRead:
			unit, ok := kitUnitOf(k, st.Name)
			if !ok {
				return nil, st.Name + " を持つ状態の単位が、通す出力にない"
			}
			out = append(out, FakeStep{Path: at(unit.Field, MethodName(st.Name)), Verb: verb, Exception: st.Exception})
		case ir.StepState:
			unit, ok := kitParam(k, KitState, st.Name)
			if !ok {
				return nil, "状態 " + st.Name + " の単位が、通す出力にない"
			}
			out = append(out, FakeStep{Path: at(unit.Field, "Get"), Verb: verb})
		case ir.StepOp:
			ns := st.Name
			if i := strings.LastIndex(ns, "."); i >= 0 {
				ns = ns[:i]
			}
			dep, ok := kitParam(k, KitExternal, ns)
			if !ok {
				return nil, "名前空間 " + ns + " を、本物で通しているので、op " + st.Name + " を、にせものにできない"
			}
			out = append(out, FakeStep{Path: at(dep.Field, MethodName(st.Name)), Verb: verb, Exception: st.Exception})
		case ir.StepDecorator:
			seams, ok := kitParam(k, KitSeams, "")
			if !ok || !hasSeam(k, st.Name, st.Decorator) {
				return nil, "デコレータ " + st.Decorator + " の継ぎ目が、この単位にない"
			}
			out = append(out, FakeStep{Path: at(seams.Field, SeamMethod(st.Name, st.Decorator)), Verb: verb, Exception: st.Exception})
		case ir.StepClassify:
			cl, _ := p.Lookup(st.Name).(*ir.Classify)
			if cl == nil {
				return nil, "分類 " + st.Name + " が、見つからない"
			}
			if _, isState := cl.SubjectStates(); isState {
				// 述語で決まる状態の分類: 状態の単位の述語が、そのケースだけ、真を返す
				unit, ok := kitParam(k, KitState, st.Name)
				if !ok {
					return nil, "分類 " + st.Name + " の単位が、この出力にない"
				}
				for _, c := range cl.Cases {
					if c.Name == cl.Absent {
						continue
					}
					out = append(out, FakeStep{Path: at(unit.Field, Capitalize(c.Name)), Verb: StepReturnsBool, Bool: c.Name == st.Case})
				}
				continue
			}
			pred, ok := kitParam(k, KitClassify, st.Name)
			if !ok {
				return nil, "分類 " + st.Name + " の述語が、この出力にない"
			}
			out = append(out, FakeStep{Path: at(pred.Field), Verb: StepIs, Classify: st.Name, Case: st.Case})
		default:
			return nil, "差し替える相手の種類 " + string(st.Kind) + " を、知らない"
		}
	}
	return out, ""
}

// kitAt は、kit の出力から、名前空間 via を、順に通った先の、出力と、そこまでの、Fakes のフィールドの名前。
func kitAt(kit *TestKit, via []string) (k *TestKit, prefix []string, why string) {
	k = kit
	for _, ns := range via {
		var dep KitParam
		found := false
		for _, p := range k.Params {
			if p.Kind == KitUnit && p.FQN == ns {
				dep, found = p, true
			}
		}
		if !found {
			return nil, nil, "名前空間 " + ns + " を、本物で通していない"
		}
		if dep.Field == "" {
			return nil, nil, "にせものを持たない出力の中にある"
		}
		prefix = append(prefix, dep.Field)
		k = dep.Unit
	}
	return k, prefix, ""
}

func kitParam(k *TestKit, kind KitKind, fqn string) (KitParam, bool) {
	for _, p := range k.Params {
		if p.Kind == kind && p.FQN == fqn {
			return p, true
		}
	}
	return KitParam{}, false
}

// kitUnitOf は、kit の出力の、read か効果 fqn を、メソッドに持つ状態の単位の Param。
func kitUnitOf(kit *TestKit, fqn string) (KitParam, bool) {
	for _, u := range kit.Shape.States {
		if contains(u.Reads, fqn) || contains(u.Effects, fqn) {
			return kitParam(kit, KitState, u.FQN)
		}
	}
	return KitParam{}, false
}

// hasSeam は、kit の出力の op が、デコレータ decorator の継ぎ目を持つか。
func hasSeam(kit *TestKit, op, decorator string) bool {
	for i := range kit.Shape.Ops {
		if kit.Shape.Ops[i].FQN != op {
			continue
		}
		for _, l := range kit.Shape.Ops[i].Layers {
			if l.Decorator == decorator && !l.Bodied {
				return true
			}
		}
	}
	return false
}
