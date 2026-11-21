package gosqlite

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは docs/plugin-go-sqlite.md「read の中身」「効果の中身」「列と引数の型」「状態ごとの型」: read と効果に付けた db の語彙の
// デコレータの言うことと、それを、どの型のメソッドにするか。read と効果は op ではないので、中身は、宣言の全体（on の状態、引数の
// 名前と型、結果の型）から作る。デコレータが言うのは、宣言が言っていないことだけ（docs/requirements.md「出力を推論しない」）。
// 書いたものが食い違っていれば、出力を合わせて変えずに、診断を返す（GOSQL-7〜GOSQL-13、GOSQL-15、GOSQL-22〜GOSQL-28）。

// condition は、条件一つ: 列 column が、引数 param（read か、@db.delete の効果の引数）か、リテラル literal（@db.where）と同じ。
type condition struct {
	column  *column
	param   *ir.OpParam // 引数の条件。@db.where なら nil
	literal ir.Literal  // @db.where のリテラル。引数の条件なら nil
}

// read は、db のデコレータを付けた read 一つと、そのデコレータの言うこと。
type read struct {
	decl *ir.Read
	// apps は、付いた db のデコレータ（コメントに書く順: @db.find / @db.list、@db.where、@db.latest。@db.where は一度だけ）。
	apps []ir.Application
	// kind は @db.find か @db.list の FQN。
	kind  string
	table *table
	// conds は条件（引数を宣言の順に、その後ろに @db.where を書いた順に）。latest は @db.latest の列（付いていなければ nil）。
	conds  []condition
	latest *column
	// ok は、食い違いがない（メソッドにできる）か。
	ok bool
}

// effect は、db のデコレータを付けた効果一つと、そのデコレータの言うこと。
type effect struct {
	decl *ir.Effect
	// kind は、ただ一つの効果のデコレータ（@db.insert / @db.update / @db.delete）の FQN。
	kind  string
	table *table
	// conds は、@db.delete の条件（引数を宣言の順に）。
	conds []condition
	// conflict は、@db.conflict が名指しした例外の FQN（一意の制約に当たったら、返す）。付けていなければ ""。
	conflict string
	// ok は、食い違いがない（メソッドにできる）か。
	ok bool
}

// tableOf は、状態 state の表。N の表の状態でなければ GOSQL-7（位置は、デコレータを付けた宣言 at。what は、その種類と、状態の
// 言い方: 「read store.x の reads の状態」）。read の reads の状態も、効果の宛先の状態も、宣言した名前空間の状態（docs/check.md「read の宛先」
// 「効果の宛先」）なので、表の状態でないのは、@db.table が付いていない状態。@db.table は付いているが、表にならなかった状態
// （GOSQL-1 か GOSQL-2 を返している）は、重ねて報告しない。
func (e *emitter) tableOf(state, what, at string) *table {
	if t := e.byState[state]; t != nil {
		return t
	}
	if !e.marked[state] {
		e.errorf(codeSQLTable, at, "%s %s が、名前空間 %s の表の状態でない（@db.table が付いていない）", what, state, e.view.Viewer)
	}
	return nil
}

// columnOf は、表 t の model の、名前が name の列。なければ GOSQL-8（位置は、デコレータを付けた宣言 at。what は、列を名指しした
// ものの言い方: 「read store.x の引数 email」「read store.x の @db.where の列 "paid"」）。
func (e *emitter) columnOf(t *table, name, what, at string) *column {
	if c := t.model.column(name); c != nil {
		return c
	}
	e.errorf(codeSQLName, at, "%s が、表 %s の model %s のフィールドにない（列は、引数の名前とデコレータの column で名指しする）", what, t.state.Name, t.model.decl.Name)
	return nil
}

