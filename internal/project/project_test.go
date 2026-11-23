package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/fakeplugin"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/ir"
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

// loadCorpus は dir のプロジェクトを、リポジトリの plugins/ を HOZO_PLUGIN_PATH にして解析する。プラグインは、起動しない:
// ここのテストが見るのは、読み込みと設定で、リポジトリのプラグインの WASM は、このパッケージのテストでは、ビルドしない。
// プラグインの起動は、TestPluginAnswers と TestPluginAnswersPerOutput が、代役のプラグインで確かめる。
func loadCorpus(t *testing.T, dir, configPath string, overlay map[string][]byte) *Project {
	t.Helper()
	return LoadWith(Options{Root: dir, ConfigPath: configPath, Overlay: overlay, PluginPath: repoPlugins(t), NoPlugins: true})
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

// TestDesignCorpus は設計書のコーパス（docs/testing.md「コーパス」）: error はなく、warning は CFG-26 だけ（単位のトランスパイラは
// 基底 go だけで、語彙のデコレータを、どのトランスパイラも解釈しない。一覧は、ゴールデン testdata/expected/check.txt が持つ。
// tests/corpus/design/README.md の想定。§7.3 の placeOrder を配線に直した経緯はその表の 14）。
func TestDesignCorpus(t *testing.T) {
	p := loadCorpus(t, corpusDir, "", nil)
	if p.NoConfig {
		t.Fatal("NoConfig = true")
	}
	if p.Errors() != 0 {
		t.Fatalf("errors:\n%s", text(p))
	}
	for _, d := range p.Diags {
		if d.Code != "CFG-26" || d.Severity != diag.Warning {
			t.Fatalf("codes = %v, want CFG-26 の warning だけ\n%s", strings.Join(codes(p.Diags), ","), text(p))
		}
	}
	if len(p.Diags) == 0 {
		t.Fatal("CFG-26 が一つも出ていない（基底 go だけの単位で、語彙のデコレータは、解釈されない）")
	}
	if got := strings.Join(p.Sources, ","); got != "client.hozo,server.hozo" {
		t.Fatalf("Sources = %v", p.Sources)
	}
	// 単位は四つ（server、client と、使う型の置き場の http、page）。どれも基底 go で、in を省いているので
	// Files は Sources のすべてと、語彙のファイル
	if len(p.Units) != 4 || p.Units[0].Namespace != "server" || p.Units[0].Language != "go" || p.Units[0].Base != "go" ||
		len(p.Units[0].Transpilers) != 1 || p.Units[0].Transpilers[0].Name != "go" || p.Units[0].Transpilers[0].Plugin.Name != "go" ||
		!strings.HasPrefix(strings.Join(p.Units[0].Files, ","), "client.hozo,server.hozo,hozo:vocab/") {
		t.Fatalf("Units = %+v", p.Units)
	}
	views := p.Views()
	if len(views) != 4 || views[0]["server"] == nil || views[1]["client"] == nil || len(views[0]) != 1 {
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
	fakePlugin(t, dir, "t", "go", "", okResult, 0)
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
	// 単位の対象でない名前空間にも検査は同じに掛かる: 効果の後に投げる例外を prevents effects と書けば REL-4
	write(t, filepath.Join(dir, "user.hozo"), "namespace user {\n  state memo: list<string>;\n  effect fn keep(text: string): effects memo.post;\n"+
		"  fn write(text: string): throws app.Busy prevents effects {\n    keep(text);\n    app.save(text);\n  }\n}\n")
	p = Load(dir, "", nil)
	if got := strings.Join(codes(p.Diags), ","); got != "REL-4" {
		t.Fatalf("codes = %v, want [REL-4]\n%s", got, text(p))
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
	if got := strings.Join(p.Sources, ","); got != "client.hozo,extra.hozo,server.hozo" {
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
	fakePlugin(t, dir, "p", "go", "", okResult, 0, "t")
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

// okResult は、何も返さない、トランスパイラ一つの答え。
const okResult = `{"files":[],"warnings":[]}`

// fakePlugin は、にせもののプラグイン（internal/fakeplugin）を plugins/<name> に置く。result は、トランスパイラ一つの、出力一つへの
// 答えの JSON（@DIR@ と @NS@ は、その出力の Dir(U, N) と名前空間に置き換わる。にせものは、要求の出力とトランスパイラの組ごとに、
// これを並べて、応答にする: docs/plugin-protocol.md「emit」）、exit は終了コード。
// transpilers は提供するトランスパイラの名前（どれも言語は lang）。省けばプラグインと同じ名前の一つ。
// extra はマニフェストに足す行（トップレベルのキーと、テーブル）。
func fakePlugin(t *testing.T, dir, name, lang, extra, result string, exit int, transpilers ...string) {
	t.Helper()
	if len(transpilers) == 0 {
		transpilers = []string{name}
	}
	pluginDir := filepath.Join(dir, "plugins", name)
	fakeplugin.Install(t, pluginDir)
	write(t, filepath.Join(pluginDir, "response.json"), result)
	write(t, filepath.Join(pluginDir, "exit"), string(rune('0'+exit)))
	manifest := "name = \"" + name + "\"\nwasm = " + fakeplugin.WasmTOML + "\nprotocol = \"0.24\"\n" + extra
	for _, tr := range transpilers {
		manifest += "\n[[transpiler]]\nname = \"" + tr + "\"\nlanguage = \"" + lang + "\"\n"
	}
	write(t, filepath.Join(pluginDir, "hozo-plugin.toml"), manifest)
}

// launches は、プラグイン name が、これまでに起動された回数。数えたら、記録を消す。
func launches(t *testing.T, dir, name string) int {
	t.Helper()
	path := filepath.Join(dir, "plugins", name, "launches")
	data, _ := os.ReadFile(path)
	_ = os.Remove(path)
	return strings.Count(string(data), "\n")
}

// asked は、プラグイン name が受け取った頼みごと（出力とトランスパイラの組ごとに一つ）を、`<名前空間>:<単位の番号>:<トランスパイラ>`
// で、受け取った順に並べたもの。読んだら、記録を消す。
func asked(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, "plugins", name, "requests.jsonl")
	data, _ := os.ReadFile(path)
	_ = os.Remove(path)
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var r struct {
			Transpiler string `json:"transpiler"`
			View       struct {
				Viewer string `json:"viewer"`
				Config struct {
					Index int `json:"index"`
				} `json:"config"`
			} `json:"view"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out = append(out, r.View.Viewer+":"+strconv.Itoa(r.View.Config.Index)+":"+r.Transpiler)
	}
	return strings.Join(out, ",")
}

// TestPluginAnswers はプラグインの探索、使うプラグインの語彙の読み込み、プラグインへの頼みごと（direction "emit"）と、その答えと
// 診断（docs/cli.md「検査」の 4、docs/plugin-protocol.md「起動」「emit」）: errors は error の診断、warnings は PLUGIN の warning。
// 答え（files を含む）は、Project.Answers に残る。
func TestPluginAnswers(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "server.hozo"), "namespace server {\n  model M { id: string; }\n  /** 注文する */\n  fn placeOrder(m: M);\n}\n")
	write(t, filepath.Join(dir, "client.hozo"), "namespace client {\n  fn open();\n}\n")
	fakePlugin(t, dir, "pick", "go", "", `{"files":[{"path":"@DIR@/pick.txt","content":"@NS@"}],"warnings":[]}`, 0)
	config := func(serverTranspilers ...string) string {
		return pluginDirs + unit("server", "gen/server", serverTranspilers...) + unit("client", "gen/client", "pick")
	}
	write(t, filepath.Join(dir, "hozo.toml"), config("chk"))
	resp := `{"files":[],"warnings":[{"at":"server.placeOrder","message":"by fqn"}],"errors":[{"code":"CHK-2","at":"server.hozo:3:5","message":"by pos"},{"severity":"warning","code":"CHK-3","at":"nowhere","message":"unknown"}]}`
	fakePlugin(t, dir, "chk", "go", "vocabulary = [\"v.hozo\"]\n", resp, 0)
	write(t, filepath.Join(dir, "plugins", "chk", "v.hozo"), "/** 語彙 */\nnamespace chk {\n  decorator fn mark();\n}\n")
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

	p := LoadWith(Options{Root: dir})
	if p.RunErr != nil {
		t.Fatalf("RunErr = %v", p.RunErr)
	}
	// 入っているプラグインは chk と pick。どちらも使うプラグイン
	if len(p.Plugins) != 2 || p.Plugins[0].Name != "chk" || p.Plugins[1].Name != "pick" || p.Units[0].Transpilers[0].Plugin != p.Plugins[0] {
		t.Fatalf("Plugins = %+v（%v）", p.Plugins, codes(p.Diags))
	}
	got := text(p)
	for _, want := range []string{
		"server.hozo:4:3: warning[PLUGIN]: chk: by fqn", // warnings は PLUGIN の warning（トランスパイラの名前を付ける）。FQN → 宣言の位置
		"server.hozo:3:5: error[CHK-2]: by pos",         // errors は error。file:line:col
		"hozoc: error[CHK-3]: unknown",                  // 位置なし。errors の severity は、読まない（いつも error）
		"extra.hozo:2:9: warning[NAME-1]",               // hozoc の診断と一緒に並ぶ
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
	// 答えは、単位の設定の順（server の chk、client の pick）。files も、残る（書き出す実行が、使う）
	if len(p.Answers) != 2 || p.Answers[0].Transpiler.Name != "chk" || p.Answers[0].Namespace != "server" || p.Answers[1].Unit != 1 {
		t.Fatalf("Answers = %+v", p.Answers)
	}
	if r := p.Answer(1, "client", "pick"); r == nil || len(r.Files) != 1 || r.Files[0].Path != "gen/client/pick.txt" || r.Files[0].Content != "client" {
		t.Errorf("client の pick の答え = %+v", r)
	}
	if r := p.Answer(0, "server", "chk"); r == nil || len(r.Errors) != 2 || p.Answer(0, "server", "pick") != nil || p.Answer(1, "server", "chk") != nil {
		t.Errorf("server の chk の答え = %+v", r)
	}
	if a, b := launches(t, dir, "chk"), launches(t, dir, "pick"); a != 1 || b != 1 {
		t.Errorf("起動: chk %d 回、pick %d 回（want 1、1）", a, b)
	}
	if !hasFile(p, "hozo:plugin/chk/v.hozo") {
		t.Error("使うプラグインの語彙 hozo:plugin/chk/v.hozo が読まれていない")
	}
	// 語彙のファイルは、どの単位の Files にも入る（in によらず、いつも合ったものとして扱う。docs/config.md「in」）
	if files := strings.Join(p.Units[1].Files, ","); !strings.Contains(files, "hozo:plugin/chk/v.hozo") || !strings.Contains(files, "hozo:vocab/http.hozo") {
		t.Errorf("Units[1].Files = %v", files)
	}

	// 同じ名前空間の単位が二つあって、同じ診断が、出力ごとに返っても、報告は一度だけ（docs/plugin-protocol.md「emit」）。
	// 起動は、一回（TestPluginAnswersPerOutput）で、答えは、出力ごとに、残る
	write(t, filepath.Join(dir, "hozo.toml"), config("chk")+unit("server", "gen/server2", "chk"))
	p = LoadWith(Options{Root: dir})
	for _, code := range []string{"warning[PLUGIN]", "error[CHK-2]", "error[CHK-3]"} {
		if n := strings.Count(text(p), code); n != 1 {
			t.Errorf("%s が %d 回出た（want 1）:\n%s", code, n, text(p))
		}
	}
	if len(p.Answers) != 3 || p.Answer(2, "server", "chk") == nil || launches(t, dir, "chk") != 1 {
		t.Errorf("Answers = %d 個（want 3）", len(p.Answers))
	}
	launches(t, dir, "pick")

	// 入っているだけのプラグイン（どの単位もそのトランスパイラを書いていない）: 語彙を読まず、起動もしない
	write(t, filepath.Join(dir, "hozo.toml"), config("pick"))
	p = LoadWith(Options{Root: dir})
	if hasFile(p, "hozo:plugin/chk/v.hozo") || strings.Contains(text(p), "CHK-") || p.Errors() != 0 || launches(t, dir, "chk") != 0 {
		t.Errorf("使っていないプラグインの語彙か、答えが効いている:\n%s", text(p))
	}
	if len(p.Plugins) != 2 {
		t.Errorf("Plugins = %d, want 2（入っているプラグインは探す）", len(p.Plugins))
	}
	// pick は、server と client の二つの出力を受け持つが、起動は一回
	if n := launches(t, dir, "pick"); n != 1 || len(p.Answers) != 2 {
		t.Errorf("pick の起動 %d 回、Answers %d 個（want 1、2）", n, len(p.Answers))
	}

	// Load（短い形）は、プラグインを起動しない
	if p = Load(dir, "", nil); launches(t, dir, "pick") != 0 || len(p.Answers) != 0 || p.RunErr != nil {
		t.Errorf("Load が、プラグインを起動した（Answers %d 個、RunErr %v）", len(p.Answers), p.RunErr)
	}

	// 異常終了は RunErr
	write(t, filepath.Join(dir, "hozo.toml"), config("chk"))
	fakePlugin(t, dir, "chk", "go", "", "", 3)
	p = LoadWith(Options{Root: dir})
	if !errors.Is(p.RunErr, hostplugin.ErrCrashed) {
		t.Fatalf("RunErr = %v, want hozo.compiler.Crashed", p.RunErr)
	}
	// 応答の数が、要求と合わないのは、形の誤り（hozo.compiler.MalformedOutput。docs/plugin-protocol.md「応答の形」）
	fakePlugin(t, dir, "chk", "go", "", okResult, 0)
	for raw, want := range map[string]string{
		`{"outputs":[]}`:               "応答の outputs が 0 個（要求の outputs は 1 個）",
		`{"outputs":[{"results":[]}]}`: "outputs[0].results が 0 個",
		`{"outputs":[{"results":[` + okResult + `,` + okResult + `]}]}`: "outputs[0].results が 2 個",
		`{"files":[],"warnings":[]}`:                                    "outputs がない",
	} {
		write(t, filepath.Join(dir, "plugins", "chk", "raw-response"), raw)
		p = LoadWith(Options{Root: dir})
		if !errors.Is(p.RunErr, hostplugin.ErrMalformedOutput) || !strings.Contains(p.RunErr.Error(), want) {
			t.Errorf("応答 %s: RunErr = %v, want hozo.compiler.MalformedOutput（%s）", raw, p.RunErr, want)
		}
	}
	// 終了コード 2 の unsupported は MAN-5 の診断（RunErr ではない）。ほかのプラグインは、起動する
	write(t, filepath.Join(dir, "plugins", "chk", "raw-response"), `{"error":{"code":"unsupported","message":"no such transpiler"}}`)
	write(t, filepath.Join(dir, "plugins", "chk", "exit"), "2")
	launches(t, dir, "pick")
	p = LoadWith(Options{Root: dir})
	if p.RunErr != nil || !strings.Contains(text(p), "error[MAN-5]") || launches(t, dir, "pick") != 1 || p.Answer(1, "client", "pick") == nil || p.Answer(0, "server", "chk") != nil {
		t.Errorf("unsupported: RunErr = %v\n%s", p.RunErr, text(p))
	}
}

// TestPluginAnswersPerOutput は、プラグインへの頼みごとの、相手と回数（docs/plugin-protocol.md「起動」）: プラグインごとに一回で、
// 一つの要求に、そのプラグインが受け持つ出力（単位と名前空間の組）を、単位の設定の順に並べ、出力ごとに、頼むトランスパイラ
// （その単位の transpilers のうち、そのプラグインのもの）を並べる。仕様の検査に error があれば、起動しない。
// Options.NoPlugins でも、起動しない。
func TestPluginAnswersPerOutput(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\nnamespace b {\n  fn g();\n}\n")
	fakePlugin(t, dir, "chk", "go", "", `{"files":[{"path":"@DIR@/@T@.txt","content":"@NS@"}],"warnings":[]}`, 0)
	m := "name = \"chk\"\nwasm = " + fakeplugin.WasmTOML + "\nprotocol = \"0.24\"\n" +
		"\n[[transpiler]]\nname = \"chk\"\nlanguage = \"go\"\n\n[[transpiler]]\nname = \"chk2\"\nlanguage = \"go\"\nrequires = [\"chk\"]\n"
	write(t, filepath.Join(dir, "plugins", "chk", "hozo-plugin.toml"), m)

	// 単位は三つ: a を二つの単位に（一つは、同じプラグインのトランスパイラを二つ並べる）、b を一つに
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a1", "chk", "chk2")+unit("a", "gen/a2", "chk")+unit("b", "gen/b", "chk"))
	p := LoadWith(Options{Root: dir})
	if p.RunErr != nil || p.Errors() != 0 {
		t.Fatalf("RunErr = %v\n%s", p.RunErr, text(p))
	}
	if n := launches(t, dir, "chk"); n != 1 {
		t.Errorf("起動が %d 回（want 1: 三つの出力を、一つの要求に並べる）", n)
	}
	if got, want := asked(t, dir, "chk"), "a:1:chk,a:1:chk2,a:2:chk,b:3:chk"; got != want {
		t.Errorf("頼みごと = %q（期待は %q: 出力ごとに、その単位の、このプラグインのトランスパイラ）", got, want)
	}
	// 答えは、頼んだ並びで、出力とトランスパイラに結ばれる
	var answers []string
	for _, a := range p.Answers {
		answers = append(answers, a.Response.Files[0].Path)
	}
	if got, want := strings.Join(answers, ","), "gen/a1/chk.txt,gen/a1/chk2.txt,gen/a2/chk.txt,gen/b/chk.txt"; got != want {
		t.Errorf("Answers = %q, want %q", got, want)
	}
	// 要求は、仕様を一度だけ持つ: いちばん上の spec に program、出力の view には、ない
	raw, err := os.ReadFile(filepath.Join(dir, "plugins", "chk", "raw.jsonl"))
	if err != nil || strings.Count(string(raw), `"program":`) != 1 || strings.Count(string(raw), `"viewer":`) != 3 {
		t.Errorf("要求が、仕様を一度、出力を三つ、持っていない (%v)", err)
	}

	// NoPlugins: 起動しない
	p = LoadWith(Options{Root: dir, NoPlugins: true})
	if n := launches(t, dir, "chk"); n != 0 || p.Errors() != 0 || len(p.Answers) != 0 {
		t.Errorf("NoPlugins なのに、起動した: %d 回\n%s", n, text(p))
	}

	// 仕様の検査に error があれば、起動しない（warning だけなら、起動する: TestPluginAnswers）
	write(t, filepath.Join(dir, "bad.hozo"), "namespace a {\n  fn h(): Missing;\n}\n")
	p = LoadWith(Options{Root: dir})
	if n := launches(t, dir, "chk"); n != 0 || p.Errors() == 0 || p.RunErr != nil {
		t.Errorf("仕様に error があるのに、起動した: %d 回（errors = %d, RunErr = %v）", n, p.Errors(), p.RunErr)
	}
}

// TestPluginContext は、Options.Context で、プラグインの起動を、やめさせられること（言語サーバーの、裏の check。
// docs/lsp.md「プラグインの診断」）: やめるのは、プラグインの起動と起動の間。やめたら、RunErr が、その誤りで、そこまでの
// プラグインは、起動している。はじめから、やめてあれば、一つも、起動しない。Context が nil なら、やめさせない。
func TestPluginContext(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\nnamespace b {\n  fn g();\n}\n")
	// プラグインは二つ（起動は、プラグインごとに一回）: 単位 a の one、単位 b の two
	fakePlugin(t, dir, "one", "go", "", okResult, 0)
	fakePlugin(t, dir, "two", "go", "", okResult, 0)
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", "one")+unit("b", "gen/b", "two"))
	calls := func() int { return launches(t, dir, "one") + launches(t, dir, "two") }

	// はじめから、やめてある: 一つも、起動しない
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := LoadWith(Options{Root: dir, Context: ctx})
	if n := calls(); n != 0 || !errors.Is(p.RunErr, context.Canceled) {
		t.Errorf("やめてあるのに: 起動 %d 回、RunErr = %v（期待は、0 回と context.Canceled）", n, p.RunErr)
	}
	if p.Errors() != 0 {
		t.Errorf("やめたことが、診断になっている:\n%s", text(p))
	}

	// 一つ目の起動の後で、やめる: 二つ目は、起動しない
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	p = LoadWith(Options{Root: dir, Context: &cancelAfterFirst{Context: ctx, cancel: cancel, calls: filepath.Join(dir, "plugins", "one", "launches")}})
	if n := calls(); n != 1 || !errors.Is(p.RunErr, context.Canceled) {
		t.Errorf("一つ目の後で、やめたのに: 起動 %d 回、RunErr = %v（期待は、1 回と context.Canceled）", n, p.RunErr)
	}

	// Context が nil: 二つとも、起動する
	p = LoadWith(Options{Root: dir})
	if n := calls(); n != 2 || p.RunErr != nil {
		t.Errorf("Context なし: 起動 %d 回、RunErr = %v（期待は、2 回と nil）", n, p.RunErr)
	}
}

// cancelAfterFirst は、プラグインが一度起動した後（calls ができた後）に、Err を聞かれたら、やめる Context。
type cancelAfterFirst struct {
	context.Context
	cancel func()
	calls  string
}

func (c *cancelAfterFirst) Err() error {
	if _, err := os.Stat(c.calls); err == nil {
		c.cancel()
	}
	return c.Context.Err()
}

// manifest は、起動しないプラグイン name のマニフェストを plugins/<name> に置く。transpilers は [[transpiler]] の中身
// （name と language のほかの行: requires、seams など）を、トランスパイラの名前 → 行で。
func manifest(t *testing.T, dir, name string, transpilers ...[2]string) {
	t.Helper()
	m := "name = \"" + name + "\"\nwasm = \"plugin.wasm\"\nprotocol = \"0.24\"\n"
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
	// go は @http.status を解釈する（解釈するトランスパイラがいなければ CFG-26: このテストが見るものではない）
	manifest(t, dir, "kit", [2]string{"go", "interprets = [\"http.status\"]\n"}, [2]string{"go-b", "interprets = [\"http.status\"]\n"})
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
	fakePlugin(t, dir, "t", "go", "", okResult, 0)
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
		return "name = \"" + name + "\"\nwasm = \"plugin.wasm\"\nprotocol = \"0.24\"\n\n[[transpiler]]\nname = \"" + transpiler + "\"\nlanguage = \"go\"\nrequires = [\"t\"]\n"
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
	fakePlugin(t, dir, "t", "go", "", okResult, 0)
	write(t, filepath.Join(dir, "plugins", "old", "hozo-plugin.toml"), "name = \"old\"\nwasm = \"plugin.wasm\"\nprotocol = \"0.1\"\n\n[[transpiler]]\nname = \"old\"\nlanguage = \"go\"\n")
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
	write(t, filepath.Join(dir, "plugins", "renamed", "hozo-plugin.toml"), "name = \"other\"\nwasm = \"plugin.wasm\"\nprotocol = \"0.24\"\n")
	p = Load(dir, "", nil)
	if got := strings.Join(codes(p.Diags), ","); got != "MAN-4,MAN-3" {
		t.Fatalf("codes = %v, want [MAN-4 MAN-3]\n%s", got, text(p))
	}
	// なくした書き方 command のマニフェスト（MAN-6）も、使っていなくても報告する。メッセージは、新しい書き方（wasm）を示す
	write(t, filepath.Join(dir, "plugins", "legacy", "hozo-plugin.toml"), "name = \"legacy\"\ncommand = [\"go\", \"run\", \"example.com/legacy\"]\nwasm = \"plugin.wasm\"\nprotocol = \"0.24\"\n")
	p = Load(dir, "", nil)
	if got := strings.Join(codes(p.Diags), ","); got != "MAN-4,MAN-6,MAN-3" {
		t.Fatalf("codes = %v, want [MAN-4 MAN-6 MAN-3]\n%s", got, text(p))
	}
	if d := p.Diags[1]; d.File != "plugins/legacy/hozo-plugin.toml" || !strings.Contains(d.Message, "wasm = ") || p.Errors() != 3 {
		t.Errorf("MAN-6 = %+v（errors = %d）", d, p.Errors())
	}
}

// TestUnitIn は docs/config.md「in」: in は読むファイルを変えず、単位の対象の宣言（Unit.Files、View.files）だけを絞る。
// in に合うファイルに宣言を持たない名前空間は、その単位では出力しない。どのファイルにも合わなければ CFG-18。
func TestUnitIn(t *testing.T) {
	dir := t.TempDir()
	fakePlugin(t, dir, "t", "go", "", okResult, 0)
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

// TestUninterpreted は CFG-26（docs/config.md「解釈するトランスパイラ」）: 本体のない、@hint でないデコレータが、単位の対象の宣言に
// 付いているが、設定のどの単位のトランスパイラも、解釈しない。見るのは、設定の全体（別の単位のトランスパイラが解釈していれば、
// 出ない）。同じ宣言を二つの単位が対象にしていても一度だけ。in の外の宣言と、単位のない設定には、出ない。
// 場合ごとの例は tests/rules/CFG-26。
func TestUninterpreted(t *testing.T) {
	dir := t.TempDir()
	manifest(t, dir, "kit", [2]string{"plain", ""}, [2]string{"reads", "requires = [\"plain\"]\ninterprets = [\"a.mark\"]\nseams = [\"a.wrap\"]\n"})
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  decorator fn wrap();\n  decorator fn mark();\n  @wrap @mark fn f();\n}\n")
	write(t, filepath.Join(dir, "b.hozo"), "namespace a {\n  @mark fn g();\n}\n")
	write(t, filepath.Join(dir, "c.hozo"), "namespace c {\n  fn h();\n}\n")
	warnings := func(units string) []string {
		t.Helper()
		write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+units)
		p := Load(dir, "", nil)
		var out []string
		for _, d := range p.Diags {
			if d.Code != "CFG-26" || d.Severity != diag.Warning || d.Span == nil {
				t.Fatalf("diags:\n%s", text(p))
			}
			pos := p.Files.Position(d.Span.File, d.Span.Start)
			out = append(out, p.Files.Path(d.Span.File)+":"+strconv.Itoa(pos.Line)+":"+strconv.Itoa(pos.Col))
		}
		return out
	}
	cases := []struct {
		name, units string
		want        []string
	}{
		{"どのトランスパイラも解釈しない", unit("a", "gen/a", "plain"), []string{"a.hozo:4:3", "a.hozo:4:9", "b.hozo:2:3"}},
		{"同じ宣言を、二つの単位が対象にしても、一度だけ", unit("a", "gen/a", "plain") + unit("a", "gen/a2", "plain"), []string{"a.hozo:4:3", "a.hozo:4:9", "b.hozo:2:3"}},
		{"同じ単位のトランスパイラが解釈する（interprets と seams）", unit("a", "gen/a", "plain", "reads"), nil},
		{"別の単位のトランスパイラが解釈する", unit("a", "gen/a", "plain") + unit("c", "gen/c", "plain", "reads"), nil},
		{"in の外の宣言には、出ない", "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"plain\"]\nout = \"gen/a\"\nin = [\"a.hozo\"]\n", []string{"a.hozo:4:3", "a.hozo:4:9"}},
		{"単位のない設定には、出ない", "", nil},
	}
	for _, c := range cases {
		if got := warnings(c.units); strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: CFG-26 = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestUnsupported は CFG-6（docs/config.md「診断」）: 単位のトランスパイラの unsupported にあるデコレータが、その単位の
// 対象の宣言に付いている。同じ宣言を二つの単位が対象にしていても一度だけ。in の外の宣言には出ない。
func TestUnsupported(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "plugins", "t", "hozo-plugin.toml"),
		"name = \"t\"\nwasm = \"plugin.wasm\"\nprotocol = \"0.24\"\n\n[[transpiler]]\nname = \"t\"\nlanguage = \"go\"\nunsupported = [\"a.legacy\"]\n\n[[transpiler]]\nname = \"u\"\nlanguage = \"go\"\n")
	// legacy は @hint: どのトランスパイラも解釈しないが、CFG-26 は出ない（このテストが見るのは CFG-6 だけ）
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  @hint decorator fn legacy();\n  @legacy fn f();\n}\n")
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

// readerPlugin は、にせものの語彙のプラグイン（[read] を持ち、トランスパイラを持たない）を plugins/<name> に置く。namespaces は、
// 読み解く語彙の名前空間、response は、direction "read" の応答、protocol は、マニフェストの protocol。
func readerPlugin(t *testing.T, dir, name, protocol, response string, namespaces ...string) {
	t.Helper()
	pluginDir := filepath.Join(dir, "plugins", name)
	fakeplugin.Install(t, pluginDir)
	write(t, filepath.Join(pluginDir, "response.json"), response)
	write(t, filepath.Join(pluginDir, "hozo-plugin.toml"),
		"name = \""+name+"\"\nwasm = "+fakeplugin.WasmTOML+"\nprotocol = \""+protocol+"\"\n\n[read]\nnamespaces = [\""+strings.Join(namespaces, "\", \"")+"\"]\n")
}

// TestVocabularyPlugins は docs/plugin-protocol.md「語彙のプラグイン」「read」: 頼むのは、仕様が、その語彙のデコレータを使っている
// ときだけで、単位がなくても頼む。一度の実行で、一回。errors は error の診断、warnings は PLUGIN の warning。語彙のプラグインが
// error を返したら、emit は、頼まない。protocol の合わない語彙のプラグイン（MAN-2）は、頼むときにだけ、報告する。
func TestVocabularyPlugins(t *testing.T) {
	// 印は @hint: トランスパイラ pick は解釈しないが、CFG-26 は出ない（このテストが見るのは、語彙のプラグインへの頼みごと）
	const vocabulary = "namespace voc {\n  /** 印 */\n  @hint\n  decorator fn mark();\n  /** フィールドの印 */\n  @hint\n  decorator tag for field;\n}\n"
	const marked = "namespace a {\n  /** f */\n  @voc.mark\n  export fn f();\n}\n"
	const plain = "namespace a {\n  /** f */\n  export fn f();\n}\n"
	const bad = `{"errors":[{"code":"VOC-1","at":"a.f","message":"bad use"}],"warnings":[{"at":"a.f","message":"hint"}]}`
	const good = `{"errors":[],"warnings":[{"at":"a.f","message":"hint"}]}`
	setup := func(t *testing.T, spec, response, protocol string, units bool) string {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "voc.hozo"), vocabulary)
		write(t, filepath.Join(dir, "a.hozo"), spec)
		readerPlugin(t, dir, "reader", protocol, response, "voc", "other")
		readerPlugin(t, dir, "unused", ir.Version, bad, "nowhere")
		fakePlugin(t, dir, "pick", "go", "", `{"files":[{"path":"@DIR@/pick.txt","content":"@NS@"}],"warnings":[]}`, 0)
		config := pluginDirs
		if units {
			config += unit("a", "gen/a", "pick")
		}
		write(t, filepath.Join(dir, "hozo.toml"), config)
		return dir
	}

	t.Run("単位がなくても、頼む", func(t *testing.T) {
		dir := setup(t, marked, bad, ir.Version, false)
		p := LoadWith(Options{Root: dir})
		if p.RunErr != nil {
			t.Fatalf("RunErr = %v", p.RunErr)
		}
		if len(p.Readers) != 1 || p.Readers[0].Name != "reader" || !p.NeedsPlugins() {
			t.Fatalf("Readers = %+v, NeedsPlugins = %v", p.Readers, p.NeedsPlugins())
		}
		got := text(p)
		for _, want := range []string{"a.hozo:3:3: error[VOC-1]: bad use", "a.hozo:3:3: warning[PLUGIN]: reader: hint"} {
			if !strings.Contains(got, want) {
				t.Errorf("診断に %q がない:\n%s", want, got)
			}
		}
		// 使っていない語彙のプラグインには、頼まない。一度の実行で、一回
		if n, m := launches(t, dir, "reader"), launches(t, dir, "unused"); n != 1 || m != 0 {
			t.Errorf("起動 = reader %d 回、unused %d 回（期待は 1 回と 0 回）", n, m)
		}
		// 要求は、direction "read" と、spec（単位がないので、namespaces は空）だけ
		data, err := os.ReadFile(filepath.Join(dir, "plugins", "reader", "requests.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		var req map[string]json.RawMessage
		if err := json.Unmarshal(bytes.TrimSpace(data), &req); err != nil {
			t.Fatalf("要求が一つでない: %v\n%s", err, data)
		}
		var spec struct {
			Version    string                     `json:"version"`
			Namespaces map[string]json.RawMessage `json:"namespaces"`
			Program    json.RawMessage            `json:"program"`
		}
		if err := json.Unmarshal(req["spec"], &spec); err != nil {
			t.Fatal(err)
		}
		if string(req["direction"]) != `"read"` || string(req["protocol"]) != `"`+ir.Version+`"` || len(req) != 3 ||
			spec.Version != ir.Version || spec.Namespaces == nil || len(spec.Namespaces) != 0 || !bytes.Contains(spec.Program, []byte(`"a.f"`)) {
			t.Errorf("要求 = %s", data)
		}
	})

	t.Run("フィールドのデコレータでも、頼む", func(t *testing.T) {
		dir := setup(t, "namespace a {\n  model M {\n    @voc.tag id: string;\n  }\n  /** f */\n  export fn f();\n}\n", good, ir.Version, false)
		if p := LoadWith(Options{Root: dir}); len(p.Readers) != 1 || launches(t, dir, "reader") != 1 {
			t.Errorf("Readers = %+v（%v）", p.Readers, codes(p.Diags))
		}
	})

	t.Run("使っていなければ、頼まない", func(t *testing.T) {
		dir := setup(t, plain, bad, ir.Version, true)
		p := LoadWith(Options{Root: dir})
		if len(p.Readers) != 0 || p.Errors() != 0 || launches(t, dir, "reader") != 0 || launches(t, dir, "pick") != 1 {
			t.Errorf("Readers = %+v, 診断 = %v", p.Readers, codes(p.Diags))
		}
	})

	t.Run("error を返したら、emit は頼まない", func(t *testing.T) {
		dir := setup(t, marked, bad, ir.Version, true)
		p := LoadWith(Options{Root: dir})
		if !strings.Contains(text(p), "error[VOC-1]: bad use") || len(p.Answers) != 0 {
			t.Errorf("診断 = %v, Answers = %d", codes(p.Diags), len(p.Answers))
		}
		if n, m := launches(t, dir, "reader"), launches(t, dir, "pick"); n != 1 || m != 0 {
			t.Errorf("起動 = reader %d 回、pick %d 回（期待は 1 回と 0 回）", n, m)
		}
	})

	t.Run("error がなければ、emit を頼む", func(t *testing.T) {
		dir := setup(t, marked, good, ir.Version, true)
		p := LoadWith(Options{Root: dir})
		if p.Errors() != 0 || len(p.Answers) != 1 || !strings.Contains(text(p), "warning[PLUGIN]: reader: hint") {
			t.Errorf("診断 = %v, Answers = %d\n%s", codes(p.Diags), len(p.Answers), text(p))
		}
		if n, m := launches(t, dir, "reader"), launches(t, dir, "pick"); n != 1 || m != 1 {
			t.Errorf("起動 = reader %d 回、pick %d 回（期待は 1 回ずつ）", n, m)
		}
	})

	t.Run("仕様に error があれば、頼まない", func(t *testing.T) {
		dir := setup(t, "namespace a {\n  /** f */\n  @voc.mark\n  export fn f(): Missing;\n}\n", bad, ir.Version, false)
		p := LoadWith(Options{Root: dir})
		if p.Errors() == 0 || strings.Contains(text(p), "VOC-1") || launches(t, dir, "reader") != 0 {
			t.Errorf("診断 = %v", codes(p.Diags))
		}
	})

	t.Run("NoPlugins なら、頼まない", func(t *testing.T) {
		dir := setup(t, marked, bad, ir.Version, false)
		p := LoadWith(Options{Root: dir, NoPlugins: true})
		// 頼む相手は、分かっている（言語サーバーが、裏の check を走らせるかを決める）
		if len(p.Readers) != 1 || !p.NeedsPlugins() || p.Errors() != 0 || launches(t, dir, "reader") != 0 {
			t.Errorf("Readers = %+v, 診断 = %v", p.Readers, codes(p.Diags))
		}
	})

	t.Run("protocol の合わないものは、頼むときにだけ MAN-2", func(t *testing.T) {
		dir := setup(t, marked, good, "0.1", true)
		p := LoadWith(Options{Root: dir})
		if got := codes(p.Diags); len(got) != 1 || got[0] != "MAN-2" {
			t.Errorf("使っているとき: 診断 = %v（期待は MAN-2 だけ）\n%s", got, text(p))
		}
		if n, m := launches(t, dir, "reader"), launches(t, dir, "pick"); n != 0 || m != 0 || p.NeedsPlugins() == false {
			t.Errorf("起動 = reader %d 回、pick %d 回（期待は、どちらも 0 回）。NeedsPlugins = %v", n, m, p.NeedsPlugins())
		}
		dir = setup(t, plain, good, "0.1", true)
		if p := LoadWith(Options{Root: dir}); p.Errors() != 0 || len(p.Answers) != 1 {
			t.Errorf("使っていないとき: 診断 = %v（期待は、なし）", codes(p.Diags))
		}
	})

	t.Run("形の違う応答は MalformedOutput", func(t *testing.T) {
		dir := setup(t, marked, `{"errors":[]}`, ir.Version, true)
		p := LoadWith(Options{Root: dir})
		if !errors.Is(p.RunErr, hostplugin.ErrMalformedOutput) || launches(t, dir, "pick") != 0 {
			t.Errorf("RunErr = %v", p.RunErr)
		}
	})

	t.Run("read を実装していなければ MAN-5", func(t *testing.T) {
		dir := setup(t, marked, `{"error":{"code":"unsupported","message":"direction \"read\" is not supported"}}`, ir.Version, true)
		write(t, filepath.Join(dir, "plugins", "reader", "exit"), "2")
		p := LoadWith(Options{Root: dir})
		if got := codes(p.Diags); p.RunErr != nil || len(got) != 1 || got[0] != "MAN-5" || launches(t, dir, "pick") != 0 {
			t.Errorf("診断 = %v, RunErr = %v（期待は MAN-5 だけ。emit は、頼まない）\n%s", got, p.RunErr, text(p))
		}
	})

	t.Run("設定なしなら、頼まない", func(t *testing.T) {
		dir := setup(t, marked, bad, ir.Version, false)
		if err := os.Remove(filepath.Join(dir, "hozo.toml")); err != nil {
			t.Fatal(err)
		}
		p := LoadWith(Options{Root: dir, PluginPath: filepath.Join(dir, "plugins")})
		if !p.NoConfig || len(p.Readers) != 0 || p.NeedsPlugins() || launches(t, dir, "reader") != 0 {
			t.Errorf("NoConfig = %v, Readers = %+v", p.NoConfig, p.Readers)
		}
	})
}

// TestRequiresVocabulary は CFG-25（docs/plugin-protocol.md「語彙のプラグイン」、docs/config.md「診断」）: 単位のトランスパイラが、
// マニフェストの requires_vocabulary に書いた名前空間を読み解く、語彙のプラグインが、入っていなければ、error。仕様が、その語彙を
// 使っているかには、よらない。
func TestRequiresVocabulary(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  /** f */\n  export fn f();\n}\n")
	fakePlugin(t, dir, "base", "go", "", `{"files":[],"warnings":[]}`, 0)
	write(t, filepath.Join(dir, "plugins", "needs", "hozo-plugin.toml"), "name = \"needs\"\nwasm = \"plugin.wasm\"\nprotocol = \""+ir.Version+"\"\n\n"+
		"[[transpiler]]\nname = \"x\"\nlanguage = \"go\"\nrequires = [\"base\"]\nrequires_vocabulary = [\"voc\", \"db\"]\n\n"+
		"[[transpiler]]\nname = \"y\"\nlanguage = \"go\"\nrequires = [\"base\"]\nrequires_vocabulary = [\"voc\"]\n")
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", "base", "x", "y"))
	p := LoadWith(Options{Root: dir, NoPlugins: true})
	got := text(p)
	for _, want := range []string{
		"hozo.toml: error[CFG-25]: 1 番目の [[emit]]（namespace = \"a\"、out = \"gen/a\"）の x、y は、語彙 voc の語彙のプラグイン",
		"hozo.toml: error[CFG-25]: 1 番目の [[emit]]（namespace = \"a\"、out = \"gen/a\"）の x は、語彙 db の語彙のプラグイン",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("診断に %q がない:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "CFG-25"); n != 2 {
		t.Errorf("CFG-25 が %d 件（期待は、名前空間ごとに一つ: 2 件）\n%s", n, got)
	}
	// 語彙のプラグインを入れれば、出ない（protocol が合うかは、見ない: 入っているかだけ）
	readerPlugin(t, dir, "voc-reader", "0.1", `{"errors":[],"warnings":[]}`, "voc")
	readerPlugin(t, dir, "db-reader", ir.Version, `{"errors":[],"warnings":[]}`, "db")
	if p := LoadWith(Options{Root: dir, NoPlugins: true}); p.Errors() != 0 {
		t.Errorf("語彙のプラグインを入れた後の診断 = %v\n%s", codes(p.Diags), text(p))
	}
}
