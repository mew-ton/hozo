package golang

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// wrapper は op に付いたデコレータの適用のうち、単位のトランスパイラが select の wraps で引き受けたもの（docs/emit-go.md「デコレータ」）。
type wrapper struct {
	typ   string // 関数の型の名前 <Op><デコレータ名>Wrapper
	group string // <デコレータ名>。Wrappers のフィールド名で、束ねる構造体は <デコレータ名>Wrappers
	field string // 束ねる構造体のフィールド名（op 名を先頭大文字に）
	name  string // デコレータの FQN（via があれば via）
	app   ir.Application
	decl  *ir.Decorator // 汎用デコレータの宣言。見つからなければ nil
}

// 生成したコードが返す *runtime.MissingError の Kind（docs/emit-go.md「実装の単位」）。
const (
	missingLeaf       = "leaf"
	missingState      = "state"
	missingClassifier = "classifier"
	missingWrapper    = "wrapper"
	missingDep        = "dep"
	missingConstraint = "constraint"
)

// missing は `&runtime.MissingError{Kind: ..., Name: ...}` の式。
func missing(imp *gol.Imports, kind, name string) string {
	return fmt.Sprintf("&%s.MissingError{Kind: %q, Name: %q}", imp.Add(runtimePath, "runtime"), kind, name)
}

// collectWrappers は op ごとの包む関数を、内側（Op.Decorators の末尾）から順に集める。
// 包むのは select の wraps が指した適用だけ。どのトランスパイラも引き受けないデコレータは印で、何も生成しない。
func (e *emitter) collectWrappers() {
	for _, op := range e.ops {
		claimed := e.sel.wraps[op.Name]
		if claimed == nil {
			continue
		}
		var ws []wrapper
		for i := len(op.Decorators) - 1; i >= 0; i-- {
			app := op.Decorators[i]
			if !claimed[app.Decorator] && !(app.Via != "" && claimed[app.Via]) {
				continue
			}
			name := app.Decorator
			if app.Via != "" {
				name = app.Via
			}
			decl, _ := e.lookup(app.Decorator).(*ir.Decorator)
			group := e.decoratorName(name)
			ws = append(ws, wrapper{
				typ:   e.methodName(op) + group + "Wrapper",
				group: group,
				field: e.methodName(op),
				name:  name,
				app:   app,
				decl:  decl,
			})
		}
		if len(ws) > 0 {
			e.wrappers[op.Name] = ws
		}
	}
}

// decoratorName はデコレータの名前を、viewer から見た型名の規則で大文字にしたもの（docs/emit-go.md「デコレータ」）:
// viewer のデコレータは宣言名（`Retry`）、別の名前空間のものは名前空間も付く（`HttpRetry`）。
func (e *emitter) decoratorName(fqn string) string {
	if e.declNS[fqn] == e.viewer {
		return gol.TypeName(fqn, e.viewer)
	}
	return gol.TypeName(fqn, "")
}

// wrapperGroups は包む関数をデコレータごとにまとめたもの。並びは、op の宣言の順に、書かれた順で最初に現れた順。
// 一つの組の中は op の宣言の順。
func (e *emitter) wrapperGroups() (names []string, byGroup map[string][]wrapper) {
	byGroup = map[string][]wrapper{}
	for _, op := range e.ops {
		ws := e.wrappers[op.Name]
		for i := len(ws) - 1; i >= 0; i-- { // 書かれた順（外側から）
			w := ws[i]
			if _, ok := byGroup[w.group]; !ok {
				names = append(names, w.group)
			}
			byGroup[w.group] = append(byGroup[w.group], w)
		}
	}
	return names, byGroup
}

// methodName は op の Go のメソッド名。
func (e *emitter) methodName(op *ir.Op) string { return gol.MethodName(localName(op.Name)) }

// funcTypeName は op の関数の型の名前 <Op>Func（docs/emit-go.md「op」の「関数の型」）。
func (e *emitter) funcTypeName(op *ir.Op) string { return e.methodName(op) + "Func" }

// depFuncTypeName は別の名前空間の op の、viewer から見た関数の型の名前 <Ns><Op>Func。
func (e *emitter) depFuncTypeName(op *ir.Op) string {
	return depsField(e.declNS[op.Name]) + e.methodName(op) + "Func"
}

// paramList は `<name> <type>, ...`。生成するコードは context を引数に取らない（docs/emit-go.md「実装の単位」）。
func (e *emitter) paramList(params []ir.OpParam, imp *gol.Imports) string {
	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, local(p.Name)+" "+gol.GoType(p.Type, imp))
	}
	return strings.Join(parts, ", ")
}

