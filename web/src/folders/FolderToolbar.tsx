import type { RefObject } from "react";

import type { VideoSort, WatchFilter } from "../api/client";
import { t, type UiText } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { Toolbar } from "../ui/patterns/toolbar";
import FilterMenu from "../videoList/FilterMenu";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import { CompactSortControls, SortMenu } from "../videoList/SortControls";
import ZoomSlider from "../videoList/ZoomSlider";

export interface FolderToolbarProps {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  searchLabel: UiText;
  searchPlaceholder: UiText;
  sort: VideoSort;
  onSortChange: (value: VideoSort) => void;
  onShuffle: () => void;
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playable: boolean;
  onPlayableChange: (value: boolean) => void;
  favorite: boolean;
  onFavoriteChange: (value: boolean) => void;
  /**
   * 検索語・視聴状態・再生可否・お気に入りのみのどれかが効いているか
   * （「条件を解除」を出す）。
   */
  canClear: boolean;
  onClear: () => void;
  /**
   * 最上位（`/folders`）で検索語が空のときは true。絞り込み・並べ替え・向きを
   * 無効にする（ui-design.md「Top level」）。検索欄と表示形式は無効にしない。
   */
  disabled?: boolean;
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
}

/**
 * FolderToolbar はフォルダ画面（各フォルダ・最上位）の一覧ページのツールバーである。
 * ライブラリのツールバーから表示形式の切り替えを除いたもので、デザインシステムの
 * `Toolbar` の placement="topBar" に載せ、画面が TopBarPortal で共通のトップバーへ入れる
 * （web/registry/rules/patterns.md の Sections）。検索欄・絞り込み・並べ替え・向き・
 * 大きさの部品は、ライブラリと共有する `web/src/videoList/` のものを使う。並べ替えは
 * `md`、大きさは `xl` から並べ、それより狭いとアイコンだけの「表示と並び順」にまとめる。
 */
export default function FolderToolbar({
  query,
  onQueryCommit,
  searchRef,
  searchLabel,
  searchPlaceholder,
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
  disabled,
  zoom,
  onZoomChange,
}: FolderToolbarProps) {
  return (
    <Toolbar
      placement="topBar"
      search={
        <SearchBox
          query={query}
          onCommit={onQueryCommit}
          inputRef={searchRef}
          label={searchLabel}
          placeholder={searchPlaceholder}
          className="w-full"
        />
      }
      view={[
        {
          id: "sort",
          label: t.list.sort.heading,
          inlineFrom: "md",
          control: (
            <SortMenu
              sort={sort}
              onSortChange={onSortChange}
              onShuffle={onShuffle}
              disabled={disabled}
            />
          ),
          compact: (
            <CompactSortControls
              name="folder-compact-sort"
              sort={sort}
              onSortChange={onSortChange}
              onShuffle={onShuffle}
              disabled={disabled}
            />
          ),
          compactLabelled: true,
        },
        {
          id: "size",
          label: t.list.cardSize,
          inlineFrom: "xl",
          hideBelowSm: true,
          control: (
            <ZoomSlider
              zoom={zoom}
              onZoomChange={onZoomChange}
              className="w-zoom"
              tooltip
            />
          ),
          compact: (
            <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-full" />
          ),
        },
      ]}
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
        disabled={disabled}
      />
    </Toolbar>
  );
}
