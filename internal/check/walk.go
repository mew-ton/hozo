package check

import "github.com/mew-ton/hozo/ir"

// walkStmts は、配線の文を、辿る順（docs/check.md「Sig(O, v)」の手順 0）に、fn に渡す:
// 書いた順で、中のブロックにも入る。for は、文そのものの後に、ブロックの文を上から。disjoint は、文そのものの後に、
// アームを書いた順に、アームの中の文を上から。loops は、その文を囲んでいる for の番号（外から内へ。本体の中で一意）。
func walkStmts(stmts []ir.Statement, fn func(st ir.Statement, loops []int)) {
	walkAll(stmts, func(st ir.Statement, loops []int, _ []region) { fn(st, loops) })
}

// region は、文を囲んでいる、例外を受ける文のブロック一つ（docs/check.md「Sig(O, v)」の「例外を受ける文」）:
// try は、その文の番号（本体の中で一意。tryInfo の添字）、catch は、catch のブロックか（偽なら、try のブロック）。
type region struct {
	try   int
	catch bool
}

// tryInfo は、例外を受ける文一つ: 受けた例外の名前と、catch のブロックが「返らない」か（Catch.never）。
type tryInfo struct {
	name  string
	never bool
}

// walkAll は、walkStmts に、文を囲んでいる、例外を受ける文のブロック（外から内へ）を足したもの。try は、文そのものの後に、
// try のブロックの文、catch のブロックの文の順に辿る。戻り値は、辿った、例外を受ける文（番号の順）。
func walkAll(stmts []ir.Statement, fn func(st ir.Statement, loops []int, regions []region)) []tryInfo {
	next := 0
	var tries []tryInfo
	var walk func(stmts []ir.Statement, loops []int, regions []region)
	walk = func(stmts []ir.Statement, loops []int, regions []region) {
		for _, st := range stmts {
			fn(st, loops, regions)
			switch s := st.(type) {
			case ir.DisjointStmt:
				for _, arm := range s.Arms {
					walk(arm.Statements, loops, regions)
				}
			case ir.ForStmt:
				next++
				inner := append(append([]int(nil), loops...), next)
				walk(s.Body, inner, regions)
			case ir.TryStmt:
				id := len(tries)
				tries = append(tries, tryInfo{name: s.Catch.Name, never: s.Catch.Never})
				walk(s.Body, loops, append(append([]region(nil), regions...), region{try: id}))
				walk(s.Catch.Body, loops, append(append([]region(nil), regions...), region{try: id, catch: true}))
			}
		}
	}
	walk(stmts, nil, nil)
	return tries
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
		case ir.TryStmt:
			mapStmts(s.Body, fn)
			mapStmts(s.Catch.Body, fn)
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
