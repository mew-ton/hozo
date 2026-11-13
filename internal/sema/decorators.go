package sema

import (
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// siteKind はデコレータを付けた先の種類（DEC-1 の照合に使う）。
type siteKind int

const (
	siteOp siteKind = iota
	siteField
	siteState
	siteException
	siteScalar
	siteModel
	siteUnion
	siteEffect
	siteClassify
	siteDecorator
	siteNamespace
)

// String は設計書の語。
func (k siteKind) String() string {
	return [...]string{"op", "field", "state", "exception", "scalar", "model", "union", "effect", "classify", "decorator", "namespace"}[k]
}

// target は対応する Decorator.targets の値。付けられない種類なら ok が false。
func (k siteKind) target() (ir.DecoratorTarget, bool) {
	switch k {
	case siteOp:
		return ir.TargetOp, true
	case siteField:
		return ir.TargetField, true
	case siteState:
		return ir.TargetState, true
	case siteException:
		return ir.TargetException, true
	case siteScalar:
		return ir.TargetScalar, true
	case siteModel:
		return ir.TargetModel, true
	case siteNamespace:
		return ir.TargetNamespace, true
	}
	return "", false
}

// appSite はデコレータの適用を下ろす場所。宣言の表がそろってから（lowerApplications）下ろす。
type appSite struct {
	decos []*ast.Decorator
	kind  siteKind
	sc    *scope
	out   *[]ir.Application
	// seen は DEC-13 の照合に使う、この宣言に付いたデコレータ → 最初の適用の位置。nil なら場所ごとに作る。
	// 名前空間は複数のブロックで一つの宣言なので（docs/ir.md「正規化」）、ブロックをまたいで同じ表を使う。
	seen map[string]source.Span
}

func (a *analyzer) addSite(decos []*ast.Decorator, kind siteKind, sc *scope, out *[]ir.Application) {
	if len(decos) == 0 {
		return
	}
	a.sites = append(a.sites, &appSite{decos: decos, kind: kind, sc: sc, out: out})
}

// lowerApplications はすべての適用を下ろす（docs/ir.md「デコレータの適用」）。並びは書かれた順。
func (a *analyzer) lowerApplications() {
	for _, site := range a.sites {
		seen := site.seen
		if seen == nil {
			seen = map[string]source.Span{}
		}
		for _, d := range site.decos {
			app, ok := a.lowerApplication(d, site)
			if !ok {
				continue
			}
			a.checkDEC13(app, d, seen)
			*site.out = append(*site.out, app)
		}
	}
}

// checkDEC13 は DEC-13（docs/check.md「デコレータ」）: 同じデコレータが同じ宣言に二度付いている（引数が違っても）。
// 名前付きデコレータは展開後の汎用デコレータで比べる。
func (a *analyzer) checkDEC13(app ir.Application, d *ast.Decorator, seen map[string]source.Span) {
	if first, dup := seen[app.Decorator]; dup {
		a.errorf("DEC-13", d.Name.Span, "デコレータ %s が同じ宣言に二度付いている", app.Decorator).Related = append([]diag.Related{}, related(first, "最初の適用"))
		return
	}
	seen[app.Decorator] = d.Name.Span
}

// lowerApplication は一つの適用。名前付きデコレータは汎用デコレータと固定した引数に展開する。
func (a *analyzer) lowerApplication(d *ast.Decorator, site *appSite) (ir.Application, bool) {
	r, ok := a.resolve(d.Name, site.sc, acDecorator)
	if !ok {
		return ir.Application{}, false
	}
	dec, ok := a.lower(r.sym).(*ir.Decorator)
	if !ok {
		return ir.Application{}, false
	}
	app := ir.Application{Decorator: dec.Name, Span: a.irSpan(d.Span)}
	generic := dec
	if dec.Alias != nil {
		generic, ok = a.aliasTarget(dec)
		if !ok {
			return ir.Application{}, false
		}
		app.Decorator, app.Via = generic.Name, dec.Name
		app.Args = append([]ir.Arg{}, dec.Alias.Args...)
		if len(d.Args) > 0 {
			a.errorf("DEC-11", d.Span, "名前付きデコレータ %s は引数を固定しているので、引数を渡せない", dec.Name)
		}
	} else {
		app.Args = a.lowerArgs(d.Args, dec, site.sc, d.Span, "DEC-11")
	}
	a.checkDEC1(generic, site.kind, d)
	return app, true
}

// aliasTarget は名前付きデコレータの汎用デコレータ。別名の別名（DEC-12）なら ok は false。
func (a *analyzer) aliasTarget(dec *ir.Decorator) (*ir.Decorator, bool) {
	s := a.syms[dec.Alias.Target]
	if s == nil {
		return nil, false
	}
	g, ok := a.lower(s).(*ir.Decorator)
	if !ok || g.Alias != nil {
		return nil, false
	}
	return g, true
}

// checkDEC1 は DEC-1（docs/check.md「デコレータ」）: 付けた先の種類が targets にない。
// union / effect / classify / decorator に付けたら常に DEC-1。namespace に付けられるのは `for namespace` の
// デコレータだけ（bind は文法で閉じる）。
func (a *analyzer) checkDEC1(dec *ir.Decorator, kind siteKind, d *ast.Decorator) {
	t, ok := kind.target()
	if ok {
		for _, want := range dec.Targets {
			if want == t {
				return
			}
		}
	}
	a.errorf("DEC-1", d.Name.Span, "デコレータ %s は %s に付けられない（for: %s）", dec.Name, kind, targetsString(dec.Targets))
}

func targetsString(ts []ir.DecoratorTarget) string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = string(t)
	}
	return join(names)
}

