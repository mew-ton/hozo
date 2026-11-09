// Command hozo-ls は Hozo の言語サーバー（LSP）。
// 提供する機能は docs/requirements.md の「言語サーバー」にある。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "hozo-ls: not implemented")
	os.Exit(64)
}
