// Package source はファイルの集合と位置を持つ（docs/packages.md「internal/source」）。
//
// 構文木と診断はバイトのオフセット（Span）で位置を持ち、
// 表示や IR の Span を作るときに Position で 1 始まりの行・列に直す。
package source

import (
	"sort"
	"unicode/utf8"
)

// FileID は FileSet の中のファイルを指す番号。Add の順に 0 から振る。
type FileID int

// Offset はファイルの先頭からのバイトのオフセット。
type Offset int

// Span はファイルの中のバイトの範囲。End は含まない。
type Span struct {
	File  FileID
	Start Offset
	End   Offset
}

// Pos は 1 始まりの行と列。Col は UTF-8 のバイトではなく Unicode のコードポイントで数える
// （docs/ir.md「Program」の Pos）。
type Pos struct {
	Line int
	Col  int
}

type file struct {
	path    string
	content []byte
	lines   []Offset // 各行の先頭のオフセット。lines[0] は 0
}

// FileSet はファイルの集合。ゼロ値で使える。
type FileSet struct {
	files []*file
}

// NewFileSet は空の FileSet を返す。
func NewFileSet() *FileSet {
	return &FileSet{}
}

// Add はファイルを足し、その FileID を返す。path は表示にそのまま使う
// （docs/ir.md「Program」の Span.file の三つの形のどれか）。
func (fs *FileSet) Add(path string, content []byte) FileID {
	f := &file{path: path, content: content, lines: []Offset{0}}
	for i, b := range content {
		if b == '\n' {
			f.lines = append(f.lines, Offset(i+1))
		}
	}
	fs.files = append(fs.files, f)
	return FileID(len(fs.files) - 1)
}

// Len はファイルの数を返す。
func (fs *FileSet) Len() int {
	return len(fs.files)
}

// Path は FileID のパスを返す。知らない FileID なら空文字列。
func (fs *FileSet) Path(id FileID) string {
	if f := fs.get(id); f != nil {
		return f.path
	}
	return ""
}

// Content は FileID の中身を返す。知らない FileID なら nil。
func (fs *FileSet) Content(id FileID) []byte {
	if f := fs.get(id); f != nil {
		return f.content
	}
	return nil
}

// Position はバイトのオフセットを 1 始まりの行・列に直す（docs/diagnostics.md「位置」）。
// 列はその行の先頭からオフセットまでのコードポイント数 + 1。
// ファイルの終わりを越えるオフセットはファイルの終わりとして扱う。
func (fs *FileSet) Position(id FileID, off Offset) Pos {
	f := fs.get(id)
	if f == nil {
		return Pos{Line: 1, Col: 1}
	}
	if off < 0 {
		off = 0
	}
	if int(off) > len(f.content) {
		off = Offset(len(f.content))
	}
	// off を含む行: lines[i] <= off となる最大の i
	i := sort.Search(len(f.lines), func(i int) bool { return f.lines[i] > off }) - 1
	start := f.lines[i]
	col := utf8.RuneCount(f.content[start:off]) + 1
	return Pos{Line: i + 1, Col: col}
}

func (fs *FileSet) get(id FileID) *file {
	if int(id) < 0 || int(id) >= len(fs.files) {
		return nil
	}
	return fs.files[id]
}
