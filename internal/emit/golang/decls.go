package golang

import "github.com/mew-ton/hozo/ir"

// stateUnit は状態の単位一つ（docs/emit-go.md「state」）: 基底の状態か、状態の分類。
type stateUnit struct {
	fqn string
	// state は基底の状態。状態の分類なら nil。
	state *ir.State
	// classify は状態の分類。基底の状態なら nil。
	classify *ir.Classify
	// commands は命令: 選ばれた葉の op のうち、効果の宛先がこの状態のもの（op の宣言の順）。
	commands []*ir.Op
}

// collectDecls は、決まった出力の対象（e.sel）から、パッケージに出す宣言を集める:
// op・状態の単位・分類・効果の定数（viewer の宣言の順）と、それらが参照する型（docs/emit-go.md「出力の対象」の 6）。
//
// 型は二段で決める。
//
//  1. reach: 出力の対象が参照する型を、名前空間を問わず参照を辿り切って集める。viewer の型はこの全部を出す。
//     別の名前空間の選択にはよらないので、[Exports] がそのまま答えになる。
//  2. copied: viewer のパッケージが実際に型として使う別の名前空間の型のうち、import できないもの
//     （docs/emit-go.md「配置」）。import できるのは、Go で同じプロジェクトの名前空間の、その出力が持つ型
//     （[Options.Exported]）だけ。複製した型が参照する型も同じ規則で辿る。
func (e *emitter) collectDecls() {
	// op、状態の単位、分類、包む関数。
	for _, d := range e.ns.Decls {
		switch x := d.(type) {
		case *ir.State:
			if e.sel.states[x.Name] {
				u := &stateUnit{fqn: x.Name, state: x}
				e.units = append(e.units, u)
				e.unitOf[x.Name] = u
			}
		case *ir.Classify:
			switch {
			case e.sel.states[x.Name]:
				u := &stateUnit{fqn: x.Name, classify: x}
				e.units = append(e.units, u)
				e.unitOf[x.Name] = u
				e.classifies = append(e.classifies, x)
			case e.sel.classifies[x.Name]:
				e.classifies = append(e.classifies, x)
			}
		case *ir.Op:
			if e.sel.ops[x.Name] {
				e.ops = append(e.ops, x)
			}
		}
	}
	for _, op := range e.ops {
		if !e.isSlot(op) {
			continue // 配線の本体を出す op
		}
		if u := e.unitOf[e.effectTargetOf(op)]; u != nil {
			u.commands = append(u.commands, op)
			e.commandOf[op.Name] = u
		} else {
			e.leaves = append(e.leaves, op)
		}
	}
	e.collectWrappers()

	// 1. reach
	for _, t := range e.seedTypes() {
		e.reachType(t)
	}
	for _, fqn := range e.seedRefs() {
		e.reachRef(fqn)
	}

	// viewer の宣言（宣言の順）。効果の定数は、選ばれた op の効果のうち viewer のもの。
	usedEffects := map[string]bool{}
	for _, op := range e.ops {
		for _, fqn := range op.Effects {
			usedEffects[fqn] = true
		}
		for _, fqn := range e.viewedSig(op.Name).Effects {
			usedEffects[fqn] = true
		}
	}
	for _, d := range e.ns.Decls {
		switch x := d.(type) {
		case *ir.Model:
			if e.reach[x.Name] {
				e.models = append(e.models, x)
			}
		case *ir.Scalar:
			if e.reach[x.Name] {
				e.scalars = append(e.scalars, x)
			}
		case *ir.Union:
			if e.reach[x.Name] {
				e.unions = append(e.unions, x)
			}
		case *ir.Exception:
			if e.reach[x.Name] {
				e.exceptions = append(e.exceptions, x)
			}
		case *ir.Effect:
			if usedEffects[x.Name] {
				e.effects = append(e.effects, x)
			}
		}
	}

	// 2. copied: パッケージが使う型から辿る。
	for _, t := range e.seedTypes() {
		e.visitType(t)
	}
	for _, fqn := range e.seedRefs() {
		e.visitRef(fqn)
	}
	for _, m := range e.models {
		e.visitDecl(m)
	}
	for _, s := range e.scalars {
		e.visitDecl(s)
	}
	for _, u := range e.unions {
		e.visitDecl(u)
	}
	for _, x := range e.exceptions {
		e.visitDecl(x)
	}
	// 複製は Program の順で後ろに足す。
	for i := range e.prog.Namespaces {
		n := &e.prog.Namespaces[i]
		if n.Name == e.viewer {
			continue
		}
		for _, d := range n.Decls {
			if d == nil || !e.copied[d.Common().Name] {
				continue
			}
			switch x := d.(type) {
			case *ir.Model:
				e.models = append(e.models, x)
			case *ir.Scalar:
				e.scalars = append(e.scalars, x)
			case *ir.Union:
				e.unions = append(e.unions, x)
			case *ir.Exception:
				e.exceptions = append(e.exceptions, x)
			}
		}
	}
}

// isSlot は、op が実装を差し込む口だけを出すものか: 葉の op と、select の stops / wraps で本体を辿らなかった配線。
func (e *emitter) isSlot(op *ir.Op) bool { return op.Body == nil || e.sel.slot[op.Name] }

