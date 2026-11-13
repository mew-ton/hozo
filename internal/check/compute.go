package check

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// Compute は docs/check.md「計算」を行い、すべての op の Op.Signature（= Sig(O, ns(O))）と
// すべての呼び出しの Call.Bound を埋める。計算は呼び出しの依存の順に行い、循環に含まれる op は
// 本体を無視して葉として計算する（WIRE-7）。
func Compute(p *ir.Program) {
	ix := newIndex(p)
	c := newComputer(ix, &reporter{})
	c.computeAll()
}

// sigKey は Sig(O, v) のメモのキー。
type sigKey struct{ op, viewer string }

// sigState は Sig(O, v) の四つの値（docs/check.md「Sig(O, v)」）。
type sigState struct {
	E    []string                             // 例外の列
	F    []string                             // 効果の列
	rel  map[string]map[string]ir.EffectState // rel[X][e]。ないものは unknown
	succ map[string]ir.EffectState            // succ[e]。ないものは kept
}

func newSigState() *sigState {
	return &sigState{rel: map[string]map[string]ir.EffectState{}, succ: map[string]ir.EffectState{}}
}

func (s *sigState) hasE(x string) bool { return contains(s.E, x) }
func (s *sigState) hasF(e string) bool { return contains(s.F, e) }

func (s *sigState) addE(x string) { s.E = appendUnique(s.E, x) }
func (s *sigState) addF(e string) { s.F = appendUnique(s.F, e) }

func (s *sigState) removeE(x string) {
	out := s.E[:0:0]
	for _, y := range s.E {
		if y != x {
			out = append(out, y)
		}
	}
	s.E = out
	delete(s.rel, x)
}

func (s *sigState) relOf(x, e string) ir.EffectState {
	if m, ok := s.rel[x]; ok {
		if st, ok := m[e]; ok {
			return st
		}
	}
	return ir.Unknown
}

func (s *sigState) setRel(x, e string, st ir.EffectState) {
	m, ok := s.rel[x]
	if !ok {
		m = map[string]ir.EffectState{}
		s.rel[x] = m
	}
	m[e] = st
}

func (s *sigState) succOf(e string) ir.EffectState {
	if st, ok := s.succ[e]; ok {
		return st
	}
	return ir.Kept
}

// signature は ir.Signature にする。relations は E × F のすべての組を E の順、F の順に、
// successEffects は F の順に並べる。
func (s *sigState) signature() ir.Signature {
	sig := ir.Signature{Exceptions: append([]string{}, s.E...), Effects: append([]string{}, s.F...)}
	sig.Relations = make([]ir.SignatureRelation, 0, len(s.E)*len(s.F))
	for _, x := range s.E {
		for _, e := range s.F {
			sig.Relations = append(sig.Relations, ir.SignatureRelation{Exception: x, Effect: e, State: s.relOf(x, e)})
		}
	}
	sig.SuccessEffects = make([]ir.SuccessEffect, 0, len(s.F))
	for _, e := range s.F {
		sig.SuccessEffects = append(sig.SuccessEffects, ir.SuccessEffect{Effect: e, State: s.succOf(e)})
	}
	return sig
}

