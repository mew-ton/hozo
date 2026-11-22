package golang

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、入口の op から辿れる、例外の出どころ（docs/ide-chain.md「テストの規約」の「インテグレーションテストの作り直し」の
// 「Hozo が作る、例外のケース」）。入口のデコレータの相乗り（docs/plugin-go-http.md「インテグレーションテストの口」）が、出どころごとの
// ケースを作るのに使う。辿るのは、テストの中の組み立て（TestKit）が、本物で通す配線だけ。

// SourceKind は、例外の出どころの種類。
type SourceKind int

const (
	// SourceLeaf は、流れの中の、葉の op。
	SourceLeaf SourceKind = iota + 1
	// SourceEffect は、流れの端の、効果。
	SourceEffect
	// SourceExternal は、流れの端の、単位のない名前空間の op。
	SourceExternal
	// SourceSeam は、継ぎ目（op に付いた、単位の継ぎ目にあるデコレータ）。
	SourceSeam
	// SourceArm は、配線の match の、throw のアーム。
	SourceArm
	// SourceNever は、never を返す op。
	SourceNever
	// SourceUnknown は、辿っても、出どころが見つからなかった例外（読む者のないデコレータが運ぶ例外など）。
	SourceUnknown
)

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

// ThrowSource は、入口の op が投げうる例外の、出どころ一つ。
type ThrowSource struct {
	// Exception は、例外の FQN。
	Exception string
	Kind      SourceKind
	// Label は、テストの名前の <出どころ>: 葉・効果・外の op・never を返す op は、その FQN。アームは `<配線の op の FQN>.<ケース>`
	// （ケースが二つ以上なら `+` でつなぐ）。継ぎ目は `<op の FQN>@<デコレータの FQN>`。SourceUnknown は ""。
	Label string
	// Ident は、口のメソッドの名前の、出どころの部分（Go の識別子）。SourceUnknown は ""。
	Ident string
	// Made は、Hozo が、そのケースを作れるか。作れるなら、Steps が、正常のケースの用意の後に、にせものに言うこと（順に）。
	// 作れないなら、Why が、その理由（日本語の句）。
	Made  bool
	Steps []FakeStep
	Why   string
}

// ThrowSources は、kit の出力の op entry が投げうる例外（Op.signature.exceptions）の、出どころ。並びは、例外の順（signature の順）、
// その中は、辿った順（文の順。呼び出しは、中へ）。同じ出どころに、二つの道で届くなら、作れる方の、最初の道。
// 出どころが見つからなかった例外は、SourceUnknown を一つ。
func ThrowSources(p *ir.Program, kit *TestKit, entry string) []ThrowSource {
	root, _ := p.Lookup(entry).(*ir.Op)
	if root == nil || kit == nil {
		return nil
	}
	w := &sourceWalker{p: p, root: root, declNS: map[string]string{}, index: map[string]int{}, stack: map[string]bool{}}
	for i := range p.Namespaces {
		for _, d := range p.Namespaces[i].Decls {
			if d != nil {
				if _, dup := w.declNS[d.Common().Name]; !dup {
					w.declNS[d.Common().Name] = p.Namespaces[i].Name
				}
			}
		}
	}
	w.op(nil, kit, nil, entry, nil, sourceCond{})
	var out []ThrowSource
	seen := map[string]bool{}
	for _, x := range root.Signature.Exceptions {
		if seen[x] {
			continue
		}
		seen[x] = true
		n := len(out)
		for _, s := range w.found {
			if s.Exception == x {
				out = append(out, s)
			}
		}
		if len(out) == n {
			out = append(out, ThrowSource{Exception: x, Kind: SourceUnknown, Why: "出どころが、流れの中に、見つからない"})
		}
	}
	return out
}

// sourceCond は、そこまで届くための条件: にせものに言うこと（steps）か、作れない理由（why）。
type sourceCond struct {
	steps []FakeStep
	why   string
}

func (c sourceCond) with(steps []FakeStep, why string) sourceCond {
	if c.why != "" {
		return c
	}
	if why != "" {
		return sourceCond{why: why}
	}
	return sourceCond{steps: append(append([]FakeStep(nil), c.steps...), steps...)}
}

