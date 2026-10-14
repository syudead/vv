import { useCallback, useEffect, useRef, useState } from "react";

import { type GroupDetail, isAborted, RequestFailed } from "../api/client";
import { type GroupItem, listGroupItems, readGroupPage } from "../api/groups";
import { subscribeServerEvents } from "../api/serverEvents";
import { appendItems } from "./groupItems";

/** GroupPageData は読めたグループのページの中身である。 */
export interface GroupPageData {
  group: GroupDetail["group"];
  trail: GroupDetail["trail"];
  items: readonly GroupItem[];
  /** 続きがあるときだけ入る。 */
  nextCursor?: string;
}

export type GroupPageState =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "gone" }
  | { kind: "ready"; data: GroupPageData };

/** MorePageState は続きのページの読み込みの状態である。 */
export type MorePageState = "idle" | "loading" | "failed";

/** videoRefreshDelayMs は、動画の知らせを読み直し 1 回にまとめる待ち時間である。 */
const videoRefreshDelayMs = 300;

/** maxRefreshPages は読み直しで続きのページを辿る回数の上限である（100 件ずつ）。 */
const maxRefreshPages = 50;

/** isEmptyGroup は読めたグループの項目が 1 つも無い（続きも無い）ときに true を返す。 */
function isEmptyGroup(state: GroupPageState): boolean {
  return (
    state.kind === "ready" &&
    state.data.items.length === 0 &&
    state.data.nextCursor === undefined
  );
}

/** isGone は応答がグループの無いこと（404）を表すかを返す。 */
function isGone(error: unknown): boolean {
  return error instanceof RequestFailed && error.status === 404;
}

/**
 * readThrough はグループを読み、項目が `atLeast` 件そろうか続きが無くなるまで続きのページを
 * 辿る。読み直しで、それまで並んでいた分を減らさずに入れ替えるためである（スクロールの位置を
 * 保つ。specs/048-groups/ui-design.md「Edits that fail」）。
 */
async function readThrough(
  id: number,
  trail: readonly number[],
  atLeast: number,
  signal: AbortSignal,
): Promise<GroupPageData> {
  const detail = await readGroupPage(id, trail, signal);
  let items: readonly GroupItem[] = detail.items;
  let nextCursor = detail.nextCursor;
  for (let page = 0; page < maxRefreshPages; page += 1) {
    if (nextCursor === undefined || items.length >= atLeast) break;
    const more = await listGroupItems(id, nextCursor, signal);
    items = appendItems(items, more.items);
    nextCursor = more.nextCursor;
  }
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
 *
 * 所有者が最後の項目を外して空にしたグループは、読み直しで `404` になる（plan.md: 見える動画の
 * 無いグループは `404`）。空の中身を見せている間の静かな読み直しの `404` は、無くなったとは
 * 見せずに空の状態のままにする（ui-design.md「States」の Content → Empty。plan.md と
 * ui-design.md の食い違いは PR に未決の問いとして残した）。
 */
export function useGroupPage(id: number | null, trail: readonly number[]) {
  const [state, setState] = useState<GroupPageState>(
    id === null ? { kind: "gone" } : { kind: "loading" },
  );
  const [more, setMore] = useState<MorePageState>("idle");
  const stateRef = useRef(state);
  /** サーバーが最後に認めた並び。並べ替えの失敗でここへ戻す。 */
  const confirmed = useRef<readonly GroupItem[]>([]);
  const reading = useRef<AbortController | null>(null);
  const trailKey = trail.join(",");

  const show = useCallback((next: GroupPageState) => {
    stateRef.current = next;
    setState(next);
  }, []);

  /** read は読み込みを始める。quiet なら読み込み中を見せず、失敗しても中身を残す。 */
  const read = useCallback(
    (quiet: boolean) => {
      if (id === null) return;
      reading.current?.abort();
      const controller = new AbortController();
      reading.current = controller;
      const current = stateRef.current;
      const atLeast = current.kind === "ready" ? current.data.items.length : 0;
      if (!quiet) show({ kind: "loading" });
      setMore("idle");
      const trailIds = trailKey === "" ? [] : trailKey.split(",").map(Number);
      readThrough(id, trailIds, atLeast, controller.signal).then(
        (data) => {
          if (controller.signal.aborted) return;
          reading.current = null;
          confirmed.current = data.items;
          show({ kind: "ready", data });
        },
        (error: unknown) => {
          if (isAborted(error) || controller.signal.aborted) return;
          reading.current = null;
          if (isGone(error) && quiet && isEmptyGroup(stateRef.current)) return;
          if (isGone(error)) show({ kind: "gone" });
          else if (!quiet || stateRef.current.kind !== "ready") show({ kind: "failed" });
        },
      );
    },
    [id, show, trailKey],
  );

  const reload = useCallback(() => read(false), [read]);
  const refresh = useCallback(() => read(true), [read]);

  const loadMore = useCallback(() => {
    const current = stateRef.current;
    if (id === null || current.kind !== "ready" || reading.current !== null) return;
    const cursor = current.data.nextCursor;
    if (cursor === undefined) return;
    const controller = new AbortController();
    reading.current = controller;
    setMore("loading");
    listGroupItems(id, cursor, controller.signal).then(
      (page) => {
        if (controller.signal.aborted) return;
        reading.current = null;
        const latest = stateRef.current;
        if (latest.kind !== "ready") return;
        setMore("idle");
        confirmed.current = appendItems(confirmed.current, page.items);
        show({
          kind: "ready",
          data: {
            ...latest.data,
            items: appendItems(latest.data.items, page.items),
            nextCursor: page.nextCursor,
          },
        });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        reading.current = null;
        if (isGone(error)) {
          setMore("idle");
          show({ kind: "gone" });
          return;
        }
        setMore("failed");
      },
    );
  }, [id, show]);

  /** update は読めた中身を変える（編集の結果を先に見せる）。読めていなければ何もしない。 */
  const update = useCallback(
    (change: (data: GroupPageData) => GroupPageData) => {
      const current = stateRef.current;
      if (current.kind !== "ready") return;
      show({ kind: "ready", data: change(current.data) });
    },
    [show],
  );

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
      unsubscribe();
      clearTimeout(videoTimer);
      reading.current?.abort();
      reading.current = null;
    };
  }, [id, refresh, reload]);

  /** markGone はグループが無くなったこと（編集の窓が受けた 404）を見せる。 */
  const markGone = useCallback(() => {
    reading.current?.abort();
    reading.current = null;
    setMore("idle");
    show({ kind: "gone" });
  }, [show]);

  return { state, more, reload, refresh, loadMore, update, confirmed, markGone };
}
