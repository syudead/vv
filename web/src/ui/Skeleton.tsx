import { cn } from "../lib/cn";

export default function Skeleton({ className }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={cn(
        "rounded-md bg-surface bg-[linear-gradient(90deg,transparent_0%,var(--color-surface-hover)_50%,transparent_100%)] bg-size-[200%_100%] animate-shimmer",
        className,
      )}
    />
  );
}
