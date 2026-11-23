package gobase

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは wiring.go（docs/plugin-go.md「配線の本体」）: 単位の対象の配線ごとに、ops のメソッドを一つ。
// 実行の順序は行の順で、並列にはしない。継ぎ目を持つ配線は、メソッドの名前が中身の名前（`loginBody`）になる。
// 配線も、配線が呼ぶもの（op、read、効果、分類の判定関数）も、最初の引数に ctx を取り、最後の戻り値に error を返す
// （同「op の形」）: 呼び出しには、いつも ctx を渡し、失敗は、いつも error で返す。panic は出さない。

// ptrHelper は、値をポインタにする補助（docs/plugin-go.md「値」）。wiring.go が使っていれば、その末尾に一つだけ出す。
const ptrHelper = "func ptr[T any](v T) *T { return &v }\n"

// genWiring は定義のパッケージの wiring.go。単位の対象の配線がなければ nil。
func (e *emitter) genWiring() *fileGen {
	any := false
	for _, os := range e.shape.Ops {
		any = any || os.Place == golang.Wiring || hasBodied(os)
	}
	if !any {
		return nil
	}
	f := e.newFile(fileWiring, true)
	for _, os := range e.shape.Ops {
		if os.Place == golang.Wiring {
			b := newBody(e, f, os)
			b.gen()
		}
		// 本体のあるデコレータの外側（docs/plugin-go.md「本体のあるデコレータ」）: 適用ごとに、メソッド一つ。書かれた順
		for i, l := range os.Layers {
			if l.Bodied {
				e.genWrap(f, os, i)
			}
		}
	}
	if f.usesPtr {
		f.print(ptrHelper)
	}
	return f
}

// through は、op の中身を包むものの呼び名: 層が、継ぎ目だけなら「継ぎ目」、本体のあるデコレータを含めば「デコレータ」。
func through(os golang.OpShape) string {
	if hasBodied(os) {
		return "デコレータ"
	}
	return "継ぎ目"
}

// hasBodied は、op が、本体のあるデコレータの層を持つか。
func hasBodied(os golang.OpShape) bool {
	for _, l := range os.Layers {
		if l.Bodied {
			return true
		}
	}
	return false
}

// wrapResult は、本体のあるデコレータの外側で、包んだ関数の呼び出しの結果を受ける val の名前（docs/plugin-go.md「本体のあるデコレータ」）。
const wrapResult = "result"

// wrapStatements は、デコレータの本体の文を、値を返す op の配線として書ける形に、直す: 包んだ関数の呼び出し（か、包んだ関数の呼び出しのアームを
// 持つ match）が、最後の文なら、その return にする。後ろに文が続くなら、結果を val に受けて、最後に、その val を return する。
// 値を返さない op では、何も変えない。
func wrapStatements(stmts []ir.Statement, o *ir.Op) []ir.Statement {
	if o.Returns == nil || ir.IsNever(o.Returns) {
		return stmts
	}
	name := wrapResult
	ir.WalkStatements(stmts, func(s ir.Statement) {
		if v, ok := s.(ir.ValStmt); ok && v.Name == name {
			name += "_"
		}
	})
	proceeds := func(m ir.Match) bool {
		for _, a := range m.Arms {
			if c, ok := a.Body.(ir.CallExpr); ok && c.Call.IsProceed() {
				return true
			}
		}
		return false
	}
	out := make([]ir.Statement, 0, len(stmts)+1)
	held := false
	for i, st := range stmts {
		last := i == len(stmts)-1
		var expr ir.Expr
		var span ir.Span
		switch x := st.(type) {
		case ir.CallStmt:
			if x.Call.IsProceed() {
				expr, span = ir.CallExpr{Call: x.Call}, x.Span
			}
		case ir.MatchStmt:
			if proceeds(x.Match) {
				expr, span = ir.MatchExpr{Match: x.Match}, x.Span
			}
		case ir.TryStmt:
			// try { next(); … }: 結果の val を、try の前に、空で宣言し、try のブロックの中で入れる（docs/plugin-go.md「例外を受ける文」）
			if len(x.Body) > 0 {
				if cs, isCall := x.Body[0].(ir.CallStmt); isCall && cs.Call.IsProceed() {
					body := append([]ir.Statement{ir.AssignStmt{Name: name, Expr: ir.CallExpr{Call: cs.Call}, Span: cs.Span}}, x.Body[1:]...)
					x.Body = body
					out = append(out, ir.ValStmt{Name: name, Type: o.Returns, Span: x.Span}, x)
					held = true
					continue
				}
			}
		}
		switch {
		case expr == nil:
			out = append(out, st)
		case last:
			out = append(out, ir.ReturnStmt{Expr: expr, Span: span})
		default:
			out = append(out, ir.ValStmt{Name: name, Type: o.Returns, Expr: expr, Span: span})
			held = true
		}
	}
	if held {
		out = append(out, ir.ReturnStmt{Expr: ir.AtomExpr{Atom: ir.NameAtom{Path: []string{name}}}})
	}
	return out
}

