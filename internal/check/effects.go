package check

import (
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// checkStates は STATE-2 / EFF-2 / READS-2（docs/check.md「状態と効果」）。Signature は計算済み。
func checkStates(ix *index, c *computer, r *reporter) {
	for i, o := range ix.ops {
		checkState2(ix, o, ix.opsNS[i], r)
		checkEff2(o, r)
		checkReads2(ix, o, r)
	}
}

// checkReads2 は READS-2: 配線の O で Op.reads が空でなく、その要素の状態（read の宣言なら、その宛先）の集合が
// Sig(O, ns(O)).reads（O に運ばれた状態を含む）と等しくない。足りないもの・余分なものを Data.missing / Data.extra に。位置は O。
func checkReads2(ix *index, o *ir.Op, r *reporter) {
	if o.Body == nil || len(o.Reads) == 0 {
		return
	}
	written := ix.readStates(o.Reads)
	missing := []string{}
	extra := []string{}
	for _, s := range o.Signature.Reads {
		if !contains(written, s) {
			missing = append(missing, s)
		}
	}
	for _, s := range written {
		if !contains(o.Signature.Reads, s) {
			extra = appendUnique(extra, s)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	msg := sprintf("%s に書いた reads が、計算した読む状態（本体が行き着くものと、デコレータが運ぶもの）と食い違う", o.Name)
	if len(missing) > 0 {
		msg += sprintf("（足りない: %s）", joinNames(missing))
	}
	if len(extra) > 0 {
		msg += sprintf("（余分: %s）", joinNames(extra))
	}
	r.report(diag.Error, "READS-2", o.Span, msg, nil, map[string]any{"missing": missing, "extra": extra})
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

// checkEff2 は EFF-2: 配線の O で Op.effects が空でなく、集合として Sig(O, ns(O)).effects（O に運ばれた効果を含む）と等しくない（効果の数え方は
// docs/check.md「節に書く要素」: 直に書いた効果と、同じ宛先の宣言は別）。足りないもの・余分なものを Data.missing /
// Data.extra に（宣言は FQN、直に書いた効果は "<状態の FQN>.<操作>"）。
func checkEff2(o *ir.Op, r *reporter) {
	if o.Body == nil || len(o.Effects) == 0 {
		return
	}
	missing := []string{}
	extra := []string{}
	for _, e := range o.Signature.Effects {
		if !contains(o.Effects, e) {
			missing = append(missing, e.String())
		}
	}
	for _, e := range o.Effects {
		if !contains(o.Signature.Effects, e) {
			extra = appendUnique(extra, e.String())
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	msg := sprintf("%s に書いた effects が、計算した効果（本体が行き着くものと、デコレータが運ぶもの）と食い違う", o.Name)
	if len(missing) > 0 {
		msg += sprintf("（足りない: %s）", joinNames(missing))
	}
	if len(extra) > 0 {
		msg += sprintf("（余分: %s）", joinNames(extra))
	}
	r.report(diag.Error, "EFF-2", o.Span, msg, nil, map[string]any{"missing": missing, "extra": extra})
}
