import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import type { PeerMediaFolder } from "../api/peer";
import { json, renderWithScan } from "../testing/autoImportServer";
import { ToastProvider } from "../ui/Toast";
import MediaFoldersSection from "./MediaFoldersSection";

function folder(id: number, path: string, enabled = true): PeerMediaFolder {
  return { id, path, enabled, refreshedAt: "2026-11-01T00:00:00Z" };
}

describe("MediaFoldersSection link sections", () => {
  const switched: { url: string; body: unknown }[] = [];
  let betaFolders: PeerMediaFolder[];
  beforeEach(() => {
    switched.length = 0;
    betaFolders = [folder(1, "/beta/movies")];
    installFakeEventSource();
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === "PUT") {
        const body = JSON.parse(String(init.body)) as { enabled: boolean };
        switched.push({ url, body });
        return Promise.resolve(json(folder(4, "/alpha/clips", body.enabled)));
      }
      if (url === "/api/scans/current")
        return Promise.resolve(json({ code: "not_found", message: "none" }, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([]));
      // Answered out of slot order, so the page has to order the sections itself.
      if (url === "/api/peer")
        return Promise.resolve(
          json({
            links: [
              { slot: 2, nodeId: "n-beta", name: "beta", state: "connected" },
              {
                slot: 1,
                nodeId: "n-alpha",
                name: "alpha",
                state: "unreachable",
                reason: "network",
              },
            ],
          }),
        );
      if (url === "/api/peer/links/n-alpha/media-folders")
        return Promise.resolve(
          json({
            state: "unreachable",
            reason: "network",
            folders: [folder(4, "/alpha/clips")],
          }),
        );
      if (url === "/api/peer/links/n-beta/media-folders")
        return Promise.resolve(json({ state: "connected", folders: betaFolders }));
      return Promise.reject(new Error(`unexpected request: ${url}`));
    });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function renderPage() {
    renderWithScan(
      <ToastProvider>
        <MediaFoldersSection />
      </ToastProvider>,
    );
  }

  it("shows one section per link in slot order after the local section", async () => {
    renderPage();

    await screen.findByText("/beta/movies");
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((heading) => heading.textContent),
    ).toEqual(["Media folders", "alpha", "beta"]);
  });

  it("keeps an unreachable link's rows with its state, and its switch saves on this node", async () => {
    renderPage();
    const alpha = await screen.findByRole("region", { name: "alpha" });
    const toggle = await within(alpha).findByRole("switch", {
      name: "Show videos from /alpha/clips on alpha",
    });
    expect(within(alpha).getByText("Not connected")).toBeDefined();

    await userEvent.setup().click(toggle);

    expect(switched).toEqual([
      {
        url: "/api/peer/links/n-alpha/media-folders/4/enabled",
        body: { enabled: false },
      },
    ]);
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(toggle.closest("label")?.textContent).toBe("Off");
  });

  it("rereads a link's folders in place on a mediaFolders event", async () => {
    renderPage();
    await screen.findByText("/beta/movies");
    betaFolders = [folder(2, "/beta/shows")];

    await emitServerEvent("mediaFolders", {});

    expect(await screen.findByText("/beta/shows")).toBeDefined();
    expect(screen.queryByText("/beta/movies")).toBeNull();
  });
});
