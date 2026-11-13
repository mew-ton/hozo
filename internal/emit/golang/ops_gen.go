package golang

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// wrapper は op に付いたデコレータのうち「包む関数」にするもの（docs/emit-go.md「デコレータ」）。
type wrapper struct {
	method string // <Op><デコレータ名>
	app    ir.Application
	decl   *ir.Decorator // 汎用デコレータの宣言。見つからなければ nil
}

// collectWrappers は op ごとの包む関数を、内側（Op.Decorators の末尾）から順に集める。
// 包むのは、宣言が external でなく、Conf(N).plugins のマニフェストの entry / stop にないもの。
// decorate にあるものも包む（実装はプラグインが出す）。
func (e *emitter) collectWrappers() {
	for _, op := range e.ops {
		var ws []wrapper
		for i := len(op.Decorators) - 1; i >= 0; i-- {
			app := op.Decorators[i]
			decl, _ := e.lookup(app.Decorator).(*ir.Decorator)
			if decl != nil && decl.External {
				continue
			}
			if e.claimed(app.Decorator) || (app.Via != "" && e.claimed(app.Via)) {
				continue
			}
			name := app.Decorator
			if app.Via != "" {
				name = app.Via
			}
			ws = append(ws, wrapper{
				method: e.methodName(op) + gol.Capitalize(localName(name)),
				app:    app,
				decl:   decl,
			})
		}
		if len(ws) > 0 {
			e.wrappers[op.Name] = ws
		}
	}
}

// claimed はデコレータの FQN がどれかのマニフェストの entry / stop にあるか。
func (e *emitter) claimed(fqn string) bool {
	for _, m := range e.opts.Manifests {
		for _, list := range [][]string{m.Entry, m.Stop} {
			for _, d := range list {
				if d == fqn {
					return true
				}
			}
		}
	}
	return false
}

// methodName は op の Go のメソッド名。
func (e *emitter) methodName(op *ir.Op) string { return gol.MethodName(localName(op.Name)) }

// paramList は `ctx context.Context, <name> <type>, ...`。
func (e *emitter) paramList(params []ir.OpParam, imp *gol.Imports) string {
	parts := []string{"ctx " + imp.Add("context", "context") + ".Context"}
	for _, p := range params {
		parts = append(parts, local(p.Name)+" "+gol.GoType(p.Type, imp))
	}
	return strings.Join(parts, ", ")
}

