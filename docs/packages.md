# Go のパッケージ

## この文書が決めること

モジュール `github.com/mew-ton/hozo` のディレクトリとパッケージの切り方、各パッケージの責務と主な API、依存の向き。リポジトリの構成の持ち主はこの文書。

## ディレクトリ

```
go.mod                 module github.com/mew-ton/hozo
hozo.toml, cli.hozo    hozoc 自身の仕様と設定（docs/cli.md）
Makefile               build / generate（「自己生成の手順」）/ vet / test
cmd/hozoc/             CLI の main。app.App の Ops()（実装を compiler.New に渡したもの）と app.Presenter を、生成した cli.Main に渡すだけ
ir/                    IR の型（公開）
plugin/                Go 向けプラグイン SDK（公開）。testdata/ に、二つの SDK のテストが読む表
plugin/golang/         Go の基底の形の規則: 置き場、名前、型、op の形（公開）
plugin/govalid/        go-valid の形の規則: どの型が Validate を持つか（公開）
plugin/httpvocab/      語彙 http の、言語に依らない読み解き: 経路、通信に出る union、使い方の検査（公開）
plugin/validvocab/     語彙 valid の、言語に依らない読み解き: 制約、使い方の検査、値一つが制約に合うか（公開）
plugin/dbvocab/        語彙 db の、言語にも保存先にも依らない読み解き: 表の状態、列、read と効果、使い方の検査（公開）
runtime/               生成した Go のコードが import する型（公開）
vocab/                 標準の語彙の .hozo と、それを embed する公開パッケージ vocab（go:embed はパッケージのディレクトリの外を参照できないため）
internal/source/       ファイルと位置
internal/diag/         診断
internal/syntax/       字句・構文解析（token/、ast/ は字句と構文木）
internal/sema/         解決、型、正規化
internal/check/        検査
internal/config/       hozo.toml
internal/vocab/        埋め込んだ標準の語彙
internal/plugin/       プラグインの探索と起動（WASM の実行環境）
internal/wasmbuild/    このリポジトリのプラグインを WASM にビルドする（テストが使う。docs/testing.md「プラグインの WASM」）
internal/testmap/      テストの対応表の形の確かめと、実行された行との突き合わせ
internal/fakeplugin/   テストのための、にせもののプラグイン（cmd/fakeplugin が中身。WASM にビルドして使う。SDK を通さずに、決まった応答を返す。docs/testing.md「環境からの独立」）
internal/project/      一度の解析のすべて
internal/app/          hozoc の葉の op と状態の単位の実装と表示
internal/lsp/          言語サーバー（jsonrpc/ は JSON-RPC、protocol/ は LSP の型のうち使うもの）
internal/gen/hozo/compiler/    cli.hozo からの生成物: 入口のパッケージ（go）。inner/ は内側のパッケージ（go）、types/ は型のパッケージ（go と go-valid）、compilertest/ はテストのパッケージ（go）、cli/ は go-cli の出力
internal/tools/        生成の道具: irschema/ と tmgrammar/（go generate）、selfgen/（make generate）
plugins/<name>/        プラグイン。一つのディレクトリが、一つの WASM（main パッケージをビルドした plugin.wasm。リポジトリには入れない）と hozo-plugin.toml（提供するトランスパイラの一覧を持つ）
plugins/go/            Go の基底のプラグイン（main。中身は internal/gobase/）
plugins/go-valid/      go-valid プラグイン（main。中身は internal/govalid/）
plugins/go-sqlite/     go-sqlite プラグイン（main。中身は internal/gosqlite/）
plugins/go-http/       go-http プラグイン（main。中身は internal/gohttp/）
plugins/http/          語彙のプラグイン http（main。中身は、公開のライブラリ plugin/httpvocab）
plugins/valid/         語彙のプラグイン valid（main。中身は、公開のライブラリ plugin/validvocab）
plugins/db/            語彙のプラグイン db（main。中身は、公開のライブラリ plugin/dbvocab）
plugins/go-cli/        go-cli プラグイン（main。中身は internal/gocli/）と、その語彙 cli.hozo
plugins/go-design/     go-design プラグイン（main。中身は internal/godesign/）
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
| `cmd/hozoc` | `internal/app` |
| `internal/app` | `internal/config`、`internal/diag`、`internal/gen/hozo/compiler`、`internal/gen/hozo/compiler/cli`、`internal/lsp`、`internal/plugin`、`internal/project`、`internal/source`、`internal/testmap`、`ir`、`plugin` |
| `internal/lsp` | `internal/diag`、`internal/lsp/jsonrpc`、`internal/lsp/protocol`、`internal/project`、`internal/sema`、`internal/source`、`internal/syntax`、`internal/syntax/ast`、`internal/vocab`、`ir` |
| `internal/lsp/jsonrpc` | — |
| `internal/lsp/protocol` | — |
| `internal/project` | `internal/check`、`internal/config`、`internal/diag`、`internal/plugin`、`internal/sema`、`internal/source`、`internal/syntax`、`internal/syntax/ast`、`internal/vocab`、`ir`、`plugin` |
| `internal/plugin` | `internal/config`、`internal/diag`、`ir`、`plugin` |
| `internal/testmap` | `plugin` |
| `internal/fakeplugin` | `internal/plugin`、`internal/wasmbuild` |
| `internal/wasmbuild` | `internal/plugin` |
| `internal/fakeplugin/cmd/fakeplugin` | — |
| `internal/check` | `internal/config`、`internal/diag`、`internal/source`、`ir` |
| `internal/sema` | `internal/diag`、`internal/source`、`internal/syntax/ast`、`internal/syntax/token`、`ir` |
| `internal/syntax` | `internal/diag`、`internal/source`、`internal/syntax/ast`、`internal/syntax/token` |
| `internal/syntax/ast` | `internal/source` |
| `internal/syntax/token` | `internal/source` |
| `internal/config` | `internal/diag` |
| `internal/diag` | `internal/source` |
| `internal/source` | — |
| `internal/vocab` | `vocab` |
| `internal/gen/hozo/compiler` | `internal/gen/hozo/compiler/inner` |
| `internal/gen/hozo/compiler/inner` | `internal/gen/hozo/compiler/types`、`runtime` |
| `internal/gen/hozo/compiler/types` | `runtime` |
| `internal/gen/hozo/compiler/compilertest` | `internal/gen/hozo/compiler`、`runtime` |
| `internal/gen/hozo/compiler/cli` | `internal/gen/hozo/compiler`、`runtime` |
| `internal/tools/irschema` | `ir`、`plugin` |
| `internal/tools/tmgrammar` | `internal/syntax/token` |
| `internal/tools/selfgen` | — |
| `ir` | — |
| `plugin` | `ir` |
| `plugin/golang` | `ir`、`plugin` |
| `plugin/govalid` | `ir`、`plugin/validvocab` |
| `plugin/httpvocab` | `ir`、`plugin`、`plugin/validvocab` |
| `plugin/validvocab` | `ir`、`plugin`、`runtime` |
| `plugin/dbvocab` | `ir`、`plugin` |
| `runtime` | — |
| `vocab` | — |
| `plugins/go` | `plugin`、`plugins/go/internal/gobase` |
| `plugins/go/internal/gobase` | `ir`、`plugin`、`plugin/golang` |
| `plugins/go-valid` | `plugin`、`plugins/go-valid/internal/govalid` |
| `plugins/go-valid/internal/govalid` | `ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`plugin/validvocab` |
| `plugins/go-sqlite` | `plugin`、`plugins/go-sqlite/internal/gosqlite` |
| `plugins/go-sqlite/internal/gosqlite` | `ir`、`plugin`、`plugin/dbvocab`、`plugin/golang` |
| `plugins/go-http` | `plugin`、`plugins/go-http/internal/gohttp` |
| `plugins/go-http/internal/gohttp` | `ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`plugin/httpvocab` |
| `plugins/http` | `plugin`、`plugin/httpvocab` |
| `plugins/valid` | `plugin`、`plugin/validvocab` |
| `plugins/db` | `plugin`、`plugin/dbvocab` |
| `plugins/go-cli` | `plugin`、`plugins/go-cli/internal/gocli` |
| `plugins/go-cli/internal/gocli` | `ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`plugin/validvocab` |
| `plugins/go-design` | `plugin`、`plugins/go-design/internal/godesign` |
| `plugins/go-design/internal/godesign` | `ir`、`plugin`、`plugin/golang` |
| `tests/corpus/view/plugins/view-dump` | `plugin` |
| `schema` | — |
| `editors/vscode/syntaxes` | — |
| `tests` | — |

