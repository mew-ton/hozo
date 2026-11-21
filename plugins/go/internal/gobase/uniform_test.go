package gobase

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、docs/plugin-go.md「op の形」が一通りであること: 基底が出す、仕様の宣言を写した関数（op のメソッド、状態の単位の
// メソッド、分類の述語と判定関数、継ぎ目のメソッドと body）は、どれも、最初の引数に ctx context.Context を取り、最後の戻り値に
// error を返す。純粋かどうか（Op.signature.pure）でも、throws の有無でも、形は変わらない。テストのパッケージ（同「単体テストの口」）の
// 関数は、仕様の宣言を写したものではなく、testing がその形を決めているので、確かめない。

// inTestPackage は、出力のファイル path（package 節の名前が pkg）が、テストのパッケージのものか: ディレクトリの名前が、パッケージ名と
// 同じで、test で終わる（同「単体テストの口」の「テストのパッケージ」）。入力の名前空間に、test で終わる名前のものはない。
func inTestPackage(path, pkg string) bool {
	return strings.HasSuffix(pkg, "test") && filepath.Base(filepath.Dir(path)) == pkg
}

// specFunc は、仕様の宣言を写した関数でないもの（同「op の形」: Go がその形を決めているもの）を除く: New、補助 ptr、union の
// マーカーメソッド（is<Union>）、例外の Error と HozoException。
func specFunc(name string) bool {
	switch {
	case name == "New", name == "ptr", name == "Error", name == "HozoException":
		return false
	case strings.HasPrefix(name, "is") && len(name) > 2 && name[2] >= 'A' && name[2] <= 'Z':
		return false
	}
	return true
}

// uniform は、関数の型 ft が「最初の引数が context.Context、最後の戻り値が error」か。
func uniform(ft *ast.FuncType) bool {
	if ft.Params == nil || len(ft.Params.List) == 0 || ft.Results == nil || len(ft.Results.List) == 0 {
		return false
	}
	first, ok := ft.Params.List[0].Type.(*ast.SelectorExpr)
	if !ok || first.Sel.Name != "Context" {
		return false
	}
	if pkg, ok := first.X.(*ast.Ident); !ok || pkg.Name != "context" {
		return false
	}
	last, ok := ft.Results.List[len(ft.Results.List)-1].Type.(*ast.Ident)
	return ok && last.Name == "error"
}

// TestEveryFunctionTakesCtxAndReturnsError は、すべての入力の出力を Go として読み、関数とメソッドの宣言、インターフェースの
// メソッド、引数の関数の型（継ぎ目の body）と関数リテラル（入れ子の継ぎ目）の、すべての形を確かめる。入力には、純粋な葉と
// 純粋な配線（statements の make、bothPure、endQuiet。matchswitch の pureLabel）、述語で決まる値の分類が入っている。
func TestEveryFunctionTakesCtxAndReturnsError(t *testing.T) {
	for _, fx := range goldenFixtures {
		_, files := emitFixture(t, fx.name)
		for path, content := range files {
			f, err := parser.ParseFile(token.NewFileSet(), path, content, 0)
			if err != nil {
				t.Fatalf("%s/%s: %v", fx.name, path, err)
			}
			if inTestPackage(path, f.Name.Name) {
				continue
			}
			checked := 0
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.FuncDecl:
					if specFunc(x.Name.Name) {
						checked++
						if !uniform(x.Type) {
							t.Errorf("%s/%s: func %s が、ctx を取り error を返す形でない", fx.name, path, x.Name.Name)
						}
					}
				case *ast.InterfaceType:
					for _, m := range x.Methods.List {
						ft, ok := m.Type.(*ast.FuncType)
						if !ok || len(m.Names) == 0 || !specFunc(m.Names[0].Name) {
							continue
						}
						checked++
						if !uniform(ft) {
							t.Errorf("%s/%s: インターフェースのメソッド %s が、ctx を取り error を返す形でない", fx.name, path, m.Names[0].Name)
						}
					}
				case *ast.FuncLit:
					checked++
					if !uniform(x.Type) {
						t.Errorf("%s/%s: 関数リテラルが、ctx を取り error を返す形でない", fx.name, path)
					}
				case *ast.Field:
					if ft, ok := x.Type.(*ast.FuncType); ok && len(x.Names) == 1 && strings.HasPrefix(x.Names[0].Name, "body") {
						checked++
						if !uniform(ft) {
							t.Errorf("%s/%s: 継ぎ目の %s が、ctx を取り error を返す形でない", fx.name, path, x.Names[0].Name)
						}
					}
				}
				return true
			})
			if filepath.Base(path) == "wiring.go" && checked == 0 {
				t.Errorf("%s/%s: 確かめる関数が一つもなかった", fx.name, path)
			}
		}
	}
}

// TestPureOpsHaveTheSameShape は、同じ入力の、すべての op を純粋にした View と、すべての op を純粋でなくした View から、
// 同じバイト列が出ること: 基底は Op.signature.pure を読まない（同「op の形」）。入力は、純粋な葉・純粋な配線・純粋でない
// 葉・純粋でない配線を持つ statements と、純粋な配線が述語で決まる分類を使う matchswitch。
func TestPureOpsHaveTheSameShape(t *testing.T) {
	for _, name := range []string{"statements", "matchswitch"} {
		emitWith := func(pure bool) map[string]string {
			views := loadViews(t, name)
			ops := 0
			for _, v := range views {
				for i := range v.Program.Namespaces {
					for _, d := range v.Program.Namespaces[i].Decls {
						if o, ok := d.(*ir.Op); ok {
							o.Signature.Pure = pure
							ops++
						}
					}
				}
				for fqn, sig := range v.Ops {
					sig.Pure = pure
					v.Ops[fqn] = sig
				}
			}
			if ops == 0 {
				t.Fatalf("%s: op が一つもない", name)
			}
			files, diags := emitViews(t, newRoot(t), views)
			if len(diags) > 0 {
				t.Fatalf("%s: 診断 %v", name, diags)
			}
			return files
		}
		pure, impure := emitWith(true), emitWith(false)
		if len(pure) != len(impure) {
			t.Fatalf("%s: ファイルの数が違う: %d と %d", name, len(pure), len(impure))
		}
		for p, content := range pure {
			if impure[p] != content {
				t.Errorf("%s: %s が、純粋さで変わった:\n%s", name, p, firstDiff(content, impure[p]))
			}
		}
		compareTree(t, name, pure) // 純粋さを書き換えても、ゴールデンのまま
	}
}

// TestNoGeneratedPanic は、基底が panic を呼ぶコードを出さないこと（同「分類」: 判定の誤りは、配線が error として返す）。
func TestNoGeneratedPanic(t *testing.T) {
	for _, fx := range goldenFixtures {
		_, files := emitFixture(t, fx.name)
		for path, content := range files {
			f, err := parser.ParseFile(token.NewFileSet(), path, content, 0)
			if err != nil {
				t.Fatalf("%s/%s: %v", fx.name, path, err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "panic" {
						t.Errorf("%s/%s: 生成したコードが panic を呼ぶ", fx.name, path)
					}
				}
				return true
			})
		}
	}
}
