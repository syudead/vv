import type { RefObject } from "react";

import type { VideoSort } from "../api/client";
import { t } from "../i18n";
import TopBarPortal from "../shell/TopBarPortal";
import { Toolbar } from "../ui/patterns/toolbar";
import FilterMenu from "./FilterMenu";
import type { HistoryMode } from "./listCriteria";
import SearchBox from "./SearchBox";
import { sortViewControl } from "./SortControls";

export interface ShelfToolbarProps {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  sort: VideoSort;
  onSortChange: (value: VideoSort) => void;
  onShuffle: () => void;
  favorite: boolean;
  onFavoriteChange: (value: boolean) => void;
  canClear: boolean;
  onClear: () => void;
}

/** The search, filter and sort of a record page's video shelves, in the shell's top bar. */
export function ShelfToolbar({
  query,
  onQueryCommit,
  searchRef,
  sort,
  onSortChange,
  onShuffle,
  favorite,
  onFavoriteChange,
  canClear,
  onClear,
}: ShelfToolbarProps) {
  return (
    <TopBarPortal>
      <Toolbar
        placement="topBar"
        search={
          <SearchBox
            query={query}
            onCommit={onQueryCommit}
            inputRef={searchRef}
            commitOn="enter"
          />
        }
        view={[
          sortViewControl({ name: "shelf-compact-sort", sort, onSortChange, onShuffle }),
        ]}
        viewLabel={t.list.sort.heading}
      >
        <FilterMenu
          favorite={favorite}
          onApply={onFavoriteChange}
          canClear={canClear}
          onClear={onClear}
        />
      </Toolbar>
    </TopBarPortal>
  );
}
