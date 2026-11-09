package sema

import (
	"strings"

	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// docContext は DocComment を下ろす場所。
type docContext struct {
	ns *namespace
	// span は付いた先の名前の位置（@throws の所属など、後の検査の位置に使う）。
	span source.Span
	// params は @param が指せる名前（op の引数名、デコレータの Param 名）。hasParams が false なら @param は常に DOC-1。
	params    map[string]bool
	hasParams bool
}

// lowerDoc は DocComment → Doc（docs/ir.md「Doc」）。summary は最初の段落、body は全文（タグの行を含まない）。
// タグ: @param は引数名に（DOC-1）、@throws は例外の FQN に（解決できなければ DOC-1。Signature.exceptions に含まれるかは
// internal/check が確かめる）、@see はどの宣言でも（DOC-1）。同じ @param 名・同じ @throws 例外が二度あれば DOC-2。
func (a *analyzer) lowerDoc(dc *ast.DocComment, ctx docContext) *ir.Doc {
	doc := &ir.Doc{Body: dc.Body, Summary: firstParagraph(dc.Body)}
	seenParam := map[string]bool{}
	seenThrows := map[string]bool{}
	for i := range dc.Tags {
		tag := &dc.Tags[i]
		switch tag.Kind {
		case ast.DocParam:
			a.lowerDocParam(tag, ctx, doc, seenParam)
		case ast.DocThrows:
			a.lowerDocThrows(tag, ctx, doc, seenThrows)
		case ast.DocSee:
			if r, err := a.resolveName(tag.Name, &scope{ns: ctx.ns}, acAnyDecl); err != nil {
				a.errorf("DOC-1", tag.Name.Span, "@see の %s を解決できない: %s", tag.Name.String(), err.msg)
			} else {
				doc.See = append(doc.See, r.sym.fqn)
			}
		}
	}
	return doc
}

// lowerDocParam は DOC-1 / DOC-2（docs/check.md「名前空間」）の @param。
func (a *analyzer) lowerDocParam(tag *ast.DocTag, ctx docContext, doc *ir.Doc, seen map[string]bool) {
	name := tag.Name.String()
	if !ctx.hasParams || !ctx.params[name] {
		a.errorf("DOC-1", tag.Name.Span, "@param の %s は引数名にない", name)
		return
	}
	if seen[name] {
		a.errorf("DOC-2", tag.Name.Span, "@param %s が二度ある", name)
		return
	}
	seen[name] = true
	doc.Params = append(doc.Params, ir.DocParam{Name: name, Text: tag.Text})
}

// lowerDocThrows は @throws。例外の名前の解決だけを行う。その op の Signature.exceptions に含まれるか（DOC-1 の後半）は
// Signature を要するので internal/check が確かめる。
func (a *analyzer) lowerDocThrows(tag *ast.DocTag, ctx docContext, doc *ir.Doc, seen map[string]bool) {
	r, err := a.resolveName(tag.Name, &scope{ns: ctx.ns}, acException)
	if err != nil {
		a.errorf("DOC-1", tag.Name.Span, "@throws の %s は例外として解決できない: %s", tag.Name.String(), err.msg)
		return
	}
	if seen[r.sym.fqn] {
		a.errorf("DOC-2", tag.Name.Span, "@throws %s が二度ある", r.sym.fqn)
		return
	}
	seen[r.sym.fqn] = true
	doc.Throws = append(doc.Throws, ir.DocThrows{Exception: r.sym.fqn, Text: tag.Text})
}

// firstParagraph は Markdown の最初の段落（最初の空行まで）。
func firstParagraph(body string) string {
	body = strings.Trim(body, "\n")
	for _, sep := range []string{"\n\n", "\n\r\n"} {
		if i := strings.Index(body, sep); i >= 0 {
			return strings.TrimSpace(body[:i])
		}
	}
	return strings.TrimSpace(body)
}
