import {
  act,
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  ActivityEntry,
  ActivityFeature,
  ActivityPage as Page,
} from "../api/activity";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { DEFAULT_ACTIVITY_CRITERIA } from "./activityCriteria";
import ActivityPage from "./ActivityPage";
import { useActivityLog } from "./useActivityLog";

// アクティビティの画面のページテスト。
// 偽のサーバーは契約どおりに絞り込み、counts は status 以外、features は feature 以外の
// 絞り込みに従う。

function entry(id: number, extra: Partial<ActivityEntry> = {}): ActivityEntry {
  return {
    id,
    kind: "import",
    feature: "import",
    status: "done",
    outcome: "completed",
    decider: "rule",
    subject: { kind: "video", items: [{ label: `file-${String(id)}.mp4`, videoId: id }] },
    candidate: false,
    retryRun: false,
    undoable: false,
    undone: false,
    createdAt: new Date(Date.now() - id * 60_000).toISOString(),
    updatedAt: new Date(Date.now() - id * 60_000).toISOString(),
    ...extra,
  };
}

const features: ActivityFeature[] = [
  "import",
  "scan",
  "duplicate_check",
  "auto_tagging",
  "work_info",
  "tags",
  "versions",
  "media_folders",
  "organize",
];

const server = {
  entries: [] as ActivityEntry[],
  requests: [] as URLSearchParams[],
  /** 置くと、一覧の要求は応答を作った後でこれを待ってから返す（読んだ後に記録が変わる場合）。 */
  gate: null as Promise<void> | null,
  /** やり直しと取り消しの要求（パスと本文）。 */
  operations: [] as { path: string; body: unknown }[],
  /** 置くと、やり直しと取り消しはこの誤りの code で 409 を返す。 */
  conflict: null as string | null,
  /** 取り消しが返す undo の記録の outcome。 */
  undoOutcome: "completed" as ActivityEntry["outcome"],
  /** 置くと、やり直しと取り消しの応答はこれを待ってから返す。 */
  operationGate: null as Promise<void> | null,
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** operate は POST /api/activity/{id}/retry と /undo に契約どおりに答える。 */
async function operate(path: string, init: RequestInit | undefined): Promise<Response> {
  const body: unknown = typeof init?.body === "string" ? JSON.parse(init.body) : null;
  server.operations.push({ path, body });
  if (server.operationGate !== null) await server.operationGate;
  if (server.conflict !== null)
    return json({ code: server.conflict, message: "conflict" }, 409);
  if (path.endsWith("/retry")) return new Response(null, { status: 202 });
  return json(
    entry(100, {
      kind: "undo",
      decider: "owner",
      outcome: server.undoOutcome,
      undoes: { kind: "version_judgement", outcome: "same" },
    }),
  );
}

function matches(item: ActivityEntry, params: URLSearchParams, skip: string): boolean {
  const status = params.get("status");
  const chosen = params.getAll("feature");
  const decider = params.get("decider");
  const q = params.get("q")?.toLowerCase();
  if (skip !== "status" && status !== null && item.status !== status) return false;
  if (skip !== "feature" && chosen.length > 0 && !chosen.includes(item.feature))
    return false;
  if (decider !== null && item.decider !== decider) return false;
  if (
    q !== undefined &&
    !item.subject.items.some((part) => part.label.toLowerCase().includes(q))
  )
    return false;
  return true;
}

function respond(params: URLSearchParams): Page {
  const all = [...server.entries].sort((a, b) => b.id - a.id);
  const listed = all.filter((item) => matches(item, params, ""));
  const limit = Number(params.get("limit") ?? "50");
  const cursor = params.get("cursor");
  const start = cursor === null ? 0 : Number(cursor);
  const items = listed.slice(start, start + limit);
  const byStatus = all.filter((item) => matches(item, params, "status"));
  const byFeature = all.filter((item) => matches(item, params, "feature"));
  return {
    items,
    ...(start + limit < listed.length ? { nextCursor: String(start + limit) } : {}),
    counts: {
      running: byStatus.filter((item) => item.status === "running").length,
      attention: byStatus.filter((item) => item.status === "attention").length,
      done: byStatus.filter((item) => item.status === "done").length,
    },
    features: Object.fromEntries(
      features.map((feature) => [
        feature,
        byFeature.filter((item) => item.feature === feature).length,
      ]),
    ) as Page["features"],
  };
}

function install() {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), "http://localhost");
      if (init?.method === "POST" && url.pathname.startsWith("/api/activity/"))
        return operate(url.pathname, init);
      if (url.pathname === "/api/activity") {
        server.requests.push(url.searchParams);
        const body = JSON.stringify(respond(url.searchParams));
        const held = url.searchParams.get("limit") === "1" ? null : server.gate;
        const reply = () =>
          new Response(body, {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        return held === null ? Promise.resolve(reply()) : held.then(reply);
      }
      throw new Error(`unexpected request: ${url.pathname}`);
    }),
  );
}

