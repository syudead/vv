import type { Ref } from "react";

/** The box useMarqueeSelection draws; hidden until a drag starts. */
export function MarqueeBox({ ref }: { ref: Ref<HTMLDivElement> }) {
  return (
    <div
      ref={ref}
      hidden
      aria-hidden="true"
      className="pointer-events-none fixed z-40 rounded-sm border border-primary bg-primary-soft/40"
    />
  );
}
