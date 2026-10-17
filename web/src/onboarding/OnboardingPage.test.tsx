import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Interest, OnboardingStep } from "../api/onboarding";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { json } from "../testing/groupPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { OnboardingGate } from "./OnboardingGate";
import OnboardingPage from "./OnboardingPage";

// オンボードの画面とゲート（specs/057-home/ui-design.md「Onboarding」、research.md R-3・R-4）。
// 段階が done でない所有者はどの経路からも手順に着き（受け入れ条件 4）、閲覧者は送られない。
// どの手順も Skip で次の段階を書き、終えるとホームが開く（受け入れ条件 3）。

const interests: Interest[] = [
  { id: "anime", name: "Anime", synonym: "アニメ", onHome: false },
  { id: "travel", name: "Travel", synonym: "旅行", onHome: true },
  { id: "cooking", name: "Cooking", synonym: "料理", onHome: false },
];

const runningScan = {
  id: 9,
  origin: "manual",
  status: "running",
  videos: { total: 4, settled: 1 },
  issues: { failed: 0, substituted: 0, revision: 0 },
  state: "running",
};

const server = {
  step: "folders" as OnboardingStep,
  writes: [] as OnboardingStep[],
  failWrites: false,
  folders: [] as { id: number; path: string; version: number }[],
  picks: [] as string[][],
  scans: 0,
  autoTagging: false,
  requests: [] as string[],
};

function respond(url: URL, init: RequestInit | undefined): Response {
  const method = init?.method ?? "GET";
  server.requests.push(`${method} ${url.pathname}`);
  const body = typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : null;
  switch (`${method} ${url.pathname}`) {
    case "GET /api/onboarding":
      return json({ step: server.step });
    case "PUT /api/onboarding": {
      if (server.failWrites) return json({ code: "internal", message: "x" }, 500);
      const { step } = body as { step: OnboardingStep };
      server.writes.push(step);
      server.step = step;
      return json({ step });
    }
    case "GET /api/media-folders":
      return json(server.folders);
    case "GET /api/scans/current":
      return server.scans === 0
        ? json({ code: "not_found", message: "x" }, 404)
        : json(runningScan);
    case "POST /api/scans":
      server.scans += 1;
      return json(runningScan);
    case "GET /api/onboarding/interests":
      return json({ interests });
    case "POST /api/onboarding/interests": {
      const { ids } = body as { ids: string[] };
      server.picks.push(ids);
      return json({
        shelves: ids.map((id, index) => ({
          key: `folder:${String(index + 20)}`,
          kind: "folder",
          name: id,
        })),
        queued: 0,
      });
    }
    case "GET /api/settings/auto-tagging":
      return json({
        enabled: server.autoTagging,
        afterImport: false,
        threshold: 0.8,
        tentativeThreshold: 0.5,
        queue: { pending: 0, running: 0 },
      });
    default:
      throw new Error(`unexpected request: ${method} ${url.toString()}`);
  }
}

beforeEach(() => {
  server.step = "folders";
  server.writes = [];
  server.failWrites = false;
  server.folders = [{ id: 1, path: "/media/videos", version: 1 }];
  server.picks = [];
  server.scans = 0;
  server.autoTagging = false;
  server.requests = [];
  installFakeEventSource();
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) =>
      Promise.resolve(respond(new URL(String(input), "http://localhost"), init)),
    ),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function Probe() {
  const location = useLocation();
  return <p>{`Opened ${location.pathname}`}</p>;
}

