package check

import (
	"github.com/mew-ton/hozo/ir"
)

// checkRel4 は REL-4（docs/check.md「throws」）。計算の手順 0 で、配線の Op.throws に relations が書かれていれば
// 計算した rel と比べ、prevents なのに prevented でない、keeps なのに kept でない組を示す。
// 目的語は常に `effects`（その op のすべての効果。s.F）。関係を付けた throws は、その op の高さでの、例外の宣言の上書きで
// （同「状態に触れるもの」の「節に書く要素」）、呼ぶ先の言った向きを変えたとき、広げた範囲に、例外の前に起きる効果が
// あるときに、ここに当たる。
//
// 配線の op の throws の節と、計算した例外の、過不足は、確かめない（THROWS-1 は欠番）: 節に書くのは、その関数が足す分。
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
							sprintf("%s の throws は %s が %s を %s と書いているが、計算した状態は %s", o.Name, x, e, rel.Kind, got))
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
