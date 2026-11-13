package golang

import (
	"fmt"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// genClassify は classify.go（docs/emit-go.md「classify」）: 分類の列挙型、人が実装する述語の
// インターフェース <C>Cases、判定関数 Classify<C>。Classifiers は ops.go。
// 値の分類（述語は値を取る）と状態の分類（述語は値を取らず、保存先に問い合わせる）で、述語と判定関数の形が違う。
// 配線の match で使われない分類も出す。
func (e *emitter) genClassify() *genFile {
	if len(e.classifies) == 0 {
		return nil
	}
	g := e.newFile("classify.go")
	for _, c := range e.classifies {
		name := e.typeName(c.Name)
		kind := "classify"
		if c.FromCondition {
			kind = "condition"
		}
		g.body.WriteString(declComment(name, c.Description, fmt.Sprintf("%s %s のケース。", kind, c.Name)))
		g.printf("type %s int\n\n", name)
		g.println("const (")
		for i, cs := range c.Cases {
			if cs.Description != nil {
				g.body.WriteString(indent(comment(cs.Description.Body), "\t"))
			}
			if i == 0 {
				g.printf("\t%s %s = iota\n", gol.CaseConst(name, cs.Name), name)
			} else {
				g.printf("\t%s\n", gol.CaseConst(name, cs.Name))
			}
		}
		if c.HasDefault {
			if c.DefaultDescription != nil {
				g.body.WriteString(indent(comment(c.DefaultDescription.Body), "\t"))
			} else {
				g.println("\t// どのケースにも当たらないとき。")
			}
			g.printf("\t%s\n", gol.CaseConst(name, "default"))
		}
		g.println(")\n")

		switch subject := c.Subject.(type) {
		case ir.TypeSubject:
			e.genValueClassify(g, c, name, subject)
		case ir.StatesSubject:
			e.genStateClassify(g, c, name, subject)
		default:
			e.errorf(CodeInternal, &c.Span, "classify %s の対象が型でも状態でもない（docs/ir.md「classify」の subject）", c.Name)
		}
	}
	return g
}

// hitsCheck は判定関数の末尾: 真の数の検証と戻り値（docs/emit-go.md「classify」。値の分類と状態の分類で同じ）。
func (e *emitter) hitsCheck(g *genFile, c *ir.Classify) {
	rt := g.imports.Add(runtimePath, "runtime")
	cond := "len(hits) != 1"
	if c.HasDefault {
		cond = "len(hits) > 1"
	}
	g.printf("\tif %s {\n\t\treturn 0, &%s.ClassificationError{Classify: %q, Hits: hits}\n\t}\n", cond, rt, c.Name)
	g.println("\treturn result, nil\n}\n")
}

// hitsDoc は判定関数のコメントのうち、真の数の検証の行。
func hitsDoc(c *ir.Classify) string {
	if c.HasDefault {
		return "// 真が二つ以上なら *runtime.ClassificationError、一つもなければ default。"
	}
	return "// 真がちょうど一つでなければ *runtime.ClassificationError。"
}

// resultDecl は判定関数の先頭: hits と result（default があればその定数で始める）。
func resultDecl(g *genFile, c *ir.Classify, name string) {
	g.println("\tvar hits []string")
	if c.HasDefault {
		g.printf("\tresult := %s\n", gol.CaseConst(name, "default"))
	} else {
		g.printf("\tvar result %s\n", name)
	}
}

// genValueClassify は値の分類の述語と判定関数: `<Case>(v T) bool` と `Classify<C>(c, v)`。
func (e *emitter) genValueClassify(g *genFile, c *ir.Classify, name string, s ir.TypeSubject) {
	subject := gol.GoType(s.Type, &g.imports)
	g.printf("// %sCases は人が実装するケースごとの述語（設計書 §10.3）。\n", name)
	g.printf("type %sCases interface {\n", name)
	for _, cs := range c.Cases {
		if cs.Description != nil {
			g.body.WriteString(indent(comment(cs.Description.Body), "\t"))
		}
		g.printf("\t%s(v %s) bool\n", gol.Capitalize(cs.Name), subject)
	}
	g.println("}\n")

	g.printf("// Classify%s は %s を分類する。すべての述語を評価し、真になった数を数える。\n", name, c.Name)
	g.println(hitsDoc(c))
	g.printf("func Classify%s(c %sCases, v %s) (%s, error) {\n", name, name, subject, name)
	resultDecl(g, c, name)
	for _, cs := range c.Cases {
		g.printf("\tif c.%s(v) {\n\t\thits = append(hits, %q)\n\t\tresult = %s\n\t}\n", gol.Capitalize(cs.Name), cs.Name, gol.CaseConst(name, cs.Name))
	}
	e.hitsCheck(g, c)
}

// genStateClassify は状態の分類の述語と判定関数: `<Case>(ctx) (bool, error)` と `Classify<C>(ctx, c)`
// （docs/emit-go.md「classify」の「状態の分類」）。述語がエラーを返しても残りの述語は評価し、最初のエラーを返す。
func (e *emitter) genStateClassify(g *genFile, c *ir.Classify, name string, s ir.StatesSubject) {
	ctx := g.imports.Add("context", "context") + ".Context"
	states := fqnList(s.States)
	g.printf("// %sCases は人が実装するケースごとの述語（設計書 §10.3）。値を取らず、保存先に問い合わせて\n", name)
	g.printf("// 状態 %s がそのケースの場合かを答える。\n", states)
	g.printf("type %sCases interface {\n", name)
	for _, cs := range c.Cases {
		if cs.Description != nil {
			g.body.WriteString(indent(comment(cs.Description.Body), "\t"))
		}
		g.printf("\t%s(ctx %s) (bool, error)\n", gol.Capitalize(cs.Name), ctx)
	}
	g.println("}\n")

	g.printf("// Classify%s は状態 %s の場合を %s で分類する。すべての述語を評価し、真になった数を数える。\n", name, states, c.Name)
	g.println("// 述語がエラーを返したら、（残りも評価したうえで）最初のエラーを返す。")
	g.println(hitsDoc(c))
	g.printf("func Classify%s(ctx %s, c %sCases) (%s, error) {\n", name, ctx, name, name)
	resultDecl(g, c, name)
	g.println("\tvar firstErr error")
	for _, cs := range c.Cases {
		g.printf("\tif ok, err := c.%s(ctx); err != nil {\n\t\tif firstErr == nil {\n\t\t\tfirstErr = err\n\t\t}\n\t} else if ok {\n\t\thits = append(hits, %q)\n\t\tresult = %s\n\t}\n",
			gol.Capitalize(cs.Name), cs.Name, gol.CaseConst(name, cs.Name))
	}
	g.println("\tif firstErr != nil {\n\t\treturn 0, firstErr\n\t}")
	e.hitsCheck(g, c)
}
