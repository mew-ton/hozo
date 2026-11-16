# テスト

## この文書が決めること

テストの置き場、書き方、CI で確かめること。品質の要件の持ち主は `docs/requirements.md`「品質の要件」で、ここはそれを満たす手順。

## 規則ごとのテスト

`tests/rules/<ID>/` に、その規則が通る例 `ok_*.hozo` と落ちる例 `ng_*.hozo` を少なくとも一つずつ置く。対象は `.hozo` と `hozo.toml` だけで出せる診断（`docs/check.md`、`docs/resolve.md`、`docs/types.md`、`docs/config.md` の「診断」）。プラグイン・言語サーバーでしか出ないもの（GO-*、GOVALID-*、GOSQL-*、GOHTTP-*、CLI-*、MAN-*、PLG-*、LSP-1）は、それぞれのパッケージの単体テスト（下）で確かめる。期待を例のそばに置くために、このコメントの形にする。

- 落ちる例は、診断が出る行の末尾に `// ERROR: <ID>` か `// WARN: <ID>` のコメントを書く。同じ行に複数なら `// ERROR: A, B`。
- テストはディレクトリごとに `hozo.toml`（なければ `tests/rules/hozo.toml` を使う）で `Project` を作り、診断の集合を「ファイル、行、重さ、ID」の組で比べる。期待にない診断、出なかった期待はどちらも失敗。
- `hozo.toml` の単位はトランスパイラを要る（`docs/config.md`「単位」）ので、起動しないプラグインの fixture を置く: 既定のもの `tests/rules/plugins/std`（基底 `go` / `go-b` / `ts` と、`go` に相乗りする `go-x`、`go` と `go-x` に相乗りする `go-y`）と、規則のディレクトリの `plugins/`（その規則のためのもの）。どちらも各例のプロジェクトの `plugins/` に写す。既定の `tests/rules/hozo.toml` は、名前空間 `a` をトランスパイラ `go` で出力する単位一つ。規則のテストは `check` だけで、プラグインを起動しない。
- 欠番にした規則（CFG-8、CFG-11）のディレクトリは消す。足した規則（CFG-19〜CFG-22）のディレクトリを足す。
- メッセージの文言は比べない（変えやすくするため）。文言はゴールデン（下）で確かめる。
- 文法で閉じている規則（`—` のもの）は、構文エラーになる例を `ng_*.hozo` に置き、`// ERROR: SYNTAX` で確かめる。

## コーパス

`tests/corpus/<名前>/` に、設計書の例を集めたプロジェクト（`hozo.toml` と `.hozo`）を置く。

