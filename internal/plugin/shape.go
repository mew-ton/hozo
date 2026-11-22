package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	sdk "github.com/mew-ton/hozo/plugin"
)

// 応答の形の検査（docs/plugin-protocol.md「要求と応答」の「応答の形」）。終了コード 0 の標準出力が JSON として読めても、
// 形が違えば hozoc.MalformedOutput（同「起動」の表）。確かめるのは次のことだけ:
//
//   - 応答は JSON のオブジェクト一つ。
//   - 必須の項目がある（null は「ない」と同じ）: emit の outputs、その要素の results、その要素（トランスパイラ一つの答え）の
//     files と warnings、read の errors と warnings、coverage の recognized と files（要素の path と lines）、
//     要素では files の path と content、warnings の message、errors の code と message。
//   - 書かれた項目の型が合う（配列、オブジェクト、文字列）。
//   - 省ける項目（errors、at、errors の severity）は、なくても null でもよい。
//   - 知らない項目は、どの階層でも無視する（版 0.3 までの select を書いたままの応答も、読まれないだけで通る）。
//
// emit の応答の数が、要求と合うか（outputs と、その results）、path が Dir(U, N) の下か（PLG-1）、重複（PLG-2）は、形が
// 合った後に呼ぶ側が確かめる。

// checkShape は終了コード 0 の標準出力 data が、resp の型の応答の形をしているかを確かめる。
// resp が *sdk.EmitBatchResponse なら emit、*sdk.CoverageResponse なら coverage、*sdk.ReadResponse なら read の応答。ほかの型は確かめない
// （import の応答の形は docs/plugin-protocol.md「import」が段階 6 で決める）。
func checkShape(data []byte, resp any) error {
	switch resp.(type) {
	case *sdk.EmitBatchResponse:
		return emitShape(data)
	case *sdk.CoverageResponse:
		return coverageShape(data)
	case *sdk.ReadResponse:
		return readShape(data)
	}
	return nil
}

// readShape は read の応答（docs/plugin-protocol.md「read」）の形: errors と warnings は、どちらも必須。要素は、emit のものと同じ。
func readShape(data []byte) error {
	top, err := shapeObject(data, "")
	if err != nil {
		return err
	}
	// errors: 必須。要素は { code, at?, message }。severity は見ない（常に error）
	if err := eachElement(top, "errors", true, func(el map[string]json.RawMessage, where string) error {
		return firstErr(requireString(el, "code", where), optionalString(el, "at", where), requireString(el, "message", where))
	}); err != nil {
		return err
	}
	// warnings: 必須。要素は { at?, message }
	return eachElement(top, "warnings", true, func(el map[string]json.RawMessage, where string) error {
		return firstErr(optionalString(el, "at", where), requireString(el, "message", where))
	})
}

// coverageShape は coverage の応答（docs/plugin-protocol.md「coverage」）の形。
func coverageShape(data []byte) error {
	top, err := shapeObject(data, "")
	if err != nil {
		return err
	}
	// recognized: 必須。真偽値
	raw, ok := present(top, "recognized")
	if !ok {
		return errors.New("recognized がない")
	}
	var recognized bool
	if json.Unmarshal(raw, &recognized) != nil {
		return errors.New("recognized が真偽値でない")
	}
	// files: 必須。要素は { path, lines }。lines は、1 以上の整数二つ（始まり ≦ 終わり）の配列の配列
	return eachElement(top, "files", true, func(el map[string]json.RawMessage, where string) error {
		if err := requireString(el, "path", where); err != nil {
			return err
		}
		raw, ok := present(el, "lines")
		if !ok {
			return fmt.Errorf("%s に lines がない（空でも [] で出す）", where)
		}
		var ranges []json.RawMessage
		if json.Unmarshal(raw, &ranges) != nil {
			return fmt.Errorf("%s の lines が配列でない", where)
		}
		for i, r := range ranges {
			if !isLineRange(r) {
				return fmt.Errorf("%s の lines[%d] が、1 以上の整数二つ（始まり ≦ 終わり）の配列でない", where, i)
			}
		}
		return nil
	})
}

