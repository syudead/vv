import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { json } from "../testing/groupPage";
import { ToastProvider } from "../ui/Toast";
import HomeSection from "./HomeSection";

// 設定の「Home」の節（docs/design-docs/home.md「Smart folder page and Settings」）。
// 「Start setup」は段階を
// `folders` に書いてオンボードを開く。

const server = {
  requests: [] as { method: string; path: string; body?: unknown }[],
};

beforeEach(() => {
  server.requests = [];
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
