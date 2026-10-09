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
};

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url.startsWith("/api/watch-history?") && method === "GET") {
      server.listUrls.push(url);
      const query = new URLSearchParams(url.slice(url.indexOf("?") + 1));
      const start = Number(query.get("cursor") ?? "0");
      const end = start + server.pageSize;
      return Promise.resolve(
        json({
          items: server.entries.slice(start, end),
          ...(end < server.entries.length ? { nextCursor: String(end) } : {}),
        }),
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
let location: { pathname: string; state: unknown } | undefined;

function LocationProbe() {
  const current = useLocation();
  location = { pathname: current.pathname, state: current.state };
  return null;
}

function renderPage() {
  render(
    <MemoryRouter initialEntries={["/history"]}>
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

function toasts(): string[] {
  return Array.from(document.querySelectorAll("[data-sonner-toast]")).map(
    (toast) => toast.textContent ?? "",
  );
}

beforeEach(() => {
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
  intersect = undefined;
  location = undefined;
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersect = callback;
      }
      observe() {}
      disconnect() {}
    },
  );
  install();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("HistoryPage（specs/043-watch-history/ui-design.md「History screen」）", () => {
  it("件を新しい順に、日の見出しの下へ時刻と題名つきで並べる", async () => {
    renderPage();
    expect(screen.getByRole("heading", { level: 1, name: "History" })).toBeDefined();
    expect(document.title).toBe("History");

    await screen.findByRole("list", { name: "Watch history" });
    const headings = within(historyList())
      .getAllByRole("heading", { level: 2 })
      .map((heading) => heading.textContent);
    expect(headings).toEqual(["Sep 27, 2026", "Sep 26, 2026"]);

    const removeNames = within(historyList())
      .getAllByRole("button", { name: /^Remove/ })
      .map((button) => button.getAttribute("aria-label"));
    expect(removeNames).toEqual([
      expect.stringMatching(
        /^Remove "Harbour lights" played Sep 27, 2026 at 9:30\sPM from history$/,
      ),
      expect.stringMatching(
        /^Remove "Gone" played Sep 27, 2026 at 3:04\sPM from history$/,
      ),
      expect.stringMatching(
        /^Remove "Unknown video" played Sep 26, 2026 at 9:00\sAM from history$/,
      ),
      expect.stringMatching(
        /^Remove "Kyoto" played Sep 26, 2026 at 8:00\sAM from history$/,
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
      /^Harbour lights, 1:00, played Sep 27, 2026 at 9:30\sPM$/,
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
      "/api/watch-history?cursor=3&limit=60",
    ]);
    expect(
      within(historyList())
        .getAllByRole("heading", { level: 2 })
        .map((heading) => heading.textContent),
    ).toEqual(["Sep 27, 2026", "Sep 26, 2026"]);
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
    expect(document.querySelectorAll('[data-slot="grouped-list-item"]')).toHaveLength(4);
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

  it("疑似ロケールではカタログの文言だけを描く", async () => {
    enablePseudoLocale();
    renderPage();
    await screen.findByRole("list", { name: /Watch history/ });
    expectCatalogTextOnly(document.body, ["Harbour lights", "Gone", "Kyoto", "1:00"]);
  });
});