// isLineRange は raw が、1 以上の整数二つ（始まり ≦ 終わり）の JSON の配列か。文字列に書いた数（"1"）は、整数ではない。
func isLineRange(raw json.RawMessage) bool {
	var pair []json.RawMessage
	if isNull(raw) || json.Unmarshal(raw, &pair) != nil || len(pair) != 2 {
		return false
	}
	start, err1 := strconv.ParseInt(string(bytes.TrimSpace(pair[0])), 10, 32)
	end, err2 := strconv.ParseInt(string(bytes.TrimSpace(pair[1])), 10, 32)
	return err1 == nil && err2 == nil && start >= 1 && start <= end
}

// emitShape は emit の応答（docs/plugin-protocol.md「emit」）の形: outputs の要素ごとに results、その要素が、トランスパイラ
// 一つの答え。
func emitShape(data []byte) error {
	top, err := shapeObject(data, "")
	if err != nil {
		return err
	}
	return eachElement(top, "outputs", true, func(output map[string]json.RawMessage, where string) error {
		return eachElementAt(output, "results", where+".results", true, resultShape)
	})
}

// resultShape は、トランスパイラ一つの答え（emit の応答の results の要素）の形。
func resultShape(top map[string]json.RawMessage, at string) error {
	in := func(key string) string { return at + "." + key }
	// files: 必須。要素は { path, content }
	if err := eachElementAt(top, "files", in("files"), true, func(el map[string]json.RawMessage, where string) error {
		return firstErr(requireString(el, "path", where), requireString(el, "content", where))
	}); err != nil {
		return err
	}
	// warnings: 必須。要素は { at?, message }
	if err := eachElementAt(top, "warnings", in("warnings"), true, func(el map[string]json.RawMessage, where string) error {
		return firstErr(optionalString(el, "at", where), requireString(el, "message", where))
	}); err != nil {
		return err
	}
	// errors: 省ける。要素は { code, at?, message }。severity は見ない（常に error）
	return eachElementAt(top, "errors", in("errors"), false, func(el map[string]json.RawMessage, where string) error {
		return firstErr(requireString(el, "code", where), optionalString(el, "at", where), requireString(el, "message", where))
	})
}

// shapeObject は raw を JSON のオブジェクトとして読む。where は誤りのメッセージに出す場所（"" なら応答の全体）。
func shapeObject(raw []byte, where string) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if isNull(raw) || json.Unmarshal(raw, &obj) != nil {
		if where == "" {
			return nil, errors.New("応答がオブジェクトでない")
		}
		return nil, fmt.Errorf("%s がオブジェクトでない", where)
	}
	return obj, nil
}

// present は obj に key があり、null でないか。
func present(obj map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	raw, ok := obj[key]
	if !ok || isNull(raw) {
		return nil, false
	}
	return raw, true
}

func isNull(raw []byte) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// eachElement は obj の key が、オブジェクトの配列であることを確かめ、要素ごとに check を呼ぶ。
// required なら、key がない（または null）のも誤り。
func eachElement(obj map[string]json.RawMessage, key string, required bool, check func(el map[string]json.RawMessage, where string) error) error {
	return eachElementAt(obj, key, key, required, check)
}

func eachElementAt(obj map[string]json.RawMessage, key, where string, required bool, check func(el map[string]json.RawMessage, where string) error) error {
	raw, ok := present(obj, key)
	if !ok {
		if required {
			return fmt.Errorf("%s がない（空でも [] で出す）", where)
		}
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("%s が配列でない", where)
	}
	for i, item := range items {
		elWhere := fmt.Sprintf("%s[%d]", where, i)
		el, err := shapeObject(item, elWhere)
		if err != nil {
			return err
		}
		if err := check(el, elWhere); err != nil {
			return err
		}
	}
	return nil
}

// requireString は el の key が文字列であることを確かめる。ない（または null）のも誤り。
func requireString(el map[string]json.RawMessage, key, where string) error {
	raw, ok := present(el, key)
	if !ok {
		return fmt.Errorf("%s に %s がない", where, key)
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return fmt.Errorf("%s の %s が文字列でない", where, key)
	}
	return nil
}

// optionalString は el の key が、あれば文字列であることを確かめる。
func optionalString(el map[string]json.RawMessage, key, where string) error {
	if _, ok := present(el, key); !ok {
		return nil
	}
	return requireString(el, key, where)
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
