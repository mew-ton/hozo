// Command hozoc は Hozo の仕様を検査し、各言語のコードに展開する CLI（docs/cli.md）。
//
// 骨組みは cli.hozo から生成したもの（docs/requirements.md「自己展開」の 3）: 名前空間 hozoc の Go の既定の出力
// internal/gen/hozoc（docs/emit-go.md）と、go-cli が出した internal/gen/hozoc/cli（docs/plugin-go-cli.md「出力」）。
// ここは internal/app の葉の op の実装（hozoc.Leaves）と表示（cli.Presenter）を組み立てて cli.Main を呼ぶだけ
// （docs/cli.md「手で書くもの」）。骨組みを変えるときは cli.hozo を変えて `make generate`。
package main

import (
	"os"

	"github.com/mew-ton/hozo/internal/app"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/gen/hozoc/cli"
)

func main() {
	ops := hozoc.New(&app.Leaves{Stderr: os.Stderr})
	os.Exit(cli.Main(ops, app.Presenter{}, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
