package lsp

import (
	"testing"

	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/source"
)

// TestEncodingConversion はコードポイントの列（docs/ir.md「Program」の Pos）と utf-16 / utf-8 の桁の変換
// （docs/lsp.md「通信」）。"😀" は UTF-8 で 4 バイト、UTF-16 で 2 単位、コードポイントで 1。
func TestEncodingConversion(t *testing.T) {
	content := []byte("ab\n日😀x\nz")
	fs := source.NewFileSet()
	id := fs.Add("t.hozo", content)
	offX := source.Offset(3 + 3 + 4) // 2 行目の x
	if pos := fs.Position(id, offX); pos.Line != 2 || pos.Col != 3 {
		t.Fatalf("Position = %+v", pos)
	}
	if got := encUTF16.toPosition(fs, id, offX); got != (protocol.Position{Line: 1, Character: 3}) {
		t.Fatalf("utf-16 = %+v", got)
	}
	if got := encUTF8.toPosition(fs, id, offX); got != (protocol.Position{Line: 1, Character: 7}) {
		t.Fatalf("utf-8 = %+v", got)
	}
	for _, e := range []encoding{encUTF16, encUTF8} {
		for _, off := range []source.Offset{0, 1, 2, 3, 6, 10, 11, 12, 13} {
			pos := e.toPosition(fs, id, off)
			if back := e.toOffset(content, pos); back != off {
				t.Errorf("%s: offset %d → %+v → %d", e, off, pos, back)
			}
		}
	}
	// 行の長さを越える桁は行の終わり、サロゲートの途中は文字の先頭、ない行は内容の終わり
	if got := encUTF16.toOffset(content, protocol.Position{Line: 0, Character: 99}); got != 2 {
		t.Errorf("past end of line = %d", got)
	}
	if got := encUTF16.toOffset(content, protocol.Position{Line: 1, Character: 2}); got != 6 {
		t.Errorf("inside surrogate = %d", got)
	}
	if got := encUTF8.toOffset(content, protocol.Position{Line: 1, Character: 4}); got != 6 {
		t.Errorf("inside multibyte = %d", got)
	}
	if got := encUTF16.toOffset(content, protocol.Position{Line: 9, Character: 0}); got != source.Offset(len(content)) {
		t.Errorf("missing line = %d", got)
	}
	if negotiate([]string{"utf-16", "utf-8"}) != encUTF8 || negotiate(nil) != encUTF16 || negotiate([]string{"utf-32"}) != encUTF16 {
		t.Error("negotiate")
	}
}

// TestURI は file: の URI とパスの往復と、hozo: の URI がそのままであること。
func TestURI(t *testing.T) {
	root := "/ws/proj"
	if got := pathToURI(root, "sub/a b.hozo"); got != "file:///ws/proj/sub/a%20b.hozo" {
		t.Fatalf("pathToURI = %s", got)
	}
	if p, ok := uriToPath(root, "file:///ws/proj/sub/a%20b.hozo"); !ok || p != "sub/a b.hozo" {
		t.Fatalf("uriToPath = %q %v", p, ok)
	}
	if p, ok := uriToPath(root, "file:///other/x.hozo"); !ok || p != "/other/x.hozo" {
		t.Fatalf("outside root = %q %v", p, ok)
	}
	if got := pathToURI(root, "hozo:vocab/valid.hozo"); got != "hozo:vocab/valid.hozo" {
		t.Fatalf("vocab = %s", got)
	}
	if p, ok := uriToPath(root, "hozo:plugin/x/y.hozo"); !ok || p != "hozo:plugin/x/y.hozo" {
		t.Fatalf("plugin vocab = %q %v", p, ok)
	}
	if _, ok := uriToPath(root, "untitled:Untitled-1"); ok {
		t.Fatal("untitled must not resolve")
	}
}
