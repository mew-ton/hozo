// Package plugin は Go で書くプラグインのための SDK。
//
// プラグインは標準入出力で JSON をやり取りする実行ファイル（設計書 §4.6）。
// このパッケージは、その JSON の読み書きと、展開（emit）・取り込み（import）の要求と応答の型を持つ。
package plugin
