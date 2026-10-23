import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  listVideoVersions,
  makeRepresentativeVersion,
  RequestFailed,
  unbundleVideo,
  type Video,
  type VideoVersions,
} from "../../api/client";
import { formatBytes } from "../../lib/format";
import { ToastProvider } from "../../ui/Toast";
import VersionsPanel, { type VersionsPanelProps } from "./VersionsPanel";

vi.mock("../../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api/client")>()),
  listVideoVersions: vi.fn(),
  makeRepresentativeVersion: vi.fn(),
  unbundleVideo: vi.fn(),
}));

const list = vi.mocked(listVideoVersions);
const makeRepresentative = vi.mocked(makeRepresentativeVersion);
const unbundle = vi.mocked(unbundleVideo);

const folder = { rootId: 1, path: "movies", rootName: "Media" };
const versionA: Video = {
  id: 7,
  title: "Alpha",
  sizeBytes: 84_331_821,
  addedAt: "2026-09-20T03:00:00Z",
  updatedAt: "2026-09-20T03:00:00Z",
  fileCreatedAt: "2026-09-20T03:00:00Z",
  fileModifiedAt: "2026-09-20T03:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  folder,
  location: { path: "/media/movies/A.mp4", openable: true },
  tags: [],
  public: false,
  versions: { count: 3, representativeId: 7 },
};
const versionB: Video = {
  ...versionA,
  id: 20,
  title: "Bravo",
  width: 1280,
  height: 720,
  container: "mkv",
  videoCodec: "hevc",
  sizeBytes: 1_048_576,
  folder: { rootId: 1, path: "movies/small", rootName: "Media" },
  location: { path: "/media/movies/small/B.mkv", openable: true },
};
const versionC: Video = {
  ...versionA,
  id: 21,
  title: "Cedar",
  location: { path: "/media/movies/C.mp4", openable: true },
};

function answer(items: Video[], representativeId = 7): VideoVersions {
  return { representativeId, items };
}

function renderPanel(overrides: Partial<VersionsPanelProps> = {}) {
  const props: VersionsPanelProps = {
    video: versionA,
    owner: true,
    navigation: { backTo: "/?q=abc", autoplay: true },
    onReplace: vi.fn(),
    onRefresh: vi.fn(),
    onLeave: vi.fn(),
    ...overrides,
  };
  const view = render(
    <ToastProvider>
      <MemoryRouter>
        <VersionsPanel {...props} />
      </MemoryRouter>
    </ToastProvider>,
  );
  return { props, view };
}

async function rows(): Promise<HTMLElement[]> {
  const listElement = await screen.findByRole("list", { name: "Versions" });
  return within(listElement).getAllByRole("listitem");
}

function row(title: string): HTMLElement {
  const found = within(screen.getByRole("list", { name: "Versions" }))
    .getAllByRole("listitem")
    .find((item) => item.textContent.includes(title));
  if (found === undefined) throw new Error(`no row for ${title}`);
  return found;
}

function actions(item: HTMLElement): string[] {
  return within(item)
    .queryAllByRole("button")
    .map((button) => button.textContent);
}

