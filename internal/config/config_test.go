package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
)

// docs/config.md「形」の例。
const example = `
[hozo]
sources      = ["**/*.hozo"]
plugin_dirs  = ["plugins"]
max_bindings = 5

[[emit]]
in          = ["api/**/*.hozo"]
namespace   = "server"
transpilers = ["go-http", "sqlc"]
out         = "api/internal/generated"

[[emit]]
namespace   = "server"
transpilers = ["go-cli"]
out         = "cmd/gen"
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
	want := []Unit{
		{Index: 1, In: []string{"api/**/*.hozo"}, Namespace: "server", Transpilers: []string{"go-http", "sqlc"}, Out: "api/internal/generated", Options: map[string]map[string]any{}},
		{Index: 2, Namespace: "server", Transpilers: []string{"go-cli"}, Out: "cmd/gen", Options: map[string]map[string]any{}}, // in を省けば nil。options を書かなければ空
	}
	if !reflect.DeepEqual(cfg.Units, want) {
		t.Errorf("Units = %+v, want %+v", cfg.Units, want)
	}
	if got := cfg.UnitNamespaces(); !reflect.DeepEqual(got, []string{"server"}) {
		t.Errorf("UnitNamespaces = %v", got)
	}
	if got := cfg.Units[1].Label(); got != `2 番目の [[emit]]（namespace = "server"、out = "cmd/gen"）` {
		t.Errorf("Label = %s", got)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, diags := Load(write(t, "[[emit]]\nnamespace = \"server\"\ntranspilers = [\"t\"]\nout = \"gen\"\n"))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if !reflect.DeepEqual(cfg.Hozo.Sources, DefaultSources) || len(cfg.Hozo.PluginDirs) != 0 || cfg.Hozo.MaxBindings != DefaultMaxBindings {
		t.Errorf("defaults: %+v", cfg.Hozo)
	}
	if len(cfg.Units) != 1 || cfg.Units[0].In != nil {
		t.Errorf("Units = %+v", cfg.Units)
	}
	// [[emit]] が一つもなくてもよい（何も出力しない）
	cfg, diags = Load(write(t, "# 単位なし\n"))
	if len(diags) != 0 || len(cfg.Units) != 0 {
		t.Errorf("no units: %+v %+v", cfg.Units, diags)
	}
}

// units は namespace と out の組から単位の列を作る hozo.toml。
func units(pairs ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		b.WriteString("[[emit]]\nnamespace = \"" + pairs[i] + "\"\ntranspilers = [\"t\"]\nout = \"" + pairs[i+1] + "\"\n")
	}
	return b.String()
}

// docs/config.md「単位の対象」の手続き（CFG-13）。
func TestCovering(t *testing.T) {
	// 0: server、1: browser、2: server（同じ名前空間の二つ目の単位）、3: server.admin（子孫を名指しする単位）
	cfg, diags := Load(write(t, units("server", "gen/api", "browser", "gen/web", "server", "gen/cli", "server.admin", "gen/admin")))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	cases := []struct {
		ns   string
		want []int
	}{
		// 同じ名前空間を複数の単位に書ける（設定の順）
		{"server", []int{0, 2}},
		{"browser", []int{1}},
		// 単位は namespace とすべての子孫を対象にする
		{"server.other", []int{0, 2}},
		{"server.other.deep", []int{0, 2}},
		{"browser.pages", []int{1}},
		// 別の単位が子孫を namespace に書いていれば、その子孫とその下は祖先の単位の対象から外れる
		{"server.admin", []int{3}},
		{"server.admin.audit", []int{3}},
		// どの単位の対象でもない
		{"unknown", nil},
		{"serverx", nil},
		{"", nil},
	}
	for _, c := range cases {
		if got := cfg.Covering(c.ns); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Covering(%q) = %v, want %v", c.ns, got, c.want)
		}
	}
}

// docs/config.md「単位の対象」の Dir(U, N)。
func TestDir(t *testing.T) {
	cfg, diags := Load(write(t, units("server", "internal/gen", "server.admin", "./internal/admin/", "server", "cmd/gen")))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	cases := []struct {
		unit int
		ns   string
		want string
	}{
		// 単位の namespace 自身は out
		{0, "server", "internal/gen"},
		// 子孫は、namespace からのセグメントをパスにして足す
		{0, "server.other", "internal/gen/other"},
		{0, "server.other.deep", "internal/gen/other/deep"},
		// 同じ名前空間の別の単位は、自分の out の下
		{2, "server", "cmd/gen"},
		{2, "server.other", "cmd/gen/other"},
		// out は ./ や余分な / を取り除いた形
		{1, "server.admin", "internal/admin"},
		{1, "server.admin.audit", "internal/admin/audit"},
	}
	for _, c := range cases {
		if got := cfg.Dir(c.unit, c.ns); got != c.want {
			t.Errorf("Dir(%d, %q) = %q, want %q", c.unit, c.ns, got, c.want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	unit := "[[emit]]\nnamespace = \"server\"\ntranspilers = [\"t\"]\nout = \"x\"\n"
	cases := []struct {
		name  string
		toml  string
		want  []string
		units int // 残る単位の数
	}{
		{"bad toml", "[[emit\nnamespace = \"go\"\n", []string{"CFG-7"}, 0},
		{"unknown top-level key", "foo = 1\n", []string{"CFG-7"}, 0},
		// 名前空間ごとのテーブルはない: [hozo] と [[emit]] 以外のトップレベルのキーは知らないキー
		{"namespace table", "[server]\nlanguage = \"go\"\nout = \"x\"\n", []string{"CFG-7"}, 0},
		{"table named language", "[language]\nout = \"x\"\n", []string{"CFG-7"}, 0},
		{"unknown hozo key", "[hozo]\nfoo = 1\n", []string{"CFG-7"}, 0},
		{"unknown unit key", unit + "language = \"go\"\n", []string{"CFG-7"}, 0},
		{"emit is a table", "[emit]\nnamespace = \"server\"\ntranspilers = [\"t\"]\nout = \"x\"\n", []string{"CFG-7"}, 0},
		{"emit is a value", "emit = 1\n", []string{"CFG-7"}, 0},
		{"namespace wrong type", "[[emit]]\nnamespace = 1\ntranspilers = [\"t\"]\nout = \"x\"\n", []string{"CFG-7"}, 0},
		{"out wrong type", "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"t\"]\nout = [\"x\"]\n", []string{"CFG-7"}, 0},
		{"transpilers wrong type", "[[emit]]\nnamespace = \"a\"\ntranspilers = \"t\"\nout = \"x\"\n", []string{"CFG-7"}, 0},
		{"transpilers element wrong type", "[[emit]]\nnamespace = \"a\"\ntranspilers = [1]\nout = \"x\"\n", []string{"CFG-7"}, 0},
		{"transpilers twice", "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"t\", \"t\"]\nout = \"x\"\n", []string{"CFG-7"}, 0},
		{"in wrong type", unit + "in = \"*.hozo\"\n", []string{"CFG-7"}, 0},
		{"sources wrong type", "[hozo]\nsources = \"**/*.hozo\"\n", []string{"CFG-7"}, 0},
		{"plugin_dirs wrong type", "[hozo]\nplugin_dirs = [1]\n", []string{"CFG-7"}, 0},
		{"max_bindings wrong type", "[hozo]\nmax_bindings = \"5\"\n", []string{"CFG-7"}, 0},
		{"max_bindings negative", "[hozo]\nmax_bindings = -1\n", []string{"CFG-7"}, 0},
		{"hozo not a table", "hozo = 1\n", []string{"CFG-7"}, 0},
		// CFG-12: 必須のキーがない、または空
		{"no namespace", "[[emit]]\ntranspilers = [\"t\"]\nout = \"x\"\n", []string{"CFG-12"}, 0},
		{"no transpilers", "[[emit]]\nnamespace = \"a\"\nout = \"x\"\n", []string{"CFG-12"}, 0},
		{"no out", "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"t\"]\n", []string{"CFG-12"}, 0},
		{"empty unit", "[[emit]]\n", []string{"CFG-12"}, 0},
		{"empty namespace", "[[emit]]\nnamespace = \"\"\ntranspilers = [\"t\"]\nout = \"x\"\n", []string{"CFG-12"}, 0},
		{"empty transpilers", "[[emit]]\nnamespace = \"a\"\ntranspilers = []\nout = \"x\"\n", []string{"CFG-12"}, 0},
		{"empty transpiler name", "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"\"]\nout = \"x\"\n", []string{"CFG-12"}, 0},
		{"empty out", "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"t\"]\nout = \"\"\n", []string{"CFG-12"}, 0},
		// CFG-12 の単位は使わないが、ほかの単位は残る
		{"one bad unit", unit + "[[emit]]\nnamespace = \"b\"\n", []string{"CFG-12"}, 1},
		// CFG-17: 二つの単位の out が同じか、一方が他方の下
		{"same out", units("a", "gen", "b", "gen"), []string{"CFG-17"}, 2},
		{"same out after cleaning", units("a", "gen/x", "b", "./gen//x/"), []string{"CFG-17"}, 2},
		{"nested out", units("a", "gen", "b", "gen/b"), []string{"CFG-17"}, 2},
		{"nested out the other way", units("a", "gen/a/deep", "b", "gen/a"), []string{"CFG-17"}, 2},
		{"descendant unit under ancestor out", units("server", "gen", "server.admin", "gen/admin"), []string{"CFG-17"}, 2},
		{"same namespace same out", units("a", "gen", "a", "gen"), []string{"CFG-17"}, 2},
		// CFG-7: out はルートの下のディレクトリ。ルートそのもの、ルートの外、絶対パスは使えない（その単位は使わない）
		{"root out", units("a", ".", "b", "gen"), []string{"CFG-7"}, 1},
		{"root out after cleaning", units("a", "gen/..", "b", "gen"), []string{"CFG-7"}, 1},
		{"root out written as ./", units("a", "./"), []string{"CFG-7"}, 0},
		{"out above root", units("a", "../x"), []string{"CFG-7"}, 0},
		{"out is parent", units("a", ".."), []string{"CFG-7"}, 0},
		{"out leaves root after cleaning", units("a", "gen/../../x"), []string{"CFG-7"}, 0},
		{"absolute out", units("a", "/tmp/x"), []string{"CFG-7"}, 0},
		{"out with dots inside root is fine", units("a", "gen/../gen2/./a", "b", "..gen"), nil, 2},
		// ルートの下でない out の単位は使わないので、ほかの単位との CFG-17 にもならない
		{"bad out does not overlap", units("a", "../gen", "b", "gen"), []string{"CFG-7"}, 1},
		{"sibling outs are fine", units("a", "gen/a", "b", "gen/b", "a", "gen/ab"), nil, 3},
		{"several", "[server]\n" + unit + "foo = 1\n[[emit]]\nout = \"y\"\n", []string{"CFG-12", "CFG-7", "CFG-7"}, 0},
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
			if len(cfg.Units) != c.units {
				t.Errorf("Units = %+v, want %d", cfg.Units, c.units)
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
	cfg, diags := Load(write(t, "[hozo]\nmax_bindings = 3\nfoo = 1\n"+units("server", "gen")+"[out]\n"))
	if len(diags) != 2 {
		t.Fatalf("diags = %+v", diags)
	}
	if cfg.Hozo.MaxBindings != 3 || len(cfg.Units) != 1 || cfg.Units[0].Namespace != "server" {
		t.Errorf("cfg = %+v", cfg)
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
	if cfg.Root != "/x" || cfg.Path != "" || cfg.Hozo.MaxBindings != DefaultMaxBindings || len(cfg.Units) != 0 {
		t.Errorf("Default = %+v", cfg)
	}
	if got := cfg.Covering("server"); got != nil {
		t.Errorf("Default must not cover any namespace: %v", got)
	}
}

// docs/config.md「オプション」: [emit.options.<名前>] は直前の [[emit]] の単位に付き、値は JSON にそのまま写せるもの。
func TestLoadOptions(t *testing.T) {
	p := write(t, `
