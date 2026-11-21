// Command go-design は、設計の図の語彙（docs/vocab.md「設計の図の語彙」）から、テストを受け取る口を出すプラグイン
// （docs/plugin-go-design.md）。トランスパイラ go-design を提供し、Go の基底 go に相乗りする。中身は internal/godesign。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugins/go-design/internal/godesign"
)

func main() {
	os.Exit(plugin.Main(godesign.Handler{}))
}
