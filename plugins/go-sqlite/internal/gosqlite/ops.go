package gosqlite

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは docs/plugin-go-sqlite.md「op の中身」「列と引数の型」「状態ごとの型」「状態をまたぐ単位の型」: 効果と op に付けた
// db の語彙のデコレータの言うことと、それを、どの単位の型のメソッドにするか。op の中身は、デコレータの引数だけから作る。
// op と効果の宣言（引数と戻り値の型、効果の宛先、reads と effects）は、デコレータと食い違っていないかを確かめるのにだけ使う
// （GOSQL-7〜GOSQL-15）。食い違っていれば、出力を合わせて変えずに、診断を返す。

// effect は、効果一つと、それに付けた効果のデコレータ（@db.insert / @db.update / @db.delete）の言うこと。
type effect struct {
	decl *ir.Effect
	// decorators は、付いた効果のデコレータの FQN（書かれた順）。空なら、db の語彙の効果ではない。
	decorators []string
	// kind は、ただ一つの効果のデコレータの FQN。table は、その引数の表。column は、@db.delete の引数の列。
	// ok は、効果のデコレータがちょうど一つで、食い違いがない（メソッドの文にできる）か。
	kind   string
	table  *table
	column *column
	ok     bool
}

// step は、@db.effects に並べた効果一つと、それが受け取る op の引数。
type step struct {
	effect *effect
	param  ir.OpParam
}

// method は、db の op のデコレータ（@db.find、@db.list、@db.effects）を付けた op 一つと、そのデコレータの言うこと。
type method struct {
	op *ir.Op
	// kind は、ただ一つの db の op のデコレータの FQN（二つ以上持つ op は ""。GOSQL-14）。
	kind string
	// writes は、@db.effects を持つか（状態をまたぐ単位の型を出すかどうかは、これで決まる）。
	writes bool
	// table と column は、@db.find と @db.list の引数の表と列（列を渡さない @db.list は nil）。
	table  *table
	column *column
	// steps は、@db.effects に並べた効果（書かれた順）。
	steps []step
	// ok は、食い違いがない（メソッドにできる）か。
	ok bool
}

// tableArg は、db の語彙のデコレータ decorator の引数の状態 state の表。N の表の状態でなければ GOSQL-7（位置は、デコレータを
// 付けた宣言 at。what は、その種類の言い方）。@db.table は付いているが、表にならなかった状態（GOSQL-1 か GOSQL-2 を返している）は、重ねて報告しない。
func (e *emitter) tableArg(state, decorator, what, at string) *table {
	if t := e.byState[state]; t != nil {
		return t
	}
	if e.marked[state] {
		return nil
	}
	why := "@db.table が付いていない"
	if e.declNS[state] != e.view.Viewer {
		why = "別の名前空間の状態"
	}
	e.errorf(codeSQLTable, at, "%s %s の @%s の引数の状態 %s が、名前空間 %s の表の状態でない（%s）", what, at, decorator, state, e.view.Viewer, why)
	return nil
}

// columnArg は、表 t の model の、名前が name の列。なければ GOSQL-8（位置は、デコレータを付けた宣言 at）。
func (e *emitter) columnArg(t *table, name, decorator, what, at string) *column {
	if c := t.model.column(name); c != nil {
		return c
	}
	e.errorf(codeSQLName, at, "%s %s の @%s の引数の列 %q が、表 %s の model %s のフィールドにない", what, at, decorator, name, t.state.Name, t.model.decl.Name)
	return nil
}

// collectEffects は、N の効果のすべてについて、効果のデコレータの言うことを確かめる（op に並べられていない効果も、同じに確かめる）。
func (e *emitter) collectEffects() {
	for _, d := range e.ns.Decls {
		if eff, ok := d.(*ir.Effect); ok {
			e.effectOf(eff.Name)
		}
	}
}

