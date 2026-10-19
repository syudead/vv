import { Globe } from "lucide-react";

import type { WorkInfoPlugin } from "../api/workInfoPlugins";
import { t, type UiText } from "../i18n";
import { FactList, type Fact } from "../ui/patterns/fact-list";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Badge } from "../ui/shadcn/badge";
import WorkInfoPluginDialog from "./WorkInfoPluginDialog";
import { operationTexts, pluginName } from "./workInfoPluginRows";

function facts(plugin: WorkInfoPlugin): Fact[] {
  const text = t.settings.workInfoPlugins.approval;
  const list: Fact[] = [];
  if (plugin.version !== undefined)
    list.push({
      id: "version",
      term: text.version,
      value: <span className="tabular-nums">{plugin.version}</span>,
    });
  list.push(
    {
      id: "file",
      term: text.file,
      value: <span className="font-mono break-all">{plugin.fileName}</span>,
    },
    { id: "can", term: text.can, value: operationTexts(plugin).join(" · ") },
  );
  if (plugin.kind === "person" && plugin.sources.length > 0)
    list.push({
      id: "sources",
      term: text.sources,
      value: (
        <span className="flex flex-wrap gap-1">
          {plugin.sources.map((source) => (
            <Badge key={source} variant="secondary">
              {source}
            </Badge>
          ))}
        </span>
      ),
    });
  if (plugin.description !== undefined)
    list.push({
      id: "description",
      term: text.pluginDescription,
      value: plugin.description,
    });
  list.push({
    id: "digest",
    term: text.digest,
    value: <span className="font-mono text-xs break-all">{plugin.digest}</span>,
  });
  return list;
}

function SettingsItReceives({ plugin }: { plugin: WorkInfoPlugin }) {
  const text = t.settings.workInfoPlugins.approval;
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-sm font-medium">{text.settingsHeading}</h3>
      {plugin.settings.length === 0 ? (
        <p className="text-sm text-muted-foreground">{text.noSettings}</p>
      ) : (
        <ul className="flex flex-col gap-1 text-sm">
          {plugin.settings.map((setting) => (
            <li key={setting.key} className="flex flex-wrap items-center gap-2">
              <span className="break-words">{setting.label}</span>
              {setting.secret && <Badge variant="secondary">{text.secret}</Badge>}
              {setting.required && <Badge variant="secondary">{text.required}</Badge>}
              {setting.default !== undefined && (
                <span className="min-w-0 text-muted-foreground wrap-anywhere">
                  {t.settings.workInfoPlugins.defaultHint(setting.default)}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** Approve the displayed digest; disable approval if the plugin changed or became unavailable. */
export default function WorkInfoPluginApprovalDialog({
  plugin,
  pending,
  changed,
  error,
  onClose,
  onApprove,
}: {
  plugin: WorkInfoPlugin;
  pending: boolean;
  changed: boolean;
  error: UiText | null;
  onClose: () => void;
  onApprove: (digest: string) => void;
}) {
  const text = t.settings.workInfoPlugins.approval;
  return (
    <WorkInfoPluginDialog
      title={text.title(pluginName(plugin))}
      description={plugin.kind === "person" ? text.personDescription : text.description}
      submitLabel={text.submit}
      pending={pending}
      submitDisabled={changed}
      failure={changed ? text.changed : error}
      onClose={onClose}
      onSubmit={() => onApprove(plugin.digest)}
    >
      <FactList facts={facts(plugin)} />
      <Alert variant="warning" role="note">
        <Globe aria-hidden="true" />
        <AlertTitle>{text.networkTitle}</AlertTitle>
        <AlertDescription>{text.networkBody}</AlertDescription>
      </Alert>
      <SettingsItReceives plugin={plugin} />
    </WorkInfoPluginDialog>
  );
}
