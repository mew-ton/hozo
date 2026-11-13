package plugin

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// echoExe は TestMain で testdata/echo-plugin から作った実行ファイル。
var echoExe string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hozo-echo-plugin")
	if err != nil {
		panic(err)
	}
	echoExe = filepath.Join(dir, "echo-plugin")
	if runtime.GOOS == "windows" {
		echoExe += ".exe"
	}
	build := exec.Command("go", "build", "-o", echoExe, "./testdata/echo-plugin")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("building echo-plugin: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// echoManifest は echoExe を絶対パスで呼ぶマニフェスト。
func echoManifest(args ...string) *Manifest {
	return &Manifest{Name: "echo", Command: append([]string{echoExe}, args...), Protocol: Protocol}
}

type echoReq struct {
	Mode   string   `json:"mode"`
	Stderr []string `json:"stderr,omitempty"`
	Hello  string   `json:"hello,omitempty"`
	N      int      `json:"n,omitempty"`
}

func TestRunEcho(t *testing.T) {
	root := t.TempDir()
	var stderr bytes.Buffer
	var resp echoReq
	req := echoReq{Mode: "echo", Hello: "world", N: 3, Stderr: []string{"line one", "line two"}}
	err := Runner{Root: root, Stderr: &stderr}.Run(echoManifest(), root, req, &resp)
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

// docs/plugin-protocol.md「起動」: 作業ディレクトリはルート、HOZO_PROJECT_ROOT と HOZO_PLUGIN_DIR を足す。
func TestRunEnv(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "plugins", "echo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Cwd, Root, Dir string
		Args           []string
	}
	m := echoManifest("--flag", "x")
	if err := (Runner{Root: root, Env: []string{"PATH=" + os.Getenv("PATH")}}).Run(m, dir, echoReq{Mode: "env"}, &resp); err != nil {
		t.Fatal(err)
	}
	// TempDir はシンボリックリンクを含みうるので EvalSymlinks で比べる
	eq := func(a, b string) bool {
		ra, _ := filepath.EvalSymlinks(a)
		rb, _ := filepath.EvalSymlinks(b)
		return ra == rb
	}
	if !eq(resp.Cwd, root) || !eq(resp.Root, root) || !eq(resp.Dir, dir) {
		t.Errorf("cwd=%q root=%q dir=%q (want root %q, dir %q)", resp.Cwd, resp.Root, resp.Dir, root, dir)
	}
	if !filepath.IsAbs(resp.Root) || !filepath.IsAbs(resp.Dir) {
		t.Errorf("env vars must be absolute: %q %q", resp.Root, resp.Dir)
	}
	if !reflect.DeepEqual(resp.Args, []string{"--flag", "x"}) {
		t.Errorf("args = %v", resp.Args)
	}
}

// docs/plugin-protocol.md「マニフェスト」: 実行ファイルにパスの区切りがあればマニフェストのディレクトリからの相対パス、
// なければ PATH から探す。
func TestRunCommandResolution(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "plugins", "echo")
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(echoExe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", filepath.Base(echoExe)), data, 0o755); err != nil {
		t.Fatal(err)
	}
	var resp echoReq
	rel := &Manifest{Name: "echo", Command: []string{"./bin/" + filepath.Base(echoExe)}}
	if err := (Runner{Root: root}).Run(rel, dir, echoReq{Mode: "echo", Hello: "rel"}, &resp); err != nil || resp.Hello != "rel" {
		t.Errorf("relative: err=%v resp=%+v", err, resp)
	}
	t.Setenv("PATH", filepath.Join(dir, "bin")+string(filepath.ListSeparator)+os.Getenv("PATH"))
	byName := &Manifest{Name: "echo", Command: []string{filepath.Base(echoExe)}}
	if err := (Runner{Root: root}).Run(byName, dir, echoReq{Mode: "echo", Hello: "path"}, &resp); err != nil || resp.Hello != "path" {
		t.Errorf("PATH: err=%v resp=%+v", err, resp)
	}
}