- `internal/` は `cmd/hozoc` と `internal/` の中からだけ使う。例外は、リポジトリをまたぐテストのパッケージ `tests` だけ（テストのファイルから `internal/app`、`internal/project` などを import する。`tests` にテストでないファイルはない）。
- プラグインが使えるのは `ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`plugin/httpvocab`、`plugin/validvocab`、`plugin/dbvocab`、`runtime`（と、自分のディレクトリの下のパッケージ）。テスト用のプラグインも同じ。別のプラグインのパッケージは import しない: 相乗りが基底やほかの相乗りの形を知るのは、公開の SDK（`plugin/golang`、`plugin/govalid`）を通してだけ（`docs/plugin-protocol.md`「相乗りが基底の形を知る方法」）。
- 公開のパッケージ（`ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`plugin/httpvocab`、`plugin/validvocab`、`plugin/dbvocab`、`runtime`、`vocab`）は `internal/` に依存しない。
- **hozoc 本体（`cmd/hozoc` と `internal/` のパッケージ。生成物 `internal/gen/` を除く）は、特定の言語の出力のコードを持たない。** `plugin/golang`、`plugin/govalid`、`plugins/` を import しない（言語を知るのはプラグインだけ。`docs/plugin-protocol.md`「この文書が決めること」）。語彙ごとの決まりも、持たない: `plugin/httpvocab`、`plugin/validvocab` を import しない（語彙の決まりを知るのは、語彙のプラグインと、その語彙を出すトランスパイラだけ。同「語彙のプラグイン」）。`runtime` を import するのは、生成物だけ。
- 解析（`internal/project`）は、プラグインの `emit` を起動しない。応答のファイルを集めて書き出すのは `internal/app`。言語サーバーは `internal/project` までで、プラグインの起動（`emit`）を行わない。
- `internal/sema` は設定（`internal/config`）に依存しない。設定に関わる検査は `internal/config`、`internal/check`、`internal/project` が行う（「`internal/config`」）。
- `internal/tools/selfgen` はこのモジュールのどのパッケージも import しない（生成物が壊れていても動くため。「自己生成の手順」）。
- 外部のモジュールは、二つ: `github.com/BurntSushi/toml`（`internal/config` と `internal/plugin` が使う）と、WASM の実行環境 `github.com/tetratelabs/wazero`（`internal/plugin` だけが使う。それが要る `golang.org/x/sys` が、間接に入る。`docs/plugin-protocol.md`「起動」）。依存を最小にするため、言語サーバーの JSON-RPC も自前で書く。
- `tests/samples/` のサンプルは別のモジュール（自分の `go.mod`）なので、この表にも上の規則にも入らない。サンプルだけが使う依存は、サンプルの `go.mod` に置く（`tests/samples/README.md`「置き方」）。サンプルが自分で持つプラグインも、プラグインの規則（使えるのは `ir`、`plugin`、`plugin/golang`、`plugin/govalid`、`runtime`）に従う。

## パッケージ

**公開のパッケージは、見本の SDK で、形は、約束しない**（`docs/decisions.md`「決めたこと」446）: プラグインを書く人に約束するのは、やり取り（`docs/plugin-protocol.md`）と、IR の JSON Schema（`schema/ir.schema.json`。版の持ち主は `docs/ir.md`「版」）。下の「（公開）」は、Go の `internal/` の外にあって、プラグインから import できる、という意味で、名前や引数を変えない、という約束ではない。

### `ir`（公開）

`docs/ir.md` の型。`encoding/json` のタグはこの文書のフィールド名。`Program` / `View` と、`Type` などの判別付きユニオンは `kind` で分ける（Go では interface と `UnmarshalJSON`）。

```go
func VersionAccepts(host, plugin string) bool      // 版 host の hozoc が、版 plugin を話すプラグインを動かせるか（docs/plugin-protocol.md「版の合わせ方」）。hozoc も SDK も、これで確かめる
func (p *Program) Lookup(fqn string) Decl          // 宣言を引く。なければ nil
func (p *Program) Namespace(fqn string) *Namespace
func (v *View) Signature(op string) *ViewedSignature
func (v *View) Split() (*Spec, *OutputView)       // View を、出力に依らない部分と、出力ごとの部分に分ける（docs/ir.md「まとめた形」）。synthesized は、viewer の decls の末尾の、合成した model
func (s *Spec) View(o *OutputView) *View           // 二つを合わせた View（同「まとめた形」の「合わせ方」）。s.Program は、変えない
func (v *View) InUnit(fqn string) bool             // viewer の名前空間の宣言で、単位の対象か（span.file が View.files にある。docs/config.md「in」）
func (o *Op) DirectReads() []ReadRef               // 葉の op が直に読むもの: Reads の後ろに Carried.Reads（重複は最初の出現に寄せる）。検査を通った仕様では、
                                                   // 葉の Reads は空（STATE-5）なので、Carried.Reads と同じ（docs/check.md「デコレータが運ぶ reads と effects」）
func (o *Op) DirectEffects() []EffectRef           // 葉の op が直に持つ効果: Effects の後ろに Carried.Effects（同上）
func (c Call) CalledOp() string                    // 呼び出し先が op なら、その FQN。read か効果の呼び出しなら ""（docs/ir.md「配線」の Callee）
func (p *Program) ReadState(r ReadRef) string      // 要素が読む基底の状態の FQN: named なら read の宣言の state、direct ならその state。宣言がなければ ""
func (p *Program) ReadStates(rs []ReadRef) []string   // 要素ごとの ReadState を同じ順に並べ、重複を最初の出現に寄せたもの。op が直に読む状態は p.ReadStates(o.DirectReads())
func (p *Program) EffectWrites(e EffectRef) []string  // 効果が書き込む基底の状態の FQN: named なら effect の宣言の writes、direct ならその state。宣言がなければ nil
func NamedReads(fqns ...string) []ReadRef          // read の宣言の FQN の列から、named の要素の列
func StateReads(states ...string) []ReadRef        // 基底の状態の FQN の列から、direct の要素の列
func NamedEffects(fqns ...string) []EffectRef      // effect の宣言の FQN の列から、named の要素の列
```

`ReadRef` と `EffectRef`（`docs/ir.md`「op」）は、具体型が値型（`NamedRead`、`DirectRead`、`NamedEffect`、`DirectEffect`）の interface で、`==` で比べられ、写像のキーにできる。`Callee`（同「配線」）も同じ形で、具体型は `OpCallee`、`ReadCallee`、`EffectCallee`。`String()` は表示のための文字列: 名前で書いた要素は宣言の FQN、直に書いた要素は `<状態の FQN>`（読む方）か `<状態の FQN>.<操作>`（効果）。

TS の SDK は、ない（`docs/decisions.md`「決めたこと」415 で、外した）。

### `plugin`（公開）

プラグインの SDK。JSON の形は `docs/plugin-protocol.md`「要求と応答」で、型はそれを写す。

```go
type Handler interface {
    Emit(req *EmitRequest) (*EmitResponse, error)   // トランスパイラ一つが、出力一つに答える。要求の outputs と transpilers の並びは、Run が回す
    Import(req *ImportRequest) (*ImportResponse, error)
    Coverage(req *CoverageRequest) (*CoverageResponse, error)
}
type EmitRequest struct { Protocol string; Direction string; Transpiler string; View *ir.View }   // Emit の一回の呼び出し: 頼まれたトランスパイラの名前と、出力一つの View（Run が、要求の spec と、出力の view を合わせて作る）
type EmitResponse struct { Files []File; Warnings []Warning; Errors []Diagnostic }                // トランスパイラ一つの答え（応答の results の要素）
// やり取りの JSON の形（docs/plugin-protocol.md「emit」）。プラグインの書き手は、触らない（Run が読み書きする）。hozoc が、要求を作るのに使う
type EmitBatch struct { Protocol string; Direction string; Spec *ir.Spec; Outputs []EmitOutput }
func NewEmitBatch(views []*ir.View, transpilers [][]string) *EmitBatch   // 同じ仕様の View の並びと、View ごとに頼むトランスパイラの名前から、要求を作る（spec は、最初の View のもの）
type EmitOutput struct { Transpilers []string; View *ir.OutputView }
type EmitBatchResponse struct { Outputs []OutputResult }
type OutputResult struct { Results []EmitResponse }
func OpsWith(view *ir.View, decorator string) []string             // view.viewer の op のうち、単位の対象（View.InUnit）で、そのデコレータ（FQN。via も見る）が付いたものの FQN。相乗りが自分の対象を探す道具
func Options(req *EmitRequest, into any) ([]string, error)         // req.View.Config.Options[req.Transpiler] を into（構造体へのポインタ。フィールドは json タグで結ぶ）に読む。
                                                                   // 書かれていなければ into はそのまま。戻り値は、into にないキー（辞書順）。型の違う値は error。
                                                                   // 知らないキーと error を診断にするのは呼ぶ側（docs/config.md「オプション」）
const Version = "0.0.0-dev"                                         // このリポジトリのプラグインの版。生成したファイルのヘッダに書く（docs/plugin-go.md「ファイル」）
// ImportRequest / ImportResponse / File / Warning / Diagnostic も同じ形
// 語彙のプラグイン（docs/plugin-protocol.md「語彙のプラグイン」「read」）。Handler が、Reader も満たしていれば、Run が、direction "read" で呼ぶ
type Reader interface { Read(req *ReadRequest) (*ReadResponse, error) }
type ReadRequest struct { Protocol string; Direction string; Spec *ir.Spec }   // direction "read" の要求。hozoc が、要求を作るのにも使う
type ReadResponse struct { Errors []Diagnostic; Warnings []Warning }
func ReadOnly(r Reader) Handler                                    // read だけを実装するプラグインの Handler（emit、import、coverage は ErrUnsupported）
type CoverageRequest struct { Protocol string; Direction string; Coverage File; Files []string; Outputs []ir.NamespaceConfig }   // docs/plugin-protocol.md「coverage」
type CoverageResponse struct { Recognized bool; Files []CoveredFile }
type CoveredFile struct { Path string; Lines [][2]int }             // 実行された所の、行の範囲（始まり、終わり）

// テストの対応表（docs/plugin-protocol.md「テストの対応表」）
type TestMap struct { Tests []TestEntry }
type TestEntry struct { Name string; File string; Lines [2]int }
func TestMapName(transpiler string) string                          // "hozo-tests.<transpiler>.json"
func TestMapTranspiler(path string) (string, bool)                  // path の名前が、対応表のもの（hozo-tests.<名前>.json）なら、その <名前>
func TestMapFile(dir, transpiler string, tests []TestEntry) File     // dir の下の、対応表のファイル。テスト一つを一行に書く（docs/plugin-go.md「テストの対応表」の並びと字下げ）

// Main は標準入力の要求を読み、protocol を確かめ、h を呼び、標準出力に応答を書く。終了コードを返す
// （docs/plugin-protocol.md「要求と応答」「起動」。文字列でない protocol も「合わない」で code "protocol"）。
// direction "emit" は、要求の outputs の順、その中は transpilers の順に、h.Emit を呼び、答えを、同じ並びで応答に置く。
// 要求の、知らない項目は、どの階層でも、読まずに無視する（同「要求と応答」）。
func Main(h Handler) int
func Run(h Handler, in io.Reader, out, errOut io.Writer) int   // Main の中身。テスト用
// 実装しない方向と、Emit で知らない Transpiler は ErrUnsupported を返す。Main は docs/plugin-protocol.md「起動」の code "unsupported" で終わる
var ErrUnsupported error
```

