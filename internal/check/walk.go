package check

import "github.com/mew-ton/hozo/ir"

// walkStmts は、配線の文を、辿る順（docs/check.md「Sig(O, v)」の手順 0）に、fn に渡す:
// 書いた順で、中のブロックにも入る。for は、文そのものの後に、ブロックの文を上から。disjoint は、文そのものの後に、
// アームを書いた順に、アームの中の文を上から。loops は、その文を囲んでいる for の番号（外から内へ。本体の中で一意）。
func walkStmts(stmts []ir.Statement, fn func(st ir.Statement, loops []int)) {
	next := 0
	var walk func(stmts []ir.Statement, loops []int)
	walk = func(stmts []ir.Statement, loops []int) {
		for _, st := range stmts {
			fn(st, loops)
			switch s := st.(type) {
			case ir.DisjointStmt:
				for _, arm := range s.Arms {
					walk(arm.Statements, loops)
				}
			case ir.ForStmt:
				next++
				inner := append(append([]int(nil), loops...), next)
				walk(s.Body, inner)
			}
		}
	}
	walk(stmts, nil)
}

// mapStmts は、配線の文を、辿る順に、fn の結果で置き換える（中のブロックの文も、その場で置き換える）。
// fn は、disjoint と for には、呼ばれない（中の文に、呼ばれる）。
func mapStmts(stmts []ir.Statement, fn func(st ir.Statement) ir.Statement) {
	for i, st := range stmts {
		switch s := st.(type) {
		case ir.DisjointStmt:
			for _, arm := range s.Arms {
				mapStmts(arm.Statements, fn)
			}
		case ir.ForStmt:
			mapStmts(s.Body, fn)
		default:
			stmts[i] = fn(st)
		}
	}
}

// stmtExpr は、文の式と位置。式を持たない文（空の val、disjoint、for）は ok が偽。
func stmtExpr(st ir.Statement) (expr ir.Expr, span ir.Span, ok bool) {
	switch s := st.(type) {
	case ir.ValStmt:
		if s.Expr == nil {
			return nil, s.Span, false
		}
		return s.Expr, s.Span, true
	case ir.AssignStmt:
		return s.Expr, s.Span, true
	case ir.CollectStmt:
		return s.Expr, s.Span, true
	case ir.ReturnStmt:
		return s.Expr, s.Span, true
	case ir.CallStmt:
		return ir.CallExpr{Call: s.Call}, s.Span, true
	case ir.MatchStmt:
		return ir.MatchExpr{Match: s.Match}, s.Span, true
	}
	return nil, ir.Span{}, false
}

// withExpr は、文の式を e に置き換えた文。
func withExpr(st ir.Statement, e ir.Expr) ir.Statement {
	switch s := st.(type) {
	case ir.ValStmt:
		s.Expr = e
		return s
	case ir.AssignStmt:
		s.Expr = e
		return s
	case ir.CollectStmt:
		s.Expr = e
		return s
	case ir.ReturnStmt:
		s.Expr = e
		return s
	case ir.CallStmt:
		if c, ok := e.(ir.CallExpr); ok {
			s.Call = c.Call
		}
		return s
	case ir.MatchStmt:
		if m, ok := e.(ir.MatchExpr); ok {
			s.Match = m.Match
		}
		return s
	}
	return st
}

// subset は a の for が、どれも b にあるか。
func subset(a, b []int) bool {
	for _, x := range a {
		found := false
		for _, y := range b {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// shares は a と b に、同じ for があるか。
func shares(a, b []int) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
