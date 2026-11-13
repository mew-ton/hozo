package gocli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// このファイルは、生成した cli.go を名前空間のパッケージの手書きの代わり（docs/emit-go.md「op」「exception」
// 「model の Validate」の形）と一緒に一時的なモジュールに置き、go vet を通し、小さな main をビルドして
// 実際に動かす。go の道具がなければ飛ばす。

func goTool(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	return path
}

// hozocStub は名前空間 hozoc のパッケージの、既定の出力が生成するはずの形（必要な部分だけ）。
const hozocStub = `package hozoc

import (
	"regexp"

	"github.com/mew-ton/hozo/runtime"
)

type Path string

func (v Path) Validate() error { return nil }

type OutputFormat string

var patternOutputFormat = regexp.MustCompile("^(text|json)$")

func (v OutputFormat) Validate() error {
	if !patternOutputFormat.MatchString(string(v)) {
		return &runtime.ValidationError{Violations: []runtime.Violation{{Path: "", Constraint: "pattern"}}}
	}
	return nil
}

type Position struct {
	Line int64 ` + "`json:\"line\"`" + `
	Col  int64 ` + "`json:\"col\"`" + `
}

type Diagnostic struct {
	Severity string    ` + "`json:\"severity\"`" + `
	Code     string    ` + "`json:\"code\"`" + `
	Message  string    ` + "`json:\"message\"`" + `
	File     *Path     ` + "`json:\"file,omitempty\"`" + `
	Start    *Position ` + "`json:\"start,omitempty\"`" + `
	End      *Position ` + "`json:\"end,omitempty\"`" + `
}

type Diagnostics struct {
	Items []Diagnostic ` + "`json:\"items\"`" + `
}

type CheckResult struct {
	Warnings []Diagnostic ` + "`json:\"warnings\"`" + `
}

type EmitResult struct {
	Warnings []Diagnostic ` + "`json:\"warnings\"`" + `
	Written  []Path       ` + "`json:\"written\"`" + `
	Removed  []Path       ` + "`json:\"removed\"`" + `
}

type CheckInput struct {
	Config *Path         ` + "`json:\"config,omitempty\"`" + `
	Format *OutputFormat ` + "`json:\"format,omitempty\"`" + `
}

func (m CheckInput) Validate() error { return validateConfigFormat(m.Config, m.Format) }

type EmitInput struct {
	Config     *Path         ` + "`json:\"config,omitempty\"`" + `
	Format     *OutputFormat ` + "`json:\"format,omitempty\"`" + `
	Namespaces []string      ` + "`json:\"namespaces\"`" + `
	DryRun     *bool         ` + "`json:\"dryRun,omitempty\"`" + `
}

func (m EmitInput) Validate() error { return validateConfigFormat(m.Config, m.Format) }

type LspInput struct {
	Config *Path ` + "`json:\"config,omitempty\"`" + `
	Log    *Path ` + "`json:\"log,omitempty\"`" + `
}

func (m LspInput) Validate() error { return nil }

func validateConfigFormat(config *Path, format *OutputFormat) error {
	var vs []runtime.Violation
	if config != nil {
		vs = append(vs, prefixed("config", config.Validate())...)
	}
	if format != nil {
		vs = append(vs, prefixed("format", format.Validate())...)
	}
	if len(vs) == 0 {
		return nil
	}
	return &runtime.ValidationError{Violations: vs}
}

func prefixed(path string, err error) []runtime.Violation {
	ve, ok := err.(*runtime.ValidationError)
	if !ok {
		return nil
	}
	out := make([]runtime.Violation, len(ve.Violations))
	for i, v := range ve.Violations {
		p := path
		if v.Path != "" {
			p += "." + v.Path
		}
		out[i] = runtime.Violation{Path: p, Constraint: v.Constraint}
	}
	return out
}

type CheckFailed struct{ Value Diagnostics }

func (CheckFailed) Error() string         { return "hozoc.CheckFailed" }
func (CheckFailed) HozoException() string { return "hozoc.CheckFailed" }

type ConfigInvalid struct{ Value Diagnostics }

func (ConfigInvalid) Error() string         { return "hozoc.ConfigInvalid" }
func (ConfigInvalid) HozoException() string { return "hozoc.ConfigInvalid" }

type LanguageMismatch struct{ Value Diagnostics }

func (LanguageMismatch) Error() string         { return "hozoc.LanguageMismatch" }
func (LanguageMismatch) HozoException() string { return "hozoc.LanguageMismatch" }

type OutputConflict struct{ Value Diagnostics }

func (OutputConflict) Error() string         { return "hozoc.OutputConflict" }
func (OutputConflict) HozoException() string { return "hozoc.OutputConflict" }

type UnknownNamespace struct{}

func (UnknownNamespace) Error() string         { return "hozoc.UnknownNamespace" }
func (UnknownNamespace) HozoException() string { return "hozoc.UnknownNamespace" }

// Crashed と MalformedOutput は proc の語彙の例外を hozoc のパッケージに複製したもの（docs/emit-go.md「配置」）。
type Crashed struct{}

func (Crashed) Error() string         { return "proc.Crashed" }
func (Crashed) HozoException() string { return "proc.Crashed" }

type MalformedOutput struct{}

func (MalformedOutput) Error() string         { return "proc.MalformedOutput" }
func (MalformedOutput) HozoException() string { return "proc.MalformedOutput" }

type Ops interface {
	Check(in CheckInput) (CheckResult, error)
	WriteOutputs(in EmitInput, checked CheckResult) (EmitResult, error)
	Emit(in EmitInput) (EmitResult, error)
	Lsp(in LspInput) error
}
`

