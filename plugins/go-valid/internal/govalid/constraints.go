package govalid

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/validvocab"
)

// このファイルは docs/plugin-go-valid.md「制約」の表: valid の標準の制約一つを、Go の条件にする（condition）。
// 制約を読むこと（何に付けられ、どの値を通すか。使い方の誤り VALID-*）は、ここではない: 公開の plugin/validvocab
// （docs/plugin-valid.md「制約」）。

// constraint は、scalar かフィールドに付いた、valid の標準の制約一つ（validvocab が読んだもの）と、Go の出力に要るもの。
type constraint struct {
	validvocab.Constraint
	// variable は、pattern の正規表現の、パッケージの変数の名前（付いた先ごとに、描くときに決める）。
	variable string
}

// wrap は、validvocab が読んだ制約の列を、このパッケージの形にする。
func wrap(cons []validvocab.Constraint) []constraint {
	out := make([]constraint, len(cons))
	for i := range cons {
		out[i] = constraint{Constraint: cons[i]}
	}
	return out
}

// condition は、値の式 value（元の型に変換できる Go の式）が制約 c に違反するときに真になる Go の条件
// （docs/plugin-go-valid.md「制約」の表の「条件」）。length は、`l := …; <比較>` の形（if の初期化の文を含む）。
// f は、条件が使う import を足す先。
func (c constraint) condition(value string, f *file) string {
	switch c.Decorator {
	case validvocab.Min, validvocab.Max:
		cmp := "<"
		if c.Decorator == validvocab.Max {
			cmp = ">"
		}
		if c.Core == ir.CoreInt {
			return fmt.Sprintf("int64(%s) %s %d", value, cmp, int64(c.Bound))
		}
		return fmt.Sprintf("float64(%s) %s %s", value, cmp, c.BoundText)
	case validvocab.Length:
		var cmps []string
		if c.HasMin {
			cmps = append(cmps, fmt.Sprintf("l < %d", c.Min))
		}
		if c.HasMax {
			cmps = append(cmps, fmt.Sprintf("l > %d", c.Max))
		}
		if c.Count {
			return fmt.Sprintf("l := len(%s); %s", value, strings.Join(cmps, " || "))
		}
		return fmt.Sprintf("l := %s.RuneCountInString(string(%s)); %s", f.im.Add("unicode/utf8", "utf8"), value, strings.Join(cmps, " || "))
	case validvocab.Pattern:
		return fmt.Sprintf("!%s.MatchString(string(%s))", c.variable, value)
	case validvocab.Format:
		return fmt.Sprintf("!%s.CheckFormat(%s, string(%s))", f.runtime(), strconv.Quote(c.Format), value)
	}
	return "false"
}
