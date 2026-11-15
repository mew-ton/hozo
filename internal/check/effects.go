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
		checkReads2(o, r)
	}
}

// checkReads2 は READS-2: 配線の O で Op.reads が空でなく、集合として Sig(O, ns(O)).reads と等しくない。
// 足りないもの・余分なものを Data.missing / Data.extra に。位置は O。
func checkReads2(o *ir.Op, r *reporter) {
	if o.Body == nil || len(o.Reads) == 0 {
		return
	}
	missing := []string{}
	extra := []string{}
	for _, s := range o.Signature.Reads {
		if !contains(o.Reads, s) {
			missing = append(missing, s)
		}
	}
	for _, s := range o.Reads {
		if !contains(o.Signature.Reads, s) {
			extra = appendUnique(extra, s)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	msg := sprintf("%s に書いた reads が、本体から計算した読む状態と食い違う", o.Name)
	if len(missing) > 0 {
		msg += sprintf("（足りない: %s）", joinNames(missing))
	}
	if len(extra) > 0 {
		msg += sprintf("（余分: %s）", joinNames(extra))
	}
	r.report(diag.Error, "READS-2", o.Span, msg, nil, map[string]any{"missing": missing, "extra": extra})
}

// checkState2 は STATE-2: 葉の op O の Op.effects の効果 e の名前空間が ns(O) と違う。デコレータの halts の効果
// （宣言に書いたもの、any effect の引数で渡したもの）も、デコレータを付けた op の名前空間と違えば同じ。
func checkState2(ix *index, o *ir.Op, ns string, r *reporter) {
	if o.Body == nil {
		for _, e := range o.Effects {
			if ix.effect(e) != nil && ix.namespaceOf(e) != ns {
				r.errorAt("STATE-2", o.Span, sprintf("%s は名前空間 %s の持ち主ではないので、効果 %s を effects に宣言できない", o.Name, ix.namespaceOf(e), e))
			}
		}
	}
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

// checkEff2 は EFF-2: 配線の O で Op.effects が空でなく、集合として Sig(O, ns(O)).effects と等しくない。
// 足りないもの・余分なものを Data.missing / Data.extra に。
func checkEff2(o *ir.Op, r *reporter) {
	if o.Body == nil || len(o.Effects) == 0 {
		return
	}
	missing := []string{}
	extra := []string{}
	for _, e := range o.Signature.Effects {
		if !contains(o.Effects, e) {
			missing = append(missing, e)
		}
	}
	for _, e := range o.Effects {
		if !contains(o.Signature.Effects, e) {
			extra = appendUnique(extra, e)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	msg := sprintf("%s に書いた effects が伝播した効果と食い違う", o.Name)
	if len(missing) > 0 {
		msg += sprintf("（足りない: %s）", joinNames(missing))
	}
	if len(extra) > 0 {
		msg += sprintf("（余分: %s）", joinNames(extra))
	}
	r.report(diag.Error, "EFF-2", o.Span, msg, nil, map[string]any{"missing": missing, "extra": extra})
}
