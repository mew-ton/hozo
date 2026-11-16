package gobase

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは wiring.go（docs/plugin-go.md「配線の本体」）: 単位の対象の配線ごとに、ops のメソッドを一つ。
// 実行の順序は行の順で、並列にはしない。継ぎ目を持つ配線は、メソッドの名前が中身の名前（`loginBody`）になる。

// ptrHelper は、値をポインタにする補助（docs/plugin-go.md「値」）。wiring.go が使っていれば、その末尾に一つだけ出す。
const ptrHelper = "func ptr[T any](v T) *T { return &v }\n"

// genWiring は定義のパッケージの wiring.go。単位の対象の配線がなければ nil。
func (e *emitter) genWiring() *fileGen {
	var wirings []golang.OpShape
	for _, os := range e.shape.Ops {
		if os.Place == golang.Wiring {
			wirings = append(wirings, os)
		}
	}
	if len(wirings) == 0 {
		return nil
	}
	f := e.newFile(fileWiring, true)
	for _, os := range wirings {
		b := newBody(e, f, os)
		b.gen()
	}
	if f.usesPtr {
		f.print(ptrHelper)
	}
	return f
}

// local は、配線の中の名前（引数、束縛）一つ: Go の式と、その値の型。
type local struct {
	expr string
	typ  ir.Type
}

// scope は、Go のブロック一つで宣言した変数。
type scope struct {
	vars   map[string]local  // Hozo の名前（引数・束縛） → Go の変数
	states map[string]string // 状態の FQN → その値を読んだ Go の変数
}

// override は、match のアームの中の、絞り込まれた対象（docs/plugin-go.md「match」の「絞り込んだ値」）。
type override struct {
	name  string   // 対象が束縛か引数なら、その名前
	state string   // 対象が状態なら、その FQN
	path  []string // 対象のパス
	typ   ir.Type  // アームの中での型
	value string   // 対象を値として使う位置の式
	base  string   // 対象を先頭に持つ、より長いパスの、先頭の式
}

// matchMode は match の使われ方。
type matchMode int

const (
	modeReturn matchMode = iota // return match …
	modeConst                   // const x = match …
	modeStmt                    // match …;
)

// bodyGen は配線の op 一つの本体を作る。
type bodyGen struct {
	e  *emitter
	f  *fileGen
	op *ir.Op
	os golang.OpShape

	buf    *strings.Builder
	depth  int
	scopes []*scope
	// taken は、このメソッドの中で使った Go の識別子（引数、束縛、生成した変数）。生成する変数は、これと当たらない名前にする。
	taken     map[string]bool
	overrides []override
	vn, kn    int

	ret    ir.Type // 配線の戻り値の型（void なら nil）
	hasErr bool    // 配線が error を返すか
	// terminated は、直前に書いた文が、関数を抜ける文（return / panic）か。
	terminated bool
}

func newBody(e *emitter, f *fileGen, os golang.OpShape) *bodyGen {
	o := e.op(os.FQN)
	b := &bodyGen{e: e, f: f, op: o, os: os, buf: &strings.Builder{}, depth: 1, taken: map[string]bool{}, hasErr: os.Err}
	if o.Returns != nil && !ir.IsNever(o.Returns) {
		b.ret = o.Returns
	}
	b.push()
	for _, p := range o.Params {
		name := golang.LocalName(p.Name, e.reserved)
		b.scopes[0].vars[p.Name] = local{expr: name, typ: p.Type}
		b.taken[name] = true
	}
	// 束縛名は、生成する変数（状態を読んだ値、途中の値）より先に押さえる
	for _, s := range o.Body.Statements {
		if c, ok := s.(ir.ConstStmt); ok {
			b.taken[golang.LocalName(c.Name, e.reserved)] = true
		}
	}
	return b
}

func (b *bodyGen) push() {
	b.scopes = append(b.scopes, &scope{vars: map[string]local{}, states: map[string]string{}})
}
func (b *bodyGen) pop() { b.scopes = b.scopes[:len(b.scopes)-1] }

// line は、今の字下げで一行書く。
func (b *bodyGen) line(s string) {
	b.buf.WriteString(strings.Repeat("\t", b.depth) + s + "\n")
	b.terminated = false
}

// fresh は base を元に、まだ使っていない Go の識別子を作る（当たれば、当たらなくなるまで末尾に _ を足す）。
func (b *bodyGen) fresh(base string) string {
	name := golang.LocalName(base, b.e.reserved)
	for b.taken[name] {
		name += "_"
	}
	b.taken[name] = true
	return name
}

// temp は途中の値の変数 v0、v1、…（メソッドの中の通し番号）。kind は分類の判定の結果 k0、k1、…。引数名、束縛名、先に作った
// 変数と、import するパッケージの名前（b.e.reserved）に当たる番号は、飛ばす。
func (b *bodyGen) temp() string {
	for {
		name := "v" + strconv.Itoa(b.vn)
		b.vn++
		if !b.taken[name] && !b.e.reserved[name] {
			b.taken[name] = true
			return name
		}
	}
}

func (b *bodyGen) kind() string {
	for {
		name := "k" + strconv.Itoa(b.kn)
		b.kn++
		if !b.taken[name] && !b.e.reserved[name] {
			b.taken[name] = true
			return name
		}
	}
}

func (b *bodyGen) lookupVar(name string) (local, bool) {
	for i := len(b.scopes) - 1; i >= 0; i-- {
		if l, ok := b.scopes[i].vars[name]; ok {
			return l, true
		}
	}
	return local{}, false
}

func (b *bodyGen) stateVar(fqn string) (string, bool) {
	for i := len(b.scopes) - 1; i >= 0; i-- {
		if v, ok := b.scopes[i].states[fqn]; ok {
			return v, true
		}
	}
	return "", false
}

// zeroPrefix は、失敗や「返らない」ときの return の、値の部分: `<零値>, `。戻り値がなければ ""。
func (b *bodyGen) zeroPrefix() string {
	if b.ret == nil {
		return ""
	}
	return b.f.zero(b.ret) + ", "
}

// ret1 は `return <零値>, <errExpr>`。error を返さない配線（純粋で、例外のない配線）では、返す先がないので panic にする。
func (b *bodyGen) returnErr(errExpr string) {
	if !b.hasErr {
		b.line("panic(" + errExpr + ")")
	} else {
		b.line("return " + b.zeroPrefix() + errExpr)
	}
	b.terminated = true
}

// fail は失敗の扱い: `if err != nil { return <零値>, err }`（docs/plugin-go.md「呼び出し」）。
func (b *bodyGen) fail() {
	b.line("if err != nil {")
	b.depth++
	b.returnErr("err")
	b.depth--
	b.line("}")
}

// gen は、メソッド一つを書く。
func (b *bodyGen) gen() {
	e, o := b.e, b.op
	stmts := o.Body.Statements
	for i, s := range stmts {
		b.stmt(s, stmts[i+1:])
	}
	if !b.terminated && b.ret == nil && b.hasErr {
		b.line("return nil") // 本体の終わり（戻り値のない配線）
	}
	name := b.os.Method
	if b.os.Body != "" {
		// 継ぎ目を持つ配線は、中身の名前のメソッド。op のコメントは、Ops のメソッドにだけ書く
		name = b.os.Body
		b.f.print(comment(name + " は op " + o.Name + " の中身（配線の本体）。外側の " + b.os.Method + " が、継ぎ目を通して呼ぶ。"))
	} else {
		b.f.print(opComment(o))
	}
	params, results := golang.Signature(o, b.f.im, e.reserved)
	b.f.printf("func (o *ops) %s {\n%s}\n\n", method(name, params, results), b.buf.String())
}

// ---- 型 ----

// strip は case を外す（絞り込みは Go の型にしない）。
func strip(t ir.Type) ir.Type {
	for {
		c, ok := t.(ir.CaseType)
		if !ok {
			return t
		}
		t = c.Of
	}
}

