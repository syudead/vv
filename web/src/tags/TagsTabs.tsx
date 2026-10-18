import { useLayoutEffect, useRef } from "react";

import { scrollIntoStrip } from "../lib/scrollIntoStrip";
import { formatNumber, t } from "../i18n";
import { Tabs, TabsList, TabsTrigger } from "../ui/shadcn/tabs";
import type { TagListTab } from "./tagListUrl";

export function tagsTabId(prefix: string, value: TagListTab): string {
  return `${prefix}-tab-${value}`;
}

export function tagsPanelId(prefix: string, value: TagListTab): string {
  return `${prefix}-panel-${value}`;
}

/** Scroll the tab strip, not the page, on narrow layouts; retain focus-ring padding and block tab changes while pending. */
export default function TagsTabs({
  idPrefix,
  tab,
  onChange,
  tagsCount,
  rejectedCount,
  kindsCount,
  locked,
}: {
  idPrefix: string;
  tab: TagListTab;
  onChange: (next: TagListTab) => void;
  tagsCount: number | undefined;
  rejectedCount: number | undefined;
  kindsCount: number | undefined;
  locked: boolean;
}) {
  const trigger = (value: TagListTab, label: string, count: number | undefined) => (
    <TabsTrigger
      value={value}
      id={tagsTabId(idPrefix, value)}
      aria-controls={tagsPanelId(idPrefix, value)}
      disabled={locked && tab !== value}
    >
      {label}
      {count !== undefined && (
        <span className="font-normal text-muted-foreground tabular-nums">
          {formatNumber(count)}
        </span>
      )}
    </TabsTrigger>
  );
  const strip = useRef<HTMLDivElement>(null);
  // Scroll before paint, then remeasure when counts or strip width change.
  useLayoutEffect(() => {
    const root = strip.current as HTMLDivElement;
    const show = () => {
      const active = root.querySelector<HTMLElement>('[role="tab"][data-state="active"]');
      if (active !== null) scrollIntoStrip(root, active);
    };
    show();
    const observer = new ResizeObserver(show);
    observer.observe(root);
    observer.observe(root.firstElementChild as Element);
    return () => {
      observer.disconnect();
    };
  }, [tab]);
  return (
    <Tabs
      value={tab}
      onValueChange={(next) => {
        onChange(next as TagListTab);
      }}
    >
      <div ref={strip} className="-m-1 overflow-x-auto p-1">
        <TabsList aria-label={t.tags.tabs.label}>
          {trigger("tags", t.tags.tabs.tags, tagsCount)}
          {trigger("rejected", t.tags.rejectedNames.heading, rejectedCount)}
          {trigger("kinds", t.tags.kinds.tab, kindsCount)}
        </TabsList>
      </div>
    </Tabs>
  );
}
