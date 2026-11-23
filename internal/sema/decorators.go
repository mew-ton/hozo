package sema

import (
	"strings"

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
	siteRead
	siteClassify
	siteDecorator
	siteCase
)

// String は設計書の語。
func (k siteKind) String() string {
	return [...]string{"op", "field", "state", "exception", "scalar", "model", "union", "effect", "read", "classify", "decorator", "分類のケース"}[k]
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
	case siteUnion:
		return ir.TargetUnion, true
	case siteEffect:
		return ir.TargetEffect, true
	case siteRead:
		return ir.TargetRead, true
	case siteClassify:
		return ir.TargetClassify, true
	case siteDecorator:
		return ir.TargetDecorator, true
	case siteCase:
		return ir.TargetCase, true
	}
	return "", false
}

// appSite はデコレータの適用を下ろす場所。宣言の表がそろってから（lowerApplications）下ろす。
type appSite struct {
	decos []*ast.Decorator
	kind  siteKind
	sc    *scope
	out   *[]ir.Application
	// op は付けた先が op のとき、その op。デコレータが運ぶ reads / effects（Op.carried）を、適用を下ろしながら集める。
	op *opSite
	// caseOf は付けた先が分類のケースのとき、その分類。
	caseOf *ir.Classify
}

// opSite はデコレータを付けた先の op。
type opSite struct {
	sym  *symbol
	decl *ast.OpDecl
	op   *ir.Op
}

func (a *analyzer) addSite(decos []*ast.Decorator, kind siteKind, sc *scope, out *[]ir.Application) *appSite {
	if len(decos) == 0 {
		return nil
	}
	site := &appSite{decos: decos, kind: kind, sc: sc, out: out}
	a.sites = append(a.sites, site)
	return site
}

// lowerApplications はすべての適用を下ろす（docs/ir.md「デコレータの適用」）。並びは書かれた順。
// op に付けた適用は、そのデコレータの reads / effects の節が運ぶものを Op.carried に足す。
func (a *analyzer) lowerApplications() {
	for _, site := range a.sites {
		seen := map[string]source.Span{}
		for _, d := range site.decos {
			app, ok := a.lowerApplication(d, site)
			if !ok {
				continue
			}
			a.checkDEC13(app, d, seen)
			*site.out = append(*site.out, app)
			if site.op != nil {
				a.carry(site.op, app, d)
			}
		}
	}
}

// carrier は、reads か effects を運んだ適用の位置（THROWS-3 の条件 5 の位置に使う）。
type carrier struct {
	span source.Span
}

// carry は docs/check.md「デコレータが運ぶ reads と effects」: 適用 app のデコレータの reads / effects の節の要素を、
// `$param:` を適用の引数の値で置き換えて（残りの引数なら渡した値を書いた順にすべて、省いた省略可能な引数なら何も足さない）、
// Op.carried に足す。any effect の引数の値は名前で書いた効果に、any state の引数の値は直に書いた状態になる。
// 重複は最初の出現に寄せる。同じデコレータを何度も付けていれば（@repeatable）、適用ごとに足す。
// Op.reads / Op.effects には足さない（docs/ir.md「正規化」）。
//
// 付いた op は、葉でも配線でもよい。運ばれたものは、付いた op の名前空間のものでなければならない（どれも Signature を
// 要しない）。位置はその適用。
//   - READS-1: op に運ばれた状態（か、その read）が、別の名前空間のもの。
//   - STATE-2: op に運ばれた効果が、別の名前空間のもの。
func (a *analyzer) carry(site *opSite, app ir.Application, d *ast.Decorator) {
	sym := a.syms[app.Decorator]
	if sym == nil {
		return
	}
	dec, ok := a.lower(sym).(*ir.Decorator)
	if !ok || (len(dec.Reads) == 0 && len(dec.Effects) == 0) {
		return
	}
	op, name := site.op, site.sym.fqn
	carried := false
	for _, raw := range dec.Reads {
		for _, read := range carriedReads(raw, app.Args) {
			carried = true
			if containsRef(op.Carried.Reads, read) {
				continue
			}
			op.Carried.Reads = append(op.Carried.Reads, read)
			if rs := a.syms[read.String()]; rs != nil && rs.ns != site.sym.ns {
				a.errorf("READS-1", d.Span, "op %s に、デコレータ %s が別の名前空間の%sを reads として運んでいる（デコレータの処理が読めるのは、付いた op の名前空間 %s の状態だけ。別の名前空間の状態は、その名前空間の op を通して読む）", name, dec.Name, readRefText(read), nsName(site.sym.ns))
			}
		}
	}
	for _, raw := range dec.Effects {
		for _, effect := range carriedEffects(raw, app.Args) {
			carried = true
			if containsRef(op.Carried.Effects, effect) {
				continue
			}
			op.Carried.Effects = append(op.Carried.Effects, effect)
			if owner := a.effectOwner(effect); owner != nil && owner != site.sym.ns {
				a.errorf("STATE-2", d.Span, "op %s に、デコレータ %s が別の名前空間の効果 %s を運んでいる（%s は名前空間 %s の持ち主ではない）", name, dec.Name, effect, name, nsName(owner))
			}
		}
	}
	if carried {
		a.carriers[op] = append(a.carriers[op], carrier{span: d.Span})
	}
}

