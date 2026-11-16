// 入力 shapes の出力を動かす（plugins/go-cli の TestBehavior が、基底 go・go-valid・go-cli の出力を書いたモジュールのルートに
// 置いて走らせる）。shapea と shapeb の cli.go は、名前空間の名前のほかは同じテキスト（plugins/go-cli の TestSameShape）。
// ここでは、型の違いが、実行時に出ることを確かめる（docs/testing.md「単体テスト」の「型に依らない形の確かめ」）:
// 値を取らないフラグか（読む先が真偽値か）、残りの位置引数か（読む先がスライスか）、省略可能か（読む先がポインタか）、
// 文字列の読み方（int / float / scalar）、補う値の型、入力が Validate を持つか、例外が引数を持つか。
package behavior

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"example.com/app/gen/shapea"
	acli "example.com/app/gen/shapea/cli"
	"example.com/app/gen/shapeb"
	bcli "example.com/app/gen/shapeb/cli"
)

type aLeaves struct {
	called bool
	got    shapea.RunInput
}

func (l *aLeaves) Run(ctx context.Context, in shapea.RunInput) (shapea.Note, error) {
	l.called, l.got = true, in
	switch in.First {
	case "broken":
		return shapea.Note{}, shapea.Broken{}
	case "lost":
		return shapea.Note{}, shapea.Lost{}
	}
	return shapea.Note{Text: "a"}, nil
}

type bLeaves struct {
	called bool
	got    shapeb.RunInput
}

func (l *bLeaves) Run(ctx context.Context, in shapeb.RunInput) (*shapeb.Note, error) {
	l.called, l.got = true, in
	switch in.First {
	case 3:
		return nil, shapeb.Broken{Value: shapeb.Note{Text: "why"}}
	case 4:
		return nil, shapeb.Lost{}
	}
	return &shapeb.Note{Text: "b"}, nil
}

// home は、必須の環境変数だけを持つ環境。
var home = map[string]string{"SHAPE_HOME": "hh"}

func runA(env map[string]string, args ...string) (int, string, string, *aLeaves) {
	l := &aLeaves{}
	var out, errOut bytes.Buffer
	code := acli.Main(context.Background(), shapea.New(l), acli.JSONPresenter{}, args, func(k string) string { return env[k] }, &out, &errOut)
	return code, out.String(), errOut.String(), l
}

func runB(env map[string]string, args ...string) (int, string, string, *bLeaves) {
	l := &bLeaves{}
	var out, errOut bytes.Buffer
	code := bcli.Main(context.Background(), shapeb.New(l), bcli.JSONPresenter{}, args, func(k string) string { return env[k] }, &out, &errOut)
	return code, out.String(), errOut.String(), l
}

