# Go のパッケージ

## この文書が決めること

モジュール `github.com/mew-ton/hozo` のディレクトリとパッケージの切り方、各パッケージの責務と主な API、依存の向き。リポジトリの構成の持ち主はこの文書。

## ディレクトリ

```
go.mod                 module github.com/mew-ton/hozo
hozo.toml, cli.hozo    hozoc 自身の仕様と設定（docs/cli.md）
Makefile               build / generate（「自己生成の手順」）/ vet / test
cmd/hozoc/             CLI の main。app.App の Ops()（実装を hozoc.New に渡したもの）と app.Presenter を、生成した cli.Main に渡すだけ
ir/                    IR の型（公開）
plugin/                Go 向けプラグイン SDK（公開）。testdata/ に、二つの SDK のテストが読む表
plugin/golang/         Go の基底の形の規則: 置き場、名前、型、op の形（公開）
plugin/govalid/        go-valid の形の規則: どの型が Validate を持つか（公開）
runtime/               生成した Go のコードが import する型（公開）
vocab/                 標準の語彙の .hozo と、それを embed する公開パッケージ vocab（go:embed はパッケージのディレクトリの外を参照できないため）
internal/source/       ファイルと位置
internal/diag/         診断
internal/syntax/       字句・構文解析（token/、ast/ は字句と構文木）
internal/sema/         解決、型、正規化
internal/check/        検査
internal/config/       hozo.toml
internal/vocab/        埋め込んだ標準の語彙
internal/plugin/       プラグインの探索と起動
internal/project/      一度の解析のすべて
internal/app/          hozoc の葉の op の実装と表示
internal/lsp/          言語サーバー（jsonrpc/ は JSON-RPC、protocol/ は LSP の型のうち使うもの）
internal/gen/hozoc/    cli.hozo からの生成物: 入口のパッケージ（go）。inner/ は内側のパッケージ（go と go-valid）、cli/ は go-cli の出力
internal/gen/proc/     cli.hozo からの生成物: 語彙 proc の例外の型（go）
internal/tools/        生成の道具: irschema/ と tmgrammar/（go generate）、selfgen/（make generate）
plugins/<name>/        プラグイン。一つのディレクトリが一つの実行ファイルと hozo-plugin.toml（提供するトランスパイラの一覧を持つ）。Go なら main パッケージ、TS なら npm パッケージ
plugins/go/            Go の基底のプラグイン（main。中身は internal/gobase/）
plugins/go-valid/      go-valid プラグイン（main。中身は internal/govalid/）
plugins/go-sqlite/     go-sqlite プラグイン（main。中身は internal/gosqlite/）
plugins/go-http/       go-http プラグイン（main。中身は internal/gohttp/）
plugins/go-cli/        go-cli プラグイン（main。中身は internal/gocli/）と、その語彙 cli.hozo
sdk/ts/                TS 向けプラグイン SDK
schema/                IR の JSON スキーマ（go generate の置き場のパッケージ schema）
editors/vscode/        VS Code 拡張（syntaxes/ は TextMate の文法と、go generate の置き場のパッケージ syntaxes）
tests/                 リポジトリをまたぐテスト（パッケージ tests。テストのファイルだけ）
tests/rules/, tests/corpus/   テストの入力（docs/testing.md）
tests/corpus/view/plugins/view-dump/   テスト用のプラグイン（main パッケージ。docs/testing.md「コーパス」）
tests/samples/<名前>/  サンプル（仕様と、手で書いた完成したコードの組。tests/samples/README.md）。自分の go.mod を持つ別のモジュールで、このモジュールのパッケージではない
                       サンプルが自分で持つプラグイン（tests/samples/<名前>/plugins/<name>/。マニフェスト、語彙、main）も、サンプルのモジュールの中
docs/                  設計文書
```

## 依存の向き

Go のパッケージごとの、このモジュールの中で import しているパッケージ（テストでないファイル）。この表が持ち主で、実際の import と同じであることを `tests/deps_test.go` が確かめる。import を足すときは、足してよい向きかを下の規則で確かめてから、この表にも足す。

