import type { ReactNode } from "react";

import { type CreditKind, personFaceSrc } from "../api/credits";
import { PersonFace, type PersonFaceShape } from "../ui/PersonFace";

/**
 * A person's face beside their name where people are chosen; organizations have none. The name is
 * already the row's text, so the image is decorative; a person not yet created shows the placeholder.
 */
export function creditFace(
  kind: CreditKind,
  credit: { id: number; imageRevision?: number } | undefined,
  shape: Extract<PersonFaceShape, "option" | "token">,
): ReactNode {
  if (kind !== "person") return undefined;
  return (
    <PersonFace
      shape={shape}
      name=""
      src={credit === undefined ? undefined : personFaceSrc(credit)}
    />
  );
}
