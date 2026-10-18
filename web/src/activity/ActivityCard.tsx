import {
  CircleAlert,
  CircleCheck,
  Clock,
  ExternalLink,
  FileText,
  LoaderCircle,
  RotateCw,
  Settings,
  Undo2,
  User,
  type LucideIcon,
} from "lucide-react";
import { type ComponentProps, Fragment, useState } from "react";
import { Link } from "react-router";

import type { ActivityEntry, ActivityTag } from "../api/activity";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { CardListItem } from "../ui/patterns/card-list";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Progress } from "../ui/shadcn/progress";
import { Spinner } from "../ui/shadcn/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import {
  cardTone,
  entryOperations,
  entryResult,
  entryTime,
  type OpenAction,
  type ResultTone,
  statusMark,
  type StatusMark,
  subjectParts,
} from "./activityEntry";

const markIcon: Record<StatusMark, { icon: LucideIcon; className: string }> = {
  running: { icon: LoaderCircle, className: "animate-spin text-primary" },
  waiting: { icon: Clock, className: "text-muted-foreground" },
  attention: { icon: CircleAlert, className: "text-destructive" },
  done: { icon: CircleCheck, className: "text-success" },
  doneByYou: { icon: User, className: "text-muted-foreground" },
};

const resultClass: Record<ResultTone, string> = {
  muted: "text-muted-foreground",
  destructive: "text-destructive",
  warning: "text-warning",
};

const actionIcon: Record<OpenAction["id"], LucideIcon> = {
  open: ExternalLink,
  duplicates: ExternalLink,
  workInfo: FileText,
  settings: Settings,
};

function StatusIcon({ entry }: { entry: ActivityEntry }) {
  const mark = statusMark(entry);
  const { icon: Icon, className } = markIcon[mark];
  const warning = cardTone(entry) === "warning";
  return (
    <Icon
      role="img"
      aria-label={t.activity.statusIcon[mark]}
      className={cn("size-5", warning ? "text-warning" : className)}
    />
  );
}

function Subject({ entry, from }: { entry: ActivityEntry; from: string }) {
  const parts = subjectParts(entry);
  return (
    <>
      {parts.map((part, index) => (
        <Fragment key={index}>
          {index > 0 && <span aria-hidden="true"> ↔ </span>}
          {part.videoId === undefined ? (
            <span>{part.label}</span>
          ) : (
            <Link
              to={`/videos/${String(part.videoId)}`}
              state={{ from }}
              className="hover:underline"
            >
              {part.label}
            </Link>
          )}
        </Fragment>
      ))}
    </>
  );
}

function Result({ entry, now }: { entry: ActivityEntry; now: Date }) {
  const progress = entry.progress;
  const result = entryResult(entry, now);
  return (
    <>
      <span className={cn("min-w-0 truncate", resultClass[result.tone])}>
        {result.parts.map((part, index) => (
          <Fragment key={index}>
            {index > 0 && " "}
            {part.strong === true ? (
              <span className="text-foreground">{part.text}</span>
            ) : (
              part.text
            )}
          </Fragment>
        ))}
      </span>
      {entry.status === "running" && progress !== undefined && (
        <>
          <span className="w-activity-bar shrink-0 max-lg:hidden">
            <Progress
              value={progress.done}
              max={progress.total}
              aria-label={t.activity.stage[progress.stage]}
            />
          </span>
          <span className="shrink-0 text-primary tabular-nums">
            {t.activity.result.progress(progress.done, progress.total)}
          </span>
        </>
      )}
    </>
  );
}

/** Disable all card actions while an operation is pending. */
type Pending = "retry" | "undo" | null;

function OpenLinks({ actions, busy }: { actions: OpenAction[]; busy: boolean }) {
  return (
    <>
      {actions.map((action) => {
        const Icon = actionIcon[action.id];
        return (
          <Tooltip key={action.id}>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                className={cn(busy && "pointer-events-none opacity-50")}
                asChild
              >
                <Link
                  to={action.to}
                  aria-label={action.label}
                  aria-disabled={busy || undefined}
                  tabIndex={busy ? -1 : undefined}
                >
                  <Icon aria-hidden="true" />
                </Link>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{action.label}</TooltipContent>
          </Tooltip>
        );
      })}
    </>
  );
}

