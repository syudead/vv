import { type ReactNode, useState } from "react";

import { cn } from "@/lib/cn";

// The image-or-placeholder body and the corner marks PersonFace and OrganizationLogo share.

export interface FrameImageProps {
  /** The image's URL; without one, or when it fails to load, the placeholder shows. */
  src?: string;
  alt: string;
  /** How the image fills the frame, such as `object-cover` or `object-contain`. */
  imageClassName: string;
  /** The placeholder's icon. */
  icon: ReactNode;
  /** Makes the placeholder a button with this name. */
  addLabel?: string;
  onAdd?: () => void;
}

export function FrameImage({
  src,
  alt,
  imageClassName,
  icon,
  addLabel,
  onAdd,
}: FrameImageProps) {
  // Remember which URL failed so that a new revision is tried again.
  const [failed, setFailed] = useState<string | null>(null);
  const shown = src !== undefined && failed !== src ? src : undefined;
  if (shown !== undefined) {
    return (
      <img
        src={shown}
        alt={alt}
        loading="lazy"
        decoding="async"
        onError={() => setFailed(shown)}
        className={cn("size-full", imageClassName)}
      />
    );
  }
  if (onAdd !== undefined) {
    return (
      <button
        type="button"
        aria-label={addLabel}
        onClick={onAdd}
        className="flex size-full items-center justify-center hover:bg-accent"
      >
        {icon}
      </button>
    );
  }
  return <span className="flex size-full items-center justify-center">{icon}</span>;
}

/** A control or mark in a corner of a card or profile frame. */
export function FrameMark({
  corner,
  children,
}: {
  corner: "top-end" | "bottom-end";
  children: ReactNode;
}) {
  return (
    <div
      data-slot="person-face-mark"
      className={cn(
        "absolute right-1.5 z-20 flex [&_svg]:drop-shadow-mark",
        corner === "top-end" ? "top-1.5" : "bottom-1.5",
      )}
    >
      {children}
    </div>
  );
}
