package syntax

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/token"
)

// lexer は docs/grammar.md「字句」の字句解析器。ファイル全体を字句の列にする。
type lexer struct {
	file  source.FileID
	src   []byte
	pos   int
	toks  []token.Token
	diags []diag.Diagnostic
}

// lex はファイルを字句に分ける。最後は必ず EOF。読めなかった字句は Illegal にして SYNTAX を出す。
func lex(file source.FileID, src []byte) ([]token.Token, []diag.Diagnostic) {
	l := &lexer{file: file, src: src}
	for {
		l.next()
		if l.toks[len(l.toks)-1].Kind == token.EOF {
			break
		}
	}
	return l.toks, l.diags
}

func (l *lexer) span(start, end int) source.Span {
	return source.Span{File: l.file, Start: source.Offset(start), End: source.Offset(end)}
}

func (l *lexer) emit(kind token.Kind, start, end int) {
	l.toks = append(l.toks, token.Token{Kind: kind, Text: string(l.src[start:end]), Span: l.span(start, end)})
}

func (l *lexer) illegal(start, end int, format string, args ...any) {
	l.diags = append(l.diags, syntaxError(l.span(start, end), fmt.Sprintf(format, args...)))
	l.emit(token.Illegal, start, end)
}

func (l *lexer) peekByte(off int) byte {
	if l.pos+off < len(l.src) {
		return l.src[l.pos+off]
	}
	return 0
}

func (l *lexer) hasPrefix(s string) bool {
	return strings.HasPrefix(string(l.src[l.pos:min(l.pos+len(s), len(l.src))]), s)
}

// next は空白とコメントを飛ばし、字句を一つ足す。
func (l *lexer) next() {
	l.skipSpaceAndComments()
	start := l.pos
	if l.pos >= len(l.src) {
		l.emit(token.EOF, start, start)
		return
	}
	switch {
	case l.hasPrefix("/**"):
		l.lexDocComment()
	case isIdentStart(l.peekByte(0)):
		l.lexName()
	case isDigit(l.peekByte(0)), l.peekByte(0) == '-' && isDigit(l.peekByte(1)):
		l.lexNumber()
	case l.peekByte(0) == '"':
		l.lexString()
	default:
		l.lexSymbol()
	}
}

// skipSpaceAndComments は空白・改行・LineComment・BlockComment を捨てる（docs/grammar.md「空白とコメント」）。
// "/**" で始まるものは DocComment なので飛ばさない。
func (l *lexer) skipSpaceAndComments() {
	for l.pos < len(l.src) {
		switch {
		case isSpace(l.src[l.pos]):
			l.pos++
		case l.hasPrefix("//"):
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
		case l.hasPrefix("/**") && !l.hasPrefix("/**/"):
			return
		case l.hasPrefix("/*"):
			start := l.pos
			end := strings.Index(string(l.src[l.pos+2:]), "*/")
			if end < 0 {
				l.pos = len(l.src)
				l.diags = append(l.diags, syntaxError(l.span(start, l.pos), "ブロックコメントが閉じていない"))
				return
			}
			l.pos += 2 + end + 2
		default:
			return
		}
	}
}

// lexDocComment は "/**" から "*/" までを DocComment にする。Text は中身。
func (l *lexer) lexDocComment() {
	start := l.pos
	end := strings.Index(string(l.src[l.pos+3:]), "*/")
	if end < 0 {
		l.pos = len(l.src)
		l.illegal(start, l.pos, "ドキュメンテーションコメントが閉じていない")
		return
	}
	l.pos += 3 + end + 2
	l.toks = append(l.toks, token.Token{
		Kind: token.DocComment,
		Text: string(l.src[start+3 : l.pos-2]),
		Span: l.span(start, l.pos),
	})
}

