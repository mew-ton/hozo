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
	// 定義のパッケージの公開の名前どうし（一覧は、相乗りと共有する golang.PublicNames）
	pkg := e.names("定義のパッケージ（" + e.layout.DefDir + "）")
	for _, n := range golang.PublicNames(e.view) {
		pkg.add(n.Name, n.What, n.Decl)
	}

	// 一つの構造体のフィールドどうし（例外の引数を含む）
	for _, d := range e.ns.Decls {
		switch x := d.(type) {
		case *ir.Model:
			if !e.typeDecl(d) {
				continue
			}
			fields := e.names("model " + x.Name + " の構造体")
			for _, fd := range x.Fields {
				fields.add(golang.FieldName(fd.Name), "フィールド "+fd.Name, x.Name)
			}
		case *ir.Exception:
			fields := e.names("exception " + x.Name + " の構造体")
			for _, p := range x.Params {
				fields.add(golang.FieldName(p.Name), "引数 "+p.Name, x.Name)
			}
		}
	}

	// 一つのインターフェースのメソッドどうし: Ops（`foo` と `Foo`）
	ops := e.names("インターフェース Ops")
	for _, o := range s.Ops {
		ops.add(o.Method, "op "+o.FQN, o.FQN)
	}
	// 状態の単位: 形で決まるメソッド（IsEmpty / Get と、出す Post / Put / Delete）、ケースの述語と、その単位の read と効果の名前を
	// 先頭大文字にしたもの
	for _, u := range s.States {
		methods := e.names("状態の単位 " + u.TypeName)
		switch u.Shape {
		case string(ir.StateList), string(ir.StateNullable):
			methods.add("IsEmpty", "形で決まる問い合わせ IsEmpty", u.FQN)
			methods.add("Get", "形で決まる問い合わせ Get", u.FQN)
		case string(ir.StateSingle):
			methods.add("Get", "形で決まる問い合わせ Get", u.FQN)
		}
		for _, operation := range u.Builtins {
			methods.add(golang.BuiltinMethod(operation), "形で決まる命令 "+golang.BuiltinMethod(operation)+"（配線が直に呼ぶ "+u.FQN+"."+operation+"）", u.FQN)
		}
		for _, c := range u.Predicates {
			methods.add(golang.Capitalize(c), "ケース "+c+" の述語", u.FQN)
		}
		for _, fqn := range u.Reads {
			methods.add(golang.MethodName(fqn), "read "+fqn, fqn)
		}
		for _, fqn := range u.Effects {
			methods.add(golang.MethodName(fqn), "effect "+fqn, fqn)
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
