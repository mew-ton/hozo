package lsp

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/sema"
	"github.com/mew-ton/hozo/ir"
)

// hover は textDocument/hover（docs/lsp.md「ホバー」）。カーソル位置のトークンの解決先について Markdown を返す。
// 解決できなければ nil。
func (s *server) hover(p protocol.TextDocumentPositionParams) *protocol.Hover {
	id, ok := s.fileOf(p.TextDocument.URI)
	if !ok {
		return nil
	}
	off := s.enc.toOffset(s.proj.Files.Content(id), p.Position)
	t, span, ok := s.targetAt(id, off)
	if !ok {
		return nil
	}
	r := &renderer{prog: s.proj.Sema.Program, declNS: s.declNS, viewer: s.namespaceAt(id, off)}
	md, ok := r.hover(t)
	if !ok {
		return nil
	}
	rng := s.enc.toRange(s.proj.Files, span)
	return &protocol.Hover{Contents: protocol.MarkupContent{Kind: protocol.Markdown, Value: md}, Range: &rng}
}

// renderer は IR からホバーの Markdown を組み立てる。名前は viewer（見ている名前空間）から見た修飾にする
// （docs/lsp.md「ホバー」の 1: 同じ名前空間なら修飾なし）。
type renderer struct {
	prog   *ir.Program
	declNS map[string]string
	viewer string
}

// hover は解決先ごとの Markdown。docs/lsp.md「ホバー」の順: 1 宣言の見出し（コードブロック）、2 description.body、
// 3 op なら throws / effects、4 デコレータの適用なら宣言の 1〜2 と固定した引数。
func (r *renderer) hover(t sema.Target) (string, bool) {
	var b strings.Builder
	code := func(lines ...string) {
		b.WriteString("```hozo\n")
		for _, l := range lines {
			b.WriteString(l)
			b.WriteByte('\n')
		}
		b.WriteString("```\n")
	}
	body := func(d *ir.Doc) {
		if d != nil && strings.TrimSpace(d.Body) != "" {
			b.WriteString("\n")
			b.WriteString(strings.TrimSpace(d.Body))
			b.WriteString("\n")
		}
	}
	switch t.Kind {
	case sema.TargetNamespace:
		code("namespace " + t.Decl)
		if ns := r.prog.Namespace(t.Decl); ns != nil {
			body(ns.Description)
		}
		return b.String(), true
	case sema.TargetDecl:
		d := r.prog.Lookup(t.Decl)
		if d == nil {
			code(r.rel(t.Decl))
			return b.String(), true
		}
		code(r.head(d))
		body(d.Common().Description)
		switch d := d.(type) {
		case *ir.Op:
			b.WriteString(r.signature(d))
		case *ir.Decorator:
			if d.Alias != nil {
				// 4: 名前付きデコレータは、汎用デコレータの宣言の 1〜2 と固定した引数
				if g, ok := r.prog.Lookup(d.Alias.Target).(*ir.Decorator); ok {
					b.WriteString("\n")
					code(r.head(g))
					body(g.Description)
				}
				if len(d.Alias.Args) > 0 {
					b.WriteString("\n固定した引数:\n")
					for _, a := range d.Alias.Args {
						fmt.Fprintf(&b, "- `%s: %s`\n", a.Param, r.argValue(a.Value))
					}
				}
			}
		}
		return b.String(), true
	case sema.TargetField:
		m, ok := r.prog.Lookup(t.Decl).(*ir.Model)
		if !ok {
			return "", false
		}
		for _, f := range m.Fields {
			if f.Name == t.Name {
				code(r.head(m), "  "+r.field(f))
				body(f.Description)
				return b.String(), true
			}
		}
	case sema.TargetArm:
		u, ok := r.prog.Lookup(t.Decl).(*ir.Union)
		if !ok {
			return "", false
		}
		for _, a := range u.Arms {
			if a.Name == t.Name {
				code(r.head(u), "  "+a.Name+": "+r.typ(a.Type))
				body(a.Description)
				return b.String(), true
			}
		}
	case sema.TargetCase:
		c, ok := r.prog.Lookup(t.Decl).(*ir.Classify)
		if !ok {
			return "", false
		}
		for _, cs := range c.Cases {
			if cs.Name == t.Name {
				code(r.head(c), "  "+r.caseLine(c, cs))
				body(cs.Description)
				return b.String(), true
			}
		}
	case sema.TargetDecoratorParam:
		d, ok := r.prog.Lookup(t.Decl).(*ir.Decorator)
		if !ok {
			return "", false
		}
		for _, p := range d.Params {
			if p.Name == t.Name {
				code(r.head(d), "  "+r.param(p))
				r.paramDoc(&b, d.Description, p.Name)
				return b.String(), true
			}
		}
	case sema.TargetOpParam:
		op, ok := r.prog.Lookup(t.Decl).(*ir.Op)
		if !ok {
			return "", false
		}
		for _, p := range op.Params {
			if p.Name == t.Name {
				code(r.head(op), "  "+p.Name+": "+r.typ(p.Type))
				r.paramDoc(&b, op.Description, p.Name)
				return b.String(), true
			}
		}
	case sema.TargetBinding:
		op, ok := r.prog.Lookup(t.Decl).(*ir.Op)
		if !ok || op.Body == nil {
			return "", false
		}
		for _, st := range op.Body.Statements {
			if c, ok := st.(ir.ConstStmt); ok && c.Name == t.Name {
				code(r.head(op), "  const "+c.Name+" = "+r.expr(c.Expr))
				return b.String(), true
			}
		}
	}
	return "", false
}

