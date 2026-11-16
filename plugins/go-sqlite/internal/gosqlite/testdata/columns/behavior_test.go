// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 列の型と値の変え方、省略可能な列、制約
// （docs/plugin-go-sqlite.md「表」）を、本物の SQLite の上で確かめる。plugins/go-sqlite の TestBehavior が、基底の出力と
// go-sqlite の出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/app/gen/app"
	"example.com/app/gen/kinds"

	_ "modernc.org/sqlite"
)

// openDB は、メモリの上の SQLite を開き、表を作る。呼ぶたびに別のデータベースになる。外部キーを有効にする。
func openDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := app.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	// すでにある表には触れない
	if err := app.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db, ctx
}

// plain は、@db のない状態の単位の、人が書いた実装。
type plain struct{}

func (plain) IsEmpty(context.Context) (bool, error)   { return true, nil }
func (plain) Get(context.Context) ([]app.Core, error) { return nil, nil }

// TestUnits は、どの型も基底の状態の単位を満たし、基底の New にそのまま渡せること（満たさなければ、ここでコンパイルできない）。
func TestUnits(t *testing.T) {
	db, ctx := openDB(t)
	ops := app.New(app.NewCoresTable(db), app.NewNamedsTable(db), app.NewDocsTable(db), app.NewSparsesTable(db),
		app.NewPairsTable(db), app.NewSharedATable(db), app.NewSharedBTable(db), app.NewLinksTable(db), plain{})
	if err := ops.AddSharedA(ctx, app.Shared{Id: "x"}); err != nil {
		t.Fatal(err)
	}
	// 同じ db から作った値は、同じ表を読み書きする。別の表（sharedB）には入らない
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

// TestCore は、核の型の列: boolean は 0 か 1 の INTEGER、Duration はナノ秒の INTEGER、Time は RFC 3339・UTC・小数部 9 桁の TEXT。
func TestCore(t *testing.T) {
	db, ctx := openDB(t)
	cores := app.NewCoresTable(db)
	at := time.Date(2026, 1, 2, 12, 4, 5, 120000000, time.FixedZone("JST", 9*60*60))
	want := app.Core{S: "a", I: -42, F: 2.5, B: true, D: 90 * time.Second, T: at}
	if err := cores.AddCore(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := cores.AddCore(ctx, app.Core{S: "b", T: at.Add(time.Nanosecond)}); err != nil {
		t.Fatal(err)
	}
	var types, text string
	var b, d int64
	err := db.QueryRow(`SELECT typeof("s") || ' ' || typeof("i") || ' ' || typeof("f") || ' ' || typeof("b") || ' ' || typeof("d") || ' ' || typeof("t"), "b", "d", "t" FROM "cores" WHERE "s" = 'a'`).
		Scan(&types, &b, &d, &text)
	if err != nil {
		t.Fatal(err)
	}
	if types != "text integer real integer integer text" || b != 1 || d != 90_000_000_000 || text != "2026-01-02T03:04:05.120000000Z" {
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
	if got[1].B || got[1].D != 0 || !got[1].T.Equal(at.Add(time.Nanosecond)) {
		t.Errorf("二つ目の行が %+v", got[1])
	}
	// 文字列の順が時刻の順になる（小数部はいつも 9 桁）
	var first string
	if err := db.QueryRow(`SELECT "s" FROM "cores" ORDER BY "t" DESC LIMIT 1`).Scan(&first); err != nil || first != "b" {
		t.Errorf("時刻の列の順で最後の行が %q, %v（期待は b）", first, err)
	}
}

// TestNamed は、scalar の列: 元の核の型で読み書きする（名前付きの真偽値の型、Time が元の型も）。
func TestNamed(t *testing.T) {
	db, ctx := openDB(t)
	nameds := app.NewNamedsTable(db)
	at := time.Date(2026, 3, 4, 5, 6, 7, 8, time.UTC)
	want := app.Named{Name: "n", Count: 7, Ratio: 0.25, Flag: true, Span: kinds.Span(time.Minute), Stamp: kinds.Stamp(at), Label: "l"}
	if err := nameds.AddNamed(ctx, want); err != nil {
		t.Fatal(err)
	}
	var types, stamp string
	err := db.QueryRow(`SELECT typeof("name") || ' ' || typeof("count") || ' ' || typeof("ratio") || ' ' || typeof("flag") || ' ' || typeof("span") || ' ' || typeof("stamp") || ' ' || typeof("label"), "stamp" FROM "nameds"`).
		Scan(&types, &stamp)
	if err != nil {
		t.Fatal(err)
	}
	if types != "text integer real integer integer text text" || stamp != "2026-03-04T05:06:07.000000008Z" {
		t.Errorf("列が %s、stamp = %s", types, stamp)
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

// TestDoc は、JSON の列: array、record、model は encoding/json の JSON の TEXT。
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
	if err := docs.AddDoc(ctx, want); err != nil {
		t.Fatal(err)
	}
	// nil のスライスとマップも、NOT NULL の列に入る（JSON の null）
	if err := docs.AddDoc(ctx, app.Doc{}); err != nil {
		t.Fatal(err)
	}
	var tags, attrs, tag, nested string
	if err := db.QueryRow(`SELECT "tags", "attrs", "tag", "nested" FROM "docs" ORDER BY rowid LIMIT 1`).Scan(&tags, &attrs, &tag, &nested); err != nil {
		t.Fatal(err)
	}
	if tags != `["a","b"]` || attrs != `{"x":1,"y":2}` || tag != `{"key":"k","value":"v"}` || nested != `[{"key":"n","value":"1"}]` {
		t.Errorf("列の値が tags = %s、attrs = %s、tag = %s、nested = %s", tags, attrs, tag, nested)
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

// TestSparse は、省略可能な列: 値がなければ NULL で、読めば nil。あれば、省略可能でない列と同じ値。
func TestSparse(t *testing.T) {
	db, ctx := openDB(t)
	sparses := app.NewSparsesTable(db)
	if err := sparses.AddSparse(ctx, app.Sparse{}); err != nil {
		t.Fatal(err)
	}
	var nulls int
	err := db.QueryRow(`SELECT COUNT(*) FROM "sparses" WHERE "s" IS NULL AND "i" IS NULL AND "f" IS NULL AND "b" IS NULL AND "d" IS NULL AND "t" IS NULL
		AND "name" IS NULL AND "count" IS NULL AND "ratio" IS NULL AND "flag" IS NULL AND "span" IS NULL AND "stamp" IS NULL
		AND "tags" IS NULL AND "attrs" IS NULL AND "tag" IS NULL`).Scan(&nulls)
	if err != nil || nulls != 1 {
		t.Fatalf("すべての列が NULL の行が %d, %v（期待は 1）", nulls, err)
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	full := app.Sparse{
		S: ptr("s"), I: ptr(int64(0)), F: ptr(1.5), B: ptr(false), D: ptr(time.Hour), T: ptr(at),
		Name: ptr(kinds.Name("n")), Count: ptr(kinds.Count(3)), Ratio: ptr(kinds.Ratio(0.5)), Flag: ptr(kinds.Flag(true)),
		Span: ptr(kinds.Span(time.Second)), Stamp: ptr(kinds.Stamp(at)),
		Tags: []string{}, Attrs: map[string]int64{"a": 1}, Tag: &kinds.Tag{Key: "k"},
	}
	if err := sparses.AddSparse(ctx, full); err != nil {
		t.Fatal(err)
	}
	var types string
	err = db.QueryRow(`SELECT typeof("s") || ' ' || typeof("i") || ' ' || typeof("f") || ' ' || typeof("b") || ' ' || typeof("d") || ' ' || typeof("t") || ' ' ||
		typeof("name") || ' ' || typeof("count") || ' ' || typeof("ratio") || ' ' || typeof("flag") || ' ' || typeof("span") || ' ' || typeof("stamp") || ' ' ||
		typeof("tags") || ' ' || typeof("attrs") || ' ' || typeof("tag") FROM "sparses" WHERE "s" = 's'`).Scan(&types)
	if err != nil || types != "text integer real integer integer text text integer real integer integer text text text text" {
		t.Errorf("値のある行の列が %s, %v", types, err)
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

// TestPair は、複数の @db.key（組で一意）と @db.unique。
func TestPair(t *testing.T) {
	db, ctx := openDB(t)
	pairs := app.NewPairsTable(db)
	for _, p := range []app.Pair{{Left: "a", Right: 1, Note: "x"}, {Left: "a", Right: 2, Note: "y"}, {Left: "b", Right: 1, Note: "z"}} {
		if err := pairs.AddPair(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := pairs.AddPair(ctx, app.Pair{Left: "a", Right: 1, Note: "w"}); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("同じキーの組の行を足したときのエラーが %v", err)
	}
	if err := pairs.AddPair(ctx, app.Pair{Left: "c", Right: 3, Note: "x"}); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("同じ @db.unique の値の行を足したときのエラーが %v", err)
	}
	if got, err := pairs.Get(ctx); err != nil || len(got) != 3 || got[0].Note != "x" || got[2].Left != "b" {
		t.Errorf("Get = %v, %v（期待は、足した順の三行）", got, err)
	}
}

// TestLink は @db.ref: 先の表がちょうど一つで、キーがちょうど一つのときだけ REFERENCES になる。
func TestLink(t *testing.T) {
	db, ctx := openDB(t)
	links := app.NewLinksTable(db)
	// 先の行がない: core は cores の s を指す
	err := links.AddLink(ctx, app.Link{Core: "nobody", Pair: "p", Tag: "t", Shared: "s"})
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("先のない行を足したときのエラーが %v", err)
	}
	if err := app.NewCoresTable(db).AddCore(ctx, app.Core{S: "c1"}); err != nil {
		t.Fatal(err)
	}
	// pair（キーが二つ）、tag（表でない）、shared（表が二つ）は、制約にならない。self は、省略可能なキーで、自分の表を指す
	if err := links.AddLink(ctx, app.Link{Core: "c1", Pair: "p", Tag: "t", Shared: "s", Self: ptr("l1")}); err != nil {
		t.Fatal(err)
	}
	if err := links.AddLink(ctx, app.Link{Core: "c1", Pair: "p", Tag: "t", Shared: "s", Self: ptr("missing")}); err != nil {
		// 自分を指す行（self = missing は、同じ行の self）は足せる
		t.Fatal(err)
	}
	if err := links.AddLink(ctx, app.Link{Core: "c1", Pair: "p", Tag: "t", Shared: "s"}); err != nil {
		t.Fatalf("self が NULL の行を足せない: %v", err)
	}
	got, err := links.Get(ctx)
	if err != nil || len(got) != 3 || got[0].Self == nil || *got[0].Self != "l1" || got[2].Self != nil {
		t.Errorf("Get = %+v, %v", got, err)
	}
}
