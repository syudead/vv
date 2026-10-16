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

// アクティビティの画面（specs/055-activity/ui-design.md「Activity page」）のページテスト。
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
};

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
    vi.fn<typeof fetch>((input) => {
      const url = new URL(String(input), "http://localhost");
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
