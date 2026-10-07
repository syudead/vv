import { useCallback, useEffect, useRef, useState } from "react";

import {
  forgetRejectedTagName,
  listRejectedTagNamePage,
  type RejectedTagNameList,
} from "../api/tags";
import { errorText, type UiText } from "../i18n";

/**
 * rejectedPageLimit は却下した名前の 1 ページの件数である（ui-design.md
 * 「Rejected names」の 100 件。research.md R-13）。
 */
const rejectedPageLimit = 100;

/** RejectedNamesState はタグ管理画面の「Rejected names」のタブが描く状態と操作である。 */
export interface RejectedNamesState {
  /**
   * 先頭から読み込んだ分（`items`）と、全部の数（`total`。入口の件数）、続きの
   * カーソル（`nextCursor`）。undefined の間は読み込み中か、先頭のページの失敗。
   */
  page: RejectedTagNameList | undefined;
  error: UiText | null;
  /** 続きの読み込みの送信中と、その失敗。 */
  morePending: boolean;
  moreError: UiText | null;
  /** 先頭のページを受けるたびに 1 増える。窓が開いていればスクロール位置を先頭へ戻す。 */
  epoch: number;
  reload: () => void;
  loadMore: () => void;
  forget: (name: string) => Promise<void>;
}

/**
 * useRejectedNames は却下した名前の一覧を読む（specs/036-tag-admin-scale/data-model.md
 * §4、research.md R-13）。この画面だけが読むので、共有の保持ではなくここで取る
 * （031 の research.md R-8）。開いたときに先頭のページを読む。
 */
export function useRejectedNames(): RejectedNamesState {
  const [page, setPage] = useState<RejectedTagNameList | undefined>(undefined);
  const [error, setError] = useState<UiText | null>(null);
  const [morePending, setMorePending] = useState(false);
  const [moreError, setMoreError] = useState<UiText | null>(null);
  const [epoch, setEpoch] = useState(0);
  const generation = useRef(0);
  /** 取り直しの応答を待っている世代。待っていなければ null。 */
  const inFlight = useRef<number | null>(null);
  /** 続きの要求を送っている間 true（同時に 1 つだけ送る）。 */
  const moreInFlight = useRef(false);
  /**
   * 先頭のページを受けたあとに × で外した名前。外す前に送った続きの応答に
   * 載っていても並びに戻さない。
   */
  const forgotten = useRef(new Set<string>());

  /**
   * reload は先頭の 1 ページを取り直す。却下・作成・改名・シノニムの追加のあと
   * （どれも一覧を変えうる。要件 15、受け入れ条件 14）と、画面を開いたときに呼ぶ。
   * 読み込んだ続きは捨て、先頭の 1 ページに戻す（ui-design.md「Rejected names」）。
   * 追い越された古い取得の結果（続きも含む）は捨てる。取り直しの失敗は、最初の
   * 読み込みの失敗と同じ見え方（件数を出さず、「Couldn't load the rejected names」と
   * Retry）にする。古い一覧を黙って残すと、却下や作成で変わったはずの並びを
   * 正しいものとして見せ続けてしまう。
   */
  const reload = useCallback(() => {
    generation.current += 1;
    const current = generation.current;
    inFlight.current = current;
    moreInFlight.current = false;
    setError(null);
    setMorePending(false);
    setMoreError(null);
    listRejectedTagNamePage(undefined, rejectedPageLimit)
      .then((first) => {
        if (current !== generation.current) return;
        inFlight.current = null;
        forgotten.current = new Set();
        setPage(first);
        setEpoch((value) => value + 1);
      })
      .catch((failure: unknown) => {
        if (current !== generation.current) return;
        inFlight.current = null;
        setPage(undefined);
        setError(errorText(failure));
      });
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  /**
   * loadMore は窓の中身を末尾までスクロールしたときに、続きの 1 ページを読んで
   * 並びの末尾に足す。同時に 1 つだけ送る。失敗しても読み込んだ名前は残し、Retry は
   * 同じカーソルで読み直す。入口の件数は先頭のページの `total` から外した数を
   * 引いたままにする（続きの応答で上書きすると、送ったあとの取り外しの分がずれる）。
   */
  function loadMore() {
    const cursor = page?.nextCursor;
    if (cursor === undefined || moreInFlight.current) return;
    if (inFlight.current !== null) return;
    const current = generation.current;
    moreInFlight.current = true;
    setMorePending(true);
    setMoreError(null);
    listRejectedTagNamePage(cursor, rejectedPageLimit)
      .then((next) => {
        if (current !== generation.current) return;
        moreInFlight.current = false;
        setMorePending(false);
        setPage((latest) => {
          if (latest === undefined) return latest;
          const known = new Set(latest.items);
          const added = next.items.filter(
            (name) => !known.has(name) && !forgotten.current.has(name),
          );
          return {
            items: [...latest.items, ...added],
            total: latest.total,
            ...(next.nextCursor === undefined ? {} : { nextCursor: next.nextCursor }),
          };
        });
      })
      .catch((failure: unknown) => {
        if (current !== generation.current) return;
        moreInFlight.current = false;
        setMorePending(false);
        setMoreError(errorText(failure));
      });
  }

  /**
   * forget は × の取り外しである。`204` でそのチップを消し、入口の件数を 1 減らす
   * （一覧は取り直さない）。取り外しの送信中に先頭のページの取り直しが重なったとき
   * （送る前から待っていた・送信中に始まった）は、その応答が取り外しの前か後かが
   * 分からない。外した名前が先頭のページの外にあると、局所の 1 減らしもできない。
   * そのため、待っている応答は捨て、取り外しのあとで取り直す。
   */
  async function forget(name: string) {
    const started = generation.current;
    const refreshing = inFlight.current !== null;
    await forgetRejectedTagName(name);
    const overlapped =
      refreshing || started !== generation.current || inFlight.current !== null;
    forgotten.current.add(name);
    setPage((current) => {
      if (current === undefined || !current.items.includes(name)) return current;
      return {
        ...current,
        items: current.items.filter((item) => item !== name),
        total: Math.max(current.total - 1, 0),
      };
    });
    if (overlapped) reload();
  }

  return { page, error, morePending, moreError, epoch, reload, loadMore, forget };
}
