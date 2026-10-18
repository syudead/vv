import { type ReactNode, useLayoutEffect, useRef, useState } from "react";

import { fitBox } from "./editRules";

/** Fit and center the aspect-ratio box within its measured container; runtime dimensions require inline styles. */
export default function FittedBox({
  aspect,
  children,
}: {
  aspect: number;
  children: ReactNode;
}) {
  const area = useRef<HTMLDivElement | null>(null);
  const [size, setSize] = useState({ width: 0, height: 0 });

  useLayoutEffect(() => {
    const element = area.current;
    if (element === null) return;
    const measure = () =>
      setSize((previous) => {
        const next = { width: element.clientWidth, height: element.clientHeight };
        return previous.width === next.width && previous.height === next.height
          ? previous
          : next;
      });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const box = fitBox(size.width, size.height, aspect);
  return (
    <div ref={area} className="relative size-full min-h-0 min-w-0">
      <div
        className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2"
        style={{ width: box.width, height: box.height }}
      >
        {children}
      </div>
    </div>
  );
}