// --- 引数（docs/types.md「デコレータの引数」） ---

// lowerArgs は適用の引数を Param に対応付ける。数と名前の違反は code（適用なら DEC-11、別名なら DEC-12）。
func (a *analyzer) lowerArgs(args []*ast.Arg, dec *ir.Decorator, sc *scope, span source.Span, code string) []ir.Arg {
	var out []ir.Arg
	given := map[string]bool{}
	for i, arg := range args {
		if arg == nil || arg.Value == nil {
			continue
		}
		p, ok := a.paramFor(arg, i, dec, code)
		if !ok {
			continue
		}
		if given[p.Name] {
			a.errorf(code, arg.Span, "デコレータ %s の引数 %s に二度渡している", dec.Name, p.Name)
			continue
		}
		given[p.Name] = true
		if v, ok := a.lowerArgValue(arg.Value, p, sc); ok {
			out = append(out, ir.Arg{Param: p.Name, Value: v})
		}
	}
	for _, p := range dec.Params {
		if !p.Optional && !given[p.Name] {
			a.errorf(code, span, "デコレータ %s の必須の引数 %s が渡されていない", dec.Name, p.Name)
		}
	}
	return out
}

// paramFor は位置か名前で Param を引く。
func (a *analyzer) paramFor(arg *ast.Arg, index int, dec *ir.Decorator, code string) (ir.Param, bool) {
	if arg.Name != nil {
		for _, p := range dec.Params {
			if p.Name == arg.Name.Name {
				return p, true
			}
		}
		a.errorf(code, arg.Name.Span, "デコレータ %s に引数 %s はない", dec.Name, arg.Name.Name)
		return ir.Param{}, false
	}
	if index < len(dec.Params) {
		return dec.Params[index], true
	}
	a.errorf(code, arg.Span, "デコレータ %s の引数は %d 個で、位置で渡した引数が多い", dec.Name, len(dec.Params))
	return ir.Param{}, false
}

// lowerArgValue は引数の値を Param.type に合わせて下ろす。
func (a *analyzer) lowerArgValue(v *ast.ArgValue, p ir.Param, sc *scope) (ir.ArgValue, bool) {
	switch pt := p.Type.(type) {
	case ir.AnyParam:
		if v.IsList {
			a.errorf("TYPE-4", v.Span, "引数 %s（any %s）にリストは渡せない", p.Name, pt.Of)
			return nil, false
		}
		return a.lowerAnyArg(v, p, pt, &scope{ns: sc.ns, decoParams: sc.decoParams})
	case ir.TypeParam:
		if u := a.unionParam(pt); u != nil {
			return a.lowerUnionArg(v, p, u, &scope{ns: sc.ns})
		}
		return a.lowerTypeArg(v, p, pt, sc)
	}
	return nil, false
}

