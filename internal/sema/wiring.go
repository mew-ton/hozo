package sema

import (
	"fmt"
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// wiringJob は配線の本体を下ろす仕事。すべての宣言が下りてから（lowerWirings）行う。
type wiringJob struct {
	sym  *symbol
	decl *ast.OpDecl
	op   *ir.Op
}

// binding は配線のスコープの名前: op の引数か const の束縛。
type binding struct {
	name    string
	typ     exprType
	span    source.Span
	isParam bool
	used    bool
	target  Target
}

func (b *binding) markUsed() { b.used = true }

// wiringScope は配線のスコープ（docs/resolve.md「スコープ」）。束縛はその行より下で見える。
type wiringScope struct {
	op       string
	params   map[string]*binding
	bindings map[string]*binding
	order    []*binding
}

// lookup は束縛か引数。
func (w *wiringScope) lookup(name string) (*binding, bool) {
	if b, ok := w.bindings[name]; ok {
		return b, true
	}
	b, ok := w.params[name]
	return b, ok
}

// lowerWirings はすべての配線の本体を下ろす。
func (a *analyzer) lowerWirings() {
	for _, j := range a.wirings {
		a.lowerWiring(j)
	}
}

// wiring は一つの配線の下ろしの状態。
type wiring struct {
	job   *wiringJob
	sc    *scope
	scope *wiringScope
}

// lowerWiring は配線の本体（docs/ir.md「配線」）。WIRE-2 / WIRE-3 / WIRE-4 / WIRE-8 と式の型。
func (a *analyzer) lowerWiring(j *wiringJob) {
	ws := &wiringScope{op: j.sym.fqn, params: map[string]*binding{}, bindings: map[string]*binding{}}
	for _, p := range j.op.Params {
		var span source.Span
		if ap := findAstParam(j.decl, p.Name); ap != nil {
			span = ap.Name.Span
		}
		typ := unknownType()
		if p.Type != nil {
			typ = valueType(p.Type)
		}
		ws.params[p.Name] = &binding{name: p.Name, typ: typ, span: span, isParam: true, target: Target{Kind: TargetOpParam, Decl: j.sym.fqn, Name: p.Name, Span: span}}
	}
	w := &wiring{job: j, scope: ws, sc: &scope{ns: j.sym.ns, wiring: ws}}
	body := &ir.Wiring{}
	stmts := j.decl.Body.Stmts
	for i, s := range stmts {
		if st, ok := a.lowerStmt(w, s, i == len(stmts)-1); ok {
			body.Statements = append(body.Statements, st)
		}
	}
	a.checkWIRE3Last(j, stmts)
	a.checkWIRE8(ws)
	j.op.Body = body
}

func findAstParam(d *ast.OpDecl, name string) *ast.OpParam {
	for _, p := range d.Params {
		if p != nil && p.Name.Name == name {
			return p
		}
	}
	return nil
}

// checkWIRE3Last は WIRE-3（docs/check.md「配線」）の「returns が null でないのに、本体の最後の文が return でない」。
func (a *analyzer) checkWIRE3Last(j *wiringJob, stmts []ast.Stmt) {
	if j.op.Returns == nil {
		return
	}
	if len(stmts) == 0 {
		a.errorf("WIRE-3", j.sym.nameSpan, "op %s は値を返すが、本体に return がない", j.sym.fqn)
		return
	}
	if _, ok := stmts[len(stmts)-1].(*ast.ReturnStmt); !ok && !hasReturn(stmts) {
		a.errorf("WIRE-3", j.sym.nameSpan, "op %s は値を返すので、本体の最後の文は return でなければならない", j.sym.fqn)
	}
}

// hasReturn は return の文があるか（最後以外にあれば lowerReturn が WIRE-3 を出している）。
func hasReturn(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		if _, ok := s.(*ast.ReturnStmt); ok {
			return true
		}
	}
	return false
}

// checkWIRE8 は WIRE-8（docs/check.md「配線」。warning）: 束縛した名前が下の行で使われていない。
func (a *analyzer) checkWIRE8(ws *wiringScope) {
	for _, b := range ws.order {
		if !b.used {
			a.warnf("WIRE-8", b.span, "束縛 %s は使われていない", b.name)
		}
	}
}

