package golang

import "github.com/mew-ton/hozo/ir"

// このファイルは、定義のパッケージの公開の名前（docs/plugin-go.md「名前」の「名前が当たれば GO-2」が並べるもの）の写し。
// 基底は、この一覧の中で名前が当たれば GO-2 を返し、入口か定義のパッケージに公開の名前を足す相乗りは、自分の名前がこの一覧の
// 名前と当たれば、自分の診断で報告する（同「相乗りのファイル」）。入口のパッケージは、定義のすべての名前を再公開するので、
// どちらのパッケージに足す名前も、当たる相手はこの一覧。

// PublicKind は、定義のパッケージの公開の名前の種類。
type PublicKind int

const (
	// PublicType は model、scalar、union、exception の型。
	PublicType PublicKind = iota
	// PublicArm は union のアームを包む型 `<Union><アーム名>`。
	PublicArm
	// PublicEffect は effect の定数 `Effect<名前>`。
	PublicEffect
	// PublicClassify は、述語で決まる分類の列挙の型 `<C>`。
	PublicClassify
	// PublicCase は、そのケースの定数 `<C><ケース名>`。
	PublicCase
	// PublicClassifyFunc は判定関数 `Classify<C>`。
	PublicClassifyFunc
	// PublicCases は、述語のインターフェース `<C>Cases`（値の分類と op の分類だけ）。
	PublicCases
	// PublicStateUnit は状態の単位 `<状態>State`。
	PublicStateUnit
	// PublicOps、PublicNew、PublicLeaves、PublicSeams は、生成したコードが決めた名前 `Ops`、`New`、`Leaves`、`Seams`。
	PublicOps
	PublicNew
	PublicLeaves
	PublicSeams
	// PublicDep は別の名前空間の口 `<Ns>Ops`。
	PublicDep
)

// PublicName は、定義のパッケージの公開の名前一つ。
type PublicName struct {
	// Name は Go の名前。
	Name string
	// Kind は名前の種類。
	Kind PublicKind
	// Decl は、その名前を生んだ宣言の FQN（状態の単位なら、状態か状態の分類）。診断の位置に使う。生成したコードが決めた名前
	// （Ops、New、Leaves、Seams）と、別の名前空間の口なら ""。
	Decl string
	// Detail は、PublicArm ならアーム名、PublicCase ならケース名、PublicDep なら名前空間の FQN。ほかは ""。
	Detail string
	// What は、その名前を生んだものの、診断のメッセージ用の言い方（`model store.Schema`、`インターフェース Ops`）。
	What string
}

// PublicNames は、view の出力 (U, N) の定義のパッケージの公開の名前のすべて（docs/plugin-go.md「名前」の「名前が当たれば GO-2」）。
// 並びは、N の宣言順（宣言ごとに、型、アームを包む型。分類は、列挙の型、ケースの定数、判定関数、述語の
// インターフェース）、状態の単位（宣言順）、`Ops`、`New`、`Leaves`、`Seams`、別の名前空間の口（FQN の辞書順）。
// 当たる名前も、そのまま二つ返す（当たっているかを見るのは呼ぶ側）。View が見る側のために合成した model、`set` の union、
// 型で決まる分類は、Go の名前にならないので、入らない。
func PublicNames(view *ir.View) []PublicName {
	p := view.Program
	if p == nil {
		return nil
	}
	ns := p.Namespace(view.Viewer)
	if ns == nil {
		return nil
	}
	declNS := map[string]string{}
	for i := range p.Namespaces {
		for _, d := range p.Namespaces[i].Decls {
			if d == nil {
				continue
			}
			if _, dup := declNS[d.Common().Name]; !dup {
				declNS[d.Common().Name] = p.Namespaces[i].Name
			}
		}
	}
	typeName := func(fqn string) string { return TypeName(fqn, declNS[fqn]) }
	var out []PublicName
	add := func(name string, kind PublicKind, decl, detail, what string) {
		out = append(out, PublicName{Name: name, Kind: kind, Decl: decl, Detail: detail, What: what})
	}
	for _, d := range ns.Decls {
		if d == nil {
			continue
		}
		name := d.Common().Name
		switch x := d.(type) {
		case *ir.Model:
			if x.SynthesizedFrom != "" {
				if _, fromOp := p.Lookup(x.SynthesizedFrom).(*ir.Op); fromOp {
					continue // View が見る側のために合成した model は、基底が出さない（docs/plugin-go.md「出力の対象」）
				}
			}
			// model から出す名前は、構造体の型だけ（docs/plugin-go.md「model」: 値を作る関数は出さない）
			add(typeName(name), PublicType, name, "", "model "+name)
		case *ir.Scalar, *ir.Exception:
			add(typeName(name), PublicType, name, "", d.Kind()+" "+name)
		case *ir.Union:
			if x.Variant != ir.UnionArms {
				continue
			}
			add(typeName(name), PublicType, name, "", "union "+name)
			for _, a := range x.Arms {
				// アームの型が、union を宣言した名前空間の model への ref でなければ、包む型 <Union><アーム名>（同「union」）
				if r, ok := a.Type.(ir.RefType); ok {
					if m, isModel := p.Lookup(r.Target).(*ir.Model); isModel && declNS[m.Name] == declNS[name] {
						continue
					}
				}
				add(typeName(name)+Capitalize(a.Name), PublicArm, name, a.Name, "union "+name+" のアーム "+a.Name+" を包む型")
			}
		case *ir.Effect:
			add(EffectConst(name), PublicEffect, name, "", "effect "+name)
		case *ir.Classify:
			if x.DecidedBy != ir.DecidedByPredicate {
				continue
			}
			typ := typeName(name)
			add(typ, PublicClassify, name, "", "classify "+name)
			for _, c := range x.Cases {
				k := CaseConst(typ, c.Name)
				add(k, PublicCase, name, c.Name, "classify "+name+" のケースの定数 "+k)
			}
			add("Classify"+typ, PublicClassifyFunc, name, "", "classify "+name+" の判定関数")
			if _, isState := x.SubjectStates(); !isState {
				add(typ+"Cases", PublicCases, name, "", "classify "+name+" の述語のインターフェース")
			}
		}
	}
	s := ShapeOf(view)
	for _, u := range s.States {
		add(u.TypeName, PublicStateUnit, u.FQN, "", "状態の単位 "+u.FQN)
	}
	if len(s.Ops) > 0 {
		add("Ops", PublicOps, "", "", "インターフェース Ops")
		add("New", PublicNew, "", "", "関数 New")
	}
	if len(s.Leaves) > 0 {
		add("Leaves", PublicLeaves, "", "", "インターフェース Leaves")
	}
	if len(s.Seams) > 0 {
		add("Seams", PublicSeams, "", "", "インターフェース Seams")
	}
	for _, d := range s.Deps {
		add(d.TypeName, PublicDep, "", d.Namespace, "別の名前空間 "+d.Namespace+" の口")
	}
	return out
}

