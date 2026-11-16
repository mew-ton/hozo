package gosqlite

import (
	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは GOSQL-4（docs/plugin-go-sqlite.md「診断」）: このトランスパイラの公開の名前（Schema、CreateTables、
// <状態>Table、New<状態>Table）が、N の定義のパッケージの名前（docs/plugin-go.md「名前」）と、または互いに当たる。
// db.go は入口のパッケージに置くが、入口は定義のすべての名前を再公開するので、当たる相手は定義のパッケージの名前。

// owned は、パッケージの名前一つと、それを生んだもの。
type owned struct {
	what string // メッセージに書く、生んだもの（「model store.Schema」）
	at   string // 診断の位置にする宣言の FQN（宣言が生んだ名前でなければ ""）
	ours bool   // このトランスパイラの名前か（そうでなければ、基底の名前）
}

// baseNames は、基底 go が N の定義のパッケージに出す公開の名前（docs/plugin-go.md「名前」の「名前が当たれば GO-2」が並べるもの）。
// 名前は plugin/golang の関数で作る。
func (e *emitter) baseNames() map[string]owned {
	names := map[string]owned{}
	add := func(name, what, at string) {
		if _, dup := names[name]; !dup {
			names[name] = owned{what: what, at: at}
		}
	}
	for _, d := range e.ns.Decls {
		name := d.Common().Name
		switch x := d.(type) {
		case *ir.Model:
			if x.SynthesizedFrom != "" {
				if _, fromOp := e.p.Lookup(x.SynthesizedFrom).(*ir.Op); fromOp {
					continue // View が見る側のために合成した model は、基底が出さない（docs/plugin-go.md「出力の対象」）
				}
			}
			add(e.typeName(name), "model "+name, name)
			for _, f := range x.Fields {
				if f.Default != nil {
					add("New"+e.typeName(name), "model "+name+" の New"+e.typeName(name), name)
					break
				}
			}
		case *ir.Scalar, *ir.Exception:
			add(e.typeName(name), d.Kind()+" "+name, name)
		case *ir.Union:
			if x.Variant != ir.UnionArms {
				continue
			}
			add(e.typeName(name), "union "+name, name)
			for _, a := range x.Arms {
				// アームの型が、union を宣言した名前空間の model への ref でなければ、包む型 <Union><アーム名>（同「union」）
				if r, ok := a.Type.(ir.RefType); ok {
					if m, isModel := e.p.Lookup(r.Target).(*ir.Model); isModel && e.declNS[m.Name] == e.declNS[name] {
						continue
					}
				}
				add(e.typeName(name)+golang.Capitalize(a.Name), "union "+name+" のアーム "+a.Name+" を包む型", name)
			}
		case *ir.Effect:
			add(golang.EffectConst(name), "effect "+name, name)
		case *ir.Classify:
			if x.DecidedBy != ir.DecidedByPredicate {
				continue
			}
			typ := e.typeName(name)
			add(typ, "classify "+name, name)
			for _, c := range x.Cases {
				k := golang.CaseConst(typ, c.Name)
				add(k, "classify "+name+" のケースの定数 "+k, name)
			}
			if x.HasDefault {
				k := golang.CaseConst(typ, "default")
				add(k, "classify "+name+" のケースの定数 "+k, name)
			}
			add("Classify"+typ, "classify "+name+" の判定関数", name)
			if _, isState := x.SubjectStates(); !isState {
				add(typ+"Cases", "classify "+name+" の述語のインターフェース", name)
			}
		}
	}
	s := e.shape
	for _, u := range s.States {
		add(u.TypeName, "状態の単位 "+u.FQN, u.FQN)
	}
	if len(s.Ops) > 0 {
		add("Ops", "インターフェース Ops", "")
		add("New", "関数 New", "")
	}
	if len(s.Leaves) > 0 {
		add("Leaves", "インターフェース Leaves", "")
	}
	if len(s.Shared) > 0 {
		add("SharedState", "インターフェース SharedState", "")
	}
	if len(s.Seams) > 0 {
		add("Seams", "インターフェース Seams", "")
	}
	for _, d := range s.Deps {
		add(d.TypeName, "別の名前空間 "+d.Namespace+" の口", "")
	}
	return names
}

// checkNames は GOSQL-4。位置は、当たった宣言（基底の名前を生んだ宣言。このトランスパイラの名前どうしなら、後ろの状態）。
func (e *emitter) checkNames() {
	if len(e.tables) == 0 {
		return
	}
	names := e.baseNames()
	type public struct{ name, what, at string }
	ours := []public{
		{"Schema", "go-sqlite の定数 Schema", ""},
		{"CreateTables", "go-sqlite の関数 CreateTables", ""},
	}
	for _, t := range e.tables {
		ours = append(ours,
			public{t.goName, "go-sqlite の型 " + t.goName + "（状態 " + t.state.Name + "）", t.state.Name},
			public{"New" + t.goName, "go-sqlite の関数 New" + t.goName + "（状態 " + t.state.Name + "）", t.state.Name})
	}
	for _, p := range ours {
		prev, taken := names[p.name]
		if !taken {
			names[p.name] = owned{what: p.what, at: p.at, ours: true}
			continue
		}
		at := prev.at
		if at == "" || prev.ours {
			at = p.at
		}
		e.errorf(codeSQLCollision, at, "入口のパッケージ（%s）の名前 %s が当たる: %s と %s（直すのは仕様の名前）", e.view.Config.Dir, p.name, prev.what, p.what)
	}
}
