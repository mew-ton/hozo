# Hozo

[English](README.md) | 日本語

## 名前

**Hozo** は、枘（ほぞ）。木の部材の端に作る突起で、もう一つの部材の穴に、はまる。ほぞ接ぎは、木の中身を合わせない。合わせるのは、二つの部材が、合わさる所の形だけで、それで、部材は組み上がる。

Hozo は、同じことを、ソフトウェアの設計にする。中身は、書かない。決めるのは、部品と部品が、合わさる所の形だけ: 何を受け取り、何を返すか。何を読み書きするか。何を、どの順に呼ぶか。

その形が、設計で、Hozo は、それを、**ただ一つの正（SSOT）**として持つ言語。コードの大半を、AI が書くようになっても、設計は、人が持ち続ける。Hozo は、設計を、人が、レビューして、手直しできる形で、持つ。

## 設計の手法を、コードで書く

設計の手法で描くものは、その形の、一つの見え方。Hozo では、どの見え方も、一つの言語で、一つの仕様に書く。

| 設計の手法 | Hozo で書くもの |
|---|---|
| 要求の一覧、トレーサビリティマトリクス | 要求は、分類のケース。設計とは、`@prd.ref` で結ぶ |
| ユースケース図、ユースケース記述 | 担い手は、分類のケース。ユースケースは、`@prd.usecase.of` の付いた関数。流れは、その関数の配線 |
| 業務フロー図 | `@prd.business.flow` の付いた関数の配線: ユースケースを、順に呼ぶ |
| アクティビティ図、シーケンス図、決定表 | 配線: 呼ぶ順、分類での `match`、`disjoint`、`for` |
| ステートマシン図、状態遷移表 | 分類のケースと、効果が宣言する、遷移 |
| クラス図、ER 図 | `model`、`state`、`@valid.*`、`@db.*` |
| コンポーネント図、API の設計 | 名前空間、`export`、`@http.*` |

対応の全体と、Hozo が、まだ書けない部品は、[`docs/ide-chain.md`](docs/ide-chain.md)「対応」。

サンプル [`tests/samples/shop-server`](tests/samples/shop-server) からの、省いた抜き書きを、三つ:

```
// ユースケース図と、業務フロー: 誰が、何を、どの順にするか。
classify Actor of never {
  @prd.actor.human Customer;
}

@prd.usecase.of(design.Actor.Customer)
export fn order(token: type.Token, lines: type.OrderLine[]): type.Order {
  return server.placeOrder({ token: token, lines: lines });
}

@prd.business.flow
export fn signInAndOrder(email: type.Email, password: type.Password, lines: type.OrderLine[]): Ordered {
  val token = client.signIn(email, password);
  val placed = client.order(token, lines);
  val listed = client.viewOrders(token);
  return { placed: placed, listed: listed };
}
```

```
// アクティビティ図と、決定表: どんな場合があり、それぞれで、何をするか。
classify Credential of type.Token? {
  Presented: type.Token? as type.Token;
  Absent:    type.Token? as else;
}

@prd.ref(design.Requirement.PRD_001)
fn authenticate(token: type.Token?): type.Session {
  return match (token by Credential) {
    Presented => sessionOf(token);
    Absent    => throw type.Unauthorized;
  }
}
```

```
// クラス図と、ER 図: もの、その制約、しまう場所。
model User {
  @db.key    @db.text id: UserId;
  @db.unique @db.text email: Email;
  @db.text @valid.length(min: 1, max: 100) name: string;
}

@db.table
state users: list<type.User>;
```

## 書いた設計が、何になるか

`hozoc` が、仕様を検査して、コードの枠と、テストのハーネスにする。

- 配線は、コードとして、生成される。
- 中身のない関数は、人か AI が書く。実装が欠けていれば、生成したコードは、コンパイルできない。
- 仕様が求めるテストは、仕様から決まり、それが走ったことを、`hozoc` が確かめる。

最初の出力先の言語は、Go。仕様と、そのコードの一組は、[`tests/samples/shop-server`](tests/samples/shop-server)。

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
