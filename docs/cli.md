# hozoc

## この文書が決めること

`hozoc` のサブコマンドの振る舞い。hozoc 自身の仕様 `spec/cli.hozo`（`docs/requirements.md`「自己展開」）の持ち主は、そのファイル（「cli.hozo」）。骨組みは `spec/cli.hozo` から三つのトランスパイラが生成する: 型と op の形と配線は基底 `go`（`docs/plugin-go.md`）、入力の制約を確かめる関数は `go-valid`（`docs/plugin-go-valid.md`）、引数の解釈と表示と終了コードは `go-cli`（`docs/plugin-go-cli.md`）。ここで決めるのは葉の op の中身。hozoc 本体は、コードを出力しない（`docs/plugin-protocol.md`「この文書が決めること」）。

## cli.hozo

hozoc 自身の仕様の持ち主は、`spec/cli.hozo`（決定 D807、D808）。宣言の意味と、決定に書いていない理由は、その宣言の説明のコメントが持つ。形を決めた決定は、その宣言の `@adr.decided` が名指しする。この文書は、写しを置かない。

`spec/cli.hozo` から生成する単位（名前空間と、トランスパイラと、置き場）は、リポジトリ直下の `hozo.toml` が持つ。

この文書と `docs/` のほかの文書が、修飾なしに `cli.hozo` と書けば、`spec/cli.hozo` を指す。go-cli プラグインの語彙のファイルは、`plugins/go-cli/cli.hozo` と書く（`docs/plugin-go-cli.md`「マニフェスト」）。

`spec/` のほかのファイルが何を書き、持ち主がどこで、コードになるかは、それぞれのファイルの頭のコメントが言う。

## build

`hozoc build [--no-emit] [--dry-run] [--format text|json] [--config <パス>] [<名前空間>…]`。仕様を検査し、プラグインから受け取ったファイルを書き出す（決定 D417）。TypeScript の `tsc` と `tsc --noEmit` と、同じ形。

| 書き方 | フラグ（分類 `Flag`） | 入るユースケース | すること |
|---|---|---|---|
| `hozoc build` | `noEmit` が `Off`、`dryRun` が `Off` | `write` | 検査して（「検査」）、書き出す（「書き出し」） |
| `hozoc build --no-emit` | `noEmit` が `On` | `verify` | 検査だけ。ファイルには、触れない（読みも、書きも、消しもしない）。「書き出し」の 1〜3（ファイルを集め、`path` と衝突を確かめる）までは、行う |
| `hozoc build --dry-run` | `noEmit` が `Off`、`dryRun` が `On` | `preview` | 書き出さずに、書き出す予定のファイルを表示する（「書き出し」の 4） |

- **入口が、フラグで、道を分ける**（決定 D472）: コマンドの op `hozo.cli.build` は、入力のフラグを、分類 `Flag` の `match` で見て、三つのユースケースの、どれかを呼ぶ（`build` が `noEmit` を見て、`On` なら `verify`。`Off` なら、内部の op `writeOrPreview` が `dryRun` を見て、`On` なら `preview`、`Off` なら `write`）。フラグは、ユースケースには、渡さない（ユースケースの入力 `EmitInput` は、`noEmit` も `dryRun` も持たず、書き出す予定 `EmitPlan` も、持たない）。三つのユースケースは、どれも、はじめに `check` を呼ぶ: `verify` は、その後に `verifyOutputs`、`preview` は `collectOutputs` と `plannedChanges`、`write` は `collectOutputs`、`outputChanges`、`syncOutputs`（`spec/cli.hozo`）。
- **`--no-emit` の表示**は、検査の結果だけ: text は、warning と、数の行（`0 errors, 0 warnings`。`docs/diagnostics.md`「text」）。warning だけなら、終了コードは 0（hozoc 自身の仕様では、CFG-26 が二つ出る: `docs/vocab.md`「解釈されなくてよいデコレータ」）。json は、`{"warnings": [...]}`。`written` と `removed` は、出さない（いつも空）。`--no-emit` のない `build` は、warning と、`written: <パス>` / `removed: <パス>` の行（json は、`warnings`、`written`、`removed`）。
- `--no-emit` と `--dry-run` を、両方書けば、`--no-emit`（書き出す予定も、表示しない）: `build` が、先に `noEmit` を見て、`verify` に入るので、`dryRun` は、見ない。
- 名前空間の引数は、`--no-emit` でも、集めて確かめるファイルの範囲を選ぶ（「書き出し」の 1。`UnknownNamespace` も、同じ）。
- **`hozo.toml` がないとき**: `build --no-emit` は、設定なしで検査する（「検査」の 1）。`build` は、書き出す単位がないので、CFG-7 で `ConfigInvalid`（「書き出し」の 0）。
- **古い名前**: `hozoc check` と `hozoc emit` は、なくした。その名前で呼ばれたら、何もせずに、新しい書き方を標準エラーに示して、使い方の誤りの終了コード（64。`docs/diagnostics.md`「終了コード」）で終わる。示すのは、残りの引数を付けたもの: `hozoc check --format json` には `hozoc build --no-emit --format json`、`hozoc emit --dry-run server` には `hozoc build --dry-run server`。

