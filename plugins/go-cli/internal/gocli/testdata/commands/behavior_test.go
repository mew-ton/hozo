// 入力 commands の出力を動かす（plugins/go-cli の TestBehavior が、基底 go・go-valid・go-cli の出力を書いたモジュールのルートに
// 置いて走らせる）。確かめること（docs/plugin-go-cli.md）:
//
//   - 「出力」: Main は、基底の New の戻り値（入口の Ops）をそのまま受け取り、ctx を op にそのまま渡す。
//   - 「引数の解釈」の各規則: コマンドがない・知らない、help / --help / -h、--name value と --name=value、値を取らない boolean の
//     フラグ（読む先の型で、実行時に決まる）、同じフラグが二度、-x は位置引数、-- 以降は位置引数、知らないフラグ・値のないフラグ・
//     変換できない値・位置引数が多い（どれも 64）、@cli.env（フラグと位置引数が優先、空文字列は「ない」）、@cli.arg（位置は引数で
//     決まり、宣言の順ではない。array は残りすべて）、名前を省いた @cli.flag / @cli.env（フィールドの名前）、値の変換
//     （int / float / boolean / scalar）、Validate の違反（64）。
//   - 値が与えられなかったときの三つの扱い: @cli.required（誤り。64）、@cli.default（その値）、どちらもない（零値のまま）。
//     フラグ、位置引数、環境変数だけのフィールド、残りを受け取る array で。
//   - 「実行と表示」: 成功は Present を呼んで 0。例外は Signature の順に値の型で見分けて Failure を作り、@cli.exitCode か 70。
//     見分けられなければ 70。Present / PresentError の失敗は 70。JSONPresenter の形。Failure の Value は、引数を持つ例外の
//     値そのもの（表示の value は、引数の名前をキーにしたオブジェクト）で、引数を持たない例外は nil（value を出さない）。
//   - 「使い方の表示」: 全体とコマンドごと。プログラム名は filepath.Base(os.Args[0])。
package behavior

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"example.com/app/gen/bare"
	barecli "example.com/app/gen/bare/cli"
	"example.com/app/gen/demo"
	democli "example.com/app/gen/demo/cli"
	"example.com/app/gen/fail"
	"example.com/app/gen/tool"
	toolcli "example.com/app/gen/tool/cli"
)

type ctxKey struct{}

// prog は、使い方に出るプログラム名。
var prog = filepath.Base(os.Args[0])

// toolLeaves は tool の葉の op。config の値で振る舞いを変え、受け取った ctx と入力を覚える。
type toolLeaves struct {
	ctx   context.Context
	calls []string
	check tool.CheckInput
	serve tool.ServeInput
}

func str[T ~string](p *T) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

func notes(code string) []tool.Note {
	return []tool.Note{{Code: code, Message: "m"}}
}

func (l *toolLeaves) Check(ctx context.Context, in tool.CheckInput) (tool.CheckResult, error) {
	l.ctx, l.check, l.calls = ctx, in, append(l.calls, "check")
	switch str(in.Config) {
	case "x1":
		return tool.CheckResult{}, tool.CheckFailed{Notes: notes("T-1")}
	case "x2":
		return tool.CheckResult{}, fmt.Errorf("wrapped: %w", tool.ConfigInvalid{File: "x2", Notes: notes("CFG-12")}) // 包んでも見分ける
	case "x3":
		return tool.CheckResult{}, &tool.ConfigInvalid{File: "x3", Notes: notes("CFG-12")} // ポインタで返した例外は見分けない（例外は値で返す）
	case "x5":
		return tool.CheckResult{}, fail.Crashed{}
	case "x6":
		return tool.CheckResult{}, errors.New("boom")
	case "x7":
		return tool.CheckResult{}, tool.Busy{}
	}
	msg := "config=" + str(in.Config) + " format=" + str(in.Format) + " path=" + str(in.SearchPath)
	return tool.CheckResult{Warnings: []tool.Note{{Code: "W", Message: msg}}}, nil
}

func (l *toolLeaves) Write(_ context.Context, in tool.EmitInput, checked tool.CheckResult) (tool.EmitResult, error) {
	l.calls = append(l.calls, "write")
	if str(in.Config) == "x4" {
		return tool.EmitResult{}, tool.UnknownName{}
	}
	res := tool.EmitResult{}
	for _, name := range in.Names {
		res.Written = append(res.Written, tool.Path(name))
	}
	if in.DryRun != nil && *in.DryRun {
		res.Removed = []tool.Path{"dry"}
	}
	return res, nil
}

func (l *toolLeaves) Serve(ctx context.Context, in tool.ServeInput) error {
	l.ctx, l.serve, l.calls = ctx, in, append(l.calls, "serve")
	if str(in.Log) == "x6" {
		return errors.New("boom")
	}
	return nil
}