// paramConditions は、引数の条件（docs/plugin-go-sqlite.md「read の中身」「効果の中身」の @db.delete）: 引数ごとに、その名前の列が、
// 引数の値と同じ。名前のフィールドがなければ GOSQL-8、型が列に合わなければ GOSQL-9（「列と引数の型」: 引数の型は、列の型と等しい。
// 列の型が T? なら、T でもよい。列より広い引数は合わない）。what は、宣言の言い方（「read store.x」）。
func (e *emitter) paramConditions(t *table, params []ir.OpParam, what, at string) []condition {
	var out []condition
	for i := range params {
		p := &params[i]
		c := e.columnOf(t, p.Name, what+" の引数 "+p.Name, at)
		if c == nil {
			continue
		}
		out = append(out, condition{column: c, param: p})
		if ir.TypeEqual(p.Type, c.typ) {
			continue
		}
		if o, optional := c.typ.(ir.OptionalType); optional && ir.TypeEqual(p.Type, o.Of) {
			continue
		}
		e.errorf(codeSQLParams, at, "%s の引数 %s の型 %s が、列 %q の型 %s に合わない（列の型と等しいか、列が T? なら T）",
			what, p.Name, golang.TypeDisplay(p.Type), c.name, golang.TypeDisplay(c.typ))
	}
	return out
}

// collectReads は、N の read のうち、db のデコレータ（@db.find、@db.list、@db.where、@db.latest）を付けたものについて、宣言と
// デコレータの言うことを確かめる（e.reads）。どの診断も、位置は read。付けていない read には、何も出さない（warning も出さない）。
func (e *emitter) collectReads() {
	for _, d := range e.ns.Decls {
		decl, ok := d.(*ir.Read)
		if !ok {
			continue
		}
		kinds := applications(decl.Decorators, decoratorFind, decoratorList)
		wheres := applications(decl.Decorators, decoratorWhere)
		latests := applications(decl.Decorators, decoratorLatest)
		if len(kinds)+len(wheres)+len(latests) == 0 {
			continue
		}
		fqn := decl.Name
		switch {
		case len(kinds) > 1:
			e.errorf(codeSQLBoth, fqn, "read %s が、@db.find と @db.list の両方を持つ（一行を引くか、行の並びを引くかの、どちらか一つ）", fqn)
			continue
		case len(kinds) == 0:
			extra := latests
			if len(wheres) > 0 {
				extra = append([]ir.Application{wheres[0]}, latests...)
			}
			e.errorf(codeSQLNeither, fqn, "read %s に %s が付いているが、@db.find も @db.list も付いていない（条件と並べ方は、引くものを言うデコレータに足す）",
				fqn, decoratorNames(extra))
			continue
		}
		r := &read{decl: decl, kind: kinds[0].Decorator, apps: kinds}
		if len(wheres) > 0 {
			r.apps = append(r.apps, wheres[0])
		}
		r.apps = append(r.apps, latests...)
		e.reads[fqn] = r
		before := len(e.errs)
		what := "read " + fqn
		r.table = e.tableOf(decl.State, what+" の reads の状態", fqn)
		if r.table == nil {
			continue
		}
		m := r.table.model

		// 条件: 引数（宣言の順）の後ろに、@db.where（書いた順）
		r.conds = e.paramConditions(r.table, decl.Params, what, fqn)
		named := len(decl.Params) + len(wheres) // 条件が名指しした列の数（フィールドになかったものを含む）
		for _, w := range wheres {
			name, lit := stringArg(w, "column"), literalArg(w, "value")
			c := e.columnOf(r.table, name, what+" の @db.where の列 "+strconv.Quote(name), fqn)
			if c == nil {
				continue
			}
			r.conds = append(r.conds, condition{column: c, literal: lit})
			if !e.literalFits(lit, c) {
				e.errorf(codeSQLLiteral, fqn, "read %s の @db.where の列 %q に、リテラル %s を渡せない（列の型は %s。%s）",
					fqn, name, literalText(lit), golang.TypeDisplay(c.typ), e.literalHint(c))
			}
		}
		// GOSQL-25: 一つの列を、二つの条件に書けない
		seen, twice := map[*column]bool{}, map[*column]bool{}
		for _, cond := range r.conds {
			if seen[cond.column] && !twice[cond.column] {
				twice[cond.column] = true
				e.errorf(codeSQLTwice, fqn, "read %s の条件が、列 %q を二度名指ししている（一つの列に書ける条件は、引数か @db.where の、どちらか一つ）", fqn, cond.column.name)
			}
			seen[cond.column] = true
		}

		// 並べ方: @db.latest の列は、新しい順を比べられるしまい方
		if len(latests) > 0 {
			name := stringArg(latests[0], "column")
			if c := e.columnOf(r.table, name, what+" の @db.latest の列 "+strconv.Quote(name), fqn); c != nil {
				r.latest = c
				if s := c.store; s != nil && s.decorator != "db.datetime" && s.decorator != "db.date" && s.decorator != "db.time" && s.decorator != "db.int" && s.decorator != "db.float" {
					e.errorf(codeSQLLatest, fqn, "read %s の @db.latest の列 %q のしまい方が @%s（新しい順に並べられるのは @db.datetime / @db.date / @db.time / @db.int / @db.float の列）", fqn, name, s.decorator)
				}
			}
		}

		// 引くもの: @db.find は M?、@db.list は M[]
		row := ir.Ref(m.decl.Name)
		want := ir.Type(ir.ArrayType{Of: row})
		if r.kind == decoratorFind {
			want = ir.OptionalType{Of: row}
		}
		if !ir.TypeEqual(decl.Returns, want) {
			e.errorf(codeSQLReturns, fqn, "read %s の結果の型 %s が、@%s の言うもの（%s。表 %s の model の行）でない", fqn, golang.TypeDisplay(decl.Returns), r.kind, golang.TypeDisplay(want), r.table.state.Name)
		}

		// GOSQL-13: @db.find は、一行に決まる。条件の列がフィールドになかった（GOSQL-8 を返した）なら、決まるかどうかは言えない
		if r.kind == decoratorFind && len(latests) == 0 && len(r.conds) == named && !single(m, seen) {
			e.errorf(codeSQLUnique, fqn, "read %s（@db.find）が、一行に決まらない: 条件の列が、model %s の @db.key のすべても、@db.unique の列も含まず、@db.latest もない（並びを引くなら @db.list）",
				fqn, m.decl.Name)
		}
		r.ok = len(e.errs) == before
	}
}

