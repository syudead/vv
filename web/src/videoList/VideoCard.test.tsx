import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { type Audience, AudienceProvider } from "../auth/audience";
import VideoCard, { VideoRow } from "./VideoCard";

function video(extra: Partial<Video> = {}): Video {
  return {
    id: 1,
    title: "動画 1",
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
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
    expect(screen.queryByText("再生に必要な情報がありません")).toBeNull();

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
    const onStart = vi.fn();
    const { rerender } = renderCard(video(), { onPreviewStart: onStart });
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
          onPreviewStart={onStart}
        />
      </MemoryRouter>,
    );
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(onStart).not.toHaveBeenCalled();
    expect(document.querySelector("video")).toBeNull();
  });

  it("returns to the thumbnail on leave, media error, or play rejection and retries", async () => {
    const onStart = vi.fn();
    renderCard(video(), { onPreviewStart: onStart });
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
    const { rerender } = renderCard(video(), { activePreviewId: 1 });
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

    rerender(
      <MemoryRouter>
        <VideoCard
          video={video()}
          backTo="/"
          selected={false}
          selectionMode={false}
          onSelect={vi.fn()}
          activePreviewId={1}
          previewResetEpoch={1}
        />
      </MemoryRouter>,
    );
    expect(document.querySelector("video")).toBeNull();
    expect(pause).toHaveBeenCalled();
    expect(load).toHaveBeenCalled();
  });

  it("removes an inactive coordinated preview in the active-card render", async () => {
    const item = video();
    const onStart = vi.fn();
    const rendered = renderCard(item, {
      activePreviewId: item.id,
      onPreviewStart: onStart,
    });
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    const preview = document.querySelector("video");
    expect(preview).not.toBeNull();
    if (preview === null) return;
    fireEvent.playing(preview);
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(true);

    rendered.rerender(
      <MemoryRouter>
        <VideoCard
          video={item}
          backTo="/"
          selected={false}
          selectionMode={false}
          onSelect={vi.fn()}
          activePreviewId={2}
          onPreviewStart={onStart}
        />
      </MemoryRouter>,
    );
    expect(document.querySelector("video")).toBeNull();
    expect(document.querySelector("img")?.classList.contains("opacity-0")).toBe(false);
    await act(async () => Promise.resolve());
    expect(pause).toHaveBeenCalled();
    expect(load).toHaveBeenCalled();
  });

  it("does not suppress an existing warning when a preview URL is present", () => {
    const onStart = vi.fn();
    renderCard(video({ durationMs: undefined }), {
      activePreviewId: 1,
      onPreviewStart: onStart,
    });
    expect(screen.getByText("再生に必要な情報がありません")).toBeDefined();

    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(screen.getByText("再生に必要な情報がありません")).toBeDefined();
    expect(onStart).not.toHaveBeenCalled();
    expect(document.querySelector("video")).toBeNull();
  });

  it("releases the media element before unmount detaches its ref", () => {
    const rendered = renderCard(video(), { activePreviewId: 1 });
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelector("video")).not.toBeNull();

    rendered.unmount();
    expect(pause).toHaveBeenCalled();
    expect(load).toHaveBeenCalled();
  });

  it("stops preview when selection mode starts and keeps card selection priority", () => {
    const onSelect = vi.fn();
    const onReset = vi.fn();
    const item = video();
    const rendered = renderCard(item, {
      activePreviewId: item.id,
      onSelect,
      onPreviewReset: onReset,
    });
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
          activePreviewId={item.id}
          onPreviewReset={onReset}
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
    renderCard(video({ tags: [{ id: 1, name: "旅行" }] }), {
      tagsRow: () => <p>タグの行</p>,
    });
    expect(screen.getByText("タグの行")).toBeDefined();
  });

  it("tagsRow はリンクの外、同じ article の中に置く", () => {
    renderCard(video({ tags: [{ id: 1, name: "旅行" }] }), {
      tagsRow: () => <button type="button">タグ</button>,
    });
    const article = screen.getByRole("article");
    const link = screen.getByRole("link");
    const button = screen.getByRole("button", { name: "タグ" });
    expect(article.contains(link)).toBe(true);
    expect(article.contains(button)).toBe(true);
    expect(link.contains(button)).toBe(false);
  });

  it("タグが無い動画では tagsRow の場所に何も出さない", () => {
    renderCard(video({ tags: [] }), { tagsRow: () => <p>タグの行</p> });
    expect(screen.queryByText("タグの行")).toBeNull();
  });

  it("題名とタグの行の間隔は今の gap-1 と同じ（B3）", () => {
    renderCard(video({ tags: [{ id: 1, name: "旅行" }] }), {
      tagsRow: () => <p>タグの行</p>,
    });
    const row = screen.getByText("タグの行").parentElement;
    expect(row?.className).toContain("pt-1");
  });

  it("同じ参照の props で親が再描画しても memo で再描画せず、tagsRow を呼び直さない（N4）", () => {
    const item = video({ tags: [{ id: 1, name: "旅行" }] });
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

    // 親を、まったく同じ props（同じ参照）で再描画する（無関係な状態が変わった
    // ことを模す）。memo(VideoCard) が効いていれば、VideoCard は再描画されず、
    // tagsRow はもう一度呼ばれない。
    rerender(
      <MemoryRouter>
        <VideoCard {...props} />
      </MemoryRouter>,
    );
    expect(stableTagsRow).toHaveBeenCalledTimes(1);

    // tagsRow の参照が変われば、当然に再描画され、新しい関数が呼ばれる。
    const nextTagsRow = vi.fn(() => <p>タグの行</p>);
    rerender(
      <MemoryRouter>
        <VideoCard {...props} tagsRow={nextTagsRow} />
      </MemoryRouter>,
    );
    expect(nextTagsRow).toHaveBeenCalledTimes(1);
  });
});

