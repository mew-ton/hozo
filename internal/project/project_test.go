package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
)

const corpusDir = "../../tests/corpus/design"

// repoPlugins は、リポジトリの plugins/（基底 go のマニフェストがある）。コーパスの単位のトランスパイラは、ここから
// HOZO_PLUGIN_PATH として渡して探す（docs/testing.md「コーパス」）。検査では起動しない。
func repoPlugins(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../plugins")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// loadCorpus は dir のプロジェクトを、リポジトリの plugins/ を HOZO_PLUGIN_PATH にして解析する。
func loadCorpus(t *testing.T, dir, configPath string, overlay map[string][]byte) *Project {
	t.Helper()
	return LoadWith(Options{Root: dir, ConfigPath: configPath, Overlay: overlay, PluginPath: repoPlugins(t)})
}

// codes は診断のコードの列。
func codes(diags []diag.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return out
}

func text(p *Project) string {
	var b strings.Builder
	_ = diag.WriteText(&b, p.Diags, p.Files)
	return b.String()
}

// write はディレクトリを作りながらファイルを書く。
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// copyCorpus は設計書のコーパスを一時ディレクトリに写す: 直下のファイルと、hozo.toml の単位のトランスパイラを提供する
// プラグインのマニフェスト（plugins/。検査では起動しない）。ゴールデン（testdata/）は写さない。
func copyCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.WalkDir(corpusDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(corpusDir, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel != "." && rel != "plugins" && !strings.HasPrefix(filepath.ToSlash(rel), "plugins/") {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		write(t, filepath.Join(dir, rel), string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestDesignCorpus は設計書のコーパス（docs/testing.md「コーパス」）: error も warning もない
// （tests/corpus/design/README.md の想定。§7.3 の placeOrder を配線に直した経緯はその表の 14）。
func TestDesignCorpus(t *testing.T) {
	p := loadCorpus(t, corpusDir, "", nil)
	if p.NoConfig {
		t.Fatal("NoConfig = true")
	}
	if p.Errors() != 0 {
		t.Fatalf("errors:\n%s", text(p))
	}
	if got := strings.Join(codes(p.Diags), ","); got != "" {
		t.Fatalf("codes = %v, want none\n%s", got, text(p))
	}
	if got := strings.Join(p.Sources, ","); got != "browser.hozo,server.hozo" {
		t.Fatalf("Sources = %v", p.Sources)
	}
	// 単位は四つ（server、browser と、使う型の置き場の http、page）。どれも基底 go で、in を省いているので
	// Files は Sources のすべてと、語彙のファイル
	if len(p.Units) != 4 || p.Units[0].Namespace != "server" || p.Units[0].Language != "go" || p.Units[0].Base != "go" ||
		len(p.Units[0].Transpilers) != 1 || p.Units[0].Transpilers[0].Name != "go" || p.Units[0].Transpilers[0].Plugin.Name != "go" ||
		!strings.HasPrefix(strings.Join(p.Units[0].Files, ","), "browser.hozo,server.hozo,hozo:vocab/") {
		t.Fatalf("Units = %+v", p.Units)
	}
	views := p.Views()
	if len(views) != 4 || views[0]["server"] == nil || views[1]["browser"] == nil || len(views[0]) != 1 {
		t.Fatalf("Views = %v", views)
	}
	if d := views[0]["server"].Config.Dir; d != "gen/server" {
		t.Fatalf("Dir(server) = %q", d)
	}
}

// TestConfigPath は --config に相当する configPath。相対パスは root からの相対。ルートは hozo.toml のディレクトリ。
func TestConfigPath(t *testing.T) {
	dir := copyCorpus(t)
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := loadCorpus(t, sub, "../hozo.toml", nil)
	if p.NoConfig || p.Errors() != 0 {
		t.Fatalf("NoConfig=%v errors:\n%s", p.NoConfig, text(p))
	}
	if abs, _ := filepath.EvalSymlinks(dir); filepath.Clean(p.Root) != filepath.Clean(abs) && filepath.Clean(p.Root) != filepath.Clean(dir) {
		t.Fatalf("Root = %q, want %q", p.Root, dir)
	}
	// 読めない configPath は CFG-7
	p = Load(dir, "nope.toml", nil)
	if p.NoConfig || !strings.Contains(strings.Join(codes(p.Diags), ","), "CFG-7") {
		t.Fatalf("want CFG-7, got %v (NoConfig=%v)", codes(p.Diags), p.NoConfig)
	}
	if p.Diags[0].File != "nope.toml" {
		t.Fatalf("File = %q, want ルートからの相対パス", p.Diags[0].File)
	}
}

// TestNoConfig は hozo.toml がないとき: 既定の設定で解析し、CFG-* を除く。プラグインも探さない。
func TestNoConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	p := Load(dir, "", nil)
	if !p.NoConfig {
		t.Fatal("NoConfig = false")
	}
	for _, c := range codes(p.Diags) {
		if strings.HasPrefix(c, "CFG-") {
			t.Fatalf("CFG-* が残っている: %v", codes(p.Diags))
		}
	}
	if p.Errors() != 0 {
		t.Fatalf("errors:\n%s", text(p))
	}
	if len(p.Sources) != 1 || p.Sources[0] != "a.hozo" {
		t.Fatalf("Sources = %v", p.Sources)
	}
	// 設定なしでは何も出力しない（docs/config.md「置き場と探し方」）。入っているプラグインも探さない
	if n := len(p.Views()); n != 0 || len(p.Units) != 0 || len(p.Plugins) != 0 {
		t.Fatalf("Views = %d, Units = %d, Plugins = %d, want 0", n, len(p.Units), len(p.Plugins))
	}
	// 設定なしでも検査は掛かる（CFG-* 以外の診断はそのまま）。名前空間の名前に使えない名前はない（out も language も書ける）
	write(t, filepath.Join(dir, "b.hozo"), "namespace out {\n  fn g(): Nope;\n}\nnamespace language { fn h(); }\n")
	p = Load(dir, "", nil)
	if got := strings.Join(codes(p.Diags), ","); !p.NoConfig || got != "RES-1" {
		t.Fatalf("codes = %v, want [RES-1]（NoConfig=%v）\n%s", got, p.NoConfig, text(p))
	}
	if err := os.Remove(filepath.Join(dir, "b.hozo")); err != nil {
		t.Fatal(err)
	}
	// 単位を書けば設定の検査が掛かる: out のない単位は CFG-12
	write(t, filepath.Join(dir, "hozo.toml"), "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"t\"]\n")
	p = Load(dir, "", nil)
	if p.NoConfig || strings.Join(codes(p.Diags), ",") != "CFG-12" {
		t.Fatalf("want CFG-12, got %v", codes(p.Diags))
	}
	if p.Diags[0].File != "hozo.toml" {
		t.Fatalf("File = %q, want hozo.toml", p.Diags[0].File)
	}
}

// unit は namespace と out とトランスパイラ t の単位一つ分の hozo.toml。
func unit(namespace, out string, transpilers ...string) string {
	if len(transpilers) == 0 {
		transpilers = []string{"t"}
	}
	return "[[emit]]\nnamespace = \"" + namespace + "\"\ntranspilers = [\"" + strings.Join(transpilers, "\", \"") + "\"]\nout = \"" + out + "\"\n"
}

// pluginDirs は [hozo] plugin_dirs = ["plugins"]。
const pluginDirs = "[hozo]\nplugin_dirs = [\"plugins\"]\n\n"

// TestUncoveredNamespace は docs/config.md「単位の対象」: どの単位の対象でもない名前空間は、state や op を
// 持っていても診断なしで、View を持たない。検査は同じに掛かり、単位の対象の名前空間の op を呼べる。
func TestUncoveredNamespace(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("app", "gen/app"))
	fakePlugin(t, dir, "t", "go", "", `{}`, 0)
	write(t, filepath.Join(dir, "app.hozo"), "namespace app {\n  exception Busy;\n  export fn save(text: string): throws Busy;\n}\n")
	write(t, filepath.Join(dir, "user.hozo"), "/** 利用者 */\nnamespace user {\n  state memo: nullable<string>;\n  fn write(text: string): throws app.Busy {\n    app.save(text);\n  }\n}\nfn globalOp();\n")
	p := Load(dir, "", nil)
	if p.NoConfig || len(p.Diags) != 0 {
		t.Fatalf("NoConfig=%v diags:\n%s", p.NoConfig, text(p))
	}
	views := p.Views()
	if len(views) != 1 || len(views[0]) != 1 || views[0]["app"] == nil {
		t.Fatalf("Views = %v, want app だけ", views)
	}
	if len(p.Config.Covering("user")) != 0 || len(views[0]["app"].Namespaces) != 1 {
		t.Fatal("user を対象にする単位がある（どの単位の対象でもない名前空間は出力先を持たない）")
	}
	if vs := views[0]["app"].Ops["user.write"]; vs == nil || strings.Join(vs.Exceptions, ",") != "app.Busy" {
		t.Fatalf("View(app).ops[user.write] = %+v", vs)
	}
	// 単位の対象でない名前空間にも検査は同じに掛かる: throws を外せば THROWS-1
	write(t, filepath.Join(dir, "user.hozo"), "namespace user {\n  fn write(text: string) {\n    app.save(text);\n  }\n}\n")
	p = Load(dir, "", nil)
	if got := strings.Join(codes(p.Diags), ","); got != "THROWS-1" {
		t.Fatalf("codes = %v, want [THROWS-1]\n%s", got, text(p))
	}
}

// TestOverlay は overlay がディスクの内容より優先し、ディスクにないファイルも足すこと。
func TestOverlay(t *testing.T) {
	dir := copyCorpus(t)
	broken := "namespace server {\n  model Broken { x: \n}\n"
	p := loadCorpus(t, dir, "", map[string][]byte{
		"server.hozo":                      []byte(broken),
		filepath.Join(dir, "extra.hozo"):   []byte("namespace extra {\n  model E { x: string; }\n}\n"),
		"gen/server/ignored.hozo":          []byte("garbage"), // out の下は除く
		filepath.Join(dir, "..", "x.hozo"): []byte("garbage"), // ルートの外
	})
	if got := strings.Join(p.Sources, ","); got != "browser.hozo,extra.hozo,server.hozo" {
		t.Fatalf("Sources = %v", p.Sources)
	}
	if p.Errors() == 0 || !strings.Contains(strings.Join(codes(p.Diags), ","), "SYNTAX") {
		t.Fatalf("overlay の構文エラーが出ていない: %v", codes(p.Diags))
	}
	for i := 0; i < p.Files.Len(); i++ {
		if p.Files.Path(source.FileID(i)) == "server.hozo" && string(p.Files.Content(source.FileID(i))) != broken {
			t.Fatal("server.hozo の内容が overlay でない")
		}
	}
}

// TestExcludesOutAndPluginDirs は各単位の out と plugin_dirs の下の .hozo を読まないこと、
// sources の glob（** とメタ文字なし）に従うこと。
func TestExcludesOutAndPluginDirs(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a")+unit("a.b", "gen/b"))
	fakePlugin(t, dir, "p", "go", "", `{}`, 0, "t")
	write(t, filepath.Join(dir, "spec", "a.hozo"), "namespace a {\n  fn f();\n  namespace b { fn g(); }\n}\n")
	write(t, filepath.Join(dir, "gen", "a", "x.hozo"), "garbage")
	write(t, filepath.Join(dir, "gen", "b", "deep", "x.hozo"), "garbage")
	write(t, filepath.Join(dir, "plugins", "p", "v.hozo"), "garbage")
	write(t, filepath.Join(dir, "other.txt"), "garbage")
	p := Load(dir, "", nil)
	if got := strings.Join(p.Sources, ","); got != "spec/a.hozo" {
		t.Fatalf("Sources = %v", p.Sources)
	}
	if p.Errors() != 0 {
		t.Fatalf("errors:\n%s", text(p))
	}
	// a.b は自分を namespace に書いた単位の対象で、a の単位の対象からは外れる（docs/config.md「単位の対象」）
	views := p.Views()
	if len(views[0]) != 1 || views[0]["a"] == nil || views[1]["a.b"] == nil || views[1]["a.b"].Config.Dir != "gen/b" {
		t.Fatalf("Views = %v", views)
	}

	// メタ文字のない sources はそのファイルだけ
	write(t, filepath.Join(dir, "hozo.toml"), "[hozo]\nsources = [\"spec/a.hozo\"]\nplugin_dirs = [\"plugins\"]\n\n"+unit("a", "gen/a"))
	write(t, filepath.Join(dir, "spec", "z.hozo"), "garbage")
	p = Load(dir, "", nil)
	if got := strings.Join(p.Sources, ","); got != "spec/a.hozo" {
		t.Fatalf("Sources = %v", p.Sources)
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"**/*.hozo", "a.hozo", true},
		{"**/*.hozo", "x/y/a.hozo", true},
		{"**/*.hozo", "a.txt", false},
		{"spec/**", "spec/a/b", true},
		{"spec/**", "other/a", false},
		{"spec/*.hozo", "spec/a.hozo", true},
		{"spec/*.hozo", "spec/x/a.hozo", false},
		{"./a.hozo", "a.hozo", true},
		{"a/**/b.hozo", "a/b.hozo", true},
		{"a/**/b.hozo", "a/x/y/b.hozo", true},
	}
	for _, tt := range tests {
		if got := matchGlob(tt.pattern, tt.name); got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

// fakePlugin はシェルで動く小さなプラグインを plugins/<name> に置く。stdout は応答の JSON、exit は終了コード。
// transpilers は提供するトランスパイラの名前（どれも言語は lang）。省けばプラグインと同じ名前の一つ。
// extra はマニフェストに足す行（トップレベルのキーと、[check] などのテーブル）。
func fakePlugin(t *testing.T, dir, name, lang, extra, stdout string, exit int, transpilers ...string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh がない")
	}
	if len(transpilers) == 0 {
		transpilers = []string{name}
	}
	script := "cat >/dev/null; printf '%s' '" + stdout + "'; exit " + string(rune('0'+exit))
	manifest := "name = \"" + name + "\"\ncommand = [\"sh\", \"-c\", \"" + strings.ReplaceAll(script, `"`, `\"`) + "\"]\nprotocol = \"0.13\"\n" + extra
	for _, tr := range transpilers {
		manifest += "\n[[transpiler]]\nname = \"" + tr + "\"\nlanguage = \"" + lang + "\"\n"
	}
	write(t, filepath.Join(dir, "plugins", name, "hozo-plugin.toml"), manifest)
}

// TestPluginCheck はプラグインの探索、使うプラグインの語彙の読み込み、[check] enabled の起動と診断の変換。
func TestPluginCheck(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "server.hozo"), "namespace server {\n  model M { id: string; }\n  /** 注文する */\n  fn placeOrder(m: M);\n}\n")
	write(t, filepath.Join(dir, "browser.hozo"), "namespace browser {\n  fn open();\n}\n")
	fakePlugin(t, dir, "pick", "go", "", `{}`, 0)
	config := func(serverTranspilers ...string) string {
		return pluginDirs + unit("server", "gen/server", serverTranspilers...) + unit("browser", "gen/browser", "pick")
	}
	write(t, filepath.Join(dir, "hozo.toml"), config("chk"))
	resp := `{"diagnostics":[{"severity":"warning","code":"CHK-1","at":"server.placeOrder","message":"by fqn"},{"severity":"error","code":"CHK-2","at":"server.hozo:3:5","message":"by pos"},{"severity":"error","code":"CHK-3","at":"nowhere","message":"unknown"}]}`
	fakePlugin(t, dir, "chk", "go", "vocabulary = [\"v.hozo\"]\n\n[check]\nenabled = true\n", resp, 0)
	write(t, filepath.Join(dir, "plugins", "chk", "v.hozo"), "/** 語彙 */\nnamespace chk {\n  decorator mark for op;\n}\n")
	// hozoc 自身の warning（NAME-1）を一つ足し、プラグインの診断と一緒に出ることを確かめる
	write(t, filepath.Join(dir, "extra.hozo"), "namespace server {\n  model lowerName { id: string; }\n}\n")
	hasFile := func(p *Project, path string) bool {
		for i := 0; i < p.Files.Len(); i++ {
			if p.Files.Path(source.FileID(i)) == path {
				return true
			}
		}
		return false
	}

	p := Load(dir, "", nil)
	if p.RunErr != nil {
		t.Fatalf("RunErr = %v", p.RunErr)
	}
	// 入っているプラグインは chk と pick。どちらも使うプラグイン
	if len(p.Plugins) != 2 || p.Plugins[0].Name != "chk" || p.Plugins[1].Name != "pick" || p.Units[0].Transpilers[0].Plugin != p.Plugins[0] {
		t.Fatalf("Plugins = %+v（%v）", p.Plugins, codes(p.Diags))
	}
	got := text(p)
	for _, want := range []string{
		"server.hozo:4:3: warning[CHK-1]: by fqn", // FQN → 宣言の位置
		"server.hozo:3:5: error[CHK-2]: by pos",   // file:line:col
		"hozoc: error[CHK-3]: unknown",            // 位置なし
		"extra.hozo:2:9: warning[NAME-1]",         // hozoc の診断と一緒に並ぶ
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
	if !hasFile(p, "hozo:plugin/chk/v.hozo") {
		t.Error("使うプラグインの語彙 hozo:plugin/chk/v.hozo が読まれていない")
	}
	// 語彙のファイルは、どの単位の Files にも入る（in によらず、いつも合ったものとして扱う。docs/config.md「in」）
	if files := strings.Join(p.Units[1].Files, ","); !strings.Contains(files, "hozo:plugin/chk/v.hozo") || !strings.Contains(files, "hozo:vocab/http.hozo") {
		t.Errorf("Units[1].Files = %v", files)
	}

	// check はプラグインと名前空間の組ごとに一回: 同じ名前空間の単位が二つあっても、診断は一度だけ（docs/plugin-protocol.md「起動」）
	write(t, filepath.Join(dir, "hozo.toml"), config("chk")+unit("server", "gen/server2", "chk"))
	p = Load(dir, "", nil)
	if n := strings.Count(text(p), "warning[CHK-1]"); n != 1 {
		t.Errorf("CHK-1 が %d 回出た（want 1）:\n%s", n, text(p))
	}

	// 入っているだけのプラグイン（どの単位もそのトランスパイラを書いていない）: 語彙を読まず、check も呼ばない
	write(t, filepath.Join(dir, "hozo.toml"), config("pick"))
	p = Load(dir, "", nil)
	if hasFile(p, "hozo:plugin/chk/v.hozo") || strings.Contains(text(p), "CHK-") || p.Errors() != 0 {
		t.Errorf("使っていないプラグインの語彙か check が効いている:\n%s", text(p))
	}
	if len(p.Plugins) != 2 {
		t.Errorf("Plugins = %d, want 2（入っているプラグインは探す）", len(p.Plugins))
	}

	// 異常終了は RunErr
	write(t, filepath.Join(dir, "hozo.toml"), config("chk"))
	fakePlugin(t, dir, "chk", "go", "[check]\nenabled = true\n", "", 3)
	p = Load(dir, "", nil)
	if p.RunErr == nil {
		t.Fatal("RunErr = nil, want proc.Crashed")
	}
}

// manifest は、起動しないプラグイン name のマニフェストを plugins/<name> に置く。transpilers は [[transpiler]] の中身
// （name と language のほかの行: requires、seams など）を、トランスパイラの名前 → 行で。
func manifest(t *testing.T, dir, name string, transpilers ...[2]string) {
	t.Helper()
	m := "name = \"" + name + "\"\ncommand = \"true\"\nprotocol = \"0.13\"\n"
	for _, tr := range transpilers {
		lang := "go"
		if strings.HasPrefix(tr[0], "ts") {
			lang = "ts"
		}
		m += "\n[[transpiler]]\nname = \"" + tr[0] + "\"\nlanguage = \"" + lang + "\"\n" + tr[1]
	}
	write(t, filepath.Join(dir, "plugins", name, "hozo-plugin.toml"), m)
}

// TestResolveTranspilers は docs/config.md「単位」「基底と相乗り」と docs/plugin-protocol.md「探索」: 単位の transpilers の名前を、
// 入っているすべてのプラグインのトランスパイラから探す。CFG-14（どのプラグインも提供していない）、CFG-15（二つ以上の
// プラグインが提供している）、CFG-16（一つの単位の言語がそろっていない）、CFG-19（requires の名前が transpilers にない）、
// CFG-20（基底がちょうど一つでない）。
func TestResolveTranspilers(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	// kit は基底 go と、それに相乗りする go-x（requires = ["go"]）、go と go-x に相乗りする go-y を提供する。
	// kit2 は、もう一つの基底 go-b。ts-kit は言語の違う基底 ts と、それに相乗りする ts-x、言語の違う基底に相乗りする ts-on-go
	manifest(t, dir, "kit", [2]string{"go", ""}, [2]string{"go-x", "requires = [\"go\"]\n"}, [2]string{"go-y", "requires = [\"go\", \"go-x\"]\n"})
	manifest(t, dir, "kit2", [2]string{"go-b", ""})
	manifest(t, dir, "ts-kit", [2]string{"ts", ""}, [2]string{"ts-x", "requires = [\"ts\"]\n"}, [2]string{"ts-on-go", "requires = [\"go\"]\n"})
	load := func(units string) *Project {
		write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+units)
		return Load(dir, "", nil)
	}
	want := func(p *Project, wantCodes string) {
		t.Helper()
		if got := strings.Join(codes(p.Diags), ","); got != wantCodes {
			t.Errorf("codes = %v, want %v\n%s", got, wantCodes, text(p))
		}
		for _, d := range p.Diags {
			if strings.HasPrefix(d.Code, "CFG-") && d.File != "hozo.toml" {
				t.Errorf("%s の File = %q, want hozo.toml", d.Code, d.File)
			}
		}
	}

	// トランスパイラの名前はプラグインの名前と別。一つのプラグインの二つのトランスパイラを同じ単位に書ける。
	// 単位の基底は、requires が空のただ一つのトランスパイラで、単位の言語は、その language。書いた順にはよらない
	p := load(unit("a", "gen/a", "go-y", "go-x", "go"))
	want(p, "")
	if u := p.Units[0]; u.Language != "go" || u.Base != "go" || len(u.Transpilers) != 3 || u.Transpilers[0].Plugin.Name != "kit" || u.Transpilers[1].Decl.Name != "go-x" {
		t.Errorf("Units[0] = %+v", u)
	}
	// CFG-14: プラグインの名前（kit）はトランスパイラの名前ではない。解決できない名前のある単位では、CFG-20 を出さない
	p = load(unit("a", "gen/a", "kit", "go-x", "nope"))
	want(p, "CFG-14,CFG-14,CFG-19")
	if !strings.Contains(text(p), `1 番目の [[emit]]（namespace = "a"、out = "gen/a"）の transpilers の kit を提供するプラグインが入っていない`) {
		t.Errorf("CFG-14 のメッセージ:\n%s", text(p))
	}
	if len(p.Units[0].Transpilers) != 1 || p.Units[0].Base != "" { // 解決できたものだけが残る。基底は決まらない
		t.Errorf("Units[0] = %+v", p.Units[0])
	}
	// CFG-16: 一つの単位に go と ts
	p = load(unit("a", "gen/a", "go", "ts-on-go"))
	want(p, "CFG-16")
	if !strings.Contains(text(p), `go は "go"、ts-on-go は "ts"`) {
		t.Errorf("CFG-16 のメッセージ:\n%s", text(p))
	}
	// 単位が違えば言語が違ってよい。言語の名前の一覧は持たない（CFG-11 は欠番）
	p = load(unit("a", "gen/a", "go") + unit("a", "gen/ts", "ts", "ts-x"))
	want(p, "")
	if p.Units[1].Language != "ts" || p.Units[1].Base != "ts" {
		t.Errorf("Units[1] = %+v", p.Units[1])
	}
	// CFG-19: requires の名前が transpilers にない。足りない名前ごとに一つで、要るトランスパイラを示す
	p = load(unit("a", "gen/a", "go-y"))
	want(p, "CFG-19,CFG-19,CFG-20")
	if !strings.Contains(text(p), "go-y は go-x を要るが、transpilers にない") || !strings.Contains(text(p), "go-y は go を要るが、transpilers にない") {
		t.Errorf("CFG-19 のメッセージ:\n%s", text(p))
	}
	p = load(unit("a", "gen/a", "go", "go-y"))
	want(p, "CFG-19")
	// CFG-20: 基底がない（相乗りだけ）、基底が二つ
	p = load(unit("a", "gen/a", "go-b", "go-x"))
	want(p, "CFG-19")
	p = load(unit("a", "gen/a", "go", "go-b"))
	want(p, "CFG-20")
	if !strings.Contains(text(p), "基底（マニフェストの requires が空のトランスパイラ）が二つ以上ある: go、go-b") || p.Units[0].Base != "" {
		t.Errorf("CFG-20 のメッセージ:\n%s", text(p))
	}
	// CFG-15: 同じ名前のトランスパイラを二つのプラグインが提供している。使っていなければ出ない
	manifest(t, dir, "other", [2]string{"go-x", "requires = [\"missing\"]\n"})
	want(load(unit("a", "gen/a", "go")), "")
	p = load(unit("a", "gen/a", "go", "go-x"))
	want(p, "CFG-15") // 解決できなかったトランスパイラの requires は見ず（CFG-19 なし）、CFG-20 も出さない
	if !strings.Contains(text(p), "kit（plugins/kit/hozo-plugin.toml）、other（plugins/other/hozo-plugin.toml）") {
		t.Errorf("CFG-15 のメッセージ:\n%s", text(p))
	}
	if p.Units[0].Transpilers[1].Plugin.Name != "kit" { // 以降の処理は、探した順で最初のプラグインのもの
		t.Errorf("Plugin = %s", p.Units[0].Transpilers[1].Plugin.Name)
	}
}

// TestSeams は docs/config.md「継ぎ目」: 単位の継ぎ目は、単位のトランスパイラのマニフェストの seams を合わせたもの。
// 同じ単位の二つのトランスパイラが同じデコレータを宣言していれば CFG-23（単位とデコレータの組ごとに一つ）。別の単位なら
// 重なってよい。解決できなかったトランスパイラの seams は見ない。View の config に、単位ごとの継ぎ目が載る。
func TestSeams(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	manifest(t, dir, "kit", [2]string{"go", ""},
		[2]string{"go-rate", "requires = [\"go\"]\nseams = [\"rate.limited\", \"rate.slow\"]\n"},
		[2]string{"go-trace", "requires = [\"go\"]\nseams = [\"trace.span\", \"rate.limited\"]\n"},
		[2]string{"go-base-seam", "seams = [\"own.dec\"]\n"})
	load := func(units string) *Project {
		write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+units)
		return Load(dir, "", nil)
	}
	// 宣言されているか（rate.limited というデコレータがあるか）、付いているかは確かめない
	p := load(unit("a", "gen/a", "go", "go-rate") + unit("a", "gen/b", "go", "go-trace") + unit("a", "gen/c", "go-base-seam"))
	if len(p.Diags) != 0 {
		t.Fatalf("diags:\n%s", text(p))
	}
	if got := p.Units[0].Seams; len(got) != 2 || got["rate.limited"] != "go-rate" || got["rate.slow"] != "go-rate" {
		t.Errorf("Units[0].Seams = %v", got)
	}
	if got := p.Units[1].Seams; len(got) != 2 || got["rate.limited"] != "go-trace" { // 別の単位なら、別のトランスパイラが宣言してよい
		t.Errorf("Units[1].Seams = %v", got)
	}
	if got := p.Units[2].Seams; got["own.dec"] != "go-base-seam" { // 基底も宣言できる
		t.Errorf("Units[2].Seams = %v", got)
	}
	views := p.Views()
	if got := views[0]["a"].Config.Seams; len(got) != 2 || got["rate.limited"] != "go-rate" {
		t.Errorf("View.config.seams = %v", got)
	}
	// namespaces[N] の要素ごとに、その単位の継ぎ目を持つ
	if got := views[0]["a"].Namespaces["a"]; len(got) != 3 || got[1].Seams["rate.limited"] != "go-trace" || len(got[2].Seams) != 1 {
		t.Errorf("View.namespaces[a] = %+v", got)
	}
	// CFG-23: 同じ単位の二つのトランスパイラが rate.limited を宣言している。継ぎ目は、設定の順で最初のもの
	p = load(unit("a", "gen/a", "go", "go-trace", "go-rate"))
	if got := strings.Join(codes(p.Diags), ","); got != "CFG-23" || p.Diags[0].File != "hozo.toml" {
		t.Fatalf("codes = %v, want [CFG-23]\n%s", got, text(p))
	}
	if !strings.Contains(text(p), "デコレータ rate.limited の継ぎ目を宣言している: go-trace、go-rate") || p.Units[0].Seams["rate.limited"] != "go-trace" {
		t.Errorf("CFG-23 のメッセージ:\n%s（Seams = %v）", text(p), p.Units[0].Seams)
	}
	// 解決できなかった（CFG-15 の）トランスパイラの seams は見ない
	manifest(t, dir, "other", [2]string{"go-rate", "requires = [\"go\"]\nseams = [\"trace.span\"]\n"})
	p = load(unit("a", "gen/a", "go", "go-trace", "go-rate"))
	if got := strings.Join(codes(p.Diags), ","); got != "CFG-15" {
		t.Fatalf("codes = %v, want [CFG-15]\n%s", got, text(p))
	}
}

