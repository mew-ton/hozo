// Package plugin はプラグインのホスト側（docs/plugin-protocol.md）。マニフェスト hozo-plugin.toml の読み込み、
// 探索（「探索」）、起動と JSON のやり取り（「起動」）。プラグインの側の SDK は plugin（Go）と sdk/ts。
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// ManifestFileName はマニフェストのファイル名（docs/plugin-protocol.md「マニフェスト」）。
const ManifestFileName = "hozo-plugin.toml"

// Protocol は hozoc が対応する docs/ir.md の version（ir.Version）。マニフェストの protocol と要求の protocol はこの値（MAN-2）。
const Protocol = ir.Version

// Manifest は hozo-plugin.toml（docs/plugin-protocol.md「マニフェスト」）。プラグイン（入れるもの）一つ分。
type Manifest struct {
	Name        string
	Command     []string // 先頭が実行ファイル。文字列で書かれていれば一要素
	Protocol    string
	Vocabulary  []string     // マニフェストからの相対パス。省略時は []
	Transpilers []Transpiler // [[transpiler]]。書いた順。一つもなければ emit を持たない
	Check       Check
	Import      Import
}

// Transpiler は [[transpiler]] 一つ（docs/plugin-protocol.md「マニフェスト」）: プラグインが提供する、名前の付いた変換。
// hozo.toml の単位が transpilers に Name を書いて選ぶ（docs/config.md「単位」）。何を出力の対象に選ぶかは
// マニフェストには書かず、emit の応答の select で返す（同「選択」）。
type Transpiler struct {
	Name        string   // hozo.toml の transpilers に書く名前
	Language    string   // 出力する言語。"go" か "ts"
	Unsupported []string // 意味を保てない記法（デコレータの FQN）。使われていれば CFG-6
}

// Check は [check] テーブル。Enabled なら hozoc check / emit で direction "check" を呼ぶ。
type Check struct {
	Enabled bool
}

// Import は [import] テーブル。Formats が空なら import を持たない。
type Import struct {
	Formats []string
}

// HasEmit は emit を持つかどうか（トランスパイラを一つでも提供するか）。
func (m *Manifest) HasEmit() bool { return len(m.Transpilers) > 0 }

// Transpiler は名前 name のトランスパイラ。なければ nil。
func (m *Manifest) Transpiler(name string) *Transpiler {
	for i := range m.Transpilers {
		if m.Transpilers[i].Name == name {
			return &m.Transpilers[i]
		}
	}
	return nil
}

// HasImport は import を持つかどうか。
func (m *Manifest) HasImport() bool { return len(m.Import.Formats) > 0 }

// rawManifest は TOML の形。知らないキーは MetaData.Undecoded で拾う（MAN-4）。
type rawManifest struct {
	Name       string         `toml:"name"`
	Command    toml.Primitive `toml:"command"`
	Protocol   string         `toml:"protocol"`
	Vocabulary []string       `toml:"vocabulary"`
	Transpiler []struct {
		Name        string   `toml:"name"`
		Language    string   `toml:"language"`
		Unsupported []string `toml:"unsupported"`
	} `toml:"transpiler"`
	Check struct {
		Enabled bool `toml:"enabled"`
	} `toml:"check"`
	Import struct {
		Formats []string `toml:"formats"`
	} `toml:"import"`
}

