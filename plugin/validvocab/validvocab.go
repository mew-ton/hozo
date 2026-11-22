// Package validvocab は、標準の語彙 valid（docs/vocab.md「同梱する語彙」）の、言語に依らない読み解き（docs/plugin-valid.md）。
//
// 仕様（ir.Program）から、scalar と、model のフィールドに付いた標準の制約を読み、書いたものの整合の検査（VALID-*。同「診断」）を返す。
// 値一つを制約で確かめる関数（CheckLiteral。同「値一つを確かめる」）も、ここに置く。読むのは、仕様だけ: 単位にも、言語にも、依らない。
//
// 語彙のプラグイン valid（plugins/valid）は、ここの診断を、そのまま返す。制約を読むプラグイン（plugins/go-valid、plugins/go-cli、
// 語彙のプラグイン http の plugin/httpvocab）は、同じ読み解きの結果を使う（docs/decisions.md「決めたこと」428: 同じ読み解きを、
// 語彙のプラグインと、言語のプラグインで、分け合うのは、ライブラリで行う）。
package validvocab

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// Namespace は、この語彙の名前空間（マニフェストの [read] の namespaces。docs/plugin-valid.md「マニフェスト」）。
const Namespace = "valid"

// 標準の制約（デコレータの FQN。docs/plugin-valid.md「対象」）。
const (
	Min     = "valid.min"
	Max     = "valid.max"
	Length  = "valid.length"
	Pattern = "valid.pattern"
	Format  = "valid.format"
)

// 診断の ID（docs/plugin-valid.md「診断」）。
const (
	CodeTarget   = "VALID-1" // 制約が、付けた先に合わない
	CodeArgument = "VALID-2" // 制約の引数が、足りないか、先に合わない
	CodePattern  = "VALID-3" // pattern の regex を、RE2 で解釈できない
	CodeFormat   = "VALID-4" // format の name が、四つの形式のどれでもない
)

// NameCount は、配列のフィールドに付けた length の、違反の名前（docs/plugin-valid.md「制約」。ほかの違反の名前は、デコレータの名前）。
const NameCount = "count"

// standard は valid の標準の制約。
var standard = map[string]bool{Min: true, Max: true, Length: true, Pattern: true, Format: true}

// Standard は、decorator（汎用デコレータの FQN。Application.Decorator）が valid の標準の制約
// （min / max / length / pattern / format）か。標準でない valid の制約（ほかのプラグインの語彙が足したもの）は false。
func Standard(decorator string) bool { return standard[decorator] }

// Applied は、decorator が valid の語彙のデコレータ（`@valid.*`）か: 名前空間 valid に宣言されたもの。標準のものも、
// 利用者が足したものも数える。子の名前空間（valid.sub.x）のものは数えない。
func Applied(decorator string) bool {
	name, ok := strings.CutPrefix(decorator, Namespace+".")
	return ok && name != "" && !strings.Contains(name, ".")
}

// formats は、format の name（docs/plugin-valid.md「形式」の四つ）。
var formats = []string{"email", "uri", "uuid", "date-time"}

// Formats は、format の name として書ける名前（docs/plugin-valid.md「形式」の表の順）。
func Formats() []string { return append([]string(nil), formats...) }

func knownFormat(name string) bool {
	for _, f := range formats {
		if f == name {
			return true
		}
	}
	return false
}

// Constraint は、scalar かフィールドに付いた、valid の標準の制約一つを、読んだもの（docs/plugin-valid.md「制約」の表の一行）。
type Constraint struct {
	// Decorator は、デコレータの FQN（valid.min）。Name は、違反の名前（min。配列のフィールドに付けた length は count）。
	Decorator, Name string
	// Core は、付いた先の元の型（核の型）。配列のフィールドに付けた length（Count が true）では、意味を持たない。
	Core ir.CoreName
	// Bound は min / max の value。BoundText は、その書かれたままの形。
	Bound     float64
	BoundText string
	// HasMin、HasMax、Min、Max は length の引数。
	HasMin, HasMax bool
	Min, Max       int64
	// Regex は pattern の引数。
	Regex string
	// Format は format の引数。
	Format string
	// Count は、配列のフィールドに付けた length（要素の数に掛かる）。
	Count bool
}

// Problem は、制約の使い方の誤り一つ（docs/plugin-valid.md「診断」）: 診断の ID と、メッセージ。
type Problem struct{ Code, Message string }

// CoreOf は、型 t の元の型（docs/plugin-valid.md「制約」: scalar ならその継承を辿った核の型）。核の型を元に持たなければ false。
func CoreOf(p *ir.Program, t ir.Type) (ir.CoreName, bool) {
	for depth := 0; depth < 64 && p != nil; depth++ {
		switch x := t.(type) {
		case ir.CoreType:
			return x.Name, true
		case ir.RefType:
			s, ok := p.Lookup(x.Target).(*ir.Scalar)
			if !ok {
				return "", false
			}
			t = s.Base
		default:
			return "", false
		}
	}
	if c, ok := t.(ir.CoreType); ok {
		return c.Name, true
	}
	return "", false
}

