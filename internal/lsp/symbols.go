package lsp

import (
	"sort"
	"strings"

	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
)

// documentSymbols は textDocument/documentSymbol（docs/lsp.md「ドキュメントのシンボル」）。構文木から
// DocumentSymbol の木を作る（docs/requirements.md「言語サーバー」: 宣言の一覧と入れ子は構文木から引く）。
// 名前空間は Namespace、その中に宣言。種類の対応は同「ドキュメントのシンボル」の表。
// プロジェクトにないファイルなら空の列。
func (s *server) documentSymbols(p protocol.DocumentSymbolParams) []protocol.DocumentSymbol {
	id, ok := s.fileOf(p.TextDocument.URI)
	if !ok {
		return []protocol.DocumentSymbol{}
	}
	f := s.fileAST(id)
	if f == nil {
		return []protocol.DocumentSymbol{}
	}
	b := &symbolBuilder{fs: s.proj.Files, content: s.proj.Files.Content(id), enc: s.enc}
	out := b.decls(f.Decls)
	if out == nil {
		out = []protocol.DocumentSymbol{}
	}
	return out
}

type symbolBuilder struct {
	fs      *source.FileSet
	content []byte
	enc     encoding
}

// text は節の元の文字列（detail に使う）。
// subject は classify / condition の対象（一つ以上。docs/grammar.md「classify / condition」の ClassifySubject）。
func (b *symbolBuilder) subject(of []ast.Type) string {
	parts := make([]string, 0, len(of))
	for _, t := range of {
		parts = append(parts, b.text(t))
	}
	return strings.Join(parts, ", ")
}

func (b *symbolBuilder) text(n ast.Node) string {
	if n == nil {
		return ""
	}
	sp := n.NodeSpan()
	if int(sp.End) > len(b.content) || sp.Start > sp.End {
		return ""
	}
	return strings.Join(strings.Fields(string(b.content[sp.Start:sp.End])), " ")
}

func (b *symbolBuilder) sym(name string, kind protocol.SymbolKind, whole, sel source.Span, detail string) protocol.DocumentSymbol {
	return protocol.DocumentSymbol{
		Name:           name,
		Detail:         detail,
		Kind:           kind,
		Range:          b.enc.toRange(b.fs, whole),
		SelectionRange: b.enc.toRange(b.fs, sel),
	}
}

