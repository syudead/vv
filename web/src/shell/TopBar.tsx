import { Menu, Play, RefreshCw } from "lucide-react";
import { Link } from "react-router";

import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";
import Tooltip from "../ui/Tooltip";
import { describeScan, useScan } from "./ScanProvider";
import SearchBox from "./SearchBox";

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
          "relative inline-flex h-9 items-center gap-2 rounded-full border px-3 text-sm font-medium transition-colors select-none",
          scan.error !== null
            ? "border-danger/40 bg-danger-soft text-danger"
            : scan.running
              ? "border-accent/40 bg-accent-soft text-accent-hover"
              : "border-border bg-surface text-fg-muted hover:border-border-strong hover:bg-surface-hover hover:text-fg",
        )}
      >
        <RefreshCw
          className={cn(
            "size-4",
            scan.running && "animate-spin motion-reduce:animate-none",
          )}
        />
        <span className="hidden sm:inline">
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

export default function TopBar({ onMenu }: { onMenu: () => void }) {
  return (
    <header className="fixed inset-x-0 top-0 z-40 flex h-topbar items-center gap-2 border-b border-border bg-bg/85 px-3 backdrop-blur-md sm:px-4">
      <div className="flex shrink-0 items-center gap-1">
        <IconButton label="メニュー" onClick={onMenu} size="lg" tooltip={false}>
          <Menu />
        </IconButton>
        <Link
          to="/"
          className="flex h-10 items-center gap-2 rounded-md px-2 text-fg select-none"
          aria-label="vv ホーム"
        >
          <span className="flex size-7 items-center justify-center rounded-md bg-accent text-accent-fg">
            <Play className="size-3.5 fill-current" />
          </span>
          <span className="hidden text-lg font-semibold tracking-tight sm:inline">
            vv
          </span>
        </Link>
      </div>

      <div className="flex min-w-0 flex-1 justify-center px-2 sm:px-6">
        <SearchBox className="max-w-2xl" />
      </div>

      <div className="flex shrink-0 items-center gap-2">
        <ScanButton />
      </div>
    </header>
  );
}