// unionParam は Param.type が set のユニオン一つへの ref（`error: ApiError`）ならそのユニオン。
func (a *analyzer) unionParam(pt ir.TypeParam) *ir.Union {
	if len(pt.Types) != 1 {
		return nil
	}
	r, ok := pt.Types[0].(ir.RefType)
	if !ok {
		return nil
	}
	s := a.syms[r.Target]
	if s == nil || s.kind != kindUnion || s.unionVariant != ir.UnionSet {
		return nil
	}
	u, _ := a.lower(s).(*ir.Union)
	return u
}

// lowerAnyArg は `any ...` の引数: 宣言への参照。exception / effect は `|` と union の展開で refs になる。
func (a *analyzer) lowerAnyArg(v *ast.ArgValue, p ir.Param, pt ir.AnyParam, sc *scope) (ir.ArgValue, bool) {
	if len(v.Terms) > 1 && pt.Of != ir.AnyException && pt.Of != ir.AnyEffect {
		a.errorf("TYPE-4", v.Span, "引数 %s（any %s）に `|` の集合は渡せない（集合になるのは例外と効果だけ）", p.Name, pt.Of)
		return nil, false
	}
	var targets []string
	sawUnion := false
	for _, term := range v.Terms {
		if term.Literal != nil {
			a.errorf("TYPE-4", term.Span, "引数 %s（any %s）にリテラルは渡せない", p.Name, pt.Of)
			return nil, false
		}
		if term.Name == nil {
			continue
		}
		r, ok := a.resolve(*term.Name, sc, anyAccept(pt.Of))
		if !ok {
			return nil, false
		}
		if pt.Of == ir.AnyVariant {
			return ir.VariantValue{Union: r.sym.fqn, Arm: r.name}, true
		}
		if r.kind == rDecl && r.sym.kind == kindUnion {
			// set のユニオンは展開して refs にする（docs/types.md「デコレータの引数」）
			sawUnion = true
			targets = append(targets, a.expandSet(r, kindOf(pt.Of), term.Span)...)
			continue
		}
		targets = append(targets, r.sym.fqn)
	}
	if len(targets) == 0 {
		return nil, false
	}
	if pt.On != nil {
		a.checkEffectOn(targets, pt, p, v.Span)
	}
	if len(v.Terms) == 1 && !sawUnion {
		return ir.RefValue{Target: targets[0]}, true
	}
	return ir.RefsValue{Targets: targets}, true
}

// anyAccept は any の種類が受け付ける解決先。exception / effect は set の union も。
func anyAccept(of ir.AnyOf) accept {
	switch of {
	case ir.AnyException:
		return acException | acUnionSet
	case ir.AnyEffect:
		return acEffect | acUnionSet
	case ir.AnyState:
		return acState
	case ir.AnyOp:
		return acOp
	case ir.AnyModel:
		return acModel
	case ir.AnyVariant:
		return acVariant
	}
	return 0
}

func kindOf(of ir.AnyOf) declKind {
	if of == ir.AnyEffect {
		return kindEffect
	}
	return kindException
}

// checkEffectOn は DEC-7（docs/types.md「デコレータの引数」）: `any effect on s.op` が受け付けるのは、宛先が
// 組み込みの効果で、その state が s、operation が op のものだけ。遷移の効果は、書き込む状態が同じでも受け付けない
// （操作が宣言から決まらない）。
func (a *analyzer) checkEffectOn(targets []string, pt ir.AnyParam, p ir.Param, span source.Span) {
	for _, t := range targets {
		s := a.syms[t]
		if s == nil {
			continue
		}
		e, ok := a.lower(s).(*ir.Effect)
		if !ok {
			continue
		}
		switch target := e.Target.(type) {
		case ir.BuiltinTarget:
			if target.State != pt.On.State || target.Operation != pt.On.Operation {
				a.errorf("DEC-7", span, "引数 %s は %s.%s への効果だけを受け付けるが、%s は %s.%s への効果", p.Name, pt.On.State, pt.On.Operation, t, target.State, target.Operation)
			}
		case ir.TransitionTarget:
			a.errorf("DEC-7", span, "引数 %s は %s.%s への効果だけを受け付けるが、%s は %s の遷移", p.Name, pt.On.State, pt.On.Operation, t, target.Of)
		}
	}
}

