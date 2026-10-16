import { useLayoutEffect, useRef } from "react";

import { scrollIntoStrip } from "../lib/scrollIntoStrip";
import { formatNumber, t } from "../i18n";
import { Tabs, TabsList, TabsTrigger } from "../ui/shadcn/tabs";
import type { TagListTab } from "./tagListUrl";

/** tagsTabId はタブの id である。パネルの `aria-labelledby` とフォーカスの行き先に使う。 */
export function tagsTabId(prefix: string, value: TagListTab): string {
  return `${prefix}-tab-${value}`;
}

/** tagsPanelId はタブのパネルの id である。 */
export function tagsPanelId(prefix: string, value: TagListTab): string {
  return `${prefix}-panel-${value}`;
}

/**
 * TagsTabs は「Tags」「Rejected names」「Kinds」のタブである（ui-design.md「Tabs」、
 * specs/051-tag-kinds/ui-design.md「Kinds tab」）。それぞれに
 * 件数を添え、読み込む前は出さない。`locked` の間（作成・改名の送信中）は、描いて
 * いないタブを押せず、矢印でも移らない。3 つのタブが幅に収まらないとき（320px で
 * 4 桁の件数が付くなど）は、画面ではなくタブの並びを包む帯が横に送れるようにし、
 * 選んでいるタブ（URL から開いたとき、戻る・進むも）を帯の見える位置へ送る。帯は
 * フォーカスの輪（外へ 4px）が切れない余白を持ち、同じ幅の負の余白で並びの位置を保つ。
 */
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
  // 描く前に送る（送る前の位置を一瞬見せない）。件数が届いてタブの幅が変わったときや、
  // 帯の幅が変わったときは、描き終えた大きさで送り直す。
  // 帯とタブの並びはいつも描くので、layout effect の時点で strip と並びはある。
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
