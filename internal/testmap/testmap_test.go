package testmap

import (
	"reflect"
	"strings"
	"testing"

	sdk "github.com/mew-ton/hozo/plugin"
)

func TestIsMap(t *testing.T) {
	for path, want := range map[string]bool{
		"gen/server/servertest/hozo-tests.go.json":      true,
		"gen/server/servertest/hozo-tests.go-http.json": true,
		"gen/server/servertest/leaves.go":               false,
		"gen/server/hozo-tests.json":                    false,
		"gen/server/.hozoc-outputs.json":                false,
	} {
		if got := IsMap(path); got != want {
			t.Errorf("IsMap(%q) = %v, want %v", path, got, want)
		}
	}
}

// 対応表の形（docs/plugin-protocol.md「テストの対応表」）。
func TestParse(t *testing.T) {
	files := map[string]bool{"gen/x/xtest/leaves.go": true, "gen/x/xtest/classify.go": true}
	const path = "gen/x/xtest/hozo-tests.go.json"
	good := `{"tests": [
		{"name": "x.one/OK", "file": "gen/x/xtest/leaves.go", "lines": [9, 10], "future": true},
		{"name": "x.Stage/Draft", "file": "gen/x/xtest/classify.go", "lines": [4, 4]}
	], "future": 1}`
	m, err := Parse(sdk.File{Path: path, Content: good}, "go", files)
	if err != nil {
		t.Fatal(err)
	}
	want := []sdk.TestEntry{
		{Name: "x.one/OK", File: "gen/x/xtest/leaves.go", Lines: [2]int{9, 10}},
		{Name: "x.Stage/Draft", File: "gen/x/xtest/classify.go", Lines: [2]int{4, 4}},
	}
	if !reflect.DeepEqual(m.Tests, want) {
		t.Errorf("tests = %+v, want %+v", m.Tests, want)
	}
	// 空の tests は、形としては合う
	if m, err := Parse(sdk.File{Path: path, Content: `{"tests": []}`}, "go", files); err != nil || len(m.Tests) != 0 {
		t.Errorf("empty: %+v, %v", m, err)
	}

	entry := func(body string) string { return `{"tests": [` + body + `]}` }
	for _, c := range []struct{ name, path, content, want string }{
		{"another transpiler's name", "gen/x/xtest/hozo-tests.go-http.json", good, `"go-http"`},
		{"not JSON", path, "tests:", "JSON のオブジェクトでない"},
		{"an array", path, "[]", "JSON のオブジェクトでない"},
		{"null", path, "null", "JSON のオブジェクトでない"},
		{"no tests", path, "{}", "tests がない"},
		{"tests is null", path, `{"tests": null}`, "tests がない"},
		{"tests is not an array", path, `{"tests": {}}`, "tests が配列でない"},
		{"element is not an object", path, entry(`"x"`), "tests[0] がオブジェクトでない"},
		{"no name", path, entry(`{"file": "gen/x/xtest/leaves.go", "lines": [1, 2]}`), "tests[0] に name がない"},
		{"no file", path, entry(`{"name": "a", "lines": [1, 2]}`), "tests[0] に file がない"},
		{"no lines", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go"}`), "tests[0] に lines がない"},
		{"name is not a string", path, entry(`{"name": 1, "file": "gen/x/xtest/leaves.go", "lines": [1, 2]}`), "name が文字列でない"},
		{"file is not a string", path, entry(`{"name": "a", "file": [], "lines": [1, 2]}`), "file が文字列でない"},
		{"lines is not an array", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go", "lines": "1-2"}`), "lines が"},
		{"lines has one element", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go", "lines": [1]}`), "lines が"},
		{"lines has three elements", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go", "lines": [1, 2, 3]}`), "lines が"},
		{"lines is not integers", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go", "lines": [1.5, 2]}`), "lines が"},
		{"lines is strings", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go", "lines": ["1", "2"]}`), "lines が"},
		{"lines is null", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go", "lines": null}`), "lines がない"},
		{"line zero", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go", "lines": [0, 2]}`), "lines が"},
		{"start after end", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go", "lines": [3, 2]}`), "lines が"},
		{"empty name", path, entry(`{"name": "", "file": "gen/x/xtest/leaves.go", "lines": [1, 2]}`), "name が空"},
		{"duplicate name", path, entry(`{"name": "a", "file": "gen/x/xtest/leaves.go", "lines": [1, 2]}, {"name": "a", "file": "gen/x/xtest/leaves.go", "lines": [4, 5]}`), `name "a" が二度`},
		{"file not in the response", path, entry(`{"name": "a", "file": "gen/x/xtest/http.go", "lines": [1, 2]}`), "同じ応答の files にない"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(sdk.File{Path: c.path, Content: c.content}, "go", files)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

// 突き合わせ（docs/cli.md「tests」の 5）。
func TestMatch(t *testing.T) {
	test := func(transpiler, name, file string, first, last int) Test {
		return Test{Transpiler: transpiler, TestEntry: sdk.TestEntry{Name: name, File: file, Lines: [2]int{first, last}}}
	}
	tests := []Test{
		test("go", "x.one/OK", "gen/x/xtest/leaves.go", 9, 10),
		test("go", "x.one/NotFound", "gen/x/xtest/leaves.go", 13, 16),
		test("go-http", "x.one/OK", "gen/x/xtest/http.go", 20, 24),
		test("go-design", "flows.f", "gen/f/ftest/design.go", 30, 30),
	}
	if tests[0].Label() != "go: x.one/OK" || tests[2].Label() != "go-http: x.one/OK" {
		t.Errorf("labels = %q, %q", tests[0].Label(), tests[2].Label())
	}
	for _, c := range []struct {
		name    string
		covered []sdk.CoveredFile
		want    []Status
	}{
		{"all ran", []sdk.CoveredFile{
			{Path: "gen/x/xtest/leaves.go", Lines: [][2]int{{8, 9}, {12, 14}}},
			{Path: "gen/x/xtest/http.go", Lines: [][2]int{{19, 22}}},
			{Path: "gen/f/ftest/design.go", Lines: [][2]int{{29, 31}}},
		}, []Status{Ran, Ran, Ran, Ran}},
		{"some did not run", []sdk.CoveredFile{
			{Path: "gen/x/xtest/leaves.go", Lines: [][2]int{{8, 9}}},
			{Path: "gen/x/xtest/http.go", Lines: [][2]int{{19, 22}}},
			{Path: "gen/f/ftest/design.go", Lines: [][2]int{{28, 29}, {31, 31}}},
		}, []Status{Ran, NotRun, Ran, NotRun}},
		// 口を呼ぶ一行を書いていない: ファイルは測られているが、どの行も実行されていない
		{"the harness is never called", []sdk.CoveredFile{
			{Path: "gen/x/xtest/leaves.go", Lines: [][2]int{}},
			{Path: "gen/x/xtest/http.go", Lines: nil},
			{Path: "gen/f/ftest/design.go", Lines: [][2]int{{29, 31}}},
		}, []Status{NotRun, NotRun, NotRun, Ran}},
		// 外の段の文（t.Run( の行で終わる区切り）だけが実行済み: 絞って走らせて、中の関数が呼ばれなかった
		{"only the enclosing statements ran", []sdk.CoveredFile{
			{Path: "gen/x/xtest/leaves.go", Lines: [][2]int{{6, 8}, {12, 12}, {17, 17}}},
			{Path: "gen/x/xtest/http.go", Lines: [][2]int{{18, 19}, {25, 25}}},
			{Path: "gen/f/ftest/design.go", Lines: [][2]int{{29, 29}, {31, 32}}},
		}, []Status{NotRun, NotRun, NotRun, NotRun}},
		{"a file is not measured", []sdk.CoveredFile{
			{Path: "gen/x/xtest/leaves.go", Lines: [][2]int{{9, 9}, {16, 16}}},
		}, []Status{Ran, Ran, Unmeasured, Unmeasured}},
		{"nothing is measured", nil, []Status{Unmeasured, Unmeasured, Unmeasured, Unmeasured}},
		// カバレッジに、対応表にないファイルがある: 何も言わない。同じ path が二度あれば、合わせて見る
		{"unknown files and split entries", []sdk.CoveredFile{
			{Path: "app/leaves.go", Lines: [][2]int{{1, 400}}},
			{Path: "gen/x/xtest/leaves.go", Lines: [][2]int{{10, 10}}},
			{Path: "gen/x/xtest/leaves.go", Lines: [][2]int{{13, 13}}},
			{Path: "gen/x/xtest/http.go", Lines: [][2]int{{24, 40}}},
			{Path: "gen/f/ftest/design.go", Lines: [][2]int{{30, 30}}},
		}, []Status{Ran, Ran, Ran, Ran}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Match(tests, c.covered); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Match = %v, want %v", got, c.want)
			}
		})
	}
	if got := Match(nil, nil); len(got) != 0 {
		t.Errorf("Match(nil) = %v", got)
	}
}
