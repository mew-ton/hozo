package plugin

import (
	"errors"
	"strings"
	"testing"

	sdk "github.com/mew-ton/hozo/plugin"
)

// wrapResult は、トランスパイラ一つの答え result を、出力が一つ、頼んだトランスパイラが一つの、emit の応答にする。
func wrapResult(result string) string { return `{"outputs":[{"results":[` + result + `]}]}` }

// docs/plugin-protocol.md「応答の形」: emit の応答は、出力ごと（outputs）、トランスパイラごと（results）の答えの並び。
func TestEmitBatchShape(t *testing.T) {
	for _, in := range []string{
		`{"outputs":[]}`,
		`{"outputs":[{"results":[]}]}`,
		`{"outputs":[{"results":[{"files":[],"warnings":[]},{"files":[],"warnings":[]}]},{"results":[{"files":[],"warnings":[]}]}]}`,
		// 知らない項目は、どの階層でも無視する
		`{"outputs":[{"results":[],"future":1}],"future":{"x":1}}`,
	} {
		if err := checkShape([]byte(in), &sdk.EmitBatchResponse{}); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
	for in, want := range map[string]string{
		`{}`:                               "outputs がない",
		`{"unknown":1}`:                    "outputs がない",
		`{"outputs":null}`:                 "outputs がない",
		`{"Outputs":[]}`:                   "outputs がない", // 名前は大文字と小文字を区別する
		`{"files":[],"warnings":[]}`:       "outputs がない", // 版 0.21 までの、答え一つの形
		`{"diagnostics":[]}`:               "outputs がない", // なくした direction check の応答
		`{"outputs":{}}`:                   "outputs が配列でない",
		`{"outputs":[[]]}`:                 "outputs[0] がオブジェクトでない",
		`{"outputs":[{}]}`:                 "outputs[0].results がない",
		`{"outputs":[{"results":null}]}`:   "outputs[0].results がない",
		`{"outputs":[{"results":{}}]}`:     "outputs[0].results が配列でない",
		`{"outputs":[{"results":["x"]}]}`:  "outputs[0].results[0] がオブジェクトでない",
		`{"outputs":[{"results":[null]}]}`: "outputs[0].results[0] がオブジェクトでない",
		`{"outputs":[{"results":[{"files":[],"warnings":[]}]},{"results":[{"files":[]}]}]}`: "outputs[1].results[0].warnings がない",
		`[]`:   "応答がオブジェクトでない",
		`"x"`:  "応答がオブジェクトでない",
		`null`: "応答がオブジェクトでない",
	} {
		err := checkShape([]byte(in), &sdk.EmitBatchResponse{})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", in, err, want)
		}
	}
}

