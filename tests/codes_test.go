package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// このファイルは、Go のコードが出しうる診断のコードと、設計文書の「診断」の表を突き合わせる。
//
// コードの側は、テストでない Go のソース（internal、plugin、plugins、cmd。生成物と go generate の道具は除く）を構文木で読んで集める:
//
//   - 診断の構造体リテラルの Code（`diag.Diagnostic{Severity: diag.Error, Code: "MAN-3", ...}`。定数も辿る）。
//     Severity も書いてあれば、そのコードの重さとして覚える。
//   - 引数 code を持つ補助関数（errorf、warnf、errorAt、warnAt、report、codedError、その場の errorf := func(code, ...)）の
//     呼び出しの、その位置の引数。重さは、引数 sev があればその値、なければ関数の名前（error / warn）から。
//   - それ以外の場所に書かれた、規則の ID の形（`CFG-16`）の文字列リテラル（比較、代入など）。重さは分からない。
//   - 文字列の頭の `PLG-1: ` の形。診断ではなく例外のメッセージに書く ID（docs/plugin-protocol.md「診断」）。
//
// 文書の側は、docs/*.md の、先頭の列が ID の表（各文書の「診断」の表）の行。重さの列（error / warning / —）を持つ。

// codeUse はコードの側で見つけた一か所。
type codeUse struct {
	code string
	sev  string // "error" / "warning" / ""（分からない） / "message"（例外のメッセージの中の ID）
	pos  string
}

