// Package dbvocab は、標準の語彙 db（docs/vocab.md「`db` の語彙」）の、言語にも保存先にも依らない読み解き（docs/plugin-db.md）。
//
// 仕様（ir.Program）の名前空間一つから、表の状態、表の model の列、db のデコレータを付けた read と効果を読み、書いたものの
// 整合の検査（DB-*。同「診断」）を返す。しまい方（列に、どうしまうか）は、読まない: 語彙を出すプラグインが読む。読むのは、仕様だけ: 単位にも、言語にも、保存先にも、依らない。
//
// 語彙のプラグイン db（plugins/db）は、ここの診断を、そのまま返す。db の語彙を出すプラグイン（plugins/go-sqlite）は、同じ
// 読み解きの結果（Reading）から、出力を作る（docs/decisions.md「決めたこと」428: 同じ読み解きを、語彙のプラグインと、
// 言語のプラグインで、分け合うのは、ライブラリで行う）。
package dbvocab

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// Namespace は、この語彙の名前空間（マニフェストの [read] の namespaces。docs/plugin-db.md「マニフェスト」）。
const Namespace = "db"

// 語彙のデコレータの FQN（docs/vocab.md「`db` の語彙」）。
const (
	Table       = "db.table"
	Text        = "db.text"
	Int         = "db.int"
	Float       = "db.float"
	Bool        = "db.bool"
	Datetime    = "db.datetime"
	Date        = "db.date"
	Time        = "db.time"
	JSON        = "db.json"
	Key         = "db.key"
	Unique      = "db.unique"
	Ref         = "db.ref"
	Find        = "db.find"
	List        = "db.list"
	Where       = "db.where"
	Latest      = "db.latest"
	Insert      = "db.insert"
	Update      = "db.update"
	Delete      = "db.delete"
	Conflict    = "db.conflict"
	Transaction = "db.transaction"
)

// 診断の ID（docs/plugin-db.md「診断」）。
const (
	CodeElement     = "DB-1"  // @db.table の状態の要素の型が、model でないか、インデックスの model
	CodeShape       = "DB-2"  // @db.table の状態の形が list でない
	CodeTable       = "DB-3"  // read の reads の状態か、効果の宛先の状態が、N の表の状態でない
	CodeName        = "DB-4"  // 列を名指しするものが、表の model のフィールドにない
	CodeParams      = "DB-5"  // 引数が、デコレータの言うものと合わない
	CodeReturns     = "DB-6"  // read の結果の型が、デコレータの言うものと合わない
	CodeTarget      = "DB-7"  // 効果のデコレータが、効果の宛先と合わない
	CodeEffect      = "DB-8"  // 一つの効果が、効果のデコレータを二つ以上持つ
	CodeSingle      = "DB-9"  // @db.find の read が、一行に決まらない
	CodeUpdate      = "DB-10" // @db.update の表の model が、キーか、キーでない列を持たない
	CodeRef         = "DB-11" // @db.ref の指す先が、一つに決まらない
	CodeBoth        = "DB-12" // 一つの read が、@db.find と @db.list の両方を持つ
	CodeNeither     = "DB-13" // @db.where か @db.latest の付いた read が、@db.find も @db.list も持たない
	CodeTwice       = "DB-14" // 一つの read の条件が、同じ列を二度名指ししている
	CodeLiteral     = "DB-15" // @db.where のリテラルが、列の型に渡せない
	CodeDeleteAll   = "DB-16" // @db.delete の効果が、引数を一つも持たない
	CodeRow         = "DB-17" // @db.update の効果の行の遷移の、後のケースが、前のケースの並びにない
	CodeConflictOn  = "DB-18" // @db.conflict を付けた効果が、@db.insert も @db.update も持たない
	CodeConflictExc = "DB-19" // @db.conflict の例外が、効果の throws にないか、引数を持つ
)

// Column は列一つ: 表の model のフィールド一つ（docs/plugin-db.md「表」「キーと参照」）。しまい方（@db.text など）は、持たない:
// 読むのは、語彙を出すトランスパイラ（同「しまい方」）。Model.Decl.Fields の、同じ位置のフィールドが、そのデコレータを持つ。
type Column struct {
	// Name は、列の名前（= フィールドの名前）。Type は、フィールドの型（省略可能なら optional のまま）。
	Name string
	Type ir.Type
	// Nullable は、フィールドの型が省略可能（T?）: 列は、値がなくてよい。
	Nullable bool
	// Key と Unique は、@db.key と @db.unique。
	Key, Unique bool
	// Ref は、@db.ref の指す model の FQN（なければ ""）。RefTable と RefColumn は、その指す先の表と列（決まらなければ ""。DB-11）。
	Ref, RefTable, RefColumn string
}

