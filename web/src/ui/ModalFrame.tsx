import { X } from "lucide-react";
import { type ReactNode, type RefObject, useEffect, useRef } from "react";
import { createPortal } from "react-dom";

import IconButton from "./IconButton";

function focusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(
    container.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => !element.hasAttribute("hidden"));
}

/**
 * ModalFrame は確認・選択の窓の共通の骨組みである（Plan の Structural
 * Decisions 9）。フォーカスの取り込み・Esc での閉じ方・背景の inert 化を持ち、
 * 設定画面のフォルダ選択・削除の確認とタグ管理画面の確認の窓が共有する。
 */
export function ModalFrame({
  title,
  onClose,
  children,
  initialFocus,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  initialFocus?: RefObject<HTMLElement | null>;
}) {
  const panel = useRef<HTMLDivElement>(null);
  const overlay = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const titleId = `dialog-${title.replaceAll(" ", "-")}`;

  useEffect(() => {
    const previous =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const background = Array.from(document.body.children).filter(
      (element) => element !== overlay.current,
    );
    const states = background.map((element) => ({
      element,
      inert: element.hasAttribute("inert"),
      ariaHidden: element.getAttribute("aria-hidden"),
    }));
    for (const element of background) {
      element.setAttribute("inert", "");
      element.setAttribute("aria-hidden", "true");
    }
    const keydown = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseRef.current();
        return;
      }
      if (event.key !== "Tab" || panel.current === null) return;
      const elements = focusableElements(panel.current);
      if (elements.length === 0) return;
      const first = elements[0]!;
      const last = elements.at(-1)!;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", keydown);
    return () => {
      document.removeEventListener("keydown", keydown);
      for (const state of states) {
        state.element.toggleAttribute("inert", state.inert);
        if (state.ariaHidden === null) state.element.removeAttribute("aria-hidden");
        else state.element.setAttribute("aria-hidden", state.ariaHidden);
      }
      queueMicrotask(() => previous?.focus());
    };
  }, []);

  useEffect(() => {
    const target = initialFocus?.current ?? focusableElements(panel.current!)[0];
    target?.focus();
  }, [initialFocus]);

  return createPortal(
    <div
      ref={overlay}
      className="fixed inset-0 z-50 flex items-stretch justify-center bg-overlay sm:items-center sm:p-6"
    >
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="flex min-h-0 w-full flex-col bg-elevated shadow-elevated sm:max-h-[calc(100dvh-3rem)] sm:max-w-2xl sm:rounded-lg sm:border sm:border-border-strong"
      >
        <div className="flex h-14 shrink-0 items-center gap-3 border-b border-border px-4">
          <h2 id={titleId} className="min-w-0 flex-1 truncate text-base font-semibold">
            {title}
          </h2>
          <IconButton label="閉じる" onClick={onClose} tooltip={false}>
            <X />
          </IconButton>
        </div>
        {children}
      </div>
    </div>,
    document.body,
  );
}

export default ModalFrame;
