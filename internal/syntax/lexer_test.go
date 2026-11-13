package syntax

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/token"
)

func lexString(t *testing.T, src string) ([]token.Token, []string) {
	t.Helper()
	toks, diags := lex(source.FileID(0), []byte(src))
	var msgs []string
	for _, d := range diags {
		if d.Code != "SYNTAX" {
			t.Errorf("Code = %q, want SYNTAX", d.Code)
		}
		msgs = append(msgs, d.Message)
	}
	return toks, msgs
}

func kindsOf(toks []token.Token) []token.Kind {
	kinds := make([]token.Kind, 0, len(toks))
	for _, t := range toks {
		kinds = append(kinds, t.Kind)
	}
	return kinds
}

func textsOf(toks []token.Token) string {
	var parts []string
	for _, t := range toks[:len(toks)-1] { // EOF を除く
		parts = append(parts, t.Text)
	}
	return strings.Join(parts, " ")
}

func sameKinds(a, b []token.Kind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLexTokens(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []token.Kind
	}{
		{"empty", "", []token.Kind{token.EOF}},
		{"keywords", "namespace model op throws", []token.Kind{token.Namespace, token.Model, token.Op, token.Throws, token.EOF}},
		{"core types and state shapes", "string boolean int float Time Duration void Record list nullable single",
			[]token.Kind{token.StringType, token.BooleanType, token.IntType, token.FloatType, token.TimeType, token.DurationType, token.Void, token.Record, token.List, token.Nullable, token.Single, token.EOF}},
		{"bool literals", "true false", []token.Kind{token.True, token.False, token.EOF}},
		{"ident", "foo _bar baz9", []token.Kind{token.Ident, token.Ident, token.Ident, token.EOF}},
		{"contextual words are idents", "field variant post put delete", []token.Kind{token.Ident, token.Ident, token.Ident, token.Ident, token.Ident, token.EOF}},
		{"qualified name", "server.User a.b.c", []token.Kind{token.QualifiedName, token.QualifiedName, token.EOF}},
		{"qualified with contextual", "sessions.post", []token.Kind{token.QualifiedName, token.EOF}},
		{"name then star", "server.* server.**", []token.Kind{token.Ident, token.Dot, token.Star, token.Ident, token.Dot, token.StarStar, token.EOF}},
		// 二つ目以降の要素は予約語でもつなげる（値のパスのフィールド名。docs/grammar.md「識別子」の Path）。
		// 名前として受けるかどうかは構文解析が位置で決める。先頭が予約語なら、その予約語だけが字句。
		{"name then keyword", "a.string", []token.Kind{token.QualifiedName, token.EOF}},
		{"path with keywords", "res.op.state.x res.match", []token.Kind{token.QualifiedName, token.QualifiedName, token.EOF}},
		{"path then keyword", "x.by by", []token.Kind{token.QualifiedName, token.By, token.EOF}},
		{"keyword then dot", "string.a", []token.Kind{token.StringType, token.Dot, token.Ident, token.EOF}},
		{"keyword then dot keyword", "op.state", []token.Kind{token.Op, token.Dot, token.State, token.EOF}},
		{"symbols", "{ } ( ) [ ] < > : ; , . = ? | @ * ** => ->",
			[]token.Kind{token.LBrace, token.RBrace, token.LParen, token.RParen, token.LBracket, token.RBracket, token.Less, token.Greater,
				token.Colon, token.Semicolon, token.Comma, token.Dot, token.Assign, token.Question, token.Pipe, token.At, token.Star, token.StarStar, token.Arrow, token.ThinArrow, token.EOF}},
		{"no >> token", "Record<string, string[]>>", []token.Kind{token.Record, token.Less, token.StringType, token.Comma, token.StringType, token.LBracket, token.RBracket, token.Greater, token.Greater, token.EOF}},
		{"int", "0 42 -7", []token.Kind{token.Int, token.Int, token.Int, token.EOF}},
		{"float", "1.5 -0.25", []token.Kind{token.Float, token.Float, token.EOF}},
		{"duration", "5m 30s 100ms 2h 1d", []token.Kind{token.Duration, token.Duration, token.Duration, token.Duration, token.Duration, token.EOF}},
		{"string", `"a" "" "\"q\" \\ \n \t \r \u{3042}"`, []token.Kind{token.String, token.String, token.String, token.EOF}},
		{"string with newline", "\"a\nb\"", []token.Kind{token.String, token.EOF}},
		{"line comment", "a // c\nb", []token.Kind{token.Ident, token.Ident, token.EOF}},
		{"block comment", "a /* c\n d */ b", []token.Kind{token.Ident, token.Ident, token.EOF}},
		{"empty block comment", "a /**/ b", []token.Kind{token.Ident, token.Ident, token.EOF}},
		{"doc comment", "/** d */ model", []token.Kind{token.DocComment, token.Model, token.EOF}},
		{"empty doc comment", "/***/ model", []token.Kind{token.DocComment, token.Model, token.EOF}},
		{"arrow and assign", "= => =", []token.Kind{token.Assign, token.Arrow, token.Assign, token.EOF}},
		{"thin arrow", "A -> B A->B", []token.Kind{token.Ident, token.ThinArrow, token.Ident, token.Ident, token.ThinArrow, token.Ident, token.EOF}},
		{"thin arrow and negative int", "-> -1 ->", []token.Kind{token.ThinArrow, token.Int, token.ThinArrow, token.EOF}},
		{"at ident", "@db @http.status", []token.Kind{token.At, token.Ident, token.At, token.QualifiedName, token.EOF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, msgs := lexString(t, tt.src)
			if len(msgs) > 0 {
				t.Errorf("診断: %v", msgs)
			}
			if got := kindsOf(toks); !sameKinds(got, tt.want) {
				t.Errorf("kinds = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLexTextAndSpan(t *testing.T) {
	src := "model  server.User\n5m"
	toks, _ := lexString(t, src)
	if got := textsOf(toks); got != "model server.User 5m" {
		t.Errorf("texts = %q", got)
	}
	qn := toks[1]
	if qn.Span.Start != 7 || qn.Span.End != 18 {
		t.Errorf("QualifiedName Span = %+v", qn.Span)
	}
	dur := toks[2]
	if dur.Span.Start != 19 || dur.Span.End != 21 {
		t.Errorf("Duration Span = %+v", dur.Span)
	}
	eof := toks[3]
	if eof.Span.Start != 21 || eof.Span.End != 21 {
		t.Errorf("EOF Span = %+v", eof.Span)
	}
}

func TestLexDocCommentText(t *testing.T) {
	toks, _ := lexString(t, "/** 要約\n * @see x */")
	if toks[0].Text != " 要約\n * @see x " {
		t.Errorf("DocComment Text = %q", toks[0].Text)
	}
}

func TestLexErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		kinds   []token.Kind
		message string
	}{
		{"unterminated string", `"abc`, []token.Kind{token.Illegal, token.EOF}, "文字列が閉じていない"},
		{"bad escape", `"\x"`, []token.Kind{token.Illegal, token.EOF}, `エスケープ \x は使えない`},
		{"bad unicode escape", `"\u{zz}"`, []token.Kind{token.Illegal, token.EOF}, "Unicode のコードポイントではない"},
		{"unicode escape without brace", `"\u0041"`, []token.Kind{token.Illegal, token.EOF}, `\u の後ろは {16進数}`},
		{"unterminated block comment", "a /* b", []token.Kind{token.Ident, token.EOF}, "ブロックコメントが閉じていない"},
		{"unterminated doc comment", "/** b", []token.Kind{token.Illegal, token.EOF}, "ドキュメンテーションコメントが閉じていない"},
		{"bad duration unit", "5w", []token.Kind{token.Illegal, token.EOF}, "数値のリテラルではない"},
		{"negative duration", "-5m", []token.Kind{token.Illegal, token.EOF}, "符号は付けられない"},
		{"float with letters", "1.5x", []token.Kind{token.Illegal, token.EOF}, "数値のリテラルではない"},
		{"non ascii ident", "日本", []token.Kind{token.Illegal, token.Illegal, token.EOF}, "使えない文字"},
		{"stray char", "a # b", []token.Kind{token.Ident, token.Illegal, token.Ident, token.EOF}, "使えない文字"},
		{"lone minus", "- 5", []token.Kind{token.Illegal, token.Int, token.EOF}, "使えない文字"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, msgs := lexString(t, tt.src)
			if got := kindsOf(toks); !sameKinds(got, tt.kinds) {
				t.Errorf("kinds = %v, want %v", got, tt.kinds)
			}
			if len(msgs) == 0 {
				t.Fatalf("診断がない")
			}
			if !strings.Contains(msgs[0], tt.message) {
				t.Errorf("message = %q, want contains %q", msgs[0], tt.message)
			}
		})
	}
}

func TestUnquote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{`""`, ""},
		{`"abc"`, "abc"},
		{`"a\"b"`, `a"b`},
		{`"a\\b"`, `a\b`},
		{`"\n\t\r"`, "\n\t\r"},
		{`"\u{3042}\u{41}"`, "あA"},
		{"\"あ\"", "あ"},
	}
	for _, tt := range tests {
		got, err := unquote(tt.in)
		if err != nil {
			t.Errorf("unquote(%s): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("unquote(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
	for _, bad := range []string{`"\u{}"`, `"\u{110000}"`, `"\u{41"`, `"\q"`, `"abc`, `"\"`} {
		if _, err := unquote(bad); err == nil {
			t.Errorf("unquote(%s) に誤りがない", bad)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"1ms", 1_000_000},
		{"30s", 30_000_000_000},
		{"5m", 300_000_000_000},
		{"2h", 7_200_000_000_000},
		{"1d", 86_400_000_000_000},
		{"0d", 0},
	}
	for _, tt := range tests {
		got, err := parseDuration(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("parseDuration(%s) = %d, %v; want %d", tt.in, got, err, tt.want)
		}
	}
	for _, bad := range []string{"99999999999999999999ms", "9223372036854775807d", "5w"} {
		if _, err := parseDuration(bad); err == nil {
			t.Errorf("parseDuration(%s) に誤りがない", bad)
		}
	}
}
