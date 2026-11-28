// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 状態を通る配線と、read と効果と、直に呼ぶ
// 組み込みの効果を呼ぶ配線（呼ぶ先が、宛先の状態の単位のメソッドであること）、葉の op の置き場
// （plugins/go/spec/go.hozo の Requirement.Ports の「op の置き場」、Requirement.Ports の「状態の単位」、Requirement.Wiring の「呼び出し」、Requirement.Wiring の「状態の参照」）。plugins/go の TestBehavior が、生成したコードの
// モジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"example.com/app/gen/app"
)

// world は、状態の単位と葉の実装が共有するもの。呼ばれた順を log に残す。
type world struct {
	token    *app.Token
	lines    []app.Line
	settings app.Settings
	stale    bool
	log      []string
	getErr   error
	// addErr は、効果 addLine が返すエラー
	addErr error
}

func (w *world) note(s string) { w.log = append(w.log, s) }

// tokenState は、状態 app.token の単位: 形で決まる問い合わせ、形で決まる命令（配線が直に呼ぶ Post、Put、Delete）、read tokenCopy、
// 効果 dropToken と、型で決まる状態の分類 SignedIn の遷移を宛先にする効果 signIn。
type tokenState struct{ w *world }

func (s tokenState) IsEmpty(context.Context) (bool, error) { return s.w.token == nil, nil }
func (s tokenState) Get(context.Context) (*app.Token, error) {
	s.w.note("token.Get")
	return s.w.token, s.w.getErr
}
func (s tokenState) Post(_ context.Context, value app.Token) error {
	s.w.note("token.Post " + string(value))
	s.w.token = &value
	return nil
}
func (s tokenState) Put(_ context.Context, value app.Token) error {
	s.w.note("token.Put " + string(value))
	s.w.token = &value
	return nil
}
func (s tokenState) Delete(context.Context) error {
	s.w.note("token.Delete")
	s.w.token = nil
	return nil
}
func (s tokenState) TokenCopy(context.Context) (*app.Token, error) { return s.w.token, nil }
func (s tokenState) DropToken(context.Context) error {
	s.w.note("token.DropToken")
	s.w.token = nil
	return nil
}
func (s tokenState) SignIn(_ context.Context, token app.Token) error {
	s.w.note("token.SignIn " + string(token))
	s.w.token = &token
	return nil
}

// linesState は、状態 app.lines の単位。
type linesState struct{ w *world }

func (s linesState) IsEmpty(context.Context) (bool, error) { return len(s.w.lines) == 0, nil }
func (s linesState) Get(context.Context) ([]app.Line, error) {
	s.w.note("lines.Get")
	return s.w.lines, nil
}
func (s linesState) Post(_ context.Context, value app.Line) error {
	s.w.note("lines.Post " + value.Item)
	s.w.lines = append(s.w.lines, value)
	return nil
}
func (s linesState) LineFor(_ context.Context, item string) (*app.Line, error) {
	s.w.note("lines.LineFor " + item)
	for i := range s.w.lines {
		if s.w.lines[i].Item == item {
			return &s.w.lines[i], nil
		}
	}
	return nil, nil
}
func (s linesState) CountLines(context.Context) (int64, error) {
	s.w.note("lines.CountLines")
	return int64(len(s.w.lines)), nil
}
func (s linesState) LinesSince(context.Context, time.Time, *int64) ([]app.Line, error) {
	return s.w.lines, nil
}
func (s linesState) AddLine(_ context.Context, line app.Line) error {
	s.w.note("lines.AddLine " + line.Item)
	if s.w.addErr != nil {
		return s.w.addErr
	}
	s.w.lines = append(s.w.lines, line)
	return nil
}
func (s linesState) ClearLines(context.Context) error {
	s.w.note("lines.ClearLines")
	s.w.lines = nil
	return nil
}
func (s linesState) RemoveLine(context.Context, string) error { return nil }

// settingsState は、状態 app.settings の単位。
type settingsState struct{ w *world }

