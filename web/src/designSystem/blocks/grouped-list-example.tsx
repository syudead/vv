import { Film, X } from "lucide-react";

import { formatMonthDay, t } from "@/i18n";
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

// registry:block grouped-list-example. Replace headings and row content while retaining shared spacing and surfaces.

export function GroupedListExample() {
  const p = t.designSystem.pattern;
  const now = new Date(2026, 8, 27, 23, 0);
  const rows = sampleVideos().slice(0, 5);
  const groups = [
    {
      key: "today",
      heading: p.today,
      detail: formatMonthDay(new Date(2026, 8, 27), now),
      rows: rows.slice(0, 3),
    },
    {
      key: "yesterday",
      heading: p.yesterday,
      detail: formatMonthDay(new Date(2026, 8, 26), now),
      rows: rows.slice(3),
    },
  ];
  return (
    <ListPage header={<PageHeader title={p.history} />}>
      <GroupedList label={p.historyEntries}>
        {groups.map((group) => (
          <GroupedListGroup key={group.key} heading={group.heading} detail={group.detail}>
            {group.rows.map((row) => (
              <GroupedListItem key={row.id}>
                <a
                  href="#grouped-list"
                  className="flex min-w-0 flex-1 items-center gap-3 rounded-md transition-colors hover:bg-accent"
                >
                  <VideoThumbnail className="w-list-thumb shrink-0 rounded-sm">
                    <div className="flex size-full items-center justify-center text-muted-foreground">
                      <Film aria-hidden="true" className="size-5" />
                    </div>
                    <VideoThumbnailDuration>{row.duration}</VideoThumbnailDuration>
                  </VideoThumbnail>
                  <span className="line-clamp-2 min-w-0 flex-1 text-sm font-medium text-foreground">
                    {row.title}
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
