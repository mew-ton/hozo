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
