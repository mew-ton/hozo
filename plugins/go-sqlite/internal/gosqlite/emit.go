package gosqlite

import (
	"errors"
	"fmt"
	"path"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// 診断のコード（docs/plugin-go-sqlite.md「診断」）。欠番（GOSQL-5、GOSQL-14、GOSQL-18、GOSQL-19、GOSQL-21、GOSQL-23）は、
// 別の意味に使い回さない。
const (
	codeSQLElement     = "GOSQL-1"  // @db.table の状態の要素の型が、model でない
	codeSQLShape       = "GOSQL-2"  // @db.table の状態の形が list でない
	codeSQLColumn      = "GOSQL-3"  // どのしまい方でも列にできない型のフィールドがある
	codeSQLCollision   = "GOSQL-4"  // 公開の名前が当たる
	codeSQLOption      = "GOSQL-6"  // [emit.options.go-sqlite] に知らないキーがある
	codeSQLTable       = "GOSQL-7"  // read の on の状態か、効果の宛先の状態が、N の表の状態でない
	codeSQLName        = "GOSQL-8"  // 列を名指しするものが、表の model のフィールドにない
	codeSQLParams      = "GOSQL-9"  // 引数が、デコレータの言うものと合わない
	codeSQLReturns     = "GOSQL-10" // read の結果の型が、デコレータの言うものと合わない
	codeSQLTarget      = "GOSQL-11" // 効果のデコレータが、効果の宛先と合わない
	codeSQLEffect      = "GOSQL-12" // 一つの効果が、効果のデコレータを二つ以上持つ
	codeSQLUnique      = "GOSQL-13" // @db.find の read が、一行に決まらない
	codeSQLUpdate      = "GOSQL-15" // @db.update の表の model が、キーか、キーでない列を持たない
	codeSQLStorage     = "GOSQL-16" // フィールドが、しまい方のデコレータを、ちょうど一つ持たない
	codeSQLFit         = "GOSQL-17" // しまい方が、フィールドの型に合わない
	codeSQLRef         = "GOSQL-20" // @db.ref の指す先が、一つに決まらない
	codeSQLBoth        = "GOSQL-22" // 一つの read が、@db.find と @db.list の両方を持つ
	codeSQLNeither     = "GOSQL-24" // @db.where か @db.latest の付いた read が、@db.find も @db.list も持たない
	codeSQLTwice       = "GOSQL-25" // 一つの read の条件が、同じ列を二度名指ししている
	codeSQLLiteral     = "GOSQL-26" // @db.where のリテラルが、列の型に渡せない
	codeSQLLatest      = "GOSQL-27" // @db.latest の列のしまい方が、@db.time / @db.int / @db.float のどれでもない
	codeSQLDeleteAll   = "GOSQL-28" // @db.delete の効果が、引数を一つも持たない
	codeSQLTransaction = "GOSQL-29" // op に @db.transaction が付いているのに、N に表の状態が一つもない
)

// 読む語彙（docs/vocab.md「`db` の語彙」）のデコレータの FQN。しまい方のデコレータは storages（tables.go）。
const (
	decoratorTable       = "db.table"
	decoratorKey         = "db.key"
	decoratorUnique      = "db.unique"
	decoratorRef         = "db.ref"
	decoratorFind        = "db.find"
	decoratorList        = "db.list"
	decoratorWhere       = "db.where"
	decoratorLatest      = "db.latest"
	decoratorInsert      = "db.insert"
	decoratorUpdate      = "db.update"
	decoratorDelete      = "db.delete"
	decoratorTransaction = "db.transaction"
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

	// 書いたものの整合の検査（GOSQL-1〜GOSQL-3、GOSQL-7〜GOSQL-29）。食い違っていれば、黙って何かを出すことも、出さないこともしない
	marked := e.collectTables()
	e.collectReads()
	e.collectEffects()
	e.collectSeams()
	if marked == 0 {
		// 表の状態が一つもない。継ぎ目の実装を出せないので、@db.transaction の付いた op があれば GOSQL-29
		for _, s := range e.seams {
			e.errorf(codeSQLTransaction, s.Op, "op %s に @db.transaction が付いているが、名前空間 %s に @db.table の付いた状態が一つもない（継ぎ目の実装を出す %s がない）",
				s.Op, view.Viewer, FileName)
		}
		if len(e.errs) == 0 {
			// ファイルを出さず、warning を一つ（docs/plugin-go-sqlite.md「出力」）
			return &plugin.EmitResponse{Warnings: []plugin.Warning{{
				At: view.Viewer, Message: fmt.Sprintf("名前空間 %s に @db.table の付いた状態がないので、%s を出していない", view.Viewer, FileName),
			}}}, nil
		}
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

	reads   map[string]*read   // read の FQN → @db.find / @db.list の言うこと（db のデコレータを持つ read だけ）
	effects map[string]*effect // 効果の FQN → 効果のデコレータの言うこと（db のデコレータを持つ効果だけ）
	seams   []golang.Seam      // このトランスパイラが中身を出す継ぎ目（基底の Seams の並びのまま）
	// allSeams は、基底の Seams のメソッドが、すべて、このトランスパイラのものか（var _ Seams を出すか）。
	allSeams bool
	units    []*unit // 出す型（ファイルの中の順）

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
	e.reads, e.effects = map[string]*read{}, map[string]*effect{}
}

// collectSeams は、このトランスパイラが中身を出す継ぎ目（docs/plugin-go-sqlite.md「トランザクション」の「対象」）: 基底の Seams の
// メソッド（golang.ShapeOf(view).Seams）のうち、宣言したトランスパイラが go-sqlite のもの。並びは、基底のまま。
func (e *emitter) collectSeams() {
	for _, s := range e.shape.Seams {
		if s.Transpiler == TranspilerName {
			e.seams = append(e.seams, s)
		}
	}
	e.allSeams = len(e.seams) == len(e.shape.Seams)
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

// decoratorNames は、適用の並び apps のデコレータの名前を、`@db.find、@db.list` の形でつないだもの。
func decoratorNames(apps []ir.Application) string {
	out := ""
	for i, a := range apps {
		if i > 0 {
			out += "、"
		}
		out += "@" + a.Decorator
	}
	return out
}

// refArg は、適用 a の引数 param に渡した宣言の FQN。渡していないか、宣言への参照でなければ ""。
func refArg(a ir.Application, param string) string {
	for _, arg := range a.Args {
		if r, ok := arg.Value.(ir.RefValue); ok && arg.Param == param {
			return r.Target
		}
	}
	return ""
}

// literalArg は、適用 a の引数 param に渡したリテラル。渡していないか、リテラルでなければ nil。
func literalArg(a ir.Application, param string) ir.Literal {
	for _, arg := range a.Args {
		if l, ok := arg.Value.(ir.LiteralValue); ok && arg.Param == param {
			return l.Literal
		}
	}
	return nil
}

// stringArg は、適用 a の引数 param に渡した文字列。渡していないか、文字列でなければ ""。
func stringArg(a ir.Application, param string) string {
	if s, ok := literalArg(a, param).(ir.StringLiteral); ok {
		return s.Value
	}
	return ""
}
