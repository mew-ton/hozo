package golang

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/plugin"
)

// テストの対応表（docs/plugin-go.md「テストの対応表」）: テスト一つは、いちばん内側の t.Run。名前は、段を / でつなぐ。
// 行は、その場の関数の本体の、{ の次の行から、} の前の行まで。
func TestTestMap(t *testing.T) {
	src := `package xtest

import "testing"

func RunLeaves(t *testing.T, c LeafCases) {
	t.Run("x.one", func(t *testing.T) {
		cases := c.One()
		t.Run("OK", func(t *testing.T) {
			k := cases.OK()
			_ = k
		})
		t.Run("NotFound", func(t *testing.T) {
			k := cases.NotFound()
			if k.Then != nil {
				k.Then(t)
			}
		})
	})
	t.Run("x.two", func(t *testing.T) {
		send := func(t *testing.T) int {
			return 1
		}
		t.Run("OK", func(t *testing.T) {
			t.Run("GET /a b", func(t *testing.T) {
				_ = send(t)
			})
			t.Run("POST /a", func(t *testing.T) {
				_ = send(t)
			})
		})
	})
}

func RunFlat(t *testing.T, c Flat) {
	t.Run("flat.A", func(t *testing.T) {
		c.A(t)
	})
}

func helper(t *testing.T) {
	t.Helper()
}
`
	got, err := TestMap("gen/x/xtest/leaves.go", src)
	if err != nil {
		t.Fatal(err)
	}
	file := "gen/x/xtest/leaves.go"
	want := []plugin.TestEntry{
		{Name: "x.one/OK", File: file, Lines: [2]int{9, 10}},
		{Name: "x.one/NotFound", File: file, Lines: [2]int{13, 16}},
		{Name: "x.two/OK/GET /a b", File: file, Lines: [2]int{25, 25}},
		{Name: "x.two/OK/POST /a", File: file, Lines: [2]int{28, 28}},
		{Name: "flat.A", File: file, Lines: [2]int{36, 36}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TestMap:\n got %+v\nwant %+v", got, want)
	}

	// t.Run のないソースは、空
	if got, err := TestMap("doc.go", "package xtest\n"); err != nil || len(got) != 0 {
		t.Errorf("no t.Run: %+v, %v", got, err)
	}
}

// 約束の形でない t.Run は、誤り（生成器の不具合）。
func TestTestMapRejects(t *testing.T) {
	head := "package xtest\n\nimport \"testing\"\n\n"
	for _, c := range []struct{ name, body, want string }{
		{"name is not a literal", "func Run(t *testing.T, n string) {\n\tt.Run(n, func(t *testing.T) {\n\t\t_ = n\n\t})\n}\n", "文字列のリテラルでない"},
		{"body is a method value", "func Run(t *testing.T, c C) {\n\tt.Run(\"a\", c.A)\n}\n", "その場の関数でない"},
		{"one-line body", "func Run(t *testing.T, c C) {\n\tt.Run(\"a\", func(t *testing.T) { c.A(t) })\n}\n", "空か、一行"},
		{"empty body", "func Run(t *testing.T) {\n\tt.Run(\"a\", func(t *testing.T) {\n\t})\n}\n", "空か、一行"},
		{"not go", "func Run(", "leaves.go"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := TestMap("leaves.go", head+c.body)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

// カバレッジを読む（docs/plugin-go.md「カバレッジを読む」）。
func TestReadCoverProfile(t *testing.T) {
	for _, mode := range []string{"set", "count", "atomic"} {
		p, recognized, err := ReadCoverProfile("mode: " + mode + "\n" +
			"example.com/m/gen/x/xtest/leaves.go:5.43,6.36 1 1\n" +
			"example.com/m/gen/x/xtest/leaves.go:6.36,8.35 2 3\n" +
			"example.com/m/gen/x/xtest/leaves.go:8.35,11.4 2 0\n" +
			"\n" +
			// 同じ区切りが、二度（パッケージごとのテストの実行を、一つにまとめたファイル）: どれかが 1 以上なら、実行済み
			"example.com/m/gen/x/xtest/leaves.go:12.3,12.41 1 0\n" +
			"example.com/m/gen/x/xtest/leaves.go:12.3,12.41 1 2\n" +
			"example.com/m/gen/x/xtest/leaves.go:8.35,11.4 2 0\n" +
			"example.com/m/gen/x/other.go:1.1,2.2 1 0\n")
		if err != nil || !recognized {
			t.Fatalf("mode %s: recognized = %v, err = %v", mode, recognized, err)
		}
		lines, measured := p.Lines("example.com/m/gen/x/xtest/leaves.go")
		if want := [][2]int{{5, 6}, {6, 8}, {12, 12}}; !measured || !reflect.DeepEqual(lines, want) {
			t.Errorf("mode %s: lines = %v, measured = %v; want %v", mode, lines, measured, want)
		}
		// 測ったが、どの行も実行されていない: 空の列（nil ではない）
		if lines, measured := p.Lines("example.com/m/gen/x/other.go"); !measured || lines == nil || len(lines) != 0 {
			t.Errorf("mode %s: other.go: lines = %v, measured = %v", mode, lines, measured)
		}
		// 測っていないファイル
		if lines, measured := p.Lines("example.com/m/gen/x/xtest/classify.go"); measured || lines != nil {
			t.Errorf("mode %s: classify.go: lines = %v, measured = %v", mode, lines, measured)
		}
	}

	// 頭の行が違うもの: 読める形ではない（誤りではない）
	for _, content := range []string{"", "TN:\nSF:src/a.ts\n", "{\"total\": {}}", "mode: other\n", "example.com/m/a.go:1.1,2.2 1 1\n"} {
		if p, recognized, err := ReadCoverProfile(content); recognized || err != nil || p != nil {
			t.Errorf("%q: recognized = %v, err = %v", content, recognized, err)
		}
	}
	// CRLF の頭の行も、読む
	if _, recognized, err := ReadCoverProfile("mode: set\r\nexample.com/m/a.go:1.1,2.2 1 1\r\n"); !recognized || err != nil {
		t.Errorf("CRLF: recognized = %v, err = %v", recognized, err)
	}

	// 壊れた行: 読める形の頭を持つのに、途中が違う
	for _, line := range []string{
		"example.com/m/a.go",
		"example.com/m/a.go:1.1,2.2 1",
		"example.com/m/a.go:1.1,2.2 x 1",
		"example.com/m/a.go:1.1,2.2 1 x",
		"example.com/m/a.go:1.1,2.2 1 -1",
		"example.com/m/a.go:1.1 1 1",
		"example.com/m/a.go:1,2 1 1",
		"example.com/m/a.go:3.1,2.2 1 1",
		"example.com/m/a.go:0.1,2.2 1 1",
		":1.1,2.2 1 1",
	} {
		_, recognized, err := ReadCoverProfile("mode: set\nexample.com/m/a.go:1.1,2.2 1 1\n" + line + "\n")
		if !recognized || err == nil || !strings.Contains(err.Error(), "3 行目") {
			t.Errorf("%q: recognized = %v, err = %v; want an error naming line 3", line, recognized, err)
		}
	}
}
