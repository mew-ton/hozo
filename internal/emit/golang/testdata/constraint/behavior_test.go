package inventory

// このファイルは生成物ではない。internal/emit/golang の TestBehavior が、生成した inventory のパッケージ
// （testdata/constraint/expected/gen/inventory と同じ内容）の隣に置いて `go test` する。valid に足した制約
// （docs/emit-go.md「scalar」の ConstraintFuncs）を確かめる。

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mew-ton/hozo/runtime"
)

func wantMissing(t *testing.T, what string, err error, name string) {
	t.Helper()
	var miss *runtime.MissingError
	if !errors.As(err, &miss) || miss.Kind != "constraint" || miss.Name != name {
		t.Errorf("%s: err = %#v, want *runtime.MissingError{Kind: \"constraint\", Name: %q}", what, err, name)
	}
}

// 設定していない制約に Validate が当たったら、違反ではなく *runtime.MissingError（panic しない）。
// model の Validate は、フィールドの Validate が返したそれをそのまま返す。
func TestMissingConstraint(t *testing.T) {
	Constraints = ConstraintFuncs{}
	wantMissing(t, "Isbn", Isbn("4000000000").Validate(), "valid.checkDigit")
	wantMissing(t, "Sku（継承した制約）", Sku("A000000000").Validate(), "valid.checkDigit")
	wantMissing(t, "Qty", Qty(6).Validate(), "valid.multipleOf")
	wantMissing(t, "Item（フィールドの型の制約）", Item{Isbn: "4000000000", Qty: 6}.Validate(), "valid.checkDigit")

	// 一つだけ設定すれば、設定していないもう一つで当たる
	Constraints.CheckDigit = func(string) bool { return true }
	if err := Isbn("4000000000").Validate(); err != nil {
		t.Errorf("Isbn: %v", err)
	}
	wantMissing(t, "Item（qty）", Item{Isbn: "4000000000", Qty: 6}.Validate(), "valid.multipleOf")
}

func TestConstraints(t *testing.T) {
	var gotBase int64
	Constraints = ConstraintFuncs{
		CheckDigit: func(v string) bool { return v != "" && v[len(v)-1] == '0' },
		MultipleOf: func(v int64, base int64) bool { gotBase = base; return v%base == 0 },
	}
	if err := (Item{Isbn: "4000000000", Qty: 12, Code: "x0", Alts: []Sku{"A000000000"}}).Validate(); err != nil {
		t.Errorf("違反がないのに %v", err)
	}
	if gotBase != 6 {
		t.Errorf("multipleOf の引数 base = %d, want 6", gotBase)
	}
	err := Item{Isbn: "4000000001", Qty: 7, Code: "x1", Alts: []Sku{"A000000000", "b000000001"}}.Validate()
	var ve *runtime.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %#v, want *runtime.ValidationError", err)
	}
	want := []runtime.Violation{
		{Path: "isbn", Constraint: "checkDigit"},
		{Path: "qty", Constraint: "multipleOf"},
		{Path: "code", Constraint: "checkDigit"},
		{Path: "alts[1]", Constraint: "checkDigit"},
		{Path: "alts[1]", Constraint: "pattern"},
	}
	if !reflect.DeepEqual(ve.Violations, want) {
		t.Errorf("Violations = %+v\nwant %+v", ve.Violations, want)
	}
}
