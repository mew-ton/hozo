package golang

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// Imports は、生成する一つのファイルの import の集まり（plugins/go/spec/go.hozo の Requirement.Layout の「パッケージと import パス」の「import の名前」）。
// GoType / Zero / Signature が、必要な import を足す。Render が import のブロックを出す。
//
// 名前の決め方: パッケージは、その `package` 節の名前で import する。名前が import パスの最後の要素と違えば、名前を書く
// （`pkgtype "example.com/shop-server/gen/type"`）。一つのファイルの中で名前が当たれば、後から足した方に 2、3、… を付ける。
// 文書の決まり（標準ライブラリ、runtime、内側のパッケージ inner の名前は変えず、名前空間のパッケージは、名前空間の FQN の
// 辞書順に名前を決める）のとおりにするには、足す順をそろえる: Canonical が、同じ import の集まりを、その順で足し直したものを返す。
// 基底は、ファイルを一度作って import の集まりを知り、Canonical の Imports でもう一度作る。
type Imports struct {
	view   *ir.View
	layout *Layout
	inDef  bool
	// inTypes は、書くファイルが、型のパッケージ（plugins/go/spec/go.hozo の Requirement.Layout の「型のパッケージ」）のものか。
	inTypes bool
	// outside は、書くファイルが、出力の外のパッケージ（テストのパッケージ）のものか。そのとき、N の宣言も、入口のパッケージで修飾する。
	outside bool

	paths    map[string]string // import パス → 使う名前
	names    map[string]string // 使う名前 → import パス（import に使わせない名前は ""）
	reserved []string          // Reserve で足した、import に使わせない名前
	pkgNames map[string]string // import パス → パッケージの名前（Add に渡されたもの）
	nsOf     map[string]string // 名前空間のパッケージの import パス → 名前空間の FQN
	named    map[string]bool   // いつも名前を書いて import するパス（テストのパッケージから見た、N の入口のパッケージ）
	declNS   map[string]string // 宣言の FQN → 宣言した名前空間（view.Program から。最初に引いたときに作る）
	homes    map[string]*Layout
	err      error
}

// basePrivateNames は、基底が定義のパッケージに宣言する非公開の名前のうち、パッケージ名になりうるもの（小文字と数字だけの名前）:
// New が返す構造体 ops と、値をポインタにする補助 ptr（plugins/go/spec/go.hozo の Requirement.Ports の「New」、Requirement.Wiring の「値」）。Go では、パッケージの宣言と、その
// パッケージのどのファイルの import の名前も、同じ名前ならコンパイルできないので、どのファイルの import も、この名前を使わない
// （同「パッケージと import パス」の「import の名前」）。
var basePrivateNames = []string{"ops", "ptr"}

// NewImports は、view の出力の一つのファイルの import を作る。layout は、その出力の置き場（LayoutOf(view)）、
// inDef は、書くファイルが定義のパッケージのものか。入口のパッケージのファイル（分けた出力の
// 入口の再公開と、相乗りのファイル）なら false で、そのとき、内側のパッケージ `inner` の名前は、ほかの import に使わせない。
// どちらでも、N の型は修飾なしの名前になる（入口は、定義のすべての型の別名を持つ）。
func NewImports(view *ir.View, layout *Layout, inDef bool) *Imports {
	return &Imports{view: view, layout: layout, inDef: inDef}
}

// NewTypesImports は、view の出力の、型のパッケージ（plugins/go/spec/go.hozo の Requirement.Layout の「型のパッケージ」）の、一つのファイルの import を作る。
// N の型は、修飾なしの名前になる。型のパッケージを分けない出力（Layout.HasTypes が偽）では、NewImports(…, true) と同じ。
func NewTypesImports(view *ir.View, layout *Layout) *Imports {
	return &Imports{view: view, layout: layout, inDef: true, inTypes: true}
}

// NewTestImports は、view の出力の、テストのパッケージ（plugins/go/spec/go.hozo の Requirement.UnitTestPort の「テストのパッケージ」）の、一つの
// ファイルの import を作る。テストのパッケージは、N の入口のパッケージを import するので、N の型も、入口のパッケージの名前で
// 修飾する（Qualifier(N) が、入口を import して `<名前>.` を返す）。その import は、名前がパスの最後の要素と同じでも、名前を書く
// （出力先のディレクトリの名前で、ファイルの中身が変わらないように: 同「決定性」）。
func NewTestImports(view *ir.View, layout *Layout) *Imports {
	return &Imports{view: view, layout: layout, outside: true}
}

