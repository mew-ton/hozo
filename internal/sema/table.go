package sema

import (
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// declKind は宣言の種類。
type declKind int

const (
	kindModel declKind = iota
	kindScalar
	kindUnion
	kindState
	kindException
	kindEffect
	kindClassify
	kindDecorator
	kindOp
)

// String は設計書の語（"model" など）。
func (k declKind) String() string {
	return [...]string{"model", "scalar", "union", "state", "exception", "effect", "classify", "decorator", "op"}[k]
}

// namespace は表の一つの名前空間。暗黙の親（`namespace a.b {}` の a）も持つが、declared が false なら Program には出さない。
type namespace struct {
	fqn      string
	name     string // 最後のセグメント
	parent   *namespace
	children map[string]*namespace
	syms     map[string]*symbol // 名前 → 宣言（併合後）
	order    []*symbol          // 出現順（docs/ir.md「原則」）。落とした宣言は含まない
	declared bool
	// description は namespace ブロックの直前の DocComment。複数あれば最初のもの（docs/ir.md「Program」）。
	description *ir.Doc
}

// lowerState は lower の進み具合。
type lowerState int

const (
	notLowered lowerState = iota
	lowering
	lowered
)

// symbol は表の一つの宣言。
type symbol struct {
	kind     declKind
	name     string
	fqn      string
	ns       *namespace
	node     ast.Decl
	nameSpan source.Span
	file     source.FileID
	vocab    bool
	// unionVariant はユニオンの種類（構文から決まる）。kindUnion 以外では空。
	unionVariant ir.UnionVariant

	state lowerState
	decl  ir.Decl
	// synthesized はこの宣言が合成した model（docs/ir.md「正規化」）。Program ではこの宣言の直後に置く。
	synthesized []ir.Decl
	// derivedDone は `is X omit` の展開が済んだか（finalizeDerived）。
	derivedDone bool
}

// buildTable はすべてのファイルから名前空間の表を作る（docs/resolve.md「名前空間の併合と重複」）。
func (a *analyzer) buildTable() {
	for _, f := range a.files {
		a.collectDecls(a.global, f.Decls, f.Span.File)
	}
}

// collectDecls は宣言の列を名前空間 ns に登録する。namespace は再帰、bind は Binds に足す。
func (a *analyzer) collectDecls(ns *namespace, decls []ast.Decl, file source.FileID) {
	for _, d := range decls {
		switch d := d.(type) {
		case nil:
		case *ast.NamespaceDecl:
			child := a.declareNamespace(ns, d, file)
			a.collectDecls(child, d.Decls, file)
		case *ast.BindDecl:
			a.collectBind(ns, d, file)
		default:
			a.declare(ns, d, file)
		}
	}
}

// declareNamespace は `namespace a.b { }` を表に登録し、その名前空間を返す（NS-2: 同じ FQN は一つにする）。
func (a *analyzer) declareNamespace(parent *namespace, d *ast.NamespaceDecl, file source.FileID) *namespace {
	ns := parent
	for _, seg := range d.Name.Segments {
		ns = a.child(ns, seg.Name)
	}
	if !ns.declared {
		ns.declared = true
	}
	if ns.description == nil && d.Doc != nil {
		ns.description = a.lowerDoc(d.Doc, docContext{ns: ns, span: d.Name.Span})
	}
	return ns
}

// child は ns の子の名前空間を返す。なければ作る（出現順の列にも足す）。
func (a *analyzer) child(ns *namespace, name string) *namespace {
	if c, ok := ns.children[name]; ok {
		return c
	}
	fqn := name
	if ns.fqn != "" {
		fqn = ns.fqn + "." + name
	}
	c := &namespace{fqn: fqn, name: name, parent: ns, children: map[string]*namespace{}, syms: map[string]*symbol{}}
	ns.children[name] = c
	a.nss[fqn] = c
	a.nsOrder = append(a.nsOrder, c)
	return c
}

// declare は一つの宣言を表に登録する。重複は RES-6（両方が語彙なら NS-3）。
func (a *analyzer) declare(ns *namespace, d ast.Decl, file source.FileID) {
	kind, name, ok := declInfo(d)
	if !ok {
		return
	}
	s := &symbol{
		kind:     kind,
		name:     name.Name,
		fqn:      qualify(ns.fqn, name.Name),
		ns:       ns,
		node:     d,
		nameSpan: name.Span,
		file:     file,
		vocab:    a.isVocab(file),
	}
	if u, ok := d.(*ast.UnionDecl); ok {
		s.unionVariant = unionVariantOf(u)
	}
	if prev, dup := ns.syms[s.name]; dup {
		a.reportDuplicate(prev, s)
		return
	}
	ns.syms[s.name] = s
	ns.order = append(ns.order, s)
	a.syms[s.fqn] = s
	a.checkName(s)
}

// reportDuplicate は RES-6 / NS-3（docs/resolve.md「診断」）。位置はユーザーのファイルの側、両方ユーザーなら後の方。
// 報告した側を落とし、もう一方を表に残す。
func (a *analyzer) reportDuplicate(prev, s *symbol) {
	loser := s
	if prev.vocab != s.vocab && s.vocab {
		// ユーザーの宣言が先に登録されていて、語彙の宣言が後から来た: ユーザーの側を報告し、語彙を残す
		loser = prev
		a.replaceSymbol(prev, s)
	}
	other := prev
	if loser == prev {
		other = s
	}
	code := "RES-6"
	msg := "%s は同じスコープに二度宣言されている（%s）"
	if prev.vocab && s.vocab {
		code = "NS-3"
		msg = "%s は複数の語彙がグローバルに宣言している（%s）: " + a.vocabSource(prev.file) + " と " + a.vocabSource(s.file)
	}
	d := a.errorf(code, loser.nameSpan, msg, loser.fqn, loser.kind)
	d.Related = append(d.Related, diag.Related{Span: other.nameSpan, Message: "もう一方の宣言"})
}

// replaceSymbol は表の prev を s に差し替える。s はファイルの順で後に来たものなので、出現順の列では prev を除いて末尾に足す。
func (a *analyzer) replaceSymbol(prev, s *symbol) {
	prev.ns.syms[s.name] = s
	a.syms[s.fqn] = s
	order := prev.ns.order[:0]
	for _, o := range prev.ns.order {
		if o != prev {
			order = append(order, o)
		}
	}
	prev.ns.order = append(order, s)
}

// vocabSource は語彙のファイルの出どころ（標準か、どのプラグインか）。
func (a *analyzer) vocabSource(f source.FileID) string {
	p := a.fs.Path(f)
	if strings.HasPrefix(p, "hozo:plugin/") {
		rest := strings.TrimPrefix(p, "hozo:plugin/")
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			return "プラグイン " + rest[:i]
		}
		return "プラグイン " + rest
	}
	return "標準の語彙"
}

