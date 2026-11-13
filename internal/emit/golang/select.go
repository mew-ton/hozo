package golang

import "github.com/mew-ton/hozo/ir"

// selection は出力の対象（docs/emit-go.md「出力の対象」）のうち、型より前に決まるもの: op、状態、値の分類、
// 別の名前空間の op、包む関数。型は decls.go が、ここで決まったものから辿って集める。
type selection struct {
	// ops は選ばれた viewer の op。
	ops map[string]bool
	// slot は、選ばれたが本体を辿らない op（select の stops と wraps の op、単位の対象でない op）。配線でも葉と同じ扱いで、
	// 実装を差し込む口だけを出す。
	slot map[string]bool
	// states は選ばれた状態: 基底の状態の FQN と、状態の分類の FQN。
	states map[string]bool
	// classifies は選ばれた値の分類。
	classifies map[string]bool
	// deps は選ばれた配線が呼ぶ、別の名前空間の op。
	deps map[string]bool
	// wraps は op の FQN → select の wraps が指したデコレータ（FQN。Application の decorator か via）。
	wraps map[string]map[string]bool
}

// empty は何も選ばれていないか。
func (s *selection) empty() bool {
	return len(s.ops) == 0 && len(s.states) == 0
}

// computeSelection は opts.Select から出力の対象を決める（docs/emit-go.md「出力の対象」の 1〜5）。
// select が指す FQN の確かめ（PLG-3）は呼ぶ側（internal/plugin.CheckSelect）が済ませている前提で、
// viewer の op / 状態でないものと、単位の対象でないもの（in の外で宣言されたもの）は黙って飛ばす。
// 単位の対象でない viewer の op は、起点にはならないが、選ばれた配線から辿られたら口だけを出す（stops と同じ扱い）。
func (e *emitter) computeSelection() *selection {
	s := &selection{
		ops: map[string]bool{}, slot: map[string]bool{}, states: map[string]bool{},
		classifies: map[string]bool{}, deps: map[string]bool{}, wraps: map[string]map[string]bool{},
	}
	own := func(fqn string) ir.Decl {
		if e.declNS[fqn] != e.viewer {
			return nil
		}
		return e.lookup(fqn)
	}
	stops := map[string]bool{}
	for _, fqn := range e.opts.Select.Stops {
		stops[fqn] = true
	}

	// 1. 起点: roots の op、wraps の op、states の状態。どれも単位の対象の宣言。
	var work []string
	for _, fqn := range e.opts.Select.Roots {
		if e.inUnit[fqn] {
			work = append(work, fqn)
		}
	}
	for _, w := range e.opts.Select.Wraps {
		op, ok := own(w.Op).(*ir.Op)
		if !ok || !e.inUnit[w.Op] || !hasDecorator(op, w.Decorator) {
			continue
		}
		if s.wraps[w.Op] == nil {
			s.wraps[w.Op] = map[string]bool{}
		}
		s.wraps[w.Op][w.Decorator] = true
		work = append(work, w.Op)
	}
	for _, fqn := range e.opts.Select.States {
		if e.inUnit[fqn] {
			s.selectState(e, fqn)
		}
	}

	// 2. op を辿る。stops と wraps の op、単位の対象でない op（in の外）は選ぶが、本体は辿らない。
	for len(work) > 0 {
		fqn := work[0]
		work = work[1:]
		op, ok := own(fqn).(*ir.Op)
		if !ok || s.ops[fqn] {
			continue
		}
		s.ops[fqn] = true
		if stops[fqn] || s.wraps[fqn] != nil || !e.inUnit[fqn] {
			s.slot[fqn] = true
		}
		if op.Body == nil || s.slot[fqn] {
			// 3. 葉の op（と、口だけを出す op）が直に持つ効果の宛先の状態。配線の op が持つ伝播した効果は数えない。
			if op.Body == nil {
				if target := e.effectTargetOf(op); target != "" {
					s.selectState(e, target)
				}
			}
			continue
		}
		forEachCall(op.Body, func(c *ir.Call) {
			if e.op(c.Op) == nil {
				return
			}
			if e.declNS[c.Op] == e.viewer {
				work = append(work, c.Op)
			} else {
				s.deps[c.Op] = true // 5. 名前空間をまたぐ呼び出しは辿らない
			}
			// 3. 選ばれた呼び出しの Call.bound の状態
			for _, b := range c.Bound {
				s.selectState(e, b.State)
			}
		})
		forEachMatch(op.Body, func(m *ir.Match) {
			switch subject := m.Subject.(type) {
			case ir.StateSubject:
				s.selectState(e, subject.State) // 3. 配線が match で読む状態
			case ir.ClassifyOwnedSubject:
				s.selectState(e, m.Classify)
			case ir.ValueSubject:
				// 4. 選ばれた配線の match が使う値の分類
				if c, ok := own(m.Classify).(*ir.Classify); ok && c.SubjectType() != nil {
					s.classifies[m.Classify] = true
				}
			}
		})
	}
	return s
}

