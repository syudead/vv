import type { ActivityEntry, ActivityTag } from "../api/activity";
import { formatDateTime, formatMonthDay, formatTime, t, type UiText } from "../i18n";
import { videoEditPath } from "../lib/videoEditPath";

/**
 * アクティビティの記録 1 件の見せ方（specs/055-activity/ui-design.md「Cards」「Words」）。
 * カードの色・状態の印・対象・結果・時刻・開く操作を記録から決める。React に依存しない純粋な
 * 関数だけを置く。
 */

/** CardTone はカードの枠と面の色である（CardList の tone）。 */
export type CardTone = "default" | "primary" | "destructive" | "warning";

/** StatusMark は状態の印の種類である。 */
export type StatusMark = "running" | "waiting" | "attention" | "done" | "doneByYou";

/** cardTone は記録のカードの色を返す（ui-design.md「Card state」）。 */
export function cardTone(entry: ActivityEntry): CardTone {
  if (entry.status === "running")
    return entry.reason === "processing" ? "primary" : "default";
  if (entry.status !== "attention") return "default";
  return entry.reason === "no_connection" || entry.reason === "undecided"
    ? "warning"
    : "destructive";
}

/** statusMark は状態の印を返す。 */
export function statusMark(entry: ActivityEntry): StatusMark {
  switch (entry.status) {
    case "running":
      return entry.reason === "processing" ? "running" : "waiting";
    case "attention":
      return "attention";
    case "done":
      return entry.decider === "owner" ? "doneByYou" : "done";
  }
}

/** SubjectPart は対象の 1 項目で、動画があれば再生画面へのリンクになる。 */
export interface SubjectPart {
  label: string;
  videoId?: number;
}

/**
 * subjectParts は対象の項目を返す。組は 2 項目（間に「↔」）、走査は手動か監視かの名前、
 * フォルダはそのパス、動画はファイル名。
 */
export function subjectParts(entry: ActivityEntry): SubjectPart[] {
  const { kind, items } = entry.subject;
  if (kind === "scan") {
    const label =
      items[0]?.label === "watch"
        ? t.activity.subject.watchScan
        : t.activity.subject.manualScan;
    return [{ label }];
  }
  return items.map((item) =>
    item.videoId === undefined
      ? { label: item.label }
      : { label: item.label, videoId: item.videoId },
  );
}

/** subjectText は対象を 1 行の文字列にする（読み上げ名とツールチップ）。 */
export function subjectText(entry: ActivityEntry): string {
  const parts = subjectParts(entry);
  const [first, second] = parts;
  if (first === undefined) return "";
  return second === undefined
    ? first.label
    : t.activity.subject.pair(first.label, second.label);
}

/** ResultPart は結果の文の一片である。strong はモデルの割合で、foreground で描く。 */
export interface ResultPart {
  text: UiText;
  strong?: boolean;
}

/** ResultTone は結果の文の色である。 */
export type ResultTone = "muted" | "destructive" | "warning";

export interface EntryResult {
  parts: ResultPart[];
  tone: ResultTone;
}

function text(value: UiText): ResultPart {
  return { text: value };
}

function percent(probability: number | undefined): ResultPart[] {
  return probability === undefined
    ? []
    : [{ text: t.activity.percent(probability), strong: true }];
}

function retryTime(iso: string, now: Date): UiText {
  const at = new Date(iso);
  const sameDay = at.toDateString() === now.toDateString();
  const time = formatTime(at);
  return sameDay ? time : t.activity.time.retryAtDay(formatMonthDay(at, now), time);
}

function attentionResult(entry: ActivityEntry, now: Date): ResultPart[] {
  const detail = entry.detail ?? "";
  switch (entry.reason) {
    case "interrupted":
      return [text(t.activity.result.interrupted)];
    case "no_connection":
      return [text(t.activity.result.noConnection)];
    case "undecided":
      return entry.probability === undefined
        ? [text(t.activity.result.undecided)]
        : [
            text(t.activity.result.undecided),
            text(t.activity.separator),
            text(t.activity.result.undecidedAi),
            ...percent(entry.probability),
          ];
    default:
      if (entry.kind === "import" && entry.stage !== undefined)
        return [
          text(t.activity.result.importFailed(t.activity.stage[entry.stage], detail)),
        ];
      if (entry.kind === "version_judgement" && entry.retryAt !== undefined)
        return [text(t.activity.result.retryAt(retryTime(entry.retryAt, now)))];
      return [text(t.activity.result.failed(detail))];
  }
}

