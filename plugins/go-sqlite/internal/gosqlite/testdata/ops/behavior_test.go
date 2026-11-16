// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 「op の中身」の規則の各行
// （docs/plugin-go-sqlite.md）を、本物の SQLite の上で確かめる。plugins/go-sqlite の TestBehavior が、基底の出力と
// go-sqlite の出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"example.com/app/gen/shop"

	_ "modernc.org/sqlite"
)

// newShop は、新しいメモリの上の SQLite に表を作り、その上の shop の Ops を返す。go-sqlite の二つの型を、基底の New に
// そのまま渡す（どちらも状態の単位を満たす。満たさなければ、ここでコンパイルできない）。
func newShop(t *testing.T) (shop.Ops, *sql.DB, context.Context) {
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
	return shop.New(shop.NewItemsTable(db), shop.NewStockTable(db)), db, ctx
}

var (
	day1 = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	day2 = time.Date(2026, 1, 2, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	// 足す順: i2、i1、i3（足した順が、キーの順とは違う）
	items = []shop.Item{
		{Id: "i2", Sku: "s2", Shelf: 1, Name: "pear", Active: true, AddedAt: day1, Tags: []string{"fruit"}},
		{Id: "i1", Sku: "s1", Shelf: 1, Name: "apple", Active: false, AddedAt: day2, Tags: []string{"fruit", "red"}},
		{Id: "i3", Sku: "s3", Shelf: 2, Name: "leek", Active: true, AddedAt: day1, Tags: nil},
	}
)

func seed(t *testing.T, s shop.Ops, ctx context.Context) {
	t.Helper()
	for _, it := range items {
		if err := s.AddItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
}

func ids(list []shop.Item) []shop.ItemId {
	var out []shop.ItemId
	for _, it := range list {
		out = append(out, it.Id)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

// TestGet は「一行を引く」: キーか一意の列で引く。なければ nil。条件が二つなら、どちらも合う行。
func TestGet(t *testing.T) {
	s, _, ctx := newShop(t)
	seed(t, s, ctx)
	got, err := s.FindItem(ctx, "i1")
	if err != nil || got == nil || got.Name != "apple" || !got.AddedAt.Equal(day2) || !reflect.DeepEqual(got.Tags, []string{"fruit", "red"}) {
		t.Errorf("FindItem(i1) = %+v, %v", got, err)
	}
	if got, err := s.FindItem(ctx, "nobody"); err != nil || got != nil {
		t.Errorf("FindItem(nobody) = %+v, %v（期待は nil, nil）", got, err)
	}
	if got, err := s.FindBySku(ctx, "s3"); err != nil || got == nil || got.Id != "i3" {
		t.Errorf("FindBySku(s3) = %+v, %v", got, err)
	}
	if got, err := s.FindOnShelf(ctx, 1, "s2"); err != nil || got == nil || got.Id != "i2" {
		t.Errorf("FindOnShelf(1, s2) = %+v, %v", got, err)
	}
	if got, err := s.FindOnShelf(ctx, 2, "s2"); err != nil || got != nil {
		t.Errorf("FindOnShelf(2, s2) = %+v, %v（期待は nil, nil）", got, err)
	}
}

// TestOptionalArgs は、省略可能な引数: 値がなければ、問い合わせずに「ない」。
func TestOptionalArgs(t *testing.T) {
	s, db, ctx := newShop(t)
	seed(t, s, ctx)
	if got, err := s.FindNamed(ctx, ptr(shop.ItemId("i2")), ptr("pear")); err != nil || got == nil || got.Id != "i2" {
		t.Errorf("FindNamed(i2, pear) = %+v, %v", got, err)
	}
	if got, err := s.FindNamed(ctx, ptr(shop.ItemId("i2")), ptr("apple")); err != nil || got != nil {
		t.Errorf("FindNamed(i2, apple) = %+v, %v（期待は nil, nil）", got, err)
	}
	// 問い合わせないことは、閉じたデータベースでもエラーにならないことで確かめる
	closed, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	c := shop.New(shop.NewItemsTable(closed), shop.NewStockTable(closed))
	if got, err := c.FindNamed(ctx, nil, ptr("pear")); err != nil || got != nil {
		t.Errorf("FindNamed(nil, pear) = %+v, %v（期待は nil, nil）", got, err)
	}
	if got, err := c.FindNamed(ctx, ptr(shop.ItemId("i2")), nil); err != nil || got != nil {
		t.Errorf("FindNamed(i2, nil) = %+v, %v（期待は nil, nil）", got, err)
	}
	if got, err := c.ListActive(ctx, true, nil); err != nil || got != nil {
		t.Errorf("ListActive(true, nil) = %+v, %v（期待は nil, nil）", got, err)
	}
	if got, err := c.ListTagged(ctx, nil); err != nil || got != nil {
		t.Errorf("ListTagged(nil) = %+v, %v（期待は nil, nil）", got, err)
	}
	if err := c.RemoveStock(ctx, nil, 1); err != nil {
		t.Errorf("RemoveStock(nil, 1) = %v（期待は nil）", err)
	}
	// 値があれば、問い合わせる（閉じたデータベースではエラー）
	if _, err := c.FindNamed(ctx, ptr(shop.ItemId("i2")), ptr("pear")); err == nil {
		t.Error("閉じたデータベースへの問い合わせが、エラーにならない")
	}
	_ = db
}

// TestList は「並びを引く」: 足した順。条件は、列の「書く値」の形で比べる（真偽値、Time、JSON）。
func TestList(t *testing.T) {
	s, _, ctx := newShop(t)
	if got, err := s.ListItems(ctx); err != nil || got != nil {
		t.Errorf("空の表の ListItems = %v, %v（期待は nil, nil）", got, err)
	}
	seed(t, s, ctx)
	check := func(name string, got []shop.Item, err error, want ...shop.ItemId) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(ids(got), want) {
			t.Errorf("%s = %v, %v（期待は %v）", name, ids(got), err, want)
		}
	}
	got, err := s.ListItems(ctx)
	check("ListItems", got, err, "i2", "i1", "i3")
	got, err = s.ListOnShelf(ctx, 1)
	check("ListOnShelf(1)", got, err, "i2", "i1")
	got, err = s.ListOnShelf(ctx, 9)
	check("ListOnShelf(9)", got, err)
	got, err = s.ListActive(ctx, true, ptr(shop.Shelf(1)))
	check("ListActive(true, 1)", got, err, "i2")
	got, err = s.ListActive(ctx, false, ptr(shop.Shelf(1)))
	check("ListActive(false, 1)", got, err, "i1")
	// 同じ時刻なら、タイムゾーンの書き方が違っても合う（列は UTC）
	got, err = s.ListAddedAt(ctx, day1.In(time.FixedZone("JST", 9*60*60)))
	check("ListAddedAt(day1)", got, err, "i2", "i3")
	got, err = s.ListTagged(ctx, []string{"fruit", "red"})
	check("ListTagged(fruit, red)", got, err, "i1")
	got, err = s.ListTagged(ctx, []string{})
	check("ListTagged([])", got, err)
}

// TestInsertUpdateDelete は「足す」「書き換える」「消す」。
func TestInsertUpdateDelete(t *testing.T) {
	s, db, ctx := newShop(t)
	seed(t, s, ctx)
	// 足す: 同じキーは足せない（@db.key）
	if err := s.AddItem(ctx, items[0]); err == nil {
		t.Error("同じキーの行を足せた")
	}
	// 書き換える: キーが同じ行の、キーでない列を書き換える。足した順は変わらない
	changed := items[1]
	changed.Name, changed.Active, changed.Tags, changed.AddedAt = "green apple", true, []string{"green"}, day1
	if err := s.SaveItem(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if got, err := s.FindItem(ctx, "i1"); err != nil || got == nil || got.Name != "green apple" || !got.Active || !reflect.DeepEqual(got.Tags, []string{"green"}) || !got.AddedAt.Equal(day1) {
		t.Errorf("書き換えた行が %+v, %v", got, err)
	}
	if got, err := s.ListItems(ctx); err != nil || !reflect.DeepEqual(ids(got), []shop.ItemId{"i2", "i1", "i3"}) {
		t.Errorf("書き換えた後の ListItems = %v, %v", ids(got), err)
	}
	// 行がなければ何もしない
	if err := s.SaveItem(ctx, shop.Item{Id: "nobody", Sku: "s9"}); err != nil {
		t.Errorf("ない行の SaveItem = %v（期待は nil）", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "items"`).Scan(&n); err != nil || n != 3 {
		t.Errorf("行の数が %d, %v（期待は 3）", n, err)
	}
	// 消す: 条件が合う行だけ。行がなければ何もしない
	if err := s.RemoveFromShelf(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListItems(ctx); !reflect.DeepEqual(ids(got), []shop.ItemId{"i2", "i1", "i3"}) {
		t.Errorf("合う行のない RemoveFromShelf の後が %v", ids(got))
	}
	if err := s.RemoveFromShelf(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListItems(ctx); !reflect.DeepEqual(ids(got), []shop.ItemId{"i3"}) {
		t.Errorf("RemoveFromShelf(1, true) の後が %v", ids(got))
	}
	if err := s.RemoveItem(ctx, "i3"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveItem(ctx, "i3"); err != nil {
		t.Errorf("ない行の RemoveItem = %v（期待は nil）", err)
	}
	if empty, err := shop.NewItemsTable(db).IsEmpty(ctx); err != nil || !empty {
		t.Errorf("IsEmpty = %v, %v（期待は true）", empty, err)
	}
}

// TestStock は、キーが二つの表: 一行を引く、書き換える（キーの組が同じ行だけ）、消す。
func TestStock(t *testing.T) {
	s, _, ctx := newShop(t)
	for _, st := range []shop.Stock{{Item: "i1", Shelf: 1, Count: 5}, {Item: "i1", Shelf: 2, Count: 7}, {Item: "i2", Shelf: 1, Count: 9}} {
		if err := s.AddStock(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveStock(ctx, shop.Stock{Item: "i1", Shelf: 2, Count: 70}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []shop.Stock{{Item: "i1", Shelf: 1, Count: 5}, {Item: "i1", Shelf: 2, Count: 70}, {Item: "i2", Shelf: 1, Count: 9}} {
		if got, err := s.FindStock(ctx, want.Item, want.Shelf); err != nil || got == nil || *got != want {
			t.Errorf("FindStock(%s, %d) = %+v, %v（期待は %+v）", want.Item, want.Shelf, got, err, want)
		}
	}
	if err := s.RemoveStock(ctx, ptr(shop.ItemId("i1")), 1); err != nil {
		t.Fatal(err)
	}
	if got, err := s.FindStock(ctx, "i1", 1); err != nil || got != nil {
		t.Errorf("消した行の FindStock = %+v, %v", got, err)
	}
	if got, err := s.FindStock(ctx, "i1", 2); err != nil || got == nil {
		t.Errorf("消していない行の FindStock = %+v, %v", got, err)
	}
}

// TestContext は、受け取った ctx で問い合わせること: 中断された ctx では、どのメソッドもエラーを返す。
func TestContext(t *testing.T) {
	s, db, _ := newShop(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.FindItem(ctx, "i1"); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx での FindItem のエラーが %v", err)
	}
	if _, err := s.ListItems(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx での ListItems のエラーが %v", err)
	}
	if err := s.AddItem(ctx, items[0]); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx での AddItem のエラーが %v", err)
	}
	if err := s.RemoveItem(ctx, "i1"); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx での RemoveItem のエラーが %v", err)
	}
	if _, err := shop.NewItemsTable(db).IsEmpty(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx での IsEmpty のエラーが %v", err)
	}
	if err := shop.CreateTables(ctx, db); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx での CreateTables のエラーが %v", err)
	}
}
