package check

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、入口ごとの、求めるケース（docs/check.md「入口ごとの、求めるケース」）: 入口の op から、流れを辿って、
// 例外の出どころと、そこへ、差し替えで入らせる方法と、受ける道を、求める。言語には、依らない: 差し替えを、その言語で、
// できるかは、プラグインが決める。見るのは、View が持つもの（program、config、files、namespaces）だけ。

// viewEntries は、View v の、単位の対象の op のうち、入口（@entry の付いたデコレータを持つ op）ごとの、求めるケース（op の宣言順）。
func viewEntries(v *ir.View) []ir.Entry {
	if v == nil || v.Program == nil {
		return nil
	}
	ns := v.Program.Namespace(v.Viewer)
	if ns == nil {
		return nil
	}
	in := fileSet(v.Files)
	var out []ir.Entry
	for _, d := range ns.Decls {
		o, ok := d.(*ir.Op)
		if !ok || !in(o) || !isEntry(v.Program, o) {
			continue
		}
		out = append(out, entryOf(v, o))
	}
	return out
}

// fileSet は、op が、files のファイルに宣言されているか（単位の対象か）を答える関数。
func fileSet(files []string) func(*ir.Op) bool {
	set := make(map[string]bool, len(files))
	for _, f := range files {
		set[f] = true
	}
	return func(o *ir.Op) bool { return set[o.Span.File] }
}

// isEntry は、op o が、入口のデコレータ（@entry）を、一つでも持つか。
func isEntry(p *ir.Program, o *ir.Op) bool {
	for i := range o.Decorators {
		if dec, ok := p.Lookup(o.Decorators[i].Decorator).(*ir.Decorator); ok && dec.Entry {
			return true
		}
	}
	return false
}

// entryOf は、入口 root の、求めるケース。
func entryOf(v *ir.View, root *ir.Op) ir.Entry {
	p := v.Program
	w := &entryWalker{v: v, p: p, root: root, declNS: map[string]string{}, index: map[string]int{}, caughtIndex: map[string]bool{}, stack: map[string]bool{}}
	for i := range p.Namespaces {
		for _, d := range p.Namespaces[i].Decls {
			if d != nil {
				if _, dup := w.declNS[d.Common().Name]; !dup {
					w.declNS[d.Common().Name] = p.Namespaces[i].Name
				}
			}
		}
	}
	w.op(nil, entryUnit{ns: v.Viewer, cfg: v.Config, in: fileSet(v.Files)}, root.Name, nil, entryCond{})
	e := ir.Entry{Op: root.Name, Normal: !ir.IsNever(root.Returns)}
	if w.blocked != "" {
		e.Blocked = w.blocked
		return e
	}
	seen := map[string]bool{}
	for _, x := range root.Signature.Exceptions {
		if seen[x] {
			continue
		}
		seen[x] = true
		n := len(e.Sources)
		for _, s := range w.found {
			if s.Exception == x {
				e.Sources = append(e.Sources, s)
			}
		}
		if len(e.Sources) == n {
			e.Sources = append(e.Sources, ir.EntrySource{Exception: x, Kind: ir.SourceUnknown, Why: "出どころが、流れの中に、見つからない"})
		}
	}
	for _, s := range w.caught {
		if _, also := w.index[s.Exception+"\x00"+s.Label]; also && s.Why == "" {
			// 同じ出どころが、受ける文の外でも、呼ばれる: 差し替えは、どの呼び出しにも、効くので、受ける道だけを、通せない
			s.Steps, s.Why = nil, "同じ出どころが、受ける文の外でも、呼ばれる"
		}
		e.Caught = append(e.Caught, s)
	}
	return e
}

// entryUnit は、辿っている出力: 名前空間と、その単位の設定と、単位の対象かを答える関数。via は、入口の名前空間から、ここまでに
// 通った名前空間。
type entryUnit struct {
	ns  string
	cfg ir.NamespaceConfig
	in  func(*ir.Op) bool
	via []string
}

// entryCond は、そこまで届くための条件: 差し替えに言うこと（steps）か、入れない理由（why）。
type entryCond struct {
	steps []ir.EntryStep
	why   string
}

func (c entryCond) with(steps []ir.EntryStep, why string) entryCond {
	if c.why != "" {
		return c
	}
	if why != "" {
		return entryCond{why: why}
	}
	return entryCond{steps: append(append([]ir.EntryStep(nil), c.steps...), steps...)}
}

