// Package golang は Go の既定の出力（docs/emit-go.md）。
//
// 入力は docs/ir.md「その名前空間から見た IR」の [ir.View]（見る側 viewer が出力する名前空間）と、その名前空間に
// 設定されたプラグインが返した選択を合わせたもの（[Options.Select]。docs/plugin-protocol.md「選択」）。
// 出力は View.Config.Dir（Dir(viewer)。docs/config.md「名前空間の設定の解決」）の下に置くファイルの列。
// 出すのは選択から決まる出力の対象だけで、何も選ばれていなければ何も出さない。何をどの形で出すかの持ち主は
// docs/emit-go.md で、名前と型の規則は plugin/golang（docs/packages.md「`plugin/golang`（公開）」）と共有する。
//
// # 文書との対応
//
//   - 「出力の対象」: select.go（op・状態・値の分類・別の名前空間の op・包む関数）と decls.go（型）。
//   - 「配置」: [Emit] の冒頭（emit.go）と layout.go。別の名前空間の型の import と複製は decls.go。
//   - 「名前」: plugin/golang と names.go（GOEMIT-2 の検出）。
//   - 「型」: plugin/golang の GoType。
//   - 「model」「scalar」「union」「exception」「effect」: types_gen.go（types.go を作る）。
//   - 「scalar」の Validate と ConstraintFuncs、「model の Validate」: validate_gen.go（validate.go を作る）。
//   - 「op」（関数の型、束ねる構造体と New、デコレータ）: ops_gen.go（ops.go を作る）。
//   - 「state」（状態の単位、StateSet、States、状態の分類の判定関数）: states_gen.go（states.go を作る）。
//   - 「配線の本体」「match」（値の分類・状態の分類の switch、組み込みの分類の if）: wiring_gen.go（wiring.go を作る）。
//   - 「classify」（列挙の型と定数、値の分類の述語と判定関数）: classify_gen.go（classify.go を作る）。
//
// # 文書にない決定
//
// 文書が決めていないところで、この実装が選んだこと。
//
//   - 別の名前空間 M が「Go で同じプロジェクト」かは View.Namespaces[M] の language で決め、
//     M のパッケージのディレクトリは View.Namespaces[M].Dir（Dir(M)）。
//   - View.Config.Dir（または View.Namespaces[M].Dir）が空なら Conf.out を使う（P が空の Dir(N) と同じ）。
//     internal/check の Views は常に Dir を埋めるので、手で組んだ View のための保険。
//   - viewer の型は、出力の対象が参照する型を、名前空間を問わず参照を辿り切った範囲で出す（別の名前空間の型を通して
//     参照されるものも含む）。別の名前空間の選択によらずに決まるので、[Exports] がそのまま「その名前空間の出力が持つ型」に
//     なる。別の名前空間の型は、[Options.Exported] にあれば import し、なければ、viewer のパッケージが実際に使うものと
//     それらが参照するものを複製する。複製した model / scalar には Validate（と既定値があれば New）も出す。
//     derivedFrom の変換関数は複製しない。
//   - select の stops は起点ではない: stops に書いた op も、起点から辿られなければ出力の対象にならない。
//   - 効果を持つ葉の op のフィールド（状態の単位の命令）が nil のときの *runtime.MissingError は、Kind が "state"
//     （束ねているのが States だから）、Name がその op の FQN。問い合わせなら Name は状態（か状態の分類）の FQN、
//     包む関数なら `<op の FQN>@<デコレータの FQN（via があれば via）>`。
//   - 配線の本体は、関数のフィールド（Deps の op、States の問い合わせ）を呼ぶ直前に、nil なら *runtime.MissingError を
//     返す文を書く。葉・命令・包む関数は Ops のメソッドの中で、値の分類は判定関数の中で確かめる。
//   - Wrappers のフィールド（デコレータごとの構造体）の並びは、op の宣言の順に、書かれた順で最初に現れたデコレータの順。
//     包む関数の最後の引数の名前は next で、op の引数と重なれば末尾に `_`。
//   - GOEMIT-2 は、フィールド名と op 名に加えて、パッケージの型名（宣言・合成・複製・ユニオンの包む型・op の関数の型・
//     包む関数の型・状態の単位）、生成したコードが決まった名前で出す宣言（Leaves、Ops、Deps、Wrappers、Classifiers、
//     StateSet、States、New）、StateSet・Deps・Wrappers のフィールド名の衝突でも出す。衝突した出力はコンパイルできないため。
//     見るのは出力の対象だけ。
//   - GOEMIT-* か内部エラーが一つでもあればファイルは返さない（CheckFailed では書き出さない）。GOEMIT-1 も、
//     出力の対象があるときにだけ出す。
//   - ユーザーが valid に足した制約のデコレータに引数があれば、制約の関数の型の v の後ろにその引数を並べる。
//     model の Validate は、フィールドの Validate が返した *runtime.MissingError を違反にせず、そのまま返す。
//   - 診断の位置は IR の Span（行・列）しかないので、[diag.Diagnostic.File] にファイルを入れ、
//     メッセージの末尾に「（file:line:col）」を付ける。
//   - op の関数の型のコメントには、op の説明と引数の説明も写す（実装する者が読むのはこの型だから）。
//   - 効果の定数は、選ばれた op の Op.effects と（viewer から見た）Signature.effects にある viewer の効果に出す。
//   - 後ろで使わない束縛（WIRE-8 は warning）には `_ = x` を出す（Go のコンパイルエラーを避ける）。
//   - optional の位置に値を渡すとき `&x` が書けない式には、types.go に出すヘルパー `ptr` を使う。
//   - 既定値の GOEMIT-4 は、scalar の標準の制約に加えてフィールド自身の valid.* も見る。
//   - min / max を付けられる「数値」は int と float（Duration は含めない）。
//   - 状態の分類の判定関数は、述語がエラーを返しても（入れていない述語も）残りの述語を呼び、最初のエラーを返す。
//     エラーがあれば真の数は検証しない。
//   - 遷移の効果の定数のコメントには、説明の後ろに `on <宛先の FQN>: <前> -> <後>` の行を必ず足す。
//   - match のアームの呼び出しが bind で状態の値を取るとき、その取得はアームの中に書き、エラーはその場で返す
//     （アームの呼び出しのエラーだけを switch / if の後でまとめて見る）。
//   - match の形と分類が合わない View（MATCH-7 を通っていれば来ない）は GOEMIT-INTERNAL の error にする。
package golang