func (im *Imports) init() {
	if im.paths != nil {
		return
	}
	im.paths, im.names, im.pkgNames, im.nsOf = map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	im.named = map[string]bool{}
	// 基底が定義のパッケージに宣言する非公開の名前は、import の名前に使わせない（当たれば、名前空間のパッケージに 2、3、… が付く）
	for _, name := range basePrivateNames {
		im.names[name] = ""
	}
	for _, name := range im.reserved {
		im.names[name] = ""
	}
	if im.layout != nil && im.layout.HasInner && !im.inDef {
		// 入口のファイルでは、inner は内側のパッケージの名前（「標準ライブラリ、runtime、inner の名前は変えない」）
		im.names[innerDir] = im.layout.DefPath
	}
	if im.layout != nil && im.layout.HasTypes && !im.inTypes && !im.outside {
		// 定義と入口のファイルでは、types は、型のパッケージの名前（inner と同じに、変えない）
		im.names[typesDir] = im.layout.TypesPath
	}
}

// Reserve は、names を、このファイルの import の名前に使わせない名前に足す: パッケージ名がそれと同じ名前空間は、2、3、… を付けて
// import される（plugins/go/spec/go.hozo の Requirement.Layout の「パッケージと import パス」の「import の名前」）。ファイルを書くトランスパイラが、関数の本体の
// 中で決まった名前で使う識別子（基底の、メソッドのレシーバ o、ctx、err など）を渡す。その名前の局所の変数が、同じ名前のパッケージを
// 隠して、本体がそのパッケージの型を書けなくなるのを防ぐ。import を一つも足していないうちに呼ぶ。Canonical にも引き継ぐ。
func (im *Imports) Reserve(names ...string) {
	im.reserved = append(im.reserved, names...)
	if im.paths != nil {
		for _, name := range names {
			if _, taken := im.names[name]; !taken {
				im.names[name] = ""
			}
		}
	}
}

// Add は pkgPath を import に足し、そのパッケージを修飾するのに使う名前を返す。pkgName はそのパッケージの `package` 節の名前。
// すでに別のパスが同じ名前を使っていれば、2、3、… を付ける（`store2`）。同じパスを二度足しても同じ名前を返す。
func (im *Imports) Add(pkgPath, pkgName string) string {
	im.init()
	if name, ok := im.paths[pkgPath]; ok {
		return name
	}
	name := pkgName
	for i := 2; ; i++ {
		if p, taken := im.names[name]; !taken || p == pkgPath {
			break
		}
		name = pkgName + strconv.Itoa(i)
	}
	im.paths[pkgPath] = name
	im.names[name] = pkgPath
	im.pkgNames[pkgPath] = pkgName
	return name
}

// Name は pkgPath を修飾するのに使う名前（Add の戻り値）。足していなければ ""。
func (im *Imports) Name(pkgPath string) string {
	if im == nil {
		return ""
	}
	return im.paths[pkgPath]
}

