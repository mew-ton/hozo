// Package config は hozo.toml（設計書 §4.5 を出力の単位の一覧に変えたもの）の読み込み。形と読み方、設定だけで
// 検出できる検査（CFG-7 / CFG-12 / CFG-17 / CFG-21）は docs/config.md。プログラムが要る検査（CFG-6 / CFG-10 / CFG-22）は
// internal/check が、マニフェストと読んだファイルが要る検査（CFG-14〜CFG-16 / CFG-18〜CFG-20 / CFG-23）は internal/project が、
// この Config を見て行う（docs/packages.md「internal/config」の表）。
package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
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
	// Options は [options.<名前>]（docs/config.md「プロジェクト全体のオプション」）: トランスパイラの名前 → プロジェクト全体の
	// オプションのテーブル（書かれたまま）。なければ空（nil ではない）。単位の Options には、これを合わせた後のものが入っている。
	Options map[string]map[string]any
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
	// Options は [emit.options.<名前>]（docs/config.md「オプション」）: トランスパイラの名前 → そのトランスパイラへのオプションの
	// テーブル（TOML の値のまま: 文字列、int64、float64、真偽値、[]any、map[string]any）。書かれていなければ空（nil ではない）。
	// 名前が transpilers にないもの（CFG-21）は入れない。中身（キーと値の型）は確かめない（確かめるのはトランスパイラ）。
	Options map[string]map[string]any
}

// Label は診断で単位を指す言い方（docs/config.md「診断」）: 何番目の [[emit]] かと、その namespace と out。
func (u *Unit) Label() string {
	return fmt.Sprintf("%d 番目の [[emit]]（namespace = %q、out = %q）", u.Index, u.Namespace, u.Out)
}

// Default は hozo.toml がないときの設定（docs/config.md「置き場と探し方」の「設定なし」、docs/lsp.md「プロジェクト」）。
// root をルートにし、[hozo] は既定値、単位はなし。
func Default(root string) *Config {
	return &Config{
		Root:    root,
		Hozo:    HozoTable{Sources: append([]string(nil), DefaultSources...), PluginDirs: []string{}, MaxBindings: DefaultMaxBindings},
		Options: map[string]map[string]any{},
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
// transpilers の重複、ルートの下でない out、options の形）、CFG-12（単位の必須のキーがない、または空）、
// CFG-17（二つの単位の out が同じか入れ子）、CFG-21（options の名前が単位の transpilers にない）。
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
	out := CleanRel(u.Out)
	if namespace == u.Namespace || !strings.HasPrefix(namespace, u.Namespace+".") {
		return out
	}
	rest := strings.ReplaceAll(namespace[len(u.Namespace)+1:], ".", "/")
	if out == "" {
		return rest
	}
	return out + "/" + rest
}

// UnitNamespaces は単位の namespace を、重複なしの辞書順で返す。CFG-10 と、診断の順序を決定的にするためのもの。
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

// CleanRel はルートからの相対パスを比べられる形にする: `/` 区切りにし、`./` や余分な `/` を取り除く
// （docs/config.md「単位の対象」）。先頭の `/` は落とし、ルートそのもの（"." など）は ""。`..` で上へ出る分は残る
// （ルートの外かどうかは [Under] と呼ぶ側が見る）。out、plugin_dirs、glob、書き出すファイルのパス、
// .hozoc-outputs.json のパスを比べる前に、どれもこの一つを通す。
func CleanRel(p string) string {
	p = path.Clean(filepath.ToSlash(p))
	p = strings.TrimPrefix(p, "/")
	if p == "." {
		return ""
	}
	return p
}

// Under は相対パス a（CleanRel した形）が dir（同じ形）の下（真に深い）にあるか。dir が ""（ルート）なら、ルートの
// 外へ出ない、空でないすべての a が下。CFG-17 の入れ子、Dir(U, N) の下かどうか（PLG-1、.hozoc-outputs.json）に使う。
func Under(a, dir string) bool {
	if a == dir || a == "" || a == ".." || strings.HasPrefix(a, "../") {
		return false
	}
	if dir == "" {
		return true
	}
	return strings.HasPrefix(a, dir+"/")
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
		case "options":
			l.loadProjectOptions(value)
		default:
			l.errorf("CFG-7", "知らないキー %s（トップレベルに書けるのは [hozo] と [options.<名前>] と [[emit]] だけ）", key)
		}
	}
	l.mergeProjectOptions()
}