var (
	ruleCodeRE    = regexp.MustCompile(`^[A-Z]+-[0-9]+$`)
	messageCodeRE = regexp.MustCompile(`^([A-Z]+-[0-9]+): `)
	// anyCodeRE は診断のコードの形（大文字と数字と -）。引数 code を持つ関数には、診断でない code を取るもの
	// （プロトコルの error の code "protocol" など）もあるので、形で見分ける。
	anyCodeRE = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:-[A-Z0-9]+)*$`)
)

// scanDirs は診断を出しうる Go のソースの置き場（リポジトリのルートから）。
var scanDirs = []string{"internal", "plugin", "plugins", "cmd"}

// skipDirs は読まないディレクトリ: 生成物、go generate / make generate の道具、テストの入力。
var skipDirs = map[string]bool{"testdata": true, "node_modules": true}
var skipPaths = map[string]bool{"internal/gen": true, "internal/tools": true}

// collectCodeUses はリポジトリの Go のソースから、診断のコードの使われ方を集める。
func collectCodeUses(t *testing.T) []codeUse {
	t.Helper()
	fset := token.NewFileSet()
	type pkgFile struct {
		dir  string
		file *ast.File
	}
	var files []pkgFile
	for _, top := range scanDirs {
		root := filepath.Join("..", top)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(p, ".."+string(filepath.Separator)))
			if d.IsDir() {
				if skipDirs[d.Name()] || skipPaths[rel] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			files = append(files, pkgFile{dir: filepath.Dir(p), file: f})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// 文字列の定数（名前 → 値）。パッケージをまたいで名前で引く（golang.CodeImportCycle など）。
	consts := map[string]string{}
	for _, pf := range files {
		ast.Inspect(pf.file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, name := range spec.Names {
				if i < len(spec.Values) {
					if s, ok := stringLit(spec.Values[i]); ok {
						consts[name.Name] = s
					}
				}
			}
			return true
		})
	}
	value := func(e ast.Expr) (string, bool) {
		if s, ok := stringLit(e); ok {
			return s, true
		}
		switch x := e.(type) {
		case *ast.Ident:
			s, ok := consts[x.Name]
			return s, ok
		case *ast.SelectorExpr:
			s, ok := consts[x.Sel.Name]
			return s, ok
		}
		return "", false
	}

	// 引数 code を持つ補助関数（ディレクトリと名前で引く）。
	type helper struct {
		codeIndex, sevIndex int
	}
	helpers := map[string]helper{}
	addHelper := func(dir, name string, ft *ast.FuncType) {
		h := helper{codeIndex: -1, sevIndex: -1}
		i := 0
		for _, field := range ft.Params.List {
			for _, n := range field.Names {
				switch n.Name {
				case "code":
					h.codeIndex = i
				case "sev":
					h.sevIndex = i
				}
				i++
			}
		}
		if h.codeIndex >= 0 {
			helpers[dir+"\x00"+name] = h
		}
	}
	for _, pf := range files {
		ast.Inspect(pf.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				addHelper(pf.dir, x.Name.Name, x.Type)
			case *ast.AssignStmt:
				for i, rhs := range x.Rhs {
					if lit, ok := rhs.(*ast.FuncLit); ok && i < len(x.Lhs) {
						if id, ok := x.Lhs[i].(*ast.Ident); ok {
							addHelper(pf.dir, id.Name, lit.Type)
						}
					}
				}
			}
			return true
		})
	}

	var uses []codeUse
	for _, pf := range files {
		claimed := map[ast.Node]bool{} // 構造体リテラルか補助関数の呼び出しとして数えた文字列リテラル
		pos := func(n ast.Node) string {
			p := fset.Position(n.Pos())
			return filepath.ToSlash(strings.TrimPrefix(p.Filename, ".."+string(filepath.Separator))) + ":" + strconv.Itoa(p.Line)
		}
		ast.Inspect(pf.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CompositeLit:
				var codeExpr, sevExpr ast.Expr
				for _, el := range x.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					switch key, _ := kv.Key.(*ast.Ident); {
					case key != nil && key.Name == "Code":
						codeExpr = kv.Value
					case key != nil && key.Name == "Severity":
						sevExpr = kv.Value
					}
				}
				if codeExpr == nil {
					return true
				}
				if code, ok := value(codeExpr); ok && anyCodeRE.MatchString(code) {
					claimed[codeExpr] = true
					uses = append(uses, codeUse{code: code, sev: severityOf(sevExpr), pos: pos(x)})
				}
			case *ast.CallExpr:
				name := ""
				switch fn := x.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				h, ok := helpers[pf.dir+"\x00"+name]
				if !ok || h.codeIndex >= len(x.Args) {
					return true
				}
				code, ok := value(x.Args[h.codeIndex])
				if !ok || !anyCodeRE.MatchString(code) {
					return true // 引数をそのまま渡しているだけか、診断のコードでない code
				}
				claimed[x.Args[h.codeIndex]] = true
				sev := ""
				switch lower := strings.ToLower(name); {
				case h.sevIndex >= 0 && h.sevIndex < len(x.Args):
					sev = severityOf(x.Args[h.sevIndex])
				case strings.Contains(lower, "warn"):
					sev = "warning"
				case strings.Contains(lower, "error"):
					sev = "error"
				}
				uses = append(uses, codeUse{code: code, sev: sev, pos: pos(x)})
			}
			return true
		})
		ast.Inspect(pf.file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || claimed[n] {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			switch {
			case ruleCodeRE.MatchString(s):
				uses = append(uses, codeUse{code: s, pos: pos(lit)})
			case messageCodeRE.MatchString(s):
				uses = append(uses, codeUse{code: messageCodeRE.FindStringSubmatch(s)[1], sev: "message", pos: pos(lit)})
			}
			return true
		})
	}
	return uses
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// severityOf は Severity の式（diag.Error、protocol.SeverityWarning など）を "error" / "warning" にする。分からなければ ""。
func severityOf(e ast.Expr) string {
	name := ""
	switch x := e.(type) {
	case *ast.Ident:
		name = x.Name
	case *ast.SelectorExpr:
		name = x.Sel.Name
	}
	switch {
	case strings.HasSuffix(name, "Error"):
		return "error"
	case strings.HasSuffix(name, "Warning"):
		return "warning"
	}
	return ""
}

// docCode は文書の表の一行: ID、重さ（"error" / "warning" / "—"）、どの文書か。
type docCode struct {
	sev string
	doc string
}

var docCodeRowRE = regexp.MustCompile("^\\|\\s*`?([A-Z]+(?:-[A-Z0-9]+)*)`?\\s*\\|")

// documentedCodes は docs/*.md の、先頭の列が ID で「重さ」の列を持つ表（各文書の「診断」の表）の行を集める。
// 先頭の列が ID で「意味」か「手続き」の列を持つのに「重さ」の列を持たない表は noSeverity に入れる
// （docs/requirements.md の索引は、規則の意味の持ち主ではないので数えない）。
func documentedCodes(t *testing.T) (codes map[string]docCode, noSeverity []string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "docs", "*.md"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("docs を読めない: %v", err)
	}
	codes = map[string]docCode{}
	for _, p := range paths {
		name := filepath.Base(p)
		if name == "requirements.md" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sevCol := -1
		inTable := false
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "|") {
				inTable, sevCol = false, -1
				continue
			}
			cells := strings.Split(strings.Trim(line, "|"), "|")
			for j := range cells {
				cells[j] = strings.TrimSpace(cells[j])
			}
			if !inTable {
				inTable = true
				if cells[0] != "ID" {
					sevCol = -2 // ID の表ではない
					continue
				}
				meaning := false
				for j, c := range cells {
					switch c {
					case "重さ":
						sevCol = j
					case "意味", "手続き":
						meaning = true
					}
				}
				if sevCol < 0 {
					if meaning { // 診断の表（意味か手続きの列を持つ）なのに重さがない。ほかの ID の表（Data の表など）は数えない
						noSeverity = append(noSeverity, name+":"+strconv.Itoa(i+1))
					}
					sevCol = -2
				}
				continue
			}
			m := docCodeRowRE.FindStringSubmatch(line)
			if sevCol < 0 || m == nil || sevCol >= len(cells) {
				continue
			}
			if prev, dup := codes[m[1]]; dup {
				t.Errorf("%s が二つの表にある（%s と %s）。持ち主は一つ", m[1], prev.doc, name)
				continue
			}
			codes[m[1]] = docCode{sev: cells[sevCol], doc: name}
		}
	}
	return codes, noSeverity
}

// TestDiagnosticCodesAreDocumented は、Go のコードが出しうる診断のコードがどれも設計文書の「診断」の表にあり、
// 表の重さがコードの重さと合っていることを確かめる。規則の ID でないコード（SYNTAX、IO、PLUGIN、GO-INTERNAL）も同じ。
// 例外のメッセージにだけ書く ID（PLG-*）は、表の重さが「—」（診断にならない）であること。
func TestDiagnosticCodesAreDocumented(t *testing.T) {
	uses := collectCodeUses(t)
	docs, noSeverity := documentedCodes(t)
	for _, table := range noSeverity {
		t.Errorf("docs/%s の ID の表に「重さ」の列がない（error / warning / — を書く）", table)
	}
	seen := map[string]bool{}
	for _, u := range uses {
		seen[u.code] = true
	}
	// 拾えていることの確かめ: 作り方の違う代表が入っている
	for _, want := range []string{"SYNTAX", "IO", "PLUGIN", "GO-INTERNAL", "LSP-1", "CFG-7", "CFG-5", "MAN-5", "PLG-1", "PLG-2", "CFG-19", "CFG-22", "THROWS-1", "RES-1", "WIRE-8", "GO-3", "GO-6", "CLI-1"} {
		if !seen[want] {
			t.Errorf("コードから %s を拾えていない（集め方が合っていない）", want)
		}
	}
	type problem struct{ code, msg string }
	var problems []problem
	reported := map[string]bool{}
	sort.Slice(uses, func(i, j int) bool { return uses[i].pos < uses[j].pos })
	for _, u := range uses {
		d, ok := docs[u.code]
		var msg string
		switch {
		case !ok:
			msg = "は、どの設計文書の「診断」の表にもない"
		case u.sev == "message" && d.sev != "—":
			msg = "は例外のメッセージに書く ID で診断にならないのに、docs/" + d.doc + " の重さが " + d.sev
		case (u.sev == "error" || u.sev == "warning") && d.sev != u.sev:
			msg = "を " + u.sev + " で出しているのに、docs/" + d.doc + " の重さは " + d.sev
		default:
			continue
		}
		if key := u.code + msg; !reported[key] {
			reported[key] = true
			problems = append(problems, problem{u.code, u.code + " " + msg + "（" + u.pos + "）"})
		}
	}
	sort.Slice(problems, func(i, j int) bool { return problems[i].msg < problems[j].msg })
	for _, p := range problems {
		t.Error(p.msg)
	}
	// 逆向き: 表が error か warning と言うコードは、コードのどこかで出している。出さないものは、理由を持つ表にあること
	// （文法で閉じる規則、別の ID で出る規則、索引として残す ID。tests/rules_test.go の grammarClosed / reportedAs / notTestable）。
	for code, d := range docs {
		if d.sev != "error" && d.sev != "warning" {
			continue
		}
		_, indexOnly := notTestable[code]
		if seen[code] || grammarClosed[code] || reportedAs[code] != "" || indexOnly {
			continue
		}
		t.Errorf("docs/%s の %s（%s）を、コードのどこも出していない", d.doc, code, d.sev)
	}
}

// TestMessagesDoNotCiteDraft は、Go のコードの文字列リテラル（診断のメッセージを含む、利用者に見える文言）が「設計書」を
// 引いていないことを確かめる。「設計書」は、出発点のアイデア集の呼び名で、仕様の持ち主ではない（docs/README.md
// 「設計書という呼び名」）ので、利用者に見せる根拠にならない。コメントは、考えの出どころを指すために「§n」を書いてよい
// （コメントは構文木の文字列リテラルではないので、ここでは見ない）。読むソースは collectCodeUses と同じ。
func TestMessagesDoNotCiteDraft(t *testing.T) {
	fset := token.NewFileSet()
	for _, top := range scanDirs {
		root := filepath.Join("..", top)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(p, ".."+string(filepath.Separator)))
			if d.IsDir() {
				if skipDirs[d.Name()] || skipPaths[rel] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				if s, err := strconv.Unquote(lit.Value); err == nil && strings.Contains(s, "設計書") {
					t.Errorf("%s: 文字列 %q が「設計書」を引いている（docs/README.md「設計書という呼び名」）", fset.Position(lit.Pos()), s)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