| パッケージ | import する |
|---|---|
| `cmd/hozoc` | `internal/app`、`internal/gen/hozoc/cli` |
| `internal/app` | `internal/config`、`internal/diag`、`internal/gen/hozoc`、`internal/gen/hozoc/cli`、`internal/gen/proc`、`internal/lsp`、`internal/plugin`、`internal/project`、`internal/source`、`ir`、`plugin` |
| `internal/lsp` | `internal/diag`、`internal/lsp/jsonrpc`、`internal/lsp/protocol`、`internal/project`、`internal/sema`、`internal/source`、`internal/syntax`、`internal/syntax/ast`、`internal/vocab`、`ir` |
| `internal/lsp/jsonrpc` | — |
| `internal/lsp/protocol` | — |
| `internal/project` | `internal/check`、`internal/config`、`internal/diag`、`internal/plugin`、`internal/sema`、`internal/source`、`internal/syntax`、`internal/syntax/ast`、`internal/vocab`、`ir`、`plugin` |
| `internal/plugin` | `internal/config`、`internal/diag`、`ir`、`plugin` |
| `internal/check` | `internal/config`、`internal/diag`、`internal/source`、`ir` |
| `internal/sema` | `internal/diag`、`internal/source`、`internal/syntax/ast`、`ir` |
| `internal/syntax` | `internal/diag`、`internal/source`、`internal/syntax/ast`、`internal/syntax/token` |
| `internal/syntax/ast` | `internal/source` |
| `internal/syntax/token` | `internal/source` |
| `internal/config` | `internal/diag` |
| `internal/diag` | `internal/source` |
| `internal/source` | — |
| `internal/vocab` | `vocab` |
| `internal/gen/hozoc` | `internal/gen/hozoc/inner` |
| `internal/gen/hozoc/inner` | `runtime` |
| `internal/gen/hozoc/cli` | `internal/gen/hozoc`、`internal/gen/proc`、`runtime` |
| `internal/gen/proc` | `runtime` |
| `internal/tools/irschema` | `ir`、`plugin` |
| `internal/tools/tmgrammar` | `internal/syntax/token` |
| `internal/tools/selfgen` | — |
| `ir` | — |
| `plugin` | `ir` |
| `plugin/golang` | `ir` |
| `plugin/govalid` | `ir` |
| `runtime` | — |
| `vocab` | — |
| `plugins/go` | `plugin`、`plugins/go/internal/gobase` |
| `plugins/go/internal/gobase` | `ir`、`plugin`、`plugin/golang` |
| `plugins/go-valid` | `plugin`、`plugins/go-valid/internal/govalid` |
| `plugins/go-valid/internal/govalid` | `ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`runtime` |
| `plugins/go-sqlite` | `plugin`、`plugins/go-sqlite/internal/gosqlite` |
| `plugins/go-sqlite/internal/gosqlite` | `ir`、`plugin`、`plugin/golang` |
| `plugins/go-http` | `plugin`、`plugins/go-http/internal/gohttp` |
| `plugins/go-http/internal/gohttp` | `ir`、`plugin`、`plugin/golang`、`plugin/govalid` |
| `plugins/go-cli` | `plugin`、`plugins/go-cli/internal/gocli` |
| `plugins/go-cli/internal/gocli` | `ir`、`plugin`、`plugin/golang`、`plugin/govalid` |
| `tests/corpus/view/plugins/view-dump` | `plugin` |
| `schema` | — |
| `editors/vscode/syntaxes` | — |
| `tests` | — |

- `internal/` は `cmd/hozoc` と `internal/` の中からだけ使う。例外は、リポジトリをまたぐテストのパッケージ `tests` だけ（テストのファイルから `internal/app`、`internal/project` などを import する。`tests` にテストでないファイルはない）。
- プラグインが使えるのは `ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`runtime`（と、自分のディレクトリの下のパッケージ）。テスト用のプラグインも同じ。別のプラグインのパッケージは import しない: 相乗りが基底やほかの相乗りの形を知るのは、公開の SDK（`plugin/golang`、`plugin/govalid`）を通してだけ（`docs/plugin-protocol.md`「相乗りが基底の形を知る方法」）。
- 公開のパッケージ（`ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`runtime`、`vocab`）は `internal/` に依存しない。
- **hozoc 本体（`cmd/hozoc` と `internal/` のパッケージ。生成物 `internal/gen/` を除く）は、特定の言語の出力のコードを持たない。** `plugin/golang`、`plugin/govalid`、`plugins/` を import しない（言語を知るのはプラグインだけ。`docs/plugin-protocol.md`「この文書が決めること」）。`runtime` を import するのは、生成物だけ。
- 解析（`internal/project`）は、プラグインの `emit` を起動しない。応答のファイルを集めて書き出すのは `internal/app`。言語サーバーは `internal/project` までで、プラグインの起動（`emit`）を行わない。
- `internal/sema` は設定（`internal/config`）に依存しない。設定に関わる検査は `internal/config`、`internal/check`、`internal/project` が行う（「`internal/config`」）。
- `internal/tools/selfgen` はこのモジュールのどのパッケージも import しない（生成物が壊れていても動くため。「自己生成の手順」）。
- 外部のモジュールは `github.com/BurntSushi/toml` だけ（`internal/config` と `internal/plugin` が使う）。依存を最小にするため、言語サーバーの JSON-RPC も自前で書く。
- `tests/samples/` のサンプルは別のモジュール（自分の `go.mod`）なので、この表にも上の規則にも入らない。サンプルだけが使う依存は、サンプルの `go.mod` に置く（`tests/samples/README.md`「置き方」）。サンプルが自分で持つプラグインも、プラグインの規則（使えるのは `ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`runtime`）に従う。

## パッケージ

### `ir`（公開）

`docs/ir.md` の型。`encoding/json` のタグはこの文書のフィールド名。`Program` / `View` と、`Type` などの判別付きユニオンは `kind` で分ける（Go では interface と `UnmarshalJSON`）。

```go
func (p *Program) Lookup(fqn string) Decl          // 宣言を引く。なければ nil
func (p *Program) Namespace(fqn string) *Namespace
func (v *View) Signature(op string) *ViewedSignature
func (v *View) InUnit(fqn string) bool             // viewer の名前空間の宣言で、単位の対象か（span.file が View.files にある。docs/config.md「in」）
```

TS の SDK（`sdk/ts`）は同じ引き方を関数で持つ: `lookup(program, fqn)`、`namespace(program, fqn)`、`signature(view, op)`、`inUnit(view, fqn)`（なければ `undefined`）。

### `plugin`（公開）

プラグインの SDK。JSON の形は `docs/plugin-protocol.md`「要求と応答」で、型はそれを写す。

