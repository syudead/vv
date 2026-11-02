import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import type { PeerMediaFolder } from "../api/peer";
import { deferred, json, renderWithScan } from "../testing/autoImportServer";
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

describe("MediaFoldersSection link section ordering of reads and saves", () => {
  type Pending = ReturnType<typeof deferred<Response>>;
  let reads: Pending[];
  let saves: Pending[];
  const first = [folder(1, "/beta/movies"), folder(2, "/beta/shows")];
  beforeEach(() => {
    reads = [];
    saves = [];
    installFakeEventSource();
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const held = deferred<Response>();
      if (init?.method === "PUT") {
        saves.push(held);
        return held.promise;
      }
      if (url === "/api/peer/links/n-beta/media-folders") {
        reads.push(held);
        return held.promise;
      }
      if (url === "/api/scans/current")
        return Promise.resolve(json({ code: "not_found", message: "none" }, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([]));
      if (url === "/api/peer")
        return Promise.resolve(
          json({
            links: [{ slot: 1, nodeId: "n-beta", name: "beta", state: "connected" }],
          }),
        );
      return Promise.reject(new Error(`unexpected request: ${url}`));
    });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function held(list: Pending[], index: number): Pending {
    const pending = list[index];
    if (pending === undefined) throw new Error(`request ${String(index)} was not sent`);
    return pending;
  }
  const answer = (folders: PeerMediaFolder[]) => json({ state: "connected", folders });
  // Returns once the page has handled the late answer, so a later assertion sees what it did with it.
  async function answerLate(pending: Pending, folders: PeerMediaFolder[]) {
    const response = answer(folders);
    const parsed = vi.spyOn(response, "json");
    pending.resolve(response);
    await vi.waitFor(() => expect(parsed).toHaveBeenCalled());
    await act(async () => {
      await parsed.mock.results[0]?.value;
      // The rest of the read is promise continuations, which all run before this macrotask.
      await new Promise((done) => setTimeout(done, 0));
    });
  }
  const toggleFor = (path: string) =>
    screen.findByRole("switch", { name: `Show videos from ${path} on beta` });

  async function renderLoaded() {
    renderWithScan(
      <ToastProvider>
        <MediaFoldersSection />
      </ToastProvider>,
    );
    await vi.waitFor(() => expect(reads).toHaveLength(1));
    held(reads, 0).resolve(answer(first));
    await toggleFor("/beta/movies");
  }

  it("keeps the latest reread when an older one answers last", async () => {
    await renderLoaded();
    await emitServerEvent("mediaFolders", {});
    await emitServerEvent("mediaFolders", {});
    expect(reads).toHaveLength(3);

    held(reads, 2).resolve(answer([folder(3, "/beta/clips")]));
    await screen.findByText("/beta/clips");
    await answerLate(held(reads, 1), first);

    expect(screen.queryByText("/beta/movies")).toBeNull();
    expect(screen.getByText("/beta/clips")).toBeDefined();
  });

  it("keeps a saved switch when a read started before the save answers after it", async () => {
    await renderLoaded();
    await emitServerEvent("mediaFolders", {});
    const toggle = await toggleFor("/beta/movies");

    await userEvent.setup().click(toggle);
    held(saves, 0).resolve(json(folder(1, "/beta/movies", false)));
    await vi.waitFor(() => expect(toggle.getAttribute("aria-disabled")).toBeNull());
    await answerLate(held(reads, 1), first);

    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(toggle.closest("label")?.textContent).toBe("Off");
  });

  it("does not reread while another switch in the section is still saving", async () => {
    await renderLoaded();
    const user = userEvent.setup();
    const movies = await toggleFor("/beta/movies");
    const shows = await toggleFor("/beta/shows");
    await user.click(movies);
    await user.click(shows);
    held(saves, 0).resolve(json(folder(1, "/beta/movies", false)));
    await vi.waitFor(() => expect(movies.getAttribute("aria-disabled")).toBeNull());

    await emitServerEvent("mediaFolders", {});
    held(saves, 1).resolve(json(folder(2, "/beta/shows", false)));

    await vi.waitFor(() => expect(shows.getAttribute("aria-checked")).toBe("false"));
    expect(reads).toHaveLength(1);
    expect(movies.getAttribute("aria-checked")).toBe("false");
  });
});
