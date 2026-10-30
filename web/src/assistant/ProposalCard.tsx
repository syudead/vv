import { CircleCheck, Plus, TriangleAlert } from "lucide-react";
import { useId } from "react";
import { Link } from "react-router";

import type { AssistantLabel, AssistantProposal } from "../api/assistant";
import { personImagePath } from "../api/credits";
import { errorText, t } from "../i18n";
import { cn } from "@/lib/cn";
import { PersonFace } from "../ui/PersonFace";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Progress } from "../ui/shadcn/progress";
import { Spinner } from "../ui/shadcn/spinner";
import {
  argumentName,
  type CardProposal,
  cardProposal,
  createdElsewhere,
  impactText,
  isCreated,
  isFinished,
  labelKey,
  mergeDestination,
  moreTargets,
  proposalSentence,
  type RunTone,
  runLine,
  valueStrings,
  valueText,
} from "./proposal";
import type { Card } from "./useProposals";

const toneClass: Record<RunTone, string> = {
  muted: "text-muted-foreground",
  success: "text-success",
  warning: "text-warning",
  destructive: "text-destructive",
};

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

function ActivityLink({ proposal }: { proposal: CardProposal }) {
  if (!isFinished(proposal.status) || proposal.activityId === undefined) return null;
  return (
    <Button asChild variant="link" size="sm" className="h-auto self-start p-0">
      <Link to={`/activity/${String(proposal.activityId)}`}>
        {t.assistant.card.openInActivity}
      </Link>
    </Button>
  );
}

function NameBadge({
  name,
  created,
  kind,
}: {
  name: string;
  created: boolean;
  kind: string;
}) {
  if (!created) return <Badge variant="secondary">{name}</Badge>;
  return (
    <span className="inline-flex gap-1">
      <span className="sr-only">{t.assistant.card.newNamed(kind, name)}</span>
      <Badge variant="outline" aria-hidden="true">
        <Plus />
        {t.assistant.card.newMark}
      </Badge>
      <Badge variant="secondary" aria-hidden="true">
        {name}
      </Badge>
    </span>
  );
}

function LabelBadge({ label }: { label: AssistantLabel }) {
  if (label.kind === "person" && label.id !== undefined) {
    return (
      <Badge variant="secondary" className="pl-0.5">
        <PersonFace shape="chip" name={label.name} src={personImagePath(label.id)} />
        {label.name}
      </Badge>
    );
  }
  return <Badge variant="secondary">{label.name}</Badge>;
}

function Targets({ proposal }: { proposal: CardProposal }) {
  const { labels } = proposal.targets;
  const more = moreTargets(proposal);
  if (labels.length === 0) return null;
  return (
    <div className="flex flex-wrap items-center gap-1">
      {labels.map((label, index) => (
        <LabelBadge key={labelKey(label, index)} label={label} />
      ))}
      {more > 0 && (
        <span className="text-xs text-muted-foreground tabular-nums">
          {t.assistant.card.more(more)}
        </span>
      )}
    </div>
  );
}

/** The kind a created name gets in its spoken name; tags are what attaching creates. */
function createdKind(argument: string, label?: AssistantLabel): string {
  if (label !== undefined) return t.assistant.clarify.kinds[label.kind];
  return argument === "tagNames" ? t.assistant.clarify.kinds.tag : "";
}

function ChangeLine({
  proposal,
  change,
}: {
  proposal: CardProposal;
  change: CardProposal["change"][number];
}) {
  const text = valueText(change.argument, change.value);
  const names = valueStrings(change.value);
  return (
    <div className="flex flex-wrap items-center gap-1 text-sm">
      <span className="text-muted-foreground">
        {argumentName(proposal, change.argument)}
      </span>
      {(change.labels ?? []).map((label, index) => (
        <NameBadge
          key={labelKey(label, index)}
          name={label.name}
          kind={createdKind(change.argument, label)}
          created={isCreated(proposal, label.name)}
        />
      ))}
      {names.map((name) => (
        <NameBadge
          key={name}
          name={name}
          kind={createdKind(change.argument)}
          created={isCreated(proposal, name)}
        />
      ))}
      {text !== undefined && <span>{text}</span>}
    </div>
  );
}

