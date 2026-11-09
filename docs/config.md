# hozo.toml

## この文書が決めること

プロジェクトの設定ファイル `hozo.toml` の形と読み方、設定に関わる検査（CFG-*）。設計書 §4.5 の規則を手続きにしたもの。読むのは `internal/config`。

## 置き場と探し方

- `hozoc` は `--config <path>` か環境変数 `HOZO_CONFIG` で指定されたファイルを読む。どちらもなければ、作業ディレクトリから親へ向かって最初に見つかる `hozo.toml`。見つからなければ `ConfigInvalid`（`docs/diagnostics.md`「終了コード」）。
- `hozo.toml` のあるディレクトリが**プロジェクトのルート**。`.hozo` の探索、`out`、プラグインの探索の起点。

## 形

```toml
[hozo]                          # 全体の設定。省略できる
sources      = ["**/*.hozo"]    # ルートからの glob。既定はこの一つ。各名前空間の out の下と、plugin_dirs の下は常に除く
plugin_dirs  = ["plugins"]      # プラグインを探すディレクトリ（docs/plugin-protocol.md「探索」）。ルートからの相対パス。既定は []
max_bindings = 5                # WIRE-6 の閾値（docs/check.md「配線」）。既定はそこに書いた値

[browser]                       # テーブル名が名前空間の名前（§4.5）
language = "ts"                 # 必須（親から継承できる）。"go" | "ts"
plugins  = ["react-router"]     # 省略時は []（既定の出力だけ）
out      = "web/src/generated"  # 必須（親から継承できる）。ルートからの相対パス

[server]
language = "go"
plugins  = ["go-cli"]
out      = "internal/gen"

[server.admin]                  # 子の名前空間。書かなければ親の設定を引き継ぐ。書けば書いたキーだけ上書き
plugins = []
```

- TOML の読み込みは `github.com/BurntSushi/toml`（`docs/packages.md`「依存の向き」）。知らないキーは CFG-7。
- `[hozo]` 以外のトップレベルのテーブルは名前空間。ドットを含むテーブル名（`[server.admin]`）は子の名前空間。`[server] admin = { ... }` のインラインテーブルも同じ意味（TOML 上は同じ構造）。
- 値の型が違えば CFG-7。

## 名前空間の設定の解決

名前空間 N（FQN）の設定 Conf(N):

1. N のテーブルがあれば、そのキー。
2. ないキーは親 parent(N) の Conf から引き継ぐ（§4.5「子の名前空間は親の設定を引き継ぎ、子に書いたキーで上書きする」）。値はそのまま（`out` も同じディレクトリ。子のファイルをその下のどこに置くかは出力の側が決める。`docs/emit-go.md`「配置」）。ルートまで辿ってもなければ「未設定」。

Conf(N) は `View.config` としてプラグインに渡す（`docs/ir.md`「その名前空間から見た IR」）。

## 診断

| ID | 重さ | 手続き |
|---|---|---|
| CFG-1 | error | `state` か `op` を一つでも持つユーザーの名前空間で、Conf の `language` か `out` が未設定。グローバル（`""`）に `state` か `op` があれば、それも CFG-1（グローバルには設定を書けない） |
| CFG-2 | error | `.hozo` の名前空間の名前（どの階層でも）が `language` / `plugins` / `out` / `hozo`。または `hozo.toml` にこれらの名前のテーブルがある（`[hozo]` 以外） |
| CFG-3 | — | 解決の手続き。診断なし |
| CFG-4 | error | Conf(N).plugins のプラグインのマニフェストの `language` が Conf(N).language と違う。例外は `LanguageMismatch` |
| CFG-5 | error | 書き出すファイルのパス（ルートからの相対パスに正規化したもの）が、二つ以上の出どころ（既定の出力、各プラグイン）から出た。出どころを `Related` 相当としてメッセージに示す。何も書き出さない。例外は `OutputConflict` |
| CFG-6 | warning | Conf(N).plugins のプラグインのマニフェストの `unsupported` にあるデコレータの FQN が、N の宣言に付いている。付いた位置に出す |
| CFG-7 | error | 知らないキー、値の型違い、TOML の構文エラー、`language` が `"go"` でも `"ts"` でもない |
| CFG-8 | error | 語彙の名前空間（`docs/vocab.md` の名前空間と、`state` も `op` も持たない名前空間）にテーブルがある |
| CFG-9 | error | `plugins` の名前のプラグインが見つからない（`docs/plugin-protocol.md`「探索」） |
| CFG-10 | error | `hozo.toml` に名前空間のテーブルがあるが、その名前空間が `.hozo` にない |
| CFG-11 | error | `language` が `"ts"` だが、TS の既定の出力はまだない（段階 5 まで） |

CFG-4 と CFG-5 以外の error は `ConfigInvalid`。終了コードは `docs/diagnostics.md`「終了コード」。
