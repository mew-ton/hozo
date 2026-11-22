# テスト

## この文書が決めること

テストの置き場、書き方、CI で確かめること。品質の要件の持ち主は `docs/requirements.md`「品質の要件」で、ここはそれを満たす手順。

## 規則ごとのテスト

`tests/rules/<ID>/` に、その規則が通る例 `ok_*.hozo` と落ちる例 `ng_*.hozo` を少なくとも一つずつ置く。対象は `.hozo` と `hozo.toml` だけで出せる診断（`docs/check.md`、`docs/resolve.md`、`docs/types.md`、`docs/config.md` の「診断」）。プラグイン・言語サーバーでしか出ないもの（GO-*、GOVALID-*、GOSQL-*、GOHTTP-*、CLI-*、MAN-*、PLG-*、LSP-1）は、それぞれのパッケージの単体テスト（下）で確かめる。期待を例のそばに置くために、このコメントの形にする。

- 落ちる例は、診断が出る行の末尾に `// ERROR: <ID>` か `// WARN: <ID>` のコメントを書く。同じ行に複数なら `// ERROR: A, B`。
- テストはディレクトリごとに `hozo.toml`（なければ `tests/rules/hozo.toml` を使う）で `Project` を作り、診断の集合を「ファイル、行、重さ、ID」の組で比べる。期待にない診断、出なかった期待はどちらも失敗。
- `hozo.toml` の単位はトランスパイラを要る（`docs/config.md`「単位」）ので、起動しないプラグインの fixture を置く: 既定のもの `tests/rules/plugins/std`（基底 `go` / `go-b` / `ts` と、`go` に相乗りする `go-x`、`go` と `go-x` に相乗りする `go-y`）と、規則のディレクトリの `plugins/`（その規則のためのもの）。どちらも各例のプロジェクトの `plugins/` に写す。既定の `tests/rules/hozo.toml` は、名前空間 `a` をトランスパイラ `go` で出力する単位一つ。規則のテストは `check` だけで、プラグインを起動しない。
- 欠番にした規則（CFG-8、CFG-11、TYPE-13、EFF-3、MATCH-8、DEC-17、DEC-18、STATE-9、THROWS-1、READS-2、EFF-2）のディレクトリは消す。配線の op の節が、照合されないこと（`docs/check.md`「状態に触れるもの」の「節に書く要素」）は、REL-4 の `ok_4`（`throws`）と、STATE-5 の `ok_2`（`reads`、`effects`）が確かめ、上書きは、REL-4 の `ng_4`、`ng_5`、`ok_3` が確かめる。足した規則（CFG-19〜CFG-23、NS-6、STATE-5〜STATE-8、MATCH-9、CLS-8）のディレクトリを足す（CFG-23 は、同じデコレータを `seams` に書いた二つのトランスパイラを、規則のディレクトリの `plugins/` に置く）。
- メッセージの文言は比べない（変えやすくするため）。文言はゴールデン（下）で確かめる。
- 文法で閉じている規則（`—` のもの）は、構文エラーになる例を `ng_*.hozo` に置き、`// ERROR: SYNTAX` で確かめる。
- なくした書き方（`docs/grammar.md` の、新しい書き方をメッセージで示して SYNTAX にするもの: `throws A | B`、`exception X: T`、引数の括弧のない `effect fn e: effects s.post`、`exception X()`、`throw X()`、`model A is B`、フィールドの既定値、分類の `default;`、`condition`、`default =>` のアーム、ケースの並びの `|`、配線の `const`、丸括弧のない `match` の頭、デコレータの宣言の `classify … case`、頭に `fn` のない関数の宣言、`op fn`、節の前に `:` のない関数の型（`fn f() throws E`）、`:` の後ろに何もない関数の型、`read fn` と `effect fn` の `on`、状態の遷移の、宛先の後ろの `:`、デコレータの宣言の、付ける先の並びの `|`、`Record<K, V>`、絞り込んだ型の `C.K<T>`、行の遷移の矢印（`as 前 -> 後`）と `any` とケースの並び）は、`tests/rules/SYNTAX/` に `ng_*.hozo` を一つずつ置く。メッセージの文言は、`internal/syntax` の単体テストが確かめる。

## コーパス

`tests/corpus/<名前>/` に、設計書の例を集めたプロジェクト（`hozo.toml` と `.hozo`）を置く。

