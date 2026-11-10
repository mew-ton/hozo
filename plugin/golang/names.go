// Package golang は、hozoc の Go の既定の出力と Go のプラグインが共有する名前と型の規則。
//
// 規則の持ち主は docs/emit-go.md「名前」「型」「配置」で、API の形は docs/packages.md「`plugin/golang`（公開）」。
// 既定の出力（internal/emit/golang）とプラグイン（plugins/go-cli）が同じ名前を出すために、
// 規則をここに一度だけ書く。
package golang

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// keywords は Go の予約語（Go 言語仕様「Keywords」）。
// docs/emit-go.md「名前」の「Go の予約語と衝突する名前 → 末尾に `_`」の判定に使う。
var keywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// IsKeyword は name が Go の予約語か。
func IsKeyword(name string) bool { return keywords[name] }

// Capitalize は先頭の一文字だけを大文字にする（`id` → `Id`、`userId` → `UserId`、`URL` → `URL`）。
// Go の慣習（`ID`）より決定性と予測しやすさを取る（docs/emit-go.md「名前」）。
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

// escapeKeyword は Go の予約語なら末尾に `_` を付ける（docs/emit-go.md「名前」）。
func escapeKeyword(s string) string {
	if keywords[s] {
		return s + "_"
	}
	return s
}

// TypeName は宣言の FQN から Go の型の名前を作る（docs/emit-go.md「名前」の「宣言名」「合成した model」）。
// namespace はその宣言を持つ名前空間の FQN（グローバルは ""）で、fqn からその前置きを外した残りの
// セグメントを、それぞれ先頭を大文字にして連結する:
//
//	TypeName("server.User", "server")               → "User"
//	TypeName("browser.OrderPage.Ready", "browser")  → "OrderPageReady"
//	TypeName("server.LoginFailed.Value", "server")  → "LoginFailedValue"
//	TypeName("db", "")                              → "Db"
//
// bind で引数を除いた合成 model `<viewer>.<op 名>$<引数名>`（docs/ir.md「その名前空間から見た IR」）は
// `<Op><Param>Input` にする（docs/emit-go.md「op」の「インターフェース」）:
//
//	TypeName("browser.getMe$in", "browser")         → "GetMeInInput"
//
// fqn が namespace の下になければ、fqn 全体をセグメントごとに連結する（呼ぶ側の誤り。黙って通す）。
func TypeName(fqn, namespace string) string {
	rest := fqn
	if namespace != "" && strings.HasPrefix(fqn, namespace+".") {
		rest = fqn[len(namespace)+1:]
	}
	suffix := ""
	if strings.IndexByte(rest, '$') >= 0 {
		// bind で合成した model `<op>$<param>[$<field>...]`（docs/ir.md「その名前空間から見た IR」）は、
		// `$` のセグメントもそれぞれ先頭大文字にして連結し、末尾に Input（docs/emit-go.md「名前」）。
		rest = strings.ReplaceAll(rest, "$", ".")
		suffix = "Input"
	}
	var b strings.Builder
	for _, seg := range strings.Split(rest, ".") {
		b.WriteString(Capitalize(seg))
	}
	b.WriteString(suffix)
	return escapeKeyword(b.String())
}

// FieldName はフィールド名を Go の公開フィールドの名前にする（docs/emit-go.md「名前」の「フィールド名」）。
// 先頭だけを大文字にし、Go の予約語なら末尾に `_`（大文字で始まる名前は予約語にならないので、
// 実際に `_` が付くのは先頭が大文字にできない名前だけ）。JSON のタグは元の名前のまま呼ぶ側が付ける。
// 大文字化で衝突する二つの名前（`id` と `Id`）の検出（GOEMIT-2）は呼ぶ側が行う。
func FieldName(name string) string { return escapeKeyword(Capitalize(name)) }

// MethodName は op 名を Go のメソッド名にする（docs/emit-go.md「名前」の「op 名」）。規則は FieldName と同じ。
func MethodName(opName string) string { return escapeKeyword(Capitalize(opName)) }

// LocalName は op の引数名や配線の束縛名など、小文字のまま出す識別子を Go の識別子にする。
// 元の名前のままで、Go の予約語なら末尾に `_`（`type` → `type_`）。
// docs/emit-go.md「名前」の表にない行だが、「Go の予約語と衝突する名前 → 末尾に `_`」は
// 大文字にしない位置でだけ実際に起きるので、その適用先としてここに置く。
func LocalName(name string) string { return escapeKeyword(name) }

// EffectConst は effect 名を定数の名前にする（docs/emit-go.md「名前」の「effect 名」）:
// `createSession` → `EffectCreateSession`。name は FQN ではなく宣言名。
func EffectConst(name string) string { return "Effect" + Capitalize(name) }

// CaseConst は classify のケースの定数の名前（docs/emit-go.md「名前」の「classify のケース」）。
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

// PackageName は名前空間の FQN から Go のパッケージ名を作る（docs/emit-go.md「配置」）:
// 最後のセグメントを小文字にし、英数字（ASCII）以外を落とす。`server.admin` → `admin`、`my-api` → `myapi`。
// 残りが空か、数字で始まるか、Go の予約語なら、`pkg` を前に付ける（有効な識別子にするため。文書にない補い）。
func PackageName(namespace string) string {
	seg := namespace
	if i := strings.LastIndexByte(namespace, '.'); i >= 0 {
		seg = namespace[i+1:]
	}
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
