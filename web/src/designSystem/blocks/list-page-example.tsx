import {
  EyeOff,
  Film,
  LayoutGrid,
  List,
  Search,
  SlidersHorizontal,
  Tag,
} from "lucide-react";
import { type ReactNode, useState } from "react";

import { t } from "@/i18n";
import {
  CardGrid,
  type CardSize,
  cardFrameClass,
  cardThumbnailClass,
} from "@/ui/patterns/card-grid";
import { ListPage } from "@/ui/patterns/list-page";
import { LoadMoreRow } from "@/ui/patterns/load-more-row";
import { PageHeader } from "@/ui/patterns/page-header";
import { SelectionBar } from "@/ui/patterns/selection-bar";
import { Toolbar } from "@/ui/patterns/toolbar";
import { Button } from "@/ui/shadcn/button";
import { Checkbox } from "@/ui/shadcn/checkbox";
import { Input } from "@/ui/shadcn/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/ui/shadcn/select";
import { Slider } from "@/ui/shadcn/slider";
import { ToggleGroup, ToggleGroupItem } from "@/ui/shadcn/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailMark,
} from "@/ui/VideoThumbnail";

// 一覧ページの型の見本（registry:block list-page-example）。トップバーの中のツールバー・
// 見出し行・カードのグリッド・末尾の追加読み込み・選択バーを、見本の動画で埋めて動かす。
// 見本には shell が無いので、トップバーの代わりの帯（ExampleTopBar）にツールバーを置く。
// 写した画面は帯の代わりに shell の TopBarPortal でツールバーを包み、見本のデータと文言を
// 自分のものに、カードを自分のカードに差し替える（web/registry/rules/patterns.md の List page）。

const durations = ["49:10", "3:12", "12:48", "27:05", "1:02:33", "8:40", "15:20", "4:55"];

/** sampleVideos は見本の動画である。 */
export function sampleVideos() {
  const p = t.designSystem.pattern;
  return Object.entries(p.videoTitles).map(([id, title], index) => ({
    id,
    title,
    meta: p.videoMeta(durations[index] ?? "", p.addedAgo(index + 1)),
    duration: durations[index] ?? "",
  }));
}

/** SampleVideoCard は見本のカードである。写した画面は自分のカードに差し替える。 */
export function SampleVideoCard({
  title,
  meta,
  duration,
  selected,
  onSelectedChange,
}: {
  title: string;
  meta: string;
  duration: string;
  selected: boolean;
  onSelectedChange: (selected: boolean) => void;
}) {
  const p = t.designSystem.pattern;
  return (
    <article className={cardFrameClass(selected)}>
      <VideoThumbnail className={cardThumbnailClass}>
        <div className="flex size-full items-center justify-center text-muted-foreground">
          <Film aria-hidden="true" className="size-6" />
        </div>
        <VideoThumbnailDuration>{duration}</VideoThumbnailDuration>
        <VideoThumbnailMark corner="top-start">
          <Checkbox
            checked={selected}
            onCheckedChange={(checked) => {
              onSelectedChange(checked === true);
            }}
            aria-label={p.select(title)}
          />
        </VideoThumbnailMark>
      </VideoThumbnail>
      <div className="flex min-w-0 flex-col gap-1 px-3 pt-2 pb-3">
        <h2 className="truncate text-sm font-semibold sm:text-base">{title}</h2>
        <p className="truncate text-xs text-muted-foreground">{meta}</p>
      </div>
    </article>
  );
}

/**
 * ExampleTopBar は見本の中でトップバーの代わりをする帯である。画面では使わず、ツールバーを
 * shell の TopBarPortal で包む。
 */
export function ExampleTopBar({ children }: { children: ReactNode }) {
  return (
    <div className="-mx-3 -mt-4 flex h-navbar items-center border-b border-border bg-background px-2 sm:-mx-4 sm:px-3">
      {children}
    </div>
  );
}

