import { useCallback, useEffect, useLayoutEffect, useMemo } from "react";

import type { LibraryItem } from "../api/client";
import { itemVideos, videoKey } from "../api/libraryItems";
import { keepMembers } from "../library/selection";
import SelectionBar from "../library/SelectionBar";
import { useListSelection } from "../library/useListSelection";
import { useSelectionTargets } from "./pageHooks";

/**
 * Owner selection over the loaded videos of a page without groups or smart folders, such as a
 * folder, with the library's checks, Shift ranges and drawn boxes. Videos that leave the list
 * drop out, so bulk actions cannot reach hidden ones; `onChanged` runs after a bulk action that
 * needs the list read again.
 */
export function useVideoSelection(
  items: readonly LibraryItem[],
  owner: boolean,
  onChanged: () => void,
  /** Boxes select grid cards only, so a list view turns them off. */
  drawsBoxes = true,
) {
  const list = useListSelection({ boxes: owner && drawsBoxes });
  const { selection, setSelection, show, clear, retain, changeVideo } = list;
  const loaded = useMemo(
    () => new Set(itemVideos(items).map((video) => video.id)),
    [items],
  );
  useEffect(() => {
    setSelection((current) => keepMembers(current, loaded));
  }, [loaded, setSelection]);
  useLayoutEffect(() => show(items));
  const selectionMode = selection.ids.size > 0;

  const { favorites, selectedIds, editTargets } = useSelectionTargets(selection, items);
  const selectedKeys = useMemo(() => selectedIds.map(videoKey), [selectedIds]);
  const afterBulk = useCallback(() => {
    clear();
    onChanged();
  }, [clear, onChanged]);

  const cardProps = useCallback(
    (id: number) => ({
      selected: selection.ids.has(id),
      selectionMode,
      onSelect: owner ? changeVideo : undefined,
    }),
    [changeVideo, owner, selection.ids, selectionMode],
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
      onRetain={retain}
    />
  ) : null;

  return {
    cardProps,
    selectedKeys,
    clear,
    bar,
    container: list.container,
    marqueeBox: list.marqueeBox,
  };
}

export type VideoSelection = ReturnType<typeof useVideoSelection>;
