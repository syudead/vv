// @vitest-environment node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 見た目の値は src/ui/tokens.css の @theme にだけ置く（specs/038-design-system/research.md R-5）。
 * 画面側に生の色が散ると、配色を変えるときに取りこぼす。shadcn が上流のテーマの CSS 変数を
 * index.css に書き足したときも、ここで落ちる。
 */

const root = join(__dirname, "..");

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return name === "gen" ? [] : walk(path);
    return /\.(tsx?|css)$/.test(name) && !name.includes(".test.") ? [path] : [];
  });
}

const rawColor = /#[0-9a-f]{3,8}\b|\brgba?\(|\bhsla?\(|\boklch\(/i;

describe("design tokens", () => {
  const tokens = join(root, "ui", "tokens.css");
  const files = walk(root).filter((path) => path !== tokens);

  it("tokens.css 以外に生の色を書かない", () => {
    const offenders = files.filter((path) => rawColor.test(readFileSync(path, "utf8")));
    expect(offenders.map((path) => path.slice(root.length))).toEqual([]);
  });

  it("Tailwind 既定のパレット名を使わない", () => {
    const palette =
      /\b(?:bg|text|border|ring|from|to|via|fill|stroke)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}\b/;
    const offenders = files.filter((path) => palette.test(readFileSync(path, "utf8")));
    expect(offenders.map((path) => path.slice(root.length))).toEqual([]);
  });
});

/** relativeLuminance / contrast は WCAG 2 の定義。 */
function luminance(hex: string): number {
  const value = hex.length === 4 ? hex.replace(/[0-9a-f]/gi, (c) => c + c) : hex;
  const channel = (i: number) => {
    const c = parseInt(value.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(1) + 0.7152 * channel(3) + 0.0722 * channel(5);
}

function contrast(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

describe("contrast", () => {
  const css = readFileSync(join(root, "ui", "tokens.css"), "utf8");
  const token = (name: string): string => {
    const match = new RegExp(`--color-${name}:\\s*(#[0-9a-f]{6})`, "i").exec(css);
    const hex = match?.[1];
    if (hex === undefined)
      throw new Error(`--color-${name} が tokens.css に 6 桁の hex で無い`);
    return hex;
  };

  // 面の 5 段階（暗い順）。どの面の上でも本文と補足の文字が読める。
  const surfaces = ["navbar", "background", "muted", "card", "popover"];
  // 色の役割の割り当ては docs/design-docs/design-system.md の Foundations。
  const pairs: [string, string, number][] = [
    ...surfaces.flatMap((surface): [string, string, number][] => [
      ["foreground", surface, 4.5],
      ["muted-foreground", surface, 4.5],
      // 失敗・注意・完了の行と、お気に入りのオンのハートの塗り
      // （specs/035-favorites/ui-design.md「Colour」）。
      ["destructive", surface, 4.5],
      ["warning", surface, 4.5],
      ["success", surface, 4.5],
      ["favorite", surface, 4.5],
      // 共通の操作の境界とキーボードのフォーカス輪郭（非文字の対比）。
      ["input", surface, 3],
      ["ring", surface, 3],
      // 主操作の塗り・選択の印・進行の帯（非文字の対比）。
      ["primary", surface, 4.5],
      ["primary-active", surface, 3],
    ]),
    ["card-foreground", "card", 4.5],
    ["popover-foreground", "popover", 4.5],
    ["secondary-foreground", "secondary", 4.5],
    ["muted-foreground", "secondary", 4.5],
    ["accent-foreground", "accent", 4.5],
    ["muted-foreground", "accent", 4.5],
    ["primary-foreground", "primary", 4.5],
    ["primary-foreground", "primary-hover", 4.5],
    ["primary-foreground", "primary-active", 4.5],
    // 選んだ行・効いている絞り込みのチップの面の上の文字と印
    // （specs/036-tag-admin-scale/ui-design.md「Colour」）。
    ["foreground", "primary-soft", 4.5],
    ["muted-foreground", "primary-soft", 4.5],
    ["primary", "primary-soft", 4.5],
    ["ring", "primary-soft", 4.5],
    ["favorite", "primary-soft", 4.5],
    ["destructive-foreground", "destructive-strong", 4.5],
    ["warning-foreground", "warning", 4.5],
    ["success-foreground", "success", 4.5],
    ["destructive", "destructive-soft", 4.5],
    ["warning", "warning-soft", 4.5],
    ["success", "success-soft", 4.5],
    // 動画詳細画面の段階表示・再生失敗（プレイヤーの上の不透明な面）のリンク。
    ["ring", "navbar", 4.5],
    ["ring", "background", 4.5],
    ["ring", "card", 4.5],
    // ツールチップは明るい面に暗い文字で出す。
    ["background", "foreground", 4.5],
  ];

  it.each(pairs)("%s on %s >= %s", (fg, bg, minimum) => {
    expect(contrast(token(fg), token(bg))).toBeGreaterThanOrEqual(minimum);
  });
});
