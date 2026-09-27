import { t } from "../i18n";
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
        aria-label={t.shell.scan.progress}
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
          ? t.shell.scan.progressPreparing
          : t.shell.scan.progressChecking
      }
      className={cn("overflow-hidden rounded-full bg-bg", className)}
    >
      <div className="h-full w-1/3 animate-pulse bg-accent motion-reduce:animate-none" />
    </div>
  );
}
