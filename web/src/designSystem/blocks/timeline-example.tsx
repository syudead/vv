import { Film, Play, RotateCcw, Search, Trash2, X } from "lucide-react";
import { useState } from "react";

import { formatMonth, formatMonthDay, formatTime, formatWeekdayDate, t } from "@/i18n";
import { JumpList } from "@/ui/patterns/jump-list";
import { ListPage } from "@/ui/patterns/list-page";
import { PageHeader } from "@/ui/patterns/page-header";
import { Timeline, TimelineGroup, TimelineItem } from "@/ui/patterns/timeline";
import { Toolbar } from "@/ui/patterns/toolbar";
import { Button } from "@/ui/shadcn/button";
import { Input } from "@/ui/shadcn/input";
import { Progress } from "@/ui/shadcn/progress";
import { ToggleGroup, ToggleGroupItem } from "@/ui/shadcn/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";
import { VideoThumbnail, VideoThumbnailDuration } from "@/ui/VideoThumbnail";

import { sampleVideos } from "./list-page-example";

// 日ごとの時間軸の見本（registry:block timeline-example）。一覧ページの本体に Timeline を
// 置き、日の見出しの横に行（時刻・サムネイル・題名・フォルダ・位置・操作・行の端の ×）を
// 並べる。見出しの行（toolbarRow="header"）に状態の切り替えと行の端の検索を、横の欄（aside）
// に日付へ移る JumpList と最後の区切りの後ろの ghost-destructive の操作を置く。写した画面は
// 見出しと行の中身を自分のものに差し替え、間隔は書かない（web/registry/rules/patterns.md の
// Sections）。

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
  const [watch, setWatch] = useState("all");
  const [date, setDate] = useState<string | null>(null);
  const jumpDays = [
    { value: "2026-09-27", label: p.today },
    { value: "2026-09-26", label: p.yesterday },
    { value: "2026-09-21", label: formatWeekdayDate(new Date(2026, 8, 21), now) },
  ];
  const jumpMonths = [
    { value: "2026-08", label: formatMonth(new Date(2026, 7, 1), now) },
    { value: "2025-12", label: formatMonth(new Date(2025, 11, 1), now) },
  ];
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
    <ListPage
      toolbarRow="header"
      header={<PageHeader title={p.history} />}
      toolbar={
        <Toolbar
          searchPlacement="end"
          search={
            <div className="relative flex w-full items-center">
              <Search
                aria-hidden="true"
                className="pointer-events-none absolute left-2 size-4 text-muted-foreground"
              />
              <Input
                type="search"
                aria-label={p.searchTitles}
                placeholder={p.searchTitles}
                className="h-8 pl-8"
              />
            </div>
          }
        >
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            aria-label={p.watchStatus}
            value={watch}
            onValueChange={(value) => {
              if (value !== "") setWatch(value);
            }}
          >
            <ToggleGroupItem value="all" className="flex-none px-3">
              {p.watchAll}
            </ToggleGroupItem>
            <ToggleGroupItem value="inProgress" className="flex-none px-3">
              {p.watchInProgress}
            </ToggleGroupItem>
            <ToggleGroupItem value="watched" className="flex-none px-3">
              {p.watchWatched}
            </ToggleGroupItem>
          </ToggleGroup>
        </Toolbar>
      }
      aside={
        <JumpList
          title={p.jumpToDate}
          groups={[jumpDays, jumpMonths]}
          value={date}
          onValueChange={setDate}
          action={
            <Button variant="ghost-destructive" size="sm" className="justify-start">
              <Trash2 aria-hidden="true" />
              {p.clearHistory}
            </Button>
          }
        />
      }
    >
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
