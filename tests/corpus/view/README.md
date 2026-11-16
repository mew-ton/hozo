# コーパス `view`

プラグインが受け取る IR（`docs/ir.md`「その名前空間から見た IR」の `View`）が文書の要求どおりであることを、一つのサンプルから端から端まで確かめるためのプロジェクト（`docs/testing.md`「コーパス」、`tests/view_test.go`、`tests/corpus_emit_test.go`）。`docs/ir.md`「正規化」の各行と `docs/check.md`「計算」の各手順を、少なくとも一か所ずつ使う。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | `[hozo] plugin_dirs = ["plugins"]` と、単位が三つ: `namespace = "api"`、`"web"`、`"page"`。どれも `transpilers = ["view-dump"]`（TS の基底はまだないので、`api` と `web` を同じ基底にする）。`api` と `page` の単位は、`web` が使う型の置き場（`docs/config.md`「型の置き場」。なければ CFG-22） |
| `t.hozo` | テストの語彙 `namespace t { decorator entry; }`。`plugin.OpsWith` が探す op の印。標準の語彙にない名前空間で、どの単位の対象でもないので出力しない（`docs/config.md`「単位の対象」） |
| `api.hozo` | サーバー側 `namespace api`。`web` から呼ばれる（呼ばれる op は `export`）。`docs/ir.md`「版」の 0.3 で足した形（継承した model の `override`、形の語、状態の型、`reads`、op の分類とケースの `as`、状態の参照、アームの値と `throw`、`never`）はここに置く |
| `web.hozo` | ブラウザ側 `namespace web`。テストが見る `View` の viewer。`@t.entry` の付いた op が、`plugin.OpsWith` の対象 |
| `plugins/view-dump/` | テスト用のプラグイン。トランスパイラ `view-dump`（`requires` を持たない、テストのための基底）を提供する: 受け取った `view` を 2 スペースの字下げで JSON にし直し、`<view.config.dir>/view.json` の一つのファイルとして返す。ほかの名前の `transpiler` は `code: "unsupported"` |
| `testdata/expected/check.txt` | `hozoc check` の text 出力のゴールデン（下の warning を含む） |
| `testdata/expected/view.web.json` | `Views()["web"]` の JSON のゴールデン（`tests/view_test.go` の `TestViewFromEntry`） |
| `testdata/expected/emit/` | `hozoc emit` が書き出すファイルの木のゴールデン: 単位ごとの `view.json`（`gen/api`、`gen/web`、`gen/page`。`gen/web/view.json` は `view.web.json` と同じ内容）と記録。Go のファイルは出ない（hozoc 本体はコードを出力しない。`docs/plugin-protocol.md`「この文書が決めること」） |

ゴールデンの更新は `go test ./tests -update`。`.hozo` の各ブロックの上の `// docs/...「見出し」` のコメントが、その構文が確かめる文書の見出し。

## 見出し → 構文

`tests/view_test.go` の `TestViewFromEntry` のサブテストは、この表の見出しの名前で期待値を明示する。

