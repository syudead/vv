import { cn } from "@/lib/cn";

// See web/registry/rules/components.md, KindMark.

/** One of the eight kind colours, `kind-1` … `kind-8`. */
export type KindColour = 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8;

/** A kind's colour from its id, so it keeps its colour when other kinds come and go. */
export function kindColour(id: number): KindColour {
  return (((((Math.trunc(id) - 1) % 8) + 8) % 8) + 1) as KindColour;
}

// Literal class names, so Tailwind generates every colour.
const fill: Record<KindColour, string> = {
  1: "bg-kind-1",
  2: "bg-kind-2",
  3: "bg-kind-3",
  4: "bg-kind-4",
  5: "bg-kind-5",
  6: "bg-kind-6",
  7: "bg-kind-7",
  8: "bg-kind-8",
};

export interface KindMarkProps {
  /** The kind's id. */
  kind: number;
  /** `bar`: the start edge of a `RecordCard`; `dot`: before the kind's name. */
  variant: "bar" | "dot";
}

/** A kind's colour; decorative, so the kind's name must stand beside it. */
export function KindMark({ kind, variant }: KindMarkProps) {
  const colour = kindColour(kind);
  return (
    <span
      aria-hidden="true"
      data-slot="kind-mark"
      data-variant={variant}
      data-kind-colour={colour}
      className={cn(
        fill[colour],
        variant === "bar"
          ? "absolute inset-y-0 start-0 w-1"
          : "inline-block size-2 shrink-0 rounded-full",
      )}
    />
  );
}