```go
type Handler interface {
    Emit(req *EmitRequest) (*EmitResponse, error)
    Check(req *CheckRequest) (*CheckResponse, error)
    Import(req *ImportRequest) (*ImportResponse, error)
}
type EmitRequest struct { Protocol string; Direction string; Transpiler string; View *ir.View }   // Transpiler は起動されたトランスパイラの名前
type EmitResponse struct { Files []File; Warnings []Warning; Errors []Diagnostic }
func OpsWith(view *ir.View, decorator string) []string             // view.viewer の op のうち、単位の対象（View.InUnit）で、そのデコレータ（FQN。via も見る）が付いたものの FQN。相乗りが自分の対象を探す道具
func Options(req *EmitRequest, into any) ([]string, error)         // req.View.Config.Options[req.Transpiler] を into（構造体へのポインタ。フィールドは json タグで結ぶ）に読む。
                                                                   // 書かれていなければ into はそのまま。戻り値は、into にないキー（辞書順）。型の違う値は error。
                                                                   // 知らないキーと error を診断にするのは呼ぶ側（docs/config.md「オプション」）
const Version = "0.0.0-dev"                                         // このリポジトリのプラグインの版。生成したファイルのヘッダに書く（docs/plugin-go.md「ファイル」）
// CheckRequest / CheckResponse / ImportRequest / ImportResponse / File / Warning / Diagnostic も同じ形

// Main は標準入力の要求を読み、protocol を確かめ、h を呼び、標準出力に応答を書く。終了コードを返す
// （docs/plugin-protocol.md「要求と応答」「起動」。文字列でない protocol も「合わない」で code "protocol"。TS の run も同じ）。
func Main(h Handler) int
func Run(h Handler, in io.Reader, out, errOut io.Writer) int   // Main の中身。テスト用
// 実装しない方向と、Emit で知らない Transpiler は ErrUnsupported を返す。Main は docs/plugin-protocol.md「起動」の code "unsupported" で終わる
var ErrUnsupported error
```

`Select`、`Wrap`、`EmitResponse.Select` はなくした（`docs/plugin-protocol.md`「選択をなくしたこと」）。TS の SDK（`sdk/ts`）も同じ: `select` の型をなくし、`options(request)` を足す。

### `plugin/golang`（公開）

`docs/plugin-go.md` の形の規則の写し。基底 `go` と、それに相乗りする Go のトランスパイラが共有する: 相乗りは、基底の出力を見ずに、ここの関数で基底の置き場と名前を求める（`docs/plugin-protocol.md`「相乗りが基底の形を知る方法」）。規則の持ち主は `docs/plugin-go.md` で、このパッケージは、どの見出しを写したかをコメントに書く。公開の関数はここに並べたものがすべて。

