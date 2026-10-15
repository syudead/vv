import {
  CircleAlert,
  CircleX,
  Globe,
  ShieldAlert,
  ShieldCheck,
  SlidersHorizontal,
} from "lucide-react";
import { Fragment, type ReactNode, type Ref } from "react";

import type { WorkInfoPlugin, WorkInfoPluginState } from "../api/workInfoPlugins";
import { t, type UiText } from "../i18n";
import { RowMoreMenu } from "../tags/TagRow";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { DropdownMenuItem } from "../ui/shadcn/dropdown-menu";
import {
  missingRequired,
  operationTexts,
  pluginName,
  settingsSummary,
} from "./workInfoPluginRows";

/** RowWork は行で進んでいる取り消しか削除である。 */
export type RowWork = "revoke" | "delete";

const badgeVariant = {
  approved: "success",
  unapproved: "warning",
  invalid: "destructive",
} as const satisfies Record<WorkInfoPluginState, string>;

const badgeIcon = {
  approved: <ShieldCheck aria-hidden="true" />,
  unapproved: <ShieldAlert aria-hidden="true" />,
  invalid: <CircleX aria-hidden="true" />,
} satisfies Record<WorkInfoPluginState, ReactNode>;

function FactsLine({ plugin }: { plugin: WorkInfoPlugin }) {
  const text = t.settings.workInfoPlugins;
  const parts: { id: string; node: ReactNode }[] = [];
  const operations = operationTexts(plugin);
  if (operations.length > 0)
    parts.push({ id: "operations", node: operations.join(" · ") });
  if (plugin.network)
    parts.push({
      id: "network",
      node: (
        <span className="inline-flex items-center gap-1">
          <Globe aria-hidden="true" className="size-3" />
          {text.network}
        </span>
      ),
    });
  const summary = settingsSummary(plugin);
  if (summary !== null) parts.push({ id: "settings", node: summary });
  return (
    <p className="text-xs break-words text-muted-foreground">
      {parts.map((part, index) => (
        <Fragment key={part.id}>
          {index > 0 && " · "}
          {part.node}
        </Fragment>
      ))}
    </p>
  );
}

function RowText({ plugin }: { plugin: WorkInfoPlugin }) {
  const text = t.settings.workInfoPlugins;
  const missing = missingRequired(plugin);
  const invalid = plugin.state === "invalid";
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-1">
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <Badge variant={badgeVariant[plugin.state]}>
          {badgeIcon[plugin.state]}
          {text.states[plugin.state]}
        </Badge>
        {plugin.name === undefined ? (
          <span className="min-w-0 font-mono break-all">{plugin.fileName}</span>
        ) : (
          <span className="min-w-0 font-medium break-words">{plugin.name}</span>
        )}
        {plugin.version !== undefined && (
          <span className="text-muted-foreground tabular-nums">
            {text.version(plugin.version)}
          </span>
        )}
      </div>
      {plugin.description !== undefined && (
        <p className="truncate text-muted-foreground" title={plugin.description}>
          {plugin.description}
        </p>
      )}
      {invalid ? (
        plugin.problem !== undefined && (
          <p className="text-destructive">{text.problems[plugin.problem]}</p>
        )
      ) : (
        <FactsLine plugin={plugin} />
      )}
      {!invalid && missing.length > 0 && (
        <p className="inline-flex items-center gap-1 text-warning">
          <CircleAlert aria-hidden="true" className="size-4 shrink-0" />
          {text.requiredMissing(missing)}
        </p>
      )}
    </div>
  );
}

/**
 * WorkInfoPluginRow は「Work-info plugins」節の 1 行である。状態のバッジを先頭に名前と版、
 * 説明、事実の行を出し、状態ごとの操作を右端に置く。360px では操作が文の下へ回る
 * （specs/049-work-info-plugins/ui-design.md「Plugin row」「Responsive behaviour」）。
 */
export default function WorkInfoPluginRow({
  plugin,
  work,
  error,
  rowRef,
  onApprove,
  onSettings,
  onRevoke,
  onDelete,
}: {
  plugin: WorkInfoPlugin;
  work: RowWork | null;
  error: UiText | null;
  rowRef: Ref<HTMLDivElement>;
  onApprove: () => void;
  onSettings: () => void;
  onRevoke: () => void;
  onDelete: () => void;
}) {
  const text = t.settings.workInfoPlugins;
  const name = pluginName(plugin);
  const busy = work !== null;
  const showSettings = plugin.state !== "invalid" && plugin.settings.length > 0;
  return (
    <div
      ref={rowRef}
      data-plugin-id={plugin.id}
      className="flex min-w-0 flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:flex-wrap sm:items-start"
    >
      <RowText plugin={plugin} />
      <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
        {work !== null && (
          <span role="status" className="text-xs text-muted-foreground">
            {work === "revoke" ? text.revoking : text.deleting}
          </span>
        )}
        {plugin.state === "unapproved" && (
          <Button size="sm" onClick={onApprove} disabled={busy}>
            {text.approve}
          </Button>
        )}
        {showSettings && (
          <Button variant="outline" size="sm" onClick={onSettings} disabled={busy}>
            <SlidersHorizontal aria-hidden="true" />
            {text.settings}
          </Button>
        )}
        <RowMoreMenu buttonRef={null} label={text.more(name)} disabled={busy}>
          {plugin.state === "approved" && (
            <DropdownMenuItem onSelect={onRevoke}>{text.revoke}</DropdownMenuItem>
          )}
          <DropdownMenuItem variant="destructive" onSelect={onDelete}>
            {text.delete}
          </DropdownMenuItem>
        </RowMoreMenu>
      </div>
      {error !== null && (
        <p role="alert" className="text-destructive sm:basis-full">
          {error}
        </p>
      )}
    </div>
  );
}