// demoLeaves は demo の葉の op。受け取った入力を覚える。
type demoLeaves struct {
	calls []string
	add   demo.AddInput
	copy  demo.CopyInput
	move  demo.MoveInput
}

// Add は入力の数を一つに畳む: a + b + count*100 + (verbose ? 0.5 : 0) + len(label) + scale*1000。
func (l *demoLeaves) Add(ctx context.Context, in demo.AddInput) (demo.Sum, error) {
	l.calls, l.add = append(l.calls, "add"), in
	switch in.A {
	case 99:
		return demo.Sum{}, demo.Overflow{}
	case 98:
		return demo.Sum{}, demo.Tired{}
	}
	t := float64(in.A) + float64(in.Count)*100
	if in.B != nil {
		t += *in.B
	}
	if in.Verbose {
		t += 0.5
	}
	if in.Label != nil {
		t += float64(len(*in.Label))
	}
	if in.Scale != nil {
		t += *in.Scale * 1000
	}
	return demo.Sum{Total: t}, nil
}

func (l *demoLeaves) Copy(ctx context.Context, in demo.CopyInput) (string, error) {
	l.calls, l.copy = append(l.calls, "copy"), in
	return "copied", nil
}

func (l *demoLeaves) Move(ctx context.Context, in demo.MoveInput) (string, error) {
	l.calls, l.move = append(l.calls, "move"), in
	return "moved", nil
}

func (l *demoLeaves) Ping(ctx context.Context) error {
	l.calls = append(l.calls, "ping")
	return nil
}

// run は、一度の実行と、その結果。
type run struct {
	code           int
	stdout, stderr string
}

