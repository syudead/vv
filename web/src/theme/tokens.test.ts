// @vitest-environment node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 見た目の値は src/index.css の @theme にだけ置く。
 * 画面側に生の色が散ると、配色を変えるときに取りこぼす。
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
  const files = walk(root).filter((path) => !path.endsWith("index.css"));

  it("index.css 以外に生の色を書かない", () => {
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
  const css = readFileSync(join(root, "index.css"), "utf8");
  const token = (name: string): string => {
    const match = new RegExp(`--color-${name}:\\s*(#[0-9a-f]{6})`, "i").exec(css);
    const hex = match?.[1];
    if (hex === undefined) throw new Error(`--color-${name} が index.css に無い`);
    return hex;
  };

  const pairs: [string, string, number][] = [
    ["fg", "bg", 4.5],
    ["fg", "surface", 4.5],
    ["fg", "elevated", 4.5],
    ["fg", "field", 4.5],
    ["fg-muted", "bg", 4.5],
    ["fg-muted", "surface", 4.5],
    ["fg-muted", "elevated", 4.5],
    ["fg-subtle", "elevated", 4.5],
    ["fg-subtle", "surface", 4.5],
    ["accent-fg", "accent", 4.5],
    ["accent-fg", "accent-hover", 4.5],
    ["accent-fg", "accent-active", 4.5],
    ["danger-fg", "danger-strong", 4.5],
    ["link", "bg", 4.5],
    ["link", "surface", 4.5],
    ["link", "navbar", 3],
    ["link", "elevated", 3],
    ["link", "field", 3],
    ["link", "accent-soft", 4.5],
    // タグ管理画面の選んだ行の面の上の名前と本数・シノニム
    // （specs/036-tag-admin-scale/ui-design.md「Colour」）。
    ["fg", "accent-soft", 4.5],
    ["fg-muted", "accent-soft", 4.5],
    ["danger", "danger-soft", 4.5],
    ["warning", "warning-soft", 4.5],
    ["success", "success-soft", 4.5],
    ["danger", "bg", 4.5],
    // 選択バーのタグ操作ポップオーバーと管理画面の窓（ModalFrame は bg-elevated）の
    // 中の理由・失敗の行（specs/014-video-tags/ui-design.md「Accessibility」）。
    ["danger", "elevated", 4.5],
    ["warning", "bg", 4.5],
    ["success", "bg", 4.5],
    ["fg", "navbar", 4.5],
    // 動画詳細画面の段階表示・読み取り失敗・再生失敗の説明（プレイヤーの上の不透明な面）。
    ["fg-muted", "navbar", 4.5],
    // 読み取り失敗・開けなかったときの文言（プレイヤーの上の不透明な面）。
    ["danger", "navbar", 4.5],
    ["accent", "navbar", 4.5],
    ["success", "navbar", 4.5],
    ["bg", "fg", 4.5],
    // 初回設定画面とログイン画面の面（bg-surface）の上の失敗の行・接続の警告・
    // 「ログインへ」（specs/016-single-account-auth/ui-design.md「Accessibility」）。
    ["danger", "surface", 4.5],
    ["warning", "surface", 4.5],
    // 共通ボタン・選択部品・入力欄の境界とフォーカス輪郭。
    ["control-border", "surface", 3],
    ["control-border", "elevated", 3],
    ["control-border", "field", 3],
    ["accent", "surface", 3],
    ["accent", "navbar", 3],
    ["accent-active", "surface", 3],
    ["accent-active", "navbar", 3],
    // 設定画面の「API tokens」節の bg-field の箱の上の文字（平文の表示の見出しと注意、
    // 失効の窓の対象）（specs/026-external-api/ui-design.md「Colour」）。
    ["fg-muted", "field", 4.5],
    ["warning", "field", 4.5],
  ];

  it.each(pairs)("%s on %s >= %s", (fg, bg, minimum) => {
    expect(contrast(token(fg), token(bg))).toBeGreaterThanOrEqual(minimum);
  });
});
