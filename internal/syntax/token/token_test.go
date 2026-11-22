package token

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestKeywordsMatchGrammar は token.Keywords と docs/grammar.md「予約語」の表が同じ集合であることを確かめる
// （docs/testing.md「予約語の一致」）。
func TestKeywordsMatchGrammar(t *testing.T) {
	reserved, contextual := readKeywordTables(t, "../../../docs/grammar.md")
	got := make([]string, 0, len(Keywords))
	for k := range Keywords {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(reserved)
	if strings.Join(got, " ") != strings.Join(reserved, " ") {
		t.Errorf("Keywords と docs/grammar.md「予約語」が違う\n Keywords: %v\n grammar:  %v", got, reserved)
	}
	c := append([]string(nil), Contextual...)
	sort.Strings(c)
	sort.Strings(contextual)
	if strings.Join(c, " ") != strings.Join(contextual, " ") {
		t.Errorf("Contextual と docs/grammar.md「予約語」の文脈キーワードが違う\n Contextual: %v\n grammar:    %v", c, contextual)
	}
}

// readKeywordTables は「### 予約語」の節の一つ目の表の 2 列目と、二つ目の表の 1 列目の、バッククォートの語を拾う。
func readKeywordTables(t *testing.T, path string) (reserved, contextual []string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("docs/grammar.md を開けない: %v", err)
	}
	defer f.Close()
	code := regexp.MustCompile("`([^`]+)`")
	inSection, table := false, 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "### ") {
			if inSection {
				break
			}
			inSection = line == "### 予約語"
			continue
		}
		if !inSection {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			if table == 1 || table == 3 {
				table++ // 表が終わった
			}
			continue
		}
		if table == 0 || table == 2 {
			table++ // 表が始まった（見出しの行）
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if strings.HasPrefix(strings.TrimSpace(cells[0]), "---") {
			continue
		}
		switch table {
		case 1:
			for _, m := range code.FindAllStringSubmatch(cells[1], -1) {
				reserved = append(reserved, m[1])
			}
		case 3:
			for _, m := range code.FindAllStringSubmatch(cells[0], -1) {
				contextual = append(contextual, m[1])
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(reserved) == 0 || len(contextual) == 0 {
		t.Fatalf("docs/grammar.md「予約語」の表が読めない: reserved=%d contextual=%d", len(reserved), len(contextual))
	}
	return reserved, contextual
}

func TestKindString(t *testing.T) {
	tests := []struct {
		k    Kind
		want string
	}{
		{Namespace, "namespace"},
		{True, "true"},
		{Semicolon, ";"},
		{StarStar, "**"},
		{Arrow, "=>"},
		{ThinArrow, "->"},
		{Catch, "catch"},
		{Ident, "識別子"},
		{EOF, "ファイルの終わり"},
		{Kind(9999), "不明な字句"},
	}
	for _, tt := range tests {
		if got := tt.k.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.k, got, tt.want)
		}
	}
}

func TestKindPredicates(t *testing.T) {
	if !Model.IsKeyword() || Ident.IsKeyword() || Semicolon.IsKeyword() {
		t.Error("IsKeyword")
	}
	for _, k := range []Kind{String, Int, Float, Duration, True, False} {
		if !k.IsLiteral() {
			t.Errorf("%s.IsLiteral() = false", k)
		}
	}
	if Ident.IsLiteral() || None.IsLiteral() {
		t.Error("IsLiteral")
	}
	if !IsContextual("post") || IsContextual("op") {
		t.Error("IsContextual")
	}
}

// TestTypeNames は、核の型と形の名前（docs/grammar.md「予約語」の 13 語）が、予約語でも、文脈キーワードでもなく、
// 文書の段落に並ぶ語と、同じ集合であることを確かめる。
func TestTypeNames(t *testing.T) {
	names := TypeNames()
	if len(names) != 13 {
		t.Fatalf("TypeNames は 13 語のはずが %d: %v", len(names), names)
	}
	for _, w := range names {
		if _, reserved := Keywords[w]; reserved || IsContextual(w) {
			t.Errorf("%q は、予約語でも文脈キーワードでもないはず", w)
		}
		if !IsTypeName(w) {
			t.Errorf("IsTypeName(%q) = false", w)
		}
	}
	if IsTypeName("fn") || IsTypeName("Time") || !IsCoreTypeName("time") || IsCoreTypeName("list") || !IsShapeName("single") || IsShapeName("never") {
		t.Error("IsTypeName / IsCoreTypeName / IsShapeName")
	}
	data, err := os.ReadFile("../../../docs/grammar.md")
	if err != nil {
		t.Fatal(err)
	}
	want := "`" + strings.Join(names, "` `") + "` の 13 語は"
	if !strings.Contains(string(data), want) {
		t.Errorf("docs/grammar.md「予約語」に、TypeNames の並びがない: %s", want)
	}
}