- `design/`: 設計書 §5〜§11 の例を名前空間ごとに一つのファイルにまとめたもの。設計書の言葉をそのまま写し、写せなかった箇所と足した宣言は、そのディレクトリの `README.md` に一覧する。
- `view/`: プラグインが受け取る `View`（`docs/ir.md`「その名前空間から見た IR」）を端から端まで確かめるためのサンプル。`docs/ir.md`「正規化」の各行と `docs/check.md`「計算」の各手順を一か所ずつ使う。プラグイン `plugins/view-dump` のトランスパイラ `view-dump`（`requires` を持たない、テストのための基底）が、受け取った `View` を `view.json` として書き出す。`api`、`web`、`page` の三つの単位が、どれもそれを使う（`web` は `api` の型と、語彙 `page` の型を使うので、その二つも同じ基底の出力を持つ。`docs/config.md`「型の置き場」）。`tests/view_test.go` が、`internal/project` の `web` の `View` をゴールデン `testdata/expected/view.web.json` と比べ、要求ごとの期待値を文書の見出しの名前のサブテストで明示し、実物の `hozoc emit` がプラグインに渡した `View` が同じであることを確かめる。見出し → 構文の対応と想定する warning は `README.md`。
- 各コーパスについて、`hozoc check` の text 出力をゴールデン `tests/corpus/<名前>/testdata/expected/check.txt` と比べる。warning も含む（`tests/corpus_emit_test.go` の `TestCorpusCheckGolden`）。
- `emit` のゴールデン: `tests/corpus/<名前>/testdata/expected/emit/` に書き出したファイルの木（`.hozoc-outputs.json` を含む）を置き、実際に一時ディレクトリに書いて比べる。単位のトランスパイラは、リポジトリの `plugins/` の本物（基底 `go`）で、`HOZO_PLUGIN_PATH` として渡す（`view` だけは、コーパスの `plugins/view-dump`）。差分はファイルごとに示す（同 `TestCorpusEmitGolden`）。ゴールデンをすべて `testdata/` の下に置くのは、ゴールデンの `.go` を `go build ./...` / `go vet ./...` がパッケージとして拾わないため（Go は `testdata` を無視する）。一時ディレクトリにはリポジトリの `go.mod` を写し、`ir/` `plugin/` `plugins/` `tests/` をシンボリックリンクで足す（`tests/generated_test.go` と同じ）。プラグインのマニフェストの `go run` がそこから解決でき、生成したコードの import パスも `github.com/mew-ton/hozo/<out>` に決まる。比べた後、書き出した Go のパッケージをその一時ディレクトリで `go vet` に通す（`runtime/` をシンボリックリンクで足す。ゴールデンと一致していても、コンパイルできない出力を見逃さないため）。
- **コーパスのゴールデンは、サンプルの期待する出力（「トランスパイラのテスト」）とは役目が違う。** サンプルは人が書いた仕様で、実装より先にあり、`-update` で書き直さない。コーパスのゴールデンは、実装が書いたものを `-update` で置き、差分をレビューで見る、後戻りの検出のためのもの（サンプルが使わない書き方 — union、述語で決まる分類、状態の分類と遷移、`bind`、値のアーム、直に呼ぶ組み込みの効果、引数を持つ `throw`、`@repeatable` — を広く通す）。
- ゴールデンの更新は `go test ./... -update`。

コーパスごとの、`emit` のゴールデンの扱い（出力をトランスパイラに任せ、選択をなくしたときに決めた。`docs/decisions.md`「決めたこと」81）:

| コーパス | 単位のトランスパイラ | `emit` のゴールデン |
|---|---|---|
| `design/` | `server` と `browser` は `go`。使う型の置き場として、語彙の `http` と `page` にも `go` の単位を足す | 持つ（基底 `go` が出したものに作り直す）。利用者が `valid` に足した制約（§6.5）は、`go-valid` を並べないので出力に現れない |
| `state/` | `go` | 持つ（作り直す）。状態の分類、遷移の効果、状態での `match`。IR の中身は `tests/view_test.go` の `TestStateCorpusIR` が見出しごとに確かめる |
| `units/` | 四つの単位とも `go` | 持つ（作り直す）。出力の単位（`docs/config.md`「単位」「単位の対象」「in」「型の置き場」）のサンプル: 同じ名前空間の二つの単位（どちらも自分の型を持つ）、`in` で絞った単位（`in` の外の op は、呼ばれていれば `Leaves` の口だけ、呼ばれていなければ出ない。型と状態はすべて出る）、子孫を名指しする単位、単位をまたぐ一方向の import（置き場は設定の順で最初の単位）。`tests/units_test.go` の `TestUnitsCorpus` が実物の `hozoc emit` の出力を確かめる |
| `view/` | `view-dump`（上） | 持つ（`view.json` だけ。作り直す） |
| `model/` | `none`（sh で動く、テストのための基底。ファイルを返さない）。出力したい `shop` のほかに、`shop` が使う例外の置き場として、`carrier` と `marks` にも `none` の単位を置く | 持たない。「モデルが先」（`docs/requirements.md`）のサンプルで、`tests/model_test.go` の `TestModelCorpus` が、IR と、前回の出力が消えることを確かめる |
| `select/` | — | **コーパスごと消す。** 出力の対象をトランスパイラが選ぶ仕組み（`select`）のサンプルで、確かめる相手がなくなった。`tests/select_test.go` も消す。別の名前空間の op の口と、印のデコレータは `design/` が通る |

テスト用のプラグイン `tests/plugins/pick`（選択だけを返す代役）は消す。コーパスの `plugins/pick/` も消す。

