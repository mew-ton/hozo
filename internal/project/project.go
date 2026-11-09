package project

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

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
	// Manifests は、どれかの名前空間の plugins に書かれ、見つかったプラグインのマニフェスト。キーはプラグインの名前。
	Manifests map[string]*hostplugin.Manifest
	// ManifestDirs はマニフェストのディレクトリ（絶対パス）。Manifests と同じキー。
	ManifestDirs map[string]string
	// RunErr はプラグインの check が proc.Crashed / proc.MalformedOutput（internal/plugin の ErrCrashed /
	// ErrMalformedOutput）で終わったときのその誤り。最初の一つで止め、以降のプラグインは呼ばない。なければ nil。
	RunErr error
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
}

// Load は root から hozo.toml を探して（configPath があればそれを読んで）プロジェクトを解析する
// （docs/packages.md「internal/project」）。hozo.toml がなければ既定の設定で解析し、CFG-* の診断を除く。
// overlay の内容はディスクの内容より優先し、ディスクにないファイルも足せる。
func Load(root, configPath string, overlay map[string][]byte) *Project {
	return LoadWith(Options{Root: root, ConfigPath: configPath, Overlay: overlay})
}

// LoadWith は Load の本体。
func LoadWith(opts Options) *Project {
	p := &Project{Files: source.NewFileSet(), Manifests: map[string]*hostplugin.Manifest{}, ManifestDirs: map[string]string{}}
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

	// プラグインのマニフェスト（docs/plugin-protocol.md「探索」）
	diags = append(diags, p.discoverPlugins()...)

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
	for _, name := range p.pluginNames() {
		m := p.Manifests[name]
		if m == nil || len(m.Vocabulary) == 0 {
			continue
		}
		if err := vocab.AddPluginTo(add, name, p.ManifestDirs[name], m.Vocabulary); err != nil {
			diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "MAN-4", Message: err.Error(),
				File: filepath.Join(p.ManifestDirs[name], hostplugin.ManifestFileName)})
		}
	}

	// 3. 解決・正規化・検査（docs/resolve.md、docs/ir.md、docs/check.md）
	p.Sema = sema.Analyze(p.Files, files, p.Config)
	diags = append(diags, p.Sema.Diags...)
	checker := check.Checker{Locate: check.FileSetLocator(p.Files)}
	diags = append(diags, checker.Check(p.Sema.Program, p.Config)...)
	diags = append(diags, p.checkPlugins(checker)...)

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

// Views は設定のある名前空間ごとの View（docs/ir.md「その名前空間から見た IR」）。
func (p *Project) Views() map[string]*ir.View {
	if p.Sema == nil {
		return map[string]*ir.View{}
	}
	return check.Views(p.Sema.Program, p.Config)
}

// Dir は名前空間 ns の出力先 Dir(N)（docs/config.md「名前空間の設定の解決」。config.Config.Dir）を
// ルートからの相対パス（"/" 区切り、正規化済み）で返す。Conf(N).out が未設定なら false。
func (p *Project) Dir(ns string) (string, bool) {
	d, ok := p.Config.Dir(ns)
	if !ok || d == "" {
		return "", false
	}
	return cleanRel(d), true
}

// Plugins は名前空間 ns の Conf(N).plugins のうち、マニフェストが見つかったもの（名前の順は設定の順）。
func (p *Project) Plugins(ns string) []*hostplugin.Manifest {
	conf, ok := p.Config.Resolve(ns)
	if !ok {
		return nil
	}
	var ms []*hostplugin.Manifest
	for _, name := range conf.Plugins {
		if m := p.Manifests[name]; m != nil {
			ms = append(ms, m)
		}
	}
	return ms
}

// ManifestPath はプラグイン name のマニフェストのパス（診断の File に使う形: ルートからの相対パス）。
func (p *Project) ManifestPath(name string) string {
	return p.relPath(filepath.Join(p.ManifestDirs[name], hostplugin.ManifestFileName))
}

