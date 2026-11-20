package sema

import (
	"fmt"
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// wiringJob は配線の本体を下ろす仕事。すべての宣言が下りてから（lowerWirings）行う。
type wiringJob struct {
	sym  *symbol
	decl *ast.OpDecl
	op   *ir.Op
}

// binding は配線のスコープの名前: op の引数、val、for の変数。
type binding struct {
	name    string
	typ     exprType
	span    source.Span
	isParam bool
	used    bool
	target  Target

	// isLoopVar は for の変数か。
	isLoopVar bool
	// empty は、空で宣言した val か（値を入れる文か、集める文で、値が入る）。
	empty bool
	// declType は、宣言に書いた型。書いていなければ nil。
	declType ir.Type
	// unassigned は、代入が、まだ済んでいないか（docs/check.md「値の入れ物」の「代入が済んでいるか」）。
	unassigned bool
	// filled は、値を入れる文か、集める文が、一つでもあったか（なければ WIRE-12）。
	filled bool
	// collected は、集める文で値が入る val か。collectTypes は、集めた式の型（型を書かない val の、型を導くため）。
	collected    bool
	collectTypes []collectedType
	// loops は、宣言した所を囲んでいる for の数。
	loops int
	// irSpan は、IR の val の文の span（後から、型を入れるための鍵）。
	irSpan ir.Span
}

type collectedType struct {
	typ  exprType
	span source.Span
}

func (b *binding) markUsed() { b.used = true }

// loopFrame は、今、中にいる for 一つ。completes は、この for を抜けたときに、集め終わる val。
type loopFrame struct {
	completes []*binding
}

// wiringScope は配線のスコープ（docs/resolve.md「スコープ」）。val は、宣言したブロックの、宣言より下で見える。
type wiringScope struct {
	op     string
	params map[string]*binding
	// frames は、ブロックごとの名前（外から内へ）。
	frames []map[string]*binding
	// order は、宣言した val のすべて（宣言の順。for の変数は入れない）。
	order []*binding
	// narrow は、今見ているアームの中で絞り込まれた型（docs/types.md「アームの中の対象の型」）。キーは narrowKey。
	// アームの本体を評価する間だけ持つ。
	narrow map[string]exprType
	// unassignedRead は、代入の済んでいない val を読んだときに呼ぶ（WIRE-10）。
	unassignedRead func(b *binding, at source.Span)
}

// lookup は、そこで見えている val、for の変数、引数。
func (w *wiringScope) lookup(name string) (*binding, bool) {
	for i := len(w.frames) - 1; i >= 0; i-- {
		if b, ok := w.frames[i][name]; ok {
			return b, true
		}
	}
	b, ok := w.params[name]
	return b, ok
}

// lowerWirings はすべての配線の本体を下ろす。
func (a *analyzer) lowerWirings() {
	for _, j := range a.wirings {
		a.lowerWiring(j)
	}
}

// wiring は一つの配線の下ろしの状態。
type wiring struct {
	job *wiringJob
	// sc は宣言の名前（呼び出す op、分類、例外）を解決するスコープ。
	sc *scope
	// vsc は値の位置（Atom の Path）のスコープ。今の名前空間の状態を名前で読める。
	vsc   *scope
	scope *wiringScope
	// loops は、今、中にいる for（外から内へ）。
	loops []*loopFrame
	// depth は、中のブロック（for のブロック、disjoint のアーム）の深さ。0 は、op の本体のいちばん外。
	depth int
	// arms は、今、中にいる disjoint（外から内へ）。アームごとに、代入の済んだ val を、数え直すため。
	arms []*disjointFrame
	// vals は、IR の val の文の span から、その束縛（後から、型を入れる）。
	vals map[ir.Span]*binding
}

// disjointFrame は、今、中にいる disjoint 一つ。
type disjointFrame struct {
	// arm は、今のアームの番号。
	arm int
	// assigned は、このアームで、代入が済んだ val。
	assigned []*binding
	// by は、val ごとの、値を入れたアームの番号（別のアームが入れる val に、入れたら WIRE-11）。
	by map[*binding]int
}

// lowerWiring は配線の本体（docs/ir.md「配線」）。WIRE-2 / WIRE-3 / WIRE-4 / WIRE-8〜WIRE-14 と式の型。
func (a *analyzer) lowerWiring(j *wiringJob) {
	ws := &wiringScope{op: j.sym.fqn, params: map[string]*binding{}}
	ws.unassignedRead = func(b *binding, at source.Span) {
		a.errorf("WIRE-10", at, "val %s は、まだ、代入が済んでいないので、読めない", b.name).Related = []diag.Related{related(b.span, "宣言")}
	}
	for _, p := range j.op.Params {
		var span source.Span
		if ap := findAstParam(j.decl, p.Name); ap != nil {
			span = ap.Name.Span
		}
		typ := unknownType()
		if p.Type != nil {
			typ = valueType(p.Type)
		}
		ws.params[p.Name] = &binding{name: p.Name, typ: typ, span: span, isParam: true, target: Target{Kind: TargetOpParam, Decl: j.sym.fqn, Name: p.Name, Span: span}}
	}
	w := &wiring{job: j, scope: ws, sc: &scope{ns: j.sym.ns, wiring: ws}, vsc: &scope{ns: j.sym.ns, wiring: ws, values: true}, vals: map[ir.Span]*binding{}}
	body := &ir.Wiring{}
	stmts := j.decl.Body.Stmts
	body.Statements = a.lowerBlock(w, stmts)
	a.checkWIRE3Last(j, stmts)
	a.checkWIRE8(ws)
	a.fillVals(w, body.Statements)
	j.op.Body = body
}

// lowerBlock は、ブロック一つの文の並び。名前の枠を一つ足し、抜けるときに WIRE-12 を見る。
func (a *analyzer) lowerBlock(w *wiring, stmts []ast.Stmt) []ir.Statement {
	frame := map[string]*binding{}
	w.scope.frames = append(w.scope.frames, frame)
	var out []ir.Statement
	for i, s := range stmts {
		if st, ok := a.lowerStmt(w, s, i == len(stmts)-1); ok {
			out = append(out, st)
		}
	}
	w.scope.frames = w.scope.frames[:len(w.scope.frames)-1]
	for _, b := range frame {
		if b.empty && !b.filled {
			a.errorf("WIRE-12", b.span, "空で宣言した val %s に、値を入れる文も、集める文も、ない", b.name)
		}
		a.finishCollect(b)
	}
	return out
}

// fillVals は、IR の val の文に、解決した型と、collected を入れる（docs/ir.md「配線」）。
func (a *analyzer) fillVals(w *wiring, stmts []ir.Statement) {
	for i, s := range stmts {
		switch s := s.(type) {
		case ir.ValStmt:
			if b, ok := w.vals[s.Span]; ok {
				s.Type, s.Collected = b.typ.typ, b.collected
				stmts[i] = s
			}
		case ir.DisjointStmt:
			for _, arm := range s.Arms {
				a.fillVals(w, arm.Statements)
			}
		case ir.ForStmt:
			a.fillVals(w, s.Body)
		}
	}
}

func findAstParam(d *ast.OpDecl, name string) *ast.OpParam {
	for _, p := range d.Params {
		if p != nil && p.Name.Name == name {
			return p
		}
	}
	return nil
}

// checkWIRE3Last は WIRE-3（docs/check.md「配線」）の「returns が null でないのに、本体の最後の文が return でない」。
func (a *analyzer) checkWIRE3Last(j *wiringJob, stmts []ast.Stmt) {
	if j.op.Returns == nil {
		return
	}
	if len(stmts) == 0 {
		a.errorf("WIRE-3", j.sym.nameSpan, "op %s は値を返すが、本体に return がない", j.sym.fqn)
		return
	}
	if _, ok := stmts[len(stmts)-1].(*ast.ReturnStmt); !ok && !hasReturn(stmts) {
		a.errorf("WIRE-3", j.sym.nameSpan, "op %s は値を返すので、本体の最後の文は return でなければならない", j.sym.fqn)
	}
}

// hasReturn は return の文があるか（最後以外にあれば lowerReturn が WIRE-3 を出している）。
func hasReturn(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		if _, ok := s.(*ast.ReturnStmt); ok {
			return true
		}
	}
	return false
}

