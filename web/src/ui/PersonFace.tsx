import { UserRound } from "lucide-react";
import { type ReactNode, useState } from "react";

import { cn } from "@/lib/cn";

// See web/registry/rules/components.md, PersonFace.

export type PersonFaceShape =
  "card" | "profile" | "chip" | "token" | "option" | "compare" | "judge" | "scene";

const frames: Record<PersonFaceShape, string> = {
  card: "aspect-square w-full rounded-full",
  profile: "aspect-portrait w-full rounded-md",
  chip: "size-5 rounded-full",
  token: "size-6 rounded-full",
  option: "size-8 rounded-full",
  compare: "size-8 rounded-full",
  judge: "size-card-face rounded-full",
  scene: "aspect-square w-full rounded-md",
};

const icons: Record<PersonFaceShape, string> = {
  card: "size-1/3",
  profile: "size-1/3",
  chip: "size-3",
  token: "size-3.5",
  option: "size-4",
  compare: "size-4",
  judge: "size-1/3",
  scene: "size-1/3",
};

export interface PersonFaceProps {
  /** The face's URL; without one, or when it fails to load, the placeholder shows. */
  src?: string;
  /** The person's name, for the image's alternative text. */
  name: string;
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
  shape,
  addLabel,
  onAdd,
  children,
}: PersonFaceProps) {
  // Remember which URL failed so that a new revision is tried again.
  const [failed, setFailed] = useState<string | null>(null);
  const shown = src !== undefined && failed !== src ? src : undefined;
  const icon = (
    <UserRound aria-hidden="true" className={cn("text-muted-foreground", icons[shape])} />
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
