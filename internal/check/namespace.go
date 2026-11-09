package check

import (
	"github.com/mew-ton/hozo/ir"
)

// checkNamespaces は「名前空間」の表（docs/check.md「診断」）のうち、計算結果が要る DOC-1 の @throws。
// DOC-1 の @param / @see、DOC-2、NAME-1、NS-*、UNION-1 は解決と正規化の側で出る。
func checkNamespaces(ix *index, r *reporter) {
	for _, o := range ix.ops {
		checkDoc1Throws(o, r)
	}
}

// checkDoc1Throws は DOC-1: op の @throws の例外が Signature.exceptions（Sig(O, ns(O))）にない。
func checkDoc1Throws(o *ir.Op, r *reporter) {
	if o.Description == nil {
		return
	}
	for _, t := range o.Description.Throws {
		if !contains(o.Signature.Exceptions, t.Exception) {
			r.errorAt("DOC-1", o.Span, sprintf("%s の @throws %s は、%s が投げうる例外にない", o.Name, t.Exception, o.Name))
		}
	}
}
