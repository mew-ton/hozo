package sema

import (
	"fmt"
	"strconv"
	"strings"

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

// lowerState は state。形は list / nullable / single（docs/check.md「状態の形」）。STATE-3: 要素の型が optional。
func (a *analyzer) lowerState(s *symbol, d *ast.StateDecl, sc *scope) ir.Decl {
	st := &ir.State{DeclBase: a.base(s, d.Doc, d.Span, docContext{}), Shape: shapeOf(d.Shape)}
	a.addSite(d.Decorators, siteState, sc, &st.DeclBase.Decorators)
	if t, ok := a.lowerType(d.Elem, sc); ok {
		st.Element = a.checkSTATE3(s, d, t)
	}
	return st
}

func shapeOf(shape ast.StateShape) ir.StateShape {
	switch shape {
	case ast.StateSingle:
		return ir.StateSingle
	case ast.StateNullable:
		return ir.StateNullable
	}
	return ir.StateList
}

// checkSTATE3 は STATE-3（docs/check.md「状態と効果」）: 状態の要素の型が optional（single<T?> など）。
// 空になりうる状態は nullable<T> と書く。IR の element は optional にならない（docs/ir.md「state / exception / effect」）
// ので、中身の型を返す。
func (a *analyzer) checkSTATE3(s *symbol, d *ast.StateDecl, t ir.Type) ir.Type {
	opt, ok := t.(ir.OptionalType)
	if !ok {
		return t
	}
	a.errorf("STATE-3", d.Elem.NodeSpan(), "状態 %s の要素の型 %s に ? は付けられない。空になりうる状態は nullable<%s> と書く（docs/grammar.md「state」）", s.fqn, typeName(t), typeName(opt.Of))
	return opt.Of
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

// lowerEffect は effect。宛先は一つだけで、基底の状態の組み込みの効果か、分類の遷移（docs/check.md「状態の形」の
// 「効果の宛先」）。宛先が決まらなければ Target は nil のまま。
func (a *analyzer) lowerEffect(s *symbol, d *ast.EffectDecl, sc *scope) ir.Decl {
	e := &ir.Effect{DeclBase: a.base(s, d.Doc, d.Span, docContext{})}
	a.addSite(d.Decorators, siteEffect, sc, &e.DeclBase.Decorators)
	switch {
	case d.On != nil:
		a.lowerBuiltinTarget(s, e, d.On, sc)
	case d.Transition != nil:
		a.lowerTransition(s, e, d.Transition, sc)
	}
	return e
}

// lowerBuiltinTarget は `on s.op`。s は現在の名前空間の state（別の名前空間なら RES-5）。STATE-1。
// 書き込むのはその状態（docs/check.md「状態の形」の「効果が書き込む状態」）。
func (a *analyzer) lowerBuiltinTarget(s *symbol, e *ir.Effect, on *ast.StateOpRef, sc *scope) {
	r, ok := a.resolve(on.State, sc, acState)
	if !ok {
		return
	}
	if r.sym.ns != s.ns {
		a.errorf("RES-5", on.State.Span, "effect %s の on は別の名前空間の状態 %s を指している（効果は状態の持ち主の名前空間に宣言する。設計書 §6.2）", s.fqn, r.sym.fqn)
		return
	}
	e.Target = ir.BuiltinTarget{State: r.sym.fqn, Operation: operationOf(on.Op)}
	e.Writes = []string{r.sym.fqn}
	a.checkSTATE1(r.sym, on)
}

// builtinOperations は形ごとの組み込みの効果（docs/check.md「状態の形」の表）。
var builtinOperations = map[ir.StateShape][]ir.Operation{
	ir.StateList:     {ir.OpPost, ir.OpPut, ir.OpDelete},
	ir.StateNullable: {ir.OpPost, ir.OpPut, ir.OpDelete},
	ir.StateSingle:   {ir.OpPut},
}

// checkSTATE1 は STATE-1（docs/check.md「状態と効果」）: 組み込みの効果の operation が、その状態の形にない
// （single に post / delete）。
func (a *analyzer) checkSTATE1(state *symbol, on *ast.StateOpRef) {
	st, ok := a.lower(state).(*ir.State)
	if !ok {
		return
	}
	op := operationOf(on.Op)
	allowed := builtinOperations[st.Shape]
	for _, o := range allowed {
		if o == op {
			return
		}
	}
	names := make([]string, len(allowed))
	for i, o := range allowed {
		names[i] = string(o)
	}
	a.errorf("STATE-1", on.OpSpan, "状態 %s は %s なので %s はできない（%s にできるのは %s だけ。docs/check.md「状態の形」）", state.fqn, st.Shape, op, st.Shape, strings.Join(names, " / "))
}

// lowerTransition は `on X: A -> B`（docs/resolve.md「位置ごとに受け付ける種類」）。X は現在の名前空間の、状態を
// 対象にした classify か、組み込みの分類を持つ state（list / nullable）。別の名前空間のものなら RES-5。
// ケースの所属は STATE-4。書き込むのは、X が分類ならその対象のすべての基底の状態、X が状態ならその状態。
func (a *analyzer) lowerTransition(s *symbol, e *ir.Effect, t *ast.Transition, sc *scope) {
	r, ok := a.resolve(t.Of, sc, acState|acClassify)
	if !ok {
		return
	}
	if r.sym.ns != s.ns {
		what := "状態"
		if r.sym.kind == kindClassify {
			what = "分類"
		}
		a.errorf("RES-5", t.Of.Span, "effect %s の on は別の名前空間の%s %s を指している（効果は状態の持ち主の名前空間に宣言する。設計書 §6.2）", s.fqn, what, r.sym.fqn)
		return
	}
	var cases []string
	var writes []string
	what := ""
	switch decl := a.lower(r.sym).(type) {
	case *ir.State:
		if !decl.Shape.HasBuiltinClassify() {
			a.errorf("STATE-4", t.Of.Span, "状態 %s は %s で組み込みの分類がないので、遷移の宛先にできない（常に一列。docs/check.md「状態の形」）", decl.Name, decl.Shape)
			return
		}
		cases, writes = ir.BuiltinCases(), []string{decl.Name}
		what = fmt.Sprintf("状態 %s（%s）の組み込みの分類", decl.Name, decl.Shape)
	case *ir.Classify:
		states, isStates := decl.SubjectStates()
		if !isStates {
			if decl.Subject != nil {
				a.errorf("RES-4", t.Of.Span, "遷移の宛先にできるのは状態を対象にした classify だけで、%s は値の分類", decl.Name)
			}
			return // 対象が決まっていない分類（CLS-3 などで報告済み）には重ねて報告しない
		}
		for _, c := range decl.Cases {
			cases = append(cases, c.Name)
		}
		writes = append([]string(nil), states...)
		what = "classify " + decl.Name + " "
	default:
		return
	}
	target := ir.TransitionTarget{Of: r.sym.fqn}
	target.From = a.lowerCaseSet(r.sym, t.From, cases, what, "前")
	target.To = a.lowerCaseSet(r.sym, t.To, cases, what, "後")
	e.Target = target
	e.Writes = writes
}

// lowerCaseSet は遷移の前か後のケースの集合。STATE-4（docs/check.md「状態と効果」）: ケースが宛先の分類のケースでない
// （宛先が基底の状態なら Empty / HasItems）、並びに重複がある。IR には書かれた順に、書かれたまま載せる。
func (a *analyzer) lowerCaseSet(of *symbol, cs *ast.CaseSet, known []string, what, side string) ir.CaseSet {
	if cs == nil {
		return ir.CaseSet{}
	}
	if cs.Any {
		return ir.AnyCase()
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(cs.Cases))
	for _, c := range cs.Cases {
		out = append(out, c.Name)
		switch {
		case !contains(known, c.Name):
			a.errorf("STATE-4", c.Span, "遷移の%sのケース %s は、%sのケースではない（%s）", side, c.Name, what, strings.Join(known, " / "))
		case seen[c.Name]:
			a.errorf("STATE-4", c.Span, "遷移の%sのケースに %s が二度ある", side, c.Name)
		default:
			if span, ok := classifyCaseSpan(of, c.Name); ok {
				a.addRef(c.Span, Target{Kind: TargetCase, Decl: of.fqn, Name: c.Name, Span: span})
			}
		}
		seen[c.Name] = true
	}
	return ir.CaseSet{Cases: out}
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
	c.Subject = a.lowerClassifySubject(s, d.Of, sc)
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
	c.Subject = a.lowerClassifySubject(s, d.Of, sc)
	return c
}

// subjectKind は classify の対象の一つが何を指しているか。
type subjectKind int

const (
	// subjectType は型（として下ろすもの）。型として受け付けない名前は lowerType が RES-* で報告する。
	subjectType subjectKind = iota
	// subjectState は基底の状態。
	subjectState
	// subjectStateClassify は状態を対象にした classify（分類の状態）。
	subjectStateClassify
	// subjectUnresolved は解決できない名前。RES-* として報告する。
	subjectUnresolved
)

// lowerClassifySubject は classify / condition の対象（docs/resolve.md「位置ごとに受け付ける種類」の
// 「classify C of ... の対象」）。一つで型を指せば値の分類、すべてが現在の名前空間の state なら状態の分類。
// それ以外は CLS-3（docs/check.md「classify / match」）で、対象は決まらない（nil）。同じ状態が二度あるだけなら、
// 重複を除いた状態の分類にする。
func (a *analyzer) lowerClassifySubject(s *symbol, of []ast.Type, sc *scope) ir.ClassifySubject {
	if len(of) == 0 {
		return nil
	}
	kinds := make([]subjectKind, len(of))
	syms := make([]*symbol, len(of))
	states := 0
	for i, t := range of {
		kinds[i], syms[i] = a.subjectKindOf(t, sc)
		if kinds[i] == subjectState {
			states++
		}
	}
	if len(of) == 1 && (kinds[0] == subjectType || kinds[0] == subjectUnresolved) {
		if t, ok := a.lowerType(of[0], sc); ok {
			return ir.TypeSubject{Type: t}
		}
		return nil
	}
	// ここからは状態の分類として読む。状態でないもの、別の名前空間の状態、分類の状態、重複は CLS-3。
	ok := true
	seen := map[string]bool{}
	var fqns []string
	for i, t := range of {
		span := t.NodeSpan()
		switch kinds[i] {
		case subjectUnresolved:
			// 名前が見つからないことを先に言う（状態か型かは決まらない）
			a.resolve(t.(*ast.NamedType).Name, sc, acState|acTypePos)
			ok = false
		case subjectStateClassify:
			a.errorf("CLS-3", span, "classify %s の対象 %s は分類の状態（対象にできるのは宣言した名前空間の基底の状態だけ。docs/check.md「状態の形」）", s.fqn, syms[i].fqn)
			ok = false
		case subjectType:
			if states > 0 {
				a.errorf("CLS-3", span, "classify %s の対象に型と状態が混ざっている（%s は状態ではない）", s.fqn, a.text(span))
			} else {
				a.errorf("CLS-3", span, "classify %s の対象が二つ以上あるのに、%s は状態ではない（値の分類の対象は型一つ）", s.fqn, a.text(span))
			}
			ok = false
		case subjectState:
			nt := t.(*ast.NamedType)
			r, rok := a.resolve(nt.Name, sc, acState) // 参照を記録する
			if !rok {
				ok = false
				continue
			}
			if r.sym.ns != s.ns {
				a.errorf("CLS-3", span, "classify %s の対象 %s は別の名前空間の状態（対象にできるのは宣言した名前空間 %s の基底の状態だけ）", s.fqn, r.sym.fqn, nsName(s.ns))
				ok = false
				continue
			}
			if seen[r.sym.fqn] {
				a.errorf("CLS-3", span, "classify %s の対象に状態 %s が二度ある", s.fqn, r.sym.fqn)
				continue
			}
			seen[r.sym.fqn] = true
			fqns = append(fqns, r.sym.fqn)
		}
	}
	if !ok || len(fqns) == 0 {
		return nil
	}
	return ir.StatesSubject{States: fqns}
}

// subjectKindOf は対象の一つを、診断も参照も残さずに見分ける。型引数のない名前が state を指せば状態、
// 状態を対象にした classify を指せば分類の状態、解決できなければ subjectUnresolved、それ以外は型。
func (a *analyzer) subjectKindOf(t ast.Type, sc *scope) (subjectKind, *symbol) {
	nt, ok := t.(*ast.NamedType)
	if !ok || nt.Arg != nil {
		return subjectType, nil
	}
	r, ok := a.peek(nt.Name, sc)
	if !ok {
		return subjectUnresolved, nil
	}
	if r.kind != rDecl {
		return subjectType, nil
	}
	switch r.sym.kind {
	case kindState:
		return subjectState, r.sym
	case kindClassify:
		if c, ok := a.lower(r.sym).(*ir.Classify); ok {
			if _, isStates := c.SubjectStates(); isStates {
				return subjectStateClassify, r.sym
			}
		}
	}
	return subjectType, r.sym
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
	a.checkEFF3(s, d, op.Effects)
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

// lowerEffectRefs は effects の要素: effect か、効果の union（展開）。
func (a *analyzer) lowerEffectRefs(names []ast.QualifiedName, sc *scope) []string {
	var out []string
	for _, n := range names {
		if r, ok := a.resolve(n, sc, acEffectsPos); ok {
			out = append(out, a.expandSet(r, kindEffect, n.Span)...)
		}
	}
	return out
}

// checkEFF3 は EFF-3（docs/check.md「状態と効果」）: 葉の op の Op.effects（ユニオンを展開した後）が二つ以上。
// 葉かどうか（本体がない）と展開後の効果の列だけで決まり、Signature を要しないので、REL-1 / REL-5 と同じく
// internal/sema が出す。同じ効果を二度書いたものは一つと数える。配線の op（本体を持つ）は伝播した効果を
// いくつ書いてもよい（EFF-2 が照合する）。位置は op の宣言。
func (a *analyzer) checkEFF3(s *symbol, d *ast.OpDecl, effects []string) {
	if d.Body != nil {
		return
	}
	var distinct []string
	for _, e := range effects {
		if !contains(distinct, e) {
			distinct = append(distinct, e)
		}
	}
	if len(distinct) < 2 {
		return
	}
	a.errorf("EFF-3", d.Span, "葉の op %s が効果を二つ以上持っている（%s）。葉の op が直に持てる効果は一つまで。効果ごとの葉の op を配線でつなぐか、まとめて変える状態を対象にした分類の遷移一つにする", s.fqn, join(distinct))
}

// lowerThrows は throws の一節（docs/ir.md「decorator」の ThrowsClause）。REL-5 もここで確かめる。
func (a *analyzer) lowerThrows(c *ast.ThrowsClause, sc *scope) ir.ThrowsClause {
	tc := ir.ThrowsClause{Exceptions: a.lowerExceptionRefs(c.Exceptions, sc)}
	for _, r := range c.Relations {
		if r == nil {
			continue
		}
		rel := ir.Relation{Kind: ir.Prevents}
		if r.Kind == ast.Keeps {
			rel.Kind = ir.Keeps
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

// checkREL5 は REL-5（docs/check.md「throws」）: 一つの ThrowsClause に prevents と keeps の両方がある。
// 一つの節の relations はその節の例外すべてと、その op のすべての効果に掛かるので、両方あれば矛盾する。
func (a *analyzer) checkREL5(c ir.ThrowsClause, span source.Span) {
	prevents, keeps := false, false
	for _, r := range c.Relations {
		if r.Kind == ir.Prevents {
			prevents = true
		} else {
			keeps = true
		}
	}
	if prevents && keeps {
		a.errorf("REL-5", span, "例外 %s の throws に prevents と keeps の両方がある", join(c.Exceptions))
	}
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
