import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useParams } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan, ScanIssue, ScanIssuePage } from "../api/client";
import { ScanProvider } from "../shell/ScanProvider";
import { OwnerAudience } from "../testing/audience";
import ScanStatusSection from "./ScanStatusSection";

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function scan(values: Partial<Scan> = {}): Scan {
  return {
    id: 2,
    status: "partial",
    state: "done",
    videos: { total: 3, settled: 3 },
    issues: { failed: 1, substituted: 1, revision: 1 },
    settledAt: "2026-09-28T06:04:00Z",
    ...values,
  };
}

function issue(values: Partial<ScanIssue> = {}): ScanIssue {
  return {
    severity: "failed",
    kinds: ["unreadable"],
    fileName: "clip.mp4",
    folder: { rootId: 1, path: "", rootName: "media" },
    ...values,
  };
}

function Player() {
  const { id } = useParams();
  return <p>Player {id}</p>;
}

function renderSection() {
  return render(
    <MemoryRouter initialEntries={["/settings"]}>
      <OwnerAudience>
        <ScanProvider>
          <Routes>
            <Route path="/settings" element={<ScanStatusSection />} />
            <Route path="/videos/:id" element={<Player />} />
          </Routes>
        </ScanProvider>
      </OwnerAudience>
    </MemoryRouter>,
  );
}

/**
 * serve は `/api/scans/current` と `/api/scans/current/issues` を返す。`pages` は cursor
 * （1ページ目は空文字）ごとの応答で、呼び出しのたびに今の値を読む。
 */
function serve(
  fetchMock: ReturnType<typeof vi.fn<typeof fetch>>,
  state: { scan: Scan; pages: Record<string, ScanIssuePage> },
) {
  const issueRequests: URL[] = [];
  fetchMock.mockImplementation((input) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
    if (url.pathname === "/api/scans/current/issues") {
      issueRequests.push(url);
      const page = state.pages[url.searchParams.get("cursor") ?? ""];
      return Promise.resolve(page === undefined ? json({}, 400) : json(page));
    }
    return Promise.resolve(json(state.scan));
  });
  return issueRequests;
}

