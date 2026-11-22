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
	// Entries は、viewer の、単位の対象の op のうち、入口（@entry の付いたデコレータを持つ op）ごとの、求めるケース
	// （docs/check.md「入口ごとの、求めるケース」）。op の宣言順。入口がなければ空（JSON では []）。
	Entries []Entry `json:"entries"`
}

// Entry は、入口の op 一つの、求めるケース（docs/ir.md「その名前空間から見た IR」の Entry）。
type Entry struct {
	// Op は、入口の op の FQN。
	Op string `json:"op"`
	// Normal は、正常の結果が、あるか（never を返す op には、ない）。
	Normal bool `json:"normal"`
	// Blocked は、流れを辿れない理由（通す先の名前空間を出力する、同じ基底の単位が、二つ以上ある）。辿れたなら ""。
	// 空でなければ、Sources と Caught は、空。
	Blocked string `json:"blocked"`
	// Sources は、op が投げうる例外（Op.signature.exceptions の順）の、出どころ。その中は、辿った順。
	Sources []EntrySource `json:"sources"`
	// Caught は、受ける道: 例外を受ける文（try）の中の出どころが投げた例外を、catch が受けて、投げずに終わる道。辿った順。
	Caught []EntrySource `json:"caught"`
}

// EntrySourceKind は、出どころの種類。
type EntrySourceKind string

const (
	// SourceLeaf は、流れの中の、葉の op。
	SourceLeaf EntrySourceKind = "leaf"
	// SourceEffect は、流れの端の、効果。
	SourceEffect EntrySourceKind = "effect"
	// SourceExternal は、流れの端の op（通さない名前空間の op）が、自分で投げる例外。
	SourceExternal EntrySourceKind = "external"
	// SourceBoundary は、呼ぶ先の op の境界（その op に付いたデコレータの outer throws）。
	SourceBoundary EntrySourceKind = "boundary"
	// SourceDecorator は、本体のないデコレータの適用（そのデコレータの throws）。
	SourceDecorator EntrySourceKind = "decorator"
	// SourceArm は、配線の match の、throw のアームと、catch の中の throw。
	SourceArm EntrySourceKind = "arm"
	// SourceNever は、never を返す op。
	SourceNever EntrySourceKind = "never"
	// SourceUnknown は、辿っても、出どころが見つからなかった例外。
	SourceUnknown EntrySourceKind = "unknown"
)

// EntrySource は、出どころ一つ（Entry.Sources）か、受ける道一つ（Entry.Caught）。
type EntrySource struct {
	// Exception は、例外の FQN（受ける道なら、受けられる例外）。
	Exception string          `json:"exception"`
	Kind      EntrySourceKind `json:"kind"`
	// Label は、テストの名前の <出どころ>（docs/check.md「入口ごとの、求めるケース」の「出どころの名前」）。unknown は ""。
	Label string `json:"label"`
	// Name は、出どころの宣言の FQN: 葉・流れの端の op・never を返す op・境界の op・デコレータの付いた op・アームのある配線の op。
	// effect は、効果の FQN。unknown は ""。
	Name string `json:"name"`
	// Decorator は、デコレータの FQN（boundary と decorator。デコレータの本体の中の arm）。それ以外は ""。
	Decorator string `json:"decorator"`
	// Cases は、arm のケース（match のアームが受けるケース。受けた例外の match のアームなら、そのケース）。それ以外は空。
	Cases []string `json:"cases"`
	// Catch は、arm が、catch の中の throw か。
	Catch bool `json:"catch"`
	// Steps は、正常のケースの用意の後に、差し替えに言うこと（順に）。Why が "" のときだけ、意味を持つ。空なら、何も言わなくても、届く。
	Steps []EntryStep `json:"steps"`
	// Why は、差し替えでは、その道に入れない理由（日本語の句）。入れるなら ""。
	Why string `json:"why"`
}

// EntryStepVerb は、差し替えに言うことの種類。
type EntryStepVerb string

const (
	// StepThrows は、例外 Exception を、投げさせる。
	StepThrows EntryStepVerb = "throws"
	// StepNone は、「ない」（か、空の並び）を、返させる。
	StepNone EntryStepVerb = "none"
	// StepIs は、分類 Target.Name の判定に、ケース Case だと、言わせる。
	StepIs EntryStepVerb = "is"
)

// EntryTargetKind は、差し替える相手の種類。
type EntryTargetKind string

const (
	StepLeaf      EntryTargetKind = "leaf"      // 葉の op
	StepEffect    EntryTargetKind = "effect"    // 効果
	StepRead      EntryTargetKind = "read"      // read
	StepState     EntryTargetKind = "state"     // 状態の値
	StepOp        EntryTargetKind = "op"        // 流れの端の op（通さない名前空間の op）
	StepDecorator EntryTargetKind = "decorator" // 本体のないデコレータの適用（Name は、付いた op。Decorator は、デコレータ）
	StepClassify  EntryTargetKind = "classify"  // 分類の判定
)

// EntryStep は、差し替えに言うこと一つ。
type EntryStep struct {
	// Via は、入口の名前空間から、この差し替えを持つ出力の名前空間までに、通った名前空間（順に。入口の名前空間は、含まない）。
	Via  []string        `json:"via"`
	Verb EntryStepVerb   `json:"verb"`
	Kind EntryTargetKind `json:"kind"`
	// Name は、差し替える相手の FQN。Decorator は、kind が decorator のときの、デコレータの FQN。
	Name      string `json:"name"`
	Decorator string `json:"decorator"`
	// Exception は、throws の例外の FQN。Case は、is のケース名。
	Exception string `json:"exception"`
	Case      string `json:"case"`
}

// Entry は、op fqn の Entry。なければ nil。
func (v *View) Entry(fqn string) *Entry {
	for i := range v.Entries {
		if v.Entries[i].Op == fqn {
			return &v.Entries[i]
		}
	}
	return nil
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
		Entries    []Entry                      `json:"entries"`
	}
	b, err := marshalStruct(head{v.Version, v.Viewer, v.Config, v.Files, v.Namespaces, v.Program, v.Entries})
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

// MarshalJSON は docs/ir.md「JSON」の形で書く（空の並びは []）。
func (e Entry) MarshalJSON() ([]byte, error) {
	type alias Entry
	return marshalStruct(alias(e))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (e *Entry) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, e) }

// MarshalJSON は docs/ir.md「JSON」の形で書く（空の並びは []）。
func (s EntrySource) MarshalJSON() ([]byte, error) {
	type alias EntrySource
	return marshalStruct(alias(s))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *EntrySource) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }

// MarshalJSON は docs/ir.md「JSON」の形で書く（空の並びは []）。
func (s EntryStep) MarshalJSON() ([]byte, error) {
	type alias EntryStep
	return marshalStruct(alias(s))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (s *EntryStep) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, s) }
