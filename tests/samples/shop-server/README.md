# サンプル `shop-server`

小さな店のサーバー（ログイン、自分の情報、注文）を、サンプルを書きながら決めたこと（`docs/decisions.md`「サンプルを書きながら決めたこと（P の番号との対応）」の P1〜P39）を**当てはめた後の形**で書いたもの。サンプルとは何か、置き方、テストの使い方は [`../README.md`](../README.md)。

P1〜P39 の言語の変更（`reads`、`as`、`throw`、`never`、`override`、`export`）と、ここに出てくる五つのトランスパイラ（`go`、`go-valid`、`go-sqlite`、`go-http` と、サンプルの中のプラグインの `go-ratelimit`。設計文書がある。`go-ratelimit` は、この README）は、実装が済んでいる。**その後の決定（`docs/decisions.md`「決めたこと」91〜102: プラグインは出力を推論せず、`store` の読み書きは `db` の語彙のデコレータで言う）は、このサンプルと設計文書だけが先にあり、実装はまだない**（「まだないもの」）。単位の `out`（`gen/`）の下のファイルはすべて、「`hozoc emit` がこう出す」内容を人が書いたもので、トランスパイラを実装するときのブラックボックスのテストの仕様になる。コードはコンパイルでき、人が書いた部分（`app/`）と合わせて動く。

**生成するコードの規則の持ち主は、トランスパイラごとの設計文書**（「各トランスパイラの規則」）。この文書が持つのは、ファイルの出どころ、この仕様に固有の事実、サンプルの中のプラグイン `go-ratelimit` の規則、書いて分かったこと。期待する出力（`gen/`）は、設計文書の形のとおりに書いてある（文書と違うところは、残していない）。

P1〜P33 を当てはめる前の形（一つの名前空間 `server` を三つの単位で出力する。以下「v1」）は、コミット `ac57788` のこのディレクトリにある。

## まだないもの

このサンプルは、実装より先に書いてある（`docs/testing.md`「トランスパイラのテスト」）。今ないのは、`docs/requirements.md`「作り直しの段階」の R3 の実装:

| ないもの | このサンプルで使っているところ | 文書 |
|---|---|---|
| 言語: 効果の宣言に付けるデコレータ、予約語の名前のデコレータ、デコレータが運ぶ `reads` / `effects`、残りの引数 | `store.hozo` の `@db.insert(users) effect createUser …`、`@db.list(orders, "userId")`、`@db.effects(createOrder, notifyUser)` | `docs/grammar.md`「decorator の宣言」、`docs/check.md`「デコレータが運ぶ reads と effects」 |
| 語彙 `db` の `find` / `list` / `insert` / `update` / `delete` / `effects` | `store.hozo` の八つの op と五つの効果 | `docs/vocab.md`「`db` の読み書きのデコレータ」 |
| 基底 `go`: 効果の宛先の単位が分かれた葉を `SharedState` に置く | `gen/store/states.go` の `SharedState`、`gen/store/ops.go` の `New` の引数 `shared` | `docs/plugin-go.md`「op の置き場」 |
| `go-sqlite`: デコレータの引数だけから中身を出す。複数の効果は一つのトランザクション。`SharedTable` | `gen/store/db.go` | `docs/plugin-go-sqlite.md`「op の中身」「トランザクション」「状態をまたぐ単位の型」 |
| 版 0.5 | `plugins/go-ratelimit/hozo-plugin.toml` の `protocol` は、今の hozoc が読める `"0.4"` のまま。hozoc とリポジトリのプラグインが 0.5 になるときに、一緒に上げる（今これだけを上げると、今の hozoc が MAN-2 で断る） | `docs/ir.md`「版」 |

`TestSamples` のサブテストは、最後の形で書いてあり、足りないものを名指しして飛ばす（`../README.md`「飛ばす条件」「移行の間」）。今は、`files` と `go`（このモジュールの `go vet` と `go test`）が通り、`check` と、六つの `transpilers/*` が、文書の版（0.5）と今の hozoc の版（0.4）の違いを名指しして飛ぶ。`sample.json` に、保留を書くところはない。

`server` の出力（`gen/server`）は、今の hozoc でも確かめてある: `store.hozo` を、同じ op を今の書き方（`reads` と `effects` を op に書く。`placeOrder` の効果は一つだけ）で書いた写しに置き換えて `hozoc emit` を掛けると、`gen/server` は、`placeOrder` の `Effects:` のコメント（効果が一つになる）のほかは、期待する出力とバイト単位で同じ。`gen/store` と `gen/storemem` は、そこから、`SharedState` と、デコレータで決まる `db.go` を、文書のとおりに手で書いた。

## 入力

仕様は名前空間ごとに一つのファイル。

| ファイル | 名前空間 | 中身 |
|---|---|---|
| `type.hozo` | `type` | 共有の型（P6）: scalar、model、例外。`model PublicUser extends User { override hash: never; }`（P24〜P26） |
| `store.hozo` | `store` | DB の状態四つ（P5）、それを読む op と書く op（八つとも `export`）、読んだ結果の分類（`classify C of <op>`、`Case: <op> as T;`）。読み書きは `db` の語彙のデコレータで言う: 読む op に `@db.find` / `@db.list`、効果に `@db.insert` / `@db.delete`、書く op に `@db.effects`。op は `reads` も `effects` も書かない |
| `server.hozo` | `server` | HTTP の入口（`export` を付けた五つ）と、業務の流れ（配線）。人が書く op は七つ。`login` に `@rate.limited`（継ぎ目。P38）。トークンのない要求を断る分類 `Credential` |
| `mailer.hozo` | `mailer` | 通知を外へ届ける仕組み。`@smtp.sendMail` を付けた op `send`（`export`） |
| `smtp.hozo` | `smtp` | メールを SMTP で送る仕組みの語彙。デコレータ `sendMail`。読むトランスパイラがないので、印（何も出ない。P37） |
| `plugins/go-ratelimit/rate.hozo` | `rate` | サンプルの中のプラグイン `go-ratelimit` が提供する語彙: 例外 `LimitExceeded` と、デコレータ `limited`（「サンプルの中のプラグイン」） |

`hozo.toml` は、`[hozo]` の `plugin_dirs = ["plugins"]`（サンプルの中のプラグインの置き場）と、五つの単位。`mailer` と `smtp` には単位を置かないので、コードは出ない。

| # | 単位の `out` | `namespace` | `transpilers` | 中身を埋める者 |
|---|---|---|---|---|
| 1 | `gen/type` | `type` | `go`、`go-valid` | —（型だけ） |
| 2 | `gen/rate` | `rate` | `go` | —（語彙 `rate` の例外の型だけ） |
| 3 | `gen/store` | `store` | `go`、`go-sqlite` | `go-sqlite`: 四つの状態の単位と `SharedState` の実装（八つの op と、状態の形で決まる問い合わせのすべて） |
| 4 | `gen/storemem` | `store` | `go` | 人: `app/memstore`（四つの状態の単位と `SharedState` の実装。メモリの上） |
| 5 | `gen/server` | `server` | `go`、`go-valid`、`go-http`、`go-ratelimit` | 人: `app/leaves.go`（葉の op 七つ）、`app/mailer.go`（`mailer.send`）、`app/limiter.go`（`@rate.limited` の数え方）。`go-ratelimit`: 継ぎ目の中身 |

- 五つの単位は、どれも基底 `go` を一つ持ち、相乗りの依存（`go-http` は `go` と `go-valid`、`go-ratelimit` は `go`）をすべて並べている（`docs/config.md`「基底と相乗り」）。
- 使う型の名前空間は、どれも `go` の単位を持つ（同「型の置き場」）: `type` は単位 1、`rate` は単位 2。**`rate` は語彙の名前空間**で、その例外 `rate.LimitExceeded` を `server` の出力が使う（`login` の `Op.signature.exceptions` にある。同「型の置き場」の表の 3）ので、単位が要る。単位 2 がなければ CFG-22。
- `gen/server` の単位だけ、`[emit.options.go-http]` で `validate = true` を渡す（P16）。
- **単位 5 の継ぎ目**は `rate.limited` → `go-ratelimit`（`go-ratelimit` のマニフェストの `seams`。`docs/config.md`「継ぎ目」）。ほかの単位の継ぎ目は空。

### `export` を付けた op

別の名前空間から呼べる op には `export` を付ける（P33）。書く位置は、説明のコメントとデコレータの後ろ、`op` のすぐ前（`server.hozo:42-44` の `@http.post("/login")`、`@rate.limited`、`export op login(…)`）。付けていない op は内部で、別の名前空間から呼べばエラー。対象は op だけで、型、例外、分類、効果、デコレータは、今までどおりどこからでも参照できる（`server` は `store.Registration` や `type.Unauthorized` を、そのまま書く）。

この仕様の二十四の op のうち、`export` を付けたのは十四。付けたのは、**その名前空間の外に、呼ぶ者がいる op だけ**。

| 名前空間 | `export` を付けた op | 外の呼び手 | 内部の op |
|---|---|---|---|
| `server` | `login`、`logout`、`getMe`、`getOrders`、`placeOrder`（五つ） | `go-http` のハンドラ（`@http.*` の入口）。仕様の中に、呼ぶ名前空間はない | 配線 `issueSession`、`authenticate`、`sessionOf` と、人が書く葉 `checkPassword`、`newSession`、`toPublicUser`、`requireFresh`、`pageOf`、`newOrder`、`composeNotice`（十） |
| `store` | `findUser`、`findUserById`、`findSession`、`listOrders`、`addSession`、`removeSession`、`placeOrder`（七つ） | `server` の配線 | なし |
| `store` | `addUser` | 人が書くコード（二つの main と、テスト）。仕様の中に、呼ぶ名前空間はない | |
| `mailer` | `send` | `server` の配線 `placeOrder` | なし |

迷ったのは `store.addUser` だけ。仕様のどの名前空間も呼ばないので、「別の名前空間から呼べる」という P33 の言い方だけなら、内部でも検査は通る。`export` にしたのは、人が書くコード（起動のときにユーザーを入れる main、テスト）が、`store.New` の返す `Ops` を通して呼ぶため: `Ops` に並ぶのは `export` を付けた op だけなので、付けなければ `store.Ops` から消える。内部にしても、状態の単位の実装（`store.NewUsersTable(db).AddUser`、`memstore` の `Users().AddUser`）を直に呼べば足せるが、それでは main とテストが、保存先ごとの型に結びつく（v1 の「分かったこと」26 の「保存先を直に書き換える」に近い形に戻る）。`export` の意味が「仕様の別の名前空間」だけでないことは、「分かったこと」N66。

### 下書きから変えたこと

仕様の元は、`ac57788` までこのディレクトリにあった下書き `NEXT.md`（P1〜P19 を当てはめたもの）。そこから変えたのは次の十一。9〜11 は、`docs/decisions.md`「決めたこと」91〜102 を当てはめたもの。

1. **継承**（P24〜P27）: `model PublicUser is User omit hash;` を `model PublicUser extends User { override hash: never; }` にした（`type.hozo:41-43`）。`type.User` を `type.PublicUser` に写す葉の op `toPublicUser` を `server` に足し（`server.hozo:73`）、下書きが `User` をそのまま渡していた二か所で呼ぶ（`server.hozo:61`、`:125`）。
2. **外部の入出力**（P30）: `server` の葉 `deliver` をやめ、名前空間 `mailer` の `op send` にし、送る仕組みの語彙のデコレータ `@smtp.sendMail` を付けた（`smtp.hozo:7`、`mailer.hozo:6-9`、`server.hozo:154`）。外部の入出力をする op は、その仕組みの語彙のデコレータを持つ形に書くのが望ましい（外部のサービスなら、そのサービスの語彙。規則ではなく書き方の指針で、検査はしない）。デコレータを持つ op は純粋でない。
3. **`store.addUser` と効果 `createUser` を足した**（`store.hozo:61-62`、`:83-84`）。下書きにない変更。ユーザーを足す op がなければ、起動のときとテストが、保存先を直に書き換えるしかない（v1 の「分かったこと」26）。
4. **`getMe` の `Found` のアームは、呼び出しをそのまま書いた**（`server.hozo:125` の `Found => toPublicUser(user);`）。写した値を先に束縛する形にはできない: `user` が `type.User` になるのはアームの中だけ。
5. **セッションの期限を確かめる**: 下書きは `expiresAt` を書くだけで、誰も読まなかった（期限の切れたトークンで入れた。「分かったこと」N3）。人が書く葉の op `requireFresh` を `server` に足し（`server.hozo:105-106`）、`LoggedIn` のアームを `LoggedIn => requireFresh(session);` にした（`server.hozo:99`。今は配線 `sessionOf` の中。下の 11）。期限が過ぎていれば `type.Unauthorized`。下書きのアームは値のアーム（P14 の `LoggedIn => session;`）だったので、この仕様に値のアームはなくなった。
6. 説明のコメントを足した。
7. **`export`**（P33）: 外に呼び手のいる十五の op に `export` を付けた（上の「`export` を付けた op」）。下書きには、op を内部にする書き方がなかった。
8. **継ぎ目**（P36〜P38）: `login` に `@rate.limited` を付けた（`server.hozo:43`）。語彙 `rate` と、その継ぎ目を宣言するトランスパイラは、サンプルの中のプラグイン `go-ratelimit` が提供する（下の「サンプルの中のプラグイン」）。`login` の `throws` には `rate.LimitExceeded` を書いていない: デコレータの `throws` が足す例外は、op が書かなくても `Signature.exceptions` に入る（`docs/check.md`「計算」の手順 i）。`login` を呼ぶ仕様の名前空間があれば、そこは書くことになる（THROWS-1）。
9. **読み書きを、`db` の語彙のデコレータで言う**（決定 92〜95）: `store` の読む op の `reads <状態>` を `@db.find(<状態>, "<列>")` / `@db.list(<状態>, "<列>")` に、書く op の `effects <効果>` を `@db.effects(<効果>)` に置き換え、五つの効果に `@db.insert(<状態>)` / `@db.delete(<状態>, "<列>")` を付けた（`store.hozo`）。op は `reads` も `effects` も書かない（デコレータが運ぶ）。引数と列を、型が合うかどうかで結ぶ規則（N39）は、なくなった: 列は、仕様が名指しする。
10. **二つの効果を、一つのトランザクションで**（決定 94）: `store.placeOrder(order, notification)` を足した（`@db.effects(createOrder, notifyUser)`。`store.hozo:95-96`）。`server.placeOrder` は、注文と通知の文面を作ってから、`store.placeOrder` で二つを一緒に記録し、その後で `mailer.send` で配送する（`server.hozo:152-154`）。前は、`store.addOrder`、`mailer.send`、`store.addNotification` を順に呼んでいて、配送に失敗すると、注文だけが残り、通知の記録は残らなかった。今は、配送に失敗しても、注文と通知の記録の両方が残る（HTTP の応答は、前と同じ 500）。**なくした op**: `store.addOrder`、`store.addNotification`（呼ぶ者が、仕様にも、人が書くコードにも、いなくなった。テストは、注文と通知を `placeOrder` で足す。`export` を付けるのは外に呼ぶ者がいる op だけ、という上の「`export` を付けた op」の決まりのとおり）と、`server.notify`（中身が `mailer.send` の呼び出し一つになった）。効果 `createOrder` と `notifyUser` は残る。
11. **トークンのない要求は、`server` が断る**（決定 96）: `store.findSession` の引数を `type.Token?` から `type.Token` にした（列 `token` の型と等しい。列より広い引数は GOSQL-9）。「トークンがなければ、問い合わせずに『ない』」は、`go-sqlite` の規則ではなくなったので、仕様に書く: `server` に、値の分類 `classify Credential of type.Token? { Presented: type.Token? as type.Token; default; }` を足し（型で決まる分類。`server.hozo:78-81`）、配線 `authenticate(token: type.Token?)` は `match token by Credential` で分け、`Presented` のアームで、新しい内部の配線 `sessionOf(token: type.Token)` を呼び、`default` のアームで `throw type.Unauthorized`（`server.hozo:84-91`）。`sessionOf` が、前の `authenticate` の中身（`store.findSession` と、`store.Login` での分岐）を持つ（`server.hozo:94-102`）。HTTP から見た振る舞いは変わらない（トークンのない要求は 401 `type.Unauthorized`）。変わったのは、そのとき `store` が呼ばれないこと。

## サンプルの中のプラグイン `go-ratelimit`

継ぎ目（`docs/config.md`「継ぎ目」、Go での形は `docs/plugin-go.md`「継ぎ目」）の、いちばん小さな例。プロジェクトが、自分のデコレータに継ぎ目を持たせるには、それを宣言するプラグインをプロジェクトの中に置く。このサンプルでは `plugins/go-ratelimit/` で、`hozo.toml` の `plugin_dirs = ["plugins"]` が、そこを探索の場所にする（`docs/plugin-protocol.md`「探索」の 1）。**このプラグインの規則の持ち主は、この節**（リポジトリの `docs/` には、文書を持たない）。

| ファイル | 中身 | 今 |
|---|---|---|
| `plugins/go-ratelimit/hozo-plugin.toml` | マニフェスト: 語彙 `rate.hozo` と、トランスパイラ `go-ratelimit`（`language = "go"`、`requires = ["go"]`、`seams = ["rate.limited"]`） | ある |
| `plugins/go-ratelimit/rate.hozo` | 語彙 `rate`: 例外 `LimitExceeded`（`@http.status(429)`）と、デコレータ `limited for op throws LimitExceeded prevents effects` | ある |
| `plugins/go-ratelimit/main.go`、`emit.go` | トランスパイラの実装（下の「実装」） | ある |
| `plugins/go-ratelimit/emit_test.go`、`testdata/` | 実装の単体テストと、その入力・ゴールデン（下の「実装」） | ある |
| `gen/server/ratelimit.go` | 期待する出力 | ある |