// hozocMain は hozoc.Ops の偽物をつなぐ main。config の値で振る舞いを変える。
const hozocMain = `package main

import (
	"errors"
	"fmt"
	"os"

	"example.com/app/internal/gen/hozoc"
	"example.com/app/internal/gen/hozoc/cli"
)

type ops struct{}

func cfg(p *hozoc.Path) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

func diags(code string) hozoc.Diagnostics {
	return hozoc.Diagnostics{Items: []hozoc.Diagnostic{{Severity: "error", Code: code, Message: "m"}}}
}

func (ops) Check(in hozoc.CheckInput) (hozoc.CheckResult, error) {
	switch cfg(in.Config) {
	case "x1":
		return hozoc.CheckResult{}, hozoc.CheckFailed{Value: diags("T-1")}
	case "x2":
		return hozoc.CheckResult{}, &hozoc.ConfigInvalid{Value: diags("CFG-12")} // ポインタでも見分ける
	case "x3":
		return hozoc.CheckResult{}, fmt.Errorf("wrapped: %w", hozoc.LanguageMismatch{Value: diags("CFG-16")}) // 包んでも見分ける
	case "x5":
		return hozoc.CheckResult{}, hozoc.Crashed{}
	case "x6":
		return hozoc.CheckResult{}, errors.New("boom")
	}
	format := ""
	if in.Format != nil {
		format = string(*in.Format)
	}
	return hozoc.CheckResult{Warnings: []hozoc.Diagnostic{{Severity: "warning", Code: "W", Message: "config=" + cfg(in.Config) + " format=" + format}}}, nil
}

func (ops) WriteOutputs(in hozoc.EmitInput, checked hozoc.CheckResult) (hozoc.EmitResult, error) {
	return hozoc.EmitResult{}, nil
}

func (ops) Emit(in hozoc.EmitInput) (hozoc.EmitResult, error) {
	switch cfg(in.Config) {
	case "x3":
		return hozoc.EmitResult{}, hozoc.OutputConflict{Value: diags("CFG-5")}
	case "x4":
		return hozoc.EmitResult{}, hozoc.UnknownNamespace{}
	case "x5":
		return hozoc.EmitResult{}, hozoc.Crashed{}
	}
	res := hozoc.EmitResult{}
	for _, ns := range in.Namespaces {
		res.Written = append(res.Written, hozoc.Path(ns))
	}
	if in.DryRun != nil && *in.DryRun {
		res.Removed = []hozoc.Path{"dry"}
	}
	return res, nil
}

func (ops) Lsp(in hozoc.LspInput) error {
	if cfg(in.Config) == "x6" {
		return errors.New("boom")
	}
	return nil
}

func main() {
	os.Exit(cli.Main(ops{}, cli.JSONPresenter{}, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
`