- `design/`: 設計書 §5〜§11 の例を名前空間ごとに一つのファイルにまとめたもの。設計書の言葉をそのまま写し、写せなかった箇所と足した宣言は、そのディレクトリの `README.md` に一覧する（`docs/decisions.md`「設計書へ返すこと」の項目がその候補）。
- `view/`: プラグインが受け取る `View`（`docs/ir.md`「その名前空間から見た IR」）を端から端まで確かめるためのサンプル。`docs/ir.md`「正規化」の各行と `docs/check.md`「計算」の各手順を一か所ずつ使う。プラグイン `plugins/view-dump` のトランスパイラ `view-dump`（`requires` を持たない、テストのための基底）が、受け取った `View` を `view.json` として書き出す。`api` と `web` の二つの単位が、どちらもそれを使う（`web` は `api` の型を使うので、`api` も同じ基底の出力を持つ。`docs/config.md`「型の置き場」）。`tests/view_test.go` が、`internal/project` の `web` の `View` をゴールデン `testdata/expected/view.web.json` と比べ、要求ごとの期待値を文書の見出しの名前のサブテストで明示し、実物の `hozoc emit` がプラグインに渡した `View` が同じであることを確かめる。見出し → 構文の対応と想定する warning は `README.md`。
- 各コーパスについて、`hozoc check` の text 出力をゴールデン `tests/corpus/<名前>/testdata/expected/check.txt` と比べる。warning も含む（`tests/corpus_emit_test.go` の `TestCorpusCheckGolden`）。
- `emit` のゴールデン: `tests/corpus/<名前>/testdata/expected/emit/` に書き出したファイルの木（`.hozoc-outputs.json` を含む）を置き、実際に一時ディレクトリに書いて比べる。単位のトランスパイラは、リポジトリの `plugins/` の本物（基底 `go`）で、`HOZO_PLUGIN_PATH` として渡す（`view` だけは、コーパスの `plugins/view-dump`）。差分はファイルごとに示す（同 `TestCorpusEmitGolden`）。ゴールデンをすべて `testdata/` の下に置くのは、ゴールデンの `.go` を `go build ./...` / `go vet ./...` がパッケージとして拾わないため（Go は `testdata` を無視する）。一時ディレクトリにはリポジトリの `go.mod` を写し、`ir/` `plugin/` `plugins/` `tests/` をシンボリックリンクで足す（`tests/generated_test.go` と同じ）。プラグインのマニフェストの `go run` がそこから解決でき、生成したコードの import パスも `github.com/mew-ton/hozo/<out>` に決まる。比べた後、書き出した Go のパッケージをその一時ディレクトリで `go vet` に通す（`runtime/` をシンボリックリンクで足す。ゴールデンと一致していても、コンパイルできない出力を見逃さないため）。
- **コーパスのゴールデンは、サンプルの期待する出力（「トランスパイラのテスト」）とは役目が違う。** サンプルは人が書いた仕様で、実装より先にあり、`-update` で書き直さない。コーパスのゴールデンは、実装が書いたものを `-update` で置き、差分をレビューで見る、後戻りの検出のためのもの（サンプルが使わない書き方 — union、述語で決まる分類、状態の分類と遷移、`bind`、値のアーム — を広く通す）。
- ゴールデンの更新は `go test ./... -update`。

コーパスごとの、`emit` のゴールデンの扱い（出力をトランスパイラに任せ、選択をなくしたときに決めた。`docs/decisions.md`「決めたこと」81）:

| コーパス | 単位のトランスパイラ | `emit` のゴールデン |
|---|---|---|
| `design/` | `server` と `browser` は `go`。使う型の置き場として、語彙の `http` と `page` にも `go` の単位を足す | 持つ（基底 `go` が出したものに作り直す）。利用者が `valid` に足した制約（§6.5）は、`go-valid` を並べないので出力に現れない |
| `state/` | `go` | 持つ（作り直す）。状態の分類、遷移の効果、状態での `match`。IR の中身は `tests/view_test.go` の `TestStateCorpusIR` が見出しごとに確かめる |
| `units/` | 四つの単位とも `go` | 持つ（作り直す）。出力の単位（`docs/config.md`「単位」「単位の対象」「in」「型の置き場」）のサンプル: 同じ名前空間の二つの単位（どちらも自分の型を持つ）、`in` で絞った単位（`in` の外の op は、呼ばれていれば `Leaves` の口だけ、呼ばれていなければ出ない。型と状態はすべて出る）、子孫を名指しする単位、単位をまたぐ一方向の import（置き場は設定の順で最初の単位）。`tests/units_test.go` の `TestUnitsCorpus` が実物の `hozoc emit` の出力を確かめる |
| `view/` | `view-dump`（上） | 持つ（`view.json` だけ。作り直す） |
| `model/` | `none`（sh で動く、テストのための基底。ファイルを返さない） | 持たない。「モデルが先」（`docs/requirements.md`）のサンプルで、`tests/model_test.go` の `TestModelCorpus` が、IR と、前回の出力が消えることを確かめる |
| `select/` | — | **コーパスごと消す。** 出力の対象をトランスパイラが選ぶ仕組み（`select`）のサンプルで、確かめる相手がなくなった。`tests/select_test.go` も消す。別の名前空間の op の口と、印のデコレータは `design/` が通る |

テスト用のプラグイン `tests/plugins/pick`（選択だけを返す代役）は消す。コーパスの `plugins/pick/` も消す。

`tests/parse_test.go` は、コーパス・標準の語彙・プラグインの語彙のすべての `.hozo` が構文エラーなしに解析できることと、`docs/cli.md`「cli.hozo」に埋め込んだ `cli.hozo` と `hozo.toml` のブロックがリポジトリ直下のファイルとバイト単位で同じであること（`TestDocsEmbedRootFiles`）を確かめる。

