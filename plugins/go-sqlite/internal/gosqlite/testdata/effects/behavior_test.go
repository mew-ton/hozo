// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go-sqlite`）: 「効果の中身」の表の各行（@db.insert、
// @db.update、一つと二つ以上の列の @db.delete）と、形で決まる命令 Post（docs/plugin-go-sqlite.md）を、本物の SQLite の上で確かめる。
// plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/app/gen/depot"

	_ "modernc.org/sqlite"
)

// tables は、新しいメモリの上の SQLite に作った、三つの表の型と、その上の Ops。
type tables struct {
	crates depot.CratesTable
	slots  depot.SlotsTable
	events depot.EventsTable
	tags   depot.TagsTable
	ops    depot.Ops
}

// open は、新しいメモリの上の SQLite に表を作る。go-sqlite の三つの型を、基底の New にそのまま渡す（EventsTable は、配線が直に呼ぶ
// 形で決まる命令 Post も持つ。持たなければ、ここでコンパイルできない）。
func open(t *testing.T) (tables, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := depot.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	tb := tables{crates: depot.NewCratesTable(db), slots: depot.NewSlotsTable(db), events: depot.NewEventsTable(db), tags: depot.NewTagsTable(db)}
	tb.ops = depot.New(tb.crates, tb.slots, tb.events, tb.tags)
	return tb, ctx
}

func ptr[T any](v T) *T { return &v }

// codes は、木箱の表の行の code（足した順）。
func codes(t *testing.T, tb tables, ctx context.Context) []depot.Code {
	t.Helper()
	got, err := tb.crates.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := []depot.Code{}
	for _, c := range got {
		out = append(out, c.Code)
	}
	return out
}