`Select`、`Wrap`、`EmitResponse.Select` はなくした（`docs/plugin-protocol.md`「選択をなくしたこと」）。`Handler` の `Check`、`CheckRequest`、`CheckResponse`、`CheckByEmit` も、なくした（同「検査のための頼みごとを、なくしたこと」）。

### `plugin/golang`（公開）

`docs/plugin-go.md` の形の規則の写し。基底 `go` と、それに相乗りする Go のトランスパイラが共有する: 相乗りは、基底の出力を見ずに、ここの関数で基底の置き場と名前を求める（`docs/plugin-protocol.md`「相乗りが基底の形を知る方法」）。規則の持ち主は `docs/plugin-go.md` で、このパッケージは、どの見出しを写したかをコメントに書く。公開の関数はここに並べたものがすべて。

```go
// 名前（docs/plugin-go.md「名前」）
func TypeName(fqn, namespace string) string       // namespace は宣言した名前空間。合成 model（`.`）の連結を含む
func FieldName(name string) string                // model のフィールドと、例外の引数（例外の構造体のフィールド。同「exception」）の名前
func MethodName(name string) string               // op、read、効果の名前から、メソッドの名前。export の有無によらず、先頭を大文字に
func BuiltinMethod(operation string) string       // 直に呼ぶ組み込みの効果の、状態の単位のメソッド: post から Post（同「状態の単位」）
func LocalName(name string, reserved map[string]bool) string   // op・read・効果の引数名と、val・for の変数の名前。Go の予約語か事前宣言の識別子か reserved にあれば末尾に _
func EffectConst(name string) string
func CaseConst(classify, caseName string) string
func StateUnitName(stateOrClassify string) string // <状態>State
func DepName(namespace string) (typeName, param string)   // 別の名前空間の口: StoreOps と store
func SeamMethod(opName, decorator string) string  // 継ぎ目のメソッド: login と rate.limited から LoginRateLimited（同「名前」「継ぎ目」）
func WrapMethod(opName, decorator string) string  // 本体のあるデコレータの外側のメソッド: logout と server.audited から logoutServerAudited（同「本体のあるデコレータ」）
func SeamCasesName(decorator string) string       // デコレータのテストの口の名前 <D>: rate.limited から RateLimited（同「デコレータのテストの口」）
func SeamDecorators(s *Shape) []string            // デコレータのテストの口の対象: s.Seams にメソッドを持つデコレータ（最初に出てくる順）
func BodyName(opName string) string               // 基底が出す中身のメソッド: loginBody
func Capitalize(s string) string                  // 先頭の一文字を大文字に
func IsKeyword(name string) bool                  // Go の予約語か
func IsPredeclared(name string) bool              // Go の事前宣言の識別子（error、string、nil、len など）か
func PackageName(namespace string) string

// 型（同「型」）
func GoType(t ir.Type, imports *Imports) string   // 型の文字列。必要な import を imports に足す
func IsNillable(t ir.Type, p *ir.Program) bool    // Go の零値 nil が「なし」を表せる型か（array、インデックスの model、union）。optional(T) を *T にしない型
func Zero(t ir.Type, imports *Imports) string     // 零値の式（同「呼び出し」）
func TypeDisplay(t ir.Type) string                // コメントの中の型（同「コメント」の「型の表示」）
func RowDisplay(e *ir.Effect) string              // コメントの中の行の遷移（同「コメント」の「行の遷移の表示」）。行の遷移を持たない効果は ""
func NarrowedParamsDisplay(e *ir.Effect) string   // コメントの中の、絞り込んだ引数（同「コメント」の「絞り込んだ引数の表示」）。なければ ""

// 置き場（同「配置」）
func ModuleOf(cfg ir.NamespaceConfig) (string, error) // 出力の設定の、Go のモジュールの名前（基底へのオプション module。同「マニフェスト」）。なければ ErrNoModule（GO-8）
func ImportPath(module, dir string) string        // module に、dir（プロジェクトのルートからの相対パス）を `/` でつないだもの。ファイルは、読まない
var ErrNoModule error
type Layout struct {                              // 一つの出力 (U, N) の置き場
    Namespace string
    Dir       string                              // Dir(U, N)。入口のパッケージのディレクトリ
    EntryPath, EntryName string                   // 入口のパッケージの import パスと名前
    HasInner  bool                                // 内側のフォルダを持つか（N の op に内部の op がある）
    DefDir, DefPath, DefName string               // 定義のパッケージ（HasInner なら <Dir>/inner と "inner"、そうでなければ入口と同じ）
    HasTypes  bool                                // 型のパッケージを、分けて持つか（N が、op、read、効果、状態の宣言を持つ。同「型のパッケージ」）
    TypesDir, TypesPath, TypesName string         // 型のパッケージ（HasTypes なら <Dir>/types と "types"、そうでなければ入口と同じ）
    TestDir, TestPath, TestName string            // テストのパッケージ（<Dir>/<EntryName>test と "<EntryName>test"。同「単体テストの口」の「テストのパッケージ」）
}
func LayoutOf(view *ir.View) (*Layout, error)                               // この View の出力の置き場。module がなければ ErrNoModule
func HomeOf(view *ir.View, namespace string) (*Layout, bool, error)         // view の単位から見た、別の名前空間の型の置き場（docs/config.md「型の置き場」）。なければ false
type Imports struct{ … }                          // 一つのファイルの import。名前の決め方は同「パッケージと import パス」
func NewImports(view *ir.View, layout *Layout, inDef bool) *Imports   // inDef は、書くファイルが定義のパッケージのものか（入口のものなら、N の型は入口の名前で修飾なし）
func NewTypesImports(view *ir.View, layout *Layout) *Imports          // 型のパッケージ（docs/plugin-go.md「型のパッケージ」）のファイルの import。N の型は、修飾なし
func NewTestImports(view *ir.View, layout *Layout) *Imports           // テストのパッケージのファイルの import。N の宣言も、入口のパッケージの名前で修飾し、その import は、いつも名前を書く
func (*Imports) Reserve(names ...string)              // このファイルの import の名前に使わせない名前を足す（ファイルを書くトランスパイラが、関数の本体の中で決まった名前で使う識別子。
                                                      // 基底の o、ctx、err）。基底の非公開の名前 ops と ptr は、Reserve しなくても、どのファイルでも使わせない
func (*Imports) Add(pkgPath, pkgName string) string   // import に足し、修飾に使う名前を返す
func (*Imports) Name(pkgPath string) string           // 修飾に使う名前。足していなければ ""
func (*Imports) Names() []string                      // 修飾に使う名前のすべて（辞書順）。局所の名前が避ける名前
func (*Imports) Paths() []string
func (*Imports) Render() string                       // import のブロック
func (*Imports) Qualifier(namespace string) string    // その名前空間の宣言を、このファイルから書くときの前置き（N なら ""。NewTestImports のものなら、N も、入口を import して `<名前>.`。別の名前空間なら、置き場の、型のパッケージを import して `<名前>.`。NewTestImports のものなら、置き場の入口）
func (*Imports) Canonical() *Imports                  // 同じ import を、文書の決まりの順（名前空間の FQN の辞書順）で足し直したもの。ファイルを一度作って import の集まりを知り、これで作り直すと、名前が決まりどおりになる
func (*Imports) Err() error                           // GoType / Zero / Qualifier が、型の置き場を求められなかった名前空間があれば、その誤り

// op と口の形（同「op の形」「実装を受け取る口」）
type Shape struct {                               // 一つの出力の、op と口のすべて
    Ops      []OpShape                            // N の op（単位の対象の op と、本体のない op として出すもの）。宣言順
    Leaves   []string                             // Leaves のメソッドになる op の FQN
    States   []StateUnit                          // 状態の単位（基底の状態と、述語で決まる状態の分類）。宣言順
    Classify []string                             // New が述語を受け取る、値の分類と op の分類の FQN（辞書順）: 単位の対象の配線が match に使うものと、N の効果の入口の確かめが使うもの
    Deps     []Dep                                // 別の名前空間の口（FQN の辞書順）
    Seams    []Seam                               // Seams のメソッド（op の宣言順、その中はデコレータの書かれた順）。空なら Seams も New の引数 seams もない（同「継ぎ目」）
    New      []Param                              // New の引数（名前と Go の型）。ops のフィールドも同じ
}
type OpShape struct {
    FQN, Method string
    Exported   bool
    Place      Place                              // Wiring / Never / Leaf（同「op の置き場」の 1〜3）
    Seams      []string                           // この op の継ぎ目のデコレータの FQN（書かれた順。先頭が最も外側）。空なら継ぎ目を持たない
    Layers     []Layer                            // この op の中身を包むデコレータ（書かれた順）: 継ぎ目（Seams と同じもの）と、本体のあるデコレータ
    Body       string                             // 包むデコレータを持ち、基底が中身を出す op（Wiring / Never）の、中身のメソッドの名前。それ以外は ""
}
type Layer struct {                               // op の中身を包むデコレータ一つ（同「継ぎ目」「本体のあるデコレータ」）
    Decorator string                              // デコレータの FQN
    Bodied    bool                                // 本体のあるデコレータか。真なら Method は外側のメソッド（WrapMethod）、偽なら継ぎ目のメソッド（SeamMethod）
    Method    string
    Args      []ir.Arg                            // その適用の引数
}
type StateUnit struct {                           // 状態の単位一つ（同「状態の単位」）。メソッドの並びは、形で決まる問い合わせ、Builtins、Reads、Effects
    FQN, TypeName, Param string
    Shape      string                             // 基底の状態の形。状態の分類なら ""
    Builtins   []string                           // 形で決まる命令: 単位の対象の配線が直に呼ぶ操作（post、put、delete の順）
    Reads      []string                           // この状態を読む read の FQN（宣言順）
    Effects    []string                           // この単位を宛先にする効果の FQN（宣言順）
    Predicates []string                           // 状態の分類のケースの述語（宣言順）
}
type Dep struct { Namespace, TypeName, Param string; Ops []string }
type Seam struct { Op, Decorator, Transpiler, Method string }   // 継ぎ目一つ: op の FQN、デコレータの FQN、宣言したトランスパイラ（View.config.seams の値）、Seams のメソッドの名前
type Param struct { Name, Type string }
func ShapeOf(view *ir.View) *Shape
func Signature(op *ir.Op, imports *Imports, reserved map[string]bool) (params, results string)   // op のメソッドの引数と戻り値のテキスト（いつも、最初に ctx、最後に error）
func ReadSignature(r *ir.Read, imports *Imports, reserved map[string]bool) (params, results string)       // read のメソッド（いつも ctx を取り、結果と error を返す）
func EffectSignature(e *ir.Effect, imports *Imports, reserved map[string]bool) (params, results string)   // 効果のメソッド（いつも ctx を取り、error を返す）
func BuiltinSignature(s *ir.State, operation string, imports *Imports) (params, results string)           // 形で決まる命令のメソッド（値を取る操作は value <要素の型>）
func BuiltinTakesValue(operation string) bool    // 形で決まる命令が、値（value 一つ）を取るか: post と put は取り、delete は取らない（docs/check.md「直に呼ぶ組み込みの効果」の表）
func SeamSignature(op *ir.Op, imports *Imports, reserved map[string]bool) (params, results string)   // 継ぎ目のメソッドの引数（op の引数の後ろに body）と戻り値のテキスト
func SeamBodyParam(op *ir.Op, reserved map[string]bool) string   // 継ぎ目のメソッドの最後の引数の名前: body。op の引数の局所の名前と当たれば、末尾に _ を足したもの

// 絞り込んだ型の確かめ（同「効果の入口の確かめ」「単体テストの口」）
type CheckedParam struct { Param, Classify string; Cases []string }   // 入口で確かめる引数一つ: 引数の名前、分類の FQN、型が言うケース
func CheckedParams(p *ir.Program, e *ir.Effect) []CheckedParam        // 効果 e の、入口で確かめる引数（宣言順）。なければ空
func PredicateNarrowing(p *ir.Program, t ir.Type) (classify string, cases []string, ok bool)   // t が、そのまま、述語で決まる値の分類か op の分類で絞り込んだ型なら、その分類と、ケース
func CheckedUnitName(stateOrClassify string) string                   // 入口の確かめを持つ単位を包む、非公開の型: checked<状態>State

// 定義のパッケージの公開の名前（同「名前」の「名前が当たれば GO-2」）
type PublicName struct {
    Name   string                                 // Go の名前
    Kind   PublicKind                             // PublicType / PublicArm / PublicEffect / PublicClassify / PublicCase / PublicClassifyFunc / PublicCases /
                                                  // PublicStateUnit / PublicOps / PublicNew / PublicLeaves / PublicSeams / PublicDep
    Decl   string                                 // その名前を生んだ宣言の FQN（状態の単位なら、状態か状態の分類）。診断の位置に使う。Ops、New、Leaves、Seams と、別の名前空間の口なら ""
    Detail string                                 // PublicArm ならアーム名、PublicCase ならケース名、PublicDep なら名前空間の FQN
    What   string                                 // その名前を生んだものの、診断のメッセージ用の言い方（`model store.Schema`、`インターフェース Ops`）
}
func PublicNames(view *ir.View) []PublicName      // 出力 (U, N) の定義のパッケージの公開の名前のすべて。当たる名前も、そのまま二つ返す

// テストのパッケージの、基底の公開の名前（同「単体テストの口」の「テストのパッケージ」）
type TestName struct {
    Name string                                   // Go の名前
    Decl string                                   // その名前を生んだ宣言の FQN（葉の op か、分類）。診断の位置に使う。まとめの LeafCases と RunLeaves なら ""
    What string                                   // その名前を生んだものの、診断のメッセージ用の言い方（`op server.newSession のケースの口 NewSessionCases`）
}
func PublicTestNames(view *ir.View) []TestName    // 出力 (U, N) の、テストのパッケージに、基底が出す公開の名前のすべて。口も、にせものも、ない出力では、空
func TestFakeNames(view *ir.View) []TestName      // そのうち、fakes.go の名前: にせものの型（New の引数の順）、Fakes、NewOps。fakes.go を持たない出力では、空
func TestKitOf(view *ir.View) (*TestKit, error)   // 出力 (U, N) の、テストのにせものと、テストの中の組み立ての形（docs/plugin-go.md「テストのにせもの」）。module のない出力の Layout は nil で、そのときは、TestKit と一緒に、ErrNoModule を返す
type TestKit struct {
    Namespace, Dir string                         // 出力する名前空間と、Dir(U, N)
    Shape  *Shape                                 // その出力の、op と口（別の単位の出力なら、その単位の in を省いたものとして求めた形）
    Layout *Layout                                // その出力の置き場
    Blocked string                                // テストの中の組み立てを、作れない理由（作れるなら ""）。作れない出力は、fakes.go を持たない
    Params []KitParam                             // New の引数ごとの、NewOps が渡すもの（New の引数の順）
    Reals  []KitReal                              // NewOps が、f の後ろに取る、人が書いたもの
}
func (k *TestKit) HasFakes() bool                 // fakes.go を持つか
func (k *TestKit) Fakes() []KitParam              // にせものの型を持つ Param（本物を通す出力を除く）
type KitParam struct {
    Kind  KitKind                                 // KitLeaves、KitState、KitClassify、KitExternal、KitUnit（本物を通す、別の名前空間の出力）、KitSeams
    Param, Field string                           // New の引数の名前と、Fakes のフィールドの名前（フィールドがなければ ""）
    FQN   string                                  // 状態、分類、別の名前空間の FQN
    Fake  string                                  // にせものの型の名前（KitUnit なら ""）
    Unit  *TestKit                                // KitUnit のとき、本物を通す出力
    Entry bool                                    // KitExternal が、通す出力のある名前空間の、入口の op の並びか（docs/plugin-go.md「テストのにせもの」の「別の名前空間」）
}
func ThrowSources(view *ir.View, kit *TestKit, entry string) []ThrowSource   // view の出力の入口 entry の、例外の出どころ（View.entries の、その入口の sources の順）を、kit のにせものに当てたもの（docs/plugin-go.md「テストのにせもの」の「求めるケースを、にせものに当てる」）。入口が View.entries になければ nil
func CaughtSources(view *ir.View, kit *TestKit, entry string) []ThrowSource  // 同じ入口の、受ける道（View.entries の caught の順）
type ThrowSource struct {
    Exception string                              // 例外の FQN（受ける道なら、受けられる例外）
    Kind  ir.EntrySourceKind                      // 出どころの種類（docs/ir.md「その名前空間から見た IR」の EntrySource.kind）
    Label, Ident string                           // テストの名前の <出どころ> と、口のメソッドの名前の、出どころの部分
    Made  bool                                    // Hozo が、ケースを作れるか
    Steps []FakeStep                              // 作れるとき、正常のケースの用意の後に、にせものに言うこと（順に）
    Why   string                                  // 作れないとき、その理由
}
type FakeStep struct {
    Path []string                                 // 入口の出力の Fakes から、にせもの（のフィールド）までの、フィールドの名前
    Verb StepVerb                                 // StepThrows、StepReturnsNone、StepReturnsBool、StepIs
    Exception string; Bool bool; Classify, Case string   // 種類ごとの中身
}
type KitReal struct {
    Name string                                   // NewOps の引数の名前（import の名前は、書く側が避ける）
    Kind KitKind                                  // KitLeaves か KitClassify
    Namespace, Classify string                    // その口を持つ出力の名前空間と、分類の FQN（葉なら ""）
}
func TestMap(path, src string) ([]plugin.TestEntry, error)   // 整形した後の Go のソース src（置き場は path）の、走らせる関数の中のテスト（docs/plugin-go.md「テストの対応表」）。行の順。約束の形でない t.Run があれば error
func ReadCoverProfile(content string) (profile *CoverProfile, recognized bool, err error)   // go test -coverprofile の形を読む（同「カバレッジを読む」）。頭の行が違えば recognized が false
func (p *CoverProfile) Lines(name string) (lines [][2]int, measured bool)   // <import パス>/<ファイル名> の、実行された行の範囲。一行もなければ measured が false
```

