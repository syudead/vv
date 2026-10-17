import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan, Video } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { ScanProvider } from "../shell/ScanProvider";
import { OwnerAudience } from "../testing/audience";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function video(id: number): Video {
  return {
    id,
    title: `video ${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-10-07T00:00:00Z",
    updatedAt: "2026-10-07T00:00:00Z",
    fileCreatedAt: "2026-10-07T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "pending",
    previewState: "pending",
    tags: [],
  };
}

function watchScan(id: number, state: Scan["state"]): Scan {
  return {
    id,
    origin: "watch",
    status: state,
    state,
    videos: { total: 1, settled: state === "running" ? 0 : 1 },
    issues: { failed: 0, substituted: 0, revision: 0 },
  };
}

function renderLibrary() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <OwnerAudience>
            <ScanProvider>
              <LibraryPage />
            </ScanProvider>
          </OwnerAudience>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

describe("LibraryPage after an auto-import", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let ids: number[] = [];

  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
    clearListSnapshot();
    installFakeEventSource();
    ids = [3, 2, 1];
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    fetchMock.mockImplementation((input) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url.pathname === "/api/tag-kinds") return Promise.resolve(json({ items: [] }));
      if (url.pathname === "/api/library") {
        return Promise.resolve(
          json({
            items: ids.map((id) => ({ kind: "video", video: video(id) })),
            total: ids.length,
          }),
        );
      }
      return Promise.resolve(json({}));
    });
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("adds the new video in place, keeping the cards and the selection", async () => {
    const user = userEvent.setup();
    const { container } = renderLibrary();
    const card = await screen.findByRole("link", { name: "video 2" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "video 2"' }));
    expect(screen.getByText("1 video selected")).toBeDefined();
    const cardCounts: number[] = [];
    const observer = new MutationObserver(() =>
      cardCounts.push(container.querySelectorAll("[data-video-id]").length),
    );
    observer.observe(container, { childList: true, subtree: true });
    const libraryCalls = () =>
      fetchMock.mock.calls.filter(
        ([input]) =>
          new URL(String(input), "http://localhost").pathname === "/api/library",
      ).length;
    const callsBefore = libraryCalls();

    ids = [4, ...ids];
    await emitServerEvent("scan", watchScan(9, "running"));
    await emitServerEvent("scan", watchScan(9, "done"));

    expect(await screen.findByRole("link", { name: "video 4" })).toBeDefined();
    observer.disconnect();
    expect(libraryCalls()).toBeGreaterThan(callsBefore);
    // 一覧は一度も空にならず、同じカードの要素を残す（消して読み直すと先頭へ戻る）。
    expect(Math.min(...cardCounts)).toBeGreaterThanOrEqual(3);
    expect(screen.getByRole("link", { name: "video 2" })).toBe(card);
    expect(screen.queryByRole("status", { name: /Loading/ })).toBeNull();
    expect(screen.getByText("1 video selected")).toBeDefined();
    expect(
      screen
        .getByRole("checkbox", { name: 'Select "video 2"' })
        .getAttribute("aria-checked"),
    ).toBe("true");
  });
});