```go
// 名前（docs/plugin-go.md「名前」）
func TypeName(fqn, namespace string) string       // namespace は宣言した名前空間。合成 model（`.`）の連結を含む
func FieldName(name string) string
func MethodName(opName string) string             // export の有無によらず、先頭を大文字に
func LocalName(name string, reserved map[string]bool) string   // 引数名・束縛名。Go の予約語か事前宣言の識別子か reserved にあれば末尾に _
func EffectConst(name string) string
func CaseConst(classify, caseName string) string
func StateUnitName(stateOrClassify string) string // <状態>State
func DepName(namespace string) (typeName, param string)   // 別の名前空間の口: StoreOps と store
func SeamMethod(opName, decorator string) string  // 継ぎ目のメソッド: login と rate.limited から LoginRateLimited（同「名前」「継ぎ目」）
func BodyName(opName string) string               // 基底が出す中身のメソッド: loginBody
func Capitalize(s string) string                  // 先頭の一文字を大文字に
func IsKeyword(name string) bool                  // Go の予約語か
func IsPredeclared(name string) bool              // Go の事前宣言の識別子（error、string、nil、len など）か
func PackageName(namespace string) string

// 型（同「型」）
func GoType(t ir.Type, imports *Imports) string   // 型の文字列。必要な import を imports に足す
func IsNillable(t ir.Type, p *ir.Program) bool    // Go の零値 nil が「なし」を表せる型か（array、record、union）。optional(T) を *T にしない型
func Zero(t ir.Type, imports *Imports) string     // 零値の式（同「呼び出し」）
func TypeDisplay(t ir.Type) string                // コメントの中の型（同「コメント」の「型の表示」）

// 置き場（同「配置」）
func ImportPath(root, dir string) (string, error) // go.mod を探す。見つからなければ ErrNoGoMod（GO-1）
var ErrNoGoMod error
type Layout struct {                              // 一つの出力 (U, N) の置き場
    Namespace string
    Dir       string                              // Dir(U, N)。入口のパッケージのディレクトリ
    EntryPath, EntryName string                   // 入口のパッケージの import パスと名前
    HasInner  bool                                // 内側のフォルダを持つか（N の op に内部の op がある）
    DefDir, DefPath, DefName string               // 定義のパッケージ（HasInner なら <Dir>/inner と "inner"、そうでなければ入口と同じ）
}
func LayoutOf(view *ir.View, root string) (*Layout, error)                  // この View の出力の置き場
func HomeOf(view *ir.View, namespace, root string) (*Layout, bool, error)   // view の単位から見た、別の名前空間の型の置き場（docs/config.md「型の置き場」）。なければ false
type Imports struct{ … }                          // 一つのファイルの import。名前の決め方は同「パッケージと import パス」
func NewImports(view *ir.View, layout *Layout, root string, inDef bool) *Imports   // inDef は、書くファイルが定義のパッケージのものか（入口のものなら、N の型は入口の名前で修飾なし）
func (*Imports) Reserve(names ...string)              // このファイルの import の名前に使わせない名前を足す（ファイルを書くトランスパイラが、関数の本体の中で決まった名前で使う識別子。
                                                      // 基底の o、ctx、err、m）。基底の非公開の名前 ops と ptr は、Reserve しなくても、どのファイルでも使わせない
func (*Imports) Add(pkgPath, pkgName string) string   // import に足し、修飾に使う名前を返す
func (*Imports) Name(pkgPath string) string           // 修飾に使う名前。足していなければ ""
func (*Imports) Names() []string                      // 修飾に使う名前のすべて（辞書順）。局所の名前が避ける名前
func (*Imports) Paths() []string
func (*Imports) Render() string                       // import のブロック
func (*Imports) Qualifier(namespace string) string    // その名前空間の宣言を、このファイルから書くときの前置き（N なら ""、別の名前空間なら置き場の入口を import して `<名前>.`）
func (*Imports) Canonical() *Imports                  // 同じ import を、文書の決まりの順（名前空間の FQN の辞書順）で足し直したもの。ファイルを一度作って import の集まりを知り、これで作り直すと、名前が決まりどおりになる
func (*Imports) Err() error                           // GoType / Zero / Qualifier が、型の置き場を求められなかった名前空間があれば、その誤り

// op と口の形（同「op の形」「実装を受け取る口」）
type Shape struct {                               // 一つの出力の、op と口のすべて
    Ops      []OpShape                            // N の op（単位の対象の op と、本体のない op として出すもの）。宣言順
    Leaves   []string                             // Leaves のメソッドになる op の FQN
    States   []StateUnit                          // 状態の単位（基底の状態と、述語で決まる状態の分類）。宣言順
    Shared   []string                             // SharedState のメソッドになる op の FQN
    Classify []string                             // New が述語を受け取る、値の分類と op の分類の FQN（辞書順）
    Deps     []Dep                                // 別の名前空間の口（FQN の辞書順）
    Seams    []Seam                               // Seams のメソッド（op の宣言順、その中はデコレータの書かれた順）。空なら Seams も New の引数 seams もない（同「継ぎ目」）
    New      []Param                              // New の引数（名前と Go の型）。ops のフィールドも同じ
}
type OpShape struct {
    FQN, Method string
    Exported   bool
    Place      Place                              // Wiring / Never / StateCommand / StateQuery / SharedQuery / Leaf（同「op の置き場」の 1〜6）
    Unit       string                             // Place が状態の単位なら、その状態（か分類）の FQN
    Ctx, Err   bool                               // ctx を取るか、error を返すか
    Seams      []string                           // この op の継ぎ目のデコレータの FQN（書かれた順。先頭が最も外側）。空なら継ぎ目を持たない
    Body       string                             // 継ぎ目を持ち、基底が中身を出す op（Wiring / Never）の、中身のメソッドの名前。それ以外は ""
}
type StateUnit struct { FQN, TypeName, Param string; Shape string; Queries, Commands []string; Predicates []string }
type Dep struct { Namespace, TypeName, Param string; Ops []string }
type Seam struct { Op, Decorator, Transpiler, Method string }   // 継ぎ目一つ: op の FQN、デコレータの FQN、宣言したトランスパイラ（View.config.seams の値）、Seams のメソッドの名前
type Param struct { Name, Type string }
func ShapeOf(view *ir.View) *Shape
func Signature(op *ir.Op, imports *Imports, reserved map[string]bool) (params, results string)   // メソッドの引数と戻り値のテキスト（ctx と error を含む）
func SeamSignature(op *ir.Op, imports *Imports, reserved map[string]bool) (params, results string)   // 継ぎ目のメソッドの引数（op の引数の後ろに body）と戻り値のテキスト
func SeamBodyParam(op *ir.Op, reserved map[string]bool) string   // 継ぎ目のメソッドの最後の引数の名前: body。op の引数の局所の名前と当たれば、末尾に _ を足したもの

// 定義のパッケージの公開の名前（同「名前」の「名前が当たれば GO-2」）
type PublicName struct {
    Name   string                                 // Go の名前
    Kind   PublicKind                             // PublicType / PublicNewModel / PublicArm / PublicEffect / PublicClassify / PublicCase / PublicClassifyFunc / PublicCases /
                                                  // PublicStateUnit / PublicOps / PublicNew / PublicLeaves / PublicShared / PublicSeams / PublicDep
    Decl   string                                 // その名前を生んだ宣言の FQN（状態の単位なら、状態か状態の分類）。診断の位置に使う。Ops、New、Leaves、SharedState、Seams と、別の名前空間の口なら ""
    Detail string                                 // PublicArm ならアーム名、PublicCase ならケース名（default は "default"）、PublicDep なら名前空間の FQN
    What   string                                 // その名前を生んだものの、診断のメッセージ用の言い方（`model store.Schema`、`インターフェース Ops`）
}
func PublicNames(view *ir.View) []PublicName      // 出力 (U, N) の定義のパッケージの公開の名前のすべて。当たる名前も、そのまま二つ返す
```

- `HomeOf` が返す、別の単位の出力の `HasInner` と `DefDir` / `DefPath` / `DefName` は、その単位の `in` を省いたものとして求めた値（`View` は、ほかの単位の `in` を持たない）。別の名前空間の型を import するのは入口のパッケージなので、import には効かない。
- `Param.Type` は、定義のパッケージの中での書き方。別の名前空間の分類の述語（`<C>Cases`）は修飾なしの名前で、書く側が `Imports.Qualifier` で修飾する。
- 継ぎ目を宣言した相乗りは、`ShapeOf(view).Seams` のうち `Transpiler` が自分の名前のものについて、メソッドを出す。基底も同じ値から `Seams` を出すので、二つは合う。
- これまでの `Resolver`（複製した型をローカルにする判断）はなくした。別の名前空間の型は、いつも置き場の入口のパッケージ。
- 当たる名前（GO-2）と import の循環（GO-3）を見つけるのは基底のプラグイン（`plugins/go/internal/gobase`）で、このパッケージは、当たらない前提で名前を返す。
- `PublicNames` は、`docs/plugin-go.md`「名前」の「名前が当たれば GO-2」が並べる、定義のパッケージの公開の名前の、ただ一つの写し。並びは、N の宣言順（宣言ごとに、型、`New<Model>`、アームを包む型。分類は、列挙の型、ケースの定数、判定関数、述語のインターフェース）、状態の単位（宣言順）、`Ops`、`New`、`Leaves`、`SharedState`、`Seams`、別の名前空間の口（FQN の辞書順）。基底は、この一覧の中で名前が当たれば GO-2 を返す。入口か定義のパッケージに公開の名前を足す相乗り（`go-sqlite` の GOSQL-4、`go-http` の GOHTTP-10、サンプルの `go-ratelimit` の GORATE-2）は、自分の名前がこの一覧の名前と当たるかを確かめる（入口のパッケージは、定義のすべての名前を再公開するので、どちらに足す名前も、当たる相手はこの一覧）。一覧を自分で書き写さない。