### 語彙と、型の置き場

- **デコレータ `rate.limited`**: 付いた op を呼べる頻度を制限する。制限を超えた呼び出しは、op の中身を呼ばずに `rate.LimitExceeded`。引数はない（何回まで、どの間隔で、を仕様には書かない。数え方は、実行のときに人が渡す）。`@external` ではない: 同じ名前空間の配線から呼んでも、例外は返る。`prevents effects` は、制限を超えたときに中身を呼ばないこと。
- **例外 `rate.LimitExceeded`** は、語彙の名前空間 `rate` に置いた（`http.BadRequest`、`proc.Crashed` と同じく、仕組みの語彙が、その失敗の例外を持つ）。`@http.status(429)` は、標準の語彙 `http` のデコレータで、`go-http` が読む（材料）。Go の型は、`rate` の単位（単位 2。`gen/rate/types.go`）の基底が出し、`gen/server` の `http.go` と `ratelimit.go` は、それを import する。プロジェクトの型の名前空間（`type`）に置いて、デコレータの引数で渡す形（`@rate.limited(type.TooManyRequests)`）も書けるが、語彙だけで閉じる方を取った。
- 語彙のファイルは `plugin_dirs` の下にあるので、仕様としては読まれず（`sources` から除かれる）、`go-ratelimit` が使うプラグインである（単位 5 が、そのトランスパイラを書いている）ので、語彙として読まれる（`docs/vocab.md`「読み込み」）。

### 出力の規則

`go-ratelimit` は、Go の基底 `go` に相乗りし、`docs/plugin-go.md`「相乗りのファイル」の決まりに従う。

- **対象**は、基底の `Seams` のメソッドのうち、自分が宣言した継ぎ目のもの: `plugin/golang` の `ShapeOf(view).Seams` のうち、`Transpiler` が `go-ratelimit` のもの（この仕様では、`server.login` の `@rate.limited` の一つ）。一つもなければ、ファイルを出さない。
- **ファイル**は、N の入口のパッケージに `ratelimit.go` を一つ。先頭は `// Code generated by hozoc go-ratelimit <版>; DO NOT EDIT.`。公開の名前は `RateLimiter`、`RateSeams`、`NewRateSeams` の三つ。非公開の名前を足すなら、前置きは `rate`。
- **`RateLimiter`** は、人が実装するインターフェース: `Allow(ctx context.Context, op string) bool`。`op` は、op の FQN。呼んでよければ `true` で、数えるのもこの中。
- **`RateSeams`** は、継ぎ目の実装の型（値の型。`struct{ limiter RateLimiter }`）。`NewRateSeams(limiter RateLimiter) RateSeams` が作る。対象の継ぎ目ごとに、基底と同じ名前と形のメソッド（`plugin/golang` の `SeamMethod` と `SeamSignature`）を、対象の並びのまま持つ:

```go
func (s RateSeams) <Op>RateLimited(<op の形の引数>, body func(<op の形の引数>) <op の形の戻り値>) <op の形の戻り値> {
	if !s.limiter.Allow(ctx, "<op の FQN>") {
		return <零値>, rate.LimitExceeded{}
	}
	return body(ctx, <引数>)
}
```

- 戻り値のない op では、`return rate.LimitExceeded{}` と `return body(ctx, <引数>)`。零値は、基底の規則（`docs/plugin-go.md`「呼び出し」）。例外の型は、`rate` の型の置き場の入口のパッケージから import し（`plugin/golang` の `HomeOf`）、値で返す。
- **`var _ Seams = RateSeams{}`** は、基底の `Seams` のメソッドが、すべて自分の対象のときだけ出す（単位に、継ぎ目を宣言したほかのトランスパイラがいれば、`RateSeams` だけでは `Seams` を満たさない。そのときは、人が、それぞれの型を一つの構造体に埋め込む。`docs/plugin-go.md`「継ぎ目の実装」）。
- op の入力と出力の値は変えない（`docs/requirements.md`「書き方の指針」）: 受け取った引数を、そのまま `body` に渡し、`body` の戻り値を、そのまま返す。
- コメントは、期待する出力のとおり（`RateLimiter` と、そのメソッド、`RateSeams`、`NewRateSeams` は決まった文。メソッドは `// <メソッド名> は op <FQN> に付いた @rate.limited の継ぎ目: limiter が許さなければ、中身を呼ばずに rate.LimitExceeded。許せば、中身をそのまま呼ぶ。`）。
- オプションはない。診断は二つで、どちらも error（`emit` の応答の `errors`）: **GORATE-1** は、`[emit.options.go-ratelimit]` に、知らないキーがある（位置なし）。**GORATE-2** は、公開の名前（`RateLimiter`、`RateSeams`、`NewRateSeams`）が、N の定義のパッケージの名前（型、`New<Model>` など。`docs/plugin-go.md`「名前」）と当たる（位置は当たった宣言。対象がなく、ファイルを出さない出力には出ない）。warning は返さない。Dir(U, N) の上に `go.mod` がないことは、基底が報告し（GO-1）、このトランスパイラは何も返さない。

### 数え方は、人が渡す

制限の数え方（何回まで、どの間隔で、何ごとに）は、仕様にも、プラグインにもない。`go-ratelimit` の出力は、インターフェース `server.RateLimiter` を出すだけで、実装は人が書く: `app/limiter.go` の `app.Limiter`（op ごとに、`Window` の間に `Max` 回まで）。組み立ては、`server.NewRateSeams(limiter)` を、`server.New` の最後の引数 `seams` に渡す一行（`app/onsqlite/compose.go:27`、`app/onmem/compose.go:22`）。葉の op（`Leaves`）と同じく、渡し忘れはコンパイルエラーになる。

### 実装

`plugins/go-ratelimit/` の `main` パッケージ（`example.com/shop-server/plugins/go-ratelimit`。マニフェストの `command` が `go run` で動かす）。サンプルのモジュールの中のパッケージで、リポジトリの公開の SDK（`plugin`、`plugin/golang`）を、`go.mod` の `replace` を通して import する。`main.go` が入口、`emit.go` が中身。

- **ブラックボックスのテスト**は、リポジトリの `TestSamples` の `transpilers/go-ratelimit`: `gen/server/ratelimit.go` とバイト単位で比べる。
- **単体テスト**は `emit_test.go`（このモジュールの `go test ./...` が走らせる。`TestSamples` の `go`）。サンプルが使わない場合を、小さな入力 `testdata/<名前>/`（`hozo.toml` と `.hozo`）とゴールデン `testdata/<名前>/expected/` で確かめる: `seams`（戻り値のない op、零値の各行、当たる引数名、内部の op、継ぎ目を宣言したトランスパイラがもう一つある単位、対象のない名前空間）、`gorate1`、`gorate2`（診断）。生成したコードは、基底のプラグインをビルドして出させた基底のファイルと合わせて、一時のモジュールで `go vet` を通し、`testdata/seams/behavior_test.go` で動かす（制限を超えれば中身を呼ばず、零値と `rate.LimitExceeded` を返す。許せば、引数と戻り値をそのまま通す）。
- 入力の `View`（`testdata/<名前>/views.json`）は、リポジトリの `tests/pluginviews_test.go` が置く（`../README.md`「置き方」）。仕様を変えたら、リポジトリのルートで `go test ./tests -run TestPluginViewFixtures -update`、その後に、このディレクトリで `GOTOOLCHAIN=local go test ./plugins/go-ratelimit -update`。

本体の側に要るもの（マニフェストの `seams` と `requires`、`View.config.seams`、基底 `go` の二層と `Seams`）は、できている（「まだないもの」）。

## ファイルの出どころ

| ファイル | 出どころ |
|---|---|
| `type.hozo`、`store.hozo`、`server.hozo`、`mailer.hozo`、`smtp.hozo`、`hozo.toml` | 人が書く（入力） |
| `plugins/go-ratelimit/hozo-plugin.toml`、`rate.hozo` | 人が書く（サンプルの中のプラグインのマニフェストと語彙。入力） |
| `sample.json` | 人が書く（各トランスパイラが出すファイル。`../README.md`「sample.json」） |
| `go.mod`、`go.sum` | 人が書く（「モジュール」） |
| `gen/type/doc.go` `types.go` | `go` |
| `gen/type/validate.go` | `go-valid` |
| `gen/rate/doc.go` `types.go` | `go` |
| `gen/store/doc.go` `types.go` `ops.go` `states.go` | `go` |
| `gen/store/db.go` | `go-sqlite` |
| `gen/storemem/doc.go` `types.go` `ops.go` `states.go` | `go`（`gen/store` の同じ名前のファイルとバイト単位で同じ） |
| `gen/server/inner/doc.go` `types.go` `ops.go` `wiring.go` | `go`（内側のパッケージ: すべての定義。`server` は内部の op を持つので、出力を二つに分ける） |
| `gen/server/inner/validate.go` | `go-valid`（型にメソッドを足すので、定義のパッケージ） |
| `gen/server/doc.go` `types.go` `ops.go` | `go`（入口のパッケージ: 内側の再公開と、`export` の op だけの `Ops`） |
| `gen/server/http.go` | `go-http`（入口のパッケージ） |
| `gen/server/ratelimit.go` | `go-ratelimit`（入口のパッケージ） |
| `gen/*/.hozoc-outputs.json` | hozoc の記録（`docs/cli.md`「emit」）。その出力のすべてのファイルのパス（`gen/server` の記録は、`inner/` の下のファイルも持つ） |
| `app/leaves.go`、`app/mailer.go`、`app/limiter.go` | 人が書く: `server` の葉の op 七つの本体（`server.Leaves` を満たす型）、`mailer.send` の実装（`server.MailerOps` を満たす型）、`@rate.limited` の数え方（`server.RateLimiter` を満たす型）。保存先を知らない |
| `app/memstore/memstore.go` | 人が書く: `store` の四つの状態の単位と `SharedState` の、メモリの上の実装（`gen/storemem` の `New` に渡す型） |
| `app/onsqlite/compose.go`、`app/onmem/compose.go` | 人が書く: 二つの組み立て（`server` を `gen/store` の上に、`gen/storemem` の上に） |
| `app/cmd/shop-sqlite/main.go`、`app/cmd/shop-mem/main.go` | 人が書く: それぞれの組み立てで待ち受ける main |
| `app/handlertest/`、`app/storetest/` | 人が書く: 二つの組み立てが共に走らせるテスト（HTTP から見た振る舞い、`store` の op と状態の問い合わせの振る舞い） |
| `app/*_test.go`、`app/*/*_test.go` | 人が書く: テスト（「人が書いたもの」） |

## モジュール

- サンプルは自分のモジュール `example.com/shop-server`（`go.mod`）。生成したコードが import する `github.com/mew-ton/hozo/runtime` は `replace github.com/mew-ton/hozo => ../../..` で解決する。
- SQLite のドライバは cgo の要らない `modernc.org/sqlite`。import するのは人が書くコード（`app/cmd/shop-sqlite` と、テスト）だけ。期待する出力（`gen/`）は `database/sql` しか import しない。
- ドライバは `v1.46.1` に固定している。それより新しい版は、リポジトリが使う Go（1.24）より新しいツールチェーンを求める。`GOTOOLCHAIN=local` で動かす。

```
GOTOOLCHAIN=local go vet ./... && GOTOOLCHAIN=local go test -race ./...
GOTOOLCHAIN=local go run ./app/cmd/shop-sqlite   # http://localhost:8080。ユーザー alice@example.com / password123 を入れる
GOTOOLCHAIN=local go run ./app/cmd/shop-mem      # 同じサーバーを、メモリの上で
```

## 各トランスパイラの規則

規則の持ち主は設計文書。下の四つの節は、文書への入口と、この仕様が規則のどこを使っているかだけを書く（「分かったこと」は、この節の名前で規則を引いている）。`go-ratelimit` の規則は、上の「サンプルの中のプラグイン」。

## 基底 `go` が従う規則

持ち主は `docs/plugin-go.md`。この README が前に持っていた規則の、移した先:

| 前の項目 | 文書の見出し |
|---|---|
| 出すもの、`in` | `docs/plugin-go.md`「出力の対象」 |
| ヘッダ、ファイル、パッケージ | 同「配置」 |
| 継承した model | 同「model」 |
| 実装はインターフェースで受け取る、`Ops`、`Leaves`、状態の単位、`<Ns>Ops` | 同「実装を受け取る口」 |
| 内部の op | 同「内側のフォルダと入口」（非公開のメソッドにする形は、やめた。「分かったこと」N68、N71） |
| メソッドの形、コメント | 同「op の形」「コメント」 |
| `New`、パッケージの変数はない、「実装がない」を返す道はない | 同「New」「実装を受け取る口」 |
| 配線の `match`、アーム | 同「match」 |
| 分類 | 同「分類」 |
| デコレータ（何も出さない） | 同「デコレータ」「継ぎ目」（読む者が継ぎ目を宣言したデコレータにだけ、二層と `Seams` を出す） |

この仕様が使っている出力の形: 分けない出力（`gen/type`、`gen/rate`、`gen/store`、`gen/storemem`。内部の op がない）と、分けた出力（`gen/server`。内側 `inner` と入口）。継ぎ目は、配線の op 一つ（`login`）に一つ。

この仕様が使っていない規則（文書にはある）: `nullable` と `single` の状態、複数の状態を読む葉（`SharedState` の問い合わせ。`SharedState` の命令は使っている: `store.placeOrder`）、述語で決まる分類、union、値のアーム、`const x = match`、`bind`、`never` を返す op、既定値のほかの型、純粋な配線。継ぎ目では: 葉と `never` を返す op の継ぎ目、一つの op の二つ以上の継ぎ目（入れ子）、内部の op の継ぎ目、継ぎ目を宣言したトランスパイラが二つ以上の単位。

この仕様での、op ごとの形（`docs/plugin-go.md`「op の形」「op の置き場」「継ぎ目」を当てはめたもの）:

| op（`server`） | 種類 | `export` | 純粋か | `throws` | メソッドの形 | Go で現れるところ |
|---|---|---|---|---|---|---|
| `login` | 配線 | ある | いいえ（デコレータ `@http.post` と `@rate.limited` があり、純粋でない op も呼ぶ） | ある（自分の `type.InvalidCredentials` と、`@rate.limited` が足す `rate.LimitExceeded`） | `ctx` を取り、`error` を返す | 入口と内側の `Ops` の `Login`。**継ぎ目を持つ**ので二層: 外側は `inner/ops.go` の `ops.Login`（`o.seams.LoginRateLimited(ctx, in, o.loginBody)`）、中身は `inner/wiring.go` の非公開のメソッド `loginBody`。継ぎ目の口は `Seams` の `LoginRateLimited`、その実装は `go-ratelimit` の `RateSeams` |
| `logout`、`getMe`、`getOrders`、`placeOrder` | 配線 | ある | いいえ（デコレータ `@http.*` があり、純粋でない op も呼ぶ） | ある | `ctx` を取り、`error` を返す | 入口と内側の `Ops` のメソッド（`Logout`）。本体は `inner/wiring.go` の、`ops` の公開のメソッド。`go-http` のハンドラが、入口の `Ops` を通して呼ぶ |
| `issueSession`、`authenticate`、`sessionOf` | 配線 | ない | いいえ（純粋でない op を呼ぶ: 順に `store.addSession`、`sessionOf`、`store.findSession`） | ある | `ctx` を取り、`error` を返す | 内側の `Ops` のメソッド（`Authenticate`）だけ。入口の `Ops` にはない。本体は `inner/wiring.go` の、`ops` の公開のメソッド |
| `checkPassword`、`requireFresh` | 葉 | ない | はい（デコレータも `reads` も効果もない） | ある | `ctx` を取らず、`error` を返す | `Leaves` のメソッド（`CheckPassword`。人が実装する）と、内側の `Ops` のメソッド。`ops` のメソッドは、`o.leaves.CheckPassword(…)` を呼ぶだけ（`inner/ops.go`） |
| `newSession`、`toPublicUser`、`pageOf`、`newOrder`、`composeNotice` | 葉 | ない | はい（同じ） | ない | `ctx` を取らず、`error` も返さない（戻り値だけ） | 同じ（`Leaves` の `NewSession` と、内側の `Ops` の `NewSession`） |

`store` の八つの op は、どれも `export` を付けた葉で、読む状態か効果を、`db` の語彙のデコレータが運ぶ（op は `reads` も `effects` も書かない。`docs/check.md`「デコレータが運ぶ reads と effects」）。`store.Ops` と、実装を受け取る口の両方に、公開のメソッドとして現れる（`gen/store/ops.go:69-99`）: 七つは、読む状態か効果の宛先の、状態の単位（`<状態>State`）に、二つの効果の宛先が `orders` と `notifications` に分かれる `placeOrder` は、`SharedState` に置かれる（`docs/plugin-go.md`「op の置き場」の 3。`gen/store/states.go` の末尾）。どれも純粋でなく、`ctx` を取り、`error` を返す（`throws` は、どれにもない）。`mailer.send` は、デコレータ `@smtp.sendMail` を持つ葉なので純粋でない（`mailer.hozo:6-9`）。`server.MailerOps` と `server.StoreOps` のメソッド（`mailer.send` と、`store` の七つ）は、相手がどれも純粋でないので、`ctx` を取り、`error` を返す。`mailer` に Go の単位を置けば、そこに出る `Ops` の `Send` も同じ判定で同じ形になり、その `Ops` は `server.MailerOps` をそのまま満たす。

**デコレータごとの、出力への効き方**（`docs/plugin-go.md`「デコレータ」の四つの形）:

