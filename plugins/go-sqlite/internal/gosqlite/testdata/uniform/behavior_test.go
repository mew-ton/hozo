// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の「型に依らない形の確かめ」）: 三つの名前空間の db.go は、名前と型の
// ほかは同じテキスト（plugins/go-sqlite の TestUniform）。違いは、実行時に、補助の中で出る: 核の型も scalar も同じ列の値になり、
// 省略可能な値は、値がなければ NULL になる。plugins/go-sqlite の TestBehavior が、基底の出力と go-sqlite の出力を置いたモジュールの
// ルートに置いて走らせる。
package behavior

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"example.com/app/gen/named"
	"example.com/app/gen/plain"
	"example.com/app/gen/sparse"

	_ "modernc.org/sqlite"
)

func open(t *testing.T, create func(context.Context, *sql.DB) error) (*sql.DB, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := create(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db, ctx
}

func ptr[T any](v T) *T { return &v }

var at = time.Date(2026, 7, 8, 9, 10, 11, 12, time.UTC)

// stored は、表 rows の、s が key の行の、列の値（SQLite の型と、文字列にした値）。j の列は除く（三つの名前空間で、型が違う）。
func stored(t *testing.T, db *sql.DB, key string) string {
	t.Helper()
	var out string
	err := db.QueryRow(`SELECT typeof("s") || ':' || "s" || ' ' || typeof("i") || ':' || "i" || ' ' || typeof("d") || ':' || "d" || ' ' ||
		typeof("f") || ':' || "f" || ' ' || typeof("b") || ':' || "b" || ' ' || typeof("at") || ':' || "at" FROM "rows" WHERE "s" = ?1`, key).Scan(&out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

const wantStored = "text:k integer:7 integer:1500000000 real:2.5 integer:1 text:2026-07-08T09:10:11.000000012Z"

// TestSameColumns は、同じ値が、核の型でも、scalar でも、省略可能でも、同じ列の値になること。
func TestSameColumns(t *testing.T) {
	pdb, ctx := open(t, plain.CreateTables)
	p := plain.New(plain.NewRowsTable(pdb))
	prow := plain.Row{S: "k", I: 7, D: 1500 * time.Millisecond, F: 2.5, B: true, At: at, J: []string{"x"}}
	if err := p.Add(ctx, prow); err != nil {
		t.Fatal(err)
	}
	ndb, _ := open(t, named.CreateTables)
	n := named.New(named.NewRowsTable(ndb))
	nrow := named.Row{S: "k", I: 7, D: named.D(1500 * time.Millisecond), F: 2.5, B: true, At: named.At(at), J: map[string]named.Tag{"x": {Name: "y"}}}
	if err := n.Add(ctx, nrow); err != nil {
		t.Fatal(err)
	}
	sdb, _ := open(t, sparse.CreateTables)
	s := sparse.New(sparse.NewRowsTable(sdb))
	srow := sparse.Row{S: ptr("k"), I: ptr(int64(7)), D: ptr(1500 * time.Millisecond), F: ptr(2.5), B: ptr(true), At: ptr(at), J: &sparse.Tag{Name: "y"}}
	if err := s.Add(ctx, srow); err != nil {
		t.Fatal(err)
	}
	for name, db := range map[string]*sql.DB{"plain": pdb, "named": ndb, "sparse": sdb} {
		if got := stored(t, db, "k"); got != wantStored {
			t.Errorf("%s の列の値が %s（期待は %s）", name, got, wantStored)
		}
	}

	// 読んだ値は、書いた値と同じ。どの op も、三つの名前空間で、同じ行に合う
	if got, err := p.Find(ctx, "k"); err != nil || got == nil || !reflect.DeepEqual(*got, prow) {
		t.Errorf("plain の Find = %+v, %v", got, err)
	}
	if got, err := n.Find(ctx, "k"); err != nil || got == nil || !reflect.DeepEqual(*got, nrow) {
		t.Errorf("named の Find = %+v, %v", got, err)
	}
	if got, err := s.Find(ctx, ptr("k")); err != nil || got == nil || !reflect.DeepEqual(*got, srow) {
		t.Errorf("sparse の Find = %+v, %v", got, err)
	}
	type lists struct {
		name string
		rows int
		err  error
	}
	count := func(name string) func(rows any, err error) lists {
		return func(rows any, err error) lists { return lists{name, reflect.ValueOf(rows).Len(), err} }
	}
	for _, c := range []lists{
		count("plain.ByI")(p.ByI(ctx, 7)), count("named.ByI")(n.ByI(ctx, 7)), count("sparse.ByI")(s.ByI(ctx, ptr(int64(7)))),
		count("plain.ByD")(p.ByD(ctx, 1500*time.Millisecond)), count("named.ByD")(n.ByD(ctx, named.D(1500*time.Millisecond))), count("sparse.ByD")(s.ByD(ctx, ptr(1500*time.Millisecond))),
		count("plain.ByF")(p.ByF(ctx, 2.5)), count("named.ByF")(n.ByF(ctx, 2.5)), count("sparse.ByF")(s.ByF(ctx, ptr(2.5))),
		count("plain.ByB")(p.ByB(ctx, true)), count("named.ByB")(n.ByB(ctx, true)), count("sparse.ByB")(s.ByB(ctx, ptr(true))),
		count("plain.ByAt")(p.ByAt(ctx, at)), count("named.ByAt")(n.ByAt(ctx, named.At(at))), count("sparse.ByAt")(s.ByAt(ctx, ptr(at))),
		count("plain.ByJ")(p.ByJ(ctx, []string{"x"})), count("named.ByJ")(n.ByJ(ctx, map[string]named.Tag{"x": {Name: "y"}})), count("sparse.ByJ")(s.ByJ(ctx, &sparse.Tag{Name: "y"})),
	} {
		if c.err != nil || c.rows != 1 {
			t.Errorf("%s = %d 行, %v（期待は 1 行）", c.name, c.rows, c.err)
		}
	}

	// 書き換えて、消す（トランザクションの op も、同じ形）
	prow.I, nrow.I, srow.I = 8, 8, ptr(int64(8))
	if err := p.Save(ctx, prow); err != nil {
		t.Fatal(err)
	}
	if err := n.Save(ctx, nrow); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, srow); err != nil {
		t.Fatal(err)
	}
	for name, db := range map[string]*sql.DB{"plain": pdb, "named": ndb, "sparse": sdb} {
		var i int64
		if err := db.QueryRow(`SELECT "i" FROM "rows"`).Scan(&i); err != nil || i != 8 {
			t.Errorf("%s の Save の後の i = %d, %v", name, i, err)
		}
	}
	prow.S, nrow.S, srow.S = "k2", "k2", ptr("k2")
	if err := p.Swap(ctx, at, prow); err != nil {
		t.Fatal(err)
	}
	if err := n.Swap(ctx, named.At(at), nrow); err != nil {
		t.Fatal(err)
	}
	if err := s.Swap(ctx, ptr(at), srow); err != nil {
		t.Fatal(err)
	}
	for name, db := range map[string]*sql.DB{"plain": pdb, "named": ndb, "sparse": sdb} {
		var key string
		if err := db.QueryRow(`SELECT group_concat("s") FROM "rows"`).Scan(&key); err != nil || key != "k2" {
			t.Errorf("%s の Swap の後の行が %s, %v（期待は k2 だけ）", name, key, err)
		}
	}
	if err := p.Remove(ctx, at); err != nil {
		t.Fatal(err)
	}
	if err := n.Remove(ctx, named.At(at)); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(ctx, ptr(at)); err != nil {
		t.Fatal(err)
	}
	for name, db := range map[string]*sql.DB{"plain": pdb, "named": ndb, "sparse": sdb} {
		var rows int
		if err := db.QueryRow(`SELECT COUNT(*) FROM "rows"`).Scan(&rows); err != nil || rows != 0 {
			t.Errorf("%s の Remove の後の行の数が %d, %v", name, rows, err)
		}
	}
}

// TestAbsentValues は、同じ形のコードの、実行時の違い: 値のない省略可能な値は NULL になり、NULL は nil に読める。
// 値のない引数は、値のない行に合う（条件は、どの名前空間でも同じ `"c" IS ?1`）。
func TestAbsentValues(t *testing.T) {
	db, ctx := open(t, sparse.CreateTables)
	rows := sparse.NewRowsTable(db)
	s := sparse.New(rows)
	if err := s.Add(ctx, sparse.Row{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(ctx, sparse.Row{S: ptr("k"), I: ptr(int64(0)), B: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	var nulls int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "rows" WHERE "s" IS NULL AND "i" IS NULL AND "d" IS NULL AND "f" IS NULL AND "b" IS NULL AND "at" IS NULL AND "j" IS NULL`).Scan(&nulls); err != nil || nulls != 1 {
		t.Fatalf("すべての列が NULL の行が %d, %v（期待は 1）", nulls, err)
	}
	got, err := rows.Get(ctx)
	if err != nil || len(got) != 2 || !reflect.DeepEqual(got[0], sparse.Row{}) {
		t.Fatalf("Get = %+v, %v（期待は、一行目のどのフィールドも nil）", got, err)
	}
	if g := got[1]; g.S == nil || *g.S != "k" || g.I == nil || *g.I != 0 || g.B == nil || *g.B || g.D != nil || g.F != nil || g.At != nil || g.J != nil {
		t.Errorf("二行目が %+v（期待は、零値を指すポインタは値、ほかは nil）", g)
	}
	// 値のない引数は、値のない行に合う。零値を指す引数は、零値の行に合う
	if got, err := s.Find(ctx, nil); err != nil || got == nil || got.S != nil {
		t.Errorf("Find(nil) = %+v, %v（期待は、s が NULL の行）", got, err)
	}
	if got, err := s.ByI(ctx, nil); err != nil || len(got) != 1 || got[0].S != nil {
		t.Errorf("ByI(nil) = %+v, %v（期待は、i が NULL の一行）", got, err)
	}
	if got, err := s.ByI(ctx, ptr(int64(0))); err != nil || len(got) != 1 || got[0].S == nil {
		t.Errorf("ByI(0) = %+v, %v（期待は、i が 0 の一行）", got, err)
	}
	if got, err := s.ByB(ctx, ptr(false)); err != nil || len(got) != 1 || got[0].S == nil {
		t.Errorf("ByB(false) = %+v, %v（期待は、b が 0 の一行）", got, err)
	}
	if got, err := s.ByJ(ctx, nil); err != nil || len(got) != 2 {
		t.Errorf("ByJ(nil) = %+v, %v（期待は、j が NULL の二行）", got, err)
	}
	// @db.update のキーの条件も `IS`: キーが NULL の行を、書き換えられる
	if err := s.Save(ctx, sparse.Row{F: ptr(1.5)}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Find(ctx, nil); err != nil || got == nil || got.F == nil || *got.F != 1.5 {
		t.Errorf("Save の後の Find(nil) = %+v, %v", got, err)
	}
	// @db.delete も同じ: at が NULL の行（二行とも）を消す
	if err := s.Remove(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if empty, err := rows.IsEmpty(ctx); err != nil || !empty {
		t.Errorf("Remove(nil) の後の IsEmpty = %v, %v", empty, err)
	}
}