// sourceFrame は、辿っている配線の op 一つ。
type sourceFrame struct {
	kit    *TestKit
	prefix []string // 入口の Fakes から、kit の Fakes までのフィールドの名前。noFakes なら、使わない
	// noFakes は、kit の出力が fakes.go を持たない（にせものに、何も言えない）。
	noFakes bool
	op      *ir.Op
	call    *ir.Call // 呼んだ側の呼び出し（入口なら nil）
	parent  *sourceFrame
	// deco は、op に付いた、本体のあるデコレータの本体を辿っているときの、そのデコレータ（docs/plugin-go.md「本体のあるデコレータ」）。
	// そのとき、body は、その本体の文で、proceed は、proceed() の先（内側の層）を辿る関数。op の配線を辿っているときは nil。
	deco    *ir.Decorator
	body    []ir.Statement
	proceed func(c sourceCond)
}

// statements は、fr が辿っている文（op の配線の本体か、デコレータの本体）。
func (fr *sourceFrame) statements() []ir.Statement {
	if fr.deco != nil {
		return fr.body
	}
	if fr.op.Body == nil {
		return nil
	}
	return fr.op.Body.Statements
}

type sourceWalker struct {
	p      *ir.Program
	root   *ir.Op
	declNS map[string]string
	found  []ThrowSource
	index  map[string]int
	stack  map[string]bool
	// capture は、try のブロックを辿っている間の、出どころの受け皿。nil でなければ、add は、found ではなく、ここに足す
	// （try のブロックの中の出どころは、catch が受けるので、そのままでは、数えない）。
	capture *[]heldSource
	// catches は、今、中にいる catch のブロック（外から内へ）: その try のブロックの中の出どころを持つ。
	catches []caughtSources
}

// heldSource は、try のブロックの中で見つけて、取っておいた出どころ（add の引数のまま）。
type heldSource struct {
	fr  *sourceFrame
	s   ThrowSource
	c   sourceCond
	own []FakeStep
}

// caughtSources は、catch のブロック一つ: その配線（fr）の、受けた例外の名前（name）と、try のブロックの中の出どころ。
type caughtSources struct {
	fr      *sourceFrame
	name    string
	sources []heldSource
}

// rethrow は、`throw e`（受けた例外 name を、そのまま投げ直す）の出どころ: try のブロックの中の出どころを、そのまま、出どころにする
// （docs/ide-chain.md「Hozo が作る、例外のケース」の「例外を受ける文」）。keep が nil でなければ、keep が真の例外だけ。
// why が空でなければ、その理由で、作れない。
func (w *sourceWalker) rethrow(fr *sourceFrame, name string, keep func(exception string) bool, why string) {
	for i := len(w.catches) - 1; i >= 0; i-- {
		if w.catches[i].fr != fr || w.catches[i].name != name {
			continue
		}
		for _, h := range w.catches[i].sources {
			if keep != nil && !keep(h.s.Exception) {
				continue
			}
			c := h.c
			if why != "" {
				c = c.with(nil, why)
			}
			w.add(h.fr, h.s, c, h.own)
		}
		return
	}
}

// thrown は、catch のブロックの `throw 例外`（文か、受けた例外の match のアームの本体）の出どころ。
// 名前は `<配線の op の FQN>.catch`（デコレータの本体なら `<op の FQN>@<デコレータの FQN>.catch`）。
func (w *sourceWalker) thrown(fr *sourceFrame, body *ir.ThrowBody, c sourceCond) {
	label, id := fr.op.Name, MethodName(fr.op.Name)
	if fr.deco != nil {
		label, id = fr.op.Name+"@"+fr.deco.Name, SeamMethod(fr.op.Name, fr.deco.Name)
	}
	w.add(fr, ThrowSource{Exception: body.Exception, Kind: SourceArm, Label: label + ".catch", Ident: id + "Catch"}, c, nil)
}

func (w *sourceWalker) path(fr *sourceFrame, names ...string) []string {
	return append(append([]string(nil), fr.prefix...), names...)
}

