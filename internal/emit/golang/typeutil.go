package golang

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// stripCase は case(C, cases, of) を of にする（docs/emit-go.md「型」: 絞り込みは Go の型にしない）。
func stripCase(t ir.Type) ir.Type {
	for {
		c, ok := t.(ir.CaseType)
		if !ok {
			return t
		}
		t = c.Of
	}
}

// stripOptional は optional(T) を T にし、optional だったかを返す。case も外す。
func stripOptional(t ir.Type) (ir.Type, bool) {
	t = stripCase(t)
	if o, ok := t.(ir.OptionalType); ok {
		return stripCase(o.Of), true
	}
	return t, false
}

// isNillable は Go の零値 nil が「なし」を表す型か（optional、array、record、union、void）。array と record の規則は
// plugin/golang.IsNillable（optional(T) を *T にしない型）と同じもので、ここはそれに、型だけでは決まらないもの
// （union への ref はインターフェース）と、optional そのもの・void を足す。
func (e *emitter) isNillable(t ir.Type) bool {
	switch x := stripCase(t).(type) {
	case nil:
		return true
	case ir.OptionalType:
		return true
	case ir.RefType:
		return e.union(x.Target) != nil
	default:
		return gol.IsNillable(x)
	}
}

// scalarChain は scalar の継承を元の方へ辿った列（自分を含む。自分が先頭）。循環は TYPE-9 で弾かれている。
func (e *emitter) scalarChain(s *ir.Scalar) []*ir.Scalar {
	var chain []*ir.Scalar
	seen := map[string]bool{}
	for s != nil && !seen[s.Name] {
		seen[s.Name] = true
		chain = append(chain, s)
		r, ok := s.Base.(ir.RefType)
		if !ok {
			break
		}
		s = e.scalar(r.Target)
	}
	return chain
}

// coreOf は型の元の核の型。scalar は継承を辿る。optional と case は外す。
// model / union / array / record なら false。
func (e *emitter) coreOf(t ir.Type) (ir.CoreName, bool) {
	t, _ = stripOptional(t)
	switch x := t.(type) {
	case ir.CoreType:
		return x.Name, true
	case ir.RefType:
		s := e.scalar(x.Target)
		if s == nil {
			return "", false
		}
		chain := e.scalarChain(s)
		if c, ok := chain[len(chain)-1].Base.(ir.CoreType); ok {
			return c.Name, true
		}
	}
	return "", false
}

// isNumeric は min / max を付けられる核の型か（docs/emit-go.md「scalar」: 数値の比較）。
func isNumeric(c ir.CoreName) bool { return c == ir.CoreInt || c == ir.CoreFloat }

// plainLiteral は型を付けない Go のリテラル。duration はナノ秒の整数。
func plainLiteral(l ir.Literal) string {
	switch x := l.(type) {
	case ir.StringLiteral:
		return strconv.Quote(x.Value)
	case ir.IntLiteral:
		return strconv.FormatInt(x.Value, 10)
	case ir.FloatLiteral:
		return strconv.FormatFloat(x.Value, 'g', -1, 64)
	case ir.BooleanLiteral:
		return strconv.FormatBool(x.Value)
	case ir.DurationLiteral:
		return strconv.FormatInt(int64(x.Value), 10)
	}
	return "nil"
}

// literalType はリテラルの型（docs/types.md「互換」の規則 10 の左側）。
func literalType(l ir.Literal) ir.Type {
	switch l.(type) {
	case ir.StringLiteral:
		return ir.String()
	case ir.IntLiteral:
		return ir.Int()
	case ir.FloatLiteral:
		return ir.Float()
	case ir.BooleanLiteral:
		return ir.Boolean()
	case ir.DurationLiteral:
		return ir.Duration()
	}
	return nil
}

// literalFor はリテラルを target の位置に置く Go の式。target が optional なら中身の型で作り、
// ポインタにするのは呼ぶ側（convert）。typed なら核の型でも `int64(5)` のように型を付ける
// （ptr(...) の型推論と、`Duration` の `time.Duration(ns)` のため）。
func (e *emitter) literalFor(l ir.Literal, target ir.Type, typed bool, imp *gol.Imports) string {
	target, _ = stripOptional(target)
	lit := plainLiteral(l)
	if target == nil {
		return lit
	}
	goType := gol.GoType(target, imp)
	if c, ok := target.(ir.CoreType); ok {
		if c.Name == ir.CoreDuration || typed {
			return goType + "(" + lit + ")"
		}
		return lit
	}
	return goType + "(" + lit + ")"
}

// defaultExpr は既定値のリテラルを target の型のフィールドに入れる式（docs/emit-go.md「model」の New<Model>）。
// optional ならポインタにする。
func (e *emitter) defaultExpr(l ir.Literal, target ir.Type, imp *gol.Imports) string {
	inner, opt := stripOptional(target)
	expr := e.literalFor(l, inner, opt, imp)
	if opt && !e.isNillable(inner) {
		return e.addressOf(expr)
	}
	return expr
}

