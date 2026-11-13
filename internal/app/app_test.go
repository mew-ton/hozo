package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
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

// warnFile は copyCorpus が足すファイル。設計書のコーパスには warning がない（§7.3 の placeOrder を配線に直したので
// REL-3 がなくなった。tests/corpus/design/README.md の表の 14）ので、warning の表示と受け渡し（check の text / json、
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

// corpusConfig は copyCorpus が置く hozo.toml。serverPlugins は [server] の plugins の中身（`"pick-server"` など）。
func corpusConfig(serverPlugins string) string {
	return "[hozo]\nplugin_dirs = [\"plugins\"]\n\n[server]\nlanguage = \"go\"\nout = \"gen/server\"\nplugins = [" + serverPlugins + "]\n\n" +
		"[browser]\nlanguage = \"go\"\nout = \"gen/browser\"\nplugins = [\"pick-browser\"]\n"
}

// copyCorpus は設計書のコーパスの .hozo を一時ディレクトリに写し、warning を一つ出す warn.hozo（warnFile）を足し、
// 既定の出力のために go.mod を置く（GOEMIT-1）。hozo.toml は corpusConfig で、名前空間ごとに、起点を選ぶだけの
// プラグイン（pick-server / pick-browser。sh で動く fakePlugin）を設定する。
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
	fakePlugin(t, dir, "pick-server", "go", "[emit]\n", `{"files":[],"warnings":[],`+serverSelect+`}`, 0)
	fakePlugin(t, dir, "pick-browser", "go", "[emit]\n", `{"files":[],"warnings":[],`+browserSelect+`}`, 0)
	return dir
}

