import { useEffect, useRef, useState } from "react";

import type { RejectedTagNameList } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import Button from "../ui/Button";
import Skeleton from "../ui/Skeleton";

/**
 * RejectedNames はタグ管理画面の「Rejected names」のタブの中身である
 * （specs/036-tag-admin-scale/ui-design.md「Rejected names tab」）。却下した名前を
 * 1 行ずつ並べ、行の「Allow again」でその名前を外す（規則は 031 のまま。
 * specs/031-tentative-tags/ui-design.md「Rejected names」）。一覧の取得・続き・
 * 取り直しは呼び出し元（TagsPage）が持ち、ここは行・取り外しと、並びの末尾が
 * 表示域に入ったことを伝える番兵を持つ。
 *
 * `page` が undefined の間は読み込み中（`error` があれば読み込み失敗）である。
 */
export default function RejectedNames({
  page,
  error,
  onRetry,
  morePending,
  moreError,
  onLoadMore,
  resetKey,
  onForget,
  onFocusFallback,
}: {
  page: RejectedTagNameList | undefined;
  error: UiText | null;
  onRetry: () => void;
  /** 続きの読み込みの送信中。 */
  morePending: boolean;
  /** 続きの読み込みの失敗。 */
  moreError: UiText | null;
  /** 続きを読む（失敗のあとの Retry も同じカーソルで読み直す）。 */
  onLoadMore: () => void;
  /** 先頭のページを受け直すたびに変わる。番兵を見張り直す。 */
  resetKey: number;
  /** 名前を外す。成功すれば呼び出し元が `page` からその名前を取り除く。 */
  onForget: (name: string) => Promise<void>;
  /** 最後の名前を外したときのフォーカスの行き先（「Rejected names」のタブ）。 */
  onFocusFallback: () => void;
}) {
  const names = page?.items;
  const hasMore = page?.nextCursor !== undefined;
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const onLoadMoreRef = useRef(onLoadMore);
  onLoadMoreRef.current = onLoadMore;
  const loadedCount = names?.length ?? 0;

  // 表示域を根にした番兵。並びの末尾が見えたら続きを読む。読み込みのたびに
  // 見張り直すので、届いた分で末尾がまだ見えていればもう 1 ページ読む。
  // 先頭のページを受け直したときも見張り直す（取り直しの間に見えた番兵の通知は
  // 呼び出し元が無視するので、件数が変わらなければ次の通知が来ない）。
  // 失敗の間は読まない（Retry を押すまで）。
  useEffect(() => {
    const target = sentinelRef.current;
    if (target === null || !hasMore || morePending || moreError !== null) return;
    if (typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onLoadMoreRef.current();
      },
      { rootMargin: "0px 0px 240px 0px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasMore, morePending, moreError, loadedCount, resetKey]);

  const [removing, setRemoving] = useState<ReadonlySet<string>>(new Set());
  const [removeError, setRemoveError] = useState<UiText | null>(null);
  const removeButtons = useRef(new Map<string, HTMLButtonElement>());
  const removingRef = useRef(new Set<string>());

  /**
   * forget は「Allow again」の1回の押下を扱う。送信中の名前のボタンは二度押しを
   * 無視する。成功したら、次の行の「Allow again」、無ければ前の行、最後の1つなら
   * 「Rejected names」のタブへフォーカスを移す。
   */
  async function forget(name: string) {
    if (removingRef.current.has(name) || names === undefined) return;
    const order = [...names];
    removingRef.current.add(name);
    setRemoving(new Set(removingRef.current));
    setRemoveError(null);
    try {
      await onForget(name);
      const index = order.indexOf(name);
      const rest = order.filter((item) => item !== name);
      const next = rest[Math.min(Math.max(index, 0), rest.length - 1)];
      setTimeout(() => {
        const target = next === undefined ? undefined : removeButtons.current.get(next);
        if (target === undefined) onFocusFallback();
        else target.focus();
      }, 0);
    } catch (failure) {
      setRemoveError(t.tags.rejectedNames.removeFailed(name, errorText(failure)));
      // 送信中に disabled にしたボタンはフォーカスを失うので、行を残したまま
      // そのボタンへ戻す。
      setTimeout(() => removeButtons.current.get(name)?.focus(), 0);
    } finally {
      removingRef.current.delete(name);
      setRemoving(new Set(removingRef.current));
    }
  }

  return (
    <div className="flex flex-col gap-3">
      {names === undefined && error === null && (
        <div className="space-y-2" aria-hidden="true">
          {Array.from({ length: 4 }, (_, index) => (
            <Skeleton key={index} className="h-10" />
          ))}
        </div>
      )}
      {names === undefined && error !== null && (
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-sm text-danger">{t.tags.rejectedNames.loadFailed}</p>
          <Button variant="ghost" size="sm" onClick={onRetry}>
            {t.common.retry}
          </Button>
        </div>
      )}
      {/*
        読み込んだ名前をすべて外しても続きが残っていれば、空の文言ではなく
        続きの番兵（失敗の間は Retry）を置く。空の文言は続きも無いときだけ。
      */}
      {names !== undefined && names.length === 0 && !hasMore && (
        <p className="py-6 text-center text-sm text-fg-muted">
          {t.tags.rejectedNames.empty}
        </p>
      )}
      {names !== undefined && (names.length > 0 || hasMore) && (
        <>
          <p className="text-sm text-fg-muted">{t.tags.rejectedNames.description}</p>
          <ul
            aria-label={t.tags.rejectedNames.heading}
            aria-busy={morePending || undefined}
            className="divide-y divide-border"
          >
            {names.map((name) => (
              <li
                key={name}
                title={name}
                className="flex min-h-12 items-center gap-3 px-2 py-1.5"
              >
                <span className="min-w-0 flex-1 truncate text-sm font-medium text-fg">
                  {name}
                </span>
                <Button
                  ref={(node) => {
                    if (node) removeButtons.current.set(name, node);
                    else removeButtons.current.delete(name);
                  }}
                  size="sm"
                  aria-label={t.tags.rejectedNames.allow(name)}
                  disabled={removing.has(name)}
                  onClick={() => void forget(name)}
                >
                  {t.tags.rejectedNames.allowShort}
                </Button>
              </li>
            ))}
          </ul>
          {morePending && (
            <div className="space-y-2" aria-hidden="true">
              {Array.from({ length: 3 }, (_, index) => (
                <Skeleton key={index} className="h-10" />
              ))}
            </div>
          )}
          {moreError !== null && (
            <div className="flex flex-wrap items-center gap-2">
              <p role="alert" className="text-sm text-danger">
                {t.tags.rejectedNames.loadMoreFailed}
              </p>
              <Button variant="ghost" size="sm" onClick={onLoadMore}>
                {t.common.retry}
              </Button>
            </div>
          )}
          {hasMore && <div ref={sentinelRef} aria-hidden="true" className="h-px" />}
        </>
      )}
      {removeError !== null && (
        <p role="alert" className="text-sm text-danger">
          {removeError}
        </p>
      )}
    </div>
  );
}
