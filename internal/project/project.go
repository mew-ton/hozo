package project

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/internal/check"
	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/sema"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/internal/vocab"
	"github.com/mew-ton/hozo/ir"
	sdk "github.com/mew-ton/hozo/plugin"
)

// Project は一度の解析のすべて（docs/packages.md「internal/project」）。
type Project struct {
	// Root はプロジェクトのルート（絶対パス）。hozo.toml のあるディレクトリ（docs/config.md「置き場と探し方」）。
	// 設定がなければ Load に渡された root。
	Root string
	// Config は設定。設定なしなら既定値（config.Default）で、NoConfig が true。
	Config *config.Config
	// NoConfig は hozo.toml が見つからなかった（--config もなかった）こと。
	NoConfig bool
	// Files はすべてのファイル。ユーザーのファイルはルートからの相対パス（"/" 区切り）、語彙は "hozo:" のパス。
	Files *source.FileSet
	// Sources はユーザーのファイルのパス（Files と同じ形）。辞書順。
	Sources []string
	// Sema は解決・正規化の結果。Program は検査の計算（Signature、Call.Bound）も済んでいる。
	Sema *sema.Result
	// Diags は解析・解決・検査・設定・プラグインの check のすべて。並べ替え済み（docs/diagnostics.md「順序」）。
	Diags []diag.Diagnostic
	// Plugins は入っているプラグイン（docs/plugin-protocol.md「探索」。探した順）。設定なしなら空。
	Plugins []*hostplugin.Plugin
	// Units は解決した単位（Config.Units と同じ順）: 言語、基底、解決できたトランスパイラ、単位の継ぎ目、in に合ったファイル。
	Units []Unit
	// RunErr はプラグインの check が proc.Crashed / proc.MalformedOutput（internal/plugin の ErrCrashed /
	// ErrMalformedOutput）で終わったときのその誤り。最初の一つで止め、以降のプラグインは呼ばない。なければ nil。
	RunErr error
}

// Unit は解決した単位（docs/config.md「単位」）。設定の単位に、マニフェストと読んだファイルから決まるものを足したもの。
type Unit struct {
	*config.Unit
	// Language は単位の言語（単位の基底の language。docs/config.md「基底と相乗り」）。基底が決まらなければ、解決できた最初の
	// トランスパイラの language。一つも解決できなければ ""。
	Language string
	// Base は単位の基底のトランスパイラの名前（requires が空の、ただ一つのトランスパイラ）。決まらなければ（CFG-20。
	// CFG-14 / CFG-15 のある単位も）""。
	Base string
	// Transpilers は transpilers の名前のうち解決できたもの（書いた順）。
	Transpilers []Transpiler
	// Seams は単位の継ぎ目（docs/config.md「継ぎ目」）: 継ぎ目を持つデコレータの FQN → それをマニフェストの seams で宣言した
	// 単位のトランスパイラの名前。二つ以上が宣言していれば（CFG-23）、設定の順で最初のもの。なければ空（nil ではない）。
	Seams map[string]string
	// Files は in に合ったユーザーのファイル（Project.Sources の形、辞書順）と、その後に、読み込んだすべての語彙のファイル
	// （辞書順。docs/config.md「in」）。in を省いた単位なら、読み込んだすべてのファイル。
	Files []string
}

// Transpiler は単位が選んだトランスパイラ一つと、それを提供するプラグイン。
type Transpiler struct {
	Name   string
	Plugin *hostplugin.Plugin
	Decl   *hostplugin.Transpiler
}

