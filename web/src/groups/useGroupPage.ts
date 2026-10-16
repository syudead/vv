import { useCallback, useEffect, useRef } from "react";

import type { GroupDetail } from "../api/client";
import { type GroupItem, listGroupItems, readGroupPage } from "../api/groups";
import { readThrough } from "../api/readThrough";
import { subscribeServerEvents } from "../api/serverEvents";
import { type PagedDetailState, usePagedDetail } from "../api/usePagedDetail";
import { appendItems } from "./groupItems";

/** GroupPageData は読めたグループのページの中身である。 */
export interface GroupPageData {
  group: GroupDetail["group"];
  trail: GroupDetail["trail"];
  items: readonly GroupItem[];
  /** 続きがあるときだけ入る。 */
  nextCursor?: string;
}

export type GroupPageState = PagedDetailState<GroupPageData>;

/** videoRefreshDelayMs は、動画の知らせを読み直し 1 回にまとめる待ち時間である。 */
const videoRefreshDelayMs = 300;

/**
 * readGroupThrough はグループを読み、項目が `atLeast` 件そろうか続きが無くなるまで続きのページを
 * 辿る（specs/048-groups/ui-design.md「Edits that fail」）。
 */
async function readGroupThrough(
  id: number,
  trail: readonly number[],
  atLeast: number,
  signal: AbortSignal,
): Promise<GroupPageData> {
  const detail = await readGroupPage(id, trail, signal);
  const { items, nextCursor } = await readThrough(
    detail,
    (cursor) => listGroupItems(id, cursor, signal),
    appendItems,
    atLeast,
  );
  return { group: detail.group, trail: detail.trail, items, nextCursor };
}

/**
 * useGroupPage はグループのページ 1 つの読み込みである（specs/048-groups/ui-design.md
 * 「Group page」）。開いたときに読み、続きは `loadMore` で 1 ページずつ足す。このグループの id を
 * 運ぶ `group` の知らせ、子孫の動画の `video` の知らせ、つなぎ直しで、並んでいる分を保ったまま
 * 読み直す（research.md R-13）。
 *
 * 新しい読み込みを始めると、古い読み込みと続きのページの応答は捨てる。読み直しの失敗
 * （404 のほか）は並んでいる中身を残す。404 はグループが無くなったこととして見せる。
 *
 * 編集の単位は `update` で並んでいる中身を先に変え（並べ替え、外す、改名）、`confirmed` に
 * サーバーが最後に認めた並びを持つ。読み込みと続きのページはそれを読んだ並びで置き換える。
 * 並べ替えは `moves` の鎖で 1 つずつ送り、送るたびに `movesSent` を進める。静かな読み直しは
 * 鎖が答えてから読み、読む間に移動を送っていたらその結果を捨てて読み直す（移動の前の並びで
 * 置いた行を上書きしない）。
 */
export function useGroupPage(id: number | null, trail: readonly number[]) {
  /** サーバーが最後に認めた並び。並べ替えの失敗でここへ戻す。 */
  const confirmed = useRef<readonly GroupItem[]>([]);
  /** 並べ替えを置いた順に 1 つずつ送る鎖（useGroupEdits が足す）と、送った移動の数。 */
  const moves = useRef<Promise<void>>(Promise.resolve());
  const movesSent = useRef(0);
  /** 画面（id）が変わるたびに進む。鎖を待った読み直しが、離れた後に読まないように使う。 */
  const epoch = useRef(0);
  /** 読む間に移動を送った読み直しを、鎖が答えてからやり直す（refresh。read より後に決まる）。 */
  const requery = useRef<() => void>(() => undefined);
  const trailKey = trail.join(",");

  const { state, stateRef, more, read, loadMore, update, markGone, stop } =
    usePagedDetail(id, {
      read: (groupId: number, atLeast: number, signal: AbortSignal) =>
        readGroupThrough(
          groupId,
          trailKey === "" ? [] : trailKey.split(",").map(Number),
          atLeast,
          signal,
        ),
      more: (groupId: number, cursor: string, signal: AbortSignal) =>
        listGroupItems(groupId, cursor, signal),
      append: appendItems,
      beforeRead: (quiet: boolean) => {
        const sentBefore = movesSent.current;
        return (data: GroupPageData) => {
          if (quiet && movesSent.current !== sentBefore) {
            // 読む間に送った移動が、読んだ並びに入っていないかもしれない。
            requery.current();
            return false;
          }
          confirmed.current = data.items;
          return true;
        };
      },
      onMore: (items: readonly GroupItem[]) => {
        confirmed.current = appendItems(confirmed.current, items);
      },
    });

  const reload = useCallback(() => read(false), [read]);
  /** refresh は静かに読み直す。送っている移動が答えてから読む。 */
  const refresh = useCallback(() => {
    const ticket = epoch.current;
    void moves.current.then(() => {
      if (ticket === epoch.current) read(true);
    });
  }, [read]);
  useEffect(() => {
    requery.current = refresh;
  }, [refresh]);

  useEffect(() => {
    let videoTimer: ReturnType<typeof setTimeout> | undefined;
    // 知らせは読み込みの前に購読し、読んだ後に起きた変化を取りこぼさない。
    const unsubscribe = subscribeServerEvents({
      group: (ids) => {
        if (id !== null && ids.includes(id) && stateRef.current.kind !== "gone") {
          refresh();
        }
      },
      // カードのタグ・お気に入り・視聴状態は動画の知らせで変わる。子孫の動画の知らせは、
      // 続けて届く分をまとめて 1 回読み直す（ライブラリの一覧と同じく、最新の内容は取り直す）。
      video: (videoId) => {
        const current = stateRef.current;
        if (current.kind !== "ready" || !current.data.group.videoIds.includes(videoId)) {
          return;
        }
        clearTimeout(videoTimer);
        videoTimer = setTimeout(refresh, videoRefreshDelayMs);
      },
      open: (reconnected) => {
        if (reconnected && stateRef.current.kind === "ready") refresh();
      },
    });
    reload();
    return () => {
      epoch.current += 1;
      unsubscribe();
      clearTimeout(videoTimer);
      stop();
    };
  }, [id, refresh, reload, stateRef, stop]);

  return {
    state,
    more,
    reload,
    refresh,
    loadMore,
    update,
    confirmed,
    moves,
    movesSent,
    markGone,
  };
}
