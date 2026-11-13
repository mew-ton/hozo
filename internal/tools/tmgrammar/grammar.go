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

// KeywordGroups は token.Keywords を docs/editors.md の scope ごとに分ける。
func KeywordGroups() []KeywordGroup {
	groups := []KeywordGroup{
		{Key: "declaration", Scope: "keyword.declaration.hozo"},
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
		all = append(all, kw{w, k})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].kind < all[j].kind })
	for _, k := range all {
		i := groupOf(k.kind)
		groups[i].Words = append(groups[i].Words, k.word)
	}
	return groups
}

// groupOf は予約語の Kind を KeywordGroups の添字に写す。
func groupOf(k token.Kind) int {
	switch {
	case k >= token.Namespace && k <= token.Bind: // 宣言の頭
		return 0
	case k >= token.Is && k <= token.Any: // 節の頭
		return 1
	case k == token.None, k == token.True, k == token.False: // true / false / none
		return 3
	case k >= token.Const && k <= token.Default: // 文の頭
		return 1
	case k >= token.StringType && k <= token.Single: // 核の型
		return 2
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

	var allWords []string
	for _, g := range groups {
		allWords = append(allWords, g.Words...)
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

		{Key: "omit", Rule: Rule{
			Comment: "omit の並びもフィールド名の位置（docs/grammar.md「model」の ModelDecl の omit FieldName { , FieldName }）。並びの中の予約語（核の型を含む）は予約語として塗らない。is の後ろの範囲の中でだけ使う",
			Begin:   `\b(omit)\b`,
			End:     `(?=;)`,
			BeginCaptures: Captures{
				"1": {Name: "keyword.control.hozo"},
			},
			Patterns: []Rule{
				{Include: "#comments"},
				{Name: "variable.other.property.hozo", Match: `\b` + anyKeyword + `\b`},
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
			Comment: "-> は遷移（docs/grammar.md「exception / effect」の Transition）、=> は match のアーム（同「配線」の MatchArm）",
			Name:    "keyword.operator.arrow.hozo",
			Match:   `->|=>`,
		}},

		// ---- デコレータ ----
		{Key: "external", Rule: Rule{
			Comment: "@external は核の印でデコレータではない（docs/grammar.md「予約語」）",
			Name:    "storage.modifier.hozo",
			Match:   `@external\b`,
		}},
		{Key: "decorator", Rule: Rule{
			Name:  "entity.name.function.decorator.hozo",
			Match: `@` + qualifiedName,
		}},

		// ---- 宣言の名前 ----
		{Key: "declarations", Rule: Rule{Patterns: []Rule{
			{
				Match: `\b(model|scalar|union|exception|classify|condition)\s+(` + ident + `)`,
				Captures: Captures{
					"1": {Name: "keyword.declaration.hozo"},
					"2": {Name: "entity.name.type.hozo"},
				},
			},
			{
				Match: `\b(op|effect|decorator)\s+(` + ident + `)`,
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
				Comment: "for の対象: field は for の後ろ（| で続く位置を含む）でだけ文脈キーワード",
				Begin:   `\b(for)\b`,
				End:     `(?=[^\s|A-Za-z0-9_])|(?=\b(?:throws|catches)\b)`,
				BeginCaptures: Captures{
					"1": {Name: "keyword.control.hozo"},
				},
				Patterns: []Rule{
					{Name: "keyword.other.hozo", Match: `\bfield\b`},
					{Include: "#keywords"},
				},
			},
			{
				Comment: "any の種類: variant は any の後ろでだけ文脈キーワード",
				Match:   `\b(any)\s+(variant)\b`,
				Captures: Captures{
					"1": {Name: "keyword.control.hozo"},
					"2": {Name: "keyword.other.hozo"},
				},
			},
			{
				Comment: "状態の操作: post / put / delete は on 状態. の後ろでだけ文脈キーワード",
				Match:   `\b(on)\s+(` + qualifiedName + `)\.(post|put|delete)\b`,
				Captures: Captures{
					"1": {Name: "keyword.control.hozo"},
					"3": {Name: "keyword.other.hozo"},
				},
			},
		}}},

		// ---- 型の位置 ----
		{Key: "type", Rule: Rule{
			Comment: "型の位置に現れるもの。核の型、< > の中、大文字で始まる名前",
			Patterns: []Rule{
				{Include: "#comments"},
				{Include: "#keywords.type"},
				{Include: "#generic"},
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
				Comment: ": の後ろ（フィールド、引数、返り値、状態、例外の値、遷移のケース）。値の位置の : も通るが、小文字で始まる名前は型にしない。any の種類もここに来る",
				Begin:   `:`,
				End:     `(?=[=;,(){}\[\]])|(?=\b(?:throws|effects|catches|prevents|keeps)\b)`,
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
				Comment: "is / extends / of の後ろ。is の元の model の後ろには omit の並びが続く",
				Begin:   `\b(is|extends|of)\b`,
				End:     `(?=[;{])`,
				BeginCaptures: Captures{
					"1": {Name: "keyword.control.hozo"},
				},
				Patterns: []Rule{{Include: "#omit"}, {Include: "#type"}},
			},
			{
				Comment: "throws / catches の後ろの例外の名前",
				Begin:   `\b(throws|catches)\b`,
				End:     `(?=[;{])|(?=\b(?:throws|catches|effects|prevents|keeps|recovers|halts|ignores)\b)`,
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
