import { ChevronRight, X } from "lucide-react";
import { useRef, useState } from "react";

import { errorText, formatNumber, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import Button from "../ui/Button";
import Chip from "../ui/Chip";
import Skeleton from "../ui/Skeleton";

/**
 * RejectedNames はタグ管理画面の一覧の下の「却下した名前」の折りたたみである
 * （specs/031-tentative-tags/ui-design.md「Rejected names」）。一覧の取得と
 * 取り直しは呼び出し元（TagsPage）が持ち、ここは見出し・中身・取り外しを持つ。
 *
 * `names` が undefined の間は読み込み中（`error` があれば読み込み失敗）で、
 * 見出しの件数は出さない。開閉はこの画面の状態で、閉じた状態で始まる。
 */
export default function RejectedNames({
  names,
  error,
  onRetry,
  onForget,
}: {
  names: readonly string[] | undefined;
  error: UiText | null;
  onRetry: () => void;
  /** 名前を外す。成功すれば呼び出し元が `names` からその名前を取り除く。 */
  onForget: (name: string) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [removing, setRemoving] = useState<ReadonlySet<string>>(new Set());
  const [removeError, setRemoveError] = useState<UiText | null>(null);
  const headingButton = useRef<HTMLButtonElement | null>(null);
  const removeButtons = useRef(new Map<string, HTMLButtonElement>());
  const removingRef = useRef(new Set<string>());

  /**
   * forget は × の1回の押下を扱う。送信中の名前の × は二度押しを無視する。
   * 成功したら、次のチップの ×、無ければ前のチップの ×、最後の1つなら
   * 見出しのボタンへフォーカスを移す（014 の再生画面の × と同じ規則）。
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
        (target ?? headingButton.current)?.focus();
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
    <section className="mt-6">
      <h2>
        <button
          ref={headingButton}
          type="button"
          aria-expanded={open}
          onClick={() => setOpen((value) => !value)}
          className="inline-flex items-center gap-1.5 rounded-sm text-sm font-medium text-fg hover:text-link"
        >
          <ChevronRight
            aria-hidden="true"
            className={cn(
              "size-4 shrink-0 transition-transform duration-150 motion-reduce:transition-none",
              open && "rotate-90",
            )}
          />
          {t.tags.rejectedNames.heading}
          {names !== undefined && (
            <span className="text-xs text-fg-muted tabular-nums">
              {formatNumber(names.length)}
            </span>
          )}
        </button>
      </h2>
      {open && (
        <div className="mt-2">
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
          {names !== undefined && names.length === 0 && (
            <p className="text-xs text-fg-muted">{t.tags.rejectedNames.empty}</p>
          )}
          {names !== undefined && names.length > 0 && (
            <>
              <p className="text-xs text-fg-muted">{t.tags.rejectedNames.description}</p>
              <ul
                aria-label={t.tags.rejectedNames.heading}
                className="mt-2 flex flex-wrap gap-1.5"
              >
                {names.map((name) => (
                  <li key={name} className="min-w-0 max-w-full">
                    <Chip title={name} className="max-w-full gap-0 pr-0">
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
              </ul>
            </>
          )}
          {removeError !== null && (
            <p role="alert" className="mt-2 text-xs text-danger">
              {removeError}
            </p>
          )}
        </div>
      )}
    </section>
  );
}