この節の細部は、決定 D694、D751、D770 が決めた。

### 検査

内部の op `check`（コマンドでも、ユースケースでもない。ユースケース `verify`、`preview`、`write`、`confirmTests` の配線が、はじめに呼ぶ）。

1. `hozo.toml` を読み、入っているプラグインのマニフェストを探して読み、各単位の `transpilers` の名前をトランスパイラに解決する（`docs/config.md`「置き場と探し方」「単位」、`docs/plugin-protocol.md`「探索」）。探索が見るプロジェクトの外の場所は、入力の `pluginPath`（`HOZO_PLUGIN_PATH`）と、`cmd/hozoc` が渡すホームディレクトリ（「手で書くもの」）。`hozo.toml` がなければ設定なしで進む（作業ディレクトリをルートにし、`sources` は既定。プラグインは探さない）。設定とマニフェストの error は、CFG-16 なら `LanguageMismatch`、それ以外は `ConfigInvalid`（`docs/config.md`「診断」、`docs/plugin-protocol.md`「診断」）。
2. 標準の語彙、使うプラグイン（`docs/plugin-protocol.md`「探索」）の語彙、`sources` のファイルの順に読み、解析する（`docs/grammar.md`）。`in` は読むファイルを変えない（`docs/config.md`「in」）。
3. 解決（`docs/resolve.md`）、正規化（`docs/ir.md`）、計算と検査（`docs/check.md`）を行う。
4. 1〜3 に error が一つもなければ、プラグインに頼む。はじめに、語彙のプラグインごとに一回、`direction: "read"` を送る（相手は `docs/plugin-protocol.md`「語彙のプラグイン」: 仕様が使っている語彙のもの。単位がなくても、頼む）。その応答に error がなければ、使うプラグインごとに一回、`direction: "emit"` を送る（相手と、受け持つ出力は同「起動」）。どちらの応答も、返った `errors` を error の診断として、`warnings` を warning の診断（`PLUGIN`）として、足す（同じ診断は、一度だけ。同「emit」）。語彙の使い方の誤り（HTTP-5 など）と、トランスパイラが返す誤り（GOHTTP-16 など）は、ここで出る（決定 D404、D410、D416）。返ったファイルは、書かない: 同じ実行の、後の段（「emit」の `collectOutputs`、「tests」の `matchTests`）が使う。プラグインの異常終了は `Crashed`、形の違う応答は `MalformedOutput`。
5. error が一つでもあれば例外（値は error と warning のすべて）。設定かマニフェストの error（CFG-*、MAN-*）が一つでもあれば `ConfigInvalid`、ただしそれがすべて CFG-16 なら `LanguageMismatch`。それ以外は `CheckFailed`。error がなければ `CheckResult`（warning）。

構文エラーのあるファイルも、読めた宣言は解決・検査に進める。診断の順序は `docs/diagnostics.md`。

### 書き出し

