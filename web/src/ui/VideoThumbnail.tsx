import type * as React from "react";

import { cn } from "@/lib/cn";
import { Progress } from "@/ui/shadcn/progress";

//
// Layer order: image, duration, progress, full-frame notice, corner marks.

function VideoThumbnail({
  className,
  selected = false,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  selected?: boolean;
}) {
  return (
    <div
      data-slot="video-thumbnail"
      data-selected={selected || undefined}
      className={cn(
        "group/video-thumbnail relative aspect-video w-full overflow-hidden rounded-md bg-navbar",
        "data-selected:ring-2 data-selected:ring-primary",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

function VideoThumbnailImage({
  className,
  alt = "",
  ...props
}: React.ComponentProps<"img">) {
  return (
    <img
      data-slot="video-thumbnail-image"
      alt={alt}
      loading="lazy"
      decoding="async"
      className={cn("relative size-full object-contain", className)}
      {...props}
    />
  );
}

function VideoThumbnailDuration({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="video-thumbnail-duration"
      className={cn(
        "absolute right-2 bottom-2 flex items-center gap-1.5 rounded-sm bg-overlay px-1.5 py-0.5 text-2xs font-medium text-foreground tabular-nums [&_svg]:size-3",
        className,
      )}
      {...props}
    />
  );
}

/** value is 0-100; callers supply aria-label. */
function VideoThumbnailProgress({
  className,
  ...props
}: React.ComponentProps<typeof Progress>) {
  return (
    <Progress
      data-slot="video-thumbnail-progress"
      className={cn("absolute inset-x-0 bottom-0 rounded-none bg-overlay", className)}
      {...props}
    />
  );
}

/** Give overlaid icons drop-shadow-mark for contrast over images. */
function VideoThumbnailMark({
  className,
  corner = "top-end",
  ...props
}: React.ComponentProps<"div"> & { corner?: "top-start" | "top-end" }) {
  return (
    <div
      data-slot="video-thumbnail-mark"
      data-corner={corner}
      className={cn(
        "absolute top-1.5 z-20 flex data-[corner=top-end]:right-1.5 data-[corner=top-start]:left-1.5 [&_svg]:drop-shadow-mark",
        className,
      )}
      {...props}
    />
  );
}

function VideoThumbnailNotice({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="video-thumbnail-notice"
      className={cn(
        "absolute inset-0 flex flex-col items-center justify-center gap-1.5 bg-overlay px-3 text-center text-xs font-medium text-warning [&_svg]:size-5",
        className,
      )}
      {...props}
    />
  );
}

export {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailMark,
  VideoThumbnailNotice,
  VideoThumbnailProgress,
};