// checkWIRE8 は WIRE-8（docs/check.md「配線」。warning）: 代入の済んだ val が、どこからも読まれていない。
func (a *analyzer) checkWIRE8(ws *wiringScope) {
	for _, b := range ws.order {
		if !b.used && (!b.empty || b.filled) {
			a.warnf("WIRE-8", b.span, "val %s は使われていない", b.name)
		}
	}
}

// lowerStmt は一つの文。last は、今のブロックの最後の文か。
func (a *analyzer) lowerStmt(w *wiring, s ast.Stmt, last bool) (ir.Statement, bool) {
	switch s := s.(type) {
	case *ast.ValStmt:
		return a.lowerVal(w, s)
	case *ast.AssignStmt:
		return a.lowerAssign(w, s)
	case *ast.CollectStmt:
		return a.lowerCollect(w, s)
	case *ast.ReturnStmt:
		if w.depth > 0 {
			a.errorf("WIRE-13", s.Span, "for のブロックと、disjoint のアームの中に、return は書けない")
			a.evalExpr(w, s.Value, nil)
			return nil, false
		}
		return a.lowerReturn(w, s, last)
	case *ast.CallStmt:
		if s.Call == nil {
			return nil, false
		}
		t, call, ok := a.evalCall(w, s.Call, nil)
		if ok && hasValue(t) {
			a.errorf("WIRE-2", s.Call.Span, "呼び出し %s は値を返すので、val に受けずに書けない", call.Callee)
		}
		a.checkWIRE9(w, t, s.Span, last)
		if !ok {
			return nil, false
		}
		return ir.CallStmt{Call: call, Span: a.irSpan(s.Span)}, true
	case *ast.MatchStmt:
		if s.Match == nil {
			return nil, false
		}
		t, m := a.evalMatch(w, s.Match, nil)
		if hasValue(t) {
			a.errorf("WIRE-2", s.Match.Span, "match は値を持つので、val に受けずに書けない")
		}
		a.checkWIRE9(w, t, s.Span, last)
		return ir.MatchStmt{Match: m, Span: a.irSpan(s.Span)}, true
	case *ast.DisjointStmt:
		return a.lowerDisjoint(w, s), true
	case *ast.ForStmt:
		return a.lowerFor(w, s)
	}
	return nil, false
}

// hasValue は式に値があるか（「値がない」「返らない」と、解決できなかった式を除く）。
func hasValue(t exprType) bool {
	return t.kind != etNoValue && t.kind != etNever && t.kind != etUnknown
}

// checkWIRE9 は WIRE-9（docs/check.md「配線」）: 「返らない」式の文が、本体の最後以外にある（下の行が評価されない）。
// for のブロックと、disjoint のアームの中では、どこにあっても。
func (a *analyzer) checkWIRE9(w *wiring, t exprType, span source.Span, last bool) {
	if t.kind != etNever {
		return
	}
	if w.depth > 0 {
		a.errorf("WIRE-9", span, "この文は決して返らないので、for のブロックと、disjoint のアームの中には書けない")
	} else if !last {
		a.errorf("WIRE-9", span, "この文は決して返らないので、本体の最後にだけ書ける（下の行が評価されない）")
	}
}

// declareVal は、val か、for の変数を、今のブロックに足す。見えている名前と同じなら WIRE-4。
func (a *analyzer) declareVal(w *wiring, b *binding, what string) bool {
	if prev, dup := w.scope.lookup(b.name); dup {
		kind := "val"
		switch {
		case prev.isParam:
			kind = "引数"
		case prev.isLoopVar:
			kind = "for の変数"
		}
		a.errorf("WIRE-4", b.span, "%s は既に%sの名前（見えている名前と同じ名前は、%sに使えない）", b.name, kind, what).Related = []diag.Related{related(prev.span, "最初の定義")}
		return false
	}
	b.loops = len(w.loops)
	w.scope.frames[len(w.scope.frames)-1][b.name] = b
	if !b.isLoopVar {
		w.scope.order = append(w.scope.order, b)
	}
	return true
}

// markAssigned は、val の代入が済んだことにする。disjoint のアームの中なら、そのアームの分として覚える。
func (w *wiring) markAssigned(b *binding) {
	b.unassigned = false
	for _, f := range w.arms {
		f.assigned = append(f.assigned, b)
	}
}

// valueOf は、val に入れる式（値と一緒の宣言、値を入れる文）を評価する。b の型を書いていれば、そこへ渡せること（TYPE-17）。
// 書いていなければ、式の型を、b の型にする（docs/types.md「val の型」）。
func (a *analyzer) valueOf(w *wiring, b *binding, name string, value ast.Expr) (ir.Expr, bool) {
	var ctx *returnContext
	if b.declType != nil {
		ctx = &returnContext{returns: b.declType}
	}
	t, expr, ok := a.evalExpr(w, value, ctx)
	switch t.kind {
	case etNoValue:
		a.errorf("WIRE-2", value.NodeSpan(), "val %s に入れる式は値を持たない", name)
		t = unknownType()
	case etNever:
		a.errorf("WIRE-2", value.NodeSpan(), "val %s に入れる式は決して返らない（val に受けられない）", name)
		t = unknownType()
	case etRecord:
		if b.declType == nil {
			a.errorf("TYPE-7", value.NodeSpan(), "レコードリテラルの当てはめる先が決まらない（val %s）。val に型を書くか、引数か return の位置に書く", name)
			t = unknownType()
		}
	}
	if b.declType != nil {
		if t.kind != etUnknown && a.assign(t, b.declType) != assignOK {
			a.errorf("TYPE-17", value.NodeSpan(), "式の型 %s は、val %s の型 %s に渡せない", exprName(t), name, typeName(b.declType))
		}
		b.typ = valueType(b.declType)
	} else {
		b.typ = t
	}
	return expr, ok
}

// lowerVal は val の宣言一つ（docs/check.md「値の入れ物」）。WIRE-4、WIRE-2、TYPE-7、TYPE-17。
func (a *analyzer) lowerVal(w *wiring, s *ast.ValStmt) (ir.Statement, bool) {
	name := s.Name.Name
	b := &binding{name: name, typ: unknownType(), span: s.Name.Span, target: Target{Kind: TargetBinding, Decl: w.job.sym.fqn, Name: name, Span: s.Name.Span}}
	if s.Type != nil {
		if t, ok := a.lowerType(s.Type, w.sc); ok {
			b.declType = t
			b.typ = valueType(t)
		}
	}
	span := a.irSpan(s.Span)
	b.irSpan = span
	if s.Value == nil {
		b.empty, b.unassigned = true, true
		if a.declareVal(w, b, "val の宣言") {
			w.vals[span] = b
		}
		return ir.ValStmt{Name: name, Span: span}, true
	}
	// 式を先に読む: 式の中から、宣言している名前は、見えない
	expr, ok := a.valueOf(w, b, name, s.Value)
	if a.declareVal(w, b, "val の宣言") {
		w.vals[span] = b
	}
	if !ok {
		return nil, false
	}
	return ir.ValStmt{Name: name, Expr: expr, Span: span}, true
}

// lowerAssign は値を入れる文 `x = 式;`（docs/check.md「値の入れ物」の「値を入れる文」）。WIRE-11、WIRE-2、TYPE-17。
func (a *analyzer) lowerAssign(w *wiring, s *ast.AssignStmt) (ir.Statement, bool) {
	name := s.Name.Name
	b, found := w.scope.lookup(name)
	if !found {
		a.errorf("RES-1", s.Name.Span, "名前 %s が見つからない（値を入れられるのは、空で宣言した val）", name)
		a.evalExpr(w, s.Value, nil)
		return nil, false
	}
	a.refs = append(a.refs, Ref{Span: s.Name.Span, Target: b.target})
	bad := ""
	switch {
	case b.isParam:
		bad = "引数には、値を入れられない"
	case b.isLoopVar:
		bad = "for の変数には、値を入れられない"
	case !b.empty:
		bad = "値と一緒に宣言した val には、もう、値を入れられない"
	case !b.unassigned:
		bad = "もう、代入が済んでいる（値が入るのは、一度だけ）"
	case len(w.loops) != b.loops:
		bad = "for の中から、その for の外の val には、値を入れられない（回ごとに入るため。外に出すには、collect で集める）"
	}
	if bad == "" {
		for _, f := range w.arms {
			if arm, ok := f.by[b]; ok && arm != f.arm {
				bad = "同じ disjoint の、別のアームが、もう、値を入れている"
				break
			}
		}
	}
	if bad != "" {
		a.errorf("WIRE-11", s.Name.Span, "%s に値を入れられない: %s", name, bad).Related = []diag.Related{related(b.span, "宣言")}
		a.evalExpr(w, s.Value, nil)
		return nil, false
	}
	expr, ok := a.valueOf(w, b, name, s.Value)
	b.filled = true
	for _, f := range w.arms {
		f.by[b] = f.arm
	}
	w.markAssigned(b)
	if !ok {
		return nil, false
	}
	return ir.AssignStmt{Name: name, Expr: expr, Span: a.irSpan(s.Span)}, true
}

