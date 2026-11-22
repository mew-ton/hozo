# サンプル `order-app`

注文を受けて、作って、そろえて、送る店のアプリ（サーバーと、客の画面と、担当者の画面）を、**仕様だけ**で書いたもの。生成するコードも、期待する出力も、人が書くコードも持たない。サンプルとは何か、置き方、テストの使い方は [`../README.md`](../README.md)（仕様だけのサンプルは、同「仕様だけのサンプル」）。

このサンプルが答えようとしている問いは一つ: **仕様は、どこまで書くのか**。「PRD の概念と、その境界だけを最低限で決めて、必要になったら足す」という立場の、「必要」の線を、本物の大きさのアプリを書いて探す。この文書が持つのは、作るもの（PRD）、名前空間の地図、難しい問題のモデル、書いて分かったこと、粒度についての事例と規則の候補、利用者への問い。**規則は、どれも決めていない**（決めるのは利用者。「利用者への問い」）。

名前を `order-app` にしたのは、`shop-server`（出すものの名前）と違い、出すものがなく、サーバーと二つの画面を一つの仕様に持つから。

## 作るもの

### 何をする店か

手作りの品と、その材料や道具と、作り方の教材（ファイル）を売る店。客はアカウントを作らず、注文ごとの鍵（`type.OrderKey`）で、自分の注文を見る。

### 商品の三つの種類

一つの注文に、三つの種類が混ざる。種類の持ち主は `domain/catalog.hozo` の `Fulfilment`。

| 種類 | 渡し方 | 大きさと重さが決まるとき |
|---|---|---|
| ダウンロード（`DigitalGoods`） | 注文を受け付けたときに、取る権利を足す。箱は要らない | — |
| 受注製作（`MadeGoods`） | 注文を受け付けてから作る。出来上がったら量って、そろえ場所に置く | 作り終えたとき。注文のときに分かるのは、いちばん大きく出来た場合（`largest`）だけ |
| 在庫（`StockedGoods`） | 注文のときに取り置き、受け付けたら棚から出して、そろえ場所に置く | 商品に登録してある（`unit`） |

### 担い手

| 担い手 | すること | 仕様の中の場所 |
|---|---|---|
| 客 | 商品を選ぶ、値段を見る、上限の額に同意する、注文のいまを見る、ファイルを取る | `web/storefront.hozo` |
| 製作の担当 | 仕事を始める、作り終えて量る | `web/backoffice.hozo` の「製作」 |
| 梱包の担当 | 棚から出す、箱を選んで量る、配送業者を選ぶ、渡す | `web/backoffice.hozo` の「そろえる」「梱包の台」 |
| 管理の担当 | 配送業者を登録する、使う・使わないを切り替える | `web/backoffice.hozo` の「配送業者の管理」 |
| 支払いを引き受ける仕組み | 支払いの画面、枠の確保、請求の確定、返金、通知 | `external/psp.hozo` |
| 配送業者の API をまとめる仕組み | 見積もり、送り状、荷物の動きの通知 | `external/carriergw.hozo` |
| ファイルの置き場、メールの配送 | 期限付きの URL、メール | `external/assets.hozo`、`external/mailer.hozo` |
| 時計 | 支払いのない注文の期限切れ、枠の期限の見張り | `vocab/clock.hozo` の `tick` を付けた op |

### 流れ

1. 客が、カートの値段を見る。箱で送るものがあれば、届け先を入れると、送料の**見積もり**と**上限**が出る（`ordering.price`）。
2. 客が、見えている上限に同意して注文する。在庫の行を取り置く（`ordering.place`）。
3. 客が、支払いの画面（外部）で、**商品の代金 + 送料の上限**の額に同意する。箱で送るものがあれば、その額の枠を確保するだけで、金は動かない（`payment.begin`）。
4. 同意の通知が届いたら、注文を受け付けて、種類ごとに分ける（`fulfilment.secured`、`fulfilment.begin`）: ダウンロードは渡し終える。受注製作は仕事にする。在庫は棚から出してよくする。箱で送る二つは、一つの出荷にまとめる。
5. 担当が、作り終えた品目と、棚から出した品目を、そろえ場所に置く（`fulfilment.made`、`fulfilment.picked`）。
6. すべてそろったら、梱包の担当が箱を選んで量る。**送料が、ここで決まる**（`shipping.pack`）。
7. 梱包の担当が配送業者を選ぶ。客への請求を確定してから、送り状を買う（`fulfilment.dispatch`）。
8. 渡したら、客に、決まった送料と、確定した請求と、追跡番号を知らせる（`fulfilment.handedOver`）。

### 作らないもの

受け付けた後のキャンセルと返品、分けて送ること、商品・箱・料金表・在庫の数・担当者を登録する画面、客のアカウント。分けて送らない理由は「難しい問題」の「分けて送ること」。

## 入力

`hozo.toml` は `sources` だけを持ち、出力の単位（`[[emit]]`）を置かない。置き場は、担い手の種類ごとのディレクトリ。

| ディレクトリ | 中身 |
|---|---|
| `type.hozo` | 共有の値の型（名前空間 `type`） |
| `domain/` | サーバーの概念。名前空間ごとに一つのファイル |
| `api/` | サーバーの HTTP の入口。呼ぶ相手ごとに一つの名前空間 |
| `external/` | 外部の仕組み。相手の口を、こちらが呼ぶ op として書いたもの |
| `web/` | ブラウザの画面 |
| `vocab/` | このサンプルの語彙（「語彙」） |
| `sample.json` | `{ "specOnly": true }`（`../README.md`「sample.json」） |

## 名前空間の地図

27 の名前空間、約 4,300 行。op は 262（別の名前空間から呼べる `export` が 96、配線が 184、人が書く葉が 78）、状態は 35、`read` は 34、効果は 64（そのうち、行の遷移を持つものが 22）。数は、この文書を書いたときのもの。

「op」の列は「全部（`export` / 配線 / 葉）」。「read」と「効果」の括弧は、`db` の語彙のデコレータで中身を言えた数。

| 名前空間 | 概念 | 状態 | read | 効果 | op | 担い手 |
|---|---|---|---|---|---|---|
| `type` | 共有の値（識別子、金額、住所、寸法）と、認証の例外 | — | — | — | — | — |
| `catalog` | 商品と、その種類 | `products` | 3（2） | 0 | 3（3 / 3 / 0） | サーバー |
| `ordering` | 注文: 何を頼み、何を約束されたか | `orders`、`paymentWindow` | 4（3） | 3（1） | 20（7 / 14 / 6） | サーバー |
| `inventory` | 在庫の数と、取り置き | `levels`、`reservations` | 5（3） | 5（1） | 13（5 / 8 / 5） | サーバー |
| `payment` | 支払い: 何に同意し、いくら請求したか | `payments`、`seen`、`holdMargin` | 4（3） | 8（2） | 31（5 / 22 / 9） | サーバー |
| `production` | 受注製作の仕事 | `jobs` | 3（2） | 3（0） | 7（5 / 6 / 1） | サーバー |
| `download` | ダウンロードの権利 | `grants`、`grantLifetime`、`linkLifetime` | 2（2） | 1（0） | 7（3 / 5 / 2） | サーバー |
| `shipping` | 出荷: そろえる、梱包する、渡す。箱と料金表 | `shipments`、`parcels`、`labels`、`boxes`、`tariffs`、`origin` | 7（6） | 6（3） | 30（14 / 24 / 6） | サーバー |
| `carrier` | 契約している配送業者と、その使い分け | `carriers` | 3（3） | 2（1） | 12（5 / 9 / 3） | サーバー |
| `staff` | 担当者と、してよい仕事 | `members`、`sessions` | 3（3） | 2（1） | 14（5 / 10 / 4） | サーバー |
| `notice` | 客への知らせ | — | — | — | 4（2 / 2 / 2） | サーバー |
| `fulfilment` | 受け付けた後の流れ。概念をまたぐ順番だけ | — | — | — | 15（7 / 15 / 0） | サーバー |
| `shopApi` | 客の画面のための入口 | `site` | — | — | 13（8 / 11 / 2） | サーバー（HTTP） |
| `staffApi` | 担当者の画面のための入口 | — | — | — | 15（15 / 15 / 0） | サーバー（HTTP） |
| `hooks` | 外部の仕組みからの通知の入口 | — | — | — | 2（2 / 2 / 0） | サーバー（HTTP） |
| `psp` | 支払いを引き受ける仕組み | `ledger`、`holds`（相手のもの） | — | 3 | 5（5 / 0 / 5） | 外部 |
| `carriergw` | 配送業者の API をまとめる仕組み | `labels`（相手のもの） | — | 1 | 2（2 / 0 / 2） | 外部 |
| `assets`、`mailer` | ファイルの置き場、メールの配送 | — | — | — | 1 ずつ | 外部 |
| `storefront` | **客の画面** | `cart`、`lastOrder`、`offer`、`problem`、`location`、`query` | — | 17 | 33（0 / 20 / 13） | ブラウザ |
| `backoffice` | **担当者の画面** | `token`、`problem`、`location`、`query` | — | 13 | 33（0 / 18 / 15） | ブラウザ |
| `clock`、`nav`、`outside`、`pact`、`smtp`、`webhook` | このサンプルの語彙 | — | — | — | `clock.now` だけ | — |

名前空間どうしの呼び出しの向き:

```
storefront ──▶ shopApi ─┐                       ┌─▶ catalog
backoffice ──▶ staffApi ├─▶ fulfilment ─────────┼─▶ ordering ──▶ catalog, inventory, shipping
psp, carriergw ─▶ hooks ┘     │                 ├─▶ payment  ──▶ psp
                              │                 ├─▶ production
                              │                 ├─▶ download ──▶ assets
                              │                 ├─▶ shipping
                              │                 ├─▶ carrier  ──▶ carriergw
                              │                 └─▶ notice   ──▶ mailer
                              └─（入口は、流れを通さずに、概念の op を直に呼ぶこともある）
```

## 画面

画面は、ページの op（`@browser.page.route`。返す union のアームが、画面の状態）と、操作の op（`@browser.page.interaction`。どのページの、どのアームでできるか）で書いた。描き方（部品、文言、見た目、入力の途中の状態）は書いていない。

### 客の画面（`web/storefront.hozo`）

| ページ | アーム（画面の状態） | 操作 |
|---|---|---|
| `/` `openCatalog` | `Ready`、`Empty`、`Unavailable` | — |
| `/product` `openProduct` | `Ready`、`Missing`、`Unavailable` | `Ready`: `onAddToCart`（受注製作なら、希望を聞くダイアログ） |
| `/cart` `openCart` | `Filled`、`Empty`、`Outdated`、`Unavailable` | `Filled`: `onChangeQuantity`、`onRemoveLine`、`onProceed`。`Outdated`: `onStartOver` |
| `/checkout` `openCheckout` | `Entering`、`Quoted`、`EmptyCart`、`Outdated`、`Unavailable` | `Entering`: `onAskQuote`。`Quoted`: `onEditContact`、`onPlaceOrder` |
| `/order` `openOrder` | `AwaitingPayment`、`Accepted`、`Expired`、`Missing`、`Unavailable` | `AwaitingPayment`: `onPay`、`onRefresh`。`Accepted`: `onDownload` |

