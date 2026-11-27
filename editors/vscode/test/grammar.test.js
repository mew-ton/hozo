// TextMate の文法を VS Code と同じ字句解析器（vscode-textmate + vscode-oniguruma）で動かし、
// 実物の .hozo（コーパス、標準の語彙、spec/cli.hozo）の選んだ字句が docs/editors.md「TextMate の文法」の
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
  client: path.join(root, 'tests', 'corpus', 'design', 'client.hozo'),
  http: path.join(root, 'vocab', 'http.hozo'),
  page: path.join(root, 'vocab', 'browser.hozo'),
  valid: path.join(root, 'vocab', 'valid.hozo'),
  cli: path.join(root, 'spec', 'cli.hozo'),
  // 状態の分類・遷移の効果・状態での match（docs/grammar.md「exception / effect / read」「classify」「配線」）
  state: path.join(root, 'tests', 'corpus', 'state', 'shop.hozo'),
  // export、reads、extends / override、op の分類とケースの as、throw、read の宣言、直に呼ぶ組み込みの効果（docs/grammar.md「model」「exception / effect / read」「classify」「op」「配線」）と、
  // 版 0.5 の構文（効果の宣言に付けたデコレータ、reads / effects の節、残りの引数、予約語のデコレータ名）
  api: path.join(root, 'tests', 'corpus', 'view', 'api.hozo'),
  // 予約語のデコレータ名の宣言（decorator list / int: 核の型と形の名前。delete: 文脈キーワード）と、対象 effect / read、核の印 @repeatable
  db: path.join(root, 'vocab', 'db.hozo'),
};

// サンプル（tests/samples/shop-server）の仕様は、落ちずに字句に分かれることだけを確かめる（行の中身は、サンプルの側が持ち主）。
const sampleDir = path.join(root, 'tests', 'samples', 'shop-server');

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
  assertScope(s, 'throws RateLimited prevents effects', 'prevents', 'keyword.control.hozo');
  assertScope(s, 'throws RateLimited prevents effects', 'effects', 'keyword.control.hozo');
  const b = linesOf(files.client);
  assertScope(b, 'bind token to server.*;', 'bind', 'keyword.declaration.hozo');
  assertScope(b, 'bind token to server.*;', 'to', 'keyword.control.hozo');
  assertScope(b, 'val line = server.addToCart', 'val', 'keyword.control.hozo');
  assertScope(b, 'val warranty = match (item by WarrantyKind) {', 'match', 'keyword.control.hozo');
  assertScope(b, 'val warranty = match (item by WarrantyKind) {', 'by', 'keyword.control.hozo');
  assertScope(b, 'NoWarranty => none;', 'none', 'constant.language.hozo');
  // 値の入れ物、disjoint、繰り返し、集める文（docs/grammar.md「配線」）。なくした const は、予約語として塗らない
  const w = tokensOf([
    'val user, orders;',
    'disjoint {',
    '  orders = store.listOrders(session.userId);',
    '}',
    'val summaries: OrderSummary[];',
    'disjoint for (val order of orders) {',
    '  summaries collect summarize(order);',
    '}',
    'const gone = 1;',
  ].join('\n'));
  assertScope(w, 'val user, orders;', 'val', 'keyword.control.hozo');
  assertScope(w, 'disjoint {', 'disjoint', 'keyword.control.hozo');
  assertScope(w, 'val summaries', 'OrderSummary', 'entity.name.type.hozo');
  for (const word of ['disjoint', 'for', 'val', 'of']) {
    assertScope(w, 'disjoint for', word, 'keyword.control.hozo');
  }
  assertScope(w, 'summaries collect', 'collect', 'keyword.control.hozo');
  const gone = w.find((l) => l.text.includes('const gone'));
  assert.ok(!gone.tokens.some((tk) => tk.text.includes('const') && tk.scopes.length > 1), 'なくした const に scope が付いている');
  // 対象のない分類と、ケースを対象にするデコレータ（docs/grammar.md「classify」「decorator の宣言」）
  const c = tokensOf([
    'classify Actor of never { @prd.actor.human Customer; }',
    'decorator human for case of Actor;',
    'decorator seen(what: any case of any) for model, case of Order;',
    'decorator fn by(who: any case of never);',
  ].join('\n'));
  assertScope(c, 'classify Actor', 'never', 'support.type.primitive.hozo');
  assertScope(c, 'classify Actor', '@prd.actor.human', 'entity.name.function.decorator.hozo');
  assertScope(c, 'decorator human', 'case', 'keyword.control.hozo');
  assertScope(c, 'decorator fn by', 'case', 'keyword.control.hozo');
  assertScope(c, 'decorator fn by', 'never', 'support.type.primitive.hozo');
  // 関数に付けるデコレータの頭（docs/grammar.md「decorator の宣言」）: decorator の直後の fn は、宣言の頭の語
  assertScope(c, 'decorator fn by', 'fn', 'keyword.declaration.hozo');
  assertScope(c, 'decorator fn by', 'by', 'entity.name.function.hozo');
  // of の後ろ（docs/grammar.md「decorator の宣言」の CaseKind）: any は節の語、型と分類の名前は、型の位置の名前
  assertScope(c, 'decorator human', 'of', 'keyword.control.hozo');
  assertScope(c, 'decorator human', 'Actor', 'entity.name.type.hozo');
  assertScope(c, 'decorator seen', 'any', 'keyword.control.hozo', 1);
  assertScope(c, 'decorator seen', 'Order', 'entity.name.type.hozo');
  assertScope(b, 'Unavailable => none;', 'none', 'constant.language.hozo');
  assertScope(b, 'return plan;', 'return', 'keyword.control.hozo');
  const t = tokensOf('@flags(true, off: false) model X { a: boolean; }');
  assertScope(t, 'model X', 'true', 'constant.language.hozo');
  assertScope(t, 'model X', 'false', 'constant.language.hozo');
});

test('文脈キーワードは位置でだけ色が付く', () => {
  const h = linesOf(files.http);
  assertScope(h, 'decorator header(name: string?) for field;', 'for', 'keyword.control.hozo');
  assertScope(h, 'decorator header(name: string?) for field;', 'field', 'keyword.other.hozo');
  const v = linesOf(files.valid);
  assertScope(v, 'decorator min(value: int | float) for scalar, field;', 'field', 'keyword.other.hozo');
  assertScope(v, 'decorator min(value: int | float) for scalar, field;', 'scalar', 'keyword.declaration.hozo');
  const p = linesOf(files.page);
  assertScope(p, 'decorator fn interaction(page: any op, when: any variant);', 'any', 'keyword.control.hozo');
  assertScope(p, 'decorator fn interaction(page: any op, when: any variant);', 'variant', 'keyword.other.hozo');
  assertScope(p, 'decorator fn route(path: string, from: any state?', 'state', 'keyword.declaration.hozo');
  const s = linesOf(files.server);
  assertScope(s, 'effect fn createSession(login: LoginOk): effects sessions.post;', 'effects', 'keyword.control.hozo');
  assertScope(s, 'effect fn createSession(login: LoginOk): effects sessions.post;', 'post', 'keyword.other.hozo');
  assertScope(s, 'effect fn deleteSession(token: Token?): effects sessions.delete;', 'delete', 'keyword.other.hozo');
  const b = linesOf(files.client);
  assertScope(b, 'effect fn transitionToLogin(): effects location.put;', 'put', 'keyword.other.hozo');
  // model Violation { field: string; } の field は識別子
  const violation = s.find((l) => l.text.includes('model Violation {'));
  assert.ok(!violation.tokens.some((t) => t.text === 'field' && t.scopes.includes('keyword.other.hozo')));
  // 値の位置の post / put / delete も識別子
  const t = tokensOf('fn post(put: int) { delete(put); }');
  assert.ok(!t[0].tokens.some((tk) => tk.scopes.includes('keyword.other.hozo')));
});

