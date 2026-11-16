package gosqlite

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは docs/plugin-go-sqlite.md「表」「しまい方」: 表の状態と、表の model の列（しまい方、@db.nullable、制約）と、
// Schema の SQL。列の型も、列の値の読み書きも、フィールドのデコレータだけから作る。フィールドの型は、デコレータと食い違って
// いないかを確かめるのにだけ使う（GOSQL-3、GOSQL-17〜GOSQL-19）。

// storage は、しまい方一つ（docs/plugin-go-sqlite.md「しまい方」の表の行）。
type storage struct {
	decorator string // デコレータの FQN
	helper    string // しまい方の補助の名前（「補助」）
	sqlType   string // SQLite の列の型
	fits      string // 合うフィールドの型（診断のメッセージ用）
	classes   []string
}

// storages は、しまい方のすべて（「しまい方」の表の順。補助を出す順でもある）。classes は、合うフィールドの型の種類（typeClass）。
var storages = []*storage{
	{decorator: "db.text", helper: "dbText", sqlType: "TEXT", fits: "string", classes: []string{"string"}},
	{decorator: "db.int", helper: "dbInt", sqlType: "INTEGER", fits: "int、Duration", classes: []string{"int", "Duration"}},
	{decorator: "db.float", helper: "dbFloat", sqlType: "REAL", fits: "float", classes: []string{"float"}},
	{decorator: "db.bool", helper: "dbBool", sqlType: "INTEGER", fits: "boolean", classes: []string{"boolean"}},
	{decorator: "db.time", helper: "dbTime", sqlType: "TEXT", fits: "Time", classes: []string{"Time"}},
	{decorator: "db.json", helper: "dbJSON", sqlType: "TEXT", fits: "array、record、model", classes: []string{"array", "record", "model"}},
}

// column は列一つ: 表の model のフィールド一つ。
type column struct {
	name     string   // 列の名前（= フィールドの名前）
	field    string   // Go のフィールドの名前
	typ      ir.Type  // フィールドの型（整合の検査にだけ使う）
	store    *storage // しまい方（ちょうど一つでなければ nil。GOSQL-16）
	nullable bool     // @db.nullable
	key      bool     // @db.key
	unique   bool     // @db.unique
	ref      string   // @db.ref の指す model の FQN。なければ ""
	// refTable と refColumn は、@db.ref の指す先の表と列（resolveRefs）。
	refTable, refColumn string
}