func getenv(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

func runTool(l *toolLeaves, env map[string]string, args ...string) run {
	var out, errOut bytes.Buffer
	code := toolcli.Main(context.Background(), tool.New(l), toolcli.JSONPresenter{}, args, getenv(env), &out, &errOut)
	return run{code, out.String(), errOut.String()}
}

func runDemo(l *demoLeaves, env map[string]string, args ...string) run {
	var out, errOut bytes.Buffer
	code := democli.Main(context.Background(), demo.New(l), democli.JSONPresenter{}, args, getenv(env), &out, &errOut)
	return run{code, out.String(), errOut.String()}
}

// scenario は一度の実行と、期待する終了コードと出力。stdout / stderr は含まれるべき部分文字列。
type scenario struct {
	name        string
	args        []string
	env         map[string]string
	code        int
	stdout      []string
	stderr      []string
	stdoutEmpty bool
	stderrEmpty bool
	notStdout   []string
	notStderr   []string
	calls       string // 呼ばれた葉の op（空白区切り）。"-" なら、どれも呼ばれない
}

func check(t *testing.T, s scenario, r run, calls []string) {
	t.Helper()
	if r.code != s.code {
		t.Errorf("終了コード = %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, s.code, r.stdout, r.stderr)
	}
	for _, w := range s.stdout {
		if !strings.Contains(r.stdout, w) {
			t.Errorf("stdout に %q がない:\n%s", w, r.stdout)
		}
	}
	for _, w := range s.stderr {
		if !strings.Contains(r.stderr, w) {
			t.Errorf("stderr に %q がない:\n%s", w, r.stderr)
		}
	}
	for _, w := range s.notStdout {
		if strings.Contains(r.stdout, w) {
			t.Errorf("stdout に %q がある:\n%s", w, r.stdout)
		}
	}
	for _, w := range s.notStderr {
		if strings.Contains(r.stderr, w) {
			t.Errorf("stderr に %q がある:\n%s", w, r.stderr)
		}
	}
	if s.stdoutEmpty && r.stdout != "" {
		t.Errorf("stdout が空でない:\n%s", r.stdout)
	}
	if s.stderrEmpty && r.stderr != "" {
		t.Errorf("stderr が空でない:\n%s", r.stderr)
	}
	switch {
	case s.calls == "-" && len(calls) != 0:
		t.Errorf("op を呼んだ: %v", calls)
	case s.calls != "" && s.calls != "-" && strings.Join(calls, " ") != s.calls:
		t.Errorf("呼んだ op = %v, want %s", calls, s.calls)
	}
}

func TestTool(t *testing.T) {
	scenarios := []scenario{
		// 実行と表示
		{name: "成功は Present して 0", args: []string{"check", "--format", "json"}, code: 0, stdout: []string{`"warnings"`, "config= format=json path="}, stderrEmpty: true, calls: "check"},
		{name: "--name=value", args: []string{"check", "--format=text"}, code: 0, stdout: []string{"format=text"}},
		{name: "引数を持つ例外と @cli.exitCode", args: []string{"check", "--config", "x1"}, code: 1, stdoutEmpty: true, stderr: []string{"{\n  \"error\": \"tool.CheckFailed\",\n  \"value\": {\n    \"notes\": [\n      {\n        \"code\": \"T-1\",\n        \"message\": \"m\"\n      }\n    ]\n  }\n}\n"}},
		{name: "包んだ例外も見分ける（引数が三つ。値のない省略可能な引数は、キーを省く）", args: []string{"check", "--config", "x2"}, code: 2, stderr: []string{"{\n  \"error\": \"tool.ConfigInvalid\",\n  \"value\": {\n    \"file\": \"x2\",\n    \"notes\": [\n", `"CFG-12"`}, notStderr: []string{`"hint"`}},
		{name: "ポインタで返した例外は見分けない", args: []string{"check", "--config", "x3"}, code: 70, stderr: []string{`"error": "tool.ConfigInvalid"`}, notStderr: []string{`"value"`}},
		{name: "別の名前空間の例外（終了コードなし）", args: []string{"check", "--config", "x5"}, code: 70, stderr: []string{`"error": "fail.Crashed"`}},
		{name: "宣言にない誤り", args: []string{"check", "--config", "x6"}, code: 70, stderr: []string{`"error": "boom"`}},
		{name: "終了コードのない例外は 70", args: []string{"check", "--config", "x7"}, code: 70, stderr: []string{`"error": "tool.Busy"`}, notStderr: []string{`"value"`}},
		{name: "引数を持たない例外", args: []string{"emit", "--config", "x4"}, code: 64, stdoutEmpty: true, stderr: []string{`"error": "tool.UnknownName"`}, notStderr: []string{`"value"`}, calls: "check write"},
		{name: "配線の先の例外", args: []string{"emit", "--config", "x1"}, code: 1, stderr: []string{`"error": "tool.CheckFailed"`}, calls: "check"},
		{name: "void の成功", args: []string{"serve"}, code: 0, stdoutEmpty: true, stderrEmpty: true, calls: "serve"},
		{name: "void の失敗", args: []string{"serve", "--log", "x6"}, code: 70, stderr: []string{`"error": "boom"`}},
		// 引数の解釈
		{name: "Validate の違反は 64", args: []string{"check", "--format", "xml"}, code: 64, stdoutEmpty: true, stderr: []string{prog + ": invalid input: format: pattern"}, notStderr: []string{"usage:"}, calls: "-"},
		{name: "知らないフラグ", args: []string{"check", "--bogus"}, code: 64, stderr: []string{prog + ": unknown flag --bogus", "usage: " + prog + " check"}, calls: "-"},
		{name: "値のないフラグ", args: []string{"check", "--config"}, code: 64, stderr: []string{"flag --config needs a value", "usage: " + prog + " check"}, calls: "-"},
		{name: "コマンドがない", args: nil, code: 64, stdoutEmpty: true, stderr: []string{"usage: " + prog + " <command> [flags]\n\ncommands:\n  check   調べる。ファイルは書き出さない\n  emit    調べてから、 書き出す\n  serve   標準入出力で動き続ける\n"}},
		{name: "知らないコマンド", args: []string{"bogus"}, code: 64, stderr: []string{prog + `: unknown command "bogus"`, "usage: " + prog + " <command> [flags]"}},
		{name: "help", args: []string{"help"}, code: 0, stdout: []string{"usage: " + prog + " <command> [flags]", "  emit    "}, stderrEmpty: true},
		{name: "--help", args: []string{"--help"}, code: 0, stdout: []string{"commands:"}, stderrEmpty: true},
		{name: "-h", args: []string{"-h"}, code: 0, stdout: []string{"commands:"}, stderrEmpty: true},
		{name: "コマンドの --help", args: []string{"emit", "--help"}, code: 0, stderrEmpty: true, calls: "-",
			stdout: []string{"usage: " + prog + " emit [flags] [NAMES...]\n\n調べてから、 書き出す\n\narguments:\n  NAMES...   書き出す名前（一つもなければすべて）\n\nflags:\n  --config <Path>   設定ファイルの場所 (env: TOOL_CONFIG)\n  --dry-run         書き出さずに、予定だけを表示する\n"}},
		{name: "環境変数だけのフィールドの使い方", args: []string{"check", "-h"}, code: 0, stdout: []string{"\nenvironment:\n  TOOL_PATH   探す場所\n", "--format <Format>   表示の形式"}},
		{name: "引数の後ろの -h", args: []string{"emit", "a", "-h"}, code: 0, stdout: []string{"usage: " + prog + " emit"}, calls: "-"},
		{name: "位置引数と boolean のフラグ", args: []string{"emit", "a", "b", "--dry-run"}, code: 0, stdout: []string{`"a"`, `"b"`, `"dry"`}, calls: "check write"},
		{name: "boolean のフラグは値を取らない（後ろは位置引数）", args: []string{"emit", "--dry-run", "false"}, code: 0, stdout: []string{`"false"`, `"dry"`}},
		{name: "-- 以降は位置引数", args: []string{"emit", "--", "--dry-run", "-h"}, code: 0, stdout: []string{`"--dry-run"`, `"-h"`}, notStdout: []string{`"dry"`}},
		{name: "-x は位置引数", args: []string{"emit", "-x", "-1"}, code: 0, stdout: []string{`"-x"`, `"-1"`}},
		{name: "boolean の =false", args: []string{"emit", "--dry-run=false", "x"}, code: 0, stdout: []string{`"x"`}, notStdout: []string{`"dry"`}},
		{name: "boolean に変換できない", args: []string{"emit", "--dry-run=maybe"}, code: 64, stderr: []string{`invalid value "maybe" for --dry-run: want boolean`, "usage: " + prog + " emit"}, calls: "-"},
		{name: "同じフラグが二度あれば後のもの", args: []string{"check", "--config", "a", "--config=b"}, code: 0, stdout: []string{"config=b "}},
		{name: "環境変数が代わり", args: []string{"check"}, env: map[string]string{"TOOL_CONFIG": "fromenv", "TOOL_PATH": "p1"}, code: 0, stdout: []string{"config=fromenv ", "path=p1"}},
		{name: "フラグが環境変数より優先", args: []string{"check", "--config", "flag"}, env: map[string]string{"TOOL_CONFIG": "fromenv"}, code: 0, stdout: []string{"config=flag "}},
		{name: "空の環境変数は、ない", args: []string{"check"}, env: map[string]string{"TOOL_CONFIG": "", "TOOL_PATH": ""}, code: 0, stdout: []string{"config= format= path=\""}},
		{name: "位置引数が多い（引数を取らないコマンド）", args: []string{"check", "x"}, code: 64, stderr: []string{"too many arguments: 1 given, at most 0 expected", "usage: " + prog + " check"}, calls: "-"},
		{name: "位置引数が多い（void）", args: []string{"serve", "extra"}, code: 64, stderr: []string{"too many arguments"}, calls: "-"},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			l := &toolLeaves{}
			check(t, s, runTool(l, s.env, s.args...), l.calls)
		})
	}
}