`tests/parse_test.go` は、コーパス・標準の語彙・プラグインの語彙のすべての `.hozo` が構文エラーなしに解析できることと、`docs/cli.md`「cli.hozo」に埋め込んだ `cli.hozo` と `hozo.toml` のブロックがリポジトリ直下のファイルとバイト単位で同じであること（`TestDocsEmbedRootFiles`）を確かめる。語彙のファイルが文書の写しであることも、同じファイルが確かめる（`TestDocsEmbedVocabulary`）: `vocab/db.hozo` は `docs/vocab.md`「`db` の語彙」の全文とバイト単位で同じ、`vocab/http.hozo` は同「`http` の語彙」のブロックをそのまま含み、`plugins/go-cli/cli.hozo` は `docs/plugin-go-cli.md`「語彙」のブロックの宣言を、同じ順に、過不足なく持つ。

hozoc 自身（リポジトリ直下の `cli.hozo` と `hozo.toml`）はコーパスには入れない。`hozoc check` が error なしで通ることを `tests/selfcheck_test.go` で確かめ、`emit` の結果は「生成物の一致」で確かめる。

## 言語サーバーと check の一致

各コーパス（`tests/corpus/` の下のすべて。足せば自動で入る）について、`internal/project.LoadWith` を一度呼び（プラグインの探索には、コーパスの `emit` と同じく、リポジトリの `plugins/` を渡す）、`hozoc check` の診断（プラグインの check の分を除く: 言語サーバーは、解析では、プラグインの check を起動せず、後から足す。`docs/lsp.md`「プラグインの診断」。足した後の診断が、プラグインの check の分を含めて、`hozoc check` と同じであることは、同じパッケージの `TestPluginDiagnosticsOnOpen` が、にせもののプラグインで確かめる）と `internal/lsp` が `publishDiagnostics` で送る診断（LSP の形から戻したもの）が同じ集合であることを確かめる（`internal/lsp` の `TestCheckAgreement`。位置のエンコーディングは utf-16 と utf-8 の両方）。`LSP-1` だけは言語サーバーにしか出ないので除く。診断のないコーパスでは「どちらも空」を確かめることになるので、設計書のコーパスに warning を一つ足したものでも確かめる。

## 生成物の一致

CI で `go generate ./...` と `make generate`（`cli.hozo` からの自己生成。`docs/packages.md`「自己生成の手順」）を実行し、`git diff --exit-code` で差分がないことを確かめる。対象は `docs/packages.md`「生成するもの」の表。`hozoc emit` の分は `tests/generated_test.go` も確かめる: 一時ディレクトリに `cli.hozo` と `hozo.toml` を写して emit し、コミット済みの `internal/gen/hozoc/**` と `internal/gen/proc/**` とバイト単位で比べる（`go test ./...` だけで遅れが分かる）。`tests/selfcheck_test.go` は、リポジトリ直下での `emit --dry-run` が、コミット済みの生成物とちょうど同じファイルを書く予定にすることを確かめる。

手で写すもの（生成しないもの）の一致もテストで確かめる: `docs/cli.md` のブロックがリポジトリ直下のファイルと同じであること（「コーパス」の `TestDocsEmbedRootFiles`）。

## 文書とコードの一致

文書が持ち主の事実のうち、コードから機械的に確かめられるものはテストにする。

- 規則ごとのテストがそろっていること（`tests/rules_test.go` の `TestRulesCoverDocs`。「規則ごとのテスト」）。
- 診断のコード: Go のコードが出しうる診断のコード（規則の ID と、`SYNTAX` / `IO` / `PLUGIN` / `GO-INTERNAL`。プラグインが `errors` で返すコードを含む）がどれも設計文書の「診断」の表にあり、表の重さ（error / warning。診断にならない ID は「—」）がコードと合っていること（`tests/codes_test.go` の `TestDiagnosticCodesAreDocumented`。コードの側はソースを構文木で読んで集める）。
- 診断のメッセージ: Go のコードの文字列が「設計書」を引いていないこと（`docs/diagnostics.md`「構造」。`tests/codes_test.go` の `TestMessagesDoNotCiteDraft`。コメントは見ない）。
- 依存の向き: `docs/packages.md`「依存の向き」の表が Go のパッケージの実際の import と同じで、箇条書きの規則がテストのファイルの import を含めて守られていること（`tests/deps_test.go`）。
- 予約語（下の「予約語の一致」）。

## 環境からの独立

テストの結果は、実行する人の環境に入っているプラグインに左右されない。プラグインの探索が見るプロジェクトの外の場所（`HOZO_PLUGIN_PATH` とホーム。`docs/plugin-protocol.md`「探索」）は、探索が自分では読まず呼ぶ側が渡すもので、プロジェクトを読むテストはどれも、それを渡さない（`project.Load`、`Home` が空の `app.App`、環境変数を返さない関数を渡した `cli.Main`）か、テストが作った一時ディレクトリか、リポジトリの `plugins/`（コーパス、サンプル、プラグインの単体テストの入力。単位のトランスパイラが、リポジトリの本物のプラグインだから）を渡す。確かめ方は、壊れたマニフェストを置いたディレクトリを `HOZO_PLUGIN_PATH` に、`.hozo/plugins` に壊れたマニフェストを置いたディレクトリを `HOME` に向けて、`go test ./...` がすべて通ること（`HOME` を変えるときは、`GOCACHE` / `GOMODCACHE` / `GOPATH` を元の値で明示する）。渡したときに探すことと、渡さなければプロセスの環境を読まないことは、`internal/plugin`、`internal/project`、`internal/app` のテストが確かめる。

