// Package config は hozo.toml（設計書 §4.5 を出力の単位の一覧に変えたもの）の読み込み。形と読み方、設定だけで
// 検出できる検査（CFG-*）は docs/config.md。プログラムが要る検査（CFG-6 / CFG-8 / CFG-10）は internal/check が、
// マニフェストと読んだファイルが要る検査（CFG-11 / CFG-14〜CFG-16 / CFG-18）は internal/project が、この Config を見て行う。
package config

import (
	"fmt"
	"os"
	"path"
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
	// Units は出力の単位（[[emit]]）。書いた順（設定の順）。必須のキーを欠いた単位（CFG-12）は入れない。
	Units []Unit
}

// HozoTable は [hozo] テーブル（docs/config.md「形」）。
type HozoTable struct {
	Sources     []string // ルートからの glob。既定は DefaultSources
	PluginDirs  []string // プラグインを探すディレクトリ。ルートからの相対パス。既定は []
	MaxBindings int      // WIRE-6 の閾値。既定は DefaultMaxBindings
}

// Unit は出力の単位一つ（docs/config.md「単位」）: どのファイルを、どの名前空間から見て、どのトランスパイラで、どこへ書き出すか。
type Unit struct {
	// Index は何番目の [[emit]] か（1 始まり。CFG-12 で使わなかった単位も数える）。診断で単位を指すのに使う。
	Index int
	// In は in の glob（ルートからの相対）。省略されていれば nil（読み込んだすべてのファイル）。
	In []string
	// Namespace は視点の名前空間（FQN）。
	Namespace string
	// Transpilers はトランスパイラの名前（書いた順）。一つ以上。
	Transpilers []string
	// Out は出力先（ルートからの相対パス。書かれたまま）。
	Out string
}

// Label は診断で単位を指す言い方（docs/config.md「診断」）: 何番目の [[emit]] かと、その namespace と out。
func (u *Unit) Label() string {
	return fmt.Sprintf("%d 番目の [[emit]]（namespace = %q、out = %q）", u.Index, u.Namespace, u.Out)
}

