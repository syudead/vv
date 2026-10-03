import { X } from "lucide-react";
import { type RefObject, useEffect, useMemo, useRef, useState } from "react";

import type { RejectedTagNameList } from "../api/tags";
import { errorText, formatNumber, t, type UiText } from "../i18n";
import Button from "../ui/Button";
import Chip from "../ui/Chip";
import { ModalFrame } from "../ui/ModalFrame";
import Skeleton from "../ui/Skeleton";

/**
 * RejectedNames はタグ管理画面の件数の行の右端の「Rejected names」の入口と、
 * 押すと開く窓である（specs/036-tag-admin-scale/ui-design.md「Count line」
 * 「Rejected names」）。窓の中身・取り外しの規則は 031 のまま
 * （specs/031-tentative-tags/ui-design.md「Rejected names」）。一覧の取得・続き・
 * 取り直しは呼び出し元（TagsPage）が持ち、ここは入口・窓・取り外しと、窓の中身を
 * 末尾までスクロールしたことを伝える番兵を持つ。
 *
 * `page` が undefined の間は読み込み中（`error` があれば読み込み失敗）で、
 * 入口の件数は出さない。件数は `page.total`（読み込んでいない名前も数えた数）。
 * 開閉はこの画面の状態で、URL には載せない。閉じても読み込んだ続きは
 * 呼び出し元に残るので、開き直せば同じ並びが出る。
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
  className,
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
  /** 先頭のページを受け直すたびに変わる。窓のスクロール位置を先頭へ戻す。 */
  resetKey: number;
  /** 名前を外す。成功すれば呼び出し元が `page` からその名前を取り除く。 */
  onForget: (name: string) => Promise<void>;
  className?: string;
}) {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        aria-haspopup="dialog"
        onClick={() => setOpen(true)}
        className={className}
      >
        {t.tags.rejectedNames.heading}
        {page !== undefined && (
          <span className="text-fg-muted tabular-nums">{formatNumber(page.total)}</span>
        )}
      </Button>
      {open && (
        <RejectedNamesDialog
          names={page?.items}
          hasMore={page?.nextCursor !== undefined}
          error={error}
          onRetry={onRetry}
          morePending={morePending}
          moreError={moreError}
          onLoadMore={onLoadMore}
          resetKey={resetKey}
          onForget={onForget}
          // 閉じるとフォーカスは ModalFrame が開く前の要素（入口）へ戻す。
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

function RejectedNamesDialog({
  names,
  hasMore,
  error,
  onRetry,
  morePending,
  moreError,
  onLoadMore,
  resetKey,
  onForget,
  onClose,
}: {
  names: readonly string[] | undefined;
  hasMore: boolean;
  error: UiText | null;
  onRetry: () => void;
  morePending: boolean;
  moreError: UiText | null;
  onLoadMore: () => void;
  resetKey: number;
  onForget: (name: string) => Promise<void>;
  onClose: () => void;
}) {
  const boxRef = useRef<HTMLDivElement | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const onLoadMoreRef = useRef(onLoadMore);
  onLoadMoreRef.current = onLoadMore;
  const loadedCount = names?.length ?? 0;

  // 先頭のページを受け直したら、並びと一緒にスクロール位置も先頭へ戻す。
  useEffect(() => {
    if (boxRef.current !== null) boxRef.current.scrollTop = 0;
  }, [resetKey]);

  // 窓の中身の箱を根にした番兵。末尾が見えたら続きを読む。読み込みのたびに
  // 見張り直すので、届いた分で末尾がまだ見えていればもう 1 ページ読む。
  // 先頭のページを受け直したときも見張り直す（取り直しの間に見えた番兵の通知は
  // 呼び出し元が無視するので、件数が変わらなければ次の通知が来ない）。
  // 失敗の間は読まない（Retry を押すまで）。
  useEffect(() => {
    const root = boxRef.current;
    const target = sentinelRef.current;
    if (root === null || target === null || !hasMore || morePending || moreError !== null)
      return;
    if (typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onLoadMoreRef.current();
      },
      { root, rootMargin: "0px 0px 120px 0px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasMore, morePending, moreError, loadedCount, resetKey]);

  const [removing, setRemoving] = useState<ReadonlySet<string>>(new Set());
  const [removeError, setRemoveError] = useState<UiText | null>(null);
  const closeButton = useRef<HTMLButtonElement | null>(null);
  const removeButtons = useRef(new Map<string, HTMLButtonElement>());
  const removingRef = useRef(new Set<string>());
  const namesRef = useRef(names);
  namesRef.current = names;

  /**
   * initialFocus は窓を開いた直後のフォーカス先で、最初の名前の ×、無ければ
   * 「Close」である（ui-design.md「Rejected names」）。
   */
  const initialFocus = useMemo<RefObject<HTMLElement | null>>(
    () => ({
      get current() {
        const first = namesRef.current?.[0];
        const remove = first === undefined ? undefined : removeButtons.current.get(first);
        return remove ?? closeButton.current;
      },
    }),
    [],
  );

  /**
   * forget は × の1回の押下を扱う。送信中の名前の × は二度押しを無視する。
   * 成功したら、次のチップの ×、無ければ前のチップの ×、最後の1つなら
   * 「Close」へフォーカスを移す（窓の見出しはフォーカスを持たないので、031 の
   * 見出しのボタンの代わりに窓の中の残る操作へ移す）。
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
        (target ?? closeButton.current)?.focus();
      }, 0);
    } catch (failure) {
      setRemoveError(t.tags.rejectedNames.removeFailed(name, errorText(failure)));
      // 送信中に disabled にした × はフォーカスを失うので、チップを残したまま
      // その × へ戻す。
      setTimeout(() => removeButtons.current.get(name)?.focus(), 0);
    } finally {
      removingRef.current.delete(name);
      setRemoving(new Set(removingRef.current));
    }
  }

  return (
    <ModalFrame
      title={t.tags.rejectedNames.heading}
      onClose={onClose}
      initialFocus={initialFocus}
      width="sm:max-w-lg"
    >
      <div
        ref={boxRef}
        className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4 sm:p-5"
      >
        {names === undefined && error === null && (
          <div className="flex flex-wrap gap-1.5" aria-hidden="true">
            {Array.from({ length: 3 }, (_, index) => (
              <Skeleton key={index} className="h-6 w-24" />
            ))}
          </div>
        )}
        {names === undefined && error !== null && (
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-xs text-danger">{t.tags.rejectedNames.loadFailed}</p>
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
          <p className="text-xs text-fg-muted">{t.tags.rejectedNames.empty}</p>
        )}
        {names !== undefined && (names.length > 0 || hasMore) && (
          <>
            <p className="text-xs text-fg-muted">{t.tags.rejectedNames.description}</p>
            <ul
              aria-label={t.tags.rejectedNames.heading}
              aria-busy={morePending || undefined}
              className="flex flex-wrap gap-1.5"
            >
              {names.map((name) => (
                <li key={name} className="min-w-0 max-w-full">
                  <Chip tone="onElevated" title={name} className="max-w-full gap-0 pr-0">
                    <span className="min-w-0 truncate">{name}</span>
                    <button
                      ref={(node) => {
                        if (node) removeButtons.current.set(name, node);
                        else removeButtons.current.delete(name);
                      }}
                      type="button"
                      aria-label={t.tags.rejectedNames.allow(name)}
                      disabled={removing.has(name)}
                      onClick={() => void forget(name)}
                      className="flex size-6 shrink-0 items-center justify-center rounded-sm enabled:hover:bg-hover-wash disabled:opacity-50"
                    >
                      <X className="size-3" aria-hidden="true" />
                    </button>
                  </Chip>
                </li>
              ))}
              {morePending &&
                Array.from({ length: 3 }, (_, index) => (
                  <li key={`more-${index}`} aria-hidden="true">
                    <Skeleton className="h-6 w-24" />
                  </li>
                ))}
            </ul>
            {moreError !== null && (
              <div className="flex flex-wrap items-center gap-2">
                <p role="alert" className="text-xs text-danger">
                  {t.tags.rejectedNames.loadMoreFailed}
                </p>
                <Button variant="ghost" size="sm" onClick={onLoadMore}>
                  {t.common.retry}
                </Button>
              </div>
            )}
            {hasMore && (
              <div ref={sentinelRef} aria-hidden="true" className="h-px shrink-0" />
            )}
          </>
        )}
        {removeError !== null && (
          <p role="alert" className="text-xs text-danger">
            {removeError}
          </p>
        )}
      </div>
      <div className="flex shrink-0 justify-end border-t border-border p-4">
        <Button ref={closeButton} onClick={onClose}>
          {t.common.close}
        </Button>
      </div>
    </ModalFrame>
  );
}
