import { useCallback, useRef, useState } from "react";

import type { LibraryGroup, LibraryItem } from "../api/client";
import { groupKey, smartFolderKey, videoKey } from "../api/libraryItems";
import { MarqueeBox } from "./MarqueeBox";
import {
  emptySelection,
  isGroupSelected,
  type LibrarySelection,
  retainVideos,
  selectedCount,
  selectedGroupOf,
  setGroup,
  setSmartFolder,
  setVideo,
} from "./selection";
import { useEscapeClearsSelection } from "./useEscapeClearsSelection";
import { useMarqueeSelection } from "./useMarqueeSelection";
import { useRangeSelection } from "./useRangeSelection";

const noItems: readonly LibraryItem[] = [];

/**
 * The owner's selection over a list of videos, groups and smart folders, shared by every page
 * with a selection bar so checks, Shift ranges, drawn boxes and Escape behave the same on each.
 * Pages keep their own rule for dropping items that leave the list, through `setSelection`.
 */
export function useListSelection({
  boxes,
  cardKeyOf,
}: {
  /** Drawn boxes select grid cards only: the owner's, outside list view and loading. */
  boxes: boolean;
  /** The `data-card-key` of an item's card, when the page does not key cards by item. */
  cardKeyOf?: (item: LibraryItem) => string;
}) {
  const [selection, setSelection] = useState<LibrarySelection>(emptySelection);
  // The items as displayed, pending moves included, so ranges and boxes never reach hidden ones.
  const displayed = useRef<readonly LibraryItem[]>(noItems);
  const show = useCallback((items: readonly LibraryItem[]) => {
    displayed.current = items;
  }, []);
  const { select, point } = useRangeSelection(displayed, setSelection);
  // Toggles read the rendered selection, so a Shift toggle extends a range like a check.
  const now = useRef(selection);
  now.current = selection;

  const changeVideo = useCallback(
    (id: number, selected: boolean) =>
      select(videoKey(id), selected, (current) => setVideo(current, id, selected)),
    [select],
  );
  const changeGroup = useCallback(
    (group: LibraryGroup, selected: boolean) =>
      select(groupKey(group.id), selected, (current) =>
        setGroup(current, selectedGroupOf(group), selected),
      ),
    [select],
  );
  const changeSmartFolder = useCallback(
    (id: number, selected: boolean) =>
      select(smartFolderKey(id), selected, (current) =>
        setSmartFolder(current, id, selected),
      ),
    [select],
  );
  const toggleVideo = useCallback(
    (id: number) => changeVideo(id, !now.current.ids.has(id)),
    [changeVideo],
  );
  const toggleGroup = useCallback(
    (group: LibraryGroup) => changeGroup(group, !isGroupSelected(now.current, group.id)),
    [changeGroup],
  );
  const toggleSmartFolder = useCallback(
    (id: number) => changeSmartFolder(id, !now.current.smartFolders.has(id)),
    [changeSmartFolder],
  );
  const clear = useCallback(() => {
    point(null);
    setSelection(emptySelection);
  }, [point]);
  const retain = useCallback(
    (kept: ReadonlySet<number>) => setSelection((current) => retainVideos(current, kept)),
    [],
  );
  useEscapeClearsSelection(selectedCount(selection) > 0, clear);

  const container = useRef<HTMLDivElement | null>(null);
  const box = useRef<HTMLDivElement | null>(null);
  useMarqueeSelection({
    container,
    box,
    enabled: boxes,
    items: displayed,
    cardKeyOf,
    selection,
    setSelection,
  });

  return {
    selection,
    setSelection,
    /** Call with the displayed items after each render, before any press can read them. */
    show,
    clear,
    retain,
    changeVideo,
    changeGroup,
    changeSmartFolder,
    toggleVideo,
    toggleGroup,
    toggleSmartFolder,
    /** Presses inside it, outside cards and controls, start a box. */
    container,
    marqueeBox: <MarqueeBox ref={box} />,
  };
}

export type ListSelection = ReturnType<typeof useListSelection>;