// zeroExpr は型の Go の零値（docs/emit-go.md「配線の本体」の「零値」）。void は ""。
func (e *emitter) zeroExpr(t ir.Type, imp *gol.Imports) string {
	t = stripCase(t)
	switch x := t.(type) {
	case nil:
		return ""
	case ir.OptionalType, ir.ArrayType, ir.RecordType:
		return "nil"
	case ir.CoreType:
		switch x.Name {
		case ir.CoreString:
			return `""`
		case ir.CoreBoolean:
			return "false"
		case ir.CoreTime:
			return gol.GoType(x, imp) + "{}"
		}
		return "0"
	case ir.RefType:
		if e.union(x.Target) != nil {
			return "nil"
		}
		if e.model(x.Target) != nil {
			return gol.GoType(x, imp) + "{}"
		}
		if s := e.scalar(x.Target); s != nil {
			// 名前付きの型の零値は、元の核の型の零値をその型に変換したもの（`T("")`、`T(0)`、`T(false)`）。Time が元なら `T{}`。
			c, ok := e.coreOf(s.Base)
			if !ok || c == ir.CoreTime {
				return gol.GoType(x, imp) + "{}"
			}
			return gol.GoType(x, imp) + "(" + e.zeroExpr(ir.CoreType{Name: c}, imp) + ")"
		}
	}
	return "nil"
}

// fieldType は model のフィールドの型。なければ nil。
func (e *emitter) fieldType(modelFQN, field string) (ir.Type, bool) {
	m := e.model(modelFQN)
	if m == nil {
		return nil, false
	}
	for _, f := range m.Fields {
		if f.Name == field {
			return f.Type, true
		}
	}
	return nil, false
}

// modelOf は型が model への参照（optional / case を外して）ならその model。
func (e *emitter) modelOf(t ir.Type) *ir.Model {
	t, _ = stripOptional(t)
	if r, ok := t.(ir.RefType); ok {
		return e.model(r.Target)
	}
	return nil
}

// scalarOf は型が scalar への参照（optional / case を外して）ならその scalar。
func (e *emitter) scalarOf(t ir.Type) *ir.Scalar {
	t, _ = stripOptional(t)
	if r, ok := t.(ir.RefType); ok {
		return e.scalar(r.Target)
	}
	return nil
}

// convert は from の型の Go の式 expr を to の型の位置に置く式にする（docs/types.md「互換」を Go に写す）。
//
//   - 等しければそのまま。case は of に落とす。
//   - to が optional(T) で from が optional でなければ、T に変換してからポインタにする
//     （array / record はそのまま）。`&x` が書ける式（名前とフィールドの並び、複合リテラル）はそれで、
//     それ以外は wiring.go の ptr ヘルパー。
//   - model から別の model へは射影の変換関数 <From>To<To>（必要になった組を記録する）。
//   - scalar から核の型（規則 11）や核の型から scalar（既定値の規則 10）は Go の型変換。
//   - それ以外（配列の要素の変換など）はそのまま出す。
func (e *emitter) convert(expr string, from, to ir.Type, imp *gol.Imports) string {
	from, to = stripCase(from), stripCase(to)
	if from == nil || to == nil || ir.TypeEqual(from, to) {
		return expr
	}
	if toOpt, ok := to.(ir.OptionalType); ok {
		if fromOpt, ok := from.(ir.OptionalType); ok {
			if ir.TypeEqual(stripCase(fromOpt.Of), stripCase(toOpt.Of)) {
				return expr
			}
			return expr // 中身の変換が要る optional どうし。nil の扱いが決められないのでそのまま
		}
		inner := e.convert(expr, from, toOpt.Of, imp)
		if e.isNillable(toOpt.Of) {
			return inner
		}
		return e.addressOf(inner)
	}
	fromRef, fok := from.(ir.RefType)
	toRef, tok := to.(ir.RefType)
	if fok && tok {
		if e.model(fromRef.Target) != nil && e.model(toRef.Target) != nil {
			return e.projectionFunc(fromRef.Target, toRef.Target) + "(" + expr + ")"
		}
		return gol.GoType(to, imp) + "(" + expr + ")"
	}
	if fok && e.scalar(fromRef.Target) != nil {
		if _, ok := to.(ir.CoreType); ok {
			return gol.GoType(to, imp) + "(" + expr + ")"
		}
	}
	if tok && e.scalar(toRef.Target) != nil {
		if _, ok := from.(ir.CoreType); ok {
			return gol.GoType(to, imp) + "(" + expr + ")"
		}
	}
	return expr
}

// addressOf は式のポインタを取る。`&x.y` が書けない式は ptr(...) にする。
func (e *emitter) addressOf(expr string) string {
	if isAddressable(expr) {
		return "&" + expr
	}
	e.needPtr = true
	return "ptr(" + expr + ")"
}

// isAddressable は、名前とフィールドの並び（`a.b.c`）か、複合リテラル（`T{...}`）か。
func isAddressable(expr string) bool {
	if strings.HasSuffix(expr, "}") && !strings.HasPrefix(expr, "func") {
		return true
	}
	for _, r := range expr {
		if !(r == '.' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return expr != ""
}

// projectionFunc は射影の変換関数の名前 <From>To<To> を返し、必要な組として記録する。
func (e *emitter) projectionFunc(from, to string) string {
	p := projection{from, to}
	found := false
	for _, q := range e.projections {
		if q == p {
			found = true
			break
		}
	}
	if !found {
		e.projections = append(e.projections, p)
	}
	return e.typeName(from) + "To" + e.typeName(to)
}
