import type { ReactNode } from "react";

import { t, type UiText } from "../../i18n";
import type { FileSearchField } from "../../videoList/SearchBox";
import { QueryChip } from "./QueryChip";
import type { ImageSearch } from "./useImageSearch";

/** The toolbar's file entry and, while an image search shows, its chip and the locked controls; guests get neither. */
export function imageSearchToolbar(
  search: ImageSearch,
  owner: boolean,
): { fileSearch?: FileSearchField; lockedReason?: UiText } {
  if (!owner) return {};
  const { phase, clear } = search;
  return {
    fileSearch: {
      onPick: () => search.setDialogOpen(true),
      onFiles: search.search,
      onClear: clear,
      chip: phase === null ? undefined : <QueryChip file={phase.file} onClear={clear} />,
    },
    lockedReason: phase === null ? undefined : t.library.imageSearch.notUsed,
  };
}

/** Library-only parts, such as the selection bar, are not drawn while an image search shows. */
export function withoutImageSearch(search: ImageSearch, node: ReactNode): ReactNode {
  return search.phase === null ? node : null;
}
