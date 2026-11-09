package lsp

import (
	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/sema"
)

// definition は textDocument/definition（docs/lsp.md「定義へジャンプ」）。カーソル位置のトークンの解決先
// （sema の Ref の Target）の Span を Location で返す。名前空間なら最初のブロックの名前。語彙の宣言なら `hozo:` の
// URI になる（pathToURI）。解決できなければ nil（JSON の null）。
func (s *server) definition(p protocol.TextDocumentPositionParams) *protocol.Location {
	id, ok := s.fileOf(p.TextDocument.URI)
	if !ok {
		return nil
	}
	off := s.enc.toOffset(s.proj.Files.Content(id), p.Position)
	t, _, ok := s.targetAt(id, off)
	if !ok {
		return nil
	}
	span := t.Span
	if t.Kind == sema.TargetNamespace {
		if span, ok = s.namespaceBlock(t.Decl); !ok {
			return nil
		}
	}
	loc := s.location(span)
	return &loc
}
