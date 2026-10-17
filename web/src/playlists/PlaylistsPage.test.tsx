import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PlaylistSummary } from "../api/playlists";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import PlaylistsPage from "./PlaylistsPage";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function playlist(
  id: number,
  name: string,
  itemCount: number,
  covers: PlaylistSummary["covers"] = [],
): PlaylistSummary {
  return { id, name, itemCount, updatedAt: "2026-09-01T00:00:00Z", covers };
}

function cover(videoId: number): NonNullable<PlaylistSummary["covers"]>[number] {
  return { videoId, thumbnailUrl: `/api/videos/${String(videoId)}/thumbnail?v=x` };
}

const server = {
  /** 一覧の応答（最後に変えた順）。作るとその先頭に入る。 */
  items: [] as PlaylistSummary[],
  /** 1 ページの件数。items がこれより多ければ nextCursor（次の先頭の位置）を返す。 */
  pageSize: 60,
  /** 一覧の版。items を変えるたびに進める。 */
  version: 1,
  listStatus: 200,
  /** 続きのページ（cursor 付き）の要求だけを失敗にする状態。0 なら応答を返さない。 */
  moreStatus: 200,
  listCalls: 0,
  /** 一覧の要求の URL。 */
  listUrls: [] as string[],
  /** 作る要求の本文。 */
  creates: [] as unknown[],
  createStatus: 201,
  nextId: 100,
};

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url.startsWith("/api/playlists?") && method === "GET") {
      server.listCalls += 1;
      server.listUrls.push(url);
      const params = new URL(url, "http://test").searchParams;
      const cursor = params.get("cursor");
      if (server.listStatus !== 200) {
        return Promise.resolve(
          json({ code: "internal", message: "x" }, server.listStatus),
        );
      }
      if (cursor !== null && server.moreStatus === 0) {
        return new Promise<Response>(() => {});
      }
      if (cursor !== null && server.moreStatus !== 200) {
        return Promise.resolve(
          json({ code: "internal", message: "x" }, server.moreStatus),
        );
      }
      const start = cursor === null ? 0 : Number(cursor);
      const end = start + server.pageSize;
      return Promise.resolve(
        json({
          items: server.items.slice(start, end),
          total: server.items.length,
          version: String(server.version),
          ...(end < server.items.length ? { nextCursor: String(end) } : {}),
        }),
      );
    }
    if (url === "/api/playlists" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { name: string };
      server.creates.push(body);
      if (server.createStatus === 400) {
        return Promise.resolve(
          json({ code: "invalid_request", message: "bad name" }, 400),
        );
      }
      if (server.createStatus !== 201) {
        return Promise.resolve(
          json({ code: "internal", message: "x" }, server.createStatus),
        );
      }
      const created = {
        ...playlist(server.nextId, body.name, 0),
        updatedAt: "2026-09-02T00:00:00Z",
      };
      server.nextId += 1;
      server.items = [created, ...server.items];
      server.version += 1;
      return Promise.resolve(json(created, 201));
    }
    throw new Error(`unexpected request: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
}

function renderPage() {
  render(
    <MemoryRouter initialEntries={["/playlists"]}>
      <TooltipProvider>
        <ToastProvider>
          <Routes>
            <Route path="/playlists" element={<PlaylistsPage />} />
            <Route path="/playlists/:id" element={<p>Playlist screen</p>} />
          </Routes>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

/** cards は格子のカードを上から返す。 */
function cards(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>("article[data-playlist-id]"));
}

/** cardNames はカードのリンクの読み上げ名を上から返す。 */
function cardNames(): string[] {
  return cards().map(
    (card) => within(card).getByRole("link").getAttribute("aria-label") ?? "",
  );
}

/** findCards は格子が描かれるのを待つ。 */
async function findCards(): Promise<HTMLElement[]> {
  await screen.findAllByRole("article");
  return cards();
}

/** 見張っている（observe から disconnect までの）IntersectionObserver の知らせ先。 */
let intersect: IntersectionObserverCallback | undefined;
/** 作られた IntersectionObserver の数。 */
let observers = 0;

/**
 * reachEnd は格子の末尾の見張りが画面に近づいたことにする。見張りは描画の後の effect で
 * 付くので、カードが見えた時点ではまだ無いことがある。付くのを待ってから知らせる。
 */
async function reachEnd() {
  await waitFor(() => expect(intersect).toBeDefined());
  act(() => {
    intersect?.(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      {} as IntersectionObserver,
    );
  });
}

beforeEach(() => {
  server.items = [
    playlist(3, "Morning", 12, [cover(31), cover(32), cover(33), cover(34)]),
    playlist(1, "Road trip", 1, [cover(11)]),
    playlist(2, "Empty one", 0),
  ];
  server.pageSize = 60;
  server.version = 1;
  server.listStatus = 200;
  server.moreStatus = 200;
  server.listCalls = 0;
  server.listUrls = [];
  server.creates = [];
  server.createStatus = 201;
  server.nextId = 100;
  intersect = undefined;
  observers = 0;
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      private readonly callback: IntersectionObserverCallback;
      constructor(callback: IntersectionObserverCallback) {
        this.callback = callback;
        observers += 1;
      }
      observe() {
        intersect = this.callback;
      }
      disconnect() {
        if (intersect === this.callback) intersect = undefined;
      }
    },
  );
  install();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("PlaylistsPage", () => {
  it("プレイリストを API の順（最後に変えた順）にカードで並べ、項目数とともに出す", async () => {
    renderPage();
    expect(screen.getByRole("heading", { level: 1, name: "Playlists" })).toBeDefined();
    expect(document.title).toBe("Playlists");

    await findCards();
    expect(cardNames()).toEqual([
      "Morning, 12 items",
      "Road trip, 1 item",
      "Empty one, 0 items",
    ]);
    expect(screen.getByText("3 playlists")).toBeDefined();
    expect(screen.getByText("0 items")).toBeDefined();
    expect(
      screen.getByRole("link", { name: "Morning, 12 items" }).getAttribute("href"),
    ).toBe("/playlists/3");
    // 表の列は無い。
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("カードはグループのカードの絵柄に先頭の項目のサムネイルを差し込み、名前と更新の時刻を出す", async () => {
    renderPage();
    const [morning, roadTrip, emptyOne] = await findCards();
    if (morning === undefined || roadTrip === undefined || emptyOne === undefined) {
      throw new Error("cards not rendered");
    }
    const sheets = (card: HTMLElement) =>
      Array.from(
        card.querySelectorAll<HTMLImageElement>(
          "[data-folder-art] [data-folder-preview] img",
        ),
      ).map((img) => img.getAttribute("src"));
    expect(sheets(morning)).toEqual([
      "/api/videos/31/thumbnail?v=x",
      "/api/videos/32/thumbnail?v=x",
      "/api/videos/33/thumbnail?v=x",
      "/api/videos/34/thumbnail?v=x",
    ]);
    expect(sheets(roadTrip)).toEqual(["/api/videos/11/thumbnail?v=x"]);
    // 空のプレイリストはフォルダの背板だけ。
    expect(emptyOne.querySelector("[data-folder-art]")).not.toBeNull();
    expect(sheets(emptyOne)).toEqual([]);

    expect(
      within(morning).getByRole("heading", { level: 3, name: "Morning" }),
    ).toBeDefined();
    expect(within(morning).getByText("12 items")).toBeDefined();
    expect(within(emptyOne).getByText("0 items")).toBeDefined();
    // 最後に変えた時刻は、表の列ではなく名前の下の 1 行。
    const updated = morning.querySelector("p span[title]");
    expect(updated?.textContent).not.toBe("");
  });

  it("カードを押すとそのプレイリストの画面を開く", async () => {
    const user = userEvent.setup();
    renderPage();
    await findCards();
    await user.click(screen.getByRole("link", { name: "Road trip, 1 item" }));
    expect(await screen.findByText("Playlist screen")).toBeDefined();
  });

  it("同じ名前の 2 つのプレイリストをまとめずにどちらも並べる", async () => {
    server.items = [playlist(5, "Same", 2), playlist(4, "Same", 7)];
    renderPage();

    await findCards();
    expect(cardNames()).toEqual(["Same, 2 items", "Same, 7 items"]);
    expect(screen.getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual([
      "/playlists/5",
      "/playlists/4",
    ]);
  });

  it("名前を入れて作ると、トリムした名前を送り、先頭のカードのリンクへフォーカスを移す", async () => {
    const user = userEvent.setup();
    renderPage();
    await findCards();

    await user.click(screen.getByRole("button", { name: "New playlist" }));
    const dialog = await screen.findByRole("dialog", { name: "New playlist" });
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    expect(document.activeElement).toBe(field);
    await user.type(field, "  Evening  ");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(server.creates).toEqual([{ name: "Evening" }]);
    expect(cardNames()[0]).toBe("Evening, 0 items");
    expect(screen.getByText("4 playlists")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("link", { name: "Evening, 0 items" }),
      ),
    );
    // 作った後に一覧を読み直す。
    await waitFor(() => expect(server.listCalls).toBe(2));
    expect(cardNames()).toHaveLength(4);
  });

  it("空の名前と 100 文字を超える名前は送らず、名前の規則を欄の誤りとして出す", async () => {
    const user = userEvent.setup();
    renderPage();
    await findCards();

    await user.click(screen.getByRole("button", { name: "New playlist" }));
    const dialog = await screen.findByRole("dialog", { name: "New playlist" });
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    await user.type(field, "   ");
    // 入れている間は誤りを出さない。
    expect(within(dialog).queryByText("Use a name of 1 to 100 characters.")).toBeNull();
    await user.keyboard("{Enter}");

    expect(within(dialog).getByText("Use a name of 1 to 100 characters.")).toBeDefined();
    expect(field.getAttribute("aria-invalid")).toBe("true");
    await user.clear(field);
    expect(field.getAttribute("aria-invalid")).toBeNull();

    await user.click(field);
    await user.paste("a".repeat(101));
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(within(dialog).getByText("Use a name of 1 to 100 characters.")).toBeDefined();
    expect(server.creates).toEqual([]);
  });

  it("400 は名前の規則を、ほかの失敗は理由を添えた 1 行を窓の中に出し、窓を開いたままにする", async () => {
    const user = userEvent.setup();
    renderPage();
    await findCards();
    await user.click(screen.getByRole("button", { name: "New playlist" }));
    const dialog = await screen.findByRole("dialog", { name: "New playlist" });
    const field = within(dialog).getByRole("textbox", { name: "Name" });

    server.createStatus = 400;
    await user.type(field, "Night{Enter}");
    expect(
      await within(dialog).findByText("Use a name of 1 to 100 characters."),
    ).toBeDefined();

    server.createStatus = 500;
    await user.type(field, "s{Enter}");
    expect(
      await within(dialog).findByText(/^Couldn't create the playlist: /),
    ).toBeDefined();
    expect(screen.getByRole("dialog", { name: "New playlist" })).toBeDefined();
    expect(server.items).toHaveLength(3);
  });

  it("Cancel で閉じると入れた名前を捨てる", async () => {
    const user = userEvent.setup();
    renderPage();
    await findCards();
    await user.click(screen.getByRole("button", { name: "New playlist" }));
    let dialog = await screen.findByRole("dialog", { name: "New playlist" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "Draft");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(screen.getByRole("button", { name: "New playlist" }));
    dialog = await screen.findByRole("dialog", { name: "New playlist" });
    expect(
      (within(dialog).getByRole("textbox", { name: "Name" }) as HTMLInputElement).value,
    ).toBe("");
    expect(server.creates).toEqual([]);
  });

  it("空なら本体の「New playlist」だけを出し、作ると格子に変わる", async () => {
    server.items = [];
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText("No playlists yet")).toBeDefined();
    const buttons = screen.getAllByRole("button", { name: "New playlist" });
    expect(buttons).toHaveLength(1);
    await user.click(buttons[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "New playlist" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Name" }),
      "First{Enter}",
    );

    await findCards();
    expect(cardNames()).toEqual(["First, 0 items"]);
    expect(screen.getByText("1 playlist")).toBeDefined();
  });

  it("読み込みに失敗したら Retry で読み直す", async () => {
    server.listStatus = 500;
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText("Couldn't load the playlists")).toBeDefined();
    server.listStatus = 200;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await findCards();
    expect(cardNames()).toHaveLength(3);
  });

  it("1 ページずつ読み、末尾の見張りが近づくと nextCursor で続きを足し、件数は全部の数を出す", async () => {
    server.items = [1, 2, 3, 4, 5].map((id) => playlist(id, `P${String(id)}`, id));
    server.pageSize = 2;
    renderPage();

    await findCards();
    expect(cardNames()).toEqual(["P1, 1 item", "P2, 2 items"]);
    expect(screen.getByText("5 playlists")).toBeDefined();
    expect(server.listUrls).toEqual(["/api/playlists?limit=60"]);
    await waitFor(() => expect(observers).toBe(1));

    await reachEnd();
    await waitFor(() => expect(cards()).toHaveLength(4));
    expect(cardNames()[2]).toBe("P3, 3 items");
    expect(server.listUrls[1]).toBe("/api/playlists?cursor=2&limit=60");
    expect(screen.getByText("5 playlists")).toBeDefined();

    await reachEnd();
    await waitFor(() => expect(cards()).toHaveLength(5));
    expect(server.listUrls).toHaveLength(3);
    // 続きが無くなれば見張りを外して付け直さず、同じ要求を繰り返さない。
    await waitFor(() => expect(intersect).toBeUndefined());
    expect(observers).toBe(2);
    expect(server.listUrls).toHaveLength(3);
    expect(document.querySelector('[data-slot="load-more-row"]')).toBeNull();
  });

  it("続きの読み込みに失敗したら今のカードを残して Retry の行を出し、Retry は同じページを求める", async () => {
    const user = userEvent.setup();
    server.items = [1, 2, 3].map((id) => playlist(id, `P${String(id)}`, id));
    server.pageSize = 2;
    server.moreStatus = 500;
    renderPage();
    await findCards();

    await reachEnd();
    const row = await screen.findByText(/^Couldn't load more: /);
    expect(row.closest('[data-slot="load-more-row"]')?.getAttribute("data-state")).toBe(
      "failed",
    );
    expect(cards()).toHaveLength(2);
    expect(screen.getByText("3 playlists")).toBeDefined();

    server.moreStatus = 200;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(cards()).toHaveLength(3));
    expect(server.listUrls.slice(1)).toEqual([
      "/api/playlists?cursor=2&limit=60",
      "/api/playlists?cursor=2&limit=60",
    ]);
    expect(document.querySelector('[data-slot="load-more-row"]')).toBeNull();
  });

  it("続きのページの version が違えば足さず、最初のページから読み直して置き換える", async () => {
    server.items = [1, 2, 3, 4, 5].map((id) => playlist(id, `P${String(id)}`, id));
    server.pageSize = 2;
    renderPage();
    await findCards();
    expect(cardNames()).toEqual(["P1, 1 item", "P2, 2 items"]);

    // 2 ページ目を読む前に P5 が変わって先頭へ動いた。元の cursor の次からは P5 は返らない。
    const [moved] = server.items.splice(4, 1);
    server.items = [moved as PlaylistSummary, ...server.items];
    server.version += 1;

    await reachEnd();
    await waitFor(() => expect(cardNames()).toEqual(["P5, 5 items", "P1, 1 item"]));
    expect(server.listUrls).toEqual([
      "/api/playlists?limit=60",
      "/api/playlists?cursor=2&limit=60",
      "/api/playlists?limit=60",
    ]);
    expect(screen.getByText("5 playlists")).toBeDefined();

    // 読み直した後は見張りが付け直され、続きは新しい並びで足される。
    await reachEnd();
    await waitFor(() => expect(cards()).toHaveLength(4));
    expect(cardNames()).toEqual([
      "P5, 5 items",
      "P1, 1 item",
      "P2, 2 items",
      "P3, 3 items",
    ]);
    await reachEnd();
    await waitFor(() => expect(cards()).toHaveLength(5));
    expect(cardNames()[4]).toBe("P4, 4 items");
  });

  it("作ると読んでいた続きを打ち切って最初のページを読み直し、失敗したら Retry の行を出し、Retry は最初のページを読み直す", async () => {
    const user = userEvent.setup();
    server.items = [1, 2, 3, 4].map((id) => playlist(id, `P${String(id)}`, id));
    server.pageSize = 2;
    renderPage();
    await findCards();

    // 2 ページ目の要求は応答が返らないまま、プレイリストを作る。
    server.moreStatus = 0;
    await reachEnd();
    expect(server.listUrls[1]).toBe("/api/playlists?cursor=2&limit=60");
    server.listStatus = 500;
    await user.click(screen.getByRole("button", { name: "New playlist" }));
    const dialog = await screen.findByRole("dialog", { name: "New playlist" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Name" }),
      "Evening{Enter}",
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    // 読み直しは最初のページを求め、失敗したら今のカードを残して Retry の行を出す。
    const row = await screen.findByText("Couldn't load the playlists");
    expect(row.closest('[data-slot="load-more-row"]')?.getAttribute("data-state")).toBe(
      "failed",
    );
    expect(server.listUrls[2]).toBe("/api/playlists?limit=60");
    expect(cardNames()).toEqual(["Evening, 0 items", "P1, 1 item", "P2, 2 items"]);
    expect(screen.getByText("5 playlists")).toBeDefined();
    expect(server.listUrls).toHaveLength(3);

    // Retry は最初のページを読み直して置き換え、見張りが付け直されて続きが読める。
    server.listStatus = 200;
    server.moreStatus = 200;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() =>
      expect(document.querySelector('[data-slot="load-more-row"]')).toBeNull(),
    );
    expect(server.listUrls[3]).toBe("/api/playlists?limit=60");
    expect(cardNames()).toEqual(["Evening, 0 items", "P1, 1 item"]);
    await reachEnd();
    await waitFor(() => expect(cards()).toHaveLength(4));
    expect(server.listUrls[4]).toBe("/api/playlists?cursor=2&limit=60");
    expect(cardNames()).toEqual([
      "Evening, 0 items",
      "P1, 1 item",
      "P2, 2 items",
      "P3, 3 items",
    ]);
  });

  it("疑似ロケールではカタログの文言だけを描く", async () => {
    enablePseudoLocale();
    renderPage();
    await findCards();
    expectCatalogTextOnly(document.body, ["Morning", "Road trip", "Empty one"]);
  });
});
