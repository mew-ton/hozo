package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/plugin"
)

// TestMap は、整形した後の Go のソース src（置き場は path。プロジェクトのルートからの相対パス）の、走らせる関数の中の
// テストを、行の順で返す（plugins/go/spec/go.hozo の Requirement.UnitTestPort の「テストの対応表」）。
//
// テスト一つは、いちばん内側の `t.Run(<名前>, func(t *testing.T) { … })`。Name は、外の段からその t.Run までの名前を
// `/` でつないだもの。Lines は、その場の関数の本体の、`{` の次の行から `}` の前の行まで。
// 名前が文字列のリテラルでない、中身がその場の関数でない、本体が空（`{` の次の行が `}` の行より後ろ）の t.Run があれば
// error（生成器の不具合）。t.Run を一つも持たないソースは、空の列。
func TestMap(path, src string) ([]plugin.TestEntry, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var out []plugin.TestEntry
	for _, d := range file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		runs, err := testRuns(fset, fn.Body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := collectTests(fset, path, nil, runs, &out); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return out, nil
}

// testRun は、`t.Run(<名前>, func…{ … })` 一つ。
type testRun struct {
	name string
	body *ast.BlockStmt
}

// testRuns は、body の中の t.Run を、ソースの順で返す。別の t.Run の、その場の関数の中のものは、含めない（その段の子）。
func testRuns(fset *token.FileSet, body *ast.BlockStmt) ([]testRun, error) {
	var runs []testRun
	var failed error
	ast.Inspect(body, func(n ast.Node) bool {
		if failed != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || !isTestRun(call) {
			return true
		}
		line := fset.Position(call.Pos()).Line
		if len(call.Args) != 2 {
			failed = fmt.Errorf("%d 行目の t.Run の引数が、二つでない", line)
			return false
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			failed = fmt.Errorf("%d 行目の t.Run の名前が、文字列のリテラルでない", line)
			return false
		}
		name, err := strconv.Unquote(lit.Value)
		if err != nil {
			failed = fmt.Errorf("%d 行目の t.Run の名前を読めない: %w", line, err)
			return false
		}
		fn, ok := call.Args[1].(*ast.FuncLit)
		if !ok {
			failed = fmt.Errorf("%d 行目の t.Run（%s）の中身が、その場の関数でない", line, name)
			return false
		}
		runs = append(runs, testRun{name: name, body: fn.Body})
		return false // 中は、この段の子として、後で読む
	})
	return runs, failed
}

// isTestRun は、call が `t.Run(…)` か。
func isTestRun(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Run" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "t"
}

// collectTests は、runs を辿り、子を持たない t.Run を、テストとして out に足す。
func collectTests(fset *token.FileSet, path string, outer []string, runs []testRun, out *[]plugin.TestEntry) error {
	for _, r := range runs {
		names := append(append([]string{}, outer...), r.name)
		children, err := testRuns(fset, r.body)
		if err != nil {
			return err
		}
		if len(children) > 0 {
			if err := collectTests(fset, path, names, children, out); err != nil {
				return err
			}
			continue
		}
		first, last := fset.Position(r.body.Lbrace).Line+1, fset.Position(r.body.Rbrace).Line-1
		if first > last {
			return fmt.Errorf("%d 行目の t.Run（%s）の本体が、空か、一行に書かれている", first-1, strings.Join(names, "/"))
		}
		*out = append(*out, plugin.TestEntry{Name: strings.Join(names, "/"), File: path, Lines: [2]int{first, last}})
	}
	return nil
}

// CoverProfile は、go test -coverprofile が書いたファイルの中身（plugins/go/spec/go.hozo の Requirement.UnitTestPort の「カバレッジを読む」）: ファイルの名前
// （<import パス>/<ファイル名>）ごとの、区切りと、実行されたか。
type CoverProfile struct {
	files map[string]map[[2]int]bool // 名前 → [始まりの行, 終わりの行] → 回数が 1 以上の行が、一つでもあったか
}

// ReadCoverProfile は content を go test -coverprofile の形として読む。最初の行が `mode: set|count|atomic` でなければ
// recognized が false（誤りではない）。二行目から先に、形の違う行があれば error。
func ReadCoverProfile(content string) (profile *CoverProfile, recognized bool, err error) {
	lines := strings.Split(content, "\n")
	head := strings.TrimRight(lines[0], "\r")
	switch head {
	case "mode: set", "mode: count", "mode: atomic":
	default:
		return nil, false, nil
	}
	p := &CoverProfile{files: map[string]map[[2]int]bool{}}
	for i, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		name, block, count, ok := coverLine(line)
		if !ok {
			return nil, true, fmt.Errorf("カバレッジの %d 行目が、<ファイル>:<行>.<列>,<行>.<列> <文の数> <回数> の形でない: %s", i+2, line)
		}
		if p.files[name] == nil {
			p.files[name] = map[[2]int]bool{}
		}
		p.files[name][block] = p.files[name][block] || count > 0
	}
	return p, true, nil
}

// coverLine は、カバレッジの一行 `<名前>:<行>.<列>,<行>.<列> <文の数> <回数>` を読む。
func coverLine(line string) (name string, block [2]int, count int, ok bool) {
	fields := strings.Split(line, " ")
	if len(fields) < 3 {
		return "", block, 0, false
	}
	n := len(fields)
	pos := strings.Join(fields[:n-2], " ") // ファイルの名前に空白があってもよい
	if _, err := strconv.Atoi(fields[n-2]); err != nil {
		return "", block, 0, false
	}
	count, err := strconv.Atoi(fields[n-1])
	if err != nil || count < 0 {
		return "", block, 0, false
	}
	colon := strings.LastIndex(pos, ":")
	if colon <= 0 {
		return "", block, 0, false
	}
	name = pos[:colon]
	start, end, found := strings.Cut(pos[colon+1:], ",")
	if !found {
		return "", block, 0, false
	}
	var okStart, okEnd bool
	block[0], okStart = coverPosLine(start)
	block[1], okEnd = coverPosLine(end)
	if !okStart || !okEnd || block[0] > block[1] {
		return "", block, 0, false
	}
	return name, block, count, true
}

// coverPosLine は `<行>.<列>` の行。
func coverPosLine(s string) (int, bool) {
	line, col, found := strings.Cut(s, ".")
	if !found {
		return 0, false
	}
	if _, err := strconv.Atoi(col); err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(line)
	return n, err == nil && n >= 1
}

// Lines は、名前 name（<import パス>/<ファイル名>）のファイルの、実行された所の行の範囲（始まりの行、終わりの行の順。
// 重複なし）。そのファイルの行が、カバレッジに一行もなければ measured が false。
func (p *CoverProfile) Lines(name string) (lines [][2]int, measured bool) {
	blocks, ok := p.files[name]
	if !ok {
		return nil, false
	}
	lines = [][2]int{}
	for b, hit := range blocks {
		if hit {
			lines = append(lines, b)
		}
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i][0] != lines[j][0] {
			return lines[i][0] < lines[j][0]
		}
		return lines[i][1] < lines[j][1]
	})
	return lines, true
}
