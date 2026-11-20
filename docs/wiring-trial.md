# 配線の綴りの確かめ（R6）

## この文書が決めること

R6（`requirements.md`「作り直しの段階」）で入れる、配線の綴りを、全体として確かめた記録（`decisions.md`「決めたこと」322）。

- 綴りの決まりは、決めない。持ち主は、`ide-chain.md`「アクティビティ図」（R6 を作るときに、`grammar.md`「配線」へ移す）。
- この文書が持つのは、三つ: 文法の全体の案、サンプル `order-app` の op の書き直し、そこから分かったこと。
- 書き直しは、記録。サンプルの `.hozo` は、変えていない。今の `hozoc` は、この綴りを、読めない。
- 確かめたときのキーワードは、`parallelizable`。後で、`disjoint` に替えた（`decisions.md`「決めたこと」343）ので、この文書の綴りも、`disjoint` に、書き換えてある。

## 確かめた綴り

| 綴り | 決めた所（`decisions.md`「決めたこと」） |
|---|---|
| `val x = …;`、`val x;`、`val x: T;`、`val x, y;`、`x = …;` | 309、310、312、336 |
| `disjoint { … }` | 294〜299、302、311、343 |
| `for (val x of xs) { … }`、`disjoint for (…) { … }` | 307、308、314、319 |
| `xs collect x;` | 317、318 |
| `match (x by C) { … }` | 315、316 |
| `;` は、文の終わりの印。文が `}` で終われば、省ける | 313 |
| 未代入の `val` は、読めない | 320 |
| `for` の中と、アームの中に、`return` は、書けない | 321 |

## 文法の全体（案）

今の文法（`grammar.md`「配線」）に、上の綴りを入れた形。**案で、確かめていない。**

```
Block = "{" { Stmt } "}" ;
Stmt  = ValDecl
      | Assign
      | Collect
      | "return" Expr End
      | Call End
      | Match End
      | Disjoint End
      | [ "disjoint" ] For End ;

ValDecl = "val" ValName { "," ValName } End    (* 空で宣言する。名前を並べられる（336） *)
        | "val" Ident "=" Expr End ;            (* 値と一緒に宣言する *)
ValName = Ident [ ":" Type ] ;
Assign  = Ident "=" Expr End ;
Collect = Ident "collect" Expr End ;
End     = [ ";" ] ;                             (* 省けるのは、直前の字句が "}" のときだけ *)

Expr  = Call | Match | Atom ;
Match = "match" "(" [ Atom ] "by" ( QualifiedName | "list" | "nullable" ) ")"
        "{" MatchArm { MatchArm } [ ElseArm ] "}" ;

Disjoint = "disjoint" "{" Arm Arm { Arm } "}" ;
Arm     = Block | ArmStmt ;
ArmStmt = Assign | Collect | Call End | Match End
        | Disjoint End | [ "disjoint" ] For End ;

For = "for" "(" "val" Ident "of" Atom ")" Block ;
```

`Call`、`Atom`、`MatchArm`、`ElseArm`、`ArmBody`、`Throw` は、今のまま（`grammar.md`「配線」）。

### 案で埋めた所

決定が言っていないので、案を書くために、仮に決めた所。四つとも、仮の決めのとおりに、決めた（`decisions.md`「決めたこと」341）。

| 所 | 仮の決め | 別の案 |
|---|---|---|
| `for` の頭の、並び | `Atom`。呼び出しは、書けない（引数に呼び出しを書かない決まりに、合わせた） | 呼び出しも書ける |
| 文の一つのアームに、書ける文 | `=`、`collect`、呼び出し、`match`、`disjoint`、`for`。`val` と `return` は、書けない | ブロックだけにする |
| `collect` の右 | 式（呼び出しを、じかに書ける）。`=` と同じ | 値だけ |
| `return` を書けない所（321） | 文法ではなく、検査で弾く | `for` とアームの中のブロックを、別の生成規則にする |

## 書き直し

サンプル `order-app` の、五つの op。「今」は、サンプルの文面のまま。五つ目（`shopApi.statusOf`）は、335 と 336 を決めるときに、足した。

### 期限切れの注文を、引く（`ordering.expireUnpaid`）

今は、時計が動くたびに、いちばん古い一つだけを、扱う（`tests/samples/order-app/README.md`「分かったこと」の F2、F50）。

```
@clock.tick(every: 1m)
@db.transaction
fn expireUnpaid() {
  const now = clock.now();
  const deadline = before(now, paymentWindow);
  const stale = oldestUnpaidBefore(deadline);
  match stale by Known {
    Found   => expireIfUnpaid(stale, now);
    Missing => none;
  };
}
```

書き直し。read を、一つ返すもの（`oldestUnpaidBefore(…): Order?`）から、並びを返すもの（`unpaidBefore(…): Order[]`）に、替える。

```
@clock.tick(every: 1m)
@db.transaction
fn expireUnpaid() {
  val now = clock.now();
  val deadline = before(now, paymentWindow);
  val stale = unpaidBefore(deadline);
  for (val order of stale) {
    expireIfUnpaid(order, now);
  }
}
```

