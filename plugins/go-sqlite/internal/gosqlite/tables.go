package gosqlite

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは docs/plugin-go-sqlite.md「表」: 表の状態と、要素の model の列（型、省略可能、制約）と、Schema の SQL。

// colKind は列の型の表の行（docs/plugin-go-sqlite.md「表」）。scalar は、元の核の型で見る。
type colKind int

const (
	kindString   colKind = iota // string: TEXT
	kindInt                     // int: INTEGER
	kindFloat                   // float: REAL
	kindBool                    // boolean: INTEGER（0 か 1）
	kindDuration                // Duration: INTEGER（ナノ秒）
	kindTime                    // Time: TEXT（RFC 3339、UTC、小数部 9 桁）
	kindJSON                    // array、record、model: TEXT（JSON）
)

// sqlType は列の型。
func (k colKind) sqlType() string {
	switch k {
	case kindInt, kindBool, kindDuration:
		return "INTEGER"
	case kindFloat:
		return "REAL"
	}
	return "TEXT"
}

// core は、その列に書く値の Go の型（直に読み書きできる行だけ。Time と JSON は ""）。
func (k colKind) core() string {
	switch k {
	case kindString:
		return "string"
	case kindInt, kindDuration:
		return "int64"
	case kindFloat:
		return "float64"
	case kindBool:
		return "bool"
	}
	return ""
}

// column は列一つ: 要素の model のフィールド一つ。
type column struct {
	name     string  // 列の名前（= フィールドの名前）
	field    string  // Go のフィールドの名前
	typ      ir.Type // フィールドの型（省略可能なら optional）
	inner    ir.Type // 省略可能を外した型
	kind     colKind
	scalar   bool // inner が scalar（Go では名前付きの型）か
	optional bool // 省略可能な列（NOT NULL を付けない）
	key      bool // @db.key
	unique   bool // @db.unique
	ref      string
}

// model は、表の状態の要素の model 一つ。
type model struct {
	decl  *ir.Model
	cols  []column
	table string // その model を要素に持つ最初の表の状態の、表の名前（補助のコメントに書く）
	// argsName と scanName は、補助 dbArgs<Model> と dbScan<Model> の名前。usesArgs は、dbArgs<Model> を使うメソッド
	// （足す、書き換える）を出したか（使う補助だけを出す）。
	argsName, scanName string
	usesArgs           bool
}

// keys は @db.key の付いた列（フィールドの順）。
func (m *model) keys() []*column {
	var out []*column
	for i := range m.cols {
		if m.cols[i].key {
			out = append(out, &m.cols[i])
		}
	}
	return out
}

// table は表の状態一つ: 表と、その上の型 <状態>Table。
type table struct {
	state  *ir.State
	name   string           // 表の名前（= 状態の名前）
	goName string           // <状態>Table
	unit   golang.StateUnit // 基底の状態の単位（<状態>State）
	model  *model
	// methods は、中身を出す op のメソッド（宣言順）。missing は、規則に当たらなかった op の FQN（宣言順）。
	methods []*method
	missing []string
}

// tableTypeName は、状態 fqn の型の名前 <状態>Table（docs/plugin-go-sqlite.md「出力」）: 状態の名前を先頭大文字にして Table を付ける。
func tableTypeName(fqn string) string { return golang.Capitalize(short(fqn)) + "Table" }

// collectTables は、N の @db の付いた基底の状態から、表の状態（e.tables）と、その要素の model（e.models）を作る。
// 形が list でなければ GOSQL-2、要素が model への ref でなければ GOSQL-1、列にできない型のフィールドがあれば GOSQL-3。
// 戻り値は、@db の付いた状態の数（診断を返したものを含む）。
func (e *emitter) collectTables() (marked int) {
	units := map[string]golang.StateUnit{}
	for _, u := range e.shape.States {
		units[u.FQN] = u
	}
	for _, d := range e.ns.Decls {
		st, ok := d.(*ir.State)
		if !ok || !hasDecorator(st.Decorators, decoratorTable) {
			continue
		}
		marked++
		valid := true
		if st.Shape != ir.StateList {
			e.errorf(codeSQLShape, st.Name, "@db の状態 %s の形が %s（表にできるのは list だけ。一行だけの表の形は、まだ決めていない）", st.Name, st.Shape)
			valid = false
		}
		ref, isRef := st.Element.(ir.RefType)
		var decl *ir.Model
		if isRef {
			decl, _ = e.p.Lookup(ref.Target).(*ir.Model)
		}
		if decl == nil {
			e.errorf(codeSQLElement, st.Name, "@db の状態 %s の要素 %s が model でない（表の列は、要素の model のフィールドから作る）", st.Name, golang.TypeDisplay(st.Element))
			valid = false
		}
		if !valid {
			continue
		}
		t := &table{state: st, name: short(st.Name), goName: tableTypeName(st.Name), unit: units[st.Name]}
		m := e.byModel[decl.Name]
		if m == nil {
			m = &model{decl: decl, table: t.name}
			e.byModel[decl.Name] = m
			e.models = append(e.models, m)
			e.columns(m)
		}
		t.model = m
		e.tables = append(e.tables, t)
	}
	e.nameHelpers()
	return marked
}

