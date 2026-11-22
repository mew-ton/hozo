package tests

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/app"
	"github.com/mew-ton/hozo/internal/gen/hozoc/cli"
)

// hozocMain は cmd/hozoc/main.go と同じ組み立て（docs/cli.md「手で書くもの」）: 生成した cli.Main に
// internal/app の App の Ops（葉の op と状態の単位の実装を hozoc.New に渡したもの）と Presenter を渡す。作業ディレクトリは tests/ で、
// hozo.toml は親（リポジトリ直下）に見つかる。
func hozocMain(args []string, stdout, stderr io.Writer) int {
	return hozocMainIn("", args, stdout, stderr)
}

// hozocMainIn は dir を作業ディレクトリ（hozo.toml を探し始める場所）にして hozoc を走らせる。空なら os の作業ディレクトリ。
func hozocMainIn(dir string, args []string, stdout, stderr io.Writer) int {
	a := &app.App{Dir: dir, Stderr: stderr}
	return cli.Main(context.Background(), a.Ops(), app.Presenter{}, args, func(string) string { return "" }, stdout, stderr)
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

// TestSelfEmitDryRun は hozoc 自身への emit --dry-run が、コミット済みの生成物（二つの単位の出力: 基底 go と go-valid と go-cli の
// internal/gen/hozoc/ と、基底 go の internal/gen/proc/）と、ちょうど同じファイルを書く予定にすること（出力先ごとの記録
// .hozoc-outputs.json は、トランスパイラのファイルではないので、予定には出ない）。書き出さないこと。プラグインは `go run` で動く
// ので、go がなければ飛ばす。
func TestSelfEmitDryRun(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go がない")
	}
	before := map[string][]byte{}
	for _, dir := range generatedDirs {
		for p, content := range readTree(t, filepath.Join("..", filepath.FromSlash(dir))) {
			before[dir+"/"+p] = content
		}
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
		} else {
			t.Errorf("書く予定でない行: %q", l)
		}
	}
	var committed []string
	for p := range before {
		if filepath.Base(p) != ".hozoc-outputs.json" {
			committed = append(committed, p)
		}
	}
	sort.Strings(committed)
	if len(committed) == 0 || strings.Join(written, "\n") != strings.Join(committed, "\n") {
		t.Errorf("書く予定のファイルが、コミット済みの生成物と違う:\n予定:\n  %s\nコミット済み:\n  %s", strings.Join(written, "\n  "), strings.Join(committed, "\n  "))
	}
	for _, want := range []string{"internal/gen/hozoc/cli/cli.go", "internal/gen/hozoc/types/validate.go", "internal/gen/hozoc/ops.go", "internal/gen/proc/types.go"} {
		if _, ok := before[want]; !ok {
			t.Errorf("%s がコミットされていない", want)
		}
	}
	// --dry-run は書き出さない
	for _, dir := range generatedDirs {
		for p, content := range readTree(t, filepath.Join("..", filepath.FromSlash(dir))) {
			if !bytes.Equal(before[dir+"/"+p], content) {
				t.Errorf("--dry-run が %s/%s を書き換えた", dir, p)
			}
		}
	}
	t.Logf("hozoc emit --dry-run:\n%s", stdout.String())
}
