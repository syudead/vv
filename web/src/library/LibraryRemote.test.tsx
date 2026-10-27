import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { clearListSnapshot } from "../api/listSnapshot";
import type { PeerState } from "../api/peer";
import { __resetTagsForTest } from "../api/tags";
import { AudienceProvider } from "../auth/audience";
import { remoteIdOffset } from "../lib/remoteId";
import PeerNotice, { usePeerNoticeShown } from "../shell/PeerNotice";
import { PeerProvider } from "../shell/PeerProvider";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

const remoteId = remoteIdOffset + 5;
const peerInfo = {
  name: "studio-pc",
  address: "http://192.168.1.20:8080",
  version: "1",
  pinned: false,
};
const connected: PeerState = { state: "connected", link: peerInfo };
const unreachable: PeerState = {
  state: "unreachable",
  reason: "network",
  link: peerInfo,
};

function video(id: number, title: string): Video {
  return {
    id,
    virtual: false,
    title,
    fileTitle: `CODE-${String(id)}`,
    contentVersion: `v${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    fileModifiedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    durationMs: 60_000,
    previewState: "pending",
    tags: [],
    cardCredits: { performers: [] },
  };
}

const local = video(1, "手元の動画");
const remote = video(remoteId, "リモートの動画");

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** The library lists the peer's video only while it is connected, as the server does. */
const server = {
  peerListed: true,
  libraryReads: 0,
  codeAnswer: { applied: 1 } as unknown,
  groupRequests: [] as string[],
};

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = init?.method ?? "GET";
    if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
    if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
    if (url.pathname === "/api/processing") {
      return Promise.resolve(
        json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
      );
    }
    if (url.pathname === "/api/tags") return Promise.resolve(json({ items: [] }));
    if (url.pathname === "/api/peer") return Promise.resolve(json(connected));
    if (url.pathname === "/api/library") {
      server.libraryReads += 1;
      const shown = server.peerListed ? [local, remote] : [local];
      const items = shown.map((item) => ({ kind: "video", video: item }));
      return Promise.resolve(json({ items, total: items.length }));
    }
    if (url.pathname === "/api/groups" && method === "GET") {
      server.groupRequests.push(url.search);
      return Promise.resolve(json({ groups: [] }));
    }
    if (url.pathname === "/api/video-code-edits/summary" && method === "POST") {
      return Promise.resolve(json({ rows: [{ video: local }, { video: remote }] }));
    }
    if (url.pathname === "/api/video-code-edits" && method === "POST") {
      return Promise.resolve(json(server.codeAnswer));
    }
    throw new Error(`unexpected request: ${method} ${url.toString()}`);
  });
  vi.stubGlobal("fetch", fetchMock);
}

/** The shell's notice slot as the app composes it, above the page. */
function Notice() {
  return usePeerNoticeShown() ? <PeerNotice /> : null;
}

function renderLibrary() {
  render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience="owner">
            <ScanProvider>
              <PeerProvider>
                <Notice />
                <Routes>
                  <Route path="/" element={<LibraryPage />} />
                </Routes>
              </PeerProvider>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

async function selectBoth(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("checkbox", { name: 'Select "手元の動画"' }));
  await user.click(screen.getByRole("checkbox", { name: 'Select "リモートの動画"' }));
}

async function applyCodes(user: ReturnType<typeof userEvent.setup>) {
  const dialog = await screen.findByRole("dialog", { name: "Edit codes" });
  await user.click(within(dialog).getByRole("button", { name: "Apply to 2 videos" }));
}

beforeEach(() => {
  __resetTagsForTest();
  clearListSnapshot();
  localStorage.clear();
  installFakeEventSource();
  server.peerListed = true;
  server.libraryReads = 0;
  server.codeAnswer = { applied: 1 };
  server.groupRequests = [];
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe("The library of two paired VVMDMs", () => {
  it("shows the notice while the peer is unreachable, keeps the list usable and reloads it both ways", async () => {
    install();
    const user = userEvent.setup();
    renderLibrary();
    expect(await screen.findByRole("link", { name: "リモートの動画" })).toBeDefined();
    await emitServerEvent("peer", connected);
    expect(screen.queryByRole("alert")).toBeNull();
    const reads = server.libraryReads;

    server.peerListed = false;
    await emitServerEvent("peer", unreachable);
    const notice = await screen.findByRole("alert");
    expect(within(notice).getByText("Can't reach studio-pc")).toBeDefined();
    expect(within(notice).getByRole("link", { name: "Open Pairing" })).toBeDefined();
    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "リモートの動画" })).toBeNull(),
    );
    expect(server.libraryReads).toBe(reads + 1);
    await user.click(screen.getByRole("checkbox", { name: 'Select "手元の動画"' }));
    expect(screen.getByText("1 video selected")).toBeDefined();

    server.peerListed = true;
    await emitServerEvent("peer", connected);
    expect(await screen.findByRole("link", { name: "リモートの動画" })).toBeDefined();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("offers no favorite, playlist, fill or group picker for a mixed selection, and the peer's groups for a remote one", async () => {
    install();
    const user = userEvent.setup();
    renderLibrary();
    await selectBoth(user);
    const bar = screen.getByRole("region", { name: "Selection actions" });
    expect(within(bar).queryByRole("button", { name: /Favorite/ })).toBeNull();
    expect(within(bar).queryByRole("button", { name: "Add to playlist" })).toBeNull();
    expect(within(bar).queryByRole("button", { name: "Fill info…" })).toBeNull();
    const add = within(bar).getByRole("button", { name: "Add to group" });
    expect(add.hasAttribute("disabled")).toBe(true);
    expect(add.getAttribute("title")).toBe("Select videos from one VVMDM to do this");

    await user.click(screen.getByRole("checkbox", { name: 'Select "手元の動画"' }));
    await user.click(within(bar).getByRole("button", { name: "Add to group" }));
    expect(await screen.findByText("Groups on studio-pc")).toBeDefined();
    await waitFor(() => expect(server.groupRequests[0]).toContain("node=peer"));
  });

  it("names the videos the connected peer did not change and keeps only them selected", async () => {
    install();
    server.codeAnswer = {
      applied: 1,
      remote: { applied: 0, failedVideoIds: [remoteId], reason: "content_changed" },
    };
    const user = userEvent.setup();
    renderLibrary();
    await selectBoth(user);
    await user.click(screen.getByRole("button", { name: "Edit codes" }));
    await applyCodes(user);

    const dialog = await screen.findByRole("dialog", {
      name: "Not changed on studio-pc",
    });
    expect(within(dialog).getByRole("list").textContent).toBe("リモートの動画");
    expect(screen.queryByText(/Set the code of/)).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    const checked = (name: string) =>
      screen
        .getByRole("checkbox", { name: `Select "${name}"` })
        .getAttribute("aria-checked");
    expect(checked("リモートの動画")).toBe("true");
    expect(checked("手元の動画")).toBe("false");
  });

  it("still names the peer's videos when the peer drops between the selection and the answer", async () => {
    install();
    server.codeAnswer = {
      applied: 1,
      remote: { applied: 0, failedVideoIds: [remoteId], reason: "peer_unreachable" },
    };
    const user = userEvent.setup();
    renderLibrary();
    await emitServerEvent("peer", connected);
    await selectBoth(user);
    await user.click(screen.getByRole("button", { name: "Edit codes" }));
    await screen.findByRole("dialog", { name: "Edit codes" });

    server.peerListed = false;
    await emitServerEvent("peer", unreachable);
    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "リモートの動画" })).toBeNull(),
    );
    await applyCodes(user);

    const dialog = await screen.findByRole("dialog", {
      name: "Not changed on studio-pc",
    });
    expect(within(dialog).getByText(/Couldn't reach the other VVMDM/)).toBeDefined();
    expect(within(dialog).getByRole("list").textContent).toBe("リモートの動画");
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();
  });
});
