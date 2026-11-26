import { Building2 } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

import { FrameImage, FrameMark } from "./ImageFrame";

// See web/registry/rules/components.md, OrganizationLogo.

export type OrganizationLogoShape = "card" | "profile";

const frames: Record<OrganizationLogoShape, string> = {
  card: "aspect-video w-full rounded-md",
  profile: "aspect-square w-full rounded-md",
};

export interface OrganizationLogoProps {
  /** The logo's URL; without one, or when it fails to load, the placeholder shows. */
  src?: string;
  /** The image's alternative text, such as "Logo of {name}". */
  alt: string;
  shape: OrganizationLogoShape;
  /** The display name a `card` placeholder shows in place of the logo. */
  name?: string;
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
  name,
  addLabel,
  onAdd,
  children,
}: OrganizationLogoProps) {
  // Most cards have no logo, and their names tell them apart where one icon would not.
  const placeholder =
    shape === "card" && name !== undefined ? (
      <span className="line-clamp-3 px-3 text-center text-sm font-semibold break-words text-muted-foreground">
        {name}
      </span>
    ) : (
      <Building2 aria-hidden="true" className="size-1/3 text-muted-foreground" />
    );
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
        icon={placeholder}
        addLabel={addLabel}
        onAdd={onAdd}
      />
      {children}
    </div>
  );
}

/** A control or mark in a corner of the logo, held as a face's marks are. */
export const OrganizationLogoMark = FrameMark;
