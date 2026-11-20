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
  // 状態の分類・遷移の効果・状態での match（docs/grammar.md「exception / effect / read」「classify」「配線」）
  state: path.join(root, 'tests', 'corpus', 'state', 'shop.hozo'),
  // export、reads、extends / override、op の分類とケースの as、throw、read の宣言、直に呼ぶ組み込みの効果（docs/grammar.md「model」「exception / effect / read」「classify」「op」「配線」）と、
  // 版 0.5 の構文（効果の宣言に付けたデコレータ、reads / effects の節、残りの引数、予約語のデコレータ名）
  api: path.join(root, 'tests', 'corpus', 'view', 'api.hozo'),
  // 予約語のデコレータ名の宣言（decorator list / int / delete）と、対象 effect / read、核の印 @repeatable
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
  const b = linesOf(files.browser);
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
    'classify Actor of never { @actor.human Customer; }',
    'decorator human for Actor case;',
    'decorator by(who: any case of never) for op;',
  ].join('\n'));
  assertScope(c, 'classify Actor', 'never', 'support.type.primitive.hozo');
  assertScope(c, 'classify Actor', '@actor.human', 'entity.name.function.decorator.hozo');
  assertScope(c, 'decorator human', 'case', 'keyword.control.hozo');
  assertScope(c, 'decorator by', 'case', 'keyword.control.hozo');
  assertScope(c, 'decorator by', 'never', 'support.type.primitive.hozo');
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
  assertScope(v, 'decorator min(value: int | float) for scalar | field;', 'field', 'keyword.other.hozo');
  assertScope(v, 'decorator min(value: int | float) for scalar | field;', 'scalar', 'keyword.declaration.hozo');
  const p = linesOf(files.page);
  assertScope(p, 'decorator interaction(page: any op, when: any variant);', 'any', 'keyword.control.hozo');
  assertScope(p, 'decorator interaction(page: any op, when: any variant);', 'variant', 'keyword.other.hozo');
  assertScope(p, 'decorator route(path: string, from: any state?', 'state', 'keyword.declaration.hozo');
  const s = linesOf(files.server);
  assertScope(s, 'effect createSession(login: LoginOk) on sessions.post;', 'on', 'keyword.control.hozo');
  assertScope(s, 'effect createSession(login: LoginOk) on sessions.post;', 'post', 'keyword.other.hozo');
  assertScope(s, 'effect deleteSession(token: Token?) on sessions.delete;', 'delete', 'keyword.other.hozo');
  const b = linesOf(files.browser);
  assertScope(b, 'effect transitionToLogin() on location.put;', 'put', 'keyword.other.hozo');
  // model Violation { field: string; } の field は識別子
  const violation = s.find((l) => l.text.includes('model Violation {'));
  assert.ok(!violation.tokens.some((t) => t.text === 'field' && t.scopes.includes('keyword.other.hozo')));
  // 値の位置の post / put / delete も識別子
  const t = tokensOf('op post(put: int) { delete(put); }');
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
  const f = tokensOf('  string: string;');
  assertScope(f, 'string: string;', 'string', prop, 0);
  assertScope(f, 'string: string;', 'string', 'support.type.primitive.hozo', 1);
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
  // 実物: 設計書のコーパスの override
  assertScope(linesOf(files.server), 'override hash: never;', 'override', 'storage.modifier.hozo');
  assertScope(linesOf(files.server), 'override hash: never;', 'never', 'support.type.primitive.hozo');
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
  assertScope(s, 'effect createSession(login: LoginOk) on sessions.post;', 'createSession', 'entity.name.function.hozo');
  assertScope(s, 'decorator requiresAuth throws', 'requiresAuth', 'entity.name.function.hozo');
  assertScope(s, 'state users: list<User>;', 'users', 'variable.other.hozo');
  const b = linesOf(files.browser);
  assertScope(b, 'union ApiError {', 'ApiError', 'entity.name.type.hozo');
  assertScope(b, 'classify WarrantyKind of server.Item {', 'WarrantyKind', 'entity.name.type.hozo');
  assertScope(b, 'classify WarrantyAvailable of server.Item { Available; Unavailable; }', 'WarrantyAvailable', 'entity.name.type.hozo');
  // condition は予約語ではない（docs/grammar.md「予約語」）: 宣言の頭としても、名前としても、塗らない
  const cond = tokensOf('op check(condition: string): string;');
  assert.ok(!cond[0].tokens.some((tk) => tk.text.includes('condition') && tk.scopes.some((sc) => sc.startsWith('keyword'))), 'condition が予約語として塗られている');
});