ユースケース `write` の、`check` の後。下の手順の 1〜3 が `collectOutputs`（ファイルを集める）、5〜7 の、書くファイルと消すファイルを決めるところが `outputChanges`、書き出すところが `syncOutputs`。ユースケース `verify`（検査だけ）は、1〜3 を、確かめるためだけに行い（`verifyOutputs`）、`preview`（書き出す予定を見る）は、1〜3 の後、4 で終わる（`plannedChanges`）。出力（単位 U と、U が出力する名前空間 N の組）と、その置き場 Dir(U, N) は `docs/config.md`「単位の対象」「in」。一度の `build` が書き出す出力の集まりを **書き出しの対象**と呼ぶ。書き出すファイルは、どれもトランスパイラが返したもので、hozoc が自分で作るファイルは、出力先ごとの記録だけ。プラグインは、`check` の 4 で、一度、起動してあり、`collectOutputs` は、起動しない（`docs/plugin-protocol.md`「起動」: 一度の実行で、同じプラグインに、同じことを、二度、頼まない）。

0. `hozo.toml` がなければ（設定なし）、`verify` は、集めるものがないので、検査の warning だけを返す（`verifyOutputs`）。`preview` と `write` は、CFG-7 で `ConfigInvalid`（`collectOutputs`）。
1. `namespaces` が空なら、すべての出力が書き出しの対象。空でなければ、引数 n ごとに、名前空間が n と同じか n の子孫である出力が書き出しの対象（その名前空間を出力するすべての単位の分。引数は単位ではなく名前空間を選ぶ）。n は単位の `namespace` に書いた名前である必要はない: n 自身がどの単位の対象でなくても、n の子孫を対象にする単位があればその出力が選ばれ、単位が名指しする名前空間の子孫を n に書けば、その子孫の出力だけが選ばれる。引数 n について、仕様にある名前空間のうち n と n の子孫のどれも、どの単位の対象（`docs/config.md`「単位の対象」）でもなければ `UnknownNamespace`（仕様にない名前を書いたときも、これに当たる）。単位の対象ではあるが `in` のせいで出力でない名前空間だけを選ぶ引数は `UnknownNamespace` ではなく、何も書かずに 5 の後始末だけを行う。
2.書き出しの対象の各出力 (U, N) について、U の `transpilers` の各トランスパイラの答え（`check` の 4 で受け取ったもの）から、`files` を集める（順序は、単位の設定の順、名前空間の IR の順、`transpilers` に書いた順）。`files` の `path` の誤り（PLG-1 / PLG-2）と、テストの対応表の誤り（PLG-4）は `MalformedOutput`（`docs/plugin-protocol.md`「応答の形」「診断」）。`errors` は、ここには、ない（一つでもあれば、`check` が `CheckFailed` で終わっている）。
3. すべての出力のファイルを集めてから、パスの重複を確かめる（CFG-5 → `OutputConflict`）。
4. `verify` は、ここで終わり: `written` も `removed` も空で返す（置き場は、読まない）。`preview` も、ここで終わり: 書く予定のパスを `written` に入れて返す（書かない。置き場は、読まないので、`removed` は、空）。
5.書き出しの対象の各出力の `<Dir(U, N)>/.hozoc-outputs.json`（前回書いたパスの一覧。ルートからの相対パスの JSON の配列、辞書順）を読み、今回書かないパスのファイルを消す。ファイルがなければ何もしない。U の対象の名前空間が `in` のせいで出力でなくなったとき（`docs/config.md`「in」）も、その Dir(U, N) は同じに扱う（前回のファイルを消す）。`hozo.toml` から単位を消した、または `namespace` か `out` を変えた場合、前の出力先はもう分からないので消さない（手で消す）。
   - 記録のパスに従って消すのは、その記録の持ち主の Dir(U, N) の下のファイルだけ。記録に、Dir(U, N) の下でないパス（`..` で外へ出るもの、絶対パス、ルートの中の別のディレクトリ、別の出力の Dir）が書かれていても、そのファイルは消さず、`removed` にも warning にも出さない。hozoc がこの記録に書くのは Dir(U, N) の下のパスだけ（PLG-1 を通ったトランスパイラのファイル）なので、それ以外は hozoc の書いたものではなく、記録が壊れているか手で直されている。壊れた記録で、出力先の外やプロジェクトの外のファイルを消さないための規則。黙って無視するのは、単位を消したときの前の出力先と同じ扱い（hozoc が持ち主だと言えないファイルには触れず、何も言わない）。
   - 消したファイルのディレクトリが空になれば、Dir(U, N) の下の範囲で、そのディレクトリも消す（Go の基底の内側のフォルダ `inner` が要らなくなったとき。`docs/plugin-go.md`「内側のフォルダと入口」）。
