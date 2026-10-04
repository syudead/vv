import { useEffect, useState } from "react";

// shadcn/ui の use-mobile。vv の狭い幅（Sidebar がドロワーになる幅）は 639px 以下で、
// library-ui.md「Width breakpoints in CSS」の sm に合わせる。
const MOBILE_BREAKPOINT = 640;

function query(): MediaQueryList | null {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function")
    return null;
  return window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`);
}

export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = useState(() => query()?.matches ?? false);

  useEffect(() => {
    const mql = query();
    if (mql === null) return;
    const onChange = () => setIsMobile(mql.matches);
    mql.addEventListener("change", onChange);
    onChange();
    return () => mql.removeEventListener("change", onChange);
  }, []);

  return isMobile;
}
