package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// fake は各方向を関数で差し替えられる Handler。nil の方向は ErrUnsupported。
type fake struct {
	emit   func(*EmitRequest) (*EmitResponse, error)
	check  func(*CheckRequest) (*CheckResponse, error)
	imp    func(*ImportRequest) (*ImportResponse, error)
	called string
}

func (f *fake) Emit(r *EmitRequest) (*EmitResponse, error) {
	f.called = "emit"
	if f.emit == nil {
		return nil, ErrUnsupported
	}
	return f.emit(r)
}

func (f *fake) Check(r *CheckRequest) (*CheckResponse, error) {
	f.called = "check"
	if f.check == nil {
		return nil, ErrUnsupported
	}
	return f.check(r)
}

func (f *fake) Import(r *ImportRequest) (*ImportResponse, error) {
	f.called = "import"
	if f.imp == nil {
		return nil, ErrUnsupported
	}
	return f.imp(r)
}

func run(t *testing.T, h Handler, input string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = Run(h, strings.NewReader(input), &o, &e)
	return code, o.String(), e.String()
}

func decode(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, s)
	}
}

type refusal struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func viewJSON(viewer string) string {
	return fmt.Sprintf(`{"version":%q,"viewer":%q,"config":{"language":"go","out":"gen","plugins":[]},"program":{"version":%q,"namespaces":[],"binds":[]},"ops":{}}`,
		ir.Version, viewer, ir.Version)
}

func TestEmit(t *testing.T) {
	h := &fake{emit: func(r *EmitRequest) (*EmitResponse, error) {
		if r.Protocol != ir.Version || r.Direction != "emit" {
			t.Errorf("request header = %q %q", r.Protocol, r.Direction)
		}
		if r.View == nil || r.View.Viewer != "cli" {
			t.Errorf("view not decoded: %+v", r.View)
		}
		return &EmitResponse{Files: []File{{Path: "cli/cli.go", Content: "package cli\n"}}}, nil
	}}
	code, out, errOut := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"emit","view":%s}`, ir.Version, viewJSON("cli")))
	if code != 0 || errOut != "" {
		t.Fatalf("code = %d, stderr = %q", code, errOut)
	}
	var got map[string]json.RawMessage
	decode(t, out, &got)
	if string(got["files"]) != `[{"path":"cli/cli.go","content":"package cli\n"}]` {
		t.Errorf("files = %s", got["files"])
	}
	if string(got["warnings"]) != `[]` {
		t.Errorf("warnings must be [] when empty, got %s", got["warnings"])
	}
	if _, ok := got["errors"]; ok {
		t.Errorf("errors must be omitted when empty, got %s", got["errors"])
	}
}

func TestEmitErrorsAndWarnings(t *testing.T) {
	h := &fake{emit: func(r *EmitRequest) (*EmitResponse, error) {
		return &EmitResponse{
			Warnings: []Warning{{At: "hozoc.emit", Message: "w"}},
			Errors:   []Diagnostic{{Code: "CLI-1", At: "hozoc.check", Message: "e"}},
		}, nil
	}}
	code, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"emit","view":%s}`, ir.Version, viewJSON("hozoc")))
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	want := `{"files":[],"warnings":[{"at":"hozoc.emit","message":"w"}],"errors":[{"code":"CLI-1","at":"hozoc.check","message":"e"}]}` + "\n"
	if out != want {
		t.Errorf("stdout = %s\nwant %s", out, want)
	}
}

func TestCheck(t *testing.T) {
	h := &fake{check: func(r *CheckRequest) (*CheckResponse, error) {
		if r.View == nil || r.View.Viewer != "browser" {
			t.Errorf("view not decoded: %+v", r.View)
		}
		return &CheckResponse{Diagnostics: []Diagnostic{{Severity: "error", Code: "PAGE-1", Message: "m", At: "browser.openOrders"}}}, nil
	}}
	code, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"check","view":%s}`, ir.Version, viewJSON("browser")))
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	want := `{"diagnostics":[{"severity":"error","code":"PAGE-1","at":"browser.openOrders","message":"m"}]}` + "\n"
	if out != want {
		t.Errorf("stdout = %s\nwant %s", out, want)
	}
}

func TestCheckEmptyDiagnostics(t *testing.T) {
	h := &fake{check: func(r *CheckRequest) (*CheckResponse, error) { return &CheckResponse{}, nil }}
	code, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"check","view":%s}`, ir.Version, viewJSON("browser")))
	if code != 0 || out != "{\"diagnostics\":[]}\n" {
		t.Errorf("code = %d, stdout = %s", code, out)
	}
}