function renderApp(path: string, audience: Audience = "owner") {
  render(
    <MemoryRouter initialEntries={[path]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <ScanProvider>
              <OnboardingGate>
                <Routes>
                  <Route path="/welcome" element={<OnboardingPage />} />
                  <Route path="*" element={<Probe />} />
                </Routes>
              </OnboardingGate>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function footerButton(name: string) {
  return screen.getByRole("button", { name });
}

describe("オンボードのゲート", () => {
  it.each(["/library", "/settings"])(
    "段階が scan の所有者が %s を開くと、スキャンの手順に着く",
    async (path) => {
      server.step = "scan";
      renderApp(path);

      expect(
        await screen.findByRole("heading", { level: 1, name: "Scan your library" }),
      ).toBeTruthy();
      expect(screen.queryByText(`Opened ${path}`)).toBeNull();
      const strip = screen.getByRole("navigation", { name: "Setup steps" });
      expect(within(strip).getByText("Scan, current step")).toBeTruthy();
      expect(within(strip).getByText("Media folders, done")).toBeTruthy();
    },
  );

  it("閲覧者は送らず、段階も読まない", async () => {
    server.step = "folders";
    renderApp("/library", "guest");

    expect(await screen.findByText("Opened /library")).toBeTruthy();
    expect(server.requests).not.toContain("GET /api/onboarding");
  });

  it("段階が done の所有者の /welcome はホームへ送る", async () => {
    server.step = "done";
    renderApp("/welcome");

    expect(await screen.findByText("Opened /")).toBeTruthy();
  });

  it("ほかの端末で段階が変わると、home の知らせでその手順へ移る", async () => {
    renderApp("/welcome");
    expect(
      await screen.findByRole("heading", { level: 1, name: "Where are your videos?" }),
    ).toBeTruthy();

    server.step = "interests";
    await emitServerEvent("home");

    expect(
      await screen.findByRole("heading", { level: 1, name: "What do you watch?" }),
    ).toBeTruthy();
  });
});

describe("オンボードの手順", () => {
  it("Skip は次の段階を書き、次の手順を出す", async () => {
    const user = userEvent.setup();
    renderApp("/welcome");
    await screen.findByRole("heading", { level: 1, name: "Where are your videos?" });

    await user.click(footerButton("Skip"));
    expect(
      await screen.findByRole("heading", { level: 1, name: "Scan your library" }),
    ).toBeTruthy();
    await user.click(footerButton("Skip"));
    expect(
      await screen.findByRole("heading", { level: 1, name: "What do you watch?" }),
    ).toBeTruthy();
    await user.click(footerButton("Skip"));

    expect(await screen.findByText("Opened /")).toBeTruthy();
    expect(server.writes).toEqual(["scan", "interests", "done"]);
    expect(server.picks).toEqual([]);
  });

  it("フォルダが無ければ Continue を押せず、足したフォルダの行を出す", async () => {
    server.folders = [];
    renderApp("/welcome");
    await screen.findByRole("button", { name: "Add folder" });

    expect(footerButton("Continue").hasAttribute("disabled")).toBe(true);
    expect(footerButton("Skip").hasAttribute("disabled")).toBe(false);
  });

  it("Scan library は取り込みを始めてから interests を書く", async () => {
    const user = userEvent.setup();
    server.step = "scan";
    renderApp("/welcome");
    await screen.findByRole("heading", { level: 1, name: "Scan your library" });

    await user.click(footerButton("Scan library"));

    expect(
      await screen.findByRole("heading", { level: 1, name: "What do you watch?" }),
    ).toBeTruthy();
    expect(server.scans).toBe(1);
    expect(server.writes).toEqual(["interests"]);
    expect(await screen.findByText("Scanning: 1 of 4")).toBeTruthy();
  });

  it("Finish は選んだ分野を足してから done を書き、ホームを開く", async () => {
    const user = userEvent.setup();
    server.step = "interests";
    renderApp("/welcome");

    const group = await screen.findByRole("toolbar", { name: "Interests" });
    expect(
      screen.getByText(
        "Auto-tagging is off, so these shelves fill in only as you tag videos. You can turn it on in Settings.",
      ),
    ).toBeTruthy();
    // ホームにもうある分野は押したまま押せない。
    const travel = within(group).getByRole("button", { name: "Travel, on your home" });
    expect(travel.getAttribute("aria-pressed")).toBe("true");
    expect(travel.hasAttribute("disabled")).toBe(true);

    await user.click(within(group).getByRole("button", { name: "Anime" }));
    await user.click(within(group).getByRole("button", { name: "Cooking" }));
    await user.click(footerButton("Finish"));

    expect(await screen.findByText("Opened /")).toBeTruthy();
    expect(server.picks).toEqual([["anime", "cooking"]]);
    expect(server.writes).toEqual(["done"]);
  });

  it("段階を書けなければ Alert を出して手順を残す", async () => {
    const user = userEvent.setup();
    server.failWrites = true;
    renderApp("/welcome");
    await screen.findByRole("heading", { level: 1, name: "Where are your videos?" });

    await user.click(footerButton("Skip"));

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/^Couldn't save your progress: /);
    expect(
      screen.getByRole("heading", { level: 1, name: "Where are your videos?" }),
    ).toBeTruthy();
    await waitFor(() =>
      expect(footerButton("Skip").hasAttribute("disabled")).toBe(false),
    );
  });
});
