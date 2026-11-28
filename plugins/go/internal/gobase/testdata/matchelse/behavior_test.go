// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 残りのケース（対象 as else。
// docs/check.md「残りのケース」）を持つ、型で決まる分類で分ける配線（plugins/go/spec/go.hozo の Requirement.Wiring の「match」）。presence は、値がなければ
// 残りのケースの枝。variant は、残りのアームなら残りのケースの節。plugins/go の TestBehavior が、生成したコードのモジュールの
// ルートに置いて走らせる。
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
	if name == "nobody" {
		return nil, nil
	}
	return &app.User{Name: name}, nil
}
func (*leaves) NameOf(_ context.Context, user app.User) (string, error) {
	return "user " + user.Name, nil
}
func (*leaves) CardText(_ context.Context, card app.Card) (string, error) {
	return "card " + card.Number, nil
}
func (*leaves) Describe(_ context.Context, payment app.Payment) (string, error) {
	switch p := payment.(type) {
	case app.Bank:
		return "bank " + p.Account, nil
	case app.PaymentByCode:
		return "code " + p.Value, nil
	}
	return "", errors.New("残りのアームでない値が、残りのケースの節に来た")
}
func (*leaves) Shout(_ context.Context, text string) (string, error) { return text + "!", nil }
func (l *leaves) Show(_ context.Context, text string) error {
	l.shown = append(l.shown, text)
	return nil
}

type session struct{ user *app.User }

func (s session) IsEmpty(context.Context) (bool, error)  { return s.user == nil, nil }
func (s session) Get(context.Context) (*app.User, error) { return s.user, nil }

type stage struct{ result app.Result }

func (s stage) Get(context.Context) (app.Result, error) { return s.result, nil }

func newOps(l *leaves, se session, st stage) app.Ops { return app.New(l, se, st) }

// presence の return match: 値があれば型を書いたケース、なければ残りのケース（throw）
func TestPresenceReturn(t *testing.T) {
	o := newOps(&leaves{}, session{}, stage{})
	if got, err := o.Login(context.Background(), "ann"); err != nil || got != "user ann" {
		t.Errorf("Registered: %q, %v", got, err)
	}
	if _, err := o.Login(context.Background(), "nobody"); err != (app.Unknown{}) {
		t.Errorf("Unregistered: err = %v, want app.Unknown", err)
	}
}

// presence の const と文: 残りのケースのアームの値、none のアーム
func TestPresenceConstAndStatement(t *testing.T) {
	ctx := context.Background()
	title := "Dr"
	l := &leaves{}
	if got, err := newOps(l, session{}, stage{}).Greet(ctx, "ann", &title); err != nil || got != "user ann" {
		t.Errorf("値がある: %q, %v", got, err)
	}
	if !reflect.DeepEqual(l.shown, []string{"Dr"}) {
		t.Errorf("値がある: shown = %v（Filled のアームだけが呼ぶ）", l.shown)
	}
	l = &leaves{}
	if got, err := newOps(l, session{}, stage{}).Greet(ctx, "nobody", nil); err != nil || got != "guest" {
		t.Errorf("値がない: %q, %v", got, err)
	}
	if !reflect.DeepEqual(l.shown, []string{"no title"}) {
		t.Errorf("値がない: shown = %v（Blank のアームだけが呼ぶ）", l.shown)
	}
	// 両方のケースを並べたアームは、分けない
	if got, err := newOps(&leaves{}, session{}, stage{}).Both(ctx, nil); err != nil || got != "any" {
		t.Errorf("Both(nil): %q, %v", got, err)
	}
}

// 型で決まる状態の分類（presence）
func TestPresenceState(t *testing.T) {
	ctx := context.Background()
	if got, err := newOps(&leaves{}, session{user: &app.User{Name: "ann"}}, stage{}).Current(ctx); err != nil || got != "user ann" {
		t.Errorf("LoggedIn: %q, %v", got, err)
	}
	if got, err := newOps(&leaves{}, session{}, stage{}).Current(ctx); err != nil || got != "nobody" {
		t.Errorf("LoggedOut: %q, %v", got, err)
	}
}