// lowerStmt は一つの文。
func (a *analyzer) lowerStmt(w *wiring, s ast.Stmt, last bool) (ir.Statement, bool) {
	switch s := s.(type) {
	case *ast.ConstStmt:
		return a.lowerConst(w, s)
	case *ast.ReturnStmt:
		return a.lowerReturn(w, s, last)
	case *ast.CallStmt:
		if s.Call == nil {
			return nil, false
		}
		t, call, ok := a.evalCall(w, s.Call, nil)
		if ok && t.kind != etNoValue && t.kind != etUnknown {
			a.errorf("WIRE-2", s.Call.Span, "呼び出し %s は値を返すので、束縛せずに書けない（設計書 §9）", call.Op)
		}
		if !ok {
			return nil, false
		}
		return ir.CallStmt{Call: call, Span: a.irSpan(s.Span)}, true
	case *ast.MatchStmt:
		if s.Match == nil {
			return nil, false
		}
		t, m := a.evalMatch(w, s.Match)
		if t.kind != etNoValue && t.kind != etUnknown {
			a.errorf("WIRE-2", s.Match.Span, "match は値を持つので、束縛せずに書けない（設計書 §9）")
		}
		return ir.MatchStmt{Match: m, Span: a.irSpan(s.Span)}, true
	}
	return nil, false
}

// lowerConst は `const x = 式;`。WIRE-4（同じ名前を二度束縛、引数と同じ名前）、WIRE-2（式が「値がない」）、
// TYPE-7（レコードリテラルの当てはめる先が決まらない）。
func (a *analyzer) lowerConst(w *wiring, s *ast.ConstStmt) (ir.Statement, bool) {
	t, expr, ok := a.evalExpr(w, s.Value)
	name := s.Name.Name
	switch t.kind {
	case etNoValue:
		a.errorf("WIRE-2", s.Value.NodeSpan(), "const %s に束縛する式は値を持たない（設計書 §9）", name)
		t = unknownType()
	case etRecord:
		a.errorf("TYPE-7", s.Value.NodeSpan(), "レコードリテラルの当てはめる先が決まらない（const %s）。引数か return の位置に書く", name)
		t = unknownType()
	}
	if prev, dup := w.scope.lookup(name); dup {
		what := "束縛"
		if prev.isParam {
			what = "引数"
		}
		a.errorf("WIRE-4", s.Name.Span, "%s は既に%sの名前（束縛は一度だけ。設計書 §9）", name, what).Related = []diag.Related{related(prev.span, "最初の定義")}
	} else {
		b := &binding{name: name, typ: t, span: s.Name.Span, target: Target{Kind: TargetBinding, Decl: w.job.sym.fqn, Name: name, Span: s.Name.Span}}
		w.scope.bindings[name] = b
		w.scope.order = append(w.scope.order, b)
	}
	if !ok {
		return nil, false
	}
	return ir.ConstStmt{Name: name, Expr: expr, Span: a.irSpan(s.Span)}, true
}

// lowerReturn は `return 式;`。WIRE-3（void の op に return、最後以外の行の return）、WIRE-2（値がない）、TYPE-3（returns に渡せない）。
func (a *analyzer) lowerReturn(w *wiring, s *ast.ReturnStmt, last bool) (ir.Statement, bool) {
	op := w.job.op
	if op.Returns == nil {
		a.errorf("WIRE-3", s.Span, "op %s は値を返さないので return を書けない（設計書 §9）", op.Name)
	} else if !last {
		a.errorf("WIRE-3", s.Span, "return は本体の最後の文にだけ書ける（下の行が評価されないため）")
	}
	t, expr, ok := a.evalExpr(w, s.Value)
	if !ok {
		return nil, false
	}
	if t.kind == etNoValue {
		a.errorf("WIRE-2", s.Value.NodeSpan(), "return の式は値を持たない（設計書 §9）")
	} else if op.Returns != nil && a.assign(t, op.Returns) != assignOK {
		a.errorf("TYPE-3", s.Value.NodeSpan(), "return の値の型 %s は op %s の戻り値の型 %s に渡せない（射影もできない）", exprName(t), op.Name, typeName(op.Returns))
	}
	return ir.ReturnStmt{Expr: expr, Span: a.irSpan(s.Span)}, true
}

