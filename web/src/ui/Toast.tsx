import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useSyncExternalStore,
} from "react";
import { toast as sonner } from "sonner";

import { t, type UiText } from "../i18n";
import { Toaster } from "./shadcn/sonner";

// アプリの通知の出し口。表示は shadcn/ui の Sonner（web/registry/rules/components.md
// 「Sonner」）に任せ、ここは同時に出す数だけを決める。一覧の画面では新しい通知を
// すぐ出して最大 3 件、再生画面では 1 件ずつ出し、残りは前のものが消えてから出す。
// シェルで出た通知を持ったまま再生画面へ移っても、待っているものは順に出る。

/** visibleLimit は配置ごとの同時に出す数である。 */
const visibleLimit = { default: 3, playback: 1 } as const;

/** toastDuration は通知を出しておく時間（ms）である。 */
export const toastDuration = 2800;

type Placement = keyof typeof visibleLimit;

const ToastContext = createContext<(message: UiText) => void>(() => undefined);

/** useToast は短い通知を出す関数を返す。 */
export function useToast(): (message: UiText) => void {
  return useContext(ToastContext);
}

interface ToastItem {
  id: string;
  message: UiText;
}

let sequence = 0;

function nextToastId(): string {
  sequence += 1;
  return `vv-toast-${String(sequence)}`;
}

/** wideQuery は一覧の画面の通知を下端の中央へ移す幅（Tailwind の lg）である。 */
const wideQuery = "(min-width: 64rem)";

function subscribeWide(onChange: () => void): () => void {
  if (typeof window.matchMedia !== "function") return () => undefined;
  const query = window.matchMedia(wideQuery);
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
}

function readWide(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia(wideQuery).matches;
}

/**
 * useWide は lg 以上かを返す。Sonner の置き場は CSS の幅の分岐では変えられないので、
 * 通知の置き場だけは幅を読む（docs/design-docs/library-ui.md「Width breakpoints in CSS」）。
 */
function useWide(): boolean {
  return useSyncExternalStore(subscribeWide, readWide, () => false);
}

/**
 * toasterPlacement は Sonner の置き場である。一覧の画面は、lg より狭い幅ではトップバーの
 * 下の右端に出してページの道具に重ねず、lg 以上では下端の中央（選択バーの上）に出し、
 * 一括の操作の結果を操作した場所の近くで読ませる。再生画面では上端の中央に出す
 * （右上には見出しの帯の閉じる × がある）。配置が変わると Sonner は通知を描き直すので、
 * 出ている通知はその時点から出し直しになる。
 */
function toasterPlacement(placement: Placement, wide: boolean) {
  if (placement === "playback") {
    return {
      position: "top-center",
      offset: { top: "0.375rem" },
      // 狭い幅でも右上の閉じる × に重ならないよう、左右を 4rem ずつ空ける。
      mobileOffset: { top: "0.375rem", left: "4rem", right: "4rem" },
    } as const;
  }
  if (wide) {
    return {
      position: "bottom-center",
      // 下端に貼り付く選択バー（bottom-3 と h-selection-bar）の上に出す。
      offset: { bottom: "calc(var(--spacing-selection-bar) + 2rem)" },
    } as const;
  }
  const top = "calc(var(--spacing-navbar) + 0.5rem)";
  return {
    position: "top-right",
    offset: { top, right: "0.75rem" },
    mobileOffset: { top, left: "0.75rem", right: "0.75rem" },
  } as const;
}

export function ToastProvider({
  children,
  placement = "default",
}: {
  children: ReactNode;
  placement?: Placement;
}) {
  // 出している通知（古い順）と、出すのを待っている通知。
  const shown = useRef<ToastItem[]>([]);
  const waiting = useRef<ToastItem[]>([]);
  const limit = useRef<number>(visibleLimit[placement]);
  const wide = useWide();

  // display は Sonner に 1 件を渡す。消えたら（時間切れでも閉じても）待ちから次を出す。
  const display = useCallback((item: ToastItem) => {
    shown.current.push(item);
    const closed = () => {
      const index = shown.current.findIndex((entry) => entry.id === item.id);
      if (index === -1) return;
      shown.current.splice(index, 1);
      while (shown.current.length < limit.current) {
        const next = waiting.current.shift();
        if (next === undefined) return;
        display(next);
      }
    };
    sonner(item.message, {
      id: item.id,
      duration: toastDuration,
      onAutoClose: closed,
      onDismiss: closed,
    });
  }, []);

  const show = useCallback(
    (message: UiText) => {
      const item = { id: nextToastId(), message };
      if (shown.current.length < limit.current) {
        display(item);
        return;
      }
      if (limit.current === visibleLimit.default) {
        // 一覧の画面では新しい通知を待たせず、いちばん古いものと入れ替える。
        const oldest = shown.current.shift();
        if (oldest !== undefined) sonner.dismiss(oldest.id);
        display(item);
        return;
      }
      waiting.current.push(item);
    },
    [display],
  );

  // 配置が変わったら数を合わせる。再生画面へ移ったときは先頭の 1 件だけを残して
  // 残りを待ちの先頭へ戻し、先頭の 1 件は出し続ける。一覧へ戻ったら
  // 待っているものを 3 件まで出す。
  useEffect(() => {
    limit.current = visibleLimit[placement];
    const extra = shown.current.splice(limit.current);
    for (const item of extra) sonner.dismiss(item.id);
    // 戻した通知は新しい id で出し直す。同じ id のまま出すと、Sonner は消す途中の
    // 通知の更新として扱い、消さずに残す。
    waiting.current.unshift(...extra.map((item) => ({ ...item, id: nextToastId() })));
    while (shown.current.length < limit.current) {
      const next = waiting.current.shift();
      if (next === undefined) break;
      display(next);
    }
  }, [display, placement]);

  // 外れるときは通知をすべて片付ける。Sonner の通知は画面の外に残り、次に置いた
  // Toaster が出し直すため。アプリに ToastProvider は 1 つだけである。
  useEffect(
    () => () => {
      shown.current = [];
      waiting.current = [];
      sonner.dismiss();
    },
    [],
  );

  return (
    <ToastContext.Provider value={show}>
      {children}
      <Toaster
        {...toasterPlacement(placement, wide)}
        expand
        visibleToasts={visibleLimit.default}
        customAriaLabel={t.common.notifications}
      />
    </ToastContext.Provider>
  );
}
