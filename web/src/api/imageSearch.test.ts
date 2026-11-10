import { afterEach, describe, expect, it, vi } from "vitest";

import { searchByImage } from "./imageSearch";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("画像検索のクライアント", () => {
  it("ファイルを file の 1 パートで送り、中断の signal を渡す", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({ query: { kind: "image" }, items: [], unindexedCount: 2 }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      ),
    );
    vi.stubGlobal("fetch", fetch);
    const controller = new AbortController();
    const file = new File(["png"], "shot.png", { type: "image/png" });

    const result = await searchByImage(file, controller.signal);

    expect(result.unindexedCount).toBe(2);
    const [path, init] = fetch.mock.calls[0] ?? [];
    expect(path).toBe("/api/image-search");
    expect(init?.method).toBe("POST");
    expect(init?.signal).toBe(controller.signal);
    const body = init?.body as FormData;
    expect([...body.keys()]).toEqual(["file"]);
    expect((body.get("file") as File).name).toBe("shot.png");
  });
});