func (s settingsState) Get(context.Context) (app.Settings, error) {
	s.w.note("settings.Get")
	return s.w.settings, nil
}
func (s settingsState) Put(_ context.Context, value app.Settings) error {
	s.w.note("settings.Put " + value.Title)
	s.w.settings = value
	return nil
}
func (s settingsState) ShopTitle(context.Context) (string, error) {
	s.w.note("settings.ShopTitle")
	return s.w.settings.Title, nil
}
func (s settingsState) Rename(_ context.Context, title string, owner *app.Owner) error {
	if owner == nil {
		s.w.note("settings.Rename " + title + " by nobody")
	} else {
		s.w.note("settings.Rename " + title + " by " + owner.Name)
	}
	s.w.settings = app.Settings{Title: title, Owner: owner}
	return nil
}

// journalState は、read も効果もなく、どの配線も直に呼ばない状態の単位: 形で決まる問い合わせだけ。
type journalState struct{}

func (journalState) IsEmpty(context.Context) (bool, error) { return true, nil }
func (journalState) Get(context.Context) ([]string, error) { return nil, nil }

// syncState は、述語で決まる状態の分類 app.Sync の単位: ケースごとの述語と、その分類の遷移を宛先にする効果 refresh。
type syncState struct{ w *world }

func (s syncState) Fresh(context.Context) (bool, error) { return !s.w.stale, nil }
func (s syncState) Stale(context.Context) (bool, error) { return s.w.stale, nil }
func (s syncState) Refresh(context.Context) error {
	s.w.note("sync.Refresh")
	s.w.stale = false
	return nil
}

// leaves は、人が書く葉。デコレータが reads / effects を運ぶ葉（Push、Summary、Checkout、Peek）も、ここ: 状態の単位にも、
// 複数の状態にまたがる口にも置かれない（そういう口は、もうない）。
type leaves struct{ w *world }

func (l leaves) Push(_ context.Context, line app.Line) error {
	l.w.note("leaves.Push " + line.Item)
	return nil
}
func (l leaves) Summary(context.Context) (string, error) {
	l.w.note("leaves.Summary")
	return "summary", nil
}
func (l leaves) Checkout(_ context.Context, line app.Line) error {
	l.w.note("leaves.Checkout " + line.Item)
	return nil
}
func (l leaves) Peek(context.Context) (*app.Token, error) {
	l.w.note("leaves.Peek")
	return l.w.token, nil
}

// Audit は、運ぶ節のないデコレータだけを持つ葉
func (l leaves) Audit(_ context.Context, note string) error {
	l.w.note("audit " + note)
	return nil
}

// デコレータも throws もない葉（仕様の上で純粋）も、同じ形: ctx を取り、error を返す（plugins/go/spec/go.hozo の Requirement.OpShape）
func (l leaves) Show(_ context.Context, title string) error {
	l.w.note("show " + title)
	return nil
}
func (l leaves) ShowCount(_ context.Context, n int64) error {
	l.w.note("count " + string(rune('0'+n)))
	return nil
}
func (l leaves) ShowOwner(_ context.Context, name *string) error {
	if name == nil {
		l.w.note("owner none")
	} else {
		l.w.note("owner " + *name)
	}
	return nil
}
func (l leaves) ShowToken(_ context.Context, token *app.Token) error {
	if token == nil {
		l.w.note("token none")
	} else {
		l.w.note("token " + string(*token))
	}
	return nil
}
func (l leaves) ShowBoth(_ context.Context, a, b *app.Token) error {
	l.w.note("both")
	return nil
}
func (l leaves) ShowLines(_ context.Context, lines []app.Line) error {
	l.w.note("lines")
	return nil
}

func newOps(w *world) app.Ops {
	// 引数の並びは、leaves、状態の単位（宣言順: 基底の状態と、述語で決まる状態の分類）
	return app.New(leaves{w}, tokenState{w}, linesState{w}, settingsState{w}, journalState{}, syncState{w})
}

func expectLog(t *testing.T, w *world, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(w.log, want) {
		t.Errorf("呼ばれた順:\n  got  %q\n  want %q", w.log, want)
	}
	w.log = nil
}

