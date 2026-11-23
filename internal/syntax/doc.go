package syntax

import (
	"strings"

	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/internal/syntax/token"
)

// parseDocComment は DocComment の中身を本文とタグに分ける（docs/grammar.md「ドキュメンテーションコメント」）。
//
//   - 各行の先頭の空白と "*" と、その後ろの空白一つを落としてから読む。行末の空白も落とす。
//   - 言語のタグは @param / @throws / @see。"@" と、"." を含む名前で始まり、その後ろが空白か行の終わりの行は、
//     デコレータのタグ（DocApply）。ほかの "@" で始まる行は本文。
//   - タグの行より後ろの、タグでない行は、直前のタグの説明の続きとして扱う（文法は「本文の行」の後に「タグの行」が並ぶとだけ決めている）。
//   - 名前の形が合わないタグの行（"@param" だけ、"@see 1" など）は SYNTAX にし、本文として残す。
func (p *parser) parseDocComment(tok token.Token) *ast.DocComment {
	dc := &ast.DocComment{Span: tok.Span, Raw: tok.Text}
	var body []string
	// 中身は "/**" の 3 バイト後ろから始まる。
	offset := int(tok.Span.Start) + 3
	for _, raw := range strings.SplitAfter(tok.Text, "\n") {
		lineStart := offset
		offset += len(raw)
		line := strings.TrimRight(raw, " \t\r\n")
		text, trimmed := stripDocPrefix(line)
		span := source.Span{File: p.file, Start: source.Offset(lineStart + trimmed), End: source.Offset(lineStart + len(line))}
		if tag, ok := p.parseDocTag(text, span); ok {
			dc.Tags = append(dc.Tags, *tag)
			continue
		}
		if len(dc.Tags) > 0 {
			if strings.TrimSpace(text) != "" {
				last := &dc.Tags[len(dc.Tags)-1]
				last.Text = strings.TrimSpace(last.Text + "\n" + strings.TrimSpace(text))
				last.Span.End = span.End
			}
			continue
		}
		body = append(body, text)
	}
	dc.Body = strings.Trim(strings.Join(body, "\n"), "\n")
	return dc
}

// stripDocPrefix は行の先頭の空白と "*" と、その後ろの空白一つを落とす。落とした長さも返す。
func stripDocPrefix(line string) (string, int) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i < len(line) && line[i] == '*' {
		i++
		if i < len(line) && line[i] == ' ' {
			i++
		}
	}
	return line[i:], i
}

// parseDocTag は "@param 名前 説明" / "@throws 名前 説明" / "@see 名前" の行を読む。
// タグの行でなければ ok は false。
func (p *parser) parseDocTag(text string, span source.Span) (*ast.DocTag, bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "@") {
		return nil, false
	}
	var kind ast.DocTagKind
	switch fields[0] {
	case "@param":
		kind = ast.DocParam
	case "@throws":
		kind = ast.DocThrows
	case "@see":
		kind = ast.DocSee
	default:
		return p.parseDocApply(text, fields[0], span)
	}
	if len(fields) < 2 {
		p.errorAt(span, fields[0]+" の後ろに名前がない")
		return nil, false
	}
	// 名前は、タグの語の後ろから探す（"@param r" の r を、"@param" の中の r と取り違えない）
	tagEnd := strings.Index(text, fields[0]) + len(fields[0])
	nameAt := tagEnd + strings.Index(text[tagEnd:], fields[1])
	nameStart := int(span.Start) + nameAt
	// @see の名前は QualifiedName か DecoratorRef で、最後の要素にだけ予約語を書ける（@see db.list。
	// docs/grammar.md「ドキュメンテーションコメント」）。
	name, ok := qualifiedNameOf(fields[1], p.file, nameStart, kind == ast.DocSee)
	if !ok || (kind == ast.DocParam && len(name.Segments) != 1) {
		p.errorAt(span, fields[0]+" の名前の形が正しくない: "+fields[1])
		return nil, false
	}
	tag := &ast.DocTag{Span: span, Kind: kind, Name: name}
	rest := strings.TrimSpace(text[nameAt+len(fields[1]):])
	if kind == ast.DocSee {
		if rest != "" {
			p.errorAt(span, "@see は名前だけを書く")
		}
	} else {
		tag.Text = rest
	}
	return tag, true
}