// Options は Load の入力のうち、docs/packages.md の Load の引数にないもの。
type Options struct {
	// Root は hozo.toml を探し始めるディレクトリ。設定がなければプロジェクトのルートにもなる。空なら作業ディレクトリ。
	Root string
	// ConfigPath は --config / HOZO_CONFIG の hozo.toml。相対パスなら Root からの相対。空なら Root から親へ探す。
	ConfigPath string
	// Overlay はディスクの代わりに使う内容（言語サーバーの開いている文書）。キーはルートからの相対パスか絶対パス。
	// ディスクにないファイルも足せる。sources の glob に合うものだけを読む。
	Overlay map[string][]byte
	// Stderr はプラグインの標準エラーを流す先（docs/plugin-protocol.md「起動」）。nil なら os.Stderr。
	Stderr io.Writer
	// Env はプラグインに渡す環境変数の元。nil なら os.Environ()。
	Env []string
	// PluginPath は HOZO_PLUGIN_PATH の値（docs/plugin-protocol.md「探索」の 2）。空なら探さない。
	PluginPath string
	// Home はホームディレクトリ（同 3 の <ホーム>/.hozo/plugins）。空なら探さない。
	// このパッケージは PluginPath も Home もプロセスの環境から読まない。hozoc と言語サーバーが渡す（internal/app）。
	Home string
}

// Load は root から hozo.toml を探して（configPath があればそれを読んで）プロジェクトを解析する
// （docs/packages.md「internal/project」）。hozo.toml がなければ既定の設定で解析し、CFG-* の診断を除く。
// overlay の内容はディスクの内容より優先し、ディスクにないファイルも足せる。
// プラグインを探すのは [hozo].plugin_dirs だけ（HOZO_PLUGIN_PATH とホームは LoadWith の Options で渡す）。
func Load(root, configPath string, overlay map[string][]byte) *Project {
	return LoadWith(Options{Root: root, ConfigPath: configPath, Overlay: overlay})
}

// LoadWith は Load の本体。
func LoadWith(opts Options) *Project {
	p := &Project{Files: source.NewFileSet()}
	var diags []diag.Diagnostic

	// 1. 設定（docs/cli.md「check」の 1）
	start := opts.Root
	if start == "" {
		start = "."
	}
	if abs, err := filepath.Abs(start); err == nil {
		start = abs
	}
	cfgPath := opts.ConfigPath
	if cfgPath != "" && !filepath.IsAbs(cfgPath) {
		cfgPath = filepath.Join(start, cfgPath)
	}
	if cfgPath == "" {
		if found, ok := config.Find(start); ok {
			cfgPath = found
		}
	}
	if cfgPath == "" {
		p.NoConfig = true
		p.Config = config.Default(start)
	} else {
		var cds []diag.Diagnostic
		p.Config, cds = config.Load(cfgPath)
		diags = append(diags, cds...)
	}
	p.Root = p.Config.Root

	// 入っているプラグインのマニフェストと、単位のトランスパイラの解決（docs/plugin-protocol.md「探索」、docs/config.md「単位」）
	diags = append(diags, p.resolveUnits(hostplugin.SearchEnv{PluginPath: opts.PluginPath, Home: opts.Home})...)

	// 2. 読み込みと解析（docs/vocab.md「読み込み」、docs/grammar.md）
	var files []*ast.File
	add := func(path string, content []byte) {
		id := p.Files.Add(path, content)
		f, ds := syntax.Parse(p.Files, id)
		files = append(files, f)
		diags = append(diags, ds...)
	}
	srcs, sds := p.readSources(opts.Overlay)
	diags = append(diags, sds...)
	for _, s := range srcs {
		add(s.path, s.content)
	}
	vocab.AddTo(add)
	for _, pl := range p.usedPlugins() {
		if len(pl.Manifest.Vocabulary) == 0 {
			continue
		}
		if err := vocab.AddPluginTo(add, pl.Name, pl.Dir, pl.Manifest.Vocabulary); err != nil {
			diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "MAN-4", Message: err.Error(), File: pl.ManifestPath()})
		}
	}
	diags = append(diags, p.matchUnitFiles()...)

	// 3. 解決・正規化・検査（docs/resolve.md、docs/ir.md、docs/check.md）
	p.Sema = sema.Analyze(p.Files, files)
	diags = append(diags, p.Sema.Diags...)
	checker := check.Checker{Locate: check.FileSetLocator(p.Files)}
	diags = append(diags, checker.Check(p.Sema.Program, p.Config)...)
	diags = append(diags, p.checkPlugins(checker)...)
	if !p.NoConfig {
		// CFG-22（docs/config.md「型の置き場」）
		diags = append(diags, checker.CheckHomes(p.Sema.Program, p.Config, p.checkUnits())...)
	}

	// 4. プラグインの check（docs/cli.md「check」の 4）
	pds, err := p.runPluginChecks(opts)
	diags = append(diags, pds...)
	p.RunErr = err

	p.Diags = p.finish(diags)
	return p
}

