package check

import (
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
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

// clone は s の写し（デコレータの本体の 包んだ関数の呼び出しの Sc にする。docs/check.md「デコレータの本体」）。
func (s *sigState) clone() *sigState {
	out := newSigState()
	out.E = append([]string(nil), s.E...)
	out.F = append([]ir.EffectRef(nil), s.F...)
	for x, m := range s.rel {
		out.rel[x] = map[ir.EffectRef]ir.EffectState{}
		for e, st := range m {
			out.rel[x][e] = st
		}
	}
	for e, st := range s.succ {
		out.succ[e] = st
	}
	return out
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
	// rethrow は、`throw e`（受けた例外を、そのまま投げ直す）の e。そのとき throw は空で、call は Callee が nil。
	// 受けた例外の match のアームの本体なら、classify が、その分類、cases が、そのアームが受けるケース（投げ直すのは、当たる例外だけ）。
	rethrow  string
	classify string
	cases    []string
}

// row は配線の本体の一行。
type row struct {
	calls   []callRef
	isMatch bool
	// loops は、この文を囲んでいる for の番号（外から内へ）。
	loops []int
	// armBodies は match のアームごとの本体（アームの順）。呼び出しを持たないアーム（値、none）も含む。
	armBodies []ir.ArmBody
	// regions は、この文を囲んでいる、例外を受ける文のブロック（外から内へ）。
	regions []region
	// caught は、受けた例外の match（`match (e by C)`）の行なら、その e。classify は、その分類、span は、その match。
	caught   string
	classify string
	span     ir.Span
}

// catcher は、この行の例外を受ける、例外を受ける文の番号: 囲んでいるブロックの、いちばん内側の、try のブロック。なければ -1。
func (r row) catcher() int {
	for k := len(r.regions) - 1; k >= 0; k-- {
		if !r.regions[k].catch {
			return r.regions[k].try
		}
	}
	return -1
}

// tryOf は、この行から見える、受けた例外 name の、例外を受ける文の番号（囲んでいる catch のブロックの、いちばん内側）。なければ -1。
func (r row) tryOf(name string, tries []tryInfo) int {
	for k := len(r.regions) - 1; k >= 0; k-- {
		if r.regions[k].catch && tries[r.regions[k].try].name == name {
			return r.regions[k].try
		}
	}
	return -1
}

// rowsOf は配線の本体を、式を持つ文ごとの呼び出しに分ける（docs/check.md「手順 0」の「文 i の呼び出し」）。
// 並びは、文を辿る順（walkStmts）。空の val、disjoint、for は、行にならない（中の文が、行になる）。
func rowsOf(w *ir.Wiring) []row {
	rows, _ := rowsAndTries(w)
	return rows
}

// throwRef は、throw（アームの本体か、catch のブロックの最後の文）を、呼び出しとして数えたもの。例外を持たない throw
// （解決できなかったもの）は ok が false。
func throwRef(i, arm int, body ir.ThrowBody, stmt ir.Span) (callRef, bool) {
	cr := callRef{row: i, arm: arm, call: ir.Call{Args: body.Args, Span: body.Span}, stmt: stmt}
	switch {
	case body.Caught != "":
		cr.rethrow = body.Caught
	case body.Exception != "":
		cr.throw = body.Exception
	default:
		return cr, false
	}
	return cr, true
}

// rowsAndTries は、rowsOf に、辿った、例外を受ける文を足したもの。catch のブロックの最後の throw の文も、行になる。
func rowsAndTries(w *ir.Wiring) ([]row, []tryInfo) {
	if w == nil {
		return nil, nil
	}
	rows := make([]row, 0, len(w.Statements))
	tries := walkAll(w.Statements, func(st ir.Statement, loops []int, regions []region) {
		if ts, isThrow := st.(ir.ThrowStmt); isThrow {
			r := row{loops: loops, regions: regions}
			if cr, ok := throwRef(len(rows), -1, ts.Throw, ts.Span); ok {
				r.calls = append(r.calls, cr)
			}
			rows = append(rows, r)
			return
		}
		expr, span, ok := stmtExpr(st)
		if !ok {
			return
		}
		i := len(rows)
		r := row{loops: loops, regions: regions}
		switch e := expr.(type) {
		case ir.CallExpr:
			r.calls = append(r.calls, callRef{row: i, arm: -1, call: e.Call, stmt: span})
		case ir.MatchExpr:
			r.isMatch = true
			if cs, isCaught := e.Match.Subject.(ir.CaughtSubject); isCaught {
				r.caught, r.classify, r.span = cs.Name, e.Match.Classify, e.Match.Span
			}
			for j, arm := range e.Match.Arms {
				r.armBodies = append(r.armBodies, arm.Body)
				switch body := arm.Body.(type) {
				case ir.CallExpr:
					r.calls = append(r.calls, callRef{row: i, arm: j, call: body.Call, stmt: span})
				case ir.ThrowBody:
					if cr, ok := throwRef(i, j, body, span); ok {
						if cr.rethrow != "" && r.caught == cr.rethrow {
							cr.classify, cr.cases = r.classify, arm.Cases
						}
						r.calls = append(r.calls, cr)
					}
				}
			}
		}
		rows = append(rows, r)
	})
	return rows, tries
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
	mapStmts(w.Statements, func(st ir.Statement) ir.Statement {
		expr, _, ok := stmtExpr(st)
		if !ok {
			return st
		}
		return withExpr(st, mapExpr(expr))
	})
}

// source は例外の出どころ（行 i, 呼び出し c）。
type exSource struct {
	ref callRef
	sig *sigState // Sc
}

// bodyInfo は手順 0 で集めた配線の情報（行と、その Sc、本体から出る例外と、その出どころ）。
type bodyInfo struct {
	rows    []row
	sigs    [][]*sigState         // rows と同じ並びの Sc
	thrown  []string              // ∪ Sc.exceptions（出現順）
	sources map[string][]exSource // 例外 → 出どころ
	// tries は、本体の、例外を受ける文。caught は、その文ごとの、受ける例外の集まり（docs/check.md「Sig(O, v)」の「例外を受ける文」）。
	tries  []tryInfo
	caught [][]string
	// unclassified は、受けた例外の match で、分類のどのケースにも当たらない例外があったもの（MATCH-10）。
	unclassified []unclassified
}

// unclassified は、MATCH-10 の一件: 受けた例外の match（span）と、その分類、当たらない例外。
type unclassified struct {
	span       ir.Span
	classify   string
	exceptions []string
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
	// proceed は、今、計算しているデコレータの本体の 包んだ関数の呼び出しの Sc（包んだ op の、そこまでの Sig）。本体の外では nil。
	proceed *sigState
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
		if o == nil {
			return nil
		}
		bodies := []*ir.Wiring{o.Body}
		// 本体のあるデコレータの呼び出しは、付けた op の呼び出しとして数える（docs/check.md「デコレータの本体」）
		for _, d := range ix.bodiedDecorators(o) {
			bodies = append(bodies, d.Body)
		}
		for _, body := range bodies {
			for _, r := range rowsOf(body) {
				for _, cr := range r.calls {
					if name := cr.call.CalledOp(); ix.op(name) != nil {
						out = appendUnique(out, name)
					}
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
	c.addCarriedThrows(o, s)
}

// wiringStep0 は手順 0（配線の op）。本体の行を順に見て E / F / rel / succ を計算し、その後ろに、運ばれた効果を足す
// （addCarried）。
// inCases は、例外 x が、例外の分類 classify で当たるケースが、cases にあるか。
func (c *computer) inCases(classify string, cases []string, x string) bool {
	cl, ok := c.ix.decls[classify].(*ir.Classify)
	if !ok {
		return false
	}
	name, hit := cl.CaseOfException(x)
	return hit && contains(cases, name)
}

// reportUnclassified は MATCH-10（docs/check.md「分類」の「例外の分類」）: 受けた例外の match で、try のブロックが投げうる例外に、
// 分類のどのケースにも当たらないものがある。app が nil でなければ、デコレータの本体の try で、位置は、その適用。
func (c *computer) reportUnclassified(info *bodyInfo, o *ir.Op, app *ir.Application) {
	for _, u := range info.unclassified {
		data := map[string]any{"unclassified": u.exceptions}
		if app != nil {
			c.r.report(diag.Error, "MATCH-10", app.Span, sprintf("@%s の本体の、受けた例外の match で、op %s が投げうる例外 %s が、classify %s のどのケースにも当たらない", shortName(app.Decorator), o.Name, joinNames(u.exceptions), u.classify),
				[]related{{span: u.span, msg: "受けた例外の match"}}, data)
			continue
		}
		c.r.report(diag.Error, "MATCH-10", u.span, sprintf("try のブロックが投げうる例外 %s が、classify %s のどのケースにも当たらない（ケースに足すか、残りのケース any exception as else を足す）", joinNames(u.exceptions), u.classify), nil, data)
	}
}

func (c *computer) wiringStep0(o *ir.Op, ns string, s *sigState, emit bool) {
	info := c.blockStep(o.Body, ns, s, declared(o), o.Throws)
	if emit {
		c.reportUnclassified(info, o, nil)
	}
	c.addWritten(o, s)
	c.addCarried(o, s)
	c.bodies[o.Name] = info
	if emit {
		checkRel4(o, s, c.r)
	}
}

// blockStep は、文の並び一つ（op の配線の本体か、デコレータの本体）の、手順 0 の計算: 行を順に見て、E / F / rel / succ を
// s に入れる。written は、節に書いた例外（Declared）、throws は、その節（出どころのない例外の rel を決める）。
// デコレータの本体では、包んだ関数の呼び出しの Sc が、c.proceed（包んだ op の、そこまでの Sig）。
func (c *computer) blockStep(body *ir.Wiring, ns string, s *sigState, written []string, throws []ir.ThrowsClause) *bodyInfo {
	info := &bodyInfo{sources: map[string][]exSource{}}
	info.rows, info.tries = rowsAndTries(body)
	info.caught = make([][]string, len(info.tries))
	info.sigs = make([][]*sigState, len(info.rows))
	// 行の順に、Sc を決める。try のブロックの中の例外は、その文が受ける（受ける例外の集まり）。throw e の Sc は、受けた例外
	// （受けた例外の match のアームなら、そのアームが受けるケースに当たるものだけ）。try のブロックの行は、catch のブロックの行より
	// 先に来るので、throw e を見るときには、集まりは、もう、決まっている。
	for i, r := range info.rows {
		for _, cr := range r.calls {
			var sc *sigState
			if cr.rethrow != "" {
				sc = newSigState()
				if t := r.tryOf(cr.rethrow, info.tries); t >= 0 {
					for _, x := range info.caught[t] {
						if cr.classify == "" || c.inCases(cr.classify, cr.cases, x) {
							sc.addE(x)
						}
					}
				}
			} else {
				sc = c.callSig(cr, ns)
			}
			info.sigs[i] = append(info.sigs[i], sc)
			if t := r.catcher(); t >= 0 && sc != nil {
				for _, x := range sc.E {
					info.caught[t] = appendUnique(info.caught[t], x)
				}
			}
		}
		if r.caught != "" {
			// 受けた例外の match の漏れ（MATCH-10）
			if cl, ok := c.ix.decls[r.classify].(*ir.Classify); ok && cl.IsExceptionClass() {
				if t := r.tryOf(r.caught, info.tries); t >= 0 {
					var missing []string
					for _, x := range info.caught[t] {
						if _, hit := cl.CaseOfException(x); !hit {
							missing = append(missing, x)
						}
					}
					if len(missing) > 0 {
						info.unclassified = append(info.unclassified, unclassified{span: r.span, classify: r.classify, exceptions: missing})
					}
				}
			}
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
		if r.catcher() >= 0 {
			continue // try のブロックの中: 出どころにならない
		}
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
	for _, x := range written {
		s.addE(x)
	}
	// 文 j が、e を持つ呼び出しを持つか
	rowHas := func(e ir.EffectRef, j int) bool {
		for _, sc := range info.sigs[j] {
			if sc != nil && sc.hasF(e) {
				return true
			}
		}
		return false
	}
	// 出どころ（文 i）での e の状態の、1・3・4（docs/check.md「Sig(O, v)」の手順 0）。
	// 1: e を持つ呼び出しが、文 j < i にあり、それを囲む for が、どれも、文 i も囲んでいる → kept（必ず、先に動いている）
	// 3: e を持つ呼び出しが、文 j < i にあり、それを囲む for に、文 i を囲まないものがある → unknown（0 回かもしれない）
	// 4: e を持つ呼び出しと、文 i を、ともに囲む for がある → unknown（前の回で、起きたかもしれない）
	// tryRel は、文 j の呼び出しを、文 i から見たときの、例外を受ける文による扱い（同「例外を受ける文」の表）:
	// ranSurely（囲まれていないのと同じ）、ranMaybe（3 と同じ）、ranNever（ないものとする）。
	const (
		ranSurely = iota
		ranMaybe
		ranNever
	)
	tryRel := func(j, i int) int {
		res := ranSurely
		for _, rg := range info.rows[j].regions {
			same, sameTry := false, false
			for _, ri := range info.rows[i].regions {
				if ri == rg {
					same = true
				}
				if ri.try == rg.try {
					sameTry = true
				}
			}
			switch {
			case same:
				// 同じ文の、同じブロック
			case sameTry:
				res = ranMaybe // try のブロックと、catch のブロック
			case info.tries[rg.try].never && rg.catch:
				return ranNever // catch が動けば、文 i には、来ない
			case info.tries[rg.try].never:
				// try のブロックは、終わりまで動いている
			default:
				res = ranMaybe
			}
		}
		return res
	}
	before := func(e ir.EffectRef, i int) bool {
		for j := 0; j < i; j++ {
			if rowHas(e, j) && subset(info.rows[j].loops, info.rows[i].loops) && tryRel(j, i) == ranSurely {
				return true
			}
		}
		return false
	}
	maybe := func(e ir.EffectRef, i int) bool {
		for j := range info.rows {
			if !rowHas(e, j) {
				continue
			}
			tr := tryRel(j, i)
			if tr == ranNever {
				continue
			}
			if j < i && (tr == ranMaybe || !subset(info.rows[j].loops, info.rows[i].loops)) {
				return true
			}
			if shares(info.rows[j].loops, info.rows[i].loops) {
				return true
			}
		}
		return false
	}
	// rel
	for _, x := range s.E {
		srcs := info.sources[x]
		for _, e := range s.F {
			if len(srcs) == 0 {
				s.setRel(x, e, relationState(throws, x, identity))
				continue
			}
			var state ir.EffectState
			for k, src := range srcs {
				var st ir.EffectState
				switch {
				case before(e, src.ref.row):
					st = ir.Kept
				case src.sig.hasF(e) && src.sig.relOf(x, e) == ir.Kept:
					st = ir.Kept
				case maybe(e, src.ref.row):
					st = ir.Unknown
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
			// 例外を受ける文の中の呼び出し: 「返らない」catch のブロックの中なら prevented（成功したなら、動いていない）。
			// 「返らない」でない catch を持つ文の、try か catch のブロックの中なら unknown
			inFailing, inUnsure := false, false
			for _, rg := range r.regions {
				switch {
				case rg.catch && info.tries[rg.try].never:
					inFailing = true
				case !info.tries[rg.try].never:
					inUnsure = true
				}
			}
			for j, sc := range info.sigs[i] {
				if sc == nil || !sc.hasF(e) || c.neverReturns(r.calls[j]) {
					continue
				}
				inArms++
				if inFailing {
					prevented++
					continue
				}
				if inUnsure {
					other++
					continue
				}
				if len(r.loops) > 0 {
					// for の中の呼び出しは、0 回かもしれない
					other++
					continue
				}
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
	return info
}

// addWritten は手順 0（配線の op）の「節に書いた効果」（docs/check.md「Sig(O, v)」）: Op.effects の効果 e のうち、本体が
// 行き着いていないものを、F の末尾に足す。rel と succ は、葉の op と同じ値（rel は Op.throws の relations から、succ は kept）:
// 本体は e を行わないので、行の順からは決まらない。本体が、もう行き着いている e は、何も変えない（節に書くのは、その関数が、
// 自分で足す分。同「状態に触れるもの」の「節に書く要素」）。
func (c *computer) addWritten(o *ir.Op, s *sigState) {
	for _, e := range o.Effects {
		if s.hasF(e) {
			continue
		}
		s.addF(e)
		for _, x := range s.E {
			s.setRel(x, e, relationState(o.Throws, x, identity))
		}
		s.succ[e] = ir.Kept
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
	c.addCarriedThrows(o, s)
}

// effectThrows は、効果 e が投げうる例外（Effect.throws）。直に書いた効果（状態.操作）と、引けない効果には、ない。
func (c *computer) effectThrows(e ir.EffectRef) []string {
	named, ok := e.(ir.NamedEffect)
	if !ok {
		return nil
	}
	if decl := c.ix.effect(named.Effect); decl != nil {
		return decl.Throws
	}
	return nil
}

// addThrown は、効果 thrower が投げる例外 x を、E に足す（docs/check.md「Sig(O, v)」の「運ばれた効果が投げる例外」と、
// halts の項）。この出どころでの状態は、thrower が prevented（own が true のとき。投げたとき、その効果は、起きていない）、
// ほかの効果は unknown（デコレータの処理が、本体のどの行の前後で起きるかを、言語は決めない）。x が、ここで初めて E に
// 入るなら、その状態。もう E にあれば、同じならその状態、違えば unknown。
func (s *sigState) addThrown(x string, thrower ir.EffectRef, own bool) {
	existed := s.hasE(x)
	s.addE(x)
	for _, e := range s.F {
		st := ir.Unknown
		if own && e == thrower {
			st = ir.Prevented
		}
		if existed && s.relOf(x, e) != st {
			st = ir.Unknown
		}
		s.setRel(x, e, st)
	}
}

// addCarriedThrows は、運ばれた効果（Op.carried.effects）が投げる例外を、E と rel に足す（葉でも、配線でも）。
// op が throws に書いていなくてよい（デコレータの throws と同じ）。
func (c *computer) addCarriedThrows(o *ir.Op, s *sigState) {
	for _, e := range o.Carried.Effects {
		for _, x := range c.effectThrows(e) {
			s.addThrown(x, e, true)
		}
	}
}

// callSig は手順 0 の表の Sc: 配線の呼び出し一つの、例外・効果・rel・succ（docs/check.md「Sig(O, v)」）。
// op の呼び出しは Sig(callee, ns)。効果の呼び出しは、効果がその効果一つで、succ は kept。例外は、宣言した効果の throws
// （直に呼んだ組み込みの効果には、ない）で、rel は、どれも prevented。read の呼び出しは何も持たない。アームの本体の throw X は、例外が X だけ。呼び出し先が引けなければ nil。
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
			// 宣言した効果が投げる例外（Effect.throws）: 投げたとき、その効果は、起きていない
			for _, x := range c.effectThrows(callee.Effect) {
				sc.addE(x)
				sc.setRel(x, callee.Effect, ir.Prevented)
			}
			return sc
		}
	case ir.ReadCallee:
		return newSigState()
	case ir.ProceedCallee:
		// デコレータの本体の 包んだ関数の呼び出し: 包んだ op の、そこまでの Sig（docs/check.md「デコレータの本体」）
		return c.proceed
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
		// applyThrows は、throws の節（か、outer throws の節）を、手順 i のとおりに適用する。
		applyThrows := func(clauses []ir.ThrowsClause) {
			for _, cl := range clauses {
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
		if d.Body != nil {
			// 本体のあるデコレータ: 付けた所で、広げたものとして計算する（docs/check.md「デコレータの本体」）。
			// 本体を、配線として手順 0 と同じに計算し、包んだ関数の呼び出しの Sc を、ここまでの Sig にする。
			inner, outer := s.clone(), c.proceed
			*s = *newSigState()
			c.proceed = inner
			var written []string
			for _, cl := range d.Throws {
				for _, raw := range cl.Exceptions {
					written = append(written, subst(raw)...)
				}
			}
			info := c.blockStep(d.Body, ns, s, written, d.Throws)
			c.proceed = outer
			if emit {
				c.reportUnclassified(info, o, app)
			}
			if viewer != ns {
				applyThrows(d.OuterThrows) // DEC-10
			}
			continue
		}
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
							added := !s.hasF(h)
							if added {
								s.addF(h)
								for _, y := range s.E {
									s.setRel(y, h, ir.Prevented)
								}
								s.succ[h] = ir.Prevented
							}
							// h が投げる例外は、E に加わる。h を、ここで足したなら、h は prevented。それ以外は、どの効果も unknown
							for _, z := range c.effectThrows(h) {
								s.addThrown(z, h, added)
							}
						}
					}
				}
			}
		}
		// throws の節、次に outer throws の節。DEC-10: outer throws の例外（fn の外で起きる）は、境界の外から見たときだけ
		applyThrows(d.Throws)
		if viewer != ns {
			applyThrows(d.OuterThrows)
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

// decoratorSets は O に付いたデコレータの ∪Catches(dᵢ)、∪Throws(dᵢ, ns(O))（throws の節）、
// outer throws の節の例外（docs/check.md「名前空間」の DOC-1 が使う）。`$param:` は置き換え済み。
func (c *computer) decoratorSets(o *ir.Op) (catches, throws, outerThrows []string) {
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
					throws = appendUnique(throws, x)
				}
			}
		}
		for _, cl := range d.OuterThrows {
			for _, raw := range cl.Exceptions {
				for _, x := range subst(raw) {
					outerThrows = appendUnique(outerThrows, x)
				}
			}
		}
	}
	return catches, throws, outerThrows
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
	if cr.throw != "" || cr.rethrow != "" {
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
// 呼んだ op の R）、その後ろに、節に書いた状態（Op.reads）、運ばれた状態を並べる。
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
	add(c.stmtReads(o.Body.Statements)...)
	for _, d := range c.ix.bodiedDecorators(o) {
		add(c.stmtReads(d.Body.Statements)...) // 本体のあるデコレータが読む状態（docs/check.md「デコレータの本体」）
	}
	add(c.ix.readStates(o.Reads)...) // 節に書いた状態は、計算した分に足す（docs/check.md「reads」）
	add(c.ix.readStates(o.Carried.Reads)...)
	c.reads[o.Name] = out
	return out
}

// stmtReads は、文の並びが読む状態（docs/check.md「reads」の 1、2。出現順。重複は最初の出現に寄せる）。
// 配線の本体の全体にも、disjoint のアームや、for のブロックにも、使う。
func (c *computer) stmtReads(stmts []ir.Statement) []string {
	var out []string
	add := func(states ...string) {
		for _, s := range states {
			if s != "" {
				out = appendUnique(out, s)
			}
		}
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
	walkStmts(stmts, func(st ir.Statement, _ []int) {
		if f, ok := st.(ir.ForStmt); ok {
			// for の頭の並びが参照する状態
			add(atomStates(f.Of)...)
			return
		}
		if t, ok := st.(ir.ThrowStmt); ok {
			// catch のブロックの最後の throw の引数の Atom が参照する状態
			for _, a := range t.Throw.Args {
				add(atomStates(a.Value)...)
			}
			return
		}
		if e, _, ok := stmtExpr(st); ok {
			expr(e)
		}
	})
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
