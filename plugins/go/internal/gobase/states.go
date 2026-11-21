package gobase

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは states.go（docs/plugin-go.md「状態の単位」）: 状態の単位（基底の状態と、述語で決まる状態の分類を、宣言順に）。
// 単位は、その状態に触れるもののすべて: 形で決まるメソッドと、その状態の read と、その状態を宛先にする効果。
// 定義のパッケージのもの（genStates）と、分けた出力の入口の再公開（genEntryStates）。

// unitComment は、状態の単位の型のコメント（docs/plugin-go.md「コメント」）。
func (e *emitter) unitComment(u golang.StateUnit) string {
	var first string
	switch d := e.lookup(u.FQN).(type) {
	case *ir.State:
		first = u.TypeName + " は状態 " + u.FQN + "（" + string(d.Shape) + "<" + golang.TypeDisplay(d.Element) + ">）の単位。"
	case *ir.Classify:
		states, _ := d.SubjectStates()
		first = u.TypeName + " は状態の分類 " + u.FQN + "（" + joinJa(states) + " を対象にした classify）の単位。"
	}
	return comment(first + "実装は New の引数 " + u.Param + " に渡す。\n" + commentMissing)
}

// unitMethodComment は、状態の単位の read と効果のメソッドのコメント（docs/plugin-go.md「コメント」の「状態の単位のメソッド」）:
// 「<head>: <説明>」。説明がなければ「<head>。」。説明が二行以上なら、二行目から先は、行ごとに続ける。
// 「は」の後ろの空白の規則は、ここでは使わない（説明の前は `: `）。
func unitMethodComment(head string, d *ir.Doc) string {
	if b := body(d); b != "" {
		return comment(head + ": " + b)
	}
	return comment(head + "。")
}

// readComment は、read のメソッドのコメント: 「<メソッド名> は問い合わせ（read <read の FQN>）: <説明>」。
func readComment(r *ir.Read) string {
	return unitMethodComment(golang.MethodName(r.Name)+" は問い合わせ（read "+r.Name+"）", r.Description)
}

// effectMethodComment は、効果のメソッドのコメント: 「<メソッド名> は命令（効果 <効果の FQN>、effects <宛先>）: <説明>」。
// 例外を投げる効果（Effect.throws）は、effects の前に「throws <例外の FQN を「, 」で>、」を置く（docs/plugin-go.md「コメント」の
// 「状態の単位のメソッド」）。
func effectMethodComment(x *ir.Effect) string {
	throws := ""
	if len(x.Throws) > 0 {
		throws = "throws " + strings.Join(x.Throws, ", ") + "、"
	}
	narrowed := ""
	if n := golang.NarrowedParamsDisplay(x); n != "" {
		narrowed = n + "、"
	}
	return unitMethodComment(golang.MethodName(x.Name)+" は命令（効果 "+x.Name+"、"+narrowed+throws+"effects "+effectTarget(x)+"）", x.Description)
}