// variant: 残りのアーム（二つ）は、どちらも残りのケースの節に来る。型を書いたケースのアームは、その節
func TestVariantRemainingArms(t *testing.T) {
	ctx := context.Background()
	o := newOps(&leaves{}, session{}, stage{})
	for _, tt := range []struct {
		payment app.Payment
		want    string
	}{
		{app.Card{Number: "42"}, "card 42"},
		{app.Bank{Account: "7"}, "bank 7"},
		{app.PaymentByCode{Value: "xyz"}, "code xyz"},
	} {
		if got, err := o.Method(ctx, tt.payment); err != nil || got != tt.want {
			t.Errorf("Method(%#v) = %q, %v, want %q", tt.payment, got, err, tt.want)
		}
	}
	// union の値がない（nil）のは、どのアームでもない: 残りのケースにも当たらず、判定の誤りを返す
	_, err := o.Method(ctx, nil)
	var ce *runtime.ClassificationError
	if !errors.As(err, &ce) || ce.Classify != "app.Method" {
		t.Errorf("Method(nil): err = %v, want *runtime.ClassificationError{Classify: app.Method}", err)
	}
}

// variant で、残りのアームがちょうど一つ: 残りのケースのアームの中で、対象はそのアームの型の値
func TestVariantSingleRemainingArm(t *testing.T) {
	ctx := context.Background()
	o := newOps(&leaves{}, session{}, stage{})
	if got, err := o.Outcome(ctx, app.User{Name: "ann"}); err != nil || got != "user ann" {
		t.Errorf("Ok: %q, %v", got, err)
	}
	if got, err := o.Outcome(ctx, app.ResultFail{Value: "boom"}); err != nil || got != "boom!" {
		t.Errorf("Failed（包む型の Value）: %q, %v", got, err)
	}
	if got, err := o.Failure(ctx, app.ResultFail{Value: "boom"}); err != nil || got != "boom" {
		t.Errorf("Message: %q, %v", got, err)
	}
	if got, err := o.Failure(ctx, app.User{Name: "ann"}); err != nil || got != "ann" {
		t.Errorf("Rest（宣言した model のフィールド）: %q, %v", got, err)
	}
}

// variant の const と文
func TestVariantConstAndStatement(t *testing.T) {
	ctx := context.Background()
	l := &leaves{}
	got, err := newOps(l, session{}, stage{}).Variants(ctx, app.Card{Number: "42"}, app.User{})
	if err != nil || got == nil || *got != "card 42" {
		t.Errorf("Plastic: %v, %v", got, err)
	}
	if !reflect.DeepEqual(l.shown, []string{"done"}) {
		t.Errorf("Plastic: shown = %v", l.shown)
	}
	l = &leaves{}
	got, err = newOps(l, session{}, stage{}).Variants(ctx, app.Bank{}, app.ResultFail{})
	if err != nil || got != nil {
		t.Errorf("Other: %v, %v（none のアームは nil）", got, err)
	}
	if !reflect.DeepEqual(l.shown, []string{"done", "other"}) {
		t.Errorf("Other: shown = %v", l.shown)
	}
}

// 型で決まる状態の分類（variant）
func TestVariantState(t *testing.T) {
	ctx := context.Background()
	if got, err := newOps(&leaves{}, session{}, stage{result: app.User{Name: "ann"}}).StageText(ctx); err != nil || got != "user ann" {
		t.Errorf("Succeeded: %q, %v", got, err)
	}
	if got, err := newOps(&leaves{}, session{}, stage{result: app.ResultFail{Value: "wait"}}).StageText(ctx); err != nil || got != "wait!" {
		t.Errorf("NotYet: %q, %v", got, err)
	}
}
