import { cn } from "../lib/cn";
import type { ScanPresentation } from "./scanPresentation";

function isIndeterminate(presentation: ScanPresentation) {
  return (
    presentation.state === "starting" ||
    presentation.state === "unknown-total" ||
    presentation.state === "preparing" ||
    (presentation.state === "failed" && presentation.progress === null)
  );
}

export default function ScanProgressBar({
  presentation,
  className,
}: {
  presentation: ScanPresentation;
  className?: string;
}) {
  if (presentation.determinate && presentation.progress !== null) {
    const value = Math.round(presentation.progress * 100);
    return (
      <div
        role="progressbar"
        aria-label="取り込みの進捗"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={value}
        className={cn("overflow-hidden rounded-full bg-bg", className)}
      >
        <div
          className="h-full bg-accent transition-[width] duration-300"
          style={{ width: `${String(value)}%` }}
        />
      </div>
    );
  }

  if (!isIndeterminate(presentation)) return null;
  return (
    <div
      role="progressbar"
      aria-label={
        presentation.state === "preparing"
          ? "取り込んだ動画を準備中"
          : "取り込み対象を確認中"
      }
      className={cn("overflow-hidden rounded-full bg-bg", className)}
    >
      <div className="h-full w-1/3 animate-pulse bg-accent motion-reduce:animate-none" />
    </div>
  );
}
