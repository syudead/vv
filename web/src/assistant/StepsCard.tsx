import {
  Circle,
  CircleCheck,
  CircleMinus,
  CircleX,
  type LucideIcon,
  TriangleAlert,
} from "lucide-react";
import { Fragment, useId, useState } from "react";

import type { AssistantProposal, AssistantProposalStep } from "../api/assistant";
import { t } from "../i18n";
import { cn } from "@/lib/cn";
import { Checkbox } from "../ui/shadcn/checkbox";
import { Progress } from "../ui/shadcn/progress";
import { Separator } from "../ui/shadcn/separator";
import { Spinner } from "../ui/shadcn/spinner";
import { impactText, proposalSentence, type RunTone } from "./proposal";
import {
  Actions,
  ActivityLink,
  Changes,
  Expired,
  Irreversible,
  Targets,
  toneClass,
} from "./ProposalParts";
import {
  allSteps,
  blockedStep,
  cardResult,
  type Selection,
  selectedIndexes,
  type StepIcon,
  stepLine,
} from "./steps";
import type { Card } from "./useProposals";

type Step = AssistantProposalStep;

const stepIcons: Record<Exclude<StepIcon, "running">, LucideIcon> = {
  skipped: CircleMinus,
  waiting: Circle,
  done: CircleCheck,
  warning: TriangleAlert,
  failed: CircleX,
};

const resultIcons: Record<RunTone, LucideIcon> = {
  muted: Circle,
  success: CircleCheck,
  warning: TriangleAlert,
  destructive: CircleX,
};

function StateIcon({ step }: { step: Step }) {
  const line = stepLine(step);
  if (line.icon === "running") return <Spinner aria-hidden="true" />;
  const Icon = stepIcons[line.icon];
  return <Icon aria-hidden="true" className={cn("size-4", toneClass[line.tone])} />;
}

function StepState({ step }: { step: Step }) {
  const line = stepLine(step);
  return (
    <div className="flex flex-col gap-1">
      <p
        className={cn(
          "tabular-nums",
          line.tone === "muted" ? "text-xs" : "text-sm",
          toneClass[line.tone],
        )}
      >
        {line.text}
      </p>
      {line.progress !== undefined && (
        <Progress
          aria-label={t.assistant.card.progress}
          value={line.progress.value}
          max={line.progress.max}
        />
      )}
    </div>
  );
}

function StepBlock({
  step,
  waiting,
  selected,
  onTick,
}: {
  step: Step;
  waiting: boolean;
  selected: Selection;
  onTick: (index: number, ticked: boolean) => void;
}) {
  const words = t.assistant.card.steps;
  const number = step.index + 1;
  const ticked = waiting ? selected.has(step.index) : step.status !== "skipped";
  const impact = impactText(step);
  const marks = waiting ? selected : undefined;
  return (
    <div
      className={cn(
        "grid grid-cols-[1rem_minmax(0,1fr)] gap-x-3 py-3",
        step.irreversible && "border-l-2 border-warning pl-3",
      )}
    >
      <div className="flex h-5 items-center">
        {waiting ? (
          <Checkbox
            aria-label={words.include(number)}
            checked={ticked}
            onCheckedChange={(checked) => onTick(step.index, checked === true)}
          />
        ) : (
          <StateIcon step={step} />
        )}
      </div>
      <div className="flex min-w-0 flex-col gap-2">
        <div className="flex flex-col gap-0.5">
          <p className="flex items-baseline gap-2">
            <span className="text-xs text-muted-foreground tabular-nums">
              {String(number)}
            </span>
            <span className="text-sm font-medium">{proposalSentence(step)}</span>
          </p>
          {waiting && !ticked && (
            <p className="text-xs text-muted-foreground">{words.wontRun}</p>
          )}
        </div>
        {impact !== undefined && (
          <p
            className={cn(
              "text-lg font-semibold tabular-nums",
              !ticked && "text-muted-foreground line-through",
            )}
          >
            {impact}
          </p>
        )}
        {step.irreversible && <Irreversible step={step} />}
        <Targets step={step} selected={marks} />
        <Changes step={step} selected={marks} />
        {!waiting && <StepState step={step} />}
      </div>
    </div>
  );
}

function Result({ proposal }: { proposal: AssistantProposal }) {
  const line = cardResult(proposal);
  if (line === undefined) return null;
  const Icon = resultIcons[line.tone];
  return (
    <p className={cn("flex items-start gap-2 text-sm", toneClass[line.tone])}>
      <Icon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      <span className="tabular-nums">{line.text}</span>
    </p>
  );
}

/** Why Execute does nothing for this selection, if it does nothing. */
function selectionMessage(
  proposal: AssistantProposal,
  selected: Selection,
): string | undefined {
  const words = t.assistant.card.steps;
  if (selected.size === 0) return words.noneSelected;
  const blocked = blockedStep(proposal, selected);
  return blocked === undefined ? undefined : words.blocked(blocked.step, blocked.needs);
}

/**
 * A card of two or more steps: each step with its checkbox while it waits, and its state after
 * Execute, so the run reads against the rows the owner approved.
 */
export default function StepsCard({
  card,
  onExecute,
  onDismiss,
}: {
  card: Card;
  onExecute: (steps: number[]) => void;
  onDismiss: () => void;
}) {
  const headingId = useId();
  const { proposal, view } = card;
  const [selected, setSelected] = useState<Selection>(() => allSteps(proposal));
  const waiting = view === "live" && proposal.status === "proposed";
  const kept = waiting
    ? selectedIndexes(proposal, selected)
    : proposal.steps.filter((step) => step.status !== "skipped").map((step) => step.index);
  const words = t.assistant.card;
  const tick = (index: number, ticked: boolean) =>
    setSelected((current) => {
      const next = new Set(current);
      if (ticked) next.add(index);
      else next.delete(index);
      return next;
    });
  return (
    <article
      aria-labelledby={headingId}
      className="flex flex-col gap-2 rounded-lg border border-border bg-card p-3 text-card-foreground"
    >
      <p id={headingId} className="text-xs text-muted-foreground tabular-nums">
        {words.steps.heading(proposal.steps.length, kept.length)}
      </p>
      <div className="flex flex-col">
        {proposal.steps.map((step, index) => (
          <Fragment key={step.index}>
            {index > 0 && <Separator />}
            <StepBlock
              step={step}
              waiting={waiting}
              selected={selected}
              onTick={tick}
            />
          </Fragment>
        ))}
      </div>
      {view === "expired" && <Expired />}
      {waiting ? (
        <Actions
          card={card}
          label={kept.length > 1 ? words.steps.execute(kept.length) : words.execute}
          destructive={proposal.steps.some(
            (step) => step.irreversible && selected.has(step.index),
          )}
          message={selectionMessage(proposal, selected)}
          describedBy={headingId}
          onExecute={() => onExecute(kept)}
          onDismiss={onDismiss}
        />
      ) : (
        view === "live" && <Result proposal={proposal} />
      )}
      <ActivityLink proposal={proposal} />
    </article>
  );
}
