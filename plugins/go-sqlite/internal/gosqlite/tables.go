package gosqlite

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/dbvocab"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは docs/plugin-go-sqlite.md「表」「しまい方」: 表の状態と、表の model の列（しまい方、値がなくてよいか、制約）と、
// Schema の SQL。どの状態が表で、どのフィールドが列か、キーと参照は、語彙の読み解き（plugin/dbvocab。docs/plugin-db.md「表」
// 「キーと参照」）が決める。しまい方は、ここが読む: 型からは当てず、フィールドのデコレータが言い、型は、それに合うかを確かめるのに
// だけ使う（GOSQL-3、GOSQL-16、GOSQL-17）。

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
	{decorator: "db.int", helper: "dbInt", sqlType: "INTEGER", fits: "int、duration", classes: []string{"int", "duration"}},
	{decorator: "db.float", helper: "dbFloat", sqlType: "REAL", fits: "float", classes: []string{"float"}},
	{decorator: "db.bool", helper: "dbBool", sqlType: "INTEGER", fits: "boolean", classes: []string{"boolean"}},
	{decorator: "db.datetime", helper: "dbDatetime", sqlType: "TEXT", fits: "datetime", classes: []string{"datetime"}},
	{decorator: "db.date", helper: "dbDate", sqlType: "TEXT", fits: "date", classes: []string{"date"}},
	{decorator: "db.time", helper: "dbTime", sqlType: "TEXT", fits: "time", classes: []string{"time"}},
	{decorator: "db.json", helper: "dbJSON", sqlType: "TEXT", fits: "array、model（インデックスの model を含む）", classes: []string{"array", "model"}},
}