test('フィールド名の位置の予約語は予約語として塗らない', () => {
  const prop = 'variable.other.property.hozo';
  const m = tokensOf('model Wrap { op: string; decorator: string; state: Target; default: boolean; }');
  for (const w of ['op', 'decorator', 'state', 'default']) {
    assertScope(m, 'model Wrap', w, prop);
    assertNoScope(m, 'model Wrap', w, 'keyword.declaration.hozo');
    assertNoScope(m, 'model Wrap', w, 'keyword.control.hozo');
  }
  // 型の位置の核の型と、宣言の頭はそのまま
  assertScope(m, 'model Wrap', 'model', 'keyword.declaration.hozo');
  assertScope(m, 'model Wrap', 'string', 'support.type.primitive.hozo');
  // 核の型と形の名前は、予約語ではない（docs/grammar.md「予約語」）: フィールド名の位置では、ふつうの名前で、scope を付けない。
  // 型の位置（: の後ろ）でだけ、核の型として塗る
  const f = tokensOf('  string: string;');
  const strings = f[0].tokens.filter((t) => t.text.trim() === 'string');
  assert.strictEqual(strings.length, 2);
  assert.ok(!strings[0].scopes.includes('support.type.primitive.hozo') && !strings[0].scopes.includes(prop), `フィールド名の string の scope: ${strings[0].scopes}`);
  assert.ok(strings[1].scopes.includes('support.type.primitive.hozo'), `型の位置の string の scope: ${strings[1].scopes}`);
  // レコードリテラルのキーと値のパス
  const r = tokensOf('val saved = store({ op: res.op, state: res.state, match: res.match });');
  for (const w of ['op', 'state', 'match']) {
    assertScope(r, 'val saved', w, prop, 0);
    assertScope(r, 'val saved', w, prop, 1);
  }
  const c = tokensOf('const name = label(res.state, res.op);');
  assertScope(c, 'const name', 'state', prop);
  assertScope(c, 'const name', 'op', prop);
  // override の後ろの名前（docs/grammar.md「model」の Override）もフィールド名の位置。核の型も、宣言の頭も、節の頭も塗らない
  const o = tokensOf('model Slim extends server.Wrap { override op; override to: never; @db.key override string; override model: Wide; override default: never; override hash: never; }');
  // override の後ろの string は、予約語ではないので、ふつうのフィールド名（scope を付けない。核の型としても塗らない）
  assert.ok(!o[0].tokens.some((t) => t.text.trim() === 'string' && (t.scopes.includes('support.type.primitive.hozo') || t.scopes.includes(prop))));
  for (const w of ['op', 'to', 'model', 'default']) {
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
  const io = tokensOf('fn is(omit: string): string;');
  assert.ok(!io[0].tokens.some((t) => (t.text === 'is' || t.text === 'omit') && t.scopes.some((x) => x.startsWith('keyword.'))));
  // 実物: 設計書のコーパスの override
  assertScope(linesOf(files.server), 'override hash: never;', 'override', 'storage.modifier.hozo');
  assertScope(linesOf(files.server), 'override hash: never;', 'never', 'support.type.primitive.hozo');
  // 宣言の頭と文の頭は予約語のまま
  const d = tokensOf('fn run(): Wrap { return fetch(); }');
  assertScope(d, 'fn run', 'fn', 'keyword.declaration.hozo');
  // 関数の宣言の頭（docs/grammar.md「関数の宣言」）: fn と、その前の種類の語は、宣言の頭の予約語。名前は、fn の直後
  const fns = tokensOf('read fn byId(id: Id): Row? reads rows;\neffect fn add(row: Row): effects rows.post;\nexport fn save(row: Row);');
  for (const [line, kind] of [['read fn byId', 'read'], ['effect fn add', 'effect']]) {
    assertScope(fns, line, kind, 'keyword.declaration.hozo');
    assertScope(fns, line, 'fn', 'keyword.declaration.hozo');
  }
  assertScope(fns, 'read fn byId', 'byId', 'entity.name.function.hozo');
  assertScope(fns, 'effect fn add', 'add', 'entity.name.function.hozo');
  assertScope(fns, 'export fn save', 'export', 'storage.modifier.hozo');
  assertScope(fns, 'export fn save', 'save', 'entity.name.function.hozo');
  // フィールド名の位置の fn は、予約語として塗らない
  const fnField = tokensOf('model M { fn: string; }');
  assertScope(fnField, 'model M', 'fn', 'variable.other.property.hozo');
  assertScope(d, 'fn run', 'return', 'keyword.control.hozo');
  const st = tokensOf('state token: nullable<Token>;');
  assertScope(st, 'state token', 'state', 'keyword.declaration.hozo');
});

test('デコレータと、核の印', () => {
  const s = linesOf(files.server);
  assertScope(s, '@http.post("/login")', '@http.post', 'entity.name.function.decorator.hozo');
  assertScope(s, '@validated', '@validated', 'entity.name.function.decorator.hozo');
  assertScope(s, '@db.key    id: UserId;', '@db.key', 'entity.name.function.decorator.hozo');
  const h = linesOf(files.http);
  assertScope(h, '@entry', '@entry', 'storage.modifier.hozo');
  assertNoScope(h, '@entry', '@entry', 'entity.name.function.decorator.hozo');
  // outer は、節 outer throws の頭でだけ、文脈キーワード
  assertScope(h, 'decorator fn get(path: string): outer throws', 'outer', 'keyword.other.hozo');
  assertScope(h, 'decorator fn get(path: string): outer throws', 'throws', 'keyword.control.hozo');
  const o = tokensOf('fn outer(outer: int): int;');
  assertNoScope(o, 'fn outer', 'outer', 'keyword.other.hozo');
  const hint = tokensOf('@hint decorator tag(name: string) for model;');
  assertScope(hint, 'decorator tag', '@hint', 'storage.modifier.hozo');
});

test('宣言の名前', () => {
  const s = linesOf(files.server);
  assertScope(s, 'model User {', 'User', 'entity.name.type.hozo');
  assertScope(s, 'scalar Email extends string;', 'Email', 'entity.name.type.hozo');
  assertScope(s, 'exception InvalidCredentials;', 'InvalidCredentials', 'entity.name.type.hozo');
  assertScope(s, 'fn login(in: LoginInput): LoginOk', 'login', 'entity.name.function.hozo');
  assertScope(s, 'effect fn createSession(login: LoginOk): effects sessions.post;', 'createSession', 'entity.name.function.hozo');
  assertScope(s, 'decorator fn requiresAuth(): throws', 'requiresAuth', 'entity.name.function.hozo');
  assertScope(s, 'state users: list<User>;', 'users', 'variable.other.hozo');
  const b = linesOf(files.client);
  assertScope(b, 'union ApiError {', 'ApiError', 'entity.name.type.hozo');
  assertScope(b, 'classify WarrantyKind of server.Item {', 'WarrantyKind', 'entity.name.type.hozo');
  assertScope(b, 'classify WarrantyAvailable of server.Item { Available; Unavailable; }', 'WarrantyAvailable', 'entity.name.type.hozo');
  // condition は予約語ではない（docs/grammar.md「予約語」）: 宣言の頭としても、名前としても、塗らない
  const cond = tokensOf('fn check(condition: string): string;');
  assert.ok(!cond[0].tokens.some((tk) => tk.text.includes('condition') && tk.scopes.some((sc) => sc.startsWith('keyword'))), 'condition が予約語として塗られている');
});

test('型の位置の名前', () => {
  const s = linesOf(files.server);
  assertScope(s, 'state users: list<User>;', 'User', 'entity.name.type.hozo');
  assertScope(s, '@db.key    id: UserId;', 'UserId', 'entity.name.type.hozo');
  assertScope(s, 'fn login(in: LoginInput): LoginOk', 'LoginInput', 'entity.name.type.hozo');
  assertScope(s, 'fn login(in: LoginInput): LoginOk', 'LoginOk', 'entity.name.type.hozo');
  assertScope(s, 'exception ClientError(value: Violations);', 'Violations', 'entity.name.type.hozo');
  assertScope(s, 'model PublicUser extends User {', 'User', 'entity.name.type.hozo');
  assertScope(s, 'throws InvalidCredentials, NotVerified', 'NotVerified', 'entity.name.type.hozo');
  // 例外の宣言の名前と、引数の名前（引数名には scope を付けない）
  assertScope(s, 'exception ClientError(value: Violations);', 'ClientError', 'entity.name.type.hozo');
  const b = linesOf(files.client);
  assertScope(b, 'classify WarrantyKind of server.Item {', 'server.Item', 'entity.name.type.hozo');
  assertScope(b, 'catches http.NetworkError, http.ServerError ignores;', 'http.ServerError', 'entity.name.type.hozo');
  assertScope(b, 'state token: nullable<server.Token>;', 'server.Token', 'entity.name.type.hozo');
  // 形の語 nullable も予約語（docs/grammar.md「予約語」の「核の型と形」）
  assertScope(b, 'state token: nullable<server.Token>;', 'nullable', 'support.type.primitive.hozo');
  // match の by の後ろの標準の分類の名前も同じ予約語
  const st = linesOf(files.state);
  assertScope(st, 'match (token by nullable) {', 'nullable', 'support.type.primitive.hozo');
  assertScope(st, 'match (carts by list) {', 'list', 'support.type.primitive.hozo');
  assertScope(st, 'match (carts by list) {', 'by', 'keyword.control.hozo');
  assertScope(b, 'state query: single<browser.page.Query>;', 'single', 'support.type.primitive.hozo');
  assertScope(b, 'state query: single<browser.page.Query>;', 'browser.page.Query', 'entity.name.type.hozo');
  // インデックスの model（docs/grammar.md「model」）: キーと値の型は、型の位置。Record は、予約語ではなくなった（ふつうの名前）
  const ix = tokensOf('model Attrs { [name: string]: Item[]; }\nmodel Holder { attrs: Record; }');
  assertScope(ix, 'model Attrs', 'model', 'keyword.declaration.hozo');
  assertScope(ix, 'model Attrs', 'Attrs', 'entity.name.type.hozo');
  assertScope(ix, 'model Attrs', 'string', 'support.type.primitive.hozo');
  assertScope(ix, 'model Attrs', 'Item', 'entity.name.type.hozo');
  assertScope(ix, 'model Holder', 'Record', 'entity.name.type.hozo');
  assertScope(b, 'fn chooseYamadaWarranty(item: server.Item on WarrantyKind.Yamada', 'WarrantyKind.Yamada', 'entity.name.type.hozo');
  // 複数行にまたがる返り値の型と throws
  assertScope(b, /^\s*: server\.WarrantyLine$/, 'server.WarrantyLine', 'entity.name.type.hozo');
  assertScope(b, 'throws browser.page.Aborted, http.NetworkError, http.ServerError', 'browser.page.Aborted', 'entity.name.type.hozo');
  // 値の位置（レコードリテラル、名前付き引数）の小文字の名前は型にしない
  const orders = b.find((l) => l.text.includes('return server.getOrders({ page: query.page'));
  assert.ok(!orders.tokens.some((t) => t.scopes.includes('entity.name.type.hozo')));
  const route = b.find((l) => l.text.includes('@browser.page.route("/orders", from: query, onInvalid: transitionToOrders)'));
  assert.ok(!route.tokens.some((t) => t.scopes.includes('entity.name.type.hozo')));
});

test('修飾（export、override）と、reads / as / throw / never', () => {
  const api = linesOf(files.api);
  const findUser = 'export fn findUser(id: UserId): User? reads users, sessionsOf reads users {';
  // export は修飾の予約語。op の名前は今まで通り
  assertScope(api, findUser, 'export', 'storage.modifier.hozo');
  assertScope(api, findUser, 'fn', 'keyword.declaration.hozo');
  assertScope(api, findUser, 'findUser', 'entity.name.function.hozo');
  // reads は節の頭。後ろの状態の名前と read の名前は小文字で始まるので scope を付けない
  assertScope(api, findUser, 'reads', 'keyword.control.hozo', 0);
  assertScope(api, findUser, 'reads', 'keyword.control.hozo', 1);
  assertScope(api, findUser, 'User', 'entity.name.type.hozo');
  const reads = api.find((l) => l.text.includes(findUser));
  assert.ok(!reads.tokens.some((tk) => /users|sessionsOf/.test(tk.text) && tk.scopes.length > 1));
  // op の分類: of の後ろの op の名前と、as の前の op の名前は scope を付けない。as の後ろは型の位置
  assertScope(api, 'classify Lookup of findUser {', 'of', 'keyword.control.hozo');
  assertScope(api, 'Found: findUser as User;', 'as', 'keyword.control.hozo');
  assertScope(api, 'Found: findUser as User;', 'User', 'entity.name.type.hozo');
  for (const line of ['classify Lookup of findUser {', 'Found: findUser as User;']) {
    const l = api.find((x) => x.text.includes(line));
    assert.ok(!l.tokens.some((tk) => tk.text.includes('findUser') && tk.scopes.length > 1), `${line} の findUser に scope が付いている`);
  }
  // 状態の分類の、対象ごとの as（, で続く）
  const as = tokensOf('classify Ready of token, profile { Ready: token as Token, profile as Profile; Neither; }');
  assertScope(as, 'classify Ready', 'as', 'keyword.control.hozo', 0);
  assertScope(as, 'classify Ready', 'as', 'keyword.control.hozo', 1);
  assertScope(as, 'classify Ready', 'Token', 'entity.name.type.hozo');
  assertScope(as, 'classify Ready', 'Profile', 'entity.name.type.hozo');
  // default は、どの生成規則も使わない予約語で、予約語として塗らない（docs/editors.md「TextMate の文法」）
  const old = tokensOf('classify Old of T { A; default; }');
  assert.ok(!old[0].tokens.some((tk) => tk.text.includes('default') && tk.scopes.some((sc) => sc.startsWith('keyword'))), 'default が予約語として塗られている');
  // 残りのケース: as の後ろの else は予約語で、型の名前としては塗らない（docs/grammar.md「予約語」「classify」）
  assertScope(api, 'Stranger: findUser as else;', 'as', 'keyword.control.hozo');
  assertScope(api, 'Stranger: findUser as else;', 'else', 'keyword.control.hozo');
  const rest = api.find((x) => x.text.includes('Stranger: findUser as else;'));
  assert.ok(!rest.tokens.some((tk) => tk.text.includes('findUser') && tk.scopes.length > 1), '残りのケースの findUser に scope が付いている');
  // フィールド名の位置の else は、予約語として塗らない
  const field = tokensOf('model M { else: string; }');
  assertScope(field, 'model M', 'else', 'variable.other.property.hozo');
  // match の by の後ろの、修飾した分類の名前
  const by = tokensOf('    return match (user by store.Registration) {');
  assertScope(by, 'return match', 'by', 'keyword.control.hozo');
  assertScope(by, 'return match', 'store.Registration', 'entity.name.type.hozo');
  // アームの throw と、その例外
  const th = tokensOf('      Unknown    => throw type.InvalidCredentials;');
  assertScope(th, 'Unknown', 'throw', 'keyword.control.hozo');
  assertScope(th, 'Unknown', 'type.InvalidCredentials', 'entity.name.type.hozo');
  assertScope(th, 'Unknown', '=>', 'keyword.operator.arrow.hozo');
  // else のアーム: アームの頭の else は予約語（docs/grammar.md「配線」の ElseArm）。本体は、ほかのアームと同じ
  // 例外を受ける文（docs/grammar.md「配線」の Try）: try と catch は、文の頭の予約語。catch (e by 分類) の by も
  const tr = tokensOf('    try {');
  assertScope(tr, 'try {', 'try', 'keyword.control.hozo');
  const ca = tokensOf('    } catch (e by payment.PrepayFailure) {');
  assertScope(ca, 'catch (e by', 'catch', 'keyword.control.hozo');
  assertScope(ca, 'catch (e by', 'by', 'keyword.control.hozo');
  const el = tokensOf('      else    => throw type.InvalidCredentials;');
  assertScope(el, 'else', 'else', 'keyword.control.hozo');
  assertScope(el, 'else', 'throw', 'keyword.control.hozo');
  assertScope(el, 'else', 'type.InvalidCredentials', 'entity.name.type.hozo');
  assertScope(el, 'else', '=>', 'keyword.operator.arrow.hozo');
  // 実物: コーパスの else のアームと、ケースを , で並べたアーム（ケースの名前は予約語ではなく、専用の scope を付けない）
  assertScope(api, 'else  => throw NotFound;', 'else', 'keyword.control.hozo');
  assertScope(api, 'else  => throw NotFound;', 'throw', 'keyword.control.hozo');
  assertScope(api, 'else  => throw NotFound;', 'NotFound', 'entity.name.type.hozo');
  assertScope(api, 'Idle, Fresh => "new or idle";', '=>', 'keyword.operator.arrow.hozo');
  assertScope(api, 'Idle, Fresh => "new or idle";', 'new or idle', 'string.quoted.double.hozo');
  const listed = api.find((x) => x.text.includes('Idle, Fresh => "new or idle";'));
  assert.ok(!listed.tokens.some((tk) => /Idle|Fresh/.test(tk.text) && tk.scopes.length > 1), 'アームのケースの名前に scope が付いている');
  // never は核の型と同じ scope
  const nv = tokensOf('fn unauthorized(): never throws Unauthorized;');
  assertScope(nv, 'fn unauthorized', 'never', 'support.type.primitive.hozo');
  assertScope(nv, 'fn unauthorized', 'throws', 'keyword.control.hozo');
  assertScope(nv, 'fn unauthorized', 'Unauthorized', 'entity.name.type.hozo');
  // 形の語は型の位置のどこでも同じ scope
  const sh = tokensOf('fn f(xs: list<Item>, t: nullable<Token>): single<Item>;');
  assertScope(sh, 'fn f', 'list', 'support.type.primitive.hozo');
  assertScope(sh, 'fn f', 'nullable', 'support.type.primitive.hozo');
  assertScope(sh, 'fn f', 'single', 'support.type.primitive.hozo');
  assertScope(sh, 'fn f', 'Item', 'entity.name.type.hozo', 0);
});

test('矢印（遷移の -> と match のアームの =>）', () => {
  const st = linesOf(files.state);
  // 遷移は `effects X as` の後ろ（as の後ろの型の位置）に来る
  assertScope(st, 'effect fn placeOrder(): effects Checkout as Shopping -> Ordered;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'effect fn startOver(): effects Checkout as any -> Shopping;', '->', 'keyword.operator.arrow.hozo');
  // 遷移のケースの並びの区切りは ,（docs/grammar.md「exception / effect / read」の CaseSet）
  assertScope(st, 'effect fn touchCheckout(): effects Checkout as Shopping, Ordered -> any;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'effect fn touchCheckout(): effects Checkout as Shopping, Ordered -> any;', 'any', 'keyword.control.hozo');
  assertScope(st, 'effect fn clearCart(): effects carts as HasItems -> Empty;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'None => showLogin();', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'Shopping => submitOrder();', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'Ordered  => none;', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'else     => showLogin();', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'else     => showLogin();', 'else', 'keyword.control.hozo');
  const b = linesOf(files.client);
  assertScope(b, 'NoWarranty => none;', '=>', 'keyword.operator.arrow.hozo');
  // 矢印の両側は今まで通り
  assertScope(st, 'effect fn startOver(): effects Checkout as any -> Shopping;', 'any', 'keyword.control.hozo');
  assertScope(st, 'effect fn placeOrder(): effects Checkout as Shopping -> Ordered;', 'effects', 'keyword.control.hozo');
  assertScope(st, 'effect fn placeOrder(): effects Checkout as Shopping -> Ordered;', 'as', 'keyword.control.hozo');
  assertScope(st, 'effect fn placeOrder(): effects Checkout as Shopping -> Ordered;', 'placeOrder', 'entity.name.function.hozo');
  assertScope(st, 'Ordered  => none;', 'none', 'constant.language.hozo');
  // 比較の記号ではない（> は型引数の閉じ、= は名前付きのデコレータ）
  const t = tokensOf('decorator once = retry(1); state s: list<X>;');
  assert.ok(!t[0].tokens.some((tk) => tk.scopes.includes('keyword.operator.arrow.hozo')));
});

test('行の遷移（effect fn …: effects 状態.操作 as …）と、絞り込んだ型（T on C.K）と、ケースの as none', () => {
  // docs/grammar.md「exception / effect / read」の RowClause: as の後ろは、効果の後のケース一つ（型の位置の名前）か、else（予約語）。
  // 同「型」の OnType: on は予約語、後ろのケース（分類の名前とケースの名前）は型の位置の名前、| には scope を付けない
  // （docs/editors.md「TextMate の文法」）
  const put = tokensOf('  effect fn cancel(order: Order on Stage.Draft | kind.Stage.Accepted): effects orders.put as Stage.Cancelled;');
  assertScope(put, 'effect fn cancel', 'effect', 'keyword.declaration.hozo');
  assertScope(put, 'effect fn cancel', 'cancel', 'entity.name.function.hozo');
  assertScope(put, 'effect fn cancel', 'Order', 'entity.name.type.hozo');
  assertScope(put, 'effect fn cancel', 'on', 'keyword.control.hozo');
  assertScope(put, 'effect fn cancel', 'Stage.Draft', 'entity.name.type.hozo');
  assertScope(put, 'effect fn cancel', 'kind.Stage.Accepted', 'entity.name.type.hozo');
  assert.ok(!put[0].tokens.some((tk) => tk.text.includes('|') && tk.scopes.length > 1), '| に scope が付いている');
  assertScope(put, 'effect fn cancel', 'effects', 'keyword.control.hozo');
  assertScope(put, 'effect fn cancel', 'put', 'keyword.other.hozo');
  assertScope(put, 'effect fn cancel', 'as', 'keyword.control.hozo');
  assertScope(put, 'effect fn cancel', 'Stage.Cancelled', 'entity.name.type.hozo');
  // 後のケースを約束しない else
  const hatch = tokensOf('effect fn patch(order: Order): effects orders.put as else;');
  assertScope(hatch, 'effect fn patch', 'as', 'keyword.control.hozo');
  assertScope(hatch, 'effect fn patch', 'else', 'keyword.control.hozo');
  const post = tokensOf('effect fn accept(order: Order): effects orders.post as Stage.Accepted;\neffect fn bring(order: Order): effects orders.post;');
  assertScope(post, 'effect fn accept', 'post', 'keyword.other.hozo');
  assertScope(post, 'effect fn accept', 'Stage.Accepted', 'entity.name.type.hozo');
  assertScope(post, 'effect fn bring', 'post', 'keyword.other.hozo');
  // 行の遷移は ; で終わる: 次の宣言は、今まで通り
  const next = tokensOf('effect fn a(o: O on Life.Open): effects s.delete as Life.Absent;\nstate rows: list<Row>;');
  assertScope(next, 'effect fn a', 'on', 'keyword.control.hozo');
  assertScope(next, 'effect fn a', 'Life.Open', 'entity.name.type.hozo');
  assertScope(next, 'effect fn a', 'delete', 'keyword.other.hozo');
  assertScope(next, 'effect fn a', 'Life.Absent', 'entity.name.type.hozo');
  assertScope(next, 'state rows', 'state', 'keyword.declaration.hozo');
  assertScope(next, 'state rows', 'rows', 'variable.other.hozo');
  // 絞り込んだ型は、< > の中、戻り値、val、ケースの as の後ろにも書ける
  const on = tokensOf('fn pick(orders: list<Order on Stage.Draft>): Order on Stage.Accepted {\n  val first: Order on Stage.Draft | Stage.Accepted = head(orders);\n}\nclassify Check of Order { Passed: Order as Order on Stage.Accepted; Failed; }');
  assertScope(on, 'fn pick', 'on', 'keyword.control.hozo', 0);
  assertScope(on, 'fn pick', 'Stage.Draft', 'entity.name.type.hozo');
  assertScope(on, 'fn pick', 'on', 'keyword.control.hozo', 1);
  assertScope(on, 'fn pick', 'Stage.Accepted', 'entity.name.type.hozo');
  assertScope(on, 'val first', 'on', 'keyword.control.hozo');
  assertScope(on, 'val first', 'Stage.Accepted', 'entity.name.type.hozo');
  assertScope(on, 'classify Check', 'on', 'keyword.control.hozo');
  assertScope(on, 'classify Check', 'Stage.Accepted', 'entity.name.type.hozo');
  // ない側のケース: as の後ろの none は、値の none と同じ scope（docs/grammar.md「classify」）
  const none = tokensOf('classify Life of Order? { Open: Order? as Order; Absent: Order? as none; }');
  assertScope(none, 'classify Life', 'as', 'keyword.control.hozo', 1);
  assertScope(none, 'classify Life', 'none', 'constant.language.hozo');
  assertScope(none, 'classify Life', 'Order', 'entity.name.type.hozo', 3);
  // 実物: コーパスの行の遷移と、ない側のケース
  const api = linesOf(files.api);
  assertScope(api, 'effect fn rest(user: User on Standing.Fresh | Standing.Active, since: datetime): effects users.put as Standing.Idle;', 'Standing.Active', 'entity.name.type.hozo');
  assertScope(api, 'effect fn rest(user: User on Standing.Fresh | Standing.Active, since: datetime): effects users.put as Standing.Idle;', 'on', 'keyword.control.hozo');
  assertScope(api, 'effect fn rest(user: User on Standing.Fresh | Standing.Active, since: datetime): effects users.put as Standing.Idle;', 'Standing.Idle', 'entity.name.type.hozo');
  assertScope(api, 'effect fn amend(user: User on Standing.Idle): effects users.put as else;', 'else', 'keyword.control.hozo');
  assertScope(api, 'effect fn closeUser(user: User): effects users.put as Standing.Closed;', 'Standing.Closed', 'entity.name.type.hozo');
  assertScope(api, 'Unknown: User? as none;', 'none', 'constant.language.hozo');
});

test('形の語と、標準の分類のケース', () => {
  const st = linesOf(files.state);
  assertScope(st, 'state token: nullable<Token>;', 'nullable', 'support.type.primitive.hozo');
  assertScope(st, 'state title: single<string>;', 'single', 'support.type.primitive.hozo');
  assertScope(st, 'match (token by nullable) {', 'match', 'keyword.control.hozo');
  assertScope(st, 'match (token by nullable) {', 'by', 'keyword.control.hozo');
  assertScope(st, 'match (by Checkout) {', 'by', 'keyword.control.hozo');
  // 状態の分類の対象（of の後ろ）は小文字の名前なので型にしない
  assertScope(st, 'classify Checkout of carts, orders {', 'of', 'keyword.control.hozo');
  const of = st.find((l) => l.text.includes('classify Checkout of carts, orders {'));
  assert.ok(!of.tokens.some((tk) => /carts|orders/.test(tk.text) && tk.scopes.includes('entity.name.type.hozo')));
  // 標準の分類のケース（None / Some、Empty / HasItems）は予約語ではない（docs/grammar.md「予約語」）。match のアームでは色を付けない
  for (const [line, word] of [['None => showLogin();', 'None'], ['Some => showCart();', 'Some'], ['Empty    => showEmpty();', 'Empty']]) {
    const l = st.find((x) => x.text.includes(line));
    assert.ok(l, `行が見つからない: ${line}`);
    const tk = l.tokens.find((x) => x.text.includes(word));
    assert.ok(tk, `字句 ${word} が行にない`);
    assert.deepEqual(tk.scopes, ['source.hozo'], `${word} に scope が付いている: ${tk.scopes.join(' ')}`);
  }
  // 遷移のケースは as の後ろなので、ほかの大文字で始まる名前と同じく型の位置の名前になる（docs/editors.md の表の「型の位置の名前」）
  assertScope(st, 'effect fn clearCart(): effects carts as HasItems -> Empty;', 'HasItems', 'entity.name.type.hozo');
  assertScope(st, 'effect fn clearCart(): effects carts as HasItems -> Empty;', 'Empty', 'entity.name.type.hozo');
});

test('リテラル', () => {
  const s = linesOf(files.server);
  assertScope(s, '@http.header(name: "Authorization") token: Token?;', 'Authorization', 'string.quoted.double.hozo');
  assertScope(s, '@http.status(401)', '401', 'constant.numeric.hozo');
  assertScope(s, '@http.default(1) @http.query page: int;', '1', 'constant.numeric.hozo');
  const t = tokensOf('@limits(5m, 30s, 100ms, -3.5, "a\\n\\u{1F600}\\"") model X { a: duration; }');
  assertScope(t, 'model X', '5m', 'constant.numeric.hozo');
  assertScope(t, 'model X', '30s', 'constant.numeric.hozo');
  assertScope(t, 'model X', '100ms', 'constant.numeric.hozo');
  assertScope(t, 'model X', '-3.5', 'constant.numeric.hozo');
  assertScope(t, 'model X', '\\n', 'constant.character.escape.hozo');
  assertScope(t, 'model X', '\\u{1F600}', 'constant.character.escape.hozo');
  assertScope(t, 'model X', '\\"', 'constant.character.escape.hozo');
  // 複数行の文字列（docs/grammar.md「リテラル」の TextBlock）。中の " と "" は閉じない。エスケープは同じ scope
  const tb = tokensOf('@adr.context("""\n  say "hi" ""x""\\t\n  model Y\n  """) model X { a: int; }');
  assertScope(tb, 'say', '  say "hi" ""x""', 'string.quoted.triple.hozo');
  assertScope(tb, 'say', '\\t', 'constant.character.escape.hozo');
  assertScope(tb, '  model Y', '  model Y', 'string.quoted.triple.hozo');
  assertScope(tb, 'model X', 'model', 'keyword.declaration.hozo');
  // 識別子の中の数字は数値ではない
  const id = tokensOf('model X { a1: int; }');
  assert.ok(!id[0].tokens.some((tk) => tk.scopes.includes('constant.numeric.hozo')));
});

test('デコレータの対象 effect、reads / effects の節、残りの引数', () => {
  const d = linesOf(files.db);
  // 効果の宣言に付けるデコレータの宣言: for の後ろの effect は、宣言の頭と同じ予約語
  const insert = 'decorator insert for effect;';
  assertScope(d, insert, 'decorator', 'keyword.declaration.hozo');
  assertScope(d, insert, 'insert', 'entity.name.function.hozo');
  assertScope(d, insert, 'for', 'keyword.control.hozo');
  assertScope(d, insert, 'effect', 'keyword.declaration.hozo');
  const into = 'decorator into(table: any state) for effect;';
  const api = linesOf(files.api);
  assertScope(api, into, 'any', 'keyword.control.hozo');
  assertScope(api, into, 'state', 'keyword.declaration.hozo');
  assertScope(api, into, 'effect', 'keyword.declaration.hozo');
  // デコレータの宣言の reads の節: reads は節の頭。後ろの引数名には scope を付けない
  const find = 'decorator fn find(table: any state, column: string?): reads table;';
  assertScope(api, find, 'reads', 'keyword.control.hozo');
  assertScope(api, find, 'fn', 'keyword.declaration.hozo');
  assertScope(api, find, 'string', 'support.type.primitive.hozo');
  const findLine = api.find((l) => l.text.includes(find));
  assert.ok(!findLine.tokens.some((tk) => tk.text.trim() === 'table' && tk.scopes.length > 1), 'reads の後ろの引数名に scope が付いている');
  assert.ok(!findLine.tokens.some((tk) => tk.scopes.includes('entity.name.function.hozo') && tk.text !== 'find'), 'デコレータの名前のほかが、宣言の名前になっている');
  // 残りの引数（...）と、effects の節
  const effects = 'decorator fn effects(...targets: any effect): effects targets reads sessions;';
  assertScope(api, effects, '...', 'keyword.operator.rest.hozo');
  assertScope(api, effects, 'any', 'keyword.control.hozo');
  assertScope(api, effects, 'effect', 'keyword.declaration.hozo');
  assertScope(api, effects, 'fn', 'keyword.declaration.hozo');
  // 一つ目の effects は宣言の名前（予約語として塗らない）、二つ目は節の頭。その後ろに reads の節が続く
  assertScope(api, effects, 'effects', 'entity.name.function.hozo', 0);
  assertNoScope(api, effects, 'effects', 'keyword.control.hozo');
  assertScope(api, effects, 'effects', 'keyword.control.hozo', 1);
  assertScope(api, effects, 'reads', 'keyword.control.hozo');
  const effectsLine = api.find((l) => l.text.includes(effects));
  assert.ok(!effectsLine.tokens.some((tk) => tk.text.trim() === 'targets' && tk.scopes.length > 1), '引数名 targets に scope が付いている');
  // for を書かない宣言の節、ほかの引数の後ろの残りの引数、節が続く形
  const t = tokensOf('decorator fn both(extra: any state?, ...more: any effect): reads logs, extra throws Busy, Late prevents effects effects more, add;');
  assertScope(t, 'decorator fn both', '...', 'keyword.operator.rest.hozo');
  assertScope(t, 'decorator fn both', 'reads', 'keyword.control.hozo');
  assertScope(t, 'decorator fn both', 'throws', 'keyword.control.hozo');
  assertScope(t, 'decorator fn both', 'Busy', 'entity.name.type.hozo');
  assertScope(t, 'decorator fn both', 'Late', 'entity.name.type.hozo');
  assertScope(t, 'decorator fn both', 'prevents', 'keyword.control.hozo');
  assertScope(t, 'decorator fn both', 'effects', 'keyword.control.hozo', 0);
  assertScope(t, 'decorator fn both', 'effects', 'keyword.control.hozo', 1);
  // 効果の宣言に付けたデコレータ（実物: コーパス）
  const renameUser = 'effect fn renameUser(id: UserId, name: string): effects users.put;';
  assertScope(api, /^\s*@into\(users\)$/, '@into', 'entity.name.function.decorator.hozo');
  assertScope(api, renameUser, 'effect', 'keyword.declaration.hozo');
  assertScope(api, renameUser, 'renameUser', 'entity.name.function.hozo');
  assertScope(api, renameUser, 'UserId', 'entity.name.type.hozo');
  assertScope(api, renameUser, 'string', 'support.type.primitive.hozo');
  assertScope(api, renameUser, 'put', 'keyword.other.hozo');
  const e = tokensOf('@db.insert effect fn createOrder(order: Order): effects orders.post;');
  assertScope(e, 'effect fn createOrder', '@db.insert', 'entity.name.function.decorator.hozo');
  assertScope(e, 'effect fn createOrder', 'effect', 'keyword.declaration.hozo');
  assertScope(e, 'effect fn createOrder', 'createOrder', 'entity.name.function.hozo');
  assertScope(e, 'effect fn createOrder', 'effects', 'keyword.control.hozo');
  assertScope(e, 'effect fn createOrder', 'post', 'keyword.other.hozo');
});

test('関数の型（「)」の後ろの「:」から: 戻り値と節）', () => {
  // docs/grammar.md「関数の宣言」: 戻り値を省いて、節から始める形。: の後ろの型の位置は、節の頭の手前で終わる
  const t = tokensOf([
    'fn save(item: Item): throws Broken { note(item); }',
    'fn register(input: Input): User throws Taken reads users effects createUser { return make(input); }',
    'effect fn emptyCart(): effects cart as HasItems -> Empty;',
    'read fn allCarriers(): Carrier[] reads carriers;',
    'fn ping();',
  ].join('\n'));
  assertScope(t, 'fn save', 'throws', 'keyword.control.hozo');
  assertScope(t, 'fn save', 'Broken', 'entity.name.type.hozo');
  assertScope(t, 'fn save', 'save', 'entity.name.function.hozo');
  assertScope(t, 'fn register', 'User', 'entity.name.type.hozo');
  assertScope(t, 'fn register', 'Taken', 'entity.name.type.hozo');
  for (const word of ['throws', 'reads', 'effects']) {
    assertScope(t, 'fn register', word, 'keyword.control.hozo');
  }
  const reg = t.find((l) => l.text.includes('fn register'));
  assert.ok(!reg.tokens.some((tk) => /users|createUser/.test(tk.text) && tk.scopes.includes('entity.name.type.hozo')), '節の、小文字で始まる名前が、型になっている');
  assertScope(t, 'effect fn emptyCart', 'effects', 'keyword.control.hozo');
  assertScope(t, 'effect fn emptyCart', 'as', 'keyword.control.hozo');
  assertScope(t, 'effect fn emptyCart', 'HasItems', 'entity.name.type.hozo');
  assertScope(t, 'effect fn emptyCart', '->', 'keyword.operator.arrow.hozo');
  assertScope(t, 'effect fn emptyCart', 'Empty', 'entity.name.type.hozo');
  assertScope(t, 'read fn allCarriers', 'Carrier', 'entity.name.type.hozo');
  assertScope(t, 'read fn allCarriers', 'reads', 'keyword.control.hozo');
  assertScope(t, 'fn ping', 'ping', 'entity.name.function.hozo');
});

test('read の宣言、@repeatable、引数を持つ例外、直に呼ぶ組み込みの効果', () => {
  const api = linesOf(files.api);
  // read の宣言: read は宣言の頭、名前は op・effect と同じ scope。引数と結果は型の位置、reads の後ろの状態には scope を付けない
  const userById = 'read fn userById(id: UserId): User? reads users;';
  assertScope(api, userById, 'read', 'keyword.declaration.hozo');
  assertScope(api, userById, 'userById', 'entity.name.function.hozo');
  assertScope(api, userById, 'UserId', 'entity.name.type.hozo');
  assertScope(api, userById, 'User', 'entity.name.type.hozo');
  assertScope(api, userById, 'reads', 'keyword.control.hozo');
  const readLine = api.find((l) => l.text.includes(userById));
  assert.ok(!readLine.tokens.some((tk) => tk.text.trim() === 'users' && tk.scopes.length > 1), 'reads の後ろの状態に scope が付いている');
  // read に付けたデコレータ（何度でも書けるものを、二度）
  assertScope(api, '@db.where("revoked", false)', '@db.where', 'entity.name.function.decorator.hozo');
  assertScope(api, '@db.where("kind", "web")', '@db.where', 'entity.name.function.decorator.hozo');
  assertScope(api, '@db.where("revoked", false)', 'false', 'constant.language.hozo');
  // 対象 read と、核の印 @repeatable（デコレータではない）
  const d = linesOf(files.db);
  assertScope(d, '@repeatable', '@repeatable', 'storage.modifier.hozo');
  assertNoScope(d, '@repeatable', '@repeatable', 'entity.name.function.decorator.hozo');
  assertScope(d, 'decorator find for read;', 'read', 'keyword.declaration.hozo');
  assertScope(d, 'decorator find for read;', 'find', 'entity.name.function.hozo');
  const marks = tokensOf('@repeatable @entry decorator tag(name: string) for read, effect, exception;');
  assertScope(marks, 'decorator tag', '@repeatable', 'storage.modifier.hozo');
  assertScope(marks, 'decorator tag', '@entry', 'storage.modifier.hozo');
  assertScope(marks, 'decorator tag', 'read', 'keyword.declaration.hozo');
  assertScope(marks, 'decorator tag', 'effect', 'keyword.declaration.hozo');
  assertScope(marks, 'decorator tag', 'exception', 'keyword.declaration.hozo');
  // read は、フィールド名とデコレータ名の位置では予約語として塗らない
  const field = tokensOf('model Mail { read: boolean; }');
  assertScope(field, 'model Mail', 'read', 'variable.other.property.hozo');
  assertNoScope(field, 'model Mail', 'read', 'keyword.declaration.hozo');
  const deco = tokensOf('@db.read read fn latest(): Mail? reads mails;');
  assertScope(deco, 'read fn latest', '@db.read', 'entity.name.function.decorator.hozo');
  assertScope(deco, 'read fn latest', 'read', 'keyword.declaration.hozo');
  assertScope(deco, 'read fn latest', 'latest', 'entity.name.function.hozo');
  // 引数を持つ例外と、引数を渡す throw
  const ex = tokensOf('exception LimitExceeded(wait: duration, reason: string);');
  assertScope(ex, 'exception LimitExceeded', 'exception', 'keyword.declaration.hozo');
  assertScope(ex, 'exception LimitExceeded', 'LimitExceeded', 'entity.name.type.hozo');
  assertScope(ex, 'exception LimitExceeded', 'duration', 'support.type.primitive.hozo');
  // 日時の核の型（docs/grammar.md「型」）: datetime、date、time、duration は、どれも、核の型
  const dt = tokensOf('model Slot { at: datetime; on: date; from: time?; span: duration[]; }');
  for (const word of ['datetime', 'date', 'time', 'duration']) {
    assertScope(dt, 'model Slot', word, 'support.type.primitive.hozo');
  }
  assertScope(ex, 'exception LimitExceeded', 'string', 'support.type.primitive.hozo');
  const weak = 'Weak   => throw Invalid("weak password");';
  assertScope(api, weak, 'throw', 'keyword.control.hozo');
  assertScope(api, weak, 'Invalid', 'entity.name.type.hozo');
  assertScope(api, weak, 'weak password', 'string.quoted.double.hozo');
  // 直に呼ぶ組み込みの効果: . の後ろで、直後が ( の post / put / delete だけが文脈キーワード
  assertScope(api, 'sessions.post(session);', 'post', 'keyword.other.hozo');
  assertScope(api, 'api.current.put(token);', 'put', 'keyword.other.hozo');
  assertScope(api, 'current.delete();', 'delete', 'keyword.other.hozo');
  const path = tokensOf('const next = save(order.post, order.put);');
  assert.ok(!path[0].tokens.some((tk) => tk.scopes.includes('keyword.other.hozo')), '値のパスの post / put が文脈キーワードになっている');
  // 配線の op の effects の節に直に書いた「状態.操作」と、名前で書いた効果（名前には scope を付けない）
  const clause = tokensOf('fn signOut(): effects sessions.delete, api.current.put, dropAll reads users { sessions.delete(); }');
  assertScope(clause, 'fn signOut', 'effects', 'keyword.control.hozo');
  assertScope(clause, 'fn signOut', 'delete', 'keyword.other.hozo', 0);
  assertScope(clause, 'fn signOut', 'put', 'keyword.other.hozo');
  assertScope(clause, 'fn signOut', 'reads', 'keyword.control.hozo');
  assertScope(clause, 'fn signOut', 'delete', 'keyword.other.hozo', 1);
  assert.ok(!clause[0].tokens.some((tk) => /dropAll|users/.test(tk.text) && tk.scopes.length > 1), '節の名前に scope が付いている');
});

test('核の型と形の名前は、型の位置でだけ塗る（docs/grammar.md「予約語」「型」）', () => {
  const prim = 'support.type.primitive.hozo';
  const primCount = (line, word) => line.tokens.filter((t) => t.text.trim() === word && t.scopes.includes(prim)).length;
  // 関数、引数の名前は、ふつうの名前。型の位置（: の後ろ）は、核の型
  const fn = tokensOf('fn list(date: date, time: time, items: list<string>): void;');
  assertScope(fn, 'fn list', 'list', 'entity.name.function.hozo', 0);
  assert.strictEqual(primCount(fn[0], 'date'), 1);
  assert.strictEqual(primCount(fn[0], 'time'), 1);
  assert.strictEqual(primCount(fn[0], 'list'), 1);
  assert.strictEqual(primCount(fn[0], 'string'), 1);
  assert.strictEqual(primCount(fn[0], 'void'), 1);
  // 配線の val と、値の位置は、塗らない
  const body = tokensOf(['fn f(): string {', '  val time = at(date);', '  val string = show(time, list);', '  return string;', '}'].join('\n'));
  for (const line of body.slice(1)) {
    for (const w of ['time', 'date', 'string', 'list']) assert.strictEqual(primCount(line, w), 0, `${line.line}: ${w}`);
  }
  // match の by の後ろの list / nullable は、標準の分類（形の語として塗る）。対象の値は、塗らない
  const m = tokensOf('  val n = match (list by list) {');
  assert.strictEqual(primCount(m[0], 'list'), 1);
  // 付ける先の case of の後ろの、核の型
  const c = tokensOf('decorator marked for case of string, case of never;');
  assert.strictEqual(primCount(c[0], 'string'), 1);
  assert.strictEqual(primCount(c[0], 'never'), 1);
  // 名前空間の要素
  const ns = tokensOf('namespace a.time {');
  assert.strictEqual(primCount(ns[0], 'time'), 0);
});

test('予約語のデコレータ名（宣言、適用、別名の先、@see）', () => {
  const deco = 'entity.name.function.decorator.hozo';
  const keywordScopes = ['keyword.declaration.hozo', 'keyword.control.hozo', 'support.type.primitive.hozo', 'keyword.other.hozo', 'variable.other.property.hozo'];
  // 適用: @ から名前まで（修飾を含む）が一つのデコレータの字句で、予約語としても、フィールド名としても塗らない
  const t = tokensOf('@db.list @db.effects(createOrder, notifyUser) @db.find @effects(a) @list @db.delete fn f();');
  for (const name of ['@db.list', '@db.effects', '@db.find', '@effects', '@list', '@db.delete']) {
    assertScope(t, 'fn f', name, deco);
    for (const kw of keywordScopes) assertNoScope(t, 'fn f', name, kw);
  }
  assertScope(t, 'fn f', 'fn', 'keyword.declaration.hozo');
  assertScope(t, 'fn f', 'f', 'entity.name.function.hozo');
  // フィールドに付けた、核の型と同じ名前のデコレータ
  const f = tokensOf('  model Order { @db.key @db.text id: OrderId; @db.int count: int; @db.float rate: float; @int raw: int; }');
  for (const name of ['@db.key', '@db.text', '@db.int', '@db.float', '@int']) {
    assertScope(f, 'model Order', name, deco);
    assertNoScope(f, 'model Order', name, 'support.type.primitive.hozo');
  }
  // フィールドの型の int / float は、核の型のまま
  assertScope(f, 'model Order', 'int', 'support.type.primitive.hozo', 0);
  assertScope(f, 'model Order', 'float', 'support.type.primitive.hozo', 0);
  assertScope(f, 'model Order', 'OrderId', 'entity.name.type.hozo');
  // 宣言: decorator の直後の一語は、予約語でもデコレータの名前
  const d = linesOf(files.db);
  for (const [line, name] of [
    ['decorator list for read;', 'list'],
    ['decorator int for field;', 'int'],
    ['decorator float for field;', 'float'],
    ['decorator delete for effect;', 'delete'],
  ]) {
    assertScope(d, line, name, 'entity.name.function.hozo', 0);
    assert.ok(!scopesAt(d, line, name, 0).some((x) => keywordScopes.includes(x)), `decorator ${name} の名前が予約語として塗られている`);
    assertScope(d, line, 'decorator', 'keyword.declaration.hozo');
  }
  const http = linesOf(files.http);
  assertScope(http, 'decorator default(value:', 'default', 'entity.name.function.hozo');
  assertNoScope(http, 'decorator default(value:', 'default', 'keyword.control.hozo');
  // 名前付きのデコレータの = の後ろ（DecoratorRef）も、デコレータの名前
  const a = tokensOf('decorator everything = db.list(orders); decorator nothing = effects();');
  assertScope(a, 'decorator everything', 'everything', 'entity.name.function.hozo');
  assertScope(a, 'decorator everything', 'db.list', deco);
  assertScope(a, 'decorator everything', 'nothing', 'entity.name.function.hozo');
  assertScope(a, 'decorator everything', 'effects', deco);
  const alias = linesOf(files.api);
  assertScope(alias, 'decorator both = effects(createSession, Cleanup);', 'effects', deco);
  assertScope(alias, 'decorator both = effects(createSession, Cleanup);', 'both', 'entity.name.function.hozo');
  // 実物: コーパスの適用
  assertScope(alias, '@find(users, "name")', '@find', deco);
  assertScope(alias, '@effects(createSession, deleteSession | renameUser)', '@effects', deco);
  assertNoScope(alias, '@effects(createSession, deleteSession | renameUser)', '@effects', 'keyword.control.hozo');
  assertScope(alias, '@db.table state users: list<User>;', '@db.table', deco);
  assertScope(alias, '@db.table state users: list<User>;', 'users', 'variable.other.hozo');
  // @see の後ろの、予約語のデコレータ名
  assertScope(alias, '* @see find', 'find', 'variable.other.jsdoc');
  assertScope(alias, '* @see api.effects', 'api.effects', 'variable.other.jsdoc');
  const see = tokensOf('/** @see db.list */');
  assertScope(see, '@see', 'db.list', 'variable.other.jsdoc');
});

test('説明のコメントの中の、デコレータのタグ（docs/grammar.md「ドキュメンテーションコメント」）', () => {
  const deco = 'entity.name.function.decorator.hozo';
  const doc = tokensOf(
    ['/**', ' * 注文', ' * @adr.context 二つの画面で、同じ形を使う', ' * @context これは、本文', ' * @adr.context("これも、本文") と書いた行', ' */'].join('\n'),
  );
  // 行の頭の、@ と、. を含む名前（後ろは、空白）は、デコレータのタグ
  assertScope(doc, '@adr.context 二つ', '@adr.context', deco);
  assertScope(doc, '@adr.context 二つ', '@adr.context', 'comment.block.documentation.hozo');
  // 接頭語のない @語 と、名前の直後が、空白でない行は、本文
  assert.ok(!doc.find((l) => l.text.includes('@context これは')).tokens.some((t) => t.scopes.includes(deco)));
  assert.ok(!doc.find((l) => l.text.includes('@adr.context("')).tokens.some((t) => t.scopes.includes(deco)));
  const oneLine = tokensOf('/** @adr.context 一行のコメント */');
  assertScope(oneLine, '@adr.context', '@adr.context', deco);
});

test('包んだ関数の引数（docs/grammar.md「デコレータの本体」）', () => {
  const t = tokensOf('decorator fn audited(next: fn) {\n  next();\n  record();\n}');
  // 頭の fn と、型の位置の fn は、どちらも、予約語。名前と、その呼び出しは、ほかの引数、ほかの呼び出しと同じ
  assertScope(t, 'decorator fn audited', 'fn', 'keyword.declaration.hozo', 0);
  assertScope(t, 'decorator fn audited', 'fn', 'keyword.declaration.hozo', 1);
  assertScope(t, 'decorator fn audited', 'audited', 'entity.name.function.hozo');
  assert.deepEqual(scopesAt(t, 'next();', '  next();'), scopesAt(t, 'record();', '  record();'));
});

test('付ける先の union / classify / decorator（docs/grammar.md「decorator の宣言」）', () => {
  // union、classify（分類そのもの）、デコレータの宣言も、付ける先。for の後ろでは、宣言の頭と同じ予約語
  const decl = 'decorator decided(...decisions: any case of never) for union, classify, decorator;';
  const t = tokensOf(decl);
  assertScope(t, decl, 'for', 'keyword.control.hozo');
  assertScope(t, 'for union', 'union', 'keyword.declaration.hozo');
  assertScope(t, 'union, classify', 'classify', 'keyword.declaration.hozo');
  // 宣言の頭の decorator と、付ける先の decorator は、同じ scope
  const last = t[0].tokens.filter((x) => x.text === 'decorator');
  assert.equal(last.length, 2);
  assert.ok(last.every((x) => x.scopes.includes('keyword.declaration.hozo')));
});

test('コメントとドキュメンテーションコメント', () => {
  const s = linesOf(files.server);
  assertScope(s, '// ---- §6.1 exception ----', '// ---- §6.1 exception ----', 'comment.line.double-slash.hozo');
  // 行の途中から始まる行コメント
  assertScope(s, 'throws RefundPending keeps effects   //', '// 返金の処理は受け付けたが、完了の確認が取れない', 'comment.line.double-slash.hozo');
  assertScope(s, 'throws RefundPending keeps effects   //', 'keeps', 'keyword.control.hozo');
  assertScope(s, '/** メールアドレス */', '/**', 'comment.block.documentation.hozo');
  assertScope(s, '/** メールアドレス */', ' メールアドレス ', 'comment.block.documentation.hozo');
  assertScope(s, '* @param in', '@param', 'storage.type.class.jsdoc');
  assertScope(s, '* @param in', 'in', 'variable.other.jsdoc');
  assertScope(s, '* @param in', '@param', 'comment.block.documentation.hozo');
  assertScope(s, '* @throws RateLimited', '@throws', 'storage.type.class.jsdoc');
  assertScope(s, '* @throws RateLimited', 'RateLimited', 'variable.other.jsdoc');
  assertScope(s, '* @see login', '@see', 'storage.type.class.jsdoc');
  assertScope(s, '* @see login', 'login', 'variable.other.jsdoc');
  // 複数行の doc comment の本文
  const b = linesOf(files.client);
  assertScope(b, '* 商品に適用される保証の種類。', '   * 商品に適用される保証の種類。', 'comment.block.documentation.hozo');
  // ブロックコメントと、行の途中から始まる doc comment
  const t = tokensOf('/* a */ /**/ model X {} /** @param x */ fn f(x: int);');
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
  // サンプルの仕様（予約語のデコレータ名、効果の宣言に付けたデコレータなどを、実物で通す）
  for (const f of fs.readdirSync(sampleDir).filter((f) => f.endsWith('.hozo'))) {
    const file = path.join(sampleDir, f);
    const lines = linesOf(file);
    assert.equal(lines.length, fs.readFileSync(file, 'utf8').split(/\r?\n/).length);
    for (const line of lines) {
      for (const tk of line.tokens) {
        assert.equal(tk.scopes[0], 'source.hozo');
        // @ で始まる字句は、デコレータ（か、核の印）一つになる。予約語の名前でも分かれない
        if (tk.text.startsWith('@') && !tk.scopes.some((x) => x.startsWith('comment.'))) {
          assert.ok(
            tk.scopes.includes('entity.name.function.decorator.hozo') || tk.scopes.includes('storage.modifier.hozo'),
            `${f}: ${JSON.stringify(tk.text)} がデコレータの字句になっていない（${JSON.stringify(line.text)}）`,
          );
        }
      }
    }
  }
});

test('例外を投げる効果（docs/grammar.md「関数の宣言」）', () => {
  // 効果の throws は、effects の前。例外は型の位置の名前、effects の後ろの操作は文脈キーワード
  const t = tokensOf('effect fn createUser(user: User): throws type.EmailTaken, Full effects users.post;');
  assertScope(t, 'effect fn', 'throws', 'keyword.control.hozo');
  assertScope(t, 'effect fn', 'type.EmailTaken', 'entity.name.type.hozo');
  assertScope(t, 'effect fn', 'Full', 'entity.name.type.hozo');
  assertScope(t, 'effect fn', 'effects', 'keyword.control.hozo');
  assertScope(t, 'effect fn', 'post', 'keyword.other.hozo');
});

test('リテラルの型は、型の位置でも、リテラルとして塗る（docs/grammar.md「型」の LiteralType）', () => {
  const lines = tokensOf([
    'model Report { severity: "error"; code: -1; ratio: 0.5; final: true; note: "memo"?; tags: "tag"[]; }',
    'union Severity { Error: "error"; Off: false; }',
    'fn retry(times: 3, loud: false): "done";',
    '  Fatal: Severity as "error";',
    '  Yes: boolean as true;',
  ].join('\n'));
  assertScope(lines, 'model Report', 'error', 'string.quoted.double.hozo', 0);
  assertScope(lines, 'model Report', '-1', 'constant.numeric.hozo', 0);
  assertScope(lines, 'model Report', '0.5', 'constant.numeric.hozo', 0);
  assertScope(lines, 'model Report', 'true', 'constant.language.hozo', 0);
  assertScope(lines, 'model Report', 'memo', 'string.quoted.double.hozo', 0);
  assertScope(lines, 'model Report', 'tag', 'string.quoted.double.hozo', 0);
  assertScope(lines, 'union Severity', 'error', 'string.quoted.double.hozo', 0);
  assertScope(lines, 'union Severity', 'false', 'constant.language.hozo', 0);
  assertScope(lines, 'fn retry', '3', 'constant.numeric.hozo', 0);
  assertScope(lines, 'fn retry', 'false', 'constant.language.hozo', 0);
  assertScope(lines, 'fn retry', 'done', 'string.quoted.double.hozo', 0);
  assertScope(lines, 'Fatal:', 'error', 'string.quoted.double.hozo', 0);
  assertScope(lines, 'Yes:', 'true', 'constant.language.hozo', 0);
  // リテラルの型は、型の名前としては、塗らない
  assertNoScope(lines, 'fn retry', 'done', 'entity.name.type.hozo');
  assertNoScope(lines, 'Yes:', 'true', 'entity.name.type.hozo');
});