// genWrap は、op os に付いた、本体のあるデコレータ（os.Layers[i]）の外側のメソッド（docs/plugin-go.md「本体のあるデコレータ」）。
// 本体を、op の配線として書く。包んだ関数の呼び出し は、内側の層を、op の引数のまま呼ぶ。
func (e *emitter) genWrap(f *fileGen, os golang.OpShape, i int) {
	o := e.op(os.FQN)
	l := os.Layers[i]
	dec, ok := e.lookup(l.Decorator).(*ir.Decorator)
	if o == nil || !ok || dec.Body == nil {
		e.errorf(codeInternal, os.FQN, "op %s に付いたデコレータ %s の本体が Program にない（生成器の不具合）", os.FQN, l.Decorator)
		return
	}
	syn := *o
	syn.Body = &ir.Wiring{Statements: wrapStatements(dec.Body.Statements, o)}
	b := newBodyOf(e, f, os, &syn)
	b.proceed = e.layerCall(f, os, o, i+1)
	// デコレータの、値の引数: 適用に書いたリテラルを、その場に書く
	for _, p := range dec.Params {
		tp, isType := p.Type.(ir.TypeParam)
		if !isType || len(tp.Types) != 1 || p.Rest {
			continue
		}
		typ := tp.Types[0]
		if p.Optional {
			typ = ir.OptionalType{Of: typ}
		}
		expr := "nil"
		for _, a := range l.Args {
			if a.Param != p.Name {
				continue
			}
			if lv, isLit := a.Value.(ir.LiteralValue); isLit {
				expr = f.literal(lv.Literal, typ)
			} else {
				e.errorf(codeInternal, os.FQN, "op %s に付いた @%s の引数 %s が、リテラルでない（本体のあるデコレータの、値の引数に渡せるのは、リテラルだけ）", os.FQN, l.Decorator, p.Name)
			}
		}
		b.scopes[0].vars[p.Name] = local{expr: expr, typ: typ}
	}
	b.block(syn.Body.Statements)
	if !b.terminated && b.ret == nil {
		b.line("return nil")
	}
	inner := e.implExpr(os)
	if i+1 < len(os.Layers) {
		inner = "@" + os.Layers[i+1].Decorator
	}
	f.print(comment(l.Method + " は op " + o.Name + " に付いた @" + l.Decorator + " の外側（デコレータの本体）。包んだ関数の呼び出しが、内側（" + inner + "）を呼ぶ。"))
	params, results := golang.Signature(o, f.im, e.reserved)
	f.printf("func (o *ops) %s {\n%s}\n\n", method(l.Method, params, results), b.buf.String())
}

// local は、配線の中の名前（引数、束縛）一つ: Go の式と、その値の型。
type local struct {
	expr string
	typ  ir.Type
}

// scope は、Go のブロック一つで宣言した変数。
type scope struct {
	vars   map[string]local  // Hozo の名前（引数・val・for の変数） → Go の変数
	states map[string]string // 状態の FQN → その値を読んだ Go の変数
	err    bool              // このブロックで、err を作る文（…, err := …）を、もう書いたか
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
	modeConst                   // val x = match …
	modeStmt                    // match …;
	modeAssign                  // x = match …（var x は、空の宣言の所に、もう、ある）
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

	ret ir.Type // 配線の戻り値の型（void なら nil）
	// terminated は、直前に書いた文が、関数を抜ける文（return）か。
	terminated bool
	// proceed は、本体のあるデコレータの外側を書いているときの、包んだ関数の呼び出しの式（内側の層の呼び出し）。配線の本体では空。
	proceed string
	// catches は、今、中にいる catch のブロック（外から内へ）。
	catches []catchVar
}

// catchVar は、catch のブロック一つ（docs/plugin-go.md「例外を受ける文」）: 仕様の、受けた例外の名前（name）と、
// 受けた失敗の変数（err。error）、仕様の例外として取り出した変数（exc。runtime.Exception）。
type catchVar struct {
	name, err, exc string
}

// catchOf は、受けた例外 name の catch のブロック（いちばん内側）。
func (b *bodyGen) catchOf(name string) (catchVar, bool) {
	for i := len(b.catches) - 1; i >= 0; i-- {
		if b.catches[i].name == name {
			return b.catches[i], true
		}
	}
	return catchVar{}, false
}

func newBody(e *emitter, f *fileGen, os golang.OpShape) *bodyGen {
	return newBodyOf(e, f, os, e.op(os.FQN))
}

// newBodyOf は、op o の形（引数と戻り値）のメソッドの中身を、o.Body の文から書く bodyGen。
func newBodyOf(e *emitter, f *fileGen, os golang.OpShape, o *ir.Op) *bodyGen {
	b := &bodyGen{e: e, f: f, op: o, os: os, buf: &strings.Builder{}, depth: 1, taken: map[string]bool{}}
	if o.Returns != nil && !ir.IsNever(o.Returns) {
		b.ret = o.Returns
	}
	b.push()
	for _, p := range o.Params {
		name := golang.LocalName(p.Name, e.reserved)
		b.scopes[0].vars[p.Name] = local{expr: name, typ: p.Type}
		b.taken[name] = true
	}
	// val と for の変数の名前は、生成する変数（状態を読んだ値、途中の値）より先に押さえる
	ir.WalkStatements(o.Body.Statements, func(s ir.Statement) {
		switch c := s.(type) {
		case ir.ValStmt:
			b.taken[golang.LocalName(c.Name, e.reserved)] = true
		case ir.ForStmt:
			b.taken[golang.LocalName(c.Var, e.reserved)] = true
		case ir.TryStmt:
			b.taken[golang.LocalName(c.Catch.Name, e.reserved)] = true
		}
	})
	return b
}

func (b *bodyGen) push() {
	b.scopes = append(b.scopes, &scope{vars: map[string]local{}, states: map[string]string{}})
}
func (b *bodyGen) pop() { b.scopes = b.scopes[:len(b.scopes)-1] }

// line は、今の字下げで一行書く。err を作る文（…, err := …）なら、今のブロックで、err が見えるようになる。
func (b *bodyGen) line(s string) {
	b.buf.WriteString(strings.Repeat("\t", b.depth) + s + "\n")
	b.terminated = false
	if strings.Contains(s, ", err := ") && !strings.HasPrefix(s, "if ") {
		b.scopes[len(b.scopes)-1].err = true
	}
}

