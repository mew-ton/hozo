// Package config は hozo.toml（設計書 §4.5）の読み込み。形と読み方、設定だけで検出できる検査（CFG-*）は
// docs/config.md。プログラムが要る検査（CFG-1 / CFG-2 の .hozo 側 / CFG-4〜6 / CFG-8〜10）は
// internal/check がこの Config を見て行う。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/mew-ton/hozo/internal/diag"
)

// FileName は設定ファイルの名前（docs/config.md「置き場と探し方」）。
const FileName = "hozo.toml"

// 既定値（docs/config.md「形」）。
const (
	DefaultMaxBindings = 5 // WIRE-6 の閾値（docs/check.md「配線」）
)

// DefaultSources は [hozo].sources の既定。
var DefaultSources = []string{"**/*.hozo"}

// Config は hozo.toml 一つ分（docs/packages.md「internal/config」）。
type Config struct {
	// Root は hozo.toml のあるディレクトリ（絶対パス）。プロジェクトのルート（docs/config.md「置き場と探し方」）。
	Root string
	// Path は hozo.toml の絶対パス。設定なし（Default）なら空。
	Path string
	// Hozo は [hozo] テーブル。省略されたキーは既定値で埋めてある。
	Hozo HozoTable
	// Namespaces は名前空間のテーブル。キーは FQN（[server.admin] は "server.admin"）。
	// 書かれたテーブルだけを持ち、継承は Resolve が行う。
	Namespaces map[string]NamespaceTable
}

// HozoTable は [hozo] テーブル（docs/config.md「形」）。
type HozoTable struct {
	Sources     []string // ルートからの glob。既定は DefaultSources
	PluginDirs  []string // プラグインを探すディレクトリ。ルートからの相対パス。既定は []
	MaxBindings int      // WIRE-6 の閾値。既定は DefaultMaxBindings
}

// NamespaceTable は名前空間のテーブルに書かれたキー。書かれていないキーは nil。
type NamespaceTable struct {
	Language *string
	Plugins  []string // 書かれていなければ nil。書かれていれば空でも非 nil
	Out      *string
}

// NamespaceConf は継承を解決した名前空間の設定 Conf(N)（docs/config.md「名前空間の設定の解決」）。
// View.config としてプラグインに渡す形（docs/ir.md「その名前空間から見た IR」）。
// Language と Out は未設定なら ""。Plugins は未設定なら空の列。
type NamespaceConf struct {
	Language string
	Plugins  []string
	Out      string
}

// Default は hozo.toml がないときの設定（docs/lsp.md「プロジェクト」の「設定なしで動く」）。
// root をルートにし、[hozo] は既定値、名前空間のテーブルはなし。
func Default(root string) *Config {
	return &Config{
		Root:       root,
		Hozo:       HozoTable{Sources: append([]string(nil), DefaultSources...), PluginDirs: []string{}, MaxBindings: DefaultMaxBindings},
		Namespaces: map[string]NamespaceTable{},
	}
}

// Find は startDir から親へ向かって最初に見つかる hozo.toml の絶対パスを返す（docs/config.md「置き場と探し方」）。
func Find(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}
	for {
		p := filepath.Join(dir, FileName)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Load は path の hozo.toml を読む（docs/config.md「形」）。診断は CFG-2（テーブル名）、CFG-7（知らないキー、
// 値の型違い、TOML の構文エラー、language が go でも ts でもない）、CFG-11（language が ts）。
// error の診断があっても、読めた範囲の Config を返す（nil は返さない）。読めなかった部分は既定値。
func Load(path string) (*Config, []diag.Diagnostic) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	cfg := Default(filepath.Dir(abs))
	cfg.Path = abs
	l := &loader{cfg: cfg, file: path}

	data, err := os.ReadFile(path)
	if err != nil {
		l.errorf("CFG-7", "hozo.toml を読めない: %v", err)
		return cfg, l.diags
	}
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		if pe, ok := err.(toml.ParseError); ok {
			l.errorf("CFG-7", "hozo.toml の %d 行目に構文の誤りがある: %s", pe.Position.Line, pe.Message)
		} else {
			l.errorf("CFG-7", "hozo.toml を TOML として読めない: %v", err)
		}
		return cfg, l.diags
	}
	l.load(raw)
	return cfg, l.diags
}

