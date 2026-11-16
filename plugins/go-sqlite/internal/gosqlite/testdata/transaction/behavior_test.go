// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 「トランザクション」（docs/plugin-go-sqlite.md）を、
// 本物の SQLite の上で確かめる: 確定と取り消し、戻り値のある op、葉の op、入れ子、同じ db の上の二つの名前空間、別の db、panic、
// 始められないとき、別の ctx、継ぎ目を宣言したトランスパイラが二つの単位。plugins/go-sqlite の TestBehavior が、基底の出力と
// go-sqlite の出力を置いたモジュールのルートに置いて走らせる。
//
// データベースの接続は一つだけにする（SetMaxOpenConns(1)）: トランザクションの外で実行した文は、トランザクションが終わるまで待つ
// ので、トランザクションに入っていない文があれば、ctx の期限で失敗する（待ち続けない）。
package behavior

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/app/gen/audit"
	"example.com/app/gen/bank"
	"example.com/app/gen/kinds"
	"example.com/app/gen/mixed"

	_ "modernc.org/sqlite"
)

// openDB は、新しいメモリの上の SQLite を開く。接続は一つだけ。
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// testContext は、期限のある ctx: トランザクションに入っていない文が、接続を待ち続けないようにする。
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// leaves は、bank の葉の op（deposit、closeAll）の、人が書いた実装。文は、受け取った ctx で、go-sqlite の型と bank.Conn を通して実行する。
type leaves struct {
	db       *sql.DB
	accounts bank.AccountsTable
}

// Deposit は、口座の残高を増やして（減らして）、新しい残高を返す。残高が負になれば、書き換えた後で、失敗する。
func (l leaves) Deposit(ctx context.Context, id kinds.AccountId, amount kinds.Amount) (kinds.Amount, error) {
	account, err := l.accounts.AccountById(ctx, id)
	if err != nil {
		return 0, err
	}
	if account == nil {
		return 0, fmt.Errorf("no account %s", id)
	}
	account.Balance += amount
	if err := l.accounts.UpdateAccount(ctx, *account); err != nil {
		return 0, err
	}
	if account.Balance < 0 {
		return 0, fmt.Errorf("account %s would go below zero", id)
	}
	return account.Balance, nil
}

// CloseAll は、口座を、並びの順に消す。なかった口座で、失敗する（それまでに消したものは、消した後）。
func (l leaves) CloseAll(ctx context.Context, ids []kinds.AccountId, note *string) error {
	for _, id := range ids {
		res, err := bank.Conn(ctx, l.db).ExecContext(ctx, `DELETE FROM "accounts" WHERE "id" = ?1`, string(id))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("no account %s", id)
		}
	}
	return nil
}

// world は、一つの db の上の、bank と audit。
type world struct {
	db       *sql.DB
	accounts bank.AccountsTable
	entries  audit.EntriesTable
	bank     bank.Ops
	audit    audit.Ops
}

// newWorld は、bank と audit を、同じ db の上に組み立てる。auditDB が nil でなければ、audit だけを、その別の db の上に。
// 継ぎ目は、go-sqlite の DbSeams（bank.Seams と audit.Seams を、そのまま満たす。満たさなければ、ここでコンパイルできない）。
func newWorld(t *testing.T, ctx context.Context, auditDB *sql.DB) world {
	t.Helper()
	db := openDB(t)
	if auditDB == nil {
		auditDB = db
	}
	if err := bank.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := audit.CreateTables(ctx, auditDB); err != nil {
		t.Fatal(err)
	}
	w := world{db: db, accounts: bank.NewAccountsTable(db), entries: audit.NewEntriesTable(auditDB)}
	w.audit = audit.New(w.entries, audit.NewDbSeams(auditDB))
	w.bank = bank.New(leaves{db: db, accounts: w.accounts}, w.accounts, w.audit, bank.NewDbSeams(db))
	return w
}

// ids は、口座の表の行の識別子（足した順）。
func (w world) ids(t *testing.T, ctx context.Context) []kinds.AccountId {
	t.Helper()
	got, err := w.accounts.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := []kinds.AccountId{}
	for _, a := range got {
		out = append(out, a.Id)
	}
	return out
}

// logged は、記録の表の行の what（足した順）。
func (w world) logged(t *testing.T, ctx context.Context) []string {
	t.Helper()
	got, err := w.entries.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, e := range got {
		out = append(out, e.What)
	}
	return out
}