// lexName は識別子・予約語・QualifiedName を読む（docs/grammar.md「識別子」「予約語」）。
// "." の直後に識別子の形が続く間はつなげて一つの QualifiedName にする（server.User、sessions.post）。
// 先頭が予約語なら、その予約語だけを字句にする。二つ目以降の要素は予約語でもつなげる（res.op）:
// 値のパスのフィールド名には予約語を書けるので（docs/grammar.md「識別子」の FieldName と Path）、
// 予約語を含む名前を受けるか（Path）誤りにするか（QualifiedName）は構文解析が位置で決める。
func (l *lexer) lexName() {
	start := l.pos
	segments := 0
	for {
		segStart := l.pos
		for l.pos < len(l.src) && isIdentChar(l.src[l.pos]) {
			l.pos++
		}
		if kind, isKeyword := token.Keywords[string(l.src[segStart:l.pos])]; isKeyword && segments == 0 {
			l.emit(kind, start, l.pos)
			return
		}
		segments++
		if l.peekByte(0) == '.' && isIdentStart(l.peekByte(1)) {
			l.pos++
			continue
		}
		break
	}
	if segments == 1 {
		l.emit(token.Ident, start, l.pos)
	} else {
		l.emit(token.QualifiedName, start, l.pos)
	}
}

// durationUnits は Duration の単位（docs/grammar.md「リテラル」「決めたこと」）。値はナノ秒。
var durationUnits = map[string]int64{
	"ms": 1_000_000,
	"s":  1_000_000_000,
	"m":  60 * 1_000_000_000,
	"h":  60 * 60 * 1_000_000_000,
	"d":  24 * 60 * 60 * 1_000_000_000,
}

// lexNumber は Int / Float / Duration を読む（docs/grammar.md「リテラル」）。
func (l *lexer) lexNumber() {
	start := l.pos
	negative := l.src[l.pos] == '-'
	if negative {
		l.pos++
	}
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
	}
	if l.peekByte(0) == '.' && isDigit(l.peekByte(1)) {
		l.pos++
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.pos++
		}
		l.checkTrailingLetters(start, token.Float)
		return
	}
	unitStart := l.pos
	for l.pos < len(l.src) && isIdentChar(l.src[l.pos]) {
		l.pos++
	}
	unit := string(l.src[unitStart:l.pos])
	if unit == "" {
		l.emit(token.Int, start, l.pos)
		return
	}
	if _, ok := durationUnits[unit]; ok {
		if negative {
			l.illegal(start, l.pos, "期間のリテラルに符号は付けられない: %s", l.src[start:l.pos])
			return
		}
		l.emit(token.Duration, start, l.pos)
		return
	}
	l.illegal(start, l.pos, "数値のリテラルではない: %s（期間の単位は ms / s / m / h / d）", l.src[start:l.pos])
}

// checkTrailingLetters は数字の直後に英字が続いていたら Illegal にする（1.5x など）。
func (l *lexer) checkTrailingLetters(start int, kind token.Kind) {
	if l.pos < len(l.src) && isIdentChar(l.src[l.pos]) {
		for l.pos < len(l.src) && isIdentChar(l.src[l.pos]) {
			l.pos++
		}
		l.illegal(start, l.pos, "数値のリテラルではない: %s", l.src[start:l.pos])
		return
	}
	l.emit(kind, start, l.pos)
}

// lexString は String を読む（docs/grammar.md「リテラル」）。エスケープの正しさもここで確かめる。
func (l *lexer) lexString() {
	start := l.pos
	l.pos++ // 開きの "
	for l.pos < len(l.src) {
		switch l.src[l.pos] {
		case '"':
			l.pos++
			if _, err := unquote(string(l.src[start:l.pos])); err != nil {
				l.illegal(start, l.pos, "%s", err.Error())
				return
			}
			l.emit(token.String, start, l.pos)
			return
		case '\\':
			l.pos += 2
		default:
			l.pos++
		}
	}
	l.pos = len(l.src)
	l.illegal(start, l.pos, "文字列が閉じていない")
}

