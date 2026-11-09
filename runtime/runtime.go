// Package runtime は、hozoc が生成した Go のコードが import する型と関数を持つ。
//
// 形は docs/emit-go.md「runtime パッケージ」。生成したコードのヘッダに書く hozoc の版
// （docs/emit-go.md「配置」）と合わせて互換を保つので、API を変えるときはそちらも見る。
// 標準ライブラリにだけ依存する（docs/packages.md「依存の向き」）。
package runtime

import (
	"fmt"
	"strings"
)

// Effect は効果の名前（FQN）。生成したコードは効果ごとに
// `const EffectCreateSession runtime.Effect = "server.createSession"` を出す
// （docs/emit-go.md「effect」）。
type Effect string

// Exception は、生成した例外の型が満たすインターフェース（docs/emit-go.md「exception」）。
// HozoException は例外の FQN を返す。
type Exception interface {
	error
	HozoException() string
}

// Violation は valid.* の制約への違反が一つ（docs/emit-go.md「scalar」「model の Validate」）。
// Path は違反したフィールドへの道（scalar 自身なら ""、model なら "body.email" や "items[2].id"）、
// Constraint は制約の名前（"min"、"length"、"pattern"、"format"、ユーザーが足した制約の名前）。
type Violation struct {
	Path, Constraint string
}

// String は "path: constraint"。Path が空なら constraint だけ。
func (v Violation) String() string {
	if v.Path == "" {
		return v.Constraint
	}
	return v.Path + ": " + v.Constraint
}

// ValidationError は Validate が返す誤り。Violations は検査した順で、空にはならない
// （違反がなければ Validate は nil を返す。docs/emit-go.md「scalar」）。
type ValidationError struct {
	Violations []Violation
}

// Error は "validation failed: <violation>; <violation>" の形。
func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Violations))
	for i, v := range e.Violations {
		parts[i] = v.String()
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// ClassificationError は分類の判定関数が返す誤り（docs/emit-go.md「classify」）。
// Classify は分類の FQN、Hits は真になったケースの名前（書かれた順）。
// default のない分類で真が一つもなければ Hits は空、どの分類でも真が二つ以上ならそのすべて。
type ClassificationError struct {
	Classify string
	Hits     []string
}

// Error は "classification <Classify>: no case matched" か
// "classification <Classify>: 2 cases matched (A, B)"。
func (e *ClassificationError) Error() string {
	if len(e.Hits) == 0 {
		return fmt.Sprintf("classification %s: no case matched", e.Classify)
	}
	return fmt.Sprintf("classification %s: %d cases matched (%s)", e.Classify, len(e.Hits), strings.Join(e.Hits, ", "))
}