| 文書の見出し | 構文（ファイル） | 確かめること |
|---|---|---|
| `docs/ir.md`「原則」 | `nullable<api.Token>`、`single<page.Route>`、`fetchUser({ id })`（`web.hozo`） | 参照はすべて FQN。名前空間の順は `""`、`api`、`t`、`web`（ファイルの辞書順）、その後に語彙 `db` `fs` `http` `page` `proc` `valid` |
| `docs/ir.md`「Program」 | `/** ... */ namespace api { }` など三つの名前空間、`page.Route` | `Namespace.description`、`Span.file` の形（ユーザーは相対パス、語彙は `hozo:vocab/<名前>.hozo`）、位置は 1 始まり、宣言の位置は最初のデコレータから |
| `docs/ir.md`「Doc」、`docs/check.md`「名前空間」DOC-1 | `openUser` / `fetchUser` / `api.getUser` の `@param` `@throws` `@see`、フィールド・アーム・ケースの DocComment | `summary` と `body`、`@throws` と `@see` は FQN、説明のない宣言は `description` なし。`api.getUser` の `@throws Timeout` は `external` なデコレータの例外（`Signature.exceptions` にない）で、DOC-1 にならない |
| `docs/ir.md`「型」 | `api.User`（`string??` `string[]` `Record<string, string[]>`）、`scalar AdminToken extends Token`、`UserKind.Admin<api.PublicUser>`、`op signOut()` | `T??` → `T?`、array / record / case、void は `returns: null`、状態の形 `list` / `nullable` / `single`（型の最も外側から決まる。`docs/decisions.md`「設計書を変える決定」S21） |
| `docs/ir.md`「正規化」`namespace` | `web.hozo` の二つの `namespace web { }` | 一つの `Namespace`、decls は出現順に連結 |
| 同 `model D extends B { override a: never; override b: T; c: U; }` | `model PublicUser extends User { override secret: never; override hash: never; }`、`model Admin extends User { override id: AdminId; @db.unique override name; ... level: int = 1; }`（`api.hozo`） | `fields` は展開済み（元の並びのまま Override を当て、足したフィールドは末尾。デコレータも保つ）、`extends` の `base` / `removed` / `overridden`。override したフィールドの説明は override に書いたもの（なければなし）、位置は override の位置 |
| 同 形の語、`state s: T;` | `model Shapes { ids: list<UserId>; nick: nullable<string>; one: single<Token>; twice: nullable<string?>; }`、`state current: Token?;`、`state motd: string;`（`api.hozo`） | `array` / `optional` / そのままの型。`State.type` と、そこから決まる `shape` / `element` |
| 同 `export op f()`、`reads a, b` | `export op findUser(id: UserId): User? reads users, sessions reads users;`（`api.hozo`） | `exported`、`reads` は FQN の列一つ（重複は最初の出現に寄せる）。`never` を返す op の `returns` は `{ kind: "never" }` |
| 同 ケースの `X as T`、`docs/ir.md`「classify」 | `classify Lookup of findUser { Found: findUser as User; default; }`、`classify SignedIn of current { Yes: current as Token; default; }`（`api.hozo`） | `subject` の `op`（`type` はその op の `returns`）、`cases[].as`（状態の分類だけ `state` を持つ）、`by`、`decidedBy` |
| 同 配線の値の位置の状態の名前、アームの本体の `none` | `tokenOrNone`、`greeting`、`guard`、`currentName`（`api.hozo`） | `Atom` の `state`（修飾して書いても同じ）、アームの本体の `atom` / `none` / `throw`、`narrowed` |
| 同 ユニオンのアームのその場の model | `union UserPage { Ready: { ... }; Missing: Unavailable; }`（`web.hozo`） | `web.UserPage.Ready`（`synthesizedFrom`）をユニオンの直後に足し、アームは `ref`。宣言済みの model はそのまま `ref` |
| 同 例外のその場の model | `exception Invalid: { reason: string; };`（`api.hozo`） | `api.Invalid.Value` を例外の直後に足し、`value` は `ref` |
| 同 ユニオンのメンバーにユニオン、`throws` にユニオン | `union AnyError { ClientError; Timeout; }`、`op touch ... throws ClientError keeps effects` | メンバーを平らに展開、`throws` は例外の FQN に展開 |
| 同 複数行の `throws` | `op reload ... throws api.NotFound \| api.Unauthorized` と `throws Offline` | `ThrowsClause` の列（まとめない） |
| 同 `condition` | `condition Verified of api.PublicUser;`、`classify UserKind` | `cases: [Verified]`、`hasDefault: true`、`fromCondition: true` |
| 同 名前付きデコレータ | `decorator redirectToLogin = redirectOn(api.Unauthorized, goLogin);`、`@redirectToLogin` | `alias`、適用は `decorator: "web.redirectOn"`、`via: "web.redirectToLogin"`、`args` は固定した引数 |
| 同 位置で書いた引数、`{ item }`、呼び出しの引数、`5m` | `@http.status(400)`、`fetchUser({ id })`、`api.touch(id)`、`@traced(id, 5m)`、`timeout: Duration = 30s` | `param` に対応付け、`{ id: id }`、`duration` のナノ秒 |
| `docs/ir.md`「decorator」 | `decorator redirectOn(error: any exception, target: any effect) catches error halts target;`、`@external decorator remote throws Timeout;`、`decorator traced(key: string, within: Duration);` | `$param:名前`、`any` の `Param`、`targets` の省略は `["op"]`、`external` |
| `docs/ir.md`「デコレータの適用」 | `@t.entry @redirectToLogin @traced(id, 5m) op openUser(id: ...)` | 書かれた順の列（最後が最も内側）、op の引数を値に取る引数は `$param:id`、`via` |
| `docs/ir.md`「配線」 | `openUser` / `welcome` / `login` / `reload` の本体 | `const` / `return` / 呼び出しの文 / `match` の文、`match` のアームの `cases` は書かれた順、`default` と `none`、入れ子のレコードリテラル、フィールド参照、`return` に束縛名 |
| `docs/ir.md`「bind」 | `bind token to api.*;` | `Bind` の `namespace` `state` `target` |
| `docs/ir.md`「その名前空間から見た IR」 | `View` 全体 | `viewer` `config` `files`（単位の `in` に合ったファイル。`in` を省いているので `api.hozo` `t.hozo` `web.hozo`）`namespaces`（`api` と `web`。値は単位ごとの設定の列。`t` はどの単位の対象でもないので入らない）`ops`（すべての op）、viewer の op の `ops` は `Program` の `signature` と一致、合成した `web.getUser$in` は `web` の decls の末尾にだけある |
| `docs/ir.md`「JSON」 | 同 | `returns: null`、`body: null`、省略可能のキーは省く、空の列は `[]`、`ops` のキーは辞書順、読み戻して書いても同じバイト列 |
| `docs/check.md`「計算」手順 0（葉） | `api.login`（`prevents effects`）、`api.touch`（`keeps effects`）、`web.greet` | E / F / rel / succ |
| 同「reads」 | `api.findUser`（葉）、`api.currentName` / `tokenOrNone` / `greeting` / `guard`、`web.fetchUser`（`Call.bound`） | R: 葉は `Op.reads`。配線は match の対象、`Atom` が参照する状態、`Call.bound` の状態、呼び出し先の R。見る側によらない |
| 同「純粋」 | `api.nameOf`、`api.unauthorized`、`web.userPageVm` と、そうでない op | `Signature.pure`。見る側によらない |
| 同 手順 0（配線）: アームの `throw` と「返らない」アーム | `api.currentName`（`default => throw NotFound`）、`api.guard`（`default => unauthorized()`） | `throw` は例外がそれだけの呼び出しとして数える |
| 同 手順 0（配線）: 効果の伝播 | `signOut`、`reload` | F は呼び出しの effects を行の順に連結。`Op.effects` は `[]` のまま |
| 同 手順 0（配線）: 出どころ | `reload`（行 1 の `fetchUser` と行 2 の `api.touch` が同じ例外を投げる） | 出どころで状態が違えば `unknown`、一つなら `prevented` |
| 同 手順 0（配線）: `match` | `welcome` | 一部のアームにしかない効果は succ が `unknown` |
| 同 手順 i: `catches ... recovers` | `@offlineFallback op login`（§8.2） | 効果が不明な例外を吸収すると成功のアームでも `unknown`（DEC-5）。`throws Offline` は relations なしで `unknown` |
| 同 手順 i: `catches ... halts` | `@redirectToLogin op openUser` | 吸収した例外を E から除き、効果を F の末尾に足し、残りの例外は `prevented`、succ も `prevented` |
| 同 手順 i: `catches ... ignores` | `@ignoreTimeout op signOut` | 例外を除き、`kept` でない効果の succ を `unknown` に |
| 同 手順 i: `throws`（DEC-10） | `@external decorator remote` を `api.getUser` / `login` / `logout` に | `Program` の `signature` に `Timeout` はなく、`web` から見た `ops` にはある。`api` から見た `View` は `Program` と一致 |
| `docs/check.md`「bind」 | `bind token to api.*;`、`api.getUser(in: GetUserInput { token: Token?; id })`、`api.logout(token: Token?)`、`api.touch(id)` | Pos が model のフィールドなら `web.getUser$in` を合成して `ref`、引数そのものなら引数を除く、位置のない op はそのまま。`Call.bound` は明示していない位置だけ（BIND-3） |
| `docs/packages.md`「`plugin`（公開）」の `OpsWith` | `@t.entry` | `TestViewFromEntry`: `plugin.OpsWith(view, "t.entry")` は、viewer の op のうち `@t.entry` の付いたもの（`openUser`、`signOut`）を返す。相乗りは、これで自分の対象を探す |
| `docs/plugin-protocol.md`「要求と応答」 | `plugins/view-dump` | `TestViewDeliveredToPlugin`: 実物の `hozoc emit` がトランスパイラ `view-dump` に渡す `View` が、`internal/project` の `web` の `View` とバイト単位で同じ。`protocol` が違えば `code: "protocol"`（MAN-2）、`import` と、知らない `transpiler` は `code: "unsupported"`（MAN-5） |
| `docs/config.md`「単位の対象」、`docs/ir.md`「その名前空間から見た IR」の `NamespaceConfig` | `hozo.toml` | `View.config`: `index = 2`、`base = "view-dump"`、`dir = "gen/web"`、`options` と `seams` は空。`namespaces[api][0].dir = "gen/api"`。`View.files` は、語彙のファイル（`hozo:vocab/...`）を含む |

