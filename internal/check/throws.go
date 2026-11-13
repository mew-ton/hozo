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
// 目的語は常に `effects`（その op のすべての効果。s.F）。
func checkRel4(o *ir.Op, s *sigState, r *reporter) {
	for _, cl := range o.Throws {
		for _, x := range cl.Exceptions {
			for _, rel := range cl.Relations {
				want := ir.Prevented
				if rel.Kind == ir.Keeps {
					want = ir.Kept
				}
				for _, e := range s.F {
					if got := s.relOf(x, e); got != want {
						r.errorAt("REL-4", o.Span,
							sprintf("%s の throws は %s が %s を %s と書いているが、本体から計算した状態は %s", o.Name, x, e, rel.Kind, got))
					}
				}
			}
		}
	}
}

// checkThrows は REL-2（docs/check.md「throws」）。Signature は計算済み。
func checkThrows(ix *index, c *computer, r *reporter) {
	for _, o := range ix.ops {
		checkRel2(o, r)
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