hozoc 自身（リポジトリ直下の `cli.hozo` と `hozo.toml`）はコーパスには入れない。`hozoc check` が error なしで通ることを `tests/selfcheck_test.go` で確かめ、`emit` の結果は「生成物の一致」で確かめる。

## 言語サーバーと check の一致

各コーパス（`tests/corpus/` の下のすべて。足せば自動で入る）について、`internal/project.Load` を一度呼び、`hozoc check` の診断と `internal/lsp` が `publishDiagnostics` で送る診断（LSP の形から戻したもの）が同じ集合であることを確かめる（`internal/lsp` の `TestCheckAgreement`。位置のエンコーディングは utf-16 と utf-8 の両方）。`LSP-1` だけは言語サーバーにしか出ないので除く。診断のないコーパスでは「どちらも空」を確かめることになるので、設計書のコーパスに warning を一つ足したものでも確かめる。

## 生成物の一致

CI で `go generate ./...` と `sdk/ts` の `npm run generate` と `make generate`（`cli.hozo` からの自己生成。`docs/packages.md`「自己生成の手順」）を実行し、`git diff --exit-code` で差分がないことを確かめる。対象は `docs/packages.md`「生成するもの」の表。`hozoc emit` の分は `tests/generated_test.go` も確かめる: 一時ディレクトリに `cli.hozo` と `hozo.toml` を写して emit し、コミット済みの `internal/gen/hozoc/**` とバイト単位で比べる（`go test ./...` だけで遅れが分かる）。

手で写すもの（生成しないもの）の一致もテストで確かめる: `sdk/ts/src/protocol.ts` がスキーマの `$defs` と合っていること（`sdk/ts/test/protocol.test.ts`）、`docs/cli.md` のブロックがリポジトリ直下のファイルと同じであること（「コーパス」の `TestDocsEmbedRootFiles`）。

## 文書とコードの一致

文書が持ち主の事実のうち、コードから機械的に確かめられるものはテストにする。

- 規則ごとのテストがそろっていること（`tests/rules_test.go` の `TestRulesCoverDocs`。「規則ごとのテスト」）。
- 診断のコード: Go のコードが出しうる診断のコード（規則の ID と、`SYNTAX` / `IO` / `PLUGIN` / `GO-INTERNAL`。プラグインが `errors` で返すコードを含む）がどれも設計文書の「診断」の表にあり、表の重さ（error / warning。診断にならない ID は「—」）がコードと合っていること（`tests/codes_test.go` の `TestDiagnosticCodesAreDocumented`。コードの側はソースを構文木で読んで集める）。
- 依存の向き: `docs/packages.md`「依存の向き」の表が Go のパッケージの実際の import と同じで、箇条書きの規則がテストのファイルの import を含めて守られていること（`tests/deps_test.go`）。
- 予約語（下の「予約語の一致」）。

## 環境からの独立

テストの結果は、実行する人の環境に入っているプラグインに左右されない。プラグインの探索が見るプロジェクトの外の場所（`HOZO_PLUGIN_PATH` とホーム。`docs/plugin-protocol.md`「探索」）は、探索が自分では読まず呼ぶ側が渡すもので、プロジェクトを読むテストはどれも、それを渡さない（`project.Load`、`Home` が空の `app.App`、環境変数を返さない関数を渡した `cli.Main`）か、テストが作った一時ディレクトリを渡す。確かめ方は、壊れたマニフェストを置いたディレクトリを `HOZO_PLUGIN_PATH` に、`.hozo/plugins` に壊れたマニフェストを置いたディレクトリを `HOME` に向けて、`go test ./...` がすべて通ること（`HOME` を変えるときは、`GOCACHE` / `GOMODCACHE` / `GOPATH` を元の値で明示する）。渡したときに探すことと、渡さなければプロセスの環境を読まないことは、`internal/plugin`、`internal/project`、`internal/app` のテストが確かめる。

## 予約語の一致

`token.Keywords` と `docs/grammar.md`「予約語」の表が同じ集合であることを、文法の文書を読んで確かめるテスト（表の行を正規表現で拾う）。TextMate の文法は `token.Keywords` から生成するので、生成物の一致で足りる。

## プラグインのプロトコル

