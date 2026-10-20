import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Profiler } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
  type Video,
} from "../api/client";
import { type Audience, AudienceProvider } from "../auth/audience";
import { createPreviewCoordinator } from "./usePreviewCoordination";
import VideoCard, { VideoRow } from "./VideoCard";

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  fetchSeekThumbnailSprite: vi.fn(),
  fetchSeekThumbnailSheet: vi.fn(),
}));

function video(extra: Partial<Video> = {}): Video {
  return {
    id: 1,
    title: "動画 1",
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: false,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: "/thumbnail.jpg",
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    previewState: "done",
    previewUrl: "/api/videos/1/preview?v=key",
    tags: [],
    ...extra,
  };
}

function renderCard(
  item: Video = video(),
  props: Partial<React.ComponentProps<typeof VideoCard>> = {},
) {
  return render(
    <MemoryRouter>
      <VideoCard
        video={item}
        backTo="/"
        selected={false}
        selectionMode={false}
        onSelect={vi.fn()}
        {...props}
      />
    </MemoryRouter>,
  );
}

/** A shared coordinator with spies on its actions. */
function coordinator() {
  const previewCoordinator = createPreviewCoordinator();
  return {
    previewCoordinator,
    start: vi.spyOn(previewCoordinator, "start"),
    reset: vi.spyOn(previewCoordinator, "reset"),
  };
}

