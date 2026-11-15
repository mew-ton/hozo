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
  // export、reads、extends / override、op の分類とケースの as、throw（docs/grammar.md「model」「classify / condition」「op」「配線」）
  sampleType: path.join(root, 'tests', 'samples', 'shop-server', 'type.hozo'),
  sampleStore: path.join(root, 'tests', 'samples', 'shop-server', 'store.hozo'),
  sampleServer: path.join(root, 'tests', 'samples', 'shop-server', 'server.hozo'),
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

test('フィールド名の位置の予約語は予約語として塗らない', () => {
  const prop = 'variable.other.property.hozo';
  const m = tokensOf('model Wrap { op: string; decorator: string; state: Target; default: boolean = false; }');
  for (const w of ['op', 'decorator', 'state', 'default']) {
    assertScope(m, 'model Wrap', w, prop);
    assertNoScope(m, 'model Wrap', w, 'keyword.declaration.hozo');
    assertNoScope(m, 'model Wrap', w, 'keyword.control.hozo');
  }
  // 型の位置の核の型と、宣言の頭はそのまま
  assertScope(m, 'model Wrap', 'model', 'keyword.declaration.hozo');
  assertScope(m, 'model Wrap', 'string', 'support.type.primitive.hozo');
  const f = tokensOf('  string: string;');
  assertScope(f, 'string: string;', 'string', prop, 0);
  assertScope(f, 'string: string;', 'string', 'support.type.primitive.hozo', 1);
  // レコードリテラルのキーと値のパス
  const r = tokensOf('const saved = store({ op: res.op, state: res.state, match: res.match });');
  for (const w of ['op', 'state', 'match']) {
    assertScope(r, 'const saved', w, prop, 0);
    assertScope(r, 'const saved', w, prop, 1);
  }
  const c = tokensOf('const name = label(res.state, res.op);');
  assertScope(c, 'const name', 'state', prop);
  assertScope(c, 'const name', 'op', prop);
  // override の後ろの名前（docs/grammar.md「model」の Override）もフィールド名の位置。核の型も、宣言の頭も、節の頭も塗らない
  const o = tokensOf('model Slim extends server.Wrap { override op; override to: never; @db.key override string; override model: Wide; override default: never; override hash: never; }');
  for (const w of ['op', 'to', 'string', 'model', 'default']) {
    assertScope(o, 'model Slim', w, prop, w === 'model' ? 1 : 0);
    for (const kw of ['keyword.declaration.hozo', 'keyword.control.hozo', 'support.type.primitive.hozo']) {
      assert.ok(!scopesAt(o, 'model Slim', w, w === 'model' ? 1 : 0).includes(kw), `override の後ろの ${w} が ${kw}`);
    }
  }
  // override そのものは修飾の予約語。元の model は型の位置の名前。予約語でないフィールド名には scope を付けない
  for (let n = 0; n < 6; n++) assertScope(o, 'model Slim', 'override', 'storage.modifier.hozo', n);
  assertScope(o, 'model Slim', 'extends', 'keyword.control.hozo');
  assertScope(o, 'model Slim', 'model', 'keyword.declaration.hozo', 0);
  assertScope(o, 'model Slim', 'server.Wrap', 'entity.name.type.hozo');
  assertScope(o, 'model Slim', 'Wide', 'entity.name.type.hozo');
  assertScope(o, 'model Slim', 'never', 'support.type.primitive.hozo', 0);
  assert.ok(!o[0].tokens.some((t) => t.text.includes('hash') && t.scopes.length > 1));
  // `override: string;` の override はフィールドの名前（: の前）。`override override: never;` は、その名前のフィールドへの Override
  const of = tokensOf('model M extends B { override: string; override override: never; }');
  assertScope(of, 'model M', 'override', prop, 0);
  assertScope(of, 'model M', 'override', 'storage.modifier.hozo', 1);
  assertScope(of, 'model M', 'override', prop, 2);
  // is と omit は予約語ではない（docs/grammar.md「予約語」）
  const io = tokensOf('op is(omit: string): string;');
  assert.ok(!io[0].tokens.some((t) => (t.text === 'is' || t.text === 'omit') && t.scopes.some((x) => x.startsWith('keyword.'))));
  // 実物: サンプルの override
  assertScope(linesOf(files.sampleType), 'override hash: never;', 'override', 'storage.modifier.hozo');
  assertScope(linesOf(files.sampleType), 'override hash: never;', 'never', 'support.type.primitive.hozo');
  // 宣言の頭と文の頭は予約語のまま
  const d = tokensOf('op run(): Wrap { return fetch(); }');
  assertScope(d, 'op run', 'op', 'keyword.declaration.hozo');
  assertScope(d, 'op run', 'return', 'keyword.control.hozo');
  const st = tokensOf('state token: nullable<Token>;');
  assertScope(st, 'state token', 'state', 'keyword.declaration.hozo');
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
  assertScope(linesOf(files.sampleType), 'model PublicUser extends User {', 'User', 'entity.name.type.hozo');
  assertScope(s, 'throws InvalidCredentials | NotVerified', 'NotVerified', 'entity.name.type.hozo');
  const b = linesOf(files.browser);
  assertScope(b, 'classify WarrantyKind of server.Item {', 'server.Item', 'entity.name.type.hozo');
  assertScope(b, 'catches http.NetworkError | http.ServerError ignores;', 'http.ServerError', 'entity.name.type.hozo');
  assertScope(b, 'state token: nullable<server.Token>;', 'server.Token', 'entity.name.type.hozo');
  // 形の語 nullable も予約語（docs/grammar.md「予約語」の「核の型と形」）
  assertScope(b, 'state token: nullable<server.Token>;', 'nullable', 'support.type.primitive.hozo');
  // match の by の後ろの標準の分類の名前も同じ予約語
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

test('修飾（export、override）と、reads / as / throw / never', () => {
  const store = linesOf(files.sampleStore);
  const server = linesOf(files.sampleServer);
  // export は修飾の予約語。op の名前は今まで通り
  assertScope(store, 'export op findUser(email: type.Email): type.User? reads users;', 'export', 'storage.modifier.hozo');
  assertScope(store, 'export op findUser(email: type.Email): type.User? reads users;', 'op', 'keyword.declaration.hozo');
  assertScope(store, 'export op findUser(email: type.Email): type.User? reads users;', 'findUser', 'entity.name.function.hozo');
  // reads は節の頭。後ろの状態の名前は小文字で始まるので scope を付けない
  assertScope(store, 'export op findUser(email: type.Email): type.User? reads users;', 'reads', 'keyword.control.hozo');
  assertScope(store, 'export op findUser(email: type.Email): type.User? reads users;', 'type.User', 'entity.name.type.hozo');
  const reads = store.find((l) => l.text.includes('export op findUser(email: type.Email): type.User? reads users;'));
  assert.ok(!reads.tokens.some((tk) => tk.text.includes('users') && tk.scopes.length > 1));
  // op の分類: of の後ろの op の名前と、as の前の op の名前は scope を付けない。as の後ろは型の位置
  assertScope(store, 'classify Registration of findUser {', 'of', 'keyword.control.hozo');
  assertScope(store, 'Registered: findUser as type.User;', 'as', 'keyword.control.hozo');
  assertScope(store, 'Registered: findUser as type.User;', 'type.User', 'entity.name.type.hozo');
  for (const line of ['classify Registration of findUser {', 'Registered: findUser as type.User;']) {
    const l = store.find((x) => x.text.includes(line));
    assert.ok(!l.tokens.some((tk) => tk.text.includes('findUser') && tk.scopes.length > 1), `${line} の findUser に scope が付いている`);
  }
  // 状態の分類の、対象ごとの as（, で続く）
  const as = tokensOf('classify Ready of token, profile { Ready: token as Token, profile as Profile; default; }');
  assertScope(as, 'classify Ready', 'as', 'keyword.control.hozo', 0);
  assertScope(as, 'classify Ready', 'as', 'keyword.control.hozo', 1);
  assertScope(as, 'classify Ready', 'Token', 'entity.name.type.hozo');
  assertScope(as, 'classify Ready', 'Profile', 'entity.name.type.hozo');
  assertScope(as, 'classify Ready', 'default', 'keyword.control.hozo');
  // match の by の後ろの、修飾した分類の名前
  assertScope(server, 'return match user by store.Registration {', 'by', 'keyword.control.hozo');
  assertScope(server, 'return match user by store.Registration {', 'store.Registration', 'entity.name.type.hozo');
  // アームの throw と、その例外
  assertScope(server, 'default    => throw type.InvalidCredentials;', 'throw', 'keyword.control.hozo');
  assertScope(server, 'default    => throw type.InvalidCredentials;', 'type.InvalidCredentials', 'entity.name.type.hozo');
  assertScope(server, 'default    => throw type.InvalidCredentials;', '=>', 'keyword.operator.arrow.hozo');
  // never は核の型と同じ scope
  const nv = tokensOf('op unauthorized(): never throws Unauthorized;');
  assertScope(nv, 'op unauthorized', 'never', 'support.type.primitive.hozo');
  assertScope(nv, 'op unauthorized', 'throws', 'keyword.control.hozo');
  assertScope(nv, 'op unauthorized', 'Unauthorized', 'entity.name.type.hozo');
  // 形の語は型の位置のどこでも同じ scope
  const sh = tokensOf('op f(xs: list<Item>, t: nullable<Token>): single<Item>;');
  assertScope(sh, 'op f', 'list', 'support.type.primitive.hozo');
  assertScope(sh, 'op f', 'nullable', 'support.type.primitive.hozo');
  assertScope(sh, 'op f', 'single', 'support.type.primitive.hozo');
  assertScope(sh, 'op f', 'Item', 'entity.name.type.hozo', 0);
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

test('形の語と、標準の分類のケース', () => {
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
