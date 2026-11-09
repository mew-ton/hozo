// schema/ir.schema.json から src/ir.ts を生成する（docs/packages.md「生成するもの」）。
// 実行: npm run generate。CI は生成し直して差分がないことを確かめる（docs/testing.md「生成物の一致」）。
import { compileFromFile } from "json-schema-to-typescript";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const schema = resolve(here, "..", "..", "..", "schema", "ir.schema.json");
const out = resolve(here, "..", "src", "ir.ts");

const banner = [
  "/* eslint-disable */",
  "// Code generated from schema/ir.schema.json by `npm run generate` (json-schema-to-typescript); DO NOT EDIT.",
  "// IR の構造の持ち主は docs/ir.md、JSON の形は Go の ir パッケージ（docs/requirements.md「プラグインの書き方」）。",
].join("\n");

const ts = await compileFromFile(schema, {
  bannerComment: banner,
  additionalProperties: false,
  unknownAny: true,
  strictIndexSignatures: false,
  cwd: dirname(schema),
  style: { semi: true, singleQuote: false, trailingComma: "all", printWidth: 100 },
});
await mkdir(dirname(out), { recursive: true });
await writeFile(out, ts);
console.log(`wrote ${out}`);
