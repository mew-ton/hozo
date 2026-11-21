package sema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// accept は位置が受け付ける解決先の種類の集合（docs/resolve.md「位置ごとに受け付ける種類」）。
type accept uint32

const (
	acModel accept = 1 << iota
	acScalar
	acUnionArms
	acUnionSet
	acState
	acException
	acEffect
	acRead
	acClassify
	acDecorator
	acOp
	acCase      // classify のケース（C.Case）
	acVariant   // ユニオンのアーム（U.Arm）
	acValue     // 束縛・引数・状態とそのフィールド（Atom.name、状態から辿ったフィールドの Atom.state）
	acDecoParam // デコレータの宣言の引数（$param:）
	acOpParam   // op の引数（デコレータの適用の引数。$param:）
	acNamespace // 名前空間（bind の対象）
	acStateOp   // 状態への操作（effects の要素に直に書いた「状態.操作」と、配線で直に呼ぶ「状態.操作(…)」。手続き 2 の表）

	acAnyDecl = acModel | acScalar | acUnionArms | acUnionSet | acState | acException | acEffect | acRead | acClassify | acDecorator | acOp
)

// rkind は解決の結果の種類。
type rkind int

const (
	rDecl rkind = iota
	rNamespace
	rCase
	rVariant
	rValue
	rDecoParam
	rOpParam
	rStateOp
)

// resolved は解決の結果。
type resolved struct {
	kind rkind
	sym  *symbol    // rDecl / rCase / rVariant / rStateOp の宣言
	ns   *namespace // rNamespace
	name string     // rCase / rVariant のケース・アーム名、rDecoParam / rOpParam の引数名
	op   ir.Operation
	// value は rValue のとき。
	value *valueRef
	span  source.Span
}

// valueRef は束縛・引数・状態とそのフィールドのパス。
type valueRef struct {
	// path は書かれたパス（診断用）。先頭は束縛名か引数名か、状態を指す名前（修飾していれば名前空間も含む）。
	path []string
	typ  exprType
	// root は起点の同一性: 束縛・引数なら "b:名前"、状態なら "s:FQN"。絞り込み（docs/types.md「アームの中の対象の型」）の照合に使う。
	root string
	// fields は起点から辿ったフィールドの名前。起点そのものなら空。
	fields []string
	// state は起点が状態（配線の値の位置で名前で書いた状態）ならその宣言。束縛・引数なら nil。
	state *symbol
}

// narrowKey は絞り込みの表のキー: 起点とフィールドのパス。
func narrowKey(root string, fields []string) string {
	return root + "\x00" + strings.Join(fields, ".")
}

// resolveError は解決の失敗。code は RES-1 〜 RES-5。
type resolveError struct {
	code       string
	span       source.Span
	msg        string
	candidates []string
}

// scope は位置 P の周りの名前（docs/resolve.md「スコープ」）。
type scope struct {
	ns *namespace
	// wiring は配線の中なら、その束縛と op の引数（手続き 1.1）。
	wiring *wiringScope
	// decoParams はデコレータの宣言の throws / catches / reads / effects の中なら、その引数（手続き 1.2）。
	decoParams *decoParamScope
	// opParams は op に付いたデコレータの引数の値で、Param.type が type のとき（手続き 1.3）。
	opParams *opParamScope
	// asNamespace は bind の対象: セグメントが一つでも名前空間として解決する。
	asNamespace bool
	// values は配線の値の位置（Atom の Path）: 現在の名前空間の state を値として読め、state から先のセグメントは
	// その値のフィールド（docs/resolve.md「手続き」2 の表）。
	values bool
}

// narrowed は、起点 root から fields を辿った値の型。アームの中で絞り込まれていればその型、そうでなければ t。
func (sc *scope) narrowed(root string, fields []string, t exprType) exprType {
	if sc.wiring == nil || len(sc.wiring.narrow) == 0 {
		return t
	}
	if n, ok := sc.wiring.narrow[narrowKey(root, fields)]; ok {
		return n
	}
	return t
}

// decoParamScope はデコレータの宣言の引数の名前。
type decoParamScope struct {
	decl   string // デコレータの FQN
	params map[string]*ast.Param
	types  map[string]ir.ParamType
}