function Changes({ proposal }: { proposal: CardProposal }) {
  const extra = createdElsewhere(proposal);
  return (
    <>
      {proposal.change.map((change, index) => (
        <ChangeLine
          key={`${change.argument}:${String(index)}`}
          proposal={proposal}
          change={change}
        />
      ))}
      {extra.length > 0 && (
        <div className="flex flex-wrap items-center gap-1 text-sm">
          <span className="text-muted-foreground">{t.assistant.card.created}</span>
          {extra.map((name) => (
            <NameBadge key={name} name={name} kind="" created />
          ))}
        </div>
      )}
    </>
  );
}

function Irreversible({ proposal, id }: { proposal: CardProposal; id: string }) {
  const destination = mergeDestination(proposal);
  return (
    <div id={id} className="flex flex-col gap-1">
      <p className="flex items-center gap-2 text-sm font-medium text-warning">
        <TriangleAlert aria-hidden="true" className="size-4 shrink-0" />
        {t.assistant.card.irreversible}
      </p>
      {proposal.impact !== undefined && destination !== undefined && (
        <p className="text-sm text-muted-foreground">
          {t.assistant.card.moves(proposal.impact.videos, destination)}
        </p>
      )}
    </div>
  );
}

function Actions({
  card,
  describedBy,
  onExecute,
  onDismiss,
}: {
  card: Card;
  describedBy: string;
  onExecute: () => void;
  onDismiss: () => void;
}) {
  const words = t.assistant.card;
  return (
    <div className="flex flex-col gap-2">
      {card.error !== undefined && (
        <p className="text-sm text-destructive">
          {words.executeFailed(errorText(card.error))}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button
          size="sm"
          variant={
            card.proposal.steps.some((step) => step.irreversible)
              ? "destructive"
              : "default"
          }
          aria-disabled={card.executing}
          aria-describedby={describedBy}
          onClick={() => {
            if (!card.executing) onExecute();
          }}
        >
          {card.executing && <Spinner aria-hidden="true" />}
          {card.executing ? words.executing : words.execute}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          aria-disabled={card.executing}
          onClick={() => {
            if (!card.executing) onDismiss();
          }}
        >
          {words.dismiss}
        </Button>
      </div>
    </div>
  );
}

function Collapsed({ children }: { children: string }) {
  return <p className="text-xs text-muted-foreground">{children}</p>;
}

/** The confirmation card of a proposal outcome; nothing is written until Execute. */
export default function ProposalCard({
  card,
  onExecute,
  onDismiss,
}: {
  card: Card;
  onExecute: () => void;
  onDismiss: () => void;
}) {
  const ids = useId();
  const { view } = card;
  const proposal = cardProposal(card.proposal);
  if (view === "dismissed") return <Collapsed>{t.assistant.card.dismissed}</Collapsed>;
  if (view === "replaced" || proposal.status === "discarded") {
    return <Collapsed>{t.assistant.card.replaced}</Collapsed>;
  }
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
  const waiting = view === "live" && proposal.status === "proposed";
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
      <Targets proposal={proposal} />
      <Changes proposal={proposal} />
      {proposal.irreversible && <Irreversible proposal={proposal} id={irreversibleId} />}
      {view === "expired" && (
        <p className="flex items-start gap-2 text-sm text-warning">
          <TriangleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
          {t.assistant.card.expired}
        </p>
      )}
      {waiting ? (
        <Actions
          card={card}
          describedBy={describedBy}
          onExecute={onExecute}
          onDismiss={onDismiss}
        />
      ) : (
        view === "live" && <RunState proposal={card.proposal} />
      )}
      <ActivityLink proposal={proposal} />
    </article>
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
