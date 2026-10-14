import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import {
  json,
  renderPage,
  seed,
  server,
  setUpGroupPageServer,
  video,
} from "../testing/groupPage";

setUpGroupPageServer();

// 子グループのカードと道筋の段へ落として移す（specs/050-drag-organize/ui-design.md
// 「What a drop does」「Group page」）。jsdom は配置を持たないので、格子のカードを横一列に
// 200px ずつ、道筋の段をその下に 100px ずつ置き、ポインタの出来事で持ち上げて落とす。

const cardWidth = 190;
const cardHeight = 150;
const step = 200;
const crumbTop = 300;

function box(left: number, top: number, width: number, height: number): DOMRect {
  return {
    x: left,
    y: top,
    top,
    left,
    right: left + width,
    bottom: top + height,
    width,
    height,
    toJSON: () => ({}),
  } as DOMRect;
}

/** layout はカードを左から 200px ずつ、道筋の段を y=300 に左から 100px ずつ置く。 */
function layout() {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
    this: Element,
  ) {
    const crumb = this.closest("[data-crumb-drop]");
    if (crumb !== null) {
      const index = Array.from(document.querySelectorAll("[data-crumb-drop]")).indexOf(
        crumb,
      );
      return box(index * 100, crumbTop, 80, 20);
    }
    const card = this.closest("[data-drag-card]");
    if (card === null) return box(0, 0, 0, 0);
    const index = Array.from(card.parentElement?.children ?? []).indexOf(card);
    return box(index * step, 0, cardWidth, cardHeight);
  });
}

const mouse = { pointerType: "mouse", isPrimary: true, button: 0, pointerId: 1 };

function onCard(index: number, ratio = 0.5) {
  return { clientX: index * step + cardWidth * ratio, clientY: cardHeight / 2 };
}

/** onCrumb は道筋の `index` 番目の段（「…」を含め、帯に描いた順）の中の点である。 */
function onCrumb(index: number) {
  return { clientX: index * 100 + 40, clientY: crumbTop + 10 };
}

function lift(title: string | RegExp, index: number) {
  const link = screen.getByRole("link", { name: title });
  fireEvent.pointerDown(link, { ...mouse, ...onCard(index) });
  fireEvent.pointerMove(document, { ...mouse, ...onCard(index, 0.4) });
}

function moveTo(point: { clientX: number; clientY: number }) {
  fireEvent.pointerMove(document, { ...mouse, ...point });
}

function release(point: { clientX: number; clientY: number }) {
  fireEvent.pointerUp(document, { ...mouse, ...point });
}

function cardTitles(): string[] {
  return Array.from(document.querySelectorAll("article")).map(
    (card) => card.querySelector("h3")?.textContent ?? "",
  );
}

function moves() {
  return server.writes
    .filter((each) => each.path === "/api/organize/move")
    .map((each) => each.body);
}

/** openSeason は「Season」（Episode 1、Extras、Episode 2）を開く。 */
async function openSeason() {
  seed();
  layout();
  renderPage("/groups/1");
  await screen.findByRole("heading", { level: 1, name: "Season" });
}