// docs/plugin-protocol.md「応答の形」: トランスパイラ一つの答え（results の要素）の、必須の項目と型を確かめ、知らない項目は無視する。
func TestEmitShape(t *testing.T) {
	ok := []string{
		`{"files":[],"warnings":[]}`,
		`{"files":[{"path":"gen/a.go","content":""}],"warnings":[{"at":"a.f","message":"m"}],"errors":[{"code":"X-1","at":"a.f","message":"m"}],"select":{"roots":["a.f"],"stops":[],"states":[],"wraps":[{"op":"a.f","decorator":"a.d"}]}}`,
		// 省ける項目は、なくても null でもよい
		`{"files":[],"warnings":[],"errors":null,"select":null}`,
		`{"files":[],"warnings":[{"message":"at は省ける"}],"errors":[{"code":"X-1","message":"m"}]}`,
		`{"files":[],"warnings":[],"select":{}}`,
		`{"files":[],"warnings":[],"select":{"roots":["a.f"]}}`,
		`{"files":[],"warnings":[],"select":{"roots":null,"wraps":null}}`,
		// 版 0.3 までの select は、知らない項目として読まれないだけで通る。中身の形も見ない（docs/plugin-protocol.md「応答の形」）
		`{"files":[],"warnings":[],"select":[]}`,
		`{"files":[],"warnings":[],"select":{"roots":"a.f","wraps":[{"op":1}]}}`,
		// 知らない項目は、どの階層でも無視する
		`{"files":[{"path":"p","content":"c","mode":"0644"}],"warnings":[{"message":"m","severity":"info"}],"future":{"x":1},"select":{"roots":[],"hints":[1,2]}}`,
		// emit の errors の severity は見ない
		`{"files":[],"warnings":[],"errors":[{"severity":"fatal","code":"X-1","message":"m"}]}`,
	}
	for _, in := range ok {
		if err := checkShape([]byte(wrapResult(in)), &sdk.EmitBatchResponse{}); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
	ng := map[string]string{
		`{"unknown":1}`:                    "files がない",
		`{}`:                               "files がない",
		`{"files":[]}`:                     "warnings がない",
		`{"warnings":[]}`:                  "files がない",
		`{"files":null,"warnings":[]}`:     "files がない",
		`{"files":[],"warnings":null}`:     "warnings がない",
		`{"Files":[],"warnings":[]}`:       "files がない", // 名前は大文字と小文字を区別する
		`[]`:                               "results[0] がオブジェクトでない",
		`"x"`:                              "results[0] がオブジェクトでない",
		`null`:                             "results[0] がオブジェクトでない",
		`{"files":{},"warnings":[]}`:       "files が配列でない",
		`{"files":"a.go","warnings":[]}`:   "files が配列でない",
		`{"files":["a.go"],"warnings":[]}`: "files[0] がオブジェクトでない",
		`{"files":[{"content":"c"}],"warnings":[]}`:              "files[0] に path がない",
		`{"files":[{"path":"p"}],"warnings":[]}`:                 "files[0] に content がない",
		`{"files":[{"path":1,"content":"c"}],"warnings":[]}`:     "files[0] の path が文字列でない",
		`{"files":[{"path":"p","content":null}],"warnings":[]}`:  "files[0] に content がない",
		`{"files":[{"path":"p","content":["c"]}],"warnings":[]}`: "files[0] の content が文字列でない",
		`{"files":[],"warnings":["careful"]}`:                    "warnings[0] がオブジェクトでない",
		`{"files":[],"warnings":[{"at":"a.f"}]}`:                 "warnings[0] に message がない",
		`{"files":[],"warnings":[{"at":3,"message":"m"}]}`:       "warnings[0] の at が文字列でない",
		`{"files":[],"warnings":[],"errors":{}}`:                 "errors が配列でない",
		`{"files":[],"warnings":[],"errors":[{"message":"m"}]}`:  "errors[0] に code がない",
		`{"files":[],"warnings":[],"errors":[{"code":"X-1"}]}`:   "errors[0] に message がない",
	}
	for in, want := range ng {
		err := checkShape([]byte(wrapResult(in)), &sdk.EmitBatchResponse{})
		if err == nil || !strings.Contains(err.Error(), "outputs[0].results[0]") || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", in, err, want)
		}
	}
}

// ほかの型（import の応答、テストの型）は確かめない。
func TestOtherShapesUnchecked(t *testing.T) {
	if err := checkShape([]byte(`{"unknown":1}`), &sdk.ImportResponse{}); err != nil {
		t.Errorf("ImportResponse: %v", err)
	}
}

// coverage の応答の形（docs/plugin-protocol.md「応答の形」「coverage」）。
func TestCoverageShape(t *testing.T) {
	ok := []string{
		`{"recognized":false,"files":[]}`,
		`{"recognized":true,"files":[{"path":"gen/a.go","lines":[[1,1],[3,9]],"extra":1},{"path":"gen/b.go","lines":[]}],"extra":true}`,
	}
	for _, in := range ok {
		if err := checkShape([]byte(in), &sdk.CoverageResponse{}); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
	ng := map[string]string{
		`[]`:                                                                  "応答がオブジェクトでない",
		`{"files":[]}`:                                                        "recognized がない",
		`{"recognized":null,"files":[]}`:                                      "recognized がない",
		`{"recognized":"yes","files":[]}`:                                     "recognized が真偽値でない",
		`{"recognized":true}`:                                                 "files がない",
		`{"recognized":true,"files":{}}`:                                      "files が配列でない",
		`{"recognized":true,"files":["a"]}`:                                   "files[0] がオブジェクトでない",
		`{"recognized":true,"files":[{"lines":[]}]}`:                          "files[0] に path がない",
		`{"recognized":true,"files":[{"path":"a.go"}]}`:                       "files[0] に lines がない",
		`{"recognized":true,"files":[{"path":"a.go","lines":null}]}`:          "files[0] に lines がない",
		`{"recognized":true,"files":[{"path":"a.go","lines":{}}]}`:            "files[0] の lines が配列でない",
		`{"recognized":true,"files":[{"path":"a.go","lines":[1,2]}]}`:         "files[0] の lines[0] が",
		`{"recognized":true,"files":[{"path":"a.go","lines":[[1]]}]}`:         "files[0] の lines[0] が",
		`{"recognized":true,"files":[{"path":"a.go","lines":[[1,2,3]]}]}`:     "files[0] の lines[0] が",
		`{"recognized":true,"files":[{"path":"a.go","lines":[[1,2],[0,1]]}]}`: "files[0] の lines[1] が",
		`{"recognized":true,"files":[{"path":"a.go","lines":[[3,2]]}]}`:       "files[0] の lines[0] が",
		`{"recognized":true,"files":[{"path":"a.go","lines":[[1.5,2]]}]}`:     "files[0] の lines[0] が",
		`{"recognized":true,"files":[{"path":"a.go","lines":[["1","2"]]}]}`:   "files[0] の lines[0] が",
	}
	for in, want := range ng {
		err := checkShape([]byte(in), &sdk.CoverageResponse{})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", in, err, want)
		}
	}
}

