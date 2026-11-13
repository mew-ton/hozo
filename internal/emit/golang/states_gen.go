package golang

import (
	"fmt"
	"strings"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// 状態の単位の、形で決まる問い合わせのフィールド名（docs/emit-go.md「state」の表）。
const (
	queryIsEmpty = "IsEmpty"
	queryGet     = "Get"
)

// unitField は StateSet のフィールド名: 状態名（状態の分類ならその名前）を先頭大文字にしたもの。
func (e *emitter) unitField(u *stateUnit) string { return gol.FieldName(localName(u.fqn)) }

// unitType は状態の単位の型の名前 <状態名を先頭大文字に>State。
func (e *emitter) unitType(u *stateUnit) string { return e.unitField(u) + "State" }

// unitQueries は単位の問い合わせのフィールド名を、単位の中の順で返す: list は IsEmpty、nullable は IsEmpty と Get、
// single は Get、状態の分類はケースごとの述語（default は持たない）。
func (e *emitter) unitQueries(u *stateUnit) []string {
	if u.classify != nil {
		out := make([]string, len(u.classify.Cases))
		for i, cs := range u.classify.Cases {
			out[i] = gol.Capitalize(cs.Name)
		}
		return out
	}
	switch u.state.Shape {
	case ir.StateList:
		return []string{queryIsEmpty}
	case ir.StateNullable:
		return []string{queryIsEmpty, queryGet}
	}
	return []string{queryGet}
}

// effectTargetText は命令のコメントに書く効果の宛先: `on carts.post` か `on Checkout: Shopping -> Ordered`。
func effectTargetText(eff *ir.Effect) string {
	switch t := eff.Target.(type) {
	case ir.BuiltinTarget:
		return fmt.Sprintf("on %s.%s", t.State, t.Operation)
	case ir.TransitionTarget:
		return fmt.Sprintf("on %s: %s -> %s", t.Of, caseSetText(t.From), caseSetText(t.To))
	}
	return ""
}

// genStates は states.go（docs/emit-go.md「state」）: 状態の単位、StateSet、パッケージの変数 States、
// 状態の分類の判定関数 Classify<C>()。選ばれた状態が一つもなければ nil。
func (e *emitter) genStates() *genFile {
	if len(e.units) == 0 {
		return nil
	}
	g := e.newFile("states.go")
	imp := &g.imports
	for _, u := range e.units {
		name := e.unitType(u)
		if u.classify != nil {
			states, _ := u.classify.SubjectStates()
			g.printf("// %s は分類の状態 %s（%s を対象にした classify）の単位。\n", name, u.fqn, fqnList(states))
			if u.classify.Description != nil {
				g.body.WriteString(comment(strings.TrimSpace(u.classify.Description.Body)))
			}
		} else {
			g.printf("// %s は状態 %s（%s<%s>）の単位。\n", name, u.fqn, u.state.Shape, u.state.Element.String())
			if u.state.Description != nil {
				g.body.WriteString(comment(strings.TrimSpace(u.state.Description.Body)))
			}
		}
		g.printf("type %s struct {\n", name)
		if u.classify != nil {
			for _, cs := range u.classify.Cases {
				g.printf("\t// 問い合わせ: ケース %s の述語。\n", cs.Name)
				if cs.Description != nil {
					g.body.WriteString(indent(comment(strings.TrimSpace(cs.Description.Body)), "\t"))
				}
				g.printf("\t%s func() (bool, error)\n", gol.Capitalize(cs.Name))
			}
		} else {
			if u.state.Shape.HasBuiltinClassify() {
				g.println("\t// 問い合わせ: 組み込みの分類。空（Empty）なら true。HasItems はその否定。")
				g.printf("\t%s func() (bool, error)\n", queryIsEmpty)
			}
			switch u.state.Shape {
			case ir.StateNullable:
				g.println("\t// 問い合わせ: 状態の値。空なら nil。")
				g.printf("\t%s func() (%s, error)\n", queryGet, gol.GoType(stateValueType(u.state), imp))
			case ir.StateSingle:
				g.println("\t// 問い合わせ: 状態の値。")
				g.printf("\t%s func() (%s, error)\n", queryGet, gol.GoType(stateValueType(u.state), imp))
			}
		}
		for _, op := range u.commands {
			eff := e.effectOf(op)
			g.printf("\t// 命令: 効果 %s（%s）を持つ葉の op %s。\n", eff.Name, effectTargetText(eff), op.Name)
			g.printf("\t%s %s\n", e.methodName(op), e.funcTypeName(op))
		}
		g.println("}\n")
	}

	g.println("// StateSet はこの名前空間の状態の単位を束ねる。")
	g.println("type StateSet struct {")
	for _, u := range e.units {
		g.printf("\t%s %s\n", e.unitField(u), e.unitType(u))
	}
	g.println("}\n")
	g.println("// States はこの名前空間の状態。保存先の実装（問い合わせ）と、命令の実装をここに入れる。")
	g.println("// 入れていないフィールドが要るところへ来たら *runtime.MissingError（Kind は \"state\"）。")
	g.println("var States StateSet\n")

	for _, u := range e.units {
		if u.classify != nil {
			e.genStateClassify(g, u)
		}
	}
	return g
}

// genStateClassify は状態の分類の判定関数 `Classify<C>() (<C>, error)`（docs/emit-go.md「state」）。
// States.<C> の述語をすべて呼び、真の数を値の分類と同じ規則で検証する。述語がエラーを返したら（入れていない述語は
// *runtime.MissingError）、残りも呼んだうえで最初のエラーを返す。
func (e *emitter) genStateClassify(g *genFile, u *stateUnit) {
	c := u.classify
	name := e.typeName(c.Name)
	states, _ := c.SubjectStates()
	g.printf("// Classify%s は状態 %s の場合を %s で分類する。States.%s の述語をすべて呼び、真になった数を数える。\n", name, fqnList(states), c.Name, e.unitField(u))
	g.println("// 述語がエラーを返したら、（残りも呼んだうえで）最初のエラーを返す。")
	g.println(hitsDoc(c))
	g.printf("func Classify%s() (%s, error) {\n", name, name)
	resultDecl(g, c, name)
	g.println("\tvar firstErr error")
	for _, cs := range c.Cases {
		g.printf("\tif p := States.%s.%s; p == nil {\n\t\tif firstErr == nil {\n\t\t\tfirstErr = %s\n\t\t}\n\t} else if ok, err := p(); err != nil {\n\t\tif firstErr == nil {\n\t\t\tfirstErr = err\n\t\t}\n\t} else if ok {\n\t\thits = append(hits, %q)\n\t\tresult = %s\n\t}\n",
			e.unitField(u), gol.Capitalize(cs.Name), missing(&g.imports, missingState, c.Name), cs.Name, gol.CaseConst(name, cs.Name))
	}
	g.println("\tif firstErr != nil {\n\t\treturn 0, firstErr\n\t}")
	e.hitsCheck(g, c)
}
