// TextMate の文法を VS Code と同じ字句解析器（vscode-textmate + vscode-oniguruma）で動かし、
// 実物の .hozo（コーパス、標準の語彙、cli.hozo）の選んだ字句が docs/editors.md「TextMate の文法」の
// scope になることを確かめる。
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { loadGrammar, tokenize } = require('./tokenize');

const root = path.join(__dirname, '..', '..', '..');
const files = {
  server: path.join(root, 'tests', 'corpus', 'design', 'server.hozo'),
  browser: path.join(root, 'tests', 'corpus', 'design', 'browser.hozo'),
  http: path.join(root, 'vocab', 'http.hozo'),
  page: path.join(root, 'vocab', 'page.hozo'),
  valid: path.join(root, 'vocab', 'valid.hozo'),
  cli: path.join(root, 'cli.hozo'),
  // 状態の分類・遷移の効果・状態での match（docs/grammar.md「exception / effect」「classify / condition」「配線」）
  state: path.join(root, 'tests', 'corpus', 'state', 'shop.hozo'),
};

let grammar;
const tokenized = new Map();

test.before(async () => {
  grammar = await loadGrammar();
});

function linesOf(file) {
  if (!tokenized.has(file)) tokenized.set(file, tokenize(grammar, fs.readFileSync(file, 'utf8')));
  return tokenized.get(file);
}

function tokensOf(source) {
  return tokenize(grammar, source);
}

/**
 * lineMatch（文字列か正規表現）に合う最初の行を探し、その行の text に一致する字句の scope（source.hozo を除く）を返す。
 * 字句が text と一致しないとき（分かれ方が違うとき）はその行の字句を添えて失敗する。
 */
function scopesAt(lines, lineMatch, text, nth = 0) {
  const line = lines.find((l) => (lineMatch instanceof RegExp ? lineMatch.test(l.text) : l.text.includes(lineMatch)));
  assert.ok(line, `行が見つからない: ${lineMatch}`);
  const hits = line.tokens.filter((t) => t.text === text);
  assert.ok(
    hits.length > nth,
    `字句 ${JSON.stringify(text)} が行にない: ${JSON.stringify(line.text)}\n` +
      line.tokens.map((t) => `${JSON.stringify(t.text)} → ${t.scopes.slice(1).join(' ')}`).join('\n'),
  );
  return hits[nth].scopes.slice(1);
}

function assertScope(lines, lineMatch, text, scope, nth) {
  const scopes = scopesAt(lines, lineMatch, text, nth);
  assert.ok(scopes.includes(scope), `${JSON.stringify(text)} の scope は ${scope} のはずが [${scopes.join(', ')}]`);
}

function assertNoScope(lines, lineMatch, text, scope) {
  const scopes = scopesAt(lines, lineMatch, text);
  assert.ok(!scopes.includes(scope), `${JSON.stringify(text)} に ${scope} が付いている`);
}

test('予約語（宣言の頭、節の頭、核の型、リテラル）', () => {
  const s = linesOf(files.server);
  assertScope(s, 'namespace server {', 'namespace', 'keyword.declaration.hozo');
  assertScope(s, 'model User {', 'model', 'keyword.declaration.hozo');
  assertScope(s, 'scalar Email extends string;', 'extends', 'keyword.control.hozo');
  assertScope(s, 'scalar Email extends string;', 'string', 'support.type.primitive.hozo');
  assertScope(s, 'state users: list<User>;', 'list', 'support.type.primitive.hozo');
  assertScope(s, 'throws RateLimited prevents effects;', 'prevents', 'keyword.control.hozo');
  assertScope(s, 'throws RateLimited prevents effects;', 'effects', 'keyword.control.hozo');
  const b = linesOf(files.browser);
  assertScope(b, 'bind token to server.*;', 'bind', 'keyword.declaration.hozo');
  assertScope(b, 'bind token to server.*;', 'to', 'keyword.control.hozo');
  assertScope(b, 'const line = server.addToCart', 'const', 'keyword.control.hozo');
  assertScope(b, 'const warranty = match item by WarrantyKind {', 'match', 'keyword.control.hozo');
  assertScope(b, 'const warranty = match item by WarrantyKind {', 'by', 'keyword.control.hozo');
  assertScope(b, 'NoWarranty => none;', 'none', 'constant.language.hozo');
  assertScope(b, 'default           => none;', 'default', 'keyword.control.hozo');
  assertScope(b, 'return plan;', 'return', 'keyword.control.hozo');
  const t = tokensOf('model X { a: boolean = true; b: boolean = false; }');
  assertScope(t, 'model X', 'true', 'constant.language.hozo');
  assertScope(t, 'model X', 'false', 'constant.language.hozo');
});