- `HomeOf` が返す、別の単位の出力の `HasInner` と `DefDir` / `DefPath` / `DefName` は、その単位の `in` を省いたものとして求めた値（`View` は、ほかの単位の `in` を持たない）。別の名前空間の型を import するのは、型のパッケージ（`Layout` の `TypesDir` / `TypesPath`。`HasTypes` が偽なら、入口と同じ）で、それは、名前空間の宣言だけから決まる（`in` に依らない）ので、import には効かない。
- `Param.Type` は、定義のパッケージの中での書き方。別の名前空間の分類の述語（`<C>Cases`）は修飾なしの名前で、書く側が `Imports.Qualifier` で修飾する。
- 継ぎ目を宣言した相乗りは、`ShapeOf(view).Seams` のうち `Transpiler` が自分の名前のものについて、メソッドを出す。基底も同じ値から `Seams` を出すので、二つは合う。
- 状態の単位を満たす型を出す相乗り（`go-sqlite`）は、`ShapeOf(view).States` の `Builtins`、`Reads`、`Effects` から、単位のメソッドの並びを、`ReadSignature`、`EffectSignature`、`BuiltinSignature` から、その形を求める。メソッドの名前は、`read` と効果が `MethodName(<FQN>)`、形で決まる命令が `BuiltinMethod(<操作>)`。N の `read` と効果は、どれも、ちょうど一つの単位の `Reads` / `Effects` にある（`read` は `Read.state` の単位、効果は宛先の単位。`docs/plugin-go.md`「状態の単位」の「効果の宛先の単位」）ので、ある `read` か効果がどの単位のメソッドかは、`States` を走査して求める。`Reads` と `Effects` は、単位の対象（`in`）によらず、配線が呼んでいるかにもよらない。`Builtins` だけが、単位の対象の配線が直に呼ぶ操作で決まる。
- op のメソッドは、どれも、最初に `ctx` を取り、最後に `error` を返す（`docs/plugin-go.md`「op の形」）。op を呼ぶ相乗りは、op ごとに確かめずに、`ctx` を渡し、`error` を受ける。これまでの `OpShape.Ctx` / `OpShape.Err`（op ごとに、取るか、返すか）は、いつも真なので、なくした。
- op の置き場（`OpShape.Place`）は三つだけで、状態の単位に op のメソッドはない: `Shape.Leaves` は、置き場が `Leaf` の op のすべて。これまでの、状態の単位の `Queries` / `Commands`、`Shape.Shared`（`SharedState`）、`OpShape.Unit`、置き場 `StateCommand` / `SharedCommand` / `StateQuery` / `SharedQuery`、`PublicShared` は、なくした（`docs/decisions.md`「決めたこと」139）。
- 例外の値を作るか読む相乗り（`go-http`、`go-cli`、サンプルの `go-ratelimit`）は、例外の構造体のフィールドを、`Exception.params` の順に、`FieldName(<引数名>)` で求める（型は `GoType(<引数の型>)`、タグは `json:"<引数名>"`、省略可能なら `,omitempty`）。引数のない例外は `struct{}`。値を一つに包むフィールド `Value` は、もうない。
- これまでの `Resolver`（複製した型をローカルにする判断）はなくした。別の名前空間の型は、いつも置き場の入口のパッケージ。
- 当たる名前（GO-2）と import の循環（GO-3）を見つけるのは基底のプラグイン（`plugins/go/internal/gobase`）で、このパッケージは、当たらない前提で名前を返す。
- `PublicNames` は、`docs/plugin-go.md`「名前」の「名前が当たれば GO-2」が並べる、定義のパッケージの公開の名前の、ただ一つの写し。並びは、N の宣言順（宣言ごとに、型、アームを包む型。分類は、列挙の型、ケースの定数、判定関数、述語のインターフェース）、状態の単位（宣言順）、`Ops`、`New`、`Leaves`、`Seams`、別の名前空間の口（FQN の辞書順）。基底は、この一覧の中で名前が当たれば GO-2 を返す。入口か定義のパッケージに公開の名前を足す相乗り（`go-sqlite` の GOSQL-4、`go-http` の GOHTTP-10、サンプルの `go-ratelimit` の GORATE-2）は、自分の名前がこの一覧の名前と当たるかを確かめる（入口のパッケージは、定義のすべての名前を再公開するので、どちらに足す名前も、当たる相手はこの一覧）。一覧を自分で書き写さない。
- `PublicTestNames` は、テストのパッケージの、基底の公開の名前（`docs/plugin-go.md`「単体テストの口」: `LeafCases`、`RunLeaves`、葉ごとの `<Op>Cases` と `<Op>Case`、分類ごとの `<C>Examples` と `Run<C>`。同「テストのにせもの」: にせものの型、`Fakes`、`NewOps`）の、ただ一つの写し。基底は、この一覧の中で名前が当たれば GO-2 を返す。テストのパッケージにファイルを足す相乗り（`go-http` の GOHTTP-20）は、自分の名前が、この一覧の名前と当たるかを確かめる。空かどうかは、基底が、テストのパッケージを作るかどうか。
- `TestKitOf` は、テストのにせものと、テストの中の組み立て（`docs/plugin-go.md`「テストのにせもの」）の、ただ一つの写し。基底は、ここから `fakes.go` を作り、組み立てを使う相乗り（`go-http` の `RunHTTP`）は、同じ値から、`Fakes` のフィールドの名前と、`NewOps` の引数を求める。別の単位の出力の形は、`HomeOf` と同じに、その単位の `in` を省いたものとして求める。

