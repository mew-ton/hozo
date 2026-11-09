// Command hozoc は Hozo の仕様を検査し、各言語のコードに展開する CLI（docs/cli.md）。
// 段階 3（docs/requirements.md「自己展開」）まで、引数の解釈は internal/app.Main が持ち、ここはそれを呼ぶだけ
// （docs/cli.md「手で書くもの」）。段階 3 で cli.hozo から生成した骨組みに置き換える。
package main

import (
	"os"

	"github.com/mew-ton/hozo/internal/app"
)

func main() {
	os.Exit(app.Run())
}
