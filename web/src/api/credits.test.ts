import { afterEach, describe, expect, it, vi } from "vitest";

import { updateCredit } from "./credits";
import { clearListSnapshot, saveListSnapshot, takeListSnapshot } from "./listSnapshot";

function holdSnapshot() {
  saveListSnapshot({ query: "" }, { items: [], total: 0, hasMore: false, scrollY: 0 });
}

function answer(status: number) {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(() =>
      Promise.resolve(
        new Response(JSON.stringify({ id: 1, name: "新しい名前", code: "internal" }), {
          status,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    ),
  );
}

afterEach(() => {
  clearListSnapshot();
  vi.unstubAllGlobals();
});

// Cards show credit names and faces, so a saved list must not come back with old ones.
describe("クレジットの変更と一覧の控え", () => {
  it("変更に成功したら一覧の控えを捨てる", async () => {
    holdSnapshot();
    answer(200);
    await updateCredit("person", 1, { name: "新しい名前" });
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
  });

  it("失敗したら控えを残す", async () => {
    holdSnapshot();
    answer(500);
    await expect(updateCredit("person", 1, { name: "新しい名前" })).rejects.toThrow();
    expect(takeListSnapshot({ query: "" })).toBeDefined();
  });
});
