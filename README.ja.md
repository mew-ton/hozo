# Hozo

[English](README.md) | 日本語

## 名前

**Hozo** は、枘（ほぞ）。木の部材の端に作る突起で、もう一つの部材の穴に、はまる。ほぞ接ぎは、木の中身を合わせない。合わせるのは、二つの部材が、合わさる所の形だけで、それで、部材は組み上がる。

Hozo は、同じことを、ソフトウェアの設計にする。中身は、書かない。決めるのは、部品と部品が、合わさる所の形だけ: 何を受け取り、何を返すか。何を読み書きするか。何を、どの順に呼ぶか。

その形が、設計で、Hozo は、それを、**ただ一つの正（SSOT）**として持つ言語。コードの大半を、AI が書くようになっても、設計は、人が持ち続ける。Hozo は、設計を、人が、レビューして、手直しできる形で、持つ。

## 設計の手法を、コードで書く

設計の手法で描くものは、その形の、一つの見え方。Hozo では、どの見え方も、一つの言語で、一つの仕様に書く。手法を開くと、図と、その図が言っていることを書いた Hozo が、出る。

**UML**

<details>
<summary>ユースケース図</summary>

担い手は、分類のケース。ユースケースは、担い手を名指しする関数。

```mermaid
flowchart LR
  Customer([Customer])
  Customer --- signIn([signIn])
  Customer --- order([order])
  Customer --- viewOrders([viewOrders])
```

```
classify Actor of never {
  @prd.actor.human Customer;
}

@prd.usecase.of(Actor.Customer)
export fn signIn();

@prd.usecase.of(Actor.Customer)
export fn order();

@prd.usecase.of(Actor.Customer)
export fn viewOrders();
```

まだ書けない: 広げる（extend）。

</details>

<details>
<summary>アクティビティ図、決定表</summary>

分かれ道は、分類での `match`: どの場合に、何をするか。

```mermaid
flowchart TD
  start([authenticate]) --> c{Credential}
  c -- Presented --> s[sessionOf]
  c -- Absent --> u[throw Unauthorized]
```

```
fn authenticate(token: Token?): Session {
  return match (token by Credential) {
    Presented => sessionOf(token);
    Absent    => throw Unauthorized;
  }
}
```

配線で書けるものは、ほかに: 並行（`disjoint`）と、繰り返し（`for`）。

</details>

<details>
<summary>シーケンス図</summary>

帯は、名前空間。メッセージは、呼び出しで、書いた順に並ぶ。

```mermaid
sequenceDiagram
  participant client
  participant server
  participant store
  participant mailer
  client->>server: placeOrder
  server->>store: saveOrder
  server->>mailer: send
```

```
namespace client {
  export fn order() {
    server.placeOrder();
  }
}

namespace server {
  export fn placeOrder() {
    store.saveOrder();
    mailer.send();
  }
}

namespace store {
  export fn saveOrder();
}

namespace mailer {
  export fn send();
}
```

まだ書けない: 複合フラグメント。

</details>

<details>
<summary>ステートマシン図、状態遷移表</summary>

状態は、分類のケース。遷移は、それを起こす効果が、宣言する: どのケースから（`on`）、どのケースへ（`as`）。

```mermaid
stateDiagram-v2
  [*] --> AwaitingPayment: createOrder
  AwaitingPayment --> Accepted: acceptOrder
  AwaitingPayment --> Expired: expireOrder
```

```
classify Standing of Order {
  AwaitingPayment;
  Accepted;
  Expired;
}

effect fn createOrder(order: Order): effects orders.post as Standing.AwaitingPayment;
effect fn acceptOrder(order: Order on Standing.AwaitingPayment): effects orders.put as Standing.Accepted;
effect fn expireOrder(order: Order on Standing.AwaitingPayment): effects orders.put as Standing.Expired;
```

まだ書けない: ガード、入れ子の状態、履歴。

</details>

<details>
<summary>クラス図</summary>

クラスは、`model`。属性は、フィールド。関連は、フィールドの型。

```mermaid
classDiagram
  class Order {
    string id
    OrderLine[] lines
    datetime placedAt
  }
  class OrderLine {
    string item
    int quantity
  }
  Order --> "*" OrderLine : lines
```

```
model Order {
  id: string;
  lines: OrderLine[];
  placedAt: datetime;
}

model OrderLine {
  item: string;
  quantity: int;
}
```

まだ書けない: 操作、汎化、集約とコンポジションの区別。

</details>

**データと API**

<details>
<summary>ER 図、テーブル定義</summary>

表は、`state`。列、キー、参照は、`model` のフィールドの、デコレータ。

```mermaid
erDiagram
  users ||--o{ orders : userId
  users {
    text id PK
    text email UK
    text name
  }
  orders {
    text id PK
    text userId FK
    json lines
    datetime placedAt
  }
```

```
model User {
  @db.key    @db.text id: string;
  @db.unique @db.text email: string;
  @db.text name: string;
}

model Order {
  @db.key       @db.text id: string;
  @db.ref(User) @db.text userId: string;
  @db.json lines: string[];
  @db.datetime placedAt: datetime;
}

@db.table
state users: list<User>;

@db.table
state orders: list<Order>;
```

</details>

<details>
<summary>API の設計</summary>

経路は、関数のデコレータ。入力を、要求のどこから読むかは、フィールドのデコレータ。

| Method | Path | Header | Body | Response |
|---|---|---|---|---|
| `POST` | `/orders` | `Authorization` | `lines` (required) | `Order` |