func TestDemo(t *testing.T) {
	scenarios := []scenario{
		{name: "int と float と補う値", args: []string{"add", "1", "2.5", "--verbose", "--mode", "m"}, code: 0, stdout: []string{`"total": 104`}},
		{name: "scalar のフラグと =false", args: []string{"add", "1", "--verbose=false", "--count", "3", "--mode", "m"}, code: 0, stdout: []string{`"total": 301`}},
		{name: "負の数の位置引数", args: []string{"add", "-5", "--verbose", "--mode", "m"}, code: 0, stdout: []string{`"total": 95.5`}},
		{name: "値を取らないフラグの後ろは位置引数", args: []string{"add", "--verbose", "7", "--mode", "m"}, code: 0, stdout: []string{`"total": 107.5`}},
		{name: "どちらもないフラグは零値のまま", args: []string{"add", "1", "--mode", "m"}, code: 0, stdout: []string{`"total": 101`}, stderrEmpty: true},
		{name: "scalar の Validate", args: []string{"add", "1", "--mode", "m", "--count", "-1"}, code: 64, stderr: []string{"invalid input: count: min"}, calls: "-"},
		{name: "int に変換できない位置引数", args: []string{"add", "x", "--mode", "m"}, code: 64, stderr: []string{`invalid value "x" for A: want int`, "usage: " + prog + " add"}, calls: "-"},
		{name: "int に変換できないフラグ", args: []string{"add", "1", "--mode", "m", "--count", "1.5"}, code: 64, stderr: []string{`invalid value "1.5" for --count: want int`}, calls: "-"},
		{name: "float に変換できない位置引数", args: []string{"add", "1", "y", "--mode", "m"}, code: 64, stderr: []string{`invalid value "y" for B: want float`}, calls: "-"},
		{name: "必須の位置引数がない", args: []string{"add", "--mode", "m"}, code: 64, stdoutEmpty: true, stderr: []string{prog + ": missing required argument A", "usage: " + prog + " add"}, calls: "-"},
		{name: "必須のフラグがない", args: []string{"add", "1"}, code: 64, stderr: []string{"missing required flag --mode", "usage: " + prog + " add"}, calls: "-"},
		{name: "位置引数が多い", args: []string{"add", "1", "2", "3", "--mode", "m"}, code: 64, stderr: []string{"too many arguments: 3 given, at most 2 expected"}, calls: "-"},
		{name: "環境変数だけの float", args: []string{"add", "1", "--verbose", "--mode", "m"}, env: map[string]string{"DEMO_SCALE": "2"}, code: 0, stdout: []string{`"total": 2101.5`}},
		{name: "環境変数を変換できない", args: []string{"add", "1", "--mode", "m"}, env: map[string]string{"DEMO_SCALE": "z"}, code: 64, stderr: []string{`invalid value "z" for DEMO_SCALE: want float`}, calls: "-"},
		{name: "フラグの代わりの環境変数", args: []string{"add", "1", "--verbose", "--mode", "m"}, env: map[string]string{"DEMO_LABEL": "abc"}, code: 0, stdout: []string{`"total": 104.5`}},
		{name: "フラグが環境変数より優先", args: []string{"add", "1", "--verbose", "--mode", "m", "--label", "a"}, env: map[string]string{"DEMO_LABEL": "abc"}, code: 0, stdout: []string{`"total": 102.5`}},
		{name: "@cli.exitCode の終了コード", args: []string{"add", "99", "--mode", "m"}, code: 5, stdoutEmpty: true, stderr: []string{`"error": "demo.Overflow"`}},
		{name: "終了コードのない例外", args: []string{"add", "98", "--mode", "m"}, code: 70, stderr: []string{`"error": "demo.Tired"`}},
		{name: "コマンドの使い方", args: []string{"add", "--help"}, code: 0, stderrEmpty: true, calls: "-",
			stdout: []string{"usage: " + prog + " add [flags] A [B]\n\n足す\n\narguments:\n  A   左 (required)\n  B   右\n\nflags:\n  --count <Count>    回数 (default: 1)\n  --verbose          詳しく\n  --label <string>   名札 (env: DEMO_LABEL)\n  --mode <string>    動かし方 (required)\n\nenvironment:\n  DEMO_SCALE   倍率\n  tag          札（環境変数 tag） (default: none)\n"}},
		{name: "引数のない void", args: []string{"ping"}, code: 0, stdoutEmpty: true, stderrEmpty: true, calls: "ping"},
		{name: "引数のないコマンドに位置引数", args: []string{"ping", "x"}, code: 64, stderr: []string{"too many arguments: 1 given, at most 0 expected"}, calls: "-"},
		{name: "引数のないコマンドにフラグ", args: []string{"ping", "--x"}, code: 64, stderr: []string{"unknown flag --x"}, calls: "-"},
		{name: "引数のないコマンドの使い方", args: []string{"ping", "--help"}, code: 0, stdout: []string{"usage: " + prog + " ping\n\n何もしない\n"}},
		{name: "never を返す op", args: []string{"quit"}, code: 5, stdoutEmpty: true, stderr: []string{`"error": "demo.Overflow"`}},
		{name: "全体の使い方（説明のないコマンド）", args: []string{"help"}, code: 0, stdout: []string{"commands:\n  add    足す\n  copy\n  move   動かす\n  ping   何もしない\n  quit   いつも失敗する\n"}},
		{name: "環境変数を代わりに持つ必須の位置引数がない", args: []string{"copy"}, code: 64, stderr: []string{"missing required argument FROM"}, calls: "-"},
		{name: "scalar を元にした scalar の Validate", args: []string{"copy", "a", "--steps", "-2"}, code: 64, stderr: []string{"invalid input: steps: min"}, calls: "-"},
		{name: "環境変数の boolean を変換できない", args: []string{"copy", "a"}, env: map[string]string{"DEMO_FORCE": "perhaps"}, code: 64, stderr: []string{`invalid value "perhaps" for DEMO_FORCE: want boolean`}, calls: "-"},
		{name: "copy の使い方", args: []string{"copy", "-h"}, code: 0,
			stdout: []string{"usage: " + prog + " copy [flags] FROM [TO...]\n\narguments:\n  FROM    元 (required, env: DEMO_FROM)\n  TO...   先\n\nflags:\n  --steps <Steps>   (env: DEMO_STEPS)\n  --ratio <Ratio>   (default: 0.5)\n  --force           (env: DEMO_FORCE)\n  --offset <int>    (default: -1)\n"}},
		{name: "必須の、残りを受け取る位置引数がない", args: []string{"move", "a", "b"}, code: 64, stderr: []string{"missing required argument REST", "usage: " + prog + " move"}, calls: "-"},
		{name: "move の使い方（位置の順）", args: []string{"move", "-h"}, code: 0,
			stdout: []string{"usage: " + prog + " move [flags] [FROM] [TO] REST...\n\n動かす\n\narguments:\n  FROM      元\n  TO        先\n  REST...   残り (required, env: DEMO_REST)\n\nflags:\n  --keep             (default: true)\n  --suffix <Name>    (default: ~)\n  --weight <float>   (default: 2)\n"}},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			l := &demoLeaves{}
			check(t, s, runDemo(l, s.env, s.args...), l.calls)
		})
	}
}