// evalExpr は式の型（docs/types.md「式の型」）と IR の式。
func (a *analyzer) evalExpr(w *wiring, e ast.Expr) (exprType, ir.Expr, bool) {
	switch e := e.(type) {
	case nil:
		return unknownType(), nil, false
	case *ast.Call:
		t, call, ok := a.evalCall(w, e, nil)
		return t, ir.CallExpr{Call: call}, ok
	case *ast.Match:
		t, m := a.evalMatch(w, e)
		return t, ir.MatchExpr{Match: m}, true
	case ast.Atom:
		t, atom := a.evalAtom(w, e)
		return t, ir.AtomExpr{Atom: atom}, atom != nil
	}
	return unknownType(), nil, false
}

// armContext は match のアームの本体の呼び出しで、subject を絞り込まれた型として渡すための文脈（MATCH-4）。
type armContext struct {
	subject  []string // subject の Atom.name のパス
	narrowed exprType
	cases    string
}

// evalCall は呼び出し。op を解決し、位置の引数を引数名に対応付け（docs/ir.md「正規化」）、型を照合する（TYPE-4、MATCH-4）。
// Bound は nil のまま（docs/check.md「bind」が埋める）。
func (a *analyzer) evalCall(w *wiring, c *ast.Call, arm *armContext) (exprType, ir.Call, bool) {
	call := ir.Call{Span: a.irSpan(c.Span)}
	args := a.evalArgs(w, c.Args)
	r, ok := a.resolve(c.Callee, w.sc, acOp)
	if !ok {
		return unknownType(), call, false
	}
	op, ok := a.lower(r.sym).(*ir.Op)
	if !ok {
		return unknownType(), call, false
	}
	call.Op = op.Name
	for i, arg := range args {
		if i >= len(op.Params) {
			a.errorf("TYPE-4", arg.span, "op %s の引数は %d 個で、渡した引数が多い", op.Name, len(op.Params))
			break
		}
		p := op.Params[i]
		call.Args = append(call.Args, ir.CallArg{Param: p.Name, Value: arg.atom})
		a.checkArg(w, op, p, arg, arm)
	}
	if op.Returns == nil {
		return noValue(), call, true
	}
	return valueType(op.Returns), call, true
}

// evaluatedArg は評価した引数。
type evaluatedArg struct {
	typ  exprType
	atom ir.Atom
	span source.Span
	path []string // Atom.name のパス（subject の照合に使う）
}

func (a *analyzer) evalArgs(w *wiring, atoms []ast.Atom) []evaluatedArg {
	out := make([]evaluatedArg, 0, len(atoms))
	for _, at := range atoms {
		if at == nil {
			continue
		}
		t, atom := a.evalAtom(w, at)
		arg := evaluatedArg{typ: t, atom: atom, span: at.NodeSpan()}
		if n, ok := at.(*ast.NameAtom); ok {
			arg.path = segmentsOf(n.Name)
		}
		out = append(out, arg)
	}
	return out
}

// checkArg は引数の型の照合。アームの中で subject をそのまま渡していれば絞り込まれた型で照合し、違反は MATCH-4。
func (a *analyzer) checkArg(w *wiring, op *ir.Op, p ir.OpParam, arg evaluatedArg, arm *armContext) {
	if p.Type == nil {
		return
	}
	if arm != nil && arg.path != nil && strings.Join(arg.path, ".") == strings.Join(arm.subject, ".") {
		if a.assign(arm.narrowed, p.Type) != assignOK {
			a.errorf("MATCH-4", arg.span, "アーム %s の中で %s は %s に絞り込まれているが、op %s の引数 %s の型 %s に渡せない", arm.cases, strings.Join(arg.path, "."), exprName(arm.narrowed), op.Name, p.Name, typeName(p.Type))
		}
		return
	}
	if fail := a.assign(arg.typ, p.Type); fail != assignOK {
		a.errorf(string(fail), arg.span, "%s", failMessage(fail, fmt.Sprintf("op %s の引数 %s", op.Name, p.Name), arg.typ, p.Type))
	}
}

