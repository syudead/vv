import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";

import type { TagRef } from "../api/client";
import { libraryFilterPath } from "../library/libraryPath";
import { favoriteTargets, type LibrarySelection } from "../library/selection";
import { TAG_PARAM } from "../library/tagCriteria";

import {
  readViewPreferences,
  type ViewMode,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import { useZoomAnchor } from "./useZoomAnchor";

export function useDocumentTitle(name: string | undefined) {
  useEffect(() => {
    if (name === undefined) return;
    const previous = document.title;
    document.title = name;
    return () => {
      document.title = previous;
    };
  }, [name]);
}

function useViewPreferences() {
  const [preferences, setPreferences] = useState(readViewPreferences);
  const save = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);
  return { preferences, save };
}

/**
 * Stop previews before changing view or zoom; capture the current anchor before resizing and
 * persist shared preferences.
 */
export function useViewControls(resetPreview: () => void) {
  const { preferences, save } = useViewPreferences();
  const { listRef, capture } = useZoomAnchor(preferences.zoom);
  const changeView = useCallback(
    (next: ViewMode) => {
      resetPreview();
      save({ ...preferences, view: next });
    },
    [preferences, resetPreview, save],
  );
  const changeZoom = useCallback(
    (next: Zoom) => {
      capture();
      resetPreview();
      save({ ...preferences, zoom: next });
    },
    [capture, preferences, resetPreview, save],
  );
  return {
    view: preferences.view,
    zoom: preferences.zoom,
    changeView,
    changeZoom,
    listRef,
  };
}

export function useSentinel(enabled: boolean, onNear: () => void) {
  const sentinel = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !enabled) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onNear();
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [enabled, onNear]);
  return sentinel;
}

/** Stop previews before pagination; observe only while enabled. */
export function useLoadMoreSentinel(
  enabled: boolean,
  loadMore: () => void,
  resetPreview: () => void,
) {
  const near = useCallback(() => {
    resetPreview();
    loadMore();
  }, [loadMore, resetPreview]);
  return useSentinel(enabled, near);
}

/**
 * Group and smart-folder pages have no independent tag filter, so tag clicks open the filtered
 * library.
 */
export function usePressTagToLibrary() {
  const navigate = useNavigate();
  return useCallback(
    (tag: TagRef) => void navigate(libraryFilterPath(TAG_PARAM, tag.id)),
    [navigate],
  );
}

export function useSelectionTargets(selection: LibrarySelection) {
  const favorites = useMemo(() => favoriteTargets(selection), [selection]);
  const selectedIds = useMemo(() => Array.from(selection.ids), [selection.ids]);
  return { favorites, selectedIds };
}
