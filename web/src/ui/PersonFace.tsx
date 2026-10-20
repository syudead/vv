import { UserRound } from "lucide-react";
import { type ReactNode, useState } from "react";

import { cn } from "@/lib/cn";

// See web/registry/rules/components.md, PersonFace.

export type PersonFaceShape = "card" | "profile" | "chip" | "compare" | "judge" | "scene";

const frames: Record<PersonFaceShape, string> = {
  card: "aspect-square w-full rounded-full",
  profile: "aspect-portrait w-full rounded-md",
  chip: "size-5 rounded-full",
  compare: "size-8 rounded-full",
  judge: "size-12 rounded-full",
  scene: "size-12 rounded-md",
};

const icons: Record<PersonFaceShape, string> = {
  card: "size-1/3",
  profile: "size-1/3",
  chip: "size-3",
  compare: "size-4",
  judge: "size-5",
  scene: "size-5",
};

export interface PersonFaceProps {
  /** The face's URL; without one, or when it fails to load, the placeholder shows. */
  src?: string;
  /** The person's name, for the image's alternative text. */
  name: string;
  /** Letters the placeholder shows in place of the icon, scaled to the frame. */
  initials?: string;
  shape: PersonFaceShape;
  /** Makes the placeholder a button with this name, such as "Add a face". */
  addLabel?: string;
  onAdd?: () => void;
  /** Marks drawn over the frame, placed with PersonFaceMark. */
  children?: ReactNode;
}

export function PersonFace({
  src,
  name,
  initials = "",
  shape,
  addLabel,
  onAdd,
  children,
}: PersonFaceProps) {
  // Remember which URL failed so that a new revision is tried again.
  const [failed, setFailed] = useState<string | null>(null);
  const shown = src !== undefined && failed !== src ? src : undefined;
  const icon =
    initials === "" ? (
      <UserRound
        aria-hidden="true"
        className={cn("text-muted-foreground", icons[shape])}
      />
    ) : (
      // An SVG viewBox scales the letters with the frame at every card size.
      <svg
        aria-hidden="true"
        viewBox="0 0 100 100"
        className="size-full text-muted-foreground"
      >
        <text
          x="50"
          y="50"
          textAnchor="middle"
          dominantBaseline="central"
          fontSize={initials.length > 1 ? 30 : 40}
          fill="currentColor"
        >
          {initials}
        </text>
      </svg>
    );
  return (
    <div
      data-slot="person-face"
      data-shape={shape}
      className={cn("relative shrink-0 overflow-hidden bg-muted", frames[shape])}
    >
      {shown !== undefined ? (
        <img
          src={shown}
          alt={name}
          loading="lazy"
          decoding="async"
          onError={() => setFailed(shown)}
          className="size-full object-cover object-face"
        />
      ) : onAdd !== undefined ? (
        <button
          type="button"
          aria-label={addLabel}
          onClick={onAdd}
          className="flex size-full items-center justify-center hover:bg-accent"
        >
          {icon}
        </button>
      ) : (
        <span className="flex size-full items-center justify-center">{icon}</span>
      )}
      {children}
    </div>
  );
}

/** A control or mark in a corner of a card or profile face. */
export function PersonFaceMark({
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
