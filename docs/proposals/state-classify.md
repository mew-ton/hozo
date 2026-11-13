# 提案: 状態を分類で表し、効果をその変化として宣言する

## この文書が決めること

言語の仕様の変更の提案。状態の「場合」を `classify` で名付け、`effect` がその変化を言えるようにし、配線が状態の場合で分岐できるようにする。まだ決定ではなく、決まったら設計書（`mew-ton/drafts` の `hozo/README.md`。「§n」はその節）の §5.2・§6.2・§10・§12・§16 と、このリポジトリの `docs/grammar.md`・`docs/ir.md`・`docs/resolve.md`・`docs/check.md`・`docs/emit-go.md` に移してこの文書は消す。「決めること」に残っている項目が決まるまで実装しない。

## 動機

値については §10.1 が「結果の種類で名付けた排他なケース」を徹底しているが、状態の場合分けは名前を持たずに三か所へ漏れている。

- 例外の名前: §7.1 の `AlreadyLoggedIn`、§5.2 の `UserHasOrders` は「呼ばれた時点で状態がどの場合だったか」だが、どの状態のどの場合かは宣言にない。
- 効果の説明文: §6.2 の `effect removeToken on token.put; // none を入れる` は、どの場合からどの場合になるかをコメントに逃がしている。
- 配線: §10.2 の `match` が分類できるのは値だけで、状態の場合では分岐できない。

## モデル

### 基底の状態

基底の状態は、列を持つ入れ物。形は列数の範囲で三つあり、形ごとに組み込みの分類と組み込みの効果が決まる。

| 形 | 列数 | 組み込みの分類 | 組み込みの効果 |
|---|---|---|---|
| `list<T>` | 0 以上 | `Empty` / `HasItems` | `post` / `put` / `delete` |
| `nullable<T>` | 0 か 1 | `Empty` / `HasItems` | `post` / `put` / `delete` |
| `single<T>` | ちょうど 1 | なし（常に一列） | `put` |

- 組み込みの効果は列数の相対的な変化。`post` は増える、`put` は変わらない、`delete` は減る。
- `single` に `post` / `delete` が書けないのは、列数が固定だから（今の STATE-1 と同じ結論）。
- 組み込みの効果が組み込みの分類をどう変えるかは、形から決まる。

  | 形 | `post` | `put` | `delete` |
  |---|---|---|---|
  | `list` | どの場合でも → `HasItems` | `HasItems` → `HasItems` | `HasItems` → `Empty` か `HasItems` |
  | `nullable` | `Empty` → `HasItems` | `HasItems` → `HasItems` | `HasItems` → `Empty` |

### 分類の状態

`classify` は、一つ以上の基底の状態を対象にして、その場合に名前を付ける。これも状態で、基底の状態の「見え方」に当たる。

- 値の分類（§10.1）と同じく、名前と説明だけを宣言し、判定は述語として人が実装する。排他・全集合・`default` も同じ。
- 複数の基底の状態にまたがる場合は、対象に並べる（`classify Checkout of carts, orders`）。
- 宣言した名前空間（＝対象の状態の持ち主）の中でだけ使える。§3.2 の「`classify` は宣言した名前空間の中でだけ使える」と、§5.1 の「状態は持ち主の外には出ない」が同じ結論になる。

### 効果

**一つの効果が変えるのは、一つの状態の一つの変化だけ。** 宛先は次のどちらか。

- 基底の状態の組み込みの効果（今の書き方）: `effect addCartLine on carts.post;`
- 分類の状態の、場合から場合への遷移: `effect placeOrder on Checkout: Shopping -> Ordered;`

実装が二つの基底の状態を同時に変える効果（注文を作ってカートを空にする、など）は、その二つに依存する分類の状態を宣言し、その遷移として書く。効果を二つの組み込みの効果の組として書くことはできない。

## 書き方

