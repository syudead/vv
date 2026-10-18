import videojs from "video.js";

import type { Video } from "../api/client";
import { t } from "../i18n";
import { formatDuration } from "../lib/format";

type ApiChapter = NonNullable<NonNullable<Video["mediaInfo"]>["chapters"]>[number];

export interface PlayerChapter {
  startMs: number;
  endMs: number;
  title: string;
}

/** Name untitled chapters by order and defensively clip chapter bounds to the video duration. */
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

/** Choose the latest-starting chapter containing start <= position < end; later entries win ties. Return -1 outside chapters. */
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

export function chapterTitleAt(
  chapters: readonly PlayerChapter[],
  positionMs: number,
): string | undefined {
  return chapters[chapterIndexAt(chapters, positionMs)]?.title;
}

/** Return unique chapter starts as percentages of the seek bar. */
export function chapterMarkPercents(
  chapters: readonly PlayerChapter[],
  durationMs: number,
): number[] {
  const starts = [...new Set(chapters.map((chapter) => chapter.startMs))];
  return starts.map((start) => (start / durationMs) * 100);
}

/** Install noninteractive, aria-hidden chapter marks; the menu exposes the same chapters. Return cleanup. */
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

const states = new WeakMap<object, ChapterState>();

type CueConstructor = new (start: number, end: number, text: string) => CueLike;

function cueConstructor(): CueConstructor | undefined {
  const scope = window as unknown as {
    VTTCue?: CueConstructor;
    vttjs?: { VTTCue?: CueConstructor };
  };
  return scope.VTTCue ?? scope.vttjs?.VTTCue;
}

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
  // Disabled tracks have no cues; use hidden mode for chapter tracks.
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

/** Register before player creation. Preserve raw titles and choose overlapping chapters by logical time with chapterIndexAt. */
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
    /** Cache the selected index to avoid updating on every timeupdate. */
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