func wantIDs(t *testing.T, what string, got []kinds.AccountId, want ...kinds.AccountId) {
	t.Helper()
	if want == nil {
		want = []kinds.AccountId{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: 口座が %v（期待は %v）", what, got, want)
	}
}

func account(id kinds.AccountId, balance kinds.Amount) bank.Account {
	return bank.Account{Id: id, Balance: balance}
}

// passSeams は、継ぎ目を素通しにする、人が書いた bank.Seams（中身を、そのまま呼ぶ。トランザクションは始めない）。
// 埋め込んだ DbSeams のメソッドのうち、OpenDbTransaction だけを差し替える。
type passSeams struct{ bank.DbSeams }

func (passSeams) OpenDbTransaction(ctx context.Context, first bank.Account, second bank.Account, body func(ctx context.Context, first bank.Account, second bank.Account) error) error {
	return body(ctx, first, second)
}

// TestCommitAndRollback は、@db.transaction の付いた op: 成功すれば、どれも残る。失敗すれば、どれも残らない。失敗が残らないのが
// 継ぎ目の実装（DbSeams）の働きであることを、継ぎ目を素通しにした組み立てと比べて確かめる。
func TestCommitAndRollback(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, nil)
	if err := w.bank.Open(ctx, account("a", 1), account("b", 2)); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, "成功した open の後", w.ids(t, ctx), "a", "b")
	// 二つ目の効果が失敗する（同じキー）: 一つ目の効果が足した c も、残らない。エラーは、そのまま返る
	err := w.bank.Open(ctx, account("c", 3), account("a", 4))
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("二つ目の効果が失敗する open のエラーが %v", err)
	}
	wantIDs(t, "失敗した open の後", w.ids(t, ctx), "a", "b")
	// 取り消した後も、db は使える（接続を返している）
	if err := w.bank.Open(ctx, account("c", 3), account("d", 4)); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, "取り消した後の open の後", w.ids(t, ctx), "a", "b", "c", "d")

	// 継ぎ目を素通しにすると、一つ目の効果が足した行が残る
	pass := bank.New(leaves{db: w.db, accounts: w.accounts}, w.accounts, w.audit, passSeams{bank.NewDbSeams(w.db)})
	if err := pass.Open(ctx, account("e", 5), account("a", 6)); err == nil {
		t.Fatal("素通しの継ぎ目で、二つ目の効果が失敗しない")
	}
	wantIDs(t, "素通しの継ぎ目で失敗した open の後", w.ids(t, ctx), "a", "b", "c", "d", "e")
}

// TestReturns は、戻り値のある op の継ぎ目: 成功すれば、中身の戻り値を、そのまま返す。失敗すれば、零値とエラー。
// op の中の read は、同じ op が先に書いたものを見る（読む方も、トランザクションに入る）。
func TestReturns(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, nil)
	got, err := w.bank.OpenAndFind(ctx, account("a", 7))
	if err != nil || got == nil || got.Id != "a" || got.Balance != 7 {
		t.Errorf("OpenAndFind(a) = %+v, %v（期待は、足した行）", got, err)
	}
	got, err = w.bank.OpenAndFind(ctx, account("a", 8))
	if err == nil || got != nil {
		t.Errorf("同じキーの OpenAndFind = %+v, %v（期待は nil と、エラー）", got, err)
	}
	// 配列を返す op。引数がない
	all, err := w.bank.Snapshot(ctx)
	if err != nil || len(all) != 1 || all[0].Balance != 7 {
		t.Errorf("Snapshot = %+v, %v", all, err)
	}
	// 継ぎ目を持たない op は、そのまま
	if got, err := w.bank.Find(ctx, "a"); err != nil || got == nil || got.Balance != 7 {
		t.Errorf("Find(a) = %+v, %v", got, err)
	}
}

// TestLeaf は、葉の op の継ぎ目: 人が書いた中身（Leaves のメソッド）が、受け取った ctx で、go-sqlite の型と Conn を通してしたことも、
// 一つのトランザクションに入る。
func TestLeaf(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, nil)
	if err := w.bank.Open(ctx, account("a", 10), account("b", 20)); err != nil {
		t.Fatal(err)
	}
	// scalar を返す葉
	if got, err := w.bank.Deposit(ctx, "a", 5); err != nil || got != 15 {
		t.Errorf("Deposit(a, 5) = %v, %v（期待は 15）", got, err)
	}
	// 書き換えた後で失敗する: 書き換えは残らず、戻り値は零値
	got, err := w.bank.Deposit(ctx, "a", -100)
	if err == nil || got != 0 {
		t.Errorf("Deposit(a, -100) = %v, %v（期待は 0 と、エラー）", got, err)
	}
	if a, err := w.bank.Find(ctx, "a"); err != nil || a == nil || a.Balance != 15 {
		t.Errorf("失敗した Deposit の後の a = %+v, %v（期待は、残高 15 のまま）", a, err)
	}
	// 人が Conn を通して実行した文: 一つ目を消した後で、二つ目で失敗する。消したものは、残る
	if err := w.bank.CloseAll(ctx, []kinds.AccountId{"a", "nobody"}, nil); err == nil {
		t.Fatal("ない口座のある CloseAll が、失敗しない")
	}
	wantIDs(t, "失敗した CloseAll の後", w.ids(t, ctx), "a", "b")
	if err := w.bank.CloseAll(ctx, []kinds.AccountId{"b", "a"}, nil); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, "成功した CloseAll の後", w.ids(t, ctx))
}

