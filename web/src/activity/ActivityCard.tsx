import {
  CircleAlert,
  CircleCheck,
  Clock,
  ExternalLink,
  FileText,
  LoaderCircle,
  Settings,
  User,
  type LucideIcon,
} from "lucide-react";
import { Fragment } from "react";
import { Link } from "react-router";

import type { ActivityEntry } from "../api/activity";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { CardListItem } from "../ui/patterns/card-list";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Progress } from "../ui/shadcn/progress";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import {
  cardTone,
  entryResult,
  entryTime,
  type OpenAction,
  type ResultTone,
  statusMark,
  type StatusMark,
  subjectParts,
} from "./activityEntry";

// アクティビティの記録 1 件のカード（specs/055-activity/ui-design.md「Cards」）。

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

function Actions({ actions }: { actions: OpenAction[] }) {
  return (
    <>
      {actions.map((action) => {
        const Icon = actionIcon[action.id];
        return (
          <Tooltip key={action.id}>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon-sm" asChild>
                <Link to={action.to} aria-label={action.label}>
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

export interface ActivityCardProps {
  entry: ActivityEntry;
  now: Date;
  /** 対象のリンクから開いた再生画面の戻り先（今の画面の URL）。 */
  from: string;
  actions: OpenAction[];
}

/** ActivityCard は記録 1 件の 1 行のカードである。 */
export function ActivityCard({ entry, now, from, actions }: ActivityCardProps) {
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
      actions={<Actions actions={actions} />}
    />
  );
}
