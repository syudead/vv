import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  AssistantProposal,
  AssistantReviewCandidate,
  AssistantReviewRun,
} from "../api/assistant";
import * as events from "../api/serverEvents";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ToastProvider } from "../ui/Toast";
import { AssistantProvider } from "./AssistantProvider";
import ReviewPage from "./ReviewPage";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function run(extra: Partial<AssistantReviewRun> = {}): AssistantReviewRun {
  return {
    id: 7,
    kind: "tags",
    status: "done",
    total: 3,
    judged: 3,
    listed: 3,
    startedAt: "2026-10-25T10:00:00Z",
    ...extra,
  };
}

function tagPair(
  id: number,
  probability: number,
  [a, b]: [string, string],
  state: AssistantReviewCandidate["state"] = "listed",
): AssistantReviewCandidate {
  return {
    id,
    probability,
    state,
    members: [
      { kind: "tag", id: id * 10, name: a, count: 4 },
      { kind: "tag", id: id * 10 + 1, name: b, count: 9 },
    ],
  };
}

function mergeProposal(
  id: number,
  status: AssistantProposal["status"],
): AssistantProposal {
  return {
    id,
    status,
    request: "タグを整理して",
    operation: "mergeTag",
    irreversible: true,
    targets: { kind: "tag", count: 1, labels: [{ kind: "tag", id: 1, name: "selfie" }] },
    change: [],
    created: [],
    impact: { videos: 4 },
    counts: { total: 1, applied: status === "done" ? 1 : 0, failed: 0 },
  };
}

interface Server {
  run: AssistantReviewRun;
  items: AssistantReviewCandidate[];
  adopt: (ids: number[]) => Response;
  reject: (ids: number[]) => Response;
  proposals: Map<number, AssistantProposal>;
  posted: { url: string; ids: number[] }[];
  emit: () => void;
}

function installServer(): Server {
  const server: Server = {
    run: run(),
    items: [],
    adopt: () => json({ proposals: [], skippedIds: [] }, 202),
    reject: (ids) => json({ rejectedIds: ids, skippedIds: [] }),
    proposals: new Map(),
    posted: [],
    emit: () => undefined,
  };
  beforeEach(() => {
    server.run = run();
    server.items = [];
    server.posted = [];
    server.proposals = new Map();
    server.reject = (ids) => {
      server.items = server.items.map((item) =>
        ids.includes(item.id) ? { ...item, state: "rejected" } : item,
      );
      return json({ rejectedIds: ids, skippedIds: [] });
    };
    const subscribers = new Set<events.ServerEventHandlers>();
    server.emit = () => {
      for (const handlers of subscribers) handlers.assistant?.();
    };
    vi.spyOn(events, "subscribeServerEvents").mockImplementation((handlers) => {
      subscribers.add(handlers);
      return () => subscribers.delete(handlers);
    });
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.startsWith("/api/assistant/review-runs/7/candidates/")) {
        const { ids } = JSON.parse(String(init?.body)) as { ids: number[] };
        server.posted.push({ url, ids });
        const answer = url.endsWith("/adopt") ? server.adopt(ids) : server.reject(ids);
        return Promise.resolve(answer);
      }
      if (url.startsWith("/api/assistant/review-runs/7")) {
        return Promise.resolve(json({ run: server.run, items: server.items }));
      }
      if (url.startsWith("/api/assistant/review-runs/")) {
        return Promise.resolve(json({ code: "not_found", message: "not found" }, 404));
      }
      const proposal = /\/api\/assistant\/proposals\/(\d+)$/.exec(url);
      if (proposal !== null) {
        return Promise.resolve(json(server.proposals.get(Number(proposal[1]))));
      }
      return Promise.resolve(json({ code: "not_found", message: "not found" }, 404));
    });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
  return server;
}

function renderPage(id = 7) {
  return render(
    <MemoryRouter initialEntries={[`/assistant/reviews/${String(id)}`]}>
      <ToastProvider>
        <TooltipProvider>
          <AssistantProvider>
            <Routes>
              <Route path="/assistant/reviews/:id" element={<ReviewPage />} />
            </Routes>
          </AssistantProvider>
        </TooltipProvider>
      </ToastProvider>
    </MemoryRouter>,
  );
}

const table = () => screen.getByRole("table", { name: "Review candidates" });

/** The first name of each body row that shows the text, in table order. */
function rowsWith(text: string): string[] {
  return within(table())
    .getAllByRole("row")
    .slice(1)
    .filter((row) => within(row).queryAllByText(text).length > 0)
    .map((row) => within(row).getAllByRole("checkbox")[0]!.getAttribute("aria-label")!)
    .map((label) => label.replace(/^Select (\S+) and .*$/, "$1"));
}