// add は、出どころを足す。入口まで伝わらない例外（途中の op の signature にないもの）は、足さない。
func (w *sourceWalker) add(fr *sourceFrame, s ThrowSource, c sourceCond, own []FakeStep) {
	if w.capture != nil {
		*w.capture = append(*w.capture, heldSource{fr: fr, s: s, c: c, own: own})
		return
	}
	if !contains(w.root.Signature.Exceptions, s.Exception) {
		return
	}
	for f := fr; f != nil; f = f.parent {
		if !contains(f.op.Signature.Exceptions, s.Exception) {
			return
		}
	}
	c = c.with(own, "")
	if fr != nil && fr.noFakes && len(c.steps) > 0 && c.why == "" {
		c = sourceCond{why: "にせものを持たない出力の中にある"}
	}
	s.Made, s.Steps, s.Why = c.why == "", c.steps, c.why
	key := s.Exception + "\x00" + s.Label
	if i, dup := w.index[key]; dup {
		if !w.found[i].Made && s.Made {
			w.found[i] = s
		}
		return
	}
	w.index[key] = len(w.found)
	w.found = append(w.found, s)
}

func (w *sourceWalker) param(k *TestKit, kind KitKind, fqn string) (KitParam, bool) {
	for _, p := range k.Params {
		if p.Kind == kind && p.FQN == fqn {
			return p, true
		}
	}
	return KitParam{}, false
}

// op は、kit の出力の中で、op fqn が呼ばれたときの、出どころを足す。caller は、呼んだ側（入口なら nil）。
func (w *sourceWalker) op(caller *sourceFrame, kit *TestKit, prefix []string, fqn string, call *ir.Call, c sourceCond) {
	o, _ := w.p.Lookup(fqn).(*ir.Op)
	if o == nil {
		return
	}
	noFakes := caller != nil && caller.noFakes
	if ns := w.declNS[fqn]; ns != kit.Namespace {
		// 別の名前空間: 外の仕組みなら、そこが端。本物を通す出力なら、その中へ
		var dep KitParam
		found := false
		for _, p := range kit.Params {
			if (p.Kind == KitExternal || p.Kind == KitUnit) && p.FQN == ns {
				dep, found = p, true
			}
		}
		if !found {
			return
		}
		if dep.Kind == KitExternal {
			at := &sourceFrame{kit: kit, prefix: prefix, noFakes: noFakes, op: o, parent: caller}
			for _, x := range o.Signature.Exceptions {
				w.add(caller, ThrowSource{Exception: x, Kind: SourceExternal, Label: fqn, Ident: MethodName(fqn)}, c,
					[]FakeStep{{Path: w.path(at, dep.Field, MethodName(fqn)), Verb: StepThrows, Exception: x}})
			}
			return
		}
		kit = dep.Unit
		if dep.Field == "" {
			noFakes = true
		} else {
			prefix = append(append([]string(nil), prefix...), dep.Field)
		}
	}
	var shape *OpShape
	for i := range kit.Shape.Ops {
		if kit.Shape.Ops[i].FQN == fqn {
			shape = &kit.Shape.Ops[i]
		}
	}
	if shape == nil {
		return
	}
	fr := &sourceFrame{kit: kit, prefix: prefix, noFakes: noFakes, op: o, call: call, parent: caller}
	w.layer(fr, shape, 0, c, map[string]bool{})
}