// unquote は引用符で囲まれた字句からエスケープを戻した値を返す（docs/grammar.md「リテラル」の Escape）。
func unquote(text string) (string, error) {
	if len(text) < 2 || text[0] != '"' || text[len(text)-1] != '"' {
		return "", fmt.Errorf("文字列が閉じていない")
	}
	body := text[1 : len(text)-1]
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(body) {
			return "", fmt.Errorf("文字列の終わりにエスケープがある")
		}
		switch body[i] {
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'u':
			if i+1 >= len(body) || body[i+1] != '{' {
				return "", fmt.Errorf(`\u の後ろは {16進数} でなければならない`)
			}
			close := strings.IndexByte(body[i+2:], '}')
			if close < 0 {
				return "", fmt.Errorf(`\u{ が閉じていない`)
			}
			hex := body[i+2 : i+2+close]
			if hex == "" {
				return "", fmt.Errorf(`\u{} に 16 進数がない`)
			}
			v, err := strconv.ParseUint(hex, 16, 32)
			if err != nil || !utf8.ValidRune(rune(v)) {
				return "", fmt.Errorf(`\u{%s} は Unicode のコードポイントではない`, hex)
			}
			b.WriteRune(rune(v))
			i += 2 + close
		default:
			return "", fmt.Errorf(`エスケープ \%c は使えない（\" \\ \n \t \r \u{...} のどれか）`, body[i])
		}
	}
	return b.String(), nil
}

// parseDuration は Duration の字句をナノ秒にする。
func parseDuration(text string) (int64, error) {
	i := 0
	for i < len(text) && isDigit(text[i]) {
		i++
	}
	n, err := strconv.ParseInt(text[:i], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("期間の値が大きすぎる: %s", text)
	}
	unit, ok := durationUnits[text[i:]]
	if !ok {
		return 0, fmt.Errorf("期間の単位ではない: %s", text[i:])
	}
	if n != 0 && n > (1<<63-1)/unit {
		return 0, fmt.Errorf("期間の値が大きすぎる: %s", text)
	}
	return n * unit, nil
}

// lexSymbol は記号を読む（docs/grammar.md「記号」）。">>" は一つの記号にしない（list<list<string>> のため）。
func (l *lexer) lexSymbol() {
	start := l.pos
	c := l.src[l.pos]
	var kind token.Kind
	switch c {
	case '{':
		kind = token.LBrace
	case '}':
		kind = token.RBrace
	case '(':
		kind = token.LParen
	case ')':
		kind = token.RParen
	case '[':
		kind = token.LBracket
	case ']':
		kind = token.RBracket
	case '<':
		kind = token.Less
	case '>':
		kind = token.Greater
	case ':':
		kind = token.Colon
	case ';':
		kind = token.Semicolon
	case ',':
		kind = token.Comma
	case '.':
		if l.peekByte(1) == '.' && l.peekByte(2) == '.' {
			// "..." はデコレータの宣言の残りの引数でだけ使う（docs/grammar.md「記号」）
			l.pos += 3
			l.emit(token.Ellipsis, start, l.pos)
			return
		}
		kind = token.Dot
	case '?':
		kind = token.Question
	case '|':
		kind = token.Pipe
	case '@':
		kind = token.At
	case '=':
		if l.peekByte(1) == '>' {
			l.pos += 2
			l.emit(token.Arrow, start, l.pos)
			return
		}
		kind = token.Assign
	case '*':
		if l.peekByte(1) == '*' {
			l.pos += 2
			l.emit(token.StarStar, start, l.pos)
			return
		}
		kind = token.Star
	case '-':
		if l.peekByte(1) == '>' {
			l.pos += 2
			l.emit(token.ThinArrow, start, l.pos)
			return
		}
		l.pos++
		l.illegal(start, l.pos, "使えない文字: %q（「-」は負の数のリテラルと「->」にだけ使う）", '-')
		return
	default:
		r, size := utf8.DecodeRune(l.src[l.pos:])
		l.pos += size
		l.illegal(start, l.pos, "使えない文字: %q（識別子は ASCII の英数字と _。docs/grammar.md「決めたこと」）", r)
		return
	}
	l.pos++
	l.emit(kind, start, l.pos)
}

func isSpace(c byte) bool      { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func isDigit(c byte) bool      { return '0' <= c && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') }
func isIdentChar(c byte) bool  { return isIdentStart(c) || isDigit(c) }