// Resolve は名前空間 namespace（FQN）の設定 Conf(N) を継承を解決して返す（docs/config.md「名前空間の設定の解決」）。
// N のテーブルにあるキーを使い、ないキーは親のテーブルから引き継ぐ。値はそのまま（out も同じディレクトリ）。
// 二つ目の返り値は、N かその祖先のどれかにテーブルがあるかどうか。false なら NamespaceConf は零値。
func (c *Config) Resolve(namespace string) (NamespaceConf, bool) {
	var conf NamespaceConf
	found := false
	plugins := false
	for n := namespace; n != ""; n = parent(n) {
		t, ok := c.Namespaces[n]
		if !ok {
			continue
		}
		found = true
		if conf.Language == "" && t.Language != nil {
			conf.Language = *t.Language
		}
		if conf.Out == "" && t.Out != nil {
			conf.Out = *t.Out
		}
		if !plugins && t.Plugins != nil {
			conf.Plugins = append([]string{}, t.Plugins...)
			plugins = true
		}
	}
	if !found {
		return NamespaceConf{}, false
	}
	if conf.Plugins == nil {
		conf.Plugins = []string{}
	}
	return conf, true
}

// Dir は名前空間 namespace（FQN）の出力先 Dir(N)（docs/config.md「名前空間の設定の解決」）。ルートからの相対パス（`/` 区切り）。
// Conf(N).out に P を足したもの。P は、N の祖先（N 自身を含む）のうち out を自分のテーブルに書いた最も近い名前空間 A から
// N までのセグメントを `/` でつないだもの（A が N 自身なら空）。P が空なら Conf(N).out そのまま。
// 二つ目の返り値は Resolve と同じで、false なら "" を返す。どの祖先も out を書いていなければ P は空（CFG-1 は check が判定する）。
func (c *Config) Dir(namespace string) (string, bool) {
	conf, ok := c.Resolve(namespace)
	if !ok {
		return "", false
	}
	rest := ""
	for n := namespace; n != ""; n = parent(n) {
		if t, ok := c.Namespaces[n]; ok && t.Out != nil {
			rest = strings.TrimPrefix(namespace[len(n):], ".")
			break
		}
	}
	if rest == "" {
		return conf.Out, true
	}
	return conf.Out + "/" + strings.ReplaceAll(rest, ".", "/"), true
}

