package golang

import (
	"path"
	"path/filepath"

	"github.com/mew-ton/hozo/ir"
)

// innerDir は内側のフォルダの名前で、内側のパッケージの名前でもある（plugins/go/spec/go.hozo の Requirement.Layout の「内側のフォルダと入口」）。
// 相乗りは、このディレクトリ名を使わない（同「相乗りのファイル」）。
const innerDir = "inner"

// typesDir は、型のパッケージのフォルダの名前で、そのパッケージの名前でもある（plugins/go/spec/go.hozo の Requirement.Layout の「型のパッケージ」）。
const typesDir = "types"

// testSuffix は、テストのパッケージの名前の後ろに付ける語（plugins/go/spec/go.hozo の Requirement.UnitTestPort の「テストのパッケージ」）。
const testSuffix = "test"

// Layout は一つの出力 (U, N) の置き場（plugins/go/spec/go.hozo の Requirement.Layout）。
//
// 入口のパッケージは `<Dir(U, N)>/` で、人が書くコード、別の名前空間の出力、相乗りのファイルが使う。定義のパッケージは、
// 型と口と配線を定義するパッケージ: 分けた出力（HasInner）では内側 `<Dir(U, N)>/inner/`、分けない出力では入口と同じ。
type Layout struct {
	// Namespace は出力する名前空間 N の FQN。
	Namespace string
	// Dir は Dir(U, N)（ルートからの相対パス、`/` 区切り）。入口のパッケージのディレクトリ。
	Dir string
	// EntryPath と EntryName は、入口のパッケージの import パスと名前。
	EntryPath, EntryName string
	// HasInner は内側のフォルダを持つか（N の op に内部の op が一つでもある）。
	HasInner bool
	// DefDir、DefPath、DefName は、定義のパッケージのディレクトリ、import パス、名前。HasInner なら `<Dir>/inner`、
	// `<EntryPath>/inner`、"inner"。そうでなければ入口と同じ。
	DefDir, DefPath, DefName string
	// HasTypes は、型のパッケージを、分けて持つか（plugins/go/spec/go.hozo の Requirement.Layout の「型のパッケージ」: N が、op、read、効果、状態の宣言を、
	// 一つでも持つ。単位の in は、見ない）。
	HasTypes bool
	// TypesDir、TypesPath、TypesName は、型のパッケージのディレクトリ、import パス、名前。HasTypes なら `<Dir>/types`、
	// `<EntryPath>/types`、"types"。そうでなければ入口と同じ。別の名前空間の出力と、相乗りのファイルは、ここを import する。
	TypesDir, TypesPath, TypesName string
	// TestDir、TestPath、TestName は、テストのパッケージ（plugins/go/spec/go.hozo の Requirement.UnitTestPort の「テストのパッケージ」）の
	// ディレクトリ、import パス、名前: `<Dir>/<EntryName>test`、`<EntryPath>/<EntryName>test`、`<EntryName>test`。
	// 口が一つもない出力では、基底は、このパッケージを作らない。
	TestDir, TestPath, TestName string
}

// LayoutOf は、この View の出力 (U, N) の置き場（plugins/go/spec/go.hozo の Requirement.Layout の「パッケージと import パス」、Requirement.Layout の「内側のフォルダと入口」）。
// import パスは、設定のオプション module から作る（ModuleOf）。module がなければ ErrNoModule を包んだ誤り（GO-8）。
func LayoutOf(view *ir.View) (*Layout, error) {
	hasInner := false
	for _, o := range ShapeOf(view).Ops {
		hasInner = hasInner || !o.Exported
	}
	return layout(view.Viewer, view.Config, hasInner, HasFuncs(view.Program, view.Viewer))
}

