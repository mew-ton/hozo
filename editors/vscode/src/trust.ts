// プラグインを起動してよいかを、言語サーバーに、どう言うか（docs/editors.md「VS Code 拡張」、docs/lsp.md「プラグインの診断」の
// 「許し」）。vscode に依らない関数だけを置く（test/trust.test.js が、エディタを立ち上げずに、確かめる）。

/** 言語サーバーに渡す、許しの項目の名前（initializationOptions と、settings.hozo の中）。 */
export const PLUGIN_CHECKS = 'pluginChecks';

/**
 * プラグインを起動してよいか: ワークスペースが信頼されていて、設定 hozo.pluginChecks が入のときだけ。
 * 設定が、真偽値でなければ（未設定を含む）、既定の、入として読む。
 */
export function pluginChecksAllowed(workspaceTrusted: boolean, setting: unknown): boolean {
  const enabled = typeof setting === 'boolean' ? setting : true;
  return workspaceTrusted === true && enabled;
}

/** initialize の initializationOptions。 */
export function initializationOptions(workspaceTrusted: boolean, setting: unknown): { pluginChecks: boolean } {
  return { [PLUGIN_CHECKS]: pluginChecksAllowed(workspaceTrusted, setting) } as { pluginChecks: boolean };
}

/** workspace/didChangeConfiguration の params（動いている間に、許しが変わったとき）。 */
export function configurationParams(
  workspaceTrusted: boolean,
  setting: unknown,
): { settings: { hozo: { pluginChecks: boolean } } } {
  return { settings: { hozo: initializationOptions(workspaceTrusted, setting) } };
}
