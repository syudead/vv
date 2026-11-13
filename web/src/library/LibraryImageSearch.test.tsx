import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ImageSearchItem, ImageSearchResult } from "../api/imageSearch";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import {
  backTarget,
  closePlayer,
  returnEntry,
  returnState,
} from "../player/pageDecisions";
import { ScanProvider } from "../shell/ScanProvider";
import { json, video } from "../testing/groupPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { __resetImageSearchesForTest } from "./imageSearch/imageSearchState";
import LibraryPage from "./LibraryPage";

/** One image search request the test answers, with the signal the page sent it with. */
interface Sent {
  file: File;
  signal: AbortSignal;
  answer: (response: Response) => void;
}

const fetchMock = vi.fn<typeof fetch>();
let sent: Sent[] = [];

function install() {
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
    if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
    if (url.pathname === "/api/processing") {
      return Promise.resolve(
        json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
      );
    }
    if (url.pathname === "/api/smart-folders") {
      return Promise.resolve(json({ smartFolders: [] }));
    }
    if (url.pathname === "/api/image-search") {
      const body = init?.body as FormData;
      const signal = init?.signal as AbortSignal;
      return new Promise<Response>((resolve, reject) => {
        signal.addEventListener("abort", () =>
          reject(new DOMException("aborted", "AbortError")),
        );
        sent.push({ file: body.get("file") as File, signal, answer: resolve });
      });
    }
    if (url.pathname === "/api/library") {
      return Promise.resolve(
        json({
          items: [{ kind: "video", video: video(1, { title: "ライブラリの動画" }) }],
          total: 1,
        }),
      );
    }
    return Promise.resolve(json({ items: [], total: 0, totalAll: 0 }));
  });
}

/** The player's two ways back: history, and its close, which pushes the list URL as a new entry. */
function Back() {
  const navigate = useNavigate();
  const { state } = useLocation();
  const close = closePlayer(returnState(backTarget(state), returnEntry(state)));
  return (
    <>
      <button type="button" onClick={() => void navigate(-1)}>
        テストで戻る
      </button>
      <button
        type="button"
        onClick={() => void navigate(close.to, { state: close.state })}
      >
        テストで閉じる
      </button>
    </>
  );
}

