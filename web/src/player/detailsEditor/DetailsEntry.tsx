import { useEffect, useRef } from "react";
import { Link } from "react-router";

import type { Video } from "../../api/client";
import { t } from "../../i18n";
import { videoEditPath } from "../../lib/videoEditPath";
import { Button } from "../../ui/shadcn/button";
import { handVideoToEditor } from "./handoff";

export interface DetailsLinkState {
  from: string;
  trail: number[];
}

/** Carry return/history state and hand off the current Video in memory; returning from the editor restores entry focus. */
export interface DetailsEntryProps {
  video: Video;
  owner: boolean;
  details?: { linkState: DetailsLinkState; focusEntry: boolean };
}

export default function DetailsEntry({ video, owner, details }: DetailsEntryProps) {
  const ref = useRef<HTMLAnchorElement | null>(null);
  const focused = useRef(false);
  const focusEntry = details?.focusEntry === true;

  useEffect(() => {
    if (!focusEntry || focused.current) return;
    focused.current = true;
    ref.current?.focus();
  }, [focusEntry]);

  if (!owner || details === undefined) return null;
  return (
    <Button variant="ghost" size="sm" asChild>
      <Link
        ref={ref}
        to={videoEditPath(video.id)}
        state={{ ...details.linkState, fromPage: true }}
        onClick={() => handVideoToEditor(video)}
      >
        {t.player.workInfo.edit}
      </Link>
    </Button>
  );
}
