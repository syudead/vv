import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { FolderListing, FolderRef } from "./client";

const { getFolder } = vi.hoisted(() => ({ getFolder: vi.fn() }));

vi.mock("./client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./client")>()),
  getFolder,
}));

const { useFolderListing } = await import("./useFolderListing");

function listing(path: string): FolderListing {
  return { folder: { rootId: 1, path } } as unknown as FolderListing;
}

describe("useFolderListing の取り直し", () => {
  beforeEach(() => {
    getFolder.mockReset();
    getFolder.mockImplementation((folder: FolderRef) =>
      Promise.resolve(listing(folder.path)),
    );
  });

  it("同じフォルダを指す間は描画し直しても取り直さない", async () => {
    const { result, rerender } = renderHook(
      ({ folder }: { folder: FolderRef }) => useFolderListing(folder),
      { initialProps: { folder: { rootId: 1, path: "a" } } },
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(getFolder).toHaveBeenCalledTimes(1);

    // 呼び出し元が描画のたびに新しいオブジェクトを渡しても、同じフォルダなら取らない。
    rerender({ folder: { rootId: 1, path: "a" } });
    rerender({ folder: { rootId: 1, path: "a" } });
    expect(getFolder).toHaveBeenCalledTimes(1);
  });

  it("フォルダが変わったときと reload のときだけ取り直す", async () => {
    const { result, rerender } = renderHook(
      ({ folder }: { folder: FolderRef }) => useFolderListing(folder),
      { initialProps: { folder: { rootId: 1, path: "a" } } },
    );
    await waitFor(() => expect(result.current.loading).toBe(false));

    rerender({ folder: { rootId: 1, path: "b" } });
    await waitFor(() => expect(result.current.data).toEqual(listing("b")));
    expect(getFolder).toHaveBeenCalledTimes(2);
    expect(getFolder).toHaveBeenLastCalledWith(
      { rootId: 1, path: "b" },
      expect.any(AbortSignal),
    );

    act(() => result.current.reload());
    await waitFor(() => expect(getFolder).toHaveBeenCalledTimes(3));
    expect(getFolder).toHaveBeenLastCalledWith(
      { rootId: 1, path: "b" },
      expect.any(AbortSignal),
    );
  });

  it("控え（seed）から始めたときは reload まで取りに行かない", async () => {
    const { result, rerender } = renderHook(
      ({ folder }: { folder: FolderRef }) => useFolderListing(folder, listing("a")),
      { initialProps: { folder: { rootId: 1, path: "a" } } },
    );
    rerender({ folder: { rootId: 1, path: "a" } });
    expect(getFolder).not.toHaveBeenCalled();
    expect(result.current.data).toEqual(listing("a"));

    act(() => result.current.reload());
    await waitFor(() => expect(getFolder).toHaveBeenCalledTimes(1));
  });
});
