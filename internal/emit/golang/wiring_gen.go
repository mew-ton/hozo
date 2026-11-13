package golang

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// genWiring は wiring.go（docs/emit-go.md「配線の本体」「match」）。配線の op ごとに本体を一つ。
// 包む関数がなければ本体がそのまま公開メソッド、あれば非公開の <op>Body（ops.go の公開メソッドが包む）。
func (e *emitter) genWiring() *genFile {
	g := e.newFile("wiring.go")
	for _, op := range e.ops {
		if op.Body == nil {
			continue
		}
		b := &bodyGen{e: e, g: g, op: op, imp: &g.imports, vars: map[string]ir.Type{}, names: map[string]string{}, declared: map[string]bool{}}
		b.gen()
	}
	return g
}

// bodyGen は配線の op 一つの本体を作る。
type bodyGen struct {
	e   *emitter
	g   *genFile
	imp *gol.Imports
	op  *ir.Op
	buf strings.Builder

	vars     map[string]ir.Type // Go の識別子 → その式の型（Go から見た型。optional の model のフィールドは中身の型）
	names    map[string]string  // Hozo の名前（引数・束縛） → Go の識別子
	declared map[string]bool    // 本体で宣言した Go の識別子
	depth    int                // 0 なら関数の直下。1 以上は switch の case の中
	ind      string
	returned bool // 直前の文が return だったか
}

func (b *bodyGen) gen() {
	e := b.e
	op := b.op
	for _, p := range op.Params {
		n := local(p.Name)
		b.names[p.Name] = n
		b.vars[n] = p.Type
		b.declared[n] = true
	}
	b.declared["ctx"] = true
	b.ind = "\t"
	stmts := op.Body.Statements
	for i, s := range stmts {
		b.returned = false
		b.stmt(s, stmts[i+1:], i == len(stmts)-1)
	}
	if !b.returned {
		if op.Returns == nil {
			b.line("return nil")
		} else {
			b.line("return " + e.zeroExpr(op.Returns, b.imp) + ", nil")
		}
	}
	g := b.g
	if _, wrapped := e.wrappers[op.Name]; wrapped {
		g.printf("// %s は %s の配線の本体。包む関数を通さずに呼ぶ（ops.go の %s が包む）。\n", e.bodyMethod(op), op.Name, e.methodName(op))
		g.printf("func (o *ops) %s(%s) %s {\n", e.bodyMethod(op), e.paramList(op.Params, b.imp), e.resultList(op.Returns, b.imp))
	} else {
		g.body.WriteString(e.opComment(e.methodName(op), op, op.Params, op.Signature, ""))
		g.printf("func (o *ops) %s(%s) %s {\n", e.methodName(op), e.paramList(op.Params, b.imp), e.resultList(op.Returns, b.imp))
	}
	g.body.WriteString(b.buf.String())
	g.println("}\n")
}

// line は字下げ付きで一行書く。
func (b *bodyGen) line(s string) {
	b.buf.WriteString(b.ind + s + "\n")
}

// fresh は base を元に、まだ宣言していない Go の識別子を作る。
func (b *bodyGen) fresh(base string) string {
	n := local(base)
	for i := 2; b.declared[n]; i++ {
		n = local(base) + strconv.Itoa(i)
	}
	b.declared[n] = true
	return n
}

// failReturn は `return <零値>, err`（void なら `return err`）。
func (b *bodyGen) failReturn() string {
	if b.op.Returns == nil {
		return "return err"
	}
	return "return " + b.e.zeroExpr(b.op.Returns, b.imp) + ", err"
}

// assignErr は `<name>, err := expr` と err の確認。switch の中（depth > 0）では `var name T` で宣言してから `=`。
// name が "" なら err だけ。
func (b *bodyGen) assignErr(name string, t ir.Type, expr string) {
	if name == "" {
		if b.depth == 0 {
			b.line("if err := " + expr + "; err != nil {")
			b.line("\t" + b.failReturn())
			b.line("}")
		} else {
			b.line("err = " + expr)
		}
		return
	}
	if b.depth == 0 {
		b.line(name + ", err := " + expr)
		b.line("if err != nil {")
		b.line("\t" + b.failReturn())
		b.line("}")
		return
	}
	b.line("var " + name + " " + gol.GoType(t, b.imp))
	b.line(name + ", err = " + expr)
	// case の中では err を見ない。switch を抜けた後でまとめて見る（match）。
}