// TestName は、テストのパッケージ（docs/plugin-go.md「単体テストの口」の「テストのパッケージ」）の、基底の公開の名前一つ。
type TestName struct {
	// Name は Go の名前。
	Name string
	// Decl は、その名前を生んだ宣言の FQN（葉の op か、分類）。診断の位置に使う。まとめの LeafCases と RunLeaves なら ""。
	Decl string
	// What は、その名前を生んだものの、診断のメッセージ用の言い方（`op server.newSession のケースの口 NewSessionCases`）。
	What string
}

// PublicTestNames は、view の出力 (U, N) の、テストのパッケージに、基底が出す公開の名前のすべて（docs/plugin-go.md「単体テストの口」）。
// 並びは、まとめの `LeafCases` と `RunLeaves`（葉が一つでもあれば）、葉ごとの `<Op>Cases` と `<Op>Case`（Leaves の並び）、
// 判定の口を出す分類（N が宣言した、述語で決まる値の分類と op の分類。宣言順）ごとの `<C>Examples` と `Run<C>`。
// 口が一つもない出力（基底が、テストのパッケージを作らない出力）では、空。当たる名前も、そのまま二つ返す（当たっているかを
// 見るのは呼ぶ側: 基底は GO-2、このパッケージにファイルを足す相乗りは、自分の診断。同「相乗りのファイル」）。
func PublicTestNames(view *ir.View) []TestName {
	p := view.Program
	if p == nil {
		return nil
	}
	ns := p.Namespace(view.Viewer)
	if ns == nil {
		return nil
	}
	var out []TestName
	leaves := ShapeOf(view).Leaves
	if len(leaves) > 0 {
		out = append(out,
			TestName{Name: "LeafCases", What: "葉のケースのまとめ LeafCases"},
			TestName{Name: "RunLeaves", What: "葉のケースを走らせる RunLeaves"})
	}
	for _, fqn := range leaves {
		m := MethodName(fqn)
		out = append(out,
			TestName{Name: m + "Cases", Decl: fqn, What: "op " + fqn + " のケースの口 " + m + "Cases"},
			TestName{Name: m + "Case", Decl: fqn, What: "op " + fqn + " のケース " + m + "Case"})
	}
	for _, d := range ns.Decls {
		c, ok := d.(*ir.Classify)
		if !ok || c.DecidedBy != ir.DecidedByPredicate {
			continue
		}
		if _, isState := c.SubjectStates(); isState {
			continue
		}
		name := TypeName(c.Name, view.Viewer)
		out = append(out,
			TestName{Name: name + "Examples", Decl: c.Name, What: "分類 " + c.Name + " の例の口 " + name + "Examples"},
			TestName{Name: "Run" + name, Decl: c.Name, What: "分類 " + c.Name + " の例を走らせる Run" + name})
	}
	return out
}
