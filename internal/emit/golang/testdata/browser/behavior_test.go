package browser

// このファイルは生成物ではない。internal/emit/golang の TestBehavior が、生成した browser のパッケージ
// （testdata/browser/expected/gen/browser と同じ内容）の隣に置いて `go test` する。Deps、Wrappers、Classifiers、
// States を偽物で埋め、docs/emit-go.md「op」「デコレータ」「配線の本体」「実装の単位」の振る舞いを確かめる。

import (
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/server"
	"github.com/mew-ton/hozo/runtime"
)

func wantMissing(t *testing.T, what string, err error, kind, name string) {
	t.Helper()
	var miss *runtime.MissingError
	if !errors.As(err, &miss) || miss.Kind != kind || miss.Name != name {
		t.Errorf("%s: err = %#v, want *runtime.MissingError{Kind: %q, Name: %q}", what, err, kind, name)
	}
}

// warranty は WarrantyKindCases の偽物。big は IsBigCases の偽物。
type warranty struct{ standard, yamada bool }

func (w warranty) Standard(server.OrderRow) bool { return w.standard }
func (w warranty) Yamada(server.OrderRow) bool   { return w.yamada }

type big bool

func (b big) IsBig(server.OrderRow) bool { return bool(b) }

// world は偽物の一式。log は呼ばれた順。
type world struct {
	log      []string
	getMeIn  GetMeInInput
	rendered RenderInput
	token    *server.Token
}

func (w *world) note(s string) { w.log = append(w.log, s) }

func (w *world) leaves() Leaves {
	return Leaves{
		Render:       func(in RenderInput) error { w.note("render"); w.rendered = in; return nil },
		PickFirst:    func(rows []server.OrderRow) (server.OrderRow, error) { w.note("pickFirst"); return rows[0], nil },
		StandardLine: func(row server.OrderRow) (server.OrderRow, error) { w.note("standardLine"); return row, nil },
		YamadaLine: func(row server.OrderRow, page *server.PosInt) (server.OrderRow, error) {
			w.note("yamadaLine")
			return row, nil
		},
		Celebrate: func() error { w.note("celebrate"); return nil },
		Logout:    func() error { w.note("logout"); return nil },
	}
}

func (w *world) deps() Deps {
	return Deps{Server: ServerOps{
		GetMe: func(in GetMeInInput) (server.PublicUser, error) {
			w.note("server.getMe")
			w.getMeIn = in
			return server.PublicUser{Id: "u_1", Name: "山田", Tags: []string{"a"}}, nil
		},
		ListOrders: func(query server.OrdersQuery) ([]server.OrderRow, error) {
			w.note("server.listOrders")
			return []server.OrderRow{{Id: "o1", Total: 10}}, nil
		},
	}}
}

// wrappers は、通ったことを記録して内側を呼ぶ包む関数。
func (w *world) wrappers() Wrappers {
	return Wrappers{
		Retry: RetryWrappers{Render: func(in RenderInput, next RenderFunc) error {
			w.note("retry(")
			err := next(in)
			w.note(")retry")
			return err
		}},
		GuardMe: GuardMeWrappers{Render: func(in RenderInput, next RenderFunc) error {
			w.note("guardMe(")
			err := next(in)
			w.note(")guardMe")
			return err
		}},
		HttpRetry: HttpRetryWrappers{PickFirst: func(rows []server.OrderRow, next PickFirstFunc) (server.OrderRow, error) {
			w.note("httpRetry")
			return next(rows)
		}},
		Tracked: TrackedWrappers{Logout: func(next LogoutFunc) error { w.note("tracked"); return next() }},
	}
}

func (w *world) install() {
	States = StateSet{Token: TokenState{
		IsEmpty: func() (bool, error) { return w.token == nil, nil },
		Get:     func() (*server.Token, error) { w.note("token"); return w.token, nil },
	}}
}

// 配線の本体: Deps を呼び、bind の値を States から取って同じ名前空間の op に渡し、包む関数を外側から順に通る。
func TestOpenMe(t *testing.T) {
	tok := server.Token("t1")
	w := &world{token: &tok}
	w.install()
	ops := New(w.leaves(), w.deps(), w.wrappers(), Classifiers{})
	me, err := ops.OpenMe()
	if err != nil {
		t.Fatal(err)
	}
	// 戻り値は server.getMe の結果そのまま（model から model への射影はない）
	if want := (server.PublicUser{Id: "u_1", Name: "山田", Tags: []string{"a"}}); !reflect.DeepEqual(me, want) {
		t.Errorf("OpenMe = %+v, want %+v", me, want)
	}
	// 別の名前空間の op には bind の位置（token）のない引数を渡す。埋めるのは Deps を埋める者
	if want := (GetMeInInput{Verbose: true}); w.getMeIn != want {
		t.Errorf("server.getMe の引数 = %+v, want %+v", w.getMeIn, want)
	}
	// 同じ名前空間の op（render）には、States から取った token を入れて渡す
	if w.rendered.Token == nil || *w.rendered.Token != "t1" || w.rendered.User.Name != "山田" || w.rendered.Where != nil {
		t.Errorf("render の引数 = %+v", w.rendered)
	}
	// 包む関数は内側（@guardMe）から順に包む: 外側の @retry が先に呼ばれる
	want := []string{"server.getMe", "token", "retry(", "guardMe(", "render", ")guardMe", ")retry"}
	if !reflect.DeepEqual(w.log, want) {
		t.Errorf("呼ばれた順 = %v, want %v", w.log, want)
	}
}