### `plugin/govalid`（公開）

`docs/plugin-go-valid.md`「Validate を持つ型」の規則の写し。`go-valid` と、`Validate` を呼ぶ相乗り（`go-http`、`go-cli`）が共有する。

```go
func HasValidate(p *ir.Program, typeFQN string) bool   // その scalar か model が Validate を持つか。仕様だけで決まる
func Standard(decorator string) bool                   // valid の標準の制約（min / max / length / pattern / format）の FQN か
const Transpiler = "go-valid"                          // Validate を出すトランスパイラの名前
func Unlisted(view *ir.View, namespace string) (unit int, ok bool)   // view の単位から見た namespace の型の置き場の単位が、transpilers に go-valid を並べていなければ、
                                                       // その単位が何番目の [[emit]] かと true。並べていれば、または置き場がなければ、0 と false
```

- `Unlisted` は、別の名前空間の型の `Validate` を呼べるかを見る、ただ一つの写し（`docs/plugin-go-valid.md`「出力」）: `Validate` は型の定義のパッケージにしか書けないので、呼ぶコードは、その型の置き場（`docs/config.md`「型の置き場」）の単位が `go-valid` を並べているときにだけコンパイルできる。`go-valid` の GOVALID-3、`go-http` の GOHTTP-11、`go-cli` の CLI-9 が、これで確かめる。置き場は `View.namespaces` と `View.config` だけから求まる。

### `runtime`（公開）

`docs/plugin-go.md`「runtime パッケージ」と `docs/plugin-go-valid.md`「runtime」。`MissingError` はなくした。

### `internal/source`

ファイルの集合と位置。

```go
type FileSet struct{ ... }
func (fs *FileSet) Add(path string, content []byte) FileID
type Span struct { File FileID; Start, End Offset }      // バイトのオフセット
func (fs *FileSet) Position(FileID, Offset) Pos          // 1 始まりの行・列（コードポイント）
func (fs *FileSet) Offset(FileID, Pos) Offset            // Position の逆。行の先頭は Add が作った行の表から引く（行数によらず一定の手間）
func (fs *FileSet) Path(FileID) string
func (fs *FileSet) Content(FileID) []byte
func (fs *FileSet) Len() int
```

IR の `Span`（行・列）は `Position` で作る。構文木はバイトのオフセットで持つ。行・列とオフセットの変換はこのパッケージだけが持つ: IR の `Span` やプラグインの `"file:line:col"` を診断の位置に戻すとき（`internal/check` の `FileSetLocator`、`internal/project` の `Locate`）も `Offset` を使い、自分では数えない。

### `internal/diag`

`docs/diagnostics.md` の `Diagnostic`、`Severity`、並べ替え（`Sort`）、text と JSON の表示。表示の形の持ち主はこのパッケージだけで、ほかのパッケージは診断を自分で文字列にしない。

```go
type Rendered struct { Severity, Code, Message, File string; Start, End *RenderedPos; Related []RenderedRelated; Data map[string]any }
                                                         // 表示する形にした診断（位置は行と列）。JSON にすると docs/diagnostics.md「json」の要素
type Report struct { Diagnostics []Rendered; Errors, Warnings int }   // 同「json」の全体
func Render(diags []Diagnostic, fs *source.FileSet) []Rendered
func NewReport(diags []Rendered) Report
func WriteRenderedText(w io.Writer, diags []Rendered) error           // 同「text」
func WriteText(w io.Writer, diags []Diagnostic, fs *source.FileSet) error   // Render して WriteRenderedText
func WriteJSON(w io.Writer, diags []Diagnostic, fs *source.FileSet) error   // Render して NewReport
func Count(diags []Diagnostic) (errors, warnings int)
```

`internal/app` の `Presenter` は、`cli.hozo` の model `Diagnostic`（位置は既に行と列）を `Rendered` に写して `WriteRenderedText` と `Report` を使う（`docs/cli.md`「手で書くもの」）。

### `internal/syntax/token`、`internal/syntax/ast`、`internal/syntax`

`docs/grammar.md`。

```go
// token
type Kind int                        // 予約語、記号、Ident、リテラル、DocComment、EOF
var Keywords map[string]Kind         // 予約語の一覧（docs/grammar.md「予約語」）。docs/testing.md の一致テストの元
var Contextual = []string{"field", "variant", "post", "put", "delete"}

// ast: 文法の非終端記号ごとの構造体。すべて Span を持つ。DocComment は付いた先の節が持つ
// syntax
func Parse(fs *source.FileSet, f source.FileID) (*ast.File, []diag.Diagnostic)
```

パーサーは再帰下降。誤りからの回復は、宣言の中なら次の `;` か `}` まで、ブロックの中なら次の `;` まで読み飛ばし、`SYNTAX` を一つ出して続ける。読めた宣言は `ast.File` に残す（`docs/requirements.md`「言語サーバー」の前提条件）。

### `internal/sema`

解決（`docs/resolve.md`）、型（`docs/types.md`）、正規化（`docs/ir.md`）。

