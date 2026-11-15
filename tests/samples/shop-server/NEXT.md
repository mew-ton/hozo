# shop-server の次の形（下書き）

`docs/decisions.md`「決めたが、まだ文書と実装に入れていないこと」の P1〜P19 を、このサンプルの仕様に当てはめた下書き。
**今の hozoc では検査できない**（新しい書き方を含む）。決定が固まって実装に入るとき、この内容で `server.hozo` と
`hozo.toml` と期待する出力を置き換え、この文書は消す。書き方のうち「仮」としたもの（`as`、`reads`、`never`、`throw`、
オプションの書き方）は、ここでもそのまま仮の語で書いている。

今の形（`server.hozo`）との違い:

- 名前空間が三つになる: 共有の型 `type`（P6）、DB の状態を持つ `store`（P5）、HTTP の入口と業務の流れを持つ `server`。
- 人が書いていた葉の op の中身（探す、確かめる、足す）が、配線と分類になる（P1〜P3、P13、P14）。
- `store` の op は、読む先（`reads`。P19）と効果から中身が決まるので、DB のプラグインが出す。
- `server` で人が書く op は、保存先に触れない。
- 入力の検証は `go-http` が要求を受けたときに行う（P15）。プロジェクトの例外 `InvalidInput` はなくなる。

## 仕様

```ts
/** 共有の型。どの担い手にも属さない */
namespace type {
  /** ユーザーの識別子 */
  scalar UserId extends string;
  /** メールアドレス */
  @valid.format("email")
  scalar Email extends string;
  /** パスワード（平文。入力でだけ使う） */
  @valid.length(min: 8, max: 128)
  scalar Password extends string;
  /** ログインセッションのトークン */
  scalar Token extends string;
  /** 注文の識別子 */
  scalar OrderId extends string;
  /** 商品の識別子 */
  scalar ItemId extends string;

  /** ユーザー */
  model User {
    @db.key    id: UserId;
    @db.unique email: Email;
    @valid.length(min: 1, max: 100) name: string;
    hash: string;
  }
  /** 外に見せるユーザー */
  model PublicUser is User omit hash;

  /** ログインセッション */
  model Session {
    @db.key       token: Token;
    @db.ref(User) userId: UserId;
    expiresAt: Time;
  }

  /** 注文の一行 */
  model OrderLine {
    item: ItemId;
    @valid.min(1) quantity: int;
  }
  /** 注文 */
  model Order {
    @db.key       id: OrderId;
    @db.ref(User) userId: UserId;
    lines: OrderLine[];
    placedAt: Time;
  }
  /** ユーザーへの通知 */
  model Notification {
    @db.ref(User) userId: UserId;
    message: string;
  }

  /** メールアドレスかパスワードが一致しない */
  @http.status(401)
  exception InvalidCredentials;
  /** 認証されていない */
  @http.status(401)
  exception Unauthorized;
  /** 通知を送れなかった */
  exception NotificationFailed;
}

/** 保存。DB の状態の持ち主 */
namespace store {
  @db state users: list<type.User>;
  @db state sessions: list<type.Session>;
  @db state orders: list<type.Order>;
  @db state notifications: list<type.Notification>;

  // ---- 読む（効果を持たない op。P1、P19） ----

  /** email からユーザーを引く */
  op findUser(email: type.Email): type.User? reads users;
  /** 識別子からユーザーを引く */
  op findUserById(id: type.UserId): type.User? reads users;
  /** トークンからセッションを引く。トークンがなければ、ない */
  op findSession(token: type.Token?): type.Session? reads sessions;
  /** ユーザーの注文を、足した順に返す */
  op listOrders(userId: type.UserId): type.Order[] reads orders;

  // ---- 読んだ結果の分類（P2、P3） ----

  /** その email が登録済みか */
  classify Registration of findUser {
    Registered: findUser as type.User;
    default;
  }
  /** その識別子のユーザーがいるか */
  classify Account of findUserById {
    Found: findUserById as type.User;
    default;
  }
  /** そのトークンでログインしているか */
  classify Login of findSession {
    LoggedIn: findSession as type.Session;
    default;
  }

  // ---- 書く ----

  /** ログインセッションを作る */
  effect createSession on sessions.post;
  /** ログインセッションを消す */
  effect deleteSession on sessions.delete;
  /** 注文を作る */
  effect createOrder on orders.post;
  /** ユーザーに通知する */
  effect notifyUser on notifications.post;

  op addSession(session: type.Session) effects createSession;
  op removeSession(token: type.Token) effects deleteSession;
  op addOrder(order: type.Order) effects createOrder;
  op addNotification(notification: type.Notification) effects notifyUser;
}

/** サーバー。HTTP の入口と、業務の流れ */
namespace server {
  model Credentials { email: type.Email; password: type.Password; }
  model LoginInput { @http.body body: Credentials; }
  model LoginOk { user: type.PublicUser; token: type.Token; }
  model AuthInput { @http.header(name: "Authorization") token: type.Token?; }
  model GetOrdersInput {
    @http.header(name: "Authorization") token: type.Token?;
    /** 1 から数えるページ番号 */
    @http.query page: int = 1;
  }
  model PlaceOrderInput {
    @http.header(name: "Authorization") token: type.Token?;
    @http.body lines: type.OrderLine[];
  }

  // ---- ログイン ----

  /** ログインする */
  @http.post("/login")
  op login(in: LoginInput): LoginOk
    throws type.InvalidCredentials
  {
    const user = store.findUser(in.body.email);
    return match user by store.Registration {
      Registered => issueSession(user, in.body.password);   // user は type.User（P3）
      default    => throw type.InvalidCredentials;          // P13
    };
  }

  /** パスワードを確かめて、セッションを作る */
  op issueSession(user: type.User, password: type.Password): LoginOk
    throws type.InvalidCredentials
  {
    checkPassword(user, password);
    const session = newSession(user);
    store.addSession(session);
    return { user: user, token: session.token };
  }

  /** パスワードが合わなければ InvalidCredentials（人が書く） */
  op checkPassword(user: type.User, password: type.Password)
    throws type.InvalidCredentials;
  /** トークンと期限を決める（人が書く） */
  op newSession(user: type.User): type.Session;

  // ---- 誰が呼んだか ----

  /** 呼んだ人のセッション。ログインしていなければ Unauthorized */
  op authenticate(token: type.Token?): type.Session
    throws type.Unauthorized
  {
    const session = store.findSession(token);
    return match session by store.Login {
      LoggedIn => session;                    // 絞り込んだ値をそのまま返す（P14）
      default  => throw type.Unauthorized;
    };
  }

  /** ログアウトする */
  @http.post("/logout")
  op logout(in: AuthInput)
    throws type.Unauthorized
  {
    const session = authenticate(in.token);
    store.removeSession(session.token);
  }

  /** 自分の情報を返す */
  @http.get("/me")
  op getMe(in: AuthInput): type.PublicUser
    throws type.Unauthorized
  {
    const session = authenticate(in.token);
    const user = store.findUserById(session.userId);
    return match user by store.Account {
      Found   => user;                        // type.User から PublicUser へは射影で渡せる
      default => throw type.Unauthorized;
    };
  }

  // ---- 注文 ----

  /** 自分の注文の一覧を返す */
  @http.get("/orders")
  op getOrders(in: GetOrdersInput): type.Order[]
    throws type.Unauthorized
  {
    const session = authenticate(in.token);
    const orders = store.listOrders(session.userId);
    return pageOf(orders, in.page);
  }
  /** 一覧から、そのページの分を取り出す（人が書く） */
  op pageOf(orders: type.Order[], page: int): type.Order[];

  /** 注文する。通知に失敗しても、注文は残る */
  @http.post("/orders")
  op placeOrder(in: PlaceOrderInput): type.Order
    throws type.Unauthorized | type.NotificationFailed
  {
    const session = authenticate(in.token);
    const order = newOrder(session, in.lines);
    store.addOrder(order);
    notify(order);
    return order;
  }
  /** 識別子と時刻を決めて、注文を作る（人が書く） */
  op newOrder(session: type.Session, lines: type.OrderLine[]): type.Order;

  /** 注文したユーザーに通知する */
  op notify(order: type.Order)
    throws type.NotificationFailed
  {
    const notice = composeNotice(order);
    deliver(notice);
    store.addNotification(notice);
  }
  /** 通知の文面を作る（人が書く） */
  op composeNotice(order: type.Order): type.Notification;
  /** 通知を外へ送る。送れなければ NotificationFailed（人が書く） */
  op deliver(notice: type.Notification)
    throws type.NotificationFailed;
}
```

