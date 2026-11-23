// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: リテラルの型（docs/plugin-go.md「型」の、
// リテラルの型の行）。配線が union に入れた値が、そのアームの、値を持たない包む型で、葉に届くこと。リテラルの型の
// フィールドを読んだ値が、そのリテラルであること。variant の match が、アームごとに分かれること。三つに分ける形
// （対象の型が boolean?）が、真、偽、値がない、を、それぞれのケースのアームに届け、ケースのない側を、分類の誤り
// （*runtime.ClassificationError）にすること。述語で決まる分類では、as の型に当たらない値で、述語が呼ばれないこと。
// plugins/go の TestBehavior が、生成したコードのモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/app"
	"github.com/mew-ton/hozo/runtime"
)

// leaves は、葉の op の実装。届いた値を、届いた順に覚える。
type leaves struct {
	levels []app.Severity
	shapes []app.Shape
	said   []string
	counts []int64
	shown  []bool
	kept   []app.Report
}

func (l *leaves) Report(_ context.Context, level app.Severity) error {
	l.levels = append(l.levels, level)
	return nil
}
func (l *leaves) Pick(_ context.Context) (app.Severity, error) { return app.SeverityInfo{}, nil }
func (l *leaves) Say(_ context.Context, text string) error {
	l.said = append(l.said, text)
	return nil
}
func (l *leaves) Count(_ context.Context, n int64) error {
	l.counts = append(l.counts, n)
	return nil
}
func (l *leaves) Show(_ context.Context, lit bool) error {
	l.shown = append(l.shown, lit)
	return nil
}
func (l *leaves) Keep(_ context.Context, r app.Report) error {
	l.kept = append(l.kept, r)
	return nil
}
func (l *leaves) Draw(_ context.Context, shape app.Shape) error {
	l.shapes = append(l.shapes, shape)
	return nil
}
func (l *leaves) Risky(_ context.Context) error { return app.Bad{Level: app.SeverityError{}} }

// moods は MoodCases の実装。呼ばれた述語を覚える。glad が真なら Glad、偽なら Calm。
type moods struct {
	glad   bool
	called []string
}

func (c *moods) Glad(_ context.Context, _ *bool) (bool, error) {
	c.called = append(c.called, "Glad")
	return c.glad, nil
}
func (c *moods) Calm(_ context.Context, _ *bool) (bool, error) {
	c.called = append(c.called, "Calm")
	return !c.glad, nil
}
func (c *moods) Sad(_ context.Context, _ *bool) (bool, error) {
	c.called = append(c.called, "Sad")
	return true, nil
}

func ptr[T any](v T) *T { return &v }

func classificationError(t *testing.T, err error, classify string) {
	t.Helper()
	var ce *runtime.ClassificationError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v（*runtime.ClassificationError でない）", err)
	}
	if ce.Classify != classify {
		t.Errorf("ClassificationError.Classify = %s, want %s", ce.Classify, classify)
	}
}

// 配線が union に入れた値は、そのアームの、値を持たない包む型
func TestEnter(t *testing.T) {
	ctx := context.Background()
	l := &leaves{}
	o := app.New(l, &moods{})
	if err := o.Raise(ctx); err != nil || !reflect.DeepEqual(l.levels, []app.Severity{app.SeverityError{}}) {
		t.Errorf("Raise: report に届いた値 = %#v, %v, want SeverityError{}", l.levels, err)
	}
	if got, err := o.Level(ctx); err != nil || got != (app.SeverityWarning{}) {
		t.Errorf("Level() = %#v, %v, want SeverityWarning{}", got, err)
	}
	if got, err := o.Held(ctx); err != nil || got != (app.SeverityError{}) {
		t.Errorf("Held() = %#v, %v, want SeverityError{}", got, err)
	}
	if err := o.File(ctx); err != nil || !reflect.DeepEqual(l.kept, []app.Report{{Severity: app.SeverityInfo{}, Shape: app.ShapeFixed{}}}) {
		t.Errorf("File: keep に届いた値 = %#v, %v", l.kept, err)
	}
	if err := o.Shapes(ctx); err != nil || !reflect.DeepEqual(l.shapes, []app.Shape{app.ShapeCount{}, app.ShapeRatio{}, app.ShapeYes{}, app.CoreType{Name: "int"}}) {
		t.Errorf("Shapes: draw に届いた値 = %#v, %v", l.shapes, err)
	}
	var bad app.Bad
	if err := o.Fail(ctx); !errors.As(err, &bad) || bad.Level != (app.SeverityWarning{}) {
		t.Errorf("Fail() = %#v, want Bad{Level: SeverityWarning{}}", err)
	}
}