// single は、条件の列 cols が、model m の行を一つに決めるか（docs/plugin-go-sqlite.md「read の中身」の 1 と 2）: m の @db.key の
// フィールドをすべて含む（m が @db.key を一つ以上持つとき）か、@db.unique のフィールドを一つ以上含む。
func single(m *model, cols map[*column]bool) bool {
	keys := m.keys()
	all := len(keys) > 0
	for _, k := range keys {
		all = all && cols[k]
	}
	if all {
		return true
	}
	for c := range cols {
		if c.unique {
			return true
		}
	}
	return false
}

// literalFits は、@db.where のリテラル lit を、列 c の型に渡せるか（docs/plugin-go-sqlite.md「read の中身」）: 列のフィールドの型
// （scalar は元の核の型。省略可能は外して見る）に、docs/types.md「互換」の 10 で渡せる（整数のリテラルは float にも）。
// datetime、date、time、duration、array、record、model の列には、渡せるリテラルがない。
func (e *emitter) literalFits(lit ir.Literal, c *column) bool {
	inner := c.typ
	if o, ok := inner.(ir.OptionalType); ok {
		inner = o.Of
	}
	class := e.typeClass(inner)
	switch lit.(type) {
	case ir.StringLiteral:
		return class == "string"
	case ir.IntLiteral:
		return class == "int" || class == "float"
	case ir.FloatLiteral:
		return class == "float"
	case ir.BooleanLiteral:
		return class == "boolean"
	}
	return false
}

// literalHint は、列 c に渡せるリテラルの言い方（GOSQL-26 のメッセージ用）。
func (e *emitter) literalHint(c *column) string {
	inner := c.typ
	if o, ok := inner.(ir.OptionalType); ok {
		inner = o.Of
	}
	switch e.typeClass(inner) {
	case "string":
		return "渡せるのは文字列のリテラル"
	case "int":
		return "渡せるのは整数のリテラル"
	case "float":
		return "渡せるのは整数か小数のリテラル"
	case "boolean":
		return "渡せるのは true か false"
	}
	return "この型の列に渡せるリテラルはない"
}