function runningResult(entry: ActivityEntry): ResultPart[] {
  if (entry.reason !== "processing") return [text(t.activity.result.waiting)];
  if (entry.progress !== undefined) return [text(t.activity.stage[entry.progress.stage])];
  return [text(t.activity.result.running)];
}

function autoTagResult(entry: ActivityEntry): ResultPart[] {
  const tags = entry.tags ?? [];
  if (tags.length === 0) return [text(t.activity.result.noTags)];
  return tags.flatMap((tag, index) => [
    ...(index === 0 ? [] : [text(t.activity.separator)]),
    text(t.activity.result.tagAdded(tag.name)),
    ...percent(tag.probability),
    ...(tag.suggested ? [text(t.activity.result.suggested)] : []),
  ]);
}

function undoResult(entry: ActivityEntry): ResultPart[] {
  const undoes = entry.undoes;
  if (entry.outcome === "already_reverted")
    return [text(t.activity.result.alreadyReverted)];
  if (undoes === undefined) return [text(t.activity.result.byYouAlone)];
  if (undoes.tagName !== undefined)
    return [text(t.activity.result.undidTag(undoes.tagName))];
  const undid = text(t.activity.result.undid(t.activity.kind[undoes.kind]));
  if (undoes.outcome === "same")
    return [undid, text(t.activity.separator), text(t.activity.result.undoneSame)];
  if (undoes.outcome === "different")
    return [undid, text(t.activity.separator), text(t.activity.result.undoneDifferent)];
  return [undid];
}

function ownerTagResult(entry: ActivityEntry, added: boolean): ResultPart[] {
  const name = entry.detail ?? "";
  return [
    text(added ? t.activity.result.tagAdded(name) : t.activity.result.tagRemoved(name)),
    text(t.activity.separator),
    text(t.activity.result.byYou),
  ];
}

function completedResult(entry: ActivityEntry): ResultPart[] {
  switch (entry.kind) {
    case "auto_tag":
      return autoTagResult(entry);
    case "undo":
      return undoResult(entry);
    case "tag_attach":
    case "tag_keep":
      return ownerTagResult(entry, true);
    case "tag_detach":
    case "tag_reject":
      return ownerTagResult(entry, false);
    case "media_folder_remove":
      return [text(t.activity.result.folderRemoved(Number(entry.detail ?? "0") || 0))];
    default:
      return entry.decider === "owner"
        ? [text(t.activity.result.byYouAlone)]
        : [text(t.activity.result.done)];
  }
}

function doneResult(entry: ActivityEntry): ResultPart[] {
  const ai = entry.decider === "ai";
  switch (entry.outcome) {
    case "same":
      return ai
        ? [text(t.activity.result.sameByAi), ...percent(entry.probability)]
        : [text(t.activity.result.sameByRule)];
    case "different":
      return ai
        ? [text(t.activity.result.differentByAi), ...percent(entry.probability)]
        : [text(t.activity.result.differentByRule)];
    case "failed":
      return [text(t.activity.result.failed(entry.detail ?? ""))];
    case "superseded":
      return [text(t.activity.result.superseded)];
    case "decided_by_owner":
      return [text(t.activity.result.decidedByOwner)];
    case "gone":
      return [text(t.activity.result.gone)];
    default:
      return completedResult(entry);
  }
}

/** missingVideo は動画の記録で、その動画がもうライブラリに無いか。 */
function missingVideo(entry: ActivityEntry): boolean {
  return (
    entry.subject.kind === "video" &&
    entry.subject.items.every((item) => item.videoId === undefined) &&
    entry.outcome !== "gone"
  );
}

/**
 * entryResult は結果の文と色を返す。所有者が取り消した記録には「· Undone by you」を足し、
 * 対象の動画がもう無い記録には「· Not in the library」を足す。
 */
export function entryResult(entry: ActivityEntry, now: Date): EntryResult {
  if (entry.status === "running") return { parts: runningResult(entry), tone: "muted" };
  if (entry.status === "attention") {
    const tone = cardTone(entry) === "warning" ? "warning" : "destructive";
    return { parts: attentionResult(entry, now), tone };
  }
  const parts = doneResult(entry);
  if (entry.undone)
    parts.push(text(t.activity.separator), text(t.activity.result.undone));
  if (missingVideo(entry))
    parts.push(text(t.activity.separator), text(t.activity.result.notInLibrary));
  return { parts, tone: "muted" };
}

