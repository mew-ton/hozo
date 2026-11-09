package app

import (
	"bytes"
	"context"
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

// copyCorpus は設計書のコーパスを一時ディレクトリに写し、既定の出力のために go.mod を置く（GOEMIT-1）。
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
	write(t, filepath.Join(dir, "go.mod"), "module example.com/design\n\ngo 1.24\n")
	return dir
}

// run は dir を作業ディレクトリにして、cmd/hozoc/main.go と同じ組み立て（hozoc.New(Leaves) と Presenter を
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
	ops := hozoc.New(&Leaves{Stderr: &errOut, Stdin: strings.NewReader("")}) // lsp は入力の終わりで戻る
	code = cli.Main(ops, Presenter{}, args, func(k string) string { return env[k] }, &out, &errOut)
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

func TestCheckMissingConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a { op f(); }\n")
	code, _, stderr := run(t, dir, nil, "check")
	if code != exitConfigInvalid || !strings.Contains(stderr, "hozo.toml: error[CFG-7]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 設定の誤り（CFG-1）も 2
	write(t, filepath.Join(dir, "hozo.toml"), "[a]\nlanguage = \"go\"\n")
	code, _, stderr = run(t, dir, nil, "check")
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
	fakePlugin(t, dir, "tsp", "ts", "[emit]\nentry = [\"cli.command\"]\n", `{"files":[],"warnings":[]}`, 0)
	write(t, filepath.Join(dir, "hozo.toml"), "[hozo]\nplugin_dirs = [\"plugins\"]\n\n[server]\nlanguage = \"go\"\nout = \"gen/server\"\nplugins = [\"tsp\"]\n\n[browser]\nlanguage = \"go\"\nout = \"gen/browser\"\n")
	code, _, stderr := run(t, dir, nil, "check")
	if code != exitConfigInvalid || !strings.Contains(stderr, "error[CFG-4]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	l := &Leaves{Dir: dir}
	_, err := l.Check(context.Background(), hozoc.CheckInput{})
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

// TestEmitStaleAndConflict はプラグインの出力の書き出し、設定から外したあとの古いファイルの削除、CFG-5、PLG-1、proc.Crashed。
func TestEmitStaleAndConflict(t *testing.T) {
	dir := copyCorpus(t)
	withPlugin := "[hozo]\nplugin_dirs = [\"plugins\"]\n\n[server]\nlanguage = \"go\"\nout = \"gen/server\"\nplugins = [\"fake\"]\n\n[browser]\nlanguage = \"go\"\nout = \"gen/browser\"\n"
	withoutPlugin := "[hozo]\nplugin_dirs = [\"plugins\"]\n\n[server]\nlanguage = \"go\"\nout = \"gen/server\"\n\n[browser]\nlanguage = \"go\"\nout = \"gen/browser\"\n"
	emitTable := "[emit]\nentry = [\"cli.command\"]\n"
	write(t, filepath.Join(dir, "hozo.toml"), withPlugin)

	// プラグインが gen/server/extra.txt を書く（warning も一つ）
	fakePlugin(t, dir, "fake", "go", emitTable, `{"files":[{"path":"gen/server/extra.txt","content":"hi"}],"warnings":[{"at":"server.placeOrder","message":"careful"}]}`, 0)
	code, stdout, stderr := run(t, dir, nil, "emit")
	if code != 0 || !strings.Contains(stdout, "written: gen/server/extra.txt\n") || !strings.Contains(stdout, "warning[PLUGIN]: fake: careful") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "gen", "server", "extra.txt")); err != nil || string(b) != "hi" {
		t.Fatalf("extra.txt: %q %v", b, err)
	}

	// プラグインを設定から外すと extra.txt は古いファイルとして消える
	write(t, filepath.Join(dir, "hozo.toml"), withoutPlugin)
	code, stdout, stderr = run(t, dir, nil, "emit")
	if code != 0 || !strings.Contains(stdout, "removed: gen/server/extra.txt\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen", "server", "extra.txt")); !os.IsNotExist(err) {
		t.Fatal("extra.txt が残っている")
	}

	// CFG-5: 既定の出力と同じパスに書こうとする → 3、何も書かない
	write(t, filepath.Join(dir, "hozo.toml"), withPlugin)
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
	// errors → CheckFailed（1）
	fakePlugin(t, dir, "fake", "go", emitTable, `{"files":[],"warnings":[],"errors":[{"code":"FAKE-1","at":"server.placeOrder","message":"nope"}]}`, 0)
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitCheckFailed || !strings.Contains(stderr, "server.hozo:230:3: error[FAKE-1]: nope") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 異常終了 → 70
	fakePlugin(t, dir, "fake", "go", emitTable, ``, 3)
	if code, _, stderr = run(t, dir, nil, "emit"); code != exitSoftwareOrCrash || !strings.Contains(stderr, "hozoc: ") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
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