| デコレータ | 読むトランスパイラ | 形 | 基底の出力への効き方 |
|---|---|---|---|
| `@http.post` / `@http.get`（op） | `go-http` | 入口を作る（ハンドラ） | 純粋さだけ |
| `@http.header` / `@http.query` / `@http.body`（フィールド）、`@http.status`（例外） | `go-http` | 材料にする | なし |
| `@db`（状態） | `go-sqlite`（単位 3 だけ） | 実装を出す（`<状態>Table`） | なし |
| `@db.key` / `@db.unique` / `@db.ref`（フィールド） | `go-sqlite`（単位 3 だけ） | 材料にする | なし |
| `@db.find` / `@db.list` / `@db.effects`（op） | `go-sqlite`（単位 3 だけ） | 実装を出す（単位の型のメソッド） | 純粋さと、op の置き場（デコレータが運ぶ読む状態と効果。hozoc が `Op.carried` に置く。単位 4 でも同じ） |
| `@db.insert` / `@db.delete`（効果） | `go-sqlite`（単位 3 だけ） | 材料にする | なし |
| `@valid.*`（scalar、フィールド） | `go-valid` | 材料にする（`Validate`） | なし |
| `@rate.limited`（op） | `go-ratelimit`（単位 5） | 継ぎ目を要る | 純粋さと、二層・`Seams`・`New` の引数 `seams` |
| `@smtp.sendMail`（op） | なし（`mailer` には単位もない） | —（印） | 純粋さだけ（`server.MailerOps` の `Send` が `ctx` を取る） |

## `go-valid` が従う規則

持ち主は `docs/plugin-go-valid.md`。出すファイルは `validate.go` 一つ（定義のパッケージ: `gen/type/` と、`gen/server/inner/`）。

- この仕様で `Validate` を持つ型（同「Validate を持つ型」）: `type` の `Email`、`Password`、`User`、`PublicUser`、`OrderLine`、`Order`。`server` の `Credentials`、`LoginInput`、`LoginOk`、`PlaceOrderInput`（`AuthInput` と `GetOrdersInput` にはない）。
- `server` の `Validate` は `type` の型のメソッドを呼ぶので、`type` の単位も `go-valid` を並べている（同「出力」の GOVALID-3）。
- `server` のメソッドは内側のパッケージの型に付く。入口の型は別名なので、`go-http` は、入口の名前（`LoginInput`）のまま `in.Validate()` を呼べる。

## `go-sqlite` が従う規則

持ち主は `docs/plugin-go-sqlite.md`。出すファイルは `db.go` 一つ（入口のパッケージ）。

- この仕様の八つの op は、どれも `db` の語彙の op のデコレータを持つ（同「op の中身」）: `findUser`、`findUserById`、`findSession` は `@db.find`、`listOrders` は `@db.list`、`addUser`、`addSession`、`removeSession`、`placeOrder` は `@db.effects`。五つの効果は、`@db.insert`（四つ）か `@db.delete`（`deleteSession`）を持つ。形で決まる問い合わせは、四つの状態に `IsEmpty` と `Get` で八つ。五つの型（`UsersTable`、`SessionsTable`、`OrdersTable`、`NotificationsTable`、`SharedTable`）は、合わせて十六のメソッドを持ち、どれも基底の口（四つの状態の単位と `SharedState`）を満たす（`gen/store/db.go:45-207`）。組み立ては、五つの型を `store.New` に渡す一行（`app/onsqlite/compose.go:23-24`）。
- **`placeOrder` は、二つの効果を一つのトランザクションで行う**（同「トランザクション」。`gen/store/db.go:183-207`）: `orders` と `notifications` への二つの `INSERT` を、書いた順に実行し、どちらかが失敗すれば、どちらも残らない。効果が一つの op（`addUser` など）は、文一つ。
- 列と引数の型（同「列と引数の型」）は、どの op でも等しい（`findUser` は `email: type.Email`、`findSession` と `removeSession` は `token: type.Token`、`listOrders` は `userId: type.UserId`）。
- デコレータを付けていない op があるときの形（人が、プラグインの型を埋め込んで書き足す）は、この仕様では起きないので、見立ての型で確かめている（`app/onsqlite/store_test.go` の `partialUsers` と `humanUsers`）。
- この仕様が使っていない規則: 省略可能な列（と、`T?` の列と比べる引数）、`Duration` / `boolean` / `float` の列、`@db.update`、列のない `@db.list`、複数の `@db.key`、遷移を宛先にする効果のデコレータ、述語で決まる状態の分類の単位の型（`<C>Table`）、効果の宛先の単位が一つの、複数の効果。

## `go-http` が従う規則

持ち主は `docs/plugin-go-http.md`。出すファイルは `http.go` 一つ（入口のパッケージ）。**通信の上の形（要求と応答の JSON）の持ち主も、その文書の「通信の形」**で、`app/handlertest` が文字列のまま比べている JSON は、その実物。

- この仕様では、`@http.*` の付いた op と、`server` の `export` を付けた op が、同じ五つ。`NewHandler` が受け取る `Ops` は、ハンドラが呼ぶ op ちょうど。
- `gen/server` の単位だけが `validate = true` を渡す（同「オプション」）。`Validate` を呼ぶのは `LoginInput` と `PlaceOrderInput`（`gen/server/http.go:36,114`）。
- **`@http.status` を持つ例外は三つ**: `type.InvalidCredentials`（401）、`rate.LimitExceeded`（429）、`type.Unauthorized`（401）。並びは、経路の op の宣言順と、その `exceptions` の順（`login` の二つが先）。`rate.LimitExceeded` は、語彙 `rate` の例外で、`login` の `throws` には書かれていないが、`@rate.limited` が足すので `login` の `Op.signature.exceptions` にあり、`go-http` は、ほかの例外と同じに扱う（`gen/rate` を import する）。`go-http` は、`go-ratelimit` も、継ぎ目も知らない。
- この仕様が使っていない規則: 二つ以上の `@http.body`、必須のヘッダとクエリ、`int` 以外のクエリ、値を運ぶ例外、省略可能な戻り値。

v1 の規則から変えたのは三つ: `r.Context()` を渡す（P29）、制約の検査（P15、P16）、op を呼ぶ前の失敗の本文（v1 は `{"error":"http.BadRequest","message":"…"}`。「分かったこと」N44）。P33 で足したのは、`export` のない op についての規則一つ。

## 人が書いたもの

- **`app/leaves.go`**: `server` の葉の op 七つの本体。型 `app.Leaves` のメソッドで、`*app.Leaves` が生成された `server.Leaves` を満たす（`app/leaves.go:36`。メソッドが足りなければ、ここでコンパイルできない）。`server.New` の引数 `leaves` に、そのまま渡す。時計、トークンと注文の識別子の作り方、セッションの有効期間、1 ページの件数は `app.Leaves` のフィールドで、テストが差し替える。**どのメソッドも `ctx` を取らず、`throws` のない五つは `error` も返さない**: `server.Leaves` のメソッドの形（`gen/server/inner/ops.go:104-141`）がそう決めていて、違う形のメソッドでは満たせない（コンパイルのときの事実。`app/server_test.go` の先頭の `var` が、メソッド式の代入で確かめる）。七つとも内部の op（`export` がない）で、入口の `server.Ops` にはない。葉の本体のテスト（`app/leaves_test.go` の `TestLeaves`）は、`server.Leaves` として持った `*app.Leaves` のメソッドを直に呼ぶ。`RequireFresh` は、今が期限より前ならセッションをそのまま返し、期限ちょうどから先は `type.Unauthorized`（`app/leaves.go:62-67`）。
- **`app/mailer.go`**: `mailer.send` の実装。`*app.Mailer` が `server.MailerOps` を満たし、`server.New` の引数 `mailer` に、そのまま渡す。`ctx` を取る（`mailer.send` は `@smtp.sendMail` を持ち、純粋でない）。届けられなければ `type.NotificationFailed`。届け方（`Deliver`）はテストが差し替える。`@smtp.sendMail` は、読むトランスパイラがなく、`mailer` には単位もないので、基底は何も出さない: デコレータの意味（SMTP で送る）を実装するのは、`server.MailerOps` を満たす型を書く人。
- **`app/limiter.go`**: `@rate.limited` の数え方。`*app.Limiter` が、`go-ratelimit` の出力の `server.RateLimiter` を満たす（`app/limiter.go:34`）。op ごとに、`Window` の間に `Max` 回まで。時計（`Now`）は、テストが差し替える。`server.NewRateSeams(limiter)` に渡し、その戻り値を `server.New` の最後の引数 `seams` に渡す。数え方のテストは `app/limiter_test.go`。
- `app` パッケージは保存先を知らない: import するのは `gen/server` と `gen/type` だけ（`app/leaves_test.go` の `TestNoStorageImport`）。
- **`app/memstore/memstore.go`**: `store` の四つの状態の単位と `SharedState` の、メモリの上の実装。`memstore.New()` が空の `Store` を返し、`Users()`、`Sessions()`、`Orders()`、`Notifications()` が、その上の状態ごとの型を、`Shared()` が、二つの状態にまたがる op（`placeOrder`）の型を返す（`app/memstore/memstore.go:44-74`）。五つの型は、`gen/storemem` の `UsersState`、`SharedState` などを満たす（`app/memstore/memstore.go:36-42`）。メソッドは合わせて十六（形で決まる問い合わせ `IsEmpty`、`Get` が八つ、op が八つ）で、どれも、はじめに `ctx.Err()` を見る。**`placeOrder` の「どちらも足すか、どちらも足さない」は、人が守る**: 一度の lock の中で、失敗しうること（同じ id の注文）を先に確かめてから、二つの状態を書く（`gen/storemem` の単位には、`@db.effects` を読むトランスパイラがいない。「一つのトランザクション」の意味は、仕様のデコレータの説明と、`go-sqlite` の文書にある）。`Store` はいくつ作ってもよく、互いに混ざらない。テストのためだけの読む口は持たない。
- **二つの組み立て**: `app/onsqlite/compose.go`（表を作る `store.CreateTables(ctx, db)` と、`go-sqlite` の五つの型を渡す `store.New`。`app/onsqlite/compose.go:17-28`）と `app/onmem/compose.go`（`memstore` の五つの型を渡す `store.New`。`app/onmem/compose.go:16-23`）。どちらも、`store.New(…)` の戻り値を `server.New(leaves, mailer, s, server.NewRateSeams(limiter))` の三つ目に渡し、その戻り値を `server.NewHandler` に渡す。**二つが使う `gen/server` と `app` の葉は同じ一つのもの**で、違うのは `store` の import と、状態の単位の実装だけ。組み立ては何度呼んでもよい（パッケージの変数がない）。
- **`app/handlertest`**: HTTP から見た振る舞い。二つの組み立て（`app/onsqlite/compose_test.go`、`app/onmem/compose_test.go` の `TestHandler`）が同じものを走らせる。サブテストは、それぞれ新しいサーバーを作り、並列に走る（`t.Parallel()`）。応答の本文を文字列のまま比べるので、そこに書いた JSON が、クライアントが合わせる形になる。確かめること: パスワード違いとユーザーなしは 401 `type.InvalidCredentials`、制約に合わない入力は 400 `http.BadRequest` と違反の一覧、ログインでトークン、`GET /me`（トークンあり・なし・知らないトークン）、`POST /orders` で注文と通知が一つずつ（通知は、状態 `store.notifications` の `Get` で数える。`Server.count`）、`mailer.send` を失敗させると 500 で**注文も、通知の記録も残る**（二つは、配送の前に、一つの op で一緒に記録してある）、`GET /orders?page=…`、読めない `page` は 400、ログアウトの後の `GET /me` は 401、中断された要求は 500 で状態が変わらない、**期限の切れたセッションは、どの入口でも 401 `type.Unauthorized`**（時計を進めて確かめる。サブテスト `expired`）。
  - **`login/LimitExceeded`**（`app/handlertest/handlertest.go:160-190`）: **`/login` を、制限（`LoginLimit` 回。パスワード違いの失敗も一回と数える）を超えて呼ぶと、429 `{"error":"rate.LimitExceeded"}`**。op の中身は呼ばれない（トークンが作られない）。制約に合わない入力は、制限を超えていても 400 が先（`go-http` の検査は op を呼ぶ前で、op を呼ばないので、数えない）。時間の幅が過ぎれば、また呼べる。`@rate.limited` の付いていない op（`GET /me`）は、何度呼んでも制限されない。
  - `handlertest.RunTwo`（二つの組み立ての `TestTwoServers`）: **同じ出力の上の二つのサーバーが、一つのプロセスで同時に動き、混ざらない**。SQLite ならデータベースが二つ、メモリなら `memstore.Store` が二つで、`store.New` と `server.New` を二度呼ぶ。二つに並行に要求を送り、注文と通知の数、ログアウトが、もう一方に及ばないことを確かめる（`app/handlertest/handlertest.go:320-346`）。数え方（`app.Limiter`）も、サーバーごとに別の値。
- **`app/storetest`**: `store` の op と、状態の形で決まる問い合わせの振る舞い。**`placeOrder` は、注文と通知を一緒に足し、同じ id の注文があって失敗するときは、通知も足さない**（サブテスト `placeOrder`。二つの実装に共通の「どちらも足すか、どちらも足さない」）。二つの実装（`app/onsqlite/store_test.go`、`app/onmem/compose_test.go` の `TestStore`）が同じものを走らせる。`server` は組み立てない。サブテストは並列に走る。効果の結果は、`store.New` に渡したのと同じ、状態の単位の実装の `Get` と `IsEmpty` で確かめる（読む op のない `notifications` も読める。どちらの実装の型も、`storetest.List[T]` をそのまま満たす）。中断された `ctx` で呼ばれた op と問い合わせが `context.Canceled` を返し、四つの状態が変わらないことも確かめる。
- **`app/onsqlite/store_test.go`**: `go-sqlite` だけの事実。**`placeOrder` は一つのトランザクション**（`TestPlaceOrderIsOneTransaction`）: 二つ目の文（通知の行。`userId` が、いないユーザーで、外部キーの違反）が失敗すれば、一つ目の文（注文の行）も取り消され、その後も続けて足せる。`CreateTables` は中断された `ctx` では表を作らず、二度呼んでも足した行が残る。プラグインの型に足りないメソッドを、人が埋め込みで書き足せる（`TestHumanCompletesPluginType`。足りないままの型が `store.UsersState` を満たさないことは、リフレクションで確かめる）。`Time` と配列の列の形、`REFERENCES`。
- **`app/server_test.go`**: `server` だけ（`store` のどの出力も使わず、`server.StoreOps` を満たす型をテストが書く）。
  - メソッドの形（`ctx` と `error` の有無）を、メソッド式の代入で確かめる: 入口の `server.Ops` の五つ、`server.Leaves`、`server.MailerOps`、`server.StoreOps`、**内側の `inner.Ops` の内部の配線**（`IssueSession`、`Authenticate`、`SessionOf`）、**継ぎ目 `server.Seams.LoginRateLimited`**（op の引数の後ろに、op と同じ形の `body`）。入口の型が内側の型の別名であること、`go-ratelimit` の `server.RateSeams` が `inner.Seams` も満たすことも、代入で確かめる。
  - 要求の `context` が `server.StoreOps` のメソッドにそのまま届く、`go-http` が読めない要求と制約に合わない入力では op を呼ばない、`New` は `nil` を確かめない（`TestNilIsNotChecked`: `nil` の `leaves` と、`nil` の `seams`）。
  - **`TestReplaceOneLeaf`**: 葉の op を一つだけ差し替え（`newSession`）、内部の配線 `issueSession` を、**内側の `Ops` から直に呼ぶ**（`inner.New(…).IssueSession(ctx, user, password)`）。`issueSession` が通る別の名前空間の op（`store.addSession`）だけを書けばよい。
  - **`TestPlaceOrderRecordsThenSends`**: 配線 `placeOrder` は、注文と通知を、一つの op（`store.placeOrder`）に一緒に渡してから、配送する（`mailer.send`）。配送に失敗しても、`store.placeOrder` は済んでいる。
  - **`TestLoginIsLimitedThroughOps`**: **制限は、HTTP ではなく op に掛かる**。`server.New` が返す `Ops` の `Login` を直に呼んでも、`LoginLimit` 回を超えれば `rate.LimitExceeded`（値）が返り、中身は呼ばれない（`store` のどの op も呼ばれない）。
- **`app/inner_test.go`**: 内側のパッケージ（`gen/server/inner`）を通すテスト。
  - `TestInternalWirings`: 内部の配線 `issueSession`、`authenticate`、`sessionOf` を、`inner.New(…)` の戻り値から直に呼ぶ（パスワード違い、トークンなし、知らないトークン、期限切れ）。**トークンのない `authenticate` は、`store.findSession` を呼ばずに `type.Unauthorized`**（分類 `server.Credential` の `default` のアーム）。継ぎ目を持つ op を呼ばないので、`seams` は `nil` のまま。
  - `TestBodyBehindSeam`: 継ぎ目を持つ op の**中身だけ**を試す。継ぎ目を素通しにする `inner.Seams`（テストが書いた型 `through`）を渡すと、制限なしに、配線の本体の結果がそのまま返る。外側は、呼ばれるたびに継ぎ目を一度通る。
  - `TestSeamOwnsBeforeAndAfter`: 継ぎ目は、中身の前と後の両方を持つ（中身に渡す値と、返す値を、継ぎ目の実装が決める。基底の外側は、何も足さない）。
- **`app/both_test.go`**: 一つのプロセスで、同じ `server` を二つの保存先の上に組み立てられ、二つが混ざらない。`store` の二つの出力の `Ops` が、どちらも `server.StoreOps` をそのまま満たす（`app/both_test.go:35-36`）。**入口の `Ops` に並ぶのは `export` を付けた op だけ**（`TestOpsHoldExportedOpsOnly`。インターフェースの型のリフレクションで確かめる）: `server.Ops` のメソッドはちょうど五つ、内側の `inner.Ops` は十五のすべて、`server.Seams` は `LoginRateLimited` の一つ。`store.Ops` は八つとも持ち、`server.StoreOps`（そのうちの七つ）を満たす。内部の op を持たない出力（`store`、`storemem`、`type`、`rate`）に、内側のフォルダはない。内部の op を、入口の `New` が返す値から取り出せないことは、確かめない（約束していない。`docs/plugin-go.md`「内側のフォルダと入口」）。一つの `store` の状態の単位を、状態ごとに別の実装（SQLite とメモリ）から取れる（`TestStatesFromTwoImplementations`。二つの状態にまたがる `placeOrder` の実装は、その二つの状態と同じ保存先のものを渡す）。`gen/store` と `gen/storemem` の基底のファイルが、バイト単位で同じ。