// Errors は error の診断の数。
func (p *Project) Errors() int {
	n, _ := diag.Count(p.Diags)
	return n
}

// Views は、単位ごと（Units と同じ順）の、その単位が出力する名前空間 → View（docs/ir.md「その名前空間から見た IR」。
// internal/check.Views）。
func (p *Project) Views() []map[string]*ir.View {
	if p.Sema == nil {
		return make([]map[string]*ir.View, len(p.Units))
	}
	return check.Views(p.Sema.Program, p.Config, p.checkUnits())
}

// checkUnits は、解決した単位を internal/check に渡す形にする（言語、基底、継ぎ目、in に合ったファイル）。
func (p *Project) checkUnits() []check.Unit {
	units := make([]check.Unit, len(p.Units))
	for i := range p.Units {
		units[i] = check.Unit{Language: p.Units[i].Language, Base: p.Units[i].Base, Seams: p.Units[i].Seams, Files: p.Units[i].Files}
	}
	return units
}

// ManifestPath はプラグイン pl のマニフェストのパス（診断の File に使う形: ルートからの相対パス）。
func (p *Project) ManifestPath(pl *hostplugin.Plugin) string {
	return p.relPath(pl.ManifestPath())
}

// usedPlugins は使うプラグイン（docs/plugin-protocol.md「探索」: どれかの単位の transpilers に書かれたトランスパイラを
// 提供するプラグイン）を名前の辞書順で返す。
func (p *Project) usedPlugins() []*hostplugin.Plugin {
	seen := map[*hostplugin.Plugin]bool{}
	var out []*hostplugin.Plugin
	for i := range p.Units {
		for _, t := range p.Units[i].Transpilers {
			if !seen[t.Plugin] {
				seen[t.Plugin] = true
				out = append(out, t.Plugin)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// resolveUnits は入っているプラグインのマニフェストを探して読み（docs/plugin-protocol.md「探索」）、各単位の transpilers の
// 名前をトランスパイラに解決して p.Units を作る（docs/config.md「単位」「基底と相乗り」「継ぎ目」）。設定なしなら探索しない。
// 診断は MAN-3 / MAN-4（入っているプラグインのすべて）、MAN-2（使うプラグインだけ）、CFG-14（どのプラグインも提供して
// いない）、CFG-15（二つ以上のプラグインが提供している。以降は探した順で最初のものを使う）、CFG-16（一つの単位の
// トランスパイラの language がそろっていない）、CFG-19（requires の名前が単位の transpilers にない）、CFG-20（基底が
// ちょうど一つでない）、CFG-23（二つ以上のトランスパイラが同じデコレータの継ぎ目を宣言している）。
func (p *Project) resolveUnits(env hostplugin.SearchEnv) []diag.Diagnostic {
	var diags []diag.Diagnostic
	if p.NoConfig {
		return nil
	}
	p.Plugins, diags = hostplugin.Discover(p.Config, env)
	cfgFile := p.Config.Path
	if cfgFile == "" {
		cfgFile = config.FileName
	}
	errorf := func(code, format string, args ...any) {
		diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: code, File: cfgFile, Message: fmt.Sprintf(format, args...)})
	}
	p.Units = make([]Unit, len(p.Config.Units))
	for i := range p.Config.Units {
		cu := &p.Config.Units[i]
		u := Unit{Unit: cu, Seams: map[string]string{}}
		// certain は、名前がただ一つのプラグインに解決できたトランスパイラ（CFG-14 でも CFG-15 でもない）。
		// requires と seams を見るのは、これだけ（docs/config.md「診断」の CFG-19 / CFG-23）。
		var certain []Transpiler
		unresolved := false
		for _, name := range cu.Transpilers {
			var providers []*hostplugin.Plugin
			for _, pl := range p.Plugins {
				if pl.Manifest.Transpiler(name) != nil {
					providers = append(providers, pl)
				}
			}
			switch {
			case len(providers) == 0:
				// CFG-14
				errorf("CFG-14", "%sの transpilers の %s を提供するプラグインが入っていない（plugin_dirs、%s、~/.hozo/plugins を探した）",
					cu.Label(), name, hostplugin.PluginPathEnv)
				unresolved = true
				continue
			case len(providers) > 1:
				// CFG-15
				var where []string
				for _, pl := range providers {
					where = append(where, fmt.Sprintf("%s（%s）", pl.Name, p.ManifestPath(pl)))
				}
				errorf("CFG-15", "%sの transpilers の %s を、二つ以上のプラグインが提供している: %s", cu.Label(), name, strings.Join(where, "、"))
				unresolved = true
			}
			t := Transpiler{Name: name, Plugin: providers[0], Decl: providers[0].Manifest.Transpiler(name)}
			u.Transpilers = append(u.Transpilers, t)
			if len(providers) == 1 {
				certain = append(certain, t)
			}
		}
		if len(u.Transpilers) > 0 {
			u.Language = u.Transpilers[0].Decl.Language
			mixed := false
			var langs []string
			for _, t := range u.Transpilers {
				langs = append(langs, fmt.Sprintf("%s は %q", t.Name, t.Decl.Language))
				if t.Decl.Language != u.Language {
					mixed = true
				}
			}
			if mixed {
				// CFG-16
				errorf("CFG-16", "%sのトランスパイラの language がそろっていない: %s", cu.Label(), strings.Join(langs, "、"))
			}
		}
		// CFG-19: requires の名前が、単位の transpilers にない。足りない名前ごとに一つ（要るトランスパイラを並べる）
		listed := map[string]bool{}
		for _, name := range cu.Transpilers {
			listed[name] = true
		}
		var missing []string
		needs := map[string][]string{}
		for _, t := range certain {
			for _, r := range t.Decl.Requires {
				if listed[r] {
					continue
				}
				if _, seen := needs[r]; !seen {
					missing = append(missing, r)
				}
				needs[r] = append(needs[r], t.Name)
			}
		}
		for _, r := range missing {
			errorf("CFG-19", "%sの %s は %s を要るが、transpilers にない（依存は、すべて transpilers に書く）",
				cu.Label(), strings.Join(needs[r], "、"), r)
		}
		// CFG-20: 基底（requires が空）がちょうど一つでない。解決できなかった名前があれば出さない（それが基底かもしれない）
		if !unresolved {
			var bases []string
			for _, t := range certain {
				if t.Decl.IsBase() {
					bases = append(bases, t.Name)
				}
			}
			switch len(bases) {
			case 1:
				u.Base = bases[0]
				u.Language = p.transpilerOf(&u, u.Base).Decl.Language
			case 0:
				errorf("CFG-20", "%sの transpilers に、基底（マニフェストの requires が空のトランスパイラ）が一つもない", cu.Label())
			default:
				errorf("CFG-20", "%sの transpilers に、基底（マニフェストの requires が空のトランスパイラ）が二つ以上ある: %s",
					cu.Label(), strings.Join(bases, "、"))
			}
		}
		// 単位の継ぎ目と CFG-23: 二つ以上のトランスパイラが、同じデコレータを seams に書いている。単位とデコレータの組ごとに一つ
		var decorators []string
		declarers := map[string][]string{}
		for _, t := range certain {
			for _, fqn := range t.Decl.Seams {
				if _, seen := declarers[fqn]; !seen {
					decorators = append(decorators, fqn)
					u.Seams[fqn] = t.Name
				}
				declarers[fqn] = append(declarers[fqn], t.Name)
			}
		}
		for _, fqn := range decorators {
			if len(declarers[fqn]) > 1 {
				errorf("CFG-23", "%sの二つ以上のトランスパイラが、デコレータ %s の継ぎ目を宣言している: %s",
					cu.Label(), fqn, strings.Join(declarers[fqn], "、"))
			}
		}
		p.Units[i] = u
	}
	// MAN-2 は使うプラグインにだけ
	for _, pl := range p.usedPlugins() {
		diags = append(diags, pl.Mismatch...)
	}
	return diags
}

// transpilerOf は単位 u の解決できたトランスパイラのうち、名前が name のもの。
func (p *Project) transpilerOf(u *Unit, name string) *Transpiler {
	for i := range u.Transpilers {
		if u.Transpilers[i].Name == name {
			return &u.Transpilers[i]
		}
	}
	return nil
}

// matchUnitFiles は各単位の in を読み込んだユーザーのファイル（p.Sources）に照らして Files を決める（docs/config.md「in」）。
// in を省いた単位は Sources のすべて。語彙のファイル（"hozo:" のパス）は、in によらず、いつも合ったものとして後ろに足す。
// in の glob がユーザーのファイルのどれにも合わなければ CFG-18。
func (p *Project) matchUnitFiles() []diag.Diagnostic {
	var diags []diag.Diagnostic
	var vocabFiles []string
	for i := 0; i < p.Files.Len(); i++ {
		if path := p.Files.Path(source.FileID(i)); vocab.IsVocab(path) {
			vocabFiles = append(vocabFiles, path)
		}
	}
	sort.Strings(vocabFiles)
	for i := range p.Units {
		u := &p.Units[i]
		u.Files = []string{}
		if u.In == nil {
			u.Files = append(u.Files, p.Sources...)
		} else {
			for _, src := range p.Sources {
				if matchesAny(u.In, nil, src) {
					u.Files = append(u.Files, src)
				}
			}
			if len(u.Files) == 0 {
				// CFG-18
				diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "CFG-18", File: p.Config.Path,
					Message: fmt.Sprintf("%sの in %s が、読み込んだファイルのどれにも合わない", u.Label(), quoteList(u.In))})
				continue // 何にも合わない単位は、語彙の名前空間も出力しない
			}
		}
		u.Files = append(u.Files, vocabFiles...)
	}
	return diags
}

