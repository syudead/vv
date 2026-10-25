import { Hash } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";

import { isAborted, RequestFailed } from "../../api/client";
import { applyVideoCodeEdits, summarizeVideoCodeEdits } from "../../api/videoCodeEdits";
import type { VideoEditTargets } from "../../api/videoEdits";
import { errorText, t, type UiText } from "../../i18n";
import { Spinner } from "../../ui/shadcn/spinner";
import { useToast } from "../../ui/Toast";
import {
  DisabledReason,
  disabledProps,
  SelectionAction,
  SelectionBusyContext,
  WithTooltip,
} from "../SelectionAction";
import { editTargetsOf, type SelectionTarget } from "../selection";
import CodeEditor from "./CodeEditor";
import type { CodeEditItem, CodeEditRow } from "./rows";

interface Reading {
  controller: AbortController;
  selectionKey: string;
}

interface Open {
  targets: VideoEditTargets;
  rows: readonly CodeEditRow[];
}

/**
 * `Edit codes` on the selection bar: reads the summary before opening the editor, so an
 * over-limit selection gets a toast and no dialog. `onBusyChange` lets the bar disable its other
 * actions while the summary is read.
 */
export default function EditCodesAction({
  targets,
  disabledReason,
  blocked,
  onBusyChange,
}: {
  targets: readonly SelectionTarget[];
  disabledReason: UiText | undefined;
  /** Another bar action is in flight. */
  blocked: boolean;
  onBusyChange: (busy: boolean) => void;
}) {
  const c = t.library.codeEdit;
  const toast = useToast();
  const reasonId = useId();
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState<Open | null>(null);
  const reading = useRef<Reading | null>(null);

  useEffect(() => () => reading.current?.controller.abort(), []);

  // Cards stay selectable while the summary is read, and its rows would be for the old selection.
  const selectionKey = JSON.stringify(editTargetsOf(targets));
  useEffect(() => {
    const pending = reading.current;
    if (pending === null || pending.selectionKey === selectionKey) return;
    pending.controller.abort();
    reading.current = null;
    setBusy(false);
    onBusyChange(false);
  }, [selectionKey, onBusyChange]);

  function settle() {
    reading.current = null;
    setBusy(false);
    onBusyChange(false);
  }

  function start() {
    if (busy) return;
    const request = editTargetsOf(targets);
    const controller = new AbortController();
    reading.current = { controller, selectionKey };
    setBusy(true);
    onBusyChange(true);
    summarizeVideoCodeEdits(request, controller.signal).then(
      (summary) => {
        if (controller.signal.aborted) return;
        settle();
        setOpen({ targets: request, rows: summary.rows });
      },
      (error: unknown) => {
        if (controller.signal.aborted || isAborted(error)) return;
        settle();
        const tooMany =
          error instanceof RequestFailed && error.reason === "too_many_videos";
        toast(tooMany ? errorText(error) : c.summaryFailed(errorText(error)));
      },
    );
  }

  async function apply(items: CodeEditItem[]) {
    if (open === null) return;
    const result = await applyVideoCodeEdits(items, open.targets.smartFolderIds);
    toast(c.applied(result.applied));
    setOpen(null);
  }

  return (
    // The pressed item stays focusable while busy, so focus is not lost to the page.
    <SelectionBusyContext.Provider value={false}>
      <WithTooltip label={disabledReason ?? c.action}>
        <SelectionAction
          icon={busy ? <Spinner /> : <Hash aria-hidden="true" />}
          label={c.action}
          {...disabledProps(disabledReason, reasonId)}
          disabled={disabledReason !== undefined || blocked}
          aria-disabled={busy || undefined}
          aria-busy={busy || undefined}
          onClick={start}
        />
      </WithTooltip>
      <DisabledReason id={reasonId} reason={disabledReason} />
      {open !== null && (
        <CodeEditor rows={open.rows} onApply={apply} onClose={() => setOpen(null)} />
      )}
    </SelectionBusyContext.Provider>
  );
}
