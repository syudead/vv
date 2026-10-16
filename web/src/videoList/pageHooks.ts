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

// 1 つのものを開いた一覧の画面（グループのページ、スマートフォルダのページ）が共有するフック。

/** useDocumentTitle は名前が分かっている間だけ、窓の題をその名前にする。 */
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

/** useViewPreferences はライブラリと共有する表示形式と大きさである。 */
function useViewPreferences() {
  const [preferences, setPreferences] = useState(readViewPreferences);
  const save = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);
  return { preferences, save };
}

/**
 * useViewControls はライブラリと共有する表示形式と大きさを、変えるときにホバープレビューを
 * 止めて保存する。大きさを変える前に見ていた位置を覚え、`listRef` の一覧で戻す（useZoomAnchor）。
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

/** useSentinel は番兵が見える範囲の 600px 以内に来たら `onNear` を呼ぶ（ライブラリと同じ）。 */
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

/**
 * useLoadMoreSentinel は番兵が近づいたら、ホバープレビューを止めて続きを読む。`enabled` の
 * 間だけ見張る。
 */
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
 * usePressTagToLibrary はカードのタグを押したとき、そのタグで絞ったライブラリを開く。開いた
 * ものの並びを見せる画面（グループ、スマートフォルダ）は自分の絞り込みを持たない。
 */
export function usePressTagToLibrary() {
  const navigate = useNavigate();
  return useCallback(
    (tag: TagRef) => void navigate(libraryFilterPath(TAG_PARAM, tag.id)),
    [navigate],
  );
}

/** useSelectionTargets は選択バーへ渡す、選んだ動画の id とお気に入りの対象である。 */
export function useSelectionTargets(selection: LibrarySelection) {
  const favorites = useMemo(() => favoriteTargets(selection), [selection]);
  const selectedIds = useMemo(() => Array.from(selection.ids), [selection.ids]);
  return { favorites, selectedIds };
}