// TestSameArguments は、同じ引数の並びを、二つのコマンドに渡す: 読み方の違いは、読む先の型だけから来る。
func TestSameArguments(t *testing.T) {
	// 文字列のフィールドと、int / scalar / 省略可能のフィールド。補う値 1 は、int にも、float の scalar にもなる
	code, _, errOut, a := runA(home, "run", "--name", "nn", "5", "rr")
	if want := (shapea.RunInput{Name: "nn", Level: 1, First: "5", Rest: "rr", Home: "hh"}); code != 0 || errOut != "" || !reflect.DeepEqual(a.got, want) {
		t.Errorf("shapea: code=%d stderr=%q 入力=%+v, want %+v", code, errOut, a.got, want)
	}
	name := shapeb.Name("nn")
	code, _, errOut, b := runB(home, "run", "--name", "nn", "5", "rr")
	if want := (shapeb.RunInput{Name: &name, Level: 1, First: 5, Rest: []shapeb.Name{"rr"}, Home: "hh"}); code != 0 || errOut != "" || !reflect.DeepEqual(b.got, want) {
		t.Errorf("shapeb: code=%d stderr=%q 入力=%+v, want %+v", code, errOut, b.got, want)
	}

	// --on: 読む先が文字列（の省略可能）なら値を取り、真偽値なら取らない
	on := "5"
	if code, _, errOut, a = runA(home, "run", "--on", "5", "6"); code != 0 || !reflect.DeepEqual(a.got.On, &on) || a.got.First != "6" {
		t.Errorf("shapea の --on: code=%d stderr=%q 入力=%+v", code, errOut, a.got)
	}
	if code, _, errOut, b = runB(home, "run", "--on", "5", "66"); code != 0 || !b.got.On || b.got.First != 5 || !reflect.DeepEqual(b.got.Rest, []shapeb.Name{"66"}) {
		t.Errorf("shapeb の --on: code=%d stderr=%q 入力=%+v", code, errOut, b.got)
	}
	if code, _, errOut, a = runA(home, "run", "5", "--on"); code != 64 || !strings.Contains(errOut, "flag --on needs a value") || a.called {
		t.Errorf("shapea の、値のない --on: code=%d stderr=%q", code, errOut)
	}
	if code, _, errOut, b = runB(home, "run", "5", "--on"); code != 0 || !b.got.On {
		t.Errorf("shapeb の、値のない --on: code=%d stderr=%q", code, errOut)
	}

	// 位置 2: 読む先がスライスなら残りすべて、そうでなければ一つだけ（多ければ誤り）
	if code, _, errOut, a = runA(home, "run", "5", "rr", "ss"); code != 64 || !strings.Contains(errOut, "too many arguments: 3 given, at most 2 expected") || a.called {
		t.Errorf("shapea の、三つの位置引数: code=%d stderr=%q", code, errOut)
	}
	if code, _, errOut, b = runB(home, "run", "5", "rr", "ss"); code != 0 || !reflect.DeepEqual(b.got.Rest, []shapeb.Name{"rr", "ss"}) {
		t.Errorf("shapeb の、三つの位置引数: code=%d stderr=%q 入力=%+v", code, errOut, b.got)
	}

	// 位置 1: 読む先が文字列ならそのまま、int なら変換する
	if code, _, errOut, a = runA(home, "run", "x"); code != 0 || a.got.First != "x" {
		t.Errorf("shapea の、数でない位置引数: code=%d stderr=%q", code, errOut)
	}
	if code, _, errOut, b = runB(home, "run", "x"); code != 64 || !strings.Contains(errOut, `invalid value "x" for FIRST: want int`) || b.called {
		t.Errorf("shapeb の、数でない位置引数: code=%d stderr=%q", code, errOut)
	}

	// 名前を省いた @cli.flag と @cli.env（どちらも size）: 読む先が int の省略可能か、float か
	size := int64(7)
	if code, _, errOut, a = runA(map[string]string{"SHAPE_HOME": "hh", "size": "7"}, "run", "5"); code != 0 || !reflect.DeepEqual(a.got.Size, &size) {
		t.Errorf("shapea の環境変数 size: code=%d stderr=%q 入力=%+v", code, errOut, a.got)
	}
	if code, _, errOut, a = runA(home, "run", "5", "--size", "1.5"); code != 64 || !strings.Contains(errOut, `invalid value "1.5" for --size: want int`) {
		t.Errorf("shapea の --size 1.5: code=%d stderr=%q", code, errOut)
	}
	if code, _, errOut, b = runB(home, "run", "5", "--size", "1.5"); code != 0 || b.got.Size != 1.5 {
		t.Errorf("shapeb の --size 1.5: code=%d stderr=%q 入力=%+v", code, errOut, b.got)
	}
	// 与えられなかった省略可能なフィールドは nil、省略可能でないフィールドは零値
	if _, _, _, a = runA(home, "run", "5"); a.got.Size != nil || a.got.On != nil {
		t.Errorf("shapea の、与えられなかったフィールド: %+v", a.got)
	}
	if _, _, _, b = runB(home, "run", "5"); b.got.Size != 0 || b.got.Name != nil || b.got.Rest != nil {
		t.Errorf("shapeb の、与えられなかったフィールド: %+v", b.got)
	}
}

