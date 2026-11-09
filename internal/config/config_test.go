package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
)

// docs/config.md「形」の例。
const example = `
[hozo]
sources      = ["**/*.hozo"]
plugin_dirs  = ["plugins"]
max_bindings = 5

[browser]
language = "go"
plugins  = ["react-router"]
out      = "web/src/generated"

[server]
language = "go"
plugins  = ["go-cli"]
out      = "internal/gen"

[server.admin]
plugins = []
`

func write(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, FileName)
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

func str(s string) *string { return &s }

func TestLoadExample(t *testing.T) {
	p := write(t, example)
	cfg, diags := Load(p)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if cfg.Root != filepath.Dir(p) || cfg.Path != p {
		t.Errorf("Root=%q Path=%q", cfg.Root, cfg.Path)
	}
	wantHozo := HozoTable{Sources: []string{"**/*.hozo"}, PluginDirs: []string{"plugins"}, MaxBindings: 5}
	if !reflect.DeepEqual(cfg.Hozo, wantHozo) {
		t.Errorf("Hozo = %+v, want %+v", cfg.Hozo, wantHozo)
	}
	want := map[string]NamespaceTable{
		"browser":      {Language: str("go"), Plugins: []string{"react-router"}, Out: str("web/src/generated")},
		"server":       {Language: str("go"), Plugins: []string{"go-cli"}, Out: str("internal/gen")},
		"server.admin": {Plugins: []string{}},
	}
	if !reflect.DeepEqual(cfg.Namespaces, want) {
		t.Errorf("Namespaces = %+v, want %+v", cfg.Namespaces, want)
	}
	if got := cfg.NamespaceNames(); !reflect.DeepEqual(got, []string{"browser", "server", "server.admin"}) {
		t.Errorf("NamespaceNames = %v", got)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, diags := Load(write(t, "[server]\nlanguage = \"go\"\nout = \"gen\"\n"))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if !reflect.DeepEqual(cfg.Hozo.Sources, DefaultSources) || len(cfg.Hozo.PluginDirs) != 0 || cfg.Hozo.MaxBindings != DefaultMaxBindings {
		t.Errorf("defaults: %+v", cfg.Hozo)
	}
	if cfg.Namespaces["server"].Plugins != nil {
		t.Errorf("plugins should be nil when unset")
	}
	conf, ok := cfg.Resolve("server")
	if !ok || len(conf.Plugins) != 0 || conf.Plugins == nil {
		t.Errorf("Resolve: %+v %v", conf, ok)
	}
}

// docs/config.md「形」: インラインテーブルも子の名前空間。
func TestLoadInlineChild(t *testing.T) {
	cfg, diags := Load(write(t, "[server]\nlanguage = \"go\"\nout = \"gen\"\nadmin = { plugins = [\"go-cli\"] }\n"))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if !reflect.DeepEqual(cfg.Namespaces["server.admin"], NamespaceTable{Plugins: []string{"go-cli"}}) {
		t.Errorf("server.admin = %+v", cfg.Namespaces["server.admin"])
	}
}

// docs/config.md「名前空間の設定の解決」。
func TestResolve(t *testing.T) {
	cfg, diags := Load(write(t, example+"\n[server.admin.audit]\nout = \"internal/audit\"\n"))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	cases := []struct {
		ns   string
		want NamespaceConf
		ok   bool
	}{
		{"server", NamespaceConf{Language: "go", Plugins: []string{"go-cli"}, Out: "internal/gen"}, true},
		// 子は書いたキーだけ上書き。out は同じ値
		{"server.admin", NamespaceConf{Language: "go", Plugins: []string{}, Out: "internal/gen"}, true},
		// 孫: out は自分、plugins は親（空）、language は祖父
		{"server.admin.audit", NamespaceConf{Language: "go", Plugins: []string{}, Out: "internal/audit"}, true},
		// テーブルのない子は親を引き継ぐ
		{"server.other", NamespaceConf{Language: "go", Plugins: []string{"go-cli"}, Out: "internal/gen"}, true},
		{"server.admin.x.y", NamespaceConf{Language: "go", Plugins: []string{}, Out: "internal/gen"}, true},
		// どこにもなければ未設定
		{"unknown", NamespaceConf{}, false},
		{"", NamespaceConf{}, false},
	}
	for _, c := range cases {
		got, ok := cfg.Resolve(c.ns)
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("Resolve(%q) = %+v, %v; want %+v, %v", c.ns, got, ok, c.want, c.ok)
		}
	}
	// 返した Plugins を書き換えても設定は変わらない
	got, _ := cfg.Resolve("server")
	got.Plugins[0] = "x"
	if cfg.Namespaces["server"].Plugins[0] != "go-cli" {
		t.Error("Resolve must copy plugins")
	}
}

