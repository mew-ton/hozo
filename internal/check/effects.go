package check

import (
	"github.com/mew-ton/hozo/ir"
)

// checkStates は STATE-2 と、EFF-4、READS-3（docs/check.md「状態と効果」）。Signature は計算済み。配線の op に書いた
// reads / effects の節と、計算した分の、過不足は、エラーにしない（READS-2 と EFF-2 は欠番）: 節に書いたものは、計算した分に
// 足す（compute.go）。計算した分にないものを書いていれば、warning（checkUnperformed）。
func checkStates(ix *index, c *computer, r *reporter) {
	for i, o := range ix.ops {
		checkState2(ix, o, ix.opsNS[i], r)
		checkUnperformed(ix, c, o, ix.opsNS[i], r)
	}
}

// checkUnperformed は EFF-4 と READS-3（docs/check.md「状態と効果」。warning）: 配線の op が、effects（reads）の節に書いた
// 効果（状態）が、計算した分にない。「計算した分」は、その節を書かなかったものとして計算した、Sig(O, ns(O)) の F（R）:
// 本体が行き着くもの、運ばれたもの、付いたデコレータの本体と halts が足すもの。作られる本体は、節に書いただけのものを
// 行わないので、呼び出しの書き忘れの恐れを、知らせる。葉の op（STATE-5）と、呼び出しが循環している op（WIRE-7）は、対象外。
func checkUnperformed(ix *index, c *computer, o *ir.Op, ns string, r *reporter) {
	if o.Body == nil || c.cyclic[o.Name] {
		return
	}
	if len(o.Effects) > 0 {
		computed := c.effectsWithoutClause(o, ns)
		var seen []ir.EffectRef
		for _, e := range o.Effects {
			if contains(seen, e) || contains(computed, e) {
				continue
			}
			seen = append(seen, e)
			r.warnAt("EFF-4", o.Span, sprintf("%s は effects の節に %s を書いているが、本体も、呼ぶ先も、デコレータも、それを行わない（作られる本体は、節に書いただけの効果を、行わない）。本体に、呼び出しを書くか、節から外す", o.Name, e))
		}
	}
	if len(o.Reads) > 0 {
		computed := c.readsWithoutClause(o)
		var seen []string
		for _, state := range ix.readStates(o.Reads) {
			if contains(seen, state) || contains(computed, state) {
				continue
			}
			seen = append(seen, state)
			r.warnAt("READS-3", o.Span, sprintf("%s は reads の節に状態 %s を書いているが、本体も、呼ぶ先も、デコレータも、それを読まない（作られる本体は、節に書いただけの状態を、読まない）。本体で読むか、節から外す", o.Name, state))
		}
	}
}

// checkState2 は STATE-2 のうち、デコレータの halts の効果の分: halts の効果（宣言に書いたもの、any effect の引数で
// 渡したもの）の名前空間が、デコレータを付けた op の名前空間と違う。位置はその適用。
// op にデコレータが運ぶ効果の分は、internal/sema が出す（docs/check.md「状態と効果」の STATE-2）。
func checkState2(ix *index, o *ir.Op, ns string, r *reporter) {
	for ai := range o.Decorators {
		app := &o.Decorators[ai]
		d, args := ix.decorator(app.Decorator)
		if d == nil {
			continue
		}
		if len(app.Args) > 0 || args == nil {
			args = app.Args
		}
		subst := substituter(args)
		for _, cl := range d.Catches {
			if cl.Mode != ir.Halts {
				continue
			}
			for _, h := range subst(cl.Halts) {
				if ix.effect(h) != nil && ix.namespaceOf(h) != ns {
					r.errorAt("STATE-2", app.Span, sprintf("@%s は効果 %s を起こすが、%s は名前空間 %s の持ち主ではない", shortName(app.Decorator), h, o.Name, ix.namespaceOf(h)))
				}
			}
		}
	}
}