describe("VideoCard hover preview", () => {
  const play = vi.fn(() => Promise.resolve());
  const pause = vi.fn();
  const load = vi.fn();

  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(play);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(pause);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(load);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("waits exactly 400ms and keeps the thumbnail until playing", async () => {
    renderCard(video({ unplayableReason: "container" }));
    const card = screen.getByRole("article");
    expect(screen.queryByText("Missing information needed for playback")).toBeNull();

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(399));
    expect(document.querySelector("video")).toBeNull();

    act(() => vi.advanceTimersByTime(1));
    const preview = document.querySelector("video");
    expect(preview).not.toBeNull();
    if (preview === null) return;
    expect(preview.getAttribute("src")).toBe("/api/videos/1/preview?v=key");
    expect(play).toHaveBeenCalledTimes(1);
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(false);

    fireEvent.playing(preview);
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(true);
    expect(preview.muted).toBe(true);
    expect(preview.playsInline).toBe(true);
    expect(preview.controls).toBe(false);
    await act(async () => Promise.resolve());
  });

  it("cancels short, touch, pen, focus, checkbox, and ineligible hovers", () => {
    const { previewCoordinator, start: onStart } = coordinator();
    const { rerender } = renderCard(video(), { previewCoordinator });
    const card = screen.getByRole("article");

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    fireEvent.pointerLeave(card);
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).not.toHaveBeenCalled();

    for (const pointerType of ["touch", "pen"]) {
      fireEvent.pointerEnter(card, { pointerType });
      act(() => vi.advanceTimersByTime(400));
    }
    expect(onStart).not.toHaveBeenCalled();

    fireEvent.focus(screen.getByRole("link"));
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).not.toHaveBeenCalled();

    fireEvent.pointerEnter(screen.getByRole("checkbox").parentElement!, {
      pointerType: "mouse",
    });
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).not.toHaveBeenCalled();

    rerender(
      <MemoryRouter>
        <VideoCard
          video={video({ previewState: "pending", previewUrl: undefined })}
          backTo="/"
          selected={false}
          selectionMode={false}
          onSelect={vi.fn()}
          previewCoordinator={previewCoordinator}
        />
      </MemoryRouter>,
    );
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).not.toHaveBeenCalled();
    expect(document.querySelector("video")).toBeNull();
  });

  it("does not start a preview for a failed preview or a done preview without a URL", () => {
    for (const extra of [
      { previewState: "failed" as const },
      { previewState: "done" as const, previewUrl: undefined },
    ]) {
      const { previewCoordinator, start: onStart } = coordinator();
      renderCard(video(extra), { previewCoordinator });
      fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
      act(() => vi.advanceTimersByTime(500));
      expect(onStart).not.toHaveBeenCalled();
      expect(document.querySelector("video")).toBeNull();
      cleanup();
    }
  });

  it("selects with Space and opens with Enter from the keyboard without a preview", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onSelect = vi.fn();
    render(
      <MemoryRouter>
        <Routes>
          <Route
            path="/"
            element={
              <VideoCard
                video={video()}
                backTo="/"
                selected={false}
                selectionMode={false}
                onSelect={onSelect}
              />
            }
          />
          <Route path="/videos/:id" element={<p>video page</p>} />
        </Routes>
      </MemoryRouter>,
    );

    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("checkbox", { name: 'Select "動画 1"' }),
    );
    await user.keyboard(" ");
    expect(onSelect).toHaveBeenCalledWith(1, true);

    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("link", { name: "動画 1" }));
    act(() => vi.advanceTimersByTime(500));
    expect(document.querySelector("video")).toBeNull();
    expect(play).not.toHaveBeenCalled();

    await user.keyboard("{Enter}");
    expect(screen.getByText("video page")).toBeDefined();
    expect(play).not.toHaveBeenCalled();
  });

  it("returns to the thumbnail on leave, media error, or play rejection and retries", async () => {
    const { previewCoordinator, start: onStart } = coordinator();
    renderCard(video(), { previewCoordinator });
    const card = screen.getByRole("article");

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    const preview = document.querySelector("video");
    expect(preview).not.toBeNull();
    if (preview === null) return;
    fireEvent.error(preview);
    expect(document.querySelector("video")).toBeNull();
    expect(load).toHaveBeenCalled();

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).toHaveBeenCalledTimes(2);

    play.mockImplementationOnce(() => Promise.reject(new Error("blocked")));
    fireEvent.pointerLeave(card);
    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    await act(async () => Promise.resolve());
    expect(document.querySelector("video")).toBeNull();
    expect(pause).toHaveBeenCalled();
  });

  it("does not let an old play rejection cancel a newer hover lifecycle", async () => {
    let rejectOldPlay: ((reason?: unknown) => void) | undefined;
    play.mockImplementationOnce(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectOldPlay = reject;
        }),
    );
    renderCard();
    const card = screen.getByRole("article");

    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    fireEvent.pointerLeave(card);
    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelector("video")).not.toBeNull();

    await act(async () => rejectOldPlay?.(new Error("late rejection")));
    expect(document.querySelector("video")).not.toBeNull();
  });

  it("keeps the current frame while buffering and releases on coordinator reset", () => {
    const { previewCoordinator } = coordinator();
    renderCard(video(), { previewCoordinator });
    const card = screen.getByRole("article");
    fireEvent.pointerEnter(card, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    const preview = document.querySelector("video");
    expect(preview).not.toBeNull();
    if (preview === null) return;

    fireEvent.playing(preview);
    fireEvent.waiting(preview);
    fireEvent.stalled(preview);
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(true);
    expect(document.querySelector("video")).not.toBeNull();

    act(() => previewCoordinator.reset());
    expect(document.querySelector("video")).toBeNull();
    expect(pause).toHaveBeenCalled();
    expect(load).toHaveBeenCalled();
  });

  it("removes an inactive coordinated preview in the active-card render", async () => {
    const item = video();
    const { previewCoordinator } = coordinator();
    renderCard(item, { previewCoordinator });
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    const preview = document.querySelector("video");
    expect(preview).not.toBeNull();
    if (preview === null) return;
    fireEvent.playing(preview);
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(true);

    act(() => previewCoordinator.start(2));
    expect(document.querySelector("video")).toBeNull();
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(false);
    await act(async () => Promise.resolve());
    expect(pause).toHaveBeenCalled();
    expect(load).toHaveBeenCalled();
  });

  it("does not suppress an existing warning when a preview URL is present", () => {
    const { previewCoordinator, start: onStart } = coordinator();
    renderCard(video({ durationMs: undefined }), { previewCoordinator });
    expect(screen.getByText("Missing information needed for playback")).toBeDefined();

    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(screen.getByText("Missing information needed for playback")).toBeDefined();
    expect(onStart).not.toHaveBeenCalled();
    expect(document.querySelector("video")).toBeNull();
  });

  it("re-renders only the cards whose preview becomes active or inactive", () => {
    const { previewCoordinator } = coordinator();
    const commits = new Map<string, number>();
    const count = (id: string) => commits.set(id, (commits.get(id) ?? 0) + 1);
    render(
      <MemoryRouter>
        {[1, 2, 3].map((id) => (
          <Profiler key={id} id={String(id)} onRender={count}>
            <VideoCard
              video={video({ id })}
              backTo="/"
              selected={false}
              selectionMode={false}
              previewCoordinator={previewCoordinator}
            />
          </Profiler>
        ))}
      </MemoryRouter>,
    );
    commits.clear();

    act(() => previewCoordinator.start(1));
    act(() => previewCoordinator.start(2));
    act(() => previewCoordinator.reset());
    expect(commits.get("1")).toBeGreaterThan(0);
    expect(commits.get("2")).toBeGreaterThan(0);
    expect(commits.has("3")).toBe(false);
  });

  it("releases the media element before unmount detaches its ref", () => {
    const rendered = renderCard(video(), coordinator());
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelector("video")).not.toBeNull();

    rendered.unmount();
    expect(pause).toHaveBeenCalled();
    expect(load).toHaveBeenCalled();
  });

  it("stops preview when selection mode starts and keeps card selection priority", () => {
    const onSelect = vi.fn();
    const { previewCoordinator, reset: onReset } = coordinator();
    const item = video();
    const rendered = renderCard(item, { onSelect, previewCoordinator });
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelector("video")).not.toBeNull();

    rendered.rerender(
      <MemoryRouter>
        <VideoCard
          video={item}
          backTo="/"
          selected={false}
          selectionMode
          onSelect={onSelect}
          previewCoordinator={previewCoordinator}
        />
      </MemoryRouter>,
    );
    expect(document.querySelector("video")).toBeNull();

    fireEvent.click(screen.getByRole("link", { name: item.title }));
    expect(onReset).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith(item.id, true);
  });
});