// model は、表の model 一つ。
type model struct {
	decl  *ir.Model
	cols  []column
	table string // その model を @db.table で名指しした最初の表の状態の、表の名前（補助のコメントに書く）
	// argsName と scanName は、補助 dbArgs<Model> と dbScan<Model> の名前。usesArgs は、dbArgs<Model> を使うメソッド
	// （@db.insert か @db.update の効果を持つ op）を出すか（使う補助だけを出す）。
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

// column は、名前が name の列。なければ nil。
func (m *model) column(name string) *column {
	for i := range m.cols {
		if m.cols[i].name == name {
			return &m.cols[i]
		}
	}
	return nil
}

// table は表の状態一つ。
type table struct {
	state *ir.State
	name  string // 表の名前（= 状態の名前）
	model *model
}

// collectTables は、N の @db.table の付いた基底の状態から、表の状態（e.tables）と、表の model（e.models）を作る。
// 形が list でなければ GOSQL-2、要素の型が、名指しした model と等しくなければ GOSQL-1。model のフィールドの検査は columns、
// @db.ref の先は resolveRefs。戻り値は、@db.table の付いた状態の数（診断を返したものを含む）。
func (e *emitter) collectTables() (marked int) {
	// perModel は、model の FQN → それを @db.table で名指しした N の状態の数（診断を返したものを含む。GOSQL-20 が重ねて出ないように）
	perModel := map[string]int{}
	for _, d := range e.ns.Decls {
		st, ok := d.(*ir.State)
		if !ok {
			continue
		}
		apps := applications(st.Decorators, decoratorTable)
		if len(apps) == 0 {
			continue
		}
		marked++
		e.marked[st.Name] = true
		target := refArg(apps[0], "element")
		perModel[target]++
		valid := true
		if st.Shape != ir.StateList {
			e.errorf(codeSQLShape, st.Name, "@db.table の状態 %s の形が %s（表にできるのは list だけ。一行だけの表の形は、まだ決めていない）", st.Name, st.Shape)
			valid = false
		}
		decl, _ := e.p.Lookup(target).(*ir.Model)
		if decl == nil || !ir.TypeEqual(st.Element, ir.Ref(target)) {
			e.errorf(codeSQLElement, st.Name, "@db.table(%s) の状態 %s の要素の型 %s が、%s と等しくない（表の列は、名指しした model のフィールドから作る）",
				target, st.Name, golang.TypeDisplay(st.Element), target)
			valid = false
		}
		if !valid {
			continue
		}
		t := &table{state: st, name: short(st.Name)}
		m := e.byModel[decl.Name]
		if m == nil {
			m = &model{decl: decl, table: t.name}
			e.byModel[decl.Name] = m
			e.models = append(e.models, m)
			e.columns(m)
		}
		t.model = m
		e.tables = append(e.tables, t)
		e.byState[st.Name] = t
	}
	e.resolveRefs(perModel)
	e.nameHelpers()
	return marked
}

// columns は、表の model m のフィールドから列を作り、フィールドのデコレータと型の食い違いを確かめる（どれも、位置は model）:
// どのしまい方でも列にできない型（union と、union を中に持つ型）は GOSQL-3、しまい方がちょうど一つでなければ GOSQL-16、
// しまい方が型に合わなければ GOSQL-17、@db.nullable の付いた、省略可能でないフィールドは GOSQL-18、@db.nullable のない、
// 省略可能なフィールドは GOSQL-19。列の形（しまい方、@db.nullable、制約）は、デコレータだけから決める。
func (e *emitter) columns(m *model) {
	for _, f := range m.decl.Fields {
		c := column{name: f.Name, field: golang.FieldName(f.Name), typ: f.Type}
		var stores []*storage
		for _, a := range f.Decorators {
			switch a.Decorator {
			case decoratorNullable:
				c.nullable = true
			case decoratorKey:
				c.key = true
			case decoratorUnique:
				c.unique = true
			case decoratorRef:
				c.ref = refArg(a, "target")
			default:
				for _, s := range storages {
					if s.decorator == a.Decorator {
						stores = append(stores, s)
					}
				}
			}
		}
		if len(stores) == 1 {
			c.store = stores[0]
		}
		inner, optional := f.Type, false
		if o, ok := f.Type.(ir.OptionalType); ok {
			inner, optional = o.Of, true
		}
		union := e.hasUnion(inner, map[string]bool{})
		if union {
			e.errorf(codeSQLColumn, m.decl.Name, "model %s のフィールド %s の型 %s は、どのしまい方でも列にできない（union と、union を中に持つ型。JSON での union の形を、まだ決めていない）",
				m.decl.Name, f.Name, golang.TypeDisplay(f.Type))
		}
		switch {
		case len(stores) == 0:
			e.errorf(codeSQLStorage, m.decl.Name, "model %s のフィールド %s が、しまい方のデコレータ（%s）を持たない（表の model のフィールドは、ちょうど一つ要る）",
				m.decl.Name, f.Name, storageNames(storages))
		case len(stores) > 1:
			e.errorf(codeSQLStorage, m.decl.Name, "model %s のフィールド %s が、しまい方のデコレータを二つ以上持つ（%s。ちょうど一つ要る）",
				m.decl.Name, f.Name, storageNames(stores))
		case !union && !fits(c.store, e.typeClass(inner)):
			e.errorf(codeSQLFit, m.decl.Name, "model %s のフィールド %s のしまい方 @%s が、型 %s に合わない（@%s に合う型は %s）",
				m.decl.Name, f.Name, c.store.decorator, golang.TypeDisplay(f.Type), c.store.decorator, c.store.fits)
		}
		switch {
		case c.nullable && !optional:
			e.errorf(codeSQLNullable, m.decl.Name, "model %s のフィールド %s に @db.nullable が付いているが、型 %s は省略可能でない（値がなくてよい列は、型も省略可能にする）",
				m.decl.Name, f.Name, golang.TypeDisplay(f.Type))
		case optional && !c.nullable:
			e.errorf(codeSQLOptional, m.decl.Name, "model %s のフィールド %s の型 %s は省略可能だが、@db.nullable がない（値がなくてよい列は、@db.nullable で言う）",
				m.decl.Name, f.Name, golang.TypeDisplay(f.Type))
		}
		m.cols = append(m.cols, c)
	}
}

// storageNames は、しまい方のデコレータの名前を「 / 」でつないだもの（`@db.text / @db.int`）。
func storageNames(list []*storage) string {
	names := make([]string, len(list))
	for i, s := range list {
		names[i] = "@" + s.decorator
	}
	return strings.Join(names, " / ")
}

// fits は、しまい方 s が、種類 class の型に合うか（docs/plugin-go-sqlite.md「しまい方」の表）。
func fits(s *storage, class string) bool {
	for _, c := range s.classes {
		if c == class {
			return true
		}
	}
	return false
}

// typeClass は、型 t（省略可能を外したもの）の種類: 核の型の名前（scalar は、元の核の型まで辿る）、"array"、"record"、"model"。
// どれでもなければ ""。しまい方が型に合うか（GOSQL-17）を確かめるのにだけ使う。
func (e *emitter) typeClass(t ir.Type) string {
	for depth := 0; depth < 64; depth++ {
		switch x := t.(type) {
		case ir.CaseType:
			t = x.Of
			continue
		case ir.CoreType:
			return string(x.Name)
		case ir.ArrayType:
			return "array"
		case ir.RecordType:
			return "record"
		case ir.RefType:
			switch d := e.p.Lookup(x.Target).(type) {
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

// resolveRefs は、@db.ref(M2) の指す先（docs/plugin-go-sqlite.md「表」）: @db.table(M2) の付いた表の状態が N にちょうど一つあり、
// M2 が @db.key をちょうど一つ持つとき、その表のその列。そうでなければ GOSQL-20（位置は model）。黙って制約を省くことはしない。
// perModel は、model を @db.table で名指しした N の状態の数（診断を返して、表にならなかったものを含む）。
func (e *emitter) resolveRefs(perModel map[string]int) {
	tablesOf := map[string][]*table{}
	for _, t := range e.tables {
		tablesOf[t.model.decl.Name] = append(tablesOf[t.model.decl.Name], t)
	}
	for _, m := range e.models {
		for i := range m.cols {
			c := &m.cols[i]
			if c.ref == "" {
				continue
			}
			switch n := perModel[c.ref]; {
			case n == 0:
				e.errorf(codeSQLRef, m.decl.Name, "model %s のフィールド %s の @db.ref(%s) の指す先が決まらない: 名前空間 %s に、@db.table(%s) の付いた状態がない",
					m.decl.Name, c.name, c.ref, e.view.Viewer, c.ref)
				continue
			case n > 1:
				e.errorf(codeSQLRef, m.decl.Name, "model %s のフィールド %s の @db.ref(%s) の指す先が決まらない: 名前空間 %s に、@db.table(%s) の付いた状態が %d 個ある",
					m.decl.Name, c.name, c.ref, e.view.Viewer, c.ref, n)
				continue
			}
			targets := tablesOf[c.ref]
			if len(targets) != 1 {
				continue // 先の状態が、表にならなかった（GOSQL-1 か GOSQL-2 を返している）
			}
			keys := targets[0].model.keys()
			if len(keys) != 1 {
				e.errorf(codeSQLRef, m.decl.Name, "model %s のフィールド %s の @db.ref(%s) の指す先が決まらない: model %s の @db.key が %d 個（ちょうど一つ要る）",
					m.decl.Name, c.name, c.ref, c.ref, len(keys))
				continue
			}
			c.refTable, c.refColumn = targets[0].name, keys[0].name
		}
	}
}

// nameHelpers は、表の model ごとの補助の名前 dbArgs<Model> と dbScan<Model> を決める。<Model> は model の Go の型の名前。
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

// schema は Schema の SQL: 表の状態ごとの CREATE TABLE IF NOT EXISTS（docs/plugin-go-sqlite.md「表」）。列の定義は
// `"<名前>" <型>[ NOT NULL][ PRIMARY KEY][ UNIQUE][ REFERENCES …]`。NOT NULL は、@db.nullable を付けていない、すべての列に付ける。
func (e *emitter) schema() string {
	var b strings.Builder
	for _, t := range e.tables {
		keys := t.model.keys()
		var lines []string
		for _, c := range t.model.cols {
			def := quote(c.name) + " " + c.store.sqlType
			if !c.nullable {
				def += " NOT NULL"
			}
			if c.key && len(keys) == 1 {
				def += " PRIMARY KEY"
			}
			if c.unique {
				def += " UNIQUE"
			}
			if c.ref != "" {
				def += " REFERENCES " + quote(c.refTable) + " (" + quote(c.refColumn) + ")"
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
