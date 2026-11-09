// Package ir は Hozo の IR（中間表現）の型を持つ。
//
// IR の意味（何をどう正規化するか）は docs/ir.md が持ち主で、このパッケージはその形。
// JSON の形の持ち主はこのパッケージで、schema/ir.schema.json と sdk/ts の型はここから生成する
// （docs/requirements.md「プラグインの書き方」）。
// プラグインが import してよいのは、このパッケージと plugin パッケージだけ。
package ir
