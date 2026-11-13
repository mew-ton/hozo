package tests

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/app"
	"github.com/mew-ton/hozo/internal/gen/hozoc/cli"
)

// hozocMain は cmd/hozoc/main.go と同じ組み立て（docs/cli.md「手で書くもの」）: 生成した cli.Main に
// internal/app の App の Ops（hozoc.States に実装を入れて hozoc.New）と Presenter を渡す。作業ディレクトリは tests/ で、
// hozo.toml は親（リポジトリ直下）に見つかる。
func hozocMain(args []string, stdout, stderr io.Writer) int {
	return hozocMainIn("", args, stdout, stderr)
}

// hozocMainIn は dir を作業ディレクトリ（hozo.toml を探し始める場所）にして hozoc を走らせる。空なら os の作業ディレクトリ。
func hozocMainIn(dir string, args []string, stdout, stderr io.Writer) int {
	a := &app.App{Dir: dir, Stderr: stderr}
	return cli.Main(a.Ops(), app.Presenter{}, args, func(string) string { return "" }, stdout, stderr)
}

// TestSelfCheck は hozoc 自身（リポジトリ直下の cli.hozo と hozo.toml）が hozoc check を error なしで通ること
// （docs/testing.md「コーパス」の最後の段落、docs/requirements.md「段階」の 2）。warning は許し、表示する。
func TestSelfCheck(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := hozocMain([]string{"check"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("hozoc check の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	t.Logf("hozoc check:\n%s", stdout.String())
}

// TestSelfEmitDryRun は hozoc 自身への emit --dry-run が、既定の出力（internal/gen/hozoc/ の下）と
// go-cli の internal/gen/hozoc/cli/cli.go を書く予定にすること。go-cli は `go run` で動く（docs/plugin-go-cli.md）ので、
// go がなければ飛ばす。
func TestSelfEmitDryRun(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go がない")
	}
	var stdout, stderr bytes.Buffer
	code := hozocMain([]string{"emit", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("hozoc emit --dry-run の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	var written []string
	for _, l := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if p, ok := strings.CutPrefix(l, "written: "); ok {
			written = append(written, p)
		}
	}
	if len(written) == 0 {
		t.Fatalf("書く予定のファイルがない:\n%s%s", stdout.String(), stderr.String())
	}
	cli := false
	for _, p := range written {
		if !strings.HasPrefix(p, "internal/gen/hozoc/") {
			t.Errorf("%s は internal/gen/hozoc/ の下にない", p)
		}
		if p == "internal/gen/hozoc/cli/cli.go" {
			cli = true
		}
	}
	if !cli {
		t.Errorf("go-cli の internal/gen/hozoc/cli/cli.go がない: %v", written)
	}
	t.Logf("hozoc emit --dry-run:\n%s", stdout.String())
}
