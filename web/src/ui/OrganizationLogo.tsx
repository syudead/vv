import { Building2 } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

import { FrameImage, FrameMark } from "./ImageFrame";

// See web/registry/rules/components.md, OrganizationLogo.

export type OrganizationLogoShape = "profile";

const frames: Record<OrganizationLogoShape, string> = {
  profile: "aspect-square w-full rounded-md",
};

export interface OrganizationLogoProps {
  /** The logo's URL; without one, or when it fails to load, the placeholder shows. */
  src?: string;
  /** The image's alternative text, such as "Logo of {name}". */
  alt: string;
  shape: OrganizationLogoShape;
  /** Makes the placeholder a button with this name, such as "Add logo". */
  addLabel?: string;
  onAdd?: () => void;
  /** Marks drawn over the frame, placed with OrganizationLogoMark. */
  children?: ReactNode;
}

export function OrganizationLogo({
  src,
  alt,
  shape,
  addLabel,
  onAdd,
  children,
}: OrganizationLogoProps) {
  return (
    <div
      data-slot="organization-logo"
      data-shape={shape}
      className={cn(
        "relative shrink-0 overflow-hidden border border-border bg-muted",
        frames[shape],
      )}
    >
      <FrameImage
        src={src}
        alt={alt}
        // A cropped wordmark cannot be read, so the logo is drawn whole.
        imageClassName="object-contain"
        icon={<Building2 aria-hidden="true" className="size-1/3 text-muted-foreground" />}
        addLabel={addLabel}
        onAdd={onAdd}
      />
      {children}
    </div>
  );
}

/** A control or mark in a corner of the logo, held as a face's marks are. */
export const OrganizationLogoMark = FrameMark;