// run は dir を作業ディレクトリにして、cmd/hozoc/main.go と同じ組み立て（App の Ops と Presenter を
// 生成した cli.Main に渡す）で hozoc を走らせる。
func run(t *testing.T, dir string, env map[string]string, args ...string) (code int, stdout, stderr string) {
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
	a := &App{Stderr: &errOut, Stdin: strings.NewReader("")} // lsp は入力の終わりで戻る
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
	// 設定の誤り（CFG-1: テーブルに language だけ）は 2
	write(t, filepath.Join(dir, "hozo.toml"), "[a]\nlanguage = \"go\"\n")
	code, _, stderr := run(t, dir, nil, "check")
	if code != exitConfigInvalid || !strings.Contains(stderr, "error[CFG-1]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// HOZO_CONFIG と --config
	write(t, filepath.Join(dir, "alt.toml"), "[a]\nlanguage = \"go\"\nout = \"gen\"\n")
	write(t, filepath.Join(dir, "go.mod"), "module example.com/a\n")
	if code, _, stderr := run(t, dir, map[string]string{"HOZO_CONFIG": "alt.toml"}, "check"); code != 0 {
		t.Fatalf("HOZO_CONFIG: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := run(t, dir, map[string]string{"HOZO_CONFIG": "alt.toml"}, "check", "--config", "hozo.toml"); code != exitConfigInvalid {
		t.Fatalf("--config が HOZO_CONFIG より優先されていない: code=%d stderr=%q", code, stderr)
	}
}

func TestCheckLanguageMismatch(t *testing.T) {
	dir := copyCorpus(t)
	fakePlugin(t, dir, "tsp", "ts", "[emit]\n", `{"files":[],"warnings":[]}`, 0)
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"tsp"`))
	code, _, stderr := run(t, dir, nil, "check")
	if code != exitConfigInvalid || !strings.Contains(stderr, "error[CFG-4]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	l := &App{Dir: dir}
	_, err := l.Check(hozoc.CheckInput{})
	var lm hozoc.LanguageMismatch
	if !errors.As(err, &lm) || len(lm.Value.Items) == 0 {
		t.Fatalf("err = %v", err)
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

// TestEmitStaleAndConflict はプラグインの出力の書き出し、設定から外したあとの古いファイルの削除、CFG-5、PLG-*、proc.Crashed。
func TestEmitStaleAndConflict(t *testing.T) {
	dir := copyCorpus(t)
	emitTable := "[emit]\n"
	// fake は server の二つ目のプラグイン。起点は選ばず、自分のファイルだけを書く
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"pick-server", "fake"`))

	// プラグインが gen/server/extra.txt を書く（warning も一つ）
	fakePlugin(t, dir, "fake", "go", emitTable, `{"files":[{"path":"gen/server/extra.txt","content":"hi"}],"warnings":[{"at":"server.placeOrder","message":"careful"}]}`, 0)
	code, stdout, stderr := run(t, dir, nil, "emit")
	if code != 0 || !strings.Contains(stdout, "written: gen/server/extra.txt\n") || !strings.Contains(stdout, "warning[PLUGIN]: fake: careful") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "gen", "server", "extra.txt")); err != nil || string(b) != "hi" {
		t.Fatalf("extra.txt: %q %v", b, err)
	}

	// プラグインを設定から外すと extra.txt は古いファイルとして消える。既定の出力は残る
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
	fakePlugin(t, dir, "fake", "go", emitTable, `{"files":[{"path":"gen/server/types.go","content":"x"}],"warnings":[]}`, 0)
	before := mtimes(t, filepath.Join(dir, "gen"))
	code, stdout, stderr = run(t, dir, nil, "emit")
	if code != exitOutputConflict || stdout != "" || !strings.Contains(stderr, "gen/server/types.go: error[CFG-5]") || !strings.Contains(stderr, "プラグイン fake") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for f, m := range mtimes(t, filepath.Join(dir, "gen")) {
		if before[f] != m {
			t.Fatalf("%s が書き換えられた", f)
		}
	}

	// PLG-1 → proc.MalformedOutput → 70
	fakePlugin(t, dir, "fake", "go", emitTable, `{"files":[{"path":"../x.go","content":"x"}],"warnings":[]}`, 0)
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "PLG-1") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// PLG-2: 同じ応答の中で同じ path が二度 → proc.MalformedOutput → 70
	fakePlugin(t, dir, "fake", "go", emitTable, `{"files":[{"path":"gen/server/a.txt","content":"1"},{"path":"gen/server/./a.txt","content":"2"}],"warnings":[]}`, 0)
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "PLG-2") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// errors → CheckFailed（1）
	fakePlugin(t, dir, "fake", "go", emitTable, `{"files":[],"warnings":[],"errors":[{"code":"FAKE-1","at":"server.placeOrder","message":"nope"}]}`, 0)
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitCheckFailed || !strings.Contains(stderr, "server.hozo:244:3: error[FAKE-1]: nope") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 異常終了 → 70
	fakePlugin(t, dir, "fake", "go", emitTable, ``, 3)
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "hozoc: ") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

