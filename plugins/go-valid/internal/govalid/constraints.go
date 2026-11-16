package govalid

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは docs/plugin-go-valid.md「制約」の表: valid の標準の制約一つを、読んで（parse）、Go の条件にする（condition）。
// 値一つを制約で確かめるのは、ここではない（境界のデコレータが補う値は、それを読む相乗りが、公開の plugin/govalid の CheckLiteral で確かめる）。

// valid の標準の制約（デコレータの FQN）。
const (
	validMin     = "valid.min"
	validMax     = "valid.max"
	validLength  = "valid.length"
	validPattern = "valid.pattern"
	validFormat  = "valid.format"
)

// formats は runtime.CheckFormat が知っている名前（docs/plugin-go-valid.md「runtime」）。
var formats = []string{"email", "uri", "uuid", "date-time"}

func knownFormat(name string) bool {
	for _, f := range formats {
		if f == name {
			return true
		}
	}
	return false
}

// constraint は、scalar かフィールドに付いた、valid の標準の制約一つ。
type constraint struct {
	// decorator はデコレータの FQN（valid.min）。name は違反の Constraint に書く、デコレータの名前（min）。
	decorator, name string
	// core は、付いた先の元の型（核の型）。
	core ir.CoreName
	// bound は min / max の value。boundText は、その書かれたままの形（メッセージ用）。
	bound     float64
	boundText string
	// hasMin、hasMax、min、max は length の引数。
	hasMin, hasMax bool
	min, max       int64
	// regex は pattern の引数。variable は、その正規表現のパッケージの変数の名前（付いた先ごとに、描くときに決める）。
	regex, variable string
	// format は format の引数。
	format string
}

// parse は、適用 app（valid の標準の制約）を読む。core は付いた先の元の型で、coreOK が false なら、付いた先は核の型を元に
// 持たない（model、union、array、record など）。subject は、メッセージに書く、付いた先（「scalar type.Email」）。
// 「制約」の表に合わなければ、problem にその理由を返す（GOVALID-1 のメッセージ）。
func parse(app *ir.Application, core ir.CoreName, coreOK bool, subject string) (c constraint, problem string) {
	c = constraint{decorator: app.Decorator, name: short(app.Decorator), core: core}
	at := "@" + app.Decorator
	base := "核の型を元に持たない"
	if coreOK {
		base = "元の型が " + string(core)
	}
	wantString := func() string {
		if coreOK && core == ir.CoreString {
			return ""
		}
		return fmt.Sprintf("%s は、元の型が string の位置にだけ付けられるが、%s は%s", at, subject, base)
	}
	switch app.Decorator {
	case validMin, validMax:
		if !coreOK || core != ir.CoreInt && core != ir.CoreFloat {
			return c, fmt.Sprintf("%s は、元の型が int か float の位置にだけ付けられるが、%s は%s", at, subject, base)
		}
		lit, ok := literal(app, "value")
		if !ok {
			return c, fmt.Sprintf("%s の引数 value が、数のリテラルでない", at)
		}
		switch v := lit.(type) {
		case ir.IntLiteral:
			c.bound = float64(v.Value)
		case ir.FloatLiteral:
			c.bound = v.Value
		default:
			return c, fmt.Sprintf("%s の引数 value が、数のリテラルでない", at)
		}
		c.boundText = displayLiteral(lit)
		if core == ir.CoreInt && (c.bound != math.Trunc(c.bound) || math.Abs(c.bound) >= 1<<63) {
			return c, fmt.Sprintf("%s の value %s は整数でないので、元の型が int の %s には付けられない", at, c.boundText, subject)
		}
	case validLength:
		if p := wantString(); p != "" {
			return c, p
		}
		if lit, ok := literal(app, "min"); ok {
			v, isInt := lit.(ir.IntLiteral)
			if !isInt {
				return c, fmt.Sprintf("%s の引数 min が、整数のリテラルでない", at)
			}
			c.hasMin, c.min = true, v.Value
		}
		if lit, ok := literal(app, "max"); ok {
			v, isInt := lit.(ir.IntLiteral)
			if !isInt {
				return c, fmt.Sprintf("%s の引数 max が、整数のリテラルでない", at)
			}
			c.hasMax, c.max = true, v.Value
		}
		if !c.hasMin && !c.hasMax {
			return c, fmt.Sprintf("%s に、min も max もない（%s）", at, subject)
		}
	case validPattern:
		if p := wantString(); p != "" {
			return c, p
		}
		lit, _ := literal(app, "regex")
		s, ok := lit.(ir.StringLiteral)
		if !ok {
			return c, fmt.Sprintf("%s の引数 regex が、文字列のリテラルでない", at)
		}
		if _, err := regexp.Compile(s.Value); err != nil {
			return c, fmt.Sprintf("%s の正規表現 %s を、RE2 で解釈できない（%s）: %v", at, strconv.Quote(s.Value), subject, err)
		}
		c.regex = s.Value
	case validFormat:
		if p := wantString(); p != "" {
			return c, p
		}
		lit, _ := literal(app, "name")
		s, ok := lit.(ir.StringLiteral)
		if !ok {
			return c, fmt.Sprintf("%s の引数 name が、文字列のリテラルでない", at)
		}
		if !knownFormat(s.Value) {
			return c, fmt.Sprintf("%s の名前 %s を知らない（%s。知っているのは %s）", at, strconv.Quote(s.Value), subject, strings.Join(formats, " / "))
		}
		c.format = s.Value
	}
	return c, ""
}

