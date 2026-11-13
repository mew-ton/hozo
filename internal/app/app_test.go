package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/gen/hozoc/cli"
)

var update = flag.Bool("update", false, "ゴールデンを更新する")

// 終了コード（docs/diagnostics.md「終了コード」、cli.hozo の @cli.exitCode）。
const (
	exitCheckFailed     = 1  // CheckFailed
	exitConfigInvalid   = 2  // ConfigInvalid / LanguageMismatch
	exitOutputConflict  = 3  // OutputConflict
	exitUsage           = 64 // UnknownNamespace、使い方の誤り
	exitSoftwareOrCrash = 70 // proc.Crashed / proc.MalformedOutput、宣言にない例外
)

const corpusDir = "../../tests/corpus/design"

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// warnFile は copyCorpus が足すファイル。設計書のコーパスには warning がない（error も warning もない。
// tests/corpus/design/README.md の想定）ので、warning の表示と受け渡し（check の text / json、
// 例外の値、EmitResult.warnings）を確かめるために、意図した warning を一つ持たせる: 効果を持たない op の
// prevents effects（REL-2。docs/check.md「throws」）。
const warnFile = `// internal/app のテストが足すファイル: 意図した warning（REL-2）を一つ出す
namespace server {
  /** 手が空いている */
  exception Idle;
  op ping() throws Idle prevents effects;
}
`

// 設計書のコーパスのうち、テストのプラグイン（pick-server / pick-browser）が起点に選ぶ op。既定の出力は
// プラグインが選んだ宣言だけを出す（docs/emit-go.md「出力の対象」）ので、選ぶプラグインがなければ何も書かれない。
const (
	serverSelect  = `"select":{"roots":["server.placeOrder"]}`
	browserSelect = `"select":{"roots":["browser.openOrders"]}`
)

// pluginDirs は [hozo] plugin_dirs = ["plugins"]。
const pluginDirs = "[hozo]\nplugin_dirs = [\"plugins\"]\n\n"

// unit は単位一つ分の hozo.toml。transpilers は transpilers の中身（`"pick-server", "fake"` など）。
func unit(namespace, out, transpilers string) string {
	return "[[emit]]\nnamespace = \"" + namespace + "\"\ntranspilers = [" + transpilers + "]\nout = \"" + out + "\"\n\n"
}

// corpusConfig は copyCorpus が置く hozo.toml: server の単位（out は gen/server）と browser の単位（gen/browser、pick-browser）。
// serverTranspilers は server の単位の transpilers の中身（`"pick-server"` など）。
func corpusConfig(serverTranspilers string) string {
	return pluginDirs + unit("server", "gen/server", serverTranspilers) + unit("browser", "gen/browser", `"pick-browser"`)
}

// copyCorpus は設計書のコーパスの .hozo を一時ディレクトリに写し、warning を一つ出す warn.hozo（warnFile）を足し、
// 既定の出力のために go.mod を置く（GOEMIT-1）。hozo.toml は corpusConfig で、単位ごとに、起点を選ぶだけの
// トランスパイラ（pick-server / pick-browser。sh で動く fakePlugin）を書く。何も選ばないトランスパイラ none も入れておく。
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
	write(t, filepath.Join(dir, "warn.hozo"), warnFile)
	write(t, filepath.Join(dir, "go.mod"), "module example.com/design\n\ngo 1.24\n")
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"pick-server"`))
	fakePlugin(t, dir, "pick-server", "go", "", `{"files":[],"warnings":[],`+serverSelect+`}`, 0)
	fakePlugin(t, dir, "pick-browser", "go", "", `{"files":[],"warnings":[],`+browserSelect+`}`, 0)
	fakePlugin(t, dir, "none", "go", "", `{"files":[],"warnings":[]}`, 0)
	return dir
}

// run は dir を作業ディレクトリにして、cmd/hozoc/main.go と同じ組み立て（App の Ops と Presenter を
// 生成した cli.Main に渡す）で hozoc を走らせる。
//
// プロセスの環境は読まない: 環境変数は env にあるものだけ（HOZO_PLUGIN_PATH も）、ホームは渡さない（App.Home が空）。
// 実行する人の環境に入っているプラグインは見えない（TestPluginSearchEnvironment）。
func run(t *testing.T, dir string, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	return runHome(t, dir, "", env, args...)
}

// runHome は run と同じで、ホームディレクトリ（App.Home。cmd/hozoc/main.go では os.UserHomeDir()）を home にする。
func runHome(t *testing.T, dir, home string, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	a := &App{Stderr: &errOut, Stdin: strings.NewReader(""), Home: home} // lsp は入力の終わりで戻る
	code = cli.Main(a.Ops(), Presenter{}, args, func(k string) string { return env[k] }, &out, &errOut)
	return code, out.String(), errOut.String()
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		write(t, path, got)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ゴールデン %s がない（go test -update で作る）: %v", path, err)
	}
	if string(want) != got {
		t.Errorf("%s と違う:\n--- want\n%s\n--- got\n%s", path, want, got)
	}
}

func TestCheckDesignText(t *testing.T) {
	dir := copyCorpus(t)
	code, stdout, stderr := run(t, dir, nil, "check")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	golden(t, "check_design.txt", stdout)
}

func TestCheckDesignJSON(t *testing.T) {
	dir := copyCorpus(t)
	code, stdout, stderr := run(t, dir, nil, "check", "--format=json")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	golden(t, "check_design.json", stdout)
}

func TestCheckFailed(t *testing.T) {
	dir := copyCorpus(t)
	write(t, filepath.Join(dir, "bad.hozo"), "namespace server {\n  model Bad { x: Nope; }\n}\n")
	code, stdout, stderr := run(t, dir, nil, "check")
	if code != exitCheckFailed || stdout != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "bad.hozo:2:") || !strings.Contains(stderr, "error[RES-1]") || !strings.Contains(stderr, "1 error, 1 warning") {
		t.Fatalf("stderr=%q", stderr)
	}
	code, stdout, stderr = run(t, dir, nil, "check", "--format", "json")
	if code != exitCheckFailed {
		t.Fatalf("code=%d", code)
	}
	var f struct {
		Error string `json:"error"`
		Value struct {
			Diagnostics []struct {
				Code string              `json:"code"`
				Data map[string][]string `json:"data"`
			} `json:"diagnostics"`
			Errors   int `json:"errors"`
			Warnings int `json:"warnings"`
		} `json:"value"`
	}
	if err := json.Unmarshal([]byte(stderr), &f); err != nil {
		t.Fatalf("json: %v\n%s", err, stderr)
	}
	if f.Error != "hozoc.CheckFailed" || f.Value.Errors != 1 || f.Value.Warnings != 1 {
		t.Fatalf("%+v", f)
	}
	// Diagnostic.Data（docs/check.md「Data」の RES-1 の candidates）は境界（cli.hozo の Diagnostic.data）を越えて json に出る
	found := false
	for _, d := range f.Value.Diagnostics {
		if d.Code == "RES-1" {
			found = true
			if _, ok := d.Data["candidates"]; !ok {
				t.Errorf("RES-1 の data に candidates がない: %s", stderr)
			}
		}
	}
	if !found {
		t.Errorf("RES-1 がない: %s", stderr)
	}
}

// TestCheckWithoutConfig は docs/config.md「置き場と探し方」の「設定なし」: hozo.toml が見つからず、--config も
// HOZO_CONFIG もなければ、check は作業ディレクトリをルートに既定の sources で検査し、CFG-* を出さない。
// 終了コードは診断だけで決まる。emit は書き出すものがないので CFG-7（ConfigInvalid、2）。
// 指定したファイルが読めないのは設定なしではなく CFG-7。
func TestCheckWithoutConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a { op f(); }\n")
	code, stdout, stderr := run(t, dir, nil, "check")
	if code != 0 || stderr != "" || stdout != "0 errors, 0 warnings\n" {
		t.Fatalf("check: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// warning は出る（終了コードは 0 のまま）
	write(t, filepath.Join(dir, "w.hozo"), "namespace a { model lower { id: string; } }\n")
	code, stdout, stderr = run(t, dir, nil, "check")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "warning[NAME-1]") || !strings.Contains(stdout, "0 errors, 1 warning") {
		t.Fatalf("check: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// emit は CFG-7（check の warning も一緒に運ぶ）。何も書かない
	code, stdout, stderr = run(t, dir, nil, "emit")
	if code != exitConfigInvalid || stdout != "" || !strings.Contains(stderr, "hozo.toml: error[CFG-7]") || !strings.Contains(stderr, "1 error, 1 warning") {
		t.Fatalf("emit: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, _, stderr = run(t, dir, nil, "emit", "--dry-run", "--format=json")
	if code != exitConfigInvalid || !strings.Contains(stderr, `"error": "hozoc.ConfigInvalid"`) || !strings.Contains(stderr, `"code": "CFG-7"`) {
		t.Fatalf("emit json: code=%d stderr=%q", code, stderr)
	}
	if files := listFiles(t, dir); len(files) != 2 {
		t.Fatalf("emit がファイルを書いた: %v", files)
	}
	// error があれば CheckFailed（1）。設定の誤りではない
	write(t, filepath.Join(dir, "bad.hozo"), "namespace a { op g(): Nope; }\n")
	code, _, stderr = run(t, dir, nil, "check")
	if code != exitCheckFailed || !strings.Contains(stderr, "error[RES-1]") || strings.Contains(stderr, "CFG-") {
		t.Fatalf("check: code=%d stderr=%q", code, stderr)
	}
	// emit でも、検査の error が先（check の例外。CFG-7 にはならない）
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitCheckFailed || strings.Contains(stderr, "CFG-7") {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	if err := os.Remove(filepath.Join(dir, "bad.hozo")); err != nil {
		t.Fatal(err)
	}
	// 指定したファイルが読めなければ、設定なしではなく CFG-7（2）
	for _, args := range [][]string{{"check", "--config", "nope.toml"}, {"emit", "--config", "nope.toml"}} {
		if code, _, stderr = run(t, dir, nil, args...); code != exitConfigInvalid || !strings.Contains(stderr, "nope.toml: error[CFG-7]") {
			t.Fatalf("%v: code=%d stderr=%q", args, code, stderr)
		}
	}
	if code, _, stderr = run(t, dir, map[string]string{"HOZO_CONFIG": "nope.toml"}, "check"); code != exitConfigInvalid || !strings.Contains(stderr, "error[CFG-7]") {
		t.Fatalf("HOZO_CONFIG: code=%d stderr=%q", code, stderr)
	}
	// 設定なしの App.Check は例外なしで結果（warning）を返す
	l := &App{Dir: dir}
	res, err := l.Check(hozoc.CheckInput{})
	if err != nil || len(res.Warnings) != 1 || res.Warnings[0].Code != "NAME-1" {
		t.Fatalf("Check = %+v, %v", res, err)
	}
	// その後の WriteOutputs は ConfigInvalid（CFG-7）。述語 UpToDate も、生成し直せないので同じ例外
	_, err = l.WriteOutputs(hozoc.EmitInput{}, res)
	var ci hozoc.ConfigInvalid
	if !errors.As(err, &ci) || len(ci.Value.Items) != 2 || ci.Value.Items[0].Code != "CFG-7" {
		t.Fatalf("WriteOutputs err = %v", err)
	}
	if _, err = l.UpToDate(); !errors.As(err, &ci) || ci.Value.Items[0].Code != "CFG-7" {
		t.Fatalf("UpToDate err = %v", err)
	}
}

func TestCheckInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a { op f(); }\n")
	// 設定の誤り（CFG-12: 単位に out がない）は 2
	write(t, filepath.Join(dir, "hozo.toml"), "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"t\"]\n")
	code, _, stderr := run(t, dir, nil, "check")
	if code != exitConfigInvalid || !strings.Contains(stderr, "hozo.toml: error[CFG-12]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 名前空間ごとのテーブル（前の形）は知らないキー（CFG-7）
	write(t, filepath.Join(dir, "old.toml"), "[a]\nlanguage = \"go\"\nout = \"gen\"\n")
	if code, _, stderr := run(t, dir, nil, "check", "--config", "old.toml"); code != exitConfigInvalid || !strings.Contains(stderr, "error[CFG-7]") {
		t.Fatalf("old form: code=%d stderr=%q", code, stderr)
	}
	// HOZO_CONFIG と --config
	write(t, filepath.Join(dir, "alt.toml"), "# 単位なし（何も出力しない）\n")
	write(t, filepath.Join(dir, "go.mod"), "module example.com/a\n")
	if code, _, stderr := run(t, dir, map[string]string{"HOZO_CONFIG": "alt.toml"}, "check"); code != 0 {
		t.Fatalf("HOZO_CONFIG: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := run(t, dir, map[string]string{"HOZO_CONFIG": "alt.toml"}, "check", "--config", "hozo.toml"); code != exitConfigInvalid {
		t.Fatalf("--config が HOZO_CONFIG より優先されていない: code=%d stderr=%q", code, stderr)
	}
}

// TestCheckLanguageMismatch は CFG-16（docs/config.md「単位」「診断」）: 一つの単位のトランスパイラの language がそろって
// いなければ LanguageMismatch（終了コード 2）。ほかの設定の誤りと一緒なら ConfigInvalid。
func TestCheckLanguageMismatch(t *testing.T) {
	dir := copyCorpus(t)
	fakePlugin(t, dir, "tsp", "ts", "", `{"files":[],"warnings":[]}`, 0)
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"pick-server", "tsp"`))
	code, _, stderr := run(t, dir, nil, "check")
	if code != exitConfigInvalid || !strings.Contains(stderr, "hozo.toml: error[CFG-16]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr = run(t, dir, nil, "check", "--format=json"); code != exitConfigInvalid || !strings.Contains(stderr, `"error": "hozoc.LanguageMismatch"`) {
		t.Fatalf("json: code=%d stderr=%q", code, stderr)
	}
	l := &App{Dir: dir}
	_, err := l.Check(hozoc.CheckInput{})
	var lm hozoc.LanguageMismatch
	if !errors.As(err, &lm) || len(lm.Value.Items) == 0 {
		t.Fatalf("err = %v", err)
	}
	// 知らないトランスパイラ（CFG-14）も一緒なら、設定の誤り（ConfigInvalid）
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"pick-server", "tsp", "nope"`))
	_, err = l.Check(hozoc.CheckInput{})
	var ci hozoc.ConfigInvalid
	if !errors.As(err, &ci) {
		t.Fatalf("err = %v, want ConfigInvalid", err)
	}
	// 単位の言語が ts だけなら CFG-11（TS の出力はまだない）。これも ConfigInvalid
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"tsp"`))
	if code, _, stderr = run(t, dir, nil, "check"); code != exitConfigInvalid || !strings.Contains(stderr, "error[CFG-11]") {
		t.Fatalf("ts: code=%d stderr=%q", code, stderr)
	}
}

