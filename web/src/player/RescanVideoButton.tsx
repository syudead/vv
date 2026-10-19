import { RotateCcw } from "lucide-react";

import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { useScanControlsIfProvided } from "../shell/ScanProvider";
import { useToast } from "../ui/Toast";
import ActionButton from "./ActionButton";

/**
 * Starts a force rescan of one video without confirmation. Blocked states stay focusable with
 * aria-disabled so the tooltip can give the reason.
 */
export default function RescanVideoButton({
  videoId,
  report,
}: {
  videoId: number;
  /** Receives null when starting and the failure otherwise, for the facts' shared error row. */
  report: (failure: UiText | null) => void;
}) {
  const controls = useScanControlsIfProvided();
  const toast = useToast();
  if (controls === null) return null;

  const f = t.player.facts;
  const reason = controls.running
    ? t.settings.scanStatus.runningReason
    : controls.noMediaFolders
      ? f.rescanNeedsMediaFolder
      : null;
  const blocked = controls.running || !controls.canStart;

  const start = () => {
    if (blocked) return;
    report(null);
    void controls.rescan({ scope: "video", videoId }).then((failure) => {
      if (failure === null) toast(f.rescanStarted);
      else report(t.shell.scan.rescanFailed(failure));
    });
  };

  return (
    <ActionButton
      label={reason === null ? f.forceRescan : f.forceRescanBlocked(reason)}
      aria-disabled={blocked || undefined}
      onClick={start}
      className={cn("aria-disabled:cursor-default", blocked && "opacity-50")}
    >
      <RotateCcw aria-hidden="true" />
    </ActionButton>
  );
}