// TestViewConfig は View の config（docs/ir.md「その名前空間から見た IR」）: index、base、options、seams。
// オプションの値は hozo.toml のテーブルのままで、書いていないトランスパイラのキーはない。
func TestViewConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	manifest(t, dir, "kit", [2]string{"go", ""}, [2]string{"go-x", "requires = [\"go\"]\nseams = [\"a.dec\"]\n"})
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/first")+
		unit("a", "gen/a", "go-x", "go")+"\n[emit.options.go-x]\nvalidate = true\nmax = 3\n")
	p := LoadWith(Options{Root: dir})
	if len(p.Diags) != 1 || p.Diags[0].Code != "CFG-14" { // 一つ目の単位のトランスパイラ t はない
		t.Fatalf("diags:\n%s", text(p))
	}
	c := p.Views()[1]["a"].Config
	if c.Index != 2 || c.Base != "go" || c.Language != "go" || c.Out != "gen/a" || c.Dir != "gen/a" ||
		strings.Join(c.Transpilers, ",") != "go-x,go" || c.Seams["a.dec"] != "go-x" || len(c.Options) != 1 ||
		c.Options["go-x"]["validate"] != true || c.Options["go-x"]["max"] != int64(3) {
		t.Errorf("config = %+v", c)
	}
}

