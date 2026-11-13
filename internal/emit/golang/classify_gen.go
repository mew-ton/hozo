package golang

import (
	"fmt"

	gol "github.com/mew-ton/hozo/plugin/golang"
)

// genClassify は classify.go（docs/emit-go.md「classify」）: 分類の列挙型、人が実装する述語の
// インターフェース <C>Cases、判定関数 Classify<C>。Classifiers は ops.go。
func (e *emitter) genClassify() *genFile {
	if len(e.classifies) == 0 {
		return nil
	}
	g := e.newFile("classify.go")
	rt := g.imports.Add(runtimePath, "runtime")
	for _, c := range e.classifies {
		name := e.typeName(c.Name)
		// 値の分類だけがここに来る。状態の分類は checkUnsupported が GOEMIT-INTERNAL で止めている。
		subject := gol.GoType(c.SubjectType(), &g.imports)
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
		if c.HasDefault {
			g.println("// 真が二つ以上なら *runtime.ClassificationError、一つもなければ default。")
		} else {
			g.println("// 真がちょうど一つでなければ *runtime.ClassificationError。")
		}
		g.printf("func Classify%s(c %sCases, v %s) (%s, error) {\n", name, name, subject, name)
		g.println("\tvar hits []string")
		if c.HasDefault {
			g.printf("\tresult := %s\n", gol.CaseConst(name, "default"))
		} else {
			g.printf("\tvar result %s\n", name)
		}
		for _, cs := range c.Cases {
			g.printf("\tif c.%s(v) {\n\t\thits = append(hits, %q)\n\t\tresult = %s\n\t}\n", gol.Capitalize(cs.Name), cs.Name, gol.CaseConst(name, cs.Name))
		}
		cond := "len(hits) != 1"
		if c.HasDefault {
			cond = "len(hits) > 1"
		}
		g.printf("\tif %s {\n\t\treturn 0, &%s.ClassificationError{Classify: %q, Hits: hits}\n\t}\n", cond, rt, c.Name)
		g.println("\treturn result, nil\n}\n")
	}
	return g
}