### `plugin/httpvocab`（公開）

`docs/plugin-http.md`「対象」「通信の形」の「union」「診断」の写し: 標準の語彙 `http` の、言語に依らない読み解き。語彙のプラグイン `http`（`plugins/http`）と、`http` の語彙をある言語に出すトランスパイラ（`go-http`）が共有する（`docs/decisions.md`「決めたこと」428）。読むのは `ir.Program` だけで、単位にも、言語にも、依らない。

```go
const Namespace = "http"                               // この語彙の名前空間
const Get, Post, Header, Query, Body, HeaderObj, QueryObj, BodyObj, Required, Default, Status   // 読むデコレータの FQN
func Check(p *ir.Program) []plugin.Diagnostic          // すべての名前空間の経路を読み解いた診断（HTTP-*）。名前空間の IR の順、その中は op の宣言順
type Reader struct { … }                               // 仕様一つを読み解く。入力の model と union は、一度だけ調べ、診断も、一度だけ
func NewReader(p *ir.Program) *Reader
func (r *Reader) Routes(namespace string, target func(op *ir.Op) bool) []Route   // その名前空間の op のうち、target が真のもの（nil なら、すべて）の経路。診断を集める。HTTP-8 は、この呼び出しが見た経路の中で比べる
func (r *Reader) Errors() []plugin.Diagnostic          // ここまでに読み解いたものの診断（HTTP-*）
func (r *Reader) Unions() []*Union                     // ここまでに読み解いた経路の、通信に出せる union（最初に現れた順）
type Route struct { Op *ir.Op; Method, Path string; Model *ir.Model; Inputs []Input }   // 経路一つ。Model は入力の model（なければ nil）
func (r *Route) HasBody() bool
type Input struct { Field *ir.Field; Decorator string; Place Place; Whole bool; Name string; Required bool; Default ir.Literal }   // フィールド一つの読み方（デコレータが言ったことだけ）
func (in *Input) Location() string                     // 通信の上の場所（header.<名前>、body など）
type Place int                                         // InHeader / InQuery / InBody。Word() は header / query / body
type Union struct { Decl *ir.Union; Arms []Arm }       // 通信に出せる union
type Arm struct { Name string; Model *ir.Model; Keys []string }   // Keys は、決め手
func Statuses(p *ir.Program, routes []Route) []ExceptionStatus    // 経路の op が投げうる例外のうち、@http.status を持つもの（最初に現れた順）
type ExceptionStatus struct { Exception *ir.Exception; Code int64 }
```

- トランスパイラは、`Routes` を、自分の出力の op だけに絞って呼ぶ（`target`）。`Errors` が空でなければ、語彙のプラグインの検査を通っていない仕様で、トランスパイラは、診断にせず、異常終了する（`docs/plugin-go-http.md`「診断」）。
- `@http.default` の値を、制約で確かめるのは `plugin/validvocab` の `CheckLiteral`（HTTP-15）。

### `plugin/validvocab`（公開）

`docs/plugin-valid.md`「制約」「形式」「値一つを確かめる」「診断」の写し: 標準の語彙 `valid` の、言語に依らない読み解き。語彙のプラグイン `valid`（`plugins/valid`）と、`valid` の制約を読むプラグイン（`go-valid`、`go-cli`、`plugin/httpvocab` を通して語彙のプラグイン `http`）が共有する（`docs/decisions.md`「決めたこと」428）。読むのは `ir.Program` だけで、単位にも、言語にも、依らない。

