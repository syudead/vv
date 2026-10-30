import type {
  AssistantLabel,
  AssistantLabelKind,
  AssistantProposal,
  AssistantProposalStatus,
  AssistantProposalStep,
} from "../api/assistant";
import type { OperationInput } from "../i18n/en";
import { errorText, formatNumber, t, type UiText } from "../i18n";
import { RequestFailed } from "../api/client";

type Change = AssistantProposalStep["change"][number];
type Counts = AssistantProposalStep["counts"];

/**
 * The one-operation card's view of a proposal: its first step, with the counts summed over the
 * kept steps and the error of the last step that ran. Every proposal a turn stores has one step
 * until turns build several.
 */
export type CardProposal = Omit<AssistantProposal, "steps"> &
  Omit<AssistantProposalStep, "index" | "status" | "createdBy" | "error"> & {
    error?: AssistantProposalStep["error"];
  };

function keptCounts(proposal: AssistantProposal): Counts {
  const counts: Counts = { total: 0, applied: 0, failed: 0 };
  for (const step of proposal.steps) {
    if (step.status === "skipped") continue;
    counts.total += step.counts.total;
    counts.applied += step.counts.applied;
    counts.failed += step.counts.failed;
  }
  return counts;
}

function lastError(proposal: AssistantProposal): AssistantProposalStep["error"] {
  let error: AssistantProposalStep["error"];
  for (const step of proposal.steps) error = step.error ?? error;
  return error;
}

export function cardProposal(proposal: AssistantProposal): CardProposal {
  const first = proposal.steps[0];
  const error = lastError(proposal);
  return {
    id: proposal.id,
    status: proposal.status,
    request: proposal.request,
    ...(proposal.turnId === undefined ? {} : { turnId: proposal.turnId }),
    ...(proposal.activityId === undefined ? {} : { activityId: proposal.activityId }),
    ...(proposal.startedAt === undefined ? {} : { startedAt: proposal.startedAt }),
    ...(proposal.finishedAt === undefined ? {} : { finishedAt: proposal.finishedAt }),
    operation: first?.operation ?? "",
    irreversible: proposal.steps.some((step) => step.irreversible),
    targets: first?.targets ?? { kind: "", count: 0, labels: [] },
    change: first?.change ?? [],
    created: first?.created ?? [],
    ...(first?.impact === undefined ? {} : { impact: first.impact }),
    counts: keptCounts(proposal),
    ...(error === undefined ? {} : { error }),
  };
}

function changeOf(proposal: CardProposal, argument: string): Change | undefined {
  return proposal.change.find((change) => change.argument === argument);
}

function changeName(change: Change | undefined): string | undefined {
  if (change === undefined) return undefined;
  const label = change.labels?.[0];
  if (label !== undefined) return label.name;
  return typeof change.value === "string" ? change.value : undefined;
}

/**
 * The name an operation sentence quotes: the new name of a rename, the collection or name the
 * change names, or the one target the row acts on.
 */
function sentenceName(proposal: CardProposal): string | undefined {
  return (
    changeName(changeOf(proposal, "newName")) ??
    changeName(changeOf(proposal, "name")) ??
    proposal.targets.labels[0]?.name
  );
}

function switchValue(proposal: CardProposal): boolean | undefined {
  const value = (changeOf(proposal, "public") ?? changeOf(proposal, "enabled"))?.value;
  return typeof value === "boolean" ? value : undefined;
}

/** The card's title; the Activity entry, which keeps only the id, passes no input. */
export function operationSentence(operation: string, input: OperationInput = {}): UiText {
  const sentences: Readonly<Record<string, (value: OperationInput) => UiText>> =
    t.assistant.operation;
  const sentence = sentences[operation];
  return sentence === undefined ? t.assistant.otherOperation : sentence(input);
}

export function proposalSentence(proposal: CardProposal): UiText {
  return operationSentence(proposal.operation, {
    name: sentenceName(proposal),
    on: switchValue(proposal),
  });
}

const videoTargets: ReadonlySet<string> = new Set([
  "videos",
  "videosOrGroups",
  "video",
  "videoPair",
  "virtualVideo",
]);

/** Batched runs count videos; every other run is one call whose counts are 0 or 1. */
export function countsVideos(proposal: CardProposal): boolean {
  return (
    videoTargets.has(proposal.targets.kind) &&
    proposal.targets.labels.every((label) => label.kind === "video") &&
    proposal.counts.total === proposal.targets.count &&
    proposal.counts.total > 0
  );
}

const targetUnits: Readonly<Record<string, AssistantLabelKind>> = {
  videos: "video",
  videosOrGroups: "video",
  video: "video",
  videoPair: "video",
  virtualVideo: "video",
  tag: "tag",
  tags: "tag",
  tagKind: "tagKind",
  person: "person",
  organization: "organization",
  playlist: "playlist",
  group: "group",
  smartFolder: "smartFolder",
  folderRoot: "folderRoot",
};

function unitText(kind: AssistantLabelKind | "items", count: number): string {
  return t.assistant.card.impact[kind](count);
}