### 担当者の画面（`web/backoffice.hozo`）

| ページ | アーム | 操作 |
|---|---|---|
| `/login` `openLogin` | `Form` | `onLogin` |
| `/production` `openProduction` | `Ready`、`Empty`、`Unavailable` | `Ready`: `onStartJob`、`onFinishJob`（量った大きさを聞くダイアログ） |
| `/picking` `openPicking` | `Ready`、`Empty`、`Unavailable` | `Ready`: `onPicked` |
| `/shipments` `openShipments` | `Ready`、`Empty`、`Unavailable` | `Ready`: `onOpenStation` |
| `/station` `openStation` | `Waiting`、`ChooseBox`、`ChooseCarrier`、`HandOver`、`Gone`、`Unavailable` | `ChooseBox`: `onPack`。`ChooseCarrier`: `onBuyLabel`。`HandOver`: `onHandOver` |
| `/carriers` `openCarriers` | `Ready`、`Empty`、`Unavailable` | `Ready`: `onSwitchCarrier`。どのアームでも: `onRegisterCarrier` |

## 語彙

このサンプルが宣言した語彙。どれも読むトランスパイラがない。プラグインのディレクトリではなく、プロジェクトの仕様（`vocab/*.hozo`）として置いた理由は、「分かったこと」の F22。

| 語彙 | 宣言 | 何を言うか | 検査に効くか |
|---|---|---|---|
| `outside` | `does(...targets)` | 外部の仕組みが、その中で行う効果 | 効く。付いた op の効果に加わる（`docs/check.md`「デコレータが運ぶ reads と effects」） |
| `outside` | `failsAs(error)` | 相手に届かない失敗（`http.NetworkError`、`http.ServerError`）を、別の例外に言い換える | 効く（`catches … recovers`、`throws`）。金が動く呼び出しには使えなかった（F5） |
| `clock` | `system`、`tick(every)`、op `now()` | 時計を読む op、誰にも呼ばれずに動く op | 印。`now()` は、時刻を配線に出すための葉 |
| `webhook` | `signed(sender)`、例外 `BadSignature` | 外部の仕組みが呼ぶ入口で、署名を確かめる | 効く（`throws BadSignature prevents effects`） |
| `pact` | `promise(whom, what)` | その op が守る、PRD の約束 | 印。約束の中身は確かめない（F11）。標準の語彙 `prd` には、足せないので、自分の名前空間に置く（`docs/vocab.md`「語彙の名前空間」） |
| `smtp` | `sendMail` | SMTP でメールを送る op | 印 |
| `nav` | `leave`、`save` | ブラウザを、アプリの外へ移す。ファイルを保存させる | 印 |

画面の名前空間は、それぞれ、失敗を画面の状態にするデコレータ `recover(error, then)`（`catches error halts then`）を持つ。

## 難しい問題

### 何が難しいか

三つの事実が、順番を決める。

1. 箱は、送る品目がすべてそろうまで決められない（受注製作の品目は、在庫の品目より後に出来上がり、大きさは作り終えるまで決まらない）。
2. 送料は、箱の大きさで決まる。
3. だから、注文のときには、請求する額が決まらない。決まるのは、梱包した後。

客は、額が決まる前に注文する。店は、額が決まる前に、作り始める。

### 選んだ形: 上限に同意して、後で確定する

| いつ | 客に見せるもの | 金 | 仕様の場所 |
|---|---|---|---|
| 注文の前 | 商品の代金、送料の**見積もり**（ふつうに出来て、いちばん小さく詰めたとき）と**上限**（いちばん大きく出来たとき）、「送料は梱包の後に決まり、請求は、商品の代金 + 上限を超えない」 | — | `shipping.estimate`、`shipping.FeeQuote`、`storefront.CheckoutPage` の `Quoted` |
| 注文 | 客が同意した上限（`Checkout.agreedCeiling`）が、サーバーがいま出す上限と違えば、注文しない（`ordering.TermsChanged`） | — | `ordering.draft` |
| 支払いの画面 | 商品の代金 + 送料の上限 | **枠の確保**だけ。金は動かない。ダウンロードだけの注文は、送料がないので、その場で請求が確定する | `payment.openSession`、`psp.openHold`、`psp.openCharge` |
| 梱包の前 | 「見積もり ○ 円（上限 ○ 円）。梱包の後に決まります」 | — | `shopApi.ShippingView` |
| 梱包 | — | 送料が決まる: 箱の大きさの区分 × 届け先の地帯（店の料金表） | `shipping.pack`、`shipping.Parcel.fee` |
| 送り状を買う直前 | — | **請求の確定**: 商品の代金 + min(送料, 上限)。残りの枠は、相手が手放す | `fulfilment.dispatch`、`payment.settle` |
| 渡した後 | 決まった送料、確定した請求、追跡番号 | — | `notice.shipped` |

支払いを引き受ける仕組みに当てにしている振る舞い（枠の確保、枠ごとに一度だけの確定、確保した額以下、一部の返金、二度届く通知）の持ち主は、`external/psp.hozo` の冒頭のコメント。

客に請求する送料（`Parcel.fee`。店の料金表）と、店が配送業者に払う額（`Label.cost`）は、別のもの。配送業者を選ぶのは店の都合で、客の額を変えない。

### 送料が、見せた額と違ったとき

| 決まった送料 | すること | 客の同意 |
|---|---|---|
| 見積もりより低い | 決まった額を請求する | 要らない |
| 見積もりより高く、上限以下 | 決まった額を請求する。発送の知らせに、理由を書く | 要らない（上限に同意している） |
| 上限を超える | 上限までを請求する。超えた分は店が持つ（`Payment.absorbed`）。梱包の担当には、箱を選ぶ前に、超える額を見せて確かめる（`shipping.BoxOption.overCeiling`、`backoffice.onPack` の `confirmAbsorb`） | 要らない |

### 枠の期限と、納期

枠には期限がある。受注製作が長引くと、送料が決まる前に、枠が切れる。時計で動く `payment.prepayExpiring` が、期限の近い支払いを、**上限の額で先に確定**する（`Standing.Prepaid`）。送料が決まったら、`payment.settle` が、差額を返す。商品の納期（`catalog.MadeGoods.leadTime`）が枠の期限より短いことは、コメントにしかない（F26）。

### モデル: 事実が、足されていく

出荷の進み具合は、**足された事実**で持つ。前の事実がなければ、次の事実を作る op に渡せる値が、手に入らない。事実を足す効果は、行の遷移（`docs/check.md`「行の遷移」）で、どのケースの行に足せるかを言う: 品目を置けるのは、まだ待っている出荷（`recordPlacement`: 行の引数は `Shipment on Gathered.Awaiting`、後は `as else`）、渡せるのは、店にある出荷（`recordHandOver`: `Shipment on Custody.InHouse` を、`as Custody.WithCarrier`）、荷物は、`Packed` で入る（`createParcel`: `post as Packing.Packed`。`post` の前は、いつも「行がない」ので、書かない。荷物がまだないことは、`shipping.pack` の `match` が確かめる: F57）。「行の遷移で書いた進み具合」に、一覧がある。

```
shipments（何を待っているか）──[すべてそろった: Gathered.Complete]──▶ parcels（箱、重さ、送料）
        ──[荷物がある: Packing.Packed]──▶ 請求の確定 ──▶ labels（配送業者、追跡番号）
        ──[送り状がある: Labelling.Labelled]──▶ handedOverAt ──▶ deliveredAt
```

`domain/shipping.hozo` から（説明のコメントを省いた抜粋）:

```ts
classify Gathered of Shipment {
  Complete;      // すべての品目が、そろえ場所にある
  Awaiting;      // まだ待っている品目がある
}

// そろっていない出荷は、引数に渡せない
export fn boxOptions(shipment: Shipment on Gathered.Complete): BoxOption[] {
  return fitting(shipment, boxes, tariffs);
}

@pact.promise("店", "箱は、送る品目がすべてそろうまで決めない")
@pact.promise("客", "送料は、梱包した箱の大きさと届け先だけで決まる")
@db.transaction
export fn pack(id: ShipmentId, box: BoxCode, weight: type.Grams): Parcel
  throws NoShipment, NotGathered, AlreadyPacked, UnknownBox, OverWeight, Undeliverable
{
  val shipment = find(id);
  val existing = parcelOf(id);
  return match (existing by Packing) {
    Packed   => throw AlreadyPacked;
    Unpacked => packGathered(shipment, box, weight);
  };
}

fn packGathered(shipment: Shipment, box: BoxCode, weight: type.Grams): Parcel
  throws NotGathered, UnknownBox, OverWeight, Undeliverable
{
  return match (shipment by Gathered) {
    Complete => packInto(shipment, box, weight);   // ここでだけ Shipment on Gathered.Complete になる
    Awaiting => throw NotGathered;
  };
}
```

`domain/fulfilment.hozo` から。請求の確定が、送り状より先:

```ts
@pact.promise("店", "請求を確定できない荷物には、送り状を買わない")
export fn dispatch(id: shipping.ShipmentId, choice: carrier.Choice): shipping.Label
  throws …
{
  val sealed = shipping.sealed(id);                               // 荷物があり、送り状がまだない
  payment.settle(sealed.shipment.orderId, sealed.parcel.fee);       // 客: 商品の代金 + min(送料, 上限)
  val origin = shipping.shipFrom();
  val label = carrier.book(choice, sealed.parcel, origin, sealed.shipment.destination);   // 店の金が動く
  shipping.attachLabel(label);
  return label;
}
```

混ざった注文そのものは、型が持つ。`ordering.Order` は、ダウンロードの行（`digital`）と、箱で送る部分（`physical: PhysicalPart?`）に分かれ、届け先と送料の約束は、箱で送る部分の中にある（ダウンロードだけの注文には、届け先がない）。

### 誰が、いつ、何を決めるか

| 決めること | 誰が | いつ | 仕様の場所 |
|---|---|---|---|
| 行の種類 | 商品（`catalog.Kind`） | 注文のとき | `ordering.sort`（葉） |
| 送料の見積もりと上限 | サーバー（料金表と、行の大きさ） | 注文の前 | `shipping.foresee`（葉） |
| 上限への同意 | 客 | 注文のとき | `storefront.onPlaceOrder` |
| 出来上がりの大きさ | 製作の担当（量る） | 作り終えたとき | `backoffice.onFinishJob` → `fulfilment.made` |
| そろったか | 品目の事実（`shipping.Gathered` の述語） | 品目を置くたび | `shipping.gather` |
| 箱と重さ | 梱包の担当 | そろった後 | `backoffice.onPack` → `shipping.pack` |
| 客に請求する送料 | 料金表（箱の大きさの区分 × 地帯） | 梱包したとき | `shipping.weighed`（葉） |
| 請求する額 | サーバー（min(送料, 上限)） | 送り状を買う直前 | `payment.charged`（葉） |
| 配送業者 | 梱包の担当（見積もりを見て） | 梱包の後 | `backoffice.onBuyLabel` → `carrier.book` |

