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
  return { synonyms: [], videoCount: 0, ...overrides };
}

/** server はタグの一覧・付け外し・要約の経路だけを扱う偽のサーバーである。 */
const server = {
  tags: [] as Tag[],
  /** videoId ごとに付いているタグの id（要約の作成に使う）。 */
  attached: new Map<number, Set<number>>(),
  addFails: false,
  removeFails: false,
  summaryFails: false,
  summaryDelay: null as (() => void) | null,
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
      if (body.action === "add" && server.addFails) {
        return Promise.resolve(jsonResponse({ code: "internal", message: "失敗" }, 500));
      }
      if (body.action === "remove" && server.removeFails) {
        return Promise.resolve(jsonResponse({ code: "internal", message: "失敗" }, 500));
      }
      let resolvedTag: { id: number; name: string };
      const requestedTag = body.tag;
      if ("id" in requestedTag) {
        const found = server.tags.find((t) => t.id === requestedTag.id);
        if (found === undefined) {
          return Promise.resolve(
            jsonResponse({ code: "tag_not_found", message: "もう無い" }, 404),
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
          return jsonResponse({ code: "internal", message: "失敗" }, 500);
        }
        const counts = new Map<number, number>();
        for (const videoId of body.videoIds) {
          for (const tagId of server.attached.get(videoId) ?? []) {
            counts.set(tagId, (counts.get(tagId) ?? 0) + 1);
          }
        }
        const items = [...counts.entries()]
          .map(([tagId, count]) => {
            const found = server.tags.find((t) => t.id === tagId);
            return found === undefined
              ? null
              : { tag: { id: found.id, name: found.name }, count };
          })
          .filter(
            (value): value is { tag: { id: number; name: string }; count: number } =>
              value !== null,
          )
          .sort((a, b) => a.tag.name.localeCompare(b.tag.name));
        return jsonResponse({ total: body.videoIds.length, items });
      };
      if (server.summaryDelay !== null) {
        return new Promise((resolve) => {
          server.summaryDelay = () => resolve(resolveNow());
        });
      }
      return Promise.resolve(resolveNow());
    }
    throw new Error(`想定しない要求: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderBar(props: Partial<React.ComponentProps<typeof SelectionBar>> = {}) {
  const onSelectAll = vi.fn();
  const onClear = vi.fn();
  const result = render(
    <TooltipProvider>
      <ToastProvider>
        <SelectionBar
          count={3}
          total={10}
          selectedIds={[1, 2, 3]}
          selectingAll={false}
          onSelectAll={onSelectAll}
          onClear={onClear}
          {...props}
        />
      </ToastProvider>
    </TooltipProvider>,
  );
  return { ...result, onSelectAll, onClear };
}

beforeEach(() => {
  __resetTagsForTest();
  server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Drama" })];
  server.attached = new Map();
  server.addFails = false;
  server.removeFails = false;
  server.summaryFails = false;
  server.summaryDelay = null;
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("SelectionBar", () => {
  it("count が 0 のときは何も描かない", () => {
    install();
    renderBar({ count: 0 });
    expect(screen.queryByRole("region", { name: "選択中の操作" })).toBeNull();
  });

  it("件数・すべて選択・選択解除を出す。すべて選択は選択中…でdisabledになる", () => {
    install();
    renderBar({ selectingAll: true });
    expect(screen.getByText("3 件を選択中")).toBeDefined();
    const selectAll = screen.getByRole("button", { name: "選択中…" });
    expect((selectAll as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByRole("button", { name: "選択を解除 (Esc)" })).toBeDefined();
  });

  it("全件選び終わっている（count>=total）ときは、すべて選択がdisabled", () => {
    install();
    renderBar({
      count: 10,
      total: 10,
      selectedIds: Array.from({ length: 10 }, (_, i) => i + 1),
    });
    expect(
      (screen.getByRole("button", { name: "すべて選択" }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it("タグを付けると、選んだ全動画に付き、トーストが出てポップオーバーが閉じ、フォーカスが戻る（受け入れ条件3）", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    const addButton = screen.getByRole("button", { name: "タグを付ける" });
    await user.click(addButton);
    const input = await screen.findByRole("combobox", { name: "タグを付ける" });
    await user.type(input, "旅行");
    await screen.findByRole("option", { name: /旅行/ });
    await user.keyboard("{Enter}");

    expect(await screen.findByText("3 件に「旅行」を付けました")).toBeDefined();
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: "タグを付ける" })).toBeNull(),
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

    await user.click(screen.getByRole("button", { name: "タグを付ける" }));
    const input = await screen.findByRole("combobox", { name: "タグを付ける" });
    await user.type(input, "新規タグ");
    await screen.findByRole("option", { name: /を作成/ });
    await user.keyboard("{Enter}");

    expect(await screen.findByText("3 件に「新規タグ」を付けました")).toBeDefined();
  });

  it("付けるのに失敗すると、入力の下に理由が出て、選択とポップオーバーを保ったまま再試行できる", async () => {
    const user = userEvent.setup();
    install();
    server.addFails = true;
    renderBar();

    await user.click(screen.getByRole("button", { name: "タグを付ける" }));
    const input = await screen.findByRole("combobox", { name: "タグを付ける" });
    await user.type(input, "旅行");
    await screen.findByRole("option", { name: /旅行/ });
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("付けられませんでした。もう一度お試しください"),
    ).toBeDefined();
    expect(screen.getByRole("combobox", { name: "タグを付ける" })).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("旅行");

    server.addFails = false;
    await user.keyboard("{Enter}");
    expect(await screen.findByText("3 件に「旅行」を付けました")).toBeDefined();
  });

  it("候補が別のタブで削除された（tag_not_found）ときは、トーストを出しポップオーバーは開いたまま", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    await user.click(screen.getByRole("button", { name: "タグを付ける" }));
    const input = await screen.findByRole("combobox", { name: "タグを付ける" });
    await user.type(input, "旅行");
    const option = await screen.findByRole("option", { name: /旅行/ });
    // 選ぶ直前にタグが消える。
    server.tags = server.tags.filter((t) => t.id !== 1);
    fireEvent.click(option);

    expect(
      await screen.findByText("タグ「旅行」はもう無いため、一覧を取り直しました"),
    ).toBeDefined();
    expect(screen.getByRole("combobox", { name: "タグを付ける" })).toBeDefined();
  });

  it("要約を取るまでは読み込み中…を role=status で出す", async () => {
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1]));
    server.attached.set(2, new Set([1]));
    server.attached.set(3, new Set([1]));
    server.summaryDelay = () => undefined;
    renderBar();

    await user.click(screen.getByRole("button", { name: "タグを外す" }));
    // role=status は選択件数の行にもあるので、「読み込み中…」の方だけを見る。
    const status = await screen.findByText("読み込み中…");
    expect(status.getAttribute("role")).toBe("status");

    await act(async () => {
      server.summaryDelay?.();
      await Promise.resolve();
    });
    const input = await screen.findByRole("combobox", { name: "タグを外す" });
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

    await user.click(screen.getByRole("button", { name: "タグを外す" }));

    const input = await screen.findByRole("combobox", { name: "タグを外す" });
    await user.click(input);
    const travelOption = await screen.findByRole("option", { name: /旅行/ });
    expect(within(travelOption).getByText("3 件")).toBeDefined();

    const dramaOption = screen.getByRole("option", {
      name: "Drama、一部の動画だけ、3 件中 1 件",
    });
    expect(within(dramaOption).getByText("一部 1 / 3 件")).toBeDefined();

    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("3 件から「旅行」を外しました")).toBeDefined();
    expect(server.attached.get(1)?.has(1)).toBe(false);
    expect(server.attached.get(2)?.has(1)).toBe(false);
    expect(server.attached.get(3)?.has(1)).toBe(false);
    // 外した後もポップオーバーは開いたまま、要約を取り直して残りの候補を見せる。
    await waitFor(() =>
      expect(
        screen.queryByRole("option", { name: "Drama、一部の動画だけ、3 件中 1 件" }),
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

    await user.click(screen.getByRole("button", { name: "タグを外す" }));
    const input = await screen.findByRole("combobox", { name: "タグを外す" });
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("外せませんでした。もう一度お試しください"),
    ).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("旅行");

    server.removeFails = false;
    await user.keyboard("{Enter}");
    expect(await screen.findByText("3 件から「旅行」を外しました")).toBeDefined();
  });

  it("選んだ動画にタグが無いときは、その旨を出す", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    await user.click(screen.getByRole("button", { name: "タグを外す" }));
    expect(await screen.findByText("選んだ動画にタグはありません")).toBeDefined();
  });

  it("要約を取れないときは理由と再試行を出し、再試行で取り直す", async () => {
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1]));
    server.attached.set(2, new Set([1]));
    server.attached.set(3, new Set([1]));
    server.summaryFails = true;
    renderBar();

    await user.click(screen.getByRole("button", { name: "タグを外す" }));
    await screen.findByText("タグを取得できませんでした");
    expect(screen.getByRole("alert").textContent).toBe("タグを取得できませんでした");

    server.summaryFails = false;
    await user.click(screen.getByRole("button", { name: "再試行" }));
    const input = await screen.findByRole("combobox", { name: "タグを外す" });
    await user.click(input);
    expect(await screen.findByRole("option", { name: /旅行/ })).toBeDefined();
  });

  it("すべて選択を押すと onSelectAll を呼ぶ", async () => {
    const user = userEvent.setup();
    install();
    const { onSelectAll } = renderBar();
    await user.click(screen.getByRole("button", { name: "すべて選択" }));
    expect(onSelectAll).toHaveBeenCalledTimes(1);
  });

  it("選択を解除ボタンで onClear を呼ぶ", async () => {
    const user = userEvent.setup();
    install();
    const { onClear } = renderBar();
    await user.click(screen.getByRole("button", { name: "選択を解除 (Esc)" }));
    expect(onClear).toHaveBeenCalledTimes(1);
  });

  // B2: Radix の DismissableLayer は document の capture 段階で Esc を先に
  // 拾うため、何もしなければ combobox 自身の Esc 処理より先にポップオーバー
  // 全体が閉じてしまう。1回目の Esc は候補の一覧だけを閉じ、選択とポップオーバー
  // は残る。一覧がすでに閉じている2回目の Esc でポップオーバーが閉じ、それでも
  // 選択は残る（ui-design.md「Combobox」、Visual review criteria 手順2）。
  it("タグを付ける: 1回目のEscは候補の一覧だけを閉じ、2回目でポップオーバーが閉じても選択は残る", async () => {
    const user = userEvent.setup();
    install();
    renderBar();

    const addButton = screen.getByRole("button", { name: "タグを付ける" });
    await user.click(addButton);
    const input = await screen.findByRole("combobox", { name: "タグを付ける" });
    // フォーカスで一覧が開く（全タグが候補になる）。
    await screen.findByRole("option", { name: /旅行/ });

    await user.keyboard("{Escape}");
    // 1回目: 一覧だけが閉じ、ポップオーバー（入力）はまだ残る。
    expect(screen.queryByRole("option")).toBeNull();
    expect(screen.getByRole("combobox", { name: "タグを付ける" })).toBeDefined();
    expect(input).toHaveProperty("value", "");

    await user.keyboard("{Escape}");
    // 2回目: ポップオーバーが閉じる。選択（3 件を選択中）は残る。
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: "タグを付ける" })).toBeNull(),
    );
    expect(screen.getByText("3 件を選択中")).toBeDefined();
  });

  it("タグを外す: 1回目のEscは候補の一覧だけを閉じ、2回目でポップオーバーが閉じても選択は残る", async () => {
    const user = userEvent.setup();
    install();
    server.attached.set(1, new Set([1]));
    server.attached.set(2, new Set([1]));
    server.attached.set(3, new Set([1]));
    renderBar();

    await user.click(screen.getByRole("button", { name: "タグを外す" }));
    await screen.findByRole("combobox", { name: "タグを外す" });
    await screen.findByRole("option", { name: /旅行/ });

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("option")).toBeNull();
    expect(screen.getByRole("combobox", { name: "タグを外す" })).toBeDefined();

    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: "タグを外す" })).toBeNull(),
    );
    expect(screen.getByText("3 件を選択中")).toBeDefined();
  });
});
