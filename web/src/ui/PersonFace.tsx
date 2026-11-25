import { UserRound } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

import { FrameImage, FrameMark } from "./ImageFrame";

// See web/registry/rules/components.md, PersonFace.

export type PersonFaceShape =
  | "card"
  | "profile"
  | "chip"
  | "token"
  | "option"
  | "compare"
  | "judge"
  | "scene"
  | "suggestion"
  | "mark";

const frames: Record<PersonFaceShape, string> = {
  card: "aspect-square w-full rounded-full",
  profile: "aspect-portrait w-full rounded-md",
  chip: "size-5 rounded-full",
  token: "size-6 rounded-full",
  option: "size-8 rounded-full",
  compare: "size-8 rounded-full",
  judge: "size-card-face rounded-full",
  scene: "aspect-square w-full rounded-md",
  suggestion: "size-16 rounded-full",
  mark: "size-5 rounded-full ring-1 ring-foreground/30",
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
  suggestion: "size-6",
  mark: "size-3",
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
  return (
    <div
      data-slot="person-face"
      data-shape={shape}
      className={cn("relative shrink-0 overflow-hidden bg-muted", frames[shape])}
    >
      <FrameImage
        src={src}
        alt={name}
        imageClassName="object-cover object-face"
        icon={
          <UserRound
            aria-hidden="true"
            className={cn("text-muted-foreground", icons[shape])}
          />
        }
        addLabel={addLabel}
        onAdd={onAdd}
      />
      {children}
    </div>
  );
}

/** A control or mark in a corner of a card or profile face. */
export const PersonFaceMark = FrameMark;