describe("VideoCard tagsRow（issue 269）", () => {
  afterEach(() => cleanup());

  it("tagsRow を渡すと、題名の下にそれを出す", () => {
    renderCard(
      video({
        tags: [
          {
            kind: null,
            id: 1,
            name: "旅行",
            manual: true,
            fromFolder: false,
            tentative: false,
          },
        ],
      }),
      {
        tagsRow: () => <p>タグの行</p>,
      },
    );
    expect(screen.getByText("タグの行")).toBeDefined();
  });

  it("tagsRow はリンクの外、同じ article の中に置く", () => {
    renderCard(
      video({
        tags: [
          {
            kind: null,
            id: 1,
            name: "旅行",
            manual: true,
            fromFolder: false,
            tentative: false,
          },
        ],
      }),
      {
        tagsRow: () => <button type="button">tags</button>,
      },
    );
    const article = screen.getByRole("article");
    const link = screen.getByRole("link");
    const button = screen.getByRole("button", { name: "tags" });
    expect(article.contains(link)).toBe(true);
    expect(article.contains(button)).toBe(true);
    expect(link.contains(button)).toBe(false);
  });

  it("タグが無い動画では tagsRow の場所に何も出さない", () => {
    renderCard(video({ tags: [] }), { tagsRow: () => <p>タグの行</p> });
    expect(screen.queryByText("タグの行")).toBeNull();
  });

  it("タグの行はリンクの外、題名の後ろに置く（B3）", () => {
    renderCard(
      video({
        tags: [
          {
            kind: null,
            id: 1,
            name: "旅行",
            manual: true,
            fromFolder: false,
            tentative: false,
          },
        ],
      }),
      {
        tagsRow: () => <p>タグの行</p>,
      },
    );
    const row = screen.getByText("タグの行").parentElement;
    expect(row?.closest("a")).toBeNull();
    expect(row?.parentElement?.className).toContain("gap-2");
  });

  it("同じ参照の props で親が再描画しても memo で再描画せず、tagsRow を呼び直さない（N4）", () => {
    const item = video({
      tags: [
        {
          kind: null,
          id: 1,
          name: "旅行",
          manual: true,
          fromFolder: false,
          tentative: false,
        },
      ],
    });
    const stableTagsRow = vi.fn(() => <p>タグの行</p>);
    const props = {
      video: item,
      backTo: "/",
      selected: false,
      selectionMode: false,
      onSelect: vi.fn(),
      tagsRow: stableTagsRow,
    };
    const { rerender } = render(
      <MemoryRouter>
        <VideoCard {...props} />
      </MemoryRouter>,
    );
    expect(stableTagsRow).toHaveBeenCalledTimes(1);

    // Rerender with identical prop references to verify memo(VideoCard) does not invoke tagsRow again.
    rerender(
      <MemoryRouter>
        <VideoCard {...props} />
      </MemoryRouter>,
    );
    expect(stableTagsRow).toHaveBeenCalledTimes(1);

    const nextTagsRow = vi.fn(() => <p>タグの行</p>);
    rerender(
      <MemoryRouter>
        <VideoCard {...props} tagsRow={nextTagsRow} />
      </MemoryRouter>,
    );
    expect(nextTagsRow).toHaveBeenCalledTimes(1);
  });
});

