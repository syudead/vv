import { useEffect, useState } from "react";

// Match the shared sm breakpoint: mobile sidebar applies through 639px.
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
