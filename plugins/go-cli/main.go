// Command go-cli は、cli の語彙（vocab/cli.hozo）を Go の CLI の骨組みにするプラグイン
// （docs/plugin-go-cli.md）。標準入出力で JSON をやり取りする（docs/plugin-protocol.md「起動」）。
// 中身は internal/gocli。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugins/go-cli/internal/gocli"
)

func main() {
	os.Exit(plugin.Main(gocli.Handler{}))
}