このコーパスに一か所しかない構文: 対象が複数の状態の分類、遷移の効果（`Effect.target` の `transition`）、状態の分類での `match` の計算（効果を持つアーム）は、専用のコーパス `tests/corpus/state` に置いた（その `README.md`。IR は `TestStateCorpusIR`、基底 `go` の出力は `TestCorpusEmitGolden` が確かめる）。ここへ足すと `view.web.json` のゴールデンと `TestViewFromEntry` の見出しごとの期待値をまとめて書き直すことになるので、分けたままにしている。

## 想定する warning

`hozoc check` の error はない。warning は一つで、意図したもの。

| 位置 | ID | 理由 |
|---|---|---|
| `web.hozo` の `@offlineFallback op login` | DEC-5 | §8.2「効果が不明な例外を `recovers` で吸収すると、成功のアームでもその効果は不明になる」の例。`succ(createSession) = unknown` を確かめるために残す |

## 書けなかったこと

文書の要求を満たすために書きたかったが、外したままの構文。

| 文書 | 書きたかった構文 | 今の実装 |
|---|---|---|
| `docs/ir.md`「その名前空間から見た IR」の `params`（入れ子の位置） | `model Auth { token: Token?; locale: string; }`、`model GetUserInput { auth: Auth; id: UserId; }`（位置 `in.auth.token`） | 外した理由（本体に組み込んでいた Go の出力が、合成した `web.getUser$in$auth` の名前を作れなかった）は、なくなった: このコーパスは Go を出さず、基底 `go` は合成した model を出さない（`docs/plugin-go.md`「出力の対象」）。足すと `view.web.json` と `TestViewFromEntry` の期待値を書き直すことになるので、まだ足していない |
| `docs/plugin-protocol.md`「起動」の終了コード 2 | マニフェストの `command = ["go", "run", ...]` のまま `protocol` の不一致を MAN-2 にする | `go run` はプログラムの終了コードを伝えず 0 以外を 1 にするので、`proc.Crashed` になる。`TestViewDeliveredToPlugin` の protocol のサブテストは `go build` した実行ファイルで起動する |
