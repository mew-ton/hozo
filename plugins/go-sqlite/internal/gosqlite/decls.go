package gosqlite

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/dbvocab"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは docs/plugin-go-sqlite.md「read の中身」「効果の中身」「列と引数の型」「状態ごとの型」: read と効果に付けた db の語彙の
// デコレータの言うことと、それを、どの型のメソッドにするか。read と効果は op ではないので、中身は、宣言の全体（on の状態、引数の
// 名前と型、結果の型）から作る。デコレータが言うのは、宣言が言っていないことだけ（docs/requirements.md「出力を推論しない」）。
// デコレータと宣言の読み解きと、その食い違いの検査（DB-*）は、語彙の読み解き（plugin/dbvocab。docs/plugin-db.md「read」「効果」）。
// ここは、その結果を、このトランスパイラの表と列に結ぶ。しまい方に依る検査は GOSQL-27、このトランスパイラが出せないものは GOSQL-33。

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

// conditions は、語彙の読み解きの条件 conds を、表 t の列に結んだもの。
func conditions(t *table, conds []dbvocab.Condition) []condition {
	var out []condition
	for _, c := range conds {
		out = append(out, condition{column: t.model.column(c.Column.Name), param: c.Param, literal: c.Literal})
	}
	return out
}

// collectReads は、N の read のうち、@db.find か @db.list を付けたもの（語彙の読み解きの Reads）を、このトランスパイラの表と列に
// 結ぶ（e.reads）。宣言順に、GOSQL-27 を確かめる: @db.latest の列のしまい方が、新しい順に並べられるものでない（位置は read）。
// 付けていない read には、何も出さない（warning も出さない）。
func (e *emitter) collectReads() {
	for _, d := range e.ns.Decls {
		decl, ok := d.(*ir.Read)
		if !ok {
			continue
		}
		v := e.vocab.Reads[decl.Name]
		if v == nil {
			continue
		}
		r := &read{decl: decl, apps: v.Apps, kind: v.Kind, table: e.byState[v.Table.State.Name], ok: v.OK}
		r.conds = conditions(r.table, v.Conds)
		e.reads[decl.Name] = r
		if v.Latest == nil {
			continue
		}
		r.latest = r.table.model.column(v.Latest.Name)
		if s := r.latest.store; s != nil && s.decorator != dbvocab.Datetime && s.decorator != dbvocab.Date && s.decorator != dbvocab.Time && s.decorator != dbvocab.Int && s.decorator != dbvocab.Float {
			r.ok = false
			e.errorf(codeSQLLatest, decl.Name, "read %s の @db.latest の列 %q のしまい方が @%s（新しい順に並べられるのは @db.datetime / @db.date / @db.time / @db.int / @db.float の列）", decl.Name, r.latest.name, s.decorator)
		}
	}
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

// collectEffects は、N の効果のうち、効果のデコレータ（@db.insert / @db.update / @db.delete）を付けたもの（語彙の読み解きの Effects）を、
// このトランスパイラの表と列に結ぶ（e.effects。配線が呼んでいない効果も、同じ）。宣言順に、GOSQL-33 を確かめる: 効果の throws に、
// @db.conflict が名指ししていない例外がある（このトランスパイラは、それを投げるコードを出せない。位置は効果）。
func (e *emitter) collectEffects() {
	for _, d := range e.ns.Decls {
		decl, ok := d.(*ir.Effect)
		if !ok {
			continue
		}
		v := e.vocab.Effects[decl.Name]
		if v == nil {
			continue
		}
		x := &effect{decl: decl, kind: v.Kind, table: e.byState[v.Table.State.Name], conflict: v.Conflict, ok: v.OK}
		x.conds = conditions(x.table, v.Conds)
		e.effects[decl.Name] = x
		for _, exc := range decl.Throws {
			if exc == v.Conflict {
				continue
			}
			x.ok = false
			e.errorf(codeSQLThrows, decl.Name, "効果 %s の throws の例外 %s を、このトランスパイラは投げられない（db のデコレータで中身を作る効果が投げるのは、@db.conflict が名指しした例外だけ）", decl.Name, exc)
		}
	}
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