describe("VideoCard の公開の印", () => {
  afterEach(() => cleanup());

  function renderAs(audience: Audience, item: Video) {
    return render(
      <MemoryRouter>
        <AudienceProvider audience={audience}>
          <VideoCard video={item} backTo="/" selected={false} selectionMode={false} />
        </AudienceProvider>
      </MemoryRouter>,
    );
  }

  it("所有者の公開の動画には、時間の面の先頭に地球の印と読み上げの「公開」を出す", () => {
    const { container } = renderAs("owner", video({ public: true }));
    const mark = screen.getByText("Public");
    expect(mark.className).toContain("sr-only");
    const face = mark.parentElement!;
    expect(face.firstElementChild?.tagName.toLowerCase()).toBe("svg");
    expect(face.firstElementChild?.getAttribute("class")).toContain("lucide-globe");
    expect(face.textContent).toBe("Public1:00");
    expect(container.querySelectorAll(".lucide-globe")).toHaveLength(1);
  });

  it("非公開の動画には印を出さない", () => {
    const { container } = renderAs("owner", video({ public: false }));
    expect(screen.queryByText("Public")).toBeNull();
    expect(container.querySelector(".lucide-globe")).toBeNull();
  });

  it("時間が無い公開の動画では、同じ面に地球の印だけを出す", () => {
    renderAs(
      "owner",
      video({ public: true, durationMs: undefined, width: undefined, height: undefined }),
    );
    const face = screen.getByText("Public").parentElement!;
    expect(face.textContent).toBe("Public");
  });

  it("ゲストには印を出さない", () => {
    const { container } = renderAs("guest", video({ public: true }));
    expect(screen.queryByText("Public")).toBeNull();
    expect(container.querySelector(".lucide-globe")).toBeNull();
  });

  it("リスト表示の行では時間の直前に同じ印を置く", () => {
    render(
      <MemoryRouter>
        <AudienceProvider audience="owner">
          <table>
            <tbody>
              <VideoRow
                video={video({ public: true })}
                backTo="/"
                selected={false}
                selectionMode={false}
              />
            </tbody>
          </table>
        </AudienceProvider>
      </MemoryRouter>,
    );
    const cell = screen.getByText("Public").closest("td")!;
    expect(cell.textContent).toBe("Public1:00");
    expect(cell.querySelector(".lucide-globe")).not.toBeNull();
  });
});

describe("VideoCard のバージョンの印", () => {
  afterEach(() => cleanup());

  const bundled = { versions: { count: 3, representativeId: 1 } };

  function renderAs(audience: Audience, item: Video) {
    return render(
      <MemoryRouter>
        <AudienceProvider audience={audience}>
          <VideoCard video={item} backTo="/" selected={false} selectionMode={false} />
        </AudienceProvider>
      </MemoryRouter>,
    );
  }

  it("2 本以上なら時間の面の先頭に Layers を置き、リンクの説明を「3 versions」にする", () => {
    const { container } = renderAs("owner", video({ ...bundled, public: true }));
    const text = screen.getByText("3 versions");
    expect(text.className).toContain("sr-only");
    const face = text.parentElement!;
    expect(face.firstElementChild?.getAttribute("class")).toContain("lucide-layers");
    expect(face.textContent).toBe("3 versionsPublic1:00");
    const link = screen.getByRole("link", { name: "動画 1" });
    expect(link.getAttribute("aria-describedby")).toBe(text.id);
    expect(container.querySelectorAll(".lucide-layers")).toHaveLength(1);
  });

  it("ゲストにも同じ印を出す", () => {
    renderAs("guest", video(bundled));
    expect(screen.getByText("3 versions").parentElement!.textContent).toBe(
      "3 versions1:00",
    );
  });

  it("見せるバージョンが 1 本か、集まりでなければ印も説明も無い", () => {
    for (const item of [
      video({ versions: { count: 1, representativeId: 1 } }),
      video(),
    ]) {
      const { container, unmount } = renderAs("owner", item);
      expect(container.querySelector(".lucide-layers")).toBeNull();
      expect(
        screen.getByRole("link", { name: "動画 1" }).hasAttribute("aria-describedby"),
      ).toBe(false);
      unmount();
    }
  });

  it("時間がまだ無くても、印だけの面を出す", () => {
    renderAs(
      "owner",
      video({ ...bundled, durationMs: undefined, width: undefined, height: undefined }),
    );
    expect(screen.getByText("3 versions").parentElement!.textContent).toBe("3 versions");
  });

  it("リスト表示の行では公開の印の前に置く", () => {
    render(
      <MemoryRouter>
        <AudienceProvider audience="owner">
          <table>
            <tbody>
              <VideoRow
                video={video({ ...bundled, public: true })}
                backTo="/"
                selected={false}
                selectionMode={false}
              />
            </tbody>
          </table>
        </AudienceProvider>
      </MemoryRouter>,
    );
    const cell = screen.getByText("3 versions").closest("td")!;
    expect(cell.textContent).toBe("3 versionsPublic1:00");
    expect(cell.querySelector(".lucide-layers")).not.toBeNull();
  });
});