// listFiles は dir の下のファイルの相対パス（"/" 区切り、辞書順）。
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func mtimes(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, f := range listFiles(t, dir) {
		info, err := os.Stat(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		out[f] = info.ModTime().UnixNano()
	}
	return out
}

func TestEmitDesign(t *testing.T) {
	dir := copyCorpus(t)
	gen := filepath.Join(dir, "gen")

	// dry-run は書かない
	code, stdout, stderr := run(t, dir, nil, "emit", "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run: code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(gen); !os.IsNotExist(err) {
		t.Fatalf("dry-run がファイルを書いた: %v", listFiles(t, gen))
	}
	if !strings.Contains(stdout, "written: gen/server/types.go\n") || !strings.Contains(stdout, "written: gen/browser/ops.go\n") {
		t.Fatalf("dry-run stdout=%q", stdout)
	}

	// 書く
	code, stdout1, stderr := run(t, dir, nil, "emit")
	if code != 0 {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	files := listFiles(t, gen)
	if len(files) == 0 || !containsStr(files, "server/types.go") || !containsStr(files, "server/.hozoc-outputs.json") || !containsStr(files, "browser/.hozoc-outputs.json") {
		t.Fatalf("files = %v", files)
	}
	var recorded []string
	data, err := os.ReadFile(filepath.Join(gen, "server", ".hozoc-outputs.json"))
	if err != nil || json.Unmarshal(data, &recorded) != nil || !containsStr(recorded, "gen/server/types.go") {
		t.Fatalf(".hozoc-outputs.json = %s (%v)", data, err)
	}
	for _, l := range strings.Split(strings.TrimSpace(stdout1), "\n") {
		if !strings.HasPrefix(l, "written: ") && !strings.Contains(l, "warning") && !strings.Contains(l, "errors,") {
			t.Fatalf("stdout line %q", l)
		}
	}

	// 二度目は何も書き直さない（mtime が変わらない）が、written には入る
	before := mtimes(t, gen)
	code, stdout2, _ := run(t, dir, nil, "emit")
	if code != 0 || stdout2 != stdout1 {
		t.Fatalf("second: code=%d\n%s\n%s", code, stdout1, stdout2)
	}
	for f, m := range mtimes(t, gen) {
		if before[f] != m {
			t.Fatalf("%s が書き直された", f)
		}
	}

	// 名前空間を指定すれば、それだけ
	code, stdout3, stderr := run(t, dir, nil, "emit", "--dry-run", "browser")
	if code != 0 || strings.Contains(stdout3, "gen/server/") || !strings.Contains(stdout3, "gen/browser/") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout3, stderr)
	}

	// json
	code, stdoutJ, _ := run(t, dir, nil, "emit", "--format=json", "--dry-run")
	var res struct {
		Warnings []json.RawMessage `json:"warnings"`
		Written  []string          `json:"written"`
		Removed  []string          `json:"removed"`
	}
	if code != 0 || json.Unmarshal([]byte(stdoutJ), &res) != nil || len(res.Written) == 0 || res.Removed == nil || len(res.Warnings) != 1 {
		t.Fatalf("json: code=%d %s", code, stdoutJ)
	}
}

