package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// echoWasm は TestMain で testdata/echo-plugin から作った WASM。
var echoWasm string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hozo-echo-plugin")
	if err != nil {
		panic(err)
	}
	echoWasm = filepath.Join(dir, "echo-plugin.wasm")
	build := exec.Command("go", "build", "-o", echoWasm, ".")
	build.Dir = filepath.Join("testdata", "echo-plugin")
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("building echo-plugin: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type echoReq struct {
	Mode   string   `json:"mode"`
	Stderr []string `json:"stderr,omitempty"`
	Hello  string   `json:"hello,omitempty"`
	N      int      `json:"n,omitempty"`
	Paths  []string `json:"paths,omitempty"`
	Addr   string   `json:"addr,omitempty"`
}

func TestRunEcho(t *testing.T) {
	var stderr bytes.Buffer
	var resp echoReq
	req := echoReq{Mode: "echo", Hello: "world", N: 3, Stderr: []string{"line one", "line two"}}
	err := Runner{Stderr: &stderr}.Run("echo", echoWasm, req, &resp)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resp, req) {
		t.Errorf("resp = %+v, want %+v", resp, req)
	}
	// docs/plugin-protocol.md「起動」: 標準エラーは各行の先頭に [<プラグイン名>] を付けて流す
	if got, want := stderr.String(), "[echo] line one\n[echo] line two\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// probeResp は、echo-plugin の mode "probe" の応答。
type probeResp struct {
	Args  []string          `json:"args"`
	Env   []string          `json:"env"`
	Cwd   string            `json:"cwd"`
	Open  map[string]string `json:"open"`
	List  map[string]string `json:"list"`
	Write map[string]string `json:"write"`
	Dial  string            `json:"dial"`
	Now   int64             `json:"now"`
	Mono  int64             `json:"mono"`
	Rand  string            `json:"rand"`
	MRand uint64            `json:"mrand"`
}

// docs/plugin-protocol.md「起動」の、渡さないものの表: プラグインは、ファイルを開けず、環境変数も、引数も、作業ディレクトリも
// 見えず、ネットワークに、つなげない・待ち受けられない。
func TestRunSandbox(t *testing.T) {
	t.Setenv("HOZO_TEST_SECRET", "s3cret")
	// 開けるはずのファイル: 絶対パスの、一時ディレクトリのファイル。hozoc の作業ディレクトリのファイル（相対と、絶対）
	tmp := t.TempDir()
	secret := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(secret, []byte("s3cret"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// つなげるはずの相手: このテストが待ち受けているソケット
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	accepted := make(chan struct{}, 4)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			accepted <- struct{}{}
			c.Close()
		}
	}()

	paths := []string{secret, tmp, "run_test.go", filepath.Join(wd, "run_test.go"), ".", "/", wd, "/etc/hosts", "/tmp"}
	var resp probeResp
	if err := (Runner{Stderr: &bytes.Buffer{}}).Run("echo", echoWasm, echoReq{Mode: "probe", Paths: paths, Addr: l.Addr().String()}, &resp); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if resp.Open[p] == "" {
			t.Errorf("プラグインが、%s を開けた", p)
		}
		if resp.List[p] == "" {
			t.Errorf("プラグインが、%s を一覧できた", p)
		}
		if resp.Write[p] == "" {
			t.Errorf("プラグインが、%s の下に、ファイルを作れた", p)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, "written-by-plugin")); err == nil {
		t.Error("プラグインが作ったファイルが、ある")
	}
	if len(resp.Env) != 0 {
		t.Errorf("プラグインに、環境変数が見えた: %v", resp.Env)
	}
	// 引数は、ない（プログラムの名前も、ない）
	for _, a := range resp.Args {
		if a != "" {
			t.Errorf("プラグインに、引数が見えた: %q", resp.Args)
		}
	}
	if resp.Cwd != "" && resp.Cwd != "/" {
		t.Errorf("プラグインに、作業ディレクトリが見えた: %q", resp.Cwd)
	}
	if resp.Dial == "" {
		t.Error("プラグインが、ネットワークに、つなげた")
	}
	select {
	case <-accepted:
		t.Error("プラグインからの接続が、届いた")
	case <-time.After(50 * time.Millisecond):
	}
	// 時計は、今の時刻ではない（決まった値から始まる、にせもの）
	if d := time.Since(time.Unix(0, resp.Now)); d < 24*time.Hour && d > -24*time.Hour {
		t.Errorf("プラグインに、今の時刻が見えた: %v", time.Unix(0, resp.Now))
	}
}