describe("VersionsPanel", () => {
  beforeEach(() => {
    list.mockResolvedValue(answer([versionA, versionB, versionC]));
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("代表と再生中の行に見える印を付け、ほかの行は詳細付きの名前を持つ Play でそのバージョンを開く", async () => {
    renderPanel();
    const [first, second, third] = await rows();
    expect(list).toHaveBeenCalledWith(7, expect.any(AbortSignal));

    expect(first?.getAttribute("aria-current")).toBe("true");
    // Both marks are visible chips, not screen-reader text.
    const nowPlaying = within(first as HTMLElement).getByText("Now playing");
    expect(nowPlaying.closest(".sr-only")).toBeNull();
    expect(within(first as HTMLElement).getByText("Representative")).toBeDefined();
    expect(within(first as HTMLElement).queryByRole("link")).toBeNull();

    expect(second?.getAttribute("aria-current")).toBeNull();
    expect(second?.textContent).not.toContain("Representative");
    const link = within(second as HTMLElement).getByRole("link");
    expect(link.textContent).toBe("Play");
    expect(link.getAttribute("aria-label")).toBe(
      `Play Bravo, 1280×720 H.265 ${formatBytes(1_048_576)} MKV Media / movies/small`,
    );
    expect(link.getAttribute("href")).toBe("/videos/20");
    // The owner reads the absolute path on its own line.
    expect(
      within(second as HTMLElement).getByText("/media/movies/small/B.mkv"),
    ).toBeDefined();
    expect(
      within(third as HTMLElement)
        .getByRole("link")
        .getAttribute("href"),
    ).toBe("/videos/21");
  });

  it("所有者には再生中を含む全行に Remove from versions、代表以外の行に Make representative を出し、再生中の行に Play を出さない", async () => {
    renderPanel({ video: versionB });
    await rows();
    expect(actions(row("Alpha"))).toEqual(["Remove from versions"]);
    expect(actions(row("Bravo"))).toEqual([
      "Make representative",
      "Remove from versions",
    ]);
    expect(row("Bravo").getAttribute("aria-current")).toBe("true");
    expect(within(row("Bravo")).queryByRole("link", { name: /^Play / })).toBeNull();
    expect(within(row("Alpha")).getByRole("link", { name: /^Play Alpha/ })).toBeDefined();
    expect(actions(row("Cedar"))).toEqual([
      "Make representative",
      "Remove from versions",
    ]);
  });

  it("Make representative は応答の一覧で印を移し、動画を取り直す（受け入れ条件 4）", async () => {
    makeRepresentative.mockResolvedValue(answer([versionB, versionA, versionC], 20));
    const { props } = renderPanel();
    await rows();
    fireEvent.click(
      within(row("Bravo")).getByRole("button", { name: "Make representative" }),
    );

    await waitFor(() => expect(row("Bravo").textContent).toContain("Representative"));
    expect(makeRepresentative).toHaveBeenCalledWith(20);
    expect(row("Alpha").textContent).not.toContain("Representative");
    expect(actions(row("Bravo"))).toEqual(["Remove from versions"]);
    expect(props.onRefresh).toHaveBeenCalledTimes(1);
  });

  it("フォーカスした Make representative が成功で消えると同じ行の残る操作へフォーカスを移し、失敗ではその場に残す", async () => {
    makeRepresentative.mockRejectedValueOnce(new Error("offline"));
    makeRepresentative.mockResolvedValueOnce(answer([versionB, versionA, versionC], 20));
    renderPanel();
    await rows();
    const pressed = within(row("Bravo")).getByRole("button", {
      name: "Make representative",
    });
    pressed.focus();
    fireEvent.click(pressed);
    await screen.findByRole("alert");
    expect(document.activeElement).toBe(pressed);

    fireEvent.click(pressed);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(row("Bravo")).getByRole("button", { name: "Remove from versions" }),
      ),
    );
  });

  it("押した操作が終わるまで、その行の操作は回転と aria-disabled になり、もう一度送らない", async () => {
    makeRepresentative.mockReturnValue(new Promise(() => undefined));
    renderPanel();
    await rows();
    const pressed = within(row("Bravo")).getByRole("button", {
      name: "Make representative",
    });
    fireEvent.click(pressed);

    const remove = within(row("Bravo")).getByRole("button", {
      name: "Remove from versions",
    });
    expect(pressed.getAttribute("aria-disabled")).toBe("true");
    expect(pressed.querySelector(".animate-spin")).not.toBeNull();
    expect(remove.getAttribute("aria-disabled")).toBe("true");
    expect(remove.querySelector(".animate-spin")).toBeNull();
    fireEvent.click(remove);
    fireEvent.click(pressed);
    expect(unbundle).not.toHaveBeenCalled();
    expect(makeRepresentative).toHaveBeenCalledTimes(1);
    // Other rows stay pressable.
    const otherRemove = within(row("Cedar")).getByRole("button", {
      name: "Remove from versions",
    });
    expect(otherRemove.hasAttribute("aria-disabled")).toBe(false);
  });

  it("別の行を外すと行を消してトーストを出し、動画を取り直す。残りが 1 本ならパネルを離れる（受け入れ条件 4）", async () => {
    unbundle.mockImplementation((id) =>
      Promise.resolve({ ...(id === 21 ? versionC : versionB), versions: undefined }),
    );
    const { props } = renderPanel();
    await rows();
    fireEvent.click(
      within(row("Cedar")).getByRole("button", { name: "Remove from versions" }),
    );

    expect(await screen.findByText('Removed "Cedar" from the versions')).toBeDefined();
    expect((await rows()).map((item) => item.textContent.includes("Cedar"))).toEqual([
      false,
      false,
    ]);
    expect(unbundle).toHaveBeenCalledWith(21);
    expect(props.onRefresh).toHaveBeenCalledTimes(1);
    expect(props.onLeave).not.toHaveBeenCalled();

    fireEvent.click(
      within(row("Bravo")).getByRole("button", { name: "Remove from versions" }),
    );
    await waitFor(() => expect(props.onLeave).toHaveBeenCalledTimes(1));
    expect(props.onRefresh).toHaveBeenCalledTimes(2);
    expect(props.onReplace).not.toHaveBeenCalled();
  });

  it("再生中のバージョンを自分の行から外すと、応答の動画でページを置き換えてパネルを離れる", async () => {
    const removed: Video = { ...versionA, versions: undefined };
    unbundle.mockResolvedValue(removed);
    const { props } = renderPanel();
    await rows();
    fireEvent.click(
      within(row("Alpha")).getByRole("button", { name: "Remove from versions" }),
    );

    await waitFor(() => expect(props.onReplace).toHaveBeenCalledTimes(1));
    expect(props.onReplace).toHaveBeenCalledWith(removed, expect.any(Object));
    expect(props.onLeave).toHaveBeenCalledTimes(1);
    expect(screen.getByText('Removed "Alpha" from the versions')).toBeDefined();
  });

  it("先に束が変わっていた失敗は末尾に 1 行出して一覧を残し、動画を取り直す", async () => {
    makeRepresentative.mockRejectedValue(
      new RequestFailed(400, "invalid_request", "x", { reason: "not_bundled" }),
    );
    const { props } = renderPanel();
    await rows();
    fireEvent.click(
      within(row("Bravo")).getByRole("button", { name: "Make representative" }),
    );

    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't change the versions: This video isn't bundled with others.",
    );
    expect(await rows()).toHaveLength(3);
    expect(props.onRefresh).toHaveBeenCalledTimes(1);
  });

  it("読めた一覧が 1 本だけなら、動画を取り直してパネルを離れる", async () => {
    list.mockResolvedValue(answer([versionA]));
    const { props } = renderPanel();
    await waitFor(() => expect(props.onLeave).toHaveBeenCalledTimes(1));
    expect(props.onRefresh).toHaveBeenCalledTimes(1);
  });

  it("一覧を読めなければ失敗の文と Retry を出し、Retry で読み直す", async () => {
    list.mockRejectedValueOnce(new RequestFailed(500, "internal", "x"));
    renderPanel();
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't load the versions",
    );
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await rows()).toHaveLength(3);
    expect(list).toHaveBeenCalledTimes(2);
  });

  it("動画の応答が替わると骨組みを出さずに一覧を読み直す", async () => {
    const { props, view } = renderPanel();
    await rows();
    view.rerender(
      <ToastProvider>
        <MemoryRouter>
          <VersionsPanel {...props} video={{ ...versionA }} />
        </MemoryRouter>
      </ToastProvider>,
    );
    expect(screen.getByRole("list", { name: "Versions" })).toBeDefined();
    await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  });

  it("ゲストの行は再生だけで、操作と絶対パスを出さない", async () => {
    const guest = (value: Video): Video => ({ ...value, location: undefined });
    list.mockResolvedValue(answer([guest(versionA), guest(versionB), guest(versionC)]));
    renderPanel({ video: guest(versionA), owner: false });
    await rows();
    expect(screen.queryByRole("button")).toBeNull();
    expect(within(row("Bravo")).getByRole("link").getAttribute("href")).toBe(
      "/videos/20",
    );
    expect(row("Bravo").querySelector('[title*="/media/"]')).toBeNull();
    // A guest reads the version's place where the owner reads the path.
    expect(within(row("Bravo")).getByText("Media / movies/small")).toBeDefined();
  });
});
