import { afterEach, describe, expect, it, vi } from "vitest";

import { readHome, readHomeShelves } from "./home";

// ホームの画面 API のクライアント（docs/design-docs/home.md）。

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("home API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("棚の一覧を GET /api/home で読む", async () => {
    const fetchMock = vi.fn<typeof fetch>(() => Promise.resolve(json({ shelves: [] })));
    vi.stubGlobal("fetch", fetchMock);

    expect(await readHome()).toEqual({ shelves: [] });
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe("/api/home");
  });

  it("棚の鍵を , で区切って送り、項目を kind で読み分けられる形にして、中身の欠けた項目を捨てる", async () => {
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(
        json({
          shelves: [
            {
              key: "folder:3",
              total: 2,
              items: [
                { kind: "video", video: { id: 5, title: "Harbour" } },
                { kind: "video" },
              ],
            },
          ],
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const shelves = await readHomeShelves(["folder:3", "recent"]);

    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), "http://localhost");
    expect(url.pathname).toBe("/api/home/shelves");
    expect(url.searchParams.get("keys")).toBe("folder:3,recent");
    expect(shelves).toEqual([
      {
        key: "folder:3",
        total: 2,
        items: [{ kind: "video", video: { id: 5, title: "Harbour" } }],
      },
    ]);
  });
});
