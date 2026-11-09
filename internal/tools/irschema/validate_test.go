package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

// validator は、このコマンドが出す JSON Schema の部分集合を検査する（テスト用。標準ライブラリだけ）。
// 対応する語: $ref（#/$defs/<名前> と #）、type（文字列か列。null / integer を含む）、const、enum、
// properties、required、additionalProperties（false かスキーマ）、items、oneOf（ちょうど一つが合う）。
// 知らない語があれば誤りにして、生成の側が語を足したときに黙って通らないようにする。
type validator struct {
	root map[string]any
}

var knownKeywords = map[string]bool{
	"$schema": true, "$id": true, "title": true, "description": true, "$defs": true,
	"$ref": true, "type": true, "const": true, "enum": true, "properties": true, "required": true,
	"additionalProperties": true, "items": true, "oneOf": true,
}

// decode は JSON を any に読む。数値は json.Number のまま（integer の判定のため）。
func decode(data []byte) (any, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// validate は inst がスキーマ schema に合うか。合わなければ最初の誤りを返す。
func (v *validator) validate(schema any, inst any, path string) error {
	s, ok := schema.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: schema is not an object", path)
	}
	for k := range s {
		if !knownKeywords[k] {
			return fmt.Errorf("%s: schema uses unknown keyword %q", path, k)
		}
	}
	if r, ok := s["$ref"].(string); ok {
		target, err := v.resolve(r)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := v.validate(target, inst, path); err != nil {
			return err
		}
	}
	if t, ok := s["type"]; ok {
		if err := checkType(t, inst, path); err != nil {
			return err
		}
	}
	if c, ok := s["const"]; ok {
		if !jsonEqual(c, inst) {
			return fmt.Errorf("%s: want const %v, got %v", path, show(c), show(inst))
		}
	}
	if e, ok := s["enum"]; ok {
		found := false
		for _, cand := range toList(e) {
			if jsonEqual(cand, inst) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: %v is not one of %v", path, show(inst), show(e))
		}
	}
	if obj, isObj := inst.(map[string]any); isObj {
		props, _ := s["properties"].(map[string]any)
		for _, r := range toList(s["required"]) {
			if _, ok := obj[r.(string)]; !ok {
				return fmt.Errorf("%s: missing required property %q", path, r)
			}
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sub := path + "." + k
			if ps, ok := props[k]; ok {
				if err := v.validate(ps, obj[k], sub); err != nil {
					return err
				}
				continue
			}
			switch ap := s["additionalProperties"].(type) {
			case bool:
				if !ap {
					return fmt.Errorf("%s: additional property %q is not allowed", path, k)
				}
			case map[string]any:
				if err := v.validate(ap, obj[k], sub); err != nil {
					return err
				}
			}
		}
	}
	if arr, isArr := inst.([]any); isArr {
		if items, ok := s["items"]; ok {
			for i, it := range arr {
				if err := v.validate(items, it, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	}
	if alts, ok := s["oneOf"]; ok {
		matched := 0
		var errs []string
		for i, alt := range toList(alts) {
			if err := v.validate(alt, inst, fmt.Sprintf("%s<oneOf %d>", path, i)); err == nil {
				matched++
			} else {
				errs = append(errs, err.Error())
			}
		}
		if matched != 1 {
			return fmt.Errorf("%s: %d of %d oneOf alternatives match (%s)", path, matched, len(toList(alts)), strings.Join(errs, "; "))
		}
	}
	return nil
}

func (v *validator) resolve(ref string) (any, error) {
	if ref == "#" {
		return v.root, nil
	}
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, fmt.Errorf("unsupported $ref %q", ref)
	}
	defs, _ := v.root["$defs"].(map[string]any)
	d, ok := defs[strings.TrimPrefix(ref, prefix)]
	if !ok {
		return nil, fmt.Errorf("unknown $ref %q", ref)
	}
	return d, nil
}

func checkType(t any, inst any, path string) error {
	var names []string
	switch tt := t.(type) {
	case string:
		names = []string{tt}
	default:
		for _, n := range toList(t) {
			names = append(names, n.(string))
		}
	}
	for _, n := range names {
		if hasType(n, inst) {
			return nil
		}
	}
	return fmt.Errorf("%s: want type %v, got %s", path, names, typeName(inst))
}

func hasType(name string, inst any) bool {
	switch name {
	case "null":
		return inst == nil
	case "boolean":
		_, ok := inst.(bool)
		return ok
	case "string":
		_, ok := inst.(string)
		return ok
	case "object":
		_, ok := inst.(map[string]any)
		return ok
	case "array":
		_, ok := inst.([]any)
		return ok
	case "number":
		_, ok := inst.(json.Number)
		return ok
	case "integer":
		n, ok := inst.(json.Number)
		if !ok {
			return false
		}
		if _, err := n.Int64(); err == nil {
			return true
		}
		f, err := n.Float64()
		return err == nil && f == math.Trunc(f) && !math.IsInf(f, 0)
	}
	return false
}

func typeName(inst any) string {
	switch inst.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	}
	return reflect.TypeOf(inst).String()
}

// jsonEqual は JSON の値としての等しさ（スキーマの定数は文字列なので、数値の精度は問わない）。
func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func toList(v any) []any {
	switch l := v.(type) {
	case []any:
		return l
	case []string:
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out
	}
	return nil
}

func show(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
