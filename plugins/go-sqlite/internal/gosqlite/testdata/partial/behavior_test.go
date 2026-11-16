// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 「状態ごとの型」（docs/plugin-go-sqlite.md）の、
// db の語彙のデコレータを付けていない read と効果があるとき: その型は、単位を満たさず、人が、型を埋め込んで、足りないメソッドを
// 書き足す。人のメソッドは、文を Conn を通して実行すれば、トランザクションに入る。plugins/go-sqlite の TestBehavior が、基底の出力と
// go-sqlite の出力を置いたモジュールのルートに置いて走らせる。足りないままの型がコンパイルできないことは、TestIncomplete が確かめる。
package behavior

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"example.com/app/gen/lib"

	_ "modernc.org/sqlite"
)

// books は、lib.BooksTable を埋め込んで、足りないメソッド（read newestBooks、countBooks と、効果 retitle、clearBooks）を書き足した型。
// 文は、db を直に使わずに、lib.Conn を通して実行する。
type books struct {
	lib.BooksTable
	db *sql.DB
}

func (b books) NewestBooks(ctx context.Context, limit int64) ([]lib.Book, error) {
	rows, err := lib.Conn(ctx, b.db).QueryContext(ctx, `SELECT "id", "title", "pages" FROM "books" ORDER BY rowid DESC LIMIT ?1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []lib.Book
	for rows.Next() {
		var book lib.Book
		if err := rows.Scan(&book.Id, &book.Title, &book.Pages); err != nil {
			return nil, err
		}
		out = append(out, book)
	}
	return out, rows.Err()
}

func (b books) CountBooks(ctx context.Context) (int64, error) {
	var n int64
	err := lib.Conn(ctx, b.db).QueryRowContext(ctx, `SELECT COUNT(*) FROM "books"`).Scan(&n)
	return n, err
}

// Retitle は、本の題を書き換える。空の題は、書き換える前に断る。
func (b books) Retitle(ctx context.Context, id lib.BookId, title string) error {
	if title == "" {
		return errors.New("empty title")
	}
	_, err := lib.Conn(ctx, b.db).ExecContext(ctx, `UPDATE "books" SET "title" = ?2 WHERE "id" = ?1`, string(id), title)
	return err
}

func (b books) ClearBooks(ctx context.Context) error {
	_, err := lib.Conn(ctx, b.db).ExecContext(ctx, `DELETE FROM "books"`)
	return err
}

// direct は、books の Retitle を、Conn を通さずに、db を直に使うものに差し替えた型（してはいけない形）。
type direct struct{ books }

func (d direct) Retitle(ctx context.Context, id lib.BookId, title string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE "books" SET "title" = ?2 WHERE "id" = ?1`, string(id), title)
	return err
}

// notes は、lib.NotesTable を埋め込んで、足りない効果 firstNote を書き足した型。
type notes struct {
	lib.NotesTable
	db *sql.DB
}

func (n notes) FirstNote(ctx context.Context, note lib.Note) error {
	_, err := lib.Conn(ctx, n.db).ExecContext(ctx, `INSERT INTO "notes" ("text", "stars") VALUES (?1, ?2)`, note.Text, note.Stars)
	return err
}

// drafts は、表でない状態 lib.drafts の単位（go-sqlite は型を出さない）の、人がメモリの上に書いた実装。
type drafts struct{ list *[]lib.Note }

func (d drafts) IsEmpty(context.Context) (bool, error)    { return len(*d.list) == 0, nil }
func (d drafts) Get(context.Context) ([]lib.Note, error) { return *d.list, nil }
func (d drafts) DraftsWith(_ context.Context, stars int64) ([]lib.Note, error) {
	var out []lib.Note
	for _, n := range *d.list {
		if n.Stars == stars {
			out = append(out, n)
		}
	}
	return out, nil
}
func (d drafts) CreateDraft(_ context.Context, note lib.Note) error {
	*d.list = append(*d.list, note)
	return nil
}

// lending は、述語で決まる状態の分類 lib.Lending の単位（go-sqlite は型を出さない）の、人が書いた実装。
type lending struct{ lent *bool }

func (l lending) Free(context.Context) (bool, error) { return !*l.lent, nil }
func (l lending) Lent(context.Context) (bool, error) { return *l.lent, nil }
func (l lending) Lend(context.Context, lib.Book) error {
	*l.lent = true
	return nil
}

// world は、一つの db の上の lib。
type world struct {
	db     *sql.DB
	books  books
	notes  notes
	stamps lib.StampsTable
	drafts drafts
	ops    lib.Ops
}