// modelOf は、型 t（case を外したもの）が model への ref なら、その model。
func (e *emitter) modelOf(t ir.Type) *ir.Model {
	if r, ok := strip(t).(ir.RefType); ok {
		m, _ := e.lookup(r.Target).(*ir.Model)
		return m
	}
	return nil
}

func fieldOf(m *ir.Model, name string) *ir.Field {
	for i := range m.Fields {
		if m.Fields[i].Name == name {
			return &m.Fields[i]
		}
	}
	return nil
}

// optionalModel は、型 t が optional(model) なら、その中の型。
func (e *emitter) optionalModel(t ir.Type) (ir.Type, bool) {
	if o, ok := strip(t).(ir.OptionalType); ok && e.modelOf(o.Of) != nil {
		return o.Of, true
	}
	return nil, false
}

// isPointer は、型 t の Go の型がポインタか（optional で、中身が nil を持てない型）。
func (e *emitter) isPointer(t ir.Type) bool {
	o, ok := strip(t).(ir.OptionalType)
	return ok && !golang.IsNillable(o.Of, e.p)
}

// nilable は、型 t の Go の型が nil を持てるか（optional、array、record、union）。
func (e *emitter) nilable(t ir.Type) bool {
	if _, ok := strip(t).(ir.OptionalType); ok {
		return true
	}
	return golang.IsNillable(t, e.p)
}

// sameGo は、二つの型の Go の型が同じか。
func (b *bodyGen) sameGo(s, t ir.Type) bool { return b.e.goKey(s) == b.e.goKey(t) }

// goKey は、型 t の Go の型を見分ける文字列（GoType と同じ形で、ref は FQN のまま。import を足さない）。
func (e *emitter) goKey(t ir.Type) string {
	switch x := t.(type) {
	case nil, ir.NeverType:
		return ""
	case ir.CoreType:
		return string(x.Name)
	case ir.RefType:
		return x.Target
	case ir.OptionalType:
		if golang.IsNillable(x.Of, e.p) {
			return e.goKey(x.Of)
		}
		return "*" + e.goKey(x.Of)
	case ir.ArrayType:
		return "[]" + e.goKey(x.Of)
	case ir.RecordType:
		return "map[" + e.goKey(x.Key) + "]" + e.goKey(x.Value)
	case ir.CaseType:
		return e.goKey(x.Of)
	}
	return ""
}

// assignable は、S の値を T の位置に渡せるか（docs/types.md「互換」の規則のうち、型どうしのもの）。match の型を決めるのに使う。
func (e *emitter) assignable(s, t ir.Type) bool {
	if s == nil || t == nil {
		return false
	}
	if ir.TypeEqual(s, t) {
		return true
	}
	if c, ok := s.(ir.CaseType); ok { // 規則 4、5
		if tc, ok := t.(ir.CaseType); ok {
			if tc.Classify != c.Classify || !ir.TypeEqual(tc.Of, c.Of) {
				return false
			}
			for _, k := range c.Cases {
				found := false
				for _, tk := range tc.Cases {
					found = found || tk == k
				}
				if !found {
					return false
				}
			}
			return true
		}
		return e.assignable(c.Of, t)
	}
	switch tt := t.(type) {
	case ir.OptionalType: // 規則 2
		if so, ok := s.(ir.OptionalType); ok {
			return e.assignable(so.Of, tt.Of)
		}
		return e.assignable(s, tt.Of)
	case ir.ArrayType: // 規則 6
		if sa, ok := s.(ir.ArrayType); ok {
			return e.assignable(sa.Of, tt.Of)
		}
	case ir.RecordType: // 規則 7
		if sr, ok := s.(ir.RecordType); ok {
			return ir.TypeEqual(sr.Key, tt.Key) && e.assignable(sr.Value, tt.Value)
		}
	}
	if r, ok := s.(ir.RefType); ok { // 規則 11
		if sc, isScalar := e.lookup(r.Target).(*ir.Scalar); isScalar {
			return e.assignable(sc.Base, t)
		}
	}
	return false
}

// ---- 値（Atom） ----

// atomKeyMatches は、絞り込み ov が、名前 name（か状態 state）のパス path の先頭に当たるか。
func (ov *override) matches(name, state string, path []string) bool {
	if ov.name != name || ov.state != state || len(ov.path) > len(path) {
		return false
	}
	for i, seg := range ov.path {
		if path[i] != seg {
			return false
		}
	}
	return true
}

// narrowedFor は、名前 name（か状態 state）のパス path に掛かる絞り込みのうち、最も長いもの。
func (b *bodyGen) narrowedFor(name, state string, path []string) *override {
	var best *override
	for i := range b.overrides {
		ov := &b.overrides[i]
		if ov.matches(name, state, path) && (best == nil || len(ov.path) >= len(best.path)) {
			best = ov
		}
	}
	return best
}

// ref は、名前の Atom（束縛・引数か、状態）とパスを、Go の式とその型にする。途中に省略可能な model があれば、文の前に途中の値を作る
// （docs/plugin-go.md「値」の「省略可能を通るパス」）。emit が false なら、型だけを求める（コードを書かない）。
func (b *bodyGen) ref(name, state string, path []string, emit bool) (string, ir.Type) {
	var expr string
	var typ ir.Type
	rest := path
	if ov := b.narrowedFor(name, state, path); ov != nil {
		rest = path[len(ov.path):]
		typ = ov.typ
		expr = ov.base
		if len(rest) == 0 {
			expr = ov.value
		}
	} else if state != "" {
		v, _ := b.stateVar(state)
		expr = v
		if st, ok := b.e.lookup(state).(*ir.State); ok {
			typ = st.Type
		}
	} else {
		l, ok := b.lookupVar(name)
		if !ok {
			l = local{expr: golang.LocalName(name, b.e.reserved)}
		}
		expr, typ = l.expr, l.typ
	}
	for i, seg := range rest {
		if inner, ok := b.e.optionalModel(typ); ok {
			return b.optionalPath(expr, inner, rest[i:], emit)
		}
		m := b.e.modelOf(typ)
		if m == nil {
			return expr, nil
		}
		fd := fieldOf(m, seg)
		if fd == nil {
			return expr, nil
		}
		expr += "." + golang.FieldName(seg)
		typ = fd.Type
	}
	return expr, typ
}

// optionalPath は、省略可能な model の値 x（中の型は inner）の先のフィールド rest を読む。式の型は optional(<フィールドの型>) で、
// 文の前に `var v<n> …; if x != nil { v<n> = ptr(x.F) }` を置く。残りのパスが、さらに省略可能な model を通るなら、入れ子にする。
func (b *bodyGen) optionalPath(x string, inner ir.Type, rest []string, emit bool) (string, ir.Type) {
	conds := []string{x + " != nil"}
	expr, typ := x, inner
	for i, seg := range rest {
		m := b.e.modelOf(typ)
		if m == nil {
			return expr, nil
		}
		fd := fieldOf(m, seg)
		if fd == nil {
			return expr, nil
		}
		expr += "." + golang.FieldName(seg)
		typ = fd.Type
		if i < len(rest)-1 {
			if in, ok := b.e.optionalModel(typ); ok {
				conds = append(conds, expr+" != nil")
				typ = in
			}
		}
	}
	result := ir.Optional(strip(typ))
	if !emit {
		return "", result
	}
	v := b.temp()
	b.line("var " + v + " " + b.f.goType(result))
	for _, c := range conds {
		b.line("if " + c + " {")
		b.depth++
	}
	if b.e.nilable(typ) {
		b.line(v + " = " + expr)
	} else {
		b.line(v + " = " + b.f.ptr(expr))
	}
	for range conds {
		b.depth--
		b.line("}")
	}
	return v, result
}

