// 入力 locals の出力を動かす（plugins/go-cli の TestBehavior が、基底 go・go-valid・go-cli の出力を書いたモジュールのルートに
// 置いて走らせる）。名前空間のパッケージの名前が、cli.go の局所の名前（env、in）や、標準ライブラリのパッケージの名前（errors、fmt）と
// 当たっても、コンパイルでき、同じに動くこと。
package behavior

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"example.com/app/gen/env"
	envcli "example.com/app/gen/env/cli"
	pkgerrors "example.com/app/gen/errors"
	pkgfmt "example.com/app/gen/fmt"
	"example.com/app/gen/in"
	incli "example.com/app/gen/in/cli"
	pkgline "example.com/app/gen/line"
	pkgreflect "example.com/app/gen/reflect"
)

type envLeaves struct{ got env.RunInput }

func (l *envLeaves) Run(ctx context.Context, input env.RunInput) (string, error) {
	l.got = input
	switch {
	case input.Name != nil && *input.Name == "lost":
		return "", pkgerrors.Lost{Value: pkgerrors.Where{Path: "p"}}
	case input.Name != nil && *input.Name == "wide":
		return "", pkgfmt.Wide{}
	}
	return "ran", nil
}

type inLeaves struct{ got in.GoInput }

func (l *inLeaves) Go(ctx context.Context, input in.GoInput) error {
	l.got = input
	switch input.N {
	case 0:
		return pkgerrors.Lost{}
	case 4:
		return pkgreflect.Bent{}
	case 5:
		return pkgline.Cut{}
	}
	return nil
}

func TestEnv(t *testing.T) {
	run := func(vars map[string]string, args ...string) (int, string, string, env.RunInput) {
		l := &envLeaves{}
		var out, errOut bytes.Buffer
		code := envcli.Main(context.Background(), env.New(l), envcli.JSONPresenter{}, args, func(k string) string { return vars[k] }, &out, &errOut)
		return code, out.String(), errOut.String(), l.got
	}
	code, out, errOut, got := run(map[string]string{"LEVEL": "4"}, "run", "a", "--name", "n", "b")
	name := "n"
	if code != 0 || out != "\"ran\"\n" || errOut != "" || !reflect.DeepEqual(got, env.RunInput{Level: 4, Name: &name, Rest: []string{"a", "b"}}) {
		t.Errorf("code=%d stdout=%q stderr=%q 入力=%+v", code, out, errOut, got)
	}
	// 補う値（@cli.default）と、Validate
	if _, _, _, got = run(nil, "run"); got.Level != 1 {
		t.Errorf("補う値: %+v", got)
	}
	if code, _, errOut, _ = run(nil, "run", "--level", "0"); code != 64 || !strings.Contains(errOut, "invalid input: level: min") {
		t.Errorf("Validate: code=%d stderr=%q", code, errOut)
	}
	// 標準ライブラリと同じ名前のパッケージの例外
	if code, _, errOut, _ = run(nil, "run", "--name", "lost"); code != 3 || !strings.Contains(errOut, `"error": "errors.Lost"`) || !strings.Contains(errOut, `"path": "p"`) {
		t.Errorf("errors.Lost: code=%d stderr=%q", code, errOut)
	}
	if code, _, errOut, _ = run(nil, "run", "--name", "wide"); code != 70 || !strings.Contains(errOut, `"error": "fmt.Wide"`) {
		t.Errorf("fmt.Wide: code=%d stderr=%q", code, errOut)
	}
}

func TestIn(t *testing.T) {
	run := func(args ...string) (int, string, in.GoInput) {
		l := &inLeaves{}
		var out, errOut bytes.Buffer
		code := incli.Main(context.Background(), in.New(l), incli.JSONPresenter{}, args, nil, &out, &errOut)
		return code, errOut.String(), l.got
	}
	yes := true
	if code, errOut, got := run("go", "3", "--deep"); code != 0 || errOut != "" || !reflect.DeepEqual(got, in.GoInput{N: 3, Deep: &yes}) {
		t.Errorf("code=%d stderr=%q 入力=%+v", code, errOut, got)
	}
	if code, errOut, _ := run("go", "0"); code != 3 || !strings.Contains(errOut, `"error": "errors.Lost"`) {
		t.Errorf("errors.Lost: code=%d stderr=%q", code, errOut)
	}
	if code, errOut, _ := run("go"); code != 64 || !strings.Contains(errOut, "missing required argument N") {
		t.Errorf("位置引数が足りない: code=%d stderr=%q", code, errOut)
	}
	// 補助が使う標準ライブラリ（reflect）や、局所の名前（line）と同じ名前のパッケージの例外
	if code, errOut, _ := run("go", "4"); code != 4 || !strings.Contains(errOut, `"error": "reflect.Bent"`) {
		t.Errorf("reflect.Bent: code=%d stderr=%q", code, errOut)
	}
	if code, errOut, _ := run("go", "5"); code != 5 || !strings.Contains(errOut, `"error": "line.Cut"`) {
		t.Errorf("line.Cut: code=%d stderr=%q", code, errOut)
	}
}