test('文脈キーワードは位置でだけ色が付く', () => {
  const h = linesOf(files.http);
  assertScope(h, 'decorator header(name: string) for field;', 'for', 'keyword.control.hozo');
  assertScope(h, 'decorator header(name: string) for field;', 'field', 'keyword.other.hozo');
  const v = linesOf(files.valid);
  assertScope(v, 'decorator min(value: int | float) for scalar | field;', 'field', 'keyword.other.hozo');
  assertScope(v, 'decorator min(value: int | float) for scalar | field;', 'scalar', 'keyword.declaration.hozo');
  const p = linesOf(files.page);
  assertScope(p, 'decorator interaction(page: any op, when: any variant);', 'any', 'keyword.control.hozo');
  assertScope(p, 'decorator interaction(page: any op, when: any variant);', 'variant', 'keyword.other.hozo');
  assertScope(p, 'decorator route(path: string, from: any state?', 'state', 'keyword.declaration.hozo');
  const s = linesOf(files.server);
  assertScope(s, 'effect createSession on sessions.post;', 'on', 'keyword.control.hozo');
  assertScope(s, 'effect createSession on sessions.post;', 'post', 'keyword.other.hozo');
  assertScope(s, 'effect deleteSession on sessions.delete;', 'delete', 'keyword.other.hozo');
  const b = linesOf(files.browser);
  assertScope(b, 'effect transitionToLogin on location.put;', 'put', 'keyword.other.hozo');
  // model Violation { field: string; } の field は識別子
  const violation = s.find((l) => l.text.includes('model Violation {'));
  assert.ok(!violation.tokens.some((t) => t.text === 'field' && t.scopes.includes('keyword.other.hozo')));
  // 値の位置の post / put / delete も識別子
  const t = tokensOf('op post(put: int) { delete(put); }');
  assert.ok(!t[0].tokens.some((tk) => tk.scopes.includes('keyword.other.hozo')));
});

test('デコレータと @external', () => {
  const s = linesOf(files.server);
  assertScope(s, '@http.post("/login")', '@http.post', 'entity.name.function.decorator.hozo');
  assertScope(s, '@validated', '@validated', 'entity.name.function.decorator.hozo');
  assertScope(s, '@db.key    id: UserId;', '@db.key', 'entity.name.function.decorator.hozo');
  const h = linesOf(files.http);
  assertScope(h, '@external', '@external', 'storage.modifier.hozo');
  assertNoScope(h, '@external', '@external', 'entity.name.function.decorator.hozo');
});