// TestHomes は CFG-22（docs/config.md「型の置き場」）: 出力が使う宣言の名前空間が、その単位の基底の出力を持たない。
// 語彙の名前空間の型も同じ規則で、使うなら、その名前空間の単位を置く。表の各行は internal/check の TestCfg22。
func TestHomes(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "type.hozo"), "namespace type {\n  model User { id: string; }\n}\n")
	write(t, filepath.Join(dir, "server.hozo"), "namespace server {\n  @http.status(404)\n  exception Gone;\n  export fn get(): type.User throws Gone, http.BadRequest;\n}\n")
	manifest(t, dir, "kit", [2]string{"go", ""}, [2]string{"go-b", ""})
	load := func(units string) *Project {
		write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+units)
		return Load(dir, "", nil)
	}
	p := load(unit("server", "gen/server", "go"))
	if got := strings.Join(codes(p.Diags), ","); got != "CFG-22,CFG-22" {
		t.Fatalf("codes = %v, want [CFG-22 CFG-22]\n%s", got, text(p))
	}
	for _, want := range []string{
		`1 番目の [[emit]]（namespace = "server"）は type.User を使うが、type を出力する go の単位がない。[[emit]] namespace = "type"、transpilers = ["go"] の単位を足す`,
		`http.BadRequest を使うが、http を出力する go の単位がない`,
	} {
		if !strings.Contains(text(p), want) {
			t.Errorf("want %q in:\n%s", want, text(p))
		}
	}
	// 型だけの名前空間と、語彙の名前空間に単位を置けば通る（CFG-8 は欠番）
	p = load(unit("server", "gen/server", "go") + unit("type", "gen/type", "go") + unit("http", "gen/http", "go"))
	if len(p.Diags) != 0 {
		t.Fatalf("diags:\n%s", text(p))
	}
	if v := p.Views()[2]["http"]; v == nil || !v.InUnit("http.BadRequest") {
		t.Error("語彙の名前空間の単位が出力を持たない")
	}
	// 基底の違う単位は、置き場にならない
	p = load(unit("server", "gen/server", "go") + unit("type", "gen/type", "go-b") + unit("http", "gen/http", "go"))
	if got := strings.Join(codes(p.Diags), ","); got != "CFG-22" {
		t.Fatalf("codes = %v, want [CFG-22]\n%s", got, text(p))
	}
}

