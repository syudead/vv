import { Film, Play, RotateCcw, X } from "lucide-react";

import { formatMonthDay, formatTime, t } from "@/i18n";
import { ListPage } from "@/ui/patterns/list-page";
import { PageHeader } from "@/ui/patterns/page-header";
import { Timeline, TimelineGroup, TimelineItem } from "@/ui/patterns/timeline";
import { Button } from "@/ui/shadcn/button";
import { Progress } from "@/ui/shadcn/progress";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";
import { VideoThumbnail, VideoThumbnailDuration } from "@/ui/VideoThumbnail";

import { sampleVideos } from "./list-page-example";

// 日ごとの時間軸の見本（registry:block timeline-example）。一覧ページの本体に Timeline を
// 置き、日の見出しの横に行（時刻・サムネイル・題名・フォルダ・位置・操作・行の端の ×）を
// 並べる。写した画面は見出しと行の中身を自分のものに差し替え、間隔は書かない
// （web/registry/rules/patterns.md の Sections）。

const rowsOf = [
  { time: [21, 30], position: ["16:05", 965, 2538], watched: false },
  { time: [15, 4], position: ["42:10", 2530, 2538], watched: true },
  { time: [9, 12], position: null, watched: false },
  { time: [22, 45], position: ["3:20", 200, 1200], watched: false },
  { time: [18, 3], position: null, watched: false },
] as const;

export function TimelineExample() {
  const p = t.designSystem.pattern;
  const now = new Date(2026, 8, 27, 23, 0);
  const rows = sampleVideos()
    .slice(0, rowsOf.length)
    .map((video, index) => {
      const sample = rowsOf[index] ?? rowsOf[0];
      const [hour, minute] = sample.time;
      return {
        ...video,
        time: formatTime(new Date(2026, 8, 27, hour, minute)),
        position: sample.position,
        watched: sample.watched,
      };
    });
  const groups = [
    {
      key: "today",
      label: p.today,
      date: formatMonthDay(new Date(2026, 8, 27), now),
      rows: rows.slice(0, 3),
    },
    {
      key: "yesterday",
      label: p.yesterday,
      date: formatMonthDay(new Date(2026, 8, 26), now),
      rows: rows.slice(3),
    },
  ];
  return (
    <ListPage header={<PageHeader title={p.history} />}>
      <Timeline label={p.historyEntries}>
        {groups.map((group) => (
          <TimelineGroup key={group.key} label={group.label} detail={group.date}>
            {group.rows.map((row) => (
              <TimelineItem
                key={row.id}
                time={row.time}
                media={
                  <VideoThumbnail className="rounded-md">
                    <div className="flex size-full items-center justify-center text-muted-foreground">
                      <Film aria-hidden="true" className="size-5" />
                    </div>
                    <VideoThumbnailDuration>{row.duration}</VideoThumbnailDuration>
                  </VideoThumbnail>
                }
                main={({ className, children }) => (
                  <a href="#timeline" className={className}>
                    {children}
                  </a>
                )}
                action={
                  row.position === null ? undefined : (
                    <Button asChild variant="outline" size="sm">
                      <a href="#timeline">
                        {row.watched ? (
                          <RotateCcw aria-hidden="true" />
                        ) : (
                          <Play aria-hidden="true" />
                        )}
                        {row.watched ? p.startOver : p.resume}
                      </a>
                    </Button>
                  )
                }
                remove={
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        className="text-muted-foreground"
                        aria-label={p.removeEntry}
                      >
                        <X aria-hidden="true" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{p.removeEntry}</TooltipContent>
                  </Tooltip>
                }
              >
                <span className="line-clamp-2 text-sm font-medium text-foreground">
                  {row.title}
                </span>
                <span className="truncate text-xs text-muted-foreground">
                  {p.folderPath}
                </span>
                {row.position !== null && (
                  <span className="flex items-center gap-2">
                    <Progress
                      value={row.watched ? row.position[2] : row.position[1]}
                      max={row.position[2]}
                      aria-label={p.watchedPortion}
                      className="max-w-xs flex-1"
                    />
                    <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                      {p.position(row.position[0], row.duration)}
                    </span>
                  </span>
                )}
              </TimelineItem>
            ))}
          </TimelineGroup>
        ))}
      </Timeline>
    </ListPage>
  );
}
