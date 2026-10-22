import { afterEach, expect, it, vi } from "vitest";

import { readGroupPage, listGroupItems } from "./groups";
import { readSmartFolderPage, listSmartFolderItems } from "./smartFolders";

afterEach(() => vi.unstubAllGlobals());

it("グループとスマートフォルダの先頭・次ページへ同じ一時条件を渡す", async () => {
  const fetch = vi
    .fn<typeof globalThis.fetch>()
    .mockImplementation(
      async () => new Response("{}", { headers: { "Content-Type": "application/json" } }),
    );
  vi.stubGlobal("fetch", fetch);
  const criteria = {
    favorite: true,
    sort: "random",
    seed: 37,
  } as const;
  await readGroupPage(1, [9], undefined, "clip", criteria);
  await listGroupItems(1, "next", undefined, undefined, "clip", criteria);
  await readSmartFolderPage(1, undefined, undefined, "clip", criteria);
  await listSmartFolderItems(1, "next", undefined, undefined, "clip", criteria);
  for (const [path] of fetch.mock.calls) {
    const params = new URL(String(path), "https://vv.test").searchParams;
    expect(Object.fromEntries(params)).toMatchObject({
      query: "clip",
      favorite: "true",
      sort: "random",
      seed: "37",
    });
  }
  expect(
    new URL(String(fetch.mock.calls[0]?.[0]), "https://vv.test").searchParams.get(
      "trail",
    ),
  ).toBe("9");
  for (const index of [1, 3])
    expect(
      new URL(String(fetch.mock.calls[index]?.[0]), "https://vv.test").searchParams.get(
        "cursor",
      ),
    ).toBe("next");
});
