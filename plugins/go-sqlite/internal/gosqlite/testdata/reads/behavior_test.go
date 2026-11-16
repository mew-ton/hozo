// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 「read の中身」の各行（@db.find、
// @db.list、二つ以上の引数、@db.where、@db.latest）と、「列と引数の型」の、合う行の表（docs/plugin-go-sqlite.md）を、本物の
// SQLite の上で確かめる。plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"example.com/app/gen/shop"

	_ "modernc.org/sqlite"
)

// tables は、新しいメモリの上の SQLite に作った、三つの表の型。
type tables struct {
	items  shop.ItemsTable
	stock  shop.StockTable
	audits shop.AuditsTable
	ops    shop.Ops
}

// open は、新しいメモリの上の SQLite に表を作る。go-sqlite の三つの型を、基底の New にそのまま渡す（どれも、状態の単位を満たす。
// 満たさなければ、ここでコンパイルできない）。
func open(t *testing.T) (tables, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := shop.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	tb := tables{items: shop.NewItemsTable(db), stock: shop.NewStockTable(db), audits: shop.NewAuditsTable(db)}
	tb.ops = shop.New(tb.items, tb.stock, tb.audits)
	return tb, ctx
}

func ptr[T any](v T) *T { return &v }

var (
	day1 = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	day2 = time.Date(2026, 1, 2, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	day3 = time.Date(2026, 1, 3, 9, 0, 0, 0, time.UTC)
	// 足す順: i2、i1、i3、i4、i5（足した順が、キーの順とも、時刻の順とも違う）
	items = []shop.Item{
		{Id: "i2", Sku: "s2", Shelf: 1, Kind: "book", Price: 9.5, Stock: 3, Active: true, AddedAt: day2, Tags: []string{"new"}, Note: ptr("fragile"), SoldAt: ptr(day3)},
		{Id: "i1", Sku: "s1", Shelf: 1, Kind: "book", Price: 10, Stock: 0, Active: false, AddedAt: day1, Tags: []string{"old", "red"}},
		{Id: "i3", Sku: "s3", Shelf: 2, Kind: "pen", Price: 10, Stock: 7, Active: true, AddedAt: day3, Tags: []string{}, Note: ptr(""), SoldAt: ptr(day1)},
		{Id: "i4", Sku: "s4", Shelf: 1, Kind: "book", Price: 12.25, Stock: 3, Active: true, AddedAt: day2, Tags: []string{"new"}},
		{Id: "i5", Sku: "s5", Shelf: -1, Kind: `"odd" \ kind`, Price: 1, Stock: 0, Active: false, AddedAt: day1, Tags: nil, SoldAt: ptr(day3)},
	}
)

func seed(t *testing.T, tb tables, ctx context.Context) {
	t.Helper()
	for _, it := range items {
		if err := tb.items.CreateItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
}

// wantIDs は、引いた行の識別子が、その順で want であること。
func wantIDs(t *testing.T, what string, got []shop.Item, err error, want ...shop.ItemId) {
	t.Helper()
	ids := []shop.ItemId{}
	for _, it := range got {
		ids = append(ids, it.Id)
	}
	if want == nil {
		want = []shop.ItemId{}
	}
	if err != nil || !reflect.DeepEqual(ids, want) {
		t.Errorf("%s = %v, %v（期待は %v）", what, ids, err, want)
	}
}

// TestFind は @db.find: 条件に合う一行を引く。なければ nil。条件が二つ以上なら、どれも満たす行。
func TestFind(t *testing.T) {
	tb, ctx := open(t)
	seed(t, tb, ctx)
	// ただ一つの @db.key
	got, err := tb.items.ItemById(ctx, "i1")
	if err != nil || got == nil || got.Sku != "s1" || !got.AddedAt.Equal(day1) || !reflect.DeepEqual(got.Tags, []string{"old", "red"}) || got.Note != nil || got.SoldAt != nil {
		t.Errorf("ItemById(i1) = %+v, %v", got, err)
	}
	if got, err := tb.items.ItemById(ctx, "nobody"); err != nil || got != nil {
		t.Errorf("ItemById(nobody) = %+v, %v（期待は nil, nil）", got, err)
	}
	// @db.unique
	if got, err := tb.items.ItemBySku(ctx, "s3"); err != nil || got == nil || got.Id != "i3" || got.Note == nil || *got.Note != "" {
		t.Errorf("ItemBySku(s3) = %+v, %v", got, err)
	}
	// キーと、ほかの列: どれも満たす行
	if got, err := tb.items.ActiveItem(ctx, "i2", true); err != nil || got == nil || got.Id != "i2" {
		t.Errorf("ActiveItem(i2, true) = %+v, %v", got, err)
	}
	if got, err := tb.items.ActiveItem(ctx, "i2", false); err != nil || got != nil {
		t.Errorf("ActiveItem(i2, false) = %+v, %v（期待は nil, nil）", got, err)
	}
	// 配線の op は、read のメソッドを呼ぶ
	if got, err := tb.ops.FindItem(ctx, "i4"); err != nil || got == nil || got.Price != 12.25 {
		t.Errorf("FindItem(i4) = %+v, %v", got, err)
	}
}

// TestFindByTwoKeys は、引数が二つの @db.find（二つの @db.key のすべて）と、キーの一つを @db.where で言う @db.find。
func TestFindByTwoKeys(t *testing.T) {
	tb, ctx := open(t)
	for _, st := range []shop.Stock{{Item: "i1", Shelf: 1, Count: 3}, {Item: "i1", Shelf: 2, Count: 4}, {Item: "i2", Shelf: 2, Count: 5}} {
		if err := tb.stock.CreateStock(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := tb.stock.StockAt(ctx, "i1", 2); err != nil || got == nil || got.Count != 4 {
		t.Errorf("StockAt(i1, 2) = %+v, %v（期待は count 4）", got, err)
	}
	if got, err := tb.stock.StockAt(ctx, "i2", 1); err != nil || got != nil {
		t.Errorf("StockAt(i2, 1) = %+v, %v（期待は nil, nil）", got, err)
	}
	// @db.where("shelf", 1)
	if got, err := tb.stock.FrontStock(ctx, "i1"); err != nil || got == nil || got.Count != 3 {
		t.Errorf("FrontStock(i1) = %+v, %v（期待は count 3）", got, err)
	}
	if got, err := tb.stock.FrontStock(ctx, "i2"); err != nil || got != nil {
		t.Errorf("FrontStock(i2) = %+v, %v（期待は nil, nil）", got, err)
	}
	// 二つの @db.key の一つだけなら、@db.list
	if got, err := tb.stock.StockOf(ctx, "i1"); err != nil || len(got) != 2 || got[0].Count != 3 || got[1].Count != 4 {
		t.Errorf("StockOf(i1) = %v, %v", got, err)
	}
}

// TestList は @db.list: 条件がなければすべての行、あれば、どの条件も満たす行。どちらも、足した順。しまい方ごとに、引数の値を
// 列の値と同じ形にして比べる。
func TestList(t *testing.T) {
	tb, ctx := open(t)
	got, err := tb.items.AllItems(ctx)
	if err != nil || got != nil {
		t.Errorf("空の表の AllItems = %v, %v（期待は nil, nil）", got, err)
	}
	seed(t, tb, ctx)
	got, err = tb.items.AllItems(ctx)
	wantIDs(t, "AllItems", got, err, "i2", "i1", "i3", "i4", "i5")
	// @db.int
	got, err = tb.items.ItemsOnShelf(ctx, 1)
	wantIDs(t, "ItemsOnShelf(1)", got, err, "i2", "i1", "i4")
	got, err = tb.items.ItemsOnShelf(ctx, 9)
	wantIDs(t, "ItemsOnShelf(9)", got, err)
	// @db.float
	got, err = tb.items.ItemsPriced(ctx, 10)
	wantIDs(t, "ItemsPriced(10)", got, err, "i1", "i3")
	// @db.bool
	got, err = tb.items.ItemsActive(ctx, false)
	wantIDs(t, "ItemsActive(false)", got, err, "i1", "i5")
	// @db.time: 同じ時刻なら、Location が違っても合う
	got, err = tb.items.ItemsAddedAt(ctx, day2.In(time.FixedZone("EST", -5*60*60)))
	wantIDs(t, "ItemsAddedAt(day2)", got, err, "i2", "i4")
	// @db.json: 同じ JSON の行
	got, err = tb.items.ItemsTagged(ctx, []string{"old", "red"})
	wantIDs(t, "ItemsTagged(old, red)", got, err, "i1")
	got, err = tb.items.ItemsTagged(ctx, []string{})
	wantIDs(t, "ItemsTagged()", got, err, "i3")
	// 引数が三つ: どれも満たす行
	got, err = tb.items.ItemsOf(ctx, "book", 1, true)
	wantIDs(t, "ItemsOf(book, 1, true)", got, err, "i2", "i4")
	got, err = tb.items.ItemsOf(ctx, "book", 1, false)
	wantIDs(t, "ItemsOf(book, 1, false)", got, err, "i1")
	got, err = tb.items.ItemsOf(ctx, "pen", 1, true)
	wantIDs(t, "ItemsOf(pen, 1, true)", got, err)
}

// TestColumnAndParam は「列と引数の型」の、合う行の表: T? の列に T の引数なら、値のない行は、どの値にも合わない。
// T? の列に T? の引数なら、引数に値がなければ、値のない行。
func TestColumnAndParam(t *testing.T) {
	tb, ctx := open(t)
	seed(t, tb, ctx)
	got, err := tb.items.ItemsNoted(ctx, "fragile")
	wantIDs(t, "ItemsNoted(fragile)", got, err, "i2")
	// 空の文字列は、値（NULL ではない）
	got, err = tb.items.ItemsNoted(ctx, "")
	wantIDs(t, `ItemsNoted("")`, got, err, "i3")
	got, err = tb.items.ItemsByNote(ctx, ptr("fragile"))
	wantIDs(t, "ItemsByNote(fragile)", got, err, "i2")
	got, err = tb.items.ItemsByNote(ctx, nil)
	wantIDs(t, "ItemsByNote(nil)", got, err, "i1", "i4", "i5")
}

// TestWhere は @db.where: 決まった値の条件。しまい方ごとのリテラル。引数の条件と合わせて、どれも満たす行。
func TestWhere(t *testing.T) {
	tb, ctx := open(t)
	seed(t, tb, ctx)
	got, err := tb.items.Books(ctx)
	wantIDs(t, "Books", got, err, "i2", "i1", "i4")
	got, err = tb.items.SoldOut(ctx)
	wantIDs(t, "SoldOut", got, err, "i1", "i5")
	got, err = tb.items.AtNineFifty(ctx)
	wantIDs(t, "AtNineFifty", got, err, "i2")
	// @db.float の列に、整数のリテラル
	got, err = tb.items.AtTen(ctx)
	wantIDs(t, "AtTen", got, err, "i1", "i3")
	got, err = tb.items.Retired(ctx)
	wantIDs(t, "Retired", got, err, "i1", "i5")
	got, err = tb.items.Unshelved(ctx)
	wantIDs(t, "Unshelved", got, err, "i5")
	// 省略可能な列: 値のない行は、合わない
	got, err = tb.items.Fragile(ctx)
	wantIDs(t, "Fragile", got, err, "i2")
	// エスケープの要る文字列
	got, err = tb.items.Odd(ctx)
	wantIDs(t, "Odd", got, err, "i5")
	// 二つの @db.where と、引数
	got, err = tb.items.ActiveBooksOn(ctx, 1)
	wantIDs(t, "ActiveBooksOn(1)", got, err, "i2", "i4")
	got, err = tb.items.ActiveBooksOn(ctx, 2)
	wantIDs(t, "ActiveBooksOn(2)", got, err)
	got, err = tb.ops.ActiveBooks(ctx, 1)
	wantIDs(t, "op ActiveBooks(1)", got, err, "i2", "i4")
}

// TestLatestList は @db.list と @db.latest: 列の新しい順（値の大きい順）。値が同じ行は、後から足した行が先。値のない行は、最後。
func TestLatestList(t *testing.T) {
	tb, ctx := open(t)
	seed(t, tb, ctx)
	// @db.time: day3 の i3、day2 の i4 と i2（後から足した i4 が先）、day1 の i5 と i1
	got, err := tb.items.NewestFirst(ctx)
	wantIDs(t, "NewestFirst", got, err, "i3", "i4", "i2", "i5", "i1")
	// @db.int: 7 の i3、3 の i4 と i2、0 の i5 と i1
	got, err = tb.items.MostStocked(ctx)
	wantIDs(t, "MostStocked", got, err, "i3", "i4", "i2", "i5", "i1")
	// 省略可能な @db.time の列と、引数の条件: 棚 1 の、売れた i2、値のない i4 と i1（値のない行は最後。その中は、後から足した行が先）
	got, err = tb.items.LastSoldOn(ctx, 1)
	wantIDs(t, "LastSoldOn(1)", got, err, "i2", "i4", "i1")
	// 引数と @db.where と @db.latest
	got, err = tb.items.NewestActiveOf(ctx, "book")
	wantIDs(t, "NewestActiveOf(book)", got, err, "i4", "i2")

	// 省略可能な @db.float の列: 値の大きい順、値のない行は最後
	for _, a := range []shop.Audit{
		{Item: "a", At: day1, Weight: ptr(1.5)},
		{Item: "b", At: day2},
		{Item: "c", At: day3, Weight: ptr(20.0)},
		{Item: "d", At: day1, Weight: ptr(1.5)},
		{Item: "e", At: day2},
	} {
		if err := tb.audits.CreateAudit(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	audits, err := tb.audits.Heaviest(ctx)
	var order []shop.ItemId
	for _, a := range audits {
		order = append(order, a.Item)
	}
	if want := []shop.ItemId{"c", "d", "a", "e", "b"}; err != nil || !reflect.DeepEqual(order, want) {
		t.Errorf("Heaviest = %v, %v（期待は %v）", order, err, want)
	}
}

// TestLatestFind は @db.find と @db.latest: 条件に合う行のうち、いちばん新しい一行。条件がなくてもよい。行がなければ nil。
func TestLatestFind(t *testing.T) {
	tb, ctx := open(t)
	// 行がない
	if got, err := tb.audits.LastAudit(ctx); err != nil || got != nil {
		t.Errorf("空の表の LastAudit = %+v, %v（期待は nil, nil）", got, err)
	}
	if got, err := tb.items.NewestActive(ctx); err != nil || got != nil {
		t.Errorf("空の表の NewestActive = %+v, %v（期待は nil, nil）", got, err)
	}
	for _, a := range []shop.Audit{
		{Item: "a", At: day2, Weight: ptr(1.0)},
		{Item: "b", At: day3, Weight: ptr(2.0)},
		{Item: "a", At: day1, Weight: ptr(3.0)},
		{Item: "c", At: day3, Weight: ptr(4.0)},
	} {
		if err := tb.audits.CreateAudit(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	// 条件がない: いちばん新しい day3 の二行のうち、後から足した c
	if got, err := tb.audits.LastAudit(ctx); err != nil || got == nil || got.Item != "c" {
		t.Errorf("LastAudit = %+v, %v（期待は c）", got, err)
	}
	if got, err := tb.ops.LatestAudit(ctx); err != nil || got == nil || got.Item != "c" {
		t.Errorf("op LatestAudit = %+v, %v（期待は c）", got, err)
	}
	// 条件がある: a の行のうち、いちばん新しい day2（足した順では、最初）
	if got, err := tb.audits.LastAuditOf(ctx, "a"); err != nil || got == nil || got.Weight == nil || *got.Weight != 1.0 {
		t.Errorf("LastAuditOf(a) = %+v, %v（期待は weight 1）", got, err)
	}
	if got, err := tb.audits.LastAuditOf(ctx, "nobody"); err != nil || got != nil {
		t.Errorf("LastAuditOf(nobody) = %+v, %v（期待は nil, nil）", got, err)
	}
	// @db.where の条件だけ: 売り出し中の行のうち、いちばん新しい i3
	seed(t, tb, ctx)
	if got, err := tb.items.NewestActive(ctx); err != nil || got == nil || got.Id != "i3" {
		t.Errorf("NewestActive = %+v, %v（期待は i3）", got, err)
	}
}

// TestShaped は、形で決まる問い合わせ: IsEmpty と Get（足した順）。
func TestShaped(t *testing.T) {
	tb, ctx := open(t)
	if empty, err := tb.items.IsEmpty(ctx); err != nil || !empty {
		t.Errorf("空の表の IsEmpty = %v, %v（期待は true）", empty, err)
	}
	if err := tb.ops.AddItem(ctx, items[0]); err != nil {
		t.Fatal(err)
	}
	if empty, err := tb.items.IsEmpty(ctx); err != nil || empty {
		t.Errorf("行のある表の IsEmpty = %v, %v（期待は false）", empty, err)
	}
	got, err := tb.items.Get(ctx)
	wantIDs(t, "Get", got, err, "i2")
	if len(got) == 1 {
		want := items[0]
		g := got[0]
		if !g.AddedAt.Equal(want.AddedAt) || g.SoldAt == nil || !g.SoldAt.Equal(*want.SoldAt) {
			t.Errorf("読んだ時刻が AddedAt = %v、SoldAt = %v", g.AddedAt, g.SoldAt)
		}
		g.AddedAt, g.SoldAt = want.AddedAt, want.SoldAt // 時刻は Equal で比べた（Location が違う）
		if !reflect.DeepEqual(g, want) {
			t.Errorf("読んだ行が %+v（期待は %+v）", g, want)
		}
	}
}