// opParamScope は op の引数の名前（デコレータの適用の引数の値で使う）。
type opParamScope struct {
	decl   string // op の FQN
	params map[string]*opParamInfo
}

type opParamInfo struct {
	typ  ir.Type
	span source.Span
}

// resolve は名前を位置 P で解決し、失敗なら診断を出す（docs/resolve.md「手続き」）。
func (a *analyzer) resolve(qn ast.QualifiedName, sc *scope, acc accept) (resolved, bool) {
	r, err := a.resolveName(qn, sc, acc)
	if err != nil {
		a.reportResolve(err)
		return resolved{}, false
	}
	return r, true
}

func (a *analyzer) reportResolve(err *resolveError) {
	d := a.errorf(err.code, err.span, "%s", err.msg)
	if err.code == "RES-1" {
		cands := err.candidates
		if cands == nil {
			cands = []string{}
		}
		d.Data = map[string]any{"candidates": cands}
	}
}

// resolveName は手続きの 1〜3 を行い、結果か失敗を返す。参照は成功したときだけ記録する。
func (a *analyzer) resolveName(qn ast.QualifiedName, sc *scope, acc accept) (resolved, *resolveError) {
	if len(qn.Segments) == 0 {
		return resolved{}, &resolveError{code: "RES-1", span: qn.Span, msg: "名前がない"}
	}
	var refs []Ref
	r, rest, err := a.resolveHead(qn, sc, acc, &refs)
	if err != nil {
		return resolved{}, err
	}
	for _, seg := range rest {
		r, err = a.resolveSegment(r, seg, sc, acc, &refs)
		if err != nil {
			return resolved{}, err
		}
	}
	r.span = qn.Span
	if err := a.checkAccept(r, acc, qn); err != nil {
		return resolved{}, err
	}
	a.refs = append(a.refs, refs...)
	return r, nil
}

// peek は名前を解決するが、診断も参照も残さない。位置が受け付ける種類の照合（手続き 3）もしない。
// 名前が何を指すかで読み方が変わる位置（classify の対象）が、先に種類を見るために使う。
func (a *analyzer) peek(qn ast.QualifiedName, sc *scope) (resolved, bool) {
	if len(qn.Segments) == 0 {
		return resolved{}, false
	}
	var refs []Ref
	r, rest, err := a.resolveHead(qn, sc, 0, &refs)
	if err != nil {
		return resolved{}, false
	}
	for _, seg := range rest {
		r, err = a.resolveSegment(r, seg, sc, 0, &refs)
		if err != nil {
			return resolved{}, false
		}
	}
	return r, true
}

