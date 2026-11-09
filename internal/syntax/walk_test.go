package syntax

import (
	"reflect"

	"github.com/mew-ton/hozo/internal/syntax/ast"
)

// walk は構文木を深さ優先でたどり、各節について fn(節, 親) を呼ぶ。テスト用。
// reflect で *struct・スライス・interface のフィールドをたどる。DocComment は親の Span の外に置かれるので親なしで呼ぶ。
func walk(root ast.Node, fn func(n ast.Node, parent ast.Node)) {
	var visit func(v reflect.Value, parent ast.Node)
	visit = func(v reflect.Value, parent ast.Node) {
		switch v.Kind() {
		case reflect.Interface:
			if !v.IsNil() {
				visit(v.Elem(), parent)
			}
		case reflect.Ptr:
			if v.IsNil() {
				return
			}
			node, isNode := v.Interface().(ast.Node)
			if isNode {
				if _, isDoc := node.(*ast.DocComment); isDoc {
					fn(node, nil)
				} else {
					fn(node, parent)
				}
				parent = node
			}
			visit(v.Elem(), parent)
		case reflect.Struct:
			if v.CanAddr() {
				if node, ok := v.Addr().Interface().(ast.Node); ok && parent != node {
					// 値として埋め込まれた節（Ident、QualifiedName など）。
					if _, isDoc := node.(*ast.DocComment); !isDoc {
						fn(node, parent)
					}
					parent = node
				}
			}
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).Name == "Span" {
					continue
				}
				visit(v.Field(i), parent)
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				visit(v.Index(i), parent)
			}
		}
	}
	visit(reflect.ValueOf(root), nil)
}
