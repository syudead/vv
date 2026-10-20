import { useEffect, useRef, useState } from "react";

import { isAborted } from "../../api/client";
import {
  summarizeVideoEdits,
  type VideoEditSummary,
  type VideoEditTargets,
} from "../../api/videoEdits";
import { errorText, type UiText } from "../../i18n";

export type SummaryState =
  | { kind: "loading"; previous?: VideoEditSummary }
  | { kind: "ready"; summary: VideoEditSummary }
  | { kind: "failed"; reason: UiText };

/** Read the summary for the current targets; changing targets keeps the previous summary until the new one arrives. */
export function useEditSummary(
  targets: VideoEditTargets,
  targetKey: string,
  attempt: number,
): SummaryState {
  const [state, setState] = useState<SummaryState>({ kind: "loading" });
  // targetKey identifies the targets; the arrays are rebuilt on every render.
  const latest = useRef(targets);
  latest.current = targets;
  useEffect(() => {
    const targets = latest.current;
    const controller = new AbortController();
    setState((current) => ({
      kind: "loading",
      previous:
        current.kind === "ready"
          ? current.summary
          : current.kind === "loading"
            ? current.previous
            : undefined,
    }));
    if (targets.videoIds.length + targets.smartFolderIds.length === 0) return;
    summarizeVideoEdits(targets, controller.signal).then(
      (summary) => {
        if (!controller.signal.aborted) setState({ kind: "ready", summary });
      },
      (error: unknown) => {
        if (controller.signal.aborted || isAborted(error)) return;
        setState({ kind: "failed", reason: errorText(error) });
      },
    );
    return () => controller.abort();
  }, [targetKey, attempt]);
  return state;
}