6. ファイルを書く。内容が同じなら書き直さない（`written` には入れる）。ディレクトリは作る。
7. `.hozoc-outputs.json` を書く。書くものが一つもない出力（どのトランスパイラもファイルを返さなかった）には作らず、前回のものがあれば消す。

-書き出しの対象を絞っても、書き出す出力の内容は、すべてを対象にしたときと同じになる。どのトランスパイラも、一つの出力の答えを、その出力の `View` だけから作り（`docs/plugin-protocol.md`「起動」）、ほかの出力の答えを要らないため。
-書き出しの対象を絞っても、プラグインには、すべての出力を頼む（`check` の 4 が、すべての出力の診断を要る）。絞るのは、集めて、書き出すファイルだけ。対象でない出力の、`path` の誤り（PLG-1 / PLG-2 / PLG-4）と、衝突（CFG-5）は、見ない。
- `BuildResult.warnings` は `check` の warning（トランスパイラの warning を含む）。
- 書く順序はパスの辞書順。途中で失敗したら、そこまでに書いたものは残る（原子性は持たない。`Crashed` は 2 で起きるので、書き出しには入っていない）。

この節の細部は、決定 D752 が決めた。

## tests

`hozoc tests --coverage <パス>` は、求めるテスト（テストを受け取る口が、ケースを受け取って走らせるテスト。`docs/plugin-protocol.md`「テストの対応表」）が、走ったかを、カバレッジのファイルと突き合わせて確かめる。hozoc は、テストを走らせない: 利用者（か CI）が、プロジェクトのやり方で、カバレッジを取りながらテストを走らせ、そのファイルの場所を渡す。分かるのは「走ったか」までで、「通ったか」は、テストの実行の終了コードが言う（`docs/ide-chain.md`「テストの規約」の「担保すること」）。

ユースケース `confirmTests`: `check` の後、`matchTests`。

1. すべての出力について、トランスパイラの答えから、ファイルを集める（「書き出し」の 2 と 3 と同じ。`OutputConflict`、`MalformedOutput` も同じ）。ファイルは、書き出さない。`hozo.toml` がなければ、CFG-7 で `ConfigInvalid`（「手で書くもの」の最後の項目）。
2. 集めたファイルから、テストの対応表を取り出す。その並び（出力の順、トランスパイラの順、対応表の中の順）が、**求めるテスト**。一つもなければ、ここで終わり（`ran` は空）。
3. 対応表と、対応表が指すファイルが、置き場にあり、集めたものと同じ内容であることを確かめる。ないか、違えば、ファイルごとに TESTS-2（置き場の出力が、今の仕様からのものでない。行が、合わない）。一つでもあれば `CoverageUnusable`。
4. `coverage` のファイルを読み（相対パスの基準は、作業ディレクトリ）、対応表を持つ単位の基底のプラグインごとに、`direction: "coverage"` を送る（相手と、渡すものは `docs/plugin-protocol.md`「coverage」）。次のどれかなら、TESTS-3 で、一つでもあれば `CoverageUnusable`: ファイルを読めない（一つ）。単位の基底のプラグインが `[coverage] enabled` でない（プラグインごとに一つ）。プラグインの応答が `recognized: false`（プラグインごとに一つ）。
5. 求めるテストごとに、応答の、そのファイルの `lines` のどれかが、テストの `lines` の範囲と、一行でも重なれば、**走った**。重ならなければ、走っていない。応答に、そのファイルがなければ、カバレッジが、そのファイルを測っていない。どちらも、テストごとに TESTS-1 で、一つでもあれば `TestsNotRun`。
6. すべて走っていれば、`TestsResult`（`warnings` は、`check` の warning（トランスパイラの warning を含む）。`ran` は、求めるテストの `<トランスパイラの名前>: <テストの名前>`）。

