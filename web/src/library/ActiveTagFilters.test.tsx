import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useRef, useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { __resetTagsForTest } from "../api/tags";
import ActiveTagFilters from "./ActiveTagFilters";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("ActiveTagFilters", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    __resetTagsForTest();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("タグの一覧をまだ取得していない間も読み上げ名を持つ（N2）", async () => {
    let resolveTags: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation(
      () =>
        new Promise<Response>((resolve) => {
          resolveTags = resolve;
        }),
    );
    render(
      <ActiveTagFilters
        tagIds={[1]}
        onRemove={vi.fn()}
        searchFieldRef={{ current: null }}
      />,
    );

    expect(screen.getByRole("button", { name: "Remove the tag filter" })).toBeDefined();

    await act(async () => {
      resolveTags?.(
        json({ items: [{ id: 1, name: "旅行", synonyms: [], videoCount: 1 }] }),
      );
      await Promise.resolve();
    });

    expect(
      screen.getByRole("button", { name: "Remove the filter for 旅行" }),
    ).toBeDefined();
  });

  it("使い回した一覧に無いタグで絞り込むと、一覧を取り直して名前を出す", async () => {
    const travel = { id: 1, name: "旅行", synonyms: [], videoCount: 1 };
    const created = { id: 2, name: "新しいタグ", synonyms: [], videoCount: 1 };
    fetchMock
      .mockResolvedValueOnce(json({ items: [travel] }))
      .mockResolvedValue(json({ items: [travel, created] }));
    const { rerender } = render(
      <ActiveTagFilters
        tagIds={[1]}
        onRemove={vi.fn()}
        searchFieldRef={{ current: null }}
      />,
    );
    expect(
      await screen.findByRole("button", { name: "Remove the filter for 旅行" }),
    ).toBeDefined();

    // 届いた直後（取り直しを省く間）に、その一覧より後に作られたタグで絞り込む。
    rerender(
      <ActiveTagFilters
        tagIds={[2]}
        onRemove={vi.fn()}
        searchFieldRef={{ current: null }}
      />,
    );
    expect(
      await screen.findByRole("button", { name: "Remove the filter for 新しいタグ" }),
    ).toBeDefined();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("古い保持に無いタグで開いたときは、確かめ直しの取得だけで名前を出す", async () => {
    const travel = { id: 1, name: "旅行", synonyms: [], videoCount: 1 };
    const created = { id: 2, name: "新しいタグ", synonyms: [], videoCount: 1 };
    fetchMock
      .mockResolvedValueOnce(json({ items: [travel] }))
      .mockResolvedValue(json({ items: [travel, created] }));
    const first = render(
      <ActiveTagFilters
        tagIds={[1]}
        onRemove={vi.fn()}
        searchFieldRef={{ current: null }}
      />,
    );
    expect(
      await screen.findByRole("button", { name: "Remove the filter for 旅行" }),
    ).toBeDefined();
    first.unmount();

    // 保持は使い回す間（2 秒）より古い。開いたときの確かめ直しが取り直す。
    const now = Date.now();
    vi.spyOn(Date, "now").mockReturnValue(now + 10_000);
    render(
      <ActiveTagFilters
        tagIds={[2]}
        onRemove={vi.fn()}
        searchFieldRef={{ current: null }}
      />,
    );
    expect(
      await screen.findByRole("button", { name: "Remove the filter for 新しいタグ" }),
    ).toBeDefined();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("チップの名前に title を持つ（N3）", async () => {
    fetchMock.mockResolvedValue(
      json({ items: [{ id: 1, name: "旅行", synonyms: [], videoCount: 1 }] }),
    );
    render(
      <ActiveTagFilters
        tagIds={[1]}
        onRemove={vi.fn()}
        searchFieldRef={{ current: null }}
      />,
    );

    const name = await screen.findByText("旅行");
    expect(name.getAttribute("title")).toBe("旅行");
  });

  it("最後のタグを外して行が消えても、フォーカスは検索欄へ移る", async () => {
    fetchMock.mockResolvedValue(
      json({ items: [{ id: 1, name: "旅行", synonyms: [], videoCount: 1 }] }),
    );
    function Harness() {
      const [ids, setIds] = useState<number[]>([1]);
      const search = useRef<HTMLInputElement | null>(null);
      return (
        <>
          <input ref={search} aria-label="Search videos" />
          {ids.length > 0 && (
            <ActiveTagFilters
              tagIds={ids}
              onRemove={(id) => setIds((current) => current.filter((x) => x !== id))}
              searchFieldRef={search}
            />
          )}
        </>
      );
    }
    render(<Harness />);
    const button = await screen.findByRole("button", {
      name: "Remove the filter for 旅行",
    });
    button.focus();
    fireEvent.click(button);
    expect(
      screen.queryByRole("button", { name: "Remove the filter for 旅行" }),
    ).toBeNull();
    expect(document.activeElement).toBe(
      screen.getByRole("textbox", { name: "Search videos" }),
    );
  });
});