// argList は `<name>, ...`（引数をそのまま渡すとき）。
func argList(params []ir.OpParam) string {
	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, local(p.Name))
	}
	return strings.Join(parts, ", ")
}

// resultList は `(<type>, error)` か `error`。
func (e *emitter) resultList(ret ir.Type, imp *gol.Imports) string {
	if ret == nil {
		return "error"
	}
	return "(" + gol.GoType(ret, imp) + ", error)"
}

// funcType は op の関数の型 `func(...) (...)`（docs/emit-go.md「op」の「関数の型」）。
func (e *emitter) funcType(params []ir.OpParam, ret ir.Type, imp *gol.Imports) string {
	return "func(" + e.paramList(params, imp) + ") " + e.resultList(ret, imp)
}

// failWith は `return <零値>, <errExpr>`（void なら `return <errExpr>`）。
func (e *emitter) failWith(ret ir.Type, errExpr string, imp *gol.Imports) string {
	if ret == nil {
		return "return " + errExpr
	}
	return "return " + e.zeroExpr(ret, imp) + ", " + errExpr
}

// opComment は op のコメント: 先頭の文（head）、説明、@param、Throws、Effects、絞り込まれた型の前提。
// head が空なら「<method> は <説明>」で始める（Ops のメソッド）。
func (e *emitter) opComment(method, head string, op *ir.Op, params []ir.OpParam, sig ir.Signature, note string) string {
	var b strings.Builder
	if head == "" {
		b.WriteString(declComment(method, op.Description, "op "+op.Name+"。"))
	} else {
		b.WriteString(comment(head))
		if op.Description != nil && strings.TrimSpace(op.Description.Body) != "" {
			b.WriteString("//\n" + comment(strings.TrimSpace(op.Description.Body)))
		}
	}
	if note != "" {
		b.WriteString("//\n// " + note + "\n")
	}
	if op.Description != nil && len(op.Description.Params) > 0 {
		b.WriteString("//\n// 引数:\n")
		for _, p := range op.Description.Params {
			b.WriteString(fmt.Sprintf("//   - %s: %s\n", p.Name, p.Text))
		}
	}
	var extra []string
	if len(sig.Exceptions) > 0 {
		extra = append(extra, "Throws: "+fqnList(sig.Exceptions))
	}
	if len(sig.Reads) > 0 {
		// 読む状態（docs/emit-go.md「移行の間の扱い」）。空なら書かない
		extra = append(extra, "Reads: "+fqnList(sig.Reads))
	}
	if len(sig.Effects) > 0 {
		extra = append(extra, "Effects: "+fqnList(sig.Effects))
	}
	for _, p := range params {
		if hasCase(p.Type) {
			extra = append(extra, fmt.Sprintf("前提: %s は %s", p.Name, p.Type.String()))
		}
	}
	if hasCase(op.Returns) {
		extra = append(extra, "前提: 戻り値は "+op.Returns.String())
	}
	if len(extra) > 0 {
		b.WriteString("//\n")
		for _, l := range extra {
			b.WriteString("// " + l + "\n")
		}
	}
	return b.String()
}