// unwrap は、フィールドの型から、optional を外した型（制約が付く値の型）。
func unwrap(t ir.Type) ir.Type {
	for {
		switch x := t.(type) {
		case ir.OptionalType:
			t = x.Of
		case ir.CaseType:
			t = x.Of
		default:
			return t
		}
	}
}

// Scalar は、scalar s に付いた標準の制約（書かれた順）と、使い方の誤り。誤りのある制約は、cons に入らない。
// 標準でない valid の制約は、読まない（docs/plugin-valid.md「対象」）。
func Scalar(p *ir.Program, s *ir.Scalar) (cons []Constraint, problems []Problem) {
	core, ok := CoreOf(p, s.Base)
	subject := "scalar " + s.Name
	for i := range s.Decorators {
		app := &s.Decorators[i]
		if !Standard(app.Decorator) {
			continue
		}
		c, problem := parse(app, core, ok, subject)
		if problem != nil {
			problems = append(problems, *problem)
			continue
		}
		cons = append(cons, c)
	}
	return cons, problems
}

// Field は、model m のフィールド f に付いた標準の制約（書かれた順）と、使い方の誤り。誤りのある制約は、cons に入らない。
// 配列のフィールドに付けた length は、要素の数に掛かる（Count が true で、Name は count）。
// 標準でない valid の制約は、読まない（docs/plugin-valid.md「対象」）。
func Field(p *ir.Program, m *ir.Model, f *ir.Field) (cons []Constraint, problems []Problem) {
	subject := fmt.Sprintf("model %s のフィールド %s", m.Name, f.Name)
	value := unwrap(f.Type)
	core, ok := CoreOf(p, value)
	for i := range f.Decorators {
		app := &f.Decorators[i]
		if !Standard(app.Decorator) {
			continue
		}
		if _, isArray := value.(ir.ArrayType); isArray {
			// 配列のフィールドに付けられるのは、length だけ: 要素の数に掛かる
			if app.Decorator != Length {
				problems = append(problems, Problem{CodeTarget, fmt.Sprintf("@%s が、配列（%s）に付いている（配列のフィールドに付けられるのは @%s だけで、要素の数に掛かる。要素に掛けるなら、要素の型に付ける）", app.Decorator, subject, Length)})
				continue
			}
			c, problem := parse(app, ir.CoreString, true, subject)
			if problem != nil {
				problems = append(problems, *problem)
				continue
			}
			c.Count, c.Name, c.Core = true, NameCount, ""
			cons = append(cons, c)
			continue
		}
		if p != nil && p.IndexOf(value) != nil {
			problems = append(problems, Problem{CodeTarget, fmt.Sprintf("@%s が、インデックスの model そのもの（%s）に付いている（制約を付けられるのは、値の型）", app.Decorator, subject)})
			continue
		}
		c, problem := parse(app, core, ok, subject)
		if problem != nil {
			problems = append(problems, *problem)
			continue
		}
		cons = append(cons, c)
	}
	return cons, problems
}