const demoStub = `package demo

import (
	"github.com/mew-ton/hozo/runtime"
)

type Count int64

func (v Count) Validate() error {
	if v < 0 {
		return &runtime.ValidationError{Violations: []runtime.Violation{{Constraint: "min"}}}
	}
	return nil
}

type Sum struct {
	Total float64 ` + "`json:\"total\"`" + `
}

func (m Sum) Validate() error { return nil }

type Overflow struct{}

func (Overflow) Error() string         { return "demo.Overflow" }
func (Overflow) HozoException() string { return "demo.Overflow" }

type Tired struct{}

func (Tired) Error() string         { return "demo.Tired" }
func (Tired) HozoException() string { return "demo.Tired" }

type AddInput struct {
	A       int64    ` + "`json:\"a\"`" + `
	B       *float64 ` + "`json:\"b,omitempty\"`" + `
	Count   Count    ` + "`json:\"count\"`" + `
	Verbose bool     ` + "`json:\"verbose\"`" + `
	Label   *string  ` + "`json:\"label,omitempty\"`" + `
	Scale   *float64 ` + "`json:\"scale,omitempty\"`" + `
}

func NewAddInput() AddInput { return AddInput{Count: 1} }

func (m AddInput) Validate() error {
	if err := m.Count.Validate(); err != nil {
		return &runtime.ValidationError{Violations: []runtime.Violation{{Path: "count", Constraint: "min"}}}
	}
	return nil
}

type Ops interface {
	Add(in AddInput) (Sum, error)
	Ping() error
}
`

const demoMain = `package main

import (
	"os"

	"example.com/app/gen/demo"
	"example.com/app/gen/demo/cli"
)

type ops struct{}

// Add は入力のすべてを一つの数に畳む: a + b + count*100 + (verbose ? 0.5 : 0) + len(label) + scale*1000。
func (ops) Add(in demo.AddInput) (demo.Sum, error) {
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

func (ops) Ping() error { return nil }

func main() {
	os.Exit(cli.Main(ops{}, cli.JSONPresenter{}, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
`

// scenario は作ったバイナリの一回の実行と、期待する終了コードと出力。Stdout / Stderr は含まれるべき部分文字列。
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
}

