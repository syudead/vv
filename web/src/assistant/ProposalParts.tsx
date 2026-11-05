import { Plus, TriangleAlert } from "lucide-react";
import { Link } from "react-router";

import type { AssistantLabel, AssistantProposal } from "../api/assistant";
import { personImagePath } from "../api/credits";
import { faceCropPath } from "../api/personFaces";
import { errorText, t } from "../i18n";
import { cn } from "@/lib/cn";
import { PersonFace } from "../ui/PersonFace";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import {
  argumentName,
  type Change,
  createdElsewhere,
  creatingStep,
  idCount,
  isCreated,
  isFinished,
  labelKey,
  mergeDestination,
  moreTargets,
  type RunTone,
  type StepFields,
  type ValuePart,
  valueParts,
} from "./proposal";
import type { Selection } from "./steps";
import type { Card } from "./useProposals";

export const toneClass: Record<RunTone, string> = {
  muted: "text-muted-foreground",
  success: "text-success",
  warning: "text-warning",
  destructive: "text-destructive",
};

/**
 * The ticked steps while the owner chooses; a name an unticked step creates reads as needing it.
 * Undefined once Execute was pressed, or on a one-step card.
 */
interface Marks {
  selected?: Selection;
}

/** The step mark after a name an earlier step creates. */
function CreatedBy({ step, selected }: { step: number; selected?: Selection }) {
  const words = t.assistant.card.steps;
  const missing = selected !== undefined && !selected.has(step);
  return (
    <span
      className={cn(
        "text-xs tabular-nums",
        missing ? "text-destructive" : "text-muted-foreground",
      )}
    >
      {missing ? words.needs(step + 1) : words.createdBy(step + 1)}
    </span>
  );
}

function NameBadge({
  name,
  created,
  kind,
  by,
  selected,
}: {
  name: string;
  created: boolean;
  kind: string;
  by?: number;
} & Marks) {
  if (!created) return <Badge variant="secondary">{name}</Badge>;
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      <span className="sr-only">{t.assistant.card.newNamed(kind, name)}</span>
      <Badge variant="outline" aria-hidden="true">
        <Plus />
        {t.assistant.card.newMark}
      </Badge>
      <Badge variant="secondary" aria-hidden="true">
        {name}
      </Badge>
      {by !== undefined && <CreatedBy step={by} selected={selected} />}
    </span>
  );
}

/** A face's crop for a face appearance, a person's face for a person. */
function faceSource(label: AssistantLabel): string | undefined {
  if (label.id === undefined || label.id <= 0) return undefined;
  if (label.kind === "person") return personImagePath(label.id);
  if (label.kind === "faceAppearance") return faceCropPath(label.id);
  return undefined;
}

function LabelBadge({ label }: { label: AssistantLabel }) {
  const face = faceSource(label);
  if (face !== undefined) {
    return (
      <Badge variant="secondary" className="pl-0.5">
        <PersonFace shape="chip" name={label.name} src={face} />
        {label.name}
      </Badge>
    );
  }
  return <Badge variant="secondary">{label.name}</Badge>;
}

/** The kind a created name gets in its spoken name; tags are what attaching creates. */
function createdKind(argument: string, label?: AssistantLabel): string {
  if (label !== undefined) return t.assistant.clarify.kinds[label.kind];
  return argument === "tagNames" ? t.assistant.clarify.kinds.tag : "";
}

function EntityBadge({
  step,
  label,
  argument,
  selected,
}: { step: StepFields; label: AssistantLabel; argument: string } & Marks) {
  if (!isCreated(step, label.name)) return <LabelBadge label={label} />;
  return (
    <NameBadge
      name={label.name}
      kind={createdKind(argument, label)}
      created
      by={creatingStep(step, label.name)}
      selected={selected}
    />
  );
}

export function Targets({ step, selected }: { step: StepFields } & Marks) {
  const { labels } = step.targets;
  const more = moreTargets(step);
  if (labels.length === 0) return null;
  return (
    <div className="flex flex-wrap items-center gap-1">
      {labels.map((label, index) => (
        <EntityBadge
          key={labelKey(label, index)}
          step={step}
          label={label}
          argument=""
          selected={selected}
        />
      ))}
      {more > 0 && (
        <span className="text-xs text-muted-foreground tabular-nums">
          {t.assistant.card.more(more)}
        </span>
      )}
    </div>
  );
}

