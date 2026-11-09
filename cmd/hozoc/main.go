// Command hozoc は Hozo の仕様を検査し、各言語のコードに展開する CLI。
// サブコマンドは docs/requirements.md の「作るもの」にある check / emit / import / extract。
package main

import (
	"fmt"
	"os"
)

// usage は引数が足りないときに出す。
const usage = `usage: hozoc <command> [args]

commands:
  check    仕様を検査する（未実装）
  emit     仕様を検査し、コードを書き出す（未実装）
  import   外部の形式から Hozo を作る（未実装）
  extract  実装から設計への取り込み候補を出す（未実装）
`

// 終了コードは examples/hozoc.hozo（設計書）の cli.exitCode に合わせる。
const (
	exitOK          = 0
	exitCheckFailed = 1 // CheckFailed
	exitConfig      = 2 // ConfigInvalid / LanguageMismatch
	exitConflict    = 3 // OutputConflict
	exitUsage       = 64
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(exitUsage)
	}
	switch os.Args[1] {
	case "check", "emit", "import", "extract":
		fmt.Fprintf(os.Stderr, "hozoc %s: not implemented\n", os.Args[1])
		os.Exit(exitUsage)
	default:
		fmt.Fprintf(os.Stderr, "hozoc: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(exitUsage)
	}
}