```ts
namespace browser {
  @localStorage
  state token: nullable<server.Token>;

  /** 手元のトークンを消す（HasItems → Empty） */
  effect removeToken on token.delete;

  op openHome(): HomePage {
    return match token by nullable {
      HasItems => loadHome();
      Empty    => guestHome();
    };
  }
}

namespace server {
  @db
  state carts: list<CartLine>;
  @db
  state orders: list<Order>;

  /** カートに一行足す */
  effect addCartLine on carts.post;

  /** 買い物の進み具合。カートと注文の両方から決まる */
  classify Checkout of carts, orders {
    /** カートに商品があり、まだ注文していない */
    Shopping;
    /** 注文を作り、カートは空になった */
    Ordered;
    default;
  }

  /** 注文を確定する。注文を作り、カートを空にする */
  effect placeOrder on Checkout: Shopping -> Ordered;
}
```

- 組み込みの分類は形の名前で指す（`match carts by list`、`match token by nullable`）。
- 遷移の前と後は、その分類のケース。ケースの和（`A | B`）と `any` も書ける。
- `match <状態> by <分類>` の対象は、今の名前空間の基底の状態。分類の状態で分岐するなら `match by Checkout { ... }` と対象を省く（対象は分類の宣言が持っている）。

## 意味

### 述語と判定関数

- 状態の述語は、状態を読む判定関数に訳される（`carts` の `Empty` なら `isCartsEmpty` に当たるもの）。値の分類の述語が純粋な関数なのに対し、状態の述語は保存先への問い合わせで、人が実装する。
- 判定関数は二つの場面で呼ばれる。配線の `match` の実行時と、テストで効果の結果を確かめるとき（「検証」）。配線で使われない分類でも、テストのために宣言する意味がある。

### 効果が書き込む状態

- 組み込みの効果なら、その基底の状態。
- 遷移なら、その分類が対象にするすべての基底の状態。
- 持ち主の検査（§6.2「持ち主ではない名前空間の葉の op がその効果を `effects` に宣言すればエラー」）は、この集合で行う。

### 検証

§12.1 の「設計どおりかのアサーション」に、効果の結果を足す。

| 効果の宛先 | アサーション |
|---|---|
| 組み込みの効果（`list` / `nullable`） | 呼び出しの前後に組み込みの分類の判定関数を呼ぶ。効果が起きたなら「モデル」の表の通りの変化。起きていない（`prevents`）なら場合は変わっていない |
| 分類の状態の遷移 | 呼び出しの前後にその分類の判定関数を呼ぶ。効果が起きたなら、前は遷移の前のケース、後は後のケース。起きていないなら場合は変わっていない |

これにより、§7.3 の `prevents` / `keeps` が実行時に確かめられる意味を持つ。§12.2 が「Hozo の外」としている「効果の観測（状態がどう変わったかの検証）」のうち、分類の場合で言える範囲は Hozo の中に入る。値そのものの検証（何がセットされたか）は外のまま。

## 設計書の廃案（§16）との関係

`nullable<T>` は、§16 の二つの廃案を覆す。

| 廃案 | 当時の理由 | 今回 |
|---|---|---|
| single の `clear` を `single<T?>` にだけ許す | 使える操作が型引数の `?` の有無で変わるのはおかしい | 操作は形だけで決まる（`nullable` という形を立てる）ので、当時の理由に当たらない |
| 空になりうる状態を別の形（`maybe<T>`）にする | 形が増え、「空」が入れ物と値の型の二か所に現れる | 形は増える。「空」が二か所に現れるかは「決めること」の 1 で決まる |

## 既定の出力（Go）

- 分類の状態ごとに、人が実装する述語のインターフェースと判定関数を出す。述語は `func(ctx context.Context) (bool, error)`。値の分類（`docs/emit-go.md`「classify」）との違いは、引数に値を取らず、失敗しうること。
- 組み込みの分類は `Empty` の述語一つで足りる（`HasItems` はその否定）。`States` に `<状態>IsEmpty(ctx) (bool, error)` を足す。
- `match` は、判定関数を呼んで `switch` する（`docs/emit-go.md`「match」と同じ形）。
- `nullable<T>` の状態の取得（`bind` 用）は `*T` を返す。今の `single<T?>` と同じ。