// docs/plugin-protocol.md「起動」の終了コードの表。
func TestRunExitCodes(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		mode     string
		check    func(t *testing.T, err error)
		exitCode int
	}{
		{"crash", func(t *testing.T, err error) {
			var ce *CrashedError
			if !errors.Is(err, ErrCrashed) || !errors.As(err, &ce) || ce.ExitCode != 1 || ce.Plugin != "echo" {
				t.Errorf("crash: %v", err)
			}
			if errors.Is(err, ErrMalformedOutput) {
				t.Error("crash must not be MalformedOutput")
			}
		}, 1},
		{"garbage", func(t *testing.T, err error) {
			var me *MalformedOutputError
			if !errors.Is(err, ErrMalformedOutput) || !errors.As(err, &me) || me.ExitCode != 0 {
				t.Errorf("garbage: %v", err)
			}
		}, 0},
		{"trailing", func(t *testing.T, err error) {
			if !errors.Is(err, ErrMalformedOutput) {
				t.Errorf("trailing: %v", err)
			}
		}, 0},
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
		}, 2},
		{"unsupported", func(t *testing.T, err error) {
			var pe *PluginError
			if !errors.As(err, &pe) || pe.Code != CodeUnsupported {
				t.Fatalf("unsupported: %v", err)
			}
			if d := pe.Diagnostic(""); d.Code != "MAN-5" {
				t.Errorf("diagnostic = %+v", d)
			}
		}, 2},
		{"exit2-other", func(t *testing.T, err error) {
			var me *MalformedOutputError
			if !errors.As(err, &me) || me.ExitCode != 2 {
				t.Errorf("exit2-other: %v", err)
			}
		}, 2},
		{"exit2-garbage", func(t *testing.T, err error) {
			var me *MalformedOutputError
			if !errors.As(err, &me) || me.ExitCode != 2 {
				t.Errorf("exit2-garbage: %v", err)
			}
		}, 2},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			var resp map[string]any
			err := Runner{Root: root, Stderr: &bytes.Buffer{}}.Run(echoManifest(), root, echoReq{Mode: c.mode}, &resp)
			if err == nil {
				t.Fatal("want error")
			}
			c.check(t, err)
		})
	}
}

// 応答の形が違う（files が配列でない）のも MalformedOutput。
func TestRunWrongShape(t *testing.T) {
	root := t.TempDir()
	var resp struct {
		Hello []string `json:"hello"`
	}
	err := Runner{Root: root}.Run(echoManifest(), root, echoReq{Mode: "echo", Hello: "not-a-list"}, &resp)
	if !errors.Is(err, ErrMalformedOutput) {
		t.Errorf("err = %v", err)
	}
}

func TestRunCannotStart(t *testing.T) {
	root := t.TempDir()
	m := &Manifest{Name: "ghost", Command: []string{filepath.Join(root, "does-not-exist")}}
	err := Runner{Root: root}.Run(m, root, echoReq{}, &map[string]any{})
	var ce *CrashedError
	if !errors.Is(err, ErrCrashed) || !errors.As(err, &ce) || ce.ExitCode != -1 || ce.Err == nil {
		t.Errorf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("message should name the plugin: %v", err)
	}
}

func TestRunPackageFunc(t *testing.T) {
	root := t.TempDir()
	var resp echoReq
	if err := Run(root, echoManifest(), root, echoReq{Mode: "echo", Hello: "pkg"}, &resp); err != nil || resp.Hello != "pkg" {
		t.Errorf("err=%v resp=%+v", err, resp)
	}
}

func TestPrefixLines(t *testing.T) {
	var buf bytes.Buffer
	prefixLines(&buf, strings.NewReader("a\nb\n\nc"), "[p] ")
	if got, want := buf.String(), "[p] a\n[p] b\n[p] \n[p] c\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	buf.Reset()
	prefixLines(&buf, strings.NewReader(""), "[p] ")
	if buf.Len() != 0 {
		t.Errorf("empty input wrote %q", buf.String())
	}
}