// 公開の印（specs/016-single-account-auth/ui-design.md「Visibility toggle」の「Card」、issue 305）。
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
    const mark = screen.getByText("公開");
    expect(mark.className).toContain("sr-only");
    const face = mark.parentElement!;
    // 先頭が地球のアイコン、その後に時間。
    expect(face.firstElementChild?.tagName.toLowerCase()).toBe("svg");
    expect(face.firstElementChild?.getAttribute("class")).toContain("lucide-globe");
    expect(face.textContent).toBe("公開1:00");
    expect(container.querySelectorAll(".lucide-globe")).toHaveLength(1);
  });

  it("非公開の動画には印を出さない", () => {
    const { container } = renderAs("owner", video({ public: false }));
    expect(screen.queryByText("公開")).toBeNull();
    expect(container.querySelector(".lucide-globe")).toBeNull();
  });

  it("時間が無い公開の動画では、同じ面に地球の印だけを出す", () => {
    renderAs(
      "owner",
      video({ public: true, durationMs: undefined, width: undefined, height: undefined }),
    );
    const face = screen.getByText("公開").parentElement!;
    expect(face.textContent).toBe("公開");
  });

  it("ゲストには印を出さない", () => {
    const { container } = renderAs("guest", video({ public: true }));
    expect(screen.queryByText("公開")).toBeNull();
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
    const cell = screen.getByText("公開").closest("td")!;
    expect(cell.textContent).toBe("公開1:00");
    expect(cell.querySelector(".lucide-globe")).not.toBeNull();
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

  // ライブラリ（tagsRow あり）・フォルダ画面（なし）・検索結果（置き場所あり）の3通り。
  const variants: [string, Partial<React.ComponentProps<typeof VideoCard>>][] = [
    ["ライブラリ", { tagsRow: () => null }],
    ["フォルダ画面", { onSelect: undefined }],
    ["検索結果", { onSelect: undefined, location: { label: "A/B", title: "/m/A/B" } }],
  ];

  it.each(variants)(
    "%s のカードに追加日時・サイズ・コーデック・解像度・視聴済みのマークを出さない",
    (_, props) => {
      renderCard(watched, props);
      const article = screen.getByRole("article");
      expect(article.textContent).not.toMatch(/h264/i);
      expect(article.textContent).not.toMatch(/1080p/i);
      expect(article.textContent).not.toMatch(/MB|KB/);
      expect(article.textContent).not.toMatch(/前|たった今/);
      expect(screen.queryByText("視聴済み")).toBeNull();
      // 再生時間は残す。
      expect(screen.getByText("1:05")).toBeDefined();
    },
  );
});