// effectOf は、効果 fqn と、その効果のデコレータの言うこと。効果でなければ nil。一度だけ確かめる（診断は、効果ごとに一度）:
// 効果のデコレータを二つ以上持てば GOSQL-12、引数の状態が N の表の状態でなければ GOSQL-7、列が表の model になければ GOSQL-8、
// デコレータが効果の宛先と合わなければ GOSQL-11、@db.update の表の model が、キーか、キーでない列を持たなければ GOSQL-15。
// どれも、位置は効果。
func (e *emitter) effectOf(fqn string) *effect {
	if x, done := e.effects[fqn]; done {
		return x
	}
	decl, _ := e.p.Lookup(fqn).(*ir.Effect)
	if decl == nil {
		e.effects[fqn] = nil
		return nil
	}
	x := &effect{decl: decl}
	e.effects[fqn] = x
	apps := applications(decl.Decorators, decoratorInsert, decoratorUpdate, decoratorDelete)
	for _, a := range apps {
		x.decorators = append(x.decorators, a.Decorator)
	}
	switch len(apps) {
	case 0:
		return x
	case 1:
	default:
		e.errorf(codeSQLEffect, fqn, "効果 %s が、効果のデコレータを二つ以上持つ（%s。@db.insert / @db.update / @db.delete のうち、ちょうど一つ）",
			fqn, "@"+strings.Join(x.decorators, "、@"))
		return x
	}
	app := apps[0]
	x.kind = app.Decorator
	before := len(e.errs)
	state := refArg(app, "table")
	x.table = e.tableArg(state, x.kind, "効果", fqn)
	if x.table != nil && x.kind == decoratorDelete {
		name, _ := stringArg(app, "column")
		x.column = e.columnArg(x.table, name, x.kind, "効果", fqn)
	}

	// GOSQL-11: 宛先が組み込みの効果なら、状態と操作が合う。遷移なら、表が、効果の書き込む状態の一つ
	switch target := decl.Target.(type) {
	case ir.BuiltinTarget:
		want := map[string]ir.Operation{decoratorInsert: ir.OpPost, decoratorUpdate: ir.OpPut, decoratorDelete: ir.OpDelete}[x.kind]
		switch {
		case target.State != state:
			e.errorf(codeSQLTarget, fqn, "効果 %s の @%s の表 %s が、効果の宛先 %s.%s の状態でない", fqn, x.kind, state, target.State, target.Operation)
		case target.Operation != want:
			e.errorf(codeSQLTarget, fqn, "効果 %s の @%s が、効果の宛先 %s.%s の操作と合わない（@%s は %s）", fqn, x.kind, target.State, target.Operation, x.kind, want)
		}
	default:
		written := false
		for _, w := range decl.Writes {
			written = written || w == state
		}
		if !written {
			e.errorf(codeSQLTarget, fqn, "効果 %s の @%s の表 %s が、効果の書き込む状態（%s）にない", fqn, x.kind, state, strings.Join(decl.Writes, "、"))
		}
	}

	// GOSQL-15: 書き換える行は @db.key で選び、キーでない列を書き換える
	if x.table != nil && x.kind == decoratorUpdate {
		switch keys := len(x.table.model.keys()); {
		case keys == 0:
			e.errorf(codeSQLUpdate, fqn, "効果 %s の @db.update の表 %s の model %s が、@db.key を一つも持たない（書き換える行を選べない）", fqn, state, x.table.model.decl.Name)
		case keys == len(x.table.model.cols):
			e.errorf(codeSQLUpdate, fqn, "効果 %s の @db.update の表 %s の model %s が、キーでない列を一つも持たない（書き換えるものがない）", fqn, state, x.table.model.decl.Name)
		}
	}
	x.ok = x.table != nil && len(e.errs) == before
	return x
}

// collectOps は、N の op のうち、db の op のデコレータ（@db.find、@db.list、@db.effects）を付けたものについて、デコレータの言うことを
// 確かめる（e.methods）。どの診断も、位置は op（GOSQL-7 と GOSQL-8 は、デコレータを付けた宣言 = op）。
func (e *emitter) collectOps() {
	for _, d := range e.ns.Decls {
		op, ok := d.(*ir.Op)
		if !ok {
			continue
		}
		apps := applications(op.Decorators, decoratorFind, decoratorList, decoratorEffects)
		if len(apps) == 0 {
			continue // デコレータを付けていない op には、何も出さない（warning も出さない）
		}
		m := &method{op: op, writes: hasDecorator(op.Decorators, decoratorEffects)}
		e.methods[op.Name] = m
		if len(apps) > 1 {
			names := make([]string, len(apps))
			for i, a := range apps {
				names[i] = "@" + a.Decorator
			}
			e.errorf(codeSQLCarried, op.Name, "op %s が、db の op のデコレータを二つ以上持つ（%s。ちょうど一つ）", op.Name, strings.Join(names, "、"))
			continue
		}
		m.kind = apps[0].Decorator
		before := len(e.errs)
		if m.kind == decoratorEffects {
			e.writeOp(m, apps[0])
		} else {
			e.readOp(m, apps[0])
		}
		m.ok = m.ok && len(e.errs) == before
	}
}