describe("VideoCard の表示（issue 308）", () => {
  afterEach(() => cleanup());

  const watched = video({
    title: "見終えた動画",
    durationMs: 65_000,
    progress: { positionMs: 0, completed: true, updatedAt: "" },
    sizeBytes: 5 * 1024 * 1024,
  });

  const variants: [string, Partial<React.ComponentProps<typeof VideoCard>>][] = [
    ["Library", { tagsRow: () => null }],
    ["フォルダ画面", { onSelect: undefined }],
    [
      "Search results",
      { onSelect: undefined, location: { label: "A/B", title: "/m/A/B" } },
    ],
  ];

  it.each(variants)(
    "%s のカードに追加日時・サイズ・コーデック・解像度・視聴済みのマークを出さない",
    (_, props) => {
      renderCard(watched, props);
      const article = screen.getByRole("article");
      expect(article.textContent).not.toMatch(/h264/i);
      expect(article.textContent).not.toMatch(/1080p/i);
      expect(article.textContent).not.toMatch(/MB|KB/);
      expect(article.textContent).not.toMatch(/ ago|just now/);
      expect(screen.queryByText("Watched")).toBeNull();
      expect(screen.getByText("1:05")).toBeDefined();
    },
  );
});

describe("VideoCard のスクラブの帯", () => {
  const play = vi.fn(() => Promise.resolve());
  const pause = vi.fn();
  const sprite: SeekThumbnailSprite = {
    intervalMs: 1_000,
    frameCount: 60,
    columns: 10,
    rows: 10,
    frameWidth: 160,
    frameHeight: 90,
    sheets: ["/api/videos/1/seek-thumbnail/0?v=c"],
  };
  let resolveSprite: ((value: SeekThumbnailSprite) => void) | undefined;
  let resolveSheet: ((value: Blob) => void) | undefined;

  function scrubVideo(extra: Partial<Video> = {}): Video {
    return video({
      playable: true,
      seekThumbnailUrl: "/api/videos/1/seek-thumbnail?v=c",
      ...extra,
    });
  }

  const band = () => document.querySelector<HTMLElement>("[data-scrub-band]");
  const timeBox = () => screen.getByText(/^\d+:\d\d( \/ \d+:\d\d)?$/);
  const scrubBar = () => document.querySelector<HTMLElement>("[data-scrub-bar]");

  // React derives enter/leave from pointerout between elements; dispatch it from the link to the band.
  function moveFromLinkToBand(element: HTMLElement, clientX: number) {
    fireEvent.pointerOut(screen.getByRole("link"), {
      pointerType: "mouse",
      clientX,
      relatedTarget: element,
    });
  }

  function enterBand(clientX: number) {
    const element = band();
    if (element === null) throw new Error("no band");
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    moveFromLinkToBand(element, clientX);
    return element;
  }

  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(play);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(pause);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      left: 100,
      width: 400,
      right: 500,
      top: 200,
      bottom: 220,
      height: 20,
      x: 100,
      y: 200,
      toJSON: () => ({}),
    });
    vi.mocked(fetchSeekThumbnailSprite).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSprite = resolve;
        }),
    );
    vi.mocked(fetchSeekThumbnailSheet).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSheet = resolve;
        }),
    );
    Object.defineProperty(URL, "createObjectURL", {
      value: vi.fn(() => "blob:sheet"),
      configurable: true,
    });
    Object.defineProperty(URL, "revokeObjectURL", { value: vi.fn(), configurable: true });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.mocked(fetchSeekThumbnailSprite).mockReset();
    vi.mocked(fetchSeekThumbnailSheet).mockReset();
    vi.useRealTimers();
  });

  it("帯にいる間だけ時刻とバーが差し替わり、帯を出ると戻る", () => {
    renderCard(
      scrubVideo({ progress: { positionMs: 15_000, completed: false, updatedAt: "" } }),
    );
    const progress = screen.getByRole("progressbar");
    expect(timeBox().textContent).toBe("1:00");
    expect(scrubBar()).toBeNull();

    const element = enterBand(300);
    expect(timeBox().textContent).toBe("0:30 / 1:00");
    expect(scrubBar()?.getAttribute("aria-hidden")).toBe("true");
    expect(scrubBar()?.firstElementChild?.getAttribute("style")).toContain("width: 50%");
    expect(progress.classList.contains("opacity-0")).toBe(true);
    expect(progress.getAttribute("aria-valuenow")).toBe("25");

    fireEvent.pointerMove(element, { pointerType: "mouse", clientX: 500 });
    expect(timeBox().textContent).toBe("0:59 / 1:00");
    expect(scrubBar()?.firstElementChild?.getAttribute("style")).toContain("width: 100%");

    fireEvent.pointerLeave(element, {
      pointerType: "mouse",
      relatedTarget: screen.getByRole("link"),
    });
    expect(timeBox().textContent).toBe("1:00");
    expect(scrubBar()).toBeNull();
    expect(progress.classList.contains("opacity-0")).toBe(false);
  });

  it("未視聴の動画でも帯にいる間はバーを出し、コマは揃ってから出す", async () => {
    renderCard(scrubVideo());
    expect(screen.queryByRole("progressbar")).toBeNull();
    enterBand(300);
    expect(scrubBar()).not.toBeNull();
    expect(document.querySelector("[data-scrub-frame]")).toBeNull();

    await act(async () => resolveSprite?.(sprite));
    expect(fetchSeekThumbnailSheet).toHaveBeenCalledTimes(1);
    await act(async () => resolveSheet?.(new Blob()));
    const frame = document.querySelector("[data-scrub-frame]");
    expect(frame?.closest("[data-preview-media]")).not.toBeNull();
    expect(frame?.getAttribute("aria-hidden")).toBe("true");
  });

  it("ループ再生中に帯へ入ると止め、帯から上へ出ると再開する", () => {
    renderCard(scrubVideo());
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(play).toHaveBeenCalledTimes(1);

    const element = band();
    if (element === null) throw new Error("no band");
    moveFromLinkToBand(element, 200);
    expect(pause).toHaveBeenCalledTimes(1);
    fireEvent.pointerLeave(element, {
      pointerType: "mouse",
      relatedTarget: screen.getByRole("link"),
    });
    expect(play).toHaveBeenCalledTimes(2);
  });

  it("帯から下へカードの外に出ると、ループを再開せずに解放する", () => {
    renderCard(scrubVideo());
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    const element = band();
    if (element === null) throw new Error("no band");
    moveFromLinkToBand(element, 200);
    fireEvent.pointerLeave(element, {
      pointerType: "mouse",
      relatedTarget: document.body,
    });
    fireEvent.pointerLeave(screen.getByRole("article"), { pointerType: "mouse" });
    expect(play).toHaveBeenCalledTimes(1);
    expect(document.querySelector("video")).toBeNull();
    expect(scrubBar()).toBeNull();
  });

  it("並び替えなどの reset と別のカードのプレビューの開始で、帯から出たのと同じに戻る", () => {
    const { previewCoordinator } = coordinator();
    renderCard(scrubVideo(), { previewCoordinator });
    enterBand(300);
    expect(timeBox().textContent).toBe("0:30 / 1:00");
    act(() => previewCoordinator.reset());
    expect(timeBox().textContent).toBe("1:00");
    expect(scrubBar()).toBeNull();

    enterBand(300);
    expect(scrubBar()).not.toBeNull();
    act(() => previewCoordinator.start(2));
    expect(scrubBar()).toBeNull();
  });

  it("帯の上のクリックは通常どおり再生画面へ遷移し、開始位置を運ばない", () => {
    function Player() {
      const location = useLocation();
      return (
        <p>{`player search=[${location.search}] state=${JSON.stringify(location.state)}`}</p>
      );
    }
    render(
      <MemoryRouter>
        <Routes>
          <Route
            path="/"
            element={
              <VideoCard
                video={scrubVideo()}
                backTo="/"
                selected={false}
                selectionMode={false}
                onSelect={vi.fn()}
              />
            }
          />
          <Route path="/videos/:id" element={<Player />} />
        </Routes>
      </MemoryRouter>,
    );
    const element = enterBand(300);
    fireEvent.click(element);
    // Without seek or autoplay overrides, playback resumes from the stored position.
    expect(screen.getByText('player search=[] state={"from":"/"}')).not.toBeNull();
  });

  it("帯は aria-hidden で、リンクの読み上げ名を変えない", () => {
    renderCard(scrubVideo());
    const element = enterBand(300);
    expect(element.getAttribute("aria-hidden")).toBe("true");
    expect(element.closest("a")).not.toBeNull();
    expect(element.closest("[data-preview-media]")).toBeNull();
    expect(screen.getByRole("link").getAttribute("aria-label")).toBe("動画 1");
  });

  it("選択モード・警告の出る動画・スプライトや長さの無い動画・リスト表示には帯が無い", () => {
    const cases: Array<[Video, boolean]> = [
      [scrubVideo(), true],
      [scrubVideo({ playable: false, probeState: "failed" }), false],
      [scrubVideo({ seekThumbnailUrl: undefined }), false],
      [scrubVideo({ durationMs: 0 }), false],
    ];
    for (const [item, expected] of cases) {
      renderCard(item);
      expect(band() !== null).toBe(expected);
      cleanup();
    }

    renderCard(scrubVideo(), { selectionMode: true });
    expect(band()).toBeNull();
    cleanup();

    render(
      <MemoryRouter>
        <table>
          <tbody>
            <VideoRow
              video={scrubVideo()}
              backTo="/"
              selected={false}
              selectionMode={false}
              onSelect={vi.fn()}
            />
          </tbody>
        </table>
      </MemoryRouter>,
    );
    expect(band()).toBeNull();
  });

  it("帯にいる間に選択モードに入ると、帯ごと外して時刻を戻す", () => {
    const { rerender } = renderCard(scrubVideo());
    enterBand(300);
    expect(timeBox().textContent).toBe("0:30 / 1:00");
    rerender(
      <MemoryRouter>
        <VideoCard
          video={scrubVideo()}
          backTo="/"
          selected={false}
          selectionMode
          onSelect={vi.fn()}
        />
      </MemoryRouter>,
    );
    expect(band()).toBeNull();
    expect(timeBox().textContent).toBe("1:00");
  });
});

