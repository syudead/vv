import { useCallback, useEffect, useRef, useState } from "react";

import { listWorkInfoPlugins, type WorkInfoPlugin } from "../../api/workInfoPlugins";

/**
 * The plugins `select` keeps from the list; abort obsolete reads and hide the entry until an
 * initial list is available. `select` must be stable. A disabled reader reads nothing and keeps
 * the list empty, for viewers who cannot list plugins. `loaded` turns true once the first read
 * settles, whether or not it succeeded.
 */
export function useApprovedPlugins(
  select: (plugins: readonly WorkInfoPlugin[]) => WorkInfoPlugin[],
  enabled = true,
): {
  plugins: WorkInfoPlugin[];
  loaded: boolean;
  reload: () => Promise<void>;
} {
  const [plugins, setPlugins] = useState<WorkInfoPlugin[]>([]);
  const [loaded, setLoaded] = useState(false);
  const latest = useRef<AbortController | null>(null);

  const reload = useCallback(async () => {
    latest.current?.abort();
    const controller = new AbortController();
    latest.current = controller;
    try {
      const list = await listWorkInfoPlugins(controller.signal);
      if (!controller.signal.aborted) setPlugins(select(list));
    } catch {
      // Keep the last successful list silently when refresh fails.
    }
    if (!controller.signal.aborted) setLoaded(true);
  }, [select]);

  useEffect(() => {
    if (!enabled) return;
    void reload();
    return () => latest.current?.abort();
  }, [reload, enabled]);
  return { plugins, loaded, reload };
}
