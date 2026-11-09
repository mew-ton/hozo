package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
)

// PluginPathEnv はプラグインを探すディレクトリを OS のパス区切りで並べた環境変数（docs/plugin-protocol.md「探索」の 2）。
const PluginPathEnv = "HOZO_PLUGIN_PATH"

// SearchDirs は name のプラグインのマニフェストを探す候補のパスを、優先する順に返す（docs/plugin-protocol.md「探索」）:
// [hozo].plugin_dirs の各 d について <ルート>/<d>/<name>/hozo-plugin.toml、HOZO_PLUGIN_PATH の各 d について
// <d>/<name>/hozo-plugin.toml、<ホーム>/.hozo/plugins/<name>/hozo-plugin.toml。
func SearchDirs(cfg *config.Config, name string) []string {
	var paths []string
	for _, d := range cfg.Hozo.PluginDirs {
		paths = append(paths, filepath.Join(cfg.Root, d, name, ManifestFileName))
	}
	for _, d := range filepath.SplitList(os.Getenv(PluginPathEnv)) {
		if d == "" {
			continue
		}
		paths = append(paths, filepath.Join(d, name, ManifestFileName))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".hozo", "plugins", name, ManifestFileName))
	}
	return paths
}

// Discover は name のプラグインのマニフェストを探して読む（docs/plugin-protocol.md「探索」）。
// 返すのはマニフェストと、そのディレクトリ（絶対パス）。見つからなければ CFG-9。マニフェストの name が name と
// 違えば MAN-3。読み込みの診断は LoadManifest（MAN-1 / MAN-2 / MAN-4）。error があればマニフェストは nil。
func Discover(cfg *config.Config, name string) (*Manifest, string, []diag.Diagnostic) {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return nil, "", []diag.Diagnostic{{Severity: diag.Error, Code: "CFG-9", Message: fmt.Sprintf("プラグインの名前 %q が不正", name), File: cfg.Path}}
	}
	for _, p := range SearchDirs(cfg, name) {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		dir, err := filepath.Abs(filepath.Dir(p))
		if err != nil {
			dir = filepath.Dir(p)
		}
		m, diags := LoadManifest(p)
		if m == nil {
			return nil, dir, diags
		}
		if m.Name != name {
			diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: "MAN-3", Message: fmt.Sprintf("マニフェストの name %q がディレクトリ名 %q と違う", m.Name, name), File: p})
		}
		if len(diags) > 0 {
			return nil, dir, diags
		}
		return m, dir, nil
	}
	return nil, "", []diag.Diagnostic{{Severity: diag.Error, Code: "CFG-9", Message: fmt.Sprintf("プラグイン %s が見つからない（plugin_dirs、%s、~/.hozo/plugins を探した）", name, PluginPathEnv), File: cfg.Path}}
}
