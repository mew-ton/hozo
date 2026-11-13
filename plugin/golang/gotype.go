package golang

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mew-ton/hozo/ir"
)

// GoType は IR の型を Go の型の文字列にする（docs/emit-go.md「型」の表）。必要な import を imports に足す。
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
// ref の修飾は imports の Resolve に従う（Resolver の説明）: ローカルなら TypeName だけ、
// 別の Go のパッケージなら imports.Add したパッケージの名前で修飾する。imports が nil なら
// import は記録せず、すべての ref をローカル（Viewer は ""）として名前にする。
// nil の型（void）は "" を返す。
func GoType(t ir.Type, imports *Imports) string {
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
		inner := GoType(x.Of, imports)
		if IsNillable(x.Of) {
			return inner
		}
		return "*" + inner
	case ir.ArrayType:
		return "[]" + GoType(x.Of, imports)
	case ir.RecordType:
		return "map[" + GoType(x.Key, imports) + "]" + GoType(x.Value, imports)
	case ir.CaseType:
		return GoType(x.Of, imports)
	}
	panic(fmt.Sprintf("golang: unknown ir.Type %T", t))
}

// IsNillable は、その型の Go の零値 nil が「なし」を表せるか（array と record。case は of を見る。docs/emit-go.md「型」の
// optional の行）。optional(T) が *T にならず T のままになるのは、T がこれに当たるとき。
func IsNillable(t ir.Type) bool {
	switch x := t.(type) {
	case ir.ArrayType, ir.RecordType:
		return true
	case ir.CaseType:
		return IsNillable(x.Of)
	}
	return false
}

// qualify は imports に pkgPath を足し、修飾に使う名前を返す。imports が nil ならパッケージ名そのまま。
func qualify(imports *Imports, pkgPath, pkgName string) string {
	if imports == nil {
		return pkgName
	}
	return imports.Add(pkgPath, pkgName)
}

// refType は ref の FQN を Go の型の名前にする（GoType の説明）。
func refType(fqn string, imports *Imports) string {
	if imports == nil {
		return TypeName(fqn, "")
	}
	if imports.Resolve != nil {
		namespace, pkgPath, pkgName, local := imports.Resolve(fqn)
		if local {
			return TypeName(fqn, namespace)
		}
		return imports.Add(pkgPath, pkgName) + "." + TypeName(fqn, namespace)
	}
	if imports.Viewer == "" || strings.HasPrefix(fqn, imports.Viewer+".") {
		return TypeName(fqn, imports.Viewer)
	}
	return TypeName(fqn, guessNamespace(fqn))
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
