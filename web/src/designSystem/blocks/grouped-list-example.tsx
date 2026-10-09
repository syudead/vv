import { Film, X } from "lucide-react";

import { formatTime, t } from "@/i18n";
import {
  GroupedList,
  GroupedListGroup,
  GroupedListItem,
} from "@/ui/patterns/grouped-list";
import { ListPage } from "@/ui/patterns/list-page";
import { PageHeader } from "@/ui/patterns/page-header";
import { Button } from "@/ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";
import { VideoThumbnail, VideoThumbnailDuration } from "@/ui/VideoThumbnail";

import { sampleVideos } from "./list-page-example";

// 見出しつきのまとまりの一覧の見本（registry:block grouped-list-example）。一覧ページの
// 本体に GroupedList を置き、日の見出しの下に行（サムネイル・題名・時刻・行の端の ×）を
// 並べる。写した画面は見出しと行の中身を自分のものに差し替え、間隔は書かない
// （web/registry/rules/patterns.md の Sections）。

const times = [
  [21, 30],
  [15, 4],
  [9, 12],
  [22, 45],
  [18, 3],
] as const;

export function GroupedListExample() {
  const p = t.designSystem.pattern;
  const rows = sampleVideos()
    .slice(0, times.length)
    .map((video, index) => {
      const [hour, minute] = times[index] ?? [0, 0];
      return { ...video, time: formatTime(new Date(2026, 8, 27, hour, minute)) };
    });
  const groups = [
    { key: "today", heading: p.today, rows: rows.slice(0, 3) },
    { key: "yesterday", heading: p.yesterday, rows: rows.slice(3) },
  ];
  return (
    <ListPage header={<PageHeader title={p.history} />}>
      <GroupedList label={p.historyEntries}>
        {groups.map((group) => (
          <GroupedListGroup key={group.key} heading={group.heading}>
            {group.rows.map((row) => (
              <GroupedListItem key={row.id}>
                <a
                  href="#grouped-list"
                  className="-m-1 flex min-w-0 flex-1 items-center gap-3 rounded-md p-1 transition-colors hover:bg-accent"
                >
                  <VideoThumbnail className="w-list-thumb shrink-0 rounded-sm">
                    <div className="flex size-full items-center justify-center text-muted-foreground">
                      <Film aria-hidden="true" className="size-5" />
                    </div>
                    <VideoThumbnailDuration>{row.duration}</VideoThumbnailDuration>
                  </VideoThumbnail>
                  <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="line-clamp-2 text-sm font-medium text-foreground">
                      {row.title}
                    </span>
                    <span className="text-xs text-muted-foreground tabular-nums">
                      {row.time}
                    </span>
                  </span>
                </a>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="shrink-0 text-muted-foreground"
                      aria-label={p.removeEntry}
                    >
                      <X aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{p.removeEntry}</TooltipContent>
                </Tooltip>
              </GroupedListItem>
            ))}
          </GroupedListGroup>
        ))}
      </GroupedList>
    </ListPage>
  );
}