// lowerCollect は集める文 `xs collect 式;`（docs/check.md「繰り返し」の「集める」）。WIRE-14、WIRE-2、TYPE-7、TYPE-17。
func (a *analyzer) lowerCollect(w *wiring, s *ast.CollectStmt) (ir.Statement, bool) {
	name := s.Name.Name
	b, found := w.scope.lookup(name)
	if !found {
		a.errorf("RES-1", s.Name.Span, "名前 %s が見つからない（集められるのは、for の外で、空で宣言した val）", name)
		a.evalExpr(w, s.Value, nil)
		return nil, false
	}
	a.refs = append(a.refs, Ref{Span: s.Name.Span, Target: b.target})
	bad := ""
	switch {
	case len(w.loops) == 0:
		bad = "集める文は、for の中にだけ書ける"
	case b.isParam, b.isLoopVar, !b.empty:
		bad = "集められるのは、空で宣言した val だけ"
	case !b.unassigned:
		bad = "もう、代入が済んでいる（一つの val に集められるのは、一つの for の中からだけ）"
	case len(w.loops) <= b.loops:
		bad = "集める先の val は、for の外で宣言する"
	}
	if bad != "" {
		a.errorf("WIRE-14", s.Name.Span, "%s に集められない: %s", name, bad).Related = []diag.Related{related(b.span, "宣言")}
		a.evalExpr(w, s.Value, nil)
		return nil, false
	}
	var elem ir.Type
	if b.declType != nil {
		arr, isArray := b.declType.(ir.ArrayType)
		if !isArray {
			a.errorf("TYPE-17", s.Name.Span, "集める先の val %s の型 %s は、並び（T[]）でない", name, typeName(b.declType))
		} else {
			elem = arr.Of
		}
	}
	var ctx *returnContext
	if elem != nil {
		ctx = &returnContext{returns: elem}
	}
	t, expr, ok := a.evalExpr(w, s.Value, ctx)
	switch t.kind {
	case etNoValue:
		a.errorf("WIRE-2", s.Value.NodeSpan(), "%s に集める式は値を持たない", name)
		t = unknownType()
	case etNever:
		a.errorf("WIRE-2", s.Value.NodeSpan(), "%s に集める式は決して返らない", name)
		t = unknownType()
	case etRecord:
		if elem == nil {
			a.errorf("TYPE-7", s.Value.NodeSpan(), "レコードリテラルの当てはめる先が決まらない（%s に集める式）。集める先の val に型を書く", name)
			t = unknownType()
		}
	}
	if elem != nil {
		if t.kind != etUnknown && a.assign(t, elem) != assignOK {
			a.errorf("TYPE-17", s.Value.NodeSpan(), "式の型 %s は、%s の要素の型 %s に渡せない", exprName(t), name, typeName(elem))
		}
	} else if b.declType == nil {
		b.collectTypes = append(b.collectTypes, collectedType{typ: t, span: s.Value.NodeSpan()})
	}
	if !b.collected {
		b.collected, b.filled = true, true
		loop := w.loops[b.loops]
		loop.completes = append(loop.completes, b)
	}
	if !ok {
		return nil, false
	}
	return ir.CollectStmt{Name: name, Expr: expr, Span: a.irSpan(s.Span)}, true
}

// finishCollect は、型を書かない、集める先の val の型を、集めた式の型から導く（docs/types.md「val の型」の 2）:
// ほかのすべてが渡せる型 U の、並び。そういう型がなければ TYPE-17。
func (a *analyzer) finishCollect(b *binding) {
	if !b.collected || b.declType != nil || len(b.collectTypes) == 0 {
		return
	}
	types := b.collectTypes
	b.collectTypes = nil
	var known []collectedType
	for _, c := range types {
		if c.typ.typ != nil && (c.typ.kind == etValue || c.typ.kind == etLiteral) {
			known = append(known, c)
		}
	}
	if len(known) == 0 {
		return
	}
	for _, cand := range known {
		ok := true
		for _, other := range types {
			if other.typ.kind != etUnknown && a.assign(other.typ, cand.typ.typ) != assignOK {
				ok = false
				break
			}
		}
		if ok {
			b.typ = valueType(ir.ArrayType{Of: cand.typ.typ})
			return
		}
	}
	a.errorf("TYPE-17", known[0].span, "val %s に集める式の型が、そろわない（ほかのすべてが渡せる型が、ない）", b.name)
}

// lowerDisjoint は `disjoint { アーム … }`（docs/check.md「disjoint」）。どのアームも、disjoint の前の、代入の済んだ集まりから始める。
// アームどうしの検査（DISJ-1〜DISJ-3）は、Signature が要るので、internal/check が行う。
func (a *analyzer) lowerDisjoint(w *wiring, s *ast.DisjointStmt) ir.Statement {
	f := &disjointFrame{by: map[*binding]int{}}
	w.arms = append(w.arms, f)
	w.depth++
	out := ir.DisjointStmt{Span: a.irSpan(s.Span)}
	var all []*binding
	for i, arm := range s.Arms {
		f.arm, f.assigned = i, nil
		stmts := a.lowerBlock(w, arm.Stmts)
		out.Arms = append(out.Arms, ir.Arm{Block: arm.Block, Statements: stmts, Span: a.irSpan(arm.Span)})
		// 次のアームからは、このアームが入れた val は、済んでいない
		for _, b := range f.assigned {
			b.unassigned = true
		}
		all = append(all, f.assigned...)
	}
	w.depth--
	w.arms = w.arms[:len(w.arms)-1]
	// 抜けたら、どのアームが入れた val も、済んでいる（外の disjoint のアームの分としても、覚える）
	for _, b := range all {
		w.markAssigned(b)
	}
	return out
}

// lowerFor は `[disjoint] for (val v of 並び) { … }`（docs/check.md「繰り返し」）。TYPE-17（並びでない）、WIRE-4（変数の名前）。
func (a *analyzer) lowerFor(w *wiring, s *ast.ForStmt) (ir.Statement, bool) {
	t, seq := a.evalAtom(w, s.Seq)
	elem := unknownType()
	var elemType ir.Type
	switch {
	case t.kind == etUnknown:
	case t.kind == etValue && t.typ != nil:
		if arr, ok := t.typ.(ir.ArrayType); ok {
			elem, elemType = valueType(arr.Of), arr.Of
		} else {
			a.errorf("TYPE-17", s.Seq.NodeSpan(), "for の並びの型 %s は、並び（T[]）でない", typeName(t.typ))
		}
	default:
		a.errorf("TYPE-17", s.Seq.NodeSpan(), "for の並びは、型が並び（T[]）の値でなければならない")
	}
	loop := &loopFrame{}
	w.loops = append(w.loops, loop)
	w.depth++
	// 変数は、ブロックの中だけで見える: 変数だけの枠を、ブロックの枠の外に、一つ足す
	w.scope.frames = append(w.scope.frames, map[string]*binding{})
	v := &binding{name: s.Var.Name, typ: elem, span: s.Var.Span, isLoopVar: true, used: true, target: Target{Kind: TargetBinding, Decl: w.job.sym.fqn, Name: s.Var.Name, Span: s.Var.Span}}
	a.declareVal(w, v, "for の変数")
	v.used = false
	var body []ir.Statement
	if s.Body != nil {
		body = a.lowerBlock(w, s.Body.Stmts)
	}
	w.scope.frames = w.scope.frames[:len(w.scope.frames)-1]
	w.depth--
	w.loops = w.loops[:len(w.loops)-1]
	// この for を抜けて、集め終わる val
	for _, b := range loop.completes {
		a.finishCollect(b)
		w.markAssigned(b)
	}
	if seq == nil {
		return nil, false
	}
	return ir.ForStmt{Var: s.Var.Name, VarType: elemType, Of: seq, Body: body, Disjoint: s.Disjoint, Span: a.irSpan(s.Span)}, true
}

