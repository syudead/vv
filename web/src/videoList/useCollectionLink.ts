import { useState } from "react";
import type { LinkProps } from "react-router";

import type { FolderPreview } from "../api/client";
import { t } from "../i18n";
import { cardLinkState } from "./cardLink";

/** One native link follows the promoted sheet; touch and keyboard open the collection. */
export function useCollectionLink({
  to,
  backTo,
  name,
  label,
  videoPath,
  selectionMode = false,
  autoplay,
}: {
  to: string;
  backTo: string;
  name: string;
  label: string;
  videoPath: (id: number) => string;
  selectionMode?: boolean;
  autoplay?: boolean;
}) {
  const [front, setFront] = useState<FolderPreview | null>(null);
  const active = selectionMode ? null : front;
  const linkProps: Pick<
    LinkProps,
    "to" | "state" | "aria-label" | "onPointerLeave" | "onPointerDown" | "onKeyDown"
  > = {
    to: active === null ? to : videoPath(active.videoId),
    state: cardLinkState(backTo, active === null ? autoplay : true),
    "aria-label":
      active === null ? label : t.library.collection.playPreview(name, active.title),
    onPointerLeave: () => setFront(null),
    onPointerDown: (event) => {
      if (event.pointerType !== "mouse") setFront(null);
    },
    onKeyDown: () => setFront(null),
  };
  return { linkProps, onFrontChange: setFront };
}