`internal/plugin` のテストは、`testdata/echo-plugin/`（テストの初めにビルドする小さなプラグイン）を使い、要求がそのまま応答に写ること、終了コードが 0 以外なら `proc.Crashed`、JSON でない・形が違う（`docs/plugin-protocol.md`「応答の形」: 必須の項目がない、型が違う。知らない項目は通る）なら `proc.MalformedOutput` になることを確かめる。二つの SDK（Go の `plugin`、TS の `sdk/ts`）が同じ要求に同じ終了コードを返すことは、どちらのテストも同じ表（`plugin/testdata/request_head.json`）を読んで確かめる。`plugins/` のプラグインは、サンプル（「トランスパイラのテスト」）、それぞれの単体テスト（「単体テスト」）、生成物の遅れの検査（「生成物の一致」。`go`、`go-valid`、`go-cli`）で確かめる。

## 単体テスト

- `internal/syntax`: 文法の各非終端記号について、受理する例と拒否する例。回復の後に読めた宣言が残ること。
- `internal/sema`: 解決の手続きの各段（`docs/resolve.md`「手続き」）、型の互換の各規則（`docs/types.md`「互換」）に一つずつ。
- `internal/check`: 計算（Sig の E / F / rel / succ、bind）は、小さな IR を直接組み立てて期待値と比べる。設計書 §9 の `browser.logout` の例（`server.deleteSession` が不明、`removeToken` が起きている）を必ず含める。
- `plugin/golang`: 名前、型、置き場、op の置き場と形（`docs/plugin-go.md` の各表の行に一つずつ）。相乗りが使うのと同じ関数を、基底も使うことを前提に、ここで細かい場合分けを確かめる。
- `plugin/govalid`: `Validate` を持つ型の規則の各行。
- `plugins/go`（`internal/gobase`）: GO-1〜GO-6 の各条件。`docs/plugin-go.md`「配線の本体」の各表の行（文、値、型をそろえる、状態の参照、`bind` で埋まる位置、`match` の分け方とアームの本体の組）を、小さな `.hozo` と、出力のゴールデン（`testdata/<名前>/`）で一つずつ。生成物は `go/format` を通ること。生成したコードを一時のモジュールで実際に動かす振る舞いのテスト（`testdata/<名前>/behavior_test.go`: 述語で決まる分類の判定、状態を通る配線、内側のフォルダから内部の op を呼べること、入口の `Ops` が `export` の op だけを持つこと）。同じ `View` から同じバイト列を返すこと。
- `plugins/go-valid`、`plugins/go-sqlite`、`plugins/go-http`、`plugins/go-cli`: それぞれの文書の「診断」の各条件と、サンプルが使わない場合（`go-valid` の `pattern` / `max` / `record` / 省略可能、`go-sqlite` の列の型と省略可能な列と書き換え・規則に当たらない op、`go-http` の複数の `@http.body`・必須のヘッダとクエリ・値を運ぶ例外・`validate = false`、`go-cli` の引数の解釈の各規則）を、小さな `.hozo` と出力のゴールデンで。相乗りのテストは、基底を起動せず、`View` だけを入力にする（基底の応答に依らないことの確かめになる）。
- サンプルにない場合の Go のテキスト（空白、補助の名前の細部）は、文書の規則を満たす限り実装が決め、この単体テストのゴールデンが固定する。
- `internal/plugin`: MAN-* の各条件（`requires` の誤りを含む）、応答の形の各条件（「プラグインのプロトコル」。版 0.3 までの `select` を持つ応答が、読まれずに通ること）。PLG-1 / PLG-2 は応答のファイルを集める `internal/app` のテストで確かめる。
- `internal/project`: 単位のトランスパイラの解決（CFG-14〜CFG-16、CFG-19、CFG-20）、`in` の照合（CFG-18。語彙のファイルがいつも合うこと）、型の置き場（CFG-22 の表の各行）、`View` の `config`（`index`、`base`、`options`）、使うプラグインだけの語彙と `check`、MAN-2 を使うプラグインにだけ出すこと。
- `internal/app`: `emit` の手順（`docs/cli.md`「emit」）。sh で動く小さなプラグインで、同じ名前空間の複数の単位、`in`、子孫を名指しする単位、引数での emit の対象の絞り込み（子孫だけが単位の対象である引数、`UnknownNamespace`。絞っても内容が同じこと、対象でない出力のトランスパイラを起動しないこと）、二つのトランスパイラが同じパスを返す衝突（CFG-5）、`errors` を返したトランスパイラがあれば何も書かないこと、`.hozoc-outputs.json` に書かれた Dir(U, N) の外のパスを消さないこと、空になったディレクトリを消すことを確かめる。`internal/app` と `internal/project` が、`plugin/golang` も `plugins/` も import しないこと（本体が言語を知らないこと）は、依存の向きのテスト（「文書とコードの一致」）が確かめる。
- `internal/tools/selfgen`: コンパイルできない生成物を置き場に置かないこと（`docs/packages.md`「自己生成の手順」）。
- `plugins/go-cli`: CLI-* の各条件と、引数の解釈の各規則（`docs/plugin-go-cli.md`「引数の解釈」）。
- `internal/lsp`: `hozo.toml` のないワークスペースで LSP-1 が出ること。