// lowerUnionArg は Param.type が set のユニオンへの ref のとき: そのユニオンのメンバーに含まれる例外か効果（DEC-7）。
func (a *analyzer) lowerUnionArg(v *ast.ArgValue, p ir.Param, u *ir.Union, sc *scope) (ir.ArgValue, bool) {
	if v.IsList {
		a.errorf("TYPE-4", v.Span, "引数 %s（%s）にリストは渡せない", p.Name, u.Name)
		return nil, false
	}
	var targets []string
	for _, term := range v.Terms {
		if term.Name == nil {
			a.errorf("TYPE-4", term.Span, "引数 %s（%s）にリテラルは渡せない", p.Name, u.Name)
			return nil, false
		}
		r, ok := a.resolve(*term.Name, sc, acException|acEffect|acUnionSet)
		if !ok {
			return nil, false
		}
		for _, m := range a.expandMember(r.sym, r, term.Span) {
			if !unionHas(u, m.fqn) {
				a.errorf("DEC-7", term.Span, "%s は union %s のメンバーではない（引数 %s）", m.fqn, u.Name, p.Name)
				continue
			}
			targets = append(targets, m.fqn)
		}
	}
	if len(targets) == 0 {
		return nil, false
	}
	if len(targets) == 1 {
		return ir.RefValue{Target: targets[0]}, true
	}
	return ir.RefsValue{Targets: targets}, true
}

func unionHas(u *ir.Union, fqn string) bool {
	for _, m := range u.Members {
		if m.Target == fqn {
			return true
		}
	}
	return false
}

// lowerTypeArg は型で受ける引数: リテラル（型が渡せること。scalar の位置は TYPE-1）、`[ ]` は array の位置に、
// op の引数名なら `$param:名前`（手続き 1.3）で、その引数の型が渡せること。
func (a *analyzer) lowerTypeArg(v *ast.ArgValue, p ir.Param, pt ir.TypeParam, sc *scope) (ir.ArgValue, bool) {
	if v.IsList {
		return a.lowerListArg(v, p, pt, sc)
	}
	if len(v.Terms) != 1 {
		a.errorf("TYPE-4", v.Span, "引数 %s は型 %s で受けるので、`|` の集合は渡せない", p.Name, typesString(pt.Types))
		return nil, false
	}
	term := v.Terms[0]
	if term.Literal != nil {
		return a.lowerLiteralArg(term.Literal, p, pt)
	}
	if term.Name == nil {
		return nil, false
	}
	r, ok := a.resolve(*term.Name, sc, acOpParam)
	if !ok {
		return nil, false
	}
	info := sc.opParams.params[r.name]
	if info.typ != nil && !a.anyAssignable(valueType(info.typ), pt.Types) {
		a.errorf("TYPE-4", term.Span, "op の引数 %s の型 %s は、デコレータの引数 %s の型 %s に渡せない", r.name, typeName(info.typ), p.Name, typesString(pt.Types))
		return nil, false
	}
	return ir.RefValue{Target: "$param:" + r.name}, true
}

// lowerListArg は `[ ... ]`: array の引数にだけ渡せ、各要素は要素の型で受ける。
func (a *analyzer) lowerListArg(v *ast.ArgValue, p ir.Param, pt ir.TypeParam, sc *scope) (ir.ArgValue, bool) {
	var elems []ir.Type
	for _, t := range pt.Types {
		if arr, ok := t.(ir.ArrayType); ok {
			elems = append(elems, arr.Of)
		}
	}
	if len(elems) == 0 {
		a.errorf("TYPE-4", v.Span, "引数 %s の型 %s は array ではないので、リストは渡せない", p.Name, typesString(pt.Types))
		return nil, false
	}
	elemParam := ir.Param{Name: p.Name, Type: ir.TypeParam{Types: elems}}
	list := ir.ListValue{}
	for _, item := range v.List {
		if iv, ok := a.lowerArgValue(item, elemParam, sc); ok {
			list.Items = append(list.Items, iv)
		}
	}
	return list, true
}

