import { useLayoutEffect, useState } from "react";

import { cn } from "../../lib/cn";
import { drawComposition } from "../thumbnailUpload/composeImage";
import type { FramedCover } from "./coverFraming";

/** The cover drawn as the thumbnail it becomes; place it inside a VideoThumbnail. */
export default function FramedCoverCanvas({
  cover,
  label,
  className,
}: {
  cover: FramedCover;
  label: string;
  className?: string;
}) {
  const [canvas, setCanvas] = useState<HTMLCanvasElement | null>(null);
  const { image, aspect, fit } = cover;
  useLayoutEffect(() => {
    if (canvas !== null) drawComposition(canvas, image, aspect, fit);
  }, [canvas, image, aspect, fit]);
  return (
    <canvas
      ref={setCanvas}
      role="img"
      aria-label={label}
      title={label}
      className={cn("relative size-full object-contain", className)}
    />
  );
}