test('型の位置の名前', () => {
  const s = linesOf(files.server);
  assertScope(s, 'state users: list<User>;', 'User', 'entity.name.type.hozo');
  assertScope(s, '@db.key    id: UserId;', 'UserId', 'entity.name.type.hozo');
  assertScope(s, 'op login(in: LoginInput): LoginOk', 'LoginInput', 'entity.name.type.hozo');
  assertScope(s, 'op login(in: LoginInput): LoginOk', 'LoginOk', 'entity.name.type.hozo');
  assertScope(s, 'exception ClientError(value: Violations);', 'Violations', 'entity.name.type.hozo');
  assertScope(s, 'model PublicUser extends User {', 'User', 'entity.name.type.hozo');
  assertScope(s, 'throws InvalidCredentials, NotVerified', 'NotVerified', 'entity.name.type.hozo');
  // 例外の宣言の名前と、引数の名前（引数名には scope を付けない）
  assertScope(s, 'exception ClientError(value: Violations);', 'ClientError', 'entity.name.type.hozo');
  const b = linesOf(files.browser);
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
  assertScope(b, 'state query: single<Record<string, string[]>>;', 'Record', 'support.type.primitive.hozo');
  assertScope(b, 'state query: single<Record<string, string[]>>;', 'string', 'support.type.primitive.hozo', 1);
  assertScope(b, 'op chooseYamadaWarranty(item: WarrantyKind.Yamada<server.Item>', 'WarrantyKind.Yamada', 'entity.name.type.hozo');
  // 複数行にまたがる返り値の型と throws
  assertScope(b, /^\s*: server\.WarrantyLine$/, 'server.WarrantyLine', 'entity.name.type.hozo');
  assertScope(b, 'throws page.Aborted, http.NetworkError, http.ServerError', 'page.Aborted', 'entity.name.type.hozo');
  // 値の位置（レコードリテラル、名前付き引数）の小文字の名前は型にしない
  const orders = b.find((l) => l.text.includes('return server.getOrders({ page: query.page'));
  assert.ok(!orders.tokens.some((t) => t.scopes.includes('entity.name.type.hozo')));
  const route = b.find((l) => l.text.includes('@page.route("/orders", from: query, onInvalid: transitionToOrders)'));
  assert.ok(!route.tokens.some((t) => t.scopes.includes('entity.name.type.hozo')));
});

