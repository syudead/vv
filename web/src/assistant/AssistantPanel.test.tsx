import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  AssistantProposal,
  AssistantTurn,
  AssistantTurnRequest,
} from "../api/assistant";
import * as events from "../api/serverEvents";
import { type Audience, AudienceProvider } from "../auth/audience";
import VideoHeader from "../player/VideoHeader";
import TopBar from "../shell/TopBar";
import { SidebarProvider } from "../ui/shadcn/sidebar";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { oneStepProposal } from "../testing/assistantProposal";
import { ToastProvider } from "../ui/Toast";
import AssistantPanel from "./AssistantPanel";
import { AssistantProvider, useAssistantScreen } from "./AssistantProvider";
import type { CardProposal } from "./proposal";
import type { AssistantScreen } from "./screenContext";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

interface Server {
  readiness: () => Response;
  turn: (body: AssistantTurnRequest) => Promise<Response> | Response;
  turns: AssistantTurnRequest[];
  /** GET /api/assistant/turns/{id}. */
  turnRead: () => Response;
  execute: () => Promise<Response> | Response;
  executes: string[];
  proposal: () => Response;
  list: () => Response;
  /** Delivers an assistant event to every subscriber, as /api/events would. */
  emit: () => void;
}

function installServer(): Server {
  const server: Server = {
    readiness: () => json({ ready: true }),
    turn: () => finished({ answer: "ok" }),
    turns: [],
    turnRead: () => json({ code: "not_found", message: "not found" }, 404),
    execute: () => json({ code: "not_found", message: "not found" }, 404),
    executes: [],
    proposal: () => json({ code: "not_found", message: "not found" }, 404),
    list: () => json({ items: [] }),
    emit: () => undefined,
  };
  beforeEach(() => {
    server.readiness = () => json({ ready: true });
    server.turns = [];
    server.turnRead = () => json({ code: "not_found", message: "not found" }, 404);
    server.executes = [];
    server.list = () => json({ items: [] });
    server.execute = () => json({ code: "not_found", message: "not found" }, 404);
    server.proposal = () => json({ code: "not_found", message: "not found" }, 404);
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
      if (url === "/api/assistant/readiness") return Promise.resolve(server.readiness());
      if (url === "/api/assistant/proposals?active=true")
        return Promise.resolve(server.list());
      if (url.endsWith("/execute") && init?.method === "POST") {
        server.executes.push(`${url} ${String(init.body)}`);
        return Promise.resolve(server.execute());
      }
      if (url.startsWith("/api/assistant/proposals/"))
        return Promise.resolve(server.proposal());
      if (url.startsWith("/api/assistant/turns/"))
        return Promise.resolve(server.turnRead());
      if (url === "/api/assistant/turns" && init?.method === "POST") {
        const body = JSON.parse(String(init.body)) as AssistantTurnRequest;
        server.turns.push(body);
        return Promise.resolve(server.turn(body));
      }
      return Promise.resolve(json({ code: "not_found", message: "not found" }, 404));
    });
    vi.stubGlobal(
      "matchMedia",
      (query: string) =>
        ({
          matches: false,
          media: query,
          addEventListener: () => undefined,
          removeEventListener: () => undefined,
        }) as unknown as MediaQueryList,
    );
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
  return server;
}

function turn(extra: Partial<AssistantTurn> = {}): AssistantTurn {
  return {
    id: 1,
    status: "done",
    request: "",
    startedAt: "2026-10-30T00:00:00Z",
    progress: [],
    judged: false,
    ...extra,
  };
}

/** The 202 a turn the server finished before answering would carry. */
function finished(extra: Partial<AssistantTurn> = {}): Response {
  return json(turn(extra), 202);
}

function proposal(extra: Partial<CardProposal> = {}): AssistantProposal {
  return oneStepProposal({
    id: 1,
    status: "proposed",
    request: "これに旅行タグを付けて",
    operation: "updateVideoTags.add",
    irreversible: false,
    targets: {
      kind: "videos",
      count: 3,
      labels: [
        { kind: "video", id: 1, name: "a" },
        { kind: "video", id: 2, name: "b" },
        { kind: "video", id: 3, name: "c" },
      ],
    },
    change: [{ argument: "tagNames", labels: [{ kind: "tag", id: 4, name: "旅行" }] }],
    created: [],
    counts: { total: 3, applied: 0, failed: 0 },
    ...extra,
  });
}

