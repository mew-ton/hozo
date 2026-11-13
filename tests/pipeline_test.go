package tests

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/check"
	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/sema"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/internal/vocab"
)

// TestCorpusPipeline は、設計書の例のコーパスを 構文解析 → sema → check に通し、
// error も warning もないこと（tests/corpus/design/README.md の想定）を確かめる（docs/testing.md「コーパス」）。
// internal/project ができるまでのつなぎ。
func TestCorpusPipeline(t *testing.T) {
	dir := filepath.Join("corpus", "design")
	cfg, cfgDiags := config.Load(filepath.Join(dir, "hozo.toml"))
	if n, _ := diag.Count(cfgDiags); n > 0 {
		t.Fatalf("hozo.toml: %+v", cfgDiags)
	}
	fs := source.NewFileSet()
	var files []*ast.File
	var diags []diag.Diagnostic
	add := func(path string, content []byte) {
		id := fs.Add(path, content)
		f, ds := syntax.Parse(fs, id)
		files = append(files, f)
		diags = append(diags, ds...)
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.hozo"))
	sort.Strings(paths)
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		add(p, content)
	}
	vocab.AddTo(add)

	res := sema.Analyze(fs, files, cfg)
	diags = append(diags, res.Diags...)
	diags = append(diags, check.Checker{Locate: check.FileSetLocator(fs)}.Check(res.Program, cfg)...)
	diag.Sort(diags, fs, nil)

	buf := new(strings.Builder)
	_ = diag.WriteText(buf, diags, fs)
	if errs, _ := diag.Count(diags); errs > 0 {
		t.Fatalf("errors:\n%s", buf.String())
	}
	var codes []string
	for _, d := range diags {
		codes = append(codes, d.Code)
	}
	if len(codes) != 0 {
		t.Fatalf("expected no diagnostics, got %v:\n%s", codes, buf.String())
	}
	t.Logf("\n%s", buf.String())
}
