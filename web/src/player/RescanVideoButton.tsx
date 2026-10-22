import { RefreshCw } from "lucide-react";
import { useId } from "react";

import { t, type UiText } from "../i18n";
import { useScanControlsIfProvided } from "../shell/ScanProvider";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useToast } from "../ui/Toast";

/**
 * Starts a force rescan of one video without confirmation. Blocked states stay focusable with
 * aria-disabled so the tooltip and the description can give the reason.
 */
export default function RescanVideoButton({
  videoId,
  report,
}: {
  videoId: number;
  /** Receives null when starting and the failure otherwise, for the panel's failure line. */
  report: (failure: UiText | null) => void;
}) {
  const controls = useScanControlsIfProvided();
  const toast = useToast();
  const reasonId = useId();
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
    const started = () => {
      report(null);
      toast(f.rescanStarted);
    };
    report(null);
    void controls.rescan({ scope: "video", videoId }, started).then((failure) => {
      if (failure === null) started();
      else report(t.shell.scan.rescanFailed(failure));
    });
  };

  const button = (
    <Button
      variant="outline"
      size="sm"
      aria-disabled={blocked || undefined}
      aria-describedby={reason === null ? undefined : reasonId}
      onClick={start}
    >
      <RefreshCw aria-hidden="true" />
      {f.rescan}
    </Button>
  );
  if (reason === null) return button;
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent>{reason}</TooltipContent>
      </Tooltip>
      <span id={reasonId} hidden>
        {reason}
      </span>
    </>
  );
}
