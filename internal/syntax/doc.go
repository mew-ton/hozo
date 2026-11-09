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
//   - タグは @param / @throws / @see だけ。ほかの "@" で始まる行は本文。
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
		return nil, false
	}
	if len(fields) < 2 {
		p.errorAt(span, fields[0]+" の後ろに名前がない")
		return nil, false
	}
	nameStart := int(span.Start) + strings.Index(text, fields[1])
	name, ok := qualifiedNameOf(fields[1], p.file, nameStart)
	if !ok || (kind == ast.DocParam && len(name.Segments) != 1) {
		p.errorAt(span, fields[0]+" の名前の形が正しくない: "+fields[1])
		return nil, false
	}
	tag := &ast.DocTag{Span: span, Kind: kind, Name: name}
	rest := strings.TrimSpace(text[strings.Index(text, fields[1])+len(fields[1]):])
	if kind == ast.DocSee {
		if rest != "" {
			p.errorAt(span, "@see は名前だけを書く")
		}
	} else {
		tag.Text = rest
	}
	return tag, true
}

// qualifiedNameOf は文字列を QualifiedName にする。識別子の形でなければ ok は false。
func qualifiedNameOf(s string, file source.FileID, start int) (ast.QualifiedName, bool) {
	qn := ast.QualifiedName{Span: source.Span{File: file, Start: source.Offset(start), End: source.Offset(start + len(s))}}
	off := start
	for _, seg := range strings.Split(s, ".") {
		if !isIdentText(seg) {
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