## 分かったこと

サンプルを書いたときの記録。**N1〜N86 が引く行番号と、op の数え方（`store` の九つの op、`notify`、`addOrder` など）は、その項目を書いたときのもの**で、決定 91〜102 を当てはめた書き直し（「下書きから変えたこと」の 9〜11）で、`store.hozo`、`server.hozo`、`gen/store`、`gen/server/inner`、`app/` の行はずれている。今の形を引いているのは、この節より上と、N87 から後ろ。規則を引いている `docs/emit-go.md` は、そのときの文書（本体に組み込んでいた Go の出力）で、今は `docs/plugin-go.md` が置き換えた。「基底 `go` が従う規則」などの節の名前で引いている規則は、上の「各トランスパイラの規則」が指す文書にある。設計文書を書くときに決まったもの（N23 の、人が書く述語の置き場。N29 の `runtime` の持ち主。N30 の、単位をまたぐ依存。N32 の、オプションと依存。N44 の `http.BadRequest`。N55 の名前の衝突）は、項目を書き直さず、ここに挙げるだけにする: 決まった中身は、文書の方にある。期待する出力を文書の形にそろえたとき（P35 の内側のフォルダ、P36〜P39 の継ぎ目）に形が変わった項目（N26、N51、N52、N54、N56、N65、N68、N70、N71、N72）は、番号と元の記録を残して、「その後」を書き足した。ファイルと行の参照は、今の `gen/` と `app/` のもの。

### v1 の「分かったこと」の、その後

v1 の 63 項目（`git show ac57788:tests/samples/shop-server/README.md` の「分かったこと」。番号はそのまま）が、P1〜P33 を当てはめてどうなったか。「解決」は、その項目が言っていた不足がこの形ではなくなったもの。「残る」は、形が変わっても同じ不足があるもの。「当たらない」は、v1 の組み方にだけあった事情。N で始まる番号は「v2 を書いて分かったこと」の項目。

| # | v1 の項目 | その後 | 理由 |
|---|---|---|---|
| 1 | トランスパイラのファイルが参照するのは共有する形だけ | 解決 | P9、P17: 参照してよいのは、基底の形と、依存を宣言したプラグインの出力。ただし N31 |
| 2 | Go の名前は `View` にない | 残る | 相乗りのプラグインは、基底の名前の規則を同じに当てはめる前提のまま |
| 3 | 既定値は基底の `New<Model>()` を呼ぶ | 解決 | P9: 基底の形を前提にしてよい、と決まった |
| 4 | `New` の引数の並びが、選択を合わせた後に決まる | 解決 | P9、P11: 仕様と設定だけで決まる |
| 5 | 命令の口があるかが、ほかのトランスパイラの選択で決まる | 解決 | P11、P19: op はすべて出る。`go-sqlite` が、その実装を出せる |
| 6 | 別の名前空間の型を import するか複製するかが、`View` から決まらない | 解決 | P7、P8 |
| 7 | ヘッダの版が、hozoc のものかトランスパイラのものか | 残る | |
| 8 | `httponly` に `users` の単位がない | 解決 | P11、P19: 同じ名前空間の二つの単位（`gen/store`、`gen/storemem`）の基底のファイルは、バイト単位で同じ |
| 9 | 保存先だけの単位とハンドラだけの単位を、別の `out` に出して組めない | 解決 | P5〜P7。ただし N16 |
| 10 | 誰も呼ばない `IsEmpty` が出る | 残る | 形で決まる問い合わせは、配線が読まなくてもいつも出す（N22）。生成したコードは呼ばない。呼ぶのはテスト |
| 11 | 記録を代役の emit から作れない | 当たらない | 代役（`pick`）と `shared` は、なくなった。記録は、`files` が、その下のファイルの一覧と比べる |
| 12 | キャンセルと期限が伝わらない | 解決 | P29 |
| 13 | 認証の要る葉が、それぞれ呼んだ人を引く | 解決 | P1〜P3、P13: 配線 `authenticate` が一度引き、セッションを引数で渡す |
| 14 | トランザクションの口がない | 残る | N37 |
| 15 | 葉をまたぐ原子性は書けない | 残る | N37 |
| 16 | パッケージの変数（`States`）を守るものがない | 解決 | P32: パッケージの変数がなくなった。状態の実装は `New` の引数 |
| 17 | テストを並列にできない | 解決 | P32: サーバーを作り直すサブテストを `t.Parallel()` にした（`app/handlertest/handlertest.go:134-135`、`app/storetest/storetest.go:76-77`）。N53 |
| 18 | 一つのプロセスに二つ持てない | 解決 | P32: 同じ出力の上にも、いくつでも持てる（二つの組み立ての `TestTwoServers`）。N18 |
| 19 | 実体の持ち方が半分ずつ（`New` の引数と `States`） | 解決 | P32: すべて `New` の引数（N17） |
| 20 | 入れ忘れが起動のときに分からない | 解決 | P31、P32: 入れ忘れは、起動より前の、コンパイルのときに分かる。`nil` と書いて渡すことは防げない（N56） |
| 21 | 500 の原因がどこにも出ない | 残る | |
| 22 | ユーザーで絞ってページで切る問い合わせを引けない | 残る | 絞る方は解決（P19 と `go-sqlite` の規則）。ページと順序は N37 |
| 23 | 期限切れのセッションを消せない | 残る | 条件を付けた削除がない。期限を確かめる方は、葉 `requireFresh` を足して直した（N3）が、切れたセッションの行は残り続ける |
| 24 | 一意の違反を見分けられない | 残る | N37 |
| 25 | 人が書く葉が、保存先のトランスパイラの API に結びつく | 解決 | P1、P5、P19 |
| 26 | ユーザーを足す op が仕様にない | 解決 | 仕様に `store.addUser` を足した（「下書きから変えたこと」3）。P の決定ではない |
| 27 | 成功の応答の形 | 残る | このサンプルで決めたまま |
| 28 | 空の配列（入れ子の `nil` は `null`） | 残る | `app/leaves.go:82` で人が空の配列を作る |
| 29 | 例外の応答の形 | 残る | このサンプルで決めたまま。N48 |
| 30 | `http.BadRequest` が語彙にない | 残る | N44 |
| 31 | 500 の `internal` は FQN ではない | 残る | |
| 32 | 404 と 405 が JSON でない | 残る | |
| 33 | `Time` のゾーンを人がそろえる | 残る | `app/leaves.go:51,83` |
| 34 | `int` が JS の `number` に収まらない | 残る | |
| 35 | 省略可能の JSON の上の形 | 残る | |
| 36 | ユニオンの判別の書き方 | 残る | |
| 37 | 本文の束ね方 | 残る | |
| 38 | ヘッダの値をそのまま scalar にする | 残る | |
| 39 | 違反の位置に、通信に現れない名前が入る | 解決 | P15 と、このサンプルで決めた規則（N46） |
| 40 | 入力の model の `json` タグは通信の形ではない | 残る | |
| 41 | 読み方の甘さ（知らないキー、ないキー、本文の大きさ） | 残る | |
| 42 | `Validate` を呼ぶのは誰か | 解決 | P15、P16 |
| 43 | 例外を値の型で見分ける（ポインタで返せば 500） | 残る | 基底の `throw` は値で返す（N25）。人が書く葉は、どちらでも書ける |
| 44 | 例外をステータスに写す表が、名前空間に一つ | 残る | |
| 45 | ステータスを持たない例外が、不具合と区別できない | 残る | `app/handlertest/handlertest.go:224-235` |
| 46 | 公開の名前の衝突 | 残る | プラグインが増えた分、増えた（N49） |
| 47 | 非公開の名前の前置きが、このサンプルだけの約束 | 残る | `valid` が増えた |
| 48 | 識別子の衝突を見つける者がいない | 残る | |
| 49 | ファイル名の約束がない | 残る | `validate.go` は基底のものから `go-valid` のものになった |
| 50 | 利用者の名前から局所の名前を作らない | 残る | N43 |
| 51 | SQL の表の名前が状態の名前だけ | 残る | |
| 52 | v1 の仕様で変えたこと | 当たらない | v1 の仕様についての記録。v2 の分は「下書きから変えたこと」 |
| 53 | 配列の要素数を書けない | 解決 | P22: 配列の値を `by list` で分けられる。この仕様では使っていない |
| 54 | 成功のステータスを書けない | 残る | |
| 55 | 応答のヘッダを書けない | 残る | |
| 56 | `@db.ref` が指す列 | 残る | このサンプルで決めたまま |
| 57 | 外部キーは既定では効かない | 残る | `app/onsqlite/compose_test.go:17-18` |
| 58 | メモリの上の SQLite は接続ごとに別 | 残る | `app/onsqlite/compose_test.go:22-23` |
| 59 | 表の作り直し（移行）の形がない | 残る | |
| 60 | `db.Exec(Schema)` が複数の文を一度に実行する | 残る | 今は `db.ExecContext(ctx, Schema)`（`gen/store/db.go:41`） |
| 61 | 表は `valid` の制約を確かめない | 残る | N35 |
| 62 | `@db` は、どの保存先かを言わない | 残る | |
| 63 | `go-sqlite` の細部で決めたもの | 残る | 表の型（`Update`、`Delete`）はなくなった。残りはそのまま |

解決 19、残る 42、当たらない 2（P31、P32 で解決に変わったのは、16〜20 の五つ。P33 で変わったものはない）。

### v2 を書いて分かったこと

決定が決めていなかったので書きながら決めたこと、二つの決定が引っ張り合ったところ、理想のコードに決定が与えてくれなかったもの、v1 より悪くなったところ。それぞれ「何が起きたか → サンプルではどうしたか」。パスはこのディレクトリから。

N1〜N53 は、P1〜P30 を当てはめた形を書いたときのもの。その後の決定（直した P30、P31、P32）と仕様の直しで片づいたものは、番号を残して、見出しに「解決」と、何で解決したかを書く。N54〜N65 は、P31 と P32 を当てはめた形（実装をインターフェースで受け取る）を書いて分かったこと。N66〜N75 は、P33 を当てはめた形（`export` を付けた op だけが `Ops` に並ぶ）を書いて分かったこと。N76〜N86 は、期待する出力を文書の形にそろえ（P35 の、内側のフォルダと入口）、継ぎ目（P36〜P39）を入れて分かったこと。N87〜N96 は、決定 91〜102（プラグインは出力を推論しない。読み書きを `db` の語彙のデコレータで言う）を当てはめて分かったこと。

N1〜N96 のうち、解決は 16: N3（仕様の直し）、N11（直した P30）、N22（先にあった決定に合わせた）、N27 と N28（P31）、N10、N14、N17、N18、N53（P32）、N26（P33）、N71 と N72（P35）、N5 と N38（決定 96）、N39（決定 92）。ほかに、N36 は規則そのものがなくなり、N37 は「複数の op を一つのトランザクションにする」だけが、形を変えて解決した（どちらも見出しに書く。数には入れていない）。ほかに、N61 は前半だけが P33 で解決した（見出しに書く。数には入れていない）。N68 と N70 は、P35 で形が変わった（見出しに書く）。

#### 仕様

- **N1 `Found => toPublicUser(user)` は、先に束縛できない**。`user` が `type.User` になるのはアームの中だけ（P3）なので、写す呼び出しはアームに書くしかない（`server.hozo:108`）。レコードのフィールドに渡す方（`issueSession`）は、先に束縛した（`server.hozo:60-61`）。アームに書けるのが呼び出し一つまでなら、「絞り込んだ値を写してから、もう一つ呼ぶ」は、そのための配線の op を別に作ることになる。
- **N2 写す op を人が書く**（P27）と、**v1 より悪くなる**。v1 は基底が `UserToPublicUser` を出していた。v2 は三つのフィールドを人が写す（`app/leaves.go:56-57`）。`type.User` にフィールドを足すと、`type.PublicUser` は継承で同じフィールドを得るが、人が書いた `toPublicUser` は写さないままコンパイルが通り、零値が外へ出る。気づく仕組みはない。
- **N3（解決: 仕様に、人が書く葉 `requireFresh` を足した）セッションの期限を、誰も確かめなかった**。下書きの配線 `authenticate` は「セッションがあるか」だけで分けていた（`store.hozo:48-51`）。`newSession` は `expiresAt` を入れる（`app/leaves.go:51`）が、読む者がいなかったので、**期限切れのトークンで入れた**。v1 は葉が時計と比べていた（期限切れは 401）。仕様に書く方法として考えたのは、時計に依る述語を持つ分類（「`as` で絞り込む場合に、人が書く述語を足せるか」が決まっていない。P3）と、比較を持つ読む op（`go-sqlite` の規則で表せない。N37）。→ どちらでもなく、`server` に葉 `op requireFresh(session: type.Session): type.Session throws type.Unauthorized;` を足し、`authenticate` の `LoggedIn` のアームから呼ぶ（`server.hozo:77-89`）。本体は人が書き、時計と比べる（`app/leaves.go:62-66`）。期限ちょうどから先は、どの入口でも 401（`app/handlertest` の `expired`）。残ること:
  - **書き方は「受け取った値を、そのまま返すか、投げる葉」**。アームに書けるのは呼び出し一つ（N1）なので、確かめるだけの op も、値を返す形にした。「期限が過ぎていない」は、説明のコメントと人の本体にしかない。
  - **`requireFresh` は時計を使うが、仕様の上では純粋**（N9）で、メソッドは `ctx` を取らない。
  - **切れたセッションは消えない**（v1 の 23）。`logout` も `authenticate` を通るので、切れたトークンでは 401 になり、その行を消せない。
  - **値のアーム（P14）を使うところが、この仕様からなくなった**。`LoggedIn => session;` が、ただ一つの値のアームだった。基底の規則には形を残した（「アーム」）が、期待する出力には現れない。
- **N4 三つの分類が同じ意味**。`Registration`、`Account`、`Login` は、どれも「`T?` が `T` である場合と、それ以外」（`store.hozo:36-51`）。P20 の `by nullable` と変わらず、違うのは場合の名前だけ（P21 の指針どおり）。`match user by store.Registration` の `user` が `findUser` の結果でなければならないのか（P2 の「その op が返した値の分類」）は決まっていない。基底の出力は、どちらでも同じ（N23）。
- **N5（解決: 決定 96。`findSession` は `type.Token` を取り、トークンがない場合は `server` の分類 `Credential` が断る）`findSession(token: type.Token?)` は下書きのまま**（`store.hozo:28`）。受ける側を `T?` にしたので、`go-sqlite` に「省略可能な引数」の規則が要った（N38）。P20 で `in.token` を先に絞り込めば、`findSession(token: type.Token)` にでき、その規則は要らない。
- **N6 継承した model は、語彙のデコレータも受け継ぐ**。`type.PublicUser` のフィールドは `@db.key`、`@db.unique`、`@valid.length` を持ったまま（`type.hozo:41-43`）。外すことはできない（P25 は足すことだけ）。`go-valid` は `PublicUser` にも同じ検査を出す（`gen/type/validate.go:50-61`）。
- **N7 `logout` は、トークンがなければ 401**。下書きの配線が `authenticate` を通るため（`server.hozo:93-97`）。v1 は 204 だった。決定の帰結ではなく、下書きの仕様の変更（`app/handlertest/handlertest.go:272-274`）。

#### 純粋さと `ctx`（P29、P30）