// genOps は ops.go（docs/emit-go.md「op」）: op の関数の型、Leaves、Ops、Deps、Wrappers、Classifiers、New、ops と、
// 口だけを出す op（葉、stops / wraps の op）のメソッド。配線の本体のメソッドは wiring.go。
func (e *emitter) genOps() *genFile {
	if len(e.ops) == 0 {
		return nil
	}
	g := e.newFile("ops.go")
	imp := &g.imports

	// 関数の型。葉でも配線でも同じ。
	for _, op := range e.ops {
		name := e.funcTypeName(op)
		g.body.WriteString(e.opComment(name, fmt.Sprintf("%s は op %s の関数の型。", name, op.Name), op, op.Params, e.viewedSig(op.Name), ""))
		g.printf("type %s %s\n\n", name, e.funcType(op.Params, op.Returns, imp))
	}

	if len(e.leaves) > 0 {
		g.println("// Leaves は効果を持たない葉の op の実装。人か、その op の語彙を読むトランスパイラがフィールドを埋める。")
		g.println("// 埋めていない op が呼ばれたら *runtime.MissingError（Kind は \"leaf\"）。")
		g.println("type Leaves struct {")
		for _, op := range e.leaves {
			g.printf("\t%s %s\n", e.methodName(op), e.funcTypeName(op))
		}
		g.println("}\n")
	}

	g.println("// Ops は呼ぶ側が使う、この名前空間の op（葉と配線）。New が返す。")
	g.println("type Ops interface {")
	for i, op := range e.ops {
		if i > 0 {
			g.println("")
		}
		g.body.WriteString(indent(e.opComment(e.methodName(op), "", op, op.Params, e.viewedSig(op.Name), ""), "\t"))
		g.printf("\t%s(%s) %s\n", e.methodName(op), e.paramList(op.Params, imp), e.resultList(op.Returns, imp))
	}
	g.println("}\n")

	depsNS := e.orderedDepsNS()
	if len(depsNS) > 0 {
		g.println("// Deps は配線から呼ぶ別の名前空間の op。名前空間ごとに一つの構造体で、フィールドは viewer から見た関数の型")
		g.println("// （bind で埋まる位置は引数から消えている。埋める者が状態の値を入れて呼び出し先に渡す）。")
		g.println("// 埋めていない op が呼ばれたら *runtime.MissingError（Kind は \"dep\"）。")
		g.println("type Deps struct {")
		for _, ns := range depsNS {
			g.printf("\t%s %s\n", depsField(ns), depsField(ns)+"Ops")
		}
		g.println("}\n")
		for _, ns := range depsNS {
			g.printf("// %sOps は名前空間 %s の op のうち、この名前空間の配線が呼ぶもの。\n", depsField(ns), ns)
			g.printf("type %sOps struct {\n", depsField(ns))
			for _, fqn := range e.orderedDepsOps(ns) {
				op := e.op(fqn)
				g.printf("\t%s %s\n", e.methodName(op), e.depFuncTypeName(op))
			}
			g.println("}\n")
			for _, fqn := range e.orderedDepsOps(ns) {
				op := e.op(fqn)
				sig := e.view.Signature(fqn)
				var bound []string
				if sig != nil {
					for _, b := range sig.Bound {
						bound = append(bound, strings.Join(b.Path, ".")+" ← "+b.State)
					}
				}
				note := ""
				if len(bound) > 0 {
					note = "bind で埋まる位置（フィールドを埋める者が埋める）: " + strings.Join(bound, ", ")
				}
				name := e.depFuncTypeName(op)
				head := fmt.Sprintf("%s は op %s の、%s から見た関数の型。", name, fqn, e.viewer)
				g.body.WriteString(e.opComment(name, head, op, e.viewedParams(fqn), e.viewedSig(fqn), note))
				g.printf("type %s %s\n\n", name, e.funcType(e.viewedParams(fqn), op.Returns, imp))
			}
		}
	}

	groups, byGroup := e.wrapperGroups()
	if len(groups) > 0 {
		for _, op := range e.ops {
			for _, w := range e.wrappers[op.Name] {
				g.body.WriteString(e.wrapperComment(op, w))
				params := e.paramList(op.Params, imp)
				if params != "" {
					params += ", "
				}
				g.printf("type %s func(%s%s %s) %s\n\n", w.typ, params, nextName(op.Params), e.funcTypeName(op), e.resultList(op.Returns, imp))
			}
		}
		for _, name := range groups {
			g.printf("// %sWrappers は @%s の包む関数。そのデコレータを引き受けたトランスパイラがフィールドを埋める。\n", name, byGroup[name][0].name)
			g.printf("type %sWrappers struct {\n", name)
			for _, w := range byGroup[name] {
				g.printf("\t%s %s\n", w.field, w.typ)
			}
			g.println("}\n")
		}
		g.println("// Wrappers は、トランスパイラが select の wraps で引き受けたデコレータの適用の「包む関数」。デコレータごとに一つの構造体。")
		g.println("// 埋めていない包む関数に当たったら *runtime.MissingError（Kind は \"wrapper\"）。")
		g.println("type Wrappers struct {")
		for _, name := range groups {
			g.printf("\t%s %sWrappers\n", name, name)
		}
		g.println("}\n")
	}

	values := e.valueClassifies()
	if len(values) > 0 {
		g.println("// Classifiers は値の分類ごとの述語の実装（classify.go の <C>Cases）。")
		g.println("// 埋めていない分類で判定しようとしたら *runtime.MissingError（Kind は \"classifier\"）。")
		g.println("type Classifiers struct {")
		for _, c := range values {
			g.printf("\t%s %sCases\n", e.typeName(c.Name), e.typeName(c.Name))
		}
		g.println("}\n")
	}

	// New と ops。引数は、この名前空間に要るものだけ。
	type part struct{ name, typ string }
	var parts []part
	if len(e.leaves) > 0 {
		parts = append(parts, part{"leaves", "Leaves"})
	}
	if len(depsNS) > 0 {
		parts = append(parts, part{"deps", "Deps"})
	}
	if len(groups) > 0 {
		parts = append(parts, part{"wrappers", "Wrappers"})
	}
	if len(values) > 0 {
		parts = append(parts, part{"classifiers", "Classifiers"})
	}
	var sigParts, fields []string
	for _, p := range parts {
		sigParts = append(sigParts, p.name+" "+p.typ)
		fields = append(fields, p.name+": "+p.name)
	}
	g.println("// New はこの名前空間の Ops を組み立てる。効果を持たない葉は leaves に、効果を持つ葉は States（states.go）の命令に、")
	g.println("// 配線は生成した本体に委ね、包む関数で内側から順に包む。")
	g.printf("func New(%s) Ops {\n", strings.Join(sigParts, ", "))
	g.printf("\treturn &ops{%s}\n}\n\n", strings.Join(fields, ", "))
	g.println("// ops は New が返す非公開の構造体。フィールドは New の引数そのまま。")
	if len(parts) == 0 {
		g.println("type ops struct{}\n")
	} else {
		g.println("type ops struct {")
		for _, p := range parts {
			g.printf("\t%s %s\n", p.name, p.typ)
		}
		g.println("}\n")
	}

	for _, op := range e.ops {
		if !e.isSlot(op) {
			continue // 本体は wiring.go
		}
		e.genSlotMethod(g, op)
	}
	return g
}

