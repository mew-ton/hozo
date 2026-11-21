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

// docs/plugin-protocol.md「マニフェスト」の形（protocol は hozoc のもの）。
const exampleManifest = `
name     = "react-router"
command  = ["node", "dist/main.js"]
protocol = "0.12"

vocabulary = ["vocab/page.hozo"]

[[transpiler]]
name        = "react-router"
language    = "ts"
requires    = ["ts", "ts-valid"]
unsupported = []
seams       = ["rate.limited"]

[check]
enabled = true

[import]
formats = []
`

// docs/plugin-go-cli.md「マニフェスト」。
const goCLIManifest = `
name     = "go-cli"
command  = ["go", "run", "github.com/mew-ton/hozo/plugins/go-cli"]
protocol = "0.12"

[[transpiler]]
name     = "go-cli"
language = "go"
`

// base は name = "x" の最小のマニフェスト（protocol は hozoc のもの）。後ろにテーブルを足して使う。
const base = "name = \"x\"\ncommand = \"x\"\nprotocol = \"0.12\"\n"

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
		Name: "react-router", Command: []string{"node", "dist/main.js"}, Protocol: "0.12",
		Vocabulary:  []string{"vocab/page.hozo"},
		Transpilers: []Transpiler{{Name: "react-router", Language: "ts", Requires: []string{"ts", "ts-valid"}, Unsupported: []string{}, Seams: []string{"rate.limited"}}},
		Check:       Check{Enabled: true},
		Import:      Import{Formats: []string{}},
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got %+v\nwant %+v", m, want)
	}
	if !m.HasEmit() || m.HasImport() {
		t.Error("HasEmit/HasImport")
	}
	if tr := m.Transpiler("react-router"); tr == nil || tr.Language != "ts" || tr.IsBase() || m.Transpiler("nope") != nil {
		t.Errorf("Transpiler: %+v", tr)
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
	// requires を書かなければ基底（docs/config.md「基底と相乗り」）。省いた requires / unsupported / seams は []
	if !reflect.DeepEqual(m.Transpilers, []Transpiler{{Name: "go-cli", Language: "go", Requires: []string{}, Unsupported: []string{}, Seams: []string{}}}) || !m.Transpilers[0].IsBase() {
		t.Errorf("transpilers = %+v", m.Transpilers)
	}
	if m.Check.Enabled || len(m.Vocabulary) != 0 || m.Vocabulary == nil || m.HasImport() {
		t.Errorf("%+v", m)
	}
}

// リポジトリの plugins/go-cli のマニフェストが読めて、docs/plugin-go-cli.md「マニフェスト」のとおりであること。
func TestLoadManifestRepoGoCLI(t *testing.T) {
	m, diags := LoadManifest(filepath.Join("..", "..", "plugins", "go-cli", ManifestFileName))
	if len(diags) != 0 || m == nil {
		t.Fatalf("m=%+v diags=%+v", m, diags)
	}
	if tr := m.Transpiler("go-cli"); tr == nil || tr.Language != "go" || len(m.Transpilers) != 1 || !reflect.DeepEqual(m.Vocabulary, []string{"cli.hozo"}) {
		t.Errorf("%+v", m)
	}
}

func TestLoadManifestStringCommand(t *testing.T) {
	m, diags := LoadManifest(writeManifest(t, t.TempDir(), "name = \"x\"\ncommand = \"hozoc-plugin-x\"\nprotocol = \"0.12\"\n"))
	if len(diags) != 0 || !reflect.DeepEqual(m.Command, []string{"hozoc-plugin-x"}) || m.HasEmit() {
		t.Errorf("m=%+v diags=%+v", m, diags)
	}
}

// 一つのプラグインが複数の、言語の違うトランスパイラを提供できる（docs/plugin-protocol.md「マニフェスト」）。
func TestLoadManifestTranspilers(t *testing.T) {
	m, diags := LoadManifest(writeManifest(t, t.TempDir(), base+
		"[[transpiler]]\nname = \"a\"\nlanguage = \"go\"\nunsupported = [\"http.retry\"]\n"+
		"[[transpiler]]\nname = \"b\"\nlanguage = \"ts\"\n"))
	if len(diags) != 0 {
		t.Fatalf("diags = %+v", diags)
	}
	want := []Transpiler{{Name: "a", Language: "go", Requires: []string{}, Unsupported: []string{"http.retry"}, Seams: []string{}},
		{Name: "b", Language: "ts", Requires: []string{}, Unsupported: []string{}, Seams: []string{}}}
	if !reflect.DeepEqual(m.Transpilers, want) {
		t.Errorf("transpilers = %+v", m.Transpilers)
	}
}

