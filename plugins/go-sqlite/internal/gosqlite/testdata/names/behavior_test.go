// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 当たる名前を避けたコード（docs/plugin-go-sqlite.md
// 「補助」の末尾）が、コンパイルできて、仕様の通りに動くこと。出力は、内側のフォルダと入口に分かれている（db.go は入口のパッケージ）。
// plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	bodypkg "example.com/app/gen/body"
	ctxpkg "example.com/app/gen/ctx"
	errpkg "example.com/app/gen/err"
	rpkg "example.com/app/gen/r"
	rowpkg "example.com/app/gen/row"
	spkg "example.com/app/gen/s"
	"example.com/app/gen/vault"

	_ "modernc.org/sqlite"
)

// leaves は、vault の葉の op（戻り値の型のパッケージの名前が、継ぎ目のメソッドの中の局所の名前と同じもの）の、人が書いた実装。
type leaves struct{ secrets vault.SecretsTable }

// Mark は、秘密があれば、その ctx の列の値を返す。なければ、失敗する。
func (l leaves) Mark(ctx context.Context, t vault.Key) (spkg.Mark, error) {
	secret, err := l.secrets.ByT(ctx, t)
	if err != nil {
		return spkg.Mark{}, err
	}
	if secret == nil {
		return spkg.Mark{}, errors.New("no secret")
	}
	return spkg.Mark{N: int64(secret.Ctx)}, nil
}

func (l leaves) Info(ctx context.Context, t vault.Key) (*ctxpkg.Info, error) {
	secret, err := l.secrets.ByT(ctx, t)
	if err != nil || secret == nil {
		return nil, err
	}
	return &ctxpkg.Info{N: int64(secret.Ctx)}, nil
}

func (l leaves) Faults(ctx context.Context, t vault.Key) ([]errpkg.Fault, error) {
	return []errpkg.Fault{{N: 1}, {N: 2}}, nil
}

func (l leaves) Part(ctx context.Context, s vault.Key, body vault.Key) (bodypkg.Part, error) {
	return bodypkg.Part{N: int64(len(s) + len(body))}, nil
}

// Report は、秘密を一つ足してから、失敗する（継ぎ目が、取り消す）。
func (l leaves) Report(ctx context.Context, fault errpkg.Fault) error {
	if err := l.secrets.Post(ctx, vault.Secret{T: "reported"}); err != nil {
		return err
	}
	return errors.New("reported")
}

type world struct {
	secrets vault.SecretsTable
	a       vault.EntriesATable
	b       vault.EntriesBTable
	ops     vault.Ops
}

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
	if err := vault.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	w := world{secrets: vault.NewSecretsTable(db), a: vault.NewEntriesATable(db), b: vault.NewEntriesBTable(db)}
	// 入口の New に、入口の型（内側の型の別名）を、そのまま渡せる
	w.ops = vault.New(leaves{w.secrets}, w.secrets, w.a, w.b, vault.NewDbSeams(db))
	return w, ctx
}

var at = time.Date(2026, 7, 8, 9, 10, 11, 0, time.UTC)

func secret(t vault.Key) vault.Secret {
	return vault.Secret{
		T: t, Ctx: 7, Err: 0.5, Args: true, Type: at, String: []vault.Tag{{Name: "x"}}, Json: map[string]string{"k": "v"},
		Sql: time.Minute, Runtime: "rt", DbGet: "get", DbScanSecret: "scan", S: "s", Value: "v",
	}
}

func keys(list []vault.Secret) []vault.Key {
	out := []vault.Key{}
	for _, x := range list {
		out = append(out, x.T)
	}
	return out
}

// TestSecrets は、名前が当たる引数を持つ read と効果: 引数の局所の名前を変えても、比べる列は、仕様の引数の名前のまま。
func TestSecrets(t *testing.T) {
	w, ctx := newWorld(t)
	// 形で決まる命令 Post（配線が直に呼ぶ）と、行の引数の名前が args の効果
	if err := w.ops.Add(ctx, secret("k1")); err != nil {
		t.Fatal(err)
	}
	second := secret("k2")
	second.Ctx, second.Time, second.Any = 8, &at, &vault.Tag{Name: "any"}
	if err := w.secrets.CreateSecret(ctx, second); err != nil {
		t.Fatal(err)
	}
	// 内部の op を呼ぶ配線
	got, err := w.ops.Find(ctx, "k2")
	if err != nil || got == nil || got.Ctx != 8 || got.Time == nil || !got.Time.Equal(at) || got.Any == nil || got.Any.Name != "any" ||
		got.Sql != time.Minute || !reflect.DeepEqual(got.Json, vault.Labels{"k": "v"}) || got.Value != "v" {
		t.Errorf("Find(k2) = %+v, %v", got, err)
	}
	if got, err := w.secrets.ByLocals(ctx, 7, 0.5, true); err != nil || !reflect.DeepEqual(keys(got), []vault.Key{"k1"}) {
		t.Errorf("ByLocals(7, 0.5, true) = %v, %v（期待は k1）", keys(got), err)
	}
	if got, err := w.secrets.ByWords(ctx, at); err != nil || !reflect.DeepEqual(keys(got), []vault.Key{"k1", "k2"}) {
		t.Errorf("ByWords(at) = %v, %v（期待は k1、k2）", keys(got), err)
	}
	// 列 time は、仕様の予約語（核の型 time）なので、引数の名前に書けない: 条件は、ほかの三つの列だけ
	if got, err := w.secrets.ByImports(ctx, map[string]string{"k": "v"}, time.Minute, "rt"); err != nil || !reflect.DeepEqual(keys(got), []vault.Key{"k1", "k2"}) {
		t.Errorf("ByImports(…) = %v, %v（期待は k1、k2）", keys(got), err)
	}
	if got, err := w.secrets.ByPrivates(ctx, "get", "scan", "s"); err != nil || len(got) != 2 {
		t.Errorf("ByPrivates(get, scan, s) = %v, %v（期待は二行）", keys(got), err)
	}
	// 行の引数の名前が err の効果
	second.Runtime = "changed"
	if err := w.secrets.UpdateSecret(ctx, second); err != nil {
		t.Fatal(err)
	}
	if got, err := w.secrets.ByT(ctx, "k2"); err != nil || got == nil || got.Runtime != "changed" {
		t.Errorf("UpdateSecret の後の ByT(k2) = %+v, %v", got, err)
	}
	// 三つの列の @db.delete
	if err := w.secrets.DeleteSecret(ctx, "k1", 8, "get"); err != nil {
		t.Fatal(err)
	}
	if err := w.secrets.DeleteSecret(ctx, "k2", 8, "get"); err != nil {
		t.Fatal(err)
	}
	if got, err := w.secrets.Get(ctx); err != nil || !reflect.DeepEqual(keys(got), []vault.Key{"k1"}) {
		t.Errorf("DeleteSecret の後の Get = %v, %v（期待は k1）", keys(got), err)
	}
}