「そろったか」「箱と重さ」「客に請求する送料」「請求する額」「配送業者」は、この順でしか決められない: どれも、一つ前の事実が要る。「行の種類」から「出来上がりの大きさ」までの四つは、箱を知らずに決める。

### 分けて送ること

**許していない**（一つの注文に、出荷は一つ: `shipping.Shipment.orderId` は `@db.unique`）。在庫の品目を先に送れば、客は早く受け取れるが、送料の決め方と当たる。

- 二つの箱の送料の合計は、たいてい、一つの箱より高い。上限を、分けた場合の額で出すと、分けない注文の客にも高い上限を見せることになる。
- 枠からの請求の確定は一度だけ（`external/psp.hozo` の 3）。一つ目の箱を送るときに確定すると、二つ目の箱の送料は、まだ決まっていない。分けて送るなら、上限の額で先に確定して後で返す形（`payment.Standing.Prepaid` の形）を、いつも使うことになる。
- 誰の都合で分けるか（客が頼んだか、店が棚を空けたいか）で、増えた送料を誰が持つかが変わる。

### 考えた別の形

| 形 | 取らなかった理由 |
|---|---|
| 送料を、地帯だけで決める（箱によらない定額） | 「送料は荷物の大きさで決まる」という要件に反する。請求する額が注文のときに決まるので、「何が難しいか」の 3 がなくなる |
| 注文のときに、見積もりの額で請求を確定し、差額を、後から請求するか返す | 後からの請求には、支払いの手段を預かることと、二度目の同意が要る。請求が通らないと、梱包した箱が止まる |
| 注文のときに、上限の額で請求を確定し、差額を、いつも返す | 納期が枠の期限を超える注文でも成り立つが、ほとんどの注文で返金が起きる。枠の期限が近いときだけ、この形にした（「枠の期限と、納期」） |
| 送料が見積もりを超えたら、客の同意を取り直す | 梱包した箱が、同意を待つ状態になる。上限に、はじめに同意してもらう形にした |
| 出荷の進み具合を、一つの値（`stage`）の列で持つ | 値の集合を言う型がなく（F8）、「この段階では荷物がある」を型で言えない（F9）。事実ごとの表にすると、存在の分類が、型で決まる |
| 出荷の進み具合を、状態の分類の遷移（`effects C as A -> B`）で書く | 状態の分類が言うのは、状態の全体の場合で、表の一行の場合ではない。一行の場合は、値の分類と、効果の行の遷移で書いた（F1） |
| 梱包を、荷物を返す葉一つにして、中で確かめる | 葉は状態に触れないので、そろったことと、まだ梱包していないことを、配線が読んで渡すことになる。確かめる場所が、仕様から見えなくなる |

### 行の遷移で書いた進み具合

行の型の分類と、その分類のケースを進める効果。効果は、どれも、前の行と、足す事実を受け取る（`effect fn startJob(job: Job on Progress.Queued, at: datetime): effects jobs.put as Progress.InProduction;`）。前のケースは、行の引数の型（`T on C.K`）が言い、後のケースは、`as` が言う（`docs/check.md`「行の遷移」）。行の引数を絞り込んだ効果は、そのケースを受ける `match` のアームの中か、型が `T on C.K` の値でしか呼べない（TYPE-4、MATCH-4）。

| 行 | 分類 | 効果 | 前（行の引数の型） | 後（`as`） |
|---|---|---|---|---|
| 注文（`ordering.orders`） | `Standing` | `createOrder` | —（`post`: 行は、まだない） | `post as Standing.AwaitingPayment` |
|  |  | `acceptOrder` | `on Standing.AwaitingPayment` | `put as Standing.Accepted` |
|  |  | `expireOrder` | `on Standing.AwaitingPayment` | `put as Standing.Expired` |
| 支払い（`payment.payments`） | `Standing` | `createPayment` | —（`post`: 行は、まだない） | `post as Standing.Pending` |
|  |  | `replaceSessionOf` | `on Standing.Pending` | `put as Standing.Pending`（ケースを変えない。できるのは、まだ同意していない支払いだけ） |
|  |  | `holdSecured` | `on Standing.Pending` | `put as Standing.Held` |
|  |  | `chargeSecured` | `on Standing.Pending` | `put as Standing.Settled` |
|  |  | `recordPrepaid` | `on Standing.Held` | `put as Standing.Prepaid` |
|  |  | `notePrepayFailure` | `on Standing.Held` | `put as Standing.Held`（ケースを変えない。失敗を残せるのは、枠を確保した支払いだけ） |
|  |  | `recordSettled` | `on Standing.Held \| Standing.Prepaid` | `put as Standing.Settled` |
|  |  | `recordReleased` | `on Standing.Held` | `put as Standing.Released` |
| 製作の仕事（`production.jobs`） | `Progress` | `addJob` | —（`post`: 行は、まだない） | `post as Progress.Queued` |
|  |  | `startJob` | `on Progress.Queued` | `put as Progress.InProduction` |
|  |  | `finishJob` | `on Progress.InProduction` | `put as Progress.Finished` |
| 出荷（`shipping.shipments`） | `Custody` | `createShipment` | —（`post`: 行は、まだない） | `post as Custody.InHouse` |
|  |  | `recordHandOver` | `on Custody.InHouse` | `put as Custody.WithCarrier` |
|  |  | `recordDelivery` | `on Custody.WithCarrier` | `put as Custody.Delivered` |
|  | `Gathered` | `recordPlacement` | `on Gathered.Awaiting` | `put as else`（そろうか、まだ待つかは、残りの品目による） |
| 荷物（`shipping.parcels`） | `Packing`（`Parcel?`） | `createParcel` | —（`post`: 行は、まだない） | `post as Packing.Packed` |
| 送り状（`shipping.labels`） | `Labelling`（`Label?`） | `createLabel` | —（`post`: 行は、まだない） | `post as Labelling.Labelled` |
| 取り置き（`inventory.reservations`） | `Picking` | `markPicked` | `on Picking.OnShelf` | `put as Picking.Picked` |
| セッション（`staff.sessions`） | `Login`（`Session?`） | `createSession` | —（`post`: 行は、まだない） | `post as Login.LoggedIn` |
|  |  | `deleteSession` | `on Login.LoggedIn` | `delete as Login.LoggedOut` |
| 配送業者（`carrier.carriers`） | `Use` | `switchCarrier` | 絞り込まない（どのケースの行でも） | `put as else`（後のケースが、引数の値で決まる。F53） |

`as` を書かない `put`（`inventory.confirmReservations`、`inventory.saveLevels`）は、行のケースを変えない: 取り置きを「棚から出してよい」にしても、`Picking` は `OnShelf` のまま。

**消すものと、消さないもの**（`docs/requirements.md`「書き方の指針」）。期限切れの注文は、消さずに、ケース `Expired` を持つ（客が、注文のいまを見る。後から届いた同意の枠を、手放す）。荷物と送り状は、作るだけで、消さない。消すのは、なくなることが意味であるもの: ログアウトしたセッション（`deleteSession`）と、戻した取り置き（`dropReservations`）。行がないことを、遷移の端になる、名前のある状態として扱うのは、三つ（`Packing.Unpacked`、`Labelling.Unlabelled`、`Login.LoggedOut`）で、どれも `as none` で書いた。「引いたものがあるか」の分類の、ない側（`Missing` など）は、`as else` のまま: 行がないことを、遷移の端として使っていない。

製作の仕事の、前と後（`domain/production.hozo`。説明のコメントを省いた抜粋）:

```ts
// 前: 値の分類と、行の全体を書き換える効果と、時刻を入れる葉
@db.update
effect fn saveJob(job: Job): effects jobs.put;

export fn start(id: JobId): Job throws NoJob, NotQueued {
  val job = require(id);
  val now = clock.now();
  val started = match (job by Progress) {
    Queued                  => startedAt(job, now);
    InProduction, Finished => throw NotQueued;
  };
  saveJob(started);
  return started;
}

fn startedAt(job: Job on Progress.Queued, now: datetime): Job;   // 人が書く
```

```ts
// 後: 効果が、どのケースからどのケースへ進めるかを言う
effect fn startJob(job: Job on Progress.Queued, at: datetime): effects jobs.put as Progress.InProduction;
effect fn finishJob(job: Job on Progress.InProduction, measured: type.Measurement, at: datetime): effects jobs.put as Progress.Finished;

export fn start(id: JobId): Job throws NoJob, NotQueued {
  val job = require(id);
  val now = clock.now();
  match (job by Progress) {
    Queued                 => startJob(job, now);      // Queued のアームの中でしか、呼べない
    InProduction, Finished => throw NotQueued;
  };
  return require(id);
}
```

### 言語が言えなかったこと

この問題の中心で、仕様に書けなかったもの。それぞれの持ち主は「分かったこと」。

| 言えなかったこと | 代わりにしたこと | 項目 |
|---|---|---|
| 請求は上限を超えない、請求する送料 = min(送料, 上限) | 葉のコメントと、印 `@pact.promise` | F11 |
| 行のケースを、宣言した向きのほかには変えないこと | 効果の中身は、人が書く。宣言と、呼ぶ側の検査（引数の型: TYPE-4、MATCH-4）まで | F1、F51 |
| いくつかの行のケースを、一度に変える | 時計で、一行ずつ | F50 |
| 後のケースが、引数の値で決まる遷移 | 何も約束しない `as else`（行の引数は、絞り込まない） | F53 |
| 一つの行の、二つの分類を一度に言う | 片方だけを書く | F52 |
| 荷物は、そろった出荷からしか作れない | 荷物を返す葉 `weighed` の引数の型。効果 `createParcel` は、どの荷物でも受け取る | F10 |
| 混ざった注文の行を、種類ごとに分ける | 葉 `ordering.sort`。分けた結果の形（`Sorted`）だけが仕様 | F2 |
| 上限が変わっていたときの、新しい上限を、画面に出す | 例外は値を運ぶ（`TermsChanged(terms)`）が、画面は、見せている値段を捨てるだけ | F4 |
| 金が動いたかどうか分からない失敗 | `http.NetworkError` / `http.ServerError` を、言い換えずに、画面まで伝える | F5 |
| 納期は、枠の期限より短い | コメント | F26 |

## 分かったこと

書けなかったもの、書きにくかったもの、規則がないまま決めたもの。それぞれ「何が起きたか → サンプルではどうしたか」。確かめた診断は、このサンプルの外の小さな仕様に `hozoc build --no-emit` を掛けたもの。

### 言語

