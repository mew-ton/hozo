package gobase

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// coverOutput は、カバレッジの要求の outputs の一つ: 出力先 dir の、モジュール module の出力の設定。
func coverOutput(index int, dir, module string) ir.NamespaceConfig {
	c := ir.NamespaceConfig{Index: index, Language: "go", Base: "go", Out: dir, Dir: dir, Transpilers: []string{"go"},
		Options: map[string]map[string]any{}, Seams: map[string]string{}}
	if module != "" {
		c.Options["go"] = map[string]any{"module": module}
	}
	return c
}

// direction "coverage"（docs/plugin-go.md「カバレッジを読む」）: 要求のファイルごとに、<そのディレクトリの import パス>/<ファイル名>
// の行を探し、回数が 1 以上の区切りの行の範囲を返す。測られていないファイルは、出さない。モジュールの名前は、要求の outputs
// （ファイルを出した出力の設定）から取る: ファイルは、読まない。
func TestCoverage(t *testing.T) {
	profile := "mode: set\n" +
		"example.com/shop/gen/server/servertest/leaves.go:156.65,157.51 1 1\n" +
		"example.com/shop/gen/server/servertest/leaves.go:157.51,159.34 2 1\n" +
		"example.com/shop/gen/server/servertest/leaves.go:159.34,162.18 3 0\n" +
		"example.com/shop/gen/server/servertest/leaves.go:159.34,162.18 3 1\n" +
		"example.com/shop/gen/server/servertest/leaves.go:169.50,173.30 4 0\n" +
		"example.com/shop/gen/server/servertest/http.go:10.2,12.3 1 0\n" +
		"example.com/shop/app/leaves.go:1.1,9.2 3 1\n"
	req := &plugin.CoverageRequest{Protocol: ir.Version, Direction: "coverage",
		Coverage: plugin.File{Path: "/tmp/cover.out", Content: profile},
		Files:    []string{"gen/server/servertest/http.go", "gen/server/servertest/leaves.go", "gen/flows/flowstest/design.go"},
		Outputs:  []ir.NamespaceConfig{coverOutput(1, "gen/server", "example.com/shop"), coverOutput(2, "gen/flows", "example.com/shop")}}
	got, err := Coverage(req)
	if err != nil {
		t.Fatal(err)
	}
	want := &plugin.CoverageResponse{Recognized: true, Files: []plugin.CoveredFile{
		{Path: "gen/server/servertest/http.go", Lines: [][2]int{}}, // 測ったが、どの行も実行されていない
		{Path: "gen/server/servertest/leaves.go", Lines: [][2]int{{156, 157}, {157, 159}, {159, 162}}},
		// gen/flows/flowstest/design.go は、カバレッジに行がない: 測られていない
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Coverage:\n got %+v\nwant %+v", got, want)
	}

	// 読める形でないファイル
	req.Coverage.Content = "TN:\nSF:src/a.ts\nend_of_record\n"
	if got, err := Coverage(req); err != nil || got.Recognized || len(got.Files) != 0 {
		t.Errorf("lcov: %+v, %v", got, err)
	}
	// 壊れた行: 誤り（どのファイルの、何行目か）
	req.Coverage.Content = "mode: count\nexample.com/shop/gen/a.go:1.1,2.2 1\n"
	if _, err := Coverage(req); err == nil || !strings.Contains(err.Error(), "/tmp/cover.out") || !strings.Contains(err.Error(), "2 行目") {
		t.Errorf("broken line: err = %v", err)
	}
	// ファイルのディレクトリか、その上にある dir のうち、いちばん深い出力の module を使う
	req.Coverage.Content = profile
	req.Outputs = []ir.NamespaceConfig{coverOutput(1, "gen", "example.com/wrong"), coverOutput(2, "gen/server", "example.com/shop"),
		coverOutput(3, "gen/server/servertest/deeper", "example.com/wrong")}
	if got, err := Coverage(req); err != nil || len(got.Files) != 2 {
		t.Errorf("いちばん深い出力: %+v, %v", got, err)
	}
	// ルートが出力先（dir が "."）の出力は、どのファイルの上にもある
	req.Outputs = []ir.NamespaceConfig{coverOutput(1, ".", "example.com/shop")}
	if got, err := Coverage(req); err != nil || len(got.Files) != 2 {
		t.Errorf("dir が .: %+v, %v", got, err)
	}
	// 出力が分からない（outputs にない）、module がない: どのファイルも、測られていない
	for name, outputs := range map[string][]ir.NamespaceConfig{
		"outputs がない": nil, "別の出力先": {coverOutput(1, "gen/other", "example.com/shop")}, "module がない": {coverOutput(1, "gen/server", "")},
	} {
		req.Outputs = outputs
		if got, err := Coverage(req); err != nil || !got.Recognized || len(got.Files) != 0 {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
	}
}

// Handler を通して: 応答の files と lines は、空でも [] で出る（docs/plugin-protocol.md「応答の形」）。
func TestCoverageHandler(t *testing.T) {
	h := Handler{}
	request := func(content string) string {
		data, err := json.Marshal(plugin.CoverageRequest{Protocol: ir.Version, Direction: "coverage",
			Coverage: plugin.File{Path: "cover.out", Content: content}, Files: []string{"gen/x/xtest/leaves.go"},
			Outputs: []ir.NamespaceConfig{coverOutput(1, "gen/x", "example.com/shop")}})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	var out, errOut bytes.Buffer
	if code := plugin.Run(h, strings.NewReader(request("mode: atomic\nexample.com/shop/gen/x/xtest/leaves.go:3.1,4.2 1 0\n")), &out, &errOut); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, errOut.String())
	}
	if want := `{"recognized":true,"files":[{"path":"gen/x/xtest/leaves.go","lines":[]}]}` + "\n"; out.String() != want {
		t.Errorf("response = %s, want %s", out.String(), want)
	}
	out.Reset()
	if code := plugin.Run(h, strings.NewReader(request("not a profile")), &out, &errOut); code != 0 || out.String() != `{"recognized":false,"files":[]}`+"\n" {
		t.Errorf("not recognized: code = %d, response = %s", code, out.String())
	}
	// 壊れた行は、終了コード 1（hozoc からは hozo.compiler.Crashed）
	out.Reset()
	if code := plugin.Run(h, strings.NewReader(request("mode: set\nbroken\n")), &out, &errOut); code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "2 行目") {
		t.Errorf("broken: code = %d, stdout = %s, stderr = %s", code, out.String(), errOut.String())
	}
}

// リポジトリのマニフェストが、[coverage] enabled を言っている（docs/plugin-go.md「マニフェスト」）。
func TestManifestDeclaresCoverage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "hozo-plugin.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\n[coverage]\nenabled = true\n") {
		t.Errorf("plugins/go/hozo-plugin.toml に [coverage] enabled = true がない:\n%s", data)
	}
}
