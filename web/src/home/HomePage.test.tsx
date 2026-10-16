import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import type { Home, HomeShelf } from "../api/home";
import { type Audience, AudienceProvider } from "../auth/audience";
import { json, Probe, video } from "../testing/groupPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import HomePage from "./HomePage";

// ホームの画面（specs/057-home/ui-design.md「Home」）。棚のカードを 1 回押すと再生し
// （受け入れ条件 1）、検索欄で送るとライブラリで探す（受け入れ条件 2）。

const travel: HomeShelf = {
  key: "folder:3",
  kind: "folder",
  name: "Travel",
  folderId: 3,
};
const recent: HomeShelf = { key: "recent", kind: "recent", name: "" };
const unshelved: HomeShelf = { key: "unshelved", kind: "unshelved", name: "" };

const server = {
  home: { shelves: [] } as Home,
  items: new Map<string, { items: unknown[]; total: number }>(),
  shelfRequests: [] as string[][],
  failShelves: false,
};

beforeEach(() => {
  server.home = { shelves: [travel, recent, unshelved] };
  server.items = new Map([
    [
      travel.key,
      { items: [{ kind: "video", video: video(5, { title: "Harbour" }) }], total: 1 },
    ],
    [
      recent.key,
      { items: [{ kind: "video", video: video(6, { title: "Ridge" }) }], total: 1 },
    ],
    [unshelved.key, { items: [], total: 0 }],
  ]);
  server.shelfRequests = [];
  server.failShelves = false;
  installFakeEventSource();
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/home") return Promise.resolve(json(server.home));
      if (url.pathname === "/api/home/shelves") {
        const keys = (url.searchParams.get("keys") ?? "").split(",");
        server.shelfRequests.push(keys);
        if (server.failShelves) {
          return Promise.resolve(json({ code: "internal", message: "x" }, 500));
        }
        return Promise.resolve(
          json({
            shelves: keys.flatMap((key) => {
              const found = server.items.get(key);
              return found === undefined ? [] : [{ key, ...found }];
            }),
          }),
        );
      }
      throw new Error(`unexpected request: ${url.toString()}`);
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderHome(audience: Audience = "owner") {
  render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <Routes>
              <Route path="/" element={<HomePage />} />
              <Route path="/videos/:id" element={<Probe />} />
              <Route path="/library" element={<Probe />} />
            </Routes>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

describe("ホーム（specs/057-home/ui-design.md「Home」）", () => {
  it("棚のカードを 1 回押すと、フォルダの棚では ?smartFolder 付きの再生画面を autoplay で開く", async () => {
    const user = userEvent.setup();
    renderHome();

    await screen.findByRole("link", { name: "Harbour" });
    const shelf = screen
      .getByRole("heading", { level: 2, name: "Travel" })
      .closest("section") as HTMLElement;
    expect(within(shelf).getByRole("link", { name: "Travel" }).getAttribute("href")).toBe(
      "/smart-folders/3",
    );
    expect(
      within(shelf).getByRole("link", { name: "See all: Travel" }).getAttribute("href"),
    ).toBe("/smart-folders/3");

    await user.click(within(shelf).getByRole("link", { name: "Harbour" }));

    expect(
      await screen.findByText(
        'Opened /videos/5?smartFolder=3 {"from":"/","autoplay":true}',
      ),
    ).toBeTruthy();
  });

  it("画面に入る棚をまとめて 1 回で読み、項目の無い棚は描かない", async () => {
    renderHome();

    expect(
      await screen.findByRole("heading", { level: 2, name: "Recently added" }),
    ).toBeTruthy();
    expect(server.shelfRequests).toEqual([["folder:3", "recent", "unshelved"]]);
    expect(
      screen.queryByRole("heading", { level: 2, name: "Not on any shelf" }),
    ).toBeNull();
    expect(
      screen.getByRole("link", { name: "See all: Recently added" }).getAttribute("href"),
    ).toBe("/library");
  });

  it("検索欄で Enter を押すと /library?q= を開く", async () => {
    const user = userEvent.setup();
    renderHome();
    await screen.findByRole("heading", { level: 2, name: "Travel" });

    await user.type(
      screen.getByRole("searchbox", { name: "Search the library" }),
      "harbour{Enter}",
    );

    expect(await screen.findByText("Opened /library?q=harbour null")).toBeTruthy();
  });

  it("棚の項目が読めなければ見出しと Retry を出し、Retry でその棚を読み直す", async () => {
    const user = userEvent.setup();
    server.failShelves = true;
    renderHome();

    const retries = await screen.findAllByRole("button", { name: "Retry" });
    expect(screen.getByRole("heading", { level: 2, name: "Travel" })).toBeTruthy();
    server.failShelves = false;
    await user.click(retries[0] as HTMLElement);

    expect(await screen.findByRole("link", { name: "Harbour" })).toBeTruthy();
    expect(server.shelfRequests.at(-1)).toEqual(["folder:3"]);
  });

  it("home の知らせで一覧と読んだ棚を読み直す", async () => {
    renderHome();
    await screen.findByRole("link", { name: "Harbour" });

    server.home = { shelves: [{ ...travel, name: "Trips" }, recent, unshelved] };
    await emitServerEvent("home");

    expect(await screen.findByRole("heading", { level: 2, name: "Trips" })).toBeTruthy();
    expect(server.shelfRequests.at(-1)).toEqual(["folder:3", "recent", "unshelved"]);
  });

  it("動画が無ければ、所有者には Set up library でオンボードへの入口を出す", async () => {
    server.home = { shelves: [recent], empty: "noVideos" };
    renderHome();

    expect(
      await screen.findByRole("heading", { level: 2, name: "No videos yet" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Set up library" }).getAttribute("href"),
    ).toBe("/welcome");
  });

  it("動画が無ければ、閲覧者には空であることだけを出す", async () => {
    server.home = { shelves: [recent], empty: "noVideos" };
    renderHome("guest");

    expect(await screen.findByText("There's nothing to watch here yet.")).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Set up library" })).toBeNull();
  });
});