test('宣言の名前', () => {
  const s = linesOf(files.server);
  assertScope(s, 'model User {', 'User', 'entity.name.type.hozo');
  assertScope(s, 'scalar Email extends string;', 'Email', 'entity.name.type.hozo');
  assertScope(s, 'exception InvalidCredentials;', 'InvalidCredentials', 'entity.name.type.hozo');
  assertScope(s, 'op login(in: LoginInput): LoginOk', 'login', 'entity.name.function.hozo');
  assertScope(s, 'effect createSession on sessions.post;', 'createSession', 'entity.name.function.hozo');
  assertScope(s, 'decorator requiresAuth throws', 'requiresAuth', 'entity.name.function.hozo');
  assertScope(s, 'state users: list<User>;', 'users', 'variable.other.hozo');
  const b = linesOf(files.browser);
  assertScope(b, 'union ApiError {', 'ApiError', 'entity.name.type.hozo');
  assertScope(b, 'classify WarrantyKind of server.Item {', 'WarrantyKind', 'entity.name.type.hozo');
  assertScope(b, 'condition WarrantyAvailable of server.Item;', 'WarrantyAvailable', 'entity.name.type.hozo');
});

test('型の位置の名前', () => {
  const s = linesOf(files.server);
  assertScope(s, 'state users: list<User>;', 'User', 'entity.name.type.hozo');
  assertScope(s, '@db.key    id: UserId;', 'UserId', 'entity.name.type.hozo');
  assertScope(s, 'op login(in: LoginInput): LoginOk', 'LoginInput', 'entity.name.type.hozo');
  assertScope(s, 'op login(in: LoginInput): LoginOk', 'LoginOk', 'entity.name.type.hozo');
  assertScope(s, 'exception ClientError: Violations;', 'Violations', 'entity.name.type.hozo');
  assertScope(s, 'model PublicUser is User omit hash;', 'User', 'entity.name.type.hozo');
  assertScope(s, 'throws InvalidCredentials | NotVerified', 'NotVerified', 'entity.name.type.hozo');
  const b = linesOf(files.browser);
  assertScope(b, 'classify WarrantyKind of server.Item {', 'server.Item', 'entity.name.type.hozo');
  assertScope(b, 'catches http.NetworkError | http.ServerError ignores;', 'http.ServerError', 'entity.name.type.hozo');
  assertScope(b, 'state token: nullable<server.Token>;', 'server.Token', 'entity.name.type.hozo');
  // 状態の形 nullable も予約語（docs/grammar.md「予約語」の「核の型と状態の形」）
  assertScope(b, 'state token: nullable<server.Token>;', 'nullable', 'support.type.primitive.hozo');
  // match の by の後ろの状態の形も同じ予約語
  const st = linesOf(files.state);
  assertScope(st, 'match token by nullable {', 'nullable', 'support.type.primitive.hozo');
  assertScope(st, 'match carts by list {', 'list', 'support.type.primitive.hozo');
  assertScope(st, 'match carts by list {', 'by', 'keyword.control.hozo');
  assertScope(b, 'state query: single<Record<string, string[]>>;', 'Record', 'support.type.primitive.hozo');
  assertScope(b, 'state query: single<Record<string, string[]>>;', 'string', 'support.type.primitive.hozo', 1);
  assertScope(b, 'op chooseYamadaWarranty(item: WarrantyKind.Yamada<server.Item>', 'WarrantyKind.Yamada', 'entity.name.type.hozo');
  // 複数行にまたがる返り値の型と throws
  assertScope(b, /^\s*: server\.WarrantyLine$/, 'server.WarrantyLine', 'entity.name.type.hozo');
  assertScope(b, 'throws page.Aborted | http.NetworkError | http.ServerError', 'page.Aborted', 'entity.name.type.hozo');
  // 値の位置（レコードリテラル、名前付き引数）の小文字の名前は型にしない
  const orders = b.find((l) => l.text.includes('return server.getOrders({ page: query.page'));
  assert.ok(!orders.tokens.some((t) => t.scopes.includes('entity.name.type.hozo')));
  const route = b.find((l) => l.text.includes('@page.route("/orders", from: query, onInvalid: transitionToOrders)'));
  assert.ok(!route.tokens.some((t) => t.scopes.includes('entity.name.type.hozo')));
});

