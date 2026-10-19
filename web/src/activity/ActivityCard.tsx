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
  UserRound,
  type LucideIcon,
} from "lucide-react";
import { type ComponentProps, Fragment, type ReactNode, useState } from "react";
import { Link } from "react-router";

import type {
  ActivityEntry,
  ActivityFaceLink,
  ActivityProgress,
  ActivityTag,
  ActivityUndoRequest,
} from "../api/activity";
import { personImagePath } from "../api/credits";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { PersonFace } from "../ui/PersonFace";
import { CardListItem } from "../ui/patterns/card-list";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Progress } from "../ui/shadcn/progress";
import { Spinner } from "../ui/shadcn/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import {
  cardTone,
  type EntryFields,
  type EntryResult,
  entryOperations,
  entryResult,
  entryTime,
  faceLinkName,
  type OpenAction,
  type ResultTone,
  statusMark,
  type StatusMark,
  subjectParts,
  subjectText,
} from "./activityEntry";
import type { OperationTarget } from "./useActivityOperations";

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
  person: UserRound,
};

export interface MarkIconProps {
  mark: StatusMark;
  warning?: boolean;
  /** `size-4` for a row inside a section; `size-5` on a card. */
  small?: boolean;
}

export function MarkIcon({ mark, warning = false, small = false }: MarkIconProps) {
  const { icon: Icon, className } = markIcon[mark];
  return (
    <Icon
      role="img"
      aria-label={t.activity.statusIcon[mark]}
      className={cn(
        "shrink-0",
        small ? "size-4" : "size-5",
        warning ? "text-warning" : className,
      )}
    />
  );
}

export function StatusIcon({ entry, small }: { entry: EntryFields; small?: boolean }) {
  return (
    <MarkIcon
      mark={statusMark(entry)}
      warning={cardTone(entry) === "warning"}
      small={small}
    />
  );
}

/** The card's title opens the entry's detail; the video opens from `Open` only. */
export function Subject({ entry, from }: { entry: ActivityEntry; from: string }) {
  const parts = subjectParts(entry);
  return (
    <Link
      to={`/activity/${String(entry.id)}`}
      state={{ from }}
      aria-label={t.activity.detailLink(t.activity.kind[entry.kind], subjectText(entry))}
      className="hover:underline"
    >
      {parts.map((part, index) => (
        <Fragment key={index}>
          {index > 0 && <span aria-hidden="true"> ↔ </span>}
          <span>{part.label}</span>
        </Fragment>
      ))}
    </Link>
  );
}

/** Truncated on a card; `wrap` shows the phrase in full, as the detail page does. */
export function ResultText({
  result,
  wrap = false,
}: {
  result: EntryResult;
  wrap?: boolean;
}) {
  return (
    <span
      className={cn(
        "min-w-0",
        wrap ? "wrap-anywhere" : "truncate",
        resultClass[result.tone],
      )}
    >
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
  );
}

