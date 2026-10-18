import { t } from "../i18n";
import { Progress } from "../ui/shadcn/progress";
import type { ScanPresentation } from "./scanPresentation";

/** Use indeterminate progress without numeric counts; hide the bar for zero-video completion. */
export default function ScanProgressBar({
  presentation,
  className,
}: {
  presentation: ScanPresentation;
  className?: string;
}) {
  const { videos } = presentation;
  if (presentation.bar === "determinate" && videos !== null) {
    return (
      <Progress
        aria-label={t.shell.scan.progress}
        value={videos.settled}
        max={videos.total}
        getValueLabel={(value, max) => t.shell.scan.videosDone(value, max)}
        className={className}
      />
    );
  }

  if (presentation.bar !== "indeterminate") return null;
  return (
    <Progress aria-label={t.shell.scan.progress} value={null} className={className} />
  );
}