```go
const Namespace = "valid"                              // この語彙の名前空間
const Min, Max, Length, Pattern, Format                // 標準の制約のデコレータの FQN
const NameCount = "count"                              // 配列のフィールドに付けた length の、違反の名前
func Standard(decorator string) bool                   // valid の標準の制約（min / max / length / pattern / format）の FQN か
func Applied(decorator string) bool                    // `@valid.*` か（標準の制約と、標準でない valid の制約）
func Formats() []string                                // format の name として書ける名前
func Check(p *ir.Program) []plugin.Diagnostic          // すべての名前空間の scalar と model のフィールドの制約を読み解いた診断（VALID-*）
type Constraint struct { Decorator, Name string; Core ir.CoreName; Bound float64; BoundText string; HasMin, HasMax bool; Min, Max int64; Regex, Format string; Count bool }   // 読んだ制約一つ。Name は、違反の名前
type Problem struct { Code, Message string }           // 制約の使い方の誤り一つ（VALID-* の ID と、メッセージ）
func Scalar(p *ir.Program, s *ir.Scalar) ([]Constraint, []Problem)             // scalar に付いた標準の制約（書かれた順）と、誤り
func Field(p *ir.Program, m *ir.Model, f *ir.Field) ([]Constraint, []Problem)  // model のフィールドに付いた標準の制約（書かれた順）と、誤り
func CoreOf(p *ir.Program, t ir.Type) (ir.CoreName, bool)                      // 元の型（scalar の継承を辿った核の型）
type Mismatch struct { Decorator, Text, Scalar string } // 値が合わなかった制約一つ: デコレータの FQN（valid.min）、書き方（@valid.min(1)）、
                                                       // scalar に付いた制約なら、その scalar の FQN（フィールドに付いたものなら ""）
func CheckLiteral(p *ir.Program, field *ir.Field, lit ir.Literal) []Mismatch   // リテラル lit が、フィールド field の制約に合うか。合わなかった制約を返す
```

- 制約を出すトランスパイラは、`Scalar` と `Field` で、自分の出力の型の制約を読む。誤り（`Problem`）が返れば、語彙のプラグインの検査を通っていない仕様で、トランスパイラは、診断にせず、異常終了する（`docs/plugin-go-valid.md`「診断」）。
- `CheckLiteral` は、値一つを制約で確かめる、ただ一つの写し（`docs/plugin-valid.md`「値一つを確かめる」）。境界のデコレータが補う値（`@http.default`、`@cli.default`）を、`http` の HTTP-15 と、`go-cli` の CLI-14 が、これで確かめる。リテラルがフィールドの型に渡せることは、呼ぶ側が先に確かめる（HTTP-14、CLI-13）。`format` は `runtime.CheckFormat` を、そのまま呼ぶ（`runtime` に依存するのは、このため）。

### `plugin/dbvocab`（公開）

`docs/plugin-db.md`「対象」「表」「キーと参照」「read」「効果」「列と引数の型」「診断」の写し: 標準の語彙 `db` の、言語にも保存先にも依らない読み解き。語彙のプラグイン `db`（`plugins/db`）と、`db` の語彙を出すプラグイン（`go-sqlite`）が共有する（`docs/decisions.md`「決めたこと」428）。読むのは `ir.Program` だけで、単位にも、言語にも、保存先にも、依らない。

```go
const Namespace = "db"                                 // この語彙の名前空間
const Table, Text, Int, Float, Bool, Datetime, Date, Time, JSON, Key, Unique, Ref,
      Find, List, Where, Latest, Insert, Update, Delete, Conflict, Transaction   // 語彙のデコレータの FQN
func Check(p *ir.Program) []plugin.Diagnostic          // すべての名前空間を読み解いた診断（DB-*）。同じ診断は、一度だけ
func ReadNamespace(p *ir.Program, name string) *Reading // 名前空間一つの読み解き
type Reading struct {                                  // 名前空間 N の読み解き
	Name    string                                     // N の名前
	Marked  int                                        // @db.table の付いた N の状態の数（表にならなかったものを含む）
	Tables  []*TableState                              // 表の状態（宣言順）
	Models  []*Model                                   // 表の model（表の状態の順。同じ model は一度）
	Reads   map[string]*ReadDecl                       // FQN → @db.find か @db.list を付けた read
	Effects map[string]*EffectDecl                     // FQN → 効果のデコレータを、ちょうど一つ付けた効果
	Errors  []plugin.Diagnostic                        // N の診断（DB-*）
}
func (r *Reading) TableOf(state string) *TableState    // FQN が state の、表の状態。なければ nil
func (r *Reading) ModelOf(model string) *Model         // FQN が model の、表の model。なければ nil
type TableState struct { State *ir.State; Name string; Model *Model }           // 表の状態一つ。Name は、表の名前（状態の名前）
type Model struct { Decl *ir.Model; Columns []*Column }                         // 表の model 一つ。Columns は、Decl.Fields と同じ順
func (m *Model) Keys() []*Column                       // @db.key の付いた列
func (m *Model) Column(name string) *Column            // 名前が name の列。なければ nil
type Column struct { Name string; Type ir.Type; Nullable, Key, Unique bool; Ref, RefTable, RefColumn string }   // 列一つ。しまい方は、持たない
type Condition struct { Column *Column; Param *ir.OpParam; Literal ir.Literal } // 条件一つ: 列が、引数か、@db.where のリテラルと同じ
type ReadDecl struct { Decl *ir.Read; Apps []ir.Application; Kind string; Table *TableState; Conds []Condition; Latest *Column; OK bool }
type EffectDecl struct { Decl *ir.Effect; Kind string; Table *TableState; Conds []Condition; Conflict string; OK bool }
func ClassOf(p *ir.Program, t ir.Type) string          // 型の種類: 核の型の名前（scalar は元の核の型）、"array"、"model"。どれでもなければ ""
func LiteralText(lit ir.Literal) string                // リテラルの、仕様での書き方
```

- `db` の語彙を出すトランスパイラは、`ReadNamespace` で、自分の出力の名前空間を読む。`Errors` が空でなければ、語彙のプラグインの検査を通っていない仕様で、トランスパイラは、診断にせず、異常終了する（`docs/plugin-go-sqlite.md`「診断」）。
- **しまい方のデコレータは、読まない**（`docs/plugin-db.md`「しまい方」）。トランスパイラは、`Model.Decl.Fields` の、列と同じ位置のフィールドから、自分で読む。
- `OK` は、その宣言に、食い違い（DB-*）がないこと。デコレータの言うことが決まらない宣言（DB-8、DB-12、DB-13、DB-18）は、`Reads` と `Effects` に入らない。

### `plugin/govalid`（公開）

`docs/plugin-go-valid.md`「Validate を持つ型」の規則の写し。`go-valid` と、`Validate` を呼ぶ相乗り（`go-http`、`go-cli`）が共有する。どのデコレータが `@valid.*` かは、`plugin/validvocab` の `Applied`。

```go
func HasValidate(p *ir.Program, typeFQN string) bool   // その scalar か model が Validate を持つか。仕様だけで決まる
func Through(p *ir.Program, t ir.Type) ir.Type         // フィールドの型を、optional、array、インデックスの model の値を通して見た型
const Transpiler = "go-valid"                          // Validate を出すトランスパイラの名前
func Unlisted(view *ir.View, namespace string) (unit int, ok bool)   // view の単位から見た namespace の型の置き場の単位が、transpilers に go-valid を並べていなければ、
                                                       // その単位が何番目の [[emit]] かと true。並べていれば、または置き場がなければ、0 と false
```

- `Unlisted` は、別の名前空間の型の `Validate` を呼べるかを見る、ただ一つの写し（`docs/plugin-go-valid.md`「出力」）: `Validate` は型の定義のパッケージにしか書けないので、呼ぶコードは、その型の置き場（`docs/config.md`「型の置き場」）の単位が `go-valid` を並べているときにだけコンパイルできる。`go-valid` の GOVALID-3、`go-http` の GOHTTP-11、`go-cli` の CLI-9 が、これで確かめる。置き場は `View.namespaces` と `View.config` だけから求まる。

### `runtime`（公開）

`docs/plugin-go.md`「runtime パッケージ」と `docs/plugin-go-valid.md`「runtime」、`docs/plugin-go-sqlite.md`「トランザクション」。`MissingError` はなくした。公開の名前はここに並べたものがすべて。

```go
type Effect string                                // 効果の名前（FQN）
type Exception interface { error; HozoException() string }   // 生成した例外の型が満たす
type ClassificationError struct { Classify string; Hits []string }   // 分類の判定の誤り。Error() を持つ
type CaseError struct { Effect, Param, Classify string; Want []string; Got string }   // 効果の入口の確かめの失敗（仕様の例外ではない）。Error() を持つ
type Violation struct { Path, Constraint string } // 制約への違反一つ。String() を持つ（go-valid）
type ValidationError struct { Violations []Violation }   // Validate が返す誤り。Error() を持つ（go-valid）
func CheckFormat(name, v string) bool             // valid.format(name) の四つの形式（docs/plugin-valid.md「形式」）の、Go の実装
type DBTxKey struct { DB *sql.DB }                // ctx が運ぶ DB のトランザクション（*sql.Tx）の鍵。DB ごとに一つ（go-sqlite）
```