// lowerReturn は `return 式;`。WIRE-3（void の op に return、最後以外の行の return）、WIRE-2（値がない）、TYPE-3（returns に渡せない）。
// 「返らない」式は、どの型の return にも書ける（docs/types.md「式の型」）。
func (a *analyzer) lowerReturn(w *wiring, s *ast.ReturnStmt, last bool) (ir.Statement, bool) {
	op := w.job.op
	if op.Returns == nil {
		a.errorf("WIRE-3", s.Span, "op %s は値を返さないので return を書けない", op.Name)
	} else if !last {
		a.errorf("WIRE-3", s.Span, "return は本体の最後の文にだけ書ける（下の行が評価されないため）")
	}
	t, expr, ok := a.evalExpr(w, s.Value, &returnContext{returns: op.Returns})
	if !ok {
		return nil, false
	}
	if t.kind == etNoValue {
		a.errorf("WIRE-2", s.Value.NodeSpan(), "return の式は値を持たない")
	} else if op.Returns != nil && a.assign(t, op.Returns) != assignOK {
		a.errorf("TYPE-3", s.Value.NodeSpan(), "return の値の型 %s は op %s の戻り値の型 %s に渡せない", exprName(t), op.Name, typeName(op.Returns))
	}
	return ir.ReturnStmt{Expr: expr, Span: a.irSpan(s.Span)}, true
}

// returnContext は `return` の式であること。match のアームがレコードリテラルと none だけのとき、当てはめる先を
// op の returns から決めるために渡す（docs/types.md「match の型」の 5.3）。
type returnContext struct {
	returns ir.Type
}

// evalExpr は式の型（docs/types.md「式の型」）と IR の式。ret は式が `return` の式なら nil でない。
func (a *analyzer) evalExpr(w *wiring, e ast.Expr, ret *returnContext) (exprType, ir.Expr, bool) {
	switch e := e.(type) {
	case nil:
		return unknownType(), nil, false
	case *ast.Call:
		t, call, ok := a.evalCall(w, e, nil)
		return t, ir.CallExpr{Call: call}, ok
	case *ast.Match:
		t, m := a.evalMatch(w, e, ret)
		return t, ir.MatchExpr{Match: m}, true
	case ast.Atom:
		t, atom := a.evalAtom(w, e)
		return t, ir.AtomExpr{Atom: atom}, atom != nil
	}
	return unknownType(), nil, false
}

// armContext は値を対象にした match のアームの本体の文脈。呼び出しに対象と同じ Atom をそのまま渡していて、
// アームの中の対象の型では渡せなければ、TYPE-4 ではなく MATCH-4 にする。
type armContext struct {
	// subject は対象の Atom の同一性（narrowKey）。対象が名前でなければ空。
	subject string
	cases   string
	// classify は match の分類の FQN（標準の分類なら空）。known は、このアームが受けるケース。outer は、対象の、アームの外での型。
	// 行の遷移の、呼ぶ側の検査（STATE-9。docs/types.md「ケースが分かっている値」）が使う。
	classify string
	known    []string
	outer    exprType
}

// callTarget は呼び出し先の、引数の並びと呼び名（docs/types.md「呼び出しの引数」の表の一行）。
type callTarget struct {
	// what は診断での呼び名（"op a.f"、"read a.r"、"効果 a.e"、"状態 a.s の post"、"例外 a.E"）。
	what   string
	params []ir.OpParam
}

// evalCall は配線の呼び出し（docs/check.md「配線からの呼び出し」）。名前を解決し、行き着いたものの種類で、何を呼ぶかを
// 決める（docs/resolve.md「呼び出しの読み分け」）: op、read、効果（effect の宣言）、直に呼ぶ組み込みの効果（状態.操作）。
// 位置の引数を、呼び出し先の引数名に対応付け（docs/ir.md「正規化」）、型を照合する（TYPE-4、MATCH-4）。
// 別の名前空間の、export のない op なら NS-5、別の名前空間の read か効果なら NS-6。Bound は nil のまま
// （docs/check.md「bind」が、op の呼び出しにだけ埋める）。
func (a *analyzer) evalCall(w *wiring, c *ast.Call, arm *armContext) (exprType, ir.Call, bool) {
	call := ir.Call{Span: a.irSpan(c.Span)}
	args := a.evalArgs(w, c.Args)
	r, ok := a.resolve(c.Callee, w.sc, acCallPos)
	if !ok {
		return unknownType(), call, false
	}
	var target callTarget
	var result exprType
	switch {
	case r.kind == rStateOp:
		direct, ok := a.directEffect(w, r, c.Callee)
		if !ok {
			return unknownType(), call, false
		}
		call.Callee = ir.EffectCallee{Effect: direct.effect}
		if !direct.valid {
			// 呼べない操作（STATE-1 / STATE-6）には、引数の誤りを重ねて報告しない
			return noValue(), call, true
		}
		target, result = direct.target, noValue()
	case r.sym.kind == kindOp:
		op, ok := a.lower(r.sym).(*ir.Op)
		if !ok {
			return unknownType(), call, false
		}
		a.checkNS5(r.sym, w.sc.ns, c.Callee.Span)
		call.Callee = ir.OpCallee{Op: op.Name}
		target, result = callTarget{what: "op " + op.Name, params: op.Params}, callType(op.Returns)
	case r.sym.kind == kindRead:
		rd, ok := a.lower(r.sym).(*ir.Read)
		if !ok {
			return unknownType(), call, false
		}
		a.checkNS6(r.sym, w.sc.ns, c.Callee.Span)
		call.Callee = ir.ReadCallee{Read: rd.Name}
		target, result = callTarget{what: "read " + rd.Name, params: rd.Params}, unknownType()
		if rd.Returns != nil {
			result = valueType(rd.Returns)
		}
	case r.sym.kind == kindEffect:
		e, ok := a.lower(r.sym).(*ir.Effect)
		if !ok {
			return unknownType(), call, false
		}
		a.checkNS6(r.sym, w.sc.ns, c.Callee.Span)
		call.Callee = ir.EffectCallee{Effect: ir.NamedEffect{Effect: e.Name}}
		target, result = callTarget{what: "効果 " + e.Name, params: e.Params}, noValue()
		a.checkSTATE9(e, args, arm)
	default:
		return unknownType(), call, false
	}
	call.Args = a.bindArgs(target, args, arm)
	return result, call, true
}

// checkSTATE9 は STATE-9（docs/check.md「行の遷移」の「呼ぶ側の検査」）: 矢印の前にケースを並べた行の遷移を持つ効果 e を、
// そのケースにあると分かっていない行で呼んだ。行の引数に渡した値が、効果の分類について、ケースが分かっている値
// （docs/types.md「ケースが分かっている値」）で、分かっているケースのすべてが、前のケースの並びに入っていなければならない。
// 前が any の形と、post の効果（IR の from は、いつも any）は、どの行でも受け取る。行の引数に値を渡していない呼び出しは、
// BIND-4 に任せる。
func (a *analyzer) checkSTATE9(e *ir.Effect, args []evaluatedArg, arm *armContext) {
	row := e.Row
	if row == nil || row.From.Any || row.Classify == "" {
		return
	}
	var arg *evaluatedArg
	for i, p := range e.Params {
		if p.Name == row.Param && i < len(args) {
			arg = &args[i]
		}
	}
	if arg == nil {
		return
	}
	// ケースが分かっているか: 1. アームから。2. 型から（アームの中での型と、宣言したままの型）
	var known [][]string
	inArm := arm != nil && arm.subject != "" && arg.key == arm.subject
	if inArm && arm.classify == row.Classify {
		known = append(known, arm.known)
	}
	types := []exprType{arg.typ}
	if inArm {
		types = append(types, arm.outer)
	}
	for _, t := range types {
		if t.kind != etValue {
			continue
		}
		if ct, ok := t.typ.(ir.CaseType); ok && ct.Classify == row.Classify {
			known = append(known, ct.Cases)
		}
	}
	for _, ks := range known {
		if len(ks) > 0 && subset(ks, row.From.Cases) {
			return
		}
	}
	from := strings.Join(row.From.Cases, ", ")
	if len(known) == 0 {
		a.errorf("STATE-9", arg.span, "効果 %s は、%s の %s にある行でしか呼べないが、%s が、そのケースにあると分かっていない。match %s by %s のアームの中で呼ぶか、型が %s.<ケース><…> の値を渡す（docs/check.md「行の遷移」の「呼ぶ側の検査」）",
			e.Name, row.Classify, from, arg.text, arg.text, row.Classify, row.Classify)
		return
	}
	a.errorf("STATE-9", arg.span, "効果 %s は、%s の %s にある行でしか呼べないが、%s について分かっているのは %s（docs/check.md「行の遷移」の「呼ぶ側の検査」）",
		e.Name, row.Classify, from, arg.text, strings.Join(known[0], ", "))
}

