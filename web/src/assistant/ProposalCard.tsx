import { CircleCheck, TriangleAlert } from "lucide-react";
import { useId } from "react";

import type { AssistantProposal } from "../api/assistant";
import { t } from "../i18n";
import { cn } from "@/lib/cn";
import { Progress } from "../ui/shadcn/progress";
import { Spinner } from "../ui/shadcn/spinner";
import { cardProposal, impactText, proposalSentence, runLine } from "./proposal";
import {
  Actions,
  ActivityLink,
  Changes,
  Collapsed,
  Expired,
  Irreversible,
  Targets,
  toneClass,
} from "./ProposalParts";
import StepsCard from "./StepsCard";
import type { Card } from "./useProposals";

/** The run's state: the card's last row, and the second line of a run in Recent runs. */
export function RunState({ proposal }: { proposal: AssistantProposal }) {
  const line = runLine(cardProposal(proposal));
  if (line === undefined) return null;
  const Icon =
    line.tone === "success"
      ? CircleCheck
      : line.tone === "muted"
        ? undefined
        : TriangleAlert;
  return (
    <div className="flex flex-col gap-1">
      <p className={cn("flex items-start gap-2 text-sm", toneClass[line.tone])}>
        {line.spinner === true && <Spinner aria-hidden="true" className="mt-0.5" />}
        {Icon !== undefined && (
          <Icon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
        )}
        <span className="tabular-nums">{line.text}</span>
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

function OneStepCard({
  card,
  onExecute,
  onDismiss,
}: {
  card: Card;
  onExecute: () => void;
  onDismiss: () => void;
}) {
  const ids = useId();
  const proposal = cardProposal(card.proposal);
  const titleId = `${ids}-title`;
  const impactId = `${ids}-impact`;
  const irreversibleId = `${ids}-irreversible`;
  const impact = impactText(proposal);
  const describedBy = [
    titleId,
    impact === undefined ? undefined : impactId,
    proposal.irreversible ? irreversibleId : undefined,
  ]
    .filter((id) => id !== undefined)
    .join(" ");
  const waiting = card.view === "live" && proposal.status === "proposed";
  return (
    <article
      aria-labelledby={titleId}
      className="flex flex-col gap-2 rounded-lg border border-border bg-card p-3 text-card-foreground"
    >
      <p id={titleId} className="text-sm font-medium">
        {proposalSentence(proposal)}
      </p>
      {impact !== undefined && (
        <p id={impactId} className="text-lg font-semibold tabular-nums">
          {impact}
        </p>
      )}
      <Targets step={proposal} />
      <Changes step={proposal} />
      {proposal.irreversible && <Irreversible step={proposal} id={irreversibleId} />}
      {card.view === "expired" && <Expired />}
      {waiting ? (
        <Actions
          card={card}
          label={t.assistant.card.execute}
          destructive={proposal.irreversible}
          describedBy={describedBy}
          onExecute={onExecute}
          onDismiss={onDismiss}
        />
      ) : (
        card.view === "live" && <RunState proposal={card.proposal} />
      )}
      <ActivityLink proposal={proposal} />
    </article>
  );
}

/**
 * The confirmation card of a proposal; nothing is written until Execute, which sends the kept
 * steps' indexes.
 */
export default function ProposalCard({
  card,
  onExecute,
  onDismiss,
}: {
  card: Card;
  onExecute: (steps: number[]) => void;
  onDismiss: () => void;
}) {
  const { view, proposal } = card;
  if (view === "dismissed") return <Collapsed>{t.assistant.card.dismissed}</Collapsed>;
  if (view === "replaced" || proposal.status === "discarded") {
    return <Collapsed>{t.assistant.card.replaced}</Collapsed>;
  }
  if (proposal.steps.length > 1) {
    return <StepsCard card={card} onExecute={onExecute} onDismiss={onDismiss} />;
  }
  return (
    <OneStepCard
      card={card}
      onExecute={() => onExecute(proposal.steps.map((step) => step.index))}
      onDismiss={onDismiss}
    />
  );
}

/** One run in Recent runs: its title and state line, without targets and change. */
export function RunRow({ proposal: shown }: { proposal: AssistantProposal }) {
  const proposal = cardProposal(shown);
  return (
    <li className="flex flex-col gap-1">
      <p className="text-sm font-medium">{proposalSentence(proposal)}</p>
      <RunState proposal={shown} />
    </li>
  );
}