// HasFuncs は、名前空間 namespace が、関数を持つか（plugins/go/spec/go.hozo の Requirement.Layout の「型のパッケージ」）: op、read、効果、状態の宣言を、一つでも
// 持つ。単位の in は、見ない（名前空間の宣言だけから決まる）。
func HasFuncs(p *ir.Program, namespace string) bool {
	if p == nil {
		return false
	}
	ns := p.Namespace(namespace)
	if ns == nil {
		return false
	}
	for _, d := range ns.Decls {
		switch d.(type) {
		case *ir.Op, *ir.Read, *ir.Effect, *ir.State:
			return true
		}
	}
	return false
}

// HomeOf は、view の単位から見た、名前空間 namespace の型の置き場（docs/config.md「型の置き場」）: view の単位自身が
// その名前空間を出力するならその出力、そうでなければ、基底が同じで、その名前空間を出力する単位のうち、設定の順で最初のものの出力
// （docs/ir.md「その名前空間から見た IR」: View.namespaces の要素のうち index が config.index と同じもの、なければ base が
// config.base と同じ最初のもの）。なければ false。namespace が view.Viewer なら LayoutOf と同じ。
//
// 使う側が import するのは、置き場の入口のパッケージ（EntryPath、EntryName）。別の単位の置き場では、その単位の in が View に
// ないので、HasInner と Def* は、in を省いた単位として求めた値（入口の import には効かない）。
func HomeOf(view *ir.View, namespace string) (*Layout, bool, error) {
	if namespace == view.Viewer {
		l, err := LayoutOf(view)
		return l, err == nil, err
	}
	var home *ir.NamespaceConfig
	list := view.Namespaces[namespace]
	for i := range list {
		if list[i].Index == view.Config.Index {
			home = &list[i]
			break
		}
	}
	for i := range list {
		if home == nil && list[i].Base == view.Config.Base {
			home = &list[i]
		}
	}
	if home == nil {
		return nil, false, nil
	}
	inUnit := func(*ir.Op) bool { return true }
	if home.Index == view.Config.Index {
		inUnit = fileSet(view.Files)
	}
	hasInner := false
	for _, o := range shapeOf(view.Program, namespace, inUnit, home.Seams).Ops {
		hasInner = hasInner || !o.Exported
	}
	l, err := layout(namespace, *home, hasInner, HasFuncs(view.Program, namespace))
	return l, err == nil, err
}

// layout は、名前空間 namespace を、設定 cfg の出力（cfg.Dir が Dir(U, N)）に出力するときの置き場。
func layout(namespace string, cfg ir.NamespaceConfig, hasInner, hasTypes bool) (*Layout, error) {
	dir := path.Clean(filepath.ToSlash(cfg.Dir))
	module, err := ModuleOf(cfg)
	if err != nil {
		return nil, err
	}
	entry := ImportPath(module, dir)
	l := &Layout{Namespace: namespace, Dir: dir, EntryPath: entry, EntryName: PackageName(namespace), HasInner: hasInner}
	l.DefDir, l.DefPath, l.DefName = l.Dir, l.EntryPath, l.EntryName
	l.TestName = l.EntryName + testSuffix
	l.TestDir, l.TestPath = path.Join(dir, l.TestName), entry+"/"+l.TestName
	if hasInner {
		l.DefDir, l.DefPath, l.DefName = path.Join(dir, innerDir), entry+"/"+innerDir, innerDir
	}
	l.TypesDir, l.TypesPath, l.TypesName = l.Dir, l.EntryPath, l.EntryName
	if hasTypes {
		l.HasTypes = true
		l.TypesDir, l.TypesPath, l.TypesName = path.Join(dir, typesDir), entry+"/"+typesDir, typesDir
	}
	return l, nil
}

// fileSet は、op が files のファイルに書かれているか（単位の対象か。docs/config.md「in」）を答える関数。
func fileSet(files []string) func(*ir.Op) bool {
	set := make(map[string]bool, len(files))
	for _, f := range files {
		set[f] = true
	}
	return func(o *ir.Op) bool { return set[o.Span.File] }
}