function LocationProbe() {
  const location = useLocation();
  return (
    <output aria-label="location">{`${location.pathname}${location.search}`}</output>
  );
}

function Screen({ screen: value }: { screen: AssistantScreen }) {
  useAssistantScreen(value);
  return null;
}

function renderApp({
  audience = "owner",
  path = "/",
  screen: value = {},
}: { audience?: Audience; path?: string; screen?: AssistantScreen } = {}) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AudienceProvider audience={audience}>
        <ToastProvider>
          <TooltipProvider>
            <AssistantProvider>
              <SidebarProvider open={false} onOpenChange={() => undefined}>
                <TopBar />
                <Screen screen={value} />
                <AssistantPanel />
              </SidebarProvider>
              <LocationProbe />
            </AssistantProvider>
          </TooltipProvider>
        </ToastProvider>
      </AudienceProvider>
    </MemoryRouter>,
  );
}

const trigger = () => screen.getByRole("button", { name: /^Assistant/ });
const field = () => screen.getByRole("textbox", { name: "Request" });
const panel = () => screen.getByRole("complementary", { name: "Assistant" });
const location = () => screen.getByRole("status", { name: "location" });

async function ask(user: ReturnType<typeof userEvent.setup>, text: string) {
  await user.click(trigger());
  await user.type(field(), `${text}{Enter}`);
}

