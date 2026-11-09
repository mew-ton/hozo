package sema

import (
	"time"

	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// exprKind は式の型の種類（docs/types.md「型の種類」: Type に加えて none の型とレコードリテラルの型、
// それに「値がない」と、解決できなかったことを表す unknown）。
type exprKind int

const (
	// etUnknown は解決できなかった式。どの位置にも渡せる扱いにし、連鎖した診断を出さない。
	etUnknown exprKind = iota
	// etValue は型 typ の値。
	etValue
	// etNoValue は「値がない」（void の op の呼び出し。docs/types.md「式の型」）。
	etNoValue
	// etNone は none の型。
	etNone
	// etLiteral はリテラル。typ はその核の型（docs/types.md「互換」10）。
	etLiteral
	// etRecord はレコードリテラル。当てはめる先が決まるまで型は未定。
	etRecord
)

// exprType は式の型。
type exprType struct {
	kind exprKind
	typ  ir.Type
	rec  *recordVal
}

func unknownType() exprType { return exprType{kind: etUnknown} }

// valueType は型 t の値。t が nil（解決できなかった型）なら unknown。
func valueType(t ir.Type) exprType {
	if t == nil {
		return unknownType()
	}
	return exprType{kind: etValue, typ: t}
}
func noValue() exprType  { return exprType{kind: etNoValue} }
func noneType() exprType { return exprType{kind: etNone} }

// literalType はリテラルの型（核の型）。
func literalType(k ast.LiteralKind) exprType {
	return exprType{kind: etLiteral, typ: literalCore(k)}
}

func literalCore(k ast.LiteralKind) ir.Type {
	switch k {
	case ast.StringLit:
		return ir.String()
	case ast.IntLit:
		return ir.Int()
	case ast.FloatLit:
		return ir.Float()
	case ast.BoolLit:
		return ir.Boolean()
	case ast.DurationLit:
		return ir.Duration()
	}
	return nil
}

// typeName は診断に出す型の名前。
func typeName(t ir.Type) string {
	if t == nil {
		return "void"
	}
	return t.String()
}

// exprName は診断に出す式の型の名前。
func exprName(e exprType) string {
	switch e.kind {
	case etValue:
		return typeName(e.typ)
	case etNoValue:
		return "値がない"
	case etNone:
		return "none"
	case etLiteral:
		return typeName(e.typ) + " のリテラル"
	case etRecord:
		return "レコードリテラル"
	}
	return "不明"
}

// findField は model のフィールドを名前で引く。
func findField(m *ir.Model, name string) *ir.Field {
	for i := range m.Fields {
		if m.Fields[i].Name == name {
			return &m.Fields[i]
		}
	}
	return nil
}

// lowerLiteral は ast.Literal を IR のリテラルにする（docs/ir.md「正規化」の `5m` → ナノ秒）。LIT-1 も確かめる。
func (a *analyzer) lowerLiteral(l *ast.Literal) ir.Literal {
	a.checkLiteralRange(l)
	switch l.Kind {
	case ast.StringLit:
		return ir.StringLiteral{Value: l.Str}
	case ast.IntLit:
		return ir.IntLiteral{Value: l.Int}
	case ast.FloatLit:
		return ir.FloatLiteral{Value: l.Float}
	case ast.BoolLit:
		return ir.BooleanLiteral{Value: l.Bool}
	case ast.DurationLit:
		return ir.DurationLiteral{Value: durationOf(l.Duration)}
	}
	return nil
}

// --- 型の下ろし（docs/grammar.md「型」→ docs/ir.md「型」） ---

// lowerType は型の位置の型を IR にする。解決できなければ ok は false（docs/resolve.md「位置ごとに受け付ける種類」の「型」）。
func (a *analyzer) lowerType(t ast.Type, sc *scope) (ir.Type, bool) {
	switch t := t.(type) {
	case nil:
		return nil, false
	case *ast.CoreType:
		return ir.CoreType{Name: ir.CoreName(t.Name)}, true
	case *ast.RecordType:
		return a.lowerRecordType(t, sc)
	case *ast.ArrayType:
		elem, ok := a.lowerType(t.Elem, sc)
		if !ok {
			return nil, false
		}
		return ir.Array(elem), true
	case *ast.OptionalType:
		elem, ok := a.lowerType(t.Elem, sc)
		if !ok {
			return nil, false
		}
		return ir.Optional(elem), true // T?? → T?（docs/ir.md「正規化」）
	case *ast.NamedType:
		return a.lowerNamedType(t, sc)
	}
	return nil, false
}

// lowerRecordType は Record<K, V>。TYPE-11: K は string か、元が string の scalar。
func (a *analyzer) lowerRecordType(t *ast.RecordType, sc *scope) (ir.Type, bool) {
	key, kok := a.lowerType(t.Key, sc)
	value, vok := a.lowerType(t.Value, sc)
	if !kok || !vok {
		return nil, false
	}
	if !a.isStringLike(key) {
		a.errorf("TYPE-11", t.Key.NodeSpan(), "Record のキーの型 %s は string でも、元が string の scalar でもない", typeName(key))
	}
	return ir.Record(key, value), true
}

// isStringLike は string か、継承を辿った元が string の scalar か（TYPE-11）。
func (a *analyzer) isStringLike(t ir.Type) bool {
	base := a.scalarBase(t)
	c, ok := base.(ir.CoreType)
	return ok && c.Name == ir.CoreString
}

// lowerNamedType は名前の型。`C.Case<T>` は絞り込まれた型（NS-4、TYPE-12）、ほかは ref。
func (a *analyzer) lowerNamedType(t *ast.NamedType, sc *scope) (ir.Type, bool) {
	r, ok := a.resolve(t.Name, sc, acTypePos)
	if !ok {
		return nil, false
	}
	if t.Arg != nil {
		return a.lowerCaseType(t, r, sc)
	}
	if r.kind == rCase {
		a.errorf("RES-4", t.Name.Span, "classify のケース %s を型にするには型引数が要る（%s<T>）", t.Name.String(), t.Name.String())
		return nil, false
	}
	return ir.Ref(r.sym.fqn), true
}

// lowerCaseType は C.Case<T>。型引数を付けられるのは classify のケースだけ（docs/grammar.md「型」）。
func (a *analyzer) lowerCaseType(t *ast.NamedType, r resolved, sc *scope) (ir.Type, bool) {
	if r.kind != rCase {
		a.errorf("RES-4", t.Name.Span, "型引数を付けられるのは classify のケースだけで、%s ではない", describe(r))
		return nil, false
	}
	of, ok := a.lowerType(t.Arg, sc)
	if !ok {
		return nil, false
	}
	a.checkNS4(r.sym, t.Name.Span, sc)
	a.checkTYPE12(r.sym, of, t.Arg.NodeSpan())
	return ir.Case(r.sym.fqn, []string{r.name}, of), true
}

// checkNS4 は NS-4（docs/check.md「名前空間」）: 型の位置の C.Case<T> の C が現在の名前空間の classify でない。
// match の C（文法上修飾できない）にも同じ関数を使う。
func (a *analyzer) checkNS4(classify *symbol, span source.Span, sc *scope) bool {
	if classify.ns == sc.ns {
		return true
	}
	a.errorf("NS-4", span, "classify %s は宣言した名前空間 %s の中でだけ使える", classify.fqn, nsName(classify.ns))
	return false
}

func nsName(ns *namespace) string {
	if ns.fqn == "" {
		return "（グローバル）"
	}
	return ns.fqn
}

// checkTYPE12 は TYPE-12（docs/types.md「診断」）: C.Case<T> の T が C の subject に渡せない。
func (a *analyzer) checkTYPE12(classify *symbol, of ir.Type, span source.Span) {
	c, ok := a.lower(classify).(*ir.Classify)
	if !ok || c.Subject == nil {
		return
	}
	if !a.assignType(of, c.Subject) {
		a.errorf("TYPE-12", span, "%s は classify %s の対象の型 %s に渡せない", typeName(of), classify.fqn, typeName(c.Subject))
	}
}

// durationOf は int64 のナノ秒を time.Duration にする（docs/ir.md「正規化」: Go の time.Duration と同じ単位）。
func durationOf(ns int64) time.Duration { return time.Duration(ns) }