// evalAtom は Atom の型と IR。名前は束縛・引数・そのフィールド（docs/resolve.md「手続き」1.1 と 2）。
func (a *analyzer) evalAtom(w *wiring, at ast.Atom) (exprType, ir.Atom) {
	switch at := at.(type) {
	case *ast.NameAtom:
		r, ok := a.resolve(at.Name, w.sc, acValue)
		atom := ir.NameAtom{Path: segmentsOf(at.Name), Span: a.irSpan(at.Span)}
		if !ok {
			return unknownType(), atom
		}
		return r.value.typ, atom
	case *ast.Literal:
		return literalType(at.Kind), ir.LiteralAtom{Literal: a.lowerLiteral(at), Span: a.irSpan(at.Span)}
	case *ast.NoneAtom:
		return noneType(), ir.NoneAtom{Span: a.irSpan(at.Span)}
	case *ast.RecordLit:
		return a.evalRecord(w, at)
	}
	return unknownType(), nil
}

// evalRecord はレコードリテラル。`{ item }` は `{ item: item }` に展開する（docs/ir.md「正規化」）。
// 型は当てはめる先が決まるまで未定（etRecord）。
func (a *analyzer) evalRecord(w *wiring, r *ast.RecordLit) (exprType, ir.Atom) {
	rv := &recordVal{span: r.Span}
	atom := ir.RecordAtom{Span: a.irSpan(r.Span)}
	for _, f := range r.Fields {
		if f == nil {
			continue
		}
		value := f.Value
		if value == nil {
			value = &ast.NameAtom{Span: f.Name.Span, Name: ast.QualifiedName{Span: f.Name.Span, Segments: []ast.Ident{f.Name}}}
		}
		t, va := a.evalAtom(w, value)
		rv.fields = append(rv.fields, recordFieldVal{name: f.Name.Name, span: f.Name.Span, valueSpan: value.NodeSpan(), typ: t})
		atom.Fields = append(atom.Fields, ir.RecordField{Name: f.Name.Name, Value: va})
	}
	return exprType{kind: etRecord, rec: rv}, atom
}

func segmentsOf(qn ast.QualifiedName) []string {
	out := make([]string, len(qn.Segments))
	for i, s := range qn.Segments {
		out[i] = s.Name
	}
	return out
}

// matchClass は match が使う分類: classify の宣言か、基底の状態の組み込みの分類（docs/check.md「状態の形」）。
type matchClass struct {
	// name は IR の Match.classify: classify の FQN か、組み込みの分類なら状態の形（"list" / "nullable"）。
	name string
	// classify は classify の宣言。組み込みの分類なら nil。
	classify   *ir.Classify
	cases      []string
	hasDefault bool
}

// label は診断に出す分類の呼び名。後ろに「のケース」と続ける形。
func (c *matchClass) label() string {
	if c.classify == nil {
		return c.name + " の組み込みの分類"
	}
	return "classify " + c.name + " "
}

func classOf(c *ir.Classify) *matchClass {
	mc := &matchClass{name: c.Name, classify: c, hasDefault: c.HasDefault}
	for _, cs := range c.Cases {
		mc.cases = append(mc.cases, cs.Name)
	}
	return mc
}

// builtinClass は形 shape の組み込みの分類。ケースは Empty / HasItems で、default はない。
func builtinClass(shape string) *matchClass {
	return &matchClass{name: shape, cases: ir.BuiltinCases()}
}

