// Package testmap は、テストの対応表（docs/plugin-protocol.md「テストの対応表」）の形の確かめと、カバレッジの
// 「実行された行」との突き合わせ（spec/cli.hozo の confirmTests の手順の 2 と 5）。
//
// 言語も、カバレッジの形も、知らない: 対応表は、トランスパイラが emit の応答に足したファイルで、実行された行は、基底の
// プラグインが direction "coverage" で返したもの。診断は作らない（internal/app が、結果から作る）。
package testmap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	sdk "github.com/mew-ton/hozo/plugin"
)

// IsMap は、パス path のファイルの名前が、対応表のもの（hozo-tests.<…>.json）か。
func IsMap(path string) bool {
	_, ok := sdk.TestMapTranspiler(path)
	return ok
}

// Parse は、emit の応答のファイル file を、トランスパイラ transpiler の対応表として読む。files は、同じ応答の files の
// path の集まり。約束の形（docs/plugin-protocol.md「テストの対応表」）でなければ error（PLG-4 の文面。ID は付けない）:
// 名前のトランスパイラが違う、JSON のオブジェクトでない、tests が配列でない、要素に name / file / lines のどれかがない、
// 型が違う、lines が 1 以上の整数二つ（始まり ≦ 終わり）でない、name が空か重なる、file が files にない。
// 知らない項目は、読まない。
func Parse(file sdk.File, transpiler string, files map[string]bool) (*sdk.TestMap, error) {
	if name, _ := sdk.TestMapTranspiler(file.Path); name != transpiler {
		return nil, fmt.Errorf("対応表の名前のトランスパイラ %q が、起動したトランスパイラ %q でない", name, transpiler)
	}
	var top map[string]json.RawMessage
	if isNull([]byte(file.Content)) || json.Unmarshal([]byte(file.Content), &top) != nil {
		return nil, errors.New("対応表が JSON のオブジェクトでない")
	}
	raw, ok := top["tests"]
	if !ok || isNull(raw) {
		return nil, errors.New("tests がない")
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil, errors.New("tests が配列でない")
	}
	out := &sdk.TestMap{Tests: make([]sdk.TestEntry, 0, len(items))}
	seen := map[string]bool{}
	for i, item := range items {
		where := fmt.Sprintf("tests[%d]", i)
		var el map[string]json.RawMessage
		if isNull(item) || json.Unmarshal(item, &el) != nil {
			return nil, fmt.Errorf("%s がオブジェクトでない", where)
		}
		var entry sdk.TestEntry
		for _, f := range []struct {
			key string
			dst *string
		}{{"name", &entry.Name}, {"file", &entry.File}} {
			raw, ok := el[f.key]
			if !ok || isNull(raw) {
				return nil, fmt.Errorf("%s に %s がない", where, f.key)
			}
			if json.Unmarshal(raw, f.dst) != nil {
				return nil, fmt.Errorf("%s の %s が文字列でない", where, f.key)
			}
		}
		raw, ok := el["lines"]
		if !ok || isNull(raw) {
			return nil, fmt.Errorf("%s に lines がない", where)
		}
		lines, ok := lineRange(raw)
		if !ok {
			return nil, fmt.Errorf("%s の lines が、1 以上の整数二つ（始まり ≦ 終わり）の配列でない", where)
		}
		entry.Lines = lines
		if entry.Name == "" {
			return nil, fmt.Errorf("%s の name が空", where)
		}
		if seen[entry.Name] {
			return nil, fmt.Errorf("name %q が二度ある", entry.Name)
		}
		seen[entry.Name] = true
		if !files[entry.File] {
			return nil, fmt.Errorf("%s の file %q が、同じ応答の files にない", where, entry.File)
		}
		out.Tests = append(out.Tests, entry)
	}
	return out, nil
}

func isNull(raw []byte) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// lineRange は raw を、1 以上の整数二つ（始まり ≦ 終わり）の配列として読む。文字列に書いた数（"1"）は、整数ではない。
func lineRange(raw json.RawMessage) ([2]int, bool) {
	var pair []json.RawMessage
	if json.Unmarshal(raw, &pair) != nil || len(pair) != 2 {
		return [2]int{}, false
	}
	start, err1 := strconv.ParseInt(string(bytes.TrimSpace(pair[0])), 10, 32)
	end, err2 := strconv.ParseInt(string(bytes.TrimSpace(pair[1])), 10, 32)
	if err1 != nil || err2 != nil || start < 1 || start > end {
		return [2]int{}, false
	}
	return [2]int{int(start), int(end)}, true
}

// Test は、求めるテスト一つ: どのトランスパイラの対応表の、どのテストか。
type Test struct {
	Transpiler string
	sdk.TestEntry
}

// Label は、テストを名指す文字列 `<トランスパイラの名前>: <テストの名前>`（spec/cli.hozo の confirmTests）。
func (t Test) Label() string { return t.Transpiler + ": " + t.Name }

// Status は、求めるテスト一つの、突き合わせの結果。
type Status int

const (
	// Ran は、走った: そのファイルの実行された行の範囲のどれかが、テストの行の範囲と、一行でも重なる。
	Ran Status = iota
	// NotRun は、カバレッジがそのファイルを測っているが、テストの行の範囲と重なる、実行された行がない。
	NotRun
	// Unmeasured は、カバレッジが、そのファイルを測っていない（実行された行の一覧に、そのファイルがない）。
	Unmeasured
)

// Match は、求めるテスト tests を、実行された行 covered と突き合わせる（spec/cli.hozo の confirmTests の手順の 5）。戻り値は tests と
// 同じ長さ、同じ順。covered に、どのテストのものでもないファイルがあっても、何も言わない。同じ path が二度あれば、
// 行の範囲を合わせて見る。
func Match(tests []Test, covered []sdk.CoveredFile) []Status {
	lines := map[string][][2]int{}
	measured := map[string]bool{}
	for _, f := range covered {
		measured[f.Path] = true
		lines[f.Path] = append(lines[f.Path], f.Lines...)
	}
	out := make([]Status, len(tests))
	for i, t := range tests {
		switch {
		case !measured[t.File]:
			out[i] = Unmeasured
		case overlaps(lines[t.File], t.Lines):
			out[i] = Ran
		default:
			out[i] = NotRun
		}
	}
	return out
}

// overlaps は、ranges のどれかが、範囲 r と、一行でも重なるか。
func overlaps(ranges [][2]int, r [2]int) bool {
	for _, x := range ranges {
		if x[0] <= r[1] && r[0] <= x[1] {
			return true
		}
	}
	return false
}
