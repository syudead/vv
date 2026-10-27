import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import type { LibraryItem } from "../api/client";
import { itemVideos, videoKey } from "../api/libraryItems";
import {
  emptySelection,
  retainVideos,
  keepMembers,
  type LibrarySelection,
  setVideo,
} from "../library/selection";
import { MarqueeBox } from "../library/MarqueeBox";
import SelectionBar from "../library/SelectionBar";
import { useEscapeClearsSelection } from "../library/useEscapeClearsSelection";
import { useMarqueeSelection } from "../library/useMarqueeSelection";
import { useRangeSelection } from "../library/useRangeSelection";
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
  const [selection, setSelection] = useState<LibrarySelection>(emptySelection);
  const loaded = useMemo(
    () => new Set(itemVideos(items).map((video) => video.id)),
    [items],
  );
  useEffect(() => {
    setSelection((current) => keepMembers(current, loaded));
  }, [loaded]);

  const displayed = useRef(items);
  useLayoutEffect(() => {
    displayed.current = items;
  });
  const { select: selectRange, point } = useRangeSelection(displayed, setSelection);
  const clear = useCallback(() => {
    point(null);
    setSelection(emptySelection);
  }, [point]);
  const retain = useCallback(
    (kept: ReadonlySet<number>) => setSelection((current) => retainVideos(current, kept)),
    [],
  );
  const selectionMode = selection.ids.size > 0;
  useEscapeClearsSelection(selectionMode, clear);
  const select = useCallback(
    (id: number, selected: boolean) =>
      selectRange(videoKey(id), selected, (current) => setVideo(current, id, selected)),
    [selectRange],
  );

  const container = useRef<HTMLDivElement | null>(null);
  const box = useRef<HTMLDivElement | null>(null);
  useMarqueeSelection({
    container,
    box,
    enabled: owner && drawsBoxes,
    items,
    selection,
    setSelection,
  });

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
      onRetain={retain}
    />
  ) : null;

  const marqueeBox = <MarqueeBox ref={box} />;

  return { cardProps, selectedKeys, clear, bar, container, marqueeBox };
}

export type VideoSelection = ReturnType<typeof useVideoSelection>;
