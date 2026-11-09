package syntax

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
)

// TestParseTestdata は設計書 §3〜§11 と examples/hozoc.hozo の例（testdata/）が誤りなく読めることを確かめる。
func TestParseTestdata(t *testing.T) {
	paths, err := filepath.Glob("testdata/*.hozo")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("testdata/*.hozo がない")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fs := source.NewFileSet()
			id := fs.Add(path, content)
			file, diags := Parse(fs, id)
			if len(diags) > 0 {
				var buf bytes.Buffer
				_ = diag.WriteText(&buf, diags, fs)
				t.Errorf("診断がある:\n%s", buf.String())
			}
			if len(file.Decls) == 0 {
				t.Errorf("宣言が一つも読めていない")
			}
			checkSpans(t, fs, file)
		})
	}
}

// checkSpans は構文木のすべての節の Span が、ファイルの中に収まり、入れ子の節が親の中にあることを確かめる。
func checkSpans(t *testing.T, fs *source.FileSet, file *ast.File) {
	t.Helper()
	size := source.Offset(len(fs.Content(file.Span.File)))
	walk(file, func(n ast.Node, parent ast.Node) {
		s := n.NodeSpan()
		if s.Start < 0 || s.End > size || s.Start > s.End {
			t.Errorf("%T の Span がファイルに収まらない: %+v", n, s)
		}
		if parent != nil {
			ps := parent.NodeSpan()
			if s.Start < ps.Start || s.End > ps.End {
				t.Errorf("%T の Span %+v が親 %T の Span %+v の外", n, s, parent, ps)
			}
		}
	})
}