describe("VideoCard の操作ボタン", () => {
  beforeEach(() => {
    // A device with hover, where the actions show only on approach.
    vi.stubGlobal("matchMedia", (query: string) => ({ matches: false, media: query }));
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  function renderOwner(item: Video) {
    return render(
      <MemoryRouter>
        <AudienceProvider audience="owner">
          <VideoCard video={item} backTo="/" selected={false} selectionMode={false} />
        </AudienceProvider>
      </MemoryRouter>,
    );
  }

  it("ポインターが乗るか中にフォーカスが来るまで作らず、来たら作る", () => {
    renderOwner(video());
    const playlist = () =>
      screen.queryByRole("button", { name: 'Add "動画 1" to a playlist' });
    expect(playlist()).toBeNull();
    expect(screen.queryByRole("button", { name: 'Favorite "動画 1"' })).toBeNull();

    fireEvent.pointerEnter(screen.getByRole("article"));
    expect(playlist()).not.toBeNull();

    cleanup();
    renderOwner(video());
    fireEvent.focus(screen.getByRole("link", { name: "動画 1" }));
    expect(playlist()).not.toBeNull();
  });

  it("お気に入りのハートは触れる前から出す", () => {
    renderOwner(video({ favorite: true }));
    expect(screen.getByRole("button", { name: 'Favorite "動画 1"' })).toBeDefined();
    expect(screen.queryByRole("button", { name: 'Add "動画 1" to a group' })).toBeNull();
  });
});
