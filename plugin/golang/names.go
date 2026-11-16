// Package golang は、Go の基底 go（docs/plugin-go.md）の形の規則の写し。基底のプラグイン（plugins/go）と、それに相乗りする
// Go のトランスパイラが共有する: 相乗りは、基底の出力を見ずに、ここの関数で基底の置き場と名前を求める
// （docs/plugin-protocol.md「相乗りが基底の形を知る方法」）。
//
// 規則の持ち主は docs/plugin-go.md で、API の形は docs/packages.md「`plugin/golang`（公開）」。それぞれの関数のコメントに、
// 写した見出しを書く。当たる名前（GO-2）と import の循環（GO-3）を見つけるのは基底のプラグインで、このパッケージは、
// 当たらない前提で名前を返す。
package golang

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// keywords は Go の予約語（Go 言語仕様「Keywords」）。docs/plugin-go.md「名前」の「局所の名前」の判定に使う。
var keywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// IsKeyword は name が Go の予約語か。
func IsKeyword(name string) bool { return keywords[name] }

// predeclared は Go の事前宣言の識別子（Go 言語仕様「Predeclared identifiers」）。
// 小文字のまま出す名前（引数名、束縛名）がこれと同じだと、生成したコードの中の型や関数を隠してしまう
// （引数 `error` があると戻り値の型 `error` が引数を指す）ので、LocalName が避ける。
var predeclared = map[string]bool{
	// 型
	"any": true, "bool": true, "byte": true, "comparable": true,
	"complex64": true, "complex128": true, "error": true, "float32": true, "float64": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true, "rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	// 定数と零値
	"true": true, "false": true, "iota": true, "nil": true,
	// 関数
	"append": true, "cap": true, "clear": true, "close": true, "complex": true, "copy": true, "delete": true,
	"imag": true, "len": true, "make": true, "max": true, "min": true, "new": true, "panic": true,
	"print": true, "println": true, "real": true, "recover": true,
}

// IsPredeclared は name が Go の事前宣言の識別子か。
func IsPredeclared(name string) bool { return predeclared[name] }

// Capitalize は先頭の一文字だけを大文字にする（`id` → `Id`、`userId` → `UserId`、`URL` → `URL`）。
// Go の慣習（`ID`）より決定性と予測しやすさを取る（docs/plugin-go.md「名前」）。
func Capitalize(s string) string {
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && n <= 1 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// lowerFirst は先頭の一文字だけを小文字にする（`Login` → `login`）。
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && n <= 1 {
		return s
	}
	return string(unicode.ToLower(r)) + s[n:]
}

