package check

import (
	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// checkConfig は設定に関わる検査のうちプログラムが要るもの（docs/config.md「診断」）: CFG-8 / CFG-10。
// CFG-6 は CheckUnsupported。CFG-7 / CFG-12 / CFG-17 は internal/config、CFG-11 / CFG-14〜CFG-16 / CFG-18 は
// マニフェストと読んだファイルが要るので internal/project、CFG-5 は書き出すとき（internal/app）に出る。
func checkConfig(ix *index, cfg *config.Config, r *reporter) {
	file := cfg.Path
	if file == "" {
		file = config.FileName
	}
	for _, name := range cfg.UnitNamespaces() {
		checkCfg8And10(ix, name, file, r)
	}
}

// checkCfg8And10 は単位の namespace name について、CFG-10（その名前空間が .hozo にない。子孫もない）と
// CFG-8（語彙の名前空間、または自分にも子孫にも state も op もない名前空間を単位の namespace に書いた）。
func checkCfg8And10(ix *index, name, file string, r *reporter) {
	var exact *ir.Namespace
	exists := false
	active := false
	for i := range ix.p.Namespaces {
		ns := &ix.p.Namespaces[i]
		if ns.Name == "" || !isDescendant(ns.Name, name) {
			continue
		}
		exists = true
		if ns.Name == name {
			exact = ns
		}
		if !isVocab(ns) && hasStateOrOp(ns) {
			active = true
		}
	}
	switch {
	case !exists:
		r.reportFile(diag.Error, "CFG-10", file, sprintf("hozo.toml の単位の namespace に %s があるが、その名前空間は .hozo にない", name))
	case exact != nil && isVocab(exact):
		r.reportFile(diag.Error, "CFG-8", file, sprintf("%s は語彙の名前空間なので、単位の namespace に書けない", name))
	case !active:
		r.reportFile(diag.Error, "CFG-8", file, sprintf("名前空間 %s は state も op も持たないので、単位の namespace に書けない", name))
	}
}

// checkUnsupported は CFG-6: 単位のトランスパイラのマニフェストの unsupported にあるデコレータの FQN が、名前空間 nsName の
// 宣言（フィールドを含む）のうち単位の対象のもの（Span.File が files にある。docs/config.md「in」）に付いている。
// 付いた位置に warning。files が nil なら、名前空間のすべての宣言を見る。
func checkUnsupported(ix *index, nsName string, files []string, unsupported []string, r *reporter) {
	ns := ix.p.Namespace(nsName)
	if ns == nil || len(unsupported) == 0 {
		return
	}
	report := func(apps []ir.Application, target string) {
		for ai := range apps {
			app := &apps[ai]
			name := app.Decorator
			if !contains(unsupported, name) {
				if app.Via == "" || !contains(unsupported, app.Via) {
					continue
				}
				name = app.Via
			}
			r.warnAt("CFG-6", app.Span, sprintf("@%s は名前空間 %s の単位のトランスパイラが対応していない（%s）", shortName(name), nsName, target))
		}
	}
	for _, d := range ns.Decls {
		if d == nil || files != nil && !contains(files, d.Common().Span.File) {
			continue
		}
		report(d.Common().Decorators, d.Common().Name)
		if m, ok := d.(*ir.Model); ok {
			for _, f := range m.Fields {
				report(f.Decorators, m.Name+"."+f.Name)
			}
		}
	}
}
