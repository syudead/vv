import type {
  ActivityListItem,
  ActivityMemberSummary,
  ActivityProgress,
} from "../api/activity";
import { t } from "../i18n";
import { type EntryFields, type EntryResult, entryResult } from "./activityEntry";

type MemberState = Pick<EntryFields, "status" | "reason" | "replaced">;

/** The members a card shows: a replaced member is kept for the detail page only. */
export function currentMembers<T extends MemberState>(
  members: readonly T[] | undefined,
): T[] {
  return (members ?? []).filter((member) => member.replaced !== true);
}

/** The member that gives a pipeline its attention reason. */
export function attentionMember<T extends MemberState>(
  members: readonly T[] | undefined,
): T | undefined {
  return currentMembers(members).find((member) => member.status === "attention");
}

/** `{kind}: {reason}`, the member's reason phrase without its message. */
export function pipelineAttentionResult(member: EntryFields, now: Date): EntryResult {
  const { detail: _message, ...withoutMessage } = member;
  const reason = entryResult(withoutMessage, now);
  return {
    parts: [
      {
        text: t.activity.detail.pipelineAttention(
          t.activity.kind[member.kind],
          reason.parts.map((part) => part.text).join(" "),
        ),
      },
    ],
    tone: reason.tone,
  };
}

function processingMember(item: ActivityListItem): ActivityMemberSummary | undefined {
  return currentMembers(item.members).find(
    (member) => member.status === "running" && member.reason === "processing",
  );
}

/** The processing member's progress, which the card shows as today's import card does. */
export function pipelineProgress(item: ActivityListItem): ActivityProgress | undefined {
  if (item.status !== "running" || item.reason !== "processing") return undefined;
  return processingMember(item)?.progress;
}

/** The card's one-line summary of a pipeline, read without expanding it. */
export function pipelineSummary(item: ActivityListItem, now: Date): EntryResult {
  const members = currentMembers(item.members);
  const total = members.length;
  const done = members.filter((member) => member.status === "done").length;
  if (item.status === "running") {
    if (item.reason !== "processing")
      return {
        parts: [{ text: t.activity.pipeline.waiting(done, total) }],
        tone: "muted",
      };
    const kind = processingMember(item)?.kind ?? item.kind;
    return {
      parts: [
        { text: t.activity.pipeline.processing(t.activity.kind[kind], done, total) },
      ],
      tone: "muted",
    };
  }
  if (item.status === "attention") {
    const member = attentionMember(item.members);
    if (member !== undefined) return pipelineAttentionResult(member, now);
    return entryResult(item, now);
  }
  if (item.outcome === "gone" || item.outcome === "superseded")
    return entryResult(item, now);
  return { parts: [{ text: t.activity.pipeline.finished(total) }], tone: "muted" };
}

/** Open while it is processing or while a filter matched one of its members. */
export function startsExpanded(item: ActivityListItem): boolean {
  if (item.status === "running" && item.reason === "processing") return true;
  return currentMembers(item.members).some((member) => member.matched === true);
}