func seed(t *testing.T, tb tables, ctx context.Context) {
	t.Helper()
	for _, c := range []depot.Crate{
		{Code: "c1", Label: "tea", Weight: 5, Zone: ptr("north"), Contents: []string{"green", "black"}},
		{Code: "c2", Label: "tea", Weight: 9, Contents: []string{}},
		{Code: "c3", Label: "salt", Weight: 5, Zone: ptr("south"), Contents: []string{"sea"}},
		{Code: "c4", Label: "tea", Weight: 5, Contents: []string{"white"}},
	} {
		if err := tb.crates.CreateCrate(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
}

// TestInsert は @db.insert: 引数の行を足す。同じキーの行は、足せない（表の制約）。
func TestInsert(t *testing.T) {
	tb, ctx := open(t)
	seed(t, tb, ctx)
	if got := codes(t, tb, ctx); !reflect.DeepEqual(got, []depot.Code{"c1", "c2", "c3", "c4"}) {
		t.Errorf("足した後の行が %v", got)
	}
	if err := tb.ops.AddCrate(ctx, depot.Crate{Code: "c1", Label: "dup"}); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("同じキーの行を足したときのエラーが %v", err)
	}
	got, err := tb.crates.CrateByCode(ctx, "c1")
	if err != nil || got == nil || got.Label != "tea" || got.Zone == nil || *got.Zone != "north" || !reflect.DeepEqual(got.Contents, []string{"green", "black"}) {
		t.Errorf("CrateByCode(c1) = %+v, %v", got, err)
	}
}

// TestUpdate は @db.update: キーが引数の行と同じ行の、キーでない列を、すべて書き換える。行がなければ何もしない。
func TestUpdate(t *testing.T) {
	tb, ctx := open(t)
	seed(t, tb, ctx)
	// 値のある列を、値のない列に。値のない列を、値のある列に
	if err := tb.ops.SaveCrate(ctx, depot.Crate{Code: "c1", Label: "coffee", Weight: 7, Contents: []string{"dark"}}); err != nil {
		t.Fatal(err)
	}
	if err := tb.crates.UpdateCrate(ctx, depot.Crate{Code: "c2", Label: "tea", Weight: 9, Zone: ptr("east"), Contents: []string{}}); err != nil {
		t.Fatal(err)
	}
	got, err := tb.crates.CrateByCode(ctx, "c1")
	if err != nil || got == nil || got.Label != "coffee" || got.Weight != 7 || got.Zone != nil || !reflect.DeepEqual(got.Contents, []string{"dark"}) {
		t.Errorf("書き換えた c1 = %+v, %v", got, err)
	}
	if got, err := tb.crates.CrateByCode(ctx, "c2"); err != nil || got == nil || got.Zone == nil || *got.Zone != "east" {
		t.Errorf("書き換えた c2 = %+v, %v", got, err)
	}
	// ほかの行は、変わらない。行の順（足した順）も、変わらない
	if got, err := tb.crates.CrateByCode(ctx, "c3"); err != nil || got == nil || got.Label != "salt" {
		t.Errorf("書き換えていない c3 = %+v, %v", got, err)
	}
	if got := codes(t, tb, ctx); !reflect.DeepEqual(got, []depot.Code{"c1", "c2", "c3", "c4"}) {
		t.Errorf("書き換えた後の行が %v", got)
	}
	// 行がなければ、何もしない（足さない）
	if err := tb.crates.UpdateCrate(ctx, depot.Crate{Code: "nobody", Label: "ghost"}); err != nil {
		t.Errorf("行のない書き換えのエラーが %v（期待は nil）", err)
	}
	if got := codes(t, tb, ctx); len(got) != 4 {
		t.Errorf("行のない書き換えの後の行が %v（期待は、四行のまま）", got)
	}
}

// TestUpdateByTwoKeys は、キーが二つの @db.update: 二つのキーが、どちらも同じ行だけを書き換える。
func TestUpdateByTwoKeys(t *testing.T) {
	tb, ctx := open(t)
	for _, s := range []depot.Slot{{Aisle: "a", Level: 1}, {Aisle: "a", Level: 2, Crate: ptr(depot.Code("c1"))}, {Aisle: "b", Level: 1}} {
		if err := tb.slots.CreateSlot(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := tb.ops.SaveSlot(ctx, depot.Slot{Aisle: "a", Level: 1, Crate: ptr(depot.Code("c9")), Locked: true}); err != nil {
		t.Fatal(err)
	}
	got, err := tb.slots.Get(ctx)
	if err != nil || len(got) != 3 {
		t.Fatalf("Get = %v, %v", got, err)
	}
	if got[0].Crate == nil || *got[0].Crate != "c9" || !got[0].Locked {
		t.Errorf("書き換えた a/1 = %+v", got[0])
	}
	if got[1].Crate == nil || *got[1].Crate != "c1" || got[1].Locked || got[2].Crate != nil || got[2].Locked {
		t.Errorf("書き換えていない行が a/2 = %+v、b/1 = %+v", got[1], got[2])
	}
	// 二つのキーの @db.delete
	if err := tb.slots.DeleteSlot(ctx, "a", 2); err != nil {
		t.Fatal(err)
	}
	if got, err := tb.slots.Get(ctx); err != nil || len(got) != 2 || got[0].Level != 1 || got[1].Aisle != "b" {
		t.Errorf("a/2 を消した後の Get = %+v, %v", got, err)
	}
}

// TestDelete は @db.delete: どの条件も満たす行を、すべて消す。行がなければ何もしない。
func TestDelete(t *testing.T) {
	tb, ctx := open(t)
	seed(t, tb, ctx)
	// 列が一つ
	if err := tb.ops.RemoveCrate(ctx, "c3"); err != nil {
		t.Fatal(err)
	}
	if got := codes(t, tb, ctx); !reflect.DeepEqual(got, []depot.Code{"c1", "c2", "c4"}) {
		t.Errorf("c3 を消した後の行が %v", got)
	}
	// 行がなければ、何もしない
	if err := tb.crates.DeleteCrate(ctx, "nobody"); err != nil {
		t.Errorf("行のない @db.delete のエラーが %v（期待は nil）", err)
	}
	// 列が二つ: label が tea で、weight が 5 の行（c1 と c4）。label が tea でも、weight が 9 の c2 は残る
	if err := tb.crates.DeleteLabelled(ctx, "tea", 5); err != nil {
		t.Fatal(err)
	}
	if got := codes(t, tb, ctx); !reflect.DeepEqual(got, []depot.Code{"c2"}) {
		t.Errorf("tea で 5 の行を消した後の行が %v", got)
	}
}

// TestDeleteNullable は、省略可能な列に T? の引数の @db.delete（「列と引数の型」）: 値があれば、同じ値の行。値がなければ、値のない行。
func TestDeleteNullable(t *testing.T) {
	tb, ctx := open(t)
	seed(t, tb, ctx)
	if err := tb.crates.DeleteInZone(ctx, ptr("north")); err != nil {
		t.Fatal(err)
	}
	if got := codes(t, tb, ctx); !reflect.DeepEqual(got, []depot.Code{"c2", "c3", "c4"}) {
		t.Errorf("north の行を消した後の行が %v", got)
	}
	if err := tb.crates.DeleteInZone(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got := codes(t, tb, ctx); !reflect.DeepEqual(got, []depot.Code{"c3"}) {
		t.Errorf("zone のない行を消した後の行が %v", got)
	}
}

// TestPost は、形で決まる命令 Post: 配線が直に呼ぶ組み込みの効果（events.post(event);）は、表に、引数の行を足す。
// 配線が呼んでいない効果（deleteEventsOf）も、メソッドになっている。
func TestPost(t *testing.T) {
	tb, ctx := open(t)
	at := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	for i, what := range []string{"in", "moved", "out"} {
		crate := depot.Code("c1")
		if i == 1 {
			crate = "c2"
		}
		if err := tb.ops.LogEvent(ctx, depot.Event{Crate: crate, What: what, At: at.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := tb.events.Get(ctx)
	if err != nil || len(got) != 3 || got[0].What != "in" || got[2].What != "out" || !got[1].At.Equal(at.Add(time.Hour)) {
		t.Fatalf("Get = %+v, %v（期待は、足した順の三行）", got, err)
	}
	if err := tb.events.Post(ctx, depot.Event{Crate: "c3", What: "in", At: at}); err != nil {
		t.Fatal(err)
	}
	if err := tb.events.DeleteEventsOf(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if got, err := tb.events.Get(ctx); err != nil || len(got) != 2 || got[0].Crate != "c2" || got[1].Crate != "c3" {
		t.Errorf("c1 の出来事を消した後の Get = %+v, %v", got, err)
	}
}

// TestConflict は、@db.conflict（docs/plugin-go-sqlite.md「効果の中身」）: 一意の制約（キーと、@db.unique）に当たった効果は、
// 名指しした例外を、値で返す。当たらなければ、ふつうに書く。一意の違反でない失敗は、例外にしない。@db.conflict のない効果の
// 一意の違反も、例外にしない。
func TestConflict(t *testing.T) {
	tb, ctx := open(t)
	names := func() []string {
		t.Helper()
		got, err := tb.tags.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, tag := range got {
			out = append(out, tag.Id+"="+tag.Name)
		}
		return out
	}
	for _, tag := range []depot.Tag{{Id: "t1", Name: "fragile"}, {Id: "t2", Name: "heavy"}} {
		if err := tb.ops.AddTag(ctx, tag); err != nil {
			t.Fatal(err)
		}
	}
	var taken depot.TagTaken
	// @db.insert: キー（id）が同じ行、一意の列（name）が同じ行
	if err := tb.ops.AddTag(ctx, depot.Tag{Id: "t1", Name: "other"}); !errors.As(err, &taken) {
		t.Errorf("キーが同じ行の AddTag = %v（期待は depot.TagTaken）", err)
	}
	if err := tb.ops.AddTag(ctx, depot.Tag{Id: "t3", Name: "heavy"}); !errors.As(err, &taken) {
		t.Errorf("一意の列が同じ行の AddTag = %v（期待は depot.TagTaken）", err)
	}
	// @db.update: 書き換えた行の、一意の列が、別の行と同じになる。当たらない書き換えは、通る
	if err := tb.ops.SaveTag(ctx, depot.Tag{Id: "t2", Name: "fragile"}); !errors.As(err, &taken) {
		t.Errorf("一意の列が、別の行と同じになる SaveTag = %v（期待は depot.TagTaken）", err)
	}
	if err := tb.ops.SaveTag(ctx, depot.Tag{Id: "t2", Name: "bulky"}); err != nil {
		t.Errorf("当たらない SaveTag = %v", err)
	}
	// 例外を返した効果は、起きていない
	if got, want := names(), []string{"t1=fragile", "t2=bulky"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %v、期待は %v", got, want)
	}
	// 一意の違反でない失敗（中断された ctx）は、例外にしない
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := tb.tags.CreateTag(cancelled, depot.Tag{Id: "t9", Name: "x"}); err == nil || errors.As(err, &taken) {
		t.Errorf("中断された ctx の CreateTag = %v（期待は、例外でない失敗）", err)
	}
	// @db.conflict のない効果の、一意の違反は、例外にしない（そのままのエラー）
	if err := tb.crates.CreateCrate(ctx, depot.Crate{Code: "c1"}); err != nil {
		t.Fatal(err)
	}
	if err := tb.crates.CreateCrate(ctx, depot.Crate{Code: "c1"}); err == nil || errors.As(err, &taken) {
		t.Errorf("@db.conflict のない効果の、キーが同じ行 = %v（期待は、例外でない失敗）", err)
	}
}

// TestCancelled は、文を、受け取った ctx で実行すること: 中断された ctx では、行は足されない。
func TestCancelled(t *testing.T) {
	tb, ctx := open(t)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := tb.crates.CreateCrate(cancelled, depot.Crate{Code: "c1"}); err == nil {
		t.Error("中断された ctx で、行を足せた")
	}
	if empty, err := tb.crates.IsEmpty(ctx); err != nil || !empty {
		t.Errorf("中断された ctx で足した後の IsEmpty = %v, %v（期待は true）", empty, err)
	}
}