// resolveHead は手続き 1（先頭のセグメント）。残りのセグメントも返す。
func (a *analyzer) resolveHead(qn ast.QualifiedName, sc *scope, acc accept, refs *[]Ref) (resolved, []ast.Ident, *resolveError) {
	head := qn.Segments[0]
	rest := qn.Segments[1:]
	// 1.1 配線の束縛・op の引数
	if sc.wiring != nil {
		if v, ok := sc.wiring.lookup(head.Name); ok {
			*refs = append(*refs, Ref{Span: head.Span, Target: v.target})
			v.markUsed()
			if v.unusable != "" {
				return resolved{}, nil, &resolveError{code: "DEC-23", span: head.Span, msg: fmt.Sprintf("デコレータの引数 %s は、%s なので、本体の中では、まだ使えない。本体で読めるのは、型を一つ書いた、値の引数だけ", head.Name, v.unusable)}
			}
			if v.unassigned && sc.wiring.unassignedRead != nil {
				sc.wiring.unassignedRead(v, head.Span)
			}
			root := "b:" + head.Name
			return resolved{kind: rValue, value: &valueRef{path: []string{head.Name}, typ: sc.narrowed(root, nil, v.typ), root: root}}, rest, nil
		}
	}
	// 1.2 デコレータの宣言の引数
	if sc.decoParams != nil {
		if p, ok := sc.decoParams.params[head.Name]; ok {
			*refs = append(*refs, Ref{Span: head.Span, Target: Target{Kind: TargetDecoratorParam, Decl: sc.decoParams.decl, Name: head.Name, Span: p.Name.Span}})
			return resolved{kind: rDecoParam, name: head.Name}, rest, nil
		}
	}
	// 1.3 op に付いたデコレータの引数の値（Param.type が type のときだけ。any では飛ばす）
	if sc.opParams != nil {
		if p, ok := sc.opParams.params[head.Name]; ok {
			*refs = append(*refs, Ref{Span: head.Span, Target: Target{Kind: TargetOpParam, Decl: sc.opParams.decl, Name: head.Name, Span: p.span}})
			return resolved{kind: rOpParam, name: head.Name}, rest, nil
		}
	}
	// 1.4 ルートの名前空間（セグメントが二つ以上のとき。bind の対象なら一つでも）
	if len(rest) > 0 || sc.asNamespace {
		if ns, ok := a.global.children[head.Name]; ok {
			*refs = append(*refs, Ref{Span: head.Span, Target: Target{Kind: TargetNamespace, Decl: ns.fqn, Span: head.Span}})
			return resolved{kind: rNamespace, ns: ns}, rest, nil
		}
		if sc.asNamespace {
			return resolved{}, nil, &resolveError{code: "RES-1", span: head.Span, msg: fmt.Sprintf("名前空間 %s が見つからない", head.Name), candidates: a.namespaceCandidates(head.Name)}
		}
	}
	// 1.5 現在の名前空間、1.6 グローバル
	for _, ns := range []*namespace{sc.ns, a.global} {
		if ns == nil {
			continue
		}
		if s, ok := ns.syms[head.Name]; ok {
			*refs = append(*refs, Ref{Span: head.Span, Target: declTarget(s)})
			return resolved{kind: rDecl, sym: s}, rest, nil
		}
	}
	// 1.7 RES-1。なくした核の型の名前（Time、Duration）は、新しい語を示す（docs/resolve.md「手続き」の 1.7）
	msg := fmt.Sprintf("%s が見つからない", head.Name)
	if now, ok := removedCoreTypes[head.Name]; ok {
		msg += fmt.Sprintf("（核の型 %s は、%s に名前が変わった。docs/grammar.md「型」）", head.Name, now)
	}
	return resolved{}, nil, &resolveError{
		code:       "RES-1",
		span:       head.Span,
		msg:        msg,
		candidates: a.candidates(head.Name, sc, acc, nil),
	}
}

// removedCoreTypes は、なくした核の型の名前と、今の語（docs/decisions.md「決めたこと」359）。今は、ふつうの名前なので、
// 同じ名前の宣言があれば、それに解決する。なければ、RES-1 のメッセージに、今の語を足す。
var removedCoreTypes = map[string]string{"Time": "datetime", "Duration": "duration"}

// resolveSegment は手続き 2（残りのセグメントを辿る）の表の一行。
func (a *analyzer) resolveSegment(prev resolved, seg ast.Ident, sc *scope, acc accept, refs *[]Ref) (resolved, *resolveError) {
	switch prev.kind {
	case rNamespace:
		return a.resolveInNamespace(prev.ns, seg, sc, acc, refs)
	case rValue:
		return a.resolveField(prev, seg, sc, refs)
	case rDecl:
		if prev.sym.kind == kindState && sc.values {
			// 配線の値の位置: 状態は値で、次のセグメントはその値のフィールド
			v, err := a.stateValue(prev.sym, sc, seg.Span)
			if err != nil {
				return resolved{}, err
			}
			return a.resolveField(resolved{kind: rValue, value: v}, seg, sc, refs)
		}
		switch prev.sym.kind {
		case kindClassify:
			if span, ok := classifyCaseSpan(prev.sym, seg.Name); ok {
				*refs = append(*refs, Ref{Span: seg.Span, Target: Target{Kind: TargetCase, Decl: prev.sym.fqn, Name: seg.Name, Span: span}})
				return resolved{kind: rCase, sym: prev.sym, name: seg.Name}, nil
			}
			return resolved{}, &resolveError{code: "RES-3", span: seg.Span, msg: fmt.Sprintf("classify %s にケース %s はない", prev.sym.fqn, seg.Name)}
		case kindUnion:
			if prev.sym.unionVariant == ir.UnionArms {
				if span, ok := unionArmSpan(prev.sym, seg.Name); ok {
					*refs = append(*refs, Ref{Span: seg.Span, Target: Target{Kind: TargetArm, Decl: prev.sym.fqn, Name: seg.Name, Span: span}})
					return resolved{kind: rVariant, sym: prev.sym, name: seg.Name}, nil
				}
				return resolved{}, &resolveError{code: "RES-3", span: seg.Span, msg: fmt.Sprintf("union %s にアーム %s はない", prev.sym.fqn, seg.Name)}
			}
		case kindState:
			if op, ok := stateOperation(seg.Name); ok {
				return resolved{kind: rStateOp, sym: prev.sym, op: op}, nil
			}
		}
	}
	return resolved{}, &resolveError{code: "RES-3", span: seg.Span, msg: fmt.Sprintf("%s から %s を辿れない", describe(prev), seg.Name)}
}

