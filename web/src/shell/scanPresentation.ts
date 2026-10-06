import type { Scan, ScanActivity } from "../api/client";
import { formatDateTime, t, type UiText } from "../i18n";
import type { ScanContextValue } from "./ScanProvider";

/**
 * ScanPresentationState は画面の状態である。取り込みの状態（`Scan.status`）に、
 * 未実行・開始中・取得の一時失敗を足したもの（specs/024-import-progress/ui-design.md「States」）。
 */
export type ScanPresentationState =
  | "not-run"
  | "starting"
  | "finding"
  | "running"
  | "done"
  | "partial"
  | "failed"
  | "fetch-failed";

/** ScanDetailLine は3行目（今の処理、または完了の時刻）である。 */
export interface ScanDetailLine {
  text: UiText;
  /** 省略した全体（登録フォルダの表示名 / 相対パス / ファイル名）。今の処理のときだけ。 */
  title?: UiText;
}

/**
 * ScanPresentation は、右下の本体・概要・設定の「Scan status」が共有する表示モデルである。
 * `Scan` の status・videos・issues・settledAt・activity だけから作る。
 */
export interface ScanPresentation {
  state: ScanPresentationState;
  scan: Scan | null;
  /** 状態の言葉。未実行と取得の一時失敗では null。 */
  statusText: UiText | null;
  /** 数字で示す進み具合。数字を出さない状態（finding など）と、対象が 0 本のときは null。 */
  videos: { settled: number; total: number } | null;
  /** 進み具合の文。「M of N videos done」か「No changed files were found.」。 */
  progressText: UiText | null;
  /** バーの形。 */
  bar: "determinate" | "indeterminate" | "none";
  /** 今の処理、または完了の時刻の行。 */
  detail: ScanDetailLine | null;
  issues: { failed: number; substituted: number };
  error: UiText | null;
  refreshing: boolean;
}

/** inProgressState は、取り込みがまだ終わっていない画面の状態かを返す。 */
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

/** Converts the scan context into the shared state model used by shell views. */
export function presentScan(value: ScanContextValue): ScanPresentation {
  const { scan } = value;
  const text = t.shell.scan;
  // 開始の失敗は設定の「Scan status」が button のそばに出す。状態の取り直し（fetch-failed・
  // refreshing）は、状態取得の失敗だけで決める。
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
    };
  }

  const state: ScanPresentationState = scan.status;
  // finding のあいだは数字を出さない（割合も出さない）。
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
  };
}

/** issueCountTexts は問題の本数の言葉である。0 の方は出さない。 */
export function issueCountTexts(presentation: ScanPresentation): UiText[] {
  const text = t.shell.scan;
  const out: UiText[] = [];
  if (presentation.issues.failed > 0)
    out.push(text.failedCount(presentation.issues.failed));
  if (presentation.issues.substituted > 0)
    out.push(text.toCheckCount(presentation.issues.substituted));
  return out;
}

/**
 * statusAnnouncement は `role="status"` で読み上げる文である。完了・一部失敗・失敗の
 * 節目だけにし、今の処理や進み具合の変化では何も読み上げない。
 */
export function statusAnnouncement(presentation: ScanPresentation): UiText | null {
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
