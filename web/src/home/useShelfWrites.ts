import { useCallback, useEffect, useRef, useState } from "react";

import { readHome, setHomeShelves } from "../api/home";
import { errorText, t } from "../i18n";
import { useToast } from "../ui/Toast";
import {
  folderShelfIds,
  withShelfAt,
  withShelfMoved,
  withShelfRemoved,
} from "./shelfOrder";

// 所有者がホームの棚を足す・外す・動かす書き込み（specs/057-home/ui-design.md「Home」「Smart
// folder page and Settings」、research.md R-7）。ホームと、スマートフォルダのページの「More」が
// 使い、同じトーストを出す。書き込みは PUT /api/home/shelves にフォルダの棚の並びを全部送る。

/** ShelfFolder は棚にするスマートフォルダである。 */
export interface ShelfFolder {
  id: number;
  name: string;
}

/**
 * KnownOrder は画面に出ているフォルダの棚の並びである。ホームは見えている並びから次の並びを
 * 作る。省くと（スマートフォルダのページ）GET /api/home で今の並びを読んでから書く。
 */
export type KnownOrder = readonly number[] | undefined;

/** currentOrder は `known` か、無ければサーバーの今のフォルダの棚の並びである。 */
async function currentOrder(known: KnownOrder): Promise<readonly number[]> {
  return known ?? folderShelfIds((await readHome()).shelves);
}

/**
 * useShelfWrites は棚の書き込みと、その結果のトーストを持つ。書けたら `onWritten` を呼ぶ
 * （呼び手が読み直す。サーバーの `home` の知らせも届く）。失敗は理由のトーストにする。
 * `busy` は書き込みと、その後の `onWritten` の読み直しが終わるまでで、呼び手は棚の操作を
 * 無効にする。ホームは見えている並びから次の並びを作るので、読み直す前の古い並びで次を
 * 書かせない。
 */
export function useShelfWrites(onWritten: () => void | Promise<void>) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const written = useRef(onWritten);
  useEffect(() => {
    written.current = onWritten;
  }, [onWritten]);

  // write は次の並びを `next` で作って書く。書けたら true。
  const write = useCallback(
    async (next: () => Promise<readonly number[]>): Promise<boolean> => {
      setBusy(true);
      try {
        await setHomeShelves(await next());
      } catch (error) {
        toast(t.home.writeFailed(errorText(error)));
        setBusy(false);
        return false;
      }
      try {
        await written.current();
      } finally {
        setBusy(false);
      }
      return true;
    },
    [toast],
  );

  /** add は `folder` を最後のフォルダの棚の後に足す。 */
  const add = useCallback(
    async (folder: ShelfFolder, known?: KnownOrder) => {
      const done = await write(async () => {
        const ids = await currentOrder(known);
        return withShelfAt(ids, folder.id, ids.length);
      });
      if (done) toast(t.home.added(folder.name));
    },
    [toast, write],
  );

  /** remove は `folder` を棚から外し、外す前の位置に戻す「Undo」付きのトーストを出す。 */
  const remove = useCallback(
    async (folder: ShelfFolder, known?: KnownOrder) => {
      let index = -1;
      const done = await write(async () => {
        const removed = withShelfRemoved(await currentOrder(known), folder.id);
        index = removed.index;
        return removed.ids;
      });
      if (!done) return;
      toast(t.home.removed(folder.name), {
        action: {
          label: t.home.undo,
          run: async () => {
            // 戻す間に並びが変わっているかもしれないので、今の並びに戻す。
            await write(async () =>
              withShelfAt(await currentOrder(undefined), folder.id, index),
            );
          },
        },
      });
    },
    [toast, write],
  );

  /** move は `id` の棚を 1 つ上か下へ動かす。 */
  const move = useCallback(
    async (id: number, direction: -1 | 1, known: readonly number[]) => {
      await write(() => Promise.resolve(withShelfMoved(known, id, direction)));
    },
    [write],
  );

  return { add, remove, move, busy };
}