// entryFrame は、辿っている op 一つ。
type entryFrame struct {
	unit   entryUnit
	op     *ir.Op
	call   *ir.Call // 呼んだ側の呼び出し（入口なら nil）
	parent *entryFrame
	// deco は、op に付いた、本体のあるデコレータの本体を辿っているときの、そのデコレータ。そのとき、body は、その本体の文で、
	// proceed は、包んだ関数の呼び出しの先（内側の層）を辿る関数。op の配線を辿っているときは nil。
	deco    *ir.Decorator
	body    []ir.Statement
	proceed func(c entryCond)
}

func (fr *entryFrame) statements() []ir.Statement {
	if fr.deco != nil {
		return fr.body
	}
	if fr.op.Body == nil {
		return nil
	}
	return fr.op.Body.Statements
}

type entryWalker struct {
	v           *ir.View
	p           *ir.Program
	root        *ir.Op
	declNS      map[string]string
	found       []ir.EntrySource
	index       map[string]int
	caught      []ir.EntrySource
	caughtIndex map[string]bool
	stack       map[string]bool
	blocked     string
	// capture は、try のブロックを辿っている間の、出どころの受け皿。nil でなければ、add は、found ではなく、ここに足す。
	capture *[]heldEntry
	// catches は、今、中にいる catch のブロック（外から内へ）。
	catches []caughtEntries
}

// heldEntry は、try のブロックの中で見つけて、取っておいた出どころ（add の引数のまま）。
type heldEntry struct {
	fr  *entryFrame
	s   ir.EntrySource
	c   entryCond
	own []ir.EntryStep
}

type caughtEntries struct {
	fr      *entryFrame
	name    string
	sources []heldEntry
}

// through は、出力 u から見た、別の名前空間 m: 同じ単位が m も出力するなら、それ。なければ、同じ基底の単位が、ちょうど一つなら、それ。
// n は、同じ基底の単位の数（同じ単位のものがあれば 1）。
func (w *entryWalker) through(u entryUnit, m string) (cfg *ir.NamespaceConfig, n int) {
	list := w.v.Namespaces[m]
	for i := range list {
		if list[i].Index == u.cfg.Index {
			return &list[i], 1
		}
	}
	for i := range list {
		if list[i].Base == u.cfg.Base {
			cfg = &list[i]
			n++
		}
	}
	if n != 1 {
		return nil, n
	}
	return cfg, 1
}

// enterNS は、出力 u から、別の名前空間 m の、通す出力へ入った entryUnit。
func (w *entryWalker) enterNS(u entryUnit, m string, cfg *ir.NamespaceConfig) entryUnit {
	in := func(*ir.Op) bool { return true }
	if cfg.Index == w.v.Config.Index {
		in = fileSet(w.v.Files)
	}
	return entryUnit{ns: m, cfg: *cfg, in: in, via: append(append([]string(nil), u.via...), m)}
}

func (w *entryWalker) step(u entryUnit, verb ir.EntryStepVerb, kind ir.EntryTargetKind, name string) ir.EntryStep {
	return ir.EntryStep{Via: append([]string{}, u.via...), Verb: verb, Kind: kind, Name: name}
}

func (w *entryWalker) throws(u entryUnit, kind ir.EntryTargetKind, name, exception string) []ir.EntryStep {
	s := w.step(u, ir.StepThrows, kind, name)
	s.Exception = exception
	return []ir.EntryStep{s}
}