// docs/config.md「名前空間の設定の解決」の Dir(N)。
func TestDir(t *testing.T) {
	cfg, diags := Load(write(t, example+`
[server.admin.audit]
out = "internal/audit"

[server.ops]
out = "internal/gen"
`))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	cases := []struct {
		ns   string
		want string
		ok   bool
	}{
		// out を自分で書いた名前空間は P が空
		{"server", "internal/gen", true},
		{"browser", "web/src/generated", true},
		// 親の out を継いだ子はそのセグメントのサブディレクトリ
		{"server.admin", "internal/gen/admin", true},
		// テーブルのない子も同じ
		{"server.other", "internal/gen/other", true},
		{"server.other.deep", "internal/gen/other/deep", true},
		// 親と同じ out を自分でも書いた子は、自分が A なので P は空
		{"server.ops", "internal/gen", true},
		{"server.ops.x", "internal/gen/x", true},
		// 三段: 孫が out を書けば孫が A
		{"server.admin.audit", "internal/audit", true},
		{"server.admin.audit.log", "internal/audit/log", true},
		// 孫のテーブルが out を書かなければ、out を書いた最も近い祖先（server）から
		{"server.admin.x.y", "internal/gen/admin/x/y", true},
		// どこにもなければ未設定
		{"unknown", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := cfg.Dir(c.ns)
		if ok != c.ok || got != c.want {
			t.Errorf("Dir(%q) = %q, %v; want %q, %v", c.ns, got, ok, c.want, c.ok)
		}
	}
}

func TestDirWithoutOut(t *testing.T) {
	// 祖先のどれも out を書いていなければ P は空で、Dir は Conf.out（""）のまま。
	cfg, diags := Load(write(t, "[a]\nlanguage = \"go\"\n"))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if got, ok := cfg.Dir("a.b"); !ok || got != "" {
		t.Errorf("Dir(a.b) = %q, %v", got, ok)
	}
}

func TestResolvePartialParent(t *testing.T) {
	// 親に language だけ、子に out だけ
	cfg, diags := Load(write(t, "[a]\nlanguage = \"go\"\n[a.b]\nout = \"gen\"\n"))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if got, ok := cfg.Resolve("a.b"); !ok || got.Language != "go" || got.Out != "gen" {
		t.Errorf("a.b = %+v", got)
	}
	// 親だけでは out が未設定（CFG-1 は check が判定する）
	if got, ok := cfg.Resolve("a"); !ok || got.Out != "" || got.Language != "go" {
		t.Errorf("a = %+v", got)
	}
}

func TestDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		toml string
		want []string
	}{
		{"bad toml", "[server\nlanguage = \"go\"\n", []string{"CFG-7"}},
		{"unknown top-level key", "foo = 1\n", []string{"CFG-7"}},
		{"unknown hozo key", "[hozo]\nfoo = 1\n", []string{"CFG-7"}},
		{"unknown namespace key", "[server]\nlanguage = \"go\"\nout = \"x\"\nfoo = \"bar\"\n", []string{"CFG-7"}},
		{"language wrong type", "[server]\nlanguage = 1\n", []string{"CFG-7"}},
		{"language unknown", "[server]\nlanguage = \"rust\"\n", []string{"CFG-7"}},
		{"language ts", "[server]\nlanguage = \"ts\"\nout = \"x\"\n", []string{"CFG-11"}},
		{"out wrong type", "[server]\nout = [\"x\"]\n", []string{"CFG-7"}},
		{"plugins wrong type", "[server]\nplugins = \"go-cli\"\n", []string{"CFG-7"}},
		{"plugins element wrong type", "[server]\nplugins = [1]\n", []string{"CFG-7"}},
		{"sources wrong type", "[hozo]\nsources = \"**/*.hozo\"\n", []string{"CFG-7"}},
		{"plugin_dirs wrong type", "[hozo]\nplugin_dirs = [1]\n", []string{"CFG-7"}},
		{"max_bindings wrong type", "[hozo]\nmax_bindings = \"5\"\n", []string{"CFG-7"}},
		{"max_bindings negative", "[hozo]\nmax_bindings = -1\n", []string{"CFG-7"}},
		{"hozo not a table", "hozo = 1\n", []string{"CFG-7"}},
		{"array of tables", "[[server]]\nlanguage = \"go\"\n", []string{"CFG-7"}},
		{"reserved table language", "[language]\nout = \"x\"\n", []string{"CFG-2"}},
		{"reserved table plugins", "[plugins]\n", []string{"CFG-2"}},
		{"reserved table out", "[out]\n", []string{"CFG-2"}},
		{"reserved child hozo", "[server.hozo]\n", []string{"CFG-2"}},
		{"reserved child language", "[server.language]\nx = 1\n", []string{"CFG-2"}},
		{"reserved child plugins table", "[server.plugins]\n", []string{"CFG-2"}},
		{"reserved child out", "[server.out]\n", []string{"CFG-2"}},
		{"several", "[server]\nlanguage = \"rust\"\nfoo = 1\n[out]\n", []string{"CFG-2", "CFG-7", "CFG-7"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := write(t, c.toml)
			cfg, diags := Load(p)
			if cfg == nil {
				t.Fatal("Load returned nil Config")
			}
			if got := codes(diags); !reflect.DeepEqual(got, c.want) {
				t.Errorf("codes = %v, want %v (%+v)", got, c.want, diags)
			}
			for _, d := range diags {
				if d.Severity != diag.Error || d.File != p || d.Span != nil || d.Message == "" {
					t.Errorf("bad diagnostic %+v", d)
				}
			}
		})
	}
}