// 起動を通して: 形が違う応答は終了コード 0 でも hozo.compiler.MalformedOutput で、形が合えば知らない項目があっても読める
// （docs/plugin-protocol.md「起動」の表、「応答の形」）。echo-plugin は要求をそのまま応答に写す。
func TestRunResponseShape(t *testing.T) {
	run := func(req map[string]any, resp any) error {
		return Runner{}.Run("echo", echoWasm, req, resp)
	}
	result := func(fields map[string]any) map[string]any {
		return map[string]any{"mode": "echo", "outputs": []any{map[string]any{"results": []any{fields}}}}
	}
	var emit sdk.EmitBatchResponse
	err := run(map[string]any{"mode": "echo", "unknown": 1}, &emit)
	var mo *MalformedOutputError
	if !errors.Is(err, ErrMalformedOutput) || !errors.As(err, &mo) || mo.ExitCode != 0 || !strings.Contains(err.Error(), "outputs がない") {
		t.Errorf("emit, 必須の項目がない: err = %v", err)
	}
	if err := run(result(map[string]any{"files": []any{}, "warnings": "none"}), &emit); !errors.Is(err, ErrMalformedOutput) {
		t.Errorf("emit, 型が違う: err = %v", err)
	}
	emit = sdk.EmitBatchResponse{}
	err = run(result(map[string]any{"future": 1, "warnings": []any{},
		"files":  []any{map[string]any{"path": "gen/a.txt", "content": "hi", "mode": "0644"}},
		"select": map[string]any{"roots": []any{"a.f"}}}), &emit)
	if err != nil || len(emit.Outputs) != 1 || len(emit.Outputs[0].Results) != 1 {
		t.Fatalf("emit, 知らない項目: err = %v resp = %+v", err, emit)
	}
	if files := emit.Outputs[0].Results[0].Files; len(files) != 1 || files[0].Path != "gen/a.txt" || files[0].Content != "hi" {
		t.Errorf("emit, 知らない項目: resp = %+v", emit)
	}
}

// docs/plugin-protocol.md「応答の形」の、read の応答: errors と warnings は、どちらも必須（空でも []）。要素は、emit のものと同じ。
func TestReadShape(t *testing.T) {
	ok := []string{
		`{"errors":[],"warnings":[]}`,
		`{"errors":[{"code":"HTTP-1","at":"a.f","message":"m","severity":"warning"},{"code":"HTTP-2","message":"m","at":null}],"warnings":[{"message":"w"},{"at":"a.f","message":"w"}],"files":[1],"extra":true}`,
	}
	for _, in := range ok {
		if err := checkShape([]byte(in), &sdk.ReadResponse{}); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
	ng := map[string]string{
		`[]`:                            "応答がオブジェクトでない",
		`{"warnings":[]}`:               "errors がない",
		`{"errors":null,"warnings":[]}`: "errors がない",
		`{"errors":[]}`:                 "warnings がない",
		`{"errors":{},"warnings":[]}`:   "errors が配列でない",
		`{"errors":[1],"warnings":[]}`:  "errors[0] がオブジェクトでない",
		`{"errors":[{"message":"m"}],"warnings":[]}`:                   "errors[0] に code がない",
		`{"errors":[{"code":"X"}],"warnings":[]}`:                      "errors[0] に message がない",
		`{"errors":[{"code":"X","message":"m","at":1}],"warnings":[]}`: "errors[0] の at が文字列でない",
		`{"errors":[],"warnings":[{"at":"a.f"}]}`:                      "warnings[0] に message がない",
	}
	for in, want := range ng {
		err := checkShape([]byte(in), &sdk.ReadResponse{})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", in, err, want)
		}
	}
}
