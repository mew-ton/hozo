package lsp

import (
	"unicode/utf8"

	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/source"
)

// encoding は位置のエンコーディング（docs/lsp.md「通信」: `general.positionEncodings` に `utf-8` があれば
// utf-8、なければ utf-16）。docs/ir.md の Pos はコードポイントで数えるので、どちらにも変換が要る。
type encoding string

const (
	encUTF16 encoding = protocol.PositionEncodingUTF16
	encUTF8  encoding = protocol.PositionEncodingUTF8
)

// negotiate はクライアントの positionEncodings から使うエンコーディングを決める。
func negotiate(offered []string) encoding {
	for _, e := range offered {
		if e == protocol.PositionEncodingUTF8 {
			return encUTF8
		}
	}
	return encUTF16
}

// units は s の長さをエンコーディングの単位で数える。utf-8 ならバイト、utf-16 なら 16 ビットの単位。
func (e encoding) units(s []byte) int {
	if e == encUTF8 {
		return len(s)
	}
	n := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRune(s)
		s = s[size:]
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// lineStart は content の中で off を含む行の先頭のオフセット。
func lineStart(content []byte, off int) int {
	if off > len(content) {
		off = len(content)
	}
	for off > 0 && content[off-1] != '\n' {
		off--
	}
	return off
}

// toPosition はバイトのオフセットを LSP の Position（0 始まりの行、エンコーディングの単位の桁）にする。
// 行は FileSet.Position（docs/packages.md「internal/source」）で、桁は行頭から off までの内容を数える。
func (e encoding) toPosition(fs *source.FileSet, id source.FileID, off source.Offset) protocol.Position {
	content := fs.Content(id)
	o := int(off)
	if o < 0 {
		o = 0
	}
	if o > len(content) {
		o = len(content)
	}
	pos := fs.Position(id, source.Offset(o))
	ls := lineStart(content, o)
	return protocol.Position{Line: pos.Line - 1, Character: e.units(content[ls:o])}
}

// toRange はバイトの範囲を LSP の Range にする。End は含まないのでそのまま（docs/lsp.md「診断」）。
func (e encoding) toRange(fs *source.FileSet, sp source.Span) protocol.Range {
	return protocol.Range{Start: e.toPosition(fs, sp.File, sp.Start), End: e.toPosition(fs, sp.File, sp.End)}
}

// toOffset は LSP の Position をバイトのオフセットにする。行がなければ内容の終わり、桁が行より長ければ行の終わり。
// 桁がコードポイントの途中（utf-16 のサロゲートの間、utf-8 の多バイトの間）を指せば、そのコードポイントの先頭。
func (e encoding) toOffset(content []byte, pos protocol.Position) source.Offset {
	ls := 0
	for line := 0; line < pos.Line; line++ {
		i := indexByte(content, ls, '\n')
		if i < 0 {
			return source.Offset(len(content))
		}
		ls = i + 1
	}
	o := ls
	remaining := pos.Character
	for o < len(content) && content[o] != '\n' && remaining > 0 {
		r, size := utf8.DecodeRune(content[o:])
		w := size
		if e == encUTF16 {
			w = 1
			if r >= 0x10000 {
				w = 2
			}
		}
		if remaining < w {
			break
		}
		remaining -= w
		o += size
	}
	return source.Offset(o)
}

func indexByte(b []byte, from int, c byte) int {
	for i := from; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// applyChange は didChange の一つの変更を text に適用する。Range が nil なら全文の置き換え
// （capabilities は Full だが、範囲付きで送るクライアントにも耐える）。
func (e encoding) applyChange(text []byte, change protocol.TextDocumentContentChangeEvent) []byte {
	if change.Range == nil {
		return []byte(change.Text)
	}
	start := int(e.toOffset(text, change.Range.Start))
	end := int(e.toOffset(text, change.Range.End))
	if end < start {
		start, end = end, start
	}
	out := make([]byte, 0, len(text)-(end-start)+len(change.Text))
	out = append(out, text[:start]...)
	out = append(out, change.Text...)
	out = append(out, text[end:]...)
	return out
}