// newWorld は、新しいメモリの上の SQLite（接続は一つだけ）に表を作り、lib を組み立てる。足りないメソッドのある二つの型は、人が
// 書き足した型（books、notes）を渡す。すべてのメソッドを持つ StampsTable は、そのまま渡す。
func newWorld(t *testing.T) (world, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	if err := lib.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	w := world{
		db:     db,
		books:  books{lib.NewBooksTable(db), db},
		notes:  notes{lib.NewNotesTable(db), db},
		stamps: lib.NewStampsTable(db),
		drafts: drafts{new([]lib.Note)},
	}
	w.ops = lib.New(w.books, w.notes, w.stamps, w.drafts, lending{new(bool)}, lib.NewDbSeams(db))
	return w, ctx
}

// titles は、本の表の行の題（足した順）。
func (w world) titles(t *testing.T, ctx context.Context) []string {
	t.Helper()
	got, err := w.books.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, b := range got {
		out = append(out, b.Title)
	}
	return out
}

// TestUnits は、どの型が、基底の単位を満たすか: デコレータを付けていない read か効果のある型（BooksTable、NotesTable）は満たさず、
// 人が書き足した型は満たす。すべてに付けた型（StampsTable）は、そのまま満たす。
func TestUnits(t *testing.T) {
	for _, c := range []struct {
		impl any
		unit reflect.Type
		want bool
	}{
		{lib.BooksTable{}, reflect.TypeFor[lib.BooksState](), false},
		{books{}, reflect.TypeFor[lib.BooksState](), true},
		{lib.NotesTable{}, reflect.TypeFor[lib.NotesState](), false},
		{notes{}, reflect.TypeFor[lib.NotesState](), true},
		{lib.StampsTable{}, reflect.TypeFor[lib.StampsState](), true},
	} {
		if got := reflect.TypeOf(c.impl).Implements(c.unit); got != c.want {
			t.Errorf("%T が %s を満たすか = %v（期待は %v）", c.impl, c.unit, got, c.want)
		}
	}
}

