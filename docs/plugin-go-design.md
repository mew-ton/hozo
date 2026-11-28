# go-design プラグイン

## この文書が決めること

トランスパイラ `go-design`（プラグインは `plugins/go-design/`）が受け付けない書き方。`go-design` は、設計の図の語彙（`docs/vocab.md`「設計の図の語彙」）を読んで、テストを受け取る口を出し、Go の基底 `go`（`docs/plugin-go.md`）に相乗りする。

ほかのことの持ち主は、プラグインのファイル（決定 D813）:

- 出す口の形と振る舞い、オプション `requirements` の意味: `plugins/go-design/spec/go-design.hozo` の要求 `hozo.godesign.Requirement`。
- マニフェスト: `plugins/go-design/hozo-plugin.toml`。
- 診断（GODESIGN-*）の ID と条件と重さ: `spec/diagnostics/go-design.hozo`（「診断」）。

## フローの op の決まり

フローの op が、次のどれかに当たれば、error（「診断」）。フローと、操作の op の役の持ち主は `docs/ide-chain.md`「業務フロー図」。どれも、書いたものの整合の検査で、出力を、黙って変えない。

- **`export` が要る**（GODESIGN-1）。走らせる側は、入口の `Ops` のメソッドとして、フローを呼ぶ。
- **配線で、値か、何も返さないもの**（GODESIGN-2）。本体のない op（葉）は、順を持たない。`never` を返す op は、正常に終わる道を持たない。
- **呼べるのは、操作の op だけ**（GODESIGN-3。`docs/ide-chain.md`「業務フロー図」）: 配線の呼び出し（`Call.callee`）の先は、どれも、`@prd.usecase.of` の付いた op。`@prd.usecase.of` のない op（同じ名前空間のものも、別のフローも）、`read`、効果（直に呼ぶ組み込みの効果を含む）は、呼べない。`match` のアームの本体の呼び出しと、`for`、`disjoint` の中の呼び出しも、見る。
- **操作の op とは、別の名前空間**（GODESIGN-4）: フローを持つ名前空間 N に、`@prd.usecase.of` の付いた op があれば、その op に、error。

`@prd.usecase.of` の引数（担い手）は、見ない。引数のない `@prd.usecase.of()` も、操作の op の印として数える。

この節の細部は、決定 D640、D643 が決めた。

## 診断

この見出しの診断の、ID と条件と重さの持ち主は、`spec/diagnostics/go-design.hozo` の分類 `hozo.diagnostics.GoDesign` のケース。

warning（コードは `PLUGIN`）は二つ: フローも、要求もない（`plugins/go-design/spec/go-design.hozo` の `Requirement.Targets`）。どこからも指されていない要求（同 `Requirement.Designs`）。

- error が一つでもあれば、ファイルも warning も返さない。
- 基底のオプション `module` がないとき（同じ単位の基底が GO-8 を返し、何も書き出されない。`docs/plugin-go.md`「診断」）は、ファイルを返さず、GO-8 も重ねて返さない。上の診断は、GODESIGN-7 のほかは、そのときも返す。warning も、返す。
- 生成器の不具合（`View` が欠けている、生成したコードを `go/format` で整形できない）は、診断にせず、異常終了する（`Crashed`。`docs/plugin-protocol.md`「起動」）。

この節の細部は、決定 D661 が決めた。
