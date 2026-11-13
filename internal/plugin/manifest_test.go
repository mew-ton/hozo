package plugin

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
)

// docs/plugin-protocol.md「マニフェスト」の例（protocol は hozoc のもの）。
const exampleManifest = `
name     = "react-router"
command  = ["node", "dist/main.js"]
language = "ts"
protocol = "0.1"

vocabulary = ["vocab/page.hozo"]

[emit]
unsupported = []

[check]
enabled = true

[import]
formats = []
`

// docs/plugin-go-cli.md「マニフェスト」。
const goCLIManifest = `
name     = "go-cli"
command  = ["go", "run", "github.com/mew-ton/hozo/plugins/go-cli"]
language = "go"
protocol = "0.1"

[emit]
`

func writeManifest(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, ManifestFileName)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func codes(diags []diag.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Code)
	}
	sort.Strings(out)
	return out
}

func TestLoadManifestExample(t *testing.T) {
	p := writeManifest(t, t.TempDir(), exampleManifest)
	m, diags := LoadManifest(p)
	if len(diags) != 0 {
		t.Fatalf("diags = %+v", diags)
	}
	want := &Manifest{
		Name: "react-router", Command: []string{"node", "dist/main.js"}, Language: "ts", Protocol: "0.1",
		Vocabulary: []string{"vocab/page.hozo"},
		Emit:       &Emit{Unsupported: []string{}},
		Check:      Check{Enabled: true},
		Import:     Import{Formats: []string{}},
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got %+v\nwant %+v", m, want)
	}
	if !m.HasEmit() || m.HasImport() {
		t.Error("HasEmit/HasImport")
	}
}

func TestLoadManifestGoCLI(t *testing.T) {
	m, diags := LoadManifest(writeManifest(t, t.TempDir(), goCLIManifest))
	if len(diags) != 0 {
		t.Fatalf("diags = %+v", diags)
	}
	if m.Name != "go-cli" || !reflect.DeepEqual(m.Command, []string{"go", "run", "github.com/mew-ton/hozo/plugins/go-cli"}) {
		t.Errorf("%+v", m)
	}
	if !reflect.DeepEqual(m.Emit, &Emit{Unsupported: []string{}}) {
		t.Errorf("emit = %+v", m.Emit)
	}
	if m.Check.Enabled || len(m.Vocabulary) != 0 || m.Vocabulary == nil || m.HasImport() {
		t.Errorf("%+v", m)
	}
}

func TestLoadManifestStringCommand(t *testing.T) {
	m, diags := LoadManifest(writeManifest(t, t.TempDir(), "name = \"x\"\ncommand = \"hozoc-plugin-x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n"))
	if len(diags) != 0 || !reflect.DeepEqual(m.Command, []string{"hozoc-plugin-x"}) || m.Emit != nil {
		t.Errorf("m=%+v diags=%+v", m, diags)
	}
}

