package gohttp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
	"github.com/mew-ton/hozo/plugin/govalid"
)

// このファイルは、入力の model のフィールドの読み方（docs/plugin-go-http.md「対象」「入力の読み方」）: デコレータが言ったことを
// input にまとめ、書いたものの整合（GOHTTP-3、GOHTTP-4、GOHTTP-12〜GOHTTP-15、GOHTTP-17〜GOHTTP-19）を確かめる。
// 型を見るのは、この整合の検査だけ: input は、デコレータの引数（と、フィールドの名前）だけを運び、出力は、そこからだけ作る
// （docs/requirements.md「出力を推論しない」）。

// place は、入力の model のフィールドを読む、通信の上の場所。
type place int

const (
	inHeader place = iota + 1
	inQuery
	inBody
)

// word は、場所の、通信の上の名前（httpPlace の二つ目と、文言の先頭。docs/plugin-go-http.md「入力の読み方」の末尾）。
func (p place) word() string {
	return [...]string{"", "header", "query", "body"}[p]
}

// label は、場所の、メッセージに書く名前。
func (p place) label() string {
	return [...]string{"", "ヘッダ", "クエリ", "本文"}[p]
}

// readers は、要求のどこから読むかを言う六つのデコレータ: 三つの場所ごとの、項目の形と、全体の形（whole）。
var readers = map[string]struct {
	place place
	whole bool
}{
	httpHeader:    {inHeader, false},
	httpQuery:     {inQuery, false},
	httpBody:      {inBody, false},
	httpHeaderObj: {inHeader, true},
	httpQueryObj:  {inQuery, true},
	httpBodyObj:   {inBody, true},
}

// input は、入力の model のフィールド一つの読み方。中身は、フィールドに付いたデコレータが言ったことだけ。
type input struct {
	field *ir.Field
	// decorator は、場所のデコレータの FQN（http.query）。place は場所、whole は全体の形か。
	decorator string
	place     place
	whole     bool
	// name は、項目の形の、項目の名前: デコレータの引数。省いていれば、フィールドの名前。全体の形では ""。
	name string
	// required は @http.required が付いているか。def は @http.default のリテラル（なければ nil）。
	required bool
	def      ir.Literal
}

// location は、通信の上の場所（httpPlace の二つ目と、違反の path の先頭）: 項目の形は `<場所>.<名前>`、全体の形は `<場所>`。
func (in *input) location() string {
	if in.whole {
		return in.place.word()
	}
	return in.place.word() + "." + in.name
}

// inputs は、入力の model m のフィールドの読み方（フィールドの順）。model ごとに一度だけ調べ、診断も一度だけ返す。
// 場所のデコレータをちょうど一つ持たないフィールド（GOHTTP-3）と、全体の形で model でないフィールド（GOHTTP-12）は、読み方に入れない。
func (g *generator) inputs(m *ir.Model) []input {
	if out, done := g.read[m.Name]; done {
		return out
	}
	out := []input{}
	for i := range m.Fields {
		f := &m.Fields[i]
		var found []string
		var in input
		item := false // 項目の形のデコレータを、一つでも持つか
		for j := range f.Decorators {
			app := &f.Decorators[j]
			r, ok := readers[app.Decorator]
			if !ok {
				continue
			}
			found = append(found, "@"+app.Decorator)
			in = input{field: f, decorator: app.Decorator, place: r.place, whole: r.whole}
			if !r.whole {
				item = true
				if in.name, ok = stringArg(app, "name"); !ok {
					in.name = f.Name
				}
			}
		}
		// GOHTTP-14、GOHTTP-15、GOHTTP-17〜GOHTTP-19
		in.required, in.def = g.checkMissing(m, f, item, "")
		// GOHTTP-3
		switch len(found) {
		case 0:
			g.errorf(codeHTTPPlace, m.Name, "入力の model %s のフィールド %s が、要求のどこから読むかを言うデコレータ（@%s / @%s / @%s / @%s / @%s / @%s）のどれも持たない（ちょうど一つ要る）",
				m.Name, f.Name, httpHeader, httpQuery, httpBody, httpHeaderObj, httpQueryObj, httpBodyObj)
			continue
		case 1:
		default:
			g.errorf(codeHTTPPlace, m.Name, "入力の model %s のフィールド %s が、%s を持つ（要求のどこから読むかを言うデコレータは、ちょうど一つ要る）", m.Name, f.Name, strings.Join(found, " と "))
			continue
		}
		if in.whole {
			// GOHTTP-12
			whole := g.wholeModel(f.Type)
			if whole == nil {
				g.errorf(codeHTTPWholeType, m.Name, "入力の model %s の @%s のフィールド %s の型 %s が、フィールドを持つ model でない（全体の形は、型が model のフィールドにだけ付けられる。省略可能な model、配列、インデックスの model には、付けられない）",
					m.Name, in.decorator, f.Name, golang.TypeDisplay(f.Type))
				continue
			}
			// 全体の形では、その model のフィールドの一つずつを、同じ場所の項目として見る
			for k := range whole.Fields {
				inner := &whole.Fields[k]
				// GOHTTP-4（本文の全体は、op の位置の GOHTTP-5）
				g.checkPlaceType(whole, inner, in.place, fmt.Sprintf("@%s で受け取る model %s のフィールド %s", in.decorator, whole.Name, inner.Name))
				// GOHTTP-14 ほか: 全体の形で受け取る model の中のフィールドには、付けられない
				g.checkMissing(whole, inner, false, fmt.Sprintf("model %s が、この model を @%s（全体の形）で受け取る", m.Name, in.decorator))
			}
		} else {
			// GOHTTP-4
			g.checkPlaceType(m, f, in.place, fmt.Sprintf("入力の model %s の @%s のフィールド %s", m.Name, in.decorator, f.Name))
		}
		out = append(out, in)
	}
	g.checkWholes(m, out)
	g.read[m.Name] = out
	return out
}