// TestAddInput は、値が与えられなかったときの三つの扱いと、名前を省いたフラグと環境変数: @cli.default のフィールドはその値、
// どちらもないフィールドは零値のまま（省略可能なら nil）。名前を省いた @cli.flag は --<フィールドの名前>、@cli.env は
// フィールドの名前の環境変数。
func TestAddInput(t *testing.T) {
	two, label := 2.0, "l"
	for name, c := range map[string]struct {
		args []string
		env  map[string]string
		want demo.AddInput
	}{
		"与えられなかったフィールド": {
			args: []string{"add", "1", "--mode", "m"},
			want: demo.AddInput{A: 1, Count: 1, Mode: "m", Tag: "none"},
		},
		"すべて与える": {
			args: []string{"add", "1", "2", "--count=7", "--verbose", "--label", "l", "--mode", "m"}, env: map[string]string{"DEMO_SCALE": "2", "tag": "t"},
			want: demo.AddInput{A: 1, B: &two, Count: 7, Verbose: true, Label: &label, Scale: &two, Mode: "m", Tag: "t"},
		},
		"空の環境変数は、ない（補う値になる）": {
			args: []string{"add", "1", "--mode", "m"}, env: map[string]string{"tag": "", "DEMO_SCALE": ""},
			want: demo.AddInput{A: 1, Count: 1, Mode: "m", Tag: "none"},
		},
	} {
		l := &demoLeaves{}
		if r := runDemo(l, c.env, c.args...); r.code != 0 || r.stderr != "" {
			t.Errorf("%s: %+v", name, r)
		}
		if !reflect.DeepEqual(l.add, c.want) {
			t.Errorf("%s: 入力 = %+v, want %+v", name, l.add, c.want)
		}
	}
}

