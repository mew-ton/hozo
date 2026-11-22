// Package project は一度の解析のすべて（docs/packages.md「internal/project」）。CLI（internal/app）と
// 言語サーバー（internal/lsp）の共通の入口で、docs/cli.md「検査」の 1〜4 をこの順に行う:
//
//  1. hozo.toml を読む（docs/config.md「置き場と探し方」）。なければ既定の設定で動き（Project.NoConfig）、
//     CFG-* の診断を除く（言語サーバーの「設定なしで動く」のため。CLI 側で CFG-7 にするのは internal/app）。
//     入っているプラグインのマニフェストを探して読み、各単位の transpilers の名前をトランスパイラに解決する
//     （docs/plugin-protocol.md「探索」、docs/config.md「単位」。CFG-11 / CFG-14〜CFG-16、MAN-*）。
//  2. 標準の語彙、使うプラグインの語彙、[hozo].sources のファイル（docs/vocab.md「読み込み」）を読み、解析する。
//     sources はルートからの glob で、各単位の out の下と plugin_dirs の下は常に除く（docs/config.md「形」）。
//     読んだファイルに各単位の in を照らす（docs/config.md「in」。CFG-18）。
//  3. 解決・正規化（internal/sema）と検査（internal/check）。CFG-6 はマニフェストが要るのでここで出す。
//  4. 1〜3 に error がなければ、使うプラグインごとに一回、direction "emit" を送り（一つの要求に、そのプラグインが受け持つ
//     出力を並べる）、返った答えを残して（Project.Answers）、その診断（errors と warnings）を足す。
//
// 診断はすべて集めて docs/diagnostics.md「順序」に並べ替える。ファイルのパスはルートからの相対パス
// （docs/ir.md「Program」の Span.file と同じ形）で、hozo.toml やマニフェストの位置のない診断の File も同じ形にする。
package project
