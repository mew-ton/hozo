package ir

// Version は IR の構造の版（docs/ir.md「Program」の version）。構造が変わったら上げる。
const Version = "0.26"

// VersionAccepts は、版 host の hozoc が、版 plugin を話すプラグインを動かせるか（docs/plugin-protocol.md「版の合わせ方」）:
// 大きい版が 0 の間（1.0 を名乗る前。docs/ir.md「版」の「今は、0.x」）は、二つの版が、ぴったり同じ。1.0 からは、
// 大きい版が同じで、plugin の小さい版が、host の小さい版以下。版は「大きい版.小さい版」（十進の数二つ）で、そう読めなければ false。
// hozoc は、マニフェストの protocol を plugin に、プラグインは、要求の protocol を host に渡して、同じ決まりで確かめる。
func VersionAccepts(host, plugin string) bool {
	hMajor, hMinor, ok := splitVersion(host)
	if !ok {
		return false
	}
	pMajor, pMinor, ok := splitVersion(plugin)
	if !ok || hMajor != pMajor {
		return false
	}
	if hMajor == 0 {
		return pMinor == hMinor
	}
	return pMinor <= hMinor
}

// splitVersion は「大きい版.小さい版」を、二つの数に分ける。数字だけの二つの部分でなければ ok は false。
func splitVersion(v string) (major, minor int, ok bool) {
	dot := -1
	for i := 0; i < len(v); i++ {
		switch c := v[i]; {
		case c == '.' && dot < 0:
			dot = i
		case c < '0' || c > '9':
			return 0, 0, false
		}
	}
	if dot <= 0 || dot == len(v)-1 {
		return 0, 0, false
	}
	number := func(s string) (int, bool) {
		n := 0
		for i := 0; i < len(s); i++ {
			n = n*10 + int(s[i]-'0')
			if n > 1<<30 {
				return 0, false
			}
		}
		return n, true
	}
	major, ok1 := number(v[:dot])
	minor, ok2 := number(v[dot+1:])
	return major, minor, ok1 && ok2
}

// Program は IR 全体（docs/ir.md「Program」）。
//
// Namespaces の先頭はグローバル（Name が ""）の Namespace。並びは docs/ir.md「原則」の
// 「宣言の順序は出現順」に従う。
type Program struct {
	// Version はこの文書の版。空なら JSON には [Version] を書く。
	Version string `json:"version"`
	// Namespaces は名前空間の列。グローバルは name "" として先頭。
	Namespaces []Namespace `json:"namespaces"`
	// Binds はすべての bind 宣言。
	Binds []Bind `json:"binds"`
}

// Lookup は FQN の宣言を引く（docs/packages.md「`ir`（公開）」）。なければ nil。
// 戻り値の動的な型は *Model、*Scalar などのポインタで、書き換えると Program に反映する。
func (p *Program) Lookup(fqn string) Decl {
	if p == nil {
		return nil
	}
	for i := range p.Namespaces {
		for _, d := range p.Namespaces[i].Decls {
			if d != nil && d.Common().Name == fqn {
				return d
			}
		}
	}
	return nil
}

// Namespace は FQN の名前空間を引く（docs/packages.md「`ir`（公開）」）。グローバルは ""。
// なければ nil。戻り値は Namespaces の要素を指す。
func (p *Program) Namespace(fqn string) *Namespace {
	if p == nil {
		return nil
	}
	for i := range p.Namespaces {
		if p.Namespaces[i].Name == fqn {
			return &p.Namespaces[i]
		}
	}
	return nil
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。Version が空なら [Version] を書く。
func (p Program) MarshalJSON() ([]byte, error) {
	type alias Program
	if p.Version == "" {
		p.Version = Version
	}
	return marshalStruct(alias(p))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。版の照合はしない
// （docs/plugin-protocol.md「要求と応答」が持ち主）。
func (p *Program) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, p) }

// Namespace は一つの名前空間（docs/ir.md「Program」の Namespace）。
// 複数の場所に書かれた同じ名前空間は一つにまとまっている（docs/ir.md「正規化」）。
type Namespace struct {
	// Name は FQN（"server.admin"）。グローバルは ""。
	Name string `json:"name"`
	// Description は namespace ブロックの直前の DocComment。複数あれば最初のもの。
	Description *Doc `json:"description,omitempty"`
	// Decls は宣言の列（出現順）。
	Decls []Decl `json:"decls"`
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (n Namespace) MarshalJSON() ([]byte, error) {
	type alias Namespace
	return marshalStruct(alias(n))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (n *Namespace) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, n) }