// TestNested は、入れ子（継ぎ目を持つ op が、継ぎ目を持つ op を呼ぶ）: 内側は、新しいトランザクションを始めずに、外側のものに加わる。
// 確定するのも、取り消すのも、いちばん外側。
func TestNested(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, nil)
	// どちらも成功する（内側が新しく始めていれば、接続が一つなので、ここで待ち続ける）
	if err := w.bank.OpenThree(ctx, account("a", 1), account("b", 2), account("c", 3)); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, "成功した openThree の後", w.ids(t, ctx), "a", "b", "c")
	// 内側（open）が失敗する: 外側が先に足した d も、残らない
	if err := w.bank.OpenThree(ctx, account("d", 4), account("e", 5), account("a", 6)); err == nil {
		t.Fatal("内側が失敗する openThree が、失敗しない")
	}
	wantIDs(t, "内側が失敗した openThree の後", w.ids(t, ctx), "a", "b", "c")
	// 内側（open）が成功した後に、外側が失敗する: 内側が足した d と e も、書き換えた b も、残らない（内側は、確定していない）
	err := w.bank.OpenThenReplace(ctx, account("d", 4), account("e", 5), account("b", 99), account("a", 6))
	if err == nil {
		t.Fatal("外側が失敗する openThenReplace が、失敗しない")
	}
	wantIDs(t, "外側が失敗した openThenReplace の後", w.ids(t, ctx), "a", "b", "c")
	if b, err := w.bank.Find(ctx, "b"); err != nil || b == nil || b.Balance != 2 {
		t.Errorf("外側が失敗した openThenReplace の後の b = %+v, %v（期待は、残高 2 のまま）", b, err)
	}
	// すべて成功すれば、内側のしたことも、外側と一緒に確定される
	if err := w.bank.OpenThenReplace(ctx, account("d", 4), account("e", 5), account("b", 99), account("f", 6)); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, "成功した openThenReplace の後", w.ids(t, ctx), "a", "b", "c", "d", "e", "f")
	if b, err := w.bank.Find(ctx, "b"); err != nil || b == nil || b.Balance != 99 {
		t.Errorf("成功した openThenReplace の後の b = %+v, %v（期待は、残高 99）", b, err)
	}
}

// TestAcrossNamespaces は、同じ db の上の二つの名前空間: bank の @db.transaction の op が呼ぶ audit の op のすることも、同じ
// トランザクションに入る（鍵 runtime.DBTxKey は、出力のパッケージをまたいで同じ）。入っていなければ、audit の文は、接続を待ち続ける。
func TestAcrossNamespaces(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, nil)
	entry := func(what string) audit.Entry { return audit.Entry{Account: "a", What: what} }
	// audit の、継ぎ目を持たない op
	if err := w.bank.OpenLogged(ctx, account("a", 1), entry("opened")); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, "成功した openLogged の後", w.ids(t, ctx), "a")
	if got := w.logged(t, ctx); !reflect.DeepEqual(got, []string{"opened"}) {
		t.Errorf("成功した openLogged の後の記録が %v", got)
	}
	// 後の効果（bank）が失敗する: 先に足した記録（audit）も、残らない
	if err := w.bank.OpenLogged(ctx, account("a", 2), entry("again")); err == nil {
		t.Fatal("同じキーの openLogged が、失敗しない")
	}
	if got := w.logged(t, ctx); !reflect.DeepEqual(got, []string{"opened"}) {
		t.Errorf("失敗した openLogged の後の記録が %v（期待は、opened だけ）", got)
	}
	// audit の、継ぎ目を持つ op（入れ子が、名前空間をまたぐ）: audit の継ぎ目は、新しく始めずに、bank のトランザクションに加わる
	if err := w.bank.OpenLoggedTwice(ctx, account("a", 3), entry("x"), entry("y")); err == nil {
		t.Fatal("同じキーの openLoggedTwice が、失敗しない")
	}
	if got := w.logged(t, ctx); !reflect.DeepEqual(got, []string{"opened"}) {
		t.Errorf("失敗した openLoggedTwice の後の記録が %v（期待は、opened だけ）", got)
	}
	if err := w.bank.OpenLoggedTwice(ctx, account("b", 3), entry("x"), entry("y")); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, "成功した openLoggedTwice の後", w.ids(t, ctx), "a", "b")
	if got := w.logged(t, ctx); !reflect.DeepEqual(got, []string{"opened", "x", "y"}) {
		t.Errorf("成功した openLoggedTwice の後の記録が %v", got)
	}
	// audit の op を、トランザクションの外から呼べば、自分の継ぎ目が始める
	if err := w.audit.RecordTwice(ctx, entry("p"), entry("q")); err != nil {
		t.Fatal(err)
	}
	if got, err := w.audit.History(ctx, "a"); err != nil || len(got) != 5 {
		t.Errorf("History(a) = %+v, %v（期待は五行）", got, err)
	}
}

