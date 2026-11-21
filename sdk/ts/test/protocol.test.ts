// src/protocol.ts は、プロトコルの要求と応答の型を schema/ir.schema.json の $defs から手で写したもの
// （docs/packages.md「生成するもの」）。写しがスキーマと合っていること（項目の名前、必須かどうか、型）を確かめる。
// スキーマは Go の plugin パッケージから生成するので、Go の型を変えてスキーマを作り直せば、ここが遅れを見つける。
//
// やり方: protocol.ts を TypeScript のコンパイラで構文木にし、interface を「項目 → 型」の正規形の文字列にする。
// スキーマの $defs も同じ正規形にして比べる。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import ts from "typescript";

// build/test/protocol.test.js から見たリポジトリのルート。
const root = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", "..");
const schema = JSON.parse(readFileSync(resolve(root, "schema", "ir.schema.json"), "utf8")) as {
  $defs: Record<string, SchemaNode>;
};
const sourcePath = resolve(root, "sdk", "ts", "src", "protocol.ts");
const source = ts.createSourceFile(sourcePath, readFileSync(sourcePath, "utf8"), ts.ScriptTarget.ES2022, true);

interface SchemaNode {
  $ref?: string;
  const?: string;
  enum?: string[];
  type?: string;
  items?: SchemaNode;
  minItems?: number;
  maxItems?: number;
  properties?: Record<string, SchemaNode>;
  required?: string[];
}

/**
 * スキーマの $defs の名前 → protocol.ts の interface の名前。プロトコルの $defs のすべて。
 * 名前が違うのは診断の二つ: スキーマ（Go の型）は emit の errors の要素を Diagnostic、check の diagnostics の要素を
 * CheckDiagnostic と呼び、TS は check の方を Diagnostic、emit の方を EmitError と呼ぶ。
 */
const mapping: Record<string, string> = {
  EmitRequest: "EmitRequest",
  EmitResponse: "EmitResponse",
  CheckRequest: "CheckRequest",
  CheckResponse: "CheckResponse",
  ImportRequest: "ImportRequest",
  ImportResponse: "ImportResponse",
  CoverageRequest: "CoverageRequest",
  CoverageResponse: "CoverageResponse",
  CoveredFile: "CoveredFile",
  TestMap: "TestMap",
  TestEntry: "TestEntry",
  File: "File",
  Warning: "Warning",
  Diagnostic: "EmitError",
  CheckDiagnostic: "Diagnostic",
  PluginError: "PluginError",
};

/** 写しがスキーマとわざと違う箇所（interface.項目 → protocol.ts の型の正規形）。今はない。 */
const deviations: Record<string, string> = {};

// --- protocol.ts の側

const interfaces = new Map<string, ts.InterfaceDeclaration>();
const aliases = new Map<string, ts.TypeNode>();
const constants = new Map<string, string>();
for (const stmt of source.statements) {
  if (ts.isInterfaceDeclaration(stmt)) interfaces.set(stmt.name.text, stmt);
  if (ts.isTypeAliasDeclaration(stmt)) aliases.set(stmt.name.text, stmt.type);
  if (ts.isVariableStatement(stmt)) {
    for (const d of stmt.declarationList.declarations) {
      let init = d.initializer;
      if (init && ts.isAsExpression(init)) init = init.expression;
      if (ts.isIdentifier(d.name) && init && ts.isStringLiteral(init)) constants.set(d.name.text, init.text);
    }
  }
}

function members(nodes: ts.NodeArray<ts.TypeElement>): string {
  const parts: string[] = [];
  for (const m of nodes) {
    assert.ok(ts.isPropertySignature(m) && m.type !== undefined && ts.isIdentifier(m.name), `項目でないメンバー: ${m.getText()}`);
    parts.push(`${m.name.text}${m.questionToken ? "?" : ""}: ${typeOf(m.type)}`);
  }
  return `{ ${parts.sort().join("; ")} }`;
}

