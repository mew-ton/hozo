package tests

import (
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

// modulePath はこのリポジトリの Go モジュール（go.mod）。
const modulePath = "github.com/mew-ton/hozo"

// imports はリポジトリの Go のパッケージ（ルートからの相対パス）→ このモジュールの中で import しているパッケージ。
// tests が true なら _test.go の import、false ならそれ以外のファイルの import。Go が見ないディレクトリ（testdata、
// 隠しディレクトリ、node_modules）と、自分の go.mod を持つディレクトリ（別のモジュール。tests/samples/ のサンプル）は読まない。
func moduleImports(t *testing.T, tests bool) map[string][]string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	sets := map[string]map[string]bool{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			if p != root {
				if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
					return filepath.SkipDir // 別のモジュール。このモジュールのパッケージではない
				}
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(p))
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(rel)
		if sets[pkg] == nil {
			sets[pkg] = map[string]bool{}
		}
		if strings.HasSuffix(name, "_test.go") != tests {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if dep, ok := strings.CutPrefix(path, modulePath+"/"); ok && dep != pkg {
				sets[pkg][dep] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for pkg, set := range sets {
		deps := make([]string, 0, len(set))
		for d := range set {
			deps = append(deps, d)
		}
		sort.Strings(deps)
		out[pkg] = deps
	}
	return out
}

var codeSpanRE = regexp.MustCompile("`([^`]+)`")

// documentedImports は docs/packages.md「依存の向き」の表（パッケージ → import するパッケージ）を読む。
func documentedImports(t *testing.T) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "docs", "packages.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimPrefix(line, "## ")
			continue
		}
		if section != "依存の向き" || !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 2 {
			t.Fatalf("表の行を読めない: %s", line)
		}
		pkg := codeSpanRE.FindStringSubmatch(cells[0])
		if pkg == nil {
			t.Fatalf("表の行を読めない: %s", line)
		}
		deps := []string{}
		for _, m := range codeSpanRE.FindAllStringSubmatch(cells[1], -1) {
			deps = append(deps, m[1])
		}
		sort.Strings(deps)
		if _, dup := out[pkg[1]]; dup {
			t.Errorf("docs/packages.md「依存の向き」に %s が二度ある", pkg[1])
		}
		out[pkg[1]] = deps
	}
	if len(out) == 0 {
		t.Fatal("docs/packages.md「依存の向き」の表を読めなかった")
	}
	return out
}

// TestDocumentedDependencies は docs/packages.md「依存の向き」の表が、Go のパッケージの実際の import（テストでないファイル）と
// 同じであることを確かめる: あるパッケージはすべて表にあり、表のパッケージはすべてあり、import するパッケージの集合が等しい。
// 表が持ち主なので、import を足したら表にも足す（足してよい向きかを、そのときに考える）。
func TestDocumentedDependencies(t *testing.T) {
	want := documentedImports(t)
	got := moduleImports(t, false)
	var pkgs []string
	for p := range want {
		pkgs = append(pkgs, p)
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			pkgs = append(pkgs, p)
		}
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		w, documented := want[p]
		g, exists := got[p]
		switch {
		case !documented:
			t.Errorf("パッケージ %s が docs/packages.md「依存の向き」の表にない（import: %s）", p, strings.Join(g, "、"))
		case !exists:
			t.Errorf("docs/packages.md「依存の向き」の %s というパッケージはない", p)
		case strings.Join(w, " ") != strings.Join(g, " "):
			t.Errorf("%s の import が docs/packages.md「依存の向き」と違う:\n  文書: %s\n  実際: %s", p, strings.Join(w, "、"), strings.Join(g, "、"))
		}
	}
}

// TestDependencyRules は docs/packages.md「依存の向き」の箇条書きの規則を、テストのファイルの import も含めて確かめる。
func TestDependencyRules(t *testing.T) {
	isPlugin := func(pkg string) bool {
		return strings.HasPrefix(pkg, "plugins/") || strings.HasPrefix(pkg, "tests/plugins/") || strings.Contains(pkg, "/plugins/")
	}
	// プラグインが使えるパッケージ（と、自分のディレクトリの下のパッケージ）
	forPlugins := map[string]bool{"ir": true, "plugin": true, "plugin/golang": true, "plugin/govalid": true, "plugin/httpvocab": true, "runtime": true}
	for _, tests := range []bool{false, true} {
		for pkg, deps := range moduleImports(t, tests) {
			for _, dep := range deps {
				// internal/ は cmd/hozoc と internal/ の中、それにリポジトリをまたぐテストのパッケージ tests からだけ使う
				if strings.HasPrefix(dep, "internal/") && !strings.HasPrefix(pkg, "internal/") && pkg != "cmd/hozoc" && pkg != "tests" {
					t.Errorf("%s が %s を import している（internal/ は cmd/hozoc、internal/、tests からだけ）", pkg, dep)
				}
				// プラグインが使えるのは ir、plugin、plugin/golang、plugin/govalid、plugin/httpvocab、runtime
				if isPlugin(pkg) {
					own := strings.SplitN(strings.TrimPrefix(pkg, "plugins/"), "/", 2)[0]
					if !forPlugins[dep] && !(strings.HasPrefix(pkg, "plugins/") && strings.HasPrefix(dep, "plugins/"+own+"/")) {
						t.Errorf("プラグイン %s が %s を import している（使えるのは ir、plugin、plugin/golang、plugin/govalid、plugin/httpvocab、runtime）", pkg, dep)
					}
				}
				// 公開のパッケージは internal/ にも、hozoc の側のパッケージにも依存しない
				if public := map[string]bool{"ir": true, "plugin": true, "plugin/golang": true, "plugin/govalid": true, "plugin/httpvocab": true, "runtime": true, "vocab": true}; public[pkg] && !public[dep] {
					t.Errorf("公開のパッケージ %s が %s を import している", pkg, dep)
				}
			}
		}
	}
}