// TestEmitStaleAndConflict はトランスパイラの出力の書き出し、単位から外したあとの古いファイルの削除、CFG-5、PLG-*、proc.Crashed。
func TestEmitStaleAndConflict(t *testing.T) {
	dir := copyCorpus(t)
	// fake は server の単位の二つ目のトランスパイラ。起点は選ばず、自分のファイルだけを書く
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"pick-server", "fake"`))

	// プラグインが gen/server/extra.txt を書く（warning も一つ）
	fakePlugin(t, dir, "fake", "go", "", `{"files":[{"path":"gen/server/extra.txt","content":"hi"}],"warnings":[{"at":"server.placeOrder","message":"careful"}]}`, 0)
	code, stdout, stderr := run(t, dir, nil, "emit")
	if code != 0 || !strings.Contains(stdout, "written: gen/server/extra.txt\n") || !strings.Contains(stdout, "warning[PLUGIN]: fake: careful") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "gen", "server", "extra.txt")); err != nil || string(b) != "hi" {
		t.Fatalf("extra.txt: %q %v", b, err)
	}

	// トランスパイラを単位から外すと extra.txt は古いファイルとして消える。既定の出力は残る
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"pick-server"`))
	code, stdout, stderr = run(t, dir, nil, "emit")
	if code != 0 || !strings.Contains(stdout, "removed: gen/server/extra.txt\n") || strings.Contains(stdout, "removed: gen/server/types.go") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen", "server", "extra.txt")); !os.IsNotExist(err) {
		t.Fatal("extra.txt が残っている")
	}

	// CFG-5: 既定の出力と同じパスに書こうとする → 3、何も書かない
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"pick-server", "fake"`))
	fakePlugin(t, dir, "fake", "go", "", `{"files":[{"path":"gen/server/types.go","content":"x"}],"warnings":[]}`, 0)
	before := mtimes(t, filepath.Join(dir, "gen"))
	code, stdout, stderr = run(t, dir, nil, "emit")
	if code != exitOutputConflict || stdout != "" || !strings.Contains(stderr, "gen/server/types.go: error[CFG-5]") ||
		!strings.Contains(stderr, "トランスパイラ fake（1 番目の [[emit]] の名前空間 server）") || !strings.Contains(stderr, "既定の出力（1 番目の [[emit]] の名前空間 server）") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for f, m := range mtimes(t, filepath.Join(dir, "gen")) {
		if before[f] != m {
			t.Fatalf("%s が書き換えられた", f)
		}
	}

	// PLG-1 → proc.MalformedOutput → 70。files の path は、その起動の Dir(U, N)（ここでは gen/server）の下でなければ
	// ならない（docs/plugin-protocol.md「emit」）: ルートの外、絶対パス、ルートの中の別のディレクトリ、別の出力の
	// Dir、Dir そのもの。何も書かない。
	outside := filepath.Join(filepath.Dir(dir), "x.go")
	for _, path := range []string{"../x.go", "gen/server/../../../x.go", "/abs/x.go", "", "x.go", "gen/x.go", "gen/browser/x.go", "gen/server", "gen/server/"} {
		fakePlugin(t, dir, "fake", "go", "", `{"files":[{"path":"`+path+`","content":"x"}],"warnings":[]}`, 0)
		before := mtimes(t, dir)
		if code, stdout, stderr = run(t, dir, nil, "emit"); code != exitSoftwareOrCrash || stdout != "" || !strings.Contains(stderr, "PLG-1") {
			t.Fatalf("path %q: code=%d stdout=%q stderr=%q", path, code, stdout, stderr)
		}
		after := mtimes(t, dir)
		for f := range after {
			if _, was := before[f]; !was && !strings.HasPrefix(f, "plugins/") {
				t.Fatalf("path %q: %s を書いた", path, f)
			}
		}
		if _, err := os.Stat(outside); !os.IsNotExist(err) {
			t.Fatalf("path %q: ルートの外に書いた", path)
		}
	}
	// Dir(U, N) の下なら、深い場所でもよい
	fakePlugin(t, dir, "fake", "go", "", `{"files":[{"path":"gen/server/./deep/../deep/a.txt","content":"1"}],"warnings":[]}`, 0)
	if code, stdout, stderr = run(t, dir, nil, "emit"); code != 0 || !strings.Contains(stdout, "written: gen/server/deep/a.txt\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// PLG-2: 同じ応答の中で同じ path が二度 → proc.MalformedOutput → 70
	fakePlugin(t, dir, "fake", "go", "", `{"files":[{"path":"gen/server/a.txt","content":"1"},{"path":"gen/server/./a.txt","content":"2"}],"warnings":[]}`, 0)
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "PLG-2") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 応答の形が違う（必須の files / warnings がない、型が違う）→ 終了コード 0 でも proc.MalformedOutput → 70
	// （docs/plugin-protocol.md「応答の形」）。知らない項目があるだけなら通る
	for _, resp := range []string{`{"unknown":1}`, `{"files":[]}`, `{"files":[],"warnings":"none"}`, `{"files":[{"path":"gen/server/a.txt"}],"warnings":[]}`} {
		fakePlugin(t, dir, "fake", "go", "", resp, 0)
		if code, stdout, stderr = run(t, dir, nil, "emit"); code != exitSoftwareOrCrash || stdout != "" || !strings.Contains(stderr, "応答の形が違う") {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", resp, code, stdout, stderr)
		}
	}
	fakePlugin(t, dir, "fake", "go", "", `{"files":[],"warnings":[],"unknown":1}`, 0)
	if code, _, stderr = run(t, dir, nil, "emit"); code != 0 {
		t.Fatalf("知らない項目: code=%d stderr=%q", code, stderr)
	}
	// errors → CheckFailed（1）
	fakePlugin(t, dir, "fake", "go", "", `{"files":[],"warnings":[],"errors":[{"code":"FAKE-1","at":"server.placeOrder","message":"nope"}]}`, 0)
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitCheckFailed || !strings.Contains(stderr, "server.hozo:244:3: error[FAKE-1]: nope") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 異常終了 → 70
	fakePlugin(t, dir, "fake", "go", "", ``, 3)
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "hozoc: ") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

// TestEmitStaleRecordStaysInDir は docs/cli.md「emit」の 6: .hozoc-outputs.json のパスに従って消すのは、その記録の持ち主の
// Dir(U, N) の下のファイルだけ。記録に `..` や絶対パス、ルートの中の別のディレクトリ、別の出力の Dir のパスが
// 書かれていても（壊れた記録、手で直した記録）、そのファイルは消さず、removed にも出さない。Dir の下の古いファイルは消す。
func TestEmitStaleRecordStaysInDir(t *testing.T) {
	dir := copyCorpus(t)
	if code, _, stderr := run(t, dir, nil, "emit"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	record := filepath.Join(dir, "gen", "server", OutputsFile)
	var recorded []string
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &recorded); err != nil || len(recorded) == 0 {
		t.Fatalf("記録を読めない: %v %s", err, data)
	}

	// 消してはならないファイル: ルートの外、ルートの直下、ルートの中の別のディレクトリ、別の出力（browser の単位）の Dir の下
	outsideRoot := filepath.Join(filepath.Dir(dir), "victim-outside.txt")
	victims := map[string]string{
		outsideRoot:                                    "../victim-outside.txt",
		filepath.Join(dir, "victim.txt"):               "victim.txt",
		filepath.Join(dir, "docs", "victim.txt"):       "gen/server/../../docs/victim.txt",
		filepath.Join(dir, "gen", "victim.txt"):        "gen/victim.txt",
		filepath.Join(dir, "gen", "browser", "v.txt"):  "gen/browser/v.txt",
		filepath.Join(dir, "gen", "serverx", "v.txt"):  "gen/serverx/v.txt",             // 名前が Dir で始まるだけの別のディレクトリ
		filepath.Join(dir, "warn.hozo"):                filepath.Join(dir, "warn.hozo"), // 絶対パス
		filepath.Join(dir, "gen", "server", "keep.go"): "",                              // 記録にないファイルは、Dir の下でも消さない
	}
	for abs, rel := range victims {
		if abs != filepath.Join(dir, "warn.hozo") {
			write(t, abs, "victim")
		}
		if rel != "" {
			recorded = append(recorded, rel)
		}
	}
	// 消すファイル: 記録にあって今回は書かない、Dir の下のもの（深い場所、正規化が要る書き方も）
	stale := map[string]string{
		filepath.Join(dir, "gen", "server", "old.txt"):        "gen/server/old.txt",
		filepath.Join(dir, "gen", "server", "sub", "old.txt"): "gen/server/./sub/../sub/old.txt",
	}
	for abs, rel := range stale {
		write(t, abs, "stale")
		recorded = append(recorded, rel)
	}
	recorded = append(recorded, "..", ".", "", "gen/server") // ファイルを指さないパスも害がない
	out, err := json.Marshal(recorded)
	if err != nil {
		t.Fatal(err)
	}
	write(t, record, string(out))

	code, stdout, stderr := run(t, dir, nil, "emit")
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for abs := range victims {
		if _, err := os.Stat(abs); err != nil {
			t.Errorf("%s が消えた: %v", abs, err)
		}
	}
	for abs := range stale {
		if _, err := os.Stat(abs); !os.IsNotExist(err) {
			t.Errorf("%s が残っている", abs)
		}
	}
	var removed []string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "removed: ") {
			removed = append(removed, strings.TrimPrefix(line, "removed: "))
		}
	}
	if want := []string{"gen/server/old.txt", "gen/server/sub/old.txt"}; strings.Join(removed, ",") != strings.Join(want, ",") {
		t.Errorf("removed = %v, want %v\n%s", removed, want, stdout)
	}
	// 記録は今回書いたパスだけに書き直される
	data, err = os.ReadFile(record)
	if err != nil || strings.Contains(string(data), "victim") || strings.Contains(string(data), "..") {
		t.Errorf("記録が書き直されていない: %v\n%s", err, data)
	}
}

// TestEmitSelectInvalid は PLG-3（docs/plugin-protocol.md「選択」「診断」）: emit の応答の select が、対象の名前空間にない
// 宣言や、付いていないデコレータを指していれば proc.MalformedOutput（70）。何も書かない。単位の対象でない宣言
// （in の外）を指した場合は TestEmitIn。
// 条件の一つずつは internal/plugin の TestCheckSelect が確かめる。
func TestEmitSelectInvalid(t *testing.T) {
	dir := copyCorpus(t)
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"pick-server", "fake"`))
	cases := map[string]string{
		"view.viewer の名前空間にない FQN":  `"select":{"roots":["browser.openOrders"]}`, // fake は server の単位のトランスパイラ
		"存在しない FQN":                 `"select":{"roots":["server.nope"]}`,
		"stops が op でない":            `"select":{"stops":["server.User"]}`,
		"states が別の名前空間の状態":         `"select":{"states":["browser.token"]}`,
		"wraps がその op に付いていないデコレータ": `"select":{"wraps":[{"op":"server.placeOrder","decorator":"http.post"}]}`,
	}
	for name, sel := range cases {
		t.Run(name, func(t *testing.T) {
			fakePlugin(t, dir, "fake", "go", "", `{"files":[{"path":"gen/server/extra.txt","content":"hi"}],"warnings":[],`+sel+`}`, 0)
			code, stdout, stderr := run(t, dir, nil, "emit")
			if code != exitSoftwareOrCrash || stdout != "" || !strings.Contains(stderr, "PLG-3") || !strings.Contains(stderr, "プラグイン fake") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
				t.Fatalf("書き出している: %v", listFiles(t, filepath.Join(dir, "gen")))
			}
			code, _, stderr = run(t, dir, nil, "emit", "--format=json")
			if code != exitSoftwareOrCrash || !strings.Contains(stderr, `"error": "proc.MalformedOutput"`) {
				t.Fatalf("json: code=%d stderr=%q", code, stderr)
			}
		})
	}
	// 正しい select（wraps が、その op に付いたデコレータを指す）は通り、包む関数になる
	fakePlugin(t, dir, "fake", "go", "", `{"files":[],"warnings":[],"select":{"wraps":[{"op":"server.login","decorator":"server.validated"}]}}`, 0)
	code, _, stderr := run(t, dir, nil, "emit")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	ops, err := os.ReadFile(filepath.Join(dir, "gen", "server", "ops.go"))
	if err != nil || !strings.Contains(string(ops), "type LoginValidatedWrapper func(in LoginInput, next LoginFunc) (LoginOk, error)") {
		t.Fatalf("包む関数がない: %v\n%s", err, ops)
	}
}