// needErr は、`x, err = <呼び出し>` の前に呼ぶ: err が、まだ、見えていなければ、`var err error` を置く
// （docs/plugin-go.md「文」）。
func (b *bodyGen) needErr() {
	for _, sc := range b.scopes {
		if sc.err {
			return
		}
	}
	b.line("var err error")
	b.scopes[len(b.scopes)-1].err = true
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

// returnErr は `return <零値>, <errExpr>`。どの配線も error を返すので、返す先は、いつもある。
func (b *bodyGen) returnErr(errExpr string) {
	b.line("return " + b.zeroPrefix() + errExpr)
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
	b.block(o.Body.Statements)
	if !b.terminated && b.ret == nil {
		b.line("return nil") // 本体の終わり（戻り値のない配線）
	}
	name := b.os.Method
	if b.os.Body != "" {
		// 継ぎ目を持つ配線は、中身の名前のメソッド。op のコメントは、Ops のメソッドにだけ書く
		name = b.os.Body
		b.f.print(comment(name + " は op " + o.Name + " の中身（配線の本体）。外側の " + b.os.Method + " が、" + through(b.os) + "を通して呼ぶ。"))
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

// nilable は、型 t の Go の型が nil を持てるか（optional、array、インデックスの model、union）。
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
// Go の型が同じなら、そのまま。array の要素をそろえる必要があれば、文の前にループを置く。
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
			})
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
func (b *bodyGen) convertEach(src string, to ir.Type, length string, assign func(dst, src, idx, el string)) string {
	dst := b.temp()
	goTo := b.f.goType(to)
	b.line("var " + dst + " " + goTo)
	b.line("if " + src + " != nil {")
	b.depth++
	b.line(dst + " = make(" + goTo + ", " + length + ")")
	idx, el := b.fresh("i"), b.fresh("e")
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
	if optional && m.Index == nil {
		// インデックスの model は、省略可能でも、マップのまま（nil が「なし」）: & を付けない
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

// callInfo は、呼び出し一つを Go にしたもの。呼ぶ先は、どれも、最後の戻り値に error を返す。
type callInfo struct {
	expr  string
	ret   ir.Type // 呼ぶ先の戻り値の型（void と never の op と、効果は nil）
	never bool    // 「返らない」呼び出し（never を返す op）
}

// target は、呼び出しの先（docs/plugin-go.md「呼び出し」の表の行）を、同じ形のメソッドの呼び出しとして見たもの:
// op か、read か、効果（宣言したものと、直に呼ぶ組み込みの効果）。どれも、最初の引数に ctx を取り、最後の戻り値に error を返す。
type target struct {
	recv   string       // メソッドの前: `o.`（同じ名前空間の op）、`o.<名前空間の引数名>.`、`o.<状態の単位の引数名>.`
	method string       // メソッドの名前
	params []ir.OpParam // ctx の後ろの引数の並び（Op.params、Read.params、Effect.params。直に呼ぶ組み込みの効果は、value 一つか、なし）
	ret    ir.Type      // 戻り値の型（void と never の op と、効果は nil）
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
	case ir.ProceedCallee:
		// デコレータの本体の 包んだ関数の呼び出し: 内側の層。形は、op のまま（式は call が作る）
		t := target{never: ir.IsNever(b.op.Returns)}
		if b.op.Returns != nil && !t.never {
			t.ret = b.op.Returns
		}
		return t, true
	case ir.OpCallee:
		o := e.op(callee.Op)
		if o == nil {
			break
		}
		t := target{recv: "o.", method: golang.MethodName(o.Name), params: o.Params, isOp: true, never: ir.IsNever(o.Returns)}
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
		return target{recv: "o." + u.Param + ".", method: golang.MethodName(r.Name), params: r.Params, ret: r.Returns}, true
	case ir.EffectCallee:
		switch fx := callee.Effect.(type) {
		case ir.NamedEffect:
			// 効果は、宛先の単位の命令
			x := e.effect(fx.Effect)
			u, placed := e.unitOf[fx.Effect]
			if x == nil || !placed {
				break
			}
			return target{recv: "o." + u.Param + ".", method: golang.MethodName(x.Name), params: x.Params}, true
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
			t := target{recv: recv, method: golang.BuiltinMethod(string(fx.Operation))}
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
	if c.IsProceed() {
		return callInfo{expr: b.proceed, ret: t.ret, never: t.never}
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
	args := []string{"ctx"}
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
	return callInfo{expr: t.recv + t.method + "(" + strings.Join(args, ", ") + ")", ret: t.ret, never: t.never}
}

// throwValue は、アームの本体の `throw E`、`throw E(<値>, …)` が作る例外の値（docs/plugin-go.md「match」の「アームの本体」）:
// 例外の構造体のリテラル。フィールドは例外の引数（Exception.params の順）で、いつもすべて書く。値は、引数の型を当てはめる型にして
// 「値」の規則で書く。渡されていない省略可能な引数は nil。
func (b *bodyGen) throwValue(t *ir.ThrowBody) string {
	if t.Caught != "" {
		// throw e: 受けた失敗を、包み直さずに返す
		cv, ok := b.catchOf(t.Caught)
		if !ok {
			b.e.errorf(codeInternal, b.op.Name, "op %s の配線の throw %s が、catch のブロックの外にある（検査を通った IR には来ないはずの形）", b.op.Name, t.Caught)
			return "nil"
		}
		return cv.err
	}
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
	used := false
	ir.WalkStatements(stmts, func(s ir.Statement) {
		if used {
			return
		}
		if f, ok := s.(ir.ForStmt); ok {
			used = inAtom(f.Of)
			return
		}
		if t, ok := s.(ir.ThrowStmt); ok {
			for _, a := range t.Throw.Args {
				if inAtom(a.Value) {
					used = true
				}
			}
			return
		}
		if x, ok := ir.StatementExpr(s); ok {
			used = inExpr(x)
		}
	})
	return used
}

// bind は、束縛名 name を、型 typ の Go の変数として、今のブロックに足す。
func (b *bodyGen) bind(name string, typ ir.Type) string {
	goName := golang.LocalName(name, b.e.reserved)
	b.taken[goName] = true
	b.scopes[len(b.scopes)-1].vars[name] = local{expr: goName, typ: typ}
	return goName
}

// stmt は文一つ。rest は、同じブロックの、後ろの文（val が読まれるかの判定）。
func (b *bodyGen) stmt(s ir.Statement, rest []ir.Statement) {
	switch x := s.(type) {
	case ir.ValStmt:
		if x.Expr == nil {
			// 空の宣言: var x T
			name := b.bind(x.Name, x.Type)
			b.line("var " + name + " " + b.f.goType(x.Type))
			b.markUnused(name, x.Name, rest)
			return
		}
		switch ex := x.Expr.(type) {
		case ir.CallExpr:
			var states []string
			statesOfCall(&ex.Call, &states)
			b.read(states, nil)
			ci := b.call(&ex.Call)
			if x.Type != nil && ci.ret != nil && !b.sameGo(ci.ret, x.Type) {
				// 型を書いた val で、型をそろえる
				v := b.temp()
				b.line(v + ", err := " + ci.expr)
				b.fail()
				name := b.bind(x.Name, x.Type)
				b.line(name + " := " + b.convert(v, ci.ret, x.Type))
				b.markUnused(name, x.Name, rest)
				return
			}
			name := b.bind(x.Name, ci.ret)
			b.line(name + ", err := " + ci.expr)
			b.fail()
			b.markUnused(name, x.Name, rest)
		case ir.AtomExpr:
			var states []string
			statesOfAtom(ex.Atom, &states)
			b.read(states, nil)
			// 値の型と、val の型（型を書いた val）が、Go で違うとき（か、値がレコードリテラルのとき）だけ、型を付けて宣言する
			if at := b.atomType(ex.Atom); x.Type != nil && (at == nil || !b.sameGo(at, x.Type)) {
				expr, _ := b.atom(ex.Atom, x.Type)
				name := b.bind(x.Name, x.Type)
				b.line("var " + name + " " + b.f.goType(x.Type) + " = " + expr)
				b.markUnused(name, x.Name, rest)
				return
			}
			expr, typ := b.atom(ex.Atom, nil)
			name := b.bind(x.Name, typ)
			b.line(name + " := " + expr)
			b.markUnused(name, x.Name, rest)
		case ir.MatchExpr:
			b.match(&ex.Match, modeConst, x.Name, x.Type, rest)
		}
	case ir.AssignStmt:
		l, ok := b.lookupVar(x.Name)
		if !ok {
			b.e.errorf(codeInternal, b.op.Name, "op %s の配線が、宣言のない val %s に値を入れている（検査を通った IR には来ないはずの形）", b.op.Name, x.Name)
			return
		}
		switch ex := x.Expr.(type) {
		case ir.CallExpr:
			var states []string
			statesOfCall(&ex.Call, &states)
			b.read(states, nil)
			ci := b.call(&ex.Call)
			if ci.ret != nil && l.typ != nil && !b.sameGo(ci.ret, l.typ) {
				v := b.temp()
				b.line(v + ", err := " + ci.expr)
				b.fail()
				b.line(l.expr + " = " + b.convert(v, ci.ret, l.typ))
				return
			}
			b.needErr()
			b.line(l.expr + ", err = " + ci.expr)
			b.fail()
		case ir.AtomExpr:
			var states []string
			statesOfAtom(ex.Atom, &states)
			b.read(states, nil)
			expr, _ := b.atom(ex.Atom, l.typ)
			b.line(l.expr + " = " + expr)
		case ir.MatchExpr:
			b.match(&ex.Match, modeAssign, x.Name, l.typ, nil)
		}
	case ir.CollectStmt:
		l, ok := b.lookupVar(x.Name)
		if !ok {
			b.e.errorf(codeInternal, b.op.Name, "op %s の配線が、宣言のない val %s に集めている（検査を通った IR には来ないはずの形）", b.op.Name, x.Name)
			return
		}
		var elem ir.Type
		if arr, isArray := strip(l.typ).(ir.ArrayType); isArray {
			elem = arr.Of
		}
		switch ex := x.Expr.(type) {
		case ir.CallExpr:
			var states []string
			statesOfCall(&ex.Call, &states)
			b.read(states, nil)
			ci := b.call(&ex.Call)
			v := b.temp()
			b.line(v + ", err := " + ci.expr)
			b.fail()
			value := v
			if elem != nil && ci.ret != nil {
				value = b.convert(v, ci.ret, elem)
			}
			b.line(l.expr + " = append(" + l.expr + ", " + value + ")")
		case ir.AtomExpr:
			var states []string
			statesOfAtom(ex.Atom, &states)
			b.read(states, nil)
			expr, _ := b.atom(ex.Atom, elem)
			b.line(l.expr + " = append(" + l.expr + ", " + expr + ")")
		case ir.MatchExpr:
			// v<n> に、val v<n> = match と同じに受けてから、集める
			v := b.temp()
			b.matchInto(&ex.Match, v, elem)
			b.line(l.expr + " = append(" + l.expr + ", " + v + ")")
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
			b.match(&ex.Match, modeReturn, "", nil, nil)
		}
	case ir.CallStmt:
		var states []string
		statesOfCall(&x.Call, &states)
		b.read(states, nil)
		b.callStmt(b.call(&x.Call))
	case ir.MatchStmt:
		b.match(&x.Match, modeStmt, "", nil, nil)
	case ir.DisjointStmt:
		// 直列に出す: // disjoint の一行と、一つの Go のブロック。ブロックのアームは、Go のブロック。文の一つのアームは、その文のまま
		b.line("// disjoint")
		b.line("{")
		b.depth++
		b.push()
		for _, arm := range x.Arms {
			if !arm.Block && len(arm.Statements) == 1 {
				b.stmt(arm.Statements[0], nil)
				continue
			}
			b.line("{")
			b.depth++
			b.push()
			b.block(arm.Statements)
			b.pop()
			b.depth--
			b.line("}")
		}
		b.pop()
		b.depth--
		b.line("}")
	case ir.TryStmt:
		b.tryStmt(x)
	case ir.ThrowStmt:
		var states []string
		for i := range x.Throw.Args {
			statesOfAtom(x.Throw.Args[i].Value, &states)
		}
		b.read(states, nil)
		b.returnErr(b.throwValue(&x.Throw))
	case ir.ForStmt:
		var states []string
		statesOfAtom(x.Of, &states)
		b.read(states, nil)
		seq, _ := b.atom(x.Of, nil)
		if x.Disjoint {
			b.line("// disjoint")
		}
		if usesName(x.Body, x.Var) {
			v := golang.LocalName(x.Var, b.e.reserved)
			b.line("for _, " + v + " := range " + seq + " {")
			b.depth++
			b.push()
			b.scopes[len(b.scopes)-1].vars[x.Var] = local{expr: v, typ: x.VarType}
		} else {
			b.line("for range " + seq + " {")
			b.depth++
			b.push()
		}
		b.block(x.Body)
		b.pop()
		b.depth--
		b.line("}")
	}
}

// tryStmt は、例外を受ける文（docs/plugin-go.md「例外を受ける文」）: try のブロックを、その場で呼ぶ関数にして、その失敗を、
// catch のブロックで受ける。受けるのは、仕様の例外（runtime.Exception）だけで、そうでない失敗は、そのまま返す。
func (b *bodyGen) tryStmt(x ir.TryStmt) {
	caught := b.fresh("caught")
	b.line("// try")
	// 関数は、基底が出すほかの関数と同じに、ctx を取り、error を返す（docs/plugin-go.md「op の形」）
	b.line("if " + caught + " := func(ctx " + b.f.im.Add("context", "context") + ".Context) error {")
	b.depth++
	b.push()
	ret := b.ret
	b.ret = nil // 関数の戻り値は error だけ: 失敗の扱いは return err
	b.block(x.Body)
	if !b.terminated {
		b.line("return nil")
	}
	b.ret = ret
	b.pop()
	b.depth--
	b.line("}(ctx); " + caught + " != nil {")
	b.depth++
	b.push()
	// 受けた例外の名前は、val と同じに、先に押さえてある（newBodyOf）。見えている名前と当たらない（WIRE-4）
	exc := golang.LocalName(x.Catch.Name, b.e.reserved)
	b.line("// catch (" + x.Catch.Name + ")")
	b.line("var " + exc + " " + b.f.im.Add(runtimePath, "runtime") + ".Exception")
	b.line("if !" + b.f.im.Add("errors", "errors") + ".As(" + caught + ", &" + exc + ") {")
	b.depth++
	b.returnErr(caught)
	b.depth--
	b.line("}")
	b.catches = append(b.catches, catchVar{name: x.Catch.Name, err: caught, exc: exc})
	b.block(x.Catch.Body)
	b.catches = b.catches[:len(b.catches)-1]
	b.pop()
	b.depth--
	b.line("}")
}

// caughtSwitch は、受けた例外の match（docs/plugin-go.md「例外を受ける文」）: 型の switch。アームごとに case 一つで、そのアームが
// 受けるケースの例外の型を、分類の宣言の順に並べる。残りのケースを受けるアームは default。残りのケースがない分類では、
// default は、受けた失敗を、そのまま返す。mode、x、t は、アームの本体の使われ方（matchSwitch と同じ）。
func (b *bodyGen) caughtSwitch(m *ir.Match, cs ir.CaughtSubject, mode matchMode, x string, t ir.Type) {
	cv, ok := b.catchOf(cs.Name)
	c, isClassify := b.e.lookup(m.Classify).(*ir.Classify)
	if !ok || !isClassify {
		b.e.errorf(codeInternal, b.op.Name, "op %s の配線の、受けた例外 %s の match が、catch のブロックの外にあるか、分類 %s が Program にない（検査を通った IR には来ないはずの形）", b.op.Name, cs.Name, m.Classify)
		return
	}
	b.line("switch " + cv.exc + ".(type) {")
	hasDefault := false
	for i := range m.Arms {
		a := &m.Arms[i]
		var types []string
		rest := false
		for _, k := range c.Cases {
			if !slices.Contains(a.Cases, k.Name) {
				continue
			}
			rest = rest || k.Rest
			for _, exc := range k.Exceptions {
				types = append(types, b.f.goType(ir.Ref(exc)))
			}
		}
		switch {
		case rest:
			hasDefault = true
			b.line("default: // " + armCasesText(a))
		case len(types) == 0:
			continue
		default:
			b.line("case " + strings.Join(types, ", ") + ": // " + armCasesText(a))
		}
		b.depth++
		b.push()
		b.read(armStates(a), nil)
		b.armBody(nil, a, mode, x, t)
		b.pop()
		b.depth--
	}
	if !hasDefault {
		b.line("default:")
		b.depth++
		b.returnErr(cv.err)
		b.depth--
	}
	b.line("}")
}

// block は、ブロック一つの文の並びを書く。
func (b *bodyGen) block(stmts []ir.Statement) {
	for i, s := range stmts {
		b.stmt(s, stmts[i+1:])
	}
}

// markUnused は、束縛が下の行で使われていなければ `_ = x` を足す（WIRE-8 は warning。Go は、使わない変数をコンパイルしない）。
func (b *bodyGen) markUnused(goName, hozoName string, rest []ir.Statement) {
	if !usesName(rest, hozoName) {
		b.line("_ = " + goName)
	}
}

// returnValue は `return <値>, nil`。
func (b *bodyGen) returnValue(expr string) {
	b.line("return " + expr + ", nil")
	b.terminated = true
}

// returnNever は、「返らない」呼び出しの文と return: `return <零値>, <呼び出し>`。
func (b *bodyGen) returnNever(ci callInfo) {
	b.returnErr(ci.expr)
}

// returnCall は `return 呼び出し`（docs/plugin-go.md「文」）: 型をそろえる必要がなければ `return <呼び出し>`（戻り値の並びは、
// 呼ぶ先も、この配線も `(T, error)`）。型をそろえる必要があれば、v<n> に束縛して失敗の扱いを置いてから
// `return <そろえた値>, nil`。
func (b *bodyGen) returnCall(ci callInfo) {
	switch {
	case ci.never:
		b.returnNever(ci)
	case ci.ret == nil || b.ret == nil:
		// 値のない呼び出し（検査を通った IR では、return には来ない）
		b.callStmt(ci)
		b.line("return nil")
		b.terminated = true
	case b.sameGo(ci.ret, b.ret):
		b.line("return " + ci.expr)
		b.terminated = true
	default:
		v := b.temp()
		b.line(v + ", err := " + ci.expr)
		b.fail()
		b.returnValue(b.convert(v, ci.ret, b.ret))
	}
}

// callStmt は `呼び出し;`: `if err := <呼び出し>; err != nil { return <零値>, err }`。値を返す op なら、値を捨てる
// （`if _, err := …`）。
func (b *bodyGen) callStmt(ci callInfo) {
	if ci.never {
		b.returnNever(ci)
		return
	}
	lhs := "err"
	if ci.ret != nil {
		lhs = "_, err"
	}
	b.line("if " + lhs + " := " + ci.expr + "; err != nil {")
	b.depth++
	b.returnErr("err")
	b.depth--
	b.line("}")
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

// armCasesText は、アームが受けるケースの名前（`, ` でつなぐ）。else のアームも、受けるケースの名前
// （docs/plugin-go.md「match」: else のアームは、受けるケースを名前で並べたアームと、同じコードになる）。
func armCasesText(a *ir.MatchArm) string {
	return strings.Join(a.Cases, ", ")
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
				if len(cs.As) > 0 && cs.As[0].By == ir.NarrowByLiteral {
					p.form = "bool" // 対象が boolean の値（そうでない分類は、GO-9 が断っている）
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
	for _, name := range a.Cases {
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
				if golang.TypeDisplay(n.Type) != golang.TypeDisplay(from) {
					texts = append(texts, text+" は "+golang.TypeDisplay(n.Type))
				}
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
		// ケースで絞っただけで、もとの型が同じアームは、Go の型が変わらないので、コメントに書かない
		if golang.TypeDisplay(n.Type) != golang.TypeDisplay(from) {
			texts = append(texts, text+" は "+golang.TypeDisplay(n.Type))
		}
		ov.value, ov.base = subject, subject
		if !b.sameGo(from, n.Type) {
			switch by := p.narrowBy(a, n.State); {
			// 残りのケースで対象の型が変わるのは、残りのアームがちょうど一つの variant だけで、式は variant と同じ
			case by == ir.NarrowByVariant, by == ir.NarrowByElse:
				if u := b.e.unionOf(from); u != nil {
					// アームを探すのは、絞り込みを捨てた、もとの型で（アームの中の対象の型は case(C, Ks, <アームの型>)）
					if arm, ok := b.e.variantArm(u, strip(n.Type), p.m.Classify); ok {
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
	if t == nil { // そういう型がなければ、絞り込みを捨てた型で、同じことをする（docs/types.md「match の型」の 5.1）
		for _, cand := range typed {
			ok := true
			for _, other := range typed {
				ok = ok && b.e.assignable(strip(other), strip(cand))
			}
			if ok {
				t = strip(cand)
				break
			}
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
			v := b.temp()
			b.line(v + ", err := " + ci.expr)
			b.fail()
			b.line(x + " = " + b.convert(v, ci.ret, t))
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

// armFor は、ケース name を受けるアーム（MatchArm.cases。else のアームも、受けるケースを cases に持つ）。なければ nil。
func armFor(m *ir.Match, name string) *ir.MatchArm {
	for i := range m.Arms {
		for _, c := range m.Arms[i].Cases {
			if c == name {
				return &m.Arms[i]
			}
		}
	}
	return nil
}

// classificationError は、分類の誤りの値（docs/plugin-go.md「match」の「分類の誤りの枝」「switch の形」）。
func (b *bodyGen) classificationError(classify string) string {
	return "&" + b.f.im.Add(runtimePath, "runtime") + ".ClassificationError{Classify: " + strconv.Quote(classify) + "}"
}

// errorBranch は、分類の誤りの枝（docs/plugin-go.md「match」の「分類の誤りの枝」）の中身のテキスト:
// `return <零値>, &runtime.ClassificationError{Classify: "<分類の FQN>"}` の一行。枝の頭のコメントは書かない。
func (b *bodyGen) errorBranch(classify string) string {
	saved, terminated := b.buf, b.terminated
	b.buf = &strings.Builder{}
	b.returnErr(b.classificationError(classify))
	text := b.buf.String()
	b.buf, b.terminated = saved, terminated
	return text
}

// match は match を書く（docs/plugin-go.md「match」）: 分類の種類ごとの分け方と、使われ方ごとのアームの本体の組み合わせ。
//
// mode が modeConst（val x = match）なら、分ける前に `var x <型>` を置く。modeAssign（x = match）なら、置かない
// （空の宣言の所に、もう、ある）。typ は、val の型（書いた型か、解決した型）。nil なら、match の型。
func (b *bodyGen) match(m *ir.Match, mode matchMode, bindName string, typ ir.Type, rest []ir.Statement) {
	if cs, caught := m.Subject.(ir.CaughtSubject); caught {
		// 受けた例外の match（docs/plugin-go.md「例外を受ける文」）
		x := ""
		if mode == modeConst || mode == modeAssign {
			if typ == nil {
				b.e.errorf(codeInternal, b.op.Name, "op %s の配線の `val %s = match` の型が決まらない（検査を通った IR には来ないはずの形）", b.op.Name, bindName)
				return
			}
			if mode == modeAssign {
				l, _ := b.lookupVar(bindName)
				x = l.expr
			} else {
				x = golang.LocalName(bindName, b.e.reserved)
				b.taken[x] = true
			}
		}
		b.line("// match (" + cs.Name + " by " + m.Classify + ")")
		inner := mode
		if mode == modeConst {
			b.line("var " + x + " " + b.f.goType(typ))
		}
		if mode == modeAssign {
			inner = modeConst
		}
		b.caughtSwitch(m, cs, inner, x, typ)
		if mode == modeConst {
			b.scopes[len(b.scopes)-1].vars[bindName] = local{expr: x, typ: typ}
			b.markUnused(x, bindName, rest)
		}
		return
	}
	p := b.plan(m)
	if p == nil {
		return
	}
	x := ""
	t := typ
	if mode == modeConst || mode == modeAssign {
		if t == nil {
			t = b.matchType(p)
		}
		if t == nil {
			b.e.errorf(codeInternal, b.op.Name, "op %s の配線の `val %s = match` の型が決まらない（検査を通った IR には来ないはずの形）", b.op.Name, bindName)
			return
		}
		if mode == modeAssign {
			l, _ := b.lookupVar(bindName)
			x = l.expr
		} else {
			x = golang.LocalName(bindName, b.e.reserved)
			b.taken[x] = true
		}
	}
	b.matchHead(m)
	if mode == modeConst {
		b.line("var " + x + " " + b.f.goType(t))
	}
	inner := mode
	if mode == modeAssign {
		inner = modeConst // アームの本体は、val x = match と同じ（x に入れる）
	}
	switch p.form {
	case "nil", "len", "bool":
		b.matchIf(p, inner, x, t)
	default:
		b.matchSwitch(p, inner, x, t)
	}
	if mode == modeConst {
		b.scopes[len(b.scopes)-1].vars[bindName] = local{expr: x, typ: t}
		b.markUnused(x, bindName, rest)
	}
}

// matchHead は、分ける前の一行: `// match (<対象> by <分類>)`。
func (b *bodyGen) matchHead(m *ir.Match) {
	if a := m.SubjectAtom(); a != nil {
		b.line("// match (" + subjectText(a) + " by " + m.Classify + ")")
	} else {
		b.line("// match (by " + m.Classify + ")")
	}
}

// matchInto は、match の値を、新しい変数 v（型 typ。nil なら match の型）に受ける: `val v = match` と同じ形。
func (b *bodyGen) matchInto(m *ir.Match, v string, typ ir.Type) {
	if cs, caught := m.Subject.(ir.CaughtSubject); caught {
		if typ == nil {
			b.e.errorf(codeInternal, b.op.Name, "op %s の配線の、集める match の型が決まらない（検査を通った IR には来ないはずの形）", b.op.Name)
			return
		}
		b.line("// match (" + cs.Name + " by " + m.Classify + ")")
		b.line("var " + v + " " + b.f.goType(typ))
		b.caughtSwitch(m, cs, modeConst, v, typ)
		return
	}
	p := b.plan(m)
	if p == nil {
		return
	}
	t := typ
	if t == nil {
		t = b.matchType(p)
	}
	if t == nil {
		b.e.errorf(codeInternal, b.op.Name, "op %s の配線の、集める match の型が決まらない（検査を通った IR には来ないはずの形）", b.op.Name)
		return
	}
	b.matchHead(m)
	b.line("var " + v + " " + b.f.goType(t))
	switch p.form {
	case "nil", "len", "bool":
		b.matchIf(p, modeConst, v, t)
	default:
		b.matchSwitch(p, modeConst, v, t)
	}
}

// matchIf は、二つに分ける形（`if`）。アームを書いた順によらず、値のある場合を if の枝にする。
func (b *bodyGen) matchIf(p *matchPlan, mode matchMode, x string, t ir.Type) {
	m := p.m
	// 標準の分類のケース名は、形ごとに違う（list は HasItems / Empty、nullable は Some / None）
	valueCase, otherCase := ir.CaseHasItems, ir.CaseEmpty
	if m.Classify == ir.ClassifyNullable {
		valueCase, otherCase = ir.CaseSome, ir.CaseNone
	}
	if p.classify != nil {
		// presence の分類は、型を書いたケースが一つだけ。もう一方は、残りのケース（対象 as else）。残りのケースがなければ、
		// もう一方は、どのケースでもない（分類の誤りの枝）
		valueCase, otherCase = "", ""
		for _, cs := range p.classify.Cases {
			// もう一方は、ない側のケース（対象 as else か、対象 as none。Classify.absent）
			if cs.IsElse() || cs.Name == p.classify.Absent {
				otherCase = cs.Name
			} else {
				valueCase = cs.Name
			}
		}
	}
	if p.form == "bool" {
		// literal の分類: if の枝は、真の側のケース（as true のケース。なければ、残りのケース）。もう一方は、偽の側のケース
		// （as false のケース。なければ、残りのケース）。ケースのない側は、分類の誤りの枝
		valueCase, otherCase = "", ""
		rest := ""
		for _, cs := range p.classify.Cases {
			switch lit := literalOf(cs); {
			case cs.IsElse():
				rest = cs.Name
			case lit != nil && *lit:
				valueCase = cs.Name
			case lit != nil:
				otherCase = cs.Name
			}
		}
		if valueCase == "" {
			valueCase = rest
		}
		if otherCase == "" {
			otherCase = rest
		}
	}
	var valueArm, otherArm *ir.MatchArm
	if valueCase != "" {
		valueArm = armFor(m, valueCase)
	}
	if otherCase != "" {
		otherArm = armFor(m, otherCase)
	}
	if (valueArm == nil && (p.form != "bool" || valueCase != "")) || (otherCase != "" && otherArm == nil) || (valueArm == nil && otherArm == nil) {
		b.e.errorf(codeInternal, b.op.Name, "op %s の配線の match（%s）のアームが、すべてのケースを尽くしていない（検査を通った IR には来ないはずの形）", b.op.Name, m.Classify)
		return
	}
	cond, neg := p.subject+" != nil", p.subject+" == nil"
	if p.form == "len" {
		cond, neg = "len("+p.subject+") != 0", "len("+p.subject+") == 0"
	}
	if p.form == "bool" {
		cond, neg = p.subject, "!"+p.subject
	}
	if valueArm == otherArm {
		// 一つのアームが両方のケースを受けていれば、分けずに、そのアームの本体だけを書く
		text, terminated := b.branch(p, valueArm, mode, x, t, false)
		if text == "" && p.freshState {
			b.line("_ = " + p.subject)
		}
		b.buf.WriteString(text)
		b.terminated = terminated
		return
	}
	// otherBranch は、もう一方の枝: そのケースのアームか、ケースがなければ、分類の誤りの枝
	otherBranch := func() (string, bool) {
		if otherArm == nil {
			return b.errorBranch(m.Classify), true
		}
		return b.branch(p, otherArm, mode, x, t, true)
	}
	// valueBranch は、if の枝: そのケースのアームか、ケースがなければ（literal の分類の、真の側だけ）、分類の誤りの枝
	valueBranch := func() (string, bool) {
		if valueArm == nil {
			return b.errorBranch(m.Classify), true
		}
		return b.branch(p, valueArm, mode, x, t, true)
	}
	if mode == modeReturn {
		// else を書かず、もう一方をそのまま後ろに続ける（どの枝も return で終わる）
		b.depth++
		value, _ := valueBranch()
		b.depth--
		b.line("if " + cond + " {")
		b.buf.WriteString(value)
		b.line("}")
		other, _ := otherBranch()
		b.buf.WriteString(other)
		b.terminated = true
		return
	}
	b.depth++
	value, valueEnds := valueBranch()
	other, otherEnds := otherBranch()
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

// literalOf は、ケース cs の、literal の絞り込みの as に書いた、真偽の値。そういう as を持たなければ nil。
func literalOf(cs ir.ClassifyCase) *bool {
	for _, as := range cs.As {
		if lt, ok := as.Type.(ir.LiteralType); ok && as.By == ir.NarrowByLiteral {
			if v, isBool := lt.Literal.(ir.BooleanLiteral); isBool {
				return &v.Value
			}
		}
	}
	return nil
}

// matchSwitch は `switch` の形: アームごとに case を一つ。並びは、アームが受ける最初のケースの、分類の宣言での順。
// else のアームも、受けるケース（MatchArm.cases）を並べた case になる（Go の default: は使わない）。
// 型の switch は、いつも最後に default: の節（分類の誤りの枝）を持つ。
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
		if len(a.Cases) == 0 {
			return len(c.Cases)
		}
		return order[a.Cases[0]]
	}
	sort.SliceStable(arms, func(i, j int) bool { return rank(arms[i]) < rank(arms[j]) })

	qualifier := b.f.im.Qualifier(e.declNS[c.Name])
	name := e.typeName(c.Name)
	switch p.form {
	case "type":
		b.line("switch " + p.subject + ".(type) {")
	case "value":
		k := b.kind()
		b.line(k + ", err := " + qualifier + "Classify" + name + "(ctx, o." + e.fields.classify[c.Name] + ", " + p.subject + ")")
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
		if p.form == "type" {
			var types []string
			seen := map[string]bool{}
			add := func(typ string) {
				if !seen[typ] {
					seen[typ] = true
					types = append(types, typ)
				}
			}
			u := e.unionOf(p.subjectType)
			for _, k := range a.Cases {
				cs := c.Case(k)
				if u == nil || cs == nil || len(cs.As) == 0 {
					continue
				}
				if cs.IsElse() {
					// 残りのケース: 残りのアームの Go の型を、union の宣言の順に並べる
					for _, arm := range e.remainingArms(u, c) {
						add(e.armGoType(b.f, u, arm))
					}
					continue
				}
				if arm, ok := e.variantArm(u, cs.As[0].Type, c.Name); ok {
					add(e.armGoType(b.f, u, arm))
				}
			}
			b.line("case " + strings.Join(types, ", ") + ":")
		} else {
			var consts []string
			for _, k := range a.Cases {
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
	if p.form == "type" {
		// 型の switch の最後の節は、いつも、分類の誤りの枝: どのケースも as に書かなかった型のアームの値と、union の値が
		// ない（nil）とき
		b.line("default:")
		b.depth++
		b.buf.WriteString(b.errorBranch(c.Name))
		b.depth--
		b.line("}")
		b.terminated = mode == modeReturn || allEnd
		return
	}
	b.line("}")
	// 述語で決まる分類の switch は default: の節を持たない。どの節も関数を抜けるなら、後ろに
	// `return <零値>, &runtime.ClassificationError{…}` を置く（判定関数が先に同じ誤りを返すので、ここには来ない。Go が、
	// 関数の終わりの return を求めるため）
	if mode == modeReturn || allEnd {
		b.returnErr(b.classificationError(c.Name))
	}
}
