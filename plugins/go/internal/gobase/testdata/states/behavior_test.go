// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 状態を通る配線と、葉の op の置き場
// （docs/plugin-go.md「op の置き場」「状態の単位」「状態の参照」）。plugins/go の TestBehavior が、生成したコードのモジュールの
// ルートに置いて走らせる。
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
	stale    bool
	log      []string
	getErr   error
	// checkoutErr は、SharedState の命令 Checkout が返すエラー
	checkoutErr error
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
func (s tokenState) Peek(context.Context) (*app.Token, error) {
	s.w.note("token.Peek")
	return s.w.token, nil
}
func (s tokenState) Enter(_ context.Context, token app.Token) error {
	s.w.note("token.Enter " + string(token))
	s.w.token = &token
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
func (s linesState) Clear(context.Context) error {
	s.w.note("lines.Clear")
	s.w.lines = nil
	return nil
}
func (s linesState) AddCounted(_ context.Context, line app.Line) (int64, error) {
	s.w.note("lines.AddCounted " + line.Item)
	s.w.lines = append(s.w.lines, line)
	return int64(len(s.w.lines)), nil
}
func (s linesState) Push(_ context.Context, line app.Line) error {
	s.w.note("lines.Push " + line.Item)
	s.w.lines = append(s.w.lines, line)
	return nil
}
func (s linesState) Replace(_ context.Context, line app.Line) error {
	s.w.note("lines.Replace " + line.Item)
	s.w.lines = []app.Line{line}
	return nil
}

type settingsState struct{ w *world }

func (s settingsState) Get(context.Context) (app.Settings, error) {
	s.w.note("settings.Get")
	return s.w.settings, nil
}

// syncState は、述語で決まる状態の分類 app.Sync の単位: ケースごとの述語と、その分類の遷移を宛先にする効果を持つ葉。
type syncState struct{ w *world }

func (s syncState) Fresh(context.Context) (bool, error) { return !s.w.stale, nil }
func (s syncState) Stale(context.Context) (bool, error) { return s.w.stale, nil }
func (s syncState) Sync(context.Context) error {
	s.w.note("sync.Sync")
	s.w.stale = false
	return nil
}

// shared は SharedState: 複数の状態を読む葉（問い合わせ）と、効果の宛先が一つの状態の単位に収まらない葉（命令）。
type shared struct{ w *world }

func (s shared) Summary(context.Context) (string, error) {
	s.w.note("shared.Summary")
	return s.w.settings.Title, nil
}

// Checkout は、二つの状態（lines と token）に効果を持つ葉。どちらも行うか、どちらも行わないかは、実装（状態の持ち主）が決める
func (s shared) Checkout(_ context.Context, line app.Line) error {
	s.w.note("shared.Checkout " + line.Item)
	if s.w.checkoutErr != nil {
		return s.w.checkoutErr
	}
	s.w.lines = append(s.w.lines, line)
	s.w.token = nil
	return nil
}
func (s shared) SyncOut(context.Context) error {
	s.w.note("shared.SyncOut")
	s.w.stale, s.w.token = false, nil
	return nil
}
func (s shared) Status(context.Context) (string, error) {
	s.w.note("shared.Status")
	if s.w.token == nil {
		return "out", nil
	}
	return "in", nil
}
func (s shared) LineFor(_ context.Context, item string) (*app.Line, error) {
	s.w.note("shared.LineFor " + item)
	for i := range s.w.lines {
		if s.w.lines[i].Item == item {
			return &s.w.lines[i], nil
		}
	}
	return nil, nil
}

type leaves struct{ w *world }

// Audit は、運ぶ節のないデコレータだけを持つ葉: 純粋でない（ctx を取り、error を返す）が、状態には触れないので Leaves
func (l leaves) Audit(_ context.Context, note string) error {
	l.w.note("audit " + note)
	return nil
}
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
	// 引数の並びは、leaves、状態の単位（宣言順: 基底の状態と、述語で決まる状態の分類）、shared
	return app.New(leaves{w}, tokenState{w}, linesState{w}, settingsState{w}, syncState{w}, shared{w})
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

// 葉の op の置き場（docs/plugin-go.md「op の置き場」の 3〜6）: 効果の宛先の単位が一つなら、その状態の単位の命令。二つ以上に
// 分かれるなら SharedState。op が書いた効果か、デコレータが運んだ効果かは、置き場を変えない。読む状態も同じ
func TestLeafPlaces(t *testing.T) {
	ctx := context.Background()
	tok := app.Token("t1")
	w := &world{token: &tok, stale: true}
	o := newOps(w)
	pen, ink := app.Line{Item: "pen", Count: 1}, app.Line{Item: "ink", Count: 2}

	// 宛先の単位が一つ: 基底の状態の遷移、述語で決まる状態の分類の遷移、型で決まる状態の分類の遷移（ただ一つの対象の状態）
	if err := o.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if err := o.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := o.Enter(ctx, "t2"); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "lines.Clear", "sync.Sync", "token.Enter t2")
	// 効果と読む状態の両方を持つ葉は、効果だけで決める
	if n, err := o.AddCounted(ctx, pen); err != nil || n != 1 {
		t.Fatalf("AddCounted = %d, %v", n, err)
	}
	expectLog(t, w, "lines.AddCounted pen")

	// デコレータが運ぶ効果: 一つ、二つで宛先の単位が同じ → その状態の単位
	if err := o.Push(ctx, ink); err != nil {
		t.Fatal(err)
	}
	if err := o.Replace(ctx, pen); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "lines.Push ink", "lines.Replace pen")

	// 二つの効果の宛先の単位が分かれる葉（lines と token）は SharedState の命令
	if err := o.Checkout(ctx, ink); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "shared.Checkout ink")
	if len(w.lines) != 2 || w.token != nil {
		t.Errorf("Checkout の後: lines = %v, token = %v", w.lines, w.token)
	}
	// op が書いた効果（分類 Sync の遷移）と、運ばれた効果（token）で分かれても SharedState
	w.stale = true
	if err := o.SyncOut(ctx); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "shared.SyncOut")

	// デコレータが運ぶ reads: 一つならその状態の単位の問い合わせ、op が書いたものと合わせて二つ以上なら SharedState
	if got, err := o.Peek(ctx); err != nil || got != nil {
		t.Fatalf("Peek = %v, %v", got, err)
	}
	if got, err := o.Status(ctx); err != nil || got != "out" {
		t.Fatalf("Status = %q, %v", got, err)
	}
	if got, err := o.LineFor(ctx, "ink"); err != nil || got == nil || got.Count != 2 {
		t.Fatalf("LineFor = %v, %v", got, err)
	}
	expectLog(t, w, "token.Peek", "shared.Status", "shared.LineFor ink")

	// 運ぶ節のないデコレータだけを持つ葉は Leaves
	if err := o.Audit(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "audit x")
}

