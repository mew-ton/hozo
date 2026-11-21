package main

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/internal/syntax/token"
)

// Grammar は TextMate の文法の全体。フィールドの順序がそのまま JSON のキーの順序になる。
type Grammar struct {
	Information []string   `json:"information_for_contributors"`
	Name        string     `json:"name"`
	ScopeName   string     `json:"scopeName"`
	FileTypes   []string   `json:"fileTypes"`
	Patterns    []Rule     `json:"patterns"`
	Repository  Repository `json:"repository"`
}

// Rule は一つのパターン。使うフィールドだけが出力される。
type Rule struct {
	Comment       string   `json:"comment,omitempty"`
	Include       string   `json:"include,omitempty"`
	Name          string   `json:"name,omitempty"`
	Match         string   `json:"match,omitempty"`
	Begin         string   `json:"begin,omitempty"`
	End           string   `json:"end,omitempty"`
	Captures      Captures `json:"captures,omitempty"`
	BeginCaptures Captures `json:"beginCaptures,omitempty"`
	EndCaptures   Captures `json:"endCaptures,omitempty"`
	Patterns      []Rule   `json:"patterns,omitempty"`
}

// Captures は捕捉番号から scope への表。encoding/json がキーを昇順に並べる。
type Captures map[string]Capture

// Capture は捕捉した部分に付ける scope。
type Capture struct {
	Name string `json:"name"`
}

// Repository は名前付きのパターンの一覧。書いた順に出力する。
type Repository []Entry

// Entry は Repository の一項目。
type Entry struct {
	Key  string
	Rule Rule
}

// MarshalJSON は Repository を書いた順のオブジェクトにする。
func (r Repository) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, e := range r {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(e.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(e.Rule)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// 予約語のパターンの repository のキー。KeywordGroups の順に並ぶ。
// main_test.go がこの接頭辞で生成した部分を見分ける。
const keywordPrefix = "keywords."

// KeywordGroup は docs/editors.md「TextMate の文法」の表の、予約語の一行。
type KeywordGroup struct {
	Key   string // repository のキー（keywordPrefix の後ろ）
	Scope string
	Words []string // token.Keywords から集めた語。Kind の順（docs/grammar.md「予約語」の表の順）
}

// unwritable は、どの生成規則も使わない予約語（docs/grammar.md「予約語」: 書ける位置は FieldName と DecoratorName だけ）。
// 予約語としては塗らない: 正しい仕様では、フィールド名かデコレータ名としてしか現れない。
var unwritable = map[token.Kind]bool{token.Default: true}

// KeywordGroups は token.Keywords を docs/editors.md の scope ごとに分ける。unwritable の語は、どの組にも入れない。
func KeywordGroups() []KeywordGroup {
	groups := []KeywordGroup{
		{Key: "declaration", Scope: "keyword.declaration.hozo"},
		{Key: "modifier", Scope: "storage.modifier.hozo"},
		{Key: "control", Scope: "keyword.control.hozo"},
		{Key: "type", Scope: "support.type.primitive.hozo"},
		{Key: "constant", Scope: "constant.language.hozo"},
	}
	type kw struct {
		word string
		kind token.Kind
	}
	var all []kw
	for w, k := range token.Keywords {
		if unwritable[k] {
			continue
		}
		all = append(all, kw{w, k})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].kind < all[j].kind })
	for _, k := range all {
		i := groupOf(k.kind)
		groups[i].Words = append(groups[i].Words, k.word)
	}
	return groups
}

// caseOfRule は、デコレータの宣言の、分類のケースの絞り（docs/grammar.md「decorator の宣言」の CaseKind: case of …）。
// of の後ろの any は節の語、never は核の型、大文字で始まる名前（型か、分類の名前）は型の位置の名前。for の対象の並びの中と、
// any の後ろの、どちらにも置く（extends / of の後ろの規則は、; か { まで続くので、ここでは使わない）。
var caseOfRule = Rule{
	Match: `\b(case)\s+(of)\s+(?:(any)|(never)|(` + typeName + `))`,
	Captures: Captures{
		"1": {Name: "keyword.control.hozo"},
		"2": {Name: "keyword.control.hozo"},
		"3": {Name: "keyword.control.hozo"},
		"4": {Name: "support.type.primitive.hozo"},
		"5": {Name: "entity.name.type.hozo"},
	},
}