- **N8 この仕様では「純粋な op」と「`server` で人が書く op」が同じ集合になった**。七つの葉がすべて純粋で、配線はどれも、純粋でない op（`store` の op か `mailer.send`）を呼ぶので純粋でない（「基底 `go` が従う規則」の表）。メソッドの形で「保存先に触れない」が分かる、という狙いどおり。P33 の後は、もう一つ重なる: `server` で `export` を付けた op は、`@http.*` を持つ五つの配線とちょうど同じで、残りの十（三つの配線と七つの葉）が内部。
- **N9 純粋と判定された op が、時計と乱数を使う**。`newSession`、`requireFresh`、`newOrder` は、仕様の上ではデコレータも `reads` も効果もないので純粋で、メソッドは `ctx` を取らない（`gen/server/inner/ops.go:114,125,135`）。本体は、今の時刻と新しい識別子を外から得る（`app/leaves.go:49,51,63,80,83`）。P30 の「関数の形を見れば、その op が状態や外部に触れるかが分かる」は、ここでは成り立たない。仕様に書く方法は、その op にデコレータを付けること（どのトランスパイラも読まない印でよい。`mailer.send` の `@remote` と同じ）で、付けなくても検査は通る。
- **N10（解決: P32）純粋な op も `error` を返していた**。`toPublicUser` の理想の形は `func(pkgtype.User) pkgtype.PublicUser` だが、P32 の前は、口が埋まっていないことを `*runtime.MissingError` で返すために `error` が要り、配線は、決して失敗しない写しの後でも `err` を確かめていた。P29、P30 は `ctx` だけを決めていて、`error` には触れていなかった。→ P32: 純粋で `throws` のない op は `error` を返さない。五つの葉（`newSession`、`toPublicUser`、`pageOf`、`newOrder`、`composeNotice`）は値だけを返し（`gen/server/inner/ops.go:104-140`）、配線は `publicUser := o.toPublicUser(user)` と書く（`gen/server/inner/wiring.go:38`）。「口が埋まっていない」は、実行のときには起きなくなった。この形で分かったことは N60。
- **N11（解決: 直した P30）外部の入出力を「純粋でない」と書く方法が、置き場しかなかった**。直す前のサンプルは「別の名前空間の op の口は、相手が純粋かどうかによらず `ctx` を取る」と決めていた。`mailer.send` は、宣言だけを見ればデコレータも `reads` も効果もない純粋な葉で、`ctx` を取るのは `server` から見た口だけだった（`mailer` に Go の単位を置けば、`ctx` を取らない `Send(notice)` が出て、口に入らなくなる形）。直した P30 は、純粋かどうかを op そのものの性質とし（どの名前空間から見ても同じ。口も相手の op の判定に従う）、別のプロセスや外部にある op には受け渡しのデコレータが付く、とした。→ `send` に、送る仕組みの語彙のデコレータ `@smtp.sendMail` を付けた（`mailer.hozo:7`。語彙は `smtp.hozo`）。`mailer.send` は純粋でなくなり、`server.MailerOps` の `Send` は `ctx` を取る（`gen/server/inner/ops.go:149`）。理由が「別の名前空間だから」から「相手が純粋でないから」に変わった。`mailer` に Go の単位を置いても、同じ判定で `ctx` を取る `Send(ctx, notice)` が出る。残ること: `smtp` の語彙を提供するプラグインがまだないので、サンプルの中で宣言している（今はどのトランスパイラも読まない印。プラグインができれば、`send` の中身を出せる）。付け忘れても検査は通り、op は純粋と判定されて `ctx` が消える（N9 と同じ）。`throws` があるので `error` は残る（P32）。生成したメソッドのコメントに、純粋でない理由（どのデコレータか）は出ない（`gen/server/inner/ops.go:146-149`）。
- **N12 宣言を一つ変えると、メソッドの形が変わる**。純粋な葉に、デコレータ（どのトランスパイラも読まない印でも）、`reads`、効果のどれかを足すと、その op のメソッド（`Leaves` か状態の単位と、`export` を付けた op なら `Ops`）に `ctx` が増える（`throws` がなかった op なら、`error` も増える）。配線なら、呼び出し先の一つが純粋でなくなるだけで同じことが起きる（伝わる）。人が書いた本体と、`Ops` を呼ぶ人のコードは、コンパイルが通らなくなる（気づける）。内部の配線の形が変わっても、人のコードには何も起きない（呼ぶのは、生成し直される配線だけ。P33）。別の名前空間のインターフェース（`<Ns>Ops`）のメソッドも、相手の op の判定に従うので同じように変わるが、相手の出力の `Ops` をそのまま渡している組み立ては、両方が同時に変わるので、そのまま通る（P31）。通らなくなるのは、人が書いた実装（`app.Mailer`、`app/memstore`）。`throws` を足したときのことは N60。
- **N13 純粋さの判定を使うのは、基底だけだった**。`go-http` が呼ぶ op は `@http.*` を持ち、`go-sqlite` が中身を出す op は `reads` か効果を持ち、形で決まる問い合わせは状態を読むので、どれも必ず `ctx` を取り、`error` を返す。相乗りのプラグインは、判定の結果を知らなくても正しい呼び出しとメソッドを書ける（`gen/server/http.go:40,55,69,92,118`、`gen/store/db.go:55-189`）。
- **N14（解決: P32 の後の `go-sqlite` の規則）`ctx` を受けない入口があった**。P32 の前の `SetupDB(db *sql.DB)` は `ctx` を取らず、表を作る文は中断できなかった。P29 が決めているのは op の関数だけ。→ `SetupDB` は、表を作ることと、実装を `States` に入れることの二つをしていた。後ろの方がなくなった（実装は、人が `New` に渡す）ので、残った方を `CreateTables(ctx context.Context, db *sql.DB) error` にした（`gen/store/db.go:40`。`app/onsqlite/store_test.go` の `TestCreateTables`）。
- **N15 基底が使う名前が増えた**。引数名 `ctx`、パッケージ名 `context` と `pkgtype`。仕様の op の引数が `ctx` という名前なら、末尾に `_` を付ける規則（`docs/emit-go.md`「名前」）に足すことになる。

#### 基底の形

- **N16 型を共有できるのは、共有の名前空間に置いた型だけ**。`gen/store` と `gen/storemem` の `Ops` が、どちらも同じ `server.StoreOps` を満たすのは、引数と戻り値の型がすべて `gen/type` のものだから（`app/both_test.go:30-36`）。`store` が自分の model を持てば、二つの出力は別々の Go の型を持ち、`server` が import するのは一方だけなので、もう一方の `Ops` は満たさない。「同じ名前空間を二つの単位で出すなら、境を越える型は共有の名前空間に置く」は、P5、P6 では指針だが、この組み方では守らなければ組めない。相手の `Ops` をそのまま渡せること（P31）も、同じ前提の上にある。
- **N17（解決: P32）何がパッケージの変数になるかは、名前空間の境で決まっていた**。P32 の前、`server` は状態を持たないので、すべてが `New(leaves, deps)` の値で、`store` は九つの op がすべて状態の単位にあり、`New()` は引数を取らず、実装はパッケージの変数 `States` にあった。→ P32: パッケージの変数はない。`server` も `store` も、実装はすべて `New` の引数（`gen/server/inner/ops.go:205`、`gen/store/ops.go:61`）。
- **N18（解決: P32）一つのプロセスに二つの `store`**。P32 の前は、二つの出力（`gen/store`、`gen/storemem`）が別のパッケージで、それぞれの `States` を持つので、同じ `server` を両方の上に同時に組み立てられた（今も: `app/both_test.go:112-139`）が、同じ出力の上に二つ（SQLite の DB を二つ、メモリの `Store` を二つ）は持てなかった: 準備が `States` を上書きした。→ P32: 同じ出力の `New` を何度呼んでもよい。二つの組み立ての `TestTwoServers` が、SQLite のデータベース二つ、`memstore.Store` 二つの上で、二つのサーバーに並行に要求を送る（`app/onsqlite/compose_test.go:53`、`app/onmem/compose_test.go:33`）。
- **N19 二つの出力は、どちらも `package store`**。パッケージ名は名前空間から決まり、`out` によらない。両方を import するファイルは、一方に別の名前を付ける（`app/both_test.go:23`）。片方だけでも、import パスの最後（`storemem`）と名前（`store`）が違う（`app/onmem/compose.go:11`）。
- **N20 パッケージ名 `pkgtype`**。共有の型の名前空間を、下書きのとおり `type` と名付けると、Go では `pkgtype.User`、ディレクトリは `gen/type`、import には名前を書くことになる（`gen/server/inner/types.go:5`）。利用者がパッケージ名を選ぶ方法はない。通信の上の例外の名前も `type.Unauthorized` になる（N48）。名前空間を `types` や `model` と名付ければ起きない。
- **N21 状態の単位に置く op**。「`reads` が一つの葉は、その状態の単位の問い合わせ」は、命令の規則（`docs/emit-go.md`「state」）と対に決まった（`gen/store/states.go:13-62`）。単位の中の並びは、形で決まる問い合わせ（N22）、op の問い合わせ、命令。決定が言っていない二つは、ここで決めた: 効果と `reads` の両方を持つ葉は効果の宛先の単位、複数の状態を読む葉はインターフェース `SharedState`（どの状態のものでもなく、実装するのは状態の持ち主なので、`Leaves` ではない。`New` の引数は、状態の単位の後ろの `shared`）。`SharedState` と `shared` という名前は、`shared` という名前の状態と当たる。
- **N22（解決: 形で決まる問い合わせは、いつも出す）状態を読む口（`IsEmpty`、`Get`）は、配線が読むときだけ出す、と決めていた**。生成したコードが呼ぶ口だけを出す、という規則で、帰結は「効果の結果を、読む op のない状態では確かめられない」だった: `notifications` には `addNotification` しかないので、テストは `go-sqlite` の表を SQL で直に数え、メモリの方は人がテストのための読む口を足していた。これは、先にあった決定（`docs/emit-go.md`「state」: 状態の単位は、配線が読まなくても、形で決まる問い合わせをすべて持つ。テストが効果の結果の判定に使う。`docs/decisions.md` の S6 も同じ向き）に反していたので、その決定に合わせて直した。→ 基底は、どの状態の単位にも `IsEmpty` と `Get` を出す（`gen/store/states.go:15-17`、`:58-60`）。`go-sqlite` と `memstore` がそれを実装し（`gen/store/db.go:178-185`、`app/memstore/memstore.go:90-108`）、テストは、その状態の単位の実装の `Get` で読む（`app/onsqlite/compose_test.go:40`、`app/onmem/compose_test.go:20`）。SQL を直に書くテストと、`memstore` の読む口はなくなった。直して分かったこと:
  - **`Ops` に載らない**。形で決まる問い合わせは op ではないので、テストは、`New` に渡した状態の単位の実装のメソッドを直に呼ぶ（`go-sqlite` の型は、同じ `db` から作り直した値でも同じ表を読む）。P32 の後は、実装していなければその型が状態の単位を満たさないので、「埋めていない口」はない。
  - **共通のテストへの渡し方**（解決: P32）。P32 の前は、二つの出力の `StateSet` が別の型で、共通のテスト（`app/storetest`）へは、口を一つずつ渡していた。今は、どちらの実装の型も、共通のテストのインターフェース（`storetest.List[T]`: `IsEmpty` と `Get`）をそのまま満たすので、`store.New` に渡したのと同じ値を渡す（`app/onsqlite/store_test.go:24-29`、`app/onmem/compose_test.go:44-48`）。
  - **`list<T>` の `Get`** は、`docs/emit-go.md`「state」の表にない（そこでは `list<T>` は `IsEmpty` だけ）。状態を名前で読むと `T[]` の値になる（P4、P22）ことから足した。
  - **`Get` は、すべての要素を返す**。件数や条件で絞る書き方はない（N37）。
- **N23 分類について、基底は何も出さない**、と決めた（規則は「基底 `go` が従う規則」の「分類」）。`store.Registration` などの名前は、`gen/server/inner/wiring.go:17,51,89` のコメントにだけ残る。決まっていないこと: 絞り込む場合と、人が述語を書く場合が混ざった分類の形、人が書く述語をどのインターフェースに置き、`New` のどの引数で受け取るか（P32 は葉の op と状態の単位について決めた。P6 は分類の置き場を自由にした）。
- **N24 `match` は `nil` の検査になった**。`return match` はアームごとの `return`（`gen/server/inner/wiring.go:17-23`）、絞り込んだ値は使う位置で `*user`、`*session`（`:24`、`:58`、`:96`）。値を束縛する `match`（`const x = match …`）はこの仕様になく、形を決めていない。
- **N25 `throw` は例外を値で返す**（`gen/server/inner/wiring.go:23,57,95`）。v1 の「分かったこと」43（値かポインタか）は、生成したコードについては決まった。値を運ぶ例外の `throw` は、この仕様に出てこない（P13 の未定のまま）。
- **N26（解決: P33）`Ops` に、業務の途中の op がすべて載っていた**（P11）。P33 の前は、`Authenticate`、`IssueSession`、`Notify` と、七つの葉が、`login` などの入口と同じ `Ops` のメソッドだった。`go-http` の `NewHandler(ops Ops)` も十五のメソッドを受け取り、五つだけ呼んでいた。入口とそうでない op を分ける書き方が、仕様になかった。→ P33: 外から呼べる op に `export` を付ける。`server.Ops` は、`export` を付けた五つの入口だけ（`gen/server/ops.go:12-46`）。途中の十は、入口の `Ops` にはない（`app/both_test.go` の `TestOpsHoldExportedOpsOnly`）。その後（P35）: 途中の十を、`ops` の非公開のメソッドにする形はやめ、内側のパッケージ `gen/server/inner` の `Ops`（十五のすべて）に置いた（N68、N71）。P31、P32 の後の形での続きは N61、この形で分かったことは N66〜N75。
- **N27（解決: P31）`Deps` に入るのは、`Ops` のメソッドだけだった**。P31 の前、`server.StoreFindUserFunc` と `store.FindUserFunc` は、中身が同じでも名前の違う関数の型なので、`States` のフィールドは型を変換しなければ `Deps` に入らなかった（メソッドの値は名前のない関数の型なので、そのまま入った）。→ P31、P32: 関数の型も、フィールドに入れる行もない。相手の `Ops` の値を、そのまま渡す。
- **N28（解決: P31）組み立ては、口の数だけの行を人が書いていた**。P31 の前、`server.StoreOps` の八つのフィールドを一つずつ入れる行が、二つの組み立てに同じ形で二度あった。→ P31: `server.StoreOps` はインターフェースで、`store.New(…)` の戻り値をそのまま渡す（`app/onsqlite/compose.go:25`、`app/onmem/compose.go:21`）。`server.StoreOps` にない op（`addUser`）を `store.Ops` が余分に持っていても渡せ、足りなければコンパイルできない。「実装の単位は関数の型」（`docs/emit-go.md`「実装の単位」）は、人が実装するもの（葉の op、状態の単位）についても、P32 でインターフェースのメソッドになった。
- **N29 `runtime` の持ち主**。`go-valid` の出力と `go-http` の出力が使う `runtime.ValidationError` は、このリポジトリの `runtime` パッケージにある。本体が特定の言語を知らない（P9）なら、`runtime` は基底 `go` のもので、`ValidationError` は `go-valid` のものになるはずだが、置き場は決まっていない。P32 の後、**`runtime.MissingError` を使う出力は、この仕様にはない**（P32 の前は、`Kind` が `"leaf"`、`"state"`、`"dep"` のものを基底が返していた）。`runtime` には、ほかの `Kind`（人が書く分類の述語の `"classifier"`、デコレータの `"wrapper"`、制約の `"constraint"`）のためにも残っている。それらを P32 と同じにインターフェースで受け取るかは、この仕様に出てこないので書いていない（N23）。

#### `go-valid`

- **N30 単位をまたぐ依存**。`server` の `Validate` は `gen/type` のメソッドを呼ぶ（`gen/server/inner/validate.go:15-16,36,47`）ので、`type` の単位にも `go-valid` を並べなければコンパイルできない。P18 が言うのは「一つの単位の `transpilers` に依存をすべて書く」で、別の単位に何を並べるかは範囲の外。型の名前空間を出す Go の単位がない（別の言語、単位なし）ときは、メソッドでは書けず、使う側のパッケージに関数を出す形が要る。
- **N31 `go-http` は、`go-valid` の規則を知っている**。入力の model が `Validate` を持つか（`LoginInput` と `PlaceOrderInput` は持ち、`AuthInput` と `GetOrdersInput` は持たない）を、`go-http` が `go-valid` と同じ規則で決める（`gen/server/http.go:36,114` で呼び、`:54,68,91` では呼ばない）。「依存を宣言する」（P17）だけでは足りず、依存先の出力の形を再現している。v1 の「分かったこと」2 と同じ構図。
- **N32 依存がオプションで変わる**。`go-http` が `go-valid` に依存するのは `validate = true` のときだけ。P17 の依存は、オプション（P16）によらず宣言するのか、決まっていない。
- **N33 メソッド名 `Validate` が、フィールドと当たる**。model に `validate` というフィールドがあれば、Go では同じ型にフィールドとメソッド `Validate` ができ、コンパイルできない。
- **N34 誰も呼ばない `Validate`**。結果の型 `LoginOk` にも出る（`gen/server/inner/validate.go:34-41`）。補助の関数 `validAppend` は、パッケージごとに同じものが出る（`gen/type/validate.go:88`、`gen/server/inner/validate.go:56`）。
- **N35 `store` は制約を確かめない**。`gen/store` の単位は `go-valid` を並べず、`go-sqlite` は足す前に確かめない。`store.addUser` に、名前が空のユーザーを渡せば入る。確かめるのは HTTP の境だけ（P15 の「いつ確かめるかは、境界を持つプラグインが決める」のとおり）。

#### `go-sqlite`

- **N36（決定 92〜94 で、当たるかどうかを見る規則そのものがなくなった。今は、八つの op のすべてに、仕様がデコレータを付けている）規則は、この仕様の九つの op に足りた**（「`go-sqlite` が従う規則」の末尾）。人が `store` について書いたのは、組み立ての二つの文（表を作る `CreateTables` と、四つの型を `store.New` に渡す行。`app/onsqlite/compose.go:18-22`）だけ。
- **N37（「複数の op を一つのトランザクションにする」は、決定 94 で形を変えて解決: 一緒に行うものは、一つの op の `@db.effects` に並べる。ほかは残る）規則で表せないもの**。
  - **順序**: `listOrders` の「足した順」は説明のコメントにしかない（`store.hozo:30`）。規則が `ORDER BY rowid` と決めているだけで（`gen/store/db.go:155`）、「新しい順」は書けない。
  - **件数と位置**: ページで切るのは、すべて読んだ後の `pageOf`（`gen/server/inner/wiring.go:107-111`）。
  - **比較**: 「期限が今より後」。読む op では書けないので、読んだ後の葉 `requireFresh` で確かめる（N3）。
  - **一意の違反**: `addUser` で `email` が重なると、ドライバのエラーがそのまま返り、HTTP では 500 になる。仕様の例外に写す書き方がなく、`db.go` はドライバを import しないのでエラーを見分けられない（`app/storetest/storetest.go:96-101` は「エラーである」ことだけ、`app/onsqlite/store_test.go:179-181` は文言を見ている）。人が書き足せる場所はできた: `UsersTable` を埋め込んだ型で `AddUser` を置き換えれば、ドライバのエラーを見分けて写せる（N63）。写す先の例外は、仕様の `addUser` に `throws` がないので、ない。
  - **書き換え**（`put`）、**複数の状態にまたがる読み取り**: この仕様にないので、規則を決めていない。
  - **複数の op を一つのトランザクションにする**: `placeOrder` の `addOrder` と `addNotification` は別々の文（`gen/server/inner/wiring.go:125`、`:147`）。配線が op の間に渡すのは仕様の引数と `ctx` だけで、`ctx` に値を載せない（P29）ので、トランザクションを運ぶ道がない。v1 では一つの葉の中に書けた処理が、配線になった分、原子的に書ける範囲は狭くなった。状態ごとの型は、それぞれが `*sql.DB` を持つ（`*sql.Tx` を受け取る作り方はない）。
