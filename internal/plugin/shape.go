package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	sdk "github.com/mew-ton/hozo/plugin"
)

// 応答の形の検査（docs/plugin-protocol.md「要求と応答」の「応答の形」）。終了コード 0 の標準出力が JSON として読めても、
// 形が違えば proc.MalformedOutput（同「起動」の表）。確かめるのは次のことだけ:
//
//   - 応答は JSON のオブジェクト一つ。
//   - 必須の項目がある（null は「ない」と同じ）: emit の files と warnings、check の diagnostics、
//     要素では files の path と content、warnings の message、errors / diagnostics の code と message、
//     check の diagnostics の severity。
//   - 書かれた項目の型が合う（配列、オブジェクト、文字列）。check の severity は "error" か "warning"。
//   - 省ける項目（errors、select、select の四つの列、at、emit の errors の severity）は、なくても null でもよい。
//   - 知らない項目は、どの階層でも無視する。
//
// path が Dir(U, N) の下か（PLG-1）、重複（PLG-2）、select の中身（PLG-3）は、形が合った後に呼ぶ側が確かめる。

// checkShape は終了コード 0 の標準出力 data が、resp の型の応答の形をしているかを確かめる。
// resp が *sdk.EmitResponse なら emit、*sdk.CheckResponse なら check の応答。ほかの型は確かめない
// （import の応答の形は docs/plugin-protocol.md「import」が段階 6 で決める）。
func checkShape(data []byte, resp any) error {
	switch resp.(type) {
	case *sdk.EmitResponse:
		return emitShape(data)
	case *sdk.CheckResponse:
		return checkResponseShape(data)
	}
	return nil
}

// emitShape は emit の応答（docs/plugin-protocol.md「emit」）の形。
func emitShape(data []byte) error {
	top, err := shapeObject(data, "")
	if err != nil {
		return err
	}
	// files: 必須。要素は { path, content }
	if err := eachElement(top, "files", true, func(el map[string]json.RawMessage, where string) error {
		return firstErr(requireString(el, "path", where), requireString(el, "content", where))
	}); err != nil {
		return err
	}
	// warnings: 必須。要素は { at?, message }
	if err := eachElement(top, "warnings", true, func(el map[string]json.RawMessage, where string) error {
		return firstErr(optionalString(el, "at", where), requireString(el, "message", where))
	}); err != nil {
		return err
	}
	// errors: 省ける。要素は { code, at?, message }。severity は見ない（常に error）
	if err := eachElement(top, "errors", false, func(el map[string]json.RawMessage, where string) error {
		return firstErr(requireString(el, "code", where), optionalString(el, "at", where), requireString(el, "message", where))
	}); err != nil {
		return err
	}
	// select: 省ける。四つの列もそれぞれ省ける
	raw, ok := present(top, "select")
	if !ok {
		return nil
	}
	sel, err := shapeObject(raw, "select")
	if err != nil {
		return err
	}
	for _, key := range []string{"roots", "stops", "states"} {
		if err := stringList(sel, key, "select."+key); err != nil {
			return err
		}
	}
	return eachElementAt(sel, "wraps", "select.wraps", false, func(el map[string]json.RawMessage, where string) error {
		return firstErr(requireString(el, "op", where), requireString(el, "decorator", where))
	})
}

// checkResponseShape は check の応答（docs/plugin-protocol.md「check」）の形。
func checkResponseShape(data []byte) error {
	top, err := shapeObject(data, "")
	if err != nil {
		return err
	}
	// diagnostics: 必須。要素は { severity, code, message, at? }
	return eachElement(top, "diagnostics", true, func(el map[string]json.RawMessage, where string) error {
		if err := firstErr(requireString(el, "severity", where), requireString(el, "code", where),
			requireString(el, "message", where), optionalString(el, "at", where)); err != nil {
			return err
		}
		var sev string
		_ = json.Unmarshal(el["severity"], &sev)
		if sev != "error" && sev != "warning" {
			return fmt.Errorf("%s の severity %q は \"error\" でも \"warning\" でもない", where, sev)
		}
		return nil
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

// stringList は obj の key が、あれば文字列の配列であることを確かめる。
func stringList(obj map[string]json.RawMessage, key, where string) error {
	raw, ok := present(obj, key)
	if !ok {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("%s が配列でない", where)
	}
	for i, item := range items {
		var s string
		if isNull(item) || json.Unmarshal(item, &s) != nil {
			return fmt.Errorf("%s[%d] が文字列でない", where, i)
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
