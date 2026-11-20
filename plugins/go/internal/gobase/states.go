package gobase

import (
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
func effectMethodComment(x *ir.Effect) string {
	return unitMethodComment(golang.MethodName(x.Name)+" は命令（効果 "+x.Name+"、effects "+effectTarget(x)+"）", x.Description)
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
	}
	return f
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
