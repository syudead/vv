import { RequestFailed } from "../../api/client";
import type {
  ImageSearchBand,
  ImageSearchItem,
  ImageSearchMatch,
  ImageSearchResult,
} from "../../api/imageSearch";
import { t, type UiText } from "../../i18n";
import { errorText } from "../../i18n/errors";
import { formatBytes, formatDuration } from "../../lib/format";

/** Why a file was not searched, or why the search failed, as the results column's alert shows it. */
export interface Refusal {
  title: UiText;
  description?: UiText;
}

/**
 * One image search as the screen shows it; the file stays for the panel, the chip, Retry and Search again.
 * `startedAt` is the `Date.now()` the request started at, `sent` the share of the upload sent.
 */
export type ImageSearchPhase =
  | { kind: "uploading"; file: File; startedAt: number; sent: number }
  | { kind: "searching"; file: File; startedAt: number }
  | { kind: "results"; file: File; result: ImageSearchResult; tookMs: number }
  | { kind: "refused"; file: File; refusal: Refusal }
  | { kind: "failed"; file: File; refusal: Refusal }
  | { kind: "stopped"; file: File };

export type RunningPhase = Extract<ImageSearchPhase, { kind: "uploading" | "searching" }>;

/** Whether the request runs: the phase ends with an answer, a failure or Cancel. */
export function isRunning(phase: ImageSearchPhase): phase is RunningPhase {
  return phase.kind === "uploading" || phase.kind === "searching";
}

/** An elapsed time as the panel reads it: `0.8 s` under 10 s, then `12 s`, then `1:05`. */
export function formatElapsed(ms: number): string {
  const text = t.library.imageSearch;
  if (ms < 10_000) return text.seconds(Math.floor(ms / 100) / 10, 1);
  if (ms < 60_000) return text.seconds(Math.floor(ms / 1000), 0);
  return formatDuration(ms);
}

// The server's pixel limit; its byte limits are powers of two, so the value tells the two apart.
const maxImagePixels = 120_000_000;

/**
 * A file the browser can tell is neither an image nor a video is refused before the upload. A file
 * without a type goes to the server, which reads its content.
 */
function searchableType(file: File): boolean {
  return file.type === "" || /^(image|video)\//.test(file.type);
}

/** The refusal of the given files without a request, or null when the first one can be sent. */
export function refuseFiles(files: readonly File[]): Refusal | null {
  const text = t.library.imageSearch.refused;
  if (files.length > 1) {
    return {
      title: text.several.title,
      description: text.several.description(files.length),
    };
  }
  const file = files[0];
  if (file !== undefined && !searchableType(file)) {
    return { ...text.unreadable };
  }
  return null;
}

function limitText(reason: string, limit: number | undefined): UiText | undefined {
  if (limit === undefined) return undefined;
  const text = t.library.imageSearch.refused;
  if (reason === "image_search_too_long") return text.limit(formatDuration(limit));
  return text.limit(
    limit === maxImagePixels ? text.megapixels(limit / 1_000_000) : formatBytes(limit),
  );
}

/** The refusal of the server's reason, or null for a failure that sending again can mend. */
function serverRefusal(error: unknown): Refusal | null {
  const text = t.library.imageSearch.refused;
  if (!(error instanceof RequestFailed)) return null;
  switch (error.reason) {
    case "image_search_unreadable":
      return { ...text.unreadable };
    case "image_search_uninformative":
      return { ...text.uninformative };
    case "image_search_too_large":
    case "image_search_too_long":
      return {
        title: error.reason === "image_search_too_large" ? text.tooLarge : text.tooLong,
        description: limitText(error.reason, error.limit),
      };
    default:
      return error.status >= 500
        ? null
        : { title: text.failed, description: errorText(error) };
  }
}

/** The phase a failed request ends in: refused for the server's reason, failed with Retry otherwise. */
export function errorPhase(file: File, error: unknown): ImageSearchPhase {
  const refusal = serverRefusal(error);
  if (refusal !== null) return { kind: "refused", file, refusal };
  return {
    kind: "failed",
    file,
    refusal: {
      title: t.library.imageSearch.refused.failed,
      description: errorText(error),
    },
  };
}

/** The match rate as a whole percent, rounded to the nearest. */
export function ratePercent(rate: number): number {
  return Math.round(rate * 100);
}

/** The player at a position; `t` is in seconds. */
export function playPath(videoId: number, positionMs: number): string {
  return `/videos/${String(videoId)}?t=${String(positionMs / 1000)}`;
}

