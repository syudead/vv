import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { videoItem } from "../api/libraryItems";
import { clearListSnapshot, saveListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import type { WorkInfo as WorkInfoValue } from "../api/workInfo";
import { type Audience, AudienceProvider } from "../auth/audience";
import WorkInfo from "../player/WorkInfo";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1024 * 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
    previewState: "pending",
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    tags: [],
    ...extra,
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const info: WorkInfoValue = {
  dates: [],
  people: [],
  organizations: [{ role: "maker", organization: { id: 5, name: "Studio" } }],
  urls: [],
  externalIds: [],
};

function LocationProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <span data-testid="location">{location.search}</span>
      <button type="button" onClick={() => void navigate(-1)}>
        テストで戻る
      </button>
    </>
  );
}

function renderAt(initial: string, audience: Audience = "owner") {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <ScanProvider>
              <Routes>
                <Route
                  path="/"
                  element={
                    <>
                      <LibraryPage />
                      <LocationProbe />
                    </>
                  }
                />
                <Route
                  path="/videos/:id"
                  element={
                    <>
                      <p>再生画面</p>
                      <WorkInfo info={info} />
                    </>
                  }
                />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

const fetchMock = vi.fn<typeof fetch>();

interface Entities {
  people: Map<number, string>;
  organizations: Map<number, string>;
}

function install(entities: Entities) {
  fetchMock.mockImplementation((input) => {
    const url = new URL(String(input), "http://localhost");
    const path = url.pathname;
    if (path === "/api/scans/current") return Promise.resolve(json({}, 404));
    if (path === "/api/media-folders") return Promise.resolve(json([{}]));
    if (path === "/api/processing") {
      return Promise.resolve(
        json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
      );
    }
    if (path === "/api/tags") {
      return Promise.resolve(
        json({ items: [{ id: 2, name: "旅行", synonyms: [], videoCount: 1 }] }),
      );
    }
    const credit = /^\/api\/(people|organizations)\/(\d+)$/.exec(path);
    if (credit !== null) {
      const [, kind, id] = credit;
      const names = kind === "people" ? entities.people : entities.organizations;
      const name = names.get(Number(id));
      if (name === undefined) {
        const code = kind === "people" ? "person_not_found" : "organization_not_found";
        return Promise.resolve(json({ code: "not_found", reason: code }, 404));
      }
      return Promise.resolve(
        json({ id: Number(id), name, aliases: [], externalIds: [], videoCount: 1 }),
      );
    }
    if (path === "/api/library") {
      const filtered =
        url.searchParams.has("person") || url.searchParams.has("organization");
      const items = filtered ? [video(1)] : [video(1), video(2), video(3)];
      return Promise.resolve(
        json({ items: items.map((item) => videoItem(item)), total: items.length }),
      );
    }
    throw new Error(`unexpected request: ${url.toString()}`);
  });
}

function libraryRequests(): URL[] {
  return fetchMock.mock.calls
    .map((call) => new URL(String(call[0]), "http://localhost"))
    .filter((url) => url.pathname === "/api/library");
}

function location(): string {
  const params = new URLSearchParams(screen.getByTestId("location").textContent);
  if (params.get("sort") === "addedDesc") params.delete("sort");
  const search = params.toString();
  return search === "" ? "" : `?${search}`;
}

describe("人物・団体での絞り込み", () => {
  beforeEach(() => {
    __resetTagsForTest();
    clearListSnapshot();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    install({
      people: new Map([[1, "Alice"]]),
      organizations: new Map([[5, "Studio"]]),
    });
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockReset();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("動画ページで団体を押すと organization=<id> のライブラリが開き、要求に載り、名前のチップが出て、Back で動画ページに戻る", async () => {
    const user = userEvent.setup();
    renderAt("/videos/7");

    await user.click(screen.getByRole("link", { name: "Show videos with Studio" }));

    expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
    expect(location()).toBe("?organization=5");
    const requests = libraryRequests();
    expect(requests.length).toBeGreaterThan(0);
    for (const url of requests) expect(url.searchParams.get("organization")).toBe("5");
    expect(screen.getByText("Filtering by")).toBeDefined();
    expect(
      await screen.findByRole("button", { name: "Remove the filter for Studio" }),
    ).toBeDefined();
    expect(screen.queryByRole("link", { name: "動画 2" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "テストで戻る" }));
    expect(await screen.findByText("再生画面")).toBeDefined();
  });

  it("名前の読みが 404 なら、そのパラメータを外して知らせ、タグともう一方の絞り込みは残す", async () => {
    renderAt("/?tag=2&person=9&organization=5");

    expect(
      await screen.findByText("Removed a person who no longer exists from the filter"),
    ).toBeDefined();
    await waitFor(() => expect(location()).toBe("?tag=2&organization=5"));
    expect(
      await screen.findByRole("button", { name: "Remove the filter for Studio" }),
    ).toBeDefined();
    expect(screen.queryByRole("button", { name: "Remove the person filter" })).toBeNull();
    await waitFor(() => {
      const last = libraryRequests().at(-1);
      expect(last?.searchParams.has("person")).toBe(false);
      expect(last?.searchParams.get("organization")).toBe("5");
      expect(last?.searchParams.getAll("tag")).toEqual(["2"]);
    });
  });

  it("人物と団体の名前の読みが同時に 404 なら、両方のパラメータが外れる", async () => {
    const pending: (() => void)[] = [];
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      const path = new URL(String(input), "http://localhost").pathname;
      if (path === "/api/people/9" || path === "/api/organizations/8") {
        return new Promise<Response>((resolve) => {
          pending.push(() => {
            void base?.(input, init)?.then(resolve);
          });
        });
      }
      return base?.(input, init) ?? Promise.reject(new Error("no mock"));
    });
    renderAt("/?tag=2&person=9&organization=8");

    await waitFor(() => expect(pending).toHaveLength(2));
    await act(async () => {
      for (const release of pending) release();
      await Promise.resolve();
    });

    await waitFor(() => expect(location()).toBe("?tag=2"));
    expect(screen.queryByRole("button", { name: "Remove the person filter" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Remove the organization filter" }),
    ).toBeNull();
    await waitFor(() => {
      const last = libraryRequests().at(-1);
      expect(last?.searchParams.has("person")).toBe(false);
      expect(last?.searchParams.has("organization")).toBe(false);
    });
  });

  it("名前が届くまではチップに Skeleton と種類ごとの読み上げ名を出す", async () => {
    let resolvePerson: ((response: Response) => void) | undefined;
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      if (new URL(String(input), "http://localhost").pathname === "/api/people/1") {
        return new Promise<Response>((resolve) => {
          resolvePerson = resolve;
        });
      }
      return base?.(input, init) ?? Promise.reject(new Error("no mock"));
    });
    renderAt("/?person=1");

    expect(
      await screen.findByRole("button", { name: "Remove the person filter" }),
    ).toBeDefined();
    await act(async () => {
      resolvePerson?.(
        json({ id: 1, name: "Alice", aliases: [], externalIds: [], videoCount: 1 }),
      );
      await Promise.resolve();
    });
    expect(
      await screen.findByRole("button", { name: "Remove the filter for Alice" }),
    ).toBeDefined();
  });

  it("チップを押すとそのパラメータだけが外れ、フォーカスは次のチップ、最後なら検索欄へ移る", async () => {
    const user = userEvent.setup();
    renderAt("/?tag=2&person=1&organization=5");

    await user.click(
      await screen.findByRole("button", { name: "Remove the filter for Alice" }),
    );
    await waitFor(() => expect(location()).toBe("?tag=2&organization=5"));
    const studio = screen.getByRole("button", { name: "Remove the filter for Studio" });
    await waitFor(() => expect(document.activeElement).toBe(studio));

    await user.click(studio);
    await waitFor(() => expect(location()).toBe("?tag=2"));
    const travel = screen.getByRole("button", { name: "Remove the filter for 旅行" });
    await waitFor(() => expect(document.activeElement).toBe(travel));

    await user.click(travel);
    await waitFor(() => expect(location()).toBe(""));
    expect(screen.queryByText("Filtering by")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("searchbox"));
  });

  it("「Clear filters」は person と organization も外し、並べ替えは残す", async () => {
    const user = userEvent.setup();
    renderAt("/?person=1&organization=5&sort=titleAsc");
    await screen.findByRole("button", { name: "Remove the filter for Alice" });

    await user.click(screen.getByRole("button", { name: "Filter" }));
    await user.click(await screen.findByRole("button", { name: "Clear filters" }));
    await waitFor(() => expect(location()).toBe("?sort=titleAsc"));
  });

  it("/ の控えは /?person=1 には使われない（listSnapshot の鍵に person が要る）", async () => {
    saveListSnapshot(
      { query: "", sort: "addedDesc" },
      {
        items: [videoItem(video(99, { title: "控えの動画" }))],
        total: 1,
        hasMore: false,
        scrollY: 0,
      },
    );
    renderAt("/?person=1");

    expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
    expect(screen.queryByRole("link", { name: "控えの動画" })).toBeNull();
    expect(libraryRequests().length).toBeGreaterThan(0);
  });

  it("ゲストの URL の person と organization は要求の前に外し、URL も直す", async () => {
    renderAt("/?q=ab&person=1&organization=5", "guest");
    await screen.findByRole("link", { name: "動画 2" });
    const requests = libraryRequests();
    expect(requests.length).toBeGreaterThan(0);
    for (const url of requests) {
      expect(url.searchParams.has("person")).toBe(false);
      expect(url.searchParams.has("organization")).toBe(false);
    }
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("?q=ab&sort=addedDesc"),
    );
    expect(screen.queryByText("Filtering by")).toBeNull();
    const calls = fetchMock.mock.calls.map(
      (call) => new URL(String(call[0]), "http://localhost").pathname,
    );
    expect(calls.some((path) => path.startsWith("/api/people"))).toBe(false);
    expect(
      within(document.body).queryByRole("list", { name: "Tags in the filter" }),
    ).toBeNull();
  });
});
