// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 「効果の中身」の「行の遷移を持つ効果」
// （docs/plugin-go-sqlite.md）: db のデコレータを付けた効果の SQL は、行の遷移があっても変わらず、行のケースを確かめない
// （前のケースも、後のケースも）。デコレータを付けられない行の遷移の効果（後のケースが前の並びにない put、行を受け取る delete）は、
// 人が、型を埋め込んで書き足す。plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の出力を置いたモジュールのルートに
// 置いて走らせる。
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

// orders は、shop.OrdersTable を埋め込んで、足りない効果（start、discard）を書き足した型。文は、shop.Conn を通して実行する。
type orders struct {
	shop.OrdersTable
	db *sql.DB
}

// Start は、作り始める: 始めた時刻を入れる。行を InProduction にする書き方（どの列を、何に変えるか）を知っているのは、人。
func (o orders) Start(ctx context.Context, order shop.Order, at time.Time) error {
	_, err := shop.Conn(ctx, o.db).ExecContext(ctx, `UPDATE "orders" SET "startedAt" = ?2 WHERE "id" = ?1`, order.Id, at.UTC().Format("2006-01-02T15:04:05.000000000Z"))
	return err
}

func (o orders) Discard(ctx context.Context, order shop.Order) error {
	_, err := shop.Conn(ctx, o.db).ExecContext(ctx, `DELETE FROM "orders" WHERE "id" = ?1`, order.Id)
	return err
}

// stage は StageCases の実装: startedAt があれば InProduction、なければ Accepted。
type stage struct{}

func (stage) Accepted(_ context.Context, v shop.Order) (bool, error) { return v.StartedAt == nil, nil }
func (stage) InProduction(_ context.Context, v shop.Order) (bool, error) {
	return v.StartedAt != nil, nil
}

func open(t *testing.T) (orders, shop.Ops, context.Context) {
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
	tb := orders{OrdersTable: shop.NewOrdersTable(db), db: db}
	return tb, shop.New(tb, stage{}), ctx
}

func get(t *testing.T, tb orders, ctx context.Context, id string) *shop.Order {
	t.Helper()
	got, err := tb.OrderById(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func classify(t *testing.T, ctx context.Context, o *shop.Order) shop.Stage {
	t.Helper()
	k, err := shop.ClassifyStage(ctx, stage{}, *o)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// @db.insert は、入る行を、そのまま足す。行が、宣言した後のケース（Accepted）にあるかは、確かめない
func TestInsertDoesNotCheckTheCase(t *testing.T) {
	tb, _, ctx := open(t)
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := tb.Accept(ctx, shop.Order{Id: "a", Note: "new"}); err != nil {
		t.Fatal(err)
	}
	// Accepted でない行（startedAt がある）を渡しても、SQL は、そのまま足す
	if err := tb.Accept(ctx, shop.Order{Id: "b", Note: "already", StartedAt: &started}); err != nil {
		t.Fatal(err)
	}
	if err := tb.Keep(ctx, shop.Order{Id: "c", Note: "kept"}); err != nil {
		t.Fatal(err)
	}
	if got := classify(t, ctx, get(t, tb, ctx, "a")); got != shop.StageAccepted {
		t.Errorf("a のケース = %v, want Accepted", got)
	}
	if got := classify(t, ctx, get(t, tb, ctx, "b")); got != shop.StageInProduction {
		t.Errorf("b のケース = %v, want InProduction（SQL は、渡した行を、そのまま足す）", got)
	}
	if got := get(t, tb, ctx, "c"); got == nil || got.Note != "kept" {
		t.Errorf("c = %+v", got)
	}
}

// @db.update は、渡した行のすべての列を、キーが同じ行に書く。保存してある行のケースも、書いた後の行のケースも、確かめない
func TestUpdateDoesNotCheckTheCase(t *testing.T) {
	tb, _, ctx := open(t)
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := tb.Accept(ctx, shop.Order{Id: "a", Note: "new"}); err != nil {
		t.Fatal(err)
	}
	// as any -> InProduction: 渡した行が InProduction にあれば、そうなる
	if err := tb.Overwrite(ctx, shop.Order{Id: "a", Note: "new", StartedAt: &started}); err != nil {
		t.Fatal(err)
	}
	if got := classify(t, ctx, get(t, tb, ctx, "a")); got != shop.StageInProduction {
		t.Errorf("Overwrite の後のケース = %v, want InProduction", got)
	}
	// 渡した行が InProduction になくても、SQL は、そのまま書く（行を作る側が担う）
	if err := tb.Overwrite(ctx, shop.Order{Id: "a", Note: "new"}); err != nil {
		t.Fatal(err)
	}
	if got := classify(t, ctx, get(t, tb, ctx, "a")); got != shop.StageAccepted {
		t.Errorf("Accepted の行を渡した Overwrite の後のケース = %v, want Accepted（SQL は、後のケースを確かめない）", got)
	}
	// as Accepted -> Accepted（ケースを変えない書き換え）と、as InProduction -> else: 保存してある行のケースを、WHERE に足さない
	if err := tb.Annotate(ctx, shop.Order{Id: "a", Note: "noted"}); err != nil {
		t.Fatal(err)
	}
	if err := tb.Revise(ctx, shop.Order{Id: "a", Note: "revised"}); err != nil {
		t.Fatal(err)
	}
	if got := get(t, tb, ctx, "a"); got == nil || got.Note != "revised" || got.StartedAt != nil {
		t.Errorf("a = %+v, want note=revised, startedAt なし（Revise は、Accepted の行にも書いた）", got)
	}
	// 行の遷移を書かない @db.update と、同じ SQL
	if err := tb.Rename(ctx, shop.Order{Id: "a", Note: "renamed"}); err != nil {
		t.Fatal(err)
	}
	if got := get(t, tb, ctx, "a"); got == nil || got.Note != "renamed" {
		t.Errorf("a = %+v", got)
	}
}

// 人が書き足した効果（start、discard）を、配線が呼ぶ。行を、宣言した後のケースにするのは、人のメソッド
func TestHumanWrittenTransition(t *testing.T) {
	tb, ops, ctx := open(t)
	at := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	order := shop.Order{Id: "a", Note: "new"}
	if err := tb.Accept(ctx, order); err != nil {
		t.Fatal(err)
	}
	if err := ops.Begin(ctx, order, at); err != nil {
		t.Fatal(err)
	}
	got := get(t, tb, ctx, "a")
	if got == nil || got.StartedAt == nil || !got.StartedAt.Equal(at) {
		t.Fatalf("Begin の後の行 = %+v, want startedAt = %v", got, at)
	}
	if k := classify(t, ctx, got); k != shop.StageInProduction {
		t.Errorf("Begin の後のケース = %v, want InProduction", k)
	}
	// もう InProduction の行は、Accepted のアームに入らないので、start を呼ばない
	later := at.Add(time.Hour)
	if err := ops.Begin(ctx, *got, later); err != nil {
		t.Fatal(err)
	}
	if again := get(t, tb, ctx, "a"); !reflect.DeepEqual(again, got) {
		t.Errorf("二度目の Begin が、行を変えた: %+v", again)
	}
	if err := tb.Discard(ctx, *got); err != nil {
		t.Fatal(err)
	}
	if gone := get(t, tb, ctx, "a"); gone != nil {
		t.Errorf("Discard の後も、行がある: %+v", gone)
	}
}