// リテラルの型のフィールド: レコードリテラルは、項目を書かず、読む式は、そのリテラル
func TestLiteralFields(t *testing.T) {
	ctx := context.Background()
	l := &leaves{}
	o := app.New(l, &moods{})
	if got, err := o.Core(ctx, "int"); err != nil || got != (app.CoreType{Name: "int"}) {
		t.Errorf("Core(int) = %#v, %v, want CoreType{Name: int}", got, err)
	}
	if got, err := o.Mark(ctx); err != nil || got != (app.Marker{}) {
		t.Errorf("Mark() = %#v, %v, want Marker{}", got, err)
	}
	if got, err := o.KindOf(ctx, app.CoreType{Name: "x"}); err != nil || got != "core" {
		t.Errorf("KindOf() = %q, %v, want core", got, err)
	}
	if err := o.Version(ctx, app.Marker{}); err != nil || !reflect.DeepEqual(l.counts, []int64{2}) {
		t.Errorf("Version: count に届いた値 = %v, %v, want [2]", l.counts, err)
	}
}

// variant の match: リテラルの型のアームごとに分かれ、アームの中の対象は、そのリテラル。union の値がない（nil）ときは、分類の誤り
func TestVariant(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		level app.Severity
		want  string
	}{
		{app.SeverityError{}, "error"},
		{app.SeverityWarning{}, "loud"},
		{app.SeverityInfo{}, "info"},
	} {
		l := &leaves{}
		if err := app.New(l, &moods{}).Speak(ctx, app.Report{Severity: tt.level}); err != nil || !reflect.DeepEqual(l.said, []string{tt.want}) {
			t.Errorf("Speak(%#v): say に届いた値 = %v, %v, want [%s]", tt.level, l.said, err, tt.want)
		}
	}
	classificationError(t, app.New(&leaves{}, &moods{}).Speak(ctx, app.Report{}), "app.Level")
	for _, tt := range []struct {
		typ  app.Type
		want string
	}{
		{app.CoreType{Name: "int"}, "int"},
		{app.ListType{Of: "int"}, "list"},
	} {
		l := &leaves{}
		if err := app.New(l, &moods{}).Name(ctx, tt.typ); err != nil || !reflect.DeepEqual(l.said, []string{tt.want}) {
			t.Errorf("Name(%#v): say に届いた値 = %v, %v, want [%s]", tt.typ, l.said, err, tt.want)
		}
	}
	for _, tt := range []struct {
		shape app.Shape
		want  string
	}{
		{app.CoreType{}, "page"},
		{app.ShapeFixed{}, "fixed"},
		{app.ShapeCount{}, "number"},
		{app.ShapeRatio{}, "number"},
		{app.ShapeYes{}, "number"},
	} {
		if got, err := app.New(&leaves{}, &moods{}).Form(ctx, tt.shape); err != nil || got != tt.want {
			t.Errorf("Form(%#v) = %q, %v, want %s", tt.shape, got, err, tt.want)
		}
	}
}

