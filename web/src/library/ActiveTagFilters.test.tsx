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

    expect(screen.getByRole("button", { name: "タグの絞り込みを外す" })).toBeDefined();

    await act(async () => {
      resolveTags?.(
        json({ items: [{ id: 1, name: "旅行", synonyms: [], videoCount: 1 }] }),
      );
      await Promise.resolve();
    });

    expect(screen.getByRole("button", { name: "旅行の絞り込みを外す" })).toBeDefined();
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
          <input ref={search} aria-label="動画を検索" />
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
    const button = await screen.findByRole("button", { name: "旅行の絞り込みを外す" });
    button.focus();
    fireEvent.click(button);
    expect(screen.queryByRole("button", { name: "旅行の絞り込みを外す" })).toBeNull();
    expect(document.activeElement).toBe(
      screen.getByRole("textbox", { name: "動画を検索" }),
    );
  });
});
