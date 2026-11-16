// Command go-sqlite は、db の語彙（@db、@db.key、@db.unique、@db.ref）の付いた状態を、SQLite の表と、その上の読み書きにする
// プラグイン（docs/plugin-go-sqlite.md）。トランスパイラ go-sqlite を提供する: Go の基底 go に相乗りし、基底の状態の単位
// （<状態>State）を満たす型を db.go に出す。中身は internal/gosqlite。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugins/go-sqlite/internal/gosqlite"
)

func main() {
	os.Exit(plugin.Main(gosqlite.Handler{}))
}