`expireIfUnpaid` と、その先（行の遷移の効果 `expireOrder`）は、変わらない。

### 枠の期限が近い支払いを、先に確定する（`payment.prepayExpiring`）

今は、時計が動くたびに、期限がいちばん近い一つだけを、扱う。

```
@clock.tick(every: 10m)
fn prepayExpiring():
  throws psp.HoldExpired, http.NetworkError, http.ServerError
{
  const now = clock.now();
  const deadline = ahead(now, holdMargin);
  const next = nextExpiringHold(deadline);
  match next by Known {
    Found   => prepayHeld(next, now);
    Missing => none;
  };
}
```

書き直し。read を、並びを返すもの（`holdsExpiringBefore(…): Payment[]`）に、替える。

```
@clock.tick(every: 10m)
fn prepayExpiring():
  throws psp.HoldExpired, http.NetworkError, http.ServerError
{
  val now = clock.now();
  val deadline = ahead(now, holdMargin);
  val expiring = holdsExpiringBefore(deadline);
  for (val payment of expiring) {
    prepayHeld(payment, now);
  }
}
```

`prepayHeld` の先は、外の仕組みを呼び（`psp.capture`）、行の遷移の効果を起こす（`recordPrepaid`）。どちらも、変わらない。

### 新しい注文を作る（`ordering.placeNew`）

今。

```
{
  const priced = price(checkout.lines, checkout.destination);
  const now = clock.now();
  const order = draft(checkout, priced, now);
  holdStock(order);
  createOrder(order);
  return order;
}
```

書き直し。値段を付けることと、時刻を読むことは、互いに依存しないので、並列にできる、と書ける。

```
{
  val priced;
  val now;
  disjoint {
    priced = price(checkout.lines, checkout.destination);
    now = clock.now();
  }
  val order = draft(checkout, priced, now);
  holdStock(order);
  createOrder(order);
  return order;
}
```

`holdStock(order);` と `createOrder(order);` は、並列にできる、とは、書けない: `holdStock` は、例外（`inventory.OutOfStock`）を出しうり、`createOrder` は、効果を持つ（295）。

### 受注製作の行を、仕事にする（`production.enqueue`）

今は、行ごとに仕事を作ることが、葉の op（`plan`）の中身に、隠れている。効果は、仕事を、まとめて受け取る。

```
effect fn addJobs(added: Job[]): effects jobs.post;

export fn enqueue(orderId: type.OrderId, lines: ordering.MadeLine[]) {
  const now = clock.now();
  const planned = plan(orderId, lines, now);
  addJobs(planned);
}

fn plan(orderId: type.OrderId, lines: ordering.MadeLine[], now: Time): Job[];
```

書き直しの一つ目。効果を、回ごとに呼ぶ。効果は、行を一つ受け取るので、入るケースを、書ける。

```
effect fn addJob(job: Job): effects jobs.post as Progress.Queued;

export fn enqueue(orderId: type.OrderId, lines: ordering.MadeLine[]) {
  val now = clock.now();
  for (val line of lines) {
    val job = planJob(orderId, line, now);
    addJob(job);
  }
}

fn planJob(orderId: type.OrderId, line: ordering.MadeLine, now: Time): Job;
```

書き直しの二つ目。効果は、まとめて受け取るまま。仕事を、`collect` で集める。

```
export fn enqueue(orderId: type.OrderId, lines: ordering.MadeLine[]) {
  val now = clock.now();
  val planned: Job[];
  for (val line of lines) {
    planned collect planJob(orderId, line, now);
  }
  addJobs(planned);
}
```

### 注文のいまを、客に見せる（`shopApi.statusOf`）

四つの概念から、事実を、一つずつ読み、画面に見せる形に、まとめる。本当に、並列にしたい形。

今:

```
fn statusOf(order: ordering.Order): OrderView {
  const charge = payment.ofOrder(order.id);
  const progress = shipping.progressOfOrder(order.id);
  const jobs = production.ofOrder(order.id);
  const grants = download.granted(order.id);
  return view(order, charge, progress, jobs, grants);
}
```

書き直し:

```
fn statusOf(order: ordering.Order): OrderView {
  val charge, progress, jobs, grants;
  disjoint {
    charge = payment.ofOrder(order.id);
    progress = shipping.progressOfOrder(order.id);
    jobs = production.ofOrder(order.id);
    grants = download.granted(order.id);
  }
  return view(order, charge, progress, jobs, grants);
}
```

## 分かったこと

### 書けるようになったこと

