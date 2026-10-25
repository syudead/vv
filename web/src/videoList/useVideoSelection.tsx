import { useCallback, useEffect, useMemo, useState } from "react";

import type { LibraryItem } from "../api/client";
import { itemVideos } from "../api/libraryItems";
import {
  emptySelection,
  keepMembers,
  type LibrarySelection,
  setVideo,
} from "../library/selection";
import SelectionBar from "../library/SelectionBar";
import { useEscapeClearsSelection } from "../library/useEscapeClearsSelection";
import { useSelectionTargets } from "./pageHooks";

/**
 * Owner selection over the loaded videos of a page without groups or smart folders, such as a
 * folder. Videos that leave the list drop out, so bulk actions cannot reach
 * hidden ones; `onChanged` runs after a bulk action that needs the list read again.
 */
export function useVideoSelection(
  items: readonly LibraryItem[],
  owner: boolean,
  onChanged: () => void,
) {
  const [selection, setSelection] = useState<LibrarySelection>(emptySelection);
  const loaded = useMemo(
    () => new Set(itemVideos(items).map((video) => video.id)),
    [items],
  );
  useEffect(() => {
    setSelection((current) => keepMembers(current, loaded));
  }, [loaded]);
  const clear = useCallback(() => setSelection(emptySelection), []);
  const selectionMode = selection.ids.size > 0;
  useEscapeClearsSelection(selectionMode, clear);
  const select = useCallback(
    (id: number, selected: boolean) =>
      setSelection((current) => setVideo(current, id, selected)),
    [],
  );
  const { favorites, selectedIds, editTargets } = useSelectionTargets(selection, items);
  const afterBulk = useCallback(() => {
    clear();
    onChanged();
  }, [clear, onChanged]);

  const cardProps = useCallback(
    (id: number) => ({
      selected: selection.ids.has(id),
      selectionMode,
      onSelect: owner ? select : undefined,
    }),
    [owner, select, selection.ids, selectionMode],
  );

  const bar = owner ? (
    <SelectionBar
      count={selection.ids.size}
      targets={editTargets}
      selectedIds={selectedIds}
      favoriteVideoIds={favorites.videoIds}
      favoriteGroupIds={favorites.groupIds}
      onClear={clear}
      onTagRemoved={onChanged}
      onBundled={afterBulk}
      onCollectionsDeleted={afterBulk}
    />
  ) : null;

  return { cardProps, clear, bar };
}

export type CardSelection = ReturnType<typeof useVideoSelection>["cardProps"];