診断:

| ID | 重さ | 意味 |
|---|---|---|
| TESTS-1 | error | 求めるテストが、走ったと確かめられない。二つの場合がある。（1）カバレッジが、そのテストの行を、実行済みと言っていない: テストを受け取る口を呼ぶテストを、書いていない。絞って走らせた。カバレッジを取るときに、測る範囲に、口のパッケージを入れていない（Go なら `-coverpkg`。`docs/plugin-go.md`「カバレッジを読む」）。（2）カバレッジが、そのファイルを測っていない。位置は、対応表の `file` と、`lines` の始まりの行 |
| TESTS-2 | error | 対応表か、対応表が指すファイルが、置き場にない、または、今の仕様から集めたものと、内容が違う（`hozoc build` をやり直して、テストを走らせ直す）。位置は、そのファイル |
| TESTS-3 | error | カバレッジを読めない: `coverage` のファイルを開けない、UTF-8 の文字列でない、単位の基底のプラグインが `[coverage] enabled` でない、プラグインが、その形を読めなかった（`recognized: false`）。位置は、`coverage` のファイル |

- メッセージは、テストを `<トランスパイラの名前>: <テストの名前>` で名指す（`go-http: server.login/OK`）。同じ名前のテストが、二つのトランスパイラの対応表にあってよい（葉の op に経路を付ければ、単体テストと、インテグレーションテストが、同じ `<op の FQN>/<結果>` になる）。
- **確かめられないこと**: カバレッジを取った後に、`hozoc build` をやり直して、同じ内容でない出力を書いた場合、3 は、今の置き場を見るので、通る。カバレッジは、前の出力の行を言っているので、突き合わせは、当てにならない。カバレッジと出力の、新しさは、比べない（ファイルの時刻は、チェックアウトで変わる）。CI では、`emit` の後の一致（`docs/testing.md`「生成物の一致」）、テスト、`hozoc tests` を、この順に、同じ作業ツリーで走らせる。
- 一度に渡せるカバレッジのファイルは、一つ（言語の違う単位を持つプロジェクトの扱いは、決めていない: `docs/open.md`「語彙とプラグイン」）。

## lsp

`docs/lsp.md`。`config` が与えられなければ、クライアントの `rootUri` から探す。

## 手で書くもの

`spec/cli.hozo` から生成するのは、二つの名前空間の出力。ユースケースの `hozo.compiler` は、`internal/gen/hozo/compiler`（基底 `go` の出力。パッケージ名は、名前空間 `hozo.compiler` の最後の要素の `compiler`。`hozo.compiler` は内部の op を持つので、入口のパッケージと、内側の `internal/gen/hozo/compiler/inner` に分かれる。`docs/plugin-go.md`「内側のフォルダと入口」。型は、`internal/gen/hozo/compiler/types` にある: 同「型のパッケージ」）、`internal/gen/hozo/compiler/compilertest`（基底 `go` の、テストのパッケージ: 七つの葉の op の、単体テストの口と、その、テストの対応表。`docs/plugin-go.md`「単体テストの口」。この口を満たすテストは、まだ書いていない: `docs/open.md`「サンプルと、hozoc 自身の仕様」）、コマンドの行の `hozo.cli` は、`internal/gen/hozo/cli`（基底 `go` の出力。パッケージ名は `cli`。内部の op `writeOrPreview` を持つので、ここも、入口のパッケージと、内側の `internal/gen/hozo/cli/inner` に分かれる。型は `internal/gen/hozo/cli/types` で、`go-valid` の `validate.go` も、そこ: 入力の `format` が、制約を持つ）、`internal/gen/hozo/cli/clitest`（にせもの。`docs/plugin-go.md`「テストのにせもの」。分類 `Flag` は、型で決まるので、述語も、分類の判定の口も、ない）、`internal/gen/hozo/cli/cli`（`go-cli`。パッケージ名は、これも `cli`: 手で書くコードは、名前空間のパッケージの方を、`hozocli` という名前で import する）。変えるときは `spec/cli.hozo` を変えて `make generate`（`docs/packages.md`「自己生成の手順」）。生成物はコミットし、`tests/generated_test.go` と CI が遅れていないことを確かめる（`docs/testing.md`「生成物の一致」）。

