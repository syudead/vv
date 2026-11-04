import { t } from "../i18n";
import { Label } from "../ui/shadcn/label";
import { Switch } from "../ui/shadcn/switch";

/**
 * The word and the track are one control in a fixed column, so every row switch of the page lines up.
 * Below sm it takes its own line under the path, track first at the left, where FormRow stacks the page's other switches.
 */
export function FolderSwitch({
  enabled,
  saving,
  label,
  onToggle,
}: {
  enabled: boolean;
  saving: boolean;
  label: string;
  onToggle: () => void;
}) {
  const text = t.settings.mediaFolders;
  return (
    <div className="flex basis-full sm:shrink-0 sm:basis-auto">
      <Label className="h-8 w-16 cursor-pointer justify-end text-xs font-normal text-muted-foreground max-sm:flex-row-reverse">
        {enabled ? text.on : text.off}
        <Switch
          checked={enabled}
          aria-label={label}
          aria-disabled={saving || undefined}
          onCheckedChange={onToggle}
        />
      </Label>
    </div>
  );
}