function typeOf(node: ts.TypeNode): string {
  if (node.kind === ts.SyntaxKind.StringKeyword) return "string";
  if (node.kind === ts.SyntaxKind.BooleanKeyword) return "boolean";
  if (node.kind === ts.SyntaxKind.NumberKeyword) return "number";
  // 長さの決まった配列（[number, number]）: 要素の型と、長さ
  if (ts.isTupleTypeNode(node)) {
    const elements = node.elements.map((e) => typeOf(e as ts.TypeNode));
    assert.ok(elements.length > 0 && elements.every((e) => e === elements[0]), `要素の型がそろっていないタプル: ${node.getText()}`);
    return `${elements[0]}[${elements.length}]`;
  }
  if (ts.isLiteralTypeNode(node) && ts.isStringLiteral(node.literal)) return JSON.stringify(node.literal.text);
  if (ts.isArrayTypeNode(node)) return `${typeOf(node.elementType)}[]`;
  if (ts.isUnionTypeNode(node)) return node.types.map(typeOf).sort().join(" | ");
  if (ts.isTypeLiteralNode(node)) return members(node.members);
  if (ts.isTypeQueryNode(node) && ts.isIdentifier(node.exprName)) {
    const value = constants.get(node.exprName.text);
    assert.ok(value !== undefined, `typeof ${node.exprName.text} の値が分からない`);
    return JSON.stringify(value);
  }
  if (ts.isTypeReferenceNode(node) && ts.isIdentifier(node.typeName)) {
    const name = node.typeName.text;
    if (node.typeArguments) return `${name}<${node.typeArguments.map(typeOf).join(", ")}>`;
    const alias = aliases.get(name);
    return alias ? typeOf(alias) : name;
  }
  assert.fail(`正規形にできない型: ${node.getText()}`);
}

// --- スキーマの側

function schemaType(node: SchemaNode): string {
  if (node.$ref !== undefined) {
    const name = node.$ref.replace("#/$defs/", "");
    return mapping[name] ?? name; // プロトコルの外（View、Program）は IR の型の名前のまま
  }
  if (node.const !== undefined) return JSON.stringify(node.const);
  if (node.enum !== undefined) return node.enum.map((v) => JSON.stringify(v)).sort().join(" | ");
  if (node.type === "string") return "string";
  if (node.type === "boolean") return "boolean";
  if (node.type === "integer") return "number";
  if (node.type === "array" && node.items && node.minItems !== undefined && node.minItems === node.maxItems) {
    return `${schemaType(node.items)}[${node.minItems}]`;
  }
  if (node.type === "array" && node.items) return `${schemaType(node.items)}[]`;
  if (node.type === "object" && node.properties) return schemaObject(node, "");
  assert.fail(`正規形にできないスキーマ: ${JSON.stringify(node)}`);
}

function schemaObject(node: SchemaNode, owner: string): string {
  const required = new Set(node.required ?? []);
  const parts = Object.entries(node.properties ?? {}).map(([name, prop]) => {
    const type = deviations[`${owner}.${name}`] ?? schemaType(prop);
    return `${name}${required.has(name) ? "" : "?"}: ${type}`;
  });
  return `{ ${parts.sort().join("; ")} }`;
}

for (const [def, name] of Object.entries(mapping)) {
  test(`protocol.ts ${name} mirrors schema $defs.${def}`, () => {
    const node = schema.$defs[def];
    assert.ok(node !== undefined, `スキーマに $defs.${def} がない`);
    const decl = interfaces.get(name);
    assert.ok(decl !== undefined, `protocol.ts に interface ${name} がない`);
    assert.equal(members(decl.members), schemaObject(node, name));
  });
}

test("protocol.ts: every interface is checked against the schema", () => {
  // Handler はプラグインが実装する形で、JSON の形ではない
  const unchecked = [...interfaces.keys()].filter((n) => n !== "Handler" && !Object.values(mapping).includes(n));
  assert.deepEqual(unchecked, [], "スキーマと比べていない interface がある（mapping に足す）");
});

test("protocol.ts: PROTOCOL is the schema's protocol version", () => {
  assert.equal(constants.get("PROTOCOL"), schema.$defs["EmitRequest"]?.properties?.["protocol"]?.const);
  assert.equal(constants.get("PROTOCOL"), schema.$defs["Program"]?.properties?.["version"]?.const);
});

test("protocol.ts: the deviations still exist", () => {
  // わざと違えた箇所がなくなったら、表からも消す
  for (const [key, want] of Object.entries(deviations)) {
    const [iface, prop] = key.split(".");
    const decl = interfaces.get(iface ?? "");
    const member = decl?.members.find((m) => ts.isPropertySignature(m) && ts.isIdentifier(m.name) && m.name.text === prop);
    assert.ok(member && ts.isPropertySignature(member) && member.type, `${key} がない`);
    assert.equal(typeOf(member.type), want);
  }
});