// Model は、表の model 一つ。
type Model struct {
	Decl    *ir.Model
	Columns []*Column
}

// Keys は @db.key の付いた列（フィールドの順）。
func (m *Model) Keys() []*Column {
	var out []*Column
	for _, c := range m.Columns {
		if c.Key {
			out = append(out, c)
		}
	}
	return out
}

// Column は、名前が name の列。なければ nil。
func (m *Model) Column(name string) *Column {
	for _, c := range m.Columns {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// TableState は、表の状態一つ（docs/plugin-db.md「表」）: 表の名前は、状態の名前。
type TableState struct {
	State *ir.State
	Name  string
	Model *Model
}

// Condition は、条件一つ: 列 Column が、引数 Param（read か、@db.delete の効果の引数）か、リテラル Literal（@db.where）と同じ。
type Condition struct {
	Column  *Column
	Param   *ir.OpParam // 引数の条件。@db.where なら nil
	Literal ir.Literal  // @db.where のリテラル。引数の条件なら nil
}

// ReadDecl は、@db.find か @db.list を付けた read 一つと、そのデコレータの言うこと（docs/plugin-db.md「read」）。
type ReadDecl struct {
	Decl *ir.Read
	// Apps は、付いた db のデコレータ（@db.find / @db.list、@db.where、@db.latest の順。@db.where は一度だけ）。
	Apps []ir.Application
	// Kind は @db.find か @db.list の FQN。
	Kind  string
	Table *TableState
	// Conds は条件（引数を宣言の順に、その後ろに @db.where を書いた順に）。Latest は @db.latest の列（付いていなければ nil）。
	Conds  []Condition
	Latest *Column
	// OK は、食い違いがないか。
	OK bool
}

// EffectDecl は、効果のデコレータ（@db.insert / @db.update / @db.delete）を、ちょうど一つ付けた効果と、その言うこと（docs/plugin-db.md「効果」）。
type EffectDecl struct {
	Decl *ir.Effect
	// Kind は、ただ一つの効果のデコレータの FQN。
	Kind  string
	Table *TableState
	// Conds は、@db.delete の条件（引数を宣言の順に）。
	Conds []Condition
	// Conflict は、@db.conflict が名指しした例外の FQN。付けていないか、使えなければ ""。
	Conflict string
	// OK は、食い違いがないか。
	OK bool
}

// Reading は、名前空間一つの読み解き（docs/plugin-db.md「対象」の N）。
type Reading struct {
	// Name は、名前空間 N の名前。
	Name string
	// Marked は、@db.table の付いた N の状態の数（診断を返して、表にならなかったものを含む）。
	Marked int
	// Tables は、表の状態（宣言順）。Models は、表の model（表の状態の順。同じ model は一度）。
	Tables []*TableState
	Models []*Model
	// Reads と Effects は、db のデコレータの言うことが決まった read と効果（FQN → 読んだもの）。
	Reads   map[string]*ReadDecl
	Effects map[string]*EffectDecl
	// Errors は、この名前空間の診断（DB-*）。
	Errors []plugin.Diagnostic

	p       *ir.Program
	byState map[string]*TableState
	byModel map[string]*Model
	marked  map[string]bool
}

// TableOf は、FQN が state の、表の状態。表の状態でなければ nil。
func (r *Reading) TableOf(state string) *TableState { return r.byState[state] }

// ModelOf は、FQN が model の、表の model。表の model でなければ nil。
func (r *Reading) ModelOf(model string) *Model { return r.byModel[model] }

func (r *Reading) errorf(code, at, format string, args ...any) {
	r.Errors = append(r.Errors, plugin.Diagnostic{Code: code, At: at, Message: fmt.Sprintf(format, args...)})
}

// Check は、仕様 p の、すべての名前空間の読み解きの診断（DB-*。docs/plugin-db.md「診断」）。順は、名前空間の IR の順、その中は
// ReadNamespace の順。同じ診断は、一度だけ。
func Check(p *ir.Program) []plugin.Diagnostic {
	if p == nil {
		return nil
	}
	var out []plugin.Diagnostic
	seen := map[plugin.Diagnostic]bool{}
	for i := range p.Namespaces {
		for _, d := range ReadNamespace(p, p.Namespaces[i].Name).Errors {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

// ReadNamespace は、仕様 p の名前空間 name を読み解く（docs/plugin-db.md「対象」）。診断の順は、表の状態（宣言順。状態、その表の
// model のフィールドの順）、@db.ref、read（宣言順）、効果（宣言順）。名前空間がなければ、空の読み解き。
func ReadNamespace(p *ir.Program, name string) *Reading {
	r := &Reading{
		Name: name, p: p,
		Reads: map[string]*ReadDecl{}, Effects: map[string]*EffectDecl{},
		byState: map[string]*TableState{}, byModel: map[string]*Model{}, marked: map[string]bool{},
	}
	if p == nil {
		return r
	}
	ns := p.Namespace(name)
	if ns == nil {
		return r
	}
	r.tables(ns)
	r.reads(ns)
	r.effects(ns)
	return r
}

// applications は、適用の並び apps のうち、デコレータが names のどれかであるもの。書かれた順。
func applications(apps []ir.Application, names ...string) []ir.Application {
	var out []ir.Application
	for _, a := range apps {
		for _, n := range names {
			if a.Decorator == n {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// decoratorNames は、適用の並び apps のデコレータの名前を、`@db.find、@db.list` の形でつないだもの。
func decoratorNames(apps []ir.Application) string {
	names := make([]string, len(apps))
	for i, a := range apps {
		names[i] = "@" + a.Decorator
	}
	return strings.Join(names, "、")
}

// refsArg は、適用 a の引数 param に渡した宣言の FQN の並び（any exception の引数は、ユニオンを展開した並びで渡ることがある）。
func refsArg(a ir.Application, param string) []string {
	for _, arg := range a.Args {
		if arg.Param != param {
			continue
		}
		switch v := arg.Value.(type) {
		case ir.RefValue:
			return []string{v.Target}
		case ir.RefsValue:
			return v.Targets
		}
	}
	return nil
}

// literalArg は、適用 a の引数 param に渡したリテラル。渡していないか、リテラルでなければ nil。
func literalArg(a ir.Application, param string) ir.Literal {
	for _, arg := range a.Args {
		if l, ok := arg.Value.(ir.LiteralValue); ok && arg.Param == param {
			return l.Literal
		}
	}
	return nil
}

// stringArg は、適用 a の引数 param に渡した文字列。渡していないか、文字列でなければ ""。
func stringArg(a ir.Application, param string) string {
	if s, ok := literalArg(a, param).(ir.StringLiteral); ok {
		return s.Value
	}
	return ""
}

// short は FQN の最後のセグメント。
func short(fqn string) string {
	if i := strings.LastIndexByte(fqn, '.'); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

// display は、メッセージに書く、型（docs/grammar.md「型」の綴り）。
func display(t ir.Type) string {
	if t == nil {
		return ""
	}
	return t.String()
}

// tables は、N の @db.table の付いた状態から、表の状態と、表の model を作る（docs/plugin-db.md「表」）。形が list でなければ DB-2、
// 要素の型が、フィールドを持つ model でなければ DB-1。
func (r *Reading) tables(ns *ir.Namespace) {
	// perModel は、model の FQN → それを要素に持つ、@db.table の付いた N の状態の数（診断を返したものを含む。DB-11 が重ねて出ないように）
	perModel := map[string]int{}
	for _, d := range ns.Decls {
		st, ok := d.(*ir.State)
		if !ok || len(applications(st.Decorators, Table)) == 0 {
			continue
		}
		r.Marked++
		r.marked[st.Name] = true
		var decl *ir.Model
		if ref, isRef := st.Element.(ir.RefType); isRef {
			decl, _ = r.p.Lookup(ref.Target).(*ir.Model)
		}
		if decl != nil {
			perModel[decl.Name]++
		}
		valid := true
		if st.Shape != ir.StateList {
			r.errorf(CodeShape, st.Name, "@db.table の状態 %s の形が %s（表にできるのは list だけ。一行だけの表の形は、まだ決めていない）", st.Name, st.Shape)
			valid = false
		}
		if decl == nil {
			r.errorf(CodeElement, st.Name, "@db.table の状態 %s の要素の型 %s が、model でない（表の列は、要素の model のフィールドから作る）",
				st.Name, display(st.Element))
			valid = false
		} else if decl.Index != nil {
			r.errorf(CodeElement, st.Name, "@db.table の状態 %s の要素の型 %s が、インデックスの model（表の列は、要素の model のフィールドから作る。インデックスの model は、フィールドを持たない）",
				st.Name, display(st.Element))
			valid = false
		}
		if !valid {
			continue
		}
		t := &TableState{State: st, Name: short(st.Name)}
		m := r.byModel[decl.Name]
		if m == nil {
			m = &Model{Decl: decl}
			r.byModel[decl.Name] = m
			r.Models = append(r.Models, m)
			r.columns(m)
		}
		t.Model = m
		r.Tables = append(r.Tables, t)
		r.byState[st.Name] = t
	}
	r.refs(perModel)
}

// columns は、表の model m のフィールドから列を作る（docs/plugin-db.md「表」「キーと参照」）。しまい方のデコレータは、読まない。
func (r *Reading) columns(m *Model) {
	for i := range m.Decl.Fields {
		f := &m.Decl.Fields[i]
		c := &Column{Name: f.Name, Type: f.Type}
		for _, a := range f.Decorators {
			switch a.Decorator {
			case Key:
				c.Key = true
			case Unique:
				c.Unique = true
			case Ref:
				if targets := refsArg(a, "target"); len(targets) > 0 {
					c.Ref = targets[0]
				}
			}
		}
		_, c.Nullable = f.Type.(ir.OptionalType)
		m.Columns = append(m.Columns, c)
	}
}

// ClassOf は、型 t（省略可能を外したもの）の種類（docs/plugin-db.md「read」の、@db.where のリテラルを渡せる型）: 核の型の名前
// （scalar は、元の核の型まで辿る）、"array"、"model"（インデックスの model を含む）。どれでもなければ ""。
func ClassOf(p *ir.Program, t ir.Type) string {
	for depth := 0; depth < 64 && p != nil; depth++ {
		switch x := t.(type) {
		case ir.CaseType:
			t = x.Of
			continue
		case ir.CoreType:
			return string(x.Name)
		case ir.ArrayType:
			return "array"
		case ir.RefType:
			switch d := p.Lookup(x.Target).(type) {
			case *ir.Scalar:
				t = d.Base
				continue
			case *ir.Model:
				return "model"
			}
		}
		break
	}
	return ""
}

// refs は、@db.ref(M2) の指す先（docs/plugin-db.md「キーと参照」）: 要素の型が M2 の表の状態が N にちょうど一つあり、M2 が @db.key を
// ちょうど一つ持つとき、その表のその列。そうでなければ DB-11（位置は model）。perModel は、model を要素に持つ、@db.table の付いた
// N の状態の数（診断を返して、表にならなかったものを含む）。
func (r *Reading) refs(perModel map[string]int) {
	tablesOf := map[string][]*TableState{}
	for _, t := range r.Tables {
		tablesOf[t.Model.Decl.Name] = append(tablesOf[t.Model.Decl.Name], t)
	}
	for _, m := range r.Models {
		for _, c := range m.Columns {
			if c.Ref == "" {
				continue
			}
			switch n := perModel[c.Ref]; {
			case n == 0:
				r.errorf(CodeRef, m.Decl.Name, "model %s のフィールド %s の @db.ref(%s) の指す先が決まらない: 名前空間 %s に、要素の型が %s の @db.table の状態がない",
					m.Decl.Name, c.Name, c.Ref, r.Name, c.Ref)
				continue
			case n > 1:
				r.errorf(CodeRef, m.Decl.Name, "model %s のフィールド %s の @db.ref(%s) の指す先が決まらない: 名前空間 %s に、要素の型が %s の @db.table の状態が %d 個ある",
					m.Decl.Name, c.Name, c.Ref, r.Name, c.Ref, n)
				continue
			}
			targets := tablesOf[c.Ref]
			if len(targets) != 1 {
				continue // 先の状態が、表にならなかった（DB-1 か DB-2 を返している）
			}
			keys := targets[0].Model.Keys()
			if len(keys) != 1 {
				r.errorf(CodeRef, m.Decl.Name, "model %s のフィールド %s の @db.ref(%s) の指す先が決まらない: model %s の @db.key が %d 個（ちょうど一つ要る）",
					m.Decl.Name, c.Name, c.Ref, c.Ref, len(keys))
				continue
			}
			c.RefTable, c.RefColumn = targets[0].Name, keys[0].Name
		}
	}
}

// tableOf は、状態 state の表。N の表の状態でなければ DB-3（位置は、デコレータを付けた宣言 at。what は、その種類と、状態の言い方）。
// @db.table は付いているが、表にならなかった状態（DB-1 か DB-2 を返している）は、重ねて報告しない。
func (r *Reading) tableOf(state, what, at string) *TableState {
	if t := r.byState[state]; t != nil {
		return t
	}
	if !r.marked[state] {
		r.errorf(CodeTable, at, "%s %s が、名前空間 %s の表の状態でない（@db.table が付いていない）", what, state, r.Name)
	}
	return nil
}

// columnOf は、表 t の model の、名前が name の列。なければ DB-4（位置は、デコレータを付けた宣言 at。what は、列を名指ししたものの言い方）。
func (r *Reading) columnOf(t *TableState, name, what, at string) *Column {
	if c := t.Model.Column(name); c != nil {
		return c
	}
	r.errorf(CodeName, at, "%s が、表 %s の model %s のフィールドにない（列は、引数の名前とデコレータの column で名指しする）", what, t.State.Name, t.Model.Decl.Name)
	return nil
}

// paramConditions は、引数の条件（docs/plugin-db.md「read」「効果」の @db.delete）: 引数ごとに、その名前の列が、引数の値と同じ。
// 名前のフィールドがなければ DB-4、型が列に合わなければ DB-5（同「列と引数の型」）。
func (r *Reading) paramConditions(t *TableState, params []ir.OpParam, what, at string) []Condition {
	var out []Condition
	for i := range params {
		p := &params[i]
		c := r.columnOf(t, p.Name, what+" の引数 "+p.Name, at)
		if c == nil {
			continue
		}
		out = append(out, Condition{Column: c, Param: p})
		if ir.TypeEqual(p.Type, c.Type) {
			continue
		}
		if o, optional := c.Type.(ir.OptionalType); optional && ir.TypeEqual(p.Type, o.Of) {
			continue
		}
		r.errorf(CodeParams, at, "%s の引数 %s の型 %s が、列 %q の型 %s に合わない（列の型と等しいか、列が T? なら T）",
			what, p.Name, display(p.Type), c.Name, display(c.Type))
	}
	return out
}

// reads は、N の read のうち、db のデコレータ（@db.find、@db.list、@db.where、@db.latest）を付けたものについて、宣言とデコレータの
// 言うことを確かめる（docs/plugin-db.md「read」）。どの診断も、位置は read。付けていない read は、見ない。
func (r *Reading) reads(ns *ir.Namespace) {
	for _, d := range ns.Decls {
		decl, ok := d.(*ir.Read)
		if !ok {
			continue
		}
		kinds := applications(decl.Decorators, Find, List)
		wheres := applications(decl.Decorators, Where)
		latests := applications(decl.Decorators, Latest)
		if len(kinds)+len(wheres)+len(latests) == 0 {
			continue
		}
		fqn := decl.Name
		switch {
		case len(kinds) > 1:
			r.errorf(CodeBoth, fqn, "read %s が、@db.find と @db.list の両方を持つ（一行を引くか、行の並びを引くかの、どちらか一つ）", fqn)
			continue
		case len(kinds) == 0:
			extra := latests
			if len(wheres) > 0 {
				extra = append([]ir.Application{wheres[0]}, latests...)
			}
			r.errorf(CodeNeither, fqn, "read %s に %s が付いているが、@db.find も @db.list も付いていない（条件と並べ方は、引くものを言うデコレータに足す）",
				fqn, decoratorNames(extra))
			continue
		}
		x := &ReadDecl{Decl: decl, Kind: kinds[0].Decorator, Apps: kinds}
		if len(wheres) > 0 {
			x.Apps = append(x.Apps, wheres[0])
		}
		x.Apps = append(x.Apps, latests...)
		r.Reads[fqn] = x
		before := len(r.Errors)
		what := "read " + fqn
		x.Table = r.tableOf(decl.State, what+" の reads の状態", fqn)
		if x.Table == nil {
			continue
		}
		m := x.Table.Model

		// 条件: 引数（宣言の順）の後ろに、@db.where（書いた順）
		x.Conds = r.paramConditions(x.Table, decl.Params, what, fqn)
		named := len(decl.Params) + len(wheres) // 条件が名指しした列の数（フィールドになかったものを含む）
		for _, w := range wheres {
			name, lit := stringArg(w, "column"), literalArg(w, "value")
			c := r.columnOf(x.Table, name, what+" の @db.where の列 "+strconv.Quote(name), fqn)
			if c == nil {
				continue
			}
			x.Conds = append(x.Conds, Condition{Column: c, Literal: lit})
			if !r.literalFits(lit, c) {
				r.errorf(CodeLiteral, fqn, "read %s の @db.where の列 %q に、リテラル %s を渡せない（列の型は %s。%s）",
					fqn, name, LiteralText(lit), display(c.Type), r.literalHint(c))
			}
		}
		// DB-14: 一つの列を、二つの条件に書けない
		seen, twice := map[*Column]bool{}, map[*Column]bool{}
		for _, cond := range x.Conds {
			if seen[cond.Column] && !twice[cond.Column] {
				twice[cond.Column] = true
				r.errorf(CodeTwice, fqn, "read %s の条件が、列 %q を二度名指ししている（一つの列に書ける条件は、引数か @db.where の、どちらか一つ）", fqn, cond.Column.Name)
			}
			seen[cond.Column] = true
		}

		// 並べ方: @db.latest の列（新しい順に並べられるしまい方かは、語彙を出すトランスパイラが確かめる）
		if len(latests) > 0 {
			name := stringArg(latests[0], "column")
			x.Latest = r.columnOf(x.Table, name, what+" の @db.latest の列 "+strconv.Quote(name), fqn)
		}

		// 引くもの: @db.find は M?、@db.list は M[]
		row := ir.Ref(m.Decl.Name)
		want := ir.Type(ir.ArrayType{Of: row})
		if x.Kind == Find {
			want = ir.OptionalType{Of: row}
		}
		if !ir.TypeEqual(decl.Returns, want) {
			r.errorf(CodeReturns, fqn, "read %s の結果の型 %s が、@%s の言うもの（%s。表 %s の model の行）でない", fqn, display(decl.Returns), x.Kind, display(want), x.Table.State.Name)
		}

		// DB-9: @db.find は、一行に決まる。条件の列がフィールドになかった（DB-4 を返した）なら、決まるかどうかは言えない
		if x.Kind == Find && len(latests) == 0 && len(x.Conds) == named && !single(m, seen) {
			r.errorf(CodeSingle, fqn, "read %s（@db.find）が、一行に決まらない: 条件の列が、model %s の @db.key のすべても、@db.unique の列も含まず、@db.latest もない（並びを引くなら @db.list）",
				fqn, m.Decl.Name)
		}
		x.OK = len(r.Errors) == before
	}
}

// single は、条件の列 cols が、model m の行を一つに決めるか（docs/plugin-db.md「read」の 1 と 2）: m の @db.key のフィールドを
// すべて含む（m が @db.key を一つ以上持つとき）か、@db.unique のフィールドを一つ以上含む。
func single(m *Model, cols map[*Column]bool) bool {
	keys := m.Keys()
	all := len(keys) > 0
	for _, k := range keys {
		all = all && cols[k]
	}
	if all {
		return true
	}
	for c := range cols {
		if c.Unique {
			return true
		}
	}
	return false
}

// literalFits は、@db.where のリテラル lit を、列 c の型に渡せるか（docs/plugin-db.md「read」）: 列のフィールドの型（scalar は元の
// 核の型。省略可能は外して見る）に、docs/types.md「互換」の 10 で渡せる（整数のリテラルは float にも）。
func (r *Reading) literalFits(lit ir.Literal, c *Column) bool {
	class := r.columnClass(c)
	switch lit.(type) {
	case ir.StringLiteral:
		return class == "string"
	case ir.IntLiteral:
		return class == "int" || class == "float"
	case ir.FloatLiteral:
		return class == "float"
	case ir.BooleanLiteral:
		return class == "boolean"
	}
	return false
}

func (r *Reading) columnClass(c *Column) string {
	inner := c.Type
	if o, ok := inner.(ir.OptionalType); ok {
		inner = o.Of
	}
	return ClassOf(r.p, inner)
}

// literalHint は、列 c に渡せるリテラルの言い方（DB-15 のメッセージ用）。
func (r *Reading) literalHint(c *Column) string {
	switch r.columnClass(c) {
	case "string":
		return "渡せるのは文字列のリテラル"
	case "int":
		return "渡せるのは整数のリテラル"
	case "float":
		return "渡せるのは整数か小数のリテラル"
	case "boolean":
		return "渡せるのは true か false"
	}
	return "この型の列に渡せるリテラルはない"
}

// LiteralText は、リテラル lit の、仕様での書き方（docs/grammar.md「リテラル」）。診断のメッセージと、生成するコードのコメントに書く。
// IR が持つのは値なので、値から書き直す: 文字列は `"…"`（`"` と `\` と改行などはエスケープ）、整数は 10 進、小数は、いつも小数点を持つ形。
func LiteralText(lit ir.Literal) string {
	switch l := lit.(type) {
	case ir.StringLiteral:
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range l.Value {
			switch {
			case r == '"' || r == '\\':
				b.WriteByte('\\')
				b.WriteRune(r)
			case r == '\n':
				b.WriteString(`\n`)
			case r == '\t':
				b.WriteString(`\t`)
			case r == '\r':
				b.WriteString(`\r`)
			case r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029:
				b.WriteString(`\u{` + strconv.FormatInt(int64(r), 16) + `}`)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		return b.String()
	case ir.IntLiteral:
		return strconv.FormatInt(l.Value, 10)
	case ir.FloatLiteral:
		s := strconv.FormatFloat(l.Value, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	case ir.BooleanLiteral:
		return strconv.FormatBool(l.Value)
	case ir.DurationLiteral:
		return l.Value.String()
	}
	return "?"
}

// effects は、N の効果のうち、db の効果のデコレータ（@db.insert / @db.update / @db.delete、@db.conflict）を付けたものについて、
// 宣言とデコレータの言うことを確かめる（docs/plugin-db.md「効果」）。どの診断も、位置は効果。
func (r *Reading) effects(ns *ir.Namespace) {
	for _, d := range ns.Decls {
		decl, ok := d.(*ir.Effect)
		if !ok {
			continue
		}
		apps := applications(decl.Decorators, Insert, Update, Delete)
		fqn := decl.Name
		conflicts := applications(decl.Decorators, Conflict)
		// DB-18: @db.conflict は、@db.insert か @db.update と一緒に付ける
		if len(conflicts) > 0 && len(applications(decl.Decorators, Insert, Update)) == 0 {
			r.errorf(CodeConflictOn, fqn, "効果 %s の @db.conflict は、@db.insert か @db.update と一緒に付ける（一意の制約に当たりうるのは、行を足す文と、書き換える文だけ）", fqn)
			continue
		}
		if len(apps) == 0 {
			continue
		}
		if len(apps) > 1 {
			r.errorf(CodeEffect, fqn, "効果 %s が、効果のデコレータを二つ以上持つ（%s。@db.insert / @db.update / @db.delete のうち、高々一つ）", fqn, decoratorNames(apps))
			continue
		}
		x := &EffectDecl{Decl: decl, Kind: apps[0].Decorator}
		r.Effects[fqn] = x
		before := len(r.Errors)
		what := "効果 " + fqn
		r.conflict(x, conflicts)

		// DB-7: 宛先は、組み込みの効果で、操作が、デコレータと合う
		target, builtin := decl.Target.(ir.BuiltinTarget)
		if !builtin {
			r.errorf(CodeTarget, fqn, "効果 %s の @%s が、効果の宛先と合わない: 宛先が、分類の遷移（どの表に何をするかが、宣言から決まらない。この効果は、デコレータを付けずに、人が書く）", fqn, x.Kind)
			continue
		}
		want := map[string]ir.Operation{Insert: ir.OpPost, Update: ir.OpPut, Delete: ir.OpDelete}[x.Kind]
		if target.Operation != want {
			r.errorf(CodeTarget, fqn, "効果 %s の @%s が、効果の宛先 %s.%s の操作と合わない（@%s は %s）", fqn, x.Kind, target.State, target.Operation, x.Kind, want)
		}
		x.Table = r.tableOf(target.State, what+" の宛先の状態", fqn)
		if x.Table == nil {
			continue
		}
		m := x.Table.Model

		switch x.Kind {
		case Delete:
			// 引数は、条件。一つ以上
			if len(decl.Params) == 0 {
				r.errorf(CodeDeleteAll, fqn, "効果 %s（@db.delete）が、引数を一つも持たない（すべての行を消す形は、まだ決めていない。消す行を、引数の名前の列で選ぶ）", fqn)
			}
			x.Conds = r.paramConditions(x.Table, decl.Params, what, fqn)
		default:
			// 引数は、表の model の行一つ（型は、model そのものか、それを分類のケースで絞り込んだ型）
			if len(decl.Params) != 1 || !ir.TypeEqual(uncase(decl.Params[0].Type), ir.Ref(m.Decl.Name)) {
				r.errorf(CodeParams, fqn, "効果 %s の引数（%s）が、@%s の受け取るもの（表 %s の model %s の行一つ）でない", fqn, paramsDisplay(decl.Params), x.Kind, x.Table.State.Name, m.Decl.Name)
			}
		}

		// DB-10: 書き換える行は @db.key で選び、キーでない列を書き換える
		if x.Kind == Update {
			switch keys := len(m.Keys()); {
			case keys == 0:
				r.errorf(CodeUpdate, fqn, "効果 %s の @db.update の表 %s の model %s が、@db.key を一つも持たない（書き換える行を選べない）", fqn, x.Table.State.Name, m.Decl.Name)
			case keys == len(m.Columns):
				r.errorf(CodeUpdate, fqn, "効果 %s の @db.update の表 %s の model %s が、キーでない列を一つも持たない（書き換えるものがない）", fqn, x.Table.State.Name, m.Decl.Name)
			}
		}
		// DB-17: @db.update は、渡った行を、そのまま書く。渡る行は、行の引数の型が言う、前のケースにあるので、後のケースが
		// 前のケースの並びになければ、書いた後の行は、宣言した後のケースにならない
		if row := decl.Row; x.Kind == Update && row != nil && row.To != "" {
			if from, ok := rowParamCases(decl); ok && !contains(from, row.To) {
				r.errorf(CodeRow, fqn, "効果 %s の @db.update が、行の遷移（%s、%s）と合わない: 渡る行は、前のケースにあり、@db.update は、その行を、そのまま書くので、書いた後の行は %s.%s にならない（行をそのケースにする書き方は、宣言から決まらない。この効果は、デコレータを付けずに、人が書く）",
					fqn, narrowedParams(decl), "as "+row.Classify+"."+row.To, row.Classify, row.To)
			}
		}
		x.OK = len(r.Errors) == before
	}
}

// uncase は、絞り込んだ型（case）なら、もとの型。それ以外は、そのまま。
func uncase(t ir.Type) ir.Type {
	if c, ok := t.(ir.CaseType); ok {
		return c.Of
	}
	return t
}

// rowParamCases は、効果の、行の引数の型が、行の遷移の分類で絞り込んだ型（case）なら、そのケースの並び（効果の前のケース）。
// 絞り込んでいなければ（どのケースの行でも受け取る）ok は false。
func rowParamCases(decl *ir.Effect) ([]string, bool) {
	for _, p := range decl.Params {
		if p.Name != decl.Row.Param {
			continue
		}
		if c, ok := p.Type.(ir.CaseType); ok && c.Classify == decl.Row.Classify {
			return c.Cases, true
		}
	}
	return nil, false
}

// narrowedParams は、効果の引数のうち、型が case のものを、宣言の順に、「、」でつないだもの。一つは `<引数名> on <分類の FQN>.<ケース名>`
// （いくつかのケースは " | " でつなぐ）。
func narrowedParams(e *ir.Effect) string {
	var parts []string
	for _, p := range e.Params {
		c, ok := p.Type.(ir.CaseType)
		if !ok {
			continue
		}
		cases := make([]string, len(c.Cases))
		for i, name := range c.Cases {
			cases[i] = c.Classify + "." + name
		}
		parts = append(parts, p.Name+" on "+strings.Join(cases, " | "))
	}
	return strings.Join(parts, "、")
}

// conflict は、効果 x の @db.conflict（conflicts は、その適用。なければ空）を確かめ、x.Conflict を決める（docs/plugin-db.md「効果」）。
// DB-19: 名指しした例外が、効果の throws にないか、引数を持つ。
func (r *Reading) conflict(x *EffectDecl, conflicts []ir.Application) {
	if len(conflicts) == 0 {
		return
	}
	fqn := x.Decl.Name
	for _, exc := range refsArg(conflicts[0], "error") {
		decl, _ := r.p.Lookup(exc).(*ir.Exception)
		switch {
		case !contains(x.Decl.Throws, exc):
			r.errorf(CodeConflictExc, fqn, "効果 %s の @db.conflict の例外 %s が、その効果の throws にない（effect fn %s(…): throws %s effects … と書く）", fqn, exc, short(fqn), short(exc))
		case decl != nil && len(decl.Params) > 0:
			r.errorf(CodeConflictExc, fqn, "効果 %s の @db.conflict の例外 %s は、引数を持つ（@db.conflict は、値を埋める書き方を持たない。この効果は、デコレータを付けずに、人が書く）", fqn, exc)
		default:
			x.Conflict = exc
		}
		break // 例外は、効果ごとに一つ（ユニオンを渡しても、最初の一つ）
	}
}

func contains(list []string, name string) bool {
	for _, c := range list {
		if c == name {
			return true
		}
	}
	return false
}

// paramsDisplay は、引数の並びの、メッセージ用の書き方（`a: T, b: U`。なければ「なし」）。
func paramsDisplay(params []ir.OpParam) string {
	if len(params) == 0 {
		return "なし"
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.Name + ": " + display(p.Type)
	}
	return strings.Join(parts, ", ")
}
