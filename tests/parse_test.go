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
	for _, pattern := range []string{"corpus/*/*.hozo", "../vocab/*.hozo", "../plugins/*/*.hozo"} {
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

// TestDocsEmbedRootFiles は、docs/cli.md「cli.hozo」に埋め込んだ二つのコードブロックが、リポジトリ直下の cli.hozo と
// hozo.toml の写しであること（バイト単位で同じ）を確かめる。文書は「写し」と言っているので、片方だけを直せばここで分かる。
// 写しが同じなら、cli.hozo が解析と検査を通ること（TestSelfCheck）は文書のブロックにも言える。
func TestDocsEmbedRootFiles(t *testing.T) {
	content, err := os.ReadFile("../docs/cli.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ fence, file string }{
		{"```ts\n", "cli.hozo"},
		{"```toml\n", "hozo.toml"},
	} {
		t.Run(c.file, func(t *testing.T) {
			s := string(content)
			if n := strings.Count(s, c.fence); n != 1 {
				t.Fatalf("docs/cli.md に %q のブロックが %d 個ある（一つのはず）", strings.TrimSpace(c.fence), n)
			}
			start := strings.Index(s, c.fence) + len(c.fence)
			end := strings.Index(s[start:], "\n```\n")
			if end < 0 {
				t.Fatal("コードブロックが閉じていない")
			}
			block := s[start:start+end] + "\n"
			want, err := os.ReadFile(filepath.Join("..", c.file))
			if err != nil {
				t.Fatal(err)
			}
			if block != string(want) {
				t.Errorf("docs/cli.md の %s のブロックが、リポジトリ直下の %s と違う（どちらかを直して合わせる）:\n%s",
					c.file, c.file, firstDiff(string(want), block))
			}
		})
	}
}