// Default は hozo.toml がないときの設定（docs/config.md「置き場と探し方」の「設定なし」、docs/lsp.md「プロジェクト」）。
// root をルートにし、[hozo] は既定値、単位はなし。
func Default(root string) *Config {
	return &Config{
		Root: root,
		Hozo: HozoTable{Sources: append([]string(nil), DefaultSources...), PluginDirs: []string{}, MaxBindings: DefaultMaxBindings},
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

// Load は path の hozo.toml を読む（docs/config.md「形」）。診断は CFG-7（知らないキー、値の型違い、TOML の構文エラー、
// transpilers の重複）、CFG-12（単位の必須のキーがない、または空）、CFG-17（二つの単位の out が同じか入れ子）。
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
	l.checkOuts()
	return cfg, l.diags
}

// Covering は名前空間 namespace（FQN）を対象にする単位を、Units の添字で設定の順に返す（docs/config.md「単位の対象」の
// 手続き。CFG-13）: namespace とその祖先のうち、どれかの単位の namespace に書かれている最も近いもの A を探し、
// namespace が A であるすべての単位を返す。どれも書かれていなければ nil。
func (c *Config) Covering(namespace string) []int {
	for n := namespace; n != ""; n = parent(n) {
		var out []int
		for i := range c.Units {
			if c.Units[i].Namespace == n {
				out = append(out, i)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// Dir は単位 unit（Units の添字）での名前空間 namespace の出力先 Dir(U, N)（docs/config.md「単位の対象」）。
// ルートからの相対パス（`/` 区切り、正規化済み）: U.out に、U.namespace から namespace までのセグメントをパスにして足したもの。
// namespace が U.namespace でもその子孫でもなければ U.out。
func (c *Config) Dir(unit int, namespace string) string {
	u := &c.Units[unit]
	out := CleanOut(u.Out)
	if namespace == u.Namespace || !strings.HasPrefix(namespace, u.Namespace+".") {
		return out
	}
	rest := strings.ReplaceAll(namespace[len(u.Namespace)+1:], ".", "/")
	if out == "" {
		return rest
	}
	return out + "/" + rest
}

// UnitNamespaces は単位の namespace を、重複なしの辞書順で返す。CFG-8 / CFG-10 と、診断の順序を決定的にするためのもの。
func (c *Config) UnitNamespaces() []string {
	seen := map[string]bool{}
	var names []string
	for i := range c.Units {
		if n := c.Units[i].Namespace; !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// CleanOut は out を比べられる形にする: `/` 区切りにし、`./` や余分な `/` を取り除く（docs/config.md「単位の対象」）。
// ルートそのもの（"." など）は ""。
func CleanOut(out string) string {
	p := path.Clean(filepath.ToSlash(out))
	p = strings.TrimPrefix(p, "/")
	if p == "." {
		return ""
	}
	return p
}

// parent は FQN の親。"server.admin" → "server"、"server" → ""。
func parent(fqn string) string {
	i := strings.LastIndexByte(fqn, '.')
	if i < 0 {
		return ""
	}
	return fqn[:i]
}

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

// load は TOML の木を Config にする。トップレベルに書けるのは [hozo] と [[emit]] だけ（docs/config.md「形」）。
func (l *loader) load(raw map[string]any) {
	for _, key := range sortedKeys(raw) {
		value := raw[key]
		switch key {
		case "hozo":
			t, ok := value.(map[string]any)
			if !ok {
				l.errorf("CFG-7", "[hozo] はテーブルでなければならない（%s）", typeName(value))
				continue
			}
			l.loadHozo(t)
		case "emit":
			units, ok := value.([]map[string]any)
			if !ok {
				l.errorf("CFG-7", "emit はテーブルの配列（[[emit]]）でなければならない（%s）", typeName(value))
				continue
			}
			for i, t := range units {
				l.loadUnit(i+1, t)
			}
		default:
			l.errorf("CFG-7", "知らないキー %s（トップレベルに書けるのは [hozo] と [[emit]] だけ）", key)
		}
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

// loadUnit は index 番目（1 始まり）の [[emit]]（docs/config.md「形」「単位」）。知らないキーと値の型違いは CFG-7、
// namespace / transpilers / out のどれかがない、または空なら CFG-12。どちらかがあれば、その単位は Units に入れない。
func (l *loader) loadUnit(index int, t map[string]any) {
	u := Unit{Index: index}
	ok := true
	where := fmt.Sprintf("%d 番目の [[emit]]", index)
	for _, key := range sortedKeys(t) {
		value := t[key]
		switch key {
		case "in":
			v, good := l.stringList(where+" の in", value)
			if !good {
				ok = false
				continue
			}
			u.In = v
		case "namespace", "out":
			s, good := value.(string)
			if !good {
				l.errorf("CFG-7", "%s の %s は文字列でなければならない（%s）", where, key, typeName(value))
				ok = false
				continue
			}
			if key == "namespace" {
				u.Namespace = s
			} else {
				u.Out = s
			}
		case "transpilers":
			v, good := l.stringList(where+" の transpilers", value)
			if !good {
				ok = false
				continue
			}
			seen := map[string]bool{}
			for _, name := range v {
				if seen[name] {
					l.errorf("CFG-7", "%s の transpilers に %s が二度ある", where, name)
					ok = false
				}
				seen[name] = true
			}
			u.Transpilers = v
		default:
			l.errorf("CFG-7", "知らないキー %s（%s）", key, where)
			ok = false
		}
	}
	// CFG-12: 必須のキーがない、または空。型違いで読めなかったキーは CFG-7 が出ているので、ここでは数えない。
	var missing []string
	for _, key := range []string{"namespace", "transpilers", "out"} {
		if _, written := t[key]; !written {
			missing = append(missing, key+" がない")
			continue
		}
		switch key {
		case "namespace":
			if s, isStr := t[key].(string); isStr && s == "" {
				missing = append(missing, key+" が空")
			}
		case "out":
			if s, isStr := t[key].(string); isStr && s == "" {
				missing = append(missing, key+" が空")
			}
		case "transpilers":
			if list, isList := t[key].([]any); isList {
				if len(list) == 0 {
					missing = append(missing, key+" が空")
				}
				for _, item := range list {
					if s, isStr := item.(string); isStr && s == "" {
						missing = append(missing, key+" に空の名前がある")
						break
					}
				}
			}
		}
	}
	if len(missing) > 0 {
		label := where + " "
		if u.Namespace != "" {
			label = fmt.Sprintf("%s（namespace = %q）", where, u.Namespace)
		}
		l.errorf("CFG-12", "%sに %s", label, strings.Join(missing, "、"))
		ok = false
	}
	if ok {
		l.cfg.Units = append(l.cfg.Units, u)
	}
}

// checkOuts は CFG-17: 二つの単位の out が同じか、一方が他方の下にある（docs/config.md「単位の対象」）。
// 比べるのは CleanOut した形。ルートそのものを out にした単位は、ほかのすべての単位と重なる。
func (l *loader) checkOuts() {
	units := l.cfg.Units
	for i := range units {
		for j := i + 1; j < len(units); j++ {
			a, b := CleanOut(units[i].Out), CleanOut(units[j].Out)
			switch {
			case a == b:
				l.errorf("CFG-17", "%sと%sの out が同じ", units[i].Label(), units[j].Label())
			case under(a, b):
				l.errorf("CFG-17", "%sの out が%sの out の下にある", units[i].Label(), units[j].Label())
			case under(b, a):
				l.errorf("CFG-17", "%sの out が%sの out の下にある", units[j].Label(), units[i].Label())
			}
		}
	}
}

// under は相対パス a が b の下（真に深い）にあるか。b が ""（ルート）なら、空でないすべての a が下。
func under(a, b string) bool {
	if a == b {
		return false
	}
	if b == "" {
		return true
	}
	return strings.HasPrefix(a, b+"/")
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
