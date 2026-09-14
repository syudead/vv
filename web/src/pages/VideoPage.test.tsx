import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";

/**
 * 再生画面の 4 状態と並び（contracts/screen-states.md 2. / FR-012〜FR-016）。
 *
 * 要点は 2 つある。**「一覧へ戻る」がどの状態でも先に出る**こと（取得に失敗
 * した画面から戻れないと、残る手が再読み込みしかなくなる）と、**知らせが映像
 * より前にある**ことである。後者は見た目を見れば分かるが、映像の上に重ねる
 * 実装と DOM の順序では区別がつかない — 重ねると映像を隠して FR-014 に反する。
 */

const { getVideo, saveProgress, beaconProgress } = vi.hoisted(() => ({
  getVideo: vi.fn(),
  saveProgress: vi.fn(),
  beaconProgress: vi.fn(),
}));

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  getVideo,
  saveProgress,
  beaconProgress,
}));

const { default: VideoPage } = await import("./VideoPage");

/** done は解析の済んだ動画である。 */
const done: Video = {
  id: 7,
  title: "長い動画",
  sizeBytes: 1024,
  addedAt: "2026-09-13T00:00:00Z",
  durationMs: 600_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  audioCodec: "aac",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
};

/** show は再生画面を描き、詳細の取得を終わらせる。 */
async function show(path = "/videos/7"): Promise<void> {
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/videos/:id" element={<VideoPage />} />
      </Routes>
    </MemoryRouter>,
  );
  await act(async () => {
    await Promise.resolve();
  });
}

/** precedes は left が right より前の DOM 位置にあるかを返す。 */
function precedes(left: Element, right: Element): boolean {
  return (left.compareDocumentPosition(right) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
}

/** facts は情報欄（dl）を返す。知らせの文言と取り違えないよう範囲を絞る。 */
function facts(): HTMLElement {
  const element = document.querySelector("dl");
  if (element === null) {
    throw new Error("情報欄が描かれていない");
  }
  return element;
}

/** player は映像の要素を返す。 */
function player(): HTMLVideoElement {
  const element = document.querySelector("video");
  if (element === null) {
    throw new Error("映像が描かれていない");
  }
  return element;
}

beforeEach(() => {
  getVideo.mockReset();
  getVideo.mockResolvedValue(done);
  saveProgress.mockReset();
  saveProgress.mockResolvedValue({
    positionMs: 0,
    completed: false,
    updatedAt: "2026-09-13T00:00:00Z",
  });
  beaconProgress.mockReset();
  beaconProgress.mockReturnValue(true);
});

describe("VideoPage の 4 状態（contracts/screen-states.md 2.）", () => {
  it("通信中も「一覧へ戻る」が先に出て、映像の場所には骨組みが出る", async () => {
    getVideo.mockReturnValue(new Promise<Video>(() => undefined));
    await show();

    const back = screen.getByRole("link", { name: "← 一覧へ戻る" });
    const loading = screen.getByRole("status", { name: "読み込み中" });
    expect(precedes(back, loading)).toBe(true);
  });

  it("失敗しても「一覧へ戻る」が先に出て、映像は出ない", async () => {
    getVideo.mockRejectedValue(new Error("つながりません"));
    await show();

    const back = screen.getByRole("link", { name: "← 一覧へ戻る" });
    const notice = screen.getByText("動画を開けません");
    expect(precedes(back, notice)).toBe(true);
    expect(screen.getByText("つながりません")).toBeDefined();

    // 取得できていないのだから、再生の入口を出しても押せることは無い。
    expect(document.querySelector("video")).toBeNull();
  });

  it("id が不正なら取得に行かず、失敗として扱う", async () => {
    await show("/videos/なにか");

    expect(getVideo.mock.calls).toHaveLength(0);
    expect(screen.getByText("動画の指定が正しくありません")).toBeDefined();
    expect(document.querySelector("video")).toBeNull();
  });
});

describe("VideoPage の情報欄（data-model.md 3. / FR-013）", () => {
  it("解析が済んでいれば値が並び、無い値だけが「なし」になる", async () => {
    getVideo.mockResolvedValue({ ...done, audioCodec: undefined } satisfies Video);
    await show();

    const list = within(facts());
    expect(list.getByText("10:00")).toBeDefined();
    expect(list.getByText("1920 × 1080")).toBeDefined();
    expect(list.getByText("mp4")).toBeDefined();
    expect(list.getByText("h264")).toBeDefined();
    // 音の無い動画では、値が無いこと自体が情報である。
    expect(list.getByText("なし")).toBeDefined();
  });

  it("解析待ちは「確認中」で、空欄にしない", async () => {
    getVideo.mockResolvedValue({
      id: 7,
      title: "取り込んだばかり",
      sizeBytes: 1024,
      addedAt: "2026-09-13T00:00:00Z",
      playable: false,
      probeState: "pending",
      thumbnailState: "pending",
    } satisfies Video);
    await show();

    // 長さ・解像度・形式・映像・音声の 5 つすべてが言い分けられる。
    expect(within(facts()).getAllByText("確認中")).toHaveLength(5);
  });

  it("解析に失敗したら理由を併記し、空欄にしない", async () => {
    getVideo.mockResolvedValue({
      id: 7,
      title: "壊れた動画",
      sizeBytes: 1024,
      addedAt: "2026-09-13T00:00:00Z",
      playable: false,
      probeState: "failed",
      probeError: "ffprobe が終了コード 1 で終わりました",
      thumbnailState: "failed",
    } satisfies Video);
    await show();

    expect(
      within(facts()).getAllByText(
        "読み取れませんでした (ffprobe が終了コード 1 で終わりました)",
      ),
    ).toHaveLength(5);
  });
});

describe("VideoPage の知らせ（contracts/screen-states.md 2.「並び」）", () => {
  it("再生できない形式は、再生を試みる前に映像より上へ出る", async () => {
    getVideo.mockResolvedValue({
      ...done,
      playable: false,
      unplayableReason: "video_codec",
      videoCodec: "hevc",
    } satisfies Video);
    await show();

    const warning = screen.getByText(
      "映像の形式 (hevc) は再生できません。ファイルは取得できますが、ブラウザでそのまま再生できない可能性があります。",
    );
    // 再生を試みる前である = 映像が読み込まれる前から出ている。
    expect(precedes(warning, player())).toBe(true);
  });

  it("続きから始まった知らせと「先頭から見直す」が映像より前に出る", async () => {
    getVideo.mockResolvedValue({
      ...done,
      progress: {
        positionMs: 65_000,
        completed: false,
        updatedAt: "2026-09-13T00:00:00Z",
      },
    } satisfies Video);
    await show();

    const element = player();
    let currentTime = 0;
    Object.defineProperty(element, "currentTime", {
      configurable: true,
      get: () => currentTime,
      set: (value: number) => {
        currentTime = value;
      },
    });

    await act(async () => {
      fireEvent.loadedMetadata(element);
      await Promise.resolve();
    });

    // 中断位置へ飛んでいる（002 のまま。FR-025）。
    expect(currentTime).toBe(65);

    const notice = screen.getByText("1:05 から再開しました");
    const restart = screen.getByRole("button", { name: "先頭から見直す" });
    // 映像の上に重ねると映像を隠す（FR-014）。情報欄の下だと気付かれない。
    expect(precedes(notice, element)).toBe(true);
    expect(precedes(restart, element)).toBe(true);
  });
});
