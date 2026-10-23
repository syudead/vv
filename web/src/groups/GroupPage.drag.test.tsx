import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import {
  json,
  layoutRows,
  renderPage,
  seed,
  server,
  setUpGroupPageServer,
  video,
} from "../testing/groupPage";

setUpGroupPageServer();

// Supply 200px-wide card positions because jsdom has no layout.

const cardWidth = 190;
const cardHeight = 150;
const step = 200;

function layoutCards() {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
    this: Element,
  ) {
    const card = this.closest("[data-drag-card]");
    const index =
      card === null ? 0 : Array.from(card.parentElement?.children ?? []).indexOf(card);
    const left = index * step;
    return {
      x: left,
      y: 0,
      top: 0,
      left,
      right: left + cardWidth,
      bottom: cardHeight,
      width: card === null ? 0 : cardWidth,
      height: card === null ? 0 : cardHeight,
      toJSON: () => ({}),
    } as DOMRect;
  });
}

function pointOn(index: number, ratio: number) {
  return { clientX: index * step + cardWidth * ratio, clientY: cardHeight / 2 };
}

const mouse = { pointerType: "mouse", isPrimary: true, button: 0, pointerId: 1 };

function lift(title: string, index: number) {
  const link = screen.getByRole("link", { name: title });
  fireEvent.pointerDown(link, { ...mouse, ...pointOn(index, 0.5) });
  fireEvent.pointerMove(document, { ...mouse, ...pointOn(index, 0.4) });
}

function moveTo(index: number, ratio: number) {
  fireEvent.pointerMove(document, { ...mouse, ...pointOn(index, ratio) });
}

function release(index: number, ratio: number) {
  fireEvent.pointerUp(document, { ...mouse, ...pointOn(index, ratio) });
}

/** Drop, then let dnd-kit's post-drop click guard expire so the next click reaches the page. */
async function releaseAndSettle(index: number, ratio: number) {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  try {
    release(index, ratio);
    // dnd-kit suppresses document clicks for 50ms after a drop.
    await act(() => vi.advanceTimersByTimeAsync(50));
  } finally {
    vi.useRealTimers();
  }
}

function cardTitles(): string[] {
  return Array.from(document.querySelectorAll("article")).map(
    (card) => card.querySelector("h3")?.textContent ?? "",
  );
}

function reorders() {
  return server.writes
    .filter((each) => each.path === "/api/organize/reorder")
    .map((each) => each.body);
}

async function openGrid() {
  seed();
  layoutCards();
  renderPage("/groups/1");
  await screen.findByRole("heading", { level: 1, name: "Season" });
}