// short は FQN の最後のセグメント（宣言名）。`.` がなければそのまま。
func short(fqn string) string {
	if i := strings.LastIndexByte(fqn, '.'); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

// escapeKeyword は Go の予約語なら末尾に `_` を付ける。
func escapeKeyword(s string) string {
	if keywords[s] {
		return s + "_"
	}
	return s
}

// TypeName は宣言の FQN から Go の型の名前を作る（docs/plugin-go.md「名前」の「宣言名」「合成した model」）。
// namespace はその宣言を宣言した名前空間の FQN（グローバルは ""）で、fqn からその前置きを外した残りの
// セグメントを、それぞれ先頭を大文字にして連結する:
//
//	TypeName("server.User", "server")               → "User"
//	TypeName("browser.OrderPage.Ready", "browser")  → "OrderPageReady"
//	TypeName("server.ClientError.Value", "server")  → "ClientErrorValue"
//	TypeName("db", "")                              → "Db"
//
// fqn が namespace の下になければ、fqn 全体をセグメントごとに連結する（呼ぶ側の誤り。黙って通す）。
func TypeName(fqn, namespace string) string {
	rest := fqn
	if namespace != "" && strings.HasPrefix(fqn, namespace+".") {
		rest = fqn[len(namespace)+1:]
	}
	var b strings.Builder
	for _, seg := range strings.Split(rest, ".") {
		b.WriteString(Capitalize(seg))
	}
	return escapeKeyword(b.String())
}

// FieldName はフィールド名を Go の公開フィールドの名前にする（docs/plugin-go.md「名前」の「フィールド名」「例外の引数名」）。
// 先頭だけを大文字にする（`id` → `Id`、`userId` → `UserId`）。JSON のタグは元の名前のまま呼ぶ側が付ける。
// 例外の構造体のフィールド（引数ごとに一つ。同「exception」）も、引数名から、これで作る（`retryAfter` → `RetryAfter`）。
// 大文字化で当たる二つの名前（`id` と `Id`）を見つける（GO-2）のは、基底のプラグイン。
func FieldName(name string) string { return escapeKeyword(Capitalize(name)) }

// MethodName は op、read、効果の名前を Go のメソッド名にする（docs/plugin-go.md「名前」の「op 名」「read 名」「effect 名」）:
// 先頭を大文字に（`login` → `Login`、`userByEmail` → `UserByEmail`、`createSession` → `CreateSession`）。op の export の有無に
// よらない。FQN を渡してもよい（最後のセグメントを使う）。
func MethodName(name string) string { return escapeKeyword(Capitalize(short(name))) }

// BuiltinMethod は、直に呼ぶ組み込みの効果の操作を、状態の単位のメソッド名にする（docs/plugin-go.md「名前」の
// 「直に呼ぶ組み込みの効果の操作」）: 先頭を大文字に（`post` → `Post`、`put` → `Put`、`delete` → `Delete`）。
func BuiltinMethod(operation string) string { return Capitalize(operation) }

// LocalName は op・read・効果の引数名や配線の束縛名を Go の局所の名前にする（docs/plugin-go.md「名前」の「局所の名前」）。
// 元の名前のままで、Go の予約語か、事前宣言の識別子か、reserved にあれば、末尾に `_`（`type` → `type_`、`error` → `error_`）。
// reserved は、生成したコードがそのメソッドの中で使う名前（`o`、`ctx`、`err`、`ptr`、そのファイルが import するパッケージの名前）で、
// 呼ぶ側が渡す。nil でもよい。
func LocalName(name string, reserved map[string]bool) string {
	if keywords[name] || predeclared[name] || reserved[name] {
		return name + "_"
	}
	return name
}

// EffectConst は effect 名を定数の名前にする（docs/plugin-go.md「名前」の「effect 名」）:
// `createSession` → `EffectCreateSession`。FQN を渡してもよい（最後のセグメントを使う）。
func EffectConst(name string) string { return "Effect" + Capitalize(short(name)) }

// CaseConst は分類のケースの定数の名前（docs/plugin-go.md「名前」の「分類 C の列挙と述語」）。
// classify はその分類の Go の型名（TypeName の結果）、caseName はケース名か "default":
//
//	CaseConst("WarrantyKind", "yamada")   → "WarrantyKindYamada"
//	CaseConst("WarrantyKind", "default")  → "WarrantyKindDefault"
func CaseConst(classify, caseName string) string {
	if caseName == "default" {
		return classify + "Default"
	}
	return classify + Capitalize(caseName)
}

// StateUnitName は状態の単位のインターフェースの名前（docs/plugin-go.md「名前」の「状態の単位」）: 状態（か状態の分類）の名前を
// 先頭大文字にして State を付ける（`users` → `UsersState`、`OutputsSync` → `OutputsSyncState`）。FQN を渡してもよい。
func StateUnitName(stateOrClassify string) string {
	return Capitalize(short(stateOrClassify)) + "State"
}

// DepName は別の名前空間の口の、インターフェースの名前と、New の引数の名前（docs/plugin-go.md「名前」の「別の名前空間の口」、
// 「New」）。型は、名前空間の FQN のセグメントをそれぞれ先頭大文字にして連結し、Ops を付ける。引数は、先頭のセグメントを
// 小文字で始め、二つ目から先頭大文字にしてつなぐ。Go の予約語か事前宣言の識別子なら末尾に `_`:
//
//	DepName("store")       → "StoreOps", "store"
//	DepName("shop.admin")  → "ShopAdminOps", "shopAdmin"
//	DepName("type")        → "TypeOps", "type_"
//
// 先に並んだ引数の名前と当たるときの `_` は、ShapeOf が New の引数を並べるときに足す。
func DepName(namespace string) (typeName, param string) {
	var t, p strings.Builder
	for i, seg := range strings.Split(namespace, ".") {
		t.WriteString(Capitalize(seg))
		if i == 0 {
			p.WriteString(lowerFirst(seg))
		} else {
			p.WriteString(Capitalize(seg))
		}
	}
	return t.String() + "Ops", LocalName(p.String(), nil)
}

// SeamMethod は継ぎ目のメソッド（インターフェース Seams のメソッド）の名前（docs/plugin-go.md「名前」「継ぎ目」）:
// op 名を先頭大文字にしたものに、デコレータの FQN のセグメントをそれぞれ先頭大文字にして連結したものを付ける
// （`login` と `rate.limited` → `LoginRateLimited`）。op は FQN を渡してもよい。
func SeamMethod(opName, decorator string) string {
	var b strings.Builder
	b.WriteString(Capitalize(short(opName)))
	for _, seg := range strings.Split(decorator, ".") {
		b.WriteString(Capitalize(seg))
	}
	return b.String()
}

// BodyName は、継ぎ目を持ち、基底が中身を出す op（配線と、never を返す op）の、中身のメソッドの名前
// （docs/plugin-go.md「名前」「継ぎ目」の「中身」）: op 名の先頭を小文字にして Body を付ける（`login` → `loginBody`）。
// op は FQN を渡してもよい。
func BodyName(opName string) string { return lowerFirst(short(opName)) + "Body" }

// PackageName は名前空間の FQN から Go のパッケージ名を作る（docs/plugin-go.md「パッケージと import パス」）:
// 最後のセグメントを小文字にし、英数字（ASCII）以外を落とす。`server.admin` → `admin`、`my-api` → `myapi`。
// 残りが空か、数字で始まるか、Go の予約語なら、`pkg` を前に付ける（名前空間 `type` は `pkgtype`）。
func PackageName(namespace string) string {
	seg := short(namespace)
	var b strings.Builder
	for _, r := range strings.ToLower(seg) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	name := b.String()
	if name == "" || name[0] >= '0' && name[0] <= '9' || keywords[name] {
		return "pkg" + name
	}
	return name
}
