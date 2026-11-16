// Package gosqlite は go-sqlite プラグインの中身（docs/plugin-go-sqlite.md）。
//
// Handler が plugin.Handler を実装し、View（出力ごとに一つ。見る側は出力する名前空間 N）から、N の入口のパッケージの db.go を
// 作る: @db.table の付いた状態（表の状態）ごとの表（Schema、CreateTables）と、基底 go の状態の単位（<状態>State）を満たす型
// （<状態>Table）と、@db.transaction の継ぎ目の実装（DbSeams）。出すものは、付ける先で二つに分かれる（docs/requirements.md
// 「出力を推論しない」）: op 以外（状態、表の model のフィールド、read、効果）は、宣言の全体から中身を作り、デコレータは、宣言が
// 言っていないことだけを言う。op（@db.transaction）は、中身を決めず、前後にトランザクションの始めと終わりを足すだけで、出すコードは
// 型に依らない。基底の形（置き場、名前、型、状態の単位のメソッド、継ぎ目）は、公開の SDK plugin/golang で求める。基底の応答は
// 見ない（docs/plugin-protocol.md「相乗りが基底の形を知る方法」）。
package gosqlite

import (
	"fmt"
	"os"

	"github.com/mew-ton/hozo/plugin"
)

// TranspilerName は、このプラグインが提供するただ一つのトランスパイラの名前（docs/plugin-go-sqlite.md「マニフェスト」）。
const TranspilerName = "go-sqlite"

// FileName は、このトランスパイラが、入口のパッケージに置くファイルの名前（docs/plugin-go.md「相乗りのファイル」）。
const FileName = "db.go"

// Handler は go-sqlite の plugin.Handler。emit だけを実装する。
type Handler struct {
	// Getenv は環境変数を引く関数。nil なら os.Getenv。HOZO_PROJECT_ROOT（docs/plugin-protocol.md「起動」）を、型のパッケージの
	// import パスを求めるルートに使う。空なら作業ディレクトリ（= プロジェクトのルート）。
	Getenv func(string) string
}

var _ plugin.Handler = Handler{}

// Emit は View.viewer の名前空間の db.go を作る（docs/plugin-go-sqlite.md）。診断（GOSQL-*）があれば Files は空で、Errors に
// それを入れる。要求の transpiler が [TranspilerName] でなければ plugin.ErrUnsupported（code "unsupported"）。
func (h Handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req == nil || req.View == nil {
		return nil, fmt.Errorf("emit: request has no view")
	}
	if req.Transpiler != TranspilerName {
		return nil, fmt.Errorf("transpiler %q is not provided by go-sqlite (it provides %q): %w", req.Transpiler, TranspilerName, plugin.ErrUnsupported)
	}
	getenv := h.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	root := getenv("HOZO_PROJECT_ROOT")
	if root == "" {
		root = "."
	}
	return Emit(req, root)
}

// Check は実装しない（語彙に固有の検査は、emit の診断で返す）。
func (Handler) Check(*plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}