## 予約語の一致

`token.Keywords` と `docs/grammar.md`「予約語」の表が同じ集合であることを、文法の文書を読んで確かめるテスト（表の行を正規表現で拾う）。TextMate の文法は `token.Keywords` から生成するので、生成物の一致で足りる。

## プラグインのプロトコル

`internal/plugin` のテストは、`testdata/echo-plugin/`（テストの初めにビルドする小さなプラグイン）を使い、要求がそのまま応答に写ること、終了コードが 0 以外なら `proc.Crashed`、JSON でない・形が違う（`docs/plugin-protocol.md`「応答の形」: 必須の項目がない、型が違う。知らない項目は通る）なら `proc.MalformedOutput` になることを確かめる。SDK（Go の `plugin`）が、要求ごとに返す終了コードは、表（`plugin/testdata/request_head.json`）を読んで確かめる。`plugins/` のプラグインは、サンプル（「トランスパイラのテスト」）、それぞれの単体テスト（「単体テスト」）、生成物の遅れの検査（「生成物の一致」。`go`、`go-valid`、`go-cli`）で確かめる。

## 単体テスト

- `internal/syntax`: 文法の各非終端記号について、受理する例と拒否する例。回復の後に読めた宣言が残ること。
- `internal/sema`: 解決の手続きの各段（`docs/resolve.md`「手続き」）、呼び出しの読み分けの各行（同「呼び出しの読み分け」）、型の互換の各規則（`docs/types.md`「互換」）、呼び出しの引数の各行（同「呼び出しの引数」）に一つずつ。
- `internal/check`: 計算（Sig の E / F / rel / succ、bind）は、小さな IR を直接組み立てて期待値と比べる。手順 0 の表の各行（op、効果、`read`、`throw` の呼び出し）を含める。設計書 §9 の `browser.logout` の例（`server.deleteSession` が不明、`removeToken` が起きている）を必ず含める。
- `plugin/golang`: 名前、型、置き場（テストのパッケージを含む）、import の名前（当たる名前、基底の非公開の名前 `ops` と `ptr`、`Reserve` した名前、テストのパッケージから見た N の入口）、入口で確かめる引数（`CheckedParams`。確かめない場合ごとに一つ）、op の置き場（配線、`never` を返す op、葉）と形（純粋な op も、`throws` のない op も、`ctx` を取り、`error` を返すこと）、状態の単位のメソッド（形で決まる問い合わせ、配線が直に呼ぶ操作、`read`、効果。効果の宛先の単位の各場合）と形、継ぎ目（継ぎ目を持つ op、`Seams` のメソッドの名前と並び、中身の名前）、定義のパッケージの公開の名前の一覧（`PublicNames`。種類ごとに一つ）、テストのパッケージの、基底の公開の名前の一覧（`PublicTestNames`）（`docs/plugin-go.md` の各表の行に一つずつ）。相乗りが使うのと同じ関数を、基底も使うことを前提に、ここで細かい場合分けを確かめる。
- `plugin/golang` の、テストの対応表とカバレッジ（`docs/plugin-go.md`「テストの対応表」「カバレッジを読む」）: `TestMap` は、段が一つ・二つ・三つの `t.Run`、走らせる関数が二つあるファイル、約束の形でない `t.Run`（名前がリテラルでない、中身がその場の関数でない、本体が空）。`ReadCoverProfile` は、三つの `mode`、頭の行が違うもの、壊れた行、同じ区切りが二度出てくるもの、測られていないファイル。
- `internal/testmap`: 対応表の形の各条件（PLG-4。`docs/plugin-protocol.md`「テストの対応表」）と、突き合わせ（`docs/cli.md`「tests」の 5）: すべて走った、一部が走っていない、口を呼んでいない（測ったが、どの行も実行されていない）、カバレッジが、そのファイルを測っていない、カバレッジに、対応表にないファイルがある、範囲の端の行だけが重なる。
- `plugin/govalid`: `Validate` を持つ型の規則の各行。別の名前空間の型の置き場の単位が `go-valid` を並べているか（`Unlisted`。置き場の選び方の各場合）。
- プラグインの単体テストの入力は、小さなプロジェクト `testdata/<名前>/`（`hozo.toml` と `.hozo`）。プラグインのパッケージは `internal/` を import できない（`docs/packages.md`「依存の向き」）ので、`.hozo` を自分では解析しない: `tests/pluginviews_test.go` の `TestPluginViewFixtures` が、`plugins/` の下（と、サンプルの中のプラグインの下。`tests/samples/README.md`「置き方」）のそれぞれの `testdata/<名前>/` を `internal/project` に解析させ、hozoc がトランスパイラに渡すのと同じ `View` の列（単位の順、その中は名前空間の辞書順）を `testdata/<名前>/views.json` に置く。プラグインのテストは、それを読んで入力にする。仕様か hozoc を変えて `View` が変われば、このテストが落ちる（更新は `go test ./tests -run TestPluginViewFixtures -update`、その後にプラグインのゴールデンを `-update`）。入力の仕様は hozoc の検査を通るものに限る（プラグインが返す診断の条件も、検査を通る仕様で書く）。
- `plugins/go`（`internal/gobase`）: GO-1〜GO-6 の各条件。名前空間のパッケージ名が、基底が決まった名前で使う識別子（`ops`、`ptr`、`o`、`ctx`、`err`）や、途中の値（`v0`、`k0`）と同じとき（入力 `pkgnames`）。`docs/plugin-go.md`「配線の本体」の各表の行（文、値、型をそろえる、状態の参照、`bind` で埋まる位置、`match` の分け方とアームの本体の組）を、小さな `.hozo` と、出力のゴールデン（`testdata/<名前>/`）で一つずつ。生成物は `go/format` を通ること。生成したコードを一時のモジュールで実際に動かす振る舞いのテスト（`testdata/<名前>/behavior_test.go`: 述語で決まる分類の判定（述語の `error` が返ること、どのケースにも当たらない判定の誤りが、純粋な配線からも、`panic` でなく `error` として返ること）、状態を通る配線、`read` と効果と、直に呼ぶ組み込みの効果を呼ぶ配線（呼ぶ先が、宛先の状態の単位のメソッドであること）、引数を持つ `throw` が作る例外の値、内側のフォルダから内部の op を呼べること、入口の `Ops` が `export` の op だけを持つこと）。実装の漏れがコンパイルエラーになること（状態の単位のメソッド — 形で決まる問い合わせと命令、`read`、効果。配線が呼んでいないものを含む — を一つでも持たない型が、生成したコードのモジュールでコンパイルできない）。`docs/plugin-go.md`「op の形」が一通りであること（純粋な葉と、純粋な配線のメソッドが、`ctx` を取り、`error` を返す）は、ゴールデンで。同じ `View` から同じバイト列を返すこと。`docs/plugin-go.md`「継ぎ目」の各行（配線・`never` を返す op・葉の中身、二つの継ぎ目の入れ子と、間の継ぎ目を持たないデコレータ、内部の op、`body` と当たる引数名、単位の継ぎ目にあるが付いていないデコレータ、別の名前空間の op のデコレータ）も、`View.config.seams` を書いた入力とゴールデンで。継ぎ目を通ることと、入れ子の順は、振る舞いのテストで確かめる。`docs/plugin-go.md`「単体テストの口」と「効果の入口の確かめ」は、入力 `unittest`（と、行の遷移を持つ `rowstate`）: 出力のゴールデン。口を満たす型が、すべてのケースで通ることと、テストの名前（`<FQN>/<結果>`）。ケースを一つでも欠いた型（葉、結果、分類のケース）が、走らせる関数に渡すところで、コンパイルできないこと。結果が、ケースのものと違えば、走らせる関数が、そのケースのテストを落とすこと（落ちるはずのテストを、一時のモジュールで走らせ、落ちたテストと文面を見る）。絞り込んだ引数を持つ効果が、引数が、言ったケースになければ、`*runtime.CaseError` を返し、単位のメソッドを呼ばないこと。分けた出力（内部の op を持つ名前空間）でも、同じに働くこと。
- `plugins/go-valid`、`plugins/go-sqlite`、`plugins/go-http`、`plugins/go-cli`: それぞれの文書の「診断」の各条件と、サンプルが使わない場合（`go-valid` の `pattern` / `max` / インデックスの model / 省略可能、`go-sqlite` の六つのしまい方（`docs/plugin-go-sqlite.md`「しまい方」の表の各行）と、省略可能なフィールドの列（しまい方ごと。`NULL` の読み書きと、`NOT NULL` の有無）と書き換え（`@db.update`）・条件のない `@db.list`・二つ以上の引数の条件・`@db.where`（一つと、二つ以上。しまい方ごとのリテラル）・`@db.latest`（`@db.list` と、条件のない `@db.find`。値が同じ行と、値のない行の順）・二つ以上の列の `@db.delete`・形で決まる `Post`・`db` の語彙のデコレータを付けていない `read` と効果（型が、単位を満たさないこと）・戻り値のある op と、入れ子の `@db.transaction`、同じ `db` の上の二つの名前空間をまたぐ `@db.transaction`（`runtime.DBTxKey`）、`go-http` の三つの場所の二つの形（`docs/plugin-go-http.md`「入力の読み方」の表の各行。名前を書いた項目と省いた項目、全体の形）・複数の本文の項目・項目がないときの三つの扱い（`@http.required`、`@http.default`、どちらもない。三つの場所で）・クエリの各型・引数を持つ例外（一つと、二つ以上。省略可能な引数）・`validate = false`、`go-cli` の引数の解釈の各規則と、位置を書いた `@cli.arg`、名前を省いた `@cli.flag` / `@cli.env`、値が与えられなかったときの三つの扱い（`@cli.required`、`@cli.default`、どちらもない）・引数を持つ例外（一つと、二つ以上。省略可能な引数。`value` という名前の引数））を、小さな `.hozo` と出力のゴールデンで。相乗りのテストは、基底を起動せず、`View` だけを入力にする（基底の応答に依らないことの確かめになる）。`go-sqlite` は、ゴールデンとの比較を `View` だけで行い、生成したコードをコンパイルして動かすテスト（一時のモジュールでの `go vet` と、本物の SQLite の上の `testdata/<名前>/behavior_test.go`。ドライバは、サンプル `shop-server` と同じもの）でだけ、基底のプラグインをビルドして、同じ `View` から基底のファイルを出させる（`db.go` は、基底の型と状態の単位がなければコンパイルできない）。`go-valid`、`go-http`、`go-cli` も同じ: 出力とゴールデンの比較は `View` だけで行い、`go vet` と `testdata/<名前>/behavior_test.go`（`go-valid` は違反の一覧、`go-http` は `net/http/httptest` での要求と応答、`go-cli` は生成した `cli.Main` に引数と環境変数を渡したときの終了コードと出力）でだけ、依存するトランスパイラ（基底 `go`。`go-http` と `go-cli` は `go-valid` も）のプラグインをビルドして、同じ `View` からそのファイルを出させる。
- `plugins/go-http` の、インテグレーションテストの口（`docs/plugin-go-http.md`「インテグレーションテストの口」）: テストのパッケージの `http.go` を、ハンドラの `http.go` と同じ入力から、ゴールデンで（入力 `routes`: 結果の数え方の各行、六つのデコレータの、要求の組み立ての各行、値を返さない op、引数のない op、経路を二つ持つ op。入力 `cases`: 結果の名前が当たるとき、`never` を返す op、`Given`・`Request`・`Then` と当たる引数の名前、基底が、テストのパッケージを作らない出力）。生成した口を、一時のモジュールで、同じ入力のハンドラに当てて走らせる振る舞いのテスト（`testdata/routes/httpcases_test.go`: すべてのケースを満たせば通ること、組み立てた要求を、ハンドラが、もとの入力と同じ値に読むこと。`testdata/routes/httpmismatch_test.go`: 応答が、そのケースのものでなければ落ち、`Then` が呼ばれないこと）。ケースを一つ欠いた型が、`RunHTTP` に渡すところでコンパイルできないこと（`TestIncompleteHTTPCasesDoNotCompile`）。型だけを変えた二つの入力（`uniforma`、`uniformb`）のテストのパッケージの `http.go` が、口の宣言の型のほかは、同じ行であること。GOHTTP-20、GOHTTP-21 の各条件。
- `plugins/go-design`（`docs/plugin-go-design.md`）: テストのパッケージの `design.go` を、ゴールデンで（入力 `flows`: 値を返すフローと、返さないフロー、引数のないフロー、説明のないフロー、`Given`・`Then` と当たる引数の名前、`match`・`for`・`disjoint` の中から操作の op を呼ぶフロー、二つの要求の分類、説明のない要求、どこからも指されていない要求、op のほかの宣言（model、状態、効果、`read`）からの `@prd.ref`、要求だけの単位、基底が、テストのパッケージを作る出力と、作らない出力）。生成した口を、一時のモジュールで、基底の出力に当てて走らせる振る舞いのテスト（`testdata/flows/behavior_test.go`: すべてのケースを満たせば通り、テストの名前が、フローの FQN と、要求の名前であること。`Given`、フロー、`Then` の順に呼ぶこと。`testdata/flows/mismatch_test.go`: `Given` かフローが失敗を返せば落ち、`Then` が呼ばれないこと）。フローのケースか、要求の入れ物を、一つ欠いた型が、`RunFlows`、`RunRequirements` に渡すところでコンパイルできないこと（`TestIncompleteDoNotCompile`）。GODESIGN-1〜GODESIGN-7 の各条件と、二つの warning。
- **プラグインの診断が、`hozoc check` で出ること**（`docs/plugin-protocol.md`「check」、`docs/decisions.md`「決めたこと」404）は、三つに分けて確かめる。プラグインごとに、`check` の答えが、`emit` の `errors` と同じこと: それぞれのプラグインの `TestCheckMatchesEmit`（`testdata` のすべての入力の、すべての `View`）。hozoc が、出力ごとに `check` を呼び、同じ診断を一度だけ報告し、仕様に error があれば起動しないこと: `internal/project` の `TestPluginCheck` と `TestPluginCheckPerOutput`（代役のプラグイン）。その二つが、コマンドとして、つながっていること: `tests/plugincheck_test.go`（本物の `go-http` で、通信に出せない union の GOHTTP-5 が、`hozoc check` に、位置と一緒に出て、終了コードが 1。`check` が通る入力は、`emit` も通る）。
- サンプルにない場合の Go のテキスト（空白、補助の名前の細部）は、文書の規則を満たす限り実装が決め、この単体テストのゴールデンが固定する。
- **型に依らない形の確かめ**（`docs/requirements.md`「出力を推論しない」の、op を包む側）: `go-http`、`go-cli` の単体テストは、フィールドの型だけを変えた二つの入力（核の型と scalar、省略可能とそうでないもの、引数を持つ例外と持たない例外、戻り値のある op と省略可能な戻り値の op）から、フィールドごと・op ごと・例外ごとの行が、名前のほかは同じテキストで出ることを確かめる。`go-sqlite` は、`@db.transaction` の継ぎ目のメソッドが、op の引数と戻り値の型のほかは同じテキストで出ること（本体の形は、戻り値の有無の二つだけ）。違いが実行時に出ること（値がなければ 204、読めない文字列の文言）は、`behavior_test.go` で確かめる。
- `internal/plugin`: MAN-* の各条件（`requires` と `seams` の誤りを含む）、応答の形の各条件（「プラグインのプロトコル」。版 0.3 までの `select` を持つ応答が、読まれずに通ること）。PLG-1 / PLG-2 は応答のファイルを集める `internal/app` のテストで確かめる。
- `internal/project`: 単位のトランスパイラの解決（CFG-14〜CFG-16、CFG-19、CFG-20）、`in` の照合（CFG-18。語彙のファイルがいつも合うこと）、型の置き場（CFG-22 の表の各行）、単位の継ぎ目（CFG-23。別の単位なら重なってよいこと、解決できなかったトランスパイラを見ないこと）、`View` の `config`（`index`、`base`、`options`、`seams`）、使うプラグインだけの語彙と `check`、MAN-2 を使うプラグインにだけ出すこと。
- `internal/app`: `emit` の手順（`docs/cli.md`「emit」）。sh で動く小さなプラグインで、同じ名前空間の複数の単位、`in`、子孫を名指しする単位、引数での emit の対象の絞り込み（子孫だけが単位の対象である引数、`UnknownNamespace`。絞っても内容が同じこと、対象でない出力のトランスパイラを起動しないこと）、二つのトランスパイラが同じパスを返す衝突（CFG-5）、`errors` を返したトランスパイラがあれば何も書かないこと、`.hozoc-outputs.json` に書かれた Dir(U, N) の外のパスを消さないこと、空になったディレクトリを消すことを確かめる。配線 `writeOutputs` が呼ぶ三つ（葉の `collectOutputs`、`read` の `outputChanges`、効果 `syncOutputs`。`docs/cli.md`「手で書くもの」）は、一つずつも動かす: 集めるのと決めるのは、生成したファイルに触れず、書き出すのは効果だけで、`dryRun` の予定では何もしない。中断された `ctx` では、`collectOutputs` は集めずに、その `error` を返し、`writeOutputs` は何も書かない。`internal/app` と `internal/project` が、`plugin/golang` も `plugins/` も import しないこと（本体が言語を知らないこと）は、依存の向きのテスト（「文書とコードの一致」）が確かめる。
- `internal/app` の `tests` の手順（`docs/cli.md`「tests」）: sh で動く小さなプラグインで、すべて走った、走っていないテストの名指し（TESTS-1。測っていないファイルの文面を含む）、置き場の出力がない・違う（TESTS-2）、カバレッジのファイルを開けない・基底のプラグインが `[coverage] enabled` でない・`recognized: false`（TESTS-3）、対応表が一つもない、対応表の形の誤り（PLG-4。`emit` でも）、text と json の表示、終了コード。
- `internal/tools/selfgen`: コンパイルできない生成物を置き場に置かないこと（`docs/packages.md`「自己生成の手順」）。
- `plugins/go-cli`: CLI-* の各条件と、引数の解釈の各規則（`docs/plugin-go-cli.md`「引数の解釈」）。
- `internal/lsp`: `hozo.toml` のないワークスペースで LSP-1 が出ること。プラグインの診断（`docs/lsp.md`「プラグインの診断」。`plugincheck_test.go`。プラグインは、`sh` で動く、にせもので、待たせる・進ませるを、テストが決める）: クライアントが、許していなければ（既定。許しとして読めない `initializationOptions` を含む）、開いても、保存しても、外でファイルが変わっても、起動しないこと。動いている間に、許しがなくなれば、プラグインの診断が消え（走っている check の答えも、出ない）、その後は、起動しないこと。与えられれば、走ること（同 「許し」）。以下は、許されているとき: 開いたときと、保存したときに、裏で走り、終わったら、足されて出ること。解析（`initialized`、`didChange`）では、起動しないこと。走っている間も、ホバーと解析が、待たされないこと。古い答えが、出ないこと。中身が変われば消え、戻れば、起動し直さずに、また出ること。error があれば、起動しないこと。ファイルの監視の知らせで、走ること（同じ中身なら、走らないこと）。異常終了で、LSP-2 が出て、動き続けること。終わるときに、走っている check を、待つこと。ほかのテストのサーバーは、リポジトリの `plugins/` の代役（マニフェストと語彙は同じで、起動するのは、診断を返さない `sh`）を見る。