// evalMatch は match。形は三つ（docs/grammar.md「配線」の表。合わなければ MATCH-7）: 値の分類での match、
// 基底の状態の組み込みの分類での match（by list / by nullable）、状態の分類での match（対象を書かない）。
// MATCH-2 はどの形にも、MATCH-4 / MATCH-6 と絞り込みは値の match にだけ掛かる。アームの型の集め方は
// docs/types.md「式の型」（値でも状態でも同じ）。
func (a *analyzer) evalMatch(w *wiring, m *ast.Match) (exprType, ir.Match) {
	out := ir.Match{Span: a.irSpan(m.Span)}
	var class *matchClass
	var value *matchValue
	if m.Builtin {
		class = builtinClass(m.Classifier.Name)
		out.Classify = class.name
		if subject, ok := a.matchStateSubject(w, m); ok {
			out.Subject = subject
		}
	} else {
		c := a.resolveClassifier(w, m)
		if c != nil {
			class = classOf(c)
			out.Classify = c.Name
		}
		out.Subject, value = a.matchSubject(w, m, c)
	}
	if class != nil {
		a.checkMATCH2(class, m)
	}
	var types []exprType
	for _, arm := range m.Arms {
		if arm == nil {
			continue
		}
		t, ia := a.evalArm(w, arm, class, value)
		types = append(types, t)
		out.Arms = append(out.Arms, ia)
	}
	return a.matchType(types, m), out
}

// matchValue は値の match の対象。アームの本体で絞り込まれた型として渡すために持つ（MATCH-4）。
type matchValue struct {
	path []string // 対象が Atom.name ならそのパス
	typ  exprType
}

// matchSubject は `match [x] by C` の対象（C は classify。解決できていなければ nil）。MATCH-7 の
// 「値の分類なのに対象がない」「状態の分類なのに対象がある」と、値の match の MATCH-6。
func (a *analyzer) matchSubject(w *wiring, m *ast.Match, c *ir.Classify) (ir.MatchSubject, *matchValue) {
	if c != nil {
		if _, isStates := c.SubjectStates(); isStates {
			if m.Subject != nil {
				if n, ok := m.Subject.(*ast.NameAtom); ok {
					a.peek(n.Name, w.sc) // 束縛を書いていれば使ったことにする（WIRE-8 を重ねない）
				}
				a.errorf("MATCH-7", m.Subject.NodeSpan(), "classify %s は状態の分類なので、match に対象は書けない（対象は分類の宣言が持つ。match by %s と書く）", c.Name, m.Classifier.Name)
			}
			return ir.ClassifyOwnedSubject{}, nil
		}
	}
	if m.Subject == nil {
		if c != nil && c.SubjectType() != nil {
			a.errorf("MATCH-7", m.Classifier.Span, "classify %s は値の分類なので、match に対象が要る（match 値 by %s と書く）", c.Name, m.Classifier.Name)
		}
		return nil, nil
	}
	typ, atom := a.evalAtom(w, m.Subject)
	value := &matchValue{typ: typ}
	if n, ok := m.Subject.(*ast.NameAtom); ok {
		value.path = segmentsOf(n.Name)
	}
	if c != nil {
		a.checkMATCH6(typ, c, m)
	}
	if atom == nil {
		return nil, value
	}
	return ir.ValueSubject{Atom: atom}, value
}