// docs/plugin-protocol.md「診断」の MAN-2 / MAN-4。
func TestLoadManifestDiagnostics(t *testing.T) {
	cases := []struct {
		name    string
		toml    string
		want    []string
		nilMani bool
	}{
		{"bad toml", "name = \n", []string{"MAN-4"}, true},
		{"unknown key", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\nfoo = 1\n", []string{"MAN-4"}, true},
		{"unknown emit key", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n[emit]\nunsupported = [\"a\"]\nfoo = 1\n", []string{"MAN-4"}, true},
		{"unknown table", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n[extra]\na = 1\n", []string{"MAN-4"}, true},
		{"no command", "name = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n", []string{"MAN-4"}, true},
		{"empty string command", "name = \"x\"\ncommand = \"\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n", []string{"MAN-4"}, true},
		{"empty array command", "name = \"x\"\ncommand = []\nlanguage = \"go\"\nprotocol = \"0.1\"\n", []string{"MAN-4"}, true},
		{"command wrong type", "name = \"x\"\ncommand = 1\nlanguage = \"go\"\nprotocol = \"0.1\"\n", []string{"MAN-4"}, true},
		{"no name", "command = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n", []string{"MAN-4"}, true},
		{"unsupported wrong type", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n[emit]\nunsupported = \"a\"\n", []string{"MAN-4"}, true},
		// 設計書 §4.6 の decorate / entry / stop は持たない（docs/plugin-protocol.md「選択」）。書いてあれば知らないキー
		{"entry is unknown", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n[emit]\nentry = [\"cli.command\"]\n", []string{"MAN-4"}, true},
		{"stop is unknown", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n[emit]\nstop = [\"http.*\"]\n", []string{"MAN-4"}, true},
		{"decorate is unknown", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n[emit]\ndecorate = []\n", []string{"MAN-4"}, true},
		// [emit] は空でもよい（何を選ぶかは応答の select）
		{"empty emit", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n[emit]\n", nil, false},
		{"MAN-2", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"0.2\"\n", []string{"MAN-2"}, false},
		{"MAN-2 missing protocol", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\n", []string{"MAN-2"}, false},
		{"MAN-2 with emit", "name = \"x\"\ncommand = \"x\"\nlanguage = \"go\"\nprotocol = \"9\"\n[emit]\n", []string{"MAN-2"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := writeManifest(t, t.TempDir(), c.toml)
			m, diags := LoadManifest(p)
			if got := codes(diags); !reflect.DeepEqual(got, c.want) {
				t.Errorf("codes = %v, want %v (%+v)", got, c.want, diags)
			}
			if (m == nil) != c.nilMani {
				t.Errorf("manifest nil = %v, want %v", m == nil, c.nilMani)
			}
			for _, d := range diags {
				if d.Severity != diag.Error || d.File != p || d.Message == "" {
					t.Errorf("bad diagnostic %+v", d)
				}
			}
		})
	}
}

func TestLoadManifestMissing(t *testing.T) {
	m, diags := LoadManifest(filepath.Join(t.TempDir(), ManifestFileName))
	if m != nil || codes(diags)[0] != "MAN-4" {
		t.Errorf("m=%v diags=%+v", m, diags)
	}
}

func TestResolveCommand(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "plugins", "x")
	cases := map[string]string{
		"node":                         "node",
		"./dist/main":                  filepath.Join(dir, "dist", "main"),
		"dist/main.js":                 filepath.Join(dir, "dist", "main.js"),
		filepath.Join(dir, "bin", "x"): filepath.Join(dir, "bin", "x"),
	}
	for exe, want := range cases {
		m := &Manifest{Command: []string{exe}}
		if got := m.ResolveCommand(dir); got != want {
			t.Errorf("ResolveCommand(%q) = %q, want %q", exe, got, want)
		}
	}
}

// docs/plugin-protocol.md「探索」の順序。
func TestDiscoverOrder(t *testing.T) {
	root := t.TempDir()
	envDir := t.TempDir()
	home := t.TempDir()
	t.Setenv(PluginPathEnv, envDir+string(filepath.ListSeparator)+t.TempDir())
	t.Setenv("HOME", home)
	cfg := config.Default(root)
	cfg.Path = filepath.Join(root, "hozo.toml")
	cfg.Hozo.PluginDirs = []string{"plugins", "more"}

	manifest := func(lang string) string {
		return "name = \"p\"\ncommand = \"p\"\nlanguage = \"" + lang + "\"\nprotocol = \"0.1\"\n"
	}
	homeDir := filepath.Join(home, ".hozo", "plugins", "p")
	writeManifest(t, homeDir, manifest("home"))
	m, dir, diags := Discover(cfg, "p")
	if len(diags) != 0 || m == nil || m.Language != "home" || dir != homeDir {
		t.Fatalf("home: m=%+v dir=%q diags=%+v", m, dir, diags)
	}

	envPluginDir := filepath.Join(envDir, "p")
	writeManifest(t, envPluginDir, manifest("env"))
	m, dir, diags = Discover(cfg, "p")
	if len(diags) != 0 || m.Language != "env" || dir != envPluginDir {
		t.Fatalf("env: m=%+v dir=%q diags=%+v", m, dir, diags)
	}

	moreDir := filepath.Join(root, "more", "p")
	writeManifest(t, moreDir, manifest("more"))
	m, dir, diags = Discover(cfg, "p")
	if len(diags) != 0 || m.Language != "more" || dir != moreDir {
		t.Fatalf("more: m=%+v dir=%q diags=%+v", m, dir, diags)
	}

	pluginsDir := filepath.Join(root, "plugins", "p")
	writeManifest(t, pluginsDir, manifest("plugins"))
	m, dir, diags = Discover(cfg, "p")
	if len(diags) != 0 || m.Language != "plugins" || dir != pluginsDir {
		t.Fatalf("plugins: m=%+v dir=%q diags=%+v", m, dir, diags)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("dir %q is not absolute", dir)
	}
}

func TestDiscoverNotFound(t *testing.T) {
	root := t.TempDir()
	t.Setenv(PluginPathEnv, "")
	t.Setenv("HOME", t.TempDir())
	cfg := config.Default(root)
	cfg.Path = filepath.Join(root, "hozo.toml")
	cfg.Hozo.PluginDirs = []string{"plugins"}
	m, dir, diags := Discover(cfg, "nope")
	if m != nil || dir != "" || !reflect.DeepEqual(codes(diags), []string{"CFG-9"}) || diags[0].File != cfg.Path {
		t.Errorf("m=%v dir=%q diags=%+v", m, dir, diags)
	}
	// ディレクトリだけあってマニフェストがないのも CFG-9
	if err := os.MkdirAll(filepath.Join(root, "plugins", "nope"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, diags := Discover(cfg, "nope"); !reflect.DeepEqual(codes(diags), []string{"CFG-9"}) {
		t.Errorf("diags=%+v", diags)
	}
	for _, bad := range []string{"", "a/b", "..", `a\b`} {
		if _, _, diags := Discover(cfg, bad); !reflect.DeepEqual(codes(diags), []string{"CFG-9"}) {
			t.Errorf("%q: diags=%+v", bad, diags)
		}
	}
}

func TestDiscoverNameMismatch(t *testing.T) {
	root := t.TempDir()
	t.Setenv(PluginPathEnv, "")
	t.Setenv("HOME", t.TempDir())
	cfg := config.Default(root)
	cfg.Hozo.PluginDirs = []string{"plugins"}
	p := writeManifest(t, filepath.Join(root, "plugins", "p"), "name = \"q\"\ncommand = \"q\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n")
	m, _, diags := Discover(cfg, "p")
	if m != nil || !reflect.DeepEqual(codes(diags), []string{"MAN-3"}) || diags[0].File != p {
		t.Errorf("m=%v diags=%+v", m, diags)
	}
}

func TestDiscoverBadManifestStops(t *testing.T) {
	// 最初に見つかったものを使う: plugin_dirs のマニフェストが壊れていても HOME のものには進まない
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv(PluginPathEnv, "")
	t.Setenv("HOME", home)
	cfg := config.Default(root)
	cfg.Hozo.PluginDirs = []string{"plugins"}
	writeManifest(t, filepath.Join(root, "plugins", "p"), "name = \n")
	writeManifest(t, filepath.Join(home, ".hozo", "plugins", "p"), "name = \"p\"\ncommand = \"p\"\nlanguage = \"go\"\nprotocol = \"0.1\"\n")
	m, _, diags := Discover(cfg, "p")
	if m != nil || !reflect.DeepEqual(codes(diags), []string{"MAN-4"}) {
		t.Errorf("m=%v diags=%+v", m, diags)
	}
	// MAN-2 でもマニフェストは返さない
	writeManifest(t, filepath.Join(root, "plugins", "p"), "name = \"p\"\ncommand = \"p\"\nlanguage = \"go\"\nprotocol = \"0.9\"\n")
	m, _, diags = Discover(cfg, "p")
	if m != nil || !reflect.DeepEqual(codes(diags), []string{"MAN-2"}) {
		t.Errorf("m=%v diags=%+v", m, diags)
	}
}
