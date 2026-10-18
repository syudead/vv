import { useCallback, useEffect, useMemo, useState } from "react";

/** Coordinate one hover preview across cards and reset it on viewport resizing. */
export function usePreviewCoordination() {
  const [activePreviewId, setActivePreviewId] = useState<number | null>(null);
  const [previewResetEpoch, setPreviewResetEpoch] = useState(0);
  const resetPreview = useCallback(() => {
    setActivePreviewId(null);
    setPreviewResetEpoch((epoch) => epoch + 1);
  }, []);
  const startPreview = useCallback((id: number) => setActivePreviewId(id), []);

  useEffect(() => {
    const onResize = () => resetPreview();
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, [resetPreview]);

  const cardProps = useMemo(
    () => ({
      activePreviewId,
      previewResetEpoch,
      onPreviewStart: startPreview,
      onPreviewReset: resetPreview,
    }),
    [activePreviewId, previewResetEpoch, resetPreview, startPreview],
  );

  return { activePreviewId, previewResetEpoch, startPreview, resetPreview, cardProps };
}

export type PreviewCardProps = ReturnType<typeof usePreviewCoordination>["cardProps"];
