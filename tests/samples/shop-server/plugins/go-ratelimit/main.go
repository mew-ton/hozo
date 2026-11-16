// Command go-ratelimit は、サンプル shop-server の中のプラグイン（../../README.md「サンプルの中のプラグイン」）。語彙 rate
// （rate.hozo）と、そのデコレータ @rate.limited の継ぎ目の中身を出すトランスパイラ go-ratelimit を提供する: Go の基底 go に
// 相乗りし、基底が出す Seams のメソッドのうち、自分が宣言した継ぎ目（マニフェストの seams）の分を、N の入口のパッケージの
// ratelimit.go に出す。標準入出力で JSON をやり取りする（docs/plugin-protocol.md「起動」）。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
)

func main() {
	os.Exit(plugin.Main(Handler{}))
}
