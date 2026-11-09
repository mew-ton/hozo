package golang

import "github.com/mew-ton/hozo/ir"

// collectDecls は viewer の宣言を種類ごとに集め、別の名前空間の型のうち複製するもの
// （docs/emit-go.md「配置」の「使う型を N のパッケージに複製する」）を推移的に集める。
//
// 複製するのは、viewer のパッケージが型として使う参照先のうち、Go で同じプロジェクトでない
// 名前空間（語彙の名前空間、グローバル、TS の名前空間）の model / scalar / union / exception と、
// それらのフィールド・元の型・アーム・値が参照する宣言。exception は op の（viewer から見た）
// Signature.exceptions から辿る: 葉の実装者が投げ、呼ぶ側（go-cli の cli.go など）が errors.As で
// 見分けるための Go の型が要るため（docs/emit-go.md「配置」「exception」）。
func (e *emitter) collectDecls() {
	for _, d := range e.ns.Decls {
		switch x := d.(type) {
		case *ir.Model:
			e.models = append(e.models, x)
			for _, f := range x.Fields {
				e.visitType(f.Type)
			}
		case *ir.Scalar:
			e.scalars = append(e.scalars, x)
			e.visitType(x.Base)
		case *ir.Union:
			e.unions = append(e.unions, x)
			for _, a := range x.Arms {
				e.visitType(a.Type)
			}
		case *ir.Exception:
			e.exceptions = append(e.exceptions, x)
			e.visitType(x.Value)
		case *ir.Effect:
			e.effects = append(e.effects, x)
		case *ir.Classify:
			e.classifies = append(e.classifies, x)
			e.visitType(x.Subject)
		case *ir.Op:
			e.ops = append(e.ops, x)
			for _, p := range x.Params {
				e.visitType(p.Type)
			}
			e.visitType(x.Returns)
			for _, fqn := range e.viewedSig(x.Name).Exceptions {
				e.visitRef(fqn)
			}
			e.visitWiring(x.Body)
		}
	}
	// 複製は Program の順で後ろに足す。
	for i := range e.prog.Namespaces {
		n := &e.prog.Namespaces[i]
		if n.Name == e.viewer {
			continue
		}
		for _, d := range n.Decls {
			if d == nil || !e.copied[d.Common().Name] {
				continue
			}
			switch x := d.(type) {
			case *ir.Model:
				e.models = append(e.models, x)
			case *ir.Scalar:
				e.scalars = append(e.scalars, x)
			case *ir.Union:
				e.unions = append(e.unions, x)
			case *ir.Exception:
				e.exceptions = append(e.exceptions, x)
			}
		}
	}
}

// visitWiring は配線が使う別の名前空間の op の引数・戻り値と、同じ名前空間の呼び出しで
// bind が埋める状態の要素の型を見る。
func (e *emitter) visitWiring(w *ir.Wiring) {
	if w == nil {
		return
	}
	for _, s := range w.Statements {
		switch x := s.(type) {
		case ir.ConstStmt:
			e.visitExpr(x.Expr)
		case ir.ReturnStmt:
			e.visitExpr(x.Expr)
		case ir.CallStmt:
			e.visitCall(&x.Call)
		case ir.MatchStmt:
			e.visitMatch(&x.Match)
		}
	}
}

func (e *emitter) visitExpr(x ir.Expr) {
	switch y := x.(type) {
	case ir.CallExpr:
		e.visitCall(&y.Call)
	case ir.MatchExpr:
		e.visitMatch(&y.Match)
	}
}

func (e *emitter) visitMatch(m *ir.Match) {
	for _, a := range m.Arms {
		if c, ok := a.Body.(ir.CallExpr); ok {
			e.visitCall(&c.Call)
		}
	}
}

func (e *emitter) visitCall(c *ir.Call) {
	op := e.op(c.Op)
	if op == nil {
		return
	}
	if e.declNS[c.Op] == e.viewer {
		for _, b := range c.Bound {
			if st, ok := e.lookup(b.State).(*ir.State); ok {
				e.visitType(st.Element)
			}
		}
		return
	}
	for _, p := range e.viewedParams(c.Op) {
		e.visitType(p.Type)
	}
	e.visitType(op.Returns)
}

// visitType は型が参照する宣言のうち複製が要るものを copied に足し、その宣言の型も辿る。
func (e *emitter) visitType(t ir.Type) {
	switch x := t.(type) {
	case nil:
	case ir.RefType:
		e.visitRef(x.Target)
	case ir.OptionalType:
		e.visitType(x.Of)
	case ir.ArrayType:
		e.visitType(x.Of)
	case ir.RecordType:
		e.visitType(x.Key)
		e.visitType(x.Value)
	case ir.CaseType:
		e.visitType(x.Of)
	}
}

func (e *emitter) visitRef(fqn string) {
	ns, known := e.declNS[fqn]
	if !known || ns == e.viewer || e.isGoNS(ns) || e.copied[fqn] {
		return
	}
	switch d := e.lookup(fqn).(type) {
	case *ir.Model:
		e.copied[fqn] = true
		for _, f := range d.Fields {
			e.visitType(f.Type)
		}
	case *ir.Scalar:
		e.copied[fqn] = true
		e.visitType(d.Base)
	case *ir.Union:
		e.copied[fqn] = true
		for _, a := range d.Arms {
			e.visitType(a.Type)
		}
	case *ir.Exception:
		e.copied[fqn] = true
		e.visitType(d.Value)
	}
}

// viewedParams は viewer から見た op の引数（docs/ir.md「その名前空間から見た IR」の params）。
// View.Ops になければ Op.Params。
func (e *emitter) viewedParams(opFQN string) []ir.OpParam {
	if sig := e.view.Signature(opFQN); sig != nil {
		return sig.Params
	}
	if op := e.op(opFQN); op != nil {
		return op.Params
	}
	return nil
}

// viewedSig は viewer から見た op の Signature。View.Ops になければ Op.Signature。
func (e *emitter) viewedSig(opFQN string) ir.Signature {
	if sig := e.view.Signature(opFQN); sig != nil {
		return sig.Signature
	}
	if op := e.op(opFQN); op != nil {
		return op.Signature
	}
	return ir.Signature{}
}
