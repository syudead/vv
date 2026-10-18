import { useEffect, useRef } from "react";

import { useScanControls } from "./ScanProvider";

/**
 * Refresh in place for automatic scans completed after mounting; earlier completions are already
 * reflected in loaded data.
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