// genSlotMethod は、口だけを出す op のメソッド: Leaves か状態の単位のフィールドを呼び、包む関数があれば内側から順に包む。
// フィールドが nil なら *runtime.MissingError（docs/emit-go.md「実装の単位」）。
func (e *emitter) genSlotMethod(g *genFile, op *ir.Op) {
	imp := &g.imports
	field := "o.leaves." + e.methodName(op)
	kind := missingLeaf
	if u := e.commandOf[op.Name]; u != nil {
		field = "States." + e.unitField(u) + "." + e.methodName(op)
		kind = missingState
	}
	params := e.paramList(op.Params, imp)
	results := e.resultList(op.Returns, imp)
	// inner は「nil なら MissingError、そうでなければ呼ぶ」の本体（ind で字下げ）。
	inner := func(ind, fieldExpr, kind, name, args string) string {
		return fmt.Sprintf("%sif %s == nil {\n%s\t%s\n%s}\n%sreturn %s(%s)\n", ind, fieldExpr, ind, e.failWith(op.Returns, missing(imp, kind, name), imp), ind, ind, fieldExpr, args)
	}
	g.printf("func (o *ops) %s(%s) %s {\n", e.methodName(op), params, results)
	ws := e.wrappers[op.Name]
	if len(ws) == 0 {
		g.body.WriteString(inner("\t", field, kind, op.Name, argList(op.Params)))
		g.println("}\n")
		return
	}
	ft := e.funcTypeName(op)
	fn := func(i int) string { return freshIdent(fmt.Sprintf("f%d", i), op.Params) }
	g.printf("\t%s := %s(func(%s) %s {\n", fn(0), ft, params, results)
	g.body.WriteString(inner("\t\t", field, kind, op.Name, argList(op.Params)))
	g.println("\t})")
	for i, w := range ws {
		args := argList(op.Params)
		if args != "" {
			args += ", "
		}
		g.printf("\t%s := %s(func(%s) %s {\n", fn(i+1), ft, params, results)
		g.body.WriteString(inner("\t\t", "o.wrappers."+w.group+"."+w.field, missingWrapper, op.Name+"@"+w.name, args+fn(i)))
		g.println("\t})")
	}
	g.printf("\treturn %s(%s)\n}\n\n", fn(len(ws)), argList(op.Params))
}

// nextName は包む関数の最後の引数（内側の関数）の名前。op の引数と重なれば末尾に `_`。
func nextName(params []ir.OpParam) string { return freshIdent("next", params) }

// freshIdent は base を、op の引数の名前と重ならない識別子にする（重なる間、末尾に `_` を足す）。
func freshIdent(base string, params []ir.OpParam) string {
	for {
		clash := false
		for _, p := range params {
			if local(p.Name) == base {
				clash = true
				break
			}
		}
		if !clash {
			return base
		}
		base += "_"
	}
}

// valueClassifies は選ばれた値の分類（Classifiers に入るもの）を宣言の順で。
func (e *emitter) valueClassifies() []*ir.Classify {
	var out []*ir.Classify
	for _, c := range e.classifies {
		if c.SubjectType() != nil {
			out = append(out, c)
		}
	}
	return out
}

