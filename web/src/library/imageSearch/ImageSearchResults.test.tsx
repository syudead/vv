import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  ImageSearchItem,
  ImageSearchMatch,
  ImageSearchResult,
} from "../../api/imageSearch";
import { video } from "../../testing/groupPage";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import { ImageSearchResults } from "./ImageSearchResults";

// Hand-built answers for the states a real search cannot be made to produce; e2e/image-search.e2e.ts
// covers the rows a real search answers.

const imageBands: ImageSearchResult["bands"] = [
  { id: "strong", minPercent: 90, maxPercent: 100 },
  { id: "similar", minPercent: 81, maxPercent: 89 },
];

function frameItem(
  id: number,
  rate: number,
  extra: Partial<ImageSearchMatch> = {},
): ImageSearchItem {
  return {
    video: video(id, { title: `動画 ${String(id)}` }),
    match: {
      videoId: id,
      rate,
      kind: "frame",
      positionMs: 1_000,
      band: rate >= 0.895 ? "strong" : "similar",
      frames: [{ positionMs: 1_000, rate }],
      frameCount: 1,
      ...extra,
    },
  };
}

function Player() {
  const { pathname, search } = useLocation();
  return <p>{`再生画面 ${pathname}${search}`}</p>;
}

function renderResults(items: ImageSearchItem[], bands = imageBands) {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <Routes>
          <Route
            path="/"
            element={
              <ImageSearchResults
                result={{ query: { kind: "image" }, bands, items, unindexedCount: 0 }}
                backTo="/"
                entry="entry"
                onOpen={() => {}}
              />
            }
          />
          <Route path="/videos/:id" element={<Player />} />
        </Routes>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

describe("画像検索の結果の行", () => {
  beforeEach(() => {
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
    vi.unstubAllGlobals();
  });

  it("行のない帯は描かず、1 つの帯だけでも見出しを出す", () => {
    renderResults([frameItem(1, 0.85)]);

    expect(screen.getAllByRole("heading").map((heading) => heading.textContent)).toEqual([
      "Similar· 81–89%",
    ]);
  });

  it("制限で切られた行は返った枚数ではなく見つかった枚数をメタに出す", () => {
    const frames = Array.from({ length: 12 }, (_, index) => ({
      positionMs: (index + 1) * 10_000,
      rate: 0.95,
    }));
    renderResults([frameItem(1, 0.95, { positionMs: 10_000, frames, frameCount: 20 })]);

    expect(screen.getByText("20 matching frames · best at 0:10")).toBeDefined();
    expect(
      within(screen.getByRole("list", { name: "Matching frames" })).getAllByRole("link"),
    ).toHaveLength(12);
  });

  it("表示率が同点の 2 行には Best candidate も default の Play も出さない", () => {
    // 0.952 and 0.948 both show as 95%.
    renderResults([frameItem(1, 0.952), frameItem(2, 0.948)]);

    expect(screen.queryByText("Best candidate")).toBeNull();
    for (const play of screen.getAllByRole("link", { name: "Play" })) {
      expect(play.getAttribute("data-variant")).toBe("secondary");
    }
  });

  it("表示率が上の最初の行だけに Best candidate と default の Play を出す", () => {
    renderResults([frameItem(1, 0.96), frameItem(2, 0.948)]);

    const [first, second] = screen
      .getAllByRole("listitem")
      .filter((row) => row.matches("[data-slot=result-row]"));
    if (first === undefined || second === undefined) throw new Error("rows missing");
    expect(within(first).getByText("Best candidate")).toBeDefined();
    expect(within(first).getByRole("link", { name: "Play" }).dataset.variant).toBe(
      "default",
    );
    expect(within(second).queryByText("Best candidate")).toBeNull();
  });

  it("矢印キー、Home、End がストリップの 1 つのタブストップを動かし、Enter でその時刻から再生する", async () => {
    const user = userEvent.setup();
    const frames = [1_000, 2_000, 3_000].map((positionMs) => ({
      positionMs,
      rate: 0.95,
    }));
    renderResults([frameItem(1, 0.95, { frames, frameCount: 3 })]);
    const strip = screen.getByRole("list", { name: "Matching frames" });
    const link = (name: string) => within(strip).getByRole("link", { name });
    const stops = () =>
      within(strip)
        .getAllByRole("link")
        .filter((frame) => frame.tabIndex === 0);

    expect(stops()).toEqual([link("Play from 0:01")]);
    link("Play from 0:01").focus();
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(link("Play from 0:02"));
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(link("Play from 0:03"));
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(link("Play from 0:03"));
    await user.keyboard("{Home}");
    expect(document.activeElement).toBe(link("Play from 0:01"));
    await user.keyboard("{ArrowLeft}{ArrowRight}");
    expect(stops()).toEqual([link("Play from 0:02")]);

    await user.keyboard("{Enter}");
    expect(screen.getByText("再生画面 /videos/1?t=2")).toBeDefined();
  });

  it("フレームのない動画の行は SpanBar だけで、代表より長い版の区間を押すとその版を区間の始めから開く", async () => {
    const user = userEvent.setup();
    renderResults(
      [
        {
          video: video(5, { title: "動画 5", durationMs: 60_000 }),
          match: {
            videoId: 7,
            rate: 1,
            kind: "partial",
            positionMs: 70_500,
            startMs: 70_000,
            endMs: 80_000,
            band: "strong",
            frames: [],
            frameCount: 0,
          },
        },
      ],
      [
        { id: "strong", minPercent: 90, maxPercent: 100 },
        { id: "similar", minPercent: 50, maxPercent: 89 },
      ],
    );

    expect(screen.queryByRole("list", { name: "Matching frames" })).toBeNull();
    expect(screen.getByText("1:10–1:20")).toBeDefined();
    expect(screen.getByRole("link", { name: "Play" }).getAttribute("href")).toBe(
      "/videos/7?t=70",
    );
    const lane = screen.getByRole("group", { name: "Matched section" });
    await user.click(within(lane).getByRole("button", { name: "Play from 1:10" }));
    expect(screen.getByText("再生画面 /videos/7?t=70")).toBeDefined();
  });
});
