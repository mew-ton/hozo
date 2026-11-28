package gobase

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは GO-3（plugins/go/spec/go.hozo の Requirement.Layout の「パッケージと import パス」）: 出力（単位と名前空間の組）が、import を辿って自分に戻る。
// import する相手は、docs/config.md「型の置き場」の表の 1〜6 が数える宣言の置き場で、どの出力についても View だけから求める。
// 7（葉の戻り値の絞り込みの分類）は、テストのパッケージだけが import する。テストのパッケージは、どの出力からも import されないので、輪に入らない。

// outputKey は出力一つ: 単位（何番目の [[emit]] か）と名前空間。
type outputKey struct {
	index int
	ns    string
}

// uses は、名前空間 nsName の出力が使う、別の名前空間（docs/config.md「型の置き場」の表）を辞書順で返す。inUnit は、その出力の
// 単位の対象の op か。
func (e *emitter) uses(nsName string, inUnit func(*ir.Op) bool, typesOnly bool) []string {
	ns := e.p.Namespace(nsName)
	if ns == nil {
		return nil
	}
	seen := map[string]bool{}
	decl := func(fqn string) {
		if m, known := e.declNS[fqn]; known && m != nsName {
			seen[m] = true
		}
	}
	var typ func(t ir.Type)
	typ = func(t ir.Type) {
		switch x := t.(type) {
		case ir.RefType:
			decl(x.Target)
		case ir.OptionalType:
			typ(x.Of)
		case ir.ArrayType:
			typ(x.Of)
		case ir.CaseType:
			typ(x.Of)
		}
	}
	signature := func(o *ir.Op) {
		for _, p := range o.Params {
			typ(p.Type)
		}
		typ(o.Returns)
	}
	for _, d := range ns.Decls {
		switch x := d.(type) {
		case *ir.Model: // 1
			if !e.typeDeclOf(x) {
				continue
			}
			for _, f := range x.Fields {
				typ(f.Type)
			}
			if x.Index != nil {
				typ(x.Index.Key)
				typ(x.Index.Value)
			}
		case *ir.Scalar:
			typ(x.Base)
		case *ir.Union:
			for _, a := range x.Arms {
				typ(a.Type)
			}
		case *ir.Exception:
			for _, p := range x.Params {
				typ(p.Type)
			}
		case *ir.Classify:
			// 値の分類・op の分類は、型のパッケージに置く（状態の分類の対象は、型でない）
			typ(x.SubjectType())
			for _, cs := range x.Cases {
				for _, as := range cs.As {
					typ(as.Type)
				}
			}
		}
		if typesOnly {
			// 型のパッケージが使うのは、ここまで（plugins/go/spec/go.hozo の Requirement.Layout の「型のパッケージ」）: 関数の側が使う型は、輪にならない
			continue
		}
		switch x := d.(type) {
		case *ir.State: // 2
			typ(x.Type)
		case *ir.Read:
			for _, p := range x.Params {
				typ(p.Type)
			}
			typ(x.Returns)
		case *ir.Effect:
			for _, p := range x.Params {
				typ(p.Type)
				if c, _, ok := golang.PredicateNarrowing(e.p, p.Type); ok { // 6
					decl(c)
				}
			}
		case *ir.Op:
			if !inUnit(x) {
				continue
			}
			signature(x) // 3
			for _, ex := range x.Signature.Exceptions {
				decl(ex)
			}
			if x.Body == nil {
				continue
			}
			var all []ir.Statement
			ir.WalkStatements(x.Body.Statements, func(st ir.Statement) { all = append(all, st) })
			for _, st := range all {
				eachCallIn(st, func(c *ir.Call) { // 4
					if callee := e.op(c.CalledOp()); callee != nil {
						signature(callee)
					}
				})
				eachMatchIn(st, func(m *ir.Match) { // 5
					if cl, ok := e.lookup(m.Classify).(*ir.Classify); ok && cl.DecidedBy == ir.DecidedByPredicate {
						decl(cl.Name)
					}
				})
			}
		}
	}
	var out []string
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// typeDeclOf は、model m が、見る側のために合成したものでないか。
func (e *emitter) typeDeclOf(m *ir.Model) bool {
	return m.SynthesizedFrom == "" || e.op(m.SynthesizedFrom) == nil
}

func eachCallIn(st ir.Statement, fn func(c *ir.Call)) {
	expr := func(x ir.Expr) {
		switch y := x.(type) {
		case ir.CallExpr:
			fn(&y.Call)
		case ir.MatchExpr:
			for j := range y.Match.Arms {
				if ce, ok := y.Match.Arms[j].Body.(ir.CallExpr); ok {
					fn(&ce.Call)
				}
			}
		}
	}
	if x, ok := ir.StatementExpr(st); ok {
		expr(x)
	}
}

func eachMatchIn(st ir.Statement, fn func(m *ir.Match)) {
	if x, ok := ir.StatementExpr(st); ok {
		if m, isMatch := x.(ir.MatchExpr); isMatch {
			fn(&m.Match)
		}
	}
}

// home は、単位 index から見た、名前空間 m の型の置き場の出力（docs/config.md「型の置き場」）: その単位自身が m を出力するなら
// その出力、そうでなければ、基底が同じで m を出力する単位のうち、設定の順で最初のもの。なければ false。
func (e *emitter) home(index int, m string) (outputKey, bool) {
	list := e.view.Namespaces[m]
	for _, c := range list {
		if c.Index == index {
			return outputKey{c.Index, m}, true
		}
	}
	for _, c := range list {
		if c.Base == e.view.Config.Base {
			return outputKey{c.Index, m}, true
		}
	}
	return outputKey{}, false
}

// edges は、出力 k が import する出力。この View の単位の出力は、単位の対象（View.files）で求める。別の単位の出力は、その単位の in が
// View にないので、その名前空間のすべての op を単位の対象として求める（in で絞った単位では、実際より多く数えることがある）。
func (e *emitter) edges(k outputKey) []outputKey {
	inUnit := func(*ir.Op) bool { return true }
	if k.index == e.view.Config.Index {
		inUnit = e.inUnit
	}
	var out []outputKey
	for _, m := range e.uses(k.ns, inUnit, true) {
		if to, ok := e.home(k.index, m); ok {
			out = append(out, to)
		}
	}
	return out
}

// checkImportCycle は GO-3: この出力が、import を辿って自分に戻る。輪に入る出力を、単位と名前空間でメッセージに示す。位置なし。
// 輪に入る出力ごとに（その出力の起動で）出る。
func (e *emitter) checkImportCycle() {
	self := outputKey{e.view.Config.Index, e.view.Viewer}
	// self から、self に戻る道を、幅優先で探す（最も短い輪を示す）
	prev := map[outputKey]outputKey{}
	queue := []outputKey{self}
	visited := map[outputKey]bool{self: true}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		for _, to := range e.edges(k) {
			if to == self {
				path := []outputKey{k}
				for at := k; at != self; {
					at = prev[at]
					path = append(path, at)
				}
				var names []string
				for i := len(path) - 1; i >= 0; i-- {
					names = append(names, label(path[i]))
				}
				names = append(names, label(self))
				e.errorf(codeImportCycle, "", "型のパッケージどうしの import が循環している（型と、値の分類・op の分類が、互いの名前空間の型を使う）: %s。互いに使う型を、一方か、第三の名前空間に寄せる", strings.Join(names, " → "))
				return
			}
			if !visited[to] {
				visited[to] = true
				prev[to] = k
				queue = append(queue, to)
			}
		}
	}
}

func label(k outputKey) string {
	return fmt.Sprintf("%d 番目の [[emit]] の名前空間 %s", k.index, k.ns)
}