// parseDocApply は、デコレータのタグ "@接頭語.名前 文章" の行を読む（docs/grammar.md「ドキュメンテーションコメント」）。
// head は、行の最初の語（"@adr.context"）。"." を含む DecoratorRef でなければ、タグではない（本文）。
// 文章がなければ SYNTAX にして、本文として残す。
func (p *parser) parseDocApply(text, head string, span source.Span) (*ast.DocTag, bool) {
	if !strings.Contains(head, ".") {
		return nil, false
	}
	at := strings.Index(text, head)
	name, ok := qualifiedNameOf(head[1:], p.file, int(span.Start)+at+1, true)
	if !ok {
		return nil, false
	}
	rest := strings.TrimSpace(text[at+len(head):])
	if rest == "" {
		p.errorAt(span, head+" の後ろに文章がない（説明のコメントの中のタグは、文章を一つ渡す。引数のないデコレータは、宣言の前に "+head+" と書く。docs/grammar.md「ドキュメンテーションコメント」）")
		return nil, false
	}
	return &ast.DocTag{Span: span, Kind: ast.DocApply, Name: name, Text: rest}, true
}

// docDecorators は、説明のコメントの中の、デコレータのタグを、デコレータの適用にする: 文章を、文字列のリテラル一つの
// 引数にする（docs/grammar.md「ドキュメンテーションコメント」）。宣言の前に書いた適用 decorators の、前に並べる（ソースの順）。
func docDecorators(doc *ast.DocComment, decorators []*ast.Decorator) []*ast.Decorator {
	if doc == nil {
		return decorators
	}
	var out []*ast.Decorator
	for i := range doc.Tags {
		tag := &doc.Tags[i]
		if tag.Kind != ast.DocApply {
			continue
		}
		lit := &ast.Literal{Span: tag.Span, Kind: ast.StringLit, Raw: tag.Text, Str: tag.Text}
		term := &ast.ArgTerm{Span: tag.Span, Literal: lit}
		value := &ast.ArgValue{Span: tag.Span, Terms: []*ast.ArgTerm{term}}
		out = append(out, &ast.Decorator{Span: tag.Span, Name: tag.Name, Args: []*ast.Arg{{Span: tag.Span, Value: value}}, FromDoc: true})
	}
	if out == nil {
		return decorators
	}
	return append(out, decorators...)
}

// firstDocApply は、説明のコメントの中の、最初の、デコレータのタグ。なければ nil。
func firstDocApply(doc *ast.DocComment) *ast.DocTag {
	if doc == nil {
		return nil
	}
	for i := range doc.Tags {
		if doc.Tags[i].Kind == ast.DocApply {
			return &doc.Tags[i]
		}
	}
	return nil
}

// qualifiedNameOf は文字列を QualifiedName にする。識別子の形でなければ ok は false。
// keywordLast が true なら、最後の要素は予約語でもよい（DecoratorRef）。
func qualifiedNameOf(s string, file source.FileID, start int, keywordLast bool) (ast.QualifiedName, bool) {
	qn := ast.QualifiedName{Span: source.Span{File: file, Start: source.Offset(start), End: source.Offset(start + len(s))}}
	off := start
	segs := strings.Split(s, ".")
	for i, seg := range segs {
		if !isIdentText(seg) && !(keywordLast && i == len(segs)-1 && isKeywordText(seg)) {
			return qn, false
		}
		qn.Segments = append(qn.Segments, ast.Ident{
			Span: source.Span{File: file, Start: source.Offset(off), End: source.Offset(off + len(seg))},
			Name: seg,
		})
		off += len(seg) + 1
	}
	return qn, true
}

func isIdentText(s string) bool {
	if s == "" || !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdentChar(s[i]) {
			return false
		}
	}
	_, keyword := token.Keywords[s]
	return !keyword
}

func isKeywordText(s string) bool {
	_, keyword := token.Keywords[s]
	return keyword
}