// layer は、op の層 i（0 が、いちばん外。len(shape.Layers) が、op の中身）から内側の、出どころを足す
// （docs/plugin-go.md「継ぎ目」「本体のあるデコレータ」）。継ぎ目は、中身の前に、デコレータが投げる（にせものの継ぎ目が、投げる）。
// 本体のあるデコレータは、本物を通すので、本体を、配線として辿り、proceed() の所で、内側の層へ進む。
func (w *sourceWalker) layer(fr *sourceFrame, shape *OpShape, i int, c sourceCond, bySeam map[string]bool) {
	o, kit, fqn := fr.op, fr.kit, fr.op.Name
	if i < len(shape.Layers) {
		l := shape.Layers[i]
		dec, _ := w.p.Lookup(l.Decorator).(*ir.Decorator)
		if dec == nil {
			w.layer(fr, shape, i+1, c, bySeam)
			return
		}
		if l.Bodied {
			if dec.Body == nil {
				return
			}
			in := *fr
			in.deco, in.body = dec, dec.Body.Statements
			in.proceed = func(pc sourceCond) { w.layer(fr, shape, i+1, pc, bySeam) }
			w.stmts(&in, in.body, c)
			return
		}
		if seams, ok := w.param(kit, KitSeams, ""); ok {
			for _, clause := range dec.Throws {
				for _, x := range clause.Exceptions {
					if strings.HasPrefix(x, "$param:") {
						continue
					}
					bySeam[x] = true
					w.add(fr, ThrowSource{Exception: x, Kind: SourceSeam, Label: fqn + "@" + l.Decorator, Ident: SeamMethod(o.Name, l.Decorator)}, c,
						[]FakeStep{{Path: w.path(fr, seams.Field, SeamMethod(o.Name, l.Decorator)), Verb: StepThrows, Exception: x}})
				}
			}
		}
		w.layer(fr, shape, i+1, c, bySeam)
		return
	}
	switch shape.Place {
	case Leaf:
		leaves, ok := w.param(kit, KitLeaves, "")
		if !ok {
			return
		}
		for _, x := range leafExceptions(o, shape) {
			if bySeam[x] {
				continue
			}
			w.add(fr, ThrowSource{Exception: x, Kind: SourceLeaf, Label: fqn, Ident: MethodName(fqn)}, c,
				[]FakeStep{{Path: w.path(fr, leaves.Field, MethodName(fqn)), Verb: StepThrows, Exception: x}})
		}
	case Never:
		for _, clause := range o.Throws {
			for _, x := range clause.Exceptions {
				w.add(fr, ThrowSource{Exception: x, Kind: SourceNever, Label: fqn, Ident: MethodName(fqn)}, c, nil)
			}
		}
	case Wiring:
		if o.Body == nil || w.stack[fqn] {
			return
		}
		w.stack[fqn] = true
		w.stmts(fr, o.Body.Statements, c)
		delete(w.stack, fqn)
	}
}

// leafExceptions は、葉の op o が、自分で投げうる例外: 計算した例外（Op.signature.exceptions）のすべて。本体のあるデコレータを
// 持つ葉では、計算した例外に、デコレータの本体の分が混ざるので、節に書いた例外（葉の op は、節が、定義）。
func leafExceptions(o *ir.Op, shape *OpShape) []string {
	bodied := false
	for _, l := range shape.Layers {
		bodied = bodied || l.Bodied
	}
	if !bodied {
		return o.Signature.Exceptions
	}
	var written []string
	for _, clause := range o.Throws {
		for _, x := range clause.Exceptions {
			if !contains(written, x) {
				written = append(written, x)
			}
		}
	}
	return written
}

func (w *sourceWalker) stmts(fr *sourceFrame, list []ir.Statement, c sourceCond) {
	for _, st := range list {
		switch s := st.(type) {
		case ir.ForStmt:
			w.stmts(fr, s.Body, c.with(nil, "繰り返し（for）の中にある"))
		case ir.DisjointStmt:
			for _, arm := range s.Arms {
				w.stmts(fr, arm.Statements, c)
			}
		case ir.TryStmt:
			// try のブロックの中の出どころは、取っておく。catch のブロックの throw e が、そのまま、出どころにする
			var held []heldSource
			saved := w.capture
			w.capture = &held
			w.stmts(fr, s.Body, c)
			w.capture = saved
			w.catches = append(w.catches, caughtSources{fr: fr, name: s.Catch.Name, sources: held})
			w.stmts(fr, s.Catch.Body, c.with(nil, "例外を受けた後の道（catch のブロック）にある"))
			w.catches = w.catches[:len(w.catches)-1]
		case ir.ThrowStmt:
			if s.Throw.Caught != "" {
				w.rethrow(fr, s.Throw.Caught, nil, "")
			} else {
				w.thrown(fr, &s.Throw, c)
			}
		default:
			if e, ok := ir.StatementExpr(st); ok {
				switch x := e.(type) {
				case ir.CallExpr:
					w.call(fr, &x.Call, c)
				case ir.MatchExpr:
					w.match(fr, &x.Match, c)
				}
			}
		}
	}
}