// bindArgs は位置で渡した値を、呼び出し先の引数名に対応付け、型を照合する（docs/types.md「呼び出しの引数」）。
// 渡した値の数が引数の数を超えれば TYPE-4（位置は、超えた最初の値）。足りない必須の引数（BIND-4）は internal/check が見る。
func (a *analyzer) bindArgs(target callTarget, args []evaluatedArg, arm *armContext) []ir.CallArg {
	var out []ir.CallArg
	for i, arg := range args {
		if i >= len(target.params) {
			a.errorf("TYPE-4", arg.span, "%s の引数は %d 個で、渡した引数が多い", target.what, len(target.params))
			break
		}
		p := target.params[i]
		out = append(out, ir.CallArg{Param: p.Name, Value: arg.atom})
		a.checkArg(target.what, p, arg, arm)
	}
	return out
}

// directCall は直に呼ぶ組み込みの効果（docs/check.md「直に呼ぶ組み込みの効果」）。
type directCall struct {
	effect ir.DirectEffect
	target callTarget
	// valid は、その状態の形で直に呼べる操作か（STATE-1 にも STATE-6 にも当たらない）。
	valid bool
}

// directEffect は配線で直に呼んだ「状態.操作(…)」。状態は今の名前空間の基底の状態（別の名前空間のものなら RES-5）。
// 操作がその状態の形になければ STATE-1、list の put / delete なら STATE-6（どの列かを引数から決める規則がない）。
// 引数は、値を取る操作なら状態の要素の型の値一つ（名前は ir.DirectEffectParam）、nullable の delete はなし。
func (a *analyzer) directEffect(w *wiring, r resolved, callee ast.QualifiedName) (directCall, bool) {
	if r.sym.ns != w.sc.ns {
		a.errorf("RES-5", callee.Span, "状態 %s は別の名前空間のもので、その操作は直には呼べない（状態を書けるのは持ち主の名前空間 %s の配線だけ。その名前空間の op を通す）", r.sym.fqn, nsName(r.sym.ns))
		return directCall{}, false
	}
	st, ok := a.lower(r.sym).(*ir.State)
	if !ok {
		return directCall{}, false
	}
	opSpan := callee.Last().Span
	out := directCall{
		effect: ir.DirectEffect{State: st.Name, Operation: r.op},
		target: callTarget{what: fmt.Sprintf("状態 %s の %s", st.Name, r.op)},
	}
	if !a.checkSTATE1(r.sym, r.op, opSpan) {
		return out, true
	}
	if st.Shape == ir.StateList && r.op != ir.OpPost {
		a.errorf("STATE-6", opSpan, "list の状態 %s の %s は、直には呼べない（どの列かを、引数から決める規則がない。名前を付けた effect を宣言して呼ぶ: effect fn 名前(…): effects %s.%s;。docs/check.md「直に呼ぶ組み込みの効果」）", st.Name, r.op, lastSegment(st.Name), r.op)
		return out, true
	}
	if r.op != ir.OpDelete && st.Element != nil {
		out.target.params = []ir.OpParam{{Name: ir.DirectEffectParam, Type: st.Element}}
	}
	out.valid = true
	return out, true
}