## 実装への影響

`effect ... on s.post` の構文と IR は変わらない。足すものと、`single<T?>` から `nullable<T>` への置き換え。

| 文書・パッケージ | 変わること |
|---|---|
| `docs/grammar.md`、`internal/syntax` | 状態の形に `nullable`。`classify ... of` の対象に状態の並び。`EffectDecl` に遷移の形と記号 `->`。`match` の `by` の後ろに形の名前、対象の省略 |
| `docs/ir.md`、`ir` | `State.shape` に `nullable`。`Classify.subject` が型か状態の列か。`Effect` の宛先が組み込みか遷移か。`Match.subject` が状態でもよい |
| `docs/resolve.md`、`docs/types.md`、`internal/sema` | 上の解決。状態の `match` にはアームでの絞り込みがない |
| `docs/check.md`、`internal/check` | STATE-1 を三つの形に。遷移のケースの所属。持ち主の検査を「効果が書き込む状態」で。`bind` の「省略可能な状態」は `nullable` の形で判定（BIND-2） |
| `docs/emit-go.md`、`internal/emit/golang` | 「既定の出力（Go）」の四つ |
| `internal/lsp`、`editors/vscode` | ホバーと色（`->`、`nullable`）。小さい |
| `tests/corpus/*`、設計書の例 | `single<Token?>` を `nullable<server.Token>` に、`removeToken on token.put` を `token.delete` に |
| `cli.hozo` | `state config: single<SourceFile?>` を `nullable<SourceFile>` に |
| `plugins/go-cli`、標準の語彙 | 変わらない |

`Sig(O, v)` の計算（`docs/check.md`「計算」）は変えない。

## 決まったこと

- 組み込みの分類のケースは `Empty` / `HasItems`。分類そのものは形の名前（`list`、`nullable`）で指す。
- 一つの効果が変えるのは一つの状態の一つの変化。複数の基底の状態に及ぶ変化は、それらに依存する分類の状態の遷移として書く。
- 状態の述語は判定関数に訳され、配線の `match` と、テストでの効果の結果の判定で呼ぶ。

## 決めること

1. **`T?` と `nullable<T>` の関係**。三つの読み方がある。
   - A: 状態は `nullable<T>` とだけ書く。`single<T?>` はエラー。「空」は入れ物にだけ現れる。
   - B: `single<T?>` を `nullable<T>` の別名として残し、IR で `nullable` にそろえる。
   - C: 型の `T?` そのものを `nullable<T>` の別名とみなし、省略可能な値も `Empty` / `HasItems` を持つ（`match warranty by nullable { ... }` が値にも書ける）。
   提案は、状態については A（`docs/grammar.md`「決めたこと」の「同じ意味の書き方を二つ持たない」と同じ理由）。C は値の側の拡張として筋が通るが、別の決定として扱う。
2. **検査器が遷移をどこまで使うか**。提案は、所属と組み込みの関係の確認、IR への搭載、「検証」のアサーションまで。配線の行をまたぐ追跡（効果の後の `match` で到達しないアームの検出など）は、§7.3 の「不明」と絡むので後で別に決める。
3. **例外と場合の結びつけ**。`AlreadyLoggedIn` のような例外が「前の場合が合わなかった」ことを表す、と書けるようにするか。§16 の「状態 × 入力の表で分岐を網羅させる」に近づくので、この提案には含めない。
4. **リストの一部を対象にした分類**。`carts` の全体ではなく「このユーザーのカート」の場合を言いたいとき、述語は引数（どのユーザーか）を要る。提案の範囲は全体の分類だけで、部分は葉の op で値を取ってから値の分類で書く（今のまま）。
5. **分類の状態を対象にした分類**。`classify ... of` の対象に、基底の状態だけでなく別の分類の状態も並べられるか。提案は基底の状態だけ（依存を一段にして、効果が書き込む状態を宣言から直接読めるようにする）。
