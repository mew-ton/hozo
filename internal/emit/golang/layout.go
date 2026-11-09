package golang

import (
	"path"
	"path/filepath"
	"strings"

	gol "github.com/mew-ton/hozo/plugin/golang"
)

// subPath は名前空間 ns のファイルを Conf(ns).out の下のどこに置くかの相対パス P
// （docs/emit-go.md「配置」）。out を自分で書いた祖先 A からの相対パスだが、Conf からは
// 「自分で書いた」かどうかが分からないので、LanguageOf が同じ out を答える最も遠い祖先を A とみなす
// （doc.go「文書にない決定」）。LanguageOf が nil なら ""。
func (e *emitter) subPath(ns, out string) string {
	if e.opts.LanguageOf == nil || ns == "" {
		return ""
	}
	top := ns
	for cur := parentNS(ns); cur != ""; cur = parentNS(cur) {
		_, o, ok := e.opts.LanguageOf(cur)
		if !ok || o != out {
			break
		}
		top = cur
	}
	if top == ns {
		return ""
	}
	rest := strings.TrimPrefix(ns[len(top):], ".")
	return strings.ReplaceAll(rest, ".", "/")
}

// isGoNS は名前空間 ns が Go で同じプロジェクトか（import する相手か）。viewer 自身には使わない。
func (e *emitter) isGoNS(ns string) bool {
	if e.opts.LanguageOf == nil || ns == "" {
		return false
	}
	lang, _, ok := e.opts.LanguageOf(ns)
	return ok && lang == "go"
}

// importPathOf は Go の名前空間 ns のパッケージの import パス。go.mod がなければ GOEMIT-1（一度だけ）。
func (e *emitter) importPathOf(ns string) string {
	if p, ok := e.pkgPaths[ns]; ok {
		return p
	}
	_, out, _ := e.opts.LanguageOf(ns)
	dir := path.Join(filepath.ToSlash(out), e.subPath(ns, out))
	p, err := gol.ImportPath(e.opts.Root, dir)
	if err != nil {
		e.errorf(CodeNoGoMod, nil, "名前空間 %s の out %q の上に go.mod がない: %v", ns, out, err)
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
