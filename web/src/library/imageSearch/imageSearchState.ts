import { RequestFailed } from "../../api/client";
import type { ImageSearchMatch, ImageSearchResult } from "../../api/imageSearch";
import { t, type UiText } from "../../i18n";
import { errorText } from "../../i18n/errors";
import { formatBytes, formatDuration } from "../../lib/format";

/** Why a file was not searched, as the body's alert shows it. */
export interface Refusal {
  title: UiText;
  description?: UiText;
  /** Retrying the same file can help: a network or server failure. */
  retry: boolean;
}

/** One image search as the page shows it; the file stays until the chip is cleared, for the chip and Retry. */
export type ImageSearchPhase =
  | { kind: "searching"; file: File }
  | { kind: "results"; file: File; result: ImageSearchResult }
  | { kind: "refused"; file: File; refusal: Refusal };

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
      retry: false,
    };
  }
  const file = files[0];
  if (file !== undefined && !searchableType(file)) {
    return { ...text.unreadable, retry: false };
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

/** The refusal a failed request shows: the server's reason, else the failure with Retry. */
export function refuseError(error: unknown): Refusal {
  const text = t.library.imageSearch.refused;
  const reason = error instanceof RequestFailed ? error.reason : undefined;
  switch (reason) {
    case "image_search_unreadable":
      return { ...text.unreadable, retry: false };
    case "image_search_uninformative":
      return { ...text.uninformative, retry: false };
    case "image_search_too_large":
    case "image_search_too_long": {
      const limit = (error as RequestFailed).limit;
      return {
        title: reason === "image_search_too_large" ? text.tooLarge : text.tooLong,
        description: limitText(reason, limit),
        retry: false,
      };
    }
    default:
      return {
        title: text.failed,
        description: errorText(error),
        retry: !(error instanceof RequestFailed) || error.status >= 500,
      };
  }
}

/** The match rate as a whole percent, rounded to the nearest. */
export function ratePercent(rate: number): number {
  return Math.round(rate * 100);
}

/** The player at a position; `t` is in seconds. */
export function playPath(videoId: number, positionMs: number): string {
  return `/videos/${String(videoId)}?t=${String(positionMs / 1000)}`;
}

/** Where a match plays from when its card is opened. */
export function matchPath(match: ImageSearchMatch): string {
  return playPath(match.videoId, match.positionMs);
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

interface Entry {
  phase: ImageSearchPhase;
  scrollY?: number;
}

// Searches by history entry, so going back from the player shows the same results; the file never
// goes in the URL. The oldest entries are dropped so files do not pile up in memory.
const entries = new Map<string, Entry>();
const maxEntries = 10;

export function rememberPhase(key: string, phase: ImageSearchPhase): void {
  entries.delete(key);
  entries.set(key, { phase });
  while (entries.size > maxEntries) {
    const oldest = entries.keys().next().value;
    if (oldest === undefined) break;
    entries.delete(oldest);
  }
}

export function rememberScroll(key: string, scrollY: number): void {
  const entry = entries.get(key);
  if (entry !== undefined) entry.scrollY = scrollY;
}

export function recall(key: string): Entry | undefined {
  return entries.get(key);
}

export function forget(key: string): void {
  entries.delete(key);
}

export function __resetImageSearchesForTest(): void {
  entries.clear();
}
