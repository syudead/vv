import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { useEffect, useState } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type RelatedVideos, setRenderedAudience, type Video } from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import type { Playlist, PlaylistPlayback, PlaylistStep } from "../api/playlists";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import type { PlayerControls } from "./playerControls";
import type { PlayerStatus } from "./VideoPlayer";
import VideoPage from "./VideoPage";

// Page tests cover routing and response coordination; playlistPlayback.test.ts covers the underlying rules.

interface PlayerProps {
  video: Video;
  initialPositionMs: number;
  autoplay: boolean;
  onControls: (controls: PlayerControls | null) => void;
  onStatus: (status: PlayerStatus) => void;
}

const playerMock = vi.hoisted(() => ({
  props: undefined as PlayerProps | undefined,
  mounts: 0,
}));

vi.mock("./VideoPlayer", () => ({
  default: function VideoPlayerMock(props: PlayerProps) {
    playerMock.props = props;
    const [initial] = useState(props);
    useEffect(() => {
      playerMock.mounts += 1;
      return () => initial.onControls(null);
    }, [initial]);
    return <div data-testid="video-player" />;
  },
  canStartPlayback: (video: Video) =>
    video.probeState === "done" &&
    (video.durationMs ?? 0) > 0 &&
    video.videoCodec !== undefined,
  initialPlayerStatus: {
    loading: false,
    reconnecting: false,
    playing: false,
    userActive: true,
    ended: false,
    stalled: false,
    positioned: false,
  },
}));

const base: Video = {
  id: 0,
  virtual: false,
  title: "",
  public: false,
  sizeBytes: 1_000,
  addedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  fileCreatedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  seekThumbnailState: "done",
  durationMs: 60_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  audioCodec: "aac",
  tags: [],
  favorite: false,
  progress: { positionMs: 30_000, completed: false, updatedAt: "2026-09-02T00:00:00Z" },
};

function videoOf(id: number, title: string): Video {
  return { ...base, id, title };
}

const first = videoOf(11, "first");
const second = videoOf(12, "second");
const third = videoOf(13, "third");
const fourth = videoOf(14, "fourth");

const early = "2026-10-01T00:00:00Z";
const late = "2026-10-01T00:00:09Z";

function step(itemId: number, video: Video): PlaylistStep {
  return { itemId, video };
}

function summary(updatedAt = early) {
  return { id: 5, name: "Tonight", itemCount: 3, updatedAt };
}

function playbackFor(itemId: number, updatedAt = early): PlaylistPlayback {
  const order = [step(21, first), step(22, second), step(23, third)];
  const index = order.findIndex((entry) => entry.itemId === itemId);
  return {
    playlist: summary(updatedAt),
    itemId,
    position: index + 1,
    count: order.length,
    next: order[index + 1],
    previousItem: order[index - 1],
    nextItem: order[index + 1],
  };
}

function playlistOf(videos: readonly [number, Video][], updatedAt = early): Playlist {
  return {
    ...summary(updatedAt),
    itemCount: videos.length,
    items: videos.map(([id, video]) => ({ id, kind: "video" as const, video })),
  };
}

const defaultPlaylist = playlistOf([
  [21, first],
  [22, second],
  [23, third],
]);

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const notFound = () => json({ code: "not_found", message: "x" }, 404);

const server = {
  videos: new Map<number, () => Response>(),
  related: new Map<number, RelatedVideos>(),
  playback: vi.fn<(item: number, video: number) => Response>(),
  playlist: vi.fn<() => Response>(),
};

function LocationProbe() {
  const location = useLocation();
  return <p data-testid="location">{location.pathname + location.search}</p>;
}

function Screen({ name }: { name: string }) {
  return <p data-testid="screen">{name}</p>;
}

