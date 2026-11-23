// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: union の位置に、アームの型の値を入れる配線
// （docs/plugin-go.md「値」の「union に入れる」）。構造体のアームは、その構造体の値のまま、包む型のアームは、包む型の値で、
// 呼び先と、呼び元に届くこと。
// plugins/go の TestBehavior が、生成したコードのモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/app"
)

// leaves は、葉の op の実装。keep と settle に届いた値を覚える。
type leaves struct {
	kept    [][2]app.Pet
	settled []app.Home
}

func (l *leaves) FindDog(context.Context) (app.Dog, error)    { return app.Dog{Name: "found"}, nil }
func (l *leaves) FindTag(context.Context) (app.Tag, error)    { return app.Tag("t"), nil }
func (l *leaves) FindNames(context.Context) ([]string, error) { return []string{"a", "b"}, nil }
func (l *leaves) Settle(_ context.Context, home app.Home) (int64, error) {
	l.settled = append(l.settled, home)
	return int64(len(l.settled)), nil
}

func (l *leaves) Keep(_ context.Context, pet app.Pet, spare app.Pet) error {
	l.kept = append(l.kept, [2]app.Pet{pet, spare})
	return nil
}

func TestArgs(t *testing.T) {
	l := &leaves{}
	dog := app.Dog{Name: "hachi"}
	err := app.New(l).Args(context.Background(), dog, app.Tag("x"))
	want := [][2]app.Pet{
		{dog, app.PetCount{Value: 2}},
		{app.PetLabel{Value: "x"}, app.Cat{Lives: 9}},
		{app.Dog{Name: "pochi"}, dog},
	}
	if err != nil || !reflect.DeepEqual(l.kept, want) {
		t.Errorf("Args = %v（keep に届いた値 %#v）, want %#v", err, l.kept, want)
	}
}

func TestFields(t *testing.T) {
	l := &leaves{}
	dog := app.Dog{Name: "hachi"}
	_, err := app.New(l).Fields(context.Background(), dog, []string{"a"})
	want := []app.Home{{Pet: app.PetNames{Value: []string{"a"}}, Spare: dog}}
	if err != nil || !reflect.DeepEqual(l.settled, want) {
		t.Errorf("Fields: %v（settle に届いた値 %#v）, want %#v", err, l.settled, want)
	}
}

func TestReturn(t *testing.T) {
	o := app.New(&leaves{})
	ctx := context.Background()
	given := app.Dog{Name: "given"}
	for _, tt := range []struct {
		name string
		call func() (app.Pet, error)
		want app.Pet
	}{
		{"value", func() (app.Pet, error) { return o.Value(ctx) }, app.PetCount{Value: 3}},
		{"called", func() (app.Pet, error) { return o.Called(ctx) }, app.Dog{Name: "found"}},
		{"tagged", func() (app.Pet, error) { return o.Tagged(ctx) }, app.PetLabel{Value: "t"}},
		{"matched（値がある）", func() (app.Pet, error) { return o.Matched(ctx, &given) }, given},
		{"matched（値がない）", func() (app.Pet, error) { return o.Matched(ctx, nil) }, app.Dog{Name: "found"}},
	} {
		got, err := tt.call()
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s = %#v, %v, want %#v", tt.name, got, err, tt.want)
		}
	}
}

func TestStmts(t *testing.T) {
	l := &leaves{}
	dog := app.Dog{Name: "hachi"}
	got, err := app.New(l).Stmts(context.Background(), dog)
	found := app.Dog{Name: "found"}
	want := []app.Pet{found, app.PetCount{Value: 2}, found, app.PetCount{Value: 2}}
	kept := [][2]app.Pet{{dog, app.PetLabel{Value: "t"}}, {app.PetCount{Value: 1}, nil}}
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(l.kept, kept) {
		t.Errorf("Stmts = %#v, %v（keep に届いた値 %#v）, want %#v（%#v）", got, err, l.kept, want, kept)
	}
}

func TestLose(t *testing.T) {
	o := app.New(&leaves{})
	dog := app.Dog{Name: "hachi"}
	_, err := o.Lose(context.Background(), &dog)
	var lost app.Lost
	if !errors.As(err, &lost) || !reflect.DeepEqual(lost.Pet, app.Pet(dog)) {
		t.Errorf("Lose(値がある) の失敗 = %#v, want Lost{Pet: %#v}", err, dog)
	}
	if got, err := o.Lose(context.Background(), nil); err != nil || got != 0 {
		t.Errorf("Lose(値がない) = %d, %v, want 0", got, err)
	}
}