// TestSeparateDB は、別の db の上の型: 鍵が db ごとなので、bank のトランザクションには入らない。bank が取り消しても、別の db の
// audit が足した記録は、残る（二つを一緒に確定する仕組みは、ない）。
func TestSeparateDB(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, openDB(t))
	if err := w.bank.Open(ctx, account("a", 1), account("b", 2)); err != nil {
		t.Fatal(err)
	}
	if err := w.bank.OpenLogged(ctx, account("a", 3), audit.Entry{Account: "a", What: "kept"}); err == nil {
		t.Fatal("同じキーの openLogged が、失敗しない")
	}
	if got := w.logged(t, ctx); !reflect.DeepEqual(got, []string{"kept"}) {
		t.Errorf("bank が取り消した後の、別の db の記録が %v（期待は、kept が残る）", got)
	}
	// 別の db の DbSeams を入れ子にすれば、db ごとに別のトランザクション: audit のものは、audit が確定する
	if err := w.bank.OpenLoggedTwice(ctx, account("a", 3), audit.Entry{Account: "a", What: "x"}, audit.Entry{Account: "a", What: "y"}); err == nil {
		t.Fatal("同じキーの openLoggedTwice が、失敗しない")
	}
	if got := w.logged(t, ctx); !reflect.DeepEqual(got, []string{"kept", "x", "y"}) {
		t.Errorf("bank が取り消した後の、別の db の記録が %v（期待は、kept、x、y が残る）", got)
	}
	wantIDs(t, "bank が取り消した後", w.ids(t, ctx), "a", "b")
}

// TestPanic は、中身が panic したとき: 取り消してから、panic が続く。接続は返っている。
func TestPanic(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, nil)
	seams := bank.NewDbSeams(w.db)
	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Errorf("recover() = %v（期待は、中身の panic の値）", r)
			}
		}()
		_ = seams.OpenDbTransaction(ctx, account("a", 1), account("b", 2), func(ctx context.Context, first bank.Account, second bank.Account) error {
			if err := w.accounts.CreateAccount(ctx, first); err != nil {
				t.Fatal(err)
			}
			panic("boom")
		})
	}()
	wantIDs(t, "panic した後", w.ids(t, ctx))
}

// TestBeginFails は、始められないとき（中断された ctx）: 中身を呼ばずに、そのエラーを返す。
func TestBeginFails(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, nil)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	called := false
	got, err := bank.NewDbSeams(w.db).OpenAndFindDbTransaction(cancelled, account("a", 1), func(ctx context.Context, account bank.Account) (*bank.Account, error) {
		called = true
		return &account, nil
	})
	if !errors.Is(err, context.Canceled) || got != nil || called {
		t.Errorf("中断された ctx の継ぎ目 = %+v, %v、中身を呼んだか = %v（期待は nil、context.Canceled、false）", got, err, called)
	}
}