## CI

`.github/workflows/ci.yml`: `go build ./...`、`go vet ./...`、`go test ./...`、生成物の一致。`editors/vscode` は Node 22 で、`npm ci`、ビルド（`npm run build`）、`npm test`。`editors/vscode` のテストのうち、`test/trust.test.js` は、ビルドした `out/trust.js` を読む（`docs/editors.md`「VS Code 拡張」）。

- **実行は、同時に一つだけ。** `main` への push と、どの PR の実行も、一つの組（`concurrency` の `group: ci`）に入れる。新しい実行が始まると、動いている実行と、待っている実行は、止まる（`cancel-in-progress`）。止められた実行は、緑ではない。

## トランスパイラのテスト

トランスパイラ（基底と相乗り。TS 向けのものを含む）のテストの方針。作る段階の持ち主は `docs/requirements.md`「段階」。

- 進め方は**テストが先**。トランスパイラごとに、まず**ブラックボックスのテスト**を書く: 入力は `.hozo` のプロジェクト（`hozo.toml` と `.hozo`）、期待は書き出されるファイルのそのままの内容（バイト単位で同じ）。このテストを、そのトランスパイラの実装を一行も書く前に、すべて書く。
- ブラックボックスのテストは、**サンプル**（下）の、期待する出力との比較。期待する出力は人が書いた仕様で、実装に合わせて書き直さない（食い違ったら、文書とサンプルのどちらが正しいかを決めてから、正しくない方を直す）。
- 細かい条件（語彙の使い方の誤りごとの診断、型や名前の変換の場合分けなど）は、単体テスト（「単体テスト」）で確かめる。ブラックボックスのテストに条件の組み合わせを背負わせない。
- 実装の順は、依存の向き: 公開の SDK（`plugin/golang`）、基底、相乗り。一つ実装するごとに、サンプルの、そのトランスパイラの分（下の表の `transpilers/<名前>`）が通る。

