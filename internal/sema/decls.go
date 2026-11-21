package sema

import (
	"fmt"
	"slices"
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
// 呼び手がそれぞれの規則（TYPE-9 / UNION-1）で報告する。
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
	case *ast.ReadDecl:
		s.decl = a.lowerRead(s, d, sc)
	case *ast.ClassifyDecl:
		s.decl = a.lowerClassify(s, d, sc)
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

// lowerModel は model（docs/ir.md「model / scalar / union」）。本体の Field を下ろす。継承した model の展開と
// Override は finalizeModels で行う（元の model のフィールドのデコレータが下りてから写すため）。
func (a *analyzer) lowerModel(s *symbol, d *ast.ModelDecl, sc *scope) ir.Decl {
	m := &ir.Model{DeclBase: a.base(s, d.Doc, d.Span, docContext{})}
	a.models[s.fqn] = m
	a.fields[s.fqn] = map[string]source.Span{}
	a.addSite(d.Decorators, siteModel, sc, &m.DeclBase.Decorators)
	if d.Body != nil {
		m.Fields = a.lowerFields(d.Body.Fields, s.fqn, sc)
	}
	a.addModelJob(&modelJob{sym: s, model: m, body: d.Body, extends: d.Extends, sc: sc})
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
	a.addModelJob(&modelJob{model: m, body: body, sc: sc})
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

// lowerState は state。型は宣言に書いた型で、形と要素はその最も外側から決まる（docs/check.md「状態の形」）。
func (a *analyzer) lowerState(s *symbol, d *ast.StateDecl, sc *scope) ir.Decl {
	st := &ir.State{DeclBase: a.base(s, d.Doc, d.Span, docContext{})}
	a.addSite(d.Decorators, siteState, sc, &st.DeclBase.Decorators)
	st.Shape = ir.StateSingle
	if t, ok := a.lowerType(d.Type, sc); ok {
		st.Type = a.checkSTATE3(s, d, t)
		st.Shape, st.Element = ir.ShapeOf(st.Type)
	}
	return st
}

// checkSTATE3 は STATE-3（docs/check.md「状態と効果」）: 状態の要素の型が optional。形が型から決まるので、当たるのは
// list の状態だけ（list<T?>、T?[]）。当たった状態は list<T> として残りの検査を続けるので、その型を返す
// （IR の element は optional にならない。docs/ir.md「state / exception / effect」）。
func (a *analyzer) checkSTATE3(s *symbol, d *ast.StateDecl, t ir.Type) ir.Type {
	arr, ok := t.(ir.ArrayType)
	if !ok {
		return t
	}
	opt, ok := arr.Of.(ir.OptionalType)
	if !ok {
		return t
	}
	a.errorf("STATE-3", d.Type.NodeSpan(), "状態 %s の型 %s は、列の要素に ? が付いている。列は値を持つので、要素は省略可能にできない（%s と書く。docs/check.md「状態の形」）", s.fqn, typeName(t), typeName(ir.Array(opt.Of)))
	return ir.Array(opt.Of)
}

// lowerDeclParams は exception / effect / read の引数の並び（docs/ir.md の OpParam）。型は「型」の位置と同じに解決する
// （op の引数と同じ扱い）。
func (a *analyzer) lowerDeclParams(params []*ast.OpParam, sc *scope) []ir.OpParam {
	var out []ir.OpParam
	for _, p := range params {
		if p == nil {
			continue
		}
		param := ir.OpParam{Name: p.Name.Name, Span: a.irSpan(p.Span)}
		if t, ok := a.lowerType(p.Type, sc); ok {
			param.Type = t
		}
		out = append(out, param)
	}
	return out
}

// lowerException は exception（docs/ir.md「state / exception / effect」）。引数は例外が運ぶ値。
func (a *analyzer) lowerException(s *symbol, d *ast.ExceptionDecl, sc *scope) ir.Decl {
	e := &ir.Exception{DeclBase: a.base(s, d.Doc, d.Span, docContext{params: paramNames(d.Params), hasParams: true})}
	a.addSite(d.Decorators, siteException, sc, &e.DeclBase.Decorators)
	e.Params = a.lowerDeclParams(d.Params, sc)
	return e
}

// lowerEffect は effect。宛先は一つだけで、基底の状態の組み込みの効果か、分類の遷移（docs/check.md「状態の形」の
// 「効果の宛先」）。宛先が決まらなければ Target は nil のまま。引数は効果が受け取る値で、検査器は宛先の状態の型と
// 照らさない（docs/check.md「read と効果」）。
func (a *analyzer) lowerEffect(s *symbol, d *ast.EffectDecl, sc *scope) ir.Decl {
	e := &ir.Effect{DeclBase: a.base(s, d.Doc, d.Span, docContext{params: paramNames(d.Params), hasParams: true})}
	a.addSite(d.Decorators, siteEffect, sc, &e.DeclBase.Decorators)
	e.Params = a.lowerDeclParams(d.Params, sc)
	// TYPE-18: 効果が返せるのは void だけ。戻り値は、省くか、void と書く（docs/grammar.md「関数の宣言」）
	if d.Return != nil && !d.Return.Void {
		a.errorf("TYPE-18", d.Return.Span, "effect fn %s は、値を返せない（効果の戻り値は、省くか、void と書く）", s.fqn)
	}
	a.lowerEffectThrows(s, e, d, sc)
	switch {
	case d.On != nil:
		a.lowerBuiltinTarget(s, e, d.On, sc)
		if d.Row != nil {
			a.lowerRow(s, e, d.Row, sc)
		}
	case d.Transition != nil:
		a.lowerTransition(s, e, d.Transition, sc)
	}
	return e
}

// lowerEffectThrows は、効果が投げる例外（`: throws E effects …`。docs/check.md「read と効果」）を e.Throws に置く。
// REL-1: 同じ例外が二度ある。DOC-1: 効果の @throws の例外が、その効果の throws にない。
func (a *analyzer) lowerEffectThrows(s *symbol, e *ir.Effect, d *ast.EffectDecl, sc *scope) {
	e.Throws = []string{}
	if d.Throws != nil {
		seen := map[string]bool{}
		for _, x := range a.lowerExceptionRefs(d.Throws.Exceptions, sc) {
			if seen[x] {
				a.errorf("REL-1", d.Throws.Span, "%s の throws に例外 %s が二度ある", s.fqn, x)
				continue
			}
			seen[x] = true
			e.Throws = append(e.Throws, x)
		}
	}
	if e.Description == nil || d.Doc == nil {
		return
	}
	for _, dt := range e.Description.Throws {
		if slices.Contains(e.Throws, dt.Exception) {
			continue
		}
		span := d.Doc.Span
		for i := range d.Doc.Tags {
			if tag := &d.Doc.Tags[i]; tag.Kind == ast.DocThrows && strings.HasSuffix(dt.Exception, tag.Name.String()) {
				span = tag.Name.Span
				break
			}
		}
		a.errorf("DOC-1", span, "@throws の %s は、効果 %s の throws にない", dt.Exception, s.fqn)
	}
}

// rowCase は、行の遷移に書いたケース一つ（`C.K`）を解決したもの。
type rowCase struct {
	classify *ir.Classify
	name     string
	span     source.Span // K の位置
}

// lowerRow は行の遷移（`effects s.op as …`。docs/check.md「行の遷移」）。確かめて、誤りがなければ e.Row に置く
// （post の `as any` は、確かめるだけで、置かない: as を書かないのと同じ意味）。
// STATE-7（結びつき）: ケース `C.K` の C が、書き込む状態の行の分類（値の分類で、対象の型が、状態の要素の型 T か T?）でない。
// K が C のケースでない。一つの as に二つ以上の分類のケースがある。前のケースの並びに同じケースが二度ある。効果の引数に、
// 型が T のものが、ちょうど一つでない。STATE-8（形）: 形が、操作に合わない（同「操作ごとに書ける形」）。
// 宛先が決まらなかった効果（RES-5、STATE-1 で報告済み）には、名前の解決だけを行う。
func (a *analyzer) lowerRow(s *symbol, e *ir.Effect, r *ast.RowClause, sc *scope) {
	var element ir.Type
	var op ir.Operation
	if target, ok := e.Target.(ir.BuiltinTarget); ok {
		op = target.Operation
		if sym := a.syms[target.State]; sym != nil {
			if st, ok := a.lower(sym).(*ir.State); ok {
				element = st.Element
			}
		}
	}
	ok := true
	var classify *ir.Classify
	// 解決: 分類は、最後の要素（ケースの名前）を除いた名前
	resolveCase := func(qn *ast.QualifiedName) *rowCase {
		n := len(qn.Segments)
		if n < 2 {
			ok = false
			return nil
		}
		last := qn.Segments[n-1]
		prefix := ast.QualifiedName{
			Span:     source.Span{File: qn.Span.File, Start: qn.Span.Start, End: qn.Segments[n-2].Span.End},
			Segments: qn.Segments[:n-1],
		}
		res, found := a.resolve(prefix, &scope{ns: sc.ns}, acClassify)
		if !found {
			ok = false
			return nil
		}
		c, isClassify := a.lower(res.sym).(*ir.Classify)
		if !isClassify || c.Subject == nil {
			ok = false
			return nil // 対象が決まっていない分類（CLS-3 などで報告済み）には重ねて報告しない
		}
		if element != nil {
			subj, isType := c.Subject.(ir.TypeSubject)
			if !isType {
				a.errorf("STATE-7", qn.Span, "effect %s の行の遷移: %s は%sで、行の分類（値の分類）ではない（docs/check.md「行の遷移」の「結びつき」）", s.fqn, c.Name, classifyKindName(c))
				ok = false
				return nil
			}
			if !ir.TypeEqual(subj.Type, element) && !ir.TypeEqual(subj.Type, ir.Optional(element)) {
				a.errorf("STATE-7", qn.Span, "effect %s の行の遷移: classify %s の対象の型は %s で、書き込む状態の要素の型 %s でも %s でもない（docs/check.md「行の遷移」の「結びつき」）",
					s.fqn, c.Name, typeName(subj.Type), typeName(element), typeName(ir.Optional(element)))
				ok = false
				return nil
			}
		}
		span, isCase := classifyCaseSpan(res.sym, last.Name)
		if !isCase || c.Case(last.Name) == nil {
			a.errorf("STATE-7", last.Span, "effect %s の行の遷移: %s は classify %s のケースではない（%s）", s.fqn, last.Name, c.Name, strings.Join(caseNames(c), " / "))
			ok = false
			return nil
		}
		a.addRef(last.Span, Target{Kind: TargetCase, Decl: c.Name, Name: last.Name, Span: span})
		if classify == nil {
			classify = c
		} else if classify.Name != c.Name {
			a.errorf("STATE-7", qn.Span, "effect %s の行の遷移に、二つの分類（%s と %s）のケースがある。一つの as が言うのは、一つの分類のこと（docs/check.md「行の遷移」の「書き方と意味」）", s.fqn, classify.Name, c.Name)
			ok = false
			return nil
		}
		return &rowCase{classify: c, name: last.Name, span: last.Span}
	}
	var before []*rowCase
	if r.Before != nil {
		seen := map[string]bool{}
		for i := range r.Before.Cases {
			rc := resolveCase(&r.Before.Cases[i])
			if rc == nil {
				continue
			}
			if seen[rc.name] {
				a.errorf("STATE-7", rc.span, "effect %s の行の遷移の、前のケースに %s が二度ある", s.fqn, rc.name)
				ok = false
				continue
			}
			seen[rc.name] = true
			before = append(before, rc)
		}
	}
	var after *rowCase
	if r.After.Case != nil {
		after = resolveCase(r.After.Case)
	}
	if element == nil || !ok {
		return
	}
	// 行の引数: 型が、状態の要素の型と等しい引数が、ちょうど一つ
	var rows []string
	for _, p := range e.Params {
		if p.Type != nil && ir.TypeEqual(p.Type, element) {
			rows = append(rows, p.Name)
		}
	}
	if len(rows) != 1 {
		found := "一つもない"
		if len(rows) > 1 {
			found = "二つ以上ある（" + strings.Join(rows, "、") + "）"
		}
		a.errorf("STATE-7", r.AsSpan, "effect %s の行の遷移: 型が %s（書き込む状態の要素の型）の引数が、%s。行の遷移を書く効果は、行を受け取る引数を、ちょうど一つ持つ（行の引数を持たない効果は as を書けない。docs/check.md「行の遷移」の「結びつき」）",
			s.fqn, typeName(element), found)
		return
	}
	absent := ""
	if classify != nil {
		absent = classify.Absent
	}
	if !a.checkSTATE8(s, op, r, before, after, absent) {
		return
	}
	if op == ir.OpPost && after == nil {
		// post の `as any` は、as を書かないのと同じ意味で、IR では同じ形（row を持たない。docs/ir.md「正規化」）
		return
	}
	row := &ir.RowTransition{Param: rows[0], From: ir.AnyCase()}
	if classify != nil {
		row.Classify = classify.Name
	}
	if after != nil {
		row.To = after.name
	}
	// post の前は、IR に残さない（入る前の行は、ない。docs/ir.md「正規化」）
	if op != ir.OpPost && r.Before != nil && !r.Before.Any {
		names := make([]string, len(before))
		for i, rc := range before {
			names[i] = rc.name
		}
		row.From = ir.Cases(names...)
	}
	e.Row = row
}

// caseNames は分類のケースの名前（宣言順）。
func caseNames(c *ir.Classify) []string {
	out := make([]string, len(c.Cases))
	for i, cs := range c.Cases {
		out[i] = cs.Name
	}
	return out
}

// checkSTATE8 は STATE-8（docs/check.md「行の遷移」の「操作ごとに書ける形」）: 行の遷移の形が、操作に合わない。
// absent は、分類のない側のケースの名前（なければ空）。一つの効果に、一つだけ報告する。合っていれば true。
func (a *analyzer) checkSTATE8(s *symbol, op ir.Operation, r *ast.RowClause, before []*rowCase, after *rowCase, absent string) bool {
	fail := func(span source.Span, format string, args ...any) bool {
		a.errorf("STATE-8", span, "effect %s の行の遷移: %s（docs/check.md「行の遷移」の「操作ごとに書ける形」）", s.fqn, fmt.Sprintf(format, args...))
		return false
	}
	isAbsent := func(rc *rowCase) bool { return rc != nil && absent != "" && rc.name == absent }
	switch op {
	case ir.OpPost:
		const forms = "post に書けるのは、as <ケース>（そのケースで入る）、as any、as <ない側のケース> -> <ケース>"
		if !r.Arrow {
			if isAbsent(after) {
				return fail(after.span, "%s は、ない側のケースで、post の後のケースには書けない（入った行は、ある）。%s", after.name, forms)
			}
			return true
		}
		if r.Before.Any {
			return fail(r.Before.Span, "post の矢印の前に any は書けない（入る前の行は、ない）。%s", forms)
		}
		for _, rc := range before {
			if !isAbsent(rc) {
				return fail(rc.span, "post の矢印の前に書けるのは、ない側のケース一つだけで、%s は、そうではない（入る前の行は、ない）。%s", rc.name, forms)
			}
		}
		if r.After.Else {
			return fail(r.After.Span, "post の矢印の後ろに else は書けない（入るケースが決まっていないなら、as を書かない）。%s", forms)
		}
		if isAbsent(after) {
			return fail(after.span, "%s は、ない側のケースで、post の後のケースには書けない（入った行は、ある）。%s", after.name, forms)
		}
	case ir.OpPut:
		const forms = "put に書けるのは、as <前> -> <後>（前は any か、ケースの並び。後は、ケース一つか else）"
		if !r.Arrow {
			return fail(r.AsSpan, "put は、矢印で書く（どのケースからでもよければ as any -> <後>）。%s", forms)
		}
		for _, rc := range before {
			if isAbsent(rc) {
				return fail(rc.span, "%s は、ない側のケースで、put の前のケースには書けない（put は、ある行を書き換える）。%s", rc.name, forms)
			}
		}
		if isAbsent(after) {
			return fail(after.span, "%s は、ない側のケースで、put の後のケースには書けない（行をなくすのは delete）。%s", after.name, forms)
		}
	case ir.OpDelete:
		const forms = "delete に書けるのは、as <ない側のケース>、as <前> -> <ない側のケース>（前は any か、ない側でないケースの並び）。ない側のケースは、T? を対象にした分類に <名前>: T? as none; と書く"
		if !r.Arrow && r.After.Any {
			return fail(r.After.Span, "delete の後は、ない側のケース（消した後の行は、ない）。%s", forms)
		}
		if r.After.Else {
			return fail(r.After.Span, "delete の矢印の後ろに else は書けない（消した後の行は、ない）。%s", forms)
		}
		if after != nil && !isAbsent(after) {
			if absent == "" {
				return fail(after.span, "classify %s は、ない側のケースを持たないので、delete の効果の as には書けない（「行がない」を言うケースがない）。%s", after.classify.Name, forms)
			}
			return fail(after.span, "delete の後のケースは、ない側のケース %s で、%s ではない（消した後の行は、ない）。%s", absent, after.name, forms)
		}
		for _, rc := range before {
			if isAbsent(rc) {
				return fail(rc.span, "%s は、ない側のケースで、delete の前のケースには書けない（消すのは、ある行）。%s", rc.name, forms)
			}
		}
	}
	return true
}

// lowerRead は read（docs/ir.md「read」）。宛先は現在の名前空間の基底の状態一つ（docs/check.md「read の宛先」）。
// 別の名前空間の状態なら RES-5、状態でなければ（状態の分類を含む）RES-4。宛先が決まらなければ State は空のまま。
// 引数と結果の型は、書かれたままの約束で、宛先の状態の型とは照らさない（同「read と効果」）。
func (a *analyzer) lowerRead(s *symbol, d *ast.ReadDecl, sc *scope) ir.Decl {
	rd := &ir.Read{DeclBase: a.base(s, d.Doc, d.Span, docContext{params: paramNames(d.Params), hasParams: true})}
	a.addSite(d.Decorators, siteRead, sc, &rd.DeclBase.Decorators)
	rd.Params = a.lowerDeclParams(d.Params, sc)
	// TYPE-18: read は値を返す。省いた形、void、never は書けない（docs/grammar.md「関数の宣言」）
	switch {
	case d.Return == nil:
		a.errorf("TYPE-18", d.Name.Span, "read fn %s は、戻り値の型を省けない（read は値を返す。read fn %s(…): T reads 状態; と書く）", s.fqn, d.Name.Name)
	case d.Return.Void, d.Return.Never:
		word := "void"
		if d.Return.Never {
			word = "never"
		}
		a.errorf("TYPE-18", d.Return.Span, "read fn %s の戻り値に %s は書けない（read は値を返す）", s.fqn, word)
	}
	if d.Result != nil {
		if t, ok := a.lowerType(d.Result, sc); ok {
			rd.Returns = t
		}
	}
	r, ok := a.resolve(d.On, sc, acState)
	if !ok {
		return rd
	}
	if r.sym.ns != s.ns {
		a.errorf("RES-5", d.On.Span, "read %s の on は別の名前空間の状態 %s を指している（read は状態の持ち主の名前空間に宣言する）", s.fqn, r.sym.fqn)
		return rd
	}
	rd.State = r.sym.fqn
	return rd
}

// lowerBuiltinTarget は `effects s.op`。s は現在の名前空間の state（別の名前空間なら RES-5）。STATE-1。
// 書き込むのはその状態（docs/check.md「状態の形」の「効果が書き込む状態」）。
func (a *analyzer) lowerBuiltinTarget(s *symbol, e *ir.Effect, on *ast.StateOpRef, sc *scope) {
	r, ok := a.resolve(on.State, sc, acState)
	if !ok {
		return
	}
	if r.sym.ns != s.ns {
		a.errorf("RES-5", on.State.Span, "effect %s の on は別の名前空間の状態 %s を指している（効果は状態の持ち主の名前空間に宣言する）", s.fqn, r.sym.fqn)
		return
	}
	e.Target = ir.BuiltinTarget{State: r.sym.fqn, Operation: operationOf(on.Op)}
	e.Writes = []string{r.sym.fqn}
	a.checkSTATE1(r.sym, operationOf(on.Op), on.OpSpan)
}

// builtinOperations は形ごとの組み込みの効果（docs/check.md「状態の形」の表）。
var builtinOperations = map[ir.StateShape][]ir.Operation{
	ir.StateList:     {ir.OpPost, ir.OpPut, ir.OpDelete},
	ir.StateNullable: {ir.OpPost, ir.OpPut, ir.OpDelete},
	ir.StateSingle:   {ir.OpPut},
}

// checkSTATE1 は STATE-1（docs/check.md「状態と効果」）: 組み込みの効果の operation が、その状態の形にない
// （single に post / delete）。effect の宣言の宛先にも、effects に直に書いた「状態.操作」にも、配線で直に呼んだ
// 「状態.操作(…)」にも掛かる。span は操作の語。形にある操作なら true。
func (a *analyzer) checkSTATE1(state *symbol, op ir.Operation, span source.Span) bool {
	st, ok := a.lower(state).(*ir.State)
	if !ok {
		return false
	}
	allowed := builtinOperations[st.Shape]
	for _, o := range allowed {
		if o == op {
			return true
		}
	}
	names := make([]string, len(allowed))
	for i, o := range allowed {
		names[i] = string(o)
	}
	a.errorf("STATE-1", span, "状態 %s は %s なので %s はできない（%s にできるのは %s だけ。docs/check.md「状態の形」）", state.fqn, st.Shape, op, st.Shape, strings.Join(names, " / "))
	return false
}

// lowerTransition は `effects X as A -> B`（docs/resolve.md「位置ごとに受け付ける種類」）。X は現在の名前空間の、状態を
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
		a.errorf("RES-5", t.Of.Span, "effect %s の on は別の名前空間の%s %s を指している（効果は状態の持ち主の名前空間に宣言する）", s.fqn, what, r.sym.fqn)
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
				a.errorf("RES-4", t.Of.Span, "遷移の宛先にできるのは状態を対象にした classify だけで、%s は%s", decl.Name, classifyKindName(decl))
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

// --- classify ---

// lowerClassify は classify。CLS-1（ケース名の重複、残りのケース `対象 as else` が二つ以上、`as none` が二つ以上）、
// CLS-2（ケースがない）、ケースの `対象 as T`（CLS-5 / CLS-6）、残りのケース（CLS-7）、ない側のケース（CLS-8）、
// ケースの判定（decidedBy。docs/check.md「ケースの判定」「残りのケース」「ない側のケース」）。
func (a *analyzer) lowerClassify(s *symbol, d *ast.ClassifyDecl, sc *scope) ir.Decl {
	c := &ir.Classify{DeclBase: a.base(s, d.Doc, d.Span, docContext{})}
	a.addSite(d.Decorators, siteClassify, sc, &c.DeclBase.Decorators)
	if d.Never {
		// 対象のない分類（docs/check.md「分類」の「対象のない分類」）: 値の分類で、対象の型が never
		c.Subject = ir.TypeSubject{Type: ir.Never()}
	} else {
		c.Subject = a.lowerClassifySubject(s, d.Of, sc)
	}
	seen := map[string]bool{}
	rest := restCases{wrote: map[string]bool{}}
	var decorated []*ast.ClassifyCase
	for _, cs := range d.Cases {
		if cs == nil {
			continue
		}
		if seen[cs.Name.Name] {
			a.errorf("CLS-1", cs.Name.Span, "classify %s にケース %s が二度ある", s.fqn, cs.Name.Name)
			continue
		}
		seen[cs.Name.Name] = true
		a.checkCaseName(s, cs.Name)
		cc := ir.ClassifyCase{Name: cs.Name.Name, Span: a.irSpan(cs.Span)}
		if cs.Doc != nil {
			cc.Description = a.lowerDoc(cs.Doc, docContext{ns: sc.ns, span: cs.Name.Span})
		}
		if d.Never {
			if len(cs.As) > 0 && cs.As[0] != nil {
				a.errorf("CLS-9", cs.As[0].Span, "classify %s は、対象のない分類（of never）なので、ケース %s に as は書けない（ケースは、名前だけ）", s.fqn, cs.Name.Name)
			}
		} else {
			cc.As = a.lowerCaseTypings(s, c, cs, sc, &rest)
		}
		c.Cases = append(c.Cases, cc)
		decorated = append(decorated, cs)
	}
	if len(c.Cases) == 0 {
		a.errorf("CLS-2", s.nameSpan, "classify %s にケースが一つもない", s.fqn)
	}
	if d.Never {
		c.DecidedBy = ir.DecidedByNone
	} else {
		a.lowerNoneCase(s, c, &rest)
		a.lowerElseCase(s, c, &rest)
		c.DecidedBy = a.decidedBy(c)
	}
	// ケースの前のデコレータ。c.Cases は、もう伸びないので、要素を指せる
	for i, cs := range decorated {
		if site := a.addSite(cs.Decorators, siteCase, sc, &c.Cases[i].Decorators); site != nil {
			site.caseOf = c
		}
	}
	return c
}

// restCases は、一つの classify の中の、残りのケース（`対象 as else`）の覚え書き（docs/check.md「残りのケース」）。
type restCases struct {
	// said は、as else を、もう見たか。二つ目からは CLS-1。
	said bool
	// wrote は、as else を書いたケースの名前（誤りのものを含む）。CLS-7 の「ほかのケース」から除く。
	wrote map[string]bool
	// first は、最初に書かれた as else（対象が分類の対象で、残りを言うものの二つ目でないもの）。なければ nil。
	first *elseTyping
	// broken は、どれかのケースの as に誤り（CLS-5 / CLS-6）があったか。あれば、CLS-7 と CLS-8 を重ねて報告しない。
	broken bool
	// none は、最初に書かれた as none（ない側のケース。docs/check.md「ない側のケース」）。なければ nil。二つ目からは CLS-1。
	// elseSpan は none の位置。
	none *elseTyping
}

// presenceOnly は、分類 c（値の分類か op の分類で、対象の型 subjectType が T?）の、skip にないケースが、一つ以上あり、
// どれも、対象を T に絞る as（presence）を持つか（docs/check.md「残りのケース」の 2、「ない側のケース」）。
// 満たさなければ、理由を返す。
func presenceOnly(c *ir.Classify, subjectType ir.Type, skip map[string]bool) (ok bool, reason string) {
	if _, isStates := c.SubjectStates(); isStates {
		return false, "状態の分類"
	}
	if _, isOptional := subjectType.(ir.OptionalType); !isOptional {
		return false, fmt.Sprintf("対象の型 %s は、値がないことのある型（T?）ではない", typeName(subjectType))
	}
	n := 0
	for _, cs := range c.Cases {
		if skip[cs.Name] {
			continue
		}
		n++
		if len(cs.As) != 1 || cs.As[0].By != ir.NarrowByPresence {
			return false, fmt.Sprintf("ケース %s が、対象を、値のある型に絞る as（T? as T）を持たない", cs.Name)
		}
	}
	if n == 0 {
		return false, "ほかに、ケースが一つもない"
	}
	return true, ""
}

// lowerNoneCase は、ない側のケース `対象 as none`（rest.none）を確かめて、そのケースの as に足す（docs/check.md「ない側のケース」）。
// CLS-8: 値の分類でも op の分類でもない。対象の型が T? でない。ほかのケースが、一つもないか、T? as T を持たないものがある。
// ほかのケースの as に誤りがあれば、重ねて報告しない。
func (a *analyzer) lowerNoneCase(s *symbol, c *ir.Classify, rest *restCases) {
	e := rest.none
	if e == nil || rest.broken || e.subjectType == nil {
		return
	}
	if ok, reason := presenceOnly(c, e.subjectType, rest.wrote); !ok {
		a.errorf("CLS-8", e.elseSpan, "classify %s のケース %s: ない側のケース（as none）を書けるのは、値の分類か op の分類で、対象の型が T?、ほかのケースが、どれも T? as T のとき。%s（docs/check.md「ない側のケース」）",
			s.fqn, e.name, reason)
		rest.none = nil
		return
	}
	if cc := c.Case(e.name); cc != nil {
		cc.As = append(cc.As, ir.CaseTyping{State: e.state, Type: e.subjectType, By: ir.NarrowByNone, Span: a.irSpan(e.span)})
		c.Absent = e.name
	}
}

// elseTyping は、ケース name の `対象 as else`。state は対象の状態の FQN（状態の分類のとき）、subjectType は対象の型。
type elseTyping struct {
	name        string
	state       string
	subjectType ir.Type
	span        source.Span // as else の全体
	elseSpan    source.Span // else
}

// lowerElseCase は、残りのケース（rest.first）を確かめて、そのケースの as に足す（docs/check.md「残りのケース」）。
// CLS-7: 残りのケースを除いたケースが、一つもないか、「ケースの判定」の条件 1〜3 を満たさない。または、
// variant の絞り込みで残りのアームがない。ほかのケースの as に誤りがあれば、重ねて報告しない。
func (a *analyzer) lowerElseCase(s *symbol, c *ir.Classify, rest *restCases) {
	e := rest.first
	if e == nil || rest.broken || e.subjectType == nil {
		return
	}
	fail := func(format string, args ...any) {
		a.errorf("CLS-7", e.elseSpan, "classify %s のケース %s: 残りのケース（as else）を書けるのは、残りが型で決まるときだけ。%s（docs/check.md「残りのケース」）",
			s.fqn, e.name, fmt.Sprintf(format, args...))
	}
	if rest.none != nil {
		fail("ない側のケース %s（as none）があるので、残りがない", rest.none.name)
		return
	}
	// 残りが、ちょうど「値がない」（docs/check.md「残りのケース」の 2）: 値の分類か op の分類で、対象の型が T?、ほかのケースが、
	// どれも T? as T。ほかのケースが二つ以上で同じ型を書いていても（述語で決まる分類でも）、書ける。この残りは、ない側のケース
	if ok, _ := presenceOnly(c, e.subjectType, rest.wrote); ok {
		if cc := c.Case(e.name); cc != nil {
			cc.As = append(cc.As, ir.CaseTyping{State: e.state, Type: e.subjectType, By: ir.NarrowByElse, Span: a.irSpan(e.span)})
			c.Absent = e.name
		}
		return
	}
	// 1. 対象が一つ
	if states, isStates := c.SubjectStates(); isStates && len(states) != 1 {
		fail("対象が二つ以上の状態の分類には書けない")
		return
	}
	// 2. ほかのケースのすべてが、その対象の、型で決まる絞り込みの as を持つ。3. その型が、ケースの間ですべて違う
	var types []ir.Type
	var names []string
	by := ir.NarrowBy("")
	for _, cs := range c.Cases {
		if rest.wrote[cs.Name] {
			continue
		}
		var as *ir.CaseTyping
		for i := range cs.As {
			if cs.As[i].State == e.state {
				as = &cs.As[i]
			}
		}
		if as == nil {
			fail("ケース %s は as を持たない（述語のケース）", cs.Name)
			return
		}
		if as.By == ir.NarrowByPredicate {
			fail("ケース %s の as は、述語の要る絞り込み（型からは決まらない）", cs.Name)
			return
		}
		for i, t := range types {
			if ir.TypeEqual(t, as.Type) {
				fail("ケース %s と %s が同じ型 %s を書いている（述語のケース）", names[i], cs.Name, typeName(as.Type))
				return
			}
		}
		types, names, by = append(types, as.Type), append(names, cs.Name), as.By
	}
	if len(types) == 0 {
		fail("ほかに、型を書いたケースが一つもない")
		return
	}
	// 残りのケースでの対象の型（docs/types.md「残りのケースでの対象の型」）
	typ := e.subjectType
	if by == ir.NarrowByVariant {
		remaining := a.remainingArms(e.subjectType, types)
		if len(remaining) == 0 {
			fail("ほかのケースが union のすべてのアームの型を覆っていて、残りのアームがない")
			return
		}
		if len(remaining) == 1 && remaining[0].Type != nil {
			typ = remaining[0].Type
		}
	}
	if cc := c.Case(e.name); cc != nil {
		cc.As = append(cc.As, ir.CaseTyping{State: e.state, Type: typ, By: ir.NarrowByElse, Span: a.irSpan(e.span)})
		if by == ir.NarrowByPresence {
			// 対象の状態が一つの状態の分類の、値がない側（docs/check.md「ない側のケース」の表の二行目）
			c.Absent = e.name
		}
	}
}

// remainingArms は、union u のアームのうち、型が types のどれとも等しくないもの（残りのアーム。docs/check.md「残りのケース」）。
func (a *analyzer) remainingArms(u ir.Type, types []ir.Type) []ir.UnionArm {
	un := a.unionArms(u)
	if un == nil {
		return nil
	}
	var out []ir.UnionArm
	for _, arm := range un.Arms {
		named := false
		for _, t := range types {
			if arm.Type != nil && ir.TypeEqual(arm.Type, t) {
				named = true
			}
		}
		if !named {
			out = append(out, arm)
		}
	}
	return out
}

// classifyKindName は分類の種類の呼び名（診断用）。
func classifyKindName(c *ir.Classify) string {
	switch c.Subject.(type) {
	case ir.OpSubject:
		return "op の分類"
	case ir.StatesSubject:
		return "状態の分類"
	}
	return "値の分類"
}

// lowerCaseTypings はケースの `対象 as T` の並び（docs/check.md「ケースの型」）。CLS-5: as の前がその分類の対象でない、
// 一つのケースに同じ対象の as が二度ある。CLS-6: as の後ろの型が、対象の型から絞り込める型でない。
// 対象が決まっていない分類（CLS-3 などで報告済み）では、型の解決だけを行う。
// `対象 as else`（残りのケース）と `対象 as none`（ない側のケース）は、ここでは rest に覚えるだけで、as には足さない
// （確かめて足すのは lowerElseCase と lowerNoneCase）。CLS-1: 残りのケースが、もうある（別の as else）。as none が、もうある。
func (a *analyzer) lowerCaseTypings(s *symbol, c *ir.Classify, cs *ast.ClassifyCase, sc *scope, rest *restCases) []ir.CaseTyping {
	var out []ir.CaseTyping
	seen := map[string]bool{}
	for _, ct := range cs.As {
		if ct == nil {
			continue
		}
		if ct.Else || ct.None {
			rest.wrote[cs.Name.Name] = true
		}
		var t ir.Type
		tok := true
		if !ct.Else && !ct.None {
			t, tok = a.lowerType(ct.Type, sc)
		}
		if c.Subject == nil {
			continue
		}
		state, subjectType, ok := a.caseTypingSubject(s, c, ct, sc)
		if !ok {
			rest.broken = true
			continue
		}
		if seen[state] {
			a.errorf("CLS-5", ct.Subject.NodeSpan(), "classify %s のケース %s に、対象 %s の as が二度ある", s.fqn, cs.Name.Name, a.text(ct.Subject.NodeSpan()))
			rest.broken = true
			continue
		}
		seen[state] = true
		if ct.Else {
			if rest.said {
				a.errorf("CLS-1", ct.ElseSpan, "classify %s に、残りのケース（as else）が二つ以上ある（残りは一つ。docs/check.md「残りのケース」）", s.fqn)
				continue
			}
			rest.said = true
			rest.first = &elseTyping{name: cs.Name.Name, state: state, subjectType: subjectType, span: ct.Span, elseSpan: ct.ElseSpan}
			continue
		}
		if ct.None {
			if rest.none != nil {
				a.errorf("CLS-1", ct.NoneSpan, "classify %s に、ない側のケース（as none）が二つ以上ある（ない側は一つ。docs/check.md「ない側のケース」）", s.fqn)
				continue
			}
			rest.none = &elseTyping{name: cs.Name.Name, state: state, subjectType: subjectType, span: ct.Span, elseSpan: ct.NoneSpan}
			continue
		}
		if !tok || subjectType == nil {
			rest.broken = rest.broken || !tok
			continue
		}
		if !a.narrowable(subjectType, t) {
			a.errorf("CLS-6", ct.Type.NodeSpan(), "classify %s のケース %s: 対象の型 %s を %s には絞り込めない（絞り込めるのは、対象の型に渡せる型か、対象の union のアームの型。docs/types.md「絞り込める型」）", s.fqn, cs.Name.Name, typeName(subjectType), typeName(t))
			rest.broken = true
			continue
		}
		out = append(out, ir.CaseTyping{State: state, Type: t, By: a.narrowBy(subjectType, t), Span: a.irSpan(ct.Span)})
	}
	return out
}

// caseTypingSubject は `X as T` の X を、その分類の対象と照らす。状態の分類ならその状態の FQN と型、値の分類と
// op の分類なら state は空で、対象の型を返す。対象でなければ CLS-5 を出して ok は false。
// 解決は対象と同じ規則で行う（docs/resolve.md「位置ごとに受け付ける種類」）。
func (a *analyzer) caseTypingSubject(s *symbol, c *ir.Classify, ct *ast.CaseTyping, sc *scope) (state string, subjectType ir.Type, ok bool) {
	span := ct.Subject.NodeSpan()
	notSubject := func() (string, ir.Type, bool) {
		a.errorf("CLS-5", span, "classify %s のケースの as の前の %s は、この分類の対象ではない（%s）", s.fqn, a.text(span), subjectText(c))
		return "", nil, false
	}
	var named *symbol
	nt, isName := ct.Subject.(*ast.NamedType)
	if isName && nt.Arg == nil {
		r, found := a.peek(nt.Name, sc)
		if !found {
			a.resolve(nt.Name, sc, acState|acOp|acTypePos) // 名前が見つからないことを言う
			return "", nil, false
		}
		if r.kind == rDecl {
			named = r.sym
		}
	}
	switch subj := c.Subject.(type) {
	case ir.OpSubject:
		if named == nil || named.kind != kindOp || named.fqn != subj.Op {
			return notSubject()
		}
		a.resolve(nt.Name, sc, acOp) // 参照を記録する
		return "", subj.Type, true
	case ir.StatesSubject:
		if named == nil || named.kind != kindState || !contains(subj.States, named.fqn) {
			return notSubject()
		}
		a.resolve(nt.Name, sc, acState)
		st, _ := a.lower(named).(*ir.State)
		if st == nil {
			return named.fqn, nil, true
		}
		return named.fqn, st.Type, true
	case ir.TypeSubject:
		if named != nil && symAccept(named)&acTypePos == 0 {
			return notSubject()
		}
		t, tok := a.lowerType(ct.Subject, sc)
		if !tok {
			return "", nil, false
		}
		if !ir.TypeEqual(t, subj.Type) {
			return notSubject()
		}
		return "", subj.Type, true
	}
	return "", nil, false
}

// subjectText は分類の対象の表示（診断用）。
func subjectText(c *ir.Classify) string {
	switch subj := c.Subject.(type) {
	case ir.OpSubject:
		return "対象は op " + subj.Op
	case ir.StatesSubject:
		return "対象は状態 " + join(subj.States)
	case ir.TypeSubject:
		return "対象は型 " + typeName(subj.Type)
	}
	return "対象が決まっていない"
}

// narrowable は docs/types.md「絞り込める型」: 対象の型 S を T に絞り込めるか。T が S に渡せるか、S から optional を
// 外した型が union（arms）への ref で、T がそのアームのどれかの型と等しい。
func (a *analyzer) narrowable(s, t ir.Type) bool {
	if a.assignType(t, s) {
		return true
	}
	return a.armOf(stripOptional(s), t)
}

// unionArms は t が union（arms）への ref なら、そのユニオン。そうでなければ nil。
func (a *analyzer) unionArms(t ir.Type) *ir.Union {
	r, ok := t.(ir.RefType)
	if !ok {
		return nil
	}
	sym := a.syms[r.Target]
	if sym == nil || sym.kind != kindUnion {
		return nil
	}
	un, ok := a.lower(sym).(*ir.Union)
	if !ok || un.Variant != ir.UnionArms {
		return nil
	}
	return un
}

// armOf は u が union（arms）への ref で、t がそのアームのどれかの型と等しいか。
func (a *analyzer) armOf(u, t ir.Type) bool {
	un := a.unionArms(u)
	if un == nil {
		return false
	}
	for _, arm := range un.Arms {
		if arm.Type != nil && ir.TypeEqual(arm.Type, t) {
			return true
		}
	}
	return false
}

// narrowBy は絞り込みの種類（docs/types.md「型で決まる絞り込み」）: S が optional(T) なら presence、S が union（arms）への
// ref で T がそのアームの型なら variant、それ以外は predicate（述語の要る絞り込み）。
func (a *analyzer) narrowBy(s, t ir.Type) ir.NarrowBy {
	if o, ok := s.(ir.OptionalType); ok && ir.TypeEqual(o.Of, t) {
		return ir.NarrowByPresence
	}
	if a.armOf(s, t) {
		return ir.NarrowByVariant
	}
	return ir.NarrowByPredicate
}

// decidedBy は docs/check.md「ケースの判定」: 型で決まる分類の条件 1〜3 をすべて満たせば type、そうでなければ predicate。
// 残りを受けるケースがなくても、型で決まる（どのケースの型にも当たらない値は、分類の誤り。同「当たらない値」）。
// c.Cases の as は、確かめた後のもの（lowerCaseTypings、lowerElseCase）。
func (a *analyzer) decidedBy(c *ir.Classify) ir.DecidedBy {
	// 1. 対象が一つ
	state := ""
	var subjectType ir.Type
	switch subj := c.Subject.(type) {
	case ir.TypeSubject:
		subjectType = subj.Type
	case ir.OpSubject:
		subjectType = subj.Type
	case ir.StatesSubject:
		if len(subj.States) != 1 {
			return ir.DecidedByPredicate
		}
		state = subj.States[0]
		if sym := a.syms[state]; sym != nil {
			if st, ok := a.lower(sym).(*ir.State); ok {
				subjectType = st.Type
			}
		}
	default:
		return ir.DecidedByPredicate
	}
	if len(c.Cases) == 0 || subjectType == nil {
		return ir.DecidedByPredicate
	}
	// 2. すべてのケースがその対象の as を持ち、型を書いた as は、どれも型で決まる絞り込み。3. as に書いた型がケースの間で
	// すべて違う。残りのケース（as else）は、型を書かないので比べない（確かめてあるのは lowerElseCase）
	var types []ir.Type
	for _, cs := range c.Cases {
		var as *ir.CaseTyping
		for i := range cs.As {
			if cs.As[i].State == state {
				as = &cs.As[i]
			}
		}
		if as == nil || as.By == ir.NarrowByPredicate {
			return ir.DecidedByPredicate
		}
		if as.By == ir.NarrowByElse || as.By == ir.NarrowByNone {
			continue
		}
		for _, t := range types {
			if ir.TypeEqual(t, as.Type) {
				return ir.DecidedByPredicate
			}
		}
		types = append(types, as.Type)
	}
	return ir.DecidedByType
}

// subjectKind は classify の対象の一つが何を指しているか。
type subjectKind int

const (
	// subjectType は型（として下ろすもの）。型として受け付けない名前は lowerType が RES-* で報告する。
	subjectType subjectKind = iota
	// subjectState は基底の状態。
	subjectState
	// subjectStateClassify は状態を対象にした classify（状態の分類）。
	subjectStateClassify
	// subjectOp は op。
	subjectOp
	// subjectUnresolved は解決できない名前。RES-* として報告する。
	subjectUnresolved
)

// lowerClassifySubject は classify / condition の対象（docs/resolve.md「位置ごとに受け付ける種類」の
// 「classify C of ... の対象」）。一つで型を指せば値の分類、一つで op を指せば op の分類（値を返さない op は CLS-4）、
// すべてが現在の名前空間の state なら状態の分類。それ以外は CLS-3（docs/check.md「classify / match」）で、対象は
// 決まらない（nil）。同じ状態が二度あるだけなら、重複を除いた状態の分類にする。
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
	if len(of) == 1 {
		switch kinds[0] {
		case subjectType, subjectUnresolved:
			if t, ok := a.lowerType(of[0], sc); ok {
				return ir.TypeSubject{Type: t}
			}
			return nil
		case subjectOp:
			return a.lowerOpSubject(s, of[0].(*ast.NamedType), sc)
		}
	}
	// ここからは状態の分類として読む。状態でないもの、別の名前空間の状態、状態の分類、重複は CLS-3。
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
			a.errorf("CLS-3", span, "classify %s の対象 %s は状態の分類（対象にできるのは宣言した名前空間の基底の状態だけ。docs/check.md「状態の形」）", s.fqn, syms[i].fqn)
			ok = false
		case subjectType, subjectOp:
			what := "型"
			if kinds[i] == subjectOp {
				what = "op"
			}
			if states > 0 {
				a.errorf("CLS-3", span, "classify %s の対象に%sと状態が混ざっている（%s は状態ではない）", s.fqn, what, a.text(span))
			} else {
				a.errorf("CLS-3", span, "classify %s の対象が二つ以上あるのに、%s は状態ではない（値の分類の対象は型一つ、op の分類の対象は op 一つ）", s.fqn, a.text(span))
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

// lowerOpSubject は op の分類の対象（docs/check.md「分類」の「対象」）。どの名前空間の op でもよく、export は要らない
// （対象にすることは呼び出しではない）。CLS-4: 値を返さない op（returns が null か never）。
// 対象の型（その op の returns）は op の宣言の構文から読む: op の引数や戻り値の型が、この分類のケースで絞り込んだ型
// （C.Case<T>）を使っていても、互いを下ろし合わないようにするため。
func (a *analyzer) lowerOpSubject(s *symbol, nt *ast.NamedType, sc *scope) ir.ClassifySubject {
	r, ok := a.resolve(nt.Name, sc, acOp)
	if !ok {
		return nil
	}
	od, isOp := r.sym.node.(*ast.OpDecl)
	if !isOp {
		return nil
	}
	if od.Return == nil || od.Return.Void || od.Return.Never {
		a.errorf("CLS-4", nt.Name.Span, "classify %s の対象の op %s は値を返さない（分類できるのは、その op が返した値）", s.fqn, r.sym.fqn)
		return nil
	}
	if op, lowered := a.lower(r.sym).(*ir.Op); lowered {
		if op.Returns == nil {
			return nil // 戻り値の型が解決できなかった（RES-* で報告済み）
		}
		return ir.OpSubject{Op: op.Name, Type: op.Returns}
	}
	// op を下ろしている途中（その op の型が、この分類を使っている）。戻り値の型だけを、診断を重ねずに読む。
	t, ok := a.quietType(od.Return.Type, &scope{ns: r.sym.ns})
	if !ok {
		return nil
	}
	return ir.OpSubject{Op: r.sym.fqn, Type: t}
}

// quietType は型を下ろすが、診断も参照も残さない（同じ型をほかの場所が下ろして報告する）。
func (a *analyzer) quietType(t ast.Type, sc *scope) (ir.Type, bool) {
	diags, refs, pending := len(a.diags), len(a.refs), len(a.pendingCaseTypes)
	out, ok := a.lowerType(t, sc)
	a.diags, a.refs, a.pendingCaseTypes = a.diags[:diags], a.refs[:refs], a.pendingCaseTypes[:pending]
	return out, ok
}

// subjectKindOf は対象の一つを、診断も参照も残さずに見分ける。型引数のない名前が state を指せば状態、op を指せば op、
// 状態を対象にした classify を指せば状態の分類、解決できなければ subjectUnresolved、それ以外は型。
// 形の語や ? / [] を持つ型は、型として読む。
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
	case kindOp:
		return subjectOp, r.sym
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
	op.Exported = d.Export
	switch {
	case d.Return == nil || d.Return.Void:
	case d.Return.Never:
		op.Returns = ir.Never()
		a.neverOps = append(a.neverOps, &neverOp{sym: s, decl: d, op: op})
	default:
		if t, ok := a.lowerType(d.Return.Type, sc); ok {
			op.Returns = t
		}
	}
	for _, c := range d.Clauses {
		switch c := c.(type) {
		case *ast.EffectsClause:
			a.checkSTATE5(s, d, c.Span, "effects")
			op.Effects = append(op.Effects, a.lowerOpEffects(c, sc)...)
		case *ast.ReadsClause:
			a.checkSTATE5(s, d, c.Span, "reads")
			for _, r := range a.lowerReads(c, sc) {
				if !containsRef(op.Reads, r) {
					op.Reads = append(op.Reads, r)
				}
			}
		case *ast.ThrowsClause:
			op.Throws = append(op.Throws, a.lowerThrows(c, sc))
		}
	}
	a.checkREL1(s.fqn, op.Throws, d.Span)
	if site := a.addSite(d.Decorators, siteOp, &scope{ns: s.ns, opParams: &opParamScope{decl: s.fqn, params: params}}, &op.DeclBase.Decorators); site != nil {
		site.op = &opSite{sym: s, decl: d, op: op}
	}
	if d.Body != nil {
		a.wirings = append(a.wirings, &wiringJob{sym: s, decl: d, op: op})
	}
	return op
}

// checkSTATE5 は STATE-5（docs/check.md「状態と効果」）: 葉の op（本体を持たない op）が reads か effects の節を書いた。
// 葉は人が中身を書く op で、状態に触れない（同「葉の op」）。節ごとに一つ。位置はその節。
func (a *analyzer) checkSTATE5(s *symbol, d *ast.OpDecl, span source.Span, clause string) {
	if d.Body != nil {
		return
	}
	what := "read を"
	if clause == "effects" {
		what = "効果（effect）を"
	}
	a.errorf("STATE-5", span, "葉の op %s は %s の節を書けない（葉は状態に触れない。%s宣言して、配線で呼ぶ。docs/check.md「葉の op」）", s.fqn, clause, what)
}

// lowerOpEffects は op の effects の一節（docs/ir.md「op」の effects）。要素は、名前で書いた効果（effect か、効果の union。
// 展開する）か、直に書いた「状態.操作」（docs/check.md「節に書く要素」）。どの名前空間のものでも書ける（配線の効果は、
// 呼び出し先から名前空間をまたいで伝わる）。本体から決まる効果との照合（EFF-2）は internal/check が行う。
func (a *analyzer) lowerOpEffects(c *ast.EffectsClause, sc *scope) []ir.EffectRef {
	var out []ir.EffectRef
	for _, n := range c.Effects {
		r, ok := a.resolve(n, sc, acEffectsPos)
		if !ok {
			continue
		}
		out = append(out, a.effectRefs(r, n)...)
	}
	return out
}

// effectRefs は effects の要素 n の解決の結果 r を EffectRef の列にする: effect は名前で書いた要素一つ、効果の union は
// 展開したメンバー、「状態.操作」は直に書いた要素一つ（操作がその状態の形になければ STATE-1。位置は操作の語）。
func (a *analyzer) effectRefs(r resolved, n ast.QualifiedName) []ir.EffectRef {
	if r.kind == rStateOp {
		a.checkSTATE1(r.sym, r.op, n.Last().Span)
		return []ir.EffectRef{ir.DirectEffect{State: r.sym.fqn, Operation: r.op}}
	}
	return ir.NamedEffects(a.expandSet(r, kindEffect, n.Span)...)
}

// effectOwner は効果 e の持ち主の名前空間: 名前で書いた効果ならその宣言の名前空間、直に書いた効果ならその状態の名前空間。
// 宣言が表になければ nil。
func (a *analyzer) effectOwner(e ir.EffectRef) *namespace {
	fqn := ""
	switch x := e.(type) {
	case ir.NamedEffect:
		fqn = x.Effect
	case ir.DirectEffect:
		fqn = x.State
	}
	if sym := a.syms[fqn]; sym != nil {
		return sym.ns
	}
	return nil
}

// lowerReads は reads の一節（docs/ir.md「op」の reads）。要素は、名前で書いた read か、直に書いた state（基底の状態）。
// どちらも、どの名前空間のものでも解決する（どちらでもなければ RES-4）。本体から決まる読む状態との照合（READS-2）は
// internal/check が行う。
func (a *analyzer) lowerReads(c *ast.ReadsClause, sc *scope) []ir.ReadRef {
	var out []ir.ReadRef
	for _, n := range c.Reads {
		r, ok := a.resolve(n, sc, acReadsPos)
		if !ok {
			continue
		}
		out = append(out, readRef(r.sym))
	}
	return out
}

// readRef は reads の要素が指す宣言 sym（read か state）を ReadRef にする。
func readRef(sym *symbol) ir.ReadRef {
	if sym.kind == kindRead {
		return ir.NamedRead{Read: sym.fqn}
	}
	return ir.DirectRead{State: sym.fqn}
}

// readRefText は診断のメッセージの中の言い方（「状態 a.s」か「read a.r」。前後に空白を持つ）。
func readRefText(r ir.ReadRef) string {
	if _, ok := r.(ir.NamedRead); ok {
		return " read " + r.String() + " "
	}
	return "状態 " + r.String() + " "
}

// containsRef は列 list に x があるか。
func containsRef[T comparable](list []T, x T) bool {
	for _, y := range list {
		if y == x {
			return true
		}
	}
	return false
}

// neverOp は never を返す op。デコレータの適用が下りてから確かめる（THROWS-3 の条件 6）。
type neverOp struct {
	sym  *symbol
	decl *ast.OpDecl
	op   *ir.Op
}

// checkNeverOps は THROWS-3（docs/check.md「throws」）: never を返す op が、docs/check.md「never を返す op」の条件 1〜6 の
// どれかを満たさない。満たさない条件ごとに一つ。位置は、1 は op の名前、2 と 3 は throws（なければ op の名前）、
// 4 は最初の引数、5 と 6 はそのデコレータ。
func (a *analyzer) checkNeverOps() {
	for _, n := range a.neverOps {
		a.checkTHROWS3(n)
	}
}

func (a *analyzer) checkTHROWS3(n *neverOp) {
	d, op, name := n.decl, n.op, n.sym.fqn
	// 1. 葉である
	if d.Body != nil {
		a.errorf("THROWS-3", n.sym.nameSpan, "never を返す op %s は本体を持てない（決して返さない op は、例外を一つ投げる葉）", name)
	}
	// 2. throws の例外（ユニオンを展開した後）が、ちょうど一つ。3. その例外が引数を持たない
	var throws *ast.ThrowsClause
	for _, c := range d.Clauses {
		if tc, ok := c.(*ast.ThrowsClause); ok && throws == nil {
			throws = tc
		}
	}
	throwsSpan := n.sym.nameSpan
	if throws != nil {
		throwsSpan = throws.Span
	}
	var exceptions []string
	for _, tc := range op.Throws {
		for _, e := range tc.Exceptions {
			if !contains(exceptions, e) {
				exceptions = append(exceptions, e)
			}
		}
	}
	if len(exceptions) != 1 {
		if len(exceptions) == 0 {
			a.errorf("THROWS-3", throwsSpan, "never を返す op %s は、throws に例外をちょうど一つ書く（投げる例外がない）", name)
		} else {
			a.errorf("THROWS-3", throwsSpan, "never を返す op %s は、throws に例外をちょうど一つ書く（%s）", name, join(exceptions))
		}
	}
	for _, e := range exceptions {
		if sym := a.syms[e]; sym != nil {
			if ex, ok := a.lower(sym).(*ir.Exception); ok && len(ex.Params) > 0 {
				a.errorf("THROWS-3", throwsSpan, "never を返す op %s の例外 %s は引数を持つ（引数を持つ例外は、配線のアームで値を渡して投げる: throw %s(…)）", name, e, e)
				break
			}
		}
	}
	// 4. 引数を持たない
	if len(d.Params) > 0 && d.Params[0] != nil {
		a.errorf("THROWS-3", d.Params[0].Span, "never を返す op %s は引数を持てない", name)
	}
	// 5. デコレータが運ぶ reads も effects も持たない（節に書いたものは、葉なので STATE-5）
	if carriers := a.carriers[op]; len(carriers) > 0 {
		a.errorf("THROWS-3", carriers[0].span, "never を返す op %s は、reads も effects も運ぶデコレータを持てない", name)
	}
	// 6. catches を持つデコレータが付いていない
	for _, app := range op.Decorators {
		sym := a.syms[app.Decorator]
		if sym == nil {
			continue
		}
		dec, ok := a.lower(sym).(*ir.Decorator)
		if !ok || len(dec.Catches) == 0 {
			continue
		}
		span := n.sym.nameSpan
		if ds := a.decoratorSpan(d.Decorators, app); ds != (source.Span{}) {
			span = ds
		}
		a.errorf("THROWS-3", span, "never を返す op %s に、catches を持つデコレータ %s は付けられない", name, app.Decorator)
		break
	}
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
