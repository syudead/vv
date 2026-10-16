import { Film, FolderSearch } from "lucide-react";

import { t } from "@/i18n";
import { ListPage } from "@/ui/patterns/list-page";
import { PageHeader } from "@/ui/patterns/page-header";
import { ShelfRow, ShelfRows } from "@/ui/patterns/shelf-row";
import { Button } from "@/ui/shadcn/button";
import { VideoThumbnail, VideoThumbnailDuration } from "@/ui/VideoThumbnail";

import { sampleVideos } from "./list-page-example";

// 棚の行の見本（registry:block shelf-row-example）。一覧ページの本体に ShelfRows を置き、
// 棚ごとに ShelfRow を並べる。フォルダの棚は見出しがリンクで FolderSearch の印を前に置き、
// 決まった棚の見出しは文字だけにする。どちらも右端に「See all」（link の sm）を置き、行には
// card-0 のカードを並べる。2 つ目の棚は項目を読んでいる間の形である。写した画面は棚とカードを
// 自分のものに差し替え、間隔・幅・送りのボタンは書かない（web/registry/rules/patterns.md の
// Sections）。

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
          title={p.recentlyAdded}
          actions={<SeeAll name={p.recentlyAdded} />}
          previousLabel={p.scrollBack(p.recentlyAdded)}
          nextLabel={p.scrollForward(p.recentlyAdded)}
          loading={p.loadingShelf(p.recentlyAdded)}
        />
      </ShelfRows>
    </ListPage>
  );
}