// columns は、model m のフィールドから列を作る。列にできない型（union と、union を中に持つ型）のフィールドは GOSQL-3。
func (e *emitter) columns(m *model) {
	for _, f := range m.decl.Fields {
		c := column{name: f.Name, field: golang.FieldName(f.Name), typ: f.Type, inner: f.Type}
		if o, ok := f.Type.(ir.OptionalType); ok {
			c.inner, c.optional = o.Of, true
		}
		if e.hasUnion(c.inner, map[string]bool{}) {
			e.errorf(codeSQLColumn, m.decl.Name, "model %s のフィールド %s の型 %s は、列にできない（union と、union を中に持つ型。JSON での union の形を、まだ決めていない）",
				m.decl.Name, f.Name, golang.TypeDisplay(f.Type))
			continue
		}
		c.kind, c.scalar = e.kindOf(c.inner)
		for _, a := range f.Decorators {
			switch a.Decorator {
			case decoratorKey:
				c.key = true
			case decoratorUnique:
				c.unique = true
			case decoratorRef:
				for _, arg := range a.Args {
					if r, ok := arg.Value.(ir.RefValue); ok {
						c.ref = r.Target
					}
				}
			}
		}
		m.cols = append(m.cols, c)
	}
}

// hasUnion は、型 t が union（arms）か、union を中に持つ array / record / model か。seen は、辿った model（自分を指す model で止まる）。
func (e *emitter) hasUnion(t ir.Type, seen map[string]bool) bool {
	switch x := t.(type) {
	case ir.OptionalType:
		return e.hasUnion(x.Of, seen)
	case ir.ArrayType:
		return e.hasUnion(x.Of, seen)
	case ir.RecordType:
		return e.hasUnion(x.Key, seen) || e.hasUnion(x.Value, seen)
	case ir.CaseType:
		return e.hasUnion(x.Of, seen)
	case ir.RefType:
		switch d := e.p.Lookup(x.Target).(type) {
		case *ir.Union:
			return true
		case *ir.Model:
			if seen[d.Name] {
				return false
			}
			seen[d.Name] = true
			for _, f := range d.Fields {
				if e.hasUnion(f.Type, seen) {
					return true
				}
			}
		}
	}
	return false
}

// kindOf は、型 t（省略可能を外したもの。union を中に持たない）の列の型の行と、scalar かどうか。scalar は、元の核の型まで辿る。
func (e *emitter) kindOf(t ir.Type) (kind colKind, scalar bool) {
	for depth := 0; depth < 64; depth++ {
		switch x := t.(type) {
		case ir.CaseType:
			t = x.Of
			continue
		case ir.CoreType:
			switch x.Name {
			case ir.CoreString:
				return kindString, scalar
			case ir.CoreInt:
				return kindInt, scalar
			case ir.CoreFloat:
				return kindFloat, scalar
			case ir.CoreBoolean:
				return kindBool, scalar
			case ir.CoreDuration:
				return kindDuration, scalar
			case ir.CoreTime:
				return kindTime, scalar
			}
		case ir.RefType:
			if s, ok := e.p.Lookup(x.Target).(*ir.Scalar); ok {
				t, scalar = s.Base, true
				continue
			}
		}
		break
	}
	return kindJSON, false
}

// nameHelpers は、要素の model ごとの補助の名前 dbArgs<Model> と dbScan<Model> を決める。<Model> は model の Go の型の名前。
// 別の名前空間の同じ名前の model と当たれば、後の方に 2、3、… を付ける。
func (e *emitter) nameHelpers() {
	taken := map[string]bool{}
	for _, m := range e.models {
		base := e.typeName(m.decl.Name)
		name := base
		for i := 2; taken[name]; i++ {
			name = base + strconv.Itoa(i)
		}
		taken[name] = true
		m.argsName, m.scanName = "dbArgs"+name, "dbScan"+name
	}
}

// quote は、SQL の名前を "…" で引用する（表と列の名前は、いつも引用する）。
func quote(name string) string { return `"` + name + `"` }

// columnList は、model m のすべての列の名前を、列の順に「, 」でつないだもの。
func columnList(m *model) string {
	names := make([]string, len(m.cols))
	for i, c := range m.cols {
		names[i] = quote(c.name)
	}
	return strings.Join(names, ", ")
}

// schema は Schema の SQL: 表の状態ごとの CREATE TABLE IF NOT EXISTS（docs/plugin-go-sqlite.md「表」）。
func (e *emitter) schema() string {
	// @db.ref(M) の先: 要素が M である表の状態が、N にちょうど一つ
	tablesOf := map[string][]*table{}
	for _, t := range e.tables {
		tablesOf[t.model.decl.Name] = append(tablesOf[t.model.decl.Name], t)
	}
	var b strings.Builder
	for _, t := range e.tables {
		keys := t.model.keys()
		var lines []string
		for _, c := range t.model.cols {
			def := quote(c.name) + " " + c.kind.sqlType()
			if !c.optional {
				def += " NOT NULL"
			}
			if c.key && len(keys) == 1 {
				def += " PRIMARY KEY"
			}
			if c.unique {
				def += " UNIQUE"
			}
			if targets := tablesOf[c.ref]; c.ref != "" && len(targets) == 1 {
				if tk := targets[0].model.keys(); len(tk) == 1 {
					def += " REFERENCES " + quote(targets[0].name) + " (" + quote(tk[0].name) + ")"
				}
			}
			lines = append(lines, "  "+def)
		}
		if len(keys) > 1 {
			names := make([]string, len(keys))
			for i, k := range keys {
				names[i] = quote(k.name)
			}
			lines = append(lines, "  PRIMARY KEY ("+strings.Join(names, ", ")+")")
		}
		b.WriteString("CREATE TABLE IF NOT EXISTS " + quote(t.name) + " (\n" + strings.Join(lines, ",\n") + "\n);\n")
	}
	return b.String()
}