// quoteList は文字列の列を ["a", "b"] の形にする。
func quoteList(list []string) string {
	quoted := make([]string, len(list))
	for i, s := range list {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// sourceFile は読んだユーザーのファイル。
type sourceFile struct {
	path    string
	content []byte
}

// readSources は [hozo].sources の glob に合うファイルを読む（docs/config.md「形」）。各単位の out の下と
// plugin_dirs の下は除く。overlay の内容はディスクより優先し、glob に合うなら足す。パスは辞書順。
func (p *Project) readSources(overlay map[string][]byte) ([]sourceFile, []diag.Diagnostic) {
	var diags []diag.Diagnostic
	ex := p.excluder()
	patterns := p.Config.Hozo.Sources
	paths, err := collectSources(p.Root, patterns, ex)
	if err != nil {
		diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "IO", Message: fmt.Sprintf("ソースを探せない: %v", err)})
	}
	byPath := map[string][]byte{}
	for _, rel := range paths {
		content, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(rel)))
		if err != nil {
			diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "IO", Message: fmt.Sprintf("読めない: %v", err), File: rel})
			continue
		}
		byPath[rel] = content
	}
	for key, content := range overlay {
		rel := p.relPath(key)
		if rel == "" || strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) || !matchesAny(patterns, ex, rel) {
			continue // ルートの外か、sources に合わない
		}
		byPath[rel] = content
	}
	out := make([]sourceFile, 0, len(byPath))
	for rel, content := range byPath {
		out = append(out, sourceFile{path: rel, content: content})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	p.Sources = make([]string, len(out))
	for i, s := range out {
		p.Sources[i] = s.path
	}
	return out, diags
}