describe("GroupPage move", () => {
  it("子グループのカードに落とした動画は from と to を送り、すぐ外れて件数が減る", async () => {
    await openSeason();
    lift("Episode 1", 0);
    moveTo(onCard(1));
    expect(screen.getByText("Put in")).toBeDefined();
    release(onCard(1));

    expect(cardTitles()).toEqual(["Extras", "Episode 2"]);
    expect(screen.getByText(/^2 videos/)).toBeDefined();
    await waitFor(() =>
      expect(moves()).toEqual([{ items: [{ kind: "video", id: 10 }], from: 1, to: 5 }]),
    );
    expect(await screen.findByText('Moved 1 item to "Extras"')).toBeDefined();
  });

  it("選択した 3 枚は、画面の順で 3 つの参照を送り、選択は解ける", async () => {
    seed();
    server.groups.get(1)?.items.push({
      id: 104,
      kind: "video",
      video: video(13, { title: "Episode 3" }),
    });
    layout();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    const user = userEvent.setup();
    for (const title of ["Episode 3", "Episode 1", "Episode 2"]) {
      await user.click(screen.getByRole("checkbox", { name: `Select "${title}"` }));
    }

    lift("Episode 3", 3);
    moveTo(onCard(1));
    expect(screen.getByLabelText("3 items").textContent).toBe("3");
    release(onCard(1));

    expect(cardTitles()).toEqual(["Extras"]);
    await waitFor(() =>
      expect(moves()).toEqual([
        {
          items: [
            { kind: "video", id: 10 },
            { kind: "video", id: 11 },
            { kind: "video", id: 13 },
          ],
          from: 1,
          to: 5,
        },
      ]),
    );
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();
  });

  it("道筋の「Library」の段に落としたカードは from だけを送る", async () => {
    await openSeason();
    lift("Episode 2", 2);
    moveTo(onCrumb(0));
    expect(document.querySelector('[data-crumb-drop="active"]')?.textContent).toBe(
      "Library",
    );
    release(onCrumb(0));

    expect(cardTitles()).toEqual(["Episode 1", "Extras"]);
    await waitFor(() =>
      expect(moves()).toEqual([{ items: [{ kind: "video", id: 11 }], from: 1 }]),
    );
    expect(await screen.findByText('Moved 1 item out of "Season"')).toBeDefined();
  });

  it("開いているグループの段の上は拒否の印を出し、落としても何も送らない", async () => {
    await openSeason();
    lift("Episode 2", 2);
    // 帯は「Library」「Season」の 2 段。
    moveTo(onCrumb(1));
    expect(screen.getByLabelText("Can't drop here")).toBeDefined();
    release(onCrumb(1));

    await act(async () => {
      await Promise.resolve();
    });
    expect(server.writes).toEqual([]);
    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
  });

  it("子グループのカードを自分の上に持っていくと拒否の印を出し、落としても何も送らない", async () => {
    await openSeason();
    lift(/^Extras/, 1);
    moveTo(onCard(1));
    expect(screen.getByLabelText("Can't drop here")).toBeDefined();
    release(onCard(1));

    await act(async () => {
      await Promise.resolve();
    });
    expect(server.writes).toEqual([]);
    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
  });

  it("Undo を押すと undo を送り、カードが元に戻る", async () => {
    await openSeason();
    lift("Episode 1", 0);
    moveTo(onCard(1));
    release(onCard(1));
    await screen.findByText('Moved 1 item to "Extras"');

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));

    expect(await screen.findByText("Undone")).toBeDefined();
    expect(server.writes.map((each) => each.path)).toEqual([
      "/api/organize/move",
      "/api/organize/undo",
    ]);
    await waitFor(() =>
      expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]),
    );
  });

  it("失敗した move は外したカードと件数を戻し、Undo を出さずに知らせる", async () => {
    await openSeason();
    const fetchMock = vi.mocked(fetch);
    const original = fetchMock.getMockImplementation() as typeof fetch;
    fetchMock.mockImplementation((input, init) =>
      String(input).endsWith("/organize/move")
        ? Promise.resolve(json({ code: "internal", message: "x" }, 500))
        : original(input, init),
    );
    lift("Episode 1", 0);
    moveTo(onCard(1));
    release(onCard(1));

    expect(await screen.findByText(/^Couldn't move:/)).toBeDefined();
    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
    expect(screen.getByText(/^3 videos/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("祖先の段に最後の項目を落とすと from と to を送り、解散した画面を離れて取り消しの通知を持ち越す", async () => {
    seed();
    layout();
    renderPage("/groups/5?trail=1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });

    lift("Bonus", 0);
    // 帯は「Library」「Season」「Extras」の 3 段。
    moveTo(onCrumb(1));
    release(onCrumb(1));

    await waitFor(() =>
      expect(moves()).toEqual([{ items: [{ kind: "video", id: 12 }], from: 5, to: 1 }]),
    );
    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(screen.getByText('Dissolved "Extras"')).toBeDefined();
    expect(screen.getByText('Moved 1 item to "Season"')).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(await screen.findByText("Undone")).toBeDefined();
    expect(server.groups.has(5)).toBe(true);
  });
});
