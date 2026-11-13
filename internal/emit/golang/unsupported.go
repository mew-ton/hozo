package golang

import (
	"github.com/mew-ton/hozo/ir"
)

// checkUnsupported は、状態を分類で表す決定（docs/decisions.md「設計書を変える決定」S1〜S6）の構文のうち、
// この出力がまだ扱えないものを GOEMIT-INTERNAL の error にする。黙って誤ったコードを出さないための歯止めで、
// 一つでもあれば true を返し、run はファイルを作らない。
//
// TODO(emit): docs/emit-go.md「state」（StateCases）「classify」（状態の分類の述語と判定関数）
// 「配線の本体」の match（match by C、match s by list / by nullable）を実装したら、該当する分岐を消す。
//
//   - 状態の分類（Classify.subject が states）: <C>Cases の述語が値を取らず状態を読む形を出せない。
//   - 遷移の効果（Effect.target が transition）: 宛先の分類（状態の分類か、基底の状態の StateCases）を出せない。
//   - 状態での match（Match.subject が state / classify）: o.stateCases と Classify<C>(ctx, ...) を出せない。
func (e *emitter) checkUnsupported() bool {
	found := false
	report := func(span ir.Span, format string, args ...any) {
		found = true
		e.errorf(CodeInternal, &span, format, args...)
	}
	for _, d := range e.ns.Decls {
		switch x := d.(type) {
		case *ir.Classify:
			if _, isStates := x.SubjectStates(); isStates {
				report(x.Span, "状態の分類は未対応: classify %s（docs/emit-go.md「classify」）", x.Name)
			}
		case *ir.Effect:
			if t, ok := x.Target.(ir.TransitionTarget); ok {
				report(x.Span, "遷移の効果は未対応: effect %s on %s（docs/emit-go.md「state」「classify」）", x.Name, t.Of)
			}
		case *ir.Op:
			if x.Body == nil {
				continue
			}
			for _, m := range matchesOf(x.Body) {
				switch s := m.Subject.(type) {
				case ir.StateSubject:
					report(m.Span, "状態での match は未対応: op %s の match %s by %s（docs/emit-go.md「state」の StateCases）", x.Name, s.State, m.Classify)
				case ir.ClassifyOwnedSubject:
					report(m.Span, "状態の分類での match は未対応: op %s の match by %s（docs/emit-go.md「配線の本体」）", x.Name, m.Classify)
				}
			}
		}
	}
	return found
}

// matchesOf は配線の本体の match（文と式）を出現順に返す。
func matchesOf(w *ir.Wiring) []*ir.Match {
	var out []*ir.Match
	for _, st := range w.Statements {
		var expr ir.Expr
		switch s := st.(type) {
		case ir.ConstStmt:
			expr = s.Expr
		case ir.ReturnStmt:
			expr = s.Expr
		case ir.MatchStmt:
			m := s.Match
			out = append(out, &m)
		}
		if me, ok := expr.(ir.MatchExpr); ok {
			m := me.Match
			out = append(out, &m)
		}
	}
	return out
}

// stateValueType は bind で埋める状態の値の型（docs/emit-go.md「state」の States）: nullable<T> なら optional(T)
// （Go では *T。空なら nil）、single<T> なら T。
func stateValueType(st *ir.State) ir.Type {
	if st.Shape == ir.StateNullable {
		return ir.Optional(st.Element)
	}
	return st.Element
}
