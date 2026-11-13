package check

import (
	"strings"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/ir"
)

// Unit は、設定の外で決まる単位の情報（docs/config.md「単位」「in」）。cfg.Units と同じ順に並べて Views に渡す。
type Unit struct {
	// Language は単位の言語（そのトランスパイラの language。マニフェストから）。
	Language string
	// Files は単位の in に合ったファイル（Span.File と同じ形、辞書順）。in を省いた単位なら読み込んだユーザーのファイルのすべて。
	Files []string
}

// Views は、単位ごと（cfg.Units と同じ順）の、その単位が出力する名前空間 → View
// （docs/ir.md「その名前空間から見た IR」）。単位が出力する名前空間は、単位の対象の名前空間（docs/config.md「単位の対象」）の
// うち、Files のファイルに宣言を一つでも持つもの（同「in」）。
// 計算（Compute）を行ってから作る。program は全体そのままに、params が合成する model を見る側の
// Namespace.decls の末尾に足したもの。ops はすべての op について Sig(O, viewer) と、bind で埋まる位置を取り除いた引数。
// units が cfg.Units より短ければ、足りない単位は Language が空で Files は nil（どの名前空間も出力しない）。
func Views(p *ir.Program, cfg *config.Config, units []Unit) []map[string]*ir.View {
	if cfg == nil {
		cfg = config.Default("")
	}
	views := make([]map[string]*ir.View, len(cfg.Units))
	for i := range views {
		views[i] = map[string]*ir.View{}
	}
	if p == nil {
		return views
	}
	unit := func(i int) Unit {
		if i < len(units) {
			return units[i]
		}
		return Unit{}
	}
	// 単位 i が出力する名前空間（Program の順）と、その設定
	outputs := make([][]string, len(cfg.Units))
	namespaces := map[string][]ir.NamespaceConfig{}
	confs := make([]map[string]ir.NamespaceConfig, len(cfg.Units))
	for i := range confs {
		confs[i] = map[string]ir.NamespaceConfig{}
	}
	for ni := range p.Namespaces {
		ns := &p.Namespaces[ni]
		if ns.Name == "" {
			continue
		}
		for _, i := range cfg.Covering(ns.Name) {
			u := unit(i)
			if !declaredIn(ns, u.Files) {
				continue
			}
			conf := ir.NamespaceConfig{
				Language:    u.Language,
				Out:         cfg.Units[i].Out,
				Dir:         cfg.Dir(i, ns.Name),
				Transpilers: append([]string{}, cfg.Units[i].Transpilers...),
			}
			outputs[i] = append(outputs[i], ns.Name)
			confs[i][ns.Name] = conf
			namespaces[ns.Name] = append(namespaces[ns.Name], conf)
		}
	}
	ix := newIndex(p)
	c := newComputer(ix, &reporter{})
	c.computeAll()
	base := map[string]*ir.View{} // viewer → 単位によらない部分
	for i := range cfg.Units {
		for _, viewer := range outputs[i] {
			b := base[viewer]
			if b == nil {
				b = c.view(viewer)
				base[viewer] = b
			}
			v := *b
			v.Config = confs[i][viewer]
			v.Files = append([]string{}, unit(i).Files...)
			v.Namespaces = make(map[string][]ir.NamespaceConfig, len(namespaces))
			for k, list := range namespaces {
				v.Namespaces[k] = append([]ir.NamespaceConfig(nil), list...)
			}
			views[i][viewer] = &v
		}
	}
	return views
}

// declaredIn は、名前空間 ns が files のファイルに宣言を一つでも持つか。
func declaredIn(ns *ir.Namespace, files []string) bool {
	for _, d := range ns.Decls {
		if d != nil && contains(files, d.Common().Span.File) {
			return true
		}
	}
	return false
}

