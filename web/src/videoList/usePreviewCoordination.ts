import { useCallback, useEffect, useMemo, useState } from "react";

/**
 * usePreviewCoordination は一覧のホバープレビューを同時に 1 件に絞る状態である。
 * 返す値をそのまま VideoCard の `activePreviewId`・`previewResetEpoch`・
 * `onPreviewStart`・`onPreviewReset` に渡す。窓の大きさが変わったら止める。
 */
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

  // VideoCard の props の名前でまとめたもの（そのまま広げて渡せる）。
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

/** PreviewCardProps は VideoCard へ広げて渡すプレビューの調整の props である。 */
export type PreviewCardProps = ReturnType<typeof usePreviewCoordination>["cardProps"];