// paramDoc は @param の説明（Doc.params）を書く。
func (r *renderer) paramDoc(b *strings.Builder, d *ir.Doc, name string) {
	if d == nil {
		return
	}
	for _, p := range d.Params {
		if p.Name == name && strings.TrimSpace(p.Text) != "" {
			b.WriteString("\n")
			b.WriteString(strings.TrimSpace(p.Text))
			b.WriteString("\n")
		}
	}
}

// signature は docs/lsp.md「ホバー」の 3: Signature の throws と effects の一覧。例外ごとに、効果の状態
// （relations）が unknown でないものを添える。続けて、reads が空でなければその一覧、純粋なら「純粋」の一行。
func (r *renderer) signature(op *ir.Op) string {
	var b strings.Builder
	sig := op.Signature
	b.WriteString("\nthrows:")
	if len(sig.Exceptions) == 0 {
		b.WriteString(" なし\n")
	} else {
		b.WriteString("\n")
		for _, ex := range sig.Exceptions {
			fmt.Fprintf(&b, "- `%s`", r.rel(ex))
			var prevented, kept []string
			for _, rel := range sig.Relations {
				if rel.Exception != ex {
					continue
				}
				switch rel.State {
				case ir.Prevented:
					prevented = append(prevented, r.rel(rel.Effect))
				case ir.Kept:
					kept = append(kept, r.rel(rel.Effect))
				}
			}
			var notes []string
			if len(prevented) > 0 {
				notes = append(notes, "prevents "+strings.Join(prevented, " | "))
			}
			if len(kept) > 0 {
				notes = append(notes, "keeps "+strings.Join(kept, " | "))
			}
			if len(notes) > 0 {
				b.WriteString(" — " + strings.Join(notes, ", "))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\neffects:")
	if len(sig.Effects) == 0 {
		b.WriteString(" なし\n")
	} else {
		b.WriteString("\n")
		for _, ef := range sig.Effects {
			fmt.Fprintf(&b, "- `%s`\n", r.rel(ef))
		}
	}
	if len(sig.Reads) > 0 {
		b.WriteString("\nreads:\n")
		for _, st := range sig.Reads {
			fmt.Fprintf(&b, "- `%s`\n", r.rel(st))
		}
	}
	if sig.Pure {
		b.WriteString("\n純粋\n")
	}
	return b.String()
}

// rel は FQN を見ている名前空間から見た形にする: 宣言した名前空間が viewer と同じなら修飾を落とす。
// デコレータの引数の参照 "$param:名前" は名前だけ。
func (r *renderer) rel(fqn string) string {
	if strings.HasPrefix(fqn, "$param:") {
		return strings.TrimPrefix(fqn, "$param:")
	}
	ns, ok := r.declNS[fqn]
	if !ok {
		// IR にない名前（解決できなかったものなど）は "." の前を名前空間とみなす
		if i := strings.LastIndex(fqn, "."); i >= 0 {
			ns = fqn[:i]
		}
	}
	if ns == r.viewer {
		if ns == "" {
			return fqn
		}
		return strings.TrimPrefix(fqn, ns+".")
	}
	return fqn
}

// head は docs/lsp.md「ホバー」の 1: Hozo の構文で書いた宣言の見出し。
func (r *renderer) head(d ir.Decl) string {
	name := r.rel(d.Common().Name)
	switch d := d.(type) {
	case *ir.Model:
		s := "model " + name
		if d.Extends != nil {
			s += " extends " + r.rel(d.Extends.Base)
		}
		return s
	case *ir.Scalar:
		return "scalar " + name + " extends " + r.typ(d.Base)
	case *ir.Union:
		return "union " + name
	case *ir.State:
		// 状態の見出しだけは、形の語で書く（docs/lsp.md「ホバー」）
		return "state " + name + ": " + string(d.Shape) + "<" + r.typ(d.Element) + ">"
	case *ir.Exception:
		s := "exception " + name
		if d.Value != nil {
			s += ": " + r.typ(d.Value)
		}
		return s
	case *ir.Effect:
		return "effect " + name + r.effectTarget(d.Target)
	case *ir.Classify:
		if d.FromCondition {
			return "condition " + name + " of " + r.classifySubject(d.Subject)
		}
		return "classify " + name + " of " + r.classifySubject(d.Subject)
	case *ir.Decorator:
		return r.decoratorHead(name, d)
	case *ir.Op:
		var params []string
		for _, p := range d.Params {
			params = append(params, p.Name+": "+r.typ(p.Type))
		}
		s := "op " + name + "(" + strings.Join(params, ", ") + ")"
		if d.Exported {
			s = "export " + s
		}
		if d.Returns != nil {
			s += ": " + r.typ(d.Returns)
		}
		return s
	}
	return name
}

func (r *renderer) decoratorHead(name string, d *ir.Decorator) string {
	if d.Alias != nil {
		var args []string
		for _, a := range d.Alias.Args {
			args = append(args, a.Param+": "+r.argValue(a.Value))
		}
		return "decorator " + name + " = " + r.rel(d.Alias.Target) + "(" + strings.Join(args, ", ") + ")"
	}
	var b strings.Builder
	if d.External {
		b.WriteString("@external ")
	}
	b.WriteString("decorator " + name)
	if len(d.Params) > 0 {
		var ps []string
		for _, p := range d.Params {
			ps = append(ps, r.param(p))
		}
		b.WriteString("(" + strings.Join(ps, ", ") + ")")
	}
	if !(len(d.Targets) == 1 && d.Targets[0] == ir.TargetOp) && len(d.Targets) > 0 {
		var ts []string
		for _, t := range d.Targets {
			ts = append(ts, string(t))
		}
		b.WriteString(" for " + strings.Join(ts, " | "))
	}
	for _, c := range d.Throws {
		b.WriteString(" " + r.throwsClause(c))
	}
	for _, c := range d.Catches {
		b.WriteString(" catches " + r.rels(c.Exceptions) + " " + string(c.Mode))
		if c.Mode == ir.Halts && c.Halts != "" {
			b.WriteString(" " + r.rel(c.Halts))
		}
	}
	// デコレータが op に運ぶ reads / effects の節（docs/grammar.md「decorator の宣言」）。区切りは op の節と同じ
	if len(d.Reads) > 0 {
		var states []string
		for _, st := range d.Reads {
			states = append(states, r.rel(st))
		}
		b.WriteString(" reads " + strings.Join(states, ", "))
	}
	if len(d.Effects) > 0 {
		b.WriteString(" effects " + r.rels(d.Effects))
	}
	return b.String()
}

// param はデコレータの引数の宣言。残りの引数は `...名前: 型`（docs/grammar.md「decorator の宣言」の RestParam）。
func (r *renderer) param(p ir.Param) string {
	s := p.Name + ": " + r.paramType(p)
	if p.Rest {
		return "..." + s
	}
	return s
}

// throwsClause は `throws A | B prevents effects` の形。
func (r *renderer) throwsClause(c ir.ThrowsClause) string {
	s := "throws " + r.rels(c.Exceptions)
	for _, rel := range c.Relations {
		s += " " + string(rel.Kind) + " effects"
	}
	return s
}

func (r *renderer) rels(fqns []string) string {
	var out []string
	for _, f := range fqns {
		out = append(out, r.rel(f))
	}
	return strings.Join(out, " | ")
}

// field は `name: Type`。省略可能は型の `?` で書く（docs/grammar.md「model」）。
func (r *renderer) field(f ir.Field) string {
	return f.Name + ": " + r.typ(f.Type)
}

// paramType はデコレータの引数の型（docs/grammar.md「decorator の宣言」の ParamType）。
func (r *renderer) paramType(p ir.Param) string {
	s := ""
	switch t := p.Type.(type) {
	case ir.TypeParam:
		var ts []string
		for _, x := range t.Types {
			ts = append(ts, r.typ(x))
		}
		s = strings.Join(ts, " | ")
	case ir.AnyParam:
		s = "any " + string(t.Of)
		if t.On != nil {
			s += " on " + r.rel(t.On.State) + "." + string(t.On.Operation)
		}
	default:
		s = "?"
	}
	if p.Optional {
		s += "?"
	}
	return s
}

// typ は型を Hozo の構文で書く（docs/grammar.md「型」）。参照は viewer から見た修飾。
func (r *renderer) typ(t ir.Type) string {
	switch t := t.(type) {
	case nil:
		return "void"
	case ir.CoreType:
		return string(t.Name)
	case ir.RefType:
		return r.rel(t.Target)
	case ir.OptionalType:
		return r.typ(t.Of) + "?"
	case ir.ArrayType:
		return r.typ(t.Of) + "[]"
	case ir.RecordType:
		return "Record<" + r.typ(t.Key) + ", " + r.typ(t.Value) + ">"
	case ir.CaseType:
		cases := strings.Join(t.Cases, " | ")
		if len(t.Cases) != 1 {
			cases = "(" + cases + ")"
		}
		return r.rel(t.Classify) + "." + cases + "<" + r.typ(t.Of) + ">"
	case ir.NeverType:
		return "never"
	}
	return t.String()
}

// caseLine は分類のケースを Hozo の構文で書く。as を持つケースなら `Registered: findUser as type.User`
// （docs/lsp.md「ホバー」）。as の前は、値の分類なら対象の型、op の分類ならその op、状態の分類ならその状態。
func (r *renderer) caseLine(c *ir.Classify, cs ir.ClassifyCase) string {
	if len(cs.As) == 0 {
		return cs.Name
	}
	var parts []string
	for _, as := range cs.As {
		subject := "?"
		switch s := c.Subject.(type) {
		case ir.TypeSubject:
			subject = r.typ(s.Type)
		case ir.OpSubject:
			subject = r.rel(s.Op)
		case ir.StatesSubject:
			subject = r.rel(as.State)
		}
		parts = append(parts, subject+" as "+r.typ(as.Type))
	}
	return cs.Name + ": " + strings.Join(parts, ", ")
}

// argValue はデコレータの引数の値（docs/ir.md「デコレータの適用」の ArgValue）。
func (r *renderer) argValue(v ir.ArgValue) string {
	switch v := v.(type) {
	case ir.LiteralValue:
		return literal(v.Literal)
	case ir.RefValue:
		return r.rel(v.Target)
	case ir.RefsValue:
		return r.rels(v.Targets)
	case ir.VariantValue:
		return r.rel(v.Union) + "." + v.Arm
	case ir.ListValue:
		var items []string
		for _, it := range v.Items {
			items = append(items, r.argValue(it))
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	return "?"
}

// effectTarget は効果の宛先を Hozo の構文で書く（docs/lsp.md「ホバー」: `effect createSession on sessions.post`、
// `effect placeOrder on Checkout: Shopping -> Ordered`）。宛先が決まっていなければ空。
func (r *renderer) effectTarget(t ir.EffectTarget) string {
	switch t := t.(type) {
	case ir.BuiltinTarget:
		return " on " + r.rel(t.State) + "." + string(t.Operation)
	case ir.TransitionTarget:
		return " on " + r.rel(t.Of) + ": " + caseSet(t.From) + " -> " + caseSet(t.To)
	}
	return ""
}

func caseSet(c ir.CaseSet) string {
	if c.Any {
		return "any"
	}
	return strings.Join(c.Cases, " | ")
}

// classifySubject は分類の対象を Hozo の構文で書く（`classify WarrantyKind of server.Item`、
// `classify Registration of findUser`、`classify Checkout of carts, orders`）。対象が決まっていなければ "?"。
func (r *renderer) classifySubject(s ir.ClassifySubject) string {
	switch s := s.(type) {
	case ir.TypeSubject:
		return r.typ(s.Type)
	case ir.OpSubject:
		return r.rel(s.Op)
	case ir.StatesSubject:
		names := make([]string, len(s.States))
		for i, st := range s.States {
			names[i] = r.rel(st)
		}
		return strings.Join(names, ", ")
	}
	return "?"
}

// matchHead は match の頭を Hozo の構文で書く（docs/grammar.md「配線」の三つの形）。
func (r *renderer) matchHead(m ir.Match) string {
	switch s := m.Subject.(type) {
	case ir.ValueSubject:
		if m.Builtin() {
			return "match " + r.atom(s.Atom) + " by " + m.Classify // 標準の分類。classify は "list" か "nullable"
		}
		return "match " + r.atom(s.Atom) + " by " + r.rel(m.Classify)
	case ir.ClassifyOwnedSubject:
		return "match by " + r.rel(m.Classify)
	}
	return "match by " + r.rel(m.Classify)
}

// expr は配線の式を短く書く（束縛のホバー）。
func (r *renderer) expr(e ir.Expr) string {
	switch e := e.(type) {
	case ir.CallExpr:
		return r.call(e.Call)
	case ir.MatchExpr:
		return r.matchHead(e.Match) + " { ... }"
	case ir.AtomExpr:
		return r.atom(e.Atom)
	}
	return "..."
}

func (r *renderer) call(c ir.Call) string {
	var args []string
	for _, a := range c.Args {
		args = append(args, r.atom(a.Value))
	}
	return r.rel(c.Op) + "(" + strings.Join(args, ", ") + ")"
}

func (r *renderer) atom(a ir.Atom) string {
	switch a := a.(type) {
	case ir.NameAtom:
		return strings.Join(a.Path, ".")
	case ir.StateAtom:
		return strings.Join(append([]string{r.rel(a.State)}, a.Path...), ".")
	case ir.LiteralAtom:
		return literal(a.Literal)
	case ir.NoneAtom:
		return "none"
	case ir.RecordAtom:
		var fields []string
		for _, f := range a.Fields {
			fields = append(fields, f.Name+": "+r.atom(f.Value))
		}
		return "{ " + strings.Join(fields, ", ") + " }"
	}
	return "?"
}

// literal はリテラルを Hozo の構文で書く（docs/grammar.md「リテラル」）。
func literal(l ir.Literal) string {
	switch l := l.(type) {
	case ir.StringLiteral:
		return strconv.Quote(l.Value)
	case ir.IntLiteral:
		return strconv.FormatInt(l.Value, 10)
	case ir.FloatLiteral:
		return strconv.FormatFloat(l.Value, 'g', -1, 64)
	case ir.BooleanLiteral:
		return strconv.FormatBool(l.Value)
	case ir.DurationLiteral:
		return duration(l.Value)
	}
	return "?"
}

// duration はナノ秒の期間を、割り切れる最も大きい単位で書く（5m、30s、500ms）。
func duration(d time.Duration) string {
	units := []struct {
		unit time.Duration
		name string
	}{{time.Hour, "h"}, {time.Minute, "m"}, {time.Second, "s"}, {time.Millisecond, "ms"}, {time.Microsecond, "us"}, {time.Nanosecond, "ns"}}
	if d == 0 {
		return "0s"
	}
	for _, u := range units {
		if d%u.unit == 0 {
			return strconv.FormatInt(int64(d/u.unit), 10) + u.name
		}
	}
	return d.String()
}