/** ExampleToolbar は見本のツールバー（トップバーの置き場）である。状態の見本でも同じものを使う。 */
export function ExampleToolbar({
  size = 1,
  onSizeChange,
}: {
  size?: CardSize;
  onSizeChange?: (size: CardSize) => void;
}) {
  const p = t.designSystem.pattern;
  const [view, setView] = useState("grid");
  const [sort, setSort] = useState("added");
  return (
    <Toolbar
      placement="topBar"
      search={
        <div className="relative">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            type="search"
            aria-label={p.search}
            placeholder={p.search}
            className="pl-9"
          />
        </div>
      }
      view={[
        {
          id: "sort",
          label: p.sort,
          inlineFrom: "md",
          control: (
            <Select value={sort} onValueChange={setSort}>
              <SelectTrigger aria-label={p.sort}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="added">{p.sortAdded}</SelectItem>
                <SelectItem value="title">{p.sortTitle}</SelectItem>
                <SelectItem value="duration">{p.sortDuration}</SelectItem>
              </SelectContent>
            </Select>
          ),
        },
        {
          id: "view",
          label: p.view,
          inlineFrom: "lg",
          control: (
            <ToggleGroup
              type="single"
              variant="outline"
              value={view}
              onValueChange={(value) => {
                if (value) setView(value);
              }}
              aria-label={p.view}
            >
              <IconToggle
                value="grid"
                label={p.grid}
                icon={<LayoutGrid aria-hidden="true" />}
              />
              <IconToggle
                value="list"
                label={p.list}
                icon={<List aria-hidden="true" />}
              />
            </ToggleGroup>
          ),
        },
        {
          id: "size",
          label: p.cardSize,
          inlineFrom: "xl",
          hideBelowSm: true,
          control: (
            <Slider
              aria-label={p.cardSize}
              min={0}
              max={3}
              step={1}
              value={[size]}
              onValueChange={([value]) => {
                onSizeChange?.((value ?? 1) as CardSize);
              }}
              className="w-zoom"
            />
          ),
        },
      ]}
      viewLabel={p.viewAndSort}
    >
      <Button variant="secondary" className="border border-input">
        <SlidersHorizontal aria-hidden="true" />
        {p.filters}
      </Button>
    </Toolbar>
  );
}

function IconToggle({
  value,
  label,
  icon,
}: {
  value: string;
  label: string;
  icon: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <ToggleGroupItem value={value} aria-label={label}>
          {icon}
        </ToggleGroupItem>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/** ExampleHeader は見本の見出し行である。 */
export function ExampleHeader() {
  const p = t.designSystem.pattern;
  return <PageHeader variant="list" title={p.library} count={p.videos(128)} />;
}

export function ListPageExample() {
  const p = t.designSystem.pattern;
  const videos = sampleVideos();
  const [size, setSize] = useState<CardSize>(1);
  const [selected, setSelected] = useState<ReadonlySet<string>>(
    () => new Set(["harbour", "market"]),
  );
  const toggle = (id: string, on: boolean) => {
    setSelected((current) => {
      const next = new Set(current);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  };
  return (
    <ListPage
      toolbarPlacement="topBar"
      header={<ExampleHeader />}
      toolbar={
        <ExampleTopBar>
          <ExampleToolbar size={size} onSizeChange={setSize} />
        </ExampleTopBar>
      }
      selectionBar={
        selected.size > 0 && (
          <SelectionBar
            count={p.selected(selected.size)}
            clearLabel={p.clearSelection}
            onClear={() => {
              setSelected(new Set());
            }}
          >
            <Button variant="ghost" size="sm">
              <Tag aria-hidden="true" />
              {p.addTag}
            </Button>
            <Button variant="ghost" size="sm">
              <EyeOff aria-hidden="true" />
              {p.hide}
            </Button>
          </SelectionBar>
        )
      }
    >
      <CardGrid size={size}>
        {videos.map((video) => (
          <SampleVideoCard
            key={video.id}
            title={video.title}
            meta={video.meta}
            duration={video.duration}
            selected={selected.has(video.id)}
            onSelectedChange={(on) => {
              toggle(video.id, on);
            }}
          />
        ))}
      </CardGrid>
      <LoadMoreRow status="loading" label={p.loadingMore} />
    </ListPage>
  );
}
