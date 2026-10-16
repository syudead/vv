import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { __resetTagKindsForTest, refreshTagKinds } from "../api/tagKinds";
import { ToastProvider } from "../ui/Toast";
import { useLibraryTagKinds } from "./useLibraryTagKinds";

// URL の `tagKind` の種類が無いときの確かめ（specs/051-tag-kinds/ui-design.md「Library」）。
// 種類の一覧は URL が書き換わる前にも届き直すが、URL から外すのは 1 度だけにする。

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const kind = (id: number, name: string) => ({
  id,
  name,
  tagCount: 0,
  createdAt: "2026-10-01T00:00:00Z",
});

const fetchMock = vi.fn<typeof fetch>();

function wrapper({ children }: { children: ReactNode }) {
  return <ToastProvider>{children}</ToastProvider>;
}

beforeEach(() => {
  __resetTagKindsForTest();
  fetchMock.mockImplementation(() => Promise.resolve(json({ items: [kind(1, "Work")] })));
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  fetchMock.mockReset();
  vi.unstubAllGlobals();
});

describe("useLibraryTagKinds", () => {
  it("URL が書き換わる前に一覧が届き直しても、外すのは 1 度だけ", async () => {
    const apply = vi.fn();
    renderHook(() => useLibraryTagKinds(9, apply), { wrapper });
    await waitFor(() => expect(apply).toHaveBeenCalledTimes(1));
    expect(apply).toHaveBeenCalledWith(expect.anything(), "replace");

    // apply は URL を変えない偽物なので、tagKind は 9 のまま一覧だけが新しくなる。
    await act(async () => {
      await refreshTagKinds();
    });
    expect(apply).toHaveBeenCalledTimes(1);
  });

  it("古い保持で見つからないときの取り直しは、StrictMode の二度の実行でも 1 度だけ", async () => {
    await refreshTagKinds();
    fetchMock.mockClear();
    // 取り直しは失敗させ、一覧を古い保持のままにする（外さずに待つ）。
    fetchMock.mockImplementation(() => Promise.resolve(json({}, 500)));
    const apply = vi.fn();

    renderHook(() => useLibraryTagKinds(9, apply), {
      wrapper,
      reactStrictMode: true,
    });

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    await act(async () => {
      await Promise.resolve();
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(apply).not.toHaveBeenCalled();
  });
});
