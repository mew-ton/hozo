# 標準の語彙

## この文書が決めること

hozoc が同梱して常に読み込む語彙（`vocab/*.hozo`）と、その読み込み方。語彙の宣言の意味の持ち主は設計書 §4.2 と `examples/hozoc.hozo`。ここに置く `.hozo` はそれを写したもの。

## 読み込み

- `vocab/*.hozo` は hozoc のバイナリに埋め込む（Go の `embed`）。`hozoc check` / `emit` / `lsp` は、ユーザーの `.hozo` より前にこれらを同じプロジェクトのファイルとして読む（`docs/resolve.md`「標準の語彙」）。`Span.file` は `hozo:vocab/<名前>.hozo`。
- プラグインのマニフェストの `vocabulary`（`docs/plugin-protocol.md`）の `.hozo` も同じ扱いで、`Span.file` は `plugin:<プラグイン名>/<パス>`。そのプラグインがどこかの名前空間の `plugins` に書かれているときだけ読む。
- ユーザーは同じ名前空間に宣言を足せる（§3.2、§6.5）。標準の語彙の宣言と同じ名前を足せば RES-6。
- 標準の語彙は `hozoc lsp` の定義へジャンプの飛び先になる。埋め込んだ内容を読み取り専用の仮想文書として返す（`docs/lsp.md`）。

## 同梱する語彙

| ファイル | 中身 | 出典 |
|---|---|---|
| `vocab/valid.hozo` | `namespace valid { min / max / length / pattern / format }` | §4.2、§6.5 |
| `vocab/http.hozo` | `namespace http { ServerError / NetworkError / get / post / status / header / body / query / retry }` | §4.2 |
| `vocab/db.hozo` | グローバルの `decorator db for state;` と `namespace db { key / unique / ref }` | §4.2 |
| `vocab/localstorage.hozo` | グローバルの `decorator localStorage for state;` | §4.2 |
| `vocab/page.hozo` | `namespace page { Aborted / Route / path / query / route / dialog / fallback / interaction }` | §4.2 |
| `vocab/cli.hozo` | `namespace cli { command / arg / flag / env / exitCode }` | `examples/hozoc.hozo` |
| `vocab/fs.hozo` | `namespace fs { files }` | `examples/hozoc.hozo` |
| `vocab/proc.hozo` | `namespace proc { Crashed / MalformedOutput / stdio }` | `examples/hozoc.hozo` |

- 各ファイルの中身は出典の宣言をそのまま写す。ドキュメンテーションコメントも写す（ホバーに出るため）。
- `cli` / `fs` / `proc` は `examples/hozoc.hozo` ではユーザーの宣言として書かれているが、hozoc 自身（`cli.hozo`）が使うので標準に入れる。`cli.hozo` からはこれらの宣言を外す（二重に宣言すると RES-6）。
- `@http.get` / `@http.post` の `@external` はそのまま写す。
- `proc.stdio` だけは出典から変える: 引数 `command` を持たず、`@external` も付けない（`decorator stdio throws Crashed | MalformedOutput;`）。呼び先は呼び出す op の入力で渡し（`docs/cli.md` の `runEmit`）、例外は同じ名前空間の呼び出し元にも見えなければならないため。`docs/decisions.md`。

## 同梱しない語彙

- `cache`（§4.2 `decorator cache(key: CacheKey, staleTime: Duration)`）: `CacheKey` の宣言がなく、§11.1 の例は `key` に文字列と配列の両方を渡していて、一つの型で受けられない。「引数（キーなど）はキャッシュのプラグインの詳細設計で決める」（§4.2）に従い、段階 5 のキャッシュのプラグインの `vocabulary` にする。`docs/decisions.md`。

## 語彙の意味の実装

語彙の宣言は名前と説明と関係だけ（§2）。意味をコードにするのは:

| 語彙 | 実装 |
|---|---|
| `valid` の標準の制約 | 既定の出力（`docs/emit-go.md`「scalar」） |
| `cli` | `plugins/go-cli/`（`docs/plugin-go-cli.md`） |
| `fs` / `proc` | 既定の出力が葉の op のインターフェースを出し、hozoc 自身の実装が人の手で書く（§4.3「対応するプラグインがなければ既定の出力になる」） |
| `http` / `db` / `page` | 段階 5 のプラグイン |