// wholeModel は、全体の形のフィールドの型 t の model。t が model への参照でなければ nil（省略可能な model、配列は、model ではない）。
func (g *generator) wholeModel(t ir.Type) *ir.Model {
	r, ok := t.(ir.RefType)
	if !ok {
		return nil
	}
	m, _ := g.p.Lookup(r.Target).(*ir.Model)
	if m != nil && m.Index != nil {
		// インデックスの model は、フィールドを持たないので、全体の形にできない（docs/plugin-go-http.md「対象」）
		return nil
	}
	return m
}

// checkPlaceType は GOHTTP-4: model m のフィールド f を、ヘッダかクエリの項目として読むとき、型（scalar は元の核の型。省略可能は
// 外して見る）が受け付けられるか（docs/plugin-go-http.md「対象」の表）。本文（JSON にできる型）は、op の位置の GOHTTP-5 が見る。
// subject は、メッセージに書く、そのフィールド。出力は変えない: 補助に、読めない型の読む先が渡らないことを、先に確かめるだけ。
func (g *generator) checkPlaceType(m *ir.Model, f *ir.Field, p place, subject string) {
	t := f.Type
	if o, ok := t.(ir.OptionalType); ok {
		t = o.Of
	}
	core, isCore := g.coreOf(t)
	switch p {
	case inHeader:
		if !isCore || core != ir.CoreString {
			g.errorf(codeHTTPFieldType, m.Name, "%s の型 %s は、ヘッダから読めない（string か、その省略可能）", subject, golang.TypeDisplay(f.Type))
		}
	case inQuery:
		if !isCore || core != ir.CoreString && core != ir.CoreInt && core != ir.CoreFloat && core != ir.CoreBoolean {
			g.errorf(codeHTTPFieldType, m.Name, "%s の型 %s は、クエリから読めない（string / int / float / boolean か、その省略可能）", subject, golang.TypeDisplay(f.Type))
		}
	}
}

// checkWholes は GOHTTP-13: 入力の model m の一つの場所に、全体の形が二つ以上ある、または、全体の形と項目の形の両方がある。
func (g *generator) checkWholes(m *ir.Model, inputs []input) {
	for _, p := range []place{inHeader, inQuery, inBody} {
		var wholes, items []string
		for i := range inputs {
			in := &inputs[i]
			if in.place != p {
				continue
			}
			if in.whole {
				wholes = append(wholes, in.field.Name)
			} else {
				items = append(items, in.field.Name)
			}
		}
		if len(wholes) > 1 {
			g.errorf(codeHTTPWholeMix, m.Name, "入力の model %s が、%sの全体の形を二つ以上持つ（フィールド %s。一つの場所に、高々一つ）",
				m.Name, p.label(), strings.Join(wholes, "、"))
		}
		if len(wholes) > 0 && len(items) > 0 {
			g.errorf(codeHTTPWholeMix, m.Name, "入力の model %s が、%sに、全体の形（フィールド %s）と項目の形（フィールド %s）の両方を持つ（同じ場所には、一緒に書けない）",
				m.Name, p.label(), strings.Join(wholes, "、"), strings.Join(items, "、"))
		}
	}
}

