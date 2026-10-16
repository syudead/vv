import { copyFileSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

// web/e2e/work-info-plugins.e2e.ts が上げる作品情報プラグインを、Go のソースからテストのたびに
// 組み立てる（specs/049-work-info-plugins/research.md R-12）。.wasm はリポジトリに置かない。
// GOOS=wasip1 GOARCH=wasm でビルドし、作者と同じ `mdm plugin pack` で vv.plugin のセクションに
// マニフェストを書き込む（specs/056-plugin-tooling/research.md R-2）。

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

/**
 * buildWorkInfoPlugin はテスト用プラグインをビルドし、mdm（mdmBinary）の `plugin pack` でマニフェストを
 * 書き込んだモジュールを output に書く。失敗すれば Go か mdm の出力を添えて投げる。
 */
export function buildWorkInfoPlugin(repoRoot, output, mdmBinary, env) {
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
    const manifest = path.join(work, "manifest.json");
    writeFileSync(manifest, JSON.stringify(WORK_INFO_PLUGIN_MANIFEST));
    const pack = spawnSync(mdmBinary, ["plugin", "pack", built, manifest], {
      env,
      encoding: "utf8",
    });
    if (pack.error !== undefined) throw pack.error;
    if (pack.status !== 0)
      throw new Error(`pack the e2e work-info plugin: ${pack.stderr}`);
    copyFileSync(built, output);
  } finally {
    rmSync(work, { recursive: true, force: true });
  }
}
