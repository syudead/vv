import { LayoutGrid, List } from "lucide-react";
import type { ReactNode, RefObject } from "react";

import type { VideoSort, WatchFilter } from "../api/client";
import { t } from "../i18n";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import { Toolbar, type ToolbarViewControl } from "../ui/patterns/toolbar";
import FilterMenu from "../videoList/FilterMenu";
import IconToggleGroup from "../videoList/IconToggleGroup";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import { SortMenu } from "../videoList/SortControls";
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
  /** 絞り込み中のタグ（ActiveTagFilters）。絞り込みの操作の後ろに並べる。 */
  activeFilters?: ReactNode;
}

/**
 * LibraryToolbar はライブラリの一覧の操作を画面の型の Toolbar に並べる。並びは Tab の
 * 順で、検索欄 → 絞り込み → 絞り込み中のタグ → 表示形式・大きさ・並べ替え
 * （ui-design.md「Toolbar」、web/registry/rules/patterns.md「Sections」）。lg より狭い幅では
 * 表示形式・大きさ・並べ替えを「View and sort」のポップオーバーに畳む。
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
  activeFilters,
}: LibraryToolbarProps) {
  const controls: ToolbarViewControl[] = [
    {
      id: "view",
      label: t.library.view.label,
      control: (
        <IconToggleGroup
          label={t.library.view.label}
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
      control: <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-zoom" />,
    });
  }
  controls.push({
    id: "sort",
    label: t.list.sort.heading,
    control: <SortMenu sort={sort} onSortChange={onSortChange} onShuffle={onShuffle} />,
  });

  return (
    <Toolbar
      search={<SearchBox query={query} onCommit={onQueryCommit} inputRef={searchRef} />}
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
      />
      {activeFilters}
    </Toolbar>
  );
}