func (w *sourceWalker) call(fr *sourceFrame, call *ir.Call, c sourceCond) {
	switch callee := call.Callee.(type) {
	case ir.ProceedCallee:
		// デコレータの本体の proceed(): 内側の層へ
		if fr.proceed != nil {
			fr.proceed(c)
		}
	case ir.OpCallee:
		w.op(fr, fr.kit, fr.prefix, callee.Op, call, c)
	case ir.EffectCallee:
		named, ok := callee.Effect.(ir.NamedEffect)
		if !ok {
			return
		}
		eff, _ := w.p.Lookup(named.Effect).(*ir.Effect)
		unit, ok := w.unitOf(fr.kit, named.Effect)
		if eff == nil || !ok {
			return
		}
		for _, x := range eff.Throws {
			w.add(fr, ThrowSource{Exception: x, Kind: SourceEffect, Label: named.Effect, Ident: MethodName(named.Effect)}, c,
				[]FakeStep{{Path: w.path(fr, unit.Field, MethodName(named.Effect)), Verb: StepThrows, Exception: x}})
		}
	}
}

// unitOf は、kit の出力の、read か効果 fqn を、メソッドに持つ状態の単位の Param。
func (w *sourceWalker) unitOf(kit *TestKit, fqn string) (KitParam, bool) {
	for _, u := range kit.Shape.States {
		if contains(u.Reads, fqn) || contains(u.Effects, fqn) {
			return w.param(kit, KitState, u.FQN)
		}
	}
	return KitParam{}, false
}

func (w *sourceWalker) match(fr *sourceFrame, m *ir.Match, c sourceCond) {
	if cs, caught := m.Subject.(ir.CaughtSubject); caught {
		// 受けた例外の match: アームの throw e は、そのアームが受けるケースの例外だけを、投げ直す
		cl, _ := w.p.Lookup(m.Classify).(*ir.Classify)
		for i := range m.Arms {
			arm := &m.Arms[i]
			switch body := arm.Body.(type) {
			case ir.ThrowBody:
				switch {
				case body.Caught == cs.Name && cl != nil:
					w.rethrow(fr, body.Caught, func(x string) bool {
						name, ok := cl.CaseOfException(x)
						return ok && contains(arm.Cases, name)
					}, "")
				case body.Caught != "":
					w.rethrow(fr, body.Caught, nil, "")
				default:
					w.thrown(fr, &body, c)
				}
			case ir.CallExpr:
				w.call(fr, &body.Call, c)
			}
		}
		return
	}
	nonThrow := 0
	for _, arm := range m.Arms {
		if _, throws := arm.Body.(ir.ThrowBody); !throws {
			nonThrow++
		}
	}
	for i := range m.Arms {
		arm := &m.Arms[i]
		switch body := arm.Body.(type) {
		case ir.ThrowBody:
			if body.Caught != "" {
				// catch のブロックの中の、ほかの match のアームの throw e: そのアームに入る用意は、作らない
				w.rethrow(fr, body.Caught, nil, "例外を受けた後の道の、match のアームにある")
				continue
			}
			steps, why := w.enter(fr, m, arm)
			var ident string
			for _, k := range arm.Cases {
				ident += Capitalize(k)
			}
			label, id := fr.op.Name, MethodName(fr.op.Name)
			if fr.deco != nil {
				// デコレータの本体のアーム: <op の FQN>@<デコレータの FQN>.<ケース>
				label, id = fr.op.Name+"@"+fr.deco.Name, SeamMethod(fr.op.Name, fr.deco.Name)
			}
			w.add(fr, ThrowSource{Exception: body.Exception, Kind: SourceArm, Label: label + "." + strings.Join(arm.Cases, "+"),
				Ident: id + ident}, c.with(steps, why), nil)
		case ir.CallExpr:
			cc := c
			if nonThrow > 1 {
				// 正常のケースが、このアームを通るとは、限らない: にせもので、入らせる
				cc = c.with(w.enter(fr, m, arm))
			}
			w.call(fr, &body.Call, cc)
		}
	}
}