// pluginNames は設定のどこかの plugins に書かれたプラグインの名前を辞書順に返す。
func (p *Project) pluginNames() []string {
	set := map[string]bool{}
	for _, ns := range p.Config.NamespaceNames() {
		conf, _ := p.Config.Resolve(ns)
		for _, name := range conf.Plugins {
			set[name] = true
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// discoverPlugins は設定に書かれたすべてのプラグインのマニフェストを探して読む（docs/plugin-protocol.md「探索」）。
// 診断は CFG-9 と MAN-*。
func (p *Project) discoverPlugins() []diag.Diagnostic {
	var diags []diag.Diagnostic
	for _, name := range p.pluginNames() {
		m, dir, ds := hostplugin.Discover(p.Config, name)
		diags = append(diags, ds...)
		if m != nil {
			p.Manifests[name] = m
			p.ManifestDirs[name] = dir
		}
	}
	return diags
}

// sourceFile は読んだユーザーのファイル。
type sourceFile struct {
	path    string
	content []byte
}

// readSources は [hozo].sources の glob に合うファイルを読む（docs/config.md「形」）。各名前空間の out の下と
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
		if rel == "" || !matchesAny(patterns, ex, rel) {
			continue
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

// excluder は sources から常に除くディレクトリ: 各名前空間の Conf(N).out と [hozo].plugin_dirs（docs/config.md「形」）。
func (p *Project) excluder() excluder {
	var ex excluder
	for _, ns := range p.Config.NamespaceNames() {
		if conf, ok := p.Config.Resolve(ns); ok && conf.Out != "" {
			ex = append(ex, cleanRel(conf.Out))
		}
	}
	for _, d := range p.Config.Hozo.PluginDirs {
		ex = append(ex, cleanRel(d))
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
		return cleanRel(rel)
	}
	return cleanRel(path)
}

// checkPlugins はマニフェストが要る設定の検査: CFG-4（language の不一致）と CFG-6（unsupported のデコレータ）。
// 設定のあるすべての名前空間（Program のもの）について行う。
func (p *Project) checkPlugins(checker check.Checker) []diag.Diagnostic {
	var diags []diag.Diagnostic
	cfgFile := p.Config.Path
	if cfgFile == "" {
		cfgFile = config.FileName
	}
	for _, ns := range p.configuredNamespaces() {
		conf, _ := p.Config.Resolve(ns)
		var unsupported []string
		for _, name := range conf.Plugins {
			m := p.Manifests[name]
			if m == nil {
				continue
			}
			if conf.Language != "" && m.Language != conf.Language {
				diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "CFG-4", File: cfgFile,
					Message: fmt.Sprintf("名前空間 %s の language %q とプラグイン %s の language %q が合わない", ns, conf.Language, name, m.Language)})
			}
			if m.Emit != nil {
				unsupported = append(unsupported, m.Emit.Unsupported...)
			}
		}
		if len(unsupported) > 0 {
			diags = append(diags, checker.CheckUnsupported(p.Sema.Program, ns, unsupported)...)
		}
	}
	return diags
}

// configuredNamespaces は Program の名前空間のうち設定のあるもの（Resolve が true）を Program の順で返す。
func (p *Project) configuredNamespaces() []string {
	var out []string
	if p.Sema == nil || p.Sema.Program == nil {
		return out
	}
	for i := range p.Sema.Program.Namespaces {
		name := p.Sema.Program.Namespaces[i].Name
		if _, ok := p.Config.Resolve(name); ok {
			out = append(out, name)
		}
	}
	return out
}

// runPluginChecks は [check] enabled のプラグインに direction "check" を送り、返った診断を足す
// （docs/cli.md「check」の 4、docs/plugin-protocol.md「check」）。一つのプラグインは名前空間ごとに一回起動する。
// code "protocol" / "unsupported" の応答は MAN-2 / MAN-5 の診断。proc.Crashed / proc.MalformedOutput なら
// そこで止めて、その誤りを返す。
func (p *Project) runPluginChecks(opts Options) ([]diag.Diagnostic, error) {
	var diags []diag.Diagnostic
	var views map[string]*ir.View
	runner := hostplugin.Runner{Root: p.Root, Stderr: opts.Stderr, Env: opts.Env}
	for _, ns := range p.configuredNamespaces() {
		conf, _ := p.Config.Resolve(ns)
		for _, name := range conf.Plugins {
			m := p.Manifests[name]
			if m == nil || !m.Check.Enabled {
				continue
			}
			if views == nil {
				views = p.Views()
			}
			view := views[ns]
			if view == nil {
				continue
			}
			req := sdk.CheckRequest{Protocol: hostplugin.Protocol, Direction: "check", View: view}
			var resp sdk.CheckResponse
			if err := runner.Run(m, p.ManifestDirs[name], &req, &resp); err != nil {
				var pe *hostplugin.PluginError
				if errors.As(err, &pe) {
					diags = append(diags, pe.Diagnostic(filepath.Join(p.ManifestDirs[name], hostplugin.ManifestFileName)))
					continue
				}
				return diags, err
			}
			for _, d := range resp.Diagnostics {
				diags = append(diags, p.PluginDiagnostic(name, d))
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

// PluginWarning はプラグインの warning（docs/plugin-protocol.md「emit」の warnings）を診断にする。
// コードは PLUGIN、メッセージの先頭にプラグイン名。
func (p *Project) PluginWarning(plugin string, w sdk.Warning) diag.Diagnostic {
	out := diag.Diagnostic{Severity: diag.Warning, Code: "PLUGIN", Message: plugin + ": " + w.Message}
	out.Span, out.File = p.Locate(w.At)
	return out
}

// Locate はプラグインの at（docs/plugin-protocol.md「要求と応答」: FQN か "file:line:col"）を診断の位置にする。
// FQN ならその宣言の span。"file:line:col" ならそのファイルのその位置（終わりも同じ位置）。どちらでもなければ
// Span は nil で、at がファイルのパスの形（":" を含まない）ならそれを File に、そうでなければ File は空。
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
			off := offsetOf(p.Files.Content(id), line, col)
			return &source.Span{File: id, Start: off, End: off}, ""
		}
		return nil, rel
	}
	if !strings.Contains(at, ":") {
		return nil, p.relPath(at)
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

// offsetOf は 1 始まりの行・列（コードポイント）をバイトのオフセットにする（docs/diagnostics.md「位置」）。
// 範囲を越えればファイルの終わり（または行の終わり）。
func offsetOf(content []byte, line, col int) source.Offset {
	off := 0
	for l := 1; l < line; l++ {
		i := strings.IndexByte(string(content[off:]), '\n')
		if i < 0 {
			return source.Offset(len(content))
		}
		off += i + 1
	}
	for c := 1; c < col && off < len(content); c++ {
		if content[off] == '\n' {
			break
		}
		_, n := utf8.DecodeRune(content[off:])
		off += n
	}
	return source.Offset(off)
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
