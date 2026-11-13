package golang

import (
	"path"
	"path/filepath"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// dirOf は単位での名前空間の設定 c の出力ディレクトリ Dir(U, N)（docs/emit-go.md「配置」）をルートからの `/` 区切りで。
// Dir が空なら out（単位の namespace 自身の Dir(U, N) と同じ。doc.go「文書にない決定」）。
func dirOf(c ir.NamespaceConfig) string {
	d := c.Dir
	if d == "" {
		d = c.Out
	}
	return path.Clean(filepath.ToSlash(d))
}

// isGoNS は、名前空間 ns を出力する Go の単位があるか（View.namespaces[ns] のどれかの language が go）。
// 複製した宣言のコメントの言い分けにだけ使う（import するかどうかは [Options.Exported] が決める）。
func (e *emitter) isGoNS(ns string) bool {
	if ns == "" {
		return false
	}
	for _, c := range e.view.Namespaces[ns] {
		if c.Language == "go" {
			return true
		}
	}
	return false
}

// importPathOf は別の出力のディレクトリ dir（Dir(U', M)。ルートからの相対パス）のパッケージの import パス。
// go.mod がなければ GOEMIT-1（ディレクトリごとに一度だけ）。
func (e *emitter) importPathOf(dir string) string {
	dir = path.Clean(filepath.ToSlash(dir))
	if p, ok := e.pkgPaths[dir]; ok {
		return p
	}
	p, err := gol.ImportPath(e.opts.Root, dir)
	if err != nil {
		e.errorf(CodeNoGoMod, nil, "出力先 %q の上に go.mod がない: %v", dir, err)
		p = "INVALID/" + dir
	}
	e.pkgPaths[dir] = p
	e.dirOfPath[p] = dir
	return p
}

// importable は、別の名前空間の型 fqn を、それを持つ出力のパッケージから import して使えるか（docs/emit-go.md「配置」）:
// その名前空間を出力する Go の単位のどれかの出力がその型を持つ（[Options.Exported] にディレクトリがある）。そうでなければ複製する。
func (e *emitter) importable(fqn string) bool {
	ns, known := e.declNS[fqn]
	return known && ns != e.viewer && e.opts.Exported[fqn] != ""
}

// resolve は plugin/golang の Resolver（docs/packages.md「`plugin/golang`（公開）」）。
// viewer の宣言と複製した宣言はローカル、import できる型（importable）はそれを持つ出力のパッケージ、それ以外（知らない FQN）はローカル。
func (e *emitter) resolve(fqn string) (namespace, pkgPath, pkgName string, local bool) {
	ns, known := e.declNS[fqn]
	if !known {
		return "", "", "", true
	}
	if ns == e.viewer || !e.importable(fqn) {
		return ns, "", "", true
	}
	return ns, e.importPathOf(e.opts.Exported[fqn]), gol.PackageName(ns), false
}

// isLocal は FQN の型が viewer のパッケージにあるか（宣言か複製）。
func (e *emitter) isLocal(fqn string) bool {
	_, _, _, local := e.resolve(fqn)
	return local
}
