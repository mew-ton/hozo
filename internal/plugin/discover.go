package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
)

// PluginPathEnv はプラグインを探すディレクトリを OS のパス区切りで並べた環境変数（docs/plugin-protocol.md「探索」の 2）。
// このパッケージは環境変数を読まない。値は呼ぶ側が [SearchEnv] で渡す（hozoc は cli.hozo の入力の pluginPath から）。
const PluginPathEnv = "HOZO_PLUGIN_PATH"

// SearchEnv は探索が使う、プロジェクトの外の場所（docs/plugin-protocol.md「探索」の 2 と 3）。プロセスの環境変数や
// ホームディレクトリを探索が自分で読むことはなく、呼ぶ側がここに入れて渡す（同じ入力から同じ結果にするため。
// テストは実行する人の環境に左右されない）。零値なら [hozo].plugin_dirs だけを探す。
type SearchEnv struct {
	// PluginPath は HOZO_PLUGIN_PATH の値（OS のパス区切りで複数）。空なら探さない。
	PluginPath string
	// Home はホームディレクトリ。空なら <ホーム>/.hozo/plugins は探さない。
	Home string
}

// Plugin は入っているプラグイン一つ（docs/plugin-protocol.md「探索」）。
type Plugin struct {
	// Name はプラグインの名前（マニフェストのあるディレクトリの名前。マニフェストの name と同じ）。
	Name string
	// Dir はマニフェストのディレクトリ（絶対パス）。
	Dir string
	// Manifest は読んだマニフェスト。
	Manifest *Manifest
	// Mismatch は protocol の不一致（MAN-2）の診断。使うプラグインにだけ報告するので、Discover の診断には入れない。
	// 空でなければ、このプラグインは起動しない。
	Mismatch []diag.Diagnostic
}

// ManifestPath はマニフェストのパス。
func (p *Plugin) ManifestPath() string { return filepath.Join(p.Dir, ManifestFileName) }

// SearchDirs は入っているプラグインを探すディレクトリを、優先する順に返す（docs/plugin-protocol.md「探索」）:
// [hozo].plugin_dirs の各ディレクトリ（ルートからの相対）、HOZO_PLUGIN_PATH（env.PluginPath）の各ディレクトリ、
// <ホーム>/.hozo/plugins（env.Home）。
func SearchDirs(cfg *config.Config, env SearchEnv) []string {
	var dirs []string
	for _, d := range cfg.Hozo.PluginDirs {
		dirs = append(dirs, filepath.Join(cfg.Root, d))
	}
	for _, d := range filepath.SplitList(env.PluginPath) {
		if d == "" {
			continue
		}
		dirs = append(dirs, d)
	}
	if env.Home != "" {
		dirs = append(dirs, filepath.Join(env.Home, ".hozo", "plugins"))
	}
	return dirs
}

// Discover は入っているプラグインをすべて探してマニフェストを読む（docs/plugin-protocol.md「探索」）。
// SearchDirs の各ディレクトリ d の直下で `<d>/<p>/hozo-plugin.toml` を持つディレクトリ p がプラグインで、同じ名前 p は
// 最初に見つかったものだけを使う。返す順は、探したディレクトリの順、その中は名前の辞書順。
//
// 診断は、読めないマニフェスト（MAN-4）と、name がディレクトリの名前と違うマニフェスト（MAN-3）。どちらも
// そのプラグインは返さない。protocol の不一致（MAN-2）はプラグインを返し、診断は Plugin.Mismatch に置く
// （使うプラグインにだけ報告する）。
func Discover(cfg *config.Config, env SearchEnv) ([]*Plugin, []diag.Diagnostic) {
	var plugins []*Plugin
	var diags []diag.Diagnostic
	seen := map[string]bool{}
	for _, d := range SearchDirs(cfg, env) {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue // ないディレクトリ、読めないディレクトリは飛ばす
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.Join(d, name, ManifestFileName)
			info, err := os.Stat(path)
			if err != nil || info.IsDir() || seen[name] {
				continue
			}
			seen[name] = true
			dir, err := filepath.Abs(filepath.Dir(path))
			if err != nil {
				dir = filepath.Dir(path)
			}
			m, ds := LoadManifest(path)
			if m == nil {
				diags = append(diags, ds...)
				continue
			}
			if m.Name != name {
				diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "MAN-3",
					Message: fmt.Sprintf("マニフェストの name %q がディレクトリ名 %q と違う", m.Name, name), File: path})
				continue
			}
			plugins = append(plugins, &Plugin{Name: name, Dir: dir, Manifest: m, Mismatch: ds})
		}
	}
	return plugins, diags
}
