package check

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// Compute は docs/check.md「計算」を行い、すべての op の Op.Signature（= Sig(O, ns(O))。例外・効果・読む状態・
// 純粋さ）とすべての呼び出しの Call.Bound を埋める。計算は呼び出しの依存の順に行い、循環に含まれる op は
// 本体を無視して葉として計算する（WIRE-7）。
func Compute(p *ir.Program) {
	ix := newIndex(p)
	c := newComputer(ix, &reporter{})
	c.computeAll()
}

// sigKey は Sig(O, v) のメモのキー。
type sigKey struct{ op, viewer string }

// sigState は Sig(O, v) の値（docs/check.md「Sig(O, v)」）のうち、見る側によるもの。
// R と pure は見る側によらないので、computer が op ごとに持つ（computer.reads、computer.pure）。
type sigState struct {
	E    []string                                   // 例外の列
	F    []ir.EffectRef                             // 効果の列（効果の数え方は docs/check.md「節に書く要素」）
	rel  map[string]map[ir.EffectRef]ir.EffectState // rel[X][e]。ないものは unknown
	succ map[ir.EffectRef]ir.EffectState            // succ[e]。ないものは kept
}

func newSigState() *sigState {
	return &sigState{rel: map[string]map[ir.EffectRef]ir.EffectState{}, succ: map[ir.EffectRef]ir.EffectState{}}
}

func (s *sigState) hasE(x string) bool       { return contains(s.E, x) }
func (s *sigState) hasF(e ir.EffectRef) bool { return contains(s.F, e) }

func (s *sigState) addE(x string)       { s.E = appendUnique(s.E, x) }
func (s *sigState) addF(e ir.EffectRef) { s.F = appendUnique(s.F, e) }

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

func (s *sigState) relOf(x string, e ir.EffectRef) ir.EffectState {
	if m, ok := s.rel[x]; ok {
		if st, ok := m[e]; ok {
			return st
		}
	}
	return ir.Unknown
}

func (s *sigState) setRel(x string, e ir.EffectRef, st ir.EffectState) {
	m, ok := s.rel[x]
	if !ok {
		m = map[ir.EffectRef]ir.EffectState{}
		s.rel[x] = m
	}
	m[e] = st
}

func (s *sigState) succOf(e ir.EffectRef) ir.EffectState {
	if st, ok := s.succ[e]; ok {
		return st
	}
	return ir.Kept
}