- **F1 表の一行の進み具合を、遷移で書けなかった。→ 行の遷移で、書けるようになった。** はじめは、状態の遷移（`effect fn …: effects C as A -> B`）の宛先が、状態を対象にした分類だけだった。注文、支払い、仕事、出荷は、どれも表の一行の話で、値の分類（`ordering.Standing`、`payment.Standing`、`production.Progress`、`shipping.Custody`）と、行の全体を書き換える効果（`saveOrder` など）と、時刻を入れる葉で近づけていた。どの op が、どのケースからどのケースへ進めるかは、`match` のアームを読まないと分からなかった。→ いまは、効果が、宛先の後ろの `as` で言う（`docs/check.md`「行の遷移」）。24 の効果が行の遷移を持ち、行の全体を書き換えていた七つの効果（`saveOrder`、`saveOrders`、`savePayment`、`saveJob`、`saveShipment`、`saveReservation`、`saveCarrier`）は、なくなった。一覧は「行の遷移で書いた進み具合」。分かったことは F50〜F57。
- **F2 配線は、一覧の要素ごとに、何もできない。** → 三つの形に逃げた。一覧を受け取る効果と op（`inventory.addReservations`、`production.addJobs`、`download.addGrants`）。一覧を作る葉（`ordering.sort`、`production.plan`）。外部の仕組みを一つずつ呼ぶものと、行のケースを変えるもの（F50）は、時計で一件ずつ（`payment.prepayExpiring`、`ordering.expireUnpaid`）。**混ざった注文を種類に分けること自体が、葉になった**: `catalog.Kind` は型で決まる分類だが、一覧の要素には当てられない。 → **R6 で、配線に、繰り返し（`for`）が入った**（下の「R6 で変えたこと」）: `production.enqueue` と `ordering.expireUnpaid` は、繰り返しで書き直した。外部の仕組みを一つずつ呼ぶ `payment.prepayExpiring` も、失敗を、状態に残す形にして、繰り返しにした。
- **F3 判断一つにつき、op が一つ要る。** アームの本体は呼び出し一つで、呼び出しの引数に呼び出しを書けない。→ `shipping.pack` は `packGathered`、`packInto`、`seal` の四段。`payment` の 31 の op のうち 22 が配線。PRD に名前のない op（`packInto`、`secureFound`、`offersIfPacked`）に、名前を付けることになる。
- **F4 例外を、配線で受けられない。受けられるのはデコレータだけで、例外の値は使えない。** → 値を運ぶ例外（`ordering.TermsChanged(terms)`、`inventory.OutOfStock(sku)`、`ordering.UnknownProduct(sku)`）の値は、画面に届かない: `storefront.sayTermsChanged` は見せている値段を捨てるだけで、`storefront.Outdated` は、どの商品かを持たない。画面を分けたい結果は、例外ではなく値で返す形に寄った（`shopApi.product` と `shopApi.status` は `T?`）。
- **F5 例外を、別の名前に言い換える書き方がない。** `catches A recovers throws B` と書くと、A を「吸収した」ことになり、効果を持つ op では DEC-5（成功のアームでも効果が不明）。金が動く三つの呼び出しに付けたら、warning が六つ出た。→ 言い換えたのは、金が動かない呼び出しだけ（`payment.openHold`、`payment.openCharge`、`download.fetchLink` の `@outside.failsAs`）。請求の確定、返金、枠を手放す、送り状を買う、の失敗は、`http.NetworkError` / `http.ServerError` のまま `staffApi.buyLabel` まで伝わる。ブラウザから見ると、自分の通信の失敗と、サーバーの先の失敗が、同じ名前になる。
- **F6 外部の仕組みの中で起きること（金が動く）は、効果ではない。** → 相手の状態を、相手の名前空間に置いた（`psp.ledger`、`psp.holds`、`carriergw.labels`）。相手の op に `@outside.does(…)` で効果を運ばせる。得たもの: `fulfilment.dispatch` の効果に `psp.takeMoney` と `carriergw.issueLabel` が並び、`payment.capture` の `throws psp.HoldExpired prevents effects` を、検査器が本体から確かめる（REL-4）。払ったもの: 相手の内側の状態を、こちらが作った。
- **F7 分類の述語は、対象の値しか見られない。** 時刻で変わる場合分け（期限が過ぎたか）は、分類にできない。→ 「受け取った値を、そのまま返すか、投げる葉」（`staff.requireFresh`、`download.stillValid`）。`ordering.Standing.Expired` は、時計で動く `ordering.expireUnpaid` が `expiredAt` を入れた後でだけ、当たる。
- **F8 値の集合を言う型（列挙）がない。** リテラルを scalar の引数に渡せない（TYPE-1）ので、`authorize(token, "packer")` は書けない。→ 仕事は、三つの真偽値と、仕事ごとの分類三つ（`staff.MakerDuty`、`PackerDuty`、`AdminDuty`）と、三つの op（`staff.asMaker`、`asPacker`、`asAdmin`）。商品の種類は、union と、型で決まる分類で書けた（`catalog.Fulfilment`、`catalog.Kind`）。`psp.Event.kind`、`shipping.SizeClass`、`shipping.Zone`、`Problem.reason` は文字列で、値の集合はコメント。
- **F9 「この段階では、この値がある」を、一つの model では言えない。** `model Packed extends Shipment { override parcel: Parcel; }` は TYPE-15。model を別の model に絞るケースは CLS-6。→ 事実を別の表にして（`parcels`、`labels`）、存在の分類（`Packing`、`Labelling`）で絞り、そろった組を model にした（`Sealed`、`Dispatched`）。
- **F10 `T on C.Case` の引数は、別の名前空間からも効いた。作り方は縛れない。** `shipping.boxOptions(shipment: Shipment on Gathered.Complete)` を `fulfilment.optionsIfGathered` が呼ぶ。そろっていない出荷を渡すと MATCH-4（`Awaiting` のアームから `boxOptions(shipment)` を呼んで確かめた）。ただし、効果 `createParcel(parcel: Parcel)` は、どの `Parcel` でも受け取る。「荷物は、そろった出荷からしか作れない」は、`Parcel` を返す葉が `weighed` だけであることに依り、検査されない。
- **F11 不変条件を書けない。** 「請求は上限を超えない」「請求する送料 = min(送料, 上限)」「`limit` = `itemsTotal` + `shippingCeiling`」「見積もり ≤ 上限」は、葉（`payment.charged`、`shipping.foresee`）のコメント。→ 印 `@pact.promise` を、約束を守る op に付けた。約束と op が結ばれるだけで、中身は確かめない。この仕様でいちばん大事な客への約束が、検査の外にある。
- **F12 分類の対象に `read` を書けない**（RES-4）。op の分類にするには、`read` をそのまま呼ぶ op が要る（`shop-server/README.md` の N119）。→ 型の分類にした（`classify Known of Order?`）。「引いたものがあるか」の分類が、名前空間ごとに並ぶ（`Known` が四つ、`Held`、`Stocked`、`Registered`、`Account`、`Entitlement`、`Login`）。どれも同じ意味（同 N4）。「ない」側の名前も、分類ごとに付けた（F44）。
- **F13 空でない一覧の型がない。** `carrier.Fleet`（使っている配送業者があるか）は、述語で決まる分類になり、「空でない」（`Available`）と「空」（`Unavailable`）を、人が二つとも書く（F45）。`by list` なら書かずに済むが、場合に業務の名前が付かない。
- **F14 設定の値を言う宣言がない。** → `single` の状態にした（`ordering.paymentWindow`、`payment.holdMargin`、`download.grantLifetime`、`shopApi.site`、`shipping.origin`）。誰が書き込むのかは、仕様にない。時計の間隔だけは、デコレータの引数に数字で書いた（`@clock.tick(every: 5m)`）。PRD の数字を、どちらに書くかの基準がない。
- **F15 識別子と鍵を作る葉は、仕様の上では、乱数を使っていない。** 時刻は `clock.now()` で配線に出したが、識別子（`ordering.draft`、`storefront.newRequestId`）は出していない（`shop-server/README.md` の N9 と同じ）。
- **F16 予約語が、業務の言葉と当たる。** `signed(by: …)` は SYNTAX（`by`）。→ `sender`。相手の「取り消す」は `void` と書けない → `psp.release`。
- **F17 「あれば置き換え、なければ入れる」効果がない。** `nullable` の `put` は、形の表では「あるものを置き換える」。→ `storefront.rememberOrder` と、理由を出す効果（`sayTryLater` など）は、空のときにも呼ぶ `put` として宣言した。検査器は、配線の行をまたいで状態の場合を追わないので、何も言わない。
- **F18 配線が、二つの値を一度に運べない。** → 運ぶためだけの model（`shipping.Sealed`、`shipping.Dispatched`、`shipping.Progress`、`inventory.Allocation`）。
- **F19 トランザクションの範囲を、仕様は言えない。** `hooks.paymentEvent` の `@db.transaction` は、六つの名前空間の表と、外部の呼び出し（`payment.release`）と、メールを含む。どの状態が同じ DB にあるかを言う宣言はない。入れ子（`fulfilment.acceptAndBegin`、`ordering.place`）の意味は、語彙を読むトランスパイラが決める（`docs/vocab.md`「`db` の read と効果」）。
- **F20 誰にも呼ばれない op の例外には、受け手がない。** `payment.prepayExpiring` は `throws psp.HoldExpired, http.NetworkError, http.ServerError`。検査器は何も言わない。
- **F21 「外から呼ばれる入口」と `export` は、別のもの。** 時計で動く op（`ordering.expireUnpaid`）と、画面の操作の op は、`export` を持たない内部の op。入口であることは、デコレータ（`@clock.tick`、`@browser.page.interaction`）にしかない。操作の op のうち、ページのどのアームでもできるもの（`backoffice.onLogout`、`onRegisterCarrier`）は、印もない。

### 語彙