基底が出す、実装を受け取る口（`docs/plugin-go.md`「op の置き場」「New」）と、それを満たすもの:

| 口 | メソッド | 実装（`internal/app`） |
|---|---|---|
| `compiler.Leaves` | `Check`、`CollectOutputs`、`VerifyOutputs`、`PlannedChanges`、`MatchTests`、`Serve`、`RunEmit`（葉の op。状態に触れない。七つとも、最初の引数に `ctx` を取り、最後の戻り値に `error` を返す: `docs/plugin-go.md`「op の形」） | `*app.App` |
| `compiler.SourcesState`（`sources: list<SourceFile>`） | `IsEmpty`、`Get` | `app.App` の `Sources()` が返す値: `sources` の glob に合うファイル |
| `compiler.ConfigState`（`config: nullable<SourceFile>`） | `IsEmpty`、`Get` | 同 `Config()`: `hozo.toml`（設定なしなら、ない） |
| `compiler.OutputsState`（`outputs: list<OutputFile>`） | `IsEmpty`、`Get`、`OutputChanges`（`read outputChanges` の宛先が、この状態） | 同 `Outputs()`: 出力先ごとの記録（`.hozoc-outputs.json`）に載っているファイル。`OutputChanges` は、書き出す予定を、その記録とディスクと比べる |
| `compiler.OutputsSyncState`（述語で決まる状態の分類 `OutputsSync`） | 述語 `UpToDate` と `OutOfDate`、命令 `SyncOutputs`（効果 `syncOutputs` の宛先が、この分類の遷移） | 同 `OutputsSync()`。`OutOfDate` は `UpToDate` の否定で、同じ比べ方を使う |
| `hozocli.HozoCompilerOps`（コマンドの行の配線が呼ぶ、ユースケースの op） | `Verify`、`Preview`、`Write`、`ConfirmTests`、`Serve` | `compiler.New` の戻り値（`compiler.Ops`）を、そのまま渡す |

`compiler.New(leaves, sources, config, outputs, outputsSync)` の引数は、この順（`Leaves`、状態の単位を宣言順に）。コマンドの行は、`hozocli.New(hozoCompiler)`（別の名前空間の op。フラグで道を分ける所は、分類 `Flag` の `match` で、生成した本体が持つ: 人が書く述語は、ない）。一つでも渡し忘れれば、コンパイルできない。口はどれもインターフェースなので、メソッドが一つでも足りない実装（`spec/cli.hozo` に葉の op、`read`、効果を足して、`internal/app` に書いていない）も、渡すところでコンパイルできない。配線は、生成した本体が持つ。ユースケース（`internal/gen/hozo/compiler/inner/wiring.go`）: `verify` は、`Check`、`VerifyOutputs`。`preview` は、`Check`、`CollectOutputs`、`PlannedChanges`。`write` は、`Check`、`CollectOutputs`、`OutputChanges`、`SyncOutputs`。`confirmTests` は、`Check`、`MatchTests`。コマンドの行（`internal/gen/hozo/cli/inner/wiring.go`）: `build` は、`in.NoEmit` が真なら `Verify`、偽なら、内部の `WriteOrPreview`。`WriteOrPreview` は、`in.DryRun` が真なら `Preview`、偽なら `Write`（真偽の値の分類の `match` は、Go の `if` になる: `docs/plugin-go.md`「分類」）。`tests` は `ConfirmTests`、`lsp` は `Serve`。

