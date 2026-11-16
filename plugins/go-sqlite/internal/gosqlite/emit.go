package gosqlite

import (
	"errors"
	"fmt"
	"path"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// 診断のコード（docs/plugin-go-sqlite.md「診断」）。GOSQL-5 は欠番。
const (
	codeSQLElement   = "GOSQL-1" // @db の状態の要素が model でない
	codeSQLShape     = "GOSQL-2" // @db の状態の形が list でない
	codeSQLColumn    = "GOSQL-3" // 列にできない型のフィールドがある
	codeSQLCollision = "GOSQL-4" // 公開の名前が当たる
	codeSQLOption    = "GOSQL-6" // [emit.options.go-sqlite] に知らないキーがある
)

// 読む語彙（docs/vocab.md「同梱する語彙」の db）のデコレータの FQN。
const (
	decoratorTable  = "db"
	decoratorKey    = "db.key"
	decoratorUnique = "db.unique"
	decoratorRef    = "db.ref"
)

// Emit は、要求の View の出力 (U, N) の db.go を作る（docs/plugin-go-sqlite.md）。root はプロジェクトのルート（go.mod を探す起点）。
// 診断があれば、ファイルは返さない。誤り（error）を返すのは、利用者の仕様の誤りではないとき（View が欠けている、生成したコードを
// 整形できない、型の置き場がない）で、プラグインは終了コード 1 で終わる。
func Emit(req *plugin.EmitRequest, root string) (*plugin.EmitResponse, error) {
	view := req.View
	if view == nil || view.Program == nil || view.Program.Namespace(view.Viewer) == nil {
		viewer := ""
		if view != nil {
			viewer = view.Viewer
		}
		return nil, fmt.Errorf("emit: View に、名前空間 %q の Program がない", viewer)
	}
	e := &emitter{view: view, root: root, p: view.Program, ns: view.Program.Namespace(view.Viewer)}
	e.prepare()

	// GOSQL-6: オプションはない
	var none struct{}
	unknown, err := plugin.Options(req, &none)
	if err != nil {
		e.errorf(codeSQLOption, "", "[emit.options.go-sqlite] を読めない: %v", err)
	}
	for _, key := range unknown {
		e.errorf(codeSQLOption, "", "%d 番目の [[emit]] の [emit.options.go-sqlite] に、知らないキー %s がある（go-sqlite にオプションはない）", view.Config.Index, key)
	}

	marked := e.collectTables()
	if marked == 0 && len(e.errs) == 0 {
		// 表の状態が一つもない: ファイルを出さず、warning を一つ（docs/plugin-go-sqlite.md「出力」）
		return &plugin.EmitResponse{Warnings: []plugin.Warning{{
			At: view.Viewer, Message: fmt.Sprintf("名前空間 %s に @db の付いた状態がないので、%s を出していない", view.Viewer, FileName),
		}}}, nil
	}
	e.checkNames()
	if len(e.errs) > 0 {
		return &plugin.EmitResponse{Errors: e.errs}, nil
	}

	layout, err := golang.LayoutOf(view, root)
	if err != nil {
		if errors.Is(err, golang.ErrNoGoMod) {
			// Dir(U, N) の上に go.mod がない。報告するのは基底（GO-1。docs/plugin-go.md「診断」）で、ここは何も出さない
			return &plugin.EmitResponse{}, nil
		}
		return nil, fmt.Errorf("emit: 名前空間 %s の出力先 %q の import パスを求められない: %w", view.Viewer, view.Config.Dir, err)
	}
	e.layout = layout
	e.matchOps()
	content, err := e.generate()
	if err != nil {
		return nil, err
	}
	return &plugin.EmitResponse{
		Files:    []plugin.File{{Path: path.Join(layout.Dir, FileName), Content: content}},
		Warnings: e.warns,
	}, nil
}

// emitter は一つの出力 (U, N) の db.go を作る状態。
type emitter struct {
	view   *ir.View
	root   string
	layout *golang.Layout
	shape  *golang.Shape
	p      *ir.Program
	ns     *ir.Namespace // N

	declNS map[string]string // 宣言の FQN → 宣言した名前空間

	tables  []*table          // 表の状態（宣言順）
	models  []*model          // 表の状態の要素の model（表の状態の順。同じ model は一度）
	byModel map[string]*model // model の FQN → models の要素

	errs  []plugin.Diagnostic
	warns []plugin.Warning
}

func (e *emitter) errorf(code, at, format string, args ...any) {
	e.errs = append(e.errs, plugin.Diagnostic{Code: code, At: at, Message: fmt.Sprintf(format, args...)})
}

// prepare は、View から、生成の間ずっと使う表を作る。
func (e *emitter) prepare() {
	e.shape = golang.ShapeOf(e.view)
	e.declNS = map[string]string{}
	for i := range e.p.Namespaces {
		for _, d := range e.p.Namespaces[i].Decls {
			if d == nil {
				continue
			}
			if _, dup := e.declNS[d.Common().Name]; !dup {
				e.declNS[d.Common().Name] = e.p.Namespaces[i].Name
			}
		}
	}
	e.byModel = map[string]*model{}
}

// short は FQN の最後のセグメント。
func short(fqn string) string {
	for i := len(fqn) - 1; i >= 0; i-- {
		if fqn[i] == '.' {
			return fqn[i+1:]
		}
	}
	return fqn
}

// typeName は宣言 fqn の Go の型の名前（修飾なし）。
func (e *emitter) typeName(fqn string) string { return golang.TypeName(fqn, e.declNS[fqn]) }

// hasDecorator は、適用の並び apps に、デコレータ fqn（汎用デコレータの FQN。名前付きデコレータは展開済み）があるか。
func hasDecorator(apps []ir.Application, fqn string) bool {
	for _, a := range apps {
		if a.Decorator == fqn {
			return true
		}
	}
	return false
}
