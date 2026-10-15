import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { videoItem } from "../api/libraryItems";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

// タグの種類でのライブラリの絞り込み（specs/051-tag-kinds/ui-design.md「Library」、
// 親 Issue #1136 の要件 5、子 Issue #1155）。

function video(id: number): Video {
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
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function kind(id: number, name: string) {
  return { id, name, tagCount: 1, createdAt: "2026-10-01T00:00:00Z" };
}

/** 種類 1 の動画は 1 だけ、種類 2 の動画は 2 だけ、ほかの種類は無し。 */
const videosByKind = new Map<string, number[]>([
  ["1", [1]],
  ["2", [2]],
]);

function LocationProbe() {
  return <span data-testid="location">{useLocation().search}</span>;
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
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

const fetchMock = vi.fn<typeof fetch>();

function install(kinds: ReturnType<typeof kind>[]) {
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
    if (path === "/api/tag-kinds") return Promise.resolve(json({ items: kinds }));
    if (path === "/api/library") {
      const tagKind = url.searchParams.get("tagKind");
      const ids = tagKind === null ? [1, 2, 3] : (videosByKind.get(tagKind) ?? []);
      return Promise.resolve(
        json({ items: ids.map((id) => videoItem(video(id))), total: ids.length }),
      );
    }
    throw new Error(`unexpected request: ${url.toString()}`);
  });
}

function requests(pathname: string): URL[] {
  return fetchMock.mock.calls
    .map((call) => new URL(String(call[0]), "http://localhost"))
    .filter((url) => url.pathname === pathname);
}

/** location は今の URL の問い合わせ部分である（既定の並び順を書き足した分は除く）。 */
function location(): string {
  const params = new URLSearchParams(screen.getByTestId("location").textContent);
  if (params.get("sort") === "addedDesc") params.delete("sort");
  const search = params.toString();
  return search === "" ? "" : `?${search}`;
}

describe("タグの種類での絞り込み", () => {
  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
    clearListSnapshot();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    // Radix Select はポインタの捕捉を使う。jsdom には無いので何もしない実装を置く。
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.releasePointerCapture ??= () => undefined;
    install([kind(1, "Work"), kind(2, "Genre"), kind(3, "Place")]);
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockReset();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("ポップオーバーで種類を選ぶと、URL に tagKind が付き、一致する動画だけが出て、ボタンが数える", async () => {
    const user = userEvent.setup();
    renderAt("/");
    expect(await screen.findByRole("link", { name: "動画 3" })).toBeDefined();
    // 開く前は種類の一覧を読まない。
    expect(requests("/api/tag-kinds")).toHaveLength(0);

    await user.click(screen.getByRole("button", { name: "Filter" }));
    await user.click(await screen.findByRole("combobox", { name: "Tag kind" }));
    const options = await screen.findAllByRole("option");
    // Any kind の後に名前の自然順。No kind は無い。
    expect(options.map((option) => option.textContent)).toEqual([
      "Any kind",
      "Genre",
      "Place",
      "Work",
    ]);
    await user.click(screen.getByRole("option", { name: "Genre" }));

    await waitFor(() => expect(location()).toBe("?tagKind=2"));
    expect(await screen.findByRole("link", { name: "動画 2" })).toBeDefined();
    expect(screen.queryByRole("link", { name: "動画 1" })).toBeNull();
    expect(screen.queryByRole("link", { name: "動画 3" })).toBeNull();
    expect(requests("/api/library").at(-1)?.searchParams.get("tagKind")).toBe("2");
    expect(
      screen.getByRole("button", { name: 'Remove the filter for the kind "Genre"' })
        .textContent,
    ).toContain("Kind: Genre");
    expect(screen.getByRole("button", { name: "Filter (1 applied)" })).toBeDefined();
    // 一覧は 1 回だけ読む。
    expect(requests("/api/tag-kinds")).toHaveLength(1);
  });

  it("チップを押すと tagKind だけが外れ、ほかの絞り込みは残る", async () => {
    const user = userEvent.setup();
    renderAt("/?tag=2&tagKind=1");

    await user.click(
      await screen.findByRole("button", {
        name: 'Remove the filter for the kind "Work"',
      }),
    );
    await waitFor(() => expect(location()).toBe("?tag=2"));
    expect(
      screen.queryByRole("button", { name: 'Remove the filter for the kind "Work"' }),
    ).toBeNull();
    await waitFor(() =>
      expect(requests("/api/library").at(-1)?.searchParams.has("tagKind")).toBe(false),
    );
  });

  it("「Clear filters」で tagKind も外れる", async () => {
    const user = userEvent.setup();
    renderAt("/?tagKind=1&sort=titleAsc");
    await screen.findByRole("button", { name: 'Remove the filter for the kind "Work"' });

    await user.click(screen.getByRole("button", { name: "Filter (1 applied)" }));
    await user.click(await screen.findByRole("button", { name: "Clear filters" }));
    await waitFor(() => expect(location()).toBe("?sort=titleAsc"));
    expect(await screen.findByRole("link", { name: "動画 3" })).toBeDefined();
  });

  it("GET /api/tag-kinds に無い種類は、メッセージとともに URL から外れ、ほかの絞り込みは残る", async () => {
    renderAt("/?tag=2&tagKind=9");

    expect(
      await screen.findByText("Removed a kind that no longer exists from the filter"),
    ).toBeDefined();
    await waitFor(() => expect(location()).toBe("?tag=2"));
    expect(screen.queryByRole("button", { name: "Remove the kind filter" })).toBeNull();
    await waitFor(() => {
      const last = requests("/api/library").at(-1);
      expect(last?.searchParams.has("tagKind")).toBe(false);
      expect(last?.searchParams.getAll("tag")).toEqual(["2"]);
    });
  });

  it("一覧が届くまではチップに Skeleton を出し、読み込みに失敗しても外さない", async () => {
    let fail: (() => void) | undefined;
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      if (new URL(String(input), "http://localhost").pathname === "/api/tag-kinds") {
        return new Promise<Response>((resolve) => {
          fail = () => resolve(json({ code: "internal" }, 500));
        });
      }
      return base?.(input, init) ?? Promise.reject(new Error("no mock"));
    });
    renderAt("/?tagKind=1");

    expect(
      await screen.findByRole("button", { name: "Remove the kind filter" }),
    ).toBeDefined();
    await act(async () => {
      fail?.();
      await Promise.resolve();
    });
    expect(screen.getByRole("button", { name: "Remove the kind filter" })).toBeDefined();
    expect(location()).toBe("?tagKind=1");
    expect(requests("/api/library").at(-1)?.searchParams.get("tagKind")).toBe("1");
  });

  it("ゲストには種類の行を出さず、URL の tagKind は要求の前に外す", async () => {
    const user = userEvent.setup();
    renderAt("/?tagKind=1", "guest");
    await screen.findByRole("link", { name: "動画 3" });
    for (const url of requests("/api/library")) {
      expect(url.searchParams.has("tagKind")).toBe(false);
    }
    await waitFor(() => expect(location()).toBe(""));

    await user.click(screen.getByRole("button", { name: "Filter" }));
    await screen.findByRole("checkbox", { name: "Playable only" });
    expect(screen.queryByRole("combobox", { name: "Tag kind" })).toBeNull();
    expect(requests("/api/tag-kinds")).toHaveLength(0);
  });
});
