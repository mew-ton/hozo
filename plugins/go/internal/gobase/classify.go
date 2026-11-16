package gobase

import (
	"strconv"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは classify.go（docs/plugin-go.md「分類」）: 述語で決まる分類の、列挙の型と定数、述語のインターフェース、判定関数
// （宣言順）。型で決まる分類は、何も出さない。定義のパッケージのもの（genClassify）と、分けた出力の入口の再公開（genEntryClassify）。
// 述語も判定関数も、op と同じに、最初の引数に ctx を取り、最後の戻り値に error を返す（同「op の形」）。

// predicateClassifies は、N の述語で決まる分類（宣言順）。
func (e *emitter) predicateClassifies() []*ir.Classify {
	var out []*ir.Classify
	for _, d := range e.ns.Decls {
		if c, ok := d.(*ir.Classify); ok && c.DecidedBy == ir.DecidedByPredicate {
			out = append(out, c)
		}
	}
	return out
}

func (e *emitter) classifyComment(c *ir.Classify) string {
	return comment(describe(e.typeName(c.Name), c.Description, "classify "+c.Name+"。"))
}

func casesComment(name, fqn string) string {
	return comment(name + "Cases は分類 " + fqn + " のケースごとの述語。人が実装し、この分類を match に使う名前空間の New に渡す。")
}

func classifyFuncComment(name, fqn string, state bool) string {
	if state {
		return comment("Classify" + name + " は、状態の分類 " + fqn + " の今のケースを返す。すべての述語を呼び、真になった数を数える。")
	}
	return comment("Classify" + name + " は v を " + fqn + " で分類する。すべての述語を呼び、真になった数を数える。")
}

// caseConsts は、分類 c の列挙の定数の名前とコメント（ケースの宣言順。hasDefault なら、最後に <C>Default）。
func (e *emitter) caseConsts(c *ir.Classify) (names, comments []string) {
	name := e.typeName(c.Name)
	for _, cs := range c.Cases {
		names = append(names, golang.CaseConst(name, cs.Name))
		comments = append(comments, comment(body(cs.Description)))
	}
	if c.HasDefault {
		names = append(names, golang.CaseConst(name, "default"))
		comments = append(comments, comment(body(c.DefaultDescription)))
	}
	return names, comments
}

// variantArm は、union u のアームのうち、型が t と等しいもの。二つ以上あれば GO-6（どのアームかを、型で分けられない）。
// at は分類の FQN（診断の位置）。
func (e *emitter) variantArm(u *ir.Union, t ir.Type, at string) (armType, bool) {
	var found []armType
	for _, a := range e.armTypes(u) {
		if ir.TypeEqual(a.typ, t) {
			found = append(found, a)
		}
	}
	switch len(found) {
	case 1:
		return found[0], true
	case 0:
		e.errorf(codeInternal, at, "分類 %s の variant の絞り込みの型 %s と等しい型のアームが、union %s にない（検査を通った IR には来ないはずの形）", at, golang.TypeDisplay(t), u.Name)
	default:
		e.errorf(codeVariant, at, "分類 %s の絞り込みの型 %s と等しい型のアームが、union %s に二つ以上ある（%s と %s）。どのアームかを、型で分けられない",
			at, golang.TypeDisplay(t), u.Name, found[0].name, found[1].name)
	}
	return armType{}, false
}

// remainingArms は、分類 c（union u の variant で決まる分類）の残りのアーム（docs/check.md「残りのケース」）: 型が、ほかの
// ケースが as に書いた型のどれとも等しくないアーム。union の宣言の順。同じ Go の型（同じ model を指す二つのアーム）は一度だけ。
func (e *emitter) remainingArms(u *ir.Union, c *ir.Classify) []armType {
	var out []armType
	seen := map[string]bool{}
	for _, a := range e.armTypes(u) {
		named := false
		for _, cs := range c.Cases {
			for _, as := range cs.As {
				if as.By != ir.NarrowByElse && ir.TypeEqual(as.Type, a.typ) {
					named = true
				}
			}
		}
		if named || seen[a.goName] {
			continue
		}
		seen[a.goName] = true
		out = append(out, a)
	}
	return out
}

// unionOf は、型 t（case と optional を外したもの）が union（arms）への ref なら、その union。
func (e *emitter) unionOf(t ir.Type) *ir.Union {
	for {
		switch x := t.(type) {
		case ir.CaseType:
			t = x.Of
			continue
		case ir.OptionalType:
			t = x.Of
			continue
		case ir.RefType:
			u, _ := e.lookup(x.Target).(*ir.Union)
			return u
		}
		return nil
	}
}

// armGoType は、アーム a（union u のもの）の Go の型を、ファイル f から書いた名前（別の名前空間の union なら修飾する）。
func (e *emitter) armGoType(f *fileGen, u *ir.Union, a armType) string {
	return f.im.Qualifier(e.declNS[u.Name]) + a.goName
}

// genClassify は定義のパッケージの classify.go。述語で決まる分類がなければ nil。
func (e *emitter) genClassify() *fileGen {
	cs := e.predicateClassifies()
	if len(cs) == 0 {
		return nil
	}
	f := e.newFile(fileClassify, true)
	rt := func() string { return f.im.Add(runtimePath, "runtime") }
	for _, c := range cs {
		name := e.typeName(c.Name)
		// 列挙の型と定数
		f.print(e.classifyComment(c))
		f.printf("type %s int\n\nconst (\n", name)
		names, comments := e.caseConsts(c)
		for i, n := range names {
			f.print(indent(comments[i]))
			if i == 0 {
				f.printf("\t%s %s = iota\n", n, name)
			} else {
				f.printf("\t%s\n", n)
			}
		}
		f.print(")\n\n")
		ctx := f.im.Add("context", "context")
		_, isState := c.SubjectStates()
		if !isState {
			// 述語のインターフェース。引数は、ctx と、分類の対象の型の値
			subject := c.SubjectType()
			f.print(casesComment(name, c.Name))
			f.printf("type %sCases interface {\n", name)
			for _, cs := range c.Cases {
				f.printf("\t// %s はケース %s の述語。\n\t%s(ctx %s.Context, v %s) (bool, error)\n", golang.Capitalize(cs.Name), cs.Name, golang.Capitalize(cs.Name), ctx, f.goType(subject))
			}
			f.print("}\n\n")
			// 判定関数: すべての述語を宣言順に呼び、真の数を数える
			f.print(classifyFuncComment(name, c.Name, false))
			f.printf("func Classify%s(ctx %s.Context, c %sCases, v %s) (%s, error) {\n\tvar hits []string\n\tvar kind %s\n\tvar first error\n",
				name, ctx, name, f.goType(subject), name, name)
			for _, cs := range c.Cases {
				call := classifyCase("c."+golang.Capitalize(cs.Name)+"(ctx, v)", cs.Name, golang.CaseConst(name, cs.Name))
				// 型で決まる絞り込みの as を持つケースは、値がその型に当たらなければ、述語を呼ばずに偽とする
				guard := ""
				if len(cs.As) > 0 {
					switch cs.As[0].By {
					case ir.NarrowByPresence:
						guard = "v != nil"
					case ir.NarrowByVariant:
						if u := e.unionOf(subject); u != nil {
							if a, ok := e.variantArm(u, cs.As[0].Type, c.Name); ok {
								guard = "_, is := v.(" + e.armGoType(f, u, a) + "); is"
							}
						}
					}
				}
				if guard != "" {
					call = "if " + guard + " {\n" + call + "}\n"
				}
				f.print(call)
			}
			e.classifyTail(f, c, name, rt())
			continue
		}
		// 状態の分類: 述語は状態の単位 <C>State のメソッド
		f.print(classifyFuncComment(name, c.Name, true))
		f.printf("func Classify%s(ctx %s.Context, s %s) (%s, error) {\n\tvar hits []string\n\tvar kind %s\n\tvar first error\n",
			name, ctx, golang.StateUnitName(c.Name), name, name)
		for _, cs := range c.Cases {
			f.print(classifyCase("s."+golang.Capitalize(cs.Name)+"(ctx)", cs.Name, golang.CaseConst(name, cs.Name)))
		}
		e.classifyTail(f, c, name, rt())
	}
	return f
}

// classifyCase は、判定関数の中の、ケース一つの述語の呼び出し（call は `c.<ケース>(ctx, v)` か `s.<ケース>(ctx)`）: 真なら、
// ケース名を hits に足す。述語が error を返したら、最初のものを first に覚えて、残りの述語も呼ぶ。
func classifyCase(call, caseName, caseConst string) string {
	return "if ok, err := " + call + "; err != nil {\nif first == nil {\nfirst = err\n}\n} else if ok {\n" +
		"hits = append(hits, " + strconv.Quote(caseName) + ")\nkind = " + caseConst + "\n}\n"
}

// classifyTail は、判定関数の終わり: 述語が error を返していれば、最初のもの。そうでなければ、真が一つならそのケース、0 で
// hasDefault なら <C>Default。hasDefault が false なら、真がちょうど一つでなければ、true なら、真が二つ以上なら、
// *runtime.ClassificationError。
func (e *emitter) classifyTail(f *fileGen, c *ir.Classify, name, rt string) {
	f.print("\tif first != nil {\n\t\treturn kind, first\n\t}\n")
	f.print("\tswitch len(hits) {\n\tcase 1:\n\t\treturn kind, nil\n")
	if c.HasDefault {
		f.printf("\tcase 0:\n\t\treturn %s, nil\n", golang.CaseConst(name, "default"))
	}
	f.printf("\t}\n\treturn kind, &%s.ClassificationError{Classify: %s, Hits: hits}\n}\n\n", rt, strconv.Quote(c.Name))
}

// genEntryClassify は、分けた出力の入口の classify.go: 列挙の型の別名、定数、述語のインターフェースの別名、内側の判定関数を
// 呼んで返す関数。なければ nil。
func (e *emitter) genEntryClassify() *fileGen {
	cs := e.predicateClassifies()
	if len(cs) == 0 {
		return nil
	}
	f := e.newFile(fileClassify, false)
	inner := f.im.Add(e.layout.DefPath, e.layout.DefName)
	for _, c := range cs {
		name := e.typeName(c.Name)
		f.print(e.classifyComment(c))
		f.printf("type %s = %s.%s\n\nconst (\n", name, inner, name)
		names, comments := e.caseConsts(c)
		for i, n := range names {
			f.print(indent(comments[i]))
			f.printf("\t%s = %s.%s\n", n, inner, n)
		}
		f.print(")\n\n")
		if _, isState := c.SubjectStates(); isState {
			f.print(classifyFuncComment(name, c.Name, true))
			f.printf("func Classify%s(ctx %s.Context, s %s) (%s, error) {\n\treturn %s.Classify%s(ctx, s)\n}\n\n",
				name, f.im.Add("context", "context"), golang.StateUnitName(c.Name), name, inner, name)
			continue
		}
		f.print(casesComment(name, c.Name))
		f.printf("type %sCases = %s.%sCases\n\n", name, inner, name)
		f.print(classifyFuncComment(name, c.Name, false))
		f.printf("func Classify%s(ctx %s.Context, c %sCases, v %s) (%s, error) {\n\treturn %s.Classify%s(ctx, c, v)\n}\n\n",
			name, f.im.Add("context", "context"), name, f.goType(c.SubjectType()), name, inner, name)
	}
	return f
}
