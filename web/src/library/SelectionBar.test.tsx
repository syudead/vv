import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { __resetTagsForTest } from "../api/tags";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import SelectionBar, { selectionReasons } from "./SelectionBar";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const summary = {
  total: 3,
  public: 1,
  tags: [],
  people: [],
  organizations: [],
  dates: [],
};

const editRequests: unknown[] = [];

function install() {
  const fetchMock = vi.fn<typeof fetch>();
  fetchMock.mockImplementation((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url === "/api/tags" && method === "GET") {
      return Promise.resolve(jsonResponse({ items: [] }));
    }
    if (url === "/api/video-edits/summary" && method === "POST") {
      editRequests.push(JSON.parse(String(init?.body)));
      return Promise.resolve(jsonResponse(summary));
    }
    throw new Error(`想定しない要求: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

type BarProps = React.ComponentProps<typeof SelectionBar>;

type DefaultedProps =
  "targets" | "favoriteVideoIds" | "favoriteGroupIds" | "onCollectionsDeleted";

const videoTargets = [1, 2, 3].map((id) => ({
  key: `v${String(id)}`,
  kind: "video" as const,
  id,
  label: `動画 ${String(id)}`,
  videoIds: [id],
}));

function barElement(
  props: Omit<BarProps, DefaultedProps> & Partial<Pick<BarProps, DefaultedProps>>,
) {
  return (
    <TooltipProvider>
      <ToastProvider>
        <SelectionBar
          {...props}
          targets={props.targets ?? videoTargets}
          favoriteVideoIds={props.favoriteVideoIds ?? []}
          favoriteGroupIds={props.favoriteGroupIds ?? []}
          onCollectionsDeleted={props.onCollectionsDeleted ?? vi.fn()}
        />
      </ToastProvider>
    </TooltipProvider>
  );
}

function renderBar(props: Partial<React.ComponentProps<typeof SelectionBar>> = {}) {
  const onClear = vi.fn();
  const onTagRemoved = vi.fn();
  const onBundled = vi.fn();
  const result = render(
    barElement({
      count: 3,
      selectedIds: [1, 2, 3],
      onClear,
      onTagRemoved,
      onBundled,
      ...props,
    }),
  );
  return { ...result, onClear, onTagRemoved, onBundled };
}

beforeEach(() => {
  __resetTagsForTest();
  editRequests.length = 0;
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("SelectionBar", () => {
  it("count が 0 のときは何も描かない", () => {
    install();
    renderBar({ count: 0 });
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();
  });

  it("件数と選択解除を出し、すべて選択は出さない", () => {
    install();
    renderBar();
    expect(screen.getByText("3 videos selected")).toBeDefined();
    expect(screen.getByRole("button", { name: "Clear selection (Esc)" })).toBeDefined();
    expect(screen.queryByRole("button", { name: /Select all/ })).toBeNull();
  });

  // Atomic batch APIs reject oversized selections; disable instead of silently splitting requests.
  it("選択が20,000件を超えると「Edit」は押せず、理由が読める", () => {
    install();
    renderBar({
      count: 20001,
      selectedIds: Array.from({ length: 20001 }, (_, i) => i + 1),
    });
    const edit = screen.getByRole("button", { name: "Edit" }) as HTMLButtonElement;
    expect(edit.disabled).toBe(true);
    expect(edit.title).toBe("Select between 1 and 20,000 videos.");
    const describedBy = edit.getAttribute("aria-describedby");
    expect(document.getElementById(describedBy!)?.textContent).toBe(
      "Select between 1 and 20,000 videos.",
    );
  });

  it("選択が20,000件ちょうどなら「Edit」を押せる", () => {
    install();
    renderBar({
      count: 20000,
      selectedIds: Array.from({ length: 20000 }, (_, i) => i + 1),
    });
    expect(
      (screen.getByRole("button", { name: "Edit" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("「Edit」は選んだカードで一括エディタを開き、閉じても選択を残す", async () => {
    const user = userEvent.setup();
    install();
    const { onClear } = renderBar({ smartFolderIds: [9] });
    await user.click(screen.getByRole("button", { name: "Edit" }));
    const editor = await screen.findByRole("dialog", { name: "Edit videos" });
    expect(
      within(within(editor).getByRole("list", { name: "Videos to edit" }))
        .getAllByRole("button")
        .map((tile) => tile.getAttribute("aria-label")),
    ).toEqual(["動画 1, edited", "動画 2, edited", "動画 3, edited"]);
    await waitFor(() => expect(editRequests).toEqual([{ videoIds: [1, 2, 3] }]));

    await user.click(within(editor).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(onClear).not.toHaveBeenCalled();
    expect(screen.getByText("3 videos and 1 smart folder selected")).toBeDefined();
  });

  it("グループのページの「Remove from group」は、外す項目が 20,000 件を超えると disabled で理由が読める", () => {
    install();
    const onRemove = vi.fn();
    const { rerender } = renderBar({
      removeFromGroup: { count: 20000, pending: false, onRemove },
    });
    const button = () =>
      screen.getByRole("button", { name: "Remove from group" }) as HTMLButtonElement;
    expect(button().disabled).toBe(false);
    expect(button().getAttribute("aria-describedby")).toBeNull();

    rerender(
      barElement({
        count: 3,
        selectedIds: [1, 2, 3],
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
        removeFromGroup: { count: 20001, pending: false, onRemove },
      }),
    );
    expect(button().disabled).toBe(true);
    expect(button().title).toBe("Select between 1 and 20,000 videos.");
    const describedBy = button().getAttribute("aria-describedby");
    expect(describedBy).not.toBeNull();
    expect(document.getElementById(describedBy!)?.textContent).toBe(
      "Select between 1 and 20,000 videos.",
    );
  });

  it("選択を解除ボタンで onClear を呼ぶ", async () => {
    const user = userEvent.setup();
    install();
    const { onClear } = renderBar();
    await user.click(screen.getByRole("button", { name: "Clear selection (Esc)" }));
    expect(onClear).toHaveBeenCalledTimes(1);
  });
});

describe("SelectionBar の英語の文言", () => {
  it("1 本と複数本の件数を単数・複数で出す", () => {
    install();
    const { rerender } = renderBar({ count: 1, selectedIds: [1] });
    expect(screen.getByText("1 video selected")).toBeDefined();
    rerender(
      barElement({
        count: 2,
        selectedIds: [1, 2],
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );
    expect(screen.getByText("2 videos selected")).toBeDefined();
  });
});

describe("SelectionBar の束ねる操作", () => {
  it("1 本の選択では出さず、2 本目を選ぶと現れる", () => {
    install();
    const { rerender } = renderBar({ count: 1, selectedIds: [1] });
    expect(screen.queryByRole("button", { name: /Bundle as versions/ })).toBeNull();

    rerender(
      barElement({
        count: 2,
        selectedIds: [1, 2],
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );
    const bundle = screen.getByRole("button", { name: "Bundle as versions" });
    expect((bundle as HTMLButtonElement).disabled).toBe(false);
  });

  it("20 本を超えると押せず、理由を添える（タグの上限は使わない）", () => {
    install();
    renderBar({
      count: 21,
      selectedIds: Array.from({ length: 21 }, (_, i) => i + 1),
    });
    const bundle = screen.getByRole("button", { name: "Bundle as versions" });
    expect((bundle as HTMLButtonElement).disabled).toBe(true);
    expect(bundle.getAttribute("title")).toBe("Bundle up to 20 videos at a time");
    const describedBy = bundle.getAttribute("aria-describedby");
    expect(describedBy).not.toBeNull();
    expect(document.getElementById(describedBy!)?.textContent).toBe(
      "Bundle up to 20 videos at a time",
    );
    expect(
      (screen.getByRole("button", { name: "Edit" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("押すと選んだ順の id で窓を開き、Esc は窓だけを閉じて引き金へ戻す", async () => {
    const fetchMock = install();
    const detail = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation((input, init) => {
      const match = /^\/api\/videos\/(\d+)$/.exec(String(input));
      if (match !== null) {
        return Promise.resolve(
          jsonResponse({
            id: Number(match[1]),
            title: `動画 ${match[1]!}`,
            public: false,
            sizeBytes: 1,
            addedAt: "2026-09-01T00:00:00Z",
            playable: true,
            probeState: "done",
            thumbnailState: "done",
            previewState: "done",
            tags: [],
          }),
        );
      }
      return detail(input, init);
    });
    const user = userEvent.setup();
    const { onClear } = renderBar({ count: 2, selectedIds: [7, 3] });
    const trigger = screen.getByRole("button", { name: "Bundle as versions" });
    await user.click(trigger);

    const dialog = screen.getByRole("dialog", { name: "Bundle as versions" });
    const radios = await within(dialog).findAllByRole("radio");
    expect(radios.map((radio) => radio.closest("label")?.textContent)).toEqual([
      expect.stringContaining("動画 7"),
      expect.stringContaining("動画 3"),
    ]);

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(onClear).not.toHaveBeenCalled();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});

describe("SelectionBar の削除", () => {
  it("グループを選ぶまで削除を押せず、理由を示す", () => {
    const deleteButton = () =>
      screen.getByRole<HTMLButtonElement>("button", { name: "Delete…" });
    const { rerender } = renderBar();
    expect(deleteButton().disabled).toBe(true);
    expect(deleteButton().title).toBe("Select a group or smart folder");

    rerender(
      barElement({
        count: 3,
        selectedIds: [1, 2, 3],
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
        favoriteGroupIds: [5],
      }),
    );
    expect(deleteButton().disabled).toBe(false);
  });
});

describe("selectionReasons", () => {
  const unsupported = "Not available for smart folders";

  it("スマートフォルダを選んでいると、グループと版の操作だけ押せない理由を返す", () => {
    const reasons = selectionReasons({
      count: 2,
      favoriteVideoIds: [1, 2],
      favoriteGroupIds: [],
      smartFolderIds: [9],
    });
    expect(reasons).toEqual({
      tag: undefined,
      favorite: undefined,
      group: unsupported,
      newGroup: unsupported,
      bundle: unsupported,
      deleteCollections: undefined,
    });
  });

  it("グループもスマートフォルダも選んでいないと、削除できない理由を返す", () => {
    const reasons = (favoriteGroupIds: number[], smartFolderIds: number[]) =>
      selectionReasons({
        count: 2,
        favoriteVideoIds: [1, 2],
        favoriteGroupIds,
        smartFolderIds,
      }).deleteCollections;
    expect(reasons([], [])).toBe("Select a group or smart folder");
    expect(reasons([5], [])).toBeUndefined();
    expect(reasons([], [9])).toBeUndefined();
  });

  it("お気に入りの上限はスマートフォルダも 1 件と数える", () => {
    const reasons = selectionReasons({
      count: 0,
      favoriteVideoIds: [],
      favoriteGroupIds: Array.from({ length: 20_000 }, (_, i) => i + 1),
      smartFolderIds: [9],
    });
    expect(reasons.favorite).toBe("Select between 1 and 20,000 videos.");
    expect(reasons.group).toBe(reasons.favorite);
    expect(reasons.deleteCollections).toBe(reasons.favorite);
  });
});