// lastSegment は FQN の最後の要素。
func lastSegment(fqn string) string {
	if i := strings.LastIndex(fqn, "."); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

// checkNS5 は NS-5（docs/check.md「名前空間」）: 別の名前空間の、export のない op を呼んだ。位置は呼び出しの op の名前
// （デコレータの引数ならその引数の値）。Related にその op の宣言。from は呼ぶ側の名前空間。
func (a *analyzer) checkNS5(op *symbol, from *namespace, span source.Span) {
	d, ok := op.node.(*ast.OpDecl)
	if !ok || d.Export || op.ns == from {
		return
	}
	a.errorf("NS-5", span, "op %s は名前空間 %s の内部の op で、別の名前空間 %s からは呼べない（呼べるようにするには export を付ける）", op.fqn, nsName(op.ns), nsName(from)).
		Related = []diag.Related{related(op.nameSpan, "op %s の宣言", op.fqn)}
}

// checkNS6 は NS-6（docs/check.md「名前空間」）: 配線が、別の名前空間の read か効果（effect の宣言）を呼んだ。
// read と効果を使えるのは、宣言した名前空間（状態の持ち主）の配線だけ。位置は呼び出しの名前。Related にその宣言。
func (a *analyzer) checkNS6(decl *symbol, from *namespace, span source.Span) {
	if decl.ns == from {
		return
	}
	a.errorf("NS-6", span, "%s %s は名前空間 %s のもので、別の名前空間 %s の配線からは呼べない（read と効果を呼べるのは、宣言した名前空間の配線だけ。その名前空間の op を通す）", decl.kind, decl.fqn, nsName(decl.ns), nsName(from)).
		Related = []diag.Related{related(decl.nameSpan, "%s %s の宣言", decl.kind, decl.fqn)}
}

// evaluatedArg は評価した引数。
type evaluatedArg struct {
	typ  exprType
	atom ir.Atom
	span source.Span
	// key は名前の Atom の同一性（narrowKey）。名前でなければ空。
	key  string
	text string
}

func (a *analyzer) evalArgs(w *wiring, atoms []ast.Atom) []evaluatedArg {
	out := make([]evaluatedArg, 0, len(atoms))
	for _, at := range atoms {
		if at == nil {
			continue
		}
		t, atom, key := a.evalAtomKey(w, at)
		out = append(out, evaluatedArg{typ: t, atom: atom, span: at.NodeSpan(), key: key, text: a.text(at.NodeSpan())})
	}
	return out
}

// checkArg は引数の型の照合（TYPE-4 / TYPE-1）。アームの中の対象は、絞り込まれた型で評価されている。
// 対象と同じ Atom をそのまま渡していて渡せなければ MATCH-4。
func (a *analyzer) checkArg(what string, p ir.OpParam, arg evaluatedArg, arm *armContext) {
	if p.Type == nil {
		return
	}
	fail := a.assign(arg.typ, p.Type)
	if fail == assignOK {
		return
	}
	if arm != nil && arm.subject != "" && arg.key == arm.subject {
		a.errorf("MATCH-4", arg.span, "アーム %s の中で %s の型は %s で、%s の引数 %s の型 %s に渡せない", arm.cases, arg.text, exprName(arg.typ), what, p.Name, typeName(p.Type))
		return
	}
	a.errorf(string(fail), arg.span, "%s", failMessage(fail, fmt.Sprintf("%s の引数 %s", what, p.Name), arg.typ, p.Type))
}

// evalAtom は Atom の型と IR。
func (a *analyzer) evalAtom(w *wiring, at ast.Atom) (exprType, ir.Atom) {
	t, atom, _ := a.evalAtomKey(w, at)
	return t, atom
}

// evalAtomKey は Atom の型と IR と、名前の Atom の同一性（narrowKey。名前でなければ空）。
// 名前は束縛・引数・今の名前空間の状態と、そのフィールド（docs/resolve.md「位置ごとに受け付ける種類」の
// 「配線の値の位置の Path」）。束縛と引数は Atom.name、状態は Atom.state になる。別の名前空間の状態は RES-5。
// アームの中では、対象は絞り込まれた型になる（スコープが持つ）。
func (a *analyzer) evalAtomKey(w *wiring, at ast.Atom) (exprType, ir.Atom, string) {
	switch at := at.(type) {
	case *ast.NameAtom:
		span := a.irSpan(at.Span)
		r, ok := a.resolve(at.Name, w.vsc, acValue|acState)
		if !ok {
			return unknownType(), ir.NameAtom{Path: segmentsOf(at.Name), Span: span}, ""
		}
		v := r.value
		if r.kind == rDecl {
			// 状態そのもの
			sv, err := a.stateValue(r.sym, w.vsc, at.Name.Span)
			if err != nil {
				a.reportResolve(err)
				return unknownType(), ir.NameAtom{Path: segmentsOf(at.Name), Span: span}, ""
			}
			v = sv
		}
		key := narrowKey(v.root, v.fields)
		if v.state != nil {
			return v.typ, ir.StateAtom{State: v.state.fqn, Path: v.fields, Span: span}, key
		}
		return v.typ, ir.NameAtom{Path: segmentsOf(at.Name), Span: span}, key
	case *ast.Literal:
		return literalType(at.Kind), ir.LiteralAtom{Literal: a.lowerLiteral(at), Span: a.irSpan(at.Span)}, ""
	case *ast.NoneAtom:
		return noneType(), ir.NoneAtom{Span: a.irSpan(at.Span)}, ""
	case *ast.RecordLit:
		t, atom := a.evalRecord(w, at)
		return t, atom, ""
	}
	return unknownType(), nil, ""
}

// evalRecord はレコードリテラル。`{ item }` は `{ item: item }` に展開する（docs/ir.md「正規化」）。
// 型は当てはめる先が決まるまで未定（etRecord）。
func (a *analyzer) evalRecord(w *wiring, r *ast.RecordLit) (exprType, ir.Atom) {
	rv := &recordVal{span: r.Span}
	atom := ir.RecordAtom{Span: a.irSpan(r.Span)}
	for _, f := range r.Fields {
		if f == nil {
			continue
		}
		value := f.Value
		if value == nil {
			value = &ast.NameAtom{Span: f.Name.Span, Name: ast.QualifiedName{Span: f.Name.Span, Segments: []ast.Ident{f.Name}}}
		}
		t, va := a.evalAtom(w, value)
		rv.fields = append(rv.fields, recordFieldVal{name: f.Name.Name, span: f.Name.Span, valueSpan: value.NodeSpan(), typ: t})
		atom.Fields = append(atom.Fields, ir.RecordField{Name: f.Name.Name, Value: va})
	}
	return exprType{kind: etRecord, rec: rv}, atom
}

func segmentsOf(qn ast.QualifiedName) []string {
	out := make([]string, len(qn.Segments))
	for i, s := range qn.Segments {
		out[i] = s.Name
	}
	return out
}

// matchClass は match が使う分類: classify の宣言か、標準の分類（docs/check.md「標準の分類」）。
type matchClass struct {
	// name は IR の Match.classify: classify の FQN か、標準の分類なら "list" / "nullable"。
	name string
	// classify は classify の宣言。標準の分類なら nil。
	classify *ir.Classify
	// cases は分類のケースのすべて（宣言順。標準の分類は Empty、HasItems）。
	cases []string
}

// label は診断に出す分類の呼び名。後ろに「のケース」と続ける形。
func (c *matchClass) label() string {
	if c.classify == nil {
		return "標準の分類 " + c.name + " "
	}
	return "classify " + c.name + " "
}

func classOf(c *ir.Classify) *matchClass {
	mc := &matchClass{name: c.Name, classify: c}
	for _, cs := range c.Cases {
		mc.cases = append(mc.cases, cs.Name)
	}
	return mc
}

// standardClass は標準の分類（list / nullable）。ケースは Empty / HasItems。
func standardClass(name string) *matchClass {
	return &matchClass{name: name, cases: ir.BuiltinCases()}
}

// evalMatch は match。形は三つ（docs/grammar.md「配線」の表。合わなければ MATCH-7）: 値を対象にした、値の分類か
// op の分類での match。値を対象にした、標準の分類での match（by list / by nullable）。状態の分類での match
// （対象を書かない）。MATCH-2 と MATCH-9 はどの形にも、MATCH-4 / MATCH-6 は値を対象にした match に掛かる。
// アームの中の対象の型は docs/types.md「アームの中の対象の型」、アームの型の集め方は同「match の型」。
// ret は match が `return` の式なら nil でない。
func (a *analyzer) evalMatch(w *wiring, m *ast.Match, ret *returnContext) (exprType, ir.Match) {
	out := ir.Match{Span: a.irSpan(m.Span)}
	var class *matchClass
	var value *matchValue
	if m.Builtin {
		class = standardClass(m.Classifier.String())
		out.Classify = class.name
		out.Subject, value = a.standardSubject(w, m)
	} else {
		c := a.resolveClassifier(w, m)
		if c != nil {
			class = classOf(c)
			out.Classify = c.Name
		}
		out.Subject, value = a.matchSubject(w, m, c)
	}
	var rest []string
	if class != nil {
		rest = a.checkArmCases(class, m)
	}
	var arms []armResult
	for _, arm := range m.Arms {
		if arm == nil {
			continue
		}
		t, ia := a.evalArm(w, arm, class, value, rest)
		arms = append(arms, armResult{typ: t, span: arm.Span})
		out.Arms = append(out.Arms, ia)
	}
	return a.matchType(arms, m, ret), out
}

// matchValue は値を対象にした match の対象。
type matchValue struct {
	// key は対象が名前の Atom ならその同一性（narrowKey）。リテラル、レコードリテラル、none なら空で、
	// 絞り込みは何にも掛からない。
	key string
	typ exprType
}

// matchSubject は `match [x] by C` の対象（C は classify。解決できていなければ nil）。MATCH-7 の
// 「値の分類・op の分類なのに対象がない」「状態の分類なのに対象がある」と、値を対象にした match の MATCH-6。
func (a *analyzer) matchSubject(w *wiring, m *ast.Match, c *ir.Classify) (ir.MatchSubject, *matchValue) {
	if c != nil && c.IsSubjectless() {
		a.errorf("MATCH-7", m.Classifier.Span, "classify %s は、対象のない分類（of never）なので、match の by には書けない（当たる値がない）", c.Name)
		if m.Subject != nil {
			a.evalAtom(w, m.Subject)
		}
		return nil, nil
	}
	if c != nil {
		if _, isStates := c.SubjectStates(); isStates {
			if m.Subject != nil {
				if n, ok := m.Subject.(*ast.NameAtom); ok {
					a.peek(n.Name, w.vsc) // 束縛を書いていれば使ったことにする（WIRE-8 を重ねない）
				}
				a.errorf("MATCH-7", m.Subject.NodeSpan(), "classify %s は状態の分類なので、match に対象は書けない（対象は分類の宣言が持つ。match by %s と書く）", c.Name, m.Classifier.String())
			}
			return ir.ClassifyOwnedSubject{}, nil
		}
	}
	if m.Subject == nil {
		if c != nil && c.SubjectType() != nil {
			a.errorf("MATCH-7", m.Classifier.Span, "classify %s は%sなので、match に対象が要る（match 値 by %s と書く）", c.Name, classifyKindName(c), m.Classifier.String())
		}
		return nil, nil
	}
	typ, atom, key := a.evalAtomKey(w, m.Subject)
	value := &matchValue{typ: typ, key: key}
	if c != nil {
		a.checkMATCH6(typ, c, m)
	}
	if atom == nil {
		return nil, value
	}
	return ir.ValueSubject{Atom: atom}, value
}

// standardSubject は `match x by list` / `by nullable` の対象: 型が list<T>（by list）か nullable<T>（by nullable）の値
// （docs/check.md「標準の分類」）。状態に限らない。MATCH-7: 対象がない、対象の型が array / optional でない。
func (a *analyzer) standardSubject(w *wiring, m *ast.Match) (ir.MatchSubject, *matchValue) {
	name := m.Classifier.String()
	if m.Subject == nil {
		a.errorf("MATCH-7", m.Classifier.Span, "match by %s には対象の値が要る（match 値 by %s と書く）", name, name)
		return nil, nil
	}
	typ, atom, key := a.evalAtomKey(w, m.Subject)
	value := &matchValue{typ: typ, key: key}
	if typ.kind != etUnknown {
		ok := false
		if typ.kind == etValue {
			switch uncase(typ.typ).(type) {
			case ir.ArrayType:
				ok = name == ir.ClassifyList
			case ir.OptionalType:
				ok = name == ir.ClassifyNullable
			}
		}
		if !ok {
			want := "list<T>（T[]）"
			if name == ir.ClassifyNullable {
				want = "nullable<T>（T?）"
			}
			a.errorf("MATCH-7", m.Subject.NodeSpan(), "match by %s の対象の型は %s でなければならないが、%s", name, want, exprName(typ))
			value.typ = unknownType()
		}
	}
	if atom == nil {
		return nil, value
	}
	return ir.ValueSubject{Atom: atom}, value
}

// resolveClassifier は `match x by C` / `match by C` の C: classify（値の分類、op の分類、状態の分類）。
// どの名前空間のものでもよく、別の名前空間のものは修飾して書く。状態の分類が別の名前空間のものなら RES-5
// （読むのは状態で、状態を読めるのは持ち主の名前空間だけ。docs/check.md「置き場」）。
func (a *analyzer) resolveClassifier(w *wiring, m *ast.Match) *ir.Classify {
	r, ok := a.resolve(m.Classifier, &scope{ns: w.sc.ns}, acClassify)
	if !ok {
		return nil
	}
	c, _ := a.lower(r.sym).(*ir.Classify)
	if c == nil {
		return nil
	}
	if _, isStates := c.SubjectStates(); isStates && r.sym.ns != w.sc.ns {
		a.errorf("RES-5", m.Classifier.Span, "classify %s は別の名前空間の状態の分類で、match by では読めない（状態を読めるのは持ち主の名前空間 %s の配線だけ。その名前空間の op を通して読む）", c.Name, nsName(r.sym.ns))
		return nil
	}
	return c
}

// checkMATCH6 は MATCH-6（docs/check.md「classify / match」）: 値の分類か op の分類での match で、対象の型が分類の対象の型に渡せない。
func (a *analyzer) checkMATCH6(subject exprType, c *ir.Classify, m *ast.Match) {
	want := c.SubjectType()
	if want == nil || subject.kind == etUnknown {
		return
	}
	if a.assign(subject, want) != assignOK {
		a.errorf("MATCH-6", m.Subject.NodeSpan(), "match の対象の型 %s は classify %s の対象の型 %s に渡せない", exprName(subject), c.Name, typeName(want))
	}
}

// checkArmCases は、match のアームが受けるケースを確かめる（docs/check.md「アームが受けるケース」）。戻り値は、else のアームが
// 受けるケース: 分類のケースのうち、どのアームも名前で並べなかったもの（宣言順）。else のアームがなければ nil。
//
// MATCH-2: 名前で並べたケースが分類のケース（標準の分類なら Empty / HasItems）でない、または二度並んでいる（Data.extra）。
// else のアームがなく、どのアームも並べていないケースがある（Data.missing）。
// MATCH-9: else のアームが受けるケースが一つもない。
func (a *analyzer) checkArmCases(c *matchClass, m *ast.Match) []string {
	seen := map[string]bool{}
	var extra []string
	var elseArm *ast.MatchArm
	for _, arm := range m.Arms {
		if arm == nil {
			continue
		}
		if arm.Else {
			if elseArm == nil {
				elseArm = arm
			}
			continue
		}
		for _, cs := range arm.Cases {
			switch {
			case !contains(c.cases, cs.Name):
				extra = append(extra, cs.Name)
			case seen[cs.Name]:
				extra = append(extra, cs.Name)
			default:
				seen[cs.Name] = true
			}
		}
	}
	var unnamed []string
	for _, name := range c.cases {
		if !seen[name] {
			unnamed = append(unnamed, name)
		}
	}
	// else のアームがあれば、並べなかったケースは、そのアームが受ける。足りないケースはない
	var missing, rest []string
	if elseArm != nil {
		rest = unnamed
	} else {
		missing = unnamed
	}
	var problems []string
	if len(missing) > 0 {
		problems = append(problems, "足りないケース: "+join(missing))
	}
	if len(extra) > 0 {
		problems = append(problems, "知らないか重複したケース: "+join(extra))
	}
	if len(problems) > 0 {
		d := a.errorf("MATCH-2", m.Classifier.Span, "match のアームが %sのケース（%s）と合わない（%s）", c.label(), strings.Join(c.cases, " / "), strings.Join(problems, "。"))
		d.Data = map[string]any{"missing": orEmpty(missing), "extra": orEmpty(extra)}
	}
	if elseArm != nil && len(rest) == 0 {
		a.errorf("MATCH-9", elseArm.Span, "else のアームが受けるケースがない（ほかのアームが、%sのすべてのケース（%s）を並べている。else のアームは要らない）", c.label(), strings.Join(c.cases, " / "))
	}
	return rest
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// armResult は評価したアーム。
type armResult struct {
	typ  exprType
	span source.Span
}

// evalArm はアーム。本体は呼び出し一つ、値一つ、throw 一つのどれか（docs/grammar.md「配線」の ArmBody）。
// 本体は、アームの中の対象の型（narrowArm）のもとで評価する。rest は、else のアームが受けるケース（checkArmCases）。
func (a *analyzer) evalArm(w *wiring, arm *ast.MatchArm, class *matchClass, value *matchValue, rest []string) (exprType, ir.MatchArm) {
	out := ir.MatchArm{Span: a.irSpan(arm.Span), Else: arm.Else}
	var cases []string
	label := ""
	if arm.Else {
		// else のアームの cases は、それが受けるケース（宣言順。docs/ir.md「配線」）
		cases = append(cases, rest...)
		label = "else（" + strings.Join(cases, ", ") + "）"
	} else {
		cases = make([]string, 0, len(arm.Cases))
		for _, cs := range arm.Cases {
			cases = append(cases, cs.Name)
			if class != nil && class.classify != nil && contains(class.cases, cs.Name) {
				a.addRef(cs.Span, Target{Kind: TargetCase, Decl: class.name, Name: cs.Name, Span: a.caseSpan(class.classify, cs.Name)})
			}
		}
		label = strings.Join(cases, ", ")
	}
	out.Cases = cases
	var ctx *armContext
	if value != nil {
		ctx = &armContext{subject: value.key, cases: label, known: cases, outer: value.typ}
		if class != nil && class.classify != nil {
			ctx.classify = class.classify.Name
		}
	}
	narrow, narrowed := a.narrowArm(arm.Else, cases, class, value)
	out.Narrowed = narrowed
	w.scope.narrow = narrow
	defer func() { w.scope.narrow = nil }()

	switch body := arm.Body.(type) {
	case *ast.Call:
		t, call, _ := a.evalCall(w, body, ctx)
		out.Body = ir.CallExpr{Call: call}
		return t, out
	case *ast.Throw:
		out.Body = a.lowerThrow(w, body, ctx)
		return neverType(), out
	case *ast.NoneAtom:
		// アームの本体の none は body の none（atom にはしない。docs/ir.md「正規化」）
		out.Body = ir.NoneBody{}
		return noneType(), out
	case ast.Atom:
		t, atom := a.evalAtom(w, body)
		if atom != nil {
			out.Body = ir.AtomExpr{Atom: atom}
		}
		return t, out
	}
	return unknownType(), out
}

// lowerThrow はアームの本体の `throw E`、`throw E(値, …)`（docs/check.md「throw」）。E は例外一つ（どの名前空間の
// ものでも。例外のユニオンとデコレータの引数は RES-4）。引数は、例外の宣言の引数に位置で対応付け、呼び出しと同じ規則で
// 確かめる（docs/types.md「呼び出しの引数」）。
func (a *analyzer) lowerThrow(w *wiring, t *ast.Throw, arm *armContext) ir.ArmBody {
	body := ir.ThrowBody{Span: a.irSpan(t.Span)}
	args := a.evalArgs(w, t.Args)
	r, ok := a.resolve(t.Exception, &scope{ns: w.sc.ns}, acException)
	if !ok {
		return body
	}
	body.Exception = r.sym.fqn
	if e, ok := a.lower(r.sym).(*ir.Exception); ok {
		body.Args = a.bindArgs(callTarget{what: "例外 " + e.Name, params: e.Params}, args, arm)
	}
	return body
}

// narrowArm は docs/types.md「アームの中の対象の型」: アーム（受けるケースの並び cases。else のアームなら isElse）の中での
// 対象の型。外と型が違う対象だけを、スコープに掛ける表（narrowKey → 型）と IR の Narrowing の列で返す。
// 値を対象にした match（value が nil でない）なら対象はその Atom、状態の分類での match なら分類の対象の状態のそれぞれ。
func (a *analyzer) narrowArm(isElse bool, cases []string, class *matchClass, value *matchValue) (map[string]exprType, []ir.Narrowing) {
	if class == nil {
		return nil, nil
	}
	if isElse && len(cases) != 1 {
		// 1. else のアームで、受けるケースが二つ以上なら、外のまま。ちょうど一つなら、そのケースを名前で書いたアームと同じ
		// （受けるケースがないのは MATCH-9）
		return nil, nil
	}
	if value != nil {
		if value.typ.kind != etValue || value.key == "" {
			return nil, nil // 対象が名前でない Atom なら、絞り込みは何にも掛からない
		}
		x := value.typ.typ
		n := a.narrowValue(x, cases, class)
		if n == nil || ir.TypeEqual(n, x) {
			return nil, nil
		}
		return map[string]exprType{value.key: valueType(n)}, []ir.Narrowing{{Type: n}}
	}
	if class.classify == nil {
		return nil, nil
	}
	states, isStates := class.classify.SubjectStates()
	if !isStates {
		return nil, nil
	}
	var narrow map[string]exprType
	var narrowed []ir.Narrowing
	for _, state := range states {
		sym := a.syms[state]
		if sym == nil {
			continue
		}
		st, ok := a.lower(sym).(*ir.State)
		if !ok || st.Type == nil {
			continue
		}
		// 3. すべてのケースがその状態の as を持ち、書いた型がすべて等しい。5. そうでなければ外のまま
		n := a.writtenType(class.classify, cases, state)
		if n == nil || a.assignType(st.Type, n) {
			continue
		}
		if narrow == nil {
			narrow = map[string]exprType{}
		}
		narrow[narrowKey("s:"+state, nil)] = valueType(n)
		narrowed = append(narrowed, ir.Narrowing{State: state, Type: n})
	}
	return narrow, narrowed
}

// narrowValue は値を対象にした match の、アームの中での対象の型（外での型は x）。規則 2〜4。
func (a *analyzer) narrowValue(x ir.Type, cases []string, class *matchClass) ir.Type {
	if class.classify == nil {
		// 2. 標準の分類: by nullable の HasItems だけのアームで、optional(T) は T。それ以外は外のまま
		if class.name == ir.ClassifyNullable && len(cases) == 1 && cases[0] == ir.CaseHasItems {
			if o, ok := uncase(x).(ir.OptionalType); ok {
				return o.Of
			}
		}
		return x
	}
	if _, isStates := class.classify.SubjectStates(); isStates {
		return x
	}
	// 3. すべてのケースが as を持ち、書いた型がすべて等しい → その型。ただし x がその型に渡せるなら x のまま
	if n := a.writtenType(class.classify, cases, ""); n != nil {
		if a.assignType(x, n) {
			return x
		}
		return n
	}
	// 4. 型を書かないケースの絞り込み: case(C, Ks, X)
	return ir.Case(class.classify.Name, cases, uncase(x))
}

// writtenType は、cases のすべてのケースが対象（状態の分類なら state、それ以外は ""）の as を持ち、書いた型が
// すべて等しければその型。そうでなければ nil。
func (a *analyzer) writtenType(c *ir.Classify, cases []string, state string) ir.Type {
	var t ir.Type
	for _, name := range cases {
		cs := c.Case(name)
		if cs == nil {
			return nil
		}
		var as *ir.CaseTyping
		for i := range cs.As {
			if cs.As[i].State == state {
				as = &cs.As[i]
			}
		}
		if as == nil || as.Type == nil {
			return nil
		}
		if t != nil && !ir.TypeEqual(t, as.Type) {
			return nil
		}
		t = as.Type
	}
	return t
}

// caseSpan は classify のケース名の定義の位置。
func (a *analyzer) caseSpan(c *ir.Classify, name string) source.Span {
	if s := a.syms[c.Name]; s != nil {
		if span, ok := classifyCaseSpan(s, name); ok {
			return span
		}
	}
	return source.Span{}
}

// matchType は docs/types.md「match の型」。「返らない」アームを除き、残りに値のある本体がなければ「値がない」。
// 値のある本体と「値がない」が混ざれば TYPE-8。そろえる型 T は、型の決まった本体があれば、ほかのすべてが渡せる型
// （なければ TYPE-8）。リテラルだけなら、その種類の核の型。レコードリテラルと none だけなら、return の式に限って
// op の returns から決める（そうでなければ TYPE-7）。リテラルとレコードリテラルの本体は T に渡せること。
// none の本体があれば optional(T)。
func (a *analyzer) matchType(arms []armResult, m *ast.Match, ret *returnContext) exprType {
	var typed, literals, records []armResult
	hasNone, hasNoValue, unknown, returning := false, false, false, false
	for _, arm := range arms {
		switch arm.typ.kind {
		case etNever:
			continue
		case etUnknown:
			unknown = true
		case etNone:
			hasNone = true
		case etNoValue:
			hasNoValue = true
		case etValue:
			typed = append(typed, arm)
		case etLiteral:
			literals = append(literals, arm)
		case etRecord:
			records = append(records, arm)
		}
		returning = true
	}
	if !returning {
		return neverType() // 2. すべてのアームが返らない
	}
	if len(typed)+len(literals)+len(records) == 0 {
		if unknown {
			return unknownType()
		}
		return noValue() // 3. 「値がない」と none だけ
	}
	if hasNoValue {
		a.errorf("TYPE-8", m.Span, "match に値のある本体と値のない呼び出しが混ざっている")
		return unknownType()
	}
	// 5. そろえる型
	var t ir.Type
	switch {
	case len(typed) > 0:
		for _, cand := range typed {
			all := true
			for _, other := range typed {
				if !a.assignType(other.typ.typ, cand.typ.typ) {
					all = false
					break
				}
			}
			if all {
				t = cand.typ.typ
				break
			}
		}
		if t == nil {
			a.errorf("TYPE-8", m.Span, "match のアームの型がそろわない（ほかのすべてのアームの型が渡せる型がない）: %s", armTypeNames(typed))
			return unknownType()
		}
	case len(literals) > 0:
		t = literals[0].typ.typ
		for _, l := range literals[1:] {
			if !ir.TypeEqual(l.typ.typ, t) {
				a.errorf("TYPE-8", m.Span, "match のアームのリテラルの種類がそろわない: %s", armTypeNames(literals))
				return unknownType()
			}
		}
	default:
		if ret == nil || ret.returns == nil || ir.IsNever(ret.returns) {
			a.errorf("TYPE-7", m.Span, "match のアームのレコードリテラルの当てはめる先が決まらない（return の式に書くか、型の決まったアームを書く）")
			return unknownType()
		}
		t = stripOptional(ret.returns)
	}
	// 6. リテラルとレコードリテラルの本体は T に渡せること
	for _, arm := range append(append([]armResult{}, literals...), records...) {
		switch a.assign(arm.typ, t) {
		case assignOK:
		case failScalar:
			a.errorf("TYPE-1", arm.span, "アームの値 %s を scalar %s の位置に渡せない（検証を通していない）", exprName(arm.typ), typeName(t))
		default:
			a.errorf("TYPE-8", arm.span, "アームの値 %s は、match の型 %s に渡せない", exprName(arm.typ), typeName(t))
		}
	}
	// 7. none の本体があれば optional(T)
	if hasNone {
		return valueType(ir.Optional(t))
	}
	return valueType(t)
}

// armTypeNames はアームの型の表示（重複を除く。診断用）。
func armTypeNames(arms []armResult) string {
	var names []string
	for _, arm := range arms {
		if n := exprName(arm.typ); !contains(names, n) {
			names = append(names, n)
		}
	}
	return strings.Join(names, " と ")
}