/** The result phrase, then a running import's bar and count; the bar hides below `lg`. */
export function ResultWithProgress({
  result,
  progress,
}: {
  result: EntryResult;
  progress: ActivityProgress | undefined;
}) {
  return (
    <>
      <ResultText result={result} />
      {progress !== undefined && (
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

function Result({ entry, now }: { entry: ActivityEntry; now: Date }) {
  return (
    <ResultWithProgress
      result={entryResult(entry, now)}
      progress={entry.status === "running" ? entry.progress : undefined}
    />
  );
}

/** Disable all card actions while an operation is pending. */
type Pending = "retry" | "undo" | null;

function OpenLinks({
  actions,
  busy,
  from,
}: {
  actions: OpenAction[];
  busy: boolean;
  from: string;
}) {
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
                  state={{ from }}
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
export function OperationButton({
  label,
  icon: Icon,
  pending,
  ...props
}: OperationButtonProps) {
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

/** A face link's person as a `PersonFace chip`; a deleted person shows the placeholder. */
export function FaceLinkFace({ link }: { link: ActivityFaceLink }) {
  return (
    <PersonFace
      shape="chip"
      name={faceLinkName(link)}
      src={link.person === undefined ? undefined : personImagePath(link.person.id)}
    />
  );
}

/** `Unnamed person` and a deleted person read quieter than a name. */
export function FaceLinkName({ link }: { link: ActivityFaceLink }) {
  return (
    <span
      className={cn(
        "min-w-0 truncate",
        link.person?.provisional !== false && "text-muted-foreground",
      )}
    >
      {faceLinkName(link)}
    </span>
  );
}

interface UndoMenuProps {
  label: string;
  pending: boolean;
  disabled: boolean;
  children: ReactNode;
}

function UndoMenu({ label, pending, disabled, children }: UndoMenuProps) {
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
      <DropdownMenuContent align="end">{children}</DropdownMenuContent>
    </DropdownMenu>
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
  return (
    <UndoMenu label={t.activity.action.undoTags} pending={pending} disabled={disabled}>
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
    </UndoMenu>
  );
}

/** One item per link not yet undone, then `All links`, which undoes every one of them. */
function UndoLinksMenu({
  links,
  pending,
  disabled,
  onUndo,
}: {
  links: ActivityFaceLink[];
  pending: boolean;
  disabled: boolean;
  onUndo: (part: ActivityUndoRequest) => void;
}) {
  return (
    <UndoMenu label={t.activity.action.undoLinks} pending={pending} disabled={disabled}>
      {links.map((link) => (
        <DropdownMenuItem
          key={link.appearanceId}
          onSelect={() => onUndo({ appearanceId: link.appearanceId })}
        >
          <FaceLinkFace link={link} />
          <FaceLinkName link={link} />
        </DropdownMenuItem>
      ))}
      <DropdownMenuSeparator />
      <DropdownMenuItem onSelect={() => onUndo({})}>
        {t.activity.action.allLinks}
      </DropdownMenuItem>
    </UndoMenu>
  );
}

export interface ActionsProps {
  entry: EntryFields;
  actions: OpenAction[];
  from: string;
  onRetry: (entry: OperationTarget) => Promise<void>;
  onUndo: (entry: OperationTarget, part?: ActivityUndoRequest) => Promise<void>;
}

/** Keep actions disabled until the refreshed card arrives to prevent duplicate requests. */
export function Actions({ entry, actions, from, onRetry, onUndo }: ActionsProps) {
  const [pending, setPending] = useState<Pending>(null);
  const operations = entryOperations(entry);
  const [onlyLink, ...otherLinks] = operations.undoLinks;
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
          onUndo={(tagId) => run("undo", () => onUndo(entry, { tagId }))}
        />
      )}
      {onlyLink !== undefined &&
        otherLinks.length === 0 &&
        simple("undo", t.activity.action.undo, Undo2, () =>
          onUndo(entry, { appearanceId: onlyLink.appearanceId }),
        )}
      {otherLinks.length > 0 && (
        <UndoLinksMenu
          links={operations.undoLinks}
          pending={pending === "undo"}
          disabled={busy}
          onUndo={(part) => run("undo", () => onUndo(entry, part))}
        />
      )}
      <OpenLinks actions={actions} busy={busy} from={from} />
    </>
  );
}

export interface ActivityCardProps {
  entry: ActivityEntry;
  now: Date;
  /** Return URL for the detail and the pages opened from this card. */
  from: string;
  actions: OpenAction[];
  /** The caller reports failures; this promise never rejects. */
  onRetry: (entry: OperationTarget) => Promise<void>;
  /** For auto_tag and face_analysis, pass the tag or link to undo; the caller reports failures. */
  onUndo: (entry: OperationTarget, part?: ActivityUndoRequest) => Promise<void>;
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
        <Actions
          entry={entry}
          actions={actions}
          from={from}
          onRetry={onRetry}
          onUndo={onUndo}
        />
      }
    />
  );
}
