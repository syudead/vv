import { useEffect, useRef } from "react";

import { useScanControls } from "./ScanProvider";

/**
 * useWatchScanRefresh は、表示を始めたあとに自動の取り込み（`origin` が `watch`）が終わる
 * たびに `refresh` を呼ぶ。開いている一覧が結果をその場で取り込むための入口で、手動の取り込みの
 * 終わりに一覧を消して読み直す経路（`ScanControlsValue.finished`）とは別である
 * （specs/042-folder-watch-import/ui-design.md「Open lists after a watch scan」）。
 * 表示を始める前に終わっていた取り込みは、読み込んだ中身に反映済みなので呼ばない。
 */
export function useWatchScanRefresh(refresh: () => void) {
  const { watchFinished } = useScanControls();
  const knownId = useRef(watchFinished?.id);
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  useEffect(() => {
    if (watchFinished === null || knownId.current === watchFinished.id) return;
    knownId.current = watchFinished.id;
    refreshRef.current();
  }, [watchFinished]);
}