// docs/plugin-protocol.md「診断」の MAN-2 / MAN-4。
func TestLoadManifestDiagnostics(t *testing.T) {
	tr := "[[transpiler]]\nname = \"t\"\nlanguage = \"go\"\n"
	cases := []struct {
		name    string
		toml    string
		want    []string
		nilMani bool
	}{
		{"bad toml", "name = \n", []string{"MAN-4"}, true},
		{"unknown key", base + "foo = 1\n", []string{"MAN-4"}, true},
		{"unknown transpiler key", base + tr + "foo = 1\n", []string{"MAN-4"}, true},
		{"unknown table", base + "[extra]\na = 1\n", []string{"MAN-4"}, true},
		{"no command", "name = \"x\"\nprotocol = \"0.12\"\n", []string{"MAN-4"}, true},
		{"empty string command", "name = \"x\"\ncommand = \"\"\nprotocol = \"0.12\"\n", []string{"MAN-4"}, true},
		{"empty array command", "name = \"x\"\ncommand = []\nprotocol = \"0.12\"\n", []string{"MAN-4"}, true},
		{"command wrong type", "name = \"x\"\ncommand = 1\nprotocol = \"0.12\"\n", []string{"MAN-4"}, true},
		{"no name", "command = \"x\"\nprotocol = \"0.12\"\n", []string{"MAN-4"}, true},
		{"unsupported wrong type", base + tr + "unsupported = \"a\"\n", []string{"MAN-4"}, true},
		// 言語を持つのはトランスパイラ。トップレベルの language と [emit]（設計書 §4.6 の decorate / entry / stop を含む）は知らないキー
		{"top-level language is unknown", base + "language = \"go\"\n", []string{"MAN-4"}, true},
		{"emit table is unknown", base + "[emit]\nunsupported = []\n", []string{"MAN-4"}, true},
		{"entry is unknown", base + tr + "entry = [\"cli.command\"]\n", []string{"MAN-4"}, true},
		// [[transpiler]] の誤り
		{"transpiler without name", base + "[[transpiler]]\nlanguage = \"go\"\n", []string{"MAN-4"}, true},
		{"transpiler without language", base + "[[transpiler]]\nname = \"t\"\n", []string{"MAN-4"}, true},
		{"transpiler language empty", base + "[[transpiler]]\nname = \"t\"\nlanguage = \"\"\n", []string{"MAN-4"}, true},
		// 言語の名前は基底が決める。hozoc は一覧を持たない（docs/plugin-protocol.md「マニフェスト」）
		{"any language name", base + "[[transpiler]]\nname = \"t\"\nlanguage = \"rust\"\n", nil, false},
		// requires（docs/plugin-protocol.md「マニフェスト」）: 自分の名前、同じ名前が二度、文字列の配列でない
		{"requires", base + tr + "requires = [\"go\", \"go-valid\"]\n", nil, false},
		{"requires itself", base + tr + "requires = [\"go\", \"t\"]\n", []string{"MAN-4"}, true},
		{"requires twice", base + tr + "requires = [\"go\", \"go\"]\n", []string{"MAN-4"}, true},
		{"requires wrong type", base + tr + "requires = \"go\"\n", []string{"MAN-4"}, true},
		{"requires element wrong type", base + tr + "requires = [1]\n", []string{"MAN-4"}, true},
		// seams: 空の文字列、同じ FQN が二度、文字列の配列でない。基底も書ける
		{"seams", base + tr + "seams = [\"rate.limited\", \"trace.span\"]\n", nil, false},
		{"seams empty string", base + tr + "seams = [\"\"]\n", []string{"MAN-4"}, true},
		{"seams twice", base + tr + "seams = [\"rate.limited\", \"rate.limited\"]\n", []string{"MAN-4"}, true},
		{"seams wrong type", base + tr + "seams = \"rate.limited\"\n", []string{"MAN-4"}, true},
		{"transpiler twice", base + tr + tr, []string{"MAN-4"}, true},
		{"transpiler is a table", base + "[transpiler]\nname = \"t\"\nlanguage = \"go\"\n", []string{"MAN-4"}, true},
		// トランスパイラはなくてもよい（emit を持たない）
		{"no transpiler", base, nil, false},
		{"one transpiler", base + tr, nil, false},
		{"MAN-2", "name = \"x\"\ncommand = \"x\"\nprotocol = \"0.1\"\n", []string{"MAN-2"}, false},
		{"MAN-2 missing protocol", "name = \"x\"\ncommand = \"x\"\n", []string{"MAN-2"}, false},
		{"MAN-2 with transpiler", "name = \"x\"\ncommand = \"x\"\nprotocol = \"9\"\n" + tr, []string{"MAN-2"}, false},
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

// names は入っているプラグインの「名前=最初のトランスパイラの名前」の列（Discover の順）。
func names(plugins []*Plugin) []string {
	var out []string
	for _, p := range plugins {
		t := ""
		if len(p.Manifest.Transpilers) > 0 {
			t = p.Manifest.Transpilers[0].Name
		}
		out = append(out, p.Name+"="+t)
	}
	return out
}

// docs/plugin-protocol.md「探索」: 入っているプラグインは plugin_dirs、HOZO_PLUGIN_PATH、~/.hozo/plugins の直下の、
// マニフェストを持つディレクトリのすべて。同じ名前は近いものだけを使う。HOZO_PLUGIN_PATH の値とホームは呼ぶ側が
// SearchEnv で渡す（Discover はプロセスの環境を読まない。TestDiscoverIgnoresProcessEnvironment）。
func TestDiscoverOrder(t *testing.T) {
	root := t.TempDir()
	envDir := t.TempDir()
	home := t.TempDir()
	env := SearchEnv{PluginPath: envDir + string(filepath.ListSeparator) + t.TempDir(), Home: home}
	cfg := config.Default(root)
	cfg.Path = filepath.Join(root, "hozo.toml")
	cfg.Hozo.PluginDirs = []string{"plugins", "more"}

	// where は、どこのマニフェストかをトランスパイラの名前に書いたマニフェスト
	manifest := func(name, where string) string {
		return "name = \"" + name + "\"\ncommand = \"p\"\nprotocol = \"0.12\"\n[[transpiler]]\nname = \"" + where + "\"\nlanguage = \"go\"\n"
	}
	plugins, diags := Discover(cfg, env)
	if len(plugins) != 0 || len(diags) != 0 {
		t.Fatalf("empty: %v %+v", names(plugins), diags)
	}

	homeDir := filepath.Join(home, ".hozo", "plugins", "p")
	writeManifest(t, homeDir, manifest("p", "home"))
	writeManifest(t, filepath.Join(home, ".hozo", "plugins", "z"), manifest("z", "home-z"))
	plugins, diags = Discover(cfg, env)
	if len(diags) != 0 || !reflect.DeepEqual(names(plugins), []string{"p=home", "z=home-z"}) || plugins[0].Dir != homeDir {
		t.Fatalf("home: %v diags=%+v", names(plugins), diags)
	}

	writeManifest(t, filepath.Join(envDir, "p"), manifest("p", "env"))
	plugins, _ = Discover(cfg, env)
	if !reflect.DeepEqual(names(plugins), []string{"p=env", "z=home-z"}) {
		t.Fatalf("env: %v", names(plugins))
	}

	writeManifest(t, filepath.Join(root, "more", "p"), manifest("p", "more"))
	writeManifest(t, filepath.Join(root, "more", "a"), manifest("a", "more-a"))
	plugins, _ = Discover(cfg, env)
	if !reflect.DeepEqual(names(plugins), []string{"a=more-a", "p=more", "z=home-z"}) {
		t.Fatalf("more: %v", names(plugins))
	}

	pluginsDir := filepath.Join(root, "plugins", "p")
	writeManifest(t, pluginsDir, manifest("p", "plugins"))
	// マニフェストのないディレクトリと、ディレクトリでないものは入っているプラグインではない
	if err := os.MkdirAll(filepath.Join(root, "plugins", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugins", "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plugins, diags = Discover(cfg, env)
	if len(diags) != 0 || !reflect.DeepEqual(names(plugins), []string{"p=plugins", "a=more-a", "z=home-z"}) {
		t.Fatalf("plugins: %v diags=%+v", names(plugins), diags)
	}
	if plugins[0].Dir != pluginsDir || !filepath.IsAbs(plugins[0].Dir) || plugins[0].ManifestPath() != filepath.Join(pluginsDir, ManifestFileName) {
		t.Errorf("dir %q", plugins[0].Dir)
	}
}

// Discover はプロセスの環境変数とホームディレクトリを読まない。
func TestDiscoverIgnoresProcessEnvironment(t *testing.T) {
	root, hostile := t.TempDir(), t.TempDir()
	writeManifest(t, filepath.Join(hostile, "broken"), "name = \n")
	writeManifest(t, filepath.Join(hostile, ".hozo", "plugins", "broken-home"), "name = \n")
	t.Setenv(PluginPathEnv, hostile)
	t.Setenv("HOME", hostile)
	t.Setenv("USERPROFILE", hostile)
	cfg := config.Default(root)
	cfg.Hozo.PluginDirs = []string{"plugins"}
	if dirs := SearchDirs(cfg, SearchEnv{}); !reflect.DeepEqual(dirs, []string{filepath.Join(root, "plugins")}) {
		t.Errorf("SearchDirs = %v", dirs)
	}
	if plugins, diags := Discover(cfg, SearchEnv{}); len(plugins) != 0 || len(diags) != 0 {
		t.Errorf("plugins=%v diags=%+v", names(plugins), diags)
	}
	// 渡せば読む
	if _, diags := Discover(cfg, SearchEnv{PluginPath: hostile, Home: hostile}); !reflect.DeepEqual(codes(diags), []string{"MAN-4", "MAN-4"}) {
		t.Errorf("diags=%+v", diags)
	}
}

func TestDiscoverNameMismatch(t *testing.T) {
	root := t.TempDir()
	env := SearchEnv{}
	cfg := config.Default(root)
	cfg.Hozo.PluginDirs = []string{"plugins"}
	p := writeManifest(t, filepath.Join(root, "plugins", "p"), "name = \"q\"\ncommand = \"q\"\nprotocol = \"0.12\"\n")
	plugins, diags := Discover(cfg, env)
	if len(plugins) != 0 || !reflect.DeepEqual(codes(diags), []string{"MAN-3"}) || diags[0].File != p {
		t.Errorf("plugins=%v diags=%+v", names(plugins), diags)
	}
}

func TestDiscoverBadManifest(t *testing.T) {
	// 同じ名前は最初に見つかったものだけを使う: plugin_dirs のマニフェストが壊れていても HOME のものには進まない。
	// 読めないマニフェスト（MAN-4）は、使っていなくても報告する
	root := t.TempDir()
	home := t.TempDir()
	env := SearchEnv{Home: home}
	cfg := config.Default(root)
	cfg.Hozo.PluginDirs = []string{"plugins"}
	writeManifest(t, filepath.Join(root, "plugins", "p"), "name = \n")
	writeManifest(t, filepath.Join(home, ".hozo", "plugins", "p"), "name = \"p\"\ncommand = \"p\"\nprotocol = \"0.12\"\n")
	plugins, diags := Discover(cfg, env)
	if len(plugins) != 0 || !reflect.DeepEqual(codes(diags), []string{"MAN-4"}) {
		t.Errorf("plugins=%v diags=%+v", names(plugins), diags)
	}
	// protocol の不一致（MAN-2）はプラグインを返し、診断は Mismatch に置く（使うプラグインにだけ報告する）
	writeManifest(t, filepath.Join(root, "plugins", "p"), "name = \"p\"\ncommand = \"p\"\nprotocol = \"0.1\"\n")
	plugins, diags = Discover(cfg, env)
	if len(plugins) != 1 || len(diags) != 0 || !reflect.DeepEqual(codes(plugins[0].Mismatch), []string{"MAN-2"}) {
		t.Errorf("plugins=%v diags=%+v", names(plugins), diags)
	}
}
