package check

import (
	"fmt"
	"strings"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/ir"
)

// Locator は IR の Span（ファイルのパスと行・列。docs/ir.md「Program」）を
// 診断の Span（バイトのオフセット。docs/diagnostics.md「位置」）に直す。
// 直せなければ false を返す。
type Locator func(ir.Span) (source.Span, bool)

// FileSetLocator は fs の中身から行・列をオフセットに戻す Locator（戻すのは source.FileSet.Offset）。
// Span.File と同じパスのファイルが fs になければ false。
func FileSetLocator(fs *source.FileSet) Locator {
	ids := map[string]source.FileID{}
	for i := 0; i < fs.Len(); i++ {
		id := source.FileID(i)
		if p := fs.Path(id); p != "" {
			if _, ok := ids[p]; !ok {
				ids[p] = id
			}
		}
	}
	return func(s ir.Span) (source.Span, bool) {
		id, ok := ids[s.File]
		if !ok {
			return source.Span{}, false
		}
		start := fs.Offset(id, source.Pos{Line: s.Start.Line, Col: s.Start.Col})
		end := fs.Offset(id, source.Pos{Line: s.End.Line, Col: s.End.Col})
		if end < start {
			end = start
		}
		return source.Span{File: id, Start: start, End: end}, true
	}
}

// Checker は検査の設定。零値は位置を直さず（Span は nil で、File に IR のファイルを置く）に検査する。
type Checker struct {
	// Locate は IR の Span を診断の Span にする。nil なら診断は Span を持たず、File だけを持つ。
	Locate Locator
}

// Check は p を docs/check.md「診断」の規則で検査する（Checker の零値）。
// 計算（docs/check.md「計算」）は Check の中で行うので、先に Compute を呼ばなくてよい。
// 返す診断は IR の順で、並べ替え（diag.Sort）は呼ぶ側が行う。
func Check(p *ir.Program, cfg *config.Config) []diag.Diagnostic {
	return Checker{}.Check(p, cfg)
}

// CheckUnsupported は CFG-6（docs/config.md「診断」）。名前空間 ns の宣言（フィールドを含む）のうち単位の対象のもの
// （Span.File が files にある。files が nil ならすべて）に付いたデコレータのうち、FQN が unsupported にあるものを
// warning にする。files は単位の in に合ったファイル、unsupported は単位のトランスパイラのマニフェストの
// unsupported を、呼ぶ側が集めて渡す（docs/plugin-protocol.md「マニフェスト」）。
func CheckUnsupported(p *ir.Program, ns string, files []string, unsupported []string) []diag.Diagnostic {
	return Checker{}.CheckUnsupported(p, ns, files, unsupported)
}

// Check は p を検査する。Check（パッケージ関数）と同じで、位置は c.Locate で直す。
func (c Checker) Check(p *ir.Program, cfg *config.Config) []diag.Diagnostic {
	if p == nil {
		return nil
	}
	if cfg == nil {
		cfg = config.Default("")
	}
	ix := newIndex(p)
	r := &reporter{loc: c.Locate}
	comp := newComputer(ix, r)
	comp.computeAll()
	checkNamespaces(comp, r)
	checkConfig(ix, cfg, r)
	checkStates(ix, comp, r)
	checkThrows(ix, comp, r)
	checkDecorators(ix, comp, r)
	checkWiring(ix, cfg, r)
	checkDisjoint(ix, comp, r)
	checkBinds(ix, comp, r)
	return r.diags
}

// CheckUnsupported は CFG-6。CheckUnsupported（パッケージ関数）と同じで、位置は c.Locate で直す。
func (c Checker) CheckUnsupported(p *ir.Program, ns string, files []string, unsupported []string) []diag.Diagnostic {
	if p == nil {
		return nil
	}
	r := &reporter{loc: c.Locate}
	checkUnsupported(newIndex(p), ns, files, unsupported, r)
	return r.diags
}

// reporter は診断を集める。位置は Locator で直し、直せなければ File に IR のファイルを置く。
type reporter struct {
	loc   Locator
	diags []diag.Diagnostic
}

// related は Locator で直す前の関連する位置。
type related struct {
	span ir.Span
	msg  string
}

func (r *reporter) locate(s ir.Span) *source.Span {
	if r.loc == nil {
		return nil
	}
	ss, ok := r.loc(s)
	if !ok {
		return nil
	}
	return &ss
}

// report は IR の位置 span を持つ診断を足す。
func (r *reporter) report(sev diag.Severity, code string, span ir.Span, msg string, rel []related, data map[string]any) {
	d := diag.Diagnostic{Severity: sev, Code: code, Message: msg, Data: data}
	if s := r.locate(span); s != nil {
		d.Span = s
	} else {
		d.File = span.File
	}
	for _, x := range rel {
		if s := r.locate(x.span); s != nil {
			d.Related = append(d.Related, diag.Related{Span: *s, Message: x.msg})
		}
	}
	r.diags = append(r.diags, d)
}

// reportFile は位置のない診断（hozo.toml のもの）を足す。
func (r *reporter) reportFile(sev diag.Severity, code, file, msg string) {
	r.diags = append(r.diags, diag.Diagnostic{Severity: sev, Code: code, Message: msg, File: file})
}

func (r *reporter) errorAt(code string, span ir.Span, msg string) {
	r.report(diag.Error, code, span, msg, nil, nil)
}

