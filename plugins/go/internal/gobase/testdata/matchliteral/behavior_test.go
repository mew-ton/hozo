// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 絞り込みが literal の、型で決まる分類
// （plugins/go/spec/go.hozo の Requirement.Wiring の「match」）で分ける配線。真と偽が、それぞれのケースのアームに届くことと、ケースのない側の値が、
// 分類の誤り（*runtime.ClassificationError）になること。述語で決まる分類（同「分類」）では、as の型に当たらない値で、
// そのケースの述語が呼ばれないこと。
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

// leaves は、葉の op の実装。呼ばれた op の名前と、show に届いた文字列を覚える。
type leaves struct{ called, shown []string }

func (l *leaves) Plan(_ context.Context, _ app.Options) (string, error) {
	l.called = append(l.called, "plan")
	return "planned", nil
}

func (l *leaves) Write(_ context.Context, _ app.Options) (string, error) {
	l.called = append(l.called, "write")
	return "written", nil
}

func (l *leaves) Show(_ context.Context, text string) error {
	l.shown = append(l.shown, text)
	return nil
}

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

// 真は as true のケースのアーム、偽は as false のケースのアーム
func TestRun(t *testing.T) {
	for _, tt := range []struct {
		dry    bool
		want   string
		called []string
	}{
		{true, "planned", []string{"plan"}},
		{false, "written", []string{"write"}},
	} {
		l := &leaves{}
		got, err := app.New(l, &speeds{}).Run(context.Background(), app.Options{DryRun: tt.dry})
		if err != nil || got != tt.want || !reflect.DeepEqual(l.called, tt.called) {
			t.Errorf("Run(dryRun=%v) = %q, %v（呼ばれた op %v）, want %q（%v）", tt.dry, got, err, l.called, tt.want, tt.called)
		}
	}
}

// 偽の側が、残りのケース
func TestLabel(t *testing.T) {
	for verbose, want := range map[bool]string{true: "loud", false: "quiet"} {
		got, err := app.New(&leaves{}, &speeds{}).Label(context.Background(), app.Options{Verbose: verbose})
		if err != nil || got != want {
			t.Errorf("Label(verbose=%v) = %q, %v, want %q", verbose, got, err, want)
		}
	}
}

// ケースのない側の値は、分類の誤り。アームの本体は、走らない
func TestCheck(t *testing.T) {
	l := &leaves{}
	if err := app.New(l, &speeds{}).Check(context.Background(), app.Options{DryRun: true}); err != nil || !reflect.DeepEqual(l.shown, []string{"dry"}) {
		t.Errorf("Check(dryRun=true) = %v（show %v）, want nil（[dry]）", err, l.shown)
	}
	l = &leaves{}
	classificationError(t, app.New(l, &speeds{}).Check(context.Background(), app.Options{DryRun: false}), "app.Must")
	if len(l.shown) != 0 {
		t.Errorf("分類の誤りなのに、アームの本体が走った: %v", l.shown)
	}
}

func TestCheckNot(t *testing.T) {
	l := &leaves{}
	if err := app.New(l, &speeds{}).CheckNot(context.Background(), app.Options{DryRun: false}); err != nil || !reflect.DeepEqual(l.shown, []string{"wet"}) {
		t.Errorf("CheckNot(dryRun=false) = %v（show %v）, want nil（[wet]）", err, l.shown)
	}
	l = &leaves{}
	classificationError(t, app.New(l, &speeds{}).CheckNot(context.Background(), app.Options{DryRun: true}), "app.MustNot")
	if len(l.shown) != 0 {
		t.Errorf("分類の誤りなのに、アームの本体が走った: %v", l.shown)
	}
}

// 一つのアームが両方のケースを受ける
func TestBoth(t *testing.T) {
	for _, dry := range []bool{true, false} {
		got, err := app.New(&leaves{}, &speeds{}).Both(context.Background(), app.Options{DryRun: dry})
		if err != nil || got != "planned" {
			t.Errorf("Both(dryRun=%v) = %q, %v, want planned", dry, got, err)
		}
	}
}

// speeds は SpeedCases の実装。呼ばれた述語を覚える。fast が真なら Fast、偽なら Careful（as true の二つのケースを、述語が分ける）。
type speeds struct {
	fast   bool
	called []string
}

func (c *speeds) Fast(_ context.Context, _ bool) (bool, error) {
	c.called = append(c.called, "Fast")
	return c.fast, nil
}

func (c *speeds) Careful(_ context.Context, _ bool) (bool, error) {
	c.called = append(c.called, "Careful")
	return !c.fast, nil
}

func (c *speeds) Off(_ context.Context, _ bool) (bool, error) {
	c.called = append(c.called, "Off")
	return true, nil
}

// as true のケースの述語は、真の値でだけ、as false のケースの述語は、偽の値でだけ、呼ばれる
func TestPace(t *testing.T) {
	for _, tt := range []struct {
		verbose, fast bool
		want          string
		called        []string
	}{
		{true, true, "fast", []string{"Fast", "Careful"}},
		{true, false, "careful", []string{"Fast", "Careful"}},
		{false, true, "off", []string{"Off"}},
	} {
		c := &speeds{fast: tt.fast}
		got, err := app.New(&leaves{}, c).Pace(context.Background(), app.Options{Verbose: tt.verbose})
		if err != nil || got != tt.want || !reflect.DeepEqual(c.called, tt.called) {
			t.Errorf("Pace(verbose=%v) = %q, %v（呼ばれた述語 %v）, want %q（%v）", tt.verbose, got, err, c.called, tt.want, tt.called)
		}
	}
}