func TestImport(t *testing.T) {
	h := &fake{imp: func(r *ImportRequest) (*ImportResponse, error) {
		if r.Format != "openapi" || len(r.Files) != 1 || r.Files[0].Path != "openapi.yaml" || r.Files[0].Content != "openapi: 3.0.0" {
			t.Errorf("request = %+v", r)
		}
		return &ImportResponse{IR: &ir.Program{Version: ir.Version}}, nil
	}}
	code, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"import","format":"openapi","files":[{"path":"openapi.yaml","content":"openapi: 3.0.0"}]}`, ir.Version))
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	var got struct {
		IR       *ir.Program `json:"ir"`
		Warnings []Warning   `json:"warnings"`
	}
	decode(t, out, &got)
	if got.IR == nil || got.IR.Version != ir.Version {
		t.Errorf("ir = %+v", got.IR)
	}
	if !strings.Contains(out, `"warnings":[]`) {
		t.Errorf("warnings must be [] when empty: %s", out)
	}
}

func TestProtocolMismatch(t *testing.T) {
	h := &fake{emit: func(*EmitRequest) (*EmitResponse, error) { return &EmitResponse{}, nil }}
	code, out, _ := run(t, h, `{"protocol":"9.9","direction":"emit","view":{}}`)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	var r refusal
	decode(t, out, &r)
	if r.Error.Code != "protocol" || !strings.Contains(r.Error.Message, "9.9") {
		t.Errorf("refusal = %+v", r.Error)
	}
	if h.called != "" {
		t.Errorf("handler must not be called on protocol mismatch, called %q", h.called)
	}
}

func TestUnsupportedDirection(t *testing.T) {
	for _, dir := range []string{"emit", "check", "import"} {
		t.Run(dir, func(t *testing.T) {
			h := &fake{} // すべて ErrUnsupported
			code, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":%q}`, ir.Version, dir))
			if code != 2 {
				t.Fatalf("code = %d, want 2", code)
			}
			var r refusal
			decode(t, out, &r)
			if r.Error.Code != "unsupported" || !strings.Contains(r.Error.Message, dir) {
				t.Errorf("refusal = %+v", r.Error)
			}
			if h.called != dir {
				t.Errorf("called = %q, want %q", h.called, dir)
			}
		})
	}
}

func TestWrappedErrUnsupported(t *testing.T) {
	h := &fake{emit: func(*EmitRequest) (*EmitResponse, error) { return nil, fmt.Errorf("emit: %w", ErrUnsupported) }}
	code, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"emit"}`, ir.Version))
	var r refusal
	decode(t, out, &r)
	if code != 2 || r.Error.Code != "unsupported" {
		t.Errorf("code = %d, refusal = %+v", code, r.Error)
	}
}

func TestUnknownDirection(t *testing.T) {
	h := &fake{}
	code, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"dance"}`, ir.Version))
	var r refusal
	decode(t, out, &r)
	if code != 2 || r.Error.Code != "unsupported" || !strings.Contains(r.Error.Message, "dance") {
		t.Errorf("code = %d, refusal = %+v", code, r.Error)
	}
	if h.called != "" {
		t.Errorf("handler called for unknown direction: %q", h.called)
	}
}

func TestHandlerError(t *testing.T) {
	h := &fake{check: func(*CheckRequest) (*CheckResponse, error) { return nil, errors.New("boom") }}
	code, out, errOut := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"check"}`, ir.Version))
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if out != "" {
		t.Errorf("stdout must be empty, got %s", out)
	}
	if !strings.Contains(errOut, "boom") || !strings.Contains(errOut, "check") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestMalformedRequest(t *testing.T) {
	for name, input := range map[string]string{"not json": "{", "empty": "", "wrong type": fmt.Sprintf(`{"protocol":%q,"direction":"emit","view":5}`, ir.Version)} {
		t.Run(name, func(t *testing.T) {
			h := &fake{emit: func(*EmitRequest) (*EmitResponse, error) { return &EmitResponse{}, nil }}
			code, out, errOut := run(t, h, input)
			if code != 1 || out != "" || errOut == "" {
				t.Errorf("code = %d, stdout = %q, stderr = %q", code, out, errOut)
			}
		})
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestWriteFailure(t *testing.T) {
	h := &fake{emit: func(*EmitRequest) (*EmitResponse, error) { return &EmitResponse{}, nil }}
	var e bytes.Buffer
	code := Run(h, strings.NewReader(fmt.Sprintf(`{"protocol":%q,"direction":"emit"}`, ir.Version)), failWriter{}, &e)
	if code != 1 || !strings.Contains(e.String(), "closed") {
		t.Errorf("code = %d, stderr = %q", code, e.String())
	}
}

func TestNoHTMLEscape(t *testing.T) {
	h := &fake{emit: func(*EmitRequest) (*EmitResponse, error) {
		return &EmitResponse{Files: []File{{Path: "a.go", Content: "a < b && c > d"}}}, nil
	}}
	_, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"emit"}`, ir.Version))
	if !strings.Contains(out, `"a < b && c > d"`) {
		t.Errorf("content must not be HTML-escaped: %s", out)
	}
}
