package sema

import (
	"fmt"
	"strconv"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// pendingBind は表ができてから解決する bind。
type pendingBind struct {
	ns   *namespace
	decl *ast.BindDecl
}

// lowerAll は表のすべての宣言を出現順に IR に下ろし、bind を解決する。
func (a *analyzer) lowerAll() {
	for _, ns := range a.nsOrder {
		for _, s := range ns.order {
			a.lower(s)
		}
	}
	for _, b := range a.pendingBinds {
		a.lowerBind(b)
	}
}

// lower は宣言を IR に下ろす。済んでいればその結果を返す。下ろしている途中（循環）なら nil を返し、
// 呼び手がそれぞれの規則（TYPE-9 / UNION-1 / TYPE-10）で報告する。
func (a *analyzer) lower(s *symbol) ir.Decl {
	switch s.state {
	case lowered:
		return s.decl
	case lowering:
		return nil
	}
	s.state = lowering
	sc := &scope{ns: s.ns}
	switch d := s.node.(type) {
	case *ast.ModelDecl:
		s.decl = a.lowerModel(s, d, sc)
	case *ast.ScalarDecl:
		s.decl = a.lowerScalar(s, d, sc)
	case *ast.UnionDecl:
		s.decl = a.lowerUnion(s, d, sc)
	case *ast.StateDecl:
		s.decl = a.lowerState(s, d, sc)
	case *ast.ExceptionDecl:
		s.decl = a.lowerException(s, d, sc)
	case *ast.EffectDecl:
		s.decl = a.lowerEffect(s, d, sc)
	case *ast.ClassifyDecl:
		s.decl = a.lowerClassify(s, d, sc)
	case *ast.ConditionDecl:
		s.decl = a.lowerCondition(s, d, sc)
	case *ast.DecoratorDecl:
		s.decl = a.lowerDecoratorDecl(s, d, sc)
	case *ast.OpDecl:
		s.decl = a.lowerOp(s, d, sc)
	}
	s.state = lowered
	return s.decl
}

// base は DeclBase を作る。Decorators は lowerApplications が埋める。
func (a *analyzer) base(s *symbol, doc *ast.DocComment, span source.Span, dctx docContext) ir.DeclBase {
	b := ir.DeclBase{Name: s.fqn, Span: a.irSpan(span)}
	if doc != nil {
		dctx.ns = s.ns
		dctx.span = s.nameSpan
		b.Description = a.lowerDoc(doc, dctx)
	}
	return b
}

// --- model ---

// lowerModel は model（docs/ir.md「model / scalar / union」）。`is X omit` は finalizeDerived で展開する。
func (a *analyzer) lowerModel(s *symbol, d *ast.ModelDecl, sc *scope) ir.Decl {
	m := &ir.Model{DeclBase: a.base(s, d.Doc, d.Span, docContext{})}
	a.models[s.fqn] = m
	a.fields[s.fqn] = map[string]source.Span{}
	a.addSite(d.Decorators, siteModel, sc, &m.DeclBase.Decorators)
	if d.Body != nil {
		m.Fields = a.lowerFields(d.Body.Fields, s.fqn, sc)
	} else if d.Base != nil {
		a.derived = append(a.derived, s)
	}
	return m
}

// lowerFields はフィールドの列。owner は持ち主の model の FQN。
func (a *analyzer) lowerFields(fields []*ast.Field, owner string, sc *scope) []ir.Field {
	out := make([]ir.Field, 0, len(fields))
	for _, f := range fields {
		if f == nil {
			continue
		}
		fld := ir.Field{Name: f.Name.Name, Span: a.irSpan(f.Span)}
		if f.Doc != nil {
			fld.Description = a.lowerDoc(f.Doc, docContext{ns: sc.ns, span: f.Name.Span})
		}
		if t, ok := a.lowerType(f.Type, sc); ok {
			fld.Type = t
		}
		if f.Default != nil {
			fld.Default = a.lowerLiteral(f.Default)
			a.checkDefault(f, fld.Type)
		}
		a.fields[owner][f.Name.Name] = f.Name.Span
		out = append(out, fld)
	}
	// デコレータの適用はフィールドの列が確定してから（スライスの要素を指すため）
	for i, f := range fields {
		if f != nil && i < len(out) {
			a.addSite(f.Decorators, siteField, sc, &out[i].Decorators)
		}
	}
	return out
}

// checkDefault は TYPE-13（docs/types.md「既定値」）: 既定値のリテラルは、フィールドの型が scalar への ref ならその元の型に、
// そうでなければフィールドの型に渡せること。
func (a *analyzer) checkDefault(f *ast.Field, t ir.Type) {
	if t == nil {
		return
	}
	target := a.scalarBase(stripOptional(t))
	if target == nil {
		return
	}
	if a.assign(literalType(f.Default.Kind), target) != assignOK {
		a.errorf("TYPE-13", f.Default.Span, "フィールド %s の既定値 %s は型 %s に渡せない", f.Name.Name, f.Default.Raw, typeName(t))
	}
}

// checkLiteralRange は LIT-1（docs/types.md「診断」）: 整数リテラルと Duration（ナノ秒）が 64 ビット符号付きの範囲を超える。
// 構文解析が先に SYNTAX として落とすので普通は届かないが、Raw から確かめる。
func (a *analyzer) checkLiteralRange(l *ast.Literal) {
	if l.Kind != ast.IntLit || l.Raw == "" {
		return
	}
	if _, err := strconv.ParseInt(l.Raw, 10, 64); err != nil {
		a.errorf("LIT-1", l.Span, "整数リテラル %s は 64 ビット符号付きの範囲を超える", l.Raw)
	}
}

// finalizeDerived は `model P is U omit a, b;` を展開する（docs/ir.md「正規化」）。U のフィールドのデコレータが
// 下りてから複製するため、lowerApplications の後に行う。TYPE-10 と循環。
func (a *analyzer) finalizeDerived() {
	for _, s := range a.derived {
		a.deriveModel(s, map[string]bool{})
	}
}

func (a *analyzer) deriveModel(s *symbol, visiting map[string]bool) {
	if s.derivedDone {
		return
	}
	d := s.node.(*ast.ModelDecl)
	m := s.decl.(*ir.Model)
	s.derivedDone = true
	r, ok := a.resolve(*d.Base, &scope{ns: s.ns}, acModel)
	if !ok {
		return
	}
	if visiting[r.sym.fqn] || r.sym == s {
		a.errorf("TYPE-10", d.Base.Span, "model %s の is の元 %s が循環している", s.fqn, r.sym.fqn)
		return
	}
	visiting[s.fqn] = true
	base := a.models[r.sym.fqn]
	if bd, ok := r.sym.node.(*ast.ModelDecl); ok && bd.Base != nil {
		a.deriveModel(r.sym, visiting)
		if base != nil && base.DerivedFrom == nil {
			return // 元の展開に失敗した（循環など）。重ねて報告しない
		}
	}
	if base == nil {
		return
	}
	omit := a.checkOmit(d, base)
	m.DerivedFrom = &ir.DerivedFrom{Base: base.Name, Omit: omit}
	for _, f := range base.Fields {
		if !contains(omit, f.Name) {
			m.Fields = append(m.Fields, f)
			a.fields[s.fqn][f.Name] = a.fields[base.Name][f.Name]
		}
	}
}

// checkOmit は TYPE-10（docs/types.md「診断」）: omit の名前が元の model にない。
func (a *analyzer) checkOmit(d *ast.ModelDecl, base *ir.Model) []string {
	omit := make([]string, 0, len(d.Omit))
	for _, o := range d.Omit {
		if findField(base, o.Name) == nil {
			a.errorf("TYPE-10", o.Span, "model %s にフィールド %s はないので omit できない", base.Name, o.Name)
			continue
		}
		omit = append(omit, o.Name)
	}
	return omit
}

// --- scalar ---

// lowerScalar は scalar。TYPE-9（docs/types.md「診断」）: extends の先が核の型でも scalar でもない、または継承が循環している。
func (a *analyzer) lowerScalar(s *symbol, d *ast.ScalarDecl, sc *scope) ir.Decl {
	sl := &ir.Scalar{DeclBase: a.base(s, d.Doc, d.Span, docContext{})}
	a.addSite(d.Decorators, siteScalar, sc, &sl.DeclBase.Decorators)
	switch t := d.Base.(type) {
	case *ast.CoreType:
		sl.Base = ir.CoreType{Name: ir.CoreName(t.Name)}
	case *ast.NamedType:
		if r, ok := a.resolve(t.Name, sc, acScalar); ok {
			if t.Arg != nil {
				a.errorf("TYPE-9", t.Span, "scalar %s の extends の先に型引数は書けない", s.fqn)
			} else if a.lower(r.sym) == nil && r.sym.state == lowering {
				a.errorf("TYPE-9", t.Span, "scalar %s の継承が循環している（%s）", s.fqn, r.sym.fqn)
			} else {
				sl.Base = ir.Ref(r.sym.fqn)
			}
		}
	case nil:
	default:
		a.errorf("TYPE-9", d.Base.NodeSpan(), "scalar %s の extends の先は核の型か scalar でなければならない", s.fqn)
	}
	return sl
}

// --- union ---

// lowerUnion は union。アームの列なら arms、参照の列なら set（メンバーのユニオンは展開）。UNION-1 は混在と重複。
func (a *analyzer) lowerUnion(s *symbol, d *ast.UnionDecl, sc *scope) ir.Decl {
	u := &ir.Union{DeclBase: a.base(s, d.Doc, d.Span, docContext{}), Variant: s.unionVariant}
	a.addSite(d.Decorators, siteUnion, sc, &u.DeclBase.Decorators)
	a.checkUnionMixed(s, d)
	if u.Variant == ir.UnionArms {
		u.Arms = a.lowerArms(s, d, sc)
	} else {
		u.Members = a.lowerMembers(s, d, sc)
	}
	return u
}

// checkUnionMixed は UNION-1（docs/check.md「名前空間」）: アーム（A: T）と参照（X;）が混ざる、アーム名が重複。
func (a *analyzer) checkUnionMixed(s *symbol, d *ast.UnionDecl) {
	arms, refs := 0, 0
	seen := map[string]bool{}
	for _, m := range d.Members {
		if m.Arm != nil {
			arms++
			if seen[m.Arm.Name] {
				a.errorf("UNION-1", m.Arm.Span, "union %s にアーム %s が二度ある", s.fqn, m.Arm.Name)
			}
			seen[m.Arm.Name] = true
		} else if m.Target != nil {
			refs++
		}
	}
	if arms > 0 && refs > 0 {
		a.errorf("UNION-1", s.nameSpan, "union %s にアーム（A: T）と参照（X;）が混ざっている", s.fqn)
	}
}

// lowerArms はアームの列。その場の model は `<ユニオンの FQN>.<アーム>` に合成する（docs/ir.md「正規化」）。
func (a *analyzer) lowerArms(s *symbol, d *ast.UnionDecl, sc *scope) []ir.UnionArm {
	var arms []ir.UnionArm
	for _, m := range d.Members {
		if m.Arm == nil || m.Type == nil {
			continue
		}
		arm := ir.UnionArm{Name: m.Arm.Name, Span: a.irSpan(m.Span)}
		if m.Doc != nil {
			arm.Description = a.lowerDoc(m.Doc, docContext{ns: sc.ns, span: m.Arm.Span})
		}
		if m.Type.Body != nil {
			arm.Type = ir.Ref(a.synthesize(s, m.Arm.Name, m.Type.Body, sc))
		} else if t, ok := a.lowerType(m.Type.Type, sc); ok {
			arm.Type = t
		}
		arms = append(arms, arm)
	}
	return arms
}

// synthesize はその場で書いた model を `<持ち主の FQN>.<名前>` の Model として合成し、持ち主の直後に置く。FQN を返す。
func (a *analyzer) synthesize(owner *symbol, name string, body *ast.ModelBody, sc *scope) string {
	fqn := owner.fqn + "." + name
	m := &ir.Model{
		DeclBase:        ir.DeclBase{Name: fqn, Span: a.irSpan(body.Span)},
		SynthesizedFrom: owner.fqn,
	}
	a.models[fqn] = m
	a.fields[fqn] = map[string]source.Span{}
	m.Fields = a.lowerFields(body.Fields, fqn, sc)
	owner.synthesized = append(owner.synthesized, m)
	return fqn
}

// lowerMembers は set のメンバー。ユニオンは展開して平らにする。例外と効果が混ざれば UNION-1。
func (a *analyzer) lowerMembers(s *symbol, d *ast.UnionDecl, sc *scope) []ir.UnionMember {
	var members []ir.UnionMember
	kinds := map[declKind]bool{}
	for _, m := range d.Members {
		if m.Target == nil {
			continue
		}
		r, ok := a.resolve(*m.Target, sc, acMemberPos)
		if !ok {
			continue
		}
		var desc *ir.Doc
		if m.Doc != nil {
			desc = a.lowerDoc(m.Doc, docContext{ns: sc.ns, span: m.Target.Span})
		}
		for _, t := range a.expandMember(s, r, m.Target.Span) {
			kinds[t.kind] = true
			members = append(members, ir.UnionMember{Target: t.fqn, Description: desc, Span: a.irSpan(m.Span)})
		}
	}
	if kinds[kindException] && kinds[kindEffect] {
		a.errorf("UNION-1", s.nameSpan, "union %s に例外と効果が混ざっている", s.fqn)
	}
	return members
}

// member は展開したメンバー。
type member struct {
	fqn  string
	kind declKind
}

// expandMember は解決した参照を例外か効果の列にする。ユニオンなら下ろして（lower）そのメンバーを使う。
func (a *analyzer) expandMember(owner *symbol, r resolved, span source.Span) []member {
	if r.sym.kind != kindUnion {
		return []member{{fqn: r.sym.fqn, kind: r.sym.kind}}
	}
	u, ok := a.lower(r.sym).(*ir.Union)
	if !ok {
		if r.sym.state == lowering {
			a.errorf("UNION-1", span, "union %s のメンバーの参照が循環している（%s）", owner.fqn, r.sym.fqn)
		}
		return nil
	}
	var out []member
	for _, m := range u.Members {
		if ms := a.syms[m.Target]; ms != nil {
			out = append(out, member{fqn: m.Target, kind: ms.kind})
		}
	}
	return out
}

// expandSet は throws / effects の位置の参照を FQN の列にする。want は許す種類（例外か効果）。
// 違う種類のメンバーを持つユニオンは RES-4。
func (a *analyzer) expandSet(r resolved, want declKind, span source.Span) []string {
	if r.kind != rDecl {
		return nil
	}
	var out []string
	for _, m := range a.expandMember(r.sym, r, span) {
		if m.kind != want {
			a.errorf("RES-4", span, "union %s のメンバー %s は %s ではない", r.sym.fqn, m.fqn, want)
			continue
		}
		out = append(out, m.fqn)
	}
	return out
}

// --- state / exception / effect ---

func (a *analyzer) lowerState(s *symbol, d *ast.StateDecl, sc *scope) ir.Decl {
	st := &ir.State{DeclBase: a.base(s, d.Doc, d.Span, docContext{}), Shape: ir.StateList}
	if d.Shape == ast.StateSingle {
		st.Shape = ir.StateSingle
	}
	a.addSite(d.Decorators, siteState, sc, &st.DeclBase.Decorators)
	if t, ok := a.lowerType(d.Elem, sc); ok {
		st.Element = t
	}
	return st
}

// lowerException は exception。値の型は model だけ（docs/resolve.md「位置ごとに受け付ける種類」）。
// その場の model は `<例外の FQN>.Value` に合成する。
func (a *analyzer) lowerException(s *symbol, d *ast.ExceptionDecl, sc *scope) ir.Decl {
	e := &ir.Exception{DeclBase: a.base(s, d.Doc, d.Span, docContext{})}
	a.addSite(d.Decorators, siteException, sc, &e.DeclBase.Decorators)
	if d.Value == nil {
		return e
	}
	switch {
	case d.Value.Body != nil:
		e.Value = ir.Ref(a.synthesize(s, "Value", d.Value.Body, sc))
	case d.Value.Type != nil:
		if nt, ok := d.Value.Type.(*ast.NamedType); ok && nt.Arg == nil {
			if r, ok := a.resolve(nt.Name, sc, acModel); ok {
				e.Value = ir.Ref(r.sym.fqn)
			}
		} else {
			a.errorf("RES-4", d.Value.Span, "exception %s の値の型は model でなければならない", s.fqn)
		}
	}
	return e
}

// lowerEffect は effect。`on s.op` の s は現在の名前空間の state（別の名前空間なら RES-5）。STATE-1。
func (a *analyzer) lowerEffect(s *symbol, d *ast.EffectDecl, sc *scope) ir.Decl {
	e := &ir.Effect{DeclBase: a.base(s, d.Doc, d.Span, docContext{}), Operation: operationOf(d.On.Op)}
	a.addSite(d.Decorators, siteEffect, sc, &e.DeclBase.Decorators)
	r, ok := a.resolve(d.On.State, sc, acState)
	if !ok {
		return e
	}
	if r.sym.ns != s.ns {
		a.errorf("RES-5", d.On.State.Span, "effect %s の on は別の名前空間の状態 %s を指している（効果は状態の持ち主の名前空間に宣言する。設計書 §6.2）", s.fqn, r.sym.fqn)
		return e
	}
	e.State = r.sym.fqn
	a.checkSTATE1(s, r.sym, d)
	return e
}

// checkSTATE1 は STATE-1（docs/check.md「状態と効果」）: single の状態に post か delete。
func (a *analyzer) checkSTATE1(s *symbol, state *symbol, d *ast.EffectDecl) {
	st, ok := a.lower(state).(*ir.State)
	if !ok || st.Shape != ir.StateSingle {
		return
	}
	if d.On.Op == ast.Post || d.On.Op == ast.Delete {
		a.errorf("STATE-1", d.On.OpSpan, "状態 %s は single なので %s はできない（single にできるのは put だけ。設計書 §5.2）", state.fqn, d.On.Op)
	}
}

func operationOf(op ast.StateOp) ir.Operation {
	switch op {
	case ast.Post:
		return ir.OpPost
	case ast.Put:
		return ir.OpPut
	}
	return ir.OpDelete
}

// --- classify / condition ---

// lowerClassify は classify。CLS-1（default が二つ以上、ケース名の重複）、CLS-2（名前の付いたケースがない）。
func (a *analyzer) lowerClassify(s *symbol, d *ast.ClassifyDecl, sc *scope) ir.Decl {
	c := &ir.Classify{DeclBase: a.base(s, d.Doc, d.Span, docContext{})}
	a.addSite(d.Decorators, siteClassify, sc, &c.DeclBase.Decorators)
	if t, ok := a.lowerType(d.Of, sc); ok {
		c.Subject = t
	}
	seen := map[string]bool{}
	for _, cs := range d.Cases {
		if cs == nil {
			continue
		}
		if cs.Default {
			a.lowerDefaultCase(s, c, cs, sc)
			continue
		}
		if cs.Name == nil {
			continue
		}
		if seen[cs.Name.Name] {
			a.errorf("CLS-1", cs.Name.Span, "classify %s にケース %s が二度ある", s.fqn, cs.Name.Name)
			continue
		}
		seen[cs.Name.Name] = true
		a.checkCaseName(s, *cs.Name)
		cc := ir.ClassifyCase{Name: cs.Name.Name, Span: a.irSpan(cs.Span)}
		if cs.Doc != nil {
			cc.Description = a.lowerDoc(cs.Doc, docContext{ns: sc.ns, span: cs.Name.Span})
		}
		c.Cases = append(c.Cases, cc)
	}
	if len(c.Cases) == 0 {
		a.errorf("CLS-2", s.nameSpan, "classify %s に名前の付いたケースが一つもない", s.fqn)
	}
	return c
}

func (a *analyzer) lowerDefaultCase(s *symbol, c *ir.Classify, cs *ast.ClassifyCase, sc *scope) {
	if c.HasDefault {
		a.errorf("CLS-1", cs.Span, "classify %s に default が二つ以上ある", s.fqn)
		return
	}
	c.HasDefault = true
	if cs.Doc != nil {
		c.DefaultDescription = a.lowerDoc(cs.Doc, docContext{ns: sc.ns, span: cs.Span})
	}
}

// lowerCondition は condition → Classify（cases は一つ、hasDefault は true、fromCondition は true。docs/ir.md「正規化」）。
func (a *analyzer) lowerCondition(s *symbol, d *ast.ConditionDecl, sc *scope) ir.Decl {
	c := &ir.Classify{
		DeclBase:      a.base(s, d.Doc, d.Span, docContext{}),
		Cases:         []ir.ClassifyCase{{Name: d.Name.Name, Span: a.irSpan(d.Name.Span)}},
		HasDefault:    true,
		FromCondition: true,
	}
	a.addSite(d.Decorators, siteClassify, sc, &c.DeclBase.Decorators)
	if t, ok := a.lowerType(d.Of, sc); ok {
		c.Subject = t
	}
	return c
}

// --- op ---

// lowerOp は op。配線の本体は lowerWirings で下ろす。Signature は零値のまま（internal/check が埋める）。
func (a *analyzer) lowerOp(s *symbol, d *ast.OpDecl, sc *scope) ir.Decl {
	params := map[string]*opParamInfo{}
	op := &ir.Op{DeclBase: ir.DeclBase{Name: s.fqn, Span: a.irSpan(d.Span)}}
	for _, p := range d.Params {
		if p == nil {
			continue
		}
		param := ir.OpParam{Name: p.Name.Name, Span: a.irSpan(p.Span)}
		if t, ok := a.lowerType(p.Type, sc); ok {
			param.Type = t
		}
		params[p.Name.Name] = &opParamInfo{typ: param.Type, span: p.Name.Span}
		op.Params = append(op.Params, param)
	}
	if d.Doc != nil {
		op.Description = a.lowerDoc(d.Doc, docContext{ns: s.ns, span: s.nameSpan, params: paramNames(d.Params), hasParams: true})
	}
	if d.Return != nil && !d.Return.Void {
		if t, ok := a.lowerType(d.Return.Type, sc); ok {
			op.Returns = t
		}
	}
	for _, c := range d.Clauses {
		switch c := c.(type) {
		case *ast.EffectsClause:
			op.Effects = append(op.Effects, a.lowerEffectRefs(c.Effects, sc)...)
		case *ast.ThrowsClause:
			op.Throws = append(op.Throws, a.lowerThrows(c, sc))
		}
	}
	a.checkREL1(s.fqn, op.Throws, d.Span)
	a.addSite(d.Decorators, siteOp, &scope{ns: s.ns, opParams: &opParamScope{decl: s.fqn, params: params}}, &op.DeclBase.Decorators)
	if d.Body != nil {
		a.wirings = append(a.wirings, &wiringJob{sym: s, decl: d, op: op})
	}
	return op
}

func paramNames(params []*ast.OpParam) map[string]bool {
	m := map[string]bool{}
	for _, p := range params {
		if p != nil {
			m[p.Name.Name] = true
		}
	}
	return m
}

// lowerEffectRefs は effects / prevents / keeps の要素: effect か、効果の union（展開）。
func (a *analyzer) lowerEffectRefs(names []ast.QualifiedName, sc *scope) []string {
	var out []string
	for _, n := range names {
		if r, ok := a.resolve(n, sc, acEffectsPos); ok {
			out = append(out, a.expandSet(r, kindEffect, n.Span)...)
		}
	}
	return out
}

// lowerThrows は throws の一節（docs/ir.md「decorator」の ThrowsClause）。REL-5 もここで確かめる。
func (a *analyzer) lowerThrows(c *ast.ThrowsClause, sc *scope) ir.ThrowsClause {
	tc := ir.ThrowsClause{Exceptions: a.lowerExceptionRefs(c.Exceptions, sc)}
	for _, r := range c.Relations {
		if r == nil || r.Effects == nil {
			continue
		}
		rel := ir.Relation{Kind: ir.Prevents}
		if r.Kind == ast.Keeps {
			rel.Kind = ir.Keeps
		}
		if r.Effects.All {
			rel.Effects = ir.AllEffects()
		} else {
			rel.Effects = ir.RelationEffects{Effects: a.lowerEffectRefs(r.Effects.Names, sc)}
		}
		tc.Relations = append(tc.Relations, rel)
	}
	a.checkREL5(tc, c.Span)
	return tc
}

// lowerExceptionRefs は throws / catches の要素: exception、例外の union（展開）、デコレータの宣言の中なら any exception の引数。
func (a *analyzer) lowerExceptionRefs(names []ast.QualifiedName, sc *scope) []string {
	var out []string
	for _, n := range names {
		r, ok := a.resolve(n, sc, acThrowsPos)
		if !ok {
			continue
		}
		if r.kind == rDecoParam {
			if a.checkAnyParam(sc, r.name, ir.AnyException, n.Span) {
				out = append(out, "$param:"+r.name)
			}
			continue
		}
		out = append(out, a.expandSet(r, kindException, n.Span)...)
	}
	return out
}

// checkAnyParam はデコレータの引数 name が `any of` の引数かどうか。違えば RES-4。
func (a *analyzer) checkAnyParam(sc *scope, name string, of ir.AnyOf, span source.Span) bool {
	if sc.decoParams == nil {
		return false
	}
	if p, ok := sc.decoParams.types[name].(ir.AnyParam); ok && p.Of == of {
		return true
	}
	a.errorf("RES-4", span, "デコレータの引数 %s は any %s ではないので、この位置では指せない", name, of)
	return false
}

// checkREL1 は REL-1（docs/check.md「throws」）: throws の全節を通して同じ例外が二度ある。
func (a *analyzer) checkREL1(owner string, clauses []ir.ThrowsClause, span source.Span) {
	seen := map[string]bool{}
	for _, c := range clauses {
		for _, e := range c.Exceptions {
			if seen[e] {
				a.errorf("REL-1", span, "%s の throws に例外 %s が二度ある", owner, e)
			}
			seen[e] = true
		}
	}
}

// checkREL5 は REL-5（docs/check.md「throws」）: 同じ例外・同じ効果に prevents と keeps の両方がある。
// 一つの節の relations はその節の例外すべてに掛かるので、節の中で prevents と keeps の効果が重なれば違反。
func (a *analyzer) checkREL5(c ir.ThrowsClause, span source.Span) {
	var prevents, keeps []ir.RelationEffects
	for _, r := range c.Relations {
		if r.Kind == ir.Prevents {
			prevents = append(prevents, r.Effects)
		} else {
			keeps = append(keeps, r.Effects)
		}
	}
	for _, p := range prevents {
		for _, k := range keeps {
			if overlap := effectsOverlap(p, k); overlap != "" {
				a.errorf("REL-5", span, "例外 %s について効果 %s に prevents と keeps の両方がある", join(c.Exceptions), overlap)
				return
			}
		}
	}
}

// effectsOverlap は二つの目的語が重なる効果の名前（"all" どうしなら "effects"）。重ならなければ空。
func effectsOverlap(p, k ir.RelationEffects) string {
	switch {
	case p.All && k.All:
		return "effects"
	case p.All:
		if len(k.Effects) > 0 {
			return k.Effects[0]
		}
	case k.All:
		if len(p.Effects) > 0 {
			return p.Effects[0]
		}
	default:
		for _, e := range p.Effects {
			if contains(k.Effects, e) {
				return e
			}
		}
	}
	return ""
}

// --- bind ---

// lowerBind は bind（docs/ir.md「bind」）。s は現在の名前空間の state、ns は名前空間（セグメントが一つでも）。
func (a *analyzer) lowerBind(b pendingBind) {
	d := b.decl
	sc := &scope{ns: b.ns}
	state, ok := a.resolve(ast.QualifiedName{Span: d.Name.Span, Segments: []ast.Ident{d.Name}}, sc, acState)
	if ok && state.sym.ns != b.ns {
		a.errorf("RES-4", d.Name.Span, "bind の状態 %s は現在の名前空間 %s のものでなければならない", state.sym.fqn, nsName(b.ns))
		ok = false
	}
	target, tok := a.resolve(d.Target, &scope{ns: b.ns, asNamespace: true}, acNamespace)
	if !ok || !tok {
		return
	}
	a.binds = append(a.binds, ir.Bind{
		Namespace: b.ns.fqn,
		State:     state.sym.fqn,
		Target:    ir.BindTarget{Namespace: target.ns.fqn, Deep: d.Deep},
		Span:      a.irSpan(d.Span),
	})
}

// related は Related を作る。
func related(span source.Span, format string, args ...any) diag.Related {
	return diag.Related{Span: span, Message: fmt.Sprintf(format, args...)}
}
