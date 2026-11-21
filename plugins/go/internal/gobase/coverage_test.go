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

// direction "coverage"（docs/plugin-go.md「カバレッジを読む」）: 要求のファイルごとに、<そのディレクトリの import パス>/<ファイル名>
// の行を探し、回数が 1 以上の区切りの行の範囲を返す。測られていないファイルは、出さない。
func TestCoverage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/shop\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
		Files:    []string{"gen/server/servertest/http.go", "gen/server/servertest/leaves.go", "gen/flows/flowstest/design.go"}}
	got, err := Coverage(req, root)
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
	if got, err := Coverage(req, root); err != nil || got.Recognized || len(got.Files) != 0 {
		t.Errorf("lcov: %+v, %v", got, err)
	}
	// 壊れた行: 誤り（どのファイルの、何行目か）
	req.Coverage.Content = "mode: count\nexample.com/shop/gen/a.go:1.1,2.2 1\n"
	if _, err := Coverage(req, root); err == nil || !strings.Contains(err.Error(), "/tmp/cover.out") || !strings.Contains(err.Error(), "2 行目") {
		t.Errorf("broken line: err = %v", err)
	}
	// go.mod が見つからないルート: どのファイルも、測られていない
	req.Coverage.Content = profile
	if got, err := Coverage(req, t.TempDir()); err != nil || !got.Recognized || len(got.Files) != 0 {
		t.Errorf("no go.mod: %+v, %v", got, err)
	}
}

// Handler を通して: ルートは HOZO_PROJECT_ROOT。応答の files と lines は、空でも [] で出る（docs/plugin-protocol.md「応答の形」）。
func TestCoverageHandler(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/shop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := Handler{Getenv: func(k string) string {
		if k == "HOZO_PROJECT_ROOT" {
			return root
		}
		return ""
	}}
	request := func(content string) string {
		data, err := json.Marshal(plugin.CoverageRequest{Protocol: ir.Version, Direction: "coverage",
			Coverage: plugin.File{Path: "cover.out", Content: content}, Files: []string{"gen/x/xtest/leaves.go"}})
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
	// 壊れた行は、終了コード 1（hozoc からは proc.Crashed）
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
