package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hozoc tests（docs/cli.md「tests」）のテスト。プラグインは、sh で動く小さなもの（fakePlugin）: emit の応答に、テストの
// 対応表を入れ、direction "coverage" には、coverage.json をそのまま返す。

const (
	exitTestsNotRun      = 4 // TestsNotRun
	exitCoverageUnusable = 5 // CoverageUnusable
)

// testsSpec は、名前空間が一つの仕様。
const testsSpec = `namespace shop {
  model Item { id: string; }
  fn issue(item: Item): Item;
}
`

// baseWithMap は、基底 base の emit の応答: テストのパッケージのファイル一つと、その、テストの対応表（テストは二つ）。
const baseWithMap = `{"files":[
  {"path":"@DIR@/shoptest/leaves.txt","content":"1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"},
  {"path":"@DIR@/shoptest/hozo-tests.base.json","content":"{\"tests\":[{\"name\":\"shop.issue/OK\",\"file\":\"@DIR@/shoptest/leaves.txt\",\"lines\":[3,5]},{\"name\":\"shop.issue/NotFound\",\"file\":\"@DIR@/shoptest/leaves.txt\",\"lines\":[8,9]}]}"}
],"warnings":[]}`

const coverageEnabled = "\n[coverage]\nenabled = true\n"

// testsProject は、単位が一つ（namespace shop、基底 base、out は gen/shop）のプロジェクトを作り、hozoc emit で出力を置き、
// カバレッジのファイル cover.out を置く。base の emit の応答は emitResponse、マニフェストに足す行は manifestExtra、
// direction "coverage" の応答は coverage。
func testsProject(t *testing.T, manifestExtra, emitResponse, coverage string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "shop.hozo"), testsSpec)
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("shop", "gen/shop", `"base"`))
	fakePlugin(t, dir, "base", "go", manifestExtra, emitResponse, 0)
	write(t, filepath.Join(dir, "plugins", "base", "coverage.json"), coverage)
	if code, stdout, stderr := run(t, dir, nil, "emit"); code != 0 {
		t.Fatalf("emit: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	write(t, filepath.Join(dir, "cover.out"), "fake coverage\n")
	_ = os.Remove(filepath.Join(dir, "plugins", "base", requestsFile)) // emit の要求は、数えない
	return dir
}

// coverageRequests は、プラグイン name が受け取った direction "coverage" の要求を返し、記録を消す。
func coverageRequests(t *testing.T, dir, name string) []map[string]any {
	t.Helper()
	path := filepath.Join(dir, "plugins", name, requestsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	_ = os.Remove(path)
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var req map[string]any
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			t.Fatalf("要求が JSON でない: %v", err)
		}
		if req["direction"] == "coverage" {
			out = append(out, req)
		}
	}
	return out
}

const leavesPath = "gen/shop/shoptest/leaves.txt"