// resolveInNamespace は名前空間の次のセグメント: 子の名前空間か宣言。両方なら RES-2、どちらもなければ RES-1。
func (a *analyzer) resolveInNamespace(ns *namespace, seg ast.Ident, sc *scope, acc accept, refs *[]Ref) (resolved, *resolveError) {
	child, hasChild := ns.children[seg.Name]
	s, hasSym := ns.syms[seg.Name]
	switch {
	case hasChild && hasSym && child.declared:
		return resolved{}, &resolveError{code: "RES-2", span: seg.Span, msg: fmt.Sprintf("%s は名前空間 %s の子の名前空間と宣言の両方に当たり曖昧", qualify(ns.fqn, seg.Name), ns.fqn)}
	case hasSym:
		*refs = append(*refs, Ref{Span: seg.Span, Target: declTarget(s)})
		return resolved{kind: rDecl, sym: s}, nil
	case hasChild:
		*refs = append(*refs, Ref{Span: seg.Span, Target: Target{Kind: TargetNamespace, Decl: child.fqn, Span: seg.Span}})
		return resolved{kind: rNamespace, ns: child}, nil
	}
	return resolved{}, &resolveError{
		code:       "RES-1",
		span:       seg.Span,
		msg:        fmt.Sprintf("%s が名前空間 %s に見つからない", seg.Name, ns.fqn),
		candidates: a.candidates(seg.Name, sc, acc, ns),
	}
}

// stateValue は配線の値の位置で名前で書いた状態（docs/check.md「状態の参照」）。型はその状態の型。
// 別の名前空間（かグローバル）の状態なら RES-5。span は診断の位置。
func (a *analyzer) stateValue(s *symbol, sc *scope, span source.Span) (*valueRef, *resolveError) {
	if s.ns != sc.ns {
		return nil, &resolveError{code: "RES-5", span: span, msg: fmt.Sprintf("状態 %s は別の名前空間のもので、名前では読めない（状態を名前で読めるのは持ち主の名前空間 %s の配線だけ。その名前空間の op を通して読む）", s.fqn, nsName(s.ns))}
	}
	typ := unknownType()
	if st, ok := a.lower(s).(*ir.State); ok && st.Type != nil {
		typ = valueType(st.Type)
	}
	root := "s:" + s.fqn
	return &valueRef{path: strings.Split(s.fqn, "."), typ: sc.narrowed(root, nil, typ), root: root, state: s}, nil
}