// genStates は定義のパッケージの states.go。状態の単位がなければ nil。
func (e *emitter) genStates() *fileGen {
	s := e.shape
	if len(s.States) == 0 {
		return nil
	}
	f := e.newFile(fileStates, true)
	ctx := "ctx " + f.im.Add("context", "context") + ".Context"
	for _, u := range s.States {
		f.print(e.unitComment(u))
		f.printf("type %s interface {\n", u.TypeName)
		// 形で決まる問い合わせと、形で決まる命令（基底の状態）か、ケースごとの述語（状態の分類）
		if st, ok := e.lookup(u.FQN).(*ir.State); ok {
			get := f.goType(st.Type)
			switch st.Shape {
			case ir.StateList:
				f.printf("\t// IsEmpty は問い合わせ（形 list で決まる）: %s に要素が一つもなければ true。\n\tIsEmpty(%s) (bool, error)\n", u.FQN, ctx)
				f.printf("\t// Get は問い合わせ（形 list で決まる）: %s の値。すべての要素を、足した順に。\n\tGet(%s) (%s, error)\n", u.FQN, ctx, get)
			case ir.StateNullable:
				f.printf("\t// IsEmpty は問い合わせ（形 nullable で決まる）: %s に値がなければ true。\n\tIsEmpty(%s) (bool, error)\n", u.FQN, ctx)
				f.printf("\t// Get は問い合わせ（形 nullable で決まる）: %s の値。なければ nil。\n\tGet(%s) (%s, error)\n", u.FQN, ctx, get)
			default:
				f.printf("\t// Get は問い合わせ（形 single で決まる）: %s の値。\n\tGet(%s) (%s, error)\n", u.FQN, ctx, get)
			}
			// 形で決まる命令: 単位の対象の配線が直に呼んでいる操作だけ（Post、Put、Delete の順）
			for _, operation := range u.Builtins {
				name := golang.BuiltinMethod(operation)
				params, results := golang.BuiltinSignature(st, operation, f.im)
				f.printf("\t// %s は命令（形 %s で決まる）: %s の組み込みの効果 %s。\n\t%s\n", name, st.Shape, u.FQN, operation, method(name, params, results))
			}
		}
		for _, c := range u.Predicates {
			f.printf("\t// %s は問い合わせ: ケース %s の述語。\n\t%s(%s) (bool, error)\n", golang.Capitalize(c), c, golang.Capitalize(c), ctx)
		}
		// read（宣言順）、効果（宣言順）。配線が呼んでいるかによらず、N のすべて
		for _, fqn := range u.Reads {
			r := e.read(fqn)
			params, results := golang.ReadSignature(r, f.im, e.reserved)
			f.print(indent(readComment(r)))
			f.printf("\t%s\n", method(golang.MethodName(fqn), params, results))
		}
		for _, fqn := range u.Effects {
			x := e.effect(fqn)
			params, results := golang.EffectSignature(x, f.im, e.reserved)
			f.print(indent(effectMethodComment(x)))
			f.printf("\t%s\n", method(golang.MethodName(fqn), params, results))
		}
		f.print("}\n\n")
		e.genChecked(f, u)
	}
	return f
}

// checkedEffects は、単位 u の効果のうち、入口で確かめる引数を持つもの（宣言順。docs/plugin-go.md「効果の入口の確かめ」）。
func (e *emitter) checkedEffects(u golang.StateUnit) []*ir.Effect {
	var out []*ir.Effect
	for _, fqn := range u.Effects {
		if x := e.effect(fqn); x != nil && len(golang.CheckedParams(e.p, x)) > 0 {
			out = append(out, x)
		}
	}
	return out
}

// checkedClassifies は、単位 u の入口の確かめが使う分類の FQN（辞書順）: 包む型のフィールドの並びで、New が包むときの引数の並び。
func (e *emitter) checkedClassifies(u golang.StateUnit) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range e.checkedEffects(u) {
		for _, c := range golang.CheckedParams(e.p, x) {
			if !seen[c.Classify] {
				seen[c.Classify] = true
				out = append(out, c.Classify)
			}
		}
	}
	sort.Strings(out)
	return out
}

// caseNames は、分類 c のケースの名前を、Go の文字列のリテラルの並びにしたもの（列挙の定数と同じ順）。確かめの失敗に、判定関数が
// 返したケースの名前を入れるのに使う: `[...]string{<これ>}[kind]`。
func caseNames(c *ir.Classify) string {
	var names []string
	for _, cs := range c.Cases {
		names = append(names, strconv.Quote(cs.Name))
	}
	return strings.Join(names, ", ")
}

// classifyArg は、分類 c の判定関数に渡す式: 値 v（Go の型が vType）を、分類の対象の型にそろえたもの。対象の型が optional(T) で、
// v の型が T なら `&v`（docs/plugin-go.md「効果の入口の確かめ」）。at は診断の位置。
func (e *emitter) classifyArg(f *fileGen, c *ir.Classify, v, vType, at string) string {
	subject := f.goType(c.SubjectType())
	switch {
	case subject == vType:
		return v
	case subject == "*"+vType:
		return "&" + v
	}
	e.errorf(codeInternal, at, "%s の値（Go の型 %s）を、分類 %s の判定関数（対象の Go の型 %s）に渡せない（生成器の不具合）", at, vType, c.Name, subject)
	return v
}