- `cmd/hozoc/main.go`: `internal/app` の `App` から、上の表の五つを取り出して `compiler.New` に渡し（`app.App` の `Usecases()`）、その戻り値を、`hozocli.New` に渡し（`app.App` の `Ops()`）、その戻り値を、`Presenter` と一緒に `cli.Main`（`docs/plugin-go-cli.md`「出力」）に渡す。その前に、古い名前のコマンド（`check`、`emit`。「build」の「古い名前」）なら、`internal/app` の `Renamed` が返す文を、標準エラーに書いて、64 で終わる（`spec/cli.hozo` に書けない、手で書く一か所: 仕様にないコマンドの名前は、`go-cli` の `Main` には、知らないコマンドとしか、見えない）。最初の引数の `ctx` は `context.Background()`。プロセスの環境を読むのはここだけ: 環境変数は `os.Getenv` を `cli.Main` に渡し（`spec/cli.hozo` の `@cli.env` のフィールドに入る）、ホームディレクトリ（`os.UserHomeDir()`。プラグインの探索の `<ホーム>/.hozo/plugins`）は `app.App` の `Home` に入れる。それだけ。
- `internal/app`: 葉の op と、状態の単位の実装。`Check` は `internal/project.LoadWith`（プラグインの起動も、ここ: `internal/plugin.Runner`）、`CollectOutputs` は「書き出し」の手順の 1〜3（`Check` が受け取った答えを使い、プラグインは、起動しない。ファイルには触れない）、`VerifyOutputs` は、同じ 1〜3 を、確かめるためだけに行って、warning だけの結果を返す（設定なしなら、集めずに、検査の warning だけ）。`PlannedChanges` は、同 4 の、書く予定のパスを結果にするところ（ファイルには触れない）。`RunEmit` は下の項目、`Serve` は `internal/lsp.Serve`（`docs/packages.md`）。`MatchTests` は「tests」の手順の 1〜6: ファイルを集めるところは `CollectOutputs` と同じ関数、対応表の形と、突き合わせは `internal/testmap`、プラグインの起動は `internal/plugin.Runner`（置き場のファイルと、カバレッジのファイルを、読むだけ。状態 `outputs` は、境界に書いていない: `spec/cli.hozo` の `matchTests` の説明）。状態に触れるのは、状態の単位のメソッドだけ: `Outputs()` の `OutputChanges` は、同 4〜7 の、書くファイルと消すファイルを決めるところ（読むだけで、何も変えない。`written` は書く予定のすべて、`removed` は、記録にあって今回は書かないパスのうち、ディスクにあるもの）、`OutputsSync()` の `SyncOutputs` は、書き出すところ（消す、書く、記録を書く）。どちらも、呼ぶのは、ユースケース `write` だけ。`OutputsSync` の述語 `UpToDate` は、生成し直した内容とディスクの内容を比べて実装する。状態の単位の、形で決まる問い合わせ（`IsEmpty`、`Get`）は、作業ディレクトリから探した設定で読む（配線は呼ばない。テストが、効果の結果の判定に使う）。
- `Check` は、受け取った `ctx` で、プラグインを起動する: `ctx` が中断されていれば、次のプラグインを起動せずに、その `error` を返す。`cmd/hozoc` が渡す `ctx` は `context.Background()` で、中断されることはない。
- `runEmit` は、どの配線からも呼ばれない内部の op だが、基底は単位の対象の op をすべて出す（`docs/plugin-go.md`「出力の対象」）ので、`compiler.Leaves` のメソッド `RunEmit` になり、実装が要る。実装は、`plugin` を起動し、`req` を JSON にして送り、応答を `hozoc.EmitResponse` に読んで返す。`hozoc.Spec` と `hozoc.OutputView` は中身のない model なので、この入口から送る要求の `spec` と、出力の `view` は空のオブジェクトになる（プラグインとの境界の記述としての op で、hozoc 自身はこの入口を呼ばない）。`Check`（`internal/project`）は、同じ起動を、本物の仕様（`ir.Spec`、`ir.OutputView`）を持つ要求で、`internal/plugin.Runner` を直に使って行う。
- 境界の型は生成したもの（`compiler.CheckInput`、`compiler.Diagnostic`。中の `internal/diag.Diagnostic` は行と列に直して写す）で、例外は生成した exception の型を**値で**返す。`Crashed` / `MalformedOutput` も、`hozoc` の例外の型（`hozo.compiler.Crashed{}`）。`spec/cli.hozo` の model は境界だけ（`docs/requirements.md`「自己展開」）なので、op をまたいで要る中身は `internal/app` の中に持つ: `check` が解析した `Project`（プラグインから受け取った答えを含む）と、書き出す予定の置き場（ルートと、出力ごとの Dir(U, N)。境界の `EmitPlan` は、warning と、ファイルだけ）。`OutputChanges`、`SyncOutputs`、`PlannedChanges` に渡す予定が、直前の `CollectOutputs` の返したものでなければ、誤り（宣言にない誤り。70）。
- **診断を運ぶ六つの例外**（`CheckFailed`、`ConfigInvalid`、`LanguageMismatch`、`OutputConflict`、`TestsNotRun`、`CoverageUnusable`。生成した構造体は、引数 `diagnostics` のフィールドを一つ持つ。`docs/plugin-go.md`「exception」）を作るのと、その診断を取り出すのは、`internal/app/exceptions.go` の関数だけ。引数の名前を書くのは、そのファイルの一か所（六つの構造体と同じ形の型 `carried` と、そのフィールドを読む一行）で、`spec/cli.hozo` の引数の名前を変えたら、そこを合わせる（合っていなければ、コンパイルできない）。ほかのコードとテストは、名前を書かない。
- 内部の op（`check`、`collectOutputs`、`verifyOutputs`、`plannedChanges`、`matchTests`、`runEmit`）のテストは、内側のパッケージ `internal/gen/hozo/compiler/inner` の `New` の戻り値から呼ぶ。`build` が、フラグの組から、入るユースケースを分けること（「build」の表）は、生成した本体に、呼ばれたことを覚える、ユースケースの代わりを渡して、フラグの四つの組で確かめる（`internal/app/buildflags_test.go` の `TestBuildFlags`）。
- `Presenter`（`cli.Presenter` の実装）: コマンドの入力（`hozo.cli` の `BuildInput` / `TestsInput`）の `format` に応じて text か JSON（表示は、コマンドの行の側の仕事なので、フラグ `format` と `noEmit` を、ここで、コマンドの入力から読む。ユースケースの結果は、検査だけでも、`BuildResult`）（`docs/diagnostics.md`「表示」。診断の text と json の形は `internal/diag` が持ち、`Presenter` は `spec/cli.hozo` の model `Diagnostic` をその形に写して呼ぶ）。結果は標準出力、例外は標準エラー（`docs/plugin-go-cli.md`「実行と表示」）。text の `CheckResult` は診断の一覧と件数、`EmitResult` は warning の一覧（あれば）に続けて `written: <path>` / `removed: <path>` を一行ずつ、`TestsResult` は warning の一覧（あれば）に続けて `ran: <トランスパイラの名前>: <テストの名前>` を一行ずつと、最後に件数（`3 tests ran`）、診断を運ぶ例外はその一覧と件数、それ以外の例外は `hozoc: <Failure.Err のメッセージ>`。json の `CheckResult` は `{"warnings": [...]}`、`EmitResult` は `{"warnings": [...], "written": [...], "removed": [...]}`、`TestsResult` は `{"warnings": [...], "ran": [...]}`、例外は `{"error": "<FQN>", "value": {...}}` で、診断を運ぶ例外の `value` は `docs/diagnostics.md`「json」の形（`{"diagnostics", "errors", "warnings"}`）、それ以外は `cli.JSONPresenter` と同じ。診断の要素は `docs/diagnostics.md`「json」と同じ形。診断を運ぶ例外の `value` を書くのは `Presenter` で、キー `diagnostics` は `docs/diagnostics.md`「json」のもの（`internal/diag` の `Report`）: 例外の引数の名前から作るのではない（`cli.JSONPresenter` なら、引数の名前をキーにしたオブジェクトで、件数の `errors` / `warnings` を持たない。`docs/plugin-go-cli.md`「実行と表示」）。引数の名前が同じ `diagnostics` なので、二つの形は、キー `diagnostics` で重なる。
- `hozo.toml` がないとき、`check` と、ユースケース `verify` は、設定なしで検査し、`preview`、`write`、`confirmTests` は CFG-7 を一つ作って `ConfigInvalid` にする（`docs/config.md`「置き場と探し方」）。

この節の細部は、決定 D649、D742、D753、D754、D770 が決めた。
