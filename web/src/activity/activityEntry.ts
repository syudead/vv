import type { ActivityEntry, ActivityFaceLink, ActivityTag } from "../api/activity";
import { creditDisplayName } from "../credits/displayName";
import { formatDateTime, formatMonthDay, formatTime, t, type UiText } from "../i18n";
import { operationSentence } from "../assistant/proposal";
import { videoEditPath } from "../lib/videoEditPath";

/**
 * What the card helpers read: a list entry, or a pipeline member's summary, which carries
 * `tagCount` instead of `tags` and no `detail`.
 */
export type EntryFields = Omit<ActivityEntry, "candidate" | "createdAt" | "updatedAt"> & {
  tagCount?: number;
};

export type CardTone = "default" | "primary" | "destructive" | "warning";

export type StatusMark = "running" | "waiting" | "attention" | "done" | "doneByYou";

export function cardTone(entry: EntryFields): CardTone {
  if (entry.status === "running")
    return entry.reason === "processing" ? "primary" : "default";
  if (entry.status !== "attention") return "default";
  return entry.reason === "no_connection" || entry.reason === "undecided"
    ? "warning"
    : "destructive";
}

export function statusMark(entry: EntryFields): StatusMark {
  switch (entry.status) {
    case "running":
      return entry.reason === "processing" ? "running" : "waiting";
    case "attention":
      return "attention";
    case "done":
      return entry.decider === "owner" ? "doneByYou" : "done";
  }
}

/** Subjects with a video link to playback, or the person of a person subject. */
export interface SubjectPart {
  label: string;
  videoId?: number;
  personId?: number;
}

export function subjectParts(entry: Pick<ActivityEntry, "subject">): SubjectPart[] {
  const { kind, items } = entry.subject;
  if (kind === "scan") {
    const label =
      items[0]?.label === "watch"
        ? t.activity.subject.watchScan
        : t.activity.subject.manualScan;
    return [{ label }];
  }
  if (kind === "person")
    // A provisional person's label is empty.
    return items.map((item) => ({
      label:
        item.label !== ""
          ? item.label
          : item.personId === undefined
            ? t.credits.unnamedPerson
            : t.credits.personNumber(item.personId),
      ...(item.personId === undefined ? {} : { personId: item.personId }),
    }));
  return items.map((item) =>
    item.videoId === undefined
      ? { label: item.label }
      : { label: item.label, videoId: item.videoId },
  );
}

export function subjectText(entry: Pick<ActivityEntry, "subject">): string {
  const parts = subjectParts(entry);
  const [first, second] = parts;
  if (first === undefined) return "";
  return second === undefined
    ? first.label
    : t.activity.subject.pair(first.label, second.label);
}

/** Render model percentages with foreground emphasis. */
export interface ResultPart {
  text: UiText;
  strong?: boolean;
}

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

