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
 * useSidebarOpen は 640px 以上のサイドバーの開閉（展開かレールか）を、画面幅と利用者の
 * 選択から決める。値は shadcn/ui の SidebarProvider の `open` と `onOpenChange` に渡す。
 *
 * - 1024px 以上: 利用者の選択（既定は開）。閉じるとレール（アイコンのみ）。
 * - 640–1023px: 既定はレール。開くとレールが展開に変わる。
 * - 639px 以下はドロワーで、SidebarProvider が自分で開閉を持つ（保存しない）。
 */
export function useSidebarOpen(): { open: boolean; setOpen: (open: boolean) => void } {
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
