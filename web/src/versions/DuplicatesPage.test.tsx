import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { VersionCandidate, Video, VideoVersions } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import DuplicatesPage from "./DuplicatesPage";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1024 * 1024 * 100,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "done",
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    container: "mp4",
    videoCodec: "h264",
    tags: [],
    ...extra,
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function pair(first: Video, second: Video): VersionCandidate {
  return { videos: [first, second], distance: 3 };
}

const movieA = video(1, {
  title: "劇場版",
  width: 1920,
  height: 1080,
  sizeBytes: 4 * 1024 * 1024 * 1024,
  folder: { rootId: 1, rootName: "movies", path: "anime" },
  location: { path: "/media/movies/anime/劇場版.mkv", openable: true },
  container: "matroska",
  videoCodec: "hevc",
});
const movieB = video(2, {
  title: "劇場版 720p",
  width: 1280,
  height: 720,
  sizeBytes: 1024 * 1024 * 1024,
  folder: { rootId: 2, rootName: "backup", path: "old" },
  location: { path: "/media/backup/old/劇場版 720p.mp4", openable: true },
});

const server = {
  candidates: [] as VersionCandidate[],
  total: undefined as number | undefined,
  listCalls: 0,
  failList: false,
  dismissRequests: [] as number[][],
  dismissError: null as { status: number; body: unknown } | null,
  holdDismiss: null as ((response: Response) => void) | null,
  /** 止めている却下の応答すべて（重なった決定を同じ時に返すため）。 */
  heldDismisses: [] as ((response: Response) => void)[],
  hold: false,
  /** 一覧の応答を止めておく（取り直しの重なりを見るため）。 */
  holdList: [] as (() => void)[],
  holdingList: false,
  bundleRequests: [] as { videoIds: number[]; representativeId: number }[],
  videoFetches: 0,
};

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url === "/api/version-candidates" && method === "GET") {
      server.listCalls += 1;
      if (server.failList) {
        return Promise.resolve(json({ code: "internal", message: "x" }, 500));
      }
      const body = () =>
        json({
          items: server.candidates,
          total: server.total ?? server.candidates.length,
        });
      if (server.holdingList) {
        return new Promise<Response>((resolve) => {
          server.holdList.push(() => resolve(body()));
        });
      }
      return Promise.resolve(body());
    }
    if (url === "/api/version-candidates/dismiss" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { videoIds: number[] };
      server.dismissRequests.push(body.videoIds);
      if (server.dismissError !== null) {
        return Promise.resolve(
          json(server.dismissError.body, server.dismissError.status),
        );
      }
      const respond = () => {
        server.candidates = server.candidates.filter(
          (candidate) => candidate.videos[0]?.id !== body.videoIds[0],
        );
        return new Response(null, { status: 204 });
      };
      if (server.hold) {
        return new Promise<Response>((resolve) => {
          server.holdDismiss = resolve;
          server.heldDismisses.push(resolve);
        }).then(respond);
      }
      return Promise.resolve(respond());
    }
    if (url === "/api/video-bundles" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as {
        videoIds: number[];
        representativeId: number;
      };
      server.bundleRequests.push(body);
      const videos = server.candidates.flatMap((candidate) => candidate.videos);
      const representative = videos.find((item) => item.id === body.representativeId)!;
      const others = videos.filter(
        (item) => body.videoIds.includes(item.id) && item.id !== body.representativeId,
      );
      server.candidates = server.candidates.filter(
        (candidate) => candidate.videos[0]?.id !== body.videoIds[0],
      );
      const response: VideoVersions = {
        representativeId: body.representativeId,
        items: [representative, ...others],
      };
      return Promise.resolve(json(response));
    }
    if (/^\/api\/videos\/\d+$/.test(url)) {
      server.videoFetches += 1;
    }
    throw new Error(`想定しない要求: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

let location: { pathname: string; state: unknown } | undefined;

function LocationProbe() {
  const current = useLocation();
  location = { pathname: current.pathname, state: current.state };
  return null;
}

function renderPage() {
  render(
    <MemoryRouter initialEntries={["/duplicates"]}>
      <TooltipProvider>
        <ToastProvider>
          <Routes>
            <Route path="/duplicates" element={<DuplicatesPage />} />
            <Route path="/videos/:id" element={<p>再生画面</p>} />
          </Routes>
          <LocationProbe />
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function pairItems(): HTMLElement[] {
  return within(screen.getByRole("list")).getAllByRole("listitem");
}

beforeEach(() => {
  server.candidates = [
    pair(movieA, movieB),
    pair(video(3, { title: "旅行" }), video(4, { title: "旅行 (1)" })),
  ];
  server.total = undefined;
  server.listCalls = 0;
  server.failList = false;
  server.dismissRequests = [];
  server.dismissError = null;
  server.holdDismiss = null;
  server.heldDismisses = [];
  server.hold = false;
  server.holdList = [];
  server.holdingList = false;
  server.bundleRequests = [];
  server.videoFetches = 0;
  location = undefined;
  installFakeEventSource();
  install();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("DuplicatesPage（specs/030-video-versions/ui-design.md「Duplicates page」）", () => {
  it("候補の組を 2 本の違い（解像度・コーデック・サイズ・場所）と共に並べる", async () => {
    renderPage();
    expect(screen.getByRole("heading", { level: 1, name: "Possible duplicates" }));
    expect(document.title).toBe("Duplicates");
    expect(await screen.findByText("2 pairs")).toBeDefined();

    const [first] = pairItems();
    const item = first!;
    expect(within(item).getByText("Same length · similar frames")).toBeDefined();

    // 判断が先、動画の中身は後。DOM の順も「Different videos」→「Same video…」→ 2 本。
    const focusable = Array.from(item.querySelectorAll("button, a")).map((element) =>
      element.getAttribute("aria-label"),
    );
    expect(focusable).toEqual([
      "Different videos, for 劇場版 and 劇場版 720p",
      "Same video, for 劇場版 and 劇場版 720p",
      "劇場版 1:00",
      "劇場版 720p 1:00",
    ]);
    expect(
      within(item).getByRole("button", { name: /^Different videos/ }).textContent,
    ).toBe("Different videos");
    expect(within(item).getByRole("button", { name: /^Same video/ }).textContent).toBe(
      "Same video…",
    );

    const left = within(item).getByRole("link", { name: "劇場版 1:00" });
    expect(left.getAttribute("href")).toBe("/videos/1");
    expect(within(left).getByText("1920×1080")).toBeDefined();
    expect(within(left).getByText("MATROSKA")).toBeDefined();
    expect(within(left).getByText("H.265")).toBeDefined();
    expect(within(left).getByText("4.0 GB")).toBeDefined();
    // 置き場所は 3 行目。title に絶対パスを入れる。
    const place = within(left).getByText("movies / anime");
    expect(place.closest("[title]")?.getAttribute("title")).toBe(
      "/media/movies/anime/劇場版.mkv",
    );

    const right = within(item).getByRole("link", { name: "劇場版 720p 1:00" });
    expect(within(right).getByText("1280×720")).toBeDefined();
    expect(within(right).getByText("1.0 GB")).toBeDefined();
    expect(within(right).getByText("backup / old")).toBeDefined();
    // 距離の数字は出さない。
    expect(item.textContent).not.toContain("3 ");
  });

  it("動画のリンクは戻り先を候補の画面にして再生画面へ移る", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("link", { name: "劇場版 720p 1:00" }));
    expect(location).toEqual({ pathname: "/videos/2", state: { from: "/duplicates" } });
  });

  it("「Same video…」で代表を選んで束ねると組が消え、件数が減り、次の組へフォーカスが移る", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(
      await screen.findByRole("button", {
        name: "Same video, for 劇場版 and 劇場版 720p",
      }),
    );

    const dialog = screen.getByRole("dialog", { name: "Bundle as versions" });
    // 組の 2 本をそのまま渡し、取り直さない。行は id の昇順。
    const radios = within(dialog).getAllByRole("radio");
    expect(radios.map((radio) => radio.closest("label")?.textContent)).toEqual([
      expect.stringContaining("劇場版"),
      expect.stringContaining("劇場版 720p"),
    ]);
    expect(server.videoFetches).toBe(0);

    await user.click(within(dialog).getByRole("radio", { name: /^劇場版,/ }));
    await user.click(within(dialog).getByRole("button", { name: "Bundle" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(server.bundleRequests).toEqual([{ videoIds: [1, 2], representativeId: 1 }]);
    expect(
      await screen.findByText('Bundled 2 videos as versions of "劇場版"'),
    ).toBeDefined();
    expect(pairItems()).toHaveLength(1);
    expect(screen.queryByText("劇場版 720p")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("1 pair");
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Different videos, for 旅行 and 旅行 (1)" }),
      ),
    );
    // 一覧の取り直しは scan の知らせに任せる。
    expect(server.listCalls).toBe(1);
  });

  it("「Different videos」は確認なしで却下を送り、送っている間は組のボタンを押せない", async () => {
    const user = userEvent.setup();
    server.hold = true;
    renderPage();
    const different = await screen.findByRole("button", {
      name: "Different videos, for 劇場版 and 劇場版 720p",
    });
    await user.click(different);

    expect(server.dismissRequests).toEqual([[1, 2]]);
    expect(screen.queryByRole("dialog")).toBeNull();
    const item = pairItems()[0]!;
    const buttons = within(item).getAllByRole("button") as HTMLButtonElement[];
    expect(buttons.every((button) => button.disabled)).toBe(true);
    expect(different.querySelector("svg.animate-spin")).not.toBeNull();
    // もう一方の組は押せる。
    expect(
      (
        screen.getByRole("button", {
          name: "Different videos, for 旅行 and 旅行 (1)",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);

    server.holdDismiss?.(new Response(null, { status: 204 }));
    expect(
      await screen.findByText(
        "Marked as different videos. They won't be suggested again",
      ),
    ).toBeDefined();
    await waitFor(() => expect(pairItems()).toHaveLength(1));
    expect(screen.getByRole("status").textContent).toBe("1 pair");
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Different videos, for 旅行 and 旅行 (1)" }),
      ),
    );
  });

  it("最後の組を決めると空の状態に替わり、フォーカスは見出しへ移る", async () => {
    const user = userEvent.setup();
    server.candidates = [pair(movieA, movieB)];
    renderPage();
    await user.click(await screen.findByRole("button", { name: /^Different videos/ }));

    expect(
      await screen.findByRole("heading", { name: "No possible duplicates" }),
    ).toBeDefined();
    expect(screen.queryByRole("list")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("");
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("heading", { level: 1, name: "Possible duplicates" }),
      ),
    );
  });

  it("前の組が無い最後の組を決めると、前の組へフォーカスが移る", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(
      await screen.findByRole("button", {
        name: "Different videos, for 旅行 and 旅行 (1)",
      }),
    );
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", {
          name: "Different videos, for 劇場版 and 劇場版 720p",
        }),
      ),
    );
  });

  it("別々の組の却下が同時に終わっても、先に消した組を戻さない", async () => {
    const user = userEvent.setup();
    server.hold = true;
    renderPage();
    await user.click(
      await screen.findByRole("button", {
        name: "Different videos, for 劇場版 and 劇場版 720p",
      }),
    );
    await user.click(
      screen.getByRole("button", { name: "Different videos, for 旅行 and 旅行 (1)" }),
    );
    expect(server.dismissRequests).toEqual([
      [1, 2],
      [3, 4],
    ]);

    // 2 つの応答を同じ描画の前に返す。
    for (const resolve of server.heldDismisses)
      resolve(new Response(null, { status: 204 }));

    expect(
      await screen.findByRole("heading", { name: "No possible duplicates" }),
    ).toBeDefined();
    expect(screen.queryByRole("list")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("");
  });

  it("見せていた組を決め尽くしても上限の外に組が残っていれば、取り直して出す", async () => {
    const user = userEvent.setup();
    server.candidates = [pair(movieA, movieB)];
    server.total = 2;
    renderPage();
    expect(await screen.findByText("Showing 1 of 2 pairs")).toBeDefined();

    // 上限の外にあった組は、見せていた組が減ると応答に入る。
    const rest = pair(
      video(5, { title: "残りの組" }),
      video(6, { title: "残りの組 (2)" }),
    );
    server.hold = true;
    await user.click(screen.getByRole("button", { name: /^Different videos/ }));
    server.candidates = [rest];
    server.total = undefined;
    server.holdDismiss?.(new Response(null, { status: 204 }));

    expect(await screen.findByText("残りの組")).toBeDefined();
    expect(screen.getByRole("status").textContent).toBe("1 pair");
    expect(screen.queryByRole("heading", { name: "No possible duplicates" })).toBeNull();
    expect(server.listCalls).toBe(2);
  });

  it("組が消えていた（404）ときは伝えて一覧を取り直す", async () => {
    const user = userEvent.setup();
    server.dismissError = {
      status: 404,
      body: { code: "not_found", reason: "video_not_found", message: "x" },
    };
    renderPage();
    const different = await screen.findByRole("button", {
      name: /^Different videos, for 劇場版 and/,
    });
    // 確かめている間に片方がスキャンで消えた。
    server.candidates = [server.candidates[1]!];
    await user.click(different);

    expect(await screen.findByText("This pair is no longer a candidate")).toBeDefined();
    await waitFor(() => expect(pairItems()).toHaveLength(1));
    expect(server.listCalls).toBe(2);
    expect(screen.queryByText("劇場版 720p")).toBeNull();
  });

  it("そのほかの失敗では組を残して理由を伝える", async () => {
    const user = userEvent.setup();
    server.dismissError = { status: 500, body: { code: "internal", message: "x" } };
    renderPage();
    const different = await screen.findByRole("button", {
      name: /^Different videos, for 劇場版 and/,
    });
    await user.click(different);

    await waitFor(() => expect((different as HTMLButtonElement).disabled).toBe(false));
    expect(pairItems()).toHaveLength(2);
    expect(server.listCalls).toBe(1);
    expect(
      screen.queryByText("Marked as different videos. They won't be suggested again"),
    ).toBeNull();
  });

  it("scan の知らせで骨組みにせず取り直し、増えた組と消えた組を反映する", async () => {
    renderPage();
    await screen.findByText("2 pairs");
    server.candidates = [
      pair(video(5, { title: "新しい組" }), video(6, { title: "新しい組 (2)" })),
      server.candidates[1]!,
    ];
    await emitServerEvent("scan", { id: 1, state: "running" });

    await waitFor(() => expect(screen.getByText("新しい組")).toBeDefined());
    expect(screen.queryByText("劇場版 720p")).toBeNull();
    expect(pairItems()).toHaveLength(2);
    expect(server.listCalls).toBe(2);
    expect(
      document.querySelector('[aria-hidden="true"] .animate-pulse, .h-24'),
    ).toBeNull();
  });

  it("取り直しが重なったら、今の取得が終わってから 1 度だけ取り直す", async () => {
    server.holdingList = true;
    renderPage();
    await emitServerEvent("scan", {});
    await emitServerEvent("scan", {});
    await emitServerEvent("scan", {});
    expect(server.listCalls).toBe(1);
    server.holdingList = false;
    server.holdList.shift()?.();
    await screen.findByText("2 pairs");
    await waitFor(() => expect(server.listCalls).toBe(2));
    // 重なった 3 回は 1 回に畳む。
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(server.listCalls).toBe(2);
  });

  it("200 組を超えるときは見せている数と全体の数を出す", async () => {
    server.total = 340;
    renderPage();
    expect(await screen.findByText("Showing 2 of 340 pairs")).toBeDefined();
  });

  it("候補が無いときは説明を出し、操作を置かない", async () => {
    server.candidates = [];
    renderPage();
    expect(
      await screen.findByRole("heading", { name: "No possible duplicates" }),
    ).toBeDefined();
    expect(
      screen.getByText(/When a scan finds files that look like the same video/),
    ).toBeDefined();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("読み込みに失敗したら Retry で取り直せる", async () => {
    const user = userEvent.setup();
    server.failList = true;
    renderPage();
    expect(
      await screen.findByRole("heading", { name: "Couldn't load the candidates" }),
    ).toBeDefined();
    server.failList = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("2 pairs")).toBeDefined();
  });

  it("疑似ロケールではカタログの文言だけを描く", async () => {
    enablePseudoLocale();
    server.candidates = [];
    renderPage();
    await screen.findByRole("heading", { level: 2 });
    expectCatalogTextOnly(document.body);
  });
});
