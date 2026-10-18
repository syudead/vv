import { subtitleUrl, type SubtitleTrack } from "../api/client";
import { t } from "../i18n";
import {
  readSubtitlePreference,
  writeSubtitlePreference,
} from "../preferences/subtitlePreference";

/** Persist explicit subtitle choices only; offset changes replace tracks while preserving the visible selection. Match labels with the server's NFC and case-folding rules. */

export interface TextTrackLike {
  mode: string;
}

export interface TextTrackListLike {
  addEventListener(type: "change", listener: () => void): void;
  removeEventListener(type: "change", listener: () => void): void;
}

export interface SubtitlePlayer {
  textTracks(): TextTrackListLike;
  addRemoteTextTrack(
    options: { kind: "subtitles"; src: string; label: string; default: boolean },
    manualCleanup: boolean,
  ): { track?: TextTrackLike } | undefined;
  removeRemoteTextTrack(track: TextTrackLike): void;
}

export interface SubtitleTracks {
  /** Remember tracks until start; identical video IDs and array references require no replacement. */
  replace(videoId: number, subtitles: readonly SubtitleTrack[]): void;
  start(): void;
  /** Original-video milliseconds at output time zero; null removes tracks until the offset is known. */
  setOffset(offsetMs: number | null): void;
  /** Toggle off, or restore the saved label/first track; persist only when tracks exist. */
  toggle(): void;
  dispose(): void;
}

/** Match server NFC/simple case folding, including sigma variants; avoid multi-character expansions such as sharp-s to SS. */
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

export function sameSubtitleLabel(a: string, b: string): boolean {
  return subtitleLabelKey(a) === subtitleLabelKey(b);
}

export function subtitleDisplayLabel(label: string): string {
  return label === "" ? t.player.subtitles.default : label;
}

interface Wanted {
  videoId: number;
  subtitles: readonly SubtitleTrack[];
}

interface Attached {
  track: TextTrackLike;
  /** Raw filename label, empty when absent, used for saved selection matching. */
  label: string;
}

export function createSubtitleTracks(
  player: SubtitlePlayer,
  storage?: Storage,
): SubtitleTracks {
  let started = false;
  let wanted: Wanted | null = null;
  let applied: Wanted | null = null;
  let attached: Attached[] = [];
  let offsetMs: number | null = 0;
  let appliedOffset: number | null = 0;
  /** New lists use saved preferences; offset-only replacements retain the current selection. */
  let fromPreference = true;
  /** Ignore changes caused by programmatic track selection. */
  let applying = false;
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
      // Retain the last selected label while off so the next keyboard toggle restores it.
      writeSubtitlePreference(
        { enabled: false, label: readSubtitlePreference(storage).label },
        storage,
      );
    } else {
      writeSubtitlePreference({ enabled: true, label }, storage);
    }
  };

  const onChange = () => {
    if (applying || disposed || attached.length === 0) return;
    const now = showing();
    if (now === selection) return;
    selection = now;
    save(now);
  };
  const list = player.textTracks();
  list.addEventListener("change", onChange);

  const attach = () => {
    if (!started || disposed) return;
    if (wanted === applied && offsetMs === appliedOffset) return;
    if (wanted !== applied) fromPreference = true;
    // Capture pending user selection changes before removing tracks.
    onChange();
    const offset = offsetMs;
    applying = true;
    try {
      for (const entry of attached) player.removeRemoteTextTrack(entry.track);
      attached = [];
      applied = wanted;
      appliedOffset = offset;
      if (wanted === null || offset === null) return;
      for (const subtitle of wanted.subtitles) {
        const element = player.addRemoteTextTrack(
          {
            kind: "subtitles",
            src: subtitleUrl(wanted.videoId, subtitle.file, offset),
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
    if (wanted === null || offset === null) return;
    if (fromPreference) {
      fromPreference = false;
      const saved = readSubtitlePreference(storage);
      show(saved.enabled ? saved.label : null);
      return;
    }
    show(selection);
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
    setOffset(next) {
      offsetMs = next === null ? null : Math.max(0, Math.round(next));
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
