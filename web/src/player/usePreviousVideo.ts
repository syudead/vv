import { useEffect, useState } from "react";

import { getVideo, isAborted } from "../api/client";

/** PreviousVideo は ‹ で戻れる、直前に見ていた動画である。 */
export interface PreviousVideo {
  id: number;
  title: string;
}

/**
 * usePreviousVideo は、直前に見ていた動画 `id` がまだ見られるかを `GET /api/videos/{id}` で
 * 確かめ、見られれば題名と合わせて返す（issue 931）。削除された・ゲストに見せない動画に
 * なった・確かめられなかったときと、確かめている間は undefined を返し、‹ を出さない。
 */
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