func (b *symbolBuilder) decls(decls []ast.Decl) []protocol.DocumentSymbol {
	var out []protocol.DocumentSymbol
	for _, d := range decls {
		switch d := d.(type) {
		case *ast.NamespaceDecl:
			sym := b.sym(d.Name.String(), protocol.SymbolNamespace, d.Span, d.Name.Span, "")
			sym.Children = b.decls(d.Decls)
			out = append(out, sym)
		case *ast.BindDecl:
			target := d.Target.String() + ".*"
			if d.Deep {
				target += "*"
			}
			out = append(out, b.sym(d.Name.Name, protocol.SymbolProperty, d.Span, d.Name.Span, "to "+target))
		case *ast.ModelDecl:
			detail := ""
			if d.Extends != nil {
				detail = "extends " + d.Extends.String()
			}
			sym := b.sym(d.Name.Name, protocol.SymbolStruct, d.Span, d.Name.Span, detail)
			if d.Body != nil {
				sym.Children = b.members(d.Body)
			}
			out = append(out, sym)
		case *ast.ScalarDecl:
			out = append(out, b.sym(d.Name.Name, protocol.SymbolTypeParameter, d.Span, d.Name.Span, "extends "+b.text(d.Base)))
		case *ast.UnionDecl:
			sym := b.sym(d.Name.Name, protocol.SymbolEnum, d.Span, d.Name.Span, "")
			for _, m := range d.Members {
				switch {
				case m.Arm != nil:
					detail := ""
					if m.Type != nil {
						if m.Type.Body != nil {
							detail = "{ ... }"
						} else {
							detail = b.text(m.Type)
						}
					}
					child := b.sym(m.Arm.Name, protocol.SymbolEnumMember, m.Span, m.Arm.Span, detail)
					if m.Type != nil && m.Type.Body != nil {
						child.Children = b.members(m.Type.Body)
					}
					sym.Children = append(sym.Children, child)
				case m.Target != nil:
					sym.Children = append(sym.Children, b.sym(m.Target.String(), protocol.SymbolEnumMember, m.Span, m.Target.Span, ""))
				}
			}
			out = append(out, sym)
		case *ast.StateDecl:
			detail := ""
			if d.Type != nil {
				detail = b.text(d.Type)
			}
			out = append(out, b.sym(d.Name.Name, protocol.SymbolVariable, d.Span, d.Name.Span, detail))
		case *ast.ExceptionDecl:
			detail := ""
			if d.Value != nil {
				if d.Value.Body != nil {
					detail = "{ ... }"
				} else {
					detail = b.text(d.Value)
				}
			}
			sym := b.sym(d.Name.Name, protocol.SymbolClass, d.Span, d.Name.Span, detail)
			if d.Value != nil && d.Value.Body != nil {
				sym.Children = b.members(d.Value.Body)
			}
			out = append(out, sym)
		case *ast.EffectDecl:
			// 宛先は組み込みの効果（on s.post）か遷移（on X: A -> B）のどちらか（docs/grammar.md「exception / effect」）
			detail := ""
			switch {
			case d.On != nil:
				detail = "on " + d.On.State.String() + "." + d.On.Op.String()
			case d.Transition != nil:
				detail = "on " + b.text(d.Transition)
			}
			out = append(out, b.sym(d.Name.Name, protocol.SymbolEvent, d.Span, d.Name.Span, detail))
		case *ast.ClassifyDecl:
			sym := b.sym(d.Name.Name, protocol.SymbolEnum, d.Span, d.Name.Span, "of "+b.subject(d.Of))
			for _, c := range d.Cases {
				if c.Name != nil {
					sym.Children = append(sym.Children, b.sym(c.Name.Name, protocol.SymbolEnumMember, c.Span, c.Name.Span, ""))
				}
			}
			out = append(out, sym)
		case *ast.ConditionDecl:
			sym := b.sym(d.Name.Name, protocol.SymbolEnum, d.Span, d.Name.Span, "of "+b.subject(d.Of))
			sym.Children = []protocol.DocumentSymbol{b.sym(d.Name.Name, protocol.SymbolEnumMember, d.Name.Span, d.Name.Span, "")}
			out = append(out, sym)
		case *ast.DecoratorDecl:
			out = append(out, b.sym(d.Name.Name, protocol.SymbolInterface, d.Span, d.Name.Span, b.declTail(d.Span, d.Name.Span)))
		case *ast.OpDecl:
			var params []string
			for _, p := range d.Params {
				params = append(params, b.text(p))
			}
			detail := "(" + strings.Join(params, ", ") + ")"
			if d.Return != nil {
				detail += ": " + b.text(d.Return)
			}
			out = append(out, b.sym(d.Name.Name, protocol.SymbolFunction, d.Span, d.Name.Span, detail))
		}
	}
	return out
}

// members は model の本体に書いたメンバー（Field と Override）を、書かれた順に子にする。構文木から引くので、
// 継承した model が受け継いだフィールドは子にならない（docs/lsp.md「ドキュメントのシンボル」）。
func (b *symbolBuilder) members(body *ast.ModelBody) []protocol.DocumentSymbol {
	type member struct {
		start source.Offset
		sym   protocol.DocumentSymbol
	}
	var all []member
	for _, f := range body.Fields {
		detail := ""
		if f.Type != nil {
			detail = b.text(f.Type)
		}
		all = append(all, member{f.Span.Start, b.sym(f.Name.Name, protocol.SymbolField, f.Span, f.Name.Span, detail)})
	}
	for _, o := range body.Overrides {
		detail := "override"
		switch {
		case o.Never:
			detail = "override: never"
		case o.Type != nil:
			detail = "override: " + b.text(o.Type)
		}
		all = append(all, member{o.Span.Start, b.sym(o.Name.Name, protocol.SymbolField, o.Span, o.Name.Span, detail)})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].start < all[j].start })
	var out []protocol.DocumentSymbol
	for _, m := range all {
		out = append(out, m.sym)
	}
	return out
}

// declTail は宣言の名前の後ろから `;` の前までの元の文字列（デコレータの宣言の detail）。
func (b *symbolBuilder) declTail(whole, name source.Span) string {
	if int(whole.End) > len(b.content) || name.End > whole.End {
		return ""
	}
	tail := strings.TrimSpace(string(b.content[name.End:whole.End]))
	tail = strings.TrimSuffix(tail, ";")
	return strings.Join(strings.Fields(tail), " ")
}
