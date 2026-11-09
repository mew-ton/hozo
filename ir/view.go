package ir

import (
	"bytes"
	"encoding/json"
	"sort"
)

// View はその名前空間から見た IR（docs/ir.md「その名前空間から見た IR」の View）。
// プラグインに渡すもの。Program 全体に「見る側」の情報を足したもの。
type View struct {
	// Version は IR の版。空なら JSON には [Version] を書く。
	Version string `json:"version"`
	// Viewer は見る側の名前空間の FQN。
	Viewer string `json:"viewer"`
	// Config は Conf(viewer)（docs/config.md「名前空間の設定の解決」）。
	Config ViewConfig `json:"config"`
	// Program は全体そのまま（各 op の Signature は op 自身の名前空間から見たもの）。
	// ただし Ops の params が合成する model を、viewer の Namespace.Decls の末尾に足してある。
	Program *Program `json:"program"`
	// Ops は FQN → viewer から見た関数の型。すべての op について。JSON ではキーを FQN の辞書順に出す。
	Ops map[string]*ViewedSignature `json:"ops"`
}

// ViewConfig は見る側の名前空間の設定（docs/ir.md「その名前空間から見た IR」の config）。
type ViewConfig struct {
	Language string   `json:"language"`
	Out      string   `json:"out"`
	Plugins  []string `json:"plugins"`
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

// MarshalJSON は docs/ir.md「JSON」の形で書く。ops のキーは FQN の辞書順。Version が空なら [Version] を書く。
func (v View) MarshalJSON() ([]byte, error) {
	if v.Version == "" {
		v.Version = Version
	}
	type head struct {
		Version string     `json:"version"`
		Viewer  string     `json:"viewer"`
		Config  ViewConfig `json:"config"`
		Program *Program   `json:"program"`
	}
	b, err := marshalStruct(head{v.Version, v.Viewer, v.Config, v.Program})
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
func (c ViewConfig) MarshalJSON() ([]byte, error) {
	type alias ViewConfig
	return marshalStruct(alias(c))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (c *ViewConfig) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, c) }

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
