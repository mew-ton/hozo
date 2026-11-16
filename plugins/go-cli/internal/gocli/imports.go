package gocli

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは、cli.go の import と型の名前の求め方。前の形の出力（hozoc 本体に組み込んでいた Go の出力。別の名前空間の型を
// 見る側のパッケージに複製していた）を前提にしたもので、公開の SDK plugin/golang から、このプラグインの中へ移した。
// 基底 go の形（docs/plugin-go.md）に合わせた作り直しは、go-cli の作り直しのとき（docs/plugin-go-cli.md）。

// Resolver は、参照（`ref`）の FQN について、その宣言を持つ名前空間と Go のパッケージを答える
// （これまでの文書「Go の既定の出力」「配置」の「別の名前空間 M の型と op を使うとき」）。
//
//   - namespace は宣言を持つ名前空間の FQN。TypeName がセグメントを切るのに使う。
//   - local が true なら、見る側（viewer）のパッケージにある型で、修飾しない。viewer 自身の宣言と、
//     Go でない名前空間から viewer のパッケージに複製した型（§4.5）がこれ。
//   - local が false なら pkgPath を import し、pkgName（そのパッケージの名前）で修飾する。
//     同じプロジェクトの Go の名前空間がこれ。
//
// FQN の文字列だけからは名前空間の境界（`server.OrderPage.Ready` の `server`）が決まらず、
// 言語の設定も Program の外（docs/config.md）にあるので、判断は呼ぶ側に任せる。
type resolver func(fqn string) (namespace, pkgPath, pkgName string, local bool)

// cliImports は生成するファイルの import の集まり。GoType が必要な import を足し、Render が import ブロックを出す。
// ゼロ値で使える。Viewer と Resolve は GoType が `ref` を名前にするときに使う（Resolve の説明）。
type cliImports struct {
	// Viewer は見る側の名前空間の FQN。Resolve が nil のとき、この名前空間の下の FQN をローカルの型にする。
	Viewer string
	// Resolve は参照先の名前空間とパッケージを答える。nil なら、Viewer の下の FQN はローカル、
	// それ以外は「先頭が大文字でない先頭からのセグメントの列」を名前空間とみなしてローカルの型（複製された型）にする。
	Resolve resolver

	paths map[string]string // import パス → 使う名前（パッケージ名か別名）
	names map[string]string // 使う名前 → import パス
}