test('修飾（export、override）と、reads / as / throw / never', () => {
  const api = linesOf(files.api);
  const findUser = 'export op findUser(id: UserId): User? reads users, sessionsOf reads users {';
  // export は修飾の予約語。op の名前は今まで通り
  assertScope(api, findUser, 'export', 'storage.modifier.hozo');
  assertScope(api, findUser, 'op', 'keyword.declaration.hozo');
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
  assertScope(st, 'effect placeOrder() on Checkout: Shopping -> Ordered;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'effect startOver() on Checkout: any -> Shopping;', '->', 'keyword.operator.arrow.hozo');
  // 遷移のケースの並びの区切りは ,（docs/grammar.md「exception / effect / read」の CaseSet）
  assertScope(st, 'effect touchCheckout() on Checkout: Shopping, Ordered -> any;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'effect touchCheckout() on Checkout: Shopping, Ordered -> any;', 'any', 'keyword.control.hozo');
  assertScope(st, 'effect clearCart() on carts: HasItems -> Empty;', '->', 'keyword.operator.arrow.hozo');
  assertScope(st, 'Empty    => showLogin();', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'Shopping => submitOrder();', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'Ordered  => none;', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'else     => showLogin();', '=>', 'keyword.operator.arrow.hozo');
  assertScope(st, 'else     => showLogin();', 'else', 'keyword.control.hozo');
  const b = linesOf(files.browser);
  assertScope(b, 'NoWarranty => none;', '=>', 'keyword.operator.arrow.hozo');
  // 矢印の両側は今まで通り
  assertScope(st, 'effect startOver() on Checkout: any -> Shopping;', 'any', 'keyword.control.hozo');
  assertScope(st, 'effect placeOrder() on Checkout: Shopping -> Ordered;', 'on', 'keyword.control.hozo');
  assertScope(st, 'effect placeOrder() on Checkout: Shopping -> Ordered;', 'placeOrder', 'entity.name.function.hozo');
  assertScope(st, 'Ordered  => none;', 'none', 'constant.language.hozo');
  // 比較の記号ではない（> は型引数の閉じ、= は名前付きのデコレータ）
  const t = tokensOf('decorator once = retry(1); state s: list<X>;');
  assert.ok(!t[0].tokens.some((tk) => tk.scopes.includes('keyword.operator.arrow.hozo')));
});

test('行の遷移（effect … on 状態.操作 as …）と、ケースの as none', () => {
  // docs/grammar.md「exception / effect / read」の RowClause: as の後ろの型の位置は ; まで続き、ケース（分類の名前とケースの名前）は
  // 型の位置の名前、-> は矢印、any と else は予約語（docs/editors.md「TextMate の文法」）
  const put = tokensOf('  effect cancel(order: Order) on orders.put as Stage.Draft, kind.Stage.Accepted -> Stage.Cancelled;');
  assertScope(put, 'effect cancel', 'effect', 'keyword.declaration.hozo');
  assertScope(put, 'effect cancel', 'cancel', 'entity.name.function.hozo');
  assertScope(put, 'effect cancel', 'on', 'keyword.control.hozo');
  assertScope(put, 'effect cancel', 'put', 'keyword.other.hozo');
  assertScope(put, 'effect cancel', 'as', 'keyword.control.hozo');
  assertScope(put, 'effect cancel', 'Stage.Draft', 'entity.name.type.hozo');
  assertScope(put, 'effect cancel', 'kind.Stage.Accepted', 'entity.name.type.hozo');
  assertScope(put, 'effect cancel', '->', 'keyword.operator.arrow.hozo');
  assertScope(put, 'effect cancel', 'Stage.Cancelled', 'entity.name.type.hozo');
  // 矢印の前の any と、後ろの else
  const hatch = tokensOf('effect patch(order: Order) on orders.put as any -> else;');
  assertScope(hatch, 'effect patch', 'as', 'keyword.control.hozo');
  assertScope(hatch, 'effect patch', 'any', 'keyword.control.hozo');
  assertScope(hatch, 'effect patch', '->', 'keyword.operator.arrow.hozo');
  assertScope(hatch, 'effect patch', 'else', 'keyword.control.hozo');
  // 矢印のない形
  const post = tokensOf('effect accept(order: Order) on orders.post as Stage.Accepted;\neffect bring(order: Order) on orders.post as any;');
  assertScope(post, 'effect accept', 'post', 'keyword.other.hozo');
  assertScope(post, 'effect accept', 'Stage.Accepted', 'entity.name.type.hozo');
  assertScope(post, 'effect bring', 'any', 'keyword.control.hozo');
  // 行の遷移は ; で終わる: 次の宣言は、今まで通り
  const next = tokensOf('effect a(o: O) on s.delete as Life.Open -> Life.Absent;\nstate rows: list<Row>;');
  assertScope(next, 'effect a', 'delete', 'keyword.other.hozo');
  assertScope(next, 'effect a', 'Life.Absent', 'entity.name.type.hozo');
  assertScope(next, 'state rows', 'state', 'keyword.declaration.hozo');
  assertScope(next, 'state rows', 'rows', 'variable.other.hozo');
  // ない側のケース: as の後ろの none は、値の none と同じ scope（docs/grammar.md「classify」）
  const none = tokensOf('classify Life of Order? { Open: Order? as Order; Absent: Order? as none; }');
  assertScope(none, 'classify Life', 'as', 'keyword.control.hozo', 1);
  assertScope(none, 'classify Life', 'none', 'constant.language.hozo');
  assertScope(none, 'classify Life', 'Order', 'entity.name.type.hozo', 3);
  // 実物: コーパスの行の遷移と、ない側のケース
  const api = linesOf(files.api);
  assertScope(api, 'effect rest(user: User, since: Time) on users.put as Standing.Fresh, Standing.Active -> Standing.Idle;', 'Standing.Active', 'entity.name.type.hozo');
  assertScope(api, 'effect rest(user: User, since: Time) on users.put as Standing.Fresh, Standing.Active -> Standing.Idle;', '->', 'keyword.operator.arrow.hozo');
  assertScope(api, 'effect amend(user: User) on users.put as Standing.Idle -> else;', 'else', 'keyword.control.hozo');
  assertScope(api, 'effect closeUser(user: User) on users.put as any -> Standing.Closed;', 'any', 'keyword.control.hozo');
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
  // Empty / HasItems は予約語ではない（docs/grammar.md「予約語」）。match のアームでは色を付けない
  for (const [line, word] of [['Empty    => showLogin();', 'Empty'], ['HasItems => showCart();', 'HasItems']]) {
    const l = st.find((x) => x.text.includes(line));
    assert.ok(l, `行が見つからない: ${line}`);
    const tk = l.tokens.find((x) => x.text.includes(word));
    assert.ok(tk, `字句 ${word} が行にない`);
    assert.deepEqual(tk.scopes, ['source.hozo'], `${word} に scope が付いている: ${tk.scopes.join(' ')}`);
  }
  // 遷移のケースは : の後ろなので、ほかの大文字で始まる名前と同じく型の位置の名前になる（docs/editors.md の表の「型の位置の名前」）
  assertScope(st, 'effect clearCart() on carts: HasItems -> Empty;', 'HasItems', 'entity.name.type.hozo');
  assertScope(st, 'effect clearCart() on carts: HasItems -> Empty;', 'Empty', 'entity.name.type.hozo');
});

test('リテラル', () => {
  const s = linesOf(files.server);
  assertScope(s, '@http.header(name: "Authorization") token: Token?;', 'Authorization', 'string.quoted.double.hozo');
  assertScope(s, '@http.status(401)', '401', 'constant.numeric.hozo');
  assertScope(s, '@http.default(1) @http.query page: int;', '1', 'constant.numeric.hozo');
  const t = tokensOf('@limits(5m, 30s, 100ms, -3.5, "a\\n\\u{1F600}\\"") model X { a: Duration; }');
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
  const find = 'decorator find(table: any state, column: string?) for op reads table;';
  assertScope(api, find, 'reads', 'keyword.control.hozo');
  assertScope(api, find, 'op', 'keyword.declaration.hozo');
  assertScope(api, find, 'for', 'keyword.control.hozo');
  assertScope(api, find, 'string', 'support.type.primitive.hozo');
  const findLine = api.find((l) => l.text.includes(find));
  assert.ok(!findLine.tokens.some((tk) => tk.text.trim() === 'table' && tk.scopes.length > 1), 'reads の後ろの引数名に scope が付いている');
  assert.ok(!findLine.tokens.some((tk) => tk.scopes.includes('entity.name.function.hozo') && tk.text !== 'find'), 'デコレータの名前のほかが、宣言の名前になっている');
  // 残りの引数（...）と、effects の節
  const effects = 'decorator effects(...targets: any effect) for op effects targets reads sessions;';
  assertScope(api, effects, '...', 'keyword.operator.rest.hozo');
  assertScope(api, effects, 'any', 'keyword.control.hozo');
  assertScope(api, effects, 'effect', 'keyword.declaration.hozo');
  assertScope(api, effects, 'for', 'keyword.control.hozo');
  assertScope(api, effects, 'op', 'keyword.declaration.hozo');
  // 一つ目の effects は宣言の名前（予約語として塗らない）、二つ目は節の頭。その後ろに reads の節が続く
  assertScope(api, effects, 'effects', 'entity.name.function.hozo', 0);
  assertNoScope(api, effects, 'effects', 'keyword.control.hozo');
  assertScope(api, effects, 'effects', 'keyword.control.hozo', 1);
  assertScope(api, effects, 'reads', 'keyword.control.hozo');
  const effectsLine = api.find((l) => l.text.includes(effects));
  assert.ok(!effectsLine.tokens.some((tk) => tk.text.trim() === 'targets' && tk.scopes.length > 1), '引数名 targets に scope が付いている');
  // for を書かない宣言の節、ほかの引数の後ろの残りの引数、節が続く形
  const t = tokensOf('decorator both(extra: any state?, ...more: any effect) reads logs, extra throws Busy, Late prevents effects effects more, add;');
  assertScope(t, 'decorator both', '...', 'keyword.operator.rest.hozo');
  assertScope(t, 'decorator both', 'reads', 'keyword.control.hozo');
  assertScope(t, 'decorator both', 'throws', 'keyword.control.hozo');
  assertScope(t, 'decorator both', 'Busy', 'entity.name.type.hozo');
  assertScope(t, 'decorator both', 'Late', 'entity.name.type.hozo');
  assertScope(t, 'decorator both', 'prevents', 'keyword.control.hozo');
  assertScope(t, 'decorator both', 'effects', 'keyword.control.hozo', 0);
  assertScope(t, 'decorator both', 'effects', 'keyword.control.hozo', 1);
  // 効果の宣言に付けたデコレータ（実物: コーパス）
  const renameUser = 'effect renameUser(id: UserId, name: string) on users.put;';
  assertScope(api, /^\s*@into\(users\)$/, '@into', 'entity.name.function.decorator.hozo');
  assertScope(api, renameUser, 'effect', 'keyword.declaration.hozo');
  assertScope(api, renameUser, 'renameUser', 'entity.name.function.hozo');
  assertScope(api, renameUser, 'UserId', 'entity.name.type.hozo');
  assertScope(api, renameUser, 'string', 'support.type.primitive.hozo');
  assertScope(api, renameUser, 'put', 'keyword.other.hozo');
  const e = tokensOf('@db.insert effect createOrder(order: Order) on orders.post;');
  assertScope(e, 'effect createOrder', '@db.insert', 'entity.name.function.decorator.hozo');
  assertScope(e, 'effect createOrder', 'effect', 'keyword.declaration.hozo');
  assertScope(e, 'effect createOrder', 'createOrder', 'entity.name.function.hozo');
  assertScope(e, 'effect createOrder', 'on', 'keyword.control.hozo');
  assertScope(e, 'effect createOrder', 'post', 'keyword.other.hozo');
});

test('read の宣言、@repeatable、引数を持つ例外、直に呼ぶ組み込みの効果', () => {
  const api = linesOf(files.api);
  // read の宣言: read は宣言の頭、名前は op・effect と同じ scope。引数と結果は型の位置、on の後ろの状態には scope を付けない
  const userById = 'read userById(id: UserId): User? on users;';
  assertScope(api, userById, 'read', 'keyword.declaration.hozo');
  assertScope(api, userById, 'userById', 'entity.name.function.hozo');
  assertScope(api, userById, 'UserId', 'entity.name.type.hozo');
  assertScope(api, userById, 'User', 'entity.name.type.hozo');
  assertScope(api, userById, 'on', 'keyword.control.hozo');
  const readLine = api.find((l) => l.text.includes(userById));
  assert.ok(!readLine.tokens.some((tk) => tk.text.trim() === 'users' && tk.scopes.length > 1), 'on の後ろの状態に scope が付いている');
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
  const marks = tokensOf('@repeatable @external decorator tag(name: string) for read | effect | exception;');
  assertScope(marks, 'decorator tag', '@repeatable', 'storage.modifier.hozo');
  assertScope(marks, 'decorator tag', '@external', 'storage.modifier.hozo');
  assertScope(marks, 'decorator tag', 'read', 'keyword.declaration.hozo');
  assertScope(marks, 'decorator tag', 'effect', 'keyword.declaration.hozo');
  assertScope(marks, 'decorator tag', 'exception', 'keyword.declaration.hozo');
  // read は、フィールド名とデコレータ名の位置では予約語として塗らない
  const field = tokensOf('model Mail { read: boolean; }');
  assertScope(field, 'model Mail', 'read', 'variable.other.property.hozo');
  assertNoScope(field, 'model Mail', 'read', 'keyword.declaration.hozo');
  const deco = tokensOf('@db.read read latest(): Mail? on mails;');
  assertScope(deco, 'read latest', '@db.read', 'entity.name.function.decorator.hozo');
  assertScope(deco, 'read latest', 'read', 'keyword.declaration.hozo');
  assertScope(deco, 'read latest', 'latest', 'entity.name.function.hozo');
  // 引数を持つ例外と、引数を渡す throw
  const ex = tokensOf('exception LimitExceeded(wait: Duration, reason: string);');
  assertScope(ex, 'exception LimitExceeded', 'exception', 'keyword.declaration.hozo');
  assertScope(ex, 'exception LimitExceeded', 'LimitExceeded', 'entity.name.type.hozo');
  assertScope(ex, 'exception LimitExceeded', 'Duration', 'support.type.primitive.hozo');
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
  const clause = tokensOf('op signOut() effects sessions.delete, api.current.put, dropAll reads users { sessions.delete(); }');
  assertScope(clause, 'op signOut', 'effects', 'keyword.control.hozo');
  assertScope(clause, 'op signOut', 'delete', 'keyword.other.hozo', 0);
  assertScope(clause, 'op signOut', 'put', 'keyword.other.hozo');
  assertScope(clause, 'op signOut', 'reads', 'keyword.control.hozo');
  assertScope(clause, 'op signOut', 'delete', 'keyword.other.hozo', 1);
  assert.ok(!clause[0].tokens.some((tk) => /dropAll|users/.test(tk.text) && tk.scopes.length > 1), '節の名前に scope が付いている');
});

test('予約語のデコレータ名（宣言、適用、別名の先、@see）', () => {
  const deco = 'entity.name.function.decorator.hozo';
  const keywordScopes = ['keyword.declaration.hozo', 'keyword.control.hozo', 'support.type.primitive.hozo', 'keyword.other.hozo', 'variable.other.property.hozo'];
  // 適用: @ から名前まで（修飾を含む）が一つのデコレータの字句で、予約語としても、フィールド名としても塗らない
  const t = tokensOf('@db.list @db.effects(createOrder, notifyUser) @db.find @effects(a) @list @db.delete op f();');
  for (const name of ['@db.list', '@db.effects', '@db.find', '@effects', '@list', '@db.delete']) {
    assertScope(t, 'op f', name, deco);
    for (const kw of keywordScopes) assertNoScope(t, 'op f', name, kw);
  }
  assertScope(t, 'op f', 'op', 'keyword.declaration.hozo');
  assertScope(t, 'op f', 'f', 'entity.name.function.hozo');
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

test('コメントとドキュメンテーションコメント', () => {
  const s = linesOf(files.server);
  assertScope(s, '// ---- §6.1 exception ----', '// ---- §6.1 exception ----', 'comment.line.double-slash.hozo');
  // 行の途中から始まる行コメント
  assertScope(s, 'throws RefundPending keeps effects   //', '// 返金の処理は受け付けたが、完了の確認が取れない', 'comment.line.double-slash.hozo');
  assertScope(s, 'throws RefundPending keeps effects   //', 'keeps', 'keyword.control.hozo');
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
  // サンプルの仕様（予約語のデコレータ名、効果の宣言に付けたデコレータなどを、実物で通す）
  for (const f of fs.readdirSync(sampleDir).filter((f) => f.endsWith('.hozo'))) {
    const file = path.join(sampleDir, f);
    const lines = linesOf(file);
    assert.equal(lines.length, fs.readFileSync(file, 'utf8').split(/\r?\n/).length);
    for (const line of lines) {
      for (const tk of line.tokens) {
        assert.equal(tk.scopes[0], 'source.hozo');
        // @ で始まる字句は、デコレータ（か @external）一つになる。予約語の名前でも分かれない
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
