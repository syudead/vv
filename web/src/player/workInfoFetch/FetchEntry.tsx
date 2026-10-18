import { CloudDownload } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import type { Video } from "../../api/client";
import type { DetailMark } from "../../api/useVideoDetail";
import { listWorkInfoPlugins, type WorkInfoPlugin } from "../../api/workInfoPlugins";
import { t } from "../../i18n";
import ActionButton from "../ActionButton";
import type { ThumbnailCapture } from "../VideoFacts";
import { approvedPlugins } from "./fetchRules";
import FetchDialog from "./FetchDialog";

/** Show approved plugins only; abort obsolete reads and hide the entry until an initial list is available. */
function useApprovedPlugins(): {
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
      if (!controller.signal.aborted) setPlugins(approvedPlugins(list));
    } catch {
      // Keep the last successful list silently when refresh fails.
    }
  }, []);

  useEffect(() => {
    void reload();
    return () => latest.current?.abort();
  }, [reload]);
  return { plugins, reload };
}

/** Pause on open and retain an open dialog even if the entry later becomes unavailable. */
export default function FetchEntry({
  video,
  capture,
  onApplied,
  onDialogChange,
}: {
  video: Video;
  capture?: ThumbnailCapture;
  onApplied: (video: Video, mark: DetailMark) => void;
  /** Suspend video-page shortcuts while the dialog is open. */
  onDialogChange?: (open: boolean) => void;
}) {
  const { plugins, reload } = useApprovedPlugins();
  const [open, setOpen] = useState(false);
  const dialogChange = useRef(onDialogChange);
  dialogChange.current = onDialogChange;

  useEffect(() => {
    dialogChange.current?.(open);
  }, [open]);
  useEffect(() => () => dialogChange.current?.(false), []);

  return (
    <>
      {plugins.length > 0 && (
        <ActionButton
          label={t.player.workInfoFetch.entry}
          onClick={() => {
            capture?.pause();
            setOpen(true);
          }}
        >
          <CloudDownload aria-hidden="true" />
        </ActionButton>
      )}
      {open && (
        <FetchDialog
          video={video}
          plugins={plugins}
          onReload={reload}
          onApplied={onApplied}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}