// resolveField は値の参照の次のセグメント: 型のフィールド（model か model の optional）。それ以外は RES-3。
// 絞り込まれた型（case）は元の型に戻してから辿る（docs/types.md「アームの中の対象の型」）。
func (a *analyzer) resolveField(prev resolved, seg ast.Ident, sc *scope, refs *[]Ref) (resolved, *resolveError) {
	v := prev.value
	next := func(t exprType) *valueRef {
		fields := append(append([]string{}, v.fields...), seg.Name)
		return &valueRef{path: append(append([]string{}, v.path...), seg.Name), typ: sc.narrowed(v.root, fields, t), root: v.root, fields: fields, state: v.state}
	}
	if v.typ.kind == etUnknown {
		return resolved{kind: rValue, value: next(unknownType())}, nil
	}
	if v.typ.kind != etValue {
		return resolved{}, &resolveError{code: "RES-3", span: seg.Span, msg: fmt.Sprintf("%s の型からフィールド %s を辿れない", strings.Join(v.path, "."), seg.Name)}
	}
	t := uncase(v.typ.typ)
	optional := false
	if o, ok := t.(ir.OptionalType); ok {
		t, optional = uncase(o.Of), true
	}
	ref, ok := t.(ir.RefType)
	var m *ir.Model
	if ok {
		m = a.models[ref.Target]
	}
	if m == nil {
		return resolved{}, &resolveError{code: "RES-3", span: seg.Span, msg: fmt.Sprintf("%s の型 %s は model ではないので、フィールド %s を辿れない", strings.Join(v.path, "."), typeName(v.typ.typ), seg.Name)}
	}
	f := findField(m, seg.Name)
	if f == nil {
		return resolved{}, &resolveError{code: "RES-3", span: seg.Span, msg: fmt.Sprintf("model %s にフィールド %s はない", m.Name, seg.Name)}
	}
	ft := f.Type
	if optional {
		ft = ir.Optional(ft)
	}
	*refs = append(*refs, Ref{Span: seg.Span, Target: Target{Kind: TargetField, Decl: m.Name, Name: seg.Name, Span: a.fields[m.Name][seg.Name]}})
	return resolved{kind: rValue, value: next(valueType(ft))}, nil
}

// uncase は絞り込まれた型（case）を元の型に戻す。
func uncase(t ir.Type) ir.Type {
	if c, ok := t.(ir.CaseType); ok {
		return c.Of
	}
	return t
}

// checkAccept は手続き 3（位置が受け付ける種類か）。表にない種類なら RES-4。
func (a *analyzer) checkAccept(r resolved, acc accept, qn ast.QualifiedName) *resolveError {
	if acc&resultAccept(r) != 0 {
		return nil
	}
	return &resolveError{code: "RES-4", span: qn.Span, msg: fmt.Sprintf("この位置で %s は指せない（%s）", qn.String(), describe(r))}
}

// resultAccept は結果の種類に対応する accept のビット。
func resultAccept(r resolved) accept {
	switch r.kind {
	case rDecl:
		return symAccept(r.sym)
	case rNamespace:
		return acNamespace
	case rCase:
		return acCase
	case rVariant:
		return acVariant
	case rValue:
		return acValue
	case rDecoParam:
		return acDecoParam
	case rOpParam:
		return acOpParam
	case rStateOp:
		return acStateOp
	}
	return 0
}

// symAccept は宣言の種類に対応する accept のビット。
func symAccept(s *symbol) accept {
	switch s.kind {
	case kindModel:
		return acModel
	case kindScalar:
		return acScalar
	case kindUnion:
		if s.unionVariant == ir.UnionArms {
			return acUnionArms
		}
		return acUnionSet
	case kindState:
		return acState
	case kindException:
		return acException
	case kindEffect:
		return acEffect
	case kindRead:
		return acRead
	case kindClassify:
		return acClassify
	case kindDecorator:
		return acDecorator
	case kindOp:
		return acOp
	}
	return 0
}

// describe は結果の種類の表示。
func describe(r resolved) string {
	switch r.kind {
	case rDecl:
		if r.sym.kind == kindUnion {
			return fmt.Sprintf("union（%s）%s", r.sym.unionVariant, r.sym.fqn)
		}
		return fmt.Sprintf("%s %s", r.sym.kind, r.sym.fqn)
	case rNamespace:
		return "名前空間 " + r.ns.fqn
	case rCase:
		return fmt.Sprintf("classify %s のケース %s", r.sym.fqn, r.name)
	case rVariant:
		return fmt.Sprintf("union %s のアーム %s", r.sym.fqn, r.name)
	case rValue:
		return "値 " + strings.Join(r.value.path, ".")
	case rDecoParam:
		return "デコレータの引数 " + r.name
	case rOpParam:
		return "op の引数 " + r.name
	case rStateOp:
		return fmt.Sprintf("状態 %s への操作 %s", r.sym.fqn, r.op)
	}
	return "?"
}

// declTarget は宣言への Target。
func declTarget(s *symbol) Target {
	return Target{Kind: TargetDecl, Decl: s.fqn, Span: s.nameSpan}
}