// TestEmitNothingSelected は docs/emit-go.md「出力の対象」: 単位のトランスパイラが何も選ばなければ、既定の出力は何も書かない。
// 前回の出力は古いファイルとして消え、.hozoc-outputs.json も残らない（docs/cli.md「emit」の 6 と 8）。
// エラーでも warning でもない。単位そのものを消したときは、前の出力先が分からないので何も消さない。
func TestEmitNothingSelected(t *testing.T) {
	dir := copyCorpus(t)
	gen := filepath.Join(dir, "gen")
	if code, _, stderr := run(t, dir, nil, "emit"); code != 0 {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	if files := listFiles(t, gen); !containsStr(files, "server/types.go") || !containsStr(files, "server/.hozoc-outputs.json") || !containsStr(files, "browser/ops.go") {
		t.Fatalf("files = %v", files)
	}

	// server の単位のトランスパイラを、何も選ばない none に替える。server は何も出力しなくなる
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"none"`))
	code, stdout, stderr := run(t, dir, nil, "emit")
	if code != 0 || strings.Contains(stdout, "written: gen/server/") || !strings.Contains(stdout, "removed: gen/server/types.go\n") || !strings.Contains(stdout, "removed: gen/server/ops.go\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// warning は check のもの（warn.hozo の REL-2）だけ。選ばれなかった宣言に診断は出ない
	if !strings.Contains(stdout, "0 errors, 1 warning") && strings.Count(stdout, "warning[") != 1 {
		t.Fatalf("stdout=%q", stdout)
	}
	for _, f := range listFiles(t, gen) {
		if strings.HasPrefix(f, "server/") {
			t.Errorf("gen/%s が残っている", f)
		}
	}
	// browser は server の型を import できなくなるので、自分のパッケージに複製する（docs/emit-go.md「配置」）
	types, err := os.ReadFile(filepath.Join(gen, "browser", "types.go"))
	if err != nil || strings.Contains(string(types), "example.com/design/gen/server") || !strings.Contains(string(types), "type OrderSummary struct {") {
		t.Fatalf("browser の types.go が server の型を複製していない: %v\n%s", err, types)
	}
	// 二度目は何も書かず、何も消さない
	code, stdout, _ = run(t, dir, nil, "emit")
	if code != 0 || strings.Contains(stdout, "removed: ") || strings.Contains(stdout, "gen/server/") {
		t.Fatalf("second: code=%d stdout=%q", code, stdout)
	}

	// 何も選ばないトランスパイラ（select を返さない）でも、そのトランスパイラのファイルは出る
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"fake"`))
	fakePlugin(t, dir, "fake", "go", "", `{"files":[{"path":"gen/server/only.txt","content":"x"}],"warnings":[]}`, 0)
	if code, _, stderr := run(t, dir, nil, "emit"); code != 0 {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	var server []string
	for _, f := range listFiles(t, gen) {
		if strings.HasPrefix(f, "server/") {
			server = append(server, f)
		}
	}
	if strings.Join(server, ",") != "server/.hozoc-outputs.json,server/only.txt" {
		t.Fatalf("gen/server = %v", server)
	}

	// server の単位を hozo.toml から消す: server はどの単位の対象でもなくなり、前の出力先は分からないので何も消さない
	// （docs/cli.md「emit」の 6）。エラーにもならない
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("browser", "gen/browser", `"pick-browser"`))
	code, stdout, stderr = run(t, dir, nil, "emit")
	if code != 0 || strings.Contains(stdout, "removed: ") || strings.Contains(stdout, "gen/server/") {
		t.Fatalf("unit removed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(gen, "server", "only.txt")); err != nil {
		t.Errorf("単位を消したのに gen/server/only.txt が消えた: %v", err)
	}
	// どの単位の対象でもない名前空間を emit の引数に書けば UnknownNamespace
	if code, _, stderr = run(t, dir, nil, "emit", "server"); code != exitUsage || !strings.Contains(stderr, "名前空間 server も、その子孫も、hozo.toml のどの単位の対象でもない") {
		t.Fatalf("emit server: code=%d stderr=%q", code, stderr)
	}
}