let search = "";

function LocationProbe() {
  search = useLocation().search;
  return null;
}

function renderPage(path = "/activity") {
  render(
    <MemoryRouter initialEntries={[path]}>
      <TooltipProvider>
        <ToastProvider>
          <Routes>
            <Route path="/activity" element={<ActivityPage />} />
          </Routes>
          <LocationProbe />
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function cards(): HTMLElement[] {
  return within(screen.getByRole("list", { name: /Activity log/ })).getAllByRole(
    "listitem",
  );
}

function filterRequests(): URLSearchParams[] {
  return server.requests.filter((params) => params.get("limit") !== "1");
}

beforeEach(() => {
  server.entries = [
    entry(1, {
      status: "running",
      reason: "processing",
      progress: { stage: "preview", done: 3, total: 5 },
    }),
    entry(2, {
      status: "attention",
      reason: "failed",
      stage: "thumbnail",
      detail: "broken",
    }),
    entry(3, { kind: "tag_attach", feature: "tags", decider: "owner", detail: "cat" }),
    entry(4, {
      kind: "auto_tag",
      feature: "auto_tagging",
      decider: "ai",
      subject: { kind: "video", items: [{ label: "gone.mp4" }] },
    }),
  ];
  server.requests = [];
  server.gate = null;
  server.operations = [];
  server.conflict = null;
  server.undoOutcome = "completed";
  server.operationGate = null;
  search = "";
  // jsdom は scrollTo を持たない。絞り込みを変えると先頭へ戻す。
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  installFakeEventSource();
  install();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("ActivityPage", () => {
  it("lists every entry newest first with its chip, result and time", async () => {
    renderPage();
    expect(screen.getByRole("heading", { level: 1, name: "Activity" })).toBeDefined();
    await waitFor(() => expect(cards()).toHaveLength(4));
    // 新しい順（id の降順）。
    expect(
      cards().map(
        (card) => card.querySelector("[data-slot=card-list-title]")?.textContent,
      ),
    ).toEqual(["gone.mp4", "file-3.mp4", "file-2.mp4", "file-1.mp4"]);
    const [, , failed, running] = cards();
    expect(within(running!).getByText("3/5")).toBeDefined();
    expect(running!.dataset.tone).toBe("primary");
    expect(within(failed!).getByText("Failed at thumbnail: broken")).toBeDefined();
    expect(failed!.dataset.tone).toBe("destructive");
    expect(document.title).toBe("Activity");
  });

  it("narrows the cards by status, keeps it in the URL and shows the response's counts", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(4));
    await user.click(screen.getByRole("button", { name: /^Status/ }));
    const attention = await screen.findByRole("menuitemradio", {
      name: /Needs attention/,
    });
    expect(within(attention).getByText("1")).toBeDefined();
    expect(
      within(screen.getByRole("menuitemradio", { name: /Done/ })).getByText("2"),
    ).toBeDefined();
    expect(
      within(screen.getByRole("menuitemradio", { name: /Running/ })).getByText("1"),
    ).toBeDefined();
    await user.click(attention);
    await waitFor(() => expect(cards()).toHaveLength(1));
    expect(search).toBe("?status=attention");
    expect(screen.getByRole("button", { name: /^Status/ }).dataset.active).toBe("true");
  });

  it("narrows by feature, decider, period and search, each kept in the URL", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(4));

    await user.click(screen.getByRole("button", { name: /^Feature/ }));
    await user.click(await screen.findByRole("menuitemcheckbox", { name: /Tags/ }));
    await user.keyboard("{Escape}");
    await waitFor(() => expect(cards()).toHaveLength(1));
    expect(search).toBe("?feature=tags");

    await user.click(screen.getByRole("button", { name: /^Decided by/ }));
    await user.click(await screen.findByRole("menuitemradio", { name: "You" }));
    await waitFor(() => expect(search).toBe("?feature=tags&decider=owner"));
    expect(cards()).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: /^Period/ }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Today" }));
    await waitFor(() => expect(search).toBe("?feature=tags&decider=owner&period=today"));
    await waitFor(() => expect(filterRequests().at(-1)?.get("since")).toBeTruthy());

    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    await waitFor(() => expect(cards()).toHaveLength(4));
    expect(search).toBe("");

    await user.type(
      screen.getAllByRole("searchbox", { name: "Search file names" })[0]!,
      "gone",
    );
    await waitFor(() => expect(cards()).toHaveLength(1));
    expect(search).toBe("?q=gone");
  });

  it("reads the filters from the URL and offers to clear when nothing matches", async () => {
    const user = userEvent.setup();
    renderPage("/activity?status=running&feature=scan");
    expect(await screen.findByText("Nothing matches these filters")).toBeDefined();
    expect(filterRequests()[0]?.get("status")).toBe("running");
    expect(filterRequests()[0]?.getAll("feature")).toEqual(["scan"]);
    const empty = screen
      .getByText("Nothing matches these filters")
      .closest("[data-state]");
    await user.click(
      within(empty as HTMLElement).getByRole("button", { name: "Clear filters" }),
    );
    await waitFor(() => expect(cards()).toHaveLength(4));
  });

  it("counts the whole log in the footer whatever the filters", async () => {
    renderPage("/activity?status=done");
    await waitFor(() => expect(cards()).toHaveLength(2));
    const footer = await screen.findByRole("status", { name: "Activity totals" });
    expect(within(footer).getByText("1 running")).toBeDefined();
    expect(within(footer).getByText("1 need attention")).toBeDefined();
    expect(within(footer).getByText("2 done")).toBeDefined();
  });

  it("refetches the counts and the loaded cards on an activity notification", async () => {
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(4));
    server.entries = server.entries.map((item) =>
      item.id === 1
        ? {
            ...item,
            status: "done",
            reason: undefined,
            outcome: "completed",
            progress: undefined,
          }
        : item,
    );
    server.entries.push(entry(5, { status: "running", reason: "waiting" }));
    await emitServerEvent("activity");
    await waitFor(() => expect(cards()).toHaveLength(5));
    const footer = screen.getByRole("status", { name: "Activity totals" });
    await waitFor(() => expect(within(footer).getByText("3 done")).toBeDefined());
    const first = cards().find((card) => within(card).queryByText("file-1.mp4") !== null);
    expect(first?.dataset.tone).toBe("default");
  });

  it("refetches after the first page when a notification came while it loaded", async () => {
    let release = () => {};
    server.gate = new Promise((resolve) => {
      release = resolve;
    });
    renderPage();
    await waitFor(() => expect(filterRequests()).toHaveLength(1));
    // 最初のページを読んだ後、応答が届く前に記録が増える。
    server.entries.push(entry(5));
    await emitServerEvent("activity");
    server.gate = null;
    release();
    await waitFor(() => expect(cards()).toHaveLength(5));
  });

  it("starts the list from the top when the filters change", async () => {
    const scrollTo = vi.mocked(window.scrollTo);
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(4));
    expect(scrollTo).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: /^Status/ }));
    await user.click(await screen.findByRole("menuitemradio", { name: /Done/ }));
    await waitFor(() => expect(search).toBe("?status=done"));
    expect(scrollTo).toHaveBeenCalledWith({ top: 0, behavior: "auto" });
  });

  it("takes a search term up to the contract's 200 characters", async () => {
    const user = userEvent.setup();
    const long = "a".repeat(150) + "z".repeat(60);
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(4));
    await user.click(screen.getAllByRole("searchbox", { name: "Search file names" })[0]!);
    await user.paste(long);
    await waitFor(() =>
      expect(new URLSearchParams(search).get("q")).toBe(long.slice(0, 200)),
    );
  });

  it("offers no open action when the subject has no video", async () => {
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(4));
    const gone = cards().find((card) => within(card).queryByText("gone.mp4") !== null)!;
    expect(within(gone).queryByRole("link", { name: "Open" })).toBeNull();
    const kept = cards().find((card) => within(card).queryByText("file-3.mp4") !== null)!;
    expect(within(kept).getByRole("link", { name: "Open" }).getAttribute("href")).toBe(
      "/videos/3",
    );
  });

  it("draws only catalog text besides the file names", async () => {
    enablePseudoLocale();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(4));
    expectCatalogTextOnly(document.body, [
      "file-1.mp4",
      "file-2.mp4",
      "file-3.mp4",
      "gone.mp4",
      "broken",
      "cat",
    ]);
  });
});

