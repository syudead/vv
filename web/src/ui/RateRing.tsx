import { cn } from "@/lib/cn";

// See web/registry/rules/components.md, RateRing.

export type RateRingTone = "primary" | "muted";

export interface RateRingProps {
  /** A rate from 0 to 100; drawn and shown as the nearest whole percent. */
  value: number;
  tone?: RateRingTone;
}

const arcClass: Record<RateRingTone, string> = {
  primary: "stroke-primary",
  muted: "stroke-muted-foreground",
};

/** Clamps a rate to 0–100 and rounds it to the whole percent the ring shows. */
export function wholePercent(value: number): number {
  return Math.round(Math.min(100, Math.max(0, value)));
}

/** A ring whose arc is a rate of 100 with the whole percent inside; not a control. */
export function RateRing({ value, tone = "primary" }: RateRingProps) {
  const percent = wholePercent(value);
  return (
    <span
      data-slot="rate-ring"
      data-tone={tone}
      className="relative inline-flex size-12 shrink-0 items-center justify-center"
    >
      <svg viewBox="0 0 48 48" aria-hidden="true" className="absolute inset-0 -rotate-90">
        <circle
          cx="24"
          cy="24"
          r="21"
          fill="none"
          strokeWidth="4"
          className="stroke-muted"
        />
        {percent > 0 && (
          <circle
            data-slot="rate-ring-arc"
            cx="24"
            cy="24"
            r="21"
            fill="none"
            strokeWidth="4"
            pathLength={100}
            strokeDasharray={`${String(percent)} 100`}
            className={cn(arcClass[tone])}
          />
        )}
      </svg>
      <span className="relative text-sm font-semibold text-foreground tabular-nums">
        {percent}%
      </span>
    </span>
  );
}
