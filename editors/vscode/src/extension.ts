// Hozo の VS Code 拡張（docs/editors.md「VS Code 拡張」）。
// hozoc lsp を起動して接続するだけの薄いクライアントで、言語の知識（キーワード、検査）は持たない。
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as vscode from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from 'vscode-languageclient/node';

/** 語彙の仮想文書の URI スキーム（docs/ir.md「Program」の Span.file、docs/lsp.md「プロジェクト」）。 */
const VOCABULARY_SCHEME = 'hozo';
/** 語彙の内容を取る要求（docs/lsp.md「プロジェクト」）。 */
const VOCABULARY_REQUEST = 'hozo/vocabulary';
/** 設定の節。`hozo.trace.server` は vscode-languageclient がこの名前で読む。 */
const CONFIG_SECTION = 'hozo';

let client: LanguageClient | undefined;
let output: vscode.OutputChannel | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  output = vscode.window.createOutputChannel('Hozo');
  context.subscriptions.push(output);

  context.subscriptions.push(
    vscode.workspace.registerTextDocumentContentProvider(VOCABULARY_SCHEME, vocabularyProvider),
  );

  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration(async (e) => {
      if (e.affectsConfiguration(`${CONFIG_SECTION}.hozocPath`)) {
        await stopClient();
        await startClient();
      }
    }),
  );

  await startClient();
}

export async function deactivate(): Promise<void> {
  await stopClient();
}

/** hozoc lsp を起動して接続する。hozoc が見つからなければ通知して何もしない。 */
async function startClient(): Promise<void> {
  const hozocPath = vscode.workspace.getConfiguration(CONFIG_SECTION).get<string>('hozocPath', 'hozoc');
  const resolved = resolveExecutable(hozocPath);
  if (!resolved) {
    await notifyMissing(hozocPath);
    return;
  }

  const serverOptions: ServerOptions = {
    command: resolved,
    args: ['lsp'],
    transport: TransportKind.stdio,
  };
  const clientOptions: LanguageClientOptions = {
    documentSelector: [
      { scheme: 'file', language: 'hozo' },
      { scheme: VOCABULARY_SCHEME, language: 'hozo' },
    ],
    synchronize: {
      // サーバー側も hozo.toml と *.hozo の変更を登録する（docs/lsp.md「プロジェクト」）
      fileEvents: vscode.workspace.createFileSystemWatcher('**/{hozo.toml,*.hozo}'),
    },
    outputChannel: output,
  };

  const c = new LanguageClient(CONFIG_SECTION, 'Hozo Language Server', serverOptions, clientOptions);
  try {
    await c.start();
    client = c;
  } catch (err) {
    output?.appendLine(`hozoc lsp の起動に失敗: ${String(err)}`);
    await notifyMissing(hozocPath, String(err));
  }
}

async function stopClient(): Promise<void> {
  const c = client;
  client = undefined;
  if (c) {
    await c.stop();
  }
}

/** hozoc が見つからないことをエラーで通知し、設定を開くボタンを出す（docs/editors.md「VS Code 拡張」）。 */
async function notifyMissing(hozocPath: string, detail?: string): Promise<void> {
  const openSettings = '設定を開く';
  const message =
    `hozoc（${hozocPath}）が見つからないか起動できないため、Hozo の言語サーバーを使えません。` +
    `設定 hozo.hozocPath に hozoc の場所を指定してください。` +
    (detail ? ` (${detail})` : '');
  const choice = await vscode.window.showErrorMessage(message, openSettings);
  if (choice === openSettings) {
    await vscode.commands.executeCommand('workbench.action.openSettings', `${CONFIG_SECTION}.hozocPath`);
  }
}

/**
 * 実行ファイルの場所を決める。区切りを含めばそのパス、含まなければ PATH から探す。
 * 見つからなければ undefined。
 */
function resolveExecutable(command: string): string | undefined {
  if (command.includes('/') || command.includes(path.sep)) {
    const p = command.startsWith('~') ? path.join(process.env.HOME ?? '', command.slice(1)) : command;
    return isExecutable(p) ? p : undefined;
  }
  const dirs = (process.env.PATH ?? '').split(path.delimiter).filter((d) => d !== '');
  const exts = process.platform === 'win32' ? (process.env.PATHEXT ?? '.EXE;.CMD;.BAT').split(';') : [''];
  for (const dir of dirs) {
    for (const ext of exts) {
      const candidate = path.join(dir, command + ext.toLowerCase());
      if (isExecutable(candidate)) {
        return candidate;
      }
      if (ext !== '') {
        const upper = path.join(dir, command + ext);
        if (isExecutable(upper)) {
          return upper;
        }
      }
    }
  }
  return undefined;
}

function isExecutable(p: string): boolean {
  try {
    fs.accessSync(p, fs.constants.X_OK);
    return fs.statSync(p).isFile();
  } catch {
    return false;
  }
}

/** hozo: スキームの文書の内容を、言語サーバーの要求 hozo/vocabulary で取る。 */
const vocabularyProvider: vscode.TextDocumentContentProvider = {
  async provideTextDocumentContent(uri: vscode.Uri): Promise<string> {
    if (!client) {
      throw new Error('Hozo の言語サーバーが動いていないため、語彙の文書を開けません。');
    }
    const result = await client.sendRequest<{ content: string }>(VOCABULARY_REQUEST, { uri: uri.toString() });
    return result.content;
  },
};