// TestEmitTargetUsesOtherNamespace は docs/cli.md「emit」の 3: 対象の出力が、対象でない Go の出力の名前空間の型を使うとき、
// その単位のトランスパイラも起動して select を使う。対象を絞っても、すべてを対象にしたときと同じ内容が出る。
func TestEmitTargetUsesOtherNamespace(t *testing.T) {
	dir := copyCorpus(t)
	if code, _, stderr := run(t, dir, nil, "emit"); code != 0 {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	all := readAll(t, filepath.Join(dir, "gen", "browser"))
	if err := os.RemoveAll(filepath.Join(dir, "gen")); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, dir, nil, "emit", "browser")
	if code != 0 || strings.Contains(stdout, "gen/server/") {
		t.Fatalf("emit browser: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	only := readAll(t, filepath.Join(dir, "gen", "browser"))
	if len(only) == 0 || len(only) != len(all) {
		t.Fatalf("files = %d, want %d", len(only), len(all))
	}
	for f, content := range all {
		if only[f] != content {
			t.Errorf("gen/browser/%s が、すべてを対象にしたときと違う", f)
		}
	}
	// server の型は import している（server の単位のトランスパイラの select から、server の出力がその型を持つと分かる）
	if !strings.Contains(only["types.go"], `"example.com/design/gen/server"`) {
		t.Errorf("browser の types.go が gen/server を import していない:\n%s", only["types.go"])
	}
}

// TestUpToDate は cli.hozo の分類 OutputsSync の述語 UpToDate（docs/cli.md「cli.hozo」「手で書くもの」）: 生成し直した内容と
// ディスクの内容を比べる。生成したコードの判定関数 hozoc.ClassifyOutputsSync が、App.Ops が hozoc.States に入れた
// 述語を呼ぶ。
func TestUpToDate(t *testing.T) {
	dir := copyCorpus(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	a := &App{Stderr: &bytes.Buffer{}}
	ops := a.Ops()
	sync := func() hozoc.OutputsSync {
		t.Helper()
		got, err := hozoc.ClassifyOutputsSync()
		if err != nil {
			t.Fatalf("ClassifyOutputsSync: %v", err)
		}
		return got
	}
	// まだ書き出していない: 足りない
	if got := sync(); got != hozoc.OutputsSyncDefault {
		t.Fatalf("emit の前: %v", got)
	}
	// 配線の emit（check → writeOutputs）。writeOutputs は hozoc.States.OutputsSync の命令で、効果 syncOutputs は
	// どの場合からも UpToDate に移す
	res, err := ops.Emit(hozoc.EmitInput{})
	if err != nil || len(res.Written) == 0 {
		t.Fatalf("Emit = %+v, %v", res, err)
	}
	if got := sync(); got != hozoc.OutputsSyncUpToDate {
		t.Fatalf("emit の後: %v", got)
	}
	// 古い（内容が違う）
	types := filepath.Join(dir, "gen", "server", "types.go")
	orig, err := os.ReadFile(types)
	if err != nil {
		t.Fatal(err)
	}
	write(t, types, string(orig)+"\n// 手で足した行\n")
	if got := sync(); got != hozoc.OutputsSyncDefault {
		t.Errorf("書き換えた後: %v", got)
	}
	write(t, types, string(orig))
	if got := sync(); got != hozoc.OutputsSyncUpToDate {
		t.Errorf("戻した後: %v", got)
	}
	// 足りない
	if err := os.Remove(types); err != nil {
		t.Fatal(err)
	}
	if got := sync(); got != hozoc.OutputsSyncDefault {
		t.Errorf("消した後: %v", got)
	}
	write(t, types, string(orig))
	// 余分（前回書いて、今回は書かないファイルが残っている）: server の単位のトランスパイラを、何も選ばない none に替える
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"none"`))
	b := &App{Stderr: &bytes.Buffer{}} // 設定を読み直すために新しい App（直前の Check の結果を持たない）
	ops = b.Ops()
	if got := sync(); got != hozoc.OutputsSyncDefault {
		t.Errorf("トランスパイラを替えた後: %v", got)
	}
	if _, err := ops.Emit(hozoc.EmitInput{}); err != nil {
		t.Fatal(err)
	}
	if got := sync(); got != hozoc.OutputsSyncUpToDate {
		t.Errorf("もう一度 emit した後: %v", got)
	}
	// 仕様に誤りがあって生成し直せなければ、その例外
	write(t, filepath.Join(dir, "bad.hozo"), "namespace server {\n  model Bad { x: Nope; }\n}\n")
	c := &App{Stderr: &bytes.Buffer{}}
	c.Ops()
	var cf hozoc.CheckFailed
	if _, err := hozoc.ClassifyOutputsSync(); !errors.As(err, &cf) {
		t.Errorf("誤りのある仕様: err = %v, want CheckFailed", err)
	}
}

// readAll は dir の直下のファイルの名前 → 内容。
func readAll(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range listFiles(t, dir) {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(b)
	}
	return out
}

// TestPluginSearchEnvironment は docs/plugin-protocol.md「探索」の 2 と 3 が hozoc に届く道: HOZO_PLUGIN_PATH は cli.Main に
// 渡した env から cli.hozo の入力の pluginPath に入り、ホームは App.Home。プロセスの環境変数とホームディレクトリは読まない。
func TestPluginSearchEnvironment(t *testing.T) {
	dir := copyCorpus(t)
	// server の単位のトランスパイラ pick-server を、プロジェクトの外（HOZO_PLUGIN_PATH のディレクトリ）へ移す
	envDir, home := t.TempDir(), t.TempDir()
	if err := os.Rename(filepath.Join(dir, "plugins", "pick-server"), filepath.Join(envDir, "pick-server")); err != nil {
		t.Fatal(err)
	}
	// browser の単位の pick-browser は、ホームへ移す
	write(t, filepath.Join(home, ".hozo", "plugins", ".keep"), "")
	if err := os.Rename(filepath.Join(dir, "plugins", "pick-browser"), filepath.Join(home, ".hozo", "plugins", "pick-browser")); err != nil {
		t.Fatal(err)
	}

	// プロセスの環境に向けても、渡していなければ見つからない（CFG-14）
	t.Setenv("HOZO_PLUGIN_PATH", envDir)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	code, _, stderr := run(t, dir, nil, "check")
	if code != exitConfigInvalid || strings.Count(stderr, "error[CFG-14]") != 2 {
		t.Fatalf("渡していない: code=%d stderr=%q", code, stderr)
	}
	// 渡せば見つかる。check も emit も lsp も同じ入力を持つ
	env := map[string]string{"HOZO_PLUGIN_PATH": t.TempDir() + string(filepath.ListSeparator) + envDir}
	if code, _, stderr = runHome(t, dir, home, env, "check"); code != 0 {
		t.Fatalf("check: code=%d stderr=%q", code, stderr)
	}
	code, stdout, stderr := runHome(t, dir, home, env, "emit")
	if code != 0 || !strings.Contains(stdout, "written: gen/server/ops.go\n") || !strings.Contains(stdout, "written: gen/browser/ops.go\n") {
		t.Fatalf("emit: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, _, stderr = runHome(t, dir, home, env, "lsp"); code != 0 {
		t.Fatalf("lsp: code=%d stderr=%q", code, stderr)
	}
	// 片方だけなら、もう片方の単位のトランスパイラが見つからない
	if code, _, stderr = runHome(t, dir, "", env, "check"); code != exitConfigInvalid || !strings.Contains(stderr, "pick-browser を提供するプラグインが入っていない") {
		t.Fatalf("ホームなし: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr = runHome(t, dir, home, nil, "check"); code != exitConfigInvalid || !strings.Contains(stderr, "pick-server を提供するプラグインが入っていない") {
		t.Fatalf("HOZO_PLUGIN_PATH なし: code=%d stderr=%q", code, stderr)
	}
	// 外の場所の壊れたマニフェストは、使っていなくても報告する（MAN-4 → ConfigInvalid）
	write(t, filepath.Join(home, ".hozo", "plugins", "broken", "hozo-plugin.toml"), "name = \n")
	if code, _, stderr = runHome(t, dir, home, env, "check"); code != exitConfigInvalid || !strings.Contains(stderr, "error[MAN-4]") {
		t.Fatalf("壊れたマニフェスト: code=%d stderr=%q", code, stderr)
	}
}

func TestEmitUnknownNamespace(t *testing.T) {
	dir := copyCorpus(t)
	code, stdout, stderr := run(t, dir, nil, "emit", "nope")
	if code != exitUsage || stdout != "" || !strings.Contains(stderr, "名前空間 nope も、その子孫も、hozo.toml のどの単位の対象でもない") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
		t.Fatal("書き出している")
	}
	code, _, stderr = run(t, dir, nil, "emit", "--format=json", "nope")
	if code != exitUsage || strings.TrimSpace(stderr) != "{\n  \"error\": \"hozoc.UnknownNamespace\"\n}" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestUsageAndLsp(t *testing.T) {
	dir := copyCorpus(t)
	cases := []struct {
		args []string
		code int
	}{
		{nil, exitUsage},
		{[]string{"help"}, 0},
		{[]string{"check", "--help"}, 0},
		{[]string{"bogus"}, exitUsage},
		{[]string{"check", "--nope"}, exitUsage},
		{[]string{"check", "--format"}, exitUsage},
		{[]string{"check", "--format=yaml"}, exitUsage},
		{[]string{"check", "server"}, exitUsage},
		{[]string{"emit", "--dry-run=maybe"}, exitUsage},
		{[]string{"lsp"}, 0}, // 入力が閉じれば終わる。終了コードは常に 0（docs/lsp.md「ライフサイクル」）
		{[]string{"emit", "--dry-run", "--", "server"}, 0},
		{[]string{"emit", "--format", "text", "--format", "json", "--dry-run", "-x"}, exitUsage}, // -x は位置引数（知らない名前空間）
	}
	for _, c := range cases {
		code, _, stderr := run(t, dir, nil, c.args...)
		if code != c.code {
			t.Errorf("%v: code=%d, want %d (stderr=%q)", c.args, code, c.code, stderr)
		}
	}
	logPath := filepath.Join(dir, "lsp.log")
	code, _, stderr := run(t, dir, nil, "lsp", "--log", logPath)
	if code != 0 || stderr != "" {
		t.Errorf("lsp: code=%d stderr=%q", code, stderr) // 標準エラーには書かない（docs/lsp.md「通信」）
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("--log のファイルがない: %v", err)
	}
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// requestsFile は fakePlugin が受け取った要求を一行ずつ足していくファイル（マニフェストのディレクトリの下）。
const requestsFile = "requests.jsonl"

// fakePlugin はシェルで動く小さなプラグインを plugins/<name> に置く。提供するトランスパイラは、プラグインと同じ名前の一つ
// （言語は lang）。stdout は応答の JSON、exit は終了コード。extra はマニフェストに足す行。受け取った要求は、
// マニフェストのディレクトリの requestsFile に一行ずつ足す（requests で読む）。
func fakePlugin(t *testing.T, dir, name, lang, extra, stdout string, exit int) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh がない")
	}
	script := "cat >>\"$HOZO_PLUGIN_DIR/" + requestsFile + "\"; echo >>\"$HOZO_PLUGIN_DIR/" + requestsFile + "\"; printf '%s' '" + stdout + "'; exit " + string(rune('0'+exit))
	manifest := "name = \"" + name + "\"\ncommand = [\"sh\", \"-c\", \"" + strings.ReplaceAll(strings.ReplaceAll(script, `\`, `\\`), `"`, `\"`) + "\"]\nprotocol = \"0.2\"\n" + extra +
		"\n[[transpiler]]\nname = \"" + name + "\"\nlanguage = \"" + lang + "\"\n"
	write(t, filepath.Join(dir, "plugins", name, "hozo-plugin.toml"), manifest)
}

// request は fakePlugin が受け取った emit の要求のうち、テストが見る部分。
type request struct {
	Direction  string `json:"direction"`
	Transpiler string `json:"transpiler"`
	View       struct {
		Viewer string `json:"viewer"`
		Config struct {
			Language    string   `json:"language"`
			Out         string   `json:"out"`
			Dir         string   `json:"dir"`
			Transpilers []string `json:"transpilers"`
		} `json:"config"`
		Files []string `json:"files"`
	} `json:"view"`
}

// requests はプラグイン name が受け取った要求を、受け取った順で返し、記録を消す。
func requests(t *testing.T, dir, name string) []request {
	t.Helper()
	path := filepath.Join(dir, "plugins", name, requestsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []request
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var r request
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("%s: %v\n%s", path, err, line)
		}
		out = append(out, r)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	return out
}

// 単位のテストのプロジェクト（docs/config.md「単位」「単位の対象」「in」）。名前空間 shop は二つのファイルに分かれ、
// 子の名前空間 shop.admin を持つ。
const (
	shopAPI = `/** 店（API の側） */
namespace shop {
  model Item { id: string; }
  model Receipt { item: Item; }
  /** 品物 */
  state items: list<Item>;
  effect addItem on items.post;

  /** 入口。nightly は batch.hozo で宣言した配線 */
  op open(item: Item): Receipt {
    save(item);
    nightly(item);
    const r = issue(item);
    return r;
  }
  op save(item: Item) effects addItem;
  op issue(item: Item): Receipt;
}
`
	shopBatch = `/** 店（バッチの側） */
namespace shop {
  model Report { count: int; }
  /** 夜間の片付け */
  op nightly(item: Item) {
    sweep(item);
  }
  op sweep(item: Item);
  op report(): Report;
}

/** 管理 */
namespace shop.admin {
  op ban(item: shop.Item);
}
`
)

// unitsProject は単位のテストのプロジェクトを一時ディレクトリに作る。単位は三つ:
//
//  1. in = ["shop/api.hozo"]、namespace shop、トランスパイラ api（起点は open）、out は gen/api
//  2. namespace shop（in なし）、トランスパイラ batch（起点は nightly と report）、out は gen/batch
//  3. namespace shop.admin、トランスパイラ admin（起点は ban）、out は gen/admin
//
// shop.admin は 3 が名指ししているので、1 と 2 の対象から外れる。
func unitsProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/units\n\ngo 1.24\n")
	write(t, filepath.Join(dir, "shop", "api.hozo"), shopAPI)
	write(t, filepath.Join(dir, "shop", "batch.hozo"), shopBatch)
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+
		"[[emit]]\nin = [\"shop/api.hozo\"]\nnamespace = \"shop\"\ntranspilers = [\"api\"]\nout = \"gen/api\"\n\n"+
		unit("shop", "gen/batch", `"batch"`)+unit("shop.admin", "gen/admin", `"admin"`))
	fakePlugin(t, dir, "api", "go", "", `{"files":[{"path":"gen/api/api.txt","content":"api"}],"warnings":[],"select":{"roots":["shop.open"]}}`, 0)
	fakePlugin(t, dir, "batch", "go", "", `{"files":[],"warnings":[],"select":{"roots":["shop.nightly","shop.report"]}}`, 0)
	fakePlugin(t, dir, "admin", "go", "", `{"files":[],"warnings":[],"select":{"roots":["shop.admin.ban"]}}`, 0)
	return dir
}

// emitAll は dir で hozoc emit を走らせ、gen の下に書かれたファイル（gen からの相対パス → 内容）を返す。
func emitAll(t *testing.T, dir string, args ...string) map[string]string {
	t.Helper()
	code, stdout, stderr := run(t, dir, nil, append([]string{"emit"}, args...)...)
	if code != 0 {
		t.Fatalf("emit %v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
	}
	return readAll(t, filepath.Join(dir, "gen"))
}

func wantIn(t *testing.T, name, content string, wants, nots []string) {
	t.Helper()
	if content == "" {
		t.Errorf("%s がない", name)
		return
	}
	for _, w := range wants {
		if !strings.Contains(content, w) {
			t.Errorf("%s に %q がない:\n%s", name, w, content)
		}
	}
	for _, n := range nots {
		if strings.Contains(content, n) {
			t.Errorf("%s に %q がある:\n%s", name, n, content)
		}
	}
}

// TestEmitUnits は docs/config.md「単位」「単位の対象」: 同じ名前空間の二つの単位が、それぞれのトランスパイラの選択で、
// それぞれの out に独立に出力する。別の単位が名指しした子孫は、祖先の単位の対象から外れる。
// トランスパイラへの要求は、起動するトランスパイラの名前と、その単位での設定を持つ（docs/plugin-protocol.md「emit」）。
func TestEmitUnits(t *testing.T) {
	dir := unitsProject(t)
	got := emitAll(t, dir)

	t.Run("同じ名前空間の二つの単位は、それぞれの out に独立に出力する", func(t *testing.T) {
		// 単位 1（api）: open から辿った宣言と、トランスパイラ自身のファイル
		wantIn(t, "gen/api/ops.go", got["api/ops.go"], []string{"type OpenFunc func(", "type SaveFunc func(", "type IssueFunc func("}, []string{"ReportFunc", "SweepFunc"})
		wantIn(t, "gen/api/api.txt", got["api/api.txt"], []string{"api"}, nil)
		// 単位 2（batch）: nightly と report から辿った宣言。open は選んでいない
		wantIn(t, "gen/batch/ops.go", got["batch/ops.go"], []string{"type NightlyFunc func(", "type SweepFunc func(", "type ReportFunc func("}, []string{"OpenFunc", "IssueFunc"})
		wantIn(t, "gen/batch/types.go", got["batch/types.go"], []string{"type Report struct {", "type Item struct {"}, []string{"type Receipt struct"})
		// どちらも自分の Item を持つ（同じ名前空間を出力する別の単位のパッケージは import しない）
		wantIn(t, "gen/api/types.go", got["api/types.go"], []string{"type Item struct {", "type Receipt struct {"}, []string{"gen/batch", "type Report struct"})
		if strings.Contains(got["batch/types.go"], "gen/api") {
			t.Errorf("gen/batch が gen/api を import している:\n%s", got["batch/types.go"])
		}
		// 記録は出力（単位と名前空間の組）ごと
		for _, rec := range []string{"api/.hozoc-outputs.json", "batch/.hozoc-outputs.json", "admin/.hozoc-outputs.json"} {
			if got[rec] == "" {
				t.Errorf("gen/%s がない", rec)
			}
		}
		if !strings.Contains(got["api/.hozoc-outputs.json"], `"gen/api/api.txt"`) || strings.Contains(got["api/.hozoc-outputs.json"], "gen/batch") {
			t.Errorf("gen/api/.hozoc-outputs.json = %s", got["api/.hozoc-outputs.json"])
		}
	})

	t.Run("別の単位が名指しした子孫は、祖先の単位の対象から外れる", func(t *testing.T) {
		wantIn(t, "gen/admin/ops.go", got["admin/ops.go"], []string{"type BanFunc func("}, nil)
		for f := range got {
			if strings.HasPrefix(f, "api/admin/") || strings.HasPrefix(f, "batch/admin/") {
				t.Errorf("祖先の単位が shop.admin を出力した: gen/%s", f)
			}
		}
		// トランスパイラ api と batch は shop.admin について起動されず、admin は shop について起動されない
		for name, viewer := range map[string]string{"api": "shop", "batch": "shop", "admin": "shop.admin"} {
			for _, r := range requests(t, dir, name) {
				if r.View.Viewer != viewer {
					t.Errorf("トランスパイラ %s が %s について起動された", name, r.View.Viewer)
				}
			}
		}
	})

	t.Run("要求は transpiler と、その単位での設定を持つ", func(t *testing.T) {
		emitAll(t, dir)
		for _, c := range []struct {
			name, viewer, dir string
			files             string
		}{
			{"api", "shop", "gen/api", "shop/api.hozo"},
			{"batch", "shop", "gen/batch", "shop/api.hozo,shop/batch.hozo"},
			{"admin", "shop.admin", "gen/admin", "shop/api.hozo,shop/batch.hozo"},
		} {
			reqs := requests(t, dir, c.name)
			if len(reqs) != 1 {
				t.Errorf("%s: 要求が %d 回（want 1）", c.name, len(reqs))
				continue
			}
			r := reqs[0]
			if r.Direction != "emit" || r.Transpiler != c.name || r.View.Viewer != c.viewer || r.View.Config.Dir != c.dir || r.View.Config.Out != c.dir ||
				r.View.Config.Language != "go" || strings.Join(r.View.Config.Transpilers, ",") != c.name || strings.Join(r.View.Files, ",") != c.files {
				t.Errorf("%s: 要求 = %+v", c.name, r)
			}
		}
	})

	t.Run("別の名前空間の型は、設定の順で最初にその型を持つ単位の出力から import する", func(t *testing.T) {
		// shop.admin.ban は shop.Item を使う。shop を出力する単位は api（1 番目）と batch（2 番目）で、どちらの出力も Item を持つ
		wantIn(t, "gen/admin/ops.go", got["admin/ops.go"], []string{`"example.com/units/gen/api"`, "item shop.Item"}, []string{"gen/batch"})
		// api の出力が Item を持たなくなれば（何も選ばない）、次の batch の出力から import する
		fakePlugin(t, dir, "api", "go", "", `{"files":[],"warnings":[]}`, 0)
		got := emitAll(t, dir)
		wantIn(t, "gen/admin/ops.go", got["admin/ops.go"], []string{`"example.com/units/gen/batch"`}, []string{"gen/api"})
		// どの単位の出力も持たなければ複製する
		fakePlugin(t, dir, "batch", "go", "", `{"files":[],"warnings":[]}`, 0)
		got = emitAll(t, dir)
		wantIn(t, "gen/admin/types.go", got["admin/types.go"], []string{"type Item struct {"}, []string{"example.com/units/gen/"})
	})
}

// TestEmitUnitsTargets は docs/cli.md「emit」の 1 と 3: 引数は名前空間で、その名前空間と子孫を出力するすべての単位の分を
// 書き出す。対象を絞っても、書き出す出力の内容は、すべてを対象にしたときと同じ。
func TestEmitUnitsTargets(t *testing.T) {
	dir := unitsProject(t)
	all := emitAll(t, dir)
	clear := func() {
		if err := os.RemoveAll(filepath.Join(dir, "gen")); err != nil {
			t.Fatal(err)
		}
	}
	same := func(got map[string]string, prefixes ...string) {
		t.Helper()
		for f, content := range all {
			inTarget := false
			for _, p := range prefixes {
				inTarget = inTarget || strings.HasPrefix(f, p)
			}
			if inTarget && got[f] != content {
				t.Errorf("gen/%s が、すべてを対象にしたときと違う", f)
			}
		}
		for f := range got {
			inTarget := false
			for _, p := range prefixes {
				inTarget = inTarget || strings.HasPrefix(f, p)
			}
			if !inTarget {
				t.Errorf("対象でない gen/%s を書いた", f)
			}
		}
	}
	// shop.admin: 単位 3 の出力だけ。import する先（単位 1 の shop）は、select だけを使って決める
	clear()
	same(emitAll(t, dir, "shop.admin"), "admin/")
	// shop: shop とその子孫を出力するすべての単位の分（三つとも）
	clear()
	same(emitAll(t, dir, "shop"), "api/", "batch/", "admin/")
	// 引数は名前空間で、単位の namespace である必要はない: 単位 3 が名指しする shop.admin の子孫 shop.admin.audit は、
	// 単位 3 の対象。その出力だけを書く
	write(t, filepath.Join(dir, "shop", "audit.hozo"), "namespace shop.admin.audit {\n  op review(id: string): boolean;\n}\n")
	fakePlugin(t, dir, "admin", "go", "", `{"files":[],"warnings":[],"select":{"roots":[]}}`, 0)
	clear()
	if got := emitAll(t, dir, "shop.admin.audit"); len(got) != 0 {
		t.Errorf("何も選ばれていないのに書いた: %v", got)
	}
	// 引数は仕様にある名前空間でなければならない（単位の対象の名前空間の下の名前でも）。子孫が単位の対象なら、
	// 引数そのものがどの単位の対象でなくてもよい（下の TestEmitArgumentDescendantOnly）
	if code, stdout, stderr := run(t, dir, nil, "emit", "shop.typo"); code != exitUsage || stdout != "" || !strings.Contains(stderr, "名前空間 shop.typo も") {
		t.Errorf("仕様にない名前空間: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, _, stderr := run(t, dir, nil, "emit", "shop", "nope"); code != exitUsage || !strings.Contains(stderr, "名前空間 nope も") || strings.Contains(stderr, "shop、") {
		t.Errorf("引数の一つが対象でない: code=%d stderr=%q", code, stderr)
	}
	if err := os.Remove(filepath.Join(dir, "shop", "audit.hozo")); err != nil {
		t.Fatal(err)
	}
	fakePlugin(t, dir, "admin", "go", "", `{"files":[],"warnings":[],"select":{"roots":["shop.admin.ban"]}}`, 0)
	// dry-run は書かない
	clear()
	code, stdout, _ := run(t, dir, nil, "emit", "--dry-run", "shop.admin")
	if code != 0 || !strings.Contains(stdout, "written: gen/admin/ops.go\n") || strings.Contains(stdout, "gen/api/") {
		t.Errorf("dry-run: code=%d stdout=%q", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
		t.Error("dry-run が書いた")
	}
}

// TestEmitArgumentDescendantOnly は docs/cli.md「emit」の 1（docs/decisions.md 50）: 引数の名前空間そのものがどの単位の
// 対象でなくても、その子孫を対象にする単位があれば、その出力が対象になる（UnknownNamespace ではない）。
// 引数もその子孫もどの単位の対象でもなければ UnknownNamespace。
func TestEmitArgumentDescendantOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/desc\n\ngo 1.24\n")
	write(t, filepath.Join(dir, "shop.hozo"), "namespace shop {\n  op browse(): boolean;\n}\n"+
		"namespace shop.admin {\n  op ban(id: string): boolean;\n}\nnamespace other {\n  op f();\n}\n")
	// 単位は shop.admin だけ。shop と other は、どの単位の対象でもない
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("shop.admin", "gen/admin", `"admin"`))
	fakePlugin(t, dir, "admin", "go", "", `{"files":[],"warnings":[],"select":{"roots":["shop.admin.ban"]}}`, 0)

	code, stdout, stderr := run(t, dir, nil, "emit", "shop")
	if code != 0 || !strings.Contains(stdout, "written: gen/admin/ops.go\n") {
		t.Fatalf("子孫だけが単位の対象: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	want := readAll(t, filepath.Join(dir, "gen", "admin"))
	if err := os.RemoveAll(filepath.Join(dir, "gen")); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr = run(t, dir, nil, "emit"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if got := readAll(t, filepath.Join(dir, "gen", "admin")); !reflect.DeepEqual(got, want) {
		t.Errorf("引数 shop の出力が、すべてを対象にしたときと違う")
	}
	// other は仕様にあるが、どの単位の対象でもない
	if code, stdout, stderr = run(t, dir, nil, "emit", "other"); code != exitUsage || stdout != "" || !strings.Contains(stderr, "名前空間 other も") {
		t.Errorf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// TestEmitIn は docs/config.md「in」と docs/emit-go.md「出力の対象」: in は単位が出力する範囲だけを絞る。
// in の外で宣言された op は、選ばれた配線から辿られたら口だけが出る（stops と同じ）。選ばれた宣言が参照する型と状態は
// in の外でも出力する。in の外の宣言を選べば PLG-3。
func TestEmitIn(t *testing.T) {
	dir := unitsProject(t)
	got := emitAll(t, dir)
	ops, wiring := got["api/ops.go"], got["api/wiring.go"]
	// 単位 1（in = shop/api.hozo）: nightly は batch.hozo の配線。open から辿られて選ばれるが、本体は出さず、Leaves の口になる
	wantIn(t, "gen/api/ops.go", ops, []string{"type NightlyFunc func(item Item) error", "return o.leaves.Nightly(item)"}, []string{"SweepFunc"})
	wantIn(t, "gen/api/wiring.go", wiring, []string{"func (o *ops) Open(item Item) (Receipt, error) {", "o.Nightly(item)"}, []string{"func (o *ops) Nightly(", "Sweep"})
	// in の中の、効果を持つ葉 save は状態 items の命令（状態は in の中）
	wantIn(t, "gen/api/states.go", got["api/states.go"], []string{"type ItemsState struct {", "Save "}, nil)
	// 単位 2（in なし）では、同じ nightly が配線の本体を持つ
	wantIn(t, "gen/batch/wiring.go", got["batch/wiring.go"], []string{"func (o *ops) Nightly(item Item) error {", "o.Sweep(item)"}, nil)

	// 選ばれた宣言が参照する型は、in の外で宣言されていても出力する: 単位を「batch.hozo だけ」にすると、
	// nightly の引数の型 Item（api.hozo で宣言）は gen/batch に出る
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+
		"[[emit]]\nin = [\"shop/batch.hozo\"]\nnamespace = \"shop\"\ntranspilers = [\"batch\"]\nout = \"gen/batch\"\n\n"+
		unit("shop.admin", "gen/admin", `"admin"`))
	got = emitAll(t, dir)
	wantIn(t, "gen/batch/types.go", got["batch/types.go"], []string{"type Item struct {", "type Report struct {"}, nil)

	// in の外の宣言（api.hozo の open）を起点に選べば PLG-3（proc.MalformedOutput、70）。何も書かない
	before := mtimes(t, filepath.Join(dir, "gen"))
	fakePlugin(t, dir, "batch", "go", "", `{"files":[],"warnings":[],"select":{"roots":["shop.open"]}}`, 0)
	code, stdout, stderr := run(t, dir, nil, "emit")
	if code != exitSoftwareOrCrash || stdout != "" || !strings.Contains(stderr, "PLG-3") || !strings.Contains(stderr, `roots の "shop.open" は単位の対象でない（in の外の shop/api.hozo で宣言されている）`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for f, m := range mtimes(t, filepath.Join(dir, "gen")) {
		if before[f] != m {
			t.Errorf("gen/%s が書き換えられた", f)
		}
	}
	// stops と states も同じ
	for _, sel := range []string{`"stops":["shop.issue"]`, `"states":["shop.items"]`} {
		fakePlugin(t, dir, "batch", "go", "", `{"files":[],"warnings":[],"select":{`+sel+`}}`, 0)
		if code, _, stderr := run(t, dir, nil, "emit"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "単位の対象でない") {
			t.Errorf("%s: code=%d stderr=%q", sel, code, stderr)
		}
	}

	// in に合うファイルに宣言を持たなくなった名前空間は、その単位では出力しない。前回の出力は消える（docs/cli.md「emit」の 6）
	fakePlugin(t, dir, "batch", "go", "", `{"files":[],"warnings":[],"select":{"roots":["shop.report"]}}`, 0)
	write(t, filepath.Join(dir, "shop", "extra.hozo"), "namespace shop.extra {\n  op only(): string;\n}\n")
	fakePlugin(t, dir, "extra", "go", "", `{"files":[{"path":"gen/x/extra/e.txt","content":"e"}],"warnings":[]}`, 0)
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("shop.extra", "gen/x/extra", `"extra"`))
	if got = emitAll(t, dir); got["x/extra/e.txt"] != "e" {
		t.Fatalf("gen/x/extra/e.txt がない: %v", got)
	}
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+
		"[[emit]]\nin = [\"shop/api.hozo\"]\nnamespace = \"shop.extra\"\ntranspilers = [\"extra\"]\nout = \"gen/x/extra\"\n")
	code, stdout, stderr = run(t, dir, nil, "emit")
	if code != 0 || stdout != "removed: gen/x/extra/e.txt\n" {
		t.Fatalf("in で出力でなくなった名前空間: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if reqs := requests(t, dir, "extra"); len(reqs) != 1 { // 最初の emit の一回だけ。出力でなくなってからは起動しない
		t.Errorf("トランスパイラ extra が %d 回起動された（want 1）", len(reqs))
	}
}

// TestEmitImportCycle は GOEMIT-5（docs/emit-go.md「配置」「診断」）: 二つの出力が互いの型を import することになれば、
// 循環した Go を書かずに CheckFailed（1）。一方向の import は普通に通る。
func TestEmitImportCycle(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/cycle\n\ngo 1.24\n")
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  model S { x: string; }\n  op f(s: S, t: b.T);\n  op own(s: S);\n}\n")
	write(t, filepath.Join(dir, "b.hozo"), "namespace b {\n  model T { y: string; }\n  op g(s: a.S, t: T);\n}\n")
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", `"pa"`)+unit("b", "gen/b", `"pb"`))
	fakePlugin(t, dir, "pa", "go", "", `{"files":[],"warnings":[],"select":{"roots":["a.f"]}}`, 0)
	fakePlugin(t, dir, "pb", "go", "", `{"files":[],"warnings":[],"select":{"roots":["b.g"]}}`, 0)

	// a の出力は b.T を、b の出力は a.S を import することになる
	code, stdout, stderr := run(t, dir, nil, "emit")
	if code != exitCheckFailed || stdout != "" || !strings.Contains(stderr, "hozo.toml: error[GOEMIT-5]") ||
		!strings.Contains(stderr, "1 番目の [[emit]] の名前空間 a（gen/a） → 2 番目の [[emit]] の名前空間 b（gen/b） → 1 番目の [[emit]] の名前空間 a（gen/a）") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.Count(stderr, "GOEMIT-5") != 1 {
		t.Errorf("同じ輪を二度報告した:\n%s", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
		t.Fatalf("書き出している: %v", listFiles(t, filepath.Join(dir, "gen")))
	}
	if code, _, stderr = run(t, dir, nil, "emit", "--format=json"); code != exitCheckFailed || !strings.Contains(stderr, `"error": "hozoc.CheckFailed"`) || !strings.Contains(stderr, `"code": "GOEMIT-5"`) {
		t.Fatalf("json: code=%d stderr=%q", code, stderr)
	}
	// 対象を絞っても、対象の出力から届く輪は見つける
	for _, ns := range []string{"a", "b"} {
		if code, _, stderr = run(t, dir, nil, "emit", ns); code != exitCheckFailed || !strings.Contains(stderr, "GOEMIT-5") {
			t.Errorf("emit %s: code=%d stderr=%q", ns, code, stderr)
		}
	}
	// check は通る（循環は出力の形の問題で、仕様の誤りではない）
	if code, _, stderr = run(t, dir, nil, "check"); code != 0 {
		t.Fatalf("check: code=%d stderr=%q", code, stderr)
	}

	// 一方向なら普通: a が b の型を選ばなくなれば（own は a の型だけ）、b の出力が a のパッケージを import するだけ
	fakePlugin(t, dir, "pa", "go", "", `{"files":[],"warnings":[],"select":{"roots":["a.own"]}}`, 0)
	got := emitAll(t, dir)
	wantIn(t, "gen/b/ops.go", got["b/ops.go"], []string{`"example.com/cycle/gen/a"`, "s a.S"}, nil)
	if strings.Contains(got["a/ops.go"]+got["a/types.go"], "gen/b") {
		t.Errorf("gen/a が gen/b を import している")
	}
	// 相手の出力が持たない型は複製するので、輪にならない: b が a.S を使っていても、a の出力が S を持たなければ複製
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  model S { x: string; }\n  op f(t: b.T);\n}\n")
	fakePlugin(t, dir, "pa", "go", "", `{"files":[],"warnings":[],"select":{"roots":["a.f"]}}`, 0)
	got = emitAll(t, dir)
	wantIn(t, "gen/a/ops.go", got["a/ops.go"], []string{`"example.com/cycle/gen/b"`}, nil)
	wantIn(t, "gen/b/types.go", got["b/types.go"], []string{"type S struct {"}, []string{"gen/a"})
}

// TestEmitImportPrefersOwnUnit は docs/emit-go.md「配置」: 別の名前空間 M の型を持つ単位が複数あるとき、自分の単位が M も
// 出力していれば、その出力から import する（同じ単位の中で閉じられる依存を、別の単位へ向けない）。
// 二つの単位がどちらも shop と shop.admin を出力し、shop.admin.ban が shop.Item を使う。
func TestEmitImportPrefersOwnUnit(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh がない")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/units\n\ngo 1.24\n")
	write(t, filepath.Join(dir, "shop", "api.hozo"), shopAPI)
	write(t, filepath.Join(dir, "shop", "batch.hozo"), shopBatch)
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("shop", "gen/first", `"first"`)+unit("shop", "gen/second", `"second"`))
	// 視点が shop.admin なら ban を、shop なら nightly（Item を使う）を起点にするプラグイン。トランスパイラを二つ提供する。
	script := `if grep -q '"viewer": *"shop.admin"'; then printf '%s' '{"files":[],"warnings":[],"select":{"roots":["shop.admin.ban"]}}'; ` +
		`else printf '%s' '{"files":[],"warnings":[],"select":{"roots":["shop.nightly"]}}'; fi`
	write(t, filepath.Join(dir, "plugins", "both", "hozo-plugin.toml"),
		"name = \"both\"\ncommand = [\"sh\", \"-c\", \""+strings.ReplaceAll(strings.ReplaceAll(script, `\`, `\\`), `"`, `\"`)+"\"]\nprotocol = \"0.2\"\n"+
			"\n[[transpiler]]\nname = \"first\"\nlanguage = \"go\"\n\n[[transpiler]]\nname = \"second\"\nlanguage = \"go\"\n")
	got := emitAll(t, dir)
	wantIn(t, "gen/first/admin/ops.go", got["first/admin/ops.go"], []string{`"example.com/units/gen/first"`, "item shop.Item"}, []string{"gen/second"})
	wantIn(t, "gen/second/admin/ops.go", got["second/admin/ops.go"], []string{`"example.com/units/gen/second"`, "item shop.Item"}, []string{"gen/first"})
}