func (r *reporter) warnAt(code string, span ir.Span, msg string) {
	r.report(diag.Warning, code, span, msg, nil, nil)
}

// index は Program を引くための表。宣言はすべて FQN で引く。
type index struct {
	p     *ir.Program
	decls map[string]ir.Decl
	nsOf  map[string]string // 宣言の FQN → 名前空間の FQN
	ops   []*ir.Op          // 出現順
	opsNS []string          // ops と同じ並びで、その名前空間
}

func newIndex(p *ir.Program) *index {
	ix := &index{p: p, decls: map[string]ir.Decl{}, nsOf: map[string]string{}}
	if p == nil {
		return ix
	}
	for i := range p.Namespaces {
		ns := &p.Namespaces[i]
		for _, d := range ns.Decls {
			if d == nil {
				continue
			}
			name := d.Common().Name
			if _, dup := ix.decls[name]; !dup {
				ix.decls[name] = d
				ix.nsOf[name] = ns.Name
			}
			if o, ok := d.(*ir.Op); ok {
				ix.ops = append(ix.ops, o)
				ix.opsNS = append(ix.opsNS, ns.Name)
			}
		}
	}
	return ix
}

// op は FQN の op。op でなければ nil。
func (ix *index) op(fqn string) *ir.Op {
	o, _ := ix.decls[fqn].(*ir.Op)
	return o
}

// decorator は FQN のデコレータ。別名なら目的の汎用デコレータまで辿り、別名の引数を返す。
func (ix *index) decorator(fqn string) (*ir.Decorator, []ir.Arg) {
	var args []ir.Arg
	for depth := 0; depth < 16; depth++ {
		d, ok := ix.decls[fqn].(*ir.Decorator)
		if !ok {
			return nil, nil
		}
		if d.Alias == nil {
			return d, args
		}
		if args == nil {
			args = d.Alias.Args
		}
		fqn = d.Alias.Target
	}
	return nil, nil
}

func (ix *index) model(fqn string) *ir.Model {
	m, _ := ix.decls[fqn].(*ir.Model)
	return m
}

func (ix *index) state(fqn string) *ir.State {
	s, _ := ix.decls[fqn].(*ir.State)
	return s
}

func (ix *index) effect(fqn string) *ir.Effect {
	e, _ := ix.decls[fqn].(*ir.Effect)
	return e
}

// readStates は reads の要素ごとの状態（read の宣言ならその宛先、直に書いた状態ならその状態。docs/check.md「節に書く要素」）を
// 同じ順に並べ、重複を最初の出現に寄せたもの。宛先の決まらない read は飛ばす。
func (ix *index) readStates(refs []ir.ReadRef) []string {
	var out []string
	for _, r := range refs {
		state := ""
		switch x := r.(type) {
		case ir.DirectRead:
			state = x.State
		case ir.NamedRead:
			if d, ok := ix.decls[x.Read].(*ir.Read); ok {
				state = d.State
			}
		}
		if state != "" {
			out = appendUnique(out, state)
		}
	}
	return out
}

// namespaceOf は宣言 FQN の名前空間。知らない宣言なら FQN の最後のセグメントを落としたもの。
func (ix *index) namespaceOf(fqn string) string {
	if ns, ok := ix.nsOf[fqn]; ok {
		return ns
	}
	return parentOf(fqn)
}

// parentOf は FQN の親。"server.admin" → "server"、"server" → ""。
func parentOf(fqn string) string {
	i := strings.LastIndexByte(fqn, '.')
	if i < 0 {
		return ""
	}
	return fqn[:i]
}

// shortName は FQN の最後のセグメント。
func shortName(fqn string) string {
	i := strings.LastIndexByte(fqn, '.')
	if i < 0 {
		return fqn
	}
	return fqn[i+1:]
}

// isDescendant は ns が root と等しいか、root の子孫か（`**` の意味。§5.3）。
func isDescendant(ns, root string) bool {
	return ns == root || strings.HasPrefix(ns, root+".")
}

// modelOf は型が model への ref（または model の optional）なら、その model。
func (ix *index) modelOf(t ir.Type) *ir.Model {
	if o, ok := t.(ir.OptionalType); ok {
		t = o.Of
	}
	if r, ok := t.(ir.RefType); ok {
		return ix.model(r.Target)
	}
	return nil
}

func isOptional(t ir.Type) bool {
	_, ok := t.(ir.OptionalType)
	return ok
}

// joinNames は FQN の列を「、」でつなぐ（メッセージ用）。
func joinNames(names []string) string {
	return strings.Join(names, "、")
}

// joinPath はパスを "in.token" の形にする（メッセージ用）。
func joinPath(path []string) string {
	return strings.Join(path, ".")
}

// contains は列に s があるか。
func contains[T comparable](list []T, s T) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// appendUnique は列に s がなければ末尾に足す。
func appendUnique[T comparable](list []T, s T) []T {
	if contains(list, s) {
		return list
	}
	return append(list, s)
}

// pathEqual はパスが等しいか。
func pathEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hasPath は BoundPosition の列に path があるか。
func hasPath(bound []ir.BoundPosition, path []string) bool {
	for _, b := range bound {
		if pathEqual(b.Path, path) {
			return true
		}
	}
	return false
}

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