// すべて走った: 終了コード 0 で、求めるテストを、対応表の順に並べる。プラグインには、カバレッジの中身と、対応表が指すファイルを渡す。
func TestTestsAllRan(t *testing.T) {
	dir := testsProject(t, coverageEnabled, baseWithMap, `{"recognized":true,"files":[{"path":"`+leavesPath+`","lines":[[1,2],[5,7],[9,9]]}]}`)
	code, stdout, stderr := run(t, dir, nil, "tests", "--coverage", "cover.out")
	if code != 0 || stdout != "ran: base: shop.issue/OK\nran: base: shop.issue/NotFound\n2 tests ran\n" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	reqs := coverageRequests(t, dir, "base")
	if len(reqs) != 1 {
		t.Fatalf("coverage の要求が %d 個（want 1）", len(reqs))
	}
	cov, _ := reqs[0]["coverage"].(map[string]any)
	abs, _ := filepath.EvalSymlinks(dir)
	got, _ := filepath.EvalSymlinks(filepath.Dir(cov["path"].(string)))
	if reqs[0]["protocol"] != "0.19" || got != abs || filepath.Base(cov["path"].(string)) != "cover.out" || cov["content"] != "fake coverage\n" {
		t.Errorf("coverage の要求 = %v", reqs[0])
	}
	if files, _ := json.Marshal(reqs[0]["files"]); string(files) != `["`+leavesPath+`"]` {
		t.Errorf("coverage の要求の files = %s", files)
	}

	// json
	code, stdout, _ = run(t, dir, nil, "tests", "--coverage", "cover.out", "--format=json")
	var result struct {
		Warnings []any    `json:"warnings"`
		Ran      []string `json:"ran"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); code != 0 || err != nil || result.Warnings == nil || len(result.Warnings) != 0 ||
		strings.Join(result.Ran, ",") != "base: shop.issue/OK,base: shop.issue/NotFound" {
		t.Fatalf("json: code=%d err=%v stdout=%s", code, err, stdout)
	}

	// 絶対パスの --coverage も、同じ
	if code, _, stderr = run(t, dir, nil, "tests", "--coverage", filepath.Join(dir, "cover.out")); code != 0 {
		t.Fatalf("絶対パス: code=%d stderr=%q", code, stderr)
	}
	// 何も書き出していない（置き場は、emit の後のまま）
	if _, err := os.Stat(filepath.Join(dir, "gen", "shop", ".hozoc-outputs.json")); err != nil {
		t.Fatal(err)
	}
}

// 走っていないテストは、名前で並び、終了コードは 4（TESTS-1）。測っていないファイルのテストは、文面が違う。
func TestTestsNotRun(t *testing.T) {
	// 二つ目のテストの行（8〜9）と重なる、実行された行がない。端の行だけが重なるものは、走った
	dir := testsProject(t, coverageEnabled, baseWithMap, `{"recognized":true,"files":[{"path":"`+leavesPath+`","lines":[[1,3],[6,7],[10,10]]}]}`)
	code, stdout, stderr := run(t, dir, nil, "tests", "--coverage", "cover.out")
	want := leavesPath + ":8:1: error[TESTS-1]: 求めるテスト base: shop.issue/NotFound が、走っていない（カバレッジが、" + leavesPath + " の 8〜9 行目を、実行済みと言っていない）\n1 error, 0 warnings\n"
	if code != exitTestsNotRun || stdout != "" || stderr != want {
		t.Fatalf("code=%d stdout=%q\nstderr=%q\n  want=%q", code, stdout, stderr, want)
	}
	code, _, stderr = run(t, dir, nil, "tests", "--coverage", "cover.out", "--format=json")
	if code != exitTestsNotRun || !strings.Contains(stderr, `"error": "hozoc.TestsNotRun"`) || !strings.Contains(stderr, `"code": "TESTS-1"`) || !strings.Contains(stderr, `"errors": 1`) {
		t.Fatalf("json: code=%d stderr=%s", code, stderr)
	}

	// 口を呼んでいない: 測っているが、どの行も実行されていない。二つとも、名指す
	write(t, filepath.Join(dir, "plugins", "base", "coverage.json"), `{"recognized":true,"files":[{"path":"`+leavesPath+`","lines":[]}]}`)
	code, _, stderr = run(t, dir, nil, "tests", "--coverage", "cover.out")
	if code != exitTestsNotRun || !strings.Contains(stderr, ":3:1: error[TESTS-1]: 求めるテスト base: shop.issue/OK が、走っていない") ||
		!strings.Contains(stderr, ":8:1: error[TESTS-1]: 求めるテスト base: shop.issue/NotFound が、走っていない") || !strings.Contains(stderr, "2 errors, 0 warnings") {
		t.Fatalf("口を呼んでいない: code=%d stderr=%q", code, stderr)
	}

	// カバレッジが、そのファイルを測っていない（応答の files にない）。要求にないパスの要素は、読まない
	write(t, filepath.Join(dir, "plugins", "base", "coverage.json"), `{"recognized":true,"files":[{"path":"app/leaves.go","lines":[[1,100]]}]}`)
	code, _, stderr = run(t, dir, nil, "tests", "--coverage", "cover.out")
	if code != exitTestsNotRun || strings.Count(stderr, "が、走ったと確かめられない（カバレッジが、ファイル "+leavesPath+" を測っていない）") != 2 {
		t.Fatalf("測っていない: code=%d stderr=%q", code, stderr)
	}
}

// 置き場の出力が、今の仕様から集めたものと違えば、TESTS-2（終了コード 5）。カバレッジは、読まない。
func TestTestsStaleOutputs(t *testing.T) {
	dir := testsProject(t, coverageEnabled, baseWithMap, `{"recognized":true,"files":[]}`)
	write(t, filepath.Join(dir, "gen", "shop", "shoptest", "leaves.txt"), "edited\n")
	if err := os.Remove(filepath.Join(dir, "gen", "shop", "shoptest", "hozo-tests.base.json")); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, dir, nil, "tests", "--coverage", "cover.out")
	want := "gen/shop/shoptest/hozo-tests.base.json: error[TESTS-2]: gen/shop/shoptest/hozo-tests.base.json が、置き場にない（hozoc emit をやり直して、テストを走らせ直す）\n" +
		leavesPath + ": error[TESTS-2]: 置き場の " + leavesPath + " が、今の仕様から集めたものと違う（hozoc emit をやり直して、テストを走らせ直す）\n2 errors, 0 warnings\n"
	if code != exitCoverageUnusable || stdout != "" || stderr != want {
		t.Fatalf("code=%d stdout=%q\nstderr=%q\n  want=%q", code, stdout, stderr, want)
	}
	if reqs := coverageRequests(t, dir, "base"); len(reqs) != 0 {
		t.Errorf("置き場が違うのに、coverage を %d 回呼んだ", len(reqs))
	}
	code, _, stderr = run(t, dir, nil, "tests", "--coverage", "cover.out", "--format=json")
	if code != exitCoverageUnusable || !strings.Contains(stderr, `"error": "hozoc.CoverageUnusable"`) || !strings.Contains(stderr, `"code": "TESTS-2"`) {
		t.Fatalf("json: code=%d stderr=%s", code, stderr)
	}
}

// カバレッジを読めなければ、TESTS-3（終了コード 5）。
func TestTestsCoverageUnusable(t *testing.T) {
	t.Run("no file", func(t *testing.T) {
		dir := testsProject(t, coverageEnabled, baseWithMap, `{"recognized":true,"files":[]}`)
		code, stdout, stderr := run(t, dir, nil, "tests", "--coverage", "nope.out")
		if code != exitCoverageUnusable || stdout != "" || !strings.HasPrefix(stderr, "nope.out: error[TESTS-3]: カバレッジのファイル nope.out を読めない: ") || !strings.Contains(stderr, "1 error, 0 warnings") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if reqs := coverageRequests(t, dir, "base"); len(reqs) != 0 {
			t.Errorf("ファイルを読めないのに、coverage を %d 回呼んだ", len(reqs))
		}
	})
	t.Run("not UTF-8", func(t *testing.T) {
		dir := testsProject(t, coverageEnabled, baseWithMap, `{"recognized":true,"files":[]}`)
		if err := os.WriteFile(filepath.Join(dir, "cover.out"), []byte{0xff, 0xfe, 0x00}, 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := run(t, dir, nil, "tests", "--coverage", "cover.out")
		if code != exitCoverageUnusable || !strings.Contains(stderr, "cover.out: error[TESTS-3]: カバレッジのファイル cover.out が、UTF-8 の文字列でない") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})
	t.Run("the base plugin does not read coverage", func(t *testing.T) {
		dir := testsProject(t, "", baseWithMap, `{"recognized":true,"files":[]}`)
		code, _, stderr := run(t, dir, nil, "tests", "--coverage", "cover.out")
		if code != exitCoverageUnusable || !strings.Contains(stderr, "cover.out: error[TESTS-3]: 基底のプラグイン base は、カバレッジを読めない（マニフェストの [coverage] enabled が true でない）") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if reqs := coverageRequests(t, dir, "base"); len(reqs) != 0 {
			t.Errorf("[coverage] のないプラグインに、coverage を %d 回送った", len(reqs))
		}
	})
	t.Run("not recognized", func(t *testing.T) {
		dir := testsProject(t, coverageEnabled, baseWithMap, `{"recognized":false,"files":[]}`)
		code, _, stderr := run(t, dir, nil, "tests", "--coverage", "cover.out")
		if code != exitCoverageUnusable || !strings.Contains(stderr, "cover.out: error[TESTS-3]: 基底のプラグイン base は、カバレッジのファイル cover.out を、読める形と認めなかった") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})
	// 応答の形の誤りは、proc.MalformedOutput（docs/plugin-protocol.md「応答の形」）
	for name, response := range map[string]string{
		"no recognized":   `{"files":[]}`,
		"no files":        `{"recognized":true}`,
		"lines not pairs": `{"recognized":true,"files":[{"path":"` + leavesPath + `","lines":[[1]]}]}`,
		"no lines":        `{"recognized":true,"files":[{"path":"` + leavesPath + `"}]}`,
		"line zero":       `{"recognized":true,"files":[{"path":"` + leavesPath + `","lines":[[0,1]]}]}`,
		"start after end": `{"recognized":true,"files":[{"path":"` + leavesPath + `","lines":[[3,1]]}]}`,
	} {
		t.Run("malformed response/"+name, func(t *testing.T) {
			dir := testsProject(t, coverageEnabled, baseWithMap, response)
			code, _, stderr := run(t, dir, nil, "tests", "--coverage", "cover.out")
			if code != exitSoftwareOrCrash || !strings.Contains(stderr, "base") {
				t.Fatalf("code=%d stderr=%q", code, stderr)
			}
		})
	}
}

// 対応表が一つもなければ、求めるテストはない: カバレッジを読まずに、終了コード 0。
func TestTestsNoMaps(t *testing.T) {
	dir := testsProject(t, "", baseResponse, `{"recognized":true,"files":[]}`)
	code, stdout, stderr := run(t, dir, nil, "tests", "--coverage", "nope.out")
	if code != 0 || stdout != "0 tests ran\n" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, _ = run(t, dir, nil, "tests", "--coverage", "nope.out", "--format=json")
	if code != 0 || !strings.Contains(stdout, `"ran": []`) {
		t.Fatalf("json: code=%d stdout=%s", code, stdout)
	}
}

// 相乗りの対応表のテストも、単位の基底のプラグインが返した行と突き合わせる。同じ名前のテストが、二つのトランスパイラの
// 対応表にあってよい。プラグインに渡す files は、重複なしの辞書順。
func TestTestsRider(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "shop.hozo"), testsSpec)
	write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("shop", "gen/shop", `"rider", "base"`))
	fakePlugin(t, dir, "base", "go", coverageEnabled, baseWithMap, 0)
	fakeRider(t, dir, "rider", "go", `"base"`, `{"files":[
  {"path":"@DIR@/shoptest/http.txt","content":"x\n"},
  {"path":"@DIR@/shoptest/hozo-tests.rider.json","content":"{\"tests\":[{\"name\":\"shop.issue/OK\",\"file\":\"@DIR@/shoptest/http.txt\",\"lines\":[2,4]}]}"}
],"warnings":[{"message":"w"}]}`, 0)
	write(t, filepath.Join(dir, "plugins", "base", "coverage.json"),
		`{"recognized":true,"files":[{"path":"`+leavesPath+`","lines":[[3,9]]},{"path":"gen/shop/shoptest/http.txt","lines":[[5,6]]}]}`)
	if code, _, stderr := run(t, dir, nil, "emit"); code != 0 {
		t.Fatalf("emit: code=%d stderr=%q", code, stderr)
	}
	write(t, filepath.Join(dir, "cover.out"), "fake coverage\n")
	_ = os.Remove(filepath.Join(dir, "plugins", "base", requestsFile))
	code, stdout, stderr := run(t, dir, nil, "tests", "--coverage", "cover.out")
	// warning は、走っていないテストの後ろに並ぶ
	if code != exitTestsNotRun || stdout != "" || !strings.HasPrefix(stderr, "gen/shop/shoptest/http.txt:2:1: error[TESTS-1]: 求めるテスト rider: shop.issue/OK が、走っていない") ||
		strings.Contains(stderr, "base: shop.issue") || !strings.Contains(stderr, "warning[PLUGIN]: rider: w") || !strings.Contains(stderr, "1 error, 1 warning") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	reqs := coverageRequests(t, dir, "base")
	if len(reqs) != 1 {
		t.Fatalf("coverage の要求が %d 個（want 1）", len(reqs))
	}
	if files, _ := json.Marshal(reqs[0]["files"]); string(files) != `["gen/shop/shoptest/http.txt","`+leavesPath+`"]` {
		t.Errorf("coverage の要求の files = %s", files)
	}
	if reqs := coverageRequests(t, dir, "rider"); len(reqs) != 0 {
		t.Errorf("相乗りのプラグインに、coverage を %d 回送った", len(reqs))
	}

	// すべて走れば、求めるテストは、トランスパイラの順（transpilers に書いた順）、対応表の中の順。warning は、先に出す
	write(t, filepath.Join(dir, "plugins", "base", "coverage.json"),
		`{"recognized":true,"files":[{"path":"`+leavesPath+`","lines":[[3,9]]},{"path":"gen/shop/shoptest/http.txt","lines":[[4,6]]}]}`)
	code, stdout, stderr = run(t, dir, nil, "tests", "--coverage", "cover.out")
	if code != 0 || !strings.HasSuffix(stdout, "ran: rider: shop.issue/OK\nran: base: shop.issue/OK\nran: base: shop.issue/NotFound\n3 tests ran\n") || !strings.Contains(stdout, "warning[PLUGIN]: rider: w") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// 対応表の名前か中身が、約束の形でなければ、PLG-4（proc.MalformedOutput）。emit でも、tests でも。
func TestTestMapMalformed(t *testing.T) {
	file := `{"path":"@DIR@/shoptest/leaves.txt","content":"1\n"}`
	for name, mapFile := range map[string]string{
		"another transpiler's name": `{"path":"@DIR@/shoptest/hozo-tests.other.json","content":"{\"tests\":[]}"}`,
		"not JSON":                  `{"path":"@DIR@/shoptest/hozo-tests.base.json","content":"tests"}`,
		"file not in the response":  `{"path":"@DIR@/shoptest/hozo-tests.base.json","content":"{\"tests\":[{\"name\":\"a\",\"file\":\"@DIR@/shoptest/http.txt\",\"lines\":[1,1]}]}"}`,
		"bad lines":                 `{"path":"@DIR@/shoptest/hozo-tests.base.json","content":"{\"tests\":[{\"name\":\"a\",\"file\":\"@DIR@/shoptest/leaves.txt\",\"lines\":[2,1]}]}"}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "shop.hozo"), testsSpec)
			write(t, filepath.Join(dir, "hozo.toml"), pluginDirs+unit("shop", "gen/shop", `"base"`))
			fakePlugin(t, dir, "base", "go", coverageEnabled, `{"files":[`+file+`,`+mapFile+`],"warnings":[]}`, 0)
			write(t, filepath.Join(dir, "cover.out"), "fake coverage\n")
			for _, args := range [][]string{{"emit"}, {"tests", "--coverage", "cover.out"}} {
				code, stdout, stderr := run(t, dir, nil, args...)
				if code != exitSoftwareOrCrash || stdout != "" || !strings.Contains(stderr, "PLG-4: files の、テストの対応表") {
					t.Fatalf("%v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "gen")); !os.IsNotExist(err) {
				t.Errorf("書き出している")
			}
		})
	}
}

// --coverage がなければ、使い方の誤り（64）。hozo.toml がなければ、CFG-7（emit と同じ）。
func TestTestsUsage(t *testing.T) {
	dir := testsProject(t, coverageEnabled, baseWithMap, `{"recognized":true,"files":[]}`)
	if code, stdout, stderr := run(t, dir, nil, "tests"); code != exitUsage || stdout != "" || !strings.Contains(stderr, "coverage") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	bare := t.TempDir()
	write(t, filepath.Join(bare, "shop.hozo"), testsSpec)
	write(t, filepath.Join(bare, "cover.out"), "fake coverage\n")
	if code, _, stderr := run(t, bare, nil, "tests", "--coverage", "cover.out"); code != exitConfigInvalid || !strings.Contains(stderr, "hozo.toml: error[CFG-7]") {
		t.Fatalf("設定なし: code=%d stderr=%q", code, stderr)
	}
}
