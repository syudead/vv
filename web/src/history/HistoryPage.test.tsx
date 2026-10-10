import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import type { WatchHistoryEntry } from "../api/history";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import HistoryPage from "./HistoryPage";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `Video ${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "done",
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    container: "mp4",
    videoCodec: "h264",
    tags: [],
    ...extra,
  };
}

/** 見る人のローカル時刻で作った playedAt。日の見出しは日付になる（2026 年 9 月）。 */
function at(day: number, hour: number, minute = 0): string {
  return new Date(2026, 8, day, hour, minute).toISOString();
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const server = {
  entries: [] as WatchHistoryEntry[],
  pageSize: 60,
  listUrls: [] as string[],
  deletes: [] as string[],
  deleteStatus: 204,
  clearStatus: 204,
  /** 有れば、一覧の応答を作った後、これが解けるまで返さない。 */
  holdList: null as Promise<void> | null,
  /** 200 でなければ、一覧の要求にこの状態で答える。 */
  listStatus: 200,
  /** 日付の一覧の応答。条件で絞らずにそのまま返す。 */
  dates: [] as string[],
  datesUrls: [] as string[],
  /** 200 でなければ、日付の一覧の要求にこの状態で答える。 */
  datesStatus: 200,
};

/** matches は一覧の要求の条件（watch・query・date）に件が合うかである。 */
function matches(entry: WatchHistoryEntry, query: URLSearchParams): boolean {
  const watch = query.get("watch");
  const progress = entry.video?.progress;
  if (watch === "inProgress" && (progress === undefined || progress.completed))
    return false;
  if (watch === "watched" && progress?.completed !== true) return false;
  const text = query.get("query");
  const title = entry.video?.title ?? entry.title;
  if (text !== null && !title.toLowerCase().includes(text.toLowerCase())) return false;
  const date = query.get("date");
  if (date !== null) {
    const [year = 0, month = 1, day] = date.split("-").map(Number);
    const end =
      day === undefined ? new Date(year, month, 1) : new Date(year, month - 1, day + 1);
    if (new Date(entry.playedAt).getTime() >= end.getTime()) return false;
  }
  return true;
}

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url.startsWith("/api/watch-history?") && method === "GET") {
      server.listUrls.push(url);
      const query = new URLSearchParams(url.slice(url.indexOf("?") + 1));
      // 鍵は前のページの最後の件の id。その間に件が消えても続きの位置はずれない。
      const cursor = query.get("cursor");
      const matching = server.entries.filter((entry) => matches(entry, query));
      const rest =
        cursor === null
          ? matching
          : matching.filter((entry) => entry.id < Number(cursor));
      const items = rest.slice(0, server.pageSize);
      const last = items.at(-1);
      const body = json({
        items,
        ...(rest.length > items.length && last !== undefined
          ? { nextCursor: String(last.id) }
          : {}),
      });
      // 状態は応答を返すときに読むので、止めた応答も後から失敗にできる。
      const respond = () =>
        server.listStatus === 200
          ? body
          : json({ code: "internal", message: "x" }, server.listStatus);
      const hold = server.holdList;
      return hold === null ? Promise.resolve(respond()) : hold.then(respond);
    }
    if (url.startsWith("/api/watch-history/dates?") && method === "GET") {
      server.datesUrls.push(url);
      return Promise.resolve(
        server.datesStatus === 200
          ? json({ days: server.dates })
          : json({ code: "internal", message: "x" }, server.datesStatus),
      );
    }
    if (url.startsWith("/api/watch-history") && method === "DELETE") {
      server.deletes.push(url);
      if (url === "/api/watch-history") {
        if (server.clearStatus !== 204) {
          return Promise.resolve(
            json({ code: "internal", message: "x" }, server.clearStatus),
          );
        }
        server.entries = [];
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (server.deleteStatus !== 204) {
        const code = server.deleteStatus === 404 ? "not_found" : "internal";
        return Promise.resolve(json({ code, message: "x" }, server.deleteStatus));
      }
      const id = Number(url.split("/").at(-1));
      server.entries = server.entries.filter((entry) => entry.id !== id);
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    throw new Error(`unexpected request: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
}

