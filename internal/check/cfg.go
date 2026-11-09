package check

import (
	"strings"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// checkConfig は設定に関わる検査のうちプログラムが要るもの（docs/config.md「診断」）: CFG-1 / CFG-2（.hozo 側）/
// CFG-8 / CFG-10。CFG-6 は CheckUnsupported。CFG-4 / CFG-5 / CFG-7 / CFG-9 / CFG-11 は設定とプラグインの側で出る。
func checkConfig(ix *index, cfg *config.Config, r *reporter) {
	file := cfg.Path
	if file == "" {
		file = config.FileName
	}
	for i := range ix.p.Namespaces {
		ns := &ix.p.Namespaces[i]
		checkCfg2(ns, r)
		checkCfg1(ns, cfg, file, r)
	}
	for _, name := range cfg.NamespaceNames() {
		checkCfg8And10(ix, name, file, r)
	}
}

// checkCfg1 は CFG-1: state か op を一つでも持つユーザーの名前空間で、Conf の language か out が未設定。
// グローバル（""）に state か op があれば、それも CFG-1（グローバルには設定を書けない）。位置は hozo.toml。
func checkCfg1(ns *ir.Namespace, cfg *config.Config, file string, r *reporter) {
	if isVocab(ns) || !hasStateOrOp(ns) {
		return
	}
	if ns.Name == "" {
		r.reportFile(diag.Error, "CFG-1", file, "グローバルに state か op があるが、グローバルには language と out を設定できない")
		return
	}
	conf, _ := cfg.Resolve(ns.Name)
	var missing []string
	if conf.Language == "" {
		missing = append(missing, "language")
	}
	if conf.Out == "" {
		missing = append(missing, "out")
	}
	if len(missing) > 0 {
		r.reportFile(diag.Error, "CFG-1", file, sprintf("名前空間 %s に %s がない", ns.Name, strings.Join(missing, " と ")))
	}
}

// checkCfg2 は CFG-2（.hozo 側）: 名前空間の名前（どの階層でも）が language / plugins / out / hozo。
// 位置は名前空間の最初の宣言（IR は名前空間の名前の位置を持たない）。
func checkCfg2(ns *ir.Namespace, r *reporter) {
	if ns.Name == "" {
		return
	}
	for _, seg := range strings.Split(ns.Name, ".") {
		if !config.IsReservedNamespaceName(seg) {
			continue
		}
		msg := sprintf("名前空間 %s の名前に %s は使えない", ns.Name, seg)
		if len(ns.Decls) > 0 {
			r.errorAt("CFG-2", ns.Decls[0].Common().Span, msg)
		} else {
			r.reportFile(diag.Error, "CFG-2", "", msg)
		}
		return
	}
}

// checkCfg8And10 は hozo.toml のテーブル name について、CFG-10（その名前空間が .hozo にない。子孫もない）と
// CFG-8（語彙の名前空間、または自分にも子孫にも state も op もない名前空間にテーブルがある）。
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
		r.reportFile(diag.Error, "CFG-10", file, sprintf("hozo.toml に名前空間 %s のテーブルがあるが、その名前空間は .hozo にない", name))
	case exact != nil && isVocab(exact):
		r.reportFile(diag.Error, "CFG-8", file, sprintf("%s は語彙の名前空間なので、hozo.toml に設定を書けない", name))
	case !active:
		r.reportFile(diag.Error, "CFG-8", file, sprintf("名前空間 %s は state も op も持たないので、hozo.toml に設定を書けない", name))
	}
}

// checkUnsupported は CFG-6: Conf(ns).plugins のプラグインのマニフェストの unsupported にあるデコレータの FQN が、
// ns の宣言（フィールドを含む）に付いている。付いた位置に warning。
func checkUnsupported(ix *index, nsName string, unsupported []string, r *reporter) {
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
			r.warnAt("CFG-6", app.Span, sprintf("@%s は名前空間 %s のプラグインが対応していない（%s）", shortName(name), nsName, target))
		}
	}
	for _, d := range ns.Decls {
		if d == nil {
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
