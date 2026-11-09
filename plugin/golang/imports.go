package golang

import (
	"path"
	"sort"
	"strconv"
	"strings"
)

// Resolver は、参照（`ref`）の FQN について、その宣言を持つ名前空間と Go のパッケージを答える
// （docs/emit-go.md「配置」の「別の名前空間 M の型と op を使うとき」）。
//
//   - namespace は宣言を持つ名前空間の FQN。TypeName がセグメントを切るのに使う。
//   - local が true なら、見る側（viewer）のパッケージにある型で、修飾しない。viewer 自身の宣言と、
//     Go でない名前空間から viewer のパッケージに複製した型（§4.5）がこれ。
//   - local が false なら pkgPath を import し、pkgName（そのパッケージの名前）で修飾する。
//     同じプロジェクトの Go の名前空間がこれ。
//
// FQN の文字列だけからは名前空間の境界（`server.OrderPage.Ready` の `server`）が決まらず、
// 言語の設定も Program の外（docs/config.md）にあるので、判断は呼ぶ側に任せる。
type Resolver func(fqn string) (namespace, pkgPath, pkgName string, local bool)

// Imports は生成するファイルの import の集まり。GoType が必要な import を足し、Render が import ブロックを出す。
// ゼロ値で使える。Viewer と Resolve は GoType が `ref` を名前にするときに使う（Resolve の説明）。
type Imports struct {
	// Viewer は見る側の名前空間の FQN。Resolve が nil のとき、この名前空間の下の FQN をローカルの型にする。
	Viewer string
	// Resolve は参照先の名前空間とパッケージを答える。nil なら、Viewer の下の FQN はローカル、
	// それ以外は「先頭が大文字でない先頭からのセグメントの列」を名前空間とみなしてローカルの型（複製された型）にする。
	Resolve Resolver

	paths map[string]string // import パス → 使う名前（パッケージ名か別名）
	names map[string]string // 使う名前 → import パス
}

// Add は pkgPath を import に足し、そのパッケージを修飾するのに使う名前を返す。
// pkgName はそのパッケージの `package` 節の名前。パスの最後の要素と違うか、すでに別のパスが同じ名前を
// 使っていれば別名を付ける（`pkgName2`、`pkgName3`、…）。同じパスを二度足しても同じ名前を返す。
func (im *Imports) Add(pkgPath, pkgName string) string {
	if im.paths == nil {
		im.paths = map[string]string{}
		im.names = map[string]string{}
	}
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
	return name
}

// Paths は import パスを辞書順で返す。
func (im *Imports) Paths() []string {
	ps := make([]string, 0, len(im.paths))
	for p := range im.paths {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

// Name は pkgPath を修飾するのに使う名前（Add の戻り値）。足していなければ ""。
func (im *Imports) Name(pkgPath string) string { return im.paths[pkgPath] }

// Render は import ブロックを返す。標準ライブラリ（パスの先頭の要素に `.` がない）を先に、
// 空行を挟んでそれ以外を、どちらも辞書順で並べる。パッケージ名がパスの最後の要素と違えば別名を書く。
// 一つなら `import "time"` の一行、なければ ""。末尾に改行を一つ付ける。
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

func isStdlib(p string) bool {
	first := p
	if i := strings.IndexByte(p, '/'); i >= 0 {
		first = p[:i]
	}
	return !strings.Contains(first, ".")
}
