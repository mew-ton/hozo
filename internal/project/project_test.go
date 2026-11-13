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

// copyCorpus は設計書のコーパスを一時ディレクトリに写す。
func copyCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, e.Name()), string(data))
	}
	return dir
}

// TestDesignCorpus は設計書のコーパス（docs/testing.md「コーパス」）: error も warning もない
// （§7.3 の placeOrder は配線に直したので、名前で書いた relations への REL-3 はもうない。tests/corpus/design/README.md の表の 14）。
func TestDesignCorpus(t *testing.T) {
	p := Load(corpusDir, "", nil)
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
	if _, ok := p.Views()["server"]; !ok {
		t.Fatal("Views に server がない")
	}
	if d, ok := p.Dir("server"); !ok || d != "gen/server" {
		t.Fatalf("Dir(server) = %q, %v", d, ok)
	}
}

// TestConfigPath は --config に相当する configPath。相対パスは root からの相対。ルートは hozo.toml のディレクトリ。
func TestConfigPath(t *testing.T) {
	dir := copyCorpus(t)
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := Load(sub, "../hozo.toml", nil)
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

// TestNoConfig は hozo.toml がないとき: 既定の設定で解析し、CFG-*（ここでは CFG-1）を除く。
func TestNoConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  op f();\n}\n")
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
	// 設定なしではどの名前空間も出力しない（docs/config.md「置き場と探し方」）
	if n := len(p.Views()); n != 0 {
		t.Fatalf("Views = %d, want 0", n)
	}
	// 設定なしでも検査は掛かる（CFG-* 以外の診断はそのまま）。.hozo の側の CFG-2（名前空間の名前）も CFG-* なので除く
	write(t, filepath.Join(dir, "b.hozo"), "namespace out {\n  op g(): Nope;\n}\n")
	p = Load(dir, "", nil)
	if got := strings.Join(codes(p.Diags), ","); !p.NoConfig || got != "RES-1" {
		t.Fatalf("codes = %v, want [RES-1]（NoConfig=%v）\n%s", got, p.NoConfig, text(p))
	}
	if err := os.Remove(filepath.Join(dir, "b.hozo")); err != nil {
		t.Fatal(err)
	}
	// テーブルを書けば同じ入力は CFG-1（language / out がない）
	write(t, filepath.Join(dir, "hozo.toml"), "[a]\n")
	p = Load(dir, "", nil)
	if p.NoConfig || !strings.Contains(strings.Join(codes(p.Diags), ","), "CFG-1") {
		t.Fatalf("want CFG-1, got %v", codes(p.Diags))
	}
	if p.Diags[0].File != "hozo.toml" {
		t.Fatalf("File = %q, want hozo.toml", p.Diags[0].File)
	}
}

// TestUnconfiguredNamespace は docs/config.md「出力する名前空間」: hozo.toml にテーブルのない名前空間は、state や op を
// 持っていても診断なし（CFG-1 ではない）で、View を持たない。検査は同じに掛かり、設定のある名前空間の op を呼べる。
func TestUnconfiguredNamespace(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "hozo.toml"), "[app]\nlanguage = \"go\"\nout = \"gen/app\"\n")
	write(t, filepath.Join(dir, "app.hozo"), "namespace app {\n  exception Busy;\n  op save(text: string) throws Busy;\n}\n")
	write(t, filepath.Join(dir, "user.hozo"), "/** 利用者 */\nnamespace user {\n  state memo: nullable<string>;\n  op write(text: string) throws app.Busy {\n    app.save(text);\n  }\n}\nop globalOp();\n")
	p := Load(dir, "", nil)
	if p.NoConfig || len(p.Diags) != 0 {
		t.Fatalf("NoConfig=%v diags:\n%s", p.NoConfig, text(p))
	}
	views := p.Views()
	if len(views) != 1 || views["app"] == nil {
		t.Fatalf("Views = %d, want app だけ", len(views))
	}
	if _, ok := p.Dir("user"); ok {
		t.Fatal("Dir(user) がある（テーブルのない名前空間は出力先を持たない）")
	}
	if vs := views["app"].Ops["user.write"]; vs == nil || strings.Join(vs.Exceptions, ",") != "app.Busy" {
		t.Fatalf("View(app).ops[user.write] = %+v", vs)
	}
	// 設定のない名前空間にも検査は同じに掛かる: throws を外せば THROWS-1
	write(t, filepath.Join(dir, "user.hozo"), "namespace user {\n  op write(text: string) {\n    app.save(text);\n  }\n}\n")
	p = Load(dir, "", nil)
	if got := strings.Join(codes(p.Diags), ","); got != "THROWS-1" {
		t.Fatalf("codes = %v, want [THROWS-1]\n%s", got, text(p))
	}
}

