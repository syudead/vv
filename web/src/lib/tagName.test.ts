import { describe, expect, it } from "vitest";

import { t } from "../i18n";
import { nameReason } from "./tagName";

describe("nameReason の制御文字", () => {
  it.each([
    ["U+0000", "a\u0000b"],
    ["U+001F", "a\u001fb"],
    ["U+007F", "a\u007fb"],
    ["U+0085", "a\u0085b"],
    ["U+009F", "a\u009fb"],
  ])("%s を含む名前を拒む", (_, name) => {
    expect(nameReason(name)).toBe(t.tagName.controlCharacters);
  });

  it.each([
    ["U+00A0", "a b"],
    ["U+200B", "a​b"],
    ["U+0020", "a b"],
  ])("制御文字でない %s は拒まない", (_, name) => {
    expect(nameReason(name)).toBeNull();
  });
});
