import { describe, expect, it } from "vitest";

/**
 * 生の色の禁止（FR-001 / contracts/design-tokens.md 1.）。
 *
 * 規則が 1 か所にあること（R-401）は、画面側が「トークン名を使うほかに手が無い」
 * ときだけ保てる。この検査がその禁止の実体である。
 *
 * 走査は Vite の import.meta.glob で行う。?raw は素のテキストを返すので、
 * Node の fs に依存せず（型の依存も増やさず）ソースをそのまま読める。
 */
const sources = import.meta.glob("../**/*.tsx", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

/** 生成物は対象外である（AGENTS.md: docs/generated/ と同じ扱い）。 */
const generatedPrefix = "api/gen/";

/**
 * palettes は Tailwind 既定のパレット名である。役割ではなく色を指しており、
 * 明るい背景を前提にした値が画面ごとに散る原因になる（R-401）。
 */
const palettes = [
  "slate",
  "gray",
  "zinc",
  "neutral",
  "stone",
  "red",
  "orange",
  "amber",
  "yellow",
  "lime",
  "green",
  "emerald",
  "teal",
  "cyan",
  "sky",
  "blue",
  "indigo",
  "violet",
  "purple",
  "fuchsia",
  "pink",
  "rose",
];

/**
 * rules は禁じる書き方である。どれも「その行を見れば直せる」形で報告する。
 *
 * `-(?:tokens)-\d` の前に `[a-z]` を置いていないのは、`bg-`・`text-` などの
 * 接頭辞を列挙し切れないからである。かわりに色名の直前が区切り（行頭・空白・
 * `-`・引用符・`{`）であることだけを見る。
 */
const rules: { name: string; pattern: RegExp }[] = [
  {
    name: "生の 16 進色",
    pattern: /#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?\b/g,
  },
  {
    name: "Tailwind 既定のパレット名",
    pattern: new RegExp(`\\b(?:${palettes.join("|")})-\\d{2,3}\\b`, "g"),
  },
  {
    name: "Tailwind 既定の白黒",
    pattern:
      /\b(?:bg|text|border|fill|stroke|ring|outline|decoration|shadow|from|via|to)-(?:black|white)\b/g,
  },
  {
    name: "生の色関数",
    pattern: /\b(?:rgba?|hsla?|oklch|color-mix)\(/g,
  },
];

interface Violation {
  file: string;
  line: number;
  rule: string;
  text: string;
}

/** scan は 1 ファイルの違反を、直せる形（行番号つき）で返す。 */
function scan(file: string, source: string): Violation[] {
  const violations: Violation[] = [];

  source.split("\n").forEach((line, index) => {
    for (const rule of rules) {
      for (const match of line.matchAll(rule.pattern)) {
        violations.push({
          file,
          line: index + 1,
          rule: rule.name,
          text: match[0],
        });
      }
    }
  });

  return violations;
}

/** format は報告の 1 行を組み立てる。 */
function format(violation: Violation): string {
  return (
    `web/src/${violation.file}:${String(violation.line)} ` +
    `${violation.rule} "${violation.text}"`
  );
}

/** entries は走査対象を web/src からの相対パスで返す。 */
function entries(): { file: string; source: string }[] {
  return Object.entries(sources)
    .map(([key, source]) => ({ file: key.replace(/^\.\.\//, ""), source }))
    .filter(({ file }) => !file.startsWith(generatedPrefix))
    .sort((left, right) => left.file.localeCompare(right.file));
}

describe("生の色を画面に書かない（FR-001）", () => {
  const targets = entries();

  it("走査対象を読めている", () => {
    expect(targets.length).toBeGreaterThan(0);
  });

  // 猶予（pendingRewrite）はもう無い。US2（T024〜T026）で VideoPage が
  // トークンへ移り、走査対象のすべてが素通りするようになった。
  for (const { file, source } of targets) {
    it(`${file} に生の色が無い`, () => {
      const violations = scan(file, source);
      expect(violations.map(format), violations.map(format).join("\n")).toEqual([]);
    });
  }
});
