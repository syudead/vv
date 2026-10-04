import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video, VideoVersions } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import BundleDialog from "./BundleDialog";
import { versionDetails } from "./VersionDetails";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1024 * 1024 * id,
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
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const server = {
  videos: new Map<number, Video>(),
  /** 取得に失敗させる id。 */
  failing: new Set<number>(),
  bundleRequests: [] as { videoIds: number[]; representativeId: number }[],
  bundleError: null as { status: number; body: unknown } | null,
  /** 束ねる応答を止めておく（送信中の状態を見るため）。 */
  holdBundle: null as ((response: Response) => void) | null,
  hold: false,
};

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    const detail = /^\/api\/videos\/(\d+)$/.exec(url);
    if (detail !== null && method === "GET") {
      const id = Number(detail[1]);
      const found = server.videos.get(id);
      if (found === undefined || server.failing.has(id)) {
        return Promise.resolve(
          json({ code: "not_found", reason: "video_not_found", message: "x" }, 404),
        );
      }
      return Promise.resolve(json(found));
    }
    if (url === "/api/video-bundles" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as {
        videoIds: number[];
        representativeId: number;
      };
      server.bundleRequests.push(body);
      if (server.bundleError !== null) {
        return Promise.resolve(json(server.bundleError.body, server.bundleError.status));
      }
      const representative = server.videos.get(body.representativeId)!;
      const others = body.videoIds
        .filter((id) => id !== body.representativeId)
        .map((id) => server.videos.get(id)!);
      const response: VideoVersions = {
        representativeId: body.representativeId,
        items: [representative, ...others],
      };
      if (server.hold) {
        return new Promise<Response>((resolve) => {
          server.holdBundle = resolve;
        }).then(() => json(response));
      }
      return Promise.resolve(json(response));
    }
    throw new Error(`想定しない要求: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderDialog(props: Partial<React.ComponentProps<typeof BundleDialog>> = {}): {
  onClose: ReturnType<typeof vi.fn>;
  onBundled: ReturnType<typeof vi.fn>;
} {
  const onClose = vi.fn();
  const onBundled = vi.fn();
  render(
    <TooltipProvider>
      <ToastProvider>
        <BundleDialog
          videoIds={[1, 2]}
          onClose={onClose}
          onBundled={onBundled}
          {...props}
        />
      </ToastProvider>
    </TooltipProvider>,
  );
  return { onClose, onBundled };
}

beforeEach(() => {
  server.videos = new Map([
    [
      1,
      video(1, {
        title: "劇場版",
        folder: { rootId: 1, rootName: "movies", path: "anime" },
        location: { path: "/media/movies/anime/劇場版.mkv", openable: true },
        container: "matroska",
        tags: [
          { id: 1, name: "アニメ", manual: true, fromFolder: false, tentative: false },
          { id: 2, name: "anime", manual: false, fromFolder: true, tentative: false },
        ],
      }),
    ],
    [2, video(2, { title: "TV 版", width: 1280, height: 720 })],
    [3, video(3, { title: "既に束ねた", versions: { count: 3, representativeId: 3 } })],
  ]);
  server.failing = new Set();
  server.bundleRequests = [];
  server.bundleError = null;
  server.holdBundle = null;
  server.hold = false;
  install();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("BundleDialog（specs/030-video-versions/ui-design.md「Bundle dialog」）", () => {
  it("選んだ id ごとに動画を取り、選んだ順に代表の行を並べる", async () => {
    renderDialog({ videoIds: [2, 1, 3] });
    const dialog = screen.getByRole("dialog", { name: "Bundle as versions" });
    expect(within(dialog).getByText(/^These 3 videos become versions of one video\./));
    const group = await within(dialog).findByRole("radiogroup", {
      name: "Representative",
    });
    const radios = within(group).getAllByRole("radio");
    expect(radios.map((radio) => radio.closest("label")?.textContent)).toEqual([
      expect.stringContaining("TV 版"),
      expect.stringContaining("劇場版"),
      expect.stringContaining("既に束ねた"),
    ]);
    // 既定で先頭を選ばない。
    expect(radios.every((radio) => radio.getAttribute("aria-checked") === "false")).toBe(
      true,
    );
  });

  it("行に違いと手で付けたタグを出し、既に束ねた動画には全部が入ることを添える", async () => {
    renderDialog({ videoIds: [1, 3] });
    const first = (await screen.findByRole("radio", { name: /劇場版/ })).closest(
      "label",
    )!;
    expect(within(first).getByText("1920×1080")).toBeDefined();
    expect(within(first).getByText("MATROSKA")).toBeDefined();
    // 手で付けたタグだけを出し、フォルダ名からだけ付いたタグは出さない。
    expect(within(first).getByText("アニメ")).toBeDefined();
    expect(first.textContent).not.toContain("anime ·");
    // 所有者の窓なので、2 行目の title に絶対パスを入れる。
    expect(
      first.querySelector('[title*="/media/movies/anime/劇場版.mkv"]'),
    ).not.toBeNull();

    const bundled = screen.getByRole("radio", { name: /既に束ねた/ }).closest("label")!;
    expect(
      within(bundled).getByText("Already 3 versions — all of them join"),
    ).toBeDefined();
  });

  it("代表を選ぶまで Bundle を押せず、選んで押すと束ねてトーストで伝える", async () => {
    const user = userEvent.setup();
    const { onBundled } = renderDialog({ videoIds: [1, 2] });
    await screen.findByRole("radio", { name: /TV 版/ });
    // 最初のフォーカスは Cancel。
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" }));
    const submit = screen.getByRole("button", { name: "Bundle" }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);

    await user.click(screen.getByRole("radio", { name: /TV 版/ }));
    expect(submit.disabled).toBe(false);
    await user.click(submit);

    await waitFor(() => expect(onBundled).toHaveBeenCalledTimes(1));
    expect(server.bundleRequests).toEqual([{ videoIds: [1, 2], representativeId: 2 }]);
    expect(
      await screen.findByText('Bundled 2 videos as versions of "TV 版"'),
    ).toBeDefined();
  });

  it("送っている間は両方のボタンを押せず、Bundle に回転の印を出す", async () => {
    const user = userEvent.setup();
    server.hold = true;
    renderDialog();
    await user.click(await screen.findByRole("radio", { name: /劇場版/ }));
    const submit = screen.getByRole("button", { name: "Bundle" }) as HTMLButtonElement;
    await user.click(submit);
    await waitFor(() => expect(submit.disabled).toBe(true));
    expect(
      (screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(submit.querySelector(".animate-spin")).not.toBeNull();
    server.holdBundle?.(new Response());
  });

  it("失敗は窓の中の 1 行で伝え、選んだ代表を残す", async () => {
    const user = userEvent.setup();
    server.bundleError = {
      status: 400,
      body: { code: "invalid_request", reason: "too_few_videos", message: "x" },
    };
    const { onBundled, onClose } = renderDialog();
    await user.click(await screen.findByRole("radio", { name: /劇場版/ }));
    await user.click(screen.getByRole("button", { name: "Bundle" }));

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Couldn't bundle: Select at least two videos.");
    expect(
      screen.getByRole("radio", { name: /劇場版/ }).getAttribute("aria-checked"),
    ).toBe("true");
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Bundle" }));
    expect(onBundled).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("選んだ中に消えた動画があれば、失敗を出したうえで一覧を取り直す", async () => {
    const user = userEvent.setup();
    server.bundleError = {
      status: 404,
      body: { code: "not_found", reason: "video_not_found", message: "x" },
    };
    renderDialog();
    await user.click(await screen.findByRole("radio", { name: /劇場版/ }));
    server.failing.add(2);
    await user.click(screen.getByRole("button", { name: "Bundle" }));

    expect(await screen.findByText("Couldn't load the selected videos")).toBeDefined();
    expect(screen.getByText(/^Couldn't bundle: /)).toBeDefined();
    expect(
      (screen.getByRole("button", { name: "Bundle" }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it("1 本でも取れなければ Retry を出し、Bundle を押せない", async () => {
    const user = userEvent.setup();
    server.failing.add(2);
    renderDialog();
    const alert = await screen.findByRole("alert");
    // 失敗の Alert は Retry を添える。
    expect(alert.textContent).toBe("Couldn't load the selected videosRetry");
    expect(
      (screen.getByRole("button", { name: "Bundle" }) as HTMLButtonElement).disabled,
    ).toBe(true);

    server.failing.clear();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("radio", { name: /TV 版/ })).toBeDefined();
  });

  it("中身を渡されたときは取らずに並べる", async () => {
    const fetchMock = install();
    renderDialog({
      videoIds: [1, 2],
      videos: [server.videos.get(1)!, server.videos.get(2)!],
    });
    expect(screen.getByRole("radio", { name: /劇場版/ })).toBeDefined();
    await Promise.resolve();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("Esc と Cancel は何も送らずに閉じる", async () => {
    const user = userEvent.setup();
    const { onClose } = renderDialog();
    await screen.findByRole("radio", { name: /劇場版/ });
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledTimes(2);
    expect(server.bundleRequests).toEqual([]);
  });

  it("文言はカタログから出す", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    renderDialog({ videoIds: [1, 3] });
    await screen.findAllByRole("radio");
    const userData = [1, 3].flatMap((id) => {
      const item = server.videos.get(id)!;
      const details = versionDetails(item);
      return [item.title, ...details.specs, details.place, item.location?.path ?? ""];
    });
    expectCatalogTextOnly(document.body, [...userData, "アニメ"]);

    server.failing.add(3);
    server.bundleError = {
      status: 404,
      body: { code: "not_found", reason: "video_not_found", message: "x" },
    };
    await user.click(screen.getAllByRole("radio")[0]!);
    await user.click(screen.getByRole("button", { name: /Bundle/ }));
    await screen.findByText(/Couldn't load the selected videos/);
    expectCatalogTextOnly(document.body, userData);
  });
});
