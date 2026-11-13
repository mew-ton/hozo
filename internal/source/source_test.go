package source

import "testing"

func TestAddAndPath(t *testing.T) {
	fs := NewFileSet()
	a := fs.Add("a.hozo", []byte("x"))
	b := fs.Add("b.hozo", []byte("y"))
	if a == b {
		t.Fatalf("FileID が重複している: %d", a)
	}
	if got := fs.Path(a); got != "a.hozo" {
		t.Errorf("Path(a) = %q", got)
	}
	if got := fs.Path(b); got != "b.hozo" {
		t.Errorf("Path(b) = %q", got)
	}
	if got := fs.Path(FileID(99)); got != "" {
		t.Errorf("Path(unknown) = %q", got)
	}
	if got := string(fs.Content(b)); got != "y" {
		t.Errorf("Content(b) = %q", got)
	}
	if fs.Len() != 2 {
		t.Errorf("Len = %d", fs.Len())
	}
}

func TestPosition(t *testing.T) {
	// 2 行目は「あい」（各 3 バイト）に続けて x。
	content := "ab\nあいx\n\nz"
	fs := NewFileSet()
	id := fs.Add("t.hozo", []byte(content))
	tests := []struct {
		off  Offset
		want Pos
	}{
		{0, Pos{1, 1}},
		{1, Pos{1, 2}},
		{2, Pos{1, 3}},  // 改行の位置
		{3, Pos{2, 1}},  // あ
		{6, Pos{2, 2}},  // い
		{9, Pos{2, 3}},  // x
		{10, Pos{2, 4}}, // 2 行目の改行
		{11, Pos{3, 1}}, // 空行
		{12, Pos{4, 1}}, // z
		{13, Pos{4, 2}}, // ファイルの終わり
		{99, Pos{4, 2}}, // 越えたら終わりに丸める
		{-1, Pos{1, 1}},
	}
	for _, tt := range tests {
		if got := fs.Position(id, tt.off); got != tt.want {
			t.Errorf("Position(%d) = %+v, want %+v", tt.off, got, tt.want)
		}
	}
	if got := fs.Position(FileID(5), 0); got != (Pos{1, 1}) {
		t.Errorf("Position(unknown) = %+v", got)
	}
}

func TestPositionEmptyFile(t *testing.T) {
	fs := NewFileSet()
	id := fs.Add("e.hozo", nil)
	if got := fs.Position(id, 0); got != (Pos{1, 1}) {
		t.Errorf("Position(0) = %+v", got)
	}
}

// Offset は Position の逆。越えた行はファイルの終わり、越えた列は行の終わり（改行の位置）に丸める。
func TestOffset(t *testing.T) {
	content := "ab\nあいx\n\nz"
	fs := NewFileSet()
	id := fs.Add("t.hozo", []byte(content))
	// Position と行き来できる（ファイルの中のすべての文字の先頭）
	for off := 0; off <= len(content); off++ {
		if off < len(content) && content[off]&0xC0 == 0x80 {
			continue // コードポイントの途中
		}
		pos := fs.Position(id, Offset(off))
		if got := fs.Offset(id, pos); got != Offset(off) {
			t.Errorf("Offset(Position(%d) = %+v) = %d", off, pos, got)
		}
	}
	tests := []struct {
		pos  Pos
		want Offset
	}{
		{Pos{1, 99}, 2},  // 列が行の終わりを越える → 改行の位置
		{Pos{2, 99}, 10}, // 同じ（コードポイントで数える）
		{Pos{4, 99}, 13}, // 最後の行はファイルの終わり
		{Pos{5, 1}, 13},  // 行がファイルの終わりを越える
		{Pos{99, 7}, 13},
		{Pos{0, 0}, 0}, // 1 より小さい行と列は 1
		{Pos{-3, 2}, 1},
	}
	for _, tt := range tests {
		if got := fs.Offset(id, tt.pos); got != tt.want {
			t.Errorf("Offset(%+v) = %d, want %d", tt.pos, got, tt.want)
		}
	}
	if got := fs.Offset(FileID(5), Pos{2, 2}); got != 0 {
		t.Errorf("Offset(unknown) = %d", got)
	}
}

// Offset は行数によらず一定の手間（行の表を引く）。行の多いファイルの終わりの方を何度引いても、行数の二乗にならない。
func BenchmarkOffsetLastLine(b *testing.B) {
	content := make([]byte, 0, 200000)
	for i := 0; i < 20000; i++ {
		content = append(content, "0123456789\n"...)
	}
	fs := NewFileSet()
	id := fs.Add("big.hozo", content)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs.Offset(id, Pos{Line: 20000, Col: 5})
	}
}
