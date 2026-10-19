import type {
  ActivityDetail,
  ActivityEntry,
  ActivityStage,
  ActivityStageState,
  ActivitySubjectItem,
} from "../api/activity";
import { t, type UiText } from "../i18n";
import {
  cardTone,
  type EntryFields,
  type EntryResult,
  entryResult,
} from "./activityEntry";
import { attentionMember, pipelineAttentionResult } from "./activityPipeline";

/** Ingest order of an import's stages, the order the server reports them in. */
export const STAGE_ORDER: readonly ActivityStage[] = [
  "probe",
  "thumbnail",
  "seek_thumbnail",
  "preview",
  "fingerprint",
];

/** `notRun` is a stage still waiting while its import has stopped; `absent` has no row. */
export type StageView = ActivityStageState | "notRun" | "absent";

export interface StageLine {
  stage: ActivityStage;
  view: StageView;
  updatedAt?: string;
  /** The stage's full error message, for a failed stage. */
  message?: string;
}

/** Null for an entry other than import; `notRecorded` for an import from before stages were kept. */
export function stageLines(entry: ActivityDetail): StageLine[] | "notRecorded" | null {
  if (entry.kind !== "import") return null;
  if (entry.stagesRecorded === false) return "notRecorded";
  const rows = entry.stages ?? [];
  return STAGE_ORDER.map((stage) => {
    const row = rows.find((item) => item.stage === stage);
    if (row === undefined) return { stage, view: "absent" };
    const stopped = row.state === "waiting" && entry.status !== "running";
    return {
      stage,
      view: stopped ? "notRun" : row.state,
      updatedAt: row.updatedAt,
      ...(row.state === "failed" && row.detail !== undefined
        ? { message: row.detail }
        : {}),
    };
  });
}

/** The list the detail was opened from, kept with its filters; the plain list otherwise. */
export function listTarget(state: unknown): string {
  const from: unknown = (state as { from?: unknown } | null)?.from;
  if (typeof from !== "string" || !/^\/activity(?:[?#]|$)/.test(from)) return "/activity";
  return from;
}

/** The video the header's Open goes to: a single-video subject still in the library. */
export function headerVideo(entry: ActivityEntry): number | undefined {
  if (entry.subject.kind !== "video") return undefined;
  return entry.subject.items[0]?.videoId;
}

export function deciderText(entry: ActivityEntry): UiText {
  if (entry.decider === "ai" && entry.probability !== undefined)
    return t.activity.detail.deciderAi(t.activity.percent(entry.probability));
  return t.activity.deciderValue[entry.decider];
}

/**
 * A video resolves to the same `videoId`, or to none, in every entry of one response, so an id on
 * only one side marks two videos; labels are snapshots and decide only when neither has an id.
 */
function sameItem(a: ActivitySubjectItem, b: ActivitySubjectItem): boolean {
  if (a.videoId !== undefined || b.videoId !== undefined) return a.videoId === b.videoId;
  return a.label === b.label;
}

/** A pair member names the video other than the pipeline's own. */
export function memberTitle(
  member: Pick<ActivityEntry, "kind" | "subject">,
  pipeline: Pick<ActivityEntry, "subject">,
): UiText {
  if (member.subject.kind !== "pair") return t.activity.kind[member.kind];
  const own = pipeline.subject.items;
  const other =
    member.subject.items.find((item) => !own.some((mine) => sameItem(mine, item))) ??
    member.subject.items[1];
  return t.activity.detail.pairMember(other?.label ?? "");
}

/**
 * A member row reads its result phrase without the message, which its own detail shows in full; a
 * replaced member reads as replaced whatever its own outcome.
 */
export function memberResult(member: EntryFields, now: Date): EntryResult {
  if (member.replaced === true)
    return { parts: [{ text: t.activity.result.superseded }], tone: "muted" };
  const { detail: _message, ...withoutMessage } = member;
  return entryResult(withoutMessage, now);
}

export interface DetailNotice {
  title: EntryResult;
  /** Shown in full under the title, line breaks kept. */
  message?: string;
  tone: "destructive" | "warning";
  retry: boolean;
}

function attentionSource(entry: ActivityDetail): ActivityEntry {
  if (entry.kind !== "pipeline") return entry;
  return attentionMember(entry.members) ?? entry;
}

/** The reason phrase goes in the title; the full message is shown once, under it. */
export function detailNotice(entry: ActivityDetail, now: Date): DetailNotice | null {
  if (entry.status !== "attention") return null;
  const source = attentionSource(entry);
  const { detail, ...withoutDetail } = source;
  const title: EntryResult =
    source === entry
      ? entryResult(withoutDetail, now)
      : pipelineAttentionResult(source, now);
  return {
    title,
    ...(detail === undefined || detail === "" ? {} : { message: detail }),
    tone: cardTone(source) === "warning" ? "warning" : "destructive",
    retry: source === entry && entry.retryRun,
  };
}