// literal は、リテラル lit を、型 target の位置に書く Go の式（docs/plugin-go.md「値」の literal の行と「型をそろえる」）。
// target が省略可能で、Go の型がポインタなら `ptr(<型>(<リテラル>))`。target が nil（位置の型が決まらない）なら、リテラルの型の値。
func (f *fileGen) literal(lit ir.Literal, target ir.Type) string {
	if opt, ok := target.(ir.OptionalType); ok {
		inner := f.literal(lit, opt.Of)
		if golang.IsNillable(opt.Of, f.e.p) {
			return inner
		}
		if _, isDuration := lit.(ir.DurationLiteral); isDuration {
			return f.ptr(inner) // すでに型の付いた式
		}
		return f.ptr(f.goType(opt.Of) + "(" + inner + ")")
	}
	if c, ok := target.(ir.CaseType); ok {
		return f.literal(lit, c.Of)
	}
	switch x := lit.(type) {
	case ir.StringLiteral:
		return strconv.Quote(x.Value)
	case ir.IntLiteral:
		if target == nil {
			return "int64(" + strconv.FormatInt(x.Value, 10) + ")"
		}
		return strconv.FormatInt(x.Value, 10)
	case ir.FloatLiteral:
		s := strconv.FormatFloat(x.Value, 'g', -1, 64)
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		return s
	case ir.BooleanLiteral:
		return strconv.FormatBool(x.Value)
	case ir.DurationLiteral:
		typ := ""
		if _, isRef := target.(ir.RefType); isRef {
			typ = f.goType(target) // Duration が元の scalar
		} else {
			typ = f.goType(ir.Duration())
		}
		return typ + "(" + strconv.FormatInt(int64(x.Value), 10) + ")"
	}
	return "nil"
}

// literalType は、リテラルの型（対応する核の型）。
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

// atomType は Atom の型（コードを書かない）。none とレコードリテラルは nil。
func (b *bodyGen) atomType(a ir.Atom) ir.Type {
	switch x := a.(type) {
	case ir.NameAtom:
		if len(x.Path) == 0 {
			return nil
		}
		_, t := b.ref(x.Path[0], "", x.Path[1:], false)
		return t
	case ir.StateAtom:
		_, t := b.ref("", x.State, x.Path, false)
		return t
	case ir.LiteralAtom:
		return literalType(x.Literal)
	}
	return nil
}

// atom は Atom を Go の式にする（docs/plugin-go.md「値」）。target は、その値を渡す位置の型（当てはめる型。決まらなければ nil）。
// 戻り値は、式と、値の型（当てはめる前の型。none とレコードリテラルは target）。
func (b *bodyGen) atom(a ir.Atom, target ir.Type) (string, ir.Type) {
	switch x := a.(type) {
	case ir.NameAtom:
		if len(x.Path) == 0 {
			return "nil", nil
		}
		expr, t := b.ref(x.Path[0], "", x.Path[1:], true)
		return b.convert(expr, t, target), t
	case ir.StateAtom:
		expr, t := b.ref("", x.State, x.Path, true)
		return b.convert(expr, t, target), t
	case ir.LiteralAtom:
		return b.f.literal(x.Literal, target), literalType(x.Literal)
	case ir.NoneAtom:
		return "nil", target
	case ir.RecordAtom:
		return b.record(&x, target, nil, nil), target
	}
	return "nil", nil
}

// convert は、型 from の式 expr を、型 to の位置に渡す式にする（docs/plugin-go.md「値」の「型をそろえる」）。
// Go の型が同じなら、そのまま。array / record の要素をそろえる必要があれば、文の前にループを置く。
func (b *bodyGen) convert(expr string, from, to ir.Type) string {
	if from == nil || to == nil || b.sameGo(from, to) {
		return expr
	}
	from, to = strip(from), strip(to)
	if topt, ok := to.(ir.OptionalType); ok {
		if _, fromOpt := from.(ir.OptionalType); fromOpt {
			return expr
		}
		inner := b.convert(expr, from, topt.Of)
		if golang.IsNillable(topt.Of, b.e.p) {
			return inner
		}
		return b.f.ptr(inner)
	}
	switch tt := to.(type) {
	case ir.ArrayType:
		if fa, ok := from.(ir.ArrayType); ok {
			src := b.simple(expr)
			return b.convertEach(src, to, "len("+src+")", func(dst, src, idx, el string) {
				b.line(dst + "[" + idx + "] = " + b.convert(el, fa.Of, tt.Of))
			}, false)
		}
	case ir.RecordType:
		if fr, ok := from.(ir.RecordType); ok {
			src := b.simple(expr)
			return b.convertEach(src, to, "len("+src+")", func(dst, src, idx, el string) {
				b.line(dst + "[" + idx + "] = " + b.convert(el, fr.Value, tt.Value))
			}, true)
		}
	case ir.CoreType:
		if fc, ok := from.(ir.CoreType); ok && fc.Name == ir.CoreInt && tt.Name == ir.CoreFloat {
			return "float64(" + expr + ")"
		}
	}
	if r, ok := from.(ir.RefType); ok {
		if _, isScalar := b.e.lookup(r.Target).(*ir.Scalar); isScalar {
			return b.f.goType(to) + "(" + expr + ")" // scalar → その元の型（規則 11）
		}
	}
	return expr
}

// simple は、式 expr が名前かパスでなければ（呼び出しなど）、途中の値に束縛して、その名前を返す。
func (b *bodyGen) simple(expr string) string {
	if strings.ContainsAny(expr, "(&{ ") {
		v := b.temp()
		b.line(v + " := " + expr)
		return v
	}
	return expr
}

// convertEach は、同じ長さのスライス（同じキーのマップ）を作り、要素ごとにそろえて写すループを置く。値が nil なら nil のまま。
func (b *bodyGen) convertEach(src string, to ir.Type, length string, assign func(dst, src, idx, el string), isMap bool) string {
	dst := b.temp()
	goTo := b.f.goType(to)
	b.line("var " + dst + " " + goTo)
	b.line("if " + src + " != nil {")
	b.depth++
	b.line(dst + " = make(" + goTo + ", " + length + ")")
	idxBase := "i"
	if isMap {
		idxBase = "key"
	}
	idx, el := b.fresh(idxBase), b.fresh("e")
	b.line("for " + idx + ", " + el + " := range " + src + " {")
	b.depth++
	assign(dst, src, idx, el)
	b.depth--
	b.line("}")
	// ループの変数は、ループの中だけの名前。次のループが同じ名前を使える
	delete(b.taken, idx)
	delete(b.taken, el)
	b.depth--
	b.line("}")
	return dst
}

// ---- bind で埋まる位置と、レコードリテラル ----

// boundValue は、bind で埋まる位置一つ: パス（引数名から）と、読んだ状態の値。
type boundValue struct {
	path []string
	expr string
	typ  ir.Type // 状態の型
}