/** OpenAction は記録から開ける画面である。 */
export interface OpenAction {
  id: "settings" | "workInfo" | "duplicates" | "open";
  label: UiText;
  to: string;
}

function firstVideo(entry: ActivityEntry): number | undefined {
  return entry.subject.items.find((item) => item.videoId !== undefined)?.videoId;
}

/**
 * openActions は記録のカードの端の開く操作を返す（ui-design.md「Cards」の Action の表）。
 * 対象に動画が無い記録の Open と Work info は出さない。
 */
export function openActions(entry: ActivityEntry): OpenAction[] {
  const actions: OpenAction[] = [];
  const videoId = firstVideo(entry);
  if (entry.reason === "no_connection")
    actions.push({ id: "settings", label: t.activity.action.settings, to: "/settings" });
  if (entry.kind === "work_info_fetch" && videoId !== undefined)
    actions.push({
      id: "workInfo",
      label: t.activity.action.workInfo,
      to: videoEditPath(videoId),
    });
  if (entry.subject.kind === "pair" && entry.candidate)
    actions.push({
      id: "duplicates",
      label: t.activity.action.openDuplicates,
      to: "/duplicates",
    });
  else if (videoId !== undefined)
    actions.push({
      id: "open",
      label: t.activity.action.open,
      to: `/videos/${String(videoId)}`,
    });
  return actions;
}

/** EntryOperations は記録のカードが出すやり直しと取り消しの操作である。 */
export interface EntryOperations {
  /** `Retry` を出すか。 */
  retry: boolean;
  /** `Undo` を出すか（`auto_tag` 以外）。 */
  undo: boolean;
  /** `Undo tags…` のメニューに並べるタグ。空ならボタンを出さない。 */
  undoTags: ActivityTag[];
}

/**
 * entryOperations は記録のカードのやり直しと取り消しの操作を返す（ui-design.md「Cards」の
 * Action の表）。`auto_tag` はまだ取り消していないタグを 1 つずつ取り消すメニューにする。
 */
export function entryOperations(entry: ActivityEntry): EntryOperations {
  const autoTag = entry.kind === "auto_tag";
  return {
    retry: entry.retryRun,
    undo: entry.undoable && !autoTag,
    undoTags:
      autoTag && entry.undoable ? (entry.tags ?? []).filter((tag) => !tag.undone) : [],
  };
}

/** EntryTime は記録の時刻の長い形・短い形・ツールチップの日時である。 */
export interface EntryTime {
  long: UiText;
  short: UiText;
  full: UiText;
}

const minuteMs = 60 * 1000;
const hourMs = 60 * minuteMs;

function startOfDay(date: Date, daysBack = 0): number {
  // 日付を引いてから 0 時を作る。夏時間の切り替わる日は 23 時間・25 時間あるので、時間では引かない。
  return new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate() - daysBack,
  ).getTime();
}

/**
 * entryTime は記録を作った時刻を「Just now」「12 minutes ago」「3 hours ago」「Yesterday
 * 2:05 PM」「Oct 7 2:05 PM」で表す。1 時間までは分、今日のうちは時間、昨日は Yesterday、
 * それより前は日付。短い形（360px）は「now」「12m」「3h」「Yesterday」「Oct 7」。
 */
export function entryTime(iso: string, now: Date): EntryTime {
  const at = new Date(iso);
  const full = formatDateTime(at);
  const elapsed = now.getTime() - at.getTime();
  if (elapsed < minuteMs)
    return { long: t.activity.time.justNow, short: t.activity.time.shortNow, full };
  if (elapsed < hourMs) {
    const minutes = Math.floor(elapsed / minuteMs);
    return {
      long: t.activity.time.minutes(minutes),
      short: t.activity.time.shortMinutes(minutes),
      full,
    };
  }
  const today = startOfDay(now);
  if (at.getTime() >= today) {
    const hours = Math.floor(elapsed / hourMs);
    return {
      long: t.activity.time.hours(hours),
      short: t.activity.time.shortHours(hours),
      full,
    };
  }
  const time = formatTime(at);
  if (at.getTime() >= startOfDay(now, 1))
    return {
      long: t.activity.time.yesterday(time),
      short: t.activity.time.shortYesterday,
      full,
    };
  const date = formatMonthDay(at, now);
  return { long: t.activity.time.at(date, time), short: date, full };
}
