package check

import (
	"fmt"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// CheckHomes は CFG-22（docs/config.md「型の置き場」「診断」）。CheckHomes（メソッド）の零値の Checker での呼び出し。
func CheckHomes(p *ir.Program, cfg *config.Config, units []Unit) []diag.Diagnostic {
	return Checker{}.CheckHomes(p, cfg, units)
}

// CheckHomes は CFG-22（docs/config.md「型の置き場」）: 出力 (U, N) が使う宣言の名前空間 M が、U の基底の出力を持たない
// （基底が U と同じ単位のどれも M を出力しない）。単位と、足りない名前空間の組ごとに一つ。基底の決まらない単位
// （Base が ""。CFG-20）では出さない。units は cfg.Units と同じ順で、Program は計算（Compute）が済んでいること
// （Op.Signature.Exceptions を見る）。位置は持たない（hozo.toml）。
func (c Checker) CheckHomes(p *ir.Program, cfg *config.Config, units []Unit) []diag.Diagnostic {
	if p == nil || cfg == nil {
		return nil
	}
	file := cfg.Path
	if file == "" {
		file = config.FileName
	}
	ix := newIndex(p)
	outs := outputs(p, cfg, units)
	// has[B][M] は、名前空間 M が基底 B の出力を持つか
	has := map[string]map[string]bool{}
	for i := range cfg.Units {
		if i >= len(units) || units[i].Base == "" {
			continue
		}
		if has[units[i].Base] == nil {
			has[units[i].Base] = map[string]bool{}
		}
		for _, name := range outs[i] {
			has[units[i].Base][name] = true
		}
	}
	var diags []diag.Diagnostic
	for i := range cfg.Units {
		if i >= len(units) || units[i].Base == "" {
			continue
		}
		u := &cfg.Units[i]
		reported := map[string]bool{}
		for _, name := range outs[i] {
			for _, use := range uses(ix, name, units[i].Files) {
				if has[units[i].Base][use.namespace] || reported[use.namespace] {
					continue
				}
				reported[use.namespace] = true
				diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "CFG-22", File: file,
					Message: fmt.Sprintf("%d 番目の [[emit]]（namespace = %q）は %s を使うが、%s を出力する %s の単位がない。"+
						"[[emit]] namespace = %q、transpilers = [%q] の単位を足す",
						u.Index, u.Namespace, use.decl, displayNamespace(use.namespace), units[i].Base, use.namespace, units[i].Base)})
			}
		}
	}
	return diags
}

// displayNamespace は診断に出す名前空間の名前。グローバルは「グローバルの名前空間」。
func displayNamespace(ns string) string {
	if ns == "" {
		return "グローバルの名前空間"
	}
	return ns
}

// use は、出力が使う別の名前空間の宣言一つ。
type use struct {
	namespace string // 宣言の名前空間
	decl      string // 宣言の FQN
}

// uses は、名前空間 nsName の出力（単位の in に合ったファイルが files）が使う、別の名前空間の宣言
// （docs/config.md「型の置き場」の表）を、名前空間ごとに最初の一つずつ、見つけた順で返す。
//
//  1. N のすべての model / scalar / union / exception: フィールドの型、scalar の元の型、アームの型、例外の引数の型。
//  2. N のすべての状態、分類、read、効果: 状態の型、分類の対象の型、ケースの as の型、read と効果の引数の型、read の結果の型。
//  3. 単位の対象の op: 引数と戻り値の型、Op.signature.exceptions の例外。
//  4. 単位の対象の配線が呼ぶ op（別の名前空間のものを含む）: 引数と戻り値の型。
//  5. 単位の対象の配線の match: 述語で決まる分類のうち、別の名前空間のもの。
//
// 数えるのは、型（ref）、例外、5 の分類が指す宣言の名前空間が N でないもの。型の中の型（array、optional、record、case の of）は
// 辿るが、参照先の宣言の中までは辿らない。デコレータの適用とその引数、別の名前空間の効果と read の宣言そのもの、別の名前空間の op そのもの、
// 型で決まる分類は数えない。
func uses(ix *index, nsName string, files []string) []use {
	ns := ix.p.Namespace(nsName)
	if ns == nil {
		return nil
	}
	var out []use
	seen := map[string]bool{}
	decl := func(fqn string) {
		if _, known := ix.decls[fqn]; !known {
			return
		}
		m := ix.namespaceOf(fqn)
		if m == nsName || seen[m] {
			return
		}
		seen[m] = true
		out = append(out, use{namespace: m, decl: fqn})
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
		case ir.RecordType:
			typ(x.Key)
			typ(x.Value)
		case ir.CaseType:
			typ(x.Of)
		}
	}
	params := func(ps []ir.OpParam) {
		for _, p := range ps {
			typ(p.Type)
		}
	}
	signature := func(o *ir.Op) {
		params(o.Params)
		typ(o.Returns)
	}
	for _, d := range ns.Decls {
		switch x := d.(type) {
		case *ir.Model: // 1
			for _, f := range x.Fields {
				typ(f.Type)
			}
		case *ir.Scalar:
			typ(x.Base)
		case *ir.Union:
			for _, a := range x.Arms {
				typ(a.Type)
			}
		case *ir.Exception:
			params(x.Params)
		case *ir.State: // 2
			typ(x.Type)
		case *ir.Read:
			params(x.Params)
			typ(x.Returns)
		case *ir.Effect:
			params(x.Params)
		case *ir.Classify:
			typ(x.SubjectType())
			for _, cs := range x.Cases {
				for _, as := range cs.As {
					typ(as.Type)
				}
			}
		case *ir.Op:
			if !contains(files, x.Span.File) {
				continue
			}
			signature(x) // 3
			for _, e := range x.Signature.Exceptions {
				decl(e)
			}
			if x.Body == nil {
				continue
			}
			eachCall(x.Body, func(c *ir.Call) { // 4
				if callee := ix.op(c.CalledOp()); callee != nil {
					signature(callee)
				}
			})
			eachMatch(x.Body, func(m *ir.Match) { // 5
				if cl, ok := ix.decls[m.Classify].(*ir.Classify); ok && cl.DecidedBy == ir.DecidedByPredicate {
					decl(cl.Name)
				}
			})
		}
	}
	return out
}

// eachCall は配線のすべての呼び出し（文、束縛、return、match のアーム）に fn を呼ぶ。
func eachCall(w *ir.Wiring, fn func(c *ir.Call)) {
	expr := func(e ir.Expr) {
		switch x := e.(type) {
		case ir.CallExpr:
			fn(&x.Call)
		case ir.MatchExpr:
			for j := range x.Match.Arms {
				if ce, ok := x.Match.Arms[j].Body.(ir.CallExpr); ok {
					fn(&ce.Call)
				}
			}
		}
	}
	walkStmts(w.Statements, func(st ir.Statement, _ []int) {
		if e, _, ok := stmtExpr(st); ok {
			expr(e)
		}
	})
}

// eachMatch は配線のすべての match（文、束縛、return）に fn を呼ぶ。
func eachMatch(w *ir.Wiring, fn func(m *ir.Match)) {
	walkStmts(w.Statements, func(st ir.Statement, _ []int) {
		if e, _, ok := stmtExpr(st); ok {
			if m, isMatch := e.(ir.MatchExpr); isMatch {
				fn(&m.Match)
			}
		}
	})
}
