import type { ReactNode, Ref } from "react";

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

/** A grid cell a drawn box can select, for cards that are not draggable. */
export function MarqueeCard({ id, children }: { id: string; children: ReactNode }) {
  return (
    <div data-card-key={id} className="grid min-w-0">
      {children}
    </div>
  );
}