- `DBTxKey` は、`go-sqlite` の出力（`db.go`）が、`@db.transaction` のトランザクションを `ctx` に置き、引くときの鍵: `context.WithValue(ctx, runtime.DBTxKey{DB: db}, tx)` と `ctx.Value(runtime.DBTxKey{DB: db}).(*sql.Tx)`。型を、出力のパッケージごとでなく、ここに一つ置くのは、同じ `*sql.DB` を使う、すべての名前空間の出力が、同じ鍵でトランザクションを見つけるため（`docs/plugin-go-sqlite.md`「トランザクション」の「鍵を `runtime` に置く理由」）。関数は持たない: 始め方と終わり方は、`go-sqlite` の出力の補助（`dbTransaction`、`Conn`）にある。
- 依存は、標準ライブラリだけ（「依存の向き」の表の `runtime` は「—」）。`DBTxKey` のために `database/sql` を import する。ドライバは import しない。

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
// TargetKind: 宣言、フィールド、ユニオンのアーム、classify のケース、デコレータの Param、val、for の変数、op の引数、名前空間
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
func (c Checker) CheckUninterpreted(p *ir.Program, ns string, files []string, interpreted map[string]bool) []diag.Diagnostic   // CFG-26。interpreted は、設定が解釈するデコレータ（すべての単位のトランスパイラの interprets と seams）
type Unit struct { Language, Base string; Seams map[string]string; Files []string }   // 設定の外で決まる単位の情報: 言語と基底の名前、単位の継ぎ目（デコレータの FQN → 宣言したトランスパイラ。どれもマニフェストから）と、in に合ったファイル
func (c Checker) CheckHomes(p *ir.Program, cfg *config.Config, units []Unit) []diag.Diagnostic   // CFG-22
func Views(p *ir.Program, cfg *config.Config, units []Unit) []map[string]*ir.View   // 単位ごと（cfg.Units と同じ順）の、出力する名前空間 → View
```

設定に関わる検査のうち、プログラムが要る CFG-10 は `Check` が（`cfg` を見て）出す。CFG-6 は `CheckUnsupported`、CFG-26 は `CheckUninterpreted`、CFG-22 は `CheckHomes`（`docs/config.md`「型の置き場」の表を写す）。

規則ごとに一つのファイル（`throws.go`、`bind.go`、…）。各関数のコメントに規則の ID。

### `internal/config`

`docs/config.md`。

```go
type Config struct { Root, Path string; Hozo HozoTable; Options map[string]map[string]any; Units []Unit }   // Options は、プロジェクト全体の [options.<名前>]（書かれたまま）
type Unit struct { Index int; In []string; Namespace string; Transpilers []string; Out string; Options map[string]map[string]any }   // [[emit]] 一つ。Index は何番目か（1 始まり）。In は省略なら nil。Options は [emit.options.<名前>] に、プロジェクト全体のオプションを混ぜたもの（docs/config.md「プロジェクト全体のオプション」。なければ空）
func Load(path string) (*Config, []diag.Diagnostic)        // CFG-7 / CFG-12 / CFG-17 / CFG-21 / CFG-24 はここで出る。Config は常に返す（既定値で補う。CFG-7 か CFG-12 の単位は Units に入れない）
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
| CFG-7、CFG-12、CFG-17、CFG-21、CFG-24 | `internal/config`（`Load`） | `hozo.toml` だけ |
| CFG-10 | `internal/check`（`Check`） | プログラム（名前空間） |
| CFG-6 | `internal/check`（`CheckUnsupported`。呼ぶのは `internal/project`） | プログラムと、マニフェストの `unsupported` |
| CFG-26 | `internal/check`（`CheckUninterpreted`。呼ぶのは `internal/project`） | プログラムと、すべての単位のトランスパイラの、マニフェストの `interprets` と `seams` |
| CFG-22 | `internal/check`（`CheckHomes`。呼ぶのは `internal/project`） | プログラムと、単位ごとの基底と `in` に合ったファイル |
| CFG-14〜CFG-16、CFG-18〜CFG-20、CFG-23 | `internal/project` | マニフェストと、読んだファイル |
| CFG-5 と、設定なしで `emit` したときの CFG-7 | `internal/app` | 書き出すファイルの一覧 |

設定なし（`Project.NoConfig`）のときは、`internal/project` が CFG-* をまとめて除く。

### `internal/vocab`

`docs/vocab.md`。公開パッケージ `vocab` が `embed` した `vocab/*.hozo` を、`Files() []File` と `AddTo(func(path string, content []byte))` で渡す。パスは `hozo:vocab/<名前>.hozo`。プラグインの語彙は `PluginFiles(plugin, dir, rels)` / `AddPluginTo` で `hozo:plugin/<名前>/<相対パス>`。

### `internal/plugin`

ホスト側。`docs/plugin-protocol.md`。

```go
type Manifest struct{ ...; Transpilers []Transpiler; Read Read; Coverage Coverage }   // Coverage.Enabled は [coverage] enabled。Read.Namespaces は [read] の namespaces（空でなければ、語彙のプラグイン: HasRead）
type Transpiler struct { Name, Language string; Requires, Unsupported, Seams, Interprets, RequiresVocabulary []string }  // [[transpiler]] 一つ。Requires が空なら基底。Seams は継ぎ目を要るデコレータの FQN。Interprets は、解釈するデコレータの FQN（Seams のものを除く。合わせたものは Interpreted()）。RequiresVocabulary は、語彙のプラグインを要る語彙の名前空間
func LoadManifest(path string) (*Manifest, []diag.Diagnostic)                           // MAN-2 / MAN-4
type Plugin struct { Name, Dir string; Manifest *Manifest; Mismatch []diag.Diagnostic } // 入っているプラグイン一つ。Dir は絶対パス。Mismatch は MAN-2（使うときにだけ報告する）
type SearchEnv struct { PluginPath, Home string }                                       // 探索が見るプロジェクトの外の場所（HOZO_PLUGIN_PATH の値、ホーム）。呼ぶ側が渡す。零値なら plugin_dirs だけ
func Discover(cfg *config.Config, env SearchEnv) ([]*Plugin, []diag.Diagnostic)         // 入っているプラグインのすべて（docs/plugin-protocol.md「探索」の順）。MAN-3 / MAN-4。プロセスの環境は読まない
func (m *Manifest) WasmPath(dir string) string                                          // マニフェストの wasm を、マニフェストのディレクトリ dir からのパスにしたもの
type Runner struct { Stderr io.Writer; Context context.Context; CacheDir string }       // Context で、動いているプラグインを止める（止める口を持つ Context を渡した起動だけが、途中で止められる形の WASM で動き、遅い）。CacheDir は、機械の言葉に直したものの置き場（"" なら、os.UserCacheDir の下の hozo）
func (r Runner) Run(name, wasm string, req any, resp any) error                         // WASM のファイル wasm を、サンドボックスの中で動かして、JSON をやり取り（docs/plugin-protocol.md「起動」）。name は、プラグインの名前。resp が *sdk.EmitBatchResponse か *sdk.CoverageResponse なら応答の形も確かめる（同「応答の形」）
func MountForTest(wasm, dir string) (undo func())                                       // テストだけの口: wasm のプラグインに、dir を見せる（docs/testing.md「環境からの独立」）。テストのプロセスでなければ panic
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
    Diags   []diag.Diagnostic        // 解析・解決・検査・設定と、プラグインの診断（emit の errors と warnings）のすべて。並べ替え済み
    Readers []*plugin.Plugin         // 頼む語彙のプラグイン（docs/plugin-protocol.md「語彙のプラグイン」: 仕様が、その語彙を使っている。名前の辞書順）。protocol の合わないものも含む
    Answers []Answer                 // プラグインから受け取った答え（docs/cli.md「検査」の 4）。単位の設定の順、名前空間の IR の順、transpilers に書いた順。起動しなかったら、空
}
type Answer struct { Unit int; Namespace string; Transpiler Transpiler; Response *sdk.EmitResponse }   // トランスパイラ一つの、出力一つ（Units の添字と、名前空間）への答え
type Unit struct { *config.Unit; Language, Base string; Transpilers []Transpiler; Seams map[string]string; Files []string }   // 言語、基底の名前（CFG-20 なら空）、解決できたトランスパイラ、単位の継ぎ目（デコレータの FQN → 宣言したトランスパイラ。CFG-23 なら、設定の順で最初のもの）、in に合ったファイル（語彙のファイルを含む）
type Transpiler struct { Name string; Plugin *plugin.Plugin; Decl *plugin.Transpiler }
type Options struct {
    Root, ConfigPath string          // hozo.toml を探し始めるディレクトリと、--config / HOZO_CONFIG
    Overlay map[string][]byte        // ディスクの代わりに使う内容（言語サーバーの開いている文書）
    Stderr io.Writer; CacheDir string // プラグインの標準エラーの流し先と、WASM を、機械の言葉に直したものの置き場（HOZO_CACHE_DIR。"" なら、ふだんの置き場）
    PluginPath, Home string          // プラグインの探索が見るプロジェクトの外の場所（internal/plugin.SearchEnv）。空なら探さない
    NoPlugins bool                   // プラグインを、起動しない（docs/cli.md「検査」の 4 を、しない）。仕様を読み直すだけの呼び出しと、言語サーバーの解析（docs/lsp.md「プラグインの診断」）
    Context context.Context          // プラグインの起動を、やめさせる口（言語サーバーの、裏の check）。nil なら、やめさせない。動いているプラグインも、その場で止まり（docs/plugin-protocol.md「起動」）、やめたら、Project.RunErr が、その誤り
}
func LoadWith(opts Options) *Project                                            // 解析の本体。NoConfig なら CFG-* の診断を除く
func Load(root string, configPath string, overlay map[string][]byte) *Project   // LoadWith の短い形。プラグインは plugin_dirs だけから探し、起動しない
func (p *Project) Errors() int
func (p *Project) Views() []map[string]*ir.View                                 // 単位ごとの、出力する名前空間 → View（internal/check.Views）
func (p *Project) Spec() *ir.Spec                                               // 仕様の、出力に依らない部分（docs/ir.md「まとめた形」の Spec）。read の要求に入れる。単位がなければ、namespaces は空
func (p *Project) Covered(unit int) []string                                    // 単位の対象の名前空間（Program の順。in で出力しなくなったものも含む）
func (p *Project) ManifestPath(pl *plugin.Plugin) string                        // 診断の File に使う形のマニフェストのパス
func (p *Project) Locate(at string) (*source.Span, string)                      // プラグインの at（FQN か "file:line:col"）を診断の位置に
func (p *Project) Answer(unit int, namespace, transpiler string) *sdk.EmitResponse    // 単位 unit（Units の添字）の、名前空間 namespace の出力への、トランスパイラ transpiler の答え。なければ nil
func (p *Project) PluginDiagnostic(plugin string, d sdk.Diagnostic) diag.Diagnostic   // プラグインの診断（emit の errors）を診断に
func (p *Project) PluginWarning(transpiler string, w sdk.Warning) diag.Diagnostic     // トランスパイラの warning を PLUGIN の診断に
```