- **W1 「時計で、一件ずつ」が、一度で書ける。** `expireUnpaid` は、期限切れの注文を、一回の動きで、すべて扱える。`match … by Known` の分かれ道は、なくなる（並びが空なら、回らない）。F2 と F50 の、逃げ道の一つが、要らなくなる。
- **W2 行の遷移の効果を、回ごとに呼べる。** 前のケースは、回の中の `match` で分かるので、呼ぶ側の検査（STATE-9）は、そのまま通る。
- **W3 行を一つ受け取る効果にできるので、入るケースを、書ける。** `production.enqueue` の一つ目の書き直しでは、`addJob` に、`as Progress.Queued` を書けた。今の `addJobs` は、まとめて受け取るので、書けない。ハーネスが、上がる。
- **W4 295 が、設計の意図を、守る。** `placeNew` の、「取り置きの後に、注文を足す」は、今は、コメントにある。書き直しでは、二つを、並列にできる、と書こうとすると、検査が弾く。

### 書けないこと、合わないこと

- **W5 失敗しうる呼び出しを繰り返すと、一つの失敗が、後ろの回を、止める。** `prepayExpiring` の書き直しでは、一つの支払いで、外の仕組みが失敗すると、残りの支払いは、その回では、扱われない。今の、時計で一件ずつ、の形は、失敗を、回ごとに分けていた。回ごとに、例外を受けて、次へ進む書き方は、配線に、ない。 → 決めた: 止める。失敗を分けたいものは、時計で一件ずつ、のままにする（`decisions.md`「決めたこと」323）。 その後、止めては困るものは、失敗を、op のデコレータで受けて、状態に残す形にする、と決めた（同 345）。サンプルの `prepayExpiring` は、その形にして、繰り返しにした。
- **W6 繰り返しの中の、例外と効果の組は、多くが、`unknown` になる。** 検査の計算（`check.md`「計算」）は、例外と効果の組ごとに、`kept`（起きている）、`prevented`（起きていない）、`unknown`（決まらない）を出す。`prepayExpiring` の書き直しでは、途中の回で例外が出たとき、前の回の効果（`recordPrepaid`）は、起きていて、今の回の効果は、起きていない。答えは、`unknown`。計算に、繰り返しの決まりは、まだない。「例外のとき、効果は起きていない」（`prevents effects`）と書いた約束は、繰り返しの中では、守れない。 → 決めた: 直列に動いたとして、計算する。本体は、0 回以上、動き、前の回の効果は、起きている（`decisions.md`「決めたこと」335）。
- **W7 `collect` は、回が、互いに依存しないときにだけ、合う。** 在庫の取り置き（`inventory.reserve` の `allocate`）は、前の行までの取り置きを見て、残りを決めうる。集める先は、`for` の中で読めない（320）ので、`collect` では、書けない。葉の op のまま。
- **W8 `disjoint` は、軽いアームでは、行が増えるだけ。** `placeNew` は、6 行が、10 行になった。二つの読みを並べるだけなら、得は、小さい。 → 空の `val` は、名前を並べて、一つの文で、宣言できる、と決めた（同 336）。

### 図の部品との対応

| 綴り | アクティビティ図の部品（`ide-chain.md`「アクティビティ図」の「対応」） |
|---|---|
| 呼び出しの文、`val x = 呼び出し;` | 作業 |
| 文の順 | 作業の順 |
| `match` | 分かれ道 |
| アームの `throw` | 分かれ道の先の、終わり（例外） |
| `disjoint` | 並行 |
| `for` | 繰り返し |
| `val x;`（空の宣言）、`x = …;`、`xs collect x;` | **確かめたときは、当たる部品が、なかった** |

- **W9 値を入れる文は、図の部品に、当たらない。** 空の `val` の宣言、`=`、`collect` は、値の受け渡しを言う。アクティビティ図に描くなら、作業から作業への、値の線になるが、今の対応の表に、その部品は、ない。図から配線を作るとき、どこで `val` を宣言し、どこで入れるかを、図の何から決めるかが、決まっていない。 → 決めた: UML の、値の流れの部品（オブジェクトフロー、拡張領域の、出口の集まり）に、対応させる（`decisions.md`「決めたこと」324）。

### 綴りそのもの

- **W10 `for` の頭に、呼び出しを書けないと、行が一つ増える。** 案では、並びを、いったん `val` に受ける（`val stale = unpaidBefore(deadline);`）。名前が付くので、読みやすさは、落ちなかった。
- **W11 `collect` の右に、呼び出しを、じかに書くと、短い。** `planned collect planJob(orderId, line, now);`。`val` に受けてから集める形と、どちらも書ける。
- **W12 四つの op で、`;` を省いたのは、`for` と `disjoint` の後ろだけ。** 式で終わる文（`val`、`=`、`collect`、呼び出し、`return`）は、どれも、`;` で終わった。
- **W13 読みをいくつか集める形には、例外と効果の組が、ない。** `statusOf` の四つのアームは、読むだけで、効果を持たず、仕様の例外も、出さない。検査の計算が答えるものが、なく、アームについての検査（295）にも、掛からない。読みが、仕様の例外を出す形でも、どのアームも、効果を持たないので、直列でも、並列でも、答えは同じ。`disjoint` に、例外のときの扱いを明示する書き方に、出番は、なかった（同 335）。確かめたのは、この一つの op。