- **N38（解決: 決定 96。規則をなくした）省略可能な引数の規則は、下書きの書き方のために足した**（N5）。「値がなければ、問い合わせずに『ない』」（`gen/store/db.go:109-111`）。SQL の `= NULL` がどの行にも合わないのと同じ結果にした。
- **N39（解決: 決定 92。列は、仕様が `@db.find(users, "email")` のように名指しする）引数と列を、型で結ぶ**。`findUser(email: type.Email)` が列 `email` に、`listOrders(userId: type.UserId)` が列 `userId` に結ばれるのは、フィールドごとに別の scalar を宣言しているから。`name: string` のような核の型の引数は、`string` のフィールドが二つあれば（`type.User` の `name` と `hash`）結べない。引数の名前は見ていない（`findUserById` の引数は `id`、`listOrders` の引数は `userId` で、どちらも型は `type.UserId`）。
- **N40 `delete` で行が減らなくても、エラーにしない**（`removeSession`。`gen/store/db.go:127-130`）。組み込みの効果 `delete` は「列数が減る」変化だが、消す行がないときを区別する書き方はない。
- **N41 `go-sqlite` の公開の名前**。P32 の前は `Schema` と `SetupDB` だけだった（v1 の表の型 `Tables`、`Insert`、`Get` などは、人が呼ぶ理由がなくなったので出さなくなっていた）。P32 の後は `Schema`、`CreateTables` と、状態ごとの `<状態>Table`、`New<状態>Table`（この仕様で、合わせて十）。状態ごとの型が戻ったが、v1 の表の型とは違い、メソッドは基底の状態の単位のもの（形で決まる問い合わせと、仕様の op）だけで、`go-sqlite` が自分で決めた読み書きはない。仕様に op のない読み書きのうち、状態の値をすべて読むことは、形で決まる問い合わせでできる（N22）。それで足りないもの（条件を付けた読み取り、書き換え）は、人が SQL を書く。
- **N42 `@db.key`、`@db.unique`、`@db.ref` の意味は、`go-sqlite` の出力にしかない**。`gen/storemem` の単位では、どのトランスパイラも `@db` を読まない（印）。仕様は「`addUser` は、`email` が重なれば失敗する」と言っていないので、メモリの上の実装がそうするかは人が決める。→ キーと一意は合わせ（`app/memstore/memstore.go:18`、`:149-150`）、参照先の検査は合わせていない（`app/onsqlite/store_test.go:172-181` の `TestRef` は SQLite だけ）。「同じ `store` の二つの実装」が同じ振る舞いをすることは、仕様からは言えない。状態の単位のインターフェースが保証するのは、メソッドの形までで、振る舞いではない。
- **N43 プラグインごとの局所の名前**。状態ごとの型のメソッドの引数は、基底と同じ規則で仕様の引数名から作る（`email`、`token`）。`go-sqlite` が自分で使う名前（レシーバの `t`、`args`、`err`）と当たれば、基底と同じく末尾に `_` を付けることになるが、当たる名前の一覧はプラグインごとに違う。形で決まる問い合わせのメソッド名は、P32 の前は `go-sqlite` が決めていて（`UsersGet`）、同じ名前になる op（`usersGet`）と当たった。今は基底のメソッド名のまま（`Get`）なので、衝突は基底の側のものになった（N58）。

#### `go-http` と JSON の形

- **N44 op を呼ぶ前の失敗を、例外の形にそろえた**。P15 の「失敗は、そのプラグインの語彙の例外で伝える」に合わせ、`{"error":"http.BadRequest","value":{"message":…,"violations":[…]}}` とした（`gen/server/http.go:129-152`）。v1 は `{"error":"http.BadRequest","message":…}` で、例外の形（`error` と `value`）ではなかった。クライアントのトランスパイラは、ほかの例外と同じ規則で読める。**`http` の語彙には、`BadRequest` も、それが運ぶ値の model もまだない**（`vocab/http.hozo` にあるのは `ServerError` と `NetworkError`）。
- **N45 `http.BadRequest` を投げるかどうかが、設定で変わる**。制約の違反で投げるのは `validate = true` のときだけ。呼ぶ側の名前空間が計算する `throws`（`@http.post` の宣言が持つ例外）は仕様だけから決まるので、単位のオプション（P16）で増える例外を表せない。
- **N46 違反の位置を、通信の上の場所にした**。`Validate` の `Path`（`lines[1].quantity`）の先頭を、そのフィールドを読む場所に置き換える（`body[1].quantity`。`gen/server/http.go:37,115,193-202`）。v1 の「分かったこと」39 はこれで解決した。
- **N47 400 が 401 より先になる**。制約を確かめるのは op を呼ぶ前なので、トークンのない `POST /orders` でも、本文が制約に合わなければ 400（`app/handlertest/handlertest.go:210-213`）。v1 は葉が先に認証していた。順序を選ぶ書き方はない。
- **N48 例外の FQN が、型の置き場を通信に出す**。`{"error":"type.Unauthorized"}`（`gen/server/http.go:220,224`）。v1 は `server.Unauthorized`。例外を別の名前空間に移すと、クライアントが見る名前が変わる。

#### 名前の衝突

- **N49 プラグインが足す公開の名前**。基底: `New`、`Ops`、`Leaves`、`SharedState`、`<Ns>Ops`、`<状態>State`、`Effect<効果>`、`New<Model>`（P31、P32 でなくなったもの: `Deps`、`States`、`StateSet`、`<Op>Func`、`<Ns><Op>Func`）。`go-valid`: メソッド `Validate`（N33）。`go-sqlite`: `Schema`、`CreateTables`、`<状態>Table`、`New<状態>Table`。`go-http`: `NewHandler`。利用者の宣言と当たる例: model `Schema`、model `UsersTable`（状態 `users` の型と当たる）、既定値を持つ model `Handler`（基底の `NewHandler()` と `go-http` の `NewHandler` が当たる）、既定値を持つ model `UsersTable`（基底の `NewUsersTable()` と `go-sqlite` の `NewUsersTable` が当たる）。プラグインをまたぐ衝突を見つける者がいないのは、v1 の「分かったこと」48 のまま。その後: 基底に `Seams` が加わった（継ぎ目を持つ op があるときだけ）。サンプルの中のプラグイン `go-ratelimit` は `RateLimiter`、`RateSeams`、`NewRateSeams` を足す。当たる例: model `Seams`、既定値を持つ model `RateSeams`（基底の `NewRateSeams()` と当たる）。
- **N50 非公開の名前とファイル名**。前置きは `valid`、`db`、`http`、ファイルは `validate.go`、`db.go`、`http.go`。どれも、プラグインどうしの約束がないまま、当たらないように選んだだけ（v1 の「分かったこと」47、49）。その後: 標準の相乗りの分は、`docs/plugin-go.md`「相乗りのファイル」の表が約束になった。表にないトランスパイラ（`go-ratelimit` の `ratelimit.go` と前置き `rate`）は、表を避けて自分で選ぶ。プロジェクトの中のプラグインどうしが当たらないことを確かめる者は、今もいない（ファイルが当たれば CFG-5、名前が当たればコンパイルエラー）。

#### 設定とテスト

- **N51 今の hozoc は、この `hozo.toml` を読めない**。`[emit.options.go-http]` は、単位の知らないキー `options` として CFG-7 になり、その単位（`gen/server`）は読まれない。はじめは、`tests/samples_test.go` が、保留のあるサンプルに限り、このテーブルを除いた写しから単位を読み、`check` は、仕様（`.hozo`）だけを写して言語の検査だけを行っていた。その後（P36）: テストを最後の形にした。`hozo.toml` を書き換えて読む道はなくし、今の hozoc が読めない間は、単位が要るサブテスト（`files`、`transpilers/*`）を、読めない理由（CFG-7 の文言）を名指しして飛ばす。`check` は、足りないものがある間だけ、言語の検査に落とす（`../README.md`「テスト」）。読めないものは増えた: サンプルの中のプラグインのマニフェストの `requires` と `seams`（MAN-4）。
- **N52 「共有する形」と「トランスパイラのファイル」の区別がなくなった**（P9）。`gen/` のどのファイルも、どれかのトランスパイラのもの。`sample.json` の `select`（代役の規則）は書いていない。その後: `select` と `pending` は、`sample.json` から、書けるキーとしてもなくなった（`../README.md`「sample.json」）。
- **N53（解決: P32）テストは並列にできなかった**。`store.States` がパッケージの変数だったので、サーバーを作り直すサブテストは順に走らせていた。→ P32: サーバーも `store` も、作るたびに別の値で、何も共有しない。`app/handlertest` と `app/storetest` のサブテストと、その上のテストを `t.Parallel()` にした（`app/handlertest/handlertest.go:126-135`、`app/storetest/storetest.go:60-77`）。`go test -race` で通る。

#### 実装をインターフェースで受け取る形（P31、P32）

- **N54 `New` の位置引数は、状態と名前空間の数だけ並ぶ**。`store.New` は四つ（`gen/store/ops.go:61`）、`server.New` は三つだった（その後、継ぎ目の `seams` が最後に加わって四つ。`gen/server/inner/ops.go:205`。N79）。状態が二十あれば、引数は二十。Go では、呼ぶ側に引数の名前を書けないので、何番目がどの状態かは、型だけが頼りになる。
  - **取り違えは、型が違えばコンパイルエラー**。ただし、**メソッドの並びが同じ二つの状態**（同じ形、同じ要素の型で、どちらにも、それを読み書きする葉の op がない。例: `state drafts: list<type.Order>` と `state archive: list<type.Order>`）は、インターフェースの中身が同じになり、入れ替えて渡してもコンパイルが通る。フィールドに名前で入れる形（P32 の前）では、起きなかった取り違え。
  - **並びは宣言順**なので、仕様で状態の宣言を入れ替えると、`New` を呼ぶすべての行が変わる。状態か、呼ぶ名前空間を一つ足しても同じ（こちらは、渡し忘れをコンパイルエラーにする、という狙いどおり）。別の名前空間を辞書順にしたのは、配線の本体を書き換えただけで並びが変わらないようにするため。
  - → サンプルでは、四つを一行に並べた（`app/onsqlite/compose.go:22`、`app/onmem/compose.go:18`）。P32 の「フィールドは省けるが、引数は省けない」と引き換えに、名前を失った。
- **N55 `New` の引数の名前**。`leaves`、状態の名前、別の名前空間の名前を、そのまま引数の名前にした。当たる例: `leaves` や `shared` という名前の状態、自分の状態と同じ名前の別の名前空間（`mailer` という状態を持つ名前空間が、名前空間 `mailer` を呼ぶ）、Go の予約語の名前空間（`type` の op を呼ぶ名前空間の `New` は、引数 `type` を持てない）。基底の、末尾に `_` を付ける規則（`docs/emit-go.md`「名前」）に足すことになる。非公開の構造体 `ops` のフィールドも同じ名前を使う。P33 の後は、そのフィールドが、内部の op の非公開のメソッドとも当たる（N68）。この仕様では、どれも起きない。その後: 引数 `seams` が加わった。`seams` という名前の状態か、呼ぶ名前空間があれば当たり、後ろの `seams` の方に `_` が付く（引数の規則のまま）。内部の op の非公開のメソッドはなくなった（N68）が、継ぎ目を持つ op の中身のメソッド（`loginBody`）が、フィールドと当たりうる（GO-2）。
- **N56 `nil` は、書けば渡せる**。`server.New(nil, nil, nil, nil)` はコンパイルが通る（四つ目は、後で加わった継ぎ目の `seams`）。型のある `nil`（`(*app.Leaves)(nil)`）、プラグインの型の零値（`store.UsersTable{}`: `db` が `nil`）、フィールドを入れていない人の型（`&app.Leaves{}`: `Now` が `nil`）も同じ。`New` は確かめないので、その実装を使う op が呼ばれたところで panic になる（`app/server_test.go` の `TestNilIsNotChecked`: `nil` の `leaves` と、`nil` の `seams`）。使わない実装は `nil` のままでよく、テストは、それを使っている（内部の配線だけを呼ぶテストは、`seams` を渡さない。`app/inner_test.go` の `TestInternalWirings`）。`*runtime.MissingError` のような、見分けられるエラーにはならない。`net/http` のサーバーは、ハンドラの panic を受け止めて、その接続を切る（500 の `internal` は返らない）。P32 が防ぐのは「書き忘れ」で、「`nil` と書くこと」ではない。`New` が `nil` を確かめるなら、panic するか、`New` が `(Ops, error)` を返す形になる。→ 確かめない、とした（生成したコードから、実行のときの「ない」をなくす方を取った）。人の型のフィールド（`app.Leaves` の `Now` など）の入れ忘れには、P32 は届かない。
- **N57 葉の op は、すべて一つの値のメソッドになる**。`server.Leaves` は七つのメソッドを持つ一つのインターフェースで、`New` の引数は一つ。→ 一つの型 `app.Leaves` に七つを書いた（`app/leaves.go:22-88`）。分かったこと:
  - **外から何も受け取らない葉も、メソッドになる**。`checkPassword`、`toPublicUser`、`composeNotice` はレシーバを使わない（P32 の前は、ただの関数を口に入れていた）。
  - **人の型の名前と、葉のメソッド名が、同じ場所に入る**。`app.Leaves` のフィールド `NewOrderID` と、葉のメソッド `NewOrder` は一語違い（`app/leaves.go:28`、`:78`）。フィールドを `NewOrder` と名付ければ、コンパイルできない。仕様に葉を足すと、人の型にある同じ名前のフィールドや補助のメソッドと当たる。
  - **分けて書くには、埋め込む**。関心ごとの型（認証の葉、注文の葉）を一つの構造体に埋め込めば、メソッドが合わさって `server.Leaves` を満たす。二つの型が同じ名前のメソッドを持てば、渡す行が「あいまい」でコンパイルできない。葉の一部を、その語彙を読むトランスパイラが実装するとき（この仕様にはない）も、`go-sqlite` の状態の型と同じに、プラグインの型を人の型に埋め込むことになる。仕様の側に、葉をまとまりに分ける書き方（`Leaves` を二つのインターフェースにする単位）はない。
- **N58 標準の読み取り（`IsEmpty`、`Get`）と、op の名前が、同じインターフェースに入る**。状態の単位のメソッドは、形で決まる問い合わせと、その状態の葉の op。その状態を読む op が `get` か `isEmpty` という名前なら、同じ名前のメソッドが二つになり、生成したコードがコンパイルできない（P32 の前の、構造体のフィールドでも同じだった。避ける規則は、ないまま）。`Ops` には形で決まる問い合わせを載せないので、そちらは当たらない。もう一つ: どの状態の単位も `Get` を持ち、戻り値の型が状態ごとに違うので、**一つの型は、要素の型が違う二つの状態の単位を兼ねられない**。→ `go-sqlite` の型は状態ごとに一つ（四つ）、`memstore` も、一つの `Store` の上に、状態ごとの型を四つ書いた（`app/memstore/memstore.go:42-63`）。要素の型が同じ状態どうしなら兼ねられるが、そのときは N54 の取り違えが起きる。
- **N59 テストが op を一つだけ差し替えるには、型を一つ書く**。P32 の前は、`server.StoreOps{FindSession: func(…) {…}}` と、差し替える関数だけをフィールドに入れればよかった（入れなかった口は、呼ばれたら `*runtime.MissingError`）。今は、インターフェースを満たす型が要る。→ 二つの書き方を使った。
  - **インターフェースを埋め込み、書きたいメソッドだけを書く**。`seenStore` は `nil` の `server.StoreOps` を埋め込み、二つのメソッドを書く（`app/server_test.go:71-84`）。書いていない op が呼ばれれば、`nil` のインターフェースの呼び出しで panic になる。生成したコードからなくした「実行のときの、ない」が、テストの中にだけ、人の選びで戻る。`server.Ops` のスパイ（`spyOps`）は、前からこの形だった。
  - **本物の実装を埋め込み、一つだけ置き換える**。`fixedNotice` は、`server.Leaves` を満たす値を埋め込んで `ComposeNotice` を置き換える（`app/server_test.go:128-131`）。`humanUsers` も同じ形（N63）。
  - Go では、メソッドを関数の中に書けないので、型はテストの関数の外に置き、テストごとの値はフィールドで渡す（`seenStore.seen`、`humanUsers.added`）。クロージャ一つで済んでいたものが、型とフィールドとメソッドになる。
- **N60 `throws` を足すと、メソッドの形が変わる**。純粋で `throws` のない op に `throws` を足すと、戻り値に `error` が増える（`NewSession(user) Session` が `NewSession(user) (Session, error)` になる）。`Leaves` のメソッドと、人が書いた本体が、コンパイルできなくなる（気づける）。`export` を付けた op なら、`Ops` のメソッドと、それを呼ぶ人のコード（`x := ops.<Op>(…)`）も同じ。配線と、`ops` の非公開のメソッドは、生成し直される。最後の `throws` を外せば、逆のことが起きる。N12 の `ctx` と合わせると、op の形は三通り（純粋でない / 純粋で `throws` がある / 純粋で `throws` がない）で、仕様の一語で行き来する。分かったこと:
  - **`error` のない葉には、失敗を返す道がない**。`newSession` と `newOrder` は、トークンと識別子を作る。main では乱数（`crypto/rand`）で、読めなければ panic にしている（`app/cmd/shop-sqlite/main.go:70-73`）。「純粋で `throws` がない」は仕様の上の性質で、本体が本当に失敗しないことは、誰も確かめない（N9 と同じ構図）。失敗を返したければ、仕様に `throws` を書く（業務の例外ではない失敗のための、ステータスを持たない例外を宣言することになる）。
  - **純粋な配線**は、この仕様にない（配線は八つとも、純粋でない op を呼ぶ）。`error` を返さない配線の本体（`return o.<Op>(…)` が `error` なしで続く形、`match` の `throw` のない形）は書いていない。
  - **戻り値がなく、純粋で、`throws` もない op** は、引数を受け取って何も返さないメソッドになる。意味のある本体は書けないが、検査は通る。