/**
 * The card's largest text. A merge counts the videos that move, which the server counted;
 * its targets count is the one destination. Undefined for a row that names no target but
 * creates something: its title already says what.
 */
export function impactText(proposal: CardProposal): string | undefined {
  if (proposal.impact !== undefined) return unitText("video", proposal.impact.videos);
  const { kind, count, labels } = proposal.targets;
  const kinds = new Set(labels.map((label) => label.kind));
  if (kinds.size > 1) return unitText("items", count);
  const [labelKind] = kinds;
  const unit = labelKind ?? targetUnits[kind];
  if (unit !== undefined) return unitText(unit, count);
  return proposal.operation.startsWith("start") || proposal.operation.endsWith("Settings")
    ? t.assistant.card.impact.setting
    : undefined;
}

/** The destination a merge moves videos to, for the irreversible line. */
export function mergeDestination(proposal: CardProposal): string | undefined {
  return proposal.impact === undefined ? undefined : proposal.targets.labels[0]?.name;
}

/** The labels past the first 10 that the card does not name. */
export function moreTargets(proposal: CardProposal): number {
  return Math.max(0, proposal.targets.count - proposal.targets.labels.length);
}

export function argumentName(proposal: CardProposal, argument: string): string {
  const words = t.assistant.card;
  if (argument === "names" && proposal.impact !== undefined) return words.mergeFrom;
  return words.argument[argument] ?? argument;
}

/** A value the change line writes as text; labels and strings are drawn as badges instead. */
export function valueText(argument: string, value: unknown): string | undefined {
  const words = t.assistant.card;
  if (typeof value === "boolean") {
    if (argument === "public") return value ? words.public : words.private;
    return value ? words.on : words.off;
  }
  if (typeof value === "number") return formatNumber(value);
  return undefined;
}

/** Strings a change names: one, or each of a list such as aliases. */
export function valueStrings(value: unknown): string[] {
  if (typeof value === "string") return value === "" ? [] : [value];
  if (Array.isArray(value)) return value.filter((item) => typeof item === "string");
  return [];
}

export function isCreated(proposal: CardProposal, name: string): boolean {
  return proposal.created.includes(name);
}

/** Created names no change line shows, listed on a line of their own. */
export function createdElsewhere(proposal: CardProposal): string[] {
  const shown = new Set<string>();
  for (const change of proposal.change) {
    for (const label of change.labels ?? []) shown.add(label.name);
    for (const value of valueStrings(change.value)) shown.add(value);
  }
  return proposal.created.filter((name) => !shown.has(name));
}

export function labelKey(label: AssistantLabel, index: number): string {
  return `${label.kind}:${String(label.id ?? label.name)}:${String(index)}`;
}

const order: Readonly<Record<AssistantProposalStatus, number>> = {
  proposed: 0,
  discarded: 1,
  approved: 1,
  running: 2,
  done: 3,
  partial: 3,
  stale: 3,
  failed: 3,
};

/** A read that answers later than another must not move the card back. */
export function isNewer(next: AssistantProposal, current: AssistantProposal): boolean {
  const step = order[next.status] - order[current.status];
  if (step !== 0) return step > 0;
  const nextCounts = keptCounts(next);
  const currentCounts = keptCounts(current);
  return (
    nextCounts.applied + nextCounts.failed >= currentCounts.applied + currentCounts.failed
  );
}

export function isFinished(status: AssistantProposalStatus): boolean {
  return order[status] === 3;
}

/** Approved and running proposals change on assistant events; the others are settled. */
export function isActive(status: AssistantProposalStatus): boolean {
  return status === "approved" || status === "running";
}

export type RunTone = "muted" | "success" | "warning" | "destructive";

export interface RunLine {
  text: string;
  tone: RunTone;
  /** Present while a batched run counts its videos. */
  progress?: { value: number; max: number };
  spinner?: boolean;
}

function reasonText(proposal: CardProposal): string {
  const error = proposal.error;
  if (error === undefined) return "";
  return String(
    errorText(new RequestFailed(0, error.code, error.code, { reason: error.reason })),
  );
}

/** The state line under the card's rows, and the one line of a run in Recent runs. */
export function runLine(proposal: CardProposal): RunLine | undefined {
  const words = t.assistant.card;
  const { applied, failed, total } = proposal.counts;
  const batched = countsVideos(proposal);
  switch (proposal.status) {
    case "approved":
    case "running":
      return batched
        ? {
            text: words.running(applied, total),
            tone: "muted",
            spinner: true,
            progress: { value: applied, max: total },
          }
        : { text: words.runningOnce, tone: "muted", spinner: true };
    case "done":
      return { text: batched ? words.done(applied) : words.doneOnce, tone: "success" };
    case "partial":
      return {
        text: words.partial(applied, failed, reasonText(proposal)),
        tone: "warning",
      };
    case "stale":
      return { text: words.stale, tone: "warning" };
    case "failed":
      return proposal.error?.code === "interrupted"
        ? { text: words.interrupted(applied), tone: "warning" }
        : { text: words.failed(reasonText(proposal)), tone: "destructive" };
    default:
      return undefined;
  }
}