// 三つに分ける形: 真、偽、値がない、が、それぞれのケースのアームに届く
func TestThree(t *testing.T) {
	ctx := context.Background()
	flags := []*bool{ptr(true), ptr(false), nil}
	o := app.New(&leaves{}, &moods{})
	for i, want := range []string{"on", "off", "unset"} {
		if got, err := o.Flag(ctx, app.Report{Flag: flags[i]}); err != nil || got != want {
			t.Errorf("Flag(%d) = %q, %v, want %s", i, got, err, want)
		}
	}
	for i, want := range []string{"kept", "off", "kept"} {
		if got, err := o.Apart(ctx, app.Report{Flag: flags[i]}); err != nil || got != want {
			t.Errorf("Apart(%d) = %q, %v, want %s", i, got, err, want)
		}
	}
	for i, want := range []string{"on", "rest", "rest"} {
		l := &leaves{}
		if err := app.New(l, &moods{}).Merged(ctx, app.Report{Flag: flags[i]}); err != nil || !reflect.DeepEqual(l.said, []string{want}) {
			t.Errorf("Merged(%d): say に届いた値 = %v, %v, want [%s]", i, l.said, err, want)
		}
		l = &leaves{}
		if err := app.New(l, &moods{}).All(ctx, app.Report{Flag: flags[i]}); err != nil || !reflect.DeepEqual(l.said, []string{"any"}) {
			t.Errorf("All(%d): say に届いた値 = %v, %v, want [any]", i, l.said, err)
		}
	}
	// 値がない側にケースがない分類: 値がなければ、分類の誤り
	for i, want := range []string{"yes", "no"} {
		if got, err := o.Strict(ctx, app.Report{Flag: flags[i]}); err != nil || got != want {
			t.Errorf("Strict(%d) = %q, %v, want %s", i, got, err, want)
		}
	}
	_, err := o.Strict(ctx, app.Report{})
	classificationError(t, err, "app.Strict")
	// 残りのケースが、偽と、値がない、の両方。as true のアームの中の対象は、真
	l := &leaves{}
	if err := app.New(l, &moods{}).Lax(ctx, ptr(true)); err != nil || !reflect.DeepEqual(l.shown, []bool{true}) || len(l.said) != 0 {
		t.Errorf("Lax(true): show %v、say %v, %v, want show [true]", l.shown, l.said, err)
	}
	for _, flag := range []*bool{ptr(false), nil} {
		l := &leaves{}
		if err := app.New(l, &moods{}).Lax(ctx, flag); err != nil || !reflect.DeepEqual(l.said, []string{"other"}) || len(l.shown) != 0 {
			t.Errorf("Lax(%v): show %v、say %v, %v, want say [other]", flag, l.shown, l.said, err)
		}
	}
}

// 述語で決まる分類: as true のケースの述語は、真の値でだけ、as false のケースの述語は、偽の値でだけ、呼ばれる。値がなければ、
// どの述語も呼ばれず、分類の誤り
func TestMood(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		flag   *bool
		glad   bool
		want   string
		called []string
	}{
		{ptr(true), true, "glad", []string{"Glad", "Calm"}},
		{ptr(true), false, "calm", []string{"Glad", "Calm"}},
		{ptr(false), true, "sad", []string{"Sad"}},
	} {
		c := &moods{glad: tt.glad}
		got, err := app.New(&leaves{}, c).Mood(ctx, app.Report{Flag: tt.flag})
		if err != nil || got != tt.want || !reflect.DeepEqual(c.called, tt.called) {
			t.Errorf("Mood(%v, glad=%v) = %q, %v（呼ばれた述語 %v）, want %s（%v）", *tt.flag, tt.glad, got, err, c.called, tt.want, tt.called)
		}
	}
	c := &moods{}
	_, err := app.New(&leaves{}, c).Mood(ctx, app.Report{})
	classificationError(t, err, "app.Mood")
	if len(c.called) != 0 {
		t.Errorf("値がないのに、述語が呼ばれた: %v", c.called)
	}
}