// TestCopyInput は、値の変換と、位置引数の当て方: scalar は元の型で変換してから scalar の型にし、array は残りすべて。
// 与えられなかったフィールドは、@cli.default の値か、零値のまま。
func TestCopyInput(t *testing.T) {
	ptr := func(v demo.Steps) *demo.Steps { return &v }
	yes, no := demo.Flag(true), demo.Flag(false)
	for name, c := range map[string]struct {
		args []string
		env  map[string]string
		want demo.CopyInput
	}{
		"位置引数とフラグ": {
			args: []string{"copy", "a", "b", "c", "--steps", "3", "--ratio=2", "--force", "--offset", "-4"},
			want: demo.CopyInput{From: "a", Steps: ptr(3), Ratio: 2, Force: &yes, Offset: -4, To: []demo.Name{"b", "c"}},
		},
		"補う値と、0 個の array": {
			args: []string{"copy", "a"},
			want: demo.CopyInput{From: "a", Ratio: 0.5, Offset: -1},
		},
		"環境変数": {
			args: []string{"copy"}, env: map[string]string{"DEMO_FROM": "e", "DEMO_STEPS": "7", "DEMO_FORCE": "false"},
			want: demo.CopyInput{From: "e", Steps: ptr(7), Ratio: 0.5, Force: &no, Offset: -1},
		},
		"位置引数が環境変数より優先": {
			args: []string{"copy", "a", "b"}, env: map[string]string{"DEMO_FROM": "e"},
			want: demo.CopyInput{From: "a", Ratio: 0.5, Offset: -1, To: []demo.Name{"b"}},
		},
	} {
		l := &demoLeaves{}
		if r := runDemo(l, c.env, c.args...); r.code != 0 || r.stdout != "\"copied\"\n" || r.stderr != "" {
			t.Errorf("%s: %+v", name, r)
		}
		if !reflect.DeepEqual(l.copy, c.want) {
			t.Errorf("%s: 入力 = %+v, want %+v", name, l.copy, c.want)
		}
	}
}

