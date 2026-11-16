package golang

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// Imports は、生成する一つのファイルの import の集まり（docs/plugin-go.md「パッケージと import パス」の「import の名前」）。
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
	root   string
	inDef  bool

	paths    map[string]string // import パス → 使う名前
	names    map[string]string // 使う名前 → import パス
	pkgNames map[string]string // import パス → パッケージの名前（Add に渡されたもの）
	nsOf     map[string]string // 名前空間のパッケージの import パス → 名前空間の FQN
	declNS   map[string]string // 宣言の FQN → 宣言した名前空間（view.Program から。最初に引いたときに作る）
	homes    map[string]*Layout
	err      error
}

// NewImports は、view の出力の一つのファイルの import を作る。layout は、その出力の置き場（LayoutOf(view, root)）、
// root はプロジェクトのルート。inDef は、書くファイルが定義のパッケージのものか。入口のパッケージのファイル（分けた出力の
// 入口の再公開と、相乗りのファイル）なら false で、そのとき、内側のパッケージ `inner` の名前は、ほかの import に使わせない。
// どちらでも、N の型は修飾なしの名前になる（入口は、定義のすべての型の別名を持つ）。
func NewImports(view *ir.View, layout *Layout, root string, inDef bool) *Imports {
	return &Imports{view: view, layout: layout, root: root, inDef: inDef}
}

func (im *Imports) init() {
	if im.paths != nil {
		return
	}
	im.paths, im.names, im.pkgNames, im.nsOf = map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	if im.layout != nil && im.layout.HasInner && !im.inDef {
		// 入口のファイルでは、inner は内側のパッケージの名前（「標準ライブラリ、runtime、inner の名前は変えない」）
		im.names[innerDir] = im.layout.DefPath
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
// （docs/plugin-go.md「名前」の「局所の名前」）を作るのに使う。
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
// 辞書順。名前がパスの最後の要素と違えば、名前を書く。一つなら `import "time"` の一行、なければ ""。末尾に改行を一つ付ける。
func (im *Imports) Render() string {
	ps := im.Paths()
	if len(ps) == 0 {
		return ""
	}
	line := func(p string) string {
		if name := im.paths[p]; name != path.Base(p) {
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

// Canonical は、同じ import の集まりを、文書の決まりの順で足し直した Imports を返す（docs/plugin-go.md「パッケージと import パス」の
// 「import の名前」）: 名前空間のパッケージでないもの（標準ライブラリ、runtime、内側のパッケージなど）をパスの辞書順に、
// その後に、名前空間のパッケージを、名前空間の FQN の辞書順に。名前が当たれば、後の方に 2、3、… が付く。
// ファイルを一度作って import の集まりを知り、返った Imports で作り直せば、名前は、足した順（型を書いた順）によらない。
func (im *Imports) Canonical() *Imports {
	out := &Imports{view: im.view, layout: im.layout, root: im.root, inDef: im.inDef, declNS: im.declNS, homes: im.homes}
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
	}
	return out
}

// Qualifier は、名前空間 namespace の宣言（型、例外、述語で決まる分類の名前）を、このファイルから書くときの前置き:
// 見る側の名前空間 N なら ""（修飾なし）、別の名前空間なら、その型の置き場の入口のパッケージを import して `<名前>.`。
// 置き場がなければ "" を返し、Err がそれを報告する。
func (im *Imports) Qualifier(namespace string) string {
	if im == nil || im.view == nil || namespace == im.view.Viewer {
		return ""
	}
	home := im.home(namespace)
	if home == nil {
		return ""
	}
	im.init()
	name := im.Add(home.EntryPath, home.EntryName)
	im.nsOf[home.EntryPath] = namespace
	return name + "."
}

// Err は、GoType / Zero / Qualifier が、型の置き場を求められなかった名前空間があれば、その誤り（最初の一つ）。なければ nil。
// 置き場があることは hozoc が確かめている（CFG-22）ので、基底は GO-INTERNAL にする。go.mod がないとき（ErrNoGoMod）もここに出る。
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
	l, ok, err := HomeOf(im.view, namespace, im.root)
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
