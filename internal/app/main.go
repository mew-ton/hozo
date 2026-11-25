package app

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mew-ton/hozo/internal/gen/hozo/cli/cli"
)

// exitUsageError は、使い方の誤りの終了コード（docs/diagnostics.md「終了コード」の 64）。
const exitUsageError = 64

// Main は hozoc の入口: 引数 args（プログラム名を含まない）を解釈して、コマンドを走らせ、終了コードを返す。cmd/hozoc が呼ぶ。
// 骨組みは、cli.hozo から生成した cli.Main（docs/plugin-go-cli.md「出力」）で、ここが足すのは、古い名前のコマンドの案内だけ
// （[Renamed]。docs/cli.md「build」の「古い名前」）。env は環境変数の値を返す関数（nil なら、環境変数を見ない）。
func Main(ctx context.Context, a *App, args []string, env func(string) string, stdout, stderr io.Writer) int {
	if message, renamed := Renamed(args); renamed {
		fmt.Fprintln(stderr, message)
		return exitUsageError
	}
	return cli.Main(ctx, a.Ops(), Presenter{}, args, env, stdout, stderr)
}

// Renamed は、引数 args の先頭が、なくしたコマンドの名前（check、emit）なら、新しい書き方を示す文と true を返す
// （docs/cli.md「build」の「古い名前」、決定 D417）。示すのは、残りの引数を付けたもの:
// check には hozoc build --no-emit、emit には hozoc build。
func Renamed(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	var now []string
	switch args[0] {
	case "check":
		now = []string{"hozoc", "build", "--no-emit"}
	case "emit":
		now = []string{"hozoc", "build"}
	default:
		return "", false
	}
	now = append(now, args[1:]...)
	return fmt.Sprintf("hozoc: コマンド %s は、なくなった。代わりに、%s を使う（hozoc build は、検査して、書き出す。--no-emit を付ければ、検査だけ）", args[0], strings.Join(now, " ")), true
}
