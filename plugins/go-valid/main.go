// Command go-valid は、valid の語彙（docs/vocab.md「同梱する語彙」）の制約を確かめる Go の関数を出すプラグイン
// （docs/plugin-go-valid.md）。トランスパイラ go-valid を提供し、Go の基底 go に相乗りする。中身は internal/govalid。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugins/go-valid/internal/govalid"
)

func main() {
	os.Exit(plugin.Main(govalid.Handler{}))
}
