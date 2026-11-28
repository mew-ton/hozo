package runtime

import (
	"fmt"
	"strings"
)

// Effect は効果の名前（FQN）。生成したコードは効果ごとに
// `const EffectCreateSession runtime.Effect = "server.createSession"` を出す
// （plugins/go/spec/go.hozo の Requirement.Types の「effect」）。
type Effect string

// Exception は、生成した例外の型が満たすインターフェース（plugins/go/spec/go.hozo の Requirement.Types の「exception」）。
// HozoException は例外の FQN を返す。
type Exception interface {
	error
	HozoException() string
}

// Violation は valid.* の制約への違反が一つ（docs/plugin-go-valid.md「runtime」）。
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
// （違反がなければ Validate は nil を返す。docs/plugin-go-valid.md「runtime」）。
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

// ClassificationError は分類の誤り（plugins/go/spec/go.hozo の Requirement.Wiring の「分類」、Requirement.Wiring の「match」の「分類の誤りの枝」）: 値が、分類が宣言したケースの
// どれにも当たらないか、二つ以上の述語が真になった。判定関数と、型で決まる分類の match が返す。
// Classify は分類の FQN、Hits は真になったケースの名前（書かれた順）。どのケースにも当たらなければ空、
// 真が二つ以上ならそのすべて。
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

// CaseError は、効果の入口の確かめの失敗（plugins/go/spec/go.hozo の Requirement.Ports の「効果の入口の確かめ」）: 絞り込んだ引数が、言ったとおりのケースに
// ない。仕様の例外ではない（Exception を満たさない）。効果は、起きていない。
// Effect は効果の FQN、Param は引数の名前、Classify は分類の FQN、Want は、引数の型が言うケース（型に書かれた順）、
// Got は、判定関数が返したケース。
type CaseError struct {
	Effect, Param, Classify string
	Want                    []string
	Got                     string
}

// Error は "effect <Effect>: argument <Param> is in case <Got> of <Classify>, want <Want を ", " で>"。
func (e *CaseError) Error() string {
	return fmt.Sprintf("effect %s: argument %s is in case %s of %s, want %s", e.Effect, e.Param, e.Got, e.Classify, strings.Join(e.Want, ", "))
}
