import { type ReactNode, useLayoutEffect, useState } from "react";
import { createPortal } from "react-dom";

const targetId = "topbar-library-tools";

/** TopBarPortal はページ固有の操作を共通トップバーの中央へ配置する。 */
export default function TopBarPortal({ children }: { children: ReactNode }) {
  const [target, setTarget] = useState<HTMLElement | null>(null);

  useLayoutEffect(() => {
    setTarget(document.getElementById(targetId));
  }, []);

  return target === null ? children : createPortal(children, target);
}
