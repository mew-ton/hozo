// Command go は Go の基底のプラグイン（docs/plugin-go.md）。トランスパイラ go を提供する: 語彙を知らなくても、仕様と設定だけから
// 決まるもの（型、op の形、実装を受け取る口、配線の本体、継ぎ目、New）をすべて出す。中身は internal/gobase。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugins/go/internal/gobase"
)

func main() {
	os.Exit(plugin.Main(gobase.Handler{}))
}
