package tests

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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

// TestSelfCheck は hozoc 自身（リポジトリ直下の cli.hozo と hozo.toml）が hozoc build --no-emit を error なしで通ること
// （docs/testing.md「コーパス」の最後の段落、docs/requirements.md「段階」の 2）。warning は許し、表示する。
func TestSelfCheck(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := hozocMain([]string{"build", "--no-emit"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("hozoc build --no-emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	t.Logf("hozoc build --no-emit:\n%s", stdout.String())
}

// splitUninterpreted は、hozoc の text の出力 stdout から、CFG-26（解釈するトランスパイラがいないデコレータ。docs/config.md
// 「解釈するトランスパイラ」）の warning の行と、それを数えた要約の行（"0 errors, N warnings"）を除いた残りと、除いた warning の行を返す。
// warning が CFG-26 だけの出力に使う: ほかの診断の行と、要約の数の食い違いは、残りに残るので、呼ぶ側の比べで、失敗する。
func splitUninterpreted(stdout string) (rest string, warnings []string) {
	var kept []string
	for _, l := range strings.SplitAfter(stdout, "\n") {
		if strings.Contains(l, ": warning[CFG-26]: @") {
			warnings = append(warnings, strings.TrimSuffix(l, "\n"))
			continue
		}
		kept = append(kept, l)
	}
	summary := "0 errors, " + strconv.Itoa(len(warnings)) + " warnings\n"
	if len(warnings) == 1 {
		summary = "0 errors, 1 warning\n"
	}
	for i, l := range kept {
		if len(warnings) > 0 && l == summary {
			kept = append(kept[:i:i], kept[i+1:]...)
			break
		}
	}
	return strings.Join(kept, ""), warnings
}

// TestSelfEmitDryRun は hozoc 自身への emit --dry-run が、コミット済みの生成物（二つの単位の出力: 基底 go と go-valid と go-cli の
// internal/gen/hozoc/）と、ちょうど同じファイルを書く予定にすること（出力先ごとの記録
// .hozoc-outputs.json は、トランスパイラのファイルではないので、予定には出ない）。書き出さないこと。プラグインの WASM は、
// go でビルドするので、go がなければ飛ばす。
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
	code := hozocMain([]string{"build", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("hozoc build --dry-run の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	// hozoc 自身の仕様の warning は、CFG-26 が二つ: @fs.files（状態 sources と config）。どのトランスパイラも
	// 解釈せず、中身は、人が書いている（docs/vocab.md「解釈されなくてよいデコレータ」、docs/decisions.md「決めたこと」433）
	rest, warnings := splitUninterpreted(stdout.String())
	var marks []string
	for _, w := range warnings {
		_, after, _ := strings.Cut(w, "warning[CFG-26]: ")
		mark, _, _ := strings.Cut(after, " ")
		marks = append(marks, mark)
	}
	if got := strings.Join(marks, ","); got != "@fs.files,@fs.files" {
		t.Errorf("CFG-26 の warning = %v, want [@fs.files @fs.files]", marks)
	}
	var written []string
	for _, l := range strings.Split(strings.TrimSpace(rest), "\n") {
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
	for _, want := range []string{"internal/gen/hozoc/cli/cli.go", "internal/gen/hozoc/types/validate.go", "internal/gen/hozoc/ops.go", "internal/gen/hozoc/types/types.go"} {
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
	t.Logf("hozoc build --dry-run:\n%s", stdout.String())
}
