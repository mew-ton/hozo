package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// fake は各方向を関数で差し替えられる Handler。nil の方向は ErrUnsupported。
type fake struct {
	emit   func(*EmitRequest) (*EmitResponse, error)
	imp    func(*ImportRequest) (*ImportResponse, error)
	cov    func(*CoverageRequest) (*CoverageResponse, error)
	called string
}

func (f *fake) Emit(r *EmitRequest) (*EmitResponse, error) {
	f.called = "emit"
	if f.emit == nil {
		return nil, ErrUnsupported
	}
	return f.emit(r)
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

// specJSON は、要求の spec（docs/ir.md「まとめた形」の Spec）。program は、名前空間を持たない。
func specJSON() string {
	return fmt.Sprintf(`{"version":%q,"namespaces":{},"program":{"version":%q,"namespaces":[],"binds":[]}}`, ir.Version, ir.Version)
}

// outputJSON は、要求の outputs の要素一つ: 見る側が viewer の出力で、頼むトランスパイラが transpilers。
func outputJSON(viewer string, transpilers ...string) string {
	names, _ := json.Marshal(transpilers)
	return fmt.Sprintf(`{"transpilers":%s,"view":{"viewer":%q,"config":{"index":1,"language":"go","base":"go","out":"gen","dir":"gen","transpilers":["go","cli"],"options":{},"seams":{}},"files":[],"synthesized":[],"entries":[],"ops":{}}}`,
		names, viewer)
}

// emitJSON は、direction "emit" の要求（docs/plugin-protocol.md「emit」）: 仕様を一度と、出力の並び。
func emitJSON(outputs ...string) string {
	return fmt.Sprintf(`{"protocol":%q,"direction":"emit","spec":%s,"outputs":[%s]}`, ir.Version, specJSON(), strings.Join(outputs, ","))
}

func TestEmit(t *testing.T) {
	h := &fake{emit: func(r *EmitRequest) (*EmitResponse, error) {
		if r.Protocol != ir.Version || r.Direction != "emit" || r.Transpiler != "go-cli" {
			t.Errorf("request header = %q %q %q", r.Protocol, r.Direction, r.Transpiler)
		}
		if r.View == nil || r.View.Viewer != "cli" || r.View.Version != ir.Version || r.View.Program == nil {
			t.Errorf("view not decoded: %+v", r.View)
		}
		return &EmitResponse{Files: []File{{Path: "cli/cli.go", Content: "package cli\n"}}}, nil
	}}
	code, out, errOut := run(t, h, emitJSON(outputJSON("cli", "go-cli")))
	if code != 0 || errOut != "" {
		t.Fatalf("code = %d, stderr = %q", code, errOut)
	}
	// 応答は、出力ごと、トランスパイラごとの答えの並び。warnings は空でも []、errors は省く。select は、ない
	// （版 0.4 でなくした。docs/plugin-protocol.md「選択をなくしたこと」）
	want := `{"outputs":[{"results":[{"files":[{"path":"cli/cli.go","content":"package cli\n"}],"warnings":[]}]}]}` + "\n"
	if out != want {
		t.Errorf("stdout = %s\nwant %s", out, want)
	}
	got, err := SingleResult([]byte(out))
	if err != nil || len(got.Files) != 1 || got.Files[0].Path != "cli/cli.go" {
		t.Errorf("SingleResult = %+v, %v", got, err)
	}
}

// TestRunEmitBatch は docs/plugin-protocol.md「emit」の「SDK が、並びを回す」: 要求の outputs の順、その中は transpilers の順に、
// Handler.Emit を呼び、答えを、同じ並びで応答に置く。Emit に渡る View は、spec と、その出力の view を合わせたもの
// （docs/ir.md「まとめた形」）: program は、spec のものに、その出力の synthesized を、viewer の名前空間の末尾に足したもの。
// 出力のない要求には、空の outputs を返す。transpilers が空の出力には、空の results。
func TestRunEmitBatch(t *testing.T) {
	spec := fmt.Sprintf(`{"version":%q,"namespaces":{"a":[],"b":[]},"program":{"version":%q,"namespaces":[`+
		`{"name":"a","decls":[{"kind":"model","name":"a.M","span":{"file":"a.hozo","start":{"line":1,"col":1},"end":{"line":1,"col":2}},"decorators":[],"fields":[]}]},`+
		`{"name":"b","decls":[]}],"binds":[]}}`, ir.Version, ir.Version)
	view := func(viewer, synthesized string) string {
		return fmt.Sprintf(`{"viewer":%q,"config":{"index":1,"language":"go","base":"go","out":"gen","dir":"gen/%s","transpilers":["go","cli"],"options":{},"seams":{}},"files":["a.hozo"],"synthesized":[%s],"entries":[],"ops":{}}`,
			viewer, viewer, synthesized)
	}
	synth := `{"kind":"model","name":"a.f$in","span":{"file":"a.hozo","start":{"line":2,"col":1},"end":{"line":2,"col":2}},"decorators":[],"synthesizedFrom":"a.f","fields":[]}`
	req := fmt.Sprintf(`{"protocol":%q,"direction":"emit","spec":%s,"outputs":[`+
		`{"transpilers":["go","cli"],"view":%s},{"transpilers":[],"view":%s},{"transpilers":["cli"],"view":%s}]}`,
		ir.Version, spec, view("a", synth), view("b", ""), view("b", ""))
	var calls []string
	var programs []*ir.Program
	h := &fake{emit: func(r *EmitRequest) (*EmitResponse, error) {
		decls := 0
		if ns := r.View.Program.Namespace("a"); ns != nil {
			decls = len(ns.Decls)
		}
		calls = append(calls, fmt.Sprintf("%s@%s dir=%s a.decls=%d", r.Transpiler, r.View.Viewer, r.View.Config.Dir, decls))
		programs = append(programs, r.View.Program)
		if r.View.Version != ir.Version || len(r.View.Namespaces) != 2 || len(r.View.Files) != 1 {
			t.Errorf("View が、spec と view を合わせたものでない: %+v", r.View)
		}
		if r.Transpiler == "cli" && r.View.Viewer == "a" {
			return &EmitResponse{Errors: []Diagnostic{{Code: "X-1", At: "a.M", Message: "e"}}}, nil
		}
		return &EmitResponse{Files: []File{{Path: "gen/" + r.View.Viewer + "/" + r.Transpiler + ".go", Content: "x"}}}, nil
	}}
	code, out, errOut := run(t, h, req)
	if code != 0 || errOut != "" {
		t.Fatalf("code = %d, stderr = %q", code, errOut)
	}
	wantCalls := []string{"go@a dir=gen/a a.decls=2", "cli@a dir=gen/a a.decls=2", "cli@b dir=gen/b a.decls=1"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Errorf("Emit の呼び出し = %q\nwant %q", calls, wantCalls)
	}
	// 合成した model を足すのは、その出力の View だけ: 仕様（spec の program）は、変えない
	if len(programs) == 3 && len(programs[2].Namespace("a").Decls) != 1 {
		t.Error("ほかの出力の synthesized が、spec の program に残った")
	}
	want := `{"outputs":[` +
		`{"results":[{"files":[{"path":"gen/a/go.go","content":"x"}],"warnings":[]},{"files":[],"warnings":[],"errors":[{"code":"X-1","at":"a.M","message":"e"}]}]},` +
		`{"results":[]},` +
		`{"results":[{"files":[{"path":"gen/b/cli.go","content":"x"}],"warnings":[]}]}]}` + "\n"
	if out != want {
		t.Errorf("stdout = %s\nwant %s", out, want)
	}
	// 出力のない要求
	if code, out, _ := run(t, h, emitJSON()); code != 0 || out != `{"outputs":[]}`+"\n" {
		t.Errorf("出力のない要求: code = %d, stdout = %s", code, out)
	}
	// spec のない要求、view のない出力は、誤り（終了コード 1）
	for name, input := range map[string]string{
		"spec がない": fmt.Sprintf(`{"protocol":%q,"direction":"emit","outputs":[%s]}`, ir.Version, outputJSON("a", "go")),
		"view がない": fmt.Sprintf(`{"protocol":%q,"direction":"emit","spec":%s,"outputs":[{"transpilers":["go"]}]}`, ir.Version, specJSON()),
	} {
		if code, out, errOut := run(t, h, input); code != 1 || out != "" || errOut == "" {
			t.Errorf("%s: code = %d, stdout = %q, stderr = %q", name, code, out, errOut)
		}
	}
}

// TestRunIgnoresUnknownFields は docs/plugin-protocol.md「要求と応答」: プラグインは、要求の、知らない項目を、どの階層でも、
// 無視する（docs/decisions.md「決めたこと」421）。知らない項目を足した要求への応答は、足さない要求への応答と、同じ。
func TestRunIgnoresUnknownFields(t *testing.T) {
	decl := func(extra string) string {
		return `{"kind":"model","name":"a.M","span":{"file":"a.hozo","start":{"line":1,"col":1` + extra + `},"end":{"line":1,"col":2}},"decorators":[],"fields":[` +
			`{"name":"id","type":{"kind":"core","name":"string"` + extra + `},"optional":false,"decorators":[],"span":{"file":"a.hozo","start":{"line":1,"col":1},"end":{"line":1,"col":2}}` + extra + `}]` + extra + `}`
	}
	request := func(extra string) string {
		config := `{"index":1,"language":"go","base":"go","out":"gen","dir":"gen/a","transpilers":["go"],"options":{"go":{"module":"example.com/app"}},"seams":{}` + extra + `}`
		return fmt.Sprintf(`{"protocol":%q,"direction":"emit"%s,"spec":{"version":%q,"namespaces":{"a":[%s]},"program":{"version":%q,"namespaces":[{"name":"a","decls":[%s]%s}],"binds":[]%s}%s},`+
			`"outputs":[{"transpilers":["go"],"view":{"viewer":"a","config":%s,"files":["a.hozo"],"synthesized":[],"entries":[],"ops":{}%s}%s}]}`,
			ir.Version, extra, ir.Version, config, ir.Version, decl(extra), extra, extra, extra, config, extra, extra)
	}
	h := &fake{emit: func(r *EmitRequest) (*EmitResponse, error) {
		m, _ := r.View.Program.Lookup("a.M").(*ir.Model)
		if m == nil || len(m.Fields) != 1 || m.Fields[0].Name != "id" || r.View.Config.Dir != "gen/a" || len(r.View.Namespaces["a"]) != 1 {
			return nil, fmt.Errorf("View を読めていない: %+v", r.View)
		}
		return &EmitResponse{Files: []File{{Path: "gen/a/" + m.Fields[0].Name + ".go", Content: r.View.Config.Options["go"]["module"].(string)}}}, nil
	}}
	code, plain, errOut := run(t, h, request(""))
	if code != 0 || errOut != "" {
		t.Fatalf("知らない項目のない要求: code = %d, stderr = %q", code, errOut)
	}
	// 文字列、数、null、配列、オブジェクトの、知らない項目を、すべての階層に足す
	extra := `,"futureString":"x","futureNumber":1,"futureNull":null,"futureList":[{"deep":true}],"futureObject":{"kind":"unknownKind","name":7}`
	if !strings.Contains(request(extra), `"futureObject"`) || strings.Count(request(extra), `"futureString"`) < 10 {
		t.Fatal("知らない項目が、要求に入っていない")
	}
	code, out, errOut := run(t, h, request(extra))
	if code != 0 || errOut != "" || out != plain {
		t.Errorf("知らない項目のある要求: code = %d, stderr = %q\nstdout = %s\nwant %s", code, errOut, out, plain)
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
	code, out, _ := run(t, h, emitJSON(outputJSON("hozoc", "go-cli")))
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	want := `{"outputs":[{"results":[{"files":[],"warnings":[{"at":"hozoc.emit","message":"w"}],"errors":[{"code":"CLI-1","at":"hozoc.check","message":"e"}]}]}]}` + "\n"
	if out != want {
		t.Errorf("stdout = %s\nwant %s", out, want)
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
// 文字列でない direction は実装していない direction と同じ。
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
				emit: func(*EmitRequest) (*EmitResponse, error) { return &EmitResponse{}, nil },
				imp:  func(*ImportRequest) (*ImportResponse, error) { return &ImportResponse{}, nil },
				cov:  func(*CoverageRequest) (*CoverageResponse, error) { return &CoverageResponse{}, nil },
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

func (f *fake) Coverage(r *CoverageRequest) (*CoverageResponse, error) {
	f.called = "coverage"
	if f.cov == nil {
		return nil, ErrUnsupported
	}
	return f.cov(r)
}

// coverage（docs/plugin-protocol.md「coverage」）: 要求の coverage と files が届き、応答の files と lines は、空でも [] で出る。
func TestCoverage(t *testing.T) {
	h := &fake{cov: func(r *CoverageRequest) (*CoverageResponse, error) {
		if r.Coverage.Path != "/p/cover.out" || r.Coverage.Content != "mode: set\n" || len(r.Files) != 2 || r.Files[0] != "gen/a.go" {
			t.Errorf("request = %+v", r)
		}
		return &CoverageResponse{Recognized: true, Files: []CoveredFile{{Path: "gen/a.go", Lines: [][2]int{{3, 5}}}, {Path: "gen/b.go"}}}, nil
	}}
	code, out, _ := run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"coverage","coverage":{"path":"/p/cover.out","content":"mode: set\n"},"files":["gen/a.go","gen/b.go"]}`, ir.Version))
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	want := `{"recognized":true,"files":[{"path":"gen/a.go","lines":[[3,5]]},{"path":"gen/b.go","lines":[]}]}` + "\n"
	if out != want {
		t.Errorf("response:\n got %s\nwant %s", out, want)
	}

	// 読める形でないファイル: recognized が false で、files は []
	h = &fake{cov: func(*CoverageRequest) (*CoverageResponse, error) { return &CoverageResponse{}, nil }}
	code, out, _ = run(t, h, fmt.Sprintf(`{"protocol":%q,"direction":"coverage","coverage":{"path":"x","content":"?"},"files":[]}`, ir.Version))
	if want := `{"recognized":false,"files":[]}` + "\n"; code != 0 || out != want {
		t.Errorf("code = %d, response = %s, want %s", code, out, want)
	}
}

// テストの対応表のファイル（docs/plugin-protocol.md「テストの対応表」、docs/plugin-go.md「テストの対応表」の並びと字下げ）。
func TestTestMapFile(t *testing.T) {
	f := TestMapFile("gen/server/servertest", "go-http", []TestEntry{
		{Name: "server.login/OK", File: "gen/server/servertest/http.go", Lines: [2]int{216, 222}},
		{Name: `server.search/OK/GET /search?q=<a>&"b"`, File: "gen/server/servertest/http.go", Lines: [2]int{230, 230}},
	})
	if f.Path != "gen/server/servertest/hozo-tests.go-http.json" {
		t.Errorf("path = %q", f.Path)
	}
	want := `{
  "tests": [
    {"name": "server.login/OK", "file": "gen/server/servertest/http.go", "lines": [216, 222]},
    {"name": "server.search/OK/GET /search?q=<a>&\"b\"", "file": "gen/server/servertest/http.go", "lines": [230, 230]}
  ]
}
`
	if f.Content != want {
		t.Errorf("content:\n got %s\nwant %s", f.Content, want)
	}
	var m TestMap
	decode(t, f.Content, &m)
	if len(m.Tests) != 2 || m.Tests[1].Name != `server.search/OK/GET /search?q=<a>&"b"` || m.Tests[0].Lines != [2]int{216, 222} {
		t.Errorf("round trip = %+v", m)
	}

	for _, c := range []struct {
		path, name string
		ok         bool
	}{
		{"gen/x/xtest/hozo-tests.go.json", "go", true},
		{"hozo-tests.go-http.json", "go-http", true},
		{"gen/hozo-tests..json", "", true},
		{"gen/hozo-tests.json", "", false},
		{"gen/tests.go.json", "", false},
		{"gen/hozo-tests.go.json/leaves.go", "", false},
	} {
		if name, ok := TestMapTranspiler(c.path); name != c.name || ok != c.ok {
			t.Errorf("TestMapTranspiler(%q) = %q, %v; want %q, %v", c.path, name, ok, c.name, c.ok)
		}
	}
	if TestMapName("go") != "hozo-tests.go.json" {
		t.Errorf("TestMapName = %q", TestMapName("go"))
	}
}

func TestUnsupportedDirection(t *testing.T) {
	for _, dir := range []string{"emit", "import", "coverage"} {
		t.Run(dir, func(t *testing.T) {
			h := &fake{} // すべて ErrUnsupported
			input := fmt.Sprintf(`{"protocol":%q,"direction":%q}`, ir.Version, dir)
			if dir == "emit" { // emit は、頼まれたトランスパイラごとに Handler を呼ぶ
				input = emitJSON(outputJSON("a", "go"))
			}
			code, out, _ := run(t, h, input)
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
	code, out, _ := run(t, h, emitJSON(outputJSON("a", "nope")))
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
	h := &fake{emit: func(*EmitRequest) (*EmitResponse, error) { return nil, errors.New("boom") }}
	code, out, errOut := run(t, h, emitJSON(outputJSON("a", "go")))
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if out != "" {
		t.Errorf("stdout must be empty, got %s", out)
	}
	if !strings.Contains(errOut, "boom") || !strings.Contains(errOut, "emit") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestMalformedRequest(t *testing.T) {
	for name, input := range map[string]string{"not json": "{", "empty": "", "wrong type": fmt.Sprintf(`{"protocol":%q,"direction":"emit","spec":5}`, ir.Version)} {
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
	code := Run(h, strings.NewReader(emitJSON(outputJSON("a", "go"))), failWriter{}, &e)
	if code != 1 || !strings.Contains(e.String(), "closed") {
		t.Errorf("code = %d, stderr = %q", code, e.String())
	}
}

func TestNoHTMLEscape(t *testing.T) {
	h := &fake{emit: func(*EmitRequest) (*EmitResponse, error) {
		return &EmitResponse{Files: []File{{Path: "a.go", Content: "a < b && c > d"}}}, nil
	}}
	_, out, _ := run(t, h, emitJSON(outputJSON("a", "go")))
	if !strings.Contains(out, `"a < b && c > d"`) {
		t.Errorf("content must not be HTML-escaped: %s", out)
	}
}
