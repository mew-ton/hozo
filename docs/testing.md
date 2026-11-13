# テスト

## この文書が決めること

テストの置き場、書き方、CI で確かめること。品質の要件の持ち主は `docs/requirements.md`「品質の要件」で、ここはそれを満たす手順。

## 規則ごとのテスト

`tests/rules/<ID>/` に、その規則が通る例 `ok_*.hozo` と落ちる例 `ng_*.hozo` を少なくとも一つずつ置く。対象は `.hozo` と `hozo.toml` だけで出せる診断（`docs/check.md`、`docs/resolve.md`、`docs/types.md`、`docs/config.md` の「診断」）。出力・プラグイン・言語サーバーでしか出ないもの（GOEMIT-*、CLI-*、MAN-*、PLG-*、LSP-1）は、それぞれのパッケージの単体テスト（下）で確かめる。期待を例のそばに置くために、このコメントの形にする。

- 落ちる例は、診断が出る行の末尾に `// ERROR: <ID>` か `// WARN: <ID>` のコメントを書く。同じ行に複数なら `// ERROR: A, B`。
- テストはディレクトリごとに `hozo.toml`（なければ `tests/rules/hozo.toml` を使う）で `Project` を作り、診断の集合を「ファイル、行、重さ、ID」の組で比べる。期待にない診断、出なかった期待はどちらも失敗。
- メッセージの文言は比べない（変えやすくするため）。文言はゴールデン（下）で確かめる。
- 文法で閉じている規則（`—` のもの）は、構文エラーになる例を `ng_*.hozo` に置き、`// ERROR: SYNTAX` で確かめる。

## コーパス

`tests/corpus/<名前>/` に、設計書の例を集めたプロジェクト（`hozo.toml` と `.hozo`）を置く。

- `design/`: 設計書 §5〜§11 の例を名前空間ごとに一つのファイルにまとめたもの。設計書の言葉をそのまま写し、写せなかった箇所と足した宣言は、そのディレクトリの `README.md` に一覧する（`docs/decisions.md`「設計書へ返すこと」の項目がその候補）。
- `view/`: プラグインが受け取る `View`（`docs/ir.md`「その名前空間から見た IR」）を端から端まで確かめるためのサンプル。`docs/ir.md`「正規化」の各行と `docs/check.md`「計算」の各手順を一か所ずつ使い、テストの語彙 `t.entry` の付いた op を起点に選ぶ（`docs/plugin-protocol.md`「選択」）プラグイン `plugins/view-dump` が、受け取った `View` を書き出す。プラグインを設定しているのは `web` だけで、`api` は何も出力しない。`tests/view_test.go` が、`internal/project` の `Views()["web"]` をゴールデン `testdata/expected/view.web.json` と比べ、要求ごとの期待値を文書の見出しの名前のサブテストで明示し、実物の `hozoc emit` がプラグインに渡した `View` が同じであることを確かめる。見出し → 構文の対応と想定する warning は `README.md`。
- 各コーパスについて、`hozoc check` の text 出力をゴールデン `tests/corpus/<名前>/testdata/expected/check.txt` と比べる。warning も含む（`tests/corpus_emit_test.go` の `TestCorpusCheckGolden`）。
- `emit` のゴールデン: `tests/corpus/<名前>/testdata/expected/emit/` に書き出したファイルの木（`.hozoc-outputs.json` を含む）を置き、実際に一時ディレクトリに書いて比べる。既定の出力はプラグインが選んだ宣言だけを出す（`docs/emit-go.md`「出力の対象」）ので、ゴールデンを持つコーパスには選ぶプラグインを設定する: `view` は `plugins/view-dump`、ほかはテスト用の `tests/plugins/pick`（コーパスの `plugins/pick/pick.json` に書いたデコレータで `roots` / `stops` / `states` / `wraps` を選ぶ。`"all": true` ならすべての op と状態）。差分はファイルごとに示す（同 `TestCorpusEmitGolden`）。ゴールデンをすべて `testdata/` の下に置くのは、ゴールデンの `.go` を `go build ./...` / `go vet ./...` がパッケージとして拾わないため（Go は `testdata` を無視する）。一時ディレクトリにはリポジトリの `go.mod` を写し、`ir/` `plugin/` `tests/` をシンボリックリンクで足す（`tests/generated_test.go` と同じ）。コーパスのプラグインのマニフェストの `go run github.com/mew-ton/hozo/tests/corpus/<名前>/plugins/<p>` がそこから解決でき、既定の出力の import パスも `github.com/mew-ton/hozo/<out>` に決まる。比べた後、書き出した Go のパッケージをその一時ディレクトリで `go vet` に通す（`runtime/` をシンボリックリンクで足す。ゴールデンと一致していても、コンパイルできない出力を見逃さないため）。
- ゴールデンの更新は `go test ./... -update`。