- **N61（前半は解決: P33）`Ops` は、業務の途中の op をすべて載せていた**（N26 の続き）。P33 の前、`server.Ops` の十五のメソッドのうち七つは、`New` に渡された `leaves` の同じ名前のメソッドを呼ぶだけで、組み立てた人は、その値をすでに持っていた。P31 で、別の名前空間が使う op は、呼ぶ側が自分のインターフェースで言うようになった（`server.StoreOps` は `store` の九つのうち八つ）ので、`Ops` が「すべて」である理由は「op はすべて出る」（P11）だけだった。重さが出たのは `go-http`: `NewHandler(ops Ops)` は十五のメソッドを求めて、五つだけ呼んでいた。→ P33: `Ops` は `export` を付けた op だけ。この仕様では、それが `@http.*` の五つと同じなので、`NewHandler(ops Ops)` は、呼ぶ op ちょうどを受け取る。入口とそうでない op を分ける書き方も、仕様にできた（`export`）。
  - **残る後半: `go-http` は、自分が呼ぶ op だけのインターフェースを持たない**。`NewHandler` が受け取るのは、今も基底の `Ops`。`export` を付けたが `@http.*` のない op（別の名前空間や人のコードだけが呼ぶ op。`store.addUser` のようなもの）が `server` にあれば、`NewHandler` は、呼ばないメソッドも求める。`go-http` が、呼ぶ op だけのインターフェースを出して受け取る形（P31 と同じ考え）にすれば、`New` の戻り値はそのまま渡せて、スパイの書き忘れはコンパイルエラーになる。→ `NewHandler(ops Ops)` のままにした。スパイは、今も `server.Ops` を埋め込んで、数える二つだけを書く（`app/server_test.go:177-191`。N59 の一つ目の書き方。五つなら、すべて書くこともできる）。
- **N62 同じ名前空間を二つの単位で出す理由が、薄くなった**。P32 の前は、実装がパッケージの変数に入ったので、SQLite とメモリを一つのプロセスに持つには、出力を二つ（`gen/store`、`gen/storemem`）に分けるしかなかった。今は、実装は `New` の引数。二つの出力の状態の単位は、中身が同じインターフェースなので、`gen/store` の `New` に `memstore` の型を渡せる（`app/both_test.go:38-39`）。一つの `store` の中で、状態ごとに別の実装を混ぜることもできる（`users` は SQLite、ほかはメモリ。`app/both_test.go:144-152`）。`gen/storemem` に残る意味は、`go-sqlite` のファイル（`db.go`）と `database/sql` の import を持たないパッケージであること。→ 二つの単位は残した（基底のファイルが、単位に並べたほかのトランスパイラによらないことを、`app/both_test.go` の `TestStoreOutputsShareBaseFiles` が確かめる）。混ぜたとき、`@db.ref` の `REFERENCES` は SQLite の表の間でしか効かない（N42）。
- **N63 プラグインが出す型の形は、ここで決めた**。P32 が言うのは「プラグインは、機械的に決まる op を実装した型を出し、足りない op は人が書き足す」まで。決めたもの: 名前（`<状態>Table`、`New<状態>Table`）、値の型でありポインタではないこと、作るのが関数であること（`db` を渡し忘れられない。零値は書ける: N56）、すべて実装できた型に出す `var _ <状態>State = …`。足りない op があるときに分かったこと（見立ての型で確かめた。`app/onsqlite/store_test.go:94-117`）:
  - **足りないことを人が知るのは、`New` に渡した行のコンパイルエラーから**（`missing method AddUser`）。プラグインがどの op を実装しなかったかの一覧は、どこにも出ない。足りない型には `var _` の行がない、という違いだけがある。
  - **書き足す人の型は、同じ `*sql.DB` を自分でも持つ**。プラグインの型の `db` は非公開で、読み書きの補助（`dbScanUser`、`dbArgsUser`）も非公開なので、人の SQL は列の並びを書き直す（`app/onsqlite/store_test.go:104-108`）。
  - **置き換えもできる**。すべて実装した型でも、埋め込んで一つのメソッドを書けば、そのメソッドだけが人のものになる（一意の違反を見分ける `AddUser` など。N37）。プラグインの型は、埋め込まれることを前提にした形になっている。
- **N64 表を作り忘れても、コンパイルは通る**。`go-sqlite` の出力を使う手順は二つ（`CreateTables` を呼ぶ、型を `New` に渡す）で、P32 がコンパイルエラーにするのは後ろだけ。`CreateTables` を呼ばなければ、はじめの問い合わせが「表がない」というドライバのエラーになる。型を作る関数が `ctx` を取って表も作る形（`NewUsersTable(ctx, db) (UsersTable, error)`）なら忘れられないが、作るたびに文を実行し、`error` が増え、`store.New(…)` の引数に直に書けなくなる。→ 分けたままにした（表を作るのは、起動のときに一度）。移行の形がないこと（v1 の 59）とも、つながる。
- **N65 op の説明が、インターフェースごとに重なる**。P32 の前は、op の説明と `Throws:` などは、op ごとの関数の型（`<Op>Func`）のコメントに一度と、`Ops` のメソッドにあった。関数の型がなくなったので、その op のメソッドを宣言するところごとに書く: `store.findUser` は `store.Ops`、`store.UsersState`、`server.StoreOps` の三か所。P33 の前は、`checkPassword` も `Ops` と `Leaves` に同じコメントがあった。→ `Ops`、`Leaves`、`<Ns>Ops` には op の説明を、状態の単位には、問い合わせか命令かの一行を書いた。`Ops` が `Leaves` を埋め込めば重なりは減るが、`Ops` のメソッドの並びが宣言順でなくなる。**P33 の後、`server` の重なりは減った**: 七つの葉は内部になり、説明は `Leaves` の一か所だけになった。**その後（P35）、また重なった**: 内側の `Ops` は内部の op も持つので、七つの葉の説明は、内側の `Ops` と `Leaves` の二か所（`gen/server/inner/ops.go:26`、`:105`）、`export` を付けた配線の説明は、入口の `Ops`、内側の `Ops`、配線の本体の三か所にある。重なるのは、`export` を付けた op（`Ops` のメソッドと、配線の本体。`export` を付けた葉なら、`Ops` と、`Leaves` か状態の単位）。内部の配線は、逆に、説明が見えにくくなった（N72）。

#### `export` と内部の op（P33）

- **N66 `export` を付ける理由になる「外の呼び手」は、三種類あった**。P33 の言い方は「別の名前空間から呼べる op」だが、この仕様で `export` を付けた十五のうち、仕様の別の名前空間が呼ぶのは九つ（`store` の八つと `mailer.send`）だけ。`server` の五つを呼ぶのは `go-http` のハンドラ（プラグインの出力）、`store.addUser` を呼ぶのは人が書くコード（main とテスト）で、どちらも仕様には現れない。基底の形では、`export` は「`Ops` に載る」と同じ意味になり、`Ops` を受け取る者は、みな外の呼び手。→ `addUser` は `export` にした（「`export` を付けた op」）。帰結:
  - **「`export` を付けたのに、どの名前空間も呼ばない」を、検査は言えない**。呼び手が仕様の外にいるかどうかは、仕様から分からない。六つ（`server` の五つと `addUser`）がそうなっている。
  - **人のコードのためだけに `export` を付けると、仕様の別の名前空間からも呼べるようになる**。「人のコードには見せるが、仕様の配線からは呼ばせない」は書けない。`addUser` は、`server` の配線からも呼べる。
- **N67 HTTP の入口には、`@http.*` と `export` の二つを書く**。五つの入口は、どれも `@http.post` / `@http.get` と `export` を並べて持つ（`server.hozo:41-43` など）。デコレータは「どう呼ばれるか」、`export` は「外から呼べるか」で、言っていることは別だが、`@http.*` を付けて `export` を付けない op に意味はない。→ `go-http` の規則にした: `export` のない op に `@http.get` / `@http.post` が付いていれば、プラグインが error を報告する（「`go-http` が従う規則」）。考えた別の形は二つ。(a) 黙って飛ばす: 仕様に書いた経路が、エラーなしにサーバーから消えるので取らなかった。(b) `@http.*` が付いていれば `export` と見なす: 語彙のデコレータが、本体の検査（別の名前空間から呼べるか）の結果を変えることになるので取らなかった。残ること: 付け忘れが分かるのは、`go-http` を並べた単位を emit したとき（または、そのプラグインの `check`）。`hozoc check` の本体の規則ではないので、`go-http` を使わない設定では、何も言われない。
- **N68（P35 で変わった）内部の op の名前は、ここで決めた**。P33 が言うのは「`Ops` には `export` の付いた op だけが並ぶ」までで、内部の op を Go でどう持つかは言っていない。→ 構造体 `ops` の非公開のメソッドにし、名前は op の名前の先頭を小文字にしたもの（規則は「基底 `go` が従う規則」の「内部の op」）。分かったこと:
  - **非公開のメソッドにしたので、パッケージの外からは、どうやっても呼べない**。考えた別の形は、「公開のメソッドのまま、`Ops` から外すだけ」（`o.Authenticate`）。配線の書き方は P33 の前と変わらないが、`New` の戻り値を、外で宣言したインターフェースに型アサーションすれば呼べる（`ops.(interface{ Authenticate(…) … })`）。非公開のメソッドなら、それも通らない（そのときの `app/both_test.go` が確かめていた）。
  - **`export` を付け外しすると、メソッドの名前が変わる**（`Notify` と `notify`）。`Ops` を呼ぶ人のコードは、外した op を呼んでいればコンパイルできなくなる（気づける）。その op を呼ぶ別の名前空間は、検査でエラーになる。生成したコードの中では、その op を呼ぶすべての配線の行が変わる。
  - **Go の予約語と当たるのは、P33 から**。op の名前は小文字で始めるので、`go`、`select`、`range`、`import`、`default` のような名前の内部の op は、そのままではメソッドにできない（先頭を大文字にする `export` の op では起きない）。→ 末尾に `_`（`docs/emit-go.md`「名前」の規則のまま）。同じ op が、`export` を付ければ `Select`、外せば `select_` になる。
  - **`ops` のフィールドと当たる**（N55 の続き）。フィールドは `New` の引数の名前（`leaves`、`shared`、状態の名前、呼ぶ名前空間の名前）なので、名前空間 `mailer` を呼ぶ `server` に、内部の op `mailer` や `leaves` があれば当たる。→ メソッドの方に `_` を付ける、とした。パッケージの名前（プラグインの `httpWriteJSON` など。N50）とは、メソッドなので当たらない。
  - **先頭の大文字・小文字だけが違う二つの op**（`foo` と `Foo`）は、P33 の前から GOEMIT-2（先頭を大文字にした名前で比べる）。一方が内部なら、`ops` のメソッドは `Foo` と `foo` で当たらなくなるが、`Leaves` と状態の単位では、内部の op も公開の名前になるので、この規則は `export` によらず、そのままにする。
  - **その後（P35）: 非公開のメソッドにする形は、やめた。** 内部の op も、`ops` の公開のメソッド（`Authenticate`）で、内側のパッケージ `gen/server/inner` の `Ops` に並ぶ。入口のパッケージの `Ops` には並ばない（`docs/plugin-go.md`「内側のフォルダと入口」）。この項目の下の四つ（付け外しで名前が変わる、予約語、フィールドとの衝突、`foo` と `Foo`）は、起きなくなった: メソッドの名前は、`export` によらず、op 名の先頭を大文字にしたもの。「パッケージの外からは、どうやっても呼べない」は、手放した: `inner` を import すれば呼べる。別の名前空間から呼べないことを守るのは、Hozo の検査（NS-5）。`export` を付け外しして変わるのは、入口の `Ops` のメソッドの有無と、内部の op が一つもなくなれば、内側のフォルダの有無（N76）。
- **N69 「内部」が隠すのは配線だけで、人が書く葉は、Go の上では公開のまま**。P33 は「内部の op も、人が書くものは実装を受け取るインターフェースに並ぶ」と決めている。`server.Leaves` の七つは内部の op だが、人が別のパッケージで実装するので、メソッドは公開の名前（`CheckPassword`）で、`app.Leaves` の公開のメソッドになる。実装の値を持つ者は、誰でも呼べる: 葉のテストは `leaves.CheckPassword(…)` と直に呼ぶ（`app/leaves_test.go:66`）。→ 矛盾ではない、と読んだ: 仕様の「内部」は「別の名前空間の配線から呼べず、`Ops` に載らない」で、「その本体を、パッケージの外の Go のコードが実行できない」ではない。本体は人のもので、基底は、それを `Ops` から配らないだけ。ただし:
  - **`Leaves` を見ても、どの op が内部かは分からない**。`export` を付けた葉（この仕様の `server` にはない）も、同じ `Leaves` に、同じ形で並ぶ。メソッドのコメントに「内部」とは書いていない。
  - **状態の単位でも同じことが起きる**（この仕様では、`store` の九つがすべて `export` なので、起きていない）。`store` の内部の葉は、`store.Ops` からは消えるが、`<状態>State` のメソッドと、`go-sqlite` が出す公開の型のメソッドには残る: `store.NewUsersTable(db).FindUser(ctx, …)` は、どのパッケージからも呼べる。`store` の op を内部にしても、保存先を読み書きする口は閉じない。
  - **テストと、人のコードには、それが抜け道になる**。`addUser` を内部にして、main が `store.NewUsersTable(db).AddUser` を呼ぶ形でも動く（そうしなかった理由は「`export` を付けた op」）。
- **N70（P35 で変わった）葉を呼ぶだけの非公開のメソッドが、七つ残った**。P33 の前、`ops` の `CheckPassword` などは、`Ops` を満たすために要った。今は、`o.checkPassword(…)` を呼ぶのは配線だけで、中身は `o.leaves.CheckPassword(…)` の一行（`gen/server/inner/ops.go:222-247`）。配線が `o.leaves.CheckPassword(user, password)` と直に呼べば、七つのメソッドは要らない。→ 残した。配線の本体の規則が一つで済む（自分の名前空間の op は、いつも `ops` のその op のメソッド。呼び出し先が葉か配線か、`export` があるか、実装が `Leaves` か状態の単位か `SharedState` かで、本体の書き方が変わらない）方を取った。直に呼ぶ形にするなら、`export` を付けた葉（`Ops` を満たすために、`ops` のメソッドが要る）だけが、二通りの呼び方を持つことになる。
  - **その後（P35）**: 七つは公開のメソッド（`o.CheckPassword(…)`）になり、内側の `Ops` を満たすために要るものに戻った。テストは、内側の `Ops` から、葉も直に呼べる。配線の本体の規則が一つで済むことは、変わらない。
- **N71（解決: P35）内部の配線は、単独では試せない**。P33 の前、テストは `ops.Notify(ctx, order)` と呼び、`notify` が通る二つの op（`mailer.send`、`store.addNotification`）だけを書けばよかった。今は、`notify` に届く入口 `placeOrder` を呼ぶしかなく、そこまでの道が通るもの（`store.findSession`、葉 `requireFresh` と `newOrder`、`store.addOrder`）も用意する（そのときの `app/server_test.go` の `TestReplaceOneLeaf`。テストの型のメソッドは二つから四つに増えた）。`authenticate` の振る舞い（トークンなし、知らないトークン、期限切れ）は、P33 の前から、四つの入口を通して確かめている（`app/handlertest` の `expired` など）。分かったこと:
  - **同じパッケージにテストを置く、という Go の普通の手が使えない**。`gen/server` は単位の `out` で、そこにあるファイルは hozoc のもの（記録 `.hozoc-outputs.json`）。人の `_test.go` を置く場所ではない。
  - **内部の配線の、メソッドの形**（`ctx` と `error` の有無）は、外から書けない。P33 の前は、`server.Ops.Authenticate` のメソッド式で確かめていた。今、確かめているのは `export` を付けた五つと、葉と、別の名前空間の op だけ（そのときの `app/server_test.go` の先頭の `var`）。内部の配線の形は、人のコードの誰にも見えないので、確かめる理由もなくなった、とも言える。
  - **`nil` を渡したときの panic**（N56）も、入口から届かせる: `TestNilIsNotChecked` は、ユーザーがいつもいる `store` を書いて、`login` を葉 `checkPassword` まで進める（そのときの `TestNilIsNotChecked`）。
  - 隠したのだから、入口を通して試すのは狙いどおり。重さは、入口から遠い配線ほど大きくなる。
  - **→ P35: 内部の op を持つ出力を、内側のフォルダ `inner` と入口に分け、テストは内側を参照する。** 内側の `Ops` は、すべての op を公開のメソッドに持つ。`TestReplaceOneLeaf` は、`inner.New(…).Notify(ctx, order)` と呼ぶ形に戻った（テストの型のメソッドは二つ。`app/server_test.go:156-173`）。内部の配線の形も、メソッド式で書ける（`inner.Ops.Authenticate`。`app/server_test.go:47-50`）。`authenticate` と `issueSession` を単独で試すテストを足した（`app/inner_test.go` の `TestInternalWirings`）。`nil` の `leaves` の panic は、葉を直に呼んで確かめる。同じパッケージにテストを置けないことは変わらないが、要らなくなった。
