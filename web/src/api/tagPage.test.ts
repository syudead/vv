import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { __resetTagsForTest, currentTags, listTagPage, tagPageLimit } from "./tags";

// listTagPage（specs/036-tag-admin-scale/contracts/screen-api.md §4・§5）。

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const page = {
  items: [
    {
      id: 1,
      name: "旅行",
      synonyms: [],
      videoCount: 0,
      tentative: false,
      createdAt: "2026-01-01T00:00:00Z",
    },
  ],
  total: 1,
  totalAll: 3,
  nextCursor: "next",
};

beforeEach(() => {
  __resetTagsForTest();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("listTagPage", () => {
  it("既定の件数で先頭のページを読み、応答をそのまま返す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse(page));
    vi.stubGlobal("fetch", fetch);

    await expect(listTagPage({})).resolves.toEqual(page);
    expect(fetch.mock.calls[0]?.[0]).toBe(`/api/tags?limit=${tagPageLimit}`);
    expect(tagPageLimit).toBe(100);
    // 共有の保持には触れない。
    expect(currentTags()).toBeUndefined();
  });

  it("条件とカーソルをパラメータにし、呼び手の AbortSignal を渡す", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse(page));
    vi.stubGlobal("fetch", fetch);
    const controller = new AbortController();

    await listTagPage(
      {
        q: "猫 犬",
        tentative: true,
        unused: true,
        sort: "countDesc",
        cursor: "abc",
        limit: 50,
      },
      controller.signal,
    );
    const url = new URL(String(fetch.mock.calls[0]?.[0]), "http://localhost");
    expect(url.pathname).toBe("/api/tags");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      q: "猫 犬",
      tentative: "true",
      unused: "true",
      sort: "countDesc",
      cursor: "abc",
      limit: "50",
    });
    expect(fetch.mock.calls[0]?.[1]?.signal).toBe(controller.signal);
  });

  it("空の検索語・偽の絞り込み・空のカーソルは送らない", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse(page));
    vi.stubGlobal("fetch", fetch);

    await listTagPage({
      q: "",
      tentative: false,
      unused: false,
      cursor: "",
      sort: "name",
    });
    expect(fetch.mock.calls[0]?.[0]).toBe(`/api/tags?sort=name&limit=${tagPageLimit}`);
  });
});