// argList は `ctx, <name>, ...`（引数をそのまま渡すとき）。
func argList(params []ir.OpParam) string {
	parts := []string{"ctx"}
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

// funcType は op の関数の型 `func(ctx context.Context, ...) (...)`（docs/emit-go.md「op」の「関数の型」）。
func (e *emitter) funcType(params []ir.OpParam, ret ir.Type, imp *gol.Imports) string {
	return "func(" + e.paramList(params, imp) + ") " + e.resultList(ret, imp)
}

// opComment は op のメソッドのコメント: 説明、@param、Throws、Effects、絞り込まれた型の前提。
func (e *emitter) opComment(method string, op *ir.Op, params []ir.OpParam, sig ir.Signature, note string) string {
	var b strings.Builder
	b.WriteString(declComment(method, op.Description, "op "+op.Name+"。"))
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

// genOps は ops.go（docs/emit-go.md「op」の「インターフェース」「デコレータ」と「state」）。
func (e *emitter) genOps() *genFile {
	if len(e.ops) == 0 && len(e.classifies) == 0 && len(e.stateCases) == 0 {
		return nil
	}
	g := e.newFile("ops.go")
	imp := &g.imports

	var leaves []*ir.Op
	for _, op := range e.ops {
		if op.Body == nil {
			leaves = append(leaves, op)
		}
	}
	if len(leaves) > 0 {
		g.println("// Leaves は人が実装する葉の op。")
		g.println("type Leaves interface {")
		for i, op := range leaves {
			if i > 0 {
				g.println("")
			}
			g.body.WriteString(indent(e.opComment(e.methodName(op), op, op.Params, op.Signature, ""), "\t"))
			g.printf("\t%s(%s) %s\n", e.methodName(op), e.paramList(op.Params, imp), e.resultList(op.Returns, imp))
		}
		g.println("}\n")
	}
	if len(e.ops) > 0 {
		g.println("// Ops は呼び出す側が使う、この名前空間のすべての op。New が返す。")
		g.println("type Ops interface {")
		for i, op := range e.ops {
			if i > 0 {
				g.println("")
			}
			g.body.WriteString(indent(e.opComment(e.methodName(op), op, op.Params, op.Signature, ""), "\t"))
			g.printf("\t%s(%s) %s\n", e.methodName(op), e.paramList(op.Params, imp), e.resultList(op.Returns, imp))
		}
		g.println("}\n")
	}

	depsNS := e.orderedDepsNS()
	if len(depsNS) > 0 {
		g.println("// Deps は配線から呼ぶ別の名前空間の op。実装者は bind で埋まる位置に状態の値を入れて呼び出し先に渡す")
		g.println("// （docs/emit-go.md「op」の「インターフェース」）。")
		g.println("type Deps struct {")
		for _, ns := range depsNS {
			g.printf("\t%s %s\n", depsField(ns), depsField(ns)+"Ops")
		}
		g.println("}\n")
		for _, ns := range depsNS {
			g.printf("// %sOps は名前空間 %s の op のうち、この名前空間の配線が呼ぶもの。引数は bind で埋まる位置を除いた形。\n", depsField(ns), ns)
			g.printf("type %sOps interface {\n", depsField(ns))
			for i, fqn := range e.orderedDepsOps(ns) {
				if i > 0 {
					g.println("")
				}
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
					note = "bind で埋まる位置（Deps の実装者が埋める）: " + strings.Join(bound, ", ")
				}
				g.body.WriteString(indent(e.opComment(e.methodName(op), op, e.viewedParams(fqn), e.viewedSig(fqn), note), "\t"))
				g.printf("\t%s(%s) %s\n", e.methodName(op), e.paramList(e.viewedParams(fqn), imp), e.resultList(op.Returns, imp))
			}
			g.println("}\n")
		}
	}

	states := e.orderedStates()
	if len(states) > 0 {
		g.println("// States は、この名前空間の配線が同じ名前空間の op を呼ぶときに bind で埋める状態の値の取得")
		g.println("// （docs/emit-go.md「state」）。nullable<T> なら *T（空なら nil）、single<T> なら T。")
		g.println("type States interface {")
		for _, fqn := range states {
			st := e.lookup(fqn).(*ir.State)
			g.printf("\t// %s は状態 %s の値。\n", e.typeName(fqn), fqn)
			g.printf("\t%s(ctx %s.Context) (%s, error)\n", e.typeName(fqn), imp.Add("context", "context"), gol.GoType(stateValueType(st), imp))
		}
		g.println("}\n")
	}

	if len(e.stateCases) > 0 {
		g.println("// StateCases は基底の状態の組み込みの分類（Empty / HasItems）の述語。人が保存先に問い合わせて実装する")
		g.println("// （docs/emit-go.md「state」）。HasItems は Empty の否定なので、述語は状態ごとに一つ。")
		g.println("// 配線の match s by list / by nullable と、効果の結果を確かめるテストが呼ぶ。")
		g.println("type StateCases interface {")
		for _, st := range e.stateCases {
			g.printf("\t// %s は状態 %s（%s）が空（Empty）か。\n", e.isEmptyMethod(st), st.Name, st.Shape)
			g.printf("\t%s(ctx %s.Context) (bool, error)\n", e.isEmptyMethod(st), imp.Add("context", "context"))
		}
		g.println("}\n")
	}

	if e.hasWrappers() {
		g.println("// Wrappers は op に付いたデコレータのうち、プラグインが扱わないものの「包む関数」（docs/emit-go.md「デコレータ」）。")
		g.println("// 引数は op の引数と next（op の関数の型）。")
		g.println("type Wrappers interface {")
		n := 0
		for _, op := range e.ops {
			for _, w := range e.wrappers[op.Name] {
				if n > 0 {
					g.println("")
				}
				n++
				g.body.WriteString(indent(e.wrapperComment(op, w), "\t"))
				g.printf("\t%s(%s, next %s) %s\n", w.method, e.paramList(op.Params, imp), e.funcType(op.Params, op.Returns, imp), e.resultList(op.Returns, imp))
			}
		}
		g.println("}\n")
	}

	if len(e.classifies) > 0 {
		g.println("// Classifiers は分類ごとの述語の実装（classify.go の <C>Cases）。")
		g.println("type Classifiers struct {")
		for _, c := range e.classifies {
			g.printf("\t%s %sCases\n", e.typeName(c.Name), e.typeName(c.Name))
		}
		g.println("}\n")
	}

	if len(e.ops) == 0 {
		return g
	}

	// New と ops。
	type part struct{ name, typ string }
	var parts []part
	if len(leaves) > 0 {
		parts = append(parts, part{"leaves", "Leaves"})
	}
	if len(depsNS) > 0 {
		parts = append(parts, part{"deps", "Deps"})
	}
	if len(states) > 0 {
		parts = append(parts, part{"states", "States"})
	}
	if e.matchesState {
		parts = append(parts, part{"stateCases", "StateCases"})
	}
	if e.hasWrappers() {
		parts = append(parts, part{"wrappers", "Wrappers"})
	}
	if len(e.classifies) > 0 {
		parts = append(parts, part{"classifiers", "Classifiers"})
	}
	var sigParts, fields []string
	for _, p := range parts {
		sigParts = append(sigParts, p.name+" "+p.typ)
		fields = append(fields, p.name+": "+p.name)
	}
	g.println("// New はこの名前空間の Ops を組み立てる。葉は leaves に、配線は生成した本体に委ね、デコレータの包む関数で包む。")
	g.printf("func New(%s) Ops {\n", strings.Join(sigParts, ", "))
	g.printf("\treturn &ops{%s}\n}\n\n", strings.Join(fields, ", "))
	g.println("// ops は New が返す非公開の構造体。フィールドは New の引数そのまま。")
	g.println("type ops struct {")
	for _, p := range parts {
		g.printf("\t%s %s\n", p.name, p.typ)
	}
	g.println("}\n")

	for _, op := range e.ops {
		ws := e.wrappers[op.Name]
		if op.Body != nil && len(ws) == 0 {
			continue // 本体がそのまま公開メソッド（wiring.go）
		}
		inner := "o.leaves." + e.methodName(op)
		if op.Body != nil {
			inner = "o." + e.bodyMethod(op)
		}
		g.printf("func (o *ops) %s(%s) %s {\n", e.methodName(op), e.paramList(op.Params, imp), e.resultList(op.Returns, imp))
		if len(ws) == 0 {
			g.printf("\treturn %s(%s)\n}\n\n", inner, argList(op.Params))
			continue
		}
		g.printf("\tf0 := %s\n", inner)
		for i, w := range ws {
			g.printf("\tf%d := %s {\n", i+1, e.funcType(op.Params, op.Returns, imp))
			g.printf("\t\treturn o.wrappers.%s(%s, f%d)\n\t}\n", w.method, argList(op.Params), i)
		}
		g.printf("\treturn f%d(%s)\n}\n\n", len(ws), argList(op.Params))
	}
	return g
}

// bodyMethod は、包む関数がある配線の op の本体のメソッド名（非公開）。
func (e *emitter) bodyMethod(op *ir.Op) string { return local(localName(op.Name)) + "Body" }

func (e *emitter) hasWrappers() bool { return len(e.wrappers) > 0 }

// depsField は Deps のフィールド名: 名前空間の最後のセグメントを大文字に。
func depsField(ns string) string { return gol.Capitalize(localName(ns)) }

// orderedDepsNS は配線から呼ぶ別の名前空間を Program の順で。
func (e *emitter) orderedDepsNS() []string {
	var out []string
	for i := range e.prog.Namespaces {
		n := e.prog.Namespaces[i].Name
		if _, ok := e.depsOps[n]; ok {
			out = append(out, n)
		}
	}
	return out
}

// orderedDepsOps は名前空間 ns の op のうち配線が呼ぶものを宣言の順で。
func (e *emitter) orderedDepsOps(ns string) []string {
	used := map[string]bool{}
	for _, fqn := range e.depsOps[ns] {
		used[fqn] = true
	}
	var out []string
	for _, d := range e.prog.Namespace(ns).Decls {
		if op, ok := d.(*ir.Op); ok && used[op.Name] {
			out = append(out, op.Name)
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

// isEmptyMethod は StateCases の述語のメソッド名 <State>IsEmpty（docs/emit-go.md「state」）。
// 状態の名前は States の取得メソッドと同じ規則（typeName）。
func (e *emitter) isEmptyMethod(st *ir.State) string { return e.typeName(st.Name) + "IsEmpty" }

// orderedStates は States に出す状態を Program の順で。
func (e *emitter) orderedStates() []string {
	used := map[string]bool{}
	for _, fqn := range e.usedStates {
		used[fqn] = true
	}
	var out []string
	for i := range e.prog.Namespaces {
		for _, d := range e.prog.Namespaces[i].Decls {
			if st, ok := d.(*ir.State); ok && used[st.Name] {
				out = append(out, st.Name)
			}
		}
	}
	return out
}

// wrapperComment は包む関数のコメント: どの op のどのデコレータか、throws / catches の宣言。
func (e *emitter) wrapperComment(op *ir.Op, w wrapper) string {
	var b strings.Builder
	via := w.app.Decorator
	if w.app.Via != "" {
		via = w.app.Via
	}
	fmt.Fprintf(&b, "// %s は %s に付いた @%s", w.method, op.Name, via)
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
				line += " " + string(r.Kind) + " "
				if r.Effects.All {
					line += "effects"
				} else {
					line += strings.Join(r.Effects.Effects, " | ")
				}
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