```go
type Result struct {
    Program *ir.Program
    Refs    []Ref            // トークンの Span → 解決先。定義へジャンプとホバーが使う
    Diags   []diag.Diagnostic
}
type Ref struct { Span source.Span; Target Target }
type Target struct { Kind TargetKind; Decl string; Name string; Span source.Span }
// TargetKind: 宣言、フィールド、ユニオンのアーム、classify のケース、デコレータの Param、束縛、op の引数、名前空間
func Analyze(fs *source.FileSet, files []*ast.File) *Result   // Op.Signature と Call.Bound は空のまま（internal/check が埋める）。設定は見ない
```

解決と型の誤りがあっても `Program` は作る。作れなかった宣言は落とす。解決できなかった参照（型、`throws` / `effects` の要素、デコレータの適用、呼び出しなど）は、IR に残さず、その参照を落とす（診断は RES-* で出ている）。未解決の印を IR に置くことはしていない。

### `internal/check`

`docs/check.md`。

```go
func Compute(p *ir.Program)                                  // Signature と Call.bound を埋める
type Locator func(ir.Span) (source.Span, bool)               // IR の行・列を FileSet のバイトの位置に戻す
func FileSetLocator(fs *source.FileSet) Locator
type Checker struct { Locate Locator }
func (c Checker) Check(p *ir.Program, cfg *config.Config) []diag.Diagnostic   // Compute を呼んでから規則を検査する。Locate が nil なら Span は nil で File だけ
func (c Checker) CheckUnsupported(p *ir.Program, ns string, files []string, unsupported []string) []diag.Diagnostic   // CFG-6。files は単位の in に合ったファイル、unsupported はマニフェストから
type Unit struct { Language, Base string; Seams map[string]string; Files []string }   // 設定の外で決まる単位の情報: 言語と基底の名前、単位の継ぎ目（デコレータの FQN → 宣言したトランスパイラ。どれもマニフェストから）と、in に合ったファイル
func (c Checker) CheckHomes(p *ir.Program, cfg *config.Config, units []Unit) []diag.Diagnostic   // CFG-22
func Views(p *ir.Program, cfg *config.Config, units []Unit) []map[string]*ir.View   // 単位ごと（cfg.Units と同じ順）の、出力する名前空間 → View
```

設定に関わる検査のうち、プログラムが要る CFG-10 は `Check` が（`cfg` を見て）出す。CFG-6 は `CheckUnsupported`、CFG-22 は `CheckHomes`（`docs/config.md`「型の置き場」の表を写す）。

規則ごとに一つのファイル（`throws.go`、`bind.go`、…）。各関数のコメントに規則の ID。

### `internal/config`

`docs/config.md`。

```go
type Config struct { Root, Path string; Hozo HozoTable; Units []Unit }
type Unit struct { Index int; In []string; Namespace string; Transpilers []string; Out string; Options map[string]map[string]any }   // [[emit]] 一つ。Index は何番目か（1 始まり）。In は省略なら nil。Options は [emit.options.<名前>]（なければ空）
func Load(path string) (*Config, []diag.Diagnostic)        // CFG-7 / CFG-12 / CFG-17 / CFG-21 はここで出る。Config は常に返す（既定値で補う。CFG-7 か CFG-12 の単位は Units に入れない）
func Find(startDir string) (string, bool)
func Default(root string) *Config                           // hozo.toml がないとき（Project.NoConfig）
func (c *Config) Covering(namespace string) []int           // 名前空間を対象にする単位（Units の添字、設定の順）。CFG-13（docs/config.md「単位の対象」）
func (c *Config) Dir(unit int, namespace string) string     // Dir(U, N)。ルートからの相対パス
func (c *Config) UnitNamespaces() []string                  // 単位の namespace（重複なし、辞書順）。CFG-10 のため
func CleanRel(p string) string                              // ルートからの相対パスを比べられる形に（/ 区切り、./ や余分な / を除く。ルートは ""）
func Under(a, dir string) bool                              // 相対パス a が dir の下（真に深い）か。CFG-17、PLG-1、.hozoc-outputs.json の判定
```

ルートからの相対パスの正規化と「下にあるか」の判定はこのパッケージだけが持つ（`out`、`plugin_dirs`、glob、書き出すファイルのパス、`.hozoc-outputs.json` のパスのどれも `CleanRel` を通す）。

設定に関わる検査（CFG-*）を出すパッケージは、何が要るかで分かれる:

| 規則 | 出すパッケージ | 要るもの |
|---|---|---|
| CFG-7、CFG-12、CFG-17、CFG-21 | `internal/config`（`Load`） | `hozo.toml` だけ |
| CFG-10 | `internal/check`（`Check`） | プログラム（名前空間） |
| CFG-6 | `internal/check`（`CheckUnsupported`。呼ぶのは `internal/project`） | プログラムと、マニフェストの `unsupported` |
| CFG-22 | `internal/check`（`CheckHomes`。呼ぶのは `internal/project`） | プログラムと、単位ごとの基底と `in` に合ったファイル |
| CFG-14〜CFG-16、CFG-18〜CFG-20、CFG-23 | `internal/project` | マニフェストと、読んだファイル |
| CFG-5 と、設定なしで `emit` したときの CFG-7 | `internal/app` | 書き出すファイルの一覧 |

設定なし（`Project.NoConfig`）のときは、`internal/project` が CFG-* をまとめて除く。

### `internal/vocab`

`docs/vocab.md`。公開パッケージ `vocab` が `embed` した `vocab/*.hozo` を、`Files() []File` と `AddTo(func(path string, content []byte))` で渡す。パスは `hozo:vocab/<名前>.hozo`。プラグインの語彙は `PluginFiles(plugin, dir, rels)` / `AddPluginTo` で `hozo:plugin/<名前>/<相対パス>`。

### `internal/plugin`

ホスト側。`docs/plugin-protocol.md`。

