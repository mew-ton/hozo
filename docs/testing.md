# テスト

## この文書が決めること

テストの置き場、書き方、CI で確かめること。品質の要件の持ち主は `docs/requirements.md`「品質の要件」で、ここはそれを満たす手順。

## 規則ごとのテスト

`tests/rules/<ID>/` に、その規則が通る例 `ok_*.hozo` と落ちる例 `ng_*.hozo` を少なくとも一つずつ置く（ID は `docs/requirements.md`「検査の規則の一覧」と各設計文書の診断の表）。

- 落ちる例は、診断が出る行の末尾に `// ERROR: <ID>` か `// WARN: <ID>` のコメントを書く。同じ行に複数なら `// ERROR: A, B`。
- テストはディレクトリごとに `hozo.toml`（なければ `tests/rules/hozo.toml` を使う）で `Project` を作り、診断の集合を「ファイル、行、重さ、ID」の組で比べる。期待にない診断、出なかった期待はどちらも失敗。
- メッセージの文言は比べない（変えやすくするため）。文言はゴールデン（下）で確かめる。
- 文法で閉じている規則（`—` のもの）は、構文エラーになる例を `ng_*.hozo` に置き、`// ERROR: SYNTAX` で確かめる。

## コーパス

`tests/corpus/<名前>/` に、設計書の例を集めたプロジェクト（`hozo.toml` と `.hozo`）を置く。

- `design/`: 設計書 §5〜§11 の例を名前空間ごとに一つのファイルにまとめたもの。設計書の言葉をそのまま写し、写せなかった箇所は `tests/corpus/README.md` に一覧する（`docs/decisions.md`「設計書へ返すこと」の項目がその候補）。
- `hozoc/`: リポジトリ直下の `cli.hozo` と `hozo.toml` のコピーではなく、シンボリックリンクでもなく、テストがリポジトリ直下を直接読む。
- 各コーパスについて、`hozoc check` の text 出力をゴールデン `expected.check.txt` と比べる。warning も含む。
- `emit` のゴールデン: `expected/` に書き出したファイルの木を置き、`hozoc emit --dry-run` ではなく実際に一時ディレクトリに書いて比べる。差分はファイルごとに示す。
- ゴールデンの更新は `go test ./... -update`。

## 言語サーバーと check の一致

各コーパスについて、`internal/project.Load` を一度呼び、`hozoc check` の診断と `internal/lsp` が `publishDiagnostics` で送る診断（LSP の形から戻したもの）が同じ集合であることを確かめる。`LSP-1` だけは言語サーバーにしか出ないので除く。

## 生成物の一致

CI で `go generate ./...` と `sdk/ts` の `npm run generate` と `hozoc emit`（段階 3 から）を実行し、`git diff --exit-code` で差分がないことを確かめる。対象は `docs/packages.md`「生成するもの」の表。

## 予約語の一致

`token.Keywords` と `docs/grammar.md`「予約語」の表が同じ集合であることを、文法の文書を読んで確かめるテスト（表の行を正規表現で拾う）。TextMate の文法は `token.Keywords` から生成するので、生成物の一致で足りる。

## プラグインのプロトコル

`internal/plugin` のテストは、`testdata/echo-plugin/`（`go run` で動く小さなプラグイン）を使い、要求がそのまま応答に写ること、終了コードが 0 以外なら `proc.Crashed`、JSON でなければ `proc.MalformedOutput` になることを確かめる。`plugins/go-cli` は、コーパス `hozoc/` に対する `emit` のゴールデンで確かめる。

## 単体テスト

- `internal/syntax`: 文法の各非終端記号について、受理する例と拒否する例。回復の後に読めた宣言が残ること。
- `internal/sema`: 解決の手続きの各段（`docs/resolve.md`「手続き」）、型の互換の各規則（`docs/types.md`「互換」）に一つずつ。
- `internal/check`: 計算（Exc / Eff / Rel / Succ / bind）は、小さな IR を直接組み立てて期待値と比べる。設計書 §9 の `browser.logout` の例（`server.deleteSession` が不明、`removeToken` が起きている）を必ず含める。

## CI

`.github/workflows/ci.yml`: `go build ./...`、`go vet ./...`、`go test ./...`、生成物の一致。`sdk/ts` と `editors/vscode` は Node 22 で `npm ci && npm test`（段階 4 から）。