// stmt は文一つ。rest は後ろの文（束縛が使われるかの判定）。last は本体の最後の文か。
func (b *bodyGen) stmt(s ir.Statement, rest []ir.Statement, last bool) {
	e := b.e
	switch x := s.(type) {
	case ir.ConstStmt:
		switch ex := x.Expr.(type) {
		case ir.CallExpr:
			expr, ret := b.call(&ex.Call)
			n := b.fresh(x.Name)
			b.names[x.Name] = n
			b.vars[n] = ret
			b.assignErr(n, ret, expr)
			b.markUnused(n, x.Name, rest)
		case ir.MatchExpr:
			b.match(&ex.Match, x.Name, true, rest)
		case ir.AtomExpr:
			expr, t := b.atom(ex.Atom, nil)
			n := b.fresh(x.Name)
			b.names[x.Name] = n
			b.vars[n] = t
			b.line(n + " := " + expr)
			b.markUnused(n, x.Name, rest)
		}
	case ir.ReturnStmt:
		switch ex := x.Expr.(type) {
		case ir.CallExpr:
			expr, ret := b.call(&ex.Call)
			if ir.TypeEqual(stripCase(ret), stripCase(b.op.Returns)) || b.op.Returns == nil {
				b.line("return " + expr)
			} else {
				n := b.fresh("result")
				b.assignErr(n, ret, expr)
				b.line("return " + e.convert(n, ret, b.op.Returns, b.imp) + ", nil")
			}
		case ir.MatchExpr:
			n := b.match(&ex.Match, "", true, nil)
			if n == "" {
				b.line("return nil")
			} else {
				b.line("return " + e.convert(n, b.vars[n], b.op.Returns, b.imp) + ", nil")
			}
		case ir.AtomExpr:
			expr, _ := b.atom(ex.Atom, b.op.Returns)
			if b.op.Returns == nil {
				b.line("return nil")
			} else {
				b.line("return " + expr + ", nil")
			}
		}
		b.returned = true
	case ir.CallStmt:
		expr, ret := b.call(&x.Call)
		if last && b.op.Returns == nil && ret == nil {
			b.line("return " + expr)
			b.returned = true
			return
		}
		if ret != nil {
			b.line("if _, err := " + expr + "; err != nil {")
			b.line("\t" + b.failReturn())
			b.line("}")
			return
		}
		b.assignErr("", nil, expr)
	case ir.MatchStmt:
		b.match(&x.Match, "", false, rest)
	}
}

// markUnused は束縛が後ろで使われなければ `_ = x`（Go の「declared and not used」を避ける。WIRE-8 は warning）。
func (b *bodyGen) markUnused(goName, hozoName string, rest []ir.Statement) {
	if !usesName(rest, hozoName) {
		b.line("_ = " + goName)
	}
}

// match は match の文か式（docs/emit-go.md「match」）。wantValue なら値を変数に束縛して
// その Go の名前を返す（bindName が空なら result）。値のない match か文なら "" を返す。
func (b *bodyGen) match(m *ir.Match, bindName string, wantValue bool, rest []ir.Statement) string {
	e := b.e
	c, _ := e.lookup(m.Classify).(*ir.Classify)
	if c == nil {
		return ""
	}
	cName := e.typeName(c.Name)
	// 値の分類での match だけがここに来る。状態での match は checkUnsupported が GOEMIT-INTERNAL で止めている。
	subj, _ := b.atom(m.SubjectAtom(), c.SubjectType())
	kind := b.fresh("kind")
	b.assignErr(kind, nil, fmt.Sprintf("Classify%s(o.classifiers.%s, %s)", cName, cName, subj))

	// match の型（docs/types.md「式の型」）。
	var valueType ir.Type
	hasNone, hasCall := false, false
	for _, a := range m.Arms {
		switch body := a.Body.(type) {
		case ir.CallExpr:
			hasCall = true
			if op := e.op(body.Call.Op); op != nil && op.Returns != nil && valueType == nil {
				valueType = op.Returns
			}
		case ir.NoneBody:
			hasNone = true
		}
	}
	matchType := valueType
	if valueType != nil && hasNone {
		matchType = ir.Optional(valueType)
	}
	result := ""
	if matchType != nil && wantValue {
		base := bindName
		if base == "" {
			base = "result"
		}
		result = b.fresh(base)
		if bindName != "" {
			b.names[bindName] = result
		}
		b.vars[result] = matchType
		b.line("var " + result + " " + gol.GoType(matchType, b.imp))
	}

	b.line("switch " + kind + " {")
	for _, a := range m.Arms {
		if a.Cases.Default {
			b.line("case " + gol.CaseConst(cName, "default") + ":")
		} else {
			cs := make([]string, len(a.Cases.Cases))
			for i, n := range a.Cases.Cases {
				cs[i] = gol.CaseConst(cName, n)
			}
			b.line("case " + strings.Join(cs, ", ") + ":")
		}
		b.depth++
		b.ind += "\t"
		switch body := a.Body.(type) {
		case ir.CallExpr:
			expr, ret := b.call(&body.Call)
			switch {
			case result == "" && ret == nil:
				b.line("err = " + expr)
			case result == "":
				b.line("_, err = " + expr)
			case ir.TypeEqual(stripCase(ret), stripCase(matchType)):
				b.line(result + ", err = " + expr)
			default:
				v := b.fresh("v")
				b.line("var " + v + " " + gol.GoType(ret, b.imp))
				b.line(v + ", err = " + expr)
				b.line(result + " = " + e.convert(v, ret, matchType, b.imp))
			}
		case ir.NoneBody:
			if result != "" {
				b.line(result + " = nil")
			}
		}
		b.depth--
		b.ind = b.ind[:len(b.ind)-1]
	}
	b.line("}")
	if hasCall {
		b.line("if err != nil {")
		b.line("\t" + b.failReturn())
		b.line("}")
	}
	if bindName != "" && result != "" {
		b.markUnused(result, bindName, rest)
	}
	return result
}