## 設定

```toml
[[emit]]                                   # 共有の型。言語ごとに一度だけ出す（P7）
namespace   = "type"
transpilers = ["go", "go-valid"]
out         = "gen/type"

[[emit]]                                   # DB の状態と、それを読み書きする op
namespace   = "store"
transpilers = ["go", "go-sqlite"]
out         = "gen/store"

[[emit]]                                   # 同じ store を、DB なしで（葉の op は人がメモリ上で埋める）
namespace   = "store"
transpilers = ["go"]
out         = "gen/storemem"

[[emit]]                                   # HTTP の入口と業務の流れ
namespace   = "server"
transpilers = ["go", "go-valid", "go-http"]   # 依存もすべて書く（P18）
out         = "gen/server"

[emit.options.go-http]
validate = true                            # 要求を受けたときに制約を確かめる（P15、P16）
```

## 誰が何を出すか

| 部分 | 出どころ |
|---|---|
| 型、op の関数の型、差し込み口、配線の本体、組み立て | 基底 `go`（P9〜P11） |
| 制約を確かめる関数 | `go-valid`（P15） |
| `store` の op の中身（一行を引く、足す、消す）と、表、準備の関数 | `go-sqlite` |
| HTTP のハンドラ（入力の読み取り、検証、例外からステータスへ） | `go-http` |
| `server` の `checkPassword`、`newSession`、`newOrder`、`pageOf`、`composeNotice`、`deliver` | 人 |
| `gen/storemem` の `store` の op の中身 | 人（メモリ上） |

## 書いていて残った点

1. **省略可能な値の絞り込み**: `in.token` は `type.Token?`。`findSession` の引数を `type.Token?` にして逃げた。値の `T?` も
   標準の分類でその場で絞り込める、と決めた（P20〜P22）。この仕様は、受ける側を `T?` にする書き方のままにしている。
2. **値を運ぶ例外の `throw`**: この仕様には出てこない。書き方は未定（P13）。
3. **Go のパッケージ名**: 名前空間 `type` は Go の予約語と同じなので、そのままではパッケージ名にできない。今の規則
   （`docs/emit-go.md`「配置」）では `pkgtype` になる。
4. **`reads` の伝播**: 配線の op が読む状態は、呼び出し先の `reads` の和として計算できる（効果と同じ）。配線に書かせるかは未定。
5. **共有の型に付く語彙のデコレータ**: `type.User` のフィールドに `@db.key` が付く。書いてよい、と決めた（P23）。
   別の置き場も選べるように、デコレータだけを足す model の派生を考える案がある（未決）:
   `model UserRow is type.User { @db.key id; @db.unique email; }` のように、フィールドは元のまま、デコレータだけを足す。
6. **包む関数（デコレータ）**: この仕様にはプロジェクトが宣言したデコレータがない。次のサンプルで扱う。
