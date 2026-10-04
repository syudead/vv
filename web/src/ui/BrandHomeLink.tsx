import { Link } from "react-router";

import { t } from "../i18n";
import { cn } from "../lib/cn";

/**
 * BrandHomeLink は上部バーのロゴで、ライブラリへ戻るリンクである。リンクが行き先の名前を
 * 1 度だけ持ち、幅に応じた 2 つの画像は飾りとして隠す。使い方は
 * web/registry/rules/components.md「BrandHomeLink」。
 */
export default function BrandHomeLink({ className }: { className?: string }) {
  return (
    <Link
      to="/"
      aria-label={t.common.brandHome}
      className={cn(
        "flex h-8 shrink-0 items-center rounded-sm px-1 hover:bg-accent focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none",
        className,
      )}
    >
      <img
        src="/brand/vvmdm-symbol-cyan.svg"
        alt=""
        aria-hidden="true"
        className="size-8 sm:hidden"
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