// TestBodyValues は、継ぎ目が、op の入力と出力の値を変えないこと: 受け取った引数を、そのまま中身に渡し、中身の戻り値を、そのまま返す。
// 中身のエラーも、そのまま（包まない）。中身に渡す ctx は、トランザクションを運ぶ。
func TestBodyValues(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, nil)
	seams := bank.NewDbSeams(w.db)
	want := &bank.Account{Id: "z", Balance: 9}
	got, err := seams.OpenAndFindDbTransaction(ctx, account("a", 1), func(ctx context.Context, account bank.Account) (*bank.Account, error) {
		if account.Id != "a" || account.Balance != 1 {
			t.Errorf("中身が受け取った引数が %+v", account)
		}
		if _, isTx := bank.Conn(ctx, w.db).(*sql.Tx); !isTx {
			t.Error("中身に渡した ctx が、トランザクションを運んでいない")
		}
		return want, nil
	})
	if err != nil || got != want {
		t.Errorf("継ぎ目の戻り値が %p, %v（期待は、中身が返したポインタ %p）", got, err, want)
	}
	failure := errors.New("spec exception")
	if err := seams.OpenDbTransaction(ctx, account("a", 1), account("b", 2), func(context.Context, bank.Account, bank.Account) error { return failure }); err != failure {
		t.Errorf("継ぎ目のエラーが %v（期待は、中身のエラーそのもの）", err)
	}
	// トランザクションの外の ctx では、Conn は db
	if _, isDB := bank.Conn(ctx, w.db).(*sql.DB); !isDB {
		t.Error("トランザクションを運ばない ctx の Conn が、db でない")
	}
}

// TestOtherContext は、別の ctx（トランザクションを運んでいない）で呼んだメソッド: トランザクションに入らない。接続が一つなので、
// トランザクションが終わるまで待ち、ここでは、期限で失敗する。
func TestOtherContext(t *testing.T) {
	ctx := testContext(t)
	w := newWorld(t, ctx, nil)
	err := bank.NewDbSeams(w.db).OpenDbTransaction(ctx, account("a", 1), account("b", 2), func(ctx context.Context, first bank.Account, second bank.Account) error {
		other, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if err := w.accounts.CreateAccount(other, first); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("別の ctx で呼んだ効果のエラーが %v（期待は、接続を待って、期限で失敗）", err)
		}
		return w.accounts.CreateAccount(ctx, second)
	})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, "別の ctx の効果が入らなかった後", w.ids(t, ctx), "b")
}

// traceSeams は、もう一つの相乗り（go-trace）の継ぎ目の、人が書いた代わりの実装: 呼ばれた順を覚えて、中身を呼ぶ。
type traceSeams struct{ log *[]string }

func (s traceSeams) TouchTTraced(ctx context.Context, row mixed.Row, body func(ctx context.Context, row mixed.Row) error) error {
	*s.log = append(*s.log, "touch")
	return body(ctx, row)
}

func (s traceSeams) AddTwoTTraced(ctx context.Context, first mixed.Row, second mixed.Row, body func(ctx context.Context, first mixed.Row, second mixed.Row) error) error {
	*s.log = append(*s.log, "addTwo")
	err := body(ctx, first, second)
	*s.log = append(*s.log, fmt.Sprintf("addTwo done: %v", err != nil))
	return err
}

// TestMixed は、継ぎ目を宣言したトランスパイラが二つの単位: DbSeams は、自分のデコレータ（@db.transaction）の分のメソッドだけを持ち、
// それだけでは mixed.Seams を満たさない（var _ も出していない）。人が、二つの型を一つの構造体に埋め込んで、New に渡す。
func TestMixed(t *testing.T) {
	ctx := testContext(t)
	db := openDB(t)
	if err := mixed.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if reflect.TypeOf(mixed.DbSeams{}).Implements(reflect.TypeFor[mixed.Seams]()) {
		t.Error("DbSeams だけで、mixed.Seams を満たす（go-trace の継ぎ目のメソッドを持っている）")
	}
	var log []string
	rows := mixed.NewRowsTable(db)
	ops := mixed.New(rows, struct {
		mixed.DbSeams
		traceSeams
	}{mixed.NewDbSeams(db), traceSeams{&log}})
	if err := ops.Add(ctx, mixed.Row{Id: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := ops.Touch(ctx, mixed.Row{Id: "b"}); err != nil {
		t.Fatal(err)
	}
	// 外側が @t.traced、内側が @db.transaction: 二つ目の効果が失敗すれば、一つ目も残らない
	if err := ops.AddTwo(ctx, mixed.Row{Id: "c"}, mixed.Row{Id: "a"}); err == nil {
		t.Fatal("同じキーの addTwo が、失敗しない")
	}
	got, err := rows.Get(ctx)
	if err != nil || len(got) != 2 || got[0].Id != "a" || got[1].Id != "b" {
		t.Errorf("Get = %+v, %v（期待は a、b）", got, err)
	}
	if want := []string{"touch", "addTwo", "addTwo done: true"}; !reflect.DeepEqual(log, want) {
		t.Errorf("go-trace の継ぎ目が呼ばれた順が %v（期待は %v）", log, want)
	}
}
