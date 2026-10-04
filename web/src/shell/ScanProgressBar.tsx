import { t } from "../i18n";
import { Progress } from "../ui/shadcn/progress";
import type { ScanPresentation } from "./scanPresentation";

/**
 * ScanProgressBar は本数による1つの進み具合である。数字の出ない状態（開始中・finding）
 * では `aria-valuenow` を持たない不確定のバーにし、対象が 0 本の完了では何も出さない
 * （specs/024-import-progress/ui-design.md「Accessibility」）。shadcn/ui の Progress で描く。
 */
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