// enter は、match m のアーム arm に、にせもので、入らせる方法。入らせられなければ、理由。
func (w *sourceWalker) enter(fr *sourceFrame, m *ir.Match, arm *ir.MatchArm) ([]FakeStep, string) {
	if len(arm.Cases) == 0 {
		return nil, "アームが、ケースを受けない"
	}
	cl, _ := w.p.Lookup(m.Classify).(*ir.Classify)
	none := ""
	switch {
	case m.Builtin():
		none = ir.StandardCases(m.Classify)[0]
	case cl == nil:
		return nil, "分類 " + m.Classify + " が、見つからない"
	default:
		none = cl.Absent
	}
	if len(arm.Cases) == 1 && arm.Cases[0] == none && none != "" {
		// 値がない（か、空）ことで、入るアーム: 値の出どころを辿る
		if _, owned := m.Subject.(ir.ClassifyOwnedSubject); !owned {
			return w.none(fr, m.SubjectAtom(), 0)
		}
	}
	if m.Builtin() || cl.DecidedBy != ir.DecidedByPredicate {
		return nil, "値で決まり、入るのに、値が要るアーム（" + strings.Join(arm.Cases, "、") + "）"
	}
	want := ""
	for _, k := range arm.Cases {
		if k != cl.Absent {
			want = k
			break
		}
	}
	if want == "" {
		return nil, "値がないことで決まるアーム"
	}
	if _, isState := cl.SubjectStates(); isState {
		// 述語で決まる状態の分類: 状態の単位の述語が、そのケースだけ、真を返す
		unit, ok := w.param(fr.kit, KitState, m.Classify)
		if !ok {
			return nil, "分類 " + m.Classify + " の単位が、この出力にない"
		}
		var steps []FakeStep
		for _, k := range cl.Cases {
			if k.Name == cl.Absent {
				continue
			}
			steps = append(steps, FakeStep{Path: w.path(fr, unit.Field, Capitalize(k.Name)), Verb: StepReturnsBool, Bool: k.Name == want})
		}
		return steps, ""
	}
	pred, ok := w.param(fr.kit, KitClassify, m.Classify)
	if !ok {
		return nil, "分類 " + m.Classify + " の述語が、この出力にない"
	}
	return []FakeStep{{Path: w.path(fr, pred.Field), Verb: StepIs, Classify: m.Classify, Case: want}}, ""
}

// noneable は、型 t の値に、「ない」（nil）を返せるか: 省略可能な型か、並び。
func noneable(t ir.Type) bool {
	switch t.(type) {
	case ir.OptionalType, ir.ArrayType:
		return true
	}
	return false
}

// none は、fr の配線の中の atom の値を、「ない」にする方法: 値の出どころを辿り、にせものに、「ない」を返させる。
func (w *sourceWalker) none(fr *sourceFrame, atom ir.Atom, depth int) ([]FakeStep, string) {
	if depth > 16 {
		return nil, "値の出どころを、辿りきれない"
	}
	switch a := atom.(type) {
	case ir.StateAtom:
		unit, ok := w.param(fr.kit, KitState, a.State)
		if !ok || len(a.Path) > 0 {
			return nil, "状態 " + a.State + " の値の一部で決まる"
		}
		return []FakeStep{{Path: w.path(fr, unit.Field, "Get"), Verb: StepReturnsNone}}, ""
	case ir.NameAtom:
		if len(a.Path) != 1 {
			return nil, "値の一部（" + strings.Join(a.Path, ".") + "）で決まる"
		}
		name := a.Path[0]
		if fr.deco != nil {
			for _, p := range fr.deco.Params {
				if p.Name == name {
					return nil, "デコレータの引数（" + name + "）で決まる"
				}
			}
		}
		for _, p := range fr.op.Params {
			if p.Name != name || fr.deco != nil {
				continue
			}
			if fr.parent == nil || fr.call == nil {
				return nil, "入口の入力（" + name + "）で決まる"
			}
			for _, arg := range fr.call.Args {
				if arg.Param == name {
					return w.none(fr.parent, arg.Value, depth+1)
				}
			}
			return nil, "引数 " + name + " の値を、辿れない"
		}
		// 束縛を探す: 値と一緒に宣言した val が、一つだけ
		var bound ir.Expr
		n := 0
		ir.WalkStatements(fr.statements(), func(st ir.Statement) {
			switch s := st.(type) {
			case ir.ValStmt:
				if s.Name == name {
					n++
					bound = s.Expr
				}
			case ir.AssignStmt:
				if s.Name == name {
					n++
				}
			case ir.CollectStmt:
				if s.Name == name {
					n++
				}
			}
		})
		if n != 1 || bound == nil {
			return nil, "値 " + name + " の出どころが、一つに決まらない"
		}
		return w.noneExpr(fr, bound, depth)
	}
	return nil, "値の出どころを、辿れない"
}

