import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { subscribeFavorites } from "../api/favorites";
import { type Home, type HomeShelfKey, readHome, readHomeShelves } from "../api/home";
import { subscribeServerEvents } from "../api/serverEvents";
import { errorText, type UiText } from "../i18n";
import {
  loadedShelfKeys,
  nextShelfBatch,
  shelfBatchSize,
  type ShelfStates,
  withFavorites,
  withShelfItems,
  withShelfStatus,
} from "./homeShelves";

// ホームの読み取り（specs/057-home/research.md R-12・R-16）。棚の一覧を読み、棚の項目は
// 描いた先頭の棚が画面に入るたびに、その棚から画面に入る分とその次の 2 つを読む。`home` と
// `smartFolder` の知らせで一覧と読んだ棚を読み直し、読み直す間は今の行を出したままにする。

/** HomeList は棚の一覧の読み込みの状態である。 */
export type HomeList =
  | { status: "loading" }
  | { status: "failed"; reason: UiText }
  | { status: "ready"; home: Home };

/** useHome はホームの棚の一覧と棚の項目を読み、知らせで読み直す。 */
export function useHome() {
  const [list, setList] = useState<HomeList>({ status: "loading" });
  const [shelves, setShelves] = useState<ShelfStates>(() => new Map());
  const listRef = useRef(list);
  const shelvesRef = useRef(shelves);
  const controller = useRef(new AbortController());
  const updateShelves = useCallback((update: (states: ShelfStates) => ShelfStates) => {
    shelvesRef.current = update(shelvesRef.current);
    setShelves(shelvesRef.current);
  }, []);
  const updateList = useCallback((next: HomeList) => {
    listRef.current = next;
    setList(next);
  }, []);

  // loadShelves は `keys` の棚の項目を読む。`quiet` の読み直しは今の行を残し、失敗しても
  // 前の項目のままにする。
  const loadShelves = useCallback(
    async (keys: readonly HomeShelfKey[], quiet: boolean) => {
      if (keys.length === 0) return;
      if (!quiet) {
        updateShelves((states) => withShelfStatus(states, keys, { status: "loading" }));
      }
      try {
        const items = await readHomeShelves(keys, controller.current.signal);
        updateShelves((states) => withShelfItems(states, keys, items));
      } catch (error) {
        if (isAborted(error) || quiet) return;
        const reason = errorText(error);
        updateShelves((states) =>
          withShelfStatus(states, keys, { status: "failed", reason }),
        );
      }
    },
    [updateShelves],
  );

  // loadList は棚の一覧を読む。`quiet` の読み直しは今の一覧を残す。
  const loadList = useCallback(
    async (quiet: boolean): Promise<boolean> => {
      if (!quiet) updateList({ status: "loading" });
      try {
        const home = await readHome(controller.current.signal);
        updateList({ status: "ready", home });
        return true;
      } catch (error) {
        if (isAborted(error)) return false;
        if (!quiet) updateList({ status: "failed", reason: errorText(error) });
        return false;
      }
    },
    [updateList],
  );

  // loadMore は描いた先頭の棚が画面に入ったときに、次の棚の項目を読む。
  const loadMore = useCallback(() => {
    const current = listRef.current;
    if (current.status !== "ready") return;
    const keys = nextShelfBatch(
      current.home.shelves,
      shelvesRef.current,
      shelfBatchSize(window.innerHeight),
    );
    void loadShelves(keys, false);
  }, [loadShelves]);

  // refresh は一覧と、もう読んだ棚を今の行を出したまま読み直す。読み直しの間に届いた
  // 知らせは、終わった後に 1 回だけ読み直す。
  const refreshing = useRef(false);
  const refreshAgain = useRef(false);
  const refresh = useCallback(async () => {
    if (refreshing.current) {
      refreshAgain.current = true;
      return;
    }
    refreshing.current = true;
    try {
      do {
        refreshAgain.current = false;
        if (!(await loadList(listRef.current.status === "ready"))) continue;
        const current = listRef.current;
        if (current.status !== "ready") continue;
        const batches = loadedShelfKeys(current.home.shelves, shelvesRef.current);
        await Promise.all(batches.map((keys) => loadShelves(keys, true)));
      } while (refreshAgain.current && !controller.current.signal.aborted);
    } finally {
      refreshing.current = false;
    }
  }, [loadList, loadShelves]);

  useEffect(() => {
    const current = new AbortController();
    controller.current = current;
    // 知らせを先に購読してから読む。読んだ後の変化を取りこぼさない。
    const unsubscribe = subscribeServerEvents({
      home: () => void refresh(),
      smartFolder: () => void refresh(),
      open: (reconnected) => {
        if (reconnected) void refresh();
      },
    });
    const unsubscribeFavorites = subscribeFavorites((change) =>
      updateShelves((states) => withFavorites(states, change)),
    );
    void loadList(false);
    return () => {
      current.abort();
      unsubscribe();
      unsubscribeFavorites();
    };
  }, [loadList, refresh, updateShelves]);

  const retryList = useCallback(() => void loadList(false), [loadList]);
  const retryShelf = useCallback(
    (key: HomeShelfKey) => void loadShelves([key], false),
    [loadShelves],
  );

  return { list, shelves, loadMore, retryList, retryShelf };
}