// excluder は sources から常に除くディレクトリ: 各単位の out と [hozo].plugin_dirs（docs/config.md「形」）。
func (p *Project) excluder() excluder {
	var ex excluder
	for i := range p.Config.Units {
		ex = append(ex, config.CleanRel(p.Config.Units[i].Out))
	}
	for _, d := range p.Config.Hozo.PluginDirs {
		ex = append(ex, config.CleanRel(d))
	}
	return ex
}

// relPath は path をルートからの相対パス（"/" 区切り）にする。絶対パスでルートの外なら、または相対にできなければそのまま。
func (p *Project) relPath(path string) string {
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(p.Root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(path)
		}
		return config.CleanRel(rel)
	}
	return config.CleanRel(path)
}

// checkPlugins は CFG-6（unsupported のデコレータ）: 各単位について、その単位が出力する名前空間の、単位の対象の宣言に付いた
// デコレータを、単位のトランスパイラのマニフェストの unsupported に照らす。同じ位置の同じ診断は一度だけ。
func (p *Project) checkPlugins(checker check.Checker) []diag.Diagnostic {
	var diags []diag.Diagnostic
	if p.Sema == nil || p.Sema.Program == nil {
		return nil
	}
	type key struct {
		span source.Span
		file string
		msg  string
	}
	seen := map[key]bool{}
	for i := range p.Units {
		u := &p.Units[i]
		var unsupported []string
		for _, t := range u.Transpilers {
			unsupported = append(unsupported, t.Decl.Unsupported...)
		}
		if len(unsupported) == 0 {
			continue
		}
		for _, ns := range p.covered(i) {
			for _, d := range checker.CheckUnsupported(p.Sema.Program, ns, u.Files, unsupported) {
				k := key{file: d.File, msg: d.Message}
				if d.Span != nil {
					k.span = *d.Span
				}
				if !seen[k] {
					seen[k] = true
					diags = append(diags, d)
				}
			}
		}
	}
	return diags
}