// call は呼び出しの式を作る。bind で埋まる位置の取得など、前に要る文はここで書く。
// 戻り値は呼び出しの Go の式と、呼び出し先の returns（void なら nil）。
func (b *bodyGen) call(c *ir.Call) (string, ir.Type) {
	e := b.e
	op := e.op(c.Op)
	if op == nil {
		return "nil", nil
	}
	sameNS := e.declNS[c.Op] == e.viewer
	var params []ir.OpParam
	var receiver string
	if sameNS {
		params = op.Params
		receiver = "o"
	} else {
		params = e.viewedParams(c.Op)
		ns := e.declNS[c.Op]
		receiver = "o.deps." + depsField(ns)
		e.depsOps[ns] = append(e.depsOps[ns], c.Op)
	}

	// 同じ名前空間への呼び出しの bind: 状態の値を取る（docs/emit-go.md「配線の本体」の Call.bound）。
	type boundVal struct {
		pos  ir.BoundPosition
		name string
		typ  ir.Type
	}
	var bounds []boundVal
	if sameNS {
		for _, pos := range c.Bound {
			st, _ := e.lookup(pos.State).(*ir.State)
			if st == nil || len(pos.Path) == 0 {
				continue
			}
			e.usedStates = append(e.usedStates, st.Name)
			n := b.fresh(localName(st.Name))
			b.assignErr(n, stateValueType(st), "o.states."+e.typeName(st.Name)+"(ctx)")
			bounds = append(bounds, boundVal{pos, n, stateValueType(st)})
		}
	}

	args := []string{"ctx"}
	for _, p := range params {
		var arg *ir.CallArg
		for i := range c.Args {
			if c.Args[i].Param == p.Name {
				arg = &c.Args[i]
				break
			}
		}
		var mine []boundVal
		whole := -1
		for _, bv := range bounds {
			if bv.pos.Path[0] == p.Name {
				mine = append(mine, bv)
				if len(bv.pos.Path) == 1 {
					whole = len(mine) - 1
				}
			}
		}
		switch {
		case whole >= 0:
			args = append(args, e.convert(mine[whole].name, mine[whole].typ, p.Type, b.imp))
		case len(mine) == 0:
			if arg == nil {
				args = append(args, e.zeroExpr(p.Type, b.imp))
			} else {
				expr, _ := b.atom(arg.Value, p.Type)
				args = append(args, expr)
			}
		default:
			// 引数の値を変数に置いてから、埋まる位置に代入する。
			v := b.fresh(p.Name)
			var init string
			if arg != nil {
				init, _ = b.atom(arg.Value, p.Type)
			} else {
				init = b.emptyValue(p.Type)
			}
			b.line(v + " := " + init)
			for _, bv := range mine {
				b.assignBound(v, p.Type, bv.pos.Path[1:], bv.name, bv.typ)
			}
			args = append(args, v)
		}
	}
	return receiver + "." + e.methodName(op) + "(" + strings.Join(args, ", ") + ")", op.Returns
}

// emptyValue は model の型の空の値（`T{}`、optional なら `&T{}`）。
func (b *bodyGen) emptyValue(t ir.Type) string {
	inner, opt := stripOptional(t)
	s := gol.GoType(inner, b.imp) + "{}"
	if opt {
		return "&" + s
	}
	return s
}

