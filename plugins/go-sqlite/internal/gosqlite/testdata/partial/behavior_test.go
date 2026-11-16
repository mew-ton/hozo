// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 規則に当たらない op があるとき
// （docs/plugin-go-sqlite.md「状態ごとの型」）。その型は、その op のメソッドを持たず、基底の状態の単位を満たさない。人は、
// その型を自分の型に埋め込み、足りないメソッドを書き足す。plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の
// 出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"example.com/app/gen/lib"

	_ "modernc.org/sqlite"
)

// books は、人が書き足した型: go-sqlite の型を埋め込み、出されなかった op のメソッドを書く。
type books struct {
	lib.BooksTable
	db *sql.DB
}

func (b books) FindByTitle(ctx context.Context, title string) (*lib.Book, error) {
	all, err := b.Get(ctx)
	if err != nil {
		return nil, err
	}
	for _, book := range all {
		if book.Title == title {
			return &book, nil
		}
	}
	return nil, nil
}
func (b books) FindByPages(context.Context, int64) (*lib.Book, error) { return nil, nil }
func (b books) CountBooks(ctx context.Context) (int64, error) {
	var n int64
	err := b.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "books"`).Scan(&n)
	return n, err
}
func (b books) LentAt(context.Context, time.Time) ([]lib.Book, error)        { return nil, nil }
func (b books) Recent(context.Context, lib.BookId, bool) ([]lib.Book, error) { return nil, nil }
func (b books) AddBookAt(context.Context, lib.Book, time.Time) error         { return nil }
func (b books) AddAndCount(context.Context, lib.Book) (int64, error)         { return 0, nil }
func (b books) AddById(context.Context, lib.BookId) error                    { return nil }
func (b books) ClearBooks(ctx context.Context) error {
	_, err := b.db.ExecContext(ctx, `DELETE FROM "books"`)
	return err
}
func (b books) TakeBook(context.Context, lib.BookId) (*lib.Book, error) { return nil, nil }
func (b books) Restock(context.Context, lib.Book) error                 { return nil }

// notes と marks も同じ: 書き換える op を書き足す。
type notes struct{ lib.NotesTable }

func (notes) SaveNote(context.Context, lib.Note) error { return nil }

type marks struct{ lib.MarksTable }

func (marks) SaveMark(context.Context, lib.Mark) error { return nil }

// 書き足した型は、状態の単位を満たす（満たさなければ、ここでコンパイルできない）。
var (
	_ lib.BooksState = books{}
	_ lib.NotesState = notes{}
	_ lib.MarksState = marks{}
)

// TestPartialTypes は、規則に当たらない op を持つ状態の型が、基底の状態の単位を満たさないこと（基底の New に渡す行は、
// コンパイルできないのでここには書けない。リフレクションで確かめる）と、すべて当たった状態の型は満たすこと。
func TestPartialTypes(t *testing.T) {
	for _, c := range []struct {
		typ  any
		unit reflect.Type
		want bool
	}{
		{lib.BooksTable{}, reflect.TypeOf((*lib.BooksState)(nil)).Elem(), false},
		{lib.NotesTable{}, reflect.TypeOf((*lib.NotesState)(nil)).Elem(), false},
		{lib.MarksTable{}, reflect.TypeOf((*lib.MarksState)(nil)).Elem(), false},
		{lib.ArchiveTable{}, reflect.TypeOf((*lib.ArchiveState)(nil)).Elem(), true},
		{books{}, reflect.TypeOf((*lib.BooksState)(nil)).Elem(), true},
	} {
		if got := reflect.TypeOf(c.typ).Implements(c.unit); got != c.want {
			t.Errorf("%T が %s を満たすか = %v（期待は %v）", c.typ, c.unit.Name(), got, c.want)
		}
	}
	// 出されなかった op のメソッドは、型にない。規則に当たった op のメソッドは、ある
	typ := reflect.TypeOf(lib.BooksTable{})
	for _, name := range []string{"FindByTitle", "FindByPages", "CountBooks", "LentAt", "Recent", "AddBookAt", "AddAndCount", "AddById", "ClearBooks", "TakeBook", "Restock"} {
		if _, has := typ.MethodByName(name); has {
			t.Errorf("BooksTable が、規則に当たらない op のメソッド %s を持つ", name)
		}
	}
	for _, name := range []string{"IsEmpty", "Get", "FindBook", "AddBook"} {
		if _, has := typ.MethodByName(name); !has {
			t.Errorf("BooksTable が、メソッド %s を持たない", name)
		}
	}
}

// TestHumanCompletes は、人が書き足した型を基底の New に渡せて、op が、人のメソッドと go-sqlite のメソッドの両方に届くこと。
func TestHumanCompletes(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	ctx := context.Background()
	if err := lib.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	ops := lib.New(books{lib.NewBooksTable(db), db}, notes{lib.NewNotesTable(db)}, marks{lib.NewMarksTable(db)}, lib.NewArchiveTable(db))
	lent := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := ops.AddBook(ctx, lib.Book{Id: "b1", Title: "Go", Author: "gopher", Pages: 300, LentAt: &lent}); err != nil {
		t.Fatal(err)
	}
	if err := ops.AddBook(ctx, lib.Book{Id: "b2", Title: "SQL", Author: "dba", Pages: 200}); err != nil {
		t.Fatal(err)
	}
	// go-sqlite のメソッド
	if got, err := ops.FindBook(ctx, "b1"); err != nil || got == nil || got.Title != "Go" || got.LentAt == nil || !got.LentAt.Equal(lent) {
		t.Errorf("FindBook(b1) = %+v, %v", got, err)
	}
	if got, err := ops.FindBook(ctx, "b2"); err != nil || got == nil || got.LentAt != nil {
		t.Errorf("FindBook(b2) = %+v, %v（lentAt は nil）", got, err)
	}
	// 人のメソッド
	if got, err := ops.FindByTitle(ctx, "SQL"); err != nil || got == nil || got.Id != "b2" {
		t.Errorf("FindByTitle(SQL) = %+v, %v", got, err)
	}
	if n, err := ops.CountBooks(ctx); err != nil || n != 2 {
		t.Errorf("CountBooks = %d, %v（期待は 2）", n, err)
	}
	if err := ops.ClearBooks(ctx); err != nil {
		t.Fatal(err)
	}
	if empty, err := lib.NewBooksTable(db).IsEmpty(ctx); err != nil || !empty {
		t.Errorf("ClearBooks の後の IsEmpty = %v, %v", empty, err)
	}
	// 別の表（archive）は、同じ model を要素に持つ
	if err := ops.ArchiveOne(ctx, lib.Book{Id: "b1", Title: "Go"}); err != nil {
		t.Fatal(err)
	}
	if got, err := lib.NewArchiveTable(db).Get(ctx); err != nil || len(got) != 1 || got[0].Id != "b1" {
		t.Errorf("archive の Get = %+v, %v", got, err)
	}
}