[[emit]]
namespace   = "server"
transpilers = ["go", "go-valid", "go-http"]
out         = "gen/server"

[emit.options.go-http]
validate = true
prefix   = "/api"
retries  = 3
ratio    = 0.5
tags     = ["a", "b"]

[emit.options.go-http.limits]
max = 10

[[emit.options.go-http.routes]]
path = "/x"

[[emit]]
namespace   = "type"
transpilers = ["go"]
out         = "gen/type"
`)
	cfg, diags := Load(p)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(cfg.Units) != 2 {
		t.Fatalf("Units = %+v", cfg.Units)
	}
	want := map[string]map[string]any{"go-http": {
		"validate": true, "prefix": "/api", "retries": int64(3), "ratio": 0.5, "tags": []any{"a", "b"},
		"limits": map[string]any{"max": int64(10)},
		"routes": []any{map[string]any{"path": "/x"}},
	}}
	if !reflect.DeepEqual(cfg.Units[0].Options, want) {
		t.Errorf("Options = %#v\nwant %#v", cfg.Units[0].Options, want)
	}
	if got := cfg.Units[1].Options; got == nil || len(got) != 0 {
		t.Errorf("オプションのない単位の Options は空の写像: %#v", got)
	}
}

// docs/config.md「診断」の CFG-21: [emit.options.<名前>] の <名前> が、その単位の transpilers にない。単位は使う。
func TestOptionsUnknownTranspiler(t *testing.T) {
	cfg, diags := Load(write(t, `
