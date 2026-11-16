package gobase

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは、コメントのテキスト（docs/plugin-go.md「コメント」）。

// comment は text の行ごとに `// ` を付ける（空の行は `//`）。末尾に改行を一つ付ける。text が空なら ""。
func comment(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			b.WriteString("//\n")
		} else {
			b.WriteString("// " + line + "\n")
		}
	}
	return b.String()
}

// indent は、コメントのテキスト c（comment の結果）の各行の頭にタブを一つ足す。
func indent(c string) string {
	if c == "" {
		return ""
	}
	return "\t" + strings.ReplaceAll(strings.TrimRight(c, "\n"), "\n", "\n\t") + "\n"
}

// body は説明の全文（description.body）。なければ ""。
func body(d *ir.Doc) string {
	if d == nil {
		return ""
	}
	return strings.TrimSpace(d.Body)
}

// wa は「<名前> は<説明>」の形にする。説明が ASCII の文字で始まるなら、「は」の後ろに空白を一つ置く
// （`// FindUser は email からユーザーを引く`）。
func wa(name, description string) string {
	if description != "" && description[0] < 0x80 && description[0] != ' ' {
		return name + " は " + description
	}
	return name + " は" + description
}

// describe は「<名前> は<説明>」。説明がなければ「<名前> は<fallback>」（fallback は `model type.User。` のような、説明がないときの文）。
func describe(name string, d *ir.Doc, fallback string) string {
	if b := body(d); b != "" {
		return wa(name, b)
	}
	return wa(name, fallback)
}

// opFacts は、op のコメントの、説明の後ろの段（docs/plugin-go.md「コメント」の「op のコメント」）: `引数:` の段と、
// Throws / Reads / Effects と「純粋」の行。sig は、その op の計算した結果（Op.signature か、見る側から見た View.ops の値）。
func opFacts(o *ir.Op, sig *ir.Signature) []string {
	var sections []string
	if o.Description != nil && len(o.Description.Params) > 0 {
		lines := []string{"引数:"}
		for _, p := range o.Description.Params {
			lines = append(lines, "  - "+p.Name+": "+strings.ReplaceAll(strings.TrimSpace(p.Text), "\n", " "))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	var facts []string
	if len(sig.Exceptions) > 0 {
		facts = append(facts, "Throws: "+strings.Join(sig.Exceptions, ", "))
	}
	if len(sig.Reads) > 0 {
		facts = append(facts, "Reads: "+strings.Join(sig.Reads, ", "))
	}
	if len(sig.Effects) > 0 {
		effects := make([]string, len(sig.Effects))
		for i, fx := range sig.Effects {
			effects[i] = fx.String() // 宣言は FQN、直に書いた効果は <状態の FQN>.<操作>
		}
		facts = append(facts, "Effects: "+strings.Join(effects, ", "))
	}
	if o.Signature.Pure {
		if len(o.Signature.Exceptions) == 0 {
			facts = append(facts, "純粋（ctx を取らない）。失敗しない（error を返さない）")
		} else {
			facts = append(facts, "純粋（ctx を取らない）")
		}
	}
	if len(facts) > 0 {
		sections = append(sections, strings.Join(facts, "\n"))
	}
	return sections
}

// opComment は、op のコメント（Ops、Leaves のメソッド、配線の本体）: 一行目は「<メソッド名> は<説明>」。説明がなければ
// 「<メソッド名> は op <FQN>。」。説明の段と次の段の間は、空の行で空ける。
func opComment(o *ir.Op) string {
	first := describe(golang.MethodName(o.Name), o.Description, "op "+o.Name+"。")
	return comment(strings.Join(append([]string{first}, opFacts(o, &o.Signature)...), "\n\n"))
}

// depComment は、別の名前空間の口 `<M>Ops` のメソッドのコメント: 一行目は「<メソッド名> は op <FQN>: <説明>」。
// Throws などは、見る側から見た値（View.ops[<FQN>]）。
func depComment(o *ir.Op, viewed *ir.ViewedSignature) string {
	first := golang.MethodName(o.Name) + " は op " + o.Name
	if b := body(o.Description); b != "" {
		first += ": " + b
	} else {
		first += "。"
	}
	sig := &o.Signature
	if viewed != nil {
		sig = &viewed.Signature
	}
	return comment(strings.Join(append([]string{first}, opFacts(o, sig)...), "\n\n"))
}

// caseSet は遷移のケースの集合の書き方: `any` か `A | B`。
func caseSet(s ir.CaseSet) string {
	if s.Any {
		return "any"
	}
	return strings.Join(s.Cases, " | ")
}

// effectTarget は、効果の宛先の書き方（状態の単位の命令のコメント、effect の定数のコメント）: 組み込みの効果なら
// `<状態の FQN>.<post / put / delete>`、遷移なら `<宛先の FQN>: <前> -> <後>`。
func effectTarget(e *ir.Effect) string {
	switch t := e.Target.(type) {
	case ir.BuiltinTarget:
		return t.State + "." + string(t.Operation)
	case ir.TransitionTarget:
		return t.Of + ": " + caseSet(t.From) + " -> " + caseSet(t.To)
	}
	return ""
}

// joinJa は名前を「、」でつなぐ。
func joinJa(names []string) string { return strings.Join(names, "、") }
