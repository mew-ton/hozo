// Command hozoc は Hozo の仕様を検査し、各言語のコードに展開する CLI（docs/cli.md）。
//
// 骨組みは cli.hozo から生成したもの（docs/requirements.md「自己展開」の 3）: 名前空間 hozoc の型と op の形と配線
// internal/gen/hozoc（基底 go と go-valid。docs/plugin-go.md、docs/plugin-go-valid.md）と、go-cli が出した
// internal/gen/hozoc/cli（docs/plugin-go-cli.md「出力」）。ここは、internal/app の Main を呼ぶだけ: internal/app の葉の op と
// 状態の単位の実装を hozoc.New に渡したもの（app.App の Ops）を、表示（cli.Presenter）と一緒に cli.Main に渡す
// （docs/cli.md「手で書くもの」）。古い名前のコマンド（check、emit）には、新しい書き方を示す。
// 骨組みを変えるときは cli.hozo を変えて `make generate`。
//
// プロセスの環境を読むのはここだけ: 環境変数は os.Getenv を app.Main（から cli.Main）に渡し（cli.hozo の @cli.env のフィールドに入る）、
// ホームディレクトリ（プラグインの探索の <ホーム>/.hozo/plugins。docs/plugin-protocol.md「探索」）は app.App に渡す。
package main

import (
	"context"
	"os"

	"github.com/mew-ton/hozo/internal/app"
)

func main() {
	home, _ := os.UserHomeDir() // 分からなければ空（ホームのプラグインは探さない）
	a := &app.App{Stderr: os.Stderr, Home: home}
	os.Exit(app.Main(context.Background(), a, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