// seedTypes は、選ばれた op・状態・分類が型として直に参照するもの（docs/emit-go.md「出力の対象」の 6）:
// op の引数と戻り値、配線が呼ぶ別の名前空間の op の（viewer から見た）引数と戻り値、基底の状態の要素の型、
// 値の分類の対象の型。
func (e *emitter) seedTypes() []ir.Type {
	var out []ir.Type
	for _, op := range e.ops {
		for _, p := range op.Params {
			out = append(out, p.Type)
		}
		out = append(out, op.Returns)
	}
	for _, fqn := range e.orderedDeps() {
		for _, p := range e.viewedParams(fqn) {
			out = append(out, p.Type)
		}
		out = append(out, e.op(fqn).Returns)
	}
	for _, u := range e.units {
		if u.state != nil {
			out = append(out, u.state.Element)
		}
	}
	for _, c := range e.classifies {
		out = append(out, c.SubjectType())
	}
	return out
}

// seedRefs は、選ばれた op が例外として参照する宣言: （viewer から見た）Signature.exceptions。
// 葉の実装者が投げ、呼ぶ側（go-cli の cli.go など）が errors.As で見分けるための Go の型が要る
// （docs/emit-go.md「配置」「exception」）。
func (e *emitter) seedRefs() []string {
	var out []string
	for _, op := range e.ops {
		out = append(out, e.viewedSig(op.Name).Exceptions...)
	}
	return out
}

// reachType は型が参照する宣言を reach に足し、その宣言が参照する型も辿る（名前空間を問わない）。
func (e *emitter) reachType(t ir.Type) {
	forEachRef(t, e.reachRef)
}

func (e *emitter) reachRef(fqn string) {
	if e.reach[fqn] {
		return
	}
	d := e.typeDecl(fqn)
	if d == nil {
		return
	}
	e.reach[fqn] = true
	e.eachTypeOf(d, e.reachType)
}

// typeDecl は FQN が Go の型になる宣言（model / scalar / union / exception）ならそれを返す。
func (e *emitter) typeDecl(fqn string) ir.Decl {
	switch d := e.lookup(fqn).(type) {
	case *ir.Model:
		return d
	case *ir.Scalar:
		return d
	case *ir.Union:
		return d
	case *ir.Exception:
		return d
	}
	return nil
}

// eachTypeOf は宣言 d が参照する型（フィールド、元の型、アーム、値、derivedFrom の元の model）を f に渡す。
func (e *emitter) eachTypeOf(d ir.Decl, f func(ir.Type)) {
	switch x := d.(type) {
	case *ir.Model:
		for _, fl := range x.Fields {
			f(fl.Type)
		}
		// 元の model からの変換関数（docs/emit-go.md「model」）が元の model を参照する。
		if x.DerivedFrom != nil {
			f(ir.Ref(x.DerivedFrom.Base))
		}
	case *ir.Scalar:
		f(x.Base)
	case *ir.Union:
		for _, a := range x.Arms {
			f(a.Type)
		}
	case *ir.Exception:
		f(x.Value)
	}
}

// forEachRef は型の中の ref の行き先（FQN）を f に渡す。
func forEachRef(t ir.Type, f func(string)) {
	switch x := t.(type) {
	case nil:
	case ir.RefType:
		f(x.Target)
	case ir.OptionalType:
		forEachRef(x.Of, f)
	case ir.ArrayType:
		forEachRef(x.Of, f)
	case ir.RecordType:
		forEachRef(x.Key, f)
		forEachRef(x.Value, f)
	case ir.CaseType:
		forEachRef(x.Of, f)
	}
}

// visitType は型が参照する宣言のうち複製が要るものを copied に足し、複製した宣言が参照する型も辿る。
func (e *emitter) visitType(t ir.Type) {
	forEachRef(t, e.visitRef)
}

func (e *emitter) visitRef(fqn string) {
	ns, known := e.declNS[fqn]
	if !known || ns == e.viewer || e.importable(fqn) || e.copied[fqn] {
		return
	}
	d := e.typeDecl(fqn)
	if d == nil {
		return
	}
	e.copied[fqn] = true
	e.visitDecl(d)
}

// visitDecl は viewer のパッケージに出す宣言 d（viewer の宣言か複製）が参照する型を辿る。
// 複製した model の derivedFrom の変換関数は出さないので、その元の model は辿らない。
func (e *emitter) visitDecl(d ir.Decl) {
	if m, ok := d.(*ir.Model); ok && e.copied[m.Name] {
		for _, fl := range m.Fields {
			e.visitType(fl.Type)
		}
		return
	}
	e.eachTypeOf(d, e.visitType)
}

// viewedParams は viewer から見た op の引数（docs/ir.md「その名前空間から見た IR」の params）。
// View.Ops になければ Op.Params。
func (e *emitter) viewedParams(opFQN string) []ir.OpParam {
	if sig := e.view.Signature(opFQN); sig != nil {
		return sig.Params
	}
	if op := e.op(opFQN); op != nil {
		return op.Params
	}
	return nil
}

// viewedSig は viewer から見た op の Signature。View.Ops になければ Op.Signature。
func (e *emitter) viewedSig(opFQN string) ir.Signature {
	if sig := e.view.Signature(opFQN); sig != nil {
		return sig.Signature
	}
	if op := e.op(opFQN); op != nil {
		return op.Signature
	}
	return ir.Signature{}
}
