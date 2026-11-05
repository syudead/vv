import type { KeyboardEvent, ReactNode, Ref } from "react";

import { isComposingNativeKeyEvent } from "@/lib/ime";

// Side-panel skeleton; see web/registry/rules/patterns.md, Side panel.
// From lg a spacer in the page's row takes the panel's width, so the page shrinks beside the fixed
// column while the window keeps scrolling the page; below lg the column is a layer with no scrim.

export interface SidePanelProps {
  id?: string;
  /** The complementary region's accessible name. */
  label: string;
  /** The header's title, set as an h2. */
  title: ReactNode;
  /** `ghost` `icon-sm` buttons at the header's end, the close button last. */
  actions?: ReactNode;
  /** Stays at the bottom while the body scrolls. */
  footer?: ReactNode;
  children: ReactNode;
  /** Called on Esc with the focus inside the panel. */
  onClose: () => void;
  bodyRef?: Ref<HTMLDivElement>;
}

export function SidePanel({
  id,
  label,
  title,
  actions,
  footer,
  children,
  onClose,
  bodyRef,
}: SidePanelProps) {
  const closeOnEscape = (event: KeyboardEvent) => {
    if (event.key !== "Escape" || event.defaultPrevented) return;
    if (isComposingNativeKeyEvent(event.nativeEvent)) return;
    event.preventDefault();
    onClose();
  };
  return (
    <>
      <div
        aria-hidden="true"
        data-slot="side-panel-space"
        className="hidden shrink-0 lg:block lg:w-assistant-panel"
      />
      <aside
        id={id}
        aria-label={label}
        data-slot="side-panel"
        onKeyDown={closeOnEscape}
        className="fixed top-navbar right-0 bottom-0 z-30 flex w-full animate-slide-up flex-col border-l border-border bg-card text-card-foreground shadow-elevated motion-reduce:animate-none sm:w-assistant-panel lg:animate-none lg:shadow-none"
      >
        <div
          data-slot="side-panel-header"
          className="flex h-navbar shrink-0 items-center gap-1 border-b border-border pr-2 pl-3"
        >
          <h2 className="min-w-0 flex-1 truncate text-sm font-semibold">{title}</h2>
          {actions}
        </div>
        <div
          ref={bodyRef}
          data-slot="side-panel-body"
          className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-3"
        >
          {children}
        </div>
        {footer !== undefined && (
          <div
            data-slot="side-panel-footer"
            className="shrink-0 border-t border-border p-3"
          >
            {footer}
          </div>
        )}
      </aside>
    </>
  );
}
