package golang

import (
	"sort"
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// Place は op の置き場（docs/plugin-go.md「op の置き場」の表の 1〜6）: N の op の実装が、どこにあるか。
type Place int

const (
	// Wiring は、単位の対象の配線（本体を持つ op）。実装は、生成した本体（wiring.go）。
	Wiring Place = iota + 1
	// Never は、never を返す op。実装は、生成した本体（ops.go）: `return <例外の型>{}`。
	Never
	// StateCommand は、効果を持つ葉で、効果の宛先の単位が、すべての効果で同じもの。実装は、その状態の単位の命令。
	StateCommand
	// SharedCommand は、効果を持つ葉で、効果の宛先の単位が、二つ以上に分かれるもの。実装は、SharedState。
	SharedCommand
	// StateQuery は、効果を持たず、読む状態が一つの葉。実装は、その状態の単位の問い合わせ。
	StateQuery
	// SharedQuery は、効果を持たず、読む状態が二つ以上の葉。実装は、SharedState。
	SharedQuery
	// Leaf は、それ以外の葉（運ぶ節のないデコレータだけを持つ葉を含む）と、単位の対象でない配線。実装は、Leaves。
	Leaf
)

// 葉の「効果」と「読む状態」は、op が直に持つ効果と、直に読む状態: op が自分で書いたもの（Op.effects、Op.reads）と、付いたデコレータが
// 運ぶもの（Op.carried）を合わせたもの（ir.Op.DirectEffects、ir.Op.DirectReads）。どちらで書かれたかは、置き場を変えない。
// デコレータの名前は見ない: 運ぶものは、hozoc が IR に置いている。

// Shape は、一つの出力 (U, N) の、op と、実装を受け取る口のすべて（docs/plugin-go.md「出力の対象」「実装を受け取る口」「継ぎ目」）。
// 基底も相乗りも、ShapeOf の同じ値から名前と並びを求める。
type Shape struct {
	// Ops は N の op: 単位の対象の op と、単位の対象の配線が呼ぶ、単位の対象でない N の op（本体のない op として出すもの）。宣言順。
	Ops []OpShape
	// Leaves は、インターフェース Leaves のメソッドになる op の FQN（宣言順）。空なら Leaves も New の引数 leaves もない。
	Leaves []string
	// States は状態の単位: 基底の状態と、述語で決まる状態の分類。宣言順。
	States []StateUnit
	// Shared は、インターフェース SharedState のメソッドになる op の FQN: 効果の宛先の単位が分かれた葉（SharedCommand）と、
	// 複数の状態を読む葉（SharedQuery）。種類で分けずに、宣言順。空なら SharedState も New の引数 shared もない。
	Shared []string
	// Classify は、New が述語を受け取る、値の分類と op の分類の FQN: 単位の対象の配線が match に使う、述語で決まるもの
	// （別の名前空間のものを含む）。FQN の辞書順。
	Classify []string
	// Deps は別の名前空間の口: 単位の対象の配線が呼ぶ別の名前空間ごとに一つ。FQN の辞書順。
	Deps []Dep
	// Seams は、インターフェース Seams のメソッド: 継ぎ目を持つ op と、それに付いた、単位の継ぎ目にあるデコレータの組ごとに一つ。
	// op の宣言順、その中はデコレータの書かれた順。空なら Seams も New の引数 seams もない。
	Seams []Seam
	// New は New の引数（名前と Go の型）。この順: leaves、状態の単位、shared、分類の述語、別の名前空間、seams。
	// 構造体 ops のフィールドも、同じ名前、同じ順。
	New []Param
}

// OpShape は N の op 一つの形（docs/plugin-go.md「op の形」「op の置き場」「継ぎ目」）。
type OpShape struct {
	// FQN は op の FQN。Method はメソッドの名前（op 名を先頭大文字にしたもの）。
	FQN, Method string
	// Exported は export が付いているか。入口の Ops に並ぶのは、これが真の op だけ。
	Exported bool
	// Place は置き場。
	Place Place
	// Unit は、Place が状態の単位（StateCommand、StateQuery）なら、その状態（か、述語で決まる状態の分類）の FQN。それ以外は ""。
	Unit string
	// Ctx は最初の引数に ctx context.Context を取るか（純粋でない）。Err は最後の戻り値に error を返すか
	// （純粋で、かつ例外が空の op だけが返さない）。
	Ctx, Err bool
	// Seams は、この op の継ぎ目のデコレータの FQN（書かれた順。先頭が最も外側）。空なら継ぎ目を持たない。
	Seams []string
	// Body は、継ぎ目を持ち、基底が中身を出す op（Wiring と Never）の、中身のメソッドの名前（`loginBody`）。それ以外は ""。
	Body string
}

// StateUnit は状態の単位一つ（docs/plugin-go.md「状態の単位」）: インターフェース `<状態>State`。
type StateUnit struct {
	// FQN は基底の状態か、述語で決まる状態の分類の FQN。TypeName はインターフェースの名前、Param は New の引数の名前。
	FQN, TypeName, Param string
	// Shape は基底の状態の形（"list"、"nullable"、"single"）。状態の分類なら ""。
	Shape string
	// Queries は op の問い合わせ（StateQuery）の op の FQN、Commands は命令（StateCommand）の op の FQN。どちらも宣言順。
	Queries, Commands []string
	// Predicates は、状態の分類のケースごとの述語のケース名（宣言順。default は持たない）。基底の状態なら空。
	Predicates []string
}

// Dep は別の名前空間の口一つ（docs/plugin-go.md「別の名前空間の op」）: インターフェース `<M>Ops`。
type Dep struct {
	// Namespace は名前空間 M の FQN。TypeName はインターフェースの名前、Param は New の引数の名前。
	Namespace, TypeName, Param string
	// Ops は、M の op のうち、単位の対象の配線が呼ぶものの FQN（M の宣言順）。
	Ops []string
}

// Seam は継ぎ目一つ（docs/plugin-go.md「Seams」）: op の FQN、デコレータの FQN、それを宣言したトランスパイラ
// （View.config.seams の値）、Seams のメソッドの名前。
type Seam struct {
	Op, Decorator, Transpiler, Method string
}

// Param は New の引数一つ: 名前と Go の型。型は、定義のパッケージの中での名前（`Leaves`、`UsersState`、`StoreOps`、`Seams`）。
// 別の名前空間の分類の述語（`<C>Cases`）は、修飾なしの名前で、書くファイルの Imports.Qualifier で修飾する
// （分類の FQN は Shape.Classify に、同じ順である）。
type Param struct {
	Name, Type string
}

// ShapeOf は、この View の出力の、op と口のすべて。設定（単位の対象、単位の継ぎ目）は View から読む。
func ShapeOf(view *ir.View) *Shape {
	if view == nil {
		return &Shape{}
	}
	return shapeOf(view.Program, view.Viewer, fileSet(view.Files), view.Config.Seams)
}

// shapeOf は ShapeOf の中身。inUnit は、N の op が単位の対象か（docs/config.md「in」）、seams は単位の継ぎ目。
func shapeOf(p *ir.Program, viewer string, inUnit func(*ir.Op) bool, seams map[string]string) *Shape {
	s := &Shape{}
	if p == nil {
		return s
	}
	ns := p.Namespace(viewer)
	if ns == nil {
		return s
	}
	declNS := map[string]string{}
	for i := range p.Namespaces {
		for _, d := range p.Namespaces[i].Decls {
			if d != nil {
				if _, dup := declNS[d.Common().Name]; !dup {
					declNS[d.Common().Name] = p.Namespaces[i].Name
				}
			}
		}
	}

	// 単位の対象の配線が呼ぶ op と、match に使う分類
	called := map[string]bool{}
	depOps := map[string]map[string]bool{} // 別の名前空間 → 呼ぶ op
	classifies := map[string]bool{}
	for _, d := range ns.Decls {
		o, ok := d.(*ir.Op)
		if !ok || o.Body == nil || !inUnit(o) {
			continue
		}
		eachCall(o.Body, func(c *ir.Call) {
			called[c.Op] = true
			if m, known := declNS[c.Op]; known && m != viewer {
				if depOps[m] == nil {
					depOps[m] = map[string]bool{}
				}
				depOps[m][c.Op] = true
			}
		})
		eachMatch(o.Body, func(m *ir.Match) {
			c, ok := p.Lookup(m.Classify).(*ir.Classify)
			if !ok || c.DecidedBy != ir.DecidedByPredicate {
				return
			}
			if _, isState := c.SubjectStates(); !isState {
				classifies[c.Name] = true
			}
		})
	}

	// 状態の単位: 基底の状態と、述語で決まる状態の分類（宣言順）
	units := map[string]int{} // FQN → s.States の添字
	for _, d := range ns.Decls {
		switch x := d.(type) {
		case *ir.State:
			units[x.Name] = len(s.States)
			s.States = append(s.States, StateUnit{FQN: x.Name, TypeName: StateUnitName(x.Name), Shape: string(x.Shape)})
		case *ir.Classify:
			if _, isState := x.SubjectStates(); !isState || x.DecidedBy != ir.DecidedByPredicate {
				continue
			}
			u := StateUnit{FQN: x.Name, TypeName: StateUnitName(x.Name)}
			for _, c := range x.Cases {
				u.Predicates = append(u.Predicates, c.Name)
			}
			units[x.Name] = len(s.States)
			s.States = append(s.States, u)
		}
	}

	// N の op と、その置き場
	for _, d := range ns.Decls {
		o, ok := d.(*ir.Op)
		if !ok {
			continue
		}
		target := inUnit(o)
		if !target && !called[o.Name] {
			continue
		}
		os := OpShape{FQN: o.Name, Method: MethodName(o.Name), Exported: o.Exported,
			Ctx: !o.Signature.Pure, Err: !(o.Signature.Pure && len(o.Signature.Exceptions) == 0)}
		os.Place, os.Unit = place(p, o, target, units)
		if target {
			for _, app := range o.Decorators {
				if _, has := seams[app.Decorator]; has && !contains(os.Seams, app.Decorator) {
					os.Seams = append(os.Seams, app.Decorator)
					s.Seams = append(s.Seams, Seam{Op: o.Name, Decorator: app.Decorator, Transpiler: seams[app.Decorator],
						Method: SeamMethod(o.Name, app.Decorator)})
				}
			}
			if len(os.Seams) > 0 && (os.Place == Wiring || os.Place == Never) {
				os.Body = BodyName(o.Name)
			}
		}
		switch os.Place {
		case StateCommand:
			u := &s.States[units[os.Unit]]
			u.Commands = append(u.Commands, o.Name)
		case StateQuery:
			u := &s.States[units[os.Unit]]
			u.Queries = append(u.Queries, o.Name)
		case SharedCommand, SharedQuery:
			s.Shared = append(s.Shared, o.Name)
		case Leaf:
			s.Leaves = append(s.Leaves, o.Name)
		}
		s.Ops = append(s.Ops, os)
	}

	// New が述語を受け取る分類（FQN の辞書順）と、別の名前空間の口（FQN の辞書順。メソッドは相手の宣言順）
	for fqn := range classifies {
		s.Classify = append(s.Classify, fqn)
	}
	sort.Strings(s.Classify)
	var deps []string
	for m := range depOps {
		deps = append(deps, m)
	}
	sort.Strings(deps)
	for _, m := range deps {
		typeName, param := DepName(m)
		dep := Dep{Namespace: m, TypeName: typeName, Param: param}
		if other := p.Namespace(m); other != nil {
			for _, d := range other.Decls {
				if o, ok := d.(*ir.Op); ok && depOps[m][o.Name] {
					dep.Ops = append(dep.Ops, o.Name)
				}
			}
		}
		s.Deps = append(s.Deps, dep)
	}

	// New の引数（docs/plugin-go.md「New」の表）。名前は、先に並んだ引数と当たらなくなるまで末尾に _ を足す
	// New の本体は、構造体 ops のリテラルを書く（`&ops{…}`）ので、引数に ops という名前を使わない
	taken := map[string]bool{"ops": true}
	hasInner := false
	for _, o := range s.Ops {
		hasInner = hasInner || !o.Exported
	}
	if hasInner {
		// 分けた出力の入口の New は inner.New(…) を呼ぶので、引数に inner という名前を使わない
		taken[innerDir] = true
	}
	param := func(name, typ string) string {
		name = LocalName(lowerFirst(name), nil)
		for taken[name] {
			name += "_"
		}
		taken[name] = true
		s.New = append(s.New, Param{Name: name, Type: typ})
		return name
	}
	if len(s.Leaves) > 0 {
		param("leaves", "Leaves")
	}
	for i := range s.States {
		s.States[i].Param = param(short(s.States[i].FQN), s.States[i].TypeName)
	}
	if len(s.Shared) > 0 {
		param("shared", "SharedState")
	}
	for _, fqn := range s.Classify {
		param(short(fqn), TypeName(fqn, declNS[fqn])+"Cases")
	}
	for i := range s.Deps {
		s.Deps[i].Param = param(s.Deps[i].Param, s.Deps[i].TypeName)
	}
	if len(s.Seams) > 0 {
		param("seams", "Seams")
	}
	return s
}

// place は op o の置き場と、状態の単位の FQN（docs/plugin-go.md「op の置き場」: 上から順に、最初に当たる行）。
// target は o が単位の対象か。units は、N の状態の単位（基底の状態と、述語で決まる状態の分類）。
func place(p *ir.Program, o *ir.Op, target bool, units map[string]int) (Place, string) {
	switch {
	case o.Body != nil && target:
		return Wiring, "" // 1
	case o.Body != nil:
		return Leaf, "" // 6: 単位の対象でない配線は、本体を出さず、人が実装する葉として扱う
	case ir.IsNever(o.Returns):
		return Never, "" // 2
	}
	if effects := o.DirectEffects(); len(effects) > 0 { // 3: 効果の宛先の単位が、すべての効果で同じか
		unit := ""
		for _, fx := range effects {
			u := effectUnit(p, fx)
			if _, ok := units[u]; !ok {
				return Leaf, "" // 宛先の単位が N にない（検査を通った IR には来ない。STATE-2）
			}
			if unit != "" && u != unit {
				return SharedCommand, ""
			}
			unit = u
		}
		return StateCommand, unit
	}
	switch reads := p.ReadStates(o.DirectReads()); { // 直に読む状態（read の宣言は、その宛先の状態として数える）
	case len(reads) == 1: // 4
		if _, ok := units[reads[0]]; ok {
			return StateQuery, reads[0]
		}
		return Leaf, ""
	case len(reads) >= 2: // 5
		return SharedQuery, ""
	}
	return Leaf, "" // 6
}

// effectUnit は、効果 fx の宛先の単位（docs/plugin-go.md「op の置き場」の「効果の宛先の単位」）: 組み込みの効果か、基底の状態の
// 遷移なら、その基底の状態。状態の分類の遷移なら、その分類が述語で決まるならその分類、型で決まるなら、その分類のただ一つの
// 対象の状態。直に書いた効果（docs/ir.md「op」の EffectRef の direct）は、組み込みの効果なので、その状態。分からなければ ""。
func effectUnit(p *ir.Program, fx ir.EffectRef) string {
	named, ok := fx.(ir.NamedEffect)
	if !ok {
		if direct, ok := fx.(ir.DirectEffect); ok {
			return direct.State
		}
		return ""
	}
	e, ok := p.Lookup(named.Effect).(*ir.Effect)
	if !ok {
		return ""
	}
	switch t := e.Target.(type) {
	case ir.BuiltinTarget:
		return t.State
	case ir.TransitionTarget:
		switch d := p.Lookup(t.Of).(type) {
		case *ir.State:
			return d.Name
		case *ir.Classify:
			if d.DecidedBy == ir.DecidedByPredicate {
				return d.Name
			}
			if states, ok := d.SubjectStates(); ok && len(states) == 1 {
				return states[0]
			}
		}
	}
	return ""
}

// Signature は、op のメソッドの引数と戻り値のテキスト（docs/plugin-go.md「op の形」）。形は op の宣言だけで決まり、
// どのインターフェースに現れても、どの名前空間から見ても同じ。
//
//   - 引数: 純粋でない op は、最初に `ctx context.Context`。続けて Op.params の並びのまま `<局所の名前> <型>`。
//   - 戻り値: returns が型なら、その型（void と never は、値はない）。純粋で、かつ例外が空の op を除いて、最後に error。
//     二つなら `(T, error)`、何もなければ ""。
//
// 必要な import（context、型のパッケージ）を imports に足す。reserved は、局所の名前が避ける名前（LocalName）。
func Signature(op *ir.Op, imports *Imports, reserved map[string]bool) (params, results string) {
	var ps []string
	if !op.Signature.Pure {
		ps = append(ps, "ctx "+qualify(imports, "context", "context")+".Context")
	}
	for _, p := range op.Params {
		ps = append(ps, LocalName(p.Name, reserved)+" "+GoType(p.Type, imports))
	}
	ret := GoType(op.Returns, imports)
	hasErr := !(op.Signature.Pure && len(op.Signature.Exceptions) == 0)
	switch {
	case ret != "" && hasErr:
		results = "(" + ret + ", error)"
	case ret != "":
		results = ret
	case hasErr:
		results = "error"
	}
	return strings.Join(ps, ", "), results
}

// SeamSignature は、継ぎ目のメソッド（インターフェース Seams のメソッド）の引数と戻り値のテキスト（docs/plugin-go.md「Seams」）:
// 引数は、op の形の引数の後ろに `body func(<op の形の引数>) <op の形の戻り値>`、戻り値は op の形の戻り値。
// body の関数の型には、引数の名前も書く。body という名前が op の引数の局所の名前と当たれば、当たらなくなるまで、
// body の方の末尾に _ を足す。
func SeamSignature(op *ir.Op, imports *Imports, reserved map[string]bool) (params, results string) {
	params, results = Signature(op, imports, reserved)
	fn := "func(" + params + ")"
	if results != "" {
		fn += " " + results
	}
	return params + ", " + SeamBodyParam(op, reserved) + " " + fn, results
}

// SeamBodyParam は、継ぎ目のメソッドの最後の引数（op の中身）の名前: `body`。op の引数の局所の名前と当たれば、当たらなくなるまで
// 末尾に _ を足す（docs/plugin-go.md「Seams」）。
func SeamBodyParam(op *ir.Op, reserved map[string]bool) string {
	used := map[string]bool{"ctx": true}
	for _, p := range op.Params {
		used[LocalName(p.Name, reserved)] = true
	}
	name := "body"
	for used[name] {
		name += "_"
	}
	return name
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
	for _, st := range w.Statements {
		switch x := st.(type) {
		case ir.ConstStmt:
			expr(x.Expr)
		case ir.ReturnStmt:
			expr(x.Expr)
		case ir.CallStmt:
			fn(&x.Call)
		case ir.MatchStmt:
			expr(ir.MatchExpr{Match: x.Match})
		}
	}
}

// eachMatch は配線のすべての match（文、束縛、return）に fn を呼ぶ。
func eachMatch(w *ir.Wiring, fn func(m *ir.Match)) {
	for _, st := range w.Statements {
		switch x := st.(type) {
		case ir.ConstStmt:
			if m, ok := x.Expr.(ir.MatchExpr); ok {
				fn(&m.Match)
			}
		case ir.ReturnStmt:
			if m, ok := x.Expr.(ir.MatchExpr); ok {
				fn(&m.Match)
			}
		case ir.MatchStmt:
			fn(&x.Match)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
