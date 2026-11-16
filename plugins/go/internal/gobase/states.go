package gobase

import (
	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは states.go（docs/plugin-go.md「状態の単位」）: 状態の単位（基底の状態と、述語で決まる状態の分類を、宣言順に）と
// SharedState。定義のパッケージのもの（genStates）と、分けた出力の入口の再公開（genEntryStates）。

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

func (e *emitter) sharedComment() string {
	return comment("SharedState は、複数の状態にまたがる葉の op: 複数の状態を読む op と、効果の宛先が一つの状態の単位に収まらない op。実装は New の引数 " +
		e.fields.shared + " に渡す。\n" + commentMissing)
}

// commandComment は、命令（効果を持つ葉。状態の単位のものも、SharedState のものも）のメソッドのコメント:
// 「<メソッド名> は命令: 効果 <効果の句>を持つ葉の op <op の FQN>。」。効果の句は、直に持つ効果（op が書いたものと、デコレータが
// 運ぶもの。ir.Op.DirectEffects）ごとの `<効果の FQN>（on <宛先>）` を、その順に「、」でつないだもの。
func (e *emitter) commandComment(o *ir.Op) string {
	var clauses []string
	for _, fqn := range o.DirectEffects() {
		eff, ok := e.lookup(fqn).(*ir.Effect)
		if !ok {
			e.errorf(codeInternal, o.Name, "op %s の効果 %s が Program にない（検査を通った IR には来ないはずの形）", o.Name, fqn)
			continue
		}
		clauses = append(clauses, eff.Name+"（on "+effectTarget(eff)+"）")
	}
	return golang.MethodName(o.Name) + " は命令: 効果 " + joinJa(clauses) + "を持つ葉の op " + o.Name + "。"
}

// queryComment は、op の問い合わせ（効果を持たず、状態を読む葉。状態の単位のものも、SharedState のものも）のメソッドのコメント:
// 「<メソッド名> は問い合わせ: <状態の FQN を「、」で> を読む（reads）葉の op <op の FQN>。」。状態は、直に読む状態
// （op が書いたものと、デコレータが運ぶもの。ir.Op.DirectReads）。
func queryComment(o *ir.Op) string {
	return golang.MethodName(o.Name) + " は問い合わせ: " + joinJa(o.DirectReads()) + " を読む（reads）葉の op " + o.Name + "。"
}

// genStates は定義のパッケージの states.go。状態の単位も SharedState もなければ nil。
func (e *emitter) genStates() *fileGen {
	s := e.shape
	if len(s.States) == 0 && len(s.Shared) == 0 {
		return nil
	}
	f := e.newFile(fileStates, true)
	ctx := "ctx " + f.im.Add("context", "context") + ".Context"
	for _, u := range s.States {
		f.print(e.unitComment(u))
		f.printf("type %s interface {\n", u.TypeName)
		// 形で決まる問い合わせ（基底の状態）か、ケースごとの述語（状態の分類）
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
		}
		for _, c := range u.Predicates {
			f.printf("\t// %s は問い合わせ: ケース %s の述語。\n\t%s(%s) (bool, error)\n", golang.Capitalize(c), c, golang.Capitalize(c), ctx)
		}
		// op の問い合わせ（宣言順）、命令（宣言順）
		for _, fqn := range u.Queries {
			o := e.op(fqn)
			f.printf("\t// %s\n\t%s\n", queryComment(o), f.signature(o))
		}
		for _, fqn := range u.Commands {
			o := e.op(fqn)
			f.printf("\t// %s\n\t%s\n", e.commandComment(o), f.signature(o))
		}
		f.print("}\n\n")
	}
	// SharedState: 効果の宛先の単位が分かれた葉（命令）と、複数の状態を読む葉（問い合わせ）を、宣言順に
	if len(s.Shared) > 0 {
		f.print(e.sharedComment())
		f.print("type SharedState interface {\n")
		for _, fqn := range s.Shared {
			o := e.op(fqn)
			text := queryComment(o)
			if e.ops[fqn].Place == golang.SharedCommand {
				text = e.commandComment(o)
			}
			f.printf("\t// %s\n\t%s\n", text, f.signature(o))
		}
		f.print("}\n\n")
	}
	return f
}

// genEntryStates は、分けた出力の入口の states.go: 状態の単位と SharedState の別名。なければ nil。
func (e *emitter) genEntryStates() *fileGen {
	s := e.shape
	if len(s.States) == 0 && len(s.Shared) == 0 {
		return nil
	}
	f := e.newFile(fileStates, false)
	inner := f.im.Add(e.layout.DefPath, e.layout.DefName)
	for _, u := range s.States {
		f.print(e.unitComment(u))
		f.printf("type %s = %s.%s\n\n", u.TypeName, inner, u.TypeName)
	}
	if len(s.Shared) > 0 {
		f.print(e.sharedComment())
		f.printf("type SharedState = %s.SharedState\n\n", inner)
	}
	return f
}