// TestMoveInput は、位置（@cli.arg の引数で決まり、フィールドの宣言の順ではない）と、残りを受け取る array の @cli.required
// （位置引数が一つは要る。環境変数が代わりなら、その一つだけが要素）と、型の違う補う値（真偽値、scalar の文字列、int の
// リテラルを float に）。
func TestMoveInput(t *testing.T) {
	for name, c := range map[string]struct {
		args []string
		env  map[string]string
		want demo.MoveInput
	}{
		"位置の順と、補う値": {
			args: []string{"move", "a", "b", "c", "d"},
			want: demo.MoveInput{From: "a", To: "b", Rest: []string{"c", "d"}, Keep: true, Suffix: "~", Weight: 2},
		},
		"フラグが補う値より優先": {
			args: []string{"move", "a", "b", "c", "--keep=false", "--suffix", ".bak", "--weight", "1.5"},
			want: demo.MoveInput{From: "a", To: "b", Rest: []string{"c"}, Suffix: ".bak", Weight: 1.5},
		},
		"残りの代わりの環境変数": {
			args: []string{"move", "a", "b"}, env: map[string]string{"DEMO_REST": "r"},
			want: demo.MoveInput{From: "a", To: "b", Rest: []string{"r"}, Keep: true, Suffix: "~", Weight: 2},
		},
		"足りない位置引数は零値のまま": {
			args: []string{"move", "a"}, env: map[string]string{"DEMO_REST": "r"},
			want: demo.MoveInput{From: "a", Rest: []string{"r"}, Keep: true, Suffix: "~", Weight: 2},
		},
		"位置引数が環境変数より優先": {
			args: []string{"move", "a", "b", "c"}, env: map[string]string{"DEMO_REST": "r"},
			want: demo.MoveInput{From: "a", To: "b", Rest: []string{"c"}, Keep: true, Suffix: "~", Weight: 2},
		},
	} {
		l := &demoLeaves{}
		if r := runDemo(l, c.env, c.args...); r.code != 0 || r.stdout != "\"moved\"\n" || r.stderr != "" {
			t.Errorf("%s: %+v", name, r)
		}
		if !reflect.DeepEqual(l.move, c.want) {
			t.Errorf("%s: 入力 = %+v, want %+v", name, l.move, c.want)
		}
	}
}

// bareLeaves は bare の葉の op。
type bareLeaves struct{ calls int }

func (l *bareLeaves) Noop(ctx context.Context) error {
	l.calls++
	return nil
}

// TestBare: 入力も例外も持たないコマンドだけの名前空間でも、引数の解釈は同じ（位置引数とフラグは誤り、--help は使い方）。
func TestBare(t *testing.T) {
	for _, c := range []struct {
		args  []string
		code  int
		calls int
		want  string // stdout か stderr に含まれるべき部分文字列
	}{
		{[]string{"noop"}, 0, 1, ""},
		{[]string{"noop", "x"}, 64, 0, "too many arguments: 1 given, at most 0 expected"},
		{[]string{"noop", "--x"}, 64, 0, "unknown flag --x"},
		{[]string{"noop", "-h"}, 0, 0, "usage: " + prog + " noop\n\n何もしない\n"},
	} {
		l := &bareLeaves{}
		var out, errOut bytes.Buffer
		code := barecli.Main(context.Background(), bare.New(l), barecli.JSONPresenter{}, c.args, nil, &out, &errOut)
		if code != c.code || l.calls != c.calls || !strings.Contains(out.String()+errOut.String(), c.want) {
			t.Errorf("%v: code=%d calls=%d stdout=%q stderr=%q", c.args, code, l.calls, out.String(), errOut.String())
		}
	}
}

// recorder は、呼ばれ方を覚える Presenter。err があれば、それを返す。
type recorder struct {
	command string
	input   any
	result  any
	failure *toolcli.Failure
	err     error
}

func (r *recorder) Present(w io.Writer, command string, input any, result any) error {
	r.command, r.input, r.result = command, input, result
	fmt.Fprint(w, "presented")
	return r.err
}

func (r *recorder) PresentError(w io.Writer, command string, input any, f toolcli.Failure) error {
	r.command, r.input, r.failure = command, input, &f
	fmt.Fprint(w, "failed")
	return r.err
}