// assignBound は v（型 t の値）のフィールドのパス path に状態の値 state（型 st）を入れる。
func (b *bodyGen) assignBound(v string, t ir.Type, path []string, state string, st ir.Type) {
	e := b.e
	expr := v
	cur := t
	var ft ir.Type
	for _, seg := range path {
		m := e.modelOf(cur)
		if m == nil {
			return
		}
		var ok bool
		ft, ok = e.fieldType(m.Name, seg)
		if !ok {
			return
		}
		expr += "." + gol.FieldName(seg)
		cur = ft
	}
	if ft == nil {
		return
	}
	_, stOpt := stripOptional(st)
	_, fOpt := stripOptional(ft)
	if stOpt && !fOpt && !e.isNillable(ft) {
		// single<T?> の値を既定値のあるフィールドに。なければ既定値のまま（BIND-2）。
		b.line("if " + state + " != nil {")
		inner, _ := stripOptional(st)
		b.line("\t" + expr + " = " + e.convert("*"+state, inner, ft, b.imp))
		b.line("}")
		return
	}
	b.line(expr + " = " + e.convert(state, st, ft, b.imp))
}

// atom は Atom の Go の式と、その式の型。target はその値を置く位置の型（nil なら不明）。
// 位置の型に合わせた変換（docs/types.md「互換」）を含む。
func (b *bodyGen) atom(a ir.Atom, target ir.Type) (string, ir.Type) {
	e := b.e
	switch x := a.(type) {
	case ir.NameAtom:
		if len(x.Path) == 0 {
			return "nil", nil
		}
		goName, ok := b.names[x.Path[0]]
		if !ok {
			goName = local(x.Path[0])
		}
		expr := goName
		t := b.vars[goName]
		for _, seg := range x.Path[1:] {
			expr += "." + gol.FieldName(seg)
			m := e.modelOf(t)
			if m == nil {
				t = nil
				continue
			}
			ft, ok := e.fieldType(m.Name, seg)
			if !ok {
				t = nil
				continue
			}
			t = ft
		}
		return e.convert(expr, t, target, b.imp), t
	case ir.LiteralAtom:
		lt := literalType(x.Literal)
		if target == nil {
			return plainLiteral(x.Literal), lt
		}
		return e.defaultExpr(x.Literal, target, b.imp), lt
	case ir.NoneAtom:
		return "nil", nil
	case ir.RecordAtom:
		m := e.modelOf(target)
		if m == nil {
			return "nil", nil
		}
		inner, opt := stripOptional(target)
		var fields []string
		for _, f := range x.Fields {
			ft, _ := e.fieldType(m.Name, f.Name)
			v, _ := b.atom(f.Value, ft)
			fields = append(fields, gol.FieldName(f.Name)+": "+v)
		}
		lit := gol.GoType(inner, b.imp) + "{" + strings.Join(fields, ", ") + "}"
		if opt {
			return "&" + lit, target
		}
		return lit, inner
	}
	return "nil", nil
}

// usesName は文の列のどこかで Hozo の名前 name（束縛名か引数名）が参照されているか。
func usesName(stmts []ir.Statement, name string) bool {
	for _, s := range stmts {
		switch x := s.(type) {
		case ir.ConstStmt:
			if usesInExpr(x.Expr, name) {
				return true
			}
		case ir.ReturnStmt:
			if usesInExpr(x.Expr, name) {
				return true
			}
		case ir.CallStmt:
			if usesInCall(&x.Call, name) {
				return true
			}
		case ir.MatchStmt:
			if usesInMatch(&x.Match, name) {
				return true
			}
		}
	}
	return false
}

func usesInExpr(x ir.Expr, name string) bool {
	switch y := x.(type) {
	case ir.CallExpr:
		return usesInCall(&y.Call, name)
	case ir.MatchExpr:
		return usesInMatch(&y.Match, name)
	case ir.AtomExpr:
		return usesInAtom(y.Atom, name)
	}
	return false
}

func usesInCall(c *ir.Call, name string) bool {
	for _, a := range c.Args {
		if usesInAtom(a.Value, name) {
			return true
		}
	}
	return false
}

func usesInMatch(m *ir.Match, name string) bool {
	if a := m.SubjectAtom(); a != nil && usesInAtom(a, name) {
		return true
	}
	for _, a := range m.Arms {
		if c, ok := a.Body.(ir.CallExpr); ok && usesInCall(&c.Call, name) {
			return true
		}
	}
	return false
}

func usesInAtom(a ir.Atom, name string) bool {
	switch x := a.(type) {
	case ir.NameAtom:
		return len(x.Path) > 0 && x.Path[0] == name
	case ir.RecordAtom:
		for _, f := range x.Fields {
			if usesInAtom(f.Value, name) {
				return true
			}
		}
	}
	return false
}
