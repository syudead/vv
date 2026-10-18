// @vitest-environment node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * Keep visual values in src/ui/tokens.css @theme; reject raw colors and upstream theme additions
 * elsewhere.
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

/** Use WCAG 2 relative luminance and contrast definitions. */
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

  const surfaces = ["navbar", "background", "muted", "card", "popover"];
  // Token roles are documented in design-system.md, Foundations.
  const pairs: [string, string, number][] = [
    ...surfaces.flatMap((surface): [string, string, number][] => [
      ["foreground", surface, 4.5],
      ["muted-foreground", surface, 4.5],
      //
      ["destructive", surface, 4.5],
      ["warning", surface, 4.5],
      ["success", surface, 4.5],
      ["favorite", surface, 4.5],
      ["input", surface, 3],
      ["ring", surface, 3],
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
    //
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
    ["ring", "navbar", 4.5],
    ["ring", "background", 4.5],
    ["ring", "card", 4.5],
    ["background", "foreground", 4.5],
  ];

  it.each(pairs)("%s on %s >= %s", (fg, bg, minimum) => {
    expect(contrast(token(fg), token(bg))).toBeGreaterThanOrEqual(minimum);
  });
});
