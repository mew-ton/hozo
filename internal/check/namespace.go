package check

import (
	"github.com/mew-ton/hozo/ir"
)

// checkNamespaces は「名前空間」の表（docs/check.md「診断」）のうち、計算結果が要る DOC-1 の @throws。
// DOC-1 の @param / @see、DOC-2、NAME-1、NS-*、UNION-1 は解決と正規化の側で出る。
func checkNamespaces(c *computer, r *reporter) {
	for _, o := range c.ix.ops {
		checkDoc1Throws(c, o, r)
	}
}

// checkDoc1Throws は DOC-1: op の @throws の例外が Signature.exceptions（Sig(O, ns(O))）にも、
// O に付いたデコレータの outer throws にもない（docs/check.md「名前空間」）。
func checkDoc1Throws(c *computer, o *ir.Op, r *reporter) {
	if o.Description == nil {
		return
	}
	_, _, external := c.decoratorSets(o)
	for _, t := range o.Description.Throws {
		if !contains(o.Signature.Exceptions, t.Exception) && !contains(external, t.Exception) {
			r.errorAt("DOC-1", o.Span, sprintf("%s の @throws %s は、%s が投げうる例外にない", o.Name, t.Exception, o.Name))
		}
	}
}