// genChecked は、単位 u を包む型と、そのメソッド（docs/plugin-go.md「効果の入口の確かめ」）: 絞り込んだ引数を持つ効果の入口で、
// 引数が、言ったとおりのケースにあるかを確かめてから、包んだ単位を呼ぶ。確かめる効果がなければ、何も出さない。
func (e *emitter) genChecked(f *fileGen, u golang.StateUnit) {
	effects := e.checkedEffects(u)
	if len(effects) == 0 {
		return
	}
	name := golang.CheckedUnitName(u.FQN)
	what := "状態 " + u.FQN
	if _, isClassify := e.lookup(u.FQN).(*ir.Classify); isClassify {
		what = "状態の分類 " + u.FQN
	}
	f.print(comment(name + " は、" + what + " の単位を包む: 絞り込んだ引数を持つ効果の入口で、引数が、言ったとおりのケースにあるかを確かめる。\n" +
		"New が、引数 " + u.Param + " を、これで包む。ほかのメソッドは、包んだ単位を、そのまま呼ぶ。"))
	f.printf("type %s struct {\n\t%s\n", name, u.TypeName)
	for _, fqn := range e.checkedClassifies(u) {
		f.printf("\t%s %s%sCases\n", e.fields.classify[fqn], f.im.Qualifier(e.declNS[fqn]), e.typeName(fqn))
	}
	f.print("}\n\n")
	// メソッドの引数の名前は、このファイルが import するパッケージの名前も避ける（本体が、別の名前空間の判定関数を呼ぶ）
	reserved := map[string]bool{}
	for n := range e.reserved {
		reserved[n] = true
	}
	for _, n := range f.im.Names() {
		reserved[n] = true
	}
	rt := f.im.Add(runtimePath, "runtime")
	for _, x := range effects {
		checks := golang.CheckedParams(e.p, x)
		var says []string
		for _, c := range checks {
			says = append(says, c.Param+" が、"+c.Classify+" の "+strings.Join(c.Cases, " か ")+" にあること")
		}
		m := golang.MethodName(x.Name)
		f.print(comment(m + " は、効果 " + x.Name + " の入口の確かめ: " + strings.Join(says, "、") + "。違えば、*runtime.CaseError を返し、効果は、起きない。"))
		params, results := golang.EffectSignature(x, f.im, reserved)
		f.printf("func (o %s) %s {\n", name, method(m, params, results))
		args := []string{"ctx"}
		for _, p := range x.Params {
			args = append(args, golang.LocalName(p.Name, reserved))
		}
		for _, c := range checks {
			cl, _ := e.lookup(c.Classify).(*ir.Classify)
			var paramType ir.Type
			for _, p := range x.Params {
				if p.Name == c.Param {
					paramType = p.Type
				}
			}
			q := f.im.Qualifier(e.declNS[c.Classify])
			tn := e.typeName(c.Classify)
			v := e.classifyArg(f, cl, golang.LocalName(c.Param, reserved), f.goType(paramType), x.Name)
			var conds, want []string
			for _, k := range c.Cases {
				conds = append(conds, "kind != "+q+golang.CaseConst(tn, k))
				want = append(want, strconv.Quote(k))
			}
			f.printf("if kind, err := %sClassify%s(ctx, o.%s, %s); err != nil {\nreturn err\n} else if %s {\n", q, tn, e.fields.classify[c.Classify], v, strings.Join(conds, " && "))
			f.printf("return &%s.CaseError{Effect: %s, Param: %s, Classify: %s, Want: []string{%s}, Got: [...]string{%s}[kind]}\n}\n",
				rt, strconv.Quote(x.Name), strconv.Quote(c.Param), strconv.Quote(c.Classify), strings.Join(want, ", "), caseNames(cl))
		}
		f.printf("return o.%s.%s(%s)\n}\n\n", u.TypeName, m, strings.Join(args, ", "))
	}
}

// genEntryStates は、分けた出力の入口の states.go: 状態の単位の別名。なければ nil。
func (e *emitter) genEntryStates() *fileGen {
	s := e.shape
	if len(s.States) == 0 {
		return nil
	}
	f := e.newFile(fileStates, false)
	inner := f.im.Add(e.layout.DefPath, e.layout.DefName)
	for _, u := range s.States {
		f.print(e.unitComment(u))
		f.printf("type %s = %s.%s\n\n", u.TypeName, inner, u.TypeName)
	}
	return f
}
