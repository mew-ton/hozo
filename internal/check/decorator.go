package check

import (
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// checkDecorators は DEC-4 / DEC-6（docs/check.md「デコレータ」）。
// DEC-5 / DEC-16 は計算の手順 i で出る（checkDec5 / checkDec16）。DEC-10 は計算（手順 i の outer throws）で、診断なし。
func checkDecorators(ix *index, c *computer, r *reporter) {
	for i := range ix.p.Namespaces {
		for _, d := range ix.p.Namespaces[i].Decls {
			dec, ok := d.(*ir.Decorator)
			if !ok || dec.Alias != nil {
				continue
			}
			checkDec6(dec, r)
		}
	}
	for i, o := range ix.ops {
		checkDec4(ix, o, r)
		checkDec24(ix, o, ix.opsNS[i], r)
		checkDec25(ix, o, r)
		checkDec27(ix, o, r)
	}
}

// checkDec27 は DEC-27（docs/check.md「デコレータの本体」）: 包んだ関数の呼び出し を try のブロックに持ち、catch のブロックが「返らない」でない
// デコレータを、returns が null でない op に付けた（DEC-4 と同じ理由: 受けた後に、返す値がない）。位置は、適用。
func checkDec27(ix *index, o *ir.Op, r *reporter) {
	if o.Returns == nil {
		return
	}
	for ai := range o.Decorators {
		app := &o.Decorators[ai]
		d, _ := ix.decorator(app.Decorator)
		if d == nil || d.Body == nil {
			continue
		}
		reported := false
		ir.WalkStatements(d.Body.Statements, func(st ir.Statement) {
			try, ok := st.(ir.TryStmt)
			if !ok || reported || try.Catch.Never || len(try.Body) == 0 {
				return
			}
			if cs, isCall := try.Body[0].(ir.CallStmt); isCall && cs.Call.IsProceed() {
				reported = true
				r.report(diag.Error, "DEC-27", app.Span, sprintf("デコレータ %s の本体は、包んだ関数の呼び出しの例外を受けて、投げずに終わる道を持つので、値を返す op %s には付けられない（受けた後に、返す値がない）", d.Name, o.Name),
					[]related{{span: try.Catch.Span, msg: "投げずに終わりうる catch"}}, nil)
			}
		})
	}
}

// checkDec25 は DEC-25（docs/check.md「デコレータ」）: 本体のあるデコレータを、never を返す op に付けていて、本体の 包んだ関数の呼び出し
// （か、包んだ関数の呼び出し のアームを持つ match）の後ろに、文がある。包んだ op は、いつも、例外を投げるので、後ろの文は、評価されない
// （WIRE-9 と、同じ理由）。位置は、適用。後ろの、最初の文を Related に示す。
func checkDec25(ix *index, o *ir.Op, r *reporter) {
	if !ir.IsNever(o.Returns) {
		return
	}
	for ai := range o.Decorators {
		app := &o.Decorators[ai]
		d, _ := ix.decorator(app.Decorator)
		if d == nil || d.Body == nil {
			continue
		}
		if call, next, ok := proceedTail(d.Body.Statements); ok {
			r.report(diag.Error, "DEC-25", app.Span, sprintf("デコレータ %s の本体は、包んだ関数の呼び出しの後ろに、文を持つので、never を返す op %s には付けられない（op は、いつも、例外を投げるので、後ろの文が、評価されない）", d.Name, o.Name),
				[]related{{span: call, msg: "包んだ関数の呼び出し"}, {span: next, msg: "評価されない文"}}, nil)
		}
	}
}

// proceedTail は、文の並びの、包んだ関数の呼び出し（か、そのアームを持つ match）の後ろの、最初の文。呼び出しを中に持つ文
// （try、for、disjoint）は、中の並びを先に見て、中に後ろの文がなければ、その文の下の文を、後ろの文とする。
// call は、包んだ関数の呼び出しを持つ文の位置。後ろの文がなければ ok は偽。
func proceedTail(stmts []ir.Statement) (call, next ir.Span, ok bool) {
	for i, st := range stmts {
		var found bool
		switch s := st.(type) {
		case ir.TryStmt:
			if call, next, ok = proceedTail(s.Body); ok {
				return call, next, true
			}
			found = hasProceed(s.Body)
			call = s.Span
		case ir.ForStmt:
			if call, next, ok = proceedTail(s.Body); ok {
				return call, next, true
			}
			found = hasProceed(s.Body)
			call = s.Span
		case ir.DisjointStmt:
			for _, arm := range s.Arms {
				if call, next, ok = proceedTail(arm.Statements); ok {
					return call, next, true
				}
				found = found || hasProceed(arm.Statements)
			}
			call = s.Span
		default:
			found = proceeds(st)
			_, call, _ = stmtExpr(st)
		}
		if !found {
			continue
		}
		if i < len(stmts)-1 {
			return call, stmtSpan(stmts[i+1]), true
		}
		return ir.Span{}, ir.Span{}, false
	}
	return ir.Span{}, ir.Span{}, false
}

// proceeds は、文が、包んだ関数の呼び出しか、そのアームを持つ match か。
func proceeds(st ir.Statement) bool {
	expr, _, ok := stmtExpr(st)
	if !ok {
		return false
	}
	switch e := expr.(type) {
	case ir.CallExpr:
		return e.Call.IsProceed()
	case ir.MatchExpr:
		for _, arm := range e.Match.Arms {
			if c, isCall := arm.Body.(ir.CallExpr); isCall && c.Call.IsProceed() {
				return true
			}
		}
	}
	return false
}

// hasProceed は、文の並び（中のブロックを含む）に、包んだ関数の呼び出しがあるか。
func hasProceed(stmts []ir.Statement) bool {
	found := false
	ir.WalkStatements(stmts, func(st ir.Statement) {
		found = found || proceeds(st)
	})
	return found
}

// stmtSpan は、文の位置。
func stmtSpan(st ir.Statement) ir.Span {
	switch s := st.(type) {
	case ir.TryStmt:
		return s.Span
	case ir.ForStmt:
		return s.Span
	case ir.DisjointStmt:
		return s.Span
	case ir.ThrowStmt:
		return s.Span
	}
	_, span, _ := stmtExpr(st)
	return span
}

// checkDec24 は DEC-24（docs/check.md「デコレータ」）: 本体のあるデコレータを、宣言した名前空間のほかの op に付けていて、
// 本体が、その op の名前空間の配線からは呼べないものに触れている: export のない op、read、効果、状態。
// 本体は、付けた所で広げたものとして扱う（同「デコレータの本体」）ので、呼べるのは、付けた op の配線が呼べるものだけ。
// 位置は、適用。本体の、当たった呼び出しを Related に示す。
func checkDec24(ix *index, o *ir.Op, ns string, r *reporter) {
	for ai := range o.Decorators {
		app := &o.Decorators[ai]
		d, _ := ix.decorator(app.Decorator)
		if d == nil || d.Body == nil || ix.namespaceOf(d.Name) == ns {
			continue
		}
		var rel []related
		var whats []string
		hit := func(what string, span ir.Span) {
			whats = appendUnique(whats, what)
			rel = append(rel, related{span: span, msg: what})
		}
		for _, row := range rowsOf(d.Body) {
			for _, cr := range row.calls {
				switch callee := cr.call.Callee.(type) {
				case ir.OpCallee:
					if op := ix.op(callee.Op); op != nil && !op.Exported && ix.namespaceOf(op.Name) != ns {
						hit("export のない op "+op.Name, cr.call.Span)
					}
				case ir.ReadCallee:
					if ix.namespaceOf(callee.Read) != ns {
						hit("read "+callee.Read, cr.call.Span)
					}
				case ir.EffectCallee:
					if callee.Effect != nil {
						hit("効果 "+callee.Effect.String(), cr.call.Span)
					}
				}
			}
		}
		ir.WalkStatements(d.Body.Statements, func(st ir.Statement) {
			// 状態を、値として読む（呼び出しの引数、throw の引数、val に入れる式、アームの本体の値。レコードリテラルの中も）
			for _, at := range stmtAtoms(st) {
				for _, state := range atomStates(at.atom) {
					hit("状態 "+state, at.span)
				}
			}
			expr, ok := ir.StatementExpr(st)
			if !ok {
				return
			}
			if m, ok := expr.(ir.MatchExpr); ok {
				if state, ok := m.Match.SubjectState(); ok {
					hit("状態 "+state, m.Match.Span)
				} else if _, owned := m.Match.Subject.(ir.ClassifyOwnedSubject); owned {
					hit("状態の分類 "+m.Match.Classify, m.Match.Span)
				}
			}
		})
		if len(whats) == 0 {
			continue
		}
		r.report(diag.Error, "DEC-24", app.Span, sprintf("デコレータ %s の本体は、名前空間 %s の配線からは呼べないものに触れているので、op %s には付けられない: %s。別の名前空間の op に付けるデコレータの本体が呼べるのは、export の op だけ", d.Name, ns, o.Name, joinNames(whats)), rel, nil)
	}
}

// spannedAtom は、文に書いた Atom と、その位置（呼び出しなら、その呼び出し。ほかは、文）。
type spannedAtom struct {
	atom ir.Atom
	span ir.Span
}

// stmtAtoms は、文そのものに書いた Atom（中のブロックの文は、含まない）: 呼び出しと throw の引数、式の Atom、match のアームの
// 本体の値と、呼び出しと throw の引数。match の対象は、含まない（状態の対象は、呼び元が、別に見る）。
func stmtAtoms(st ir.Statement) []spannedAtom {
	var out []spannedAtom
	call := func(c ir.Call) {
		for _, a := range c.Args {
			out = append(out, spannedAtom{a.Value, c.Span})
		}
	}
	if ts, ok := st.(ir.ThrowStmt); ok {
		for _, a := range ts.Throw.Args {
			out = append(out, spannedAtom{a.Value, ts.Span})
		}
		return out
	}
	expr, span, ok := stmtExpr(st)
	if !ok {
		return nil
	}
	switch e := expr.(type) {
	case ir.CallExpr:
		call(e.Call)
	case ir.AtomExpr:
		out = append(out, spannedAtom{e.Atom, span})
	case ir.MatchExpr:
		for _, arm := range e.Match.Arms {
			switch b := arm.Body.(type) {
			case ir.CallExpr:
				call(b.Call)
			case ir.AtomExpr:
				out = append(out, spannedAtom{b.Atom, span})
			case ir.ThrowBody:
				for _, a := range b.Args {
					out = append(out, spannedAtom{a.Value, b.Span})
				}
			}
		}
	}
	return out
}

// checkDec4 は DEC-4: catches ... ignores を持つデコレータが、returns が null でない op に付いている。
func checkDec4(ix *index, o *ir.Op, r *reporter) {
	if o.Returns == nil {
		return
	}
	for ai := range o.Decorators {
		app := &o.Decorators[ai]
		d, _ := ix.decorator(app.Decorator)
		if d == nil {
			continue
		}
		for _, cl := range d.Catches {
			if cl.Mode == ir.Ignores {
				r.errorAt("DEC-4", app.Span, sprintf("@%s は ignores を持つので、値を返す op %s には付けられない", shortName(app.Decorator), o.Name))
				break
			}
		}
	}
}

// checkDec5 は DEC-5: 計算の手順 i で recovers が succ(e) を unknown にした（効果が不明な例外を recovers で吸収する
// デコレータを、効果を持つ op に付けた。§8.2）。位置はデコレータの適用。
func checkDec5(o *ir.Op, app *ir.Application, x string, lost []string, r *reporter) {
	r.warnAt("DEC-5", app.Span, sprintf("@%s は %s を recovers で吸収するが、%s のとき効果 %s は不明なので、%s の成功のアームでも不明になる", shortName(app.Decorator), x, x, joinNames(lost), o.Name))
}

// checkDec6 は DEC-6: 一つのデコレータの宣言で、catches の例外と throws の例外に同じものがある。
func checkDec6(d *ir.Decorator, r *reporter) {
	var same []string
	for _, c := range d.Catches {
		for _, x := range c.Exceptions {
			for _, t := range d.Throws {
				if contains(t.Exceptions, x) {
					same = appendUnique(same, x)
				}
			}
		}
	}
	if len(same) > 0 {
		r.warnAt("DEC-6", d.Span, sprintf("%s は %s を吸収して投げ直すので、差し引きがゼロになる", d.Name, joinNames(same)))
	}
}

// checkDec16 は DEC-16: 計算の手順 i で、catches の例外がそこまでの E にない（吸収する相手がない）。位置はデコレータの適用。
func checkDec16(o *ir.Op, app *ir.Application, x string, r *reporter) {
	r.warnAt("DEC-16", app.Span, sprintf("@%s は %s を吸収するが、%s はここまでに %s を投げない", shortName(app.Decorator), x, o.Name, x))
}