test('矢印（遷移の -> と match のアームの =>）', () => {
  const st = linesOf(files.state);
  // 遷移は `on X:` の後ろ（: の後ろの型の位置）に来る
  assertScope(st, 'effect placeOrder on Checkout: Shopping -> Ordered;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'effect startOver on Checkout: any -> Shopping;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'effect touchCheckout on Checkout: Shopping | Ordered -> any;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'effect clearCart on carts: HasItems -> Empty;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'Empty    => showLogin();', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'Shopping => submitOrder();', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'Ordered  => none;', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'default  => showLogin();', '=>', 'keyword.operator.arrow.hozo');
  const b = linesOf(files.browser);
  assertScope(b, 'NoWarranty => none;', '=>', 'keyword.operator.arrow.hozo');
  // 矢印の両側は今まで通り
  assertScope(st, 'effect startOver on Checkout: any -> Shopping;', 'any', 'keyword.control.hozo');
  assertScope(st, 'effect placeOrder on Checkout: Shopping -> Ordered;', 'on', 'keyword.control.hozo');
  assertScope(st, 'effect placeOrder on Checkout: Shopping -> Ordered;', 'placeOrder', 'entity.name.function.hozo');
  assertScope(st, 'Ordered  => none;', 'none', 'constant.language.hozo');
  // 比較の記号ではない（> は型引数の閉じ、= は既定値）
  const t = tokensOf('model X { a: int = 1; } state s: list<X>;');
  assert.ok(!t[0].tokens.some((tk) => tk.scopes.includes('keyword.operator.arrow.hozo')));
});

test('状態の形と、組み込みの分類のケース', () => {
  const st = linesOf(files.state);
  assertScope(st, 'state token: nullable<Token>;', 'nullable', 'support.type.primitive.hozo');
  assertScope(st, 'state title: single<string>;', 'single', 'support.type.primitive.hozo');
  assertScope(st, 'match token by nullable {', 'match', 'keyword.control.hozo');
  assertScope(st, 'match token by nullable {', 'by', 'keyword.control.hozo');
  assertScope(st, 'match by Checkout {', 'by', 'keyword.control.hozo');
  // 状態の分類の対象（of の後ろ）は小文字の名前なので型にしない
  assertScope(st, 'classify Checkout of carts, orders {', 'of', 'keyword.control.hozo');
  const of = st.find((l) => l.text.includes('classify Checkout of carts, orders {'));
  assert.ok(!of.tokens.some((tk) => /carts|orders/.test(tk.text) && tk.scopes.includes('entity.name.type.hozo')));
  // Empty / HasItems は予約語ではない（docs/grammar.md「予約語」）。match のアームでは色を付けない
  for (const [line, word] of [['Empty    => showLogin();', 'Empty'], ['HasItems => showCart();', 'HasItems']]) {
    const l = st.find((x) => x.text.includes(line));
    assert.ok(l, `行が見つからない: ${line}`);
    const tk = l.tokens.find((x) => x.text.includes(word));
    assert.ok(tk, `字句 ${word} が行にない`);
    assert.deepEqual(tk.scopes, ['source.hozo'], `${word} に scope が付いている: ${tk.scopes.join(' ')}`);
  }
  // 遷移のケースは : の後ろなので、ほかの大文字で始まる名前と同じく型の位置の名前になる（docs/editors.md の表の「型の位置の名前」）
  assertScope(st, 'effect clearCart on carts: HasItems -> Empty;', 'HasItems', 'entity.name.type.hozo');
  assertScope(st, 'effect clearCart on carts: HasItems -> Empty;', 'Empty', 'entity.name.type.hozo');
});