// checkMissing は、model m のフィールド f の、項目がないときの扱いを言うデコレータ（@http.required、@http.default）を読み、
// 四つの決まり（docs/plugin-go-http.md「対象」）と、補う値の制約（同「補う値と制約」）を確かめる。item は、f が項目の形のデコレータを
// 持つか。via は、f が、全体の形で受け取る model の中のフィールドのときの、メッセージに書く、その受け取り方（そのとき、item は false）。
// 戻り値は、デコレータが言ったこと: required（@http.required があるか）と、def（@http.default のリテラル。なければ nil）。
func (g *generator) checkMissing(m *ir.Model, f *ir.Field, item bool, via string) (required bool, def ir.Literal) {
	var found []string
	for i := range f.Decorators {
		app := &f.Decorators[i]
		switch app.Decorator {
		case httpRequired:
			required = true
		case httpDefault:
			def, _ = argLiteral(app, "value")
		default:
			continue
		}
		found = append(found, "@"+app.Decorator)
	}
	if len(found) == 0 {
		return false, nil
	}
	subject := fmt.Sprintf("model %s のフィールド %s", m.Name, f.Name)
	// GOHTTP-14
	if !item {
		why := fmt.Sprintf("項目の形（@%s / @%s / @%s）を持たない", httpHeader, httpQuery, httpBody)
		if via != "" {
			why = via
		}
		g.errorf(codeHTTPNoItem, m.Name, "%s に %s が付いているが、%s（全体の形の中の項目は、なければ、いつも零値のまま。扱いを言いたい項目は、項目の形で書く）",
			subject, strings.Join(found, " と "), why)
	}
	// GOHTTP-15
	if _, optional := f.Type.(ir.OptionalType); optional {
		g.errorf(codeHTTPOptional, m.Name, "%s に %s が付いているが、型 %s が省略可能（型は「なくてよい」と言っている。なければ誤りにするか、値を入れるなら、省略可能にしない）",
			subject, strings.Join(found, " と "), golang.TypeDisplay(f.Type))
	}
	// GOHTTP-17
	if required && def != nil {
		g.errorf(codeHTTPBoth, m.Name, "%s に、@%s と @%s の両方が付いている（項目がなければ誤りにするか、値を入れるかの、どちらか）", subject, httpRequired, httpDefault)
	}
	if def != nil {
		g.checkDefault(m, f, def)
	}
	return required, def
}

// checkDefault は、@http.default のリテラル lit を、model m のフィールド f の宣言と突き合わせる。補う値についての検査は、ここだけ:
//
//   - GOHTTP-18: リテラルが、フィールドの型（scalar は元の核の型）に渡せない。
//   - GOHTTP-19: 値が、フィールドの制約（フィールドに付いた @valid.* と、型の scalar に付いた @valid.*）に合わない。
//     validate の設定によらない。確かめるのは、go-cli の @cli.default と同じ govalid.CheckLiteral。
func (g *generator) checkDefault(m *ir.Model, f *ir.Field, lit ir.Literal) {
	// GOHTTP-18
	if !g.assignable(lit, f.Type) {
		g.errorf(codeHTTPDefault, m.Name, "model %s のフィールド %s の @%s のリテラル %s（%s）は、型 %s に渡せない",
			m.Name, f.Name, httpDefault, literalDisplay(lit), lit.Kind(), golang.TypeDisplay(f.Type))
		return
	}
	// GOHTTP-19
	for _, x := range govalid.CheckLiteral(g.p, f, lit) {
		owner := "フィールドの制約"
		if x.Scalar != "" {
			owner = "型の scalar " + x.Scalar + " の制約"
		}
		g.errorf(codeHTTPConstraint, m.Name, "model %s のフィールド %s の @%s の値 %s が、%s %s に合わない（補う値は、制約に合う値にする）",
			m.Name, f.Name, httpDefault, literalDisplay(lit), owner, x.Text)
	}
}

// assignable は、リテラル lit を、型 t の位置に渡せるか（docs/types.md「互換」の 10 と 2。scalar は、元の核の型で見る）:
// 文字列は string、整数は int か float、小数は float、真偽値は boolean、期間は Duration。
func (g *generator) assignable(lit ir.Literal, t ir.Type) bool {
	if o, ok := t.(ir.OptionalType); ok {
		t = o.Of
	}
	core, ok := g.coreOf(t)
	if !ok {
		return false
	}
	switch lit.(type) {
	case ir.StringLiteral:
		return core == ir.CoreString
	case ir.IntLiteral:
		return core == ir.CoreInt || core == ir.CoreFloat
	case ir.FloatLiteral:
		return core == ir.CoreFloat
	case ir.BooleanLiteral:
		return core == ir.CoreBoolean
	case ir.DurationLiteral:
		return core == ir.CoreDuration
	}
	return false
}

// literalDisplay は、メッセージに書く、リテラル。
func literalDisplay(lit ir.Literal) string {
	if d, ok := lit.(ir.DurationLiteral); ok {
		return d.Value.String()
	}
	return literalBody(lit)
}

// literalBody は、リテラルの、Go での書き方のうち、型を書かない部分（docs/plugin-go.md「値」の表の literal の行）:
// 文字列は strconv.Quote、整数は 10 進、小数は 'g'（. も e も含まなければ .0 を足す）、真偽値は true / false、期間はナノ秒。
func literalBody(lit ir.Literal) string {
	switch x := lit.(type) {
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
		return strconv.FormatInt(int64(x.Value), 10)
	}
	return "nil"
}