// TestRequired: @cli.required の扱いは、型によらない（どちらも、なければ 64）。
func TestRequired(t *testing.T) {
	for name, run := range map[string]func(map[string]string, ...string) (int, string, bool){
		"shapea": func(env map[string]string, args ...string) (int, string, bool) {
			code, _, errOut, l := runA(env, args...)
			return code, errOut, l.called
		},
		"shapeb": func(env map[string]string, args ...string) (int, string, bool) {
			code, _, errOut, l := runB(env, args...)
			return code, errOut, l.called
		},
	} {
		if code, errOut, called := run(home, "run"); code != 64 || !strings.Contains(errOut, "missing required argument FIRST") || called {
			t.Errorf("%s: 位置引数がない: code=%d stderr=%q", name, code, errOut)
		}
		if code, errOut, called := run(nil, "run", "5"); code != 64 || !strings.Contains(errOut, "missing required environment variable SHAPE_HOME") || called {
			t.Errorf("%s: 環境変数がない: code=%d stderr=%q", name, code, errOut)
		}
	}
}

// TestValidate: どちらのコマンドも、同じ補助を呼ぶ。入力が Validate を持つ shapeb だけが、違反を 64 にする。
func TestValidate(t *testing.T) {
	if code, _, errOut, a := runA(home, "run", "5", "--name", "n"); code != 0 || errOut != "" || !a.called {
		t.Errorf("shapea（Validate を持たない）: code=%d stderr=%q", code, errOut)
	}
	code, _, errOut, b := runB(home, "run", "5", "--name", "n")
	if code != 64 || !strings.Contains(errOut, "invalid input: name: length") || strings.Contains(errOut, "usage:") || b.called {
		t.Errorf("shapeb（Validate を持つ）: code=%d stderr=%q", code, errOut)
	}
	// 環境変数から読んだ値も、array の要素も確かめる
	if code, _, errOut, _ = runB(map[string]string{"SHAPE_HOME": "h"}, "run", "5", "ok", "x"); code != 64 || !strings.Contains(errOut, "rest[1]: length") || !strings.Contains(errOut, "home: length") {
		t.Errorf("shapeb の、環境変数と array の違反: code=%d stderr=%q", code, errOut)
	}
}

// TestThrown: 例外ごとの行は同じ形。引数を持つかどうか（表示に value を出すか）は、実行時に決まる。value は、引数の名前を
// キーにしたオブジェクトで、引数の名前が value でも、同じ（value の中の、キー value）。
func TestThrown(t *testing.T) {
	code, out, errOut, _ := runA(home, "run", "broken")
	if code != 3 || out != "" || errOut != "{\n  \"error\": \"shapea.Broken\"\n}\n" {
		t.Errorf("shapea.Broken（引数を持たない）: code=%d stderr=%q", code, errOut)
	}
	code, out, errOut, _ = runB(home, "run", "3")
	if code != 3 || out != "" || errOut != "{\n  \"error\": \"shapeb.Broken\",\n  \"value\": {\n    \"value\": {\n      \"text\": \"why\"\n    }\n  }\n}\n" {
		t.Errorf("shapeb.Broken（引数を持つ）: code=%d stderr=%q", code, errOut)
	}
	if code, _, errOut, _ = runA(home, "run", "lost"); code != 70 || !strings.Contains(errOut, `"error": "shapea.Lost"`) {
		t.Errorf("shapea.Lost: code=%d stderr=%q", code, errOut)
	}
	if code, _, errOut, _ = runB(home, "run", "4"); code != 70 || !strings.Contains(errOut, `"error": "shapeb.Lost"`) {
		t.Errorf("shapeb.Lost: code=%d stderr=%q", code, errOut)
	}
}

// TestUsage: 使い方の、型で変わるところ（値の型の名前、値を取らないフラグ、残りの位置引数）は、補助が、並べた行から実行時に作る。
func TestUsage(t *testing.T) {
	_, out, _, _ := runA(nil, "run", "--help")
	for _, want := range []string{" run [flags] FIRST [REST]\n", "  --name <string>   名前\n", "  --level <int>     (default: 1)\n", "  --on <string>\n", "  --size <int>      (env: size)\n", "  SHAPE_HOME   (required)\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("shapea の使い方に %q がない:\n%s", want, out)
		}
	}
	_, out, _, _ = runB(nil, "run", "--help")
	for _, want := range []string{" run [flags] FIRST [REST...]\n", "  --name <Name>     名前\n", "  --level <Level>   (default: 1)\n", "  --on\n", "  --size <float>    (env: size)\n", "  SHAPE_HOME   (required)\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("shapeb の使い方に %q がない:\n%s", want, out)
		}
	}
}
