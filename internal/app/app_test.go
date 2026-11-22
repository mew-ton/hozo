package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/fakeplugin"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/gen/hozoc/inner"
	"github.com/mew-ton/hozo/internal/gen/proc"
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
  fn ping(): throws Idle prevents effects;
}
`

// baseResponse は、テストの基底 base の応答: 出力ごとに、その Dir(U, N) の下に out.txt を一つ書く（中身は名前空間）。
// @DIR@ と @NS@ は、fakePlugin のスクリプトが、要求の view.config.dir と view.viewer に置き換える。hozoc 本体はコードを
// 出力しない（docs/plugin-protocol.md「この文書が決めること」）ので、書き出されるのは、トランスパイラが返したファイルだけ。
const baseResponse = `{"files":[{"path":"@DIR@/out.txt","content":"@NS@"}],"warnings":[]}`

// pluginDirs は [hozo] plugin_dirs = ["plugins"]。
const pluginDirs = "[hozo]\nplugin_dirs = [\"plugins\"]\n\n"

// unit は単位一つ分の hozo.toml。transpilers は transpilers の中身（`"base", "fake"` など）。
func unit(namespace, out, transpilers string) string {
	return "[[emit]]\nnamespace = \"" + namespace + "\"\ntranspilers = [" + transpilers + "]\nout = \"" + out + "\"\n\n"
}

// corpusConfig は copyCorpus が置く hozo.toml: server、browser と、使う型の置き場の http、page の四つの単位
// （docs/config.md「型の置き場」）。out は gen/<名前空間>。serverTranspilers は server の単位の transpilers の中身
// （`"base"`、`"base", "fake"` など）。ほかの三つは基底 base だけ。
func corpusConfig(serverTranspilers string) string {
	return pluginDirs + unit("server", "gen/server", serverTranspilers) + unit("browser", "gen/browser", `"base"`) +
		unit("http", "gen/http", `"base"`) + unit("page", "gen/page", `"base"`)
}

// copyCorpus は設計書のコーパスの .hozo を一時ディレクトリに写し、warning を一つ出す warn.hozo（warnFile）を足す。
// hozo.toml は corpusConfig で、どの単位も、テストの基底 base（にせもののプラグイン fakePlugin。出力ごとに out.txt を一つ書く）を使う。
// ファイルを返さない基底 none も入れておく。
func copyCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".hozo") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, e.Name()), string(data))
	}
	write(t, filepath.Join(dir, "warn.hozo"), warnFile)
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"base"`))
	fakePlugin(t, dir, "base", "go", "", baseResponse, 0)
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
	code = Main(context.Background(), a, args, func(k string) string { return env[k] }, &out, &errOut)
	return code, out.String(), errOut.String()
}

// innerOps は、a の実装で、内側のパッケージの Ops（内部の op writeOutputs と runEmit も持つ）を組み立てる。内部の op のテストは、
// ここから呼ぶ（docs/cli.md「手で書くもの」、docs/plugin-go.md「内側のフォルダと入口」）。引数は App.Ops と同じ。
func innerOps(a *App) inner.Ops {
	return inner.New(a, a.Sources(), a.Config(), a.Outputs(), a.OutputsSync())
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
	code, stdout, stderr := run(t, dir, nil, "build", "--no-emit")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	golden(t, "check_design.txt", stdout)
}

func TestCheckDesignJSON(t *testing.T) {
	dir := copyCorpus(t)
	code, stdout, stderr := run(t, dir, nil, "build", "--no-emit", "--format=json")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	golden(t, "check_design.json", stdout)
}

