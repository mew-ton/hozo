package golang

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// reservedLocals は配線の本体と op のメソッドで生成器が使う識別子。引数名・束縛名がこれと同じなら末尾に `_`。
var reservedLocals = map[string]bool{"ctx": true, "o": true, "err": true, "ptr": true}

// local は引数名・束縛名を Go の識別子にする（docs/emit-go.md「名前」の「op の引数名、配線の束縛名」）。
// 予約語に加えて、生成器が使う ctx / o / err / ptr とも衝突させない。
func local(name string) string {
	n := gol.LocalName(name)
	if reservedLocals[n] {
		return n + "_"
	}
	return n
}

// checkNames は GOEMIT-2（docs/emit-go.md「名前」の「大文字化で衝突する二つの名前」）。
// model のフィールド名、viewer の op 名、そしてパッケージの型名（doc.go「文書にない決定」）。
func (e *emitter) checkNames() {
	for _, m := range e.models {
		seen := map[string]string{}
		for _, f := range m.Fields {
			g := gol.FieldName(f.Name)
			if prev, ok := seen[g]; ok {
				e.errorf(CodeNameCollision, &f.Span, "model %s のフィールド %s と %s は Go では同じ名前 %s になる", m.Name, prev, f.Name, g)
				continue
			}
			seen[g] = f.Name
		}
	}
	seen := map[string]string{}
	for _, o := range e.ops {
		g := gol.MethodName(localName(o.Name))
		if prev, ok := seen[g]; ok {
			e.errorf(CodeNameCollision, &o.Span, "op %s と %s は Go では同じメソッド名 %s になる", prev, o.Name, g)
			continue
		}
		seen[g] = o.Name
	}
	types := map[string]string{}
	add := func(goName, fqn string, span *ir.Span) {
		if prev, ok := types[goName]; ok {
			e.errorf(CodeNameCollision, span, "%s と %s は Go では同じ型名 %s になる", prev, fqn, goName)
			return
		}
		types[goName] = fqn
	}
	for _, m := range e.models {
		add(e.typeName(m.Name), m.Name, &m.Span)
	}
	for _, s := range e.scalars {
		add(e.typeName(s.Name), s.Name, &s.Span)
	}
	for _, u := range e.unions {
		if u.Variant != ir.UnionArms {
			continue
		}
		add(e.typeName(u.Name), u.Name, &u.Span)
		for _, a := range u.Arms {
			if w, ok := e.armWrapper(u, a); ok {
				add(w, u.Name+"."+a.Name+"（包む型）", &a.Span)
			}
		}
	}
	for _, x := range e.exceptions {
		add(e.typeName(x.Name), x.Name, &x.Span)
	}
	for _, c := range e.classifies {
		add(e.typeName(c.Name), c.Name, &c.Span)
	}
}

// comment は text の各行に "// " を付ける。text が空なら ""。末尾に改行を一つ付ける。
func comment(text string) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			b.WriteString("//\n")
			continue
		}
		b.WriteString("// " + line + "\n")
	}
	return b.String()
}

// docText は宣言のコメントの本文。description があれば body、なければ fallback。
func docText(d *ir.Doc, fallback string) string {
	if d != nil && strings.TrimSpace(d.Body) != "" {
		return strings.TrimSpace(d.Body)
	}
	return fallback
}

// declComment は「<Name> は <body>」の形のコメント（docs/emit-go.md「model」の例）。
func declComment(goName string, d *ir.Doc, fallback string) string {
	return comment(joinJa(goName+" は", docText(d, fallback)))
}

// joinJa は日本語の文をつなぐ。後ろが ASCII で始まるなら間に空白を置く（文書の書き方に合わせる）。
func joinJa(prefix, text string) string {
	if text == "" {
		return prefix
	}
	if text[0] < 0x80 {
		return prefix + " " + text
	}
	return prefix + text
}

// fqnList は FQN の列を ", " でつなぐ。
func fqnList(s []string) string { return strings.Join(s, ", ") }
