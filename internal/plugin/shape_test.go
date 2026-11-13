package plugin

import (
	"errors"
	"strings"
	"testing"

	sdk "github.com/mew-ton/hozo/plugin"
)

// docs/plugin-protocol.md「応答の形」: 必須の項目と型を確かめ、知らない項目は無視する。
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
		// 知らない項目は、どの階層でも無視する
		`{"files":[{"path":"p","content":"c","mode":"0644"}],"warnings":[{"message":"m","severity":"info"}],"future":{"x":1},"select":{"roots":[],"hints":[1,2]}}`,
		// emit の errors の severity は見ない
		`{"files":[],"warnings":[],"errors":[{"severity":"fatal","code":"X-1","message":"m"}]}`,
	}
	for _, in := range ok {
		if err := checkShape([]byte(in), &sdk.EmitResponse{}); err != nil {
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
		`[]`:                               "応答がオブジェクトでない",
		`"x"`:                              "応答がオブジェクトでない",
		`null`:                             "応答がオブジェクトでない",
		`{"files":{},"warnings":[]}`:       "files が配列でない",
		`{"files":"a.go","warnings":[]}`:   "files が配列でない",
		`{"files":["a.go"],"warnings":[]}`: "files[0] がオブジェクトでない",
		`{"files":[{"content":"c"}],"warnings":[]}`:                         "files[0] に path がない",
		`{"files":[{"path":"p"}],"warnings":[]}`:                            "files[0] に content がない",
		`{"files":[{"path":1,"content":"c"}],"warnings":[]}`:                "files[0] の path が文字列でない",
		`{"files":[{"path":"p","content":null}],"warnings":[]}`:             "files[0] に content がない",
		`{"files":[{"path":"p","content":["c"]}],"warnings":[]}`:            "files[0] の content が文字列でない",
		`{"files":[],"warnings":["careful"]}`:                               "warnings[0] がオブジェクトでない",
		`{"files":[],"warnings":[{"at":"a.f"}]}`:                            "warnings[0] に message がない",
		`{"files":[],"warnings":[{"at":3,"message":"m"}]}`:                  "warnings[0] の at が文字列でない",
		`{"files":[],"warnings":[],"errors":{}}`:                            "errors が配列でない",
		`{"files":[],"warnings":[],"errors":[{"message":"m"}]}`:             "errors[0] に code がない",
		`{"files":[],"warnings":[],"errors":[{"code":"X-1"}]}`:              "errors[0] に message がない",
		`{"files":[],"warnings":[],"select":[]}`:                            "select がオブジェクトでない",
		`{"files":[],"warnings":[],"select":{"roots":"a.f"}}`:               "select.roots が配列でない",
		`{"files":[],"warnings":[],"select":{"stops":[1]}}`:                 "select.stops[0] が文字列でない",
		`{"files":[],"warnings":[],"select":{"states":[null]}}`:             "select.states[0] が文字列でない",
		`{"files":[],"warnings":[],"select":{"wraps":{}}}`:                  "select.wraps が配列でない",
		`{"files":[],"warnings":[],"select":{"wraps":["a.f"]}}`:             "select.wraps[0] がオブジェクトでない",
		`{"files":[],"warnings":[],"select":{"wraps":[{"op":"a.f"}]}}`:      "select.wraps[0] に decorator がない",
		`{"files":[],"warnings":[],"select":{"wraps":[{"decorator":"d"}]}}`: "select.wraps[0] に op がない",
	}
	for in, want := range ng {
		err := checkShape([]byte(in), &sdk.EmitResponse{})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", in, err, want)
		}
	}
}

func TestCheckResponseShape(t *testing.T) {
	ok := []string{
		`{"diagnostics":[]}`,
		`{"diagnostics":[{"severity":"error","code":"PAGE-1","message":"m","at":"browser.openOrders"},{"severity":"warning","code":"PAGE-2","message":"m"}],"extra":true}`,
	}
	for _, in := range ok {
		if err := checkShape([]byte(in), &sdk.CheckResponse{}); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
	ng := map[string]string{
		`{}`:                         "diagnostics がない",
		`{"diagnostics":null}`:       "diagnostics がない",
		`{"files":[],"warnings":[]}`: "diagnostics がない",
		`{"diagnostics":{}}`:         "diagnostics が配列でない",
		`{"diagnostics":[{"code":"P-1","message":"m"}]}`:                             "diagnostics[0] に severity がない",
		`{"diagnostics":[{"severity":"fatal","code":"P-1","message":"m"}]}`:          `"error" でも "warning" でもない`,
		`{"diagnostics":[{"severity":"error","message":"m"}]}`:                       "diagnostics[0] に code がない",
		`{"diagnostics":[{"severity":"error","code":"P-1"}]}`:                        "diagnostics[0] に message がない",
		`{"diagnostics":[{"severity":"error","code":"P-1","message":"m","at":[1]}]}`: "diagnostics[0] の at が文字列でない",
	}
	for in, want := range ng {
		err := checkShape([]byte(in), &sdk.CheckResponse{})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", in, err, want)
		}
	}
	// ほかの型（import の応答、テストの型）は確かめない
	if err := checkShape([]byte(`{"unknown":1}`), &sdk.ImportResponse{}); err != nil {
		t.Errorf("ImportResponse: %v", err)
	}
}

// 起動を通して: 形が違う応答は終了コード 0 でも proc.MalformedOutput で、形が合えば知らない項目があっても読める
// （docs/plugin-protocol.md「起動」の表、「応答の形」）。echo-plugin は要求をそのまま応答に写す。
func TestRunResponseShape(t *testing.T) {
	root := t.TempDir()
	run := func(req map[string]any, resp any) error {
		return Runner{Root: root}.Run(echoManifest(), root, req, resp)
	}
	var emit sdk.EmitResponse
	err := run(map[string]any{"mode": "echo", "unknown": 1}, &emit)
	var mo *MalformedOutputError
	if !errors.Is(err, ErrMalformedOutput) || !errors.As(err, &mo) || mo.ExitCode != 0 || !strings.Contains(err.Error(), "files がない") {
		t.Errorf("emit, 必須の項目がない: err = %v", err)
	}
	if err := run(map[string]any{"mode": "echo", "files": []any{}, "warnings": "none"}, &emit); !errors.Is(err, ErrMalformedOutput) {
		t.Errorf("emit, 型が違う: err = %v", err)
	}
	emit = sdk.EmitResponse{}
	err = run(map[string]any{"mode": "echo", "future": 1, "warnings": []any{},
		"files":  []any{map[string]any{"path": "gen/a.txt", "content": "hi", "mode": "0644"}},
		"select": map[string]any{"roots": []any{"a.f"}}}, &emit)
	if err != nil || len(emit.Files) != 1 || emit.Files[0].Path != "gen/a.txt" || emit.Files[0].Content != "hi" ||
		len(emit.Select.Roots) != 1 || emit.Select.Roots[0] != "a.f" {
		t.Errorf("emit, 知らない項目: err = %v resp = %+v", err, emit)
	}

	var check sdk.CheckResponse
	if err := run(map[string]any{"mode": "echo"}, &check); !errors.Is(err, ErrMalformedOutput) || !strings.Contains(err.Error(), "diagnostics がない") {
		t.Errorf("check, 必須の項目がない: err = %v", err)
	}
	if err := run(map[string]any{"mode": "echo", "diagnostics": []any{}}, &check); err != nil {
		t.Errorf("check: err = %v", err)
	}
}