describe("GroupPage drag", () => {
  it("3 番目のカードを先頭に落とすと after なしの reorder を送り、すぐその順で出す", async () => {
    await openGrid();
    lift("Episode 2", 2);
    moveTo(0, 0.1);
    expect(document.querySelector('[data-drop-bar="before"]')).not.toBeNull();
    release(0, 0.1);

    expect(cardTitles()).toEqual(["Episode 2", "Episode 1", "Extras"]);
    await waitFor(() => expect(reorders()).toEqual([{ groupId: 1, itemIds: [103] }]));
    expect(await screen.findByText('Reordered "Season"')).toBeDefined();
  });

  it("選択した 2 枚のカードは、その順のままいっしょに動く", async () => {
    await openGrid();
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: /^Select "?Episode 1"?$/ }));
    await user.click(screen.getByRole("checkbox", { name: /^Select "?Episode 2"?$/ }));

    lift("Episode 2", 2);
    moveTo(1, 0.1);
    expect(screen.getByLabelText("2 items").textContent).toBe("2");
    release(1, 0.1);

    expect(cardTitles()).toEqual(["Episode 1", "Episode 2", "Extras"]);
    await waitFor(() =>
      expect(reorders()).toEqual([{ groupId: 1, itemIds: [101, 103] }]),
    );
  });

  it("選んだ子グループのメンバーの直接の動画は、チェックが見えていてもそれだけが動く", async () => {
    seed();
    // Bonus is both a direct video and a member of Extras.
    server.groups.get(1)?.items.push({
      id: 104,
      kind: "video",
      video: video(12, { title: "Bonus" }),
    });
    layoutCards();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    const user = userEvent.setup();
    await user.click(
      screen.getByRole("checkbox", { name: /^Select the group "?Extras"?$/ }),
    );
    expect(
      (screen.getByRole("checkbox", { name: /^Select "?Bonus"?$/ }) as HTMLButtonElement)
        .dataset.state,
    ).toBe("checked");

    lift("Bonus", 3);
    moveTo(0, 0.1);
    expect(screen.queryByLabelText(/items$/)).toBeNull();
    release(0, 0.1);

    expect(cardTitles()).toEqual(["Bonus", "Episode 1", "Extras", "Episode 2"]);
    await waitFor(() => expect(reorders()).toEqual([{ groupId: 1, itemIds: [104] }]));
  });

  it("カードの隙間でも、いちばん近いカードの間に落とせる", async () => {
    await openGrid();
    lift("Episode 2", 2);
    // Six pixels beyond the card edge is the midpoint of gap-3.
    moveTo(0, 1 + 6 / cardWidth);
    expect(document.querySelector('[data-drop-bar="before"]')).not.toBeNull();
    release(0, 1 + 6 / cardWidth);

    expect(cardTitles()).toEqual(["Episode 1", "Episode 2", "Extras"]);
    await waitFor(() =>
      expect(reorders()).toEqual([{ groupId: 1, itemIds: [103], after: 101 }]),
    );
  });

  it("持ち上げたカードの上は拒否の印を出し、離しても何も送らない", async () => {
    await openGrid();
    lift("Episode 2", 2);
    moveTo(2, 0.5);
    expect(screen.getByLabelText("Can't drop here")).toBeDefined();
    release(2, 0.5);

    await act(async () => {
      await Promise.resolve();
    });
    expect(server.writes).toEqual([]);
    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
  });

  it("Undo を押すと undo を送り、元の順を出して「Undone」を知らせる", async () => {
    await openGrid();
    lift("Episode 2", 2);
    moveTo(0, 0.1);
    await releaseAndSettle(0, 0.1);
    await screen.findByText('Reordered "Season"');

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));

    expect(await screen.findByText("Undone")).toBeDefined();
    expect(server.writes.map((each) => each.path)).toEqual([
      "/api/organize/reorder",
      "/api/organize/undo",
    ]);
    expect(server.writes[1]?.body).toEqual({ token: "token-1" });
    await waitFor(() =>
      expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]),
    );
  });

  it("409 undo_conflict では取り消せなかったと知らせ、グループを読み直す", async () => {
    await openGrid();
    lift("Episode 2", 2);
    moveTo(0, 0.1);
    await releaseAndSettle(0, 0.1);
    await screen.findByText('Reordered "Season"');
    server.undoAnswer = () =>
      json({ code: "conflict", reason: "undo_conflict", message: "x" }, 409);
    const reads = server.reads.get(1) ?? 0;

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));

    expect(
      await screen.findByText("Couldn't undo: it changed in the meantime"),
    ).toBeDefined();
    await waitFor(() => expect(server.reads.get(1)).toBe(reads + 1));
    expect(cardTitles()).toEqual(["Episode 2", "Episode 1", "Extras"]);
  });

  it("失敗した reorder は、その前に出していた順に戻して知らせる", async () => {
    await openGrid();
    const fetchMock = vi.mocked(fetch);
    const original = fetchMock.getMockImplementation() as typeof fetch;
    fetchMock.mockImplementation((input, init) =>
      String(input).endsWith("/organize/reorder")
        ? Promise.resolve(json({ code: "internal", message: "x" }, 500))
        : original(input, init),
    );
    lift("Episode 2", 2);
    moveTo(0, 0.1);
    release(0, 0.1);

    expect(await screen.findByText(/^Couldn't reorder:/)).toBeDefined();
    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("ドラッグ中の Esc では何も送らず、選択も残す", async () => {
    await openGrid();
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: /^Select "?Episode 1"?$/ }));
    lift("Episode 2", 2);
    moveTo(0, 0.1);
    fireEvent.keyDown(document, { key: "Escape", code: "Escape" });
    release(0, 0.1);

    await act(async () => {
      await Promise.resolve();
    });
    expect(server.writes).toEqual([]);
    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
    expect(
      (
        screen.getByRole("checkbox", {
          name: /^Select "?Episode 1"?$/,
        }) as HTMLButtonElement
      ).dataset.state,
    ).toBe("checked");
  });

  it("動かさないクリックはカードを開く", async () => {
    await openGrid();
    const user = userEvent.setup();
    await user.click(screen.getByRole("link", { name: "Episode 2" }));
    expect(await screen.findByText(/^Opened \/videos\/11\?group=1 /)).toBeDefined();
    expect(server.writes).toEqual([]);
  });

  it("リストの行も、どこからでも持ち上げて間に落とせる", async () => {
    seed();
    vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
      this: Element,
    ) {
      const row = this.closest("tr[data-item-id]");
      const index =
        row === null ? 0 : Array.from(row.parentElement?.children ?? []).indexOf(row);
      const top = index * 48;
      const size = row === null ? 0 : 48;
      return {
        x: 0,
        y: top,
        top,
        left: 0,
        right: 800,
        bottom: top + size,
        width: row === null ? 0 : 800,
        height: size,
        toJSON: () => ({}),
      } as DOMRect;
    });
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1 });
    await user.click(await screen.findByRole("radio", { name: "List" }));
    const table = screen.getByRole("table", { name: "Items" });
    const at = (row: number, ratio: number) => ({
      clientX: 400,
      clientY: row * 48 + 48 * ratio,
    });

    fireEvent.pointerDown(within(table).getByRole("link", { name: "Episode 2" }), {
      ...mouse,
      ...at(2, 0.5),
    });
    fireEvent.pointerMove(document, { ...mouse, ...at(2, 0.2) });
    fireEvent.pointerMove(document, { ...mouse, ...at(0, 0.1) });
    fireEvent.pointerUp(document, { ...mouse, ...at(0, 0.1) });

    await waitFor(() => expect(reorders()).toEqual([{ groupId: 1, itemIds: [103] }]));
    expect(
      within(table)
        .getAllByRole("link")
        .map((link) => link.getAttribute("title")),
    ).toEqual(["Episode 2", "Episode 1", "Extras"]);
  });

  it("リストのハンドルは 044 のまま 4px で持ち上がる", async () => {
    seed();
    layoutRows();
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1 });
    await user.click(await screen.findByRole("radio", { name: "List" }));
    const handle = screen.getByRole("button", { name: 'Reorder "Episode 2"' });

    fireEvent.pointerDown(handle, { ...mouse, clientX: 10, clientY: 120 });
    fireEvent.pointerMove(document, { ...mouse, clientX: 10, clientY: 115 });
    expect(handle.getAttribute("aria-pressed")).toBe("true");

    fireEvent.pointerMove(document, { ...mouse, clientX: 10, clientY: 5 });
    fireEvent.pointerUp(document, { ...mouse, clientX: 10, clientY: 5 });
    await waitFor(() => expect(reorders()).toEqual([{ groupId: 1, itemIds: [103] }]));
  });

  it("ゲストにはドラッグが無い", async () => {
    seed();
    server.groups.get(1)?.items.push({
      id: 104,
      kind: "video",
      video: video(13, { title: "Episode 3" }),
    });
    layoutCards();
    renderPage("/groups/1", "guest");
    await screen.findByRole("heading", { level: 1, name: "Season" });

    lift("Episode 3", 3);
    moveTo(0, 0.1);
    release(0, 0.1);

    await act(async () => {
      await Promise.resolve();
    });
    expect(document.querySelector("[data-drag-card]")).toBeNull();
    expect(document.querySelector("[data-drag-preview]")).toBeNull();
    expect(server.writes).toEqual([]);
  });
});