// noneExpr は、fr の配線の中の式 e の値を、「ない」にする方法。
func (w *sourceWalker) noneExpr(fr *sourceFrame, e ir.Expr, depth int) ([]FakeStep, string) {
	switch x := e.(type) {
	case ir.AtomExpr:
		return w.none(fr, x.Atom, depth+1)
	case ir.CallExpr:
		switch callee := x.Call.Callee.(type) {
		case ir.ReadCallee:
			r, _ := w.p.Lookup(callee.Read).(*ir.Read)
			unit, ok := w.unitOf(fr.kit, callee.Read)
			if r == nil || !ok || !noneable(r.Returns) {
				return nil, "read " + callee.Read + " が、「ない」を返せない"
			}
			return []FakeStep{{Path: w.path(fr, unit.Field, MethodName(callee.Read)), Verb: StepReturnsNone}}, ""
		case ir.OpCallee:
			return w.noneOp(fr, callee.Op, &x.Call, depth)
		}
	}
	return nil, "値の出どころを、辿れない"
}

// noneOp は、fr の配線が呼ぶ op fqn の戻り値を、「ない」にする方法。
func (w *sourceWalker) noneOp(fr *sourceFrame, fqn string, call *ir.Call, depth int) ([]FakeStep, string) {
	o, _ := w.p.Lookup(fqn).(*ir.Op)
	if o == nil || !noneable(o.Returns) {
		return nil, "op " + fqn + " が、「ない」を返せない"
	}
	kit, prefix, noFakes := fr.kit, fr.prefix, fr.noFakes
	if ns := w.declNS[fqn]; ns != kit.Namespace {
		var dep KitParam
		found := false
		for _, p := range kit.Params {
			if (p.Kind == KitExternal || p.Kind == KitUnit) && p.FQN == ns {
				dep, found = p, true
			}
		}
		if !found {
			return nil, "op " + fqn + " の名前空間が、この出力の口にない"
		}
		if dep.Kind == KitExternal {
			return []FakeStep{{Path: w.path(fr, dep.Field, MethodName(fqn)), Verb: StepReturnsNone}}, ""
		}
		kit = dep.Unit
		if dep.Field == "" {
			noFakes = true
		} else {
			prefix = append(append([]string(nil), prefix...), dep.Field)
		}
	}
	var shape *OpShape
	for i := range kit.Shape.Ops {
		if kit.Shape.Ops[i].FQN == fqn {
			shape = &kit.Shape.Ops[i]
		}
	}
	if shape == nil {
		return nil, "op " + fqn + " が、通す出力にない"
	}
	in := &sourceFrame{kit: kit, prefix: prefix, noFakes: noFakes, op: o, call: call, parent: fr}
	switch shape.Place {
	case Leaf:
		leaves, ok := w.param(kit, KitLeaves, "")
		if !ok {
			return nil, "op " + fqn + " の葉が、通す出力にない"
		}
		return []FakeStep{{Path: w.path(in, leaves.Field, MethodName(fqn)), Verb: StepReturnsNone}}, ""
	case Wiring:
		// 配線の op: return の式を、辿る（return が、一つだけのとき）
		var ret ir.Expr
		n := 0
		if o.Body != nil {
			ir.WalkStatements(o.Body.Statements, func(st ir.Statement) {
				if s, ok := st.(ir.ReturnStmt); ok {
					n++
					ret = s.Expr
				}
			})
		}
		if n != 1 || ret == nil {
			return nil, "op " + fqn + " の戻り値の出どころが、一つに決まらない"
		}
		return w.noneExpr(in, ret, depth+1)
	}
	return nil, "op " + fqn + " の戻り値を、辿れない"
}
