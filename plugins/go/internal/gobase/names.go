package gobase

import (
	"fmt"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは GO-2（docs/plugin-go.md「名前」の「名前が当たれば GO-2」）: 生成する名前が当たる。当たった二つの宣言を
// メッセージに示し、位置は後ろの宣言。直すのは仕様の名前。

// nameSet は、一つの場所（パッケージ、構造体、インターフェース）の名前と、それを生んだ宣言。
type nameSet struct {
	e     *emitter
	where string
	owner map[string]string // Go の名前 → それを生んだもの（宣言の FQN か、生成したコードの名前の説明）
	at    map[string]string // Go の名前 → それを生んだ宣言の FQN（生成したコードが決めた名前なら ""）
}

func (e *emitter) names(where string) *nameSet {
	return &nameSet{e: e, where: where, owner: map[string]string{}, at: map[string]string{}}
}

// add は、名前 name を足す。owner は、それを生んだもの（メッセージ用）、at は診断の位置（宣言の FQN。生成したコードが決めた
// 名前なら ""）。当たれば GO-2 で、位置は後ろの宣言（後ろが宣言でなければ、前の宣言）。
func (s *nameSet) add(name, owner, at string) {
	if prev, taken := s.owner[name]; taken {
		if prev != owner {
			if at == "" {
				at = s.at[name]
			}
			sep := ""
			if r := s.where[len(s.where)-1]; r < 0x80 {
				sep = " " // ASCII で終わる場所の名前（Ops、ops）の後ろは空ける
			}
			s.e.errorf(codeCollision, at, "%s%sの名前 %s が当たる: %s と %s（直すのは仕様の名前）", s.where, sep, name, prev, owner)
		}
		return
	}
	s.owner[name], s.at[name] = owner, at
}

// checkNames は GO-2。
func (e *emitter) checkNames() {
	s := e.shape
	// 定義のパッケージの公開の名前どうし
	pkg := e.names("定義のパッケージ（" + e.layout.DefDir + "）")
	for _, d := range e.ns.Decls {
		if e.typeDecl(d) {
			name := d.Common().Name
			switch x := d.(type) {
			case *ir.Effect:
				pkg.add(golang.EffectConst(name), "effect "+name, name)
			case *ir.Model:
				pkg.add(e.typeName(name), "model "+name, name)
				if hasDefaults(x) {
					pkg.add("New"+e.typeName(name), "model "+name+" の New"+e.typeName(name), name)
				}
			case *ir.Union:
				pkg.add(e.typeName(name), "union "+name, name)
				for _, a := range e.armTypes(x) {
					if a.wrapped {
						pkg.add(a.goName, "union "+name+" のアーム "+a.name+" を包む型", name)
					}
				}
			default:
				pkg.add(e.typeName(name), d.Kind()+" "+name, name)
			}
		}
		if c, ok := d.(*ir.Classify); ok && c.DecidedBy == ir.DecidedByPredicate {
			name := e.typeName(c.Name)
			pkg.add(name, "classify "+c.Name, c.Name)
			consts, _ := e.caseConsts(c)
			for _, k := range consts {
				pkg.add(k, "classify "+c.Name+" のケースの定数 "+k, c.Name)
			}
			pkg.add("Classify"+name, "classify "+c.Name+" の判定関数", c.Name)
			if _, isState := c.SubjectStates(); !isState {
				pkg.add(name+"Cases", "classify "+c.Name+" の述語のインターフェース", c.Name)
			}
		}
	}
	for _, u := range s.States {
		pkg.add(u.TypeName, "状態の単位 "+u.FQN, u.FQN)
	}
	if len(s.Ops) > 0 {
		pkg.add("Ops", "インターフェース Ops", "")
		pkg.add("New", "関数 New", "")
	}
	if len(s.Leaves) > 0 {
		pkg.add("Leaves", "インターフェース Leaves", "")
	}
	if len(s.Shared) > 0 {
		pkg.add("SharedState", "インターフェース SharedState", "")
	}
	if len(s.Seams) > 0 {
		pkg.add("Seams", "インターフェース Seams", "")
	}
	for _, d := range s.Deps {
		pkg.add(d.TypeName, "別の名前空間 "+d.Namespace+" の口", "")
	}

	// 一つの構造体のフィールドどうし
	for _, d := range e.ns.Decls {
		if m, ok := d.(*ir.Model); ok && e.typeDecl(d) {
			fields := e.names("model " + m.Name + " の構造体")
			for _, fd := range m.Fields {
				fields.add(golang.FieldName(fd.Name), "フィールド "+fd.Name, m.Name)
			}
		}
	}

	// 一つのインターフェースのメソッドどうし: Ops（`foo` と `Foo`）
	ops := e.names("インターフェース Ops")
	for _, o := range s.Ops {
		ops.add(o.Method, "op "+o.FQN, o.FQN)
	}
	// 状態の単位: IsEmpty / Get / ケースの述語と、op 名を先頭大文字にしたもの
	for _, u := range s.States {
		methods := e.names("状態の単位 " + u.TypeName)
		switch u.Shape {
		case string(ir.StateList), string(ir.StateNullable):
			methods.add("IsEmpty", "形で決まる問い合わせ IsEmpty", u.FQN)
			methods.add("Get", "形で決まる問い合わせ Get", u.FQN)
		case string(ir.StateSingle):
			methods.add("Get", "形で決まる問い合わせ Get", u.FQN)
		}
		for _, c := range u.Predicates {
			methods.add(golang.Capitalize(c), "ケース "+c+" の述語", u.FQN)
		}
		for _, fqn := range append(append([]string{}, u.Queries...), u.Commands...) {
			methods.add(golang.MethodName(fqn), "op "+fqn, fqn)
		}
	}
	// Seams（op loginRate の @limited と、op login の @rate.limited）
	seams := e.names("インターフェース Seams")
	for _, sm := range s.Seams {
		seams.add(sm.Method, fmt.Sprintf("op %s の @%s", sm.Op, sm.Decorator), sm.Op)
	}
	// 別の名前空間の口
	for _, d := range s.Deps {
		methods := e.names("インターフェース " + d.TypeName)
		for _, fqn := range d.Ops {
			methods.add(golang.MethodName(fqn), "op "+fqn, fqn)
		}
	}
	// 構造体 ops のフィールドと、中身のメソッド（状態 loginBody と、継ぎ目を持つ op login）
	members := e.names("構造体 ops")
	for i, p := range s.New {
		members.add(p.Name, "New の "+fmt.Sprint(i+1)+" 番目の引数 "+p.Name, "")
	}
	for _, o := range s.Ops {
		if o.Body != "" {
			members.add(o.Body, "op "+o.FQN+" の中身のメソッド", o.FQN)
		}
	}
}
