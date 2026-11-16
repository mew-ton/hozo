// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: db の語彙のデコレータを付けていない op が
// あるとき（docs/plugin-go-sqlite.md「状態ごとの型」「状態をまたぐ単位の型」）。その型は、その op のメソッドを持たず、基底の口を
// 満たさない。人は、その型を自分の型に埋め込み、足りないメソッド（<C>Table なら、ケースの述語も）を書き足す。
// plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/app/gen/lib"

	_ "modernc.org/sqlite"
)

// books は、人が書き足した型: go-sqlite の型を埋め込み、デコレータを付けていない op のメソッドを書く。
type books struct {
	lib.BooksTable
	db *sql.DB
}

// NewestBooks は、順序と件数（デコレータで言えないもの）を、人が書く。
func (b books) NewestBooks(ctx context.Context, limit int64) ([]lib.Book, error) {
	all, err := b.Get(ctx)
	if err != nil {
		return nil, err
	}
	var out []lib.Book
	for i := len(all) - 1; i >= 0 && int64(len(out)) < limit; i-- {
		out = append(out, all[i])
	}
	return out, nil
}

func (b books) CountBooks(ctx context.Context) (int64, error) {
	var n int64
	err := b.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "books"`).Scan(&n)
	return n, err
}

func (b books) ClearBooks(ctx context.Context) error {
	_, err := b.db.ExecContext(ctx, `DELETE FROM "books"`)
	return err
}

// drafts は、@db.table のない状態の単位の、人が書いた実装。
type drafts struct{}

func (drafts) IsEmpty(context.Context) (bool, error)          { return true, nil }
func (drafts) Get(context.Context) ([]lib.Note, error)        { return nil, nil }
func (drafts) ListDrafts(context.Context) ([]lib.Note, error) { return nil, nil }

// lending は、述語で決まる状態の分類 Lending の単位: go-sqlite の型（命令）を埋め込み、ケースの述語と、デコレータを付けていない
// 命令を書き足す。
type lending struct {
	lib.LendingTable
	db *sql.DB
}

func (l lending) Free(ctx context.Context) (bool, error) {
	lent, err := l.Lent(ctx)
	return !lent, err
}

func (l lending) Lent(ctx context.Context) (bool, error) {
	var n int64
	err := l.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "loans"`).Scan(&n)
	return n > 0, err
}

func (l lending) ReturnBook(ctx context.Context, book lib.BookId) error {
	_, err := l.db.ExecContext(ctx, `DELETE FROM "loans" WHERE "book" = ?1`, string(book))
	return err
}

// shelving は、命令のすべてにデコレータの付いた分類 Shelving の単位: それでも、述語は人が書く。
type shelving struct{ lib.ShelvingTable }

func (shelving) Open(context.Context) (bool, error)   { return true, nil }
func (shelving) Closed(context.Context) (bool, error) { return false, nil }

// shared は SharedState: go-sqlite の型を埋め込み、複数の状態を読む op を書き足す。
type shared struct {
	lib.SharedTable
	db *sql.DB
}

