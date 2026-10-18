import { type ReactNode, useLayoutEffect, useState } from "react";
import { createPortal } from "react-dom";

const targetId = "topbar-library-tools";

export default function TopBarPortal({ children }: { children: ReactNode }) {
  const [target, setTarget] = useState<HTMLElement | null>(null);

  useLayoutEffect(() => {
    setTarget(document.getElementById(targetId));
  }, []);

  return target === null ? children : createPortal(children, target);
}
