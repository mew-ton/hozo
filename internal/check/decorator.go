package check

import (
	"github.com/mew-ton/hozo/ir"
)

// checkDecorators は DEC-4 / DEC-6 / DEC-14（docs/check.md「デコレータ」）。
// DEC-5 / DEC-16 は計算の手順 i で出る（checkDec5 / checkDec16）。DEC-10 は計算（手順 i の throws）で、診断なし。
func checkDecorators(ix *index, c *computer, r *reporter) {
	for i := range ix.p.Namespaces {
		for _, d := range ix.p.Namespaces[i].Decls {
			dec, ok := d.(*ir.Decorator)
			if !ok || dec.Alias != nil {
				continue
			}
			checkDec6(dec, r)
			checkDec14(dec, r)
		}
	}
	for _, o := range ix.ops {
		checkDec4(ix, o, r)
	}
}

// checkDec4 は DEC-4: catches ... ignores を持つデコレータが、returns が null でない op に付いている。
func checkDec4(ix *index, o *ir.Op, r *reporter) {
	if o.Returns == nil {
		return
	}
	for ai := range o.Decorators {
		app := &o.Decorators[ai]
		d, _ := ix.decorator(app.Decorator)
		if d == nil {
			continue
		}
		for _, cl := range d.Catches {
			if cl.Mode == ir.Ignores {
				r.errorAt("DEC-4", app.Span, sprintf("@%s は ignores を持つので、値を返す op %s には付けられない", shortName(app.Decorator), o.Name))
				break
			}
		}
	}
}

// checkDec5 は DEC-5: 計算の手順 i で recovers が succ(e) を unknown にした（効果が不明な例外を recovers で吸収する
// デコレータを、効果を持つ op に付けた。§8.2）。位置はデコレータの適用。
func checkDec5(o *ir.Op, app *ir.Application, x string, lost []string, r *reporter) {
	r.warnAt("DEC-5", app.Span, sprintf("@%s は %s を recovers で吸収するが、%s のとき効果 %s は不明なので、%s の成功のアームでも不明になる", shortName(app.Decorator), x, x, joinNames(lost), o.Name))
}

// checkDec6 は DEC-6: 一つのデコレータの宣言で、catches の例外と throws の例外に同じものがある。
func checkDec6(d *ir.Decorator, r *reporter) {
	var same []string
	for _, c := range d.Catches {
		for _, x := range c.Exceptions {
			for _, t := range d.Throws {
				if contains(t.Exceptions, x) {
					same = appendUnique(same, x)
				}
			}
		}
	}
	if len(same) > 0 {
		r.warnAt("DEC-6", d.Span, sprintf("%s は %s を吸収して投げ直すので、差し引きがゼロになる", d.Name, joinNames(same)))
	}
}

// checkDec14 は DEC-14: external なデコレータが throws を持たない。
func checkDec14(d *ir.Decorator, r *reporter) {
	if d.External && len(d.Throws) == 0 {
		r.warnAt("DEC-14", d.Span, sprintf("%s は @external だが throws を持たないので、境界の外から見ても型が変わらない", d.Name))
	}
}

// checkDec16 は DEC-16: 計算の手順 i で、catches の例外がそこまでの E にない（吸収する相手がない）。位置はデコレータの適用。
func checkDec16(o *ir.Op, app *ir.Application, x string, r *reporter) {
	r.warnAt("DEC-16", app.Span, sprintf("@%s は %s を吸収するが、%s はここまでに %s を投げない", shortName(app.Decorator), x, o.Name, x))
}
