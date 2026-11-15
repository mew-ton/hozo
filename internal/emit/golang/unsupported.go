package golang

import (
	"github.com/mew-ton/hozo/ir"
)

// CodeUnsupported は GOEMIT-6（docs/emit-go.md「移行の間の扱い」）: 既定の出力が支えない書き方を、出力の対象の宣言が使っている。
const CodeUnsupported = "GOEMIT-6"

// checkUnsupported は GOEMIT-6（docs/emit-go.md「移行の間の扱い」）。出力の対象の宣言（選ばれた op と配線、選ばれた分類）が、
// 既定の出力が支えない書き方を使っていれば、使っている箇所ごとに一つ出す。出力の対象でない宣言が使っていても、何もしない。
// 支える新しい書き方は、継承した model、export、reads、状態を名前で書いた標準の分類での match の四つだけ。
func (e *emitter) checkUnsupported() {
	unsupported := func(span ir.Span, what string) {
		e.errorf(CodeUnsupported, &span, "%s は、組み込みの Go の出力では使えない", what)
	}
	// op の分類と、as を持つケースのある分類
	for _, c := range e.classifies {
		if _, isOp := c.Subject.(ir.OpSubject); isOp {
			unsupported(c.Span, "op の分類（classify "+c.Name+"）")
		}
		for _, cs := range c.Cases {
			for _, as := range cs.As {
				unsupported(as.Span, "ケースの as（classify "+c.Name+" の "+cs.Name+"）")
			}
		}
	}
	for _, op := range e.ops {
		// never を返す op（選ばれた op）
		if ir.IsNever(op.Returns) {
			unsupported(op.Span, "never を返す op（"+op.Name+"）")
		}
		if op.Body == nil || e.isSlot(op) {
			continue
		}
		e.checkUnsupportedWiring(op, unsupported)
	}
}

// checkUnsupportedWiring は選ばれた配線の本体の GOEMIT-6。
func (e *emitter) checkUnsupportedWiring(op *ir.Op, unsupported func(ir.Span, string)) {
	// 状態の参照のうち、「状態を名前で書いた、標準の分類での match の対象」でないもの
	var atom func(a ir.Atom)
	atom = func(a ir.Atom) {
		switch x := a.(type) {
		case ir.StateAtom:
			unsupported(x.Span, "状態の参照（"+x.State+"）")
		case ir.RecordAtom:
			for _, f := range x.Fields {
				atom(f.Value)
			}
		}
	}
	call := func(c *ir.Call) {
		// never を返す op（選ばれた配線が呼ぶ op）
		if callee := e.op(c.Op); callee != nil && ir.IsNever(callee.Returns) {
			unsupported(c.Span, "never を返す op の呼び出し（"+c.Op+"）")
		}
		for _, a := range c.Args {
			atom(a.Value)
		}
	}
	match := func(m *ir.Match) {
		switch {
		case m.Builtin():
			// 対象が状態そのものでない、標準の分類での match（値の by list / by nullable）。
			// 対象のない IR（MATCH-7 を通っていれば来ない）は、配線を作るときの内部エラーに任せる
			if _, ok := m.SubjectState(); !ok && m.SubjectAtom() != nil {
				unsupported(m.Span, "値を対象にした、標準の分類での match（by "+m.Classify+"）")
				atom(m.SubjectAtom())
			}
		default:
			if _, ok := e.lookup(m.Classify).(*ir.Classify); ok && e.declNS[m.Classify] != e.viewer {
				unsupported(m.Span, "別の名前空間の分類での match（by "+m.Classify+"）")
			}
			if a := m.SubjectAtom(); a != nil {
				atom(a)
			}
		}
		for i := range m.Arms {
			arm := &m.Arms[i]
			switch body := arm.Body.(type) {
			case ir.CallExpr:
				call(&body.Call)
			case ir.AtomExpr:
				unsupported(arm.Span, "アームの本体の値")
				atom(body.Atom)
			case ir.ThrowBody:
				unsupported(body.Span, "アームの本体の throw")
			}
		}
	}
	expr := func(x ir.Expr) {
		switch y := x.(type) {
		case ir.CallExpr:
			call(&y.Call)
		case ir.MatchExpr:
			match(&y.Match)
		case ir.AtomExpr:
			atom(y.Atom)
		}
	}
	for _, s := range op.Body.Statements {
		switch x := s.(type) {
		case ir.ConstStmt:
			expr(x.Expr)
		case ir.ReturnStmt:
			expr(x.Expr)
		case ir.CallStmt:
			call(&x.Call)
		case ir.MatchStmt:
			match(&x.Match)
		}
	}
}
