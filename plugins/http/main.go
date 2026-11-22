// Command http は、標準の語彙 http（docs/vocab.md「`http` の語彙」）の、語彙のプラグイン（docs/plugin-http.md）: 仕様を受け取り、
// その語彙の、言語に依らない決まりの診断（HTTP-*）を返す。読み解きの中身は、公開のライブラリ plugin/httpvocab
// （http の語彙を、ある言語に出すトランスパイラと、同じもの）。
package main

import (
	"os"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/httpvocab"
)

// reader は、このプラグインの plugin.Reader。
type reader struct{}

// Read は、仕様の、すべての名前空間の経路を読み解いて、診断を返す（docs/plugin-http.md「対象」「診断」）。warning は、返さない。
func (reader) Read(req *plugin.ReadRequest) (*plugin.ReadResponse, error) {
	return &plugin.ReadResponse{Errors: httpvocab.Check(req.Spec.Program)}, nil
}

func main() {
	os.Exit(plugin.Main(plugin.ReadOnly(reader{})))
}