// 値の分類での match と、別の名前空間のデコレータの包む関数。
func TestChooseLine(t *testing.T) {
	w := &world{}
	w.install()
	ops := New(w.leaves(), w.deps(), w.wrappers(), Classifiers{WarrantyKind: warranty{yamada: true}, IsBig: big(true)})
	line, err := ops.ChooseLine(server.NewOrdersQuery())
	if err != nil || line == nil || line.Id != "o1" {
		t.Fatalf("ChooseLine = %v, %v", line, err)
	}
	want := []string{"server.listOrders", "httpRetry", "pickFirst", "yamadaLine", "celebrate"}
	if !reflect.DeepEqual(w.log, want) {
		t.Errorf("呼ばれた順 = %v, want %v", w.log, want)
	}
	// default のアーム（none）は nil
	w = &world{}
	ops = New(w.leaves(), w.deps(), w.wrappers(), Classifiers{WarrantyKind: warranty{}, IsBig: big(false)})
	if line, err = ops.ChooseLine(server.NewOrdersQuery()); err != nil || line != nil {
		t.Errorf("default: ChooseLine = %v, %v", line, err)
	}
}

// select の wraps の op は、配線でも口だけを出す（docs/emit-go.md「出力の対象」の 2）: 実装は Leaves、包むのは Wrappers。
func TestWrappedWiringIsASlot(t *testing.T) {
	w := &world{}
	ops := New(w.leaves(), Deps{}, w.wrappers(), Classifiers{})
	if err := ops.Logout(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"tracked", "logout"}; !reflect.DeepEqual(w.log, want) {
		t.Errorf("呼ばれた順 = %v, want %v", w.log, want)
	}
}

// docs/emit-go.md「実装の単位」: nil のフィールドが要るところへ来たら *runtime.MissingError。
func TestMissing(t *testing.T) {
	tok := server.Token("t1")
	t.Run("dep", func(t *testing.T) {
		w := &world{token: &tok}
		w.install()
		ops := New(w.leaves(), Deps{}, w.wrappers(), Classifiers{})
		_, err := ops.OpenMe()
		wantMissing(t, "OpenMe", err, "dep", "server.getMe")
		_, err = ops.ChooseLine(server.NewOrdersQuery())
		wantMissing(t, "ChooseLine", err, "dep", "server.listOrders")
		if len(w.log) != 0 {
			t.Errorf("Deps がないのに呼ばれた: %v", w.log)
		}
	})
	t.Run("state", func(t *testing.T) {
		w := &world{}
		States = StateSet{}
		ops := New(w.leaves(), w.deps(), w.wrappers(), Classifiers{})
		_, err := ops.OpenMe()
		wantMissing(t, "OpenMe", err, "state", "browser.token")
	})
	t.Run("classifier", func(t *testing.T) {
		w := &world{}
		w.install()
		ops := New(w.leaves(), w.deps(), w.wrappers(), Classifiers{})
		_, err := ops.ChooseLine(server.NewOrdersQuery())
		wantMissing(t, "ChooseLine", err, "classifier", "browser.WarrantyKind")
		// 一つ目の分類だけ埋めれば、二つ目で当たる
		ops = New(w.leaves(), w.deps(), w.wrappers(), Classifiers{WarrantyKind: warranty{standard: true}})
		_, err = ops.ChooseLine(server.NewOrdersQuery())
		wantMissing(t, "ChooseLine", err, "classifier", "browser.IsBig")
		_, err = ClassifyIsBig(nil, server.OrderRow{})
		wantMissing(t, "ClassifyIsBig(nil)", err, "classifier", "browser.IsBig")
	})
	t.Run("wrapper", func(t *testing.T) {
		w := &world{}
		ops := New(w.leaves(), Deps{}, Wrappers{}, Classifiers{})
		// 外側の包む関数（@retry）に先に当たる
		wantMissing(t, "Render", ops.Render(RenderInput{}), "wrapper", "browser.render@browser.retry")
		wantMissing(t, "Logout", ops.Logout(), "wrapper", "browser.logout@browser.tracked")
		_, err := ops.PickFirst(nil)
		wantMissing(t, "PickFirst", err, "wrapper", "browser.pickFirst@http.retry")
		// 外側だけ埋めれば、それが内側を呼んだところで当たる
		ws := w.wrappers()
		ws.GuardMe = GuardMeWrappers{}
		ops = New(w.leaves(), Deps{}, ws, Classifiers{})
		wantMissing(t, "Render", ops.Render(RenderInput{}), "wrapper", "browser.render@browser.guardMe")
		if want := []string{"retry(", ")retry"}; !reflect.DeepEqual(w.log, want) {
			t.Errorf("呼ばれた順 = %v, want %v", w.log, want)
		}
	})
	t.Run("leaf behind wrappers", func(t *testing.T) {
		w := &world{}
		ops := New(Leaves{}, Deps{}, w.wrappers(), Classifiers{})
		wantMissing(t, "Render", ops.Render(RenderInput{}), "leaf", "browser.render")
		wantMissing(t, "Logout", ops.Logout(), "leaf", "browser.logout")
		wantMissing(t, "Celebrate", ops.Celebrate(), "leaf", "browser.celebrate")
	})
}
