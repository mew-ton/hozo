// Command db は、標準の語彙 db（docs/vocab.md「`db` の語彙」）の、語彙のプラグイン（docs/plugin-db.md）: 仕様を受け取り、
// その語彙の、言語にも保存先にも依らない決まりの診断（DB-*）を返す。読み解きの中身は、公開のライブラリ plugin/dbvocab
// （db の語彙を出すプラグイン go-sqlite と、同じもの）。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/dbvocab"
)

// reader は、このプラグインの plugin.Reader。
type reader struct{}

// Read は、仕様の、すべての名前空間の、表の状態と、db のデコレータを付けた read と効果を読み解いて、診断を返す
// （docs/plugin-db.md「対象」「診断」）。warning は、返さない。
func (reader) Read(req *plugin.ReadRequest) (*plugin.ReadResponse, error) {
	return &plugin.ReadResponse{Errors: dbvocab.Check(req.Spec.Program)}, nil
}

func main() {
	os.Exit(plugin.Main(plugin.ReadOnly(reader{})))
}
