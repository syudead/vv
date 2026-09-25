import { useCallback, useEffect, useState } from "react";

import {
  type FolderListing,
  type FolderRef,
  getFolder,
  isAborted,
  listRootFolders,
  RequestFailed,
  type RootFolderListing,
} from "./client";

/** FolderListingState はフォルダ画面の子フォルダ一覧の状態である。 */
export interface FolderListingState<T> {
  data: T | null;
  loading: boolean;
  /** notFound はフォルダが存在しない（404）ことを表す。 */
  notFound: boolean;
  error: string | null;
  reload: () => void;
}

/**
 * useFolderData は1回の要求で取れるフォルダの情報を読む。画面を離れたら
 * 要求を取り消し、古い応答が新しい画面を上書きしないようにする。
 */
function useFolderData<T>(
  load: (signal: AbortSignal) => Promise<T>,
  key: string,
  seed?: T,
): FolderListingState<T> {
  const [data, setData] = useState<T | null>(seed ?? null);
  const [loading, setLoading] = useState(seed === undefined);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [generation, setGeneration] = useState(seed === undefined ? 1 : 0);

  useEffect(() => {
    // 0 番目の世代は復元した控え（seed）で、取りに行かない。
    if (generation === 0) return;
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    setNotFound(false);
    load(controller.signal)
      .then((value) => {
        setData(value);
        setLoading(false);
      })
      .catch((failure: unknown) => {
        if (isAborted(failure)) return;
        if (failure instanceof RequestFailed && failure.status === 404) {
          setNotFound(true);
        } else {
          setError(failure instanceof Error ? failure.message : String(failure));
        }
        setData(null);
        setLoading(false);
      });
    return () => controller.abort();
    // load は key が同じ間は同じ要求を表すので、依存は key で足りる。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [generation, key]);

  const reload = useCallback(() => setGeneration((value) => value + 1), []);
  return { data, loading, notFound, error, reload };
}

/** useRootFolders はフォルダ画面の最上位（登録済みメディアフォルダ）を読む。 */
export function useRootFolders(): FolderListingState<RootFolderListing> {
  return useFolderData(listRootFolders, "root");
}

/** useFolderListing はフォルダ1件と直下の子フォルダを読む。 */
export function useFolderListing(
  folder: FolderRef,
  seed?: FolderListing,
): FolderListingState<FolderListing> {
  return useFolderData(
    (signal) => getFolder(folder, signal),
    `${String(folder.rootId)}\0${folder.path}`,
    seed,
  );
}
