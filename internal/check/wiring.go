package check

import (
	"strings"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// checkWiring は WIRE-6（docs/check.md「配線」）。WIRE-7 は計算の循環の検出で出る（checkWire7）。
func checkWiring(ix *index, cfg *config.Config, r *reporter) {
	max := config.DefaultMaxBindings
	if cfg != nil {
		max = cfg.Hozo.MaxBindings
	}
	for _, o := range ix.ops {
		checkWire6(o, max, r)
	}
}

// checkWire6 は WIRE-6: 本体の val の名前の数（for のブロックと、disjoint のアームの中も数える。for の変数は、数えない）が
// [hozo].max_bindings を超える（op に切り出す候補）。
func checkWire6(o *ir.Op, max int, r *reporter) {
	if o.Body == nil {
		return
	}
	n := 0
	walkStmts(o.Body.Statements, func(st ir.Statement, _ []int) {
		if _, ok := st.(ir.ValStmt); ok {
			n++
		}
	})
	if n > max {
		r.warnAt("WIRE-6", o.Span, sprintf("%s の val が %d 個あり、上限 %d を超えている。op に切り出す候補", o.Name, n, max))
	}
}

// checkWire7 は WIRE-7: 配線の呼び出しが循環している。位置は循環上で出現順に最初の op、Related に循環上のすべての op。
func checkWire7(members []*ir.Op, r *reporter) {
	if len(members) == 0 {
		return
	}
	names := make([]string, 0, len(members))
	rel := make([]related, 0, len(members))
	for _, o := range members {
		names = append(names, o.Name)
		rel = append(rel, related{span: o.Span, msg: sprintf("循環上の op: %s", o.Name)})
	}
	r.report(diag.Error, "WIRE-7", members[0].Span,
		sprintf("配線の呼び出しが循環している（%s）", strings.Join(names, " → ")), rel, nil)
}
