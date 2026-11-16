package gosqlite

import "github.com/mew-ton/hozo/plugin/golang"

// このファイルは GOSQL-4（docs/plugin-go-sqlite.md「診断」）: このトランスパイラの公開の名前（Schema、CreateTables、Querier、Conn、
// <状態>Table、New<状態>Table、出すなら DbSeams、NewDbSeams）が、N の定義のパッケージの名前（docs/plugin-go.md「名前」）と、
// または互いに当たる。db.go は入口のパッケージに置くが、入口は定義のすべての名前を再公開するので、当たる相手は定義のパッケージの名前。

// owned は、パッケージの名前一つと、それを生んだもの。
type owned struct {
	what string // メッセージに書く、生んだもの（「model store.Schema」）
	at   string // 診断の位置にする宣言の FQN（宣言が生んだ名前でなければ ""）
	ours bool   // このトランスパイラの名前か（そうでなければ、基底の名前）
}

// baseNames は、基底 go が N の定義のパッケージに出す公開の名前（docs/plugin-go.md「名前」の「名前が当たれば GO-2」が並べるもの。
// 一覧は、基底と共有する golang.PublicNames）。同じ名前が二つあれば、先のもの。
func (e *emitter) baseNames() map[string]owned {
	names := map[string]owned{}
	for _, n := range golang.PublicNames(e.view) {
		if _, dup := names[n.Name]; !dup {
			names[n.Name] = owned{what: n.What, at: n.Decl}
		}
	}
	return names
}

// checkNames は GOSQL-4。位置は、当たった宣言: 基底の名前を生んだ宣言。このトランスパイラの名前どうしなら、後ろの宣言
// （型を出させた状態。DbSeams と NewDbSeams は、それを出させた最初の op）。
func (e *emitter) checkNames() {
	if len(e.units) == 0 {
		return
	}
	names := e.baseNames()
	type public struct{ name, what, at string }
	ours := []public{
		{"Schema", "go-sqlite の定数 Schema", ""},
		{"CreateTables", "go-sqlite の関数 CreateTables", ""},
		{"Querier", "go-sqlite の型 Querier", ""},
		{"Conn", "go-sqlite の関数 Conn", ""},
	}
	for _, u := range e.units {
		ours = append(ours,
			public{u.goName, "go-sqlite の型 " + u.goName + "（状態 " + u.fqn + "）", u.fqn},
			public{"New" + u.goName, "go-sqlite の関数 New" + u.goName + "（状態 " + u.fqn + "）", u.fqn})
	}
	if len(e.seams) > 0 {
		at := e.seams[0].Op
		ours = append(ours,
			public{"DbSeams", "go-sqlite の型 DbSeams（@db.transaction の継ぎ目）", at},
			public{"NewDbSeams", "go-sqlite の関数 NewDbSeams（@db.transaction の継ぎ目）", at})
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
