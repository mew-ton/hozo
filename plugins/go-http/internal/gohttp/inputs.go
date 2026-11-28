package gohttp

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/httpvocab"
)

// このファイルは、入力の model のフィールドの読み方（docs/plugin-go-http.md「入力の読み方」）。読み解くのは、語彙のプラグインと同じ
// ライブラリ plugin/httpvocab（docs/plugin-http.md「対象」）で、ここは、その結果を、出力を作る形に写すだけ。
// input は、デコレータの引数（と、フィールドの名前）だけを運び、出力は、そこからだけ作る（docs/requirements.md「出力を推論しない」）。

// inBody は、通信の上の場所の、本文。
const inBody = httpvocab.InBody

// input は、入力の model のフィールド一つの読み方。中身は、フィールドに付いたデコレータが言ったことだけ（httpvocab.Input の写し）。
type input struct {
	field *ir.Field
	// decorator は、場所のデコレータの FQN（http.query）。place は場所、whole は全体の形か。
	decorator string
	place     httpvocab.Place
	whole     bool
	// name は、項目の形の、項目の名前: デコレータの引数。省いていれば、フィールドの名前。全体の形では ""。
	name string
	// required は @http.required が付いているか。def は @http.default のリテラル（なければ nil）。
	required bool
	def      ir.Literal
	// where は、通信の上の場所（location）。
	where string
}

// inputsOf は、httpvocab が読み解いた読み方を、input の並びに写す（フィールドの順）。
func inputsOf(read []httpvocab.Input) []input {
	out := make([]input, len(read))
	for i := range read {
		x := &read[i]
		out[i] = input{field: x.Field, decorator: x.Decorator, place: x.Place, whole: x.Whole, name: x.Name,
			required: x.Required, def: x.Default, where: x.Location()}
	}
	return out
}

// location は、通信の上の場所（httpPlace の二つ目と、違反の path の先頭）: 項目の形は `<場所>.<名前>`、全体の形は `<場所>`。
func (in *input) location() string { return in.where }

// literalBody は、リテラルの、Go での書き方のうち、型を書かない部分（plugins/go/spec/go.hozo の Requirement.Wiring の「値」の表の literal の行）:
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
