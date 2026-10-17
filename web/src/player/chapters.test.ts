import videojs from "video.js";
import { afterEach, describe, expect, it } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import {
  attachChapterMarks,
  attachChaptersTrack,
  chapterIndexAt,
  chaptersButtonName,
  chapterTitleAt,
  playerChapters,
  registerChaptersButton,
  type ChaptersPlayer,
} from "./chapters";
import { attachSeekPreview } from "./seekPreview";

const durationMs = 60_000;

/** 3 つのチャプター。2 つ目はタイトルが無く、3 つ目は長さを過ぎて終わる。 */
const threeChapters = [
  { startMs: 0, endMs: 12_000, title: "Opening" },
  { startMs: 12_000, endMs: 30_000 },
  { startMs: 30_000, endMs: 90_000, title: "Ending" },
];

describe("playerChapters", () => {
  it("タイトルの無いチャプターは開始の順の番号で呼び、長さを過ぎる終わりは長さで切る", () => {
    expect(playerChapters(threeChapters, durationMs)).toEqual([
      { startMs: 0, endMs: 12_000, title: "Opening" },
      { startMs: 12_000, endMs: 30_000, title: "Chapter 2" },
      { startMs: 30_000, endMs: 60_000, title: "Ending" },
    ]);
  });

  it("チャプターが無い・長さが無いときは空で、長さ以降に始まるチャプターは落とす", () => {
    expect(playerChapters(undefined, durationMs)).toEqual([]);
    expect(playerChapters(threeChapters, undefined)).toEqual([]);
    expect(
      playerChapters([{ startMs: 60_000, endMs: 70_000, title: "Past" }], durationMs),
    ).toEqual([]);
  });
});

describe("chapterIndexAt", () => {
  const chapters = playerChapters(threeChapters, durationMs);

  it("位置を含むチャプターを返し、外は -1 を返す", () => {
    expect(chapterIndexAt(chapters, 0)).toBe(0);
    expect(chapterIndexAt(chapters, 11_999)).toBe(0);
    expect(chapterIndexAt(chapters, 12_000)).toBe(1);
    expect(chapterIndexAt(chapters, 45_000)).toBe(2);
    expect(chapterIndexAt(chapters, 60_000)).toBe(-1);
    expect(chapterIndexAt([{ startMs: 5000, endMs: 8000 }], 1000)).toBe(-1);
  });

  it("重なるチャプターは開始の遅いもの、同じ開始なら開始の順で後のものを選ぶ", () => {
    const overlapping = [
      { startMs: 0, endMs: 50_000 },
      { startMs: 10_000, endMs: 20_000 },
      { startMs: 10_000, endMs: 15_000 },
    ];
    expect(chapterIndexAt(overlapping, 12_000)).toBe(2);
    expect(chapterIndexAt(overlapping, 17_000)).toBe(1);
    expect(chapterIndexAt(overlapping, 30_000)).toBe(0);
  });

  it("プレビューのタイトルはその位置のチャプターのもので、外では無い", () => {
    expect(chapterTitleAt(chapters, 20_000)).toBe("Chapter 2");
    expect(chapterTitleAt(chapters, 31_000)).toBe("Ending");
    expect(
      chapterTitleAt([{ startMs: 5000, endMs: 8000, title: "A" }], 0),
    ).toBeUndefined();
  });
});

describe("attachChapterMarks", () => {
  it("チャプターの開始ごとに、バーの幅の開始÷長さの位置へ印を描き、外すと消える", () => {
    const progress = document.createElement("div");
    const detach = attachChapterMarks(
      progress,
      playerChapters(threeChapters, durationMs),
      durationMs,
    );
    const container = progress.querySelector(".vv-chapter-marks");
    expect(container?.getAttribute("aria-hidden")).toBe("true");
    const marks = Array.from(
      progress.querySelectorAll<HTMLElement>(".vv-chapter-mark"),
      (mark) => mark.style.left,
    );
    expect(marks).toEqual(["0%", "20%", "50%"]);
    detach();
    expect(progress.querySelector(".vv-chapter-marks")).toBeNull();
  });

  it("同じ開始のチャプターは印 1 つにする", () => {
    const progress = document.createElement("div");
    attachChapterMarks(
      progress,
      [
        { startMs: 0, endMs: 10_000, title: "A" },
        { startMs: 6000, endMs: 10_000, title: "B" },
        { startMs: 6000, endMs: 9000, title: "C" },
      ],
      durationMs,
    );
    expect(progress.querySelectorAll(".vv-chapter-mark")).toHaveLength(2);
  });
});

