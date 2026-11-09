// Package ir は Hozo の IR（中間表現）の型を持つ。
//
// IR の意味（何をどう正規化するか）は docs/ir.md が持ち主で、このパッケージはその
// 「構造」を Go の型に写したもの。JSON の形の持ち主はこのパッケージで、
// schema/ir.schema.json と sdk/ts の型はここから生成する
// （docs/requirements.md「プラグインの書き方」）。
// プラグインが import してよいのは、このパッケージと plugin パッケージだけ。
//
// # JSON
//
// docs/ir.md「JSON」の通り。フィールド名は lowerCamelCase、判別付きユニオン
// （[Type]、[ParamType]、[Statement]、[Expr]、[Atom]、[ArgValue]、[Literal]、[Decl]、[ArmBody]）
// は kind で分ける。省略可能なフィールドは値がなければキーごと省き、null は出さない
// （例外は docs/ir.md「op」の returns と body で、null が void と葉を表す）。
// 空の列は [] で出す。Go の nil の列と空の列は区別せず、どちらも [] になり、
// [] を読むと nil になる。duration は整数のナノ秒。
//
// すべての構造は encoding/json でそのまま使える（json.Marshal / json.Unmarshal）。
// ユニオンを単独で読むには [UnmarshalType] などを使う。
package ir
