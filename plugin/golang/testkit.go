package golang

import (
	"strconv"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、テストのにせものと、テストの中の組み立て（docs/plugin-go.md「テストのにせもの」）の形。基底は、ここから fakes.go を
// 作り、その組み立てを使う相乗り（docs/plugin-go-http.md「インテグレーションテストの口」）は、同じ値から、名前と並びを求める。

// KitKind は、New の引数の種類（docs/plugin-go.md「New」の表）。
type KitKind int

const (
	// KitLeaves は、人が実装する葉。にせもの LeavesFake が、本物を包む。
	KitLeaves KitKind = iota + 1
	// KitState は、状態の単位。にせもの `<状態>StateFake`。
	KitState
	// KitClassify は、分類の述語。にせもの `<C>CasesFake` が、本物を包む。
	KitClassify
	// KitExternal は、同じ基底の単位がない、別の名前空間。にせもの `<M>OpsFake`。
	KitExternal
	// KitUnit は、本物を通す、別の名前空間の出力。
	KitUnit
	// KitSeams は、継ぎ目。にせもの SeamsFake。
	KitSeams
)

// TestKit は、一つの出力 (U, N) の、テストのにせものと、テストの中の組み立て（docs/plugin-go.md「テストのにせもの」）。
type TestKit struct {
	// Namespace は、出力する名前空間 N の FQN。Dir は Dir(U, N)。
	Namespace, Dir string
	// Shape は、その出力の、op と口。別の単位の出力なら、その単位の in を省いたものとして求めた形。
	Shape *Shape
	// Layout は、その出力の置き場。TestKitOf に root を渡さなかったとき（""）は nil。
	Layout *Layout
	// Blocked は、テストの中の組み立てを、作れない理由（日本語の句。作れるなら ""）。作れない出力は、fakes.go を持たない。
	Blocked string
	// Params は、New の引数ごとの、渡すもの（New の引数の順）。
	Params []KitParam
	// Reals は、NewOps が、f の後ろに取る引数: 人が書いたもの（この出力の葉と、分類の述語。その後ろに、本物を通す出力のもの）。
	Reals []KitReal
}

// KitParam は、New の引数一つに、NewOps が渡すもの。
type KitParam struct {
	Kind KitKind
	// Param は、New の引数の名前。Field は、Fakes のフィールドの名前（Param の先頭を大文字にしたもの）。本物を通す出力が、
	// fakes.go を持たない（New が、引数を取らない）なら ""。
	Param, Field string
	// FQN は、状態（か、状態の分類）、分類、別の名前空間の FQN。葉と継ぎ目なら ""。
	FQN string
	// Fake は、にせものの型の名前（`LeavesFake`、`UsersStateFake`、`StandingCasesFake`、`MailerOpsFake`、`SeamsFake`）。KitUnit なら ""。
	Fake string
	// Unit は、KitUnit のとき、本物を通す出力。
	Unit *TestKit
	// Entry は、KitExternal が、通す出力のある名前空間の、入口の op の並びか（呼ぶ op が、どれも、入口。相手の入口は、流れの端）。
	Entry bool
}

// KitReal は、NewOps が受け取る、人が書いたもの一つ。
type KitReal struct {
	// Name は、引数の名前（先に並んだ名前と f を避けたもの。import の名前は、書く側が避ける）。
	Name string
	// Kind は KitLeaves か KitClassify。
	Kind KitKind
	// Namespace は、その口を持つ出力の名前空間（葉なら、Leaves の入口のパッケージの名前空間）。Classify は、分類の FQN（葉なら ""）。
	Namespace, Classify string
}

// HasFakes は、その出力が fakes.go（にせものの型、Fakes、NewOps）を持つか: New があり（N に op が一つでもある）、引数を一つでも
// 取り、組み立てを作れる。
func (k *TestKit) HasFakes() bool {
	return k != nil && k.Blocked == "" && len(k.Shape.Ops) > 0 && len(k.Shape.New) > 0
}

// Fakes は、その出力が持つ、にせものの型の Param（Kind が KitUnit でないもの。New の引数の順）。
func (k *TestKit) Fakes() []KitParam {
	var out []KitParam
	for _, p := range k.Params {
		if p.Kind != KitUnit {
			out = append(out, p)
		}
	}
	return out
}

// TestKitOf は、view の出力 (U, N) の、テストのにせものと組み立て。root は、プロジェクトのルート（置き場 Layout を求めるのに使う。
// "" なら、Layout は nil のまま）。置き場を求められなければ、誤り（LayoutOf と同じ）。
func TestKitOf(view *ir.View, root string) (*TestKit, error) {
	if view == nil || view.Program == nil {
		return &TestKit{Shape: &Shape{}}, nil
	}
	b := &kitBuilder{view: view, root: root, building: map[string]bool{}, done: map[string]*TestKit{}}
	k := b.kit(view.Viewer, view.Config, fileSet(view.Files))
	return k, b.err
}

type kitBuilder struct {
	view     *ir.View
	root     string
	building map[string]bool     // 作っている途中の出力（輪を見つける）
	done     map[string]*TestKit // 出来上がった出力
	err      error
}

func kitKey(ns string, index int) string { return ns + "\x00" + strconv.Itoa(index) }

// throughUnit は、単位 cfg の出力から見て、別の名前空間 m の、本物を通す出力: cfg と同じ単位が m も出力するなら、それ。
// なければ、同じ基底の単位が、ちょうど一つなら、それ。n は、同じ基底の単位の数（同じ単位のものがあれば 1）。
func (b *kitBuilder) throughUnit(cfg ir.NamespaceConfig, m string) (unit *ir.NamespaceConfig, n int) {
	list := b.view.Namespaces[m]
	for i := range list {
		if list[i].Index == cfg.Index {
			return &list[i], 1
		}
	}
	for i := range list {
		if list[i].Base == cfg.Base {
			unit = &list[i]
			n++
		}
	}
	if n != 1 {
		return nil, n
	}
	return unit, 1
}

func (b *kitBuilder) kit(ns string, cfg ir.NamespaceConfig, inUnit func(*ir.Op) bool) *TestKit {
	key := kitKey(ns, cfg.Index)
	if k := b.done[key]; k != nil {
		return k
	}
	p := b.view.Program
	s := shapeOf(p, ns, inUnit, cfg.Seams)
	k := &TestKit{Namespace: ns, Dir: cfg.Dir, Shape: s}
	if b.root != "" {
		hasInner := false
		for _, o := range s.Ops {
			hasInner = hasInner || !o.Exported
		}
		l, err := layout(ns, cfg.Dir, b.root, hasInner, HasFuncs(b.view.Program, ns))
		if err != nil && b.err == nil {
			b.err = err
		}
		k.Layout = l
	}
	b.building[key] = true
	defer func() { delete(b.building, key); b.done[key] = k }()

	used := map[string]bool{"f": true}
	real := func(name string, kind KitKind, namespace, classify string) {
		for used[name] {
			name += "_"
		}
		used[name] = true
		k.Reals = append(k.Reals, KitReal{Name: name, Kind: kind, Namespace: namespace, Classify: classify})
	}
	i := 0
	next := func() string { name := s.New[i].Name; i++; return name }
	if len(s.Leaves) > 0 {
		name := next()
		k.Params = append(k.Params, KitParam{Kind: KitLeaves, Param: name, Field: FieldName(name), Fake: "LeavesFake"})
		real(name, KitLeaves, ns, "")
	}
	for _, u := range s.States {
		name := next()
		k.Params = append(k.Params, KitParam{Kind: KitState, Param: name, Field: FieldName(name), FQN: u.FQN, Fake: u.TypeName + "Fake"})
	}
	declNS := map[string]string{}
	for j := range p.Namespaces {
		for _, d := range p.Namespaces[j].Decls {
			if d != nil {
				if _, dup := declNS[d.Common().Name]; !dup {
					declNS[d.Common().Name] = p.Namespaces[j].Name
				}
			}
		}
	}
	for _, fqn := range s.Classify {
		name := next()
		k.Params = append(k.Params, KitParam{Kind: KitClassify, Param: name, Field: FieldName(name), FQN: fqn, Fake: TypeName(fqn, declNS[fqn]) + "CasesFake"})
		real(name, KitClassify, ns, fqn)
	}
	type through struct {
		param string
		unit  *TestKit
	}
	var units []through
	for _, d := range s.Deps {
		name := next()
		unit, n := b.throughUnit(cfg, d.Namespace)
		// 呼ぶ op が、どれも、入口（@entry のデコレータを持つ op）の名前空間は、相手の入口の並びで、流れの端: 通す出力があっても、
		// にせものにする（docs/plugin-go.md「テストのにせもの」の「流れの端」）
		entry := n == 1 && allEntries(p, d.Ops)
		switch {
		case n == 0 || entry:
			k.Params = append(k.Params, KitParam{Kind: KitExternal, Param: name, Field: FieldName(name), FQN: d.Namespace, Fake: d.TypeName + "Fake", Entry: entry})
		case unit == nil:
			if k.Blocked == "" {
				k.Blocked = "名前空間 " + d.Namespace + " を出力する、基底 " + cfg.Base + " の単位が、二つ以上ある"
			}
		case b.building[kitKey(d.Namespace, unit.Index)]:
			if k.Blocked == "" {
				k.Blocked = "名前空間 " + ns + " と " + d.Namespace + " が、輪になって呼び合う"
			}
		default:
			in := func(*ir.Op) bool { return true }
			if unit.Index == b.view.Config.Index {
				in = fileSet(b.view.Files)
			}
			sub := b.kit(d.Namespace, *unit, in)
			if sub.Blocked != "" {
				if k.Blocked == "" {
					k.Blocked = "名前空間 " + d.Namespace + " の出力（" + sub.Dir + "）の組み立てを、作れない: " + sub.Blocked
				}
				continue
			}
			kp := KitParam{Kind: KitUnit, Param: name, FQN: d.Namespace, Unit: sub}
			if sub.HasFakes() {
				kp.Field = FieldName(name)
			}
			k.Params = append(k.Params, kp)
			units = append(units, through{name, sub})
		}
	}
	if len(s.Seams) > 0 {
		name := next()
		k.Params = append(k.Params, KitParam{Kind: KitSeams, Param: name, Field: FieldName(name), Fake: "SeamsFake"})
	}
	for _, u := range units {
		for _, r := range u.unit.Reals {
			real(u.param+Capitalize(r.Name), r.Kind, r.Namespace, r.Classify)
		}
	}
	return k
}

// allEntries は、op の並び ops が、空でなく、どれも、入口のデコレータ（@entry）を持つか。
func allEntries(p *ir.Program, ops []string) bool {
	for _, fqn := range ops {
		o, _ := p.Lookup(fqn).(*ir.Op)
		if o == nil {
			return false
		}
		entry := false
		for i := range o.Decorators {
			if dec, ok := p.Lookup(o.Decorators[i].Decorator).(*ir.Decorator); ok && dec.Entry {
				entry = true
			}
		}
		if !entry {
			return false
		}
	}
	return len(ops) > 0
}

// TestFakeNames は、view の出力 (U, N) の fakes.go の、公開の名前（docs/plugin-go.md「テストのにせもの」）: にせものの型（New の引数の順）、
// `Fakes`、`NewOps`。fakes.go を持たない出力では、空。
func TestFakeNames(view *ir.View) []TestName {
	k, _ := TestKitOf(view, "")
	if !k.HasFakes() {
		return nil
	}
	var out []TestName
	for _, p := range k.Fakes() {
		what, decl := "", p.FQN
		switch p.Kind {
		case KitLeaves:
			what = "葉のにせもの " + p.Fake
		case KitState:
			what = "状態の単位 " + p.FQN + " のにせもの " + p.Fake
		case KitClassify:
			what = "分類 " + p.FQN + " の述語のにせもの " + p.Fake
		case KitExternal:
			what, decl = "別の名前空間 "+p.FQN+" の口のにせもの "+p.Fake, ""
		case KitSeams:
			what = "継ぎ目のにせもの " + p.Fake
		}
		out = append(out, TestName{Name: p.Fake, Decl: decl, What: what})
	}
	out = append(out,
		TestName{Name: "Fakes", What: "にせもののまとめ Fakes"},
		TestName{Name: "NewOps", What: "テストの中の組み立て NewOps"})
	return out
}