`hozoc` の検査の op `check`（`internal/app`。`hozoc build` と `hozoc tests` が、はじめに呼ぶ）と言語サーバー（`internal/lsp`）は `LoadWith` を呼ぶ。`Project` は `Root`、`Sources`、`RunErr`（プラグインが `Crashed` / `MalformedOutput` で終わったときの誤り）も持つ。`Covered` 以下の六つは、`internal/app` が `emit` の手順（`docs/cli.md`「書き出し」）で使うためのもの。

### `internal/testmap`

テストの対応表と、カバレッジの「実行された行」の突き合わせ（`docs/cli.md`「tests」の 2 と 5）。言語も、カバレッジの形も、知らない。診断は作らない（`internal/app` が、結果から作る）。

```go
func IsMap(path string) bool                                          // 名前が hozo-tests.<…>.json か
func Parse(file sdk.File, transpiler string, files map[string]bool) (*sdk.TestMap, error)   // 形を確かめて読む。違えば error（PLG-4 の文面）。files は、同じ応答の files の path
type Test struct { Transpiler string; sdk.TestEntry }                  // 求めるテスト一つ
type Status int                                                       // Ran / NotRun（測ったが、行が実行されていない）/ Unmeasured（カバレッジが、そのファイルを測っていない）
func Match(tests []Test, covered []sdk.CoveredFile) []Status          // tests と同じ長さ、同じ順
```

### `internal/app`

`docs/cli.md`「手で書くもの」。葉の op と状態の単位の実装を持つ `App`（`*App` が生成した `compiler.Leaves` を満たし、`Sources()` / `Config()` / `Outputs()` / `OutputsSync()` が状態の単位の実装を返し、`Ops()` がそれらを `hozoc.New` に渡す。状態に触れる `read outputChanges` と効果 `syncOutputs` は、`Outputs()` と `OutputsSync()` が返す単位のメソッド）と、`cli.Presenter`（`internal/gen/hozo/compiler/cli`）の実装 `Presenter`。引数の解釈は持たない（生成した `cli.Main`）。入口の `Main(ctx, a, args, env, stdout, stderr)` は、`cli.Main` を呼ぶ前に、古い名前のコマンド（`check`、`emit`）を見て、新しい書き方を示す（`Renamed(args)` が、その文を返す。`docs/cli.md`「build」の「古い名前」）。`App` はプロセスの環境を自分で読まない: ホームディレクトリは `App.Home`（`cmd/hozoc` が入れる）、環境変数は `cli.hozo` の入力のフィールドで届く。特定の言語のコードは持たない: トランスパイラを起動し、返ったファイルを書き出すだけ（`docs/cli.md`「書き出し」）。

### `internal/lsp`

`docs/lsp.md`。`func Serve(in io.Reader, out io.Writer, opts Options) error`。`Options` は `Root`、`ConfigPath`、`Log` と、`internal/project.Options` にそのまま渡す `CacheDir`、`PluginPath`、`Home`。`internal/lsp/jsonrpc`（`Content-Length` 付きの JSON-RPC 2.0 の読み書き）と `internal/lsp/protocol`（LSP の型のうち使うもの）は、このパッケージの中だけで使う。

### `plugins/<名前>`

Go のプラグインは、どれも同じ形: `main` パッケージが `plugin.Main(<中身>.Handler{})` を呼び、中身（`View` の読み取り、検査、ファイルの生成）は `plugins/<名前>/internal/<パッケージ>` に置く。

| プラグイン | 文書 | 中身 |
|---|---|---|
| `plugins/go` | `docs/plugin-go.md` | `internal/gobase`: 形は `plugin/golang` で求め、ファイルのテキストを作る。GO-* |
| `plugins/go-valid` | `docs/plugin-go-valid.md` | `internal/govalid`: `validate.go`。GOVALID-* |
| `plugins/go-sqlite` | `docs/plugin-go-sqlite.md` | `internal/gosqlite`: `db.go`。表、列、`read` と効果は、`plugin/dbvocab` の `ReadNamespace` で読む。GOSQL-* |
| `plugins/go-http` | `docs/plugin-go-http.md` | `internal/gohttp`: 入口のパッケージの `http.go` と、テストのパッケージの `http.go`。GOHTTP-* |
| `plugins/http` | `docs/plugin-http.md` | 語彙のプラグイン: `main` が `plugin.Main(plugin.ReadOnly(…))` を呼び、`plugin/httpvocab` の `Check` の診断を返す。HTTP-* |
| `plugins/valid` | `docs/plugin-valid.md` | 語彙のプラグイン: 同じ形で、`plugin/validvocab` の `Check` の診断を返す。VALID-* |
| `plugins/db` | `docs/plugin-db.md` | 語彙のプラグイン: 同じ形で、`plugin/dbvocab` の `Check` の診断を返す。DB-* |
| `plugins/go-cli` | `docs/plugin-go-cli.md` | `internal/gocli`: `cli/cli.go`。CLI-* |
| `plugins/go-design` | `docs/plugin-go-design.md` | `internal/godesign`: テストのパッケージの `design.go`。GODESIGN-* |

## 生成するもの

| 元 | 生成物 | 道具 |
|---|---|---|
| `ir` と `plugin` の型 | `schema/ir.schema.json`（`Program` / `View` と、`$defs` にプロトコルの要求と応答） | `internal/tools/irschema`（`go generate ./schema/...`。構造体は reflect で写し、`kind` のユニオンと列挙は道具の表で持つ） |
| `token.Keywords` | `editors/vscode/syntaxes/hozo.tmLanguage.json`（予約語の部分は `token.Keywords` から、それ以外は道具の中の表から） | `internal/tools/tmgrammar`（`go generate ./editors/...`） |
| `cli.hozo` | `internal/gen/hozo/compiler/**` | `internal/tools/selfgen`（`make generate`。下の「自己生成の手順」） |
| `plugins/` の下の `testdata/<名前>/` の `hozo.toml` と `.hozo` | `testdata/<名前>/views.json`（プラグインの単体テストの入力の `View`） | `tests/pluginviews_test.go` の `TestPluginViewFixtures`（`go test ./tests -run TestPluginViewFixtures -update`。遅れていれば、このテストが落ちる。`docs/testing.md`「単体テスト」） |

生成物はコミットする。CI で生成し直して差分がないことを確かめる（`docs/testing.md`「生成物の一致」）。

生成せずに手で写すものは、写しが合っていることをテストで確かめる: `docs/cli.md` に埋め込んだ `cli.hozo` と `hozo.toml`（`docs/testing.md`「コーパス」）。

### 自己生成の手順

`internal/gen/hozo/compiler`（以下、「置き場」）は `cmd/hozoc` が import するパッケージなので、hozoc がそこへ直に書くと、プラグインの不具合でコンパイルできない Go が出たときに hozoc 自身がビルドできなくなり、生成し直すこともできなくなる。そうならないように、`make generate`（`go run ./internal/tools/selfgen`）は、置き場に、コンパイルできると確かめたものしか置かない。

1. 今のソース（今の置き場を含む）で `hozoc` を一時ディレクトリにビルドする。使うプラグイン（`plugins/` の `go`、`go-valid`、`go-cli`）を、WASM にビルドして、それぞれのディレクトリの `plugin.wasm` に置く（`docs/testing.md`「プラグインの WASM」）。
2. 別の一時ディレクトリに `cli.hozo`、`hozo.toml`、`go.mod`、`go.sum` を写し、プラグインの置き場 `plugins` と、3 の vet が要る `ir`、`plugin`、`runtime` をシンボリックリンクで足して、そこで 1 の `hozoc build` を走らせる（`tests/generated_test.go` と同じ組み立て。import パスは、`hozo.toml` の `[options.go]` の `module` から決まるので、同じになる。`go.mod` は、3 の vet とビルドのために写す）。プラグインの探索が見るプロジェクトの外の場所（`HOZO_PLUGIN_PATH`、ホーム）は空にして、生成物が実行する人の環境に左右されないようにする。WASM を、機械の言葉に直したものの置き場（`HOZO_CACHE_DIR`）は、ホームを空にする前の、もとの場所を渡す（生成のたびに、直し直さないように。答えは、変わらない）。
3. 生成した `internal/gen/hozo/compiler/**` を、その一時ディレクトリで `go vet` に通し、次に、リポジトリの今のものの代わりに置いたつもりで（`go build -overlay`。ディスクの上の置き場には触れない）、それを import するパッケージ（`cmd/hozoc`、`internal/app`）ごと `go build` する。
4. 3 が通ったときだけ、置き場の中身を生成したものに置き換える（なくなったファイルは消し、同じ内容のファイルには触れない）。通らなければ何も変えず、生成したものを残した一時ディレクトリの場所を示して失敗する。

- 生成は不動点: 生成し直した直後にもう一度 `make generate` しても、何も変わらない。置き換えた後の `hozoc build` も同じ内容を書く。
- リポジトリのルートで `go run ./cmd/hozoc build` を直に走らせれば、確かめずに置き場へ書く。生成器を変えている間は `make generate` を使う。
- 置き場が壊れて 1 のビルドが通らないときは、`internal/gen/` を git で元に戻してからやり直す。`internal/tools/selfgen` 自身はこのモジュールのどのパッケージも import しないので、そのときも動く。