// TestDiscoveryEnvironmentComesFromCaller は docs/plugin-protocol.md「探索」の 2 と 3: プロジェクトの外でプラグインを探す場所
// （HOZO_PLUGIN_PATH とホーム）は、呼ぶ側が Options で渡したものだけ。プロセスの環境変数とホームディレクトリは読まない
// （読めば、テストや検査の結果が実行する人の環境に入っているプラグインに左右される）。
func TestDiscoveryEnvironmentComesFromCaller(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	fakePlugin(t, dir, "t", "go", "", `{}`, 0)
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a"))

	// プロセスの環境に、壊れたマニフェストのあるディレクトリを向ける。読めば MAN-4 が出る
	hostilePath, hostileHome := t.TempDir(), t.TempDir()
	write(t, filepath.Join(hostilePath, "broken", "hozo-plugin.toml"), "name = \n")
	write(t, filepath.Join(hostileHome, ".hozo", "plugins", "broken-home", "hozo-plugin.toml"), "name = \n")
	t.Setenv("HOZO_PLUGIN_PATH", hostilePath)
	t.Setenv("HOME", hostileHome)
	t.Setenv("USERPROFILE", hostileHome)
	for name, p := range map[string]*Project{
		"Load":     Load(dir, "", nil),
		"LoadWith": LoadWith(Options{Root: dir}),
	} {
		if len(p.Diags) != 0 || len(p.Plugins) != 1 || p.Plugins[0].Name != "t" {
			t.Errorf("%s がプロセスの環境を読んだ: plugins = %d\n%s", name, len(p.Plugins), text(p))
		}
	}

	// 渡せば探す。順は plugin_dirs、PluginPath、Home（同じ名前は近いものだけ）
	envDir, home := t.TempDir(), t.TempDir()
	// env-t と home-t は、基底 t に相乗りする（一つの単位は、基底をちょうど一つ持つ）
	manifest := func(name, transpiler string) string {
		return "name = \"" + name + "\"\ncommand = \"true\"\nprotocol = \"0.13\"\n\n[[transpiler]]\nname = \"" + transpiler + "\"\nlanguage = \"go\"\nrequires = [\"t\"]\n"
	}
	write(t, filepath.Join(envDir, "from-env", "hozo-plugin.toml"), manifest("from-env", "env-t"))
	write(t, filepath.Join(envDir, "t", "hozo-plugin.toml"), "name = \n") // plugin_dirs の t が先なので読まない
	write(t, filepath.Join(home, ".hozo", "plugins", "from-home", "hozo-plugin.toml"), manifest("from-home", "home-t"))
	write(t, filepath.Join(home, ".hozo", "plugins", "from-env", "hozo-plugin.toml"), "name = \n") // PluginPath の from-env が先
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+
		"[[emit]]\nnamespace = \"a\"\ntranspilers = [\"t\", \"env-t\", \"home-t\"]\nout = \"gen/a\"\n")
	p := LoadWith(Options{Root: dir, PluginPath: t.TempDir() + string(filepath.ListSeparator) + envDir, Home: home})
	var names []string
	for _, pl := range p.Plugins {
		names = append(names, pl.Name)
	}
	if len(p.Diags) != 0 || strings.Join(names, ",") != "t,from-env,from-home" || len(p.Units) != 1 || len(p.Units[0].Transpilers) != 3 {
		t.Errorf("plugins = %v\n%s", names, text(p))
	}
	// 渡さなければ、env-t と home-t を提供するプラグインは入っていない（CFG-14）
	if got := strings.Join(codes(Load(dir, "", nil).Diags), ","); got != "CFG-14,CFG-14" {
		t.Errorf("codes = %v, want CFG-14,CFG-14", got)
	}
	// 渡した場所の壊れたマニフェストは、使っていなくても報告する（MAN-4）
	p = LoadWith(Options{Root: dir, PluginPath: hostilePath, Home: hostileHome})
	if got := strings.Join(codes(p.Diags), ","); !strings.Contains(got, "MAN-4,MAN-4") {
		t.Errorf("codes = %v, want MAN-4 が二つ\n%s", got, text(p))
	}
}

