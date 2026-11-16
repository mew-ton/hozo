// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 「op の中身」の表の各行、
// 「列と引数の型」の、合う行の表、「トランザクション」（docs/plugin-go-sqlite.md）を、本物の SQLite の上で確かめる。
// plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/app/gen/shop"

	_ "modernc.org/sqlite"
)

// newShop は、新しいメモリの上の SQLite に表を作り、その上の shop の Ops を返す。go-sqlite の四つの型を、基底の New に
// そのまま渡す（どれも、状態の単位か SharedState を満たす。満たさなければ、ここでコンパイルできない）。
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
	return shop.New(shop.NewItemsTable(db), shop.NewStockTable(db), shop.NewAuditsTable(db), shop.NewSharedTable(db)), db, ctx
}

func ptr[T any](v T) *T { return &v }

var (
	day1 = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	day2 = time.Date(2026, 1, 2, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	// 足す順: i2、i1、i3（足した順が、キーの順とは違う）
	items = []shop.Item{
		{Id: "i2", Sku: "s2", Shelf: 1, Name: "pear", Active: true, AddedAt: day1, Tags: []string{"fruit"}, Note: ptr("ripe")},
		{Id: "i1", Sku: "s1", Shelf: 1, Name: "apple", Active: false, AddedAt: day2, Tags: []string{"fruit", "red"}},
		{Id: "i3", Sku: "s3", Shelf: 2, Name: "leek", Active: true, AddedAt: day1, Tags: []string{}, Note: ptr("")},
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
	out := []shop.ItemId{}
	for _, it := range list {
		out = append(out, it.Id)
	}
	return out
}

// wantIDs は、引いた行の識別子が、その順で want であること。
func wantIDs(t *testing.T, what string, got []shop.Item, err error, want ...shop.ItemId) {
	t.Helper()
	if want == nil {
		want = []shop.ItemId{}
	}
	if err != nil || !reflect.DeepEqual(ids(got), want) {
		t.Errorf("%s = %v, %v（期待は %v）", what, ids(got), err, want)
	}
}

// count は、表 table の行の数。
func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "` + table + `"`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestFind は @db.find: キーか一意の列で、一行を引く。なければ nil。
func TestFind(t *testing.T) {
	s, _, ctx := newShop(t)
	seed(t, s, ctx)
	got, err := s.FindItem(ctx, "i1")
	if err != nil || got == nil || got.Name != "apple" || !got.AddedAt.Equal(day2) || !reflect.DeepEqual(got.Tags, []string{"fruit", "red"}) || got.Note != nil {
		t.Errorf("FindItem(i1) = %+v, %v", got, err)
	}
	if got, err := s.FindItem(ctx, "nobody"); err != nil || got != nil {
		t.Errorf("FindItem(nobody) = %+v, %v（期待は nil, nil）", got, err)
	}
	if got, err := s.FindBySku(ctx, "s3"); err != nil || got == nil || got.Id != "i3" || got.Note == nil || *got.Note != "" {
		t.Errorf("FindBySku(s3) = %+v, %v", got, err)
	}
}

// TestList は @db.list: 列を渡さなければすべての行、渡せば、その列が引数と同じ行。どちらも、足した順。しまい方ごとに、引数の値を
// 列の値と同じ形にして比べる。
func TestList(t *testing.T) {
	s, _, ctx := newShop(t)
	got, err := s.ListItems(ctx)
	if err != nil || got != nil {
		t.Errorf("空の表の ListItems = %v, %v（期待は nil, nil）", got, err)
	}
	seed(t, s, ctx)
	got, err = s.ListItems(ctx)
	wantIDs(t, "ListItems", got, err, "i2", "i1", "i3")
	// @db.int
	got, err = s.ListOnShelf(ctx, 1)
	wantIDs(t, "ListOnShelf(1)", got, err, "i2", "i1")
	got, err = s.ListOnShelf(ctx, 9)
	wantIDs(t, "ListOnShelf(9)", got, err)
	// @db.bool
	got, err = s.ListActive(ctx, true)
	wantIDs(t, "ListActive(true)", got, err, "i2", "i3")
	got, err = s.ListActive(ctx, false)
	wantIDs(t, "ListActive(false)", got, err, "i1")
	// @db.time: 同じ時刻なら、Location が違っても合う
	got, err = s.ListAddedAt(ctx, day1.In(time.FixedZone("EST", -5*60*60)))
	wantIDs(t, "ListAddedAt(day1)", got, err, "i2", "i3")
	// @db.json: 同じ JSON の行
	got, err = s.ListTagged(ctx, []string{"fruit", "red"})
	wantIDs(t, "ListTagged(fruit, red)", got, err, "i1")
	got, err = s.ListTagged(ctx, []string{})
	wantIDs(t, "ListTagged()", got, err, "i3")
	// 複数の @db.key の一つ
	for _, st := range []shop.Stock{{Item: "i1", Shelf: 1, Count: 3}, {Item: "i1", Shelf: 2, Count: 4}, {Item: "i2", Shelf: 1, Count: 5}} {
		if err := s.AddStock(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.ListStock(ctx, "i1"); err != nil || len(got) != 2 || got[0].Count != 3 || got[1].Count != 4 {
		t.Errorf("ListStock(i1) = %v, %v", got, err)
	}
}

// TestColumnAndParam は「列と引数の型」の、合う行の表: T? の列に T の引数なら、値のない行（NULL）は、どの値にも合わない。
// T? の列に T? の引数なら、値があれば同じ、値がなければ、値のない行。条件は、どちらも同じ `"c" IS ?1` で、違いは、実行時の値。
func TestColumnAndParam(t *testing.T) {
	s, _, ctx := newShop(t)
	seed(t, s, ctx)
	got, err := s.ListNoted(ctx, "ripe")
	wantIDs(t, "ListNoted(ripe)", got, err, "i2")
	// 空の文字列は値で、NULL には合わない
	got, err = s.ListNoted(ctx, "")
	wantIDs(t, "ListNoted(\"\")", got, err, "i3")
	got, err = s.ListByNote(ctx, ptr("ripe"))
	wantIDs(t, "ListByNote(ripe)", got, err, "i2")
	got, err = s.ListByNote(ctx, ptr(""))
	wantIDs(t, "ListByNote(\"\")", got, err, "i3")
	// 値のない引数は、値のない行に合う（問い合わせをやめて「ない」を返すことはしない）
	got, err = s.ListByNote(ctx, nil)
	wantIDs(t, "ListByNote(nil)", got, err, "i1")
}

// TestSingleEffect は、効果が一つの @db.effects: @db.insert は行を足し、@db.update は、キーが同じ行を書き換え、@db.delete は、
// 列が引数と同じ行を消す。行がなければ何もしない。
func TestSingleEffect(t *testing.T) {
	s, db, ctx := newShop(t)
	seed(t, s, ctx)
	// @db.insert: 同じキーの行は、エラー
	if err := s.AddItem(ctx, items[0]); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("同じキーの行を足したときのエラーが %v", err)
	}
	// @db.update: キーが一つ。キーでない列を、すべて書き換える
	changed := items[1]
	changed.Name, changed.Shelf, changed.Active, changed.Note, changed.Tags = "green apple", 3, true, ptr("sour"), nil
	if err := s.SaveItem(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if got, err := s.FindItem(ctx, "i1"); err != nil || got == nil || got.Name != "green apple" || got.Shelf != 3 || !got.Active || got.Note == nil || *got.Note != "sour" || got.Tags != nil {
		t.Errorf("書き換えた後の FindItem(i1) = %+v, %v", got, err)
	}
	if got, err := s.FindItem(ctx, "i2"); err != nil || got == nil || got.Name != "pear" {
		t.Errorf("書き換えていない行 FindItem(i2) = %+v, %v", got, err)
	}
	// 行がなければ何もしない（足しもしない）
	if err := s.SaveItem(ctx, shop.Item{Id: "nobody", Sku: "s9"}); err != nil || count(t, db, "items") != 3 {
		t.Errorf("行のない SaveItem の後: err = %v、行の数 = %d（期待は 3）", err, count(t, db, "items"))
	}
	// @db.update: キーが二つ。どちらのキーも同じ行だけ
	for _, st := range []shop.Stock{{Item: "i1", Shelf: 1, Count: 3}, {Item: "i1", Shelf: 2, Count: 4}, {Item: "i2", Shelf: 1, Count: 5}} {
		if err := s.AddStock(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveStock(ctx, shop.Stock{Item: "i1", Shelf: 2, Count: 40}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ListStock(ctx, "i1"); err != nil || len(got) != 2 || got[0].Count != 3 || got[1].Count != 40 {
		t.Errorf("SaveStock の後の ListStock(i1) = %v, %v", got, err)
	}
	if got, err := s.ListStock(ctx, "i2"); err != nil || len(got) != 1 || got[0].Count != 5 {
		t.Errorf("SaveStock の後の ListStock(i2) = %v, %v", got, err)
	}
	// @db.delete
	if err := s.RemoveItem(ctx, "i2"); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListItems(ctx)
	wantIDs(t, "RemoveItem(i2) の後の ListItems", got, err, "i1", "i3")
	if err := s.RemoveItem(ctx, "nobody"); err != nil || count(t, db, "items") != 2 {
		t.Errorf("行のない RemoveItem の後: err = %v、行の数 = %d（期待は 2）", err, count(t, db, "items"))
	}
}

// TestDeleteNullable は、@db.nullable の列の @db.delete: 値のない引数は、値のない行を消す。値があれば、同じ値の行だけ。
func TestDeleteNullable(t *testing.T) {
	s, _, ctx := newShop(t)
	seed(t, s, ctx)
	if err := s.RemoveNoted(ctx, ptr("ripe")); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListItems(ctx)
	wantIDs(t, "RemoveNoted(ripe) の後の ListItems", got, err, "i1", "i3")
	if err := s.RemoveNoted(ctx, nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListItems(ctx)
	wantIDs(t, "RemoveNoted(nil) の後の ListItems", got, err, "i3")
}

// TestTransaction は、効果が二つ以上の @db.effects: 一つのトランザクションで、書いた順に行う。どれかが失敗すれば、どれも残らない。
func TestTransaction(t *testing.T) {
	s, db, ctx := newShop(t)
	a := shop.Item{Id: "a", Sku: "sa", Name: "a", AddedAt: day1}
	b := shop.Item{Id: "b", Sku: "sb", Name: "b", AddedAt: day1}

	// 同じ効果を二度（宛先の単位は一つ: ItemsTable のメソッド）
	if err := s.AddTwo(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListItems(ctx)
	wantIDs(t, "AddTwo の後の ListItems", got, err, "a", "b")
	// 二つ目が失敗すれば（同じキー）、一つ目も残らない
	c := shop.Item{Id: "c", Sku: "sc", Name: "c", AddedAt: day1}
	if err := s.AddTwo(ctx, c, a); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("二つ目が失敗する AddTwo のエラーが %v", err)
	}
	got, err = s.ListItems(ctx)
	wantIDs(t, "失敗した AddTwo の後の ListItems", got, err, "a", "b")

	// 書いた順に行う: 消してから、同じキーの行を足し、別の行を書き換える
	a2, b2 := a, b
	a2.Name, b2.Name = "a2", "b2"
	if err := s.ReplaceItem(ctx, "a", a2, b2); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListItems(ctx)
	wantIDs(t, "ReplaceItem の後の ListItems", got, err, "b", "a")
	if got[0].Name != "b2" || got[1].Name != "a2" {
		t.Errorf("ReplaceItem の後の行が %+v", got)
	}
	// 途中（二つ目）が失敗すれば、先に消した行も、消えない
	if err := s.ReplaceItem(ctx, "a", b, b2); err == nil {
		t.Fatal("二つ目が失敗する ReplaceItem が、エラーを返さない")
	}
	got, err = s.ListItems(ctx)
	wantIDs(t, "失敗した ReplaceItem の後の ListItems", got, err, "b", "a")

	// 宛先の単位が分かれる（SharedTable のメソッド）: 三つの表に、一緒に足す
	st := shop.Stock{Item: "c", Shelf: 1, Count: 2}
	au := shop.Audit{Item: "c", At: day2}
	if err := s.Receive(ctx, c, st, au); err != nil {
		t.Fatal(err)
	}
	if count(t, db, "items") != 3 || count(t, db, "stock") != 1 || count(t, db, "audits") != 1 {
		t.Errorf("Receive の後の行の数が items = %d、stock = %d、audits = %d", count(t, db, "items"), count(t, db, "stock"), count(t, db, "audits"))
	}
	// 二つ目（stock の同じキー）が失敗すれば、一つ目（items）も、三つ目（audits）も残らない
	d := shop.Item{Id: "d", Sku: "sd", Name: "d", AddedAt: day1}
	if err := s.Receive(ctx, d, st, au); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("二つ目が失敗する Receive のエラーが %v", err)
	}
	if count(t, db, "items") != 3 || count(t, db, "stock") != 1 || count(t, db, "audits") != 1 {
		t.Errorf("失敗した Receive の後の行の数が items = %d、stock = %d、audits = %d（期待は 3、1、1）", count(t, db, "items"), count(t, db, "stock"), count(t, db, "audits"))
	}
	// 一つ目（items の同じキー）が失敗すれば、後ろは行わない
	if err := s.Receive(ctx, c, shop.Stock{Item: "c", Shelf: 2}, au); err == nil {
		t.Fatal("一つ目が失敗する Receive が、エラーを返さない")
	}
	if count(t, db, "stock") != 1 || count(t, db, "audits") != 1 {
		t.Errorf("一つ目が失敗した Receive の後の行の数が stock = %d、audits = %d（期待は 1、1）", count(t, db, "stock"), count(t, db, "audits"))
	}

	// @db.update と @db.insert を、別の表に
	if err := s.Recount(ctx, shop.Stock{Item: "c", Shelf: 1, Count: 9}, shop.Audit{Item: "c", At: day1}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ListStock(ctx, "c"); err != nil || len(got) != 1 || got[0].Count != 9 || count(t, db, "audits") != 2 {
		t.Errorf("Recount の後の ListStock(c) = %v, %v、audits の行の数 = %d", got, err, count(t, db, "audits"))
	}
	// @db.delete だけを、二つの表に
	if err := s.Discontinue(ctx, "c", "c"); err != nil {
		t.Fatal(err)
	}
	if count(t, db, "items") != 2 || count(t, db, "stock") != 0 {
		t.Errorf("Discontinue の後の行の数が items = %d、stock = %d（期待は 2、0）", count(t, db, "items"), count(t, db, "stock"))
	}
	// @db.delete だけを、一つの表に（@db.nullable の列の条件を含む）
	if err := s.Purge(ctx, "a", nil); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, "items"); n != 0 {
		t.Errorf("Purge の後の items の行の数が %d（期待は 0: a はキーで、b は、値のない note で消える）", n)
	}
}

// TestTransactionContext は、受け取った ctx でトランザクションを始めること: 取り消された ctx では、始められず、何も残らない。
func TestTransactionContext(t *testing.T) {
	s, db, ctx := newShop(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	a := shop.Item{Id: "a", Sku: "sa", Name: "a", AddedAt: day1}
	if err := s.Receive(canceled, a, shop.Stock{Item: "a", Shelf: 1}, shop.Audit{Item: "a", At: day1}); err == nil {
		t.Fatal("取り消された ctx の Receive が、エラーを返さない")
	}
	if err := s.AddItem(canceled, a); err == nil {
		t.Fatal("取り消された ctx の AddItem が、エラーを返さない")
	}
	if got, err := s.FindItem(canceled, "a"); err == nil {
		t.Fatalf("取り消された ctx の FindItem = %v, nil", got)
	}
	if count(t, db, "items") != 0 || count(t, db, "stock") != 0 || count(t, db, "audits") != 0 {
		t.Error("取り消された ctx の op の後に、行が残っている")
	}
}