// lineSignal は、一行、書かれたら、それを ch に送る Writer。
type lineSignal struct{ ch chan string }

func (w lineSignal) Write(p []byte) (int, error) {
	select {
	case w.ch <- string(p):
	default:
	}
	return len(p), nil
}

// docs/plugin-protocol.md「起動」: プラグインは、待ち受けられない。Go の WASI のランタイムは、プログラムの中だけの、にせものの
// ネットワークを持つので、プラグインの側では、待ち受けに「成功」しうる。外から、そのアドレスに、つなげないことを確かめる。
func TestRunSandboxListen(t *testing.T) {
	// 空いているアドレスを、一つ決める
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	said := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Runner{Stderr: lineSignal{said}, Context: ctx}.Run("echo", echoWasm, echoReq{Mode: "listen", Addr: addr}, &map[string]any{})
	}()
	select {
	case line := <-said:
		t.Logf("プラグインの側: %s", strings.TrimSpace(line))
	case err := <-done:
		t.Fatalf("待ち受ける前に、終わった: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("プラグインが、何も言わない")
	}
	// プラグインは、まだ、動いている。外からは、つなげない
	if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		conn.Close()
		t.Errorf("プラグインが待ち受けたアドレス %s に、外から、つなげた", addr)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// docs/plugin-protocol.md「起動」: 同じ WASM に、同じ要求を渡せば、同じ答えが返る（時計と乱数は、決まった値）。
func TestRunSameAnswer(t *testing.T) {
	req := echoReq{Mode: "probe", Addr: "127.0.0.1:9"}
	var answers [3]json.RawMessage
	for i := range answers {
		if err := (Runner{Stderr: &bytes.Buffer{}}).Run("echo", echoWasm, req, &answers[i]); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			time.Sleep(20 * time.Millisecond) // 本物の時計が、進んでも
		}
	}
	for i := 1; i < len(answers); i++ {
		if !bytes.Equal(answers[0], answers[i]) {
			t.Errorf("%d 度目の答えが、違う:\n%s\n%s", i+1, answers[0], answers[i])
		}
	}
	var resp probeResp
	if err := json.Unmarshal(answers[0], &resp); err != nil || resp.Rand == "" || strings.HasPrefix(resp.Rand, "error") {
		t.Errorf("乱数を読めていない: %q (%v)", resp.Rand, err)
	}
}

