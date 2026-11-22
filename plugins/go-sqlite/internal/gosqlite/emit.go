package gosqlite

import (
	"errors"
	"fmt"
	"path"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/dbvocab"
	"github.com/mew-ton/hozo/plugin/golang"
)

// 診断のコード（docs/plugin-go-sqlite.md「診断」）。欠番（語彙のプラグイン db に移した 19 個と、GOSQL-5、GOSQL-14、GOSQL-18、GOSQL-19、
// GOSQL-21、GOSQL-23）は、使い回さない。
const (
	codeSQLColumn      = "GOSQL-3"  // どのしまい方でも列にできない型のフィールドがある
	codeSQLCollision   = "GOSQL-4"  // 公開の名前が当たる
	codeSQLOption      = "GOSQL-6"  // [emit.options.go-sqlite] に知らないキーがある
	codeSQLStorage     = "GOSQL-16" // フィールドが、しまい方のデコレータを、ちょうど一つ持たない
	codeSQLFit         = "GOSQL-17" // しまい方が、フィールドの型に合わない
	codeSQLLatest      = "GOSQL-27" // @db.latest の列のしまい方が、@db.datetime / @db.date / @db.time / @db.int / @db.float のどれでもない
	codeSQLTransaction = "GOSQL-29" // op に @db.transaction が付いているのに、N に表の状態が一つもない
	codeSQLThrows      = "GOSQL-33" // db のデコレータで中身を作る効果の throws に、@db.conflict が名指ししていない例外がある
)

// 出力で名指しする、db の語彙のデコレータの FQN（語彙の持ち主は plugin/dbvocab）。
const (
	decoratorFind        = dbvocab.Find
	decoratorInsert      = dbvocab.Insert
	decoratorUpdate      = dbvocab.Update
	decoratorDelete      = dbvocab.Delete
	decoratorConflict    = dbvocab.Conflict
	decoratorTransaction = dbvocab.Transaction
)

// Emit は、要求の View の出力 (U, N) の db.go を作る（docs/plugin-go-sqlite.md）。読むのは、要求だけ（ファイルも、環境変数も、読まない）。
// 診断があれば、ファイルは返さない。誤り（error）を返すのは、利用者の仕様の誤りではないとき（View が欠けている、生成したコードを
// 整形できない、型の置き場がない）で、プラグインは終了コード 1 で終わる。
func Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	view := req.View
	if view == nil || view.Program == nil || view.Program.Namespace(view.Viewer) == nil {
		viewer := ""
		if view != nil {
			viewer = view.Viewer
		}
		return nil, fmt.Errorf("emit: View に、名前空間 %q の Program がない", viewer)
	}
	e := &emitter{view: view, p: view.Program, ns: view.Program.Namespace(view.Viewer)}
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

	// db の語彙の、書いたものの整合の検査（DB-*。docs/plugin-db.md「診断」）は、語彙のプラグイン db が、先に断る。その検査を
	// 通っていない仕様を受け取ったら、診断にせず、誤りで終わる（docs/plugin-go-sqlite.md「診断」）
	e.vocab = dbvocab.ReadNamespace(e.p, view.Viewer)
	if errs := e.vocab.Errors; len(errs) > 0 {
		return nil, fmt.Errorf("emit: db の語彙の検査（%s）を通っていない仕様を受け取った: %s", errs[0].Code, errs[0].Message)
	}
	marked := e.collectTables()
	e.collectReads()
	e.collectEffects()
	e.collectSeams()
	if marked == 0 {
		// 表の状態が一つもない。継ぎ目の実装を出せないので、@db.transaction の付いた op があれば GOSQL-29
		for _, s := range e.seams {
			e.errorf(codeSQLTransaction, s.Op, "op %s に @%s が付いているが、名前空間 %s に @db.table の付いた状態が一つもない（継ぎ目の実装を出す %s がない）",
				s.Op, s.Decorator, view.Viewer, FileName)
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

	layout, err := golang.LayoutOf(view)
	if err != nil {
		if errors.Is(err, golang.ErrNoModule) {
			// 基底のオプション module がない。報告するのは基底（GO-8。docs/plugin-go.md「診断」）で、ここは何も出さない
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
	layout *golang.Layout
	shape  *golang.Shape
	p      *ir.Program
	ns     *ir.Namespace // N

	declNS map[string]string // 宣言の FQN → 宣言した名前空間

	vocab *dbvocab.Reading // N の、db の語彙の読み解き（表の状態、表の model、read と効果）

	tables  []*table          // 表の状態（宣言順）
	byState map[string]*table // 表の状態の FQN → tables の要素
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
	e.byState, e.byModel = map[string]*table{}, map[string]*model{}
	e.reads, e.effects = map[string]*read{}, map[string]*effect{}
}

// collectSeams は、このトランスパイラが中身を出す継ぎ目（docs/plugin-go-sqlite.md「トランザクション」の「対象」）: 基底の Seams の
// メソッド（golang.ShapeOf(view).Seams）のうち、宣言したトランスパイラが go-sqlite のもの（マニフェストが宣言する継ぎ目は
// @db.transaction だけ）。並びは、基底のまま。
func (e *emitter) collectSeams() {
	for _, s := range e.shape.Seams {
		if s.Transpiler == TranspilerName && s.Decorator == decoratorTransaction {
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
