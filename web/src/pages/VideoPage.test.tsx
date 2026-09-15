import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";

/**
 * 再生画面の 4 状態と構図（contracts/layout.md 4. / FR-015〜FR-017）。
 *
 * 005 で縦 5 段から 2 分割になった。確かめる事柄は 2 つで、004 から引き継ぐ。
 *
 * **× がどの状態でも出る**こと。取得に失敗した画面から戻れないと、利用者に
 * 残る手が再読み込みしかなくなる（004 の「一覧へ戻る」が持っていた役割を、
 * 005 では C15 の × が持つ）。
 *
 * **知らせがパネルの中にあり、映像の外にある**こと。004 では「知らせが映像より
 * 前にある」ことで確かめていた。005 は知らせを別の器（パネル）へ移したので、
 * 器の内外で確かめる ── 知らせが何段増えても映像の大きさは変わらず
 * （spec US5-6）、× も映像に重ならない（FR-017）。
 *
 * **画面幅による出し分け（2 分割 / 縦の畳み）は確かめない。** jsdom は CSS
 * メディアクエリを解決しないので、擬似的に確かめると「テストは通るが画面は
 * 壊れている」状態を招く（R-511）。人が quickstart S6 で見る。
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

/** panel は情報パネル（C14）を返す。 */
function panel(): HTMLElement {
  const element = document.querySelector("aside");
  if (element === null) {
    throw new Error("情報パネルが描かれていない");
  }
  return element;
}

/** facts は情報欄（C16 の dl）を返す。知らせの文言と取り違えないよう範囲を絞る。 */
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

/** close は一覧へ戻る × （C15）を返す。 */
function close(): HTMLElement {
  return screen.getByRole("link", { name: "一覧へ戻る" });
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

describe("VideoPage の 4 状態（contracts/layout.md 4.）", () => {
  it("通信中もパネルの × が出て、映像の場所には骨組みが出る", async () => {
    getVideo.mockReturnValue(new Promise<Video>(() => undefined));
    await show();

    // 戻る道は取得の成否によらず出ている。
    expect(panel().contains(close())).toBe(true);

    // 骨組みは映像の側にあり、パネルの中ではない。
    const loading = screen.getByRole("status", { name: "読み込み中" });
    expect(panel().contains(loading)).toBe(false);
  });

  it("失敗してもパネルの × が出て、理由はパネルの中に出る。映像は出ない", async () => {
    getVideo.mockRejectedValue(new Error("つながりません"));
    await show();

    expect(panel().contains(close())).toBe(true);

    const list = within(panel());
    expect(list.getByText("動画を開けません")).toBeDefined();
    expect(list.getByText("つながりません")).toBeDefined();

    // 取得できていないのだから、再生の入口を出しても押せることは無い。
    expect(document.querySelector("video")).toBeNull();
  });

  it("id が不正なら取得に行かず、失敗として扱う", async () => {
    await show("/videos/なにか");

    expect(getVideo.mock.calls).toHaveLength(0);
    expect(within(panel()).getByText("動画の指定が正しくありません")).toBeDefined();
    expect(document.querySelector("video")).toBeNull();
  });

  it("× の行き先は遷移元の一覧で、遷移元が無ければ一覧の先頭である", async () => {
    await show();

    // 直接 /videos/7 を開いた場合。復元の控えを持たない側へ落とす。
    expect(close().getAttribute("href")).toBe("/");
  });
});

describe("VideoPage のパネル（contracts/layout.md 4. / C14・C15・C16）", () => {
  it("題名と情報はパネルの中にあり、映像はパネルの外にある", async () => {
    await show();

    const heading = screen.getByRole("heading", { level: 1, name: "長い動画" });
    expect(panel().contains(heading)).toBe(true);
    expect(panel().contains(facts())).toBe(true);

    // 映像がパネルの外にあることが、パネルの中身が増えても映像の大きさが
    // 変わらないことの根拠である（spec US5-6）。
    expect(panel().contains(player())).toBe(false);
  });

  it("画面の見出し文字は出さない。見出しはパネルの題名だけである（FR-015）", async () => {
    await show();

    expect(screen.getAllByRole("heading")).toHaveLength(1);
  });
});

describe("VideoPage の情報欄（data-model.md 4. / FR-016）", () => {
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

describe("VideoPage の知らせ（contracts/layout.md 4.「パネルの中身の順序」）", () => {
  it("再生できない形式は、再生を試みる前にパネルの中へ出る", async () => {
    getVideo.mockResolvedValue({
      ...done,
      playable: false,
      unplayableReason: "video_codec",
      videoCodec: "hevc",
    } satisfies Video);
    await show();

    // 再生を試みる前である = 映像が読み込まれる前から出ている。
    const warning = screen.getByText(
      "映像の形式 (hevc) は再生できません。ファイルは取得できますが、ブラウザでそのまま再生できない可能性があります。",
    );
    expect(panel().contains(warning)).toBe(true);
    expect(panel().contains(player())).toBe(false);
  });

  it("続きから始まった知らせと「先頭から見直す」がパネルの中へ出る", async () => {
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

    // 中断位置へ飛んでいる（002 のまま。FR-018）。
    expect(currentTime).toBe(65);

    const notice = screen.getByText("1:05 から再開しました");
    const restart = screen.getByRole("button", { name: "先頭から見直す" });
    // 知らせがパネルの中に閉じているので、映像の大きさは変わらない
    // （spec US5-6 / US4-6）。
    expect(panel().contains(notice)).toBe(true);
    expect(panel().contains(restart)).toBe(true);
    expect(panel().contains(element)).toBe(false);
  });

  it("知らせは題名の下、動画の情報の上に入る（パネルの中身の順序）", async () => {
    getVideo.mockResolvedValue({
      ...done,
      playable: false,
      unplayableReason: "video_codec",
      videoCodec: "hevc",
    } satisfies Video);
    await show();

    const nodes = [...panel().children];
    const index = (node: Element | null): number =>
      nodes.findIndex((child) => child === node || child.contains(node));

    const heading = screen.getByRole("heading", { level: 1, name: "長い動画" });
    const warning = screen.getByText(/映像の形式 \(hevc\) は再生できません/);

    // × → 題名 → 知らせ → 動画の情報（spec US5-3）。
    expect(index(close())).toBeLessThan(index(heading));
    expect(index(heading)).toBeLessThan(index(warning));
    expect(index(warning)).toBeLessThan(index(facts()));
  });
});
