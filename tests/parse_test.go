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

// docBlock は、文書 doc の中の、marker の後ろの最初の ```ts のコードブロックの中身（末尾の改行つき）。
func docBlock(t *testing.T, doc, marker string) string {
	t.Helper()
	content, err := os.ReadFile(doc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	i := strings.Index(s, marker)
	if i < 0 {
		t.Fatalf("%s に %q がない", doc, marker)
	}
	const fence = "```ts\n"
	j := strings.Index(s[i:], fence)
	if j < 0 {
		t.Fatalf("%s の %q の後ろに、コードブロックがない", doc, marker)
	}
	start := i + j + len(fence)
	end := strings.Index(s[start:], "\n```\n")
	if end < 0 {
		t.Fatalf("%s のコードブロックが閉じていない", doc)
	}
	return s[start:start+end] + "\n"
}

// TestDocsEmbedVocabulary は、語彙のファイルが、文書に書いた宣言の写しであることを確かめる（docs/vocab.md「同梱する語彙」:
// 「ここに置く .hozo はそれを写したもの」）。文書が持ち主で、片方だけを直せばここで分かる。
//   - vocab/db.hozo は、docs/vocab.md「`db` の読み書きのデコレータ」の全文と、バイト単位で同じ。
//   - vocab/http.hozo は、docs/vocab.md「`http` の語彙」のブロック（「この部分」）を、そのまま含む。
//   - vocab/prd.hozo（prd と、その下の actor、usecase、business）は、docs/vocab.md「設計の図の語彙」の全文と、
//     バイト単位で同じ。
//   - plugins/go-cli/cli.hozo は、docs/plugin-go-cli.md「語彙」のブロックの宣言（説明のコメントは略してある）を、
//     同じ順に、過不足なく持つ。
func TestDocsEmbedVocabulary(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	t.Run("vocab/db.hozo", func(t *testing.T) {
		block := docBlock(t, "../docs/vocab.md", "`vocab/db.hozo` の全文は次のとおり")
		if got := read("../vocab/db.hozo"); got != block {
			t.Errorf("vocab/db.hozo が、docs/vocab.md の全文と違う（文書に合わせる）:\n%s", firstDiff(block, got))
		}
	})
	t.Run("vocab/http.hozo", func(t *testing.T) {
		block := docBlock(t, "../docs/vocab.md", "`vocab/http.hozo` の、この部分は次のとおり")
		if got := read("../vocab/http.hozo"); !strings.Contains(got, block) {
			t.Errorf("vocab/http.hozo が、docs/vocab.md「`http` の語彙」のブロックを、そのままは含まない（文書に合わせる）:\n%s", block)
		}
	})
	for _, name := range []string{"prd"} {
		t.Run("vocab/"+name+".hozo", func(t *testing.T) {
			block := docBlock(t, "../docs/vocab.md", "`vocab/"+name+".hozo` の全文は次のとおり")
			if got := read("../vocab/" + name + ".hozo"); got != block {
				t.Errorf("vocab/%s.hozo が、docs/vocab.md の全文と違う（文書に合わせる）:\n%s", name, firstDiff(block, got))
			}
		})
	}
	t.Run("plugins/go-cli/cli.hozo", func(t *testing.T) {
		decls := func(s string) []string {
			var out []string
			for _, line := range strings.Split(s, "\n") {
				if l := strings.TrimSpace(line); strings.HasPrefix(l, "decorator ") || strings.HasPrefix(l, "@external") || strings.HasPrefix(l, "namespace ") {
					out = append(out, l)
				}
			}
			return out
		}
		want := decls(docBlock(t, "../docs/plugin-go-cli.md", "`plugins/go-cli/cli.hozo` の宣言は次のとおり"))
		got := decls(read("../plugins/go-cli/cli.hozo"))
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("plugins/go-cli/cli.hozo の宣言が、docs/plugin-go-cli.md「語彙」と違う（文書に合わせる）:\n文書:\n  %s\nファイル:\n  %s",
				strings.Join(want, "\n  "), strings.Join(got, "\n  "))
		}
	})
}