// covered は Program の名前空間のうち、単位 unit（Units の添字）の対象のもの（docs/config.md「単位の対象」）を Program の順で返す。
func (p *Project) covered(unit int) []string {
	var out []string
	if p.Sema == nil || p.Sema.Program == nil {
		return out
	}
	for i := range p.Sema.Program.Namespaces {
		name := p.Sema.Program.Namespaces[i].Name
		if name == "" {
			continue
		}
		for _, c := range p.Config.Covering(name) {
			if c == unit {
				out = append(out, name)
			}
		}
	}
	return out
}

// Covered は Program の名前空間のうち、単位 unit（Units の添字）の対象のもの。in で出力しなくなった名前空間も含む
// （View を持つとは限らない）。
func (p *Project) Covered(unit int) []string { return p.covered(unit) }

// runPluginChecks は、使うプラグインのうち [check] enabled のものに direction "check" を送り、返った診断を足す
// （docs/cli.md「check」の 4、docs/plugin-protocol.md「起動」「check」）。プラグインと名前空間の組ごとに一回で、
// 相手の名前空間は、そのプラグインのトランスパイラを書いた単位が出力するもの。渡す view は、その単位のうち設定の順で
// 最初のもの。protocol の合わないプラグイン（MAN-2）は起動しない。
// code "protocol" / "unsupported" の応答は MAN-2 / MAN-5 の診断。proc.Crashed / proc.MalformedOutput なら
// そこで止めて、その誤りを返す。
func (p *Project) runPluginChecks(opts Options) ([]diag.Diagnostic, error) {
	var diags []diag.Diagnostic
	var views []map[string]*ir.View
	runner := hostplugin.Runner{Root: p.Root, Stderr: opts.Stderr, Env: opts.Env}
	type pair struct {
		plugin *hostplugin.Plugin
		ns     string
	}
	done := map[pair]bool{}
	for i := range p.Units {
		for _, t := range p.Units[i].Transpilers {
			pl := t.Plugin
			if !pl.Manifest.Check.Enabled || len(pl.Mismatch) > 0 {
				continue
			}
			if views == nil {
				views = p.Views()
			}
			for _, ns := range p.covered(i) {
				view := views[i][ns]
				if view == nil || done[pair{pl, ns}] {
					continue
				}
				done[pair{pl, ns}] = true
				req := sdk.CheckRequest{Protocol: hostplugin.Protocol, Direction: "check", View: view}
				var resp sdk.CheckResponse
				if err := runner.Run(pl.Manifest, pl.Dir, &req, &resp); err != nil {
					var pe *hostplugin.PluginError
					if errors.As(err, &pe) {
						diags = append(diags, pe.Diagnostic(pl.ManifestPath()))
						continue
					}
					return diags, err
				}
				for _, d := range resp.Diagnostics {
					diags = append(diags, p.PluginDiagnostic(pl.Name, d))
				}
			}
		}
	}
	return diags, nil
}

