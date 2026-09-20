import { useCallback, useEffect, useState } from "react";

const storageKey = "vv.sidebar.v2";
const railBreakpoint = "(max-width: 1023px)";
const drawerBreakpoint = "(max-width: 639px)";

export type SidebarMode = "expanded" | "rail" | "drawer";

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
  return typeof window !== "undefined" && window.matchMedia(query).matches;
}

/**
 * useSidebar はサイドバーの開閉を画面幅と利用者の選択から決める。
 *
 * - 1024px 以上: 利用者の選択（既定は開）。閉じるとレール（アイコンのみ）。
 * - 640–1023px: 既定はレール。開くとレールが展開に変わる。
 * - 639px 以下: ドロワー。開くとオーバーレイで被さる。
 */
export function useSidebar(): {
  mode: SidebarMode;
  open: boolean;
  toggle: () => void;
  close: () => void;
} {
  const [narrow, setNarrow] = useState(() => matches(railBreakpoint));
  const [phone, setPhone] = useState(() => matches(drawerBreakpoint));
  const [preferred, setPreferred] = useState<boolean | undefined>(readStored);
  const [drawerOpen, setDrawerOpen] = useState(false);

  useEffect(() => {
    const rail = window.matchMedia(railBreakpoint);
    const drawer = window.matchMedia(drawerBreakpoint);
    const sync = () => {
      setNarrow(rail.matches);
      setPhone(drawer.matches);
    };
    rail.addEventListener("change", sync);
    drawer.addEventListener("change", sync);
    return () => {
      rail.removeEventListener("change", sync);
      drawer.removeEventListener("change", sync);
    };
  }, []);

  const open = phone ? drawerOpen : (preferred ?? !narrow);

  const toggle = useCallback(() => {
    if (phone) {
      setDrawerOpen((value) => !value);
      return;
    }
    setPreferred((current) => {
      const next = !(current ?? !narrow);
      writeStored(next);
      return next;
    });
  }, [narrow, phone]);

  const close = useCallback(() => setDrawerOpen(false), []);

  const mode: SidebarMode = phone ? "drawer" : open ? "expanded" : "rail";
  return { mode, open, toggle, close };
}