// TestManifestDiagnosticsOfInstalledPlugins は docs/plugin-protocol.md「探索」: 読めないマニフェスト（MAN-4）と name の違う
// マニフェスト（MAN-3）は使っていなくても報告し、protocol の不一致（MAN-2）は使うプラグインにだけ報告する。
func TestManifestDiagnosticsOfInstalledPlugins(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	fakePlugin(t, dir, "t", "go", "", `{}`, 0)
	write(t, filepath.Join(dir, "plugins", "old", "hozo-plugin.toml"), "name = \"old\"\ncommand = \"old\"\nprotocol = \"0.1\"\n\n[[transpiler]]\nname = \"old\"\nlanguage = \"go\"\n")
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a"))
	// 古いプラグインは入っているだけなら何も言わない
	p := Load(dir, "", nil)
	if len(p.Diags) != 0 {
		t.Fatalf("使っていない古いプラグインに診断が出た:\n%s", text(p))
	}
	// 使えば MAN-2（CFG-14 ではない: 名前は解決できている）
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", "old"))
	p = Load(dir, "", nil)
	if got := strings.Join(codes(p.Diags), ","); got != "MAN-2" || p.Diags[0].File != "plugins/old/hozo-plugin.toml" {
		t.Fatalf("codes = %v, want [MAN-2]\n%s", got, text(p))
	}
	// 読めないマニフェストと name の違うマニフェストは、使っていなくても報告する
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a"))
	write(t, filepath.Join(dir, "plugins", "broken", "hozo-plugin.toml"), "name = \n")
	write(t, filepath.Join(dir, "plugins", "renamed", "hozo-plugin.toml"), "name = \"other\"\ncommand = \"x\"\nprotocol = \"0.13\"\n")
	p = Load(dir, "", nil)
	if got := strings.Join(codes(p.Diags), ","); got != "MAN-4,MAN-3" {
		t.Fatalf("codes = %v, want [MAN-4 MAN-3]\n%s", got, text(p))
	}
}