[[emit]]
namespace   = "server"
transpilers = ["go", "go-http"]
out         = "gen/server"

[emit.options.go-htpp]
validate = true

[emit.options.go-http]
validate = true
`))
	if got := codes(diags); !reflect.DeepEqual(got, []string{"CFG-21"}) {
		t.Fatalf("codes = %v: %+v", got, diags)
	}
	if !strings.Contains(diags[0].Message, "go-htpp") || !strings.Contains(diags[0].Message, "1 番目の [[emit]]") {
		t.Errorf("メッセージに名前と単位がない: %s", diags[0].Message)
	}
	if len(cfg.Units) != 1 {
		t.Fatalf("CFG-21 の単位は使う: %+v", cfg.Units)
	}
	if _, ok := cfg.Units[0].Options["go-htpp"]; ok {
		t.Errorf("transpilers にない名前のオプションは持たない: %#v", cfg.Units[0].Options)
	}
	if cfg.Units[0].Options["go-http"]["validate"] != true {
		t.Errorf("Options = %#v", cfg.Units[0].Options)
	}
}

// docs/config.md「オプション」「診断」の CFG-7: options がテーブルのテーブルでない、オプションの値が日時。その単位は使わない。
func TestOptionsShape(t *testing.T) {
	head := "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"go\"]\nout = \"gen\"\n"
	for name, body := range map[string]string{
		"options がテーブルでない":      "options = 1\n",
		"options.<名前> がテーブルでない": "[emit.options]\ngo = true\n",
		"値が日時":          "[emit.options.go]\nat = 2024-01-02T03:04:05Z\n",
		"値が日付":          "[emit.options.go]\nat = 2024-01-02\n",
		"配列の中の日時":       "[emit.options.go]\nat = [1979-05-27T07:32:00Z]\n",
		"入れ子のテーブルの中の日時": "[emit.options.go.x]\nat = 07:32:00\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, diags := Load(write(t, head+body))
			if got := codes(diags); !reflect.DeepEqual(got, []string{"CFG-7"}) {
				t.Fatalf("codes = %v: %+v", got, diags)
			}
			if len(cfg.Units) != 0 {
				t.Errorf("CFG-7 の単位は使わない: %+v", cfg.Units)
			}
		})
	}
}