func TestCheckFailed(t *testing.T) {
	dir := copyCorpus(t)
	write(t, filepath.Join(dir, "bad.hozo"), "namespace server {\n  model Bad { x: Nope; }\n}\n")
	code, stdout, stderr := run(t, dir, nil, "build", "--no-emit")
	if code != exitCheckFailed || stdout != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "bad.hozo:2:") || !strings.Contains(stderr, "error[RES-1]") || !strings.Contains(stderr, "1 error, 1 warning") {
		t.Fatalf("stderr=%q", stderr)
	}
	code, stdout, stderr = run(t, dir, nil, "build", "--no-emit", "--format", "json")
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
	write(t, filepath.Join(dir, "a.hozo"), "namespace a { fn f(); }\n")
	code, stdout, stderr := run(t, dir, nil, "build", "--no-emit")
	if code != 0 || stderr != "" || stdout != "0 errors, 0 warnings\n" {
		t.Fatalf("check: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// warning は出る（終了コードは 0 のまま）
	write(t, filepath.Join(dir, "w.hozo"), "namespace a { model lower { id: string; } }\n")
	code, stdout, stderr = run(t, dir, nil, "build", "--no-emit")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "warning[NAME-1]") || !strings.Contains(stdout, "0 errors, 1 warning") {
		t.Fatalf("check: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// emit は CFG-7（check の warning も一緒に運ぶ）。何も書かない
	code, stdout, stderr = run(t, dir, nil, "build")
	if code != exitConfigInvalid || stdout != "" || !strings.Contains(stderr, "hozo.toml: error[CFG-7]") || !strings.Contains(stderr, "1 error, 1 warning") {
		t.Fatalf("emit: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, _, stderr = run(t, dir, nil, "build", "--dry-run", "--format=json")
	if code != exitConfigInvalid || !strings.Contains(stderr, `"error": "hozoc.ConfigInvalid"`) || !strings.Contains(stderr, `"code": "CFG-7"`) {
		t.Fatalf("emit json: code=%d stderr=%q", code, stderr)
	}
	if files := listFiles(t, dir); len(files) != 2 {
		t.Fatalf("emit がファイルを書いた: %v", files)
	}
	// error があれば CheckFailed（1）。設定の誤りではない
	write(t, filepath.Join(dir, "bad.hozo"), "namespace a { fn g(): Nope; }\n")
	code, _, stderr = run(t, dir, nil, "build", "--no-emit")
	if code != exitCheckFailed || !strings.Contains(stderr, "error[RES-1]") || strings.Contains(stderr, "CFG-") {
		t.Fatalf("check: code=%d stderr=%q", code, stderr)
	}
	// emit でも、検査の error が先（check の例外。CFG-7 にはならない）
	if code, _, stderr = run(t, dir, nil, "build"); code != exitCheckFailed || strings.Contains(stderr, "CFG-7") {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	if err := os.Remove(filepath.Join(dir, "bad.hozo")); err != nil {
		t.Fatal(err)
	}
	// 指定したファイルが読めなければ、設定なしではなく CFG-7（2）
	for _, args := range [][]string{{"build", "--no-emit", "--config", "nope.toml"}, {"build", "--config", "nope.toml"}} {
		if code, _, stderr = run(t, dir, nil, args...); code != exitConfigInvalid || !strings.Contains(stderr, "nope.toml: error[CFG-7]") {
			t.Fatalf("%v: code=%d stderr=%q", args, code, stderr)
		}
	}
	if code, _, stderr = run(t, dir, map[string]string{"HOZO_CONFIG": "nope.toml"}, "build", "--no-emit"); code != exitConfigInvalid || !strings.Contains(stderr, "error[CFG-7]") {
		t.Fatalf("HOZO_CONFIG: code=%d stderr=%q", code, stderr)
	}
	// 設定なしの App.Check は例外なしで結果（warning）を返す
	l := &App{Dir: dir}
	ctx := context.Background()
	res, err := l.Check(ctx, hozoc.CheckInput{})
	if err != nil || len(res.Warnings) != 1 || res.Warnings[0].Code != "NAME-1" {
		t.Fatalf("Check = %+v, %v", res, err)
	}
	// その後の WriteOutputs（内部の op。内側のパッケージの Ops から呼ぶ）は ConfigInvalid（CFG-7）: 配線の最初の collectOutputs
	// （葉の op）が投げる。述語 UpToDate と OutOfDate も、生成し直せないので同じ例外
	cfg7 := func(what string, err error) {
		t.Helper()
		var ci hozoc.ConfigInvalid
		if !errors.As(err, &ci) {
			t.Fatalf("%s err = %v, want ConfigInvalid", what, err)
		}
		if diags, _ := carriedBy(ci); len(diags) != 2 || diags[0].Code != "CFG-7" || diags[1].Code != "NAME-1" {
			t.Fatalf("%s の診断 = %+v, want CFG-7 と、check の warning", what, diags)
		}
	}
	_, err = innerOps(l).WriteOutputs(ctx, hozoc.BuildInput{}, res)
	cfg7("WriteOutputs", err)
	_, err = l.CollectOutputs(ctx, hozoc.BuildInput{}, res)
	cfg7("CollectOutputs", err)
	_, err = l.OutputsSync().UpToDate(ctx)
	cfg7("UpToDate", err)
	_, err = l.OutputsSync().OutOfDate(ctx)
	cfg7("OutOfDate", err)
}

func TestCheckInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a { fn f(); }\n")
	// 設定の誤り（CFG-12: 単位に out がない）は 2
	write(t, filepath.Join(dir, "hozo.toml"), "[[emit]]\nnamespace = \"a\"\ntranspilers = [\"t\"]\n")
	code, _, stderr := run(t, dir, nil, "build", "--no-emit")
	if code != exitConfigInvalid || !strings.Contains(stderr, "hozo.toml: error[CFG-12]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 名前空間ごとのテーブル（前の形）は知らないキー（CFG-7）
	write(t, filepath.Join(dir, "old.toml"), "[a]\nlanguage = \"go\"\nout = \"gen\"\n")
	if code, _, stderr := run(t, dir, nil, "build", "--no-emit", "--config", "old.toml"); code != exitConfigInvalid || !strings.Contains(stderr, "error[CFG-7]") {
		t.Fatalf("old form: code=%d stderr=%q", code, stderr)
	}
	// HOZO_CONFIG と --config
	write(t, filepath.Join(dir, "alt.toml"), "# 単位なし（何も出力しない）\n")
	write(t, filepath.Join(dir, "go.mod"), "module example.com/a\n")
	if code, _, stderr := run(t, dir, map[string]string{"HOZO_CONFIG": "alt.toml"}, "build", "--no-emit"); code != 0 {
		t.Fatalf("HOZO_CONFIG: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := run(t, dir, map[string]string{"HOZO_CONFIG": "alt.toml"}, "build", "--no-emit", "--config", "hozo.toml"); code != exitConfigInvalid {
		t.Fatalf("--config が HOZO_CONFIG より優先されていない: code=%d stderr=%q", code, stderr)
	}
}

// TestCheckLanguageMismatch は CFG-16（docs/config.md「基底と相乗り」「診断」）: 一つの単位のトランスパイラの language がそろって
// いなければ LanguageMismatch（終了コード 2）。ほかの設定の誤りと一緒なら ConfigInvalid。
func TestCheckLanguageMismatch(t *testing.T) {
	dir := copyCorpus(t)
	fakeRider(t, dir, "tsp", "ts", `"base"`, `{"files":[],"warnings":[]}`, 0)
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"base", "tsp"`))
	code, _, stderr := run(t, dir, nil, "build", "--no-emit")
	if code != exitConfigInvalid || !strings.Contains(stderr, "hozo.toml: error[CFG-16]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr = run(t, dir, nil, "build", "--no-emit", "--format=json"); code != exitConfigInvalid || !strings.Contains(stderr, `"error": "hozoc.LanguageMismatch"`) {
		t.Fatalf("json: code=%d stderr=%q", code, stderr)
	}
	l := &App{Dir: dir}
	_, err := l.Check(context.Background(), hozoc.CheckInput{})
	var lm hozoc.LanguageMismatch
	if !errors.As(err, &lm) {
		t.Fatalf("err = %v", err)
	}
	if diags, ok := carriedBy(lm); !ok || len(diags) == 0 || diags[0].Code != "CFG-16" {
		t.Fatalf("LanguageMismatch の診断 = %+v", diags)
	}
	// 知らないトランスパイラ（CFG-14）も一緒なら、設定の誤り（ConfigInvalid）
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"base", "tsp", "nope"`))
	_, err = l.Check(context.Background(), hozoc.CheckInput{})
	var ci hozoc.ConfigInvalid
	if !errors.As(err, &ci) {
		t.Fatalf("err = %v, want ConfigInvalid", err)
	}
	// 基底のない単位（CFG-20）、足りない依存（CFG-19）、型の置き場のない単位（CFG-22）も、設定の誤り
	for cfg, want := range map[string]string{
		corpusConfig(`"tsp"`): "error[CFG-19]",
		pluginDirs + unit("server", "gen/server", `"base", "none"`) + unit("browser", "gen/browser", `"base"`) +
			unit("http", "gen/http", `"base"`) + unit("page", "gen/page", `"base"`): "error[CFG-20]",
		pluginDirs + unit("browser", "gen/browser", `"base"`): "error[CFG-22]",
	} {
		write(t, filepath.Join(dir, "hozo.toml"), cfg)
		if code, _, stderr = run(t, dir, nil, "build", "--no-emit"); code != exitConfigInvalid || !strings.Contains(stderr, want) {
			t.Fatalf("%s: code=%d stderr=%q", want, code, stderr)
		}
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

func TestEmit(t *testing.T) {
	dir := copyCorpus(t)
	gen := filepath.Join(dir, "gen")

	// dry-run は書かない
	code, stdout, stderr := run(t, dir, nil, "build", "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run: code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(gen); !os.IsNotExist(err) {
		t.Fatalf("dry-run がファイルを書いた: %v", listFiles(t, gen))
	}
	if !strings.Contains(stdout, "written: gen/server/out.txt\n") || !strings.Contains(stdout, "written: gen/browser/out.txt\n") {
		t.Fatalf("dry-run stdout=%q", stdout)
	}

	// 書く。書き出されるのは、トランスパイラが返したファイルと、出力先ごとの記録だけ（hozoc 本体はコードを出力しない）
	code, stdout1, stderr := run(t, dir, nil, "build")
	if code != 0 {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	files := listFiles(t, gen)
	want := []string{"browser/.hozoc-outputs.json", "browser/out.txt", "http/.hozoc-outputs.json", "http/out.txt",
		"page/.hozoc-outputs.json", "page/out.txt", "server/.hozoc-outputs.json", "server/out.txt"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
	if b, err := os.ReadFile(filepath.Join(gen, "server", "out.txt")); err != nil || string(b) != "server" {
		t.Fatalf("gen/server/out.txt = %q, %v", b, err)
	}
	data, err := os.ReadFile(filepath.Join(gen, "server", ".hozoc-outputs.json"))
	if err != nil || string(data) != "[\n  \"gen/server/out.txt\"\n]\n" {
		t.Fatalf(".hozoc-outputs.json = %q (%v)", data, err)
	}
	for _, l := range strings.Split(strings.TrimSpace(stdout1), "\n") {
		if !strings.HasPrefix(l, "written: ") && !strings.Contains(l, "warning") && !strings.Contains(l, "errors,") {
			t.Fatalf("stdout line %q", l)
		}
	}

	// 二度目は何も書き直さない（mtime が変わらない）が、written には入る
	before := mtimes(t, gen)
	code, stdout2, _ := run(t, dir, nil, "build")
	if code != 0 || stdout2 != stdout1 {
		t.Fatalf("second: code=%d\n%s\n%s", code, stdout1, stdout2)
	}
	for f, m := range mtimes(t, gen) {
		if before[f] != m {
			t.Fatalf("%s が書き直された", f)
		}
	}

	// 名前空間を指定すれば、それだけ
	code, stdout3, stderr := run(t, dir, nil, "build", "--dry-run", "browser")
	if code != 0 || strings.Contains(stdout3, "gen/server/") || !strings.Contains(stdout3, "gen/browser/") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout3, stderr)
	}

	// json
	code, stdoutJ, _ := run(t, dir, nil, "build", "--format=json", "--dry-run")
	var res struct {
		Warnings []json.RawMessage `json:"warnings"`
		Written  []string          `json:"written"`
		Removed  []string          `json:"removed"`
	}
	if code != 0 || json.Unmarshal([]byte(stdoutJ), &res) != nil || len(res.Written) != 4 || res.Removed == nil || len(res.Warnings) != 1 {
		t.Fatalf("json: code=%d %s", code, stdoutJ)
	}
}

// TestEmitStaleAndConflict はトランスパイラの出力の書き出し、単位から外したあとの古いファイルの削除、CFG-5、PLG-*、errors、proc.Crashed。
func TestEmitStaleAndConflict(t *testing.T) {
	dir := copyCorpus(t)
	// fake は server の単位の二つ目のトランスパイラ（基底 base に相乗りする）
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"base", "fake"`))

	// fake が gen/server/extra.txt を書く（warning も一つ）
	fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[{"path":"gen/server/extra.txt","content":"hi"}],"warnings":[{"at":"server.placeOrder","message":"careful"}]}`, 0)
	code, stdout, stderr := run(t, dir, nil, "build")
	if code != 0 || !strings.Contains(stdout, "written: gen/server/extra.txt\n") || !strings.Contains(stdout, "warning[PLUGIN]: fake: careful") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "gen", "server", "extra.txt")); err != nil || string(b) != "hi" {
		t.Fatalf("extra.txt: %q %v", b, err)
	}

	// トランスパイラを単位から外すと extra.txt は古いファイルとして消える。基底のファイルは残る
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"base"`))
	code, stdout, stderr = run(t, dir, nil, "build")
	if code != 0 || !strings.Contains(stdout, "removed: gen/server/extra.txt\n") || strings.Contains(stdout, "removed: gen/server/out.txt") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen", "server", "extra.txt")); !os.IsNotExist(err) {
		t.Fatal("extra.txt が残っている")
	}

	// CFG-5: 同じ出力の二つのトランスパイラが同じパスを返す → 3、何も書かない
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"base", "fake"`))
	fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[{"path":"gen/server/out.txt","content":"x"}],"warnings":[]}`, 0)
	before := mtimes(t, filepath.Join(dir, "gen"))
	code, stdout, stderr = run(t, dir, nil, "build")
	if code != exitOutputConflict || stdout != "" || !strings.Contains(stderr, "gen/server/out.txt: error[CFG-5]") ||
		!strings.Contains(stderr, "トランスパイラ fake（1 番目の [[emit]] の名前空間 server）") || !strings.Contains(stderr, "トランスパイラ base（1 番目の [[emit]] の名前空間 server）") {
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
		fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[{"path":"`+path+`","content":"x"}],"warnings":[]}`, 0)
		before := mtimes(t, dir)
		if code, stdout, stderr = run(t, dir, nil, "build"); code != exitSoftwareOrCrash || stdout != "" || !strings.Contains(stderr, "PLG-1") {
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
	if code, _, stderr = run(t, dir, nil, "build", "--format=json"); code != exitSoftwareOrCrash || !strings.Contains(stderr, `"error": "proc.MalformedOutput"`) {
		t.Fatalf("json: code=%d stderr=%q", code, stderr)
	}
	// Dir(U, N) の下なら、深い場所でもよい
	fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[{"path":"gen/server/./deep/../deep/a.txt","content":"1"}],"warnings":[]}`, 0)
	if code, stdout, stderr = run(t, dir, nil, "build"); code != 0 || !strings.Contains(stdout, "written: gen/server/deep/a.txt\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// PLG-2: 同じ応答の中で同じ path が二度 → proc.MalformedOutput → 70
	fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[{"path":"gen/server/a.txt","content":"1"},{"path":"gen/server/./a.txt","content":"2"}],"warnings":[]}`, 0)
	if code, _, stderr = run(t, dir, nil, "build"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "PLG-2") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 応答の形が違う（必須の files / warnings がない、型が違う）→ 終了コード 0 でも proc.MalformedOutput → 70
	// （docs/plugin-protocol.md「応答の形」）。知らない項目（版 0.3 までの select を含む）があるだけなら通る
	for _, resp := range []string{`{"unknown":1}`, `{"files":[]}`, `{"files":[],"warnings":"none"}`, `{"files":[{"path":"gen/server/a.txt"}],"warnings":[]}`} {
		fakeRider(t, dir, "fake", "go", `"base"`, resp, 0)
		if code, stdout, stderr = run(t, dir, nil, "build"); code != exitSoftwareOrCrash || stdout != "" || !strings.Contains(stderr, "応答の形が違う") {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", resp, code, stdout, stderr)
		}
	}
	fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[],"warnings":[],"unknown":1,"select":{"roots":["server.nope"]}}`, 0)
	if code, _, stderr = run(t, dir, nil, "build"); code != 0 {
		t.Fatalf("知らない項目: code=%d stderr=%q", code, stderr)
	}
	// 異常終了 → 70
	fakeRider(t, dir, "fake", "go", `"base"`, ``, 3)
	if code, _, stderr = run(t, dir, nil, "build"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "hozoc: ") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

// TestEmitErrors は docs/cli.md「書き出し」の 2: どれかのトランスパイラが errors を返せば、残りのトランスパイラも起動して errors を
// すべて集めたうえで CheckFailed（1）。emit の全体が、何も書き出さない（errors を返していない出力のファイルも）。
func TestEmitErrors(t *testing.T) {
	dir := copyCorpus(t)
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"base", "fake", "fake2"`))
	fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[{"path":"gen/server/extra.txt","content":"hi"}],"warnings":[{"message":"w"}],"errors":[{"code":"FAKE-1","at":"server.placeOrder","message":"nope"}]}`, 0)
	fakeRider(t, dir, "fake2", "go", `"base"`, `{"files":[],"warnings":[],"errors":[{"code":"FAKE-2","message":"again"}]}`, 0)
	code, stdout, stderr := run(t, dir, nil, "build")
	if code != exitCheckFailed || stdout != "" || !strings.Contains(stderr, "server.hozo:281:3: error[FAKE-1]: nope") ||
		!strings.Contains(stderr, "error[FAKE-2]: again") || !strings.Contains(stderr, "warning[PLUGIN]: fake: w") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
		t.Fatalf("書き出している: %v", listFiles(t, filepath.Join(dir, "gen")))
	}
	// 残りの出力のトランスパイラも起動している（errors をすべて集める）
	if n := len(requests(t, dir, "base")); n != 4 {
		t.Errorf("基底 base が %d 回起動された（want 4: 出力ごとに一回）", n)
	}
	if code, _, stderr = run(t, dir, nil, "build", "--format=json"); code != exitCheckFailed || !strings.Contains(stderr, `"error": "hozoc.CheckFailed"`) || !strings.Contains(stderr, `"code": "FAKE-2"`) {
		t.Fatalf("json: code=%d stderr=%q", code, stderr)
	}
	// dry-run でも同じ（書く予定を返さない）
	if code, stdout, _ = run(t, dir, nil, "build", "--dry-run"); code != exitCheckFailed || stdout != "" {
		t.Fatalf("dry-run: code=%d stdout=%q", code, stdout)
	}
}

// TestEmitNestedOutput は PLG-1 の「別の出力の Dir」: 単位が子孫の名前空間も出力するとき、Dir(U, N.child) は Dir(U, N) の下にある。
// N の出力のトランスパイラは、そこには書けない。記録は出力ごとで、消したファイルのディレクトリが空になれば、それも消す
// （docs/cli.md「書き出し」の 5）。
func TestEmitNestedOutput(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\nnamespace a.child {\n  fn g();\n}\n")
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", `"base", "fake"`))
	fakePlugin(t, dir, "base", "go", "", baseResponse, 0)
	// fake は、どの出力でも、Dir(U, N) の下の inner/deep/x.txt を書く
	fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[{"path":"@DIR@/inner/deep/x.txt","content":"@NS@"}],"warnings":[]}`, 0)
	code, stdout, stderr := run(t, dir, nil, "build")
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	got := listFiles(t, filepath.Join(dir, "gen"))
	want := []string{"a/.hozoc-outputs.json", "a/child/.hozoc-outputs.json", "a/child/inner/deep/x.txt", "a/child/out.txt", "a/inner/deep/x.txt", "a/out.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	// a の記録は、a の出力のファイルだけ（子の出力のものは、子の記録）
	if rec, _ := os.ReadFile(filepath.Join(dir, "gen", "a", OutputsFile)); string(rec) != "[\n  \"gen/a/inner/deep/x.txt\",\n  \"gen/a/out.txt\"\n]\n" {
		t.Errorf("gen/a の記録 = %s", rec)
	}
	// fake を外すと、x.txt が消え、空になった inner/deep と inner も消える。Dir(U, N) そのものと、子の出力の Dir は残る
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", `"base"`))
	code, stdout, stderr = run(t, dir, nil, "build")
	if code != 0 || !strings.Contains(stdout, "removed: gen/a/child/inner/deep/x.txt\n") || !strings.Contains(stdout, "removed: gen/a/inner/deep/x.txt\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, gone := range []string{"gen/a/inner", "gen/a/child/inner"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("空になった %s が残っている", gone)
		}
	}
	if got := listFiles(t, filepath.Join(dir, "gen")); !reflect.DeepEqual(got, []string{"a/.hozoc-outputs.json", "a/child/.hozoc-outputs.json", "a/child/out.txt", "a/out.txt"}) {
		t.Errorf("files = %v", got)
	}
	// 空でないディレクトリ（人が置いたファイルがある）は消さない
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", `"base", "fake"`))
	if code, _, stderr = run(t, dir, nil, "build"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	write(t, filepath.Join(dir, "gen", "a", "inner", "keep.txt"), "mine")
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", `"base"`))
	if code, _, stderr = run(t, dir, nil, "build"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen", "a", "inner", "keep.txt")); err != nil {
		t.Errorf("人のファイルが消えた: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen", "a", "inner", "deep")); !os.IsNotExist(err) {
		t.Error("空になった gen/a/inner/deep が残っている")
	}
	// a の出力のトランスパイラが、子の出力の Dir（gen/a/child）の下に書けば PLG-1
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", `"base", "fake"`))
	for _, path := range []string{"gen/a/child/x.txt", "gen/a/child/deep/x.txt", "gen/a/child"} {
		fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[{"path":"`+path+`","content":"x"}],"warnings":[]}`, 0)
		if code, stdout, stderr = run(t, dir, nil, "build", "a.child"); code != exitSoftwareOrCrash || stdout != "" || !strings.Contains(stderr, "PLG-1") {
			if path != "gen/a/child" { // a.child の出力から見れば、gen/a/child/x.txt は自分の Dir の下
				continue
			}
			t.Fatalf("path %q: code=%d stdout=%q stderr=%q", path, code, stdout, stderr)
		}
		if code, stdout, stderr = run(t, dir, nil, "build"); code != exitSoftwareOrCrash || stdout != "" || !strings.Contains(stderr, "PLG-1") {
			t.Fatalf("path %q: code=%d stdout=%q stderr=%q", path, code, stdout, stderr)
		}
	}
}

// TestEmitStaleRecordStaysInDir は docs/cli.md「書き出し」の 5: .hozoc-outputs.json のパスに従って消すのは、その記録の持ち主の
// Dir(U, N) の下のファイルだけ。記録に `..` や絶対パス、ルートの中の別のディレクトリ、別の出力の Dir のパスが
// 書かれていても（壊れた記録、手で直した記録）、そのファイルは消さず、removed にも出さない。Dir の下の古いファイルは消す。
func TestEmitStaleRecordStaysInDir(t *testing.T) {
	dir := copyCorpus(t)
	if code, _, stderr := run(t, dir, nil, "build"); code != 0 {
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

	code, stdout, stderr := run(t, dir, nil, "build")
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
	// 消したファイルのディレクトリが空になれば、Dir(U, N) の下の範囲で、そのディレクトリも消す
	if _, err := os.Stat(filepath.Join(dir, "gen", "server", "sub")); !os.IsNotExist(err) {
		t.Error("空になった gen/server/sub が残っている")
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

// TestEmitNothing は docs/cli.md「書き出し」の 5 と 7: 単位のどのトランスパイラもファイルを返さなければ、その出力には何も書かない。
// 前回の出力は古いファイルとして消え、.hozoc-outputs.json も残らない。エラーでも warning でもない。
// 単位そのものを消したときは、前の出力先が分からないので何も消さない。
func TestEmitNothing(t *testing.T) {
	dir := copyCorpus(t)
	gen := filepath.Join(dir, "gen")
	if code, _, stderr := run(t, dir, nil, "build"); code != 0 {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	if files := listFiles(t, gen); !containsStr(files, "server/out.txt") || !containsStr(files, "server/.hozoc-outputs.json") || !containsStr(files, "browser/out.txt") {
		t.Fatalf("files = %v", files)
	}

	// 基底が、どの出力にもファイルを返さなくなる。前回の出力は消え、記録も残らない
	fakePlugin(t, dir, "base", "go", "", `{"files":[],"warnings":[]}`, 0)
	code, stdout, stderr := run(t, dir, nil, "build")
	if code != 0 || strings.Contains(stdout, "written: ") || !strings.Contains(stdout, "removed: gen/server/out.txt\n") || !strings.Contains(stdout, "removed: gen/browser/out.txt\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// warning は check のもの（warn.hozo の REL-2）だけ
	if strings.Count(stdout, "warning[") != 1 {
		t.Fatalf("stdout=%q", stdout)
	}
	if files := listFiles(t, gen); len(files) != 0 {
		t.Errorf("gen に残っている: %v", files)
	}
	// 二度目は何も書かず、何も消さない
	code, stdout, _ = run(t, dir, nil, "build")
	if code != 0 || strings.Contains(stdout, "removed: ") || strings.Contains(stdout, "written: ") {
		t.Fatalf("second: code=%d stdout=%q", code, stdout)
	}
	fakePlugin(t, dir, "base", "go", "", baseResponse, 0)

	// server の単位を hozo.toml から消す: server はどの単位の対象でもなくなり、前の出力先は分からないので何も消さない
	// （docs/cli.md「書き出し」の 5）。エラーにもならない
	if code, _, stderr := run(t, dir, nil, "build"); code != 0 {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	// （browser は server の型を使うので、server の単位を消すなら、browser の単位も消す。docs/config.md「型の置き場」）
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("http", "gen/http", `"base"`)+unit("page", "gen/page", `"base"`))
	code, stdout, stderr = run(t, dir, nil, "build")
	if code != 0 || strings.Contains(stdout, "removed: ") || strings.Contains(stdout, "gen/server/") {
		t.Fatalf("unit removed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(gen, "server", "out.txt")); err != nil {
		t.Errorf("単位を消したのに gen/server/out.txt が消えた: %v", err)
	}
	// どの単位の対象でもない名前空間を emit の引数に書けば UnknownNamespace
	if code, _, stderr = run(t, dir, nil, "build", "server"); code != exitUsage || !strings.Contains(stderr, "名前空間 server も、その子孫も、hozo.toml のどの単位の対象でもない") {
		t.Fatalf("emit server: code=%d stderr=%q", code, stderr)
	}
}

// TestUpToDate は cli.hozo の分類 OutputsSync の述語 UpToDate と OutOfDate（docs/cli.md「cli.hozo」「手で書くもの」）: 生成し直した
// 内容とディスクの内容を比べる。生成したコードの判定関数 hozoc.ClassifyOutputsSync が、状態の分類の単位（App.OutputsSync）の
// 二つの述語を呼び、真がちょうど一つのケースを返す（OutOfDate は UpToDate の否定なので、いつも、どちらか一つ）。
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
	ctx := context.Background()
	a := &App{Stderr: &bytes.Buffer{}}
	ops := a.Ops()
	unit := a.OutputsSync() // 判定関数に渡す、状態の分類の単位（hozoc.New に渡したものと同じ App のもの）
	sync := func() hozoc.OutputsSync {
		t.Helper()
		got, err := hozoc.ClassifyOutputsSync(ctx, unit)
		if err != nil {
			t.Fatalf("ClassifyOutputsSync: %v", err)
		}
		return got
	}
	// predicates は、二つの述語の値（UpToDate、OutOfDate）
	predicates := func() [2]bool {
		t.Helper()
		up, err := unit.UpToDate(ctx)
		if err != nil {
			t.Fatalf("UpToDate: %v", err)
		}
		out, err := unit.OutOfDate(ctx)
		if err != nil {
			t.Fatalf("OutOfDate: %v", err)
		}
		return [2]bool{up, out}
	}
	// まだ書き出していない: 足りない
	if got := sync(); got != hozoc.OutputsSyncOutOfDate {
		t.Fatalf("emit の前: %v", got)
	}
	if got := predicates(); got != [2]bool{false, true} {
		t.Errorf("emit の前の述語（UpToDate、OutOfDate）= %v", got)
	}
	// 配線の emit（check → writeOutputs → collectOutputs、outputChanges、syncOutputs）。効果 syncOutputs は、状態の分類
	// OutputsSync の単位の命令で、どの場合からも UpToDate に移す
	res, err := ops.Build(ctx, hozoc.BuildInput{})
	if err != nil || len(res.Written) == 0 {
		t.Fatalf("Emit = %+v, %v", res, err)
	}
	if got := sync(); got != hozoc.OutputsSyncUpToDate {
		t.Fatalf("emit の後: %v", got)
	}
	if got := predicates(); got != [2]bool{true, false} {
		t.Errorf("emit の後の述語（UpToDate、OutOfDate）= %v", got)
	}
	// 古い（内容が違う）
	types := filepath.Join(dir, "gen", "server", "out.txt")
	orig, err := os.ReadFile(types)
	if err != nil {
		t.Fatal(err)
	}
	write(t, types, string(orig)+"\n手で足した行\n")
	if got := sync(); got != hozoc.OutputsSyncOutOfDate {
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
	if got := sync(); got != hozoc.OutputsSyncOutOfDate {
		t.Errorf("消した後: %v", got)
	}
	write(t, types, string(orig))
	// 余分（前回書いて、今回は書かないファイルが残っている）: 基底が、ファイルを返さなくなる
	fakePlugin(t, dir, "base", "go", "", `{"files":[],"warnings":[]}`, 0)
	b := &App{Stderr: &bytes.Buffer{}} // 設定を読み直すために新しい App（直前の Check の結果を持たない）
	ops, unit = b.Ops(), b.OutputsSync()
	if got := sync(); got != hozoc.OutputsSyncOutOfDate {
		t.Errorf("基底がファイルを返さなくなった後: %v", got)
	}
	if _, err := ops.Build(ctx, hozoc.BuildInput{}); err != nil {
		t.Fatal(err)
	}
	if got := sync(); got != hozoc.OutputsSyncUpToDate {
		t.Errorf("もう一度 emit した後: %v", got)
	}
	// 仕様に誤りがあって生成し直せなければ、その例外
	write(t, filepath.Join(dir, "bad.hozo"), "namespace server {\n  model Bad { x: Nope; }\n}\n")
	c := &App{Stderr: &bytes.Buffer{}}
	var cf hozoc.CheckFailed
	if _, err := hozoc.ClassifyOutputsSync(ctx, c.OutputsSync()); !errors.As(err, &cf) {
		t.Errorf("誤りのある仕様: err = %v, want CheckFailed", err)
	}
}

// TestWriteOutputsSteps は、配線 writeOutputs が呼ぶ三つ（docs/cli.md「書き出し」「手で書くもの」）を、一つずつ動かす: 葉の op collectOutputs
// は、ファイルに触れずに、書き出す予定を集める。read outputChanges（状態 outputs の単位の問い合わせ）は、予定を今のファイルと比べて、
// 書くファイルと消すファイルを決めるだけで、何も変えない。効果 syncOutputs（状態の分類 OutputsSync の単位の命令）が、書き出す。
// dryRun の予定では、消すファイルを決めず、書き出さない。予定は、直前の collectOutputs が返したものでなければ、誤り。
func TestWriteOutputsSteps(t *testing.T) {
	dir := copyCorpus(t)
	gen := filepath.Join(dir, "gen")
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"base", "fake"`))
	fakeRider(t, dir, "fake", "go", `"base"`, `{"files":[{"path":"gen/server/extra.txt","content":"hi"}],"warnings":[{"at":"server.placeOrder","message":"careful"}]}`, 0)
	ctx := context.Background()
	a := &App{Dir: dir, Stderr: &bytes.Buffer{}}
	ops := innerOps(a)
	if _, err := ops.Build(ctx, hozoc.BuildInput{}); err != nil {
		t.Fatal(err)
	}
	// fake を単位から外す: gen/server/extra.txt は、前回書いて、今回は書かないファイル。基底は、内容を変える
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"base"`))
	fakePlugin(t, dir, "base", "go", "", `{"files":[{"path":"@DIR@/out.txt","content":"new @NS@"}],"warnings":[{"message":"again"}]}`, 0)
	checked, err := ops.Check(ctx, hozoc.CheckInput{})
	if err != nil {
		t.Fatal(err)
	}
	before, beforeTimes := readAll(t, gen), mtimes(t, gen)
	unchanged := func(what string) {
		t.Helper()
		if !reflect.DeepEqual(readAll(t, gen), before) || !reflect.DeepEqual(mtimes(t, gen), beforeTimes) {
			t.Fatalf("%s が、生成したファイルに触れた", what)
		}
	}
	paths := []hozoc.Path{"gen/browser/out.txt", "gen/http/out.txt", "gen/page/out.txt", "gen/server/out.txt"}

	// collectOutputs: トランスパイラのファイル（パスの辞書順）と、check の warning（トランスパイラの warning を含む: check が、
	// プラグインに頼んで、もう受け取っている。四つの出力の基底が返す、同じ warning は、一度だけ。docs/plugin-protocol.md「emit」）
	plan, err := ops.CollectOutputs(ctx, hozoc.BuildInput{}, checked)
	if err != nil {
		t.Fatal(err)
	}
	unchanged("CollectOutputs")
	var planned []hozoc.Path
	for _, f := range plan.Files {
		planned = append(planned, f.Path)
	}
	if !reflect.DeepEqual(planned, paths) || plan.Files[3].Content != "new server" || plan.DryRun != nil {
		t.Fatalf("plan.Files = %+v, DryRun = %v", plan.Files, plan.DryRun)
	}
	plugins := 0
	for _, w := range plan.Warnings {
		if w.Code == "PLUGIN" {
			plugins++
		}
	}
	if !reflect.DeepEqual(plan.Warnings, checked.Warnings) || plugins != 1 || plan.Warnings[0].Code != "PLUGIN" {
		t.Fatalf("plan.Warnings = %+v（want check の %d 個と同じで、基底の warning が、一つ）", plan.Warnings, len(checked.Warnings))
	}

	// outputChanges: 書くファイルは、予定のすべて（内容が同じでも）。消すファイルは、前回書いて、今回は書かないもの
	changes, err := a.Outputs().OutputChanges(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	unchanged("OutputChanges")
	if !reflect.DeepEqual(changes.Written, paths) || !reflect.DeepEqual(changes.Removed, []hozoc.Path{"gen/server/extra.txt"}) ||
		!reflect.DeepEqual(changes.Warnings, plan.Warnings) {
		t.Fatalf("OutputChanges = %+v", changes)
	}

	// dryRun の予定: 書くファイルだけを返し（消すファイルは決めない）、syncOutputs は何もしない
	yes := true
	dry, err := ops.CollectOutputs(ctx, hozoc.BuildInput{DryRun: &yes}, checked)
	if err != nil || dry.DryRun == nil || !*dry.DryRun {
		t.Fatalf("dryRun の plan = %+v, %v", dry, err)
	}
	if changes, err = a.Outputs().OutputChanges(ctx, dry); err != nil || !reflect.DeepEqual(changes.Written, paths) || changes.Removed == nil || len(changes.Removed) != 0 {
		t.Fatalf("dryRun の OutputChanges = %+v, %v", changes, err)
	}
	if err := a.OutputsSync().SyncOutputs(ctx, dry); err != nil {
		t.Fatal(err)
	}
	unchanged("dryRun の SyncOutputs")

	// 予定は、直前の collectOutputs が返したもの（ここでは dry）でなければ、誤り。何も変えない
	stale := plan
	stale.Files = plan.Files[:1]
	if _, err := a.Outputs().OutputChanges(ctx, stale); err == nil {
		t.Error("直前の CollectOutputs の結果でない予定で、OutputChanges が通った")
	}
	if err := a.OutputsSync().SyncOutputs(ctx, stale); err == nil {
		t.Error("直前の CollectOutputs の結果でない予定で、SyncOutputs が通った")
	}
	unchanged("誤った予定の SyncOutputs")

	// collectOutputs は、受け取った ctx で、トランスパイラを起動する: 中断された ctx では、集めずに、その error を返す。
	// 配線 writeOutputs も、その error を、そのまま返し、何も書かない
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := ops.CollectOutputs(cancelled, hozoc.BuildInput{}, checked); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx の CollectOutputs: err = %v, want context.Canceled", err)
	}
	if _, err := ops.WriteOutputs(cancelled, hozoc.BuildInput{}, checked); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx の WriteOutputs: err = %v, want context.Canceled", err)
	}
	unchanged("中断された ctx の WriteOutputs")

	// syncOutputs: 余分なファイルを消し、古いファイルを書き換え、記録を書く。その後は UpToDate で、消すファイルはない
	if plan, err = ops.CollectOutputs(ctx, hozoc.BuildInput{}, checked); err != nil {
		t.Fatal(err)
	}
	if err := a.OutputsSync().SyncOutputs(ctx, plan); err != nil {
		t.Fatal(err)
	}
	after := readAll(t, gen)
	if _, left := after["server/extra.txt"]; left || after["server/out.txt"] != "new server" || after["server/.hozoc-outputs.json"] != "[\n  \"gen/server/out.txt\"\n]\n" {
		t.Fatalf("SyncOutputs の後の gen = %v", after)
	}
	if got, err := hozoc.ClassifyOutputsSync(ctx, a.OutputsSync()); err != nil || got != hozoc.OutputsSyncUpToDate {
		t.Errorf("SyncOutputs の後の OutputsSync = %v, %v", got, err)
	}
	if changes, err = a.Outputs().OutputChanges(ctx, plan); err != nil || len(changes.Removed) != 0 || !reflect.DeepEqual(changes.Written, paths) {
		t.Errorf("SyncOutputs の後の OutputChanges = %+v, %v", changes, err)
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

// TestCacheDirEnvironment は docs/plugin-protocol.md「起動」の、機械の言葉に直したものの置き場が、hozoc に届く道:
// HOZO_CACHE_DIR は、cli.Main に渡した env から、cli.hozo の入力の cacheDir に入る。プロセスの環境変数は、読まない。
func TestCacheDirEnvironment(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", `"base"`))
	fakePlugin(t, dir, "base", "go", "", baseResponse, 0)
	passed, ambient := t.TempDir(), t.TempDir()
	t.Setenv("HOZO_CACHE_DIR", ambient)
	empty := func(d string) bool {
		entries, err := os.ReadDir(d)
		return err != nil || len(entries) == 0
	}
	if code, _, stderr := runHome(t, dir, "", map[string]string{"HOZO_CACHE_DIR": passed}, "build", "--no-emit"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if empty(filepath.Join(passed, "wasm")) {
		t.Errorf("渡した置き場 %s の下の wasm/ に、覚えたものが、ない", passed)
	}
	if !empty(ambient) {
		t.Errorf("プロセスの環境変数 HOZO_CACHE_DIR の置き場 %s を、使った", ambient)
	}
}

// TestManifestCommandIsGone は docs/plugin-protocol.md「診断」の MAN-6: なくした書き方 command のマニフェストがあれば、
// hozoc build は、設定の誤り（ConfigInvalid）で止まり、新しい書き方を示す。プラグインは、起動しない。
func TestManifestCommandIsGone(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", `"base"`))
	fakePlugin(t, dir, "base", "go", "", baseResponse, 0)
	manifest := filepath.Join(dir, "plugins", "base", "hozo-plugin.toml")
	write(t, manifest, strings.Replace(readFile(t, manifest), "wasm = "+fakeplugin.WasmTOML, `command = ["go", "run", "example.com/base"]`, 1))
	code, _, stderr := run(t, dir, nil, "build", "--no-emit")
	if code != exitConfigInvalid || !strings.Contains(stderr, "error[MAN-6]") || !strings.Contains(stderr, `wasm = "`) {
		t.Errorf("code=%d stderr=%q", code, stderr)
	}
	if n := strings.Count(readFile(t, filepath.Join(dir, "plugins", "base", "launches")), "\n"); n != 0 {
		t.Errorf("プラグインを %d 回、起動した", n)
	}
}