// TestPresenter は「実行と表示」: Present は標準出力と、コマンドの名前、入力、結果を受け取る。PresentError は標準エラーと Failure
// （例外の FQN、運ぶ値、op が返した error そのもの）を受け取る。Present / PresentError 自体の失敗は、標準エラーに出して 70。
// ctx は op にそのまま渡る。env が nil なら、環境変数を見ない。
func TestPresenter(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	invoke := func(p toolcli.Presenter, env func(string) string, args ...string) (int, string, string, *toolLeaves) {
		l := &toolLeaves{}
		var out, errOut bytes.Buffer
		code := toolcli.Main(ctx, tool.New(l), p, args, env, &out, &errOut)
		return code, out.String(), errOut.String(), l
	}
	// 成功
	rec := &recorder{}
	code, out, errOut, l := invoke(rec, nil, "check", "--config", "c")
	if code != 0 || out != "presented" || errOut != "" {
		t.Errorf("成功: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if l.ctx == nil || l.ctx.Value(ctxKey{}) != "marker" {
		t.Error("op に、Main の ctx が渡っていない")
	}
	in, ok := rec.input.(tool.CheckInput)
	if rec.command != "check" || !ok || str(in.Config) != "c" {
		t.Errorf("Present の command=%q input=%#v", rec.command, rec.input)
	}
	if res, ok := rec.result.(tool.CheckResult); !ok || len(res.Warnings) != 1 {
		t.Errorf("Present の result=%#v", rec.result)
	}
	// void の op の result は nil
	rec = &recorder{}
	if code, _, _, _ := invoke(rec, nil, "serve"); code != 0 || rec.result != nil || rec.command != "serve" {
		t.Errorf("void: code=%d result=%#v", code, rec.result)
	}
	if _, ok := rec.input.(tool.ServeInput); !ok {
		t.Errorf("void: input=%#v", rec.input)
	}
	// 例外: Failure の Name、Value、Err
	rec = &recorder{}
	code, out, errOut, _ = invoke(rec, nil, "check", "--config", "x2")
	if code != 2 || out != "" || errOut != "failed" || rec.failure == nil {
		t.Fatalf("例外: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	// Value は、引数を持つ例外の、値そのもの（包んだ error ではなく、見分けた例外の値）
	if v, ok := rec.failure.Value.(tool.ConfigInvalid); rec.failure.Name != "tool.ConfigInvalid" || !ok || v.File != "x2" || v.Notes[0].Code != "CFG-12" || v.Hint != nil {
		t.Errorf("Failure = %+v", rec.failure)
	}
	if rec.failure.Err == nil || rec.failure.Err.Error() != "wrapped: tool.ConfigInvalid" {
		t.Errorf("Failure.Err = %v（op が返した error そのもの）", rec.failure.Err)
	}
	// 引数を持たない例外の Value は nil。宣言にない誤りは Name が空で Err だけ
	rec = &recorder{}
	if code, _, _, _ = invoke(rec, nil, "check", "--config", "x7"); code != 70 || rec.failure.Name != "tool.Busy" || rec.failure.Value != nil || rec.failure.Err == nil {
		t.Errorf("引数を持たない例外: code=%d Failure=%+v", code, rec.failure)
	}
	rec = &recorder{}
	if code, _, _, _ = invoke(rec, nil, "check", "--config", "x6"); code != 70 || rec.failure.Name != "" || rec.failure.Value != nil || rec.failure.Err.Error() != "boom" {
		t.Errorf("宣言にない誤り: code=%d Failure=%+v", code, rec.failure)
	}
	// Present / PresentError 自体の失敗は 70
	rec = &recorder{err: errors.New("disk full")}
	if code, _, errOut, _ = invoke(rec, nil, "check"); code != 70 || errOut != prog+": disk full\n" {
		t.Errorf("Present の失敗: code=%d stderr=%q", code, errOut)
	}
	rec = &recorder{err: errors.New("disk full")}
	if code, _, errOut, _ = invoke(rec, nil, "check", "--config", "x1"); code != 70 || errOut != "failed"+prog+": disk full\n" {
		t.Errorf("PresentError の失敗: code=%d stderr=%q", code, errOut)
	}
	// env が nil でも動く（環境変数を見ない）
	if code, out, _, l = invoke(toolcli.JSONPresenter{}, nil, "check"); code != 0 || !strings.Contains(out, "config= ") || l.check.Config != nil {
		t.Errorf("env が nil: code=%d stdout=%q", code, out)
	}
}

// TestJSONPresenter は「実行と表示」の JSONPresenter の形。
func TestJSONPresenter(t *testing.T) {
	p := toolcli.JSONPresenter{}
	var b bytes.Buffer
	if err := p.Present(&b, "check", nil, tool.EmitResult{Written: []tool.Path{"a"}}); err != nil || b.String() != "{\n  \"written\": [\n    \"a\"\n  ],\n  \"removed\": null\n}\n" {
		t.Errorf("Present = %q, %v", b.String(), err)
	}
	b.Reset()
	if err := p.Present(&b, "serve", nil, nil); err != nil || b.Len() != 0 {
		t.Errorf("nil の result: %q, %v", b.String(), err)
	}
	hint := "h"
	for want, f := range map[string]toolcli.Failure{
		// value は、例外の引数の名前をキーにしたオブジェクト（例外の値そのものの JSON）。キーの並びは、宣言の引数の順
		"{\n  \"error\": \"tool.CheckFailed\",\n  \"value\": {\n    \"notes\": null\n  }\n}\n":                                               {Name: "tool.CheckFailed", Value: tool.CheckFailed{}, Err: errors.New("x")},
		"{\n  \"error\": \"tool.ConfigInvalid\",\n  \"value\": {\n    \"file\": \"f\",\n    \"notes\": null,\n    \"hint\": \"h\"\n  }\n}\n": {Name: "tool.ConfigInvalid", Value: tool.ConfigInvalid{File: "f", Hint: &hint}, Err: errors.New("x")},
		"{\n  \"error\": \"tool.UnknownName\"\n}\n":                                                                                          {Name: "tool.UnknownName", Err: errors.New("x")},
		"{\n  \"error\": \"boom\"\n}\n": {Err: errors.New("boom")},
	} {
		b.Reset()
		if err := p.PresentError(&b, "check", nil, f); err != nil || b.String() != want {
			t.Errorf("PresentError(%+v) = %q, %v\nwant %q", f, b.String(), err, want)
		}
	}
}
