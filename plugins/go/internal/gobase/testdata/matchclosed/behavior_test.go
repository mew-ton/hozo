// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 残りのケースを書かない、型で決まる分類
// （presence）の match は、値がないとき、return / const / 文のどの形でも、分類の誤り（*runtime.ClassificationError）を返す
// （docs/plugin-go.md「match」の「分類の誤りの枝」）。else のアームは、受けるケースを名前で並べたアームと同じに動く。
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

type leaves struct{ shown []string }

func (*leaves) FindUser(_ context.Context, name string) (*app.User, error) {
	if name == "" {
		return nil, nil
	}
	return &app.User{Name: name}, nil
}
func (*leaves) NameOf(_ context.Context, user app.User) (string, error) { return user.Name, nil }
func (*leaves) UseToken(_ context.Context, token app.Token) (string, error) {
	return "token " + string(token), nil
}
func (l *leaves) Show(_ context.Context, text string) error {
	l.shown = append(l.shown, text)
	return nil
}

// tokens は TokenState の実装。
type tokens struct{ value *app.Token }

func (s tokens) IsEmpty(context.Context) (bool, error)   { return s.value == nil, nil }
func (s tokens) Get(context.Context) (*app.Token, error) { return s.value, nil }

func classificationError(t *testing.T, err error, classify string) {
	t.Helper()
	var ce *runtime.ClassificationError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v（*runtime.ClassificationError でない）", err)
	}
	if ce.Classify != classify || len(ce.Hits) != 0 {
		t.Errorf("ClassificationError = %+v, want Classify=%s Hits=[]", ce, classify)
	}
}

// 残りのケースを書かない presence の分類: 値があれば、そのケースのアーム。なければ、分類の誤り
func TestPartialPresence(t *testing.T) {
	ctx := context.Background()
	l := &leaves{}
	held := app.Token("t")
	o := app.New(l, tokens{value: &held})

	if got, err := o.ArrivedName(ctx, "ann"); err != nil || got != "ann" {
		t.Errorf("ArrivedName(ann) = %q, %v", got, err)
	}
	got, err := o.ArrivedName(ctx, "")
	if got != "" {
		t.Errorf("ArrivedName(\"\") = %q（期待は零値）", got)
	}
	classificationError(t, err, "app.Arrival")

	// const の match と、文の match（本体が呼び出しのアーム、none のアーム）
	if got, err := o.ArrivedForms(ctx, "ann"); err != nil || got != "ann" || !reflect.DeepEqual(l.shown, []string{"ann"}) {
		t.Errorf("ArrivedForms(ann) = %q, %v, shown = %v", got, err, l.shown)
	}
	l.shown = nil
	got, err = o.ArrivedForms(ctx, "")
	if got != "" || len(l.shown) != 0 {
		t.Errorf("ArrivedForms(\"\") = %q, shown = %v（期待は、零値と、何も呼ばないこと）", got, l.shown)
	}
	classificationError(t, err, "app.Arrival")

	// 状態の分類
	if got, err := o.HeldText(ctx); err != nil || got != "token t" {
		t.Errorf("HeldText = %q, %v", got, err)
	}
	empty := app.New(l, tokens{})
	got, err = empty.HeldText(ctx)
	if got != "" {
		t.Errorf("HeldText（トークンなし） = %q（期待は零値）", got)
	}
	classificationError(t, err, "app.Held")
}

// else のアーム: 受けるケースを名前で並べたアームと同じに動く
func TestElseArm(t *testing.T) {
	ctx := context.Background()
	l := &leaves{}
	o := app.New(l, tokens{})
	ann := &app.User{Name: "ann"}

	if got, err := o.NameOrDeny(ctx, ann); err != nil || got != "ann" {
		t.Errorf("NameOrDeny(ann) = %q, %v", got, err)
	}
	if _, err := o.NameOrDeny(ctx, nil); !errors.Is(err, app.Denied{}) {
		t.Errorf("NameOrDeny(nil) の err = %v, want app.Denied", err)
	}
	if got, err := o.NameOrNobody(ctx, ann); err != nil || got != "ann" {
		t.Errorf("NameOrNobody(ann) = %q, %v", got, err)
	}
	if got, err := o.NameOrNobody(ctx, nil); err != nil || got != "nobody" {
		t.Errorf("NameOrNobody(nil) = %q, %v", got, err)
	}
	if got, err := o.Standard(ctx, ann, []app.Line{{Item: "x"}}); err != nil || got != "ann" {
		t.Errorf("Standard(ann, 1 行) = %q, %v", got, err)
	}
	if got, err := o.Standard(ctx, nil, nil); err != nil || got != "nobody" {
		t.Errorf("Standard(nil, 0 行) = %q, %v", got, err)
	}
	if !reflect.DeepEqual(l.shown, []string{"some", "none"}) {
		t.Errorf("shown = %v", l.shown)
	}
}
