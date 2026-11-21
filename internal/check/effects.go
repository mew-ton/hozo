package check

import (
	"github.com/mew-ton/hozo/ir"
)

// checkStates は STATE-2（docs/check.md「状態と効果」）。Signature は計算済み。配線の op に書いた reads / effects の節と、
// 計算した分の、過不足は、確かめない（READS-2 と EFF-2 は欠番）: 節に書いたものは、計算した分に足す（compute.go）。
func checkStates(ix *index, c *computer, r *reporter) {
	for i, o := range ix.ops {
		checkState2(ix, o, ix.opsNS[i], r)
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
