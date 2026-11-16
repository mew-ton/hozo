package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	return fmt.Sprintf(`{"version":%q,"viewer":%q,"config":{"index":1,"language":"go","base":"go","out":"gen","dir":"gen","transpilers":["go","cli"],"options":{},"seams":{}},"files":[],"namespaces":{},"program":{"version":%q,"namespaces":[],"binds":[]},"ops":{}}`,
		ir.Version, viewer, ir.Version)
}

func TestEmit(t *testing.T) {
	h := &fake{emit: func(r *EmitRequest) (*EmitResponse, error) {
		if r.Protocol != ir.Version || r.Direction != "emit" || r.Transpiler != "go-cli" {
			t.Errorf("request header = %q %q %q", r.Protocol, r.Direction, r.Transpiler)
		}
		if r.View == nil || r.View.Viewer != "cli" {
			t.Errorf("view not decoded: %+v", r.View)
		}
		return &EmitResponse{Files: []File{{Path: "cli/cli.go", Content: "package cli\n"}}}, nil
	}}
	code, out, errOut := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"emit","transpiler":"go-cli","view":%s}`, ir.Version, viewJSON("cli")))
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
	// 応答に select はない（版 0.4 でなくした。docs/plugin-protocol.md「選択をなくしたこと」）
	if _, ok := got["select"]; ok {
		t.Errorf("select must not be written, got %s", got["select"])
	}
}

// docs/packages.md「`plugin`（公開）」の Options: req.View.Config.Options[req.Transpiler] を、json タグで結んだ構造体に読む。
// 書かれていなければ into はそのまま。戻り値は into にないキー（辞書順）。型の違う値は error。
func TestOptions(t *testing.T) {
	type opts struct {
		Validate bool   `json:"validate"`
		Prefix   string `json:"prefix"`
		Limits   struct {
			Max int `json:"max"`
		} `json:"limits"`
	}
	req := func(transpiler string, options map[string]map[string]any) *EmitRequest {
		return &EmitRequest{Transpiler: transpiler, View: &ir.View{Config: ir.NamespaceConfig{Options: options}}}
	}
	t.Run("書かれたキーを読む", func(t *testing.T) {
		o := opts{Prefix: "keep"}
		unknown, err := Options(req("go-http", map[string]map[string]any{
			"go-http": {"validate": true, "limits": map[string]any{"max": int64(3)}},
			"go":      {"other": 1},
		}), &o)
		if err != nil || len(unknown) != 0 {
			t.Fatalf("unknown = %v, err = %v", unknown, err)
		}
		if !o.Validate || o.Prefix != "keep" || o.Limits.Max != 3 {
			t.Errorf("options = %+v", o)
		}
	})
	t.Run("書かれていなければそのまま", func(t *testing.T) {
		o := opts{Validate: true, Prefix: "p"}
		for _, r := range []*EmitRequest{req("go-http", nil), req("go-http", map[string]map[string]any{"go": {"x": 1}}), {Transpiler: "go-http"}} {
			unknown, err := Options(r, &o)
			if err != nil || len(unknown) != 0 || !o.Validate || o.Prefix != "p" {
				t.Errorf("unknown = %v, err = %v, options = %+v", unknown, err, o)
			}
		}
	})
	t.Run("知らないキーは辞書順で返す", func(t *testing.T) {
		var o opts
		unknown, err := Options(req("go-http", map[string]map[string]any{"go-http": {"zeta": 1, "validate": true, "alpha": "x"}}), &o)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(unknown, ",") != "alpha,zeta" || !o.Validate {
			t.Errorf("unknown = %v, options = %+v", unknown, o)
		}
	})
	t.Run("型の違う値は error", func(t *testing.T) {
		var o opts
		if _, err := Options(req("go-http", map[string]map[string]any{"go-http": {"validate": "yes"}}), &o); err == nil {
			t.Error("want error")
		}
	})
	t.Run("into が構造体でなければ、すべてのキーが知らないキー", func(t *testing.T) {
		var o struct{}
		unknown, err := Options(req("go", map[string]map[string]any{"go": {"b": 1, "a": 2}}), &o)
		if err != nil || strings.Join(unknown, ",") != "a,b" {
			t.Errorf("unknown = %v, err = %v", unknown, err)
		}
	})
}

// Version は、このリポジトリのプラグインの版（docs/packages.md「`plugin`（公開）」）。生成したファイルのヘッダに書く。
func TestVersion(t *testing.T) {
	if Version != "0.0.0-dev" {
		t.Errorf("Version = %q", Version)
	}
}

// docs/packages.md「`plugin`（公開）」の OpsWith: viewer の op のうち、単位の対象（span.file が View.files にある）で、
// そのデコレータが付いたもの（via も見る）を宣言の順で。
func TestOpsWith(t *testing.T) {
	app := func(decorator, via string) ir.Application { return ir.Application{Decorator: decorator, Via: via} }
	opIn := func(file, name string, apps ...ir.Application) *ir.Op {
		return &ir.Op{DeclBase: ir.DeclBase{Name: name, Decorators: apps, Span: ir.Span{File: file}}}
	}
	op := func(name string, apps ...ir.Application) *ir.Op { return opIn("web.hozo", name, apps...) }
	view := &ir.View{Viewer: "web", Files: []string{"api.hozo", "web.hozo"}, Program: &ir.Program{Namespaces: []ir.Namespace{
		{Name: "web", Decls: []ir.Decl{
			op("web.open", app("t.entry", "")),
			op("web.plain"),
			&ir.Model{DeclBase: ir.DeclBase{Name: "web.M", Decorators: []ir.Application{app("t.entry", "")}, Span: ir.Span{File: "web.hozo"}}},
			op("web.viaNamed", app("page.route", "t.entry")),
			// in の外（batch.hozo）で宣言された op は、デコレータが付いていても返さない
			opIn("batch.hozo", "web.nightly", app("t.entry", "")),
			op("web.both", app("t.entry", ""), app("t.other", "")),
		}},
		{Name: "api", Decls: []ir.Decl{opIn("api.hozo", "api.open", app("t.entry", ""))}},
	}}}
	if !view.InUnit("web.open") || view.InUnit("web.nightly") || view.InUnit("api.open") {
		t.Errorf("InUnit: open=%v nightly=%v api.open=%v", view.InUnit("web.open"), view.InUnit("web.nightly"), view.InUnit("api.open"))
	}
	got := OpsWith(view, "t.entry")
	want := []string{"web.open", "web.viaNamed", "web.both"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("OpsWith = %v, want %v", got, want)
	}
	if got := OpsWith(view, "t.none"); got == nil || len(got) != 0 {
		t.Errorf("OpsWith(none) = %#v, want empty non-nil", got)
	}
	if got := OpsWith(&ir.View{Viewer: "x"}, "t.entry"); got == nil || len(got) != 0 {
		t.Errorf("OpsWith(no program) = %#v", got)
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

// TestRequestHead は testdata/request_head.json の表: 要求の protocol と direction の読み方と終了コード
// （docs/plugin-protocol.md「要求と応答」「起動」）。文字列でない protocol は「合わない」で code "protocol"（終了コード 2）、
// 文字列でない direction は実装していない direction と同じ。TS の SDK のテスト（sdk/ts/test/run.test.ts）も同じ表を
// 読むので、二つの SDK は同じ要求に同じ終了コードを返す。
func TestRequestHead(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "request_head.json"))
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Cases []struct {
			Name    string `json:"name"`
			Request string `json:"request"`
			Exit    int    `json:"exit"`
			Code    string `json:"code"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &table); err != nil || len(table.Cases) == 0 {
		t.Fatalf("表を読めない: %v", err)
	}
	for _, c := range table.Cases {
		t.Run(c.Name, func(t *testing.T) {
			h := &fake{
				emit:  func(*EmitRequest) (*EmitResponse, error) { return &EmitResponse{}, nil },
				check: func(*CheckRequest) (*CheckResponse, error) { return &CheckResponse{}, nil },
				imp:   func(*ImportRequest) (*ImportResponse, error) { return &ImportResponse{}, nil },
			}
			code, out, errOut := run(t, h, c.Request)
			if code != c.Exit {
				t.Fatalf("code = %d, want %d（stdout %q、stderr %q）", code, c.Exit, out, errOut)
			}
			switch code {
			case 2:
				var r refusal
				decode(t, out, &r)
				if r.Error.Code != c.Code || r.Error.Message == "" {
					t.Errorf("refusal = %+v, want code %q", r.Error, c.Code)
				}
				if h.called != "" {
					t.Errorf("断った要求で handler を呼んだ: %q", h.called)
				}
			case 1:
				if out != "" || errOut == "" {
					t.Errorf("stdout = %q, stderr = %q", out, errOut)
				}
			}
		})
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

// 知らないトランスパイラは、ErrUnsupported を包んで返す（docs/plugin-protocol.md「要求と応答」）。包んだ文がメッセージになる。
func TestWrappedErrUnsupported(t *testing.T) {
	h := &fake{emit: func(r *EmitRequest) (*EmitResponse, error) {
		return nil, fmt.Errorf("transpiler %q: %w", r.Transpiler, ErrUnsupported)
	}}
	code, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"emit","transpiler":"nope"}`, ir.Version))
	var r refusal
	decode(t, out, &r)
	if code != 2 || r.Error.Code != "unsupported" || !strings.Contains(r.Error.Message, `"nope"`) {
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