// Paths は import パスを辞書順で返す。
func (im *Imports) Paths() []string {
	if im == nil {
		return nil
	}
	ps := make([]string, 0, len(im.paths))
	for p := range im.paths {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

// Names は、このファイルが import するパッケージを修飾するのに使う名前（辞書順）。局所の名前が避ける名前
// （plugins/go/spec/go.hozo の Requirement.Names の「局所の名前」）を作るのに使う。
func (im *Imports) Names() []string {
	if im == nil {
		return nil
	}
	ns := make([]string, 0, len(im.paths))
	for _, n := range im.paths {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}

// Render は import のブロックを返す: 標準ライブラリ（パスの先頭の要素に `.` がない）、空行、それ以外の順で、それぞれパスの
// 辞書順。名前がパスの最後の要素と違えば、名前を書く（テストのパッケージから見た、N の入口のパッケージは、いつも書く）。一つなら `import "time"` の一行、なければ ""。末尾に改行を一つ付ける。
func (im *Imports) Render() string {
	ps := im.Paths()
	if len(ps) == 0 {
		return ""
	}
	line := func(p string) string {
		if name := im.paths[p]; name != path.Base(p) || im.named[p] {
			return name + " " + strconv.Quote(p)
		}
		return strconv.Quote(p)
	}
	if len(ps) == 1 {
		return "import " + line(ps[0]) + "\n"
	}
	var std, other []string
	for _, p := range ps {
		if isStdlib(p) {
			std = append(std, p)
		} else {
			other = append(other, p)
		}
	}
	var b strings.Builder
	b.WriteString("import (\n")
	for _, p := range std {
		b.WriteString("\t" + line(p) + "\n")
	}
	if len(std) > 0 && len(other) > 0 {
		b.WriteString("\n")
	}
	for _, p := range other {
		b.WriteString("\t" + line(p) + "\n")
	}
	b.WriteString(")\n")
	return b.String()
}

// Canonical は、同じ import の集まりを、文書の決まりの順で足し直した Imports を返す（plugins/go/spec/go.hozo の Requirement.Layout の「パッケージと import パス」の
// 「import の名前」）: 名前空間のパッケージでないもの（標準ライブラリ、runtime、内側のパッケージなど）をパスの辞書順に、
// その後に、名前空間のパッケージを、名前空間の FQN の辞書順に。名前が当たれば、後の方に 2、3、… が付く。
// ファイルを一度作って import の集まりを知り、返った Imports で作り直せば、名前は、足した順（型を書いた順）によらない。
func (im *Imports) Canonical() *Imports {
	out := &Imports{view: im.view, layout: im.layout, inDef: im.inDef, inTypes: im.inTypes, outside: im.outside, declNS: im.declNS, homes: im.homes,
		reserved: append([]string(nil), im.reserved...)}
	out.init()
	var plain, spaces []string
	for _, p := range im.Paths() {
		if _, isNS := im.nsOf[p]; isNS {
			spaces = append(spaces, p)
		} else {
			plain = append(plain, p)
		}
	}
	sort.SliceStable(spaces, func(i, j int) bool { return im.nsOf[spaces[i]] < im.nsOf[spaces[j]] })
	for _, p := range plain {
		out.Add(p, im.pkgNames[p])
	}
	for _, p := range spaces {
		out.Add(p, im.pkgNames[p])
		out.nsOf[p] = im.nsOf[p]
		out.named[p] = im.named[p]
	}
	return out
}

// Qualifier は、名前空間 namespace の宣言（型、例外、述語で決まる分類の名前）を、このファイルから書くときの前置き:
// 見る側の名前空間 N なら ""（修飾なし。テストのパッケージのファイルでは、N の入口のパッケージを import して `<名前>.`）、
// 別の名前空間なら、その型の置き場の入口のパッケージを import して `<名前>.`。
// 置き場がなければ "" を返し、Err がそれを報告する。
func (im *Imports) Qualifier(namespace string) string {
	if im == nil || im.view == nil {
		return ""
	}
	if namespace == im.view.Viewer {
		if !im.outside || im.layout == nil {
			return ""
		}
		// テストのパッケージから見た N: 入口のパッケージ
		im.init()
		name := im.Add(im.layout.EntryPath, im.layout.EntryName)
		im.nsOf[im.layout.EntryPath] = namespace
		im.named[im.layout.EntryPath] = true
		return name + "."
	}
	home := im.home(namespace)
	if home == nil {
		return ""
	}
	im.init()
	// 別の名前空間の型と分類は、型のパッケージから（plugins/go/spec/go.hozo の Requirement.Layout の「パッケージと import パス」）。テストのパッケージのファイルは、
	// 入口から（入口は、型のすべての別名を持つ）
	pkgPath := home.TypesPath
	if im.outside {
		pkgPath = home.EntryPath
	}
	name := im.Add(pkgPath, home.EntryName)
	im.nsOf[pkgPath] = namespace
	return name + "."
}

// Err は、GoType / Zero / Qualifier が、型の置き場を求められなかった名前空間があれば、その誤り（最初の一つ）。なければ nil。
// 置き場があることは hozoc が確かめている（CFG-22）ので、基底は GO-INTERNAL にする。module がないとき（ErrNoModule）もここに出る。
func (im *Imports) Err() error {
	if im == nil {
		return nil
	}
	return im.err
}

// program は view の Program。なければ nil。
func (im *Imports) program() *ir.Program {
	if im == nil || im.view == nil {
		return nil
	}
	return im.view.Program
}

// namespaceOf は宣言 fqn を宣言した名前空間。Program にない宣言は、FQN の形から推測する。
func (im *Imports) namespaceOf(fqn string) string {
	if p := im.program(); p != nil {
		if im.declNS == nil {
			im.declNS = map[string]string{}
			for i := range p.Namespaces {
				for _, d := range p.Namespaces[i].Decls {
					if d == nil {
						continue
					}
					if _, dup := im.declNS[d.Common().Name]; !dup {
						im.declNS[d.Common().Name] = p.Namespaces[i].Name
					}
				}
			}
		}
		if ns, ok := im.declNS[fqn]; ok {
			return ns
		}
	}
	return guessNamespace(fqn)
}

// home は名前空間 namespace の型の置き場（HomeOf）。なければ nil で、誤りを覚える。
func (im *Imports) home(namespace string) *Layout {
	if l, ok := im.homes[namespace]; ok {
		return l
	}
	l, ok, err := HomeOf(im.view, namespace)
	switch {
	case err != nil:
		if im.err == nil {
			im.err = fmt.Errorf("名前空間 %s の型の置き場: %w", namespace, err)
		}
		l = nil
	case !ok:
		if im.err == nil {
			im.err = fmt.Errorf("名前空間 %s の型の置き場がない（基底 %s の単位が、%s を出力していない）", namespace, im.view.Config.Base, namespace)
		}
		l = nil
	}
	if im.homes == nil {
		im.homes = map[string]*Layout{}
	}
	im.homes[namespace] = l
	return l
}

// ref は ref の FQN を、このファイルから書く Go の型の名前にする（GoType の説明）。
func (im *Imports) ref(fqn string) string {
	if im == nil || im.view == nil {
		return TypeName(fqn, guessNamespace(fqn))
	}
	ns := im.namespaceOf(fqn)
	return im.Qualifier(ns) + TypeName(fqn, ns)
}

func isStdlib(p string) bool {
	first := p
	if i := strings.IndexByte(p, '/'); i >= 0 {
		first = p[:i]
	}
	return !strings.Contains(first, ".")
}
