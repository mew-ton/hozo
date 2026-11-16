package gosqlite

import (
	"fmt"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは docs/plugin-go-sqlite.md「op の中身」: 表の状態の単位に置かれた葉の op が、どの規則に当たるか。

// rule は「op の中身」の表の行。
type rule int

const (
	ruleGet    rule = iota + 1 // 一行を引く
	ruleList                   // 並びを引く
	ruleInsert                 // 足す
	ruleUpdate                 // 書き換える
	ruleDelete                 // 消す
)

// cond は条件一つ: 引数と、それに対応する列（docs/plugin-go-sqlite.md「op の中身」の「引数と列の対応」）。
type cond struct {
	param    ir.OpParam
	col      *column
	optional bool // 省略可能な引数（値がなければ、問い合わせない）
}

// method は、中身を出す op 一つ。
type method struct {
	op     *ir.Op
	rule   rule
	conds  []cond // 引く op と消す op の条件（引数の順）
	effect string // 足す・書き換える・消す op の効果の FQN
}

// matchOps は、表の状態ごとに、その単位に置かれた op（基底の状態の単位のメソッド。宣言順）を規則に当て、中身を出すメソッド
// （table.methods）と、当たらなかった op（table.missing）に分ける。当たらなかった op ごとに warning を一つ。
func (e *emitter) matchOps() {
	byState := map[string]*table{}
	for _, t := range e.tables {
		byState[t.state.Name] = t
	}
	for _, o := range e.shape.Ops {
		if o.Place != golang.StateQuery && o.Place != golang.StateCommand {
			continue
		}
		t := byState[o.Unit]
		op, _ := e.p.Lookup(o.FQN).(*ir.Op)
		if t == nil || op == nil {
			continue
		}
		if m := e.match(op, t); m != nil {
			t.methods = append(t.methods, m)
			if m.rule == ruleInsert || m.rule == ruleUpdate {
				t.model.usesArgs = true
			}
			continue
		}
		t.missing = append(t.missing, op.Name)
		e.warns = append(e.warns, plugin.Warning{At: op.Name,
			Message: fmt.Sprintf("%s: 中身を決められないので、%s に出していない", op.Name, t.goName)})
	}
}

// match は、表の状態 t の単位に置かれた葉の op が当たる規則（上から順に、最初に当たるもの）。当たらなければ nil。
func (e *emitter) match(op *ir.Op, t *table) *method {
	state, m := t.state.Name, t.model
	row := ir.Ref(m.decl.Name)
	switch {
	case len(op.Effects) == 0 && len(op.Reads) == 1 && op.Reads[0] == state:
		conds, ok := correspond(op.Params, m)
		if !ok {
			return nil
		}
		switch {
		case ir.TypeEqual(op.Returns, ir.OptionalType{Of: row}):
			// 一行を引く: 引数は一つ以上で、対応した列のどれかが @db.key か @db.unique
			for _, c := range conds {
				if c.col.key || c.col.unique {
					return &method{op: op, rule: ruleGet, conds: conds}
				}
			}
		case ir.TypeEqual(op.Returns, ir.ArrayType{Of: row}):
			// 並びを引く: 引数はなくてもよい
			return &method{op: op, rule: ruleList, conds: conds}
		}
	case len(op.Reads) == 0 && len(op.Effects) == 1 && op.Returns == nil:
		eff, _ := e.p.Lookup(op.Effects[0]).(*ir.Effect)
		if eff == nil {
			return nil
		}
		target, builtin := eff.Target.(ir.BuiltinTarget)
		if !builtin || target.State != state {
			return nil // 遷移を宛先にする効果は、規則に当たらない
		}
		one := len(op.Params) == 1 && ir.TypeEqual(op.Params[0].Type, row)
		switch target.Operation {
		case ir.OpPost:
			if one {
				return &method{op: op, rule: ruleInsert, effect: eff.Name}
			}
		case ir.OpPut:
			// M が @db.key を一つ以上と、キーでない列（書き換えるもの）を一つ以上持つ
			if keys := len(m.keys()); one && keys >= 1 && keys < len(m.cols) {
				return &method{op: op, rule: ruleUpdate, effect: eff.Name}
			}
		case ir.OpDelete:
			if conds, ok := correspond(op.Params, m); ok && len(conds) >= 1 {
				return &method{op: op, rule: ruleDelete, conds: conds, effect: eff.Name}
			}
		}
	}
	return nil
}

// correspond は、引数のそれぞれを列に対応させる（docs/plugin-go-sqlite.md「op の中身」の「引数と列の対応」）: 引数の型
// （省略可能なら、その中の型）と等しい型のフィールドが、M にちょうど一つあれば、その列。引数の名前は見ない。
// どれか一つでも対応しなければ false。
func correspond(params []ir.OpParam, m *model) ([]cond, bool) {
	conds := make([]cond, 0, len(params))
	for _, p := range params {
		c := cond{param: p}
		typ := p.Type
		if o, ok := typ.(ir.OptionalType); ok {
			typ, c.optional = o.Of, true
		}
		for i := range m.cols {
			if ir.TypeEqual(m.cols[i].typ, typ) {
				if c.col != nil {
					return nil, false // 二つ以上
				}
				c.col = &m.cols[i]
			}
		}
		if c.col == nil {
			return nil, false
		}
		conds = append(conds, c)
	}
	return conds, true
}