// carriedReads はデコレータの reads の節の要素一つが、適用の引数 args のもとで指す ReadRef の列。FQN の位置が
// `$param:名前` でなければその要素一つ。`$param:名前` なら、その引数の値ごとに、同じ種類の要素（carriedTargets）。
func carriedReads(element ir.ReadRef, args []ir.Arg) []ir.ReadRef {
	switch x := element.(type) {
	case ir.NamedRead:
		return ir.NamedReads(carriedTargets(x.Read, args)...)
	case ir.DirectRead:
		return ir.StateReads(carriedTargets(x.State, args)...)
	}
	return nil
}

// carriedEffects はデコレータの effects の節の要素一つが、適用の引数 args のもとで指す EffectRef の列。直に書いた要素は
// そのまま一つ。名前で書いた要素は、FQN の位置が `$param:名前` なら、その引数の値ごとに一つ（carriedTargets）。
func carriedEffects(element ir.EffectRef, args []ir.Arg) []ir.EffectRef {
	if x, ok := element.(ir.NamedEffect); ok {
		return ir.NamedEffects(carriedTargets(x.Effect, args)...)
	}
	return []ir.EffectRef{element}
}

// carriedTargets はデコレータの reads / effects の節の要素一つの FQN の位置 element が、適用の引数 args のもとで指す宣言の
// FQN の列。`$param:名前` でなければその FQN 一つ。`$param:名前` なら、その引数の値: ref は一つ、refs と list（残りの引数）は
// 書かれた順にすべて。渡されていない引数（省いた省略可能な引数）は空。
func carriedTargets(element string, args []ir.Arg) []string {
	param, isParam := strings.CutPrefix(element, "$param:")
	if !isParam {
		return []string{element}
	}
	for _, arg := range args {
		if arg.Param == param {
			return argTargets(arg.Value)
		}
	}
	return nil
}

// argTargets は引数の値が指す宣言の FQN の列（書かれた順）。op の引数（"$param:"）、リテラル、バリアントは含めない。
func argTargets(v ir.ArgValue) []string {
	switch x := v.(type) {
	case ir.RefValue:
		if strings.HasPrefix(x.Target, "$param:") {
			return nil
		}
		return []string{x.Target}
	case ir.RefsValue:
		var out []string
		for _, t := range x.Targets {
			if !strings.HasPrefix(t, "$param:") {
				out = append(out, t)
			}
		}
		return out
	case ir.ListValue:
		var out []string
		for _, item := range x.Items {
			out = append(out, argTargets(item)...)
		}
		return out
	}
	return nil
}