function renderPage(
  path = "/videos/12?playlist=5&item=22",
  audience: Audience = "owner",
) {
  const [pathname, search] = path.split("?");
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter
          initialEntries={[
            {
              pathname,
              search: search === undefined ? "" : `?${search}`,
              state: { from: "/playlists/5" },
            },
          ]}
        >
          <AudienceProvider audience={audience}>
            <LocationProbe />
            <Routes>
              <Route path="/videos/:id" element={<VideoPage />} />
              <Route path="/playlists/:id" element={<Screen name="playlist" />} />
            </Routes>
          </AudienceProvider>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

function player(): PlayerProps {
  const props = playerMock.props;
  if (props === undefined) throw new Error("player が描画されていません");
  return props;
}

function setStatus(ended: boolean, playing = false) {
  act(() =>
    player().onStatus({
      loading: false,
      reconnecting: false,
      playing,
      userActive: true,
      ended,
      stalled: false,
      positioned: true,
    }),
  );
}

const location = () => screen.getByTestId("location").textContent;
const line = () => screen.findByRole("link", { name: /^Playlist "Tonight", item/ });
const panel = () =>
  document.querySelector<HTMLElement>("[data-playlist-panel]") ??
  (() => {
    throw new Error("パネルがありません");
  })();
const rowTitle = (row: HTMLElement) =>
  ["first", "second", "third", "fourth"].find((name) => row.textContent?.endsWith(name));
const panelRows = () =>
  within(
    screen.getByRole("list", { name: 'Playlist "Tonight". Open the playlist' }),
  ).getAllByRole("listitem");

describe("VideoPage のプレイリストの文脈", () => {
  const fetchMock = vi.fn<typeof fetch>();

  const calls = (prefix: string) =>
    fetchMock.mock.calls.filter(([input]) => String(input).startsWith(prefix)).length;

  beforeEach(() => {
    playerMock.props = undefined;
    playerMock.mounts = 0;
    server.videos.clear();
    server.related.clear();
    for (const entry of [first, second, third, fourth])
      server.videos.set(entry.id, () => json(entry));
    server.related.set(12, {
      items: [videoOf(31, "related-next"), videoOf(32, "related-previous")],
      nextId: 31,
      prevId: 32,
    });
    server.playback.mockReset();
    server.playback.mockImplementation((item) => json(playbackFor(item)));
    server.playlist.mockReset();
    server.playlist.mockImplementation(() => json(defaultPlaylist));
    fetchMock.mockReset();
    installFakeEventSource();
    setRenderedAudience(null);
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url === "/api/auth/session") return Promise.resolve(json({ state: "owner" }));
      if (url === "/api/tags") return Promise.resolve(json([]));
      const playback = /^\/api\/playlists\/5\/playback\?item=(\d+)&video=(\d+)$/.exec(
        url,
      );
      if (playback !== null)
        return Promise.resolve(server.playback(Number(playback[1]), Number(playback[2])));
      if (url === "/api/playlists/5") return Promise.resolve(server.playlist());
      const match = /^\/api\/videos\/(\d+)(\/[a-z-]+)?(?:\?smartFolder=\d+)?$/.exec(url);
      if (match === null) return Promise.resolve(json({}));
      const id = Number(match[1]);
      const suffix = match[2];
      if (suffix === "/subtitles") return Promise.resolve(json({ subtitles: [] }));
      if (suffix === "/related")
        return Promise.resolve(json(server.related.get(id) ?? { items: [] }));
      if (suffix !== undefined) return Promise.resolve(json({}));
      return Promise.resolve(server.videos.get(id)?.() ?? notFound());
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("題名の上にプレイリスト名と位置の行を出し、プレイリストの画面を開く", async () => {
    renderPage();
    const link = await line();
    expect(link.textContent).toBe("Tonight·2 / 3");
    expect(link.getAttribute("aria-label")).toBe(
      'Playlist "Tonight", item 2 of 3. Open the playlist',
    );
    expect(calls("/api/playlists/5/playback?item=22&video=12")).toBe(1);
    fireEvent.click(link);
    expect(screen.getByTestId("screen").textContent).toBe("playlist");
  });

  it("矢印は previousItem と nextItem を開き、関連動画の前後は使わない", async () => {
    renderPage();
    await line();
    expect(screen.queryByRole("button", { name: /^Previous video/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Next item: third" }));
    expect(location()).toBe("/videos/13?playlist=5&item=23");
    await screen.findByRole("button", { name: "Previous item: second" });
    fireEvent.click(screen.getByRole("button", { name: "Previous item: second" }));
    expect(location()).toBe("/videos/12?playlist=5&item=22");
  });

  it("`?group` があってもプレイリストの文脈を優先し、読み取りに group を運ばない", async () => {
    renderPage("/videos/12?playlist=5&item=22&group=9");
    await line();
    expect(calls("/api/playlists/5/playback?item=22&video=12")).toBe(1);
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes("group="))).toBe(
      false,
    );
    fireEvent.click(screen.getByRole("button", { name: "Next item: third" }));
    expect(location()).toBe("/videos/13?playlist=5&item=23");
  });

  it("最初の項目に前の矢印は無い", async () => {
    renderPage("/videos/11?playlist=5&item=21");
    await line();
    expect(screen.queryByRole("button", { name: /^Previous item/ })).toBeNull();
    expect(screen.getByRole("button", { name: "Next item: second" })).toBeDefined();
  });

  it("終わりで再生の応答を読み直し、予告がその next を示し、進むと item をその項目にして開く", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderPage();
    await line();
    server.playback.mockImplementation(() =>
      json({
        ...playbackFor(22, late),
        next: step(24, fourth),
        nextItem: step(24, fourth),
      }),
    );
    server.playlist.mockImplementation(() =>
      json(
        playlistOf(
          [
            [21, first],
            [22, second],
            [24, fourth],
            [23, third],
          ],
          late,
        ),
      ),
    );
    setStatus(true);
    expect(await screen.findByText("Up next")).toBeDefined();
    expect(screen.getByRole("link", { name: /^fourth/ }).getAttribute("href")).toBe(
      "/videos/14?playlist=5&item=24",
    );
    expect(calls("/api/playlists/5/playback")).toBe(2);
    expect(panelRows().map((row) => rowTitle(row))).toEqual([
      "first",
      "second",
      "fourth",
      "third",
    ]);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    await waitFor(() => expect(location()).toBe("/videos/14?playlist=5&item=24"));
    await waitFor(() => expect(player().video.id).toBe(14));
    expect(player().autoplay).toBe(true);
  });

  it("予告を取り消すと進まず、「Play next」と「Watch again」の層を出す", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderPage();
    await line();
    setStatus(true);
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("button", { name: "Play next" })).toBeDefined();
    expect(screen.getByRole("button", { name: "Watch again" })).toBeDefined();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6_000);
    });
    expect(location()).toBe("/videos/12?playlist=5&item=22");
    fireEvent.click(screen.getByRole("button", { name: "Play next" }));
    expect(location()).toBe("/videos/13?playlist=5&item=23");
  });

  it("予告が次の動画の消えたのを知ると読み直し、新しい next に移る。無ければ終わりの層を出す", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderPage();
    await line();
    server.videos.set(13, notFound);
    setStatus(true);
    await screen.findByRole("button", { name: "Cancel" });
    server.playback.mockImplementation(() =>
      json({ ...playbackFor(22), next: step(24, fourth), nextItem: step(24, fourth) }),
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    await waitFor(() =>
      expect(screen.getByRole("link", { name: /^fourth/ }).getAttribute("href")).toBe(
        "/videos/14?playlist=5&item=24",
      ),
    );
    expect(calls("/api/playlists/5/playback")).toBe(3);
    server.videos.set(14, notFound);
    server.playback.mockImplementation(() =>
      json({ ...playbackFor(22), next: undefined, nextItem: undefined }),
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    expect(await screen.findByText("End of playlist")).toBeDefined();
    expect(location()).toBe("/videos/12?playlist=5&item=22");
  });

  it("next が無ければ終わりの層を出し、何も進めない", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderPage("/videos/13?playlist=5&item=23");
    await line();
    setStatus(true);
    expect(await screen.findByText("End of playlist")).toBeDefined();
    expect(
      screen.getByText('Playback finished. End of the playlist "Tonight"'),
    ).toBeDefined();
    expect(screen.queryByRole("button", { name: "Play next" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
    expect(screen.getByRole("button", { name: "Watch again" })).toBeDefined();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6_000);
    });
    expect(location()).toBe("/videos/13?playlist=5&item=23");
  });

  it("動画が同じで別の項目へ進むと、その動画を始めから再生し直し、行に新しい位置を出す", async () => {
    // The same video appears both as item 22 and as the first member of group item 23.
    server.playback.mockImplementation((item) =>
      item === 22
        ? json({ ...playbackFor(22), next: step(23, second), nextItem: step(23, second) })
        : json({ ...playbackFor(23), previousItem: step(22, second) }),
    );
    renderPage();
    await line();
    const mounts = playerMock.mounts;
    setStatus(true);
    fireEvent.click(await screen.findByRole("button", { name: "Play now" }));
    expect(location()).toBe("/videos/12?playlist=5&item=23");
    await waitFor(() => expect(playerMock.mounts).toBeGreaterThan(mounts));
    expect(player().initialPositionMs).toBe(0);
    expect(player().autoplay).toBe(true);
    await waitFor(async () => expect((await line()).textContent).toBe("Tonight·3 / 3"));
    expect(screen.queryByText("Up next")).toBeNull();
  });

  it("404 では文脈を落とし、関連動画の前後に戻り、終わった後に予告も終わりの層も出さない", async () => {
    server.playback.mockImplementation(notFound);
    renderPage();
    expect(
      await screen.findByText(
        "This video is no longer in the playlist. Playback stops after it.",
      ),
    ).toBeDefined();
    expect(
      await screen.findByRole("button", { name: "Next video: related-next" }),
    ).toBeDefined();
    expect(screen.queryByRole("link", { name: /^Playlist "Tonight"/ })).toBeNull();
    expect(document.querySelector("[data-playlist-panel]")).toBeNull();
    setStatus(true);
    expect(screen.getByRole("button", { name: "Watch again" })).toBeDefined();
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Play next" })).toBeNull();
    expect(screen.queryByText("End of playlist")).toBeNull();
    expect(calls("/api/playlists/5/playback")).toBe(1);
    expect(location()).toBe("/videos/12?playlist=5&item=22");
  });

  it("プレイリストの読みの 404 でも文脈を落とし、パネルを出さない", async () => {
    server.playlist.mockImplementation(notFound);
    renderPage();
    expect(
      await screen.findByText(
        "This video is no longer in the playlist. Playback stops after it.",
      ),
    ).toBeDefined();
    expect(document.querySelector("[data-playlist-panel]")).toBeNull();
    expect(screen.queryByRole("button", { name: /item/ })).toBeNull();
  });

  it("ゲストは再生もプレイリストも読まず、行もパネルも出さずに公開の動画を再生する", async () => {
    server.videos.set(12, () => json({ ...second, public: true, favorite: undefined }));
    renderPage("/videos/12?playlist=1&item=2", "guest");
    await screen.findByRole("heading", { level: 1 });
    await screen.findByRole("button", { name: "Next video: related-next" });
    expect(calls("/api/playlists")).toBe(0);
    expect(screen.queryByRole("link", { name: /^Playlist/ })).toBeNull();
    expect(document.querySelector("[data-playlist-panel]")).toBeNull();
  });

  it("aside の先頭にパネルを出し、再生中の行は印を付けてリンクにせず、ほかの行はその項目を開く", async () => {
    renderPage();
    await line();
    const heading = within(panel()).getByRole("heading", { level: 2 });
    expect(heading.textContent).toBe("Tonight");
    expect(
      within(heading)
        .getByRole("link", { name: 'Playlist "Tonight". Open the playlist' })
        .getAttribute("href"),
    ).toBe("/playlists/5");
    expect(within(panel()).getByText("2 / 3")).toBeDefined();
    const aside = document.querySelector('[data-slot="detail-page-aside"]');
    expect(aside?.firstElementChild?.firstElementChild).toBe(panel());
    const rows = panelRows();
    expect(rows.map((row) => row.getAttribute("aria-current"))).toEqual([
      null,
      "true",
      null,
    ]);
    expect(within(rows[1]!).queryByRole("link")).toBeNull();
    expect(within(rows[1]!).getByText("Now playing")).toBeDefined();
    const third = within(rows[2]!).getByRole("link", { name: "Play from third" });
    expect(third.getAttribute("href")).toBe("/videos/13?playlist=5&item=23");
    fireEvent.click(third);
    expect(location()).toBe("/videos/13?playlist=5&item=23");
  });

  it("グループの項目の中では、Up next のメンバーの行がプレイリストの文脈を保って開く", async () => {
    const member1 = videoOf(41, "member-one");
    const member3 = videoOf(43, "member-three");
    const group = {
      id: 1,
      name: "series",
      items: [member1, second, member3],
      offset: 0,
      total: 3,
    };
    server.videos.set(41, () => json(member1));
    server.related.set(12, { items: [videoOf(31, "related-next")], group });
    server.playback.mockImplementation(() =>
      json({ ...playbackFor(22), next: step(22, member3) }),
    );
    renderPage();
    await line();
    const memberLink = await screen.findByRole("link", { name: /^member-three/ });
    expect(memberLink.getAttribute("href")).toBe("/videos/43?playlist=5&item=22");
    expect(screen.getByRole("link", { name: /^related-next/ }).getAttribute("href")).toBe(
      "/videos/31",
    );
    setStatus(false, true);
    fireEvent.click(screen.getByRole("link", { name: /^member-one/ }));
    expect(location()).toBe("/videos/41?playlist=5&item=22");
    await waitFor(() =>
      expect(calls("/api/playlists/5/playback?item=22&video=41")).toBe(1),
    );
    expect((await line()).textContent).toBe("Tonight·2 / 3");
  });

  it("単体の動画の項目では、メンバーの行は今までどおり動画だけを開く", async () => {
    const group = {
      id: 1,
      name: "series",
      items: [videoOf(41, "member-one"), second],
      offset: 0,
      total: 2,
    };
    server.related.set(12, { items: [], group });
    renderPage();
    await line();
    expect(
      (await screen.findByRole("link", { name: /^member-one/ })).getAttribute("href"),
    ).toBe("/videos/41");
  });

  it("2 つの応答の updatedAt が違うと両方を読み直し、パネルはそろった組の並びを出す", async () => {
    let reads = 0;
    server.playlist.mockImplementation(() => {
      reads += 1;
      return reads === 1
        ? json(defaultPlaylist)
        : json(
            playlistOf(
              [
                [22, second],
                [21, first],
                [23, third],
              ],
              late,
            ),
          );
    });
    server.playback.mockImplementation(() =>
      json({ ...playbackFor(22, late), position: 1 }),
    );
    renderPage();
    await waitFor(() =>
      expect(panelRows().map((row) => rowTitle(row))).toEqual([
        "second",
        "first",
        "third",
      ]),
    );
    expect(calls("/api/playlists/5/playback")).toBe(2);
    expect(calls("/api/playlists/5")).toBe(4);
  });

  it("プレイリストの読みが失敗するとパネルに Retry を出し、行と矢印は変わらない", async () => {
    server.playlist.mockImplementation(() =>
      json({ code: "internal", message: "x" }, 500),
    );
    renderPage();
    expect(
      await within(await waitForPanel()).findByText("Couldn't load the playlist"),
    ).toBeDefined();
    expect((await line()).textContent).toBe("Tonight·2 / 3");
    expect(screen.getByRole("button", { name: "Next item: third" })).toBeDefined();
    server.playlist.mockImplementation(() => json(defaultPlaylist));
    fireEvent.click(within(panel()).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(panelRows()).toHaveLength(3));
  });

  it("Retry の再生の読みが失敗しても、パネルは読み込み中に留まらず Retry をもう一度出す", async () => {
    server.playlist.mockImplementation(() =>
      json({ code: "internal", message: "x" }, 500),
    );
    renderPage();
    await within(await waitForPanel()).findByText("Couldn't load the playlist");
    server.playback.mockImplementation(() =>
      json({ code: "internal", message: "x" }, 500),
    );
    fireEvent.click(within(panel()).getByRole("button", { name: "Retry" }));
    expect(await within(panel()).findByRole("button", { name: "Retry" })).toBeDefined();
    expect(calls("/api/playlists/5/playback")).toBe(2);
    expect((await line()).textContent).toBe("Tonight·2 / 3");
    expect(screen.getByRole("button", { name: "Next item: third" })).toBeDefined();
  });

  it("終わりの読み直しでプレイリストだけが失敗すると、前の組（パネルと予告）をそろえて保つ", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderPage();
    await line();
    server.playback.mockImplementation(() =>
      json({
        ...playbackFor(22, late),
        next: step(24, fourth),
        nextItem: step(24, fourth),
      }),
    );
    server.playlist.mockImplementation(() =>
      json({ code: "internal", message: "x" }, 500),
    );
    setStatus(true);
    expect(await screen.findByText("Up next")).toBeDefined();
    await waitFor(() => expect(calls("/api/playlists/5/playback")).toBe(2));
    expect(screen.getByRole("link", { name: /^third/ }).getAttribute("href")).toBe(
      "/videos/13?playlist=5&item=23",
    );
    expect(screen.getByRole("button", { name: "Next item: third" })).toBeDefined();
    expect(panelRows().map((row) => rowTitle(row))).toEqual(["first", "second", "third"]);
  });

  it("読み直した並びで再生中の行の位置が変わると、一覧の scrollTop を合わせ直す", async () => {
    // jsdom has no layout; derive row offsets from sibling order.
    const top = vi
      .spyOn(HTMLElement.prototype, "offsetTop", "get")
      .mockImplementation(function (this: HTMLElement) {
        return this.parentElement === null
          ? 0
          : Array.from(this.parentElement.children).indexOf(this) * 100;
      });
    const height = vi
      .spyOn(HTMLElement.prototype, "offsetHeight", "get")
      .mockReturnValue(90);
    try {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      renderPage();
      await line();
      const list = () =>
        screen.getByRole("list", { name: 'Playlist "Tonight". Open the playlist' });
      await waitFor(() => expect(list().scrollTop).toBe(60));
      server.playback.mockImplementation(() =>
        json({
          ...playbackFor(22, late),
          position: 3,
          next: undefined,
          nextItem: undefined,
        }),
      );
      server.playlist.mockImplementation(() =>
        json(
          playlistOf(
            [
              [21, first],
              [23, third],
              [22, second],
            ],
            late,
          ),
        ),
      );
      setStatus(true);
      await waitFor(() =>
        expect(panelRows().map((row) => rowTitle(row))).toEqual([
          "first",
          "third",
          "second",
        ]),
      );
      expect(list().scrollTop).toBe(160);
    } finally {
      top.mockRestore();
      height.mockRestore();
    }
  });

  it("クエリが無ければ、関連動画の前後と今までの終わりの層のまま", async () => {
    renderPage("/videos/12");
    await screen.findByRole("button", { name: "Next video: related-next" });
    expect(calls("/api/playlists")).toBe(0);
    expect(document.querySelector("[data-playlist-panel]")).toBeNull();
    setStatus(true);
    expect(screen.getByRole("button", { name: "Play next" })).toBeDefined();
  });

  it("再生できない項目に自動再生で着くと、次の項目へ飛ばしてトーストで伝える", async () => {
    server.videos.set(12, () =>
      json({ ...second, probeState: "failed", playable: false }),
    );
    render(
      <TooltipProvider>
        <ToastProvider>
          <MemoryRouter
            initialEntries={[
              {
                pathname: "/videos/12",
                search: "?playlist=5&item=22",
                state: { from: "/playlists/5", autoplay: true },
              },
            ]}
          >
            <AudienceProvider audience="owner">
              <LocationProbe />
              <Routes>
                <Route path="/videos/:id" element={<VideoPage />} />
              </Routes>
            </AudienceProvider>
          </MemoryRouter>
        </ToastProvider>
      </TooltipProvider>,
    );
    expect(
      await screen.findByText('Skipped "second": it can\'t be played here.'),
    ).toBeDefined();
    await waitFor(() => expect(location()).toBe("/videos/13?playlist=5&item=23"));
    await waitFor(() => expect(player().video.id).toBe(13));
    expect(player().autoplay).toBe(true);
  });

  it("飛ばした再生できない項目に自動再生でもう一度着くと、もう一度飛ばす", async () => {
    server.videos.set(12, () =>
      json({ ...second, probeState: "failed", playable: false }),
    );
    render(
      <TooltipProvider>
        <ToastProvider>
          <MemoryRouter
            initialEntries={[
              {
                pathname: "/videos/12",
                search: "?playlist=5&item=22",
                state: { from: "/playlists/5", autoplay: true },
              },
            ]}
          >
            <AudienceProvider audience="owner">
              <LocationProbe />
              <Routes>
                <Route path="/videos/:id" element={<VideoPage />} />
              </Routes>
            </AudienceProvider>
          </MemoryRouter>
        </ToastProvider>
      </TooltipProvider>,
    );
    await waitFor(() => expect(location()).toBe("/videos/13?playlist=5&item=23"));
    await waitFor(() => expect(player().video.id).toBe(13));
    setStatus(false, true);
    fireEvent.click(await screen.findByRole("button", { name: "Previous item: second" }));
    await waitFor(() => expect(location()).toBe("/videos/13?playlist=5&item=23"));
    expect(calls("/api/playlists/5/playback?item=22&video=12")).toBe(2);
  });

  it("再生できない項目に自動再生なしで着くと進まず、矢印を状態の層の上に出す", async () => {
    server.videos.set(12, () =>
      json({ ...second, probeState: "failed", playable: false }),
    );
    renderPage();
    await line();
    expect(screen.getByRole("button", { name: "Next item: third" })).toBeDefined();
    expect(location()).toBe("/videos/12?playlist=5&item=22");
  });

  describe("スマートフォルダの項目", () => {
    const member3 = videoOf(43, "member-three");
    const folderItem = (videoCount = 2) => ({
      id: 22,
      kind: "smartFolder" as const,
      smartFolder: {
        id: 7,
        name: "タグ: 猫",
        videoCount,
        watchedCount: 0,
        watchState: "unwatched" as const,
        sizeBytes: 2_000,
        previews: [],
        tags: [],
        ...(videoCount === 0 ? {} : { openVideoId: 12, durationMs: 120_000 }),
      },
    });
    const folderPlaylist = (videoCount = 2): Playlist => ({
      ...summary(),
      items: [
        { id: 21, kind: "video", video: first },
        folderItem(videoCount),
        { id: 23, kind: "video", video: third },
      ],
    });
    const inFolder = (step: PlaylistStep): PlaylistStep => ({
      ...step,
      smartFolderId: 7,
    });

    beforeEach(() => {
      server.videos.set(12, () =>
        json({
          ...second,
          smartFolder: { id: 7, name: "タグ: 猫", position: 1, count: 2 },
        }),
      );
      server.videos.set(43, () => json(member3));
      server.playlist.mockImplementation(() => json(folderPlaylist()));
    });

    it("フォルダの項目へのステップは再生の応答の smartFolderId を運び、パネルの読みが失敗しても同じ", async () => {
      server.playlist.mockImplementation(() =>
        json({ code: "internal", message: "x" }, 500),
      );
      server.playback.mockImplementation(() =>
        json({
          ...playbackFor(21),
          next: inFolder(step(22, second)),
          nextItem: inFolder(step(22, second)),
        }),
      );
      renderPage("/videos/11?playlist=5&item=21");
      await within(await waitForPanel()).findByText("Couldn't load the playlist");
      fireEvent.click(screen.getByRole("button", { name: "Next item: second" }));
      expect(location()).toBe("/videos/12?playlist=5&item=22&smartFolder=7");
    });

    it("フォルダの項目の中では読み取りが smartFolder を運び、フォルダの行・Up next・予告がフォルダに従う", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      server.related.set(12, {
        items: [videoOf(31, "related-next")],
        group: { id: 7, name: "タグ: 猫", items: [second, member3], offset: 0, total: 2 },
      });
      server.playback.mockImplementation(() =>
        json({
          ...playbackFor(22),
          next: inFolder(step(22, member3)),
          previousItem: step(21, first),
        }),
      );
      renderPage("/videos/12?playlist=5&item=22&smartFolder=7");
      await line();
      expect(calls("/api/videos/12?smartFolder=7")).toBeGreaterThan(0);
      expect(calls("/api/videos/12/related?smartFolder=7")).toBe(1);
      const folderLine = await screen.findByRole("link", {
        name: 'Smart folder "タグ: 猫", video 1 of 2. Open the smart folder',
      });
      expect(folderLine.getAttribute("href")).toBe("/smart-folders/7");
      const rows = panelRows();
      expect(rows.map((row) => row.getAttribute("aria-current"))).toEqual([
        null,
        "true",
        null,
      ]);
      expect(within(rows[1]!).getByText("2 videos")).toBeDefined();
      const memberLink = await screen.findByRole("link", { name: /^member-three/ });
      expect(memberLink.getAttribute("href")).toBe(
        "/videos/43?playlist=5&item=22&smartFolder=7",
      );
      expect(screen.getByRole("button", { name: "Previous item: first" })).toBeDefined();
      setStatus(true);
      await screen.findByText(
        'Playback finished. The next video, "member-three", plays in 5 seconds',
      );
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5_000);
      });
      await waitFor(() =>
        expect(location()).toBe("/videos/43?playlist=5&item=22&smartFolder=7"),
      );
    });

    it("今の動画がフォルダから外れると、フォルダの行を出さず、動画のグループのメンバーの行は項目の外として開く", async () => {
      // The folder no longer contains this video; the server falls back to its group and advances to the next item.
      server.videos.set(12, () => json(second));
      server.related.set(12, {
        items: [],
        group: {
          id: 1,
          name: "series",
          items: [videoOf(41, "member-one"), second],
          offset: 0,
          total: 2,
        },
      });
      server.playback.mockImplementation(() =>
        json({ ...playbackFor(22), next: step(23, third), nextItem: step(23, third) }),
      );
      renderPage("/videos/12?playlist=5&item=22&smartFolder=7");
      await line();
      const memberLink = await screen.findByRole("link", { name: /^member-one/ });
      expect(memberLink.getAttribute("href")).toBe("/videos/41");
      expect(screen.queryByRole("link", { name: /^Smart folder/ })).toBeNull();
      expect(panelRows().map((row) => row.getAttribute("aria-current"))).toEqual([
        null,
        "true",
        null,
      ]);
    });

    it("smartFolderId の無いステップは smartFolder を運ばない", async () => {
      server.playback.mockImplementation((item) =>
        json(
          item === 22
            ? { ...playbackFor(22), previousItem: step(21, first) }
            : playbackFor(item),
        ),
      );
      renderPage("/videos/12?playlist=5&item=22&smartFolder=7");
      await line();
      fireEvent.click(screen.getByRole("button", { name: "Previous item: first" }));
      expect(location()).toBe("/videos/11?playlist=5&item=21");
      fireEvent.click(await screen.findByRole("button", { name: "Next item: second" }));
      expect(location()).toBe("/videos/12?playlist=5&item=22");
    });

    it("パネルのフォルダの行はその開く動画を smartFolder 付きで開き、動画の無いフォルダはリンクにしない", async () => {
      renderPage("/videos/11?playlist=5&item=21");
      await line();
      const row = within(panelRows()[1]!);
      const link = row.getByRole("link", { name: "Play from タグ: 猫" });
      expect(link.getAttribute("href")).toBe(
        "/videos/12?playlist=5&item=22&smartFolder=7",
      );
      expect(row.getByText("2 videos")).toBeDefined();

      cleanupAndRender(() => json(folderPlaylist(0)));
      await line();
      const empty = within(panelRows()[1]!);
      expect(empty.queryByRole("link")).toBeNull();
      expect(empty.getByText("0 videos")).toBeDefined();
      expect(empty.getByText("タグ: 猫").className).toContain("text-muted-foreground");

      cleanup();
      server.playback.mockImplementation(() =>
        json({ ...playbackFor(22), next: step(23, third), nextItem: step(23, third) }),
      );
      renderPage("/videos/12?playlist=5&item=22");
      await line();
      const current = panelRows()[1]!;
      expect(current.getAttribute("aria-current")).toBe("true");
      expect(within(current).getAllByText("0 videos")).toHaveLength(2);
    });

    function cleanupAndRender(answer: () => Response) {
      cleanup();
      server.playlist.mockImplementation(answer);
      renderPage("/videos/11?playlist=5&item=21");
    }
  });

  async function waitForPanel(): Promise<HTMLElement> {
    await waitFor(() => panel());
    return panel();
  }
});
