// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 六つのしまい方（列の型と、値の変え方）、
// 省略可能なフィールドの列（NULL の読み書きと、NOT NULL の有無）、制約（docs/plugin-go-sqlite.md「表」「しまい方」「補助」）を、
// 本物の SQLite の上で確かめる。plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の出力を置いたモジュールのルートに
// 置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/app/gen/app"
	"example.com/app/gen/kinds"

	_ "modernc.org/sqlite"
)

// rawDB は、メモリの上の SQLite を開く。呼ぶたびに別のデータベースになる。外部キーを有効にする。表は作らない。
func rawDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db, context.Background()
}

// openDB は、メモリの上の SQLite を開き、表を作る。
func openDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	db, ctx := rawDB(t)
	if err := app.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	// すでにある表には触れない
	if err := app.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db, ctx
}

// plain は、@db.table のない状態の単位の、人が書いた実装。
type plain struct{}

func (plain) IsEmpty(context.Context) (bool, error)   { return true, nil }
func (plain) Get(context.Context) ([]app.Core, error) { return nil, nil }

// TestUnits は、どの型も基底の状態の単位を満たし、基底の New にそのまま渡せること（満たさなければ、ここでコンパイルできない）。
func TestUnits(t *testing.T) {
	db, ctx := openDB(t)
	ops := app.New(app.NewCoresTable(db), app.NewNamedsTable(db), app.NewDocsTable(db), app.NewSparsesTable(db),
		app.NewPairsTable(db), app.NewSharedATable(db), app.NewSharedBTable(db), app.NewLinksTable(db), plain{})
	// 配線の op は、効果のメソッドを呼ぶ
	if err := ops.AddCore(ctx, app.Core{S: "via op"}); err != nil {
		t.Fatal(err)
	}
	if got, err := app.NewCoresTable(db).Get(ctx); err != nil || len(got) != 1 || got[0].S != "via op" {
		t.Errorf("cores の Get = %v, %v", got, err)
	}
	if err := app.NewSharedATable(db).CreateSharedA(ctx, app.Shared{Id: "x"}); err != nil {
		t.Fatal(err)
	}
	// 同じ db から作った値は、同じ表を読み書きする。同じ model の別の表（sharedB）には入らない
	if got, err := app.NewSharedATable(db).Get(ctx); err != nil || len(got) != 1 || got[0].Id != "x" {
		t.Errorf("sharedA の Get = %v, %v", got, err)
	}
	if empty, err := app.NewSharedBTable(db).IsEmpty(ctx); err != nil || !empty {
		t.Errorf("sharedB の IsEmpty = %v, %v（期待は true）", empty, err)
	}
	if empty, err := app.NewSharedATable(db).IsEmpty(ctx); err != nil || empty {
		t.Errorf("sharedA の IsEmpty = %v, %v（期待は false）", empty, err)
	}
}

// notNull は、表 table の列ごとの、NOT NULL が付いているか（列の名前 → 付いていれば true）。
func notNull(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.Query(`SELECT "name", "notnull" FROM pragma_table_info(?1)`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		var flag bool
		if err := rows.Scan(&name, &flag); err != nil {
			t.Fatal(err)
		}
		out[name] = flag
	}
	if err := rows.Err(); err != nil || len(out) == 0 {
		t.Fatalf("表 %s の列を読めない: %v", table, err)
	}
	return out
}

// TestNotNull は、NOT NULL が、フィールドの型が省略可能（T?）でない、すべての列に付き、省略可能なフィールドの列には付かないこと。
func TestNotNull(t *testing.T) {
	db, _ := openDB(t)
	for _, table := range []string{"cores", "nameds", "docs", "pairs"} {
		for col, flag := range notNull(t, db, table) {
			if !flag {
				t.Errorf("表 %s の列 %s に NOT NULL がない（フィールドは、省略可能でない）", table, col)
			}
		}
	}
	for col, flag := range notNull(t, db, "sparses") {
		if flag {
			t.Errorf("表 sparses の列 %s に NOT NULL がある（フィールドは、省略可能）", col)
		}
	}
	if got := notNull(t, db, "links"); !got["id"] || !got["core"] || got["parent"] {
		t.Errorf("表 links の NOT NULL が %v（期待は id と core だけ）", got)
	}
}