describe("AssistantPanel", () => {
  const server = installServer();

  it("ゲストにはトップバーにも動画ページの帯にもトリガーを出さない", () => {
    render(
      <MemoryRouter>
        <AudienceProvider audience="guest">
          <TooltipProvider>
            <AssistantProvider>
              <SidebarProvider open={false} onOpenChange={() => undefined}>
                <TopBar />
                <VideoHeader folder={undefined} onClose={() => undefined} />
              </SidebarProvider>
            </AssistantProvider>
          </TooltipProvider>
        </AudienceProvider>
      </MemoryRouter>,
    );

    expect(screen.queryByRole("button", { name: /Assistant/ })).toBeNull();
    expect(screen.getByRole("button", { name: "Close" })).toBeDefined();
  });

  it("準備ができていなければ、入力欄の代わりに Connections への案内を出す", async () => {
    server.readiness = () => json({ ready: false, reason: "generation_model_unset" });
    const user = userEvent.setup();
    renderApp();

    await user.click(trigger());

    const alert = await within(panel()).findByRole("alert");
    expect(alert.textContent).toContain("Choose a model for the assistant");
    expect(
      within(alert).getByRole("link", { name: "Open Connections" }).getAttribute("href"),
    ).toBe("/settings/connections");
    expect(screen.queryByRole("textbox", { name: "Request" })).toBeNull();
  });

  it("filter の directive でライブラリの URL を条件に変え、lg 未満ではパネルを閉じる", async () => {
    server.turn = () =>
      finished({
        directive: {
          kind: "filter",
          criteria: {
            rule: { tags: [4], minDurationMs: 1_800_000, watchState: "unwatched" },
            labels: [{ kind: "tag", id: 4, name: "旅行" }],
          },
        },
      });
    const user = userEvent.setup();
    renderApp({ path: "/folders" });

    await ask(user, "30分以上の未視聴の旅行");

    await waitFor(() =>
      expect(location().textContent).toBe(
        "/?tag=4&minDurationMs=1800000&watchState=unwatched",
      ),
    );
    expect(screen.queryByRole("complementary", { name: "Assistant" })).toBeNull();

    await user.click(trigger());
    const chips = within(panel()).getByRole("list", { name: "Tags in the filter" });
    expect(
      within(chips)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["旅行", "30 min or longer", "Unwatched"]);
  });

  it("navigate の directive で開くページへ移る", async () => {
    server.turn = () =>
      finished({
        directive: {
          kind: "navigate",
          destination: { kind: "settings", settingsPage: "scan" },
        },
      });
    const user = userEvent.setup();
    renderApp();

    await ask(user, "スキャンの設定を開いて");

    await waitFor(() => expect(location().textContent).toBe("/settings/scan"));
  });

  it("聞き返しで候補を選ぶと、元の依頼を選んだ ID を pin して送り直す", async () => {
    server.turn = () =>
      finished({
        id: 5,
        status: "clarify",
        clarify: {
          name: "コスプレ",
          kind: "tag",
          candidates: [
            { kind: "tag", id: 11, name: "コスプレ衣装", count: 12 },
            { kind: "tag", id: 12, name: "コスプレ衣裳", count: 3 },
          ],
        },
      });
    const user = userEvent.setup();
    renderApp();
    await ask(user, "コスプレのタグを消して");
    const choice = await within(panel()).findByRole("button", { name: /^コスプレ衣裳/ });

    server.turn = () => finished({ id: 6, answer: "ok" });
    await user.click(choice);

    await waitFor(() => expect(server.turns).toHaveLength(2));
    expect(server.turns[1]).toMatchObject({
      text: "コスプレのタグを消して",
      context: { pinned: [{ kind: "tag", id: 12, name: "コスプレ衣裳" }] },
      previousTurnId: 5,
    });
  });

  it("running の turn を assistant イベントで読み直し、終わったら答えの文を出す", async () => {
    server.turn = () => json(turn({ id: 9, status: "running" }), 202);
    const reads = vi.fn(() => json(turn({ id: 9, status: "running" })));
    server.turnRead = reads;
    const user = userEvent.setup();
    renderApp();

    await ask(user, "お気に入りは何本？");
    // The first read follows the 202 at once; the event makes the second.
    await waitFor(() => expect(reads).toHaveBeenCalledTimes(1));
    act(() => server.emit());
    await waitFor(() => expect(reads).toHaveBeenCalledTimes(2));
    expect(within(panel()).getByRole("status").textContent).toContain("Asking the model");

    reads.mockImplementation(() =>
      json(turn({ id: 9, answer: "お気に入りは\n12 本です。" })),
    );
    act(() => server.emit());

    expect(await within(panel()).findByText(/12 本です。/)).toBeDefined();
    expect(within(panel()).queryByRole("status")).toBeNull();
  });

  it("止まった turn はその理由を出す", async () => {
    server.turn = () => finished({ status: "stopped", stop: { reason: "step_limit" } });
    const user = userEvent.setup();
    renderApp();

    await ask(user, "全部まとめて整理して");

    expect(
      await within(panel()).findByText(
        "Stopped: the request needed more steps than one request may take. Nothing was changed. Try a narrower request.",
      ),
    ).toBeDefined();
  });

  it("turn の途中でパネルを閉じて開き直すと、届いた答えが出ている", async () => {
    let answer: (response: Response) => void = () => undefined;
    server.turn = () => new Promise<Response>((resolve) => (answer = resolve));
    const user = userEvent.setup();
    renderApp();
    await ask(user, "何かして");
    await user.click(within(panel()).getByRole("button", { name: "Close" }));
    expect(trigger().getAttribute("aria-label")).toBe("Assistant · working");

    await act(async () =>
      answer(
        finished({
          status: "stopped",
          stop: {
            reason: "step_limit",
            excluded: { operation: "unknown", reason: "", page: "/" },
          },
        }),
      ),
    );
    await user.click(trigger());

    expect(within(panel()).getByText("何かして")).toBeDefined();
    expect(within(panel()).getByText("The assistant can’t do that.")).toBeDefined();
  });

  it("turn の途中でスレッドを消すと、遅れて届いた結果でページを変えず、次の依頼を送れる", async () => {
    let answer: (response: Response) => void = () => undefined;
    server.turn = () => new Promise<Response>((resolve) => (answer = resolve));
    const user = userEvent.setup();
    renderApp({ path: "/folders" });
    await ask(user, "スキャンの設定を開いて");

    await user.click(within(panel()).getByRole("button", { name: "Clear thread" }));
    await act(async () =>
      answer(
        finished({
          directive: {
            kind: "navigate",
            destination: { kind: "settings", settingsPage: "scan" },
          },
        }),
      ),
    );

    expect(location().textContent).toBe("/folders");
    server.turn = () => finished({ answer: "できました" });
    await user.type(field(), "何かして{Enter}");
    expect(await within(panel()).findByText("できました")).toBeDefined();
    expect(server.turns).toHaveLength(2);
  });

  it("IME の確定の Enter と 500 文字を超える依頼は送らない", async () => {
    const user = userEvent.setup();
    renderApp();
    await user.click(trigger());
    await user.type(field(), "タグ");
    fireEvent.keyDown(field(), { key: "Enter", isComposing: true });

    fireEvent.change(field(), { target: { value: "あ".repeat(501) } });
    fireEvent.keyDown(field(), { key: "Enter" });

    expect(within(panel()).getByText("501 / 500")).toBeDefined();
    expect(
      within(panel()).getByRole("button", { name: "Send" }).getAttribute("aria-disabled"),
    ).toBe("true");
    expect(server.turns).toEqual([]);
  });

  it("選んでいる動画とコレクションを依頼の context に入れる", async () => {
    const user = userEvent.setup();
    renderApp({
      screen: { videoIds: [3, 5], groupIds: [7], smartFolderIds: [9], count: 3 },
    });
    await user.click(trigger());
    expect(within(panel()).getByText("Talking about 3 selected items")).toBeDefined();

    await user.type(field(), "これにタグを付けて{Enter}");

    await waitFor(() => expect(server.turns).toHaveLength(1));
    expect(server.turns[0]?.context).toEqual({
      route: { kind: "library" },
      selectedVideoIds: [3, 5],
      selectedCollectionIds: { groups: [7], smartFolders: [9] },
    });
  });

  it("Execute を 2 回押しても要求は 1 回で、カードの状態は 409 の本文から来る", async () => {
    server.turn = () => finished({ status: "done", proposal: proposal() });
    let answer: (response: Response) => void = () => undefined;
    server.execute = () => new Promise<Response>((resolve) => (answer = resolve));
    const user = userEvent.setup();
    renderApp();
    await ask(user, "これに旅行タグを付けて");

    await user.click(await within(panel()).findByRole("button", { name: "Execute" }));
    const pressed = within(panel()).getByRole("button", { name: "Executing…" });
    expect(pressed.getAttribute("aria-disabled")).toBe("true");
    await user.click(pressed);
    await act(async () =>
      answer(
        json(
          {
            code: "proposal_not_pending",
            message: "The proposal is no longer waiting for approval.",
            proposal: proposal({
              status: "running",
              counts: { total: 3, applied: 0, failed: 0 },
            }),
          },
          409,
        ),
      ),
    );

    expect(server.executes).toEqual([`/api/assistant/proposals/1/execute {"steps":[0]}`]);
    expect(within(panel()).getByText("Running · 0 of 3")).toBeDefined();
    expect(within(panel()).queryByRole("button", { name: /Execute/ })).toBeNull();
  });

  it("Dismiss は何も送らずにカードを畳む", async () => {
    server.turn = () => finished({ status: "done", proposal: proposal() });
    const user = userEvent.setup();
    renderApp();
    await ask(user, "これに旅行タグを付けて");

    await user.click(await within(panel()).findByRole("button", { name: "Dismiss" }));

    expect(within(panel()).getByText("Dismissed · nothing changed")).toBeDefined();
    expect(server.executes).toEqual([]);
  });

  it("追いの指示の答えで前のカードを置き換え、Execute は 1 つだけになる", async () => {
    server.turn = () => finished({ status: "done", proposal: proposal() });
    const user = userEvent.setup();
    renderApp();
    await ask(user, "これに旅行タグを付けて");
    await within(panel()).findByRole("button", { name: "Execute" });

    server.turn = () =>
      finished({
        id: 2,
        proposal: proposal({
          id: 2,
          targets: { kind: "videos", count: 2, labels: [] },
          counts: { total: 2, applied: 0, failed: 0 },
        }),
      });
    await user.type(field(), "長いのは除いて{Enter}");

    expect(
      await within(panel()).findByText("Replaced by the next request"),
    ).toBeDefined();
    expect(within(panel()).getByText("2 videos")).toBeDefined();
    expect(within(panel()).queryByText("3 videos")).toBeNull();
    expect(within(panel()).getAllByRole("button", { name: "Execute" })).toHaveLength(1);
    expect(server.turns[1]?.previousTurnId).toBe(1);
  });

  it("assistant イベントで提案を読み直し、件数と結果の行を出す", async () => {
    const big = { kind: "videos", count: 1200, labels: [] };
    server.turn = () =>
      finished({
        proposal: proposal({
          targets: big,
          counts: { total: 1200, applied: 0, failed: 0 },
        }),
      });
    server.execute = () =>
      json(
        proposal({
          status: "approved",
          targets: big,
          counts: { total: 1200, applied: 0, failed: 0 },
          activityId: 77,
        }),
        202,
      );
    const user = userEvent.setup();
    renderApp();
    await ask(user, "これに旅行タグを付けて");
    await user.click(await within(panel()).findByRole("button", { name: "Execute" }));
    await within(panel()).findByText("Running · 0 of 1,200");

    server.proposal = () =>
      json(
        proposal({
          status: "running",
          targets: big,
          counts: { total: 1200, applied: 500, failed: 0 },
          activityId: 77,
        }),
      );
    act(() => server.emit());
    expect(await within(panel()).findByText("Running · 500 of 1,200")).toBeDefined();

    server.proposal = () =>
      json(
        proposal({
          status: "done",
          targets: big,
          counts: { total: 1200, applied: 1200, failed: 0 },
          activityId: 77,
        }),
      );
    act(() => server.emit());
    expect(await within(panel()).findByText("Done · 1,200 videos changed")).toBeDefined();
    expect(
      within(panel())
        .getByRole("link", { name: "Open in Activity" })
        .getAttribute("href"),
    ).toBe("/activity/77");
  });

  it("Execute の答えより先に実行が終わっても、カードは読み直した結果を出す", async () => {
    server.turn = () => finished({ status: "done", proposal: proposal() });
    let answer: (response: Response) => void = () => undefined;
    server.execute = () => new Promise<Response>((resolve) => (answer = resolve));
    const user = userEvent.setup();
    renderApp();
    await ask(user, "これに旅行タグを付けて");
    await user.click(await within(panel()).findByRole("button", { name: "Execute" }));

    server.proposal = () =>
      json(proposal({ status: "done", counts: { total: 3, applied: 3, failed: 0 } }));
    act(() => server.emit());
    await act(async () => {
      answer(json(proposal({ status: "approved" }), 202));
      await Promise.resolve();
    });

    expect(await within(panel()).findByText("Done · 3 videos changed")).toBeDefined();
  });

  it("stale の結果はもう一度頼む行を出す", async () => {
    server.turn = () => finished({ status: "done", proposal: proposal() });
    server.execute = () => json(proposal({ status: "approved" }), 202);
    const user = userEvent.setup();
    renderApp();
    await ask(user, "これに旅行タグを付けて");
    await user.click(await within(panel()).findByRole("button", { name: "Execute" }));
    await within(panel()).findByText("Running · 0 of 3");

    server.proposal = () => json(proposal({ status: "stale" }));
    act(() => server.emit());

    expect(
      await within(panel()).findByText(
        "Something changed since this was proposed, so nothing was done. Ask again.",
      ),
    ).toBeDefined();
  });

  it("開いたときに実行中と最近終わった実行を Recent runs に出す", async () => {
    server.list = () =>
      json({
        items: [
          proposal({
            id: 5,
            status: "running",
            counts: { total: 3, applied: 0, failed: 0 },
          }),
          proposal({
            id: 4,
            operation: "startScan",
            status: "done",
            targets: { kind: "", count: 0, labels: [] },
            change: [],
            counts: { total: 1, applied: 1, failed: 0 },
          }),
        ],
      });
    const user = userEvent.setup();
    renderApp();

    await user.click(trigger());

    const runs = await within(panel()).findByRole("region", { name: "Recent runs" });
    expect(
      within(runs)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["Add tagsRunning · 0 of 3", "Start a scanDone"]);
  });
});
