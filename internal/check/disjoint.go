package check

import (
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// armCall は、disjoint のアーム（か、disjoint for のブロック）の中の、呼び出し一つと、その Sc。
type armCall struct {
	ref callRef
	sig *sigState
}

// armInfo は、アーム一つについて集めたもの（docs/check.md「disjoint」の「検査」の E、F、W、R）。
type armInfo struct {
	// throwing は、例外を出しうる呼び出し（Sc.exceptions が空でない。アームの本体の throw を含む）。
	throwing []armCall
	// effectful は、効果を持つ呼び出し（Sc.effects が空でない）。
	effectful []armCall
	// writes は、効果が書き込む状態から、その状態に書く呼び出し（出現順）。states は、その状態の出現順。
	writes map[string][]armCall
	states []string
	// reads は、アームが読む状態。
	reads []string
}

// checkDisjoint は DISJ-1〜DISJ-3（docs/check.md「disjoint」の「検査」）。同じ disjoint の、別々のアームのすべての組と、
// disjoint for のブロック（回どうしを、アームと見る）を確かめる。位置は、disjoint の文。当たった二つの呼び出しを Related に示す。
func checkDisjoint(ix *index, c *computer, r *reporter) {
	for _, o := range ix.ops {
		if o.Body == nil || c.cyclic[o.Name] {
			continue
		}
		ns := ix.namespaceOf(o.Name)
		walkStmts(o.Body.Statements, func(st ir.Statement, _ []int) {
			switch s := st.(type) {
			case ir.DisjointStmt:
				arms := make([]armInfo, len(s.Arms))
				for i, arm := range s.Arms {
					arms[i] = c.armInfo(arm.Statements, ns)
				}
				for i := range arms {
					for j := range arms {
						if i != j {
							c.checkArmPair(&arms[i], &arms[j], i < j, s.Span, "", r)
						}
					}
				}
			case ir.ForStmt:
				if s.Disjoint {
					info := c.armInfo(s.Body, ns)
					c.checkArmPair(&info, &info, true, s.Span, s.Var, r)
				}
			}
		})
	}
}

// armInfo は、アームの文の並び（中の for と disjoint も含む）から、E、F、W、R を集める。
func (c *computer) armInfo(stmts []ir.Statement, ns string) armInfo {
	info := armInfo{writes: map[string][]armCall{}}
	for _, row := range rowsOf(&ir.Wiring{Statements: stmts}) {
		for _, cr := range row.calls {
			sig := c.callSig(cr, ns)
			if sig == nil {
				continue
			}
			ac := armCall{ref: cr, sig: sig}
			if len(sig.E) > 0 {
				info.throwing = append(info.throwing, ac)
			}
			if len(sig.F) > 0 {
				info.effectful = append(info.effectful, ac)
			}
			for _, e := range sig.F {
				for _, state := range c.ix.effectWrites(e) {
					if _, seen := info.writes[state]; !seen {
						info.states = append(info.states, state)
					}
					if !containsCall(info.writes[state], ac) {
						info.writes[state] = append(info.writes[state], ac)
					}
				}
			}
		}
	}
	info.reads = c.stmtReads(stmts)
	return info
}

func containsCall(list []armCall, ac armCall) bool {
	for _, x := range list {
		if x.ref.call.Span == ac.ref.call.Span {
			return true
		}
	}
	return false
}

// effectWrites は、効果が書き込む基底の状態（docs/check.md「効果の宛先」の「効果が書き込む状態」）。
func (ix *index) effectWrites(e ir.EffectRef) []string {
	switch x := e.(type) {
	case ir.NamedEffect:
		if eff := ix.effect(x.Effect); eff != nil {
			return eff.Writes
		}
	case ir.DirectEffect:
		return []string{x.State}
	}
	return nil
}

// checkArmPair は、アーム a、b（a ≠ b）の組を確かめる。loopVar が空でなければ、disjoint for のブロック（a と b は同じもの）で、
// loopVar は、その for の変数。first は、対称な規則（DISJ-3）を、組ごとに一度だけ報告するための印。
func (c *computer) checkArmPair(a, b *armInfo, first bool, at ir.Span, loopVar string, r *reporter) {
	// DISJ-1: E(a) が空でなく、F(b) が空でない
	if len(a.throwing) > 0 && len(b.effectful) > 0 {
		x, e := a.throwing[0], b.effectful[0]
		msg := "disjoint に、例外を出しうるアームと、効果を持つ、別のアームがある（順を入れ替えると、例外のときに、起きている効果が変わる。直列に書く）"
		if loopVar != "" {
			msg = "disjoint for のブロックが、例外を出しうるのに、効果を持つ（回の順を入れ替えると、例外のときに、起きている効果が変わる。disjoint を外して、直列の for にする）"
		}
		r.report(diag.Error, "DISJ-1", at, msg, []related{
			{span: x.ref.call.Span, msg: sprintf("例外を出しうる: %s", strings.Join(x.sig.E, ", "))},
			{span: e.ref.call.Span, msg: sprintf("効果を持つ: %s", joinEffects(e.sig.F))},
		}, nil)
	}
	// DISJ-2: W(a) と R(b) に、同じ状態がある
	for _, state := range a.states {
		if !contains(b.reads, state) {
			continue
		}
		msg := sprintf("disjoint で、あるアームが効果で書く状態 %s を、別のアームが読んでいる（順を入れ替えると、読む値が変わる。直列に書く）", state)
		if loopVar != "" {
			msg = sprintf("disjoint for のブロックが、効果で書く状態 %s を、読んでいる（回の順を入れ替えると、読む値が変わる。disjoint を外して、直列の for にする）", state)
		}
		r.report(diag.Error, "DISJ-2", at, msg, []related{
			{span: a.writes[state][0].ref.call.Span, msg: sprintf("%s に書く", state)},
		}, nil)
	}
	// DISJ-3: W(a) と W(b) に、同じ状態 s があり、行で見て、通らない
	if !first {
		return
	}
	for _, state := range a.states {
		if _, both := b.writes[state]; !both {
			continue
		}
		if reason, at2 := c.rowConflict(state, a.writes[state], b.writes[state], loopVar); reason != "" {
			msg := sprintf("disjoint で、二つのアームが、同じ状態 %s に、効果を持つ: %s（同じ行に書くと、順で、結果が変わる。直列に書く）", state, reason)
			if loopVar != "" {
				msg = sprintf("disjoint for のブロックが、状態 %s に、効果を持つ: %s（回どうしが、同じ行に書きうる。disjoint を外して、直列の for にする）", state, reason)
			}
			r.report(diag.Error, "DISJ-3", at, msg, []related{{span: at2, msg: sprintf("%s に書く", state)}}, nil)
		}
	}
}

func joinEffects(es []ir.EffectRef) string {
	names := make([]string, len(es))
	for i, e := range es {
		names[i] = e.String()
	}
	return strings.Join(names, ", ")
}

// rowConflict は、状態 state に書く呼び出し as（アーム a）と bs（アーム b）を、行で見る（docs/check.md「disjoint」の「検査」）。
// 通れば理由は空。通らなければ、理由と、示す呼び出しの位置。loopVar が空でなければ、disjoint for のブロックで、通るのは、
// どの呼び出しも、行を受け取り、渡した値が、loopVar から始まる Atom のとき。
func (c *computer) rowConflict(state string, as, bs []armCall, loopVar string) (string, ir.Span) {
	st := c.ix.state(state)
	if st == nil || st.Shape != ir.StateList || st.Element == nil {
		return "並び（list）の状態でないので、行がない", as[0].ref.call.Span
	}
	rowOf := func(ac armCall) (ir.Atom, bool) { return c.rowArg(ac.ref.call, st.Element) }
	var av, bv []ir.Atom
	for _, ac := range as {
		v, ok := rowOf(ac)
		if !ok {
			return "行を受け取らない呼び出しがある（比べる値がない）", ac.ref.call.Span
		}
		av = append(av, v)
	}
	if loopVar != "" {
		for i, v := range av {
			if n, ok := v.(ir.NameAtom); !ok || len(n.Path) == 0 || n.Path[0] != loopVar {
				return sprintf("渡した値が、for の変数 %s から始まらない（どの回も、同じ値を渡す）", loopVar), as[i].ref.call.Span
			}
		}
		return "", ir.Span{}
	}
	for _, ac := range bs {
		v, ok := rowOf(ac)
		if !ok {
			return "行を受け取らない呼び出しがある（比べる値がない）", ac.ref.call.Span
		}
		bv = append(bv, v)
	}
	for i, x := range av {
		for _, y := range bv {
			if sameAtom(x, y) {
				return "同じ値を渡している", as[i].ref.call.Span
			}
		}
	}
	return "", ir.Span{}
}

// rowArg は、呼び出しが、状態の行（要素の型 elem）を受け取る引数に、渡した値。行を受け取らない呼び出しなら ok が偽。
//   - 効果（宣言）の呼び出し: 引数のうち、型が elem と等しいものが、ちょうど一つ（行の引数と、同じ決まり）。
//   - 直に呼ぶ組み込みの効果: 値の引数。型が elem と等しいとき。
//   - op の呼び出し: 引数のうち、型が、elem と等しいか、elem を絞り込んだ型（case）のものが、ちょうど一つ。
func (c *computer) rowArg(call ir.Call, elem ir.Type) (ir.Atom, bool) {
	var names []string
	switch callee := call.Callee.(type) {
	case ir.EffectCallee:
		switch e := callee.Effect.(type) {
		case ir.NamedEffect:
			eff := c.ix.effect(e.Effect)
			if eff == nil {
				return nil, false
			}
			for _, p := range eff.Params {
				if sameType(p.Type, elem) {
					names = append(names, p.Name)
				}
			}
		case ir.DirectEffect:
			// 値を取る操作の引数名は "value"（docs/ir.md「配線」の Call）。型は、渡した値から決まらないので、
			// 行の並び（post の一覧）でなければ、行と見る
			if e.Operation == ir.OpDelete {
				return nil, false
			}
			names = append(names, "value")
		}
	case ir.OpCallee:
		o := c.ix.op(callee.Op)
		if o == nil {
			return nil, false
		}
		for _, p := range o.Params {
			t := p.Type
			if ct, ok := t.(ir.CaseType); ok {
				t = ct.Of
			}
			if sameType(t, elem) {
				names = append(names, p.Name)
			}
		}
	}
	if len(names) != 1 {
		return nil, false
	}
	for _, a := range call.Args {
		if a.Param == names[0] {
			return a.Value, a.Value != nil
		}
	}
	return nil, false
}

// sameAtom は、渡した値が同じか: 同じ名前（val、引数、for の変数）の、同じパスか、同じ状態の、同じパス。
// リテラルと、レコードリテラルは、どの値とも、同じでない。
func sameAtom(x, y ir.Atom) bool {
	switch a := x.(type) {
	case ir.NameAtom:
		b, ok := y.(ir.NameAtom)
		return ok && strings.Join(a.Path, ".") == strings.Join(b.Path, ".")
	case ir.StateAtom:
		b, ok := y.(ir.StateAtom)
		return ok && a.State == b.State && strings.Join(a.Path, ".") == strings.Join(b.Path, ".")
	}
	return false
}

// sameType は、二つの型が等しいか（docs/types.md「等しい」）。型は、正規化した後なので、表示の文字列で比べる。
func sameType(a, b ir.Type) bool {
	return a != nil && b != nil && a.String() == b.String()
}
