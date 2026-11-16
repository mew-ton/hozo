// Package sema は名前の解決（docs/resolve.md）、型（docs/types.md）、IR への正規化（docs/ir.md「正規化」）を行う
// （docs/packages.md「internal/sema」）。
//
// 入口は [Analyze]。構文木（internal/syntax の ast.File）の列から ir.Program を作り、解決した参照の列（[Ref]）と
// 診断を返す。解決と型の誤りがあっても Program は作る: 作れなかった宣言は落とし、解決できなかった参照は IR から省く
// （docs/packages.md「internal/sema」）。
//
// # 手順
//
//  1. 名前空間の表を作る（table.go）。同じ FQN の名前空間を一つにし（NS-2）、重複を RES-6 / NS-3 にする。
//  2. 宣言を IR に下ろす（decls.go、types.go）。scalar の継承、ユニオンの展開、`is X omit` は別の宣言の中身を要するので、
//     必要になったときに下ろす（lower）。循環はそこで検出する。状態の形（STATE-1 / STATE-3）、classify の対象
//     （型か状態か。CLS-3）、効果の宛先（組み込みの効果か遷移か。STATE-4、RES-5）と Effect.writes もここ
//     （docs/check.md「状態の形」。どれも Signature を要しない）。
//  3. デコレータの適用を下ろす（decorators.go）。DEC-1 / DEC-7 / DEC-11 / DEC-12 / DEC-13。op に付けた適用は、
//     そのデコレータの reads / effects の節が運ぶものを Op.carried に集める（docs/check.md「デコレータが運ぶ reads と
//     effects」。付く先は葉でも配線でもよく、運ばれたものの READS-1 / STATE-2 を出す）。
//  4. 配線の本体を下ろし、式の型を決める（wiring.go、compat.go）。WIRE-* / MATCH-* / TYPE-*。match の三つの形
//     （値、基底の状態の組み込みの分類、状態の分類。MATCH-7）を見分け、絞り込み（MATCH-4 / MATCH-6）は値の match にだけ掛ける。
//
// docs/check.md「計算」が埋める Op.Signature と Call.Bound は零値のまま残す。Signature を要する規則
// （THROWS-1、EFF-2、REL-2/4、DEC-4/5/6/14/16、WIRE-6/7、BIND-*、DOC-1 の @throws の所属、CFG-*）は internal/check が行う。
// STATE-2 は、op にデコレータが運ぶ効果の分をここで、デコレータの halts の効果の分を
// internal/check が出す。
package sema