// matchStateSubject は `match s by list` / `by nullable` の s: 現在の名前空間の、その形の state
// （docs/resolve.md「位置ごとに受け付ける種類」）。そうでなければ MATCH-7。名前は手続きの通りに解決するので、
// 同じ名前の束縛や引数があれば値の参照になり、状態ではないので MATCH-7。
func (a *analyzer) matchStateSubject(w *wiring, m *ast.Match) (ir.StateSubject, bool) {
	shape := m.Classifier.Name
	if m.Subject == nil {
		a.errorf("MATCH-7", m.Classifier.Span, "match by %s には対象の状態が要る（match 状態 by %s と書く）", shape, shape)
		return ir.StateSubject{}, false
	}
	n, ok := m.Subject.(*ast.NameAtom)
	if !ok {
		a.errorf("MATCH-7", m.Subject.NodeSpan(), "match by %s の対象は、今の名前空間の %s の状態でなければならない", shape, shape)
		return ir.StateSubject{}, false
	}
	r, err := a.resolveName(n.Name, w.sc, acAnyDecl|acCase|acVariant|acValue|acNamespace|acStateOp)
	if err != nil {
		a.reportResolve(err)
		return ir.StateSubject{}, false
	}
	if r.kind != rDecl || r.sym.kind != kindState {
		a.errorf("MATCH-7", n.Span, "match by %s の対象は、今の名前空間の %s の状態でなければならないが、%s は状態ではない（%s）", shape, shape, n.Name.String(), describe(r))
		return ir.StateSubject{}, false
	}
	if r.sym.ns != w.sc.ns {
		a.errorf("MATCH-7", n.Span, "match by %s の対象 %s は別の名前空間の状態（分岐できるのは今の名前空間 %s の状態だけ）", shape, r.sym.fqn, nsName(w.sc.ns))
		return ir.StateSubject{}, false
	}
	st, ok := a.lower(r.sym).(*ir.State)
	if !ok {
		return ir.StateSubject{}, false
	}
	if string(st.Shape) != shape {
		a.errorf("MATCH-7", n.Span, "状態 %s の形は %s で、%s ではない", st.Name, st.Shape, shape)
		return ir.StateSubject{}, false
	}
	return ir.StateSubject{State: st.Name}, true
}

// resolveClassifier は `match x by C` / `match by C` の C: 現在の名前空間の classify（NS-4）。
func (a *analyzer) resolveClassifier(w *wiring, m *ast.Match) *ir.Classify {
	qn := ast.QualifiedName{Span: m.Classifier.Span, Segments: []ast.Ident{m.Classifier}}
	r, ok := a.resolve(qn, &scope{ns: w.sc.ns}, acClassify)
	if !ok {
		return nil
	}
	a.checkNS4(r.sym, m.Classifier.Span, w.sc)
	c, _ := a.lower(r.sym).(*ir.Classify)
	return c
}

// checkMATCH6 は MATCH-6（docs/check.md「classify / match」）: 値の match で、subject の型が Classify.subject の型に渡せない。
func (a *analyzer) checkMATCH6(subject exprType, c *ir.Classify, m *ast.Match) {
	want := c.SubjectType()
	if want == nil || subject.kind == etUnknown {
		return
	}
	if a.assign(subject, want) != assignOK {
		a.errorf("MATCH-6", m.Subject.NodeSpan(), "match の対象の型 %s は classify %s の対象の型 %s に渡せない", exprName(subject), c.Name, typeName(want))
	}
}