// TestEntries は、同じ名前の model（row.Entry と r.Entry）の二つの表: 補助（dbArgsEntry と dbArgsEntry2）が、それぞれの表を読み書きする。
func TestEntries(t *testing.T) {
	w, ctx := newWorld(t)
	if err := w.a.CreateA(ctx, rowpkg.Entry{Id: "e1", At: at}); err != nil {
		t.Fatal(err)
	}
	if err := w.b.CreateB(ctx, rpkg.Entry{Code: 9, Tags: []string{"t"}}); err != nil {
		t.Fatal(err)
	}
	if got, err := w.a.Get(ctx); err != nil || len(got) != 1 || got[0].Id != "e1" || !got[0].At.Equal(at) {
		t.Errorf("entriesA の Get = %+v, %v", got, err)
	}
	if got, err := w.b.Get(ctx); err != nil || len(got) != 1 || got[0].Code != 9 || !reflect.DeepEqual(got[0].Tags, []string{"t"}) {
		t.Errorf("entriesB の Get = %+v, %v", got, err)
	}
}

// TestSeams は、名前が当たる引数と戻り値の型を持つ op の継ぎ目: 引数を、そのまま中身に渡し、戻り値を、そのまま返す。失敗すれば、取り消す。
func TestSeams(t *testing.T) {
	w, ctx := newWorld(t)
	if err := w.ops.Add(ctx, secret("old")); err != nil {
		t.Fatal(err)
	}
	// 五つの効果を、一つのトランザクションで: 足す、書き換える、消す、二つの表に足す
	changed := secret("new")
	changed.Runtime = "changed"
	if err := w.ops.Mix(ctx, secret("new"), changed, "old", 7, "get", rowpkg.Entry{Id: "e1", At: at}, rpkg.Entry{Code: 1}); err != nil {
		t.Fatal(err)
	}
	if got, err := w.secrets.Get(ctx); err != nil || len(got) != 1 || got[0].T != "new" || got[0].Runtime != "changed" {
		t.Errorf("mix の後の Get = %+v, %v（期待は、書き換えた new だけ）", got, err)
	}
	// 最後の効果が失敗する（entriesB の同じキー）: どれも残らない
	if err := w.ops.Mix(ctx, secret("more"), changed, "new", 7, "get", rowpkg.Entry{Id: "e2", At: at}, rpkg.Entry{Code: 1}); err == nil {
		t.Fatal("同じキーの mix が、失敗しない")
	}
	if got, err := w.secrets.Get(ctx); err != nil || !reflect.DeepEqual(keys(got), []vault.Key{"new"}) {
		t.Errorf("失敗した mix の後の Get = %v, %v（期待は new だけ）", keys(got), err)
	}
	if got, err := w.a.Get(ctx); err != nil || len(got) != 1 {
		t.Errorf("失敗した mix の後の entriesA の Get = %+v, %v（期待は一行）", got, err)
	}
	// 戻り値の型のパッケージの名前が、継ぎ目のメソッドの中の局所の名前と同じ op
	if got, err := w.ops.Mark(ctx, "new"); err != nil || got.N != 7 {
		t.Errorf("Mark(new) = %+v, %v（期待は N = 7）", got, err)
	}
	if got, err := w.ops.Mark(ctx, "nobody"); err == nil || got.N != 0 {
		t.Errorf("Mark(nobody) = %+v, %v（期待は、零値とエラー）", got, err)
	}
	if got, err := w.ops.Info(ctx, "new"); err != nil || got == nil || got.N != 7 {
		t.Errorf("Info(new) = %+v, %v", got, err)
	}
	if got, err := w.ops.Faults(ctx, "new"); err != nil || len(got) != 2 {
		t.Errorf("Faults(new) = %+v, %v", got, err)
	}
	if got, err := w.ops.Part(ctx, "ab", "cde"); err != nil || got.N != 5 {
		t.Errorf("Part(ab, cde) = %+v, %v（期待は N = 5）", got, err)
	}
	// 戻り値がない形: 葉が足した行は、葉が失敗すれば、残らない
	if err := w.ops.Report(ctx, errpkg.Fault{N: 1}); err == nil || err.Error() != "reported" {
		t.Errorf("Report のエラーが %v（期待は、葉のエラーそのもの）", err)
	}
	if got, err := w.secrets.Get(ctx); err != nil || !reflect.DeepEqual(keys(got), []vault.Key{"new"}) {
		t.Errorf("失敗した Report の後の Get = %v, %v（期待は new だけ）", keys(got), err)
	}
}
