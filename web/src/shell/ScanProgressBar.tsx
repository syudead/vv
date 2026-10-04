import { t } from "../i18n";
import { cn } from "../lib/cn";
import type { ScanPresentation } from "./scanPresentation";

/**
 * ScanProgressBar は本数による1つの進み具合である。数字の出ない状態（開始中・finding）
 * では `aria-valuenow` を持たない不確定のバーにし、対象が 0 本の完了では何も出さない
 * （specs/024-import-progress/ui-design.md「Accessibility」）。
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
    const percent = Math.round((videos.settled / videos.total) * 100);
    return (
      <div
        role="progressbar"
        aria-label={t.shell.scan.progress}
        aria-valuemin={0}
        aria-valuemax={videos.total}
        aria-valuenow={videos.settled}
        aria-valuetext={t.shell.scan.videosDone(videos.settled, videos.total)}
        className={cn("overflow-hidden rounded-full bg-background", className)}
      >
        <div
          className="h-full bg-primary transition-[width] duration-300"
          style={{ width: `${String(percent)}%` }}
        />
      </div>
    );
  }

  if (presentation.bar !== "indeterminate") return null;
  return (
    <div
      role="progressbar"
      aria-label={t.shell.scan.progress}
      className={cn("overflow-hidden rounded-full bg-background", className)}
    >
      <div className="h-full w-1/3 animate-pulse bg-primary motion-reduce:animate-none" />
    </div>
  );
}
