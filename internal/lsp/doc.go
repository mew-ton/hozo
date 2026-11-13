// Package lsp は hozoc lsp の中身（Language Server Protocol 3.17 のサーバー。docs/lsp.md）。
// 提供する機能と本体への前提条件は docs/requirements.md「言語サーバー」にある。
//
// 入口は [Serve]。標準入出力（テストでは任意の io.Reader / io.Writer）の上で Content-Length 付きの
// JSON-RPC 2.0（internal/lsp/jsonrpc）を話し、要求の型は internal/lsp/protocol にある。
//
//   - ライフサイクル（docs/lsp.md「ライフサイクル」）: initialize でワークスペースと位置のエンコーディングを決め、
//     initialized でプロジェクト全体の診断を一度発行し、shutdown / exit で終わる（server.go）。
//   - プロジェクト（docs/lsp.md「プロジェクト」）: 解析の単位はプロジェクト全体で、internal/project.LoadWith を
//     呼び直す。開いている文書の最新の内容を overlay として渡し、didChange は 150 ms 待ってまとめる。
//     解析し直すたびに、診断のあるファイルと、診断がなくなったファイル（空の配列）の publishDiagnostics を送り、
//     同じ内容なら送らない。一度も診断を送っていないファイルに空の配列は送らない（server.go、diagnostics.go）。
//   - 診断（docs/lsp.md「診断」）: hozoc check と同じ internal/check の診断を LSP の Diagnostic に写す（diagnostics.go）。
//   - 定義へジャンプ（docs/lsp.md「定義へジャンプ」）: sema の Ref の Target を Location に（definition.go、locate.go）。
//   - ホバー（docs/lsp.md「ホバー」）: IR から宣言の見出しと説明、op なら throws / effects の Markdown（hover.go）。
//   - ドキュメントのシンボル（docs/lsp.md「ドキュメントのシンボル」）: 構文木から DocumentSymbol の木（symbols.go）。
//   - hozo/vocabulary: `hozo:` の URI の語彙の内容（server.go）。
//
// ログは Options.Log にだけ書き、標準エラーには書かない（docs/lsp.md「通信」）。
package lsp