// lowerLiteralArg はリテラルの引数。
func (a *analyzer) lowerLiteralArg(l *ast.Literal, p ir.Param, pt ir.TypeParam) (ir.ArgValue, bool) {
	if len(pt.Types) == 0 {
		return nil, false // 引数の型が解決できていない（報告済み）
	}
	lit := literalType(l.Kind)
	fail := failType
	for _, t := range pt.Types {
		f := a.assign(lit, t)
		if f == assignOK {
			return ir.LiteralValue{Literal: a.lowerLiteral(l)}, true
		}
		if f == failScalar {
			fail = failScalar
		}
	}
	a.errorf(string(fail), l.Span, "%s", failMessage(fail, "引数 "+p.Name, lit, pt.Types[0]))
	return nil, false
}

// anyAssignable は s が types のどれかに渡せるか。
func (a *analyzer) anyAssignable(s exprType, types []ir.Type) bool {
	for _, t := range types {
		if a.assign(s, t) == assignOK {
			return true
		}
	}
	return false
}

func typesString(ts []ir.Type) string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = typeName(t)
	}
	return join(names)
}

// --- デコレータの宣言（docs/ir.md「decorator」） ---

// lowerDecoratorDecl は decorator の宣言。名前付き（別名）なら Alias だけを持つ。
func (a *analyzer) lowerDecoratorDecl(s *symbol, d *ast.DecoratorDecl, sc *scope) ir.Decl {
	dec := &ir.Decorator{External: d.External}
	if d.Alias != nil {
		dec.DeclBase = a.base(s, d.Doc, d.Span, docContext{})
		a.addSite(d.Decorators, siteDecorator, sc, &dec.DeclBase.Decorators)
		dec.Alias = a.lowerAlias(s, d.Alias, sc)
		return dec
	}
	params := &decoParamScope{decl: s.fqn, params: map[string]*ast.Param{}, types: map[string]ir.ParamType{}}
	for _, p := range d.Params {
		if p == nil || p.Type == nil {
			continue
		}
		param := a.lowerParam(p, sc)
		params.params[p.Name.Name] = p
		params.types[p.Name.Name] = param.Type
		dec.Params = append(dec.Params, param)
	}
	dec.DeclBase = a.base(s, d.Doc, d.Span, docContext{params: setOf(params.params), hasParams: true})
	a.addSite(d.Decorators, siteDecorator, sc, &dec.DeclBase.Decorators)
	dec.Targets = lowerTargets(d.For)
	csc := &scope{ns: s.ns, decoParams: params}
	for _, c := range d.Clauses {
		switch c := c.(type) {
		case *ast.ThrowsClause:
			dec.Throws = append(dec.Throws, a.lowerThrows(c, csc))
		case *ast.CatchesClause:
			dec.Catches = append(dec.Catches, a.lowerCatches(c, csc))
		}
	}
	a.checkREL1(s.fqn, dec.Throws, d.Span)
	a.checkDEC2(s, dec, d)
	return dec
}

func setOf(m map[string]*ast.Param) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}

// lowerParam は Param。`int?` は Types を int にして Optional を true にする。`any state?` も Optional。
func (a *analyzer) lowerParam(p *ast.Param, sc *scope) ir.Param {
	param := ir.Param{Name: p.Name.Name}
	if p.Type.Any != nil {
		param.Type = a.lowerAnyType(p.Type.Any, sc)
		param.Optional = p.Type.Optional
		return param
	}
	tp := ir.TypeParam{}
	for _, t := range p.Type.Types {
		if o, ok := t.(*ast.OptionalType); ok {
			param.Optional = true
			t = o.Elem
		}
		if lt, ok := a.lowerParamType(t, sc); ok {
			tp.Types = append(tp.Types, lt)
		}
	}
	param.Type = tp
	return param
}