// LoadManifest は path のマニフェストを読む。診断は MAN-4（TOML の誤り、知らないキー、command が空、値の型違い、
// [[transpiler]] に name か language がない、language が go でも ts でもない、同じ名前の [[transpiler]] が二つ）と
// MAN-2（protocol が Protocol と違う）。MAN-4 なら Manifest は nil。MAN-2 だけなら Manifest も返す。
// 設計書 §4.6 の [emit]（decorate / entry / stop）とトップレベルの language は持たないので、書いてあれば知らないキー（MAN-4）。
func LoadManifest(path string) (*Manifest, []diag.Diagnostic) {
	var diags []diag.Diagnostic
	errorf := func(code, format string, args ...any) {
		diags = append(diags, diag.Diagnostic{Severity: diag.Error, Code: code, Message: fmt.Sprintf(format, args...), File: path})
	}

	data, err := os.ReadFile(path)
	if err != nil {
		errorf("MAN-4", "マニフェストを読めない: %v", err)
		return nil, diags
	}
	var raw rawManifest
	meta, err := toml.Decode(string(data), &raw)
	if err != nil {
		if pe, ok := err.(toml.ParseError); ok {
			errorf("MAN-4", "マニフェストの %d 行目に誤りがある: %s", pe.Position.Line, pe.Message)
		} else {
			errorf("MAN-4", "マニフェストを読めない: %v", err)
		}
		return nil, diags
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			if k.String() == "command" { // Primitive はあとで読む
				continue
			}
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		reported := ""
		for _, k := range keys {
			if reported != "" && strings.HasPrefix(k, reported+".") {
				continue // 知らないテーブルの中のキーは、そのテーブルで一度だけ報告する
			}
			errorf("MAN-4", "マニフェストに知らないキー %s がある", k)
			reported = k
		}
	}
	if raw.Name == "" {
		errorf("MAN-4", "マニフェストに name がない")
	}

	m := &Manifest{Name: raw.Name, Protocol: raw.Protocol, Vocabulary: raw.Vocabulary}
	if m.Vocabulary == nil {
		m.Vocabulary = []string{}
	}
	if !meta.IsDefined("command") {
		errorf("MAN-4", "マニフェストに command がない")
	} else {
		var s string
		var list []string
		if err := meta.PrimitiveDecode(raw.Command, &s); err == nil {
			if s == "" {
				errorf("MAN-4", "マニフェストの command が空")
			} else {
				m.Command = []string{s}
			}
		} else if err := meta.PrimitiveDecode(raw.Command, &list); err == nil {
			if len(list) == 0 || list[0] == "" {
				errorf("MAN-4", "マニフェストの command が空")
			} else {
				m.Command = list
			}
		} else {
			errorf("MAN-4", "マニフェストの command は文字列か文字列の配列でなければならない")
		}
	}
	seen := map[string]bool{}
	for i, t := range raw.Transpiler {
		switch {
		case t.Name == "":
			errorf("MAN-4", "マニフェストの %d 番目の [[transpiler]] に name がない", i+1)
			continue
		case seen[t.Name]:
			errorf("MAN-4", "マニフェストに同じ名前の [[transpiler]] %s が二つある", t.Name)
			continue
		}
		seen[t.Name] = true
		switch t.Language {
		case "go", "ts":
		case "":
			errorf("MAN-4", "マニフェストの [[transpiler]] %s に language がない", t.Name)
			continue
		default:
			errorf("MAN-4", "マニフェストの [[transpiler]] %s の language は \"go\" か \"ts\" でなければならない（%q）", t.Name, t.Language)
			continue
		}
		m.Transpilers = append(m.Transpilers, Transpiler{Name: t.Name, Language: t.Language, Unsupported: orEmpty(t.Unsupported)})
	}
	m.Check.Enabled = raw.Check.Enabled
	m.Import.Formats = orEmpty(raw.Import.Formats)

	if len(diags) > 0 {
		return nil, diags
	}
	if m.Protocol != Protocol {
		errorf("MAN-2", "プラグイン %s の protocol %q は hozoc の %q と違う", m.Name, m.Protocol, Protocol)
	}
	return m, diags
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ResolveCommand は Command[0] を実行ファイルのパスにする（docs/plugin-protocol.md「マニフェスト」）。
// パスの区切りを含めばマニフェストのディレクトリ dir からの相対パス（絶対ならそのまま）、含まなければ PATH から探す名前のまま返す。
func (m *Manifest) ResolveCommand(dir string) string {
	exe := m.Command[0]
	if filepath.Base(exe) == exe {
		return exe
	}
	if filepath.IsAbs(exe) {
		return exe
	}
	return filepath.Join(dir, exe)
}
