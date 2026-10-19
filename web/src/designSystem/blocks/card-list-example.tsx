import {
  CircleAlert,
  CircleCheck,
  Clock,
  ExternalLink,
  LoaderCircle,
} from "lucide-react";

import { t } from "@/i18n";
import { useState } from "react";

import {
  CardList,
  CardListItem,
  CardListMember,
  type CardListTone,
} from "@/ui/patterns/card-list";
import { ListPage } from "@/ui/patterns/list-page";
import { PageHeader } from "@/ui/patterns/page-header";
import { Badge } from "@/ui/shadcn/badge";
import { Button } from "@/ui/shadcn/button";
import { Progress } from "@/ui/shadcn/progress";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";

import { sampleVideos } from "./list-page-example";

// registry:block card-list-example. Replace card content while retaining the shared spacing, surfaces and columns.

/** A card with members: one run whose parts are rows inside its border. */
function PipelineCardExample({ title }: { title: string }) {
  const a = t.activity;
  const [expanded, setExpanded] = useState(true);
  return (
    <CardListItem
      tone="primary"
      icon={
        <LoaderCircle
          role="img"
          aria-label={a.statusIcon.running}
          className="size-5 animate-spin text-primary"
        />
      }
      title={title}
      badge={<Badge variant="secondary">{a.kind.pipeline}</Badge>}
      detail={
        <span className="min-w-0 truncate">
          {a.pipeline.processing(a.kind.auto_tag, 1, 2)}
        </span>
      }
      time={a.time.justNow}
      timeShort={a.time.shortNow}
      expanded={expanded}
      onExpandedChange={setExpanded}
      toggleLabel={
        expanded
          ? { tooltip: a.pipeline.hideTasks, name: a.pipeline.hideTasksFor(title) }
          : { tooltip: a.pipeline.showTasks, name: a.pipeline.showTasksFor(title) }
      }
      members={
        <>
          <CardListMember
            icon={
              <CircleCheck
                role="img"
                aria-label={a.statusIcon.done}
                className="size-4 text-success"
              />
            }
            title={a.kind.import}
            detail={<span className="min-w-0 truncate">{a.result.done}</span>}
          />
          <CardListMember
            icon={
              <LoaderCircle
                role="img"
                aria-label={a.statusIcon.running}
                className="size-4 animate-spin text-primary"
              />
            }
            title={a.kind.auto_tag}
            detail={
              <>
                <Badge variant="soft">{a.pipeline.matches}</Badge>
                <span className="min-w-0 truncate">{a.result.running}</span>
              </>
            }
          />
        </>
      }
    />
  );
}

export function CardListExample() {
  const a = t.activity;
  const [first, second, third, fourth] = sampleVideos();
  const rows: {
    key: string;
    tone: CardListTone;
    icon: React.ReactNode;
    title: string;
    kind: string;
    detail: React.ReactNode;
    time: string;
    timeShort: string;
    open: boolean;
  }[] = [
    {
      key: "running",
      tone: "primary",
      icon: (
        <LoaderCircle
          role="img"
          aria-label={a.statusIcon.running}
          className="size-5 animate-spin text-primary"
        />
      ),
      title: first?.title ?? "",
      kind: a.kind.import,
      detail: (
        <>
          <span className="min-w-0 truncate">{a.stage.preview}</span>
          <span className="w-activity-bar shrink-0 max-lg:hidden">
            <Progress value={3} max={5} aria-label={a.stage.preview} />
          </span>
          <span className="shrink-0 text-primary tabular-nums">
            {a.result.progress(3, 5)}
          </span>
        </>
      ),
      time: a.time.justNow,
      timeShort: a.time.shortNow,
      open: true,
    },
    {
      key: "failed",
      tone: "destructive",
      icon: (
        <CircleAlert
          role="img"
          aria-label={a.statusIcon.attention}
          className="size-5 text-destructive"
        />
      ),
      title: second?.title ?? "",
      kind: a.kind.import,
      detail: (
        <span className="min-w-0 truncate text-destructive">
          {a.result.importFailed(a.stage.thumbnail, "")}
        </span>
      ),
      time: a.time.minutes(12),
      timeShort: a.time.shortMinutes(12),
      open: true,
    },
    {
      key: "warning",
      tone: "warning",
      icon: (
        <CircleAlert
          role="img"
          aria-label={a.statusIcon.attention}
          className="size-5 text-warning"
        />
      ),
      title: a.subject.pair(third?.title ?? "", fourth?.title ?? ""),
      kind: a.kind.version_judgement,
      detail: (
        <span className="min-w-0 truncate text-warning">{a.result.noConnection}</span>
      ),
      time: a.time.hours(2),
      timeShort: a.time.shortHours(2),
      open: false,
    },
    {
      key: "waiting",
      tone: "default",
      icon: (
        <Clock
          role="img"
          aria-label={a.statusIcon.waiting}
          className="size-5 text-muted-foreground"
        />
      ),
      title: fourth?.title ?? "",
      kind: a.kind.auto_tag,
      detail: <span className="min-w-0 truncate">{a.result.waiting}</span>,
      time: a.time.hours(3),
      timeShort: a.time.shortHours(3),
      open: false,
    },
    {
      key: "done",
      tone: "default",
      icon: (
        <CircleCheck
          role="img"
          aria-label={a.statusIcon.done}
          className="size-5 text-success"
        />
      ),
      title: third?.title ?? "",
      kind: a.kind.import,
      detail: <span className="min-w-0 truncate">{a.result.done}</span>,
      time: a.time.hours(5),
      timeShort: a.time.shortHours(5),
      open: true,
    },
  ];
  return (
    <ListPage header={<PageHeader title={a.title} />}>
      <CardList label={a.list}>
        <PipelineCardExample title={first?.title ?? ""} />
        {rows.map((row) => (
          <CardListItem
            key={row.key}
            tone={row.tone}
            icon={row.icon}
            title={row.title}
            badge={<Badge variant="secondary">{row.kind}</Badge>}
            detail={row.detail}
            time={row.time}
            timeShort={row.timeShort}
            actions={
              row.open && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="icon-sm" aria-label={a.action.open}>
                      <ExternalLink aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{a.action.open}</TooltipContent>
                </Tooltip>
              )
            }
          />
        ))}
      </CardList>
    </ListPage>
  );
}
