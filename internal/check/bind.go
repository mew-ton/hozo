package check

import (
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// position は bind で埋める位置（docs/check.md「bind」の Pos(b, O) の一つ）。
type position struct {
	path       []string // ["in", "token"]。先頭は引数名
	typ        ir.Type  // その位置の型（optional を含む）
	hasDefault bool     // フィールドで default があるか
	span       ir.Span  // 引数か、最後のフィールドの位置
}

type bindOpKey struct {
	bind int
	op   string
}

// bindTable は Pos(b, O) をメモする。
type bindTable struct {
	ix    *index
	cache map[bindOpKey][]position
}

func newBindTable(ix *index) *bindTable {
	return &bindTable{ix: ix, cache: map[bindOpKey][]position{}}
}

// positions は Pos(b, O)（docs/check.md「bind」1〜4）。bi は Program.binds の添字。
// 対象の名前空間に合わない、状態が list（BIND-6）、状態が引けない、のどれかなら空。
func (t *bindTable) positions(bi int, o *ir.Op) []position {
	key := bindOpKey{bi, o.Name}
	if pos, ok := t.cache[key]; ok {
		return pos
	}
	pos := t.compute(bi, o)
	t.cache[key] = pos
	return pos
}

func (t *bindTable) compute(bi int, o *ir.Op) []position {
	ix := t.ix
	b := ix.p.Binds[bi]
	ns := ix.namespaceOf(o.Name)
	if b.Target.Deep {
		if !isDescendant(ns, b.Target.Namespace) {
			return nil
		}
	} else if ns != b.Target.Namespace {
		return nil
	}
	st := ix.state(b.State)
	if st == nil || st.Shape == ir.StateList || st.Element == nil {
		return nil
	}
	tau := st.Element
	if opt, ok := tau.(ir.OptionalType); ok {
		tau = opt.Of
	}
	var out []position
	visiting := map[string]bool{}
	for _, p := range o.Params {
		t.collect([]string{p.Name}, p.Type, false, p.Span, tau, visiting, &out)
	}
	return out
}

// collect は型 typ の位置 path から、τ または optional(τ) の位置を集める。model（と model の optional）は
// フィールドを再帰的に辿り、array と record の中は辿らない。辿っている途中で同じ model に戻ったら止める。
func (t *bindTable) collect(path []string, typ ir.Type, hasDefault bool, span ir.Span, tau ir.Type, visiting map[string]bool, out *[]position) {
	if typ == nil {
		return
	}
	base := typ
	if opt, ok := base.(ir.OptionalType); ok {
		base = opt.Of
	}
	if ir.TypeEqual(base, tau) {
		*out = append(*out, position{path: append([]string(nil), path...), typ: typ, hasDefault: hasDefault, span: span})
		return
	}
	m := t.ix.modelOf(base)
	if m == nil || visiting[m.Name] {
		return
	}
	visiting[m.Name] = true
	for _, f := range m.Fields {
		t.collect(append(append([]string(nil), path...), f.Name), f.Type, f.Default != nil, f.Span, tau, visiting, out)
	}
	delete(visiting, m.Name)
}

// boundOf は名前空間 ns の配線の呼び出し call の Call.bound（docs/check.md「bind」の末尾。BIND-3）。
// ns のすべての Bind の Pos(b, callee) から、call が明示している位置を除く。同じ位置は最初の Bind のものだけ。
func (t *bindTable) boundOf(ns string, call *ir.Call) []ir.BoundPosition {
	callee := t.ix.op(call.Op)
	if callee == nil {
		return nil
	}
	var out []ir.BoundPosition
	for bi, b := range t.ix.p.Binds {
		if b.Namespace != ns {
			continue
		}
		for _, pos := range t.positions(bi, callee) {
			if explicit(call, pos.path) || hasPath(out, pos.path) {
				continue
			}
			out = append(out, ir.BoundPosition{Path: append([]string(nil), pos.path...), State: b.State})
		}
	}
	return out
}

// explicit は call が path の位置を明示して渡しているか。パスの引数が name の Atom（中身は分からないので明示と
// みなす）、リテラル、none で渡されているか、レコードリテラルの連なりがパスのフィールドを持つとき。
func explicit(call *ir.Call, path []string) bool {
	for _, a := range call.Args {
		if a.Param == path[0] {
			return explicitAtom(a.Value, path[1:])
		}
	}
	return false
}

func explicitAtom(atom ir.Atom, rest []string) bool {
	if len(rest) == 0 {
		return true
	}
	rec, ok := atom.(ir.RecordAtom)
	if !ok {
		return true // name / literal / none: 中身は分からないので明示とみなす
	}
	for _, f := range rec.Fields {
		if f.Name == rest[0] {
			return explicitAtom(f.Value, rest[1:])
		}
	}
	return false
}

// checkBinds は BIND-1 / BIND-2 / BIND-4 / BIND-5 / BIND-6（docs/check.md「bind」）。
func checkBinds(ix *index, c *computer, r *reporter) {
	for bi := range ix.p.Binds {
		checkBind6(ix, bi, r)
	}
	for bi := range ix.p.Binds {
		for _, o := range ix.ops {
			checkBind1And2(ix, c.binds, bi, o, r)
		}
	}
	for _, o := range ix.ops {
		checkBind5(ix, c.binds, o, r)
	}
	for i, o := range ix.ops {
		if o.Body == nil {
			continue
		}
		for _, row := range rowsOf(o.Body) {
			for _, cr := range row.calls {
				call := cr.call
				call.Bound = c.binds.boundOf(ix.opsNS[i], &call)
				checkBind4(ix, &call, r)
			}
		}
	}
}

// checkBind6 は BIND-6: list の状態を bind に書いた。位置は Bind。
func checkBind6(ix *index, bi int, r *reporter) {
	b := ix.p.Binds[bi]
	if st := ix.state(b.State); st != nil && st.Shape == ir.StateList {
		r.errorAt("BIND-6", b.Span, sprintf("bind に list の状態 %s は書けない（埋められるのは single の状態だけ）", b.State))
	}
}

// checkBind1And2 は BIND-1（位置が二つ以上。Related に各位置）と BIND-2（状態が省略可能で、位置が省略できず
// default もない）。計算「bind」5。位置は Bind。
func checkBind1And2(ix *index, t *bindTable, bi int, o *ir.Op, r *reporter) {
	b := ix.p.Binds[bi]
	pos := t.positions(bi, o)
	switch {
	case len(pos) >= 2:
		var rel []related
		var paths []string
		for _, p := range pos {
			rel = append(rel, related{span: p.span, msg: sprintf("位置: %s", joinPath(p.path))})
			paths = append(paths, joinPath(p.path))
		}
		r.report(diag.Error, "BIND-1", b.Span,
			sprintf("bind %s は %s の二つ以上の位置に当たる（%s）。意味の違う値には別の型を付ける", b.State, o.Name, joinNames(paths)),
			rel, nil)
	case len(pos) == 1:
		st := ix.state(b.State)
		if st == nil || !isOptional(st.Element) {
			return
		}
		p := pos[0]
		if !isOptional(p.typ) && !p.hasDefault {
			r.report(diag.Error, "BIND-2", b.Span,
				sprintf("bind %s は省略可能だが、%s の位置 %s は省略できない", b.State, o.Name, joinPath(p.path)),
				[]related{{span: p.span, msg: sprintf("位置: %s", joinPath(p.path))}}, nil)
		}
	}
}

// checkBind5 は BIND-5: 同じ名前空間の二つの Bind が同じ op の同じ位置に当たる。位置は後の Bind。計算「bind」6。
func checkBind5(ix *index, t *bindTable, o *ir.Op, r *reporter) {
	binds := ix.p.Binds
	for j := range binds {
		for i := 0; i < j; i++ {
			if binds[i].Namespace != binds[j].Namespace {
				continue
			}
			var dup []string
			for _, pj := range t.positions(j, o) {
				for _, pi := range t.positions(i, o) {
					if pathEqual(pi.path, pj.path) {
						dup = append(dup, joinPath(pj.path))
					}
				}
			}
			if len(dup) == 0 {
				continue
			}
			r.report(diag.Error, "BIND-5", binds[j].Span,
				sprintf("bind %s と bind %s が %s の同じ位置 %s に当たる", binds[j].State, binds[i].State, o.Name, joinNames(dup)),
				[]related{{span: binds[i].Span, msg: sprintf("もう一方の bind %s", binds[i].State)}}, nil)
		}
	}
}

// checkBind4 は BIND-4（docs/check.md「必須の入力」）。callee の各引数 p について、optional でなく、args にも
// Call.bound の [p] にもなければ足りない。レコードリテラルで渡した引数は「当てはまる」の手続きで必須のフィールドを
// 再帰的に確かめる。位置は呼び出し。Data.missing に足りないパスの列。
func checkBind4(ix *index, call *ir.Call, r *reporter) {
	callee := ix.op(call.Op)
	if callee == nil {
		return
	}
	var missing [][]string
	for _, p := range callee.Params {
		path := []string{p.Name}
		var given ir.Atom
		found := false
		for _, a := range call.Args {
			if a.Param == p.Name {
				given, found = a.Value, true
				break
			}
		}
		if !found {
			if !isOptional(p.Type) && !hasPath(call.Bound, path) {
				missing = append(missing, path)
			}
			continue
		}
		if rec, ok := given.(ir.RecordAtom); ok {
			if m := ix.modelOf(p.Type); m != nil {
				missing = checkRecordFits(ix, path, rec, m, call.Bound, map[string]bool{}, missing)
			}
		}
	}
	if len(missing) == 0 {
		return
	}
	var names []string
	for _, m := range missing {
		names = append(names, joinPath(m))
	}
	r.report(diag.Error, "BIND-4", call.Span,
		sprintf("%s の呼び出しで必須の入力 %s が明示でも bind でも埋まっていない", call.Op, joinNames(names)),
		nil, map[string]any{"missing": missing})
}

// checkRecordFits は docs/types.md「当てはまる」の必須のフィールドの確認。T の必須のフィールド（optional でなく
// default もない）がリテラルにあるか、Call.bound のパスにある。リテラルのフィールドがレコードリテラルで、型が model なら再帰する。
func checkRecordFits(ix *index, path []string, rec ir.RecordAtom, m *ir.Model, bound []ir.BoundPosition, visiting map[string]bool, missing [][]string) [][]string {
	if visiting[m.Name] {
		return missing
	}
	visiting[m.Name] = true
	defer delete(visiting, m.Name)
	for _, f := range m.Fields {
		fpath := append(append([]string(nil), path...), f.Name)
		var given ir.Atom
		found := false
		for _, rf := range rec.Fields {
			if rf.Name == f.Name {
				given, found = rf.Value, true
				break
			}
		}
		if !found {
			required := !isOptional(f.Type) && f.Default == nil
			if required && !hasPath(bound, fpath) {
				missing = append(missing, fpath)
			}
			continue
		}
		if sub, ok := given.(ir.RecordAtom); ok {
			if fm := ix.modelOf(f.Type); fm != nil {
				missing = checkRecordFits(ix, fpath, sub, fm, bound, visiting, missing)
			}
		}
	}
	return missing
}