/** Where a match plays from when its row is opened: a video query's section start, else the best frame. */
export function matchPath(match: ImageSearchMatch): string {
  return playPath(match.videoId, match.startMs ?? match.positionMs);
}

/** The bands that have rows, in the server's order, each with its items in the server's order. */
export function bandsWithItems(
  result: ImageSearchResult,
): { band: ImageSearchBand; items: ImageSearchItem[] }[] {
  return result.bands
    .map((band) => ({
      band,
      items: result.items.filter((item) => item.match.band === band.id),
    }))
    .filter((group) => group.items.length > 0);
}

/**
 * Whether the first row is named the best candidate: its shown rate is above the second row's, or it is
 * the only row, so the screen never names one of two equal rows the answer.
 */
export function hasBestCandidate(items: readonly ImageSearchItem[]): boolean {
  const [first, second] = items;
  if (first === undefined) return false;
  if (second === undefined) return true;
  return ratePercent(first.match.rate) > ratePercent(second.match.rate);
}

/** A video match's section on a lane, in seconds. */
export interface MatchedSection {
  lengthSeconds: number;
  span: { start: number; end: number };
}

/**
 * The matched section on the listed video's length. The match may be in a longer version than the
 * listed one, whose length the result does not carry, so the lane reaches at least to the section's end.
 */
export function matchedSection(
  match: ImageSearchMatch,
  durationMs: number | undefined,
): MatchedSection | null {
  if (
    match.startMs === undefined ||
    match.endMs === undefined ||
    durationMs === undefined
  ) {
    return null;
  }
  return {
    lengthSeconds: Math.max(durationMs, match.endMs) / 1000,
    span: { start: match.startMs / 1000, end: match.endMs / 1000 },
  };
}

/** Where the owner gave the files; the screen they open puts focus back there. */
export type GivenFrom = "field" | "dialog";

/** What the screen keeps of one search, under the token its history entry names. */
export interface SearchEntry {
  /** Every file given, so the first search or refusal is decided by the screen that shows it. */
  files: readonly File[];
  from: GivenFrom;
  /** The last answer, refusal, failure or stop; absent while the search runs. */
  phase?: ImageSearchPhase;
  scrollY?: number;
  /** The keys of the result rows the owner collapsed. */
  collapsed?: ReadonlySet<string>;
  /** The owner's own toggle of the panel below lg; absent follows the phase. */
  panelOpen?: boolean;
  /** The frame last focused in each row's strip, by row key. */
  strips?: ReadonlyMap<string, number>;
  /** The control that opened the player, focused again on the way back. */
  opener?: string;
}

// Searches by the token their history entry names, so going back from the player shows the same results;
// the files never go in the URL or in history state. The oldest are dropped so files do not pile up.
const entries = new Map<string, SearchEntry>();
const maxEntries = 10;
let lastToken = 0;

/** Keeps the files for the screen the caller opens next, under a new token for that entry's state. */
export function handOver(files: readonly File[], from: GivenFrom): string {
  lastToken += 1;
  const token = `${Date.now().toString(36)}-${String(lastToken)}`;
  entries.set(token, { files, from });
  while (entries.size > maxEntries) {
    const oldest = entries.keys().next().value;
    if (oldest === undefined) break;
    entries.delete(oldest);
  }
  return token;
}

/** Keeps an answer or a refusal; what the owner did with the previous one is dropped. */
export function rememberPhase(key: string, phase: ImageSearchPhase): void {
  const entry = entries.get(key);
  if (entry === undefined) return;
  entries.set(key, {
    files: entry.files,
    from: entry.from,
    phase: isRunning(phase) ? undefined : phase,
  });
}

/** Merges what the owner changed on the shown answer, such as the scroll position or a collapsed row. */
export function rememberView(
  key: string,
  change: Pick<SearchEntry, "scrollY" | "collapsed" | "panelOpen" | "opener">,
): void {
  const entry = entries.get(key);
  if (entry !== undefined) entries.set(key, { ...entry, ...change });
}

export function rememberStrip(key: string, row: string, frame: number): void {
  const entry = entries.get(key);
  if (entry === undefined) return;
  const strips = new Map(entry.strips);
  strips.set(row, frame);
  entries.set(key, { ...entry, strips });
}

export function recall(key: string): SearchEntry | undefined {
  return entries.get(key);
}

export function __resetImageSearchesForTest(): void {
  entries.clear();
}
