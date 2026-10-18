import { Film, FolderSearch } from "lucide-react";

import { t } from "@/i18n";
import { ListPage } from "@/ui/patterns/list-page";
import { PageHeader } from "@/ui/patterns/page-header";
import { ShelfRow, ShelfRows } from "@/ui/patterns/shelf-row";
import { Button } from "@/ui/shadcn/button";
import { VideoThumbnail, VideoThumbnailDuration } from "@/ui/VideoThumbnail";

import { sampleVideos } from "./list-page-example";

// ShelfRows contains linked smart-folder headings and cards; the second row shows loading.

/** SampleShelfCard は見本の棚のカードである。写した画面は自分のカードに差し替える。 */
function SampleShelfCard({ title, duration }: { title: string; duration: string }) {
  return (
    <a href="#shelf-row" className="flex min-w-0 flex-col gap-2 rounded-md">
      <VideoThumbnail>
        <div className="flex size-full items-center justify-center text-muted-foreground">
          <Film aria-hidden="true" className="size-6" />
        </div>
        <VideoThumbnailDuration>{duration}</VideoThumbnailDuration>
      </VideoThumbnail>
      <span className="line-clamp-2 text-sm font-medium text-foreground">{title}</span>
    </a>
  );
}

/** SeeAll は棚の見出しの右端の「See all」である。 */
function SeeAll({ name }: { name: string }) {
  const p = t.designSystem.pattern;
  return (
    <Button variant="link" size="sm" asChild>
      <a href="#shelf-row" aria-label={p.seeAllFor(name)}>
        {p.seeAll}
      </a>
    </Button>
  );
}

export function ShelfRowExample() {
  const p = t.designSystem.pattern;
  const videos = sampleVideos();
  const travel = p.tagNames.travel;
  const night = p.tagNames.night;
  return (
    <ListPage header={<PageHeader title={p.home} titleHidden />}>
      <ShelfRows label={p.shelves}>
        <ShelfRow
          title={travel}
          leading={<FolderSearch aria-hidden="true" />}
          href="#shelf-row"
          actions={<SeeAll name={travel} />}
          previousLabel={p.scrollBack(travel)}
          nextLabel={p.scrollForward(travel)}
        >
          {videos.map((video) => (
            <SampleShelfCard
              key={video.id}
              title={video.title}
              duration={video.duration}
            />
          ))}
        </ShelfRow>
        <ShelfRow
          title={night}
          leading={<FolderSearch aria-hidden="true" />}
          href="#shelf-row"
          actions={<SeeAll name={night} />}
          previousLabel={p.scrollBack(night)}
          nextLabel={p.scrollForward(night)}
          loading={p.loadingShelf(night)}
        />
      </ShelfRows>
    </ListPage>
  );
}