// 配線は、SharedState の葉も、状態の単位の葉と同じに呼ぶ（o.<Op>）。SharedState の命令が失敗すれば、配線はそこで止まり、
// 戻り値の型の零値と、そのエラーを返す
func TestSharedInWiring(t *testing.T) {
	ctx := context.Background()
	tok := app.Token("t1")
	w := &world{token: &tok, stale: true}
	o := newOps(w)
	got, err := o.Finish(ctx, app.Line{Item: "pen", Count: 1})
	if err != nil || got != "out" {
		t.Fatalf("Finish = %q, %v", got, err)
	}
	expectLog(t, w, "lines.Replace pen", "shared.Checkout pen", "sync.Sync", "audit finished", "shared.Status")
	if w.stale {
		t.Error("分類の単位の命令 Sync が、状態を変えていない")
	}

	boom := errors.New("boom")
	w.checkoutErr = boom
	got, err = o.Finish(ctx, app.Line{Item: "ink", Count: 1})
	if got != "" || err != boom {
		t.Errorf("Finish = %q, %v, want \"\", boom", got, err)
	}
	expectLog(t, w, "lines.Replace ink", "shared.Checkout ink")
}

// 述語で決まる状態の分類は、配線が match に使っていなくても、判定関数を出す（テストが、効果の結果の判定に使う）
func TestClassifySync(t *testing.T) {
	ctx := context.Background()
	w := &world{stale: true}
	o := newOps(w)
	if k, err := app.ClassifySync(ctx, syncState{w}); err != nil || k != app.SyncStale {
		t.Fatalf("ClassifySync = %v, %v, want SyncStale", k, err)
	}
	if err := o.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if k, err := app.ClassifySync(ctx, syncState{w}); err != nil || k != app.SyncFresh {
		t.Fatalf("ClassifySync = %v, %v, want SyncFresh", k, err)
	}
}
