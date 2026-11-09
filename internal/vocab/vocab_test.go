package vocab

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docs/vocab.md「同梱する語彙」の表。
var expected = map[string][]string{
	"hozo:vocab/valid.hozo":        {"namespace valid {", "decorator min(", "decorator max(", "decorator length(min: int?, max: int?)", "decorator pattern(", "decorator format("},
	"hozo:vocab/http.hozo":         {"namespace http {", "exception ServerError;", "exception NetworkError;", "@external\n  decorator get(", "@external\n  decorator post(", "decorator status(", "decorator header(", "decorator body for field;", "decorator query for field;", "decorator retry("},
	"hozo:vocab/db.hozo":           {"decorator db for state;", "namespace db {", "decorator key for field;", "decorator unique for field;", "decorator ref(target: any model) for field;"},
	"hozo:vocab/localstorage.hozo": {"decorator localStorage for state;"},
	"hozo:vocab/page.hozo":         {"namespace page {", "exception Aborted;", "scalar Route extends string;", "decorator path for state;", "decorator query for state;", "decorator route(path: string, from: any state?, onInvalid: any effect?);", "decorator dialog(", "decorator fallback(", "decorator interaction("},
	"hozo:vocab/cli.hozo":          {"namespace cli {", "decorator command(", "decorator arg for field;", "decorator flag(", "decorator env(", "decorator exitCode("},
	"hozo:vocab/fs.hozo":           {"namespace fs {", "decorator files(glob: string) for state;"},
	"hozo:vocab/proc.hozo":         {"namespace proc {", "exception Crashed;", "exception MalformedOutput;", "decorator stdio throws Crashed | MalformedOutput;"},
}

func TestFiles(t *testing.T) {
	files := Files()
	if len(files) != len(expected) {
		t.Fatalf("got %d files, want %d", len(files), len(expected))
	}
	for i := 1; i < len(files); i++ {
		if files[i-1].Path >= files[i].Path {
			t.Errorf("not sorted: %s before %s", files[i-1].Path, files[i].Path)
		}
	}
	for _, f := range files {
		want, ok := expected[f.Path]
		if !ok {
			t.Errorf("unexpected file %s", f.Path)
			continue
		}
		s := string(f.Content)
		for _, w := range want {
			if !strings.Contains(s, w) {
				t.Errorf("%s: missing %q", f.Path, w)
			}
		}
		// docs/vocab.md「同梱する語彙」: x?: T は書かない。cache は同梱しない。proc.stdio は @external でなく引数もない。
		if regexp.MustCompile(`\w\?:`).MatchString(s) {
			t.Errorf("%s: uses x?: T", f.Path)
		}
		if strings.Contains(s, "cache") {
			t.Errorf("%s: contains cache", f.Path)
		}
		if strings.Contains(s, "stdio(") || regexp.MustCompile(`@external\s+decorator stdio`).MatchString(s) {
			t.Errorf("%s: stdio must have no parameter and no @external", f.Path)
		}
	}
}

func TestAddTo(t *testing.T) {
	var got []string
	AddTo(func(path string, content []byte) {
		if len(content) == 0 {
			t.Errorf("%s: empty", path)
		}
		got = append(got, path)
	})
	if len(got) != len(expected) {
		t.Fatalf("added %d, want %d", len(got), len(expected))
	}
	for _, p := range got {
		if !strings.HasPrefix(p, VocabPrefix) || !IsVocab(p) {
			t.Errorf("bad path %s", p)
		}
	}
}

func TestPluginPath(t *testing.T) {
	cases := map[[2]string]string{
		{"react-router", "vocab/page.hozo"}:   "hozo:plugin/react-router/vocab/page.hozo",
		{"x", "./a/../b.hozo"}:                "hozo:plugin/x/b.hozo",
		{"x", filepath.Join("sub", "y.hozo")}: "hozo:plugin/x/sub/y.hozo",
	}
	for in, want := range cases {
		if got := PluginPath(in[0], in[1]); got != want {
			t.Errorf("PluginPath(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestPluginFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vocab"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vocab", "x.hozo"), []byte("namespace x {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := PluginFiles("p", dir, []string{"vocab/x.hozo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "hozo:plugin/p/vocab/x.hozo" || string(files[0].Content) != "namespace x {}" {
		t.Errorf("unexpected %+v", files)
	}
	if _, err := PluginFiles("p", dir, []string{"missing.hozo"}); err == nil {
		t.Error("missing file: want error")
	}
	if _, err := PluginFiles("p", dir, []string{"../x.hozo"}); err == nil {
		t.Error("escaping path: want error")
	}
	var n int
	if err := AddPluginTo(func(string, []byte) { n++ }, "p", dir, []string{"vocab/x.hozo"}); err != nil || n != 1 {
		t.Errorf("AddPluginTo: n=%d err=%v", n, err)
	}
}
