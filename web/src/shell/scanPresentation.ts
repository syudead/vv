import type { Scan, ScanActivity } from "../api/client";
import { formatDateTime, t, type UiText } from "../i18n";
import type { ScanContextValue } from "./ScanProvider";

export type ScanPresentationState =
  | "not-run"
  | "starting"
  | "finding"
  | "running"
  | "done"
  | "partial"
  | "failed"
  | "fetch-failed";

export interface ScanDetailLine {
  text: UiText;
  /** Full activity path for the truncated line's title. */
  title?: UiText;
}

export interface ScanPresentation {
  state: ScanPresentationState;
  scan: Scan | null;
  statusText: UiText | null;
  /** Null for states without numeric progress or zero affected videos. */
  videos: { settled: number; total: number } | null;
  progressText: UiText | null;
  bar: "determinate" | "indeterminate" | "none";
  detail: ScanDetailLine | null;
  issues: { failed: number; substituted: number };
  error: UiText | null;
  refreshing: boolean;
  /** Automatic scans remain invisible until completion; starting denotes a manual scan. */
  watch: boolean;
}

export function inProgressState(state: ScanPresentationState): boolean {
  return state === "starting" || state === "finding" || state === "running";
}

function activityLine(activity: ScanActivity): ScanDetailLine {
  const text = t.shell.scan;
  const parts: string[] = [];
  if (activity.folder?.rootName !== undefined) parts.push(activity.folder.rootName);
  if (activity.folder !== undefined && activity.folder.path !== "")
    parts.push(activity.folder.path);
  parts.push(activity.fileName);
  return {
    text: text.activityLine(text.activity[activity.kind], activity.fileName),
    title: text.location(parts),
  };
}

function detailFor(
  state: ScanPresentationState,
  scan: Scan,
  activity: ScanActivity | null,
): ScanDetailLine | null {
  const text = t.shell.scan;
  switch (state) {
    case "finding":
    case "running":
      if (activity !== null) return activityLine(activity);
      return state === "finding" ? { text: text.lookingForFiles } : null;
    case "done":
    case "partial":
      return scan.settledAt === undefined
        ? null
        : { text: text.finishedAt(formatDateTime(scan.settledAt)) };
    case "failed":
      return { text: text.couldNotFinish };
    default:
      return null;
  }
}

const noIssues = { failed: 0, substituted: 0 };

/** Automatic scans never show progress; only terminal status, time and issue counts are visible. */
function presentWatchScan(
  scan: Scan,
  state: ScanPresentationState,
  loadError: UiText | null,
): ScanPresentation {
  const text = t.shell.scan;
  const ended = state === "done" || state === "partial" || state === "failed";
  return {
    state,
    scan,
    statusText: ended ? text.watchStatus[state] : null,
    videos: null,
    progressText: null,
    bar: "none",
    detail: ended ? detailFor(state, scan, null) : null,
    issues: ended
      ? { failed: scan.issues.failed, substituted: scan.issues.substituted }
      : noIssues,
    error: loadError,
    refreshing: loadError !== null,
    watch: true,
  };
}

/** Converts the scan context into the shared state model used by shell views. */
export function presentScan(value: ScanContextValue): ScanPresentation {
  const { scan } = value;
  const text = t.shell.scan;
  // Start failures belong beside the settings button; fetch-failed/refreshing reflect only status-fetch failures.
  const loadError = value.startError === null ? value.error : null;
  if (value.starting) {
    return {
      state: "starting",
      scan,
      statusText: text.status.starting,
      videos: null,
      progressText: null,
      bar: "indeterminate",
      detail: { text: text.lookingForFiles },
      issues: noIssues,
      error: value.error,
      refreshing: false,
      watch: false,
    };
  }

  if (scan === null) {
    return {
      state: loadError === null ? "not-run" : "fetch-failed",
      scan: null,
      statusText: null,
      videos: null,
      progressText: null,
      bar: "none",
      detail: null,
      issues: noIssues,
      error: value.error,
      refreshing: loadError !== null,
      watch: false,
    };
  }

  const state: ScanPresentationState = scan.status;
  if (scan.origin === "watch") return presentWatchScan(scan, state, loadError);
  // Do not show counts or percentages during finding.
  const counted = state !== "finding" && scan.videos !== undefined;
  const total = counted ? (scan.videos?.total ?? 0) : 0;
  const videos =
    counted && total > 0
      ? { settled: Math.min(scan.videos?.settled ?? 0, total), total }
      : null;
  const noChanges = counted && total === 0 && (state === "done" || state === "partial");
  const progressText =
    videos !== null
      ? text.videosDone(videos.settled, videos.total)
      : noChanges
        ? text.noChanges
        : null;
  const bar =
    videos !== null
      ? "determinate"
      : state === "finding" || state === "running"
        ? "indeterminate"
        : "none";

  return {
    state,
    scan,
    statusText: text.status[state],
    videos,
    progressText,
    bar,
    detail: detailFor(state, scan, value.activity),
    issues: { failed: scan.issues.failed, substituted: scan.issues.substituted },
    error: value.error,
    refreshing: loadError !== null,
    watch: false,
  };
}

export function issueCountTexts(presentation: ScanPresentation): UiText[] {
  const text = t.shell.scan;
  const out: UiText[] = [];
  if (presentation.issues.failed > 0)
    out.push(text.failedCount(presentation.issues.failed));
  if (presentation.issues.substituted > 0)
    out.push(text.toCheckCount(presentation.issues.substituted));
  return out;
}

/** Announce terminal milestones only, never individual activity or progress updates. */
export function statusAnnouncement(presentation: ScanPresentation): UiText | null {
  if (presentation.watch) {
    // Automatic scans notify only terminal failures, never successful completion.
    const watch = t.shell.scan.watchAnnounce;
    if (presentation.state === "partial")
      return watch.partial(presentation.issues.failed);
    return presentation.state === "failed" ? watch.failed : null;
  }
  const announce = t.shell.scan.announce;
  switch (presentation.state) {
    case "done":
      return announce.done(presentation.issues.substituted);
    case "partial":
      return announce.partial(presentation.issues.failed);
    case "failed":
      return announce.failed;
    default:
      return null;
  }
}