// loadProjectOptions は [options.<名前>]（docs/config.md「プロジェクト全体のオプション」）を cfg.Options に読む。形の決まりは、
// 単位のオプションと同じ（CFG-7）。
func (l *loader) loadProjectOptions(raw any) {
	table, ok := raw.(map[string]any)
	if !ok {
		l.errorf("CFG-7", "options はテーブルでなければならない（%s）", typeName(raw))
		return
	}
	for _, name := range sortedKeys(table) {
		opts, ok := table[name].(map[string]any)
		if !ok {
			l.errorf("CFG-7", "options.%s はテーブルでなければならない（%s）", name, typeName(table[name]))
			continue
		}
		clean, bad := optionValue(opts, "options."+name)
		if bad != "" {
			l.errorf("CFG-7", "%s に日時は書けない（オプションの値は、文字列、整数、小数、真偽値、それらの配列とテーブル）", bad)
			continue
		}
		l.cfg.Options[name] = clean.(map[string]any)
	}
}

// mergeProjectOptions は、プロジェクト全体のオプションを、そのトランスパイラを並べた単位の Options に合わせる: 単位が同じキーを
// 書いていれば、単位の値（キーごと。テーブルの中は合わせない）。どの単位の transpilers にもない名前は CFG-24。
func (l *loader) mergeProjectOptions() {
	names := make([]string, 0, len(l.cfg.Options))
	for name := range l.cfg.Options {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		used := false
		for i := range l.cfg.Units {
			u := &l.cfg.Units[i]
			if !slices.Contains(u.Transpilers, name) {
				continue
			}
			used = true
			merged := map[string]any{}
			for k, v := range l.cfg.Options[name] {
				merged[k] = v
			}
			for k, v := range u.Options[name] {
				merged[k] = v
			}
			u.Options[name] = merged
		}
		if !used {
			l.errorf("CFG-24", "[options.%s] の %s が、どの単位の transpilers にもない", name, name)
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

// loadUnit は index 番目（1 始まり）の [[emit]]（docs/config.md「形」「単位」「オプション」）。知らないキー、値の型違い、
// ルートの下でない out、options の形は CFG-7、namespace / transpilers / out のどれかがない、または空なら CFG-12。
// どちらかがあれば、その単位は Units に入れない。options の名前が transpilers になければ CFG-21（単位は使う）。
func (l *loader) loadUnit(index int, t map[string]any) {
	u := Unit{Index: index, Options: map[string]map[string]any{}}
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
				continue
			}
			u.Out = s
			if reason := badOut(s); reason != "" {
				l.errorf("CFG-7", "%s の out %q は%s（out は、ルートの下のディレクトリをルートからの相対パスで書く）", where, s, reason)
				ok = false
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
		case "options":
			// 中身は transpilers を読んだ後で見る（キーは辞書順に回るので、"options" は "transpilers" より前に来る）
		default:
			l.errorf("CFG-7", "知らないキー %s（%s）", key, where)
			ok = false
		}
	}
	if raw, written := t["options"]; written {
		if !l.loadOptions(where, raw, &u) {
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

// loadOptions は単位の options（docs/config.md「オプション」）を u.Options に読む。options がテーブルでない、
// options.<名前> がテーブルでない、値が日時なら CFG-7 で false（その単位は使わない）。<名前> が単位の transpilers になければ
// CFG-21（その名前のオプションは捨てるが、単位は使う）。
func (l *loader) loadOptions(where string, raw any, u *Unit) bool {
	table, isTable := raw.(map[string]any)
	if !isTable {
		l.errorf("CFG-7", "%s の options はテーブルでなければならない（%s）", where, typeName(raw))
		return false
	}
	ok := true
	for _, name := range sortedKeys(table) {
		opts, isTable := table[name].(map[string]any)
		if !isTable {
			l.errorf("CFG-7", "%s の options.%s はテーブルでなければならない（%s）", where, name, typeName(table[name]))
			ok = false
			continue
		}
		clean, bad := optionValue(opts, "options."+name)
		if bad != "" {
			l.errorf("CFG-7", "%s の %s は日時で、オプションの値に書けない（書けるのは、文字列、整数、小数、真偽値、それらの配列とテーブル）", where, bad)
			ok = false
			continue
		}
		listed := false
		for _, t := range u.Transpilers {
			listed = listed || t == name
		}
		if !listed {
			// CFG-21
			label := where
			if u.Namespace != "" {
				label = fmt.Sprintf("%s（namespace = %q）", where, u.Namespace)
			}
			l.errorf("CFG-21", "%sの [emit.options.%s] の %s が、その単位の transpilers %s にない", label, name, name, quoteList(u.Transpilers))
			continue
		}
		u.Options[name] = clean.(map[string]any)
	}
	return ok
}

// optionValue はオプションの値 v を、JSON にそのまま写せる形にして返す（テーブルの配列は []any にそろえる）。
// 書けない値（日時）があれば、その場所（"options.go.at" の形）を bad に返す。
func optionValue(v any, where string) (clean any, bad string) {
	switch x := v.(type) {
	case string, int64, float64, bool:
		return x, ""
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			c, bad := optionValue(item, fmt.Sprintf("%s[%d]", where, i))
			if bad != "" {
				return nil, bad
			}
			out[i] = c
		}
		return out, ""
	case []map[string]any:
		out := make([]any, len(x))
		for i, item := range x {
			c, bad := optionValue(item, fmt.Sprintf("%s[%d]", where, i))
			if bad != "" {
				return nil, bad
			}
			out[i] = c
		}
		return out, ""
	case map[string]any:
		out := make(map[string]any, len(x))
		for _, k := range sortedKeys(x) {
			c, bad := optionValue(x[k], where+"."+k)
			if bad != "" {
				return nil, bad
			}
			out[k] = c
		}
		return out, ""
	}
	return nil, where // time.Time と、TOML の日付・時刻
}

// quoteList は文字列の列を ["a", "b"] の形にする。
func quoteList(list []string) string {
	quoted := make([]string, len(list))
	for i, s := range list {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// badOut は単位の out が出力先にできない理由を返す（CFG-7。docs/config.md「形」）: 絶対パス、ルートの外へ出るパス、
// ルートそのもの。出力先にできるなら ""。空の out は CFG-12 が報告するので、ここでは数えない。
// ルートの外を許さないのは、hozoc が書いて消すファイルをプロジェクトの中に限るため（docs/cli.md「書き出し」）。ルートそのものを
// 許さないのは、out の下は sources から常に除くので、何も読めなくなるため。
func badOut(out string) string {
	if out == "" {
		return ""
	}
	if filepath.IsAbs(out) || strings.HasPrefix(out, "/") || strings.HasPrefix(out, "\\") || filepath.VolumeName(out) != "" {
		return "絶対パス"
	}
	switch clean := CleanRel(out); {
	case clean == "":
		return "プロジェクトのルートそのもの"
	case clean == ".." || strings.HasPrefix(clean, "../"):
		return "プロジェクトのルートの外"
	}
	return ""
}

// checkOuts は CFG-17: 二つの単位の out が同じか、一方が他方の下にある（docs/config.md「単位の対象」）。
// 比べるのは CleanRel した形。out がルートそのものやルートの外の単位は CFG-7 で Units に入っていない。
func (l *loader) checkOuts() {
	units := l.cfg.Units
	for i := range units {
		for j := i + 1; j < len(units); j++ {
			a, b := CleanRel(units[i].Out), CleanRel(units[j].Out)
			switch {
			case a == b:
				l.errorf("CFG-17", "%sと%sの out が同じ", units[i].Label(), units[j].Label())
			case Under(a, b):
				l.errorf("CFG-17", "%sの out が%sの out の下にある", units[i].Label(), units[j].Label())
			case Under(b, a):
				l.errorf("CFG-17", "%sの out が%sの out の下にある", units[j].Label(), units[i].Label())
			}
		}
	}
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
