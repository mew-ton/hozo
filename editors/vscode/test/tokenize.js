// TextMate の文法で .hozo を字句に分ける補助（grammar.test.js が使う）。
// vscode-textmate + vscode-oniguruma で VS Code と同じ字句解析器を動かす。
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const vsctm = require('vscode-textmate');
const oniguruma = require('vscode-oniguruma');

const grammarPath = path.join(__dirname, '..', 'syntaxes', 'hozo.tmLanguage.json');

async function loadGrammar() {
  const wasm = fs.readFileSync(require.resolve('vscode-oniguruma/release/onig.wasm')).buffer;
  await oniguruma.loadWASM(wasm);
  const registry = new vsctm.Registry({
    onigLib: Promise.resolve({
      createOnigScanner: (sources) => new oniguruma.OnigScanner(sources),
      createOnigString: (s) => new oniguruma.OnigString(s),
    }),
    loadGrammar: async (scopeName) => {
      if (scopeName !== 'source.hozo') return null;
      return vsctm.parseRawGrammar(fs.readFileSync(grammarPath, 'utf8'), grammarPath);
    },
  });
  const grammar = await registry.loadGrammar('source.hozo');
  if (!grammar) throw new Error('source.hozo を読み込めない');
  return grammar;
}

/**
 * text を行ごとに字句に分ける。
 * 返り値は行の配列で、各行は { text, tokens: [{ text, start, end, scopes }] }。
 */
function tokenize(grammar, text) {
  const lines = text.split(/\r?\n/);
  let state = vsctm.INITIAL;
  return lines.map((line) => {
    const r = grammar.tokenizeLine(line, state);
    state = r.ruleStack;
    return {
      text: line,
      tokens: r.tokens.map((t) => ({
        text: line.slice(t.startIndex, t.endIndex),
        start: t.startIndex,
        end: t.endIndex,
        scopes: t.scopes,
      })),
    };
  });
}

module.exports = { loadGrammar, tokenize, grammarPath };
