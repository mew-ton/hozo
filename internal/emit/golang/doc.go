// Package golang は Go の既定の出力（docs/emit-go.md）。
//
// 入力は docs/ir.md「その名前空間から見た IR」の [ir.View]（見る側 viewer が出力する名前空間）で、
// 出力は View.Config.Dir（Dir(viewer)。docs/config.md「名前空間の設定の解決」）の下に置くファイルの列。何をどの形で出すかの持ち主は docs/emit-go.md で、
// 名前と型の規則は plugin/golang（docs/packages.md「`plugin/golang`（公開）」）と共有する。
//
// # 文書との対応
//
//   - 「配置」: [Emit] の冒頭（emit.go）と layout.go。
//   - 「名前」: plugin/golang と names.go（GOEMIT-2 の検出）。
//   - 「型」: plugin/golang の GoType。
//   - 「model」「scalar」「union」「exception」「effect」: types_gen.go（types.go を作る）。
//   - 「scalar」の Validate と「model の Validate」: validate_gen.go（validate.go を作る）。
//   - 「state」「op」（インターフェース、デコレータ）: ops_gen.go（ops.go を作る）。
//   - 「配線の本体」「match」: wiring_gen.go（wiring.go を作る）。
//   - 「classify」: classify_gen.go（classify.go を作る）。
//   - 別の名前空間の型の複製（「配置」の三つ目）: copy.go。
//
// # 文書にない決定
//
// 文書が決めていないところで、この実装が選んだこと。
//
//   - 別の名前空間 M が「Go で同じプロジェクト」かは View.Namespaces[M] の language で決め、
//     M のパッケージのディレクトリは View.Namespaces[M].Dir（Dir(M)）。View.Namespaces にない
//     名前空間（語彙など）の型は複製する。
//   - View.Config.Dir（または View.Namespaces[M].Dir）が空なら Conf.out を使う（P が空の Dir(N) と同じ）。
//     internal/check の Views は常に Dir を埋めるので、手で組んだ View のための保険。
//   - 複製する宣言は、参照した model / scalar / union と、それらが参照する宣言（推移的）。
//     複製した model / scalar には Validate（と既定値があれば New）も出す。derivedFrom の
//     変換関数は複製しない。
//   - GOEMIT-2 は、フィールド名と op 名に加えて、パッケージの型名（宣言・合成・複製・
//     ユニオンの包む型）の衝突でも出す。衝突した出力はコンパイルできないため。
//   - GOEMIT-* か内部エラーが一つでもあればファイルは返さない（CheckFailed では書き出さない）。
//   - ユーザーが valid に足した制約のデコレータに引数があれば、ConstraintImpls のメソッドに
//     v の後ろにその引数を並べる。
//   - 診断の位置は IR の Span（行・列）しかないので、[diag.Diagnostic.File] にファイルを入れ、
//     メッセージの末尾に「（file:line:col）」を付ける。
//   - `<Ns>Ops` には、その名前空間の op のうち配線が呼ぶものだけを出す。
//   - Classifiers は名前空間に classify が一つでもあれば出し、New の引数にも取る（match で使うかは見ない）。
//   - 包む関数のある配線の op は、本体を非公開の `<op>Body` にし、ops.go の公開メソッドが包む。
//   - 後ろで使わない束縛（WIRE-8 は warning）には `_ = x` を出す（Go のコンパイルエラーを避ける）。
//   - optional の位置に値を渡すとき `&x` が書けない式には、types.go に出すヘルパー `ptr` を使う。
//   - 既定値の GOEMIT-4 は、scalar の標準の制約に加えてフィールド自身の valid.* も見る。
//   - min / max を付けられる「数値」は int と float（Duration は含めない）。
//
// まだ実装していないところ（docs/decisions.md「設計書を変える決定」S1〜S6 の出力の側）。
//
//   - 状態の分類、遷移の効果、状態での match（match by C、match s by list / by nullable）は、
//     unsupported.go の checkUnsupported が GOEMIT-INTERNAL の error にして、ファイルを出さない。
//   - StateCases（docs/emit-go.md「state」）は出さない。New の引数にも ops の構造体にもない。
//   - States の取得メソッドの型は対応済み: nullable<T> なら *T、single<T> なら T。
package golang