// collectBind は bind を解決して Binds に足す（docs/resolve.md「位置ごとに受け付ける種類」の bind）。
// 表がそろってから解決するので、ここでは順番だけを記録し lowerAll で解決する。
func (a *analyzer) collectBind(ns *namespace, d *ast.BindDecl, file source.FileID) {
	a.pendingBinds = append(a.pendingBinds, pendingBind{ns: ns, decl: d})
}

// declInfo は宣言の種類と名前。namespace / bind では ok が false。
func declInfo(d ast.Decl) (declKind, ast.Ident, bool) {
	switch d := d.(type) {
	case *ast.ModelDecl:
		return kindModel, d.Name, true
	case *ast.ScalarDecl:
		return kindScalar, d.Name, true
	case *ast.UnionDecl:
		return kindUnion, d.Name, true
	case *ast.StateDecl:
		return kindState, d.Name, true
	case *ast.ExceptionDecl:
		return kindException, d.Name, true
	case *ast.EffectDecl:
		return kindEffect, d.Name, true
	case *ast.ClassifyDecl:
		return kindClassify, d.Name, true
	case *ast.ConditionDecl:
		return kindClassify, d.Name, true
	case *ast.DecoratorDecl:
		return kindDecorator, d.Name, true
	case *ast.OpDecl:
		return kindOp, d.Name, true
	}
	return 0, ast.Ident{}, false
}

// unionVariantOf はユニオンの種類を構文から決める。アームが一つでもあれば arms、そうでなければ set。
// 混在は UNION-1 として lowerUnion が報告する。
func unionVariantOf(u *ast.UnionDecl) ir.UnionVariant {
	for _, m := range u.Members {
		if m.Arm != nil {
			return ir.UnionArms
		}
	}
	return ir.UnionSet
}

// qualify は名前空間の FQN と宣言名をつなぐ。グローバルなら宣言名だけ（docs/ir.md「原則」）。
func qualify(ns, name string) string {
	if ns == "" {
		return name
	}
	return ns + "." + name
}
