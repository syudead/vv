import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Tag } from "../api/tags";
import { __resetTagsForTest } from "../api/tags";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import SelectionBar from "./SelectionBar";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return {
    synonyms: [],
    videoCount: 0,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

/** server はタグの一覧・付け外し・要約の経路だけを扱う偽のサーバーである。 */
const server = {
  tags: [] as Tag[],
  /** videoId ごとに付いているタグの id（要約の作成に使う）。 */
  attached: new Map<number, Set<number>>(),
  /** videoId ごとに、フォルダ名から付いているタグの id（手で付けた分とは別）。 */
  fromFolder: new Map<number, Set<number>>(),
  addFails: false,
  /** 付ける要求に返す失敗の応答（addFails より先に見る）。 */
  addError: null as { status: number; body: unknown } | null,
  removeFails: false,
  summaryFails: false,
  summaryDelay: null as (() => void) | null,
  /**
   * true の間、/api/video-tags/summary への要求は resolveNow を呼ばず
   * summaryQueue に積む。順不同の応答（古い要求が新しい要求より後に届く）を
   * 再現するテスト専用（Devin の指摘2）。
   */
  summaryHold: false,
  summaryQueue: [] as (() => void)[],
  visibilityFails: false,
  /** 受け取った `PUT /api/video-visibility` の本文。 */
  visibilityRequests: [] as { videoIds: number[]; public: boolean }[],
};

function install() {
  const fetchMock = vi.fn<typeof fetch>();
  fetchMock.mockImplementation((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url === "/api/tags" && method === "GET") {
      return Promise.resolve(jsonResponse({ items: server.tags }));
    }
    if (url === "/api/video-tags" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as {
        videoIds: number[];
        action: "add" | "remove";
        tag: { id: number } | { name: string };
      };
      if (body.action === "add" && server.addError !== null) {
        return Promise.resolve(
          jsonResponse(server.addError.body, server.addError.status),
        );
      }
      if (body.action === "add" && server.addFails) {
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "internal error" }, 500),
        );
      }
      if (body.action === "remove" && server.removeFails) {
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "internal error" }, 500),
        );
      }
      let resolvedTag: { id: number; name: string };
      const requestedTag = body.tag;
      if ("id" in requestedTag) {
        const found = server.tags.find((t) => t.id === requestedTag.id);
        if (found === undefined) {
          return Promise.resolve(
            jsonResponse({ code: "tag_not_found", message: "tag not found" }, 404),
          );
        }
        resolvedTag = { id: found.id, name: found.name };
      } else {
        const name = requestedTag.name;
        const found = server.tags.find(
          (t) => t.name === name || t.synonyms.includes(name),
        );
        if (found !== undefined) {
          resolvedTag = { id: found.id, name: found.name };
        } else {
          const created = tag({ id: server.tags.length + 100, name });
          server.tags.push(created);
          resolvedTag = { id: created.id, name: created.name };
        }
      }
      for (const videoId of body.videoIds) {
        const set = server.attached.get(videoId) ?? new Set<number>();
        if (body.action === "add") set.add(resolvedTag.id);
        else set.delete(resolvedTag.id);
        server.attached.set(videoId, set);
      }
      return Promise.resolve(
        jsonResponse({ tag: resolvedTag, applied: body.videoIds.length }),
      );
    }
    if (url === "/api/video-tags/summary" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { videoIds: number[] };
      const resolveNow = () => {
        if (server.summaryFails) {
          return jsonResponse({ code: "internal", message: "internal error" }, 500);
        }
        // count はどちらかの出所で付いている本数、manualCount は手で付けた本数
        // （017 の contracts/folder-groups-api.md §4）。
        const counts = new Map<number, { count: number; manualCount: number }>();
        for (const videoId of body.videoIds) {
          const manual = server.attached.get(videoId) ?? new Set<number>();
          const folder = server.fromFolder.get(videoId) ?? new Set<number>();
          for (const tagId of new Set([...manual, ...folder])) {
            const current = counts.get(tagId) ?? { count: 0, manualCount: 0 };
            counts.set(tagId, {
              count: current.count + 1,
              manualCount: current.manualCount + (manual.has(tagId) ? 1 : 0),
            });
          }
        }
        const items = [...counts.entries()]
          .flatMap(([tagId, value]) => {
            const found = server.tags.find((t) => t.id === tagId);
            return found === undefined
              ? []
              : [{ tag: { id: found.id, name: found.name }, ...value }];
          })
          .sort((a, b) => a.tag.name.localeCompare(b.tag.name));
        return jsonResponse({ total: body.videoIds.length, items });
      };
      if (server.summaryHold) {
        return new Promise((resolve) => {
          server.summaryQueue.push(() => resolve(resolveNow()));
        });
      }
      if (server.summaryDelay !== null) {
        return new Promise((resolve) => {
          server.summaryDelay = () => resolve(resolveNow());
        });
      }
      return Promise.resolve(resolveNow());
    }
    if (url === "/api/video-visibility" && method === "PUT") {
      const body = JSON.parse(String(init?.body)) as {
        videoIds: number[];
        public: boolean;
      };
      server.visibilityRequests.push(body);
      if (server.visibilityFails) {
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "internal error" }, 500),
        );
      }
      return Promise.resolve(jsonResponse({ applied: body.videoIds.length }));
    }
    throw new Error(`想定しない要求: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function barElement(props: React.ComponentProps<typeof SelectionBar>) {
  return (
    <TooltipProvider>
      <ToastProvider>
        <SelectionBar {...props} />
      </ToastProvider>
    </TooltipProvider>
  );
}

function renderBar(props: Partial<React.ComponentProps<typeof SelectionBar>> = {}) {
  const onSelectAll = vi.fn();
  const onClear = vi.fn();
  const onTagRemoved = vi.fn();
  const onBundled = vi.fn();
  const result = render(
    barElement({
      count: 3,
      allSelected: false,
      selectedIds: [1, 2, 3],
      selectingAll: false,
      onSelectAll,
      onClear,
      onTagRemoved,
      onBundled,
      ...props,
    }),
  );
  return { ...result, onSelectAll, onClear, onTagRemoved, onBundled };
}

beforeEach(() => {
  __resetTagsForTest();
  server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Drama" })];
  server.attached = new Map();
  server.fromFolder = new Map();
  server.addFails = false;
  server.addError = null;
  server.removeFails = false;
  server.summaryFails = false;
  server.summaryDelay = null;
  server.summaryHold = false;
  server.summaryQueue = [];
  server.visibilityFails = false;
  server.visibilityRequests = [];
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

  it("件数・すべて選択・選択解除を出す。すべて選択は選択中…でdisabledになる", () => {
    install();
    renderBar({ selectingAll: true });
    expect(screen.getByText("3 videos selected")).toBeDefined();
    const selectAll = screen.getByRole("button", { name: "Selecting…" });
    expect((selectAll as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByRole("button", { name: "Clear selection (Esc)" })).toBeDefined();
  });

  it("選択が直前の「すべて選択」と同じ集合（allSelected）のときだけ、すべて選択がdisabled", () => {
    install();
    const { rerender } = renderBar({
      count: 10,
      allSelected: true,
      selectedIds: Array.from({ length: 10 }, (_, i) => i + 1),
    });
    expect(
      (screen.getByRole("button", { name: "Select all" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    // 選んだ本数が項目の数（total）を超えていても、集合が違えば押せる
    // （specs/017-folder-groups/ui-design.md「Pressing and selection」）。
    rerender(
      barElement({
        count: 9,
        allSelected: false,
        selectedIds: Array.from({ length: 9 }, (_, i) => i + 1),
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );
    expect(
      (screen.getByRole("button", { name: "Select all" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  // POST /api/video-tags は videoIds を全部か無しかでしか受け付けず、20000件を
  // 超えると400になる（contracts/tags-api.md §4）。静かに分割して送らず、
  // 一括操作を disabled にして理由を伝える（Devin の指摘2）。
  it("選択が20,000件を超えると、タグの一括操作はdisabledで理由が読める", () => {
    install();
    renderBar({
      count: 20001,
      allSelected: false,
      selectedIds: Array.from({ length: 20001 }, (_, i) => i + 1),
    });
    const addButton = screen.getByRole("button", {
      name: "Add tag",
    }) as HTMLButtonElement;
    const removeButton = screen.getByRole("button", {
      name: "Remove tag",
    }) as HTMLButtonElement;
    const visibilityButton = screen.getByRole("button", {
      name: "Visibility",
    }) as HTMLButtonElement;
    expect(addButton.disabled).toBe(true);
    expect(removeButton.disabled).toBe(true);
    // 公開の一括の切り替えも同じ上限で、同じ理由を添える（016 ui-design.md「Selection bar」）。
    expect(visibilityButton.disabled).toBe(true);
    expect(visibilityButton.title).toBe("Select between 1 and 20,000 videos.");
    expect(visibilityButton.getAttribute("aria-describedby")).toBe(
      addButton.getAttribute("aria-describedby"),
    );
    expect(addButton.title).toBe("Select between 1 and 20,000 videos.");
    expect(removeButton.title).toBe("Select between 1 and 20,000 videos.");

    const addDescribedBy = addButton.getAttribute("aria-describedby");
    expect(addDescribedBy).not.toBeNull();
    expect(document.getElementById(addDescribedBy!)?.textContent).toBe(
      "Select between 1 and 20,000 videos.",
    );

    // すべて選択はこの上限と無関係なので、ちょうど全件選び終わっていなければ
    // 引き続き押せる。
    expect(
      (screen.getByRole("button", { name: "Select all" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("選択が20,000件ちょうどなら、タグの一括操作はdisabledにならない", () => {
    install();
    renderBar({
      count: 20000,
      allSelected: false,
      selectedIds: Array.from({ length: 20000 }, (_, i) => i + 1),
    });
    expect(
      (screen.getByRole("button", { name: "Add tag" }) as HTMLButtonElement).disabled,
    ).toBe(false);
    expect(
      (screen.getByRole("button", { name: "Remove tag" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  // トリガを disabled にするだけでは、すでに開いているポップオーバーから
  // 送れてしまう。選択が上限を超えたら、開いているポップオーバー自体を
  // 閉じる（Devin の指摘）。
  it("開いているポップオーバーは、選択が上限を超えると自動で閉じる", async () => {
    const user = userEvent.setup();
    install();
    const { rerender } = renderBar({
      count: 3,
      allSelected: false,
      selectedIds: [1, 2, 3],
    });

    await user.click(screen.getByRole("button", { name: "Add tag" }));
    await screen.findByRole("combobox", { name: "Add tag" });

    rerender(
      barElement({
        count: 20001,
        allSelected: false,
        selectedIds: Array.from({ length: 20001 }, (_, i) => i + 1),
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );

    expect(screen.queryByRole("combobox", { name: "Add tag" })).toBeNull();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // 親の自動で閉じる効果が走るまでのごく短い間の保険として、ポップオーバー
  // 自身も送る直前に selectedIds の最新の件数を確かめる。ここでは親の count
  // をわざと上限内のままにし（自動では閉じない）、selectedIds だけが上限を
  // 超えた状況を作って、その保険が単独で効くことを確かめる（Devin の指摘）。
  it("タグを付ける: 開いたまま selectedIds が上限を超えると、送らず理由を示す", async () => {
    const user = userEvent.setup();
    const fetchMock = install();
    const { rerender } = renderBar({
      count: 3,
      allSelected: false,
      selectedIds: [1, 2, 3],
    });

    await user.click(screen.getByRole("button", { name: "Add tag" }));
    await screen.findByRole("combobox", { name: "Add tag" });

    rerender(
      barElement({
        count: 3,
        allSelected: false,
        selectedIds: Array.from({ length: 20001 }, (_, i) => i + 1),
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );

    expect(await screen.findByText("Select between 1 and 20,000 videos.")).toBeDefined();
    expect(screen.queryByRole("combobox", { name: "Add tag" })).toBeNull();
    expect(
      fetchMock.mock.calls.some(
        ([input, init]) => String(input) === "/api/video-tags" && init?.method === "POST",
      ),
    ).toBe(false);
  });

  it("タグを外す: 開いたまま selectedIds が上限を超えると、要約を取りに行かず理由を示す", async () => {
    const user = userEvent.setup();
    const fetchMock = install();
    server.attached.set(1, new Set([1]));
    const { rerender } = renderBar({
      count: 3,
      allSelected: false,
      selectedIds: [1, 2, 3],
    });

    await user.click(screen.getByRole("button", { name: "Remove tag" }));
    await screen.findByRole("combobox", { name: "Remove tag" });
    const summaryCallsBefore = fetchMock.mock.calls.filter(
      ([input, init]) =>
        String(input) === "/api/video-tags/summary" && init?.method === "POST",
    ).length;

    rerender(
      barElement({
        count: 3,
        allSelected: false,
        selectedIds: Array.from({ length: 20001 }, (_, i) => i + 1),
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );

    expect(await screen.findByText("Select between 1 and 20,000 videos.")).toBeDefined();
    expect(screen.queryByRole("combobox", { name: "Remove tag" })).toBeNull();
    const summaryCallsAfter = fetchMock.mock.calls.filter(
      ([input, init]) =>
        String(input) === "/api/video-tags/summary" && init?.method === "POST",
    ).length;
    expect(summaryCallsAfter).toBe(summaryCallsBefore);
  });

  it("タグを付けると、選んだ全動画に付き、トーストが出てポップオーバーが閉じ、フォーカスが戻る（受け入れ条件3）", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    const addButton = screen.getByRole("button", { name: "Add tag" });
    await user.click(addButton);
    const input = await screen.findByRole("combobox", { name: "Add tag" });
    await user.type(input, "旅行");
    await screen.findByRole("option", { name: /旅行/ });
    await user.keyboard("{Enter}");

    expect(await screen.findByText('Added "旅行" to 3 videos')).toBeDefined();
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: "Add tag" })).toBeNull(),
    );
    expect(document.activeElement).toBe(addButton);
    // 反映されたことをサーバー側で確認する。
    expect(server.attached.get(1)?.has(1)).toBe(true);
    expect(server.attached.get(2)?.has(1)).toBe(true);
    expect(server.attached.get(3)?.has(1)).toBe(true);
  });

  it("新しい名前で作成すると新しいタグが選んだ全動画に付く", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    await user.click(screen.getByRole("button", { name: "Add tag" }));
    const input = await screen.findByRole("combobox", { name: "Add tag" });
    await user.type(input, "新規タグ");
    await screen.findByRole("option", { name: /^Create/ });
    await user.keyboard("{Enter}");

    expect(await screen.findByText('Added "新規タグ" to 3 videos')).toBeDefined();
  });

  it("付けるのに失敗すると、入力の下に理由が出て、選択とポップオーバーを保ったまま再試行できる", async () => {
    const user = userEvent.setup();
    install();
    server.addFails = true;
    renderBar();

    await user.click(screen.getByRole("button", { name: "Add tag" }));
    const input = await screen.findByRole("combobox", { name: "Add tag" });
    await user.type(input, "旅行");
    await screen.findByRole("option", { name: /旅行/ });
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText(
        "Couldn't add the tag: Something went wrong on the server.",
      ),
    ).toBeDefined();
    expect(screen.getByRole("combobox", { name: "Add tag" })).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("旅行");

    server.addFails = false;
    await user.keyboard("{Enter}");
    expect(await screen.findByText('Added "旅行" to 3 videos')).toBeDefined();
  });

  it("候補が別のタブで削除された（tag_not_found）ときは、トーストを出しポップオーバーは開いたまま", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    await user.click(screen.getByRole("button", { name: "Add tag" }));
    const input = await screen.findByRole("combobox", { name: "Add tag" });
    await user.type(input, "旅行");
    const option = await screen.findByRole("option", { name: /旅行/ });
    // 選ぶ直前にタグが消える。
    server.tags = server.tags.filter((t) => t.id !== 1);
    fireEvent.click(option);

    expect(
      await screen.findByText(
        'The tag "旅行" no longer exists, so the tags were reloaded',
      ),
    ).toBeDefined();
    expect(screen.getByRole("combobox", { name: "Add tag" })).toBeDefined();
  });

  it("要約を取るまでは読み込み中…を role=status で出す", async () => {
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1]));
    server.attached.set(2, new Set([1]));
    server.attached.set(3, new Set([1]));
    server.summaryDelay = () => undefined;
    renderBar();

    await user.click(screen.getByRole("button", { name: "Remove tag" }));
    // role=status は選択件数の行にもあるので、「読み込み中…」の方だけを見る。
    const status = await screen.findByText("Loading…");
    expect(status.getAttribute("role")).toBe("status");

    await act(async () => {
      server.summaryDelay?.();
      await Promise.resolve();
    });
    const input = await screen.findByRole("combobox", { name: "Remove tag" });
    await user.click(input);
    expect(await screen.findByRole("option", { name: /旅行/ })).toBeDefined();
  });

  it("タグを外すと、選んだ動画から外れトーストが出る。一部にしか付いていないタグは文言とアイコンで示す（受け入れ条件3）", async () => {
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1, 2]));
    server.attached.set(2, new Set([1]));
    server.attached.set(3, new Set([1]));
    renderBar();

    await user.click(screen.getByRole("button", { name: "Remove tag" }));

    const input = await screen.findByRole("combobox", { name: "Remove tag" });
    await user.click(input);
    const travelOption = await screen.findByRole("option", { name: /旅行/ });
    expect(within(travelOption).getByText("3 videos")).toBeDefined();

    const dramaOption = screen.getByRole("option", {
      name: "Drama, only some videos, 1 of 3",
    });
    expect(within(dramaOption).getByText("Some: 1 / 3")).toBeDefined();

    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(await screen.findByText('Removed "旅行" from 3 videos')).toBeDefined();
    expect(server.attached.get(1)?.has(1)).toBe(false);
    expect(server.attached.get(2)?.has(1)).toBe(false);
    expect(server.attached.get(3)?.has(1)).toBe(false);
    // 外した後もポップオーバーは開いたまま、要約を取り直して残りの候補を見せる。
    await waitFor(() =>
      expect(
        screen.queryByRole("option", { name: "Drama, only some videos, 1 of 3" }),
      ).toBeDefined(),
    );
  });

  it("外すのに失敗すると、理由が出て選択とポップオーバーを保ったまま再試行できる", async () => {
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1]));
    server.attached.set(2, new Set([1]));
    server.attached.set(3, new Set([1]));
    server.removeFails = true;
    renderBar();

    await user.click(screen.getByRole("button", { name: "Remove tag" }));
    const input = await screen.findByRole("combobox", { name: "Remove tag" });
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText(
        "Couldn't remove the tag: Something went wrong on the server.",
      ),
    ).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("旅行");

    server.removeFails = false;
    await user.keyboard("{Enter}");
    expect(await screen.findByText('Removed "旅行" from 3 videos')).toBeDefined();
  });

  it("選んだ動画にタグが無いときは、外せるタグが無いことを出す", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    await user.click(screen.getByRole("button", { name: "Remove tag" }));
    expect(
      await screen.findByText("The selected videos have no tags that can be removed"),
    ).toBeDefined();
  });

  // 017 の ui-design.md「Folder-derived tag chip」: フォルダ名から付いているだけの
  // タグは外せないので候補に出さず、「一部」の判定と本数は手で付けた本数で行う。
  it("タグを外す: 候補は手で付けたタグだけで、一部の本数は手で付けた本数で数える", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "旅行" }),
      tag({ id: 2, name: "Drama" }),
      tag({ id: 3, name: "京都" }),
    ];
    // 旅行: 3本ともフォルダ名から付き、手では1本だけ。
    // 京都: フォルダ名からだけ（3本とも）。
    // Drama: 手で3本。
    for (const id of [1, 2, 3]) server.fromFolder.set(id, new Set([1, 3]));
    server.attached.set(1, new Set([1, 2]));
    server.attached.set(2, new Set([2]));
    server.attached.set(3, new Set([2]));
    renderBar();

    await user.click(screen.getByRole("button", { name: "Remove tag" }));
    const input = await screen.findByRole("combobox", { name: "Remove tag" });
    await user.click(input);
    const options = await screen.findAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual([
      "Drama3 videos",
      "旅行Some: 1 / 3",
    ]);
    expect(
      screen.getByRole("option", { name: "旅行, only some videos, 1 of 3" }),
    ).toBeDefined();
    expect(screen.queryByRole("option", { name: /京都/ })).toBeNull();
  });

  it("タグを外す: フォルダ名から付いたタグしか無いときは、外せるタグが無いことを出す", async () => {
    const user = userEvent.setup();
    install();
    for (const id of [1, 2, 3]) server.fromFolder.set(id, new Set([1]));
    renderBar();

    await user.click(screen.getByRole("button", { name: "Remove tag" }));
    expect(
      await screen.findByText("The selected videos have no tags that can be removed"),
    ).toBeDefined();
    expect(screen.queryByRole("combobox", { name: "Remove tag" })).toBeNull();
  });

  it("要約を取れないときは理由と再試行を出し、再試行で取り直す", async () => {
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1]));
    server.attached.set(2, new Set([1]));
    server.attached.set(3, new Set([1]));
    server.summaryFails = true;
    renderBar();

    await user.click(screen.getByRole("button", { name: "Remove tag" }));
    await screen.findByText("Couldn't load the tags");
    expect(screen.getByRole("alert").textContent).toBe("Couldn't load the tags");

    server.summaryFails = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    const input = await screen.findByRole("combobox", { name: "Remove tag" });
    await user.click(input);
    expect(await screen.findByRole("option", { name: /旅行/ })).toBeDefined();
  });

  it("すべて選択を押すと onSelectAll を呼ぶ", async () => {
    const user = userEvent.setup();
    install();
    const { onSelectAll } = renderBar();
    await user.click(screen.getByRole("button", { name: "Select all" }));
    expect(onSelectAll).toHaveBeenCalledTimes(1);
  });

  it("選択を解除ボタンで onClear を呼ぶ", async () => {
    const user = userEvent.setup();
    install();
    const { onClear } = renderBar();
    await user.click(screen.getByRole("button", { name: "Clear selection (Esc)" }));
    expect(onClear).toHaveBeenCalledTimes(1);
  });

  // B2: Radix の DismissableLayer は document の capture 段階で Esc を先に
  // 拾うため、何もしなければ combobox 自身の Esc 処理より先にポップオーバー
  // 全体が閉じてしまう。1回目の Esc は候補の一覧だけを閉じ、選択とポップオーバー
  // は残る。一覧がすでに閉じている2回目の Esc でポップオーバーが閉じ、それでも
  // 選択は残る（ui-design.md「Combobox」）。
  it("タグを付ける: 1回目のEscは候補の一覧だけを閉じ、2回目でポップオーバーが閉じても選択は残る", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    const addButton = screen.getByRole("button", { name: "Add tag" });
    await user.click(addButton);
    const input = await screen.findByRole("combobox", { name: "Add tag" });
    // フォーカスで一覧が開く（全タグが候補になる）。
    await screen.findByRole("option", { name: /旅行/ });

    await user.keyboard("{Escape}");
    // 1回目: 一覧だけが閉じ、ポップオーバー（入力）はまだ残る。
    expect(screen.queryByRole("option")).toBeNull();
    expect(screen.getByRole("combobox", { name: "Add tag" })).toBeDefined();
    expect(input).toHaveProperty("value", "");

    await user.keyboard("{Escape}");
    // 2回目: ポップオーバーが閉じる。選択（3 件を選択中）は残る。
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: "Add tag" })).toBeNull(),
    );
    expect(screen.getByText("3 videos selected")).toBeDefined();
  });

  it("タグを外す: 1回目のEscは候補の一覧だけを閉じ、2回目でポップオーバーが閉じても選択は残る", async () => {
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1]));
    server.attached.set(2, new Set([1]));
    server.attached.set(3, new Set([1]));
    renderBar();

    await user.click(screen.getByRole("button", { name: "Remove tag" }));
    await screen.findByRole("combobox", { name: "Remove tag" });
    await screen.findByRole("option", { name: /旅行/ });

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("option")).toBeNull();
    expect(screen.getByRole("combobox", { name: "Remove tag" })).toBeDefined();

    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: "Remove tag" })).toBeNull(),
    );
    expect(screen.getByText("3 videos selected")).toBeDefined();
  });

  // Devin の指摘2: 「タグを外す」が開いたまま選択が変わっても、以前は要約を
  // 取り直さなかった。submit は最新の selectedIds を使うので、古い候補（前の
  // 選択の要約）を選ぶと、意図しない動画からタグを外してしまう。
  it("「タグを外す」を開いたまま選択が変わると、要約を取り直し、届くまで候補を隠す（Devin の指摘2）", async () => {
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1]));
    server.attached.set(2, new Set([1]));
    server.attached.set(3, new Set());
    const { rerender } = renderBar({ selectedIds: [1, 2, 3], count: 3 });

    await user.click(screen.getByRole("button", { name: "Remove tag" }));
    await screen.findByRole("option", { name: "旅行, only some videos, 2 of 3" });

    // ポップオーバーを開いたまま、選択が動画1だけに変わる（動画2・3の選択を
    // 外した想定。LibraryPage が selectedIds を新しい配列で渡し直す）。
    rerender(
      barElement({
        count: 1,
        allSelected: false,
        selectedIds: [1],
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );

    // 取り直している間は候補ごと消え、古い候補（一部2/3件）を選べない。
    await waitFor(() => expect(screen.queryByRole("option")).toBeNull());

    // 新しい選択（動画1だけ）では「旅行」は全部に付いていて、もう一部ではない。
    const option = await screen.findByRole("option", { name: /旅行/ });
    expect(within(option).getByText("1 video")).toBeDefined();
    expect(screen.queryByText(/Some:/)).toBeNull();
  });

  it("要約の取り直しでは、古い応答が新しい応答を上書きしない（Devin の指摘2）", async () => {
    install();
    server.attached.set(1, new Set([1]));
    server.attached.set(2, new Set([1]));
    server.summaryHold = true;
    const { rerender } = renderBar({ selectedIds: [1, 2], count: 2 });

    fireEvent.click(screen.getByRole("button", { name: "Remove tag" }));
    await waitFor(() => expect(server.summaryQueue).toHaveLength(1));

    // 開いている間に選択が変わり、2回目の要求も止められる。
    rerender(
      barElement({
        count: 1,
        allSelected: false,
        selectedIds: [1],
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );
    await waitFor(() => expect(server.summaryQueue).toHaveLength(2));

    // 新しい方（2番目）を先に、古い方（1番目）を後から届かせる。
    const [first, second] = server.summaryQueue;
    await act(async () => {
      second?.();
      await Promise.resolve();
    });
    const optionAfterSecond = await screen.findByRole("option", { name: /旅行/ });
    expect(within(optionAfterSecond).getByText("1 video")).toBeDefined();

    await act(async () => {
      first?.();
      await Promise.resolve();
    });
    // 古い応答（選択が[1,2]だった頃、2件中2件）が後から届いても上書きしない。
    const optionAfterStale = screen.getByRole("option", { name: /旅行/ });
    expect(within(optionAfterStale).getByText("1 video")).toBeDefined();
  });

  // Devin の指摘3: addOpen・removeOpen は SelectionBar 自身の状態で、選択が
  // 0 件になって count===0 return null になっても SelectionBar はアンマウント
  // しない（呼び出し元は常に描画している）ので、以前はそのまま残った。
  it("選択が0件になって消えたあと選び直すと、ポップオーバーは開いた状態で戻らない（Devin の指摘3）", async () => {
    const user = userEvent.setup();
    install();
    const { rerender } = renderBar({ selectedIds: [1, 2, 3], count: 3 });

    await user.click(screen.getByRole("button", { name: "Add tag" }));
    await screen.findByRole("combobox", { name: "Add tag" });

    // 選択が0件になる（バーは何も描かなくなる）。
    rerender(
      barElement({
        count: 0,
        allSelected: false,
        selectedIds: [],
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();

    // 選び直す（バーがまた出る）。
    rerender(
      barElement({
        count: 2,
        allSelected: false,
        selectedIds: [4, 5],
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );

    // ポップオーバーは勝手に開いた状態で戻らない。押せば開く（表示自体は壊れて
    // いない）ことも確かめる。
    expect(screen.queryByRole("combobox", { name: "Add tag" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Add tag" }));
    expect(await screen.findByRole("combobox", { name: "Add tag" })).toBeDefined();
  });
});

// 公開の一括の切り替え（specs/016-single-account-auth/ui-design.md「Selection bar」、issue 305）。
describe("SelectionBar の公開", () => {
  it("「公開にする」で選んだ動画を1回で送り、applied の件数をトーストで伝えて選択を残す", async () => {
    const user = userEvent.setup();
    const fetchMock = install();
    const { onClear } = renderBar();

    await user.click(screen.getByRole("button", { name: "Visibility" }));
    const items = await screen.findAllByRole("menuitem");
    // 今の状態は示さず、2つとも常に押せる。
    expect(items.map((item) => item.textContent)).toEqual([
      "Make public",
      "Make private",
    ]);
    expect(items.every((item) => item.getAttribute("aria-disabled") !== "true")).toBe(
      true,
    );
    await user.click(screen.getByRole("menuitem", { name: "Make public" }));

    expect(await screen.findByText("Made 3 videos public")).toBeDefined();
    const puts = fetchMock.mock.calls.filter(
      ([url, init]) => url === "/api/video-visibility" && init?.method === "PUT",
    );
    expect(puts).toHaveLength(1);
    expect(server.visibilityRequests).toEqual([{ videoIds: [1, 2, 3], public: true }]);
    expect(onClear).not.toHaveBeenCalled();
    expect(screen.getByText("3 videos selected")).toBeDefined();
  });

  it("「非公開にする」は public: false を送り、非公開にしたと伝える", async () => {
    const user = userEvent.setup();
    install();
    renderBar({ count: 2, selectedIds: [4, 5] });

    await user.click(screen.getByRole("button", { name: "Visibility" }));
    await user.click(await screen.findByRole("menuitem", { name: "Make private" }));

    expect(await screen.findByText("Made 2 videos private")).toBeDefined();
    expect(server.visibilityRequests).toEqual([{ videoIds: [4, 5], public: false }]);
  });

  it("失敗したらトースト「変更できませんでした」を出し、選択を残す", async () => {
    const user = userEvent.setup();
    install();
    server.visibilityFails = true;
    const { onClear } = renderBar();

    await user.click(screen.getByRole("button", { name: "Visibility" }));
    await user.click(await screen.findByRole("menuitem", { name: "Make public" }));

    expect(
      await screen.findByText(
        "Couldn't change the visibility: Something went wrong on the server.",
      ),
    ).toBeDefined();
    expect(onClear).not.toHaveBeenCalled();
    expect(screen.getByText("3 videos selected")).toBeDefined();
  });

  // ui/Menu（Radix）はキーボードで開くと先頭の項目にフォーカスを置く。↓ で
  // 「非公開にする」へ、↑ で「公開にする」へ戻る。
  it("キーボードだけで「公開」を開き、「公開にする」を選べる", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    screen.getByRole("button", { name: "Visibility" }).focus();
    await user.keyboard("{Enter}");
    await screen.findByRole("menu");
    await waitFor(() => expect(document.activeElement?.textContent).toBe("Make public"));
    await user.keyboard("{ArrowDown}");
    await waitFor(() => expect(document.activeElement?.textContent).toBe("Make private"));
    await user.keyboard("{ArrowUp}");
    await waitFor(() => expect(document.activeElement?.textContent).toBe("Make public"));
    await user.keyboard("{Enter}");

    expect(await screen.findByText("Made 3 videos public")).toBeDefined();
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
        allSelected: false,
        selectedIds: [1, 2],
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );
    expect(screen.getByText("2 videos selected")).toBeDefined();
  });

  it("一括操作の上限の失敗は、API の limit を埋め込んだ文で出す", async () => {
    const user = userEvent.setup();
    install();
    server.addError = {
      status: 400,
      body: {
        code: "invalid_request",
        reason: "too_many_videos",
        limit: 500,
        message: "videoIds must contain between 1 and 500 items",
      },
    };
    renderBar();

    await user.click(screen.getByRole("button", { name: "Add tag" }));
    const input = await screen.findByRole("combobox", { name: "Add tag" });
    await user.type(input, "旅行");
    await screen.findByRole("option", { name: /旅行/ });
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("Couldn't add the tag: Select between 1 and 500 videos."),
    ).toBeDefined();
  });

  it("疑似ロケールで、通常・付ける・外す・上限超過の文言がすべてカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1, 2]));
    server.attached.set(2, new Set([2]));
    server.attached.set(3, new Set([2]));
    const tagNames = ["旅行", "Drama"];
    const { rerender } = renderBar();
    expectCatalogTextOnly(document.body, tagNames);

    await user.click(screen.getByRole("button", { name: /Add tag/ }));
    const addInput = await screen.findByRole("combobox", { name: /Add tag/ });
    await user.type(addInput, "新規");
    await screen.findByRole("option", { name: /新規/ });
    expectCatalogTextOnly(document.body, [...tagNames, "新規"]);
    await user.keyboard("{Escape}{Escape}");

    await user.click(screen.getByRole("button", { name: /Remove tag/ }));
    const removeInput = await screen.findByRole("combobox", { name: /Remove tag/ });
    await user.click(removeInput);
    await screen.findAllByRole("option");
    expectCatalogTextOnly(document.body, tagNames);
    await user.keyboard("{Escape}{Escape}");

    rerender(
      barElement({
        count: 20_001,
        allSelected: false,
        selectedIds: [1, 2, 3],
        selectingAll: false,
        onSelectAll: vi.fn(),
        onClear: vi.fn(),
        onTagRemoved: vi.fn(),
        onBundled: vi.fn(),
      }),
    );
    expectCatalogTextOnly(document.body, tagNames);
  });

  it("疑似ロケールで、要約の読み込み中・失敗・外せるタグが無い状態もカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    install();
    server.summaryFails = true;
    renderBar();

    await user.click(screen.getByRole("button", { name: /Remove tag/ }));
    await screen.findByRole("button", { name: /Retry/ });
    expectCatalogTextOnly(document.body);

    server.summaryFails = false;
    await user.click(screen.getByRole("button", { name: /Retry/ }));
    await screen.findByText(/no tags that can be removed/);
    expectCatalogTextOnly(document.body);
  });
});

describe("SelectionBar の束ねる操作（specs/030-video-versions/ui-design.md「Bundle action」）", () => {
  it("1 本の選択では出さず、2 本目を選ぶと現れる", () => {
    install();
    const { rerender } = renderBar({ count: 1, selectedIds: [1] });
    expect(screen.queryByRole("button", { name: /Bundle as versions/ })).toBeNull();

    rerender(
      barElement({
        count: 2,
        allSelected: false,
        selectedIds: [1, 2],
        selectingAll: false,
        onSelectAll: vi.fn(),
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
    // タグと公開の操作は上限（20,000 本）の内なので押せる。
    expect(
      (screen.getByRole("button", { name: /Add tag/ }) as HTMLButtonElement).disabled,
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
