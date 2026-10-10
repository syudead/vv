import { describe, expect, it } from "vitest";

import { entryAction, entryPosition, folderLine } from "./historyEntry";

const updatedAt = "2026-09-27T12:00:00Z";

describe("entryPosition", () => {
  it("途中の動画は位置の比率のバーと位置・長さの文字", () => {
    expect(
      entryPosition({
        durationMs: 2_538_000,
        progress: { positionMs: 965_000, completed: false, updatedAt },
      }),
    ).toEqual({ value: 965_000, max: 2_538_000, text: "16:05 / 42:18" });
  });

  it("見終わった動画はバーを満たし、文字は保存された位置のまま", () => {
    expect(
      entryPosition({
        durationMs: 2_538_000,
        progress: { positionMs: 2_530_000, completed: true, updatedAt },
      }),
    ).toEqual({ value: 2_538_000, max: 2_538_000, text: "42:10 / 42:18" });
  });

  it("位置か長さが無ければ出さない", () => {
    expect(entryPosition({ durationMs: 60_000 })).toBeNull();
    expect(
      entryPosition({ progress: { positionMs: 1_000, completed: false, updatedAt } }),
    ).toBeNull();
    expect(
      entryPosition({
        durationMs: 0,
        progress: { positionMs: 1_000, completed: false, updatedAt },
      }),
    ).toBeNull();
  });
});

describe("entryAction", () => {
  it("途中は Resume、見終わりは Start over、位置が無ければ無し", () => {
    expect(
      entryAction({ progress: { positionMs: 1_000, completed: false, updatedAt } }),
    ).toBe("resume");
    expect(
      entryAction({ progress: { positionMs: 1_000, completed: true, updatedAt } }),
    ).toBe("startOver");
    expect(entryAction({})).toBeNull();
  });
});

describe("folderLine", () => {
  it("フォルダのパスを / で区切って書き、直下なら出さない", () => {
    expect(folderLine({ folder: { rootId: 1, path: "Travel/2024" } })).toBe(
      "Travel / 2024",
    );
    expect(folderLine({ folder: { rootId: 1, path: "" } })).toBeNull();
    expect(folderLine({})).toBeNull();
  });
});
