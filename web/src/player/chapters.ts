import videojs from "video.js";

import type { Video } from "../api/client";
import { t } from "../i18n";
import { formatDuration } from "../lib/format";

/**
 * プレーヤーのチャプター。
 *
 * 詳細の `mediaInfo.chapters` から `chapters` の text track を作り、チャプターごとに cue を 1 つ
 * 足す。シークバーの印（attachChapterMarks）、シークプレビューのタイトル（chapterTitleAt）、
 * 操作バーのチャプターのボタン（registerChaptersButton）は同じ一覧を読む。チャプターの無い
 * 動画にはトラックも印もボタンも無い。
 */

type ApiChapter = NonNullable<NonNullable<Video["mediaInfo"]>["chapters"]>[number];

/** PlayerChapter はプレーヤーで使うチャプターである。title はタイトルの無いとき `Chapter {n}`。 */
export interface PlayerChapter {
  startMs: number;
  endMs: number;
  title: string;
}

/**
 * playerChapters は、詳細のチャプター（開始の順）をプレーヤーのチャプターにする。タイトルの
 * 無いチャプターは開始の順の番号で呼ぶ。サーバーが長さで切った一覧を受けるが、念のため
 * 長さを過ぎる開始は落とし、終わりを長さで切る。
 */
export function playerChapters(
  chapters: readonly ApiChapter[] | undefined,
  durationMs: number | undefined,
): PlayerChapter[] {
  if (chapters === undefined || durationMs === undefined || durationMs <= 0) return [];
  const result: PlayerChapter[] = [];
  chapters.forEach((chapter, index) => {
    if (chapter.startMs >= durationMs) return;
    const title = chapter.title ?? "";
    result.push({
      startMs: chapter.startMs,
      endMs: Math.max(chapter.startMs, Math.min(chapter.endMs, durationMs)),
      title: title === "" ? t.player.chapters.untitled(index + 1) : title,
    });
  });
  return result;
}

/**
 * chapterIndexAt は位置 positionMs のチャプターの番号を返す。位置を含む（開始 ≦ 位置 < 終わり）
 * チャプターのうち開始がいちばん遅いもの、同じ開始なら開始の順で後のもので、無ければ -1。
 * シークプレビューのタイトルとメニューの選択の印はこの規則を共有する。
 */
export function chapterIndexAt(
  chapters: readonly Pick<PlayerChapter, "startMs" | "endMs">[],
  positionMs: number,
): number {
  let found = -1;
  chapters.forEach((chapter, index) => {
    if (chapter.startMs <= positionMs && positionMs < chapter.endMs) found = index;
  });
  return found;
}

/** chapterTitleAt は位置 positionMs のチャプターのタイトルを返す。チャプターの外は undefined。 */
export function chapterTitleAt(
  chapters: readonly PlayerChapter[],
  positionMs: number,
): string | undefined {
  return chapters[chapterIndexAt(chapters, positionMs)]?.title;
}

/**
 * chapterMarkPercents は印を描く位置（バーの幅に対する %）を返す。同じ開始のチャプターは
 * 印 1 つにまとめる。
 */
export function chapterMarkPercents(
  chapters: readonly PlayerChapter[],
  durationMs: number,
): number[] {
  const starts = [...new Set(chapters.map((chapter) => chapter.startMs))];
  return starts.map((start) => (start / durationMs) * 100);
}

/**
 * attachChapterMarks はシークバー（`.vjs-progress-holder`）の中にチャプターの開始の印を描く。
 * 印は pointer を取らず、読み上げない（メニューが同じチャプターを並べる）。戻り値で外す。
 */
export function attachChapterMarks(
  progress: HTMLElement,
  chapters: readonly PlayerChapter[],
  durationMs: number,
): () => void {
  const marks = document.createElement("div");
  marks.className = "vv-chapter-marks";
  marks.setAttribute("aria-hidden", "true");
  for (const percent of chapterMarkPercents(chapters, durationMs)) {
    const mark = document.createElement("div");
    mark.className = "vv-chapter-mark";
    mark.style.left = `${String(percent)}%`;
    marks.append(mark);
  }
  progress.append(marks);
  return () => marks.remove();
}

interface CueLike {
  startTime: number;
  endTime: number;
  text: string;
}

interface ChaptersTextTrack {
  mode: string;
  addCue(cue: CueLike): void;
}

/** ChaptersPlayer は、チャプターのトラックを付けるために触る video.js の Player の部分である。 */
export interface ChaptersPlayer {
  addRemoteTextTrack(
    options: { kind: "chapters"; label: string; default: boolean },
    manualCleanup: boolean,
  ): { track?: ChaptersTextTrack } | undefined;
  removeRemoteTextTrack(track: ChaptersTextTrack): void;
  getChild?(name: string): unknown;
}

interface ChapterState {
  chapters: readonly PlayerChapter[];
  cues: readonly CueLike[];
}

/** プレイヤーごとの今のチャプター。ボタンはここから項目を作る。 */
const states = new WeakMap<object, ChapterState>();

type CueConstructor = new (start: number, end: number, text: string) => CueLike;

function cueConstructor(): CueConstructor | undefined {
  const scope = window as unknown as {
    VTTCue?: CueConstructor;
    vttjs?: { VTTCue?: CueConstructor };
  };
  return scope.VTTCue ?? scope.vttjs?.VTTCue;
}

