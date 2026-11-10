package check

import (
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// checkThrows1 は THROWS-1（docs/check.md「throws」）。配線の O について
// Missing = (∪ Sc.exceptions) − Declared(O) − ∪Catches(dᵢ) − ∪Throws(dᵢ, ns(O))（external の throws は Sig と同じく数えない）。
// 空でなければ、位置は O、メッセージに Missing の FQN、Related に各例外の出どころの呼び出し、Data.missing に FQN の列。
func checkThrows1(c *computer, o *ir.Op, info *bodyInfo) {
	decl := declared(o)
	catches, throws, _ := c.decoratorSets(o)
	var missing []string
	for _, x := range info.thrown {
		if contains(decl, x) || contains(catches, x) || contains(throws, x) {
			continue
		}
		missing = append(missing, x)
	}
	if len(missing) == 0 {
		return
	}
	var rel []related
	for _, x := range missing {
		for _, src := range info.sources[x] {
			rel = append(rel, related{span: src.ref.call.Span, msg: sprintf("出どころ: %s の呼び出し（%s）", src.ref.call.Op, x)})
		}
	}
	c.r.report(diag.Error, "THROWS-1", o.Span,
		sprintf("%s は %s を投げうるが throws に書かれていない", o.Name, joinNames(missing)),
		rel, map[string]any{"missing": missing})
}

// checkRel4 は REL-4（docs/check.md「throws」）。計算の手順 0 で、配線の Op.throws に relations が書かれていれば
// 計算した rel と比べ、prevents なのに prevented でない、keeps なのに kept でない組を示す。
func checkRel4(o *ir.Op, s *sigState, r *reporter) {
	for _, cl := range o.Throws {
		for _, x := range cl.Exceptions {
			for _, rel := range cl.Relations {
				effects := rel.Effects.Effects
				if rel.Effects.All {
					effects = s.F
				}
				want := ir.Prevented
				if rel.Kind == ir.Keeps {
					want = ir.Kept
				}
				for _, e := range effects {
					if !s.hasF(e) {
						continue // REL-6
					}
					if got := s.relOf(x, e); got != want {
						r.errorAt("REL-4", o.Span,
							sprintf("%s の throws は %s が %s を %s と書いているが、本体から計算した状態は %s", o.Name, x, e, rel.Kind, got))
					}
				}
			}
		}
	}
}

// checkThrows は REL-2 / REL-3 / REL-6（docs/check.md「throws」）。Signature は計算済み。
func checkThrows(ix *index, c *computer, r *reporter) {
	for _, o := range ix.ops {
		checkRel2(o, r)
		checkRel3Op(o, r)
		checkRel6(o, r)
	}
	for i := range ix.p.Namespaces {
		for _, d := range ix.p.Namespaces[i].Decls {
			if dec, ok := d.(*ir.Decorator); ok && dec.Alias == nil {
				checkRel3Decorator(dec, r)
			}
		}
	}
}

// hasRelations は throws の節のどれかが relations を持つか。
func hasRelations(clauses []ir.ThrowsClause) bool {
	for _, cl := range clauses {
		if len(cl.Relations) > 0 {
			return true
		}
	}
	return false
}

// checkRel2 は REL-2: Sig(O, ns(O)).effects が空の op の Op.throws に relations がある。
// デコレータの throws の relations は対象外（§7.3）。
func checkRel2(o *ir.Op, r *reporter) {
	if len(o.Signature.Effects) == 0 && hasRelations(o.Throws) {
		r.warnAt("REL-2", o.Span, sprintf("%s は効果を持たないので、throws の prevents / keeps に意味がない", o.Name))
	}
}

// checkRel3Op は REL-3: op の relations.effects が all でなく名前の列。
func checkRel3Op(o *ir.Op, r *reporter) {
	if names := namedRelationEffects(o.Throws); len(names) > 0 {
		r.warnAt("REL-3", o.Span, sprintf("%s の throws は効果の名前 %s を個別に書いている。効果ごとに葉の op を分ければ宣言が要らなくなる", o.Name, joinNames(names)))
	}
}

// checkRel3Decorator は REL-3: デコレータの throws の relations.effects が名前の列。
func checkRel3Decorator(d *ir.Decorator, r *reporter) {
	if names := namedRelationEffects(d.Throws); len(names) > 0 {
		r.warnAt("REL-3", d.Span, sprintf("%s の throws は効果の名前 %s を個別に書いている", d.Name, joinNames(names)))
	}
}

func namedRelationEffects(clauses []ir.ThrowsClause) []string {
	var names []string
	for _, cl := range clauses {
		for _, rel := range cl.Relations {
			if rel.Effects.All {
				continue
			}
			for _, e := range rel.Effects.Effects {
				names = appendUnique(names, e)
			}
		}
	}
	return names
}

// checkRel6 は REL-6: relations の効果名が Sig(O, ns(O)).effects にない。
func checkRel6(o *ir.Op, r *reporter) {
	var unknown []string
	for _, e := range namedRelationEffects(o.Throws) {
		if !contains(o.Signature.Effects, e) {
			unknown = appendUnique(unknown, e)
		}
	}
	if len(unknown) > 0 {
		r.errorAt("REL-6", o.Span, sprintf("%s の throws に書いた効果 %s は %s の効果にない", o.Name, joinNames(unknown), o.Name))
	}
}