```
model PlaceOrderInput {
  @http.header("Authorization") token: string?;
  @http.required @http.body lines: OrderLine[];
}

@http.post("/orders")
export fn placeOrder(in: PlaceOrderInput): Order;
```

</details>

**要求、ドメイン、アーキテクチャ**

<details>
<summary>ユーザーストーリー、要求のトレーサビリティ</summary>

要求は、名前を持つケースで、文は、コメント。設計の側が、応える要求を指すので、どこからも指されていない要求は、穴。

| 要求 | `authenticate` | `placeOrder` |
|---|---|---|
| `PRD_001` ログインしていない人は、注文できない | ✓ | ✓ |
| `PRD_002` 注文したら、その注文が、自分の注文の一覧に出る | | ✓ |

```
classify Requirement of never {
  /** ログインしていない人は、注文できない */
  PRD_001;
  /** 注文したら、その注文が、自分の注文の一覧に出る */
  PRD_002;
}

@prd.ref(Requirement.PRD_001)
fn authenticate();

@prd.ref(Requirement.PRD_001, Requirement.PRD_002)
export fn placeOrder();
```

まだ書けない: 値についての条件としての、受け入れ条件。

</details>

<details>
<summary>イベントストーミング</summary>

コマンドは、関数。事実（イベント）は、効果。読むモデルは、`read`。集約は、状態を持つ名前空間。

```mermaid
flowchart LR
  subgraph ordering
    c[placeOrder]:::command --> e[orderPlaced]:::event --> r[ordersOf]:::read
  end
  classDef command fill:#8ecbff,color:#000
  classDef event fill:#ffb347,color:#000
  classDef read fill:#a8e6a1,color:#000
```

```
namespace ordering {
  state orders: list<Order>;

  export fn placeOrder(order: Order) {
    orderPlaced(order);
  }

  effect fn orderPlaced(order: Order): effects orders.post;

  read fn ordersOf(userId: string): Order[] reads orders;
}
```

図に出ないもの: 方針。仕様では、呼ぶ順。

</details>

<details>
<summary>C4 モデル、コンポーネント図</summary>

部品は、名前空間。口は、外に出した（`export`）関数。依る向きは、名前空間をまたぐ呼び出し。シーケンス図と、同じ仕様: その、別の見え方。

```mermaid
flowchart LR
  client --> server
  server --> store
  server --> mailer
```

```
namespace client {
  export fn order() {
    server.placeOrder();
  }
}

namespace server {
  export fn placeOrder() {
    store.saveOrder();
    mailer.send();
  }
}

namespace store {
  export fn saveOrder();
}

namespace mailer {
  export fn send();
}
```

まだ書けない: こちらと、外の境。

</details>

<details>
<summary>業務フロー図、サービスブループリント</summary>

フローは、ユースケースを、順に呼ぶ関数。レーンは、そのユースケースの、担い手。

```mermaid
flowchart LR
  subgraph Customer
    signIn --> order --> viewOrders
  end
```

```
@prd.business.flow
export fn signInAndOrder() {
  client.signIn();
  client.order();
  client.viewOrders();
}
```

まだ書けない: ステップの間の、分かれ道、待つこと、補償。

</details>

<details>
<summary>決定の記録（ADR）</summary>

決定は、名前を持つケース。背景と、採らなかった案は、コメントの中のタグ。ほかの決定との関係は、デコレータ。

> **480. コメントの中に、タグとして書けるのは、注記だけ**
>
> 直す決定: 478
>
> 背景: そうしないと、振る舞いを決めるデコレータが、コメントの中から、本当に、付く。
>
> 採らなかった案: 今のまま。

```
/**
 * コメントの中に、タグとして書けるのは、注記だけ。
 *
 * @adr.context そうしないと、振る舞いを決めるデコレータが、コメントの中から、本当に、付く。
 * @adr.rejected 今のまま。
 */
@adr.amends(Decision.D478)
D480;
```

</details>

図は、このページのために、手で描いたもの: 仕様から図を描く UI は、まだ、ない。部品ごとの対応は、[`docs/ide-chain.md`](docs/ide-chain.md)「対応」。

## 書いた設計が、何になるか

`hozoc` が、仕様を検査して、コードの枠と、テストのハーネスにする。

- 配線は、コードとして、生成される。
- 中身のない関数は、人か AI が書く。実装が欠けていれば、生成したコードは、コンパイルできない。
- 仕様が求めるテストは、仕様から決まり、それが走ったことを、`hozoc` が確かめる。

最初の出力先の言語は、Go。

## 状態

Hozo は、未公開で、版は 0.x。版から版への互換は、まだ、何もない。

- **あるもの**: コンパイラ `hozoc`、言語サーバー、VS Code 拡張、Go のプラグイン。
- **まだ、ないもの**: 設計を、描いて、レビューして、手直しする UI。Go のほかの言語への出力。ライセンス。

## 次に読むもの

| したいこと | 読む場所 |
|---|---|
| 何のための言語かを知る | [`docs/requirements.md`](docs/requirements.md)「価値と位置づけ」 |
| 仕様を書く | [`docs/grammar.md`](docs/grammar.md)、[`docs/requirements.md`](docs/requirements.md)「書き方の指針」 |
| `hozoc` を動かす | [`docs/cli.md`](docs/cli.md)。`make build` で、`bin/` にできる（Go 1.24.7 以上） |
| プラグインを書く | [`docs/plugin-protocol.md`](docs/plugin-protocol.md) |
| Hozo 自身を直す | [`CLAUDE.md`](CLAUDE.md)、続けて [`docs/README.md`](docs/README.md) |

文書は、今は、日本語。処理系の第一言語である、英語に、移す。
