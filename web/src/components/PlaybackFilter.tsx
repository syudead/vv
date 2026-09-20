import { useEffect, useRef, useState } from "react";

import Icon from "../layout/icons";

export type PlaybackFilterValue = "all" | "unwatched" | "inProgress" | "watched";

const options: readonly { value: PlaybackFilterValue; label: string }[] = [
  { value: "all", label: "すべて" },
  { value: "unwatched", label: "未再生" },
  { value: "inProgress", label: "視聴途中" },
  { value: "watched", label: "視聴済み" },
];

export default function PlaybackFilter({
  value,
  onChange,
}: {
  value: PlaybackFilterValue;
  onChange: (value: PlaybackFilterValue) => void;
}) {
  const root = useRef<HTMLDetailsElement>(null);
  const trigger = useRef<HTMLElement | null>(null);
  const [open, setOpen] = useState(false);
  const selectedLabel = options.find((option) => option.value === value)?.label;

  useEffect(() => {
    const closeFromOutside = (event: PointerEvent) => {
      if (root.current?.open && !root.current.contains(event.target as Node)) {
        root.current?.removeAttribute("open");
        setOpen(false);
      }
    };
    const closeFromEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && root.current?.open) {
        root.current?.removeAttribute("open");
        setOpen(false);
        trigger.current?.focus();
      }
    };

    document.addEventListener("pointerdown", closeFromOutside);
    document.addEventListener("keydown", closeFromEscape);
    return () => {
      document.removeEventListener("pointerdown", closeFromOutside);
      document.removeEventListener("keydown", closeFromEscape);
    };
  }, []);

  const choose = (next: PlaybackFilterValue) => {
    onChange(next);
    root.current?.removeAttribute("open");
    setOpen(false);
    trigger.current?.focus();
  };

  return (
    <details
      ref={root}
      onToggle={(event) => setOpen(event.currentTarget.open)}
      className="playback-filter group/filter relative"
    >
      <summary
        ref={trigger}
        className={
          "playback-filter-trigger flex min-h-[var(--size-tap)] cursor-pointer list-none items-center gap-2 rounded-control border px-3 text-sm outline-none transition-colors " +
          "hover:border-body/80 hover:bg-body/10 active:bg-surface-sunken " +
          "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus motion-reduce:transition-none " +
          "[&::-webkit-details-marker]:hidden " +
          (open || value !== "all"
            ? "border-accent bg-accent-surface text-accent"
            : "border-border bg-surface-raised text-body")
        }
      >
        <Icon name="filter" className="h-4 w-4" />
        <span className="playback-filter-label">
          {value === "all" ? "絞り込み" : selectedLabel}
        </span>
      </summary>

      <div className="absolute top-full right-0 z-40 mt-2 w-44 rounded-card border border-border bg-surface-raised p-1.5 shadow-xl">
        <p className="px-2 py-1.5 text-xs font-medium text-muted">再生状態</p>
        {options.map((option) => (
          <button
            key={option.value}
            type="button"
            aria-pressed={value === option.value}
            tabIndex={open ? 0 : -1}
            onClick={() => choose(option.value)}
            className={
              "flex w-full items-center gap-2 rounded-control px-2 text-left text-sm outline-none transition-colors " +
              "hover:bg-body/10 active:bg-surface-sunken focus-visible:outline-2 focus-visible:outline-offset-0 focus-visible:outline-focus motion-reduce:transition-none " +
              (value === option.value ? "bg-accent-surface text-accent" : "text-body")
            }
          >
            <span className="flex h-4 w-4 items-center justify-center">
              {value === option.value && <Icon name="check" className="h-4 w-4" />}
            </span>
            {option.label}
          </button>
        ))}
      </div>
    </details>
  );
}