describe("シークプレビューのチャプターのタイトル", () => {
  function hover(target: HTMLElement, clientX: number) {
    target.dispatchEvent(
      new PointerEvent("pointermove", { clientX, pointerType: "mouse", bubbles: true }),
    );
  }

  function progressBar() {
    const progress = document.createElement("div");
    progress.getBoundingClientRect = () =>
      ({ left: 0, width: 600, top: 0, right: 600, bottom: 10, height: 10 }) as DOMRect;
    document.body.append(progress);
    return progress;
  }

  it("スプライトの無い動画でも、時刻の上にその位置のチャプターのタイトルを出す", () => {
    const progress = progressBar();
    const chapters = playerChapters(threeChapters, durationMs);
    const requests: string[] = [];
    const detach = attachSeekPreview(progress, {
      durationMs,
      thumbnailUrl: undefined,
      chapterTitleAt: (positionMs) => chapterTitleAt(chapters, positionMs),
      fetchSprite: (url) => {
        requests.push(url);
        return Promise.reject(new Error("unused"));
      },
    });
    const preview = progress.querySelector<HTMLElement>(".vv-seek-preview");
    const title = progress.querySelector(".vv-seek-preview-title");

    // 600px のバーの 150px は 15 秒で、2 つ目のチャプターの中にある。
    hover(progress, 150);
    expect(preview?.dataset.state).toBe("unavailable");
    expect(title?.textContent).toBe("Chapter 2");
    expect(preview?.textContent).toBe("Chapter 20:15");
    hover(progress, 420);
    expect(title?.textContent).toBe("Ending");
    expect(requests).toEqual([]);
    detach();
    progress.remove();
  });

  it("チャプターの外の位置では時刻だけを出す", () => {
    const progress = progressBar();
    const chapters = [{ startMs: 30_000, endMs: 40_000, title: "Middle" }];
    const detach = attachSeekPreview(progress, {
      durationMs,
      thumbnailUrl: undefined,
      chapterTitleAt: (positionMs) => chapterTitleAt(chapters, positionMs),
    });
    hover(progress, 60);
    expect(progress.querySelector(".vv-seek-preview")?.textContent).toBe("0:06");
    hover(progress, 330);
    expect(progress.querySelector(".vv-seek-preview-title")?.textContent).toBe("Middle");
    detach();
    progress.remove();
  });
});

/**
 * チャプターのボタンとメニューを本物の video.js で組む。
 */