function renderLibrary(audience: Audience = "owner") {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <ScanProvider>
              <Routes>
                <Route path="/" element={<LibraryPage />} />
                <Route
                  path="/videos/:id"
                  element={
                    <>
                      <p>再生画面</p>
                      <Back />
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

function png(name = "shot.png") {
  return new File(["png"], name, { type: "image/png" });
}

async function pick(user: ReturnType<typeof userEvent.setup>, file: File) {
  await user.click(screen.getByRole("button", { name: "Search by image or video" }));
  const dialog = await screen.findByRole("dialog", { name: "Search by image or video" });
  fireEvent.change(within(dialog).getByTestId("image-upload-input"), {
    target: { files: [file] },
  });
}

/** A drag of the given files, as the browser describes one from outside the page. */
function files(given: File[]) {
  return { dataTransfer: { types: ["Files"], files: given } };
}

/** The element that takes a drop: the frame around the search input. */
function searchField(): HTMLElement {
  const box = screen.getByLabelText("Search videos");
  const frame = box.parentElement;
  if (frame === null) throw new Error("search field has no frame");
  return frame;
}

function item(id: number, memberId: number, positionMs: number): ImageSearchItem {
  return {
    video: video(id, { title: `動画 ${String(id)}` }),
    match: {
      videoId: memberId,
      rate: 0.905,
      kind: "frame",
      positionMs,
      band: "strong",
      frames: [{ positionMs, rate: 0.905 }],
      frameCount: 1,
    },
  };
}

function result(items: ImageSearchItem[], unindexedCount = 0): Response {
  return json({
    query: { kind: "image" },
    bands: [
      { id: "strong", minPercent: 90, maxPercent: 100 },
      { id: "similar", minPercent: 81, maxPercent: 89 },
    ],
    items,
    unindexedCount,
  } satisfies ImageSearchResult);
}

function imageSearchCount() {
  return fetchMock.mock.calls.filter(
    ([input]) =>
      new URL(String(input), "http://localhost").pathname === "/api/image-search",
  ).length;
}

describe("ライブラリの画像検索", () => {
  beforeEach(() => {
    __resetTagsForTest();
    __resetImageSearchesForTest();
    clearListSnapshot();
    sent = [];
    install();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("scrollTo", vi.fn());
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
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("選んだファイルで 1 回だけ要求して検索中を出し、結果は一致した版をその位置で開く", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: /ライブラリの動画/ });
    const file = png();

    await pick(user, file);

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]?.file.name).toBe("shot.png");
    expect(screen.queryByRole("dialog")).toBeNull();
    const body = screen.getByRole("region", { name: "Image search" });
    expect(
      within(body)
        .getAllByRole("status")
        .map((status) => status.textContent),
    ).toContain("Searching…");
    expect(within(body).getByRole("button", { name: "Cancel" })).toBeDefined();
    expect(screen.getByRole("button", { name: "Clear image search" })).toBeDefined();
    expect(screen.queryByRole("link", { name: /ライブラリの動画/ })).toBeNull();

    sent[0]?.answer(result([item(5, 7, 12_500)]));

    const title = await screen.findByRole("link", { name: /動画 5/ });
    expect(title.getAttribute("href")).toBe("/videos/7?t=12.5");
    expect(
      screen.getByRole("link", { name: "Play from 0:12" }).getAttribute("href"),
    ).toBe("/videos/7?t=12.5");
    expect(screen.getByRole("status").textContent).toBe("1 video");
    expect(imageSearchCount()).toBe(1);
  });

  it("新しいファイルは前の要求を中断する", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: /ライブラリの動画/ });

    await pick(user, png("first.png"));
    await waitFor(() => expect(sent).toHaveLength(1));
    await pick(user, png("second.png"));

    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[0]?.signal.aborted).toBe(true);
    expect(sent[1]?.signal.aborted).toBe(false);
    expect(sent[1]?.file.name).toBe("second.png");
  });

  it.each(["テストで戻る", "テストで閉じる"])(
    "再生画面から %s と、要求せずに同じ結果を出す",
    async (way) => {
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: /ライブラリの動画/ });
      await pick(user, png());
      await waitFor(() => expect(sent).toHaveLength(1));
      sent[0]?.answer(result([item(5, 7, 12_500), item(6, 6, 3_000)]));

      // jsdom does not scroll, so the position the page records is set here.
      vi.stubGlobal("scrollY", 640);
      await user.click(await screen.findByRole("link", { name: /動画 6/ }));
      expect(await screen.findByText("再生画面")).toBeDefined();
      const scrollTo = vi.mocked(window.scrollTo);
      scrollTo.mockClear();
      await user.click(screen.getByRole("button", { name: way }));

      expect(await screen.findByRole("link", { name: /動画 5/ })).toBeDefined();
      expect(screen.getByRole("link", { name: /動画 6/ })).toBeDefined();
      expect(screen.getByRole("button", { name: "Clear image search" })).toBeDefined();
      expect(scrollTo).toHaveBeenCalledWith({ top: 640, behavior: "auto" });
      expect(imageSearchCount()).toBe(1);
    },
  );

  it("再生画面から戻ると、畳んだ行を畳んだままにしてからスクロール位置を戻す", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: /ライブラリの動画/ });
    await pick(user, png());
    await waitFor(() => expect(sent).toHaveLength(1));
    sent[0]?.answer(result([item(5, 7, 12_500), item(6, 6, 3_000)]));
    const collapsed = (title: string) =>
      screen.queryByRole("button", { name: `Show frames of ${title}` }) !== null;
    await user.click(
      await screen.findByRole("button", { name: "Hide frames of 動画 5" }),
    );

    vi.stubGlobal("scrollY", 640);
    await user.click(screen.getByRole("link", { name: /動画 6/ }));
    expect(await screen.findByText("再生画面")).toBeDefined();
    // The rows' heights decide where the position lands, so the collapse is drawn when it applies.
    const collapsedAtScroll: boolean[] = [];
    vi.mocked(window.scrollTo).mockImplementation(() => {
      collapsedAtScroll.push(collapsed("動画 5"));
    });
    await user.click(screen.getByRole("button", { name: "テストで戻る" }));

    await screen.findByRole("link", { name: /動画 5/ });
    expect(collapsed("動画 5")).toBe(true);
    expect(collapsed("動画 6")).toBe(false);
    expect(collapsedAtScroll).toEqual([true]);
    expect(window.scrollTo).toHaveBeenCalledWith({ top: 640, behavior: "auto" });
    expect(imageSearchCount()).toBe(1);
  });

  it("一致なしは調べていない動画の数を出し、消すとライブラリに戻る", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: /ライブラリの動画/ });
    await pick(user, png());
    await waitFor(() => expect(sent).toHaveLength(1));

    sent[0]?.answer(result([], 3));

    expect(
      await screen.findByRole("heading", { name: "No video has this footage" }),
    ).toBeDefined();
    expect(
      screen.getByText("3 videos aren't indexed yet and weren't searched."),
    ).toBeDefined();

    const body = screen.getByRole("region", { name: "Image search" });
    await user.click(within(body).getByRole("button", { name: "Clear image search" }));
    expect(await screen.findByRole("link", { name: /ライブラリの動画/ })).toBeDefined();
  });

  it("サーバーが断った理由と別のファイルを選ぶ操作を本体に出す", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: /ライブラリの動画/ });
    await pick(user, png());
    await waitFor(() => expect(sent).toHaveLength(1));

    sent[0]?.answer(
      json(
        {
          code: "invalid_request",
          reason: "image_search_uninformative",
          message: "flat",
        },
        400,
      ),
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("There's nothing to compare in this file");
    expect(within(alert).queryByRole("button", { name: "Retry" })).toBeNull();
    await user.click(within(alert).getByRole("button", { name: "Choose another file…" }));
    expect(
      await screen.findByRole("dialog", { name: "Search by image or video" }),
    ).toBeDefined();
  });

  it("チップがある間の Enter は画像検索を終えて文字で検索する", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: /ライブラリの動画/ });
    await pick(user, png());
    await waitFor(() => expect(sent).toHaveLength(1));

    const box = screen.getByRole("combobox", { name: "Search videos" });
    expect(box.getAttribute("placeholder")).toBe("Or search by text");
    await user.type(box, "京都{Enter}");

    expect(sent[0]?.signal.aborted).toBe(true);
    expect(screen.queryByRole("button", { name: "Clear image search" })).toBeNull();
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(([input]) =>
          String(input).includes("query=%E4%BA%AC%E9%83%BD"),
        ),
      ).toBe(true),
    );
  });

  it("検索欄に落としたファイルで 1 回だけ要求する", async () => {
    renderLibrary();
    await screen.findByRole("link", { name: /ライブラリの動画/ });
    const file = png("dropped.png");

    const taken = fireEvent.drop(searchField(), files([file]));

    expect(taken).toBe(false);
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]?.file.name).toBe(file.name);
    expect(imageSearchCount()).toBe(1);
  });

  it("画像の貼り付けは文字を入れずにその画像で検索し、文字の貼り付けは文字を入れるだけ", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: /ライブラリの動画/ });
    const box = screen.getByRole<HTMLInputElement>("combobox", { name: "Search videos" });

    await user.click(box);
    await user.paste("京都");
    expect(box.value).toBe("京都");
    expect(imageSearchCount()).toBe(0);

    await user.clear(box);
    const file = png("pasted.png");
    const taken = fireEvent.paste(box, { clipboardData: { files: [file] } });

    expect(taken).toBe(false);
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]?.file.name).toBe(file.name);
    expect(box.value).toBe("");
  });

  it("検索欄の外に落としたファイルはブラウザに開かせず、何も要求しない", async () => {
    renderLibrary();
    const card = await screen.findByRole("link", { name: /ライブラリの動画/ });

    expect(fireEvent.dragOver(card, files([png()]))).toBe(false);
    expect(fireEvent.drop(card, files([png()]))).toBe(false);

    expect(screen.getByRole("link", { name: /ライブラリの動画/ })).toBe(card);
    expect(screen.queryByRole("region", { name: "Image search" })).toBeNull();
    expect(imageSearchCount()).toBe(0);
  });

  it.each([
    {
      given: [png("first.png"), png("second.png")],
      title: "Search with one file at a time",
      description: "2 files were given.",
    },
    {
      given: [new File(["memo"], "first.png.txt", { type: "text/plain" })],
      title: "This file can't be searched",
      description: "Use a PNG, JPEG or WebP image, or a video.",
    },
  ])(
    "$title: 要求せずに最初のファイルと理由を出す",
    async ({ given, title, description }) => {
      renderLibrary();
      await screen.findByRole("link", { name: /ライブラリの動画/ });

      fireEvent.drop(searchField(), files(given));

      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toContain(title);
      expect(alert.textContent).toContain(description);
      expect(screen.getByRole("button", { name: "Clear image search" })).toBeDefined();
      expect(screen.getByText(given[0]?.name ?? "")).toBeDefined();
      expect(imageSearchCount()).toBe(0);
    },
  );

  it("ゲストの検索欄にはファイルで検索するボタンがなく、落としても何も始まらない", async () => {
    renderLibrary("guest");
    await screen.findByRole("link", { name: /ライブラリの動画/ });
    expect(screen.queryByRole("button", { name: "Search by image or video" })).toBeNull();

    // The browser's default stays as before, so the drop is not taken.
    expect(fireEvent.drop(searchField(), files([png()]))).toBe(true);
    expect(screen.queryByRole("region", { name: "Image search" })).toBeNull();
    expect(imageSearchCount()).toBe(0);
  });
});