// checkDEC13 は DEC-13（docs/check.md「デコレータ」）: @repeatable の印のないデコレータが、同じ宣言に二度付いている
// （引数が違っても）。名前付きデコレータは展開後の汎用デコレータで比べ、印も汎用デコレータのものを見る。
func (a *analyzer) checkDEC13(app ir.Application, d *ast.Decorator, seen map[string]source.Span) {
	if sym := a.syms[app.Decorator]; sym != nil {
		if dec, ok := a.lower(sym).(*ir.Decorator); ok && dec.Repeatable {
			return
		}
	}
	if first, dup := seen[app.Decorator]; dup {
		a.errorf("DEC-13", d.Name.Span, "デコレータ %s が同じ宣言に二度付いている（何度も付けられるのは、宣言に @repeatable を持つデコレータだけ）", app.Decorator).Related = append([]diag.Related{}, related(first, "最初の適用"))
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
	if d.FromDoc && !dec.Hint {
		a.errorf("DEC-29", d.Name.Span, "説明のコメントの中に、タグとして書けるのは、@hint の付いたデコレータ（解釈されなくてよい、注記）だけ。%s は、宣言の前に @%s(…) と書く", dec.Name, d.Name.String())
		return ir.Application{}, false
	}
	if d.FromDoc && !takesOneString(dec) {
		a.errorf("DEC-29", d.Name.Span, "説明のコメントの中に、タグとして書けるのは、文章を一つ取るデコレータ（引数が string の一つ）だけ。%s は、宣言の前に @%s(…) と書く", dec.Name, d.Name.String())
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
	a.checkDEC1(generic, site, d)
	return app, true
}

// takesOneString は、dec が、文字列を一つ取るデコレータか（docs/check.md「コメントの中のタグ」）: 名前付きデコレータでなく、
// 引数が、ちょうど一つで、残りの引数でなく、型が string 一つ（省ける引数でもよい）。
func takesOneString(dec *ir.Decorator) bool {
	if dec.Alias != nil || len(dec.Params) != 1 || dec.Params[0].Rest {
		return false
	}
	pt, ok := dec.Params[0].Type.(ir.TypeParam)
	if !ok || len(pt.Types) != 1 {
		return false
	}
	core, ok := pt.Types[0].(ir.CoreType)
	return ok && core.Name == "string"
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
// union / classify / decorator の宣言も、targets に、その種類があれば、付けられる（namespace / bind は文法で閉じる）。
// 分類のケースに付けたときは、targets に、そのケースを含む、分類のケース（CaseKind）がなければ DEC-1。
func (a *analyzer) checkDEC1(dec *ir.Decorator, site *appSite, d *ast.Decorator) {
	kind := site.kind
	t, ok := kind.target()
	if ok && kind == siteCase {
		for _, k := range dec.CaseTargets {
			if k.Matches(site.caseOf) {
				return
			}
		}
	} else if ok {
		for _, want := range dec.Targets {
			if want == t {
				return
			}
		}
	}
	a.errorf("DEC-1", d.Name.Span, "デコレータ %s は %s に付けられない（for: %s）", dec.Name, kind, targetsString(dec))
}

// caseKindString は、分類のケースの絞りを、もとの形（case of …）で書く（docs/grammar.md「decorator の宣言」）。
func caseKindString(k ir.CaseKind) string {
	switch {
	case k.Classify != "":
		return "case of " + k.Classify
	case k.Subject != nil:
		return "case of " + typeName(k.Subject)
	}
	return "case of any"
}

func targetsString(dec *ir.Decorator) string {
	names := make([]string, 0, len(dec.Targets))
	cases := 0
	for _, t := range dec.Targets {
		if t != ir.TargetCase || cases >= len(dec.CaseTargets) {
			names = append(names, string(t))
			continue
		}
		// 宣言の書き方（docs/grammar.md「decorator の宣言」の CaseKind）
		k := dec.CaseTargets[cases]
		cases++
		names = append(names, caseKindString(k))
	}
	return strings.Join(names, ", ")
}

// --- 引数（docs/types.md「デコレータの引数」） ---

// lowerArgs は適用の引数を Param に対応付ける。数と名前の違反は code（適用なら DEC-11、別名なら DEC-12）。
//
// 宣言が残りの引数（Param.rest）を持てば、ほかの Param の数を超えて位置で渡した値は、書いた順に、残りの引数の値になる。
// 残りの引数は、いつも一つの Arg で、value は list（0 個なら items が空。docs/ir.md「デコレータの適用」）。値は一つずつ、
// 残りの引数の ParamType で受ける。Arg の並びは書かれた順で、残りの引数の Arg は最初の値の位置（値がなければ末尾）。
func (a *analyzer) lowerArgs(args []*ast.Arg, dec *ir.Decorator, sc *scope, span source.Span, code string) []ir.Arg {
	var out []ir.Arg
	given := map[string]bool{}
	rest, hasRest := restParam(dec)
	restAt := -1
	var restItems []ir.ArgValue
	for i, arg := range args {
		if arg == nil || arg.Value == nil {
			continue
		}
		p, ok := a.paramFor(arg, i, dec, code)
		if !ok {
			continue
		}
		if p.Rest {
			if restAt < 0 {
				restAt = len(out)
				out = append(out, ir.Arg{Param: p.Name})
			}
			// 値一つの型で受ける（残りの引数そのものではない）
			if v, ok := a.lowerArgValue(arg.Value, ir.Param{Name: p.Name, Type: p.Type}, sc); ok {
				restItems = append(restItems, v)
			}
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
		if !p.Optional && !p.Rest && !given[p.Name] {
			a.errorf(code, span, "デコレータ %s の必須の引数 %s が渡されていない", dec.Name, p.Name)
		}
	}
	if hasRest {
		if restAt < 0 {
			restAt = len(out)
			out = append(out, ir.Arg{Param: rest.Name})
		}
		out[restAt].Value = ir.ListValue{Items: restItems}
	}
	return out
}

// restParam は宣言の残りの引数（Param.rest）。Params の最後の一つだけがなりうる。
func restParam(dec *ir.Decorator) (ir.Param, bool) {
	if n := len(dec.Params); n > 0 && dec.Params[n-1].Rest {
		return dec.Params[n-1], true
	}
	return ir.Param{}, false
}

// paramFor は位置か名前で Param を引く。位置で渡した引数が、残りの引数のほかの Param の数を超えていれば、残りの引数。
// 残りの引数には名前では渡せない（DEC-11。docs/types.md「デコレータの引数」）。
func (a *analyzer) paramFor(arg *ast.Arg, index int, dec *ir.Decorator, code string) (ir.Param, bool) {
	rest, hasRest := restParam(dec)
	fixed := dec.Params
	if hasRest {
		fixed = fixed[:len(fixed)-1]
	}
	if arg.Name != nil {
		for _, p := range fixed {
			if p.Name == arg.Name.Name {
				return p, true
			}
		}
		if hasRest && rest.Name == arg.Name.Name {
			a.errorf(code, arg.Name.Span, "デコレータ %s の残りの引数 %s には、名前では渡せない（ほかの引数の後ろに、位置で並べる）", dec.Name, rest.Name)
			return ir.Param{}, false
		}
		a.errorf(code, arg.Name.Span, "デコレータ %s に引数 %s はない", dec.Name, arg.Name.Name)
		return ir.Param{}, false
	}
	if index < len(fixed) {
		return fixed[index], true
	}
	if hasRest {
		return rest, true
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
		if pt.Of == ir.AnyClassifyCase {
			// 分類のケース。絞り（分類の名前、対象の型）に合わなければ DEC-7（docs/types.md「デコレータの引数」）
			if pt.Case != nil {
				if c, _ := a.lower(r.sym).(*ir.Classify); c != nil && !pt.Case.Matches(c) {
					a.errorf("DEC-7", term.Span, "引数 %s は、%s を受け取るが、%s.%s は、それに当たらない分類のケース", p.Name, caseKindString(*pt.Case), r.sym.fqn, r.name)
				}
			}
			return ir.CaseValue{Classify: r.sym.fqn, Case: r.name}, true
		}
		if r.kind == rDecl && r.sym.kind == kindUnion {
			// set のユニオンは展開して refs にする（docs/types.md「デコレータの引数」）
			sawUnion = true
			targets = append(targets, a.expandSet(r, kindOf(pt.Of), term.Span)...)
			continue
		}
		if pt.Of == ir.AnyOp {
			// デコレータの引数に渡した op は「呼ぶ」に当たる（docs/check.md「export」）。呼ぶ側は、デコレータを付けた
			// 宣言の名前空間（名前付きのデコレータの固定した引数なら、その別名の宣言の名前空間）
			a.checkNS5(r.sym, sc.ns, term.Span)
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
	case ir.AnyClassifyCase:
		return acCase
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
	dec := &ir.Decorator{Entry: d.Entry, Hint: d.Hint, Repeatable: d.Repeatable, Fn: d.Fn}
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
	if d.Fn {
		dec.Targets = []ir.DecoratorTarget{ir.TargetOp}
	}
	for _, t := range d.For {
		if t != nil && t.Case != nil {
			dec.CaseTargets = append(dec.CaseTargets, a.lowerCaseKind(t.Case, sc))
		}
	}
	csc := &scope{ns: s.ns, decoParams: params}
	for _, c := range d.Clauses {
		switch c := c.(type) {
		case *ast.ThrowsClause:
			if c.Outer {
				dec.OuterThrows = append(dec.OuterThrows, a.lowerThrows(c, csc))
			} else {
				dec.Throws = append(dec.Throws, a.lowerThrows(c, csc))
			}
		case *ast.CatchesClause:
			dec.Catches = append(dec.Catches, a.lowerCatches(c, csc))
		case *ast.ReadsClause:
			dec.Reads = append(dec.Reads, a.lowerDecoReads(c, csc)...)
		case *ast.EffectsClause:
			dec.Effects = append(dec.Effects, a.lowerDecoEffects(c, csc)...)
		}
	}
	a.checkREL1(s.fqn, dec.Throws, d.Span)
	a.checkREL1(s.fqn, dec.OuterThrows, d.Span)
	if d.Hint {
		// DEC-26: @hint を、付けられない宣言（docs/check.md「デコレータ」）
		var why []string
		if len(d.Clauses) > 0 {
			why = append(why, "振る舞いを言う節（throws、outer throws、catches、reads、effects）を持つ")
		}
		if d.Body != nil {
			why = append(why, "本体を持つ")
		}
		if d.Entry {
			why = append(why, "@entry を持つ（入口は、解釈されることを期待する）")
		}
		if len(why) > 0 {
			a.errorf("DEC-26", d.HintSpan, "デコレータ %s に @hint は付けられない: %s。@hint は、解釈されなくても、よいデコレータの印", s.fqn, strings.Join(why, "。"))
		}
	}
	// DEC-28: 包んだ関数の引数と、本体が、そろっていない（docs/check.md「デコレータ」）
	switch wp := d.Wrapped; {
	case d.Body != nil && wp == nil:
		a.errorf("DEC-28", d.Name.Span, "本体のあるデコレータ %s は、包んだ関数を、最初の引数で受け取る（decorator fn %s(next: fn) { next(); … }。名前は、自由）。前の書き方 proceed() は、なくなった", s.fqn, d.Name.Name)
	case d.Body == nil && wp != nil:
		a.errorf("DEC-28", wp.Span, "本体のないデコレータ %s は、包んだ関数の引数 %s: fn を持てない（プラグインが解釈するので、呼ぶ所がない）。本体を書くか、この引数を外す", s.fqn, wp.Name.Name)
	case wp != nil && params.params[wp.Name.Name] != nil:
		a.errorf("DEC-28", wp.Name.Span, "包んだ関数の引数 %s は、デコレータ %s の、ほかの引数と同じ名前", wp.Name.Name, s.fqn)
	}
	if d.Body != nil {
		for _, c := range d.Clauses {
			if cc, ok := c.(*ast.CatchesClause); ok {
				a.errorf("DEC-22", cc.Span, "本体のあるデコレータ %s に、catches の節は書けない（例外を受ける処理は、本体に書けない。例外を受けるデコレータは、本体を持たない形で宣言する）", s.fqn)
			}
		}
		a.wirings = append(a.wirings, &wiringJob{sym: s, deco: dec, decoDecl: d})
	}
	return dec
}

func setOf(m map[string]*ast.Param) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}

// lowerDecoReads はデコレータの宣言の reads の一節（docs/ir.md「decorator」の reads）。要素は op の節と同じ（read か、
// 直に書いた state。どの名前空間のものでも解決する）か、any state の引数（残りの引数でもよい。`$param:名前` を、
// 直に書いた要素の位置に置く）。どれでもなければ RES-4。
func (a *analyzer) lowerDecoReads(c *ast.ReadsClause, sc *scope) []ir.ReadRef {
	var out []ir.ReadRef
	for _, n := range c.Reads {
		r, ok := a.resolve(n, sc, acReadsPos|acDecoParam)
		if !ok {
			continue
		}
		if r.kind != rDecoParam {
			out = append(out, readRef(r.sym))
			continue
		}
		if a.checkAnyParam(sc, r.name, ir.AnyState, n.Span) {
			out = append(out, ir.DirectRead{State: "$param:" + r.name})
		}
	}
	return out
}

// lowerDecoEffects はデコレータの宣言の effects の一節（docs/ir.md「decorator」の effects）。要素は op の節と同じ（effect、
// 効果の union（展開）、直に書いた「状態.操作」）か、any effect の引数（残りの引数でもよい。`$param:名前`）。
func (a *analyzer) lowerDecoEffects(c *ast.EffectsClause, sc *scope) []ir.EffectRef {
	var out []ir.EffectRef
	for _, n := range c.Effects {
		r, ok := a.resolve(n, sc, acEffectsPos|acDecoParam)
		if !ok {
			continue
		}
		if r.kind == rDecoParam {
			if a.checkAnyParam(sc, r.name, ir.AnyEffect, n.Span) {
				out = append(out, ir.NamedEffect{Effect: "$param:" + r.name})
			}
			continue
		}
		out = append(out, a.effectRefs(r, n)...)
	}
	return out
}

// lowerParam は Param。`int?` は Types を int にして Optional を true にする。`any state?` も Optional。
// 残りの引数（`...名前: 型`）は Rest で、Optional にはならない（構文解析が閉じている）。
func (a *analyzer) lowerParam(p *ast.Param, sc *scope) ir.Param {
	param := ir.Param{Name: p.Name.Name, Rest: p.Rest}
	if p.Type.Any != nil {
		param.Type = a.lowerAnyType(p.Type.Any, sc)
		param.Optional = p.Type.Optional && !p.Rest
		return param
	}
	tp := ir.TypeParam{}
	for _, t := range p.Type.Types {
		if o, ok := t.(*ast.OptionalType); ok {
			param.Optional = !p.Rest
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
	if !ok {
		return a.lowerType(t, sc)
	}
	r, ok := a.resolve(nt.Name, sc, acTypePos|acUnionSet)
	if !ok {
		return nil, false
	}
	if r.kind == rCase {
		a.errorf("RES-4", nt.Name.Span, "classify のケース %s は、そのままでは型にならない。絞り込んだ型は、T on %s と書く", nt.Name.String(), nt.Name.String())
		return nil, false
	}
	return ir.Ref(r.sym.fqn), true
}

// lowerCaseKind は、デコレータの宣言の、分類のケース（`case of …`。docs/grammar.md「decorator の宣言」）。
// of の後ろの名前が、classify（どの名前空間のものでも）なら、その分類で絞る。型なら、対象の型で絞る。
// どちらでもなければ（op、状態など）RES-4。
func (a *analyzer) lowerCaseKind(k *ast.CaseKind, sc *scope) ir.CaseKind {
	out := ir.CaseKind{}
	switch {
	case k.Never:
		out.Subject = ir.Never()
	case k.Of != nil:
		if n, ok := k.Of.(*ast.NamedType); ok {
			// 名前: 分類の名前か、型の名前かを、名前の解決で分ける
			r, ok := a.resolve(n.Name, sc, acClassify|acModel|acScalar|acUnionArms)
			if !ok || r.sym == nil {
				return out
			}
			if r.sym.kind == kindClassify {
				out.Classify = r.sym.fqn
				return out
			}
			out.Subject = ir.Ref(r.sym.fqn)
			return out
		}
		if t, ok := a.lowerType(k.Of, sc); ok {
			out.Subject = t
		}
	}
	return out
}

// lowerAnyType は `any 種類 [on 状態.操作]`。on の状態はどの名前空間のものでもよい。
func (a *analyzer) lowerAnyType(t *ast.AnyType, sc *scope) ir.ParamType {
	if t.Kind == ast.AnyCase {
		k := ir.CaseKind{}
		if t.Case != nil {
			k = a.lowerCaseKind(t.Case, sc)
		}
		return ir.AnyParam{Of: ir.AnyClassifyCase, Case: &k}
	}
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
			if t.Kind == ast.TargetCase {
				out = append(out, ir.TargetCase)
				continue
			}
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

// lowerAlias は名前付きデコレータ `decorator X = Y(args);`。DEC-12: Y が別名、または args が Y の Param に合わない。
func (a *analyzer) lowerAlias(s *symbol, al *ast.DecoratorAlias, sc *scope) *ir.DecoratorAlias {
	r, ok := a.resolve(al.Name, sc, acDecorator)
	if !ok {
		return nil
	}
	target, ok := a.lower(r.sym).(*ir.Decorator)
	if !ok || target.Alias != nil {
		a.errorf("DEC-12", al.Name.Span, "名前付きデコレータ %s の元 %s も名前付きデコレータ（別名の別名は作れない）", s.fqn, r.sym.fqn)
		return nil
	}
	return &ir.DecoratorAlias{Target: target.Name, Args: a.lowerArgs(al.Args, target, sc, al.Span, "DEC-12")}
}