// carried は GOSQL-14: op が直に読む状態と直に持つ効果は、db の op のデコレータ decorator が運ぶもの（reads か effects）だけ。
func (e *emitter) carried(op *ir.Op, decorator string, reads, effects []string) {
	var extra []string
	for _, r := range op.DirectReads() {
		if !contains(reads, r) {
			extra = append(extra, "状態 "+r+" を読む")
		}
	}
	for _, x := range op.DirectEffects() {
		if !contains(effects, x) {
			extra = append(extra, "効果 "+x+" を持つ")
		}
	}
	if len(extra) > 0 {
		e.errorf(codeSQLCarried, op.Name, "op %s が、@%s の言う以外のものを読み書きする: %s（op の中身は、デコレータの言うことだけ）", op.Name, decorator, strings.Join(extra, "、"))
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// readOp は、読む op（@db.find、@db.list）のデコレータ app の言うことを確かめる（docs/plugin-go-sqlite.md「op の中身」の、読む op の表）:
// 引数は、列を渡せば一つで、型が列に合い、渡さなければない（GOSQL-9）。戻り値は、@db.find は M?、@db.list は M[]（GOSQL-10）。
// @db.find の列は、M のただ一つの @db.key か、@db.unique（GOSQL-13）。
func (e *emitter) readOp(m *method, app ir.Application) {
	op := m.op
	state := refArg(app, "table")
	e.carried(op, m.kind, []string{state}, nil)
	m.table = e.tableArg(state, m.kind, "op", op.Name)
	if m.table == nil {
		return
	}
	row := ir.Ref(m.table.model.decl.Name)
	name, hasColumn := stringArg(app, "column")
	if hasColumn {
		m.column = e.columnArg(m.table, name, m.kind, "op", op.Name)
	}
	if c := m.column; c != nil && m.kind == decoratorFind && !c.unique && !(c.key && len(m.table.model.keys()) == 1) {
		e.errorf(codeSQLUnique, op.Name, "op %s の @db.find の列 %q が、一行に決まる列でない（model %s のただ一つの @db.key でも、@db.unique でもない。並びを引くなら @db.list）",
			op.Name, name, m.table.model.decl.Name)
	}
	switch {
	case !hasColumn && len(op.Params) != 0:
		e.errorf(codeSQLParams, op.Name, "op %s の引数が %d 個（列を渡さない @%s の op は、引数を持たない）", op.Name, len(op.Params), m.kind)
	case hasColumn && len(op.Params) != 1:
		e.errorf(codeSQLParams, op.Name, "op %s の引数が %d 個（@%s の op の引数は、列 %q と比べる値一つ）", op.Name, len(op.Params), m.kind, name)
	case m.column != nil:
		e.columnParam(op, op.Params[0], m.column, m.kind)
	}
	want := ir.Type(ir.ArrayType{Of: row})
	if m.kind == decoratorFind {
		want = ir.OptionalType{Of: row}
	}
	if !ir.TypeEqual(op.Returns, want) {
		e.errorf(codeSQLReturns, op.Name, "op %s の戻り値 %s が、@%s の言うもの（%s）でない", op.Name, displayReturns(op.Returns), m.kind, golang.TypeDisplay(want))
	}
	m.ok = true
}

// displayReturns は、診断に書く戻り値の型。戻り値がなければ「なし」。
func displayReturns(t ir.Type) string {
	if t == nil {
		return "なし"
	}
	return golang.TypeDisplay(t)
}

// columnParam は「列と引数の型」（docs/plugin-go-sqlite.md）: 列 c と比べる引数 p の型は、列の型と等しい。列の型が T? なら、T でもよい。
// 向きは一方だけで、列より広い引数は GOSQL-9。出力は変えない（条件は、型によらず `"c" IS ?n`）。
func (e *emitter) columnParam(op *ir.Op, p ir.OpParam, c *column, decorator string) {
	if ir.TypeEqual(p.Type, c.typ) {
		return
	}
	if o, optional := c.typ.(ir.OptionalType); optional && ir.TypeEqual(p.Type, o.Of) {
		return
	}
	e.errorf(codeSQLParams, op.Name, "op %s の引数 %s の型 %s が、@%s の列 %q の型 %s に合わない（列の型と等しいか、列が T? なら T）",
		op.Name, p.Name, golang.TypeDisplay(p.Type), decorator, c.name, golang.TypeDisplay(c.typ))
}

// effectTargets は、@db.effects の適用 app に並べた効果の FQN（書かれた順。同じ効果を二度並べたら、二度）。効果の union を渡したら、
// そのメンバーを、その順で。
func (e *emitter) effectTargets(app ir.Application) []string {
	var out []string
	var add func(v ir.ArgValue)
	add = func(v ir.ArgValue) {
		switch x := v.(type) {
		case ir.ListValue:
			for _, item := range x.Items {
				add(item)
			}
		case ir.RefsValue:
			out = append(out, x.Targets...)
		case ir.RefValue:
			if u, ok := e.p.Lookup(x.Target).(*ir.Union); ok {
				for _, member := range u.Members {
					out = append(out, member.Target)
				}
				return
			}
			out = append(out, x.Target)
		}
	}
	for _, arg := range app.Args {
		if arg.Param == "targets" {
			add(arg.Value)
		}
	}
	return out
}

// writeOp は、書く op（@db.effects）のデコレータ app の言うことを確かめる（docs/plugin-go-sqlite.md「op の中身」の、書く op）:
// 効果は一つ以上（GOSQL-14）で、どれも、効果のデコレータを持つ（GOSQL-12）。引数の数は効果の数と同じで、n 番目の引数を、
// n 番目の効果が受け取る: @db.insert と @db.update は表の model の行、@db.delete は列に合う型の値（GOSQL-9）。戻り値はない（GOSQL-10）。
func (e *emitter) writeOp(m *method, app ir.Application) {
	op := m.op
	targets := e.effectTargets(app)
	if len(targets) == 0 {
		e.errorf(codeSQLCarried, op.Name, "op %s の @db.effects に、効果が一つもない（行う効果を並べる）", op.Name)
		return
	}
	e.carried(op, m.kind, nil, targets)
	usable := true
	for _, fqn := range targets {
		x := e.effectOf(fqn)
		if x == nil {
			usable = false
			continue
		}
		if len(x.decorators) == 0 {
			e.errorf(codeSQLEffect, op.Name, "op %s の @db.effects に並べた効果 %s が、@db.insert / @db.update / @db.delete のどれも持たない（表に何をするかを、効果に言う）", op.Name, fqn)
		}
		usable = usable && x.ok
		m.steps = append(m.steps, step{effect: x})
	}
	if len(op.Params) != len(targets) {
		e.errorf(codeSQLParams, op.Name, "op %s の引数が %d 個（@db.effects に並べた効果は %d 個。n 番目の効果が、n 番目の引数を受け取る）", op.Name, len(op.Params), len(targets))
	} else if len(m.steps) == len(targets) {
		for i := range m.steps {
			s := &m.steps[i]
			s.param = op.Params[i]
			if !s.effect.ok {
				continue // 効果の側の診断を返している
			}
			switch s.effect.kind {
			case decoratorDelete:
				e.columnParam(op, s.param, s.effect.column, s.effect.kind)
			default:
				if row := s.effect.table.model.decl.Name; !ir.TypeEqual(s.param.Type, ir.Ref(row)) {
					e.errorf(codeSQLParams, op.Name, "op %s の引数 %s の型 %s が、効果 %s（@%s）の受け取るもの（表 %s の model %s の行）でない",
						op.Name, s.param.Name, golang.TypeDisplay(s.param.Type), s.effect.decl.Name, s.effect.kind, s.effect.table.state.Name, row)
				}
			}
		}
	}
	if op.Returns != nil {
		e.errorf(codeSQLReturns, op.Name, "op %s の戻り値 %s が、@db.effects の言うもの（戻り値なし）でない", op.Name, golang.TypeDisplay(op.Returns))
	}
	m.ok = usable
}

// unitKind は、出す型の種類（docs/plugin-go-sqlite.md「出力」の公開の名前の表）。
type unitKind int

const (
	unitTable    unitKind = iota + 1 // <状態>Table: 表の状態の単位の実装
	unitClassify                     // <C>Table: 述語で決まる状態の分類の単位の、命令の実装
	unitShared                       // SharedTable: 基底の SharedState の実装
)

// unit は、出す型一つ: 基底の単位（状態の単位か SharedState）の op のうち、db の語彙のデコレータを付けたものを、メソッドに持つ。
type unit struct {
	kind   unitKind
	goName string // 型の名前
	fqn    string // 状態か、状態の分類の FQN（SharedTable は ""）
	iface  string // 基底の口の名前（<状態>State、<C>State、SharedState）
	table  *table // unitTable の表
	// at は、公開の名前が当たったときの診断の位置にする宣言（GOSQL-4）: 状態、状態の分類、SharedTable なら、それを出させた最初の op。
	at string
	// methods は、メソッドにする op（基底の口のメソッドの順）。missing は、デコレータを付けていない op の FQN（同じ順）。
	methods []*method
	missing []string
}

// complete は、その型が、基底の口のメソッドをすべて持つか（var _ を出すか）。述語で決まる状態の分類の型は、述語を持たないので、いつも偽。
func (u *unit) complete() bool { return u.kind != unitClassify && len(u.missing) == 0 }

// collectUnits は、出す型（e.units）を、ファイルの中の順で作る（docs/plugin-go-sqlite.md「出力」: 基底の states.go と同じ並び。
// 表の状態と、型を出す状態の分類を宣言順に、最後に SharedTable）。どの op がどの単位のメソッドになるか（op の置き場）は、基底が
// 決める（golang.ShapeOf）。SharedTable と <C>Table は、その単位の op に、@db.effects を付けたものが一つでもあるときだけ出す。
func (e *emitter) collectUnits() {
	for _, su := range e.shape.States {
		if t := e.byState[su.FQN]; t != nil {
			u := &unit{kind: unitTable, goName: golang.Capitalize(short(su.FQN)) + "Table", fqn: su.FQN, iface: su.TypeName, table: t, at: su.FQN}
			// 基底の状態の単位のメソッドの順: op の問い合わせ、命令（docs/plugin-go.md「状態の単位」）
			e.fill(u, append(append([]string(nil), su.Queries...), su.Commands...), false)
			e.units = append(e.units, u)
			continue
		}
		if _, isClassify := e.p.Lookup(su.FQN).(*ir.Classify); !isClassify {
			continue // @db.table のない状態の単位は、人が書く
		}
		u := &unit{kind: unitClassify, goName: golang.Capitalize(short(su.FQN)) + "Table", fqn: su.FQN, iface: su.TypeName, at: su.FQN}
		if e.fill(u, su.Commands, true) {
			e.units = append(e.units, u)
		}
	}
	u := &unit{kind: unitShared, goName: "SharedTable", iface: "SharedState"}
	if e.fill(u, e.shape.Shared, true) {
		e.units = append(e.units, u)
	}
	for _, u := range e.units {
		for _, m := range u.methods {
			for _, s := range m.steps {
				if s.effect.ok && s.effect.kind != decoratorDelete {
					s.effect.table.model.usesArgs = true
				}
			}
		}
	}
}

// fill は、単位の op（基底の口のメソッドの順）を、メソッドにする op（u.methods）と、デコレータを付けていない op（u.missing）に分ける。
// writesOnly なら（状態をまたぐ単位の型）、メソッドにするのは @db.effects を付けた op だけ。戻り値は、@db.effects を付けた op が
// 一つでもあるか。
func (e *emitter) fill(u *unit, ops []string, writesOnly bool) (writes bool) {
	for _, fqn := range ops {
		m := e.methods[fqn]
		if m == nil || (writesOnly && !m.writes) {
			u.missing = append(u.missing, fqn)
			continue
		}
		if m.writes && u.at == "" {
			u.at = fqn
		}
		writes = writes || m.writes
		u.methods = append(u.methods, m)
	}
	return writes
}