// TestCompleted は、人が書き足した型を渡した組み立て: 生成したメソッド（bookById、createBook）と、人のメソッド（newestBooks、
// retitle）が、同じ表を読み書きする。
func TestCompleted(t *testing.T) {
	w, ctx := newWorld(t)
	for _, b := range []lib.Book{{Id: "b1", Title: "one", Pages: 10}, {Id: "b2", Title: "two", Pages: 20}, {Id: "b3", Title: "three", Pages: 30}} {
		if err := w.ops.AddBook(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := w.ops.Newest(ctx, 2); err != nil || len(got) != 2 || got[0].Id != "b3" || got[1].Id != "b2" {
		t.Errorf("Newest(2) = %+v, %v（期待は b3、b2）", got, err)
	}
	if err := w.ops.Rename(ctx, "b2", "second"); err != nil {
		t.Fatal(err)
	}
	if got, err := w.ops.FindBook(ctx, "b2"); err != nil || got == nil || got.Title != "second" || got.Pages != 20 {
		t.Errorf("Rename の後の FindBook(b2) = %+v, %v", got, err)
	}
	if n, err := w.books.CountBooks(ctx); err != nil || n != 3 {
		t.Errorf("CountBooks = %d, %v（期待は 3）", n, err)
	}
	// 表でない状態と、状態の分類の単位は、人の実装
	if err := w.ops.Draft(ctx, lib.Note{Text: "idea", Stars: 3}); err != nil {
		t.Fatal(err)
	}
	if got, err := w.drafts.DraftsWith(ctx, 3); err != nil || len(got) != 1 {
		t.Errorf("DraftsWith(3) = %+v, %v", got, err)
	}
	if err := w.ops.LendBook(ctx, lib.Book{Id: "b1"}); err != nil {
		t.Fatal(err)
	}
	// notes: 生成した read と、人の効果
	if err := w.notes.FirstNote(ctx, lib.Note{Text: "good", Stars: 5}); err != nil {
		t.Fatal(err)
	}
	if got, err := w.notes.NotesWith(ctx, 5); err != nil || len(got) != 1 || got[0].Text != "good" {
		t.Errorf("NotesWith(5) = %+v, %v", got, err)
	}
}

// TestHumanJoinsTransaction は、人が書き足したメソッドが、Conn を通して、@db.transaction の継ぎ目のトランザクションに入ること:
// 人の効果が失敗すれば、先に生成した効果が足した行は残らず、後の生成した効果が失敗すれば、先に人の効果が書き換えたものも残らない。
func TestHumanJoinsTransaction(t *testing.T) {
	w, ctx := newWorld(t)
	if err := w.ops.AddBook(ctx, lib.Book{Id: "b1", Title: "one", Pages: 10}); err != nil {
		t.Fatal(err)
	}
	// 生成した効果（createBook）の後に、人の効果（retitle）が成功する: 接続は一つなので、人の文がトランザクションに入っていなければ、
	// ここで待ち続ける
	if err := w.ops.AddRenamed(ctx, lib.Book{Id: "b2", Title: "two", Pages: 20}, "second"); err != nil {
		t.Fatal(err)
	}
	if got := w.titles(t, ctx); !reflect.DeepEqual(got, []string{"one", "second"}) {
		t.Errorf("成功した addRenamed の後の題が %v", got)
	}
	// 人の効果が失敗する: 生成した効果が足した b3 は、残らない
	if err := w.ops.AddRenamed(ctx, lib.Book{Id: "b3", Title: "three", Pages: 30}, ""); err == nil {
		t.Fatal("空の題の addRenamed が、失敗しない")
	}
	if got := w.titles(t, ctx); !reflect.DeepEqual(got, []string{"one", "second"}) {
		t.Errorf("失敗した addRenamed の後の題が %v（期待は、変わらない）", got)
	}
	// 人の効果が書き換えた後に、生成した効果が失敗する（同じキー）: 人の効果の書き換えも、残らない
	if err := w.ops.RenameThenAdd(ctx, "b1", "first", lib.Book{Id: "b2", Title: "dup"}); err == nil {
		t.Fatal("同じキーの renameThenAdd が、失敗しない")
	}
	if got := w.titles(t, ctx); !reflect.DeepEqual(got, []string{"one", "second"}) {
		t.Errorf("失敗した renameThenAdd の後の題が %v（期待は、変わらない）", got)
	}
	// 人のメソッドを、継ぎ目のメソッドの中で直に呼んでも、同じ
	err := lib.NewDbSeams(w.db).AddRenamedDbTransaction(ctx, lib.Book{}, "", func(ctx context.Context, _ lib.Book, _ string) error {
		if err := w.books.ClearBooks(ctx); err != nil {
			return err
		}
		if n, err := w.books.CountBooks(ctx); err != nil || n != 0 {
			t.Errorf("トランザクションの中の CountBooks = %d, %v（期待は 0: 同じトランザクションが消したものは、見えない）", n, err)
		}
		return errors.New("undo")
	})
	if err == nil || err.Error() != "undo" {
		t.Errorf("継ぎ目のエラーが %v（期待は、中身のエラー）", err)
	}
	if n, err := w.books.CountBooks(ctx); err != nil || n != 2 {
		t.Errorf("取り消した後の CountBooks = %d, %v（期待は 2）", n, err)
	}
}

// TestHumanBypassesConn は、Conn を通さない人のメソッド（db を直に使う）: トランザクションに入らない。接続が一つなので、
// トランザクションが終わるまで待ち、ここでは、期限で失敗する。
func TestHumanBypassesConn(t *testing.T) {
	w, ctx := newWorld(t)
	bypass := lib.New(direct{w.books}, w.notes, w.stamps, w.drafts, lending{new(bool)}, lib.NewDbSeams(w.db))
	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err := bypass.AddRenamed(short, lib.Book{Id: "b1", Title: "one", Pages: 10}, "first")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Conn を通さない人の効果のある addRenamed のエラーが %v（期待は、接続を待って、期限で失敗）", err)
	}
	if got := w.titles(t, ctx); len(got) != 0 {
		t.Errorf("失敗した addRenamed の後の題が %v（期待は、何も残らない）", got)
	}
}

// TestStamps は、行を受け取るメソッドを持たない表（dbArgs<Model> を出さない）: @db.delete の効果と、形で決まる問い合わせ。
func TestStamps(t *testing.T) {
	w, ctx := newWorld(t)
	at := time.Date(2026, 2, 3, 4, 5, 6, 7, time.UTC)
	for _, v := range []time.Time{at, at.Add(time.Hour)} {
		if _, err := w.db.ExecContext(ctx, `INSERT INTO "stamps" ("at") VALUES (?1)`, v.UTC().Format("2006-01-02T15:04:05.000000000Z07:00")); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := w.stamps.Get(ctx); err != nil || len(got) != 2 || !got[0].At.Equal(at) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if err := w.stamps.DropStamp(ctx, at); err != nil {
		t.Fatal(err)
	}
	if got, err := w.stamps.Get(ctx); err != nil || len(got) != 1 || !got[0].At.Equal(at.Add(time.Hour)) {
		t.Errorf("DropStamp の後の Get = %+v, %v", got, err)
	}
}