describe("ChaptersMenuButton", () => {
  let player: ReturnType<typeof videojs> | undefined;

  afterEach(() => {
    player?.dispose();
    player = undefined;
  });

  function create() {
    registerChaptersButton();
    const element = document.createElement("video-js");
    document.body.append(element);
    player = videojs(element, {
      controls: true,
      controlBar: {
        children: [
          "playToggle",
          "customControlSpacer",
          chaptersButtonName,
          "playbackRateMenuButton",
        ],
      },
    });
    const host = player.el();
    const wrapper = host.querySelector<HTMLElement>(
      ".vjs-chapters-button.vjs-menu-button",
    );
    if (wrapper === null) throw new Error("チャプターのボタンがありません");
    return { host, wrapper, player };
  }

  function items(wrapper: HTMLElement) {
    return Array.from(wrapper.querySelectorAll(".vjs-menu-item"), (item) => ({
      text: item.querySelector(".vjs-menu-item-text")?.textContent,
      selected: item.classList.contains("vjs-selected"),
    }));
  }

  it("チャプターの無い動画ではボタンを隠す", () => {
    const { wrapper } = create();
    expect(wrapper.classList.contains("vjs-hidden")).toBe(true);
    expect(items(wrapper)).toEqual([]);
  });

  it("チャプターごとに cue を作り、メニューに `{start} {title}` を開始の順に並べる", () => {
    const { wrapper, player: current } = create();
    const detach = attachChaptersTrack(
      current as unknown as ChaptersPlayer,
      playerChapters(threeChapters, durationMs),
    );
    const tracks = current.textTracks() as unknown as ArrayLike<{
      kind: string;
      mode: string;
      cues: ArrayLike<{ startTime: number; endTime: number; text: string }> | null;
    }>;
    const track = Array.from(tracks).find((item) => item.kind === "chapters");
    expect(track?.mode).toBe("hidden");
    expect(
      Array.from(track?.cues ?? [], (cue) => [cue.startTime, cue.endTime, cue.text]),
    ).toEqual([
      [0, 12, "Opening"],
      [12, 30, "Chapter 2"],
      [30, 60, "Ending"],
    ]);

    expect(wrapper.classList.contains("vjs-hidden")).toBe(false);
    expect(wrapper.querySelector(".vjs-menu-title")?.textContent).toBe("Chapters");
    expect(items(wrapper)).toEqual([
      { text: "0:00 Opening", selected: true },
      { text: "0:12 Chapter 2", selected: false },
      { text: "0:30 Ending", selected: false },
    ]);

    detach();
    expect(wrapper.classList.contains("vjs-hidden")).toBe(true);
  });

  it("タイトルが言語の表の鍵と同じでも、そのまま出す", () => {
    videojs.addLanguage("en", { Mute: "Mute (M)" });
    const { wrapper, player: current } = create();
    attachChaptersTrack(
      current as unknown as ChaptersPlayer,
      playerChapters([{ startMs: 0, endMs: 1000, title: "Mute" }], durationMs),
    );
    expect(items(wrapper)[0]?.text).toBe("0:00 Mute");
  });

  it("チャプターを選ぶとその開始へ移動し、再生の印は今の位置のチャプターに移る", () => {
    const { wrapper, player: current } = create();
    attachChaptersTrack(
      current as unknown as ChaptersPlayer,
      playerChapters(threeChapters, durationMs),
    );
    let time = 0;
    const seeks: number[] = [];
    const media = current as unknown as { currentTime(seconds?: number): number };
    media.currentTime = (seconds?: number) => {
      if (seconds !== undefined) {
        time = seconds;
        seeks.push(seconds);
      }
      return time;
    };
    const paused = current.paused();

    const ending = wrapper.querySelectorAll<HTMLElement>(".vjs-menu-item")[2];
    ending?.click();
    expect(seeks).toEqual([30]);
    expect(current.paused()).toBe(paused);
    current.trigger("timeupdate");
    expect(items(wrapper).map((item) => item.selected)).toEqual([false, false, true]);

    time = 13;
    current.trigger("timeupdate");
    expect(items(wrapper).map((item) => item.selected)).toEqual([false, true, false]);
  });

  it("重なるチャプターでは、プレビューと同じチャプターに印を付ける", () => {
    const { wrapper, player: current } = create();
    const media = current as unknown as { currentTime(seconds?: number): number };
    media.currentTime = () => 12;
    attachChaptersTrack(
      current as unknown as ChaptersPlayer,
      playerChapters(
        [
          { startMs: 0, endMs: 50_000, title: "Long" },
          { startMs: 10_000, endMs: 20_000, title: "Inner" },
        ],
        durationMs,
      ),
    );
    expect(items(wrapper)).toEqual([
      { text: "0:00 Long", selected: false },
      { text: "0:10 Inner", selected: true },
    ]);
  });

  it("疑似ロケールで、メニューの題とタイトルの無いチャプターの名前がカタログから出る", () => {
    enablePseudoLocale();
    const { wrapper, player: current } = create();
    attachChaptersTrack(
      current as unknown as ChaptersPlayer,
      playerChapters([{ startMs: 0, endMs: 1000 }], durationMs),
    );
    const title = wrapper.querySelector(".vjs-menu-title");
    const item = wrapper.querySelector(".vjs-menu-item-text");
    if (title === null || item === null) throw new Error("文言の要素がありません");
    expectCatalogTextOnly(title);
    expectCatalogTextOnly(item, ["0:00"]);
  });
});