// asRule は、as の後ろの型の位置。ケースの as の後ろの型（docs/grammar.md「classify」の CaseTyping。, で次の CaseTyping が続く）と、
// 遷移のケース（同「exception / effect / read」の RowClause と Transition: as C.D、as else、as HasItems -> Empty。-> は、状態の遷移の矢印、
// any と else は予約語）。; か { まで続く。型の位置の規則と、effects の節の中の、どちらにも置く。
var asRule = Rule{
	Comment: "as の後ろの型の位置。ケースの as の後ろの型（docs/grammar.md「classify」の CaseTyping。, で次の CaseTyping が続く）と、遷移のケース（同「exception / effect / read」の RowClause と Transition: as C.D、as else、as HasItems -> Empty。-> は、状態の遷移の矢印、any と else は予約語）。; か { まで続く",
	Begin:   `\b(as)\b`,
	End:     `(?=[;{])`,
	BeginCaptures: Captures{
		"1": {Name: "keyword.control.hozo"},
	},
	Patterns: []Rule{
		{Include: "#arrow"},
		{Include: "#type"},
		{Include: "#keywords"},
	},
}

// groupOf は予約語の Kind を KeywordGroups の添字に写す。
func groupOf(k token.Kind) int {
	switch {
	case k >= token.Namespace && k <= token.Bind: // 宣言の頭
		return 0
	case k >= token.Export && k <= token.Override: // 修飾
		return 1
	case k >= token.Extends && k <= token.Case: // 節の頭
		return 2
	case k == token.None, k == token.True, k == token.False: // true / false / none
		return 4
	case k >= token.Val && k <= token.Throw: // 文の頭（none は上で拾っている）
		return 2
	case k >= token.StringType && k <= token.Single: // 核の型と形
		return 3
	}
	panic("tmgrammar: 予約語でない Kind: " + k.String())
}

// wordsPattern は語の選択を \b(a|b|c)\b の形にする。
func wordsPattern(words []string) string {
	return `\b(?:` + strings.Join(words, "|") + `)\b`
}

// 正規表現の部品。
const (
	ident         = `[A-Za-z_][A-Za-z0-9_]*`
	qualifiedName = ident + `(?:\.` + ident + `)*`
	// typeName は型の位置に置ける名前。最後の要素が大文字で始まるもの（§3.3 の命名規則。
	// docs/editors.md「TextMate の文法」の「残りは命名規則に頼り」）。核の型は keywords.type が受ける。
	typeName = `(?:` + ident + `\.)*[A-Z][A-Za-z0-9_]*`
)

