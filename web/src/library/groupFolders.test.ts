import { beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup } from "../api/client";

const { getGroup } = vi.hoisted(() => ({ getGroup: vi.fn() }));

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  getGroup,
}));

const { RequestFailed } = await import("../api/client");
const { withGroupFolders } = await import("./groupFolders");

function detail(id: number, folder?: { rootId: number; path: string }) {
  return {
    group: { id, folder, videoIds: [] } as unknown as LibraryGroup,
    trail: [],
    items: [],
  };
}

beforeEach(() => {
  getGroup.mockReset();
});

describe("withGroupFolders", () => {
  it("読み込んでいないグループのフォルダを引いて足し、手で作ったグループと消えたグループは足さない", async () => {
    getGroup.mockImplementation((id: number) => {
      if (id === 6) return Promise.resolve(detail(6, { rootId: 3, path: "x" }));
      if (id === 7) return Promise.resolve(detail(7));
      return Promise.reject(new RequestFailed(404, "not_found", "gone"));
    });
    const folders = await withGroupFolders([{ rootId: 3, path: "series" }], [6, 7, 8]);
    expect(folders).toEqual([
      { rootId: 3, path: "series" },
      { rootId: 3, path: "x" },
    ]);
    expect(getGroup.mock.calls.map((call) => call[0] as number)).toEqual([6, 7, 8]);
  });

  it("404 以外の失敗はそのまま投げる", async () => {
    getGroup.mockRejectedValue(new RequestFailed(500, "internal", "boom"));
    await expect(withGroupFolders([], [6])).rejects.toBeInstanceOf(RequestFailed);
  });

  it("引くグループが無ければ要求しない", async () => {
    expect(await withGroupFolders([], [])).toEqual([]);
    expect(getGroup).not.toHaveBeenCalled();
  });
});