// TestCore は、核の型の列（しまい方の表の各行）: @db.text は TEXT、@db.int は INTEGER（Duration はナノ秒）、@db.float は REAL、
// @db.bool は 0 か 1 の INTEGER、@db.time は RFC 3339・UTC・小数部 9 桁の TEXT。
func TestCore(t *testing.T) {
	db, ctx := openDB(t)
	cores := app.NewCoresTable(db)
	at := time.Date(2026, 1, 2, 12, 4, 5, 120000000, time.FixedZone("JST", 9*60*60))
	want := app.Core{S: "a", I: -42, D: 90 * time.Second, F: 2.5, B: true, T: at}
	if err := cores.CreateCore(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := cores.CreateCore(ctx, app.Core{S: "b", T: at.Add(time.Nanosecond)}); err != nil {
		t.Fatal(err)
	}
	var types, text string
	var b, d int64
	err := db.QueryRow(`SELECT typeof("s") || ' ' || typeof("i") || ' ' || typeof("d") || ' ' || typeof("f") || ' ' || typeof("b") || ' ' || typeof("t"), "b", "d", "t" FROM "cores" WHERE "s" = 'a'`).
		Scan(&types, &b, &d, &text)
	if err != nil {
		t.Fatal(err)
	}
	if types != "text integer integer real integer text" || b != 1 || d != 90_000_000_000 || text != "2026-01-02T03:04:05.120000000Z" {
		t.Errorf("列が %s、b = %d、d = %d、t = %s", types, b, d, text)
	}
	got, err := cores.Get(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("Get = %v, %v", got, err)
	}
	g := got[0]
	if g.S != want.S || g.I != want.I || g.F != want.F || g.B != want.B || g.D != want.D || !g.T.Equal(want.T) || g.T.Location() != time.UTC {
		t.Errorf("読んだ行が %+v（期待は %+v。時刻は UTC）", g, want)
	}
	// 零値も、値（NULL ではない）: false は 0、0 は 0
	if err := db.QueryRow(`SELECT typeof("i") || ' ' || typeof("f") || ' ' || typeof("b"), "b" FROM "cores" WHERE "s" = 'b'`).Scan(&types, &b); err != nil || types != "integer real integer" || b != 0 {
		t.Errorf("零値の行の列が %s、b = %d, %v", types, b, err)
	}
	if got[1].B || got[1].D != 0 || !got[1].T.Equal(at.Add(time.Nanosecond)) {
		t.Errorf("二つ目の行が %+v", got[1])
	}
	// 文字列の順が時刻の順になる（小数部はいつも 9 桁）
	var first string
	if err := db.QueryRow(`SELECT "s" FROM "cores" ORDER BY "t" DESC LIMIT 1`).Scan(&first); err != nil || first != "b" {
		t.Errorf("時刻の列の順で最後の行が %q, %v（期待は b）", first, err)
	}
}

// TestNamed は、scalar の列: 核の型の列と同じ列の型、同じ値（名前付きの真偽値の型、Time が元の型、scalar が元の scalar も）。
func TestNamed(t *testing.T) {
	db, ctx := openDB(t)
	nameds := app.NewNamedsTable(db)
	at := time.Date(2026, 3, 4, 5, 6, 7, 8, time.UTC)
	want := app.Named{Name: "n", Count: 7, Span: kinds.Span(time.Minute), Ratio: 0.25, Flag: true, Stamp: kinds.Stamp(at), Label: "l"}
	if err := nameds.CreateNamed(ctx, want); err != nil {
		t.Fatal(err)
	}
	var types, stamp string
	var flag, span int64
	err := db.QueryRow(`SELECT typeof("name") || ' ' || typeof("count") || ' ' || typeof("span") || ' ' || typeof("ratio") || ' ' || typeof("flag") || ' ' || typeof("stamp") || ' ' || typeof("label"), "stamp", "flag", "span" FROM "nameds"`).
		Scan(&types, &stamp, &flag, &span)
	if err != nil {
		t.Fatal(err)
	}
	if types != "text integer integer real integer text text" || stamp != "2026-03-04T05:06:07.000000008Z" || flag != 1 || span != 60_000_000_000 {
		t.Errorf("列が %s、stamp = %s、flag = %d、span = %d", types, stamp, flag, span)
	}
	got, err := nameds.Get(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("Get = %v, %v", got, err)
	}
	g := got[0]
	if g.Name != want.Name || g.Count != want.Count || g.Ratio != want.Ratio || g.Flag != want.Flag || g.Span != want.Span ||
		!time.Time(g.Stamp).Equal(at) || g.Label != want.Label {
		t.Errorf("読んだ行が %+v（期待は %+v）", g, want)
	}
}

// TestDoc は、@db.json の列: array、record、model は encoding/json の JSON の TEXT。
func TestDoc(t *testing.T) {
	db, ctx := openDB(t)
	docs := app.NewDocsTable(db)
	want := app.Doc{
		Tags:   []string{"a", "b"},
		Attrs:  map[string]int64{"x": 1, "y": 2},
		Tag:    kinds.Tag{Key: "k", Value: "v"},
		Nested: []kinds.Tag{{Key: "n", Value: "1"}},
		Tree:   app.Tree{Label: "root", Children: []app.Tree{{Label: "leaf"}}},
	}
	if err := docs.CreateDoc(ctx, want); err != nil {
		t.Fatal(err)
	}
	// 省略可能でないフィールドの列では、値のないスライスとマップも、しまい方のまま書く（JSON の null という文字列。NULL ではない）
	if err := docs.CreateDoc(ctx, app.Doc{}); err != nil {
		t.Fatal(err)
	}
	var tags, attrs, tag, nested string
	if err := db.QueryRow(`SELECT "tags", "attrs", "tag", "nested" FROM "docs" ORDER BY rowid LIMIT 1`).Scan(&tags, &attrs, &tag, &nested); err != nil {
		t.Fatal(err)
	}
	wantTag, _ := json.Marshal(want.Tag)
	wantNested, _ := json.Marshal(want.Nested)
	if tags != `["a","b"]` || attrs != `{"x":1,"y":2}` || tag != string(wantTag) || nested != string(wantNested) {
		t.Errorf("列の値が tags = %s、attrs = %s、tag = %s、nested = %s", tags, attrs, tag, nested)
	}
	var types string
	if err := db.QueryRow(`SELECT typeof("tags") || ' ' || typeof("attrs") || ' ' || typeof("tag") || ' ' || typeof("nested") || ' ' || typeof("tree"), "tags", "attrs" FROM "docs" ORDER BY rowid DESC LIMIT 1`).
		Scan(&types, &tags, &attrs); err != nil || types != "text text text text text" || tags != "null" || attrs != "null" {
		t.Errorf("空の行の列が %s、tags = %s、attrs = %s, %v（期待は、どれも TEXT で、値のないスライスとマップは null という文字列）", types, tags, attrs, err)
	}
	got, err := docs.Get(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("Get = %v, %v", got, err)
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("読んだ行が %+v（期待は %+v）", got[0], want)
	}
	if got[1].Tags != nil || got[1].Attrs != nil || got[1].Tag != (kinds.Tag{}) {
		t.Errorf("空の行が %+v", got[1])
	}
}

func ptr[T any](v T) *T { return &v }

// TestSparse は、省略可能なフィールドの列（しまい方ごと）: 値がなければ NULL で、読めば nil。@db.json でも、JSON の null という
// 文字列にはしない。値があれば、省略可能でないフィールドの列と同じ値。
func TestSparse(t *testing.T) {
	db, ctx := openDB(t)
	sparses := app.NewSparsesTable(db)
	if err := sparses.CreateSparse(ctx, app.Sparse{}); err != nil {
		t.Fatal(err)
	}
	var nulls int
	err := db.QueryRow(`SELECT COUNT(*) FROM "sparses" WHERE "s" IS NULL AND "i" IS NULL AND "d" IS NULL AND "f" IS NULL AND "b" IS NULL AND "t" IS NULL
		AND "name" IS NULL AND "count" IS NULL AND "span" IS NULL AND "ratio" IS NULL AND "flag" IS NULL AND "stamp" IS NULL
		AND "tags" IS NULL AND "attrs" IS NULL AND "tag" IS NULL`).Scan(&nulls)
	if err != nil || nulls != 1 {
		t.Fatalf("すべての列が NULL の行が %d, %v（期待は 1）", nulls, err)
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	// 零値を指すポインタと、空のスライスは、値（NULL ではない）
	full := app.Sparse{
		S: ptr(""), I: ptr(int64(0)), D: ptr(time.Hour), F: ptr(1.5), B: ptr(false), T: ptr(at),
		Name: ptr(kinds.Name("n")), Count: ptr(kinds.Count(3)), Span: ptr(kinds.Span(time.Second)), Ratio: ptr(kinds.Ratio(0.5)),
		Flag: ptr(kinds.Flag(true)), Stamp: ptr(kinds.Stamp(at)),
		Tags: []string{}, Attrs: map[string]int64{"a": 1}, Tag: &kinds.Tag{Key: "k"},
	}
	if err := sparses.CreateSparse(ctx, full); err != nil {
		t.Fatal(err)
	}
	var types, tags string
	var b int64
	err = db.QueryRow(`SELECT typeof("s") || ' ' || typeof("i") || ' ' || typeof("d") || ' ' || typeof("f") || ' ' || typeof("b") || ' ' || typeof("t") || ' ' ||
		typeof("name") || ' ' || typeof("count") || ' ' || typeof("span") || ' ' || typeof("ratio") || ' ' || typeof("flag") || ' ' || typeof("stamp") || ' ' ||
		typeof("tags") || ' ' || typeof("attrs") || ' ' || typeof("tag"), "tags", "b" FROM "sparses" WHERE "s" = ''`).Scan(&types, &tags, &b)
	if err != nil || types != "text integer integer real integer text text integer integer real integer text text text text" || tags != "[]" || b != 0 {
		t.Errorf("値のある行の列が %s、tags = %s、b = %d, %v", types, tags, b, err)
	}
	got, err := sparses.Get(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("Get = %v, %v", got, err)
	}
	if !reflect.DeepEqual(got[0], app.Sparse{}) {
		t.Errorf("NULL の行が %+v（期待は、どのフィールドも nil）", got[0])
	}
	g := got[1]
	if g.T == nil || !g.T.Equal(at) || g.Stamp == nil || !time.Time(*g.Stamp).Equal(at) {
		t.Fatalf("時刻が T = %v、Stamp = %v", g.T, g.Stamp)
	}
	g.T, g.Stamp = full.T, full.Stamp // 時刻は Equal で比べた（Location が違う）
	if !reflect.DeepEqual(g, full) {
		t.Errorf("値のある行が %+v（期待は %+v）", g, full)
	}
}

// TestNullWithoutNullable は、省略可能でないフィールドの列に NULL があれば、読むときにエラーになること（その列は NOT NULL なので、
// Schema で作った表では起きない。ここでは、NOT NULL のない表を手で作る）。
func TestNullWithoutNullable(t *testing.T) {
	db, ctx := rawDB(t)
	if _, err := db.Exec(`CREATE TABLE "cores" ("s", "i", "d", "f", "b", "t")`); err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"s", "i", "d", "f", "b", "t"} {
		if _, err := db.Exec(`DELETE FROM "cores"`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO "cores" VALUES ('a', 1, 2, 1.5, 1, '2026-01-02T03:04:05.000000000Z')`); err != nil {
			t.Fatal(err)
		}
		if got, err := app.NewCoresTable(db).Get(ctx); err != nil || len(got) != 1 {
			t.Fatalf("Get = %v, %v", got, err)
		}
		if _, err := db.Exec(`UPDATE "cores" SET "` + col + `" = NULL`); err != nil {
			t.Fatal(err)
		}
		if got, err := app.NewCoresTable(db).Get(ctx); err == nil || !strings.Contains(err.Error(), "NULL in a NOT NULL column") {
			t.Errorf("列 %s が NULL の行の Get = %v, %v（期待は、NOT NULL の列の NULL のエラー）", col, got, err)
		}
	}
}

// TestPair は、複数の @db.key（組で一意）と @db.unique。
func TestPair(t *testing.T) {
	db, ctx := openDB(t)
	pairs := app.NewPairsTable(db)
	for _, p := range []app.Pair{{Left: "a", Right: 1, Note: "x"}, {Left: "a", Right: 2, Note: "y"}, {Left: "b", Right: 1, Note: "z"}} {
		if err := pairs.CreatePair(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := pairs.CreatePair(ctx, app.Pair{Left: "a", Right: 1, Note: "w"}); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("同じキーの組の行を足したときのエラーが %v", err)
	}
	if err := pairs.CreatePair(ctx, app.Pair{Left: "c", Right: 3, Note: "x"}); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("同じ @db.unique の値の行を足したときのエラーが %v", err)
	}
	if got, err := pairs.Get(ctx); err != nil || len(got) != 3 || got[0].Note != "x" || got[2].Left != "b" {
		t.Errorf("Get = %v, %v（期待は、足した順の三行）", got, err)
	}
}

// TestLink は @db.ref: 先の表の、ただ一つのキーへの REFERENCES。自分の表も指せる。省略可能なフィールドの列は、値がなければ、先を持たない。
func TestLink(t *testing.T) {
	db, ctx := openDB(t)
	links := app.NewLinksTable(db)
	// 先の行がない: core は cores の s を指す
	err := links.CreateLink(ctx, app.Link{Id: "l0", Core: "nobody"})
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("先のない行を足したときのエラーが %v", err)
	}
	if err := app.NewCoresTable(db).CreateCore(ctx, app.Core{S: "c1"}); err != nil {
		t.Fatal(err)
	}
	if err := links.CreateLink(ctx, app.Link{Id: "l1", Core: "c1"}); err != nil {
		t.Fatalf("parent が NULL の行を足せない: %v", err)
	}
	if err := links.CreateLink(ctx, app.Link{Id: "l2", Core: "c1", Parent: ptr("l1")}); err != nil {
		t.Fatal(err)
	}
	// parent は links の id を指す
	if err := links.CreateLink(ctx, app.Link{Id: "l3", Core: "c1", Parent: ptr("missing")}); err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Errorf("parent の先のない行を足したときのエラーが %v", err)
	}
	// @db.unique は、省略可能なフィールドの列でも効く（NULL どうしは、重ならない）
	if err := links.CreateLink(ctx, app.Link{Id: "l4", Core: "c1", Parent: ptr("l1")}); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("同じ parent の行を足したときのエラーが %v", err)
	}
	if err := links.CreateLink(ctx, app.Link{Id: "l5", Core: "c1"}); err != nil {
		t.Fatalf("parent が NULL の二つ目の行を足せない: %v", err)
	}
	got, err := links.Get(ctx)
	if err != nil || len(got) != 3 || got[0].Parent != nil || got[1].Parent == nil || *got[1].Parent != "l1" || got[2].Parent != nil {
		t.Errorf("Get = %+v, %v", got, err)
	}
}
