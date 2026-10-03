import { X } from "lucide-react";
import { type RefObject, useMemo, useRef, useState } from "react";

import { errorText, formatNumber, t, type UiText } from "../i18n";
import Button from "../ui/Button";
import Chip from "../ui/Chip";
import { ModalFrame } from "../ui/ModalFrame";
import Skeleton from "../ui/Skeleton";

/**
 * RejectedNames はタグ管理画面の件数の行の右端の「Rejected names」の入口と、
 * 押すと開く窓である（specs/036-tag-admin-scale/ui-design.md「Count line」
 * 「Rejected names」）。窓の中身・取り外し・取り直しの規則は 031 のまま
 * （specs/031-tentative-tags/ui-design.md「Rejected names」）。一覧の取得と
 * 取り直しは呼び出し元（TagsPage）が持ち、ここは入口・窓・取り外しを持つ。
 *
 * `names` が undefined の間は読み込み中（`error` があれば読み込み失敗）で、
 * 入口の件数は出さない。開閉はこの画面の状態で、URL には載せない。
 */
export default function RejectedNames({
  names,
  error,
  onRetry,
  onForget,
  className,
}: {
  names: readonly string[] | undefined;
  error: UiText | null;
  onRetry: () => void;
  /** 名前を外す。成功すれば呼び出し元が `names` からその名前を取り除く。 */
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
        {names !== undefined && (
          <span className="text-fg-muted tabular-nums">{formatNumber(names.length)}</span>
        )}
      </Button>
      {open && (
        <RejectedNamesDialog
          names={names}
          error={error}
          onRetry={onRetry}
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
  error,
  onRetry,
  onForget,
  onClose,
}: {
  names: readonly string[] | undefined;
  error: UiText | null;
  onRetry: () => void;
  onForget: (name: string) => Promise<void>;
  onClose: () => void;
}) {
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
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4 sm:p-5">
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
            </ul>
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