func hasPrefix(path, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

// record は、レコードリテラル rec（書かれていない引数なら nil）を、当てはめる model の構造体リテラルにする
// （docs/plugin-go.md「値」の record の行、「bind で埋まる位置」の 3 と 4）。並びは model のフィールドの順。書かれていない
// フィールド（省略可能なフィールドだけ）は書かず、零値のまま。当てはめる型が省略可能なら `&<型>{…}`。
// path は、引数名からこの値までのパス、bounds は bind で埋まる位置。
func (b *bodyGen) record(rec *ir.RecordAtom, target ir.Type, path []string, bounds []boundValue) string {
	m := b.e.modelOf(target)
	inner := strip(target)
	optional := false
	if o, ok := inner.(ir.OptionalType); ok {
		optional, inner = true, o.Of
		m = b.e.modelOf(inner)
	}
	if m == nil {
		b.e.errorf(codeInternal, b.op.Name, "op %s の配線: レコードリテラルを当てはめる model が決まらない（検査を通った IR には来ないはずの形）", b.op.Name)
		return "nil"
	}
	written := map[string]ir.Atom{}
	if rec != nil {
		for _, fd := range rec.Fields {
			written[fd.Name] = fd.Value
		}
	}
	var fields []string
	for i := range m.Fields {
		fd := &m.Fields[i]
		fpath := append(append([]string{}, path...), fd.Name)
		var exact *boundValue
		below := false
		for j := range bounds {
			if len(bounds[j].path) == len(fpath) && hasPrefix(bounds[j].path, fpath) {
				exact = &bounds[j]
			} else if hasPrefix(bounds[j].path, fpath) {
				below = true
			}
		}
		value := ""
		switch {
		case exact != nil: // bind で埋まる位置: 読んだ値を書く
			value = b.convert(exact.expr, exact.typ, fd.Type)
		case written[fd.Name] != nil:
			if r, ok := written[fd.Name].(ir.RecordAtom); ok {
				value = b.record(&r, fd.Type, fpath, bounds)
			} else {
				value, _ = b.atom(written[fd.Name], fd.Type)
			}
		case below: // 書かれていないフィールドの中に、bind で埋まる位置がある
			value = b.record(nil, fd.Type, fpath, bounds)
		}
		if value != "" {
			fields = append(fields, golang.FieldName(fd.Name)+": "+value)
		}
	}
	lit := b.f.goType(inner) + "{" + strings.Join(fields, ", ") + "}"
	if optional {
		return "&" + lit
	}
	return lit
}

// ---- 状態の参照 ----

// statesOfAtom は、Atom が名前で読む状態（出てきた順）を out に足す。
func statesOfAtom(a ir.Atom, out *[]string) {
	switch x := a.(type) {
	case ir.StateAtom:
		addUnique(out, x.State)
	case ir.RecordAtom:
		for _, fd := range x.Fields {
			statesOfAtom(fd.Value, out)
		}
	}
}

// statesOfCall は、呼び出しが読む状態: 引数に名前で書いた状態と、bind で埋まる位置の状態。
func statesOfCall(c *ir.Call, out *[]string) {
	for _, a := range c.Args {
		statesOfAtom(a.Value, out)
	}
	for _, bp := range c.Bound {
		addUnique(out, bp.State)
	}
}

func addUnique(out *[]string, s string) {
	for _, x := range *out {
		if x == s {
			return
		}
	}
	*out = append(*out, s)
}

// read は、状態を、状態の単位の Get で読む（docs/plugin-go.md「状態の参照」）。読むのは、その状態を使う文の直前。
// 変数の名前は、状態の名前（先頭を小文字に。当たれば末尾に _）。同じ状態をもう一度読むときは、同じ変数に = で代入する。
// skip は、読まない状態（match が読んだ値を使うもの）。
func (b *bodyGen) read(states []string, skip map[string]bool) {
	for _, fqn := range states {
		if skip[fqn] {
			continue
		}
		u, ok := b.e.units[fqn]
		if !ok {
			b.e.errorf(codeInternal, b.op.Name, "op %s の配線が読む状態 %s が、名前空間 %s の状態の単位でない（検査を通った IR には来ないはずの形）", b.op.Name, fqn, b.e.view.Viewer)
			continue
		}
		get := "o." + u.Param + ".Get(ctx)"
		if v, declared := b.stateVar(fqn); declared {
			b.line(v + ", err = " + get)
		} else {
			name := short(fqn)
			v = b.fresh(strings.ToLower(name[:1]) + name[1:])
			b.scopes[len(b.scopes)-1].states[fqn] = v
			b.line(v + ", err := " + get)
		}
		b.fail()
	}
}

// ---- 呼び出し ----

// callInfo は、呼び出し一つを Go にしたもの。
type callInfo struct {
	expr   string
	ret    ir.Type // 呼ぶ先の戻り値の型（void と never の op と、効果は nil）
	hasErr bool    // 呼ぶ先が error を返すか（read と効果は、いつも）
	never  bool    // 「返らない」呼び出し（never を返す op）
}

// target は、呼び出しの先（docs/plugin-go.md「呼び出し」の表の行）を、同じ形のメソッドの呼び出しとして見たもの:
// op か、read か、効果（宣言したものと、直に呼ぶ組み込みの効果）。
type target struct {
	recv   string       // メソッドの前: `o.`（同じ名前空間の op）、`o.<名前空間の引数名>.`、`o.<状態の単位の引数名>.`
	method string       // メソッドの名前
	params []ir.OpParam // 引数の並び（Op.params、Read.params、Effect.params。直に呼ぶ組み込みの効果は、value 一つか、なし）
	ctx    bool         // 最初の引数に ctx を渡すか（read と効果は、いつも）
	ret    ir.Type      // 戻り値の型（void と never の op と、効果は nil）
	hasErr bool         // error を返すか（read と効果は、いつも）
	never  bool         // never を返す op か
	isOp   bool         // op か（bind で埋まる位置を持つのは、op の呼び出しだけ）
}

// targetOf は、呼び出し c の先。Program にないもの、状態の単位が N にないもの（検査を通った IR には来ない）は、GO-INTERNAL で false。
func (b *bodyGen) targetOf(c *ir.Call) (target, bool) {
	e := b.e
	unit := func(fqn string) (string, bool) {
		u, ok := e.units[fqn]
		if !ok {
			e.errorf(codeInternal, b.op.Name, "op %s の配線が呼ぶ %s の宛先の状態 %s が、名前空間 %s の状態の単位でない（検査を通った IR には来ないはずの形）", b.op.Name, c.Callee, fqn, e.view.Viewer)
			return "", false
		}
		return "o." + u.Param + ".", true
	}
	switch callee := c.Callee.(type) {
	case ir.OpCallee:
		o := e.op(callee.Op)
		if o == nil {
			break
		}
		t := target{recv: "o.", method: golang.MethodName(o.Name), params: o.Params, ctx: !o.Signature.Pure, isOp: true,
			hasErr: !(o.Signature.Pure && len(o.Signature.Exceptions) == 0), never: ir.IsNever(o.Returns)}
		if ns := e.declNS[o.Name]; ns != e.view.Viewer {
			t.recv = "o." + e.fields.deps[ns] + "."
		}
		if o.Returns != nil && !t.never {
			t.ret = o.Returns
		}
		return t, true
	case ir.ReadCallee:
		// read は、読む状態の単位の問い合わせ（read を呼べるのは、宣言した名前空間の配線だけ。NS-6）
		r := e.read(callee.Read)
		u, placed := e.unitOf[callee.Read]
		if r == nil || !placed {
			break
		}
		return target{recv: "o." + u.Param + ".", method: golang.MethodName(r.Name), params: r.Params, ctx: true, ret: r.Returns, hasErr: true}, true
	case ir.EffectCallee:
		switch fx := callee.Effect.(type) {
		case ir.NamedEffect:
			// 効果は、宛先の単位の命令
			x := e.effect(fx.Effect)
			u, placed := e.unitOf[fx.Effect]
			if x == nil || !placed {
				break
			}
			return target{recv: "o." + u.Param + ".", method: golang.MethodName(x.Name), params: x.Params, ctx: true, hasErr: true}, true
		case ir.DirectEffect:
			// 直に呼ぶ組み込みの効果は、その状態の単位の、形で決まる命令。値を取る操作の引数は、状態の要素の型の value 一つ
			st, ok := e.lookup(fx.State).(*ir.State)
			if !ok {
				break
			}
			recv, ok := unit(fx.State)
			if !ok {
				return target{}, false
			}
			t := target{recv: recv, method: golang.BuiltinMethod(string(fx.Operation)), ctx: true, hasErr: true}
			if golang.BuiltinTakesValue(string(fx.Operation)) {
				t.params = []ir.OpParam{{Name: ir.DirectEffectParam, Type: st.Element}}
			}
			return t, true
		}
	}
	e.errorf(codeInternal, b.op.Name, "op %s の配線が呼ぶ %v が Program にないか、その状態の単位が名前空間 %s にない（検査を通った IR には来ないはずの形）", b.op.Name, c.Callee, e.view.Viewer)
	return target{}, false
}

// call は、呼び出しの式を作る（docs/plugin-go.md「呼び出し」「bind で埋まる位置」）。呼ぶ先が op でも、read でも、効果でも、
// 同じ形のメソッドの呼び出しになる。bind で埋まる位置の状態は、先に read で読んであること。
func (b *bodyGen) call(c *ir.Call) callInfo {
	e := b.e
	t, ok := b.targetOf(c)
	if !ok {
		return callInfo{expr: "nil"}
	}
	var bounds []boundValue
	if t.isOp { // bind は、read と効果の引数を埋めない
		for _, bp := range c.Bound {
			st, ok := e.lookup(bp.State).(*ir.State)
			v, read := b.stateVar(bp.State)
			if !ok || !read || len(bp.Path) == 0 {
				continue
			}
			bounds = append(bounds, boundValue{path: bp.Path, expr: v, typ: st.Type})
		}
	}
	var args []string
	if t.ctx {
		args = append(args, "ctx")
	}
	for _, p := range t.params {
		var written ir.Atom
		for i := range c.Args {
			if c.Args[i].Param == p.Name {
				written = c.Args[i].Value
			}
		}
		var whole *boundValue
		below := false
		for i := range bounds {
			if bounds[i].path[0] != p.Name {
				continue
			}
			if len(bounds[i].path) == 1 {
				whole = &bounds[i]
			} else {
				below = true
			}
		}
		arg := ""
		switch {
		case whole != nil: // 位置が引数そのもの
			arg = b.convert(whole.expr, whole.typ, p.Type)
		case written != nil:
			if r, ok := written.(ir.RecordAtom); ok {
				arg = b.record(&r, p.Type, []string{p.Name}, bounds)
			} else {
				arg, _ = b.atom(written, p.Type)
			}
		case below: // 渡されていない引数の中のフィールド
			arg = b.record(nil, p.Type, []string{p.Name}, bounds)
		default: // 渡されていない省略可能な引数
			arg = "nil"
		}
		args = append(args, arg)
	}
	return callInfo{expr: t.recv + t.method + "(" + strings.Join(args, ", ") + ")", ret: t.ret, hasErr: t.hasErr, never: t.never}
}

// throwValue は、アームの本体の `throw E`、`throw E(<値>, …)` が作る例外の値（docs/plugin-go.md「match」の「アームの本体」）:
// 例外の構造体のリテラル。フィールドは例外の引数（Exception.params の順）で、いつもすべて書く。値は、引数の型を当てはめる型にして
// 「値」の規則で書く。渡されていない省略可能な引数は nil。
func (b *bodyGen) throwValue(t *ir.ThrowBody) string {
	typ := b.f.goType(ir.Ref(t.Exception))
	x, ok := b.e.lookup(t.Exception).(*ir.Exception)
	if !ok {
		b.e.errorf(codeInternal, b.op.Name, "op %s の配線が投げる %s が Program にない（検査を通った IR には来ないはずの形）", b.op.Name, t.Exception)
		return "nil"
	}
	var fields []string
	for _, p := range x.Params {
		value := "nil"
		for i := range t.Args {
			if t.Args[i].Param == p.Name {
				value, _ = b.atom(t.Args[i].Value, p.Type)
			}
		}
		fields = append(fields, golang.FieldName(p.Name)+": "+value)
	}
	return typ + "{" + strings.Join(fields, ", ") + "}"
}

// ---- 文 ----

// usesName は、文の列のどこかで、名前 name（束縛名か引数名）が参照されているか。
func usesName(stmts []ir.Statement, name string) bool {
	var inAtom func(a ir.Atom) bool
	inAtom = func(a ir.Atom) bool {
		switch x := a.(type) {
		case ir.NameAtom:
			return len(x.Path) > 0 && x.Path[0] == name
		case ir.RecordAtom:
			for _, fd := range x.Fields {
				if inAtom(fd.Value) {
					return true
				}
			}
		}
		return false
	}
	inCall := func(c *ir.Call) bool {
		for _, a := range c.Args {
			if inAtom(a.Value) {
				return true
			}
		}
		return false
	}
	inMatch := func(m *ir.Match) bool {
		if a := m.SubjectAtom(); a != nil && inAtom(a) {
			return true
		}
		for _, arm := range m.Arms {
			switch body := arm.Body.(type) {
			case ir.CallExpr:
				if inCall(&body.Call) {
					return true
				}
			case ir.AtomExpr:
				if inAtom(body.Atom) {
					return true
				}
			case ir.ThrowBody:
				for _, a := range body.Args {
					if inAtom(a.Value) {
						return true
					}
				}
			}
		}
		return false
	}
	inExpr := func(x ir.Expr) bool {
		switch y := x.(type) {
		case ir.CallExpr:
			return inCall(&y.Call)
		case ir.MatchExpr:
			return inMatch(&y.Match)
		case ir.AtomExpr:
			return inAtom(y.Atom)
		}
		return false
	}
	for _, s := range stmts {
		switch x := s.(type) {
		case ir.ConstStmt:
			if inExpr(x.Expr) {
				return true
			}
		case ir.ReturnStmt:
			if inExpr(x.Expr) {
				return true
			}
		case ir.CallStmt:
			if inCall(&x.Call) {
				return true
			}
		case ir.MatchStmt:
			if inMatch(&x.Match) {
				return true
			}
		}
	}
	return false
}

// bind は、束縛名 name を、型 typ の Go の変数として、今のブロックに足す。
func (b *bodyGen) bind(name string, typ ir.Type) string {
	goName := golang.LocalName(name, b.e.reserved)
	b.taken[goName] = true
	b.scopes[len(b.scopes)-1].vars[name] = local{expr: goName, typ: typ}
	return goName
}

// stmt は文一つ。rest は後ろの文（束縛が使われるかの判定）。
func (b *bodyGen) stmt(s ir.Statement, rest []ir.Statement) {
	switch x := s.(type) {
	case ir.ConstStmt:
		switch ex := x.Expr.(type) {
		case ir.CallExpr:
			var states []string
			statesOfCall(&ex.Call, &states)
			b.read(states, nil)
			ci := b.call(&ex.Call)
			name := b.bind(x.Name, ci.ret)
			if ci.hasErr {
				b.line(name + ", err := " + ci.expr)
				b.fail()
			} else {
				b.line(name + " := " + ci.expr)
			}
			b.markUnused(name, x.Name, rest)
		case ir.AtomExpr:
			var states []string
			statesOfAtom(ex.Atom, &states)
			b.read(states, nil)
			expr, typ := b.atom(ex.Atom, nil)
			name := b.bind(x.Name, typ)
			b.line(name + " := " + expr)
			b.markUnused(name, x.Name, rest)
		case ir.MatchExpr:
			b.match(&ex.Match, modeConst, x.Name, rest)
		}
	case ir.ReturnStmt:
		switch ex := x.Expr.(type) {
		case ir.CallExpr:
			var states []string
			statesOfCall(&ex.Call, &states)
			b.read(states, nil)
			b.returnCall(b.call(&ex.Call))
		case ir.AtomExpr:
			var states []string
			statesOfAtom(ex.Atom, &states)
			b.read(states, nil)
			expr, _ := b.atom(ex.Atom, b.ret)
			b.returnValue(expr)
		case ir.MatchExpr:
			b.match(&ex.Match, modeReturn, "", nil)
		}
	case ir.CallStmt:
		var states []string
		statesOfCall(&x.Call, &states)
		b.read(states, nil)
		b.callStmt(b.call(&x.Call))
	case ir.MatchStmt:
		b.match(&x.Match, modeStmt, "", nil)
	}
}

// markUnused は、束縛が下の行で使われていなければ `_ = x` を足す（WIRE-8 は warning。Go は、使わない変数をコンパイルしない）。
func (b *bodyGen) markUnused(goName, hozoName string, rest []ir.Statement) {
	if !usesName(rest, hozoName) {
		b.line("_ = " + goName)
	}
}

// returnValue は `return <値>, nil`（error を返さない配線なら `return <値>`）。
func (b *bodyGen) returnValue(expr string) {
	if b.hasErr {
		b.line("return " + expr + ", nil")
	} else {
		b.line("return " + expr)
	}
	b.terminated = true
}

// returnNever は、「返らない」呼び出しの文と return: `return <零値>, <呼び出し>`。
func (b *bodyGen) returnNever(ci callInfo) {
	b.returnErr(ci.expr)
}

// returnCall は `return 呼び出し`（docs/plugin-go.md「文」）: 呼ぶ op の戻り値の並びが、この配線と同じで、型をそろえる必要が
// なければ `return <呼び出し>`。呼ぶ op だけが error を返さないなら `return <呼び出し>, nil`。型をそろえる必要があれば、
// v<n> に束縛してから `return <そろえた値>, nil`。
func (b *bodyGen) returnCall(ci callInfo) {
	switch {
	case ci.never:
		b.returnNever(ci)
		return
	case ci.ret == nil || b.ret == nil:
		// 値のない呼び出し（検査を通った IR では、return には来ない）
		b.callStmt(ci)
		if b.hasErr {
			b.line("return nil")
		} else {
			b.line("return")
		}
		b.terminated = true
		return
	}
	same := b.sameGo(ci.ret, b.ret)
	switch {
	case same && ci.hasErr == b.hasErr:
		b.line("return " + ci.expr)
		b.terminated = true
	case same && !ci.hasErr:
		b.returnValue(ci.expr)
	default:
		v := b.temp()
		if ci.hasErr {
			b.line(v + ", err := " + ci.expr)
			b.fail()
		} else {
			b.line(v + " := " + ci.expr)
		}
		b.returnValue(b.convert(v, ci.ret, b.ret))
	}
}

// callStmt は `呼び出し;`: `if err := <呼び出し>; err != nil { return <零値>, err }`。error を返さない op なら、呼ぶだけ。
func (b *bodyGen) callStmt(ci callInfo) {
	switch {
	case ci.never:
		b.returnNever(ci)
	case ci.hasErr:
		lhs := "err"
		if ci.ret != nil {
			lhs = "_, err"
		}
		b.line("if " + lhs + " := " + ci.expr + "; err != nil {")
		b.depth++
		b.returnErr("err")
		b.depth--
		b.line("}")
	default:
		b.line(ci.expr)
	}
}

// ---- match ----

// subjectText は、match の対象の、仕様に書いたままのパス（コメント用）。
func subjectText(a ir.Atom) string {
	switch x := a.(type) {
	case ir.NameAtom:
		return strings.Join(x.Path, ".")
	case ir.StateAtom:
		return strings.Join(append([]string{short(x.State)}, x.Path...), ".")
	case ir.LiteralAtom:
		switch l := x.Literal.(type) {
		case ir.StringLiteral:
			return strconv.Quote(l.Value)
		case ir.IntLiteral:
			return strconv.FormatInt(l.Value, 10)
		case ir.FloatLiteral:
			return strconv.FormatFloat(l.Value, 'g', -1, 64)
		case ir.BooleanLiteral:
			return strconv.FormatBool(l.Value)
		}
	case ir.NoneAtom:
		return "none"
	}
	return "{…}"
}

// matchPlan は、match 一つの分け方（docs/plugin-go.md「match」の「分け方」の表）。
type matchPlan struct {
	m        *ir.Match
	classify *ir.Classify // 標準の分類なら nil
	// form は分け方: "nil"（`if <対象> != nil`）、"len"（`if len(<対象>) != 0`）、"type"（`switch <対象>.(type)`）、
	// "value"（述語で決まる値の分類・op の分類）、"state"（述語で決まる状態の分類）。
	form string
	// 対象
	subject     string   // Go の式
	subjectType ir.Type  // 対象の型（アームの外での型）
	key         override // 対象の名前・状態・パス（絞り込みを掛ける相手）。対象が名前でない Atom なら、どれも空
	hasKey      bool
	// subjectState は、match が読んだ値を使う状態（対象が状態のとき）。アームの中では読み直さない。
	subjectState string
	// freshState は、対象の状態を、この match のために新しい変数に読んだか（どの枝も書かないときに `_ =` を足すため）。
	freshState bool
}

// armCasesText は、アームのケースの名前（` | ` でつなぐ。default は `default`）。
func armCasesText(a *ir.MatchArm) string {
	if a.Cases.Default {
		return "default"
	}
	return strings.Join(a.Cases.Cases, " | ")
}

// plan は、match の分け方と対象を決め、対象を読む（状態なら Get、名前でない Atom なら v<n> に束縛）。
func (b *bodyGen) plan(m *ir.Match) *matchPlan {
	e := b.e
	p := &matchPlan{m: m}
	switch {
	case m.Classify == ir.ClassifyNullable:
		p.form = "nil"
	case m.Classify == ir.ClassifyList:
		p.form = "len"
	default:
		c, ok := e.lookup(m.Classify).(*ir.Classify)
		if !ok {
			e.errorf(codeInternal, b.op.Name, "op %s の配線の match の分類 %s が Program にない", b.op.Name, m.Classify)
			return nil
		}
		p.classify = c
		_, isState := c.SubjectStates()
		switch {
		case c.DecidedBy == ir.DecidedByPredicate && isState:
			p.form = "state"
		case c.DecidedBy == ir.DecidedByPredicate:
			p.form = "value"
		default:
			p.form = "nil"
			for _, cs := range c.Cases {
				if len(cs.As) > 0 && cs.As[0].By == ir.NarrowByVariant {
					p.form = "type"
				}
			}
		}
	}
	switch s := m.Subject.(type) {
	case ir.ValueSubject:
		var states []string
		statesOfAtom(s.Atom, &states)
		switch a := s.Atom.(type) {
		case ir.NameAtom:
			p.subject, p.subjectType = b.ref(a.Path[0], "", a.Path[1:], true)
			p.key, p.hasKey = override{name: a.Path[0], path: a.Path[1:]}, true
		case ir.StateAtom:
			_, declared := b.stateVar(a.State)
			b.read(states, nil)
			p.freshState = !declared
			p.subject, p.subjectType = b.ref("", a.State, a.Path, true)
			p.key, p.hasKey = override{state: a.State, path: a.Path}, true
			p.subjectState = a.State
		default:
			// 対象が名前でない Atom（リテラル、レコードリテラル、none）なら、v<n> に束縛してから分ける
			var target ir.Type
			if p.classify != nil {
				target = p.classify.SubjectType()
			}
			b.read(states, nil)
			expr, typ := b.atom(s.Atom, target)
			if target == nil {
				target = typ
			}
			if target == nil {
				e.errorf(codeInternal, b.op.Name, "op %s の配線の match の対象の型が決まらない", b.op.Name)
				return nil
			}
			v := b.temp()
			if _, isRecord := s.Atom.(ir.RecordAtom); isRecord {
				b.line(v + " := " + expr) // 構造体リテラルが型を持つ
			} else {
				b.line("var " + v + " " + b.f.goType(target) + " = " + expr)
			}
			p.subject, p.subjectType = v, target
		}
	case ir.ClassifyOwnedSubject:
		if p.form == "state" {
			break
		}
		// 型で決まる状態の分類: 対象の状態を読んでから、その値を対象にして分ける
		states, _ := p.classify.SubjectStates()
		if len(states) != 1 {
			e.errorf(codeInternal, b.op.Name, "op %s の配線の match の、型で決まる状態の分類 %s の対象が一つでない", b.op.Name, m.Classify)
			return nil
		}
		_, declared := b.stateVar(states[0])
		b.read(states, nil)
		p.freshState = !declared
		p.subject, p.subjectType = b.ref("", states[0], nil, true)
		p.key, p.hasKey = override{state: states[0]}, true
		p.subjectState = states[0]
	}
	return p
}

// narrowBy は、アーム a の、対象（状態の分類なら状態 state）の絞り込みの種類。標準の分類は presence。
func (p *matchPlan) narrowBy(a *ir.MatchArm, state string) ir.NarrowBy {
	if p.classify == nil {
		return ir.NarrowByPresence
	}
	for _, name := range a.Cases.Cases {
		if cs := p.classify.Case(name); cs != nil {
			for _, as := range cs.As {
				if as.State == state || as.State == "" {
					return as.By
				}
			}
		}
	}
	return ""
}

// narrow は、アーム a の中での絞り込みを b.overrides に足す（docs/plugin-go.md「match」の「絞り込んだ値」）。
// 戻り値は、枝の頭のコメントの「<対象> は <型>」の部分（絞り込まれなければ ""）。
func (b *bodyGen) narrow(p *matchPlan, a *ir.MatchArm) string {
	var texts []string
	for _, n := range a.Narrowed {
		ov := override{typ: n.Type}
		var subject string
		var from ir.Type
		text := ""
		if n.State != "" { // 状態の分類での match
			v, read := b.stateVar(n.State)
			st, ok := b.e.lookup(n.State).(*ir.State)
			if !ok {
				continue
			}
			ov.state, subject, from = n.State, v, st.Type
			text = short(n.State)
			if !read {
				// アームの本体が、この状態を使っていない
				texts = append(texts, text+" は "+golang.TypeDisplay(n.Type))
				continue
			}
		} else {
			if !p.hasKey {
				continue // 対象が名前でない Atom なら、絞り込みは何にも掛からない
			}
			ov.name, ov.state, ov.path = p.key.name, p.key.state, p.key.path
			subject, from = p.subject, p.subjectType
			text = subjectText(p.m.SubjectAtom())
		}
		texts = append(texts, text+" は "+golang.TypeDisplay(n.Type))
		ov.value, ov.base = subject, subject
		if !b.sameGo(from, n.Type) {
			switch p.narrowBy(a, n.State) {
			case ir.NarrowByVariant:
				if u := b.e.unionOf(from); u != nil {
					if arm, ok := b.e.variantArm(u, n.Type, p.m.Classify); ok {
						ov.value = subject + ".(" + b.e.armGoType(b.f, u, arm) + ")"
						if arm.wrapped {
							ov.value += ".Value"
						}
						ov.base = ov.value
					}
				}
			default: // presence と predicate
				e := subject
				inner := strip(from)
				if o, ok := inner.(ir.OptionalType); ok {
					if _, stillOptional := strip(n.Type).(ir.OptionalType); !stillOptional {
						inner = o.Of
						if b.e.isPointer(from) {
							e = "*" + subject
						}
					}
				}
				ov.value = e
				if !b.sameGo(inner, n.Type) {
					// Go の型が違えば `<絞り込んだ型>(<…>)`（string を Email に）
					ov.value = b.f.goType(n.Type) + "(" + e + ")"
					ov.base = ov.value
				}
			}
		}
		b.overrides = append(b.overrides, ov)
	}
	return strings.Join(texts, "、")
}

// armStates は、アームの本体が読む状態（match が読んだ値を使うものを除く）。
func armStates(a *ir.MatchArm) []string {
	var states []string
	switch body := a.Body.(type) {
	case ir.CallExpr:
		statesOfCall(&body.Call, &states)
	case ir.AtomExpr:
		statesOfAtom(body.Atom, &states)
	case ir.ThrowBody:
		for _, arg := range body.Args {
			statesOfAtom(arg.Value, &states)
		}
	}
	return states
}

// armType は、アームの本体の型（docs/types.md「match の型」の 1）: 値のある呼び出し（op か read）はその戻り値、名前の Atom は
// その型（そのアームの絞り込みを掛けた後）、リテラルはその型。それ以外（返らない、値がない、none、レコードリテラル）は nil。
func (b *bodyGen) armType(p *matchPlan, a *ir.MatchArm) ir.Type {
	switch body := a.Body.(type) {
	case ir.CallExpr:
		switch callee := body.Call.Callee.(type) {
		case ir.OpCallee:
			if o := b.e.op(callee.Op); o != nil && o.Returns != nil && !ir.IsNever(o.Returns) {
				return o.Returns
			}
		case ir.ReadCallee:
			if r := b.e.read(callee.Read); r != nil {
				return r.Returns
			}
		}
	case ir.AtomExpr:
		saved := b.overrides
		for _, n := range a.Narrowed {
			if n.State != "" {
				b.overrides = append(b.overrides, override{state: n.State, typ: n.Type})
			} else if p.hasKey {
				b.overrides = append(b.overrides, override{name: p.key.name, state: p.key.state, path: p.key.path, typ: n.Type})
			}
		}
		t := b.atomType(body.Atom)
		b.overrides = saved
		return t
	}
	return nil
}

// matchType は、`const x = match …` の match の型（docs/types.md「match の型」）。
func (b *bodyGen) matchType(p *matchPlan) ir.Type {
	var typed, literals []ir.Type
	hasNone := false
	for i := range p.m.Arms {
		a := &p.m.Arms[i]
		if _, none := a.Body.(ir.NoneBody); none {
			hasNone = true
			continue
		}
		if ae, ok := a.Body.(ir.AtomExpr); ok {
			switch ae.Atom.(type) {
			case ir.NoneAtom:
				hasNone = true
				continue
			case ir.LiteralAtom:
				literals = append(literals, b.armType(p, a))
				continue
			}
		}
		if t := b.armType(p, a); t != nil {
			typed = append(typed, t)
		}
	}
	var t ir.Type
	for _, cand := range typed { // 型の決まった本体のうち、ほかのすべてが渡せる型（アームの順で最初のもの）
		ok := true
		for _, other := range typed {
			ok = ok && b.e.assignable(other, cand)
		}
		if ok {
			t = cand
			break
		}
	}
	if t == nil && len(typed) > 0 {
		t = typed[0]
	}
	if t == nil && len(literals) > 0 {
		t = literals[0]
	}
	if t == nil {
		return nil
	}
	if hasNone {
		return ir.Optional(t)
	}
	return t
}

// armBody は、アームの本体を書く（docs/plugin-go.md「match」の「アームの本体」の表）。x は、`const x = match` の変数（その型は t）。
func (b *bodyGen) armBody(p *matchPlan, a *ir.MatchArm, mode matchMode, x string, t ir.Type) {
	switch body := a.Body.(type) {
	case ir.CallExpr:
		ci := b.call(&body.Call)
		switch {
		case ci.never:
			b.returnNever(ci)
		case mode == modeReturn:
			b.returnCall(ci)
		case mode == modeConst:
			if ci.ret == nil {
				b.callStmt(ci)
				return
			}
			if ci.hasErr {
				v := b.temp()
				b.line(v + ", err := " + ci.expr)
				b.fail()
				b.line(x + " = " + b.convert(v, ci.ret, t))
			} else {
				expr := ci.expr
				if !b.sameGo(ci.ret, t) {
					expr = b.convert(b.simple(expr), ci.ret, t)
				}
				b.line(x + " = " + expr)
			}
		default:
			b.callStmt(ci)
		}
	case ir.AtomExpr:
		switch mode {
		case modeReturn:
			expr, _ := b.atom(body.Atom, b.ret)
			b.returnValue(expr)
		case modeConst:
			if _, none := body.Atom.(ir.NoneAtom); none {
				return
			}
			expr, _ := b.atom(body.Atom, t)
			b.line(x + " = " + expr)
		}
	case ir.NoneBody:
		if mode == modeReturn {
			b.returnValue("nil")
		}
	case ir.ThrowBody:
		b.returnErr(b.throwValue(&body))
	}
}

// branch は、アーム一つを、枝（if の枝、後ろに続く部分、else の枝、switch の節）の中身として書き、そのテキストを返す。
// withComment なら、頭に、ケースの名前のコメントを書く（switch の節には書かない）。中身がなければ ""。
func (b *bodyGen) branch(p *matchPlan, a *ir.MatchArm, mode matchMode, x string, t ir.Type, withComment bool) (text string, terminated bool) {
	saved, savedOverrides := b.buf, b.overrides
	b.buf = &strings.Builder{}
	b.push()
	// アームの本体だけが使う状態は、そのアームの枝の中で読む。match が読んだ値を使う状態は、読み直さない
	b.read(armStates(a), map[string]bool{p.subjectState: p.subjectState != ""})
	narrowed := b.narrow(p, a)
	b.terminated = false
	b.armBody(p, a, mode, x, t)
	terminated = b.terminated
	b.pop()
	text = b.buf.String()
	b.buf, b.overrides = saved, savedOverrides
	if text != "" && withComment {
		head := armCasesText(a)
		if narrowed != "" {
			head += ": " + narrowed
		}
		text = strings.Repeat("\t", b.depth) + "// " + head + "\n" + text
	}
	return text, terminated
}

// armFor は、ケース name のアーム: そのケースを並べたアーム。なければ default のアーム。
func armFor(m *ir.Match, name string) *ir.MatchArm {
	var def *ir.MatchArm
	for i := range m.Arms {
		if m.Arms[i].Cases.Default {
			def = &m.Arms[i]
		}
		for _, c := range m.Arms[i].Cases.Cases {
			if c == name {
				return &m.Arms[i]
			}
		}
	}
	return def
}

// match は match を書く（docs/plugin-go.md「match」）: 分類の種類ごとの分け方と、使われ方ごとのアームの本体の組み合わせ。
func (b *bodyGen) match(m *ir.Match, mode matchMode, bindName string, rest []ir.Statement) {
	p := b.plan(m)
	if p == nil {
		return
	}
	// `const x = match` は、分ける前に `var x <match の型>` を置く
	x := ""
	var t ir.Type
	if mode == modeConst {
		t = b.matchType(p)
		if t == nil {
			b.e.errorf(codeInternal, b.op.Name, "op %s の配線の `const %s = match` の型が決まらない（検査を通った IR には来ないはずの形）", b.op.Name, bindName)
			return
		}
		x = golang.LocalName(bindName, b.e.reserved)
		b.taken[x] = true
	}
	if a := m.SubjectAtom(); a != nil {
		b.line("// match " + subjectText(a) + " by " + m.Classify)
	} else {
		b.line("// match by " + m.Classify)
	}
	if mode == modeConst {
		b.line("var " + x + " " + b.f.goType(t))
	}
	switch p.form {
	case "nil", "len":
		b.matchIf(p, mode, x, t)
	default:
		b.matchSwitch(p, mode, x, t)
	}
	if mode == modeConst {
		b.scopes[len(b.scopes)-1].vars[bindName] = local{expr: x, typ: t}
		b.markUnused(x, bindName, rest)
	}
}

// matchIf は、二つに分ける形（`if`）。アームを書いた順によらず、値のある場合を if の枝にする。
func (b *bodyGen) matchIf(p *matchPlan, mode matchMode, x string, t ir.Type) {
	m := p.m
	valueCase, otherCase := ir.CaseHasItems, ir.CaseEmpty
	if p.classify != nil {
		valueCase, otherCase = "", ""
		for _, cs := range p.classify.Cases {
			valueCase = cs.Name // presence の分類は、名前の付いたケースが一つだけ
		}
	}
	valueArm, otherArm := armFor(m, valueCase), armFor(m, otherCase)
	if valueArm == nil || otherArm == nil {
		b.e.errorf(codeInternal, b.op.Name, "op %s の配線の match（%s）のアームが、すべてのケースを尽くしていない（検査を通った IR には来ないはずの形）", b.op.Name, m.Classify)
		return
	}
	cond, neg := p.subject+" != nil", p.subject+" == nil"
	if p.form == "len" {
		cond, neg = "len("+p.subject+") != 0", "len("+p.subject+") == 0"
	}
	if valueArm == otherArm {
		// 一つのアームが両方のケースを並べていれば、分けずに、そのアームの本体だけを書く
		text, terminated := b.branch(p, valueArm, mode, x, t, false)
		if text == "" && p.freshState {
			b.line("_ = " + p.subject)
		}
		b.buf.WriteString(text)
		b.terminated = terminated
		return
	}
	if mode == modeReturn {
		// else を書かず、もう一方をそのまま後ろに続ける（どの枝も return で終わる）
		b.depth++
		value, _ := b.branch(p, valueArm, mode, x, t, true)
		b.depth--
		b.line("if " + cond + " {")
		b.buf.WriteString(value)
		b.line("}")
		other, _ := b.branch(p, otherArm, mode, x, t, true)
		b.buf.WriteString(other)
		b.terminated = true
		return
	}
	b.depth++
	value, valueEnds := b.branch(p, valueArm, mode, x, t, true)
	other, otherEnds := b.branch(p, otherArm, mode, x, t, true)
	b.depth--
	switch {
	case value != "" && other != "":
		b.line("if " + cond + " {")
		b.buf.WriteString(value)
		b.line("} else {")
		b.buf.WriteString(other)
		b.line("}")
		b.terminated = valueEnds && otherEnds
	case value != "":
		b.line("if " + cond + " {")
		b.buf.WriteString(value)
		b.line("}")
	case other != "":
		// if の枝がなければ、条件を逆にする
		b.line("if " + neg + " {")
		b.buf.WriteString(other)
		b.line("}")
	case p.freshState:
		b.line("_ = " + p.subject)
	}
}

// matchSwitch は `switch` の形: アームごとに case を一つ。並びは、アームの最初のケースの、分類の宣言での順。default のアームは最後。
func (b *bodyGen) matchSwitch(p *matchPlan, mode matchMode, x string, t ir.Type) {
	e, m, c := b.e, p.m, p.classify
	order := map[string]int{}
	for i, cs := range c.Cases {
		order[cs.Name] = i
	}
	arms := make([]*ir.MatchArm, len(m.Arms))
	for i := range m.Arms {
		arms[i] = &m.Arms[i]
	}
	rank := func(a *ir.MatchArm) int {
		if a.Cases.Default || len(a.Cases.Cases) == 0 {
			return len(c.Cases)
		}
		return order[a.Cases.Cases[0]]
	}
	sort.SliceStable(arms, func(i, j int) bool { return rank(arms[i]) < rank(arms[j]) })

	qualifier := b.f.im.Qualifier(e.declNS[c.Name])
	name := e.typeName(c.Name)
	hasDefaultClause := false
	switch p.form {
	case "type":
		b.line("switch " + p.subject + ".(type) {")
	case "value":
		k := b.kind()
		b.line(k + ", err := " + qualifier + "Classify" + name + "(o." + e.fields.classify[c.Name] + ", " + p.subject + ")")
		b.fail()
		b.line("switch " + k + " {")
	case "state":
		k := b.kind()
		b.line(k + ", err := Classify" + name + "(ctx, o." + e.units[c.Name].Param + ")")
		b.fail()
		b.line("switch " + k + " {")
	}
	allEnd := true
	for _, a := range arms {
		switch {
		case p.form == "type" && a.Cases.Default:
			b.line("default:")
			hasDefaultClause = true
		case p.form == "type":
			var types []string
			u := e.unionOf(p.subjectType)
			for _, k := range a.Cases.Cases {
				cs := c.Case(k)
				if u == nil || cs == nil || len(cs.As) == 0 {
					continue
				}
				if arm, ok := e.variantArm(u, cs.As[0].Type, c.Name); ok {
					types = append(types, e.armGoType(b.f, u, arm))
				}
			}
			b.line("case " + strings.Join(types, ", ") + ":")
		default:
			var consts []string
			if a.Cases.Default {
				consts = append(consts, qualifier+golang.CaseConst(name, "default"))
			}
			for _, k := range a.Cases.Cases {
				consts = append(consts, qualifier+golang.CaseConst(name, k))
			}
			b.line("case " + strings.Join(consts, ", ") + ":")
		}
		b.depth++
		text, ends := b.branch(p, a, mode, x, t, false)
		b.depth--
		b.buf.WriteString(text)
		allEnd = allEnd && ends
	}
	b.line("}")
	// どの節も関数を抜ける switch で、default の節を持たないものの後ろには、`return <零値>, &runtime.ClassificationError{…}` を置く
	// （判定関数が先に同じ誤りを返すので、ここには来ない。Go が、関数の終わりの return を求めるため）
	if (mode == modeReturn || allEnd) && !hasDefaultClause {
		b.returnErr("&" + b.f.im.Add(runtimePath, "runtime") + ".ClassificationError{Classify: " + strconv.Quote(c.Name) + "}")
	} else if mode == modeReturn || allEnd {
		b.terminated = true
	}
}
