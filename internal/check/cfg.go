package check

import (
	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// checkConfig は設定に関わる検査のうち、プログラムだけが要るもの（docs/config.md「診断」）: CFG-10。
// CFG-6 は CheckUnsupported、CFG-22 は CheckHomes。CFG-7 / CFG-12 / CFG-17 / CFG-21 は internal/config、
// CFG-14〜CFG-16 / CFG-18〜CFG-20 / CFG-23 はマニフェストと読んだファイルが要るので internal/project、
// CFG-5 は書き出すとき（internal/app）に出る（docs/packages.md「internal/config」の表）。
func checkConfig(ix *index, cfg *config.Config, r *reporter) {
	file := cfg.Path
	if file == "" {
		file = config.FileName
	}
	for _, name := range cfg.UnitNamespaces() {
		checkCfg10(ix, name, file, r)
	}
}

// checkCfg10 は CFG-10: 単位の namespace name の名前空間も、その子孫も .hozo にない（語彙のファイルにもない）。
// 宣言を持つどの名前空間も、単位の namespace に書ける（型だけの名前空間も、語彙の名前空間も。CFG-8 は欠番。
// docs/config.md「単位の対象」）。
func checkCfg10(ix *index, name, file string, r *reporter) {
	for i := range ix.p.Namespaces {
		ns := &ix.p.Namespaces[i]
		if ns.Name != "" && isDescendant(ns.Name, name) {
			return
		}
	}
	r.reportFile(diag.Error, "CFG-10", file, sprintf("hozo.toml の単位の namespace に %s があるが、その名前空間は .hozo にない", name))
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
