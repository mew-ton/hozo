package ir

import (
	"bytes"
	"encoding/json"
	"sort"
)

// View はその名前空間から見た IR（docs/ir.md「その名前空間から見た IR」の View）。
// トランスパイラに渡すもの。Program 全体に「見る側」の情報を足したもの。単位（docs/config.md「単位」）と、
// その単位が出力する名前空間の組ごとに一つ。
type View struct {
	// Version は IR の版。空なら JSON には [Version] を書く。
	Version string `json:"version"`
	// Viewer は見る側の名前空間の FQN。
	Viewer string `json:"viewer"`
	// Config は、この View の単位での viewer の設定。
	Config NamespaceConfig `json:"config"`
	// Files は、この View の単位の in に合ったユーザーのファイルと、読み込んだすべての語彙のファイル（docs/config.md「in」）。
	// Span.File と同じ形で、ユーザーのファイルを辞書順に、その後に語彙のファイルを辞書順に。in を省いた単位なら、
	// 読み込んだすべてのファイル。viewer の宣言のうち Span.File がここにあるものが、単位の対象の宣言（[View.InUnit]）。
	Files []string `json:"files"`
	// Namespaces は出力のあるすべての名前空間（viewer を含む）。キーは FQN。JSON ではキーを辞書順に出す。
	// 値は、その名前空間を出力する単位ごとの設定（設定の順）。トランスパイラが別の名前空間（と別の単位）の言語と
	// 出力先を知るため。
	Namespaces map[string][]NamespaceConfig `json:"namespaces"`
	// Program は全体そのまま（各 op の Signature は op 自身の名前空間から見たもの）。
	// ただし Ops の params が合成する model を、viewer の Namespace.Decls の末尾に足してある。
	Program *Program `json:"program"`
	// Ops は FQN → viewer から見た関数の型。すべての op について。JSON ではキーを FQN の辞書順に出す。
	Ops map[string]*ViewedSignature `json:"ops"`
}

// NamespaceConfig は単位 U での名前空間 N の設定（docs/ir.md「その名前空間から見た IR」の NamespaceConfig）。
type NamespaceConfig struct {
	// Index は U が何番目の [[emit]] か（1 始まり）。診断のメッセージと、単位どうしを見分けるのに使う。
	Index int `json:"index"`
	// Language は U の言語（U の基底の language。docs/config.md「基底と相乗り」）。
	Language string `json:"language"`
	// Base は U の基底のトランスパイラの名前（同上）。
	Base string `json:"base"`
	// Out は U の out。
	Out string `json:"out"`
	// Dir は Dir(U, N)。U での N のファイルを置くディレクトリ（ルートからの相対パス、`/` 区切り。docs/config.md「単位の対象」）。
	Dir string `json:"dir"`
	// Transpilers は U の transpilers（書かれた順）。
	Transpilers []string `json:"transpilers"`
	// Options は U の options（docs/config.md「オプション」）。キーはトランスパイラの名前、値は hozo.toml のテーブルを
	// JSON に写したもの。オプションを書いていないトランスパイラのキーはない。JSON ではキーを辞書順に出す（入れ子のテーブルも）。
	Options map[string]map[string]any `json:"options"`
	// Seams は U の継ぎ目（docs/config.md「継ぎ目」）。キーは継ぎ目を持つデコレータの FQN、値はそれをマニフェストの seams で
	// 宣言した U のトランスパイラの名前。JSON ではキーを辞書順に出す。なければ {}。
	Seams map[string]string `json:"seams"`
}

// ViewedSignature は viewer から見た op の関数の型（docs/ir.md「その名前空間から見た IR」の ViewedSignature）。
// Signature は Sig(O, viewer)。
type ViewedSignature struct {
	Signature
	// Params は bind で埋まる位置（docs/check.md「bind」の Pos(b, O)）を取り除いた引数。
	// 位置が model のフィールドなら、そのフィールドを除いた model を
	// `<viewer>.<op 名>$<引数名>` の FQN（SynthesizedFrom は op の FQN）で合成し、ref にする。
	Params []OpParam `json:"params"`
	// Bound は取り除いた位置。
	Bound []BoundPosition `json:"bound"`
}

// Signature は op の FQN から viewer から見た関数の型を引く（docs/packages.md「`ir`（公開）」）。なければ nil。
func (v *View) Signature(op string) *ViewedSignature {
	if v == nil {
		return nil
	}
	return v.Ops[op]
}

// InUnit は、FQN fqn が viewer の名前空間の宣言で、この View の単位の対象か（docs/config.md「in」）:
// 宣言の Span.File が Files にある。viewer の宣言でなければ false。
func (v *View) InUnit(fqn string) bool {
	if v == nil || v.Program == nil {
		return false
	}
	ns := v.Program.Namespace(v.Viewer)
	if ns == nil {
		return false
	}
	for _, d := range ns.Decls {
		if d == nil || d.Common().Name != fqn {
			continue
		}
		file := d.Common().Span.File
		for _, f := range v.Files {
			if f == file {
				return true
			}
		}
		return false
	}
	return false
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。namespaces と ops のキーは FQN の辞書順。
// 空の namespaces は {} で出す。Version が空なら [Version] を書く。
func (v View) MarshalJSON() ([]byte, error) {
	if v.Version == "" {
		v.Version = Version
	}
	type head struct {
		Version    string                       `json:"version"`
		Viewer     string                       `json:"viewer"`
		Config     NamespaceConfig              `json:"config"`
		Files      []string                     `json:"files"`
		Namespaces map[string][]NamespaceConfig `json:"namespaces"` // encoding/json は写像のキーを辞書順に書く
		Program    *Program                     `json:"program"`
	}
	b, err := marshalStruct(head{v.Version, v.Viewer, v.Config, v.Files, v.Namespaces, v.Program})
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(v.Ops))
	for k := range v.Ops {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.Write(b[:len(b)-1]) // drop the closing brace
	buf.WriteString(`,"ops":{`)
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(quote(k))
		buf.WriteByte(':')
		sig, err := json.Marshal(v.Ops[k])
		if err != nil {
			return nil, err
		}
		buf.Write(sig)
	}
	buf.WriteString("}}")
	return buf.Bytes(), nil
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。版の照合はしない。
func (v *View) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, v) }

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (c NamespaceConfig) MarshalJSON() ([]byte, error) {
	type alias NamespaceConfig
	return marshalStruct(alias(c))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (c *NamespaceConfig) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, c) }

// MarshalJSON は Signature のフィールドの後に params と bound を書く。
func (s ViewedSignature) MarshalJSON() ([]byte, error) {
	type sig Signature // Signature の MarshalJSON が昇格しないように
	type alias struct {
		sig
		Params []OpParam       `json:"params"`
		Bound  []BoundPosition `json:"bound"`
	}
	return marshalStruct(alias{sig(s.Signature), s.Params, s.Bound})
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *ViewedSignature) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }
