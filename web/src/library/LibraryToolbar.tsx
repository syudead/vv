import { LayoutGrid, List } from "lucide-react";
import type { RefObject } from "react";

import type { VideoSort, WatchFilter } from "../api/client";
import { t } from "../i18n";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import { Toolbar, type ToolbarViewControl } from "../ui/patterns/toolbar";
import FilterMenu from "../videoList/FilterMenu";
import IconToggleGroup from "../videoList/IconToggleGroup";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import { CompactSortControls, SortMenu } from "../videoList/SortControls";
import ZoomSlider from "../videoList/ZoomSlider";

/** viewOptions は表示形式の選択肢である。文言は描画のたびにカタログから引く。 */
function viewOptions() {
  return [
    {
      value: "grid",
      label: t.library.view.grid,
      icon: <LayoutGrid aria-hidden="true" />,
    },
    { value: "list", label: t.library.view.list, icon: <List aria-hidden="true" /> },
  ] as const;
}

export interface LibraryToolbarProps {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  sort: VideoSort;
  onSortChange: (value: VideoSort) => void;
  onShuffle: () => void;
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playable: boolean;
  onPlayableChange: (value: boolean) => void;
  favorite: boolean;
  onFavoriteChange: (value: boolean) => void;
  canClear: boolean;
  onClear: () => void;
  view: ViewMode;
  onViewChange: (value: ViewMode) => void;
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
}

/**
 * LibraryToolbar はライブラリの操作をトップバーの中の 1 行に集める（画面の型の Toolbar の
 * topBar の置き場。LibraryPage が TopBarPortal で入れる）。並びは Tab の順で、
 * 検索欄 → 絞り込み → 並べ替え → 向き（並べ直す）→ 表示形式・大きさ
 * （ui-design.md「Toolbar」）。並べ替えは md、表示形式は lg、大きさは xl から行に並び、
 * それより狭い幅ではアイコンだけの「View and sort」のポップオーバーに入る。
 */
export default function LibraryToolbar({
  query,
  onQueryCommit,
  searchRef,
  sort,
  onSortChange,
  onShuffle,
  watch,
  onWatchChange,
  playable,
  onPlayableChange,
  favorite,
  onFavoriteChange,
  canClear,
  onClear,
  view,
  onViewChange,
  zoom,
  onZoomChange,
}: LibraryToolbarProps) {
  const controls: ToolbarViewControl[] = [
    {
      id: "sort",
      label: t.list.sort.heading,
      inlineFrom: "md",
      control: (
        <SortMenu
          sort={sort}
          onSortChange={onSortChange}
          onShuffle={onShuffle}
          inTopBar
        />
      ),
      compact: (
        <CompactSortControls
          name="compact-sort"
          sort={sort}
          onSortChange={onSortChange}
          onShuffle={onShuffle}
        />
      ),
      compactLabelled: true,
    },
    {
      id: "view",
      label: t.library.view.label,
      inlineFrom: "lg",
      control: (
        <IconToggleGroup
          label={t.library.view.label}
          value={view}
          onValueChange={onViewChange}
          options={viewOptions()}
          inTopBar
        />
      ),
      compact: (
        <IconToggleGroup
          label={t.library.view.compact}
          value={view}
          onValueChange={onViewChange}
          options={viewOptions()}
        />
      ),
    },
  ];
  // 大きさは格子表示のときだけ意味を持つ。
  if (view === "grid") {
    controls.push({
      id: "size",
      label: t.list.cardSize,
      inlineFrom: "xl",
      hideBelowSm: true,
      control: (
        <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-zoom" tooltip />
      ),
      compact: <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-full" />,
    });
  }

  return (
    <Toolbar
      placement="topBar"
      search={
        <SearchBox query={query} onCommit={onQueryCommit} inputRef={searchRef} inTopBar />
      }
      view={controls}
      viewLabel={t.list.viewAndSort}
    >
      <FilterMenu
        watch={watch}
        onWatchChange={onWatchChange}
        playable={playable}
        onPlayableChange={onPlayableChange}
        favorite={favorite}
        onFavoriteChange={onFavoriteChange}
        canClear={canClear}
        onClear={onClear}
        inTopBar
      />
    </Toolbar>
  );
}
