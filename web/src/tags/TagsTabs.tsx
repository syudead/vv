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
 * TagsTabs は「Tags」「Rejected names」のタブである（ui-design.md「Tabs」）。それぞれに
 * 件数を添え、読み込む前は出さない。`locked` の間（作成・改名の送信中）は、描いて
 * いないタブを押せず、矢印でも移らない。
 */
export default function TagsTabs({
  idPrefix,
  tab,
  onChange,
  tagsCount,
  rejectedCount,
  locked,
}: {
  idPrefix: string;
  tab: TagListTab;
  onChange: (next: TagListTab) => void;
  tagsCount: number | undefined;
  rejectedCount: number | undefined;
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
  return (
    <Tabs
      value={tab}
      onValueChange={(next) => {
        onChange(next as TagListTab);
      }}
    >
      <TabsList aria-label={t.tags.tabs.label}>
        {trigger("tags", t.tags.tabs.tags, tagsCount)}
        {trigger("rejected", t.tags.rejectedNames.heading, rejectedCount)}
      </TabsList>
    </Tabs>
  );
}