// TestPluginSearchEnvironment は docs/plugin-protocol.md「探索」の 2 と 3 が hozoc に届く道: HOZO_PLUGIN_PATH は cli.Main に
// 渡した env から cli.hozo の入力の pluginPath に入り、ホームは App.Home。プロセスの環境変数とホームディレクトリは読まない。
func TestPluginSearchEnvironment(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\nnamespace b {\n  fn g();\n}\n")
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("a", "gen/a", `"from-env"`)+unit("b", "gen/b", `"from-home"`))
	fakePlugin(t, dir, "from-env", "go", "", baseResponse, 0)
	fakePlugin(t, dir, "from-home", "go", "", baseResponse, 0)
	// a の単位のトランスパイラ from-env を、プロジェクトの外（HOZO_PLUGIN_PATH のディレクトリ）へ移す
	envDir, home := t.TempDir(), t.TempDir()
	if err := os.Rename(filepath.Join(dir, "plugins", "from-env"), filepath.Join(envDir, "from-env")); err != nil {
		t.Fatal(err)
	}
	// b の単位の from-home は、ホームへ移す
	write(t, filepath.Join(home, ".hozo", "plugins", ".keep"), "")
	if err := os.Rename(filepath.Join(dir, "plugins", "from-home"), filepath.Join(home, ".hozo", "plugins", "from-home")); err != nil {
		t.Fatal(err)
	}

	// にせものに見せるディレクトリは、置いた場所で決まるので、移した先で、置き直す
	fakeplugin.Install(t, filepath.Join(envDir, "from-env"))
	fakeplugin.Install(t, filepath.Join(home, ".hozo", "plugins", "from-home"))

	// プロセスの環境に向けても、渡していなければ見つからない（CFG-14）
	t.Setenv("HOZO_PLUGIN_PATH", envDir)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	code, _, stderr := run(t, dir, nil, "build", "--no-emit")
	if code != exitConfigInvalid || strings.Count(stderr, "error[CFG-14]") != 2 {
		t.Fatalf("渡していない: code=%d stderr=%q", code, stderr)
	}
	// 渡せば見つかる。check も emit も lsp も同じ入力を持つ
	env := map[string]string{"HOZO_PLUGIN_PATH": t.TempDir() + string(filepath.ListSeparator) + envDir}
	if code, _, stderr = runHome(t, dir, home, env, "build", "--no-emit"); code != 0 {
		t.Fatalf("check: code=%d stderr=%q", code, stderr)
	}
	code, stdout, stderr := runHome(t, dir, home, env, "build")
	if code != 0 || !strings.Contains(stdout, "written: gen/a/out.txt\n") || !strings.Contains(stdout, "written: gen/b/out.txt\n") {
		t.Fatalf("emit: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, _, stderr = runHome(t, dir, home, env, "lsp"); code != 0 {
		t.Fatalf("lsp: code=%d stderr=%q", code, stderr)
	}
	// 片方だけなら、もう片方の単位のトランスパイラが見つからない
	if code, _, stderr = runHome(t, dir, "", env, "build", "--no-emit"); code != exitConfigInvalid || !strings.Contains(stderr, "from-home を提供するプラグインが入っていない") {
		t.Fatalf("ホームなし: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr = runHome(t, dir, home, nil, "build", "--no-emit"); code != exitConfigInvalid || !strings.Contains(stderr, "from-env を提供するプラグインが入っていない") {
		t.Fatalf("HOZO_PLUGIN_PATH なし: code=%d stderr=%q", code, stderr)
	}
	// 外の場所の壊れたマニフェストは、使っていなくても報告する（MAN-4 → ConfigInvalid）
	write(t, filepath.Join(home, ".hozo", "plugins", "broken", "hozo-plugin.toml"), "name = \n")
	if code, _, stderr = runHome(t, dir, home, env, "build", "--no-emit"); code != exitConfigInvalid || !strings.Contains(stderr, "error[MAN-4]") {
		t.Fatalf("壊れたマニフェスト: code=%d stderr=%q", code, stderr)
	}
}

func TestEmitUnknownNamespace(t *testing.T) {
	dir := copyCorpus(t)
	code, stdout, stderr := run(t, dir, nil, "build", "nope")
	if code != exitUsage || stdout != "" || !strings.Contains(stderr, "名前空間 nope も、その子孫も、hozo.toml のどの単位の対象でもない") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
		t.Fatal("書き出している")
	}
	code, _, stderr = run(t, dir, nil, "build", "--format=json", "nope")
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
		{[]string{"build", "--no-emit", "--help"}, 0},
		{[]string{"bogus"}, exitUsage},
		{[]string{"build", "--no-emit", "--nope"}, exitUsage},
		{[]string{"build", "--no-emit", "--format"}, exitUsage},
		{[]string{"build", "--no-emit", "--format=yaml"}, exitUsage},
		{[]string{"build", "--no-emit", "server"}, 0},          // 名前空間の引数は、--no-emit でも書ける（集めて確かめる範囲を選ぶ）
		{[]string{"build", "--no-emit", "nowhere"}, exitUsage}, // UnknownNamespace も、同じ
		{[]string{"check"}, exitUsage},                         // なくしたコマンド（TestRenamedCommands）
		{[]string{"emit", "--dry-run"}, exitUsage},
		{[]string{"build", "--dry-run=maybe"}, exitUsage},
		{[]string{"lsp"}, 0}, // 入力が閉じれば終わる。終了コードは常に 0（docs/lsp.md「ライフサイクル」）
		{[]string{"build", "--dry-run", "--", "server"}, 0},
		{[]string{"build", "--format", "text", "--format", "json", "--dry-run", "-x"}, exitUsage}, // -x は位置引数（知らない名前空間）
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

// requestsFile は fakePlugin が、受け取った頼みごとを一行ずつ足していくファイル（マニフェストのディレクトリの下）。
const requestsFile = "requests.jsonl"

// fakePlugin は、にせもののプラグイン（internal/fakeplugin）を plugins/<name> に置く。提供するトランスパイラは、プラグインと同じ
// 名前の基底一つ（言語は lang）。stdout は、トランスパイラ一つの、出力一つへの答えの JSON（@DIR@ と @NS@ は、その出力の
// Dir(U, N) と名前空間に置き換わる。にせものは、要求の出力とトランスパイラの組ごとに、これを並べて、応答にする:
// docs/plugin-protocol.md「emit」）、exit は終了コード。extra はマニフェストに足す行。受け取った頼みごとは、出力とトランスパイラの
// 組ごとに一行、マニフェストのディレクトリの requestsFile に足す（requests で読む）。起動の回数は、launches で読む。
func fakePlugin(t *testing.T, dir, name, lang, extra, stdout string, exit int) {
	t.Helper()
	fakeTranspiler(t, dir, name, lang, extra, "", stdout, exit)
}

// fakeRider は fakePlugin と同じで、提供するトランスパイラが相乗り（requires は `"base"` のような、配列の中身）。
func fakeRider(t *testing.T, dir, name, lang, requires, stdout string, exit int) {
	t.Helper()
	fakeTranspiler(t, dir, name, lang, "", "requires = ["+requires+"]\n", stdout, exit)
}

func fakeTranspiler(t *testing.T, dir, name, lang, extra, transpilerExtra, stdout string, exit int) {
	t.Helper()
	pluginDir := filepath.Join(dir, "plugins", name)
	fakeplugin.Install(t, pluginDir)
	write(t, filepath.Join(pluginDir, "response.json"), stdout)
	write(t, filepath.Join(pluginDir, "exit"), string(rune('0'+exit)))
	manifest := "name = \"" + name + "\"\nwasm = " + fakeplugin.WasmTOML + "\nprotocol = \"0.22\"\n" + extra +
		"\n[[transpiler]]\nname = \"" + name + "\"\nlanguage = \"" + lang + "\"\n" + transpilerExtra
	write(t, filepath.Join(pluginDir, "hozo-plugin.toml"), manifest)
}

// readFile は、ファイルの中身（なければ ""）。
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// launches は、プラグイン name が、これまでに起動された回数（記録は、消さない）。
func launches(t *testing.T, dir, name string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "plugins", name, "launches"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

// request は fakePlugin が受け取った emit の頼みごと（出力とトランスパイラの組ごとに一つ。にせものが、要求の outputs と transpilers を
// ほどいて、記録する）のうち、テストが見る部分。view は、その出力の view（docs/ir.md「まとめた形」の OutputView）。
type request struct {
	Protocol   string `json:"protocol"`
	Direction  string `json:"direction"`
	Transpiler string `json:"transpiler"`
	View       struct {
		Viewer string `json:"viewer"`
		Config struct {
			Index       int                       `json:"index"`
			Language    string                    `json:"language"`
			Base        string                    `json:"base"`
			Out         string                    `json:"out"`
			Dir         string                    `json:"dir"`
			Transpilers []string                  `json:"transpilers"`
			Options     map[string]map[string]any `json:"options"`
			Seams       map[string]string         `json:"seams"`
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

// 単位のテストのプロジェクト（docs/config.md「単位」「単位の対象」「in」）。名前空間 shop は二つのファイルに分かれ、
// 子の名前空間 shop.admin を持つ。
const (
	shopAPI = `/** 店（API の側） */
namespace shop {
  model Item { id: string; }
  model Receipt { item: Item; }
  /** 品物 */
  state items: list<Item>;
  effect fn addItem(item: Item): effects items.post;

  /** 入口。nightly は batch.hozo で宣言した配線 */
  fn open(item: Item): Receipt {
    save(item);
    nightly(item);
    val   r = issue(item);
    return r;
  }
  fn save(item: Item): effects addItem { addItem(item); }
  fn issue(item: Item): Receipt;
}
`
	shopBatch = `/** 店（バッチの側） */
namespace shop {
  model Report { count: int; }
  /** 夜間の片付け */
  fn nightly(item: Item) {
    sweep(item);
  }
  fn sweep(item: Item);
  fn report(): Report;
}

/** 管理 */
namespace shop.admin {
  fn ban(item: shop.Item);
}
`
)

// unitsProject は単位のテストのプロジェクトを一時ディレクトリに作る。単位は三つ:
//
//  1. in = ["shop/api.hozo"]、namespace shop、トランスパイラ api（基底）と rider（api に相乗り。オプション付き）、out は gen/api
//  2. namespace shop（in なし）、トランスパイラ api、out は gen/batch
//  3. namespace shop.admin、トランスパイラ admin（基底）、out は gen/admin。shop.admin は shop.Item を使うので、型の置き場として
//     admin の単位 4（namespace shop、out は gen/adminshop）も置く
//
// shop.admin は 3 が名指ししているので、1 と 2 と 4 の対象から外れる。どのトランスパイラも、出力ごとに、
// Dir(U, N) の下に <トランスパイラの名前>.txt を一つ書く。
func unitsProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "shop", "api.hozo"), shopAPI)
	write(t, filepath.Join(dir, "shop", "batch.hozo"), shopBatch)
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+
		"[[emit]]\nin = [\"shop/api.hozo\"]\nnamespace = \"shop\"\ntranspilers = [\"rider\", \"api\"]\nout = \"gen/api\"\n\n"+
		"[emit.options.rider]\nvalidate = true\n\n"+
		unit("shop", "gen/batch", `"api"`)+unit("shop.admin", "gen/admin", `"admin"`)+unit("shop", "gen/adminshop", `"admin"`))
	fakePlugin(t, dir, "api", "go", "", `{"files":[{"path":"@DIR@/api.txt","content":"@NS@"}],"warnings":[]}`, 0)
	fakeTranspiler(t, dir, "rider", "go", "", "requires = [\"api\"]\nseams = [\"shop.dec\"]\n", `{"files":[{"path":"@DIR@/rider.txt","content":"@NS@"}],"warnings":[]}`, 0)
	fakePlugin(t, dir, "admin", "go", "", `{"files":[{"path":"@DIR@/admin.txt","content":"@NS@"}],"warnings":[]}`, 0)
	return dir
}

// emitAll は dir で hozoc build を走らせ、gen の下に書かれたファイル（gen からの相対パス → 内容）を返す。
func emitAll(t *testing.T, dir string, args ...string) map[string]string {
	t.Helper()
	code, stdout, stderr := run(t, dir, nil, append([]string{"build"}, args...)...)
	if code != 0 {
		t.Fatalf("emit %v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
	}
	return readAll(t, filepath.Join(dir, "gen"))
}

// TestEmitUnits は docs/config.md「単位」「単位の対象」: 同じ名前空間の二つの単位が、それぞれの out に独立に出力する。
// 別の単位が名指しした子孫は、祖先の単位の対象から外れる。トランスパイラへの要求は、起動するトランスパイラの名前と、
// その単位での設定を持つ（docs/plugin-protocol.md「emit」）。
func TestEmitUnits(t *testing.T) {
	dir := unitsProject(t)
	got := emitAll(t, dir)

	t.Run("同じ名前空間の二つの単位は、それぞれの out に独立に出力する", func(t *testing.T) {
		want := map[string]string{
			"api/api.txt": "shop", "api/rider.txt": "shop", "api/.hozoc-outputs.json": "[\n  \"gen/api/api.txt\",\n  \"gen/api/rider.txt\"\n]\n",
			"batch/api.txt": "shop", "batch/.hozoc-outputs.json": "[\n  \"gen/batch/api.txt\"\n]\n",
			"admin/admin.txt": "shop.admin", "admin/.hozoc-outputs.json": "[\n  \"gen/admin/admin.txt\"\n]\n",
			"adminshop/admin.txt": "shop", "adminshop/.hozoc-outputs.json": "[\n  \"gen/adminshop/admin.txt\"\n]\n",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("gen = %v\nwant %v", got, want)
		}
	})

	t.Run("別の単位が名指しした子孫は、祖先の単位の対象から外れる", func(t *testing.T) {
		// トランスパイラ api と rider は shop.admin について起動されず、admin は shop.admin と、自分の単位の shop について起動される
		for name, viewers := range map[string]string{"api": "shop,shop", "rider": "shop", "admin": "shop.admin,shop"} {
			var seen []string
			for _, r := range requests(t, dir, name) {
				seen = append(seen, r.View.Viewer)
			}
			if strings.Join(seen, ",") != viewers {
				t.Errorf("トランスパイラ %s が起動された名前空間 = %v, want %s", name, seen, viewers)
			}
		}
	})

	t.Run("要求は transpiler と、その単位での設定を持つ", func(t *testing.T) {
		emitAll(t, dir)
		for _, c := range []struct {
			name, viewer, dir, out string
			index                  int
			base, transpilers      string
			files                  string
		}{
			{"rider", "shop", "gen/api", "gen/api", 1, "api", "rider,api", "shop/api.hozo"},
			{"api", "shop", "gen/api", "gen/api", 1, "api", "rider,api", "shop/api.hozo"},
			{"admin", "shop.admin", "gen/admin", "gen/admin", 3, "admin", "admin", "shop/api.hozo,shop/batch.hozo"},
		} {
			reqs := requests(t, dir, c.name)
			if len(reqs) == 0 {
				t.Errorf("%s: 要求がない", c.name)
				continue
			}
			r := reqs[0]
			cfg := r.View.Config
			if r.Protocol != "0.22" || r.Direction != "emit" || r.Transpiler != c.name || r.View.Viewer != c.viewer || cfg.Dir != c.dir || cfg.Out != c.out ||
				cfg.Index != c.index || cfg.Language != "go" || cfg.Base != c.base || strings.Join(cfg.Transpilers, ",") != c.transpilers ||
				userFiles(r.View.Files) != c.files || !containsStr(r.View.Files, "hozo:vocab/http.hozo") {
				t.Errorf("%s: 要求 = %+v", c.name, r)
			}
			// オプションと継ぎ目は、単位のすべてのトランスパイラに同じものが渡る（docs/config.md「オプション」「継ぎ目」）
			if c.index == 1 && (cfg.Options["rider"]["validate"] != true || len(cfg.Options) != 1 || cfg.Seams["shop.dec"] != "rider") {
				t.Errorf("%s: options = %v, seams = %v", c.name, cfg.Options, cfg.Seams)
			}
			if c.index == 3 && (len(cfg.Options) != 0 || len(cfg.Seams) != 0) {
				t.Errorf("%s: options = %v, seams = %v", c.name, cfg.Options, cfg.Seams)
			}
		}
	})

	t.Run("起動は、プラグインごとに一回。順は、最初に現れた順", func(t *testing.T) {
		// docs/plugin-protocol.md「起動」: 一度の emit（check と、書き出し）で、プラグインは、一回ずつ。api は、二つの単位の出力を、
		// admin は、二つの出力を受け持つが、起動は一回。順は、単位の設定の順、transpilers に書いた順に見て、最初に現れた順
		// （単位 1 の transpilers は、rider が先）
		// 起動の順は、プラグインが、標準エラーに書いた行（hozoc が、プラグインの名前を頭に付けて流す）の順で見る
		for _, name := range []string{"api", "rider", "admin"} {
			write(t, filepath.Join(dir, "plugins", name, "stderr"), "launched\n")
		}
		code, stdout, stderr := run(t, dir, nil, "build")
		if code != 0 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if stderr != "[rider] launched\n[api] launched\n[admin] launched\n" {
			t.Errorf("起動の順（標準エラー） = %q", stderr)
		}
	})
}

// TestEmitUnitsTargets は docs/cli.md「書き出し」の 1: 引数は名前空間で、その名前空間と子孫を出力するすべての単位の分を
// 書き出す。対象を絞っても、書き出す出力の内容は、すべてを対象にしたときと同じ。プラグインには、対象を絞っても、すべての
// 出力を頼む（check が、すべての出力の診断を要る）: 絞るのは、書き出すファイルだけ。
func TestEmitUnitsTargets(t *testing.T) {
	dir := unitsProject(t)
	all := emitAll(t, dir)
	// すべてを対象にしたときの、頼みごとの数（出力とトランスパイラの組ごとに一つ）
	asked := map[string]int{}
	for _, name := range []string{"api", "rider", "admin"} {
		asked[name] = len(requests(t, dir, name))
	}
	clear := func() {
		if err := os.RemoveAll(filepath.Join(dir, "gen")); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"api", "rider", "admin"} {
			requests(t, dir, name)
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
	// shop.admin: 書き出すのは、単位 3 の出力だけ。プラグインへの頼みごとは、すべてを対象にしたときと同じ（起動は、一回ずつ）
	clear()
	before := map[string]int{}
	for _, name := range []string{"api", "rider", "admin"} {
		before[name] = launches(t, dir, name)
	}
	same(emitAll(t, dir, "shop.admin"), "admin/")
	for _, name := range []string{"api", "rider", "admin"} {
		if n, started := len(requests(t, dir, name)), launches(t, dir, name)-before[name]; n != asked[name] || started != 1 {
			t.Errorf("%s: 頼みごとが %d 個（want %d。すべてを対象にしたときと同じ）、起動が %d 回（want 1）", name, n, asked[name], started)
		}
	}
	// shop: shop とその子孫を出力するすべての単位の分（四つとも）
	clear()
	same(emitAll(t, dir, "shop"), "api/", "batch/", "admin/", "adminshop/")
	// 引数は名前空間で、単位の namespace である必要はない: 単位 3 が名指しする shop.admin の子孫 shop.admin.audit は、
	// 単位 3 の対象。その出力だけを書く（Dir(U, N) は gen/admin/audit）
	write(t, filepath.Join(dir, "shop", "audit.hozo"), "namespace shop.admin.audit {\n  fn review(id: string): boolean;\n}\n")
	clear()
	if got := emitAll(t, dir, "shop.admin.audit"); len(got) != 2 || got["admin/audit/admin.txt"] != "shop.admin.audit" {
		t.Errorf("shop.admin.audit の出力 = %v", got)
	}
	// 引数は仕様にある名前空間でなければならない（単位の対象の名前空間の下の名前でも）。子孫が単位の対象なら、
	// 引数そのものがどの単位の対象でなくてもよい（下の TestEmitArgumentDescendantOnly）
	if code, stdout, stderr := run(t, dir, nil, "build", "shop.typo"); code != exitUsage || stdout != "" || !strings.Contains(stderr, "名前空間 shop.typo も") {
		t.Errorf("仕様にない名前空間: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, _, stderr := run(t, dir, nil, "build", "shop", "nope"); code != exitUsage || !strings.Contains(stderr, "名前空間 nope も") || strings.Contains(stderr, "shop、") {
		t.Errorf("引数の一つが対象でない: code=%d stderr=%q", code, stderr)
	}
	if err := os.Remove(filepath.Join(dir, "shop", "audit.hozo")); err != nil {
		t.Fatal(err)
	}
	// dry-run は書かない
	clear()
	code, stdout, _ := run(t, dir, nil, "build", "--dry-run", "shop.admin")
	if code != 0 || !strings.Contains(stdout, "written: gen/admin/admin.txt\n") || strings.Contains(stdout, "gen/api/") {
		t.Errorf("dry-run: code=%d stdout=%q", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
		t.Error("dry-run が書いた")
	}
}

// TestEmitArgumentDescendantOnly は docs/cli.md「書き出し」の 1（docs/decisions.md 50）: 引数の名前空間そのものがどの単位の
// 対象でなくても、その子孫を対象にする単位があれば、その出力が対象になる（UnknownNamespace ではない）。
// 引数もその子孫もどの単位の対象でもなければ UnknownNamespace。
func TestEmitArgumentDescendantOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "shop.hozo"), "namespace shop {\n  fn browse(): boolean;\n}\n"+
		"namespace shop.admin {\n  fn ban(id: string): boolean;\n}\nnamespace other {\n  fn f();\n}\n")
	// 単位は shop.admin だけ。shop と other は、どの単位の対象でもない
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("shop.admin", "gen/admin", `"admin"`))
	fakePlugin(t, dir, "admin", "go", "", `{"files":[{"path":"@DIR@/admin.txt","content":"@NS@"}],"warnings":[]}`, 0)

	code, stdout, stderr := run(t, dir, nil, "build", "shop")
	if code != 0 || !strings.Contains(stdout, "written: gen/admin/admin.txt\n") {
		t.Fatalf("子孫だけが単位の対象: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	want := readAll(t, filepath.Join(dir, "gen", "admin"))
	if err := os.RemoveAll(filepath.Join(dir, "gen")); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr = run(t, dir, nil, "build"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if got := readAll(t, filepath.Join(dir, "gen", "admin")); !reflect.DeepEqual(got, want) {
		t.Errorf("引数 shop の出力が、すべてを対象にしたときと違う")
	}
	// other は仕様にあるが、どの単位の対象でもない
	if code, stdout, stderr = run(t, dir, nil, "build", "other"); code != exitUsage || stdout != "" || !strings.Contains(stderr, "名前空間 other も") {
		t.Errorf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// TestEmitIn は docs/config.md「in」: in は単位が出力する範囲だけを絞る。View.files は、in に合ったファイル（と語彙のファイル）。
// in に合うファイルに宣言を持たなくなった名前空間は、その単位では出力せず、トランスパイラも起動しない。前回の出力は消える。
func TestEmitIn(t *testing.T) {
	dir := unitsProject(t)
	emitAll(t, dir)
	// 単位 1（in = shop/api.hozo）と単位 2（in なし）の要求の files
	reqs := requests(t, dir, "api")
	if len(reqs) != 2 || userFiles(reqs[0].View.Files) != "shop/api.hozo" || userFiles(reqs[1].View.Files) != "shop/api.hozo,shop/batch.hozo" {
		t.Fatalf("api の要求の files = %+v", reqs)
	}
	requests(t, dir, "rider")
	requests(t, dir, "admin")

	write(t, filepath.Join(dir, "shop", "extra.hozo"), "namespace shop.extra {\n  fn only(): string;\n}\n")
	fakePlugin(t, dir, "extra", "go", "", `{"files":[{"path":"@DIR@/e.txt","content":"e"}],"warnings":[]}`, 0)
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("shop.extra", "gen/x/extra", `"extra"`))
	if got := emitAll(t, dir); got["x/extra/e.txt"] != "e" {
		t.Fatalf("gen/x/extra/e.txt がない: %v", got)
	}
	// in に合うファイルに宣言を持たなくなった名前空間は、その単位では出力しない。前回の出力は消える（docs/cli.md「書き出し」の 5）
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+
		"[[emit]]\nin = [\"shop/api.hozo\"]\nnamespace = \"shop.extra\"\ntranspilers = [\"extra\"]\nout = \"gen/x/extra\"\n")
	code, stdout, stderr := run(t, dir, nil, "build")
	if code != 0 || stdout != "removed: gen/x/extra/e.txt\n" {
		t.Fatalf("in で出力でなくなった名前空間: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen", "x", "extra", OutputsFile)); !os.IsNotExist(err) {
		t.Error("書くものがない出力の記録が残っている")
	}
	if reqs := requests(t, dir, "extra"); len(reqs) != 1 { // 最初の emit の一回だけ。出力でなくなってからは起動しない
		t.Errorf("トランスパイラ extra が %d 回起動された（want 1）", len(reqs))
	}
}

// TestRunEmit は cli.hozo の op runEmit（hozoc.Leaves のメソッド。docs/cli.md「手で書くもの」）: plugin を起動し、req を JSON に
// して送り、応答を hozoc.EmitResponse に読んで返す。内部の op なので、内側のパッケージの Ops から呼ぶ。動かすのは、
// plugin.wasm の WASM。hozoc.Spec と hozoc.OutputView は中身のない model なので、要求の spec と、
// 出力の view は空のオブジェクト。異常終了は proc.Crashed、形の違う応答は proc.MalformedOutput（どちらも internal/gen/proc の型）。
func TestRunEmit(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "plugins", "echo")
	// 受け取った要求（raw.jsonl）を記録して、頼まれたトランスパイラごとに、決まった答えを返す
	fakeplugin.Install(t, dir)
	write(t, filepath.Join(dir, "response.json"), `{"files":[{"path":"gen/a/@T@.go","content":"package a\n"}],"warnings":[{"at":"a.f","message":"w"}],"errors":[{"code":"X-1","at":"a.g","message":"e"}]}`)
	write(t, filepath.Join(dir, "exit"), "0")
	plugin := hozoc.Plugin{Name: "echo", Wasm: hozoc.Path(filepath.Join(dir, fakeplugin.WasmFile))}
	// 出力が二つ: 一つ目は、トランスパイラを二つ、二つ目は、一つ
	req := hozoc.EmitRequest{Protocol: "0.22", Direction: "emit", Outputs: []hozoc.EmitOutput{
		{Transpilers: []string{"echo", "echo2"}}, {Transpilers: []string{"echo"}}}}
	ctx := context.Background()
	a := &App{Stderr: &bytes.Buffer{}}
	ops := innerOps(a)

	resp, err := ops.RunEmit(ctx, plugin, req)
	if err != nil {
		t.Fatal(err)
	}
	result := func(name string) hozoc.TranspilerResult {
		return hozoc.TranspilerResult{
			Files:    []hozoc.OutputFile{{Path: hozoc.Path("gen/a/" + name + ".go"), Content: "package a\n"}},
			Warnings: []hozoc.PluginWarning{{At: "a.f", Message: "w"}},
			Errors:   []hozoc.PluginError{{Code: "X-1", At: "a.g", Message: "e"}},
		}
	}
	want := hozoc.EmitResponse{Outputs: []hozoc.OutputResult{
		{Results: []hozoc.TranspilerResult{result("echo"), result("echo2")}},
		{Results: []hozoc.TranspilerResult{result("echo")}},
	}}
	if !reflect.DeepEqual(resp, want) {
		t.Errorf("応答 = %+v, want %+v", resp, want)
	}
	sent, err := os.ReadFile(filepath.Join(dir, "raw.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(sent)); got != `{"protocol":"0.22","direction":"emit","spec":{},"outputs":[{"transpilers":["echo","echo2"],"view":{}},{"transpilers":["echo"],"view":{}}]}` {
		t.Errorf("送った要求 = %s", got)
	}

	// 異常終了は proc.Crashed
	write(t, filepath.Join(dir, "exit"), "3")
	var crashed proc.Crashed
	if _, err = ops.RunEmit(ctx, plugin, req); !errors.As(err, &crashed) {
		t.Errorf("異常終了: err = %v, want proc.Crashed", err)
	}
	// ない WASM も、空の wasm も proc.Crashed
	for _, wasm := range []hozoc.Path{hozoc.Path(filepath.Join(root, "no-such.wasm")), ""} {
		broken := plugin
		broken.Wasm = wasm
		if _, err = ops.RunEmit(ctx, broken, req); !errors.As(err, &crashed) {
			t.Errorf("wasm %q: err = %v, want proc.Crashed", wasm, err)
		}
	}
	// 形の違う応答（必須の files がない）と、JSON でない応答は proc.MalformedOutput
	write(t, filepath.Join(dir, "exit"), "0")
	var malformed proc.MalformedOutput
	for _, body := range []string{`{"warnings":[]}`, `not json`} {
		write(t, filepath.Join(dir, "response.json"), body)
		if _, err = ops.RunEmit(ctx, plugin, req); !errors.As(err, &malformed) {
			t.Errorf("応答 %s: err = %v, want proc.MalformedOutput", body, err)
		}
	}
	// 中断された ctx では、起動しない
	before := strings.Count(readFile(t, filepath.Join(dir, "launches")), "\n")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = ops.RunEmit(cancelled, plugin, req); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx: err = %v", err)
	}
	if after := strings.Count(readFile(t, filepath.Join(dir, "launches")), "\n"); after != before {
		t.Error("中断された ctx で、プラグインを起動した")
	}
}

// TestStateUnits は、cli.hozo の状態 sources、config、outputs の単位の、形で決まる問い合わせ（IsEmpty と Get。docs/cli.md
// 「手で書くもの」の表）: 作業ディレクトリから探した設定で、ディスクから読む。outputs は、出力先ごとの記録に載っているファイルで、
// 効果 syncOutputs（emit）の結果を判定できる。
func TestStateUnits(t *testing.T) {
	dir := copyCorpus(t)
	ctx := context.Background()
	a := &App{Dir: dir, Stderr: &bytes.Buffer{}}

	// sources: 設定の sources の glob に合うファイル（辞書順。語彙のファイルは含まない）
	sources, err := a.Sources().Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range sources {
		names = append(names, string(f.Path))
		if disk, err := os.ReadFile(filepath.Join(dir, string(f.Path))); err != nil || string(disk) != f.Content {
			t.Errorf("sources の %s の内容が、ディスクと違う（%v）", f.Path, err)
		}
	}
	var want []string
	for _, f := range listFiles(t, dir) {
		if strings.HasSuffix(f, ".hozo") && !strings.HasPrefix(f, "plugins/") {
			want = append(want, f)
		}
	}
	sort.Strings(want)
	if len(want) == 0 || !reflect.DeepEqual(names, want) {
		t.Errorf("sources = %v, want %v", names, want)
	}
	if empty, err := a.Sources().IsEmpty(ctx); err != nil || empty {
		t.Errorf("sources.IsEmpty = %v, %v", empty, err)
	}

	// config: hozo.toml
	cfg, err := a.Config().Get(ctx)
	disk, _ := os.ReadFile(filepath.Join(dir, "hozo.toml"))
	if err != nil || cfg == nil || cfg.Path != "hozo.toml" || cfg.Content != string(disk) {
		t.Errorf("config = %+v, %v", cfg, err)
	}
	if empty, err := a.Config().IsEmpty(ctx); err != nil || empty {
		t.Errorf("config.IsEmpty = %v, %v", empty, err)
	}

	// outputs: emit の前は空。emit の後は、書き出したファイル（記録に載っているもの）
	if empty, err := a.Outputs().IsEmpty(ctx); err != nil || !empty {
		t.Errorf("emit の前の outputs.IsEmpty = %v, %v", empty, err)
	}
	res, err := a.Ops().Build(ctx, hozoc.BuildInput{})
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := a.Outputs().Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range outputs {
		got[string(f.Path)] = f.Content
	}
	if len(got) != len(res.Written) || len(got) == 0 {
		t.Errorf("outputs = %d 個, want emit が書いた %d 個", len(got), len(res.Written))
	}
	for _, p := range res.Written {
		disk, err := os.ReadFile(filepath.Join(dir, string(p)))
		if content, ok := got[string(p)]; !ok || err != nil || content != string(disk) {
			t.Errorf("outputs に %s がないか、内容がディスクと違う（%v）", p, err)
		}
	}
	// 記録にあってもディスクにないファイルは、返さない
	if err := os.Remove(filepath.Join(dir, "gen", "server", "out.txt")); err != nil {
		t.Fatal(err)
	}
	if outputs, err = a.Outputs().Get(ctx); err != nil || len(outputs) != len(res.Written)-1 {
		t.Errorf("一つ消した後の outputs = %d 個, %v", len(outputs), err)
	}

	// 設定なし: config は空（nil）、outputs も空。sources は既定の glob
	bare := t.TempDir()
	write(t, filepath.Join(bare, "a.hozo"), "namespace a { fn f(); }\n")
	b := &App{Dir: bare, Stderr: &bytes.Buffer{}}
	if cfg, err := b.Config().Get(ctx); err != nil || cfg != nil {
		t.Errorf("設定なしの config = %+v, %v", cfg, err)
	}
	if empty, err := b.Config().IsEmpty(ctx); err != nil || !empty {
		t.Errorf("設定なしの config.IsEmpty = %v, %v", empty, err)
	}
	if sources, err := b.Sources().Get(ctx); err != nil || len(sources) != 1 || sources[0].Path != "a.hozo" {
		t.Errorf("設定なしの sources = %+v, %v", sources, err)
	}
	if empty, err := b.Outputs().IsEmpty(ctx); err != nil || !empty {
		t.Errorf("設定なしの outputs.IsEmpty = %v, %v", empty, err)
	}
}

// TestEmitAsksEachPluginOnce は docs/plugin-protocol.md「起動」: 一度の実行で、プラグインごとに、多くて一回、起動する。hozoc build は、
// 検査（check）と、書き出し（collectOutputs）で、同じ答えを使い、同じプラグインに、二度、頼まない。プラグインが受け持つ出力は、
// 一つの要求に並ぶ。
func TestEmitAsksEachPluginOnce(t *testing.T) {
	dir := unitsProject(t)
	for _, c := range []struct {
		args []string
	}{{[]string{"build", "--no-emit"}}, {[]string{"build"}}, {[]string{"build", "--dry-run"}}} {
		before := map[string]int{}
		for _, name := range []string{"api", "rider", "admin"} {
			before[name] = launches(t, dir, name)
			requests(t, dir, name)
		}
		if code, stdout, stderr := run(t, dir, nil, c.args...); code != 0 {
			t.Fatalf("%v: code=%d stdout=%q stderr=%q", c.args, code, stdout, stderr)
		}
		for name, outputs := range map[string]int{"api": 2, "rider": 1, "admin": 2} {
			if started := launches(t, dir, name) - before[name]; started != 1 {
				t.Errorf("%v: %s の起動が %d 回（want 1）", c.args, name, started)
			}
			if asked := len(requests(t, dir, name)); asked != outputs {
				t.Errorf("%v: %s への頼みごとが %d 個（want %d: 受け持つ出力ごとに一つ）", c.args, name, asked, outputs)
			}
		}
	}
	// 一つの要求に、仕様は一度: 要求の、いちばん上の項目は、protocol、direction、spec、outputs
	raw := strings.Split(strings.TrimSpace(readFile(t, filepath.Join(dir, "plugins", "api", "raw.jsonl"))), "\n")
	var req map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw[len(raw)-1]), &req); err != nil {
		t.Fatal(err)
	}
	var outputs []map[string]json.RawMessage
	if err := json.Unmarshal(req["outputs"], &outputs); err != nil || len(outputs) != 2 || len(req) != 4 || req["spec"] == nil {
		t.Fatalf("api への要求の項目 = %v, outputs = %d 個 (%v)", len(req), len(outputs), err)
	}
	var spec map[string]json.RawMessage
	_ = json.Unmarshal(req["spec"], &spec)
	if spec["program"] == nil || spec["namespaces"] == nil || string(spec["version"]) != `"0.22"` {
		t.Errorf("spec の項目が足りない: %v", len(spec))
	}
	for i, o := range outputs {
		var view map[string]json.RawMessage
		_ = json.Unmarshal(o["view"], &view)
		if view["program"] != nil || view["namespaces"] != nil || view["viewer"] == nil || view["config"] == nil || view["ops"] == nil {
			t.Errorf("outputs[%d].view が、出力ごとの部分だけでない", i)
		}
	}
}

// TestRenamedCommands は docs/cli.md「build」の「古い名前」: hozoc check と hozoc emit は、なくした。その名前で呼ばれたら、何もせずに、
// 新しい書き方（残りの引数を付けたもの）を、標準エラーに示して、使い方の誤りの終了コード（64）で終わる。
func TestRenamedCommands(t *testing.T) {
	dir := unitsProject(t)
	for _, c := range []struct {
		args []string
		now  string
	}{
		{[]string{"check"}, "hozoc build --no-emit を使う"},
		{[]string{"check", "--format", "json"}, "hozoc build --no-emit --format json を使う"},
		{[]string{"emit"}, "hozoc build を使う"},
		{[]string{"emit", "--dry-run", "shop.admin"}, "hozoc build --dry-run shop.admin を使う"},
	} {
		code, stdout, stderr := run(t, dir, nil, c.args...)
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "コマンド "+c.args[0]+" は、なくなった") || !strings.Contains(stderr, c.now) {
			t.Errorf("%v: code=%d stdout=%q stderr=%q（want 64 と、%q）", c.args, code, stdout, stderr, c.now)
		}
	}
	// 何もしていない: プラグインは、起動せず、ファイルも、書いていない
	for _, name := range []string{"api", "rider", "admin"} {
		if n := launches(t, dir, name); n != 0 {
			t.Errorf("古い名前のコマンドで、プラグイン %s を起動した（%d 回）", name, n)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
		t.Error("古い名前のコマンドで、ファイルを書いた")
	}
	// 新しい名前の引数（名前空間 check、emit）は、古い名前ではない
	if msg, renamed := Renamed([]string{"build", "check"}); renamed || msg != "" {
		t.Errorf("Renamed(build check) = %q, %v", msg, renamed)
	}
	if _, renamed := Renamed(nil); renamed {
		t.Error("Renamed(nil) が、古い名前と言った")
	}
}

// TestBuildNoEmit は docs/cli.md「build」: hozoc build --no-emit は、検査だけ。ファイルには、触れない（書かず、消さず、置き場も
// 読まない）が、ファイルを集めて、path と衝突を確かめるところ（「書き出し」の 1〜3）までは、行う。表示は、検査の結果だけ。
func TestBuildNoEmit(t *testing.T) {
	dir := unitsProject(t)
	// 先に、ふつうに書き出して、置き場を作る
	all := emitAll(t, dir)
	if len(all) == 0 {
		t.Fatal("書き出したファイルがない")
	}
	gen := filepath.Join(dir, "gen")
	// 置き場に、前回書いて、今回は書かないはずのファイルを、記録ごと足す（build なら、消す）
	stale := filepath.Join(gen, "admin", "stale.txt")
	write(t, stale, "old")
	record := filepath.Join(gen, "admin", OutputsFile)
	write(t, record, strings.Replace(readFile(t, record), "[", "[\n  \"gen/admin/stale.txt\",", 1))
	before, beforeTimes := readAll(t, gen), mtimes(t, gen)

	for _, args := range [][]string{{"build", "--no-emit"}, {"build", "--no-emit", "--dry-run"}, {"build", "--no-emit", "shop.admin"}} {
		code, stdout, stderr := run(t, dir, nil, args...)
		// text の表示は、検査の結果だけ（warning と、数の行）: written も removed も、出ない
		if code != 0 || stderr != "" || strings.Contains(stdout, "written:") || strings.Contains(stdout, "removed:") || !strings.HasSuffix(stdout, "0 errors, 0 warnings\n") {
			t.Errorf("%v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
		if !reflect.DeepEqual(readAll(t, gen), before) || !reflect.DeepEqual(mtimes(t, gen), beforeTimes) {
			t.Fatalf("%v が、置き場に触れた", args)
		}
	}
	// json は、warnings だけ
	code, stdout, _ := run(t, dir, nil, "build", "--no-emit", "--format=json")
	var shown map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &shown); err != nil || code != 0 || len(shown) != 1 || shown["warnings"] == nil {
		t.Errorf("json: code=%d stdout=%q (%v)", code, stdout, err)
	}
	// --dry-run だけなら、書き出す予定を出す（--no-emit との違い）
	if code, stdout, _ = run(t, dir, nil, "build", "--dry-run"); code != 0 || !strings.Contains(stdout, "written: gen/admin/admin.txt\n") {
		t.Errorf("dry-run: code=%d stdout=%q", code, stdout)
	}
	// --no-emit のない build は、古いファイルを消す
	if code, stdout, _ = run(t, dir, nil, "build"); code != 0 || !strings.Contains(stdout, "removed: gen/admin/stale.txt\n") {
		t.Errorf("build: code=%d stdout=%q", code, stdout)
	}

	// 集めて確かめるところは、--no-emit でも、行う: 出力先の外の path は PLG-1（proc.MalformedOutput。70）
	write(t, filepath.Join(dir, "plugins", "rider", "response.json"), `{"files":[{"path":"elsewhere/x.txt","content":"x"}],"warnings":[]}`)
	if code, _, stderr := run(t, dir, nil, "build", "--no-emit"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "PLG-1") {
		t.Errorf("PLG-1: code=%d stderr=%q", code, stderr)
	}
	// 名前空間の引数は、確かめる範囲を選ぶ: rider の出力（単位 1 の shop）を含まない範囲なら、通る
	if code, _, stderr := run(t, dir, nil, "build", "--no-emit", "shop.admin"); code != 0 {
		t.Errorf("範囲の外の PLG-1 を、見た: code=%d stderr=%q", code, stderr)
	}
}

// TestBuildNoEmitWithoutConfig は docs/cli.md「build」の「hozo.toml がないとき」: build --no-emit は、設定なしで検査する。build は、
// 書き出す単位がないので、CFG-7。
func TestBuildNoEmitWithoutConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	if code, stdout, stderr := run(t, dir, nil, "build", "--no-emit"); code != 0 || stdout != "0 errors, 0 warnings\n" || stderr != "" {
		t.Errorf("build --no-emit: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, _, stderr := run(t, dir, nil, "build"); code != exitConfigInvalid || !strings.Contains(stderr, "hozo.toml: error[CFG-7]") {
		t.Errorf("build: code=%d stderr=%q", code, stderr)
	}
}
