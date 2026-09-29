import { subtitleUrl, type SubtitleTrack } from "../api/client";
import { t } from "../i18n";
import {
  readSubtitlePreference,
  writeSubtitlePreference,
} from "../preferences/subtitlePreference";

/**
 * 動画の隣に置いた字幕を video.js のトラックとして付け、選択を覚える
 * （specs/028-sidecar-subtitles research.md R-9〜R-11）。
 *
 * - トラックは `kind: "subtitles"` の remote text track として付ける。字幕ボタンとメニューは
 *   video.js の `SubsCapsButton` が出し、トラックが無ければボタンは自分を隠す。
 * - 付けたとき、保存値が `enabled: true` でそのラベルのトラックがあればそれを表示する。
 *   無ければオフにし、保存値は書き換えない。ラベルはサーバーと同じ規則（NFC 正規化と
 *   case folding）で比べる。
 * - 保存するのは、利用者がメニューか `c` キーで選択を変えたときだけである。
 */

/** TextTrackLike は、ここで触る TextTrack の部分である。 */
export interface TextTrackLike {
  mode: string;
}

/** TextTrackListLike は、ここで触る TextTrackList の部分である。 */
export interface TextTrackListLike {
  addEventListener(type: "change", listener: () => void): void;
  removeEventListener(type: "change", listener: () => void): void;
}

/** SubtitlePlayer は、ここで使う video.js の Player の部分である。 */
export interface SubtitlePlayer {
  textTracks(): TextTrackListLike;
  addRemoteTextTrack(
    options: { kind: "subtitles"; src: string; label: string; default: boolean },
    manualCleanup: boolean,
  ): { track?: TextTrackLike } | undefined;
  removeRemoteTextTrack(track: TextTrackLike): void;
}

export interface SubtitleTracks {
  /**
   * replace は、動画 videoId の字幕の一覧でトラックを作り直す。start の前に呼んだときは
   * 一覧を覚え、start で付ける。同じ動画の同じ一覧（参照が同じ）なら何もしない。
   */
  replace(videoId: number, subtitles: readonly SubtitleTrack[]): void;
  /** start はプレイヤーが準備できたときに呼ぶ。覚えた一覧のトラックを付ける。 */
  start(): void;
  /**
   * toggle は `c` キーの切り替えである。トラックが無ければ何もしない。表示中ならオフに、
   * オフなら保存済みのラベルのトラック（無ければメニューの最初のトラック）を表示し、保存する。
   */
  toggle(): void;
  dispose(): void;
}

/**
 * subtitleLabelKey は字幕のラベルを比べるための鍵である。サーバーの `SubtitleSidecars` と同じく、
 * NFC 正規化のうえ文字ごとの case folding で同じとみなす文字をそろえる（`JA` と `ja`、
 * `Σ`・`σ`・`ς`）。文字が複数の文字に変わる大文字・小文字の変換（`ß` → `SS`）は使わない。
 */
export function subtitleLabelKey(label: string): string {
  let key = "";
  for (const char of label.normalize("NFC")) {
    const upper = char.toUpperCase();
    const base = [...upper].length === 1 ? upper : char;
    const lower = base.toLowerCase();
    key += [...lower].length === 1 ? lower : base;
  }
  return key;
}

/** sameSubtitleLabel は 2 つのラベルが同じ字幕のラベルかを返す（subtitleLabelKey で比べる）。 */
export function sameSubtitleLabel(a: string, b: string): boolean {
  return subtitleLabelKey(a) === subtitleLabelKey(b);
}

/** subtitleDisplayLabel はメニューに出す字幕の名前である。ラベルが無ければカタログの既定の名前。 */
export function subtitleDisplayLabel(label: string): string {
  return label === "" ? t.player.subtitles.default : label;
}

interface Wanted {
  videoId: number;
  subtitles: readonly SubtitleTrack[];
}

interface Attached {
  track: TextTrackLike;
  /** ファイル名のラベル（保存値と比べる値）。ラベルの無い字幕は `""`。 */
  label: string;
}

export function createSubtitleTracks(
  player: SubtitlePlayer,
  storage?: Storage,
): SubtitleTracks {
  let started = false;
  let wanted: Wanted | null = null;
  /** 今付けているトラックの元の一覧。wanted と同じなら付け直さない。 */
  let applied: Wanted | null = null;
  let attached: Attached[] = [];
  /** 自分で表示を変えている間。その間に届く change は利用者の選択ではない。 */
  let applying = false;
  /** 最後に見た表示中のラベル（オフなら null）。change と比べて、変わったときだけ保存する。 */
  let selection: string | null = null;
  let disposed = false;

  const showing = (): string | null =>
    attached.find((entry) => entry.track.mode === "showing")?.label ?? null;

  const show = (label: string | null) => {
    applying = true;
    try {
      let shown: string | null = null;
      for (const entry of attached) {
        const on =
          shown === null && label !== null && sameSubtitleLabel(entry.label, label);
        if (on) shown = entry.label;
        entry.track.mode = on ? "showing" : "disabled";
      }
      selection = shown;
    } finally {
      applying = false;
    }
  };

  const save = (label: string | null) => {
    if (label === null) {
      // オフでも最後に選んだラベルは残し、次の `c` で同じ字幕に戻れるようにする。
      writeSubtitlePreference(
        { enabled: false, label: readSubtitlePreference(storage).label },
        storage,
      );
    } else {
      writeSubtitlePreference({ enabled: true, label }, storage);
    }
  };

  const onChange = () => {
    if (applying || disposed) return;
    const now = showing();
    if (now === selection) return;
    selection = now;
    save(now);
  };
  const list = player.textTracks();
  list.addEventListener("change", onChange);

  const attach = () => {
    if (!started || disposed || wanted === applied) return;
    applying = true;
    try {
      for (const entry of attached) player.removeRemoteTextTrack(entry.track);
      attached = [];
      applied = wanted;
      if (wanted === null) return;
      for (const subtitle of wanted.subtitles) {
        const element = player.addRemoteTextTrack(
          {
            kind: "subtitles",
            src: subtitleUrl(wanted.videoId, subtitle.file, 0),
            label: subtitleDisplayLabel(subtitle.label),
            default: false,
          },
          true,
        );
        if (element?.track !== undefined) {
          attached.push({ track: element.track, label: subtitle.label });
        }
      }
    } finally {
      applying = false;
    }
    const saved = readSubtitlePreference(storage);
    show(saved.enabled ? saved.label : null);
  };

  return {
    replace(videoId, subtitles) {
      if (
        wanted !== null &&
        wanted.videoId === videoId &&
        wanted.subtitles === subtitles
      ) {
        return;
      }
      wanted = { videoId, subtitles };
      attach();
    },
    start() {
      started = true;
      attach();
    },
    toggle() {
      if (disposed || attached.length === 0) return;
      const now = showing();
      if (now !== null) {
        show(null);
        writeSubtitlePreference({ enabled: false, label: now }, storage);
        return;
      }
      const saved = readSubtitlePreference(storage);
      const target =
        attached.find((entry) => sameSubtitleLabel(entry.label, saved.label)) ??
        attached[0];
      if (target === undefined) return;
      show(target.label);
      save(target.label);
    },
    dispose() {
      disposed = true;
      list.removeEventListener("change", onChange);
    },
  };
}