let intersect: IntersectionObserverCallback | undefined;
/** 作られた IntersectionObserver の数。 */
let observers = 0;
let location: { pathname: string; state: unknown } | undefined;
let search = "";

function LocationProbe() {
  const current = useLocation();
  location = { pathname: current.pathname, state: current.state };
  search = current.search;
  return null;
}

function renderPage(url = "/history") {
  render(
    <MemoryRouter initialEntries={[url]}>
      <TooltipProvider>
        <ToastProvider>
          <Routes>
            <Route path="/history" element={<HistoryPage />} />
            <Route path="/videos/:id" element={<p>Video page</p>} />
          </Routes>
          <LocationProbe />
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function historyList(): HTMLElement {
  return screen.getByRole("list", { name: "Watch history" });
}

/** positionBar は行の位置の行のバー（最大が動画の長さのもの）である。 */
function positionBar(bars: HTMLElement[]): HTMLElement {
  const bar = bars.find((item) => item.getAttribute("aria-valuemax") === "2538000");
  if (bar === undefined) throw new Error("no position bar");
  return bar;
}

function toasts(): string[] {
  return Array.from(document.querySelectorAll("[data-sonner-toast]")).map(
    (toast) => toast.textContent ?? "",
  );
}

beforeEach(() => {
  // 日の見出しの「今日」「今年」を決める今の日付を固定する（2026 年 10 月 1 日）。
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 9, 1, 12, 0));
  server.entries = [
    {
      id: 4,
      playedAt: at(27, 21, 30),
      title: "Old name",
      video: video(10, { title: "Harbour lights" }),
    },
    { id: 3, playedAt: at(27, 15, 4), title: "Gone" },
    { id: 2, playedAt: at(26, 9, 0), title: "" },
    {
      id: 1,
      playedAt: at(26, 8, 0),
      title: "Kyoto",
      video: video(11, { title: "Kyoto" }),
    },
  ];
  server.pageSize = 60;
  server.listUrls = [];
  server.deletes = [];
  server.deleteStatus = 204;
  server.clearStatus = 204;
  server.holdList = null;
  server.listStatus = 200;
  server.dates = ["2026-09-27", "2026-09-26"];
  server.datesUrls = [];
  server.datesStatus = 200;
  search = "";
  vi.stubGlobal("scrollTo", vi.fn());
  intersect = undefined;
  observers = 0;
  location = undefined;
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersect = callback;
        observers += 1;
      }
      observe() {}
      disconnect() {}
    },
  );
  install();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("HistoryPage（specs/043-watch-history/ui-design.md「History screen」）", () => {
  it("件を新しい順に、日の見出しの下へ時刻と題名つきで時間軸に並べる", async () => {
    renderPage();
    expect(screen.getByRole("heading", { level: 1, name: "History" })).toBeDefined();
    expect(document.title).toBe("History");

    await screen.findByRole("list", { name: "Watch history" });
    const headings = within(historyList())
      .getAllByRole("heading", { level: 2 })
      .map((heading) => heading.textContent);
    expect(headings).toEqual(["Sunday · Sep 27", "Saturday · Sep 26"]);

    const removeNames = within(historyList())
      .getAllByRole("button", { name: /^Remove/ })
      .map((button) => button.getAttribute("aria-label"));
    expect(removeNames).toEqual([
      expect.stringMatching(
        /^Remove "Harbour lights" played Sunday, Sep 27 at 9:30\sPM from history$/,
      ),
      expect.stringMatching(
        /^Remove "Gone" played Sunday, Sep 27 at 3:04\sPM from history$/,
      ),
      expect.stringMatching(
        /^Remove "Unknown video" played Saturday, Sep 26 at 9:00\sAM from history$/,
      ),
      expect.stringMatching(
        /^Remove "Kyoto" played Saturday, Sep 26 at 8:00\sAM from history$/,
      ),
    ]);
    expect(within(historyList()).getByText(/^3:04\sPM$/)).toBeDefined();
  });

  it("動画のある件は再生画面を開き、無い件はリンクにせず再生できないと書く", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });

    const links = within(historyList()).getAllByRole("link");
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/videos/10",
      "/videos/11",
    ]);
    // 題名はいまの動画の題名。写しの題名は使わない。
    expect(links[0]?.getAttribute("aria-label")).toMatch(
      /^Harbour lights, 1:00, played Sunday, Sep 27 at 9:30\sPM$/,
    );
    expect(within(historyList()).queryByText("Old name")).toBeNull();

    const gone = within(historyList()).getByText("Gone");
    expect(gone.closest("a")).toBeNull();
    expect(within(historyList()).getAllByText("Not in the library")).toHaveLength(2);
    expect(within(historyList()).getByText("Unknown video").closest("a")).toBeNull();

    await user.click(links[0]!);
    expect(await screen.findByText("Video page")).toBeDefined();
    expect(location).toEqual({ pathname: "/videos/10", state: { from: "/history" } });
  });

  it("途中の件は位置と長さとその比率のバー、「Resume」を出し、見終わった件は「Start over」", async () => {
    const updatedAt = "2026-09-27T12:00:00Z";
    server.entries = [
      {
        id: 4,
        playedAt: at(27, 21, 30),
        title: "Harbour lights",
        video: video(10, {
          title: "Harbour lights",
          durationMs: 2_538_000,
          progress: { positionMs: 965_000, completed: false, updatedAt },
        }),
      },
      {
        id: 3,
        playedAt: at(27, 15, 4),
        title: "Kyoto",
        video: video(11, {
          title: "Kyoto",
          durationMs: 2_538_000,
          progress: { positionMs: 2_530_000, completed: true, updatedAt },
        }),
      },
      {
        id: 2,
        playedAt: at(27, 9, 0),
        title: "Fresh",
        video: video(12, { title: "Fresh" }),
      },
      { id: 1, playedAt: at(26, 9, 0), title: "Gone" },
    ];
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });

    const rows = within(historyList()).getAllByRole("listitem");
    const row = (title: string) => {
      const found = rows.find(
        (item) =>
          item.getAttribute("data-slot") === "timeline-item" &&
          within(item).queryByText(title) !== null,
      );
      if (found === undefined) throw new Error(`no row for ${title}`);
      return within(found);
    };

    const midway = row("Harbour lights");
    expect(midway.getByText("16:05 / 42:18")).toBeDefined();
    // サムネイルの下端の縁（百分率）とは別に、位置の行のバーを長さで描く。
    const bar = positionBar(
      midway.getAllByRole("progressbar", { name: "Watched portion" }),
    );
    expect(bar.getAttribute("aria-valuenow")).toBe("965000");
    expect(midway.getByRole("link", { name: "Resume Harbour lights" })).toBeDefined();
    expect(midway.queryByRole("link", { name: /^Start/ })).toBeNull();

    const watched = row("Kyoto");
    expect(watched.getByText("42:10 / 42:18")).toBeDefined();
    const full = positionBar(
      watched.getAllByRole("progressbar", { name: "Watched portion" }),
    );
    expect(full.getAttribute("aria-valuenow")).toBe("2538000");
    expect(watched.getByRole("link", { name: "Start Kyoto over" })).toBeDefined();
    expect(watched.queryByRole("link", { name: /^Resume/ })).toBeNull();

    // 位置の無い動画は位置も操作も出さず、行そのものが動画を開く。
    const fresh = row("Fresh");
    expect(fresh.queryByRole("progressbar")).toBeNull();
    expect(fresh.getAllByRole("link")).toHaveLength(1);

    // 動画の無い件はどちらの操作も出さない。
    const gone = row("Gone");
    expect(gone.queryByRole("link")).toBeNull();
    expect(gone.queryByRole("progressbar")).toBeNull();
  });

  it("「Resume」は今の履歴の URL を戻り先に、自動再生を頼んで再生画面を開く", async () => {
    const user = userEvent.setup();
    server.entries = [
      {
        id: 4,
        playedAt: at(27, 21, 30),
        title: "Harbour lights",
        video: video(10, {
          title: "Harbour lights",
          progress: { positionMs: 30_000, completed: false, updatedAt: at(27, 21, 40) },
        }),
      },
    ];
    renderPage("/history?q=harbour");
    await screen.findByRole("list", { name: "Watch history" });

    await user.click(screen.getByRole("link", { name: "Resume Harbour lights" }));

    expect(await screen.findByText("Video page")).toBeDefined();
    expect(location).toEqual({
      pathname: "/videos/10",
      state: { from: "/history?q=harbour", autoplay: true },
    });
  });

  it("行そのものは自動再生を頼まずに再生画面を開く", async () => {
    const user = userEvent.setup();
    server.entries = [
      {
        id: 4,
        playedAt: at(27, 21, 30),
        title: "Harbour lights",
        video: video(10, {
          title: "Harbour lights",
          progress: { positionMs: 30_000, completed: false, updatedAt: at(27, 21, 40) },
        }),
      },
    ];
    renderPage("/history?q=harbour");
    await screen.findByRole("list", { name: "Watch history" });

    await user.click(screen.getByRole("link", { name: /^Harbour lights, 1:00, played/ }));

    expect(await screen.findByText("Video page")).toBeDefined();
    expect(location).toEqual({
      pathname: "/videos/10",
      state: { from: "/history?q=harbour" },
    });
  });

  it("続きは前回の nextCursor で読み、同じ日のまとまりに加える", async () => {
    server.pageSize = 3;
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });
    expect(
      within(historyList()).getAllByRole("button", { name: /^Remove/ }),
    ).toHaveLength(3);

    act(() => {
      intersect?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    });

    await waitFor(() =>
      expect(
        within(historyList()).getAllByRole("button", { name: /^Remove/ }),
      ).toHaveLength(4),
    );
    expect(server.listUrls).toEqual([
      "/api/watch-history?limit=60",
      "/api/watch-history?cursor=2&limit=60",
    ]);
    expect(
      within(historyList())
        .getAllByRole("heading", { level: 2 })
        .map((heading) => heading.textContent),
    ).toEqual(["Sunday · Sep 27", "Saturday · Sep 26"]);
  });

  it("× でその行だけを消し、フォーカスを次の行の × へ移す", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });

    await user.click(screen.getByRole("button", { name: /^Remove "Gone"/ }));

    await waitFor(() => expect(screen.queryByText("Gone")).toBeNull());
    expect(server.deletes).toEqual(["/api/watch-history/3"]);
    expect(
      within(historyList()).getAllByRole("button", { name: /^Remove/ }),
    ).toHaveLength(3);
    await waitFor(() =>
      expect(document.activeElement?.getAttribute("aria-label")).toMatch(
        /^Remove "Unknown video"/,
      ),
    );
    expect(toasts()).toEqual([]);
  });

  it("削除の 404 では知らせずに最初のページから読み直す", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });
    // 別のタブで消えた。
    server.entries = server.entries.filter((entry) => entry.id !== 3 && entry.id !== 1);
    server.deleteStatus = 404;

    await user.click(screen.getByRole("button", { name: /^Remove "Gone"/ }));

    await waitFor(() => expect(screen.queryByText("Gone")).toBeNull());
    expect(screen.queryByText("Kyoto")).toBeNull();
    expect(server.listUrls).toHaveLength(2);
    expect(
      within(historyList()).getAllByRole("button", { name: /^Remove/ }),
    ).toHaveLength(2);
    expect(toasts()).toEqual([]);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("読み直しの間に表示の行を消し尽くしても、応答の nextCursor から続きを読む", async () => {
    const user = userEvent.setup();
    server.pageSize = 2;
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });
    // 別のタブで消えたと答えさせ、読み直しの応答を止めておく。
    server.deleteStatus = 404;
    let release = () => {};
    server.holdList = new Promise((resolve) => {
      release = resolve;
    });

    await user.click(screen.getByRole("button", { name: /^Remove "Gone"/ }));
    await waitFor(() => expect(server.listUrls).toHaveLength(2));
    server.deleteStatus = 204;
    await user.click(screen.getByRole("button", { name: /^Remove "Harbour lights"/ }));
    await waitFor(() => expect(screen.queryByText("Harbour lights")).toBeNull());
    await user.click(screen.getByRole("button", { name: /^Remove "Gone"/ }));
    await waitFor(() => expect(screen.queryByText("Gone")).toBeNull());

    // 止めていた応答は消した 2 件と、その後ろの鍵を返す。
    server.holdList = null;
    act(() => release());

    await waitFor(() =>
      expect(
        within(historyList())
          .getAllByRole("button", { name: /^Remove/ })
          .map((button) => button.getAttribute("aria-label")),
      ).toEqual([
        expect.stringMatching(/^Remove "Unknown video"/),
        expect.stringMatching(/^Remove "Kyoto"/),
      ]),
    );
    expect(server.listUrls).toEqual([
      "/api/watch-history?limit=60",
      "/api/watch-history?limit=60",
      "/api/watch-history?cursor=3&limit=60",
    ]);
  });

  it("読み直しに失敗したら、その間に断った続きの読み込みのために最後の行を見張り直す", async () => {
    const user = userEvent.setup();
    server.pageSize = 3;
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });
    server.deleteStatus = 404;
    let release = () => {};
    server.holdList = new Promise((resolve) => {
      release = resolve;
    });

    await user.click(screen.getByRole("button", { name: /^Remove "Gone"/ }));
    await waitFor(() => expect(server.listUrls).toHaveLength(2));
    // 読み直しの間に最後の行が見えても、続きは読まない。
    const armed = observers;
    act(() => {
      intersect?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    });
    expect(server.listUrls).toHaveLength(2);

    server.listStatus = 500;
    server.holdList = null;
    act(() => release());
    await waitFor(() =>
      expect(toasts()).toEqual(["Something went wrong on the server."]),
    );

    // 見張りを付け直すので、最後の行が見えたままなら続きを読む。
    expect(observers).toBeGreaterThan(armed);
    server.listStatus = 200;
    act(() => {
      intersect?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    });
    await waitFor(() =>
      expect(server.listUrls.at(-1)).toBe("/api/watch-history?cursor=2&limit=60"),
    );
  });

  it("読み直しに失敗したとき、その間に行が尽きていれば続きを読む", async () => {
    const user = userEvent.setup();
    server.pageSize = 2;
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });
    server.deleteStatus = 404;
    let release = () => {};
    server.holdList = new Promise((resolve) => {
      release = resolve;
    });

    await user.click(screen.getByRole("button", { name: /^Remove "Gone"/ }));
    await waitFor(() => expect(server.listUrls).toHaveLength(2));
    // 読み直しの間に見えている行を全部消す。続きの読み込みは断られる。
    server.deleteStatus = 204;
    await user.click(screen.getByRole("button", { name: /^Remove "Harbour lights"/ }));
    await user.click(screen.getByRole("button", { name: /^Remove "Gone"/ }));
    await waitFor(() => expect(server.deletes).toHaveLength(3));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /^Remove/ })).toBeNull(),
    );
    expect(server.listUrls).toHaveLength(2);

    server.listStatus = 500;
    server.holdList = null;
    act(() => release());
    await waitFor(() =>
      expect(toasts()).toEqual(["Something went wrong on the server."]),
    );

    // 見張る行が無くても、残っている古い件を読む。
    server.listStatus = 200;
    await waitFor(() =>
      expect(server.listUrls.at(-1)).toBe("/api/watch-history?cursor=3&limit=60"),
    );
  });

  it("削除に失敗したら行を残してトーストを出す", async () => {
    const user = userEvent.setup();
    server.deleteStatus = 500;
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });

    await user.click(screen.getByRole("button", { name: /^Remove "Gone"/ }));

    await waitFor(() =>
      expect(toasts()).toEqual(["Something went wrong on the server."]),
    );
    expect(screen.getByText("Gone")).toBeDefined();
    const button = screen.getByRole("button", { name: /^Remove "Gone"/ });
    expect((button as HTMLButtonElement).disabled).toBe(false);
    expect(server.listUrls).toHaveLength(1);
  });

  it("全件の削除は先に確かめ、取り消せば何も変えず、確定すれば空にする", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });

    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Clear history…" }));
    let dialog = await screen.findByRole("alertdialog", { name: "Clear watch history?" });
    expect(
      within(dialog).getByText(
        "Every entry is removed. Playback positions and watched marks stay as they are.",
      ),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(server.deletes).toEqual([]);
    expect(
      within(historyList()).getAllByRole("button", { name: /^Remove/ }),
    ).toHaveLength(4);

    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Clear history…" }));
    dialog = await screen.findByRole("alertdialog", { name: "Clear watch history?" });
    await user.click(within(dialog).getByRole("button", { name: "Clear" }));

    expect(
      await screen.findByRole("heading", { level: 2, name: "No watch history" }),
    ).toBeDefined();
    expect(server.deletes).toEqual(["/api/watch-history"]);
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("heading", { level: 1, name: "History" }),
      ),
    );
  });

  it("全件の削除に失敗したら窓に理由を出し、一覧を変えない", async () => {
    const user = userEvent.setup();
    server.clearStatus = 500;
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });

    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Clear history…" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "Clear watch history?",
    });
    await user.click(within(dialog).getByRole("button", { name: "Clear" }));

    expect(
      await within(dialog).findByText(
        "Couldn't clear the history: Something went wrong on the server.",
      ),
    ).toBeDefined();
    expect(screen.getByRole("alertdialog")).toBeDefined();
    // 窓の後ろの一覧は読み上げから隠れているので、行を数える。
    expect(document.querySelectorAll('[data-slot="timeline-item"]')).toHaveLength(4);
  });

  it("履歴が無ければ空の表示にし、More を出さない", async () => {
    server.entries = [];
    renderPage();
    expect(
      await screen.findByRole("heading", { level: 2, name: "No watch history" }),
    ).toBeDefined();
    expect(
      screen.getByText("Videos you play are listed here, newest first."),
    ).toBeDefined();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
  });

  it("状態の切り替えと検索語は、最初のページにも続きのページにも watch と query を送る", async () => {
    const user = userEvent.setup();
    const updatedAt = at(27, 22, 0);
    server.pageSize = 1;
    server.entries = [
      {
        id: 3,
        playedAt: at(27, 21, 30),
        title: "Harbour lights",
        video: video(10, {
          title: "Harbour lights",
          progress: { positionMs: 30_000, completed: false, updatedAt },
        }),
      },
      {
        id: 2,
        playedAt: at(27, 15, 0),
        title: "Kyoto",
        video: video(11, {
          title: "Kyoto",
          progress: { positionMs: 60_000, completed: true, updatedAt },
        }),
      },
      {
        id: 1,
        playedAt: at(26, 9, 0),
        title: "Harbour at dawn",
        video: video(12, {
          title: "Harbour at dawn",
          progress: { positionMs: 10_000, completed: false, updatedAt },
        }),
      },
    ];
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });

    const filter = screen.getByRole("radiogroup", { name: "Watch status" });
    expect(
      within(filter).getByRole("radio", { name: "All" }).getAttribute("aria-checked"),
    ).toBe("true");
    await user.click(within(filter).getByRole("radio", { name: "In progress" }));
    await waitFor(() => expect(search).toBe("?watch=inProgress"));
    await user.type(screen.getByRole("searchbox", { name: "Search titles" }), "harbour");
    await waitFor(() => expect(search).toBe("?watch=inProgress&q=harbour"));
    await waitFor(() =>
      expect(
        within(historyList()).getAllByRole("link", { name: /^Harbour lights,/ }),
      ).toHaveLength(1),
    );

    server.listUrls = [];
    act(() => {
      intersect?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    });
    expect(
      await within(historyList()).findByRole("link", { name: /^Harbour at dawn,/ }),
    ).toBeDefined();
    const next = new URLSearchParams(server.listUrls.at(-1)?.split("?")[1]);
    expect(next.get("cursor")).toBe("3");
    expect(next.get("watch")).toBe("inProgress");
    expect(next.get("query")).toBe("harbour");
    expect(next.has("date")).toBe(false);
    expect(within(historyList()).queryByText("Kyoto")).toBeNull();
  });

  it("日付の一覧は今の watch と query で日を求め、日と月を並べ、切り替えや検索が変わると求め直す", async () => {
    const user = userEvent.setup();
    server.dates = ["2026-09-27", "2026-09-26", "2026-08-20", "2026-08-03", "2025-12-31"];
    renderPage("/history?watch=watched&q=kyoto");

    const jump = await screen.findByRole("radiogroup", { name: "Jump to date" });
    await waitFor(() =>
      expect(
        within(jump)
          .getAllByRole("radio")
          .map((item) => item.textContent),
      ).toEqual(["Sun, Sep 27", "Sat, Sep 26", "August", "December 2025"]),
    );
    expect(server.datesUrls).toHaveLength(1);
    const first = new URLSearchParams(server.datesUrls[0]?.split("?")[1]);
    expect(first.get("watch")).toBe("watched");
    expect(first.get("query")).toBe("kyoto");
    expect(first.get("tz")).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone);

    // 日を選んでも日付は読み直さない。
    await user.click(within(jump).getByRole("radio", { name: "August" }));
    await waitFor(() => expect(search).toBe("?watch=watched&q=kyoto&date=2026-08"));
    expect(server.datesUrls).toHaveLength(1);

    await user.click(
      within(screen.getByRole("radiogroup", { name: "Watch status" })).getByRole(
        "radio",
        {
          name: "All",
        },
      ),
    );
    await waitFor(() => expect(server.datesUrls).toHaveLength(2));
    const second = new URLSearchParams(server.datesUrls[1]?.split("?")[1]);
    expect(second.has("watch")).toBe(false);
    expect(second.get("query")).toBe("kyoto");
    // 状態を変えても日は残る。
    expect(search).toBe("?q=kyoto&date=2026-08");
  });

  it("日を選ぶと date と tz を送って返ったページを出し、最初の項目か押した項目で date を外す", async () => {
    const user = userEvent.setup();
    renderPage();
    const jump = await screen.findByRole("radiogroup", { name: "Jump to date" });
    await screen.findByRole("list", { name: "Watch history" });

    server.listUrls = [];
    await user.click(within(jump).getByRole("radio", { name: "Sat, Sep 26" }));
    await waitFor(() => expect(search).toBe("?date=2026-09-26"));
    await waitFor(() =>
      expect(within(historyList()).queryByText("Harbour lights")).toBeNull(),
    );
    expect(within(historyList()).getByText("Kyoto")).toBeDefined();
    const request = new URLSearchParams(server.listUrls.at(-1)?.split("?")[1]);
    expect(request.get("date")).toBe("2026-09-26");
    expect(request.get("tz")).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone);
    expect(
      within(jump)
        .getByRole("radio", { name: "Sat, Sep 26" })
        .getAttribute("aria-checked"),
    ).toBe("true");
    expect(window.scrollTo).toHaveBeenCalled();

    // 押している項目をもう一度押すと date を外す。
    await user.click(within(jump).getByRole("radio", { name: "Sat, Sep 26" }));
    await waitFor(() => expect(search).toBe(""));
    expect(await within(historyList()).findByText("Harbour lights")).toBeDefined();

    // 最初の項目は一覧の先頭なので date を書かない。
    await user.click(within(jump).getByRole("radio", { name: "Sun, Sep 27" }));
    expect(search).toBe("");
  });

  it("date の無い一覧で最初の項目を選ぶと、URL も一覧も変えずに先頭へ戻る", async () => {
    const user = userEvent.setup();
    renderPage();
    const jump = await screen.findByRole("radiogroup", { name: "Jump to date" });
    await screen.findByRole("list", { name: "Watch history" });

    server.listUrls = [];
    vi.mocked(window.scrollTo).mockClear();
    await user.click(within(jump).getByRole("radio", { name: "Sun, Sep 27" }));
    expect(window.scrollTo).toHaveBeenCalledWith({ top: 0, behavior: "auto" });
    expect(search).toBe("");
    expect(server.listUrls).toEqual([]);
  });

  it("絞り込みや検索で 1 件も返らなければ、空の表示ではなく該当なしの表示を出し、Clear filters で外す", async () => {
    const user = userEvent.setup();
    server.dates = [];
    renderPage("/history?watch=watched&q=zzz&date=2026-09-27");

    expect(
      await screen.findByRole("heading", {
        level: 2,
        name: "No history matches these conditions",
      }),
    ).toBeDefined();
    expect(screen.queryByRole("heading", { name: "No watch history" })).toBeNull();
    expect(
      screen.getByText("Try a different search or change the filters."),
    ).toBeDefined();
    // 見出しの行は何が外したかを見せたまま。
    expect(
      within(screen.getByRole("radiogroup", { name: "Watch status" }))
        .getByRole("radio", { name: "Watched" })
        .getAttribute("aria-checked"),
    ).toBe("true");
    expect(
      (screen.getByRole("searchbox", { name: "Search titles" }) as HTMLInputElement)
        .value,
    ).toBe("zzz");
    expect(screen.getByText("No dates to jump to")).toBeDefined();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Clear history…" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    await waitFor(() => expect(search).toBe(""));
    expect(await screen.findByRole("list", { name: "Watch history" })).toBeDefined();
  });

  it("検索語を消すと絞り込みの無い一覧に戻る", async () => {
    const user = userEvent.setup();
    renderPage("/history?q=kyoto");
    await screen.findByRole("list", { name: "Watch history" });
    expect(within(historyList()).queryByText("Harbour lights")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Clear search" }));
    await waitFor(() => expect(search).toBe(""));
    expect(await within(historyList()).findByText("Harbour lights")).toBeDefined();
    const request = new URLSearchParams(server.listUrls.at(-1)?.split("?")[1]);
    expect(request.has("query")).toBe(false);
  });

  it("行のリンクと操作の state.from は今のクエリ文字列を持つ", async () => {
    const user = userEvent.setup();
    renderPage("/history?watch=all&q=kyoto&date=2026-09");
    await screen.findByRole("list", { name: "Watch history" });

    await user.click(within(historyList()).getByRole("link", { name: /^Kyoto,/ }));
    expect(await screen.findByText("Video page")).toBeDefined();
    expect(location).toEqual({
      pathname: "/videos/11",
      state: { from: "/history?watch=all&q=kyoto&date=2026-09" },
    });
  });

  it("日付の読み込みに失敗したら、横の欄に理由と Retry を出し、一覧はそのまま出す", async () => {
    const user = userEvent.setup();
    server.datesStatus = 500;
    renderPage();
    await screen.findByRole("list", { name: "Watch history" });
    expect(await screen.findByText("Couldn't load the dates")).toBeDefined();

    server.datesStatus = 200;
    const jump = screen.getByRole("region", { name: "Jump to date" });
    await user.click(within(jump).getByRole("button", { name: "Retry" }));
    expect(await within(jump).findByRole("radio", { name: "Sun, Sep 27" })).toBeDefined();
    // 横の欄の最後の「Clear history…」から確認の窓を開く。
    await user.click(within(jump).getByRole("button", { name: "Clear history…" }));
    expect(
      await screen.findByRole("alertdialog", { name: "Clear watch history?" }),
    ).toBeDefined();
  });

  it("全件を消すと条件を URL から外し、日付の一覧を空にする", async () => {
    const user = userEvent.setup();
    renderPage("/history?q=o");
    await screen.findByRole("list", { name: "Watch history" });
    const jump = screen.getByRole("region", { name: "Jump to date" });
    await user.click(await within(jump).findByRole("button", { name: "Clear history…" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "Clear watch history?",
    });
    await user.click(within(dialog).getByRole("button", { name: "Clear" }));

    expect(
      await screen.findByRole("heading", { level: 2, name: "No watch history" }),
    ).toBeDefined();
    expect(search).toBe("");
    expect(screen.getByText("No dates to jump to")).toBeDefined();
  });

  it("疑似ロケールではカタログの文言だけを描く", async () => {
    enablePseudoLocale();
    renderPage();
    await screen.findByRole("list", { name: /Watch history/ });
    expectCatalogTextOnly(document.body, ["Harbour lights", "Gone", "Kyoto", "1:00"]);
  });
});
