import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AssistantOutcome, AssistantTurnRequest } from "../api/assistant";
import { type Audience, AudienceProvider } from "../auth/audience";
import VideoHeader from "../player/VideoHeader";
import TopBar from "../shell/TopBar";
import { SidebarProvider } from "../ui/shadcn/sidebar";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ToastProvider } from "../ui/Toast";
import AssistantPanel from "./AssistantPanel";
import { AssistantProvider, useAssistantScreen } from "./AssistantProvider";
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
}

function installServer(): Server {
  const server: Server = {
    readiness: () => json({ ready: true }),
    turn: () => json({ outcome: { kind: "unreadable" } }),
    turns: [],
  };
  beforeEach(() => {
    server.readiness = () => json({ ready: true });
    server.turns = [];
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/assistant/readiness") return Promise.resolve(server.readiness());
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
  afterEach(() => vi.unstubAllGlobals());
  return server;
}

function outcome(value: AssistantOutcome): Response {
  return json({ outcome: value });
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

  it("filter の結果でライブラリの URL を条件に変え、lg 未満ではパネルを閉じる", async () => {
    server.turn = () =>
      outcome({
        kind: "filter",
        criteria: {
          rule: { tags: [4], minDurationMs: 1_800_000, watchState: "unwatched" },
          labels: [{ kind: "tag", id: 4, name: "旅行" }],
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

  it("navigate の結果で開くページへ移る", async () => {
    server.turn = () =>
      outcome({
        kind: "navigate",
        destination: { kind: "settings", settingsPage: "scan" },
      });
    const user = userEvent.setup();
    renderApp();

    await ask(user, "スキャンの設定を開いて");

    await waitFor(() => expect(location().textContent).toBe("/settings/scan"));
  });

  it("聞き返しで候補を選ぶと、元の依頼を選んだ ID を pin して送り直す", async () => {
    server.turn = () =>
      outcome({
        kind: "clarify",
        about: "value",
        candidates: [
          { kind: "tag", id: 11, name: "コスプレ衣装", count: 12 },
          { kind: "tag", id: 12, name: "コスプレ衣裳", count: 3 },
        ],
      });
    const user = userEvent.setup();
    renderApp();
    await ask(user, "コスプレのタグを消して");
    const choice = await within(panel()).findByRole("button", { name: /^コスプレ衣裳/ });

    server.turn = () => outcome({ kind: "unreadable" });
    await user.click(choice);

    await waitFor(() => expect(server.turns).toHaveLength(2));
    expect(server.turns[1]).toMatchObject({
      text: "コスプレのタグを消して",
      context: { pinned: [{ kind: "tag", id: 12, name: "コスプレ衣裳" }] },
      previous: { kind: "clarify" },
    });
  });

  it("count の答えは数とライブラリへのリンクを出す", async () => {
    server.turn = () =>
      outcome({
        kind: "answer",
        question: "count",
        total: 1234,
        link: { criteria: { rule: { favorite: true }, labels: [] } },
      });
    const user = userEvent.setup();
    renderApp();

    await ask(user, "お気に入りは何本？");

    expect(await within(panel()).findByText("1,234 videos")).toBeDefined();
    expect(
      within(panel())
        .getByRole("link", { name: "Show in the library" })
        .getAttribute("href"),
    ).toBe("/?fav=1");
  });

  it("turn の途中でパネルを閉じて開き直すと、届いた答えが出ている", async () => {
    let answer: (response: Response) => void = () => undefined;
    server.turn = () => new Promise<Response>((resolve) => (answer = resolve));
    const user = userEvent.setup();
    renderApp();
    await ask(user, "何かして");
    await user.click(within(panel()).getByRole("button", { name: "Close" }));
    expect(trigger().getAttribute("aria-label")).toBe("Assistant · working");

    await act(async () => answer(outcome({ kind: "unsupported" })));
    await user.click(trigger());

    expect(within(panel()).getByText("何かして")).toBeDefined();
    expect(within(panel()).getByText("The assistant can’t do that.")).toBeDefined();
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
});