describe("ScanIssueList", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => vi.unstubAllGlobals());

  it("tells failures from substitutions with words and icons, not color alone", async () => {
    serve(fetchMock, {
      scan: scan(),
      pages: {
        "": {
          scanId: 2,
          items: [
            issue({ kinds: ["probe_failed", "thumbnail_failed"], videoId: 7 }),
            issue({
              severity: "substituted",
              kinds: ["thumbnail_first_frame"],
              fileName: "other.mp4",
              videoId: 8,
            }),
          ],
        },
      },
    });
    renderSection();

    const list = await screen.findByRole("region", { name: "Videos with problems" });
    const rows = await within(list).findAllByTestId("scan-issue");
    expect(rows).toHaveLength(2);
    const [failed, substituted] = rows as [HTMLElement, HTMLElement];
    expect(within(failed).getByText("Failed")).toBeDefined();
    expect(
      failed.querySelector("svg.lucide-triangle-alert, svg.lucide-alert-triangle"),
    ).not.toBeNull();
    expect(
      within(failed).getByText("It may not play. It couldn't be analyzed as a video."),
    ).toBeDefined();
    expect(within(failed).getByText("Also: It has no thumbnail.")).toBeDefined();
    expect(within(substituted).getByText("Check")).toBeDefined();
    expect(substituted.querySelector("svg.lucide-info")).not.toBeNull();
    expect(
      within(substituted).getByText(
        "The thumbnail uses the first frame instead. The frame at the usual position couldn't be read.",
      ),
    ).toBeDefined();
    expect(
      screen.getByRole("heading", { level: 3, name: "Videos with problems" }),
    ).toBeDefined();
  });

  it("tells a long name and files with the same name apart by their location", async () => {
    const longName = `${"very-long-name-".repeat(12)}clip.mp4`;
    serve(fetchMock, {
      scan: scan({ issues: { failed: 3, substituted: 0, revision: 1 } }),
      pages: {
        "": {
          scanId: 2,
          items: [
            issue({ fileName: longName }),
            issue({ folder: { rootId: 1, path: "旅行/2024", rootName: "media" } }),
            issue({ folder: { rootId: 2, path: "旅行/2024", rootName: "archive" } }),
          ],
        },
      },
    });
    renderSection();

    const rows = await screen.findAllByTestId("scan-issue");
    expect(rows).toHaveLength(3);
    const long = within(rows[0]!).getByText(longName);
    expect(long.getAttribute("title")).toBe(longName);
    expect(long.className).toContain("truncate");
    const places = rows.slice(1).map((row) =>
      within(row)
        .getByText(/旅行\/2024/)
        .closest("[title]")
        ?.getAttribute("title"),
    );
    expect(places).toEqual(["media / 旅行/2024", "archive / 旅行/2024"]);
    // 未登録のファイルはリンクにしない。
    expect(within(rows[0]!).queryByRole("link")).toBeNull();
  });

  it("loads the next page with Show more", async () => {
    const first = Array.from({ length: 50 }, (_, index) =>
      issue({ fileName: `a-${String(index).padStart(2, "0")}.mp4` }),
    );
    const requests = serve(fetchMock, {
      scan: scan({ issues: { failed: 52, substituted: 0, revision: 1 } }),
      pages: {
        "": { scanId: 2, items: first, nextCursor: "next" },
        next: {
          scanId: 2,
          items: [issue({ fileName: "b-00.mp4" }), issue({ fileName: "b-01.mp4" })],
        },
      },
    });
    renderSection();

    expect(await screen.findAllByTestId("scan-issue")).toHaveLength(50);
    expect(requests[0]?.searchParams.get("limit")).toBe("50");
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Show more" }));
    await waitFor(() => expect(screen.getAllByTestId("scan-issue")).toHaveLength(52));
    expect(screen.getByText("b-01.mp4")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  });

  it("replaces the list for a new scan and rereads it when the revision changes", async () => {
    const state = {
      scan: scan(),
      pages: {
        "": { scanId: 2, items: [issue({ fileName: "old.mp4" })] },
      } as Record<string, ScanIssuePage>,
    };
    const requests = serve(fetchMock, state);
    renderSection();
    expect(await screen.findByText("old.mp4")).toBeDefined();

    // 同じ取り込みで問題が増えた（つながり直したあとの取り直しと同じ経路）。
    state.scan = scan({ issues: { failed: 2, substituted: 1, revision: 2 } });
    state.pages[""] = {
      scanId: 2,
      items: [issue({ fileName: "old.mp4" }), issue({ fileName: "added.mp4" })],
    };
    act(() => window.dispatchEvent(new Event("focus")));
    expect(await screen.findByText("added.mp4")).toBeDefined();

    // 新しい取り込み。
    state.scan = scan({ id: 3, issues: { failed: 1, substituted: 0, revision: 1 } });
    state.pages[""] = { scanId: 3, items: [issue({ fileName: "new.mp4" })] };
    act(() => window.dispatchEvent(new Event("focus")));
    expect(await screen.findByText("new.mp4")).toBeDefined();
    expect(screen.queryByText("old.mp4")).toBeNull();
    expect(screen.queryByText("added.mp4")).toBeNull();
    expect(requests).toHaveLength(3);
  });

  it("rereads the scan and the list when the response belongs to another scan", async () => {
    let scanReads = 0;
    const state = {
      // 最初に読んだ状態は古く、一覧はもう次の取り込みのものである。
      get scan() {
        scanReads += 1;
        return scanReads === 1 ? scan() : scan({ id: 3 });
      },
      pages: {
        "": { scanId: 3, items: [issue({ fileName: "newer.mp4" })] },
      } as Record<string, ScanIssuePage>,
    };
    const requests = serve(fetchMock, state);
    renderSection();
    expect(await screen.findByText("newer.mp4")).toBeDefined();
    expect(scanReads).toBeGreaterThanOrEqual(2);
    expect(requests.length).toBeGreaterThanOrEqual(2);
  });

  it("walks the rows with the keyboard and opens the video", async () => {
    serve(fetchMock, {
      scan: scan(),
      pages: {
        "": {
          scanId: 2,
          items: [
            issue({ kinds: ["probe_failed"], fileName: "broken.mp4", videoId: 7 }),
            issue({
              severity: "substituted",
              kinds: ["seek_thumbnail_full_decode"],
              fileName: "slow.mp4",
              videoId: 8,
            }),
          ],
        },
      },
    });
    renderSection();
    const first = await screen.findByRole("link", {
      name: "broken.mp4. Failed: It may not play. Open the video",
    });
    const second = screen.getByRole("link", {
      name: "slow.mp4. Check: The seek thumbnails were rebuilt from the whole video. Open the video",
    });
    const user = userEvent.setup();
    first.focus();
    await user.tab();
    expect(document.activeElement).toBe(second);
    await user.tab({ shift: true });
    expect(document.activeElement).toBe(first);
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Player 7")).toBeDefined();
  });

  it("does not ask for the list when the scan has no problems", async () => {
    const requests = serve(fetchMock, {
      scan: scan({ status: "done", issues: { failed: 0, substituted: 0, revision: 0 } }),
      pages: {},
    });
    renderSection();
    await screen.findByText("3 of 3 videos done");
    expect(screen.queryByRole("region", { name: "Videos with problems" })).toBeNull();
    expect(requests).toHaveLength(0);
  });
});
