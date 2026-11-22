package validvocab

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/runtime"
)

// このファイルは、値一つを制約で確かめる CheckLiteral（docs/plugin-valid.md「値一つを確かめる」）。境界のデコレータが補う値
// （`@http.default`、`@cli.default`）を、それを読むプラグインが、生成の前に確かめるのに使う（docs/plugin-http.md「補う値と制約」、
// docs/plugin-go-cli.md「補う値と制約」）。値が制約を通るかは、docs/plugin-valid.md「制約」の表の「通す値」。

// Mismatch は、値が合わなかった制約一つ。
type Mismatch struct {
	// Decorator は、制約のデコレータの FQN（`valid.min`）。
	Decorator string
	// Text は、制約の書き方（`@valid.min(1)`、`@valid.length(min: 1, max: 100)`）。診断のメッセージに書く。
	Text string
	// Scalar は、制約が scalar に付いたものなら、その scalar の FQN。フィールドに付いたものなら ""。
	Scalar string
}

// CheckLiteral は、リテラル lit が、フィールド field の制約に合うかを確かめ、合わなかった制約を返す。すべてに合えば nil。
//
// 確かめる制約は、valid の標準の制約（[Standard]）のうち、フィールドの型の scalar（と、その元の scalar）に付いたものと、
// フィールドに付いたもの。返す順は、元の scalar から自分の scalar へ、その後にフィールド。それぞれ、書かれた順。
// 省略可能なフィールドは、中の型で見る（docs/plugin-valid.md「値一つを確かめる」）。
//
// リテラルがフィールドの型に渡せることは、呼ぶ側が先に確かめる（http の HTTP-14）。ここで確かめられないものは、合わないとは
// 言わない: リテラルの型に当たらない制約と、解釈できない正規表現（VALID-1、VALID-3 が、別に知らせる）、標準でない valid の制約。
// p が nil なら、フィールドに付いた制約だけを確かめる。
func CheckLiteral(p *ir.Program, field *ir.Field, lit ir.Literal) []Mismatch {
	if field == nil || lit == nil {
		return nil
	}
	var out []Mismatch
	check := func(decorators []ir.Application, scalar string) {
		for i := range decorators {
			app := &decorators[i]
			if Standard(app.Decorator) && !satisfies(app, lit) {
				out = append(out, Mismatch{Decorator: app.Decorator, Text: describe(app), Scalar: scalar})
			}
		}
	}
	// 型の scalar を、自分から元へ辿って、元の方から確かめる
	var chain []*ir.Scalar
	t := field.Type
	if o, ok := t.(ir.OptionalType); ok {
		t = o.Of
	}
	seen := map[string]bool{}
	for p != nil {
		r, ok := t.(ir.RefType)
		if !ok || seen[r.Target] {
			break
		}
		seen[r.Target] = true
		s, ok := p.Lookup(r.Target).(*ir.Scalar)
		if !ok {
			break
		}
		chain = append(chain, s)
		t = s.Base
	}
	for i := len(chain) - 1; i >= 0; i-- {
		check(chain[i].Decorators, chain[i].Name)
	}
	check(field.Decorators, "")
	return out
}

// satisfies は、リテラル lit が、標準の制約の適用 app を通るか（docs/plugin-valid.md「制約」の表の「通す値」）。
// 確かめられない組（リテラルの型に当たらない制約、引数の形が合わない制約）は true。
func satisfies(app *ir.Application, lit ir.Literal) bool {
	switch app.Decorator {
	case Min, Max:
		bound, ok := argLiteral(app, "value")
		if !ok {
			return true
		}
		cmp, ok := compare(lit, bound)
		if !ok {
			return true
		}
		if app.Decorator == Min {
			return cmp >= 0
		}
		return cmp <= 0
	case Length:
		s, ok := lit.(ir.StringLiteral)
		if !ok {
			return true
		}
		l := int64(utf8.RuneCountInString(s.Value))
		if lo, ok := argLiteral(app, "min"); ok {
			if v, isInt := lo.(ir.IntLiteral); isInt && l < v.Value {
				return false
			}
		}
		if hi, ok := argLiteral(app, "max"); ok {
			if v, isInt := hi.(ir.IntLiteral); isInt && l > v.Value {
				return false
			}
		}
		return true
	case Pattern:
		s, ok := lit.(ir.StringLiteral)
		regex, _ := argLiteral(app, "regex")
		r, isString := regex.(ir.StringLiteral)
		if !ok || !isString {
			return true
		}
		re, err := regexp.Compile(r.Value)
		return err != nil || re.MatchString(s.Value)
	case Format:
		s, ok := lit.(ir.StringLiteral)
		name, _ := argLiteral(app, "name")
		n, isString := name.(ir.StringLiteral)
		if !ok || !isString {
			return true
		}
		return runtime.CheckFormat(n.Value, s.Value)
	}
	return true
}

// compare は、数のリテラル a と b を比べる（負、0、正）。どちらかが数でなければ false。二つとも整数なら、整数のまま比べる
// （2^53 を超える値を、倍精度に丸めない）。
func compare(a, b ir.Literal) (int, bool) {
	ai, aInt := a.(ir.IntLiteral)
	bi, bInt := b.(ir.IntLiteral)
	if aInt && bInt {
		switch {
		case ai.Value < bi.Value:
			return -1, true
		case ai.Value > bi.Value:
			return 1, true
		}
		return 0, true
	}
	af, aOK := number(a)
	bf, bOK := number(b)
	if !aOK || !bOK {
		return 0, false
	}
	switch {
	case af < bf:
		return -1, true
	case af > bf:
		return 1, true
	}
	return 0, true
}

// number は、数のリテラルの値。
func number(l ir.Literal) (float64, bool) {
	switch x := l.(type) {
	case ir.IntLiteral:
		return float64(x.Value), true
	case ir.FloatLiteral:
		return x.Value, true
	}
	return 0, false
}

// describe は、標準の制約の適用 app の書き方（`@valid.length(min: 1, max: 100)`）: 引数が一つのデコレータは値だけ、
// length は名前を付けて。
func describe(app *ir.Application) string {
	var args []string
	for _, a := range app.Args {
		l, ok := a.Value.(ir.LiteralValue)
		if !ok || l.Literal == nil {
			continue
		}
		text := literalText(l.Literal)
		if app.Decorator == Length {
			text = a.Param + ": " + text
		}
		args = append(args, text)
	}
	return "@" + app.Decorator + "(" + strings.Join(args, ", ") + ")"
}