// depsField は Deps のフィールド名: 名前空間の最後のセグメントを大文字に。
func depsField(ns string) string { return gol.Capitalize(localName(ns)) }

// orderedDepsNS は配線から呼ぶ別の名前空間を Program の順で。
func (e *emitter) orderedDepsNS() []string {
	var out []string
	for i := range e.prog.Namespaces {
		n := e.prog.Namespaces[i].Name
		if len(e.orderedDepsOps(n)) > 0 {
			out = append(out, n)
		}
	}
	return out
}

// orderedDepsOps は名前空間 ns の op のうち配線が呼ぶものを宣言の順で。
func (e *emitter) orderedDepsOps(ns string) []string {
	var out []string
	n := e.prog.Namespace(ns)
	if n == nil || ns == e.viewer {
		return nil
	}
	for _, d := range n.Decls {
		if op, ok := d.(*ir.Op); ok && e.sel.deps[op.Name] {
			out = append(out, op.Name)
		}
	}
	return out
}

// orderedDeps は配線が呼ぶ別の名前空間の op を、Program の順、宣言の順で。
func (e *emitter) orderedDeps() []string {
	var out []string
	for _, ns := range e.orderedDepsNS() {
		out = append(out, e.orderedDepsOps(ns)...)
	}
	return out
}

// stateValueType は状態の値の型（docs/emit-go.md「state」の Get）: nullable<T> なら optional(T)
// （Go では *T。空なら nil）、single<T> なら T。
func stateValueType(st *ir.State) ir.Type {
	if st.Shape == ir.StateNullable {
		return ir.Optional(st.Element)
	}
	return st.Element
}

// wrapperComment は包む関数のコメント: どの op のどのデコレータか、throws / catches の宣言。
func (e *emitter) wrapperComment(op *ir.Op, w wrapper) string {
	var b strings.Builder
	via := w.app.Decorator
	if w.app.Via != "" {
		via = w.app.Via
	}
	fmt.Fprintf(&b, "// %s は %s に付いた @%s", w.typ, op.Name, via)
	if w.app.Via != "" || len(w.app.Args) > 0 {
		fmt.Fprintf(&b, "（= %s%s）", w.app.Decorator, renderArgs(w.app.Args))
	}
	b.WriteString("。\n")
	if w.decl != nil && w.decl.Description != nil {
		b.WriteString(comment(strings.TrimSpace(w.decl.Description.Body)))
	}
	subst := func(s string) string {
		if strings.HasPrefix(s, "$param:") {
			for _, a := range w.app.Args {
				if a.Param == s[len("$param:"):] {
					return renderArgValue(a.Value)
				}
			}
		}
		return s
	}
	if w.decl != nil {
		for _, t := range w.decl.Throws {
			ex := make([]string, len(t.Exceptions))
			for i, x := range t.Exceptions {
				ex[i] = subst(x)
			}
			line := "// throws " + strings.Join(ex, " | ")
			for _, r := range t.Relations {
				line += " " + string(r.Kind) + " effects"
			}
			b.WriteString(line + "\n")
		}
		for _, c := range w.decl.Catches {
			ex := make([]string, len(c.Exceptions))
			for i, x := range c.Exceptions {
				ex[i] = subst(x)
			}
			line := "// catches " + strings.Join(ex, " | ") + " " + string(c.Mode)
			if c.Mode == ir.Halts {
				line += " " + subst(c.Halts)
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// renderArgs はデコレータの引数を `(a: x, b: y)` の形に。空なら ""。
func renderArgs(args []ir.Arg) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.Param + ": " + renderArgValue(a.Value)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// renderArgValue はデコレータの引数の値を Hozo の表面の形で。
func renderArgValue(v ir.ArgValue) string {
	switch x := v.(type) {
	case ir.LiteralValue:
		if d, ok := x.Literal.(ir.DurationLiteral); ok {
			return d.Value.String()
		}
		if s, ok := x.Literal.(ir.StringLiteral); ok {
			return strconv.Quote(s.Value)
		}
		return plainLiteral(x.Literal)
	case ir.RefValue:
		return x.Target
	case ir.RefsValue:
		return strings.Join(x.Targets, " | ")
	case ir.VariantValue:
		return x.Union + "." + x.Arm
	case ir.ListValue:
		parts := make([]string, len(x.Items))
		for i, it := range x.Items {
			parts[i] = renderArgValue(it)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return "?"
}
