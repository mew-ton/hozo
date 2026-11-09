// Package schema は IR の JSON スキーマ ir.schema.json を置くディレクトリ（docs/packages.md「ディレクトリ」）。
//
// ir.schema.json は Go の ir / plugin パッケージから internal/tools/irschema が生成する
// （docs/packages.md「生成するもの」）。手で編集せず、`go generate ./schema/...` で作り直す。
// CI は生成し直して差分がないことを確かめる（docs/testing.md「生成物の一致」）。
package schema

//go:generate go run ../internal/tools/irschema -o ir.schema.json
