import { CircleDashed } from "lucide-react";

/**
 * Callers provide tentative accessible text; this nonshrinking visual mark stays hidden from
 * assistive technology.
 */
export default function TentativeMark({ size = "chip" }: { size?: "chip" | "row" }) {
  return (
    <CircleDashed
      className={`${size === "chip" ? "size-3" : "size-4"} shrink-0 text-muted-foreground`}
      aria-hidden="true"
    />
  );
}
