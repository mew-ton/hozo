// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 名前を付け替えた引数（docs/plugin-go-sqlite.md
// 「補助」の末尾）が、付け替える前と同じ値を運ぶこと、内側のフォルダを持つ出力で、db.go の型が、入口の New にそのまま渡せること。
// plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"example.com/app/gen/r"
	"example.com/app/gen/row"
	"example.com/app/gen/vault"
	"example.com/app/gen/vault/inner"

	_ "modernc.org/sqlite"
)

func newVault(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := vault.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db, ctx
}

func secret(key vault.Key, num vault.Num) vault.Secret {
	at := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	return vault.Secret{Row: key, R: num, Err: 0.5, Data: true, Type: at, String: []vault.Tag{{Name: "x"}},
		Json: map[string]string{"k": "v"}, Any: &vault.Tag{Name: "any"}, Sql: time.Minute}
}

// TestRenamedParams は、名前を付け替えた引数の op: 入口の Ops（export の op）から呼ぶ。
func TestRenamedParams(t *testing.T) {
	db, ctx := newVault(t)
	// db.go の型は、入口のパッケージの型で、入口の New にそのまま渡せる（満たさなければ、ここでコンパイルできない）
	ops := vault.New(vault.NewSecretsTable(db), vault.NewEntriesATable(db), vault.NewEntriesBTable(db), vault.NewSharedTable(db))
	one, two := secret("k1", 1), secret("k2", 2)
	if err := ops.Add(ctx, one); err != nil {
		t.Fatal(err)
	}
	if err := ops.Add(ctx, two); err != nil {
		t.Fatal(err)
	}
	if got, err := ops.Find(ctx, "k2"); err != nil || got == nil || got.R != 2 || got.Any == nil || got.Any.Name != "any" || got.Time != nil {
		t.Errorf("Find(k2) = %+v, %v", got, err)
	}
	if got, err := ops.Open(ctx, "k1"); err != nil || got == nil || got.R != 1 {
		t.Errorf("Open(k1) = %+v, %v", got, err)
	}
	for name, list := range map[string]func() ([]vault.Secret, error){
		"Scan(2)":       func() ([]vault.Secret, error) { return ops.Scan(ctx, 2) },
		"Count(2)":      func() ([]vault.Secret, error) { return ops.Count(ctx, 2) },
		"Near(0.5)":     func() ([]vault.Secret, error) { return ops.Near(ctx, 0.5) },
		"ListAt":        func() ([]vault.Secret, error) { return ops.ListAt(ctx, one.Type) },
		"ListJson":      func() ([]vault.Secret, error) { return ops.ListJson(ctx, map[string]string{"k": "v"}) },
		"ListSql":       func() ([]vault.Secret, error) { return ops.ListSql(ctx, time.Minute) },
		"ListData(yes)": func() ([]vault.Secret, error) { return ops.ListData(ctx, true) },
	} {
		want := 2
		if name == "Scan(2)" || name == "Count(2)" {
			want = 1
		}
		if got, err := list(); err != nil || len(got) != want {
			t.Errorf("%s = %d 行, %v（期待は %d 行）", name, len(got), err, want)
		}
	}
	one.Sql = time.Hour
	if err := ops.Save(ctx, one); err != nil {
		t.Fatal(err)
	}
	if got, err := ops.ListSql(ctx, time.Hour); err != nil || len(got) != 1 || got[0].Row != "k1" {
		t.Errorf("Save の後の ListSql(1h) = %v, %v", got, err)
	}

	// トランザクションの中の名前と当たる引数: 足す（k3）、書き換える（k1）、消す（k2 を row で、k3 を r で）
	one.Sql = 2 * time.Hour
	if err := ops.Mix(ctx, secret("k3", 3), one, "k2", 3); err != nil {
		t.Fatal(err)
	}
	got, err := vault.NewSecretsTable(db).Get(ctx)
	if err != nil || len(got) != 1 || got[0].Row != "k1" || got[0].Sql != 2*time.Hour {
		t.Errorf("Mix の後の secrets = %+v, %v（期待は、書き換えた k1 だけ）", got, err)
	}
	if err := ops.Mix2(ctx, "k1", 9, secret("k4", 4)); err != nil {
		t.Fatal(err)
	}
	if got, err := vault.NewSecretsTable(db).Get(ctx); err != nil || len(got) != 1 || got[0].Row != "k4" {
		t.Errorf("Mix2 の後の secrets = %+v, %v（期待は k4 だけ）", got, err)
	}
	if err := ops.Drop(ctx, "k4"); err != nil {
		t.Fatal(err)
	}
	if err := ops.DropNum(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if empty, err := vault.NewSecretsTable(db).IsEmpty(ctx); err != nil || !empty {
		t.Errorf("Drop の後の IsEmpty = %v, %v", empty, err)
	}
}

// TestSameNameModels は、同じ名前の model（row.Entry と r.Entry）の表: 補助（dbArgsEntry と dbArgsEntry2）を取り違えない。
// パッケージの名前が、補助の中の局所の名前（row、r）と同じでも、コンパイルできて動く。
func TestSameNameModels(t *testing.T) {
	db, ctx := newVault(t)
	ops := vault.New(vault.NewSecretsTable(db), vault.NewEntriesATable(db), vault.NewEntriesBTable(db), vault.NewSharedTable(db))
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := ops.AddA(ctx, row.Entry{Id: "a", At: at}); err != nil {
		t.Fatal(err)
	}
	if err := ops.AddB(ctx, r.Entry{Code: 7, Tags: []string{"t"}}); err != nil {
		t.Fatal(err)
	}
	if err := ops.AddBoth(ctx, row.Entry{Id: "b", At: at}, r.Entry{Code: 8}); err != nil {
		t.Fatal(err)
	}
	// 二つ目が失敗すれば（同じキー）、一つ目も残らない
	if err := ops.AddBoth(ctx, row.Entry{Id: "c", At: at}, r.Entry{Code: 8}); err == nil {
		t.Fatal("二つ目が失敗する AddBoth が、エラーを返さない")
	}
	if got, err := vault.NewEntriesATable(db).Get(ctx); err != nil || len(got) != 2 || got[0].Id != "a" || got[1].Id != "b" || !got[1].At.Equal(at) {
		t.Errorf("entriesA = %+v, %v", got, err)
	}
	if got, err := vault.NewEntriesBTable(db).Get(ctx); err != nil || len(got) != 2 || got[0].Code != 7 || len(got[0].Tags) != 1 || got[1].Code != 8 {
		t.Errorf("entriesB = %+v, %v", got, err)
	}
}

// TestInnerOp は、内部の op（export がない）: 状態の単位のメソッドは、export の有無によらず公開で、内側の New にも、db.go の型を
// そのまま渡せる（入口の型は、内側の型の別名）。
func TestInnerOp(t *testing.T) {
	db, ctx := newVault(t)
	ops := inner.New(vault.NewSecretsTable(db), vault.NewEntriesATable(db), vault.NewEntriesBTable(db), vault.NewSharedTable(db))
	if err := ops.Add(ctx, secret("k1", 1)); err != nil {
		t.Fatal(err)
	}
	if got, err := ops.Touch(ctx, "k1"); err != nil || got == nil || got.Row != "k1" {
		t.Errorf("Touch(k1) = %+v, %v", got, err)
	}
	if got, err := vault.NewSecretsTable(db).Touch(ctx, "nobody"); err != nil || got != nil {
		t.Errorf("Touch(nobody) = %+v, %v（期待は nil, nil）", got, err)
	}
}
