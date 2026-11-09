// Package syntax は .hozo の字句解析・構文解析と構文木を持つ。
// 文法は docs/grammar.md が持ち主。
// パーサーは構文の誤りに耐え、すべての節は元の位置（ファイル、バイトの範囲）を持つ
// （docs/requirements.md「言語サーバー」の前提条件）。
package syntax
