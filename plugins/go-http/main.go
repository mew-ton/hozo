// Command go-http は、http の語彙（docs/vocab.md「同梱する語彙」）の付いた op を net/http のハンドラにするプラグイン
// （docs/plugin-go-http.md）。トランスパイラ go-http を提供し、Go の基底 go と、検証の go-valid に相乗りする。中身は internal/gohttp。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugins/go-http/internal/gohttp"
)

func main() {
	os.Exit(plugin.Main(gohttp.Handler{}))
}
