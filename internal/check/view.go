package check

import (
	"strings"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/ir"
)

// Views は設定のある名前空間ごとの View（docs/ir.md「その名前空間から見た IR」）。キーは見る側の名前空間の FQN。
// 計算（Compute）を行ってから作る。program は全体そのままに、params が合成する model を見る側の
// Namespace.decls の末尾に足したもの。ops はすべての op について Sig(O, viewer) と、bind で埋まる位置を取り除いた引数。
func Views(p *ir.Program, cfg *config.Config) map[string]*ir.View {
	if p == nil {
		return map[string]*ir.View{}
	}
	if cfg == nil {
		cfg = config.Default("")
	}
	ix := newIndex(p)
	c := newComputer(ix, &reporter{})
	c.computeAll()
	namespaces := namespaceConfigs(p, cfg)
	views := map[string]*ir.View{}
	for i := range p.Namespaces {
		viewer := p.Namespaces[i].Name
		conf, ok := namespaces[viewer]
		if !ok {
			continue
		}
		views[viewer] = c.view(viewer, conf, namespaces)
	}
	return views
}

// namespaceConfigs は設定のあるすべての名前空間の設定（docs/ir.md「その名前空間から見た IR」の View.namespaces）。
// hozo.toml にテーブルのある名前空間と、Program の名前空間のうち祖先のテーブルから設定を継ぐもの
// （View を持つ名前空間はすべてここに入る）。
func namespaceConfigs(p *ir.Program, cfg *config.Config) map[string]ir.NamespaceConfig {
	names := cfg.NamespaceNames()
	for i := range p.Namespaces {
		names = append(names, p.Namespaces[i].Name)
	}
	out := map[string]ir.NamespaceConfig{}
	for _, n := range names {
		if _, done := out[n]; done {
			continue
		}
		if nc, ok := namespaceConfig(cfg, n); ok {
			out[n] = nc
		}
	}
	return out
}

// namespaceConfig は名前空間 ns の Conf(ns) と Dir(ns)（docs/config.md「名前空間の設定の解決」）。設定がなければ false。
func namespaceConfig(cfg *config.Config, ns string) (ir.NamespaceConfig, bool) {
	conf, ok := cfg.Resolve(ns)
	if !ok {
		return ir.NamespaceConfig{}, false
	}
	dir, _ := cfg.Dir(ns)
	plugins := conf.Plugins
	if plugins == nil {
		plugins = []string{}
	}
	return ir.NamespaceConfig{Language: conf.Language, Out: conf.Out, Dir: dir, Plugins: plugins}, true
}

// view は viewer から見た View を作る。conf は viewer の設定、namespaces は設定のあるすべての名前空間の設定（複製して持つ）。
func (c *computer) view(viewer string, conf ir.NamespaceConfig, namespaces map[string]ir.NamespaceConfig) *ir.View {
	ix := c.ix
	nss := make(map[string]ir.NamespaceConfig, len(namespaces))
	for k, nc := range namespaces {
		nss[k] = nc
	}
	v := &ir.View{
		Version:    ir.Version,
		Viewer:     viewer,
		Config:     conf,
		Namespaces: nss,
		Ops:        map[string]*ir.ViewedSignature{},
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
