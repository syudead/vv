import type { AssistantProposal, AssistantProposalStep } from "../api/assistant";
import { t } from "../i18n";
import { countsVideos, isFinished, reasonText, type RunLine } from "./proposal";

type Step = AssistantProposalStep;

/** The steps the owner ticked, by index. */
export type Selection = ReadonlySet<number>;

export function allSteps(proposal: AssistantProposal): Selection {
  return new Set(proposal.steps.map((step) => step.index));
}

/** Kept steps in card order, as Execute sends them. */
export function selectedIndexes(
  proposal: AssistantProposal,
  selected: Selection,
): number[] {
  return proposal.steps.map((step) => step.index).filter((index) => selected.has(index));
}

/** A ticked step that uses what an unticked step creates; the step numbers are from 1. */
export interface Blocked {
  step: number;
  needs: number;
}

export function blockedStep(
  proposal: AssistantProposal,
  selected: Selection,
): Blocked | undefined {
  for (const step of proposal.steps) {
    if (!selected.has(step.index)) continue;
    const missing = step.createdBy.find((by) => !selected.has(by.step));
    if (missing !== undefined) return { step: step.index + 1, needs: missing.step + 1 };
  }
  return undefined;
}

const stopping: ReadonlySet<Step["status"]> = new Set(["partial", "stale", "failed"]);

export type StepIcon = "skipped" | "waiting" | "running" | "done" | "warning" | "failed";

export interface StepLine extends RunLine {
  icon: StepIcon;
}

function runningLine(step: Step, batched: boolean): StepLine {
  const words = t.assistant.card;
  const { applied, total } = step.counts;
  return batched
    ? {
        icon: "running",
        text: words.running(applied, total),
        tone: "muted",
        progress: { value: applied, max: total },
      }
    : { icon: "running", text: words.runningOnce, tone: "muted" };
}

function failedLine(step: Step): StepLine {
  const words = t.assistant.card;
  return step.error?.code === "interrupted"
    ? { icon: "warning", text: words.interrupted(step.counts.applied), tone: "warning" }
    : {
        icon: "failed",
        text: words.steps.failed(reasonText(step.error)),
        tone: "destructive",
      };
}

/** A step's state after Execute: the icon in the checkbox column and the line under its rows. */
export function stepLine(step: Step): StepLine {
  const words = t.assistant.card;
  const { applied, failed } = step.counts;
  const batched = countsVideos(step);
  switch (step.status) {
    case "skipped":
      return { icon: "skipped", text: words.steps.skipped, tone: "muted" };
    case "running":
      return runningLine(step, batched);
    case "done":
      return {
        icon: "done",
        text: batched ? words.steps.done(applied) : words.doneOnce,
        tone: "success",
      };
    case "partial":
      return {
        icon: "warning",
        text: words.partial(applied, failed, reasonText(step.error)),
        tone: "warning",
      };
    case "stale":
      return { icon: "warning", text: words.steps.stale, tone: "warning" };
    case "failed":
      return failedLine(step);
    case "notRun":
      return { icon: "skipped", text: words.steps.notRun, tone: "muted" };
    default:
      return { icon: "waiting", text: words.steps.waiting, tone: "muted" };
  }
}

/** The card's last line once the run finished: every kept step done, or where it stopped. */
export function cardResult(proposal: AssistantProposal): RunLine | undefined {
  if (!isFinished(proposal.status)) return undefined;
  const words = t.assistant.card.steps;
  const stop = proposal.steps.find((step) => stopping.has(step.status));
  if (stop !== undefined) {
    return { text: words.stoppedAt(stop.index + 1), tone: stepLine(stop).tone };
  }
  const kept = proposal.steps.filter((step) => step.status !== "skipped").length;
  return { text: words.allDone(kept), tone: "success" };
}
