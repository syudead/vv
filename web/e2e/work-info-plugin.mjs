import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

// web/e2e/work-info-plugins.e2e.ts が上げる作品情報プラグインを、Go のソースからテストのたびに
// 組み立てる（specs/049-work-info-plugins/research.md R-12）。.wasm はリポジトリに置かない。
// GOOS=wasip1 GOARCH=wasm でビルドし、vv.plugin のカスタムセクションにマニフェストを足す。
// 手順は internal/wasmplugin/plugintest と同じである。

/** WORK_INFO_PLUGIN_SOURCE はテスト用プラグインの Go のソースの置き場（リポジトリから）。 */
const WORK_INFO_PLUGIN_SOURCE = path.join(
  "internal",
  "wasmplugin",
  "testdata",
  "e2eworks",
);

/**
 * WORK_INFO_PLUGIN_MANIFEST はテスト用プラグインのマニフェストである。秘密で必須の設定 token を
 * 宣言し、e2e は設定画面でその値を入れてから承認する。
 */
export const WORK_INFO_PLUGIN_MANIFEST = {
  contract: 1,
  name: "e2e-works",
  version: "1.0.0",
  description: "Answers with fixed work information for the browser tests.",
  operations: ["search", "fetch"],
  settings: [{ key: "token", label: "API token", secret: true, required: true }],
};

/** uvarint は n を LEB128 の符号なし整数に符号化する。 */
function uvarint(n) {
  const bytes = [];
  let rest = n;
  do {
    let byte = rest & 0x7f;
    rest = Math.floor(rest / 128);
    if (rest > 0) byte |= 0x80;
    bytes.push(byte);
  } while (rest > 0);
  return Buffer.from(bytes);
}

/** customSection は名前 name、中身 data のカスタムセクション（id 0）を符号化する。 */
function customSection(name, data) {
  const nameBytes = Buffer.from(name, "utf8");
  const payload = Buffer.concat([uvarint(nameBytes.length), nameBytes, data]);
  return Buffer.concat([Buffer.from([0]), uvarint(payload.length), payload]);
}

/**
 * buildWorkInfoPlugin はテスト用プラグインをビルドし、マニフェストを足したモジュールを output に
 * 書く。失敗すれば Go の出力を添えて投げる。
 */
export function buildWorkInfoPlugin(repoRoot, output, env) {
  const work = mkdtempSync(path.join(os.tmpdir(), "vv-e2e-plugin-"));
  try {
    const built = path.join(work, "plugin.wasm");
    const result = spawnSync("go", ["build", "-trimpath", "-o", built, "."], {
      cwd: path.join(repoRoot, WORK_INFO_PLUGIN_SOURCE),
      env: { ...env, GOOS: "wasip1", GOARCH: "wasm", CGO_ENABLED: "0" },
      encoding: "utf8",
    });
    if (result.error !== undefined) throw result.error;
    if (result.status !== 0)
      throw new Error(`build the e2e work-info plugin: ${result.stderr}`);
    const manifest = Buffer.from(JSON.stringify(WORK_INFO_PLUGIN_MANIFEST), "utf8");
    writeFileSync(
      output,
      Buffer.concat([readFileSync(built), customSection("vv.plugin", manifest)]),
    );
  } finally {
    rmSync(work, { recursive: true, force: true });
  }
}