// checkMATCH2 は MATCH-2（docs/check.md「classify / match」）: アームに並べたケースの集合が分類のケース（組み込みの
// 分類なら Empty / HasItems で、default はない）と等しくない（足りない・知らない名前・重複）。default は hasDefault の
// ときだけ書け、あるときは必ず書く。Data.missing / Data.extra。
func (a *analyzer) checkMATCH2(c *matchClass, m *ast.Match) {
	seen := map[string]bool{}
	var extra []string
	defaults := 0
	for _, arm := range m.Arms {
		if arm == nil {
			continue
		}
		if arm.Default {
			defaults++
			continue
		}
		for _, cs := range arm.Cases {
			switch {
			case !contains(c.cases, cs.Name):
				extra = append(extra, cs.Name)
			case seen[cs.Name]:
				extra = append(extra, cs.Name)
			default:
				seen[cs.Name] = true
			}
		}
	}
	var missing []string
	for _, name := range c.cases {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	var problems []string
	if len(missing) > 0 {
		problems = append(problems, "足りないケース: "+join(missing))
	}
	if len(extra) > 0 {
		problems = append(problems, "知らないか重複したケース: "+join(extra))
	}
	if c.hasDefault && defaults == 0 {
		problems = append(problems, "default のアームがない")
	}
	if !c.hasDefault && defaults > 0 {
		if c.classify == nil {
			problems = append(problems, "組み込みの分類に default はないのに default のアームがある")
		} else {
			problems = append(problems, "classify に default がないのに default のアームがある")
		}
	}
	if defaults > 1 {
		problems = append(problems, "default のアームが二つ以上ある")
	}
	if len(problems) == 0 {
		return
	}
	d := a.errorf("MATCH-2", m.Classifier.Span, "match のアームが %sのケース（%s）と合わない（%s）", c.label(), strings.Join(c.cases, " / "), strings.Join(problems, "。"))
	d.Data = map[string]any{"missing": orEmpty(missing), "extra": orEmpty(extra)}
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// evalArm はアーム。本体は呼び出しか none。値の match（value が nil でない）の呼び出しでは、subject を絞り込まれた型
// case(C, アームのケース, T) として渡す（MATCH-4）。状態の match には絞り込みがない（docs/types.md「式の型」）。
func (a *analyzer) evalArm(w *wiring, arm *ast.MatchArm, class *matchClass, value *matchValue) (exprType, ir.MatchArm) {
	out := ir.MatchArm{Span: a.irSpan(arm.Span)}
	var ctx *armContext
	if arm.Default {
		out.Cases = ir.DefaultArm()
		if value != nil {
			ctx = &armContext{subject: value.path, narrowed: value.typ, cases: "default"}
		}
	} else {
		cases := make([]string, 0, len(arm.Cases))
		for _, cs := range arm.Cases {
			cases = append(cases, cs.Name)
			if class != nil && class.classify != nil && contains(class.cases, cs.Name) {
				a.addRef(cs.Span, Target{Kind: TargetCase, Decl: class.name, Name: cs.Name, Span: a.caseSpan(class.classify, cs.Name)})
			}
		}
		out.Cases = ir.ArmCases{Cases: cases}
		if value != nil {
			var c *ir.Classify
			if class != nil {
				c = class.classify
			}
			ctx = &armContext{subject: value.path, narrowed: a.narrow(value.typ, c, cases), cases: strings.Join(cases, " | ")}
		}
	}
	switch body := arm.Body.(type) {
	case *ast.NoneAtom:
		out.Body = ir.NoneBody{}
		return noneType(), out
	case *ast.Call:
		t, call, _ := a.evalCall(w, body, ctx)
		out.Body = ir.CallExpr{Call: call}
		return t, out
	}
	return unknownType(), out
}

// narrow は subject の型を case(C, cases, T) に絞り込む。T は subject の型（既に絞り込まれていればその元）。
func (a *analyzer) narrow(subject exprType, c *ir.Classify, cases []string) exprType {
	if c == nil || subject.kind != etValue {
		return subject
	}
	of := subject.typ
	if ct, ok := of.(ir.CaseType); ok {
		of = ct.Of
	}
	return valueType(ir.Case(c.Name, cases, of))
}

// caseSpan は classify のケース名の定義の位置。
func (a *analyzer) caseSpan(c *ir.Classify, name string) source.Span {
	if s := a.syms[c.Name]; s != nil {
		if span, ok := classifyCaseSpan(s, name); ok {
			return span
		}
	}
	return source.Span{}
}

// matchType は docs/types.md「式の型」の match: 値のある呼び出しがなければ「値がない」。すべて等しい型 T なら、
// none のアームがあれば optional(T)、なければ T。等しくなければ TYPE-8。「値がない」と値が混ざれば TYPE-8。
func (a *analyzer) matchType(types []exprType, m *ast.Match) exprType {
	var value *exprType
	hasNone, hasNoValue, unknown := false, false, false
	for i := range types {
		t := types[i]
		switch t.kind {
		case etNone:
			hasNone = true
		case etNoValue:
			hasNoValue = true
		case etUnknown:
			unknown = true
		case etValue:
			if value == nil {
				value = &t
			} else if !ir.TypeEqual(value.typ, t.typ) {
				a.errorf("TYPE-8", m.Span, "match のアームの型がそろわない: %s と %s", typeName(value.typ), typeName(t.typ))
				return unknownType()
			}
		}
	}
	if value == nil {
		if unknown {
			return unknownType()
		}
		return noValue()
	}
	if hasNoValue {
		a.errorf("TYPE-8", m.Span, "match に値のある呼び出しと値のない呼び出しが混ざっている")
		return unknownType()
	}
	if hasNone {
		return valueType(ir.Optional(value.typ))
	}
	return *value
}
