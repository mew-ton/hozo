package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/syntax/token"
)

// committed はコミット済みの生成物の場所（docs/packages.md「生成するもの」）。
const committed = "../../../editors/vscode/syntaxes/hozo.tmLanguage.json"

// TestGenerated は生成し直した文法がコミット済みの生成物と一致することを確かめる
// （docs/testing.md「生成物の一致」）。ずれていれば `go generate ./editors/...` で生成し直す。
func TestGenerated(t *testing.T) {
	want, err := os.ReadFile(filepath.FromSlash(committed))
	if err != nil {
		t.Fatalf("コミット済みの生成物を読めない: %v", err)
	}
	got, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s が生成し直した内容とずれている。`go generate ./editors/...` で生成し直すこと", committed)
	}
}

// TestDeterministic は同じ入力から同じバイトが出ることを確かめる（docs/requirements.md「品質の要件」の決定性）。
func TestDeterministic(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		b, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatal("生成のたびに出力が変わる")
		}
	}
}

// TestKeywordsCovered は token.Keywords の予約語が、生成した予約語のパターン（repository の keywords.*）のちょうど一つに
// 現れ、予約語でない語が混ざらないことを確かめる。どの生成規則も使わない予約語 default（docs/grammar.md「予約語」）だけは、
// どのパターンにも現れない（予約語として塗らない。docs/editors.md「TextMate の文法」）。
func TestKeywordsCovered(t *testing.T) {
	data, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Repository map[string]struct {
			Name  string `json:"name"`
			Match string `json:"match"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatal(err)
	}

	// \b(?:a|b|c)\b の形から語を取り出す。
	form := regexp.MustCompile(`^\\b\(\?:([A-Za-z|]+)\)\\b$`)
	count := map[string]int{}
	scopes := map[string]string{}
	groups := 0
	for key, rule := range g.Repository {
		if !strings.HasPrefix(key, keywordPrefix) {
			continue
		}
		groups++
		m := form.FindStringSubmatch(rule.Match)
		if m == nil {
			t.Errorf("%s の match が予約語の選択の形でない: %q", key, rule.Match)
			continue
		}
		for _, w := range strings.Split(m[1], "|") {
			count[w]++
			scopes[w] = rule.Name
		}
	}
	if groups != 5 {
		t.Errorf("予約語のパターンは docs/editors.md の表の 5 行のはずが %d", groups)
	}
	for w := range token.Keywords {
		want := 1
		if w == "default" {
			want = 0
		}
		if count[w] != want {
			t.Errorf("予約語 %q が予約語のパターンに %d 回現れる（%d 回のはず）", w, count[w], want)
		}
	}
	if _, reserved := token.Keywords["condition"]; reserved || count["condition"] != 0 {
		t.Errorf("condition は予約語ではない（docs/grammar.md「予約語」）のに、予約語の表かパターンにある")
	}
	for w := range count {
		if _, ok := token.Keywords[w]; !ok {
			t.Errorf("予約語でない %q が予約語のパターンにある", w)
		}
	}

	// docs/editors.md「TextMate の文法」の表の対応。
	wantScope := map[string]string{
		"namespace": "keyword.declaration.hozo",
		"bind":      "keyword.declaration.hozo",
		"export":    "storage.modifier.hozo",
		"override":  "storage.modifier.hozo",
		"extends":   "keyword.control.hozo",
		"as":        "keyword.control.hozo",
		"reads":     "keyword.control.hozo",
		"any":       "keyword.control.hozo",
		"val":       "keyword.control.hozo",
		"disjoint":  "keyword.control.hozo",
		"collect":   "keyword.control.hozo",
		"case":      "keyword.control.hozo",
		"else":      "keyword.control.hozo",
		"throw":     "keyword.control.hozo",
		"string":    "support.type.primitive.hozo",
		"never":     "support.type.primitive.hozo",
		"list":      "support.type.primitive.hozo",
		"nullable":  "support.type.primitive.hozo",
		"single":    "support.type.primitive.hozo",
		"true":      "constant.language.hozo",
		"false":     "constant.language.hozo",
		"none":      "constant.language.hozo",
	}
	for w, s := range wantScope {
		if scopes[w] != s {
			t.Errorf("%q の scope は %s のはずが %s", w, s, scopes[w])
		}
	}
}

// TestContextualNotReserved は文脈キーワードが予約語のパターンに入らないことを確かめる
// （docs/grammar.md「予約語」の二つ目の表。位置の規則でだけ色を付ける）。
func TestContextualNotReserved(t *testing.T) {
	for _, g := range KeywordGroups() {
		for _, w := range g.Words {
			if token.IsContextual(w) {
				t.Errorf("文脈キーワード %q が予約語のパターン %s にある", w, g.Key)
			}
		}
	}
}