```go
type Manifest struct{ ...; Transpilers []Transpiler }
type Transpiler struct { Name, Language string; Requires, Unsupported, Seams []string }  // [[transpiler]] 一つ。Requires が空なら基底。Seams は継ぎ目を要るデコレータの FQN
func LoadManifest(path string) (*Manifest, []diag.Diagnostic)                           // MAN-2 / MAN-4
type Plugin struct { Name, Dir string; Manifest *Manifest; Mismatch []diag.Diagnostic } // 入っているプラグイン一つ。Dir は絶対パス。Mismatch は MAN-2（使うときにだけ報告する）
type SearchEnv struct { PluginPath, Home string }                                       // 探索が見るプロジェクトの外の場所（HOZO_PLUGIN_PATH の値、ホーム）。呼ぶ側が渡す。零値なら plugin_dirs だけ
func Discover(cfg *config.Config, env SearchEnv) ([]*Plugin, []diag.Diagnostic)         // 入っているプラグインのすべて（docs/plugin-protocol.md「探索」の順）。MAN-3 / MAN-4。プロセスの環境は読まない
type Runner struct { Root string; Stderr io.Writer; Env []string }
func (r Runner) Run(m *Manifest, dir string, req any, resp any) error                   // 起動して JSON をやり取り（docs/plugin-protocol.md「起動」）。resp が *sdk.EmitResponse か *sdk.CheckResponse なら応答の形も確かめる（同「応答の形」）
var ErrCrashed, ErrMalformedOutput error                                                // errors.Is で判定。*PluginError は exit 2 の code を持ち、MAN-2 / MAN-5 の診断にする
```

### `internal/project`

一度の解析のすべて。CLI と言語サーバーの共通の入口。

```go
type Project struct {
    Config  *config.Config          // 設定なしなら既定値で、NoConfig が true
    NoConfig bool
    Files   *source.FileSet
    Sema    *sema.Result
    Plugins []*plugin.Plugin         // 入っているプラグイン（internal/plugin.Discover）
    Units   []Unit                   // 解決した単位（Config.Units と同じ順）
    Diags   []diag.Diagnostic        // 解析・解決・検査・設定・プラグインの check のすべて。並べ替え済み
}
type Unit struct { *config.Unit; Language, Base string; Transpilers []Transpiler; Seams map[string]string; Files []string }   // 言語、基底の名前（CFG-20 なら空）、解決できたトランスパイラ、単位の継ぎ目（デコレータの FQN → 宣言したトランスパイラ。CFG-23 なら、設定の順で最初のもの）、in に合ったファイル（語彙のファイルを含む）
type Transpiler struct { Name string; Plugin *plugin.Plugin; Decl *plugin.Transpiler }
type Options struct {
    Root, ConfigPath string          // hozo.toml を探し始めるディレクトリと、--config / HOZO_CONFIG
    Overlay map[string][]byte        // ディスクの代わりに使う内容（言語サーバーの開いている文書）
    Stderr io.Writer; Env []string   // プラグインの標準エラーの流し先と、プラグインに渡す環境変数の元
    PluginPath, Home string          // プラグインの探索が見るプロジェクトの外の場所（internal/plugin.SearchEnv）。空なら探さない
}
func LoadWith(opts Options) *Project                                            // 解析の本体。NoConfig なら CFG-* の診断を除く
func Load(root string, configPath string, overlay map[string][]byte) *Project   // LoadWith の短い形。プラグインは plugin_dirs だけから探す
func (p *Project) Errors() int
func (p *Project) Views() []map[string]*ir.View                                 // 単位ごとの、出力する名前空間 → View（internal/check.Views）
func (p *Project) Covered(unit int) []string                                    // 単位の対象の名前空間（Program の順。in で出力しなくなったものも含む）
func (p *Project) ManifestPath(pl *plugin.Plugin) string                        // 診断の File に使う形のマニフェストのパス
func (p *Project) Locate(at string) (*source.Span, string)                      // プラグインの at（FQN か "file:line:col"）を診断の位置に
func (p *Project) PluginDiagnostic(plugin string, d sdk.Diagnostic) diag.Diagnostic   // プラグインの診断（check の diagnostics、emit の errors）を診断に
func (p *Project) PluginWarning(transpiler string, w sdk.Warning) diag.Diagnostic     // トランスパイラの warning を PLUGIN の診断に
```

`hozoc` の `check` / `emit`（`internal/app`）と言語サーバー（`internal/lsp`）は `LoadWith` を呼ぶ。`Project` は `Root`、`Sources`、`RunErr`（プラグインの `check` が `proc.Crashed` / `proc.MalformedOutput` で終わったときの誤り）も持つ。`Covered` 以下の五つは、`internal/app` が `emit` の手順（`docs/cli.md`「emit」）で使うためのもの。

### `internal/app`

`docs/cli.md`「手で書くもの」。葉の op の実装を持つ `App`（`*App` が生成した `hozoc.Leaves` を満たし、`Sources()` / `Config()` / `Outputs()` / `OutputsSync()` が状態の単位の実装を返し、`Ops()` がそれらを `hozoc.New` に渡す）と、`cli.Presenter`（`internal/gen/hozoc/cli`）の実装 `Presenter`。引数の解釈は持たない（生成した `cli.Main`）。`App` はプロセスの環境を自分で読まない: ホームディレクトリは `App.Home`（`cmd/hozoc` が入れる）、環境変数は `cli.hozo` の入力のフィールドで届く。特定の言語のコードは持たない: トランスパイラを起動し、返ったファイルを書き出すだけ（`docs/cli.md`「emit」）。

### `internal/lsp`