- **F22 プラグインの語彙は、設計だけの段階では使えない。** 読まれるのは、単位の `transpilers` に書いたトランスパイラのプラグインの語彙だけ（`docs/vocab.md`「読み込み」）。単位のないプロジェクトで、`plugin_dirs` のプラグインの語彙を使うと RES-1。→ 語彙を、プロジェクトの仕様（`vocab/*.hozo`）として書いた。マニフェスト（継ぎ目の宣言など）は書いていない: 読む者がいないものは、確かめられない。
- **F23 `http` の語彙に足りなかったもの。** パスの中の引数（`/orders/:key`）→ クエリにした。POST と GET のほかのメソッド → POST にした。外へ呼ぶときの、相手の場所、鍵、やり直しても二度起きないための識別子のヘッダ、待つ長さ → 書いていない。`@http.retry` は、金が動く呼び出しに、その識別子なしでは付けられないので、使っていない。
- **F24 外へ呼ぶ HTTP は、相手の名前空間の op に `@http.post` を付けて書けた。こちらの入口と、相手の口が、同じ書き方になる。** `@http.post` の `outer throws`（前は、核の印 `@external`）が、呼ぶ側に `http.NetworkError` と `http.ServerError` を足す（`external/psp.hozo`、`external/carriergw.hozo`）。違いは、`hozo.toml` に単位があるかだけ。このサンプルには単位がないので、どれがこちらのサーバーかは、ディレクトリの名前と、この文書にしかない。
- **F25 二度届く通知は、配線で書けた。届く順は、書けない。** `payment.seen` と分類 `Redelivery`、`payment.Securing`。「もう扱った」と記録する行と、扱う行が、一つのトランザクションに入ることは、F19 のとおり、仕様の外。
- **F26 時刻。** `clock.now()` と `@clock.tick` を作った。「期限の N 日前」は、時刻を作る葉（`ordering.before`、`payment.ahead`）と、比較を人が書く `read`（`oldestUnpaidBefore`、`nextExpiringHold`）。`duration` の上限は、`valid` で言えない（`@valid.max(45d)` は TYPE-4）。
- **F27 `db` の語彙で言えなかった読み書き。** サーバーの 34 の `read` のうち 7、30 の効果のうち 21 に、デコレータを付けられなかった: 集合に入っているか（`productsBySkus`、`levelsOf`）、比較（`oldestUnpaidBefore`）、値がないこと（`toPick`、`shipmentsInHouse`、`openJobs`）、まとめて書く（`saveLevels`、`addJobs`）、キーでない列で書き換える（`confirmReservations`）。効果の 21 のうち 16 は、行のケースを変える効果で、`@db.update` と `@db.delete` の形に合わない（F51、F54）。列のしまい方（`@db.text` など）は、一つも書いていない（G1）。
- **F28 「後で、必ず」がない。** 通知の入口は、受け付けて、分けて、メールを送るまでを、その場で行う（`hooks.paymentEvent`）。失敗したら、相手がもう一度送ることに頼る。
- **F29 金額。** `type.Yen` は整数の scalar。足し算も比較も配線にないので、合計と差額は、どれも葉（`payment.limitOf`、`payment.charged`、`payment.refundOf`）。
- **F30 `browser.page` の語彙。**
  - `@browser.page.fallback` は、一つの op に一つ（DEC-13）。→ 二つ目は、内側の op に付けた（`storefront.filledCart` の `@outdatedOnFailure`）。
  - `@browser.page.interaction` も一つの op に一つで、`when` はアーム一つ。→ **ページの union のアームを、できる操作で切ることになった**（`storefront.OrderPage.Accepted` は、進み具合を、アームではなく中の値に畳んだ）。
  - 状態が変わったとき、ページを描き直すかを言えない。→ 効果 `reload()`（`location.put`）を、操作の最後に呼ぶ。
  - 待つ、定期的に見直す、がない。→ 同意の通知を待つ間は、客が `onRefresh` を押す。
  - 操作の op が投げた例外が、画面でどうなるかを言う語彙がない。→ `recover(error, then)` を、画面の名前空間ごとに作った。
  - 操作の結果を画面に出すには、それを受け取る葉が要る（`backoffice.seen`、`placedIn`、`sealed`）: 値を返す呼び出しは束縛しなければならず（WIRE-2）、使わない束縛は WIRE-8。
- **F31 認証を、入口ごとに一行書いた**（`staffApi` の 13 の op の一行目が、`staff.asMaker`、`asPacker`、`asAdmin` のどれか）。デコレータにすれば一語で書ける（`decorator fn packerOnly(): throws type.Unauthorized, type.Forbidden prevents effects;`）。そうしなかったのは、確かめるために `staff` のセッションを読むことが、仕様から消えるから: 別の名前空間の状態を読むことは、デコレータの `reads` に書けない（READS-1。`docs/check.md`「デコレータが運ぶ reads と effects」）。配線なら、`staff.sessions` が、入口の op の読む状態に現れる。
- **F32 一つの述語を、二つの担い手が要る。** `shipping.Gathered` をブラウザでも使うと、述語の実装が、二つの言語に要る。分類の結果を、値として運ぶ型はない。→ サーバーが、結果を、値の形で運ぶ（`fulfilment.Station.options` があるか）。

### 検査器

- **F33 検査器が error にしたのは、言語の限りを確かめた実験だけ。** F1、F8、F9、F12、F16、F22、F26、F30 の書き方は、このサンプルの外で確かめた。サンプルの約 4,300 行は、まだ書いていない名前空間を引いたとき（RES-1）のほかは、書いたまま通った。順番の誤り（請求の確定より先に、送り状を買う）は、通る。
- **F34 書く量をいちばん増やしたのは THROWS-1。** `staffApi.buyLabel` の `throws` は 13、`storefront.placeOrder` は 12。同じ並びを、`fulfilment.dispatch`、`staffApi.buyLabel`、`backoffice.onBuyLabel` の `@recover` で、三度書く。例外のユニオンでまとめることは、していない（G8）。 → 配線の op は、`throws` を書かなくてよくなり（`docs/decisions.md`「決めたこと」379）、`staffApi.buyLabel`、`storefront.placeOrder`、`fulfilment.dispatch` の節は、消した（「配線の op の `throws` を消したこと」）。同じ並びを書くのは、`backoffice.onBuyLabel` の `@recover` の一度だけ。
- **F35 DEC-16 は、画面で役に立った。** `@recover` の相手を、その op が投げない例外にすると warning。
- **F36 op が値を返すかどうかが、呼ぶ側の都合で決まった。** 確かめるためだけに呼ぶ op は、値を返せない（WIRE-2 と WIRE-8）。→ `staff.asPacker`、`inventory.pick`、`payment.settle` を、値を返さない op に直した。
- **F37 `hozoc build --no-emit` は、親のディレクトリの `hozo.toml` を拾う。** サンプルに `hozo.toml` を置く前は、リポジトリ直下の設定で検査され、「0 errors」と出た（`docs/config.md`「置き場と探し方」のとおりの振る舞い）。

### 指針が引っ張り合ったところ

指針は `docs/requirements.md`「書き方の指針」。

- **F38 「DB の状態は、使う名前空間とは別の名前空間に」には、従わなかった。** 概念の名前空間が、状態と規則の両方を持つ。`store` に切り出すと、八つの概念の表が一つの名前空間に集まり、`read` と効果を呼ぶだけの op が、その数だけ要る（`shop-server/README.md` の N119）。引き換えに、概念の中の配線は、同じ概念の状態に、どこからでも触れられる。
- **F39 「共有の型は、中立の名前空間に」は、値の型にだけ当てた。** `type` に置いたのは、識別子、金額、住所、寸法。行の model は、状態の持ち主に置いた。すると、`ordering.MadeLine` を、`production`、`shipping`、`inventory` が引く。画面まで入れると、ほとんどの model が「共有」になるので、「共有なら `type`」では、`type` が全部を持つ。
- **F40 「分類には、業務の名前を」と、同じ意味の分類が増えること**（F12）。
- **F41 「外部の仕組みの op には、その仕組みの語彙のデコレータ」。** `@http.post` を付けると、相手の口の宣言になり、通信の形（パス、本文）まで仕様に入る（`psp`、`carriergw`、`assets`）。印だけにすると（`mailer.send` の `@smtp.sendMail`）、入らない。どちらにするかの基準は、指針にない。
- **F42 入口の名前空間を、呼ぶ相手ごとに分けた（`shopApi`、`staffApi`、`hooks`）。指針にない。** `staffApi` の 15 の op のうち 13 は、担当を確かめて、一つ呼ぶだけ（残りは、ログインとログアウト）。概念の op に `@http.*` を直に付ければ消えるが、要求のどこから読むか（`@http.header` のトークン）と認証が、概念に入る。
- **F43 状態を持たない、流れの名前空間 `fulfilment` を作った。指針にない。** 概念どうしが直に呼び合う（`production` が `shipping` を呼ぶ）代わりに、順番を一か所に置いた。`ordering` は `inventory` と `shipping` を直に呼んでいて、そろっていない。
- **F44 分類を閉じると（`docs/check.md`「閉じた分類」）、名前のないケースは、なくなった。** はじめに 24 あった `default` のうち、22 は意味のある状態で、名前を付けた: 19 は型で決まる分類の残りのケース（`Missing: Order? as else;`。同「残りのケース」）、3 は述語のケース（`shipping.Gathered` の `Awaiting`、`carrier.Fleet` の `Unavailable`、`catalog.Offer` の `Absent`）。残りの 2 は、はじめ、この仕様が使わない場合だと考えて、ケースごと消した: `psp.Meaning`（客の同意のほかの通知）と、`fulfilment.Arrival`（「届いた」のほかの、荷物の動きの通知）。振る舞いが変わったのは、この 2 だけで、その後、どちらにも、名前のあるケースを戻した（F47）。「ない」側の名前は、分類ごとにばらついた（`Missing` が四つ、`Unknown` が二つ、`Absent`、`Unlisted`、`Unreserved`、`Uncounted`、`Ungranted`、`LoggedOut`）: 同じ意味の分類が並ぶこと（F12、F40）が、名前の数だけ目立つ。例外と同じことを二度言うアームもできた（`Ungranted => throw NotGranted;`、`Unknown => throw UnknownCarrier;`）。
- **F45 述語で決まる分類は、ケースの数だけ、人が述語を書く。** `classify Gathered of Shipment { Complete; Awaiting; }` は、`Awaiting` の述語も人が書き、二つが排他で、どの出荷もどちらかに当たることは、書く人が担保する（`docs/plugin-go.md`「分類」: 真がちょうど一つでなければ、分類の誤り）。`Awaiting` が `Complete` の否定であることは、仕様のどこにも書けない。`carrier.Fleet` の `Unavailable` も同じ。三つの `condition`（`staff.Maker`、`Packer`、`Admin`）を、両側に名前のある分類に直したので、否定の側の述語が、さらに三つ増えた（`NotMaker`、`NotPacker`、`NotAdmin`）。いまは、36 の分類のうち、述語で決まるものは 15 で、人が書く述語は 36（行の遷移のために足した `payment.Billing` の二つと、`psp.Meaning.Echo` で三つ増え、`catalog.Offer.Absent` で一つ減った: F46、F47、F56）。
- **F46 型で決まる側が、述語のケースになっていた。→ ない側のケースで、型で決まるようになった。** `catalog.Offer`（`OnSale` と `Withdrawn` が、どちらも `Product? as Product`）の「ない」は、値がないことで、型から決まる。はじめは、残りのケースを述語のケースの隣に書けず（CLS-7）、`as` のないケース `Absent;` にして、「値がない」という述語を人が書いていた。→ いまは、`Absent: Product? as none;` と書ける（`docs/check.md`「ない側のケース」）: 値がなければ `Absent`（述語を呼ばない）、あれば、`OnSale` か `Withdrawn` かを述語が決める。
- **F47 使わない通知を宣言しないと、通知の入口が、その通知で失敗した。→ 受け取るだけの通知に、名前のあるケースを戻した。** 分類を閉じたとき、`psp.Meaning` のケースを `Secured` だけ、`fulfilment.Arrival` のケースを `Arrived` だけにした。すると、`payment.recordNew` と `fulfilment.tracked` は、ほかの通知で、分類の誤りになって失敗した。
  - `hooks.paymentEvent` は `@db.transaction` なので、失敗すれば、`payment.remember` も取り消される。通知を「扱った」と記録しないので、相手は同じ通知を送り直し、そのたびに同じ失敗になる。
  - `hooks.trackingEvent` は、運んでいる途中、不在、返送の通知のたびに失敗する。配送業者は、荷物一つにつき、そういう通知をいくつも送る。
  - 分類の誤りは、仕様が扱わない、ありえないことが起きた誤りで、`throws` に書かず、受けることもできない。**当たらない値に対して何かをしたい（ここでは、受け取ったとだけ答える）なら、それは分類できるということなので、ケースとして宣言して、枝に書く**（`docs/requirements.md`「書き方の指針」）。二つの通知は、どちらも、これに当たった:
    - `psp.Meaning`: `Echo;` を足した（こちらが頼んだことの結果の知らせ: 請求の確定、返金、枠の解放）。述語で決まる（`kind` は文字列）。`payment.recordNew` は、`Echo => none;` で、通知を「扱った」と記録して、何もしない。
    - `fulfilment.Arrival`: `Underway: datetime? as else;` を足した（届いた時刻のない通知: 運んでいる途中、不在、返送）。型で決まる。`fulfilment.tracked` は、`Underway => none;`。
  - 宣言しないままにしたものは、ない: この二つの入口のほかに、「来るはずのない値」を受けている分類は、このサンプルにない（ほかの分類の、ケースの外の値は、述語の実装の誤りで、仕様が何かをする場合ではない）。Q11 は、これで閉じた。