## CI

`.github/workflows/ci.yml`: `go build ./...`、`go vet ./...`、`go test ./...`、生成物の一致。`sdk/ts` と `editors/vscode` は Node 22 で `npm ci && npm test`。

## トランスパイラのテスト

トランスパイラ（基底と相乗り。TS 向けのものを含む）のテストの方針。作る段階の持ち主は `docs/requirements.md`「段階」。

- 進め方は**テストが先**。トランスパイラごとに、まず**ブラックボックスのテスト**を書く: 入力は `.hozo` のプロジェクト（`hozo.toml` と `.hozo`）、期待は書き出されるファイルのそのままの内容（バイト単位で同じ）。このテストを、そのトランスパイラの実装を一行も書く前に、すべて書く。
- ブラックボックスのテストは、**サンプル**（下）の、期待する出力との比較。期待する出力は人が書いた仕様で、実装に合わせて書き直さない（食い違ったら、文書とサンプルのどちらが正しいかを決めてから、正しくない方を直す）。
- 細かい条件（語彙の使い方の誤りごとの診断、型や名前の変換の場合分けなど）は、単体テスト（「単体テスト」）で確かめる。ブラックボックスのテストに条件の組み合わせを背負わせない。
- 実装の順は、依存の向き: 公開の SDK（`plugin/golang`）、基底、相乗り。一つ実装するごとに、サンプルの、そのトランスパイラの分（下の表の `transpilers/<名前>`）が通る。

### サンプル

トランスパイラの形についての問いを具体的な組で決めるために、**サンプル**を先に書く: `tests/samples/<名前>/` に置いた、仕様（`hozo.toml` と `.hozo`）と、手で書いた完成したコード（各トランスパイラが出すべきファイルと、人が書く部分）の組。置き方、`sample.json`、テストの使い方の持ち主は `tests/samples/README.md`。

`tests/samples_test.go` の `TestSamples` が、サンプルごとに、仕様と設定が `hozoc check` を通ること（`check`）、期待する出力のファイルの持ち主がそろっていること（`files`）、期待するコードが本当に動くこと（`go`）、`hozoc emit` の出力が期待する出力とバイト単位で同じこと（`transpilers/<名前>` と `transpilers/all`）を確かめる。サブテストの中身と、飛ばす条件の持ち主は `tests/samples/README.md`「テスト」。

- `transpilers/<名前>` をトランスパイラごとに分けるのは、失敗がどのトランスパイラのものかが分かり、一つ実装するごとに、その分が通るようにするため。
- これまでの `shared`（期待する出力のうち、本体に組み込んだ出力のファイルが、今の `hozoc emit` の出力と同じ）と、選ぶ宣言を代役に伝える `sample.json` の `select`、まだ読めない書き方のサンプルを保留にする `pending` は、なくす。本体に組み込んだ出力がなくなり、代役が要らなくなったため。
- サンプルは自分の `go.mod` を持つ別のモジュールで、リポジトリのモジュールの `go build ./...` などはその Go を見ない。サンプルだけが使う依存（SQLite のドライバなど）は、サンプルの `go.mod` にだけ置く。
- 書いていて今の設計に足りなかったものは、サンプルの `README.md`「分かったこと」に残す。