// Span は宣言や文の位置（docs/ir.md「Program」の Span）。
type Span struct {
	// File は三つの形のどれか: ユーザーのファイルは hozo.toml からの相対パス、
	// 標準の語彙は "hozo:vocab/<名前>.hozo"、
	// プラグインの語彙は "hozo:plugin/<プラグイン名>/<マニフェストからの相対パス>"。
	File  string `json:"file"`
	Start Pos    `json:"start"`
	End   Pos    `json:"end"`
}

// Pos はファイルの中の位置（docs/ir.md「Program」の Pos）。
// Line と Col は 1 始まり。Col は UTF-8 のバイトではなく Unicode のコードポイント数。
type Pos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// Doc は DocComment の内容（docs/ir.md「Doc」）。
type Doc struct {
	// Summary は最初の段落（Markdown）。
	Summary string `json:"summary"`
	// Body は全文（Markdown。Summary を含む。タグの行は含まない）。
	Body string `json:"body"`
	// Params は @param。Name は解決済み（op の引数名）。
	Params []DocParam `json:"params"`
	// Throws は @throws。Exception は FQN。
	Throws []DocThrows `json:"throws"`
	// See は @see の FQN。
	See []string `json:"see"`
}

// DocParam は Doc の @param 一つ（docs/ir.md「Doc」の params）。
type DocParam struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// DocThrows は Doc の @throws 一つ（docs/ir.md「Doc」の throws）。
type DocThrows struct {
	Exception string `json:"exception"`
	Text      string `json:"text"`
}

// MarshalJSON は docs/ir.md「JSON」の形で書く。
func (d Doc) MarshalJSON() ([]byte, error) {
	type alias Doc
	return marshalStruct(alias(d))
}

// UnmarshalJSON は docs/ir.md「JSON」の形を読む。
func (d *Doc) UnmarshalJSON(data []byte) error { return unmarshalStruct(data, d) }

// Decl は宣言（docs/ir.md「Program」の Decl）。
// 具体型は *Model、*Scalar、*Union、*State、*Exception、*Effect、*Read、*Classify、*Decorator、*Op。
// JSON では kind で分ける。
type Decl interface {
	// Kind は JSON の判別子（"model"、"scalar"、…）。
	Kind() string
	// Common はすべての宣言に共通の部分。
	Common() *DeclBase
	isDecl()
}

// DeclBase はすべての宣言に共通の部分（docs/ir.md「Program」の DeclBase）。
// kind は Go の型で表し、フィールドには持たない。
type DeclBase struct {
	// Name は FQN。
	Name string `json:"name"`
	// Description は宣言の直前の DocComment。
	Description *Doc `json:"description,omitempty"`
	// Decorators は付いたデコレータ（書かれた順。docs/ir.md「デコレータの適用」:
	// 最後の要素が最も内側で、最初に適用される）。
	Decorators []Application `json:"decorators"`
	// Span は宣言の位置。
	Span Span `json:"span"`
}

// Common は自分自身を返す。
func (b *DeclBase) Common() *DeclBase { return b }

var declTable = map[string]func([]byte) (Decl, error){
	"model":     asPointer[Decl, Model],
	"scalar":    asPointer[Decl, Scalar],
	"union":     asPointer[Decl, Union],
	"state":     asPointer[Decl, State],
	"exception": asPointer[Decl, Exception],
	"effect":    asPointer[Decl, Effect],
	"read":      asPointer[Decl, Read],
	"classify":  asPointer[Decl, Classify],
	"decorator": asPointer[Decl, Decorator],
	"op":        asPointer[Decl, Op],
}

// UnmarshalDecl は kind で分けて Decl を読む（docs/ir.md「JSON」）。
func UnmarshalDecl(data []byte) (Decl, error) {
	return decodeUnion("decl", data, declTable)
}

// IndexOf は、型 t が、インデックスの model（docs/types.md「インデックスの model」）への ref なら、そのインデックス。
// そうでなければ nil（optional、array、case の中は見ない: 見るのは、その型そのもの）。
func (p *Program) IndexOf(t Type) *ModelIndex {
	r, ok := t.(RefType)
	if !ok {
		return nil
	}
	if m, isModel := p.Lookup(r.Target).(*Model); isModel {
		return m.Index
	}
	return nil
}
