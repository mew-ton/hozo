package golang

import (
	"sort"
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// Place は op の置き場（docs/plugin-go.md「op の置き場」の表の 1〜3）: N の op の実装が、どこにあるか。
// op は、関数の枠だけを作る。状態に触れるのは read と効果（どちらも状態の単位のメソッド）で、op ではないので、置き場は三つだけ。
type Place int

const (
	// Wiring は、単位の対象の配線（本体を持つ op）。実装は、生成した本体（wiring.go）。
	Wiring Place = iota + 1
	// Never は、never を返す op。実装は、生成した本体（ops.go）: `return <例外の型>{}`。
	Never
	// Leaf は、葉（本体を持たない op）と、単位の対象でない配線。実装は、Leaves。葉に付いたデコレータが reads / effects を
	// 運んでいても（Op.carried）、置き場は変わらない。
	Leaf
)

// Shape は、一つの出力 (U, N) の、op と、実装を受け取る口のすべて（docs/plugin-go.md「出力の対象」「実装を受け取る口」「継ぎ目」）。
// 基底も相乗りも、ShapeOf の同じ値から名前と並びを求める。
type Shape struct {
	// Ops は N の op: 単位の対象の op と、単位の対象の配線が呼ぶ、単位の対象でない N の op（本体のない op として出すもの）。宣言順。
	Ops []OpShape
	// Leaves は、インターフェース Leaves のメソッドになる op の FQN（宣言順）: 置き場が Leaf の op。空なら Leaves も New の引数 leaves もない。
	Leaves []string
	// States は状態の単位: 基底の状態と、述語で決まる状態の分類。宣言順。N の read と効果は、どれも、このどれか一つのメソッド。
	States []StateUnit
	// Classify は、New が述語を受け取る、値の分類と op の分類の FQN: 単位の対象の配線が match に使う、述語で決まるもの
	// （別の名前空間のものを含む）。FQN の辞書順。
	Classify []string
	// Deps は別の名前空間の口: 単位の対象の配線が呼ぶ別の名前空間ごとに一つ。FQN の辞書順。
	Deps []Dep
	// Seams は、インターフェース Seams のメソッド: 継ぎ目を持つ op と、それに付いた、単位の継ぎ目にあるデコレータの組ごとに一つ。
	// op の宣言順、その中はデコレータの書かれた順。空なら Seams も New の引数 seams もない。
	Seams []Seam
	// New は New の引数（名前と Go の型）。この順: leaves、状態の単位、分類の述語、別の名前空間、seams。
	// 構造体 ops のフィールドも、同じ名前、同じ順。
	New []Param
}

// OpShape は N の op 一つの形（docs/plugin-go.md「op の形」「op の置き場」「継ぎ目」）。メソッドは、どの op も、最初の引数に
// ctx context.Context を取り、最後の戻り値に error を返す（同「op の形」）ので、それを言うフィールドは持たない。
type OpShape struct {
	// FQN は op の FQN。Method はメソッドの名前（op 名を先頭大文字にしたもの）。
	FQN, Method string
	// Exported は export が付いているか。入口の Ops に並ぶのは、これが真の op だけ。
	Exported bool
	// Place は置き場。
	Place Place
	// Seams は、この op の継ぎ目のデコレータの FQN（書かれた順。先頭が最も外側）。空なら継ぎ目を持たない。
	Seams []string
	// Body は、継ぎ目を持ち、基底が中身を出す op（Wiring と Never）の、中身のメソッドの名前（`loginBody`）。それ以外は ""。
	Body string
}

// StateUnit は状態の単位一つ（docs/plugin-go.md「状態の単位」）: インターフェース `<状態>State`。その状態に触れるもののすべて。
// メソッドの並びは、形で決まる問い合わせ（基底の状態。Shape から決まる IsEmpty / Get）か、ケースごとの述語（状態の分類。Predicates）、
// 形で決まる命令（Builtins）、read（Reads）、効果（Effects）。どれも ctx を取り、error を返す。
type StateUnit struct {
	// FQN は基底の状態か、述語で決まる状態の分類の FQN。TypeName はインターフェースの名前、Param は New の引数の名前。
	FQN, TypeName, Param string
	// Shape は基底の状態の形（"list"、"nullable"、"single"）。状態の分類なら ""。
	Shape string
	// Builtins は、形で決まる命令の操作（"post"、"put"、"delete" の順）: 単位の対象の配線が、この状態の組み込みの効果を直に
	// 呼んでいるものだけ。メソッドの名前は BuiltinMethod、形は BuiltinSignature。状態の分類なら空。
	Builtins []string
	// Reads は、この状態を読む read（Read.state が、この状態）の FQN。N の read のすべてを、配線が呼んでいるかによらず、宣言順に。
	// メソッドの名前は MethodName、形は ReadSignature。状態の分類なら空（read の宛先は、基底の状態）。
	Reads []string
	// Effects は、この単位を宛先にする効果の FQN（同「状態の単位」の「効果の宛先の単位」）。N の効果のすべてを、配線が呼んでいるかに
	// よらず、宣言順に。メソッドの名前は MethodName、形は EffectSignature。
	Effects []string
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

	// 単位の対象の配線が呼ぶ op と、直に呼ぶ組み込みの効果と、match に使う分類
	called := map[string]bool{}
	depOps := map[string]map[string]bool{}         // 別の名前空間 → 呼ぶ op
	builtins := map[string]map[ir.Operation]bool{} // 基底の状態 → 直に呼ぶ操作
	classifies := map[string]bool{}
	for _, d := range ns.Decls {
		o, ok := d.(*ir.Op)
		if !ok || o.Body == nil || !inUnit(o) {
			continue
		}
		eachCall(o.Body, func(c *ir.Call) {
			switch callee := c.Callee.(type) {
			case ir.OpCallee:
				name := callee.Op
				called[name] = true
				if m, known := declNS[name]; known && m != viewer {
					if depOps[m] == nil {
						depOps[m] = map[string]bool{}
					}
					depOps[m][name] = true
				}
			case ir.EffectCallee:
				// 直に呼ぶ組み込みの効果は、その状態の単位の、形で決まる命令になる。read と、宣言した効果の呼び出しは、
				// 単位の形を変えない（N の read と効果は、呼ばれていなくても、すべて単位のメソッド）
				if direct, ok := callee.Effect.(ir.DirectEffect); ok {
					if builtins[direct.State] == nil {
						builtins[direct.State] = map[ir.Operation]bool{}
					}
					builtins[direct.State][direct.Operation] = true
				}
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
			u := StateUnit{FQN: x.Name, TypeName: StateUnitName(x.Name), Shape: string(x.Shape)}
			for _, operation := range builtinOrder {
				if builtins[x.Name][operation] {
					u.Builtins = append(u.Builtins, string(operation))
				}
			}
			units[x.Name] = len(s.States)
			s.States = append(s.States, u)
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
	// N の read と効果: それぞれ、読む状態の単位の問い合わせと、宛先の単位の命令（宣言順。配線が呼んでいるかによらない）。
	// 単位が N にないもの（検査を通った IR には来ない）は、どの単位にも置かない（基底が GO-INTERNAL で報告する）
	for _, d := range ns.Decls {
		switch x := d.(type) {
		case *ir.Read:
			if i, ok := units[x.State]; ok {
				s.States[i].Reads = append(s.States[i].Reads, x.Name)
			}
		case *ir.Effect:
			if i, ok := units[effectUnit(p, x)]; ok {
				s.States[i].Effects = append(s.States[i].Effects, x.Name)
			}
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
		os := OpShape{FQN: o.Name, Method: MethodName(o.Name), Exported: o.Exported, Place: place(o, target)}
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
		if os.Place == Leaf {
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

// builtinOrder は、形で決まる命令の並び（docs/plugin-go.md「状態の単位」: Post、Put、Delete の順）。
var builtinOrder = []ir.Operation{ir.OpPost, ir.OpPut, ir.OpDelete}

// place は op o の置き場（docs/plugin-go.md「op の置き場」: 上から順に、最初に当たる行）。target は o が単位の対象か。
// 読む状態と効果（デコレータが運ぶ Op.carried を含む）は、置き場を変えない: 状態に触れるのは read と効果で、op ではない。
func place(o *ir.Op, target bool) Place {
	switch {
	case o.Body != nil && target:
		return Wiring // 1
	case o.Body == nil && ir.IsNever(o.Returns):
		return Never // 2
	}
	return Leaf // 3: 葉と、単位の対象でない配線（本体を出さず、人が実装する葉として扱う）
}

// effectUnit は、効果 e の宛先の単位（docs/plugin-go.md「状態の単位」の「効果の宛先の単位」）: 組み込みの効果か、基底の状態の
// 遷移なら、その基底の状態。状態の分類の遷移なら、その分類が述語で決まるならその分類、型で決まるなら、その分類のただ一つの
// 対象の状態。分からなければ ""。
func effectUnit(p *ir.Program, e *ir.Effect) string {
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
// どのインターフェースに現れても、どの名前空間から見ても同じ。純粋かどうか（Op.signature.pure）と、throws の有無は、見ない。
//
//   - 引数: いつも、最初に `ctx context.Context`。続けて Op.params の並びのまま `<局所の名前> <型>`。
//   - 戻り値: returns が型なら、その型（void と never は、値はない）。いつも、最後に error。値があれば `(T, error)`、
//     なければ `error`。
//
// 必要な import（context、型のパッケージ）を imports に足す。reserved は、局所の名前が避ける名前（LocalName）。
func Signature(op *ir.Op, imports *Imports, reserved map[string]bool) (params, results string) {
	results = "error"
	if ret := GoType(op.Returns, imports); ret != "" {
		results = "(" + ret + ", error)"
	}
	return unitParams(op.Params, imports, reserved), results
}

// ReadSignature は、read のメソッド（状態の単位の問い合わせ）の引数と戻り値のテキスト（docs/plugin-go.md「状態の単位」）:
// `ctx context.Context` と、Read.params の並びのまま `<局所の名前> <型>`、戻り値は `(<結果の型>, error)`。op と同じに、
// いつも ctx を取り、error を返す。reserved は、局所の名前が避ける名前（LocalName）。
func ReadSignature(r *ir.Read, imports *Imports, reserved map[string]bool) (params, results string) {
	return unitParams(r.Params, imports, reserved), "(" + GoType(r.Returns, imports) + ", error)"
}

// EffectSignature は、効果のメソッド（状態の単位の命令）の引数と戻り値のテキスト（docs/plugin-go.md「状態の単位」）:
// `ctx context.Context` と、Effect.params の並びのまま `<局所の名前> <型>`、戻り値は `error`。
func EffectSignature(e *ir.Effect, imports *Imports, reserved map[string]bool) (params, results string) {
	return unitParams(e.Params, imports, reserved), "error"
}

// BuiltinSignature は、形で決まる命令（直に呼ぶ組み込みの効果。状態 s の単位の Post / Put / Delete）の引数と戻り値のテキスト
// （docs/plugin-go.md「状態の単位」）: `ctx context.Context[, value <状態の要素の型>]` と `error`。値を取るかどうかは、
// docs/check.md「直に呼ぶ組み込みの効果」の表のとおり: 取らないのは delete だけ。引数の名前は、いつも value（ir.DirectEffectParam）。
func BuiltinSignature(s *ir.State, operation string, imports *Imports) (params, results string) {
	params = "ctx " + qualify(imports, "context", "context") + ".Context"
	if BuiltinTakesValue(operation) {
		params += ", " + ir.DirectEffectParam + " " + GoType(s.Element, imports)
	}
	return params, "error"
}

// BuiltinTakesValue は、直に呼ぶ組み込みの効果の操作が、値（状態の要素の型の value 一つ）を取るか（docs/check.md
// 「直に呼ぶ組み込みの効果」の表）: post と put は取り、delete は取らない。
func BuiltinTakesValue(operation string) bool { return ir.Operation(operation) != ir.OpDelete }

// unitParams は、op のメソッドと、状態の単位のメソッド（read と効果）の引数のテキスト: ctx と、宣言の引数の並び。
func unitParams(ps []ir.OpParam, imports *Imports, reserved map[string]bool) string {
	out := []string{"ctx " + qualify(imports, "context", "context") + ".Context"}
	for _, p := range ps {
		out = append(out, LocalName(p.Name, reserved)+" "+GoType(p.Type, imports))
	}
	return strings.Join(out, ", ")
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
	ir.WalkStatements(w.Statements, func(st ir.Statement) {
		if e, ok := ir.StatementExpr(st); ok {
			expr(e)
		}
	})
}

// eachMatch は配線のすべての match（文、束縛、return）に fn を呼ぶ。
func eachMatch(w *ir.Wiring, fn func(m *ir.Match)) {
	ir.WalkStatements(w.Statements, func(st ir.Statement) {
		if e, ok := ir.StatementExpr(st); ok {
			if m, isMatch := e.(ir.MatchExpr); isMatch {
				fn(&m.Match)
			}
		}
	})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