- `state/`: 状態の分類、遷移の効果、状態での `match`（`docs/check.md`「状態の形」）のサンプル。IR の中身は `tests/view_test.go` の `TestStateCorpusIR` が見出しごとに確かめ、Go の既定の出力（`docs/emit-go.md`「state」「classify」「match」）は `emit` のゴールデンが確かめる。
- `select/`: 出力の対象（`docs/emit-go.md`「出力の対象」「デコレータ」、`docs/plugin-protocol.md`「選択」）のサンプル。起点から辿られる op と辿られない op、`stops` の op、`wraps` で引き受けたデコレータと印のままのデコレータ、`states` で選ぶ状態、別の名前空間の op（`Deps`）、プラグインのない名前空間を一か所ずつ使う。`tests/select_test.go` の `TestSelectCorpus` が、実物の `hozoc emit` の出力を手順ごとに確かめる。
- `model/`: 「モデルが先」（`docs/requirements.md`）のサンプル。出力する名前空間 `shop` にプラグインがないので、`emit` は何も書かない（ゴールデンを持たない）。`tests/model_test.go` の `TestModelCorpus` が、IR と、前回の出力が消えることを確かめる。

段階 1 の途中（検査器ができるまで）は、`tests/parse_test.go` がコーパス・標準の語彙・`docs/cli.md` の `cli.hozo` を構文だけ確かめる。

hozoc 自身（リポジトリ直下の `cli.hozo` と `hozo.toml`）はコーパスには入れない。`hozoc check` が error なしで通ることを `tests/selfcheck_test.go` で確かめ、`emit` の結果は「生成物の一致」で確かめる。

## 言語サーバーと check の一致

各コーパスについて、`internal/project.Load` を一度呼び、`hozoc check` の診断と `internal/lsp` が `publishDiagnostics` で送る診断（LSP の形から戻したもの）が同じ集合であることを確かめる。`LSP-1` だけは言語サーバーにしか出ないので除く。

## 生成物の一致

CI で `go generate ./...` と `sdk/ts` の `npm run generate` と `make generate`（`hozoc emit`。段階 3 から）を実行し、`git diff --exit-code` で差分がないことを確かめる。対象は `docs/packages.md`「生成するもの」の表。`hozoc emit` の分は `tests/generated_test.go` も確かめる: 一時ディレクトリに `cli.hozo` と `hozo.toml` を写して emit し、コミット済みの `internal/gen/hozoc/**` とバイト単位で比べる（`go test ./...` だけで遅れが分かる）。

## 予約語の一致

`token.Keywords` と `docs/grammar.md`「予約語」の表が同じ集合であることを、文法の文書を読んで確かめるテスト（表の行を正規表現で拾う）。TextMate の文法は `token.Keywords` から生成するので、生成物の一致で足りる。

## プラグインのプロトコル

`internal/plugin` のテストは、`testdata/echo-plugin/`（`go run` で動く小さなプラグイン）を使い、要求がそのまま応答に写ること、終了コードが 0 以外なら `proc.Crashed`、JSON でなければ `proc.MalformedOutput` になることを確かめる。`plugins/go-cli` は、`plugins/go-cli/internal/gocli/testdata/` のゴールデン（`cli.hozo` と小さな例から作った `cli.go`）と、生成物の遅れの検査（「生成物の一致」）で確かめる。

## 単体テスト

- `internal/syntax`: 文法の各非終端記号について、受理する例と拒否する例。回復の後に読めた宣言が残ること。
- `internal/sema`: 解決の手続きの各段（`docs/resolve.md`「手続き」）、型の互換の各規則（`docs/types.md`「互換」）に一つずつ。
- `internal/check`: 計算（Sig の E / F / rel / succ、bind）は、小さな IR を直接組み立てて期待値と比べる。設計書 §9 の `browser.logout` の例（`server.deleteSession` が不明、`removeToken` が起きている）を必ず含める。
- `internal/emit/golang`: GOEMIT-* の各条件。生成物は `go/format` を通ること。出力の対象（`docs/emit-go.md`「出力の対象」の各手順）。生成したコードを一時のモジュールで実際に動かす振る舞いのテスト（`testdata/<名前>/behavior_test.go`。`nil` のフィールドが `*runtime.MissingError` になること、`States` を通る配線）。
- `internal/plugin`: MAN-* と PLG-3 の各条件（「プラグインのプロトコル」）。PLG-1 / PLG-2 は応答のファイルを集める `internal/app` のテストで確かめる。
- `plugins/go-cli`: CLI-* の各条件と、引数の解釈の各規則（`docs/plugin-go-cli.md`「引数の解釈」）。
- `internal/lsp`: `hozo.toml` のないワークスペースで LSP-1 が出ること。

## CI

`.github/workflows/ci.yml`: `go build ./...`、`go vet ./...`、`go test ./...`、生成物の一致。`sdk/ts` と `editors/vscode` は Node 22 で `npm ci && npm test`（段階 4 から）。