// NamespaceNames は Namespaces のキーを辞書順に返す。診断の順序を決定的にするためのもの。
func (c *Config) NamespaceNames() []string {
	names := make([]string, 0, len(c.Namespaces))
	for n := range c.Namespaces {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// parent は FQN の親。"server.admin" → "server"、"server" → ""。
func parent(fqn string) string {
	i := strings.LastIndexByte(fqn, '.')
	if i < 0 {
		return ""
	}
	return fqn[:i]
}

// reservedNames は名前空間の名前にできない名前（CFG-2）。
var reservedNames = map[string]bool{"language": true, "plugins": true, "out": true, "hozo": true}

// IsReservedNamespaceName は name（FQN の一つのセグメント）が CFG-2 の名前かどうか。
// internal/check が .hozo の名前空間の名前を確かめるときにも使う。
func IsReservedNamespaceName(name string) bool { return reservedNames[name] }

// loader は一つの hozo.toml の読み込みの状態。
type loader struct {
	cfg   *Config
	file  string
	diags []diag.Diagnostic
}

func (l *loader) errorf(code, format string, args ...any) {
	l.diags = append(l.diags, diag.Diagnostic{
		Severity: diag.Error,
		Code:     code,
		Message:  fmt.Sprintf(format, args...),
		File:     l.file,
	})
}

// load は TOML の木を Config にする。
func (l *loader) load(raw map[string]any) {
	for _, key := range sortedKeys(raw) {
		value := raw[key]
		if key == "hozo" {
			t, ok := value.(map[string]any)
			if !ok {
				l.errorf("CFG-7", "[hozo] はテーブルでなければならない（%s）", typeName(value))
				continue
			}
			l.loadHozo(t)
			continue
		}
		t, ok := value.(map[string]any)
		if !ok {
			l.errorf("CFG-7", "知らないキー %s（名前空間のテーブルか [hozo] だけを書ける）", key)
			continue
		}
		l.loadNamespace(key, key, t)
	}
}

// loadHozo は [hozo] テーブル（docs/config.md「形」）。
func (l *loader) loadHozo(t map[string]any) {
	for _, key := range sortedKeys(t) {
		value := t[key]
		switch key {
		case "sources":
			if v, ok := l.stringList("hozo."+key, value); ok {
				l.cfg.Hozo.Sources = v
			}
		case "plugin_dirs":
			if v, ok := l.stringList("hozo."+key, value); ok {
				l.cfg.Hozo.PluginDirs = v
			}
		case "max_bindings":
			n, ok := value.(int64)
			if !ok {
				l.errorf("CFG-7", "hozo.max_bindings は整数でなければならない（%s）", typeName(value))
				continue
			}
			if n < 0 {
				l.errorf("CFG-7", "hozo.max_bindings は 0 以上でなければならない（%d）", n)
				continue
			}
			l.cfg.Hozo.MaxBindings = int(n)
		default:
			l.errorf("CFG-7", "知らないキー hozo.%s", key)
		}
	}
}

// loadNamespace は名前空間のテーブル。fqn はこのテーブルの名前空間の FQN、name はその最後のセグメント。
// 子のテーブル（ドットを含むテーブル名、またはインラインテーブル）は子の名前空間（docs/config.md「形」）。
func (l *loader) loadNamespace(fqn, name string, t map[string]any) {
	if IsReservedNamespaceName(name) {
		// CFG-2: hozo.toml にこれらの名前のテーブルがある（[hozo] 以外）
		l.errorf("CFG-2", "名前空間の名前に %s は使えない（[%s]）", name, fqn)
		return
	}
	var nt NamespaceTable
	for _, key := range sortedKeys(t) {
		value := t[key]
		switch key {
		case "language":
			s, ok := value.(string)
			if !ok {
				if _, isTable := value.(map[string]any); isTable {
					l.errorf("CFG-2", "名前空間の名前に %s は使えない（[%s.%s]）", key, fqn, key)
				} else {
					l.errorf("CFG-7", "%s.language は文字列でなければならない（%s）", fqn, typeName(value))
				}
				continue
			}
			switch s {
			case "go":
			case "ts":
				// CFG-11: TS の既定の出力はまだない（段階 5 まで）
				l.errorf("CFG-11", "%s の language が \"ts\" だが、TS の出力はまだない", fqn)
			default:
				l.errorf("CFG-7", "%s.language は \"go\" か \"ts\" でなければならない（%q）", fqn, s)
				continue
			}
			nt.Language = &s
		case "out":
			s, ok := value.(string)
			if !ok {
				if _, isTable := value.(map[string]any); isTable {
					l.errorf("CFG-2", "名前空間の名前に %s は使えない（[%s.%s]）", key, fqn, key)
				} else {
					l.errorf("CFG-7", "%s.out は文字列でなければならない（%s）", fqn, typeName(value))
				}
				continue
			}
			nt.Out = &s
		case "plugins":
			if _, isTable := value.(map[string]any); isTable {
				l.errorf("CFG-2", "名前空間の名前に %s は使えない（[%s.%s]）", key, fqn, key)
				continue
			}
			if v, ok := l.stringList(fqn+"."+key, value); ok {
				nt.Plugins = v
			}
		default:
			child, ok := value.(map[string]any)
			if !ok {
				l.errorf("CFG-7", "知らないキー %s.%s", fqn, key)
				continue
			}
			l.loadNamespace(fqn+"."+key, key, child)
		}
	}
	l.cfg.Namespaces[fqn] = nt
}

// stringList は文字列の配列を読む。違えば CFG-7。
func (l *loader) stringList(key string, value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		l.errorf("CFG-7", "%s は文字列の配列でなければならない（%s）", key, typeName(value))
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			l.errorf("CFG-7", "%s の要素は文字列でなければならない（%s）", key, typeName(item))
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// typeName は診断に出す TOML の値の種類の名前。
func typeName(v any) string {
	switch v.(type) {
	case string:
		return "文字列"
	case int64:
		return "整数"
	case float64:
		return "小数"
	case bool:
		return "真偽値"
	case []any:
		return "配列"
	case map[string]any:
		return "テーブル"
	case []map[string]any:
		return "テーブルの配列"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