// docs/plugin-protocol.md「起動」: 動いているプラグインを、途中で、止められる。止めたとき、Run は、Context の誤りを返す。
func TestRunStop(t *testing.T) {
	for _, mode := range []string{"spin", "sleep"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				done <- Runner{Stderr: &bytes.Buffer{}, Context: ctx}.Run("echo", echoWasm, echoReq{Mode: mode}, &map[string]any{})
			}()
			select {
			case err := <-done:
				t.Fatalf("止める前に、終わった: %v", err)
			case <-time.After(300 * time.Millisecond):
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("err = %v, want context.Canceled", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("止めても、終わらない")
			}
			// 止めた後も、次の起動は、動く
			var resp echoReq
			if err := (Runner{}).Run("echo", echoWasm, echoReq{Mode: "echo", Hello: "again"}, &resp); err != nil || resp.Hello != "again" {
				t.Errorf("止めた後の起動: err=%v resp=%+v", err, resp)
			}
		})
	}
	// はじめから、やめさせてある Context では、動かさない
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (Runner{Context: ctx}).Run("echo", echoWasm, echoReq{Mode: "echo"}, &map[string]any{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// docs/plugin-protocol.md「起動」の終了コードの表。
func TestRunExitCodes(t *testing.T) {
	cases := []struct {
		mode  string
		check func(t *testing.T, err error)
	}{
		{"crash", func(t *testing.T, err error) {
			var ce *CrashedError
			if !errors.Is(err, ErrCrashed) || !errors.As(err, &ce) || ce.ExitCode != 1 || ce.Plugin != "echo" {
				t.Errorf("crash: %v", err)
			}
			if errors.Is(err, ErrMalformedOutput) {
				t.Error("crash must not be MalformedOutput")
			}
		}},
		{"garbage", func(t *testing.T, err error) {
			var me *MalformedOutputError
			if !errors.Is(err, ErrMalformedOutput) || !errors.As(err, &me) || me.ExitCode != 0 {
				t.Errorf("garbage: %v", err)
			}
		}},
		{"trailing", func(t *testing.T, err error) {
			if !errors.Is(err, ErrMalformedOutput) {
				t.Errorf("trailing: %v", err)
			}
		}},
		{"protocol", func(t *testing.T, err error) {
			var pe *PluginError
			if !errors.As(err, &pe) || pe.Code != CodeProtocol || pe.Message != "want 9.9" {
				t.Fatalf("protocol: %v", err)
			}
			if d := pe.Diagnostic("m.toml"); d.Code != "MAN-2" || d.File != "m.toml" {
				t.Errorf("diagnostic = %+v", d)
			}
			if errors.Is(err, ErrCrashed) || errors.Is(err, ErrMalformedOutput) {
				t.Error("protocol error must not be Crashed/MalformedOutput")
			}
		}},
		{"unsupported", func(t *testing.T, err error) {
			var pe *PluginError
			if !errors.As(err, &pe) || pe.Code != CodeUnsupported {
				t.Fatalf("unsupported: %v", err)
			}
			if d := pe.Diagnostic(""); d.Code != "MAN-5" {
				t.Errorf("diagnostic = %+v", d)
			}
		}},
		{"exit2-other", func(t *testing.T, err error) {
			var me *MalformedOutputError
			if !errors.As(err, &me) || me.ExitCode != 2 {
				t.Errorf("exit2-other: %v", err)
			}
		}},
		{"exit2-garbage", func(t *testing.T, err error) {
			var me *MalformedOutputError
			if !errors.As(err, &me) || me.ExitCode != 2 {
				t.Errorf("exit2-garbage: %v", err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			var resp map[string]any
			err := Runner{Stderr: &bytes.Buffer{}}.Run("echo", echoWasm, echoReq{Mode: c.mode}, &resp)
			if err == nil {
				t.Fatal("want error")
			}
			c.check(t, err)
		})
	}
}

// 応答の形が違う（files が配列でない）のも MalformedOutput。
func TestRunWrongShape(t *testing.T) {
	var resp struct {
		Hello []string `json:"hello"`
	}
	err := Runner{}.Run("echo", echoWasm, echoReq{Mode: "echo", Hello: "not-a-list"}, &resp)
	if !errors.Is(err, ErrMalformedOutput) {
		t.Errorf("err = %v", err)
	}
}

// docs/plugin-protocol.md「起動」: wasm のファイルがない、WASM として読めない、は hozo.compiler.Crashed。
func TestRunCannotStart(t *testing.T) {
	root := t.TempDir()
	notWasm := filepath.Join(root, "not.wasm")
	if err := os.WriteFile(notWasm, []byte("#!/bin/sh\necho '{}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "does-not-exist.wasm"), notWasm} {
		err := Runner{}.Run("ghost", path, echoReq{}, &map[string]any{})
		var ce *CrashedError
		if !errors.Is(err, ErrCrashed) || !errors.As(err, &ce) || ce.ExitCode != -1 || ce.Err == nil {
			t.Errorf("%s: err = %v", path, err)
		}
		if err == nil || !strings.Contains(err.Error(), "ghost") || !strings.Contains(err.Error(), filepath.Base(path)) {
			t.Errorf("誤りは、プラグインと、ファイルを、名指しする: %v", err)
		}
	}
}

// docs/plugin-protocol.md「起動」: 機械の言葉に直したものは、言った置き場の下の wasm/（途中で止められる形は wasm-stoppable/）に
// 置く。置き場を作れなければ、覚えずに動く。
func TestRunCacheDir(t *testing.T) {
	dir := t.TempDir()
	var resp echoReq
	if err := (Runner{CacheDir: dir}).Run("echo", echoWasm, echoReq{Mode: "echo", Hello: "cached"}, &resp); err != nil || resp.Hello != "cached" {
		t.Fatalf("err=%v resp=%+v", err, resp)
	}
	hasFiles := func(sub string) bool {
		found := false
		_ = filepath.WalkDir(filepath.Join(dir, sub), func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				found = true
			}
			return nil
		})
		return found
	}
	if !hasFiles("wasm") || hasFiles("wasm-stoppable") {
		t.Errorf("止める口のない起動: wasm=%v wasm-stoppable=%v", hasFiles("wasm"), hasFiles("wasm-stoppable"))
	}
	// 止める口を渡した起動は、途中で止められる形で、置き場が、別（二つの形を、混ぜない）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := (Runner{CacheDir: dir, Context: ctx}).Run("echo", echoWasm, echoReq{Mode: "echo", Hello: "stoppable"}, &resp); err != nil || resp.Hello != "stoppable" {
		t.Fatalf("err=%v resp=%+v", err, resp)
	}
	if !hasFiles("wasm-stoppable") {
		t.Errorf("%s の下に、覚えたものが、ない", filepath.Join(dir, "wasm-stoppable"))
	}
	// 置き場に、ふつうのファイルを言えば、ディレクトリを作れない: 覚えずに動く
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (Runner{CacheDir: file}).Run("echo", echoWasm, echoReq{Mode: "echo", Hello: "uncached"}, &resp); err != nil || resp.Hello != "uncached" {
		t.Errorf("置き場を作れないとき: err=%v resp=%+v", err, resp)
	}
}

// MountForTest（docs/testing.md「環境からの独立」）: 言った WASM にだけ、言ったディレクトリだけが、"/" として見える。
// 取り消せば、見えない。
func TestMountForTest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mounted := filepath.Join(dir, "plugin.wasm")
	data, err := os.ReadFile(echoWasm)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mounted, data, 0o644); err != nil {
		t.Fatal(err)
	}
	probe := func(wasm string) probeResp {
		t.Helper()
		var resp probeResp
		if err := (Runner{Stderr: &bytes.Buffer{}}).Run("echo", wasm, echoReq{Mode: "probe", Paths: []string{"/marker", dir}, Addr: "127.0.0.1:9"}, &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}
	undo := MountForTest(mounted, dir)
	if got := probe(mounted); got.Open["/marker"] != "" || got.Open[dir] == "" {
		t.Errorf("見せたディレクトリ: /marker=%q、外のパス %s=%q", got.Open["/marker"], dir, got.Open[dir])
	}
	// 同じ中身でも、別のファイルの WASM には、見えない
	if got := probe(echoWasm); got.Open["/marker"] == "" {
		t.Error("言っていない WASM に、ディレクトリが見えた")
	}
	undo()
	if got := probe(mounted); got.Open["/marker"] == "" {
		t.Error("取り消した後も、ディレクトリが見えた")
	}
}

func TestPrefixWriter(t *testing.T) {
	var buf bytes.Buffer
	w := &prefixWriter{w: &buf, prefix: "[p] "}
	for _, chunk := range []string{"a\nb", "\n\n", "c"} {
		if n, err := w.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	w.flush()
	if got, want := buf.String(), "[p] a\n[p] b\n[p] \n[p] c\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	buf.Reset()
	(&prefixWriter{w: &buf, prefix: "[p] "}).flush()
	if buf.Len() != 0 {
		t.Errorf("empty input wrote %q", buf.String())
	}
}