// column は列一つ: 表の model のフィールド一つ。
type column struct {
	name     string   // 列の名前（= フィールドの名前）
	field    string   // Go のフィールドの名前
	typ      ir.Type  // フィールドの型（省略可能なら optional のまま）
	store    *storage // しまい方（ちょうど一つでなければ nil。GOSQL-16）
	nullable bool     // フィールドの型が省略可能（T?）: 列は、値がなくてよい
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
	table string // その model を要素に持つ、最初の表の状態の、表の名前（補助のコメントに書く）
	// argsName と scanName は、補助 dbArgs<Model> と dbScan<Model> の名前。usesArgs は、dbArgs<Model> を使うメソッド
	// （@db.insert か @db.update の効果か、形で決まる Post）を出すか（使う補助だけを出す）。
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

// collectTables は、語彙の読み解き（e.vocab）の、表の状態と、表の model から、このトランスパイラの表（e.tables）と model（e.models）を
// 作る: 列に、Go のフィールドの名前と、しまい方を足す（columns）。戻り値は、@db.table の付いた N の状態の数。
func (e *emitter) collectTables() (marked int) {
	for _, vm := range e.vocab.Models {
		m := &model{decl: vm.Decl}
		e.byModel[vm.Decl.Name] = m
		e.models = append(e.models, m)
		e.columns(m, vm)
	}
	for _, vt := range e.vocab.Tables {
		t := &table{state: vt.State, name: vt.Name, model: e.byModel[vt.Model.Decl.Name]}
		if t.model.table == "" {
			t.model.table = t.name
		}
		e.tables = append(e.tables, t)
		e.byState[vt.State.Name] = t
	}
	e.nameHelpers()
	return e.vocab.Marked
}

// columns は、表の model m の列を、語彙の読み解きの列（vm.Columns。フィールドの順）から作り、しまい方を読んで、フィールドの
// デコレータと型の食い違いを確かめる（どれも、位置は model）: どのしまい方でも列にできない型（union と、union を中に持つ型）は GOSQL-3、
// リテラルの型のフィールドを持つ model を中に持つ型は GOSQL-34、
// しまい方がちょうど一つでなければ GOSQL-16、しまい方が型に合わなければ GOSQL-17。
func (e *emitter) columns(m *model, vm *dbvocab.Model) {
	for i, f := range m.decl.Fields {
		vc := vm.Columns[i]
		c := column{
			name: vc.Name, field: golang.FieldName(vc.Name), typ: vc.Type,
			nullable: vc.Nullable, key: vc.Key, unique: vc.Unique, ref: vc.Ref, refTable: vc.RefTable, refColumn: vc.RefColumn,
		}
		var stores []*storage
		for _, a := range f.Decorators {
			for _, s := range storages {
				if s.decorator == a.Decorator {
					stores = append(stores, s)
				}
			}
		}
		if len(stores) == 1 {
			c.store = stores[0]
		}
		inner := f.Type
		if o, ok := f.Type.(ir.OptionalType); ok {
			inner = o.Of
		}
		union := e.hasUnion(inner, map[string]bool{})
		if union {
			e.errorf(codeSQLColumn, m.decl.Name, "model %s のフィールド %s の型 %s は、どのしまい方でも列にできない（union と、union を中に持つ型。JSON での union の形を、まだ決めていない）",
				m.decl.Name, f.Name, golang.TypeDisplay(f.Type))
		}
		// GOSQL-34: リテラルの型のフィールドを持つ model を中に持つ型（基底の構造体が、その項目を持たない）
		literal := !union && e.hasLiteralField(inner, map[string]bool{})
		if literal {
			e.errorf(codeSQLLiteral, m.decl.Name, "model %s のフィールド %s の型 %s は、列にできない（リテラルの型のフィールドを持つ model を中に持つ型。JSON での、リテラルの型の形を、まだ決めていない）",
				m.decl.Name, f.Name, golang.TypeDisplay(f.Type))
		}
		switch {
		case len(stores) == 0:
			e.errorf(codeSQLStorage, m.decl.Name, "model %s のフィールド %s が、しまい方のデコレータ（%s）を持たない（表の model のフィールドは、ちょうど一つ要る）",
				m.decl.Name, f.Name, storageNames(storages))
		case len(stores) > 1:
			e.errorf(codeSQLStorage, m.decl.Name, "model %s のフィールド %s が、しまい方のデコレータを二つ以上持つ（%s。ちょうど一つ要る）",
				m.decl.Name, f.Name, storageNames(stores))
		case !union && !literal && !fits(c.store, dbvocab.ClassOf(e.p, inner)):
			e.errorf(codeSQLFit, m.decl.Name, "model %s のフィールド %s のしまい方 @%s が、型 %s に合わない（@%s に合う型は %s）",
				m.decl.Name, f.Name, c.store.decorator, golang.TypeDisplay(f.Type), c.store.decorator, c.store.fits)
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

// hasLiteralField は、型 t が、リテラルの型のフィールドを持つ model か、それを中に持つ array / model（インデックスの model を含む）か。
// seen は、辿った model（自分を指す model で止まる）。union は、辿らない（GOSQL-3 が、先に断る）。
func (e *emitter) hasLiteralField(t ir.Type, seen map[string]bool) bool {
	switch x := t.(type) {
	case ir.OptionalType:
		return e.hasLiteralField(x.Of, seen)
	case ir.ArrayType:
		return e.hasLiteralField(x.Of, seen)
	case ir.CaseType:
		return e.hasLiteralField(x.Of, seen)
	case ir.RefType:
		if d, ok := e.p.Lookup(x.Target).(*ir.Model); ok && !seen[d.Name] {
			seen[d.Name] = true
			for _, f := range d.Fields {
				if _, direct := f.Type.(ir.LiteralType); direct || e.hasLiteralField(f.Type, seen) {
					return true
				}
			}
			if d.Index != nil {
				return e.hasLiteralField(d.Index.Value, seen)
			}
		}
	}
	return false
}

// hasUnion は、型 t が union（arms）か、union を中に持つ array / model（インデックスの model を含む）か。seen は、辿った model（自分を指す model で止まる）。
func (e *emitter) hasUnion(t ir.Type, seen map[string]bool) bool {
	switch x := t.(type) {
	case ir.OptionalType:
		return e.hasUnion(x.Of, seen)
	case ir.ArrayType:
		return e.hasUnion(x.Of, seen)
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
			if d.Index != nil {
				return e.hasUnion(d.Index.Key, seen) || e.hasUnion(d.Index.Value, seen)
			}
		}
	}
	return false
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
// `"<名前>" <型>[ NOT NULL][ PRIMARY KEY][ UNIQUE][ REFERENCES …]`。NOT NULL は、フィールドの型が省略可能でない、すべての列に付ける。
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