// PluginDiagnostic はプラグインが返した診断（docs/plugin-protocol.md「check」の diagnostics、「emit」の errors）を
// 診断にする。severity が "warning" なら warning、それ以外は error。at は FQN（その宣言の位置）か "file:line:col"（Locate）。
func (p *Project) PluginDiagnostic(plugin string, d sdk.Diagnostic) diag.Diagnostic {
	sev := diag.Error
	if d.Severity == "warning" {
		sev = diag.Warning
	}
	out := diag.Diagnostic{Severity: sev, Code: d.Code, Message: d.Message}
	out.Span, out.File = p.Locate(d.At)
	return out
}

// PluginWarning はトランスパイラの warning（docs/plugin-protocol.md「emit」の warnings）を診断にする。
// コードは PLUGIN、メッセージの先頭にトランスパイラの名前。
func (p *Project) PluginWarning(transpiler string, w sdk.Warning) diag.Diagnostic {
	out := diag.Diagnostic{Severity: diag.Warning, Code: "PLUGIN", Message: transpiler + ": " + w.Message}
	out.Span, out.File = p.Locate(w.At)
	return out
}

// Locate はプラグインの at（docs/plugin-protocol.md「要求と応答」: FQN か "file:line:col"）を診断の位置にする。
// FQN ならその宣言の span。"file:line:col" ならそのファイルのその位置（終わりも同じ位置）。どちらでもなければ
// Span は nil で、at が読んだファイルのパスならそれを File に、そうでなければ File は空（表示は "hozoc: "）。
func (p *Project) Locate(at string) (*source.Span, string) {
	if at == "" {
		return nil, ""
	}
	if p.Sema != nil && p.Sema.Program != nil {
		if d := p.Sema.Program.Lookup(at); d != nil {
			if s, ok := check.FileSetLocator(p.Files)(d.Common().Span); ok {
				return &s, ""
			}
			return nil, d.Common().Span.File
		}
	}
	if file, line, col, ok := splitFileLineCol(at); ok {
		rel := p.relPath(file)
		for i := 0; i < p.Files.Len(); i++ {
			id := source.FileID(i)
			if p.Files.Path(id) != rel {
				continue
			}
			off := p.Files.Offset(id, source.Pos{Line: line, Col: col})
			return &source.Span{File: id, Start: off, End: off}, ""
		}
		return nil, rel
	}
	if !strings.Contains(at, ":") {
		rel := p.relPath(at)
		for i := 0; i < p.Files.Len(); i++ {
			if p.Files.Path(source.FileID(i)) == rel {
				return nil, rel
			}
		}
	}
	return nil, ""
}

// splitFileLineCol は "file:line:col" を分ける。
func splitFileLineCol(s string) (file string, line, col int, ok bool) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return "", 0, 0, false
	}
	j := strings.LastIndexByte(s[:i], ':')
	if j < 0 {
		return "", 0, 0, false
	}
	if _, err := fmt.Sscanf(s[j+1:i], "%d", &line); err != nil || line < 1 {
		return "", 0, 0, false
	}
	if _, err := fmt.Sscanf(s[i+1:], "%d", &col); err != nil || col < 1 {
		return "", 0, 0, false
	}
	return s[:j], line, col, true
}

// finish は診断の File をルートからの相対パスにし、設定なしなら CFG-* を除き、並べ替える。
func (p *Project) finish(diags []diag.Diagnostic) []diag.Diagnostic {
	out := make([]diag.Diagnostic, 0, len(diags))
	for _, d := range diags {
		if p.NoConfig && strings.HasPrefix(d.Code, "CFG-") {
			continue
		}
		if d.Span == nil && d.File != "" && !vocab.IsVocab(d.File) {
			d.File = p.relPath(d.File)
		}
		out = append(out, d)
	}
	diag.Sort(out, p.Files, nil)
	return out
}
