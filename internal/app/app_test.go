package app

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "ゴールデンを更新する")

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

// run は dir を作業ディレクトリにして Main を呼ぶ。
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
	code = Main(args, func(k string) string { return env[k] }, &out, &errOut)
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
	if code != ExitCheckFailed || stdout != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "bad.hozo:2:") || !strings.Contains(stderr, "error[RES-1]") || !strings.Contains(stderr, "1 error, 1 warning") {
		t.Fatalf("stderr=%q", stderr)
	}
	code, stdout, stderr = run(t, dir, nil, "check", "--format", "json")
	if code != ExitCheckFailed {
		t.Fatalf("code=%d", code)
	}
	var f struct {
		Error string `json:"error"`
		Value struct {
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
}

func TestCheckMissingConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.hozo"), "namespace a { op f(); }\n")
	code, _, stderr := run(t, dir, nil, "check")
	if code != ExitConfigInvalid || !strings.Contains(stderr, "hozo.toml: error[CFG-7]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 設定の誤り（CFG-1）も 2
	write(t, filepath.Join(dir, "hozo.toml"), "[a]\nlanguage = \"go\"\n")
	code, _, stderr = run(t, dir, nil, "check")
	if code != ExitConfigInvalid || !strings.Contains(stderr, "error[CFG-1]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// HOZO_CONFIG と --config
	write(t, filepath.Join(dir, "alt.toml"), "[a]\nlanguage = \"go\"\nout = \"gen\"\n")
	write(t, filepath.Join(dir, "go.mod"), "module example.com/a\n")
	if code, _, stderr := run(t, dir, map[string]string{"HOZO_CONFIG": "alt.toml"}, "check"); code != 0 {
		t.Fatalf("HOZO_CONFIG: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := run(t, dir, map[string]string{"HOZO_CONFIG": "alt.toml"}, "check", "--config", "hozo.toml"); code != ExitConfigInvalid {
		t.Fatalf("--config が HOZO_CONFIG より優先されていない: code=%d stderr=%q", code, stderr)
	}
}

func TestCheckLanguageMismatch(t *testing.T) {
	dir := copyCorpus(t)
	fakePlugin(t, dir, "tsp", "ts", "[emit]\nentry = [\"cli.command\"]\n", `{"files":[],"warnings":[]}`, 0)
	write(t, filepath.Join(dir, "hozo.toml"), "[hozo]\nplugin_dirs = [\"plugins\"]\n\n[server]\nlanguage = \"go\"\nout = \"gen/server\"\nplugins = [\"tsp\"]\n\n[browser]\nlanguage = \"go\"\nout = \"gen/browser\"\n")
	code, _, stderr := run(t, dir, nil, "check")
	if code != ExitConfigInvalid || !strings.Contains(stderr, "error[CFG-4]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	l := &Leaves{Dir: dir}
	if _, err := l.Check(CheckInput{}); err == nil || !strings.HasPrefix(err.Error(), "hozoc.LanguageMismatch") {
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
	if code != ExitOutputConflict || stdout != "" || !strings.Contains(stderr, "gen/server/types.go: error[CFG-5]") || !strings.Contains(stderr, "プラグイン fake") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for f, m := range mtimes(t, filepath.Join(dir, "gen")) {
		if before[f] != m {
			t.Fatalf("%s が書き換えられた", f)
		}
	}

	// PLG-1 → proc.MalformedOutput → 70
	fakePlugin(t, dir, "fake", "go", emitTable, `{"files":[{"path":"../x.go","content":"x"}],"warnings":[]}`, 0)
	if code, _, stderr = run(t, dir, nil, "emit"); code != ExitSoftwareOrCrash || !strings.Contains(stderr, "PLG-1") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// errors → CheckFailed（1）
	fakePlugin(t, dir, "fake", "go", emitTable, `{"files":[],"warnings":[],"errors":[{"code":"FAKE-1","at":"server.placeOrder","message":"nope"}]}`, 0)
	if code, _, stderr = run(t, dir, nil, "emit"); code != ExitCheckFailed || !strings.Contains(stderr, "server.hozo:230:3: error[FAKE-1]: nope") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// 異常終了 → 70
	fakePlugin(t, dir, "fake", "go", emitTable, ``, 3)
	if code, _, stderr = run(t, dir, nil, "emit"); code != ExitSoftwareOrCrash || !strings.Contains(stderr, "hozoc: ") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestEmitUnknownNamespace(t *testing.T) {
	dir := copyCorpus(t)
	code, stdout, stderr := run(t, dir, nil, "emit", "nope")
	if code != ExitUsage || stdout != "" || !strings.Contains(stderr, "名前空間 nope は hozo.toml にない") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
		t.Fatal("書き出している")
	}
	code, _, stderr = run(t, dir, nil, "emit", "--format=json", "nope")
	if code != ExitUsage || strings.TrimSpace(stderr) != "{\n  \"error\": \"hozoc.UnknownNamespace\"\n}" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestUsageAndLsp(t *testing.T) {
	dir := copyCorpus(t)
	cases := []struct {
		args []string
		code int
	}{
		{nil, ExitUsage},
		{[]string{"help"}, 0},
		{[]string{"check", "--help"}, 0},
		{[]string{"bogus"}, ExitUsage},
		{[]string{"check", "--nope"}, ExitUsage},
		{[]string{"check", "--format"}, ExitUsage},
		{[]string{"check", "--format=yaml"}, ExitUsage},
		{[]string{"check", "server"}, ExitUsage},
		{[]string{"emit", "--dry-run=maybe"}, ExitUsage},
		{[]string{"lsp"}, ExitUsage},
		{[]string{"emit", "--dry-run", "--", "server"}, 0},
	}
	for _, c := range cases {
		code, _, stderr := run(t, dir, nil, c.args...)
		if code != c.code {
			t.Errorf("%v: code=%d, want %d (stderr=%q)", c.args, code, c.code, stderr)
		}
	}
	_, _, stderr := run(t, dir, nil, "lsp")
	if !strings.Contains(stderr, "not implemented") {
		t.Errorf("lsp stderr=%q", stderr)
	}
}

func TestParseArgs(t *testing.T) {
	flags, pos, err := parseArgs([]string{"--config", "a.toml", "--format=json", "--dry-run", "-x", "ns1", "--", "--ns2"})
	if err != nil {
		t.Fatal(err)
	}
	if flags["config"] != "a.toml" || flags["format"] != "json" || flags["dry-run"] != "true" {
		t.Fatalf("flags = %v", flags)
	}
	if strings.Join(pos, ",") != "-x,ns1,--ns2" {
		t.Fatalf("positional = %v", pos)
	}
	flags, _, _ = parseArgs([]string{"--format", "text", "--format", "json", "--dry-run=false"})
	if flags["format"] != "json" || flags["dry-run"] != "false" {
		t.Fatalf("flags = %v", flags)
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
