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
		// op のほか、exception、effect、read の引数も同じ形（docs/ir.md の OpParam）
		owner := r.prog.Lookup(t.Decl)
		var params []ir.OpParam
		switch d := owner.(type) {
		case *ir.Op:
			params = d.Params
		case *ir.Exception:
			params = d.Params
		case *ir.Effect:
			params = d.Params
		case *ir.Read:
			params = d.Params
		default:
			return "", false
		}
		for _, p := range params {
			if p.Name == t.Name {
				code(r.head(owner), "  "+p.Name+": "+r.typ(p.Type))
				r.paramDoc(&b, owner.Common().Description, p.Name)
				return b.String(), true
			}
		}
	case sema.TargetBinding:
		op, ok := r.prog.Lookup(t.Decl).(*ir.Op)
		if !ok || op.Body == nil {
			return "", false
		}
		found := false
		ir.WalkStatements(op.Body.Statements, func(st ir.Statement) {
			if found {
				return
			}
			switch c := st.(type) {
			case ir.ValStmt:
				if c.Name != t.Name {
					return
				}
				found = true
				line := "  val " + c.Name
				if c.Type != nil {
					line += ": " + r.typ(c.Type)
				}
				if c.Expr != nil {
					line += " = " + r.expr(c.Expr)
				}
				code(r.head(op), line)
			case ir.ForStmt:
				if c.Var != t.Name {
					return
				}
				found = true
				line := "  for (val " + c.Var
				if c.VarType != nil {
					line += ": " + r.typ(c.VarType)
				}
				code(r.head(op), line+" of …)")
			}
		})
		if found {
			return b.String(), true
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
					prevented = append(prevented, r.effectRef(rel.Effect))
				case ir.Kept:
					kept = append(kept, r.effectRef(rel.Effect))
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
			fmt.Fprintf(&b, "- `%s`\n", r.effectRef(ef))
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
		if d.Index != nil {
			// インデックスの model は、インデックスまで出す（docs/lsp.md「ホバー」）
			s += " { [" + d.Index.Name + ": " + r.typ(d.Index.Key) + "]: " + r.typ(d.Index.Value) + " }"
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
		// 引数のない例外は括弧を書かない（docs/grammar.md「exception / effect / read」）
		if len(d.Params) == 0 {
			return "exception " + name
		}
		return "exception " + name + r.params(d.Params)
	case *ir.Effect:
		return "effect fn " + name + r.params(d.Params) + r.effectTarget(d) + r.rowTransition(d)
	case *ir.Read:
		// 「)」の後ろは、「:」で始める、関数の型: 戻り値と、reads の節（docs/grammar.md「関数の宣言」）
		s := "read fn " + name + r.params(d.Params)
		var parts []string
		if d.Returns != nil {
			parts = append(parts, r.typ(d.Returns))
		}
		if d.State != "" {
			parts = append(parts, "reads "+r.rel(d.State))
		}
		if len(parts) > 0 {
			s += ": " + strings.Join(parts, " ")
		}
		return s
	case *ir.Classify:
		return "classify " + name + " of " + r.classifySubject(d.Subject)
	case *ir.Decorator:
		return r.decoratorHead(name, d)
	case *ir.Op:
		s := "fn " + name + r.params(d.Params)
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

// params は引数の並び `(a: T, b: U)`（op、exception、effect、read の見出し）。
func (r *renderer) params(ps []ir.OpParam) string {
	var params []string
	for _, p := range ps {
		params = append(params, p.Name+": "+r.typ(p.Type))
	}
	return "(" + strings.Join(params, ", ") + ")"
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
	if d.Repeatable {
		b.WriteString("@repeatable ")
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
		cases := 0
		for _, t := range d.Targets {
			// 分類のケースは、絞り（Decorator.caseTargets。Targets の "case" と、同じ順）ごとに書く
			if t == ir.TargetCase && cases < len(d.CaseTargets) {
				ts = append(ts, r.caseKind(d.CaseTargets[cases]))
				cases++
				continue
			}
			ts = append(ts, string(t))
		}
		b.WriteString(" for " + strings.Join(ts, ", "))
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
	// デコレータが op に運ぶ reads / effects の節（docs/grammar.md「decorator の宣言」）。要素と区切りは op の節と同じ
	if len(d.Reads) > 0 {
		var reads []string
		for _, rd := range d.Reads {
			reads = append(reads, r.rel(rd.String()))
		}
		b.WriteString(" reads " + strings.Join(reads, ", "))
	}
	if len(d.Effects) > 0 {
		var effects []string
		for _, e := range d.Effects {
			effects = append(effects, r.effectRef(e))
		}
		b.WriteString(" effects " + strings.Join(effects, ", "))
	}
	return b.String()
}

// effectRef は effects の要素を Hozo の構文で書く（docs/grammar.md「op」の EffectElem）: 名前で書いた効果は、見ている
// 名前空間から見た名前。直に書いた効果は `状態.操作`。
func (r *renderer) effectRef(e ir.EffectRef) string {
	if d, ok := e.(ir.DirectEffect); ok {
		return r.rel(d.State) + "." + string(d.Operation)
	}
	return r.rel(e.String())
}

// param はデコレータの引数の宣言。残りの引数は `...名前: 型`（docs/grammar.md「decorator の宣言」の RestParam）。
func (r *renderer) param(p ir.Param) string {
	s := p.Name + ": " + r.paramType(p)
	if p.Rest {
		return "..." + s
	}
	return s
}

// throwsClause は `throws A, B prevents effects` の形。
func (r *renderer) throwsClause(c ir.ThrowsClause) string {
	s := "throws " + r.rels(c.Exceptions)
	for _, rel := range c.Relations {
		s += " " + string(rel.Kind) + " effects"
	}
	return s
}

// caseKind は、デコレータの宣言の、分類のケース（docs/grammar.md「decorator の宣言」の CaseKind）。もとの形（case of …）で書く。
func (r *renderer) caseKind(k ir.CaseKind) string {
	switch {
	case k.Classify != "":
		return "case of " + r.rel(k.Classify)
	case k.Subject != nil:
		return "case of " + r.typ(k.Subject)
	}
	return "case of any"
}

// rels は節の並び（throws / catches の例外）を Hozo の構文で書く。区切りは `,`（docs/grammar.md「op」）。
func (r *renderer) rels(fqns []string) string {
	var out []string
	for _, f := range fqns {
		out = append(out, r.rel(f))
	}
	return strings.Join(out, ", ")
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
		if t.Case != nil {
			s = "any " + r.caseKind(*t.Case)
		}
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
		// on の後ろに ? と [] は続けられないので、絞り込んだ型を包む形は、nullable<…>、list<…>
		if _, narrowed := t.Of.(ir.CaseType); narrowed {
			return "nullable<" + r.typ(t.Of) + ">"
		}
		return r.typ(t.Of) + "?"
	case ir.ArrayType:
		if _, narrowed := t.Of.(ir.CaseType); narrowed {
			return "list<" + r.typ(t.Of) + ">"
		}
		return r.typ(t.Of) + "[]"
	case ir.CaseType:
		// 絞り込んだ型は、仕様の綴りで（docs/grammar.md「型」の OnType）: T on C.K | C.K2
		cases := make([]string, len(t.Cases))
		for i, c := range t.Cases {
			cases[i] = r.rel(t.Classify) + "." + c
		}
		return r.typ(t.Of) + " on " + strings.Join(cases, " | ")
	case ir.NeverType:
		return "never"
	}
	return t.String()
}

// caseLine は分類のケースを Hozo の構文で書く。as を持つケースなら `Registered: findUser as type.User`、残りのケースなら
// `Unregistered: findUser as else`（docs/lsp.md「ホバー」）。as の前は、値の分類なら対象の型、op の分類ならその op、
// 状態の分類ならその状態。
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
		if as.By == ir.NarrowByElse || as.By == ir.NarrowByNone {
			// 残りのケースと、ない側のケースは、型を書いていない（as.Type は、そのケースでの対象の型）
			parts = append(parts, subject+" as "+string(as.By))
			continue
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
	case ir.CaseValue:
		return r.rel(v.Classify) + "." + v.Case
	case ir.RefsValue:
		// 一つの引数に渡した宣言の集合の区切りは `|`（docs/grammar.md「デコレータの適用」）
		names := make([]string, len(v.Targets))
		for i, t := range v.Targets {
			names[i] = r.rel(t)
		}
		return strings.Join(names, " | ")
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

// effectTarget は効果の宛先を Hozo の構文で書く（docs/lsp.md「ホバー」: `effect fn createSession(session: Session): effects sessions.post`、
// `effect fn placeOrder(): effects Checkout as Shopping -> Ordered` の「:」から後ろ）。宛先が決まっていなければ空。
func (r *renderer) effectTarget(e *ir.Effect) string {
	// 投げる例外（Effect.throws）は、effects の前に書く（docs/grammar.md「関数の宣言」）
	head := ": "
	if len(e.Throws) > 0 {
		head += "throws " + r.rels(e.Throws) + " "
	}
	switch t := e.Target.(type) {
	case ir.BuiltinTarget:
		return head + "effects " + r.rel(t.State) + "." + string(t.Operation)
	case ir.TransitionTarget:
		return head + "effects " + r.rel(t.Of) + " as " + caseSet(t.From) + " -> " + caseSet(t.To)
	}
	return ""
}

// rowTransition は行の遷移を Hozo の構文で書く（docs/lsp.md「ホバー」: `… effects orders.put as Stage.InProduction` の
// as から後ろ。頭に空白を一つ置く）。後のケースがあれば `as <分類>.<ケース>`、なければ `as else`。前のケースは、引数の型に出る。
// 分類の名前は、見ている名前空間から見た修飾。行の遷移を持たない効果は空。
func (r *renderer) rowTransition(e *ir.Effect) string {
	if _, ok := e.Target.(ir.BuiltinTarget); !ok || e.Row == nil {
		return ""
	}
	if e.Row.To == "" {
		return " as else"
	}
	return " as " + r.rel(e.Row.Classify) + "." + e.Row.To
}

func caseSet(c ir.CaseSet) string {
	if c.Any {
		return "any"
	}
	return strings.Join(c.Cases, ", ")
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
			return "match (" + r.atom(s.Atom) + " by " + m.Classify + ")" // 標準の分類。classify は "list" か "nullable"
		}
		return "match (" + r.atom(s.Atom) + " by " + r.rel(m.Classify) + ")"
	case ir.ClassifyOwnedSubject:
		return "match (by " + r.rel(m.Classify) + ")"
	}
	return "match (by " + r.rel(m.Classify) + ")"
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
	return r.callee(c.Callee) + "(" + strings.Join(args, ", ") + ")"
}

// callee は呼び出し先の名前: op、read、効果の宣言は見ている名前空間から見た名前、直に呼ぶ組み込みの効果は「状態.操作」。
func (r *renderer) callee(c ir.Callee) string {
	switch c := c.(type) {
	case ir.OpCallee:
		return r.rel(c.Op)
	case ir.ReadCallee:
		return r.rel(c.Read)
	case ir.EffectCallee:
		if c.Effect != nil {
			return r.effectRef(c.Effect)
		}
	}
	return "?"
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
