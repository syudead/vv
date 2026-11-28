import type { ReactNode, RefObject } from "react";

import type { VideoSort } from "../api/client";
import { Toolbar } from "../ui/patterns/toolbar";
import FilterMenu from "./FilterMenu";
import type { HistoryMode } from "./listCriteria";
import SearchBox from "./SearchBox";
import { SortMenu } from "./SortControls";

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

/** The search, filter and sort of a record page's video shelves, drawn above them in the page. */
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
    <Toolbar
      search={
        <SearchBox
          query={query}
          onCommit={onQueryCommit}
          inputRef={searchRef}
          commitOn="enter"
        />
      }
      // The sort is the only view control, so it stays in the row rather than behind a second button.
      actions={<SortMenu sort={sort} onSortChange={onSortChange} onShuffle={onShuffle} />}
    >
      <FilterMenu
        favorite={favorite}
        onApply={onFavoriteChange}
        canClear={canClear}
        onClear={onClear}
      />
    </Toolbar>
  );
}

/** The toolbar over the shelves it narrows, closer to them than the shelves are to each other. */
export function ShelfArea({
  toolbar,
  children,
}: {
  toolbar: ShelfToolbarProps;
  children: ReactNode;
}) {
  return (
    <section data-slot="shelf-area" className="flex min-w-0 flex-col gap-4">
      <ShelfToolbar {...toolbar} />
      <div className="flex min-w-0 flex-col gap-8">{children}</div>
    </section>
  );
}
