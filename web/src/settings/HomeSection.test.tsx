import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import type { HomeOrganizer } from "../api/home";
import { json } from "../testing/groupPage";
import { ToastProvider } from "../ui/Toast";
import HomeSection from "./HomeSection";

// 設定の「Home」の節（docs/design-docs/home.md「Smart folder page and Settings」）。
// 「Reorganize」は組み直しを待ちに入れ（research.md R-9）、「Start setup」は段階を
// `folders` に書いてオンボードを開く。

const server = {
  organizer: { running: false } as HomeOrganizer,
  requests: [] as { method: string; path: string; body?: unknown }[],
  /** 置くと POST /api/home/organize はこの約束が解けるまで答えない。 */
  holdOrganize: undefined as Promise<void> | undefined,
};

beforeEach(() => {
  server.organizer = { running: false };
  server.requests = [];
  server.holdOrganize = undefined;
  installFakeEventSource();
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const path = String(input);
      const method = init?.method ?? "GET";
      server.requests.push({
        method,
        path,
        body: init?.body === undefined ? undefined : JSON.parse(String(init.body)),
      });
      if (path === "/api/home" && method === "GET") {
        return Promise.resolve(json({ shelves: [], organizer: server.organizer }));
      }
      if (path === "/api/home/organize" && method === "POST") {
        server.organizer = { running: false, pending: "rebuild" };
        const accepted = server.organizer;
        const hold = server.holdOrganize ?? Promise.resolve();
        return hold.then(() => json(accepted, 202));
      }
      if (path === "/api/onboarding" && method === "PUT") {
        return Promise.resolve(json({ step: "folders" }));
      }
      throw new Error(`unexpected request: ${method} ${path}`);
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderSection() {
  render(
    <MemoryRouter initialEntries={["/settings"]}>
      <ToastProvider>
        <Routes>
          <Route path="/settings" element={<HomeSection />} />
          <Route path="/welcome" element={<p>Onboarding opened</p>} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("設定の Home の節", () => {
  it("Reorganize は POST /api/home/organize を呼び、待ちの間は押せない", async () => {
    const user = userEvent.setup();
    server.organizer = {
      running: false,
      last: { run: "extend", outcome: "done", finishedAt: 0, added: 2, removed: 0 },
    };
    renderSection();
    const button = await screen.findByRole("button", { name: "Reorganize" });
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
    expect(screen.getByText(/^Last organized .*: 2 added, 0 removed\.$/)).toBeTruthy();

    await user.click(button);

    expect(server.requests).toContainEqual({
      method: "POST",
      path: "/api/home/organize",
      body: undefined,
    });
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(true));

    // 実行が終わって `home` の知らせが来ると、読み直して押せるようにする。
    server.organizer = {
      running: false,
      last: { run: "rebuild", outcome: "unchanged", finishedAt: 0, added: 0, removed: 0 },
    };
    await emitServerEvent("home");
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
    expect(screen.getByText(/^Last organized .*: nothing changed\.$/)).toBeTruthy();
  });

  it("応答より先に実行の終わりの知らせが来ても、待ちのまま残さない", async () => {
    const user = userEvent.setup();
    renderSection();
    const button = await screen.findByRole("button", { name: "Reorganize" });
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
    let release = () => {};
    server.holdOrganize = new Promise((resolve) => {
      release = resolve;
    });

    await user.click(button);
    // 待ちに入った組み直しが応答の前に終わり、`home` の知らせが先に届く。
    server.organizer = {
      running: false,
      last: { run: "rebuild", outcome: "unchanged", finishedAt: 0, added: 0, removed: 0 },
    };
    await emitServerEvent("home");
    release();

    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
    expect(screen.getByText(/^Last organized .*: nothing changed\.$/)).toBeTruthy();
  });

  it("Start setup は段階を folders にしてオンボードを開く", async () => {
    const user = userEvent.setup();
    renderSection();

    await user.click(await screen.findByRole("button", { name: "Start setup" }));

    expect(await screen.findByText("Onboarding opened")).toBeTruthy();
    expect(server.requests).toContainEqual({
      method: "PUT",
      path: "/api/onboarding",
      body: { step: "folders" },
    });
  });
});
