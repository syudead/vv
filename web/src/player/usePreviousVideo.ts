import { useEffect, useState } from "react";

import { getVideo, isAborted } from "../api/client";

export interface PreviousVideo {
  id: number;
  title: string;
}

/** Return an accessible previous video, or undefined while checking, missing, inaccessible, or failed. */
export function usePreviousVideo(id: number | undefined): PreviousVideo | undefined {
  const [found, setFound] = useState<PreviousVideo | undefined>(undefined);
  useEffect(() => {
    setFound(undefined);
    if (id === undefined) return;
    const controller = new AbortController();
    getVideo(id, controller.signal).then(
      (video) => {
        if (!controller.signal.aborted) setFound({ id, title: video.title });
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        setFound(undefined);
      },
    );
    return () => controller.abort();
  }, [id]);
  return found?.id === id ? found : undefined;
}
