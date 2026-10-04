import { useCallback, useEffect, useState } from "react";

const storageKey = "vv.sidebar.v2";
const railBreakpoint = "(max-width: 1023px)";

function readStored(): boolean | undefined {
  try {
    const raw = localStorage.getItem(storageKey);
    return raw === null ? undefined : raw === "open";
  } catch {
    return undefined;
  }
}

function writeStored(open: boolean): void {
  try {
    localStorage.setItem(storageKey, open ? "open" : "closed");
  } catch {
    // 保存できなくても今の画面では切り替えられる
  }
}

function matches(query: string): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia(query).matches
  );
}

/**
 * useSidebarPreference は、広い幅のサイドバーを展開にするかレールにするかを、画面幅と
 * 利用者の選択から決める。SidebarProvider（ui/sidebar）の `open` と `onOpenChange` に渡す。
 * 狭い幅（639px 以下）のドロワーの開閉は SidebarProvider 自身が持つ。
 *
 * - 1024px 以上: 利用者の選択（既定は展開）。閉じるとレール（アイコンのみ）。
 * - 640–1023px: 既定はレール。開くとレールが展開に変わる。
 */
export function useSidebarPreference(): {
  open: boolean;
  setOpen: (open: boolean) => void;
} {
  const [narrow, setNarrow] = useState(() => matches(railBreakpoint));
  const [preferred, setPreferred] = useState<boolean | undefined>(readStored);

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const rail = window.matchMedia(railBreakpoint);
    const sync = () => setNarrow(rail.matches);
    rail.addEventListener("change", sync);
    return () => rail.removeEventListener("change", sync);
  }, []);

  const setOpen = useCallback((next: boolean) => {
    writeStored(next);
    setPreferred(next);
  }, []);

  return { open: preferred ?? !narrow, setOpen };
}
