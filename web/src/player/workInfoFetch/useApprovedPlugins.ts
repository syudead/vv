import { useCallback, useEffect, useRef, useState } from "react";

import { listWorkInfoPlugins, type WorkInfoPlugin } from "../../api/workInfoPlugins";

/**
 * The plugins `select` keeps from the list; abort obsolete reads and hide the entry until an
 * initial list is available. `select` must be stable. A disabled reader reads nothing and keeps
 * the list empty, for viewers who cannot list plugins.
 */
export function useApprovedPlugins(
  select: (plugins: readonly WorkInfoPlugin[]) => WorkInfoPlugin[],
  enabled = true,
): {
  plugins: WorkInfoPlugin[];
  reload: () => Promise<void>;
} {
  const [plugins, setPlugins] = useState<WorkInfoPlugin[]>([]);
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
  }, [select]);

  useEffect(() => {
    if (!enabled) return;
    void reload();
    return () => latest.current?.abort();
  }, [reload, enabled]);
  return { plugins, reload };
}