// view は viewer から見た View のうち、単位によらない部分（version、viewer、program、ops）を作る。
func (c *computer) view(viewer string) *ir.View {
	ix := c.ix
	v := &ir.View{
		Version: ir.Version,
		Viewer:  viewer,
		Ops:     map[string]*ir.ViewedSignature{},
	}
	var synthesized []ir.Decl
	for _, o := range ix.ops {
		vs := &ir.ViewedSignature{Signature: c.sig(o, viewer).signature()}
		for bi, b := range ix.p.Binds {
			if b.Namespace != viewer {
				continue
			}
			for _, pos := range c.binds.positions(bi, o) {
				if !hasPath(vs.Bound, pos.path) {
					vs.Bound = append(vs.Bound, ir.BoundPosition{Path: append([]string(nil), pos.path...), State: b.State})
				}
			}
		}
		vs.Params, synthesized = c.viewedParams(viewer, o, vs.Bound, synthesized)
		v.Ops[o.Name] = vs
	}
	prog := *ix.p
	prog.Namespaces = append([]ir.Namespace(nil), ix.p.Namespaces...)
	if ns := prog.Namespace(viewer); ns != nil && len(synthesized) > 0 {
		ns.Decls = append(append([]ir.Decl(nil), ns.Decls...), synthesized...)
	}
	v.Program = &prog
	return v
}

// viewedParams は bind で埋まる位置を取り除いた引数。[p] が埋まる引数は除く。model のフィールドが埋まる引数は、
// そのフィールドを除いた model を `<viewer>.<op 名>$<パスを $ でつないだもの>`（synthesizedFrom は op の FQN）で合成し、ref にする。
func (c *computer) viewedParams(viewer string, o *ir.Op, bound []ir.BoundPosition, synthesized []ir.Decl) ([]ir.OpParam, []ir.Decl) {
	params := make([]ir.OpParam, 0, len(o.Params))
	for _, p := range o.Params {
		path := []string{p.Name}
		if hasPath(bound, path) {
			continue
		}
		if !hasPathBelow(bound, path) {
			params = append(params, p)
			continue
		}
		q := p
		q.Type, synthesized = c.synthesize(viewer, o, path, p.Type, p.Span, bound, synthesized, map[string]bool{})
		params = append(params, q)
	}
	return params, synthesized
}

// synthesize は型 typ（model への ref か、その optional）から bound の位置を除いた model を合成し、その ref を返す。
// 位置が下にないか、model でなければ typ のまま。
func (c *computer) synthesize(viewer string, o *ir.Op, path []string, typ ir.Type, span ir.Span, bound []ir.BoundPosition, synthesized []ir.Decl, visiting map[string]bool) (ir.Type, []ir.Decl) {
	m := c.ix.modelOf(typ)
	if m == nil || visiting[m.Name] || !hasPathBelow(bound, path) {
		return typ, synthesized
	}
	visiting[m.Name] = true
	defer delete(visiting, m.Name)
	name := viewer + "." + shortName(o.Name) + "$" + strings.Join(path, "$")
	synth := &ir.Model{
		DeclBase:        ir.DeclBase{Name: name, Span: span},
		SynthesizedFrom: o.Name,
		Fields:          []ir.Field{},
	}
	synthesized = append(synthesized, synth) // 親を先に、入れ子の model をその後に並べる
	for _, f := range m.Fields {
		fpath := append(append([]string(nil), path...), f.Name)
		if hasPath(bound, fpath) {
			continue
		}
		g := f
		g.Type, synthesized = c.synthesize(viewer, o, fpath, f.Type, f.Span, bound, synthesized, visiting)
		synth.Fields = append(synth.Fields, g)
	}
	var out ir.Type = ir.Ref(name)
	if isOptional(typ) {
		out = ir.Optional(out)
	}
	return out, synthesized
}

// hasPathBelow は path より下（真に長い）の位置が bound にあるか。
func hasPathBelow(bound []ir.BoundPosition, path []string) bool {
	for _, b := range bound {
		if len(b.Path) > len(path) && pathEqual(b.Path[:len(path)], path) {
			return true
		}
	}
	return false
}