test('リテラル', () => {
  const s = linesOf(files.server);
  assertScope(s, '@http.header(name: "Authorization") token: Token?;', 'Authorization', 'string.quoted.double.hozo');
  assertScope(s, '@http.status(401)', '401', 'constant.numeric.hozo');
  assertScope(s, '@http.query page: int = 1;', '1', 'constant.numeric.hozo');
  const t = tokensOf('model X { a: Duration = 5m; b: Duration = 30s; c: Duration = 100ms; d: float = -3.5; e: string = "a\\n\\u{1F600}\\""; }');
  assertScope(t, 'model X', '5m', 'constant.numeric.hozo');
  assertScope(t, 'model X', '30s', 'constant.numeric.hozo');
  assertScope(t, 'model X', '100ms', 'constant.numeric.hozo');
  assertScope(t, 'model X', '-3.5', 'constant.numeric.hozo');
  assertScope(t, 'model X', '\\n', 'constant.character.escape.hozo');
  assertScope(t, 'model X', '\\u{1F600}', 'constant.character.escape.hozo');
  assertScope(t, 'model X', '\\"', 'constant.character.escape.hozo');
  // 識別子の中の数字は数値ではない
  const id = tokensOf('model X { a1: int; }');
  assert.ok(!id[0].tokens.some((tk) => tk.scopes.includes('constant.numeric.hozo')));
});

test('コメントとドキュメンテーションコメント', () => {
  const s = linesOf(files.server);
  assertScope(s, '// ---- §6.1 exception ----', '// ---- §6.1 exception ----', 'comment.line.double-slash.hozo');
  // 行の途中から始まる行コメント
  assertScope(s, 'throws RefundPending keeps effects;   //', '// 返金の処理は受け付けたが、完了の確認が取れない', 'comment.line.double-slash.hozo');
  assertScope(s, 'throws RefundPending keeps effects;   //', 'keeps', 'keyword.control.hozo');
  assertScope(s, '/** メールアドレス */', '/**', 'comment.block.documentation.hozo');
  assertScope(s, '/** メールアドレス */', ' メールアドレス ', 'comment.block.documentation.hozo');
  assertScope(s, '* @param email', '@param', 'storage.type.class.jsdoc');
  assertScope(s, '* @param email', 'email', 'variable.other.jsdoc');
  assertScope(s, '* @param email', '@param', 'comment.block.documentation.hozo');
  assertScope(s, '* @throws RateLimited', '@throws', 'storage.type.class.jsdoc');
  assertScope(s, '* @throws RateLimited', 'RateLimited', 'variable.other.jsdoc');
  assertScope(s, '* @see login', '@see', 'storage.type.class.jsdoc');
  assertScope(s, '* @see login', 'login', 'variable.other.jsdoc');
  // 複数行の doc comment の本文
  const b = linesOf(files.browser);
  assertScope(b, '* 商品に適用される保証の種類。', '   * 商品に適用される保証の種類。', 'comment.block.documentation.hozo');
  // ブロックコメントと、行の途中から始まる doc comment
  const t = tokensOf('/* a */ /**/ model X {} /** @param x */ op f(x: int);');
  assertScope(t, 'model X', '/*', 'comment.block.hozo');
  assertScope(t, 'model X', ' a ', 'comment.block.hozo');
  assertScope(t, 'model X', '@param', 'storage.type.class.jsdoc');
  assertScope(t, 'model X', 'model', 'keyword.declaration.hozo');
  // doc comment の中の予約語には色が付かない
  const d = tokensOf('/** model string */');
  assert.ok(d[0].tokens.every((tk) => tk.scopes.includes('comment.block.documentation.hozo')));
});

test('すべての実物の .hozo が落ちずに字句に分かれる', () => {
  for (const file of Object.values(files)) {
    const lines = linesOf(file);
    assert.equal(lines.length, fs.readFileSync(file, 'utf8').split(/\r?\n/).length);
    for (const line of lines) {
      for (const tk of line.tokens) assert.equal(tk.scopes[0], 'source.hozo');
    }
  }
  for (const f of fs.readdirSync(path.join(root, 'vocab')).filter((f) => f.endsWith('.hozo'))) {
    linesOf(path.join(root, 'vocab', f));
  }
});