// TestEmitSelectInvalid は PLG-3（docs/plugin-protocol.md「選択」「診断」）: emit の応答の select が、対象の名前空間にない
// 宣言や、付いていないデコレータを指していれば proc.MalformedOutput（70）。何も書かない。
// 条件の一つずつは internal/plugin の TestCheckSelect が確かめる。
func TestEmitSelectInvalid(t *testing.T) {
	dir := copyCorpus(t)
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"pick-server", "fake"`))
	cases := map[string]string{
		"view.viewer の名前空間にない FQN":  `"select":{"roots":["browser.openOrders"]}`, // fake は server のプラグイン
		"存在しない FQN":                 `"select":{"roots":["server.nope"]}`,
		"stops が op でない":            `"select":{"stops":["server.User"]}`,
		"states が別の名前空間の状態":         `"select":{"states":["browser.token"]}`,
		"wraps がその op に付いていないデコレータ": `"select":{"wraps":[{"op":"server.placeOrder","decorator":"http.post"}]}`,
	}
	for name, sel := range cases {
		t.Run(name, func(t *testing.T) {
			fakePlugin(t, dir, "fake", "go", "[emit]\n", `{"files":[{"path":"gen/server/extra.txt","content":"hi"}],"warnings":[],`+sel+`}`, 0)
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
	fakePlugin(t, dir, "fake", "go", "[emit]\n", `{"files":[],"warnings":[],"select":{"wraps":[{"op":"server.login","decorator":"server.validated"}]}}`, 0)
	code, _, stderr := run(t, dir, nil, "emit")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	ops, err := os.ReadFile(filepath.Join(dir, "gen", "server", "ops.go"))
	if err != nil || !strings.Contains(string(ops), "type LoginValidatedWrapper func(in LoginInput, next LoginFunc) (LoginOk, error)") {
		t.Fatalf("包む関数がない: %v\n%s", err, ops)
	}
}

// TestEmitWithoutPlugin は docs/emit-go.md「出力の対象」: 名前空間にプラグインが一つも設定されていなければ（設定されていても
// 何も選ばなければ）、既定の出力は何も書かない。前回の出力は古いファイルとして消え、.hozoc-outputs.json も残らない
// （docs/cli.md「emit」の 6 と 8）。エラーでも warning でもない。
func TestEmitWithoutPlugin(t *testing.T) {
	dir := copyCorpus(t)
	gen := filepath.Join(dir, "gen")
	if code, _, stderr := run(t, dir, nil, "emit"); code != 0 {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	if files := listFiles(t, gen); !containsStr(files, "server/types.go") || !containsStr(files, "server/.hozoc-outputs.json") || !containsStr(files, "browser/ops.go") {
		t.Fatalf("files = %v", files)
	}

	// server からプラグインを外す。server は何も出力しなくなる
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(``))
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

	// 設定されていても、何も選ばないプラグイン（select を返さない）なら同じ。そのプラグインのファイルだけが出る
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(`"fake"`))
	fakePlugin(t, dir, "fake", "go", "[emit]\n", `{"files":[{"path":"gen/server/only.txt","content":"x"}],"warnings":[]}`, 0)
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
}

// TestEmitTargetUsesOtherNamespace は docs/cli.md「emit」の 3: 対象の名前空間の出力が、対象でない Go の名前空間の型を使うとき、
// その名前空間のプラグインも起動して select を使う。対象を絞っても、すべてを対象にしたときと同じ内容が出る。
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
	// server の型は import している（server のプラグインの select から、server の出力がその型を持つと分かる）
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
	// 余分（前回書いて、今回は書かないファイルが残っている）: server のプラグインを外す
	write(t, filepath.Join(dir, "hozo.toml"), corpusConfig(``))
	b := &App{Stderr: &bytes.Buffer{}} // 設定を読み直すために新しい App（直前の Check の結果を持たない）
	ops = b.Ops()
	if got := sync(); got != hozoc.OutputsSyncDefault {
		t.Errorf("プラグインを外した後: %v", got)
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

func TestEmitUnknownNamespace(t *testing.T) {
	dir := copyCorpus(t)
	code, stdout, stderr := run(t, dir, nil, "emit", "nope")
	if code != exitUsage || stdout != "" || !strings.Contains(stderr, "名前空間 nope は hozo.toml にない") {
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

// fakePlugin はシェルで動く小さなプラグインを plugins/<name> に置く。
func fakePlugin(t *testing.T, dir, name, lang, extra, stdout string, exit int) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh がない")
	}
	script := "cat >/dev/null; printf '%s' '" + stdout + "'; exit " + string(rune('0'+exit))
	manifest := "name = \"" + name + "\"\ncommand = [\"sh\", \"-c\", \"" + strings.ReplaceAll(script, `"`, `\"`) + "\"]\nlanguage = \"" + lang + "\"\nprotocol = \"0.1\"\n" + extra
	write(t, filepath.Join(dir, "plugins", name, "hozo-plugin.toml"), manifest)
}