// literalText は、リテラル lit の、仕様での書き方（docs/grammar.md「リテラル」）。コメントと、診断のメッセージに書く。
// IR が持つのは値なので、値から書き直す: 文字列は `"…"`（`"` と `\` と改行などはエスケープ）、整数は 10 進、小数は、いつも小数点を持つ形。
func literalText(lit ir.Literal) string {
	switch l := lit.(type) {
	case ir.StringLiteral:
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range l.Value {
			switch {
			case r == '"' || r == '\\':
				b.WriteByte('\\')
				b.WriteRune(r)
			case r == '\n':
				b.WriteString(`\n`)
			case r == '\t':
				b.WriteString(`\t`)
			case r == '\r':
				b.WriteString(`\r`)
			case r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029:
				b.WriteString(`\u{` + strconv.FormatInt(int64(r), 16) + `}`)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		return b.String()
	case ir.IntLiteral:
		return strconv.FormatInt(l.Value, 10)
	case ir.FloatLiteral:
		s := strconv.FormatFloat(l.Value, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	case ir.BooleanLiteral:
		return strconv.FormatBool(l.Value)
	case ir.DurationLiteral:
		return l.Value.String()
	}
	return "?"
}

// goLiteral は、@db.where のリテラル lit の Go での書き方（docs/plugin-go.md「値」の表の literal の行と同じ）。@db.float の列に
// 渡した整数のリテラルは、小数のリテラルで書く（`1.0`）。
func goLiteral(lit ir.Literal, c *column) string {
	float := func(v float64) string {
		s := strconv.FormatFloat(v, 'g', -1, 64)
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		return s
	}
	switch l := lit.(type) {
	case ir.StringLiteral:
		return strconv.Quote(l.Value)
	case ir.IntLiteral:
		if c.store != nil && c.store.decorator == "db.float" {
			return float(float64(l.Value))
		}
		return strconv.FormatInt(l.Value, 10)
	case ir.FloatLiteral:
		return float(l.Value)
	case ir.BooleanLiteral:
		return strconv.FormatBool(l.Value)
	}
	return "nil"
}

// collectEffects は、N の効果のうち、効果のデコレータ（@db.insert / @db.update / @db.delete）を付けたものについて、宣言とデコレータの
// 言うことを確かめる（e.effects。配線が呼んでいない効果も、同じに確かめる）。どの診断も、位置は効果。
func (e *emitter) collectEffects() {
	for _, d := range e.ns.Decls {
		decl, ok := d.(*ir.Effect)
		if !ok {
			continue
		}
		apps := applications(decl.Decorators, decoratorInsert, decoratorUpdate, decoratorDelete)
		fqn := decl.Name
		conflicts := applications(decl.Decorators, decoratorConflict)
		// GOSQL-31: @db.conflict は、@db.insert か @db.update と一緒に付ける
		if len(conflicts) > 0 && len(applications(decl.Decorators, decoratorInsert, decoratorUpdate)) == 0 {
			e.errorf(codeSQLConflictOn, fqn, "効果 %s の @db.conflict は、@db.insert か @db.update と一緒に付ける（一意の制約に当たりうるのは、行を足す文と、書き換える文だけ）", fqn)
			if len(apps) == 1 {
				// @db.delete の効果は、ほかの食い違いを重ねて報告しないように、メソッドにしないものとして、覚える
				e.effects[fqn] = &effect{decl: decl, kind: apps[0].Decorator}
			}
			continue
		}
		if len(apps) == 0 {
			continue
		}
		if len(apps) > 1 {
			e.errorf(codeSQLEffect, fqn, "効果 %s が、効果のデコレータを二つ以上持つ（%s。@db.insert / @db.update / @db.delete のうち、高々一つ）", fqn, decoratorNames(apps))
			continue
		}
		x := &effect{decl: decl, kind: apps[0].Decorator}
		e.effects[fqn] = x
		before := len(e.errs)
		what := "効果 " + fqn
		e.conflictOf(x, conflicts)

		// GOSQL-11: 宛先は、組み込みの効果で、操作が、デコレータと合う
		target, builtin := decl.Target.(ir.BuiltinTarget)
		if !builtin {
			e.errorf(codeSQLTarget, fqn, "効果 %s の @%s が、効果の宛先と合わない: 宛先が、分類の遷移（どの表に何をするかが、宣言から決まらない。この効果は、デコレータを付けずに、人が書く）", fqn, x.kind)
			continue
		}
		want := map[string]ir.Operation{decoratorInsert: ir.OpPost, decoratorUpdate: ir.OpPut, decoratorDelete: ir.OpDelete}[x.kind]
		if target.Operation != want {
			e.errorf(codeSQLTarget, fqn, "効果 %s の @%s が、効果の宛先 %s.%s の操作と合わない（@%s は %s）", fqn, x.kind, target.State, target.Operation, x.kind, want)
		}
		x.table = e.tableOf(target.State, what+" の宛先の状態", fqn)
		if x.table == nil {
			continue
		}
		m := x.table.model

		switch x.kind {
		case decoratorDelete:
			// 引数は、条件。一つ以上
			if len(decl.Params) == 0 {
				e.errorf(codeSQLDeleteAll, fqn, "効果 %s（@db.delete）が、引数を一つも持たない（すべての行を消す形は、まだ決めていない。消す行を、引数の名前の列で選ぶ）", fqn)
			}
			x.conds = e.paramConditions(x.table, decl.Params, what, fqn)
		default:
			// 引数は、表の model の行一つ
			if len(decl.Params) != 1 || !ir.TypeEqual(decl.Params[0].Type, ir.Ref(m.decl.Name)) {
				e.errorf(codeSQLParams, fqn, "効果 %s の引数（%s）が、@%s の受け取るもの（表 %s の model %s の行一つ）でない", fqn, paramsDisplay(decl.Params), x.kind, x.table.state.Name, m.decl.Name)
			}
		}

		// GOSQL-15: 書き換える行は @db.key で選び、キーでない列を書き換える
		if x.kind == decoratorUpdate {
			switch keys := len(m.keys()); {
			case keys == 0:
				e.errorf(codeSQLUpdate, fqn, "効果 %s の @db.update の表 %s の model %s が、@db.key を一つも持たない（書き換える行を選べない）", fqn, x.table.state.Name, m.decl.Name)
			case keys == len(m.cols):
				e.errorf(codeSQLUpdate, fqn, "効果 %s の @db.update の表 %s の model %s が、キーでない列を一つも持たない（書き換えるものがない）", fqn, x.table.state.Name, m.decl.Name)
			}
		}
		// GOSQL-30: @db.update は、渡った行を、そのまま書く。渡る行は、前のケースにある（STATE-9）ので、後のケースが
		// 前のケースの並びになければ、書いた後の行は、宣言した後のケースにならない
		if row := decl.Row; x.kind == decoratorUpdate && row != nil && !row.From.Any && row.To != "" && !containsCase(row.From.Cases, row.To) {
			e.errorf(codeSQLRow, fqn, "効果 %s の @db.update が、行の遷移（%s）と合わない: 渡る行は、前のケースにあり、@db.update は、その行を、そのまま書くので、書いた後の行は %s.%s にならない（行をそのケースにする書き方は、宣言から決まらない。この効果は、デコレータを付けずに、人が書く）",
				fqn, golang.RowDisplay(decl), row.Classify, row.To)
		}
		x.ok = len(e.errs) == before
	}
}

// conflictOf は、効果 x の @db.conflict（conflicts は、その適用。なければ空）と、効果の throws を確かめ、x.conflict を決める
// （docs/plugin-go-sqlite.md「効果の中身」）。GOSQL-32: 名指しした例外が、効果の throws にないか、引数を持つ。
// GOSQL-33: 効果の throws に、@db.conflict が名指ししていない例外がある（このトランスパイラは、それを投げるコードを出せない）。
func (e *emitter) conflictOf(x *effect, conflicts []ir.Application) {
	fqn := x.decl.Name
	if len(conflicts) > 0 {
		for _, exc := range refsArg(conflicts[0], "error") {
			switch decl := e.exception(exc); {
			case !containsCase(x.decl.Throws, exc):
				e.errorf(codeSQLConflictExc, fqn, "効果 %s の @db.conflict の例外 %s が、その効果の throws にない（effect fn %s(…): throws %s effects … と書く）", fqn, exc, short(fqn), short(exc))
			case decl != nil && len(decl.Params) > 0:
				e.errorf(codeSQLConflictExc, fqn, "効果 %s の @db.conflict の例外 %s は、引数を持つ（このトランスパイラは、値を埋められない。この効果は、デコレータを付けずに、人が書く）", fqn, exc)
			default:
				x.conflict = exc
			}
			break // 例外は、効果ごとに一つ（ユニオンを渡しても、最初の一つ）
		}
	}
	named := refsArg(firstOr(conflicts), "error")
	for _, exc := range x.decl.Throws {
		if len(named) > 0 && named[0] == exc {
			continue
		}
		e.errorf(codeSQLThrows, fqn, "効果 %s の throws の例外 %s を、このトランスパイラは投げられない（db のデコレータで中身を作る効果が投げるのは、@db.conflict が名指しした例外だけ）", fqn, exc)
	}
}

// exception は、FQN が fqn の例外の宣言。なければ nil。
func (e *emitter) exception(fqn string) *ir.Exception {
	ns := e.p.Namespace(e.declNS[fqn])
	if ns == nil {
		return nil
	}
	for _, d := range ns.Decls {
		if x, ok := d.(*ir.Exception); ok && x.Name == fqn {
			return x
		}
	}
	return nil
}

// firstOr は、適用の並びの最初の一つ。なければ、零値。
func firstOr(apps []ir.Application) ir.Application {
	if len(apps) == 0 {
		return ir.Application{}
	}
	return apps[0]
}

// containsCase は、ケースの名前の列 cases に name があるか。
func containsCase(cases []string, name string) bool {
	for _, c := range cases {
		if c == name {
			return true
		}
	}
	return false
}

// paramsDisplay は、引数の並びの、メッセージ用の書き方（`a: T, b: U`。なければ「なし」）。
func paramsDisplay(params []ir.OpParam) string {
	if len(params) == 0 {
		return "なし"
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.Name + ": " + golang.TypeDisplay(p.Type)
	}
	return strings.Join(parts, ", ")
}

// unit は、出す型一つ（<状態>Table）: 表の状態の単位（基底の <状態>State）のメソッドのうち、形で決まるもの（IsEmpty、Get、単位に
// あれば Post）と、db の語彙のデコレータを付けた read と効果を、メソッドに持つ（docs/plugin-go-sqlite.md「状態ごとの型」）。
type unit struct {
	goName string // 型の名前
	fqn    string // 状態の FQN
	iface  string // 基底の口の名前（<状態>State）
	table  *table
	// builtins は、形で決まる命令の操作（基底の単位の並び。表の状態は list なので、現れるのは post だけ）。
	builtins []string
	// reads と effects は、メソッドにする read と効果（基底の単位の並び）。missing は、デコレータを付けていない read と効果の FQN
	// （単位の並び: read、効果の順）。
	reads   []*read
	effects []*effect
	missing []string
}

// complete は、その型が、基底の口のメソッドをすべて持つか（var _ を出すか）。
func (u *unit) complete() bool { return len(u.missing) == 0 }

// collectUnits は、出す型（e.units）を、基底の状態の単位の並び（宣言順）で作る。どの read と効果が、どの単位のメソッドか（read は
// reads の状態の単位、効果は宛先の単位）と、メソッドの並びは、基底が決める（golang.ShapeOf(view).States の Builtins、Reads、Effects）。
// 表の状態でない状態の単位と、述語で決まる状態の分類の単位の型は、出さない（人が書く）。
func (e *emitter) collectUnits() {
	for _, su := range e.shape.States {
		t := e.byState[su.FQN]
		if t == nil {
			continue
		}
		u := &unit{goName: golang.Capitalize(short(su.FQN)) + "Table", fqn: su.FQN, iface: su.TypeName, table: t, builtins: su.Builtins}
		if len(u.builtins) > 0 {
			t.model.usesArgs = true
		}
		for _, fqn := range su.Reads {
			if r := e.reads[fqn]; r != nil {
				u.reads = append(u.reads, r)
			} else {
				u.missing = append(u.missing, fqn)
			}
		}
		for _, fqn := range su.Effects {
			x := e.effects[fqn]
			if x == nil {
				u.missing = append(u.missing, fqn)
				continue
			}
			u.effects = append(u.effects, x)
			if x.ok && x.kind != decoratorDelete {
				t.model.usesArgs = true
			}
		}
		e.units = append(e.units, u)
	}
}
