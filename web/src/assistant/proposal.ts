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
import { formatDuration } from "../lib/format";

export type Change = AssistantProposalStep["change"][number];
type Counts = AssistantProposalStep["counts"];

/** What the card draws for one step; the one-step card's merged proposal has the same fields. */
export type StepFields = Pick<
  AssistantProposalStep,
  "operation" | "targets" | "change" | "created" | "impact" | "counts"
> & { createdBy?: AssistantProposalStep["createdBy"] };

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

function changeOf(proposal: StepFields, argument: string): Change | undefined {
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
function sentenceName(proposal: StepFields): string | undefined {
  return (
    changeName(changeOf(proposal, "newName")) ??
    changeName(changeOf(proposal, "name")) ??
    // A move names the person the faces go to, which the body's targetId carries.
    (proposal.operation === "moveFaceAppearances"
      ? changeName(changeOf(proposal, "targetId"))
      : undefined) ??
    proposal.targets.labels[0]?.name
  );
}

function switchValue(proposal: StepFields): boolean | undefined {
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

export function proposalSentence(proposal: StepFields): UiText {
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
export function countsVideos(proposal: StepFields): boolean {
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
export function impactText(proposal: StepFields): string | undefined {
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
export function mergeDestination(proposal: StepFields): string | undefined {
  return proposal.impact === undefined ? undefined : proposal.targets.labels[0]?.name;
}

/** The labels past the first 10 that the card does not name. */
export function moreTargets(proposal: StepFields): number {
  return Math.max(0, proposal.targets.count - proposal.targets.labels.length);
}

/** The field names the work-information and person editors give the fields a fetch applies. */
function fetchFieldName(operation: string, argument: string): string | undefined {
  const work: Readonly<Record<string, string>> = t.player.workInfoFetch.fields;
  const person: Readonly<Record<string, string>> = t.credits.personPage.pluginFetch.items;
  if (operation === "applyWorkInfoFetch") return work[argument];
  if (operation === "applyPersonPluginFetch")
    return person[argument] ?? person[`profile.${argument}`];
  return undefined;
}

export function argumentName(proposal: StepFields, argument: string): string {
  const words = t.assistant.card;
  if (argument === "names" && proposal.impact !== undefined) return words.mergeFrom;
  return (
    fetchFieldName(proposal.operation, argument) ?? words.argument[argument] ?? argument
  );
}

/**
 * One piece of a change line after the argument's name: a name, drawn as a badge that can carry
 * New; one item of a list, drawn as a badge; or text.
 */
export type ValuePart =
  | { kind: "name"; name: string }
  | { kind: "item"; text: string }
  | { kind: "text"; text: string; long?: boolean };

/** Rows whose strings are fetched or typed text rather than names. */
const textRows: ReadonlySet<string> = new Set([
  "applyWorkInfoFetch",
  "applyPersonPluginFetch",
  "editVideoThumbnail",
]);

function switchText(argument: string, value: boolean): string {
  const words = t.assistant.card;
  if (argument === "public") return value ? words.public : words.private;
  return value ? words.on : words.off;
}

/** Any JSON value as one line, so a body field the card cannot name is still shown in full. */
function itemText(value: unknown): string {
  if (value === null || value === undefined) return t.assistant.card.none;
  if (typeof value === "string") return value;
  if (typeof value === "number") return formatNumber(value);
  if (typeof value === "boolean") return switchText("", value);
  if (Array.isArray(value)) return value.map(itemText).join(", ");
  if (typeof value === "object") {
    return Object.entries(value)
      .map(([key, item]) => `${key}: ${itemText(item)}`)
      .join(", ");
  }
  return JSON.stringify(value) ?? "";
}

/** An id list's count, which the change line writes before the first labels. */
export function idCount(change: Change): string | undefined {
  const first = change.labels?.[0];
  if (first === undefined || typeof change.value !== "number") return undefined;
  return unitText(first.kind, change.value);
}

/** What a change line shows of its value; labels are drawn before it. */
export function valueParts(step: StepFields, change: Change): ValuePart[] {
  const { argument, value } = change;
  if (value === undefined || idCount(change) !== undefined) return [];
  const textRow = textRows.has(step.operation);
  // A fetch applies an image as its index in the result; the card names it, since it shows no image.
  if (textRow && (argument === "cover" || argument === "image")) {
    return [{ kind: "text", text: t.assistant.card.newImage }];
  }
  if (argument === "atMs" && typeof value === "number") {
    return [{ kind: "text", text: formatDuration(value) }];
  }
  if (typeof value === "boolean")
    return [{ kind: "text", text: switchText(argument, value) }];
  if (typeof value === "string") {
    if (value === "") return [];
    return textRow
      ? [{ kind: "text", text: value, long: true }]
      : [{ kind: "name", name: value }];
  }
  if (Array.isArray(value)) {
    return value.map((item: unknown) =>
      typeof item === "string" && !textRow
        ? { kind: "name", name: item }
        : { kind: "item", text: itemText(item) },
    );
  }
  return [{ kind: "text", text: itemText(value) }];
}

/** Strings a change names: one, or each of a list such as aliases. */
export function valueStrings(value: unknown): string[] {
  if (typeof value === "string") return value === "" ? [] : [value];
  if (Array.isArray(value)) return value.filter((item) => typeof item === "string");
  return [];
}

/** The step, from 0, that creates a name this step uses. */
export function creatingStep(step: StepFields, name: string): number | undefined {
  return step.createdBy?.find((by) => by.name === name)?.step;
}

export function isCreated(proposal: StepFields, name: string): boolean {
  return proposal.created.includes(name) || creatingStep(proposal, name) !== undefined;
}

/** Created names no change line shows, listed on a line of their own. */
export function createdElsewhere(proposal: StepFields): string[] {
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

export function reasonText(error: AssistantProposalStep["error"]): string {
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
        text: words.partial(applied, failed, reasonText(proposal.error)),
        tone: "warning",
      };
    case "stale":
      return { text: words.steps.stale, tone: "warning" };
    case "failed":
      return proposal.error?.code === "interrupted"
        ? { text: words.interrupted(applied), tone: "warning" }
        : { text: words.failed(reasonText(proposal.error)), tone: "destructive" };
    default:
      return undefined;
  }
}