### サンプル

トランスパイラの形についての問いを具体的な組で決めるために、**サンプル**を先に書く: `tests/samples/<名前>/` に置いた、仕様（`hozo.toml` と `.hozo`）と、手で書いた完成したコード（各トランスパイラが出すべきファイルと、人が書く部分）の組。置き方、`sample.json`、テストの使い方の持ち主は `tests/samples/README.md`。

`tests/samples_test.go` の `TestSamples` が、サンプルごとに、仕様と設定が `hozoc check` を通ること（`check`）、期待する出力のファイルの持ち主がそろっていること（`files`）、期待するコードが本当に動くこと（`go`）、`hozoc emit` の出力が期待する出力とバイト単位で同じこと（`transpilers/<名前>` と `transpilers/all`）、求めるテストが走ったかを、カバレッジと突き合わせられること（`tests`）を確かめる。サブテストの中身と、飛ばす条件の持ち主は `tests/samples/README.md`「テスト」。

- `transpilers/<名前>` をトランスパイラごとに分けるのは、失敗がどのトランスパイラのものかが分かり、一つ実装するごとに、その分が通るようにするため。
- これまでの `shared`（期待する出力のうち、本体に組み込んだ出力のファイルが、今の `hozoc emit` の出力と同じ）と、選ぶ宣言を代役に伝える `sample.json` の `select`、まだ読めない書き方のサンプルを保留にする `pending` は、なくす。本体に組み込んだ出力がなくなり、代役が要らなくなったため。
- サンプルは自分の `go.mod` を持つ別のモジュールで、リポジトリのモジュールの `go build ./...` などはその Go を見ない。サンプルだけが使う依存（SQLite のドライバなど）は、サンプルの `go.mod` にだけ置く。
- サンプルは、自分のプラグイン（サンプルの `plugin_dirs` に置く、プロジェクトの中のプラグイン。`docs/config.md`「継ぎ目」）を持てる。期待する出力と、マニフェストと語彙を先に書き、プラグインの実装は、ほかのトランスパイラと同じく、その後に作る。
- **テストは、最後の形で書く。** まだ通せないサブテスト（トランスパイラがまだない、今の hozoc がサンプルの設定やマニフェストを読めない、設計文書の版が今の hozoc より先にある）は、足りないものを名指しして飛ばし（設定かマニフェストを読めないだけなら、`check` は、設定を使わない言語の検査に落として通す）、足りないものがそろえば、何も変えずに走る。飛ばす条件の持ち主は `tests/samples/README.md`「飛ばす条件」。
- 書いていて今の設計に足りなかったものは、サンプルの `README.md`「分かったこと」に残す。