// literal は、適用 app の引数 param のリテラル。書かれていない（省略可能な引数を省いた）か、リテラルでなければ false。
func literal(app *ir.Application, param string) (ir.Literal, bool) {
	for _, a := range app.Args {
		if a.Param == param {
			l, ok := a.Value.(ir.LiteralValue)
			return l.Literal, ok && l.Literal != nil
		}
	}
	return nil, false
}

// condition は、値の式 value（元の型に変換できる Go の式）が制約 c に違反するときに真になる Go の条件
// （docs/plugin-go-valid.md「制約」の表の「条件」）。length は、`l := …; <比較>` の形（if の初期化の文を含む）。
// f は、条件が使う import を足す先。
func (c constraint) condition(value string, f *file) string {
	switch c.decorator {
	case validMin, validMax:
		cmp := "<"
		if c.decorator == validMax {
			cmp = ">"
		}
		if c.core == ir.CoreInt {
			return fmt.Sprintf("int64(%s) %s %d", value, cmp, int64(c.bound))
		}
		return fmt.Sprintf("float64(%s) %s %s", value, cmp, c.boundText)
	case validLength:
		var cmps []string
		if c.hasMin {
			cmps = append(cmps, fmt.Sprintf("l < %d", c.min))
		}
		if c.hasMax {
			cmps = append(cmps, fmt.Sprintf("l > %d", c.max))
		}
		return fmt.Sprintf("l := %s.RuneCountInString(string(%s)); %s", f.im.Add("unicode/utf8", "utf8"), value, strings.Join(cmps, " || "))
	case validPattern:
		return fmt.Sprintf("!%s.MatchString(string(%s))", c.variable, value)
	case validFormat:
		return fmt.Sprintf("!%s.CheckFormat(%s, string(%s))", f.runtime(), strconv.Quote(c.format), value)
	}
	return "false"
}

// formatFloat は小数を Go のリテラルにする（docs/plugin-go.md「値」の小数と同じ書き方）。
func formatFloat(v float64) string {
	s := strconv.FormatFloat(v, 'g', -1, 64)
	if !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s
}

// displayLiteral は、メッセージと条件に書く、制約の引数のリテラル。
func displayLiteral(lit ir.Literal) string {
	switch x := lit.(type) {
	case ir.StringLiteral:
		return strconv.Quote(x.Value)
	case ir.IntLiteral:
		return strconv.FormatInt(x.Value, 10)
	case ir.FloatLiteral:
		return formatFloat(x.Value)
	case ir.BooleanLiteral:
		return strconv.FormatBool(x.Value)
	case ir.DurationLiteral:
		return x.Value.String()
	}
	return "?"
}

// short は FQN の最後のセグメント。
func short(fqn string) string {
	if i := strings.LastIndexByte(fqn, '.'); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}