function attentionResult(entry: EntryFields, now: Date): ResultPart[] {
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

function runningResult(entry: EntryFields): ResultPart[] {
  if (entry.reason !== "processing") return [text(t.activity.result.waiting)];
  if (entry.progress !== undefined) return [text(t.activity.stage[entry.progress.stage])];
  return [text(t.activity.result.running)];
}

function autoTagResult(entry: EntryFields): ResultPart[] {
  if (entry.tags === undefined && entry.tagCount !== undefined)
    return [
      text(
        entry.tagCount === 0
          ? t.activity.result.noTags
          : t.activity.result.tagsAdded(entry.tagCount),
      ),
    ];
  const tags = entry.tags ?? [];
  if (tags.length === 0) return [text(t.activity.result.noTags)];
  return tags.flatMap((tag, index) => [
    ...(index === 0 ? [] : [text(t.activity.separator)]),
    text(t.activity.result.tagAdded(tag.name)),
    ...percent(tag.probability),
    ...(tag.suggested ? [text(t.activity.result.suggested)] : []),
  ]);
}

/** The name a face link shows; a deleted person has none left. */
export function faceLinkName(link: ActivityFaceLink): string {
  return link.person === undefined
    ? t.activity.result.personGone
    : creditDisplayName(link.person);
}

function linkedNames(links: ActivityFaceLink[]): string {
  return links
    .map((link) => {
      const name = faceLinkName(link);
      return link.createdPerson ? t.activity.result.createdPerson(name) : name;
    })
    .join(", ");
}

/** Names of the links still in place, then how many were undone. */
function faceAnalysisResult(entry: EntryFields): ResultPart[] {
  const links = entry.faceLinks ?? [];
  if (links.length === 0) return [text(t.activity.result.noMainPerson)];
  const kept = links.filter((link) => !link.undone);
  if (kept.length === 0) return [text(t.activity.result.allLinksUndone)];
  const parts = [text(t.activity.result.linked(linkedNames(kept)))];
  const undone = links.length - kept.length;
  if (undone > 0)
    parts.push(text(t.activity.separator), text(t.activity.result.linksUndone(undone)));
  return parts;
}

function faceMergeResult(entry: EntryFields): ResultPart[] {
  const [target] = subjectParts(entry);
  return [
    text(t.activity.result.mergedInto(t.credits.unnamedPerson, target?.label ?? "")),
  ];
}

function undoResult(entry: EntryFields): ResultPart[] {
  const undoes = entry.undoes;
  if (entry.outcome === "already_reverted")
    return [text(t.activity.result.alreadyReverted)];
  if (undoes === undefined) return [text(t.activity.result.byYouAlone)];
  if (undoes.tagName !== undefined)
    return [text(t.activity.result.undidTag(undoes.tagName))];
  // Undoing one named link keeps the name in detail; an unnamed person's link and all links keep none.
  if (
    undoes.kind === "face_analysis" &&
    entry.detail !== undefined &&
    entry.detail !== ""
  )
    return [text(t.activity.result.undidLink(entry.detail))];
  const undid = text(t.activity.result.undid(t.activity.kind[undoes.kind]));
  if (undoes.outcome === "same")
    return [undid, text(t.activity.separator), text(t.activity.result.undoneSame)];
  if (undoes.outcome === "different")
    return [undid, text(t.activity.separator), text(t.activity.result.undoneDifferent)];
  return [undid];
}

function ownerTagResult(entry: EntryFields, added: boolean): ResultPart[] {
  const name = entry.detail ?? "";
  return [
    text(added ? t.activity.result.tagAdded(name) : t.activity.result.tagRemoved(name)),
    text(t.activity.separator),
    text(t.activity.result.byYou),
  ];
}

function completedResult(entry: EntryFields): ResultPart[] {
  switch (entry.kind) {
    case "auto_tag":
      return autoTagResult(entry);
    case "face_analysis":
      return faceAnalysisResult(entry);
    case "face_merge":
      return faceMergeResult(entry);
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

/** The operations a run's detail names: each step's, or the one operation of an older entry. */
function runOperations(detail: { operation?: unknown; steps?: unknown }): string[] {
  if (Array.isArray(detail.steps)) {
    const steps = detail.steps as { operation?: unknown }[];
    return steps.map((step) => (typeof step.operation === "string" ? step.operation : ""));
  }
  return [typeof detail.operation === "string" ? detail.operation : ""];
}

/**
 * A run's detail is JSON with the summed counts and each step's operation and counts, as the
 * panel's card showed them.
 */
function assistantRunResult(entry: EntryFields): ResultPart[] {
  let detail: { operation?: unknown; steps?: unknown; applied?: unknown; failed?: unknown } =
    {};
  try {
    detail = (JSON.parse(entry.detail ?? "") ?? {}) as typeof detail;
  } catch {
    // A detail that is not JSON reads as an unknown operation with no counts.
  }
  const count = (value: unknown) => (typeof value === "number" ? value : 0);
  const parts = runOperations(detail).flatMap((operation) => [
    text(operationSentence(operation)),
    text(t.activity.separator),
  ]);
  if (entry.outcome === "stale") return [...parts, text(t.assistant.card.stale)];
  return [
    ...parts,
    text(t.assistant.runCounts(count(detail.applied), count(detail.failed))),
  ];
}

function doneResult(entry: EntryFields): ResultPart[] {
  if (entry.kind === "assistant_run") return assistantRunResult(entry);
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

function missingVideo(entry: EntryFields): boolean {
  return (
    entry.subject.kind === "video" &&
    entry.subject.items.every((item) => item.videoId === undefined) &&
    entry.outcome !== "gone"
  );
}

export function entryResult(entry: EntryFields, now: Date): EntryResult {
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

export interface OpenAction {
  id: "settings" | "workInfo" | "duplicates" | "open" | "person";
  label: UiText;
  to: string;
}

function firstVideo(entry: ActivityEntry): number | undefined {
  return entry.subject.items.find((item) => item.videoId !== undefined)?.videoId;
}

/** The person of a person subject, for `Open person`. */
export function subjectPerson(entry: Pick<ActivityEntry, "subject">): number | undefined {
  if (entry.subject.kind !== "person") return undefined;
  return entry.subject.items[0]?.personId;
}

/** Hide Open and Work info when the subject has no video. */
export function openActions(entry: ActivityEntry): OpenAction[] {
  const actions: OpenAction[] = [];
  const personId = subjectPerson(entry);
  if (personId !== undefined)
    actions.push({
      id: "person",
      label: t.activity.action.openPerson,
      to: `/people/${String(personId)}`,
    });
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

export interface EntryOperations {
  retry: boolean;
  /** Undo excludes auto_tag and face_analysis, which undo one part at a time. */
  undo: boolean;
  /** Omit the button when no undoable tags remain. */
  undoTags: ActivityTag[];
  /** The face links not yet undone; one shows `Undo`, several the `Undo links…` menu. */
  undoLinks: ActivityFaceLink[];
}

/** Auto-tag and face-analysis entries undo their remaining tags or links individually. */
export function entryOperations(entry: EntryFields): EntryOperations {
  const autoTag = entry.kind === "auto_tag";
  const faceAnalysis = entry.kind === "face_analysis";
  return {
    retry: entry.retryRun,
    undo: entry.undoable && !autoTag && !faceAnalysis,
    undoTags:
      autoTag && entry.undoable ? (entry.tags ?? []).filter((tag) => !tag.undone) : [],
    undoLinks:
      faceAnalysis && entry.undoable
        ? (entry.faceLinks ?? []).filter((link) => !link.undone)
        : [],
  };
}

export interface EntryTime {
  long: UiText;
  short: UiText;
  full: UiText;
}

const minuteMs = 60 * 1000;
const hourMs = 60 * minuteMs;

function startOfDay(date: Date, daysBack = 0): number {
  // Subtract calendar days before finding midnight: DST days may have 23 or 25 hours.
  return new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate() - daysBack,
  ).getTime();
}

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