// TestUnitIn は docs/config.md「in」: in は読むファイルを変えず、単位の対象の宣言（Unit.Files、View.files）だけを絞る。
// in に合うファイルに宣言を持たない名前空間は、その単位では出力しない。どのファイルにも合わなければ CFG-18。
func TestUnitIn(t *testing.T) {
	dir := t.TempDir()
	fakePlugin(t, dir, "t", "go", "", `{}`, 0)
	write(t, filepath.Join(dir, "shop", "api.hozo"), "namespace shop {\n  model Item { id: string; }\n  fn open(item: Item);\n}\n")
	// batch.hozo は同じ名前空間 shop の続きと、子の名前空間 shop.jobs。検査は in によらず全体に掛かる（Item を参照できる）
	write(t, filepath.Join(dir, "shop", "batch.hozo"), "namespace shop {\n  fn nightly(item: Item);\n}\nnamespace shop.jobs {\n  fn sweep();\n}\n")
	in := func(globs string) string {
		return pluginDirs + "[[emit]]\nin = [" + globs + "]\nnamespace = \"shop\"\ntranspilers = [\"t\"]\nout = \"gen/api\"\n" + unit("shop", "gen/all")
	}
	write(t, filepath.Join(dir, "hozo.toml"), in(`"shop/api.hozo"`))
	p := Load(dir, "", nil)
	if len(p.Diags) != 0 {
		t.Fatalf("diags:\n%s", text(p))
	}
	if got := strings.Join(p.Sources, ","); got != "shop/api.hozo,shop/batch.hozo" {
		t.Fatalf("Sources = %v（in は読むファイルを変えない）", got)
	}
	// Files は、in に合ったユーザーのファイルの後に、語彙のファイル（in によらず、いつも合ったものとして扱う）
	if got := userFiles(p.Units[0].Files); got != "shop/api.hozo" {
		t.Errorf("Units[0].Files = %v", got)
	}
	if got := userFiles(p.Units[1].Files); got != "shop/api.hozo,shop/batch.hozo" {
		t.Errorf("Units[1].Files = %v（in を省けば読み込んだすべてのファイル）", got)
	}
	if all := strings.Join(p.Units[0].Files, ","); !strings.HasPrefix(all, "shop/api.hozo,hozo:vocab/") || !strings.Contains(all, "hozo:vocab/valid.hozo") {
		t.Errorf("Units[0].Files = %v（語彙のファイルがない）", all)
	}
	views := p.Views()
	// 単位 0 は shop だけ（shop.jobs は in に合うファイルに宣言がない）。単位 1 は両方
	if len(views[0]) != 1 || views[0]["shop"] == nil || len(views[1]) != 2 {
		t.Fatalf("Views = %v", views)
	}
	v := views[0]["shop"]
	if userFiles(v.Files) != "shop/api.hozo" || !v.InUnit("shop.open") || v.InUnit("shop.nightly") {
		t.Errorf("View.files = %v, InUnit(open) = %v, InUnit(nightly) = %v", v.Files, v.InUnit("shop.open"), v.InUnit("shop.nightly"))
	}
	if v.Program.Lookup("shop.nightly") == nil {
		t.Error("View.program に in の外の宣言がない（Program は全体そのまま）")
	}
	// glob（** と *）
	write(t, filepath.Join(dir, "hozo.toml"), in(`"shop/**/*.hozo"`))
	if p = Load(dir, "", nil); len(p.Diags) != 0 || userFiles(p.Units[0].Files) != "shop/api.hozo,shop/batch.hozo" {
		t.Errorf("glob: Files = %v\n%s", p.Units[0].Files, text(p))
	}
	// CFG-18: in がどのファイルにも合わない（空の in も同じ）
	for _, globs := range []string{`"api/**/*.hozo"`, `"shop/api.txt", "nope.hozo"`, ``} {
		write(t, filepath.Join(dir, "hozo.toml"), in(globs))
		p = Load(dir, "", nil)
		if got := strings.Join(codes(p.Diags), ","); got != "CFG-18" || p.Diags[0].File != "hozo.toml" {
			t.Errorf("in = [%s]: codes = %v, want [CFG-18]\n%s", globs, got, text(p))
		}
		if len(p.Views()[0]) != 0 {
			t.Errorf("in = [%s]: 何にも合わない単位が出力を持つ", globs)
		}
	}
}

