// Package tests は、リポジトリ全体にまたがるテスト（docs/testing.md）。
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax"
)

// TestParseAllHozo は、コーパス（docs/testing.md「コーパス」）と標準の語彙（docs/vocab.md）の
// すべての .hozo が構文エラーなしに解析できることを確かめる。
func TestParseAllHozo(t *testing.T) {
	var files []string
	for _, pattern := range []string{"corpus/*/*.hozo", "../vocab/*.hozo"} {
		m, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatal("no .hozo files found")
	}
	for _, path := range files {
		t.Run(path, func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fs := source.NewFileSet()
			id := fs.Add(path, content)
			file, diags := syntax.Parse(fs, id)
			if len(diags) > 0 {
				var buf = new(strings.Builder)
				_ = diag.WriteText(buf, diags, fs)
				t.Fatalf("syntax errors:\n%s", buf.String())
			}
			if file == nil || len(file.Decls) == 0 {
				t.Fatal("no declarations parsed")
			}
		})
	}
}

// TestParseCliHozoInDocs は、docs/cli.md「cli.hozo」のコードブロックが解析できることを確かめる。
// 段階 2 でリポジトリ直下の cli.hozo になるまでの間、文書の方を検査の対象にする。
func TestParseCliHozoInDocs(t *testing.T) {
	content, err := os.ReadFile("../docs/cli.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	start := strings.Index(s, "```ts\n")
	if start < 0 {
		t.Fatal("no ```ts block in docs/cli.md")
	}
	start += len("```ts\n")
	end := strings.Index(s[start:], "\n```")
	if end < 0 {
		t.Fatal("unterminated code block")
	}
	src := s[start : start+end]
	fs := source.NewFileSet()
	id := fs.Add("docs/cli.md#cli.hozo", []byte(src))
	file, diags := syntax.Parse(fs, id)
	if len(diags) > 0 {
		buf := new(strings.Builder)
		_ = diag.WriteText(buf, diags, fs)
		t.Fatalf("syntax errors:\n%s", buf.String())
	}
	if file == nil || len(file.Decls) != 1 {
		t.Fatalf("expected one namespace declaration, got %d", len(file.Decls))
	}
}