// parse は、適用 app（valid の標準の制約）を読む。core は付いた先の元の型で、coreOK が false なら、付いた先は核の型を元に
// 持たない（model、union など）。subject は、メッセージに書く、付いた先（「scalar type.Email」）。
// docs/plugin-valid.md「制約」に合わなければ、その誤りを返す。
func parse(app *ir.Application, core ir.CoreName, coreOK bool, subject string) (Constraint, *Problem) {
	c := Constraint{Decorator: app.Decorator, Name: short(app.Decorator), Core: core}
	at := "@" + app.Decorator
	base := "核の型を元に持たない"
	if coreOK {
		base = "元の型が " + string(core)
	}
	wantString := func() *Problem {
		if coreOK && core == ir.CoreString {
			return nil
		}
		return &Problem{CodeTarget, fmt.Sprintf("%s は、元の型が string の位置にだけ付けられるが、%s は%s", at, subject, base)}
	}
	switch app.Decorator {
	case Min, Max:
		if !coreOK || core != ir.CoreInt && core != ir.CoreFloat {
			return c, &Problem{CodeTarget, fmt.Sprintf("%s は、元の型が int か float の位置にだけ付けられるが、%s は%s", at, subject, base)}
		}
		lit, ok := argLiteral(app, "value")
		if !ok {
			return c, &Problem{CodeArgument, fmt.Sprintf("%s の引数 value が、数のリテラルでない", at)}
		}
		switch v := lit.(type) {
		case ir.IntLiteral:
			c.Bound = float64(v.Value)
		case ir.FloatLiteral:
			c.Bound = v.Value
		default:
			return c, &Problem{CodeArgument, fmt.Sprintf("%s の引数 value が、数のリテラルでない", at)}
		}
		c.BoundText = literalText(lit)
		if core == ir.CoreInt && (c.Bound != math.Trunc(c.Bound) || math.Abs(c.Bound) >= 1<<63) {
			return c, &Problem{CodeArgument, fmt.Sprintf("%s の value %s は整数でないので、元の型が int の %s には付けられない", at, c.BoundText, subject)}
		}
	case Length:
		if p := wantString(); p != nil {
			return c, p
		}
		if lit, ok := argLiteral(app, "min"); ok {
			v, isInt := lit.(ir.IntLiteral)
			if !isInt {
				return c, &Problem{CodeArgument, fmt.Sprintf("%s の引数 min が、整数のリテラルでない", at)}
			}
			c.HasMin, c.Min = true, v.Value
		}
		if lit, ok := argLiteral(app, "max"); ok {
			v, isInt := lit.(ir.IntLiteral)
			if !isInt {
				return c, &Problem{CodeArgument, fmt.Sprintf("%s の引数 max が、整数のリテラルでない", at)}
			}
			c.HasMax, c.Max = true, v.Value
		}
		if !c.HasMin && !c.HasMax {
			return c, &Problem{CodeArgument, fmt.Sprintf("%s に、min も max もない（%s）", at, subject)}
		}
	case Pattern:
		if p := wantString(); p != nil {
			return c, p
		}
		lit, _ := argLiteral(app, "regex")
		s, ok := lit.(ir.StringLiteral)
		if !ok {
			return c, &Problem{CodeArgument, fmt.Sprintf("%s の引数 regex が、文字列のリテラルでない", at)}
		}
		if _, err := regexp.Compile(s.Value); err != nil {
			return c, &Problem{CodePattern, fmt.Sprintf("%s の正規表現 %s を、RE2 で解釈できない（%s）: %v", at, strconv.Quote(s.Value), subject, err)}
		}
		c.Regex = s.Value
	case Format:
		if p := wantString(); p != nil {
			return c, p
		}
		lit, _ := argLiteral(app, "name")
		s, ok := lit.(ir.StringLiteral)
		if !ok {
			return c, &Problem{CodeArgument, fmt.Sprintf("%s の引数 name が、文字列のリテラルでない", at)}
		}
		if !knownFormat(s.Value) {
			return c, &Problem{CodeFormat, fmt.Sprintf("%s の名前 %s を知らない（%s。知っているのは %s）", at, strconv.Quote(s.Value), subject, strings.Join(formats, " / "))}
		}
		c.Format = s.Value
	}
	return c, nil
}

// Check は、仕様 p の、すべての名前空間の scalar と model のフィールドに付いた標準の制約を読み解いた診断（VALID-*。
// docs/plugin-valid.md「診断」）。順は、名前空間の IR の順、その中は、宣言順、フィールドの順、制約の書かれた順。
// 位置は、制約の付いた scalar か、フィールドを持つ model の FQN。同じ診断は、一度だけ。
func Check(p *ir.Program) []plugin.Diagnostic {
	if p == nil {
		return nil
	}
	var out []plugin.Diagnostic
	seen := map[plugin.Diagnostic]bool{}
	add := func(at string, problems []Problem) {
		for _, x := range problems {
			d := plugin.Diagnostic{Code: x.Code, At: at, Message: x.Message}
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	for i := range p.Namespaces {
		for _, d := range p.Namespaces[i].Decls {
			switch x := d.(type) {
			case *ir.Scalar:
				_, problems := Scalar(p, x)
				add(x.Name, problems)
			case *ir.Model:
				for j := range x.Fields {
					_, problems := Field(p, x, &x.Fields[j])
					add(x.Name, problems)
				}
			}
		}
	}
	return out
}

// argLiteral は、適用 app の引数 param のリテラル。書かれていない（省略可能な引数を省いた）か、リテラルでなければ false。
func argLiteral(app *ir.Application, param string) (ir.Literal, bool) {
	for _, a := range app.Args {
		if a.Param == param {
			l, ok := a.Value.(ir.LiteralValue)
			return l.Literal, ok && l.Literal != nil
		}
	}
	return nil, false
}

// literalText は、リテラルの書き方（メッセージに書く。小数は、小数点を必ず持つ）。
func literalText(l ir.Literal) string {
	switch x := l.(type) {
	case ir.StringLiteral:
		return strconv.Quote(x.Value)
	case ir.IntLiteral:
		return strconv.FormatInt(x.Value, 10)
	case ir.FloatLiteral:
		s := strconv.FormatFloat(x.Value, 'g', -1, 64)
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		return s
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
