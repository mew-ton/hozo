// Package vocab は hozoc が同梱して常に読み込む標準の語彙の .hozo（docs/vocab.md「同梱する語彙」）。
//
// go:embed はパッケージのディレクトリの外のファイルを埋め込めないので、埋め込みはこのディレクトリで行い、
// 読み込み（パスの形、FileSet への追加）は internal/vocab が受け持つ（docs/vocab.md「読み込み」）。
package vocab

import "embed"

// Files は vocab/*.hozo のすべて。ファイル名は "valid.hozo" のようにディレクトリなし。
//
//go:embed *.hozo
var Files embed.FS