// lowerParamType は Param の型。型の位置が受け付けるものに加えて、set のユニオンへの ref（`error: ApiError`。
// docs/types.md「デコレータの引数」）も受け付ける。
func (a *analyzer) lowerParamType(t ast.Type, sc *scope) (ir.Type, bool) {
	nt, ok := t.(*ast.NamedType)
	if !ok || nt.Arg != nil {
		return a.lowerType(t, sc)
	}
	r, ok := a.resolve(nt.Name, sc, acTypePos|acUnionSet)
	if !ok {
		return nil, false
	}
	if r.kind == rCase {
		a.errorf("RES-4", nt.Name.Span, "classify のケース %s を型にするには型引数が要る（%s<T>）", nt.Name.String(), nt.Name.String())
		return nil, false
	}
	return ir.Ref(r.sym.fqn), true
}

// lowerAnyType は `any 種類 [on 状態.操作]`。on の状態はどの名前空間のものでもよい。
func (a *analyzer) lowerAnyType(t *ast.AnyType, sc *scope) ir.ParamType {
	ap := ir.AnyParam{Of: ir.AnyOf(t.Kind.String())}
	if t.On != nil {
		if r, ok := a.resolve(t.On.State, sc, acState); ok {
			ap.On = &ir.EffectOn{State: r.sym.fqn, Operation: operationOf(t.On.Op)}
		}
	}
	return ap
}

// lowerTargets は for の対象。省略なら ["op"]（docs/ir.md「decorator」）。
func lowerTargets(for_ []*ast.Target) []ir.DecoratorTarget {
	if len(for_) == 0 {
		return []ir.DecoratorTarget{ir.TargetOp}
	}
	out := make([]ir.DecoratorTarget, 0, len(for_))
	for _, t := range for_ {
		if t != nil {
			out = append(out, ir.DecoratorTarget(t.Kind.String()))
		}
	}
	return out
}

// lowerCatches は catches の一節。halts の効果は effect か any effect の引数。
func (a *analyzer) lowerCatches(c *ast.CatchesClause, sc *scope) ir.CatchesClause {
	cc := ir.CatchesClause{Exceptions: a.lowerExceptionRefs(c.Exceptions, sc), Mode: ir.CatchMode(c.Outcome.String())}
	if c.Outcome == ast.Halts && c.Halts != nil {
		if r, ok := a.resolve(*c.Halts, sc, acHaltsPos); ok {
			if r.kind == rDecoParam {
				if a.checkAnyParam(sc, r.name, ir.AnyEffect, c.Halts.Span) {
					cc.Halts = "$param:" + r.name
				}
			} else {
				cc.Halts = r.sym.fqn
			}
		}
	}
	return cc
}

// checkDEC2 は DEC-2（docs/check.md「デコレータ」）: targets に op を含まない宣言が throws か catches を持つ。
func (a *analyzer) checkDEC2(s *symbol, dec *ir.Decorator, d *ast.DecoratorDecl) {
	if len(d.Clauses) == 0 {
		return
	}
	for _, t := range dec.Targets {
		if t == ir.TargetOp {
			return
		}
	}
	a.errorf("DEC-2", d.Clauses[0].NodeSpan(), "デコレータ %s は op に付けないので throws / catches を宣言できない（for: %s）", s.fqn, targetsString(dec.Targets))
}

// lowerAlias は名前付きデコレータ `decorator X = Y(args);`。DEC-12: Y が別名、または args が Y の Param に合わない。
func (a *analyzer) lowerAlias(s *symbol, al *ast.DecoratorAlias, sc *scope) *ir.DecoratorAlias {
	r, ok := a.resolve(al.Name, sc, acDecorator)
	if !ok {
		return nil
	}
	target, ok := a.lower(r.sym).(*ir.Decorator)
	if !ok || target.Alias != nil {
		a.errorf("DEC-12", al.Name.Span, "名前付きデコレータ %s の元 %s も名前付きデコレータ（別名の別名は作れない。設計書 §8.1）", s.fqn, r.sym.fqn)
		return nil
	}
	return &ir.DecoratorAlias{Target: target.Name, Args: a.lowerArgs(al.Args, target, sc, al.Span, "DEC-12")}
}