// userFiles は files のうち、ユーザーのファイル（語彙でないもの）を「,」でつないだもの。
func userFiles(files []string) string {
	var out []string
	for _, f := range files {
		if !strings.HasPrefix(f, "hozo:") {
			out = append(out, f)
		}
	}
	return strings.Join(out, ",")
}

// TestUnsupported は CFG-6（docs/config.md「診断」）: 単位のトランスパイラの unsupported にあるデコレータが、その単位の
// 対象の宣言に付いている。同じ宣言を二つの単位が対象にしていても一度だけ。in の外の宣言には出ない。
func TestUnsupported(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "plugins", "t", "hozo-plugin.toml"),
		"name = \"t\"\ncommand = \"true\"\nprotocol = \"0.13\"\n\n[[transpiler]]\nname = \"t\"\nlanguage = \"go\"\nunsupported = [\"a.legacy\"]\n\n[[transpiler]]\nname = \"u\"\nlanguage = \"go\"\n")
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  decorator legacy;\n  @legacy fn f();\n}\n")
	write(t, filepath.Join(dir, "b.hozo"), "namespace a {\n  @legacy fn g();\n}\n")
	count := func(units string) int {
		t.Helper()
		write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+units)
		p := Load(dir, "", nil)
		n := 0
		for _, d := range p.Diags {
			if d.Code != "CFG-6" || d.Severity != diag.Warning {
				t.Fatalf("diags:\n%s", text(p))
			}
			n++
		}
		return n
	}
	if n := count(unit("a", "gen/a")); n != 2 {
		t.Errorf("CFG-6 = %d, want 2（f と g）", n)
	}
	// 二つの単位が同じ宣言を対象にしていても、同じ位置の診断は一度だけ
	if n := count(unit("a", "gen/a") + unit("a", "gen/a2")); n != 2 {
		t.Errorf("CFG-6 with two units = %d, want 2", n)
	}
	// in の外の宣言（b.hozo の g）には出ない
	if n := count("[[emit]]\nin = [\"a.hozo\"]\nnamespace = \"a\"\ntranspilers = [\"t\"]\nout = \"gen/a\"\n"); n != 1 {
		t.Errorf("CFG-6 with in = %d, want 1（f だけ）", n)
	}
	// unsupported を持たないトランスパイラだけの単位には出ない
	if n := count(unit("a", "gen/a", "u")); n != 0 {
		t.Errorf("CFG-6 with another transpiler = %d, want 0", n)
	}
}