// rethrow は、`throw e`（受けた例外 name を、そのまま投げ直す）の出どころ: try のブロックの中の出どころを、そのまま、出どころにする。
// keep が nil でなければ、keep が真の例外だけ。why が空でなければ、その理由で、入れない。
func (w *entryWalker) rethrow(fr *entryFrame, name string, keep func(exception string) bool, why string) {
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

// catchCond は、catch のブロック（cases が nil でなければ、受けた例外の match の、そのケースを受けるアーム）に、入るための条件:
// try のブロックの中の、差し替えで起こせる、最初の出どころに、投げさせる。なければ、入れない。
func (w *entryWalker) catchCond(held []heldEntry, keep func(exception string) bool) entryCond {
	for _, h := range held {
		if keep != nil && !keep(h.s.Exception) {
			continue
		}
		if c := h.c.with(h.own, ""); c.why == "" {
			return c
		}
	}
	return entryCond{why: "例外を受けた後の道にあり、そこへ入らせる出どころを、差し替えで、起こせない"}
}

// addCaught は、受ける道を足す: try のブロックの中の出どころ h が投げた例外を、catch が受けて、投げずに終わる。
func (w *entryWalker) addCaught(h heldEntry) {
	s := h.s
	c := h.c.with(h.own, "")
	s.Steps, s.Why = c.steps, c.why
	if c.why != "" {
		s.Steps = nil
	}
	key := s.Exception + "\x00" + s.Label
	if w.caughtIndex[key] {
		return
	}
	w.caughtIndex[key] = true
	w.caught = append(w.caught, s)
}

// thrown は、catch のブロックの `throw 例外`（文か、受けた例外の match のアームの本体）の出どころ。名前は `<配線の op の FQN>.catch`
// （デコレータの本体なら `<op の FQN>@<デコレータの FQN>.catch`）。アームなら、後ろに `.<ケース>`。
func (w *entryWalker) thrown(fr *entryFrame, body *ir.ThrowBody, cases []string, c entryCond) {
	s := ir.EntrySource{Exception: body.Exception, Kind: ir.SourceArm, Name: fr.op.Name, Catch: true, Cases: append([]string(nil), cases...)}
	s.Label = fr.op.Name
	if fr.deco != nil {
		s.Decorator = fr.deco.Name
		s.Label += "@" + fr.deco.Name
	}
	s.Label += ".catch"
	if len(cases) > 0 {
		s.Label += "." + strings.Join(cases, "+")
	}
	w.add(fr, s, c, nil)
}

// add は、出どころを足す。入口まで伝わらない例外（途中の op の signature にないもの）は、足さない。
func (w *entryWalker) add(fr *entryFrame, s ir.EntrySource, c entryCond, own []ir.EntryStep) {
	if w.capture != nil {
		*w.capture = append(*w.capture, heldEntry{fr: fr, s: s, c: c, own: own})
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
	s.Steps, s.Why = c.steps, c.why
	if c.why != "" {
		s.Steps = nil
	}
	key := s.Exception + "\x00" + s.Label
	if i, dup := w.index[key]; dup {
		if w.found[i].Why != "" && s.Why == "" {
			w.found[i] = s
		}
		return
	}
	w.index[key] = len(w.found)
	w.found = append(w.found, s)
}

// op は、出力 u の中で、op fqn が呼ばれたときの、出どころを足す。caller は、呼んだ側（入口なら nil）。
func (w *entryWalker) op(caller *entryFrame, u entryUnit, fqn string, call *ir.Call, c entryCond) {
	o, _ := w.p.Lookup(fqn).(*ir.Op)
	if o == nil {
		return
	}
	if ns := w.declNS[fqn]; ns != u.ns {
		// 別の名前空間: 境界の op（入口のデコレータを持つ）と、通す出力のない名前空間の op は、流れの端。そうでなければ、その中へ
		cfg, n := w.through(u, ns)
		if n > 1 {
			if w.blocked == "" {
				w.blocked = "名前空間 " + ns + " を出力する、基底 " + u.cfg.Base + " の単位が、二つ以上ある"
			}
			return
		}
		w.boundary(caller, u, o, c)
		if n == 0 || isEntry(w.p, o) {
			for _, x := range o.Signature.Exceptions {
				w.add(caller, ir.EntrySource{Exception: x, Kind: ir.SourceExternal, Label: fqn, Name: fqn}, c, w.throws(u, ir.StepOp, fqn, x))
			}
			return
		}
		u = w.enterNS(u, ns, cfg)
	}
	fr := &entryFrame{unit: u, op: o, call: call, parent: caller}
	w.layer(fr, w.layers(u, o), 0, c, map[string]bool{})
}

// boundary は、別の名前空間から呼んだ op o の、境界の出どころ: o に付いたデコレータの outer throws（docs/grammar.md「decorator の宣言」）。
// 差し替える相手は、その op（流れの端として、にせものにできる言語では、そのにせものが、投げる）。
func (w *entryWalker) boundary(caller *entryFrame, u entryUnit, o *ir.Op, c entryCond) {
	for i := range o.Decorators {
		dec, _ := w.p.Lookup(o.Decorators[i].Decorator).(*ir.Decorator)
		if dec == nil {
			continue
		}
		for _, clause := range dec.OuterThrows {
			for _, x := range clause.Exceptions {
				if strings.HasPrefix(x, "$param:") {
					continue
				}
				w.add(caller, ir.EntrySource{Exception: x, Kind: ir.SourceBoundary, Label: o.Name + "@" + dec.Name, Name: o.Name, Decorator: dec.Name},
					c, w.throws(u, ir.StepOp, o.Name, x))
			}
		}
	}
}

// entryLayer は、op の中身を包むデコレータ一つ（書かれた順。先頭が最も外側）。
type entryLayer struct {
	dec *ir.Decorator
}

// layers は、出力 u の op o の層: 単位の対象の op に付いた、デコレータ（書かれた順。同じデコレータは、一度）のうち、本体のあるものと、
// 本体がなく、throws の節を持つもの。
func (w *entryWalker) layers(u entryUnit, o *ir.Op) []entryLayer {
	if !u.in(o) {
		return nil
	}
	var out []entryLayer
	seen := map[string]bool{}
	for i := range o.Decorators {
		dec, _ := w.p.Lookup(o.Decorators[i].Decorator).(*ir.Decorator)
		if dec == nil || seen[dec.Name] {
			continue
		}
		if dec.Body != nil || len(dec.Throws) > 0 {
			seen[dec.Name] = true
			out = append(out, entryLayer{dec: dec})
		}
	}
	return out
}

// layer は、op の層 i（0 が、いちばん外。len(layers) が、op の中身）から内側の、出どころを足す。本体のないデコレータは、
// 中身の前に、デコレータが投げる。本体のあるデコレータは、本体を、配線として辿り、包んだ関数の呼び出しの所で、内側の層へ進む。
func (w *entryWalker) layer(fr *entryFrame, layers []entryLayer, i int, c entryCond, byDecorator map[string]bool) {
	o, u, fqn := fr.op, fr.unit, fr.op.Name
	if i < len(layers) {
		dec := layers[i].dec
		if dec.Body != nil {
			in := *fr
			in.deco, in.body = dec, dec.Body.Statements
			in.proceed = func(pc entryCond) { w.layer(fr, layers, i+1, pc, byDecorator) }
			w.stmts(&in, in.body, c)
			return
		}
		for _, clause := range dec.Throws {
			for _, x := range clause.Exceptions {
				if strings.HasPrefix(x, "$param:") {
					continue
				}
				byDecorator[x] = true
				st := w.step(u, ir.StepThrows, ir.StepDecorator, fqn)
				st.Decorator, st.Exception = dec.Name, x
				w.add(fr, ir.EntrySource{Exception: x, Kind: ir.SourceDecorator, Label: fqn + "@" + dec.Name, Name: fqn, Decorator: dec.Name}, c, []ir.EntryStep{st})
			}
		}
		w.layer(fr, layers, i+1, c, byDecorator)
		return
	}
	switch {
	case u.in(o) && o.Body != nil:
		// 単位の対象の配線
		if w.stack[fqn] {
			return
		}
		w.stack[fqn] = true
		w.stmts(fr, o.Body.Statements, c)
		delete(w.stack, fqn)
	case o.Body == nil && ir.IsNever(o.Returns):
		for _, clause := range o.Throws {
			for _, x := range clause.Exceptions {
				w.add(fr, ir.EntrySource{Exception: x, Kind: ir.SourceNever, Label: fqn, Name: fqn}, c, nil)
			}
		}
	default:
		// 葉（本体を持たない op）と、単位の対象でない配線
		for _, x := range leafExceptions(o) {
			if byDecorator[x] {
				continue
			}
			w.add(fr, ir.EntrySource{Exception: x, Kind: ir.SourceLeaf, Label: fqn, Name: fqn}, c, w.throws(u, ir.StepLeaf, fqn, x))
		}
	}
}

// leafExceptions は、葉として扱う op o が、自分で投げうる例外: 本体を持たない op は、節に書いた例外（葉の op は、節が、定義）。
// 単位の対象でない配線は、計算した例外（Op.signature.exceptions）。
func leafExceptions(o *ir.Op) []string {
	if o.Body != nil {
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

func (w *entryWalker) stmts(fr *entryFrame, list []ir.Statement, c entryCond) {
	for _, st := range list {
		switch s := st.(type) {
		case ir.ForStmt:
			w.stmts(fr, s.Body, c.with(nil, "繰り返し（for）の中にある"))
		case ir.DisjointStmt:
			for _, arm := range s.Arms {
				w.stmts(fr, arm.Statements, c)
			}
		case ir.TryStmt:
			w.try(fr, &s, c)
		case ir.ThrowStmt:
			if s.Throw.Caught != "" {
				w.rethrow(fr, s.Throw.Caught, nil, "")
			} else {
				w.thrown(fr, &s.Throw, nil, c)
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

// try は、例外を受ける文。try のブロックの中の出どころは、取っておく: catch のブロックの throw e が、そのまま、出どころにし、
// 投げずに終わる道が、受ける道にする。
func (w *entryWalker) try(fr *entryFrame, s *ir.TryStmt, c entryCond) {
	var held []heldEntry
	saved := w.capture
	w.capture = &held
	w.stmts(fr, s.Body, c)
	w.capture = saved
	w.catches = append(w.catches, caughtEntries{fr: fr, name: s.Catch.Name, sources: held})
	// catch のブロックに、受けた例外の match（`catch (e by C)` か、`match (e by C)` の文）があれば、アームごとに決める。
	// なければ、ブロックの全体が、すべての例外を受ける
	classified := false
	for _, st := range s.Catch.Body {
		if e, ok := ir.StatementExpr(st); ok {
			if m, ok := e.(ir.MatchExpr); ok {
				if cs, ok := m.Match.Subject.(ir.CaughtSubject); ok && cs.Name == s.Catch.Name {
					classified = true
				}
			}
		}
	}
	w.stmts(fr, s.Catch.Body, w.catchCond(held, nil))
	if !classified && !s.Catch.Never {
		for _, h := range held {
			w.addCaught(h)
		}
	}
	w.catches = w.catches[:len(w.catches)-1]
}

func (w *entryWalker) call(fr *entryFrame, call *ir.Call, c entryCond) {
	switch callee := call.Callee.(type) {
	case ir.ProceedCallee:
		if fr.proceed != nil {
			fr.proceed(c)
		}
	case ir.OpCallee:
		w.op(fr, fr.unit, callee.Op, call, c)
	case ir.EffectCallee:
		named, ok := callee.Effect.(ir.NamedEffect)
		if !ok {
			return
		}
		eff, _ := w.p.Lookup(named.Effect).(*ir.Effect)
		if eff == nil {
			return
		}
		for _, x := range eff.Throws {
			w.add(fr, ir.EntrySource{Exception: x, Kind: ir.SourceEffect, Label: named.Effect, Name: named.Effect}, c,
				w.throws(fr.unit, ir.StepEffect, named.Effect, x))
		}
	}
}

// held は、今いる catch のブロック（fr の、受けた例外 name）の、try のブロックの中の出どころ。
func (w *entryWalker) held(fr *entryFrame, name string) []heldEntry {
	for i := len(w.catches) - 1; i >= 0; i-- {
		if w.catches[i].fr == fr && w.catches[i].name == name {
			return w.catches[i].sources
		}
	}
	return nil
}

func (w *entryWalker) match(fr *entryFrame, m *ir.Match, c entryCond) {
	if cs, caught := m.Subject.(ir.CaughtSubject); caught {
		// 受けた例外の match: アームは、そのアームが受けるケースの例外だけを、受ける
		cl, _ := w.p.Lookup(m.Classify).(*ir.Classify)
		held := w.held(fr, cs.Name)
		for i := range m.Arms {
			arm := &m.Arms[i]
			keep := func(x string) bool {
				if cl == nil {
					return false
				}
				name, ok := cl.CaseOfException(x)
				return ok && contains(arm.Cases, name)
			}
			switch body := arm.Body.(type) {
			case ir.ThrowBody:
				switch {
				case body.Caught == cs.Name:
					w.rethrow(fr, body.Caught, keep, "")
				case body.Caught != "":
					w.rethrow(fr, body.Caught, nil, "")
				default:
					w.thrown(fr, &body, arm.Cases, w.catchCond(held, keep))
				}
			default:
				// 投げないアーム: そのケースの例外は、受けられて、投げずに終わる
				if call, ok := arm.Body.(ir.CallExpr); ok {
					w.call(fr, &call.Call, w.catchCond(held, keep))
				}
				for _, h := range held {
					if keep(h.s.Exception) {
						w.addCaught(h)
					}
				}
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
			s := ir.EntrySource{Exception: body.Exception, Kind: ir.SourceArm, Name: fr.op.Name, Cases: append([]string(nil), arm.Cases...)}
			s.Label = fr.op.Name
			if fr.deco != nil {
				// デコレータの本体のアーム: <op の FQN>@<デコレータの FQN>.<ケース>
				s.Decorator = fr.deco.Name
				s.Label += "@" + fr.deco.Name
			}
			s.Label += "." + strings.Join(arm.Cases, "+")
			w.add(fr, s, c.with(steps, why), nil)
		case ir.CallExpr:
			cc := c
			if nonThrow > 1 {
				// 正常のケースが、このアームを通るとは、限らない: 差し替えで、入らせる
				cc = c.with(w.enter(fr, m, arm))
			}
			w.call(fr, &body.Call, cc)
		}
	}
}

// enter は、match m のアーム arm に、差し替えで、入らせる方法。入らせられなければ、理由。
func (w *entryWalker) enter(fr *entryFrame, m *ir.Match, arm *ir.MatchArm) ([]ir.EntryStep, string) {
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
	st := w.step(fr.unit, ir.StepIs, ir.StepClassify, m.Classify)
	st.Case = want
	return []ir.EntryStep{st}, ""
}

// noneable は、型 t の値に、「ない」を返せるか: 省略可能な型か、並び。
func noneable(t ir.Type) bool {
	switch t.(type) {
	case ir.OptionalType, ir.ArrayType:
		return true
	}
	return false
}

// none は、fr の配線の中の atom の値を、「ない」にする方法: 値の出どころを辿り、差し替えに、「ない」を返させる。
func (w *entryWalker) none(fr *entryFrame, atom ir.Atom, depth int) ([]ir.EntryStep, string) {
	if depth > 16 {
		return nil, "値の出どころを、辿りきれない"
	}
	switch a := atom.(type) {
	case ir.StateAtom:
		if len(a.Path) > 0 {
			return nil, "状態 " + a.State + " の値の一部で決まる"
		}
		return []ir.EntryStep{w.step(fr.unit, ir.StepNone, ir.StepState, a.State)}, ""
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

func (w *entryWalker) noneExpr(fr *entryFrame, e ir.Expr, depth int) ([]ir.EntryStep, string) {
	switch x := e.(type) {
	case ir.AtomExpr:
		return w.none(fr, x.Atom, depth+1)
	case ir.CallExpr:
		switch callee := x.Call.Callee.(type) {
		case ir.ReadCallee:
			r, _ := w.p.Lookup(callee.Read).(*ir.Read)
			if r == nil || !noneable(r.Returns) {
				return nil, "read " + callee.Read + " が、「ない」を返せない"
			}
			return []ir.EntryStep{w.step(fr.unit, ir.StepNone, ir.StepRead, callee.Read)}, ""
		case ir.OpCallee:
			return w.noneOp(fr, callee.Op, &x.Call, depth)
		}
	}
	return nil, "値の出どころを、辿れない"
}

// noneOp は、fr の配線が呼ぶ op fqn の戻り値を、「ない」にする方法。
func (w *entryWalker) noneOp(fr *entryFrame, fqn string, call *ir.Call, depth int) ([]ir.EntryStep, string) {
	o, _ := w.p.Lookup(fqn).(*ir.Op)
	if o == nil || !noneable(o.Returns) {
		return nil, "op " + fqn + " が、「ない」を返せない"
	}
	u := fr.unit
	if ns := w.declNS[fqn]; ns != u.ns {
		cfg, n := w.through(u, ns)
		if n > 1 {
			return nil, "op " + fqn + " の名前空間を出力する単位が、二つ以上ある"
		}
		if n == 0 || isEntry(w.p, o) {
			return []ir.EntryStep{w.step(u, ir.StepNone, ir.StepOp, fqn)}, ""
		}
		u = w.enterNS(u, ns, cfg)
	}
	in := &entryFrame{unit: u, op: o, call: call, parent: fr}
	switch {
	case u.in(o) && o.Body != nil:
		// 配線の op: return の式を、辿る（return が、一つだけのとき）
		var ret ir.Expr
		n := 0
		ir.WalkStatements(o.Body.Statements, func(st ir.Statement) {
			if s, ok := st.(ir.ReturnStmt); ok {
				n++
				ret = s.Expr
			}
		})
		if n != 1 || ret == nil {
			return nil, "op " + fqn + " の戻り値の出どころが、一つに決まらない"
		}
		return w.noneExpr(in, ret, depth+1)
	case o.Body == nil && ir.IsNever(o.Returns):
		return nil, "op " + fqn + " の戻り値を、辿れない"
	}
	return []ir.EntryStep{w.step(u, ir.StepNone, ir.StepLeaf, fqn)}, ""
}