`docs/lsp.md`。`func Serve(in io.Reader, out io.Writer, opts Options) error`。`Options` は `Root`、`ConfigPath`、`Log` と、`internal/project.Options` にそのまま渡す `Env`、`PluginPath`、`Home`。`internal/lsp/jsonrpc`（`Content-Length` 付きの JSON-RPC 2.0 の読み書き）と `internal/lsp/protocol`（LSP の型のうち使うもの）は、このパッケージの中だけで使う。

### `plugins/<名前>`

Go のプラグインは、どれも同じ形: `main` パッケージが `plugin.Main(<中身>.Handler{})` を呼び、中身（`View` の読み取り、検査、ファイルの生成）は `plugins/<名前>/internal/<パッケージ>` に置く。

| プラグイン | 文書 | 中身 |
|---|---|---|
| `plugins/go` | `docs/plugin-go.md` | `internal/gobase`: 形は `plugin/golang` で求め、ファイルのテキストを作る。GO-* |
| `plugins/go-valid` | `docs/plugin-go-valid.md` | `internal/govalid`: `validate.go`。GOVALID-* |
| `plugins/go-sqlite` | `docs/plugin-go-sqlite.md` | `internal/gosqlite`: `db.go`。GOSQL-* |
| `plugins/go-http` | `docs/plugin-go-http.md` | `internal/gohttp`: `http.go`。GOHTTP-* |
| `plugins/go-cli` | `docs/plugin-go-cli.md` | `internal/gocli`: `cli/cli.go`。CLI-* |

## 生成するもの

| 元 | 生成物 | 道具 |
|---|---|---|
| `ir` と `plugin` の型 | `schema/ir.schema.json`（`Program` / `View` と、`$defs` にプロトコルの要求と応答） | `internal/tools/irschema`（`go generate ./schema/...`。構造体は reflect で写し、`kind` のユニオンと列挙は道具の表で持つ） |
| `schema/ir.schema.json` | `sdk/ts/src/ir.ts`（IR の型） | `json-schema-to-typescript`（`sdk/ts` の `npm run generate`） |
| `token.Keywords` | `editors/vscode/syntaxes/hozo.tmLanguage.json`（予約語の部分は `token.Keywords` から、それ以外は道具の中の表から） | `internal/tools/tmgrammar`（`go generate ./editors/...`） |
| `cli.hozo` | `internal/gen/hozoc/**`、`internal/gen/proc/**` | `internal/tools/selfgen`（`make generate`。下の「自己生成の手順」） |
| `plugins/` の下の `testdata/<名前>/` の `hozo.toml` と `.hozo` | `testdata/<名前>/views.json`（プラグインの単体テストの入力の `View`） | `tests/pluginviews_test.go` の `TestPluginViewFixtures`（`go test ./tests -run TestPluginViewFixtures -update`。遅れていれば、このテストが落ちる。`docs/testing.md`「単体テスト」） |

生成物はコミットする。CI で生成し直して差分がないことを確かめる（`docs/testing.md`「生成物の一致」）。

生成せずに手で写すものは、写しが合っていることをテストで確かめる: プロトコルの要求と応答の TS の型 `sdk/ts/src/protocol.ts`（スキーマの `$defs` を写す。項目の名前、必須かどうか、型が合っていることを `sdk/ts/test/protocol.test.ts` が確かめる）と、`docs/cli.md` に埋め込んだ `cli.hozo` と `hozo.toml`（`docs/testing.md`「コーパス」）。

### 自己生成の手順

`internal/gen/hozoc` と `internal/gen/proc`（以下、合わせて「置き場」）は `cmd/hozoc` が import するパッケージなので、hozoc がそこへ直に書くと、プラグインの不具合でコンパイルできない Go が出たときに hozoc 自身がビルドできなくなり、生成し直すこともできなくなる。そうならないように、`make generate`（`go run ./internal/tools/selfgen`）は、置き場に、コンパイルできると確かめたものしか置かない。

1. 今のソース（今の置き場を含む）で `hozoc` を一時ディレクトリにビルドする。
2. 別の一時ディレクトリに `cli.hozo`、`hozo.toml`、`go.mod`、`go.sum` を写し、プラグイン（`plugins/` の `go`、`go-valid`、`go-cli`）を `go run` で動かすのに要る `ir`、`plugin`、`plugins`、`runtime` をシンボリックリンクで足して、そこで 1 の `hozoc emit` を走らせる（`tests/generated_test.go` と同じ組み立て。`go.mod` が同じなので import パスも同じになる）。プラグインの探索が見るプロジェクトの外の場所（`HOZO_PLUGIN_PATH`、ホーム）は空にして、生成物が実行する人の環境に左右されないようにする。
3. 生成した `internal/gen/hozoc/**` と `internal/gen/proc/**` を、その一時ディレクトリで `go vet` に通し、次に、リポジトリの今のものの代わりに置いたつもりで（`go build -overlay`。ディスクの上の置き場には触れない）、それを import するパッケージ（`cmd/hozoc`、`internal/app`）ごと `go build` する。
4. 3 が通ったときだけ、置き場の中身を生成したものに置き換える（なくなったファイルは消し、同じ内容のファイルには触れない）。通らなければ何も変えず、生成したものを残した一時ディレクトリの場所を示して失敗する。

- 生成は不動点: 生成し直した直後にもう一度 `make generate` しても、何も変わらない。置き換えた後の `hozoc emit` も同じ内容を書く。
- リポジトリのルートで `go run ./cmd/hozoc emit` を直に走らせれば、確かめずに置き場へ書く。生成器を変えている間は `make generate` を使う。
- 置き場が壊れて 1 のビルドが通らないときは、`internal/gen/` を git で元に戻してからやり直す。`internal/tools/selfgen` 自身はこのモジュールのどのパッケージも import しないので、そのときも動く。