func TestLoadKeepsValidParts(t *testing.T) {
	cfg, diags := Load(write(t, "[server]\nlanguage = \"go\"\nout = \"gen\"\nfoo = 1\n[out]\n"))
	if len(diags) != 2 {
		t.Fatalf("diags = %+v", diags)
	}
	if conf, ok := cfg.Resolve("server"); !ok || conf.Language != "go" || conf.Out != "gen" {
		t.Errorf("server = %+v, %v", conf, ok)
	}
	if _, ok := cfg.Namespaces["out"]; ok {
		t.Error("reserved table must not be kept")
	}
}

func TestLoadMissing(t *testing.T) {
	cfg, diags := Load(filepath.Join(t.TempDir(), FileName))
	if cfg == nil || len(diags) != 1 || diags[0].Code != "CFG-7" {
		t.Errorf("cfg=%v diags=%+v", cfg, diags)
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := Find(sub); ok {
		t.Error("found hozo.toml where there is none")
	}
	p := filepath.Join(root, FileName)
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, start := range []string{sub, filepath.Join(root, "a"), root} {
		got, ok := Find(start)
		if !ok || got != p {
			t.Errorf("Find(%q) = %q, %v", start, got, ok)
		}
	}
	// ディレクトリの hozo.toml は無視する
	if err := os.Mkdir(filepath.Join(sub, FileName), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := Find(sub); !ok || got != p {
		t.Errorf("Find with dir named hozo.toml = %q, %v", got, ok)
	}
}

func TestDefault(t *testing.T) {
	cfg := Default("/x")
	if cfg.Root != "/x" || cfg.Path != "" || cfg.Hozo.MaxBindings != DefaultMaxBindings || len(cfg.Namespaces) != 0 {
		t.Errorf("Default = %+v", cfg)
	}
	if _, ok := cfg.Resolve("server"); ok {
		t.Error("Default must not resolve any namespace")
	}
}

func TestIsReservedNamespaceName(t *testing.T) {
	for _, n := range []string{"language", "plugins", "out", "hozo"} {
		if !IsReservedNamespaceName(n) {
			t.Errorf("%s should be reserved", n)
		}
	}
	if IsReservedNamespaceName("server") {
		t.Error("server should not be reserved")
	}
}
