import { Menu, RefreshCw } from "lucide-react";
import { Link } from "react-router";

import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";
import Tooltip from "../ui/Tooltip";
import { describeScan, useScan } from "./ScanProvider";

function ScanButton() {
  const scan = useScan();
  const description = describeScan(scan);
  const buttonDescription = scan.canStart
    ? scan.running
      ? description
      : "ライブラリを更新"
    : "メディアフォルダを設定してください";
  const progress =
    scan.scan?.state === "running" && scan.scan.total > 0
      ? scan.scan.completed / scan.scan.total
      : null;

  return (
    <Tooltip content={buttonDescription}>
      <button
        type="button"
        onClick={scan.start}
        disabled={scan.running || !scan.canStart}
        aria-label={buttonDescription}
        className={cn(
          "inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-sm transition-colors select-none",
          scan.error !== null
            ? "bg-danger-soft text-danger"
            : scan.running
              ? "bg-accent-soft text-link"
              : "text-fg hover:bg-hover-wash active:bg-active-wash",
        )}
      >
        <RefreshCw
          className={cn(
            "size-4",
            scan.running && "animate-spin motion-reduce:animate-none",
          )}
        />
        <span className="hidden tabular-nums md:inline">
          {scan.running
            ? progress === null
              ? "更新中"
              : `${String(Math.round(progress * 100))}%`
            : "更新"}
        </span>
        <span role="status" aria-live="polite" className="sr-only">
          {description}
        </span>
      </button>
    </Tooltip>
  );
}

/** TopBar は ☰・ロゴ・更新だけを持つ。ナビと設定は Sidebar にある。 */
export default function TopBar({ onMenu }: { onMenu: () => void }) {
  return (
    <header className="fixed inset-x-0 top-0 z-40 flex h-navbar items-center gap-1 border-b border-border bg-bg/90 px-2 backdrop-blur-md sm:px-3">
      <IconButton label="メニュー" onClick={onMenu} tooltip={false}>
        <Menu />
      </IconButton>
      <Link
        to="/"
        className="mr-1 hidden h-8 shrink-0 items-center gap-2 rounded-md px-2 text-base font-semibold tracking-tight text-fg select-none hover:bg-hover-wash sm:flex"
      >
        <span className="size-2.5 rounded-full bg-accent" aria-hidden="true" />
        vv
      </Link>

      <div id="topbar-library-tools" className="flex min-w-0 flex-1 items-center" />

      <div className="ml-auto flex shrink-0 items-center gap-0.5">
        <ScanButton />
      </div>
    </header>
  );
}