// TestOverlay は overlay がディスクの内容より優先し、ディスクにないファイルも足すこと。
func TestOverlay(t *testing.T) {
	dir := copyCorpus(t)
	broken := "namespace server {\n  model Broken { x: \n}\n"
	p := Load(dir, "", map[string][]byte{
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

// TestExcludesOutAndPluginDirs は各名前空間の out と plugin_dirs の下の .hozo を読まないこと、
// sources の glob（** とメタ文字なし）に従うこと。
func TestExcludesOutAndPluginDirs(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "hozo.toml"), "[hozo]\nplugin_dirs = [\"plugins\"]\n\n[a]\nlanguage = \"go\"\nout = \"gen/a\"\n\n[a.b]\nout = \"gen/b\"\n")
	write(t, filepath.Join(dir, "spec", "a.hozo"), "namespace a {\n  op f();\n  namespace b { op g(); }\n}\n")
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
	if d, _ := p.Dir("a.b"); d != "gen/b" {
		t.Fatalf("Dir(a.b) = %q", d)
	}

	// メタ文字のない sources はそのファイルだけ
	write(t, filepath.Join(dir, "hozo.toml"), "[hozo]\nsources = [\"spec/a.hozo\"]\n\n[a]\nlanguage = \"go\"\nout = \"gen/a\"\n")
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
func fakePlugin(t *testing.T, dir, name, lang, extra, stdout string, exit int) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh がない")
	}
	script := "cat >/dev/null; printf '%s' '" + stdout + "'; exit " + string(rune('0'+exit))
	manifest := "name = \"" + name + "\"\ncommand = [\"sh\", \"-c\", \"" + strings.ReplaceAll(script, `"`, `\"`) + "\"]\nlanguage = \"" + lang + "\"\nprotocol = \"0.1\"\n" + extra
	write(t, filepath.Join(dir, "plugins", name, "hozo-plugin.toml"), manifest)
}

// TestPluginCheck はプラグインの探索、語彙の読み込み、[check] enabled の起動と診断の変換、CFG-4。
func TestPluginCheck(t *testing.T) {
	dir := copyCorpus(t)
	write(t, filepath.Join(dir, "hozo.toml"), "[hozo]\nplugin_dirs = [\"plugins\"]\n\n[server]\nlanguage = \"go\"\nout = \"gen/server\"\nplugins = [\"chk\"]\n\n[browser]\nlanguage = \"go\"\nout = \"gen/browser\"\n")
	resp := `{"diagnostics":[{"severity":"warning","code":"CHK-1","at":"server.placeOrder","message":"by fqn"},{"severity":"error","code":"CHK-2","at":"server.hozo:3:5","message":"by pos"},{"severity":"error","code":"CHK-3","at":"nowhere","message":"unknown"}]}`
	fakePlugin(t, dir, "chk", "go", "vocabulary = [\"v.hozo\"]\n\n[check]\nenabled = true\n", resp, 0)
	write(t, filepath.Join(dir, "plugins", "chk", "v.hozo"), "/** 語彙 */\nnamespace chk {\n  decorator mark for op;\n}\n")
	// hozoc 自身の warning（NAME-1）を一つ足し、プラグインの診断と一緒に出ることを確かめる
	write(t, filepath.Join(dir, "extra.hozo"), "namespace server {\n  model lowerName { id: string; }\n}\n")

	p := Load(dir, "", nil)
	if p.RunErr != nil {
		t.Fatalf("RunErr = %v", p.RunErr)
	}
	if p.Manifests["chk"] == nil {
		t.Fatalf("マニフェストが見つからない: %v", codes(p.Diags))
	}
	got := text(p)
	for _, want := range []string{
		"server.hozo:244:3: warning[CHK-1]: by fqn", // FQN → 宣言の位置
		"server.hozo:3:5: error[CHK-2]: by pos",     // file:line:col
		"hozoc: error[CHK-3]: unknown",              // 位置なし
		"extra.hozo:2:9: warning[NAME-1]",           // hozoc の診断と一緒に並ぶ
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
	found := false
	for i := 0; i < p.Files.Len(); i++ {
		if p.Files.Path(source.FileID(i)) == "hozo:plugin/chk/v.hozo" {
			found = true
		}
	}
	if !found {
		t.Error("プラグインの語彙 hozo:plugin/chk/v.hozo が読まれていない")
	}

	// CFG-4: language の不一致（マニフェストは ts、名前空間は go）
	fakePlugin(t, dir, "chk", "ts", "", `{"diagnostics":[]}`, 0)
	p = Load(dir, "", nil)
	if !strings.Contains(strings.Join(codes(p.Diags), ","), "CFG-4") {
		t.Fatalf("want CFG-4, got %v", codes(p.Diags))
	}

	// 異常終了は RunErr
	fakePlugin(t, dir, "chk", "go", "[check]\nenabled = true\n", "", 3)
	p = Load(dir, "", nil)
	if p.RunErr == nil {
		t.Fatal("RunErr = nil, want proc.Crashed")
	}

	// 見つからないプラグインは CFG-9
	write(t, filepath.Join(dir, "hozo.toml"), "[server]\nlanguage = \"go\"\nout = \"gen/server\"\nplugins = [\"missing\"]\n\n[browser]\nlanguage = \"go\"\nout = \"gen/browser\"\n")
	p = Load(dir, "", nil)
	if !strings.Contains(strings.Join(codes(p.Diags), ","), "CFG-9") {
		t.Fatalf("want CFG-9, got %v", codes(p.Diags))
	}
}
