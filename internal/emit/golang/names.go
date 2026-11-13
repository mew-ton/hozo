package golang

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// reservedLocals は配線の本体と op のメソッドで生成したコードが使う名前（docs/emit-go.md「名前」）。
// 引数名・束縛名がこれと同じなら末尾に `_`。
var reservedLocals = map[string]bool{"o": true, "err": true, "ptr": true, "States": true, "runtime": true}

// local は引数名・束縛名を Go の識別子にする（docs/emit-go.md「名前」の「op の引数名、配線の束縛名」）。
// 予約語に加えて、生成したコードが使う o / err / ptr / States / runtime（*runtime.MissingError を返すためのパッケージ名）
// とも衝突させない。
func local(name string) string {
	n := gol.LocalName(name)
	if reservedLocals[n] {
		return n + "_"
	}
	return n
}

// checkNames は GOEMIT-2（docs/emit-go.md「名前」の「大文字化で衝突する二つの名前」「型名どうしの衝突」と、
// 「state」の「単位の中でフィールド名が衝突したら」）。見るのは出力の対象だけ:
// model のフィールド名、op 名、状態の単位（StateSet のフィールドと、単位の中の問い合わせ・命令）、Deps と Wrappers の
// フィールド、そしてパッケージに出す型と、生成したコードが決まった名前で出す宣言（doc.go「文書にない決定」）。
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
	// 状態の単位: StateSet のフィールド名と、単位の中のフィールド名。
	fields := map[string]string{}
	for _, u := range e.units {
		span := e.lookup(u.fqn).Common().Span
		g := e.unitField(u)
		if prev, ok := fields[g]; ok {
			e.errorf(CodeNameCollision, &span, "状態 %s と %s は Go では StateSet の同じフィールド名 %s になる", prev, u.fqn, g)
			continue
		}
		fields[g] = u.fqn
		inUnit := map[string]string{}
		for _, q := range e.unitQueries(u) {
			if prev, ok := inUnit[q]; ok {
				e.errorf(CodeNameCollision, &span, "状態 %s の単位で、%s と問い合わせ %s が同じフィールド名になる", u.fqn, prev, q)
				continue
			}
			inUnit[q] = "問い合わせ " + q
		}
		for _, op := range u.commands {
			g := e.methodName(op)
			if prev, ok := inUnit[g]; ok {
				e.errorf(CodeNameCollision, &op.Span, "状態 %s の単位で、%s と命令の op %s が同じフィールド名 %s になる", u.fqn, prev, op.Name, g)
				continue
			}
			inUnit[g] = "命令の op " + op.Name
		}
	}
	deps := map[string]string{}
	for _, ns := range e.orderedDepsNS() {
		g := depsField(ns)
		if prev, ok := deps[g]; ok {
			e.errorf(CodeNameCollision, nil, "名前空間 %s と %s は Go では Deps の同じフィールド名 %s になる", prev, ns, g)
			continue
		}
		deps[g] = ns
	}
	groups, byGroup := e.wrapperGroups()
	for _, name := range groups {
		first := byGroup[name][0].name
		for _, w := range byGroup[name] {
			if w.name != first {
				e.errorf(CodeNameCollision, &w.app.Span, "デコレータ %s と %s は Go では Wrappers の同じフィールド名 %s になる", first, w.name, name)
				break
			}
		}
	}

	// パッケージの宣言の名前。
	types := map[string]string{}
	add := func(goName, what string, span *ir.Span) {
		if prev, ok := types[goName]; ok {
			e.errorf(CodeNameCollision, span, "%s と %s は Go では同じ名前 %s になる", prev, what, goName)
			return
		}
		types[goName] = what
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
		if c.SubjectType() != nil {
			add(e.typeName(c.Name)+"Cases", c.Name+" の述語のインターフェース", &c.Span)
		}
	}
	for _, o := range e.ops {
		add(e.funcTypeName(o), "op "+o.Name+" の関数の型", &o.Span)
		for _, w := range e.wrappers[o.Name] {
			add(w.typ, "op "+o.Name+" の @"+w.name+" の包む関数の型", &w.app.Span)
		}
	}
	for _, name := range groups {
		add(name+"Wrappers", "@"+byGroup[name][0].name+" の包む関数の構造体", nil)
	}
	for _, ns := range e.orderedDepsNS() {
		add(depsField(ns)+"Ops", "名前空間 "+ns+" の op の構造体", nil)
		for _, fqn := range e.orderedDepsOps(ns) {
			add(e.depFuncTypeName(e.op(fqn)), "op "+fqn+" の関数の型", nil)
		}
	}
	for _, u := range e.units {
		span := e.lookup(u.fqn).Common().Span
		add(e.unitType(u), "状態 "+u.fqn+" の単位", &span)
	}
	// 生成したコードが決まった名前で出す宣言。
	fixed := func(name string, present bool) {
		if present {
			add(name, "生成したコードの "+name, nil)
		}
	}
	hasOps := len(e.ops) > 0
	fixed("Ops", hasOps)
	fixed("New", hasOps)
	fixed("Leaves", len(e.leaves) > 0)
	fixed("Deps", len(deps) > 0)
	fixed("Wrappers", len(groups) > 0)
	fixed("Classifiers", len(e.valueClassifies()) > 0)
	fixed("StateSet", len(e.units) > 0)
	fixed("States", len(e.units) > 0)
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