// read を呼ぶ配線は、読む状態の単位の問い合わせを呼ぶ: return、const、型をそろえる return、アームの本体
func TestReads(t *testing.T) {
	ctx := context.Background()
	w := &world{lines: []app.Line{{Item: "pen", Count: 2}}, settings: app.Settings{Title: "shop"}}
	o := newOps(w)

	if got, err := o.Find(ctx, "pen"); err != nil || got == nil || got.Count != 2 {
		t.Fatalf("Find = %v, %v", got, err)
	}
	if got, err := o.Find(ctx, "ink"); err != nil || got != nil {
		t.Fatalf("Find = %v, %v", got, err)
	}
	expectLog(t, w, "lines.LineFor pen", "lines.LineFor ink")

	if got, err := o.Total(ctx); err != nil || got != 1 {
		t.Fatalf("Total = %d, %v", got, err)
	}
	expectLog(t, w, "lines.CountLines", "count 1")
	if got, err := o.MaybeTotal(ctx); err != nil || got == nil || *got != 1 {
		t.Fatalf("MaybeTotal = %v, %v", got, err)
	}
	if got, err := o.Title(ctx); err != nil || got != "shop" {
		t.Fatalf("Title = %q, %v", got, err)
	}
	expectLog(t, w, "lines.CountLines", "settings.ShopTitle")

	// アームの本体の read: 絞り込んだ対象を引数に渡す。もう一方のアームは、read を呼ばない
	pen := "pen"
	if got, err := o.FindOrNone(ctx, &pen); err != nil || got == nil {
		t.Fatalf("FindOrNone = %v, %v", got, err)
	}
	if got, err := o.FindOrNone(ctx, nil); err != nil || got != nil {
		t.Fatalf("FindOrNone(nil) = %v, %v", got, err)
	}
	if got, err := o.CountOrZero(ctx, &pen); err != nil || got != 1 {
		t.Fatalf("CountOrZero = %d, %v", got, err)
	}
	if got, err := o.CountOrZero(ctx, nil); err != nil || got != 0 {
		t.Fatalf("CountOrZero(nil) = %d, %v", got, err)
	}
	expectLog(t, w, "lines.LineFor pen", "lines.CountLines")
}

// 効果を呼ぶ配線は、宛先の単位の命令を呼ぶ: 基底の状態、述語で決まる状態の分類の単位、型で決まる状態の分類の、ただ一つの
// 対象の状態。引数の型をそろえ、渡されていない省略可能な引数は nil
func TestEffects(t *testing.T) {
	ctx := context.Background()
	tok := app.Token("t0")
	w := &world{token: &tok, stale: true, lines: []app.Line{{Item: "old", Count: 1}}}
	o := newOps(w)

	if err := o.Add(ctx, app.Line{Item: "pen", Count: 1}); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "lines.AddLine pen")
	if len(w.lines) != 2 {
		t.Errorf("Add の後の lines = %v", w.lines)
	}

	if err := o.Reset(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "lines.ClearLines", "sync.Refresh", "token.SignIn t1", "token.DropToken")
	if len(w.lines) != 0 || w.stale || w.token != nil {
		t.Errorf("Reset の後: lines = %v, stale = %v, token = %v", w.lines, w.stale, w.token)
	}

	if err := o.Retitle(ctx, "name", app.Owner{Name: "me"}); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "settings.Rename name by me", "settings.Rename name by nobody", "lines.AddLine gift")

	// 効果が失敗すれば、配線はそこで止まり、そのエラーを返す
	boom := errors.New("boom")
	w.addErr = boom
	if err := o.Add(ctx, app.Line{Item: "ink"}); err != boom {
		t.Fatalf("Add = %v, want boom", err)
	}
	if err := o.Retitle(ctx, "again", app.Owner{Name: "you"}); err != boom {
		t.Fatalf("Retitle = %v, want boom", err)
	}
	expectLog(t, w, "lines.AddLine ink", "settings.Rename again by you", "settings.Rename again by nobody", "lines.AddLine gift")
}

