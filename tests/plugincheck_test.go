package tests

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// このファイルは、プラグインの診断が、hozoc build --no-emit で出ることを、本物のプラグインで確かめる（docs/cli.md「検査」の 4、
// docs/plugin-protocol.md「語彙のプラグイン」「起動」「read」「emit」、docs/decisions.md「決めたこと」404、410、416）。
//
// 検査は、プラグインに emit を頼み、その答えの診断を使う。SDK が、要求の並びを回すことは、plugin の TestRunEmitBatch、hozoc が、
// プラグインごとに一回、受け持つ出力を並べて頼むことは、internal/project の TestPluginAnswersPerOutput。ここは、その二つが、
// コマンドとして、つながっていること。

// pluginCheckProject は、プラグインの単体テストの入力 plugins/<plugin>/internal/<pkg>/testdata/<name>/ を、一時ディレクトリの
// プロジェクトに写す: hozo.toml と .hozo と、Go の基底が import パスを決めるための go.mod（リポジトリを replace で指す）。プラグインは、
// マニフェストの wasm（TestMain がビルドしたもの）を動かす。
func pluginCheckProject(t *testing.T, plugin, pkg, name string) string {
	t.Helper()
	src := filepath.Join(repoPluginsDir, plugin, "internal", pkg, "testdata", name)
	root := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !(e.Name() == "hozo.toml" || strings.HasSuffix(e.Name(), ".hozo")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	// リポジトリの go.mod の require を、間接の依存として写す（プラグインを `go run` で起動していた間に、要った形。今は、プラグインは WASM で、これに依らないが、写すプロジェクトの形は、変えていない）
	repoMod, err := os.ReadFile(filepath.Join(repo, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	var indirect []string
	for _, m := range regexp.MustCompile(`(?m)^(?:require\s+)?\t?([a-zA-Z0-9./_-]+\.[a-zA-Z0-9./_-]+) (v[^\s]+)`).FindAllStringSubmatch(string(repoMod), -1) {
		indirect = append(indirect, "require "+m[1]+" "+m[2]+" // indirect\n")
	}
	// go の版は、リポジトリの go.mod と同じ（依存の求める版より古いと、go.mod の書き換えが要る）
	goVersion := regexp.MustCompile(`(?m)^go (\S+)$`).FindStringSubmatch(string(repoMod))
	if goVersion == nil {
		t.Fatal("リポジトリの go.mod に、go の版がない")
	}
	gomod := fmt.Sprintf("module example.com/app\n\ngo %s\n\nrequire github.com/mew-ton/hozo v0.0.0\n\n%s\nreplace github.com/mew-ton/hozo => %s\n",
		goVersion[1], strings.Join(indirect, ""), filepath.ToSlash(repo))
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	if sum, err := os.ReadFile(filepath.Join(repo, "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(root, "go.sum"), sum, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestPluginDiagnosticsAtCheck は、トランスパイラの診断（省略可能な配列を返す経路の GOHTTP-16。docs/plugin-go-http.md「対象」）が、
// hozoc build --no-emit で、位置と一緒に出て、0 でない終了コードで終わること。hozoc build も、同じ診断で止まり、何も書き出さない。
// この入力は、http の語彙の検査（語彙のプラグイン http）を通る: 語彙のプラグインの後に、go-http が、頼まれている。
func TestPluginDiagnosticsAtCheck(t *testing.T) {
	root := pluginCheckProject(t, "go-http", "gohttp", "gohttp16")
	want, err := os.ReadFile(filepath.Join(repoPluginsDir, "go-http", "internal", "gohttp", "testdata", "gohttp16", "expected", "errors.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(want)), "\n")

	var stdout, stderr bytes.Buffer
	code := hozocCorpus(t, root, []string{"build", "--no-emit"}, &stdout, &stderr)
	out := stderr.String()
	if code != 1 {
		t.Fatalf("hozoc build --no-emit の終了コード = %d（期待は 1: CheckFailed）\n%s", code, out)
	}
	// プラグインの単体テストが待つ診断（コード、位置の宣言、文面）が、すべて、位置（ファイル、行、列）と一緒に出る
	located := regexp.MustCompile(`(?m)^app\.hozo:\d+:\d+: error\[GOHTTP-16\]: `)
	if got := len(located.FindAllString(out, -1)); got != len(lines) {
		t.Errorf("位置の付いた GOHTTP-16 が %d 件（期待は %d 件）\n%s", got, len(lines), out)
	}
	for _, l := range lines {
		f := strings.SplitN(l, "\t", 3)
		if len(f) != 3 || !strings.Contains(out, "error[GOHTTP-16]: "+f[2]) {
			t.Errorf("診断が出ない: %s", l)
		}
	}
	if !strings.Contains(out, fmt.Sprintf("%d errors, 0 warnings", len(lines))) {
		t.Errorf("数が合わない（期待は %d errors）\n%s", len(lines), out)
	}

	// emit も、同じ診断で止まり、何も書き出さない
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"build"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "error[GOHTTP-16]") {
		t.Errorf("hozoc build の終了コード = %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "gen")); err == nil {
		t.Error("診断があるのに、emit が、ファイルを書き出した")
	}
}

// TestCheckPassesThenEmitPasses は、hozoc build --no-emit が通った仕様は、hozoc build で、プラグインの errors に止められないこと:
// 通信に出せる union を持つ入力（go-http の unions）で、check も emit も、0 で終わる。
func TestCheckPassesThenEmitPasses(t *testing.T) {
	root := pluginCheckProject(t, "go-http", "gohttp", "unions")
	var stdout, stderr bytes.Buffer
	if code := hozocCorpus(t, root, []string{"build", "--no-emit"}, &stdout, &stderr); code != 0 || stdout.String() != "0 errors, 0 warnings\n" {
		t.Fatalf("hozoc build --no-emit の終了コード = %d\n%s%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"build"}, &stdout, &stderr); code != 0 {
		t.Fatalf("hozoc build の終了コード = %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "gen", "api", "http.go")); err != nil {
		t.Errorf("emit が、gen/api/http.go を書き出していない: %v", err)
	}
}

// vocabularyProject は、語彙の読み解きの単体テストの入力 plugin/<library>/testdata/<name>/ の .hozo を、一時ディレクトリの
// プロジェクトに写し、hozo.toml を config の中身で置く。期待する診断（その expected/errors.txt の行）も返す。
func vocabularyProject(t *testing.T, library, name, config string) (root string, lines []string) {
	t.Helper()
	src := filepath.Join("..", "plugin", library, "testdata", name)
	root = t.TempDir()
	sources, err := filepath.Glob(filepath.Join(src, "*.hozo"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("%s に .hozo がない（%v）", src, err)
	}
	for _, path := range sources {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.Base(path)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "hozo.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(src, "expected", "errors.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return root, strings.Split(strings.TrimSpace(string(want)), "\n")
}

// wantVocabularyDiagnostics は、hozoc の出力 out に、期待する診断 lines（コード code、位置の宣言、文面）が、どれも、位置（ファイル、行、列）と
// 一緒に、一度ずつ出ていて、数が合うことを確かめる。
func wantVocabularyDiagnostics(t *testing.T, out, code string, lines []string) {
	t.Helper()
	located := regexp.MustCompile(`(?m)^app\.hozo:\d+:\d+: error\[` + regexp.QuoteMeta(code) + `\]: `)
	if got := len(located.FindAllString(out, -1)); got != len(lines) {
		t.Errorf("位置の付いた %s が %d 件（期待は %d 件）\n%s", code, got, len(lines), out)
	}
	for _, l := range lines {
		f := strings.SplitN(l, "\t", 3)
		if len(f) != 3 || strings.Count(out, "error["+code+"]: "+f[2]+"\n") != 1 {
			t.Errorf("診断が、ちょうど一度、出ていない: %s", l)
		}
	}
	if !strings.Contains(out, fmt.Sprintf("%d errors, 0 warnings", len(lines))) {
		t.Errorf("数が合わない（期待は %d errors）\n%s", len(lines), out)
	}
}

// TestVocabularyDiagnosticsWithoutUnits は、語彙のプラグインが、出力の単位がなくても、働くこと（docs/plugin-protocol.md
// 「語彙のプラグイン」、docs/decisions.md「決めたこと」410）: 単位を一つも持たない設定の仕様が、通信に出せない union を、経路に
// 使っていれば（HTTP-5。docs/plugin-http.md「通信の形」の「union」）、hozoc build --no-emit が、本物の語彙のプラグイン http の診断を、
// 位置と一緒に出して、0 でない終了コードで終わる。
func TestVocabularyDiagnosticsWithoutUnits(t *testing.T) {
	root, lines := vocabularyProject(t, "httpvocab", "http5", "# 単位は、一つもない\n[hozo]\nsources = [\"*.hozo\"]\n")
	var stdout, stderr bytes.Buffer
	code := hozocCorpus(t, root, []string{"build", "--no-emit"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("hozoc build --no-emit の終了コード = %d（期待は 1: CheckFailed）\n%s", code, stderr.String())
	}
	wantVocabularyDiagnostics(t, stderr.String(), "HTTP-5", lines)

	// http の語彙を、正しく使っている仕様（plugin/httpvocab の入力 namespaces）は、単位がなくても、通る
	root, _ = vocabularyProject(t, "httpvocab", "http5", "[hozo]\nsources = [\"*.hozo\"]\n")
	ok, err := os.ReadFile(filepath.Join("..", "plugin", "httpvocab", "testdata", "namespaces", "app.hozo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.hozo"), ok, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"build", "--no-emit"}, &stdout, &stderr); code != 0 || !strings.HasPrefix(stdout.String(), "0 errors") {
		t.Errorf("正しい仕様の hozoc build --no-emit の終了コード = %d\n%s%s", code, stdout.String(), stderr.String())
	}
}

// TestVocabularyDiagnosticsBeforeEmit は、同じ仕様に、go-http の単位を足しても、語彙の使い方の診断（HTTP-5）が、一度だけ出ること
// （docs/plugin-protocol.md「語彙のプラグイン」: 語彙のプラグインが error を返したら、emit は、頼まない。go-http は、同じ診断を、
// 自分では返さない）。hozoc build も、同じ診断で止まり、何も書き出さない。
func TestVocabularyDiagnosticsBeforeEmit(t *testing.T) {
	config := "[options.go]\nmodule = \"example.com/app\"\n\n[[emit]]\nnamespace = \"app\"\ntranspilers = [\"go\", \"go-valid\", \"go-http\"]\nout = \"gen/app\"\n"
	root, lines := vocabularyProject(t, "httpvocab", "http5", config)
	var stdout, stderr bytes.Buffer
	if code := hozocCorpus(t, root, []string{"build", "--no-emit"}, &stdout, &stderr); code != 1 {
		t.Fatalf("hozoc build --no-emit の終了コード = %d（期待は 1: CheckFailed）\n%s", code, stderr.String())
	}
	wantVocabularyDiagnostics(t, stderr.String(), "HTTP-5", lines)
	if strings.Contains(stderr.String(), "GOHTTP-") {
		t.Errorf("語彙の使い方の誤りが、go-http の診断としても出た\n%s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"build"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "error[HTTP-5]") {
		t.Errorf("hozoc build の終了コード = %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "gen")); err == nil {
		t.Error("診断があるのに、build が、ファイルを書き出した")
	}
}

// TestValidVocabularyDiagnostics は、語彙のプラグイン valid が、制約の使い方の誤り（VALID-1。docs/plugin-valid.md「診断」）を、
// 出力の単位がなくても、位置と一緒に出すこと（docs/decisions.md「決めたこと」429）。同じ仕様に、go-valid の単位を足しても、
// 診断は、一度だけ出る: 語彙のプラグインが error を返したら、emit は、頼まない（go-valid は、同じ診断を、自分では返さない。
// docs/plugin-go-valid.md「診断」）。hozoc build も、同じ診断で止まり、何も書き出さない。
func TestValidVocabularyDiagnostics(t *testing.T) {
	root, lines := vocabularyProject(t, "validvocab", "valid1", "# 単位は、一つもない\n[hozo]\nsources = [\"*.hozo\"]\n")
	var stdout, stderr bytes.Buffer
	if code := hozocCorpus(t, root, []string{"build", "--no-emit"}, &stdout, &stderr); code != 1 {
		t.Fatalf("hozoc build --no-emit の終了コード = %d（期待は 1: CheckFailed）\n%s", code, stderr.String())
	}
	wantVocabularyDiagnostics(t, stderr.String(), "VALID-1", lines)

	// 制約を、正しく使っている仕様（利用者が valid に足した制約を含む。plugin/validvocab の入力 clean）は、単位がなくても、通る
	ok, err := os.ReadFile(filepath.Join("..", "plugin", "validvocab", "testdata", "clean", "app.hozo"))
	if err != nil {
		t.Fatal(err)
	}
	clean := t.TempDir()
	for name, data := range map[string][]byte{"app.hozo": ok, "hozo.toml": []byte("[hozo]\nsources = [\"*.hozo\"]\n")} {
		if err := os.WriteFile(filepath.Join(clean, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, clean, []string{"build", "--no-emit"}, &stdout, &stderr); code != 0 || !strings.HasPrefix(stdout.String(), "0 errors") {
		t.Errorf("正しい仕様の hozoc build --no-emit の終了コード = %d\n%s%s", code, stdout.String(), stderr.String())
	}

	config := "[options.go]\nmodule = \"example.com/app\"\n\n[[emit]]\nnamespace = \"app\"\ntranspilers = [\"go\", \"go-valid\"]\nout = \"gen/app\"\n"
	root, lines = vocabularyProject(t, "validvocab", "valid1", config)
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"build", "--no-emit"}, &stdout, &stderr); code != 1 {
		t.Fatalf("hozoc build --no-emit の終了コード = %d（期待は 1: CheckFailed）\n%s", code, stderr.String())
	}
	wantVocabularyDiagnostics(t, stderr.String(), "VALID-1", lines)
	if strings.Contains(stderr.String(), "GOVALID-") || strings.Contains(stderr.String(), "PLG-") {
		t.Errorf("制約の使い方の誤りが、go-valid からも出た\n%s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"build"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "error[VALID-1]") {
		t.Errorf("hozoc build の終了コード = %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "gen")); !os.IsNotExist(err) {
		t.Errorf("診断で止まったのに、gen/ がある（%v）", err)
	}
}

// TestDBVocabularyDiagnostics は、語彙のプラグイン db が、db のデコレータと宣言の食い違い（DB-6。docs/plugin-db.md「診断」）を、
// 出力の単位がなくても、位置と一緒に出すこと（docs/decisions.md「決めたこと」429）。同じ仕様に、go-sqlite の単位を足しても、
// 診断は、一度だけ出る: 語彙のプラグインが error を返したら、emit は、頼まない（go-sqlite は、同じ診断を、自分では返さない。
// docs/plugin-go-sqlite.md「診断」）。hozoc build も、同じ診断で止まり、何も書き出さない。
func TestDBVocabularyDiagnostics(t *testing.T) {
	root, lines := vocabularyProject(t, "dbvocab", "db6", "# 単位は、一つもない\n[hozo]\nsources = [\"*.hozo\"]\n")
	var stdout, stderr bytes.Buffer
	if code := hozocCorpus(t, root, []string{"build", "--no-emit"}, &stdout, &stderr); code != 1 {
		t.Fatalf("hozoc build --no-emit の終了コード = %d（期待は 1: CheckFailed）\n%s", code, stderr.String())
	}
	wantVocabularyDiagnostics(t, stderr.String(), "DB-6", lines)

	// db の語彙を、正しく使っている仕様（単位のない名前空間の、表の状態と read を含む。plugin/dbvocab の入力 clean）は、単位がなくても、通る
	ok, err := os.ReadFile(filepath.Join("..", "plugin", "dbvocab", "testdata", "clean", "app.hozo"))
	if err != nil {
		t.Fatal(err)
	}
	clean := t.TempDir()
	for name, data := range map[string][]byte{"app.hozo": ok, "hozo.toml": []byte("[hozo]\nsources = [\"*.hozo\"]\n")} {
		if err := os.WriteFile(filepath.Join(clean, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, clean, []string{"build", "--no-emit"}, &stdout, &stderr); code != 0 || !strings.HasPrefix(stdout.String(), "0 errors") {
		t.Errorf("正しい仕様の hozoc build --no-emit の終了コード = %d\n%s%s", code, stdout.String(), stderr.String())
	}

	config := "[options.go]\nmodule = \"example.com/app\"\n\n[[emit]]\nnamespace = \"app\"\ntranspilers = [\"go\", \"go-sqlite\"]\nout = \"gen/app\"\n"
	root, lines = vocabularyProject(t, "dbvocab", "db6", config)
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"build", "--no-emit"}, &stdout, &stderr); code != 1 {
		t.Fatalf("hozoc build --no-emit の終了コード = %d（期待は 1: CheckFailed）\n%s", code, stderr.String())
	}
	wantVocabularyDiagnostics(t, stderr.String(), "DB-6", lines)
	if strings.Contains(stderr.String(), "GOSQL-") || strings.Contains(stderr.String(), "PLG-") {
		t.Errorf("db の語彙の食い違いが、go-sqlite からも出た\n%s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"build"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "error[DB-6]") {
		t.Errorf("hozoc build の終了コード = %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "gen")); !os.IsNotExist(err) {
		t.Errorf("診断で止まったのに、gen/ がある（%v）", err)
	}
}
