// プラグインを起動してよいかを、言語サーバーに、どう言うか（docs/editors.md「VS Code 拡張」）。エディタは、立ち上げない:
// package.json の宣言と、vscode に依らない関数（src/trust.ts。ビルドした out/trust.js）を、確かめる。
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'package.json'), 'utf8'));
const trust = require('../out/trust.js');

test('package.json: 信頼していないワークスペースでは、限られた形で動く、と宣言している', () => {
  const u = pkg.capabilities && pkg.capabilities.untrustedWorkspaces;
  assert.ok(u, 'capabilities.untrustedWorkspaces がない');
  assert.equal(u.supported, 'limited');
  assert.equal(typeof u.description, 'string');
  assert.ok(u.description.length > 0);
  // 信頼していないワークスペースの設定が、起動する実行ファイルを、決められない
  assert.deepEqual(u.restrictedConfigurations, ['hozo.hozocPath']);
});

test('package.json: 設定 hozo.pluginChecks は、真偽値で、既定は、入', () => {
  const p = pkg.contributes.configuration.properties['hozo.pluginChecks'];
  assert.ok(p, 'hozo.pluginChecks がない');
  assert.equal(p.type, 'boolean');
  assert.equal(p.default, true);
});

test('許すのは、ワークスペースが信頼されていて、設定が入のときだけ', () => {
  const cases = [
    // [信頼, 設定, 許す]
    [true, true, true],
    [true, undefined, true], // 未設定は、既定の、入
    [true, false, false],
    [false, true, false],
    [false, undefined, false],
    [false, false, false],
    [true, 'yes', true], // 真偽値でない設定は、既定として読む
    [false, 'yes', false],
    [undefined, true, false], // 信頼が、分からなければ、許さない
    ['true', true, false],
  ];
  for (const [trusted, setting, want] of cases) {
    assert.equal(trust.pluginChecksAllowed(trusted, setting), want, `信頼=${trusted} 設定=${setting}`);
  }
});

test('言語サーバーに渡す形: initializationOptions と、workspace/didChangeConfiguration の params', () => {
  assert.deepEqual(trust.initializationOptions(true, true), { pluginChecks: true });
  assert.deepEqual(trust.initializationOptions(false, true), { pluginChecks: false });
  assert.deepEqual(trust.configurationParams(true, undefined), { settings: { hozo: { pluginChecks: true } } });
  assert.deepEqual(trust.configurationParams(true, false), { settings: { hozo: { pluginChecks: false } } });
});
