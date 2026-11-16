// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 状態を通る配線
// （docs/plugin-go.md「状態の単位」「状態の参照」）。plugins/go の TestBehavior が、生成したコードのモジュールのルートに
// 置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/app"
)

// world は、状態の単位と葉の実装。呼ばれた順を log に残す。
type world struct {
	token    *app.Token
	lines    []app.Line
	settings app.Settings
	log      []string
	getErr   error
}

func (w *world) note(s string) { w.log = append(w.log, s) }

type tokenState struct{ w *world }

func (s tokenState) IsEmpty(context.Context) (bool, error) { return s.w.token == nil, nil }
func (s tokenState) Get(context.Context) (*app.Token, error) {
	s.w.note("token.Get")
	return s.w.token, s.w.getErr
}
func (s tokenState) Logout(context.Context) error {
	s.w.note("token.Logout")
	s.w.token = nil
	return nil
}

type linesState struct{ w *world }

func (s linesState) IsEmpty(context.Context) (bool, error) { return len(s.w.lines) == 0, nil }
func (s linesState) Get(context.Context) ([]app.Line, error) {
	s.w.note("lines.Get")
	return s.w.lines, nil
}
func (s linesState) CountLines(context.Context) (int64, error) {
	s.w.note("lines.CountLines")
	return int64(len(s.w.lines)), nil
}
func (s linesState) Add(_ context.Context, line app.Line) error {
	s.w.note("lines.Add " + line.Item)
	s.w.lines = append(s.w.lines, line)
	return nil
}

type settingsState struct{ w *world }

func (s settingsState) Get(context.Context) (app.Settings, error) {
	s.w.note("settings.Get")
	return s.w.settings, nil
}

type shared struct{ w *world }

func (s shared) Summary(context.Context) (string, error) {
	s.w.note("shared.Summary")
	return s.w.settings.Title, nil
}

type leaves struct{ w *world }

func (l leaves) Show(title string) { l.w.note("show " + title) }
func (l leaves) ShowOwner(name *string) {
	if name == nil {
		l.w.note("owner none")
	} else {
		l.w.note("owner " + *name)
	}
}
func (l leaves) ShowToken(token *app.Token) {
	if token == nil {
		l.w.note("token none")
	} else {
		l.w.note("token " + string(*token))
	}
}
func (l leaves) ShowBoth(a, b *app.Token) { l.w.note("both") }
func (l leaves) ShowLines(lines []app.Line) {
	l.w.note("lines")
}

func newOps(w *world) app.Ops {
	return app.New(leaves{w}, tokenState{w}, linesState{w}, settingsState{w}, shared{w})
}

func expectLog(t *testing.T, w *world, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(w.log, want) {
		t.Errorf("呼ばれた順:\n  got  %q\n  want %q", w.log, want)
	}
	w.log = nil
}

func TestStateReads(t *testing.T) {
	ctx := context.Background()
	tok := app.Token("t1")
	w := &world{token: &tok, settings: app.Settings{Title: "shop", Owner: &app.Owner{Name: "o"}}}
	o := newOps(w)

	// 文ごとに読む（別の文なら、もう一度読む）。省略可能を通るパス
	if err := o.Header(ctx); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "settings.Get", "show shop", "settings.Get", "owner o")

	// 一つの文の中の同じ状態は、一度だけ読む。効果を持つ葉は、状態の単位の命令
	if err := o.Once(ctx); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "token.Get", "both", "settings.Get", "lines.Add shop", "token.Get", "token t1")

	// match の対象が状態: 先に一度読む。アームの本体だけが使う状態は、その枝の中で読む
	if err := o.Branches(ctx); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "lines.Get", "lines", "token.Get", "token.Logout")
	w.lines = nil
	if err := o.Branches(ctx); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "lines.Get", "settings.Get", "show shop", "token.Get", "token none")

	// 状態を読む葉は、状態の単位の問い合わせ（reads が一つ）と SharedState（二つ以上）
	got, err := o.Report(ctx, app.Line{Item: "pen", Count: 1})
	if err != nil || got != "shop" {
		t.Fatalf("Report = %q, %v", got, err)
	}
	expectLog(t, w, "lines.Add pen", "lines.CountLines", "shared.Summary")
}

// 状態を読めなければ、戻り値の型の零値と、そのエラーを返す
func TestStateReadFails(t *testing.T) {
	boom := errors.New("boom")
	w := &world{getErr: boom}
	got, err := newOps(w).Current(context.Background())
	if got != nil || err != boom {
		t.Errorf("Current = %v, %v", got, err)
	}
	expectLog(t, w, "token.Get")
}