// classifyCaseSpan は classify のケース名の位置。
func classifyCaseSpan(s *symbol, name string) (source.Span, bool) {
	if d, ok := s.node.(*ast.ClassifyDecl); ok {
		for _, c := range d.Cases {
			if c != nil && c.Name.Name == name {
				return c.Name.Span, true
			}
		}
	}
	return source.Span{}, false
}

// unionArmSpan はユニオンのアーム名の位置。
func unionArmSpan(s *symbol, name string) (source.Span, bool) {
	if d, ok := s.node.(*ast.UnionDecl); ok {
		for _, m := range d.Members {
			if m.Arm != nil && m.Arm.Name == name {
				return m.Arm.Span, true
			}
		}
	}
	return source.Span{}, false
}

func stateOperation(name string) (ir.Operation, bool) {
	switch name {
	case "post":
		return ir.OpPost, true
	case "put":
		return ir.OpPut, true
	case "delete":
		return ir.OpDelete, true
	}
	return "", false
}

// --- RES-1 の候補（docs/resolve.md「診断」） ---

// candidates は、同じ種類で編集距離 2 以内の名前と、修飾なしで別の名前空間に同じ名前があればその FQN。
// within が nil でなければ、その名前空間の中だけを探す（名前空間に続くセグメント）。
func (a *analyzer) candidates(name string, sc *scope, acc accept, within *namespace) []string {
	set := map[string]bool{}
	add := func(s *symbol) {
		if acc != 0 && acc&symAccept(s) == 0 && !(acc&acCase != 0 && s.kind == kindClassify) && !(acc&acVariant != 0 && s.kind == kindUnion) {
			return
		}
		set[s.fqn] = true
	}
	if within != nil {
		for _, s := range within.syms {
			if editDistance(name, s.name) <= 2 {
				add(s)
			}
		}
	} else {
		for _, ns := range []*namespace{sc.ns, a.global} {
			if ns == nil {
				continue
			}
			for _, s := range ns.syms {
				if editDistance(name, s.name) <= 2 {
					add(s)
				}
			}
		}
		for _, ns := range a.nss {
			if s, ok := ns.syms[name]; ok {
				add(s)
			}
		}
	}
	out := make([]string, 0, len(set))
	for fqn := range set {
		out = append(out, fqn)
	}
	sort.Strings(out)
	return out
}

// namespaceCandidates は名前空間の名前の候補（bind の対象）。
func (a *analyzer) namespaceCandidates(name string) []string {
	var out []string
	for _, ns := range a.nsOrder {
		if ns.declared && ns.fqn != "" && editDistance(name, ns.fqn) <= 2 {
			out = append(out, ns.fqn)
		}
	}
	sort.Strings(out)
	return out
}

// editDistance は Levenshtein 距離。
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// --- 位置ごとの accept（docs/resolve.md「位置ごとに受け付ける種類」） ---

const (
	// acTypePos は型の位置。classify のケースを書けるのは、絞り込んだ型 `T on C.K` の on の後ろだけ（型そのものの位置に
	// 書いたケースは、lowerNamedType が RES-4 にする）。
	acTypePos = acModel | acScalar | acUnionArms | acCase
	// acThrowsPos は throws / catches の要素。デコレータの宣言の中では any exception の引数も（lowerExceptions が確かめる）。
	acThrowsPos = acException | acUnionSet | acDecoParam
	// acEffectsPos は effects の要素: 名前で書いた効果（effect、効果の union）か、直に書いた「状態.操作」
	// （docs/resolve.md「effects の要素の読み分け」）。
	acEffectsPos = acEffect | acUnionSet | acStateOp
	// acReadsPos は reads の要素: 名前で書いた read か、直に書いた状態。
	acReadsPos = acRead | acState
	// acCallPos は配線の呼び出しの名前: op、read、effect か、直に呼ぶ「状態.操作」
	// （docs/resolve.md「呼び出しの読み分け」）。
	acCallPos = acOp | acRead | acEffect | acStateOp
	// acHaltsPos は halts の効果。デコレータの宣言の中では any effect の引数も。
	acHaltsPos = acEffect | acDecoParam
	// acMemberPos はユニオンのメンバー。
	acMemberPos = acException | acEffect | acUnionSet
)