- **N72（解決: P35）内部の配線の説明は、非公開のメソッドにしかない**。P33 の前、`authenticate` の説明と `Throws:`、`Reads:` は、`Ops` のメソッドのコメント（公開。`go doc` に出る）と、配線の本体の二か所にあった。今は、`wiring.go` の非公開の構造体の、非公開のメソッドのコメントだけ（そのときの `gen/server/wiring.go`）で、`go doc` には出ない。読むには、生成したファイルを開く。内部の葉の説明は、`Leaves`（公開）にある。→ そのままにした（外から呼べないものの説明を、外に出す理由がない）。残ること: `export` を付けた op のコメントの `Throws:`、`Reads:`、`Effects:` は、内部の op から伝わってきたものを含む（`PlaceOrder` の `Effects: store.createOrder, store.notifyUser` の後ろは、内部の `notify` のもの）が、どの内部の op を通ったかは、公開のコメントからは分からない。仕様を読めば分かる。
  - **→ P35**: 内部の配線の説明と `Throws:`、`Reads:` は、内側の `Ops` のメソッドのコメント（公開。内側のパッケージの `go doc` に出る）と、配線の本体にある（`gen/server/inner/ops.go:42-46`、`gen/server/inner/wiring.go:42-46`）。入口のパッケージの `go doc` には、今も出ない（外から呼べないものの説明を、入口に出す理由がない）。
- **N73 内部の op の分類を、外から使えるか**。P33 は、分類を、今までどおりどこからでも参照できるものにした。分類は op について宣言する（`classify Registration of findUser`。`store.hozo:36-39`）ので、**その op が内部でも、分類の名前は外から書ける**。この仕様の三つの分類は、どれも `export` を付けた op のもので、`server` は、自分で呼んだ `store.findUser` の結果を `by store.Registration` で分けている（`server.hozo:46-47`）。`findUser` が内部だったら:
  - `server` は `findUser` を呼べないので、その結果を持てない。`match x by store.Registration` の `x` が `findUser` の結果でなければならないなら（N4 の、決まっていない点）、内部の op の分類は、外からは**参照できるが、使えない**。
  - `store` の `export` を付けた配線が、内部の `findUser` の結果をそのまま返すとき、その値を外で `by store.Registration` で分けられるか（値の出どころが、配線を通って伝わるか）も、N4 と同じく決まっていない。
  - 分類の宣言 `Registered: findUser as type.User;` は、内部の op の名前と戻り値の型を、外から読めるところに書く。
  - 基底は、この形の分類について何も出さない（N23）ので、Go のコードは、どちらに決まっても変わらない。人が述語を書く分類（基底が判定関数を出す形）なら、内部の op の分類の関数を、公開の名前で出すかどうかを決めることになる。
- **N74 すべての op が `export` の名前空間では、`export` は九回書くだけの語**。`store` の op は、ほかの名前空間に読み書きさせるためにあるので、九つとも `export`（`store.hozo:22-83`）。`mailer` も、一つの op が `export`。内部の op があるのは、配線を持つ `server` だけ。既定を逆（付けなければ外から呼べる。隠したいものに印）にすれば、`store` と `mailer` には何も書かずに済むが、`server` の十に印が要る。P33 は「付けたものだけが外から見える」を取っていて、付け忘れは、呼ぶ側の検査のエラーか、人のコードのコンパイルエラーで分かる（外に見せすぎる方の誤りは、気づけない）。関連して:
  - **単位のない名前空間（`mailer`）の `export`** が効くのは、検査だけ。`server.MailerOps` は、P33 の前と同じ。
  - **`export` を付けた op が一つもない名前空間**は、`New` が空の `Ops` を返す、と決めた（「基底 `go` が従う規則」）。外から何も呼べないパッケージに意味があるのかは、この仕様に出てこないので考えていない（相乗りのプラグインが、`Ops` を通さずに入口を作る形があれば、意味がある）。
- **N75 呼ばれない内部の op を、検査が言えるようになった**。P33 の前は、どの op も `Ops` にあり、仕様のどこからも呼ばれていない op も、人のコードが呼んでいるかもしれなかった。今は、内部の op を呼べるのは、同じ名前空間の配線だけなので、**どの配線からも呼ばれない内部の op は、誰にも呼べない**。配線なら、使われないメソッドが出る（P35 の後は、内側の `Ops` の公開のメソッド。テストだけが呼べる）。葉なら、人は、誰も呼ばない op の本体を `Leaves` に書かされる（書かなければコンパイルできない）。この仕様の十の内部の op は、どれも呼ばれている。検査の規則（warning）にできるが、P33 は決めていない。

#### 内側のフォルダと入口（P35）、継ぎ目（P36〜P39）

- **N76 `export` の付け外しで、定義のパッケージが動く**。内部の op が一つでもあれば、出力は内側 `inner` と入口に分かれ、一つもなくなれば、分かれなくなる。この仕様では `server` だけが分かれる。入口の名前（`server.Ops`、`server.LoginInput`、`server.New`）は変わらないので、`app/` の、テストでないコードは、どれも入口だけを import している。動くもの: 型の本当の名前（`%T`、コンパイルエラーの文言は `inner.LoginInput`）、`go-valid` の `validate.go` の置き場（`gen/server/inner/validate.go`。型にメソッドを足すので、定義のパッケージ）、`inner` を import するテスト（内部の op がなくなれば、パッケージごとなくなって、コンパイルできない）。最後のものは、狙いどおり: 内部の op を試すテストは、内部の op がなくなれば要らない。
- **N77 入口の型は別名なので、同じ型に名前が二つある**。`server.LoginInput` と `inner.LoginInput` は同じ型で、どちらで書いても通る。`go-ratelimit` の出力は入口のパッケージにあるので入口の名前で書き、テストが書く継ぎ目（`app/inner_test.go` の `through`）は内側の名前で書いた。どちらも `inner.Seams` を満たす（`app/server_test.go:57-63`）。人が読むときに、二つが同じものだと分かるのは、`gen/server/types.go` の `type LoginInput = inner.LoginInput` を見たとき。
- **N78 入口の `New` が返す値は、内部の op も持っている**。入口の `New` は、内側の `New` の戻り値を、入口の `Ops`（五つ）として返す。値そのものは十五のメソッドを持つので、`ops.(inner.Ops)` と型アサーションすれば、内部の op を呼べる。→ そのままにした（文書の決定: 外から呼べなくすることは目的にしない）。`TestOpsHoldExportedOpsOnly` は、インターフェースのメソッドだけを見る形に直した。P33 の後の形（N68）で確かめていた「型アサーションでも取り出せない」は、確かめなくなった。
- **N79 継ぎ目は、`New` の引数を一つ増やす**。継ぎ目を宣言するトランスパイラを単位に足すと、`New` を呼ぶ行（二つの組み立てと、`server.New` を直に呼ぶテスト）が、どれもコンパイルできなくなり、`seams` を渡すように直す。`Ops` を使うコード（`go-http` の出力、main）と、葉の実装（`app/leaves.go`）は、変わらない。逆向きも気づける: 単位から `go-ratelimit` を外すと、`Seams` と四つ目の引数がなくなり（語彙 `rate` も読まれなくなるので、その前に `@rate.limited` が解決できない）、仕様から `@rate.limited` を消すと、`ratelimit.go` が出なくなって `server.NewRateSeams` がなくなる。どちらも、組み立ての行がコンパイルできない。「外したのに、掛かっていると思い込む」は、この単位では起きない（起きる場合は N81）。
- **N80 継ぎ目の内側だけを試すには、素通しの継ぎ目を書く**。中身のメソッド（`loginBody`）は非公開で、どのインターフェースにもない。テストは、`body` をそのまま呼ぶ `inner.Seams`（`app/inner_test.go:102-107`）を書いて `inner.New` に渡し、外側の `Login` を呼ぶ。継ぎ目が一つなら、型は一つ、メソッドは一つ。継ぎ目を持つ op が二十あれば、素通しの型のメソッドも二十（op ごとに型の付いたメソッドなので、まとめて一つには書けない）。→ そのままにした。基底が素通しの実装を出す（`PassSeams` のような型）ことは、決めていない。
- **N81 同じ名前空間の別の単位では、同じデコレータが、黙って印になる**。継ぎ目は単位ごとのもの（`docs/config.md`「継ぎ目」）。この仕様の `store` のように、同じ名前空間を二つの単位で出力していて、`go-ratelimit` を一方にしか並べなければ、もう一方の出力の `login` は、制限なしで動く。warning はなく（P37: 読む者がなければ、何も出ない）、`Ops` のコメントには、どちらの出力でも `Throws: … rate.LimitExceeded` と出る（`Signature.exceptions` は仕様だけで決まる）。この仕様では、`server` の単位は一つなので起きない。どのトランスパイラも読まないデコレータが `throws` / `catches` を持つときに、何か言うかどうか（単位ごとの warning など）は、P37 が「言わない」と決めている。
- **N82 デコレータの節が言うことを守るのは、継ぎ目の中身だけ**。`decorator limited … throws LimitExceeded prevents effects` の「制限を超えたら、効果は起きない（中身を呼ばない）」は、検査の計算に効く（`login` の `rate.LimitExceeded` と `store.createSession` の関係は `prevented`）が、`go-ratelimit` の出力がそう書いてあること（`gen/server/ratelimit.go:27-29`）を、hozoc も基底も確かめない。確かめているのは、人のテスト（`app/server_test.go` の `TestLoginIsLimitedThroughOps`: 制限を超えた呼び出しで、`store` のどの op も呼ばれない）。「デコレータは、op の入力と出力の値を変えない」（P39）も指針で、継ぎ目は、中身に渡す値も、返す値も差し替えられる（`app/inner_test.go` の `TestSeamOwnsBeforeAndAfter` が、差し替える継ぎ目を書いて、基底が止めないことを確かめている）。
- **N83 語彙の例外を使うと、設定に単位が一つ増える**。`rate.LimitExceeded` の Go の型の置き場として、`rate` の単位（`gen/rate`。型だけのパッケージ）を足した。プロジェクトの中のプラグインを使うために `hozo.toml` に書くのは三つ: `plugin_dirs`、継ぎ目を持つ単位の `transpilers` に `go-ratelimit`、語彙の名前空間の単位。三つ目は、忘れると CFG-22 が、足す単位の形を示す。例外を `type` に置いて、デコレータの引数で渡す形なら、単位は増えない（「サンプルの中のプラグイン」の「語彙と、型の置き場」）。`go-http` は、`go-ratelimit` を知らないまま 429 を返す: 語彙 `rate` が、自分の例外に、標準の語彙のデコレータ `@http.status(429)` を書いているから（プラグインの語彙が、別の語彙に依ってよいか、依る先が標準でない語彙だったらどうするかは、決めていない）。
- **N84 数え方の受け取り方は、プラグインが決めた。呼んだ人ごとの制限は、書けていない**。基底の `Seams` は、継ぎ目の実装が実行のときに何を要るかを知らない。`go-ratelimit` は、`RateLimiter` を出して、`NewRateSeams(limiter)` で受け取る形にした（葉の op の `Leaves` と同じ作法）。`Allow(ctx, op)` が受け取るのは、op の FQN だけ。**呼んだ人ごとに数えるには、呼んだ人が要る**が、op の入力（`LoginInput`）は op ごとに型が違い、送り元のアドレスは op の入力になく（HTTP の層にある）、基底は `ctx` に値を載せない。→ このサンプルでは、op ごとの回数だけを数えた。道は二つ考えられる: デコレータの引数で、入力のどの値ごとに数えるかを書く（`@rate.limited(by: …)`。継ぎ目のメソッドは op ごとに型が付いているので、プラグインは、その値を `Allow` に渡すコードを出せる）か、人が `ctx` に載せて、人の `Allow` が読む。どちらも決めていない。
- **N85 継ぎ目は、`go-http` の検査より内側にある**。`POST /login` は、読めない要求と制約の違反（400）が先で、制限（429）は op の中。制限を超えていても、制約に合わない入力には 400 が返り、その要求は数えない（`app/handlertest/handlertest.go:174-177`。N47 と同じ並び）。本文を読む前に断る（負荷を避ける）ための制限は、op の継ぎ目ではなく、`http.Handler` を包む人のコード（ミドルウェア）の仕事になる。`@rate.limited` が言うのは「この op を呼べる頻度」で、HTTP の要求の頻度ではない: `server.New` の `Ops` を直に呼んでも、同じに数える。
- **N86 テストは、まだないものを名指しして飛ばす**。`TestSamples` を最後の形にした（P36）。サンプルの中のプラグインの実装がまだないことは、`sample.json` に書かず、テストが見分ける（マニフェストの `command` が `go run` で、そのディレクトリに Go のファイルがない）。今の hozoc は、その前に、このマニフェストを読めない（`requires` と `seams` が知らないキーで MAN-4）。`hozo.toml` も読めない（N51）ので、単位の `out` が要る `files`（`sample.json` と記録の照合）も、今は飛ぶ: 文書の形にそろえた今回の `gen/` と `sample.json` と記録の一致は、同じ規則を手で当てて確かめた。hozoc が `[emit.options]` を読むようになれば、`files` が、何も変えずに走る。

#### デコレータで言う読み書き（決定 91〜102）

- **N87 列を渡す引数の名前を `by` にできなかった**。決定の書き方は `find(table: any state, by: string)` だったが、`by` は予約語（`match … by`）で、デコレータの引数の名前（`Ident`）に書けない。→ `column` にした（`docs/vocab.md`「`db` の読み書きのデコレータ」）。位置で渡すので、仕様の書き方（`@db.find(users, "email")`）は変わらない。
- **N88 `@db.find` の列は、一行に決まる列でなければならない**（GOSQL-13）。決定が並べた整合の検査にはなかった。足さなければ、一意でない列を名指しした `find` が、合う行のうちのどれか一つを黙って返す。前の規則の「対応した列のどれかが `@db.key` か `@db.unique`」を、検査として残した形。
- **N89 `T?` の列と `T?` の引数で、値がないときの意味**。決定 96 は「型が等しければよい」までで、値のない引数が何に合うかは言っていない。→ 等しい型どうしの比較として、値のない行に合う、と決めた（`"c" IS ?n`。`docs/plugin-go-sqlite.md`「列と引数の型」）。この仕様には、省略可能な列がないので、期待する出力には現れない。
- **N90 配送に失敗したときに、通知の記録が残るようになった**。前は、`mailer.send` の後で `store.addNotification` を呼んでいたので、配送に失敗すれば記録は残らなかった。二つの書き込みを一つの op にまとめるには、記録を配送の前に置くしかない（配送は別の名前空間の op で、トランザクションに入らない）。記録が「送った通知」ではなく「送るべき通知」になり、送れたかどうかは、どこにも残らない（`app/handlertest/handlertest.go:224-235`）。配送の前に注文だけを書き、配送の後で通知を書く形に戻すなら、二つの効果を一つの op にする理由がなくなる。
- **N91 メモリの上の実装では、「一体」を人が守る**（`app/memstore/memstore.go` の `Shared.PlaceOrder`）。`gen/storemem` の単位には `@db.effects` を読むトランスパイラがいないので、「一つのトランザクション」は、デコレータの説明のコメントにしかない。二つ目の効果だけが失敗する場合（SQLite では外部キーの違反で起こせる。`TestPlaceOrderIsOneTransaction`）は、メモリの上では起こせないので、二つの実装が共に走らせるテスト（`storetest` の `placeOrder`）は、一つ目が失敗する場合だけを確かめる。N42 と同じ: 状態の単位のインターフェースが保証するのは、メソッドの形まで。
- **N92 トークンがない場合を仕様に書くと、配線が一つ増える**。アームに書けるのは呼び出し一つ（N1）なので、「トークンがあれば、セッションを引いて、分ける」は、`authenticate` のアームから呼ぶ別の配線 `sessionOf` になった（`server.hozo:94-102`）。分類 `Credential` は、`by nullable` と同じ判定で、場合に名前を付けただけ（N4 と同じ）。
- **N93 `SharedState` の実装は、またがる状態と同じ保存先のものでなければ意味がないが、型はそれを言わない**。`store.New(users, sessions, orders, notifications, shared)` に、`orders` はメモリ、`shared` は SQLite、と渡しても、コンパイルは通る（`placeOrder` は SQLite の表に書き、`listOrders` はメモリを読む）。`app/both_test.go` の `TestStatesFromTwoImplementations` は、そろえて渡している。N42 の、状態ごとの実装を混ぜられることの、裏側。
- **N94 サンプルの中のプラグインのマニフェストの `protocol` を、先に上げられない**。文書の版は 0.5 だが、`plugins/go-ratelimit/hozo-plugin.toml` を 0.5 にすると、今の hozoc が MAN-2 で断り、プラグインの単体テストの入力（`testdata/*/views.json`。今の hozoc が作る）とも食い違う。マニフェストの版は、仕様ではなく、読む hozoc と対のものなので、実装と一緒に上げる（「まだないもの」）。
- **N95 文書にあって、この仕様が使っていない形**: 述語で決まる状態の分類の単位の型（`<C>Table`）、遷移を宛先にする効果のデコレータ、`@db.update`、列のない `@db.list`、`T?` の列。どれも、期待する出力による確かめがない（`go-sqlite` の単体テストが、小さな入力で確かめることになる。`docs/testing.md`「単体テスト」）。
- **N96 期待する出力を、実装のない間に、どこまで道具で確かめられたか**。`gen/server` は、今の hozoc で確かめられた（「まだないもの」の末尾）。`gen/store` の基底のファイル（`SharedState`、`New` の引数 `shared`）と `db.go` は、文書から手で書いたもので、確かめたのは、コンパイルできることと、テストが通ること（一つのトランザクションであることを含む）まで。空白やコメントの文言が文書の型どおりかは、実装ができて `transpilers/go` と `transpilers/go-sqlite` が走るまで、機械では確かめられていない。
