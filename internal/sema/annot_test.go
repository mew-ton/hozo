package sema

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/internal/vocab"
)

// このファイルは `// ERROR: ID` / `// WARN: ID` の注釈（docs/testing.md「規則ごとのテスト」）で期待を書いた .hozo を
// 解析し、診断の集合を「ファイル、行、重さ、ID」の組で比べる道具。ほかのパッケージのテストからも使えるように、
// Analyze 以外に依存しない形にしてある。

// project はテストの入力: パス → 中身。
type project map[string]string

// parseProject は project を FileSet に足して構文解析する。withVocab なら標準の語彙も足す（docs/vocab.md「読み込み」）。
// 構文エラーがあればテストを失敗させる（規則のテストの対象は解決・型・正規化だけ）。
func parseProject(t testing.TB, p project, withVocab bool) (*source.FileSet, []*ast.File) {
	t.Helper()
	fs := source.NewFileSet()
	var files []*ast.File
	add := func(path string, content []byte) {
		id := fs.Add(path, content)
		f, diags := syntax.Parse(fs, id)
		if len(diags) > 0 {
			var buf bytes.Buffer
			_ = diag.WriteText(&buf, diags, fs)
			t.Fatalf("%s に構文エラー:\n%s", path, buf.String())
		}
		files = append(files, f)
	}
	paths := make([]string, 0, len(p))
	for path := range p {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		add(path, []byte(p[path]))
	}
	if withVocab {
		vocab.AddTo(add)
	}
	return fs, files
}

// analyzeProject は parseProject して Analyze する。
func analyzeProject(t testing.TB, p project, withVocab bool) (*Result, *source.FileSet) {
	t.Helper()
	fs, files := parseProject(t, p, withVocab)
	return Analyze(fs, files, nil), fs
}

// expectation は注釈一つ分の期待: ファイル、行、重さ、ID。
type expectation struct {
	file string
	line int
	sev  diag.Severity
	code string
}

func (e expectation) String() string {
	return fmt.Sprintf("%s:%d %s[%s]", e.file, e.line, e.sev, e.code)
}

// annotRE は `// ERROR: A, B` / `// WARN: A`。`// ERROR(-1): A` のように相対の行を付けると、その行数だけずらした行の期待になる
// （ドキュメンテーションコメントの @see のように、同じ行に注釈を書けない場所のため）。一行に複数の注釈を書ける。
var annotRE = regexp.MustCompile(`//\s*(ERROR|WARN)(?:\(([+-]\d+)\))?:\s*([A-Z0-9-]+(?:\s*,\s*[A-Z0-9-]+)*)`)

// expectations は注釈を読む。
func expectations(p project) map[expectation]bool {
	out := map[expectation]bool{}
	for path, content := range p {
		for i, line := range strings.Split(content, "\n") {
			for _, m := range annotRE.FindAllStringSubmatch(line, -1) {
				sev := diag.Error
				if m[1] == "WARN" {
					sev = diag.Warning
				}
				offset := 0
				if m[2] != "" {
					offset, _ = strconv.Atoi(m[2])
				}
				for _, code := range strings.Split(m[3], ",") {
					out[expectation{file: path, line: i + 1 + offset, sev: sev, code: strings.TrimSpace(code)}] = true
				}
			}
		}
	}
	return out
}

// actual は診断を期待の形にする。語彙のファイルの診断もそのまま含める。
func actual(fs *source.FileSet, diags []diag.Diagnostic) map[expectation]bool {
	out := map[expectation]bool{}
	for _, d := range diags {
		e := expectation{file: d.File, sev: d.Severity, code: d.Code}
		if d.Span != nil {
			e.file = fs.Path(d.Span.File)
			e.line = fs.Position(d.Span.File, d.Span.Start).Line
		}
		out[e] = true
	}
	return out
}

// checkAnnotations は注釈と診断を比べる。期待にない診断、出なかった期待はどちらも失敗（docs/testing.md「規則ごとのテスト」）。
func checkAnnotations(t testing.TB, fs *source.FileSet, p project, diags []diag.Diagnostic) {
	t.Helper()
	want := expectations(p)
	got := actual(fs, diags)
	var problems []string
	for e := range want {
		if !got[e] {
			problems = append(problems, "出なかった期待: "+e.String())
		}
	}
	for e := range got {
		if !want[e] {
			problems = append(problems, "期待にない診断: "+e.String())
		}
	}
	if len(problems) == 0 {
		return
	}
	sort.Strings(problems)
	var buf bytes.Buffer
	_ = diag.WriteText(&buf, diags, fs)
	t.Errorf("%s\n診断:\n%s", strings.Join(problems, "\n"), buf.String())
}

// runAnnotated は project を解析して注釈と比べる。
func runAnnotated(t testing.TB, p project, withVocab bool) *Result {
	t.Helper()
	res, fs := analyzeProject(t, p, withVocab)
	checkAnnotations(t, fs, p, res.Diags)
	return res
}

// loadDir はディレクトリの .hozo を project にする（パスはディレクトリからの相対パス）。
func loadDir(t testing.TB, dir string) project {
	t.Helper()
	p := project{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".hozo" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		p[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p) == 0 {
		t.Fatalf("%s に .hozo がない", dir)
	}
	return p
}

// TestRules は testdata/rules の各 .hozo（一つで一つのプロジェクト）と、各サブディレクトリ（複数ファイルのプロジェクト）を
// 注釈と比べる。規則ごとの落ちる例と通る例（docs/testing.md「規則ごとのテスト」の形）。
func TestRules(t *testing.T) {
	entries, err := os.ReadDir("testdata/rules")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		e := e
		t.Run(e.Name(), func(t *testing.T) {
			path := filepath.Join("testdata/rules", e.Name())
			if e.IsDir() {
				runAnnotated(t, loadDir(t, path), true)
				return
			}
			if filepath.Ext(e.Name()) != ".hozo" {
				t.Skip()
			}
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			runAnnotated(t, project{e.Name(): string(content)}, true)
		})
	}
}
