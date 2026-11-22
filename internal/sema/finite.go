package sema

import (
	"sort"
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/ir"
)

// finiteNode は、有限の値を持てるかを決める宣言一つ: model（その場で書いた合成 model を含む）か、アームを持つ union。
type finiteNode struct {
	fqn   string
	span  source.Span
	model *ir.Model
	union *ir.Union
	// ok は、有限の値を持てるか。
	ok bool
}

// finiteEdge は、値が、必ず持つ先一つ: model の、省略できないフィールドか、union のアーム。
type finiteEdge struct {
	// step は、辿った所の名前（`<持ち主の FQN>.<フィールドかアームの名前>`）。
	step string
	to   *finiteNode
}

// checkFiniteTypes は TYPE-19（docs/types.md「値を、有限に作れない型」）: 省略できないフィールドと、union のアームを辿って、
// 自分に戻る型は、値を、有限に作れない。持てるかは、宣言を辿って、漏れなく決める（辿る回数の上限は、ない）。
// 診断は、互いに辿り合える宣言のまとまりごとに一つ: 位置は、その中で、最初に現れる宣言。Related に、示した輪の上の宣言。
func (a *analyzer) checkFiniteTypes() {
	nodes := map[string]*finiteNode{}
	bodySpan := map[*ir.Model]source.Span{}
	for _, j := range a.bodies {
		if j.body != nil {
			bodySpan[j.model] = j.body.Span
		}
	}
	for fqn, m := range a.models {
		n := &finiteNode{fqn: fqn, model: m}
		if s := a.syms[fqn]; s != nil {
			n.span = s.nameSpan
		} else if sp, ok := bodySpan[m]; ok {
			n.span = sp
		} else {
			continue
		}
		nodes[fqn] = n
	}
	for fqn, s := range a.syms {
		if s.kind != kindUnion {
			continue
		}
		// 例外や効果の集合の union と、アームを一つも持たない union は、見ない
		if u, ok := s.decl.(*ir.Union); ok && u.Variant == ir.UnionArms && len(u.Arms) > 0 {
			nodes[fqn] = &finiteNode{fqn: fqn, span: s.nameSpan, union: u}
		}
	}
	order := make([]*finiteNode, 0, len(nodes))
	for _, n := range nodes {
		order = append(order, n)
	}
	sort.Slice(order, func(i, j int) bool {
		x, y := order[i], order[j]
		if fx, fy := a.fileOrder[x.span.File], a.fileOrder[y.span.File]; fx != fy {
			return fx < fy
		}
		if x.span.Start != y.span.Start {
			return x.span.Start < y.span.Start
		}
		return x.fqn < y.fqn
	})

	// target は、型が、必ず持つ先の宣言（model か union）。核の型、scalar、T?、T[]、解決できなかった参照なら nil（有限の値を持てる）。
	target := func(t ir.Type) *finiteNode {
		for {
			switch tt := t.(type) {
			case ir.CaseType:
				t = tt.Of
				continue
			case ir.RefType:
				return nodes[tt.Target]
			}
			return nil
		}
	}
	// edges は、n の値が、必ず持つ先（model は、省略できないフィールド。union は、アーム）。free は、どこにも戻らずに終われる
	// アームを、union が持つか（model では、使わない）。
	edges := func(n *finiteNode) (out []finiteEdge, free bool) {
		if n.model != nil {
			if n.model.Index != nil {
				return nil, false // インデックスの model は、空の構造を持てる
			}
			for _, f := range n.model.Fields {
				if to := target(f.Type); to != nil {
					out = append(out, finiteEdge{step: n.fqn + "." + f.Name, to: to})
				}
			}
			return out, false
		}
		for _, arm := range n.union.Arms {
			to := target(arm.Type)
			if to == nil {
				free = true
				continue
			}
			out = append(out, finiteEdge{step: n.fqn + "." + arm.Name, to: to})
		}
		return out, free
	}

	// 持てる、と決まるものを、変わるものがなくなるまで、移す
	for changed := true; changed; {
		changed = false
		for _, n := range order {
			if n.ok {
				continue
			}
			out, free := edges(n)
			ok := true
			if n.union != nil {
				ok = free
				for _, e := range out {
					ok = ok || e.to.ok
				}
			} else {
				for _, e := range out {
					ok = ok && e.to.ok
				}
			}
			if ok {
				n.ok, changed = true, true
			}
		}
	}

	// 持てない宣言の中の輪を、最初に現れる宣言から、一つずつ報告する
	stuck := func(n *finiteNode) []finiteEdge {
		var out []finiteEdge
		all, _ := edges(n)
		for _, e := range all {
			if !e.to.ok {
				out = append(out, e)
			}
		}
		return out
	}
	reported := map[*finiteNode]bool{}
	for _, start := range order {
		if start.ok || reported[start] {
			continue
		}
		path := finiteCycle(start, stuck)
		if path == nil {
			continue // 輪に入らず、持てない型を持つだけの宣言
		}
		// 互いに辿り合える宣言のまとまり（start から行けて、start に戻れる宣言）は、一度だけ報告する
		for n := range finiteReach(start, stuck) {
			if finiteReach(n, stuck)[start] {
				reported[n] = true
			}
		}
		steps := make([]string, 0, len(path)+1)
		rel := make([]diag.Related, 0, len(path))
		at := start
		for _, e := range path {
			steps = append(steps, e.step)
			rel = append(rel, related(at.span, "輪の上の宣言: %s", at.fqn))
			at = e.to
		}
		// その場で書いた model は、アームの名前と、同じ名前を持つ。同じ名前を、二度、並べない
		if steps[len(steps)-1] != start.fqn {
			steps = append(steps, start.fqn)
		}
		a.errorf("TYPE-19", start.span,
			"%s %s は、値を、有限に作れない（%s と辿って、自分に戻る）。輪の上のフィールドを、省略できる型（T?）か、並び（T[]）にするか、union に、戻らないアームを足す",
			finiteKind(start), start.fqn, strings.Join(steps, " → ")).Related = rel
	}
}

// finiteKind は、宣言の種類の語。
func finiteKind(n *finiteNode) string {
	if n.union != nil {
		return "union"
	}
	return "model"
}

// finiteReach は、start から、持てない宣言だけを辿って、行き着く宣言の集まり（一歩以上、辿った先）。
func finiteReach(start *finiteNode, stuck func(*finiteNode) []finiteEdge) map[*finiteNode]bool {
	seen := map[*finiteNode]bool{}
	queue := []*finiteNode{start}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, e := range stuck(n) {
			if !seen[e.to] {
				seen[e.to] = true
				queue = append(queue, e.to)
			}
		}
	}
	return seen
}

// finiteCycle は、start から、持てない宣言だけを辿って、start に戻る、いちばん短い道（辺の列）。戻れなければ nil。
// 同じ長さなら、フィールドとアームの、書かれた順で、先のもの。
func finiteCycle(start *finiteNode, stuck func(*finiteNode) []finiteEdge) []finiteEdge {
	type item struct {
		n    *finiteNode
		path []finiteEdge
	}
	seen := map[*finiteNode]bool{}
	queue := []item{{n: start}}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		for _, e := range stuck(it.n) {
			path := append(append([]finiteEdge{}, it.path...), e)
			if e.to == start {
				return path
			}
			if !seen[e.to] {
				seen[e.to] = true
				queue = append(queue, item{n: e.to, path: path})
			}
		}
	}
	return nil
}
