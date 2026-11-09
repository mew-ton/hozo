package golang

import (
	"path"
	"path/filepath"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// dirOf は名前空間の設定 c の出力ディレクトリ Dir(N)（docs/emit-go.md「配置」）をルートからの `/` 区切りで。
// Dir が空なら Conf(N).out（P が空の Dir(N) と同じ。doc.go「文書にない決定」）。
func dirOf(c ir.NamespaceConfig) string {
	d := c.Dir
	if d == "" {
		d = c.Out
	}
	return path.Clean(filepath.ToSlash(d))
}

// isGoNS は名前空間 ns が Go で同じプロジェクトか（import する相手か。docs/emit-go.md「配置」）。
// View.namespaces にあって language が go なら真。viewer 自身には使わない。
func (e *emitter) isGoNS(ns string) bool {
	if ns == "" {
		return false
	}
	c, ok := e.view.Namespaces[ns]
	return ok && c.Language == "go"
}

// importPathOf は Go の名前空間 ns のパッケージ（Dir(ns)）の import パス。go.mod がなければ GOEMIT-1（一度だけ）。
func (e *emitter) importPathOf(ns string) string {
	if p, ok := e.pkgPaths[ns]; ok {
		return p
	}
	c := e.view.Namespaces[ns]
	dir := dirOf(c)
	p, err := gol.ImportPath(e.opts.Root, dir)
	if err != nil {
		e.errorf(CodeNoGoMod, nil, "名前空間 %s の out %q の上に go.mod がない: %v", ns, c.Out, err)
		p = "INVALID/" + dir
	}
	e.pkgPaths[ns] = p
	return p
}

// resolve は plugin/golang の Resolver（docs/packages.md「`plugin/golang`（公開）」）。
// viewer の宣言と複製した宣言はローカル、Go の名前空間は import、それ以外（知らない FQN）はローカル。
func (e *emitter) resolve(fqn string) (namespace, pkgPath, pkgName string, local bool) {
	ns, known := e.declNS[fqn]
	if !known {
		return "", "", "", true
	}
	if ns == e.viewer || e.copied[fqn] || !e.isGoNS(ns) {
		return ns, "", "", true
	}
	return ns, e.importPathOf(ns), gol.PackageName(ns), false
}

// isLocal は FQN の型が viewer のパッケージにあるか（宣言か複製）。
func (e *emitter) isLocal(fqn string) bool {
	_, _, _, local := e.resolve(fqn)
	return local
}