func TestCompileAndRun(t *testing.T) {
	goBin := goTool(t)
	root := repoRoot(t)
	mod := t.TempDir()

	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(mod, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/app\n\ngo 1.24\n\nrequire github.com/mew-ton/hozo v0.0.0\n\nreplace github.com/mew-ton/hozo => "+root+"\n")
	if sum, err := os.ReadFile(filepath.Join(root, "go.sum")); err == nil {
		write("go.sum", string(sum))
	}

	h := handlerFor(mod)
	hv := hozocView() // out は internal/gen/hozoc
	resp := emit(t, h, hv)
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v", resp.Errors)
	}
	write("internal/gen/hozoc/hozoc.go", hozocStub)
	write(resp.Files[0].Path, resp.Files[0].Content) // Path はルートからの相対（<dir>/cli/cli.go）
	write("cmd/app/main.go", hozocMain)

	dv := demoView() // out は gen/demo
	resp = emit(t, h, dv)
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v", resp.Errors)
	}
	write("gen/demo/demo.go", demoStub)
	write(resp.Files[0].Path, resp.Files[0].Content)
	write("cmd/demo/main.go", demoMain)

	run := func(args ...string) (string, error) {
		cmd := exec.Command(goBin, args...)
		cmd.Dir = mod
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("vet", "./..."); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
	if out, err := run("build", "-o", filepath.Join(mod, "bin", "app"), "./cmd/app"); err != nil {
		t.Fatalf("go build app: %v\n%s", err, out)
	}
	if out, err := run("build", "-o", filepath.Join(mod, "bin", "demo"), "./cmd/demo"); err != nil {
		t.Fatalf("go build demo: %v\n%s", err, out)
	}

	appScenarios := []scenario{
		{name: "success json", args: []string{"check", "--format", "json"}, code: 0, stdout: []string{`"warnings"`, "config= format=json"}, stderrEmpty: true},
		{name: "flag with equals", args: []string{"check", "--format=text"}, code: 0, stdout: []string{"format=text"}},
		{name: "exit 1 CheckFailed with value", args: []string{"check", "--config", "x1"}, code: 1, stdoutEmpty: true, stderr: []string{`"error": "hozoc.CheckFailed"`, `"value"`, `"T-1"`}},
		{name: "exit 2 pointer exception", args: []string{"check", "--config", "x2"}, code: 2, stderr: []string{`"hozoc.ConfigInvalid"`, `"CFG-12"`}},
		{name: "exit 2 wrapped exception", args: []string{"check", "--config", "x3"}, code: 2, stderr: []string{`"hozoc.LanguageMismatch"`}},
		{name: "exit 3", args: []string{"emit", "--config", "x3"}, code: 3, stderr: []string{`"hozoc.OutputConflict"`}},
		{name: "exit 64 exception without value", args: []string{"emit", "--config", "x4"}, code: 64, stderr: []string{`"error": "hozoc.UnknownNamespace"`}, notStderr: []string{`"value"`}},
		{name: "exit 70 foreign exception", args: []string{"emit", "--config", "x5"}, code: 70, stderr: []string{`"proc.Crashed"`}},
		{name: "exit 70 undeclared error", args: []string{"check", "--config", "x6"}, code: 70, stderr: []string{`"error": "boom"`}},
		{name: "validate 64", args: []string{"check", "--format", "xml"}, code: 64, stderr: []string{"invalid input: format: pattern"}},
		{name: "unknown flag", args: []string{"check", "--bogus"}, code: 64, stderr: []string{"unknown flag --bogus", "usage: app check"}},
		{name: "flag without value", args: []string{"check", "--config"}, code: 64, stderr: []string{"flag --config needs a value"}},
		{name: "no args", args: nil, code: 64, stderr: []string{"usage: app <command> [flags]", "commands:", "  check   "}},
		{name: "unknown command", args: []string{"bogus"}, code: 64, stderr: []string{`unknown command "bogus"`, "usage:"}},
		{name: "help", args: []string{"help"}, code: 0, stdout: []string{"usage: app <command> [flags]", "  emit    "}, stderrEmpty: true},
		{name: "--help", args: []string{"--help"}, code: 0, stdout: []string{"commands:"}},
		{name: "-h", args: []string{"-h"}, code: 0, stdout: []string{"commands:"}},
		{name: "command --help", args: []string{"emit", "--help"}, code: 0, stdout: []string{"usage: app emit [flags] [NAMESPACES...]", "--dry-run", "(env: HOZO_CONFIG)"}, stderrEmpty: true},
		{name: "command -h after args", args: []string{"emit", "a", "-h"}, code: 0, stdout: []string{"usage: app emit"}},
		{name: "positional and boolean flag", args: []string{"emit", "a", "b", "--dry-run"}, code: 0, stdout: []string{`"a"`, `"b"`, `"dry"`}},
		{name: "double dash", args: []string{"emit", "--", "--dry-run"}, code: 0, stdout: []string{`"--dry-run"`}, notStdout: []string{`"dry"`}},
		{name: "boolean false", args: []string{"emit", "--dry-run=false", "x"}, code: 0, stdout: []string{`"x"`}, notStdout: []string{`"dry"`}},
		{name: "boolean bad", args: []string{"emit", "--dry-run=maybe"}, code: 64, stderr: []string{`invalid value "maybe" for --dry-run: want boolean`}},
		{name: "env fallback", args: []string{"check"}, env: map[string]string{"HOZO_CONFIG": "fromenv"}, code: 0, stdout: []string{"config=fromenv"}},
		{name: "flag beats env", args: []string{"check", "--config", "flag"}, env: map[string]string{"HOZO_CONFIG": "fromenv"}, code: 0, stdout: []string{"config=flag"}},
		{name: "empty env is absent", args: []string{"check"}, env: map[string]string{"HOZO_CONFIG": ""}, code: 0, stdout: []string{"config= "}},
		{name: "void success", args: []string{"lsp"}, code: 0, stdoutEmpty: true, stderrEmpty: true},
		{name: "void error", args: []string{"lsp", "--config", "x6"}, code: 70, stderr: []string{`"boom"`}},
		{name: "too many args", args: []string{"lsp", "extra"}, code: 64, stderr: []string{"too many arguments"}},
		{name: "too many args 2", args: []string{"check", "x"}, code: 64, stderr: []string{"too many arguments: 1 given, at most 0 expected"}},
	}
	runScenarios(t, filepath.Join(mod, "bin", "app"), appScenarios)

	demoScenarios := []scenario{
		{name: "int float default", args: []string{"add", "1", "2.5", "--verbose"}, code: 0, stdout: []string{`"total": 104`}},
		{name: "count and verbose=false", args: []string{"add", "1", "--verbose=false", "--count", "3"}, code: 0, stdout: []string{`"total": 301`}},
		{name: "scalar validate", args: []string{"add", "1", "--verbose", "--count", "-1"}, code: 64, stderr: []string{"invalid input: count: min"}},
		{name: "bad int arg", args: []string{"add", "x", "--verbose"}, code: 64, stderr: []string{`invalid value "x" for A: want int`}},
		{name: "bad int flag", args: []string{"add", "1", "--verbose", "--count", "1.5"}, code: 64, stderr: []string{`invalid value "1.5" for --count: want int`}},
		{name: "bad float arg", args: []string{"add", "1", "y", "--verbose"}, code: 64, stderr: []string{`invalid value "y" for B: want float`}},
		{name: "missing arg", args: []string{"add", "--verbose"}, code: 64, stderr: []string{"missing required argument A"}},
		{name: "missing flag", args: []string{"add", "1"}, code: 64, stderr: []string{"missing required flag --verbose"}},
		{name: "too many", args: []string{"add", "1", "2", "3", "--verbose"}, code: 64, stderr: []string{"too many arguments: 3 given, at most 2 expected"}},
		{name: "env only float", args: []string{"add", "1", "--verbose"}, env: map[string]string{"DEMO_SCALE": "2"}, code: 0, stdout: []string{`"total": 2101.5`}},
		{name: "env only bad", args: []string{"add", "1", "--verbose"}, env: map[string]string{"DEMO_SCALE": "z"}, code: 64, stderr: []string{`invalid value "z" for DEMO_SCALE: want float`}},
		{name: "env for flag", args: []string{"add", "1", "--verbose"}, env: map[string]string{"DEMO_LABEL": "abc"}, code: 0, stdout: []string{`"total": 104.5`}},
		{name: "flag beats env", args: []string{"add", "1", "--verbose", "--label", "a"}, env: map[string]string{"DEMO_LABEL": "abc"}, code: 0, stdout: []string{`"total": 102.5`}},
		{name: "custom exit code", args: []string{"add", "99", "--verbose"}, code: 5, stderr: []string{`"error": "demo.Overflow"`}},
		{name: "default exit code", args: []string{"add", "98", "--verbose"}, code: 70, stderr: []string{`"error": "demo.Tired"`}},
		{name: "usage", args: []string{"add", "--help"}, code: 0, stdout: []string{"usage: demo add [flags] A [B]", "A   左 (required)", "--count <Count>", "(default: 1)", "environment:", "DEMO_SCALE"}},
		{name: "no-arg void", args: []string{"ping"}, code: 0, stdoutEmpty: true, stderrEmpty: true},
		{name: "no-arg too many", args: []string{"ping", "x"}, code: 64, stderr: []string{"too many arguments"}},
		{name: "no-arg help", args: []string{"ping", "--help"}, code: 0, stdout: []string{"usage: demo ping\n"}},
	}
	runScenarios(t, filepath.Join(mod, "bin", "demo"), demoScenarios)
}