- **F48 仕事の分類は、一つの `Role` にしなかった。** 担当者は、三つの仕事を、いくつでも持てる（`Member.maker`、`packer`、`admin` は、別々の真偽値）。`classify Role of Member { Maker; Packer; Admin; }` は、製作と梱包の両方をする担当者で、二つの述語が真になり、分類の誤りになる（ケースは排他）。仕事ごとに、「してよい / してはいけない」の分類を一つずつ持った。「してはいけない」側の名前（`NotMaker`）は、業務の言葉になかった。
- **F49 `match` の `else` のアームが合ったのは、`payment.Standing` だけ。** 五つのケースから一つを取り出して、残りの四つを同じに扱う `match` が四つあり（`reopen`、`secureFound`、`prepayHeld`、`release`）、`else` にした（`docs/requirements.md`「書き方の指針」の「1 : 4」）。`payment.settle` は「1 : 1 : 1 : 2」なので、どのアームも名前で並べた。三つのケースの「1 : 2」（`production.start` と `finish`、`shipping.release` と `markDelivered`、`storefront.onAddToCart`）も、名前で並べた: 並べる名前が二つなら、`else` にしても短くならず、ケースを足したときに MATCH-2 が尋ねる方がよい。
- **F50 行のケースを変える効果は、行を一つずつ受け取る。** 行の遷移を書く効果は、行の引数（型が、状態の要素の型の引数）を、ちょうど一つ持つ（STATE-7）。行の並びを受け取る効果（`saveOrders(changed: Order[])`）には `as` を書けず、`as` を書かない `put` は「行のケースを変えない」という意味になる。→ 期限切れは、時計で一件ずつにした（`ordering.expireUnpaid` は、いちばん古い一つを `expireOrder` に渡す。間隔を 5 分から 1 分にした）。`inventory.release` は、注文一つの取り置きを戻す形になり、葉 `idsOf` と、集合で引く `read` が要らなくなった。まとめて足す `post`（`production.addJobs`、`inventory.addReservations`、`download.addGrants`）は、入るケースを書けないまま（どれも、はじめのケースで作ることは、葉のコメント）。 → **R6 で、効果を、繰り返しの中で、一つずつ呼べるようになった**: `ordering.expireUnpaid` は、期限切れの注文を、一回の動きで、すべて扱う。
- **F51 `@db.update` は、行のケースを変える効果の、どれにも合わなかった。** 呼ぶ側の検査（行の引数の型。TYPE-4、MATCH-4）が求めるのは、前のケースにあると分かっている行、つまり、書き換える前の行。`@db.update` は、渡した行の全体を書く（`docs/plugin-go-sqlite.md`「効果の中身」）ので、前の行を渡せば、何も変わらない。→ ケースを変える効果は、前の行と、足す事実を受け取る形にして（`startJob(job, at)`、`recordSettled(payment, charge, at)`）、中身は、人が書く。`db` のデコレータを付けた効果は、15 から 9 に減った。引き換えに、変えた行を作るだけの葉が 11 なくなった（H2。人が書く葉は、90 から 78 に減った）: 「行をどう変えるか」を書く場所が、葉から、効果の中身に移った。足す事実が込み入っているところは、事実を数える葉と、それを運ぶ model が要る（`payment.Charge`、`shipping.Placement`。F18 が、二つ増えた）。「この列に、この値を入れる」と言う `db` の語彙があれば、中身も宣言から出せる。
- **F52 一つの `as` が言うのは、一つの分類。** 出荷は、二つの分類を持つ（`Custody` と `Gathered`）。`createShipment` は `as Custody.InHouse` と書き、「まだ待っている（`Gathered.Awaiting`）で入る」は、書けなかった。`recordHandOver`（`Custody` を進める）が `Gathered` を変えないことは、「ほかの分類については、`as` を書かない効果と同じ」という決まりから来る。
- **F53 後のケースが、引数の値で決まる遷移は、約束できない。** `carrier.setEnabled(code, enabled)` は、`enabled` の値で、`InUse` にも `Suspended` にもする。配線は、真偽値で分けられない（F8）。→ `switchCarrier(carrier, enabled)` を、何も約束しない形（行の引数を絞り込まず、後は `as else`）にした。このサンプルで、抜け道を使ったのは、ここだけ。op を二つ（使う、止める）に分ければ、二つの遷移として書けるが、画面の操作（スイッチ一つ）と形が離れる。
- **F54 行を受け取る `@db.delete` がない。** `delete` に行の遷移を書くと、効果は行を受け取る。`@db.delete` の引数は、列の条件（`docs/plugin-go-sqlite.md`「効果の中身」）。→ `staff.deleteSession(session: Session)` は、デコレータなしで、人が書く。ログアウトは、消す前に、セッションを読むことになった（`endSession`: `LoggedIn => deleteSession(session); LoggedOut => none;`）。「あれば消す」が、仕様に見えるようになったとも言える。
- **F55 変えた後の行を返す op は、読み直す。** 効果は値を返さず、渡した行は、前の行。→ `production.start`、`shipping.gather`、`carrier.setEnabled` は、効果の後で、もう一度引いて返す（`return require(id);`）。`ordering.accept` は、読み直しのために `throws NoOrder` が増え、`fulfilment.acceptAndBegin` に伝わった（いま書き換えた行なので、起きない例外）。
- **F56 一つの op が、場合によって二つのケースへ進めていたところは、分類が一つ増えた。** 客の同意を記録する `payment.markSecured` は、箱で送るものがあれば `Held`、なければ `Settled` にしていた（葉 `securedBy` の中）。後のケースは一つなので、効果を二つにし（`holdSecured`、`chargeSecured`）、どちらを呼ぶかを分ける分類 `Billing { Deferred; Immediate; }` を足した。葉の中にあった分かれ道が、仕様に出た。`as else` で一つにまとめることもできたが、「決まっていない」のではなく、「二つのどちらかに、決まる」ので、そうしなかった（`recordPlacement` の `as else` は、残りの品目で決まり、宣言からは決まらないので、`else`）。
- **F57 `post` の「前」は、書かない。** `createParcel` は、`post as Packing.Packed`。`post` の前は、いつも「行がない」で、操作が言っている（`docs/check.md`「行の遷移」の「操作ごとに書ける形」）。渡す行は、入る行で、「まだない」ことを示す値ではないので、「梱包は一度だけ」は、効果の宣言からは、守られない。一度だけであることは、今までどおり、`shipping.pack` の `match (existing by Packing) { Packed => throw AlreadyPacked; … }` が担う。
- **F58 呼ぶ側の検査（行の引数の型: TYPE-4、MATCH-4）は、行の遷移への書き直しの間、error を一つも出さなかった。** 行を進める op は、もともと、`match` のアームの中で、絞り込んだ型（`Job on Progress.Queued`）の葉を呼ぶ形だった（F10）。その葉を効果に置き換えただけで、検査を通った。確かめたのは、逆の向き: アームの外に効果の呼び出しを出すと、絞り込んでいない行を渡すことになり、TYPE-4（このサンプルの外の小さな仕様で確かめた）。検査が守るのは「どの行に対して呼べるか」までで、効果の中身が、宣言したとおりに行を変えることは、まだ誰も確かめない（`docs/requirements.md`「段階」の、行の遷移の保証）。
- **F59 一つの値は、二つの分類の絞り込みを、同時には持てない。** `payment.markSecured(payment: Payment on Standing.Pending, …)` は、請求のしかた（`Billing`）で分けて、`Standing.Pending` の行を受け取る効果（`holdSecured`、`chargeSecured`）を呼ぶ。`match (payment by Billing)` のアームの中で、`payment` は `Payment on Billing.Deferred` になり、`Standing.Pending` の絞り込みを持たなくなる（`docs/types.md`「アームの中の対象の型」）ので、そのままでは、効果に渡せない（MATCH-4）。→ 分ける対象を、別の名前に受けた（`val billed = payment; match (billed by Billing) { Deferred => holdSecured(payment, …); … }`）。絞り込みが掛かるのは、対象と同じ名前だけなので、`payment` は、アームの中でも、もとの型のまま。回り道で、読む人には、理由が要る（`docs/open.md`「言語」）。

## 粒度: 仕様は、どこまで書くか

### 言語が決めている下限

**状態に触れる順は、配線に書くしかない。** 葉は状態に触れない（STATE-5）ので、読むことと書くことは、どれも配線の行になる。配線が呼べるのは、同じ名前空間の `read` と効果と、`export` された op だけなので、概念の境界を越えるところも、配線の行になる。「最低限」は、ここより上には上げられない。この下限の持ち主は `docs/check.md`「状態に触れるもの」。

### 名前空間ごとに、どこで止めたか

| 名前空間 | 止めたところ | 理由 |
|---|---|---|
| `type` | 値の型と、その制約（`@valid`） | 二つ以上の概念と画面が使う |
| `catalog` | 種類ごとに持つ事実と、引き方 | 商品の登録は PRD の外に置いた |
| `ordering` | 種類に分けた結果の形、届け先が要る条件、同じ申し込みは一つ、取り置きと注文の順 | 分け方と、値段の付け方は、葉（`sort`、`toShip`、`draft`） |
| `inventory` | 取り置く、戻す、出す、の順と、足りないときの例外 | 数の計算は、葉（`allocate`、`restore`、`minus`） |
| `payment` | 支払いの五つの場合、場合ごとにしてよいこと、相手を呼ぶ順 | 額の計算は、葉（`charged`、`limitOf`）。この名前空間が、いちばん深い: 金が動く順を、葉に入れたくなかった |
| `production` | 仕事の三つの場合と、進める二つの op | 納期の決め方は、葉（`plan`） |
| `shipping` | 事実の足される順と、それぞれの前提。箱の選択肢と送料を、選ぶ前に見せること | 詰め方、地帯の決め方、送料の引き方は、葉（`foresee`、`fitting`、`weighed`） |
| `carrier` | 登録と切り替え、使っている配送業者にだけ頼むこと | 依頼の組み立ては、葉 |
| `staff` | 三つの仕事と、確かめる op | パスワードの確かめ方は、葉 |
| `notice` | 何を知らせるか（op の説明）と、送れなくても流れを止めないこと | 文面は、葉 |
| `fulfilment` | 概念をまたぐ順番のすべて | 葉がない。順番だけの名前空間 |
| `shopApi`、`staffApi`、`hooks` | 経路、入力の読み方、誰が呼べるか、客に見せる形（`OrderView`） | 見せる形への写しは、葉（`view`） |
| `psp`、`carriergw`、`assets`、`mailer` | こちらが使う口と、当てにしている振る舞い | 相手の内側は、金が動く効果だけ（F6） |
| `storefront`、`backoffice` | ページ、アーム、操作、操作が変える状態、失敗の行き先 | 表示への写しは、葉（`…Vm`）。部品、文言、入力の途中は、書いていない |

