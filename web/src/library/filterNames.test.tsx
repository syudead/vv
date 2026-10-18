import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { __resetTagsForTest } from "../api/tags";
import { useCreditName, useTagNames } from "./filterNames";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function tag(id: number, name: string) {
  return { id, name, synonyms: [], videoCount: 1 };
}

const fetchMock = vi.fn<typeof fetch>();

function paths(): string[] {
  return fetchMock.mock.calls.map(
    ([input]) => new URL(String(input), "http://localhost").pathname,
  );
}

beforeEach(() => {
  __resetTagsForTest();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  fetchMock.mockReset();
  vi.unstubAllGlobals();
});

describe("useTagNames", () => {
  it("確かめ直した一覧に無いタグがあれば 1 度だけ取り直し、その名前を引く", async () => {
    fetchMock
      .mockResolvedValueOnce(json({ items: [tag(1, "猫")] }))
      .mockResolvedValueOnce(json({ items: [tag(1, "猫"), tag(2, "犬")] }));
    const { result } = renderHook(() => useTagNames([1, 2]));

    await waitFor(() => expect(result.current.get(2)?.name).toBe("犬"));
    expect(paths()).toEqual(["/api/tags", "/api/tags"]);
  });

  it("取り直しても無いタグで取り直しを繰り返さない", async () => {
    fetchMock.mockImplementation(() => Promise.resolve(json({ items: [tag(1, "猫")] })));
    const { result } = renderHook(() => useTagNames([1, 7]));

    await waitFor(() => expect(paths()).toHaveLength(2));
    expect(result.current.get(1)?.name).toBe("猫");
    expect(result.current.has(7)).toBe(false);
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(paths()).toHaveLength(2);
  });

  it("一覧を読めなければ空のまま（名前の代わりの Skeleton）で、取り直しの失敗も投げない", async () => {
    fetchMock.mockResolvedValue(json({ code: "internal", message: "boom" }, 500));
    const { result } = renderHook(() => useTagNames([1]));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(result.current.size).toBe(0);
  });
});

describe("useCreditName", () => {
  const alice = { id: 1, name: "Alice", aliases: [], externalIds: [], videoCount: 1 };

  it("名前を読み、id が変わると次の名前が届くまで undefined を返す", async () => {
    let resolveSecond: ((response: Response) => void) | undefined;
    fetchMock.mockResolvedValueOnce(json(alice)).mockReturnValueOnce(
      new Promise<Response>((resolve) => {
        resolveSecond = resolve;
      }),
    );
    const onGone = vi.fn();
    const { result, rerender } = renderHook(
      ({ id }) => useCreditName("person", id, onGone),
      { initialProps: { id: 1 } },
    );

    await waitFor(() => expect(result.current).toBe("Alice"));
    rerender({ id: 2 });
    expect(result.current).toBeUndefined();
    resolveSecond?.(json({ ...alice, id: 2, name: "Bob" }));
    await waitFor(() => expect(result.current).toBe("Bob"));
    expect(paths()).toEqual(["/api/people/1", "/api/people/2"]);
    expect(onGone).not.toHaveBeenCalled();
  });

  it("404 なら onGone を呼び、ほかの失敗では呼ばない", async () => {
    fetchMock
      .mockResolvedValueOnce(json({ code: "not_found", message: "gone" }, 404))
      .mockResolvedValueOnce(json({ code: "internal", message: "boom" }, 500));
    const onGone = vi.fn();
    const gone = renderHook(() => useCreditName("organization", 8, onGone));
    await waitFor(() => expect(onGone).toHaveBeenCalledWith("organization"));
    gone.unmount();

    onGone.mockClear();
    const failed = renderHook(() => useCreditName("person", 9, onGone));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(onGone).not.toHaveBeenCalled();
    expect(failed.result.current).toBeUndefined();
  });

  it("届く前に外れたら、打ち切った要求の失敗で onGone を呼ばない", async () => {
    fetchMock.mockImplementation(
      (_input, init) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () =>
            reject(new DOMException("aborted", "AbortError")),
          );
        }),
    );
    const onGone = vi.fn();
    const { unmount } = renderHook(() => useCreditName("person", 1, onGone));
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    unmount();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(onGone).not.toHaveBeenCalled();
  });
});