// Add は pkgPath を import に足し、そのパッケージを修飾するのに使う名前を返す。
// pkgName はそのパッケージの `package` 節の名前。パスの最後の要素と違うか、すでに別のパスが同じ名前を
// 使っていれば別名を付ける（`pkgName2`、`pkgName3`、…）。同じパスを二度足しても同じ名前を返す。
func (im *cliImports) Add(pkgPath, pkgName string) string {
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
func (im *cliImports) Paths() []string {
	ps := make([]string, 0, len(im.paths))
	for p := range im.paths {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

// Name は pkgPath を修飾するのに使う名前（Add の戻り値）。足していなければ ""。
func (im *cliImports) Name(pkgPath string) string { return im.paths[pkgPath] }

// Render は import ブロックを返す。標準ライブラリ（パスの先頭の要素に `.` がない）を先に、
// 空行を挟んでそれ以外を、どちらも辞書順で並べる。パッケージ名がパスの最後の要素と違えば別名を書く。
// 一つなら `import "time"` の一行、なければ ""。末尾に改行を一つ付ける。
func (im *cliImports) Render() string {
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

// goType は IR の型を Go の型の文字列にする（これまでの文書「Go の既定の出力」「型」の表）。必要な import を imports に足す。
//
//	string / boolean / int / float   → string / bool / int64 / float64
//	Time / Duration                  → time.Time / time.Duration（"time" を import）
//	optional(T)                      → T が array か record ならそのまま、それ以外は *T
//	array(T)                         → []T
//	record(K, V)                     → map[K]V
//	ref                              → 型の名前（model は構造体、scalar は名前付きの型、union はインターフェース。
//	                                   どれも名前の規則は同じ TypeName）
//	case(C, cases, of)               → of の型（絞り込みは Go の型にしない。docs/decisions.md 29）
//
// optional の中の case は of を見て判断する（`optional(case(C, c, array(T)))` は `[]T`）。
//
// ref の修飾は imports の Resolve に従う（resolver の説明）: ローカルなら TypeName だけ、
// 別の Go のパッケージなら imports.Add したパッケージの名前で修飾する。imports が nil なら
// import は記録せず、すべての ref をローカル（Viewer は ""）として名前にする。
// nil の型（void）は "" を返す。
func goType(t ir.Type, imports *cliImports) string {
	switch x := t.(type) {
	case nil:
		return ""
	case ir.CoreType:
		switch x.Name {
		case ir.CoreString:
			return "string"
		case ir.CoreBoolean:
			return "bool"
		case ir.CoreInt:
			return "int64"
		case ir.CoreFloat:
			return "float64"
		case ir.CoreTime:
			return qualify(imports, "time", "time") + ".Time"
		case ir.CoreDuration:
			return qualify(imports, "time", "time") + ".Duration"
		}
		panic(fmt.Sprintf("golang: unknown core type %q", x.Name))
	case ir.RefType:
		return refType(x.Target, imports)
	case ir.OptionalType:
		inner := goType(x.Of, imports)
		if isNillable(x.Of) {
			return inner
		}
		return "*" + inner
	case ir.ArrayType:
		return "[]" + goType(x.Of, imports)
	case ir.RecordType:
		return "map[" + goType(x.Key, imports) + "]" + goType(x.Value, imports)
	case ir.CaseType:
		return goType(x.Of, imports)
	}
	panic(fmt.Sprintf("golang: unknown ir.Type %T", t))
}

// isNillable は、その型の Go の零値 nil が「なし」を表せるか（array と record。case は of を見る。これまでの文書「Go の既定の出力」「型」の
// optional の行）。optional(T) が *T にならず T のままになるのは、T がこれに当たるとき。
func isNillable(t ir.Type) bool {
	switch x := t.(type) {
	case ir.ArrayType, ir.RecordType:
		return true
	case ir.CaseType:
		return isNillable(x.Of)
	}
	return false
}

// qualify は imports に pkgPath を足し、修飾に使う名前を返す。imports が nil ならパッケージ名そのまま。
func qualify(imports *cliImports, pkgPath, pkgName string) string {
	if imports == nil {
		return pkgName
	}
	return imports.Add(pkgPath, pkgName)
}

// refType は ref の FQN を Go の型の名前にする（GoType の説明）。
func refType(fqn string, imports *cliImports) string {
	if imports == nil {
		return golang.TypeName(fqn, "")
	}
	if imports.Resolve != nil {
		namespace, pkgPath, pkgName, local := imports.Resolve(fqn)
		if local {
			return golang.TypeName(fqn, namespace)
		}
		return imports.Add(pkgPath, pkgName) + "." + golang.TypeName(fqn, namespace)
	}
	if imports.Viewer == "" || strings.HasPrefix(fqn, imports.Viewer+".") {
		return golang.TypeName(fqn, imports.Viewer)
	}
	return golang.TypeName(fqn, guessNamespace(fqn))
}

// guessNamespace は FQN の名前空間を、先頭が大文字でないセグメントの先頭からの並びとみなす
// （Resolve がないときの補い。宣言名は大文字で始まる慣習 NAME-1 に頼る）。
// `server.OrderPage.Ready` → `server`、`server.admin.Report` → `server.admin`、`Db` → ""（空）。
func guessNamespace(fqn string) string {
	segs := strings.Split(fqn, ".")
	n := 0
	for n < len(segs)-1 {
		r, _ := utf8.DecodeRuneInString(segs[n])
		if unicode.IsUpper(r) {
			break
		}
		n++
	}
	return strings.Join(segs[:n], ".")
}
