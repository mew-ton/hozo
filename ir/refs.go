package ir

// ReadRef は reads の要素（docs/ir.md「op」の ReadRef）。名前で書いたか、直に書いたかを要素ごとに持つ
// （docs/check.md「直に書く要素」）。具体型は値型の [NamedRead]（kind "named"）と [DirectRead]（kind "direct"）で、
// == で比べられ、写像のキーにできる。
type ReadRef interface {
	// Kind は JSON の判別子（"named" か "direct"）。
	Kind() string
	// String は表示のための文字列: 名前で書いた要素は read の宣言の FQN、直に書いた要素は状態の FQN。
	String() string
	isReadRef()
}

// NamedRead は名前で書いた reads の要素（`reads userByEmail`）。
type NamedRead struct {
	// Read は read の宣言の FQN。読む状態は、その宣言の State（[Program.ReadState]）。
	Read string `json:"read"`
}

// DirectRead は直に書いた reads の要素（`reads users`）。
type DirectRead struct {
	// State は基底の状態の FQN。
	State string `json:"state"`
}

var (
	_ ReadRef = NamedRead{}
	_ ReadRef = DirectRead{}
)

// Kind は "named"。
func (NamedRead) Kind() string { return "named" }

// Kind は "direct"。
func (DirectRead) Kind() string { return "direct" }

// String は read の宣言の FQN。
func (r NamedRead) String() string { return r.Read }

// String は状態の FQN。
func (r DirectRead) String() string { return r.State }

func (NamedRead) isReadRef()  {}
func (DirectRead) isReadRef() {}

// MarshalJSON は kind を付けて書く。
func (r NamedRead) MarshalJSON() ([]byte, error) {
	type alias NamedRead
	return marshalKind("named", alias(r))
}

// MarshalJSON は kind を付けて書く。
func (r DirectRead) MarshalJSON() ([]byte, error) {
	type alias DirectRead
	return marshalKind("direct", alias(r))
}

var readRefTable = map[string]func([]byte) (ReadRef, error){
	"named":  asValue[ReadRef, NamedRead],
	"direct": asValue[ReadRef, DirectRead],
}

// UnmarshalReadRef は kind で分けて ReadRef を読む（docs/ir.md「JSON」）。
func UnmarshalReadRef(data []byte) (ReadRef, error) {
	return decodeUnion("read ref", data, readRefTable)
}

// EffectRef は effects の要素（docs/ir.md「op」の EffectRef）。名前で書いたか、直に書いたかを要素ごとに持つ
// （docs/check.md「直に書く要素」: 直に書いた効果と、同じ宛先の宣言は、別の効果）。具体型は値型の [NamedEffect]
// （kind "named"）と [DirectEffect]（kind "direct"）で、== で比べられ、写像のキーにできる。
type EffectRef interface {
	// Kind は JSON の判別子（"named" か "direct"）。
	Kind() string
	// String は表示のための文字列: 名前で書いた要素は effect の宣言の FQN、直に書いた要素は "<状態の FQN>.<操作>"。
	String() string
	isEffectRef()
}

// NamedEffect は名前で書いた effects の要素（`effects addUser`）。
type NamedEffect struct {
	// Effect は effect の宣言の FQN。
	Effect string `json:"effect"`
}

// DirectEffect は直に書いた effects の要素（`effects users.post`）: 基底の状態の組み込みの効果。
type DirectEffect struct {
	// State は基底の状態の FQN。
	State     string    `json:"state"`
	Operation Operation `json:"operation"`
}

var (
	_ EffectRef = NamedEffect{}
	_ EffectRef = DirectEffect{}
)

// Kind は "named"。
func (NamedEffect) Kind() string { return "named" }

// Kind は "direct"。
func (DirectEffect) Kind() string { return "direct" }

// String は effect の宣言の FQN。
func (e NamedEffect) String() string { return e.Effect }

// String は "<状態の FQN>.<操作>"。
func (e DirectEffect) String() string { return e.State + "." + string(e.Operation) }

func (NamedEffect) isEffectRef()  {}
func (DirectEffect) isEffectRef() {}

// MarshalJSON は kind を付けて書く。
func (e NamedEffect) MarshalJSON() ([]byte, error) {
	type alias NamedEffect
	return marshalKind("named", alias(e))
}

// MarshalJSON は kind を付けて書く。
func (e DirectEffect) MarshalJSON() ([]byte, error) {
	type alias DirectEffect
	return marshalKind("direct", alias(e))
}

var effectRefTable = map[string]func([]byte) (EffectRef, error){
	"named":  asValue[EffectRef, NamedEffect],
	"direct": asValue[EffectRef, DirectEffect],
}

// UnmarshalEffectRef は kind で分けて EffectRef を読む（docs/ir.md「JSON」）。
func UnmarshalEffectRef(data []byte) (EffectRef, error) {
	return decodeUnion("effect ref", data, effectRefTable)
}

// NamedReads は read の宣言の FQN の列から、名前で書いた要素の列を作る。
func NamedReads(fqns ...string) []ReadRef {
	out := make([]ReadRef, len(fqns))
	for i, f := range fqns {
		out[i] = NamedRead{Read: f}
	}
	return out
}

// StateReads は基底の状態の FQN の列から、直に書いた要素の列を作る。
func StateReads(states ...string) []ReadRef {
	out := make([]ReadRef, len(states))
	for i, s := range states {
		out[i] = DirectRead{State: s}
	}
	return out
}

// NamedEffects は effect の宣言の FQN の列から、名前で書いた要素の列を作る。
func NamedEffects(fqns ...string) []EffectRef {
	out := make([]EffectRef, len(fqns))
	for i, f := range fqns {
		out[i] = NamedEffect{Effect: f}
	}
	return out
}

// ReadState は reads の要素 r が読む基底の状態の FQN: 名前で書いた要素なら read の宣言の State、直に書いた要素なら
// その State（docs/check.md「直に書く要素」）。宣言がなければ ""。
func (p *Program) ReadState(r ReadRef) string {
	switch x := r.(type) {
	case DirectRead:
		return x.State
	case NamedRead:
		if d, ok := p.Lookup(x.Read).(*Read); ok {
			return d.State
		}
	}
	return ""
}

// ReadStates は要素ごとの [Program.ReadState] を同じ順に並べ、重複を最初の出現に寄せたもの（宣言のない要素は飛ばす）。
// op が直に読む状態（docs/check.md「デコレータが運ぶ reads と effects」）は p.ReadStates(o.DirectReads())。
func (p *Program) ReadStates(rs []ReadRef) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range rs {
		if s := p.ReadState(r); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// EffectWrites は効果 e が書き込む基底の状態の FQN: 名前で書いた要素なら effect の宣言の Writes、直に書いた要素なら
// その State（docs/check.md「直に書く要素」）。宣言がなければ nil。
func (p *Program) EffectWrites(e EffectRef) []string {
	switch x := e.(type) {
	case DirectEffect:
		return []string{x.State}
	case NamedEffect:
		if d, ok := p.Lookup(x.Effect).(*Effect); ok {
			return d.Writes
		}
	}
	return nil
}