/**
 * attachChaptersTrack はチャプターごとの cue を持つ `chapters` のトラックをプレイヤーに付け、
 * チャプターのボタンを作り直す。チャプターが無ければ何もしない。戻り値でトラックを外す。
 */
export function attachChaptersTrack(
  player: ChaptersPlayer,
  chapters: readonly PlayerChapter[],
): () => void {
  const Cue = cueConstructor();
  if (chapters.length === 0 || Cue === undefined) return () => undefined;
  const track = player.addRemoteTextTrack(
    { kind: "chapters", label: t.player.chapters.button, default: false },
    true,
  )?.track;
  if (track === undefined) return () => undefined;
  // disabled のトラックは cue の一覧を持たない。チャプターのボタンも hidden にする。
  track.mode = "hidden";
  const cues = chapters.map(
    (chapter) => new Cue(chapter.startMs / 1000, chapter.endMs / 1000, chapter.title),
  );
  for (const cue of cues) track.addCue(cue);
  states.set(player, { chapters, cues });
  updateButton(player);
  return () => {
    states.delete(player);
    player.removeRemoteTextTrack(track);
    updateButton(player);
  };
}

function updateButton(player: ChaptersPlayer) {
  const bar = player.getChild?.("ControlBar") as
    { getChild(name: string): unknown } | undefined;
  const button = bar?.getChild(chaptersButtonComponent) as { update(): void } | undefined;
  button?.update();
}

/** chaptersButtonName は操作バーの `children` に書くチャプターのボタンの名前である。 */
export const chaptersButtonName = "chaptersMenuButton";
const chaptersButtonComponent = "ChaptersMenuButton";

type Player = ReturnType<typeof videojs>;

interface MenuItemLike {
  el(): HTMLElement;
  selected(value: boolean): void;
}

interface ChaptersButtonLike {
  items?: MenuItemLike[];
  player(): Player;
  on(target: unknown, event: string, listener: () => void): void;
  createItems(): MenuItemLike[];
}

type ChaptersButtonClass = new (
  player: Player,
  options?: Record<string, unknown>,
) => ChaptersButtonLike;
type MenuItemClass = new (
  player: Player,
  options: { track: unknown; cue: CueLike },
) => MenuItemLike;

let registered = false;

/**
 * registerChaptersButton はチャプターのボタンの部品を video.js に登録する。プレイヤーを作る前に
 * 呼ぶ。2 回目からは何もしない。
 *
 * video.js の `ChaptersButton` との違いは 2 つある。項目の文字を `{start} {title}` にする
 * （`ChaptersTrackMenuItem` は cue の文字だけを、しかも言語の表に通して出す）。選択の印を
 * chapterIndexAt の規則で付ける（video.js は重なるチャプターの先のものを選ぶ）。印は論理上の
 * 再生位置（player.currentTime）で決めるので、変換して再生している間も正しい。
 */
export function registerChaptersButton(): void {
  if (registered) return;
  const ChaptersButton = videojs.getComponent(
    "ChaptersButton",
  ) as unknown as ChaptersButtonClass;
  const ChaptersTrackMenuItem = videojs.getComponent(
    "ChaptersTrackMenuItem",
  ) as unknown as MenuItemClass;

  class ChapterMenuItem extends ChaptersTrackMenuItem {
    constructor(player: Player, options: { track: unknown; cue: CueLike }) {
      super(player, options);
      const text = this.el().querySelector(".vjs-menu-item-text");
      if (text === null) return;
      const start = document.createElement("span");
      start.className = "vv-chapter-start";
      start.textContent = formatDuration(options.cue.startTime * 1000);
      text.replaceChildren(start, ` ${options.cue.text}`);
    }
  }

  class ChaptersMenuButton extends ChaptersButton {
    selectCurrentItem_: () => void;
    /** 印を付けている項目の番号。timeupdate ごとに印を付け直さないために持つ。 */
    playingIndex = -1;

    constructor(player: Player, options?: Record<string, unknown>) {
      super(player, options);
      this.selectCurrentItem_ = () => this.selectPlaying();
      this.on(player, "timeupdate", this.selectCurrentItem_);
    }

    override createItems(): MenuItemLike[] {
      const state = states.get(this.player());
      const track = (this as unknown as { track_?: unknown }).track_;
      if (state === undefined || track === undefined) return [];
      const player = this.player();
      const items = state.cues.map((cue) => new ChapterMenuItem(player, { track, cue }));
      this.playingIndex = playingIndex(state, player);
      items.forEach((item, index) => item.selected(index === this.playingIndex));
      return items;
    }

    selectPlaying() {
      const state = states.get(this.player());
      if (state === undefined || this.items === undefined) return;
      const next = playingIndex(state, this.player());
      if (next === this.playingIndex) return;
      this.items[this.playingIndex]?.selected(false);
      this.items[next]?.selected(true);
      this.playingIndex = next;
    }
  }

  videojs.registerComponent(
    chaptersButtonComponent,
    ChaptersMenuButton as unknown as Parameters<typeof videojs.registerComponent>[1],
  );
  registered = true;
}

function playingIndex(state: ChapterState, player: Player): number {
  const seconds = player.currentTime();
  return chapterIndexAt(state.chapters, typeof seconds === "number" ? seconds * 1000 : 0);
}