### 書けたが、書かなかったもの

- **G1 列のしまい方**（`@db.text`、`@db.json`）。`go-sqlite` を選ぶまで、要らない。書いていないので、`go-sqlite` には、そのままでは通らない。
- **G2 画面の部品、文言、入力の途中の状態、制約の違反の出し方。**
- **G3 人の手の動き**（棚から出す、量る、貼る）を、担い手ごとの名前空間の op として。画面の操作の op の引数（量った重さ）に現れるだけ。
- **G4 管理の画面**（商品、箱、料金表、在庫の数、担当者）。状態はあるが、書き込む op がない。
- **G5 メールの中身の形**（`mailer.Mail` は、件名と本文）。
- **G6 見積もりと箱の選び方の中**（`shipping.foresee`、`shipping.fitting`）: 詰め方、地帯の決め方。
- **G7 請求する額の計算を、配線に。** 「送料が上限を超えるか」を分類にすれば、`payment.charged` の中の分岐を、配線に出せる（画面では、そうした: `backoffice.Overrun`）。足し算は、どのみち葉。
- **G8 例外のユニオン**（F34 の並びに、名前を付ける）。
- **G9 `effects` と `reads` の節**。一つも書いていない。`prevents effects` は、一か所だけ（`payment.capture`）。
- **G10 通知の種類の値**（`psp.Event.kind`）と、荷物の動きの、届いた以外の通知。
- **G11 受け付けた後のキャンセル。** 枠を手放す op（`payment.release`）は、期限切れの注文のためにだけある。作るものに入れるなら、取り消した注文は、消さずに、`ordering.Standing` のケース（`Cancelled`）と、そこへ進める効果（行の引数が `Order on Standing.Accepted` で、`as Standing.Cancelled`）を足す（期限切れの `Expired` と同じ形。`docs/requirements.md`「書き方の指針」）。

### 実装に見えたが、書いたもの

- **H1 「引いて、なければ投げる」op**（`ordering.byId`、`shipping.find`、`payment.require`、`production.require`）。
- **H2 時刻を入れるだけの葉は、なくなった。** 効果が行の全体を受け取っていた間は、変えた行を作る葉（`startedAt`、`acceptedAt`、`handedOverAt`、`deliveredAt`、`pickedAt`）が要った。行の遷移を持つ効果は、前の行と、足す事実（時刻など）を受け取るので、葉は要らない（F51）。残っているのは、足す事実を数える葉（`payment.charged`、`shipping.place`）。
- **H3 相手の型に写す葉**（`payment.captureOf`、`refundOf`、`holdOf`、`carrier.rateRequest`、`labelRequest`、`toLabel`）。
- **H4 一覧から項目を抜く葉**（`ordering.skusOf`、`inventory.idsOf`、`skusOfHeld`）。
- **H5 判断の段ごとの配線**（F3）。
- **H6 扱った通知の表**（`payment.seen`）。なくなれば、二度受け付ける。
- **H7 表のキーと一意**（`@db.key`、`@db.unique`）。`Order.requestId` の一意は、「同じ申し込みは一つ」の根拠。ほかのキーは、引き方（`@db.find`）が一行に決まるため。
- **H8 値を運ぶためだけの model**（F18）。
- **H9 `clock.now()` を呼んで、葉に渡す行。**
- **H10 画面を描き直す効果 `reload()`。**

### 規則の候補

どれも決めていない。それぞれ、説明できる事例と、外れる事例。

**R1 境界だけ。** 名前空間の外から見えるもの（`export` の op と、その引数、戻り値、例外、効果）だけを書き、中は葉一つにする。
- 説明できる: G1、G6、H1〜H5 が、仕様から消える。地図が、PRD の概念の一覧と同じ大きさになる。
- 外れる: 今の言語では書けない（「言語が決めている下限」）。書けるようにしても、`shipping.pack` の中の「そろっていなければ梱包できない」は、境界の内側にあるのに、PRD の中心。

**R2 状態に触れる順と、PRD に名前のある分かれ道まで。** `read` と効果を呼ぶ順、概念をまたぐ呼び出し、業務の名前が付く場合分け（分類）は、配線。値を作ることは、葉。このサンプルは、ほぼ、この形。
- 説明できる: `fulfilment.begin` の分け方、`fulfilment.dispatch` の順、`payment.settle` の場合分け、H6。
- 外れる: PRD に名前のない段（H5）と葉（H2〜H4）を、大量に書かせる（葉は 90）。PRD の中心の約束（F11）は、葉の中に残る。「PRD に名前があるか」を決める人が要る。

**R3 約束したことだけ。** 誰か（客、担当者、外部の仕組み）への約束と、それを破りうる順番だけを書く。このサンプルでは、`@pact.promise` を付けた七つの op と、そこから辿れる配線。
- 説明できる: 難しい問題の全部、同じ申し込みは一つ、`TermsChanged`、G2〜G5 を書かないこと。
- 外れる: 何が約束かを決める規則がない。今の言語は、約束の中身（金額の不等式）を書けず、約束でない段（H1〜H5）を書かせるので、言語の形と合っていない。`staff` と `carrier` の管理のように、約束らしい約束のない概念が、何も書かれなくなる。

**R4 作り直しても、変わってはいけないものだけ。** op の中身を AI が書き直したときに、変わったら困るものが、仕様。変わってよいものは、葉。
- 説明できる: G6（詰め方は、変わってよい）、H2〜H4 は葉でよいこと、H6 と H7（表が変わると、動いているデータが壊れる）、`external/` の口（相手は、作り直せない）。
- 外れる: G1（列のしまい方も、データがあれば変えられないので、書く側に倒れる）。画面の文言と部品（変わってよいのか、決める人が要る）。「困る」の相手が、利用者か、データか、外部かで、線が変わる。

四つは、「必要になったら足す」の「必要」を、別のものに置いている: R1 は、別の概念が見るとき。R2 は、状態か、名前の付いた判断に触れるとき。R3 は、誰かに約束したとき。R4 は、作り直しで壊れるとき。

## 利用者への問い

残りを止めている順。それぞれ、一つで読める問いと、選択肢と、勧め。

### Q1 表の一行の進み具合を、遷移として書けるようにするか

**決まった: 書ける。** 効果が、宛先（`effects 状態.操作`）の後ろの `as` で、受け取った行のケースをどう変えるかを言う（行の遷移。`docs/check.md`「行の遷移」、`docs/decisions.md`「決めたこと」163〜173）。このサンプルの書き直しは「難しい問題」の「行の遷移で書いた進み具合」、分かったことは F1 と F50〜F58。事実ごとの表で持つ書き方（`parcels`、`labels`）は、そのまま使っている。

残っている問い（どれも、決めていない）:

- **行の並びを受け取る効果に、行の遷移を書けるようにするか**（F50）。今は、一行ずつ。
- **一つの効果が、二つの分類について言えるようにするか**（F52）。今は、一つの `as` に、一つの分類。
- **行のケースを変える効果の中身を、`db` の語彙で言えるようにするか**（F51、F54）。今は、人が書く。

### Q2 仕様に書く深さの規則を、どれにするか

「規則の候補」の R1〜R4。今の言語は R2 を強いている。

- **(a) R2 を指針にする。** 言語は変えない。
- **(b) R1 に寄せる。** 葉が、読む状態と効果を宣言できる形に戻す（STATE-5 をやめる）。
  ```ts
  fn pack(id: ShipmentId, box: BoxCode, weight: type.Grams): Parcel
    reads shipments, boxes, tariffs effects createParcel
    throws NotGathered, AlreadyPacked;
  ```
- **(c) R2 を下限、R3 を上限にする。** 配線は、約束（`@pact.promise` のような印）から辿れるところまで下ろし、その先は、葉にしてよい。

勧め: **(c)**。(b) は、`docs/requirements.md`「出力を推論しない」の「人が書くコードを、できるだけ減らす」と当たる。(c) は、どこで止めるかに、理由を書かせる。

### Q3 分けて送ることを、作るものに入れるか

入れると、支払いのモデルが変わる（「難しい問題」の「分けて送ること」）。

- **(a) 入れない。** 一つの注文に、出荷は一つ。
- **(b) 店が決めて分ける。** 増えた送料は店が持つ。請求の確定は、最後の箱のとき。客の額は変わらない。
- **(c) 客が頼んで分ける。** 上限を出し直して、同意を取り直す。請求は、上限の額で先に確定して、後で返す。

勧め: **(a)** のまま、粒度の問い（Q2）を先に決める。入れるなら **(b)**: 客への約束を変えずに済む。

### Q4 例外が運ぶ値を、受ける側で使えるようにするか

今は、デコレータが例外を受けても、値を使えない（F4）。

- **(a) 今のまま。** 画面で使う値は、例外ではなく、op の結果で返す。
- **(b) `halts` の効果に、例外の引数を渡せる。**
  ```ts
  decorator fn recover(error: any exception, then: any effect): catches error halts then(error);
  // @recover(ordering.TermsChanged, showNewTerms)  →  showNewTerms(terms)
  ```
- **(c) 配線で、例外を値にできる。**
  ```ts
  const placed = try shopApi.checkout(…);
  match placed by Outcome { … }
  ```

勧め: **(b)**。(c) は、`docs/grammar.md`「決めたこと」の「`throw` はアームの本体にだけ」と同じ理由で、配線を、制御の言語にする。

### Q5 配線に、一覧の要素ごとの呼び出しを入れるか

今は、一覧を受け取る効果か、葉か、時計で一件ずつ（F2）。

- **(a) 今のまま。**
- **(b) 要素ごとに op を一つ呼ぶ行だけを足す。**
  ```ts
  each line of part.made => production.enqueueOne(orderId, line);
  ```
- **(c) 要素ごとの分類だけを足す**（混ざった注文を、配線で分けられる）。
  ```ts
  const sorted = partition lines by catalog.Kind;
  ```

勧め: **(a)**。一覧を受け取る op で、足りた。困ったのは、外部の仕組みを要素ごとに呼ぶ一か所（`payment.prepayExpiring`）だけ。

### Q6 設計だけの段階で、プラグインの語彙を読めるようにするか

今は、単位に書いたトランスパイラのプラグインの語彙しか、読まれない（F22）。

- **(a) 今のまま。** 設計の段階の語彙は、プロジェクトの `.hozo` に書く。
- **(b) `hozo.toml` で、語彙だけを読むと言える。**
  ```toml
  [hozo]
  plugin_dirs  = ["plugins"]
  vocabularies = ["pay"]        # このプラグインの語彙を、単位がなくても読む
  ```
- **(c) 出力のない単位を書ける**（`out` のない `[[emit]]`）。

勧め: **(b)**。`docs/requirements.md`「モデルが先」の「設定なしの検査」と同じ向き。

### Q7 概念の名前空間が、状態を持ってよいか

指針は「DB の状態は、別の名前空間に切り出す」。このサンプルは、従っていない（F38）。