// relationState は ThrowsClause の relations から、例外 x のときの効果の状態を決める（葉の rel の規則）。
// x を含む節に prevents があれば prevented、keeps があれば kept、どちらもなければ unknown、両方あれば
// unknown（REL-5 は internal/sema が別に出す）。目的語は常にその op のすべての効果（`effects`）なので、
// 状態は効果によらない。
func relationState(clauses []ir.ThrowsClause, x string, subst func(string) []string) ir.EffectState {
	prevents, keeps := false, false
	for _, cl := range clauses {
		found := false
		for _, y := range cl.Exceptions {
			if contains(subst(y), x) {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		for _, r := range cl.Relations {
			switch r.Kind {
			case ir.Prevents:
				prevents = true
			case ir.Keeps:
				keeps = true
			}
		}
	}
	switch {
	case prevents && keeps:
		return ir.Unknown
	case prevents:
		return ir.Prevented
	case keeps:
		return ir.Kept
	}
	return ir.Unknown
}

func identity(s string) []string { return []string{s} }

// declared は Declared(O): Op.throws の例外を出現順に重複なく集めたもの。
func declared(o *ir.Op) []string {
	var out []string
	for _, cl := range o.Throws {
		for _, x := range cl.Exceptions {
			out = appendUnique(out, x)
		}
	}
	return out
}

// callRef は配線の本体の一つの呼び出し。
type callRef struct {
	row  int     // 行（0 始まり）
	arm  int     // match のアーム（アームでなければ -1）
	call ir.Call // 呼び出し（写し）
	stmt ir.Span // 行の位置
}

// row は配線の本体の一行。
type row struct {
	calls   []callRef
	isMatch bool
	arms    int // match のアームの数（none のアームを含む）
}

// rowsOf は配線の本体を行ごとの呼び出しに分ける（docs/check.md「手順 0」の「行 i の呼び出し」）。
func rowsOf(w *ir.Wiring) []row {
	if w == nil {
		return nil
	}
	rows := make([]row, 0, len(w.Statements))
	for i, st := range w.Statements {
		var r row
		var expr ir.Expr
		var span ir.Span
		switch s := st.(type) {
		case ir.ConstStmt:
			expr, span = s.Expr, s.Span
		case ir.ReturnStmt:
			expr, span = s.Expr, s.Span
		case ir.CallStmt:
			expr, span = ir.CallExpr{Call: s.Call}, s.Span
		case ir.MatchStmt:
			expr, span = ir.MatchExpr{Match: s.Match}, s.Span
		}
		switch e := expr.(type) {
		case ir.CallExpr:
			r.calls = append(r.calls, callRef{row: i, arm: -1, call: e.Call, stmt: span})
		case ir.MatchExpr:
			r.isMatch = true
			r.arms = len(e.Match.Arms)
			for j, arm := range e.Match.Arms {
				if ce, ok := arm.Body.(ir.CallExpr); ok {
					r.calls = append(r.calls, callRef{row: i, arm: j, call: ce.Call, stmt: span})
				}
			}
		}
		rows = append(rows, r)
	}
	return rows
}

// mapCalls は配線のすべての呼び出しに fn を適用し、変更を本体に書き戻す（Call.Bound を埋めるため）。
func mapCalls(w *ir.Wiring, fn func(c *ir.Call)) {
	if w == nil {
		return
	}
	mapExpr := func(e ir.Expr) ir.Expr {
		switch x := e.(type) {
		case ir.CallExpr:
			fn(&x.Call)
			return x
		case ir.MatchExpr:
			arms := append([]ir.MatchArm(nil), x.Match.Arms...)
			for j := range arms {
				if ce, ok := arms[j].Body.(ir.CallExpr); ok {
					fn(&ce.Call)
					arms[j].Body = ce
				}
			}
			x.Match.Arms = arms
			return x
		}
		return e
	}
	for i, st := range w.Statements {
		switch s := st.(type) {
		case ir.ConstStmt:
			s.Expr = mapExpr(s.Expr)
			w.Statements[i] = s
		case ir.ReturnStmt:
			s.Expr = mapExpr(s.Expr)
			w.Statements[i] = s
		case ir.CallStmt:
			fn(&s.Call)
			w.Statements[i] = s
		case ir.MatchStmt:
			m := mapExpr(ir.MatchExpr{Match: s.Match}).(ir.MatchExpr)
			s.Match = m.Match
			w.Statements[i] = s
		}
	}
}

// source は例外の出どころ（行 i, 呼び出し c）。
type exSource struct {
	ref callRef
	sig *sigState // Sc
}

// bodyInfo は手順 0 で集めた配線の情報。THROWS-1 と REL-4 が使う。
type bodyInfo struct {
	rows    []row
	sigs    [][]*sigState         // rows と同じ並びの Sc
	thrown  []string              // ∪ Sc.exceptions（出現順）
	sources map[string][]exSource // 例外 → 出どころ
}

// computer は Sig(O, v) をメモしながら計算する。診断は v = ns(O) の計算のときにだけ出す。
type computer struct {
	ix     *index
	r      *reporter
	memo   map[sigKey]*sigState
	cyclic map[string]bool // 循環に含まれる op（本体を無視する）
	bodies map[string]*bodyInfo
	binds  *bindTable
}

func newComputer(ix *index, r *reporter) *computer {
	c := &computer{ix: ix, r: r, memo: map[sigKey]*sigState{}, cyclic: map[string]bool{}, bodies: map[string]*bodyInfo{}}
	c.binds = newBindTable(ix)
	return c
}

// computeAll は WIRE-7 の検出、全 op の Signature、全呼び出しの Call.Bound を順に行う。
func (c *computer) computeAll() {
	c.detectCycles()
	for i, o := range c.ix.ops {
		o.Signature = c.sig(o, c.ix.opsNS[i]).signature()
	}
	c.fillBound()
}

// detectCycles は配線の呼び出しのグラフの強連結成分を出現順に求め、循環（二つ以上の op、または自分を呼ぶ op）を
// WIRE-7 にして、その op を葉として扱う印を付ける。
func (c *computer) detectCycles() {
	ix := c.ix
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	counter := 0
	edges := func(fqn string) []string {
		var out []string
		o := ix.op(fqn)
		if o == nil || o.Body == nil {
			return nil
		}
		for _, r := range rowsOf(o.Body) {
			for _, cr := range r.calls {
				if ix.op(cr.call.Op) != nil {
					out = appendUnique(out, cr.call.Op)
				}
			}
		}
		return out
	}
	var visit func(fqn string)
	visit = func(fqn string) {
		index[fqn] = counter
		low[fqn] = counter
		counter++
		stack = append(stack, fqn)
		onStack[fqn] = true
		for _, to := range edges(fqn) {
			if _, seen := index[to]; !seen {
				visit(to)
				if low[to] < low[fqn] {
					low[fqn] = low[to]
				}
			} else if onStack[to] && index[to] < low[fqn] {
				low[fqn] = index[to]
			}
		}
		if low[fqn] != index[fqn] {
			return
		}
		var scc []string
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			scc = append(scc, top)
			if top == fqn {
				break
			}
		}
		if len(scc) > 1 || contains(edges(fqn), fqn) {
			c.reportCycle(scc)
		}
	}
	for _, o := range ix.ops {
		if _, seen := index[o.Name]; !seen {
			visit(o.Name)
		}
	}
}

// reportCycle は WIRE-7 を出し、循環上の op に葉として扱う印を付ける。位置は循環上で出現順に最初の op、
// Related に循環上のすべての op。
func (c *computer) reportCycle(scc []string) {
	var members []*ir.Op
	for _, o := range c.ix.ops {
		if contains(scc, o.Name) {
			members = append(members, o)
			c.cyclic[o.Name] = true
		}
	}
	checkWire7(members, c.r)
}

// sig は Sig(O, v)（docs/check.md「Sig(O, v)」）。
func (c *computer) sig(o *ir.Op, viewer string) *sigState {
	key := sigKey{o.Name, viewer}
	if s, ok := c.memo[key]; ok {
		return s
	}
	ns := c.ix.namespaceOf(o.Name)
	emit := viewer == ns
	s := newSigState()
	c.memo[key] = s // 循環の保険。detectCycles が循環を葉にしているので、ここには戻らない
	if o.Body == nil || c.cyclic[o.Name] {
		c.leafStep0(o, s)
	} else {
		c.wiringStep0(o, ns, s, emit)
	}
	c.decoratorSteps(o, ns, viewer, s, emit)
	return s
}

// leafStep0 は手順 0（葉の op）。E = Declared(O)、F = Op.effects、rel は Op.throws の relations、succ は kept。
func (c *computer) leafStep0(o *ir.Op, s *sigState) {
	for _, x := range declared(o) {
		s.addE(x)
	}
	for _, e := range o.Effects {
		s.addF(e)
	}
	for _, x := range s.E {
		for _, e := range s.F {
			s.setRel(x, e, relationState(o.Throws, x, identity))
		}
	}
	for _, e := range s.F {
		s.succ[e] = ir.Kept
	}
}

// wiringStep0 は手順 0（配線の op）。本体の行を順に見て E / F / rel / succ を計算する。
func (c *computer) wiringStep0(o *ir.Op, ns string, s *sigState, emit bool) {
	info := &bodyInfo{rows: rowsOf(o.Body), sources: map[string][]exSource{}}
	info.sigs = make([][]*sigState, len(info.rows))
	for i, r := range info.rows {
		for _, cr := range r.calls {
			callee := c.ix.op(cr.call.Op)
			var sc *sigState
			if callee != nil {
				sc = c.sig(callee, ns)
			}
			info.sigs[i] = append(info.sigs[i], sc)
		}
	}
	// F: 各 Sc.effects を行の順、アームの順に連結し、重複を最初の出現に寄せる
	for i := range info.rows {
		for _, sc := range info.sigs[i] {
			if sc == nil {
				continue
			}
			for _, e := range sc.F {
				s.addF(e)
			}
		}
	}
	// E: 各 Sc.exceptions の和に Declared(O) を足す。出どころを集める
	for i, r := range info.rows {
		for j, sc := range info.sigs[i] {
			if sc == nil {
				continue
			}
			for _, x := range sc.E {
				info.thrown = appendUnique(info.thrown, x)
				s.addE(x)
				info.sources[x] = append(info.sources[x], exSource{ref: r.calls[j], sig: sc})
			}
		}
	}
	for _, x := range declared(o) {
		s.addE(x)
	}
	// 行 j < i の呼び出しの effects にあるか
	before := func(e string, i int) bool {
		for j := 0; j < i; j++ {
			for _, sc := range info.sigs[j] {
				if sc != nil && sc.hasF(e) {
					return true
				}
			}
		}
		return false
	}
	// rel
	for _, x := range s.E {
		srcs := info.sources[x]
		for _, e := range s.F {
			if len(srcs) == 0 {
				s.setRel(x, e, relationState(o.Throws, x, identity))
				continue
			}
			var state ir.EffectState
			for k, src := range srcs {
				var st ir.EffectState
				switch {
				case before(e, src.ref.row):
					st = ir.Kept
				case src.sig.hasF(e):
					st = src.sig.relOf(x, e)
				default:
					st = ir.Prevented
				}
				if k == 0 {
					state = st
				} else if st != state {
					state = ir.Unknown
				}
			}
			s.setRel(x, e, state)
		}
	}
	// succ
	for _, e := range s.F {
		kept, prevented, other := 0, 0, 0
		partial := false
		for i, r := range info.rows {
			inArms := 0
			for _, sc := range info.sigs[i] {
				if sc == nil || !sc.hasF(e) {
					continue
				}
				inArms++
				switch sc.succOf(e) {
				case ir.Kept:
					kept++
				case ir.Prevented:
					prevented++
				default:
					other++
				}
			}
			if r.isMatch && inArms > 0 && inArms < r.arms {
				partial = true
			}
		}
		switch {
		case partial || other > 0:
			s.succ[e] = ir.Unknown
		case prevented == 0:
			s.succ[e] = ir.Kept
		case kept == 0:
			s.succ[e] = ir.Prevented
		default:
			s.succ[e] = ir.Unknown
		}
	}
	c.bodies[o.Name] = info
	if emit {
		checkRel4(o, s, c.r)
		checkThrows1(c, o, info)
	}
}

// decoratorSteps は手順 i = 1..n。Op.decorators の末尾（最も内側）から順に、catches の節、次に throws の節を適用する。
func (c *computer) decoratorSteps(o *ir.Op, ns, viewer string, s *sigState, emit bool) {
	for i := len(o.Decorators) - 1; i >= 0; i-- {
		app := &o.Decorators[i]
		d, args := c.ix.decorator(app.Decorator)
		if d == nil {
			continue
		}
		if len(app.Args) > 0 || args == nil {
			args = app.Args
		}
		subst := substituter(args)
		for _, cl := range d.Catches {
			for _, raw := range cl.Exceptions {
				for _, x := range subst(raw) {
					if !s.hasE(x) {
						if emit {
							checkDec16(o, app, x, c.r)
						}
						continue
					}
					switch cl.Mode {
					case ir.Recovers, ir.Ignores:
						var lost []string
						for _, e := range s.F {
							if s.relOf(x, e) != ir.Kept {
								s.succ[e] = ir.Unknown
								lost = append(lost, e)
							}
						}
						if emit && cl.Mode == ir.Recovers && len(lost) > 0 {
							checkDec5(o, app, x, lost, c.r)
						}
						s.removeE(x)
					case ir.Halts:
						s.removeE(x)
						for _, h := range subst(cl.Halts) {
							if s.hasF(h) {
								continue
							}
							s.addF(h)
							for _, y := range s.E {
								s.setRel(y, h, ir.Prevented)
							}
							s.succ[h] = ir.Prevented
						}
					}
				}
			}
		}
		if d.External && viewer == ns {
			continue // DEC-10: external の例外は境界の外から見たときだけ
		}
		for _, cl := range d.Throws {
			for _, raw := range cl.Exceptions {
				for _, y := range subst(raw) {
					existed := s.hasE(y)
					s.addE(y)
					for _, e := range s.F {
						st := relationState([]ir.ThrowsClause{cl}, y, subst)
						if existed && s.relOf(y, e) != st {
							st = ir.Unknown
						}
						s.setRel(y, e, st)
					}
				}
			}
		}
	}
}

// substituter は `$param:名前` をデコレータの適用の引数で置き換える関数を返す。
// ref は一つ、refs と list は複数に展開する。渡されていない引数（省略された省略可能な引数）は空。
// それ以外の名前はそのまま一つ。
func substituter(args []ir.Arg) func(string) []string {
	return func(name string) []string {
		if !strings.HasPrefix(name, "$param:") {
			return []string{name}
		}
		param := strings.TrimPrefix(name, "$param:")
		for _, a := range args {
			if a.Param == param {
				return refTargets(a.Value)
			}
		}
		return nil
	}
}

// refTargets は引数の値が指す宣言の FQN の列。op の引数（"$param:"）とリテラル・バリアントは含めない。
func refTargets(v ir.ArgValue) []string {
	switch x := v.(type) {
	case ir.RefValue:
		if strings.HasPrefix(x.Target, "$param:") {
			return nil
		}
		return []string{x.Target}
	case ir.RefsValue:
		var out []string
		for _, t := range x.Targets {
			if !strings.HasPrefix(t, "$param:") {
				out = appendUnique(out, t)
			}
		}
		return out
	case ir.ListValue:
		var out []string
		for _, item := range x.Items {
			for _, t := range refTargets(item) {
				out = appendUnique(out, t)
			}
		}
		return out
	}
	return nil
}

// decoratorSets は O に付いたデコレータの ∪Catches(dᵢ)、∪Throws(dᵢ, ns(O))（external の throws を除く）、
// external の throws（docs/check.md「throws」の THROWS-1 と「名前空間」の DOC-1 が使う）。`$param:` は置き換え済み。
func (c *computer) decoratorSets(o *ir.Op) (catches, throws, externalThrows []string) {
	for _, app := range o.Decorators {
		d, args := c.ix.decorator(app.Decorator)
		if d == nil {
			continue
		}
		if len(app.Args) > 0 || args == nil {
			args = app.Args
		}
		subst := substituter(args)
		for _, cl := range d.Catches {
			for _, raw := range cl.Exceptions {
				for _, x := range subst(raw) {
					catches = appendUnique(catches, x)
				}
			}
		}
		for _, cl := range d.Throws {
			for _, raw := range cl.Exceptions {
				for _, x := range subst(raw) {
					if d.External {
						externalThrows = appendUnique(externalThrows, x)
					} else {
						throws = appendUnique(throws, x)
					}
				}
			}
		}
	}
	return catches, throws, externalThrows
}

// fillBound はすべての配線の呼び出しの Call.Bound を埋める（docs/check.md「bind」の Call.bound）。
func (c *computer) fillBound() {
	for i, o := range c.ix.ops {
		if o.Body == nil {
			continue
		}
		ns := c.ix.opsNS[i]
		mapCalls(o.Body, func(call *ir.Call) {
			call.Bound = c.binds.boundOf(ns, call)
		})
	}
}
