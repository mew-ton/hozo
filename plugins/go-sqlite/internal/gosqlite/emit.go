package gosqlite

import (
	"errors"
	"fmt"
	"path"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// 診断のコード（docs/plugin-go-sqlite.md「診断」）。GOSQL-5 は欠番（番号は、別の意味に使い回さない）。
const (
	codeSQLElement   = "GOSQL-1"  // @db.table(M) の状態の要素の型が、M と等しくない
	codeSQLShape     = "GOSQL-2"  // @db.table の状態の形が list でない
	codeSQLColumn    = "GOSQL-3"  // どのしまい方でも列にできない型のフィールドがある
	codeSQLCollision = "GOSQL-4"  // 公開の名前が当たる
	codeSQLOption    = "GOSQL-6"  // [emit.options.go-sqlite] に知らないキーがある
	codeSQLTable     = "GOSQL-7"  // デコレータの引数の状態が、N の表の状態でない
	codeSQLName      = "GOSQL-8"  // デコレータの引数の列の名前が、表の model のフィールドにない
	codeSQLParams    = "GOSQL-9"  // op の引数が、デコレータの言うものと合わない
	codeSQLReturns   = "GOSQL-10" // op の戻り値が、デコレータの言うものと合わない
	codeSQLTarget    = "GOSQL-11" // 効果のデコレータが、効果の宛先と合わない
	codeSQLEffect    = "GOSQL-12" // 並べた効果が効果のデコレータを持たない。または、二つ以上持つ
	codeSQLUnique    = "GOSQL-13" // @db.find の列が、一行に決まる列でない
	codeSQLCarried   = "GOSQL-14" // op が、db の op のデコレータの言う以外のものを読み書きする
	codeSQLUpdate    = "GOSQL-15" // @db.update の表の model が、キーか、キーでない列を持たない
	codeSQLStorage   = "GOSQL-16" // フィールドが、しまい方のデコレータを、ちょうど一つ持たない
	codeSQLFit       = "GOSQL-17" // しまい方が、フィールドの型に合わない
	codeSQLNullable  = "GOSQL-18" // @db.nullable の付いたフィールドの型が、省略可能でない
	codeSQLOptional  = "GOSQL-19" // 省略可能なフィールドに、@db.nullable がない
	codeSQLRef       = "GOSQL-20" // @db.ref の指す先が、一つに決まらない
)

// 読む語彙（docs/vocab.md「`db` の読み書きのデコレータ」）のデコレータの FQN。しまい方のデコレータは storages（tables.go）。
const (
	decoratorTable    = "db.table"
	decoratorNullable = "db.nullable"
	decoratorKey      = "db.key"
	decoratorUnique   = "db.unique"
	decoratorRef      = "db.ref"
	decoratorFind     = "db.find"
	decoratorList     = "db.list"
	decoratorEffects  = "db.effects"
	decoratorInsert   = "db.insert"
	decoratorUpdate   = "db.update"
	decoratorDelete   = "db.delete"
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

	// 書いたものの整合の検査（GOSQL-1〜GOSQL-3、GOSQL-7〜GOSQL-20）。出力を、型に合わせて変えることはしない
	marked := e.collectTables()
	e.collectEffects()
	e.collectOps()
	if marked == 0 && len(e.errs) == 0 {
		// 表の状態が一つもない: ファイルを出さず、warning を一つ（docs/plugin-go-sqlite.md「出力」）
		return &plugin.EmitResponse{Warnings: []plugin.Warning{{
			At: view.Viewer, Message: fmt.Sprintf("名前空間 %s に @db.table の付いた状態がないので、%s を出していない", view.Viewer, FileName),
		}}}, nil
	}
	e.collectUnits()
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
	content, err := e.generate()
	if err != nil {
		return nil, err
	}
	return &plugin.EmitResponse{Files: []plugin.File{{Path: path.Join(layout.Dir, FileName), Content: content}}}, nil
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
	byState map[string]*table // 表の状態の FQN → tables の要素
	marked  map[string]bool   // @db.table の付いた N の状態（診断を返して、表にならなかったものを含む）
	models  []*model          // 表の model（表の状態の順。同じ model は一度）
	byModel map[string]*model // model の FQN → models の要素

	effects map[string]*effect // 効果の FQN → 効果のデコレータの言うこと（collectEffects、effectOf）
	methods map[string]*method // op の FQN → db の op のデコレータの言うこと（デコレータを持つ op だけ）
	units   []*unit            // 出す型（ファイルの中の順）

	errs []plugin.Diagnostic
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
	e.byState, e.marked, e.byModel = map[string]*table{}, map[string]bool{}, map[string]*model{}
	e.effects, e.methods = map[string]*effect{}, map[string]*method{}
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

// applications は、適用の並び apps のうち、デコレータ（汎用デコレータの FQN。名前付きデコレータは展開済み）が names のどれかであるもの。
// 書かれた順。
func applications(apps []ir.Application, names ...string) []ir.Application {
	var out []ir.Application
	for _, a := range apps {
		for _, n := range names {
			if a.Decorator == n {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// hasDecorator は、適用の並び apps に、デコレータ fqn があるか。
func hasDecorator(apps []ir.Application, fqn string) bool { return len(applications(apps, fqn)) > 0 }

// refArg は、適用 a の引数 param に渡した宣言の FQN。渡していないか、宣言への参照でなければ ""。
func refArg(a ir.Application, param string) string {
	for _, arg := range a.Args {
		if r, ok := arg.Value.(ir.RefValue); ok && arg.Param == param {
			return r.Target
		}
	}
	return ""
}

// stringArg は、適用 a の引数 param に渡した文字列。ok は、渡したか（省いた省略可能な引数は、Args にない）。
func stringArg(a ir.Application, param string) (value string, ok bool) {
	for _, arg := range a.Args {
		if arg.Param != param {
			continue
		}
		if l, isLit := arg.Value.(ir.LiteralValue); isLit {
			if s, isString := l.Literal.(ir.StringLiteral); isString {
				return s.Value, true
			}
		}
	}
	return "", false
}
