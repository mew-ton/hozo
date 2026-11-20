# コーパス `view`

プラグインが受け取る IR（`docs/ir.md`「その名前空間から見た IR」の `View`）が文書の要求どおりであることを、一つのサンプルから端から端まで確かめるためのプロジェクト（`docs/testing.md`「コーパス」、`tests/view_test.go`、`tests/corpus_emit_test.go`）。`docs/ir.md`「正規化」の各行と `docs/check.md`「計算」の各手順を、少なくとも一か所ずつ使う。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | `[hozo] plugin_dirs = ["plugins"]` と、単位が三つ: `namespace = "api"`、`"web"`、`"page"`。どれも `transpilers = ["view-dump"]`（TS の基底はまだないので、`api` と `web` を同じ基底にする）。`api` と `page` の単位は、`web` が使う型の置き場（`docs/config.md`「型の置き場」。なければ CFG-22） |
| `t.hozo` | テストの語彙 `namespace t { decorator entry; }`。`plugin.OpsWith` が探す op の印。標準の語彙にない名前空間で、どの単位の対象でもないので出力しない（`docs/config.md`「単位の対象」） |
| `api.hozo` | サーバー側 `namespace api`。`web` から呼ばれる（呼ばれる op は `export`）。`docs/ir.md`「版」の 0.3 で足した形（継承した model の `override`、形の語、状態の型、`reads`、op の分類とケースの `as`、状態の参照、アームの値と `throw`、`never`）と、0.6 で足した形（`read` の宣言、引数を持つ例外と効果、`read` と効果の呼び出し、直に呼ぶ組み込みの効果、引数を渡す `throw`、`@repeatable`）と、0.7 で足した形（残りのケース `対象 as else`。`presence` と `variant`）と、0.8 で足した形（`else` のアーム、残りのケースを書かない型で決まる分類）はここに置く |
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
| `docs/ir.md`「型」 | `api.User`（`string??` `string[]` `Record<string, string[]>`）、`scalar AdminToken extends Token`、`UserKind.Admin<api.PublicUser>`、`fn signOut()` | `T??` → `T?`、array / record / case、void は `returns: null`、状態の形 `list` / `nullable` / `single`（型の最も外側から決まる。`docs/decisions.md`「設計書から離れた決定」S21） |
| `docs/ir.md`「正規化」`namespace` | `web.hozo` の二つの `namespace web { }` | 一つの `Namespace`、decls は出現順に連結 |
| 同 `model D extends B { override a: never; override b: T; c: U; }` | `model PublicUser extends User { override secret: never; override hash: never; }`、`model Admin extends User { override id: AdminId; @db.unique override name; ... level: int; }`（`api.hozo`） | `fields` は展開済み（元の並びのまま Override を当て、足したフィールドは末尾。デコレータも保つ）、`extends` の `base` / `removed` / `overridden`。override したフィールドの説明は override に書いたもの（なければなし）、位置は override の位置 |
| 同 形の語、`state s: T;` | `model Shapes { ids: list<UserId>; nick: nullable<string>; one: single<Token>; twice: nullable<string?>; }`、`state current: Token?;`、`state motd: string;`（`api.hozo`） | `array` / `optional` / そのままの型。`State.type` と、そこから決まる `shape` / `element` |
| 同 `export fn f()`、`reads a, b` | `export fn findUser(id: UserId): User? reads users, sessionsOf reads users { … }`（`api.hozo`） | `exported`、`reads` は `ReadRef` の列一つ（直に書いた状態は `direct`、`read` の名前は `named`。重複は最初の出現に寄せる）。`never` を返す op の `returns` は `{ kind: "never" }` |
| 同 ケースの `X as T`、`docs/ir.md`「classify」 | `classify Lookup of findUser { Found: findUser as User; Missing: findUser as else; }`、`classify SignedIn of current { Yes: current as Token; No: current as else; }`（`api.hozo`） | `subject` の `op`（`type` はその op の `returns`）、`cases[].as`（状態の分類だけ `state` を持つ）、`by`、`decidedBy` |
| `docs/ir.md`「classify」の残りのケース（`docs/check.md`「残りのケース」、`docs/types.md`「残りのケースでの対象の型」） | `classify Membership of findUser { Member: findUser as User; Stranger: findUser as else; }`、`classify Detail of Shown { Short: Shown as string; Full: Shown as else; }`、それで分ける `memberName` と `shownName`（`api.hozo`） | 残りのケースは `cases` に入り、`as` の `by` は `else`、`type` はそのケースでの対象の型（`presence` は対象の型のまま、`variant` で残りのアームが一つならそのアームの型）。`decidedBy` は `type`。アームの `narrowed` |
| 同 配線の値の位置の状態の名前、アームの本体の `none` | `tokenOrNone`、`greeting`、`guard`、`currentName`（`api.hozo`） | `Atom` の `state`（修飾して書いても同じ）、アームの本体の `atom` / `none` / `throw`、`narrowed` |
| 同 ユニオンのアームのその場の model | `union UserPage { Ready: { ... }; Missing: Unavailable; }`（`web.hozo`） | `web.UserPage.Ready`（`synthesizedFrom`）をユニオンの直後に足し、アームは `ref`。宣言済みの model はそのまま `ref` |
| `docs/ir.md`「state / exception / effect」「read」の引数 | `exception Invalid(reason: string);`、`effect fn createSession(session: Session): effects sessions.post;`、`effect fn goLogin(): effects location.put;`（`web.hozo`）、`@db.find read fn userById(id: UserId): User? reads users;`、`@db.list @db.where("revoked", false) @db.where("kind", "web") read fn sessionsOf(…)`（`api.hozo`） | 例外と効果の `params`（なければ `[]`。その場の model `api.Invalid.Value` は合成しない）、例外の `@param`、`Read` の `params` / `returns` / `state`、`read` に付けたデコレータ（対象 `read`）と、何度も付けた `@repeatable` の適用（書かれた順）、`Decorator.repeatable` |
| `docs/ir.md`「正規化」の配線の呼び出しと `throw` の引数 | `signIn` の `userById(id)`、`sessions.post(session);`、`current.post(token);`、`api.current.put(token);`、`throw Invalid("weak password")`、`keep` の `current.delete();`、`login` の `issue(in)` と `createSession(session);`（`api.hozo`） | `Call.callee` は `read` / `effect`（`named` か `direct`）/ `op`。直に呼ぶ組み込みの効果の引数名は `value`（`delete` は引数なし）。`throw` の `args` は例外の引数名に対応付く。`read` と効果の呼び出しの `bound` は空 |
| `docs/check.md`「状態に触れるもの」の「配線からの呼び出し」「節に書く要素」 | `signIn`（`effects sessions.post, current.post, current.put, current.delete`、`reads userById`）と、葉の `issue` / `verify` / `whenLive` / `mint`（`api.hozo`） | 直に呼んだ効果は `direct` で、同じ宛先の宣言（`createSession`）とは別の効果。F は行の順。R は呼んだ `read` の宛先の状態で、書き方によらない。`throw` は、引数があっても、例外がそれだけ。葉は節を持たず、運ばれるものもなければ純粋 |
| 同 ユニオンのメンバーにユニオン、`throws` にユニオン | `union AnyError { ClientError; Timeout; }`、`fn touch ... throws ClientError keeps effects` | メンバーを平らに展開、`throws` は例外の FQN に展開 |
| 同 複数行の `throws` | `fn reload ... throws api.NotFound, api.Unauthorized` と `throws Offline` | `ThrowsClause` の列（まとめない） |
| `docs/ir.md`「classify」: 二つに分かれる分類 | `classify Verified of api.PublicUser { Verified; Unverified; }`、`classify UserKind` | 両側が `cases` に入る（`[Verified, Unverified]`）。`hasDefault` と `fromCondition` のキーは、ない |
| `docs/ir.md`「配線」の `MatchArm`（版 0.8）、`docs/check.md`「アームが受けるケース」「ケースの判定」、`docs/types.md`「アームの中の対象の型」の 1 | `classify Standing of User { Fresh; Active; Idle; Dormant; Closed; }` と `standingName`（`Closed => …; else => …`）、`standingNote`（`Idle, Fresh => …`）、`arrivedName`（受けるケースが一つの `else`）、`classify Arrival of findUser { Arrived: findUser as User; }` と `onlyArrived`（`api.hozo`） | `else` のアームは `else: true` で、`cases` は、並べなかったケースを宣言の順に。名前で並べたアームの `cases` は書かれた順。受けるケースが二つ以上の `else` のアームは `narrowed` が空、一つなら、そのケースのアームと同じ。残りのケースを書かない `Arrival` も `decidedBy` は `type` |
| `docs/ir.md`「state / exception / effect」の `RowTransition`、「classify」の `absent` と `CaseTyping.by` の `none`（版 0.9）、`docs/check.md`「行の遷移」「ない側のケース」 | `classify Enrolment of User? { Fresh: User? as User; Lapsed: User? as User; Unknown: User? as none; }`、`effect fn enrol(user: User): effects users.post as Standing.Fresh;`、`rest`（`as Standing.Fresh, Standing.Active -> Standing.Idle`）、`closeUser`（`as any -> …`）、`amend`（`-> else`）、`forget`（`effects users.delete as Enrolment.Lapsed -> Enrolment.Unknown`）と、それを呼ぶ `restUser`、`forgetUser`（`api.hozo`） | 効果の `row` は、行の引数、分類、前のケース、後のケース（`post` の前は `any`、`-> else` は `to` なし）。`row` を書かない効果は、キーを持たない。`Enrolment` は `decidedBy` が `predicate` で、`absent` が `Unknown`、そのケースの `by` は `none`。ほかの分類の、値がない側の `as else`（`Membership.Stranger`）も `absent` に入る |
| 同 名前付きデコレータ | `decorator redirectToLogin = redirectOn(api.Unauthorized, goLogin);`、`@redirectToLogin` | `alias`、適用は `decorator: "web.redirectOn"`、`via: "web.redirectToLogin"`、`args` は固定した引数 |
| 同 位置で書いた引数、`{ item }`、呼び出しの引数、`5m` | `@http.status(400)`、`fetchUser({ id })`、`api.touch(id)`、`@traced(id, 5m)`、`@limits(1, 30s, false) model Query`（`web.hozo`） | `param` に対応付け、`{ id: id }`、リテラル（`int`、`duration` のナノ秒、`boolean`）。フィールドの既定値は書けない（`docs/grammar.md`「model」）ので、リテラルはデコレータの引数で確かめる |
| 同 残りの引数に渡した値、デコレータの宣言の `reads` / `effects`、op に付いたデコレータの `reads` / `effects` の節 | `decorator find(table: any state, column: string?) for op reads table;`、`decorator effects(...targets: any effect) for op effects targets reads sessions;`、`decorator both = effects(createSession, Cleanup);`、`@effects(createSession, deleteSession \| renameUser) export fn rotate(...)`、`@effects fn idle();`、`@find(users, "name") export fn findByName(...);`（`api.hozo`） | 残りの引数は `list` の `Arg` 一つ（一つずつ `ref` か `refs`。0 個なら `items` が `[]`。名前付きデコレータの固定した引数も同じ）、`Decorator.reads` / `effects`（`$param:名前`、節は連結）、`Param.rest`、`Op.carried`（`Op.reads` / `Op.effects` には足さない） |
| `docs/ir.md`「decorator」の `targets` の `effect`、`docs/grammar.md`「識別子」の予約語のデコレータ名 | `decorator into(table: any state) for effect;`、`@into(users) effect fn renameUser(id: UserId, name: string): effects users.put;`、`decorator effects(…)`、`@see find` / `@see api.effects`、`@db.table state users`（`api.hozo`） | 効果の宣言の `decorators`、予約語の名前の宣言と適用と `@see`、標準の語彙の `db.list`（対象 `read`、節なし）/ `db.insert`（対象 `effect`）/ `db.table` の形 |
| `docs/ir.md`「decorator」 | `decorator redirectOn(error: any exception, target: any effect) catches error halts target;`、`@external decorator remote throws Timeout;`、`decorator traced(key: string, within: Duration);` | `$param:名前`、`any` の `Param`、`targets` の省略は `["op"]`、`external` |
| `docs/ir.md`「デコレータの適用」 | `@t.entry @redirectToLogin @traced(id, 5m) fn openUser(id: ...)` | 書かれた順の列（最後が最も内側）、op の引数を値に取る引数は `$param:id`、`via` |
| `docs/ir.md`「配線」 | `openUser` / `welcome` / `login` / `reload` の本体 | `const` / `return` / 呼び出しの文 / `match` の文、`match` のアームの `cases` は書かれた順、アームの本体の `none`、入れ子のレコードリテラル、フィールド参照、`return` に束縛名 |
| `docs/ir.md`「bind」 | `bind token to api.*;` | `Bind` の `namespace` `state` `target` |
| `docs/ir.md`「その名前空間から見た IR」 | `View` 全体 | `viewer` `config` `files`（単位の `in` に合ったファイル。`in` を省いているので `api.hozo` `t.hozo` `web.hozo`）`namespaces`（`api` と `web`。値は単位ごとの設定の列。`t` はどの単位の対象でもないので入らない）`ops`（すべての op）、viewer の op の `ops` は `Program` の `signature` と一致、合成した `web.getUser$in` は `web` の decls の末尾にだけある |
| `docs/ir.md`「JSON」 | 同 | `returns: null`、`body: null`、省略可能のキーは省く、空の列は `[]`、`ops` のキーは辞書順、読み戻して書いても同じバイト列 |
| `docs/check.md`「計算」手順 0（配線）: 効果の呼び出し | `api.login`（`issue(in)` の後ろで `createSession(session);`。`prevents effects`）、`api.touch`（`touchUser(id);` の後ろで `verify(id)`。`keeps effects`）、`web.greet` | E / F / rel / succ。書いた `effects` と関係の節は、本体から計算したものと照合される（EFF-2、REL-4） |
| 同「reads」 | `api.findUser`（`read` を二つ呼ぶ配線）、`api.currentName` / `tokenOrNone` / `greeting` / `guard`、`web.fetchUser`（`Call.bound`） | R: 配線は、呼んだ `read` の宛先の状態、match の対象、`Atom` が参照する状態、`Call.bound` の状態、呼び出し先の R。葉は、運ばれたものだけ。見る側によらない |
| 同「デコレータが運ぶ reads と effects」 | `api.rotate`（運ばれた効果）、`api.findByName`（運ばれた状態）、`api.idle`、配線 `api.renew`、運ぶ節を持つデコレータの付いた配線 `api.renewAudited`（節を書かない）と `api.renewDeclared`（節を書く）、それを呼ぶ配線 `api.renewAgain` | 葉の F は `Op.carried.effects`、R は `Op.carried.reads` の要素の状態（葉の `Op.effects` / `Op.reads` は空）。op の `throws` の relations は、運ばれた効果に掛かる。呼び出し元へは、配線が呼んだ効果と `read` と同じに伝わる。デコレータの付いていない配線の `Op.carried` は空。配線に運ばれたものは、本体が行き着くものの後ろに並び（本体も行き着く効果は、位置を変えず、rel と succ は、本体とデコレータの分が違えば `unknown`）、書いた節はそれを含み（READS-2、EFF-2）、呼び出し元へ伝わる |
| 同「純粋」 | `api.nameOf`、`api.unauthorized`、`web.userPageVm` と、そうでない op | `Signature.pure`。見る側によらない |
| 同 手順 0（配線）: アームの `throw` と「返らない」アーム | `api.currentName`（`else => throw NotFound`）、`api.guard`（`No => unauthorized()`） | `throw` は例外がそれだけの呼び出しとして数える |
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