// selectState は viewer の基底の状態か状態の分類を選ぶ。それ以外の FQN は何もしない。
// 状態の分類を選んでも、その対象の基底の状態は選ばない（読むのは分類の述語）。
func (s *selection) selectState(e *emitter, fqn string) {
	if e.declNS[fqn] != e.viewer {
		return
	}
	switch d := e.lookup(fqn).(type) {
	case *ir.State:
		s.states[fqn] = true
	case *ir.Classify:
		if _, ok := d.SubjectStates(); ok {
			s.states[fqn] = true
		}
	}
}

// hasDecorator は op にデコレータ fqn の適用があるか（Application の decorator か via）。
func hasDecorator(op *ir.Op, fqn string) bool {
	for _, app := range op.Decorators {
		if fqn != "" && (app.Decorator == fqn || app.Via == fqn) {
			return true
		}
	}
	return false
}

// effectOf は葉の op が直に持つ効果（docs/check.md EFF-3: 高々一つ）の宣言。持たなければ nil。
func (e *emitter) effectOf(op *ir.Op) *ir.Effect {
	if op.Body != nil || len(op.Effects) == 0 {
		return nil
	}
	eff, _ := e.lookup(op.Effects[0]).(*ir.Effect)
	return eff
}

// effectTargetOf は葉の op が直に持つ効果の宛先の状態（docs/emit-go.md「state」の「命令」）: 組み込みの効果なら
// その基底の状態、遷移なら of が指す状態の分類か基底の状態。効果を持たなければ ""。
func (e *emitter) effectTargetOf(op *ir.Op) string {
	eff := e.effectOf(op)
	if eff == nil {
		return ""
	}
	switch t := eff.Target.(type) {
	case ir.BuiltinTarget:
		return t.State
	case ir.TransitionTarget:
		return t.Of
	}
	return ""
}

// forEachCall は配線のすべての呼び出し（文、式、match のアーム）を行の順に f に渡す。
func forEachCall(w *ir.Wiring, f func(*ir.Call)) {
	if w == nil {
		return
	}
	inMatch := func(m *ir.Match) {
		for i := range m.Arms {
			if c, ok := m.Arms[i].Body.(ir.CallExpr); ok {
				f(&c.Call)
			}
		}
	}
	inExpr := func(x ir.Expr) {
		switch y := x.(type) {
		case ir.CallExpr:
			f(&y.Call)
		case ir.MatchExpr:
			inMatch(&y.Match)
		}
	}
	for _, s := range w.Statements {
		switch x := s.(type) {
		case ir.ConstStmt:
			inExpr(x.Expr)
		case ir.ReturnStmt:
			inExpr(x.Expr)
		case ir.CallStmt:
			f(&x.Call)
		case ir.MatchStmt:
			inMatch(&x.Match)
		}
	}
}

// forEachMatch は配線のすべての match（文と式）を行の順に f に渡す。
func forEachMatch(w *ir.Wiring, f func(*ir.Match)) {
	if w == nil {
		return
	}
	inExpr := func(x ir.Expr) {
		if y, ok := x.(ir.MatchExpr); ok {
			f(&y.Match)
		}
	}
	for _, s := range w.Statements {
		switch x := s.(type) {
		case ir.ConstStmt:
			inExpr(x.Expr)
		case ir.ReturnStmt:
			inExpr(x.Expr)
		case ir.MatchStmt:
			f(&x.Match)
		}
	}
}