// 直に呼ぶ組み込みの効果は、その状態の単位の、形で決まる命令を呼ぶ: 値を取る操作は値を渡し、nullable の delete は渡さない
func TestDirectEffects(t *testing.T) {
	ctx := context.Background()
	w := &world{}
	o := newOps(w)
	if err := o.Direct(ctx, app.Line{Item: "pen", Count: 1}, "t1"); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "lines.Post pen", "token.Put t1", "token.Delete", "token.Post t1", "settings.Put direct")
	if len(w.lines) != 1 || w.token == nil || *w.token != "t1" || w.settings.Title != "direct" {
		t.Errorf("Direct の後: lines = %v, token = %v, settings = %+v", w.lines, w.token, w.settings)
	}

	// 文の match のアームの本体: 効果と、直に呼ぶ組み込みの効果
	if err := o.Toggle(ctx, "t2"); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "token.Get", "token.DropToken")
	if err := o.Toggle(ctx, "t2"); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "token.Get", "token.Post t2")
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

	// 一つの文の中の同じ状態は、一度だけ読む。効果の引数に書いた状態も、その文の前に読む
	if err := o.Once(ctx); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "token.Get", "both", "settings.Get", "lines.AddLine shop", "token.Get", "token t1")

	// match の対象が状態: 先に一度読む。アームの本体だけが使う状態は、その枝の中で読む
	if err := o.Branches(ctx); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "lines.Get", "lines", "token.Get", "token.DropToken")
	w.lines = nil
	if err := o.Branches(ctx); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "lines.Get", "settings.Get", "show shop", "token.Get", "token none")
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

// 葉の op の置き場（plugins/go/spec/go.hozo の Requirement.Ports の「op の置き場」の 3）: 葉は、いつも Leaves。デコレータが reads / effects を運んでいても
// （一つの状態でも、二つの状態でも）、状態の単位のメソッドにはならない
func TestLeafPlaces(t *testing.T) {
	ctx := context.Background()
	w := &world{stale: true}
	o := newOps(w)
	pen := app.Line{Item: "pen", Count: 1}
	if err := o.Push(ctx, pen); err != nil {
		t.Fatal(err)
	}
	if err := o.Checkout(ctx, pen); err != nil {
		t.Fatal(err)
	}
	if got, err := o.Summary(ctx); err != nil || got != "summary" {
		t.Fatalf("Summary = %q, %v", got, err)
	}
	if got, err := o.Peek(ctx); err != nil || got != nil {
		t.Fatalf("Peek = %v, %v", got, err)
	}
	if err := o.Audit(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	expectLog(t, w, "leaves.Push pen", "leaves.Checkout pen", "leaves.Summary", "leaves.Peek", "audit x")

	// 葉と、read と、効果と、直に呼ぶ組み込みの効果を、一つの配線から呼ぶ（行の順）
	got, err := o.Finish(ctx, pen)
	if err != nil || got != "summary" {
		t.Fatalf("Finish = %q, %v", got, err)
	}
	expectLog(t, w, "leaves.Push pen", "leaves.Checkout pen", "lines.Post pen", "sync.Refresh", "audit finished", "lines.CountLines", "count 1", "leaves.Summary")
	if w.stale {
		t.Error("分類の単位の命令 Refresh が、状態を変えていない")
	}
}

// 述語で決まる状態の分類は、配線が match に使っていなくても、判定関数を出す（テストが、効果の結果の判定に使う）
func TestClassifySync(t *testing.T) {
	ctx := context.Background()
	w := &world{stale: true}
	o := newOps(w)
	if k, err := app.ClassifySync(ctx, syncState{w}); err != nil || k != app.SyncStale {
		t.Fatalf("ClassifySync = %v, %v, want SyncStale", k, err)
	}
	if err := o.Reset(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	if k, err := app.ClassifySync(ctx, syncState{w}); err != nil || k != app.SyncFresh {
		t.Fatalf("ClassifySync = %v, %v, want SyncFresh", k, err)
	}
}