func (s shared) Overdue(ctx context.Context) ([]lib.Book, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b."id", b."title", b."pages" FROM "books" b JOIN "loans" l ON l."book" = b."id" ORDER BY b.rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []lib.Book
	for rows.Next() {
		var b lib.Book
		if err := rows.Scan(&b.Id, &b.Title, &b.Pages); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// 書き足した型は、基底の口を満たす（満たさなければ、ここでコンパイルできない）。
var (
	_ lib.BooksState    = books{}
	_ lib.LendingState  = lending{}
	_ lib.ShelvingState = shelving{}
	_ lib.SharedState   = shared{}
)

// TestPartialTypes は、デコレータを付けていない op を持つ単位の型が、基底の口を満たさないこと（基底の New に渡す行は、
// コンパイルできないのでここには書けない。リフレクションで確かめる）と、すべてに付けた状態の型は満たすこと。
// 述語で決まる状態の分類の型は、命令のすべてにデコレータが付いていても、述語を持たないので、満たさない。
func TestPartialTypes(t *testing.T) {
	for _, c := range []struct {
		typ  any
		unit reflect.Type
		want bool
	}{
		{lib.BooksTable{}, reflect.TypeOf((*lib.BooksState)(nil)).Elem(), false},
		{lib.NotesTable{}, reflect.TypeOf((*lib.NotesState)(nil)).Elem(), true},
		{lib.LoansTable{}, reflect.TypeOf((*lib.LoansState)(nil)).Elem(), true},
		{lib.ArchiveTable{}, reflect.TypeOf((*lib.ArchiveState)(nil)).Elem(), true},
		{lib.LendingTable{}, reflect.TypeOf((*lib.LendingState)(nil)).Elem(), false},
		{lib.ShelvingTable{}, reflect.TypeOf((*lib.ShelvingState)(nil)).Elem(), false},
		{lib.SharedTable{}, reflect.TypeOf((*lib.SharedState)(nil)).Elem(), false},
	} {
		if got := reflect.TypeOf(c.typ).Implements(c.unit); got != c.want {
			t.Errorf("%T が %s を満たすか = %v（期待は %v）", c.typ, c.unit.Name(), got, c.want)
		}
	}
	// 持つメソッドは、デコレータを付けた op のものだけ
	for typ, want := range map[any][]string{
		lib.BooksTable{}:    {"AddBook", "FindBook", "Get", "IsEmpty"},
		lib.LendingTable{}:  {"LendBook", "LendRenamed"},
		lib.ShelvingTable{}: {"ShelveBook"},
		lib.SharedTable{}:   {"AddAndLend", "AddWithNote"},
	} {
		rt := reflect.TypeOf(typ)
		var got []string
		for i := 0; i < rt.NumMethod(); i++ {
			got = append(got, rt.Method(i).Name)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s のメソッドが %v（期待は %v）", rt.Name(), got, want)
		}
	}
}

func newLib(t *testing.T) (lib.Ops, *sql.DB, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := lib.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	ops := lib.New(books{lib.NewBooksTable(db), db}, lib.NewNotesTable(db), lib.NewLoansTable(db), lib.NewArchiveTable(db), lib.NewStampsTable(db), drafts{},
		lending{lib.NewLendingTable(db), db}, shelving{lib.NewShelvingTable(db)}, shared{lib.NewSharedTable(db), db})
	return ops, db, ctx
}

// TestEmbedded は、埋め込んで書き足した型を基底の New に渡せば、go-sqlite のメソッドと、人が書いたメソッドが、同じ表の上で動くこと。
func TestEmbedded(t *testing.T) {
	ops, db, ctx := newLib(t)
	for _, b := range []lib.Book{{Id: "b1", Title: "one", Pages: 10}, {Id: "b2", Title: "two", Pages: 20}, {Id: "b3", Title: "three", Pages: 30}} {
		if err := ops.AddBook(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := ops.FindBook(ctx, "b2"); err != nil || got == nil || got.Title != "two" {
		t.Errorf("FindBook(b2) = %+v, %v", got, err)
	}
	if got, err := ops.NewestBooks(ctx, 2); err != nil || len(got) != 2 || got[0].Id != "b3" || got[1].Id != "b2" {
		t.Errorf("NewestBooks(2) = %v, %v", got, err)
	}
	if n, err := ops.CountBooks(ctx); err != nil || n != 3 {
		t.Errorf("CountBooks = %d, %v", n, err)
	}

	// 基底の状態の遷移を宛先にする効果（notes: Empty -> HasItems）
	notes := lib.NewNotesTable(db)
	if err := ops.StartNotes(ctx, lib.Note{Text: "first", Stars: 1}); err != nil {
		t.Fatal(err)
	}
	if err := ops.SwapNote(ctx, "first", lib.Note{Text: "second", Stars: 2}); err != nil {
		t.Fatal(err)
	}
	if got, err := notes.Get(ctx); err != nil || len(got) != 1 || got[0].Text != "second" {
		t.Errorf("SwapNote の後の notes = %v, %v", got, err)
	}

	// 述語で決まる状態の分類の単位の命令（<C>Table）: 状態の分類の遷移を宛先にする効果が、表 loans に行を足す
	if err := ops.LendBook(ctx, lib.Loan{Book: "b1", Member: "m1"}); err != nil {
		t.Fatal(err)
	}
	// 二つの効果の宛先の単位が、どちらも Lending: 一つのトランザクションで、books を書き換え、loans に足す
	if err := ops.LendRenamed(ctx, lib.Book{Id: "b2", Title: "TWO", Pages: 21}, lib.Loan{Book: "b2", Member: "m2"}); err != nil {
		t.Fatal(err)
	}
	if got, err := ops.FindBook(ctx, "b2"); err != nil || got == nil || got.Title != "TWO" || got.Pages != 21 {
		t.Errorf("LendRenamed の後の FindBook(b2) = %+v, %v", got, err)
	}
	// 二つ目（loans の同じキー）が失敗すれば、一つ目（books の書き換え）も残らない
	if err := ops.LendRenamed(ctx, lib.Book{Id: "b3", Title: "THREE", Pages: 31}, lib.Loan{Book: "b1", Member: "m3"}); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("二つ目が失敗する LendRenamed のエラーが %v", err)
	}
	if got, err := ops.FindBook(ctx, "b3"); err != nil || got == nil || got.Title != "three" {
		t.Errorf("失敗した LendRenamed の後の FindBook(b3) = %+v, %v（期待は、書き換わっていない）", got, err)
	}
	if got, err := ops.Overdue(ctx); err != nil || len(got) != 2 || got[0].Id != "b1" || got[1].Id != "b2" {
		t.Errorf("Overdue = %v, %v", got, err)
	}
	if err := ops.ReturnBook(ctx, "b1"); err != nil {
		t.Fatal(err)
	}
	if got, err := lib.NewLoansTable(db).Get(ctx); err != nil || len(got) != 1 || got[0].Book != "b2" {
		t.Errorf("ReturnBook の後の loans = %v, %v", got, err)
	}

	// 宛先の単位が分かれる op（SharedTable）: books と notes に、一緒に足す
	if err := ops.AddWithNote(ctx, lib.Book{Id: "b4", Title: "four", Pages: 40}, lib.Note{Text: "about four", Stars: 4}); err != nil {
		t.Fatal(err)
	}
	// 一つ目（books の同じキー）が失敗すれば、notes にも足さない
	if err := ops.AddWithNote(ctx, lib.Book{Id: "b4", Title: "again", Pages: 1}, lib.Note{Text: "never", Stars: 0}); err == nil {
		t.Fatal("一つ目が失敗する AddWithNote が、エラーを返さない")
	}
	// 二つ目（loans の、先のない book ではなく、同じキー）が失敗すれば、books にも足さない
	if err := ops.AddAndLend(ctx, lib.Book{Id: "b5", Title: "five", Pages: 50}, lib.Loan{Book: "b2", Member: "m5"}); err == nil {
		t.Fatal("二つ目が失敗する AddAndLend が、エラーを返さない")
	}
	if err := ops.AddAndLend(ctx, lib.Book{Id: "b6", Title: "six", Pages: 60}, lib.Loan{Book: "b6", Member: "m6"}); err != nil {
		t.Fatal(err)
	}
	if n, err := ops.CountBooks(ctx); err != nil || n != 5 {
		t.Errorf("CountBooks = %d, %v（期待は 5: b1〜b4 と b6）", n, err)
	}
	if got, err := notes.Get(ctx); err != nil || len(got) != 2 || got[1].Text != "about four" {
		t.Errorf("notes = %v, %v", got, err)
	}

	// 命令のすべてにデコレータの付いた分類の単位
	if err := ops.ShelveBook(ctx, lib.Archived{Id: "old"}); err != nil {
		t.Fatal(err)
	}
	if got, err := lib.NewArchiveTable(db).Get(ctx); err != nil || len(got) != 1 || got[0].Id != "old" {
		t.Errorf("archive = %v, %v", got, err)
	}

	// 行を受け取る op を持たない表（dbArgsStamp は出ない）: 読むことと、消すことはできる
	stampAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO "stamps" ("at") VALUES ('2026-02-03T04:05:06.000000000Z'), ('2026-02-03T04:05:07.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if err := ops.Unstamp(ctx, stampAt); err != nil {
		t.Fatal(err)
	}
	if got, err := lib.NewStampsTable(db).Get(ctx); err != nil || len(got) != 1 || !got[0].At.Equal(stampAt.Add(time.Second)) {
		t.Errorf("Unstamp の後の stamps = %v, %v", got, err)
	}

	// 人が書いた命令
	if err := ops.ClearBooks(ctx); err == nil {
		// loans が books を指している間は、消せない（外部キー）
		t.Error("貸し出している本がある間の ClearBooks が、エラーを返さない")
	}
}
