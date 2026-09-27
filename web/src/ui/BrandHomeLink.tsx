import { Link } from "react-router";

import { t } from "../i18n";
import { cn } from "../lib/cn";

/** The link names the destination once; its two responsive images are decorative. */
export default function BrandHomeLink({ className }: { className?: string }) {
  return (
    <Link
      to="/"
      aria-label={t.common.brandHome}
      className={cn(
        "flex h-8 shrink-0 items-center rounded-sm px-1 hover:bg-hover-wash focus-visible:ring-2 focus-visible:ring-link focus-visible:outline-none",
        className,
      )}
    >
      <img
        src="/brand/vvmdm-symbol-cyan.svg"
        alt=""
        aria-hidden="true"
        className="size-7 sm:hidden"
      />
      <img
        src="/brand/vvmdm-wordmark-cyan.svg"
        alt=""
        aria-hidden="true"
        className="hidden h-8 w-auto sm:block"
      />
    </Link>
  );
}
