import { Menu, RefreshCw } from "lucide-react";
import { Link } from "react-router";

import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";
import Tooltip from "../ui/Tooltip";
import { describeScan, useScan } from "./ScanProvider";

function ScanButton() {
  const scan = useScan();
  const description = describeScan(scan);
  const progress =
    scan.scan?.state === "running" && scan.scan.total > 0
      ? scan.scan.completed / scan.scan.total
      : null;

  return (
    <Tooltip content={description}>
      <button
        type="button"
        onClick={scan.start}
        disabled={scan.running}
        aria-label={scan.running ? description : "ライブラリを更新"}
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
        className="mr-2 flex h-8 items-center gap-2 rounded-md px-2 text-base font-semibold tracking-tight text-fg select-none hover:bg-hover-wash"
      >
        <span className="size-2.5 rounded-full bg-accent" aria-hidden="true" />
        vv
      </Link>

      <div className="ml-auto flex items-center gap-0.5">
        <ScanButton />
      </div>
    </header>
  );
}