type OperationButtonProps = Omit<ComponentProps<"button">, "children"> & {
  label: string;
  icon: LucideIcon;
  pending: boolean;
};

/** Forward trigger attributes and refs to the button for Tooltip and DropdownMenu. */
function OperationButton({ label, icon: Icon, pending, ...props }: OperationButtonProps) {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={label}
      aria-busy={pending || undefined}
      {...props}
    >
      {pending ? <Spinner aria-hidden="true" /> : <Icon aria-hidden="true" />}
    </Button>
  );
}

function UndoTagsMenu({
  tags,
  pending,
  disabled,
  onUndo,
}: {
  tags: ActivityTag[];
  pending: boolean;
  disabled: boolean;
  onUndo: (tagId: number) => void;
}) {
  const label = t.activity.action.undoTags;
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild disabled={disabled}>
            <OperationButton label={label} icon={Undo2} pending={pending} />
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">
        {tags.map((tag) => (
          <DropdownMenuItem key={tag.id} onSelect={() => onUndo(tag.id)}>
            <span className="min-w-0 truncate">{tag.name}</span>
            {tag.suggested && (
              <span className="text-muted-foreground">{t.activity.result.suggested}</span>
            )}
            <span className="ml-auto text-muted-foreground tabular-nums">
              {t.activity.percent(tag.probability)}
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

interface ActionsProps {
  entry: ActivityEntry;
  actions: OpenAction[];
  onRetry: (entry: ActivityEntry) => Promise<void>;
  onUndo: (entry: ActivityEntry, tagId?: number) => Promise<void>;
}

/** Keep actions disabled until the refreshed card arrives to prevent duplicate requests. */
function Actions({ entry, actions, onRetry, onUndo }: ActionsProps) {
  const [pending, setPending] = useState<Pending>(null);
  const operations = entryOperations(entry);
  const busy = pending !== null;
  const run = (which: Exclude<Pending, null>, send: () => Promise<void>) => {
    setPending(which);
    void send().finally(() => setPending(null));
  };
  const simple = (
    which: Exclude<Pending, null>,
    label: string,
    icon: LucideIcon,
    send: () => Promise<void>,
  ) => (
    <Tooltip>
      <TooltipTrigger asChild>
        <OperationButton
          label={label}
          icon={icon}
          pending={pending === which}
          disabled={busy}
          onClick={() => run(which, send)}
        />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
  return (
    <>
      {operations.retry &&
        simple("retry", t.activity.action.retry, RotateCw, () => onRetry(entry))}
      {operations.undo &&
        simple("undo", t.activity.action.undo, Undo2, () => onUndo(entry))}
      {operations.undoTags.length > 0 && (
        <UndoTagsMenu
          tags={operations.undoTags}
          pending={pending === "undo"}
          disabled={busy}
          onUndo={(tagId) => run("undo", () => onUndo(entry, tagId))}
        />
      )}
      <OpenLinks actions={actions} busy={busy} />
    </>
  );
}

export interface ActivityCardProps {
  entry: ActivityEntry;
  now: Date;
  /** Return URL for playback opened from this card. */
  from: string;
  actions: OpenAction[];
  /** The caller reports failures; this promise never rejects. */
  onRetry: (entry: ActivityEntry) => Promise<void>;
  /** For auto_tag, pass the tag to undo; the caller reports failures. */
  onUndo: (entry: ActivityEntry, tagId?: number) => Promise<void>;
}

export function ActivityCard({
  entry,
  now,
  from,
  actions,
  onRetry,
  onUndo,
}: ActivityCardProps) {
  const time = entryTime(entry.createdAt, now);
  return (
    <CardListItem
      tone={cardTone(entry)}
      icon={<StatusIcon entry={entry} />}
      title={<Subject entry={entry} from={from} />}
      badge={<Badge variant="secondary">{t.activity.kind[entry.kind]}</Badge>}
      detail={<Result entry={entry} now={now} />}
      time={<span title={time.full}>{time.long}</span>}
      timeShort={<span title={time.full}>{time.short}</span>}
      actions={
        <Actions entry={entry} actions={actions} onRetry={onRetry} onUndo={onUndo} />
      }
    />
  );
}