func runScenarios(t *testing.T, bin string, scenarios []scenario) {
	t.Helper()
	for _, s := range scenarios {
		t.Run(filepath.Base(bin)+"/"+s.name, func(t *testing.T) {
			cmd := exec.Command(bin, s.args...)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
			for k, v := range s.env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
			var out, errOut bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errOut
			err := cmd.Run()
			code := 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("run: %v", err)
			}
			if code != s.code {
				t.Errorf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, s.code, out.String(), errOut.String())
			}
			for _, w := range s.stdout {
				if !strings.Contains(out.String(), w) {
					t.Errorf("stdout lacks %q:\n%s", w, out.String())
				}
			}
			for _, w := range s.stderr {
				if !strings.Contains(errOut.String(), w) {
					t.Errorf("stderr lacks %q:\n%s", w, errOut.String())
				}
			}
			for _, w := range s.notStdout {
				if strings.Contains(out.String(), w) {
					t.Errorf("stdout must not contain %q:\n%s", w, out.String())
				}
			}
			for _, w := range s.notStderr {
				if strings.Contains(errOut.String(), w) {
					t.Errorf("stderr must not contain %q:\n%s", w, errOut.String())
				}
			}
			if s.stdoutEmpty && out.Len() != 0 {
				t.Errorf("stdout must be empty:\n%s", out.String())
			}
			if s.stderrEmpty && errOut.Len() != 0 {
				t.Errorf("stderr must be empty:\n%s", errOut.String())
			}
		})
	}
}