// signature は ir.Signature にする。relations は E × F のすべての組を E の順、F の順に、
// successEffects は F の順に並べる。
func (s *sigState) signature() ir.Signature {
	sig := ir.Signature{Exceptions: append([]string{}, s.E...), Effects: append([]ir.EffectRef{}, s.F...)}
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

// callRef は配線の本体の一つの呼び出し（op、read、効果）。アームの本体の `throw X` も、例外が X だけで効果を持たない
// 呼び出しとして数える（docs/check.md「手順 0」）。そのときは throw に X が入り、call は Callee が nil で、Args が
// その throw の引数、Span がその throw の位置。
type callRef struct {
	row   int     // 行（0 始まり）
	arm   int     // match のアーム（アームでなければ -1）
	call  ir.Call // 呼び出し（写し）
	throw string  // アームの本体の throw の例外。呼び出しなら空
	stmt  ir.Span // 行の位置
}

// row は配線の本体の一行。
type row struct {
	calls   []callRef
	isMatch bool
	// armBodies は match のアームごとの本体（アームの順）。呼び出しを持たないアーム（値、none）も含む。
	armBodies []ir.ArmBody
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
			for j, arm := range e.Match.Arms {
				r.armBodies = append(r.armBodies, arm.Body)
				switch body := arm.Body.(type) {
				case ir.CallExpr:
					r.calls = append(r.calls, callRef{row: i, arm: j, call: body.Call, stmt: span})
				case ir.ThrowBody:
					if body.Exception != "" {
						r.calls = append(r.calls, callRef{row: i, arm: j, call: ir.Call{Args: body.Args, Span: body.Span}, throw: body.Exception, stmt: span})
					}
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
	// reads と pure は op ごとの R と pure（docs/check.md「reads」「純粋」）。見る側によらない。
	reads map[string][]string
	pure  map[string]bool
}

func newComputer(ix *index, r *reporter) *computer {
	c := &computer{
		ix: ix, r: r, memo: map[sigKey]*sigState{}, cyclic: map[string]bool{}, bodies: map[string]*bodyInfo{},
		reads: map[string][]string{}, pure: map[string]bool{},
	}
	c.binds = newBindTable(ix)
	return c
}

// computeAll は WIRE-7 の検出、全呼び出しの Call.Bound、全 op の Signature を順に行う。
// Call.Bound を先に埋めるのは、読む状態（R）が bind で埋まる位置の状態を含むため（docs/check.md「reads」の 2.2）。
func (c *computer) computeAll() {
	c.detectCycles()
	c.fillBound()
	for i, o := range c.ix.ops {
		o.Signature = c.signature(o, c.ix.opsNS[i])
	}
}

// signature は Sig(O, viewer) を ir.Signature にする。reads と pure は見る側によらない。
func (c *computer) signature(o *ir.Op, viewer string) ir.Signature {
	sig := c.sig(o, viewer).signature()
	sig.Reads = append([]string{}, c.readsOf(o)...)
	sig.Pure = c.pureOf(o)
	return sig
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
				if name := cr.call.CalledOp(); ix.op(name) != nil {
					out = appendUnique(out, name)
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

// leafStep0 は手順 0（葉の op）。E = Declared(O)、F = O が直に持つ効果（デコレータが運ぶ Op.carried.effects。
// STATE-5 に当たった葉は、節に書いた Op.effects を前に並べる。docs/check.md「デコレータが運ぶ reads と effects」）、
// rel は Op.throws の relations、succ は kept。
// 呼び出しが循環している op（WIRE-7）も、節を書いた葉と同じ: F = Op.effects（節に書いたもの）の後ろに、運ばれたもの。
func (c *computer) leafStep0(o *ir.Op, s *sigState) {
	for _, x := range declared(o) {
		s.addE(x)
	}
	for _, e := range o.DirectEffects() {
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

// wiringStep0 は手順 0（配線の op）。本体の行を順に見て E / F / rel / succ を計算し、その後ろに、運ばれた効果を足す
// （addCarried）。
func (c *computer) wiringStep0(o *ir.Op, ns string, s *sigState, emit bool) {
	info := &bodyInfo{rows: rowsOf(o.Body), sources: map[string][]exSource{}}
	info.sigs = make([][]*sigState, len(info.rows))
	for i, r := range info.rows {
		for _, cr := range r.calls {
			info.sigs[i] = append(info.sigs[i], c.callSig(cr, ns))
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
	before := func(e ir.EffectRef, i int) bool {
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
	// succ。本体が「返らない」アーム（throw、never を返す op の呼び出し）は、成功しないので数えない
	for _, e := range s.F {
		kept, prevented, other := 0, 0, 0
		partial := false
		for i, r := range info.rows {
			inArms := 0
			for j, sc := range info.sigs[i] {
				if sc == nil || !sc.hasF(e) || c.neverReturns(r.calls[j]) {
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
			if r.isMatch && inArms > 0 && inArms < c.returningArms(r) {
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
	c.addCarried(o, s)
	c.bodies[o.Name] = info
	if emit {
		checkRel4(o, s, c.r)
		checkThrows1(c, o, info)
	}
}

// addCarried は手順 0（配線の op）の「運ばれた効果」（docs/check.md「Sig(O, v)」）: Op.carried.effects の効果 e を、
// 本体から決めた F / rel / succ に足す。e のデコレータの分は、葉の op と同じ値（rel は Op.throws の relations から、
// succ は kept）: デコレータの処理が、本体のどの行の前後で起きるかを、言語は決めない。本体が行き着かない e は F の末尾に
// 足して、デコレータの分をそのまま持つ。本体も行き着く e は、位置を変えず、本体から決めた状態とデコレータの分が違えば unknown。
func (c *computer) addCarried(o *ir.Op, s *sigState) {
	for _, e := range o.Carried.Effects {
		inBody := s.hasF(e)
		s.addF(e)
		for _, x := range s.E {
			st := relationState(o.Throws, x, identity)
			if inBody && s.relOf(x, e) != st {
				st = ir.Unknown
			}
			s.setRel(x, e, st)
		}
		if inBody && s.succOf(e) != ir.Kept {
			s.succ[e] = ir.Unknown
		} else {
			s.succ[e] = ir.Kept
		}
	}
}

// callSig は手順 0 の表の Sc: 配線の呼び出し一つの、例外・効果・rel・succ（docs/check.md「Sig(O, v)」）。
// op の呼び出しは Sig(callee, ns)。効果の呼び出しは、例外がなく、効果がその効果一つで、succ は kept。read の呼び出しは
// 何も持たない。アームの本体の throw X は、例外が X だけ。呼び出し先が引けなければ nil。
func (c *computer) callSig(cr callRef, ns string) *sigState {
	if cr.throw != "" {
		sc := newSigState()
		sc.addE(cr.throw)
		return sc
	}
	switch callee := cr.call.Callee.(type) {
	case ir.OpCallee:
		if o := c.ix.op(callee.Op); o != nil {
			return c.sig(o, ns)
		}
	case ir.EffectCallee:
		if callee.Effect != nil {
			sc := newSigState()
			sc.addF(callee.Effect)
			sc.succ[callee.Effect] = ir.Kept
			return sc
		}
	case ir.ReadCallee:
		return newSigState()
	}
	return nil
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
								lost = append(lost, e.String())
							}
						}
						if emit && cl.Mode == ir.Recovers && len(lost) > 0 {
							checkDec5(o, app, x, lost, c.r)
						}
						s.removeE(x)
					case ir.Halts:
						s.removeE(x)
						for _, name := range subst(cl.Halts) {
							var h ir.EffectRef = ir.NamedEffect{Effect: name} // halts の効果は、いつも宣言
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

// neverReturns は呼び出し（か throw）が「返らない」か: アームの本体の throw と、never を返す op の呼び出し
// （docs/types.md「式の型」）。
func (c *computer) neverReturns(cr callRef) bool {
	if cr.throw != "" {
		return true
	}
	callee := c.ix.op(cr.call.CalledOp())
	return callee != nil && ir.IsNever(callee.Returns)
}

// returningArms は match の行の、本体が「返らない」でないアームの数（値と none のアームを含む）。
func (c *computer) returningArms(r row) int {
	n := 0
	for _, body := range r.armBodies {
		switch b := body.(type) {
		case ir.ThrowBody:
			continue
		case ir.CallExpr:
			if callee := c.ix.op(b.Call.CalledOp()); callee != nil && ir.IsNever(callee.Returns) {
				continue
			}
		}
		n++
	}
	return n
}

// readsOf は op が読む状態 R（docs/check.md「reads」）。基底の状態の FQN の列で、デコレータが R に足すのは、reads の節が
// 運ぶものだけ（throws / catches の節は R を変えない）。葉の op は直に読む状態（デコレータが運ぶ Op.carried.reads の、
// 要素ごとの状態。read の宣言は、その宛先の状態として数える。STATE-5 に当たった葉は、節に書いた Op.reads を前に並べる）。
// 呼び出しが循環している op も、葉として同じ（Op.reads の要素の状態の後ろに、運ばれた状態）。
// 配線の op は、本体を行の順に見て集め（名前で書いた状態、bind で埋まる位置の状態、呼んだ read の宛先の状態、
// 呼んだ op の R）、その後ろに、運ばれた状態を並べる。
func (c *computer) readsOf(o *ir.Op) []string {
	if r, ok := c.reads[o.Name]; ok {
		return r
	}
	c.reads[o.Name] = nil // 循環の保険。detectCycles が循環を葉にしているので、ここには戻らない
	var out []string
	add := func(states ...string) {
		for _, s := range states {
			if s != "" {
				out = appendUnique(out, s)
			}
		}
	}
	if o.Body == nil || c.cyclic[o.Name] {
		add(c.ix.readStates(o.DirectReads())...)
		c.reads[o.Name] = out
		return out
	}
	call := func(call ir.Call) {
		// 2.1 引数の Atom が参照する状態、2.2 Call.bound の状態、2.3 呼び出し先が op ならその R、read ならその宛先の状態
		// （効果は何も足さない）
		for _, a := range call.Args {
			add(atomStates(a.Value)...)
		}
		for _, b := range call.Bound {
			add(b.State)
		}
		switch callee := call.Callee.(type) {
		case ir.OpCallee:
			if o := c.ix.op(callee.Op); o != nil {
				add(c.readsOf(o)...)
			}
		case ir.ReadCallee:
			if rd, ok := c.ix.decls[callee.Read].(*ir.Read); ok {
				add(rd.State)
			}
		}
	}
	expr := func(e ir.Expr) {
		switch x := e.(type) {
		case ir.CallExpr:
			call(x.Call)
		case ir.AtomExpr:
			add(atomStates(x.Atom)...)
		case ir.MatchExpr:
			// 1. match の対象が読む状態
			m := x.Match
			if atom := m.SubjectAtom(); atom != nil {
				add(atomStates(atom)...)
			} else if cl, ok := c.ix.decls[m.Classify].(*ir.Classify); ok {
				states, _ := cl.SubjectStates()
				add(states...)
			}
			// 2. アームの順に、各アームの本体
			for _, arm := range m.Arms {
				switch body := arm.Body.(type) {
				case ir.CallExpr:
					call(body.Call)
				case ir.AtomExpr:
					add(atomStates(body.Atom)...)
				case ir.ThrowBody:
					// throw の引数の Atom が参照する状態
					for _, a := range body.Args {
						add(atomStates(a.Value)...)
					}
				}
			}
		}
	}
	for _, st := range o.Body.Statements {
		switch s := st.(type) {
		case ir.ConstStmt:
			expr(s.Expr)
		case ir.ReturnStmt:
			expr(s.Expr)
		case ir.CallStmt:
			call(s.Call)
		case ir.MatchStmt:
			expr(ir.MatchExpr{Match: s.Match})
		}
	}
	add(c.ix.readStates(o.Carried.Reads)...)
	c.reads[o.Name] = out
	return out
}

// atomStates は Atom が参照する状態（書かれた順）。レコードリテラルの中の値も見る。
func atomStates(a ir.Atom) []string {
	switch x := a.(type) {
	case ir.StateAtom:
		return []string{x.State}
	case ir.RecordAtom:
		var out []string
		for _, f := range x.Fields {
			out = append(out, atomStates(f.Value)...)
		}
		return out
	}
	return nil
}

// pureOf は op が純粋か（docs/check.md「純粋」）: デコレータが付いていない、R が空、F が空、配線なら本体の呼び出し先が
// すべて純粋。呼び出しが循環している op は、葉としてはじめの三つだけで決める。
func (c *computer) pureOf(o *ir.Op) bool {
	if p, ok := c.pure[o.Name]; ok {
		return p
	}
	c.pure[o.Name] = false // 循環の保険
	pure := len(o.Decorators) == 0 && len(c.readsOf(o)) == 0 && len(c.sig(o, c.ix.namespaceOf(o.Name)).F) == 0
	if pure && o.Body != nil && !c.cyclic[o.Name] {
		for _, r := range rowsOf(o.Body) {
			for _, cr := range r.calls {
				if callee := c.ix.op(cr.call.CalledOp()); callee != nil && !c.pureOf(callee) {
					pure = false
				}
			}
		}
	}
	c.pure[o.Name] = pure
	return pure
}