describe("ActivityPage retry and undo", () => {
  beforeEach(() => {
    server.entries = [
      entry(1, {
        status: "attention",
        reason: "failed",
        stage: "thumbnail",
        retryRun: true,
      }),
      entry(2, {
        kind: "version_judgement",
        feature: "duplicate_check",
        decider: "ai",
        outcome: "same",
        probability: 0.9,
        subject: {
          kind: "pair",
          items: [
            { label: "a.mp4", videoId: 2 },
            { label: "b.mp4", videoId: 20 },
          ],
        },
        undoable: true,
      }),
      entry(3, {
        kind: "auto_tag",
        feature: "auto_tagging",
        decider: "ai",
        undoable: true,
        tags: [
          { id: 7, name: "cat", suggested: false, probability: 0.9, undone: false },
          { id: 8, name: "dog", suggested: true, probability: 0.6, undone: false },
          { id: 9, name: "fox", suggested: false, probability: 0.8, undone: true },
        ],
      }),
      entry(4, {
        kind: "version_judgement",
        feature: "duplicate_check",
        decider: "rule",
        outcome: "same",
        undone: true,
      }),
      entry(5),
    ];
  });

  function card(label: string): HTMLElement {
    return cards().find((item) => within(item).queryByText(label) !== null)!;
  }

  it("offers retry and undo only on the cards whose entry allows them", async () => {
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(5));
    const failed = card("file-1.mp4");
    expect(within(failed).getByRole("button", { name: "Retry" })).toBeDefined();
    expect(within(failed).queryByRole("button", { name: /^Undo/ })).toBeNull();
    const bundled = card("a.mp4");
    expect(within(bundled).getByRole("button", { name: "Undo" })).toBeDefined();
    expect(within(bundled).queryByRole("button", { name: "Retry" })).toBeNull();
    const tagged = card("file-3.mp4");
    expect(within(tagged).getByRole("button", { name: "Undo tags…" })).toBeDefined();
    expect(within(tagged).queryByRole("button", { name: "Undo" })).toBeNull();
    for (const label of ["file-4.mp4", "file-5.mp4"]) {
      expect(
        within(card(label)).queryByRole("button", { name: /^(Retry|Undo)/ }),
      ).toBeNull();
    }
  });

  it("retries without asking and refetches the cards", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(5));
    const before = filterRequests().length;
    await user.click(within(card("file-1.mp4")).getByRole("button", { name: "Retry" }));
    await waitFor(() =>
      expect(server.operations).toEqual([{ path: "/api/activity/1/retry", body: null }]),
    );
    await waitFor(() => expect(filterRequests().length).toBeGreaterThan(before));
  });

  it("disables every button of the card while its request is pending", async () => {
    let release = () => {};
    server.operationGate = new Promise((resolve) => {
      release = resolve;
    });
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(5));
    const bundled = card("a.mp4");
    const undo = within(bundled).getByRole("button", { name: "Undo" });
    await user.click(undo);
    await waitFor(() => expect(undo.getAttribute("aria-busy")).toBe("true"));
    expect((undo as HTMLButtonElement).disabled).toBe(true);
    expect(
      within(bundled).getByRole("link", { name: "Open" }).getAttribute("aria-disabled"),
    ).toBe("true");
    server.operationGate = null;
    release();
    await waitFor(() => expect((undo as HTMLButtonElement).disabled).toBe(false));
  });

  it("keeps the card's buttons disabled until the refetched card arrives", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(5));
    // 取り消しの後のサーバーの記録。読み直しの応答は gate が開くまで届かない。
    server.entries = server.entries.map((item) =>
      item.id === 2 ? { ...item, undoable: false, undone: true } : item,
    );
    let release = () => {};
    server.gate = new Promise((resolve) => {
      release = resolve;
    });
    const before = filterRequests().length;
    const undo = within(card("a.mp4")).getByRole("button", { name: "Undo" });
    await user.click(undo);
    expect(
      await screen.findByText("Undone. vv won't decide this again on its own"),
    ).toBeDefined();
    await waitFor(() => expect(filterRequests().length).toBeGreaterThan(before));
    expect((undo as HTMLButtonElement).disabled).toBe(true);
    await user.click(undo);
    expect(server.operations).toHaveLength(1);
    server.gate = null;
    release();
    await waitFor(() =>
      expect(within(card("a.mp4")).queryByRole("button", { name: "Undo" })).toBeNull(),
    );
  });

  it("undoes one tag of an auto-tagging card from its menu", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(5));
    await user.click(
      within(card("file-3.mp4")).getByRole("button", { name: "Undo tags…" }),
    );
    const items = await screen.findAllByRole("menuitem");
    // 取り消し済みの fox は並べない。
    expect(items.map((item) => item.textContent)).toEqual([
      "cat90%",
      "dog(suggested)60%",
    ]);
    await user.click(items[1]!);
    await waitFor(() =>
      expect(server.operations).toEqual([
        { path: "/api/activity/3/undo", body: { tagId: 8 } },
      ]),
    );
    expect(
      await screen.findByText("Undone. vv won't decide this again on its own"),
    ).toBeDefined();
  });

  it("says so when the owner had already changed the decision back", async () => {
    server.undoOutcome = "already_reverted";
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(5));
    await user.click(within(card("a.mp4")).getByRole("button", { name: "Undo" }));
    expect(
      await screen.findByText(
        "Already changed back. vv won't decide this again on its own",
      ),
    ).toBeDefined();
    expect(server.operations).toEqual([{ path: "/api/activity/2/undo", body: {} }]);
  });

  it("keeps the card and shows the reason when the request fails", async () => {
    server.conflict = "activity_not_retryable";
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(cards()).toHaveLength(5));
    await user.click(within(card("file-1.mp4")).getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByText("Couldn't retry: This can no longer be retried"),
    ).toBeDefined();
    expect(
      within(card("file-1.mp4")).getByRole("button", { name: "Retry" }),
    ).toBeDefined();
    server.conflict = "activity_not_undoable";
    await user.click(within(card("a.mp4")).getByRole("button", { name: "Undo" }));
    expect(
      await screen.findByText("Couldn't undo: This can no longer be undone"),
    ).toBeDefined();
  });
});

describe("useActivityLog", () => {
  it("reads the next page with the period of the first page", async () => {
    server.entries = Array.from({ length: 60 }, (_, index) => entry(index + 1));
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 9, 16, 23, 59, 59));
    const { result } = renderHook(() =>
      useActivityLog({ ...DEFAULT_ACTIVITY_CRITERIA, period: "today" }),
    );
    await waitFor(() => expect(result.current.state.kind).toBe("ready"));
    // 次のページは日をまたいでから読む。
    vi.setSystemTime(new Date(2026, 9, 17, 0, 0, 1));
    act(() => result.current.loadMore());
    await waitFor(() => expect(filterRequests()).toHaveLength(2));
    const [first, next] = filterRequests();
    expect(next?.get("cursor")).toBeTruthy();
    expect(next?.get("since")).toBe(first?.get("since"));
    expect(first?.get("since")).toBe(new Date(2026, 9, 16).toISOString());
  });
});