describe("ReviewPage", () => {
  const server = installServer();

  it("実行中の進み具合は assistant イベントで読み直した run に従う", async () => {
    server.run = run({ status: "running", total: 40, judged: 5, listed: 1 });
    server.items = [tagPair(1, 0.9, ["selfie", "selfy"])];
    renderPage();

    expect(await screen.findByText("Judged 5 of 40 · 1 listed")).toBeDefined();
    expect(
      screen.getByRole("progressbar", { name: "Progress of the review" }),
    ).toBeDefined();

    server.run = run({ status: "running", total: 40, judged: 30, listed: 2 });
    server.items = [
      tagPair(2, 0.95, ["cat", "cats"]),
      tagPair(1, 0.9, ["selfie", "selfy"]),
    ];
    act(() => server.emit());

    expect(await screen.findByText("Judged 30 of 40 · 2 listed")).toBeDefined();
    expect(within(table()).getAllByRole("row")).toHaveLength(3);

    server.run = run({ status: "done", total: 40, judged: 40, listed: 2 });
    act(() => server.emit());

    expect(await screen.findByText("2 of 40 candidates listed")).toBeDefined();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("2 つの組の採用はその ID を送り、行に Adopted と結果の実行を出す", async () => {
    server.items = [
      tagPair(1, 0.97, ["selfie", "selfy"]),
      tagPair(2, 0.8, ["cat", "cats"]),
      tagPair(3, 0.6, ["dog", "dogs"]),
    ];
    server.adopt = (ids) => {
      server.items = server.items.map((item) =>
        ids.includes(item.id) ? { ...item, state: "adopted" } : item,
      );
      return json(
        {
          proposals: [mergeProposal(11, "approved"), mergeProposal(12, "approved")],
          skippedIds: [],
        },
        202,
      );
    };
    const user = userEvent.setup();
    renderPage();

    await user.click(
      await screen.findByRole("checkbox", { name: "Select selfie and selfy" }),
    );
    await user.click(screen.getByRole("checkbox", { name: "Select dog and dogs" }));
    await user.click(screen.getByRole("button", { name: "Adopt selected" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Merge 2 tag pairs?" });
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(server.posted).toEqual([
      { url: "/api/assistant/review-runs/7/candidates/adopt", ids: [1, 3] },
    ]);
    await waitFor(() => expect(rowsWith("Adopted")).toEqual(["selfie", "dog"]));
    expect(rowsWith("Running…")).toEqual(["selfie", "dog"]);

    // The merge removes a member, so the server stops sending the two sets; the rows stay with their runs.
    server.items = [tagPair(2, 0.8, ["cat", "cats"])];
    server.proposals.set(11, mergeProposal(11, "done"));
    server.proposals.set(12, mergeProposal(12, "done"));
    act(() => server.emit());

    await waitFor(() => expect(rowsWith("Done")).toEqual(["selfie", "dog"]));
    expect(rowsWith("Adopted")).toEqual(["selfie", "dog"]);
    expect(rowsWith("Running…")).toEqual([]);
    expect(screen.getByText("3 of 3 candidates listed · 2 adopted")).toBeDefined();
  });

  it("works の採用はダイアログを開かずにすぐ送る", async () => {
    server.run = run({ kind: "works" });
    server.items = [
      {
        id: 4,
        probability: 0.9,
        state: "listed",
        members: [
          { kind: "video", id: 1, name: "Trip 1", count: 0 },
          { kind: "video", id: 2, name: "Trip 2", count: 0 },
        ],
      },
    ];
    server.adopt = () => json({ proposals: [], skippedIds: [4] }, 202);
    const user = userEvent.setup();
    renderPage();

    expect(
      await screen.findByRole("heading", { name: "Review: parts of one work" }),
    ).toBeDefined();
    await user.click(screen.getByRole("checkbox", { name: "Select Trip" }));
    await user.click(screen.getByRole("button", { name: "Adopt selected" }));

    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(server.posted).toEqual([
      { url: "/api/assistant/review-runs/7/candidates/adopt", ids: [4] },
    ]);
    expect(
      await screen.findByText("1 candidate was skipped because a tag or person changed"),
    ).toBeDefined();
  });

  it("却下した組は一覧から外れ、トーストを出す", async () => {
    server.items = [
      tagPair(1, 0.97, ["selfie", "selfy"]),
      tagPair(2, 0.8, ["cat", "cats"]),
    ];
    const user = userEvent.setup();
    renderPage();

    await user.click(
      await screen.findByRole("checkbox", { name: "Select cat and cats" }),
    );
    await user.click(screen.getByRole("button", { name: "Reject selected" }));

    expect(server.posted).toEqual([
      { url: "/api/assistant/review-runs/7/candidates/reject", ids: [2] },
    ]);
    await waitFor(() =>
      expect(
        within(table()).queryByRole("checkbox", { name: "Select cat and cats" }),
      ).toBeNull(),
    );
    expect(
      within(table()).getByRole("checkbox", { name: "Select selfie and selfy" }),
    ).toBeDefined();
    expect(
      await screen.findByText("Rejected 1 candidate. It won’t be listed again."),
    ).toBeDefined();
  });

  it("見つからない run は ErrorState を出す", async () => {
    renderPage(8);

    expect(await screen.findByText("This review isn’t here any more.")).toBeDefined();
  });
});
