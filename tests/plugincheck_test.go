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

// このファイルは、プラグインの診断が、hozoc check で出ることを、本物のプラグインで確かめる（docs/cli.md「check」の 4、
// docs/plugin-protocol.md「起動」「emit」、docs/decisions.md「決めたこと」404、416）。
//
// 検査は、プラグインに emit を頼み、その答えの診断を使う。SDK が、要求の並びを回すことは、plugin の TestRunEmitBatch、hozoc が、
// プラグインごとに一回、受け持つ出力を並べて頼むことは、internal/project の TestPluginAnswersPerOutput。ここは、その二つが、
// コマンドとして、つながっていること。

// pluginCheckProject は、プラグインの単体テストの入力 plugins/<plugin>/internal/<pkg>/testdata/<name>/ を、一時ディレクトリの
// プロジェクトに写す: hozo.toml と .hozo と、Go の基底が import パスを決めるための go.mod（リポジトリを replace で指す。プラグインは、
// 開発中のマニフェストの `go run` で起動するので、go.mod は、プラグインのパッケージを解決するのにも要る）。
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
	// リポジトリの go.mod の require を、間接の依存として写す（`go run` が、go.mod を書き換えずに、プラグインを解決できるように）
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

// TestPluginDiagnosticsAtCheck は、通信に出せない union（GOHTTP-5。docs/plugin-go-http.md「通信の形」の「union」）が、
// hozoc check で、位置と一緒に出て、0 でない終了コードで終わること。hozoc emit も、同じ診断で止まり、何も書き出さない。
func TestPluginDiagnosticsAtCheck(t *testing.T) {
	root := pluginCheckProject(t, "go-http", "gohttp", "gohttp5")
	want, err := os.ReadFile(filepath.Join(repoPluginsDir, "go-http", "internal", "gohttp", "testdata", "gohttp5", "expected", "errors.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(want)), "\n")

	var stdout, stderr bytes.Buffer
	code := hozocCorpus(t, root, []string{"check"}, &stdout, &stderr)
	out := stderr.String()
	if code != 1 {
		t.Fatalf("hozoc check の終了コード = %d（期待は 1: CheckFailed）\n%s", code, out)
	}
	// プラグインの単体テストが待つ診断（コード、位置の宣言、文面）が、すべて、位置（ファイル、行、列）と一緒に出る
	located := regexp.MustCompile(`(?m)^app\.hozo:\d+:\d+: error\[GOHTTP-5\]: `)
	if got := len(located.FindAllString(out, -1)); got != len(lines) {
		t.Errorf("位置の付いた GOHTTP-5 が %d 件（期待は %d 件）\n%s", got, len(lines), out)
	}
	for _, l := range lines {
		f := strings.SplitN(l, "\t", 3)
		if len(f) != 3 || !strings.Contains(out, "error[GOHTTP-5]: "+f[2]) {
			t.Errorf("診断が出ない: %s", l)
		}
	}
	if !strings.Contains(out, fmt.Sprintf("%d errors, 0 warnings", len(lines))) {
		t.Errorf("数が合わない（期待は %d errors）\n%s", len(lines), out)
	}

	// emit も、同じ診断で止まり、何も書き出さない
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"emit"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "error[GOHTTP-5]") {
		t.Errorf("hozoc emit の終了コード = %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "gen")); err == nil {
		t.Error("診断があるのに、emit が、ファイルを書き出した")
	}
}

// TestCheckPassesThenEmitPasses は、hozoc check が通った仕様は、hozoc emit で、プラグインの errors に止められないこと:
// 通信に出せる union を持つ入力（go-http の unions）で、check も emit も、0 で終わる。
func TestCheckPassesThenEmitPasses(t *testing.T) {
	root := pluginCheckProject(t, "go-http", "gohttp", "unions")
	var stdout, stderr bytes.Buffer
	if code := hozocCorpus(t, root, []string{"check"}, &stdout, &stderr); code != 0 || stdout.String() != "0 errors, 0 warnings\n" {
		t.Fatalf("hozoc check の終了コード = %d\n%s%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := hozocCorpus(t, root, []string{"emit"}, &stdout, &stderr); code != 0 {
		t.Fatalf("hozoc emit の終了コード = %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "gen", "api", "http.go")); err != nil {
		t.Errorf("emit が、gen/api/http.go を書き出していない: %v", err)
	}
}