// Build は文法を組み立てる。
func Build() Grammar {
	groups := KeywordGroups()

	var keywordEntries []Entry
	var keywordIncludes []Rule
	for _, g := range groups {
		key := keywordPrefix + g.Key
		keywordEntries = append(keywordEntries, Entry{Key: key, Rule: Rule{
			Name:  g.Scope,
			Match: wordsPattern(g.Words),
		}})
		keywordIncludes = append(keywordIncludes, Rule{Include: "#" + key})
	}

	// フィールド名の位置の予約語のパターンは、塗らない予約語（unwritable）も含めた、すべての予約語から作る
	var allWords []string
	for _, g := range groups {
		allWords = append(allWords, g.Words...)
	}
	for w, k := range token.Keywords {
		if unwritable[k] {
			allWords = append(allWords, w)
		}
	}
	// 予約語の語の選択（\b なし）。フィールド名の位置の予約語（docs/grammar.md「識別子」の FieldName）に使う。
	anyKeyword := `(?:` + strings.Join(allWords, "|") + `)`

	repo := Repository{
		// ---- フィールド名の位置の予約語 ----
		{Key: "field-names", Rule: Rule{
			Comment: "フィールド名の位置（: の前、. の後ろ）の予約語は予約語として塗らない（docs/grammar.md「識別子」の FieldName / Path）。予約語が : の前や . の後ろに来る構文はほかにない",
			Patterns: []Rule{
				{Name: "variable.other.property.hozo", Match: `\b` + anyKeyword + `\b(?=\s*:)`},
				{Name: "variable.other.property.hozo", Match: `(?<=\.)` + anyKeyword + `\b`},
			},
		}},

		{Key: "override-name", Rule: Rule{
			Comment: "override の後ろの一語もフィールド名の位置（docs/grammar.md「model」の Override）。予約語（核の型を含む）は予約語として塗らない。override そのものは修飾の予約語。予約語でないフィールド名には scope を付けない",
			Match:   `\b(override)\s+(` + anyKeyword + `)\b`,
			Captures: Captures{
				"1": {Name: "storage.modifier.hozo"},
				"2": {Name: "variable.other.property.hozo"},
			},
		}},

		// ---- コメント ----
		{Key: "comments", Rule: Rule{Patterns: []Rule{
			{Include: "#doc-comment"},
			{Include: "#block-comment"},
			{Include: "#line-comment"},
		}}},
		{Key: "doc-comment", Rule: Rule{
			Comment: "docs/grammar.md「ドキュメンテーションコメント」。タグは行の頭（* の後ろ）にだけ置ける",
			Name:    "comment.block.documentation.hozo",
			Begin:   `/\*\*(?!/)`,
			End:     `\*/`,
			Patterns: []Rule{
				{
					Match: `(?:^|(?<=\*))\s*(@param)(?:\s+(` + ident + `))?`,
					Captures: Captures{
						"1": {Name: "storage.type.class.jsdoc"},
						"2": {Name: "variable.other.jsdoc"},
					},
				},
				{
					Match: `(?:^|(?<=\*))\s*(@(?:throws|see))(?:\s+(` + qualifiedName + `))?`,
					Captures: Captures{
						"1": {Name: "storage.type.class.jsdoc"},
						"2": {Name: "variable.other.jsdoc"},
					},
				},
			},
		}},
		{Key: "block-comment", Rule: Rule{
			Name:  "comment.block.hozo",
			Begin: `/\*`,
			End:   `\*/`,
		}},
		{Key: "line-comment", Rule: Rule{
			Name:  "comment.line.double-slash.hozo",
			Match: `//.*$`,
		}},

		// ---- リテラル ----
		{Key: "string", Rule: Rule{
			Name:  "string.quoted.double.hozo",
			Begin: `"`,
			End:   `"`,
			Patterns: []Rule{{
				Name:  "constant.character.escape.hozo",
				Match: `\\(?:["\\ntr]|u\{[0-9A-Fa-f]+\})`,
			}},
		}},
		{Key: "number", Rule: Rule{
			Comment: "Int / Float / Duration（docs/grammar.md「リテラル」）。Duration は単位を含む",
			Name:    "constant.numeric.hozo",
			Match:   `(?<![A-Za-z0-9_.])(?:-?[0-9]+\.[0-9]+|[0-9]+(?:ms|s|m|h|d)|-?[0-9]+)\b`,
		}},

		// ---- 記号 ----
		{Key: "arrow", Rule: Rule{
			Comment: "-> は状態の遷移（docs/grammar.md「exception / effect / read」の Transition）、=> は match のアーム（同「配線」の MatchArm）",
			Name:    "keyword.operator.arrow.hozo",
			Match:   `->|=>`,
		}},

		{Key: "rest", Rule: Rule{
			Comment: "... はデコレータの宣言の残りの引数（docs/grammar.md「decorator の宣言」の RestParam）",
			Name:    "keyword.operator.rest.hozo",
			Match:   `\.\.\.`,
		}},

		// ---- デコレータ ----
		{Key: "external", Rule: Rule{
			Comment: "@external と @repeatable は核の印でデコレータではない（docs/grammar.md「予約語」）",
			Name:    "storage.modifier.hozo",
			Match:   `@(?:external|repeatable)\b`,
		}},
		{Key: "decorator", Rule: Rule{
			Comment: "@ から DecoratorRef まで。最後の要素は予約語でもよい（@db.list、@effects。docs/grammar.md「識別子」）。予約語のパターンより先に拾い、予約語として塗らない",
			Name:    "entity.name.function.decorator.hozo",
			Match:   `@` + qualifiedName,
		}},

		// ---- 宣言の名前 ----
		{Key: "declarations", Rule: Rule{Patterns: []Rule{
			{
				Match: `\b(model|scalar|union|exception|classify)\s+(` + ident + `)`,
				Captures: Captures{
					"1": {Name: "keyword.declaration.hozo"},
					"2": {Name: "entity.name.type.hozo"},
				},
			},
			{
				Comment: "デコレータの宣言の名前は DecoratorName で、予約語でもよい（decorator list(…)。docs/grammar.md「識別子」）。名前付きのデコレータの = の後ろ（DecoratorRef）は、適用と同じくデコレータの名前",
				Match:   `\b(decorator)\s+(` + ident + `)(?:\s*=\s*(` + qualifiedName + `))?`,
				Captures: Captures{
					"1": {Name: "keyword.declaration.hozo"},
					"2": {Name: "entity.name.function.hozo"},
					"3": {Name: "entity.name.function.decorator.hozo"},
				},
			},
			{
				Comment: "関数（op、read、効果）の名前は、fn の直後（docs/grammar.md「関数の宣言」）。fn の前の read と effect は、予約語のパターンが塗る",
				Match:   `\b(fn)\s+(` + ident + `)`,
				Captures: Captures{
					"1": {Name: "keyword.declaration.hozo"},
					"2": {Name: "entity.name.function.hozo"},
				},
			},
			{
				Match: `\b(state)\s+(` + ident + `)`,
				Captures: Captures{
					"1": {Name: "keyword.declaration.hozo"},
					"2": {Name: "variable.other.hozo"},
				},
			},
		}}},

		// ---- 文脈キーワード ----
		{Key: "contextual", Rule: Rule{Patterns: []Rule{
			{
				Comment: "for の対象: field は for の後ろ（, で続く位置を含む）でだけ文脈キーワード。対象の並びは、節の頭（throws / catches / reads / effects）か、, のほかの記号で終わる",
				Begin:   `\b(for)\b`,
				End:     `(?=[^\s,A-Za-z0-9_])|(?=\b(?:throws|catches|reads|effects)\b)`,
				BeginCaptures: Captures{
					"1": {Name: "keyword.control.hozo"},
				},
				Patterns: []Rule{
					{Name: "keyword.other.hozo", Match: `\bfield\b`},
					caseOfRule,
					{Include: "#keywords"},
				},
			},
			caseOfRule,
			{
				Comment: "any の種類: variant は any の後ろでだけ文脈キーワード",
				Match:   `\b(any)\s+(variant)\b`,
				Captures: Captures{
					"1": {Name: "keyword.control.hozo"},
					"2": {Name: "keyword.other.hozo"},
				},
			},
			{
				Comment: "状態の操作: post / put / delete は on 状態. の後ろでだけ文脈キーワード（デコレータの引数の絞り any effect on 状態.操作。docs/grammar.md「decorator の宣言」の AnyType）",
				Match:   `\b(on)\s+(` + qualifiedName + `)\.(post|put|delete)\b`,
				Captures: Captures{
					"1": {Name: "keyword.control.hozo"},
					"3": {Name: "keyword.other.hozo"},
				},
			},
			{
				Comment: "effects の節の「状態.操作」（docs/grammar.md「exception / effect / read」の BuiltinEffect と、「op」の EffectElem）: 要素の最後の . の後ろの post / put / delete。節は ; か { か次の節の頭で終わる。節の中の as（行の遷移と、状態の遷移）は、as の後ろの型の位置",
				Begin:   `\b(effects)\b`,
				End:     `(?=[;{])|(?=\b(?:throws|catches|effects|reads|prevents|keeps|recovers|halts|ignores)\b)`,
				BeginCaptures: Captures{
					"1": {Name: "keyword.control.hozo"},
				},
				Patterns: []Rule{
					{Include: "#comments"},
					asRule,
					{Name: "keyword.other.hozo", Match: `(?<=\.)(?:post|put|delete)\b(?!\s*\.)`},
				},
			},
			{
				Comment: "配線で直に呼ぶ「状態.操作(…)」（docs/grammar.md「配線」の Call）: . の後ろで、直後が ( の post / put / delete",
				Name:    "keyword.other.hozo",
				Match:   `(?<=\.)(?:post|put|delete)(?=\s*\()`,
			},
		}}},

		// ---- 型の位置 ----
		{Key: "type", Rule: Rule{
			Comment: "型の位置に現れるもの。核の型、< > の中、大文字で始まる名前、絞り込んだ型の on（docs/grammar.md「型」の OnType: T on C.K | C.K2。on の後ろのケースは、大文字で始まる名前の規則が拾い、| には scope を付けない）",
			Patterns: []Rule{
				{Include: "#comments"},
				{Include: "#keywords.type"},
				{Include: "#generic"},
				{Name: "keyword.control.hozo", Match: `\bon\b`},
				{Name: "entity.name.type.hozo", Match: `\b` + typeName + `\b`},
			},
		}},
		{Key: "generic", Rule: Rule{
			Begin:    `<`,
			End:      `>`,
			Patterns: []Rule{{Include: "#type"}},
		}},
		{Key: "type-positions", Rule: Rule{Patterns: []Rule{
			{
				Comment: ": の後ろ（フィールド、引数、返り値、状態、例外の値、遷移のケース、ケースの as）。値の位置の : も通るが、小文字で始まる名前は型にしない。any の種類もここに来る",
				Begin:   `:`,
				End:     `(?=[=;,(){}\[\]])|(?=\b(?:throws|effects|reads|catches|prevents|keeps)\b)`,
				Patterns: []Rule{
					{Include: "#arrow"},
					{Include: "#field-names"},
					{Include: "#contextual"},
					{Include: "#type"},
					{Include: "#string"},
					{Include: "#number"},
					{Include: "#keywords"},
				},
			},
			{
				Comment: "extends / of の後ろ（元の model、scalar の元の型、分類の対象）。of の後ろの op や状態の名前は小文字で始まるので、scope が付かない",
				Begin:   `\b(extends|of)\b`,
				End:     `(?=[;{])`,
				BeginCaptures: Captures{
					"1": {Name: "keyword.control.hozo"},
				},
				Patterns: []Rule{{Include: "#type"}},
			},
			asRule,
			{
				Comment: "match の by の後ろの分類の名前（修飾できる。docs/grammar.md「配線」の Match）。標準の分類 list / nullable は形の語として塗る",
				Begin:   `\b(by)\b`,
				End:     `(?=[;{])`,
				BeginCaptures: Captures{
					"1": {Name: "keyword.control.hozo"},
				},
				Patterns: []Rule{{Include: "#type"}},
			},
			{
				Comment: "throws / catches / throw の後ろの例外の名前。throw の引数の括弧（docs/grammar.md「配線」の Throw）の手前で終わり、引数は値の位置として塗る",
				Begin:   `\b(throws|catches|throw)\b`,
				End:     `(?=[;{(])|(?=\b(?:throws|catches|effects|reads|prevents|keeps|recovers|halts|ignores)\b)`,
				BeginCaptures: Captures{
					"1": {Name: "keyword.control.hozo"},
				},
				Patterns: []Rule{{Include: "#type"}},
			},
		}}},

		// ---- 予約語（token.Keywords から生成） ----
		{Key: "keywords", Rule: Rule{
			Comment:  "予約語。docs/grammar.md「予約語」の表の順。token.Keywords から生成",
			Patterns: keywordIncludes,
		}},
	}
	repo = append(repo, keywordEntries...)

	return Grammar{
		Information: []string{
			"このファイルは internal/tools/tmgrammar が生成する。手で編集せず、`go generate ./editors/...` で生成し直す。",
			"予約語のパターン（repository の keywords.*）は internal/syntax/token の Keywords から、それ以外は internal/tools/tmgrammar/grammar.go から作る。",
			"scope の決め方は docs/editors.md「TextMate の文法」。",
		},
		Name:      "Hozo",
		ScopeName: "source.hozo",
		FileTypes: []string{"hozo"},
		Patterns: []Rule{
			{Include: "#comments"},
			{Include: "#string"},
			{Include: "#override-name"},
			{Include: "#field-names"},
			{Include: "#external"},
			{Include: "#decorator"},
			{Include: "#declarations"},
			{Include: "#contextual"},
			{Include: "#type-positions"},
			{Include: "#generic"},
			{Include: "#keywords"},
			{Include: "#number"},
			{Include: "#arrow"},
			{Include: "#rest"},
		},
		Repository: repo,
	}
}

// Generate は文法を整形した JSON にする。
func Generate() ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(Build()); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
