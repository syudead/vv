import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { favoriteMark, subscribeFavorites } from "../api/favorites";
import { type Home, type HomeShelfKey, readHome, readHomeShelves } from "../api/home";
import { subscribeServerEvents } from "../api/serverEvents";
import { errorText, type UiText } from "../i18n";
import {
  loadedShelfKeys,
  nextShelfBatch,
  shelfBatchSize,
  type ShelfStates,
  withFavorites,
  withFavoritesSince,
  withShelfItems,
  withShelfStatus,
} from "./homeShelves";

// ホームの読み取り（specs/057-home/research.md R-12・R-16）。棚の一覧を読み、棚の項目は
// 描いた先頭の棚が画面に入るたびに、その棚から画面に入る分とその次の 2 つを読む。`home`・
// `smartFolder`・`video`・`group` の知らせ（サーバーが棚の控えを捨てる知らせと同じ。
// internal/store/home_shelf_cache.go）で一覧と読んだ棚を読み直し、読み直す間は今の行を
// 出したままにする。一覧も棚も、最後に頼んだ読み込みの応答だけを当て、先に頼んだ古い応答が
// 後から届いても新しい行を戻さない。

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

  // shelfRequests は棚ごとの、最後に頼んだ読み込みの通し番号である。
  const shelfRequests = useRef(new Map<HomeShelfKey, number>());
  const shelfSequence = useRef(0);

  // loadShelves は `keys` の棚の項目を読む。`mode` が "quiet" の読み直しは今の行を残し、
  // 失敗しても前の項目のままにする。"retry" は失敗した棚の読み直しで、その行を描いたままにする。
  // 応答は、その棚に後から別の読み込みを頼んでいない鍵にだけ当てる。
  const loadShelves = useCallback(
    async (keys: readonly HomeShelfKey[], mode: "load" | "quiet" | "retry") => {
      if (keys.length === 0) return;
      shelfSequence.current += 1;
      const sequence = shelfSequence.current;
      for (const key of keys) shelfRequests.current.set(key, sequence);
      const latest = () =>
        keys.filter((key) => shelfRequests.current.get(key) === sequence);
      if (mode !== "quiet") {
        const loading = { status: "loading", retry: mode === "retry" } as const;
        updateShelves((states) => withShelfStatus(states, keys, loading));
      }
      const mark = favoriteMark();
      try {
        const items = await readHomeShelves(keys, controller.current.signal);
        const current = latest();
        const corrected = withFavoritesSince(items, mark);
        updateShelves((states) => withShelfItems(states, current, corrected));
      } catch (error) {
        if (isAborted(error)) return;
        const reason = errorText(error);
        // 静かな読み直しの失敗は、項目のある棚を前のまま残す。まだ項目の無い棚は失敗にする。
        updateShelves((states) =>
          withShelfStatus(
            states,
            latest().filter(
              (key) => mode !== "quiet" || states.get(key)?.status !== "ready",
            ),
            { status: "failed", reason },
          ),
        );
      }
    },
    [updateShelves],
  );

  // listRequest は最後に頼んだ一覧の読み込みの通し番号である。
  const listRequest = useRef(0);

  // loadList は棚の一覧を読む。`quiet` の読み直しは今の一覧を残す。後から別の読み込みを
  // 頼んでいれば、この応答は当てない。
  const loadList = useCallback(
    async (quiet: boolean) => {
      listRequest.current += 1;
      const sequence = listRequest.current;
      if (!quiet) updateList({ status: "loading" });
      try {
        const home = await readHome(controller.current.signal);
        if (sequence === listRequest.current) updateList({ status: "ready", home });
      } catch (error) {
        if (isAborted(error) || sequence !== listRequest.current) return;
        if (!quiet) updateList({ status: "failed", reason: errorText(error) });
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
    void loadShelves(keys, "load");
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
        await loadList(listRef.current.status === "ready");
        const current = listRef.current;
        if (current.status !== "ready") continue;
        const batches = loadedShelfKeys(current.home.shelves, shelvesRef.current);
        await Promise.all(batches.map((keys) => loadShelves(keys, "quiet")));
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
      video: () => void refresh(),
      group: () => void refresh(),
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
    (key: HomeShelfKey) => void loadShelves([key], "retry"),
    [loadShelves],
  );

  return { list, shelves, loadMore, retryList, retryShelf };
}
