// Command valid は、標準の語彙 valid（docs/vocab.md「同梱する語彙」）の、語彙のプラグイン（docs/plugin-valid.md）: 仕様を受け取り、
// その語彙の、言語に依らない決まりの診断（VALID-*）を返す。読み解きの中身は、公開のライブラリ plugin/validvocab
// （valid の制約を読む、ほかのプラグインと、同じもの）。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/validvocab"
)

// reader は、このプラグインの plugin.Reader。
type reader struct{}

// Read は、仕様の、すべての名前空間の scalar と model のフィールドに付いた制約を読み解いて、診断を返す
// （docs/plugin-valid.md「対象」「診断」）。warning は、返さない。
func (reader) Read(req *plugin.ReadRequest) (*plugin.ReadResponse, error) {
	return &plugin.ReadResponse{Errors: validvocab.Check(req.Spec.Program)}, nil
}

func main() {
	os.Exit(plugin.Main(plugin.ReadOnly(reader{})))
}
