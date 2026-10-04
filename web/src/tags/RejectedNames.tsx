import { Ban, CircleAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import type { RejectedTagNameList } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { DataTable } from "../ui/patterns/data-table";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { TableBody, TableCell, TableRow } from "../ui/shadcn/table";

/**
 * RejectedNames はタグ管理画面の「Rejected names」のタブの中身である
 * （specs/036-tag-admin-scale/ui-design.md「Rejected names tab」）。却下した名前を
 * 表（`DataTable`）に 1 行ずつ並べ、行の「Allow again」でその名前を外す（規則は 031 のまま。
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
    <>
      {names === undefined && error === null && (
        <LoadingState label={t.tags.loading} layout="table" count={4} />
      )}
      {names === undefined && error !== null && (
        <ErrorState
          title={t.tags.rejectedNames.loadFailed}
          retryLabel={t.common.retry}
          onRetry={onRetry}
        />
      )}
      {/*
        読み込んだ名前をすべて外しても続きが残っていれば、空の文言ではなく
        続きの番兵（失敗の間は Retry）を置く。空の文言は続きも無いときだけ。
      */}
      {names !== undefined && names.length === 0 && !hasMore && (
        <EmptyState
          icon={<Ban aria-hidden="true" />}
          title={t.tags.rejectedNames.empty}
        />
      )}
      {names !== undefined && (names.length > 0 || hasMore) && (
        <>
          <p className="text-sm text-muted-foreground">
            {t.tags.rejectedNames.description}
          </p>
          <DataTable label={t.tags.rejectedNames.heading}>
            <TableBody aria-busy={morePending || undefined}>
              {names.map((name) => (
                <TableRow key={name} title={name}>
                  <TableCell className="w-full max-w-0">
                    <span className="block truncate font-medium">{name}</span>
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      ref={(node) => {
                        if (node) removeButtons.current.set(name, node);
                        else removeButtons.current.delete(name);
                      }}
                      variant="outline"
                      size="sm"
                      aria-label={t.tags.rejectedNames.allow(name)}
                      disabled={removing.has(name)}
                      onClick={() => void forget(name)}
                    >
                      {t.tags.rejectedNames.allowShort}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </DataTable>
          {morePending && (
            <LoadMoreRow status="loading" label={t.common.loading} />
          )}
          {moreError !== null && (
            <LoadMoreRow
              status="failed"
              title={t.tags.rejectedNames.loadMoreFailed}
              retryLabel={t.common.retry}
              onRetry={onLoadMore}
            />
          )}
          {hasMore && <div ref={sentinelRef} aria-hidden="true" className="h-px" />}
        </>
      )}
      {removeError !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{removeError}</AlertTitle>
        </Alert>
      )}
    </>
  );
}