- **(a) 指針のまま。** 概念ごとに、状態の名前空間を切り出す（`shipping` と `shipping.store`）。
- **(b) 指針を、「状態は、それを守る規則と同じ名前空間に置き、ほかの概念は `export` の op を通す」に変える。**
- **(c) 指針を、小さな仕様（`shop-server`）のためのものと書き足す。**

勧め: **(b)**。切り出す目的（ほかの名前空間が op を通すしかなくなる）は、概念の境界で、もう満たされている。

### Q8 上限を超えた送料を、店が持つ、でよいか

作るものの問い。今は、上限までを請求し、超えた分を店が持つ（「送料が、見せた額と違ったとき」）。

- **(a) 店が持つ。**
- **(b) 客に、追加の同意を求める。** 梱包した箱が、同意を待つ。
- **(c) 上限を超える箱は、選べない。** 梱包の担当が、管理の担当に渡す。

勧め: **(a)**。客への約束が一つ（「上限を超えない」）で済む。

### Q9 例外の言い換えを、吸収と別のものにするか

`catches A recovers throws B` は、効果を持つ op で DEC-5 になる（F5）。

- **(a) 今のまま。** 境界で、名前をそろえない。
- **(b) 言い換えの節を足す。** 効果の状態は、元の例外のものを引き継ぐ。
  ```ts
  decorator fn failsAs(error: any exception): catches http.NetworkError, http.ServerError rethrows error;
  ```

勧め: **(b)**。入口の応答（`@http.status`）を、相手ごとに決められる。

### Q10 不変条件を、検査できる形で書けるようにするか

今は、葉のコメントと、印 `@pact.promise`（F11）。

- **(a) 印のまま。** 約束と op を結ぶだけ。
- **(b) 葉の op に、結果の条件を書ける。** アサーションの材料にする。
  ```ts
  fn charged(payment: Payment, fee: type.Yen, now: datetime): Payment
    ensures result.captured <= payment.limit;
  ```

勧め: **(a)** から始める。(b) は、式の言語が要る。印の文を、テストの名前にするところまでなら、言語を変えずにできる。

### Q11 宣言したケースの外の値を、仕様の上で受けられるようにするか

**閉じた: 受けられるようには、しない。** 分類の誤りは、仕様が扱わない、ありえないことが起きた誤りで、受ける例外は、言語に足さない。当たらない値に対して何かをしたいなら、それは分類できるということなので、ケースとして宣言して、枝に書く（`docs/requirements.md`「書き方の指針」）。このサンプルの二つの通知の入口は、受け取るだけの通知に、名前のあるケースを足した（`psp.Meaning.Echo`、`fulfilment.Arrival.Underway`。F47）。

## 近づけたもの

言語で言えず、近いもので書いたところ。

| 言いたかったこと | 書いたもの |
|---|---|
| 行のケースを変える効果の中身 | 人が書く効果（`db` の語彙は、行の全体を書くか、列の条件で消す。F51、F54） |
| いくつかの行のケースを、一度に変える | 時計で、一行ずつ（F50） |
| 設定の値 | `single` の状態（F14） |
| 「あれば置き換え、なければ入れる」 | `nullable` の `put`（F17） |
| 相手の中で金が動くこと | 相手の名前空間の状態と、`@outside.does`（F6） |
| プラグインが提供する語彙 | プロジェクトの `.hozo`（F22） |
| パスの引数、PUT / DELETE | クエリ、POST（F23） |
| 一覧の要素ごとの処理 | 一覧を受け取る効果、葉、時計で一件ずつ（F2） |
| 一つのトランザクション | 入口の `@db.transaction`（F19） |
| 値の集合 | 文字列の scalar とコメント、真偽値の組（F8） |

## R6 で変えたこと

R6 の、言語の部分（`docs/requirements.md`「R6 の、言語の部分」）を、実装より先に、このサンプルに入れた。実装は、その後に入り、新しい書き方の仕様は、`check` を通る（「確かめ方」）。

| 変えたもの | 変えたこと |
|---|---|
| すべての `.hozo` | `const` を `val` に（162 か所）。`match` の頭を、丸括弧で囲み、`}` の後ろの `;` を、省く（74 か所） |
| `ordering.expireUnpaid` | 期限切れの注文を、一回の動きで、すべて扱う: read `oldestUnpaidBefore(…): Order?` を、並びを返す `unpaidBefore(…): Order[]` に替え、`for` で、一つずつ、`expireIfUnpaid` を呼ぶ |
| `ordering.placeNew` | 値段を付けることと、時刻を読むことを、`disjoint` に並べる。取り置き（例外を出しうる）と、注文を足すこと（効果）は、並べられない（DISJ-1）ので、直列のまま |
| `production.enqueue` | 行ごとに、仕事を作って、足す: 効果 `addJobs(added: Job[])` を、行を一つ受け取る `addJob(job: Job) … as Progress.Queued` に替え、葉 `plan`（並びを返す）を、`planJob`（一つを返す）に替えた。入るケースを、書けるようになった |
| `payment.prepayExpiring` | 期限の近い支払いを、一回の動きで、すべて扱う: read `nextExpiringHold(…): Payment?` を、並びを返す `holdsExpiringBefore(…): Payment[]` に替え、`for` で、一つずつ、`prepayHeld` を呼ぶ。`prepay` は、想定した失敗を、受けて、支払いに残す（`try` / `catch (e by PrepayFailure)`、効果 `notePrepayFailure`、項目 `prepayFailedAt`、`prepayFailure`）。`prepayExpiring`、`prepayHeld`、`prepay` の `throws` は、なくなった |
| `shopApi.statusOf` | 四つの読みを、`disjoint` に並べる。名前を並べた、空の `val` |

- **`payment.prepayExpiring` は、失敗を、状態に残す形にしてから、繰り返しにした**（`docs/decisions.md`「決めたこと」345）。そのまま繰り返しにすると、一つの支払いで、外の仕組みが失敗したとき、残りの支払いが、その回では、扱われない（`docs/wiring-trial.md` の W5）。`prepay` は、本体で、想定した失敗を受けて、支払いに残す: `try { … } catch (e by PrepayFailure) { Expected => notePrepayFailure(payment, now); Unexpected => throw e; }`（`docs/decisions.md`「決めたこと」419、422）。前は、デコレータ `payment.noteFailure`（`catches … halts …`）を、例外ごとに三度付けていた: そこでは、効果は、引数を受け取れず、どの行に書くかを、デコレータの中身（継ぎ目）が決めていた。受ける文にして、効果 `notePrepayFailure` に、行（`payment`）と時刻（`now`）を、配線が渡すので、どの支払いに残すかが、仕様に書かれる。`noteFailure` は、なくした。例外の分類 `PrepayFailure` が、想定した失敗（`Expected`）と、そうでないもの（`Unexpected`）を分ける。繰り返しの中身は、想定した失敗を出さなくなり、一つが失敗しても、残りは、続く。残した失敗は、次に時計が動いたときに、もう一度、試される。想定していない失敗は、ここまで届き、繰り返しを、止める。
- **`notice.bestEffort` に、本体を書いた**（同 419、422）: `catches mailer.Undelivered ignores` の節を、`try { next(); } catch (e by Delivery) { Undelivered => none; Other => throw e; }` にした。受ける例外は、例外の分類 `notice.Delivery` が言う。
- `ordering.expireUnpaid` は、外の仕組みを呼ばず、`@db.transaction` を持つ: 途中で失敗したら、それまでの分も、戻る。
- **`Record<K, V>` をなくした**（`docs/decisions.md`「決めたこと」360）のに合わせて、二つの画面（`storefront`、`backoffice`）の、URL のクエリの状態を、`state query: single<browser.page.Query>;` にした。`browser.page.Query` は、標準の語彙 `browser.page` が宣言する、インデックスの model（`docs/vocab.md`）。
- **標準の分類 `nullable` のケースの名前を、`None` / `Some` に替えた**（`docs/decisions.md`「決めたこと」362）のに合わせて、二つの画面（`storefront`、`backoffice`）の、形が `nullable` の状態の遷移（`effects token as None -> Some`、`effects offer as None -> Some`）と、`match (… by nullable)` のアームを、書き直した。形が `list` の所（`Empty` / `HasItems`）は、変えていない。
- **絞り込んだ型を `T on C.K` と書き、行の遷移の、前のケースを、行の引数の型で言う**（`docs/decisions.md`「決めたこと」368〜370）のに合わせて、絞り込んだ型（`C.K<T>`）と、行の遷移を持つ 24 の効果を、書き直した（`effect fn startJob(job: Job, …): effects jobs.put as Progress.Queued -> Progress.InProduction;` を、`effect fn startJob(job: Job on Progress.Queued, …): effects jobs.put as Progress.InProduction;` に）。`post` の効果の「前」（`as Packing.Unpacked -> Packing.Packed`）と、`as any` は、なくなった（F57、F53）。配線で変えたのは、`payment.markSecured` の一か所（F59）。

## 配線の op の `throws` を消したこと

配線の op は、`throws` の節を書かなくてよい（`docs/decisions.md`「決めたこと」379。決まりの持ち主は `docs/check.md`「状態に触れるもの」の「節に書く要素」）。このサンプルの、配線の op の `throws` の節を、99 の op から消した。op の例外（`Signature.exceptions`）は、どの op も、消す前と同じ。

| 置き場 | 節を消した op の数 |
|---|---|
| `api/` | 22（`hooks.hozo` 1、`shop-api.hozo` 7、`staff-api.hozo` 14） |
| `domain/` | 69（`carrier.hozo` 8、`download.hozo` 2、`fulfilment.hozo` 10、`inventory.hozo` 4、`ordering.hozo` 10、`payment.hozo` 10、`production.hozo` 3、`shipping.hozo` 14、`staff.hozo` 8） |
| `web/` | 8（`backoffice.hozo` 3、`storefront.hozo` 5） |

- **節を残した配線の op は、`payment.capture` の一つ。** 例外と効果の関係（`throws psp.HoldExpired prevents effects`）を、書いて言っている op で、残した節は、その一つだけ。はじめは、関係を付けない節（`throws http.NetworkError, http.ServerError`）も、一緒に残していた（節を書いた配線の op は、計算した例外を、節が覆わなければ、エラーだった）。節を、その関数が足すものにした決定（`docs/decisions.md`「決めたこと」381）で、その節を消した: 二つの例外は、`psp.capture` の呼び出しから伝わってくるもので、書かなくても、`capture` の例外に入る（`docs/check.md`「状態に触れるもの」の「節に書く要素」）。
- **ほかに `throws` を書いているのは、定義する所だけ**: 葉の op（12）と、デコレータの宣言（`outside.failsAs`、`webhook.signed`）。
- 「分かったこと」の F34（書く量をいちばん増やしたのは THROWS-1）は、消す前の記録。THROWS-1 は、その後、欠番になった（同 381）。

## 確かめ方

- このディレクトリで `go run ../../../cmd/hozoc build --no-emit`。結果は `0 errors, 0 warnings`。
- リポジトリで `go test ./tests/ -run TestSamples/order-app -count=1 -v`。走るのは `check` だけ（`../README.md`「仕様だけのサンプル」）。