function Value({
  step,
  part,
  argument,
  selected,
}: { step: StepFields; part: ValuePart; argument: string } & Marks) {
  switch (part.kind) {
    case "name":
      return (
        <NameBadge
          name={part.name}
          kind={createdKind(argument)}
          created={isCreated(step, part.name)}
          by={creatingStep(step, part.name)}
          selected={selected}
        />
      );
    case "item":
      return <Badge variant="secondary">{part.text}</Badge>;
    default:
      return (
        <span className={cn(part.long === true && "line-clamp-2 min-w-0 break-words")}>
          {part.text}
        </span>
      );
  }
}

function ChangeLine({
  step,
  change,
  selected,
}: { step: StepFields; change: Change } & Marks) {
  const count = idCount(change);
  const parts = valueParts(step, change);
  return (
    <div className="flex flex-wrap items-center gap-1 text-sm">
      <span className="text-muted-foreground">{argumentName(step, change.argument)}</span>
      {count !== undefined && <span className="tabular-nums">{count}</span>}
      {(change.labels ?? []).map((label, index) => (
        <EntityBadge
          key={labelKey(label, index)}
          step={step}
          label={label}
          argument={change.argument}
          selected={selected}
        />
      ))}
      {parts.map((part, index) => (
        <Value
          // Parts are positional: a list may repeat a value.
          key={`${part.kind}:${String(index)}`}
          step={step}
          part={part}
          argument={change.argument}
          selected={selected}
        />
      ))}
    </div>
  );
}

export function Changes({ step, selected }: { step: StepFields } & Marks) {
  const extra = createdElsewhere(step);
  return (
    <>
      {step.change.map((change, index) => (
        <ChangeLine
          key={`${change.argument}:${String(index)}`}
          step={step}
          change={change}
          selected={selected}
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
      {step.operation === "editVideoThumbnail" && (
        <p className="text-xs text-muted-foreground">{t.assistant.card.restoreUndoes}</p>
      )}
    </>
  );
}

export function ActivityLink({
  proposal,
}: {
  proposal: Pick<AssistantProposal, "status" | "activityId">;
}) {
  if (!isFinished(proposal.status) || proposal.activityId === undefined) return null;
  return (
    <Button asChild variant="link" size="sm" className="h-auto self-start p-0">
      <Link to={`/activity/${String(proposal.activityId)}`}>
        {t.assistant.card.openInActivity}
      </Link>
    </Button>
  );
}

/** Execute and Dismiss while the card waits; message says why Execute does nothing. */
export function Actions({
  card,
  label,
  destructive,
  message,
  describedBy,
  onExecute,
  onDismiss,
}: {
  card: Card;
  label: string;
  destructive: boolean;
  message?: string;
  describedBy: string;
  onExecute: () => void;
  onDismiss: () => void;
}) {
  const words = t.assistant.card;
  const blocked = card.executing || message !== undefined;
  return (
    <div className="flex flex-col gap-2">
      {message !== undefined && <p className="text-sm text-destructive">{message}</p>}
      {card.error !== undefined && (
        <p className="text-sm text-destructive">
          {words.executeFailed(errorText(card.error))}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button
          size="sm"
          variant={destructive ? "destructive" : "default"}
          aria-disabled={blocked}
          aria-describedby={describedBy}
          onClick={() => {
            if (!blocked) onExecute();
          }}
        >
          {card.executing && <Spinner aria-hidden="true" />}
          {card.executing ? words.executing : label}
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

export function Collapsed({ children }: { children: string }) {
  return <p className="text-xs text-muted-foreground">{children}</p>;
}

export function Expired() {
  return (
    <p className="flex items-start gap-2 text-sm text-warning">
      <TriangleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      {t.assistant.card.expired}
    </p>
  );
}

export function Irreversible({ step, id }: { step: StepFields; id?: string }) {
  const destination = mergeDestination(step);
  return (
    <div id={id} className="flex flex-col gap-1">
      <p className="flex items-center gap-2 text-sm font-medium text-warning">
        <TriangleAlert aria-hidden="true" className="size-4 shrink-0" />
        {t.assistant.card.irreversible}
      </p>
      {step.impact !== undefined && destination !== undefined && (
        <p className="text-sm text-muted-foreground">
          {t.assistant.card.moves(step.impact.videos, destination)}
        </p>
      )}
    </div>
  );
}
