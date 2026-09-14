// @vitest-environment node
//
// この検査は DOM を使わない（CSS を読んで計算するだけ）。jsdom では
// import.meta.url が http の URL になり、隣のファイルを指せない。
import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

// 色の定義は web/src/index.css の @theme が唯一の真実である（R-401）。
// TypeScript 側に値を複製せず、そのファイルを**ファイルとして**読んで計算する
// （R-405）。取り込み（import）にしないのは、vite.config.ts の css: false が
// CSS の取り込みを空に差し替えるためで、そもそも描画は要らない。
const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

/**
 * themeColors は @theme ブロックの --color-* を取り出す。
 *
 * @theme の外（@layer base など）に同じ名前があっても拾わない。規則の置き場が
 * 1 か所であることが FR-001 の要求で、検査もその 1 か所だけを見る。
 */
function themeColors(source: string): Map<string, string> {
  const found = new Map<string, string>();

  const at = source.indexOf("@theme");
  if (at < 0) {
    return found;
  }
  const open = source.indexOf("{", at);
  if (open < 0) {
    return found;
  }

  // 対応する閉じ括弧まで（@theme の中に入れ子が来ても数え違えないようにする）。
  let depth = 0;
  let close = -1;
  for (let i = open; i < source.length; i += 1) {
    const char = source[i];
    if (char === "{") {
      depth += 1;
    } else if (char === "}") {
      depth -= 1;
      if (depth === 0) {
        close = i;
        break;
      }
    }
  }
  if (close < 0) {
    return found;
  }

  const block = source.slice(open + 1, close);
  const declaration = /--color-([a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{6})\s*;/g;
  for (const match of block.matchAll(declaration)) {
    const name = match[1];
    const value = match[2];
    if (name !== undefined && value !== undefined) {
      found.set(name, value.toLowerCase());
    }
  }
  return found;
}

/** channel は sRGB の 1 成分（0〜255）を線形化する（WCAG 2.1 の定義）。 */
function channel(value: number): number {
  const ratio = value / 255;
  return ratio <= 0.03928 ? ratio / 12.92 : ((ratio + 0.055) / 1.055) ** 2.4;
}

/** luminance は #rrggbb の相対輝度を返す。 */
function luminance(hex: string): number {
  const red = Number.parseInt(hex.slice(1, 3), 16);
  const green = Number.parseInt(hex.slice(3, 5), 16);
  const blue = Number.parseInt(hex.slice(5, 7), 16);
  return 0.2126 * channel(red) + 0.7152 * channel(green) + 0.0722 * channel(blue);
}

/** contrast は 2 色の対比（1〜21）を返す。 */
function contrast(foreground: string, background: string): number {
  const first = luminance(foreground);
  const second = luminance(background);
  const lighter = Math.max(first, second);
  const darker = Math.min(first, second);
  return (lighter + 0.05) / (darker + 0.05);
}

/**
 * pairs は検査する組である。
 *
 * specs/004-library-ui/contracts/design-tokens.md 2.「対比」の表をそのまま
 * 持つ。**表に無い組は検査されない**ので、トークンを足したら両方に足す。
 * 必要な比は FR-004 による（本文 4.5:1、境界と大きな文字 3:1）。
 */
const pairs: { foreground: string; background: string; required: number }[] = [
  { foreground: "body", background: "surface", required: 4.5 },
  { foreground: "body", background: "surface-raised", required: 4.5 },
  { foreground: "body", background: "badge", required: 4.5 },
  { foreground: "muted", background: "surface", required: 4.5 },
  { foreground: "muted", background: "surface-raised", required: 4.5 },
  { foreground: "muted", background: "surface-sunken", required: 4.5 },
  { foreground: "body", background: "surface-sunken", required: 4.5 },
  { foreground: "accent", background: "surface", required: 4.5 },
  { foreground: "accent", background: "surface-raised", required: 4.5 },
  { foreground: "accent-ink", background: "accent", required: 4.5 },
  { foreground: "danger", background: "danger-surface", required: 4.5 },
  { foreground: "danger", background: "surface", required: 4.5 },
  { foreground: "warning", background: "warning-surface", required: 4.5 },
  { foreground: "warning", background: "surface", required: 4.5 },
  { foreground: "border", background: "surface", required: 3 },
  { foreground: "border", background: "surface-raised", required: 3 },
  { foreground: "focus", background: "surface", required: 3 },
  { foreground: "focus", background: "surface-raised", required: 3 },
  { foreground: "accent", background: "surface-sunken", required: 3 },
];

describe("見た目のトークンの対比（FR-004 / SC-006）", () => {
  const colors = themeColors(css);

  it("@theme から色トークンを読めている", () => {
    expect(colors.size).toBeGreaterThan(0);
  });

  for (const { foreground, background, required } of pairs) {
    it(`${foreground} / ${background} が ${String(required)}:1 以上である`, () => {
      const front = colors.get(foreground);
      const back = colors.get(background);

      // 写し（契約の表）と実体（CSS）のずれを検出する。表に載っているのに
      // CSS に無いトークンは、検査したつもりで検査できていない状態である。
      expect(
        front,
        `--color-${foreground} が web/src/index.css の @theme にない`,
      ).toBeDefined();
      expect(
        back,
        `--color-${background} が web/src/index.css の @theme にない`,
      ).toBeDefined();
      if (front === undefined || back === undefined) {
        return;
      }

      const actual = contrast(front, back);
      expect(
        actual,
        `${foreground}(${front}) / ${background}(${back}) の対比は ` +
          `${actual.toFixed(2)}:1 で、必要な ${String(required)}:1 に足りない`,
      ).toBeGreaterThanOrEqual(required);
    });
  }
});
