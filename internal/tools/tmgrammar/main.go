// tmgrammar は editors/vscode/syntaxes/hozo.tmLanguage.json を生成する go generate の道具
// （docs/packages.md「生成するもの」、docs/editors.md「TextMate の文法」）。
//
// 予約語のパターンは token.Keywords から作り、それ以外のパターンは grammar.go に手で書く。
// 出力は決定的（キーの順序は固定）で、コミットした生成物との一致を CI が確かめる
// （docs/testing.md「生成物の一致」）。
//
//	go run ./internal/tools/tmgrammar -o editors/vscode/syntaxes/hozo.tmLanguage.json
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	out := flag.String("o", "", "書き出す先。省略すれば標準出力")
	flag.Parse()

	data, err := Generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tmgrammar:", err)
		os.Exit(1)
	}
	if *out == "" {
		os.Stdout.Write(data)
		return
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "tmgrammar:", err)
		os.Exit(1)
	}
}
